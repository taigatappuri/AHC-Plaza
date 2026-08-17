package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

type FileConfig struct {
	Project     ProjectConfig     `toml:"project" json:"project"`
	Paths       PathsConfig       `toml:"paths" json:"paths"`
	Execution   ExecutionConfig   `toml:"execution" json:"execution"`
	Pahcer      PahcerConfig      `toml:"pahcer" json:"pahcer"`
	Score       ScoreConfig       `toml:"score" json:"score"`
	Statistics  StatisticsConfig  `toml:"statistics" json:"statistics"`
	InputFormat InputFormatConfig `toml:"input_format" json:"input_format"`
}

type ProjectConfig struct {
	Problem   string `toml:"problem" json:"problem"`
	Objective string `toml:"objective" json:"objective"`
}

type PathsConfig struct {
	SolverDir string `toml:"solver_dir" json:"solver_dir"`
	ToolsDir  string `toml:"tools_dir" json:"tools_dir"`
}

type ExecutionConfig struct {
	DefaultInputDir string `toml:"default_input_dir" json:"default_input_dir"`
	Threads         int    `toml:"threads" json:"threads"`
	TimeoutSeconds  int    `toml:"timeout_seconds" json:"timeout_seconds"`
}

type PahcerConfig struct {
	SettingFile string `toml:"setting_file" json:"setting_file"`
}

type ScoreConfig struct {
	InvalidScore        float64 `toml:"invalid_score" json:"invalid_score"`
	IncludeInvalidCases bool    `toml:"include_invalid_cases" json:"include_invalid_cases"`
}

type StatisticsConfig struct {
	ConfidenceLevel     float64 `toml:"confidence_level" json:"confidence_level"`
	BootstrapIterations int     `toml:"bootstrap_iterations" json:"bootstrap_iterations"`
}

type InputFormatConfig struct {
	Variables []InputVariableConfig `toml:"variables" json:"variables"`
	Features  []InputFeatureConfig  `toml:"features" json:"features"`
}

type InputVariableConfig struct {
	Name   string `toml:"name" json:"name"`
	Line   int    `toml:"line" json:"line"`
	Column int    `toml:"column" json:"column"`
}

type InputFeatureConfig struct {
	Name                string `toml:"name" json:"name"`
	Source              string `toml:"source" json:"source"`
	TimeoutMilliseconds int    `toml:"timeout_ms" json:"timeout_ms"`
}

type Config struct {
	FilePath    string
	ProjectRoot string
	PathRoot    string
	File        FileConfig
}

func Load(path string) (Config, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("could not resolve the configuration file path: %w", err)
	}

	var file FileConfig
	if _, err := toml.DecodeFile(absolutePath, &file); err != nil {
		return Config{}, fmt.Errorf("could not read the configuration file: %w", err)
	}
	applyDefaults(&file)

	projectRoot := filepath.Dir(absolutePath)
	config := Config{FilePath: absolutePath, ProjectRoot: projectRoot, PathRoot: filepath.Join(projectRoot, "ahc-plaza"), File: file}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Save は設定を検証して保存します。
