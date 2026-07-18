package blocks

import (
	"math"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const speakerOutputProtectionID = "speaker_output_protection"

var speakerDelayEvidence = struct {
	ResistorComponentID        string
	CapacitorComponentID       string
	ZenerComponentID           string
	ResistorToleranceFraction  float64
	CapacitorToleranceFraction float64
	ZenerToleranceFraction     float64
}{
	ResistorComponentID:        "resistor.vishay.tnpw0805.680k.1p0",
	CapacitorComponentID:       "capacitor.kemet.t491a475k025at.case_a",
	ZenerComponentID:           "diode.diodes.ddz5v1b_7.sod123",
	ResistorToleranceFraction:  0.01,
	CapacitorToleranceFraction: 0.10,
	ZenerToleranceFraction:     0.02564,
}

const (
	speakerRelayDriverVBEMinV = 0.55
	speakerRelayDriverVBEMaxV = 0.85
)

func speakerOutputProtectionDefinition() BlockDefinition {
	verified := components.ConfidenceVerified
	fabrication := components.AcceptanceFabricationCandidate
	return BlockDefinition{
		ID:          speakerOutputProtectionID,
		Name:        "Speaker Output Protection",
		Description: "Normally-open speaker relay with bipolar DC window detection, delayed engagement, fast supply-loss release, and coil clamp.",
		Version:     "0.1.0",
		Category:    "protection",
		Parameters: []BlockParameter{
			{Name: "protection_supply_voltage", Type: ParameterVoltage, Default: "12V", Description: "Regulated relay and detector supply."},
			{Name: "dc_trip_threshold", Type: ParameterVoltage, Default: "1V", Description: "Nominal positive and negative raw-output DC trip magnitude."},
			{Name: "sense_input_resistance", Type: ParameterResistance, Default: "47kΩ", Description: "Raw-output injection resistance into the level-shifted detector."},
			{Name: "sense_bias_resistance", Type: ParameterResistance, Default: "10kΩ", Description: "Equal upper and lower detector bias resistances."},
			{Name: "reference_bottom_resistance", Type: ParameterResistance, Default: "10kΩ", Description: "Bottom resistance for both window thresholds."},
			{Name: "resistor_tolerance_percent", Type: ParameterNumber, Default: 0.1, Description: "Applied precision detector resistor tolerance."},
			{Name: "delay_resistance", Type: ParameterResistance, Default: "680kΩ", Description: "Relay engagement delay resistance."},
			{Name: "delay_capacitance", Type: ParameterCapacitance, Default: "4.7uF", Description: "Relay engagement delay capacitance."},
			{Name: "delay_zener_voltage", Type: ParameterVoltage, Default: "5.1V", Description: "Driver threshold Zener voltage."},
			{Name: "release_resistance", Type: ParameterResistance, Default: "47Ω", Description: "Supply-loss discharge-path resistance."},
			{Name: "relay_component_id", Type: ParameterString, Default: "relay.omron.g5q_1a.dc12", Description: "Concrete normally-open speaker relay."},
			{Name: "maximum_load_current", Type: ParameterCurrent, Default: "3A", Description: "Maximum protected speaker current."},
		},
		Ports: []BlockPort{
			{Name: "RAW_OUT", Direction: PortInput, NetClass: "speaker_current", Description: "Unprotected amplifier output and DC-sense origin."},
			{Name: "SPEAKER_OUT", Direction: PortOutput, NetClass: "speaker_current", Description: "Relay-isolated speaker output."},
			{Name: "PROTECT_12V", Direction: PortPower, Description: "Regulated protection and relay supply."},
			{Name: "POWER_STAR", Direction: PortPower, Description: "Protection and speaker-return star."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Comparator:TLV1701", Required: true, Description: "Two physical open-collector window comparators."},
			{Kind: "symbol", ID: "Relay:G5Q-1A", Required: true, Description: "Normally-open speaker isolation relay."},
			{Kind: "symbol", ID: "Device:D", Required: true, Description: "Flyback and supply-loss discharge diodes."},
		},
		Components:     speakerOutputProtectionComponents(verified, fabrication),
		Nets:           speakerOutputProtectionNets(),
		SchematicHints: speakerOutputProtectionSchematicHints(),
		PCBRealization: speakerOutputProtectionPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "speaker.protection.window.tolerance", Severity: BlockValidationSeverityBlocked, Description: "Both DC thresholds must remain safe under the declared resistor tolerance."},
			{ID: "speaker.protection.turn_on.delay", Severity: BlockValidationSeverityBlocked, Description: "Worst-case relay engagement must remain inside the reviewed mute interval."},
			{ID: "speaker.protection.supply_loss.release", Severity: BlockValidationSeverityBlocked, Description: "The diode discharge path must release the relay within the supply-fault limit."},
			{ID: "speaker.protection.relay.rating", Severity: BlockValidationSeverityBlocked, Description: "The normally-open relay contact rating must exceed the bounded load current."},
			{ID: "speaker.protection.coil.clamp", Severity: BlockValidationSeverityBlocked, Description: "Relay driver and flyback clamp are mandatory."},
		},
		Verification: VerificationRecord{Level: VerificationStructural, Notes: []string{"Window thresholds, tolerance corners, mute timing, supply-loss release, normally-open contact path, and coil clamp are explicit; real KiCad evidence remains required."}},
	}
}

