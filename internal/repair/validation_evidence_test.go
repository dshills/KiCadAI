package repair

import (
	"testing"

	"kicadai/internal/reports"
)

func TestStableIssueKeySortsIdentitySlices(t *testing.T) {
	left := reports.Issue{
		Code:        reports.CodeDisconnectedPad,
		Severity:    reports.SeverityError,
		Path:        `pcb\board.kicad_pcb`,
		Message:     "pad is disconnected",
		Refs:        []string{"U2", "U1"},
		Nets:        []string{"GND", "VCC"},
		UUIDs:       []string{"b", "a"},
		OperationID: "route-1",
	}
	right := left
	right.Path = "pcb/board.kicad_pcb"
	right.Refs = []string{"U1", "U2"}
	right.Nets = []string{"VCC", "GND"}
	right.UUIDs = []string{"a", "b"}
	if StableIssueKey(left) != StableIssueKey(right) {
		t.Fatalf("stable keys differ:\nleft=%q\nright=%q", StableIssueKey(left), StableIssueKey(right))
	}
}

func TestSummarizePostValidationCountsAdaptersIssuesAndArtifacts(t *testing.T) {
	summary := SummarizePostValidation([]PostApplyValidation{
		{
			Name: "writer",
			Issues: []reports.Issue{
				{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Message: "warning"},
				{Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing"},
			},
			Artifacts: []reports.Artifact{{Kind: reports.ArtifactValidationReport, Path: "writer-report.json"}},
		},
		{Name: "round_trip", Skipped: true},
	})
	if summary.AdapterCount != 2 || summary.SkippedCount != 1 || summary.IssueCount != 2 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.BlockingCount != 1 || summary.WarningCount != 1 || summary.ArtifactCount != 1 {
		t.Fatalf("unexpected issue/artifact summary: %+v", summary)
	}
	if summary.ByAdapter["writer"] != 2 || summary.ByCode[string(reports.CodeMissingBoardOutline)] != 1 {
		t.Fatalf("missing buckets: %+v", summary)
	}
}

func TestSummarizePostValidationDeduplicatesProjectIssueCounts(t *testing.T) {
	issue := reports.Issue{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Path: "pcb", Message: "same"}
	summary := SummarizePostValidation([]PostApplyValidation{
		{Name: "board", Issues: []reports.Issue{issue}},
		{Name: "drc", Issues: []reports.Issue{issue}},
	})
	if summary.IssueCount != 1 || summary.BlockingCount != 1 {
		t.Fatalf("summary should count unique project issues: %+v", summary)
	}
	if summary.ByAdapter["board"] != 1 || summary.ByAdapter["drc"] != 1 {
		t.Fatalf("summary should retain per-adapter report counts: %+v", summary)
	}
}

func TestCompareValidationIssuesReportsClearedRepeatedNewAndWorsened(t *testing.T) {
	cleared := reports.Issue{Code: reports.CodeMissingFootprint, Severity: reports.SeverityError, Path: "a", Message: "cleared"}
	repeated := reports.Issue{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityWarning, Path: "b", Message: "same"}
	newBlocking := reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: "c", Message: "new"}
	delta := CompareValidationIssues([]reports.Issue{cleared, repeated}, []reports.Issue{repeated, newBlocking})
	if len(delta.Cleared) != 1 || delta.Cleared[0].Message != "cleared" {
		t.Fatalf("cleared = %+v", delta.Cleared)
	}
	if len(delta.Repeated) != 1 || delta.Repeated[0].Message != "same" {
		t.Fatalf("repeated = %+v", delta.Repeated)
	}
	if len(delta.New) != 1 || delta.New[0].Message != "new" {
		t.Fatalf("new = %+v", delta.New)
	}
	if !delta.Worsened {
		t.Fatalf("delta should be worsened: %+v", delta)
	}
}

func TestCompareValidationIssuesDoesNotWorsenForWarningOnlyRegression(t *testing.T) {
	before := []reports.Issue{
		{Code: reports.CodeMissingFootprint, Severity: reports.SeverityError, Path: "a", Message: "blocking"},
	}
	after := []reports.Issue{
		{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "b", Message: "warning"},
	}
	delta := CompareValidationIssues(before, after)
	if delta.Worsened {
		t.Fatalf("warning-only after state should not be worsened: %+v", delta)
	}
}

func TestCompareValidationIssuesSummariesUseUniqueIssueIdentity(t *testing.T) {
	issue := reports.Issue{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Path: "pcb", Message: "same"}
	delta := CompareValidationIssues([]reports.Issue{issue, issue}, []reports.Issue{issue})
	if delta.Before.IssueCount != 1 || delta.After.IssueCount != 1 || len(delta.Repeated) != 1 {
		t.Fatalf("delta should summarize unique issues: %+v", delta)
	}
}
