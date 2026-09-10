package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/simulationadmission"
)

// The extra midpoint requirement distinguishes linear feedback from an
// open-loop rail transition with the same endpoint slope. This is independent
// public test evidence, not an evaluation-corpus requirement.
func testRepairFixtureV23(t *testing.T) (Requirement, CandidateGraph, PrimitiveInventory, SimulationEnvironment, simulationadmission.Environment) {
	t.Helper()
	r, before, _, _, inventory, environment, admission := testElectricalFixtureV23(t, true)
	r.Requirements.OperatingCases = append(r.Requirements.OperatingCases, OperatingCase{ID: "midpoint", Conditions: []OperatingCondition{{Axis: "supply_voltage", Target: "rail", Min: 12, Max: 12, Unit: "V"}, {Axis: "input_voltage", Target: "stimulus", Min: 6, Max: 6, Unit: "V"}}})
	r.Requirements.BehavioralRequirements = append(r.Requirements.BehavioralRequirements, BehavioralAssertion{ID: "tracking", Metric: "output_voltage", Analysis: "dc_operating_point", Observation: Observation{Kind: "port", ID: "reading"}, Min: graphFloat(5.99), Max: graphFloat(6.01), Unit: "V", OperatingCases: []string{"midpoint"}})
	r = Normalize(r)
	if issues := Validate(r); len(issues) != 0 {
		t.Fatal(issues)
	}
	return r, before, inventory, environment, admission
}

