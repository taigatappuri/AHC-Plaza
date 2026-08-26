package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadAppliesDefaults(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	content := `[project]
problem = "ahc000"
objective = "max"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.File.Execution.DefaultInputDir != "ahc-plaza/inputs" {
		t.Fatalf("input dir = %q, want ahc-plaza/inputs", config.File.Execution.DefaultInputDir)
	}
	if config.File.Execution.TimeoutMilliseconds != 300000 {
		t.Fatalf("timeout = %d, want 300000", config.File.Execution.TimeoutMilliseconds)
	}
	if config.File.Statistics.ConfidenceLevel != 0.95 {
		t.Fatalf("confidence = %v, want 0.95", config.File.Statistics.ConfidenceLevel)
	}
	if config.File.Pahcer.SettingFile != "pahcer_config.toml" {
		t.Fatalf("pahcer config = %#v", config.File.Pahcer)
	}
	if config.File.InputFormat.Variables == nil || len(config.File.InputFormat.Variables) != 0 {
		t.Fatalf("input format = %#v, want empty variables", config.File.InputFormat)
	}
	if config.File.InputFormat.Features == nil || len(config.File.InputFormat.Features) != 0 {
		t.Fatalf("input features = %#v, want empty features", config.File.InputFormat)
	}
}

func TestLoadMigratesLegacyTimeoutSeconds(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	content := `[project]
problem = "ahc000"
objective = "max"

[execution]
timeout_seconds = 2
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.File.Execution.TimeoutMilliseconds != 2000 {
		t.Fatalf("timeout = %d, want 2000", loaded.File.Execution.TimeoutMilliseconds)
	}
	if _, err := Save(path, loaded.File); err != nil {
		t.Fatal(err)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migrated), "timeout_ms = 2000") || strings.Contains(string(migrated), "timeout_seconds") {
		t.Fatalf("旧設定がミリ秒へ移行されていません: %s", migrated)
	}
}

func TestLoadReadsInputFeatures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	content := `[project]
problem = "ahc000"
objective = "max"

[[input_format.variables]]
name = "N"
line = 1
column = 1

[[input_format.features]]
name = " average "
source = "features/average.cpp"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []InputFeatureConfig{{Name: "average", Source: "features/average.cpp", TimeoutMilliseconds: 2000}}
	if !reflect.DeepEqual(loaded.File.InputFormat.Features, want) {
		t.Fatalf("features = %#v, want %#v", loaded.File.InputFormat.Features, want)
	}
}

func TestLoadRejectsInvalidInputFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features string
	}{
		{name: "変数名との重複", features: `[[input_format.variables]]
name = "N"
line = 1
column = 1
[[input_format.features]]
name = "N"
source = "features/n.cpp"`},
		{name: "ディレクトリ外", features: `[[input_format.features]]
name = "average"
source = "solver/average.cpp"`},
		{name: "C++以外", features: `[[input_format.features]]
name = "average"
source = "features/average.py"`},
		{name: "タイムアウト超過", features: `[[input_format.features]]
name = "average"
source = "features/average.cpp"
timeout_ms = 60001`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "ahc-plaza.toml")
			content := `[project]
problem = "ahc000"
objective = "max"
` + test.features + "\n"
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("不正な入力特徴量を読み込みました")
			}
		})
	}
}

func TestLoadReadsInputFormatVariables(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	content := `[project]
problem = "ahc000"
objective = "max"

[[input_format.variables]]
name = " N "
line = 1
column = 1

[[input_format.variables]]
name = "M"
line = 1
column = 2
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []InputVariableConfig{
		{Name: "N", Line: 1, Column: 1},
		{Name: "M", Line: 1, Column: 2},
	}
	if !reflect.DeepEqual(loaded.File.InputFormat.Variables, want) {
		t.Fatalf("variables = %#v, want %#v", loaded.File.InputFormat.Variables, want)
	}
}

