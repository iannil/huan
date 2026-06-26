package serve

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewAdminDevProxy creates an http.Handler that proxies non-API /admin/ requests
// to the Vite dev server during development. API requests (/admin/api/) are
// served directly by huan, preserving the full API behavior.
//
// Usage:
//
//	./huan serve --adminDev http://localhost:5173
//	cd web/admin && npm run dev   # separate terminal
func NewAdminDevProxy(devURL string, apiHandler http.Handler) http.Handler {
	target, err := url.Parse(devURL)
	if err != nil {
		panic(fmt.Sprintf("admin dev proxy: invalid URL %q: %v", devURL, err))
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "admin dev proxy: cannot reach %s\n\n", devURL)
		fmt.Fprintf(w, "Make sure the Vite dev server is running:\n")
		fmt.Fprintf(w, "\tcd web/admin && npm run dev\n")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes are served directly by huan
		if strings.HasPrefix(r.URL.Path, "/admin/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		// Everything else (SPA HTML, JS/CSS assets) is proxied to Vite dev server
		proxy.ServeHTTP(w, r)
	})
}