func speakerOutputProtectionComponents(verified components.ConfidenceLevel, fabrication components.AcceptanceLevel) []BlockComponent {
	resistor := func(role, value, valueParam string, precision bool) BlockComponent {
		component := BlockComponent{Role: role, RefPrefix: "R", Value: value, SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance", ToleranceKind: "resistance", MaximumTolerance: 1, ToleranceUnit: "%"}, ComponentValueParam: valueParam, MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "dc_detector"}
		if precision {
			component.ComponentToleranceParam = "resistor_tolerance_percent"
		}
		return component
	}
	delayPullup := resistor("delay_pullup", "680kΩ", "delay_resistance", false)
	delayPullup.ComponentID = speakerDelayEvidence.ResistorComponentID
	delayPullup.ComponentQuery = nil
	items := []BlockComponent{
		resistor("sense_input", "47kΩ", "sense_input_resistance", true), resistor("sense_bias_top", "10kΩ", "sense_bias_resistance", true), resistor("sense_bias_bottom", "10kΩ", "sense_bias_resistance", true),
		resistor("upper_reference_top", "11.7kΩ", "upper_reference_top_value", true), resistor("upper_reference_bottom", "10kΩ", "reference_bottom_resistance", true), resistor("lower_reference_top", "12.5kΩ", "lower_reference_top_value", true), resistor("lower_reference_bottom", "10kΩ", "reference_bottom_resistance", true),
		{Role: "positive_detector", RefPrefix: "U", Value: "TLV1701AIDBVR", SymbolID: "Comparator:TLV1701", FootprintID: "Package_TO_SOT_SMD:SOT-23-5", Pins: speakerComparatorPins(), PreferResolverSymbol: true, ComponentID: "comparator.ti.tlv1701aidbvr.sot23_5", ComponentVariant: "sot23_5", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "dc_detector"},
		{Role: "negative_detector", RefPrefix: "U", Value: "TLV1701AIDBVR", SymbolID: "Comparator:TLV1701", FootprintID: "Package_TO_SOT_SMD:SOT-23-5", Pins: speakerComparatorPins(), PreferResolverSymbol: true, ComponentID: "comparator.ti.tlv1701aidbvr.sot23_5", ComponentVariant: "sot23_5", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "dc_detector"},
		{Role: "comparator_bypass", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_THT:C_Rect_L7.2mm_W3.0mm_P5.00mm_FKS2_FKP2_MKS2_MKP2", Pins: twoTerminalHorizontalPins(), ComponentID: "capacitor.wima.mks2c031001a00kssd.tht", ComponentVariant: "mks2_pcm5", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "dc_detector"},
		delayPullup,
		{Role: "delay_capacitor", RefPrefix: "C", Value: "4.7uF", SymbolID: "Device:C_Polarized", FootprintID: "Capacitor_Tantalum_SMD:CP_EIA-3216-18_Kemet-A", Pins: twoTerminalHorizontalPins(), ComponentID: speakerDelayEvidence.CapacitorComponentID, ComponentVariant: "eia_3216_18", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "mute_timing"},
		{Role: "delay_zener", RefPrefix: "D", Value: "5.1V", SymbolID: "Device:D_Zener", FootprintID: "Diode_SMD:D_SOD-123", Pins: twoTerminalHorizontalPins(), ComponentID: speakerDelayEvidence.ZenerComponentID, ComponentVariant: "sod_123", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "mute_timing"},
		resistor("driver_base_resistor", "4.7kΩ", "", false),
		{Role: "relay_driver", RefPrefix: "Q", Value: "MMBT3904", SymbolID: "Transistor_BJT:Q_NPN_BEC", FootprintID: "Package_TO_SOT_SMD:SOT-23", Pins: bjtBECPins(), PreferResolverSymbol: true, ComponentID: "bjt.onsemi.mmbt3904.sot23", ComponentVariant: "sot23", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "relay_drive"},
		{Role: "relay", RefPrefix: "K", Value: "G5Q-1A DC12", SymbolID: "Relay:G5Q-1A", FootprintID: "Relay_THT:Relay_SPST_Omron-G5Q-1A", Pins: speakerRelayPins(), PreferResolverSymbol: true, ComponentIDParam: "relay_component_id", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "speaker_relay"},
		{Role: "relay_flyback", RefPrefix: "D", Value: "1N4148W", SymbolID: "Device:D", FootprintID: "Diode_SMD:D_SOD-123", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "diode.onsemi.1n4148w.sod_123", ComponentVariant: "sod_123", MinimumConfidence: verified, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true, PlacementGroup: "relay_drive"},
		{Role: "supply_loss_diode", RefPrefix: "D", Value: "1N4148W", SymbolID: "Device:D", FootprintID: "Diode_SMD:D_SOD-123", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "diode.onsemi.1n4148w.sod_123", ComponentVariant: "sod_123", MinimumConfidence: verified, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true, PlacementGroup: "mute_timing"},
		resistor("release_resistor", "47Ω", "release_resistance", false),
	}
	for index := range items {
		if items[index].Role == "delay_pullup" || items[index].Role == "driver_base_resistor" || items[index].Role == "release_resistor" {
			items[index].PlacementGroup = "mute_timing"
		}
	}
	return items
}

func speakerOutputProtectionNets() []BlockNet {
	return []BlockNet{
		{NameTemplate: "protect_vcc", Visibility: "exported", Role: "protection_supply", Pins: []NetPin{{ComponentRole: "sense_bias_top", Pin: "1"}, {ComponentRole: "upper_reference_top", Pin: "1"}, {ComponentRole: "lower_reference_top", Pin: "1"}, {ComponentRole: "positive_detector", Pin: "5"}, {ComponentRole: "negative_detector", Pin: "5"}, {ComponentRole: "comparator_bypass", Pin: "1"}, {ComponentRole: "delay_pullup", Pin: "1"}, {ComponentRole: "relay", Pin: "1"}, {ComponentRole: "relay_flyback", Pin: "1"}, {ComponentRole: "release_resistor", Pin: "2"}}},
		{NameTemplate: "power_star", Visibility: "exported", Role: "protection_return", Pins: []NetPin{{ComponentRole: "sense_bias_bottom", Pin: "2"}, {ComponentRole: "upper_reference_bottom", Pin: "2"}, {ComponentRole: "lower_reference_bottom", Pin: "2"}, {ComponentRole: "positive_detector", Pin: "2"}, {ComponentRole: "negative_detector", Pin: "2"}, {ComponentRole: "comparator_bypass", Pin: "2"}, {ComponentRole: "delay_capacitor", Pin: "2"}, {ComponentRole: "relay_driver", Pin: "2"}}},
		{NameTemplate: "raw_out", Visibility: "exported", Role: "unprotected_speaker_output", Pins: []NetPin{{ComponentRole: "sense_input", Pin: "1"}, {ComponentRole: "relay", Pin: "3"}}},
		{NameTemplate: "sense_node", Visibility: "local", Role: "level_shifted_output", Pins: []NetPin{{ComponentRole: "sense_input", Pin: "2"}, {ComponentRole: "sense_bias_top", Pin: "2"}, {ComponentRole: "sense_bias_bottom", Pin: "1"}, {ComponentRole: "positive_detector", Pin: "3"}, {ComponentRole: "negative_detector", Pin: "1"}}},
		{NameTemplate: "upper_reference", Visibility: "local", Role: "positive_dc_threshold", Pins: []NetPin{{ComponentRole: "upper_reference_top", Pin: "2"}, {ComponentRole: "upper_reference_bottom", Pin: "1"}, {ComponentRole: "positive_detector", Pin: "1"}}},
		{NameTemplate: "lower_reference", Visibility: "local", Role: "negative_dc_threshold", Pins: []NetPin{{ComponentRole: "lower_reference_top", Pin: "2"}, {ComponentRole: "lower_reference_bottom", Pin: "1"}, {ComponentRole: "negative_detector", Pin: "3"}}},
		{NameTemplate: "fault_delay", Visibility: "local", Role: "wired_or_fault_and_delay", Pins: []NetPin{{ComponentRole: "positive_detector", Pin: "4"}, {ComponentRole: "negative_detector", Pin: "4"}, {ComponentRole: "delay_pullup", Pin: "2"}, {ComponentRole: "delay_capacitor", Pin: "1"}, {ComponentRole: "delay_zener", Pin: "1"}, {ComponentRole: "supply_loss_diode", Pin: "2"}}},
		{NameTemplate: "zener_drive", Visibility: "local", Role: "delayed_driver_threshold", Pins: []NetPin{{ComponentRole: "delay_zener", Pin: "2"}, {ComponentRole: "driver_base_resistor", Pin: "1"}}},
		{NameTemplate: "relay_base", Visibility: "local", Role: "relay_driver_base", Pins: []NetPin{{ComponentRole: "driver_base_resistor", Pin: "2"}, {ComponentRole: "relay_driver", Pin: "1"}}},
		{NameTemplate: "coil_low", Visibility: "local", Role: "relay_coil_sink", Pins: []NetPin{{ComponentRole: "relay", Pin: "2"}, {ComponentRole: "relay_driver", Pin: "3"}, {ComponentRole: "relay_flyback", Pin: "2"}}},
		{NameTemplate: "supply_loss", Visibility: "local", Role: "fast_release", Pins: []NetPin{{ComponentRole: "supply_loss_diode", Pin: "1"}, {ComponentRole: "release_resistor", Pin: "1"}}},
		{NameTemplate: "speaker_out", Visibility: "exported", Role: "protected_speaker_output", Pins: []NetPin{{ComponentRole: "relay", Pin: "5"}}},
	}
}

func speakerOutputProtectionSchematicHints() []SchematicHint {
	roles := []string{"sense_input", "sense_bias_top", "sense_bias_bottom", "upper_reference_top", "upper_reference_bottom", "lower_reference_top", "lower_reference_bottom", "positive_detector", "negative_detector", "comparator_bypass", "delay_pullup", "delay_capacitor", "delay_zener", "driver_base_resistor", "relay_driver", "relay", "relay_flyback", "supply_loss_diode", "release_resistor"}
	positions := [][2]float64{{8, 20}, {18, 8}, {18, 32}, {28, 5}, {28, 14}, {28, 26}, {28, 35}, {42, 12}, {42, 28}, {42, 38}, {55, 7}, {55, 32}, {63, 20}, {72, 20}, {82, 24}, {95, 20}, {88, 8}, {62, 38}, {72, 38}}
	hints := make([]SchematicHint, 0, len(roles))
	for index, role := range roles {
		hints = append(hints, SchematicHint{Kind: "speaker_protection_flow", ComponentRole: role, XMM: positions[index][0], YMM: positions[index][1]})
	}
	return hints
}

func instantiateSpeakerOutputProtection(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	params = ApplyParameterDefaults(definition, params)
	vs, vsOK := parseUnit(params["protection_supply_voltage"], "V", voltageMultipliers())
	trip, tripOK := parseUnit(params["dc_trip_threshold"], "V", voltageMultipliers())
	rin, rinOK := parseUnit(params["sense_input_resistance"], "Ω", resistanceMultipliers())
	rbias, rbiasOK := parseUnit(params["sense_bias_resistance"], "Ω", resistanceMultipliers())
	rbottom, rbottomOK := parseUnit(params["reference_bottom_resistance"], "Ω", resistanceMultipliers())
	delayR, delayROK := parseUnit(params["delay_resistance"], "Ω", resistanceMultipliers())
	delayC, delayCOK := parseUnit(params["delay_capacitance"], "F", capacitanceMultipliers())
	zener, zenerOK := parseUnit(params["delay_zener_voltage"], "V", voltageMultipliers())
	releaseR, releaseROK := parseUnit(params["release_resistance"], "Ω", resistanceMultipliers())
	loadCurrent, loadOK := parseUnit(params["maximum_load_current"], "A", currentMultipliers())
	tolerance, toleranceOK := numericValue(params["resistor_tolerance_percent"])
	if !vsOK || vs < 10 || vs > 15 || !tripOK || trip < 0.5 || trip > 1.5 || !rinOK || !rbiasOK || !rbottomOK || rin <= 0 || rbias <= 0 || rbottom <= 0 || !toleranceOK || tolerance <= 0 || tolerance > 5 {
		issues = append(issues, blockIssue("params.dc_window", "protection supply, 0.5-1.5 V trip threshold, positive detector resistances, and 0-5 percent tolerance are required"))
	}
	if !delayROK || !delayCOK || !zenerOK || delayR <= 0 || delayC <= 0 || zener <= 0 || zener+speakerRelayDriverVBEMaxV >= vs || !releaseROK || releaseR <= 0 || !loadOK || loadCurrent <= 0 {
		issues = append(issues, blockIssue("params.mute_timing", "positive delay, threshold, release, and load-current parameters are required"))
	}
	if strings.TrimSpace(stringParam(params, "relay_component_id")) == "" {
		issues = append(issues, blockIssue("params.relay_component_id", "a concrete normally-open relay component ID is required"))
	}
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}

	baseline, slope := speakerSenseTransfer(vs, rin, rbias, rbias)
	upperTarget, lowerTarget := baseline+slope*trip, baseline-slope*trip
	// Select the nearest verified E96 nominal carried by the precision-passive
	// catalog. The declared 0.1% tolerance is then applied around that selected
	// nominal below; it is not an accuracy claim against the unrounded ideal.
	precisionNominals := speakerPrecisionResistorNominals()
	if len(precisionNominals) == 0 {
		issues = append(issues, blockIssue("components.precision_reference", "no verified precision resistor inventory is available for the DC detector"))
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}
	upperTop := nearestResistance(referenceTopResistance(vs, upperTarget, rbottom), precisionNominals)
	lowerTop := nearestResistance(referenceTopResistance(vs, lowerTarget, rbottom), precisionNominals)
	if !finiteSpeakerValue(upperTop) || !finiteSpeakerValue(lowerTop) {
		issues = append(issues, blockIssue("components.precision_reference", "precision resistor selection did not produce finite detector values"))
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}
	positiveRange := speakerTripToleranceRange(vs, rin, rbias, upperTop, rbottom, tolerance/100, true)
	negativeRange := speakerTripToleranceRange(vs, rin, rbias, lowerTop, rbottom, tolerance/100, false)
	params["upper_reference_top_value"] = formatOhms(upperTop)
	params["lower_reference_top_value"] = formatOhms(lowerTop)
	params["positive_trip_min_v"] = positiveRange[0]
	params["positive_trip_max_v"] = positiveRange[1]
	params["negative_trip_min_v"] = negativeRange[0]
	params["negative_trip_max_v"] = negativeRange[1]
	if positiveRange[0] < 0.25 || negativeRange[0] < 0.25 || positiveRange[1] > 1.5 || negativeRange[1] > 1.5 {
		issues = append(issues, blockIssue("params.dc_trip_threshold", "detector tolerance corners leave the safe 0.25-1.5 V DC trip envelope"))
	}
	// These bounds come from the selected 680 kΩ TNPW (1%), T491A475K
	// capacitor (10%), and DDZ5V1B Zener (2.564%) catalog evidence.
	delayMin := speakerRelayDelay(delayR*(1-speakerDelayEvidence.ResistorToleranceFraction), delayC*(1-speakerDelayEvidence.CapacitorToleranceFraction), zener*(1-speakerDelayEvidence.ZenerToleranceFraction)+speakerRelayDriverVBEMinV, vs)
	delayMax := speakerRelayDelay(delayR*(1+speakerDelayEvidence.ResistorToleranceFraction), delayC*(1+speakerDelayEvidence.CapacitorToleranceFraction), zener*(1+speakerDelayEvidence.ZenerToleranceFraction)+speakerRelayDriverVBEMaxV, vs)
	releaseMax := 5 * releaseR * (1 + speakerDelayEvidence.ResistorToleranceFraction) * delayC * (1 + speakerDelayEvidence.CapacitorToleranceFraction)
	params["engagement_delay_min_s"] = delayMin
	params["engagement_delay_max_s"] = delayMax
	params["release_time_max_s"] = releaseMax
	if !finiteSpeakerValue(delayMin) || !finiteSpeakerValue(delayMax) || delayMin < 1 || delayMax > 4 {
		issues = append(issues, blockIssue("params.delay_capacitance", "worst-case relay engagement must remain between 1 and 4 seconds"))
	}
	if !finiteSpeakerValue(releaseMax) || releaseMax > 0.05 {
		issues = append(issues, blockIssue("params.release_resistance", "worst-case supply-loss release must not exceed 50 ms"))
	}
	if loadCurrent >= 5 {
		issues = append(issues, blockIssue("params.maximum_load_current", "load current must remain below the concrete 5 A relay contact rating"))
	}
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
		case "sense_input":
			component.Value = stringParam(params, "sense_input_resistance")
		case "sense_bias_top", "sense_bias_bottom":
			component.Value = stringParam(params, "sense_bias_resistance")
		case "upper_reference_top":
			component.Value = formatOhms(upperTop)
		case "lower_reference_top":
			component.Value = formatOhms(lowerTop)
		case "upper_reference_bottom", "lower_reference_bottom":
			component.Value = stringParam(params, "reference_bottom_resistance")
		case "delay_pullup":
			component.Value = stringParam(params, "delay_resistance")
		case "delay_capacitor":
			component.Value = stringParam(params, "delay_capacitance")
		case "delay_zener":
			component.Value = stringParam(params, "delay_zener_voltage")
		case "release_resistor":
			component.Value = stringParam(params, "release_resistance")
		case "relay":
			component.ComponentID = stringParam(params, "relay_component_id")
		}
		ref := allocator.Next(component.RefPrefix)
		refsByRole[component.Role], refs = ref, append(refs, ref)
		hint := hints[component.Role]
		ops, componentIssues := ComponentOperations(component, ref, transactions.Point{XMM: hint.XMM, YMM: hint.YMM})
		operations, issues = append(operations, ops...), append(issues, componentIssues...)
	}
	var nets []string
	for _, net := range definition.Nets {
		netName := InstanceNetName(request.InstanceID, net.NameTemplate)
		nets = append(nets, netName)
		for index := 1; index < len(net.Pins); index++ {
			from, to := net.Pins[index-1], net.Pins[index]
			appendRoleConnectOperation(&operations, &issues, refsByRole, from, to, netName)
		}
	}
	for _, port := range []struct{ name, role, pin, net string }{{"RAW_OUT", "relay", "3", "raw_out"}, {"SPEAKER_OUT", "relay", "5", "speaker_out"}, {"PROTECT_12V", "relay", "1", "protect_vcc"}, {"POWER_STAR", "relay_driver", "2", "power_star"}} {
		appendConnectOperation(&operations, &issues, request.InstanceID, port.name, refsByRole[port.role], port.pin, InstanceNetName(request.InstanceID, port.net))
	}
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params, output.Instance.Refs, output.Instance.Nets = params, refs, nets
	return output
}

