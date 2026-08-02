package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"kicadai/internal/repairloop"
)

func TestHeldOutPolarityFailureRecoversThroughCrossStageCoordinator(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		architectureGeneralizationCorpusRoot(), "low_current_voltage_converter.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 32
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384
	passing := Synthesize(context.Background(), requirement, inventory, environment, policy)
	if passing.Report.Status != StatusPassed || passing.SelectedGraph == nil {
		t.Fatalf("held-out source did not pass: status=%s", passing.Report.Status)
	}

	faulted, found := firstPolarityFault(*passing.SelectedGraph, inventory)
	if !found {
		t.Fatal("held-out source has no polarity-bearing primitive")
	}
	initial := EvaluateCandidate(context.Background(), requirement, faulted, nil, inventory, environment, policy)
	if initial.Status != SimulationEvaluationFailed {
		t.Fatalf("polarity perturbation status=%s, want failed", initial.Status)
	}

	run := func() repairloop.CrossStageReport {
		target, err := NewCausalCrossStageTarget(CausalCrossStageTargetOptions{
			Requirement: requirement, Graph: faulted, Evaluation: initial,
			Inventory: inventory, Environment: environment, Policy: policy,
		})
		if err != nil {
			t.Fatal(err)
		}
		coordinatorPolicy := repairloop.DefaultCrossStagePolicy()
		coordinatorPolicy.MaxTrials = 48
		coordinatorPolicy.MaxTrialsPerDiagnostic = 48
		report, err := repairloop.RunCrossStageRepair(context.Background(), target, coordinatorPolicy)
		if err != nil {
			t.Fatal(err)
		}
		if err := repairloop.ValidateCrossStageReport(report); err != nil {
			t.Fatal(err)
		}
		return report
	}

	first := run()
	second := run()
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("cross-stage causal repair replay differs")
	}
	if first.Status != repairloop.CrossStageStatusPassed || first.StopReason != repairloop.CrossStageStopPassed {
		t.Fatalf("cross-stage status=%s stop=%s trials=%#v", first.Status, first.StopReason, first.Trials)
	}
	if first.Consumption.CommittedRepairs != 1 || first.Consumption.ConfirmationAttempts != 1 {
		t.Fatalf("unexpected cross-stage consumption: %#v", first.Consumption)
	}
	selected := false
	for _, trial := range first.Trials {
		if !trial.Selected {
			continue
		}
		selected = true
		if !trial.Confirmed || trial.Proposal.Operator != "correct_feedback_polarity" ||
			trial.Proposal.ReenterStage != repairloop.CrossStageSynthesis {
			t.Fatalf("unexpected selected trial: %#v", trial)
		}
	}
	if !selected {
		t.Fatal("cross-stage report lacks a selected repair")
	}
}
