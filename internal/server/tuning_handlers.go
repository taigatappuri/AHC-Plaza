package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/cases"
	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
	bundled "github.com/taigatappuri/AHC-Plaza/internal/tuning/runtime"
)

func (s *Server) handleTuning(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host != r.Host {
			writeError(w, 403, fmt.Errorf("別オリジンからの操作はできません"))
			return
		}
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeError(w, 403, fmt.Errorf("別サイトからの操作はできません"))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/tuning/")
	if path == "environment" && r.Method == "GET" {
		writeJSON(w, 200, bundled.Status(s.Root))
		return
	}
	if path == "environment/setup" && r.Method == "POST" {
		info, e := bundled.Setup(r.Context(), s.Root)
		if e == nil {
			e = tuning.ToolHealth(r.Context(), filepath.Join(info.Path, "bin", "python3.12"))
		}
		if !writeErrorIf(w, 400, e) {
			writeJSON(w, 200, info)
		}
		return
	}
	if path == "input-directories" && r.Method == "GET" {
		s.listInputDirectories(w, r, config.TuningInputRoot)
		return
	}
	if path == "input-count" && r.Method == "GET" {
		cfg, e := config.Load(s.ConfigPath)
		if writeErrorIf(w, 400, e) {
			return
		}
		cfg.File.Execution.DefaultInputDir = config.TuningInputRoot
		dir, e := cfg.InputSetDir(r.URL.Query().Get("input_dir"))
		if writeErrorIf(w, 400, e) {
			return
		}
		inputs, e := cases.Discover(dir)
		if !writeErrorIf(w, 400, e) {
			writeJSON(w, 200, map[string]int{"count": len(inputs)})
		}
		return
	}
	if path == "scan" && r.Method == "POST" {
		var in struct {
			Solver string `json:"solver"`
		}
		if writeErrorIf(w, 400, decodeJSON(r, &in)) {
			return
		}
		scan, e := s.Tuning.Scan(in.Solver)
		if writeErrorIf(w, 400, e) {
			return
		}
		profile, e := s.Store.Profile(r.Context(), scan.Hash)
		if !writeErrorIf(w, 500, e) {
			previous, e := s.Store.PreviousProfile(r.Context(), in.Solver, scan.Hash)
			if writeErrorIf(w, 500, e) {
				return
			}
			writeJSON(w, 200, map[string]any{"scan": scan, "profile": profile, "previous_profile": previous})
		}
		return
	}
	if path == "profiles" && r.Method == "PUT" {
		var in struct {
			Solver     string             `json:"solver"`
			Hash       string             `json:"hash"`
			Parameters []params.Parameter `json:"parameters"`
		}
		if writeErrorIf(w, 400, decodeJSON(r, &in)) {
			return
		}
		scan, e := s.Tuning.Scan(in.Solver)
		if writeErrorIf(w, 400, e) {
			return
		}
		if scan.Hash != in.Hash {
			writeError(w, 409, fmt.Errorf("ソースが変更されています"))
			return
		}
		ps, e := params.Resolve(scan, in.Parameters)
		if writeErrorIf(w, 400, e) {
			return
		}
		b, _ := json.Marshal(ps)
		if !writeErrorIf(w, 500, s.Store.SaveSourceProfile(r.Context(), in.Solver, scan.Hash, b)) {
			writeJSON(w, 200, ps)
		}
		return
	}
	if path == "studies" {
		switch r.Method {
		case "GET":
			studies, e := s.Store.ListStudies(r.Context())
			if !writeErrorIf(w, 500, e) {
				writeJSON(w, 200, studies)
			}
		case "POST":
			var in tuning.StartRequest
			if writeErrorIf(w, 400, decodeJSON(r, &in)) {
				return
			}
			v, e := s.Tuning.Start(r.Context(), in)
			status := 400
			if errors.Is(e, tuning.ErrSourceChanged) {
				status = 409
			}
			if !writeErrorIf(w, status, e) {
				writeJSON(w, 202, v)
			}
		default:
			writeMethodNotAllowed(w, "GET", "POST")
		}
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "studies" {
		http.NotFound(w, r)
		return
	}
	id := parts[1]
	action := ""
	if len(parts) == 3 {
		action = parts[2]
	} else if len(parts) > 3 {
		http.NotFound(w, r)
		return
	}
	if action == "" && r.Method == "GET" {
		study, manifest, e := s.Tuning.Load(r.Context(), id)
		if !writeErrorIf(w, 404, e) {
			study, active := s.Tuning.LiveStudy(study)
			workerLog := ""
			if study.Error != "" {
				f, e := os.Open(filepath.Join(s.Root, "ahc-plaza", "tuning", id, "worker.log"))
				if e == nil {
					info, e := f.Stat()
					if e == nil {
						start := max(int64(0), info.Size()-(64<<10))
						b := make([]byte, info.Size()-start)
						n, _ := f.ReadAt(b, start)
						workerLog = string(b[:n])
					}
					f.Close()
				}
			}
			writeJSON(w, 200, map[string]any{"study": study, "manifest": manifest, "usage_bytes": s.Tuning.Usage(id), "active": active, "worker_log": workerLog})
		}
		return
	}
	if action == "" && r.Method == "DELETE" {
		result, e := s.Tuning.Delete(r.Context(), id)
		if !writeErrorIf(w, http.StatusConflict, e) {
			writeJSON(w, http.StatusOK, result)
		}
		return
	}
	if action == "trials" && r.Method == "GET" {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit := 50
		if q := r.URL.Query().Get("limit"); q != "" {
			limit, _ = strconv.Atoi(q)
		}
		if limit > 100 {
			limit = 100
		}
		trials, e := s.Store.Trials(r.Context(), id, offset, limit)
		if !writeErrorIf(w, 400, e) {
			writeJSON(w, 200, trials)
		}
		return
	}
	if action == "events" && r.Method == "GET" {
		if _, _, e := s.Tuning.Load(r.Context(), id); writeErrorIf(w, 404, e) {
			return
		}
		f, ok := w.(http.Flusher)
		if !ok {
			writeError(w, 500, fmt.Errorf("SSE unavailable"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			study, e := s.Store.GetStudy(r.Context(), id)
			if e != nil {
				return
			}
			study, active := s.Tuning.LiveStudy(study)
			writeSSE(w, map[string]any{"study": study, "active": active})
			f.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
		}
	}
	if r.Method != "POST" {
		writeMethodNotAllowed(w, "POST")
		return
	}
	switch action {
	case "pause", "cancel":
		if !writeErrorIf(w, 409, s.Tuning.Pause(id, action == "cancel")) {
			writeJSON(w, 202, map[string]string{"status": "stopping"})
		}
	case "resume":
		var in struct {
			Additional int `json:"additional_trials"`
		}
		if writeErrorIf(w, 400, decodeJSON(r, &in)) {
			return
		}
		v, e := s.Tuning.Resume(r.Context(), id, in.Additional)
		if !writeErrorIf(w, 409, e) {
			writeJSON(w, 202, v)
		}
	case "export":
		var in struct {
			Number *int `json:"number"`
		}
		if writeErrorIf(w, 400, decodeJSON(r, &in)) {
			return
		}
		v, e := s.Tuning.Export(r.Context(), id, in.Number)
		if !writeErrorIf(w, 400, e) {
			writeJSON(w, 200, v)
		}
	default:
		http.NotFound(w, r)
	}
}