func speakerSenseTransfer(vs, inputR, topR, bottomR float64) (float64, float64) {
	denominator := 1/inputR + 1/topR + 1/bottomR
	return (vs / topR) / denominator, (1 / inputR) / denominator
}

func referenceTopResistance(vs, reference, bottom float64) float64 {
	return bottom * (vs/reference - 1)
}

func speakerPrecisionResistorNominals() []float64 {
	options := components.AudioPrecisionResistorOptions()
	nominals := make([]float64, 0, len(options))
	for _, option := range options {
		if option.TolerancePercent <= 0.1 {
			nominals = append(nominals, option.NominalOhm)
		}
	}
	return nominals
}
func nearestResistance(value float64, candidates []float64) float64 {
	selected := math.NaN()
	selectedDelta := math.Inf(1)
	for _, candidate := range candidates {
		if candidate <= 0 || math.IsNaN(candidate) || math.IsInf(candidate, 0) {
			continue
		}
		delta := math.Abs(candidate - value)
		if delta < selectedDelta || delta == selectedDelta && candidate < selected {
			selected = candidate
			selectedDelta = delta
		}
	}
	return selected
}

func speakerTripToleranceRange(vs, inputR, biasR, referenceTop, referenceBottom, tolerance float64, positive bool) [2]float64 {
	result := [2]float64{math.Inf(1), math.Inf(-1)}
	for mask := 0; mask < 32; mask++ {
		values := []float64{inputR, biasR, biasR, referenceTop, referenceBottom}
		for index := range values {
			factor := 1 - tolerance
			if mask&(1<<index) != 0 {
				factor = 1 + tolerance
			}
			values[index] *= factor
		}
		baseline, slope := speakerSenseTransfer(vs, values[0], values[1], values[2])
		reference := vs * values[4] / (values[3] + values[4])
		trip := (reference - baseline) / slope
		if !positive {
			trip = (baseline - reference) / slope
		}
		result[0], result[1] = math.Min(result[0], trip), math.Max(result[1], trip)
	}
	return result
}

