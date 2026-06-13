package cloudflare

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iannil/huan/internal/deploy"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// R2EndpointHostPattern is the hostname pattern for Cloudflare R2's S3-
// compatible API. minio.New takes a bare hostname (no scheme); the scheme is
// controlled by Options.Secure (set to true for R2).
const R2EndpointHostPattern = "%s.r2.cloudflarestorage.com"

// r2ObjectClient is the minimal subset of minio.Client operations R2Syncer
// needs. Extracted as an interface for test substitution (we mock this rather
// than fake the full S3 wire protocol).
type r2ObjectClient interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
	ListObjects(ctx context.Context, bucket, prefix string) ([]R2Object, error)
	PutObject(ctx context.Context, bucket, key, contentType string, r io.Reader, size int64) error
	RemoveObject(ctx context.Context, bucket, key string) error
}

// R2Object is the subset of remote-object metadata R2Syncer uses.
type R2Object struct {
	Key  string
	Size int64
	ETag string // hex-encoded MD5 without quotes
}

// R2Syncer uploads local files to a Cloudflare R2 bucket via S3-compatible
// API. See ADR 0002 §6 for the strategy.
type R2Syncer struct {
	client r2ObjectClient
	bucket string
	logger *deploy.Logger
}

// NewR2Syncer constructs an R2Syncer with a minio-go-backed client. accountID
// and access keys come from cloudflare.Config.R2; for test substitution use
// NewR2SyncerWithClient.
func NewR2Syncer(cfg R2Config, logger *deploy.Logger) (*R2Syncer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// minio.New takes a bare hostname (no scheme); the scheme is set via
	// Options.Secure. R2 requires HTTPS.
	endpoint := fmt.Sprintf(R2EndpointHostPattern, cfg.AccountID)
	if cfg.Endpoint != "" {
		endpoint = cfg.Endpoint
	}
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Region:       "auto",
		Secure:       true,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("init minio client: %w", err)
	}
	minioClient.SetAppInfo("huan", "0.1.0")
	// Silence minio's verbose internal logging; structured logs go through our logger.
	log.SetOutput(io.Discard)
	return &R2Syncer{
		client: &minioObjectClient{c: minioClient},
		bucket: cfg.Bucket,
		logger: logger,
	}, nil
}

// NewR2SyncerWithClient lets tests inject a mock r2ObjectClient.
func NewR2SyncerWithClient(client r2ObjectClient, bucket string, logger *deploy.Logger) *R2Syncer {
	return &R2Syncer{client: client, bucket: bucket, logger: logger}
}

// R2SyncOptions configures a single sync invocation.
type R2SyncOptions struct {
	// Prune deletes remote objects whose keys aren't present in the local
	// set. Default false (keep orphans) per ADR 0002 §6.
	Prune bool

	// DryRun computes the diff but performs no network calls (upload/delete).
	DryRun bool

	// Concurrency caps CPU-bound work (file walk, MD5 hashing). HTTP upload
	// parallelism is governed separately (default 3).
	Concurrency int
}

// R2SyncResult is the per-invocation outcome. Counts mirror deploy.Report
// semantics but are scoped to R2 only.
type R2SyncResult struct {
	Attempted int      // local files considered
	Succeeded int      // uploads completed
	Skipped   int      // remote already had matching MD5
	Failed    int      // uploads that exhausted retries
	Pruned    int      // remote objects deleted (--prune)
	Failures  []R2FileFailure
}

// R2FileFailure describes a single file that failed during R2 sync.
type R2FileFailure struct {
	LocalPath string
	Key       string
	Stage     string // "walk" / "list" / "hash" / "upload" / "prune"
	Error     string
}

