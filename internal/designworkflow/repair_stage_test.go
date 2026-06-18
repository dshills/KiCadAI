package designworkflow

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/repair"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
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

func TestPersistedValidationRepairStageAppliesGeneratedTransaction(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := mustDesignWorkflowTransaction(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"set_board_outline","board":{"width_mm":40,"height_mm":25}},
	  {"op":"write_project"}
	]}`)
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output})
	if len(apply.Issues) != 0 {
		t.Fatalf("apply issues: %#v", apply.Issues)
	}
	request := Request{
		Name:  "demo",
		Board: BoardSpec{WidthMM: 40, HeightMM: 25},
	}
	stage := persistedValidationRepairStage(context.Background(), &request, ProjectWriteResult{Transaction: tx}, nil, CreateOptions{
		OutputDir: output,
		Overwrite: true,
		Repair:    repair.Options{Enabled: true, Apply: true},
	})
	if stage.Name != StageValidationRepair || stage.Status != StageStatusOK || stage.Summary["validation_count"] != 1 {
		t.Fatalf("stage = %#v", stage)
	}
}

func mustDesignWorkflowTransaction(t *testing.T, input string) transactions.Transaction {
	t.Helper()
	tx, err := transactions.Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return tx
}
