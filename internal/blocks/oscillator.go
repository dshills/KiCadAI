package blocks

import (
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

var footprintPackageQueryIDs = []string{"01005", "0201", "0402", "0603", "0805", "1206", "1210", "1812"}

var capacitanceQuerySuffixes = []struct {
	suffix string
	unit   string
}{
	{"pf", "p"},
	{"nf", "n"},
	{"uf", "u"},
	{"µf", "u"},
	{"μf", "u"},
	{"mf", "m"},
}

func crystalOscillatorDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "crystal_oscillator",
		Name:        "Crystal Oscillator",
		Description: "Two-pin crystal with load capacitors for MCU clock inputs.",
		Version:     "0.1.0",
		Category:    "timing",
		Parameters: []BlockParameter{
			{Name: "frequency", Type: ParameterFrequency, Default: "16MHz", Allowed: []any{"16MHz"}, Description: "Supported crystal frequency. Only the verified 16 MHz ABM3 seed is currently implemented."},
			{Name: "load_capacitance", Type: ParameterCapacitance, Default: "18pF", Description: "Crystal load capacitance evidence from the selected crystal record."},
			{Name: "load_capacitor_value", Type: ParameterCapacitance, Default: "22pF", Description: "Nominal load capacitor value after board stray capacitance estimate."},
			{Name: "crystal_footprint", Type: ParameterFootprintID, Default: "Crystal:Crystal_SMD_5032-2Pin_5.0x3.2mm", Description: "Crystal footprint ID."},
			{Name: "capacitor_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0603_1608Metric", Description: "Load capacitor footprint ID."},
		},
		Ports: []BlockPort{
			{Name: "XTAL1", Direction: PortPassive, Description: "MCU oscillator input pin."},
			{Name: "XTAL2", Direction: PortPassive, Description: "MCU oscillator output pin."},
			{Name: "GND", Direction: PortPower, Description: "Ground return for load capacitors."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:Crystal", Required: true, Description: "Two-pin crystal symbol."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Load capacitors."},
			{Kind: "footprint", ID: "Crystal:Crystal_SMD_5032-2Pin_5.0x3.2mm", Required: true, Description: "Verified 5032 2-pin crystal footprint."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_0603_1608Metric", Required: true, Description: "Default load capacitor footprint."},
		},
		Components:     crystalOscillatorComponents(),
		Nets:           crystalOscillatorNets(),
		PCBRealization: crystalOscillatorPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "oscillator.frequency.supported", Severity: BlockValidationSeverityBlocked, Description: "Only verified crystal frequencies may be selected."},
			{ID: "oscillator.load_capacitors.required", Severity: BlockValidationSeverityBlocked, Description: "Two load capacitors are required."},
			{ID: "oscillator.pinmap.required", Severity: BlockValidationSeverityBlocked, Description: "Crystal symbol-footprint pinmap evidence is required."},
			{ID: "oscillator.local_route.required", Severity: BlockValidationSeverityBlocked, Description: "Crystal loop local route evidence is required."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"component:crystal.abracon.abm3_16mhz.5032_2pin",
				"builtin_pinmap:Device:Crystal",
			},
			Notes: []string{"Oscillator startup and final layout quality are not simulated."},
		},
	}
}

func crystalOscillatorComponents() []BlockComponent {
	return []BlockComponent{
		{
			Role:              "crystal",
			RefPrefix:         "Y",
			Value:             "16MHz",
			SymbolID:          "Device:Crystal",
			FootprintID:       "Crystal:Crystal_SMD_5032-2Pin_5.0x3.2mm",
			Pins:              twoTerminalHorizontalPins(),
			ComponentID:       "crystal.abracon.abm3_16mhz.5032_2pin",
			ComponentVariant:  "5032_2pin",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
			PinmapRequired:    true,
			PlacementGroup:    "oscillator_core",
		},
		{
			Role:              "load_capacitor_1",
			RefPrefix:         "C",
			Value:             "22pF",
			SymbolID:          "Device:C",
			FootprintID:       "Capacitor_SMD:C_0603_1608Metric",
			Pins:              twoTerminalHorizontalPins(),
			ComponentQuery:    &components.Query{Family: "capacitor", Package: "0603", ValueKind: "capacitance", Value: "22p"},
			MinimumConfidence: components.ConfidenceRuleInferred,
			Acceptance:        components.AcceptanceConnectivity,
			PlacementGroup:    "oscillator_core",
		},
		{
			Role:              "load_capacitor_2",
			RefPrefix:         "C",
			Value:             "22pF",
			SymbolID:          "Device:C",
			FootprintID:       "Capacitor_SMD:C_0603_1608Metric",
			Pins:              twoTerminalHorizontalPins(),
			ComponentQuery:    &components.Query{Family: "capacitor", Package: "0603", ValueKind: "capacitance", Value: "22p"},
			MinimumConfidence: components.ConfidenceRuleInferred,
			Acceptance:        components.AcceptanceConnectivity,
			PlacementGroup:    "oscillator_core",
		},
	}
}

