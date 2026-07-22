### Task 3: HTTP Handler

**Files:**
- Create: `internal/daemon/contentindex/handler.go` — /api/v1/* 路由
- Create: `internal/daemon/contentindex/handler_test.go` — Handler 测试

**Interfaces:**
- Consumes: ContentIndex (Task 1-2)
- Produces: `Handler`, `NewHandler(index *ContentIndex) *Handler`, `ServeHTTP`

- [ ] **Step 1: 编写测试**

`internal/daemon/contentindex/handler_test.go`：

```go
package contentindex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_PagesList(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages", "")
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var r Result
	json.Unmarshal(rec.Body.Bytes(), &r)
	if r.Total == 0 {
		t.Error("expected non-empty list")
	}
	if r.Limit != 10 {
		t.Errorf("default limit = %d, want 10", r.Limit)
	}
}

func TestHandler_PagesFilter(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages?section=books", "")
	var r Result
	json.Unmarshal(rec.Body.Bytes(), &r)
	for _, it := range r.Data {
		if it.Section != "books" {
			t.Errorf("filter leaked section %q", it.Section)
		}
	}
}

func TestHandler_PageDetail(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages/posts/go/", "")
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var it Item
	json.Unmarshal(rec.Body.Bytes(), &it)
	if it.URL != "/posts/go/" {
		t.Errorf("URL = %q", it.URL)
	}
}

func TestHandler_PageDetail404(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages/nope/", "")
	if rec.Code != 404 {
		t.Errorf("code = %d, want 404", rec.Code)
	}
}

func TestHandler_Tags(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/tags", "")
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var m map[string]int
	json.Unmarshal(rec.Body.Bytes(), &m)
	if m["go"] == 0 {
		t.Errorf("expected go tag, got %v", m)
	}
}

func TestHandler_Sections(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/sections", "")
	var m map[string]int
	json.Unmarshal(rec.Body.Bytes(), &m)
	if m["posts"] == 0 {
		t.Errorf("expected posts section, got %v", m)
	}
}

func TestHandler_NoAuthRequired(t *testing.T) {
	// Public endpoint: no Authorization header, no token → still 200.
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages", "")
	if rec.Code != 200 {
		t.Errorf("public endpoint returned %d", rec.Code)
	}
}

func TestHandler_IndexNotReady(t *testing.T) {
	// Empty index (Len 0) should still serve 200 with empty data, not 503.
	h := NewHandler(NewContentIndex(baseURL))
	rec := serveHandler(t, h, "GET", "/api/v1/pages", "")
	if rec.Code != 200 {
		t.Errorf("empty index code = %d, want 200", rec.Code)
	}
}

func serveHandler(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/contentindex/ -run "TestHandler" -v
```
Expected: COMPILATION ERROR (no Handler)

- [ ] **Step 3: 实现 handler.go**

`internal/daemon/contentindex/handler.go`：

```go
package contentindex

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Handler exposes the read-only content query API at /api/v1/*.
// No authentication — these endpoints are public.
type Handler struct {
	index *ContentIndex
}

// NewHandler creates a Handler backed by the given ContentIndex.
func NewHandler(index *ContentIndex) *Handler {
	return &Handler{index: index}
}

// ServeHTTP routes /api/v1/pages, /api/v1/pages/{url}, /api/v1/tags,
// /api/v1/sections.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody("method not allowed"))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "pages":
		h.handlePagesList(w, r)
	case strings.HasPrefix(path, "pages/"):
		h.handlePageDetail(w, strings.TrimPrefix(path, "pages/"))
	case path == "tags":
		h.handleTags(w, r)
	case path == "sections":
		h.handleSections(w, r)
	default:
		writeJSON(w, http.StatusNotFound, errBody("not found"))
	}
}

func (h *Handler) handlePagesList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{
		Section: q.Get("section"),
		Tag:     q.Get("tag"),
		Query:   q.Get("q"),
		Sort:    q.Get("sort"),
	}
	if v, err := strconv.Atoi(q.Get("page")); err == nil {
		f.Page = v
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = v
	}
	res := h.index.Query(f)
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handlePageDetail(w http.ResponseWriter, rest string) {
	// rest is like "posts/go/" → normalize to "/posts/go/"
	url := "/" + strings.TrimPrefix(rest, "/")
	item, ok := h.index.GetByURL(url)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleTags(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.index.Tags())
}

func (h *Handler) handleSections(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.index.Sections())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string {
	return map[string]string{"error": msg}
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/contentindex/ -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/contentindex/handler.go internal/daemon/contentindex/handler_test.go
git commit -m "feat(contentindex): add HTTP handler for /api/v1/* query endpoints"
```

---

