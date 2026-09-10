package simmodel

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func activeStatePlanV23(t *testing.T, input float64, extra ...ComponentEvidence) Plan {
	t.Helper()
	components := []ComponentEvidence{
		voltageSourceEvidence("power", "VCC", "GND"),
		voltageSourceEvidence("stimulus", "IN", "GND"),
		opAmpEvidence("amplifier", "IN", "OUT", "OUT", "VCC", "GND", .2, .2),
		resistorEvidence("termination", 22000, "OUT", "GND"),
	}
	components = append(components, extra...)
	return centeredBiasTestPlan(t, components,
		[]NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VCC"}},
		[]SourceExcitation{{Component: "power", DCValue: 9}, {Component: "stimulus", DCValue: input}},
	)
}

func initialStateSystemV23(t *testing.T, plan Plan) (mnaSystem, []complex128) {
	t.Helper()
	system, diagnostics := buildMNASystem(plan, plan.Analyses[0], 0)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	solution, diagnostic := solveMNA(system)
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	return system, solution
}

func TestActiveStateV23RecoversFiniteGainWithoutMutatingHistory(t *testing.T) {
	plan := activeStatePlanV23(t, 2.4)
	system, solution := initialStateSystemV23(t, plan)
	before, _ := json.Marshal(plan)
	for _, rail := range []float64{.2, 8.8} {
		initial := map[string]float64{"amplifier": rail}
		var first []complex128
		for replay := 0; replay < 2; replay++ {
			after, result, states, recovery, diagnostics := solveBoundedOpAmpDCFromStateV23(plan, plan.Analyses[0], system, solution, initial)
			if len(diagnostics) != 0 || len(states) != 0 || !recovery.attempted || !recovery.accepted || recovery.maximumAdditionalSolves != 11 {
				t.Fatalf("recovery = %#v, states %v, diagnostics %v", recovery, states, diagnostics)
			}
			want := 2.4 * 100000 / 100001
			if actual := real(solvedNodeVoltage(after, result, "OUT")); math.Abs(actual-want) > 1e-10 {
				t.Fatalf("finite-gain output %g, want %g", actual, want)
			}
			if replay == 0 {
				first = append([]complex128(nil), result...)
			} else if !reflect.DeepEqual(first, result) {
				t.Fatal("nondeterministic solver output")
			}
			if initial["amplifier"] != rail || len(initial) != 1 {
				t.Fatal("initial clamp state mutated")
			}
		}
		_, _, _, oldDiagnostics := solveBoundedOpAmpDCFromState(plan, plan.Analyses[0], system, solution, initial)
		if !activeStateNonconvergenceV23(oldDiagnostics) {
			t.Fatal("historical solver behavior changed")
		}
	}
	after, _ := json.Marshal(plan)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("plan/model evidence mutated")
	}
}

func TestActiveStateV23ChainedAndResistiveFeedback(t *testing.T) {
	for _, resistive := range []bool{false, true} {
		components := []ComponentEvidence{voltageSourceEvidence("power", "VCC", "GND"), voltageSourceEvidence("stimulus", "IN", "GND"), opAmpEvidence("first", "IN", "MID", "MID", "VCC", "GND", .2, .2)}
		nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "MID"}, {Name: "OUT"}, {Name: "VCC"}}
		want := .6 * 100000 / 100001 * 100000 / 100001
		if resistive {
			nodes = append(nodes, NodeEvidence{Name: "FB"})
			components = append(components, opAmpEvidence("second", "MID", "FB", "OUT", "VCC", "GND", .2, .2), resistorEvidence("feedback", 24000, "OUT", "FB"), resistorEvidence("bottom", 8000, "FB", "GND"))
			want = .6 * 100000 / 100001 * 100000 / 25001
		} else {
			components = append(components, opAmpEvidence("second", "MID", "OUT", "OUT", "VCC", "GND", .2, .2))
		}
		components = append(components, resistorEvidence("termination", 22000, "OUT", "GND"))
		plan := centeredBiasTestPlan(t, components, nodes, []SourceExcitation{{Component: "power", DCValue: 9}, {Component: "stimulus", DCValue: .6}})
		system, solution := initialStateSystemV23(t, plan)
		for _, initial := range []map[string]float64{{"first": .2, "second": .2}, {"first": 8.8, "second": 8.8}, {"first": .2, "second": 8.8}} {
			system, solution, _, recovery, diagnostics := solveBoundedOpAmpDCFromStateV23(plan, plan.Analyses[0], system, solution, initial)
			if len(diagnostics) != 0 || !recovery.accepted || recovery.maximumAdditionalSolves != 18 {
				t.Fatalf("resistive=%v, recovery=%#v, diagnostics=%v", resistive, recovery, diagnostics)
			}
			if actual := real(solvedNodeVoltage(system, solution, "OUT")); math.Abs(actual-want) > 1e-9 {
				t.Fatalf("output %g, want %g", actual, want)
			}
		}
	}
}

