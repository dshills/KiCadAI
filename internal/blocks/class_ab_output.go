package blocks

import (
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const classABOutputStageID = "class_ab_output_stage"

func classABOutputStageDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          classABOutputStageID,
		Name:        "Class AB Output Stage",
		Description: "Diode-biased complementary emitter follower output stage for headphone-class loads.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "topology", Type: ParameterEnum, Default: "diode_string", Allowed: []any{"diode_string"}, Description: "Bias topology. VBE multiplier support is intentionally blocked until verified."},
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "9V", Description: "Rail-to-rail supply voltage used for initial output-device screening."},
			{Name: "load_impedance", Type: ParameterResistance, Default: "32Ω", Description: "Nominal headphone load impedance."},
			{Name: "upper_output_component_id", Type: ParameterString, Default: "bjt.onsemi.mmbt3904.sot23", Description: "Verified NPN output-device component ID."},
			{Name: "lower_output_component_id", Type: ParameterString, Default: "bjt.onsemi.mmbt3906.sot23", Description: "Verified PNP output-device component ID."},
			{Name: "emitter_resistor_value", Type: ParameterResistance, Default: "0.47Ω", Description: "Small emitter resistor used to stabilize quiescent current."},
			{Name: "emitter_resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_1206_3216Metric", Description: "Footprint for output emitter resistors."},
			{Name: "bias_feed_resistor_value", Type: ParameterResistance, Default: "10kΩ", Description: "Resistor that provides bias-string current from the rails."},
			{Name: "bias_feed_resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Footprint for bias feed resistors."},
			{Name: "bias_diode_footprint", Type: ParameterFootprintID, Default: "Diode_SMD:D_SOD-123", Description: "Footprint for each bias diode."},
			{Name: "output_footprint", Type: ParameterFootprintID, Default: "Package_TO_SOT_SMD:SOT-23", Description: "Footprint for the complementary output BJTs."},
		},
		Ports: []BlockPort{
			{Name: "DRIVER_OUT", Direction: PortInput, Description: "Small-signal driver output feeding the bias string."},
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Positive output-device collector rail."},
			{Name: "VEE", Direction: PortPower, Description: "Negative rail or ground return for single-supply headphone stages."},
			{Name: "AMP_OUT", Direction: PortOutput, Description: "Emitter-follower output node."},
			{Name: "LOAD_REF", Direction: PortPassive, Description: "Load return or virtual ground reference."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:D", Required: true, Description: "Bias diode symbol."},
			{Kind: "symbol", ID: "Device:Q_NPN_BEC", Required: true, Description: "NPN output transistor symbol."},
			{Kind: "symbol", ID: "Device:Q_PNP_BEC", Required: true, Description: "PNP output transistor symbol."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Emitter degeneration resistor symbol."},
			{Kind: "symbol", ID: "Connector:TestPoint", Required: true, Description: "Load-reference anchor symbol."},
			{Kind: "footprint", ID: "Diode_SMD:D_SOD-123", Required: true, Description: "Default bias diode footprint."},
			{Kind: "footprint", ID: "Resistor_SMD:R_1206_3216Metric", Required: true, Description: "Default emitter resistor footprint."},
			{Kind: "footprint", ID: "Package_TO_SOT_SMD:SOT-23", Required: true, Description: "Default small-signal output BJT footprint."},
			{Kind: "footprint", ID: "TestPoint:TestPoint_Pad_D1.0mm", Required: true, Description: "Default load-reference test point footprint."},
		},
		Components:     classABOutputStageComponents(),
		PCBRealization: classABOutputStagePCBRealization(),
		Nets: []BlockNet{
			{NameTemplate: "upper_drive", Visibility: "local", Role: "upper_output_drive", Pins: []NetPin{{ComponentRole: "upper_bias_feed", Pin: "2"}, {ComponentRole: "bias_upper", Pin: "1"}, {ComponentRole: "upper_output", Pin: "1"}}},
			{NameTemplate: "driver", Visibility: "exported", Role: "driver_output", Pins: []NetPin{{ComponentRole: "bias_upper", Pin: "2"}, {ComponentRole: "bias_lower", Pin: "1"}}},
			{NameTemplate: "lower_drive", Visibility: "exported", Role: "lower_output_drive", Pins: []NetPin{{ComponentRole: "bias_lower", Pin: "2"}, {ComponentRole: "lower_output", Pin: "1"}, {ComponentRole: "lower_bias_feed", Pin: "1"}}},
			{NameTemplate: "upper_emitter", Visibility: "local", Role: "upper_emitter_resistor_input", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "2"}, {ComponentRole: "upper_emitter_resistor", Pin: "1"}}},
			{NameTemplate: "lower_emitter", Visibility: "local", Role: "lower_emitter_resistor_input", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "2"}, {ComponentRole: "lower_emitter_resistor", Pin: "1"}}},
			{NameTemplate: "amp_out", Visibility: "exported", Role: "amplifier_output", Pins: []NetPin{{ComponentRole: "upper_emitter_resistor", Pin: "2"}, {ComponentRole: "lower_emitter_resistor", Pin: "2"}}},
			{NameTemplate: "vcc", Visibility: "exported", Role: "positive_rail", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "3"}, {ComponentRole: "upper_bias_feed", Pin: "1"}}},
			{NameTemplate: "vee", Visibility: "exported", Role: "negative_rail", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "3"}, {ComponentRole: "lower_bias_feed", Pin: "2"}}},
			{NameTemplate: "load_ref", Visibility: "exported", Role: "load_reference", Pins: []NetPin{{ComponentRole: "load_reference", Pin: "1"}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "signal_flow", ComponentRole: "upper_bias_feed", XMM: 8, YMM: -7.62, Note: "Feed the upper bias node from the positive rail."},
			{Kind: "signal_flow", ComponentRole: "bias_upper", XMM: 20, YMM: -2.54, Note: "Keep driver and bias string left of the output pair."},
			{Kind: "signal_flow", ComponentRole: "bias_lower", XMM: 20, YMM: 7.62, Note: "Keep lower-bias path below the upper-bias path."},
			{Kind: "signal_flow", ComponentRole: "lower_bias_feed", XMM: 8, YMM: 12.7, Note: "Feed the lower bias node from the negative rail."},
			{Kind: "output_pair", ComponentRole: "upper_output", XMM: 45, YMM: 0, Note: "Place NPN device above the output node."},
			{Kind: "output_pair", ComponentRole: "lower_output", XMM: 45, YMM: 5.08, Note: "Place PNP device below the output node."},
			{Kind: "output_pair", ComponentRole: "upper_emitter_resistor", XMM: 56, YMM: 0, Note: "Keep the upper emitter resistor close to the output transistor emitter."},
			{Kind: "output_pair", ComponentRole: "lower_emitter_resistor", XMM: 56, YMM: 5.08, Note: "Keep the lower emitter resistor close to the output transistor emitter."},
			{Kind: "load_reference", ComponentRole: "load_reference", XMM: 65, YMM: 12, Note: "Keep load return separate from the negative rail unless the parent design intentionally ties them."},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "class_ab.topology.diode_string_only", Severity: BlockValidationSeverityBlocked, Description: "Only diode-string bias is supported until VBE multiplier evidence exists."},
			{ID: "class_ab.supply_voltage.positive", Severity: BlockValidationSeverityBlocked, Description: "Supply voltage must be a positive voltage literal."},
			{ID: "class_ab.load.headphone_class", Severity: BlockValidationSeverityBlocked, Description: "Initial realization is limited to headphone loads with peak current below verified output-device ratings."},
			{ID: "class_ab.output_pair.supported_ids", Severity: BlockValidationSeverityBlocked, Description: "Output-device IDs must match the verified complementary MMBT3904/MMBT3906 pair until selector-backed realization lands."},
			{ID: "class_ab.output_pair.evidence", Severity: BlockValidationSeverityBlocked, Description: "Complementary output devices must have verified polarity, role, pinmap, and SOA evidence."},
			{ID: "class_ab.emitter_resistors.required", Severity: BlockValidationSeverityBlocked, Description: "Emitter resistors must be present and positive for the diode-biased Class AB stage."},
			{ID: "class_ab.thermal.review", Severity: BlockValidationSeverityBlocked, Description: "Power amplifier and speaker-load variants require thermal and PCB-current evidence before fabrication."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"component:bjt.onsemi.mmbt3904.sot23",
				"component:bjt.onsemi.mmbt3906.sot23",
				"amplifier.output_pair:headphone_class",
			},
			Notes: []string{
				"Structural realization emits a diode-biased complementary pair for reviewable headphone-class schematics.",
				"Thermal design, quiescent-current trim, stability compensation, and high-current PCB evidence are still blocked for fabrication claims.",
			},
		},
	}
}

func classABOutputStagePCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationUnrealized,
		Components: []PCBComponentRealization{
			{ComponentRole: "bias_upper", FootprintParam: "bias_diode_footprint", Placement: RelativePlacement{XMM: 6, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "bias_lower", FootprintParam: "bias_diode_footprint", Placement: RelativePlacement{XMM: 6, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "upper_output", FootprintParam: "output_footprint", Placement: RelativePlacement{XMM: 8, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "lower_output", FootprintParam: "output_footprint", Placement: RelativePlacement{XMM: 8, YMM: 4, Layer: "F.Cu"}},
			{ComponentRole: "upper_emitter_resistor", FootprintParam: "emitter_resistor_footprint", Placement: RelativePlacement{XMM: 13, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "lower_emitter_resistor", FootprintParam: "emitter_resistor_footprint", Placement: RelativePlacement{XMM: 13, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "upper_bias_feed", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: -7, Layer: "F.Cu"}},
			{ComponentRole: "lower_bias_feed", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: 7, Layer: "F.Cu"}},
			{ComponentRole: "load_reference", FootprintID: "TestPoint:TestPoint_Pad_D1.0mm", Placement: RelativePlacement{XMM: 16, YMM: 7, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{
			{ID: "bias_string", ComponentRoles: []string{"upper_bias_feed", "bias_upper", "bias_lower", "lower_bias_feed"}, AnchorRole: "bias_upper", Bounds: &RelativeBounds{MinXMM: 0, MinYMM: -9, MaxXMM: 9, MaxYMM: 9}, Description: "Keep the diode bias string thermally adjacent to the output pair."},
			{ID: "output_pair", ComponentRoles: []string{"upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor"}, AnchorRole: "upper_output", Bounds: &RelativeBounds{MinXMM: 4, MinYMM: -8, MaxXMM: 16, MaxYMM: 8}, Description: "Keep complementary output devices and emitter resistors symmetric around the amplifier output node."},
		},
		Constraints: []PCBConstraint{
			{ID: "upper_bias_thermal_coupling", Kind: "max_spacing", AppliesTo: []string{"bias_upper", "upper_output"}, MaxLengthMM: 3, Description: "Upper bias diode must stay thermally close to the NPN output device."},
			{ID: "lower_bias_thermal_coupling", Kind: "max_spacing", AppliesTo: []string{"bias_lower", "lower_output"}, MaxLengthMM: 3, Description: "Lower bias diode must stay thermally close to the PNP output device."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"upper_drive", "driver", "lower_drive", "upper_emitter", "lower_emitter", "amp_out", "vcc", "vee", "load_ref"}},
		UnsupportedBehaviors: []string{
			"thermal placement and copper-area sizing are not verified",
			"quiescent-current trimming and VBE multiplier placement are not implemented",
			"speaker-load and power-amplifier routing constraints are intentionally blocked",
		},
	}
}

func classABOutputStageComponents() []BlockComponent {
	return []BlockComponent{
		{
			Role:              "bias_upper",
			RefPrefix:         "D",
			Value:             "1N4148",
			SymbolID:          "Device:D",
			FootprintID:       "Diode_SMD:D_SOD-123",
			Pins:              twoTerminalHorizontalPins(),
			ComponentID:       "diode.onsemi.1n4148w.sod_123",
			ComponentVariant:  "sod_123",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
			PinmapRequired:    true,
			PlacementGroup:    "bias_string",
		},
		{
			Role:              "bias_lower",
			RefPrefix:         "D",
			Value:             "1N4148",
			SymbolID:          "Device:D",
			FootprintID:       "Diode_SMD:D_SOD-123",
			Pins:              twoTerminalHorizontalPins(),
			ComponentID:       "diode.onsemi.1n4148w.sod_123",
			ComponentVariant:  "sod_123",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
			PinmapRequired:    true,
			PlacementGroup:    "bias_string",
		},
		{
			Role:           "upper_bias_feed",
			RefPrefix:      "R",
			Value:          "10kΩ",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_0805_2012Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "bias_string",
		},
		{
			Role:           "lower_bias_feed",
			RefPrefix:      "R",
			Value:          "10kΩ",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_0805_2012Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "bias_string",
		},
		{
			Role:           "upper_emitter_resistor",
			RefPrefix:      "R",
			Value:          "0.47Ω",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_1206_3216Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "output_pair",
		},
		{
			Role:           "lower_emitter_resistor",
			RefPrefix:      "R",
			Value:          "0.47Ω",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_1206_3216Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "output_pair",
		},
		{
			Role:              "upper_output",
			RefPrefix:         "Q",
			Value:             "MMBT3904",
			SymbolID:          "Device:Q_NPN_BEC",
			FootprintID:       "Package_TO_SOT_SMD:SOT-23",
			Pins:              bjtBECPins(),
			ComponentID:       "bjt.onsemi.mmbt3904.sot23",
			ComponentVariant:  "sot23",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
			PinmapRequired:    true,
			PlacementGroup:    "output_pair",
		},
		{
			Role:              "lower_output",
			RefPrefix:         "Q",
			Value:             "MMBT3906",
			SymbolID:          "Device:Q_PNP_BEC",
			FootprintID:       "Package_TO_SOT_SMD:SOT-23",
			Pins:              pnpBECPins(),
			ComponentID:       "bjt.onsemi.mmbt3906.sot23",
			ComponentVariant:  "sot23",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
			PinmapRequired:    true,
			PlacementGroup:    "output_pair",
		},
		{
			Role:           "load_reference",
			RefPrefix:      "TP",
			Value:          "LOAD_REF",
			SymbolID:       "Connector:TestPoint",
			FootprintID:    "TestPoint:TestPoint_Pad_D1.0mm",
			Pins:           []transactions.PinSpec{{Number: "1"}},
			PlacementGroup: "load_reference",
		},
	}
}

func instantiateClassABOutputStage(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if topology := strings.TrimSpace(stringParam(params, "topology")); topology != "diode_string" {
		issues = append(issues, blockIssue("params.topology", "only diode_string Class AB bias is currently supported"))
	}
	supplyVoltage, supplyOK := parseUnit(params["supply_voltage"], "V", voltageMultipliers())
	if !supplyOK {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a voltage literal"))
	} else if supplyVoltage <= 0 {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be positive"))
	}
	loadOhms, loadOK := parseUnit(params["load_impedance"], "Ω", resistanceMultipliers())
	if !loadOK {
		issues = append(issues, blockIssue("params.load_impedance", "load_impedance must be a resistance literal"))
	} else if loadOhms < 16 {
		issues = append(issues, blockIssue("params.load_impedance", "Class AB output stage is currently limited to headphone loads of 16Ω or greater"))
	}
	if upperID := stringParam(params, "upper_output_component_id"); upperID != "bjt.onsemi.mmbt3904.sot23" {
		issues = append(issues, blockIssue("params.upper_output_component_id", "only bjt.onsemi.mmbt3904.sot23 is currently verified for the upper output device"))
	}
	if lowerID := stringParam(params, "lower_output_component_id"); lowerID != "bjt.onsemi.mmbt3906.sot23" {
		issues = append(issues, blockIssue("params.lower_output_component_id", "only bjt.onsemi.mmbt3906.sot23 is currently verified for the lower output device"))
	}
	emitterOhms, emitterOK := parseUnit(params["emitter_resistor_value"], "Ω", resistanceMultipliers())
	if !emitterOK {
		issues = append(issues, blockIssue("params.emitter_resistor_value", "emitter_resistor_value must be a resistance literal"))
	} else if emitterOhms <= 0 {
		issues = append(issues, blockIssue("params.emitter_resistor_value", "emitter_resistor_value must be positive"))
	}
	if supplyOK && loadOK && emitterOK && loadOhms > 0 && emitterOhms > 0 {
		estimatedPeakCurrent := ((supplyVoltage / 2) - 0.7) / (loadOhms + emitterOhms)
		if estimatedPeakCurrent < 0 {
			estimatedPeakCurrent = 0
		}
		if estimatedPeakCurrent > 0.16 {
			issues = append(issues, blockIssue("params.load_impedance", "estimated output peak current exceeds the derated 160mA MMBT3904/MMBT3906 envelope"))
		}
	}
	biasFeedOhms, biasFeedOK := parseUnit(params["bias_feed_resistor_value"], "Ω", resistanceMultipliers())
	if !biasFeedOK {
		issues = append(issues, blockIssue("params.bias_feed_resistor_value", "bias_feed_resistor_value must be a resistance literal"))
	} else if biasFeedOhms <= 0 {
		issues = append(issues, blockIssue("params.bias_feed_resistor_value", "bias_feed_resistor_value must be positive"))
	}
	biasFootprint := stringParam(params, "bias_diode_footprint")
	emitterFootprint := stringParam(params, "emitter_resistor_footprint")
	biasFeedFootprint := stringParam(params, "bias_feed_resistor_footprint")
	outputFootprint := stringParam(params, "output_footprint")
	if biasFootprint == "" {
		issues = append(issues, blockIssue("params.bias_diode_footprint", "bias_diode_footprint is required"))
	}
	if emitterFootprint == "" {
		issues = append(issues, blockIssue("params.emitter_resistor_footprint", "emitter_resistor_footprint is required"))
	}
	if biasFeedFootprint == "" {
		issues = append(issues, blockIssue("params.bias_feed_resistor_footprint", "bias_feed_resistor_footprint is required"))
	}
	if outputFootprint == "" {
		issues = append(issues, blockIssue("params.output_footprint", "output_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	componentsByRole := blockComponentByRole(classABOutputStageComponents())
	refs := map[string]string{
		"bias_upper":             allocator.Next("D"),
		"bias_lower":             allocator.Next("D"),
		"upper_bias_feed":        allocator.Next("R"),
		"lower_bias_feed":        allocator.Next("R"),
		"upper_emitter_resistor": allocator.Next("R"),
		"lower_emitter_resistor": allocator.Next("R"),
		"upper_output":           allocator.Next("Q"),
		"lower_output":           allocator.Next("Q"),
		"load_reference":         allocator.Next("TP"),
	}
	points := map[string]transactions.Point{
		"bias_upper":             {XMM: 20, YMM: -2.54},
		"bias_lower":             {XMM: 20, YMM: 7.62},
		"upper_bias_feed":        {XMM: 8, YMM: -7.62},
		"lower_bias_feed":        {XMM: 8, YMM: 12.7},
		"upper_emitter_resistor": {XMM: 56, YMM: 0},
		"lower_emitter_resistor": {XMM: 56, YMM: 10.16},
		"upper_output":           {XMM: 45, YMM: 0},
		"lower_output":           {XMM: 45, YMM: 10.16},
		"load_reference":         {XMM: 65, YMM: 12},
	}
	var operations []transactions.Operation
	for _, role := range classABOutputStageRoleOrder() {
		component := componentsByRole[role]
		switch role {
		case "bias_upper", "bias_lower":
			component.FootprintID = biasFootprint
		case "upper_emitter_resistor", "lower_emitter_resistor":
			component.Value = formatOhms(emitterOhms)
			component.FootprintID = emitterFootprint
		case "upper_bias_feed", "lower_bias_feed":
			component.Value = formatOhms(biasFeedOhms)
			component.FootprintID = biasFeedFootprint
		case "upper_output", "lower_output":
			component.FootprintID = outputFootprint
		case "load_reference":
		default:
			issues = append(issues, blockIssue("component."+role, "unsupported Class AB component role"))
			continue
		}
		componentOps, componentIssues := ComponentOperations(component, refs[role], points[role])
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}

	upperDriveNet := InstanceNetName(request.InstanceID, "upper_drive")
	driverNet := InstanceNetName(request.InstanceID, "driver")
	lowerDriveNet := InstanceNetName(request.InstanceID, "lower_drive")
	upperEmitterNet := InstanceNetName(request.InstanceID, "upper_emitter")
	lowerEmitterNet := InstanceNetName(request.InstanceID, "lower_emitter")
	ampOutNet := InstanceNetName(request.InstanceID, "amp_out")
	vccNet := InstanceNetName(request.InstanceID, "vcc")
	veeNet := InstanceNetName(request.InstanceID, "vee")
	loadRefNet := InstanceNetName(request.InstanceID, "load_ref")
	appendConnectOperation(&operations, &issues, refs["bias_upper"], "1", refs["upper_output"], "1", upperDriveNet)
	appendConnectOperation(&operations, &issues, refs["upper_bias_feed"], "2", refs["bias_upper"], "1", upperDriveNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "DRIVER_OUT", refs["bias_upper"], "2", driverNet)
	appendConnectOperation(&operations, &issues, refs["bias_upper"], "2", refs["bias_lower"], "1", driverNet)
	appendConnectOperation(&operations, &issues, refs["bias_lower"], "2", refs["lower_output"], "1", lowerDriveNet)
	appendConnectOperation(&operations, &issues, refs["bias_lower"], "2", refs["lower_bias_feed"], "1", lowerDriveNet)
	appendConnectOperation(&operations, &issues, refs["upper_output"], "2", refs["upper_emitter_resistor"], "1", upperEmitterNet)
	appendConnectOperation(&operations, &issues, refs["lower_output"], "2", refs["lower_emitter_resistor"], "1", lowerEmitterNet)
	appendConnectOperation(&operations, &issues, refs["upper_emitter_resistor"], "2", refs["lower_emitter_resistor"], "2", ampOutNet)
	appendConnectOperation(&operations, &issues, refs["upper_emitter_resistor"], "2", request.InstanceID, "AMP_OUT", ampOutNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", refs["upper_output"], "3", vccNet)
	appendConnectOperation(&operations, &issues, refs["upper_bias_feed"], "1", refs["upper_output"], "3", vccNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "VEE", refs["lower_output"], "3", veeNet)
	appendConnectOperation(&operations, &issues, refs["lower_bias_feed"], "2", refs["lower_output"], "3", veeNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "LOAD_REF", refs["load_reference"], "1", loadRefNet)

	for _, label := range []struct {
		text string
		at   transactions.Point
	}{
		{text: "DRIVER_OUT", at: transactions.Point{XMM: 8, YMM: 0}},
		{text: "BIAS", at: transactions.Point{XMM: 24, YMM: 2.54}},
		{text: "VCC", at: transactions.Point{XMM: 45, YMM: -20}},
		{text: "AMP_OUT", at: transactions.Point{XMM: 62, YMM: 2.54}},
		{text: "VEE", at: transactions.Point{XMM: 45, YMM: 30}},
		{text: "LOAD_REF", at: transactions.Point{XMM: 72, YMM: 12}},
	} {
		appendLabelOperation(&operations, &issues, label.text, label.at)
	}

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{refs["bias_upper"], refs["bias_lower"], refs["upper_bias_feed"], refs["lower_bias_feed"], refs["upper_emitter_resistor"], refs["lower_emitter_resistor"], refs["upper_output"], refs["lower_output"], refs["load_reference"]}
	output.Instance.Nets = []string{upperDriveNet, driverNet, lowerDriveNet, upperEmitterNet, lowerEmitterNet, ampOutNet, vccNet, veeNet, loadRefNet}
	return output
}

func bjtBECPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: 0},
		{Number: "2", XMM: 2.54, YMM: 2.54},
		{Number: "3", XMM: 2.54, YMM: -2.54},
	}
}

func pnpBECPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: 0},
		{Number: "2", XMM: 2.54, YMM: -2.54},
		{Number: "3", XMM: 2.54, YMM: 2.54},
	}
}

func classABOutputStageRoleOrder() []string {
	return []string{"upper_bias_feed", "bias_upper", "bias_lower", "lower_bias_feed", "upper_emitter_resistor", "lower_emitter_resistor", "upper_output", "lower_output", "load_reference"}
}

func appendLabelOperation(operations *[]transactions.Operation, issues *[]reports.Issue, text string, at transactions.Point) {
	operation, err := wrapOperation(transactions.OpAddLabel, transactions.AddLabelOperation{
		Op:   transactions.OpAddLabel,
		Text: text,
		At:   at,
		Kind: "local",
	})
	if err != nil {
		*issues = append(*issues, blockIssue("label."+text, err.Error()))
		return
	}
	*operations = append(*operations, operation)
}
