package blocks

import (
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const headphoneOutputConnectorID = "headphone_output_connector"

func headphoneOutputConnectorDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          headphoneOutputConnectorID,
		Name:        "Headphone Output Connector",
		Description: "Board-edge headphone connector interface for AC-coupled headphone outputs.",
		Version:     "0.1.0",
		Category:    "interconnect",
		Parameters: []BlockParameter{
			{Name: "connector_mode", Type: ParameterEnum, Default: "mono_trs", Allowed: []any{"mono_trs"}, Description: "Connector wiring mode. Stereo and balanced modes are future work."},
			{Name: "connector_symbol", Type: ParameterSymbolID, Default: "Connector_Generic:Conn_01x03", Description: "KiCad symbol for the connector."},
			{Name: "connector_footprint", Type: ParameterFootprintID, Default: "Connector_PinHeader_2.54mm:PinHeader_1x03_P2.54mm_Vertical", Description: "Footprint for the connector."},
			{Name: "load_kind", Type: ParameterEnum, Default: "headphone", Allowed: []any{"headphone"}, Description: "Only headphone connectors are implemented; speaker connectors are a separate unsupported family."},
		},
		Ports: []BlockPort{
			{Name: "HP_OUT", Direction: PortInput, Description: "AC-coupled headphone signal."},
			{Name: "LOAD_RET", Direction: PortPassive, Description: "Headphone return/sleeve connection."},
			{Name: "LOAD_REF", Direction: PortPassive, Description: "Load reference or shield anchor."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Connector_Generic:Conn_01x03", Required: true, Description: "Default three-pin connector symbol."},
			{Kind: "footprint", ID: "Connector_PinHeader_2.54mm:PinHeader_1x03_P2.54mm_Vertical", Required: true, Description: "Default reviewable connector footprint."},
		},
		Components: []BlockComponent{
			{Role: "headphone_connector", RefPrefix: "J", Value: "HEADPHONE", SymbolID: "Connector_Generic:Conn_01x03", FootprintID: "Connector_PinHeader_2.54mm:PinHeader_1x03_P2.54mm_Vertical", Pins: []transactions.PinSpec{{Number: "1", XMM: -2.54, YMM: -2.54}, {Number: "2", XMM: -2.54, YMM: 0}, {Number: "3", XMM: -2.54, YMM: 2.54}}, PlacementGroup: "board_edge_connector"},
		},
		Nets: []BlockNet{
			{NameTemplate: "hp_out", Visibility: "exported", Role: "headphone_signal", Pins: []NetPin{{ComponentRole: "headphone_connector", Pin: "1"}}},
			{NameTemplate: "load_ret", Visibility: "exported", Role: "headphone_return", Pins: []NetPin{{ComponentRole: "headphone_connector", Pin: "2"}}},
			{NameTemplate: "load_ref", Visibility: "exported", Role: "headphone_reference", Pins: []NetPin{{ComponentRole: "headphone_connector", Pin: "3"}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "connector", ComponentRole: "headphone_connector", XMM: 80, YMM: 8, Note: "Place the load connector at the right edge of the schematic and board."},
		},
		PCBRealization: &PCBRealization{
			Version:           "0.1.0",
			VerificationLevel: PCBVerificationPlacementVerified,
			Components: []PCBComponentRealization{
				{ComponentRole: "headphone_connector", FootprintParam: "connector_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			},
			EntryAnchors: []PCBEntryAnchor{
				{ID: "hp_out", Port: "HP_OUT", NetTemplate: "hp_out", Placement: RelativePlacement{XMM: -4, YMM: -2.54, Layer: "F.Cu"}, Description: "Headphone signal entry."},
				{ID: "load_ret", Port: "LOAD_RET", NetTemplate: "load_ret", Placement: RelativePlacement{XMM: -4, YMM: 0, Layer: "F.Cu"}, Description: "Headphone return entry."},
				{ID: "load_ref", Port: "LOAD_REF", NetTemplate: "load_ref", Placement: RelativePlacement{XMM: -4, YMM: 2.54, Layer: "F.Cu"}, Description: "Load reference entry."},
			},
			PlacementGroups: []PCBPlacementGroup{{ID: "board_edge_connector", ComponentRoles: []string{"headphone_connector"}, AnchorRole: "headphone_connector", Bounds: &RelativeBounds{MinXMM: -5, MinYMM: -6, MaxXMM: 5, MaxYMM: 6}, Description: "Place the headphone connector on an accessible board edge."}},
			LocalRoutes: []PCBLocalRoute{
				{ID: "hp_out_pin", NetTemplate: "hp_out", From: RouteEndpoint{Port: "HP_OUT"}, To: RouteEndpoint{ComponentRole: "headphone_connector", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
				{ID: "load_ret_pin", NetTemplate: "load_ret", From: RouteEndpoint{Port: "LOAD_RET"}, To: RouteEndpoint{ComponentRole: "headphone_connector", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
				{ID: "load_ref_pin", NetTemplate: "load_ref", From: RouteEndpoint{Port: "LOAD_REF"}, To: RouteEndpoint{ComponentRole: "headphone_connector", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			},
			Constraints: []PCBConstraint{{ID: "headphone_connector_board_edge", Kind: "board_edge", AppliesTo: []string{"headphone_connector"}, Description: "Connector must remain accessible at the board edge."}},
			Validation:  PCBValidationExpectations{RequiredNets: []string{"hp_out", "load_ret", "load_ref"}, RequiredRoutes: []string{"hp_out_pin", "load_ret_pin", "load_ref_pin"}},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "headphone_connector.mode.mono_trs_only", Severity: BlockValidationSeverityBlocked, Description: "Only the mono three-pin headphone connector contract is implemented."},
			{ID: "headphone_connector.load.headphone_only", Severity: BlockValidationSeverityBlocked, Description: "Speaker connectors remain blocked until protection and current evidence exist."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Connector realization is a reviewable mono headphone edge connector; production audio jack footprints and stereo wiring remain future work."},
		},
	}
}

func instantiateHeadphoneOutputConnector(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	params = ApplyParameterDefaults(definition, params)
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}
	if stringParam(params, "load_kind") != "headphone" {
		issues = append(issues, blockIssue("params.load_kind", "speaker connectors are blocked until DC fault protection and current/thermal evidence exist"))
	}
	if stringParam(params, "connector_mode") != "mono_trs" {
		issues = append(issues, blockIssue("params.connector_mode", "only mono_trs headphone connector mode is implemented"))
	}
	symbol := stringParam(params, "connector_symbol")
	footprint := stringParam(params, "connector_footprint")
	if symbol == "" {
		issues = append(issues, blockIssue("params.connector_symbol", "connector_symbol is required"))
	}
	if footprint == "" {
		issues = append(issues, blockIssue("params.connector_footprint", "connector_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}
	component, ok := blockComponentByRole(definition.Components)["headphone_connector"]
	if !ok {
		issues = append(issues, blockIssue("components.headphone_connector", "headphone_output_connector requires a headphone_connector component definition"))
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	component.SymbolID = symbol
	component.FootprintID = footprint
	ref := NewInstanceReferenceAllocator(request.InstanceID).Next("J")
	var operations []transactions.Operation
	componentOps, componentIssues := ComponentOperations(component, ref, transactions.Point{XMM: 80, YMM: 8})
	issues = append(issues, componentIssues...)
	operations = append(operations, componentOps...)
	hpNet := InstanceNetName(request.InstanceID, "hp_out")
	retNet := InstanceNetName(request.InstanceID, "load_ret")
	refNet := InstanceNetName(request.InstanceID, "load_ref")
	appendConnectOperation(&operations, &issues, request.InstanceID, "HP_OUT", ref, "1", hpNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "LOAD_RET", ref, "2", retNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "LOAD_REF", ref, "3", refNet)
	appendLabelOperation(&operations, &issues, hpNet, transactions.Point{XMM: 72, YMM: 5})
	appendLabelOperation(&operations, &issues, retNet, transactions.Point{XMM: 72, YMM: 8})
	appendLabelOperation(&operations, &issues, refNet, transactions.Point{XMM: 72, YMM: 11})
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{ref}
	output.Instance.Nets = []string{hpNet, retNet, refNet}
	return output
}
