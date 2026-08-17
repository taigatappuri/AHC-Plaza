// Package inputfeature は入力ファイルから派生特徴量を計算するC++プログラムを管理します。
package inputfeature

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxSourceBytes = 512 << 10
	maxOutputBytes = 64 << 10
	compileTimeout = 30 * time.Second
)

// Runner はコンパイル済みバイナリと入力ごとの計算結果を再利用します。
type Runner struct {
	root        string
	featureRoot string
	cacheDir    string

	compileMu sync.Mutex
	cacheMu   sync.RWMutex
	values    map[string]float64
}

// Program はコンパイル済みの派生特徴量です。
type Program struct {
	runner     *Runner
	binaryPath string
	sourceHash string
	timeout    time.Duration
}

func NewRunner(root string) (*Runner, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("could not resolve the project root: %w", err)
	}
	return &Runner{
		root:        absoluteRoot,
		featureRoot: filepath.Join(absoluteRoot, "ahc-plaza", "features"),
		cacheDir:    filepath.Join(absoluteRoot, "ahc-plaza", "cache", "features"),
		values:      make(map[string]float64),
	}, nil
}

// Prepare はC++ソースを検証してコンパイルします。戻り値のboolはキャッシュ利用を表します。
func (r *Runner) Prepare(ctx context.Context, source string, timeoutMilliseconds int) (Program, bool, error) {
	sourcePath, err := r.resolveSource(source)
	if err != nil {
		return Program{}, false, err
	}
	content, err := readSource(sourcePath)
	if err != nil {
		return Program{}, false, err
	}
	hash := sha256.Sum256(append([]byte("g++-std=c++20-O2-pipe\x00"), content...))
	sourceHash := hex.EncodeToString(hash[:])
	if timeoutMilliseconds <= 0 {
		timeoutMilliseconds = 2000
	}
	program := Program{
		runner:     r,
		binaryPath: filepath.Join(r.cacheDir, sourceHash),
		sourceHash: sourceHash,
		timeout:    time.Duration(timeoutMilliseconds) * time.Millisecond,
	}
	if regularFile(program.binaryPath) {
		return program, true, nil
	}

	r.compileMu.Lock()
	defer r.compileMu.Unlock()
	if regularFile(program.binaryPath) {
		return program, true, nil
	}
	if err := r.compile(ctx, content, program.binaryPath); err != nil {
		return Program{}, false, err
	}
	return program, false, nil
}

// EvaluateFile は入力ファイルを標準入力として実行し、標準出力の数値を返します。
func (p Program) EvaluateFile(ctx context.Context, inputPath string) (float64, time.Duration, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("入力ファイルを確認できません: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, 0, errors.New("入力パスが通常ファイルではありません")
	}
	cacheKey := strings.Join([]string{p.sourceHash, inputPath, strconv.FormatInt(info.Size(), 10), strconv.FormatInt(info.ModTime().UnixNano(), 10)}, "\x00")
	p.runner.cacheMu.RLock()
	cached, exists := p.runner.values[cacheKey]
	p.runner.cacheMu.RUnlock()
	if exists {
		return cached, 0, nil
	}

	input, err := os.Open(inputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("入力ファイルを開けません: %w", err)
	}
	defer input.Close()
	value, elapsed, err := p.evaluate(ctx, input)
	if err != nil {
		return 0, elapsed, err
	}
	p.runner.cacheMu.Lock()
	p.runner.values[cacheKey] = value
	p.runner.cacheMu.Unlock()
	return value, elapsed, nil
}

