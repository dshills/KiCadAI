package preservation

import (
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestReportNormalizeAggregatesStatusAndSummary(t *testing.T) {
	report := New(ScopeImported)
	report.Files = []File{
		{Path: "b.kicad_pcb", Kind: "pcb", Ownership: OwnershipImportedUser},
		{Path: "a.kicad_sch", Kind: "schematic", Ownership: OwnershipImportedUser},
	}
	report.Objects = []Object{
		{Path: "b.kicad_pcb", Kind: "future_widget", Count: 2, Ownership: OwnershipUnknown, Mutability: MutabilityReadOnly},
		{Path: "a.kicad_sch", Kind: "rule_area", Count: 1, Ownership: OwnershipPreservationOnly, Mutability: MutabilityReadOnly},
	}
	report.OperationReviews = []OperationReview{
		OperationReviewFor(0, "add_symbol", "op-add", MutabilitySafeAdd, "isolated addition", nil),
		OperationReviewFor(1, "remove_symbol", "op-remove", MutabilityUnsafe, "unsafe remove", []reports.Issue{{
			Code:     reports.CodeUnsafeRemove,
			Severity: reports.SeverityBlocked,
			Path:     "operations[1]",
			Message:  "unsafe",
		}}),
	}

	report.Normalize()

	if report.Status != StatusBlocked {
		t.Fatalf("status = %q, want blocked", report.Status)
	}
	if report.Summary.Files != 2 || report.Summary.PreservationOnly != 1 || report.Summary.Unsupported != 2 ||
		report.Summary.SafeAddOperations != 1 || report.Summary.BlockedOperations != 1 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
	if report.Files[0].Path != "a.kicad_sch" || report.Objects[0].Path != "a.kicad_sch" {
		t.Fatalf("report was not deterministically ordered: %#v %#v", report.Files, report.Objects)
	}
	if !report.HasBlockedOperation() {
		t.Fatalf("expected blocked operation")
	}
}

func TestReportJSONShapeOmitsEmptySections(t *testing.T) {
	report := New(ScopeGenerated)
	report.Normalize()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"objects", "issues", "operation_reviews"} {
		if strings.Contains(text, `"`+forbidden+`":`) {
			t.Fatalf("empty field %q should be omitted: %s", forbidden, text)
		}
	}
}

func TestOperationReviewForClassifiesWarnings(t *testing.T) {
	review := OperationReviewFor(2, "assign_footprint", "op-assign", MutabilityPlanOnly, "existing ref", []reports.Issue{{
		Code:     reports.CodePinmapUnverified,
		Severity: reports.SeverityWarning,
		Path:     "operations[2]",
		Message:  "warning",
	}})
	if review.Status != StatusWarning || review.Mutability != MutabilityPlanOnly {
		t.Fatalf("unexpected review: %#v", review)
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	report := New(ScopeImported)
	report.Status = StatusWarning
	report.OperationReviews = []OperationReview{
		OperationReviewFor(0, "remove_symbol", "op-remove", MutabilityUnsafe, "unsafe remove", []reports.Issue{{
			Code:     reports.CodeUnsafeRemove,
			Severity: reports.SeverityBlocked,
			Path:     "operations[0]",
			Message:  "unsafe",
		}}),
	}
	report.Normalize()
	firstIssueCount := len(report.Issues)
	firstStatus := report.Status
	report.Normalize()
	if len(report.Issues) != firstIssueCount || report.Status != firstStatus {
		t.Fatalf("normalize not idempotent: issues=%d/%d status=%s/%s", firstIssueCount, len(report.Issues), firstStatus, report.Status)
	}
}

func TestNormalizeBlocksDirectUnsafeReview(t *testing.T) {
	report := New(ScopeImported)
	report.OperationReviews = []OperationReview{{Index: 0, Op: "remove_symbol", Mutability: MutabilityUnsafe}}
	report.Normalize()
	if report.OperationReviews[0].Status != StatusBlocked || report.Status != StatusBlocked {
		t.Fatalf("unexpected unsafe review normalization: %#v", report)
	}
}

func TestNormalizeDedupesDirectIssuesAndCountsFiles(t *testing.T) {
	issue := reports.Issue{
		Code:     reports.CodePreservationConflict,
		Severity: reports.SeverityWarning,
		Path:     "project",
		Message:  "preservation warning",
	}
	report := New(ScopeImported)
	report.Issues = []reports.Issue{issue, issue}
	report.Files = []File{
		{Path: "preserved.kicad_sch", Ownership: OwnershipPreservationOnly, Mutability: MutabilityReadOnly},
		{Path: "unknown.kicad_pcb", Ownership: OwnershipUnknown, Mutability: MutabilityReadOnly},
	}
	report.Normalize()
	if len(report.Issues) != 1 {
		t.Fatalf("direct issues were not deduped: %#v", report.Issues)
	}
	if report.Summary.PreservationOnly != 1 || report.Summary.Unsupported != 1 {
		t.Fatalf("file ownership not counted: %#v", report.Summary)
	}
}
