package designworkflow

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestExplicitNetWeightDoesNotLetSharedGroundCurrentDominatePlacement(t *testing.T) {
	required := true
	ground := explicitNetWeight(ExplicitNetSpec{Name: "GND", Role: "ground", Required: required, CurrentMA: 500})
	returnNet := explicitNetWeight(ExplicitNetSpec{Name: "RETURN", Role: "return", Required: required, CurrentMA: 500})
	power := explicitNetWeight(ExplicitNetSpec{Name: "VBUS", Role: "power_pos", Required: required, CurrentMA: 500})
	if ground != 10 || returnNet != 10 {
		t.Fatalf("ground/return weights = %d/%d, want required-net weight 10", ground, returnNet)
	}
	if power != 14 {
		t.Fatalf("power weight = %d, want current-weighted value 14", power)
	}
}

func TestExplicitThermalPlacementRulePreservesCatalogEvidence(t *testing.T) {
	component := ExplicitComponentSpec{
		ID: "pass", Reference: "Q1", Role: "bjt",
		Placement: ExplicitPlacementSpec{
			Region: "functional_power", Edge: "top", ThermalRole: "power_switch",
			ThermalPathID: "thermal_path.reviewed", ThermalPackage: "to220",
			ThermalPathCPerW: 2.45, ThermalClearanceMM: 7,
			ThermalKeepAwayRole: "sensor", ThermalEdgeRequired: true, PreferThermalCopper: true,
		},
	}
	rule, ok := explicitThermalPlacementRule(component)
	if !ok || rule.ID != "explicit.thermal.pass" || rule.Source != "catalog_thermal_path:thermal_path.reviewed" ||
		!reflect.DeepEqual(rule.Refs, []string{"Q1"}) || rule.ThermalRole != placement.ThermalRolePowerSwitch ||
		rule.PreferredEdge != placement.EdgeTop || rule.PreferredRegion != "" ||
		!reflect.DeepEqual(rule.KeepAwayRoles, []string{"sensor"}) || rule.MinDistanceMM != 7 || !rule.PreferCopper ||
		rule.Severity != placement.AdvancedRuleSeverityError || rule.Enforcement != placement.AdvancedRuleHard {
		t.Fatalf("thermal placement rule = %#v, ok=%t", rule, ok)
	}
	if _, ok := explicitThermalPlacementRule(ExplicitComponentSpec{ID: "ordinary", Reference: "R1"}); ok {
		t.Fatal("nonthermal component produced an advanced thermal rule")
	}
	if _, ok := explicitThermalPlacementRule(ExplicitComponentSpec{
		ID: "blank", Reference: "Q2", Placement: ExplicitPlacementSpec{ThermalRole: "  \t  "},
	}); ok {
		t.Fatal("whitespace-only thermal role produced an advanced thermal rule")
	}
	trimmed := component
	trimmed.Placement.ThermalRole = "  power_switch "
	trimmed.Placement.ThermalPathID = " thermal_path.reviewed  "
	trimmedRule, ok := explicitThermalPlacementRule(trimmed)
	if !ok || trimmedRule.ThermalRole != placement.ThermalRolePowerSwitch ||
		trimmedRule.Source != "catalog_thermal_path:thermal_path.reviewed" {
		t.Fatalf("trimmed thermal rule = %#v, ok=%t", trimmedRule, ok)
	}
	interior := component
	interior.Placement.Edge = ""
	interior.Placement.ThermalEdgeRequired = false
	interior.Placement.ThermalClearanceMM = 0
	interior.Placement.ThermalKeepAwayRole = ""
	interiorRule, ok := explicitThermalPlacementRule(interior)
	if !ok || len(interiorRule.KeepAwayRoles) != 0 || interiorRule.PreferredEdge != placement.EdgeNone ||
		interiorRule.PreferredRegion != "functional_power" {
		t.Fatalf("interior thermal rule = %#v, ok=%t", interiorRule, ok)
	}
}

