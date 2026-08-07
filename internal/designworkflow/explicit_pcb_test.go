package designworkflow

import (
	"math"
	"reflect"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/routing"
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
		rule.PreferredEdge != placement.EdgeTop || rule.PreferredRegion != "functional_power" ||
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
	if !ok || len(interiorRule.KeepAwayRoles) != 0 || interiorRule.PreferredEdge != placement.EdgeNone {
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
		evidence[0].SampleCount != 3 {
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
	if got := copperLayerSeparationMM("F.Cu", "In1.Cu", layers, 1.6); math.Abs(got-1.6/3) > 1e-12 {
		t.Fatalf("uniform multilayer separation = %.12g, want %.12g", got, 1.6/3)
	}
}

func TestExplicitReturnPathEvidenceValidatesViaOnlySignal(t *testing.T) {
	nets := []ExplicitNetSpec{{
		Name: "CLK", ReturnNet: "GND", ReturnPathMaxDistanceMM: 1,
	}}
	routes := []routing.Route{
		{Net: "CLK", Vias: []routing.Via{{
			Net: "CLK", At: routing.Point{XMM: 0, YMM: 0}, Layers: []string{"F.Cu", "B.Cu"},
		}}},
		{Net: "GND", Vias: []routing.Via{{
			Net: "GND", At: routing.Point{XMM: .5, YMM: 0}, Layers: []string{"F.Cu", "B.Cu"},
		}}},
	}
	layers := []routing.Layer{
		{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
	}
	evidence, issues := explicitReturnPathEvidence(nets, nil, routes, layers, 1.6)
	if len(issues) != 0 || len(evidence) != 1 || !evidence[0].Pass ||
		evidence[0].SampleCount != 2 || math.Abs(evidence[0].WorstDistanceMM-.5) > 1e-12 {
		t.Fatalf("via-only return evidence = %#v issues=%#v", evidence, issues)
	}
}
