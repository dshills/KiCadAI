package opentopologysynthesis

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrequencySelectiveRequirementsGenerateBalancedBridgeArchitectures(t *testing.T) {
	requirement := testArchitectureRequirement(t, "mains_notch_filter.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	if len(search.Candidates) == 0 {
		t.Fatalf("no frequency-selective candidates: status=%s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
	}
	bridgeCandidates := 0
	opAmpCounts := map[int]bool{}
	for _, candidate := range search.Candidates {
		resistors, capacitors, opAmps := 0, 0, 0
		for _, instance := range candidate.Graph.Instances {
			switch instance.Kind {
			case "resistor":
				resistors++
			case "capacitor":
				capacitors++
			case "opamp":
				opAmps++
			}
		}
		if resistors < 4 || capacitors < 4 || opAmps == 0 {
			continue
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, DefaultPolicy())
		if plan.Status != ValuePlanReady ||
			!planHasAnalyticScaleID(plan, "topology:frequency_selective:balanced_bridge_resistance") ||
			!planHasAnalyticScaleID(plan, "topology:frequency_selective:balanced_bridge_capacitance") {
			continue
		}
		bridgeCandidates++
		opAmpCounts[opAmps] = true
	}
	if bridgeCandidates < 2 || !opAmpCounts[1] || !opAmpCounts[2] {
		t.Fatalf("balanced bridge candidates=%d opamp_counts=%v, want output-buffered and input/output-buffered alternatives; retained=%d rejections=%#v", bridgeCandidates, opAmpCounts, len(search.Candidates), search.Rejections)
	}
}

func planHasAnalyticScaleID(plan ValueSearchPlan, id string) bool {
	for _, domain := range plan.Domains {
		for _, scale := range domain.AnalyticScales {
			if scale.ID == id {
				return true
			}
		}
	}
	return false
}

func TestFrequencySelectiveHeldOutCandidateReachesTrustedSimulation(t *testing.T) {
	requirement := testArchitectureRequirement(t, "mains_notch_filter.json")
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	var selected *TopologyCandidate
	for index := range search.Candidates {
		opAmps, resistors, capacitors := 0, 0, 0
		for _, instance := range search.Candidates[index].Graph.Instances {
			switch instance.Kind {
			case "opamp":
				opAmps++
			case "resistor":
				resistors++
			case "capacitor":
				capacitors++
			}
		}
		if opAmps != 2 || resistors < 4 || capacitors < 4 {
			continue
		}
		selected = &search.Candidates[index]
		break
	}
	if selected == nil {
		t.Fatalf("no dual-buffer frequency-selective candidate: %#v", search.Rejections)
	}
	plan := BuildValueSearchPlan(requirement, selected.Graph, inventory, policy)
	if plan.Status != ValuePlanReady {
		t.Fatalf("value plan status=%s issues=%#v rejections=%#v", plan.Status, plan.Issues, plan.Rejections)
	}
	trials := EnumerateValueTrials(plan, 1).Trials
	if len(trials) != 1 {
		t.Fatalf("first analytic trial count=%d", len(trials))
	}
	assertValueTrialHasExplainableComponents(t, plan, trials[0])
	appliedGraph, err := ApplyValueTrial(selected.Graph, trials[0], inventory)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := EvaluateCandidate(
		context.Background(), requirement, appliedGraph, nil,
		inventory, environment, policy,
	)
	if evaluation.Status != SimulationEvaluationPassed {
		attempts := []string{}
		for _, attempt := range evaluation.Attempts {
			actual := ""
			if attempt.Actual != nil {
				actual = fmt.Sprintf("(%.9g)", *attempt.Actual)
			}
			attempts = append(attempts, attempt.RequirementID+"="+string(attempt.Status)+actual)
		}
		t.Fatalf("frequency-selective analytic trial status=%s attempts=%s diagnoses=%s", evaluation.Status, strings.Join(attempts, "; "), diagnosisSummary(evaluation.Diagnoses))
	}
	assertEvaluationCoversRequirementAnalyses(t, requirement, evaluation)
	physical := LowerPassingCandidate(
		context.Background(), requirement, appliedGraph, evaluation, inventory, environment,
	)
	if physical.Status != PhysicalLoweringReady || physical.Hash == "" {
		t.Fatalf("frequency-selective physical lowering status=%s issues=%#v", physical.Status, physical.Issues)
	}
}

func testArchitectureRequirement(t *testing.T, file string) Requirement {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(architectureCorpusRoot(), file))
	if err != nil {
		t.Fatal(err)
	}
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("decode issues: %#v", issues)
	}
	return requirement
}

func diagnosisSummary(diagnoses []Diagnosis) string {
	parts := make([]string, 0, len(diagnoses))
	for _, diagnosis := range diagnoses {
		parts = append(parts, diagnosis.RequirementID+"="+diagnosis.Code+":"+diagnosis.Message)
	}
	return strings.Join(parts, ",")
}
