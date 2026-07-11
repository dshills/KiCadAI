package blocks

import (
	"fmt"

	"kicadai/internal/components"
	"kicadai/internal/transactions"
)

const (
	i2cVCCSourceXMM              = 19.4
	bmp280VCCTrunkXMM            = i2cVCCSourceXMM
	bmp280VCCTrunkYMM            = 15.8
	ap2112RegulatorSymbol        = "Regulator_Linear:AP2112K-3.3"
	ap2112OutputGroundCapXMM     = 6.6
	ap2112GroundPinXMM           = -1.5
	ap2112OutputGroundChannelYMM = -6.0
)

func boolPtr(value bool) *bool {
	return &value
}

func ledIndicatorComponents() []BlockComponent {
	return []BlockComponent{
		{
			Role:              "resistor",
			RefPrefix:         "R",
			Value:             "330",
			SymbolID:          "Device:R",
			FootprintID:       "Resistor_SMD:R_0805_2012Metric",
			Pins:              twoTerminalHorizontalPins(),
			ComponentQuery:    &components.Query{Family: "resistor", Package: "0805", ValueKind: "resistance"},
			MinimumConfidence: components.ConfidenceRuleInferred,
			Acceptance:        components.AcceptanceConnectivity,
		},
		{
			Role:              "led",
			RefPrefix:         "D",
			Value:             "LED",
			SymbolID:          "Device:LED",
			FootprintID:       "LED_SMD:LED_0805_2012Metric",
			Pins:              twoTerminalHorizontalPins(),
			ComponentID:       "led.generic.0805",
			ComponentVariant:  "0805",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
		},
	}
}

func ledIndicatorPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "resistor", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "led", FootprintParam: "led_footprint", Placement: RelativePlacement{XMM: 5, YMM: 0, RotationDeg: 180, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "inline_indicator", ComponentRoles: []string{"resistor", "led"}, AnchorRole: "resistor", Bounds: &RelativeBounds{MinXMM: -2, MinYMM: -2, MaxXMM: 8, MaxYMM: 2}}},
		LocalRoutes:     []PCBLocalRoute{{ID: "series", NetTemplate: "led_series", From: RouteEndpoint{ComponentRole: "resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "led", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 5.08, YMM: -2}, {XMM: -0.08, YMM: -2}}, Layer: "B.Cu", WidthMM: 0.25, Required: true}},
		Validation:      PCBValidationExpectations{RequiredNets: []string{"in", "led_series", "gnd"}, RequiredRoutes: []string{"series"}},
	}
}

func connectorBreakoutComponents() []BlockComponent {
	return []BlockComponent{{
		Role:                     "connector",
		RefPrefix:                "J",
		Value:                    "Connector",
		SymbolID:                 defaultConnectorSymbol,
		FootprintID:              defaultConnectorFootprint,
		Pins:                     connectorSymbolPins(2),
		ComponentQuery:           &components.Query{Family: "connector", ValueKind: "pin_count", Value: "2"},
		ComponentValueParam:      "pin_count",
		ComponentPackageParam:    "connector_footprint",
		ComponentPackageTemplate: "1x%02d",
		ComponentPinsParam:       "pin_count",
		ComponentSymbolTemplate:  "Connector:Conn_01x%02d",
		MinimumConfidence:        components.ConfidenceVerified,
		Acceptance:               components.AcceptanceConnectivity,
	}}
}

func connectorBreakoutPCBRealization() *PCBRealization {
	edgeFacing := RealizationWhen{Params: map[string]any{"edge_facing": true}}
	notEdgeFacing := RealizationWhen{Params: map[string]any{"edge_facing": false}}
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "connector", FootprintParam: "connector_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}, When: notEdgeFacing},
			{ComponentRole: "connector", FootprintParam: "connector_footprint", Placement: RelativePlacement{XMM: 12, YMM: 14.5, Layer: "F.Cu"}, When: edgeFacing},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "connector_edge", ComponentRoles: []string{"connector"}, AnchorRole: "connector", Bounds: &RelativeBounds{MinXMM: -3, MinYMM: -10, MaxXMM: 3, MaxYMM: 10}}},
		Constraints:     []PCBConstraint{{ID: "connector_right_edge_facing", Kind: "edge_facing", AppliesTo: []string{"connector"}, Description: "Place generated connector breakouts on the right board edge when requested.", When: edgeFacing}},
		Validation:      PCBValidationExpectations{AllowedUnroutedNets: []string{"*"}},
	}
}

func combinedRealizationWhen(conditions ...RealizationWhen) RealizationWhen {
	combined := RealizationWhen{Params: map[string]any{}}
	for _, condition := range conditions {
		for key, value := range condition.Params {
			combined.Params[key] = value
		}
	}
	return combined
}

type voltageRegulatorComponentDefaults struct {
	OutputVoltage      string
	RegulatorSymbol    string
	RegulatorFootprint string
	RegulatorPins      []transactions.PinSpec
	InputCapacitance   string
	OutputCapacitance  string
	CapacitorFootprint string
	PowerLEDResistor   string
}

