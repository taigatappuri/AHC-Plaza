package server

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
)

const (
	visualizerURLHost  = "img.atcoder.jp"
	maxVisualizerFile  = 32 << 20
	maxVisualizerFiles = 128
)

var (
	visualizerScriptSourceRE  = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	visualizerLinkHrefRE      = regexp.MustCompile(`(?is)<link\b[^>]*\bhref\s*=\s*["']([^"']+)["']`)
	visualizerImportRE        = regexp.MustCompile("(?m)\\bimport(?:[^\"'`]*?\\bfrom\\s*)?\\s*[\"'`]([^\"'`]+?\\.js(?:[?#][^\"'`]*)?)[\"'`]")
	visualizerDynamicImportRE = regexp.MustCompile("(?m)\\bimport\\s*\\(\\s*[\"'`]([^\"'`]+?\\.js(?:[?#][^\"'`]*)?)[\"'`]\\s*\\)")
	visualizerWasmRE          = regexp.MustCompile("[\"'`]([^\"'`]+?\\.wasm(?:[?#][^\"'`]*)?)[\"'`]")
	visualizerProtocolURLRE   = regexp.MustCompile(`([="'(])//img\.atcoder\.jp/`)
)

type visualizerStatusResponse struct {
	Ready bool   `json:"ready"`
	Entry string `json:"entry,omitempty"`
}

type visualizerDownloadResponse struct {
	Ready bool     `json:"ready"`
	Entry string   `json:"entry"`
	Files []string `json:"files"`
}

func (s *Server) visualizerDirectory() string {
	return filepath.Join(s.Root, "ahc-plaza", "visualizer")
}

func (s *Server) handleVisualizer(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.visualizerMu.Lock()
		defer s.visualizerMu.Unlock()
		if err := os.RemoveAll(s.visualizerDirectory()); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("ビジュアライザを削除できません: %w", err))
			return
		}
		writeJSON(w, http.StatusOK, visualizerStatusResponse{})
		return
	}
	s.visualizerMu.Lock()
	defer s.visualizerMu.Unlock()
	entry, err := visualizerEntry(s.visualizerDirectory())
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	writeJSON(w, http.StatusOK, visualizerStatusResponse{Ready: entry != "", Entry: entry})
}

func (s *Server) handleVisualizerDownload(w http.ResponseWriter, r *http.Request) {
	var request visualizerDownloadRequest
	if writeErrorIf(w, http.StatusBadRequest, decodeJSON(r, &request)) {
		return
	}

	s.visualizerMu.Lock()
	defer s.visualizerMu.Unlock()
	entry, files, err := downloadVisualizer(r.Context(), s.visualizerDirectory(), request.URL)
	if writeErrorIf(w, http.StatusBadRequest, err) {
		return
	}
	writeJSON(w, http.StatusOK, visualizerDownloadResponse{Ready: true, Entry: entry, Files: files})
}

func (s *Server) handleVisualizerAsset(w http.ResponseWriter, r *http.Request) {
	relative := strings.TrimPrefix(r.URL.Path, "/visualizer/")
	if relative == "" || relative == "." {
		entry, err := visualizerEntry(s.visualizerDirectory())
		if err != nil || entry == "" {
			if err == nil {
				err = os.ErrNotExist
			}
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		relative = entry
	}
	if filepath.IsAbs(relative) {
		http.Error(w, "ビジュアライザのパスが不正です", http.StatusBadRequest)
		return
	}
	relative = path.Clean(strings.ReplaceAll(relative, "\\", "/"))
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		http.Error(w, "ビジュアライザのパスが不正です", http.StatusBadRequest)
		return
	}
	target := filepath.Join(s.visualizerDirectory(), filepath.FromSlash(relative))
	within, err := config.IsWithin(s.visualizerDirectory(), target)
	if err != nil || !within {
		http.Error(w, "ビジュアライザのパスが不正です", http.StatusForbidden)
		return
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, target)
}

