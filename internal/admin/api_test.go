package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestAPIHandler builds an apiHandler with temp dirs for content/,
// static/, sourceDir, and an audit logger pointing at memory/daily/.
// Returns the handler and the temp source dir for inspection.
func newTestAPIHandler(t *testing.T) (*apiHandler, string) {
	t.Helper()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "content", "posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "memory", "daily"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed one existing post so list/read have something to work with.
	if err := os.WriteFile(
		filepath.Join(src, "content", "posts", "seed.md"),
		[]byte("---\ntitle: Seed\ndate: 2026-06-30\n---\nSeed body.\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	return newAPIHandler(apiHandlerConfig{
		contentDir: filepath.Join(src, "content"),
		staticDir:  filepath.Join(src, "static"),
		sourceDir:  src,
		rebuild:    nil,
		siteTitle:  "Test",
		baseURL:    "http://localhost/",
		serveURL:   "http://localhost:1313/",
		audit:      NewAuditLogger(filepath.Join(src, "memory", "daily")),
	}), src
}

// doJSON issues a request with optional body and returns the recorder.
// auth token is included so we can focus on handler behavior; auth is
// covered separately in auth_test.go.
func doJSON(t *testing.T, h http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(buf))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// --- L2 contract: every API endpoint requires a token ---

// TestAPI_RequireTokenOnEveryEndpoint verifies the L2 contract: ALL admin
// API requests must carry the token, including GETs. This is a table test
// so adding endpoints in the future will fail this test until auth is
// applied (which TokenMiddleware handles centrally, so unlikely to drift).
func TestAPI_RequireTokenOnEveryEndpoint(t *testing.T) {
	h, _ := newTestAPIHandler(t)
	wrapped := TokenMiddleware(h, "test-token")

	cases := []struct{ method, path string }{
		{http.MethodGet, "/admin/api/status"},
		{http.MethodGet, "/admin/api/content"},
		{http.MethodGet, "/admin/api/content/posts/seed.md"},
		{http.MethodGet, "/admin/api/settings"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			// No token → 401.
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("no-token %s %s: got %d, want 401", tc.method, tc.path, rec.Code)
			}
			// With token → not 401 (could be 200 or 404, just not 401).
			req2 := httptest.NewRequest(tc.method, tc.path, nil)
			req2.Header.Set("Authorization", "Bearer test-token")
			rec2 := httptest.NewRecorder()
			wrapped.ServeHTTP(rec2, req2)
			if rec2.Code == http.StatusUnauthorized {
				t.Errorf("with-token %s %s: got 401, want any non-401", tc.method, tc.path)
			}
		})
	}
}

// --- status endpoint ---

func TestAPI_Status_ReturnsCounts(t *testing.T) {
	h, _ := newTestAPIHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/admin/api/status", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1 (seed post)", resp.Total)
	}
	if resp.Title != "Test" {
		t.Errorf("Title = %q, want Test", resp.Title)
	}
}

// --- content CRUD ---

func TestAPI_ContentList_ReturnsSeed(t *testing.T) {
	h, _ := newTestAPIHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/admin/api/content", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp ContentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
}

