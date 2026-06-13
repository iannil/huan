package cloudflare

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// Hard limits enforced by Cloudflare Pages (per wrangler constants.ts).
// Manifest build rejects inputs exceeding these so we fail fast in the
// manifest stage rather than mid-upload.
//
// "bucket" here = one POST /pages/assets/upload request body, NOT per-
// deployment total. A 20,000-file deployment is split into many buckets
// of at most MaxFilesPerBatch files or MaxBatchSize bytes (whichever hits
// first) per ADR §14.4.
const (
	MaxFileCount     = 20000              // total files per deployment
	MaxFileSize      = 25 * 1024 * 1024   // 25 MiB per file
	MaxFilesPerBatch = 2000               // files per upload bucket (POST request)
	MaxBatchSize     = 40 * 1024 * 1024   // bytes per upload bucket (POST request)
)

// Asset describes one file in a Pages deployment manifest.
type Asset struct {
	// Path is the deployment-relative path with leading slash, e.g. "/index.html".
	// Cloudflare requires leading slash on manifest keys; BuildManifest enforces this.
	Path string

	// Hash is the 32-hex blake3 hash (see hash.go).
	Hash string

	// Size is the file size in bytes.
	Size int64

	// ContentType is the MIME type guessed from extension (e.g. "text/html; charset=utf-8").
	// Empty if no mapping found; Cloudflare accepts empty content-type.
	ContentType string

	// Content is the raw file bytes, loaded at manifest build time. For large
	// deployments this can be memory-heavy; if memory becomes a concern, switch
	// to lazy loading in pages.go upload step.
	Content []byte
}

// ManifestError reports a structured error from manifest building, including
// the file path and limit name so the user can fix the input.
type ManifestError struct {
	Path    string
	Limit   string // "MaxFileSize" / "MaxFileCount" / "MaxFilesPerBatch"
	Details string
}

func (e *ManifestError) Error() string {
	return fmt.Sprintf("manifest: %s: %s (%s)", e.Path, e.Details, e.Limit)
}

// BuildManifest walks publishDir and returns Asset entries ready for upload.
//
// Path conventions:
//   - publishDir itself becomes the deployment root; e.g. publishDir/index.html
//     becomes Asset.Path "/index.html".
//   - Subdirectories preserved: publishDir/blog/2024/post.html -> "/blog/2024/post.html".
//   - All paths use forward slashes regardless of host OS (Cloudflare expects /).
//
// Hard limits enforced (see constants). Returned error unwraps to *ManifestError
// for the first violation encountered.
//
// Symbolic links are skipped (not followed) to avoid cycles and unexpected
// out-of-tree content. Hidden files (leading dot) are included — Pages typically
// wants .well-known/, robots.txt, etc. Callers can post-filter if needed.
func BuildManifest(publishDir string) ([]Asset, error) {
	info, err := os.Stat(publishDir)
	if err != nil {
		return nil, fmt.Errorf("manifest: stat publishDir %q: %w", publishDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("manifest: publishDir %q is not a directory", publishDir)
	}

	var assets []Asset
	walkErr := filepath.WalkDir(publishDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Skip symlinks (avoid cycles and out-of-tree content).
		if d.Type() == os.ModeSymlink {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", path, err)
		}

		if info.Size() > MaxFileSize {
			return &ManifestError{
				Path:    path,
				Limit:   "MaxFileSize",
				Details: fmt.Sprintf("file size %d bytes exceeds %d bytes (%d MiB)", info.Size(), MaxFileSize, MaxFileSize/(1024*1024)),
			}
		}

		// Enforce total file count up front.
		if len(assets) >= MaxFileCount {
			return &ManifestError{
				Path:    path,
				Limit:   "MaxFileCount",
				Details: fmt.Sprintf("file count would exceed %d", MaxFileCount),
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}

		relPath, err := filepath.Rel(publishDir, path)
		if err != nil {
			return fmt.Errorf("rel path %q: %w", path, err)
		}
		// Cloudflare requires leading slash on manifest keys.
		deployPath := "/" + filepath.ToSlash(relPath)
		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		contentType := guessContentType(path)

		assets = append(assets, Asset{
			Path:        deployPath,
			Hash:        Hash(content, ext),
			Size:        info.Size(),
			ContentType: contentType,
			Content:     content,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return assets, nil
}

// Batch splits assets into chunks suitable for POST /pages/assets/upload.
// Each chunk satisfies BOTH constraints:
//   - len(chunk) <= MaxFilesPerBatch (2000)
//   - sum(chunk[i].Size) <= MaxBatchSize (40 MiB)
//
// Whichever constraint hits first triggers a new bucket. Per ADR §14.4 /
// wrangler constants.ts: "bucket" = one POST request body, not per-
// deployment total.
//
// Returns nil if input is empty.
func Batch(assets []Asset) [][]Asset {
	if len(assets) == 0 {
		return nil
	}
	var batches [][]Asset
	var current []Asset
	var currentSize int64
	for _, a := range assets {
		if len(current) >= MaxFilesPerBatch || currentSize+a.Size > MaxBatchSize {
			if len(current) > 0 {
				batches = append(batches, current)
			}
			current = nil
			currentSize = 0
		}
		current = append(current, a)
		currentSize += a.Size
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// guessContentType returns the MIME type for the given filename, with a UTF-8
// charset suffix for text types. Returns "" for unknown extensions.
func guessContentType(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return ""
	}
	// mime.TypeByExtension returns "text/html; charset=utf-8" etc. on most
	// systems; we trust the registry but fall back to "" on miss.
	t := mime.TypeByExtension(ext)
	return t
}

// readAll is a thin wrapper kept for symmetry with future streaming variants.
// Currently identical to os.ReadFile but isolated for future lazy-load swap.
func readAll(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
