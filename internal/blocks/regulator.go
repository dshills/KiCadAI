package blocks

import (
	"fmt"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const (
	defaultRegulatorSymbol     = "Regulator_Linear:AMS1117-3.3"
	ams1117DropoutWarningVolts = 1.3
	ap2112kDropoutWarningVolts = 0.5
	sot223DissipationWarningW  = 1.0
	sot23DissipationWarningW   = 0.25
	powerLEDForwardVolts       = 2.0
	powerLEDCurrentAmps        = 0.002
)

type regulatorProfile struct {
	Name                string
	Symbol              string
	Footprint           string
	Pins                []transactions.PinSpec
	VINPin              string
	VOUTPin             string
	GNDPin              string
	EnablePin           string
	NoConnectPins       []string
	DropoutWarningVolts float64
	DissipationWarningW float64
}

func instantiateVoltageRegulator(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	inputMin, inputMinOK := parseUnit(params["input_voltage_min"], "V", voltageMultipliers())
	if !inputMinOK {
		issues = append(issues, blockIssue("params.input_voltage_min", "input_voltage_min must be a voltage literal"))
	}
	inputMax, inputMaxOK := parseUnit(params["input_voltage_max"], "V", voltageMultipliers())
	if !inputMaxOK {
		issues = append(issues, blockIssue("params.input_voltage_max", "input_voltage_max must be a voltage literal"))
	}
	inputNominal, inputNominalOK := parseUnit(params["input_voltage"], "V", voltageMultipliers())
	if !inputNominalOK {
		issues = append(issues, blockIssue("params.input_voltage", "input_voltage must be a voltage literal"))
	}
	outputVoltage, outputOK := parseUnit(params["output_voltage"], "V", voltageMultipliers())
	if !outputOK {
		issues = append(issues, blockIssue("params.output_voltage", "output_voltage must be a voltage literal"))
	}
	outputCurrent, currentOK := parseUnit(params["output_current"], "A", currentMultipliers())
	if !currentOK {
		issues = append(issues, blockIssue("params.output_current", "output_current must be a current literal"))
	}
	if currentOK && outputCurrent <= 0 {
		issues = append(issues, blockIssue("params.output_current", "output_current must be positive"))
	}
	if inputMinOK && inputMaxOK && inputMin > inputMax {
		issues = append(issues, blockIssue("params.input_voltage_min", "input_voltage_min must not exceed input_voltage_max"))
	}
	if inputNominalOK && outputOK && inputNominal <= outputVoltage {
		issues = append(issues, blockIssue("params.input_voltage", "input_voltage must be greater than output_voltage for a linear regulator"))
	}
	if _, ok := parseUnit(params["input_capacitance"], "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.input_capacitance", "input_capacitance must be a capacitance literal"))
	}
	if _, ok := parseUnit(params["output_capacitance"], "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.output_capacitance", "output_capacitance must be a capacitance literal"))
	}
	regulatorSymbol := stringParam(params, "regulator_symbol")
	if regulatorSymbol == "" {
		issues = append(issues, blockIssue("params.regulator_symbol", "regulator_symbol is required"))
	}
	regulatorFootprint := stringParam(params, "regulator_footprint")
	if regulatorFootprint == "" {
		issues = append(issues, blockIssue("params.regulator_footprint", "regulator_footprint is required"))
	}
	profile, profileOK := regulatorProfileFor(regulatorSymbol, regulatorFootprint)
	if regulatorSymbol != "" && !profileOK {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.regulator_symbol",
			Message:    "voltage_regulator requires a supported regulator profile with symbol, footprint, and pin-role evidence",
			Suggestion: "use AMS1117 SOT-223 or AP2112K-3.3 SOT-23-5 profile values",
		})
	}
	capacitorFootprint := stringParam(params, "capacitor_footprint")
	if capacitorFootprint == "" {
		issues = append(issues, blockIssue("params.capacitor_footprint", "capacitor_footprint is required"))
	}
	enableMode := stringParam(params, "enable_mode")
	if profileOK {
		issues = append(issues, validateRegulatorEnableMode(profile, enableMode)...)
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if inputMinOK && outputOK && inputMin-outputVoltage < profile.DropoutWarningVolts {
		issues = append(issues, regulatorWarning("params.input_voltage_min", fmt.Sprintf("input_voltage_min is within %.1f V of output_voltage; dropout margin may be insufficient", profile.DropoutWarningVolts)))
	}
	if inputNominalOK && outputOK && currentOK && inputNominal > outputVoltage && (inputNominal-outputVoltage)*outputCurrent > profile.DissipationWarningW {
		issues = append(issues, regulatorWarning("params.output_current", fmt.Sprintf("linear regulator dissipation exceeds %.2g W at nominal input", profile.DissipationWarningW)))
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	regulatorRef := allocator.Next("U")
	inputCapRef := allocator.Next("C")
	outputCapRef := allocator.Next("C")
	regulator := BlockComponent{
		Role:        "regulator",
		RefPrefix:   "U",
		Value:       fmt.Sprintf("LDO %s", stringParam(params, "output_voltage")),
		SymbolID:    regulatorSymbol,
		FootprintID: regulatorFootprint,
		Pins:        profile.Pins,
	}
	inputCap := BlockComponent{
		Role:        "input_capacitor",
		RefPrefix:   "C",
		Value:       stringParam(params, "input_capacitance"),
		SymbolID:    "Device:C",
		FootprintID: capacitorFootprint,
		Pins:        twoTerminalHorizontalPins(),
	}
	outputCap := BlockComponent{
		Role:        "output_capacitor",
		RefPrefix:   "C",
		Value:       stringParam(params, "output_capacitance"),
		SymbolID:    "Device:C",
		FootprintID: capacitorFootprint,
		Pins:        twoTerminalHorizontalPins(),
	}
	var operations []transactions.Operation
	componentOps, componentIssues := ComponentOperations(regulator, regulatorRef, transactions.Point{XMM: 10, YMM: 0})
	issues = append(issues, componentIssues...)
	operations = append(operations, componentOps...)
	componentOps, componentIssues = ComponentOperations(inputCap, inputCapRef, transactions.Point{XMM: 0, YMM: 10})
	issues = append(issues, componentIssues...)
	operations = append(operations, componentOps...)
	componentOps, componentIssues = ComponentOperations(outputCap, outputCapRef, transactions.Point{XMM: 20, YMM: 10})
	issues = append(issues, componentIssues...)
	operations = append(operations, componentOps...)

	vinNet := InstanceNetName(request.InstanceID, "vin")
	voutNet := InstanceNetName(request.InstanceID, "vout")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	appendConnectOperation(&operations, &issues, request.InstanceID, "VIN", regulatorRef, profile.VINPin, vinNet)
	appendConnectOperation(&operations, &issues, regulatorRef, profile.VOUTPin, request.InstanceID, "VOUT", voutNet)
	appendConnectOperation(&operations, &issues, regulatorRef, profile.GNDPin, request.InstanceID, "GND", gndNet)
	appendConnectOperation(&operations, &issues, inputCapRef, "1", regulatorRef, profile.VINPin, vinNet)
	appendConnectOperation(&operations, &issues, outputCapRef, "1", regulatorRef, profile.VOUTPin, voutNet)
	appendConnectOperation(&operations, &issues, regulatorRef, profile.GNDPin, inputCapRef, "2", gndNet)
	appendConnectOperation(&operations, &issues, inputCapRef, "2", outputCapRef, "2", gndNet)
	if profile.EnablePin != "" && (enableMode == "" || enableMode == "tied_input") {
		appendConnectOperation(&operations, &issues, regulatorRef, profile.EnablePin, regulatorRef, profile.VINPin, vinNet)
	}
	for _, pin := range profile.NoConnectPins {
		appendNoConnectOperation(&operations, &issues, regulatorRef, pin)
	}

	refs := []string{regulatorRef, inputCapRef, outputCapRef}
	nets := []string{vinNet, voutNet, gndNet}
	if boolParam(params, "include_power_led", false) {
		ledOutput, ledIssues := instantiateRegulatorPowerLED(definition, request, params, allocator, voutNet, gndNet, outputVoltage)
		issues = append(issues, ledIssues...)
		operations = append(operations, ledOutput.Operations...)
		refs = append(refs, ledOutput.Instance.Refs...)
		nets = append(nets, ledOutput.Instance.Nets...)
	}
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func isSupportedAMS1117Symbol(symbol string) bool {
	return symbol == defaultRegulatorSymbol || strings.HasPrefix(symbol, "Regulator_Linear:AMS1117")
}

func regulatorProfileFor(symbol string, footprint string) (regulatorProfile, bool) {
	switch {
	case isSupportedAMS1117Symbol(symbol) && footprint == "Package_TO_SOT_SMD:SOT-223-3_TabPin2":
		return regulatorProfile{
			Name:                "ams1117_sot223",
			Symbol:              symbol,
			Footprint:           footprint,
			Pins:                fixedRegulatorPins(),
			VINPin:              "3",
			VOUTPin:             "2",
			GNDPin:              "1",
			DropoutWarningVolts: ams1117DropoutWarningVolts,
			DissipationWarningW: sot223DissipationWarningW,
		}, true
	case symbol == "Regulator_Linear:AP2112K-3.3" && footprint == "Package_TO_SOT_SMD:SOT-23-5":
		return regulatorProfile{
			Name:                "ap2112k_3v3_sot23_5",
			Symbol:              symbol,
			Footprint:           footprint,
			Pins:                ap2112kRegulatorPins(),
			VINPin:              "1",
			GNDPin:              "2",
			EnablePin:           "3",
			NoConnectPins:       []string{"4"},
			VOUTPin:             "5",
			DropoutWarningVolts: ap2112kDropoutWarningVolts,
			DissipationWarningW: sot23DissipationWarningW,
		}, true
	default:
		return regulatorProfile{}, false
	}
}

func validateRegulatorEnableMode(profile regulatorProfile, enableMode string) []reports.Issue {
	if profile.EnablePin == "" {
		if enableMode == "" || enableMode == "none" {
			return nil
		}
		return []reports.Issue{{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.enable_mode",
			Message:    "enable_mode is not supported by the selected three-pin regulator profile",
			Suggestion: "use enable_mode none or select a regulator profile with an enable pin map",
		}}
	}
	if enableMode == "" || enableMode == "tied_input" {
		return nil
	}
	if enableMode == "none" {
		return []reports.Issue{{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.enable_mode",
			Message:    "selected regulator has an enable pin that must be handled",
			Suggestion: "use enable_mode tied_input so the regulator is enabled from VIN",
		}}
	}
	if enableMode == "export" {
		return []reports.Issue{{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.enable_mode",
			Message:    "external enable export is not implemented for voltage_regulator",
			Suggestion: "use enable_mode tied_input so the regulator is enabled from VIN",
		}}
	}
	return []reports.Issue{{
		Code:       reports.CodeUnsupportedOperation,
		Severity:   reports.SeverityBlocked,
		Path:       "params.enable_mode",
		Message:    "enable_mode must be none, tied_input, or export",
		Suggestion: "use tied_input for supported enable-pin regulators",
	}}
}

func fixedRegulatorPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: 2.54},
		{Number: "2", XMM: 2.54, YMM: 0},
		{Number: "3", XMM: -2.54, YMM: -2.54},
	}
}