func TestExpandExplicitPhysicalPadEndpointsIncludesDuplicateSameNetPads(t *testing.T) {
	request := routing.Request{
		Components: []routing.Component{{
			Ref: "J1",
			Pads: []routing.Pad{
				{Name: "1", Net: "VBUS"},
				{Name: "SH", Net: "GND"},
				{Name: "SH", Net: "GND"},
				{Name: "SH", Net: "GND"},
				{Name: "SH", Net: "GND"},
			},
		}},
		Nets: []routing.Net{
			{Name: "VBUS", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}}},
			{Name: "GND", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "SH"}}},
		},
	}

	got := expandExplicitPhysicalPadEndpoints(request)
	wantPads := []string{"1", "SH", "SH#2", "SH#3", "SH#4"}
	padNames := make([]string, len(got.Components[0].Pads))
	for index, pad := range got.Components[0].Pads {
		padNames[index] = pad.Name
	}
	if !reflect.DeepEqual(padNames, wantPads) {
		t.Fatalf("routing pad names = %#v, want %#v", padNames, wantPads)
	}
	wantEndpoints := []routing.Endpoint{{Ref: "J1", Pin: "SH"}, {Ref: "J1", Pin: "SH#2"}, {Ref: "J1", Pin: "SH#3"}, {Ref: "J1", Pin: "SH#4"}}
	if !reflect.DeepEqual(got.Nets[1].Endpoints, wantEndpoints) {
		t.Fatalf("GND endpoints = %#v, want %#v", got.Nets[1].Endpoints, wantEndpoints)
	}
	if request.Components[0].Pads[2].Name != "SH" {
		t.Fatalf("input request mutated: %#v", request.Components[0].Pads)
	}
}

func TestExpandExplicitPhysicalPadEndpointsDoesNotAddUnrelatedComponent(t *testing.T) {
	request := routing.Request{
		Components: []routing.Component{
			{Ref: "J1", Pads: []routing.Pad{{Name: "1", Net: "GND"}}},
			{Ref: "MH1", Pads: []routing.Pad{{Name: "1", Net: "GND"}}},
		},
		Nets: []routing.Net{{Name: "GND", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}}}},
	}

	got := expandExplicitPhysicalPadEndpoints(request)
	if !reflect.DeepEqual(got.Nets[0].Endpoints, request.Nets[0].Endpoints) {
		t.Fatalf("endpoints = %#v, want unrelated component excluded", got.Nets[0].Endpoints)
	}
}

func TestExplicitReturnPathEvidenceMeasuresFinalCopper(t *testing.T) {
	nets := []ExplicitNetSpec{{
		Name: "CLK", ReturnNet: "GND", ReturnPathMaxDistanceMM: 2,
	}}
	routes := []routing.Route{
		{Net: "CLK", Segments: []routing.Segment{{Net: "CLK", Layer: "B.Cu", Start: routing.Point{XMM: 0, YMM: 0}, End: routing.Point{XMM: 4, YMM: 0}}}},
		{Net: "GND", Segments: []routing.Segment{{Net: "GND", Layer: "F.Cu", Start: routing.Point{XMM: 0, YMM: 1}, End: routing.Point{XMM: 4, YMM: 1}}}},
	}
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	const thicknessMM = 1.6
	evidence, issues := explicitReturnPathEvidence(nets, nil, routes, layers, thicknessMM)
	if len(issues) != 0 || len(evidence) != 1 || !evidence[0].Pass ||
		math.Abs(evidence[0].WorstDistanceMM-math.Hypot(1, thicknessMM)) > 1e-12 ||
		evidence[0].SampleCount != 5 || evidence[0].SampleSpacingMM > 1 {
		t.Fatalf("return-path evidence = %#v issues=%#v", evidence, issues)
	}

	routes[1].Segments[0].Start.YMM = 3
	routes[1].Segments[0].End.YMM = 3
	evidence, issues = explicitReturnPathEvidence(nets, nil, routes, layers, thicknessMM)
	if len(issues) != 1 || evidence[0].Pass ||
		math.Abs(evidence[0].WorstDistanceMM-math.Hypot(3, thicknessMM)) > 1e-12 {
		t.Fatalf("unbounded return path was accepted: evidence=%#v issues=%#v", evidence, issues)
	}
}

