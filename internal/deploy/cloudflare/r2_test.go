package cloudflare

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/observability"
)

// mockR2Client is an in-memory r2ObjectClient for unit tests. It simulates a
// single bucket with a flat key→object map.
type mockR2Client struct {
	mu           sync.Mutex
	bucketExists bool
	objects      map[string]R2Object // key -> metadata
	contents     map[string][]byte   // key -> content (for read-back verification)

	// Hooks for fault injection.
	putShouldFailFor map[string]error // key -> error to return on PutObject
	listShouldErr    error
	bucketShouldErr  error

	// Counters for verification.
	putCount    int
	removeCount int
}

func newMockR2Client() *mockR2Client {
	return &mockR2Client{
		bucketExists:     true,
		objects:          make(map[string]R2Object),
		contents:         make(map[string][]byte),
		putShouldFailFor: make(map[string]error),
	}
}

func (m *mockR2Client) BucketExists(ctx context.Context, bucket string) (bool, error) {
	if m.bucketShouldErr != nil {
		return false, m.bucketShouldErr
	}
	return m.bucketExists, nil
}

func (m *mockR2Client) ListObjects(ctx context.Context, bucket, prefix string) ([]R2Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listShouldErr != nil {
		return nil, m.listShouldErr
	}
	var out []R2Object
	for _, obj := range m.objects {
		if prefix == "" || strings.HasPrefix(obj.Key, prefix) {
			out = append(out, obj)
		}
	}
	return out, nil
}

func (m *mockR2Client) PutObject(ctx context.Context, bucket, key, contentType string, r io.Reader, size int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.putCount++
	if err, ok := m.putShouldFailFor[key]; ok {
		return err
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	// Compute MD5 (real R2/S3 does this; tests should match local MD5 logic).
	sum := MD5Hex(body)
	m.objects[key] = R2Object{Key: key, Size: int64(len(body)), ETag: sum}
	m.contents[key] = body
	return nil
}

func (m *mockR2Client) RemoveObject(ctx context.Context, bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeCount++
	delete(m.objects, key)
	delete(m.contents, key)
	return nil
}

// seedRemoteObject adds a fake object to the mock's bucket.
func (m *mockR2Client) seedRemoteObject(key string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = R2Object{Key: key, Size: int64(len(content)), ETag: MD5Hex(content)}
	m.contents[key] = content
}

// writeLocalFixture writes files under dir; relPaths must be relative paths
// like "images/cat.jpg" or "sub/dir/x.txt".
func writeLocalFixture(t *testing.T, dir string, relPath string, content []byte) string {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return full
}

func newR2TestLogger() *observability.Logger {
	return observability.NewLoggerWithWriter("r2-test", &bytes.Buffer{})
}

func TestR2Sync_HappyPath_AllUploaded(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("cat-bytes"))
	writeLocalFixture(t, dir, "dog.jpg", []byte("dog-bytes"))

	syncer := NewR2SyncerWithClient(mock, "testbucket", newR2TestLogger())
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Attempted != 2 {
		t.Errorf("Attempted = %d, want 2", result.Attempted)
	}
	if result.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", result.Succeeded)
	}
	if result.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", result.Skipped)
	}
	if len(mock.objects) != 2 {
		t.Errorf("remote objects = %d, want 2", len(mock.objects))
	}
	// Verify keys have "images/" prefix.
	for k := range mock.objects {
		if !strings.HasPrefix(k, "images/") {
			t.Errorf("key %q missing images/ prefix", k)
		}
	}
}

func TestR2Sync_IncrementalSkip_MD5Match(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	content := []byte("cat-bytes")
	writeLocalFixture(t, dir, "cat.jpg", content)
	// Pre-seed remote with matching MD5.
	mock.seedRemoteObject("images/cat.jpg", content)

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Succeeded != 0 {
		t.Errorf("Succeeded = %d, want 0 (should skip)", result.Succeeded)
	}
	if mock.putCount != 0 {
		t.Errorf("putCount = %d, want 0", mock.putCount)
	}
}

func TestR2Sync_MD5Mismatch_UploadsReplacement(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("new-content"))
	// Pre-seed remote with stale content.
	mock.seedRemoteObject("images/cat.jpg", []byte("old-content"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1 (replace)", result.Succeeded)
	}
	if result.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", result.Skipped)
	}
	// Verify remote content was replaced.
	got := mock.contents["images/cat.jpg"]
	if string(got) != "new-content" {
		t.Errorf("remote content = %q, want 'new-content'", string(got))
	}
}

