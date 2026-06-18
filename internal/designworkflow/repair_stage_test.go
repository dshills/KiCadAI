package designworkflow

import (
	"testing"

	"kicadai/internal/repair"
	"kicadai/internal/reports"
)

func TestValidationRepairStagePlansKnownIssues(t *testing.T) {
	stage := validationRepairStage([]repair.StageIssues{{Stage: string(StageValidation), Issues: []reports.Issue{{
		Code: reports.CodeMissingBoardOutline, Message: "missing outline",
	}}}}, repair.Options{Enabled: true, AllowOutlineGeneration: true})
	if stage.Name != StageValidationRepair || stage.Status != StageStatusWarning {
		t.Fatalf("stage = %#v", stage)
	}
	if stage.Summary["planned_count"] != 1 {
		t.Fatalf("summary = %#v", stage.Summary)
	}
}

func TestValidationRepairStageReportsBlockedIssues(t *testing.T) {
	stage := validationRepairStage([]repair.StageIssues{{Stage: string(StageValidation), Issues: []reports.Issue{{
		Code: reports.CodeRoundTripDiff, Message: "diff",
	}}}}, repair.Options{Enabled: true})
	if stage.Status != StageStatusBlocked || len(stage.Issues) != 1 {
		t.Fatalf("stage = %#v", stage)
	}
}