func TestExplicitReturnPathEvidenceSamplesOffCenterReturnGap(t *testing.T) {
	if intervals, spacing, complete := explicitReturnPathSegmentSampling(routing.Segment{
		Start: routing.Point{}, End: routing.Point{XMM: 1},
	}, 1e-12); complete || intervals != 0 || spacing <= 1e-12 {
		t.Fatalf("unbounded sampling request was accepted: intervals=%d spacing=%g complete=%t", intervals, spacing, complete)
	}
	nets := []ExplicitNetSpec{{Name: "SIG", ReturnNet: "RET", ReturnPathMaxDistanceMM: .4}}
	routes := []routing.Route{
		{Net: "SIG", Segments: []routing.Segment{{
			Net: "SIG", Layer: "F.Cu", Start: routing.Point{}, End: routing.Point{XMM: 4},
		}}},
		{Net: "RET", Segments: []routing.Segment{
			{Net: "RET", Layer: "F.Cu", Start: routing.Point{}, End: routing.Point{XMM: .5}},
			{Net: "RET", Layer: "F.Cu", Start: routing.Point{XMM: .5}, End: routing.Point{XMM: .5, YMM: 3}},
			{Net: "RET", Layer: "F.Cu", Start: routing.Point{XMM: .5, YMM: 3}, End: routing.Point{XMM: 1.5, YMM: 3}},
			{Net: "RET", Layer: "F.Cu", Start: routing.Point{XMM: 1.5, YMM: 3}, End: routing.Point{XMM: 1.5}},
			{Net: "RET", Layer: "F.Cu", Start: routing.Point{XMM: 1.5}, End: routing.Point{XMM: 4}},
		}},
	}
	layers := []routing.Layer{{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true}}
	evidence, issues := explicitReturnPathEvidence(nets, nil, routes, layers, 1.6)
	if len(issues) != 1 || evidence[0].Pass || evidence[0].SampleSpacingMM > .2 || evidence[0].WorstDistanceMM <= .4 {
		t.Fatalf("off-center return gap was not detected: evidence=%#v issues=%#v", evidence, issues)
	}
}

func TestReturnPathEvaluationBudgetFailsClosedWithoutOverflow(t *testing.T) {
	remaining := explicitReturnPathMaximumDistanceEvaluations
	if consumeReturnPathEvaluationBudget(&remaining, explicitReturnPathMaximumDistanceEvaluations, 2) {
		t.Fatal("oversized return-path evaluation was accepted")
	}
	if remaining != explicitReturnPathMaximumDistanceEvaluations {
		t.Fatalf("rejected evaluation consumed budget: %d", remaining)
	}
	if !consumeReturnPathEvaluationBudget(&remaining, 500000, 2) || remaining != 0 {
		t.Fatalf("bounded evaluation budget was not consumed exactly: %d", remaining)
	}
}

func TestExplicitReturnPathEvidenceFailureRemainsJSONEncodable(t *testing.T) {
	nets := []ExplicitNetSpec{{Name: "SIG", ReturnNet: "RET", ReturnPathMaxDistanceMM: 1}}
	routes := []routing.Route{
		{Net: "SIG", Segments: []routing.Segment{{Layer: "unknown", End: routing.Point{XMM: 1}}}},
		{Net: "RET", Segments: []routing.Segment{{Layer: "also-unknown", End: routing.Point{XMM: 1}}}},
	}
	layers := []routing.Layer{{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true}}
	evidence, issues := explicitReturnPathEvidence(nets, nil, routes, layers, 1.6)
	if len(issues) != 1 || len(evidence) != 1 || evidence[0].Pass || evidence[0].WorstDistanceMM != math.MaxFloat64 {
		t.Fatalf("fail-closed evidence = %#v issues=%#v", evidence, issues)
	}
	if _, err := json.Marshal(evidence); err != nil {
		t.Fatalf("marshal fail-closed return-path evidence: %v", err)
	}
}

func TestExplicitReturnPathEvidenceUsesDeclaredReturnPlaneAndBoardThickness(t *testing.T) {
	nets := []ExplicitNetSpec{{
		Name: "CLK", ReturnNet: "GND", ReturnPathMaxDistanceMM: 1,
	}}
	zones := []ExplicitZoneSpec{{Net: "GND", Layers: []string{"B.Cu"}}}
	routes := []routing.Route{{
		Net: "CLK", Segments: []routing.Segment{{
			Net: "CLK", Layer: "F.Cu",
			Start: routing.Point{XMM: 0, YMM: 0}, End: routing.Point{XMM: 4, YMM: 0},
		}},
	}}
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	evidence, issues := explicitReturnPathEvidence(nets, zones, routes, layers, .8)
	if len(issues) != 0 || len(evidence) != 1 || !evidence[0].Pass ||
		!reflect.DeepEqual(evidence[0].ReturnPlaneLayers, []string{"B.Cu"}) ||
		math.Abs(evidence[0].WorstDistanceMM-.8) > 1e-12 {
		t.Fatalf("return-plane evidence = %#v issues=%#v", evidence, issues)
	}
}

