package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCausalRepairEvidenceIsBoundedAndByteIdentical(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	requirement.Requirements.BehavioralRequirements[0].Min = graphFloat(1000)
	requirement.Requirements.BehavioralRequirements[0].Max = graphFloat(10_000)
	graph = seriesPathOnly(t, graph)
	policy := DefaultPolicy()
	policy.MaxTopologyRepairs = 32
	policy.MaxValueTrials = 64
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1024
	initial := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)

	first := RepairCandidate(context.Background(), requirement, graph, initial, inventory, environment, policy)
	second := RepairCandidate(context.Background(), requirement, graph, initial, inventory, environment, policy)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("causal repair replay differs")
	}
	if first.Status != RepairSearchPassed || len(first.CausalAnalyses) == 0 {
		t.Fatalf("causal repair status=%s analyses=%d", first.Status, len(first.CausalAnalyses))
	}
	for _, analysis := range first.CausalAnalyses {
		if err := validateCausalRepairAnalysis(analysis); err != nil {
			t.Fatalf("%v: budget=%#v consumption=%#v", err, analysis.Budget, analysis.Consumption)
		}
		for _, trial := range analysis.Trials {
			if trial.Hash == "" || len(trial.Perturbations) == 0 || len(trial.Effects) == 0 || trial.EvaluationHash == "" {
				t.Fatalf("incomplete causal trial: %#v", trial)
			}
			if trial.Authorized && len(trial.Regressions) != 0 {
				t.Fatalf("regressing trial was authorized: %#v", trial)
			}
		}
	}
}

func TestCausalRepairRejectsCriticalMarginRegression(t *testing.T) {
	requirement := Requirement{Requirements: Requirements{BehavioralRequirements: []BehavioralAssertion{
		{ID: "failed", Critical: false},
		{ID: "safety", Critical: true},
	}}}
	baseline := SimulationEvaluation{Attempts: []SimulationAttempt{
		{RequirementID: "failed", OperatingCase: "case", CornerID: "nominal", Analysis: "dc", Metric: "value", Actual: graphFloat(2), RequiredMax: graphFloat(1), AssertionPass: false},
		{RequirementID: "safety", OperatingCase: "case", CornerID: "hot", Analysis: "thermal", Metric: "temperature", Actual: graphFloat(80), RequiredMax: graphFloat(100), AssertionPass: true},
	}}
	trial := SimulationEvaluation{Attempts: []SimulationAttempt{
		{RequirementID: "failed", OperatingCase: "case", CornerID: "nominal", Analysis: "dc", Metric: "value", Actual: graphFloat(1), RequiredMax: graphFloat(1), AssertionPass: true},
		{RequirementID: "safety", OperatingCase: "case", CornerID: "hot", Analysis: "thermal", Metric: "temperature", Actual: graphFloat(90), RequiredMax: graphFloat(100), AssertionPass: true},
	}}
	_, regressions, baselineViolation, trialViolation := causalAssertionEffects(requirement, baseline, trial, 1)
	if baselineViolation <= trialViolation || len(regressions) == 0 {
		t.Fatalf("critical regression not retained: baseline=%g trial=%g regressions=%#v", baselineViolation, trialViolation, regressions)
	}
}

func TestCausalRepairTrialsCoverBiasAndCompensationOperators(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	requirement.Requirements.BehavioralRequirements[0].Min = graphFloat(1000)
	requirement.Requirements.BehavioralRequirements[0].Max = graphFloat(10_000)
	graph = seriesPathOnly(t, graph)
	policy := DefaultPolicy()
	policy.MaxTopologyRepairs = 32
	policy.MaxValueTrials = 64
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1024
	initial := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)
	if initial.Status != SimulationEvaluationFailed || len(initial.Diagnoses) == 0 {
		t.Fatalf("causal operator fixture status=%s diagnoses=%d", initial.Status, len(initial.Diagnoses))
	}

	for _, test := range []struct {
		name     string
		code     string
		operator string
	}{
		{name: "bias_reference", code: diagnosisNonconvergent, operator: "repair_bias_reference"},
		{name: "compensation", code: diagnosisUnstable, operator: "add_compensation_edge"},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnosed := initial
			diagnosed.Diagnoses = append([]Diagnosis(nil), initial.Diagnoses...)
			diagnosed.Diagnoses[0].Code = test.code
			analysis, _ := analyzeCausalRepairs(
				context.Background(), requirement, graph, diagnosed, inventory, environment, policy,
			)
			if err := validateCausalRepairAnalysis(analysis); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, trial := range analysis.Trials {
				if trial.Repair.Operator == test.operator && trial.EvaluationHash != "" && len(trial.Effects) != 0 {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("bounded causal trials lack operator %q", test.operator)
			}
		})
	}
}

func TestHeldOutFeedbackPolarityFailureRecoversCausally(t *testing.T) {
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
	repaired := RepairCandidate(context.Background(), requirement, faulted, initial, inventory, environment, policy)
	if repaired.Status != RepairSearchPassed || repaired.Selected == nil || repaired.Selected.Repair.Operator != "correct_feedback_polarity" {
		t.Fatalf("polarity repair status=%s selected=%#v", repaired.Status, repaired.Selected)
	}
	ratedTrial := false
	for _, analysis := range repaired.CausalAnalyses {
		for _, trial := range analysis.Trials {
			for _, perturbation := range trial.Perturbations {
				if perturbation.Kind == "substitute_rated_device" {
					ratedTrial = true
				}
			}
		}
	}
	if !ratedTrial {
		t.Fatal("bounded held-out causal analysis lacks a rated-device substitution trial")
	}
}

