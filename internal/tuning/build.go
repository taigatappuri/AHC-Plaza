package tuning

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning/params"
)

type buildStep struct {
	Program    string   `toml:"program"`
	Args       []string `toml:"args"`
	CurrentDir string   `toml:"current_dir"`
	Stdin      string   `toml:"stdin"`
}

type BuildInspection struct {
	SourceTarget string            `json:"source_target"`
	Tools        map[string]string `json:"tools"`
}

func inspectBuild(ctx context.Context, setting, projectRoot, selectedSolver, explicitTarget string) (BuildInspection, error) {
	var raw struct {
		Test struct {
			Compile []buildStep `toml:"compile_steps"`
			Test    []buildStep `toml:"test_steps"`
		} `toml:"test"`
		Compile   []buildStep `toml:"compile_steps"`
		TestSteps []buildStep `toml:"test_steps"`
	}
	if _, err := toml.DecodeFile(setting, &raw); err != nil {
		return BuildInspection{}, err
	}
	compile := raw.Test.Compile
	if len(compile) == 0 {
		compile = raw.Compile
	}
	tests := raw.Test.Test
	if len(tests) == 0 {
		tests = raw.TestSteps
	}
	if len(compile) == 0 {
		return BuildInspection{}, fmt.Errorf("compile_stepsがありません")
	}
	if len(tests) == 0 {
		return BuildInspection{}, fmt.Errorf("test_stepsがありません")
	}
	hasStdin := false
	for _, step := range tests {
		if strings.TrimSpace(step.Stdin) != "" {
			hasStdin = true
		}
	}
	if !hasStdin {
		return BuildInspection{}, fmt.Errorf("test_stepsにstdinを受け取るstepがありません")
	}
	all := []struct {
		kind  string
		steps []buildStep
	}{{"compile_steps", compile}, {"test_steps", tests}}
	tools := map[string]string{}
	for _, group := range all {
		for i, step := range group.steps {
			if strings.TrimSpace(step.Program) == "" {
				return BuildInspection{}, fmt.Errorf("%s[%d].programが空です", group.kind, i)
			}
			cwd, err := workspaceRelativeDir(step.CurrentDir)
			if err != nil {
				return BuildInspection{}, fmt.Errorf("%s[%d].current_dir: %w", group.kind, i, err)
			}
			resolved, exists, err := resolveProgram(projectRoot, cwd, step.Program)
			if err != nil {
				return BuildInspection{}, fmt.Errorf("%s[%d].program %q: %w", group.kind, i, step.Program, err)
			}
			if exists {
				b, err := os.ReadFile(resolved)
				if err != nil {
					return BuildInspection{}, err
				}
				tools[fmt.Sprintf("%s[%d]:%s", group.kind, i, step.Program)] = resolved + "\n" + params.Hash(b)
			} else if !strings.ContainsAny(step.Program, "/\\") || group.kind == "compile_steps" && i == 0 {
				return BuildInspection{}, fmt.Errorf("%s[%d].program %qを解決できません", group.kind, i, step.Program)
			}
		}
	}
	// pahcer本体も再開時に同一性を確認します。
	if path, err := exec.LookPath("pahcer"); err == nil {
		b, e := os.ReadFile(path)
		if e != nil {
			return BuildInspection{}, e
		}
		tools["runner:pahcer"] = path + "\n" + params.Hash(b)
	} else {
		return BuildInspection{}, err
	}
	target, err := inferSourceTarget(projectRoot, selectedSolver, explicitTarget, compile)
	if err != nil {
		return BuildInspection{}, err
	}
	return BuildInspection{SourceTarget: target, Tools: tools}, nil
}

func workspaceRelativeDir(value string) (string, error) {
	if value == "" {
		return ".", nil
	}
	v := filepath.Clean(filepath.FromSlash(value))
	if filepath.IsAbs(v) || !filepath.IsLocal(v) {
		return "", fmt.Errorf("workspace内の相対pathを指定してください")
	}
	return v, nil
}

