package repair

import (
	"testing"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func TestNormalizeKiCadCheckFindingMapsERCToSchematicCategory(t *testing.T) {
	result := checks.CheckResult{Kind: checks.CheckKindERC, ReportPath: "artifacts/erc.json"}
	finding := checks.CheckFinding{
		Kind:           checks.CheckKindERC,
		Severity:       "warning",
		Rule:           "power_pin_not_driven",
		Message:        "power pin not driven",
		File:           "demo.kicad_sch",
		References:     []string{"U1"},
		Net:            "VCC",
		RepairCategory: checks.RepairPower,
	}
	normalized := NormalizeKiCadCheckFinding(finding, result, "")
	if normalized.Source != FindingSourceKiCadERC || normalized.Adapter != postValidatorKiCadERC {
		t.Fatalf("unexpected source/adapter: %+v", normalized)
	}
	if normalized.Category != FindingCategorySchematicERC || normalized.Repairability != RepairabilityRepairable {
		t.Fatalf("unexpected classification: %+v", normalized)
	}
	if normalized.Subject.Ref != "U1" || normalized.Subject.Net != "VCC" || normalized.Subject.Rule != "power_pin_not_driven" {
		t.Fatalf("unexpected subject: %+v", normalized.Subject)
	}
	if normalized.RawCode != "power_pin_not_driven" || normalized.EvidencePath != "artifacts/erc.json" {
		t.Fatalf("unexpected evidence identity: %+v", normalized)
	}
}

func TestNormalizeKiCadCheckFindingMapsDRCRepairCategories(t *testing.T) {
	tests := []struct {
		name     string
		finding  checks.CheckFinding
		category FindingCategory
		code     reports.Code
	}{
		{
			name: "clearance route",
			finding: checks.CheckFinding{
				Kind: checks.CheckKindDRC, Severity: "error", Rule: "clearance", Message: "track too close", File: "demo.kicad_pcb", Layer: "F.Cu", RepairCategory: checks.RepairClearance,
			},
			category: FindingCategoryRoute,
			code:     reports.CodeValidationFailed,
		},
		{
			name: "outline",
			finding: checks.CheckFinding{
				Kind: checks.CheckKindDRC, Severity: "error", Rule: "edge_cuts", Message: "board outline malformed", File: "demo.kicad_pcb", RepairCategory: checks.RepairOutline,
			},
			category: FindingCategoryOutline,
			code:     reports.CodeMissingBoardOutline,
		},
		{
			name: "net assignment",
			finding: checks.CheckFinding{
				Kind: checks.CheckKindDRC, Severity: "error", Rule: "netclass", Message: "pad has wrong net", File: "demo.kicad_pcb", References: []string{"J1"}, Net: "GND", RepairCategory: checks.RepairNetAssignment,
			},
			category: FindingCategoryConnectivity,
			code:     reports.CodeInvalidNetAssignment,
		},
		{
			name: "zone heuristic",
			finding: checks.CheckFinding{
				Kind: checks.CheckKindDRC, Severity: "error", Rule: "copper_zone", Message: "zone clearance problem", File: "demo.kicad_pcb", RepairCategory: checks.RepairClearance,
			},
			category: FindingCategoryZone,
			code:     reports.CodeValidationFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized := NormalizeKiCadCheckFinding(test.finding, checks.CheckResult{Kind: checks.CheckKindDRC}, "")
			if normalized.Source != FindingSourceKiCadDRC || normalized.Category != test.category || normalized.Code != test.code {
				t.Fatalf("normalized = %+v, want category %s code %s", normalized, test.category, test.code)
			}
		})
	}
}

func TestNormalizeKiCadCheckFindingDoesNotPathNormalizeIdentifiers(t *testing.T) {
	normalized := NormalizeKiCadCheckFinding(checks.CheckFinding{
		Kind:           checks.CheckKindDRC,
		Severity:       "error",
		Rule:           `custom\rule`,
		Message:        "identifier test",
		File:           `boards\demo.kicad_pcb`,
		References:     []string{`U\1`},
		Net:            `A\B`,
		RepairCategory: checks.RepairConnectivity,
	}, checks.CheckResult{Kind: checks.CheckKindDRC}, "")
	if normalized.Path != "boards/demo.kicad_pcb" {
		t.Fatalf("path was not slash-normalized: %+v", normalized)
	}
	if normalized.Subject.Ref != `U\1` || normalized.Subject.Net != `A\B` || normalized.Subject.Rule != `custom\rule` {
		t.Fatalf("EDA identifiers should not be slash-normalized: %+v", normalized.Subject)
	}
}

func TestNormalizeKiCadParserIssueIsExternalToolBlockedParse(t *testing.T) {
	normalized := NormalizeKiCadParserIssue(
		checks.ParserIssue{Message: "invalid JSON", Raw: "{"},
		checks.CheckResult{Kind: checks.CheckKindDRC, ReportPath: "reports/drc.json"},
		"",
	)
	if normalized.Source != FindingSourceKiCadDRC || normalized.Category != FindingCategoryParse {
		t.Fatalf("unexpected parser classification: %+v", normalized)
	}
	if normalized.Repairability != RepairabilityExternalToolBlocked || normalized.RawCode != "parser_issue" {
		t.Fatalf("unexpected parser repairability: %+v", normalized)
	}
}

func TestNormalizeKiCadCheckResultIncludesToolErrorWithoutParserDuplicate(t *testing.T) {
	result := checks.CheckResult{
		Kind:       checks.CheckKindDRC,
		Status:     checks.CheckStatusError,
		TargetPath: "demo.kicad_pcb",
		Stderr:     "tool failed",
	}
	findings := NormalizeKiCadCheckResult(result, "")
	if len(findings) != 1 {
		t.Fatalf("findings = %+v", findings)
	}
	if findings[0].Category != FindingCategoryExternalTool || findings[0].RawCode != "tool_error" || findings[0].Message != "tool failed" {
		t.Fatalf("unexpected tool error finding: %+v", findings[0])
	}

	result.ParserIssues = []checks.ParserIssue{{Message: "bad report"}}
	findings = NormalizeKiCadCheckResult(result, "")
	if len(findings) != 1 || findings[0].RawCode != "parser_issue" {
		t.Fatalf("parser issue should suppress duplicate generic tool error: %+v", findings)
	}
}

func TestNormalizeKiCadUnavailableMapsMissingCLI(t *testing.T) {
	issue := reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityError, Path: "demo.kicad_pcb", Message: "missing kicad-cli"}
	normalized := NormalizeKiCadUnavailable(issue, checks.CheckKindDRC, "")
	if normalized.Source != FindingSourceKiCadDRC || normalized.Category != FindingCategoryExternalTool {
		t.Fatalf("unexpected source/category: %+v", normalized)
	}
	if normalized.Repairability != RepairabilityExternalToolBlocked || normalized.RawCode != "missing_kicad_cli" {
		t.Fatalf("unexpected missing CLI finding: %+v", normalized)
	}
}
