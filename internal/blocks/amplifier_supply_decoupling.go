package blocks

import (
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const amplifierSupplyDecouplingID = "amplifier_supply_decoupling"

func amplifierSupplyDecouplingDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          amplifierSupplyDecouplingID,
		Name:        "Amplifier Supply Decoupling",
		Description: "Local amplifier rail decoupling fragment with ceramic and optional bulk capacitors.",
		Version:     "0.1.0",
		Category:    "power",
		Parameters: []BlockParameter{
			{Name: "rail_mode", Type: ParameterEnum, Default: "single_supply", Allowed: []any{"single_supply", "dual_supply"}, Description: "Supply topology. Single-supply uses VCC to GND; dual-supply also decouples VEE to GND."},
			{Name: "rail_voltage", Type: ParameterVoltage, Default: "9V", Description: "Maximum rail voltage seen by each decoupling capacitor."},
			{Name: "ceramic_capacitance", Type: ParameterCapacitance, Default: "100nF", Description: "Local high-frequency ceramic decoupling capacitor value."},
			{Name: "bulk_capacitance", Type: ParameterCapacitance, Default: "10uF", Description: "Optional local bulk capacitor value."},
			{Name: "include_bulk", Type: ParameterBool, Default: true, Description: "Emit local bulk capacitors in addition to ceramic capacitors."},
			{Name: "capacitor_voltage_rating", Type: ParameterVoltage, Default: "16V", Description: "Rated voltage of emitted decoupling capacitors."},
			{Name: "ceramic_component_id", Type: ParameterString, Default: "", Description: "Optional concrete stable-capacitance local bypass component ID."},
			{Name: "bulk_component_id", Type: ParameterString, Default: "", Description: "Optional concrete low-ESR local bulk component ID."},
			{Name: "ceramic_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0805_2012Metric", Description: "Ceramic capacitor footprint."},
			{Name: "bulk_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_1210_3225Metric", Description: "Bulk capacitor footprint."},
		},
		Ports: []BlockPort{
			{Name: "VCC", Direction: PortPower, Voltage: "rail_voltage", Description: "Positive amplifier rail."},
			{Name: "VEE", Direction: PortPower, Description: "Negative rail for dual-supply stages."},
			{Name: "GND", Direction: PortPower, Description: "Local ground/reference return."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Non-polar ceramic decoupling capacitor symbol."},
			{Kind: "symbol", ID: "Device:C_Polarized", Required: true, Description: "Default bulk capacitor symbol."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_0805_2012Metric", Required: true, Description: "Default ceramic capacitor footprint."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_1210_3225Metric", Required: true, Description: "Default bulk capacitor footprint."},
		},
		Components: amplifierSupplyDecouplingComponents(),
		Nets: []BlockNet{
			{NameTemplate: "vcc", Visibility: "exported", Role: "positive_rail", Pins: []NetPin{{ComponentRole: "vcc_ceramic", Pin: "1"}, {ComponentRole: "vcc_bulk", Pin: "1", When: RealizationWhen{Params: map[string]any{"include_bulk": true}}}}},
			{NameTemplate: "vee", Visibility: "exported", Role: "negative_rail", Pins: []NetPin{{ComponentRole: "vee_ceramic", Pin: "1", When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply"}}}, {ComponentRole: "vee_bulk", Pin: "2", When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply", "include_bulk": true}}}}},
			{NameTemplate: "gnd", Visibility: "exported", Role: "local_return", Pins: []NetPin{{ComponentRole: "vcc_ceramic", Pin: "2"}, {ComponentRole: "vcc_bulk", Pin: "2", When: RealizationWhen{Params: map[string]any{"include_bulk": true}}}, {ComponentRole: "vee_ceramic", Pin: "2", When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply"}}}, {ComponentRole: "vee_bulk", Pin: "1", When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply", "include_bulk": true}}}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "decoupling", ComponentRole: "vcc_ceramic", XMM: 10, YMM: -8, Note: "Place ceramic VCC decoupling near the active amplifier device."},
			{Kind: "decoupling", ComponentRole: "vcc_bulk", XMM: 20, YMM: -8, Note: "Place bulk VCC decoupling near the local stage rail entry."},
			{Kind: "decoupling", ComponentRole: "vee_ceramic", XMM: 10, YMM: 8, Note: "Place ceramic VEE decoupling near the active amplifier device for dual-supply stages."},
			{Kind: "decoupling", ComponentRole: "vee_bulk", XMM: 20, YMM: 8, Note: "Place bulk VEE decoupling near the local stage rail entry for dual-supply stages."},
		},
		PCBRealization: amplifierSupplyDecouplingPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "amplifier_decoupling.rail_voltage.positive", Severity: BlockValidationSeverityBlocked, Description: "Rail voltage must be positive."},
			{ID: "amplifier_decoupling.cap_voltage.derated", Severity: BlockValidationSeverityBlocked, Description: "Capacitor voltage rating must be at least 1.5x the rail voltage."},
			{ID: "amplifier_decoupling.local", Severity: BlockValidationSeverityBlocked, Description: "Decoupling capacitors must be placed near the active amplifier stage."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Structural rail decoupling evidence only; power integrity, rail splitter, and stability simulation remain future work."},
		},
	}
}

func amplifierSupplyDecouplingComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "vcc_ceramic", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentIDParam: "ceramic_component_id", ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentValueParam: "ceramic_capacitance", ComponentVoltageParam: "capacitor_voltage_rating", ComponentPackageParam: "ceramic_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "local_decoupling"},
		{Role: "vcc_bulk", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C_Polarized", FootprintID: "Capacitor_SMD:C_1210_3225Metric", Pins: twoTerminalHorizontalPins(), ComponentIDParam: "bulk_component_id", ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentValueParam: "bulk_capacitance", ComponentVoltageParam: "capacitor_voltage_rating", ComponentPackageParam: "bulk_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "local_decoupling", When: RealizationWhen{Params: map[string]any{"include_bulk": true}}},
		{Role: "vee_ceramic", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentIDParam: "ceramic_component_id", ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentValueParam: "ceramic_capacitance", ComponentVoltageParam: "capacitor_voltage_rating", ComponentPackageParam: "ceramic_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "local_decoupling", When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply"}}},
		{Role: "vee_bulk", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C_Polarized", FootprintID: "Capacitor_SMD:C_1210_3225Metric", Pins: twoTerminalHorizontalPins(), ComponentIDParam: "bulk_component_id", ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentValueParam: "bulk_capacitance", ComponentVoltageParam: "capacitor_voltage_rating", ComponentPackageParam: "bulk_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "local_decoupling", When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply", "include_bulk": true}}},
	}
}

func amplifierSupplyDecouplingPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "vcc_ceramic", FootprintParam: "ceramic_footprint", Placement: RelativePlacement{XMM: 0, YMM: -5, Layer: "F.Cu"}},
			{ComponentRole: "vcc_bulk", FootprintParam: "bulk_footprint", Placement: RelativePlacement{XMM: 10, YMM: -5, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"include_bulk": true}}},
			{ComponentRole: "vee_ceramic", FootprintParam: "ceramic_footprint", Placement: RelativePlacement{XMM: 0, YMM: 5, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply"}}},
			{ComponentRole: "vee_bulk", FootprintParam: "bulk_footprint", Placement: RelativePlacement{XMM: 10, YMM: 5, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply", "include_bulk": true}}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "vcc", Port: "VCC", NetTemplate: "vcc", Placement: RelativePlacement{XMM: -4, YMM: -5, Layer: "F.Cu"}, Description: "Positive-rail entry for the local decoupling star."},
			{ID: "vee", Port: "VEE", NetTemplate: "vee", Placement: RelativePlacement{XMM: -4, YMM: 5, Layer: "F.Cu"}, When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply"}}, Description: "Negative-rail entry for dual-supply local decoupling."},
			{ID: "gnd", Port: "GND", NetTemplate: "gnd", Placement: RelativePlacement{XMM: 3, YMM: 0, Layer: "F.Cu"}, Description: "Local return entry shared by the rail bypass branches."},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "local_decoupling", ComponentRoles: []string{"vcc_ceramic", "vcc_bulk", "vee_ceramic", "vee_bulk"}, AnchorRole: "vcc_ceramic", Bounds: &RelativeBounds{MinXMM: -5, MinYMM: -10, MaxXMM: 15, MaxYMM: 10}, TranslateAsUnit: true, Description: "Keep amplifier rail decoupling local to the active stage and preserve its routed rail/return star."}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "vcc_ceramic", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "vcc_ceramic", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true},
			{ID: "vcc_ceramic_gnd", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "vcc_ceramic", Pin: "2"}, To: RouteEndpoint{Port: "GND"}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true},
			{ID: "vcc_bulk", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "vcc_bulk", Pin: "1"}, Waypoints: []RelativePoint{{XMM: -4, YMM: -8}, {XMM: 10, YMM: -8}}, Layer: "F.Cu", WidthMM: 0.6, Required: true, DisableEntryAnchorVia: true, When: RealizationWhen{Params: map[string]any{"include_bulk": true}}},
			{ID: "vcc_bulk_gnd", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "vcc_bulk", Pin: "2"}, To: RouteEndpoint{Port: "GND"}, Layer: "F.Cu", WidthMM: 0.6, Required: true, DisableEntryAnchorVia: true, When: RealizationWhen{Params: map[string]any{"include_bulk": true}}},
			{ID: "vee_ceramic", NetTemplate: "vee", From: RouteEndpoint{Port: "VEE"}, To: RouteEndpoint{ComponentRole: "vee_ceramic", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true, When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply"}}},
			{ID: "vee_ceramic_gnd", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "vee_ceramic", Pin: "2"}, To: RouteEndpoint{Port: "GND"}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true, When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply"}}},
			{ID: "vee_bulk", NetTemplate: "vee", From: RouteEndpoint{Port: "VEE"}, To: RouteEndpoint{ComponentRole: "vee_bulk", Pin: "2"}, Waypoints: []RelativePoint{{XMM: -4, YMM: 8}, {XMM: 14, YMM: 8}}, Layer: "F.Cu", WidthMM: 0.6, Required: true, DisableEntryAnchorVia: true, When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply", "include_bulk": true}}},
			{ID: "vee_bulk_gnd", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "vee_bulk", Pin: "1"}, To: RouteEndpoint{Port: "GND"}, Layer: "F.Cu", WidthMM: 0.6, Required: true, DisableEntryAnchorVia: true, When: RealizationWhen{Params: map[string]any{"rail_mode": "dual_supply", "include_bulk": true}}},
		},
		Constraints: []PCBConstraint{{ID: "amplifier_decoupling_proximity", Kind: "proximity", AppliesTo: []string{"vcc_ceramic", "vcc_bulk", "vee_ceramic", "vee_bulk"}, MaxLengthMM: 8, Description: "Local decoupling must remain near the active amplifier supply pins."}},
	}
}

