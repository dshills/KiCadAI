package transactions

import (
	"sort"

	"kicadai/internal/reports"
)

type OperationFeedback struct {
	OperationID string             `json:"operation_id"`
	Index       int                `json:"index"`
	Op          OperationKind      `json:"op"`
	Refs        []string           `json:"refs,omitempty"`
	Nets        []string           `json:"nets,omitempty"`
	Severity    reports.Severity   `json:"severity,omitempty"`
	Issues      []reports.Issue    `json:"issues"`
	Artifacts   []reports.Artifact `json:"artifacts,omitempty"`
	Suggestions []string           `json:"suggestions,omitempty"`
}

type FeedbackSummary struct {
	OperationCount int `json:"operation_count"`
	IssueCount     int `json:"issue_count"`
	BlockingCount  int `json:"blocking_count"`
	ErrorCount     int `json:"error_count"`
	WarningCount   int `json:"warning_count"`
	UnlinkedCount  int `json:"unlinked_count"`
}

type FeedbackReport struct {
	Target     string              `json:"target,omitempty"`
	Operations []OperationFeedback `json:"operations"`
	Issues     []reports.Issue     `json:"issues"`
	Artifacts  []reports.Artifact  `json:"artifacts,omitempty"`
	Summary    FeedbackSummary     `json:"summary"`
}

type feedbackOperation struct {
	ID        string
	Index     int
	Op        OperationKind
	Refs      []string
	Nets      []string
	Artifacts []reports.Artifact
}

func FeedbackFromPlan(plan Plan) FeedbackReport {
	operations := make([]feedbackOperation, 0, len(plan.Operations))
	var artifacts []reports.Artifact
	for _, operation := range plan.Operations {
		operationArtifacts := append([]reports.Artifact(nil), operation.Artifacts...)
		artifacts = append(artifacts, operationArtifacts...)
		operations = append(operations, feedbackOperation{
			ID:        operation.ID,
			Index:     operation.Index,
			Op:        operation.Op,
			Refs:      append([]string(nil), operation.Refs...),
			Nets:      append([]string(nil), operation.Nets...),
			Artifacts: operationArtifacts,
		})
	}
	return buildFeedbackReport(plan.Target, operations, plan.Issues, artifacts)
}

func FeedbackFromValidation(result ValidationResult) FeedbackReport {
	operations := make([]feedbackOperation, 0, len(result.Operations))
	for _, operation := range result.Operations {
		operations = append(operations, feedbackOperation{
			ID:    operation.ID,
			Index: operation.Index,
			Op:    operation.Op,
			Refs:  append([]string(nil), operation.Refs...),
			Nets:  append([]string(nil), operation.Nets...),
		})
	}
	return buildFeedbackReport("", operations, result.Issues, nil)
}

func buildFeedbackReport(target string, operations []feedbackOperation, issues []reports.Issue, artifacts []reports.Artifact) FeedbackReport {
	report := FeedbackReport{
		Target:     target,
		Operations: make([]OperationFeedback, 0, len(operations)),
		Issues:     append([]reports.Issue(nil), issues...),
		Artifacts:  append([]reports.Artifact(nil), artifacts...),
		Summary: FeedbackSummary{
			OperationCount: len(operations),
			IssueCount:     len(issues),
		},
	}
	issueGroups := map[string][]reports.Issue{}
	validOperationIDs := map[string]struct{}{}
	for _, operation := range operations {
		if operation.ID != "" {
			validOperationIDs[operation.ID] = struct{}{}
		}
	}
	for _, issue := range issues {
		if issue.Blocking() {
			report.Summary.BlockingCount++
		}
		if issue.Severity == reports.SeverityError {
			report.Summary.ErrorCount++
		}
		if issue.Severity == reports.SeverityWarning {
			report.Summary.WarningCount++
		}
		if issue.OperationID == "" {
			report.Summary.UnlinkedCount++
			continue
		}
		if _, exists := validOperationIDs[issue.OperationID]; !exists {
			report.Summary.UnlinkedCount++
			continue
		}
		issueGroups[issue.OperationID] = append(issueGroups[issue.OperationID], issue)
	}
	for _, operation := range operations {
		groupIssues := issueGroups[operation.ID]
		report.Operations = append(report.Operations, OperationFeedback{
			OperationID: operation.ID,
			Index:       operation.Index,
			Op:          operation.Op,
			Refs:        operation.Refs,
			Nets:        operation.Nets,
			Severity:    highestSeverity(groupIssues),
			Issues:      groupIssues,
			Artifacts:   operation.Artifacts,
			Suggestions: issueSuggestions(groupIssues),
		})
	}
	sort.SliceStable(report.Operations, func(i, j int) bool {
		return report.Operations[i].Index < report.Operations[j].Index
	})
	return report
}

func highestSeverity(issues []reports.Issue) reports.Severity {
	if len(issues) == 0 {
		return ""
	}
	severity := issues[0].Severity
	for _, issue := range issues[1:] {
		if severityRank(issue.Severity) > severityRank(severity) {
			severity = issue.Severity
		}
	}
	return severity
}

func severityRank(severity reports.Severity) int {
	switch severity {
	case reports.SeverityBlocked:
		return 4
	case reports.SeverityError:
		return 3
	case reports.SeverityWarning:
		return 2
	case reports.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func issueSuggestions(issues []reports.Issue) []string {
	var suggestions []string
	seen := map[string]struct{}{}
	for _, issue := range issues {
		if issue.Suggestion == "" {
			continue
		}
		if _, exists := seen[issue.Suggestion]; exists {
			continue
		}
		seen[issue.Suggestion] = struct{}{}
		suggestions = append(suggestions, issue.Suggestion)
	}
	return suggestions
}
