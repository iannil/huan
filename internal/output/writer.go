// Package output handles writing rendered HTML and assets to the publish directory.
package output

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Writer writes files to the publish directory.
// Configure it before writing; cleaning must occur outside the write stage.
// minify v2.24.13 documents Minify/String as concurrent-safe. Registration
// happens only in NewMinifier. CSS/SVG copy options per call; HTML/JS/JSON/XML
// keep working state local (HTML's deprecated mutating option is disabled).
// Thus different paths can minify, canonify and write concurrently.
type Writer struct {
	mu        sync.Mutex // statistics only
	pathLocks [256]sync.Mutex

	publishDir string
	minifier   *Minifier
	canonOpts  *CanonifyOptions
	dirs       sync.Map // directories already created by MkdirAll
	written    int
	bytes      int64
}

// NewWriter creates a new Writer targeting publishDir without minification.
func NewWriter(publishDir string) *Writer {
	return &Writer{publishDir: publishDir}
}

// NewWriterWithMinify creates a Writer that minifies output before writing.
func NewWriterWithMinify(publishDir string) *Writer {
	return &Writer{publishDir: publishDir, minifier: NewMinifier()}
}

// SetCanonify enables root-relative URL canonification with the given options.
func (w *Writer) SetCanonify(opts CanonifyOptions) {
	w.canonOpts = &opts
}

// URLToFilePath converts a URL path to an output file path under publishDir.
// Hugo convention: /foo/ → publishDir/foo/index.html
func URLToFilePath(url, publishDir string) string {
	clean := strings.TrimPrefix(url, "/")
	clean = strings.TrimSuffix(clean, "/")

	if clean == "" {
		return filepath.Join(publishDir, "index.html")
	}
	return filepath.Join(publishDir, clean, "index.html")
}

// PathToFilePath maps a path (without trailing slash) directly.
// e.g., ("sitemap.xml") → publishDir/sitemap.xml
// e.g., ("posts/index.xml") → publishDir/posts/index.xml
func PathToFilePath(path, publishDir string) string {
	clean := strings.TrimPrefix(path, "/")
	return filepath.Join(publishDir, clean)
}

// Write writes content to a file path under publishDir.
// Creates parent directories as needed. If a minifier is set, content is
// minified according to the file's media type before writing. If canonify
// is set, root-relative URLs in HTML are rewritten to absolute URLs.
func (w *Writer) Write(relPath, content string) error {
	lock := w.pathMutex(relPath)
	lock.Lock()
	defer lock.Unlock()
	if w.minifier != nil {
		content = w.minifier.Minify(relPath, content)
	}
	if w.canonOpts != nil && mediaTypeForExt(relPath) == "text/html" {
		opts := *w.canonOpts
		// Home page (top-level index.html) gets the Hugo generator meta tag.
		opts.IsHome = isHomePath(relPath)
		content = Canonify(content, opts)
	}

	fullPath := PathToFilePath(relPath, w.publishDir)
	dir := filepath.Dir(fullPath)
	if err := w.ensureDir(dir); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if err := writeStringFile(fullPath, content, 0644); err != nil {
		return fmt.Errorf("write %s: %w", fullPath, err)
	}

	w.recordWrite(int64(len(content)))
	return nil
}

// writeStringFile follows os.WriteFile's create/truncate and error semantics,
// using File.WriteString to avoid allocating a full copy of rendered output.
func writeStringFile(path, content string, perm fs.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.WriteString(content)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// ensureDir creates dir once per Writer lifetime; subsequent calls with the
// same dir skip the MkdirAll syscall chain. Directories are never removed
// during a build, so a created dir stays valid for the Writer's lifetime.
func (w *Writer) ensureDir(dir string) error {
	if _, ok := w.dirs.Load(dir); ok {
		return nil
	}
	// Most output paths are new leaves under a parent created by an earlier
	// page. Mkdir avoids MkdirAll's leaf and parent Stat calls in that case.
	// Fall back unchanged for missing parents, existing paths and all errors.
	if err := os.Mkdir(dir, 0755); err != nil {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	w.dirs.Store(dir, struct{}{})
	return nil
}

// isHomePath returns true if relPath is the site's home page output or one of
// its paginated aliases (/page/N/index.html). Hugo injects the generator meta
// into all of these.
func isHomePath(relPath string) bool {
	if relPath == "index.html" {
		return true
	}
	if strings.HasPrefix(relPath, "page/") && strings.HasSuffix(relPath, "/index.html") {
		return true
	}
	return false
}

// WriteBytesPath writes raw bytes WITHOUT applying minify/canonify.
// Use for pre-formatted content that should be emitted verbatim.
func (w *Writer) WriteBytesPath(relPath string, data []byte) error {
	lock := w.pathMutex(relPath)
	lock.Lock()
	defer lock.Unlock()
	fullPath := PathToFilePath(relPath, w.publishDir)
	dir := filepath.Dir(fullPath)
	if err := w.ensureDir(dir); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", fullPath, err)
	}
	w.recordWrite(int64(len(data)))
	return nil
}

// WriteBytes writes raw bytes to a file path under publishDir.
// Minification is applied if a minifier is set.
func (w *Writer) WriteBytes(relPath string, data []byte) error {
	lock := w.pathMutex(relPath)
	lock.Lock()
	defer lock.Unlock()
	if w.minifier != nil {
		data = w.minifier.MinifyBytes(relPath, data)
	}

	fullPath := PathToFilePath(relPath, w.publishDir)
	dir := filepath.Dir(fullPath)
	if err := w.ensureDir(dir); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", fullPath, err)
	}
	w.recordWrite(int64(len(data)))
	return nil
}

// CopyStatic copies all files from srcDir into publishDir, preserving relative
// paths. Files whose slash-relative path starts with one of the excludes
// prefixes (e.g. "en/") are skipped; nil/empty excludes copies everything.
func (w *Writer) CopyStatic(srcDir string, excludes []string) error {
	_, err := os.Stat(srcDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return filepath.Walk(srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		for _, ex := range excludes {
			if strings.HasPrefix(relPath, ex) {
				return nil
			}
		}

		return w.copyFile(path, relPath)
	})
}

func (w *Writer) copyFile(src, relPath string) error {
	lock := w.pathMutex(relPath)
	lock.Lock()
	defer lock.Unlock()
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	dst := PathToFilePath(relPath, w.publishDir)
	if err := w.ensureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy to %s: %w", dst, err)
	}

	var size int64
	if info, err := os.Stat(src); err == nil {
		size = info.Size()
	}
	w.recordWrite(size)
	return nil
}

// CleanPublishDir removes the publish directory.
// Use cautiously - this is destructive.
func CleanPublishDir(publishDir string) error {
	return os.RemoveAll(publishDir)
}

// Stats returns the number of files written and total bytes.
func (w *Writer) Stats() (files int, bytes int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written, w.bytes
}

// pathMutex bounds lock storage and serializes aliases of the same clean path.
// Hash collisions only reduce concurrency.
func (w *Writer) pathMutex(relPath string) *sync.Mutex {
	path := filepath.Clean(PathToFilePath(relPath, w.publishDir))
	var hash uint32 = 2166136261
	for i := 0; i < len(path); i++ {
		hash = (hash ^ uint32(path[i])) * 16777619
	}
	return &w.pathLocks[hash%uint32(len(w.pathLocks))]
}

func (w *Writer) recordWrite(size int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.written++
	w.bytes += size
}