func TestCopperLayerSeparationUsesDeclaredUniformStackModel(t *testing.T) {
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	stack := newCopperLayerStack(layers, 1.6)
	if got := stack.separationMM("F.Cu", "In1.Cu"); math.Abs(got-1.6/3) > 1e-12 {
		t.Fatalf("uniform multilayer separation = %.12g, want %.12g", got, 1.6/3)
	}
	if got := stack.separationMM("IN2.CU", "in1.cu"); math.Abs(got-1.6/3) > 1e-12 {
		t.Fatalf("canonical multilayer separation = %.12g, want %.12g", got, 1.6/3)
	}
	paddedLayers := append([]routing.Layer(nil), layers...)
	paddedLayers[1].Name = " In1.Cu "
	if got := newCopperLayerStack(paddedLayers, 1.6).separationMM("F.Cu", "in1.cu"); math.Abs(got-1.6/3) > 1e-12 {
		t.Fatalf("normalized stack-index separation = %.12g, want %.12g", got, 1.6/3)
	}
	via := routing.Via{Layers: []string{"In2.Cu", "F.Cu", "B.Cu"}}
	if got := viaVerticalSpanMM(via, stack); math.Abs(got-1.6) > 1e-12 {
		t.Fatalf("unordered multilayer via span = %.12g, want 1.6", got)
	}
	ambiguousLayers := append(append([]routing.Layer(nil), layers...), routing.Layer{
		Name: " in1.cu ", Kind: routing.LayerCopper, Routable: true,
	})
	if stack := newCopperLayerStack(ambiguousLayers, 1.6); !stack.ambiguous {
		t.Fatalf("normalized copper-layer collision was accepted: %#v", stack)
	}
}

func TestExplicitReturnPathEvidenceTreatsPreferredLayerAsPreference(t *testing.T) {
	nets := []ExplicitNetSpec{{
		Name: "POWER", PreferLayer: "In2.Cu", ReturnNet: "GND", ReturnPathMaxDistanceMM: 1,
	}}
	zones := []ExplicitZoneSpec{{Net: "GND", Layers: []string{"in1.cu"}}}
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	routes := []routing.Route{{Net: "POWER", Segments: []routing.Segment{{
		Net: "POWER", Layer: "F.Cu", Start: routing.Point{}, End: routing.Point{XMM: 2},
	}}}}
	evidence, issues := explicitReturnPathEvidence(nets, zones, routes, layers, 1.6)
	if len(issues) != 0 || len(evidence) != 1 || !evidence[0].Pass || evidence[0].PreferredLayerUsed ||
		!reflect.DeepEqual(evidence[0].ReturnPlaneLayers, []string{"In1.Cu"}) {
		t.Fatalf("alternate-layer return evidence = %#v issues=%#v", evidence, issues)
	}
	routes[0].Segments[0].Layer = "IN2.CU"
	evidence, issues = explicitReturnPathEvidence(nets, zones, routes, layers, 1.6)
	if len(issues) != 0 || !evidence[0].Pass || !evidence[0].PreferredLayerUsed ||
		!reflect.DeepEqual(evidence[0].UsedLayers, []string{"In2.Cu"}) {
		t.Fatalf("canonical preferred-layer evidence = %#v issues=%#v", evidence, issues)
	}
}