func voltageRegulatorComponents(defaults voltageRegulatorComponentDefaults) []BlockComponent {
	powerLEDResistorQuery := normalizeUnitLiteral(defaults.PowerLEDResistor, "Ω", resistanceMultipliers())
	powerLEDResistorFootprint := "Resistor_SMD:R_0805_2012Metric"
	powerLEDFootprint := "LED_SMD:LED_0805_2012Metric"
	return []BlockComponent{
		{Role: "regulator", RefPrefix: "U", Value: "LDO " + defaults.OutputVoltage, SymbolID: defaults.RegulatorSymbol, FootprintID: defaults.RegulatorFootprint, Pins: defaults.RegulatorPins, ComponentQuery: &components.Query{Family: "regulator", ValueKind: "output_voltage"}, ComponentValueParam: "output_voltage", ComponentPackageParam: "regulator_footprint", ComponentPinsParam: "regulator_symbol", MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceConnectivity},
		{Role: "input_capacitor", RefPrefix: "C", Value: defaults.InputCapacitance, SymbolID: "Device:C", FootprintID: defaults.CapacitorFootprint, Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentValueParam: "input_capacitance", ComponentPackageParam: "capacitor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "output_capacitor", RefPrefix: "C", Value: defaults.OutputCapacitance, SymbolID: "Device:C", FootprintID: defaults.CapacitorFootprint, Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentValueParam: "output_capacitance", ComponentPackageParam: "capacitor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "power_led_resistor", RefPrefix: "R", Value: defaults.PowerLEDResistor, SymbolID: "Device:R", FootprintID: powerLEDResistorFootprint, Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", Package: packageQueryFromFootprint(powerLEDResistorFootprint), ValueKind: "resistance", Value: powerLEDResistorQuery}, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, When: RealizationWhen{Params: map[string]any{"include_power_led": true}}},
		{Role: "power_led", RefPrefix: "D", Value: "POWER LED", SymbolID: "Device:LED", FootprintID: powerLEDFootprint, Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "led", Package: packageQueryFromFootprint(powerLEDFootprint)}, MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceConnectivity, When: RealizationWhen{Params: map[string]any{"include_power_led": true}}},
	}
}

func voltageRegulatorPCBRealization() *PCBRealization {
	ap2112 := RealizationWhen{Params: map[string]any{"regulator_symbol": ap2112RegulatorSymbol}}
	ap2112VINAnchor := []PCBAnchorPlacementVariant{{Placement: RelativePlacement{XMM: -7, YMM: -4, Layer: "F.Cu"}, When: ap2112}}
	ap2112VINEntry := []PCBRouteGeometryVariant{{Waypoints: []RelativePoint{{XMM: -6.8, YMM: -4}}, DisableEntryAnchorDogbone: true, DisableEntryAnchorVia: true, When: ap2112}}
	ap2112VOUTEntry := []PCBRouteGeometryVariant{{ClearWaypoints: true, DisableEntryAnchorDogbone: true, DisableEntryAnchorVia: true, When: ap2112}}
	ap2112GNDEntry := []PCBRouteGeometryVariant{{ClearWaypoints: true, Layer: "F.Cu", When: ap2112}}
	ap2112VINBypass := []PCBRouteGeometryVariant{{Waypoints: []RelativePoint{{XMM: -6.6, YMM: -2}, {XMM: -3, YMM: -2}, {XMM: -3, YMM: 0.95}}, When: ap2112}}
	ap2112VOUTBypass := []PCBRouteGeometryVariant{{ClearWaypoints: true, Layer: "B.Cu", When: ap2112}}
	ap2112DisableGenericGround := []PCBRouteGeometryVariant{{DisableRoute: true, When: ap2112}}
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "regulator", FootprintParam: "regulator_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "input_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -6, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "output_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: 6, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "power_led_resistor", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 13, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "power_led", FootprintID: "LED_SMD:LED_0805_2012Metric", Placement: RelativePlacement{XMM: 18, YMM: 0, RotationDeg: 180, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "vin", Port: "VIN", NetTemplate: "vin", Placement: RelativePlacement{XMM: -8, YMM: -4, Layer: "F.Cu"}, Variants: ap2112VINAnchor, Description: "Regulator input rail entry with clearance from adjacent ground-return access."},
			{ID: "vout", Port: "VOUT", NetTemplate: "vout", Placement: RelativePlacement{XMM: 5.4, YMM: -4, Layer: "F.Cu"}, Description: "Regulator output rail entry at the output capacitor pad."},
			{ID: "gnd", Port: "GND", NetTemplate: "gnd", Placement: RelativePlacement{XMM: -5.4, YMM: -4, Layer: "F.Cu"}, Description: "Regulator ground entry at the input capacitor return pad."},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "regulator_core", ComponentRoles: []string{"regulator", "input_capacitor", "output_capacitor"}, AnchorRole: "regulator", Bounds: &RelativeBounds{MinXMM: -9, MinYMM: -7, MaxXMM: 9, MaxYMM: 5}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "vin_entry", NetTemplate: "vin", From: RouteEndpoint{Port: "VIN"}, To: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "1"}, Waypoints: []RelativePoint{{XMM: -11.1, YMM: -4}, {XMM: -11.1, YMM: 13}}, GeometryVariants: ap2112VINEntry, Layer: "F.Cu", WidthMM: 0.5, Required: true, EntryAnchorDogbone: &PCBEntryAnchorDogbone{TieOffset: RelativePoint{XMM: -1, YMM: 0}, Description: "Tie the virtual top-edge VIN entry anchor via to copper on both layers for KiCad DRC."}},
			{ID: "vout_entry", NetTemplate: "vout", From: RouteEndpoint{Port: "VOUT"}, To: RouteEndpoint{ComponentRole: "output_capacitor", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 5.4, YMM: 22.5}, {XMM: -5.6, YMM: 22.5}}, GeometryVariants: ap2112VOUTEntry, Layer: "F.Cu", WidthMM: 0.5, Required: true, EntryAnchorDogbone: &PCBEntryAnchorDogbone{TieOffset: RelativePoint{XMM: -1, YMM: 0}, Description: "Tie the virtual top-edge VOUT entry anchor via to copper on both layers for KiCad DRC."}},
			{ID: "gnd_entry", NetTemplate: "gnd", From: RouteEndpoint{Port: "GND"}, To: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "2"}, Waypoints: []RelativePoint{{XMM: -1, YMM: -4}, {XMM: -1, YMM: 13}, {XMM: -9.9, YMM: 13}}, GeometryVariants: ap2112GNDEntry, Layer: "B.Cu", WidthMM: 0.5, Required: true, Description: "Bottom-layer ground entry avoids top-layer VIN crossing and ties into the regulator ground bypass through endpoint vias."},
			{ID: "vin_bypass", NetTemplate: "vin", From: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "3"}, Waypoints: []RelativePoint{{XMM: -11.1, YMM: 11.5}, {XMM: -2.45, YMM: 11.5}, {XMM: -2.45, YMM: 18.9}}, GeometryVariants: ap2112VINBypass, Layer: "F.Cu", WidthMM: 0.5, Required: true, Description: "Input bypass doglegs outside the AMS1117 VOUT tab clearance before returning to VIN pad 3."},
			{ID: "vout_bypass", NetTemplate: "vout", From: RouteEndpoint{ComponentRole: "output_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "2"}, GeometryVariants: ap2112VOUTBypass, Layer: "F.Cu", WidthMM: 0.5, Required: true, Description: "Output capacitor bypass to the regulator output pin; component pinmap resolution selects AP2112 pin 5."},
			{ID: "gnd_bypass", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "output_capacitor", Pin: "2"}, GeometryVariants: ap2112DisableGenericGround, Layer: "B.Cu", WidthMM: 0.5, Required: true, Description: "Bottom-layer ground bypass keeps regulator return connected while avoiding top-layer VOUT/VIN crossings; local-route emission adds endpoint vias for SMD pad access."},
			{ID: "ap2112_gnd_core", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "2"}, Layer: "B.Cu", WidthMM: 0.5, Required: true, When: ap2112, Description: "Bottom-layer AP2112 ground tie keeps the regulator return out of the compact VIN/EN fanout."},
			{ID: "ap2112_output_ground", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "output_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "2"}, Waypoints: []RelativePoint{{XMM: ap2112OutputGroundCapXMM, YMM: ap2112OutputGroundChannelYMM}, {XMM: ap2112GroundPinXMM, YMM: ap2112OutputGroundChannelYMM}}, Layer: "B.Cu", WidthMM: 0.5, Required: true, When: ap2112, Description: "Bottom-layer output-capacitor return reaches AP2112 ground above the VOUT corridor."},
			{ID: "ap2112_vin_core", NetTemplate: "vin", From: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "1"}, Waypoints: []RelativePoint{{XMM: -6.6, YMM: -2}, {XMM: -3, YMM: -2}}, Layer: "F.Cu", WidthMM: 0.5, Required: true, When: ap2112, Description: "Direct low-impedance input-capacitor path to AP2112 VIN pin 1."},
			{ID: "ap2112_vin_enable_tie", NetTemplate: "vin", From: RouteEndpoint{ComponentRole: "regulator", Pin: "1"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "3"}, Waypoints: []RelativePoint{{XMM: -2.4, YMM: -0.95}, {XMM: -2.4, YMM: 0.95}}, Layer: "F.Cu", WidthMM: 0.3, Required: true, When: ap2112, Description: "Tie AP2112 VIN to EN around the ground pad for always-on operation."},
		},
		Constraints: []PCBConstraint{
			{ID: "regulator_power_width", Kind: "min_width", NetTemplate: "vin", MinWidthMM: 0.5, Description: "Regulator input path should use a wider local route."},
			{ID: "regulator_output_width", Kind: "min_width", NetTemplate: "vout", MinWidthMM: 0.5, Description: "Regulator output path should use a wider local route."},
			{ID: "regulator_input_capacitor_proximity", Kind: "proximity", NetTemplate: "vin", AppliesTo: []string{"regulator", "input_capacitor"}, MaxLengthMM: 8, Description: "Input capacitor should remain close to the regulator input pin."},
			{ID: "regulator_output_capacitor_proximity", Kind: "proximity", NetTemplate: "vout", AppliesTo: []string{"regulator", "output_capacitor"}, MaxLengthMM: 8, Description: "Output capacitor should remain close to the regulator output pin."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vin", "vout", "gnd"}, RequiredRoutes: []string{"vin_entry", "vout_entry", "gnd_entry", "vin_bypass", "vout_bypass", "gnd_bypass"}},
	}
}

