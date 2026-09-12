package usecase

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	ProjectDir      string             `json:"project_dir,omitempty"`
	SourceTarget    string             `json:"source_target,omitempty"`
}

type SnapshotLimits struct {
	MaxFiles int
	MaxBytes int64
	Exclude  []string
}

var defaultSnapshotExcludes = map[string]bool{".git": true, "ahc-plaza": true, "pahcer": true, "build": true, "target": true, "dist": true}

// SnapshotTuningProject はリンクと過大なツリーを拒否してプロジェクト資材を固定します。
func SnapshotTuningProject(ctx context.Context, root, destination string, limits SnapshotLimits) error {
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = 20000
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = 512 << 20
	}
	excluded := map[string]bool{}
	for k, v := range defaultSnapshotExcludes {
		excluded[k] = v
	}
	for _, name := range limits.Exclude {
		if name != "" {
			excluded[filepath.Clean(name)] = true
		}
	}
	if relDestination, err := filepath.Rel(root, destination); err == nil && relDestination != "." && filepath.IsLocal(relDestination) {
		excluded[filepath.Clean(relDestination)] = true
	}
	files := 0
	var size int64
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(destination, 0755)
		}
		for ex := range excluded {
			if rel == ex || strings.HasPrefix(rel, ex+string(os.PathSeparator)) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("project snapshotではsymlinkを使用できません: %s", rel)
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(destination, rel), info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("project snapshotには通常ファイルだけを使用できます: %s", rel)
		}
		files++
		size += info.Size()
		if files > limits.MaxFiles {
			return fmt.Errorf("project snapshotのファイル数が上限%dを超えました", limits.MaxFiles)
		}
		if size > limits.MaxBytes {
			return fmt.Errorf("project snapshotの容量が上限%d bytesを超えました", limits.MaxBytes)
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(destination, rel)
		dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		sourceCloseErr := src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if sourceCloseErr != nil {
			return sourceCloseErr
		}
		return closeErr
	})
}

func PrepareTuningInputs(ctx context.Context, request RunRequest, destination string) (PreparedRun, error) {
	return prepareTuningInputs(ctx, request, destination, true)
}

// PrepareTuningValidationInputs は検証用入力だけを固定します。
func PrepareTuningValidationInputs(ctx context.Context, request RunRequest, destination string) (PreparedRun, error) {
	return prepareTuningInputs(ctx, request, destination, false)
}

func prepareTuningInputs(ctx context.Context, request RunRequest, destination string, includeBuildAssets bool) (PreparedRun, error) {
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
	if includeBuildAssets {
		p.ToolsDir = filepath.Join(destination, "tools")
		p.SettingFile = filepath.Join(destination, "pahcer_config.toml")
		p.ProjectDir = filepath.Join(destination, "project")
	}
	if e = os.MkdirAll(p.InputDir, 0755); e != nil {
		return p, e
	}
	if !includeBuildAssets {
		for _, input := range inputs {
			if e = ctx.Err(); e != nil {
				return p, e
			}
			if e = copyRegularFile(input.Path, filepath.Join(p.InputDir, filepath.Base(input.Path)), 0644); e != nil {
				return p, e
			}
		}
		p.Inputs, e = cases.Discover(p.InputDir)
		return p, e
	}
	tools, e := cfg.ResolveProjectPath("tools", cfg.File.Paths.ToolsDir)
	if e != nil {
		return p, e
	}
	if toolsInfo, statErr := os.Stat(tools); statErr == nil {
		if !toolsInfo.IsDir() {
			return p, fmt.Errorf("paths.tools_dirはディレクトリにしてください")
		}
		if e = copyPreparedTree(ctx, tools, p.ToolsDir); e != nil {
			return p, e
		}
	} else if !os.IsNotExist(statErr) {
		return p, statErr
	}
	inputRoot, e := cfg.ResolveProjectPath("input root", config.TuningInputRoot)
	if e != nil {
		return p, e
	}
	exclude := []string{relativePath(cfg.ProjectRoot, tools), relativePath(cfg.ProjectRoot, inputRoot)}
	if e = SnapshotTuningProject(ctx, cfg.ProjectRoot, p.ProjectDir, SnapshotLimits{Exclude: exclude}); e != nil {
		return p, e
	}
	setting := request.SettingFile
	if setting == "" {
		setting = cfg.File.Pahcer.SettingFile
	}
	setting, e = cfg.ResolveProjectPath("setting", setting)
	if e != nil {
		return p, e
	}
	if e = copyRegularFile(setting, p.SettingFile, 0644); e != nil {
		return p, e
	}
	for _, input := range inputs {
		if e = ctx.Err(); e != nil {
			return p, e
		}
		if e = copyRegularFile(input.Path, filepath.Join(p.InputDir, filepath.Base(input.Path)), 0644); e != nil {
			return p, e
		}
	}
	p.Inputs, e = cases.Discover(p.InputDir)
	return p, e
}

func copyRegularFile(source, destination string, mode os.FileMode) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, b, mode)
}

func copyPreparedTree(ctx context.Context, source, destination string) error {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	relDestination, err := filepath.Rel(absSource, absDestination)
	if err != nil {
		return err
	}
	if relDestination == "." || filepath.IsLocal(relDestination) {
		return fmt.Errorf("コピー先はコピー元の外にしてください")
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("toolsではsymlinkを使用できません: %s", rel)
		}
		dst := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("toolsには通常ファイルだけを使用できます: %s", rel)
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(out, src)
		srcErr := src.Close()
		dstErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if srcErr != nil {
			return srcErr
		}
		return dstErr
	})
}
