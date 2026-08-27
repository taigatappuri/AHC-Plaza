package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	latestReleaseAPIURL = "https://api.github.com/repos/taigatappuri/AHC-Plaza/releases/latest"
	githubAPIVersion    = "2026-03-10"
	maxReleaseJSONSize  = 2 << 20
	maxChecksumsSize    = 1 << 20
	maxBinarySize       = 256 << 20
)

var updateHTTPClient = &http.Client{Timeout: 5 * time.Minute}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type latestRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type updateOptions struct {
	CurrentVersion   string
	ExecutablePath   string
	AssetName        string
	LatestReleaseURL string
	Client           *http.Client
}

type updateResult struct {
	PreviousVersion string
	LatestVersion   string
	ExecutablePath  string
	Updated         bool
}

func executeUpdate(args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return &commandError{Code: 2, Err: err}
	}
	if flags.NArg() != 0 {
		return &commandError{Code: 2, Err: fmt.Errorf("update does not accept arguments")}
	}

	assetName, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine the executable path: %w", err)
	}
	executablePath, err = resolveExecutablePath(executablePath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := updateExecutable(ctx, updateOptions{
		CurrentVersion:   version,
		ExecutablePath:   executablePath,
		AssetName:        assetName,
		LatestReleaseURL: latestReleaseAPIURL,
		Client:           updateHTTPClient,
	})
	if err != nil {
		return err
	}
	if !result.Updated {
		fmt.Printf("AHC Plazaは最新です: %s\n", result.LatestVersion)
		return nil
	}
	fmt.Printf("AHC Plazaを更新しました: %s -> %s\n", result.PreviousVersion, result.LatestVersion)
	fmt.Printf("実行ファイル: %s\n", result.ExecutablePath)
	return nil
}

