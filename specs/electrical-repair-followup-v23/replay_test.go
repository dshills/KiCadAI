package electricalfollowup

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	diag "kicadai/internal/electricaldiagnostics"
	ot "kicadai/internal/opentopologysynthesis"
)

type trialDiagnostic struct {
	Trial      ot.ElectricalRepairTrialV22 `json:"trial"`
	Policy     ot.Policy                   `json:"policy"`
	Evaluation diag.Evaluation             `json:"evaluation"`
}

type evaluateFunc func(context.Context, ot.CandidateGraph, ot.Policy) ot.SimulationEvaluation

// replayRecorded never enumerates or selects graphs. The caller authenticates
// the publication and seed before passing their retained, immutable records.
func replayRecorded(ctx context.Context, seed ot.CandidateGraph, repair ot.ElectricalRepairResultV22, policy ot.Policy, evaluate evaluateFunc) ([]trialDiagnostic, error) {
	unsigned := repair
	unsigned.Hash = ""
	if digest(unsigned) != repair.Hash || repair.Schema != "kicadai.electrical-repair.v22" || repair.Selected != nil {
		return nil, fmt.Errorf("invalid or selected repair evidence")
	}
	if repair.Limits != ot.DefaultElectricalRepairLimitsV22() || policy != ot.DefaultPolicy() {
		return nil, fmt.Errorf("diagnostic limits differ from V22")
	}
	seedHash, err := ot.GraphHash(seed)
	if err != nil || seedHash != repair.InitialGraphHash {
		return nil, fmt.Errorf("initial graph identity differs")
	}
	graphs := map[string]ot.CandidateGraph{seedHash: seed}
	depths := map[string]int{seedHash: 0}
	proposals := make([]ot.CandidateGraph, len(repair.Trials))
	// Authenticate every reconstruction before executing any numerical work.
	for i, trial := range repair.Trials {
		parent, ok := graphs[trial.ParentGraphHash]
		if !ok || trial.Number != i+1 || trial.Depth != depths[trial.ParentGraphHash]+1 || trial.Depth > repair.Limits.MaxDepth {
			return nil, fmt.Errorf("trial %d has invalid parent, order, or depth", i+1)
		}
		graph, err := redirectRecorded(parent, trial.Change)
		if err != nil {
			return nil, fmt.Errorf("trial %d: %w", i+1, err)
		}
		graphHash, err := ot.GraphHash(graph)
		if err != nil || graphHash != trial.GraphHash {
			return nil, fmt.Errorf("trial %d graph identity differs", i+1)
		}
		proposals[i] = graph
		if trial.Continuable {
			graphs[graphHash], depths[graphHash] = graph, trial.Depth
		}
	}
	results := make([]trialDiagnostic, 0, len(repair.Trials))
	simulations, corners := 0, 0
	for i, trial := range repair.Trials {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		remaining := policy
		remaining.MaxCandidateSimulations = min(repair.Limits.MaxEvaluations, policy.MaxCandidateSimulations) - simulations
		remaining.MaxCornerEvaluations = min(repair.Limits.MaxCorners, policy.MaxCornerEvaluations) - corners
		if remaining.MaxCandidateSimulations <= 0 || remaining.MaxCornerEvaluations <= 0 {
			return results, fmt.Errorf("trial %d exceeds frozen budget", i+1)
		}
		evaluation := evaluate(ctx, proposals[i], remaining)
		if err := ctx.Err(); err != nil {
			return results, err
		}
		if evaluation.Hash != trial.EvaluationHash || evaluation.GraphHash != trial.GraphHash || evaluation.RequirementHash != repair.RequirementHash || evaluation.InventoryHash != repair.InventoryHash || evaluation.Status != trial.Status || evaluation.ValueTrialHash != "" {
			return results, fmt.Errorf("trial %d evaluation identity differs: got %s, want %s", i+1, evaluation.Hash, trial.EvaluationHash)
		}
		passing := 0
		for _, attempt := range evaluation.Attempts {
			if attempt.Status == ot.SimulationEvaluationPassed && attempt.AssertionPass {
				passing++
			}
		}
		if passing != trial.PassingAssertions {
			return results, fmt.Errorf("trial %d passing-attempt count differs", i+1)
		}
		projection, err := diag.ProjectEvaluation(evaluation)
		if err != nil {
			return results, fmt.Errorf("trial %d: %w", i+1, err)
		}
		simulations += max(1, evaluation.Consumption.CandidateSimulations)
		corners += evaluation.Consumption.CornerEvaluations
		results = append(results, trialDiagnostic{Trial: trial, Policy: remaining, Evaluation: projection})
	}
	if simulations != repair.Consumption.CandidateSimulations || corners != repair.Consumption.CornerEvaluations || len(results) != repair.Consumption.TopologyRepairs {
		return results, fmt.Errorf("aggregate numerical consumption differs")
	}
	return results, nil
}

