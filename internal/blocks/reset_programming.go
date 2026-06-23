package blocks

import (
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func resetProgrammingHeaderDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "reset_programming_header",
		Name:        "Reset And Programming Header",
		Description: "MCU reset pull-up, optional reset switch, and programming header wiring.",
		Version:     "0.1.0",
		Category:    "mcu_support",
		Parameters: []BlockParameter{
			{Name: "programming_interface", Type: ParameterEnum, Default: "isp", Allowed: []any{"isp", "uart"}, Description: "Programming/debug header style to emit."},
			{Name: "include_reset_switch", Type: ParameterBool, Default: true, Description: "When true, add a reset pushbutton from RESET to GND."},
			{Name: "pullup_value", Type: ParameterResistance, Default: "10k", Description: "Reset pull-up resistor value."},
			{Name: "pullup_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Reset pull-up resistor footprint."},
			{Name: "isp_header_footprint", Type: ParameterFootprintID, Default: ispHeaderFootprint, Description: "AVR ISP header footprint."},
			{Name: "uart_header_footprint", Type: ParameterFootprintID, Default: uartHeaderFootprint, Description: "UART header footprint."},
			{Name: "reset_switch_footprint", Type: ParameterFootprintID, Default: resetSwitchFootprint, Description: "Reset switch footprint."},
		},
		Ports: []BlockPort{
			{Name: "VCC", Direction: PortPower, Description: "Target rail for programming header and reset pull-up."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "RESET", Direction: PortPassive, Description: "MCU reset net."},
			{Name: "MOSI", Direction: PortPassive, Description: "ISP MOSI net."},
			{Name: "MISO", Direction: PortPassive, Description: "ISP MISO net."},
			{Name: "SCK", Direction: PortPassive, Description: "ISP SCK net."},
			{Name: "UART_TX", Direction: PortPassive, Description: "Target UART transmit net."},
			{Name: "UART_RX", Direction: PortPassive, Description: "Target UART receive net."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Reset pull-up resistor."},
			{Kind: "symbol", ID: resetSwitchSymbol, Required: false, Description: "Reset pushbutton."},
			{Kind: "symbol", ID: ispHeaderSymbol, Required: false, Description: "AVR ISP header."},
			{Kind: "symbol", ID: uartHeaderSymbol, Required: false, Description: "UART header."},
			{Kind: "footprint", ID: "Resistor_SMD:R_0805_2012Metric", Required: true, Description: "Default reset pull-up footprint."},
			{Kind: "footprint", ID: ispHeaderFootprint, Required: false, Description: "Default AVR ISP footprint."},
			{Kind: "footprint", ID: uartHeaderFootprint, Required: false, Description: "Default UART footprint."},
			{Kind: "footprint", ID: resetSwitchFootprint, Required: false, Description: "Default reset switch footprint."},
		},
		Components:     resetProgrammingComponents(),
		Nets:           resetProgrammingNets(),
		PCBRealization: resetProgrammingPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "reset_programming.interface.supported", Severity: BlockValidationSeverityBlocked, Description: "Programming interface must be isp or uart."},
			{ID: "reset_programming.pullup.valid", Severity: BlockValidationSeverityBlocked, Description: "Reset pull-up value and footprint are required."},
			{ID: "reset_programming.header.present", Severity: BlockValidationSeverityBlocked, Description: "The selected programming header must be emitted."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"builtin_pinmap:" + ispHeaderSymbol,
				"builtin_pinmap:" + uartHeaderSymbol,
				"builtin_pinmap:" + resetSwitchSymbol,
			},
			Notes: []string{"Header pinout follows the AVR ISP and simple VCC/RX/TX/GND UART conventions."},
		},
	}
}

func resetProgrammingComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "reset_pullup", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k"}, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "reset_programming"},
		{Role: "reset_switch", RefPrefix: "SW", Value: "RESET", SymbolID: resetSwitchSymbol, FootprintID: resetSwitchFootprint, Pins: twoTerminalHorizontalPins()},
		{Role: "isp_header", RefPrefix: "J", Value: "AVR ISP", SymbolID: ispHeaderSymbol, FootprintID: ispHeaderFootprint, Pins: twoByThreeHeaderPins()},
		{Role: "uart_header", RefPrefix: "J", Value: "UART", SymbolID: uartHeaderSymbol, FootprintID: uartHeaderFootprint, Pins: connectorSymbolPins(4)},
	}
}

