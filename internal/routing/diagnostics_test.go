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

func TestDiagnosticForIssueClassifiesRoutingPolicyRepairs(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		category RepairCategory
		action   RepairAction
	}{
		{name: "rules", message: "power net has no explicit net class", category: RepairRoutingRules, action: ActionAdjustRoutingRules},
		{name: "zone", message: "zone routing policy unsupported", category: RepairZonePolicy, action: ActionResolveZonePolicy},
		{name: "length", message: "route length exceeds maximum", category: RepairLengthPolicy, action: ActionRelaxLengthPolicy},
		{name: "via", message: "max vias exceeded", category: RepairViaPolicy, action: ActionAdjustViaPolicy},
		{name: "keepout_zone", message: "clearance violation near keepout zone", category: RepairClearance, action: ActionReduceClearance},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic := DiagnosticForIssue(reports.Issue{Message: tc.message})
			if diagnostic.Category != tc.category || diagnostic.Action != tc.action {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
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
