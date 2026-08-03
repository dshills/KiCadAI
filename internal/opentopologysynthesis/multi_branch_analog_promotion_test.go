package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiBranchAnalogNeutralCorpusPromotion(t *testing.T) {
	if os.Getenv("KICADAI_OPEN_TOPOLOGY_PROMOTION") != "1" {
		t.Skip("set KICADAI_OPEN_TOPOLOGY_PROMOTION=1 to run the neutral multi-branch synthesis lane")
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 256
	policy.MaxTopologyRepairs = 32
	policy.MaxCandidateSimulations = 50_000
	policy.MaxCornerEvaluations = 200_000
	graphChangingRepairs := 0

	for _, file := range []string{
		"outside_window_supply_guard.json",
		"precision_low_voltage_rail.json",
	} {
		t.Run(file, func(t *testing.T) {
			requirement := testMultiBranchAnalogRequirement(t, file)
			first := Synthesize(context.Background(), requirement, inventory, environment, policy)
			if first.Report.Status != StatusPassed || first.Report.Selected == nil ||
				first.SelectedGraph == nil || first.Physical == nil ||
				first.Physical.Status != PhysicalLoweringReady {
				logNeutralSynthesisFailures(t, first)
				t.Fatalf(
					"neutral synthesis = status=%s stop=%s selected=%t physical=%#v diagnostics=%#v consumption=%#v",
					first.Report.Status,
					first.Report.StopReason,
					first.Report.Selected != nil,
					first.Physical,
					first.Report.Diagnostics,
					first.Report.Consumption,
				)
			}
			if len(first.Report.Diagnostics) != 0 {
				t.Fatalf("passing neutral synthesis retained diagnostics: %#v", first.Report.Diagnostics)
			}
			if !materiallyMultiBranchGraph(*first.SelectedGraph) {
				t.Fatalf("neutral synthesis selected a non-multi-branch graph: %s", testGraphTopologySummary(*first.SelectedGraph))
			}
			if selectedRepairChangesTopology(first.SelectedRepair) {
				graphChangingRepairs++
			}
			second := Synthesize(context.Background(), requirement, inventory, environment, policy)
			firstJSON, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			secondJSON, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("neutral multi-branch synthesis replay is not byte-identical")
			}
			assertSynthesisConsumptionMatchesEvidence(t, first)
		})
	}
	if graphChangingRepairs == 0 {
		t.Fatal("neutral corpus selected no graph-changing repair")
	}
}

func selectedRepairChangesTopology(repair *RepairSearchResult) bool {
	if repair == nil || repair.Selected == nil || repair.InitialGraphHash == repair.Selected.Evaluation.GraphHash {
		return false
	}
	for _, selected := range repair.Selected.Repairs {
		for _, change := range selected.Changes {
			switch change.Kind {
			case "add_primitive", "remove_primitive", "split_primitive", "connect_terminal", "disconnect_terminal":
				return true
			}
		}
	}
	return false
}

func logNeutralSynthesisFailures(t *testing.T, run SynthesisRun) {
	t.Helper()
	for candidateIndex, candidate := range run.Candidates {
		bestPenalty := math.Inf(1)
		bestEvaluation := -1
		for evaluationIndex, evaluation := range candidate.Evaluations {
			if penalty := simulationEvaluationPenalty(evaluation); penalty < bestPenalty {
				bestPenalty = penalty
				bestEvaluation = evaluationIndex
			}
		}
		if bestEvaluation >= 0 {
			t.Logf(
				"candidate=%d best_evaluation=%d penalty=%g diagnoses=%#v topology=%s values=%s",
				candidateIndex,
				bestEvaluation,
				bestPenalty,
				candidate.Evaluations[bestEvaluation].Diagnoses,
				testGraphTopologySummary(run.Search.Candidates[candidateIndex].Graph),
				testValueTrialSummary(candidate.ValuePlan, bestEvaluation),
			)
		}
		if candidate.Repair == nil {
			continue
		}
		if os.Getenv("KICADAI_OPEN_TOPOLOGY_DIAGNOSTIC") == "1" {
			for attemptIndex, attempt := range candidate.Repair.Attempts {
				t.Logf(
					"candidate=%d repair=%d penalty=%g operator=%s changes=%#v diagnoses=%#v",
					candidateIndex,
					attemptIndex,
					simulationEvaluationPenalty(attempt.Evaluation),
					attempt.Repair.Operator,
					attempt.Repair.Changes,
					attempt.Evaluation.Diagnoses,
				)
			}
		}
		bestPenalty = math.Inf(1)
		bestAttempt := -1
		for attemptIndex, attempt := range candidate.Repair.Attempts {
			if penalty := simulationEvaluationPenalty(attempt.Evaluation); penalty < bestPenalty {
				bestPenalty = penalty
				bestAttempt = attemptIndex
			}
		}
		if bestAttempt >= 0 {
			attempt := candidate.Repair.Attempts[bestAttempt]
			t.Logf(
				"candidate=%d best_repair=%d penalty=%g operator=%s changes=%#v diagnoses=%#v",
				candidateIndex,
				bestAttempt,
				bestPenalty,
				attempt.Repair.Operator,
				attempt.Repair.Changes,
				attempt.Evaluation.Diagnoses,
			)
		}
	}
}

func testMultiBranchAnalogRequirement(t *testing.T, file string) Requirement {
	t.Helper()
	path := filepath.Join("testdata", "multi_branch_analog_corpus", file)
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, path)))
	if len(issues) != 0 {
		t.Fatalf("neutral requirement decode issues: %#v", issues)
	}
	return requirement
}

func materiallyMultiBranchGraph(graph CandidateGraph) bool {
	if len(graph.Instances) < 4 || internalNodeCount(graph) < 2 {
		return false
	}
	degree := map[string]int{}
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			degree[terminal.Node]++
		}
	}
	for _, node := range graph.Nodes {
		if node.Scope == "internal" && degree[node.ID] >= 3 {
			return true
		}
	}
	return false
}