func i2cSensorComponents() []BlockComponent {
	pullupsEnabled := RealizationWhen{Params: map[string]any{"include_pullups": true}}
	return []BlockComponent{
		{Role: "sensor", RefPrefix: "U", Value: "I2C Sensor", SymbolID: defaultI2CSensorSymbol, FootprintID: "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm", Pins: i2cSensorPins(genericI2CSensorPins), ComponentIDParam: "sensor_component_id", MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true},
		{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: deviceCTemplatePins()},
		{Role: "sda_pullup", RefPrefix: "R", Value: "4.7k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: deviceRTemplatePins(), When: pullupsEnabled},
		{Role: "scl_pullup", RefPrefix: "R", Value: "4.7k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: deviceRTemplatePins(), When: pullupsEnabled},
	}
}

func i2cSensorPCBRealization() *PCBRealization {
	pullupsEnabled := RealizationWhen{Params: map[string]any{"include_pullups": true}}
	bmp280 := RealizationWhen{Params: map[string]any{"sensor_component_id": "sensor.bosch.bmp280.lga8"}}
	// The decoupling and both pull-up VCC pads share X=19.4 in this
	// realization. Replacing the generic detours with this point produces
	// vertical source-to-trunk segments and a horizontal approach to VDD.
	bmp280VCCTrunk := []PCBWaypointVariant{{Waypoints: []RelativePoint{{XMM: bmp280VCCTrunkXMM, YMM: bmp280VCCTrunkYMM}}, When: bmp280}}
	bmp280SDAEscape := []PCBWaypointVariant{{Waypoints: []RelativePoint{{XMM: 20.6, YMM: 9.5}, {XMM: 14, YMM: 9.5}, {XMM: 14, YMM: 15.635}, {XMM: 25.05, YMM: 15.635}, {XMM: 30, YMM: 10}}, When: bmp280}}
	bmp280SCLEscape := []PCBWaypointVariant{{Waypoints: []RelativePoint{{XMM: 20.6, YMM: 16.905}, {XMM: 25.05, YMM: 16.905}, {XMM: 31, YMM: 11}}, When: bmp280}}
	bmp280EndpointDogbone := []PCBEndpointVariant{{ToEndpointDogbone: true, When: bmp280}}
	fixedLayout := RealizationWhen{Params: map[string]any{"fixed_pcb_layout": true}}
	movableLayout := RealizationWhen{Params: map[string]any{"fixed_pcb_layout": false}}
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "sensor", FootprintParam: "sensor_footprint", Placement: RelativePlacement{XMM: 28, YMM: 15, Layer: "F.Cu"}, When: movableLayout},
			{ComponentRole: "decoupling_capacitor", FootprintParam: "decoupling_footprint", Placement: RelativePlacement{XMM: 20, YMM: 14.365, Layer: "F.Cu"}, When: movableLayout},
			{ComponentRole: "sda_pullup", FootprintParam: "pullup_footprint", Placement: RelativePlacement{XMM: 20, YMM: 11, Layer: "F.Cu"}, When: combinedRealizationWhen(movableLayout, pullupsEnabled)},
			{ComponentRole: "scl_pullup", FootprintParam: "pullup_footprint", Placement: RelativePlacement{XMM: 20, YMM: 20, Layer: "F.Cu"}, When: combinedRealizationWhen(movableLayout, pullupsEnabled)},
			{ComponentRole: "sensor", FootprintParam: "sensor_footprint", Placement: RelativePlacement{XMM: 28, YMM: 15, Layer: "F.Cu", Fixed: true}, When: fixedLayout},
			{ComponentRole: "decoupling_capacitor", FootprintParam: "decoupling_footprint", Placement: RelativePlacement{XMM: 20, YMM: 14.365, Layer: "F.Cu", Fixed: true}, When: fixedLayout},
			{ComponentRole: "sda_pullup", FootprintParam: "pullup_footprint", Placement: RelativePlacement{XMM: 20, YMM: 11, Layer: "F.Cu", Fixed: true}, When: combinedRealizationWhen(fixedLayout, pullupsEnabled)},
			{ComponentRole: "scl_pullup", FootprintParam: "pullup_footprint", Placement: RelativePlacement{XMM: 20, YMM: 20, Layer: "F.Cu", Fixed: true}, When: combinedRealizationWhen(fixedLayout, pullupsEnabled)},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "sensor_core", ComponentRoles: []string{"sensor", "decoupling_capacitor", "sda_pullup", "scl_pullup"}, AnchorRole: "sensor", Bounds: &RelativeBounds{MinXMM: -14, MinYMM: -6, MaxXMM: 3, MaxYMM: 7}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "vcc_decoupling", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.VCC}, Waypoints: []RelativePoint{{XMM: i2cVCCSourceXMM, YMM: 13.095}, {XMM: 25.05, YMM: 13.095}}, WaypointVariants: bmp280VCCTrunk, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "gnd_decoupling", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.GND}, Waypoints: []RelativePoint{{XMM: 25.05, YMM: 14.365}}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "sda_pullup_vcc", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "sda_pullup", Pin: "1"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.VCC}, Waypoints: []RelativePoint{{XMM: 16, YMM: 11}, {XMM: 16, YMM: 13.095}, {XMM: 25.05, YMM: 13.095}}, WaypointVariants: bmp280VCCTrunk, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: pullupsEnabled},
			{ID: "scl_pullup_vcc", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "scl_pullup", Pin: "1"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.VCC}, Waypoints: []RelativePoint{{XMM: 15, YMM: 20}, {XMM: 15, YMM: 13.095}, {XMM: 25.05, YMM: 13.095}}, WaypointVariants: bmp280VCCTrunk, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: pullupsEnabled},
			{ID: "sda_pullup", NetTemplate: "sda", From: RouteEndpoint{ComponentRole: "sda_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.SDA}, Waypoints: []RelativePoint{{XMM: 20.6, YMM: 9.5}, {XMM: 14, YMM: 9.5}, {XMM: 14, YMM: 15.635}, {XMM: 25.05, YMM: 15.635}}, WaypointVariants: bmp280SDAEscape, EndpointVariants: bmp280EndpointDogbone, Layer: "B.Cu", WidthMM: 0.25, Required: true, When: pullupsEnabled},
			{ID: "scl_pullup", NetTemplate: "scl", From: RouteEndpoint{ComponentRole: "scl_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.SCL}, Waypoints: []RelativePoint{{XMM: 20.6, YMM: 16.905}, {XMM: 25.05, YMM: 16.905}}, WaypointVariants: bmp280SCLEscape, EndpointVariants: bmp280EndpointDogbone, Layer: "B.Cu", WidthMM: 0.25, Required: true, When: pullupsEnabled},
			{ID: "bmp280_vddio_tie", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "sensor", Pin: "6"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: "8"}, Waypoints: []RelativePoint{{XMM: 28.325, YMM: 16.5}, {XMM: 27.025, YMM: 16.5}}, Layer: "F.Cu", WidthMM: 0.2, Required: true, When: bmp280},
			{ID: "bmp280_csb_tie", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "sensor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: "8"}, Waypoints: []RelativePoint{{XMM: 27.675, YMM: 13.4}, {XMM: 26.4, YMM: 13.4}, {XMM: 26.4, YMM: 15.8}}, Layer: "F.Cu", WidthMM: 0.2, Required: true, When: bmp280},
		},
		Constraints: []PCBConstraint{
			{ID: "i2c_decoupling_proximity", Kind: "proximity", NetTemplate: "vcc", AppliesTo: []string{"sensor", "decoupling_capacitor"}, MaxLengthMM: 5, Description: "Sensor decoupling capacitor should remain close to the sensor supply pins."},
			{ID: "i2c_bus_pullup_group", Kind: "shared_bus_pullup", AppliesTo: []string{"sda_pullup", "scl_pullup"}, Description: "SDA and SCL pull-ups must be owned once per bus."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vcc", "gnd", "sda", "scl"}, RequiredRoutes: []string{"vcc_decoupling", "gnd_decoupling", "sda_pullup_vcc", "scl_pullup_vcc", "sda_pullup", "scl_pullup"}},
	}
}

func amplifierInputBufferComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "input_coupling", RefPrefix: "C", Value: "1uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentValueParam: "coupling_capacitance", ComponentPackageParam: "capacitor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "bias_top", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, ComponentPackageParam: "resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "bias_bottom", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, ComponentPackageParam: "resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "input_stopper", RefPrefix: "R", Value: "100", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, ComponentValueParam: "input_stopper_value", ComponentPackageParam: "resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
	}
}

func amplifierInputBufferPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "input_stopper", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: -8, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "input_coupling", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "bias_top", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: 6, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "bias_bottom", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: 6, YMM: 5, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "input", Port: "IN", NetTemplate: "in", Placement: RelativePlacement{XMM: -12, YMM: 0, Layer: "F.Cu"}, Description: "Audio input side before coupling."},
			{ID: "output", Port: "OUT", NetTemplate: "out", Placement: RelativePlacement{XMM: 12, YMM: 0, Layer: "F.Cu"}, Description: "Biased signal output to the gain stage."},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "input_conditioning", ComponentRoles: []string{"input_stopper", "input_coupling", "bias_top", "bias_bottom"}, AnchorRole: "input_coupling", Bounds: &RelativeBounds{MinXMM: -14, MinYMM: -8, MaxXMM: 14, MaxYMM: 8}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "input_to_stopper", NetTemplate: "in", From: RouteEndpoint{Port: "IN"}, To: RouteEndpoint{ComponentRole: "input_stopper", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "stopper_to_coupling", NetTemplate: "pre_coupling", From: RouteEndpoint{ComponentRole: "input_stopper", Pin: "2"}, To: RouteEndpoint{ComponentRole: "input_coupling", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "coupled_output", NetTemplate: "out", From: RouteEndpoint{ComponentRole: "input_coupling", Pin: "2"}, To: RouteEndpoint{Port: "OUT"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "bias_reference", NetTemplate: "out", From: RouteEndpoint{ComponentRole: "bias_top", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_bottom", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "bias_to_signal", NetTemplate: "out", From: RouteEndpoint{ComponentRole: "bias_top", Pin: "2"}, To: RouteEndpoint{ComponentRole: "input_coupling", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "bias_vcc", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "bias_top", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "bias_gnd", NetTemplate: "gnd", From: RouteEndpoint{Port: "GND"}, To: RouteEndpoint{ComponentRole: "bias_bottom", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "amplifier_input_left_to_right", Kind: "signal_flow", AppliesTo: []string{"input_stopper", "input_coupling"}, Description: "Place input conditioning before active gain stages."},
			{ID: "amplifier_input_output_separation", Kind: "analog_separation", AppliesTo: []string{"input_coupling", "bias_top", "bias_bottom"}, ClearanceMM: 3, Description: "Keep high-impedance input nodes away from output-current copper."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"in", "pre_coupling", "out", "vcc", "gnd"}, RequiredRoutes: []string{"input_to_stopper", "stopper_to_coupling", "coupled_output", "bias_reference", "bias_to_signal", "bias_vcc", "bias_gnd"}},
	}
}

func opAmpGainStageComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "opamp", RefPrefix: "U", Value: "LMV321", SymbolID: defaultOpAmpSymbol, FootprintID: "Package_TO_SOT_SMD:SOT-23-5", Pins: opAmpPins(lmv321Pins), ComponentQuery: &components.Query{Text: "LMV321", Family: "opamp", Package: "sot23_5"}, Acceptance: components.AcceptanceConnectivity},
		{Role: "gain_to_ground", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "feedback", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "output_resistor", RefPrefix: "R", Value: "100", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "bias_top", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "bias_bottom", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "input_coupling", RefPrefix: "C", Value: "1uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
	}
}

func opAmpGainStagePCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "opamp", FootprintParam: "opamp_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "feedback", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: -4, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "gain_to_ground", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: -4, YMM: 5, Layer: "F.Cu"}},
			{ComponentRole: "decoupling_capacitor", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Placement: RelativePlacement{XMM: 4, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "output_resistor", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: 8, YMM: 0, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"include_output_resistor": true}}},
			{ComponentRole: "input_coupling", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Placement: RelativePlacement{XMM: -12, YMM: 0, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ComponentRole: "bias_top", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: -8, YMM: -5, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ComponentRole: "bias_bottom", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: -8, YMM: 5, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "in", Port: "IN", NetTemplate: "in", Placement: RelativePlacement{XMM: -14, YMM: 0, Layer: "F.Cu"}, Description: "Input signal entry before optional AC coupling."},
			{ID: "out", Port: "OUT", NetTemplate: "out", Placement: RelativePlacement{XMM: 10, YMM: 0, Layer: "F.Cu"}, Description: "Gain-stage output after optional output resistor."},
			{ID: "vcc", Port: "VCC", NetTemplate: "vcc", Placement: RelativePlacement{XMM: 0, YMM: -2.54, Layer: "F.Cu"}, Description: "Positive supply interface point aligned to the supported LMV321 VCC pad offset."},
			{ID: "gnd", Port: "GND", NetTemplate: "gnd", Placement: RelativePlacement{XMM: 0, YMM: 2.54, Layer: "F.Cu"}, Description: "Reference or negative supply interface point aligned to the supported LMV321 VEE pad offset."},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "opamp_core", ComponentRoles: []string{"opamp", "feedback", "gain_to_ground", "decoupling_capacitor", "input_coupling", "bias_top", "bias_bottom", "output_resistor"}, AnchorRole: "opamp", Bounds: &RelativeBounds{MinXMM: -14, MinYMM: -8, MaxXMM: 10, MaxYMM: 8}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "dc_input", NetTemplate: "in", From: RouteEndpoint{Port: "IN"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.INP}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"input_coupling": "dc"}}},
			{ID: "ac_input_coupling", NetTemplate: "in", From: RouteEndpoint{Port: "IN"}, To: RouteEndpoint{ComponentRole: "input_coupling", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ID: "ac_input_bias", NetTemplate: "bias", From: RouteEndpoint{ComponentRole: "input_coupling", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.INP}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ID: "bias_top", NetTemplate: "bias", From: RouteEndpoint{ComponentRole: "bias_top", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.INP}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ID: "bias_bottom", NetTemplate: "bias", From: RouteEndpoint{ComponentRole: "bias_bottom", Pin: "1"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.INP}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ID: "bias_vcc", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "bias_top", Pin: "1"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VCC}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ID: "bias_gnd", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "bias_bottom", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VEE}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"input_coupling": "ac"}}},
			{ID: "opamp_vcc_entry", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VCC}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "opamp_gnd_entry", NetTemplate: "gnd", From: RouteEndpoint{Port: "GND"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VEE}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "feedback_output", NetTemplate: "out", From: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.OUT}, To: RouteEndpoint{ComponentRole: "feedback", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"include_output_resistor": false}}},
			{ID: "feedback_output_drive", NetTemplate: "out_drive", From: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.OUT}, To: RouteEndpoint{ComponentRole: "feedback", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: RealizationWhen{Params: map[string]any{"include_output_resistor": true}}},
			{ID: "opamp_output_direct", NetTemplate: "out", From: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.OUT}, To: RouteEndpoint{Port: "OUT"}, Layer: "F.Cu", WidthMM: 0.3, Required: true, When: RealizationWhen{Params: map[string]any{"include_output_resistor": false}}},
			{ID: "output_resistor_input", NetTemplate: "out_drive", From: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.OUT}, To: RouteEndpoint{ComponentRole: "output_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.3, Required: true, When: RealizationWhen{Params: map[string]any{"include_output_resistor": true}}},
			{ID: "output_resistor_output", NetTemplate: "out", From: RouteEndpoint{ComponentRole: "output_resistor", Pin: "2"}, To: RouteEndpoint{Port: "OUT"}, Layer: "F.Cu", WidthMM: 0.3, Required: true, When: RealizationWhen{Params: map[string]any{"include_output_resistor": true}}},
			{ID: "feedback_loop", NetTemplate: "feedback", From: RouteEndpoint{ComponentRole: "feedback", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.INN}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "gain_reference", NetTemplate: "feedback", From: RouteEndpoint{ComponentRole: "gain_to_ground", Pin: "1"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.INN}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "gain_ground", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "gain_to_ground", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VEE}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "supply_decoupling", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VCC}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "supply_decoupling_return", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VEE}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "opamp_feedback_proximity", Kind: "proximity", NetTemplate: "feedback", AppliesTo: []string{"opamp", "feedback", "gain_to_ground"}, MaxLengthMM: 6, Description: "Feedback network should remain close to the op-amp inverting input."},
			{ID: "opamp_supply_decoupling_proximity", Kind: "proximity", NetTemplate: "vcc", AppliesTo: []string{"opamp", "decoupling_capacitor"}, MaxLengthMM: 5, Description: "Supply decoupling capacitor should remain close to the op-amp supply pins."},
			{ID: "opamp_input_output_separation", Kind: "analog_separation", AppliesTo: []string{"opamp", "feedback", "output_resistor"}, ClearanceMM: 3, Description: "Keep input and feedback nodes separated from output copper where placement permits."},
			{ID: "opamp_output_resistor_pairing", Kind: "output_pairing", NetTemplate: "out", AppliesTo: []string{"opamp", "output_resistor"}, MaxLengthMM: 6, Description: "Place the optional output resistor as the first output-side element after the op-amp pin."},
			{ID: "opamp_output_min_width", Kind: "high_current_width", NetTemplate: "out", AppliesTo: []string{"output_resistor"}, MinWidthMM: 0.5, Description: "Classify headphone/output paths for wider copper until load-current evidence proves a smaller width."},
			{ID: "opamp_thermal_edge_preference", Kind: "thermal_region", AppliesTo: []string{"opamp", "output_resistor"}, Description: "Prefer output-drive heat sources near board edge or copper-spread regions for later thermal review."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"in", "out", "feedback", "vcc", "gnd"}, RequiredRoutes: []string{"feedback_output", "feedback_loop", "gain_reference", "gain_ground", "supply_decoupling"}},
	}
}

func mcuMinimalComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "mcu", RefPrefix: "U", Value: "ATmega328P-A", SymbolID: defaultMCUSymbol, FootprintID: defaultMCUFootprint, Pins: mcuPins(supportedMCUTemplates[defaultMCUSymbol])},
		{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "aref_decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "reset_pullup", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "reset_switch", RefPrefix: "SW", Value: "RESET", SymbolID: resetSwitchSymbol, FootprintID: resetSwitchFootprint, Pins: twoTerminalHorizontalPins()},
		{Role: "isp_header", RefPrefix: "J", Value: "AVR ISP", SymbolID: ispHeaderSymbol, FootprintID: ispHeaderFootprint, Pins: twoByThreeHeaderPins()},
		{Role: "uart_header", RefPrefix: "J", Value: "UART", SymbolID: uartHeaderSymbol, FootprintID: uartHeaderFootprint, Pins: connectorSymbolPins(4)},
	}
}

func mcuMinimalPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "mcu", FootprintParam: "mcu_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "decoupling_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -8, YMM: -8, Layer: "F.Cu"}},
			{ComponentRole: "aref_decoupling_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: 8, YMM: -8, Layer: "F.Cu"}},
			{ComponentRole: "reset_pullup", FootprintParam: "reset_resistor_footprint", Placement: RelativePlacement{XMM: -10, YMM: 7, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "mcu_core", ComponentRoles: []string{"mcu", "decoupling_capacitor", "aref_decoupling_capacitor", "reset_pullup"}, AnchorRole: "mcu", Bounds: &RelativeBounds{MinXMM: -14, MinYMM: -14, MaxXMM: 14, MaxYMM: 14}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "mcu_vcc_decoupling", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "mcu", Pin: defaultMCUPrimaryVCCPin()}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "mcu_gnd_decoupling", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "mcu", Pin: defaultMCUPrimaryGNDPin()}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "mcu_aref_decoupling", NetTemplate: "aref", From: RouteEndpoint{ComponentRole: "aref_decoupling_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "mcu", Pin: defaultMCUAREFPin()}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "mcu_aref_ground", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "aref_decoupling_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "mcu", Pin: defaultMCUPrimaryGNDPin()}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "mcu_reset_pullup", NetTemplate: "reset", From: RouteEndpoint{ComponentRole: "reset_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "mcu", Pin: defaultMCUResetPin()}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "mcu_decoupling_proximity", Kind: "proximity", NetTemplate: "vcc", AppliesTo: []string{"mcu", "decoupling_capacitor", "aref_decoupling_capacitor"}, MaxLengthMM: 6, Description: "MCU decoupling capacitors should remain close to the package power pins."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vcc", "gnd", "reset", "aref"}, RequiredRoutes: []string{"mcu_vcc_decoupling", "mcu_gnd_decoupling", "mcu_aref_decoupling", "mcu_aref_ground", "mcu_reset_pullup"}},
		UnsupportedBehaviors: []string{
			"multiple decoupling capacitor instances share one component role until indexed component realization is implemented",
			"programming headers and reset switch placement are metadata-only until conditional realizations are implemented",
		},
	}
}

