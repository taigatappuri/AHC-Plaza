package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReleaseAssetName(t *testing.T) {
	for _, test := range []struct {
		name    string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{name: "amd64", goos: "linux", goarch: "amd64", want: "ahc-plaza-linux-amd64"},
		{name: "arm64", goos: "linux", goarch: "arm64", want: "ahc-plaza-linux-arm64"},
		{name: "未対応OS", goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "未対応CPU", goos: "linux", goarch: "riscv64", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := releaseAssetName(test.goos, test.goarch)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("asset = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveExecutablePathFollowsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "releases", "ahc-plaza")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestExecutable(t, target, []byte("binary"))
	link := filepath.Join(directory, "ahc-plaza")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got, err := resolveExecutablePath(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("resolved path = %q, want %q", got, target)
	}
}

func TestUpdateExecutableReplacesCurrentExecutable(t *testing.T) {
	assetName := "ahc-plaza-linux-amd64"
	downloaded := []byte("#!/bin/sh\nprintf '0.3.0\\n'\n")
	server, assetDownloads := newUpdateTestServer(t, "v0.3.0", assetName, downloaded, "")
	defer server.Close()

	directory := t.TempDir()
	target := filepath.Join(directory, "ahc-plaza")
	original := []byte("#!/bin/sh\nprintf '0.2.0\\n'\n")
	writeTestExecutable(t, target, original)

	result, err := updateExecutable(context.Background(), updateOptions{
		CurrentVersion:   "0.2.0",
		ExecutablePath:   target,
		AssetName:        assetName,
		LatestReleaseURL: server.URL + "/latest",
		Client:           server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.PreviousVersion != "0.2.0" || result.LatestVersion != "0.3.0" {
		t.Fatalf("result = %#v", result)
	}
	if assetDownloads.Load() != 2 {
		t.Fatalf("asset downloads = %d, want 2", assetDownloads.Load())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(downloaded) {
		t.Fatalf("updated executable = %q, want %q", got, downloaded)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 755", info.Mode().Perm())
	}
	gotVersion, err := executableVersion(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if gotVersion != "0.3.0" {
		t.Fatalf("version = %q, want 0.3.0", gotVersion)
	}
	assertNoUpdateFiles(t, directory)
}

func TestUpdateExecutableSkipsDownloadWhenAlreadyLatest(t *testing.T) {
	assetName := "ahc-plaza-linux-amd64"
	downloaded := []byte("#!/bin/sh\nprintf '0.3.0\\n'\n")
	server, assetDownloads := newUpdateTestServer(t, "v0.3.0", assetName, downloaded, "")
	defer server.Close()

	directory := t.TempDir()
	target := filepath.Join(directory, "ahc-plaza")
	original := []byte("#!/bin/sh\nprintf '0.3.0\\n'\n")
	writeTestExecutable(t, target, original)

	result, err := updateExecutable(context.Background(), updateOptions{
		CurrentVersion:   "0.3.0",
		ExecutablePath:   target,
		AssetName:        assetName,
		LatestReleaseURL: server.URL + "/latest",
		Client:           server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated {
		t.Fatalf("result = %#v", result)
	}
	if assetDownloads.Load() != 0 {
		t.Fatalf("asset downloads = %d, want 0", assetDownloads.Load())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("最新バージョンの実行ファイルが変更されました")
	}
}

func TestUpdateExecutablePreservesCurrentExecutableOnChecksumFailure(t *testing.T) {
	assetName := "ahc-plaza-linux-amd64"
	downloaded := []byte("#!/bin/sh\nprintf '0.3.0\\n'\n")
	invalidChecksum := strings.Repeat("0", sha256.Size*2) + "  " + assetName + "\n"
	server, _ := newUpdateTestServer(t, "v0.3.0", assetName, downloaded, invalidChecksum)
	defer server.Close()

	directory := t.TempDir()
	target := filepath.Join(directory, "ahc-plaza")
	original := []byte("current executable")
	writeTestExecutable(t, target, original)

	_, err := updateExecutable(context.Background(), updateOptions{
		CurrentVersion:   "0.2.0",
		ExecutablePath:   target,
		AssetName:        assetName,
		LatestReleaseURL: server.URL + "/latest",
		Client:           server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "checksum verification failed") {
		t.Fatalf("error = %v", err)
	}
	assertExecutableUnchanged(t, target, original)
	assertNoUpdateFiles(t, directory)
}

func TestUpdateExecutablePreservesCurrentExecutableOnVersionMismatch(t *testing.T) {
	assetName := "ahc-plaza-linux-amd64"
	downloaded := []byte("#!/bin/sh\nprintf '0.4.0\\n'\n")
	server, _ := newUpdateTestServer(t, "v0.3.0", assetName, downloaded, "")
	defer server.Close()

	directory := t.TempDir()
	target := filepath.Join(directory, "ahc-plaza")
	original := []byte("current executable")
	writeTestExecutable(t, target, original)

	_, err := updateExecutable(context.Background(), updateOptions{
		CurrentVersion:   "0.2.0",
		ExecutablePath:   target,
		AssetName:        assetName,
		LatestReleaseURL: server.URL + "/latest",
		Client:           server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "does not match release tag") {
		t.Fatalf("error = %v", err)
	}
	assertExecutableUnchanged(t, target, original)
	assertNoUpdateFiles(t, directory)
}

func TestChecksumForAsset(t *testing.T) {
	digestBytes := sha256.Sum256([]byte("release"))
	digest := hex.EncodeToString(digestBytes[:])
	assetName := "ahc-plaza-linux-amd64"
	for _, test := range []struct {
		name      string
		checksums string
		want      string
		wantErr   bool
	}{
		{name: "通常形式", checksums: digest + "  " + assetName + "\n", want: digest},
		{name: "バイナリ形式と相対パス", checksums: digest + " *./" + assetName + "\n", want: digest},
		{name: "対象なし", checksums: digest + "  another-file\n", wantErr: true},
		{name: "不正なハッシュ", checksums: "invalid  " + assetName + "\n", wantErr: true},
		{name: "重複", checksums: digest + "  " + assetName + "\n" + digest + "  " + assetName + "\n", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := checksumForAsset([]byte(test.checksums), assetName)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("checksum = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFindReleaseAsset(t *testing.T) {
	valid := releaseAsset{Name: "binary", BrowserDownloadURL: "https://example.com/binary"}
	for _, test := range []struct {
		name    string
		assets  []releaseAsset
		wantErr bool
	}{
		{name: "対象あり", assets: []releaseAsset{valid}},
		{name: "対象なし", assets: []releaseAsset{{Name: "another", BrowserDownloadURL: "https://example.com/another"}}, wantErr: true},
		{name: "重複", assets: []releaseAsset{valid, valid}, wantErr: true},
		{name: "相対URL", assets: []releaseAsset{{Name: "binary", BrowserDownloadURL: "/binary"}}, wantErr: true},
		{name: "認証情報付きURL", assets: []releaseAsset{{Name: "binary", BrowserDownloadURL: "https://user@example.com/binary"}}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := findReleaseAsset(test.assets, "binary")
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
		})
	}
}

func TestUpdateCommandRejectsArguments(t *testing.T) {
	var commandErr *commandError
	err := execute([]string{"update", "unexpected"})
	if !errors.As(err, &commandErr) || commandErr.Code != 2 {
		t.Fatalf("error = %#v, want exit code 2", err)
	}
}

func newUpdateTestServer(t *testing.T, tagName, assetName string, binary []byte, checksums string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	if checksums == "" {
		digest := sha256.Sum256(binary)
		checksums = fmt.Sprintf("%x  %s\n", digest, assetName)
	}
	var assetDownloads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			if got := request.Header.Get("Accept"); got != "application/vnd.github+json" {
				t.Errorf("Accept = %q", got)
			}
			if got := request.Header.Get("X-GitHub-Api-Version"); got != githubAPIVersion {
				t.Errorf("X-GitHub-Api-Version = %q", got)
			}
			if got := request.Header.Get("User-Agent"); got != "AHC-Plaza/0.2.0" && got != "AHC-Plaza/0.3.0" {
				t.Errorf("User-Agent = %q", got)
			}
			response.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(response).Encode(latestRelease{
				TagName: tagName,
				Assets: []releaseAsset{
					{Name: assetName, BrowserDownloadURL: "http://" + request.Host + "/binary"},
					{Name: "checksums.txt", BrowserDownloadURL: "http://" + request.Host + "/checksums"},
				},
			})
			if err != nil {
				t.Errorf("release response: %v", err)
			}
		case "/binary":
			assetDownloads.Add(1)
			_, _ = response.Write(binary)
		case "/checksums":
			assetDownloads.Add(1)
			_, _ = response.Write([]byte(checksums))
		default:
			http.NotFound(response, request)
		}
	}))
	return server, &assetDownloads
}

func writeTestExecutable(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertExecutableUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("current executable = %q, want %q", got, want)
	}
}

func assertNoUpdateFiles(t *testing.T, directory string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".ahc-plaza-update-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary update files remain: %v", matches)
	}
}
