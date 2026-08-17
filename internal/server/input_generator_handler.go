package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/process"
)

var inputGeneratorNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)
var inputGeneratorCandidateRE = regexp.MustCompile(`^(gen|generator)[A-Za-z0-9_-]*$`)
var inputParameterNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)

func (s *Server) handleInputGenerators(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.ConfigPath)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	directory, err := cfg.ResolveProjectPath("paths.tools_dir", cfg.File.Paths.ToolsDir)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	options, err := discoverInputGenerators(cfg.ProjectRoot, cfg.File.Paths.ToolsDir, directory)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generators": options})
}

func discoverInputGenerators(root, relativeDir, directory string) ([]inputGeneratorOption, error) {
	binDirectory := filepath.Join(directory, "src", "bin")
	entries, err := os.ReadDir(binDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return []inputGeneratorOption{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("入力生成器を一覧できません: %w", err)
	}
	options := make([]inputGeneratorOption, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".rs" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !inputGeneratorNameRE.MatchString(name) || !inputGeneratorCandidateRE.MatchString(name) {
			continue
		}
		source, err := filepath.Rel(root, filepath.Join(binDirectory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("入力生成器のパスを解決できません: %w", err)
		}
		options = append(options, inputGeneratorOption{
			Name:   name,
			Dir:    filepath.ToSlash(relativeDir),
			Source: filepath.ToSlash(source),
		})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Name < options[j].Name })
	return options, nil
}

func (s *Server) handleToolInputGenerate(w http.ResponseWriter, r *http.Request) {
	var request toolInputGenerateRequest
	if writeErrorIf(w, http.StatusBadRequest, decodeJSON(r, &request)) {
		return
	}
	if request.Generator == "" {
		request.Generator = "gen"
	}
	if !inputGeneratorNameRE.MatchString(request.Generator) {
		writeError(w, http.StatusBadRequest, errors.New("生成器名が不正です"))
		return
	}
	if request.CaseCount < 1 || request.CaseCount > 10000 {
		writeError(w, http.StatusBadRequest, errors.New("ケース数は1〜10,000にしてください"))
		return
	}
	request.InputDirectory = strings.TrimSpace(request.InputDirectory)
	if !validInputDirectoryName(request.InputDirectory) {
		writeError(w, http.StatusBadRequest, errors.New("入力ディレクトリ名は64文字以内で、パス区切りや制御文字を含めないでください"))
		return
	}
	if writeErrorIf(w, http.StatusBadRequest, validateInputParameters(request.Parameters)) {
		return
	}
	cfg, err := config.Load(s.ConfigPath)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	if request.GeneratorDir == "" {
		request.GeneratorDir = cfg.File.Paths.ToolsDir
	}
	generatorDir, err := cfg.ResolveProjectPath("generator directory", request.GeneratorDir)
	if writeErrorIf(w, http.StatusBadRequest, err) {
		return
	}
	inputRoot, err := cfg.InputDir("")
	if writeErrorIf(w, http.StatusBadRequest, err) {
		return
	}
	outputDir := filepath.Join(inputRoot, request.InputDirectory)
	if writeErrorIf(w, http.StatusBadRequest, validateGenerator(generatorDir, request.Generator)) {
		return
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("出力ディレクトリを作成できません: %w", err))
		return
	}

	caseNames := make([]string, request.CaseCount)
	for index := range caseNames {
		caseNames[index] = fmt.Sprintf("%04d.txt", index)
		if !request.Overwrite {
			if _, statErr := os.Stat(filepath.Join(outputDir, caseNames[index])); statErr == nil {
				writeError(w, http.StatusConflict, fmt.Errorf("出力ファイルが存在します: %s", caseNames[index]))
				return
			} else if !errors.Is(statErr, os.ErrNotExist) {
				writeError(w, http.StatusInternalServerError, fmt.Errorf("出力ファイルを確認できません: %w", statErr))
				return
			}
		}
	}

	workDirectory, err := os.MkdirTemp(cfg.PathRoot, "input-generation-")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("生成器の作業領域を作成できません: %w", err))
		return
	}
	defer os.RemoveAll(workDirectory)
	seedsPath := filepath.Join(workDirectory, "seeds.txt")
	if writeErrorIf(w, http.StatusInternalServerError, writeSeeds(seedsPath, request.SeedStart, request.CaseCount)) {
		return
	}
	constraintsPath := filepath.Join(workDirectory, "constraints.txt")
	if writeErrorIf(w, http.StatusInternalServerError, writeInputParameters(constraintsPath, request.Parameters)) {
		return
	}
	stagingDirectory := filepath.Join(workDirectory, "out")
	if err := os.MkdirAll(stagingDirectory, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("一時生成ディレクトリを作成できません: %w", err))
		return
	}
	stdoutPath := filepath.Join(workDirectory, "stdout.log")
	stderrPath := filepath.Join(workDirectory, "stderr.log")
	result, err := process.Run(r.Context(), process.Request{
		Command: []string{"cargo", "run", "--release", "--bin", request.Generator, "--", seedsPath, "--dir", stagingDirectory},
		Dir:     generatorDir,
		Env: []string{
			"AHC_PLAZA_CONSTRAINTS_FILE=" + constraintsPath,
			"AHC_PLAZA_CONSTRAINTS_VERSION=1",
		},
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
		Timeout:    10 * time.Minute,
	})
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	if result.Status != process.StatusSucceeded {
		stderr := readGeneratorLog(stderrPath)
		message := fmt.Sprintf("生成器の実行に失敗しました (status=%s)", result.Status)
		if stderr != "" {
			message += ": " + stderr
		}
		writeError(w, http.StatusUnprocessableEntity, errors.New(message))
		return
	}
	for _, name := range caseNames {
		info, statErr := os.Stat(filepath.Join(stagingDirectory, name))
		if statErr != nil || !info.Mode().IsRegular() {
			writeError(w, http.StatusUnprocessableEntity, fmt.Errorf("生成器が必要なファイルを作成しませんでした: %s", name))
			return
		}
	}
	for _, name := range caseNames {
		if err := os.Rename(filepath.Join(stagingDirectory, name), filepath.Join(outputDir, name)); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("生成結果を配置できません: %w", err))
			return
		}
	}

	relativeOutputDir, err := filepath.Rel(cfg.ProjectRoot, outputDir)
	if writeErrorIf(w, http.StatusInternalServerError, err) {
		return
	}
	response := toolInputGenerateResponse{
		OutputDir: filepath.ToSlash(relativeOutputDir),
		CaseCount: request.CaseCount,
	}
	writeJSON(w, http.StatusCreated, response)
}