func resolveExecutablePath(executablePath string) (string, error) {
	absolutePath, err := filepath.Abs(executablePath)
	if err != nil {
		return "", fmt.Errorf("could not resolve the executable path: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", fmt.Errorf("could not resolve executable symlinks: %w", err)
	}
	return resolvedPath, nil
}

func releaseAssetName(goos, goarch string) (string, error) {
	if goos != "linux" {
		return "", fmt.Errorf("updates are not supported on %s", goos)
	}
	switch goarch {
	case "amd64":
		return "ahc-plaza-linux-amd64", nil
	case "arm64":
		return "ahc-plaza-linux-arm64", nil
	default:
		return "", fmt.Errorf("updates are not supported on linux/%s", goarch)
	}
}

func updateExecutable(ctx context.Context, options updateOptions) (updateResult, error) {
	if options.Client == nil {
		return updateResult{}, errors.New("HTTP client is required")
	}
	if options.ExecutablePath == "" {
		return updateResult{}, errors.New("executable path is required")
	}
	if options.AssetName == "" {
		return updateResult{}, errors.New("release asset name is required")
	}
	if options.LatestReleaseURL == "" {
		return updateResult{}, errors.New("latest release URL is required")
	}

	release, err := fetchLatestRelease(ctx, options.Client, options.LatestReleaseURL, options.CurrentVersion)
	if err != nil {
		return updateResult{}, err
	}
	latestVersion := normalizeVersion(release.TagName)
	if latestVersion == "" {
		return updateResult{}, errors.New("latest release tag is empty")
	}
	result := updateResult{
		PreviousVersion: normalizeVersion(options.CurrentVersion),
		LatestVersion:   latestVersion,
		ExecutablePath:  options.ExecutablePath,
	}
	if result.PreviousVersion == result.LatestVersion {
		return result, nil
	}

	binaryAsset, err := findReleaseAsset(release.Assets, options.AssetName)
	if err != nil {
		return updateResult{}, err
	}
	checksumsAsset, err := findReleaseAsset(release.Assets, "checksums.txt")
	if err != nil {
		return updateResult{}, err
	}
	checksums, err := downloadBytes(ctx, options.Client, checksumsAsset.BrowserDownloadURL, maxChecksumsSize, options.CurrentVersion)
	if err != nil {
		return updateResult{}, fmt.Errorf("could not download checksums: %w", err)
	}
	expectedChecksum, err := checksumForAsset(checksums, options.AssetName)
	if err != nil {
		return updateResult{}, err
	}

	info, err := os.Stat(options.ExecutablePath)
	if err != nil {
		return updateResult{}, fmt.Errorf("could not inspect the current executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return updateResult{}, fmt.Errorf("current executable is not a regular file: %s", options.ExecutablePath)
	}

	temporary, err := os.CreateTemp(filepath.Dir(options.ExecutablePath), ".ahc-plaza-update-*")
	if err != nil {
		return updateResult{}, fmt.Errorf("could not create an update file next to the executable: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	actualChecksum, err := downloadFile(ctx, options.Client, binaryAsset.BrowserDownloadURL, temporary, maxBinarySize, options.CurrentVersion)
	if err != nil {
		return updateResult{}, fmt.Errorf("could not download %s: %w", options.AssetName, err)
	}
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		return updateResult{}, fmt.Errorf("checksum verification failed for %s", options.AssetName)
	}
	if err := temporary.Chmod(0o755); err != nil {
		return updateResult{}, fmt.Errorf("could not make the downloaded executable runnable: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return updateResult{}, fmt.Errorf("could not flush the downloaded executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return updateResult{}, fmt.Errorf("could not close the downloaded executable: %w", err)
	}
	downloadedVersion, err := executableVersion(ctx, temporaryPath)
	if err != nil {
		return updateResult{}, err
	}
	if normalizeVersion(downloadedVersion) != latestVersion {
		return updateResult{}, fmt.Errorf("downloaded version %q does not match release tag %q", downloadedVersion, release.TagName)
	}
	if err := os.Rename(temporaryPath, options.ExecutablePath); err != nil {
		return updateResult{}, fmt.Errorf("could not replace the current executable: %w", err)
	}
	result.Updated = true
	return result, nil
}

func fetchLatestRelease(ctx context.Context, client *http.Client, releaseURL, currentVersion string) (latestRelease, error) {
	body, err := downloadBytes(ctx, client, releaseURL, maxReleaseJSONSize, currentVersion)
	if err != nil {
		return latestRelease{}, fmt.Errorf("could not get the latest release: %w", err)
	}
	var release latestRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return latestRelease{}, fmt.Errorf("could not decode the latest release: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return latestRelease{}, errors.New("latest release tag is missing")
	}
	return release, nil
}

func findReleaseAsset(assets []releaseAsset, name string) (releaseAsset, error) {
	var found *releaseAsset
	for index := range assets {
		if assets[index].Name != name {
			continue
		}
		if found != nil {
			return releaseAsset{}, fmt.Errorf("release asset is duplicated: %s", name)
		}
		found = &assets[index]
	}
	if found == nil {
		return releaseAsset{}, fmt.Errorf("release asset was not found: %s", name)
	}
	if err := validateDownloadURL(found.BrowserDownloadURL); err != nil {
		return releaseAsset{}, fmt.Errorf("invalid download URL for %s: %w", name, err)
	}
	return *found, nil
}

func validateDownloadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return errors.New("URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return errors.New("URL must not include credentials")
	}
	return nil
}

func downloadBytes(ctx context.Context, client *http.Client, downloadURL string, limit int64, currentVersion string) ([]byte, error) {
	response, err := get(ctx, client, downloadURL, currentVersion)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.ContentLength > limit {
		return nil, fmt.Errorf("response is larger than %d bytes", limit)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response is larger than %d bytes", limit)
	}
	return body, nil
}

func downloadFile(ctx context.Context, client *http.Client, downloadURL string, destination io.Writer, limit int64, currentVersion string) (string, error) {
	response, err := get(ctx, client, downloadURL, currentVersion)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.ContentLength > limit {
		return "", fmt.Errorf("response is larger than %d bytes", limit)
	}
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hasher), io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", err
	}
	if written > limit {
		return "", fmt.Errorf("response is larger than %d bytes", limit)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func get(ctx context.Context, client *http.Client, requestURL, currentVersion string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	request.Header.Set("User-Agent", "AHC-Plaza/"+normalizeVersion(currentVersion))
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("GET %s returned %s", requestURL, response.Status)
	}
	return response, nil
}

func checksumForAsset(checksums []byte, assetName string) (string, error) {
	var found string
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(fields[1], "*"), "./")
		if name != assetName {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("checksum is duplicated: %s", assetName)
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("checksum is invalid: %s", assetName)
		}
		found = strings.ToLower(fields[0])
	}
	if found == "" {
		return "", fmt.Errorf("checksum was not found: %s", assetName)
	}
	return found, nil
}

func executableVersion(ctx context.Context, executablePath string) (string, error) {
	verificationContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(verificationContext, executablePath, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not verify the downloaded executable: %w: %s", err, strings.TrimSpace(string(output)))
	}
	downloadedVersion := strings.TrimSpace(string(output))
	if downloadedVersion == "" {
		return "", errors.New("downloaded executable returned an empty version")
	}
	return downloadedVersion, nil
}

func normalizeVersion(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "v")
}