func redirectRecorded(parent ot.CandidateGraph, change ot.GraphChange) (ot.CandidateGraph, error) {
	if change.Kind != "redirect_terminal" || change.FromNode == change.ToNode || change.FromValue != nil || change.ToValue != nil {
		return ot.CandidateGraph{}, fmt.Errorf("record is not one terminal redirect")
	}
	graph := ot.CloneGraph(parent)
	destination, count := false, 0
	for _, node := range graph.Nodes {
		destination = destination || node.ID == change.ToNode
	}
	for i := range graph.Instances {
		instance := &graph.Instances[i]
		if instance.ID != change.Primitive {
			continue
		}
		for j := range instance.Terminals {
			terminal := &instance.Terminals[j]
			if terminal.Terminal != change.Terminal {
				continue
			}
			if terminal.Node != change.FromNode {
				return ot.CandidateGraph{}, fmt.Errorf("recorded source terminal differs")
			}
			terminal.Node = change.ToNode
			count++
		}
	}
	if !destination || count != 1 {
		return ot.CandidateGraph{}, fmt.Errorf("redirect has missing or ambiguous endpoint")
	}
	return ot.NormalizeGraph(graph)
}

func TestRecordedReplayAuthenticationAndBudget(t *testing.T) {
	seed := ot.CandidateGraph{Schema: ot.CandidateGraphSchema, Version: ot.CandidateGraphVersion,
		Nodes:     []ot.GraphNode{{ID: "port_a", Scope: "external", SemanticKind: "port", SemanticID: "a"}, {ID: "port_b", Scope: "external", SemanticKind: "port", SemanticID: "b"}, {ID: "port_c", Scope: "external", SemanticKind: "port", SemanticID: "c"}},
		Instances: []ot.GraphInstance{{ID: "primitive_000", PrimitiveKey: "independent", Kind: "opamp", Terminals: []ot.TerminalConnection{{Terminal: "input", Node: "port_a"}}}}}
	repair := ot.ElectricalRepairResultV22{Schema: "kicadai.electrical-repair.v22", RequirementHash: "requirement", InventoryHash: "inventory", Limits: ot.DefaultElectricalRepairLimitsV22()}
	repair.InitialGraphHash, _ = ot.GraphHash(seed)
	current := seed
	evaluations := []ot.SimulationEvaluation{}
	for i, pair := range [][2]string{{"port_a", "port_b"}, {"port_b", "port_c"}} {
		change := ot.GraphChange{Kind: "redirect_terminal", Primitive: "primitive_000", Terminal: "input", FromNode: pair[0], ToNode: pair[1]}
		parentHash, _ := ot.GraphHash(current)
		var err error
		current, err = redirectRecorded(current, change)
		if err != nil {
			t.Fatal(err)
		}
		graphHash, _ := ot.GraphHash(current)
		evaluation := ot.SimulationEvaluation{Schema: ot.SimulationEvaluationSchema, Version: ot.SimulationEvaluationVersion, RequirementHash: repair.RequirementHash, InventoryHash: repair.InventoryHash, GraphHash: graphHash, Status: ot.SimulationEvaluationFailed, Consumption: ot.Consumption{CandidateSimulations: 2, CornerEvaluations: 3}}
		evaluation.Hash = digest(evaluation)
		evaluations = append(evaluations, evaluation)
		repair.Trials = append(repair.Trials, ot.ElectricalRepairTrialV22{Number: i + 1, Depth: i + 1, ParentGraphHash: parentHash, GraphHash: graphHash, EvaluationHash: evaluation.Hash, Change: change, Status: evaluation.Status, Continuable: true})
	}
	repair.Consumption = ot.Consumption{CandidateSimulations: 4, CornerEvaluations: 6, TopologyRepairs: 2}
	repair.Hash = digest(repair)
	before, _ := json.Marshal(seed)
	for range 2 {
		calls := 0
		result, err := replayRecorded(context.Background(), seed, repair, ot.DefaultPolicy(), func(_ context.Context, _ ot.CandidateGraph, policy ot.Policy) ot.SimulationEvaluation {
			if policy.MaxCandidateSimulations != 128-calls*2 || policy.MaxCornerEvaluations != 4096-calls*3 {
				t.Fatal("remaining budget differs")
			}
			result := evaluations[calls]
			calls++
			return result
		})
		if err != nil || len(result) != 2 || calls != 2 {
			t.Fatalf("replay: %v", err)
		}
	}
	after, _ := json.Marshal(seed)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("source graph mutated")
	}
	for _, change := range []func(*ot.ElectricalRepairResultV22){
		func(r *ot.ElectricalRepairResultV22) { r.Hash = "tampered" },
		func(r *ot.ElectricalRepairResultV22) {
			r.Trials[1].ParentGraphHash = "missing"
			r.Hash = ""
			r.Hash = digest(*r)
		},
		func(r *ot.ElectricalRepairResultV22) {
			r.Trials[0].Change.FromNode = "missing"
			r.Hash = ""
			r.Hash = digest(*r)
		},
		func(r *ot.ElectricalRepairResultV22) {
			r.Trials[1].GraphHash = "tampered"
			r.Hash = ""
			r.Hash = digest(*r)
		},
	} {
		encoded, _ := json.Marshal(repair)
		var bad ot.ElectricalRepairResultV22
		if err := json.Unmarshal(encoded, &bad); err != nil {
			t.Fatal(err)
		}
		change(&bad)
		_, err := replayRecorded(context.Background(), seed, bad, ot.DefaultPolicy(), func(context.Context, ot.CandidateGraph, ot.Policy) ot.SimulationEvaluation {
			t.Fatal("numerical execution before authentication")
			return ot.SimulationEvaluation{}
		})
		if err == nil {
			t.Fatal("accepted altered evidence")
		}
	}
	for _, scenario := range []string{"evaluation_hash", "evaluation_content", "passing_count", "consumption", "cancellation"} {
		t.Run(scenario, func(t *testing.T) {
			encoded, _ := json.Marshal(repair)
			var copy ot.ElectricalRepairResultV22
			if err := json.Unmarshal(encoded, &copy); err != nil {
				t.Fatal(err)
			}
			if scenario == "passing_count" {
				copy.Trials[0].PassingAssertions++
			}
			if scenario == "consumption" {
				copy.Consumption.CornerEvaluations++
			}
			copy.Hash = ""
			copy.Hash = digest(copy)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancellation" {
				cancel()
			}
			calls := 0
			_, err := replayRecorded(ctx, seed, copy, ot.DefaultPolicy(), func(context.Context, ot.CandidateGraph, ot.Policy) ot.SimulationEvaluation {
				if scenario == "cancellation" {
					t.Fatal("evaluated canceled replay")
				}
				evaluation := evaluations[calls]
				calls++
				if scenario == "evaluation_hash" {
					evaluation.Hash = "different"
				}
				if scenario == "evaluation_content" {
					evaluation.Consumption.CornerEvaluations++
				}
				return evaluation
			})
			if err == nil {
				t.Fatal("accepted diagnostic mismatch")
			}
		})
	}
}