func TestExplicitReturnPathEvidenceValidatesViaOnlySignal(t *testing.T) {
	nets := []ExplicitNetSpec{{
		Name: "CLK", ReturnNet: "GND", ReturnPathMaxDistanceMM: 1,
	}}
	routes := []routing.Route{
		{Net: "CLK", Vias: []routing.Via{{
			Net: "CLK", At: routing.Point{XMM: 0, YMM: 0}, Layers: []string{"f.cu", "B.CU"},
		}}},
		{Net: "GND", Vias: []routing.Via{{
			Net: "GND", At: routing.Point{XMM: .5, YMM: 0}, Layers: []string{"F.Cu", "B.Cu"},
		}}},
	}
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	evidence, issues := explicitReturnPathEvidence(nets, nil, routes, layers, 1.6)
	if len(issues) != 0 || len(evidence) != 1 || !evidence[0].Pass ||
		evidence[0].SampleCount != 2 || math.Abs(evidence[0].WorstDistanceMM-.5) > 1e-12 ||
		len(evidence[0].LayerTransitions) != 1 || !evidence[0].LayerTransitions[0].ReturnViaRequired ||
		!evidence[0].LayerTransitions[0].ReturnViaFound || !evidence[0].LayerTransitions[0].Pass {
		t.Fatalf("via-only return evidence = %#v issues=%#v", evidence, issues)
	}
	routes[1].Vias[0].Layers = []string{"In2.Cu", "B.Cu"}
	evidence, issues = explicitReturnPathEvidence(nets, nil, routes, layers, 1.6)
	wantBlindViaDistanceMM := math.Hypot(.5, 1.6*2/3)
	if len(issues) != 1 || evidence[0].Pass || math.Abs(evidence[0].WorstDistanceMM-wantBlindViaDistanceMM) > 1e-12 {
		t.Fatalf("distant blind return via was accepted: evidence=%#v issues=%#v", evidence, issues)
	}
	routes[1].Vias[0].Layers = []string{"F.Cu", "B.Cu"}
	routes[0].Vias[0].Layers = nil
	evidence, issues = explicitReturnPathEvidence(nets, nil, routes, layers, 1.6)
	if len(issues) != 0 || !evidence[0].Pass || evidence[0].SampleCount != len(layers) {
		t.Fatalf("through-via return evidence = %#v issues=%#v", evidence, issues)
	}
}

func TestExplicitReturnTransitionEvidenceUsesCommonPlaneOrPairedVia(t *testing.T) {
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	stack := newCopperLayerStack(layers, 1.6)
	signal := routing.Route{Vias: []routing.Via{{
		At: routing.Point{XMM: 2, YMM: 3}, Layers: []string{"F.Cu", "B.Cu"},
	}}}
	evidence := explicitReturnTransitionEvidence(signal, routing.Route{}, []string{"In1.Cu"}, stack, .5)
	if len(evidence) != 1 || !evidence[0].Pass || evidence[0].ReturnViaRequired || evidence[0].ReturnViaFound ||
		!reflect.DeepEqual(evidence[0].ReferenceLayers, []string{"In1.Cu"}) {
		t.Fatalf("common-plane transition evidence = %#v", evidence)
	}
	returnPath := routing.Route{Vias: []routing.Via{{
		At: routing.Point{XMM: 2.4, YMM: 3}, Layers: []string{"In1.Cu", "In2.Cu"},
	}}}
	evidence = explicitReturnTransitionEvidence(signal, returnPath, []string{"In1.Cu", "In2.Cu"}, stack, .5)
	if len(evidence) != 1 || !evidence[0].Pass || !evidence[0].ReturnViaRequired || !evidence[0].ReturnViaFound ||
		math.Abs(evidence[0].ReturnViaDistanceMM-.4) > 1e-12 ||
		!reflect.DeepEqual(evidence[0].ReferenceLayers, []string{"In1.Cu", "In2.Cu"}) {
		t.Fatalf("paired-return-via evidence = %#v", evidence)
	}
	returnPath.Vias[0].At.XMM = 2.6
	evidence = explicitReturnTransitionEvidence(signal, returnPath, []string{"In1.Cu", "In2.Cu"}, stack, .5)
	if len(evidence) != 1 || evidence[0].Pass || evidence[0].ReturnViaFound ||
		math.Abs(evidence[0].ReturnViaDistanceMM-.6) > 1e-12 {
		t.Fatalf("distant paired return via was accepted: %#v", evidence)
	}
}

