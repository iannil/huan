package admin

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/internal/config"
)

//go:embed dist/*
var adminDist embed.FS

// NewHandler creates an http.Handler for the admin panel (SPA + API).
// It serves the embedded React app on /admin/ and provides /admin/api/* JSON endpoints.
func NewHandler(cfg *config.Config, sourceDir string, rebuild func(), serveURL string) http.Handler {
	mux := http.NewServeMux()

	// API routes
	contentDir := filepath.Join(sourceDir, "content")
	staticDir := filepath.Join(sourceDir, "static")
	api := newAPIHandler(contentDir, staticDir, sourceDir, rebuild, cfg.Title, cfg.BaseURL, serveURL)
	mux.Handle("/admin/api/", api)

	// Static SPA files from embedded dist/
	subFS, err := fs.Sub(adminDist, "dist")
	if err != nil {
		mux.Handle("/admin/", servePlaceholder())
		return mux
	}
	staticFS := http.FileServer(http.FS(subFS))
	strippedFS := http.StripPrefix("/admin/", staticFS)

	mux.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/admin/")

		// SPA fallback: paths without a file extension → serve index.html
		if !hasFileExtension(cleanPath) || cleanPath == "" {
			serveIndex(subFS, w, r)
			return
		}

		// Static files (js, css, etc.) — strip prefix and serve from embedded FS
		strippedFS.ServeHTTP(w, r)
	})

	return mux
}

func serveIndex(subFS fs.FS, w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(subFS, "index.html")
	if err != nil {
		http.Error(w, "admin UI not built (run: cd web/admin && npm run build)", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func hasFileExtension(p string) bool {
	ext := path.Ext(p)
	return ext != "" && ext != "/"
}

func servePlaceholder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>huan admin</title></head><body>
<h1>huan admin</h1>
<p>Admin UI not built. Run:</p>
<pre><code>cd web/admin && npm install && npm run build
cd .. && go build -o huan ./cmd/huan</code></pre>
</body></html>`))
	}
}
