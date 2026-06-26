package blocks

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func instantiateLEDIndicator(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	supplyVoltage, supplyOK := parseUnit(params["supply_voltage"], "V", voltageMultipliers())
	if !supplyOK {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a voltage literal"))
	}
	if supplyOK && supplyVoltage <= 0 {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be positive"))
	}
	forwardVoltage, forwardOK := parseUnit(params["led_forward_voltage"], "V", voltageMultipliers())
	if !forwardOK {
		issues = append(issues, blockIssue("params.led_forward_voltage", "led_forward_voltage must be a voltage literal"))
	}
	ledCurrent, currentOK := parseUnit(params["led_current"], "A", currentMultipliers())
	if !currentOK {
		issues = append(issues, blockIssue("params.led_current", "led_current must be a current literal"))
	}
	if currentOK && ledCurrent <= 0 {
		issues = append(issues, blockIssue("params.led_current", "led_current must be positive"))
	}
	if supplyOK && forwardOK && supplyVoltage <= forwardVoltage {
		issues = append(issues, blockIssue("params.led_forward_voltage", "led_forward_voltage must be below supply_voltage"))
	}
	resistorOhms, resistorProvided := parseUnit(params["resistor_value"], "Ω", resistanceMultipliers())
	resistorPresent := params["resistor_value"] != nil
	if resistorPresent && !resistorProvided {
		issues = append(issues, blockIssue("params.resistor_value", "resistor_value must be a resistance literal"))
	}
	if !resistorProvided && currentOK && supplyOK && forwardOK && ledCurrent > 0 && supplyVoltage > forwardVoltage {
		resistorOhms = (supplyVoltage - forwardVoltage) / ledCurrent
		resistorProvided = true
		params["resistor_value"] = formatOhms(resistorOhms)
	}
	if resistorProvided && resistorOhms <= 0 {
		issues = append(issues, blockIssue("params.resistor_value", "resistor_value must be positive"))
	}
	resistorFootprint := stringParam(params, "resistor_footprint")
	if resistorFootprint == "" {
		issues = append(issues, blockIssue("params.resistor_footprint", "resistor_footprint is required"))
	}
	ledFootprint := stringParam(params, "led_footprint")
	if ledFootprint == "" {
		issues = append(issues, blockIssue("params.led_footprint", "led_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	resistorRef := allocator.Next("R")
	ledRef := allocator.Next("D")
	resistor := BlockComponent{
		Role:        "resistor",
		RefPrefix:   "R",
		Value:       formatOhms(resistorOhms),
		SymbolID:    "Device:R",
		FootprintID: resistorFootprint,
		Pins:        twoTerminalHorizontalPins(),
	}
	led := BlockComponent{
		Role:        "led",
		RefPrefix:   "D",
		Value:       strings.ToUpper(stringParam(params, "color")) + " LED",
		SymbolID:    "Device:LED",
		FootprintID: ledFootprint,
		Pins:        twoTerminalHorizontalPins(),
	}
	var operations []transactions.Operation
	resistorOps, componentIssues := ComponentOperations(resistor, resistorRef, transactions.Point{XMM: 0, YMM: 0})
	issues = append(issues, componentIssues...)
	operations = append(operations, resistorOps...)
	ledOps, componentIssues := ComponentOperations(led, ledRef, transactions.Point{XMM: 12.7, YMM: 0})
	issues = append(issues, componentIssues...)
	operations = append(operations, ledOps...)
	seriesNet := InstanceNetName(request.InstanceID, "led_series")
	inputNet := InstanceNetName(request.InstanceID, "in")
	vccNet := InstanceNetName(request.InstanceID, "vcc")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	activeHigh := boolParam(params, "active_high", true)
	if activeHigh {
		appendConnectOperation(&operations, &issues, request.InstanceID, "IN", resistorRef, "1", inputNet)
	} else {
		appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", resistorRef, "1", vccNet)
	}
	connect, connectIssues := ConnectOperation(resistorRef, "2", ledRef, "1", seriesNet)
	issues = append(issues, connectIssues...)
	if len(connectIssues) == 0 {
		operations = append(operations, connect)
	}
	if activeHigh {
		appendConnectOperation(&operations, &issues, ledRef, "2", request.InstanceID, "GND", gndNet)
	} else {
		appendConnectOperation(&operations, &issues, ledRef, "2", request.InstanceID, "IN", inputNet)
	}
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = []string{resistorRef, ledRef}
	if activeHigh {
		output.Instance.Nets = []string{inputNet, seriesNet, gndNet}
	} else {
		output.Instance.Nets = []string{vccNet, seriesNet, inputNet}
	}
	return output
}

func twoTerminalHorizontalPins() []transactions.PinSpec {
	// These are schematic symbol anchors only. Footprint pad geometry is
	// resolved separately by the PCB/library writer.
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: 0},
		{Number: "2", XMM: 2.54, YMM: 0},
	}
}

func appendConnectOperation(operations *[]transactions.Operation, issues *[]reports.Issue, fromRef string, fromPin string, toRef string, toPin string, netName string) {
	connect, connectIssues := ConnectOperation(fromRef, fromPin, toRef, toPin, netName)
	*issues = append(*issues, connectIssues...)
	if len(connectIssues) == 0 {
		*operations = append(*operations, connect)
	}
}

func appendNoConnectOperation(operations *[]transactions.Operation, issues *[]reports.Issue, ref string, pin string) {
	noConnect, noConnectIssues := NoConnectOperation(ref, pin)
	*issues = append(*issues, noConnectIssues...)
	if len(noConnectIssues) == 0 {
		*operations = append(*operations, noConnect)
	}
}

func stringParam(params map[string]any, name string) string {
	value, _ := params[name].(string)
	return value
}

func boolParam(params map[string]any, name string, fallback bool) bool {
	value, ok := params[name].(bool)
	if !ok {
		return fallback
	}
	return value
}

type unitMultiplier struct {
	unit       string
	multiplier float64
}

func parseUnit(value any, suffix string, multipliers []unitMultiplier) (float64, bool) {
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	text = normalizeUnitLiteral(text, suffix, multipliers)
	for _, multiplier := range sortedUnitMultipliers(multipliers) {
		unit := multiplier.unit
		if strings.HasSuffix(text, unit+suffix) {
			number := strings.TrimSuffix(text, unit+suffix)
			parsed, err := strconv.ParseFloat(number, 64)
			return parsed * multiplier.multiplier, err == nil
		}
		if strings.HasSuffix(text, unit) {
			number := strings.TrimSuffix(text, unit)
			parsed, err := strconv.ParseFloat(number, 64)
			return parsed * multiplier.multiplier, err == nil
		}
	}
	if strings.HasSuffix(text, suffix) {
		number := strings.TrimSuffix(text, suffix)
		parsed, err := strconv.ParseFloat(number, 64)
		return parsed, err == nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	return parsed, err == nil
}

func sortedUnitMultipliers(multipliers []unitMultiplier) []unitMultiplier {
	sorted := append([]unitMultiplier(nil), multipliers...)
	slices.SortFunc(sorted, func(a unitMultiplier, b unitMultiplier) int {
		if len(a.unit) != len(b.unit) {
			return len(b.unit) - len(a.unit)
		}
		if a.unit < b.unit {
			return -1
		}
		if a.unit > b.unit {
			return 1
		}
		return 0
	})
	return sorted
}

func normalizeUnitLiteral(text string, suffix string, multipliers []unitMultiplier) string {
	text = strings.ReplaceAll(text, " ", "")
	aliases := map[string]string{
		"ohms": "Ω",
		"Ohms": "Ω",
		"ohm":  "Ω",
		"Ohm":  "Ω",
	}
	for alias, replacement := range aliases {
		text = strings.ReplaceAll(text, alias, replacement)
	}
	if suffix == "Ω" {
		text = normalizeResistanceNotation(text, multipliers)
	}
	return text
}

func normalizeResistanceNotation(text string, multipliers []unitMultiplier) string {
	core := strings.TrimSuffix(text, "Ω")
	hasOhmSuffix := core != text
	if strings.HasSuffix(core, "R") {
		return strings.TrimSuffix(core, "R") + "Ω"
	}
	for i := 1; i < len(core)-1; i++ {
		if core[i] == 'R' && isASCIIDigit(core[i-1]) && isASCIIDigit(core[i+1]) {
			return core[:i] + "." + core[i+1:] + "Ω"
		}
	}
	for _, multiplier := range sortedUnitMultipliers(multipliers) {
		unit := multiplier.unit
		for i := 1; i < len(core)-1; i++ {
			next := i + len(unit)
			if next >= len(core) || core[i:next] != unit {
				continue
			}
			if isASCIIDigit(core[i-1]) && isASCIIDigit(core[next]) {
				return core[:i] + "." + core[next:] + unit + "Ω"
			}
		}
	}
	if hasOhmSuffix {
		return core + "Ω"
	}
	return text
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func voltageMultipliers() []unitMultiplier {
	return []unitMultiplier{{"m", 1e-3}, {"u", 1e-6}, {"µ", 1e-6}, {"μ", 1e-6}, {"n", 1e-9}, {"k", 1e3}, {"K", 1e3}}
}

func currentMultipliers() []unitMultiplier {
	return []unitMultiplier{{"m", 1e-3}, {"u", 1e-6}, {"µ", 1e-6}, {"μ", 1e-6}, {"n", 1e-9}, {"k", 1e3}, {"K", 1e3}}
}

func resistanceMultipliers() []unitMultiplier {
	return []unitMultiplier{{"m", 1e-3}, {"u", 1e-6}, {"µ", 1e-6}, {"μ", 1e-6}, {"n", 1e-9}, {"k", 1e3}, {"K", 1e3}, {"M", 1e6}}
}

func formatOhms(ohms float64) string {
	if math.Abs(ohms-math.Round(ohms)) < 1e-9 {
		return fmt.Sprintf("%.0f", ohms)
	}
	return strconv.FormatFloat(ohms, 'f', 2, 64)
}

func hasBlockingIssues(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}