func crystalOscillatorNets() []BlockNet {
	return []BlockNet{
		{NameTemplate: "xtal1", Visibility: "exported", Role: "clock", Pins: []NetPin{{ComponentRole: "crystal", Pin: "1"}, {ComponentRole: "load_capacitor_1", Pin: "1"}}},
		{NameTemplate: "xtal2", Visibility: "exported", Role: "clock", Pins: []NetPin{{ComponentRole: "crystal", Pin: "2"}, {ComponentRole: "load_capacitor_2", Pin: "1"}}},
		{NameTemplate: "gnd", Visibility: "exported", Role: "ground", Pins: []NetPin{{ComponentRole: "load_capacitor_1", Pin: "2"}, {ComponentRole: "load_capacitor_2", Pin: "2"}}},
	}
}

func crystalOscillatorPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "crystal", FootprintParam: "crystal_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "load_capacitor_1", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -4, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "load_capacitor_2", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: 4, YMM: 3, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "oscillator_core", ComponentRoles: []string{"crystal", "load_capacitor_1", "load_capacitor_2"}, AnchorRole: "crystal", Bounds: &RelativeBounds{MinXMM: -7, MinYMM: -3, MaxXMM: 7, MaxYMM: 7}, Description: "Keep crystal loop compact and near MCU oscillator pins."}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "xtal1_load", NetTemplate: "xtal1", From: RouteEndpoint{ComponentRole: "crystal", Pin: "1"}, To: RouteEndpoint{ComponentRole: "load_capacitor_1", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.2, Required: true},
			{ID: "xtal2_load", NetTemplate: "xtal2", From: RouteEndpoint{ComponentRole: "crystal", Pin: "2"}, To: RouteEndpoint{ComponentRole: "load_capacitor_2", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.2, Required: true},
			{ID: "load_caps_ground", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "load_capacitor_1", Pin: "2"}, To: RouteEndpoint{ComponentRole: "load_capacitor_2", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "oscillator_mcu_proximity", Kind: "proximity", NetTemplate: "xtal1", AppliesTo: []string{"crystal"}, MaxLengthMM: 10, Description: "Place the oscillator close to MCU oscillator pins."},
			{ID: "oscillator_loop_short", Kind: "max_length", NetTemplate: "xtal1", AppliesTo: []string{"crystal", "load_capacitor_1", "load_capacitor_2"}, MaxLengthMM: 12, Description: "Keep the local crystal loop short."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"xtal1", "xtal2", "gnd"}, RequiredRoutes: []string{"xtal1_load", "xtal2_load", "load_caps_ground"}},
		UnsupportedBehaviors: []string{
			"oscillator startup margin is not simulated",
			"shield/guard-ring layout is not generated",
		},
	}
}

