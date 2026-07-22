package contentindex

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
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

func TestHandler_QueryTooLong(t *testing.T) {
	// Resource-exhaustion guard: a `q` param longer than 200 chars must be
	// rejected with 400 Bad Request before any substring scan runs.
	h := NewHandler(loadTestIndex(t))
	longQ := "/api/v1/pages?q=" + strings.Repeat("a", 201)
	rec := serveHandler(t, h, "GET", longQ, "")
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if m["error"] != "query too long" {
		t.Errorf("error = %q, want \"query too long\"", m["error"])
	}
}

func TestHandler_QueryAtMaxLengthOK(t *testing.T) {
	// Boundary: 200 chars exactly is still allowed (200 is inclusive).
	h := NewHandler(loadTestIndex(t))
	okQ := "/api/v1/pages?q=" + strings.Repeat("a", 200)
	rec := serveHandler(t, h, "GET", okQ, "")
	if rec.Code != 200 {
		t.Errorf("code = %d, want 200 (boundary)", rec.Code)
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
