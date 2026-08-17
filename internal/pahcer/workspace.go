package pahcer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

type Workspace struct {
	Dir         string
	SettingFile string
}

func PrepareWorkspace(runDir, sourceSnapshot, toolsDir, settingFile string, inputCases []domain.InputCase, threads int) (Workspace, error) {
	if len(inputCases) == 0 {
		return Workspace{}, fmt.Errorf("no input cases found")
	}
	workspaceDir := filepath.Join(runDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return Workspace{}, fmt.Errorf("could not create pahcer workspace: %w", err)
	}

	if err := copyFile(sourceSnapshot, filepath.Join(workspaceDir, "main.cpp"), 0o644); err != nil {
		return Workspace{}, fmt.Errorf("could not copy solver to workspace: %w", err)
	}
	if info, err := os.Stat(toolsDir); err == nil && info.IsDir() {
		if err := os.CopyFS(filepath.Join(workspaceDir, "tools"), os.DirFS(toolsDir)); err != nil {
			return Workspace{}, fmt.Errorf("could not copy tools to workspace: %w", err)
		}
	}

	inputDir := filepath.Join(workspaceDir, "tools", "in")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		return Workspace{}, fmt.Errorf("could not create input case directory: %w", err)
	}
	for _, directory := range []string{"out", "err"} {
		if err := os.MkdirAll(filepath.Join(workspaceDir, "tools", directory), 0o755); err != nil {
			return Workspace{}, fmt.Errorf("could not create tools/%s: %w", directory, err)
		}
	}
	for index, inputCase := range inputCases {
		stagedPath := filepath.Join(inputDir, fmt.Sprintf("%04d.txt", index))
		if err := copyFile(inputCase.Path, stagedPath, 0o644); err != nil {
			return Workspace{}, fmt.Errorf("could not copy input case to workspace: %s: %w", inputCase.ID, err)
		}
	}

	settingDestination := filepath.Join(workspaceDir, filepath.Base(settingFile))
	if err := rewriteSettingFile(settingFile, settingDestination, len(inputCases), threads); err != nil {
		return Workspace{}, err
	}
	return Workspace{Dir: workspaceDir, SettingFile: filepath.Base(settingDestination)}, nil
}

func rewriteSettingFile(source, destination string, inputCaseCount, threads int) error {
	var raw map[string]interface{}
	if _, err := toml.DecodeFile(source, &raw); err != nil {
		return fmt.Errorf("could not read pahcer settings: %w", err)
	}

	test, ok := raw["test"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("pahcer settings do not contain [test]")
	}
	test["start_seed"] = 0
	test["end_seed"] = inputCaseCount
	test["threads"] = threads
	test["out_dir"] = "pahcer"

	steps, ok := asSlice(test["test_steps"])
	if !ok {
		steps, ok = asSlice(raw["test_steps"])
	}
	if !ok || len(steps) == 0 {
		return fmt.Errorf("pahcer settings do not contain [[test_steps]]")
	}
	stdinCount := 0
	for _, value := range steps {
		step, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("pahcer test_steps are invalid")
		}
		if _, exists := step["stdin"]; exists {
			step["stdin"] = "./tools/in/{SEED04}.txt"
			stdinCount++
		}
	}
	if stdinCount == 0 {
		return fmt.Errorf("test_steps.stdin for receiving input is missing")
	}
	if _, exists := test["test_steps"]; exists {
		test["test_steps"] = steps
	} else {
		raw["test_steps"] = steps
	}

	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("could not create Run pahcer settings: %w", err)
	}
	defer file.Close()
	if err := toml.NewEncoder(file).Encode(raw); err != nil {
		return fmt.Errorf("could not write Run pahcer settings: %w", err)
	}
	return nil
}

func asSlice(value interface{}) ([]interface{}, bool) {
	switch values := value.(type) {
	case []interface{}:
		return values, true
	case []map[string]interface{}:
		result := make([]interface{}, len(values))
		for index := range values {
			result[index] = values[index]
		}
		return result, true
	default:
		return nil, false
	}
}

func copyFile(source, destination string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
