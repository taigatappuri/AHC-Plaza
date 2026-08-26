package project

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
)

func Initialize(root, problem, objective string) error {
	if problem == "" {
		return errors.New("problem is required")
	}
	if objective != "max" && objective != "min" {
		return fmt.Errorf("objective must be either max or min: %q", objective)
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("could not resolve the project root: %w", err)
	}
	configPath := filepath.Join(root, "ahc-plaza.toml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("ahc-plaza.toml already exists: %s", configPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not inspect ahc-plaza.toml: %w", err)
	}

	content := fmt.Sprintf(`[project]
problem = %q
objective = %q

[paths]
solver_dir = "solver"
tools_dir = "tools"

[execution]
default_input_dir = "ahc-plaza/inputs"
threads = 0
timeout_ms = 300000

[pahcer]
setting_file = "pahcer_config.toml"

[score]
invalid_score = 0.0
include_invalid_cases = true

[statistics]
confidence_level = 0.95
bootstrap_iterations = 10000

[input_format]
`, problem, objective)

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("could not create ahc-plaza.toml: %w", err)
	}
	for _, directory := range []string{"solver", "ahc-plaza/runs", "ahc-plaza/inputs", "ahc-plaza/features"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			return fmt.Errorf("could not create %s: %w", directory, err)
		}
	}
	return nil
}

func Check(cfg config.Config) []string {
	checks := make([]string, 0)
	if compiler, err := exec.LookPath("g++"); err != nil {
		checks = append(checks, "MISSING: g++")
	} else {
		checks = append(checks, fmt.Sprintf("OK: g++ (%s)", compiler))
	}
	for _, item := range [][2]string{
		{"solver", cfg.File.Paths.SolverDir},
		{"tools", cfg.File.Paths.ToolsDir},
		{"inputs", cfg.File.Execution.DefaultInputDir},
		{"features", "features"},
	} {
		var path string
		var err error
		if item[0] == "features" {
			path, err = cfg.ResolvePath(item[0], item[1])
		} else {
			path, err = cfg.ResolveProjectPath(item[0], item[1])
		}
		if err != nil {
			checks = append(checks, fmt.Sprintf("MISSING: %s", item[1]))
			continue
		}
		if _, err := os.Stat(path); err != nil {
			checks = append(checks, fmt.Sprintf("MISSING: %s", item[1]))
			continue
		}
		checks = append(checks, fmt.Sprintf("OK: %s", item[1]))
	}
	return checks
}
