package cases

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func Discover(inputDir string) ([]domain.InputCase, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, fmt.Errorf("could not enumerate input cases: %w", err)
	}

	result := make([]domain.InputCase, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || entry.IsDir() {
			continue
		}
		path := filepath.Join(inputDir, entry.Name())
		entryInfo, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("could not inspect input case: %s: %w", path, err)
		}
		if !entryInfo.Mode().IsRegular() || entryInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		hash, err := fileHash(path)
		if err != nil {
			return nil, fmt.Errorf("could not hash input case: %s: %w", path, err)
		}
		result = append(result, domain.InputCase{
			ID:     entry.Name(),
			Path:   path,
			SHA256: hash,
			Size:   entryInfo.Size(),
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func HashList(inputCases []domain.InputCase) string {
	hasher := sha256.New()
	for _, inputCase := range inputCases {
		fmt.Fprintf(hasher, "%s\x00%s\n", inputCase.ID, inputCase.SHA256)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
