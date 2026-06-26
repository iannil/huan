package admin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// mediaItem represents a single media file in the API response.
type mediaItem struct {
	Name string `json:"name"`
	Path string `json:"path"` // relative to static/
	Size int64  `json:"size"`
	Ext  string `json:"ext"`
}

// mediaListResponse wraps the media listing.
type mediaListResponse struct {
	Files  []mediaItem `json:"files"`
	Groups map[string][]mediaItem `json:"groups"`
	Total  int         `json:"total"`
}

// mediaOps handles file-system operations on the static/ directory.
type mediaOps struct {
	staticDir string
}

func newMediaOps(staticDir string) *mediaOps {
	return &mediaOps{staticDir: staticDir}
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".avif": true, ".svg": true, ".bmp": true, ".ico": true,
}

var mediaExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".avif": true, ".svg": true, ".bmp": true, ".ico": true,
	".mp4": true, ".webm": true, ".ogg": true,
	".mp3": true, ".wav": true, ".flac": true,
	".pdf": true, ".zip": true, ".gz": true,
}

func (m *mediaOps) listAll() (*mediaListResponse, error) {
	groups := make(map[string][]mediaItem)
	var all []mediaItem

	err := filepath.WalkDir(m.staticDir, func(fpath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(fpath))
		if !mediaExts[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(m.staticDir, fpath)
		item := mediaItem{
			Name: d.Name(),
			Path: rel,
			Size: info.Size(),
			Ext:  ext,
		}
		dir := filepath.Dir(rel)
		if dir == "." {
			dir = "/"
		}
		groups[dir] = append(groups[dir], item)
		all = append(all, item)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk static dir: %w", err)
	}

	return &mediaListResponse{
		Files:  all,
		Groups: groups,
		Total:  len(all),
	}, nil
}

// handleMediaList handles GET /admin/api/media
func (h *apiHandler) handleMediaList(w http.ResponseWriter, r *http.Request) {
	resp, err := h.media.listAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleMediaUpload handles POST /admin/api/media (multipart form)
func (h *apiHandler) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form, max 50MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid form: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "missing file: " + err.Error()})
		return
	}
	defer file.Close()

	dir := r.FormValue("dir") // optional subdirectory under static/
	if dir == "" {
		dir = "."
	}

	// Validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !mediaExts[ext] {
		writeJSON(w, http.StatusBadRequest, APIError{Error: "unsupported file type: " + ext})
		return
	}

	targetDir := filepath.Join(h.staticDir, dir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: "create dir: " + err.Error()})
		return
	}

	targetPath := filepath.Join(targetDir, header.Filename)
	dst, err := os.Create(targetPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: "create file: " + err.Error()})
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(targetPath)
		writeJSON(w, http.StatusInternalServerError, APIError{Error: "write file: " + err.Error()})
		return
	}

	rel, _ := filepath.Rel(h.staticDir, targetPath)
	writeJSON(w, http.StatusCreated, mediaItem{
		Name: header.Filename,
		Path: rel,
		Size: written,
		Ext:  ext,
	})
}

// handleMediaDelete handles DELETE /admin/api/media/{path}
func (h *apiHandler) handleMediaDelete(w http.ResponseWriter, r *http.Request, relPath string) {
	fullPath := filepath.Join(h.staticDir, relPath)

	// Security: ensure the resolved path is within staticDir
	absStatic, _ := filepath.Abs(h.staticDir)
	absTarget, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absTarget, absStatic) {
		writeJSON(w, http.StatusForbidden, APIError{Error: "path traversal denied"})
		return
	}

	if err := os.Remove(fullPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, APIError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

