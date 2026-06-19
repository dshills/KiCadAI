package repair

import (
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

type PostValidationOptions struct {
	WriterCorrectness       bool   `json:"writer_correctness"`
	BoardValidation         bool   `json:"board_validation"`
	KiCadERC                bool   `json:"kicad_erc"`
	KiCadDRC                bool   `json:"kicad_drc"`
	RoundTrip               bool   `json:"round_trip"`
	RequireKiCadERC         bool   `json:"require_kicad_erc"`
	RequireKiCadDRC         bool   `json:"require_kicad_drc"`
	RequireRoundTrip        bool   `json:"require_round_trip"`
	StrictZones             bool   `json:"strict_zones"`
	StrictUnrouted          bool   `json:"strict_unrouted"`
	AllowMissingKiCadChecks bool   `json:"allow_missing_kicad_checks"`
	KeepArtifacts           bool   `json:"keep_artifacts"`
	ArtifactDir             string `json:"artifact_dir,omitempty"`
	KiCadCLI                string `json:"kicad_cli,omitempty"`
}

type ValidationSummary struct {
	AdapterCount  int            `json:"adapter_count"`
	SkippedCount  int            `json:"skipped_count"`
	IssueCount    int            `json:"issue_count"`
	BlockingCount int            `json:"blocking_count"`
	ErrorCount    int            `json:"error_count"`
	WarningCount  int            `json:"warning_count"`
	InfoCount     int            `json:"info_count"`
	ArtifactCount int            `json:"artifact_count"`
	ByCode        map[string]int `json:"by_code,omitempty"`
	ByAdapter     map[string]int `json:"by_adapter,omitempty"`
}

type ValidationDelta struct {
	Before   ValidationSummary `json:"before"`
	After    ValidationSummary `json:"after"`
	Cleared  []reports.Issue   `json:"cleared,omitempty"`
	Repeated []reports.Issue   `json:"repeated,omitempty"`
	New      []reports.Issue   `json:"new,omitempty"`
	Worsened bool              `json:"worsened,omitempty"`
}

func SummarizePostValidation(validations []PostApplyValidation) ValidationSummary {
	summary := ValidationSummary{AdapterCount: len(validations)}
	uniqueIssues := map[string]reports.Issue{}
	for _, validation := range validations {
		if validation.Skipped {
			summary.SkippedCount++
		}
		if len(validation.Issues) > 0 {
			if summary.ByAdapter == nil {
				summary.ByAdapter = map[string]int{}
			}
			summary.ByAdapter[validation.Name] += len(validation.Issues)
		}
		summary.ArtifactCount += len(validation.Artifacts)
		for _, issue := range validation.Issues {
			key := StableIssueKey(issue)
			if _, ok := uniqueIssues[key]; !ok {
				uniqueIssues[key] = issue
			}
		}
	}
	addIssueSummary(&summary, issuesFromMap(uniqueIssues, false))
	return summary
}

func SummarizeIssues(issues []reports.Issue) ValidationSummary {
	summary := ValidationSummary{}
	addIssueSummary(&summary, issues)
	return summary
}

func CompareValidationIssues(before []reports.Issue, after []reports.Issue) ValidationDelta {
	beforeByKey := issueMap(before)
	afterByKey := issueMap(after)
	delta := ValidationDelta{
		Before: SummarizeIssues(issuesFromMap(beforeByKey, false)),
		After:  SummarizeIssues(issuesFromMap(afterByKey, false)),
	}
	for key, issue := range beforeByKey {
		if _, ok := afterByKey[key]; ok {
			delta.Repeated = append(delta.Repeated, issue)
			continue
		}
		delta.Cleared = append(delta.Cleared, issue)
	}
	for key, issue := range afterByKey {
		if _, ok := beforeByKey[key]; !ok {
			delta.New = append(delta.New, issue)
		}
	}
	sortIssuesForEvidence(delta.Cleared)
	sortIssuesForEvidence(delta.Repeated)
	sortIssuesForEvidence(delta.New)
	delta.Worsened = delta.After.BlockingCount > delta.Before.BlockingCount || len(blockingIssues(delta.New)) > 0
	return delta
}

func StableIssueKey(issue reports.Issue) string {
	var builder strings.Builder
	writeKeyPart(&builder, string(issue.Code))
	writeKeyPart(&builder, string(issue.Severity))
	writeKeyPart(&builder, slashPathForEvidence(issue.Path))
	writeKeyPart(&builder, strings.TrimSpace(issue.Message))
	writeKeyPart(&builder, strings.TrimSpace(issue.OperationID))
	writeKeyStringList(&builder, issue.UUIDs)
	writeKeyStringList(&builder, issue.Refs)
	writeKeyStringList(&builder, issue.Nets)
	return builder.String()
}

func writeKeyStringList(builder *strings.Builder, values []string) {
	values = sortedStrings(values)
	writeKeyPart(builder, strconv.Itoa(len(values)))
	for _, value := range values {
		writeKeyPart(builder, value)
	}
}

func writeKeyPart(builder *strings.Builder, part string) {
	builder.WriteString(strconv.Itoa(len(part)))
	builder.WriteByte(':')
	builder.WriteString(part)
	builder.WriteByte('|')
}

func addIssueSummary(summary *ValidationSummary, issues []reports.Issue) {
	for _, issue := range issues {
		summary.IssueCount++
		if summary.ByCode == nil {
			summary.ByCode = map[string]int{}
		}
		summary.ByCode[string(issue.Code)]++
		if issue.Blocking() {
			summary.BlockingCount++
		}
		switch issue.Severity {
		case reports.SeverityError, reports.SeverityBlocked:
			summary.ErrorCount++
		case reports.SeverityWarning:
			summary.WarningCount++
		case reports.SeverityInfo:
			summary.InfoCount++
		}
	}
}

func issueMap(issues []reports.Issue) map[string]reports.Issue {
	mapped := make(map[string]reports.Issue, len(issues))
	for _, issue := range issues {
		key := StableIssueKey(issue)
		if _, ok := mapped[key]; !ok {
			mapped[key] = issue
		}
	}
	return mapped
}

func issuesFromMap(mapped map[string]reports.Issue, sorted bool) []reports.Issue {
	issues := make([]reports.Issue, 0, len(mapped))
	for _, issue := range mapped {
		issues = append(issues, issue)
	}
	if sorted {
		sortIssuesForEvidence(issues)
	}
	return issues
}

func sortIssuesForEvidence(issues []reports.Issue) {
	if len(issues) < 2 {
		return
	}
	type keyedIssue struct {
		key   string
		issue reports.Issue
	}
	keyed := make([]keyedIssue, 0, len(issues))
	for _, issue := range issues {
		keyed = append(keyed, keyedIssue{key: StableIssueKey(issue), issue: issue})
	}
	sort.SliceStable(keyed, func(i, j int) bool {
		return keyed[i].key < keyed[j].key
	})
	for index, entry := range keyed {
		issues[index] = entry.issue
	}
}

func blockingIssues(issues []reports.Issue) []reports.Issue {
	blocking := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Blocking() {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

func slashPathForEvidence(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}
