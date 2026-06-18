package repair

import (
	"testing"

	"kicadai/internal/reports"
)

func TestBuildPlanChoosesExpectedActions(t *testing.T) {
	groups := []StageIssues{{
		Stage: "validation",
		Issues: []reports.Issue{
			{Code: reports.CodeMissingFootprint, Message: "missing footprint"},
			{Code: reports.CodeInvalidNetAssignment, Message: "bad net"},
			{Code: reports.CodeMissingBoardOutline, Message: "missing outline"},
			{Code: reports.CodePlacementCollision, Message: "collision"},
			{Code: reports.CodeValidationFailed, Message: "net GND is unrouted"},
		},
	}}
	plan := BuildPlan(groups, Options{
		Enabled:                  true,
		MaxAttempts:              10,
		MaxAttemptsPerIssue:      1,
		AllowFootprintAssignment: true,
		AllowPadNetRegeneration:  true,
		AllowOutlineGeneration:   true,
		AllowPlacementRetry:      true,
		AllowRoutingRetry:        true,
	})
	want := []Action{ActionAssignFootprint, ActionRegeneratePadNets, ActionGenerateOutline, ActionRetryPlacement, ActionRerouteNet}
	if len(plan.Attempts) != len(want) {
		t.Fatalf("attempts = %#v", plan.Attempts)
	}
	for index, action := range want {
		if plan.Attempts[index].Action != action {
			t.Fatalf("attempt %d action = %s, want %s", index, plan.Attempts[index].Action, action)
		}
		if !plan.Attempts[index].DryRun {
			t.Fatalf("attempt %d should be dry-run: %#v", index, plan.Attempts[index])
		}
		if plan.Attempts[index].Status != StatusPlanned {
			t.Fatalf("attempt %d status = %s, want %s", index, plan.Attempts[index].Status, StatusPlanned)
		}
	}
	if plan.Status != StatusPlanned || plan.Summary.PlannedCount != len(want) {
		t.Fatalf("plan status = %s summary = %#v", plan.Status, plan.Summary)
	}
}

func TestBuildPlanRespectsMaxAttempts(t *testing.T) {
	groups := []StageIssues{{Stage: "validation", Issues: []reports.Issue{
		{Code: reports.CodeMissingFootprint, Path: "a"},
		{Code: reports.CodeMissingFootprint, Path: "b"},
	}}}
	plan := BuildPlan(groups, Options{Enabled: true, MaxAttempts: 1, MaxAttemptsPerIssue: 1, AllowFootprintAssignment: true})
	if len(plan.Attempts) != 1 || plan.Summary.AttemptCount != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanRespectsMaxAttemptsPerIssue(t *testing.T) {
	issue := reports.Issue{Code: reports.CodeMissingFootprint, Path: "same", Message: "same"}
	groups := []StageIssues{{Stage: "validation", Issues: []reports.Issue{issue, issue}}}
	plan := BuildPlan(groups, Options{Enabled: true, MaxAttempts: 3, MaxAttemptsPerIssue: 1, AllowFootprintAssignment: true})
	if len(plan.Attempts) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanDisabledSkips(t *testing.T) {
	plan := BuildPlan([]StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeMissingFootprint}}}}, Options{})
	if plan.Status != StatusSkipped || len(plan.Attempts) != 0 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanUnsupportedBlocks(t *testing.T) {
	plan := BuildPlan([]StageIssues{{Stage: "roundtrip", Issues: []reports.Issue{{Code: reports.CodeRoundTripDiff, Message: "diff"}}}}, Options{Enabled: true})
	if plan.Status != StatusBlocked || len(plan.Attempts) != 1 || plan.Attempts[0].Action != ActionUnsupported {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanApplyMarksAttemptApplied(t *testing.T) {
	plan := BuildPlan([]StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeInvalidNetAssignment}}}}, Options{Enabled: true, Apply: true, MaxAttempts: 1, MaxAttemptsPerIssue: 1, AllowPadNetRegeneration: true})
	if len(plan.Attempts) != 1 || plan.Attempts[0].DryRun || plan.Summary.AppliedCount != 0 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanDisabledSpecificRepairSkips(t *testing.T) {
	plan := BuildPlan([]StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeInvalidNetAssignment}}}}, Options{Enabled: true, MaxAttempts: 1, MaxAttemptsPerIssue: 1})
	if plan.Status != StatusSkipped || len(plan.Attempts) != 1 || plan.Attempts[0].Status != StatusSkipped {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanMixedAttemptsIsPartial(t *testing.T) {
	plan := BuildPlan([]StageIssues{{Stage: "validation", Issues: []reports.Issue{
		{Code: reports.CodeMissingFootprint, Path: "U1"},
		{Code: reports.CodeInvalidNetAssignment, Path: "U1.1"},
	}}}, Options{Enabled: true, MaxAttempts: 3, MaxAttemptsPerIssue: 1, AllowFootprintAssignment: true})
	if plan.Status != StatusPartial || plan.Summary.PlannedCount != 1 || plan.Summary.SkippedCount != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildPlanNormalizesNonPositiveLimits(t *testing.T) {
	plan := BuildPlan([]StageIssues{{Stage: "validation", Issues: []reports.Issue{
		{Code: reports.CodeMissingFootprint, Path: "U1"},
		{Code: reports.CodeMissingFootprint, Path: "U2"},
	}}}, Options{Enabled: true, MaxAttempts: -1, MaxAttemptsPerIssue: -1, AllowFootprintAssignment: true})
	if len(plan.Attempts) != 2 {
		t.Fatalf("plan = %#v", plan)
	}
}
