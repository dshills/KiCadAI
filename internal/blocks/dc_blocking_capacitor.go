package blocks

import (
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const dcBlockingCouplingCapacitorRole = "coupling_capacitor"

func dcBlockingCapacitorDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "dc_blocking_capacitor",
		Name:        "DC Blocking Capacitor",
		Description: "Series coupling capacitor that blocks DC bias between amplifier stages or loads.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "capacitance", Type: ParameterCapacitance, Default: "220uF", Description: "Coupling capacitance value."},
			{Name: "polarized", Type: ParameterBool, Default: true, Description: "Use a polarized capacitor symbol for high-value electrolytic coupling capacitors."},
			{Name: "reverse_polarity", Type: ParameterBool, Default: false, Description: "By default, IN has the higher DC bias and connects to capacitor anode/pin 1. Set true when OUT has the higher DC bias."},
			{Name: "capacitor_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_1210_3225Metric", Description: "Capacitor footprint ID."},
		},
		Ports: []BlockPort{
			{Name: "IN", Direction: PortPassive, Description: "DC-biased signal input."},
			{Name: "OUT", Direction: PortPassive, Description: "AC-coupled signal output."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Series non-polarized coupling capacitor symbol."},
			{Kind: "symbol", ID: "Device:C_Polarized", Required: true, Description: "Series polarized coupling capacitor symbol."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_1210_3225Metric", Required: true, Description: "Default coupling capacitor footprint."},
		},
		Components: []BlockComponent{{
			Role:           dcBlockingCouplingCapacitorRole,
			RefPrefix:      "C",
			SymbolID:       "Device:C_Polarized",
			FootprintID:    "Capacitor_SMD:C_1210_3225Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "series_coupling",
		}},
		Nets: []BlockNet{
			{NameTemplate: "in", Visibility: "exported", Role: "dc_biased_input", Pins: []NetPin{{ComponentRole: dcBlockingCouplingCapacitorRole, Pin: "1"}}},
			{NameTemplate: "out", Visibility: "exported", Role: "ac_coupled_output", Pins: []NetPin{{ComponentRole: dcBlockingCouplingCapacitorRole, Pin: "2"}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "signal_flow", ComponentRole: dcBlockingCouplingCapacitorRole, XMM: 0, YMM: 0, Note: "Place in series between DC-biased amplifier output and load input."},
		},
		PCBRealization: &PCBRealization{
			Version:           "0.1.0",
			VerificationLevel: PCBVerificationUnrealized,
			Components: []PCBComponentRealization{
				{ComponentRole: dcBlockingCouplingCapacitorRole, FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			},
			EntryAnchors: []PCBEntryAnchor{
				{ID: "input", Port: "IN", NetTemplate: "in", Placement: RelativePlacement{XMM: -4, YMM: 0, Layer: "F.Cu"}, Description: "DC-biased input side of the coupling capacitor."},
				{ID: "output", Port: "OUT", NetTemplate: "out", Placement: RelativePlacement{XMM: 4, YMM: 0, Layer: "F.Cu"}, Description: "AC-coupled output side of the coupling capacitor."},
			},
			PlacementGroups: []PCBPlacementGroup{{ID: "series_coupling", ComponentRoles: []string{dcBlockingCouplingCapacitorRole}, AnchorRole: dcBlockingCouplingCapacitorRole, Bounds: &RelativeBounds{MinXMM: -4, MinYMM: -3, MaxXMM: 4, MaxYMM: 3}, Description: "Place the coupling capacitor in the signal path near the output connector."}},
			Validation:      PCBValidationExpectations{RequiredNets: []string{"in", "out"}},
			UnsupportedBehaviors: []string{
				"capacitance is not yet synthesized from load impedance and cutoff frequency",
				"polarity and voltage-rating selection require a concrete capacitor catalog record",
			},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "dc_block.capacitance.required", Severity: BlockValidationSeverityBlocked, Description: "Coupling capacitance must be provided."},
			{ID: "dc_block.footprint.required", Severity: BlockValidationSeverityBlocked, Description: "Coupling capacitor footprint must be provided."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Structural series capacitor only; value sizing, voltage rating, ESR, leakage, and polarity selection remain future work."},
		},
	}
}

func instantiateDCBlockingCapacitor(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	defaulted := ApplyParameterDefaults(definition, params)
	capacitance := stringParam(defaulted, "capacitance")
	if capacitance == "" {
		issues = append(issues, blockIssue("params.capacitance", "capacitance is required"))
	} else if _, ok := parseUnit(capacitance, "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.capacitance", "capacitance must be a capacitance literal"))
	}
	footprint := stringParam(defaulted, "capacitor_footprint")
	if footprint == "" {
		issues = append(issues, blockIssue("params.capacitor_footprint", "capacitor_footprint is required"))
	} else if !strings.Contains(footprint, ":") {
		issues = append(issues, blockIssue("params.capacitor_footprint", "capacitor_footprint must use Library:Footprint format"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	ref := allocator.Next("C")
	component, ok := blockComponentByRole(definition.Components)[dcBlockingCouplingCapacitorRole]
	if !ok {
		issues = append(issues, blockIssue("component.coupling_capacitor", "coupling capacitor component is required"))
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	component.Value = capacitance
	component.FootprintID = footprint
	if !boolParam(defaulted, "polarized", true) {
		component.SymbolID = "Device:C"
	}
	operations, componentIssues := ComponentOperations(component, ref, transactions.Point{XMM: 0, YMM: 0})
	issues = append(issues, componentIssues...)
	inNet := InstanceNetName(request.InstanceID, "in")
	outNet := InstanceNetName(request.InstanceID, "out")
	inPin, outPin := "1", "2"
	if boolParam(defaulted, "reverse_polarity", false) {
		inPin, outPin = outPin, inPin
	}
	appendConnectOperation(&operations, &issues, request.InstanceID, "IN", ref, inPin, inNet)
	appendConnectOperation(&operations, &issues, ref, outPin, request.InstanceID, "OUT", outNet)
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = defaulted
	output.Instance.Refs = []string{ref}
	output.Instance.Nets = []string{inNet, outNet}
	return output
}
