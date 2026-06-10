package roundtrip

import (
	"fmt"
	"strings"
)

type AllowlistEntry struct {
	FileType     FileType
	FixtureName  string
	Category     string
	Section      string
	Message      string
	DiffContains string
	Reason       string
	CleanupNote  string
}

func ValidateAllowlist(entries []AllowlistEntry) error {
	for i, entry := range entries {
		if strings.TrimSpace(entry.Reason) == "" {
			return fmt.Errorf("allowlist[%d].reason is required", i)
		}
		if entry.FileType == "" && strings.TrimSpace(entry.FixtureName) == "" && strings.TrimSpace(entry.Category) == "" && strings.TrimSpace(entry.Section) == "" && strings.TrimSpace(entry.Message) == "" && strings.TrimSpace(entry.DiffContains) == "" {
			return fmt.Errorf("allowlist[%d] must match a category, message, or diff hunk", i)
		}
	}
	return nil
}

func FilterAllowedDifferences(result Result, entries []AllowlistEntry) (Result, []Difference, error) {
	if err := ValidateAllowlist(entries); err != nil {
		return result, nil, err
	}
	if len(entries) == 0 || len(result.Differences) == 0 {
		return result, nil, nil
	}
	remaining := make([]Difference, 0, len(result.Differences))
	allowed := make([]Difference, 0)
	for _, diff := range result.Differences {
		if matchesAnyAllowlist(result, diff, entries) {
			allowed = append(allowed, diff)
			continue
		}
		remaining = append(remaining, diff)
	}
	result.Differences = remaining
	result.Equal = result.Equal || len(remaining) == 0
	return result, allowed, nil
}

func matchesAnyAllowlist(result Result, diff Difference, entries []AllowlistEntry) bool {
	for _, entry := range entries {
		if entry.FileType != "" && entry.FileType != result.FileType {
			continue
		}
		if entry.FixtureName != "" && entry.FixtureName != result.FixtureName {
			continue
		}
		if entry.Category != "" && entry.Category != diff.Category {
			continue
		}
		if entry.Section != "" && entry.Section != diff.Section {
			continue
		}
		if entry.Message != "" && !strings.Contains(diff.Message, entry.Message) {
			continue
		}
		if entry.DiffContains != "" && !strings.Contains(diff.Diff, entry.DiffContains) {
			continue
		}
		return true
	}
	return false
}
