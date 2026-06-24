package blocks

import (
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const (
	defaultOscillatorSymbol    = "Oscillator:Oscillator"
	defaultOscillatorFootprint = "Oscillator:Oscillator_SMD_7050-4Pin_7.0x5.0mm"
	defaultOscillatorFrequency = "16MHz"
)

func cannedOscillatorDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "canned_oscillator",
		Name:        "Canned Oscillator",
		Description: "Packaged oscillator with local decoupling, enable pull-up, and clock output.",
		Version:     "0.1.0",
		Category:    "timing",
		Parameters: []BlockParameter{
			{Name: "frequency", Type: ParameterFrequency, Default: defaultOscillatorFrequency, Allowed: []any{defaultOscillatorFrequency}, Description: "Supported oscillator frequency. Only the verified 16 MHz seed is currently implemented."},
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "Oscillator supply rail."},
			{Name: "oscillator_symbol", Type: ParameterSymbolID, Default: defaultOscillatorSymbol, Description: "Verified KiCad oscillator symbol ID. Arbitrary pinouts are blocked until resolver-backed pin mapping is available."},
			{Name: "oscillator_footprint", Type: ParameterFootprintID, Default: defaultOscillatorFootprint, Description: "Verified KiCad oscillator footprint ID. Arbitrary packages are blocked until resolver-backed pad mapping is available."},
			{Name: "decoupling_value", Type: ParameterCapacitance, Default: "100nF", Description: "Local oscillator bypass capacitor value."},
			{Name: "decoupling_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0603_1608Metric", Description: "Local bypass capacitor footprint ID."},
			{Name: "enable_pullup_value", Type: ParameterResistance, Default: "10k", Description: "Enable pull-up resistor value."},
			{Name: "enable_pullup_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0603_1608Metric", Description: "Enable pull-up resistor footprint ID."},
		},
		Ports: []BlockPort{
			{Name: "CLK_OUT", Direction: PortOutput, Description: "Clock output from oscillator."},
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Oscillator supply input."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "EN", Direction: PortInput, Description: "Oscillator enable node, pulled up by default."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: defaultOscillatorSymbol, Required: true, Description: "Four-pin oscillator symbol."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Local decoupling capacitor."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Enable pull-up resistor."},
			{Kind: "footprint", ID: defaultOscillatorFootprint, Required: true, Description: "Default four-pin oscillator footprint."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_0603_1608Metric", Required: true, Description: "Default local decoupling capacitor footprint."},
			{Kind: "footprint", ID: "Resistor_SMD:R_0603_1608Metric", Required: true, Description: "Default enable pull-up footprint."},
		},
		Components:     cannedOscillatorComponents(),
		Nets:           cannedOscillatorNets(),
		PCBRealization: cannedOscillatorPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "oscillator.frequency.supported", Severity: BlockValidationSeverityBlocked, Description: "Only verified canned oscillator frequencies may be selected."},
			{ID: "oscillator.decoupling.required", Severity: BlockValidationSeverityBlocked, Description: "A local oscillator decoupling capacitor is required."},
			{ID: "oscillator.enable.handled", Severity: BlockValidationSeverityBlocked, Description: "Oscillator enable must have a defined pull state."},
			{ID: "oscillator.clock_output.required", Severity: BlockValidationSeverityBlocked, Description: "Clock output net must be emitted."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"builtin_pinmap:" + defaultOscillatorSymbol,
			},
			Notes: []string{"Oscillator startup, jitter, and signal integrity are not simulated."},
		},
	}
}

func cannedOscillatorPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "oscillator", FootprintParam: "oscillator_footprint", Placement: cannedOscillatorPlacement("oscillator")},
			{ComponentRole: "decoupling", FootprintParam: "decoupling_footprint", Placement: cannedOscillatorPlacement("decoupling")},
			{ComponentRole: "enable_pullup", FootprintParam: "enable_pullup_footprint", Placement: cannedOscillatorPlacement("enable_pullup")},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "oscillator_core", ComponentRoles: []string{"oscillator", "decoupling", "enable_pullup"}, AnchorRole: "oscillator", Bounds: &RelativeBounds{MinXMM: -7, MinYMM: -6, MaxXMM: 7, MaxYMM: 6}, Description: "Keep oscillator, decoupling, and enable pull-up compact."}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "osc_vcc_decoupling", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "oscillator", Pin: "4"}, To: RouteEndpoint{ComponentRole: "decoupling", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "osc_gnd_decoupling", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "oscillator", Pin: "2"}, To: RouteEndpoint{ComponentRole: "decoupling", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "osc_enable_pullup", NetTemplate: "enable", From: RouteEndpoint{ComponentRole: "oscillator", Pin: "1"}, To: RouteEndpoint{ComponentRole: "enable_pullup", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.2, Required: true},
			{ID: "osc_enable_vcc", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "enable_pullup", Pin: "1"}, To: RouteEndpoint{ComponentRole: "oscillator", Pin: "4"}, Layer: "F.Cu", WidthMM: 0.2, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "oscillator_decoupling_proximity", Kind: "proximity", NetTemplate: "vcc", AppliesTo: []string{"oscillator", "decoupling"}, MaxLengthMM: 6, Description: "Place oscillator decoupling close to the package supply pins."},
			{ID: "oscillator_enable_pullup_proximity", Kind: "proximity", NetTemplate: "enable", AppliesTo: []string{"oscillator", "enable_pullup"}, MaxLengthMM: 8, Description: "Keep the enable pull-up near the oscillator enable pin."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vcc", "gnd", "clk_out", "enable"}, RequiredRoutes: []string{"osc_vcc_decoupling", "osc_gnd_decoupling", "osc_enable_pullup", "osc_enable_vcc"}},
		UnsupportedBehaviors: []string{
			"oscillator startup and jitter are not simulated",
			"clock-output consumer placement is handled by composition rules outside this block",
			"signal-integrity review is still required before fabrication",
		},
	}
}

func cannedOscillatorComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "oscillator", RefPrefix: "Y", Value: defaultOscillatorFrequency, SymbolID: defaultOscillatorSymbol, FootprintID: defaultOscillatorFootprint, Pins: oscillatorFourPinPins(), ComponentID: "oscillator.generic.16mhz.7050_4pin", ComponentVariant: "7050_4pin", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true, PlacementGroup: "oscillator_core"},
		{Role: "decoupling", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0603_1608Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", Package: "0603", ValueKind: "capacitance", Value: "100n"}, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "oscillator_core"},
		{Role: "enable_pullup", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0603_1608Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", Package: "0603", ValueKind: "resistance", Value: "10k"}, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "oscillator_core"},
	}
}

func cannedOscillatorNets() []BlockNet {
	return []BlockNet{
		{NameTemplate: "vcc", Visibility: "exported", Role: "power", Pins: []NetPin{{ComponentRole: "oscillator", Pin: "4"}, {ComponentRole: "decoupling", Pin: "1"}, {ComponentRole: "enable_pullup", Pin: "1"}}},
		{NameTemplate: "gnd", Visibility: "exported", Role: "ground", Pins: []NetPin{{ComponentRole: "oscillator", Pin: "2"}, {ComponentRole: "decoupling", Pin: "2"}}},
		{NameTemplate: "clk_out", Visibility: "exported", Role: "clock", Pins: []NetPin{{ComponentRole: "oscillator", Pin: "3"}}},
		{NameTemplate: "enable", Visibility: "exported", Role: "control", Pins: []NetPin{{ComponentRole: "oscillator", Pin: "1"}, {ComponentRole: "enable_pullup", Pin: "2"}}},
	}
}

