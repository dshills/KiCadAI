package repair

import (
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestRunnerNotNeededWhenInitialValidationPasses(t *testing.T) {
	result := NewRunner(Options{Enabled: true}, NewExecutor(ExecutionContext{}), nil).Run(nil)
	if result.Status != StatusNotNeeded || len(result.Attempts) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunnerReportsRepairedAfterValidatorClearsIssue(t *testing.T) {
	tx := transactions.Transaction{}
	runner := NewRunner(Options{Enabled: true, AllowFootprintAssignment: true}, NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints:  map[string]FootprintEvidence{"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true}},
	}), ValidatorFunc(func() []reports.Issue { return nil }))
	result := runner.Run([]StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}}}}})
	if result.Status != StatusRepaired || len(result.Attempts) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunnerAttemptAwareValidatorCanClearCurrentIssueWhenMultipleRemain(t *testing.T) {
	tx := transactions.Transaction{}
	runner := NewRunner(Options{Enabled: true, AllowFootprintAssignment: true, AllowOutlineGeneration: true}, NewExecutor(ExecutionContext{
		Transaction: &tx,
		Board:       &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
	}), issueClearingValidator{})
	issues := []reports.Issue{
		{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
		{Code: reports.CodeMissingBoardOutline, Message: "missing outline"},
	}
	result := runner.Run([]StageIssues{{Stage: "validation", Issues: issues}})
	if result.Status != StatusRepaired || len(result.Attempts) != 2 {
		t.Fatalf("result = %#v", result)
	}
}

type issueClearingValidator struct{}

func (issueClearingValidator) Validate() []reports.Issue {
	return nil
}

func (issueClearingValidator) ValidateAttempt(attempt Attempt, issues []reports.Issue) []reports.Issue {
	return removeAttemptedIssue(issues, attempt.Issue)
}

func TestRunnerBlocksAppliedRepairWithoutValidator(t *testing.T) {
	tx := transactions.Transaction{}
	result := NewRunner(Options{Enabled: true, AllowFootprintAssignment: true}, NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints:  map[string]FootprintEvidence{"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true}},
	}), nil).Run([]StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}}}}})
	if result.Status != StatusBlocked {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunnerBlocksWhenIssueRepeats(t *testing.T) {
	issue := reports.Issue{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}}
	tx := transactions.Transaction{}
	runner := NewRunner(Options{Enabled: true, AllowFootprintAssignment: true}, NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints:  map[string]FootprintEvidence{"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true}},
	}), ValidatorFunc(func() []reports.Issue { return []reports.Issue{issue} }))
	result := runner.Run([]StageIssues{{Stage: "validation", Issues: []reports.Issue{issue}}})
	if result.Status != StatusBlocked || len(result.FinalIssues) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunnerBlocksWhenIssueCountWorsens(t *testing.T) {
	tx := transactions.Transaction{}
	runner := NewRunner(Options{Enabled: true, AllowFootprintAssignment: true}, NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints:  map[string]FootprintEvidence{"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true}},
	}), ValidatorFunc(func() []reports.Issue {
		return []reports.Issue{{Code: reports.CodeMissingFootprint}, {Code: reports.CodeInvalidNetAssignment}}
	}))
	result := runner.Run([]StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}}}}})
	if result.Status != StatusBlocked || len(result.FinalIssues) != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunnerHonorsAttemptBudget(t *testing.T) {
	tx := transactions.Transaction{}
	result := NewRunner(Options{Enabled: true, MaxAttempts: 1, AllowFootprintAssignment: true}, NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
			"R2": {Ref: "R2", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
	}), ValidatorFunc(func() []reports.Issue {
		return []reports.Issue{{Code: reports.CodeMissingFootprint, Refs: []string{"R2"}}}
	})).Run([]StageIssues{{Stage: "validation", Issues: []reports.Issue{
		{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
		{Code: reports.CodeMissingFootprint, Refs: []string{"R2"}},
	}}})
	if len(result.Attempts) != 1 {
		t.Fatalf("result = %#v", result)
	}
}