// Sync runs the R2 sync algorithm against the given local-to-remote mappings.
//
// Algorithm:
//  1. Verify bucket exists (fail fast if not — ADR 0002 §10).
//  2. Walk local paths → build {key → localPath} map for each sync.from→to.
//  3. For each unique "to" prefix, list remote objects in one call.
//  4. Compare: skip if remote exists with matching MD5; upload otherwise.
//  5. If Prune: delete remote keys not in local set.
//
// Per ADR 0002 §9 collection-not-interruption: per-file failures are
// collected into Failures; the sync continues.
func (s *R2Syncer) Sync(ctx context.Context, mappings []SyncMapping, opts R2SyncOptions) (*R2SyncResult, error) {
	start := time.Now()
	result := &R2SyncResult{}

	// Step 1: verify bucket exists.
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return result, fmt.Errorf("r2 check bucket: %w", err)
	}
	if !exists {
		return result, fmt.Errorf("r2 bucket %q does not exist (create it in Cloudflare dashboard first per ADR 0002 §10)", s.bucket)
	}

	// Step 2: walk local paths → map of key → localPath + sizes + md5.
	localObjs, walkErrs := s.walkLocal(ctx, mappings, result)
	if len(walkErrs) > 0 {
		// walk errors are not fatal — we sync what we can.
		s.logger.Log("r2-walk", deploy.EventError, map[string]any{
			"walk_errors": len(walkErrs),
		})
	}
	result.Attempted = len(localObjs)

	if opts.DryRun {
		// In dry-run, just compute what would happen without network.
		// We still list remote so the user sees accurate skip/upload counts.
		remote, err := s.listAll(ctx, mappings)
		if err != nil {
			return result, fmt.Errorf("r2 list (dry-run): %w", err)
		}
		for key, local := range localObjs {
			if remoteObj, ok := remote[key]; ok && remoteObj.ETag == local.MD5 {
				result.Skipped++
			} else {
				result.Succeeded++ // would-be-uploaded
			}
		}
		if opts.Prune {
			for key := range remote {
				if _, ok := localObjs[key]; !ok {
					result.Pruned++ // would-be-pruned
				}
			}
		}
		s.logger.Log("r2-sync", deploy.EventFunctionEnd, map[string]any{
			"dry_run":    true,
			"attempted":  result.Attempted,
			"would_skip": result.Skipped,
			"would_upload": result.Succeeded,
			"would_prune": result.Pruned,
			"duration_ms": time.Since(start).Milliseconds(),
		})
		// Reset Succeeded for dry-run; caller interprets Succeeded=0 as "nothing actually sent".
		result.Succeeded = 0
		return result, nil
	}

	// Step 3: list remote objects per unique "to" prefix.
	remote, err := s.listAll(ctx, mappings)
	if err != nil {
		return result, fmt.Errorf("r2 list: %w", err)
	}

	// Step 4: upload missing / mismatched. Uses Limiter (HTTPMaxParallel=3)
	// to cap concurrent PUT requests per ADR §14.3. opts.Concurrency governs
	// CPU-bound work (file walk, MD5 hashing) — but those are currently serial
	// in walkLocal; future enhancement. HTTP concurrency is hard-capped at 3
	// and not user-tunable.
	limiter := NewLimiter()
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, le := range localObjs {
		wg.Add(1)
		go func(le localEntry) {
			defer wg.Done()
			if err := limiter.Acquire(ctx); err != nil {
				return // ctx cancelled during acquire
			}
			defer limiter.Release()

			remoteObj, exists := remote[le.Key]
			if exists && remoteObj.ETag == le.MD5 {
				mu.Lock()
				result.Skipped++
				mu.Unlock()
				return
			}
			if err := s.uploadOne(ctx, le); err != nil {
				mu.Lock()
				result.Failed++
				result.Failures = append(result.Failures, R2FileFailure{
					LocalPath: le.LocalPath,
					Key:       le.Key,
					Stage:     "upload",
					Error:     err.Error(),
				})
				mu.Unlock()
				return
			}
			mu.Lock()
			result.Succeeded++
			mu.Unlock()
		}(le)
	}
	wg.Wait()

	// Step 5: optional prune.
	if opts.Prune {
		for key, remoteObj := range remote {
			if _, ok := localObjs[key]; ok {
				continue
			}
			if err := s.client.RemoveObject(ctx, s.bucket, key); err != nil {
				result.Failures = append(result.Failures, R2FileFailure{
					Key:    key,
					Stage:  "prune",
					Error:  err.Error(),
				})
				continue
			}
			result.Pruned++
			_ = remoteObj
		}
	}

	s.logger.Log("r2-sync", deploy.EventFunctionEnd, map[string]any{
		"attempted":   result.Attempted,
		"succeeded":   result.Succeeded,
		"skipped":     result.Skipped,
		"failed":      result.Failed,
		"pruned":      result.Pruned,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return result, nil
}

// localEntry is one local file's metadata used for sync.
type localEntry struct {
	Key       string
	LocalPath string
	Size      int64
	MD5       string
	ContentType string
}

// walkLocal walks each sync.from dir, returning {key → localEntry} and a list
// of walk errors. Each entry's key is computed as `to + relative-path`.
func (s *R2Syncer) walkLocal(ctx context.Context, mappings []SyncMapping, result *R2SyncResult) (map[string]localEntry, []error) {
	out := make(map[string]localEntry)
	var errs []error

	for _, m := range mappings {
		info, err := os.Stat(m.From)
		if err != nil {
			errs = append(errs, fmt.Errorf("stat %q: %w", m.From, err))
			result.Failures = append(result.Failures, R2FileFailure{
				LocalPath: m.From,
				Stage:     "walk",
				Error:     err.Error(),
			})
			continue
		}
		if !info.IsDir() {
			// Single-file mapping: key = to + basename.
			if err := s.addFile(m.From, m.To, "", out); err != nil {
				errs = append(errs, err)
			}
			continue
		}

		walkErr := filepath.WalkDir(m.From, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || d.Type() == os.ModeSymlink {
				return nil
			}
			rel, err := filepath.Rel(m.From, path)
			if err != nil {
				return err
			}
			return s.addFile(path, m.To, rel, out)
		})
		if walkErr != nil {
			errs = append(errs, walkErr)
		}
	}
	return out, errs
}

