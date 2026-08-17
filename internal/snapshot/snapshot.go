package snapshot

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/domain"
)

func Create(projectRoot, sourcePath, runDir string) (domain.SourceSnapshot, error) {
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return domain.SourceSnapshot{}, fmt.Errorf("could not resolve solver path: %w", err)
	}
	within, err := config.IsWithin(projectRoot, absSource)
	if err != nil {
		return domain.SourceSnapshot{}, fmt.Errorf("could not validate solver path: %w", err)
	}
	if !within {
		return domain.SourceSnapshot{}, fmt.Errorf("solver must be inside the project root: %s", sourcePath)
	}

	info, err := os.Lstat(absSource)
	if err != nil {
		return domain.SourceSnapshot{}, fmt.Errorf("could not open solver: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return domain.SourceSnapshot{}, fmt.Errorf("solver is not a regular file: %s", sourcePath)
	}

	content, err := os.ReadFile(absSource)
	if err != nil {
		return domain.SourceSnapshot{}, fmt.Errorf("could not open solver: %w", err)
	}
	snapshotPath := filepath.Join(runDir, "source", filepath.Base(absSource))
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		return domain.SourceSnapshot{}, fmt.Errorf("could not create snapshot directory: %w", err)
	}
	if err := os.WriteFile(snapshotPath, content, 0o444); err != nil {
		return domain.SourceSnapshot{}, fmt.Errorf("could not save solver snapshot: %w", err)
	}
	hash := sha256.Sum256(content)
	return domain.SourceSnapshot{
		SnapshotPath: snapshotPath,
		SHA256:       fmt.Sprintf("%x", hash),
	}, nil
}