func TestMaterializeExplicitReturnTransitionViasIsClearanceSafeAndIdempotent(t *testing.T) {
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	routingRequest := routing.Request{
		Board:    routing.Board{WidthMM: 20, HeightMM: 20, Layers: layers, MarginMM: .5},
		Nets:     []routing.Net{{Name: "SIG", Role: routing.NetSignal}, {Name: "GND", Role: routing.NetGround}},
		Rules:    routing.DefaultRules(),
		Strategy: routing.Strategy{Mode: routing.ModeTwoLayer},
	}
	routing.NormalizeRequest(&routingRequest)
	operation := func(payload transactions.RouteOperation) transactions.Operation {
		t.Helper()
		result, err := workflowOperation(transactions.OpRoute, payload)
		if err != nil {
			t.Fatalf("encode route operation: %v", err)
		}
		return result
	}
	operations := []transactions.Operation{
		operation(transactions.RouteOperation{
			Op: transactions.OpRoute, NetName: "SIG", Layer: "F.Cu", WidthMM: .25,
			Points: []transactions.Point{{XMM: 5, YMM: 10}, {XMM: 10, YMM: 10}},
		}),
		operation(transactions.RouteOperation{
			Op: transactions.OpRoute, NetName: "SIG", Layer: "B.Cu", WidthMM: .25,
			Points: []transactions.Point{{XMM: 10, YMM: 10}, {XMM: 15, YMM: 10}},
		}),
		operation(transactions.RouteOperation{
			Op: transactions.OpRoute, NetName: "SIG", Vias: []transactions.RouteViaSpec{{
				At: transactions.Point{XMM: 10, YMM: 10}, DiameterMM: .6, DrillMM: .3,
				Layers: []string{"F.Cu", "B.Cu"},
			}},
		}),
		operation(transactions.RouteOperation{
			Op: transactions.OpRoute, NetName: "SIG", Layer: "F.Cu", WidthMM: .25,
			Points: []transactions.Point{{XMM: 14, YMM: 10}, {XMM: 14, YMM: 8}},
		}),
		operation(transactions.RouteOperation{
			Op: transactions.OpRoute, NetName: "SIG", Vias: []transactions.RouteViaSpec{{
				At: transactions.Point{XMM: 14, YMM: 10}, DiameterMM: .6, DrillMM: .3,
				Layers: []string{"F.Cu", "B.Cu"},
			}},
		}),
	}
	nets := []ExplicitNetSpec{{Name: "SIG", ReturnNet: "GND", ReturnPathMaxDistanceMM: 1.2}}
	zones := []ExplicitZoneSpec{{Net: "GND", Layers: []string{"In1.Cu", "In2.Cu"}}}
	materialized, added, issues := materializeExplicitReturnTransitionVias(
		nets, zones, operations, routingRequest, 1.6,
	)
	if len(issues) != 0 || added != 2 {
		t.Fatalf("paired return-via materialization added=%d issues=%#v", added, issues)
	}
	evidence, evidenceIssues := explicitReturnPathEvidence(nets, zones, routingRoutesFromOperations(materialized), layers, 1.6)
	if len(evidenceIssues) != 0 || len(evidence) != 1 || !evidence[0].Pass ||
		len(evidence[0].LayerTransitions) != 2 {
		t.Fatalf("materialized return-transition evidence=%#v issues=%#v", evidence, evidenceIssues)
	}
	for _, transition := range evidence[0].LayerTransitions {
		if !transition.ReturnViaFound || transition.ReturnViaDistanceMM > 1.2 {
			t.Fatalf("unproven materialized transition: %#v", transition)
		}
	}
	repeated, repeatedAdded, repeatedIssues := materializeExplicitReturnTransitionVias(
		nets, zones, materialized, routingRequest, 1.6,
	)
	if len(repeatedIssues) != 0 || repeatedAdded != 0 || !reflect.DeepEqual(repeated, materialized) {
		t.Fatalf("return-via materialization is not idempotent: added=%d issues=%#v", repeatedAdded, repeatedIssues)
	}
	unrepairableNets := []ExplicitNetSpec{{Name: "SIG", ReturnNet: "GND", ReturnPathMaxDistanceMM: .1}}
	_, unrepairableAdded, unrepairableIssues := materializeExplicitReturnTransitionVias(
		unrepairableNets, zones, operations, routingRequest, 1.6,
	)
	if unrepairableAdded != 0 || len(unrepairableIssues) == 0 {
		t.Fatalf("non-transition return-path failure was swallowed: added=%d issues=%#v", unrepairableAdded, unrepairableIssues)
	}
}

func TestExplicitReturnViaCandidatesFailClosedAtWorkBound(t *testing.T) {
	candidates, complete := explicitReturnViaCandidates(transactions.Point{}, .25, .5)
	if !complete || len(candidates) != 12 || candidates[0] != (transactions.Point{XMM: .25}) {
		t.Fatalf("bounded perimeter candidates=%#v complete=%t", candidates, complete)
	}
	seen := map[transactions.Point]bool{}
	for _, candidate := range candidates {
		if seen[candidate] || math.Hypot(candidate.XMM, candidate.YMM) > .5+explicitReturnViaDistanceToleranceMM {
			t.Fatalf("duplicate or out-of-radius candidate: %#v", candidate)
		}
		seen[candidate] = true
	}
	if candidates, complete := explicitReturnViaCandidates(transactions.Point{}, .01, 10); complete || candidates != nil {
		t.Fatalf("oversized return-via search was accepted: candidates=%d complete=%t", len(candidates), complete)
	}
}

