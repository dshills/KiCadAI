package reports

import (
	"cmp"
	"slices"
)

const (
	DefaultMaxEmittedIssues    = 128
	DefaultMaxDiagnosticGroups = 32
	DefaultDiagnosticSamples   = 3
)

// DiagnosticSummary records the complete issue count while keeping stdout
// bounded for machine consumers.
type DiagnosticSummary struct {
	TotalCount        int               `json:"total_count"`
	EmittedCount      int               `json:"emitted_count"`
	OmittedCount      int               `json:"omitted_count"`
	GroupCount        int               `json:"group_count"`
	EmittedGroupCount int               `json:"emitted_group_count"`
	OmittedGroupCount int               `json:"omitted_group_count"`
	Groups            []DiagnosticGroup `json:"groups"`
}

// DiagnosticGroup identifies repeated findings by their stable producer
// stage, code, and source path.
type DiagnosticGroup struct {
	Stage        string  `json:"stage,omitempty"`
	Code         Code    `json:"code"`
	Source       string  `json:"source,omitempty"`
	TotalCount   int     `json:"total_count"`
	EmittedCount int     `json:"emitted_count"`
	OmittedCount int     `json:"omitted_count"`
	Samples      []Issue `json:"samples"`
}

type diagnosticKey struct {
	stage  string
	code   Code
	source string
}

// DiagnosticDataBounder lets command-specific envelopes bound duplicated
// issue collections without converting arbitrary report data through JSON.
type DiagnosticDataBounder interface {
	BoundedDiagnostics(maxIssues int) any
}

// BoundedResult returns a copy suitable for stdout. Results at or below the
// issue limit are unchanged so existing compact envelopes remain stable.
func BoundedResult(result Result) Result {
	if len(result.Issues) <= DefaultMaxEmittedIssues {
		return result
	}

	ordered := SortedIssues(result.Issues)
	result.Issues = append([]Issue(nil), ordered[:DefaultMaxEmittedIssues]...)
	if bounder, ok := result.Data.(DiagnosticDataBounder); ok {
		result.Data = bounder.BoundedDiagnostics(DefaultMaxEmittedIssues)
	}

	grouped := make(map[diagnosticKey][]Issue)
	for _, issue := range ordered {
		key := diagnosticKey{stage: issue.Stage, code: issue.Code, source: issue.Path}
		grouped[key] = append(grouped[key], issue)
	}
	keys := make([]diagnosticKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b diagnosticKey) int {
		if value := cmp.Compare(len(grouped[b]), len(grouped[a])); value != 0 {
			return value
		}
		if value := cmp.Compare(a.stage, b.stage); value != 0 {
			return value
		}
		if value := cmp.Compare(a.code, b.code); value != 0 {
			return value
		}
		return cmp.Compare(a.source, b.source)
	})

	emittedGroups := min(len(keys), DefaultMaxDiagnosticGroups)
	groups := make([]DiagnosticGroup, 0, emittedGroups)
	for _, key := range keys[:emittedGroups] {
		issues := grouped[key]
		emitted := min(len(issues), DefaultDiagnosticSamples)
		groups = append(groups, DiagnosticGroup{
			Stage:        key.stage,
			Code:         key.code,
			Source:       key.source,
			TotalCount:   len(issues),
			EmittedCount: emitted,
			OmittedCount: len(issues) - emitted,
			Samples:      append([]Issue(nil), issues[:emitted]...),
		})
	}
	result.Diagnostics = &DiagnosticSummary{
		TotalCount:        len(ordered),
		EmittedCount:      len(result.Issues),
		OmittedCount:      len(ordered) - len(result.Issues),
		GroupCount:        len(keys),
		EmittedGroupCount: len(groups),
		OmittedGroupCount: len(keys) - len(groups),
		Groups:            groups,
	}
	return result
}

// SortedIssues returns a stable copy for durable diagnostic artifacts.
func SortedIssues(issues []Issue) []Issue {
	ordered := append([]Issue(nil), issues...)
	slices.SortFunc(ordered, compareIssues)
	return ordered
}

// BoundedIssues returns a deterministic issue sample no larger than maxIssues.
func BoundedIssues(issues []Issue, maxIssues int) []Issue {
	ordered := SortedIssues(issues)
	if maxIssues >= 0 && len(ordered) > maxIssues {
		ordered = ordered[:maxIssues]
	}
	return ordered
}

func compareIssues(a, b Issue) int {
	for _, value := range []int{
		cmp.Compare(a.Stage, b.Stage),
		cmp.Compare(a.Code, b.Code),
		cmp.Compare(a.Path, b.Path),
		cmp.Compare(a.Severity, b.Severity),
		cmp.Compare(a.RetryScope, b.RetryScope),
		cmp.Compare(a.IssueID, b.IssueID),
		cmp.Compare(a.RootCauseID, b.RootCauseID),
		cmp.Compare(a.Message, b.Message),
		cmp.Compare(a.Suggestion, b.Suggestion),
		cmp.Compare(a.OperationID, b.OperationID),
	} {
		if value != 0 {
			return value
		}
	}
	if value := slices.Compare(a.UUIDs, b.UUIDs); value != 0 {
		return value
	}
	if value := slices.Compare(a.Refs, b.Refs); value != 0 {
		return value
	}
	return slices.Compare(a.Nets, b.Nets)
}
