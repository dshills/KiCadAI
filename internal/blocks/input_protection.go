package blocks

import (
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func reversePolarityProtectionDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "reverse_polarity_protection",
		Name:        "Reverse-Polarity Protection",
		Description: "Series Schottky diode input protection for low-current positive supplies.",
		Version:     "0.1.0",
		Category:    "protection",
		Parameters: []BlockParameter{
			{Name: "input_voltage", Type: ParameterVoltage, Default: "5V", Description: "Nominal positive input voltage."},
			{Name: "input_current", Type: ParameterCurrent, Default: "500mA", Description: "Expected protected load current. The current seed is limited to 1 A."},
			{Name: "diode_footprint", Type: ParameterFootprintID, Default: "Diode_SMD:D_SMA", Description: "Series Schottky diode footprint."},
		},
		Ports: []BlockPort{
			{Name: "VIN_RAW", Direction: PortPower, Voltage: "input_voltage", Description: "Unprotected positive input."},
			{Name: "VIN_PROTECTED", Direction: PortPower, Voltage: "input_voltage", Description: "Protected positive output after the diode drop."},
			{Name: "GND", Direction: PortPower, Description: "Ground reference; not switched by this series-diode topology."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:D_Schottky", Required: true, Description: "Series Schottky diode."},
			{Kind: "footprint", ID: "Diode_SMD:D_SMA", Required: true, Description: "Verified SMA diode footprint."},
		},
		Components:     reversePolarityProtectionComponents(),
		Nets:           reversePolarityProtectionNets(),
		PCBRealization: reversePolarityProtectionPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "reverse_polarity.positive_input", Severity: BlockValidationSeverityBlocked, Description: "Input voltage must be positive for a unidirectional series diode."},
			{ID: "reverse_polarity.current_rating", Severity: BlockValidationSeverityBlocked, Description: "Input current must not exceed the verified generic Schottky record."},
			{ID: "reverse_polarity.footprint.supported", Severity: BlockValidationSeverityBlocked, Description: "Only the verified SMA Schottky footprint is currently supported."},
		},
		Verification: VerificationRecord{
			Level:    VerificationStructural,
			Evidence: []string{"component:diode.schottky.generic", "builtin_pinmap:Device:D_Schottky"},
			Notes:    []string{"This topology drops voltage and dissipates power; ideal-diode MOSFET controllers are not generated yet."},
		},
	}
}

func reversePolarityProtectionComponents() []BlockComponent {
	return []BlockComponent{{
		Role:              "series_diode",
		RefPrefix:         "D",
		Value:             "Schottky",
		SymbolID:          "Device:D_Schottky",
		FootprintID:       "Diode_SMD:D_SMA",
		Pins:              twoTerminalHorizontalPins(),
		ComponentID:       "diode.schottky.generic",
		ComponentVariant:  "sma",
		MinimumConfidence: components.ConfidenceVerified,
		Acceptance:        components.AcceptanceConnectivity,
		PinmapRequired:    true,
		PlacementGroup:    "input_protection",
	}}
}

func reversePolarityProtectionNets() []BlockNet {
	return []BlockNet{
		{NameTemplate: "vin_raw", Visibility: "exported", Role: "raw_input", Pins: []NetPin{{ComponentRole: "series_diode", Pin: "2"}}},
		{NameTemplate: "vin_protected", Visibility: "exported", Role: "protected_input", Pins: []NetPin{{ComponentRole: "series_diode", Pin: "1"}}},
		{NameTemplate: "gnd", Visibility: "exported", Role: "ground"},
	}
}

func reversePolarityProtectionPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "series_diode", FootprintParam: "diode_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "input_protection", ComponentRoles: []string{"series_diode"}, AnchorRole: "series_diode", Bounds: &RelativeBounds{MinXMM: -4, MinYMM: -3, MaxXMM: 4, MaxYMM: 3}, Description: "Place series diode close to the raw input connector."}},
		Constraints: []PCBConstraint{
			{ID: "input_diode_current_width", Kind: "min_width", AppliesTo: []string{"series_diode"}, MinWidthMM: 0.6, Description: "Protected input current path should use a wider trace than signal routing."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vin_raw", "vin_protected", "gnd"}},
		UnsupportedBehaviors: []string{
			"raw connector entry anchors are advisory until entry-point roles are modeled",
			"thermal dissipation and forward-voltage budget are not calculated",
			"ideal-diode MOSFET reverse-polarity topologies are not generated",
		},
	}
}

func instantiateReversePolarityProtection(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	inputVoltage, voltageOK := parseUnit(params["input_voltage"], "V", voltageMultipliers())
	if !voltageOK {
		issues = append(issues, blockIssue("params.input_voltage", "input_voltage must be a voltage literal"))
	}
	if voltageOK && inputVoltage <= 0 {
		issues = append(issues, blockIssue("params.input_voltage", "input_voltage must be positive"))
	}
	inputCurrent, currentOK := parseUnit(params["input_current"], "A", currentMultipliers())
	if !currentOK {
		issues = append(issues, blockIssue("params.input_current", "input_current must be a current literal"))
	}
	if currentOK && (inputCurrent <= 0 || inputCurrent > 1) {
		issues = append(issues, blockIssue("params.input_current", "input_current must be positive and no more than 1A for the verified generic Schottky"))
	}
	diodeFootprint := stringParam(params, "diode_footprint")
	if diodeFootprint == "" {
		issues = append(issues, blockIssue("params.diode_footprint", "diode_footprint is required"))
	} else if diodeFootprint != "Diode_SMD:D_SMA" {
		issues = append(issues, blockIssue("params.diode_footprint", "only the verified SMA Schottky footprint is currently supported"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	diode := blockComponentByRole(reversePolarityProtectionComponents())["series_diode"]
	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	diodeRef := allocator.Next(diode.RefPrefix)
	diode.FootprintID = diodeFootprint
	diodeOps, diodeIssues := ComponentOperations(diode, diodeRef, transactions.Point{XMM: 0, YMM: 0})
	issues = append(issues, diodeIssues...)
	operations := make([]transactions.Operation, 0, len(diodeOps)+3)
	operations = append(operations, diodeOps...)
	rawNet := InstanceNetName(request.InstanceID, "vin_raw")
	protectedNet := InstanceNetName(request.InstanceID, "vin_protected")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	appendConnectOperation(&operations, &issues, request.InstanceID, "VIN_RAW", diodeRef, "2", rawNet)
	appendConnectOperation(&operations, &issues, diodeRef, "1", request.InstanceID, "VIN_PROTECTED", protectedNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "GND", request.InstanceID, "GND", gndNet)

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{diodeRef}
	output.Instance.Nets = []string{rawNet, protectedNet, gndNet}
	return output
}