func defaultMCUPrimaryVCCPin() string {
	template, ok := supportedMCUTemplates[defaultMCUSymbol]
	if !ok || len(template.Roles.VCC) == 0 {
		panic(fmt.Sprintf("default MCU template %s missing VCC pins", defaultMCUSymbol))
	}
	return template.Roles.VCC[0]
}

func defaultMCUPrimaryGNDPin() string {
	template, ok := supportedMCUTemplates[defaultMCUSymbol]
	if !ok || len(template.Roles.GND) == 0 {
		panic(fmt.Sprintf("default MCU template %s missing GND pins", defaultMCUSymbol))
	}
	return template.Roles.GND[0]
}

func defaultMCUResetPin() string {
	template, ok := supportedMCUTemplates[defaultMCUSymbol]
	if !ok || template.Roles.RESET == "" {
		panic(fmt.Sprintf("default MCU template %s missing reset pin", defaultMCUSymbol))
	}
	return template.Roles.RESET
}

func defaultMCUAREFPin() string {
	template, ok := supportedMCUTemplates[defaultMCUSymbol]
	if !ok || template.Roles.AREF == "" {
		panic(fmt.Sprintf("default MCU template %s missing AREF pin", defaultMCUSymbol))
	}
	return template.Roles.AREF
}

func usbCPowerComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "usb_c_receptacle", RefPrefix: "J", Value: "USB-C Power", SymbolID: defaultUSBCSymbol, FootprintID: defaultUSBCFootprint, Pins: usbCSymbolPins(usbCPowerPins)},
		{Role: "cc1_rd", RefPrefix: "R", Value: "5.1k", SymbolID: "kicadai:USB_CC_R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: usbVerticalTwoTerminalPins()},
		{Role: "cc2_rd", RefPrefix: "R", Value: "5.1k", SymbolID: "kicadai:USB_CC_R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: usbVerticalTwoTerminalPins()},
		{Role: "vbus_fuse", RefPrefix: "F", Value: "Fuse", SymbolID: "Device:Fuse", FootprintID: "Fuse:Fuse_1206_3216Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "vbus_tvs", RefPrefix: "D", Value: "VBUS TVS", SymbolID: "Device:D_TVS", FootprintID: "Diode_SMD:D_SOD-323", Pins: twoTerminalHorizontalPins()},
		{Role: "bulk_capacitor", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "power_led_resistor", RefPrefix: "R", Value: usbPowerLEDResistorValue, SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "power_led", RefPrefix: "D", Value: "VBUS LED", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
	}
}

const (
	usbCPowerGroundReturnCorridorYMM = -5.0
	usbCPowerTVSGroundChannelXMM     = 20.0
	usbCPowerBulkGroundChannelXMM    = 18.0
	usbCPowerCC2GroundHubXMM         = 7.1
)

func usbCPowerPCBRealization() *PCBRealization {
	usbPowerRoles := []string{"usb_c_receptacle", "cc1_rd", "cc2_rd", "vbus_fuse", "vbus_tvs", "bulk_capacitor"}
	minimalPowerOnly := RealizationWhen{Params: map[string]any{"include_fuse": false, "include_tvs": false, "include_bulk_capacitor": false, "include_power_led": false}}
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "usb_c_receptacle", FootprintParam: "connector_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "cc1_rd", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 4.5, YMM: 3.5, Layer: "F.Cu", Fixed: true}},
			{ComponentRole: "cc2_rd", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 6.5, YMM: 1, Layer: "F.Cu", Fixed: true}},
			{ComponentRole: "vbus_fuse", FootprintID: "Fuse:Fuse_1206_3216Metric", Placement: RelativePlacement{XMM: 13, YMM: 1.5, Layer: "F.Cu", Fixed: true}},
			{ComponentRole: "vbus_tvs", FootprintID: "Diode_SMD:D_SOD-323", Placement: RelativePlacement{XMM: 18, YMM: 4, Layer: "F.Cu", Fixed: true}},
			{ComponentRole: "bulk_capacitor", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Placement: RelativePlacement{XMM: 18, YMM: -1, Layer: "F.Cu", Fixed: true}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "usb_c_power_entry", ComponentRoles: append([]string(nil), usbPowerRoles...), AnchorRole: "usb_c_receptacle", Bounds: &RelativeBounds{MinXMM: -5, MinYMM: -8, MaxXMM: 22, MaxYMM: 10}}},
		Keepouts: []PCBKeepout{
			{ID: "usb_c_edge_keepout", Layer: "F.Cu", Bounds: RelativeBounds{MinXMM: -5, MinYMM: -8, MaxXMM: 3, MaxYMM: 8}, AppliesTo: []string{"usb_c_receptacle"}, BlocksRoute: boolPtr(false), Description: "Reserve board-edge clearance around the USB-C receptacle."},
			{ID: "usb_c_power_entry_placement", Layer: "F.Cu", Bounds: RelativeBounds{MinXMM: -5, MinYMM: -8, MaxXMM: 23, MaxYMM: 12}, AppliesTo: append([]string(nil), usbPowerRoles...), BlocksRoute: boolPtr(false), Description: "Reserve placement area for USB-C power-entry companions."},
		},
		LocalRoutes: []PCBLocalRoute{
			{ID: "cc1_pull_down", NetTemplate: "cc1", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPins.CC1}, To: RouteEndpoint{ComponentRole: "cc1_rd", Pin: "1"}, Waypoints: []RelativePoint{{XMM: -0.5, YMM: -0.4}, {XMM: 2.4, YMM: -0.4}, {XMM: 2.4, YMM: 3.5}}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			// CC2 uses top-layer intermediate waypoints because the former B.Cu
			// via-in-pad route to cc2_rd pin 1 triggers KiCad's copper_sliver
			// warning in this placement.
			{ID: "cc2_pull_down", NetTemplate: "cc2", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPins.CC2}, To: RouteEndpoint{ComponentRole: "cc2_rd", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 0.5, YMM: -0.6}, {XMM: 5.0, YMM: -0.6}}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "vbus_entry_a", NetTemplate: "vbus_connector", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPinAt(usbCPowerPins.VBUS, 0, "A9")}, To: RouteEndpoint{ComponentRole: "vbus_fuse", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 1.52, YMM: -4.0}, {XMM: 11.6, YMM: -4.0}}, Layer: "B.Cu", WidthMM: 0.75, Required: true},
			{ID: "vbus_entry_b", NetTemplate: "vbus_connector", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPinAt(usbCPowerPins.VBUS, 1, "B9")}, To: RouteEndpoint{ComponentRole: "vbus_fuse", Pin: "1"}, Waypoints: []RelativePoint{{XMM: -1.52, YMM: -4.8}, {XMM: 11.6, YMM: -4.8}}, Layer: "B.Cu", WidthMM: 0.75, Required: true},
			{ID: "vbus_tvs", NetTemplate: "vbus_out", From: RouteEndpoint{ComponentRole: "vbus_fuse", Pin: "2"}, To: RouteEndpoint{ComponentRole: "vbus_tvs", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.8, Required: true, When: RealizationWhen{Params: map[string]any{"include_tvs": true}}},
			{ID: "vbus_bulk", NetTemplate: "vbus_out", From: RouteEndpoint{ComponentRole: "vbus_fuse", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bulk_capacitor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.8, Required: true, When: RealizationWhen{Params: map[string]any{"include_bulk_capacitor": true}}},
			{ID: "tvs_ground", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "vbus_tvs", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bulk_capacitor", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.8, Required: true, Description: "Short, wide TVS return into the adjacent bulk-capacitor ground node.", When: RealizationWhen{Params: map[string]any{"include_tvs": true, "include_bulk_capacitor": true}}},
			{
				ID:          "tvs_ground_fallback",
				NetTemplate: "gnd",
				From:        RouteEndpoint{ComponentRole: "vbus_tvs", Pin: "2"},
				To:          RouteEndpoint{ComponentRole: "cc2_rd", Pin: "2"},
				Waypoints: []RelativePoint{
					{XMM: usbCPowerTVSGroundChannelXMM, YMM: 4},
					{XMM: usbCPowerTVSGroundChannelXMM, YMM: usbCPowerGroundReturnCorridorYMM},
					{XMM: usbCPowerCC2GroundHubXMM, YMM: usbCPowerGroundReturnCorridorYMM},
				},
				Layer:       "F.Cu",
				WidthMM:     0.8,
				Required:    true,
				Description: "Fallback wide TVS ground route when the protected bulk capacitor is disabled.",
				When:        RealizationWhen{Params: map[string]any{"include_tvs": true, "include_bulk_capacitor": false}},
			},
			{
				ID:          "bulk_ground",
				NetTemplate: "gnd",
				From:        RouteEndpoint{ComponentRole: "bulk_capacitor", Pin: "2"},
				To:          RouteEndpoint{ComponentRole: "cc2_rd", Pin: "2"},
				Waypoints: []RelativePoint{
					{XMM: usbCPowerBulkGroundChannelXMM, YMM: usbCPowerGroundReturnCorridorYMM},
					{XMM: usbCPowerCC2GroundHubXMM, YMM: usbCPowerGroundReturnCorridorYMM},
				},
				Layer:       "F.Cu",
				WidthMM:     0.8,
				Required:    true,
				Description: "Wide ground return path from bulk capacitance into the local USB-C ground network.",
				When:        RealizationWhen{Params: map[string]any{"include_bulk_capacitor": true}},
			},
			{ID: "gnd_receptacle_pair", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPinAt(usbCPowerPins.GND, 0, "A12")}, To: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPinAt(usbCPowerPins.GND, 1, "B12")}, Waypoints: []RelativePoint{{XMM: 2.75, YMM: -5.8}, {XMM: -2.75, YMM: -5.8}}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "minimal_cc_ground_pair", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "cc2_rd", Pin: "2"}, To: RouteEndpoint{ComponentRole: "cc1_rd", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.4, Required: true, When: minimalPowerOnly, Description: "Join both CC pull-down returns locally before entering the receptacle ground return."},
			{ID: "minimal_cc_ground_return", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPinAt(usbCPowerPins.GND, 0, "A12")}, To: RouteEndpoint{ComponentRole: "cc2_rd", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 3.5, YMM: -2.5}, {XMM: 7.8, YMM: -2.5}, {XMM: 7.8, YMM: 1}}, Layer: "B.Cu", WidthMM: 0.4, Required: true, When: minimalPowerOnly, Description: "Bottom-layer CC return stays outside the top-layer CC2 and VBUS access corridor."},
		},
		Constraints: []PCBConstraint{
			{ID: "usb_c_vbus_width", Kind: "min_width", NetTemplate: "vbus_connector", MinWidthMM: 0.75, Description: "VBUS entry path should support requested current."},
			{ID: "usb_c_edge_facing", Kind: "edge_facing", AppliesTo: []string{"usb_c_receptacle"}, Description: "USB-C receptacle should be placed at the board edge."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vbus_connector", "vbus_out", "gnd", "cc1", "cc2"}, RequiredRoutes: []string{"cc1_pull_down", "cc2_pull_down", "vbus_entry_a", "vbus_entry_b", "vbus_tvs", "vbus_bulk", "tvs_ground", "bulk_ground", "gnd_receptacle_pair"}},
		UnsupportedBehaviors: []string{
			"USB2 data no-connect markers remain schematic metadata only",
			"shield policy routing depends on project grounding strategy",
		},
	}
}