func ap2112kRegulatorPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: -2.54},
		{Number: "2", XMM: 0, YMM: 2.54},
		{Number: "3", XMM: -2.54, YMM: 0},
		{Number: "4", XMM: 2.54, YMM: 0},
		{Number: "5", XMM: 2.54, YMM: -2.54},
	}
}

func regulatorWarning(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: path, Message: message}
}

func instantiateRegulatorPowerLED(definition BlockDefinition, request BlockRequest, params map[string]any, allocator *ReferenceAllocator, voutNet string, gndNet string, outputVoltage float64) (BlockOutput, []reports.Issue) {
	var issues []reports.Issue
	if outputVoltage <= powerLEDForwardVolts {
		issues = append(issues, regulatorWarning("params.output_voltage", "output_voltage is too low for the default power LED model"))
		return BlockOutput{Definition: Summary(definition), Instance: BlockInstance{BlockID: definition.ID, InstanceID: request.InstanceID, Params: params}, Issues: issues}, issues
	}
	resistorRef := allocator.Next("R")
	ledRef := allocator.Next("D")
	resistorOhms := (outputVoltage - powerLEDForwardVolts) / powerLEDCurrentAmps
	resistorPower := powerLEDCurrentAmps * powerLEDCurrentAmps * resistorOhms
	if resistorPower > 0.125 {
		issues = append(issues, regulatorWarning("params.output_voltage", "default power LED resistor dissipation exceeds 0.125 W"))
	}
	resistor := BlockComponent{
		Role:        "power_led_resistor",
		RefPrefix:   "R",
		Value:       formatOhms(resistorOhms),
		SymbolID:    "Device:R",
		FootprintID: "Resistor_SMD:R_0805_2012Metric",
		Pins:        twoTerminalHorizontalPins(),
	}
	led := BlockComponent{
		Role:        "power_led",
		RefPrefix:   "D",
		Value:       "POWER LED",
		SymbolID:    "Device:LED",
		FootprintID: "LED_SMD:LED_0805_2012Metric",
		Pins:        twoTerminalHorizontalPins(),
	}
	var operations []transactions.Operation
	componentOps, componentIssues := ComponentOperations(resistor, resistorRef, transactions.Point{XMM: 35, YMM: 0})
	issues = append(issues, componentIssues...)
	operations = append(operations, componentOps...)
	componentOps, componentIssues = ComponentOperations(led, ledRef, transactions.Point{XMM: 47.7, YMM: 0})
	issues = append(issues, componentIssues...)
	operations = append(operations, componentOps...)
	seriesNet := InstanceNetName(request.InstanceID, "power_led_series")
	appendConnectOperation(&operations, &issues, resistorRef, "2", ledRef, "1", seriesNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "VOUT", resistorRef, "1", voutNet)
	appendConnectOperation(&operations, &issues, ledRef, "2", request.InstanceID, "GND", gndNet)
	return BlockOutput{
		Definition: Summary(definition),
		Instance:   BlockInstance{BlockID: definition.ID, InstanceID: request.InstanceID, Params: params, Refs: []string{resistorRef, ledRef}, Nets: []string{seriesNet}},
		Operations: operations,
		Issues:     issues,
	}, issues
}

func capacitanceMultipliers() []unitMultiplier {
	return []unitMultiplier{{"p", 1e-12}, {"n", 1e-9}, {"u", 1e-6}, {"µ", 1e-6}, {"μ", 1e-6}, {"m", 1e-3}}
}