func instantiateCrystalOscillator(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	frequency := strings.TrimSpace(stringParam(params, "frequency"))
	if frequency != "16MHz" {
		issues = append(issues, blockIssue("params.frequency", "only 16MHz crystal_oscillator frequency is currently verified"))
	}
	if _, ok := parseUnit(params["load_capacitance"], "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.load_capacitance", "load_capacitance must be a capacitance literal"))
	}
	if _, ok := parseUnit(params["load_capacitor_value"], "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.load_capacitor_value", "load_capacitor_value must be a capacitance literal"))
	}
	crystalFootprint := stringParam(params, "crystal_footprint")
	capacitorFootprint := stringParam(params, "capacitor_footprint")
	if crystalFootprint == "" {
		issues = append(issues, blockIssue("params.crystal_footprint", "crystal_footprint is required"))
	} else if crystalFootprint != "Crystal:Crystal_SMD_5032-2Pin_5.0x3.2mm" {
		issues = append(issues, blockIssue("params.crystal_footprint", "only the verified 5032 2-pin crystal footprint is currently supported"))
	}
	if capacitorFootprint == "" {
		issues = append(issues, blockIssue("params.capacitor_footprint", "capacitor_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	crystalRef := allocator.Next("Y")
	c1Ref := allocator.Next("C")
	c2Ref := allocator.Next("C")
	componentsByRole := blockComponentByRole(crystalOscillatorComponents())
	crystal := componentsByRole["crystal"]
	crystal.FootprintID = crystalFootprint
	c1 := componentsByRole["load_capacitor_1"]
	capValue := normalizeCapacitanceQueryValue(params["load_capacitor_value"])
	if capValue == "" {
		capValue = stringParam(params, "load_capacitor_value")
	}
	c1.Value = capValue
	c1.FootprintID = capacitorFootprint
	c1.ComponentQuery = oscillatorCapacitorQuery(capacitorFootprint, capValue)
	c2 := componentsByRole["load_capacitor_2"]
	c2.Value = capValue
	c2.FootprintID = capacitorFootprint
	c2.ComponentQuery = oscillatorCapacitorQuery(capacitorFootprint, capValue)
	var operations []transactions.Operation
	for _, item := range []struct {
		component BlockComponent
		ref       string
		at        transactions.Point
	}{
		{crystal, crystalRef, transactions.Point{XMM: 0, YMM: 0}},
		{c1, c1Ref, transactions.Point{XMM: -4, YMM: 3}},
		{c2, c2Ref, transactions.Point{XMM: 4, YMM: 3}},
	} {
		componentOps, componentIssues := ComponentOperations(item.component, item.ref, item.at)
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}
	xtal1Net := InstanceNetName(request.InstanceID, "xtal1")
	xtal2Net := InstanceNetName(request.InstanceID, "xtal2")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	appendConnectOperation(&operations, &issues, request.InstanceID, "XTAL1", crystalRef, "1", xtal1Net)
	appendConnectOperation(&operations, &issues, crystalRef, "1", c1Ref, "1", xtal1Net)
	appendConnectOperation(&operations, &issues, request.InstanceID, "XTAL2", crystalRef, "2", xtal2Net)
	appendConnectOperation(&operations, &issues, crystalRef, "2", c2Ref, "1", xtal2Net)
	appendConnectOperation(&operations, &issues, c1Ref, "2", request.InstanceID, "GND", gndNet)
	appendConnectOperation(&operations, &issues, c2Ref, "2", request.InstanceID, "GND", gndNet)
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{crystalRef, c1Ref, c2Ref}
	output.Instance.Nets = []string{xtal1Net, xtal2Net, gndNet}
	return output
}

func oscillatorCapacitorQuery(footprint string, value string) *components.Query {
	return &components.Query{Family: "capacitor", Package: packageQueryFromFootprint(footprint), ValueKind: "capacitance", Value: value}
}

func normalizeCapacitanceQueryValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.ReplaceAll(normalizeUnitLiteral(text, "F", capacitanceMultipliers()), " ", "")
	lowerText := strings.ToLower(text)
	for _, replacement := range capacitanceQuerySuffixes {
		if strings.HasSuffix(lowerText, replacement.suffix) {
			return text[:len(text)-len(replacement.suffix)] + replacement.unit
		}
	}
	return text
}

func packageQueryFromFootprint(footprint string) string {
	footprint = strings.ToLower(footprint)
	for _, packageID := range footprintPackageQueryIDs {
		if footprintContainsPackageID(footprint, packageID) {
			return packageID
		}
	}
	if _, after, ok := strings.Cut(footprint, ":"); ok {
		return after
	}
	return footprint
}

func footprintContainsPackageID(footprint string, packageID string) bool {
	index := strings.Index(footprint, packageID)
	for index >= 0 {
		beforeOK := index == 0 || !isPackageIDChar(footprint[index-1])
		after := index + len(packageID)
		afterOK := after == len(footprint) || !isPackageIDChar(footprint[after])
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(footprint[index+1:], packageID)
		if next < 0 {
			return false
		}
		index += next + 1
	}
	return false
}

func isPackageIDChar(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}
