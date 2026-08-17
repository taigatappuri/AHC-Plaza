package server

import (
	"net/http"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
)

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.Load(s.ConfigPath)
		if writeErrorIf(w, http.StatusInternalServerError, err) {
			return
		}
		writeJSON(w, http.StatusOK, cfg.File)
	case http.MethodPut:
		var file config.FileConfig
		if writeErrorIf(w, http.StatusBadRequest, decodeJSON(r, &file)) {
			return
		}
		cfg, err := config.Save(s.ConfigPath, file)
		if writeErrorIf(w, http.StatusBadRequest, err) {
			return
		}
		writeJSON(w, http.StatusOK, cfg.File)
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}