func resetProgrammingNets() []BlockNet {
	return []BlockNet{
		{NameTemplate: "vcc", Visibility: "exported", Role: "power", Pins: []NetPin{{ComponentRole: "reset_pullup", Pin: "1"}, {ComponentRole: "isp_header", Pin: "2"}, {ComponentRole: "uart_header", Pin: "1"}}},
		{NameTemplate: "gnd", Visibility: "exported", Role: "ground", Pins: []NetPin{{ComponentRole: "reset_switch", Pin: "2"}, {ComponentRole: "isp_header", Pin: "6"}, {ComponentRole: "uart_header", Pin: "4"}}},
		{NameTemplate: "reset", Visibility: "exported", Role: "reset", Pins: []NetPin{{ComponentRole: "reset_pullup", Pin: "2"}, {ComponentRole: "reset_switch", Pin: "1"}, {ComponentRole: "isp_header", Pin: "5"}}},
		{NameTemplate: "mosi", Visibility: "exported", Role: "programming", Pins: []NetPin{{ComponentRole: "isp_header", Pin: "4"}}},
		{NameTemplate: "miso", Visibility: "exported", Role: "programming", Pins: []NetPin{{ComponentRole: "isp_header", Pin: "1"}}},
		{NameTemplate: "sck", Visibility: "exported", Role: "programming", Pins: []NetPin{{ComponentRole: "isp_header", Pin: "3"}}},
		{NameTemplate: "uart_rx", Visibility: "exported", Role: "programming", Pins: []NetPin{{ComponentRole: "uart_header", Pin: "2"}}},
		{NameTemplate: "uart_tx", Visibility: "exported", Role: "programming", Pins: []NetPin{{ComponentRole: "uart_header", Pin: "3"}}},
	}
}

func resetProgrammingPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationUnrealized,
		Components: []PCBComponentRealization{
			{ComponentRole: "reset_pullup", FootprintParam: "pullup_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "reset_programming", ComponentRoles: []string{"reset_pullup"}, AnchorRole: "reset_pullup", Bounds: &RelativeBounds{MinXMM: -3, MinYMM: -3, MaxXMM: 3, MaxYMM: 3}, Description: "Keep reset pull-up close to the target MCU reset net."}},
		Constraints: []PCBConstraint{
			{ID: "reset_pullup_proximity", Kind: "proximity", NetTemplate: "reset", AppliesTo: []string{"reset_pullup"}, MaxLengthMM: 20, Description: "Reset pull-up should stay near the target MCU reset net."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vcc", "gnd", "reset"}},
		UnsupportedBehaviors: []string{
			"target-specific SWD/JTAG pinouts are not generated",
			"auto-placement cannot yet force the header to a physical board edge",
			"conditional programming header and reset switch PCB realization awaits conditional realization support",
		},
	}
}

func instantiateResetProgrammingHeader(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if _, ok := parseUnit(params["pullup_value"], "Ω", resistanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.pullup_value", "pullup_value must be a resistance literal"))
	}
	if stringParam(params, "pullup_footprint") == "" {
		issues = append(issues, blockIssue("params.pullup_footprint", "pullup_footprint is required"))
	}
	interfaceMode := stringParam(params, "programming_interface")
	switch interfaceMode {
	case "isp":
		if stringParam(params, "isp_header_footprint") == "" {
			issues = append(issues, blockIssue("params.isp_header_footprint", "isp_header_footprint is required"))
		}
	case "uart":
		if stringParam(params, "uart_header_footprint") == "" {
			issues = append(issues, blockIssue("params.uart_header_footprint", "uart_header_footprint is required"))
		}
	default:
		issues = append(issues, blockIssue("params.programming_interface", "programming_interface must be isp or uart"))
	}
	if boolParam(params, "include_reset_switch", true) && stringParam(params, "reset_switch_footprint") == "" {
		issues = append(issues, blockIssue("params.reset_switch_footprint", "reset_switch_footprint is required when include_reset_switch is true"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	resetPullupRef := allocator.Next("R")
	resetPullup := BlockComponent{Role: "reset_pullup", RefPrefix: "R", Value: stringParam(params, "pullup_value"), SymbolID: "Device:R", FootprintID: stringParam(params, "pullup_footprint"), Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", Package: packageQueryFromFootprint(stringParam(params, "pullup_footprint")), ValueKind: "resistance", Value: normalizeUnitLiteral(stringParam(params, "pullup_value"), "Ω", resistanceMultipliers())}}
	var operations []transactions.Operation
	var refs []string
	pullupOps, pullupIssues := ComponentOperations(resetPullup, resetPullupRef, transactions.Point{XMM: 0, YMM: 0})
	issues = append(issues, pullupIssues...)
	operations = append(operations, pullupOps...)
	refs = append(refs, resetPullupRef)

	vccNet := InstanceNetName(request.InstanceID, "vcc")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	resetNet := InstanceNetName(request.InstanceID, "reset")
	appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", resetPullupRef, "1", vccNet)
	appendConnectOperation(&operations, &issues, resetPullupRef, "2", request.InstanceID, "RESET", resetNet)

	if boolParam(params, "include_reset_switch", true) {
		switchRef := allocator.Next("SW")
		resetSwitch := BlockComponent{Role: "reset_switch", RefPrefix: "SW", Value: "RESET", SymbolID: resetSwitchSymbol, FootprintID: stringParam(params, "reset_switch_footprint"), Pins: twoTerminalHorizontalPins()}
		switchOps, switchIssues := ComponentOperations(resetSwitch, switchRef, transactions.Point{XMM: -7, YMM: 0})
		issues = append(issues, switchIssues...)
		operations = append(operations, switchOps...)
		appendConnectOperation(&operations, &issues, switchRef, "1", request.InstanceID, "RESET", resetNet)
		appendConnectOperation(&operations, &issues, switchRef, "2", request.InstanceID, "GND", gndNet)
		refs = append(refs, switchRef)
	}

	nets := []string{vccNet, gndNet, resetNet}
	switch interfaceMode {
	case "isp":
		headerRef := allocator.Next("J")
		header := BlockComponent{Role: "isp_header", RefPrefix: "J", Value: "AVR ISP", SymbolID: ispHeaderSymbol, FootprintID: stringParam(params, "isp_header_footprint"), Pins: twoByThreeHeaderPins()}
		headerOps, headerIssues := ComponentOperations(header, headerRef, transactions.Point{XMM: 12, YMM: 0})
		issues = append(issues, headerIssues...)
		operations = append(operations, headerOps...)
		misoNet := InstanceNetName(request.InstanceID, "miso")
		mosiNet := InstanceNetName(request.InstanceID, "mosi")
		sckNet := InstanceNetName(request.InstanceID, "sck")
		appendConnectOperation(&operations, &issues, headerRef, "1", request.InstanceID, "MISO", misoNet)
		appendConnectOperation(&operations, &issues, headerRef, "2", request.InstanceID, "VCC", vccNet)
		appendConnectOperation(&operations, &issues, headerRef, "3", request.InstanceID, "SCK", sckNet)
		appendConnectOperation(&operations, &issues, headerRef, "4", request.InstanceID, "MOSI", mosiNet)
		appendConnectOperation(&operations, &issues, headerRef, "5", request.InstanceID, "RESET", resetNet)
		appendConnectOperation(&operations, &issues, headerRef, "6", request.InstanceID, "GND", gndNet)
		refs = append(refs, headerRef)
		nets = append(nets, misoNet, mosiNet, sckNet)
	case "uart":
		headerRef := allocator.Next("J")
		header := BlockComponent{Role: "uart_header", RefPrefix: "J", Value: "UART", SymbolID: uartHeaderSymbol, FootprintID: stringParam(params, "uart_header_footprint"), Pins: connectorSymbolPins(4)}
		headerOps, headerIssues := ComponentOperations(header, headerRef, transactions.Point{XMM: 12, YMM: -8})
		issues = append(issues, headerIssues...)
		operations = append(operations, headerOps...)
		uartRXNet := InstanceNetName(request.InstanceID, "uart_rx")
		uartTXNet := InstanceNetName(request.InstanceID, "uart_tx")
		appendConnectOperation(&operations, &issues, headerRef, "1", request.InstanceID, "VCC", vccNet)
		appendConnectOperation(&operations, &issues, headerRef, "2", request.InstanceID, "UART_RX", uartRXNet)
		appendConnectOperation(&operations, &issues, headerRef, "3", request.InstanceID, "UART_TX", uartTXNet)
		appendConnectOperation(&operations, &issues, headerRef, "4", request.InstanceID, "GND", gndNet)
		refs = append(refs, headerRef)
		nets = append(nets, uartRXNet, uartTXNet)
	}

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}
