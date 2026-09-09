package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/simulationadmission"
)

func TestElectricalCertificateV22RejectsIncompleteOrTamperedEvidence(t *testing.T) {
	r, before, inventory, environment, admission := testControlBindingFixtureV22(t)
	prepared := simulationadmission.PrepareEnvironment(admission)
	batch, err := controlRebindingsV22(context.Background(), r, before, inventory, 16)
	if err != nil {
		t.Fatal(err)
	}
	var chosen controlRebindingV22
	var evaluation SimulationEvaluation
	for _, proposal := range batch.candidates {
		e := EvaluateCandidateV20(context.Background(), r, proposal.graph, nil, inventory, environment, admission, DefaultPolicy())
		if e.Status == SimulationEvaluationPassed {
			chosen, evaluation = proposal, e
			break
		}
	}
	if evaluation.Status != SimulationEvaluationPassed {
		t.Fatal("fixture has no electrical pass")
	}
	certificate, err := certifyElectricalRebindingV22(context.Background(), r, before, chosen.graph, inventory, environment, prepared, chosen.change, evaluation)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyElectricalCertificateV22(context.Background(), certificate, r, before, chosen.graph, inventory, environment, prepared, evaluation); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(evaluation)
	for name, mutate := range map[string]func(*SimulationEvaluation){
		"hash":             func(e *SimulationEvaluation) { e.Hash = "changed" },
		"missing corner":   func(e *SimulationEvaluation) { e.Attempts = e.Attempts[:1] },
		"duplicate corner": func(e *SimulationEvaluation) { e.Attempts = append(e.Attempts, e.Attempts[0]) },
		"model provenance": func(e *SimulationEvaluation) { e.Attempts[0].ModelEvidenceSHA256s = nil },
		"actual":           func(e *SimulationEvaluation) { e.Attempts[0].Actual = graphFloat(2) },
		"report":           func(e *SimulationEvaluation) { e.Attempts[0].Report = nil },
		"unrelated plan":   func(e *SimulationEvaluation) { e.Attempts[0].PlanHash = strings.Repeat("4", 64) },
		"report registry": func(e *SimulationEvaluation) {
			e.Attempts[0].Report.RegistryHash = strings.Repeat("5", 64)
			e.Attempts[0].ReportHash = causalCrossStageHash(*e.Attempts[0].Report)
		},
		"report catalog": func(e *SimulationEvaluation) {
			e.Attempts[0].Report.CatalogHash = strings.Repeat("6", 64)
			e.Attempts[0].ReportHash = causalCrossStageHash(*e.Attempts[0].Report)
		},
		"report devices": func(e *SimulationEvaluation) {
			e.Attempts[0].Report.Devices = nil
			e.Attempts[0].ReportHash = causalCrossStageHash(*e.Attempts[0].Report)
		},
		"claimed report value": func(e *SimulationEvaluation) {
			e.Attempts[0].Report.Assertions = nil
			e.Attempts[0].ReportHash = causalCrossStageHash(*e.Attempts[0].Report)
		},
		"different graph": func(e *SimulationEvaluation) { e.GraphHash = strings.Repeat("3", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			var e SimulationEvaluation
			if err := json.Unmarshal(encoded, &e); err != nil {
				t.Fatal(err)
			}
			mutate(&e)
			if name != "hash" {
				e.Hash = ""
				e.Hash = causalCrossStageHash(e)
			}
			if _, err := certifyElectricalRebindingV22(context.Background(), r, before, chosen.graph, inventory, environment, prepared, chosen.change, e); err == nil {
				t.Fatal("tampered or incomplete numerical evidence certified")
			}
		})
	}
	stale := certificate
	stale.Feedback = nil
	stale.Hash = ""
	stale.Hash = causalCrossStageHash(stale)
	if err := verifyElectricalCertificateV22(context.Background(), stale, r, before, chosen.graph, inventory, environment, prepared, evaluation); err == nil {
		t.Fatal("rehashed certificate with omitted feedback accepted")
	}
	wrongChange := chosen.change
	wrongChange.FromNode = "port_common"
	if _, err := certifyElectricalRebindingV22(context.Background(), r, before, chosen.graph, inventory, environment, prepared, wrongChange, evaluation); err == nil {
		t.Fatal("false operation provenance accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := certifyElectricalRebindingV22(ctx, r, before, chosen.graph, inventory, environment, prepared, chosen.change, evaluation); err == nil {
		t.Fatal("canceled certification succeeded")
	}
}

func TestControlRebindingV22ReplayBudgetAndCycleGuards(t *testing.T) {
	r, before, inventory, _, _ := testControlBindingFixtureV22(t)
	original, _ := GraphHash(before)
	first, err := controlRebindingsV22(context.Background(), r, before, inventory, 16)
	if err != nil {
		t.Fatal(err)
	}
	permuted := CloneGraph(before)
	slices.Reverse(permuted.Nodes)
	slices.Reverse(permuted.Instances)
	for i := range permuted.Instances {
		slices.Reverse(permuted.Instances[i].Terminals)
	}
	second, err := controlRebindingsV22(context.Background(), r, permuted, inventory, 16)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("binding replay depends on slice order")
	}
	if hash, _ := GraphHash(before); hash != original {
		t.Fatal("input graph mutated")
	}
	limited, err := controlRebindingsV22(context.Background(), r, before, inventory, 1)
	if err != nil || limited.work != 1 || !limited.exhausted {
		t.Fatalf("bound not enforced: %+v %v", limited, err)
	}
	if _, err := controlRebindingsV22(context.Background(), r, before, inventory, 0); err == nil {
		t.Fatal("zero work bound accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if batch, err := controlRebindingsV22(ctx, r, before, inventory, 16); err == nil || batch.work != 0 {
		t.Fatal("cancellation consumed work")
	}
	for _, proposal := range first.candidates {
		if len(proposal.bindings) == 0 {
			continue
		}
		if analyzeElectricalTopologyV22(r, proposal.graph, inventory, nil).Complete {
			t.Fatal("unexplained cycle accepted")
		}
		bad := slices.Clone(proposal.bindings)
		bad[0].GraphHash = "stale"
		if analyzeElectricalTopologyV22(r, proposal.graph, inventory, bad).Complete {
			t.Fatal("stale cycle binding accepted")
		}
		bad = append(slices.Clone(proposal.bindings), proposal.bindings[0])
		if analyzeElectricalTopologyV22(r, proposal.graph, inventory, bad).Complete {
			t.Fatal("duplicate cycle binding accepted")
		}
	}
}
