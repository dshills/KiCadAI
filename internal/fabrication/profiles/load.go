package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const EnvProfileDir = "KICADAI_FABRICATION_PROFILE_DIR"

type LoadOptions struct {
	ProfileDir string
}

func LoadRegistry(opts LoadOptions) (Registry, []reports.Issue) {
	dir := strings.TrimSpace(opts.ProfileDir)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv(EnvProfileDir))
	}
	registry := builtinRegistry().mutableCopy()
	if dir == "" {
		return registry, nil
	}
	localProfiles, issues := LoadDirectory(dir)
	for _, profile := range localProfiles {
		if _, exists := registry.profiles[profile.ID]; exists {
			issues = append(issues, profileLoadIssue(profile.Source.Path, "fabrication profile shadows an existing profile ID"))
			continue
		}
		registry.profiles[profile.ID] = profile
		registry.order = append(registry.order, profile.ID)
	}
	return registry, sortIssues(issues)
}

func LoadDirectory(dir string) ([]Profile, []reports.Issue) {
	root, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return nil, []reports.Issue{profileLoadIssue(dir, err.Error())}
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, []reports.Issue{profileLoadIssue(dir, err.Error())}
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, []reports.Issue{profileLoadIssue(dir, err.Error())}
	}
	if !info.IsDir() {
		return nil, []reports.Issue{profileLoadIssue(dir, "fabrication profile path is not a directory")}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, []reports.Issue{profileLoadIssue(dir, err.Error())}
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	var profiles []Profile
	var issues []reports.Issue
	seenLocal := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			issues = append(issues, profileLoadIssue(path, err.Error()))
			continue
		}
		if !pathWithin(root, realPath) {
			issues = append(issues, profileLoadIssue(path, "fabrication profile file resolves outside the profile directory"))
			continue
		}
		data, err := os.ReadFile(realPath)
		if err != nil {
			issues = append(issues, profileLoadIssue(path, err.Error()))
			continue
		}
		var profile Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			issues = append(issues, profileLoadIssue(path, "invalid fabrication profile JSON: "+err.Error()))
			continue
		}
		profile.Source.Kind = SourceLocal
		profile.Source.Path = filepath.ToSlash(realPath)
		profileIssues := Validate(profile)
		if len(profileIssues) > 0 {
			issues = append(issues, profileIssues...)
			continue
		}
		if previousPath, exists := seenLocal[profile.ID]; exists {
			issues = append(issues, profileLoadIssue(path, fmt.Sprintf("duplicate local fabrication profile ID %q also defined in %s", profile.ID, filepath.ToSlash(previousPath))))
			continue
		}
		seenLocal[profile.ID] = realPath
		profiles = append(profiles, profile)
	}
	slices.SortFunc(profiles, func(a, b Profile) int {
		if a.ID != b.ID {
			return strings.Compare(a.ID, b.ID)
		}
		return strings.Compare(a.Source.Path, b.Source.Path)
	})
	return profiles, sortIssues(issues)
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "..")
}

func profileLoadIssue(path string, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "fabrication_profile." + filepath.ToSlash(path),
		Message:  message,
	}
}

func sortIssues(issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	issues = slices.Clone(issues)
	slices.SortFunc(issues, compareIssues)
	return issues
}
