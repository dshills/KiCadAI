package blocks

import (
	"math"
	"strconv"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const usbPowerLEDResistorValue = "1.5k"

type usbCPinRoleMap struct {
	VBUS   []string
	GND    []string
	CC1    string
	CC2    string
	Shield string
}

var usbCPowerPins = usbCPinRoleMap{
	VBUS: []string{"A9", "B9"},
	GND:  []string{"A12", "B12"},
	CC1:  "A5",
	CC2:  "B5",
	// KiCad 10 Connector:USB_C_Receptacle_PowerOnly_6P names the shield pin SH.
	// KiCadAI uses a project-local 6-pin symbol with the same electrical pin map.
	Shield: "SH",
}

func usbCPowerPinAt(pins []string, index int, fallback string) string {
	if index >= 0 && index < len(pins) && pins[index] != "" {
		return pins[index]
	}
	return fallback
}

func instantiateUSBCPower(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	currentLimit, currentOK := parseUnit(params["current_limit"], "A", currentMultipliers())
	if !currentOK {
		issues = append(issues, blockIssue("params.current_limit", "current_limit must be a current literal"))
	}
	if currentOK && currentLimit <= 0 {
		issues = append(issues, blockIssue("params.current_limit", "current_limit must be positive"))
	}
	connectorFootprint := stringParam(params, "connector_footprint")
	if connectorFootprint == "" {
		issues = append(issues, blockIssue("params.connector_footprint", "connector_footprint is required"))
	}
	if stringParam(params, "data_mode") != "power_only" {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.data_mode",
			Message:    "usb_c_power currently supports power_only mode; USB2 data routing is not implemented",
			Suggestion: "use data_mode power_only until USB data pin-role support is added",
		})
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	issues = append(issues, reports.Issue{
		Code:       reports.CodeUnsupportedOperation,
		Severity:   reports.SeverityWarning,
		Path:       "params.data_mode",
		Message:    "power_only mode does not emit D+/D- no-connect markers because transactions do not support no-connect operations yet",
		Suggestion: "review generated schematic in KiCad and add no-connect markers manually if required",
	})

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	connectorRef := allocator.Next("J")
	connector := BlockComponent{
		Role:        "usb_c_receptacle",
		RefPrefix:   "J",
		Value:       "USB-C Power",
		SymbolID:    defaultUSBCSymbol,
		FootprintID: connectorFootprint,
		Pins:        usbCSymbolPins(usbCPowerPins),
	}
	var operations []transactions.Operation
	var issuesOut []reports.Issue
	issuesOut = append(issuesOut, issues...)
	connectorOps, connectorIssues := ComponentOperations(connector, connectorRef, transactions.Point{XMM: 0, YMM: 0})
	issuesOut = append(issuesOut, connectorIssues...)
	operations = append(operations, connectorOps...)

	vbusConnectorNet := InstanceNetName(request.InstanceID, "vbus_connector")
	vbusOutNet := InstanceNetName(request.InstanceID, "vbus_out")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	cc1Net := InstanceNetName(request.InstanceID, "cc1")
	cc2Net := InstanceNetName(request.InstanceID, "cc2")
	includeFuse := boolParam(params, "include_fuse", true)

	refs := []string{connectorRef}
	nets := []string{vbusOutNet, gndNet, cc1Net, cc2Net}
	if includeFuse {
		nets = append(nets, vbusConnectorNet)
	}
	gndRef := ""
	gndPin := ""
	for _, cc := range []struct {
		role string
		pin  string
		net  string
		xmm  float64
		ymm  float64
	}{
		{role: "cc1_rd", pin: usbCPowerPins.CC1, net: cc1Net, xmm: 20, ymm: 1.27},
		{role: "cc2_rd", pin: usbCPowerPins.CC2, net: cc2Net, xmm: 30, ymm: 3.81},
	} {
		ref := allocator.Next("R")
		rd := BlockComponent{Role: cc.role, RefPrefix: "R", Value: "5.1k", SymbolID: "kicadai:USB_CC_R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: deviceRTemplatePins()}
		rdOps, rdIssues := ComponentOperations(rd, ref, transactions.Point{XMM: cc.xmm, YMM: cc.ymm})
		issuesOut = append(issuesOut, rdIssues...)
		operations = append(operations, rdOps...)
		appendConnectOperation(&operations, &issuesOut, connectorRef, cc.pin, ref, "1", cc.net)
		appendConnectOperation(&operations, &issuesOut, ref, "2", request.InstanceID, "GND", gndNet)
		if gndRef == "" {
			gndRef = ref
			gndPin = "2"
		} else {
			appendConnectOperation(&operations, &issuesOut, ref, "2", gndRef, gndPin, gndNet)
		}
		refs = append(refs, ref)
	}
	for _, pin := range usbCPowerPins.GND {
		appendConnectOperation(&operations, &issuesOut, connectorRef, pin, gndRef, gndPin, gndNet)
	}

	protectedRef := connectorRef
	protectedPin := usbCPowerPins.VBUS[0]
	if includeFuse {
		fuseRef := allocator.Next("F")
		fuse := BlockComponent{Role: "vbus_fuse", RefPrefix: "F", Value: currentLimitLabel(currentLimit, currentOK), SymbolID: "Device:Fuse", FootprintID: "Fuse:Fuse_1206_3216Metric", Pins: usbVerticalTwoTerminalPins()}
		fuseOps, fuseIssues := ComponentOperations(fuse, fuseRef, transactions.Point{XMM: 20, YMM: -3.81})
		issuesOut = append(issuesOut, fuseIssues...)
		operations = append(operations, fuseOps...)
		for _, pin := range usbCPowerPins.VBUS {
			appendConnectOperation(&operations, &issuesOut, connectorRef, pin, fuseRef, "1", vbusConnectorNet)
		}
		appendConnectOperation(&operations, &issuesOut, fuseRef, "2", request.InstanceID, "VBUS_OUT", vbusOutNet)
		protectedRef = fuseRef
		protectedPin = "2"
		refs = append(refs, fuseRef)
	} else {
		for _, pin := range usbCPowerPins.VBUS {
			appendConnectOperation(&operations, &issuesOut, connectorRef, pin, request.InstanceID, "VBUS_OUT", vbusOutNet)
		}
	}
	if boolParam(params, "include_tvs", true) {
		tvsRef := allocator.Next("D")
		tvs := BlockComponent{Role: "vbus_tvs", RefPrefix: "D", Value: "VBUS TVS", SymbolID: "Device:D_TVS", FootprintID: "Diode_SMD:D_SOD-323", Pins: twoTerminalHorizontalPins()}
		tvsOps, tvsIssues := ComponentOperations(tvs, tvsRef, transactions.Point{XMM: 30, YMM: -10.16})
		issuesOut = append(issuesOut, tvsIssues...)
		operations = append(operations, tvsOps...)
		appendConnectOperation(&operations, &issuesOut, tvsRef, "1", protectedRef, protectedPin, vbusOutNet)
		appendConnectOperation(&operations, &issuesOut, tvsRef, "2", gndRef, gndPin, gndNet)
		refs = append(refs, tvsRef)
	}
	if boolParam(params, "include_bulk_capacitor", true) {
		capRef := allocator.Next("C")
		cap := BlockComponent{Role: "bulk_capacitor", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: deviceCTemplatePins()}
		capOps, capIssues := ComponentOperations(cap, capRef, transactions.Point{XMM: 45, YMM: -10.16})
		issuesOut = append(issuesOut, capIssues...)
		operations = append(operations, capOps...)
		appendConnectOperation(&operations, &issuesOut, capRef, "1", protectedRef, protectedPin, vbusOutNet)
		appendConnectOperation(&operations, &issuesOut, capRef, "2", gndRef, gndPin, gndNet)
		refs = append(refs, capRef)
	}
	if boolParam(params, "include_power_led", false) {
		ledOutput, ledIssues := instantiateUSBPowerLED(definition, request, allocator, protectedRef, protectedPin, vbusOutNet, gndRef, gndPin, gndNet)
		issuesOut = append(issuesOut, ledIssues...)
		operations = append(operations, ledOutput.Operations...)
		refs = append(refs, ledOutput.Instance.Refs...)
		nets = append(nets, ledOutput.Instance.Nets...)
	}
	switch stringParam(params, "shield_policy") {
	case "gnd":
		appendConnectOperation(&operations, &issuesOut, connectorRef, usbCPowerPins.Shield, gndRef, gndPin, gndNet)
	case "chassis":
		appendConnectOperation(&operations, &issuesOut, connectorRef, usbCPowerPins.Shield, request.InstanceID, "SHIELD", InstanceNetName(request.InstanceID, "shield"))
		nets = append(nets, InstanceNetName(request.InstanceID, "shield"))
	case "floating":
		appendNoConnectOperation(&operations, &issuesOut, connectorRef, usbCPowerPins.Shield)
	}

	output := dryRunBlockOutput(definition, request, operations, issuesOut)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func usbCSymbolPins(roles usbCPinRoleMap) []transactions.PinSpec {
	positions := map[string]transactions.Point{
		"A5":  {XMM: 15.24, YMM: 5.08},
		"A9":  {XMM: 15.24, YMM: -7.62},
		"A12": {XMM: 0, YMM: 17.78},
		"B5":  {XMM: 15.24, YMM: 7.62},
		"B9":  {XMM: 15.24, YMM: -7.62},
		"B12": {XMM: 0, YMM: 17.78},
		"SH":  {XMM: -7.62, YMM: 17.78},
	}
	pinOrder := append([]string{}, roles.CC1)
	pinOrder = append(pinOrder, roles.VBUS...)
	pinOrder = append(pinOrder, roles.GND...)
	pinOrder = append(pinOrder, roles.CC2, roles.Shield)
	pins := make([]transactions.PinSpec, 0, len(pinOrder))
	seen := map[string]struct{}{}
	for _, pin := range pinOrder {
		if _, exists := seen[pin]; exists {
			continue
		}
		seen[pin] = struct{}{}
		position, ok := positions[pin]
		if !ok {
			continue
		}
		pins = append(pins, transactions.PinSpec{Number: pin, XMM: position.XMM, YMM: position.YMM})
	}
	return pins
}

func usbVerticalTwoTerminalPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: 0, YMM: -3.81},
		{Number: "2", XMM: 0, YMM: 3.81},
	}
}

