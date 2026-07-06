package blocks

import (
	"math"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type opAmpPinRoleMap struct {
	OUT string
	INN string
	INP string
	VEE string
	VCC string
}

var lmv321Pins = opAmpPinRoleMap{OUT: "1", INN: "4", INP: "3", VEE: "2", VCC: "5"}

func instantiateOpAmpGainStage(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if stringParam(params, "topology") != "non_inverting" {
		issues = append(issues, reports.Issue{Code: reports.CodeUnsupportedOperation, Severity: reports.SeverityBlocked, Path: "params.topology", Message: "only non_inverting op-amp topology is implemented"})
	}
	gain, gainOK := numericValue(params["gain"])
	if !gainOK || gain <= 1 {
		issues = append(issues, blockIssue("params.gain", "gain must be greater than 1 for the non-inverting template"))
	}
	if stringParam(params, "opamp_symbol") != defaultOpAmpSymbol {
		issues = append(issues, reports.Issue{Code: reports.CodeUnsupportedOperation, Severity: reports.SeverityBlocked, Path: "params.opamp_symbol", Message: "opamp_gain_stage currently supports only the LMV321 pin-role template"})
	}
	singleSupply := boolParam(params, "single_supply", true)
	if !singleSupply && stringParam(params, "input_coupling") == "ac" {
		issues = append(issues, reports.Issue{Code: reports.CodeUnsupportedOperation, Severity: reports.SeverityBlocked, Path: "params.input_coupling", Message: "ac input coupling currently requires single_supply bias generation"})
	}
	opampFootprint := stringParam(params, "opamp_footprint")
	if opampFootprint == "" {
		issues = append(issues, blockIssue("params.opamp_footprint", "opamp_footprint is required"))
	}
	feedbackFootprint := stringParam(params, "feedback_footprint")
	if feedbackFootprint == "" {
		issues = append(issues, blockIssue("params.feedback_footprint", "feedback_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if !singleSupply {
		issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "params.single_supply", Message: "dual-supply mode treats the GND port as the op-amp VEE rail"})
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	opampRef := allocator.Next("U")
	rgRef := allocator.Next("R")
	rfRef := allocator.Next("R")
	decouplingRef := allocator.Next("C")
	rgOhms := 10000.0
	rfOhms := nearestStandardResistance((gain - 1) * rgOhms)
	opamp := BlockComponent{Role: "opamp", RefPrefix: "U", Value: "LMV321", SymbolID: defaultOpAmpSymbol, FootprintID: opampFootprint, Pins: opAmpPins(lmv321Pins)}
	rg := BlockComponent{Role: "gain_to_ground", RefPrefix: "R", Value: formatOhms(rgOhms), SymbolID: "Device:R", FootprintID: feedbackFootprint, Pins: twoTerminalHorizontalPins()}
	rf := BlockComponent{Role: "feedback", RefPrefix: "R", Value: formatOhms(rfOhms), SymbolID: "Device:R", FootprintID: feedbackFootprint, Pins: twoTerminalHorizontalPins()}
	decoupling := BlockComponent{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()}
	var operations []transactions.Operation
	var issuesOut []reports.Issue
	issuesOut = append(issuesOut, issues...)
	for _, item := range []struct {
		component BlockComponent
		ref       string
		at        transactions.Point
	}{
		{opamp, opampRef, transactions.Point{XMM: 45, YMM: 25}},
		{rg, rgRef, transactions.Point{XMM: 30, YMM: 48}},
		{rf, rfRef, transactions.Point{XMM: 45, YMM: 7}},
		{decoupling, decouplingRef, transactions.Point{XMM: 65, YMM: 8}},
	} {
		componentOps, componentIssues := ComponentOperations(item.component, item.ref, item.at)
		issuesOut = append(issuesOut, componentIssues...)
		operations = append(operations, componentOps...)
	}
	inNet := InstanceNetName(request.InstanceID, "in")
	outNet := InstanceNetName(request.InstanceID, "out")
	includeOutputResistor := boolParam(params, "include_output_resistor", false)
	opampOutputNet := outNet
	if includeOutputResistor {
		opampOutputNet = InstanceNetName(request.InstanceID, "out_drive")
	}
	feedbackNet := InstanceNetName(request.InstanceID, "feedback")
	vccNet := InstanceNetName(request.InstanceID, "vcc")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	if stringParam(params, "input_coupling") == "dc" {
		appendConnectOperation(&operations, &issuesOut, request.InstanceID, "IN", opampRef, lmv321Pins.INP, inNet)
	}
	if !includeOutputResistor {
		appendConnectOperation(&operations, &issuesOut, opampRef, lmv321Pins.OUT, request.InstanceID, "OUT", outNet)
	}
	appendConnectOperation(&operations, &issuesOut, opampRef, lmv321Pins.VCC, request.InstanceID, "VCC", vccNet)
	appendConnectOperation(&operations, &issuesOut, opampRef, lmv321Pins.VEE, request.InstanceID, "GND", gndNet)
	appendConnectOperation(&operations, &issuesOut, rgRef, "1", opampRef, lmv321Pins.INN, feedbackNet)
	appendConnectOperation(&operations, &issuesOut, rgRef, "2", opampRef, lmv321Pins.VEE, gndNet)
	appendConnectOperation(&operations, &issuesOut, rfRef, "1", opampRef, lmv321Pins.OUT, opampOutputNet)
	appendConnectOperation(&operations, &issuesOut, rfRef, "2", opampRef, lmv321Pins.INN, feedbackNet)
	appendConnectOperation(&operations, &issuesOut, decouplingRef, "1", opampRef, lmv321Pins.VCC, vccNet)
	appendConnectOperation(&operations, &issuesOut, decouplingRef, "2", opampRef, lmv321Pins.VEE, gndNet)

	refs := []string{opampRef, rgRef, rfRef, decouplingRef}
	nets := []string{inNet, outNet, feedbackNet, vccNet, gndNet}
	if includeOutputResistor {
		nets = append(nets, opampOutputNet)
	}
	if stringParam(params, "input_coupling") == "ac" {
		refs, nets, operations = appendOpAmpBiasNetwork(request.InstanceID, allocator, feedbackFootprint, opampRef, refs, nets, operations, &issuesOut)
	}
	if includeOutputResistor {
		outRef := allocator.Next("R")
		component := BlockComponent{Role: "output_resistor", RefPrefix: "R", Value: "100", SymbolID: "Device:R", FootprintID: feedbackFootprint, Pins: twoTerminalHorizontalPins()}
		componentOps, componentIssues := ComponentOperations(component, outRef, transactions.Point{XMM: 72, YMM: 25})
		issuesOut = append(issuesOut, componentIssues...)
		operations = append(operations, componentOps...)
		appendConnectOperation(&operations, &issuesOut, opampRef, lmv321Pins.OUT, outRef, "1", opampOutputNet)
		appendConnectOperation(&operations, &issuesOut, outRef, "2", request.InstanceID, "OUT", outNet)
		refs = append(refs, outRef)
	}
	output := dryRunBlockOutput(definition, request, operations, issuesOut)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func opAmpPins(roles opAmpPinRoleMap) []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: roles.OUT, XMM: 2.54, YMM: 0},
		{Number: roles.INN, XMM: -2.54, YMM: -1.27},
		{Number: roles.INP, XMM: -2.54, YMM: 1.27},
		{Number: roles.VEE, XMM: 0, YMM: 2.54},
		{Number: roles.VCC, XMM: 0, YMM: -2.54},
	}
}

