package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// apiHandlerConfig holds the constructor inputs for apiHandler. Split into
// its own struct so NewHandler can build it inline without 8-arg function
// signatures drifting as new dependencies are added.
type apiHandlerConfig struct {
	contentDir string
	staticDir  string
	sourceDir  string
	rebuild    func()
	siteTitle  string
	baseURL    string
	serveURL   string
	audit      *AuditLogger
}

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
	audit     *AuditLogger
}

func newAPIHandler(cfg apiHandlerConfig) *apiHandler {
	return &apiHandler{
		ops:       newContentOps(cfg.contentDir),
		media:     newMediaOps(cfg.staticDir),
		sourceDir: cfg.sourceDir,
		rebuild:   cfg.rebuild,
		siteTitle: cfg.siteTitle,
		baseURL:   cfg.baseURL,
		serveURL:  cfg.serveURL,
		staticDir: cfg.staticDir,
		audit:     cfg.audit,
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
		rest := strings.TrimPrefix(path, "content/")
		if strings.HasSuffix(rest, "/languages") {
			h.getContentLanguages(w, r, strings.TrimSuffix(rest, "/languages"))
		} else {
			h.readContent(w, r, rest)
		}
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

func (h *apiHandler) getContentLanguages(w http.ResponseWriter, r *http.Request, relPath string) {
	relPath = strings.TrimPrefix(relPath, "/")
	siblings, err := h.ops.siblings(relPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	current := detectLanguage(relPath)
	writeJSON(w, http.StatusOK, SiblingResponse{Current: current, Siblings: siblings})
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
	// L4 audit: create has no "before" SHA (file didn't exist).
	h.auditLog(AuditRecord{
		Action:   ActionContentCreate,
		Path:     detail.RelPath,
		AfterSHA: safeSHA(filepath.Join(h.ops.contentDir, detail.RelPath)),
	})
	writeJSON(w, http.StatusCreated, detail)
}

func (h *apiHandler) updateContent(w http.ResponseWriter, r *http.Request, relPath string) {
	var req UpdateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
		return
	}

	fullPath := filepath.Join(h.ops.contentDir, relPath)
	beforeSHA := safeSHA(fullPath)
	detail, err := h.ops.update(relPath, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	h.auditLog(AuditRecord{
		Action:    ActionContentUpdate,
		Path:      relPath,
		BeforeSHA: beforeSHA,
		AfterSHA:  safeSHA(fullPath),
	})
	writeJSON(w, http.StatusOK, detail)
}

func (h *apiHandler) deleteContent(w http.ResponseWriter, r *http.Request, relPath string) {
	fullPath := filepath.Join(h.ops.contentDir, relPath)
	beforeSHA := safeSHA(fullPath)
	if err := h.ops.remove(relPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	h.auditLog(AuditRecord{
		Action:    ActionContentDelete,
		Path:      relPath,
		BeforeSHA: beforeSHA,
	})
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
	published := 0
	langSet := make(map[string]bool)
	breakdown := make(map[string]int)
	var allItems []ContentItem
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
		allItems = append(allItems, items...)
	}
	published = total - drafts
	langs := make([]string, 0, len(langSet))
	for l := range langSet {
		langs = append(langs, l)
	}

	// Recent content: up to 5 items sorted by date (newest first, items without date last)
	sort.Slice(allItems, func(i, j int) bool {
		if allItems[i].Date == "" && allItems[j].Date == "" {
			return allItems[i].RelPath > allItems[j].RelPath
		}
		if allItems[i].Date == "" {
			return false
		}
		if allItems[j].Date == "" {
			return true
		}
		return allItems[i].Date > allItems[j].Date
	})
	recentLimit := 5
	if len(allItems) < recentLimit {
		recentLimit = len(allItems)
	}
	recentContent := make([]ContentItem, recentLimit)
	copy(recentContent, allItems[:recentLimit])

	// Media count
	mediaCount := 0
	if h.media != nil {
		if mediaResp, err := h.media.listAll(); err == nil {
			mediaCount = mediaResp.Total
		}
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		Title:            h.siteTitle,
		BaseURL:          h.baseURL,
		ServeURL:         h.serveURL,
		Total:            total,
		Published:        published,
		Drafts:           drafts,
		Sections:         len(sections),
		Languages:        langs,
		MediaCount:       mediaCount,
		SectionBreakdown: breakdown,
		RecentContent:    recentContent,
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
	yamlPath := filepath.Join(h.sourceDir, "huan.yaml")
	beforeSHA := safeSHA(yamlPath)
	if err := updateSettings(h.sourceDir, &s); err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	h.auditLog(AuditRecord{
		Action:    ActionSettingsUpdate,
		Path:      "huan.yaml",
		BeforeSHA: beforeSHA,
		AfterSHA:  safeSHA(yamlPath),
	})
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
	yamlPath := filepath.Join(h.sourceDir, "huan.yaml")
	beforeSHA := safeSHA(yamlPath)
	if err := updateSettingsYaml(h.sourceDir, body.Content); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: err.Error()})
		return
	}
	h.auditLog(AuditRecord{
		Action:    ActionSettingsYAML,
		Path:      "huan.yaml",
		BeforeSHA: beforeSHA,
		AfterSHA:  safeSHA(yamlPath),
	})
	// Trigger rebuild after saving YAML
	if h.rebuild != nil {
		go h.rebuild()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// auditLog is a nil-safe wrapper: if audit logger is unset (e.g., in tests
// with no memoryDir), the call is a no-op. Errors are logged via the
// server's logf but never propagate to the HTTP response — a failed audit
// log entry must not block the write operation.
func (h *apiHandler) auditLog(rec AuditRecord) {
	if h.audit == nil {
		return
	}
	if err := h.audit.Log(rec); err != nil {
		// Best-effort: print to stderr; the write op already succeeded.
		fmt.Fprintf(os.Stderr, "huan: admin audit log failed: %v\n", err)
	}
}

// safeSHA returns the hex SHA256 of filePath, or "" if the file cannot be
// read (including "not exists" — which is the expected case for create
// before-SHA and delete after-SHA).
func safeSHA(filePath string) string {
	sha, err := ComputeSHA256(filePath)
	if err != nil {
		return ""
	}
	return sha
}