func Save(path string, file FileConfig) (Config, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("could not resolve the configuration file path: %w", err)
	}

	applyDefaults(&file)
	projectRoot := filepath.Dir(absolutePath)
	config := Config{FilePath: absolutePath, ProjectRoot: projectRoot, PathRoot: filepath.Join(projectRoot, "ahc-plaza"), File: file}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	output, err := os.OpenFile(absolutePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return Config{}, fmt.Errorf("could not open the configuration file: %w", err)
	}
	if err := toml.NewEncoder(output).Encode(file); err != nil {
		output.Close()
		return Config{}, fmt.Errorf("could not write the configuration file: %w", err)
	}
	if err := output.Close(); err != nil {
		return Config{}, fmt.Errorf("could not close the configuration file: %w", err)
	}
	return config, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.File.Project.Problem) == "" {
		return errors.New("project.problem is required")
	}
	if c.File.Project.Objective != "max" && c.File.Project.Objective != "min" {
		return fmt.Errorf("project.objective must be either max or min: %q", c.File.Project.Objective)
	}
	if c.File.Execution.Threads < 0 {
		return errors.New("execution.threads must be 0 or greater")
	}
	if c.File.Execution.TimeoutSeconds <= 0 {
		return errors.New("execution.timeout_seconds must be 1 or greater")
	}
	if _, err := c.ResolveProjectPath("pahcer.setting_file", c.File.Pahcer.SettingFile); err != nil {
		return err
	}
	if c.File.Statistics.ConfidenceLevel <= 0 || c.File.Statistics.ConfidenceLevel >= 1 {
		return errors.New("statistics.confidence_level must be greater than 0 and less than 1")
	}
	if c.File.Statistics.BootstrapIterations <= 0 {
		return errors.New("statistics.bootstrap_iterations must be 1 or greater")
	}
	if len(c.File.InputFormat.Variables) > 256 {
		return errors.New("input_format.variables must contain no more than 256 entries")
	}
	if len(c.File.InputFormat.Features) > 32 {
		return errors.New("input_format.features must contain no more than 32 entries")
	}

	names := make(map[string]struct{}, len(c.File.InputFormat.Variables)+len(c.File.InputFormat.Features))
	positions := make(map[[2]int]struct{}, len(c.File.InputFormat.Variables))
	for index, variable := range c.File.InputFormat.Variables {
		if variable.Name == "" {
			return fmt.Errorf("input_format.variables[%d].name is required", index)
		}
		if utf8.RuneCountInString(variable.Name) > 64 || strings.ContainsAny(variable.Name, "\r\n\t") {
			return fmt.Errorf("input_format.variables[%d].name must be a single line of no more than 64 characters", index)
		}
		if variable.Line <= 0 {
			return fmt.Errorf("input_format.variables[%d].line must be 1 or greater", index)
		}
		if variable.Column <= 0 {
			return fmt.Errorf("input_format.variables[%d].column must be 1 or greater", index)
		}
		if _, exists := names[variable.Name]; exists {
			return fmt.Errorf("duplicate input_format.variables name: %q", variable.Name)
		}
		names[variable.Name] = struct{}{}
		position := [2]int{variable.Line, variable.Column}
		if _, exists := positions[position]; exists {
			return fmt.Errorf("duplicate input_format.variables position: line %d, column %d", variable.Line, variable.Column)
		}
		positions[position] = struct{}{}
	}

	featureSources := make(map[string]struct{}, len(c.File.InputFormat.Features))
	featureRoot := filepath.Join(c.PathRoot, "features")
	for index, feature := range c.File.InputFormat.Features {
		if feature.Name == "" {
			return fmt.Errorf("input_format.features[%d].name is required", index)
		}
		if utf8.RuneCountInString(feature.Name) > 64 || strings.ContainsAny(feature.Name, "\r\n\t") {
			return fmt.Errorf("input_format.features[%d].name must be a single line of no more than 64 characters", index)
		}
		if _, exists := names[feature.Name]; exists {
			return fmt.Errorf("duplicate input feature name: %q", feature.Name)
		}
		names[feature.Name] = struct{}{}
		sourcePath, err := c.ResolvePath(fmt.Sprintf("input_format.features[%d].source", index), feature.Source)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(featureRoot, sourcePath)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("input_format.features[%d].source must be under features", index)
		}
		if strings.ToLower(filepath.Ext(sourcePath)) != ".cpp" {
			return fmt.Errorf("input_format.features[%d].source must be a .cpp file", index)
		}
		normalizedSource := filepath.ToSlash(sourcePath)
		if _, exists := featureSources[normalizedSource]; exists {
			return fmt.Errorf("duplicate input_format.features source: %q", normalizedSource)
		}
		featureSources[normalizedSource] = struct{}{}
		if feature.TimeoutMilliseconds < 1 || feature.TimeoutMilliseconds > 60000 {
			return fmt.Errorf("input_format.features[%d].timeout_ms must be between 1 and 60000", index)
		}
	}

	for name, path := range map[string]string{
		"paths.solver_dir":            c.File.Paths.SolverDir,
		"paths.tools_dir":             c.File.Paths.ToolsDir,
		"execution.default_input_dir": c.File.Execution.DefaultInputDir,
	} {
		if _, err := c.ResolveProjectPath(name, path); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) InputDir(relativeOverride string) (string, error) {
	relative := relativeOverride
	if relative == "" {
		relative = c.File.Execution.DefaultInputDir
	}
	return c.ResolveProjectPath("input_dir", relative)
}

