package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestSynthesizeRetainsDeterministicBoundedEvidenceAcrossTopologies(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 1_000
	policy.MaxGeneratedGraphs = 5_000
	policy.MaxRetainedCandidates = 4
	policy.MaxValueTrials = 4
	policy.MaxTopologyRepairs = 2
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1_024

	first := Synthesize(context.Background(), requirement, inventory, environment, policy)
	if len(first.Hash) != 64 || first.Report.PolicyHash == "" ||
		first.Report.RequirementHash == "" || len(first.Search.Candidates) < 2 ||
		len(first.Candidates) != len(first.Search.Candidates) {
		t.Fatalf("incomplete synthesis evidence: status=%s search=%d evidence=%d report=%#v",
			first.Report.Status, len(first.Search.Candidates), len(first.Candidates), first.Report)
	}
	evaluatedCandidates := 0
	for index, candidate := range first.Candidates {
		if candidate.Fingerprint != first.Search.Candidates[index].Fingerprint ||
			candidate.TopologyHash != first.Search.Candidates[index].TopologyHash ||
			candidate.ValuePlan.GraphHash != first.Search.Candidates[index].Fingerprint {
			t.Fatalf("candidate evidence is not search-bound at %d: %#v", index, candidate)
		}
		if len(candidate.Evaluations) != 0 {
			evaluatedCandidates++
		}
	}
	if evaluatedCandidates < 2 {
		t.Fatalf("value evaluation did not cover distinct retained topologies: %d", evaluatedCandidates)
	}
	if first.Report.Consumption.ValueTrials > policy.MaxValueTrials ||
		first.Report.Consumption.TopologyRepairs > policy.MaxTopologyRepairs ||
		first.Report.Consumption.CandidateSimulations > policy.MaxCandidateSimulations ||
		first.Report.Consumption.CornerEvaluations > policy.MaxCornerEvaluations {
		t.Fatalf("synthesis exceeded explicit budgets: %#v", first.Report.Consumption)
	}
	assertSynthesisConsumptionMatchesEvidence(t, first)

	second := Synthesize(context.Background(), requirement, inventory, environment, policy)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("synthesis replay differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func assertSynthesisConsumptionMatchesEvidence(t *testing.T, run SynthesisRun) {
	t.Helper()
	expected := run.Search.Consumption
	for _, candidate := range run.Candidates {
		expected.ValueTrials += len(candidate.Evaluations)
		for _, evaluation := range candidate.Evaluations {
			expected.CandidateSimulations += evaluation.Consumption.CandidateSimulations
			expected.CornerEvaluations += evaluation.Consumption.CornerEvaluations
		}
		if candidate.Repair != nil {
			expected.CandidateSimulations += candidate.Repair.Consumption.CandidateSimulations
			expected.CornerEvaluations += candidate.Repair.Consumption.CornerEvaluations
			expected.ValueTrials += candidate.Repair.Consumption.ValueTrials
			expected.TopologyRepairs += candidate.Repair.Consumption.TopologyRepairs
			expected.MaximumFrontier = max(
				expected.MaximumFrontier,
				candidate.Repair.Consumption.MaximumFrontier,
			)
			expected.BudgetExhausted =
				expected.BudgetExhausted || candidate.Repair.Consumption.BudgetExhausted
		}
	}
	// Exhaustion can be raised by the synthesis scheduler when it observes that
	// the next unit of work cannot start; that scheduler decision is not a
	// retained child-consumption field.
	expected.BudgetExhausted = run.Report.Consumption.BudgetExhausted
	if run.Report.Consumption != expected {
		t.Fatalf(
			"synthesis consumption does not match retained evidence:\nreport=%#v\nexpected=%#v",
			run.Report.Consumption,
			expected,
		)
	}
}

func TestSynthesizeFailsClosedBeforeSearchAndHonorsCancellation(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	invalid := requirement
	invalid.Schema = "not-a-schema"
	result := Synthesize(context.Background(), invalid, inventory, environment, DefaultPolicy())
	if result.Report.Status != StatusInvalid ||
		result.Report.StopReason != StopRequirementInvalid ||
		len(result.Report.Diagnostics) == 0 || result.Search.Schema != "" ||
		result.SelectedGraph != nil || result.Physical != nil {
		t.Fatalf("invalid synthesis result = %#v", result)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = Synthesize(ctx, requirement, inventory, environment, DefaultPolicy())
	if result.Report.Status != StatusCanceled ||
		result.Report.StopReason != StopCanceled ||
		len(result.Report.Diagnostics) != 1 ||
		result.Report.Diagnostics[0].Code != CodeCanceled ||
		result.SelectedGraph != nil || result.Physical != nil {
		t.Fatalf("canceled synthesis result = %#v", result)
	}
}

func TestFinalizeSynthesisRunRejectsIncompletePassingSelection(t *testing.T) {
	result := finalizeSynthesisRun(SynthesisRun{
		Schema:  SynthesisRunSchema,
		Version: SynthesisRunVersion,
		Report: Report{
			Schema: ReportSchema, Version: ReportVersion,
			Status: StatusPassed, StopReason: StopPassed,
		},
	})
	if result.Report.Status != StatusFailed ||
		result.Report.StopReason != StopNoPassingGraph ||
		len(result.Report.Diagnostics) != 1 ||
		result.Report.Diagnostics[0].Code != CodeNoPassingGraph ||
		result.SelectedGraph != nil || result.Physical != nil ||
		result.Hash == "" {
		t.Fatalf("incomplete passing selection was not rejected: %#v", result)
	}
}