func appendOpAmpBiasNetwork(instanceID string, allocator *ReferenceAllocator, footprint string, opampRef string, refs []string, nets []string, operations []transactions.Operation, issues *[]reports.Issue) ([]string, []string, []transactions.Operation) {
	biasTopRef := allocator.Next("R")
	biasBottomRef := allocator.Next("R")
	couplingRef := allocator.Next("C")
	biasNet := InstanceNetName(instanceID, "bias")
	for _, item := range []struct {
		component BlockComponent
		ref       string
		at        transactions.Point
	}{
		{BlockComponent{Role: "bias_top", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: footprint, Pins: twoTerminalHorizontalPins()}, biasTopRef, transactions.Point{XMM: 18, YMM: 16}},
		{BlockComponent{Role: "bias_bottom", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: footprint, Pins: twoTerminalHorizontalPins()}, biasBottomRef, transactions.Point{XMM: 18, YMM: 42}},
		{BlockComponent{Role: "input_coupling", RefPrefix: "C", Value: "1uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()}, couplingRef, transactions.Point{XMM: 5, YMM: 25}},
	} {
		componentOps, componentIssues := ComponentOperations(item.component, item.ref, item.at)
		*issues = append(*issues, componentIssues...)
		operations = append(operations, componentOps...)
	}
	appendConnectOperation(&operations, issues, biasTopRef, "1", opampRef, lmv321Pins.VCC, InstanceNetName(instanceID, "vcc"))
	appendConnectOperation(&operations, issues, biasTopRef, "2", biasBottomRef, "1", biasNet)
	appendConnectOperation(&operations, issues, biasBottomRef, "2", opampRef, lmv321Pins.VEE, InstanceNetName(instanceID, "gnd"))
	appendConnectOperation(&operations, issues, couplingRef, "1", instanceID, "IN", InstanceNetName(instanceID, "in"))
	appendConnectOperation(&operations, issues, couplingRef, "2", opampRef, lmv321Pins.INP, biasNet)
	appendConnectOperation(&operations, issues, biasBottomRef, "1", opampRef, lmv321Pins.INP, biasNet)
	refs = append(refs, biasTopRef, biasBottomRef, couplingRef)
	nets = append(nets, biasNet)
	return refs, nets, operations
}

func nearestStandardResistance(target float64) float64 {
	if target <= 0 {
		return 0
	}
	e24 := []float64{10, 11, 12, 13, 15, 16, 18, 20, 22, 24, 27, 30, 33, 36, 39, 43, 47, 51, 56, 62, 68, 75, 82, 91}
	decade := math.Pow(10, math.Floor(math.Log10(target/10)))
	best := e24[0] * decade
	bestDelta := math.Abs(target - best)
	for scale := decade; scale <= decade*100; scale *= 10 {
		for _, base := range e24 {
			candidate := base * scale
			if delta := math.Abs(target - candidate); delta < bestDelta {
				best = candidate
				bestDelta = delta
			}
		}
	}
	return best
}
