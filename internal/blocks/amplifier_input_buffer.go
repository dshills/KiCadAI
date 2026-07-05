package blocks

import (
	"math"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func instantiateAmplifierInputBuffer(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	inputOhms, inputOK := parseUnit(params["input_impedance"], "Ω", resistanceMultipliers())
	if !inputOK || inputOhms <= 0 {
		issues = append(issues, blockIssue("params.input_impedance", "input_impedance must be a positive resistance literal"))
	}
	couplingFarads, couplingOK := parseUnit(params["coupling_capacitance"], "F", capacitanceMultipliers())
	if !couplingOK || couplingFarads <= 0 {
		issues = append(issues, blockIssue("params.coupling_capacitance", "coupling_capacitance must be a positive capacitance literal"))
	}
	resistorFootprint := stringParam(params, "resistor_footprint")
	if resistorFootprint == "" {
		issues = append(issues, blockIssue("params.resistor_footprint", "resistor_footprint is required"))
	}
	stopperOhms, stopperOK := parseUnit(params["input_stopper_value"], "Ω", resistanceMultipliers())
	if !stopperOK || stopperOhms <= 0 {
		issues = append(issues, blockIssue("params.input_stopper_value", "input_stopper_value must be a positive resistance literal"))
	}
	capacitorFootprint := stringParam(params, "capacitor_footprint")
	if capacitorFootprint == "" {
		issues = append(issues, blockIssue("params.capacitor_footprint", "capacitor_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	couplingRef := allocator.Next("C")
	biasTopRef := allocator.Next("R")
	biasBottomRef := allocator.Next("R")
	stopperRef := allocator.Next("R")
	dividerResistor := formatOhms(inputOhms * 2)
	cutoffHz := 1 / (2 * math.Pi * (inputOhms + stopperOhms) * couplingFarads)
	params["high_pass_cutoff_hz"] = cutoffHz

	coupling, ok := amplifierInputBufferComponent(definition, "input_coupling")
	if !ok {
		issues = append(issues, blockIssue("components.input_coupling", "amplifier_input_buffer requires an input_coupling component definition"))
	}
	biasTop, ok := amplifierInputBufferComponent(definition, "bias_top")
	if !ok {
		issues = append(issues, blockIssue("components.bias_top", "amplifier_input_buffer requires a bias_top component definition"))
	}
	biasBottom, ok := amplifierInputBufferComponent(definition, "bias_bottom")
	if !ok {
		issues = append(issues, blockIssue("components.bias_bottom", "amplifier_input_buffer requires a bias_bottom component definition"))
	}
	stopper, ok := amplifierInputBufferComponent(definition, "input_stopper")
	if !ok {
		issues = append(issues, blockIssue("components.input_stopper", "amplifier_input_buffer requires an input_stopper component definition"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	coupling.Value = stringParam(params, "coupling_capacitance")
	coupling.FootprintID = capacitorFootprint
	biasTop.Value = dividerResistor
	biasTop.FootprintID = resistorFootprint
	biasBottom.Value = dividerResistor
	biasBottom.FootprintID = resistorFootprint
	stopper.Value = formatOhms(stopperOhms)
	stopper.FootprintID = resistorFootprint

	var operations []transactions.Operation
	var issuesOut []reports.Issue
	issuesOut = append(issuesOut, issues...)
	for _, item := range []struct {
		component BlockComponent
		ref       string
		at        transactions.Point
	}{
		{stopper, stopperRef, transactions.Point{XMM: 4, YMM: 25}},
		{coupling, couplingRef, transactions.Point{XMM: 18, YMM: 25}},
		{biasTop, biasTopRef, transactions.Point{XMM: 32, YMM: 13}},
		{biasBottom, biasBottomRef, transactions.Point{XMM: 32, YMM: 37}},
	} {
		componentOps, componentIssues := ComponentOperations(item.component, item.ref, item.at)
		issuesOut = append(issuesOut, componentIssues...)
		operations = append(operations, componentOps...)
	}

	inNet := InstanceNetName(request.InstanceID, "in")
	preCoupleNet := InstanceNetName(request.InstanceID, "pre_coupling")
	outNet := InstanceNetName(request.InstanceID, "out")
	vccNet := InstanceNetName(request.InstanceID, "vcc")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	appendConnectOperation(&operations, &issuesOut, request.InstanceID, "IN", stopperRef, "1", inNet)
	appendConnectOperation(&operations, &issuesOut, stopperRef, "2", couplingRef, "1", preCoupleNet)
	appendConnectOperation(&operations, &issuesOut, couplingRef, "2", request.InstanceID, "OUT", outNet)
	appendConnectOperation(&operations, &issuesOut, couplingRef, "2", biasTopRef, "2", outNet)
	appendConnectOperation(&operations, &issuesOut, biasTopRef, "1", request.InstanceID, "VCC", vccNet)
	appendConnectOperation(&operations, &issuesOut, biasBottomRef, "1", biasTopRef, "2", outNet)
	appendConnectOperation(&operations, &issuesOut, biasBottomRef, "2", request.InstanceID, "GND", gndNet)

	refs := []string{stopperRef, couplingRef, biasTopRef, biasBottomRef}
	nets := []string{inNet, outNet, vccNet, gndNet, preCoupleNet}
	output := dryRunBlockOutput(definition, request, operations, issuesOut)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func amplifierInputBufferComponent(definition BlockDefinition, role string) (BlockComponent, bool) {
	for _, component := range definition.Components {
		if component.Role == role {
			return component, true
		}
	}
	return BlockComponent{}, false
}
