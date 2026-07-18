package blocks

import (
	"math"
	"strconv"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const classAVoltageStageID = "class_a_voltage_stage"

type classADevicePins struct {
	Control string
	Output  string
	Return  string
}

func classAVoltageStageDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          classAVoltageStageID,
		Name:        "Class A Voltage Stage",
		Description: "Single-ended BJT common-emitter or MOSFET common-source Class A voltage stage with calculated bias, degeneration, load, and coupling networks.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "device_technology", Type: ParameterEnum, Default: "bjt", Allowed: []any{"bjt", "mosfet"}, Description: "Active-device contract used by the common-emitter/common-source stage."},
			{Name: "device_component_id", Type: ParameterString, Default: "bjt.onsemi.mmbt3904.sot23", Description: "Catalog component selected for the active device."},
			{Name: "device_symbol", Type: ParameterSymbolID, Default: "Device:Q_NPN_BEC", Description: "KiCad symbol whose terminal order matches the selected device."},
			{Name: "device_footprint", Type: ParameterFootprintID, Default: "Package_TO_SOT_SMD:SOT-23", Description: "Verified footprint for the selected device."},
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "12V", Description: "Positive single supply."},
			{Name: "target_quiescent_current", Type: ParameterCurrent, Default: "1mA", Description: "Target collector or drain current."},
			{Name: "target_output_bias", Type: ParameterVoltage, Default: "6V", Description: "Target collector or drain DC voltage."},
			{Name: "target_gain", Type: ParameterNumber, Default: 10.0, Min: float64Pointer(1), Description: "Requested midband voltage-gain magnitude."},
			{Name: "control_drop_voltage", Type: ParameterVoltage, Default: "0.65V", Description: "BJT VBE or reviewed MOSFET VGS operating-point assumption."},
			{Name: "bias_divider_current", Type: ParameterCurrent, Default: "100uA", Description: "Current through the gate/base bias divider."},
			{Name: "minimum_current_gain", Type: ParameterNumber, Default: 100.0, Min: float64Pointer(1), Description: "Conservative BJT beta used to check divider stiffness; ignored for MOSFETs."},
			{Name: "minimum_transconductance_s", Type: ParameterNumber, Default: 0.05, Min: float64Pointer(0.000001), Description: "Conservative MOSFET small-signal transconductance in siemens at the target drain current; ignored for BJTs and must be backed by device evidence before fabrication promotion."},
			{Name: "input_impedance", Type: ParameterResistance, Default: "47kΩ", Description: "Declared small-signal input impedance used for coupling-capacitor sizing."},
			{Name: "load_impedance", Type: ParameterResistance, Default: "10kΩ", Description: "Declared line-level output load."},
			{Name: "low_frequency_cutoff", Type: ParameterFrequency, Default: "20Hz", Description: "Maximum coupling high-pass corner used for deterministic capacitor sizing."},
			{Name: "coupling_policy", Type: ParameterEnum, Default: "ac_both", Allowed: []any{"ac_both"}, Description: "Initial verified envelope requires AC coupling at input and output."},
			{Name: "resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Footprint for calculated bias, load, and degeneration resistors."},
			{Name: "coupling_capacitor_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0805_2012Metric", Description: "Footprint for non-polar line-level coupling capacitors."},
			{Name: "decoupling_capacitor_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0805_2012Metric", Description: "Footprint for local high-frequency rail decoupling."},
		},
		Ports: []BlockPort{
			{Name: "IN", Direction: PortInput, Description: "Line-level signal input."},
			{Name: "OUT", Direction: PortOutput, Description: "AC-coupled line-level output."},
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Positive single supply."},
			{Name: "AGND", Direction: PortPower, Description: "Quiet analog reference and stage return."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:Q_NPN_BEC", Required: true, Description: "Default BJT common-emitter symbol."},
			{Kind: "symbol", ID: "Device:Q_NMOS_GDS", Required: true, Description: "MOSFET common-source symbol contract."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Bias, degeneration, and collector/drain load resistors."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Input, output, and local decoupling capacitors."},
		},
		Components:     classAVoltageStageComponents(),
		PCBRealization: classAVoltageStagePCBRealization(),
		Nets: []BlockNet{
			{NameTemplate: "in", Visibility: "exported", Role: "quiet_input"},
			{NameTemplate: "control", Visibility: "local", Role: "biased_control"},
			{NameTemplate: "output_dc", Visibility: "local", Role: "collector_or_drain_output"},
			{NameTemplate: "out", Visibility: "exported", Role: "ac_coupled_output"},
			{NameTemplate: "vcc", Visibility: "exported", Role: "positive_rail"},
			{NameTemplate: "agnd", Visibility: "exported", Role: "quiet_analog_return"},
		},
		SchematicHints: []SchematicHint{
			{Kind: "signal_flow", ComponentRole: "input_coupling", XMM: 0, YMM: 0},
			{Kind: "bias", ComponentRole: "bias_top", XMM: 10, YMM: -7.62},
			{Kind: "bias", ComponentRole: "bias_bottom", XMM: 10, YMM: 7.62},
			{Kind: "active_device", ComponentRole: "active_device", XMM: 25, YMM: 0},
			{Kind: "load", ComponentRole: "stage_load", XMM: 25, YMM: -12.7},
			{Kind: "degeneration", ComponentRole: "degeneration", XMM: 25, YMM: 12.7},
			{Kind: "signal_flow", ComponentRole: "output_coupling", XMM: 40, YMM: -5.08},
			{Kind: "decoupling", ComponentRole: "local_decoupling", XMM: 40, YMM: 10.16},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "class_a.operating_point.headroom", Severity: BlockValidationSeverityBlocked, Description: "The requested output bias must leave positive load and active-device headroom."},
			{ID: "class_a.bias.divider_stiffness", Severity: BlockValidationSeverityBlocked, Description: "A BJT bias divider must source at least ten times the conservative base current."},
			{ID: "class_a.coupling.cutoff", Severity: BlockValidationSeverityBlocked, Description: "Coupling capacitors are calculated so each declared high-pass corner does not exceed the requested cutoff."},
			{ID: "class_a.device.catalog_contract", Severity: BlockValidationSeverityBlocked, Description: "Fabrication promotion requires a catalog-selected device whose technology, pinmap, ratings, thermal evidence, and SOA cover the operating point."},
			{ID: "class_a.thermal.validation", Severity: BlockValidationSeverityBlocked, Description: "Device and load-resistor dissipation must pass the amplifier thermal validator before fabrication promotion."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"component:bjt.onsemi.mmbt3904.sot23",
				"amplifier.class_a:calculated_operating_point",
			},
			Notes: []string{
				"BJT and MOSFET terminal contracts lower deterministically; fabrication acceptance remains catalog- and analysis-gated.",
				"The first promotion envelope is a low-power BJT line preamplifier.",
			},
		},
	}
}

