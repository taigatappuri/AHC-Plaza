package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/taigatappuri/AHC-Plaza/internal/cases"
	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

// PreparedRun はStudy開始時のコピーだけを参照します。
type PreparedRun struct {
	CompilerVersion string             `json:"compiler_version"`
	PahcerVersion   string             `json:"pahcer_version"`
	ConfigHash      string             `json:"config_hash"`
	Config          config.Config      `json:"config"`
	Inputs          []domain.InputCase `json:"inputs"`
	InputDir        string             `json:"input_dir"`
	ToolsDir        string             `json:"tools_dir"`
	SettingFile     string             `json:"setting_file"`
}

func PrepareTuningInputs(ctx context.Context, request RunRequest, destination string) (PreparedRun, error) {
	cfg, e := config.Load(request.ConfigPath)
	if e != nil {
		return PreparedRun{}, e
	}
	cfg.File.Execution.DefaultInputDir = config.TuningInputRoot
	p := PreparedRun{Config: cfg, ConfigHash: fileHash(cfg.FilePath)}
	dir, e := cfg.InputSetDir(request.InputDir)
	if e != nil {
		return p, e
	}
	inputs, e := cases.Discover(dir)
	if e != nil {
		return p, e
	}
	if len(inputs) == 0 {
		return p, fmt.Errorf("入力ケースがありません")
	}
	p.InputDir = filepath.Join(destination, "inputs")
	p.ToolsDir = filepath.Join(destination, "tools")
	p.SettingFile = filepath.Join(destination, "pahcer_config.toml")
	if e = os.MkdirAll(p.InputDir, 0755); e != nil {
		return p, e
	}
	tools, e := cfg.ResolveProjectPath("tools", cfg.File.Paths.ToolsDir)
	if e != nil {
		return p, e
	}
	if _, e = os.Stat(tools); e == nil {
		if e = os.CopyFS(p.ToolsDir, os.DirFS(tools)); e != nil {
			return p, e
		}
	}
	setting := request.SettingFile
	if setting == "" {
		setting = cfg.File.Pahcer.SettingFile
	}
	setting, e = cfg.ResolveProjectPath("setting", setting)
	if e != nil {
		return p, e
	}
	copyFile := func(src, dst string) error {
		b, e := os.ReadFile(src)
		if e != nil {
			return e
		}
		return os.WriteFile(dst, b, 0644)
	}
	if e = copyFile(setting, p.SettingFile); e != nil {
		return p, e
	}
	for _, input := range inputs {
		if e = ctx.Err(); e != nil {
			return p, e
		}
		if e = copyFile(input.Path, filepath.Join(p.InputDir, filepath.Base(input.Path))); e != nil {
			return p, e
		}
	}
	p.Inputs, e = cases.Discover(p.InputDir)
	return p, e
}
