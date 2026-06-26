package admin

import (
	"encoding/json"
	"net/http"
	"strings"
)

// apiHandler holds the contentOps and registers API routes.
type apiHandler struct {
	ops       *contentOps
	media     *mediaOps
	rebuild   func()
	sourceDir string
	siteTitle string
	baseURL   string
	serveURL  string
	staticDir string
}

func newAPIHandler(contentDir, staticDir, sourceDir string, rebuild func(), siteTitle, baseURL, serveURL string) *apiHandler {
	return &apiHandler{
		ops:       newContentOps(contentDir),
		media:     newMediaOps(staticDir),
		sourceDir: sourceDir,
		rebuild:   rebuild,
		siteTitle: siteTitle,
		baseURL:   baseURL,
		serveURL:  serveURL,
		staticDir: staticDir,
	}
}

func (h *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Normalize path: /admin/api/content/...
	path := strings.TrimPrefix(r.URL.Path, "/admin/api")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "status" && r.Method == http.MethodGet:
		h.getStatus(w, r)
	case path == "content" && r.Method == http.MethodGet:
		h.listContent(w, r)
	case path == "content" && r.Method == http.MethodPost:
		h.createContent(w, r)
	case strings.HasPrefix(path, "content/") && r.Method == http.MethodGet:
		h.readContent(w, r, strings.TrimPrefix(path, "content/"))
	case strings.HasPrefix(path, "content/") && r.Method == http.MethodPut:
		h.updateContent(w, r, strings.TrimPrefix(path, "content/"))
	case strings.HasPrefix(path, "content/") && r.Method == http.MethodDelete:
		h.deleteContent(w, r, strings.TrimPrefix(path, "content/"))
	case path == "build" && r.Method == http.MethodPost:
		h.triggerBuild(w, r)
	case path == "media" && r.Method == http.MethodGet:
		h.handleMediaList(w, r)
	case path == "media" && r.Method == http.MethodPost:
		h.handleMediaUpload(w, r)
	case strings.HasPrefix(path, "media/") && r.Method == http.MethodDelete:
		h.handleMediaDelete(w, r, strings.TrimPrefix(path, "media/"))
	case path == "settings" && r.Method == http.MethodGet:
		h.getSettings(w, r)
	case path == "settings" && r.Method == http.MethodPut:
		h.updateSettings(w, r)
	case path == "settings/yaml" && r.Method == http.MethodGet:
		h.getSettingsYaml(w, r)
	case path == "settings/yaml" && r.Method == http.MethodPut:
		h.updateSettingsYaml(w, r)
	default:
		writeJSON(w, http.StatusNotFound, APIError{Error: "not found"})
	}
}

func (h *apiHandler) listContent(w http.ResponseWriter, r *http.Request) {
	sections, err := h.ops.listAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	total := 0
	var allItems []ContentItem
	for _, items := range sections {
		total += len(items)
		allItems = append(allItems, items...)
	}
	tree := h.ops.buildTree(allItems)
	writeJSON(w, http.StatusOK, ContentListResponse{Sections: sections, Tree: tree, Total: total})
}

func (h *apiHandler) readContent(w http.ResponseWriter, r *http.Request, relPath string) {
	detail, err := h.ops.readOne(relPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, APIError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *apiHandler) createContent(w http.ResponseWriter, r *http.Request) {
	var req CreateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Title == "" || req.Filename == "" {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "title and filename are required"})
		return
	}

	detail, err := h.ops.create(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (h *apiHandler) updateContent(w http.ResponseWriter, r *http.Request, relPath string) {
	var req UpdateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
		return
	}

	detail, err := h.ops.update(relPath, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *apiHandler) deleteContent(w http.ResponseWriter, r *http.Request, relPath string) {
	if err := h.ops.remove(relPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *apiHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	sections, err := h.ops.listAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	total := 0
	drafts := 0
	langSet := make(map[string]bool)
	breakdown := make(map[string]int)
	for sec, items := range sections {
		n := len(items)
		breakdown[sec] = n
		total += n
		for _, item := range items {
			if item.Draft {
				drafts++
			}
			if item.Language != "" {
				langSet[item.Language] = true
			}
		}
	}
	langs := make([]string, 0, len(langSet))
	for l := range langSet {
		langs = append(langs, l)
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		Title:            h.siteTitle,
		BaseURL:          h.baseURL,
		ServeURL:         h.serveURL,
		Total:            total,
		Drafts:           drafts,
		Sections:         len(sections),
		Languages:        langs,
		SectionBreakdown: breakdown,
	})
}

func (h *apiHandler) triggerBuild(w http.ResponseWriter, r *http.Request) {
	if h.rebuild == nil {
		writeJSON(w, http.StatusServiceUnavailable, APIError{Error: "rebuild not available"})
		return
	}
	go h.rebuild()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "rebuild triggered"})
}

func (h *apiHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	s, err := readSettings(h.sourceDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *apiHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var s SiteSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
		return
	}
	if err := updateSettings(h.sourceDir, &s); err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	// Trigger rebuild after saving settings
	if h.rebuild != nil {
		go h.rebuild()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (h *apiHandler) getSettingsYaml(w http.ResponseWriter, r *http.Request) {
	content, err := readSettingsYaml(h.sourceDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

func (h *apiHandler) updateSettingsYaml(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
		return
	}
	if err := updateSettingsYaml(h.sourceDir, body.Content); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	// Trigger rebuild after saving YAML
	if h.rebuild != nil {
		go h.rebuild()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