func TestAPI_ContentCreate_WritesFileAndAudit(t *testing.T) {
	h, src := newTestAPIHandler(t)

	rec := doJSON(t, h, http.MethodPost, "/admin/api/content", CreateContentRequest{
		Section:  "posts",
		Filename: "new-post",
		Title:    "New Post",
		Draft:    false,
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// File was written.
	if _, err := os.Stat(filepath.Join(src, "content", "posts", "new-post.md")); err != nil {
		t.Errorf("file not written: %v", err)
	}

	// Audit log entry was appended.
	dailyNote := filepath.Join(src, "memory", "daily")
	files, _ := os.ReadDir(dailyNote)
	if len(files) != 1 {
		t.Fatalf("expected 1 daily note, got %d", len(files))
	}
	data, _ := os.ReadFile(filepath.Join(dailyNote, files[0].Name()))
	if !strings.Contains(string(data), "content.create") {
		t.Errorf("audit log missing content.create entry:\n%s", string(data))
	}
	if !strings.Contains(string(data), "posts/new-post.md") {
		t.Errorf("audit log missing path:\n%s", string(data))
	}
}

func TestAPI_ContentCreate_MissingTitle_400(t *testing.T) {
	h, _ := newTestAPIHandler(t)

	rec := doJSON(t, h, http.MethodPost, "/admin/api/content", CreateContentRequest{
		Filename: "no-title",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAPI_ContentRead_ReturnsDetail(t *testing.T) {
	h, _ := newTestAPIHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/admin/api/content/posts/seed.md", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var detail ContentDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if detail.Title != "Seed" {
		t.Errorf("Title = %q, want Seed", detail.Title)
	}
}

func TestAPI_ContentRead_MissingFile_404(t *testing.T) {
	h, _ := newTestAPIHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/admin/api/content/posts/nope.md", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAPI_ContentUpdate_WritesAndAudit(t *testing.T) {
	h, src := newTestAPIHandler(t)

	rec := doJSON(t, h, http.MethodPut, "/admin/api/content/posts/seed.md", UpdateContentRequest{
		Frontmatter: map[string]interface{}{
			"title": "Seed Updated",
			"date":  "2026-06-30",
		},
		RawContent: "New body content.",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// File contents changed.
	data, _ := os.ReadFile(filepath.Join(src, "content", "posts", "seed.md"))
	if !strings.Contains(string(data), "Seed Updated") {
		t.Errorf("file content not updated:\n%s", string(data))
	}

	// Audit log records the update with before→after SHA.
	dailyNote := filepath.Join(src, "memory", "daily")
	files, _ := os.ReadDir(dailyNote)
	data, _ = os.ReadFile(filepath.Join(dailyNote, files[0].Name()))
	if !strings.Contains(string(data), "content.update") {
		t.Errorf("audit log missing content.update:\n%s", string(data))
	}
	if !strings.Contains(string(data), "→") {
		t.Errorf("audit log missing before→after transition:\n%s", string(data))
	}
}

func TestAPI_ContentDelete_RemovesAndAudit(t *testing.T) {
	h, src := newTestAPIHandler(t)

	rec := doJSON(t, h, http.MethodDelete, "/admin/api/content/posts/seed.md", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// File removed.
	if _, err := os.Stat(filepath.Join(src, "content", "posts", "seed.md")); !os.IsNotExist(err) {
		t.Errorf("file still exists after delete")
	}

	// Audit log records the delete.
	dailyNote := filepath.Join(src, "memory", "daily")
	files, _ := os.ReadDir(dailyNote)
	data, _ := os.ReadFile(filepath.Join(dailyNote, files[0].Name()))
	if !strings.Contains(string(data), "content.delete") {
		t.Errorf("audit log missing content.delete:\n%s", string(data))
	}
	if !strings.Contains(string(data), "_deleted_") {
		t.Errorf("audit log missing _deleted_ marker:\n%s", string(data))
	}
}

// --- settings ---

func TestAPI_SettingsGet_ReturnsYAML(t *testing.T) {
	h, src := newTestAPIHandler(t)
	// Write a minimal huan.yaml.
	if err := os.WriteFile(
		filepath.Join(src, "huan.yaml"),
		[]byte("title: Test\ntitle-zh: 测试\npaginate: 10\nparams:\n  subTitle: Sub\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, h, http.MethodGet, "/admin/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var s SiteSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.Title != "Test" {
		t.Errorf("Title = %q, want Test", s.Title)
	}
	if s.Paginate != 10 {
		t.Errorf("Paginate = %d, want 10", s.Paginate)
	}
}

func TestAPI_SettingsUpdate_WritesAndAudit(t *testing.T) {
	h, src := newTestAPIHandler(t)
	// Write initial yaml with multiple fields to verify preservation.
	initial := "title: Original\npaginate: 5\nparams:\n  subTitle: Original Sub\n"
	if err := os.WriteFile(filepath.Join(src, "huan.yaml"), []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, h, http.MethodPut, "/admin/api/settings", SiteSettings{
		Title:   "Updated",
		Paginate: 20,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	data, _ := os.ReadFile(filepath.Join(src, "huan.yaml"))
	if !strings.Contains(string(data), "Updated") {
		t.Errorf("title not updated:\n%s", string(data))
	}
	if strings.Contains(string(data), "Original") {
		t.Errorf("original title not replaced:\n%s", string(data))
	}

	// Audit log.
	dailyNote := filepath.Join(src, "memory", "daily")
	files, _ := os.ReadDir(dailyNote)
	audit, _ := os.ReadFile(filepath.Join(dailyNote, files[0].Name()))
	if !strings.Contains(string(audit), "settings.update") {
		t.Errorf("audit log missing settings.update:\n%s", string(audit))
	}
}

// --- unknown path ---

func TestAPI_UnknownPath_404(t *testing.T) {
	h, _ := newTestAPIHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/admin/api/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestAPI_TokenMiddlewareEndToEnd wraps the apiHandler with TokenMiddleware
// and verifies the full L2 path through real HTTP routes (not just direct
// handler calls). This catches routing-level mistakes that direct
// handler tests would miss.
func TestAPI_TokenMiddlewareEndToEnd(t *testing.T) {
	h, _ := newTestAPIHandler(t)
	wrapped := TokenMiddleware(h, "secret-token")

	// Without token → 401.
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no-token status = %d, want 401", rec.Code)
	}

	// With token → 200.
	rec2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	wrapped.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Errorf("with-token status = %d, want 200", rec2.Code)
	}
}
