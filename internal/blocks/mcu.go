package blocks

import (
	"strconv"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const (
	ispHeaderSymbol      = "Connector_Generic:Conn_02x03_Odd_Even"
	ispHeaderFootprint   = "Connector_PinHeader_2.54mm:PinHeader_2x03_P2.54mm_Vertical"
	uartHeaderSymbol     = "Connector_Generic:Conn_01x04"
	uartHeaderFootprint  = "Connector_PinHeader_2.54mm:PinHeader_1x04_P2.54mm_Vertical"
	resetSwitchSymbol    = "Switch:SW_Push"
	resetSwitchFootprint = "Button_Switch_SMD:SW_SPST_SKQG_WithStem"
	defaultMCUValue      = "ATmega328P-A"
)

type mcuPinRoleMap struct {
	VCC    []string
	GND    []string
	RESET  string
	AREF   string
	MOSI   string
	MISO   string
	SCK    string
	GPIO   string
	UARTTX string
	UARTRX string
}

type mcuTemplate struct {
	Value                string
	Roles                mcuPinRoleMap
	Positions            map[string]transactions.Point
	PinCount             int
	CompatibleFootprints []string
}

var atmega328PAU = mcuPinRoleMap{
	VCC:    []string{"4", "6", "18"},
	GND:    []string{"3", "5", "21"},
	RESET:  "29",
	AREF:   "20",
	MOSI:   "15",
	MISO:   "16",
	SCK:    "17",
	GPIO:   "12",
	UARTTX: "31",
	UARTRX: "30",
}

var supportedMCUTemplates = map[string]mcuTemplate{
	defaultMCUSymbol: {
		Value:     defaultMCUValue,
		Roles:     atmega328PAU,
		Positions: atmega328PinPositions,
		PinCount:  32,
		CompatibleFootprints: []string{
			defaultMCUFootprint,
		},
	},
}

var atmega328PinPositions = map[string]transactions.Point{
	"1":  {XMM: 15.24, YMM: -20.32},
	"2":  {XMM: 15.24, YMM: -22.86},
	"3":  {XMM: 0, YMM: -38.1},
	"4":  {XMM: 0, YMM: 38.1},
	"5":  {XMM: 2.54, YMM: -38.1},
	"6":  {XMM: -2.54, YMM: 38.1},
	"7":  {XMM: 15.24, YMM: 15.24},
	"8":  {XMM: 15.24, YMM: 12.7},
	"9":  {XMM: 15.24, YMM: -25.4},
	"10": {XMM: 15.24, YMM: -27.94},
	"11": {XMM: 15.24, YMM: -30.48},
	"12": {XMM: 15.24, YMM: 30.48},
	"13": {XMM: 15.24, YMM: 27.94},
	"14": {XMM: 15.24, YMM: 25.4},
	"15": {XMM: 15.24, YMM: 22.86},
	"16": {XMM: 15.24, YMM: 20.32},
	"17": {XMM: 15.24, YMM: 17.78},
	"18": {XMM: 2.54, YMM: 38.1},
	"19": {XMM: -15.24, YMM: 25.4},
	"20": {XMM: -15.24, YMM: 30.48},
	"21": {XMM: -2.54, YMM: -38.1},
	"22": {XMM: -15.24, YMM: 22.86},
	"23": {XMM: 15.24, YMM: 7.62},
	"24": {XMM: 15.24, YMM: 5.08},
	"25": {XMM: 15.24, YMM: 2.54},
	"26": {XMM: 15.24, YMM: 0},
	"27": {XMM: 15.24, YMM: -2.54},
	"28": {XMM: 15.24, YMM: -5.08},
	"29": {XMM: 15.24, YMM: -7.62},
	"30": {XMM: 15.24, YMM: -12.7},
	"31": {XMM: 15.24, YMM: -15.24},
	"32": {XMM: 15.24, YMM: -17.78},
}

func instantiateMCUMinimal(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	mcuSymbol := stringParam(params, "mcu_symbol")
	template, supported := supportedMCUTemplates[mcuSymbol]
	if !supported {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.mcu_symbol",
			Message:    "mcu_minimal requires a supported MCU pin-role template and currently supports only ATmega328P-A",
			Suggestion: "use " + defaultMCUSymbol + " until resolver-backed MCU pin-role metadata is available",
		})
	} else {
		issues = append(issues, validateMCUTemplate("params.mcu_symbol", template)...)
	}
	if _, ok := parseUnit(params["supply_voltage"], "V", voltageMultipliers()); !ok {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a voltage literal"))
	}
	mcuFootprint := stringParam(params, "mcu_footprint")
	if mcuFootprint == "" {
		issues = append(issues, blockIssue("params.mcu_footprint", "mcu_footprint is required"))
	} else if supported && !templateSupportsFootprint(template, mcuFootprint) {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.mcu_footprint",
			Message:    "mcu_footprint is not compatible with the selected MCU pin-role template",
			Suggestion: "use " + defaultMCUFootprint + " until additional package pin maps are added",
		})
	}
	decouplingCount, ok := integerParam(params, "decoupling_count")
	if !ok || decouplingCount < 1 || decouplingCount > 16 {
		issues = append(issues, blockIssue("params.decoupling_count", "decoupling_count must be an integer between 1 and 16"))
	}
	if stringParam(params, "clock_mode") != "internal" {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.clock_mode",
			Message:    "mcu_minimal currently supports only internal clock mode",
			Suggestion: "use clock_mode internal until crystal and oscillator support is implemented",
		})
	}
	if _, ok := parseUnit(params["decoupling_value"], "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.decoupling_value", "decoupling_value must be a capacitance literal"))
	}
	if _, ok := parseUnit(params["reset_pullup_value"], "Ω", resistanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.reset_pullup_value", "reset_pullup_value must be a resistance literal"))
	}
	if stringParam(params, "reset_resistor_footprint") == "" {
		issues = append(issues, blockIssue("params.reset_resistor_footprint", "reset_resistor_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	issuesOut := append([]reports.Issue{}, issues...)
	issuesOut = append(issuesOut, reports.Issue{
		Code:       reports.CodeUnsupportedOperation,
		Severity:   reports.SeverityWarning,
		Path:       "params.mcu_symbol",
		Message:    "MCU alternate-function metadata is not resolver-backed; this block uses a fixed ATmega328P-A role map",
		Suggestion: "review application GPIO, clock, and analog-reference requirements before fabrication",
	})
	if stringParam(params, "reset_mode") == "external" {
		issuesOut = append(issuesOut, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityWarning,
			Path:       "params.reset_mode",
			Message:    "external reset mode does not generate a pull-up or switch",
			Suggestion: "provide an external reset circuit so the MCU reset pin is not left floating",
		})
	}
	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	mcuRef := allocator.Next("U")
	mcu := BlockComponent{
		Role:        "mcu",
		RefPrefix:   "U",
		Value:       template.Value,
		SymbolID:    mcuSymbol,
		FootprintID: mcuFootprint,
		Pins:        mcuPins(template),
	}
	var operations []transactions.Operation
	mcuOps, mcuIssues := ComponentOperations(mcu, mcuRef, transactions.Point{XMM: 0, YMM: 0})
	issuesOut = append(issuesOut, mcuIssues...)
	operations = append(operations, mcuOps...)

	vccNet := InstanceNetName(request.InstanceID, "vcc")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	resetNet := InstanceNetName(request.InstanceID, "reset")
	arefNet := InstanceNetName(request.InstanceID, "aref")
	mosiNet := InstanceNetName(request.InstanceID, "mosi")
	misoNet := InstanceNetName(request.InstanceID, "miso")
	sckNet := InstanceNetName(request.InstanceID, "sck")
	gpioNet := InstanceNetName(request.InstanceID, "gpio")
	uartTXNet := InstanceNetName(request.InstanceID, "uart_tx")
	uartRXNet := InstanceNetName(request.InstanceID, "uart_rx")
	programmingMode := stringParam(params, "programming_header")
	roleMap := template.Roles

	for _, pin := range roleMap.VCC {
		appendConnectOperation(&operations, &issuesOut, request.InstanceID, "VCC", mcuRef, pin, vccNet)
	}
	for _, pin := range roleMap.GND {
		appendConnectOperation(&operations, &issuesOut, request.InstanceID, "GND", mcuRef, pin, gndNet)
	}
	appendConnectOperation(&operations, &issuesOut, request.InstanceID, "RESET", mcuRef, roleMap.RESET, resetNet)
	appendConnectOperation(&operations, &issuesOut, request.InstanceID, "AREF", mcuRef, roleMap.AREF, arefNet)
	appendConnectOperation(&operations, &issuesOut, request.InstanceID, "GPIO", mcuRef, roleMap.GPIO, gpioNet)

	refs := []string{mcuRef}
	nets := []string{vccNet, gndNet, resetNet, arefNet, gpioNet}
	if programmingMode == "isp" {
		appendConnectOperation(&operations, &issuesOut, request.InstanceID, "MOSI", mcuRef, roleMap.MOSI, mosiNet)
		appendConnectOperation(&operations, &issuesOut, mcuRef, roleMap.MISO, request.InstanceID, "MISO", misoNet)
		appendConnectOperation(&operations, &issuesOut, request.InstanceID, "SCK", mcuRef, roleMap.SCK, sckNet)
		nets = append(nets, mosiNet, misoNet, sckNet)
	}
	if programmingMode == "uart" {
		appendConnectOperation(&operations, &issuesOut, mcuRef, roleMap.UARTTX, request.InstanceID, "UART_TX", uartTXNet)
		appendConnectOperation(&operations, &issuesOut, request.InstanceID, "UART_RX", mcuRef, roleMap.UARTRX, uartRXNet)
		nets = append(nets, uartTXNet, uartRXNet)
	}
	for i := 0; i < decouplingCount; i++ {
		capRef := allocator.Next("C")
		cap := BlockComponent{Role: "decoupling_capacitor", RefPrefix: "C", Value: stringParam(params, "decoupling_value"), SymbolID: "Device:C", FootprintID: stringParam(params, "capacitor_footprint"), Pins: twoTerminalHorizontalPins()}
		capOps, capIssues := ComponentOperations(cap, capRef, transactions.Point{XMM: -70 + float64(i)*6, YMM: 12})
		issuesOut = append(issuesOut, capIssues...)
		operations = append(operations, capOps...)
		appendConnectOperation(&operations, &issuesOut, capRef, "1", mcuRef, cyclicPin(roleMap.VCC, i), vccNet)
		appendConnectOperation(&operations, &issuesOut, capRef, "2", mcuRef, cyclicPin(roleMap.GND, i), gndNet)
		refs = append(refs, capRef)
	}
	arefCapRef := allocator.Next("C")
	arefCap := BlockComponent{Role: "aref_decoupling_capacitor", RefPrefix: "C", Value: stringParam(params, "decoupling_value"), SymbolID: "Device:C", FootprintID: stringParam(params, "capacitor_footprint"), Pins: twoTerminalHorizontalPins()}
	arefCapOps, arefCapIssues := ComponentOperations(arefCap, arefCapRef, transactions.Point{XMM: -50, YMM: 0})
	issuesOut = append(issuesOut, arefCapIssues...)
	operations = append(operations, arefCapOps...)
	appendConnectOperation(&operations, &issuesOut, arefCapRef, "1", mcuRef, roleMap.AREF, arefNet)
	appendConnectOperation(&operations, &issuesOut, arefCapRef, "2", mcuRef, firstPin(roleMap.GND), gndNet)
	refs = append(refs, arefCapRef)

	if stringParam(params, "reset_mode") != "external" {
		resetPullupRef := allocator.Next("R")
		resetPullup := BlockComponent{Role: "reset_pullup", RefPrefix: "R", Value: stringParam(params, "reset_pullup_value"), SymbolID: "Device:R", FootprintID: stringParam(params, "reset_resistor_footprint"), Pins: twoTerminalHorizontalPins()}
		pullupOps, pullupIssues := ComponentOperations(resetPullup, resetPullupRef, transactions.Point{XMM: -50, YMM: -12})
		issuesOut = append(issuesOut, pullupIssues...)
		operations = append(operations, pullupOps...)
		appendConnectOperation(&operations, &issuesOut, resetPullupRef, "1", mcuRef, firstPin(roleMap.VCC), vccNet)
		appendConnectOperation(&operations, &issuesOut, resetPullupRef, "2", mcuRef, roleMap.RESET, resetNet)
		refs = append(refs, resetPullupRef)
	}
	if stringParam(params, "reset_mode") == "pullup_switch" {
		switchRef := allocator.Next("SW")
		resetSwitch := BlockComponent{Role: "reset_switch", RefPrefix: "SW", Value: "RESET", SymbolID: resetSwitchSymbol, FootprintID: resetSwitchFootprint, Pins: twoTerminalHorizontalPins()}
		switchOps, switchIssues := ComponentOperations(resetSwitch, switchRef, transactions.Point{XMM: -62, YMM: -12})
		issuesOut = append(issuesOut, switchIssues...)
		operations = append(operations, switchOps...)
		appendConnectOperation(&operations, &issuesOut, switchRef, "1", mcuRef, roleMap.RESET, resetNet)
		appendConnectOperation(&operations, &issuesOut, switchRef, "2", mcuRef, firstPin(roleMap.GND), gndNet)
		refs = append(refs, switchRef)
	}

	switch programmingMode {
	case "isp":
		headerRef := allocator.Next("J")
		header := BlockComponent{Role: "isp_header", RefPrefix: "J", Value: "AVR ISP", SymbolID: ispHeaderSymbol, FootprintID: ispHeaderFootprint, Pins: twoByThreeHeaderPins()}
		headerOps, headerIssues := ComponentOperations(header, headerRef, transactions.Point{XMM: 55, YMM: 0})
		issuesOut = append(issuesOut, headerIssues...)
		operations = append(operations, headerOps...)
		appendConnectOperation(&operations, &issuesOut, headerRef, "1", mcuRef, roleMap.MISO, misoNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "2", mcuRef, firstPin(roleMap.VCC), vccNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "3", mcuRef, roleMap.SCK, sckNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "4", mcuRef, roleMap.MOSI, mosiNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "5", mcuRef, roleMap.RESET, resetNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "6", mcuRef, firstPin(roleMap.GND), gndNet)
		refs = append(refs, headerRef)
	case "uart":
		headerRef := allocator.Next("J")
		header := BlockComponent{Role: "uart_header", RefPrefix: "J", Value: "UART", SymbolID: uartHeaderSymbol, FootprintID: uartHeaderFootprint, Pins: connectorSymbolPins(4)}
		headerOps, headerIssues := ComponentOperations(header, headerRef, transactions.Point{XMM: 55, YMM: -15})
		issuesOut = append(issuesOut, headerIssues...)
		operations = append(operations, headerOps...)
		appendConnectOperation(&operations, &issuesOut, headerRef, "1", mcuRef, firstPin(roleMap.VCC), vccNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "2", mcuRef, roleMap.UARTRX, uartRXNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "3", mcuRef, roleMap.UARTTX, uartTXNet)
		appendConnectOperation(&operations, &issuesOut, headerRef, "4", mcuRef, firstPin(roleMap.GND), gndNet)
		refs = append(refs, headerRef)
	}

	output := dryRunBlockOutput(definition, request, operations, issuesOut)
	output.Instance.Params = cloneAnyParams(params)
	output.Instance.Ports = resolvePortVoltages(mcuPorts(definition.Ports, programmingMode), output.Instance.Params)
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func integerParam(params map[string]any, name string) (int, bool) {
	value, ok := numericValue(params[name])
	if !ok || value != float64(int(value)) {
		return 0, false
	}
	return int(value), true
}

func validateMCUTemplate(path string, template mcuTemplate) []reports.Issue {
	var issues []reports.Issue
	roleMap := template.Roles
	if template.Value == "" {
		issues = append(issues, blockIssue(path, "MCU template must define a display value"))
	}
	if template.PinCount <= 0 {
		issues = append(issues, blockIssue(path, "MCU template must define a positive pin count"))
	}
	if len(template.Positions) != template.PinCount {
		issues = append(issues, blockIssue(path, "MCU template must define pin positions for every pin"))
	}
	if len(template.CompatibleFootprints) == 0 {
		issues = append(issues, blockIssue(path, "MCU template must define at least one compatible footprint"))
	}
	for i := 1; i <= template.PinCount; i++ {
		number := strconv.Itoa(i)
		if _, ok := template.Positions[number]; !ok {
			issues = append(issues, blockIssue(path, "MCU template missing pin position for pin "+number))
		}
	}
	if len(roleMap.VCC) == 0 {
		issues = append(issues, blockIssue(path, "MCU role map must define at least one VCC pin"))
	}
	if len(roleMap.GND) == 0 {
		issues = append(issues, blockIssue(path, "MCU role map must define at least one GND pin"))
	}
	for name, pin := range map[string]string{
		"RESET":  roleMap.RESET,
		"AREF":   roleMap.AREF,
		"MOSI":   roleMap.MOSI,
		"MISO":   roleMap.MISO,
		"SCK":    roleMap.SCK,
		"GPIO":   roleMap.GPIO,
		"UARTTX": roleMap.UARTTX,
		"UARTRX": roleMap.UARTRX,
	} {
		if pin == "" {
			issues = append(issues, blockIssue(path, "MCU role map must define "+name+" pin"))
		}
	}
	return issues
}

func templateSupportsFootprint(template mcuTemplate, footprint string) bool {
	for _, candidate := range template.CompatibleFootprints {
		if candidate == footprint {
			return true
		}
	}
	return false
}

func firstPin(pins []string) string {
	return cyclicPin(pins, 0)
}

func cyclicPin(pins []string, index int) string {
	if len(pins) == 0 {
		return ""
	}
	return pins[index%len(pins)]
}

func mcuPins(template mcuTemplate) []transactions.PinSpec {
	pins := make([]transactions.PinSpec, 0, template.PinCount)
	for i := 1; i <= template.PinCount; i++ {
		number := strconv.Itoa(i)
		position := template.Positions[number]
		pins = append(pins, transactions.PinSpec{Number: number, XMM: position.XMM, YMM: position.YMM})
	}
	return pins
}

func twoByThreeHeaderPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: -2.54},
		{Number: "2", XMM: 2.54, YMM: -2.54},
		{Number: "3", XMM: -2.54, YMM: 0},
		{Number: "4", XMM: 2.54, YMM: 0},
		{Number: "5", XMM: -2.54, YMM: 2.54},
		{Number: "6", XMM: 2.54, YMM: 2.54},
	}
}

func mcuPorts(base []BlockPort, programmingMode string) []BlockPort {
	ports := make([]BlockPort, 0, len(base))
	for _, port := range base {
		switch port.Name {
		case "MOSI", "MISO", "SCK":
			if programmingMode != "isp" {
				continue
			}
		case "UART_TX", "UART_RX":
			if programmingMode != "uart" {
				continue
			}
		}
		ports = append(ports, port)
	}
	return ports
}
