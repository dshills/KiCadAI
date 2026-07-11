package blocks

import (
	"context"
	"encoding/json"
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

func TestRealizeBlockPCBMatchesComponentsByEmittedRole(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("mcu_minimal")
	if !ok {
		t.Fatal("missing mcu_minimal")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "mcu_minimal", InstanceID: "mcu1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	rolesByRef := addSymbolRolesByRef(t, output.Operations)
	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	routeByID := map[string]RealizedPCBLocalRoute{}
	for _, route := range result.LocalRoutes {
		routeByID[route.ID] = route
	}
	aref := routeByID["mcu_aref_decoupling"]
	if aref.ID == "" {
		t.Fatalf("routes = %#v, want mcu_aref_decoupling", result.LocalRoutes)
	}
	if got := rolesByRef[aref.From.Ref]; got != "aref_decoupling_capacitor" {
		t.Fatalf("AREF route starts at ref %s role %q, want aref_decoupling_capacitor; routes = %#v", aref.From.Ref, got, result.LocalRoutes)
	}
	if aref.From.Pin != "1" || aref.To.Pin != defaultMCUAREFPin() {
		t.Fatalf("AREF route endpoints = %#v -> %#v", aref.From, aref.To)
	}
}

func TestRealizeBlockPCBPreservesRoleAfterConcreteComponentSelection(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("i2c_sensor")
	if !ok {
		t.Fatal("missing i2c_sensor")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "sensor1",
		Params: map[string]any{
			"i2c_address":     "0x76",
			"include_pullups": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}

	sensorRef := ""
	for _, operation := range output.Operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Role == "sensor" {
			sensorRef = payload.Ref
			break
		}
	}
	if sensorRef == "" {
		t.Fatal("sensor ref not emitted")
	}
	for index, operation := range output.Operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Role != "sensor" {
				continue
			}
			payload.LibraryID = "Sensor_Pressure:BMP280"
			output.Operations[index] = mustWrapRealizeTestOperation(t, operation.Op, operation.Ref, payload)
		case transactions.OpAssignFootprint:
			var payload transactions.AssignFootprintOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Ref != sensorRef {
				continue
			}
			payload.FootprintID = "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering"
			output.Operations[index] = mustWrapRealizeTestOperation(t, operation.Op, operation.Ref, payload)
		case transactions.OpPlaceFootprint:
			var payload transactions.PlaceFootprintOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Ref != sensorRef {
				continue
			}
			payload.FootprintID = "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering"
			output.Operations[index] = mustWrapRealizeTestOperation(t, operation.Op, operation.Ref, payload)
		}
	}
	facts, factIssues := componentFactsFromOperations(output.Operations)
	if len(factIssues) != 0 {
		t.Fatalf("component fact issues = %#v", factIssues)
	}
	if got := facts[sensorRef].FootprintID; got != "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering" {
		t.Fatalf("selected sensor fact footprint = %q; operations = %#v", got, output.Operations)
	}

	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	if result.RoleRefs["sensor"] != sensorRef {
		t.Fatalf("sensor role ref = %q, want %q", result.RoleRefs["sensor"], sensorRef)
	}
	if got := realizedFootprintForRole(result.Components, "sensor"); got != "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering" {
		t.Fatalf("sensor footprint = %q", got)
	}
	if len(result.LocalRoutes) != 6 {
		t.Fatalf("local routes = %d, want 6; issues = %#v", len(result.LocalRoutes), result.Issues)
	}
}

