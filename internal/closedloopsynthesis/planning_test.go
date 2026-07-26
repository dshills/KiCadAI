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

func TestConditionAssignmentsKeepsCompleteUncertaintySelectionsAtomic(t *testing.T) {
	for _, axis := range []string{"tolerance", "model_parameter"} {
		assignments := conditionAssignments(architecturesearch.OperatingCondition{Axis: axis, Target: "circuit", Selection: "all"}, "circuit")
		if len(assignments) != 1 || assignments[0].Selection != "all" {
			t.Fatalf("%s assignments = %#v; want one complete expansion", axis, assignments)
		}
	}
}

func TestOperatingConditionTargetAcceptsRegisteredWorstCaseAggregates(t *testing.T) {
	for _, test := range []struct {
		axis   string
		target string
	}{
		{axis: "tolerance", target: "all_passives"},
		{axis: "model_parameter", target: "all_components"},
	} {
		resolved, ok := operatingConditionTarget(architecturesearch.OperatingCondition{
			Axis: test.axis, Target: test.target, Selection: "minimum_nominal_maximum",
		}, nil)
		if !ok || resolved != test.target {
			t.Fatalf("%s/%s target = %q, %t", test.axis, test.target, resolved, ok)
		}
	}
}

func TestBuildAnalysisPlanBindsV5EventsAndCoversEveryCaseAnalysis(t *testing.T) {
	requirement := closedLoopTestRequirement()
	requirement.Schema = architecturesearch.SchemaIDV5
	requirement.Version = architecturesearch.VersionV5
	requirement.Acceptance.RequireHierarchicalDecomposition = true
	requirement.Acceptance.RequireInterfaceContracts = true
	requirement.Acceptance.RequireSharedResourcePlanning = true
	requirement.Acceptance.RequireDeterministicBacktracking = true
	requirement.Acceptance.RequirePhysicalPartitioning = true
	requirement.Acceptance.RequireEndToEndTraceability = true
	requirement.Acceptance.RequireDynamicModelProvenance = true
	requirement.Acceptance.RequireReturnRatioEvidence = true
	requirement.Acceptance.RequireDynamicElectrothermalEvidence = true
	requirement.Acceptance.RequireEventCoverage = true
	requirement.Acceptance.RequireDynamicArchitectureSelection = true
	requirement.Acceptance.RequireBoundedDynamicRepair = true
	initial, applied, recovered := 0.0, 1.0, 0.0
	requirement.Requirements.OperatingCases[0].Events = []architecturesearch.OperatingEvent{{
		ID: "load_step", Kind: "load_step",
		Target:       architecturesearch.Observation{Kind: "port", ID: "output"},
		TriggerTimeS: 1e-4, DurationS: 2e-4,
		Initial: &initial, Applied: &applied, Recovered: &recovered, Unit: "A",
	}}
	maximumRecovery := 1e-3
	requirement.Requirements.BehavioralRequirements = append(requirement.Requirements.BehavioralRequirements, architecturesearch.BehavioralRequirement{
		ID: "load_step_recovery", Metric: "protection_response_time", Analysis: simmodel.AnalysisTransient,
		Observation: architecturesearch.Observation{Kind: "event", ID: "load_step"},
		Max:         &maximumRecovery, Unit: "s", OperatingCases: []string{"rated"}, Critical: true,
	})

	bindings := []SemanticBinding{{Kind: "port", ID: "output", Target: "OUT"}, {Kind: "domain", ID: "supply", Target: "VCC"}}
	decisions := closedLoopModelDecisions()
	decisions[0].RequiredAnalyses = []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal, simmodel.AnalysisTransient}
	decisions[0].Provenance.AllowedAnalyses = []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal, simmodel.AnalysisTransient}
	plan, diagnostics := BuildAnalysisPlan(requirement, bindings, decisions)
	if len(diagnostics) != 0 {
		t.Fatalf("V5 analysis plan diagnostics: %#v", diagnostics)
	}
	if len(plan.Events) != 1 || plan.Events[0].ID != "load_step" || plan.Events[0].Target != "OUT" || plan.Events[0].Applied != 1 {
		t.Fatalf("planned events = %#v", plan.Events)
	}
	eventAnalysisCount := 0
	for _, analysis := range plan.Analyses {
		if slices.Equal(analysis.Events, []string{"load_step"}) {
			eventAnalysisCount++
		} else if len(analysis.Events) != 0 {
			t.Fatalf("analysis %s event coverage = %#v", analysis.ID, analysis.Events)
		}
	}
	if eventAnalysisCount != 1 {
		t.Fatalf("event-specific analysis count = %d in %#v", eventAnalysisCount, plan.Analyses)
	}
	foundEventAssertion := false
	for _, assertion := range plan.Assertions {
		if assertion.RequirementID == "load_step_recovery" {
			foundEventAssertion = assertion.Target == "event:load_step"
		}
	}
	if !foundEventAssertion {
		t.Fatalf("event assertion was not bound: %#v", plan.Assertions)
	}
	replay, diagnostics := BuildAnalysisPlan(requirement, bindings, decisions)
	if len(diagnostics) != 0 || replay.PlanHash != plan.PlanHash {
		t.Fatalf("V5 event plan replay differs: %#v diagnostics=%#v", replay, diagnostics)
	}
}

func closedLoopModelDecisions() []ModelDecision {
	return []ModelDecision{{
		Component: "r1", Family: "resistor", Claim: simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1},
		Provenance: &simmodel.ModelProvenance{Source: "manufacturer:test", Revision: "a", SHA256: testHash("model"), ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal}},
		Status:     "used", Reason: "trusted behavioral model", RequiredAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal},
	}}
}