func classAVoltageStageComponents() []BlockComponent {
	resistor := func(role string) BlockComponent {
		return BlockComponent{Role: role, RefPrefix: "R", Value: "1kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, ComponentPackageParam: "resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "class_a_core"}
	}
	capacitor := func(role string, footprintParam string) BlockComponent {
		return BlockComponent{Role: role, RefPrefix: "C", Value: "1uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", ValueKind: "capacitance"}, ComponentPackageParam: footprintParam, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "class_a_core"}
	}
	return []BlockComponent{
		capacitor("input_coupling", "coupling_capacitor_footprint"),
		resistor("bias_top"),
		resistor("bias_bottom"),
		{Role: "active_device", RefPrefix: "Q", Value: "MMBT3904", SymbolID: "Device:Q_NPN_BEC", FootprintID: "Package_TO_SOT_SMD:SOT-23", Pins: bjtBECPins(), ComponentID: "bjt.onsemi.mmbt3904.sot23", ComponentIDParam: "device_component_id", MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true, PlacementGroup: "class_a_core"},
		resistor("stage_load"),
		resistor("degeneration"),
		capacitor("output_coupling", "coupling_capacitor_footprint"),
		capacitor("local_decoupling", "decoupling_capacitor_footprint"),
	}
}

func classAVoltageStagePCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationUnrealized,
		Components: []PCBComponentRealization{
			{ComponentRole: "input_coupling", FootprintParam: "coupling_capacitor_footprint", Placement: RelativePlacement{XMM: -10, YMM: 1, Layer: "F.Cu"}},
			{ComponentRole: "bias_top", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: -5, YMM: -3, RotationDeg: 270, Layer: "F.Cu"}},
			{ComponentRole: "bias_bottom", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: -5, YMM: 4, RotationDeg: 270, Layer: "F.Cu"}},
			{ComponentRole: "active_device", FootprintParam: "device_footprint", Placement: RelativePlacement{XMM: 2, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "stage_load", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: -4, RotationDeg: 270, Layer: "F.Cu"}},
			{ComponentRole: "degeneration", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: 4, RotationDeg: 270, Layer: "F.Cu"}},
			{ComponentRole: "output_coupling", FootprintParam: "coupling_capacitor_footprint", Placement: RelativePlacement{XMM: 9, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "local_decoupling", FootprintParam: "decoupling_capacitor_footprint", Placement: RelativePlacement{XMM: 7, YMM: -3, RotationDeg: 270, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "in", Port: "IN", NetTemplate: "in", Placement: RelativePlacement{XMM: -14, YMM: 1, Layer: "F.Cu"}, Description: "Quiet input edge of the Class A core."},
			{ID: "out", Port: "OUT", NetTemplate: "out", Placement: RelativePlacement{XMM: 14, YMM: 0, Layer: "F.Cu"}, Description: "AC-coupled output edge kept away from the input."},
			{ID: "vcc", Port: "VCC", NetTemplate: "vcc", Placement: RelativePlacement{XMM: 2, YMM: -7, Layer: "F.Cu"}},
			{ID: "agnd", Port: "AGND", NetTemplate: "agnd", Placement: RelativePlacement{XMM: 2, YMM: 7, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{
			{ID: "class_a_core", ComponentRoles: []string{"input_coupling", "bias_top", "bias_bottom", "active_device", "stage_load", "degeneration", "output_coupling", "local_decoupling"}, AnchorRole: "active_device", Bounds: &RelativeBounds{MinXMM: -12, MinYMM: -8, MaxXMM: 12, MaxYMM: 8}, Description: "Compact signal core with separated input and output edges and local rail bypass."},
		},
		LocalRoutes: []PCBLocalRoute{
			{ID: "input_entry", NetTemplate: "in", From: RouteEndpoint{Port: "IN"}, To: RouteEndpoint{ComponentRole: "input_coupling", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, DisableEntryAnchorVia: true},
			{ID: "control_input", NetTemplate: "control", From: RouteEndpoint{ComponentRole: "input_coupling", Pin: "2"}, To: RouteEndpoint{ComponentRole: "active_device", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "bias_top_control", NetTemplate: "control", From: RouteEndpoint{ComponentRole: "bias_top", Pin: "2"}, To: RouteEndpoint{ComponentRole: "active_device", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "bias_bottom_control", NetTemplate: "control", From: RouteEndpoint{ComponentRole: "bias_bottom", Pin: "1"}, To: RouteEndpoint{ComponentRole: "active_device", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "stage_load_output", NetTemplate: "output_dc", From: RouteEndpoint{ComponentRole: "stage_load", Pin: "2"}, To: RouteEndpoint{ComponentRole: "active_device", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.35, Required: true, When: RealizationWhen{Params: map[string]any{"device_technology": "bjt"}}},
			{ID: "stage_load_output_mosfet", NetTemplate: "output_dc", From: RouteEndpoint{ComponentRole: "stage_load", Pin: "2"}, To: RouteEndpoint{ComponentRole: "active_device", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.35, Required: true, When: RealizationWhen{Params: map[string]any{"device_technology": "mosfet"}}},
			{ID: "output_coupling", NetTemplate: "output_dc", From: RouteEndpoint{ComponentRole: "stage_load", Pin: "2"}, To: RouteEndpoint{ComponentRole: "output_coupling", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 6, YMM: -2}, {XMM: 6, YMM: 0}}, Layer: "F.Cu", WidthMM: 0.35, Required: true},
			{ID: "output_entry", NetTemplate: "out", From: RouteEndpoint{ComponentRole: "output_coupling", Pin: "2"}, To: RouteEndpoint{Port: "OUT"}, Layer: "F.Cu", WidthMM: 0.3, Required: true, DisableEntryAnchorVia: true},
			{ID: "device_return_bjt", NetTemplate: "source_or_emitter", From: RouteEndpoint{ComponentRole: "active_device", Pin: "2"}, To: RouteEndpoint{ComponentRole: "degeneration", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 0, YMM: -1}, {XMM: 0, YMM: 4}}, Layer: "F.Cu", WidthMM: 0.35, Required: true, When: RealizationWhen{Params: map[string]any{"device_technology": "bjt"}}},
			{ID: "device_return_mosfet", NetTemplate: "source_or_emitter", From: RouteEndpoint{ComponentRole: "active_device", Pin: "3"}, To: RouteEndpoint{ComponentRole: "degeneration", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 0, YMM: -1}, {XMM: 0, YMM: 4}}, Layer: "F.Cu", WidthMM: 0.35, Required: true, When: RealizationWhen{Params: map[string]any{"device_technology": "mosfet"}}},
			{ID: "vcc_load", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "stage_load", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true, DisableEntryAnchorVia: true},
			{ID: "vcc_bias", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "stage_load", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bias_top", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 2, YMM: -7}, {XMM: -5, YMM: -7}}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "vcc_decoupling", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "stage_load", Pin: "1"}, To: RouteEndpoint{ComponentRole: "local_decoupling", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 2, YMM: -7}, {XMM: 7, YMM: -7}}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "agnd_return", NetTemplate: "agnd", From: RouteEndpoint{Port: "AGND"}, To: RouteEndpoint{ComponentRole: "degeneration", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.4, Required: true, DisableEntryAnchorVia: true},
			{ID: "agnd_bias", NetTemplate: "agnd", From: RouteEndpoint{ComponentRole: "degeneration", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_bottom", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 2, YMM: 7}, {XMM: -5, YMM: 7}}, Layer: "B.Cu", WidthMM: 0.3, Required: true},
			{ID: "agnd_decoupling", NetTemplate: "agnd", From: RouteEndpoint{ComponentRole: "degeneration", Pin: "2"}, To: RouteEndpoint{ComponentRole: "local_decoupling", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 2, YMM: 7}, {XMM: 8, YMM: 7}, {XMM: 8, YMM: -3}}, Layer: "B.Cu", WidthMM: 0.5, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "class_a_input_output_separation", Kind: "min_spacing", Category: PCBConstraintAnalogInputSeparation, AppliesTo: []string{"input_coupling", "output_coupling"}, ClearanceMM: 6, Description: "Separate high-impedance input copper from the inverted output node."},
			{ID: "class_a_decoupling_proximity", Kind: "max_spacing", Category: PCBConstraintDecouplingProximity, AppliesTo: []string{"active_device", "local_decoupling"}, MaxLengthMM: 6, Description: "Keep local high-frequency bypass close to the active-device rail loop."},
			{ID: "class_a_quiet_return", Kind: "return_topology", Category: PCBConstraintReturnTopology, NetTemplate: "agnd", AppliesTo: []string{"bias_bottom", "degeneration", "local_decoupling"}, Description: "Join signal bias, device degeneration, and decoupling returns at the declared analog-reference branch."},
			{ID: "class_a_thermal_separation", Kind: "thermal_keepout", Category: PCBConstraintThermalKeepout, AppliesTo: []string{"active_device", "input_coupling"}, ClearanceMM: 5, Description: "Keep the temperature-sensitive input coupling and bias edge away from the dissipating active device."},
		},
		Validation:           PCBValidationExpectations{RequiredNets: []string{"in", "control", "source_or_emitter", "output_dc", "out", "vcc", "agnd"}, RequiredRoutes: []string{"input_entry", "control_input", "bias_top_control", "bias_bottom_control", "output_coupling", "output_entry", "vcc_load", "vcc_bias", "vcc_decoupling", "agnd_return", "agnd_bias", "agnd_decoupling"}, RequiresDRC: true},
		UnsupportedBehaviors: []string{"direct-coupled input or output", "speaker-load drive", "constant-current-source load", "fabrication promotion without catalog-backed operating-point, thermal, and SOA validation"},
	}
}

func instantiateClassAVoltageStage(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	technology := strings.TrimSpace(stringParam(params, "device_technology"))
	devicePins := classADevicePins{Control: "1", Output: "3", Return: "2"}
	if technology == "mosfet" {
		devicePins = classADevicePins{Control: "1", Output: "2", Return: "3"}
	}
	if technology != "bjt" && technology != "mosfet" {
		issues = append(issues, blockIssue("params.device_technology", "device_technology must be bjt or mosfet"))
	}
	if stringParam(params, "device_component_id") == "" {
		issues = append(issues, blockIssue("params.device_component_id", "device_component_id is required for catalog evidence selection"))
	}
	if stringParam(params, "device_symbol") == "" || stringParam(params, "device_footprint") == "" {
		issues = append(issues, blockIssue("params.device_symbol", "device_symbol and device_footprint are required"))
	}
	vcc, vccOK := positiveClassAUnit(params, "supply_voltage", "V", voltageMultipliers(), &issues)
	iq, iqOK := positiveClassAUnit(params, "target_quiescent_current", "A", currentMultipliers(), &issues)
	vout, voutOK := positiveClassAUnit(params, "target_output_bias", "V", voltageMultipliers(), &issues)
	controlDrop, dropOK := positiveClassAUnit(params, "control_drop_voltage", "V", voltageMultipliers(), &issues)
	biasCurrent, biasOK := positiveClassAUnit(params, "bias_divider_current", "A", currentMultipliers(), &issues)
	inputOhms, inputOK := positiveClassAUnit(params, "input_impedance", "Ω", resistanceMultipliers(), &issues)
	loadOhms, loadOK := positiveClassAUnit(params, "load_impedance", "Ω", resistanceMultipliers(), &issues)
	cutoffHz, cutoffOK := positiveClassAUnit(params, "low_frequency_cutoff", "Hz", classAFrequencyMultipliers(), &issues)
	gain, gainOK := numericValue(params["target_gain"])
	if !gainOK || gain < 1 || !finite(gain) {
		issues = append(issues, blockIssue("params.target_gain", "target_gain must be a finite number of at least 1"))
	}
	beta, betaOK := numericValue(params["minimum_current_gain"])
	if !betaOK || beta <= 0 || !finite(beta) {
		issues = append(issues, blockIssue("params.minimum_current_gain", "minimum_current_gain must be a positive finite number"))
	}
	minimumTransconductanceS, transconductanceOK := numericValue(params["minimum_transconductance_s"])
	if technology == "mosfet" && (!transconductanceOK || minimumTransconductanceS <= 0 || !finite(minimumTransconductanceS)) {
		issues = append(issues, blockIssue("params.minimum_transconductance_s", "MOSFET minimum_transconductance_s must be a positive finite reviewed value"))
	}
	if stringParam(params, "coupling_policy") != "ac_both" {
		issues = append(issues, blockIssue("params.coupling_policy", "the initial Class A envelope requires ac_both coupling"))
	}
	if vccOK && voutOK && vout >= vcc {
		issues = append(issues, blockIssue("params.target_output_bias", "target_output_bias must be below supply_voltage"))
	}
	if technology == "bjt" && iqOK && biasOK && betaOK && biasCurrent*(1+1e-12) < 10*(iq/beta) {
		issues = append(issues, blockIssue("params.bias_divider_current", "BJT bias divider current must be at least ten times the conservative base current"))
	}

	var stageLoadOhms, degenerationOhms, biasTopOhms, biasBottomOhms float64
	if vccOK && iqOK && voutOK && dropOK && biasOK && gainOK && gain >= 1 && vout < vcc && (technology != "mosfet" || transconductanceOK) {
		stageLoadOhms = (vcc - vout) / iq
		intrinsicOhms := 0.0
		if technology == "bjt" {
			intrinsicOhms = 0.02585 / iq
		} else if technology == "mosfet" {
			// The common-source gain denominator includes the conservative
			// intrinsic source resistance 1/gm. This prevents ideal-gm sizing
			// from overstating gain at low drain current.
			intrinsicOhms = 1 / minimumTransconductanceS
		}
		degenerationOhms = stageLoadOhms/gain - intrinsicOhms
		if degenerationOhms <= 0 || !finite(degenerationOhms) {
			issues = append(issues, blockIssue("params.target_gain", "requested gain leaves no positive emitter/source degeneration; reduce target_gain or change the operating point"))
		}
		controlBias := iq*math.Max(degenerationOhms, 0) + controlDrop
		if controlBias >= vcc {
			issues = append(issues, blockIssue("params.control_drop_voltage", "calculated gate/base bias reaches or exceeds the supply rail"))
		} else {
			upperDividerCurrent := biasCurrent
			if technology == "bjt" {
				// The lower leg carries the declared divider current while the
				// upper leg must also supply the conservative base current.
				upperDividerCurrent += iq / beta
				params["calculated_conservative_base_current_a"] = iq / beta
			}
			biasTopOhms = (vcc - controlBias) / upperDividerCurrent
			biasBottomOhms = controlBias / biasCurrent
		}
		params["calculated_stage_load_ohms"] = stageLoadOhms
		params["calculated_intrinsic_device_resistance_ohms"] = intrinsicOhms
		params["calculated_degeneration_ohms"] = degenerationOhms
		params["calculated_control_bias_v"] = controlBias
		params["calculated_bias_top_ohms"] = biasTopOhms
		params["calculated_bias_bottom_ohms"] = biasBottomOhms
		params["calculated_device_dissipation_w"] = (vout - iq*math.Max(degenerationOhms, 0)) * iq
		params["calculated_load_dissipation_w"] = (vcc - vout) * iq
	}
	if inputOK && loadOK && cutoffOK {
		params["calculated_input_coupling_f"] = preferredCapacitanceAtLeast(1 / (2 * math.Pi * inputOhms * cutoffHz))
		params["calculated_output_coupling_f"] = preferredCapacitanceAtLeast(1 / (2 * math.Pi * loadOhms * cutoffHz))
	}
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	componentsByRole := blockComponentByRole(definition.Components)
	roles := []string{"input_coupling", "bias_top", "bias_bottom", "active_device", "stage_load", "degeneration", "output_coupling", "local_decoupling"}
	refs := make(map[string]string, len(roles))
	for _, role := range roles {
		refs[role] = allocator.Next(componentsByRole[role].RefPrefix)
	}
	points := classAVoltageStageHintPoints(definition)
	inputCap := params["calculated_input_coupling_f"].(float64)
	outputCap := params["calculated_output_coupling_f"].(float64)
	var operations []transactions.Operation
	for _, role := range roles {
		component := componentsByRole[role]
		switch role {
		case "active_device":
			component.ComponentID = stringParam(params, "device_component_id")
			component.SymbolID = stringParam(params, "device_symbol")
			component.FootprintID = stringParam(params, "device_footprint")
			if technology == "mosfet" {
				component.Value = "NMOS Class A"
				component.Pins = []transactions.PinSpec{{Number: "1"}, {Number: "2"}, {Number: "3"}}
			} else {
				component.Value = "NPN Class A"
				component.Pins = bjtBECPins()
			}
		case "bias_top":
			component.Value = formatOhms(biasTopOhms)
			component.FootprintID = stringParam(params, "resistor_footprint")
		case "bias_bottom":
			component.Value = formatOhms(biasBottomOhms)
			component.FootprintID = stringParam(params, "resistor_footprint")
		case "stage_load":
			component.Value = formatOhms(stageLoadOhms)
			component.FootprintID = stringParam(params, "resistor_footprint")
		case "degeneration":
			component.Value = formatOhms(degenerationOhms)
			component.FootprintID = stringParam(params, "resistor_footprint")
		case "input_coupling":
			component.Value = formatFarads(inputCap)
			component.FootprintID = stringParam(params, "coupling_capacitor_footprint")
		case "output_coupling":
			component.Value = formatFarads(outputCap)
			component.FootprintID = stringParam(params, "coupling_capacitor_footprint")
		case "local_decoupling":
			component.Value = "100nF"
			component.FootprintID = stringParam(params, "decoupling_capacitor_footprint")
		}
		componentOps, componentIssues := ComponentOperations(component, refs[role], points[role])
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}

	nets := appendClassAVoltageStageConnections(request.InstanceID, refs, devicePins, &operations, &issues)
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	for _, role := range roles {
		output.Instance.Refs = append(output.Instance.Refs, refs[role])
	}
	output.Instance.Nets = nets
	return output
}

func positiveClassAUnit(params map[string]any, name string, suffix string, multipliers []unitMultiplier, issues *[]reports.Issue) (float64, bool) {
	value, ok := parseUnit(params[name], suffix, multipliers)
	if !ok || value <= 0 || !finite(value) {
		*issues = append(*issues, blockIssue("params."+name, name+" must be a positive "+suffix+" literal"))
		return 0, false
	}
	return value, true
}

func classAFrequencyMultipliers() []unitMultiplier {
	return []unitMultiplier{{"m", 1e-3}, {"k", 1e3}, {"K", 1e3}, {"M", 1e6}, {"G", 1e9}}
}

func classAVoltageStageHintPoints(definition BlockDefinition) map[string]transactions.Point {
	points := make(map[string]transactions.Point, len(definition.SchematicHints))
	for _, hint := range definition.SchematicHints {
		if hint.ComponentRole != "" {
			points[hint.ComponentRole] = transactions.Point{XMM: hint.XMM, YMM: hint.YMM}
		}
	}
	return points
}

func appendClassAVoltageStageConnections(instanceID string, refs map[string]string, pins classADevicePins, operations *[]transactions.Operation, issues *[]reports.Issue) []string {
	connect := func(fromRole string, fromPin string, toRole string, toPin string, netRole string) {
		fromRef := refs[fromRole]
		if fromRole == "port" {
			fromRef = instanceID
		}
		toRef := refs[toRole]
		if toRole == "port" {
			toRef = instanceID
		}
		appendConnectOperation(operations, issues, fromRef, fromPin, toRef, toPin, InstanceNetName(instanceID, netRole))
	}
	connect("port", "IN", "input_coupling", "1", "in")
	connect("input_coupling", "2", "active_device", pins.Control, "control")
	connect("bias_top", "2", "active_device", pins.Control, "control")
	connect("bias_bottom", "1", "active_device", pins.Control, "control")
	connect("stage_load", "2", "active_device", pins.Output, "output_dc")
	connect("stage_load", "2", "output_coupling", "1", "output_dc")
	connect("output_coupling", "2", "port", "OUT", "out")
	connect("active_device", pins.Return, "degeneration", "1", "source_or_emitter")
	connect("port", "VCC", "stage_load", "1", "vcc")
	connect("stage_load", "1", "bias_top", "1", "vcc")
	connect("stage_load", "1", "local_decoupling", "1", "vcc")
	connect("port", "AGND", "degeneration", "2", "agnd")
	connect("degeneration", "2", "bias_bottom", "2", "agnd")
	connect("degeneration", "2", "local_decoupling", "2", "agnd")
	return []string{
		InstanceNetName(instanceID, "in"),
		InstanceNetName(instanceID, "control"),
		InstanceNetName(instanceID, "source_or_emitter"),
		InstanceNetName(instanceID, "output_dc"),
		InstanceNetName(instanceID, "out"),
		InstanceNetName(instanceID, "vcc"),
		InstanceNetName(instanceID, "agnd"),
	}
}

func preferredCapacitanceAtLeast(farads float64) float64 {
	if farads <= 0 || !finite(farads) {
		return 0
	}
	exponent := math.Floor(math.Log10(farads))
	scale := math.Pow(10, exponent)
	normalized := farads / scale
	for _, preferred := range []float64{1, 1.2, 1.5, 1.8, 2.2, 2.7, 3.3, 3.9, 4.7, 5.6, 6.8, 8.2, 10} {
		if normalized <= preferred*(1+1e-12) {
			return preferred * scale
		}
	}
	return 10 * scale
}

func formatFarads(farads float64) string {
	switch {
	case farads >= 1:
		return formatScaledBlockValue(farads) + "F"
	case farads >= 1e-3:
		return formatScaledBlockValue(farads*1e3) + "mF"
	case farads >= 1e-6:
		return formatScaledBlockValue(farads*1e6) + "uF"
	case farads >= 1e-9:
		return formatScaledBlockValue(farads*1e9) + "nF"
	default:
		return formatScaledBlockValue(farads*1e12) + "pF"
	}
}

func formatScaledBlockValue(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconvFormatFloat(value), "0"), ".")
}

func strconvFormatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func float64Pointer(value float64) *float64 {
	return &value
}
