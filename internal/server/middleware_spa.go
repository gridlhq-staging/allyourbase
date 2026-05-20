// Package server contains embedded admin SPA serving helpers.
package server

import (
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/allyourbase/ayb/ui"
)

// staticSPAHandler serves the embedded admin SPA with index.html fallback
// for client-side routing support. Files are served directly from the
// embedded FS to avoid http.FileServer's index.html redirect behavior.
func staticSPAHandler(adminPath string) http.HandlerFunc {
	adminPath = normalizedAdminPath(adminPath)
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, adminPath)
		path = strings.TrimPrefix(path, "/")

		// Explicit index requests should get rewritten SPA HTML.
		if path == "" || path == "index.html" {
			serveEmbeddedIndexHTML(w, adminPath)
			return
		}

		// Try exact file; fall back to index.html for SPA routing.
		if !serveEmbeddedFile(w, path, false) {
			serveEmbeddedIndexHTML(w, adminPath)
		}
	}
}

func serveEmbeddedFile(w http.ResponseWriter, path string, mustExist bool) bool {
	f, err := ui.DistDirFS.Open(path)
	if err != nil {
		if mustExist {
			http.Error(w, "not found", http.StatusNotFound)
		}
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		if mustExist {
			http.Error(w, "not found", http.StatusNotFound)
		}
		return false
	}

	// Cache static assets (not index.html).
	if path != "index.html" {
		w.Header().Set("Cache-Control", "public, max-age=1209600")
	}
	ct := mime.TypeByExtension(filepath.Ext(path))
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
	return true
}

// serveEmbeddedIndexHTML reads the embedded index.html file, rewrites its paths with rewriteAdminIndexHTML, and writes the result to w with appropriate HTTP headers.
func serveEmbeddedIndexHTML(w http.ResponseWriter, adminPath string) {
	f, err := ui.DistDirFS.Open("index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if ct := mime.TypeByExtension(".html"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, rewriteAdminIndexHTML(string(raw), adminPath))
}

// rewriteAdminIndexHTML modifies HTML by rewriting relative asset and admin paths to be prefixed with adminPath, enabling the embedded SPA to serve correctly from a non-root URL.
func rewriteAdminIndexHTML(html string, adminPath string) string {
	adminBase := adminPathWithTrailingSlash(adminPath)
	replacer := strings.NewReplacer(
		`="/assets/`, `="`+adminBase+`assets/`,
		`='/assets/`, `='`+adminBase+`assets/`,
		`="/admin/`, `="`+adminBase,
		`='/admin/`, `='`+adminBase,
		`url(/assets/`, `url(`+adminBase+`assets/`,
		`url('/assets/`, `url('`+adminBase+`assets/`,
		`url("/assets/`, `url("`+adminBase+`assets/`,
		`url(/admin/`, `url(`+adminBase,
		`url('/admin/`, `url('`+adminBase,
		`url("/admin/`, `url("`+adminBase,
	)
	return replacer.Replace(html)
}
