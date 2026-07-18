package blocks

import (
	"math"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const speakerOpAmpDriverID = "speaker_opamp_driver"

func newSpeakerOpAmpDriverDefinition() BlockDefinition {
	verified := components.ConfidenceVerified
	return BlockDefinition{
		ID:          speakerOpAmpDriverID,
		Name:        "Speaker Op-Amp Driver",
		Description: "Dual-rail audio op-amp driver with load-side Kelvin feedback, local rail bypassing, and output isolation.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "gain", Type: ParameterNumber, Default: 11.0, Min: speakerFloatPointer(2), Max: speakerFloatPointer(30), Description: "Closed-loop non-inverting voltage gain."},
			{Name: "gain_resistor_value", Type: ParameterResistance, Default: "1kΩ", Description: "Signal-star leg of the feedback divider."},
			{Name: "opamp_component_id", Type: ParameterString, Default: "opamp.ti.opa134ua.soic8", Description: "Concrete dual-rail audio op-amp component ID."},
			{Name: "output_isolation_value", Type: ParameterResistance, Default: "47Ω", Description: "Local op-amp output isolation resistor."},
		},
		Ports: []BlockPort{
			{Name: "IN", Direction: PortInput, Description: "Ground-referenced audio input."},
			{Name: "DRIVER_OUT", Direction: PortOutput, Description: "Isolated drive to the Class AB bias/output stage."},
			{Name: "FEEDBACK_SENSE", Direction: PortInput, Description: "Kelvin feedback sensed at the load side of the power-stage emitter resistors."},
			{Name: "VCC", Direction: PortPower, Description: "Positive analog rail."},
			{Name: "VEE", Direction: PortPower, Description: "Negative analog rail."},
			{Name: "SIGNAL_STAR", Direction: PortPower, Description: "Quiet signal return joined to the power star at one declared point."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Amplifier_Operational:OPA134", Required: true, Description: "Reviewed single audio op-amp."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Feedback and isolation resistors."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Local dual-rail bypass capacitors."},
		},
		Components: []BlockComponent{
			{Role: "opamp", RefPrefix: "U", Value: "OPA134UA", SymbolID: "Amplifier_Operational:OPA134", FootprintID: "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm", Pins: opAmpPinsWithAuxiliary(opAmpPinRoleMap{OUT: "6", INN: "2", INP: "3", VEE: "4", VCC: "7"}, "1", "5", "8"), PreferResolverSymbol: true, ComponentIDParam: "opamp_component_id", MinimumConfidence: verified, Acceptance: components.AcceptanceFabricationCandidate, PinmapRequired: true, PlacementGroup: "gain_loop"},
			{Role: "gain_to_star", RefPrefix: "R", Value: "1kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance", ToleranceKind: "resistance", MaximumTolerance: 1, ToleranceUnit: "%"}, ComponentValueParam: "gain_resistor_value", MinimumConfidence: verified, Acceptance: components.AcceptanceFabricationCandidate, PinmapRequired: true, PlacementGroup: "gain_loop"},
			{Role: "feedback", RefPrefix: "R", Value: "10kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance", ToleranceKind: "resistance", MaximumTolerance: 1, ToleranceUnit: "%"}, ComponentValueParam: "feedback_resistor_value", MinimumConfidence: verified, Acceptance: components.AcceptanceFabricationCandidate, PinmapRequired: true, PlacementGroup: "gain_loop"},
			{Role: "output_isolation", RefPrefix: "R", Value: "47Ω", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "resistor.yageo.rc0805fr_0747rl.0805", ComponentVariant: "0805", MinimumConfidence: verified, Acceptance: components.AcceptanceFabricationCandidate, PinmapRequired: true, PlacementGroup: "output_drive"},
			{Role: "positive_bypass", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_THT:C_Rect_L7.2mm_W3.0mm_P5.00mm_FKS2_FKP2_MKS2_MKP2", Pins: twoTerminalHorizontalPins(), ComponentID: "capacitor.wima.mks2c031001a00kssd.tht", ComponentVariant: "mks2_pcm5", MinimumConfidence: verified, Acceptance: components.AcceptanceFabricationCandidate, PinmapRequired: true, PlacementGroup: "rail_bypass"},
			{Role: "negative_bypass", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_THT:C_Rect_L7.2mm_W3.0mm_P5.00mm_FKS2_FKP2_MKS2_MKP2", Pins: twoTerminalHorizontalPins(), ComponentID: "capacitor.wima.mks2c031001a00kssd.tht", ComponentVariant: "mks2_pcm5", MinimumConfidence: verified, Acceptance: components.AcceptanceFabricationCandidate, PinmapRequired: true, PlacementGroup: "rail_bypass"},
		},
		Nets: []BlockNet{
			{NameTemplate: "in", Visibility: "exported", Role: "audio_input", Pins: []NetPin{{ComponentRole: "opamp", Pin: "3"}}},
			{NameTemplate: "feedback_sense", Visibility: "exported", Role: "load_side_feedback", Pins: []NetPin{{ComponentRole: "feedback", Pin: "1"}}},
			{NameTemplate: "feedback_node", Visibility: "local", Role: "inverting_input", Pins: []NetPin{{ComponentRole: "feedback", Pin: "2"}, {ComponentRole: "opamp", Pin: "2"}, {ComponentRole: "gain_to_star", Pin: "1"}}},
			{NameTemplate: "signal_star", Visibility: "exported", Role: "quiet_signal_return", Pins: []NetPin{{ComponentRole: "gain_to_star", Pin: "2"}, {ComponentRole: "positive_bypass", Pin: "2"}, {ComponentRole: "negative_bypass", Pin: "2"}}},
			{NameTemplate: "opamp_out", Visibility: "local", Role: "opamp_output", Pins: []NetPin{{ComponentRole: "opamp", Pin: "6"}, {ComponentRole: "output_isolation", Pin: "1"}}},
			{NameTemplate: "driver_out", Visibility: "exported", Role: "power_stage_drive", Pins: []NetPin{{ComponentRole: "output_isolation", Pin: "2"}}},
			{NameTemplate: "vcc", Visibility: "exported", Role: "positive_analog_rail", Pins: []NetPin{{ComponentRole: "opamp", Pin: "7"}, {ComponentRole: "positive_bypass", Pin: "1"}}},
			{NameTemplate: "vee", Visibility: "exported", Role: "negative_analog_rail", Pins: []NetPin{{ComponentRole: "opamp", Pin: "4"}, {ComponentRole: "negative_bypass", Pin: "1"}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "speaker_gain_flow", ComponentRole: "opamp", XMM: 38, YMM: 20},
			{Kind: "speaker_gain_flow", ComponentRole: "gain_to_star", XMM: 28, YMM: 38},
			{Kind: "speaker_gain_flow", ComponentRole: "feedback", XMM: 48, YMM: 8},
			{Kind: "speaker_gain_flow", ComponentRole: "output_isolation", XMM: 58, YMM: 20},
			{Kind: "speaker_gain_flow", ComponentRole: "positive_bypass", XMM: 38, YMM: 2},
			{Kind: "speaker_gain_flow", ComponentRole: "negative_bypass", XMM: 38, YMM: 42},
		},
		PCBRealization: speakerOpAmpDriverPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "speaker.opamp.gain.bounded", Severity: BlockValidationSeverityBlocked, Description: "Gain must be finite, greater than one, and realizable by the deterministic E24 feedback network."},
			{ID: "speaker.opamp.feedback.kelvin", Severity: BlockValidationSeverityBlocked, Description: "Feedback must originate at the load side of both emitter resistors."},
			{ID: "speaker.opamp.return.star", Severity: BlockValidationSeverityBlocked, Description: "The feedback divider and bypass capacitors return through the declared quiet signal star."},
		},
		Verification: VerificationRecord{Level: VerificationStructural, Notes: []string{"OPA134 pin mapping and load-side feedback topology are explicit; fabrication readiness remains gated on real KiCad and applied stability evidence."}},
	}
}

