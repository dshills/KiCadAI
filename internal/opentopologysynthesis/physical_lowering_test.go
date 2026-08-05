package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
)

func TestPassingPrimitiveGraphLowersToResolvedDesignRequest(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	evaluation := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, DefaultPolicy())
	if evaluation.Status != SimulationEvaluationPassed {
		t.Fatalf("simulation prerequisite = status=%s issues=%#v diagnoses=%#v", evaluation.Status, evaluation.Issues, evaluation.Diagnoses)
	}
	first := LowerPassingCandidate(context.Background(), requirement, graph, evaluation, inventory, environment)
	if first.Status != PhysicalLoweringReady || len(first.Issues) != 0 ||
		len(first.Document.Components) == 0 || len(first.Document.Nets) == 0 ||
		len(first.Resolved.Components) != len(first.Document.Components) ||
		first.DesignRequest.ExplicitCircuit == nil ||
		len(first.DesignRequest.ExplicitCircuit.Components) != len(first.Document.Components) ||
		len(first.Bindings) == 0 || len(first.Hash) != 64 {
		t.Fatalf("physical lowering = status=%s issues=%#v components=%d/%d request=%#v bindings=%#v",
			first.Status, first.Issues, len(first.Resolved.Components), len(first.Document.Components), first.DesignRequest, first.Bindings)
	}
	if issues := circuitgraph.Validate(first.Document); len(issues) != 0 {
		t.Fatalf("lowered circuit graph issues: %#v", issues)
	}
	if first.DesignRequest.ExplicitCircuit.RoutingPolicy != designworkflow.ExplicitRoutingPolicyConstrainedEndpointAccessV1 {
		t.Fatalf("open-topology routing policy = %q", first.DesignRequest.ExplicitCircuit.RoutingPolicy)
	}
	for _, component := range first.Document.Components {
		if component.ComponentID == "" || component.VariantID == "" {
			t.Fatalf("component lacks deterministic physical selection: %#v", component)
		}
	}

	second := LowerPassingCandidate(context.Background(), requirement, graph, evaluation, inventory, environment)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("physical lowering replay differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestPhysicalLoweringRejectsUnprovenGraph(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	result := LowerPassingCandidate(context.Background(), requirement, graph, SimulationEvaluation{}, inventory, environment)
	if result.Status != PhysicalLoweringUnsupported || len(result.Issues) != 1 ||
		result.Issues[0].Code != CodePhysicalPromotionFailed {
		t.Fatalf("unproven lowering = status=%s issues=%#v", result.Status, result.Issues)
	}
}

func TestPhysicalComponentUnitsDeclareRequiredPowerUnit(t *testing.T) {
	primitive := PrimitiveCandidate{
		UnitID: "A",
		Terminals: []PrimitiveTerminal{
			{Terminal: "IN_PLUS", UnitID: "A", Electrical: "input"},
			{Terminal: "IN_MINUS", UnitID: "A", Electrical: "input"},
			{Terminal: "OUT", UnitID: "A", Electrical: "output"},
			{Terminal: "V_PLUS", UnitID: "P", Electrical: "power_in"},
			{Terminal: "V_MINUS", UnitID: "P", Electrical: "power_in"},
		},
	}
	units := physicalComponentUnits(primitive, "opamp")
	want := []circuitgraph.ComponentUnit{{ID: "A", Role: "opamp"}, {ID: "P", Role: "power"}}
	if !bytes.Equal(mustJSON(t, units), mustJSON(t, want)) {
		t.Fatalf("physical component units = %#v, want %#v", units, want)
	}
}

func TestPhysicalPackageCompletionTerminatesUnusedFunctionalUnits(t *testing.T) {
	primitive := PrimitiveCandidate{
		CatalogID: "test.dual_opamp",
		UnitID:    "A",
		Terminals: []PrimitiveTerminal{
			{Terminal: "IN_PLUS", UnitID: "A", Electrical: "input"},
			{Terminal: "IN_MINUS", UnitID: "A", Electrical: "input"},
			{Terminal: "OUT", UnitID: "A", Electrical: "output"},
			{Terminal: "V_PLUS", UnitID: "P", Electrical: "power_in"},
			{Terminal: "V_MINUS", UnitID: "P", Electrical: "power_in"},
		},
	}
	catalog := &components.Catalog{Records: []components.ComponentRecord{{
		ID: primitive.CatalogID,
		Symbols: []components.SymbolBinding{
			{UnitID: "A", Unit: 1, UnitType: components.SymbolUnitFunctional},
			{UnitID: "B", Unit: 2, UnitType: components.SymbolUnitFunctional, FunctionPins: []components.FunctionPin{
				{Function: "IN_PLUS"}, {Function: "IN_MINUS"}, {Function: "OUT"},
			}},
			{UnitID: "P", Unit: 3, UnitType: components.SymbolUnitPower, RequiredUnit: true},
		},
	}}}
	units, noConnects, issues := physicalPackageCompletion("amplifier", primitive, "opamp", catalog)
	wantUnits := []circuitgraph.ComponentUnit{
		{ID: "A", Role: "opamp"}, {ID: "B", Role: "unused"}, {ID: "P", Role: "power"},
	}
	if len(issues) != 0 || !bytes.Equal(mustJSON(t, units), mustJSON(t, wantUnits)) || len(noConnects) != 3 {
		t.Fatalf("package completion: units=%#v no_connects=%#v issues=%#v", units, noConnects, issues)
	}
	for _, endpoint := range noConnects {
		if endpoint.Component != "amplifier" || endpoint.Unit != "B" ||
			endpoint.SelectorKind != circuitgraph.SelectorFunction {
			t.Fatalf("unused-unit no-connect = %#v", endpoint)
		}
	}
}

func TestAppendPhysicalNetEndpointIsIdempotent(t *testing.T) {
	endpoint := circuitgraph.Endpoint{
		Component: "controller", Unit: "A",
		SelectorKind: circuitgraph.SelectorFunction, Selector: "BIAS",
	}
	document := circuitgraph.Document{Nets: []circuitgraph.Net{{Name: "OUTPUT"}}}

	appendPhysicalNetEndpoint(&document, "OUTPUT", endpoint)
	appendPhysicalNetEndpoint(&document, "OUTPUT", endpoint)

	if len(document.Nets[0].Endpoints) != 1 || document.Nets[0].Endpoints[0] != endpoint {
		t.Fatalf("idempotent physical endpoint append produced %#v", document.Nets[0].Endpoints)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
