package boardvalidation

import (
	"errors"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
)

func TestAggregateStatusRequiredChecks(t *testing.T) {
	tests := []struct {
		name   string
		checks []Check
		want   Status
	}{
		{
			name: "all required pass",
			checks: []Check{
				{Name: CheckPCBStructuralValidation, Required: true, Status: StatusPass},
				{Name: CheckKiCadDRC, Required: false, Status: StatusSkipped},
			},
			want: StatusPass,
		},
		{
			name: "required fail",
			checks: []Check{
				{Name: CheckPCBStructuralValidation, Required: true, Status: StatusFail},
			},
			want: StatusFail,
		},
		{
			name: "required skipped",
			checks: []Check{
				{Name: CheckPCBStructuralValidation, Required: true, Status: StatusSkipped},
			},
			want: StatusSkipped,
		},
		{
			name: "optional fail fails aggregate",
			checks: []Check{
				{Name: CheckPCBStructuralValidation, Required: true, Status: StatusPass},
				{Name: CheckKiCadDRC, Required: false, Status: StatusFail},
			},
			want: StatusFail,
		},
		{
			name: "optional only pass is pass",
			checks: []Check{
				{Name: CheckKiCadDRC, Required: false, Status: StatusPass},
			},
			want: StatusPass,
		},
		{
			name: "optional only fail is fail",
			checks: []Check{
				{Name: CheckKiCadDRC, Required: false, Status: StatusFail},
			},
			want: StatusFail,
		},
		{
			name: "optional only skipped is skipped",
			checks: []Check{
				{Name: CheckKiCadDRC, Required: false, Status: StatusSkipped},
			},
			want: StatusSkipped,
		},
		{
			name: "error wins",
			checks: []Check{
				{Name: CheckPCBStructuralValidation, Required: true, Status: StatusPass},
				{Name: CheckKiCadDRC, Required: false, Status: StatusError},
			},
			want: StatusError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AggregateStatus(tt.checks); got != tt.want {
				t.Fatalf("AggregateStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortChecksUsesStableOrder(t *testing.T) {
	checks := []Check{
		{Name: CheckKiCadDRC},
		{Name: "aaa_custom"},
		{Name: CheckPCBStructuralValidation},
		{Name: CheckRouteCompletion},
	}
	SortChecks(checks)
	got := []string{checks[0].Name, checks[1].Name, checks[2].Name, checks[3].Name}
	want := []string{CheckPCBStructuralValidation, CheckRouteCompletion, CheckKiCadDRC, "aaa_custom"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("sorted check %d = %q, want %q; all=%v", index, got[index], want[index], got)
		}
	}
}

func TestResultFinishSummarizesIssues(t *testing.T) {
	result := NewResult("board.kicad_pcb")
	result.AddCheck(Check{
		Name:     CheckPCBStructuralValidation,
		Required: true,
		Issues: []reports.Issue{{
			Code:     reports.CodeInvalidNetAssignment,
			Severity: reports.SeverityError,
			Nets:     []string{"VCC"},
			Message:  "bad net",
		}},
	})
	result.Finish()
	if result.Status != StatusFail {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.FabricationReady {
		t.Fatalf("FabricationReady = true, want false")
	}
	if result.Summary.BlockingIssues != 1 {
		t.Fatalf("blocking issues = %d, want 1", result.Summary.BlockingIssues)
	}
	if result.Summary.ByCode[string(reports.CodeInvalidNetAssignment)] != 1 {
		t.Fatalf("missing code summary: %#v", result.Summary.ByCode)
	}
	if result.Summary.ByNet["VCC"] != 1 {
		t.Fatalf("missing net summary: %#v", result.Summary.ByNet)
	}
}

func TestIssuesFromValidationErrors(t *testing.T) {
	errs := kicadfiles.ValidationErrors{
		{Section: "footprints.0.pads.1", Field: "net_name", Message: "must match net code"},
		{Section: "drawings", Field: "edge_cuts", Message: "closed outline required"},
	}
	issues := IssuesFromError(errs, "board.kicad_pcb")
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}
	if issues[0].Code != reports.CodeInvalidNetAssignment {
		t.Fatalf("issue[0].Code = %q, want invalid net assignment", issues[0].Code)
	}
	if issues[0].Path != "board.kicad_pcb.footprints.0.pads.1.net_name" {
		t.Fatalf("issue[0].Path = %q", issues[0].Path)
	}
	if issues[1].Code != reports.CodeMissingBoardOutline {
		t.Fatalf("issue[1].Code = %q, want missing board outline", issues[1].Code)
	}
}

func TestIssuesFromGenericError(t *testing.T) {
	issues := IssuesFromError(errors.New("boom"), "board.kicad_pcb")
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Code != reports.CodeValidationFailed {
		t.Fatalf("Code = %q, want validation failed", issues[0].Code)
	}
	if issues[0].Path != "board.kicad_pcb" {
		t.Fatalf("Path = %q, want board path", issues[0].Path)
	}
}