func instantiateSpeakerOpAmpDriver(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	params = ApplyParameterDefaults(definition, params)
	gain, gainOK := numericValue(params["gain"])
	rg, rgOK := parseUnit(params["gain_resistor_value"], "Ω", resistanceMultipliers())
	if !gainOK || gain < 2 || gain > 30 || !rgOK || rg <= 0 {
		issues = append(issues, blockIssue("params.gain", "gain must be finite in the 2-30 range with a positive gain resistor"))
	}
	if strings.TrimSpace(stringParam(params, "opamp_component_id")) == "" {
		issues = append(issues, blockIssue("params.opamp_component_id", "a concrete op-amp component ID is required"))
	}
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}
	rf := nearestStandardResistance((gain - 1) * rg)
	actualGain := 1 + rf/rg
	if math.Abs(actualGain-gain)/gain > 0.03 {
		issues = append(issues, blockIssue("params.gain", "nearest deterministic E24 feedback pair exceeds the three-percent gain error bound"))
	}
	params["feedback_resistor_value"] = formatOhms(rf)
	params["actual_gain"] = actualGain
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	refsByRole := map[string]string{}
	var refs []string
	var operations []transactions.Operation
	hints := map[string]SchematicHint{}
	for _, hint := range definition.SchematicHints {
		hints[hint.ComponentRole] = hint
	}
	for _, component := range definition.Components {
		switch component.Role {
		case "opamp":
			component.ComponentID = stringParam(params, "opamp_component_id")
		case "gain_to_star":
			component.Value = stringParam(params, "gain_resistor_value")
		case "feedback":
			component.Value = formatOhms(rf)
		case "output_isolation":
			component.Value = stringParam(params, "output_isolation_value")
		}
		ref := allocator.Next(component.RefPrefix)
		refsByRole[component.Role] = ref
		refs = append(refs, ref)
		hint := hints[component.Role]
		ops, componentIssues := ComponentOperations(component, ref, transactions.Point{XMM: hint.XMM, YMM: hint.YMM})
		operations = append(operations, ops...)
		issues = append(issues, componentIssues...)
	}
	var nets []string
	for _, net := range definition.Nets {
		netName := InstanceNetName(request.InstanceID, net.NameTemplate)
		nets = append(nets, netName)
		for index := 1; index < len(net.Pins); index++ {
			from, to := net.Pins[index-1], net.Pins[index]
			fromRef, fromOK := instantiatedRoleRef(refsByRole, from.ComponentRole, &issues)
			toRef, toOK := instantiatedRoleRef(refsByRole, to.ComponentRole, &issues)
			if fromOK && toOK {
				appendConnectOperation(&operations, &issues, fromRef, from.Pin, toRef, to.Pin, netName)
			}
		}
	}
	opampRef, opampOK := instantiatedRoleRef(refsByRole, "opamp", &issues)
	// KiCad's OPA134 extends OP07, whose hidden pin 5 is already declared with
	// electrical type no_connect. Emit markers only for the two visible VOS pins;
	// placing another marker on hidden pin 5 creates SCH_PIN_NC_MISSING in ERC.
	for _, pin := range []string{"1", "8"} {
		if !opampOK {
			break
		}
		operation, operationIssues := NoConnectOperation(opampRef, pin)
		issues = append(issues, operationIssues...)
		if len(operationIssues) == 0 {
			operations = append(operations, operation)
		}
	}
	for _, port := range []struct{ name, role, pin, net string }{
		{name: "IN", role: "opamp", pin: "3", net: "in"},
		{name: "FEEDBACK_SENSE", role: "feedback", pin: "1", net: "feedback_sense"},
		{name: "DRIVER_OUT", role: "output_isolation", pin: "2", net: "driver_out"},
		{name: "VCC", role: "opamp", pin: "7", net: "vcc"},
		{name: "VEE", role: "opamp", pin: "4", net: "vee"},
		{name: "SIGNAL_STAR", role: "gain_to_star", pin: "2", net: "signal_star"},
	} {
		roleRef, ok := instantiatedRoleRef(refsByRole, port.role, &issues)
		if ok {
			appendConnectOperation(&operations, &issues, request.InstanceID, port.name, roleRef, port.pin, InstanceNetName(request.InstanceID, port.net))
		}
	}
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func instantiatedRoleRef(refsByRole map[string]string, role string, issues *[]reports.Issue) (string, bool) {
	ref, ok := refsByRole[role]
	if ok && strings.TrimSpace(ref) != "" {
		return ref, true
	}
	if issues != nil {
		*issues = append(*issues, blockIssue("definition.component_roles."+role, "instantiator requires a component role that is absent from the block definition"))
	}
	return "", false
}

func speakerOpAmpDriverPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version: "0.1.0", VerificationLevel: PCBVerificationConnectivityVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "opamp", FootprintID: "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm", Placement: RelativePlacement{XMM: 18, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "gain_to_star", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 10, YMM: 8, Layer: "F.Cu"}},
			{ComponentRole: "feedback", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 18, YMM: -7, Layer: "F.Cu"}},
			{ComponentRole: "output_isolation", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 26, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "positive_bypass", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Placement: RelativePlacement{XMM: 16, YMM: -11, Layer: "F.Cu"}},
			{ComponentRole: "negative_bypass", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Placement: RelativePlacement{XMM: 20, YMM: 11, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "in", Port: "IN", NetTemplate: "in", Placement: RelativePlacement{XMM: -4, YMM: 0, Layer: "B.Cu"}},
			{ID: "feedback_sense", Port: "FEEDBACK_SENSE", NetTemplate: "feedback_sense", Placement: RelativePlacement{XMM: 17.0875, YMM: -7, Layer: "B.Cu"}},
			{ID: "driver_out", Port: "DRIVER_OUT", NetTemplate: "driver_out", Placement: RelativePlacement{XMM: 32, YMM: 0, Layer: "F.Cu"}},
			{ID: "vcc", Port: "VCC", NetTemplate: "vcc", Placement: RelativePlacement{XMM: 12, YMM: -15, Layer: "F.Cu"}},
			{ID: "vee", Port: "VEE", NetTemplate: "vee", Placement: RelativePlacement{XMM: 24, YMM: 15, Layer: "F.Cu"}},
			{ID: "signal_star", Port: "SIGNAL_STAR", NetTemplate: "signal_star", Placement: RelativePlacement{XMM: 10.9125, YMM: 8, Layer: "B.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{
			{ID: "gain_loop", ComponentRoles: []string{"opamp", "gain_to_star", "feedback"}, AnchorRole: "opamp", Bounds: &RelativeBounds{MinXMM: 7, MinYMM: -10, MaxXMM: 24, MaxYMM: 11}},
			{ID: "output_drive", ComponentRoles: []string{"opamp", "output_isolation"}, AnchorRole: "output_isolation", Bounds: &RelativeBounds{MinXMM: 15, MinYMM: -3, MaxXMM: 30, MaxYMM: 3}},
			{ID: "rail_bypass", ComponentRoles: []string{"opamp", "positive_bypass", "negative_bypass"}, AnchorRole: "opamp", Bounds: &RelativeBounds{MinXMM: 13, MinYMM: -14, MaxXMM: 23, MaxYMM: 14}},
		},
		LocalRoutes: []PCBLocalRoute{
			{ID: "input", NetTemplate: "in", From: RouteEndpoint{Port: "IN"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: "3"}, Layer: "B.Cu", WidthMM: 0.3, Required: true},
			{ID: "feedback_sense", NetTemplate: "feedback_sense", From: RouteEndpoint{Port: "FEEDBACK_SENSE"}, To: RouteEndpoint{ComponentRole: "feedback", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 32, YMM: -12}, {XMM: 16, YMM: -12}}, Layer: "B.Cu", WidthMM: 0.3, Required: true},
			{ID: "feedback_to_input", NetTemplate: "feedback_node", From: RouteEndpoint{ComponentRole: "feedback", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "gain_to_input", NetTemplate: "feedback_node", From: RouteEndpoint{ComponentRole: "gain_to_star", Pin: "1"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "gain_return", NetTemplate: "signal_star", From: RouteEndpoint{ComponentRole: "gain_to_star", Pin: "2"}, To: RouteEndpoint{Port: "SIGNAL_STAR"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
			{ID: "opamp_output", NetTemplate: "opamp_out", From: RouteEndpoint{ComponentRole: "opamp", Pin: "6"}, To: RouteEndpoint{ComponentRole: "output_isolation", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "driver_output", NetTemplate: "driver_out", From: RouteEndpoint{ComponentRole: "output_isolation", Pin: "2"}, To: RouteEndpoint{Port: "DRIVER_OUT"}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true},
			{ID: "positive_bypass", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "opamp", Pin: "7"}, To: RouteEndpoint{ComponentRole: "positive_bypass", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "negative_bypass", NetTemplate: "vee", From: RouteEndpoint{ComponentRole: "opamp", Pin: "4"}, To: RouteEndpoint{ComponentRole: "negative_bypass", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "load_side_feedback", Kind: "kelvin_origin", Category: PCBConstraintFeedbackSense, NetTemplate: "feedback_sense", AppliesTo: []string{"feedback", "opamp"}, MaxLengthMM: 20, Description: "Route load-side feedback away from the output current loop."},
			{ID: "quiet_return", Kind: "return_topology", Category: PCBConstraintReturnTopology, NetTemplate: "signal_star", AppliesTo: []string{"gain_to_star", "positive_bypass", "negative_bypass"}, Description: "Join quiet returns only at the declared signal star."},
			{ID: "gain_loop_length", Kind: "max_spacing", Category: PCBConstraintFeedbackSense, AppliesTo: []string{"opamp", "feedback", "gain_to_star"}, MaxLengthMM: 8, Required: true, Description: "Keep the inverting-node loop compact."},
			{ID: "input_output_separation", Kind: "minimum_separation", Category: PCBConstraintAnalogInputSeparation, NetTemplate: "in", AppliesTo: []string{"output_isolation"}, MaxLengthMM: 12, Description: "Separate the input from the driver-output path."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"in", "feedback_sense", "feedback_node", "signal_star", "opamp_out", "driver_out", "vcc", "vee"}, RequiredRoutes: []string{"input", "feedback_sense", "feedback_to_input", "opamp_output", "driver_output", "positive_bypass", "negative_bypass"}, RequiresDRC: true},
	}
}

func speakerFloatPointer(value float64) *float64 {
	return &value
}
