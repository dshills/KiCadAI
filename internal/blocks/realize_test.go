package blocks

import (
	"context"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestRealizeBlockPCBProducesPlacementsAndRoutes(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("led_indicator")
	if !ok {
		t.Fatal("missing led_indicator")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "led1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 10, OriginYMM: 5})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	if len(result.Components) != 2 {
		t.Fatalf("components = %#v, want 2", result.Components)
	}
	if result.Components[0].Ref == "" || result.Components[0].Placement.XMM != 10 {
		t.Fatalf("unexpected first component = %#v", result.Components[0])
	}
	if len(result.LocalRoutes) != 1 || result.LocalRoutes[0].NetName != "led1_led_series" {
		t.Fatalf("routes = %#v", result.LocalRoutes)
	}
	if countOperations(result.Operations, transactions.OpPlaceFootprint) != 2 ||
		countOperations(result.Operations, transactions.OpRoute) != 1 {
		t.Fatalf("operations = %#v", result.Operations)
	}
}

func TestRealizeBlockPCBOffsetsRouteWaypoints(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.Components[0].Pins = twoTerminalHorizontalPins()
	definition.Components[1].Pins = twoTerminalHorizontalPins()
	definition.PCBRealization.Components = append(definition.PCBRealization.Components, PCBComponentRealization{
		ComponentRole: "led",
		FootprintID:   "LED:D_0805",
		Placement:     RelativePlacement{XMM: 4, YMM: 0, Layer: "F.Cu"},
	})
	definition.PCBRealization.LocalRoutes[0].To.Pin = "1"
	output := BlockOutput{
		Definition: Summary(definition),
		Instance: BlockInstance{
			BlockID:    definition.ID,
			InstanceID: "demo1",
			Params:     map[string]any{"footprint": "Device:R_0805"},
			Refs:       []string{"R1", "D1"},
		},
		Operations: mustComponentOps(t,
			BlockComponent{Role: "resistor", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Device:R_0805", Pins: twoTerminalHorizontalPins()}, "R1",
			BlockComponent{Role: "led", RefPrefix: "D", Value: "LED", SymbolID: "Device:LED", FootprintID: "LED:D_0805", Pins: twoTerminalHorizontalPins()}, "D1",
		),
	}

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 10, OriginYMM: 20})
	if reports.HasBlockingIssue(result.Issues) || len(result.LocalRoutes) == 0 {
		t.Fatalf("realize result issues = %#v routes = %#v components = %#v", result.Issues, result.LocalRoutes, result.Components)
	}
	var route transactions.RouteOperation
	for _, operation := range result.Operations {
		if operation.Op == transactions.OpRoute {
			if err := decodeOperation(operation, &route); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(route.Points) != 3 || route.Points[1].XMM != 12 || route.Points[1].YMM != 22 {
		t.Fatalf("route points = %#v, want offset waypoint at 12,22", route.Points)
	}
}

func TestRealizeBlockPCBProducesEntryAnchorRoutes(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.Components[0].Pins = twoTerminalHorizontalPins()
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{{
		ID:          "input_entry",
		Port:        "IN",
		NetTemplate: "SIG",
		Placement:   RelativePlacement{XMM: -3, YMM: 1, Layer: "F.Cu"},
		Description: "Signal entry",
	}}
	definition.PCBRealization.LocalRoutes = []PCBLocalRoute{{
		ID:          "entry_to_resistor",
		NetTemplate: "SIG",
		From:        RouteEndpoint{AnchorID: "input_entry"},
		To:          RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
		Waypoints:   []RelativePoint{{XMM: -1, YMM: 1}},
		Layer:       "F.Cu",
		WidthMM:     0.25,
	}}
	output := BlockOutput{
		Definition: Summary(definition),
		Instance: BlockInstance{
			BlockID:    definition.ID,
			InstanceID: "demo1",
			Params:     map[string]any{"footprint": "Device:R_0805"},
			Refs:       []string{"R1"},
		},
		Operations: mustSingleComponentOps(t,
			BlockComponent{Role: "resistor", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Device:R_0805", Pins: twoTerminalHorizontalPins()}, "R1",
		),
	}

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 10, OriginYMM: 20})
	if reports.HasBlockingIssue(result.Issues) || len(result.Issues) != 0 {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	if len(result.EntryAnchors) != 1 {
		t.Fatalf("entry anchors = %#v, want one", result.EntryAnchors)
	}
	anchor := result.EntryAnchors[0]
	if anchor.ID != "input_entry" || anchor.NetName != "demo1_SIG" || anchor.Placement.XMM != 7 || anchor.Placement.YMM != 21 {
		t.Fatalf("entry anchor = %#v", anchor)
	}
	if len(result.LocalRoutes) != 1 {
		t.Fatalf("local routes = %#v, want one", result.LocalRoutes)
	}
	route := result.LocalRoutes[0]
	if route.From.Ref != "@anchor:input_entry" || route.From.Pin != "IN" || route.To.Ref != "R1" || route.To.Pin != "1" {
		t.Fatalf("realized route endpoints = %#v -> %#v", route.From, route.To)
	}
	if route.LengthMM <= 0 {
		t.Fatalf("route length = %f, want positive", route.LengthMM)
	}
	var op transactions.RouteOperation
	for _, operation := range result.Operations {
		if operation.Op == transactions.OpRoute {
			if err := decodeOperation(operation, &op); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(op.Points) != 3 || op.Points[0].XMM != 7 || op.Points[0].YMM != 21 || op.Points[1].XMM != 9 || op.Points[1].YMM != 21 {
		t.Fatalf("route operation points = %#v", op.Points)
	}
	if result.Metadata["entry_anchors"].Count != 1 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestRealizeBlockPCBResolvesPortEndpointThroughEntryAnchor(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.Components[0].Pins = twoTerminalHorizontalPins()
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{{
		ID:        "input_entry",
		Port:      "IN",
		Placement: RelativePlacement{XMM: -3, YMM: 0, Layer: "F.Cu"},
	}}
	definition.PCBRealization.LocalRoutes = []PCBLocalRoute{{
		ID:          "port_to_resistor",
		NetTemplate: "SIG",
		From:        RouteEndpoint{Port: "IN"},
		To:          RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
		Layer:       "F.Cu",
	}}
	output := singleResistorOutput(t, definition)

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if len(result.LocalRoutes) != 1 || result.LocalRoutes[0].From.Ref != "@anchor:input_entry" {
		t.Fatalf("local routes = %#v", result.LocalRoutes)
	}
}

func TestRealizeBlockPCBSkipsConditionalEntryAnchorAndRoute(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.Parameters = append(definition.Parameters, BlockParameter{Name: "enabled", Type: ParameterBool, Default: true})
	definition.Components[0].Pins = twoTerminalHorizontalPins()
	condition := RealizationWhen{Params: map[string]any{"enabled": true}}
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{{
		ID:        "input_entry",
		Port:      "IN",
		Placement: RelativePlacement{XMM: -3, YMM: 0, Layer: "F.Cu"},
		When:      condition,
	}}
	definition.PCBRealization.LocalRoutes = []PCBLocalRoute{{
		ID:          "conditional",
		NetTemplate: "SIG",
		From:        RouteEndpoint{AnchorID: "input_entry"},
		To:          RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
		Layer:       "F.Cu",
		When:        condition,
	}}
	output := singleResistorOutput(t, definition)
	output.Instance.Params["enabled"] = false

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if len(result.EntryAnchors) != 0 || len(result.LocalRoutes) != 0 {
		t.Fatalf("anchors/routes = %#v %#v, want none", result.EntryAnchors, result.LocalRoutes)
	}
}

func TestRealizeBlockPCBReportsInactiveAnchorRouteEndpoint(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.Parameters = append(definition.Parameters, BlockParameter{Name: "enabled", Type: ParameterBool, Default: true})
	definition.Components[0].Pins = twoTerminalHorizontalPins()
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{{
		ID:        "input_entry",
		Port:      "IN",
		Placement: RelativePlacement{XMM: -3, YMM: 0, Layer: "F.Cu"},
		When:      RealizationWhen{Params: map[string]any{"enabled": true}},
	}}
	definition.PCBRealization.LocalRoutes = []PCBLocalRoute{{
		ID:          "missing_anchor",
		NetTemplate: "SIG",
		From:        RouteEndpoint{AnchorID: "input_entry"},
		To:          RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
		Layer:       "F.Cu",
	}}
	output := singleResistorOutput(t, definition)
	output.Instance.Params["enabled"] = false

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if len(result.EntryAnchors) != 0 || len(result.LocalRoutes) != 0 {
		t.Fatalf("anchors/routes = %#v %#v, want route skipped", result.EntryAnchors, result.LocalRoutes)
	}
	if len(result.Issues) == 0 {
		t.Fatalf("issues = %#v, want inactive anchor endpoint issue", result.Issues)
	}
	assertIssuePath(t, result.Issues, "pcb_realization.local_routes.missing_anchor.from")
}

func TestRealizeBlockPCBBlocksMissingFootprint(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.Components[0].FootprintParam = ""
	definition.PCBRealization.Components[0].FootprintID = ""
	output := BlockOutput{
		Definition: Summary(definition),
		Instance:   BlockInstance{BlockID: definition.ID, InstanceID: "demo1", Params: map[string]any{}, Refs: []string{"R1", "D1"}},
	}

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if !reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("issues = %#v, want missing footprint error", result.Issues)
	}
	if len(result.Components) != 0 {
		t.Fatalf("components = %#v, want none because placement operation failed", result.Components)
	}
}

func TestRealizeBlockPCBBlocksMissingMetadata(t *testing.T) {
	definition := minimalDefinition()
	output := dryRunBlockOutput(definition, BlockRequest{BlockID: definition.ID, InstanceID: "x"}, nil, nil)
	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if !reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("issues = %#v, want blocking missing realization issue", result.Issues)
	}
}

func mustComponentOps(t *testing.T, first BlockComponent, firstRef string, second BlockComponent, secondRef string) []transactions.Operation {
	t.Helper()
	firstOps, issues := ComponentOperations(first, firstRef, transactions.Point{})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("first component issues = %#v", issues)
	}
	secondOps, issues := ComponentOperations(second, secondRef, transactions.Point{})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("second component issues = %#v", issues)
	}
	return append(firstOps, secondOps...)
}

func mustSingleComponentOps(t *testing.T, component BlockComponent, ref string) []transactions.Operation {
	t.Helper()
	ops, issues := ComponentOperations(component, ref, transactions.Point{})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("component issues = %#v", issues)
	}
	return ops
}

func singleResistorOutput(t *testing.T, definition BlockDefinition) BlockOutput {
	t.Helper()
	return BlockOutput{
		Definition: Summary(definition),
		Instance: BlockInstance{
			BlockID:    definition.ID,
			InstanceID: "demo1",
			Params:     map[string]any{"footprint": "Device:R_0805"},
			Refs:       []string{"R1"},
		},
		Operations: mustSingleComponentOps(t,
			BlockComponent{Role: "resistor", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Device:R_0805", Pins: twoTerminalHorizontalPins()}, "R1",
		),
	}
}

func countOperations(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}
