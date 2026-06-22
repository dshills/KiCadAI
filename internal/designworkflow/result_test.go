package designworkflow

import (
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestStageStatusForIssues(t *testing.T) {
	if got := StageStatusForIssues(nil); got != StageStatusOK {
		t.Fatalf("empty status = %q", got)
	}
	if got := StageStatusForIssues([]reports.Issue{{Severity: reports.SeverityWarning}}); got != StageStatusWarning {
		t.Fatalf("warning status = %q", got)
	}
	if got := StageStatusForIssues([]reports.Issue{{Severity: reports.SeverityError}}); got != StageStatusBlocked {
		t.Fatalf("error status = %q", got)
	}
}

func TestBuildWorkflowResultComputesFeedback(t *testing.T) {
	result := BuildWorkflowResult(ProjectSummary{Name: "demo"}, AcceptanceConnectivity, []StageResult{
		NewStageResult(StageSchematic, nil),
		NewStageResult(StagePlacement, []reports.Issue{{
			Code:        reports.CodePlacementOutsideBoard,
			Severity:    reports.SeverityError,
			Message:     "outside board",
			Refs:        []string{"U1"},
			Suggestion:  "increase board width",
			OperationID: "op-1",
		}}),
	})
	if result.Acceptance.Achieved != AcceptanceStructural {
		t.Fatalf("achieved = %q", result.Acceptance.Achieved)
	}
	if result.Feedback.Summary.BlockingCount != 1 || len(result.Feedback.Repairs) != 1 {
		t.Fatalf("feedback = %#v", result.Feedback)
	}
	repair := result.Feedback.Repairs[0]
	if repair.RetryScope != RetryScopePlacement || repair.OperationID != "op-1" || repair.Refs[0] != "U1" {
		t.Fatalf("repair = %#v", repair)
	}
}

func TestAchievedAcceptanceWithKiCadChecks(t *testing.T) {
	result := BuildWorkflowResult(ProjectSummary{Name: "demo"}, AcceptanceERCDRC, []StageResult{
		NewStageResult(StageSchematic, nil),
		NewStageResult(StagePCBRealization, nil),
		NewStageResult(StagePlacement, nil),
		NewStageResult(StageRouting, nil),
		NewStageResult(StageValidation, nil),
		NewStageResult(StageKiCadChecks, nil),
	})
	if result.Acceptance.Achieved != AcceptanceERCDRC {
		t.Fatalf("achieved = %q", result.Acceptance.Achieved)
	}
	if result.Acceptance.FabricationReady {
		t.Fatal("erc-drc should not imply fabrication candidate")
	}
}

func TestAchievedAcceptanceAllowsFabricationCandidate(t *testing.T) {
	result := BuildWorkflowResult(ProjectSummary{Name: "demo"}, AcceptanceFabricationCandidate, []StageResult{
		NewStageResult(StageSchematic, nil),
		NewStageResult(StagePCBRealization, nil),
		NewStageResult(StagePlacement, nil),
		NewStageResult(StageRouting, nil),
		NewStageResult(StageValidation, nil),
		NewStageResult(StageKiCadChecks, nil),
		NewStageResult(StageFabricationReady, nil),
	})
	if result.Acceptance.Achieved != AcceptanceFabricationCandidate || !result.Acceptance.FabricationReady {
		t.Fatalf("acceptance = %#v", result.Acceptance)
	}
}

func TestAchievedAcceptanceBlocksFabricationCandidateWithoutReadiness(t *testing.T) {
	result := BuildWorkflowResult(ProjectSummary{Name: "demo"}, AcceptanceFabricationCandidate, []StageResult{
		NewStageResult(StageSchematic, nil),
		NewStageResult(StagePCBRealization, nil),
		NewStageResult(StagePlacement, nil),
		NewStageResult(StageRouting, nil),
		NewStageResult(StageValidation, nil),
		NewStageResult(StageKiCadChecks, nil),
		NewStageResult(StageFabricationReady, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Message:  "missing fabrication evidence",
		}}),
	})
	if result.Acceptance.Achieved != AcceptanceERCDRC || result.Acceptance.FabricationReady {
		t.Fatalf("acceptance = %#v", result.Acceptance)
	}
}

func TestRetryScopeForExternalCheckIssue(t *testing.T) {
	scope := RetryScopeForStage(StageKiCadChecks, reports.Issue{Code: reports.CodeSkippedExternalTool})
	if scope != RetryScopeExternal {
		t.Fatalf("scope = %q", scope)
	}
}

func TestBuildWorkflowResultClonesIssues(t *testing.T) {
	issue := reports.Issue{Severity: reports.SeverityWarning, Refs: []string{"R1"}}
	stage := NewStageResult(StageRouting, []reports.Issue{issue})
	result := BuildWorkflowResult(ProjectSummary{Name: "demo"}, AcceptanceDraft, []StageResult{stage})
	result.Stages[0].Issues[0].Refs[0] = "mutated"
	if stage.Issues[0].Refs[0] != "R1" {
		t.Fatalf("stage issue was mutated: %#v", stage.Issues[0])
	}
}

func TestFeedbackRepairActionsByIssueCode(t *testing.T) {
	cases := []struct {
		name  string
		stage StageName
		issue reports.Issue
		want  string
	}{
		{name: "unknown block", stage: StageBlockPlanning, issue: reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Message: "missing"}, want: "select a supported circuit block"},
		{name: "missing footprint", stage: StagePCBRealization, issue: reports.Issue{Code: reports.CodeMissingFootprint, Severity: reports.SeverityError, Message: "missing"}, want: "assign a KiCad-resolvable footprint"},
		{name: "placement", stage: StagePlacement, issue: reports.Issue{Code: reports.CodePlacementOutsideBoard, Severity: reports.SeverityError, Message: "outside"}, want: "increase board size"},
		{name: "connectivity", stage: StageValidation, issue: reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Message: "bad net"}, want: "repair net-to-pad assignments"},
		{name: "external", stage: StageKiCadChecks, issue: reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityBlocked, Message: "missing cli"}, want: "provide kicad-cli"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feedback := BuildFeedback([]StageResult{{Name: tc.stage, Status: StageStatusBlocked, Issues: []reports.Issue{tc.issue}}})
			if len(feedback.Repairs) != 1 {
				t.Fatalf("repairs = %#v", feedback.Repairs)
			}
			if !strings.Contains(feedback.Repairs[0].SuggestedAction, tc.want) {
				t.Fatalf("action = %q, want %q", feedback.Repairs[0].SuggestedAction, tc.want)
			}
		})
	}
}
