package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Info struct {
	Available       bool   `json:"available"`
	Ready           bool   `json:"ready"`
	Path            string `json:"path"`
	SHA256          string `json:"sha256"`
	Python          string `json:"python"`
	Optuna          string `json:"optuna"`
	CompressedBytes int64  `json:"compressed_bytes"`
	UnpackedBytes   int64  `json:"unpacked_bytes"`
}

var setupMu sync.Mutex

func Status(root string) Info {
	var info Info
	if len(metadata) == 0 || json.Unmarshal(metadata, &info) != nil {
		return info
	}
	info.Available = len(bundle) > 0
	info.Path = filepath.Join(root, "ahc-plaza", "runtime", info.SHA256)
	b, e := os.ReadFile(filepath.Join(info.Path, ".ready"))
	info.Ready = e == nil && string(b) == info.SHA256
	return info
}
func Setup(ctx context.Context, root string) (Info, error) {
	setupMu.Lock()
	defer setupMu.Unlock()
	info := Status(root)
	if !info.Available {
		return info, fmt.Errorf("この開発ビルドにはPython環境が同梱されていません。make buildで同梱ビルドを作成してください")
	}
	if info.Ready {
		return info, nil
	}
	if fmt.Sprintf("%x", sha256.Sum256(bundle)) != info.SHA256 {
		return info, fmt.Errorf("同梱環境のハッシュが一致しません")
	}
	parent := filepath.Dir(info.Path)
	if e := os.MkdirAll(parent, 0700); e != nil {
		return info, e
	}
	tmp, e := os.MkdirTemp(parent, ".extract-")
	if e != nil {
		return info, e
	}
	defer os.RemoveAll(tmp)
	if e = extract(ctx, bundle, tmp); e != nil {
		return info, e
	}
	if e = os.WriteFile(filepath.Join(tmp, ".ready"), []byte(info.SHA256), 0600); e != nil {
		return info, e
	}
	if _, e = os.Lstat(info.Path); e == nil {
		return info, fmt.Errorf("同梱環境の展開が不完全です。tune clean-runtime後に再実行してください")
	}
	if e = os.Rename(tmp, info.Path); e != nil {
		return info, e
	}
	info.Ready = true
	return info, nil
}
func extract(ctx context.Context, data []byte, directory string) error {
	gz, e := gzip.NewReader(bytes.NewReader(data))
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	total := int64(0)
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		h, e := tr.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		name := filepath.Clean(h.Name)
		if !filepath.IsLocal(name) || strings.Contains(h.Name, "\\") || h.Size < 0 {
			return fmt.Errorf("展開パスが不正です: %s", h.Name)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return fmt.Errorf("同梱環境には通常ファイルのみ許可します")
		}
		total += h.Size
		if total > 512<<20 {
			return fmt.Errorf("展開容量の上限を超えました")
		}
		path := filepath.Join(directory, name)
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		mode := os.FileMode(0600)
		if h.Mode&0111 != 0 {
			mode = 0700
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if e != nil {
			return e
		}
		_, copyErr := io.CopyN(f, tr, h.Size)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}

// Clean はプロジェクト所有ロック取得後、workerが停止した状態で呼びます。
func Clean(root string) error {
	setupMu.Lock()
	defer setupMu.Unlock()
	return os.RemoveAll(filepath.Join(root, "ahc-plaza", "runtime"))
}