func TestHeldOutCoordinatedValueFailureRecoversCausally(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	policy := DefaultPolicy()
	policy.MaxTopologyRepairs = 32
	policy.MaxValueTrials = 64
	policy.MaxCandidateSimulations = 512
	policy.MaxCornerEvaluations = 2048
	for caseIndex := range requirement.Requirements.OperatingCases {
		for conditionIndex := range requirement.Requirements.OperatingCases[caseIndex].Conditions {
			condition := &requirement.Requirements.OperatingCases[caseIndex].Conditions[conditionIndex]
			midpoint := (condition.Min + condition.Max) / 2
			condition.Min = midpoint
			condition.Max = midpoint
		}
	}
	passing := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)
	if passing.Status != SimulationEvaluationPassed {
		t.Fatalf("held-out coordinated source status=%s", passing.Status)
	}

	plan := BuildValueSearchPlan(requirement, graph, inventory, policy)
	selections := []ValueTrialSelection{}
	wantedKinds := map[string]bool{"resistance": true, "capacitance": true}
	for _, domain := range plan.Domains {
		if !wantedKinds[domain.Quantity] {
			continue
		}
		instanceIndex := graphInstanceIndex(graph, domain.InstanceID)
		if instanceIndex < 0 || graph.Instances[instanceIndex].ValueSI == nil {
			continue
		}
		var selected *ComponentValueCandidate
		selectedMagnitude := 0.0
		for index := range domain.Candidates {
			candidate := &domain.Candidates[index]
			if candidate.ValueSI == nil || *candidate.ValueSI < *graph.Instances[instanceIndex].ValueSI*1.4 ||
				sameCausalValue(graph.Instances[instanceIndex], *candidate) {
				continue
			}
			magnitude := causalValueMagnitude(graph.Instances[instanceIndex].ValueSI, candidate.ValueSI, true)
			if selected == nil || magnitude < selectedMagnitude {
				selected = candidate
				selectedMagnitude = magnitude
			}
		}
		if selected == nil {
			continue
		}
		selections = append(selections, ValueTrialSelection{
			InstanceID: domain.InstanceID, PrimitiveKey: selected.PrimitiveKey,
			ValueSI: cloneInventoryFloat(selected.ValueSI), CandidateHash: selected.Hash,
		})
		delete(wantedKinds, domain.Quantity)
		if len(selections) == 2 {
			break
		}
	}
	if len(selections) != 2 {
		t.Fatalf("held-out coordinated fault selections=%#v", selections)
	}
	faultTrial := ValueTrial{Number: 1, Selections: selections}
	faultTrial.Hash = valueTrialHash(faultTrial.Selections)
	faulted, err := ApplyValueTrial(graph, faultTrial, inventory)
	if err != nil {
		t.Fatal(err)
	}
	initial := EvaluateCandidate(context.Background(), requirement, faulted, nil, inventory, environment, policy)
	if initial.Status != SimulationEvaluationFailed {
		t.Fatalf("coordinated perturbation status=%s, want failed", initial.Status)
	}
	for _, selection := range selections {
		singleRestored := CloneGraph(faulted)
		faultIndex := graphInstanceIndex(singleRestored, selection.InstanceID)
		originalIndex := graphInstanceIndex(graph, selection.InstanceID)
		if faultIndex < 0 || originalIndex < 0 {
			t.Fatalf("missing coordinated instance %q", selection.InstanceID)
		}
		singleRestored.Instances[faultIndex].PrimitiveKey = graph.Instances[originalIndex].PrimitiveKey
		singleRestored.Instances[faultIndex].ValueSI = cloneInventoryFloat(graph.Instances[originalIndex].ValueSI)
		singleRestored, err = NormalizeGraph(singleRestored)
		if err != nil {
			t.Fatal(err)
		}
		singleEvaluation := EvaluateCandidate(context.Background(), requirement, singleRestored, nil, inventory, environment, policy)
		if singleEvaluation.Status != SimulationEvaluationFailed {
			t.Fatalf("single restoration of %q status=%s, want failed", selection.InstanceID, singleEvaluation.Status)
		}
	}
	repaired := RepairCandidate(context.Background(), requirement, faulted, initial, inventory, environment, policy)
	if repaired.Status != RepairSearchPassed || repaired.Selected == nil {
		t.Fatalf("coordinated repair status=%s selected=%#v", repaired.Status, repaired.Selected)
	}
	coordinated := false
	for _, analysis := range repaired.CausalAnalyses {
		for _, trial := range analysis.Trials {
			if trial.Coordinated && trial.Authorized && trial.Status == SimulationEvaluationPassed && len(trial.Perturbations) == 2 {
				coordinated = true
			}
		}
	}
	if !coordinated {
		t.Fatalf("held-out recovery lacks a passing coordinated causal trial: selected=%#v", repaired.Selected)
	}
}

func firstPolarityFault(graph CandidateGraph, inventory PrimitiveInventory) (CandidateGraph, bool) {
	for _, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			continue
		}
		pairs := causalPolarityTerminalPairs(primitive)
		if len(pairs) == 0 {
			continue
		}
		candidate := CloneGraph(graph)
		index := graphInstanceIndex(candidate, instance.ID)
		left, right := -1, -1
		for terminalIndex, connection := range candidate.Instances[index].Terminals {
			if connection.Terminal == pairs[0][0] {
				left = terminalIndex
			}
			if connection.Terminal == pairs[0][1] {
				right = terminalIndex
			}
		}
		if left < 0 || right < 0 {
			continue
		}
		candidate.Instances[index].Terminals[left].Node, candidate.Instances[index].Terminals[right].Node =
			candidate.Instances[index].Terminals[right].Node, candidate.Instances[index].Terminals[left].Node
		candidate, err := NormalizeGraph(candidate)
		if err == nil {
			return candidate, true
		}
	}
	return graph, false
}
