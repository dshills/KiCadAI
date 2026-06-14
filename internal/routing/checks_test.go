package routing

import (
	"testing"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func TestIssuesFromCheckResultMapsDRCFindings(t *testing.T) {
	check := checks.CheckResult{
		Kind:   checks.CheckKindDRC,
		Status: checks.CheckStatusFail,
		Findings: []checks.CheckFinding{{
			ID:             "abc123",
			Kind:           checks.CheckKindDRC,
			Severity:       "error",
			Rule:           "clearance",
			Message:        "Track too close to pad",
			File:           "board.kicad_pcb",
			References:     []string{"R1"},
			Net:            "SIG",
			Layer:          "F.Cu",
			RepairCategory: checks.RepairClearance,
		}},
	}

	issues := IssuesFromCheckResult(check)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v", issues)
	}
	issue := issues[0]
	if issue.Code != reports.CodeValidationFailed || issue.Severity != reports.SeverityBlocked {
		t.Fatalf("issue = %#v", issue)
	}
	if len(issue.Refs) != 1 || issue.Refs[0] != "R1" {
		t.Fatalf("refs = %#v", issue.Refs)
	}
	if len(issue.Nets) != 1 || issue.Nets[0] != "SIG" {
		t.Fatalf("nets = %#v", issue.Nets)
	}
	if issue.Suggestion == "" {
		t.Fatalf("suggestion missing: %#v", issue)
	}
}

func TestAttachCheckResultBlocksPreviouslyRoutedResult(t *testing.T) {
	result := AttachCheckResult(Result{Status: StatusRouted}, checks.CheckResult{
		Kind:   checks.CheckKindDRC,
		Status: checks.CheckStatusFail,
		Findings: []checks.CheckFinding{{
			Kind:           checks.CheckKindDRC,
			Severity:       "error",
			Message:        "not connected",
			RepairCategory: checks.RepairConnectivity,
		}},
	})
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeDisconnectedPad {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestIssuesFromCheckResultIncludesToolError(t *testing.T) {
	issues := IssuesFromCheckResult(checks.CheckResult{
		Kind:   checks.CheckKindDRC,
		Status: checks.CheckStatusError,
		Stderr: "failed to get working directory",
	})
	if len(issues) != 1 {
		t.Fatalf("issues = %#v", issues)
	}
	if issues[0].Code != reports.CodeKiCadCLIFailed || issues[0].Severity != reports.SeverityBlocked {
		t.Fatalf("issue = %#v", issues[0])
	}
}
