package libraryresolver

import (
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/reports"
)

func ResolveRoots() (LibraryRoots, []reports.Issue) {
	roots := LibraryRoots{
		KLCRoot:        firstNonEmpty(os.Getenv(EnvKLCRoot)),
		SymbolsRoot:    firstNonEmpty(os.Getenv(EnvSymbolsRoot)),
		FootprintsRoot: firstNonEmpty(os.Getenv(EnvFootprintsRoot)),
		TemplatesRoot:  firstNonEmpty(os.Getenv(EnvTemplatesRoot)),
	}
	return roots, ValidateRoots(roots)
}

func ValidateRoots(roots LibraryRoots) []reports.Issue {
	var issues []reports.Issue
	issues = append(issues, validateRoot("roots.klc_root", "KLC root", roots.KLCRoot)...)
	issues = append(issues, validateRoot("roots.symbols_root", "symbols root", roots.SymbolsRoot)...)
	issues = append(issues, validateRoot("roots.footprints_root", "footprints root", roots.FootprintsRoot)...)
	issues = append(issues, validateRoot("roots.templates_root", "templates root", roots.TemplatesRoot)...)
	return issues
}

func ValidateCachePath(path string) []reports.Issue {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if hasParentTraversal(path) {
		return []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library_cache",
			Message:  "library cache path must not contain parent directory traversal",
		}}
	}
	path = filepath.Clean(path)
	if hasParentTraversal(path) {
		return []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library_cache",
			Message:  "library cache path must not contain parent directory traversal",
		}}
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library_cache",
			Message:  "library cache path must name a file, not a directory",
		}}
	} else if err != nil && !os.IsNotExist(err) {
		return []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "library_cache",
			Message:  err.Error(),
		}}
	}
	if filepath.Base(path) == "." || filepath.Base(path) == string(filepath.Separator) {
		return []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library_cache",
			Message:  "library cache path must name a file",
		}}
	}
	return nil
}

func validateRoot(path string, label string, root string) []reports.Issue {
	root = strings.TrimSpace(root)
	if root == "" {
		return []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityWarning,
			Path:     path,
			Message:  label + " is not configured",
		}}
	}
	info, err := os.Stat(root)
	if err != nil {
		code := reports.CodeValidationFailed
		if os.IsNotExist(err) {
			code = reports.CodeMissingFile
		}
		return []reports.Issue{{
			Code:     code,
			Severity: reports.SeverityWarning,
			Path:     path,
			Message:  err.Error(),
		}}
	}
	if !info.IsDir() {
		return []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityWarning,
			Path:     path,
			Message:  label + " must be a directory",
		}}
	}
	return nil
}

func hasParentTraversal(path string) bool {
	slashed := filepath.ToSlash(path)
	if strings.Contains(slashed, ":..") {
		return true
	}
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