func TestR2Sync_PruneDeletesOrphans(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("cat"))
	// Remote has 2 objects: cat (matches local) + orphan (no local).
	mock.seedRemoteObject("images/cat.jpg", []byte("cat"))
	mock.seedRemoteObject("images/orphan.jpg", []byte("orphan"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{Prune: true},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Pruned != 1 {
		t.Errorf("Pruned = %d, want 1", result.Pruned)
	}
	if _, exists := mock.objects["images/orphan.jpg"]; exists {
		t.Errorf("orphan still in remote: %v", mock.objects)
	}
}

func TestR2Sync_PruneFalse_KeepsOrphans(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("cat"))
	mock.seedRemoteObject("images/cat.jpg", []byte("cat"))
	mock.seedRemoteObject("images/orphan.jpg", []byte("orphan"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{Prune: false},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, exists := mock.objects["images/orphan.jpg"]; !exists {
		t.Errorf("orphan should be preserved without --prune")
	}
	if mock.removeCount != 0 {
		t.Errorf("removeCount = %d, want 0", mock.removeCount)
	}
}

// TestR2Sync_PruneLogsEachDelete verifies audit H3 fix: each prune delete
// emits a structured r2-prune log event with key + size, so users have an
// audit trail of what got removed.
func TestR2Sync_PruneLogsEachDelete(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("cat"))
	mock.seedRemoteObject("images/cat.jpg", []byte("cat"))
	mock.seedRemoteObject("images/orphan1.jpg", []byte("orphan1-bytes"))
	mock.seedRemoteObject("images/orphan2.jpg", []byte("orphan2-bytes"))

	var buf bytes.Buffer
	logger := observability.NewLoggerWithWriter("prune-log-test", &buf)
	syncer := NewR2SyncerWithClient(mock, "b", logger)
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{Prune: true},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	logOutput := buf.String()
	for _, key := range []string{"images/orphan1.jpg", "images/orphan2.jpg"} {
		if !strings.Contains(logOutput, key) {
			t.Errorf("prune log missing key %q; log was:\n%s", key, logOutput)
		}
	}
	if !strings.Contains(logOutput, "r2-prune") {
		t.Errorf("log missing r2-prune event type; log was:\n%s", logOutput)
	}
	// Make sure the matched file (cat.jpg) is NOT in a prune log (it survived).
	catPruneEntries := strings.Count(logOutput, "images/orphan1.jpg") +
		strings.Count(logOutput, "images/orphan2.jpg")
	if catPruneEntries != 2 {
		t.Errorf("expected 2 distinct r2-prune entries, got %d", catPruneEntries)
	}
}

// TestR2Sync_PruneDoesNotTouchKeysOutsideConfiguredPrefix verifies audit
// H3 fact-finding: prune is scoped to configured `to:` prefixes only.
// Objects under other prefixes are invisible to listAll and never deleted.
func TestR2Sync_PruneDoesNotTouchKeysOutsideConfiguredPrefix(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("cat"))
	// Local only has cat.jpg mapped to images/gallery/cat.jpg.
	// Remote has unrelated images/banners/x.jpg from "another app".
	mock.seedRemoteObject("images/gallery/cat.jpg", []byte("cat"))
	mock.seedRemoteObject("images/banners/x.jpg", []byte("not-mine"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images/gallery"}},
		R2SyncOptions{Prune: true},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// images/banners/x.jpg is OUTSIDE the configured prefix "images/gallery"
	// and must NOT be deleted.
	if _, exists := mock.objects["images/banners/x.jpg"]; !exists {
		t.Errorf("images/banners/x.jpg was deleted but is outside configured prefix")
	}
}

func TestR2Sync_DryRun_NoMutations(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("cat"))
	mock.seedRemoteObject("images/orphan.jpg", []byte("orphan"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{DryRun: true, Prune: true},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if mock.putCount != 0 {
		t.Errorf("putCount = %d in dry-run, want 0", mock.putCount)
	}
	if mock.removeCount != 0 {
		t.Errorf("removeCount = %d in dry-run, want 0", mock.removeCount)
	}
	if result.Attempted != 1 {
		t.Errorf("Attempted = %d, want 1", result.Attempted)
	}
	// In dry-run, Succeeded is reset to 0 to signal "nothing actually sent".
	if result.Succeeded != 0 {
		t.Errorf("Succeeded = %d in dry-run, want 0", result.Succeeded)
	}
}

func TestR2Sync_BucketMissing_Error(t *testing.T) {
	mock := newMockR2Client()
	mock.bucketExists = false
	dir := t.TempDir()
	writeLocalFixture(t, dir, "cat.jpg", []byte("cat"))

	syncer := NewR2SyncerWithClient(mock, "missing", newR2TestLogger())
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{},
	)
	if err == nil {
		t.Fatal("want error for missing bucket")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("err = %q", err.Error())
	}
}

func TestR2Sync_MultiPathMapping(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "images/cat.jpg", []byte("cat"))
	writeLocalFixture(t, dir, "videos/dog.mp4", []byte("dog"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{
			{From: filepath.Join(dir, "images"), To: "img"},
			{From: filepath.Join(dir, "videos"), To: "vid"},
		},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", result.Succeeded)
	}
	if _, ok := mock.objects["img/cat.jpg"]; !ok {
		t.Errorf("missing img/cat.jpg")
	}
	if _, ok := mock.objects["vid/dog.mp4"]; !ok {
		t.Errorf("missing vid/dog.mp4")
	}
}

func TestR2Sync_CollectionNotInterruption(t *testing.T) {
	mock := newMockR2Client()
	// Inject failure for one specific key.
	mock.putShouldFailFor["images/bad.jpg"] = errors.New("simulated upload failure")

	dir := t.TempDir()
	writeLocalFixture(t, dir, "good1.jpg", []byte("good1"))
	writeLocalFixture(t, dir, "bad.jpg", []byte("bad"))
	writeLocalFixture(t, dir, "good2.jpg", []byte("good2"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "images"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync should not return top-level error for per-file failure: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if result.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", result.Succeeded)
	}
	if len(result.Failures) != 1 {
		t.Errorf("len(Failures) = %d, want 1", len(result.Failures))
	}
	if !strings.Contains(result.Failures[0].Error, "simulated upload failure") {
		t.Errorf("Failures[0].Error = %q", result.Failures[0].Error)
	}
}

func TestR2Sync_NestedDirectoriesPreserved(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	writeLocalFixture(t, dir, "a/b/c/deep.jpg", []byte("deep"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "img"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	wantKey := "img/a/b/c/deep.jpg"
	if _, ok := mock.objects[wantKey]; !ok {
		t.Errorf("missing %q; got %v", wantKey, sortedObjectKeys(mock.objects))
	}
}

func TestR2Sync_SingleFileMapping_UsesToAsKey(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	filePath := writeLocalFixture(t, dir, "lonely.txt", []byte("lone"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: filePath, To: "uploads/renamed.txt"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, ok := mock.objects["uploads/renamed.txt"]; !ok {
		t.Errorf("single-file mapping should use 'to' as key; got %v", sortedObjectKeys(mock.objects))
	}
}

// TestR2Sync_SingleFileMapping_TrailingSlashAppendsBasename verifies audit
// M7 fix: {from: "logo.png", to: "brand/"} should produce key "brand/logo.png"
// (treating trailing slash as directory intent), not "brand/" (literal).
func TestR2Sync_SingleFileMapping_TrailingSlashAppendsBasename(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	filePath := writeLocalFixture(t, dir, "logo.png", []byte("logo-bytes"))

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: filePath, To: "brand/"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	wantKey := "brand/logo.png"
	if _, ok := mock.objects[wantKey]; !ok {
		t.Errorf("missing key %q; got %v", wantKey, sortedObjectKeys(mock.objects))
	}
	// And make sure the buggy literal "brand/" is NOT present.
	if _, ok := mock.objects["brand/"]; ok {
		t.Errorf("found buggy literal key 'brand/' in remote; should be 'brand/logo.png'")
	}
}

func TestR2Sync_SymlinkSkipped(t *testing.T) {
	mock := newMockR2Client()
	dir := t.TempDir()
	real := writeLocalFixture(t, dir, "real.jpg", []byte("real"))
	if err := os.Symlink(real, filepath.Join(dir, "link.jpg")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	var buf bytes.Buffer
	logger := observability.NewLoggerWithWriter("symlink-test", &buf)
	syncer := NewR2SyncerWithClient(mock, "b", logger)
	result, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "img"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Attempted != 1 {
		t.Errorf("Attempted = %d, want 1 (symlink skipped)", result.Attempted)
	}
	if _, exists := mock.objects["img/link.jpg"]; exists {
		t.Errorf("symlink was uploaded")
	}
	// Audit L7: skipped symlinks should be logged for observability.
	logOutput := buf.String()
	if !strings.Contains(logOutput, "r2-walk-skip") {
		t.Errorf("log missing r2-walk-skip event:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, "link.jpg") {
		t.Errorf("log missing symlink path:\n%s", logOutput)
	}
}

func TestR2Sync_MD5MatchesContent(t *testing.T) {
	// Sanity-check that the MD5 used for incremental comparison is the actual
	// MD5 of the file content (S3/R2 etag for single-part uploads).
	mock := newMockR2Client()
	dir := t.TempDir()
	content := []byte("verify-md5-content")
	writeLocalFixture(t, dir, "f.jpg", content)

	syncer := NewR2SyncerWithClient(mock, "b", newR2TestLogger())
	_, err := syncer.Sync(context.Background(),
		[]SyncMapping{{From: dir, To: "x"}},
		R2SyncOptions{},
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	obj, ok := mock.objects["x/f.jpg"]
	if !ok {
		t.Fatalf("missing x/f.jpg")
	}
	if obj.ETag != MD5Hex(content) {
		t.Errorf("ETag = %q, want %q", obj.ETag, MD5Hex(content))
	}
}

func TestR2Config_Validate_AllRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  R2Config
		want string // substring expected in error message
	}{
		{"empty", R2Config{}, "r2.accountId"},
		{"missing access key", R2Config{AccountID: "a", Bucket: "b"}, "r2.accessKeyId"},
		{"missing secret", R2Config{AccountID: "a", AccessKeyID: "k", Bucket: "b"}, "r2.secretAccessKey"},
		{"missing bucket", R2Config{AccountID: "a", AccessKeyID: "k", SecretAccessKey: "s"}, "r2.bucket"},
		{"endpoint substitutes for accountId", R2Config{Endpoint: "http://test", AccessKeyID: "k", SecretAccessKey: "s", Bucket: "b"}, ""},
		{"complete", R2Config{AccountID: "a", AccessKeyID: "k", SecretAccessKey: "s", Bucket: "b"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.want == "" {
				if err != nil {
					t.Errorf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestR2Config_HasR2Configured(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"only accountId", Config{R2: R2Config{AccountID: "a"}}, true},
		{"only bucket", Config{R2: R2Config{Bucket: "b"}}, true},
		{"only sync", Config{R2: R2Config{Sync: []SyncMapping{{}}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.HasR2Configured(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestR2Sync_PluginDispatch_R2Target(t *testing.T) {
	// Plugin.Deploy dispatches to R2 when target="r2".
	// We don't mock the underlying client here (would need to thread the mock
	// through); instead, we test that dispatch rejects missing R2 config.
	p := New(Config{
		AccountID: "a",
		APIToken:  "t",
		Pages:     PagesConfig{Project: "p", Branch: "main"},
		// R2 intentionally empty
	})
	report, err := p.Deploy(context.Background(), deploy.Options{
		Targets: []string{"r2"},
	})
	if err == nil {
		t.Fatal("want error for r2 target without R2 config")
	}
	if !strings.Contains(err.Error(), "r2.* config") {
		t.Errorf("err = %q", err.Error())
	}
	// Audit L1: Report.TraceID must be non-empty even on early-return errors.
	if report == nil {
		t.Fatal("report is nil")
	}
	if report.TraceID == "" {
		t.Error("Report.TraceID empty on early-return; want non-empty for correlation")
	}
}

func TestR2Sync_PluginDispatch_UnknownTarget(t *testing.T) {
	p := New(Config{
		AccountID: "a", APIToken: "t",
		Pages: PagesConfig{Project: "p", Branch: "main"},
	})
	_, err := p.Deploy(context.Background(), deploy.Options{
		Targets: []string{"unknown-target"},
	})
	if err == nil {
		t.Fatal("want error for unknown target")
	}
}

func TestR2Sync_PluginDispatch_MultipleTargets(t *testing.T) {
	p := New(Config{
		AccountID: "a", APIToken: "t",
		Pages: PagesConfig{Project: "p", Branch: "main"},
	})
	_, err := p.Deploy(context.Background(), deploy.Options{
		Targets: []string{"pages", "r2"},
	})
	if err == nil {
		t.Fatal("want error for multiple targets")
	}
	if !strings.Contains(err.Error(), "multiple targets") {
		t.Errorf("err = %q", err.Error())
	}
}

func TestR2Sync_PluginDispatch_WorkerTarget(t *testing.T) {
	// After PR3, worker target IS supported. Without worker config → clear
	// "worker.* config required" error (not the old PR3-not-implemented error).
	p := New(Config{
		AccountID: "a", APIToken: "t",
		Pages: PagesConfig{Project: "p", Branch: "main"},
	})
	_, err := p.Deploy(context.Background(), deploy.Options{
		Targets: []string{"worker"},
	})
	if err == nil {
		t.Fatal("want error for worker without config")
	}
	if !strings.Contains(err.Error(), "worker.*") || !strings.Contains(err.Error(), "config") {
		t.Errorf("err = %q", err.Error())
	}
}