func TestRealizeBlockPCBI2CPullupVCCRoutesResolvePullupRefs(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("i2c_sensor")
	if !ok {
		t.Fatal("missing i2c_sensor")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "sensor1",
		Params: map[string]any{
			"i2c_address":     "0x48",
			"include_pullups": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	rolesByRef := addSymbolRolesByRef(t, output.Operations)
	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	routeByID := map[string]RealizedPCBLocalRoute{}
	for _, route := range result.LocalRoutes {
		routeByID[route.ID] = route
	}
	sdaVCC := routeByID["sda_pullup_vcc"]
	sclVCC := routeByID["scl_pullup_vcc"]
	if sdaVCC.ID == "" || sclVCC.ID == "" {
		t.Fatalf("routes = %#v, want pull-up VCC local routes", result.LocalRoutes)
	}
	if got := rolesByRef[sdaVCC.From.Ref]; got != "sda_pullup" {
		t.Fatalf("SDA pull-up VCC route starts at ref %s role %q, want sda_pullup; routes = %#v", sdaVCC.From.Ref, got, result.LocalRoutes)
	}
	if got := rolesByRef[sclVCC.From.Ref]; got != "scl_pullup" {
		t.Fatalf("SCL pull-up VCC route starts at ref %s role %q, want scl_pullup; routes = %#v", sclVCC.From.Ref, got, result.LocalRoutes)
	}
	if sdaVCC.From == sclVCC.From {
		t.Fatalf("pull-up VCC routes share from endpoint %#v, want distinct pull-up refs", sdaVCC.From)
	}
}

func TestRealizeBlockPCBUsesConcreteI2CSensorPortPins(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("i2c_sensor")
	if !ok {
		t.Fatal("missing i2c_sensor")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "sensor1",
		Params: map[string]any{
			"sensor_component_id": "sensor.bosch.bmp280.lga8",
			"i2c_address":         "0x76",
			"include_pullups":     true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	routes := map[string]RealizedPCBLocalRoute{}
	for _, route := range result.LocalRoutes {
		routes[route.ID] = route
	}
	for routeID, wantPin := range map[string]string{
		"vcc_decoupling": "8",
		"gnd_decoupling": "1",
		"sda_pullup":     "3",
		"scl_pullup":     "4",
	} {
		route := routes[routeID]
		if route.To.Ref != result.RoleRefs["sensor"] || route.To.Pin != wantPin {
			t.Fatalf("route %s to = %#v, want sensor pin %s", routeID, route.To, wantPin)
		}
	}
	if route := routes["bmp280_vddio_tie"]; route.From.Pin != "6" || route.To.Pin != "8" {
		t.Fatalf("BMP280 VDDIO tie = %#v -> %#v", route.From, route.To)
	}
	if route := routes["bmp280_csb_tie"]; route.From.Pin != "2" || route.To.Pin != "6" {
		t.Fatalf("BMP280 CSB tie = %#v -> %#v", route.From, route.To)
	}
	for _, routeID := range []string{"vcc_decoupling", "sda_pullup_vcc", "scl_pullup_vcc"} {
		points := routes[routeID].Points
		if len(points) != 3 || points[1].XMM != bmp280VCCTrunkXMM || points[1].YMM != bmp280VCCTrunkYMM {
			t.Fatalf("BMP280 route %s does not use left-side VCC trunk: %#v", routeID, points)
		}
	}
}

func TestRealizeBlockPCBAddsAP2112VINEnableTie(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("voltage_regulator")
	if !ok {
		t.Fatal("missing voltage_regulator")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail",
		Params: map[string]any{
			"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
			"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
			"input_voltage":       "5V",
			"output_voltage":      "3.3V",
			"output_current":      "0.05A",
			"enable_mode":         "tied_input",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	for _, route := range result.LocalRoutes {
		if route.ID == "ap2112_vin_enable_tie" {
			if route.From.Pin != "1" || route.To.Pin != "3" || route.WidthMM != 0.3 {
				t.Fatalf("AP2112 tie = %#v", route)
			}
			return
		}
	}
	t.Fatalf("AP2112 VIN/EN tie missing: routes=%#v", result.LocalRoutes)
}

func TestRealizeBlockPCBI2COmitsPullupRoutesWhenDisabled(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("i2c_sensor")
	if !ok {
		t.Fatal("missing i2c_sensor")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "sensor1",
		Params: map[string]any{
			"i2c_address":     "0x48",
			"include_pullups": false,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("realize issues = %#v", result.Issues)
	}
	routeByID := map[string]bool{}
	for _, route := range result.LocalRoutes {
		routeByID[route.ID] = true
	}
	for _, omitted := range []string{"sda_pullup_vcc", "scl_pullup_vcc", "sda_pullup", "scl_pullup"} {
		if routeByID[omitted] {
			t.Fatalf("routes = %#v, want %s omitted when pull-ups disabled", result.LocalRoutes, omitted)
		}
		for _, required := range result.Validation.RequiredRoutes {
			if required == omitted {
				t.Fatalf("required routes = %#v, want %s omitted when pull-ups disabled", result.Validation.RequiredRoutes, omitted)
			}
		}
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
	if len(result.LocalRoutes[0].Points) != 3 || result.LocalRoutes[0].Points[1].XMM != 12 || result.LocalRoutes[0].Points[1].YMM != 22 {
		t.Fatalf("realized local route points = %#v, want offset waypoint at 12,22", result.LocalRoutes[0].Points)
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

func mustWrapRealizeTestOperation(t *testing.T, kind transactions.OperationKind, ref string, payload any) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return transactions.Operation{Op: kind, Ref: ref, Raw: raw}
}

func realizedFootprintForRole(components []RealizedPCBComponent, role string) string {
	for _, component := range components {
		if component.ComponentRole == role {
			return component.FootprintID
		}
	}
	return ""
}

func addSymbolRolesByRef(t *testing.T, operations []transactions.Operation) map[string]string {
	t.Helper()
	roles := map[string]string{}
	for _, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("decode add_symbol: %v", err)
		}
		roles[payload.Ref] = payload.Role
	}
	return roles
}