func validInputDirectoryName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") || utf8.RuneCountInString(name) > 64 {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) || strings.ContainsRune(`/\\:*?"<>|`, character) {
			return false
		}
	}
	return true
}

func validateGenerator(directory, name string) error {
	for _, path := range []string{filepath.Join(directory, "Cargo.toml"), filepath.Join(directory, "src", "bin", name+".rs")} {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("生成器ファイルがありません: %s", filepath.ToSlash(path))
		}
		if err != nil {
			return fmt.Errorf("生成器ファイルを確認できません: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("生成器パスがファイルではありません: %s", filepath.ToSlash(path))
		}
	}
	return nil
}

func writeSeeds(path string, start uint64, count int) error {
	if count > 0 && ^uint64(0)-start < uint64(count-1) {
		return errors.New("seedが範囲外です")
	}
	var content strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintln(&content, start+uint64(index))
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
		return fmt.Errorf("seedファイルを作成できません: %w", err)
	}
	return nil
}

func validateInputParameters(parameters []inputParameter) error {
	if len(parameters) > 128 {
		return errors.New("入力条件は最大128件です")
	}
	seen := make(map[string]struct{}, len(parameters))
	for index, parameter := range parameters {
		name := strings.TrimSpace(parameter.Name)
		constraint := strings.TrimSpace(parameter.Constraint)
		if name == "" || constraint == "" {
			return fmt.Errorf("入力条件%dにパラメータ名と式が必要です", index+1)
		}
		if !inputParameterNameRE.MatchString(name) {
			return fmt.Errorf("入力条件%dのパラメータ名が不正です: %s", index+1, name)
		}
		if len(constraint) > 1000 || strings.ContainsAny(constraint, "\r\n\t") {
			return fmt.Errorf("入力条件%dが長すぎるか改行・タブを含みます", index+1)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("入力条件のパラメータ名が重複しています: %s", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func writeInputParameters(path string, parameters []inputParameter) error {
	var content strings.Builder
	content.WriteString("# AHC_PLAZA_CONSTRAINTS_VERSION=1\n")
	for _, parameter := range parameters {
		fmt.Fprintf(&content, "%s\t%s\n", strings.TrimSpace(parameter.Name), strings.TrimSpace(parameter.Constraint))
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
		return fmt.Errorf("入力条件ファイルを作成できません: %w", err)
	}
	return nil
}

func readGeneratorLog(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	message := strings.TrimSpace(string(content))
	if len(message) > 1000 {
		message = message[len(message)-1000:]
	}
	return message
}
