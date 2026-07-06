package blocks

import (
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const amplifierBiasNetworkID = "amplifier_bias_network"

func amplifierBiasNetworkDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          amplifierBiasNetworkID,
		Name:        "Amplifier Bias Network",
		Description: "Diode-string Class AB bias network for headphone-class complementary output stages.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "topology", Type: ParameterEnum, Default: "diode_string", Allowed: []any{"diode_string", "vbe_multiplier"}, Description: "Bias topology. VBE multiplier support is intentionally blocked until verified."},
			{Name: "application", Type: ParameterEnum, Default: "headphone", Allowed: []any{"headphone", "speaker", "power_amplifier"}, Description: "Load class for evidence gating."},
			{Name: "diode_count", Type: ParameterNumber, Default: 2.0, Description: "Number of series silicon bias diodes. Only two-diode Class AB bias is currently implemented."},
			{Name: "emitter_resistor_value", Type: ParameterResistance, Default: "0.47Ω", Description: "Expected output-pair emitter resistor value used for downstream quiescent-current review."},
			{Name: "bias_feed_resistor_value", Type: ParameterResistance, Default: "10kΩ", Description: "Resistor that provides bias-string current from the rails."},
			{Name: "target_quiescent_current", Type: ParameterString, Default: "review_required", Description: "Requested idle-current evidence. Numeric verified targets are blocked until simulation/evidence exists."},
			{Name: "thermal_coupling_policy", Type: ParameterEnum, Default: "adjacent_to_output_pair", Allowed: []any{"adjacent_to_output_pair", "unconstrained"}, Description: "Placement policy for thermally coupling bias diodes to output devices."},
			{Name: "bias_diode_footprint", Type: ParameterFootprintID, Default: "Diode_SMD:D_SOD-123", Description: "Footprint for each bias diode."},
			{Name: "bias_feed_resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Footprint for bias feed resistors."},
		},
		Ports: []BlockPort{
			{Name: "DRIVER_OUT", Direction: PortInput, Description: "Small-signal driver output feeding the midpoint of the bias string."},
			{Name: "BIAS_P", Direction: PortOutput, Description: "Upper output-device base/gate bias node."},
			{Name: "BIAS_N", Direction: PortOutput, Description: "Lower output-device base/gate bias node."},
			{Name: "AMP_OUT", Direction: PortPassive, Description: "Amplifier output placement anchor for downstream output-pair alignment."},
			{Name: "VCC", Direction: PortPower, Description: "Positive rail feeding the upper bias resistor."},
			{Name: "VEE", Direction: PortPower, Description: "Negative rail or ground return feeding the lower bias resistor."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:D", Required: true, Description: "Bias diode symbol."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Bias feed resistor symbol."},
			{Kind: "symbol", ID: "Connector:TestPoint", Required: true, Description: "Amplifier output alignment anchor."},
			{Kind: "footprint", ID: "Diode_SMD:D_SOD-123", Required: true, Description: "Default bias diode footprint."},
			{Kind: "footprint", ID: "Resistor_SMD:R_0805_2012Metric", Required: true, Description: "Default bias feed resistor footprint."},
			{Kind: "footprint", ID: "TestPoint:TestPoint_Pad_D1.0mm", Required: true, Description: "Default amplifier output anchor footprint."},
		},
		Components:     amplifierBiasNetworkComponents(),
		PCBRealization: amplifierBiasNetworkPCBRealization(),
		Nets: []BlockNet{
			{NameTemplate: "bias_p", Visibility: "exported", Role: "upper_output_bias", Pins: []NetPin{{ComponentRole: "upper_bias_feed", Pin: "2"}, {ComponentRole: "bias_upper", Pin: "2"}}},
			{NameTemplate: "driver_out", Visibility: "exported", Role: "driver_output", Pins: []NetPin{{ComponentRole: "bias_upper", Pin: "1"}, {ComponentRole: "bias_lower", Pin: "2"}}},
			{NameTemplate: "bias_n", Visibility: "exported", Role: "lower_output_bias", Pins: []NetPin{{ComponentRole: "bias_lower", Pin: "1"}, {ComponentRole: "lower_bias_feed", Pin: "1"}}},
			{NameTemplate: "vcc", Visibility: "exported", Role: "positive_rail", Pins: []NetPin{{ComponentRole: "upper_bias_feed", Pin: "1"}}},
			{NameTemplate: "vee", Visibility: "exported", Role: "negative_rail", Pins: []NetPin{{ComponentRole: "lower_bias_feed", Pin: "2"}}},
			{NameTemplate: "amp_out", Visibility: "exported", Role: "amplifier_output_anchor", Pins: []NetPin{{ComponentRole: "amp_out_anchor", Pin: "1"}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "signal_flow", ComponentRole: "upper_bias_feed", XMM: 8, YMM: -7.62, Note: "Feed the upper bias node from the positive rail."},
			{Kind: "signal_flow", ComponentRole: "bias_upper", XMM: 20, YMM: -2.54, Note: "Keep the upper diode above the driver midpoint."},
			{Kind: "signal_flow", ComponentRole: "bias_lower", XMM: 20, YMM: 7.62, Note: "Keep the lower diode below the driver midpoint."},
			{Kind: "signal_flow", ComponentRole: "lower_bias_feed", XMM: 8, YMM: 12.7, Note: "Feed the lower bias node from the negative rail."},
			{Kind: "output_anchor", ComponentRole: "amp_out_anchor", XMM: 42, YMM: 2.54, Note: "Align this anchor with the output-pair emitter/output node."},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "amplifier_bias.topology.diode_string_only", Severity: BlockValidationSeverityBlocked, Description: "Only diode-string bias is supported until VBE multiplier evidence exists."},
			{ID: "amplifier_bias.application.headphone_only", Severity: BlockValidationSeverityBlocked, Description: "Speaker and power-amplifier bias networks are blocked until thermal and current evidence exists."},
			{ID: "amplifier_bias.quiescent_current.review_required", Severity: BlockValidationSeverityBlocked, Description: "Verified quiescent-current targets are blocked until simulation-backed evidence exists."},
			{ID: "amplifier_bias.thermal.adjacent", Severity: BlockValidationSeverityBlocked, Description: "Bias diodes must be placed adjacent to the output pair for thermal tracking."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"component:diode.onsemi.1n4148w.sod_123",
				"amplifier.bias_network:diode_string_headphone",
			},
			Notes: []string{
				"Structural realization emits a two-diode Class AB bias string with rail feed resistors.",
				"Quiescent-current trimming, VBE multiplier support, and power-amplifier thermal proof remain blocked.",
			},
		},
	}
}

