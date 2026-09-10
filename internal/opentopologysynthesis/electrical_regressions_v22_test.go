package opentopologysynthesis

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/simulationadmission"
)

func TestElectricalAdmissionV22DoesNotAliasAssertions(t *testing.T) {
	r, before, inventory, environment, admission := testMultiControlFixtureV22(t)
	after := CloneGraph(before)
	changes := []GraphChange{}
	for i := range after.Instances {
		output := ""
		for _, terminal := range after.Instances[i].Terminals {
			if terminal.Terminal == "OUT" {
				output = terminal.Node
			}
		}
		for j, terminal := range after.Instances[i].Terminals {
			if terminal.Terminal == "IN_MINUS" {
				changes = append(changes, GraphChange{Kind: "redirect_terminal", Primitive: after.Instances[i].ID, Terminal: terminal.Terminal, FromNode: terminal.Node, ToNode: output})
				after.Instances[i].Terminals[j].Node = output
			}
		}
	}
	ctx := context.Background()
	prepared := simulationadmission.PrepareEnvironment(admission)
	legacy := EvaluateCandidateV20(ctx, r, after, nil, inventory, environment, admission, DefaultPolicy())
	current := EvaluateElectricalCandidateV22(ctx, r, after, inventory, environment, admission, DefaultPolicy())
	if legacy.Status != SimulationEvaluationPassed || current.Status != SimulationEvaluationPassed {
		t.Fatal("comparison requires two numerical passes")
	}
	if _, err := certifyElectricalPathV22(ctx, r, before, after, inventory, environment, prepared, changes, legacy); err == nil {
		t.Fatal("legacy admission alias no longer reproduced; reassess the regression")
	}
	if _, err := certifyElectricalPathV22(ctx, r, before, after, inventory, environment, prepared, changes, current); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Attempts) != len(current.Attempts) {
		t.Fatal("admission-key fix changed numerical attempt coverage")
	}
	for i, attempt := range current.Attempts {
		old := legacy.Attempts[i]
		if old.PlanHash != attempt.PlanHash || old.ReportHash != attempt.ReportHash || !sameBoundV22(old.Actual, attempt.Actual) {
			t.Fatal("admission-key fix changed numerical evidence")
		}
	}
	slices.Reverse(r.Requirements.BehavioralRequirements)
	replay := EvaluateElectricalCandidateV22(ctx, r, after, inventory, environment, admission, DefaultPolicy())
	if replay.Hash != current.Hash {
		t.Fatal("assertion ordering changes admission/evaluation identity")
	}
}

func TestElectricalControlPathV22RejectsNonControlAndCrossDomainEdits(t *testing.T) {
	_, graph, inventory, _, _ := testControlBindingFixtureV22(t)
	base := GraphChange{Kind: "redirect_terminal", Primitive: "primitive_000", Terminal: "IN_MINUS", FromNode: "port_stimulus", ToNode: "port_reading"}
	for _, category := range []string{"output", "supply", "value", "unknown_terminal", "unknown_role", "cross_domain", "missing_source"} {
		t.Run(category, func(t *testing.T) {
			before := CloneGraph(graph)
			inv := inventory
			change := base
			switch category {
			case "output":
				change.Terminal, change.FromNode = "OUT", "port_reading"
				change.ToNode = "port_stimulus"
			case "supply":
				change.Terminal, change.FromNode = "V_PLUS", "port_rail"
			case "value":
				change.ToValue = graphFloat(1)
			case "unknown_terminal":
				change.Terminal = "UNDECLARED"
			case "missing_source":
				change.FromNode = "missing"
			case "cross_domain":
				for i := range before.Nodes {
					if before.Nodes[i].ID == "port_reading" {
						before.Nodes[i].Domain = "isolated_reference"
					}
				}
			case "unknown_role":
				inv.Primitives = slices.Clone(inventory.Primitives)
				for i := range inv.Primitives {
					if inv.Primitives[i].Key == before.Instances[0].PrimitiveKey {
						inv.Primitives[i].Terminals = slices.Clone(inv.Primitives[i].Terminals)
						for j := range inv.Primitives[i].Terminals {
							if inv.Primitives[i].Terminals[j].Terminal == change.Terminal {
								inv.Primitives[i].Terminals[j].Electrical = ""
							}
						}
					}
				}
			}
			if _, err := applyControlChangeV22(before, inv, change); err == nil {
				t.Fatal("unsupported control edit accepted")
			}
		})
	}
}

func TestElectricalRepairV22CompoundInvocationBudgets(t *testing.T) {
	r, graph, inventory, environment, admission := testMultiControlFixtureV22(t)
	ctx := context.Background()
	initial := EvaluateCandidateV20(ctx, r, graph, nil, inventory, environment, admission, DefaultPolicy())
	for maximum := 1; maximum <= 8; maximum++ {
		limits := DefaultElectricalRepairLimitsV22()
		limits.MaxEvaluations = maximum
		result := RepairElectricalV22(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), limits)
		if result.Consumption.CandidateSimulations > maximum {
			t.Fatalf("limit=%d consumed=%d", maximum, result.Consumption.CandidateSimulations)
		}
	}
}