func instantiateAmplifierSupplyDecoupling(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	params = ApplyParameterDefaults(definition, params)
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}
	railVoltage, railOK := parseUnit(params["rail_voltage"], "V", voltageMultipliers())
	if !railOK || railVoltage <= 0 {
		issues = append(issues, blockIssue("params.rail_voltage", "rail_voltage must be a positive voltage literal"))
	}
	ratingVoltage, ratingOK := parseUnit(params["capacitor_voltage_rating"], "V", voltageMultipliers())
	if !ratingOK {
		issues = append(issues, blockIssue("params.capacitor_voltage_rating", "capacitor_voltage_rating must be a voltage literal"))
	} else if railOK && ratingVoltage < railVoltage*1.5 {
		issues = append(issues, blockIssue("params.capacitor_voltage_rating", "capacitor voltage rating must be at least 1.5x rail_voltage"))
	}
	ceramicFarads, ceramicOK := parseUnit(params["ceramic_capacitance"], "F", capacitanceMultipliers())
	if !ceramicOK || ceramicFarads <= 0 {
		issues = append(issues, blockIssue("params.ceramic_capacitance", "ceramic_capacitance must be a positive capacitance literal"))
	}
	bulkFarads, bulkOK := parseUnit(params["bulk_capacitance"], "F", capacitanceMultipliers())
	if boolParam(params, "include_bulk", true) && (!bulkOK || bulkFarads <= 0) {
		issues = append(issues, blockIssue("params.bulk_capacitance", "bulk_capacitance must be positive when include_bulk is true"))
	}
	if stringParam(params, "ceramic_footprint") == "" {
		issues = append(issues, blockIssue("params.ceramic_footprint", "ceramic_footprint is required"))
	}
	if boolParam(params, "include_bulk", true) && stringParam(params, "bulk_footprint") == "" {
		issues = append(issues, blockIssue("params.bulk_footprint", "bulk_footprint is required when include_bulk is true"))
	}
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}

	componentsByRole := blockComponentByRole(definition.Components)
	points := amplifierSupplyDecouplingHintPoints(definition)
	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	dualSupply := stringParam(params, "rail_mode") == "dual_supply"
	includeBulk := boolParam(params, "include_bulk", true)
	roles := []string{"vcc_ceramic"}
	if includeBulk {
		roles = append(roles, "vcc_bulk")
	}
	if dualSupply {
		roles = append(roles, "vee_ceramic")
		if includeBulk {
			roles = append(roles, "vee_bulk")
		}
	}
	refsByRole := make(map[string]string, len(roles))
	var refs []string
	var operations []transactions.Operation
	for _, role := range roles {
		component := componentsByRole[role]
		if component.ComponentValueParam != "" {
			component.Value = stringParam(params, component.ComponentValueParam)
		}
		if component.ComponentPackageParam != "" {
			component.FootprintID = stringParam(params, component.ComponentPackageParam)
		}
		if component.ComponentIDParam != "" {
			component.ComponentID = stringParam(params, component.ComponentIDParam)
		}
		ref := allocator.Next("C")
		refsByRole[role] = ref
		refs = append(refs, ref)
		componentOps, componentIssues := ComponentOperations(component, ref, points[role])
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}
	vccNet := InstanceNetName(request.InstanceID, "vcc")
	veeNet := InstanceNetName(request.InstanceID, "vee")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	// Connect operations carry the authoritative named rails into both the
	// schematic graph and PCB net table. Standalone labels are intentionally not
	// emitted here: without a wire anchor they are visually suggestive but
	// electrically detached, which weakens writer-correctness evidence.
	appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", refsByRole["vcc_ceramic"], "1", vccNet)
	appendConnectOperation(&operations, &issues, refsByRole["vcc_ceramic"], "2", request.InstanceID, "GND", gndNet)
	if includeBulk {
		appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", refsByRole["vcc_bulk"], "1", vccNet)
		appendConnectOperation(&operations, &issues, refsByRole["vcc_bulk"], "2", request.InstanceID, "GND", gndNet)
	}
	nets := []string{vccNet, gndNet}
	if dualSupply {
		appendConnectOperation(&operations, &issues, request.InstanceID, "VEE", refsByRole["vee_ceramic"], "1", veeNet)
		appendConnectOperation(&operations, &issues, refsByRole["vee_ceramic"], "2", request.InstanceID, "GND", gndNet)
		if includeBulk {
			appendConnectOperation(&operations, &issues, request.InstanceID, "VEE", refsByRole["vee_bulk"], "2", veeNet)
			appendConnectOperation(&operations, &issues, refsByRole["vee_bulk"], "1", request.InstanceID, "GND", gndNet)
		}
		nets = append(nets, veeNet)
	}
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func amplifierSupplyDecouplingHintPoints(definition BlockDefinition) map[string]transactions.Point {
	points := make(map[string]transactions.Point, len(definition.SchematicHints))
	for _, hint := range definition.SchematicHints {
		if hint.ComponentRole != "" {
			points[hint.ComponentRole] = transactions.Point{XMM: hint.XMM, YMM: hint.YMM}
		}
	}
	return points
}
