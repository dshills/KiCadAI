package reportartifacts

import (
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/reports"
)

// ExistingReportArtifact returns one artifact for an existing report file.
// Empty paths, missing paths, and directories are omitted.
func ExistingReportArtifact(kind reports.ArtifactKind, path string, description string) []reports.Artifact {
	reportPath := strings.TrimSpace(path)
	if reportPath == "" {
		return nil
	}
	info, err := os.Stat(reportPath)
	if err != nil || info.IsDir() {
		return nil
	}
	artifact := reports.Artifact{Kind: kind, Path: filepath.ToSlash(reportPath), Description: description}
	return []reports.Artifact{artifact}
}