func amplifierBiasNetworkComponents() []BlockComponent {
	return []BlockComponent{
		{
			Role:                  "upper_bias_feed",
			RefPrefix:             "R",
			Value:                 "10kΩ",
			SymbolID:              "Device:R",
			FootprintID:           "Resistor_SMD:R_0805_2012Metric",
			Pins:                  twoTerminalHorizontalPins(),
			ComponentQuery:        &components.Query{Family: "resistor", ValueKind: "resistance"},
			ComponentValueParam:   "bias_feed_resistor_value",
			ComponentPackageParam: "bias_feed_resistor_footprint",
			MinimumConfidence:     components.ConfidenceRuleInferred,
			Acceptance:            components.AcceptanceConnectivity,
			PlacementGroup:        "bias_string",
		},
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
			Role:                  "lower_bias_feed",
			RefPrefix:             "R",
			Value:                 "10kΩ",
			SymbolID:              "Device:R",
			FootprintID:           "Resistor_SMD:R_0805_2012Metric",
			Pins:                  twoTerminalHorizontalPins(),
			ComponentQuery:        &components.Query{Family: "resistor", ValueKind: "resistance"},
			ComponentValueParam:   "bias_feed_resistor_value",
			ComponentPackageParam: "bias_feed_resistor_footprint",
			MinimumConfidence:     components.ConfidenceRuleInferred,
			Acceptance:            components.AcceptanceConnectivity,
			PlacementGroup:        "bias_string",
		},
		{
			Role:           "amp_out_anchor",
			RefPrefix:      "TP",
			Value:          "AMP_OUT",
			SymbolID:       "Connector:TestPoint",
			FootprintID:    "TestPoint:TestPoint_Pad_D1.0mm",
			Pins:           []transactions.PinSpec{{Number: "1"}},
			PlacementGroup: "output_anchor",
		},
	}
}

func amplifierBiasNetworkPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationUnrealized,
		Components: []PCBComponentRealization{
			{ComponentRole: "upper_bias_feed", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: -7, Layer: "F.Cu"}},
			{ComponentRole: "bias_upper", FootprintParam: "bias_diode_footprint", Placement: RelativePlacement{XMM: 6, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "bias_lower", FootprintParam: "bias_diode_footprint", Placement: RelativePlacement{XMM: 6, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "lower_bias_feed", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: 7, Layer: "F.Cu"}},
			{ComponentRole: "amp_out_anchor", FootprintID: "TestPoint:TestPoint_Pad_D1.0mm", Placement: RelativePlacement{XMM: 14, YMM: 0, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "driver", Port: "DRIVER_OUT", NetTemplate: "driver_out", Placement: RelativePlacement{XMM: -2, YMM: 0, Layer: "F.Cu"}, Description: "Driver midpoint entering the diode string."},
			{ID: "bias_p", Port: "BIAS_P", NetTemplate: "bias_p", Placement: RelativePlacement{XMM: 10, YMM: -3, Layer: "F.Cu"}, Description: "Upper output device bias node."},
			{ID: "bias_n", Port: "BIAS_N", NetTemplate: "bias_n", Placement: RelativePlacement{XMM: 10, YMM: 3, Layer: "F.Cu"}, Description: "Lower output device bias node."},
			{ID: "amp_out", Port: "AMP_OUT", NetTemplate: "amp_out", Placement: RelativePlacement{XMM: 14, YMM: 0, Layer: "F.Cu"}, Description: "Output-pair placement anchor."},
		},
		PlacementGroups: []PCBPlacementGroup{
			{ID: "bias_string", ComponentRoles: []string{"upper_bias_feed", "bias_upper", "bias_lower", "lower_bias_feed"}, AnchorRole: "bias_upper", Bounds: &RelativeBounds{MinXMM: 0, MinYMM: -9, MaxXMM: 9, MaxYMM: 9}, Description: "Keep the bias string thermally adjacent to the future output pair."},
		},
		LocalRoutes: []PCBLocalRoute{
			{ID: "vcc_to_upper_feed", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "upper_bias_feed", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "upper_feed_to_bias_p", NetTemplate: "bias_p", From: RouteEndpoint{ComponentRole: "upper_bias_feed", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_upper", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "driver_midpoint", NetTemplate: "driver_out", From: RouteEndpoint{ComponentRole: "bias_upper", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bias_lower", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "driver_port", NetTemplate: "driver_out", From: RouteEndpoint{Port: "DRIVER_OUT"}, To: RouteEndpoint{ComponentRole: "bias_upper", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "bias_n_to_lower_feed", NetTemplate: "bias_n", From: RouteEndpoint{ComponentRole: "bias_lower", Pin: "1"}, To: RouteEndpoint{ComponentRole: "lower_bias_feed", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "lower_feed_to_vee", NetTemplate: "vee", From: RouteEndpoint{ComponentRole: "lower_bias_feed", Pin: "2"}, To: RouteEndpoint{Port: "VEE"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "amp_out_anchor", NetTemplate: "amp_out", From: RouteEndpoint{Port: "AMP_OUT"}, To: RouteEndpoint{ComponentRole: "amp_out_anchor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "bias_string_thermal_coupling", Kind: "max_spacing", AppliesTo: []string{"bias_upper", "bias_lower", "amp_out_anchor"}, MaxLengthMM: 5, Description: "Bias diodes must stay near the output-pair thermal region represented by AMP_OUT."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"bias_p", "driver_out", "bias_n", "vcc", "vee", "amp_out"}, RequiredRoutes: []string{"vcc_to_upper_feed", "upper_feed_to_bias_p", "driver_midpoint", "driver_port", "bias_n_to_lower_feed", "lower_feed_to_vee", "amp_out_anchor"}},
		UnsupportedBehaviors: []string{
			"VBE multiplier bias is not implemented",
			"numeric quiescent-current targets are not verified",
			"speaker-load and power-amplifier thermal evidence is intentionally blocked",
		},
	}
}

func instantiateAmplifierBiasNetwork(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if topology := strings.TrimSpace(stringParam(params, "topology")); topology != "diode_string" {
		issues = append(issues, blockIssue("params.topology", "only diode_string bias is currently supported"))
	}
	if application := strings.TrimSpace(stringParam(params, "application")); application != "headphone" {
		issues = append(issues, blockIssue("params.application", "amplifier_bias_network is currently limited to headphone-class use"))
	}
	diodeCount, diodeOK := numericValue(params["diode_count"])
	if !diodeOK {
		issues = append(issues, blockIssue("params.diode_count", "diode_count must be numeric"))
	} else if diodeCount != 2 {
		issues = append(issues, blockIssue("params.diode_count", "only a two-diode Class AB bias string is currently supported"))
	}
	emitterOhms, emitterOK := parseUnit(params["emitter_resistor_value"], "Ω", resistanceMultipliers())
	if !emitterOK {
		issues = append(issues, blockIssue("params.emitter_resistor_value", "emitter_resistor_value must be a resistance literal"))
	} else if emitterOhms <= 0 {
		issues = append(issues, blockIssue("params.emitter_resistor_value", "emitter_resistor_value must be positive"))
	}
	biasFeedOhms, biasFeedOK := parseUnit(params["bias_feed_resistor_value"], "Ω", resistanceMultipliers())
	if !biasFeedOK {
		issues = append(issues, blockIssue("params.bias_feed_resistor_value", "bias_feed_resistor_value must be a resistance literal"))
	} else if biasFeedOhms <= 0 {
		issues = append(issues, blockIssue("params.bias_feed_resistor_value", "bias_feed_resistor_value must be positive"))
	}
	targetIQ := strings.TrimSpace(stringParam(params, "target_quiescent_current"))
	if targetIQ != "" && targetIQ != "review_required" {
		issues = append(issues, blockIssue("params.target_quiescent_current", "verified quiescent-current targets are blocked until simulation-backed evidence exists"))
	}
	if policy := strings.TrimSpace(stringParam(params, "thermal_coupling_policy")); policy != "adjacent_to_output_pair" {
		issues = append(issues, blockIssue("params.thermal_coupling_policy", "bias diodes must remain adjacent to the output pair until thermal alternatives are verified"))
	}
	biasFootprint := stringParam(params, "bias_diode_footprint")
	biasFeedFootprint := stringParam(params, "bias_feed_resistor_footprint")
	if biasFootprint == "" {
		issues = append(issues, blockIssue("params.bias_diode_footprint", "bias_diode_footprint is required"))
	}
	if biasFeedFootprint == "" {
		issues = append(issues, blockIssue("params.bias_feed_resistor_footprint", "bias_feed_resistor_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	componentsByRole := blockComponentByRole(definition.Components)
	refs := map[string]string{
		"upper_bias_feed": allocator.Next("R"),
		"bias_upper":      allocator.Next("D"),
		"bias_lower":      allocator.Next("D"),
		"lower_bias_feed": allocator.Next("R"),
		"amp_out_anchor":  allocator.Next("TP"),
	}
	points := amplifierBiasNetworkHintPoints(definition)
	roles := []string{"upper_bias_feed", "bias_upper", "bias_lower", "lower_bias_feed", "amp_out_anchor"}
	var operations []transactions.Operation
	for _, role := range roles {
		component, ok := componentsByRole[role]
		if !ok {
			issues = append(issues, blockIssue("components."+role, "amplifier_bias_network missing required component definition"))
			continue
		}
		point, ok := points[role]
		if !ok {
			issues = append(issues, blockIssue("schematic_hints."+role, "amplifier_bias_network missing required schematic hint"))
			continue
		}
		switch role {
		case "upper_bias_feed", "lower_bias_feed":
			component.Value = formatOhms(biasFeedOhms)
			component.FootprintID = biasFeedFootprint
		case "bias_upper", "bias_lower":
			component.FootprintID = biasFootprint
			if biasFootprint != "Diode_SMD:D_SOD-123" {
				component.ComponentID = ""
				component.ComponentVariant = ""
				component.PinmapRequired = false
			}
		}
		componentOps, componentIssues := ComponentOperations(component, refs[role], point)
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}

	nets := appendAmplifierBiasNetworkConnections(definition, request.InstanceID, refs, &operations, &issues)
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{refs["upper_bias_feed"], refs["bias_upper"], refs["bias_lower"], refs["lower_bias_feed"], refs["amp_out_anchor"]}
	output.Instance.Nets = nets
	return output
}

func amplifierBiasNetworkHintPoints(definition BlockDefinition) map[string]transactions.Point {
	points := make(map[string]transactions.Point, len(definition.SchematicHints))
	for _, hint := range definition.SchematicHints {
		if hint.ComponentRole == "" {
			continue
		}
		points[hint.ComponentRole] = transactions.Point{XMM: hint.XMM, YMM: hint.YMM}
	}
	return points
}

func appendAmplifierBiasNetworkConnections(definition BlockDefinition, instanceID string, refs map[string]string, operations *[]transactions.Operation, issues *[]reports.Issue) []string {
	portsByNet := map[string]string{
		"bias_p":  "BIAS_P",
		"driver":  "DRIVER_OUT",
		"bias_n":  "BIAS_N",
		"vcc":     "VCC",
		"vee":     "VEE",
		"amp_out": "AMP_OUT",
	}
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
