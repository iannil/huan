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