func currentLimitLabel(currentLimit float64, ok bool) string {
	if !ok || currentLimit <= 0 {
		return "Fuse"
	}
	return formatCurrent(currentLimit)
}

func formatCurrent(amps float64) string {
	if amps < 1 {
		return formatScalar(amps*1000) + "mA"
	}
	return formatScalar(amps) + "A"
}

func formatScalar(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatFloat(value, 'f', 0, 64)
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func instantiateUSBPowerLED(definition BlockDefinition, request BlockRequest, allocator *ReferenceAllocator, vbusRef string, vbusPin string, vbusNet string, gndRef string, gndPin string, gndNet string) (BlockOutput, []reports.Issue) {
	resistorRef := allocator.Next("R")
	ledRef := allocator.Next("D")
	resistor := BlockComponent{Role: "power_led_resistor", RefPrefix: "R", Value: usbPowerLEDResistorValue, SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: deviceRTemplatePins()}
	led := BlockComponent{Role: "power_led", RefPrefix: "D", Value: "POWER LED", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric", Pins: ledPins()}
	var operations []transactions.Operation
	var issues []reports.Issue
	resistorOps, resistorIssues := ComponentOperations(resistor, resistorRef, transactions.Point{XMM: 55, YMM: -12})
	issues = append(issues, resistorIssues...)
	operations = append(operations, resistorOps...)
	ledOps, ledIssues := ComponentOperations(led, ledRef, transactions.Point{XMM: 67.7, YMM: -12})
	issues = append(issues, ledIssues...)
	operations = append(operations, ledOps...)
	seriesNet := InstanceNetName(request.InstanceID, "power_led_series")
	appendConnectOperation(&operations, &issues, vbusRef, vbusPin, resistorRef, "1", vbusNet)
	appendConnectOperation(&operations, &issues, resistorRef, "2", ledRef, "2", seriesNet)
	appendConnectOperation(&operations, &issues, ledRef, "1", gndRef, gndPin, gndNet)
	return BlockOutput{Definition: Summary(definition), Instance: BlockInstance{BlockID: definition.ID, InstanceID: request.InstanceID, Refs: []string{resistorRef, ledRef}, Nets: []string{seriesNet}}, Operations: operations, Issues: issues}, issues
}