// addFile reads one local file, computes MD5 + content-type, and adds to map.
// to is the destination prefix; rel is the path relative to walk root.
// For single-file mappings, rel is empty and basename(to) is used as key.
func (s *R2Syncer) addFile(path, to, rel string, out map[string]localEntry) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	md5hex := MD5Hex(content)

	var key string
	if rel == "" {
		// Single-file mapping: use basename of "to" as key.
		key = strings.TrimPrefix(to, "/")
	} else {
		// Compose: to + "/" + rel (with forward slashes).
		to = strings.TrimSuffix(to, "/")
		key = to + "/" + filepath.ToSlash(rel)
	}

	out[key] = localEntry{
		Key:         key,
		LocalPath:   path,
		Size:        int64(len(content)),
		MD5:         md5hex,
		ContentType: guessContentType(path),
	}
	return nil
}

// MD5Hex returns the hex-encoded MD5 of content. Used for R2 etag comparison
// (R2/S3 single-part uploads use MD5 as the etag). Exported for tests.
func MD5Hex(content []byte) string {
	sum := md5.Sum(content)
	return hex.EncodeToString(sum[:])
}

// listAll merges ListObjects results for each unique "to" prefix in mappings.
// Returns {key → R2Object}.
func (s *R2Syncer) listAll(ctx context.Context, mappings []SyncMapping) (map[string]R2Object, error) {
	// Deduplicate prefixes.
	prefixes := make(map[string]bool)
	for _, m := range mappings {
		prefix := strings.TrimSuffix(m.To, "/")
		prefixes[prefix] = true
	}

	out := make(map[string]R2Object)
	for prefix := range prefixes {
		objs, err := s.client.ListObjects(ctx, s.bucket, prefix)
		if err != nil {
			return nil, fmt.Errorf("list prefix %q: %w", prefix, err)
		}
		for _, o := range objs {
			out[o.Key] = o
		}
	}
	return out, nil
}

// uploadOne opens the local file and PUTs it to R2.
func (s *R2Syncer) uploadOne(ctx context.Context, le localEntry) error {
	f, err := os.Open(le.LocalPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	return s.client.PutObject(ctx, s.bucket, le.Key, le.ContentType, f, le.Size)
}

// sortedKeys returns remote keys in lexicographic order. Helper for tests.
func sortedObjectKeys(m map[string]R2Object) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// minioObjectClient adapts *minio.Client to the r2ObjectClient interface.
type minioObjectClient struct {
	c *minio.Client
}

func (m *minioObjectClient) BucketExists(ctx context.Context, bucket string) (bool, error) {
	exists, err := m.c.BucketExists(ctx, bucket)
	if err != nil {
		return false, errors.New("minio bucket exists: " + err.Error())
	}
	return exists, nil
}

func (m *minioObjectClient) ListObjects(ctx context.Context, bucket, prefix string) ([]R2Object, error) {
	var out []R2Object
	objCh := m.c.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for obj := range objCh {
		if obj.Err != nil {
			return nil, fmt.Errorf("list %q: %w", prefix, obj.Err)
		}
		out = append(out, R2Object{
			Key:  obj.Key,
			Size: obj.Size,
			ETag: stripETagQuotes(obj.ETag),
		})
	}
	return out, nil
}

func (m *minioObjectClient) PutObject(ctx context.Context, bucket, key, contentType string, r io.Reader, size int64) error {
	_, err := m.c.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return errors.New("minio put: " + err.Error())
	}
	return nil
}

func (m *minioObjectClient) RemoveObject(ctx context.Context, bucket, key string) error {
	return m.c.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

// stripETagQuotes removes surrounding quotes from S3 ETags.
// S3 returns etags like "abc123"; minio returns them without quotes; either way
// we want to compare the bare hex.
func stripETagQuotes(s string) string {
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")
	return s
}