func instantiateCannedOscillator(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	frequency := strings.TrimSpace(stringParam(params, "frequency"))
	if frequency != defaultOscillatorFrequency {
		issues = append(issues, blockIssue("params.frequency", "only 16MHz canned_oscillator frequency is currently verified"))
	}
	if _, ok := parseUnit(stringParam(params, "supply_voltage"), "V", voltageMultipliers()); !ok {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a voltage literal"))
	}
	if _, ok := parseUnit(stringParam(params, "decoupling_value"), "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.decoupling_value", "decoupling_value must be a capacitance literal"))
	}
	if _, ok := parseUnit(stringParam(params, "enable_pullup_value"), "Ω", resistanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.enable_pullup_value", "enable_pullup_value must be a resistance literal"))
	}
	for _, field := range []string{"oscillator_symbol", "oscillator_footprint", "decoupling_footprint", "enable_pullup_footprint"} {
		if stringParam(params, field) == "" {
			issues = append(issues, blockIssue("params."+field, field+" is required"))
		}
	}
	if symbol := stringParam(params, "oscillator_symbol"); symbol != "" && symbol != defaultOscillatorSymbol {
		issues = append(issues, blockIssue("params.oscillator_symbol", "only the verified four-pin oscillator symbol is currently supported"))
	}
	if footprint := stringParam(params, "oscillator_footprint"); footprint != "" && footprint != defaultOscillatorFootprint {
		issues = append(issues, blockIssue("params.oscillator_footprint", "only the verified 7050 four-pin oscillator footprint is currently supported"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	oscRef := allocator.Next("Y")
	decouplingRef := allocator.Next("C")
	enableRef := allocator.Next("R")
	decouplingValue := normalizeCapacitanceQueryValue(stringParam(params, "decoupling_value"))
	if decouplingValue == "" {
		decouplingValue = "100n"
	}
	oscillator := BlockComponent{Role: "oscillator", RefPrefix: "Y", Value: frequency, SymbolID: stringParam(params, "oscillator_symbol"), FootprintID: stringParam(params, "oscillator_footprint"), Pins: oscillatorFourPinPins(), ComponentID: cannedOscillatorComponentID(frequency), ComponentVariant: "7050_4pin", PinmapRequired: true}
	decoupling := BlockComponent{Role: "decoupling", RefPrefix: "C", Value: decouplingValue, SymbolID: "Device:C", FootprintID: stringParam(params, "decoupling_footprint"), Pins: twoTerminalHorizontalPins(), ComponentQuery: oscillatorCapacitorQuery(stringParam(params, "decoupling_footprint"), decouplingValue)}
	enablePullup := BlockComponent{Role: "enable_pullup", RefPrefix: "R", Value: stringParam(params, "enable_pullup_value"), SymbolID: "Device:R", FootprintID: stringParam(params, "enable_pullup_footprint"), Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", Package: packageQueryFromFootprint(stringParam(params, "enable_pullup_footprint")), ValueKind: "resistance", Value: normalizeUnitLiteral(stringParam(params, "enable_pullup_value"), "Ω", resistanceMultipliers())}}

	var operations []transactions.Operation
	for _, item := range []struct {
		component BlockComponent
		ref       string
		role      string
	}{
		{oscillator, oscRef, "oscillator"},
		{decoupling, decouplingRef, "decoupling"},
		{enablePullup, enableRef, "enable_pullup"},
	} {
		placement := cannedOscillatorPlacement(item.role)
		componentOps, componentIssues := ComponentOperations(item.component, item.ref, transactions.Point{XMM: placement.XMM, YMM: placement.YMM})
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}

	vccNet := InstanceNetName(request.InstanceID, "vcc")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	clkNet := InstanceNetName(request.InstanceID, "clk_out")
	enableNet := InstanceNetName(request.InstanceID, "enable")
	appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", oscRef, "4", vccNet)
	appendConnectOperation(&operations, &issues, oscRef, "4", decouplingRef, "1", vccNet)
	appendConnectOperation(&operations, &issues, enableRef, "1", oscRef, "4", vccNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "GND", oscRef, "2", gndNet)
	appendConnectOperation(&operations, &issues, oscRef, "2", decouplingRef, "2", gndNet)
	appendConnectOperation(&operations, &issues, oscRef, "3", request.InstanceID, "CLK_OUT", clkNet)
	appendConnectOperation(&operations, &issues, oscRef, "1", enableRef, "2", enableNet)
	appendConnectOperation(&operations, &issues, enableRef, "2", request.InstanceID, "EN", enableNet)

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{oscRef, decouplingRef, enableRef}
	output.Instance.Nets = []string{vccNet, gndNet, clkNet, enableNet}
	return output
}

var cannedOscillatorPlacements = []struct {
	role      string
	placement RelativePlacement
}{
	{role: "oscillator", placement: RelativePlacement{XMM: 0, YMM: 0}},
	{role: "decoupling", placement: RelativePlacement{XMM: 4, YMM: -2}},
	{role: "enable_pullup", placement: RelativePlacement{XMM: 4, YMM: -3}},
}

func cannedOscillatorPlacement(role string) RelativePlacement {
	for _, candidate := range cannedOscillatorPlacements {
		if candidate.role == role {
			return candidate.placement
		}
	}
	return RelativePlacement{}
}

func cannedOscillatorComponentID(frequency string) string {
	return "oscillator.generic." + strings.ToLower(strings.TrimSpace(frequency)) + ".7050_4pin"
}

func oscillatorFourPinPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: -1.8},
		{Number: "2", XMM: -2.54, YMM: 1.8},
		{Number: "3", XMM: 2.54, YMM: 1.8},
		{Number: "4", XMM: 2.54, YMM: -1.8},
	}
}