func TestLoadRejectsInvalidInputFormatVariables(t *testing.T) {
	tests := []struct {
		name      string
		variables string
	}{
		{
			name: "空の変数名",
			variables: `[[input_format.variables]]
name = ""
line = 1
column = 1`,
		},
		{
			name: "0行目",
			variables: `[[input_format.variables]]
name = "N"
line = 0
column = 1`,
		},
		{
			name: "0列目",
			variables: `[[input_format.variables]]
name = "N"
line = 1
column = 0`,
		},
		{
			name: "変数名の重複",
			variables: `[[input_format.variables]]
name = "N"
line = 1
column = 1
[[input_format.variables]]
name = "N"
line = 1
column = 2`,
		},
		{
			name: "位置の重複",
			variables: `[[input_format.variables]]
name = "N"
line = 1
column = 1
[[input_format.variables]]
name = "M"
line = 1
column = 1`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "ahc-plaza.toml")
			content := `[project]
problem = "ahc000"
objective = "max"
` + test.variables + "\n"
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("不正な入力形式を受け入れました")
			}
		})
	}
}

func TestInputSetDirOnlyAllowsDirectChildOfInputRoot(t *testing.T) {
	root := t.TempDir()
	pathRoot := filepath.Join(root, "ahc-plaza")
	c := Config{ProjectRoot: root, PathRoot: pathRoot, File: FileConfig{Execution: ExecutionConfig{DefaultInputDir: "ahc-plaza/inputs"}}}

	got, err := c.InputSetDir("ahc-plaza/inputs/baseline")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(pathRoot, "inputs", "baseline") {
		t.Fatalf("input set dir = %q", got)
	}
	for _, path := range []string{"", "ahc-plaza/inputs", "ahc-plaza/inputs/nested/cases", "tools/in"} {
		if _, err := c.InputSetDir(path); err == nil {
			t.Fatalf("入力セットとして受け入れました: %q", path)
		}
	}
}

func TestLoadRejectsProjectEscape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	content := `[project]
problem = "ahc000"
objective = "max"

[execution]
default_input_dir = "../outside"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("プロジェクト外のパスを受け入れました")
	}
}

func TestLoadRejectsInvalidObjective(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	content := `[project]
problem = "ahc000"
objective = "invalid"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("不正なobjectiveを受け入れました")
	}
}

func TestSaveWritesValidatedConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	written, err := Save(path, FileConfig{
		Project:    ProjectConfig{Problem: "ahc999", Objective: "min"},
		Paths:      PathsConfig{SolverDir: "src", ToolsDir: "tester"},
		Execution:  ExecutionConfig{DefaultInputDir: "tester/in", Threads: 8, TimeoutMilliseconds: 15000},
		Pahcer:     PahcerConfig{SettingFile: "pahcer_config.toml"},
		Score:      ScoreConfig{InvalidScore: -1, IncludeInvalidCases: false},
		Statistics: StatisticsConfig{ConfidenceLevel: 0.99, BootstrapIterations: 20000},
		InputFormat: InputFormatConfig{Variables: []InputVariableConfig{
			{Name: "N", Line: 1, Column: 1},
			{Name: "M", Line: 1, Column: 2},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if written.File.Project.Problem != "ahc999" || written.File.Execution.Threads != 8 {
		t.Fatalf("保存結果 = %#v", written.File)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.File, written.File) {
		t.Fatalf("再読込結果 = %#v, 保存結果 = %#v", loaded.File, written.File)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "[project]") ||
		!strings.Contains(string(content), "problem = \"ahc999\"") ||
		!strings.Contains(string(content), "timeout_ms = 15000") ||
		strings.Contains(string(content), "timeout_seconds") ||
		!strings.Contains(string(content), "[[input_format.variables]]") ||
		!strings.Contains(string(content), "name = \"N\"") {
		t.Fatalf("保存内容 = %s", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
}

func TestSaveDoesNotReplaceFileWhenValidationFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ahc-plaza.toml")
	original := []byte("original content\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Save(path, FileConfig{
		Project: ProjectConfig{Problem: "ahc999", Objective: "invalid"},
	})
	if err == nil {
		t.Fatal("不正な設定を保存しました")
	}
	current, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(current) != string(original) {
		t.Fatalf("検証失敗後の内容 = %q, want %q", current, original)
	}
}
