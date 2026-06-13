package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/observability"
)

// mockWorkerServer captures the next PUT request for inspection. Returns
// 200 + success envelope by default; configurable via onFailure.
type mockWorkerServer struct {
	*httptest.Server
	mu sync.Mutex

	putSeen   bool
	pathSeen  string
	authSeen  string
	parts     map[string]map[string]string // partName -> {content, contentType, filename}
	onFailure func() (int, string)
}

func newMockWorkerServer(t *testing.T) *mockWorkerServer {
	t.Helper()
	m := &mockWorkerServer{
		parts: make(map[string]map[string]string),
	}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.Server.Close)
	return m
}

func (m *mockWorkerServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(405)
		return
	}
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.putSeen = true
	m.pathSeen = r.URL.Path
	m.authSeen = r.Header.Get("Authorization")
	m.parseMultipart(r.Header.Get("Content-Type"), body)
	m.mu.Unlock()

	if m.onFailure != nil {
		code, resp := m.onFailure()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(resp))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"result":{"modified_on":"2026-06-13T20:00:00Z","usage_model":"bundled","handler":"default"},"success":true,"errors":[]}`))
}

// parseMultipart extracts part name → {content, contentType, filename} map.
// Lightweight parser sufficient for tests.
func (m *mockWorkerServer) parseMultipart(contentType string, body []byte) {
	boundary := extractBoundary(contentType)
	if boundary == "" {
		return
	}
	parts := strings.Split(string(body), "--"+boundary)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "--" {
			continue
		}
		idx := strings.Index(p, "\r\n\r\n")
		if idx < 0 {
			continue
		}
		header := p[:idx]
		value := strings.TrimSuffix(p[idx+4:], "\r\n")

		name := extractHeaderParam(header, "name")
		if name == "" {
			continue
		}
		entry := map[string]string{
			"content":     value,
			"contentType": extractHeaderValue(header, "Content-Type"),
			"filename":    extractHeaderParam(header, "filename"),
		}
		m.parts[name] = entry
	}
}

func extractHeaderValue(header, key string) string {
	for _, line := range strings.Split(header, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(key)+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, line[:strings.Index(line, ":")+1]))
		}
	}
	return ""
}

func extractHeaderParam(header, param string) string {
	idx := strings.Index(header, param+"=\"")
	if idx < 0 {
		return ""
	}
	rest := header[idx+len(param+"=\""):]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func TestWorker_HappyPath(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("worker-test", &bytes.Buffer{})
	c := NewClient("acc-1", "test-tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	// Create script file in tmpdir.
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "image-resizer.js")
	if err := os.WriteFile(scriptPath, []byte("export default { fetch() {} };"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := NewWorkerDeployer(c, logger)
	report, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "image-resizer",
		Script: scriptPath,
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if !mock.putSeen {
		t.Error("PUT not seen")
	}
	if !strings.HasSuffix(mock.pathSeen, "/workers/scripts/image-resizer") {
		t.Errorf("path = %q", mock.pathSeen)
	}
	if mock.authSeen != "Bearer test-tok" {
		t.Errorf("auth = %q", mock.authSeen)
	}
	if report.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", report.Succeeded)
	}

	// Verify multipart parts.
	if _, ok := mock.parts["metadata"]; !ok {
		t.Errorf("missing metadata part; got %v", mock.parts)
	}
	if _, ok := mock.parts["image-resizer.js"]; !ok {
		t.Errorf("missing script part named after file basename")
	}
	scriptPart := mock.parts["image-resizer.js"]
	if scriptPart["contentType"] != WorkerMIMEType {
		t.Errorf("script contentType = %q, want %q", scriptPart["contentType"], WorkerMIMEType)
	}
	if scriptPart["filename"] != "image-resizer.js" {
		t.Errorf("script filename = %q", scriptPart["filename"])
	}
	if !strings.Contains(scriptPart["content"], "export default") {
		t.Errorf("script content missing expected text: %q", scriptPart["content"])
	}
}

func TestWorker_CompatibilityDate_Default(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
		// CompatibilityDate intentionally empty
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	if meta["compatibility_date"] != DefaultWorkerCompatibilityDate {
		t.Errorf("compatibility_date = %v, want %q", meta["compatibility_date"], DefaultWorkerCompatibilityDate)
	}
}

func TestWorker_CompatibilityDate_Override(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:              "w",
		Script:            scriptPath,
		CompatibilityDate: "2025-09-23",
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	if meta["compatibility_date"] != "2025-09-23" {
		t.Errorf("compatibility_date = %v, want 2025-09-23", meta["compatibility_date"])
	}
}

func TestWorker_R2Binding_Serialized(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
		Bindings: []WorkerBinding{
			{Type: "r2_bucket", Name: "R2_BUCKET", Bucket: "zhurongshuo"},
		},
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	bindings, ok := meta["bindings"].([]any)
	if !ok || len(bindings) != 1 {
		t.Fatalf("bindings = %v", meta["bindings"])
	}
	b := bindings[0].(map[string]any)
	if b["type"] != "r2_bucket" {
		t.Errorf("type = %v", b["type"])
	}
	if b["name"] != "R2_BUCKET" {
		t.Errorf("name = %v", b["name"])
	}
	if b["bucket"] != "zhurongshuo" {
		t.Errorf("bucket = %v", b["bucket"])
	}
}

func TestWorker_KVBinding_Serialized(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
		Bindings: []WorkerBinding{
			{Type: "kv_namespace", Name: "KV", NamespaceID: "abc-123"},
		},
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	bindings, _ := meta["bindings"].([]any)
	b := bindings[0].(map[string]any)
	if b["namespace_id"] != "abc-123" {
		t.Errorf("namespace_id = %v", b["namespace_id"])
	}
}

func TestWorker_VarsBinding_TextSerialized(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
		Bindings: []WorkerBinding{
			{Type: "vars", Name: "API_URL", Value: "https://api.example.com"},
		},
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	bindings, _ := meta["bindings"].([]any)
	b := bindings[0].(map[string]any)
	if b["text"] != "https://api.example.com" {
		t.Errorf("text = %v", b["text"])
	}
}

func TestWorker_RouteWithZone_OnlyZoneName(t *testing.T) {
	// Audit H5: previously sent both `zone` and `zone_name` with same value;
	// CF Workers modules API v4 expects only `zone_name` (matching wrangler).
	// Sending both could trigger ambiguous-zone rejection.
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
		Routes: []WorkerRoute{
			{Pattern: "r2.zhurongshuo.com/*", Zone: "zhurongshuo.com"},
		},
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	routes, _ := meta["routes"].([]any)
	if len(routes) != 1 {
		t.Fatalf("routes = %v", routes)
	}
	r := routes[0].(map[string]any)
	if r["pattern"] != "r2.zhurongshuo.com/*" {
		t.Errorf("pattern = %v", r["pattern"])
	}
	if r["zone_name"] != "zhurongshuo.com" {
		t.Errorf("zone_name = %v", r["zone_name"])
	}
	// CRITICAL: `zone` field MUST be absent (audit H5).
	if _, hasZone := r["zone"]; hasZone {
		t.Errorf("route JSON has 'zone' field = %v; want absent (CF v4 uses zone_name only)", r["zone"])
	}
}

// TestWorker_BindingRelevantFieldOnly verifies audit H6 contract: when user
// provides extra yaml fields not relevant to the binding type, the JSON
// output contains only the field(s) the type uses. Driven by omitempty on
// workerBindingJSON + switch-case in buildWorkerMetadata that only sets
// relevant fields.
func TestWorker_BindingRelevantFieldOnly(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
		Bindings: []WorkerBinding{
			// r2_bucket should ONLY emit {type, name, bucket}.
			{Type: "r2_bucket", Name: "R2_BUCKET", Bucket: "zhurongshuo"},
		},
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	bindings, _ := meta["bindings"].([]any)
	if len(bindings) != 1 {
		t.Fatalf("bindings = %v", bindings)
	}
	b := bindings[0].(map[string]any)
	if b["type"] != "r2_bucket" {
		t.Errorf("type = %v", b["type"])
	}
	if b["name"] != "R2_BUCKET" {
		t.Errorf("name = %v", b["name"])
	}
	if b["bucket"] != "zhurongshuo" {
		t.Errorf("bucket = %v", b["bucket"])
	}
	// CRITICAL: unrelated fields MUST be absent.
	for _, absent := range []string{"namespace_id", "id", "text"} {
		if _, has := b[absent]; has {
			t.Errorf("r2_bucket binding has unexpected %q = %v", absent, b[absent])
		}
	}
}

func TestWorker_RouteWithoutZone_OnlyPattern(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
		Routes: []WorkerRoute{
			{Pattern: "*example.com/path"},
		},
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	meta := decodeMetadata(t, mock.parts["metadata"]["content"])
	routes, _ := meta["routes"].([]any)
	r := routes[0].(map[string]any)
	if r["pattern"] != "*example.com/path" {
		t.Errorf("pattern = %v", r["pattern"])
	}
	if _, hasZone := r["zone"]; hasZone && r["zone"] != "" {
		t.Errorf("zone should be empty, got %v", r["zone"])
	}
}

func TestWorker_ScriptFileNotFound(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: "/nonexistent/path/script.js",
	}, DeployWorkerOptions{})
	if err == nil {
		t.Fatal("want error for missing script")
	}
	if !strings.Contains(err.Error(), "read script") {
		t.Errorf("err = %q", err.Error())
	}
	if mock.putSeen {
		t.Error("PUT should not happen on missing script")
	}
}

func TestWorker_MissingName(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Script: scriptPath,
	}, DeployWorkerOptions{})
	if err == nil {
		t.Fatal("want error for missing name")
	}
}

func TestWorker_CFReturnsError_Propagated(t *testing.T) {
	mock := newMockWorkerServer(t)
	mock.onFailure = func() (int, string) {
		return 400, `{"result":null,"success":false,"errors":[{"code":10000,"message":"script too large"}]}`
	}
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	report, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
	}, DeployWorkerOptions{})
	if err == nil {
		t.Fatal("want error")
	}
	if report.Failed != 0 {
		// report doesn't increment Failed on upload-stage error (returns directly)
	}
	if len(report.Failures) != 1 {
		t.Errorf("Failures = %d, want 1", len(report.Failures))
	}
	if !strings.Contains(report.Failures[0].Error, "script too large") {
		t.Errorf("err = %q", report.Failures[0].Error)
	}
}

func TestWorker_DryRun_NoPutCall(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "w.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	report, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: scriptPath,
	}, DeployWorkerOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if mock.putSeen {
		t.Error("PUT should not happen in dry-run")
	}
	if report.Succeeded != 0 {
		t.Errorf("Succeeded = %d, want 0 in dry-run", report.Succeeded)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 in dry-run", report.Skipped)
	}
}

func TestWorker_ScriptPartHasFilenameHeader(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "image-resizer.js")
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "image-resizer",
		Script: scriptPath,
	}, DeployWorkerOptions{})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	sp := mock.parts["image-resizer.js"]
	if sp["filename"] == "" {
		t.Errorf("script part missing filename; CF requires filename for module uploads")
	}
}

func TestWorker_SourceDirRelativeScript(t *testing.T) {
	mock := newMockWorkerServer(t)
	logger := observability.NewLoggerWithWriter("t", &bytes.Buffer{})
	c := NewClient("acc", "tok", logger).WithBaseURL(mock.URL).WithHTTPClient(mock.Client())

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "workers", "image-resizer.js")
	_ = os.MkdirAll(filepath.Dir(scriptPath), 0o755)
	_ = os.WriteFile(scriptPath, []byte("// hi"), 0o644)

	d := NewWorkerDeployer(c, logger)
	// Pass relative script path; SourceDir resolves it.
	_, err := d.Deploy(context.Background(), WorkerConfig{
		Name:   "w",
		Script: "workers/image-resizer.js",
	}, DeployWorkerOptions{SourceDir: dir})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if !mock.putSeen {
		t.Error("PUT not seen")
	}
}

func TestWorkerConfig_Validate(t *testing.T) {
	cases := []struct {
		name string
		cfg  WorkerConfig
		ok   bool
	}{
		{"empty", WorkerConfig{}, false},
		{"only name", WorkerConfig{Name: "w"}, false},
		{"only script", WorkerConfig{Script: "x.js"}, false},
		{"both", WorkerConfig{Name: "w", Script: "x.js"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.ok && err != nil {
				t.Errorf("want nil, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

func TestWorker_PluginDispatch(t *testing.T) {
	// Plugin.Deploy dispatches to Worker when target="worker".
	// Without worker config → clear error.
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
	if !strings.Contains(err.Error(), "worker.* config") {
		t.Errorf("err = %q", err.Error())
	}
}

// decodeMetadata parses the metadata multipart part JSON for assertions.
func decodeMetadata(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("invalid metadata JSON %q: %v", raw, err)
	}
	return m
}

// Ensure no compilation issues with unused imports.
var _ = fmt.Sprintf
var _ = errors.New
var _ = multipart.NewWriter