func TestMaterializeExplicitZoneAccessViasIsClearanceSafeAndIdempotent(t *testing.T) {
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	routingRequest := routing.Request{
		Board: routing.Board{WidthMM: 20, HeightMM: 20, Layers: layers, MarginMM: .5},
		Nets:  []routing.Net{{Name: "POWER", Role: routing.NetPower}},
		Rules: routing.DefaultRules(), Strategy: routing.Strategy{Mode: routing.ModeTwoLayer},
	}
	routing.NormalizeRequest(&routingRequest)
	operation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
		Op: transactions.OpRoute, NetName: "POWER", Layer: "F.Cu", WidthMM: .5,
		Points: []transactions.Point{{XMM: 5, YMM: 10}, {XMM: 15, YMM: 10}},
	})
	if err != nil {
		t.Fatal(err)
	}
	zones := []ExplicitZoneSpec{{Net: "POWER", Layers: []string{"In2.Cu"}}}
	materialized, added, issues := materializeExplicitZoneAccessVias(zones, []transactions.Operation{operation}, routingRequest)
	if len(issues) != 0 || added != 1 {
		t.Fatalf("zone access-via materialization added=%d issues=%#v", added, issues)
	}
	routes := routingRoutesFromOperations(materialized)
	if len(routes) != 1 || len(routes[0].Vias) != 1 || routes[0].Vias[0].At != (routing.Point{XMM: 10, YMM: 10}) ||
		!reflect.DeepEqual(routes[0].Vias[0].Layers, []string{"F.Cu", "B.Cu"}) {
		t.Fatalf("materialized zone access route=%#v", routes)
	}
	repeated, repeatedAdded, repeatedIssues := materializeExplicitZoneAccessVias(zones, materialized, routingRequest)
	if len(repeatedIssues) != 0 || repeatedAdded != 0 || !reflect.DeepEqual(repeated, materialized) {
		t.Fatalf("zone access materialization is not idempotent: added=%d issues=%#v", repeatedAdded, repeatedIssues)
	}

	throughHoleRequest := routingRequest
	throughHoleRequest.Components = []routing.Component{{
		Ref: "J1", Pads: []routing.Pad{{
			Name: "1", Net: "POWER", Type: routing.PadThroughHole,
			Drill: &routing.Drill{DiameterMM: .8}, Size: routing.Size{WidthMM: 1.6, HeightMM: 1.6},
		}},
	}}
	throughHole, throughHoleAdded, throughHoleIssues := materializeExplicitZoneAccessVias(zones, nil, throughHoleRequest)
	if len(throughHoleIssues) != 0 || throughHoleAdded != 0 || len(throughHole) != 0 {
		t.Fatalf("through-hole plane access added redundant via: added=%d issues=%#v operations=%#v", throughHoleAdded, throughHoleIssues, throughHole)
	}

	_, missingAdded, missingIssues := materializeExplicitZoneAccessVias(zones, nil, routingRequest)
	if missingAdded != 0 || len(missingIssues) == 0 {
		t.Fatalf("missing plane access did not fail closed: added=%d issues=%#v", missingAdded, missingIssues)
	}
}

func TestExplicitZoneAccessViaCandidatesDeduplicateAtNanometrePrecision(t *testing.T) {
	near := .1 + .2
	route := routing.Route{Segments: []routing.Segment{
		{Layer: "F.Cu", Start: routing.Point{XMM: near, YMM: 1}, End: routing.Point{XMM: 1, YMM: 1}},
		{Layer: "F.Cu", Start: routing.Point{XMM: .3, YMM: 1}, End: routing.Point{XMM: 2, YMM: 1}},
	}}
	candidates := explicitZoneAccessViaCandidates(route, routing.Via{Net: "POWER"})
	matches := 0
	for _, candidate := range candidates {
		if math.Round(candidate.At.XMM*1_000_000) == 300_000 && math.Round(candidate.At.YMM*1_000_000) == 1_000_000 {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("nanometre-equivalent candidates retained %d times: %#v", matches, candidates)
	}
}
