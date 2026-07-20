package closedloopsynthesis

import (
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

func TestBuildAnalysisPlanBindsBehaviorExpandsCornersAndReplays(t *testing.T) {
	requirement := closedLoopTestRequirement()
	bindings := []SemanticBinding{{Kind: "port", ID: "output", Target: "OUT"}, {Kind: "domain", ID: "supply", Target: "VCC"}}
	decisions := closedLoopModelDecisions()
	plan, diagnostics := BuildAnalysisPlan(requirement, bindings, decisions)
	if len(diagnostics) != 0 {
		t.Fatalf("analysis plan diagnostics: %#v", diagnostics)
	}
	if plan.PlanHash == "" || len(plan.Analyses) != 2 || len(plan.Assertions) != 2 || len(plan.Corners) != 2 {
		t.Fatalf("analysis plan = %#v", plan)
	}
	if plan.Assertions[0].Target != "OUT" || plan.Corners[0].Assignments[0].Target != "VCC" {
		t.Fatalf("semantic bindings were not preserved: %#v", plan)
	}
	reorderedBindings := append([]SemanticBinding(nil), bindings...)
	slices.Reverse(reorderedBindings)
	reorderedDecisions := append([]ModelDecision(nil), decisions...)
	slices.Reverse(reorderedDecisions)
	replayed, diagnostics := BuildAnalysisPlan(requirement, reorderedBindings, reorderedDecisions)
	if len(diagnostics) != 0 || replayed.PlanHash != plan.PlanHash {
		t.Fatalf("reordered plan = %#v diagnostics %#v", replayed, diagnostics)
	}
}

func TestBuildAnalysisPlanFailsClosedForMissingBindingAndTemperatureDomain(t *testing.T) {
	requirement := closedLoopTestRequirement()
	bindings := []SemanticBinding{{Kind: "domain", ID: "supply", Target: "VCC"}}
	if _, diagnostics := BuildAnalysisPlan(requirement, bindings, closedLoopModelDecisions()); len(diagnostics) == 0 {
		t.Fatal("missing behavioral observation binding was accepted")
	}
	minimum, maximum := -40.0, 85.0
	requirement.Requirements.OperatingCases[0].Conditions = append(requirement.Requirements.OperatingCases[0].Conditions, architecturesearch.OperatingCondition{Axis: "ambient_temperature", Target: "circuit", Min: &minimum, Max: &maximum, Unit: "degC"})
	if _, diagnostics := BuildAnalysisPlan(requirement, []SemanticBinding{{Kind: "port", ID: "output", Target: "OUT"}, {Kind: "domain", ID: "supply", Target: "VCC"}}, closedLoopModelDecisions()); len(diagnostics) == 0 {
		t.Fatal("model without reviewed temperature applicability was accepted")
	}
}

func closedLoopModelDecisions() []ModelDecision {
	return []ModelDecision{{
		Component: "r1", Family: "resistor", Claim: simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1},
		Provenance: &simmodel.ModelProvenance{Source: "manufacturer:test", Revision: "a", SHA256: testHash("model"), ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal}},
		Status:     "used", Reason: "trusted behavioral model", RequiredAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal},
	}}
}