func speakerRelayDelay(resistance, capacitance, threshold, supply float64) float64 {
	if threshold <= 0 || threshold >= supply {
		return math.NaN()
	}
	return -resistance * capacitance * math.Log(1-threshold/supply)
}

func finiteSpeakerValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func speakerComparatorPins() []transactions.PinSpec {
	return []transactions.PinSpec{{Number: "1", XMM: -2.54, YMM: 1.27}, {Number: "2", XMM: 0, YMM: 2.54}, {Number: "3", XMM: -2.54, YMM: -1.27}, {Number: "4", XMM: 2.54, YMM: 0}, {Number: "5", XMM: 0, YMM: -2.54}}
}
func speakerRelayPins() []transactions.PinSpec {
	return []transactions.PinSpec{{Number: "1", XMM: -2.54, YMM: 3.81}, {Number: "2", XMM: -2.54, YMM: -3.81}, {Number: "3", XMM: 2.54, YMM: -1.27}, {Number: "5", XMM: 2.54, YMM: 1.27}}
}

func speakerOutputProtectionPCBRealization() *PCBRealization {
	placements := []PCBComponentRealization{}
	roles := []string{"sense_input", "sense_bias_top", "sense_bias_bottom", "upper_reference_top", "upper_reference_bottom", "lower_reference_top", "lower_reference_bottom", "positive_detector", "negative_detector", "comparator_bypass", "delay_pullup", "delay_capacitor", "delay_zener", "driver_base_resistor", "relay_driver", "relay", "relay_flyback", "supply_loss_diode", "release_resistor"}
	footprintByRole := make(map[string]string, len(roles))
	for _, component := range speakerOutputProtectionComponents(components.ConfidenceVerified, components.AcceptanceFabricationCandidate) {
		footprintByRole[component.Role] = component.FootprintID
	}
	for index, role := range roles {
		placements = append(placements, PCBComponentRealization{ComponentRole: role, FootprintID: footprintByRole[role], Placement: RelativePlacement{XMM: float64((index % 7) * 5), YMM: float64((index / 7) * 7), Layer: "F.Cu"}})
	}
	return &PCBRealization{
		Version: "0.1.0", VerificationLevel: PCBVerificationConnectivityVerified, Components: placements,
		EntryAnchors:    []PCBEntryAnchor{{ID: "raw_out", Port: "RAW_OUT", NetTemplate: "raw_out", Placement: RelativePlacement{XMM: -5, YMM: 0, Layer: "F.Cu"}}, {ID: "speaker_out", Port: "SPEAKER_OUT", NetTemplate: "speaker_out", Placement: RelativePlacement{XMM: 40, YMM: 7, Layer: "F.Cu"}}, {ID: "protect_vcc", Port: "PROTECT_12V", NetTemplate: "protect_vcc", Placement: RelativePlacement{XMM: 10, YMM: -7, Layer: "B.Cu"}}, {ID: "power_star", Port: "POWER_STAR", NetTemplate: "power_star", Placement: RelativePlacement{XMM: 10, YMM: 21, Layer: "B.Cu"}}},
		PlacementGroups: []PCBPlacementGroup{{ID: "dc_detector", ComponentRoles: []string{"sense_input", "sense_bias_top", "sense_bias_bottom", "upper_reference_top", "upper_reference_bottom", "lower_reference_top", "lower_reference_bottom", "positive_detector", "negative_detector", "comparator_bypass"}, AnchorRole: "positive_detector", Bounds: &RelativeBounds{MinXMM: 0, MinYMM: -2, MaxXMM: 31, MaxYMM: 9}}, {ID: "mute_timing", ComponentRoles: []string{"delay_pullup", "delay_capacitor", "delay_zener", "driver_base_resistor", "supply_loss_diode", "release_resistor"}, AnchorRole: "delay_capacitor", Bounds: &RelativeBounds{MinXMM: 12, MinYMM: 5, MaxXMM: 32, MaxYMM: 20}}, {ID: "relay_drive", ComponentRoles: []string{"relay_driver", "relay_flyback", "relay"}, AnchorRole: "relay_driver", Bounds: &RelativeBounds{MinXMM: 0, MinYMM: 12, MaxXMM: 25, MaxYMM: 23}}, {ID: "speaker_relay", ComponentRoles: []string{"relay"}, AnchorRole: "relay", Bounds: &RelativeBounds{MinXMM: 4, MinYMM: 12, MaxXMM: 26, MaxYMM: 24}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "raw_contact", NetTemplate: "raw_out", From: RouteEndpoint{Port: "RAW_OUT"}, To: RouteEndpoint{ComponentRole: "relay", Pin: "3"}, Layer: "F.Cu", WidthMM: 2, Required: true},
			{ID: "raw_sense", NetTemplate: "raw_out", From: RouteEndpoint{Port: "RAW_OUT"}, To: RouteEndpoint{ComponentRole: "sense_input", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.3, Required: true},
			{ID: "speaker_contact", NetTemplate: "speaker_out", From: RouteEndpoint{ComponentRole: "relay", Pin: "5"}, To: RouteEndpoint{Port: "SPEAKER_OUT"}, Layer: "F.Cu", WidthMM: 2, Required: true, DisableEntryAnchorVia: true},
			{ID: "sense_positive", NetTemplate: "sense_node", From: RouteEndpoint{ComponentRole: "sense_input", Pin: "2"}, To: RouteEndpoint{ComponentRole: "positive_detector", Pin: "3"}, Layer: "B.Cu", WidthMM: 0.3, Required: true},
			{ID: "sense_negative", NetTemplate: "sense_node", From: RouteEndpoint{ComponentRole: "sense_input", Pin: "2"}, To: RouteEndpoint{ComponentRole: "negative_detector", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.3, Required: true},
			{ID: "upper_threshold", NetTemplate: "upper_reference", From: RouteEndpoint{ComponentRole: "upper_reference_top", Pin: "2"}, To: RouteEndpoint{ComponentRole: "positive_detector", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "lower_threshold", NetTemplate: "lower_reference", From: RouteEndpoint{ComponentRole: "lower_reference_top", Pin: "2"}, To: RouteEndpoint{ComponentRole: "negative_detector", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "fault_wire_or", NetTemplate: "fault_delay", From: RouteEndpoint{ComponentRole: "positive_detector", Pin: "4"}, To: RouteEndpoint{ComponentRole: "negative_detector", Pin: "4"}, Layer: "F.Cu", WidthMM: 0.35, Required: true},
			{ID: "fault_delay_cap", NetTemplate: "fault_delay", From: RouteEndpoint{ComponentRole: "negative_detector", Pin: "4"}, To: RouteEndpoint{ComponentRole: "delay_capacitor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "delay_driver", NetTemplate: "zener_drive", From: RouteEndpoint{ComponentRole: "delay_zener", Pin: "2"}, To: RouteEndpoint{ComponentRole: "driver_base_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.35, Required: true},
			{ID: "relay_base", NetTemplate: "relay_base", From: RouteEndpoint{ComponentRole: "driver_base_resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "relay_driver", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.35, Required: true},
			{ID: "coil_sink", NetTemplate: "coil_low", From: RouteEndpoint{ComponentRole: "relay", Pin: "2"}, To: RouteEndpoint{ComponentRole: "relay_driver", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.6, Required: true},
			{ID: "coil_clamp", NetTemplate: "coil_low", From: RouteEndpoint{ComponentRole: "relay_driver", Pin: "3"}, To: RouteEndpoint{ComponentRole: "relay_flyback", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.6, Required: true},
			{ID: "supply_loss", NetTemplate: "supply_loss", From: RouteEndpoint{ComponentRole: "supply_loss_diode", Pin: "1"}, To: RouteEndpoint{ComponentRole: "release_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
		},
		Constraints: []PCBConstraint{{ID: "protected_output_width", Kind: "route_width", Category: PCBConstraintCurrentPath, NetTemplate: "speaker_out", MinWidthMM: 2, ClearanceMM: 0.5, Description: "Carry the bounded speaker current through the relay contact."}, {ID: "relay_detector_separation", Kind: "minimum_separation", Category: PCBConstraintAnalogInputSeparation, NetTemplate: "sense_node", AppliesTo: []string{"relay", "relay_driver", "relay_flyback"}, MaxLengthMM: 10, Description: "Separate high-impedance detector nodes from relay switching current."}, {ID: "protection_star_return", Kind: "return_topology", Category: PCBConstraintReturnTopology, NetTemplate: "power_star", AppliesTo: []string{"sense_bias_bottom", "delay_capacitor", "relay_driver"}, Description: "Return detector and relay drive at the declared power star without sharing the speaker-current trace."}},
		Validation:  PCBValidationExpectations{RequiredNets: []string{"protect_vcc", "power_star", "raw_out", "sense_node", "upper_reference", "lower_reference", "fault_delay", "zener_drive", "relay_base", "coil_low", "supply_loss", "speaker_out"}, RequiredRoutes: []string{"raw_contact", "speaker_contact", "sense_positive", "sense_negative", "upper_threshold", "lower_threshold", "fault_wire_or", "fault_delay_cap", "delay_driver", "relay_base", "coil_sink", "coil_clamp", "supply_loss"}, RequiresDRC: true},
	}
}