func TestElectricalRepairV23RecoversAndCertifiesBoundedSweep(t *testing.T) {
	r, graph, inventory, environment, admission := testRepairFixtureV23(t)
	ctx := context.Background()
	initial := EvaluateElectricalCandidateV22(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	if initial.Status != SimulationEvaluationFailed {
		t.Fatalf("initial status=%s", initial.Status)
	}
	prior := RepairElectricalV22(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if prior.Status == RepairSearchPassed {
		t.Fatal("independent sweep does not isolate historical nonconvergence")
	}
	result := RepairElectricalV23(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if result.Status != RepairSearchPassed || result.Selected == nil {
		t.Fatalf("repair failed: %s; trials=%+v", result.StopReason, result.Trials)
	}
	if err := VerifyElectricalRepairSelectionV23(ctx, result, r, graph, inventory, environment, admission); err != nil {
		t.Fatal(err)
	}
	if len(result.Selected.Certificate.PrerequisiteProof.Changes) != 1 {
		t.Fatal("independent feedback required unexpected edits")
	}
	permuted := CloneGraph(graph)
	slices.Reverse(permuted.Instances)
	slices.Reverse(permuted.Nodes)
	replay := RepairElectricalV23(ctx, r, permuted, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if !reflect.DeepEqual(result, replay) {
		t.Fatal("V23 repair or execution ledger is nondeterministic")
	}
	t.Logf("historical=%s; V23=%s; trials=%d calls=%d corners=%d", prior.Status, result.Status, len(result.Trials), result.Consumption.CandidateSimulations, result.Consumption.CornerEvaluations)
}

func TestElectricalRepairV23PreservesHistoricalCompoundSearch(t *testing.T) {
	r, graph, inventory, environment, admission := testMultiControlFixtureV22(t)
	ctx := context.Background()
	initial := EvaluateElectricalCandidateV22(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	prior := RepairElectricalV22(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	result := RepairElectricalV23(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if prior.Status != RepairSearchPassed || result.Status != prior.Status || result.Selected == nil || !reflect.DeepEqual(prior.Consumption, result.Consumption) || prior.BindingWork != result.BindingWork || len(prior.Trials) != len(result.Trials) {
		t.Fatal("historical compound repair work or outcome changed")
	}
	for i, trial := range result.Trials {
		if !reflect.DeepEqual(prior.Trials[i], trial.ElectricalRepairTrialV22) {
			t.Fatalf("historical trial %d changed", i)
		}
	}
	if !reflect.DeepEqual(prior.Selected.Graph, result.Selected.Graph) || !reflect.DeepEqual(prior.Selected.Evaluation, result.Selected.Evaluation.Evaluation) {
		t.Fatal("historical selected graph or numerical result changed")
	}
	if err := VerifyElectricalRepairSelectionV23(ctx, result, r, graph, inventory, environment, admission); err != nil {
		t.Fatal(err)
	}
}

func TestElectricalRepairV23RejectsTamperedLedger(t *testing.T) {
	r, graph, inventory, environment, admission := testRepairFixtureV23(t)
	ctx := context.Background()
	initial := EvaluateElectricalCandidateV22(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	result := RepairElectricalV23(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if result.Selected == nil {
		t.Fatal("missing independent selection")
	}
	differentRequirement := cloneRequirement(r)
	differentRequirement.Project.Description += " Changed requirement identity."
	if VerifyElectricalRepairSelectionV23(ctx, result, differentRequirement, graph, inventory, environment, admission) == nil {
		t.Fatal("selection verified against a different requirement")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ElectricalRepairResultV23){
		"solver":        func(r *ElectricalRepairResultV23) { r.SolverPolicyID = "historical" },
		"solver hash":   func(r *ElectricalRepairResultV23) { r.SolverPolicyHash = "wrong" },
		"missing trial": func(r *ElectricalRepairResultV23) { r.Trials = nil },
		"execution":     func(r *ElectricalRepairResultV23) { r.Trials[len(r.Trials)-1].ExecutionHash = r.InitialEvaluationHash },
		"number":        func(r *ElectricalRepairResultV23) { r.Trials[0].Number++ },
		"reordered":     func(r *ElectricalRepairResultV23) { slices.Reverse(r.Trials) },
		"false parent": func(r *ElectricalRepairResultV23) {
			r.Trials[len(r.Trials)-1].ParentGraphHash = r.InitialEvaluationHash
		},
		"count":              func(r *ElectricalRepairResultV23) { r.Trials[0].SimulationCalls++ },
		"negative count":     func(r *ElectricalRepairResultV23) { r.Trials[0].CornerEvaluations = -1 },
		"negative binding":   func(r *ElectricalRepairResultV23) { r.BindingWork = -1 },
		"depth":              func(r *ElectricalRepairResultV23) { r.Limits.MaxDepth = 0 },
		"failed final trial": func(r *ElectricalRepairResultV23) { r.Trials[len(r.Trials)-1].Continuable = false },
		"empty path":         func(r *ElectricalRepairResultV23) { r.Selected.Certificate.PrerequisiteProof.Changes = nil },
		"missing solver evidence": func(r *ElectricalRepairResultV23) {
			r.Selected.Evaluation.SolverAttempts = nil
			r.Selected.Evaluation = finalizeElectricalEvaluationV23(r.Selected.Evaluation)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var altered ElectricalRepairResultV23
			if err := json.Unmarshal(data, &altered); err != nil {
				t.Fatal(err)
			}
			mutate(&altered)
			altered.Hash = ""
			altered.Hash = causalCrossStageHash(altered)
			if VerifyElectricalRepairSelectionV23(ctx, altered, r, graph, inventory, environment, admission) == nil {
				t.Fatal("rehashed repair forgery accepted")
			}
		})
	}
}

func TestElectricalRepairV23PreservesCriticalAndWorkGuards(t *testing.T) {
	r, graph, inventory, environment, admission := testRepairFixtureV23(t)
	ctx := context.Background()
	initial := EvaluateElectricalCandidateV22(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	limits := DefaultElectricalRepairLimitsV22()
	limits.MaxEvaluations, limits.MaxCorners = 1, 1
	exhausted := RepairElectricalV23(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), limits)
	if exhausted.Selected != nil || exhausted.Consumption.CandidateSimulations > 1 || exhausted.Consumption.CornerEvaluations > 1 || !exhausted.Consumption.BudgetExhausted {
		t.Fatal("explicit work budget bypassed")
	}
	r.Requirements.BehavioralRequirements[0].Critical = true
	initial = EvaluateElectricalCandidateV22(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	guarded := RepairElectricalV23(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if guarded.StopReason != "critical_failure" || guarded.BindingWork != 0 || len(guarded.Trials) != 0 || guarded.Consumption.CandidateSimulations != 0 {
		t.Fatalf("critical guard bypassed: %+v", guarded)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	stopped := RepairElectricalV23(canceled, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if stopped.Status != RepairSearchCanceled || stopped.BindingWork != 0 {
		t.Fatal("cancellation bypassed")
	}
	initial.Hash = "tampered"
	invalid := RepairElectricalV23(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if invalid.StopReason != "invalid_initial_evidence" || invalid.BindingWork != 0 {
		t.Fatal("initial provenance bypassed")
	}
}
