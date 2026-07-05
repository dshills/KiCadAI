package blocks

import (
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const classABOutputPairID = "class_ab_output_pair"

func classABOutputPairDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          classABOutputPairID,
		Name:        "Class AB Output Pair",
		Description: "Complementary emitter-follower output pair for headphone-class Class AB amplifiers.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "9V", Description: "Rail-to-rail supply voltage used for output-device screening."},
			{Name: "load_impedance", Type: ParameterResistance, Default: "32Ω", Description: "Nominal headphone load impedance."},
			{Name: "upper_output_component_id", Type: ParameterString, Default: "bjt.onsemi.mmbt3904.sot23", Description: "Verified NPN output-device component ID."},
			{Name: "lower_output_component_id", Type: ParameterString, Default: "bjt.onsemi.mmbt3906.sot23", Description: "Verified PNP output-device component ID."},
			{Name: "emitter_resistor_value", Type: ParameterResistance, Default: "0.47Ω", Description: "Small emitter resistor used to stabilize quiescent current."},
			{Name: "emitter_resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_1206_3216Metric", Description: "Footprint for output emitter resistors."},
			{Name: "output_footprint", Type: ParameterFootprintID, Default: "Package_TO_SOT_SMD:SOT-23", Description: "Footprint for the complementary output BJTs."},
			{Name: "application", Type: ParameterEnum, Default: "headphone", Allowed: []any{"headphone", "speaker", "power_amplifier"}, Description: "Load class for SOA/thermal gating."},
		},
		Ports: []BlockPort{
			{Name: "BIAS_P", Direction: PortInput, Description: "Upper output-device base bias node."},
			{Name: "BIAS_N", Direction: PortInput, Description: "Lower output-device base bias node."},
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Positive output-device collector rail."},
			{Name: "VEE", Direction: PortPower, Description: "Negative rail or ground return for single-supply headphone stages."},
			{Name: "AMP_OUT", Direction: PortOutput, Description: "Emitter-follower output node."},
			{Name: "LOAD_REF", Direction: PortPassive, Description: "Load return or virtual ground reference."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:Q_NPN_BEC", Required: true, Description: "NPN output transistor symbol."},
			{Kind: "symbol", ID: "Device:Q_PNP_BEC", Required: true, Description: "PNP output transistor symbol."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Emitter resistor symbol."},
			{Kind: "symbol", ID: "Connector:TestPoint", Required: true, Description: "Load-reference anchor symbol."},
		},
		Components:     classABOutputPairComponents(),
		PCBRealization: classABOutputPairPCBRealization(),
		Nets: []BlockNet{
			{NameTemplate: "bias_p", Visibility: "exported", Role: "upper_output_bias", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "1"}}},
			{NameTemplate: "bias_n", Visibility: "exported", Role: "lower_output_bias", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "1"}}},
			{NameTemplate: "upper_emitter", Visibility: "local", Role: "upper_emitter_resistor_input", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "2"}, {ComponentRole: "upper_emitter_resistor", Pin: "1"}}},
			{NameTemplate: "lower_emitter", Visibility: "local", Role: "lower_emitter_resistor_input", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "2"}, {ComponentRole: "lower_emitter_resistor", Pin: "1"}}},
			{NameTemplate: "amp_out", Visibility: "exported", Role: "amplifier_output", Pins: []NetPin{{ComponentRole: "upper_emitter_resistor", Pin: "2"}, {ComponentRole: "lower_emitter_resistor", Pin: "2"}}},
			{NameTemplate: "vcc", Visibility: "exported", Role: "positive_rail", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "3"}}},
			{NameTemplate: "vee", Visibility: "exported", Role: "negative_rail", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "3"}}},
			{NameTemplate: "load_ref", Visibility: "exported", Role: "load_reference", Pins: []NetPin{{ComponentRole: "load_reference", Pin: "1"}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "output_pair", ComponentRole: "upper_output", XMM: 45, YMM: 0, Note: "Place NPN device above the output node."},
			{Kind: "output_pair", ComponentRole: "lower_output", XMM: 45, YMM: 5.08, Note: "Place PNP device below the output node."},
			{Kind: "output_pair", ComponentRole: "upper_emitter_resistor", XMM: 56, YMM: 0, Note: "Keep the upper emitter resistor close to the NPN emitter."},
			{Kind: "output_pair", ComponentRole: "lower_emitter_resistor", XMM: 56, YMM: 5.08, Note: "Keep the lower emitter resistor close to the PNP emitter."},
			{Kind: "load_reference", ComponentRole: "load_reference", XMM: 65, YMM: 12, Note: "Keep load return separate from the negative rail unless the parent design intentionally ties them."},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "class_ab_output_pair.load.headphone_class", Severity: BlockValidationSeverityBlocked, Description: "Initial realization is limited to headphone loads with peak current below verified output-device ratings."},
			{ID: "class_ab_output_pair.supported_ids", Severity: BlockValidationSeverityBlocked, Description: "Output-device IDs must match the verified complementary MMBT3904/MMBT3906 pair until selector-backed realization lands."},
			{ID: "class_ab_output_pair.soa", Severity: BlockValidationSeverityBlocked, Description: "Speaker and power-amplifier output pairs require SOA and thermal evidence."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"component:bjt.onsemi.mmbt3904.sot23",
				"component:bjt.onsemi.mmbt3906.sot23",
				"amplifier.output_pair:headphone_class",
			},
			Notes: []string{
				"Structural realization emits a complementary emitter follower pair for headphone-class schematics.",
				"Thermal design, quiescent-current trim, stability compensation, and high-current PCB evidence are still blocked for fabrication claims.",
			},
		},
	}
}

func classABOutputPairComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "upper_output", RefPrefix: "Q", Value: "MMBT3904", SymbolID: "Device:Q_NPN_BEC", FootprintID: "Package_TO_SOT_SMD:SOT-23", Pins: bjtBECPins(), ComponentID: "bjt.onsemi.mmbt3904.sot23", ComponentVariant: "sot23", MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true, PlacementGroup: "output_pair"},
		{Role: "lower_output", RefPrefix: "Q", Value: "MMBT3906", SymbolID: "Device:Q_PNP_BEC", FootprintID: "Package_TO_SOT_SMD:SOT-23", Pins: pnpBECPins(), ComponentID: "bjt.onsemi.mmbt3906.sot23", ComponentVariant: "sot23", MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true, PlacementGroup: "output_pair"},
		{Role: "upper_emitter_resistor", RefPrefix: "R", Value: "0.47Ω", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_1206_3216Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, ComponentValueParam: "emitter_resistor_value", ComponentPackageParam: "emitter_resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "output_pair"},
		{Role: "lower_emitter_resistor", RefPrefix: "R", Value: "0.47Ω", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_1206_3216Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, ComponentValueParam: "emitter_resistor_value", ComponentPackageParam: "emitter_resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "output_pair"},
		{Role: "load_reference", RefPrefix: "TP", Value: "LOAD_REF", SymbolID: "Connector:TestPoint", FootprintID: "TestPoint:TestPoint_Pad_D1.0mm", Pins: []transactions.PinSpec{{Number: "1"}}, PlacementGroup: "load_reference"},
	}
}

func classABOutputPairPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationUnrealized,
		Components: []PCBComponentRealization{
			{ComponentRole: "upper_output", FootprintParam: "output_footprint", Placement: RelativePlacement{XMM: 0, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "lower_output", FootprintParam: "output_footprint", Placement: RelativePlacement{XMM: 0, YMM: 4, Layer: "F.Cu"}},
			{ComponentRole: "upper_emitter_resistor", FootprintParam: "emitter_resistor_footprint", Placement: RelativePlacement{XMM: 6, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "lower_emitter_resistor", FootprintParam: "emitter_resistor_footprint", Placement: RelativePlacement{XMM: 6, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "load_reference", FootprintID: "TestPoint:TestPoint_Pad_D1.0mm", Placement: RelativePlacement{XMM: 12, YMM: 6, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "bias_p", Port: "BIAS_P", NetTemplate: "bias_p", Placement: RelativePlacement{XMM: -5, YMM: -4, Layer: "F.Cu"}, Description: "Upper output-device bias entry."},
			{ID: "bias_n", Port: "BIAS_N", NetTemplate: "bias_n", Placement: RelativePlacement{XMM: -5, YMM: 4, Layer: "F.Cu"}, Description: "Lower output-device bias entry."},
			{ID: "amp_out", Port: "AMP_OUT", NetTemplate: "amp_out", Placement: RelativePlacement{XMM: 12, YMM: 0, Layer: "F.Cu"}, Description: "Shared emitter output node."},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "output_pair", ComponentRoles: []string{"upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor"}, AnchorRole: "upper_output", Bounds: &RelativeBounds{MinXMM: -4, MinYMM: -8, MaxXMM: 10, MaxYMM: 8}, Description: "Keep complementary devices and emitter resistors symmetric around the amplifier output node."}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "upper_bias", NetTemplate: "bias_p", From: RouteEndpoint{Port: "BIAS_P"}, To: RouteEndpoint{ComponentRole: "upper_output", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "lower_bias", NetTemplate: "bias_n", From: RouteEndpoint{Port: "BIAS_N"}, To: RouteEndpoint{ComponentRole: "lower_output", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "upper_emitter", NetTemplate: "upper_emitter", From: RouteEndpoint{ComponentRole: "upper_output", Pin: "2"}, To: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "lower_emitter", NetTemplate: "lower_emitter", From: RouteEndpoint{ComponentRole: "lower_output", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "amp_out_join", NetTemplate: "amp_out", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "amp_out_port", NetTemplate: "amp_out", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, To: RouteEndpoint{Port: "AMP_OUT"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "vcc_collector", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "upper_output", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "vee_collector", NetTemplate: "vee", From: RouteEndpoint{Port: "VEE"}, To: RouteEndpoint{ComponentRole: "lower_output", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "load_reference", NetTemplate: "load_ref", From: RouteEndpoint{Port: "LOAD_REF"}, To: RouteEndpoint{ComponentRole: "load_reference", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "output_pair_proximity", Kind: "max_spacing", AppliesTo: []string{"upper_output", "lower_output"}, MaxLengthMM: 4, Description: "Complementary output devices should be close for thermal symmetry."},
			{ID: "emitter_resistor_proximity", Kind: "max_spacing", AppliesTo: []string{"upper_emitter_resistor", "lower_emitter_resistor"}, MaxLengthMM: 4, Description: "Emitter resistors should stay near the output devices and output node."},
			{ID: "headphone_output_current_width", Kind: "route_width", NetTemplate: "amp_out", MinWidthMM: 0.5, Description: "Use wider copper for the headphone output current path."},
		},
		Validation:           PCBValidationExpectations{RequiredNets: []string{"bias_p", "bias_n", "upper_emitter", "lower_emitter", "amp_out", "vcc", "vee", "load_ref"}, RequiredRoutes: []string{"upper_bias", "lower_bias", "upper_emitter", "lower_emitter", "amp_out_join", "amp_out_port", "vcc_collector", "vee_collector", "load_reference"}},
		UnsupportedBehaviors: []string{"speaker-load and power-amplifier current paths require SOA, copper, and thermal evidence"},
	}
}

func instantiateClassABOutputPair(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if stringParam(params, "application") != "headphone" {
		issues = append(issues, blockIssue("params.application", "class_ab_output_pair is currently limited to headphone-class use"))
	}
	supplyVoltage, supplyOK := parseUnit(params["supply_voltage"], "V", voltageMultipliers())
	if !supplyOK || supplyVoltage <= 0 {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a positive voltage literal"))
	}
	loadOhms, loadOK := parseUnit(params["load_impedance"], "Ω", resistanceMultipliers())
	if !loadOK {
		issues = append(issues, blockIssue("params.load_impedance", "load_impedance must be a resistance literal"))
	} else if loadOhms < 16 {
		issues = append(issues, blockIssue("params.load_impedance", "Class AB output pair is currently limited to headphone loads of 16Ω or greater"))
	}
	if upperID := stringParam(params, "upper_output_component_id"); upperID != "bjt.onsemi.mmbt3904.sot23" {
		issues = append(issues, blockIssue("params.upper_output_component_id", "only bjt.onsemi.mmbt3904.sot23 is currently verified for the upper output device"))
	}
	if lowerID := stringParam(params, "lower_output_component_id"); lowerID != "bjt.onsemi.mmbt3906.sot23" {
		issues = append(issues, blockIssue("params.lower_output_component_id", "only bjt.onsemi.mmbt3906.sot23 is currently verified for the lower output device"))
	}
	emitterOhms, emitterOK := parseUnit(params["emitter_resistor_value"], "Ω", resistanceMultipliers())
	if !emitterOK || emitterOhms <= 0 {
		issues = append(issues, blockIssue("params.emitter_resistor_value", "emitter_resistor_value must be a positive resistance literal"))
	}
	if supplyOK && loadOK && emitterOK && loadOhms > 0 && emitterOhms > 0 {
		estimatedPeakCurrent := ((supplyVoltage / 2) - 0.7) / (loadOhms + emitterOhms)
		if estimatedPeakCurrent < 0 {
			estimatedPeakCurrent = 0
		}
		params["estimated_peak_current_a"] = estimatedPeakCurrent
		params["estimated_peak_emitter_resistor_dissipation_w"] = estimatedPeakCurrent * estimatedPeakCurrent * emitterOhms
		if estimatedPeakCurrent > 0.16 {
			issues = append(issues, blockIssue("params.load_impedance", "estimated output peak current exceeds the derated 160mA MMBT3904/MMBT3906 envelope"))
		}
	}
	emitterFootprint := stringParam(params, "emitter_resistor_footprint")
	outputFootprint := stringParam(params, "output_footprint")
	if emitterFootprint == "" {
		issues = append(issues, blockIssue("params.emitter_resistor_footprint", "emitter_resistor_footprint is required"))
	}
	if outputFootprint == "" {
		issues = append(issues, blockIssue("params.output_footprint", "output_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	componentsByRole := blockComponentByRole(definition.Components)
	refs := map[string]string{
		"upper_output":           allocator.Next("Q"),
		"lower_output":           allocator.Next("Q"),
		"upper_emitter_resistor": allocator.Next("R"),
		"lower_emitter_resistor": allocator.Next("R"),
		"load_reference":         allocator.Next("TP"),
	}
	points := classABOutputPairHintPoints(definition)
	roles := []string{"upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor", "load_reference"}
	var operations []transactions.Operation
	for _, role := range roles {
		component := componentsByRole[role]
		switch role {
		case "upper_output", "lower_output":
			component.FootprintID = outputFootprint
			if outputFootprint != "Package_TO_SOT_SMD:SOT-23" {
				component.ComponentID = ""
				component.ComponentVariant = ""
				component.PinmapRequired = false
			}
		case "upper_emitter_resistor", "lower_emitter_resistor":
			component.Value = formatOhms(emitterOhms)
			component.FootprintID = emitterFootprint
		}
		componentOps, componentIssues := ComponentOperations(component, refs[role], points[role])
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}
	nets := appendClassABOutputPairConnections(definition, request.InstanceID, refs, &operations, &issues)
	for _, label := range []struct {
		text string
		at   transactions.Point
	}{
		{text: "BIAS_P", at: transactions.Point{XMM: 36, YMM: 0}},
		{text: "BIAS_N", at: transactions.Point{XMM: 36, YMM: 5.08}},
		{text: "VCC", at: transactions.Point{XMM: 45, YMM: -16}},
		{text: "AMP_OUT", at: transactions.Point{XMM: 62, YMM: 2.54}},
		{text: "VEE", at: transactions.Point{XMM: 45, YMM: 24}},
		{text: "LOAD_REF", at: transactions.Point{XMM: 72, YMM: 12}},
	} {
		appendLabelOperation(&operations, &issues, label.text, label.at)
	}

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{refs["upper_output"], refs["lower_output"], refs["upper_emitter_resistor"], refs["lower_emitter_resistor"], refs["load_reference"]}
	output.Instance.Nets = nets
	return output
}

func classABOutputPairHintPoints(definition BlockDefinition) map[string]transactions.Point {
	points := make(map[string]transactions.Point, len(definition.SchematicHints))
	for _, hint := range definition.SchematicHints {
		if hint.ComponentRole != "" {
			points[hint.ComponentRole] = transactions.Point{XMM: hint.XMM, YMM: hint.YMM}
		}
	}
	return points
}

func appendClassABOutputPairConnections(definition BlockDefinition, instanceID string, refs map[string]string, operations *[]transactions.Operation, issues *[]reports.Issue) []string {
	portsByNet := map[string]string{"bias_p": "BIAS_P", "bias_n": "BIAS_N", "amp_out": "AMP_OUT", "vcc": "VCC", "vee": "VEE", "load_ref": "LOAD_REF"}
	nets := make([]string, 0, len(definition.Nets))
	for _, net := range definition.Nets {
		netName := InstanceNetName(instanceID, net.NameTemplate)
		nets = append(nets, netName)
		if len(net.Pins) == 0 {
			continue
		}
		previousRef := instanceID
		previousPin := portsByNet[net.NameTemplate]
		for index, pin := range net.Pins {
			ref := refs[pin.ComponentRole]
			if ref == "" {
				*issues = append(*issues, blockIssue("nets."+net.NameTemplate, "missing component ref for role "+pin.ComponentRole))
				continue
			}
			if index > 0 || previousPin != "" {
				appendConnectOperation(operations, issues, previousRef, previousPin, ref, pin.Pin, netName)
			}
			previousRef = ref
			previousPin = pin.Pin
		}
	}
	return nets
}
