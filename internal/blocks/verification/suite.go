package verification

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"

	"kicadai/internal/reports"
)

func DiscoverManifestPaths(root string) ([]string, []reports.Issue) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "manifest.json" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		code := reports.CodeUnknown
		if errors.Is(err, fs.ErrNotExist) {
			code = reports.CodeMissingFile
		}
		return nil, []reports.Issue{issue(code, reports.SeverityError, "verification.suite", err.Error())}
	}
	slices.Sort(paths)
	return paths, nil
}

func LoadSuite(root string) ([]Manifest, []reports.Issue) {
	paths, issues := DiscoverManifestPaths(root)
	if len(issues) != 0 {
		return nil, issues
	}
	manifests := make([]Manifest, 0, len(paths))
	for _, path := range paths {
		manifest, manifestIssues := LoadManifest(path)
		issues = append(issues, manifestIssues...)
		if len(manifestIssues) != 0 {
			continue
		}
		manifests = append(manifests, manifest)
	}
	return manifests, issues
}