func (r *Runner) resolveSource(source string) (string, error) {
	if strings.TrimSpace(source) == "" || filepath.IsAbs(filepath.FromSlash(source)) {
		return "", errors.New("特徴量ソースはfeatures配下の相対パスで指定してください")
	}
	target := filepath.Join(r.root, "ahc-plaza", filepath.FromSlash(source))
	relative, err := filepath.Rel(r.featureRoot, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("特徴量ソースはfeatures配下にしてください")
	}
	if strings.ToLower(filepath.Ext(target)) != ".cpp" {
		return "", errors.New("特徴量ソースは.cppファイルにしてください")
	}
	info, err := os.Lstat(target)
	if err != nil {
		return "", fmt.Errorf("特徴量ソースを確認できません: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("特徴量ソースは通常のC++ファイルにしてください")
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("特徴量ソースを解決できません: %w", err)
	}
	resolvedRelative, err := filepath.Rel(r.featureRoot, resolved)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(os.PathSeparator)) {
		return "", errors.New("特徴量ソースがfeaturesの外を参照しています")
	}
	return resolved, nil
}

func readSource(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("特徴量ソースを読めません: %w", err)
	}
	if len(content) > maxSourceBytes {
		return nil, fmt.Errorf("特徴量ソースは%d KiB以下にしてください", maxSourceBytes>>10)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, errors.New("特徴量ソースが空です")
	}
	return content, nil
}

func (r *Runner) compile(ctx context.Context, source []byte, binaryPath string) error {
	compiler, err := exec.LookPath("g++")
	if err != nil {
		return errors.New("g++が見つかりません。C++コンパイラをインストールしてください")
	}
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return fmt.Errorf("特徴量キャッシュを作成できません: %w", err)
	}
	binaryFile, err := os.CreateTemp(r.cacheDir, "feature-bin-*")
	if err != nil {
		return fmt.Errorf("一時バイナリを作成できません: %w", err)
	}
	temporaryBinary := binaryFile.Name()
	_ = binaryFile.Close()
	defer os.Remove(temporaryBinary)

	compileContext, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()
	stderr := &cappedBuffer{limit: maxOutputBytes}
	command := exec.CommandContext(compileContext, compiler, "-x", "c++", "-std=c++20", "-O2", "-pipe", "-", "-o", temporaryBinary)
	command.Dir = r.root
	command.Stdin = bytes.NewReader(source)
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if errors.Is(compileContext.Err(), context.DeadlineExceeded) {
			return errors.New("特徴量のコンパイルが30秒でタイムアウトしました")
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("特徴量をコンパイルできません:\n%s", message)
	}
	if err := os.Rename(temporaryBinary, binaryPath); err != nil {
		return fmt.Errorf("特徴量バイナリをキャッシュできません: %w", err)
	}
	return nil
}

func (p Program) evaluate(ctx context.Context, input io.Reader) (float64, time.Duration, error) {
	runContext, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	stdout := &cappedBuffer{limit: maxOutputBytes}
	stderr := &cappedBuffer{limit: maxOutputBytes}
	command := exec.CommandContext(runContext, p.binaryPath)
	command.Dir = p.runner.root
	command.Stdin = input
	command.Stdout = stdout
	command.Stderr = stderr
	started := time.Now()
	err := command.Run()
	elapsed := time.Since(started)
	if err != nil {
		if errors.Is(runContext.Err(), context.DeadlineExceeded) {
			return 0, elapsed, fmt.Errorf("%dmsでタイムアウトしました", p.timeout.Milliseconds())
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return 0, elapsed, fmt.Errorf("実行に失敗しました: %s", message)
	}
	if stdout.truncated {
		return 0, elapsed, fmt.Errorf("標準出力が%d KiBを超えました", maxOutputBytes>>10)
	}
	fields := strings.Fields(stdout.String())
	if len(fields) != 1 {
		return 0, elapsed, fmt.Errorf("標準出力には数値を1個だけ出力してください（出力トークン数: %d）", len(fields))
	}
	value, parseErr := strconv.ParseFloat(fields[0], 64)
	if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, elapsed, fmt.Errorf("標準出力を有限な数値として読めません: %q", fields[0])
	}
	return value, elapsed, nil
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

type cappedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(content []byte) (int, error) {
	originalLength := len(content)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(content) > remaining {
			content = content[:remaining]
			b.truncated = true
		}
		_, _ = b.Buffer.Write(content)
	} else if originalLength > 0 {
		b.truncated = true
	}
	return originalLength, nil
}
