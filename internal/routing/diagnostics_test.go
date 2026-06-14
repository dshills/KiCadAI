package routing

import (
	"testing"

	"kicadai/internal/reports"
)

func TestDiagnosticsForResultClassifiesRouteSearchFailure(t *testing.T) {
	result := Result{Issues: []reports.Issue{{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     "nets.SIG",
		Message:  "no legal single-layer path found",
		Refs:     []string{"J1", "J2"},
		Nets:     []string{"SIG"},
	}}}

	diagnostics := DiagnosticsForResult(result)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Category != RepairRouteSearch || diagnostic.Action != ActionMoveComponents {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Suggestion == "" {
		t.Fatalf("suggestion missing: %#v", diagnostic)
	}
	if len(diagnostic.Refs) != 2 || len(diagnostic.Nets) != 1 {
		t.Fatalf("refs/nets not preserved: %#v", diagnostic)
	}
}

func TestDiagnosticForIssueClassifiesLayerAccess(t *testing.T) {
	diagnostic := DiagnosticForIssue(reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Message:  "vias are not allowed",
		Nets:     []string{"SIG"},
	})
	if diagnostic.Category != RepairLayerAccess || diagnostic.Action != ActionAllowAdditionalLayer {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestDiagnosticForIssuePreservesExplicitSuggestion(t *testing.T) {
	diagnostic := DiagnosticForIssue(reports.Issue{
		Code:       reports.CodeKiCadCLIFailed,
		Severity:   reports.SeverityBlocked,
		Message:    "kicad-cli failed",
		Suggestion: "rerun from a valid working directory",
	})
	if diagnostic.Category != RepairExternalCheck {
		t.Fatalf("category = %s", diagnostic.Category)
	}
	if diagnostic.Suggestion != "rerun from a valid working directory" {
		t.Fatalf("suggestion = %q", diagnostic.Suggestion)
	}
}
