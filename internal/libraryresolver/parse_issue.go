package libraryresolver

import (
	"path/filepath"

	"kicadai/internal/reports"
)

func parseIssue(path string, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityWarning,
		Path:     filepath.ToSlash(path),
		Message:  message,
	}
}