// InputSetDir は入力セットとして扱える、入力ルート直下のディレクトリを返します。
func (c Config) InputSetDir(relativeOverride string) (string, error) {
	inputRoot, err := c.InputDir("")
	if err != nil {
		return "", err
	}
	inputDir, err := c.InputDir(relativeOverride)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(inputRoot, inputDir)
	if err != nil {
		return "", fmt.Errorf("could not resolve the input set path: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || strings.Contains(relative, string(os.PathSeparator)) {
		return "", errors.New("the input set must be a direct child directory of the configured input directory")
	}
	return inputDir, nil
}

func IsWithin(root, target string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)), nil
}

func applyDefaults(file *FileConfig) {
	setDefault(&file.Paths.SolverDir, "solver")
	setDefault(&file.Paths.ToolsDir, "tools")
	setDefault(&file.Execution.DefaultInputDir, "ahc-plaza/inputs")
	setDefault(&file.Execution.TimeoutSeconds, 300)
	setDefault(&file.Pahcer.SettingFile, "pahcer_config.toml")
	setDefault(&file.Statistics.ConfidenceLevel, 0.95)
	setDefault(&file.Statistics.BootstrapIterations, 10000)
	if file.InputFormat.Variables == nil {
		file.InputFormat.Variables = []InputVariableConfig{}
	}
	if file.InputFormat.Features == nil {
		file.InputFormat.Features = []InputFeatureConfig{}
	}
	for index := range file.InputFormat.Variables {
		file.InputFormat.Variables[index].Name = strings.TrimSpace(file.InputFormat.Variables[index].Name)
	}
	for index := range file.InputFormat.Features {
		file.InputFormat.Features[index].Name = strings.TrimSpace(file.InputFormat.Features[index].Name)
		file.InputFormat.Features[index].Source = strings.TrimSpace(file.InputFormat.Features[index].Source)
		if file.InputFormat.Features[index].Source != "" {
			file.InputFormat.Features[index].Source = filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.InputFormat.Features[index].Source)))
		}
		if file.InputFormat.Features[index].TimeoutMilliseconds == 0 {
			file.InputFormat.Features[index].TimeoutMilliseconds = 2000
		}
	}
}

func setDefault[T comparable](value *T, fallback T) {
	var zero T
	if *value == zero {
		*value = fallback
	}
}

// ResolvePath はahc-plazaディレクトリ基準の設定パスを解決します。
func (c Config) ResolvePath(name, path string) (string, error) {
	return resolvePath(c.ProjectRoot, c.PathRoot, name, path, "ahc-plaza")
}

// ResolveProjectPath はプロジェクトディレクトリ基準の設定パスを解決します。
func (c Config) ResolveProjectPath(name, path string) (string, error) {
	return resolvePath(c.ProjectRoot, c.ProjectRoot, name, path, "the project")
}

func resolvePath(projectRoot, base, name, path, baseName string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	if filepath.IsAbs(filepath.FromSlash(path)) {
		return "", fmt.Errorf("%s must be relative to %s: %q", name, baseName, path)
	}
	target := filepath.Join(base, filepath.Clean(filepath.FromSlash(path)))
	within, err := IsWithin(projectRoot, target)
	if err != nil {
		return "", err
	}
	if !within {
		return "", fmt.Errorf("%s must be inside the project: %q", name, path)
	}
	return target, nil
}