func TestActiveStateV23PreservesComparatorSeed(t *testing.T) {
	for _, comparatorState := range []float64{0, 1} {
		plus, minus := "IN", "GND"
		if comparatorState == 1 {
			plus, minus = minus, plus
		}
		components := []ComponentEvidence{voltageSourceEvidence("power", "VCC", "GND"), voltageSourceEvidence("stimulus", "IN", "GND"), opAmpEvidence("amplifier", "IN", "OUT", "OUT", "VCC", "GND", .2, .2), resistorEvidence("pullup", 18000, "VCC", "LOGIC"),
			{InstanceID: "comparator", CatalogID: "independent.comparator", Family: "comparator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveComparatorOpenCollectorV1, Parameters: comparatorParameters(200e-9)}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: plus}, {Function: "IN_MINUS", Net: minus}, {Function: "OUT", Net: "LOGIC"}, {Function: "V_PLUS", Net: "VCC"}, {Function: "V_MINUS", Net: "GND"}}}}
		intent := Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "power", DCValue: 9}, {Component: "stimulus", DCValue: 2.4}}}}, Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: 2, Max: 3}}}
		plan, resolveDiagnostics := ResolveWithTopology(intent, "independent", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "LOGIC"}, {Name: "VCC"}})
		if len(resolveDiagnostics) != 0 {
			t.Fatal(resolveDiagnostics)
		}
		system, solution := initialStateSystemV23(t, plan)
		initial := map[string]float64{"amplifier": .2, "comparator": comparatorState}
		_, _, states, recovery, diagnostics := solveBoundedOpAmpDCFromStateV23(plan, plan.Analyses[0], system, solution, initial)
		if len(diagnostics) != 0 || !recovery.accepted || len(states) != 1 || states["comparator"] != comparatorState || initial["comparator"] != comparatorState || len(initial) != 2 {
			t.Fatalf("comparator state=%g, recovery=%#v, states=%v, diagnostics=%v", comparatorState, recovery, states, diagnostics)
		}
	}
}

func TestActiveStateV23PreservesLegitimateSaturationAndColdResults(t *testing.T) {
	for _, input := range []float64{0, 2.4, 9} {
		plan := activeStatePlanV23(t, input)
		system, solution := initialStateSystemV23(t, plan)
		for _, initial := range []map[string]float64{nil, {"amplifier": .2}, {"amplifier": 8.8}} {
			oldSystem, oldSolution, oldStates, oldDiagnostics := solveBoundedOpAmpDCFromState(plan, plan.Analyses[0], system, solution, initial)
			if len(oldDiagnostics) != 0 {
				continue
			}
			newSystem, newSolution, newStates, evidence, diagnostics := solveBoundedOpAmpDCFromStateV23(plan, plan.Analyses[0], system, solution, initial)
			if !reflect.DeepEqual(oldSystem, newSystem) || !reflect.DeepEqual(oldSolution, newSolution) || !reflect.DeepEqual(oldStates, newStates) || !reflect.DeepEqual(oldDiagnostics, diagnostics) || evidence.attempted {
				t.Fatal("historical success changed")
			}
		}
	}
}

func TestActiveStateV23OnlyMatchesExactBoundedRefusals(t *testing.T) {
	for _, diagnostics := range [][]Diagnostic{nil, {{Path: "devices", Message: "nonconvergent"}}, {{Path: "devices.opamp", Message: "bounded op-amp operating-point states did not converge"}}, {{Path: "devices", Message: "bounded op-amp operating-point states did not converge"}, {Path: "supply"}}} {
		if activeStateNonconvergenceV23(diagnostics) {
			t.Fatal("accepted unrelated refusal")
		}
	}
	plan := activeStatePlanV23(t, 2.4)
	plan.Analyses[0].Excitations[0].DCValue = 1
	system, solution := initialStateSystemV23(t, plan)
	_, _, _, prior := solveBoundedOpAmpDCFromState(plan, plan.Analyses[0], system, solution, map[string]float64{"amplifier": .2})
	_, _, _, recovery, diagnostics := solveBoundedOpAmpDCFromStateV23(plan, plan.Analyses[0], system, solution, map[string]float64{"amplifier": .2})
	if len(prior) == 0 || !reflect.DeepEqual(prior, diagnostics) || recovery.attempted {
		t.Fatal("supply refusal changed")
	}
}

func TestActiveStateV23RejectsUnstableAlternativeAndPreservesExhaustion(t *testing.T) {
	components := []ComponentEvidence{voltageSourceEvidence("power", "VCC", "GND"), voltageSourceEvidence("stimulus", "IN", "GND"), opAmpEvidence("amplifier", "IN", "OUT", "OUT", "VCC", "GND", .2, .2), opAmpEvidence("positive_feedback", "LOOP", "IN", "LOOP", "VCC", "GND", .2, .2)}
	plan := centeredBiasTestPlan(t, components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "LOOP"}, {Name: "VCC"}}, []SourceExcitation{{Component: "power", DCValue: 9}, {Component: "stimulus", DCValue: 2.4}})
	system, solution := initialStateSystemV23(t, plan)
	initial := map[string]float64{"amplifier": .2, "positive_feedback": .2}
	oldSystem, oldSolution, oldStates, oldDiagnostics := solveBoundedOpAmpDCFromState(plan, plan.Analyses[0], system, solution, initial)
	if !activeStateNonconvergenceV23(oldDiagnostics) {
		t.Fatalf("missing bounded historical refusal: %v", oldDiagnostics)
	}
	newSystem, newSolution, newStates, recovery, diagnostics := solveBoundedOpAmpDCFromStateV23(plan, plan.Analyses[0], system, solution, initial)
	if !recovery.attempted || recovery.accepted || recovery.maximumAdditionalSolves != 18 || !reflect.DeepEqual(oldSystem, newSystem) || !reflect.DeepEqual(oldSolution, newSolution) || !reflect.DeepEqual(oldStates, newStates) || !reflect.DeepEqual(oldDiagnostics, diagnostics) {
		t.Fatalf("unstable alternative altered exhausted result: recovery=%#v diagnostics=%v", recovery, diagnostics)
	}
}
