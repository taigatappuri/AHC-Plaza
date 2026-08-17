package server

import (
	"fmt"
	"net/http"
	"path/filepath"
)

func (s *Server) handleInputFeatures(w http.ResponseWriter, r *http.Request) {
	pathRoot := filepath.Join(s.Root, "ahc-plaza")
	sources, err := listRegularFiles(pathRoot, filepath.Join(pathRoot, "features"), ".cpp")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("特徴量ソースを一覧できません: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}