func resolveProgram(root, cwd, program string) (string, bool, error) {
	if filepath.IsAbs(program) {
		info, err := os.Stat(program)
		if err != nil {
			return "", false, err
		}
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("通常ファイルではありません")
		}
		if info.Mode().Perm()&0111 == 0 {
			return "", false, fmt.Errorf("実行可能ファイルではありません")
		}
		return filepath.Clean(program), true, nil
	}
	if !strings.ContainsAny(program, "/\\") {
		path, err := exec.LookPath(program)
		if err != nil {
			return "", false, nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", false, err
		}
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("通常ファイルではありません")
		}
		if info.Mode().Perm()&0111 == 0 {
			return "", false, fmt.Errorf("実行可能ファイルではありません")
		}
		abs, _ := filepath.Abs(path)
		return abs, true, nil
	}
	rel := filepath.Clean(filepath.Join(cwd, filepath.FromSlash(program)))
	if !filepath.IsLocal(rel) {
		return "", false, fmt.Errorf("workspace外を参照しています")
	}
	path := filepath.Join(root, rel)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return path, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("通常ファイルではありません")
	}
	if info.Mode().Perm()&0111 == 0 {
		return "", false, fmt.Errorf("実行可能ファイルではありません")
	}
	return path, true, nil
}

func inferSourceTarget(root, selected, explicit string, steps []buildStep) (string, error) {
	validate := func(value string) (string, error) {
		v := filepath.Clean(filepath.FromSlash(value))
		if filepath.IsAbs(v) || !filepath.IsLocal(v) {
			return "", fmt.Errorf("tuning.source_targetはworkspace内の相対pathにしてください")
		}
		info, err := os.Lstat(filepath.Join(root, v))
		if err != nil {
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("source target %q: %w", value, err)
			}
			for parent := filepath.Dir(v); parent != "."; parent = filepath.Dir(parent) {
				parentInfo, parentErr := os.Lstat(filepath.Join(root, parent))
				if os.IsNotExist(parentErr) {
					continue
				}
				if parentErr != nil {
					return "", fmt.Errorf("source target %q: %w", value, parentErr)
				}
				if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
					return "", fmt.Errorf("source target %qの親pathがディレクトリではありません", value)
				}
			}
			return filepath.ToSlash(v), nil
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("source target %qは通常ファイルにしてください", value)
		}
		return filepath.ToSlash(v), nil
	}
	if explicit != "" {
		return validate(explicit)
	}
	selectedClean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(selected)))
	candidates := map[string]bool{}
	for _, step := range steps {
		cwd, _ := workspaceRelativeDir(step.CurrentDir)
		for _, arg := range step.Args {
			if strings.EqualFold(filepath.Ext(arg), ".cpp") {
				v := filepath.ToSlash(filepath.Clean(filepath.Join(cwd, filepath.FromSlash(arg))))
				if filepath.IsLocal(v) {
					candidates[v] = true
				}
			}
		}
	}
	if candidates[selectedClean] {
		return validate(selectedClean)
	}
	basenameMatches := []string{}
	for candidate := range candidates {
		if filepath.Base(candidate) == filepath.Base(selectedClean) {
			basenameMatches = append(basenameMatches, candidate)
		}
	}
	if len(basenameMatches) == 1 {
		return validate(basenameMatches[0])
	}
	if len(candidates) == 1 {
		for candidate := range candidates {
			return validate(candidate)
		}
	}
	if len(candidates) > 1 {
		return "", fmt.Errorf("compile_stepsにC++ソースが複数あります。tuning.source_targetを指定してください")
	}
	if selectedClean != "" {
		if v, err := validate(selectedClean); err == nil && filepath.Base(v) == "main.cpp" {
			return v, nil
		}
	}
	if _, err := os.Stat(filepath.Join(root, "main.cpp")); err == nil {
		return validate("main.cpp")
	}
	return "", fmt.Errorf("調整対象ソースを推定できません。tuning.source_targetを指定してください")
}