func (s *Server) getCaseFile(w http.ResponseWriter, r *http.Request, runID string, input bool) {
	if !validRunID(runID) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("Run IDが不正です"))
		return
	}
	caseID := r.URL.Query().Get("case_id")
	if caseID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("case_idは必須です"))
		return
	}
	cases, ok := s.loadCases(w, r, runID)
	if !ok {
		return
	}
	for _, item := range cases {
		if item.InputCaseID != caseID {
			continue
		}
		kind, storedPath := "出力", item.OutputPath
		if input {
			kind, storedPath = "入力", item.InputPath
		}
		if storedPath == "" {
			writeError(w, http.StatusNotFound, fmt.Errorf("ケース%sが保存されていません", kind))
			return
		}
		if filepath.IsAbs(storedPath) {
			writeError(w, http.StatusForbidden, fmt.Errorf("%sパスが不正です", kind))
			return
		}
		filePath := filepath.Join(s.Root, filepath.FromSlash(storedPath))
		within, pathErr := config.IsWithin(s.Root, filePath)
		if pathErr != nil || !within {
			writeError(w, http.StatusForbidden, fmt.Errorf("%sパスが不正です", kind))
			return
		}
		content, readErr := os.ReadFile(filePath)
		if os.IsNotExist(readErr) {
			writeError(w, http.StatusNotFound, fmt.Errorf("ケース%sファイルがありません", kind))
			return
		}
		if readErr != nil {
			writeError(w, http.StatusInternalServerError, readErr)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(content)
		return
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("ケースがありません"))
}

func validRunID(runID string) bool {
	return runID != "" && filepath.Base(runID) == runID && runID != "." && runID != string(filepath.Separator)
}

func visualizerEntry(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("ビジュアライザのディレクトリを読めません: %w", err)
	}
	htmlFiles := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".html") {
			htmlFiles = append(htmlFiles, entry.Name())
		}
	}
	if len(htmlFiles) == 0 {
		return "", nil
	}
	if len(htmlFiles) > 1 {
		return "", fmt.Errorf("ビジュアライザHTMLが複数あります")
	}
	return htmlFiles[0], nil
}

type visualizerDownloader struct {
	root      string
	baseURL   *url.URL
	client    *http.Client
	files     map[string]bool
	fileCount int
}

func downloadVisualizer(ctx context.Context, outputRoot, rawURL string) (string, []string, error) {
	baseURL, err := validateVisualizerURL(rawURL, true)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outputRoot), 0o755); err != nil {
		return "", nil, fmt.Errorf("ビジュアライザの保存先を作成できません: %w", err)
	}
	temporary, err := os.MkdirTemp(filepath.Dir(outputRoot), ".visualizer-download-")
	if err != nil {
		return "", nil, fmt.Errorf("一時保存先を作成できません: %w", err)
	}
	defer os.RemoveAll(temporary)

	downloader := &visualizerDownloader{
		root:    temporary,
		baseURL: baseURL,
		client:  &http.Client{CheckRedirect: visualizerRedirectPolicy},
		files:   make(map[string]bool),
	}
	htmlName := path.Base(baseURL.Path)
	html, err := downloader.fetch(ctx, baseURL)
	if err != nil {
		return "", nil, err
	}
	if err := downloader.writeFile(htmlName, html); err != nil {
		return "", nil, err
	}
	downloader.files[htmlName] = true
	if err := downloader.downloadHTMLAssets(ctx, htmlName, html); err != nil {
		return "", nil, err
	}

	if err := replaceDirectory(temporary, outputRoot); err != nil {
		return "", nil, err
	}
	return htmlName, slices.Sorted(maps.Keys(downloader.files)), nil
}

func replaceDirectory(source, destination string) error {
	backup := destination + ".backup"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("既存ビジュアライザを退避できません: %w", err)
		}
	}
	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return fmt.Errorf("ビジュアライザを配置できません: %w", err)
	}
	_ = os.RemoveAll(backup)
	return nil
}

func (d *visualizerDownloader) downloadHTMLAssets(ctx context.Context, htmlName string, html []byte) error {
	text := string(html)
	base := d.baseURL
	for _, expression := range []*regexp.Regexp{visualizerScriptSourceRE, visualizerImportRE, visualizerDynamicImportRE} {
		for _, raw := range uniqueMatches(expression, text) {
			if err := d.downloadAsset(ctx, raw, base, 0); err != nil {
				return err
			}
		}
	}
	for _, raw := range uniqueMatches(visualizerLinkHrefRE, text) {
		if strings.HasSuffix(strings.ToLower(strings.Split(raw, "?")[0]), ".js") {
			if err := d.downloadAsset(ctx, raw, base, 0); err != nil {
				return err
			}
		}
	}
	for _, raw := range uniqueMatches(visualizerWasmRE, text) {
		if err := d.downloadAsset(ctx, raw, base, 0); err != nil {
			return err
		}
	}

	// AtCoderのHTMLで使われる protocol-relative URLを、保存した相対パスへ切り替えます。
	text = visualizerProtocolURLRE.ReplaceAllString(text, `$1./`+visualizerURLHost+`/`)
	return d.writeFile(htmlName, []byte(text))
}

