package server

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	staticFS, err := fs.Sub(staticFiles, "static")
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	path := strings.TrimPrefix(filepath.ToSlash(r.URL.Path), "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(staticFS, path); err != nil {
		path = "index.html"
	}
	http.ServeFileFS(w, r, staticFS, path)
}
