package physicalrules

import (
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestNormalizeAggregatesStatusAndIssues(t *testing.T) {
	report := NewReport(" generic_assembly ", BoardRef{Path: "demo.kicad_pcb", LayerCount: 2}, []Check{
		{ID: CheckStackupCopperLayers, Category: CategoryStackup, Status: StatusPass, Message: "copper layers are valid"},
		{ID: CheckCourtyardPresence, Category: CategoryCourtyard, Status: StatusWarning, Message: "missing courtyard", References: []string{"U1"}, Suggestion: "hydrate the footprint courtyard"},
		{ID: CheckEdgeCutsOutline, Category: CategoryEdgeCuts, Status: StatusBlocked, Message: "missing outline", IssueCode: reports.CodeMissingBoardOutline},
	})

	if report.Schema != ReportSchema || report.Profile != "generic_assembly" {
		t.Fatalf("report identity = %#v", report)
	}
	if report.Status != StatusBlocked {
		t.Fatalf("status = %q, want blocked", report.Status)
	}
	if report.Summary.PassCount != 1 || report.Summary.WarningCount != 1 || report.Summary.BlockedCount != 1 {
		t.Fatalf("summary = %#v", report.Summary)
	}
	if got := report.Summary.Categories[CategoryCourtyard]; got != StatusWarning {
		t.Fatalf("courtyard category = %q, want warning", got)
	}
	if len(report.Issues) != 2 {
		t.Fatalf("issues = %#v, want warning and blocked issues", report.Issues)
	}
	if report.Issues[0].Path == "" || report.Issues[1].Path == "" {
		t.Fatalf("issues missing paths: %#v", report.Issues)
	}
}

func TestMarshalJSONIsDeterministic(t *testing.T) {
	report := Report{Checks: []Check{
		{ID: CheckMountingHolePresence, Category: CategoryMountingHole, Status: StatusSkipped, Message: "not required"},
		{ID: CheckStackupThickness, Category: CategoryStackup, Status: StatusPass, Message: "thickness is valid"},
		{ID: CheckAnnularRingVia, Category: CategoryAnnularRing, Status: StatusPass, Message: "via rings are valid"},
	}}
	data, err := MarshalReportJSON(report)
	if err != nil {
		t.Fatalf("MarshalJSON returned error: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		`"schema": "kicadai.fabrication.physical_rules.v1"`,
		`"status": "pass"`,
		`"annular_ring": "pass"`,
		`"mounting_hole": "skipped"`,
		`"stackup": "pass"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("JSON missing %s:\n%s", want, text)
		}
	}
}

func TestNormalizeOptionsAppliesDFMDefaults(t *testing.T) {
	opts := NormalizeOptions(Options{})
	if opts.MinPlatedPadAnnularRingMM != defaultMinPlatedPadRingMM {
		t.Fatalf("MinPlatedPadAnnularRingMM = %g", opts.MinPlatedPadAnnularRingMM)
	}
	if opts.MinViaRingMM != defaultMinViaRingMM {
		t.Fatalf("MinViaRingMM = %g", opts.MinViaRingMM)
	}
	if opts.MinCopperFeatureMM != defaultMinCopperFeatureMM {
		t.Fatalf("MinCopperFeatureMM = %g", opts.MinCopperFeatureMM)
	}
	if opts.MinSolderMaskWebMM != defaultMinSolderMaskWebMM {
		t.Fatalf("MinSolderMaskWebMM = %g", opts.MinSolderMaskWebMM)
	}
	if opts.EdgePlatingPolicy != PolicyWarn || opts.ImpedancePolicy != PolicyWarn || opts.PanelizationPolicy != PolicyIgnore {
		t.Fatalf("policies = %#v", opts)
	}

	opts = NormalizeOptions(Options{
		MinPlatedPadAnnularRingMM: 0.20,
		MinViaRingMM:              0.12,
		MinCopperFeatureMM:        0.15,
		MinSolderMaskWebMM:        0.09,
		EdgePlatingPolicy:         PolicyBlock,
		ImpedancePolicy:           PolicyIgnore,
		PanelizationPolicy:        PolicyBlock,
	})
	if opts.MinPlatedPadAnnularRingMM != 0.20 || opts.MinViaRingMM != 0.12 || opts.MinCopperFeatureMM != 0.15 || opts.MinSolderMaskWebMM != 0.09 {
		t.Fatalf("overrides not preserved: %#v", opts)
	}
	if opts.EdgePlatingPolicy != PolicyBlock || opts.ImpedancePolicy != PolicyIgnore || opts.PanelizationPolicy != PolicyBlock {
		t.Fatalf("policy overrides not preserved: %#v", opts)
	}
}

func TestIssueForCheckUsesDefaults(t *testing.T) {
	issue, ok := IssueForCheck(Check{
		ID:         CheckSolderPastePadLayers,
		Category:   CategorySolderPaste,
		Status:     StatusBlocked,
		Message:    "SMD pad has no paste layer",
		References: []string{"U1"},
		Nets:       []string{"VCC"},
		Objects:    []string{"pad-1"},
	})
	if !ok {
		t.Fatal("IssueForCheck did not return issue")
	}
	if issue.Code != reports.CodeValidationFailed || issue.Severity != reports.SeverityError || issue.Path != CheckSolderPastePadLayers {
		t.Fatalf("issue defaults = %#v", issue)
	}
	if len(issue.Refs) != 1 || issue.Refs[0] != "U1" || len(issue.Nets) != 1 || issue.Nets[0] != "VCC" || len(issue.UUIDs) != 1 || issue.UUIDs[0] != "pad-1" {
		t.Fatalf("issue evidence = %#v", issue)
	}
}

func TestNormalizePreservesExplicitIssues(t *testing.T) {
	report := Normalize(Report{
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     "physical.custom",
			Message:  "custom warning",
		}},
		Checks: []Check{{
			ID:       CheckEdgeCutsOutline,
			Category: CategoryEdgeCuts,
			Status:   StatusBlocked,
			Message:  "missing outline",
		}},
	})
	if len(report.Issues) != 2 {
		t.Fatalf("issues = %#v, want explicit and derived issue", report.Issues)
	}
	again := Normalize(report)
	if len(again.Issues) != 2 {
		t.Fatalf("renormalized issues = %#v, want no duplicate derived issue", again.Issues)
	}
}

func TestIssueForCheckFallsBackForEmptyPathAndMessage(t *testing.T) {
	issue, ok := IssueForCheck(normalizeCheck(Check{Status: StatusBlocked}))
	if !ok {
		t.Fatal("IssueForCheck did not return issue")
	}
	if issue.Path != "physical.unknown" {
		t.Fatalf("path = %q, want fallback", issue.Path)
	}
	if issue.Message == "" {
		t.Fatal("message should not be empty")
	}
}
