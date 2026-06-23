package blocks

import (
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func esdProtectionDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "esd_protection",
		Name:        "ESD Protection",
		Description: "Single-line unidirectional TVS shunt from protected signal to ground.",
		Version:     "0.1.0",
		Category:    "protection",
		Parameters: []BlockParameter{
			{Name: "working_voltage", Type: ParameterVoltage, Default: "5V", Description: "Verified working voltage for the protected line. Only 5 V is currently supported."},
			{Name: "tvs_footprint", Type: ParameterFootprintID, Default: "Diode_SMD:D_SOD-323", Description: "TVS diode footprint."},
		},
		Ports: []BlockPort{
			{Name: "SIGNAL", Direction: PortPassive, Voltage: "working_voltage", Description: "Protected signal or rail."},
			{Name: "GND", Direction: PortPower, Description: "Low-inductance ground return."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:D_TVS", Required: true, Description: "TVS diode symbol."},
			{Kind: "footprint", ID: "Diode_SMD:D_SOD-323", Required: true, Description: "Verified SOD-323 TVS footprint."},
		},
		Components:     esdProtectionComponents(),
		Nets:           esdProtectionNets(),
		PCBRealization: esdProtectionPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "esd.working_voltage.supported", Severity: BlockValidationSeverityBlocked, Description: "Working voltage must match the verified 5 V TVS record."},
			{ID: "esd.ground_path.short", Severity: BlockValidationSeverityBlocked, Description: "TVS ground connection should be short and low inductance."},
		},
		Verification: VerificationRecord{
			Level:    VerificationStructural,
			Evidence: []string{"component:protection.tvs.usb_5v_unidirectional", "builtin_pinmap:Device:D_TVS"},
			Notes:    []string{"Only the generic 5 V TVS seed is currently selectable; exact surge rating, capacitance, and clamping voltage still require part-specific selection before fabrication."},
		},
	}
}

func esdProtectionComponents() []BlockComponent {
	return []BlockComponent{{
		Role:              "tvs",
		RefPrefix:         "D",
		Value:             "5V TVS",
		SymbolID:          "Device:D_TVS",
		FootprintID:       "Diode_SMD:D_SOD-323",
		Pins:              twoTerminalHorizontalPins(),
		ComponentID:       "protection.tvs.usb_5v_unidirectional",
		ComponentVariant:  "sod_323",
		MinimumConfidence: components.ConfidenceVerified,
		Acceptance:        components.AcceptanceConnectivity,
		PinmapRequired:    true,
		PlacementGroup:    "esd_shunt",
	}}
}

func esdProtectionNets() []BlockNet {
	return []BlockNet{
		{NameTemplate: "signal", Visibility: "exported", Role: "protected_signal", Pins: []NetPin{{ComponentRole: "tvs", Pin: "1"}}},
		{NameTemplate: "gnd", Visibility: "exported", Role: "ground", Pins: []NetPin{{ComponentRole: "tvs", Pin: "2"}}},
	}
}

func esdProtectionPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "tvs", FootprintParam: "tvs_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "esd_shunt", ComponentRoles: []string{"tvs"}, AnchorRole: "tvs", Bounds: &RelativeBounds{MinXMM: -2, MinYMM: -2, MaxXMM: 2, MaxYMM: 2}, Description: "Place TVS adjacent to the external connector or exposed trace."}},
		Constraints: []PCBConstraint{
			{ID: "esd_ground_short", Kind: "max_length", NetTemplate: "gnd", AppliesTo: []string{"tvs"}, MaxLengthMM: 3, Description: "Ground return should be short and low inductance."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"signal", "gnd"}},
		UnsupportedBehaviors: []string{
			"surge rating and line capacitance are not selected from signal class yet",
			"route-through connector ordering is advisory until ordered net segments are modeled",
			"external connector entry-point proximity is advisory until entry anchors are modeled",
		},
	}
}

func instantiateESDProtection(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	workingVoltage, ok := parseUnit(params["working_voltage"], "V", voltageMultipliers())
	if !ok {
		issues = append(issues, blockIssue("params.working_voltage", "working_voltage must be a voltage literal"))
	}
	if ok && workingVoltage != 5 {
		issues = append(issues, blockIssue("params.working_voltage", "only the verified 5 V TVS working voltage is currently supported"))
	}
	tvsFootprint := stringParam(params, "tvs_footprint")
	if tvsFootprint == "" {
		issues = append(issues, blockIssue("params.tvs_footprint", "tvs_footprint is required"))
	} else if tvsFootprint != "Diode_SMD:D_SOD-323" {
		issues = append(issues, blockIssue("params.tvs_footprint", "only the verified SOD-323 TVS footprint is currently supported"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	tvsRef := allocator.Next("D")
	tvs := blockComponentByRole(esdProtectionComponents())["tvs"]
	tvs.FootprintID = tvsFootprint
	tvsOps, tvsIssues := ComponentOperations(tvs, tvsRef, transactions.Point{XMM: 0, YMM: 0})
	issues = append(issues, tvsIssues...)
	operations := append([]transactions.Operation(nil), tvsOps...)
	signalNet := InstanceNetName(request.InstanceID, "signal")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	appendConnectOperation(&operations, &issues, request.InstanceID, "SIGNAL", tvsRef, "1", signalNet)
	appendConnectOperation(&operations, &issues, tvsRef, "2", request.InstanceID, "GND", gndNet)

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{tvsRef}
	output.Instance.Nets = []string{signalNet, gndNet}
	return output
}