func (d *visualizerDownloader) downloadAsset(ctx context.Context, raw string, base *url.URL, depth int) error {
	abs, err := d.resolveAsset(raw, base)
	if err != nil {
		return err
	}
	if path.Ext(abs.Path) != ".wasm" && depth > 3 {
		return nil
	}
	local, err := d.localPath(abs)
	if err != nil {
		return err
	}
	if d.files[local] {
		return nil
	}
	d.files[local] = true
	content, err := d.fetch(ctx, abs)
	if err != nil {
		return err
	}
	if err := d.writeFile(local, content); err != nil {
		return err
	}
	if path.Ext(abs.Path) == ".wasm" {
		return nil
	}
	text := string(content)
	for _, rawWASM := range uniqueMatches(visualizerWasmRE, text) {
		if err := d.downloadAsset(ctx, rawWASM, abs, depth); err != nil {
			return err
		}
	}
	for _, expression := range []*regexp.Regexp{visualizerImportRE, visualizerDynamicImportRE} {
		for _, rawImport := range uniqueMatches(expression, text) {
			if err := d.downloadAsset(ctx, rawImport, abs, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *visualizerDownloader) resolveAsset(raw string, base *url.URL) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" || strings.HasPrefix(raw, "data:") {
		return nil, fmt.Errorf("ビジュアライザ素材のURLが不正です")
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("素材URLを解析できません: %w", err)
	}
	abs := base.ResolveReference(ref)
	if _, err := validateVisualizerURL(abs.String(), false); err != nil {
		return nil, err
	}
	return abs, nil
}

func validateVisualizerURL(raw string, requireHTML bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != visualizerURLHost {
		return nil, fmt.Errorf("ビジュアライザURLはhttps://%sを使用してください", visualizerURLHost)
	}
	if parsed.Path == "" || (requireHTML && !strings.HasSuffix(strings.ToLower(parsed.Path), ".html")) {
		return nil, fmt.Errorf("ビジュアライザURLはHTMLを指定してください")
	}
	return parsed, nil
}

func visualizerRedirectPolicy(request *http.Request, via []*http.Request) error {
	if _, err := validateVisualizerURL(request.URL.String(), false); err != nil {
		return fmt.Errorf("ビジュアライザの転送先が許可されていません")
	}
	return nil
}

func (d *visualizerDownloader) fetch(ctx context.Context, target *url.URL) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := d.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("ビジュアライザ素材を取得できません: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("ビジュアライザ素材の取得に失敗しました: %s", response.Status)
	}
	if d.fileCount >= maxVisualizerFiles {
		return nil, fmt.Errorf("ビジュアライザ素材が多すぎます")
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxVisualizerFile+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxVisualizerFile {
		return nil, fmt.Errorf("ビジュアライザ素材が大きすぎます")
	}
	d.fileCount++
	return content, nil
}

func (d *visualizerDownloader) localPath(target *url.URL) (string, error) {
	basePath := path.Dir(d.baseURL.Path) + "/"
	remotePath := path.Clean(target.Path)
	var relative string
	if strings.HasPrefix(remotePath, basePath) {
		relative = strings.TrimPrefix(remotePath, basePath)
	} else {
		relative = target.Host + "/" + strings.TrimPrefix(remotePath, "/")
	}
	relative = path.Clean(relative)
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("ビジュアライザ素材のパスが不正です")
	}
	return relative, nil
}

func (d *visualizerDownloader) writeFile(relative string, content []byte) error {
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") {
		return fmt.Errorf("ビジュアライザの保存先が不正です")
	}
	target := filepath.Join(d.root, filepath.FromSlash(relative))
	within, err := config.IsWithin(d.root, target)
	if err != nil || !within {
		return fmt.Errorf("ビジュアライザの保存先が不正です")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("ビジュアライザのディレクトリを作成できません: %w", err)
	}
	return os.WriteFile(target, content, 0o644)
}

func uniqueMatches(expression *regexp.Regexp, content string) []string {
	matches := expression.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		result = append(result, match[1])
	}
	return result
}
