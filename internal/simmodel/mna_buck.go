package simmodel

import (
	"math"
	"math/cmplx"
	"slices"
)

// stampSynchronousBuckCurrentMode stamps a deterministic averaged
// peak-current-mode power stage. The controlled branch represents positive
// inductor current delivered from SW. Its KCL coefficients preserve power at
// the reviewed conversion-efficiency boundary while the branch equation
// closes the catalog controller around FB. The external inductor, output
// capacitor, load, and feedback divider remain explicit resolved devices.
func stampSynchronousBuckCurrentMode(
	system *mnaSystem,
	component string,
	terminals map[string]string,
	transconductance complex128,
	referenceV float64,
	inputCurrentRatio float64,
) {
	branch := system.branchIndex[component]
	if sw, ok := system.nodeIndex[terminals["SW"]]; ok {
		system.matrix[sw][branch] -= 1
	}
	if input, ok := system.nodeIndex[terminals["PVIN"]]; ok {
		system.matrix[input][branch] += complex(inputCurrentRatio, 0)
	}
	if ground, ok := system.nodeIndex[terminals["PGND"]]; ok {
		system.matrix[ground][branch] += complex(1-inputCurrentRatio, 0)
	}
	stampSynchronousBuckControlEquation(system, component, terminals, transconductance, referenceV)
}

func stampSynchronousBuckControlEquation(
	system *mnaSystem,
	component string,
	terminals map[string]string,
	transconductance complex128,
	referenceV float64,
) {
	branch := system.branchIndex[component]
	system.matrix[branch][branch] += 1 / transconductance
	if feedback, ok := system.nodeIndex[terminals["FB"]]; ok {
		system.matrix[branch][feedback] += 1
	}
	if ground, ok := system.nodeIndex[terminals["AGND"]]; ok {
		system.matrix[branch][ground] -= 1
	}
	system.rhs[branch] += complex(referenceV, 0)
}

func resetSynchronousBuckControlEquation(
	system *mnaSystem,
	component string,
	terminals map[string]string,
	transconductance complex128,
	referenceV float64,
) {
	branch := system.branchIndex[component]
	for column := range system.matrix[branch] {
		system.matrix[branch][column] = 0
	}
	system.rhs[branch] = 0
	stampSynchronousBuckControlEquation(system, component, terminals, transconductance, referenceV)
}

func adjustSynchronousBuckInputCurrentRatio(system *mnaSystem, device ResolvedDevice, delta float64) {
	branch := system.branchIndex[device.Component]
	terminals := terminalMap(device)
	if input, ok := system.nodeIndex[terminals["PVIN"]]; ok {
		system.matrix[input][branch] += complex(delta, 0)
	}
	if ground, ok := system.nodeIndex[terminals["PGND"]]; ok {
		system.matrix[ground][branch] -= complex(delta, 0)
	}
}

func synchronousBuckTransconductance(parameters map[string]float64, frequencyHz float64) complex128 {
	transconductance := complex(parameters["control_transconductance_s"], 0)
	if frequencyHz <= 0 {
		return transconductance
	}
	return transconductance / complex(1, frequencyHz/parameters["control_pole_hz"])
}

func synchronousBuckInputCurrentRatio(plan Plan, analysis Analysis, device ResolvedDevice, timeS float64) float64 {
	parameters := deviceParameterMap(device)
	return synchronousBuckInputCurrentRatioForOutput(
		plan, analysis, device, timeS, parameters["nominal_output_voltage_v"],
	)
}

func synchronousBuckInputCurrentRatioForOutput(plan Plan, analysis Analysis, device ResolvedDevice, timeS, outputV float64) float64 {
	parameters := deviceParameterMap(device)
	inputV := synchronousBuckInputVoltage(plan, analysis, device, timeS)
	if inputV <= 0 {
		inputV = parameters["nominal_input_voltage_v"]
	}
	if outputV <= 0 {
		outputV = parameters["nominal_output_voltage_v"]
	}
	ratio := outputV / inputV / parameters["conversion_efficiency_fraction"]
	if !finite(ratio) {
		return 0
	}
	return math.Max(0, math.Min(1/parameters["conversion_efficiency_fraction"], ratio))
}

func synchronousBuckInputVoltage(plan Plan, analysis Analysis, device ResolvedDevice, timeS float64) float64 {
	terminals := terminalMap(device)
	input, inputKnown := transientKnownNodeVoltage(plan, analysis, terminals["PVIN"], timeS)
	ground, groundKnown := transientKnownNodeVoltage(plan, analysis, terminals["PGND"], timeS)
	if !inputKnown || !groundKnown {
		return 0
	}
	return math.Abs(input - ground)
}

func synchronousBuckSoftStartScale(plan Plan, analysis Analysis, device ResolvedDevice, timeS float64) float64 {
	parameters := deviceParameterMap(device)
	softStartS := parameters["soft_start_time_s"]
	if output, ok := synchronousBuckOutputNet(plan, device); ok {
		ground := terminalMap(device)["PGND"]
		capacitanceF := 0.0
		for _, candidate := range plan.Devices {
			if candidate.PrimitiveModel != PrimitiveCapacitorTransientV1 || candidate.ValueSI == nil {
				continue
			}
			terminals := terminalMap(candidate)
			if (terminals["A"] == output && terminals["B"] == ground) ||
				(terminals["B"] == output && terminals["A"] == ground) {
				capacitanceF += *candidate.ValueSI
			}
		}
		if peakCurrentA := parameters["peak_current_limit_a"]; capacitanceF > 0 && peakCurrentA > 0 {
			softStartS = math.Max(softStartS, capacitanceF*parameters["nominal_output_voltage_v"]/peakCurrentA)
		}
	}
	if softStartS <= 0 {
		return 1
	}
	if analysis.Kind == AnalysisStartup {
		return math.Max(0, math.Min(1, timeS/softStartS))
	}
	terminals := terminalMap(device)
	activation, found := sourceActivationTime(
		plan, analysis, terminals["PVIN"], terminals["PGND"],
		parameters["min_input_voltage_v"], timeS,
	)
	if enableActivation, enableFound := sourceActivationTime(
		plan, analysis, terminals["EN"], terminals["AGND"],
		parameters["enable_threshold_v"], timeS,
	); enableFound && (!found || enableActivation > activation) {
		activation, found = enableActivation, true
	}
	if !found {
		return 1
	}
	return math.Max(0, math.Min(1, (timeS-activation)/softStartS))
}

func sourceActivationTime(plan Plan, analysis Analysis, positiveNode, negativeNode string, threshold, timeS float64) (float64, bool) {
	activation := 0.0
	found := false
	for _, event := range analysis.SourceValueEvents {
		if event.TriggerTimeS > timeS {
			continue
		}
		sourceIndex := slices.IndexFunc(plan.Devices, func(candidate ResolvedDevice) bool { return candidate.Component == event.Component })
		if sourceIndex < 0 {
			continue
		}
		terminals := terminalMap(plan.Devices[sourceIndex])
		sourcePositive, sourceNegative := "", ""
		switch plan.Devices[sourceIndex].PrimitiveModel {
		case PrimitiveVoltageSourceV1:
			sourcePositive, sourceNegative = terminals["POSITIVE"], terminals["NEGATIVE"]
		case PrimitiveConnectorVoltageSourceV1:
			sourcePositive, sourceNegative = terminals["PIN_1"], terminals["PIN_2"]
		default:
			continue
		}
		polarity := 0.0
		switch {
		case sourcePositive == positiveNode && sourceNegative == negativeNode:
			polarity = 1
		case sourcePositive == negativeNode && sourceNegative == positiveNode:
			polarity = -1
		default:
			continue
		}
		initial := math.Abs(polarity * event.Initial)
		applied := math.Abs(polarity * event.Applied)
		if initial >= threshold || applied < threshold {
			continue
		}
		if !found || event.TriggerTimeS > activation {
			activation, found = event.TriggerTimeS, true
		}
	}
	return activation, found
}

func synchronousBuckOutputNet(plan Plan, device ResolvedDevice) (string, bool) {
	switchNode := terminalMap(device)["SW"]
	output := ""
	for _, candidate := range plan.Devices {
		if candidate.PrimitiveModel != PrimitiveInductorTransientV1 {
			continue
		}
		terminals := terminalMap(candidate)
		other := ""
		switch {
		case terminals["A"] == switchNode:
			other = terminals["B"]
		case terminals["B"] == switchNode:
			other = terminals["A"]
		}
		if other == "" {
			continue
		}
		if output != "" && output != other {
			return "", false
		}
		output = other
	}
	return output, output != ""
}

func synchronousBuckDissipation(device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if device.PrimitiveModel != PrimitiveSynchronousBuckRegulatorV1 {
		return 0, false
	}
	branch, exists := system.branchIndex[device.Component]
	if !exists || branch >= len(solution) {
		return 0, true
	}
	parameters := deviceParameterMap(device)
	terminals := terminalMap(device)
	nodeV := func(node string) float64 { return real(solvedNodeVoltage(system, solution, node)) }
	inputV := math.Abs(nodeV(terminals["PVIN"]) - nodeV(terminals["PGND"]))
	switchV := math.Abs(nodeV(terminals["SW"]) - nodeV(terminals["PGND"]))
	current := math.Abs(real(solution[branch]))
	duty := 0.0
	if inputV > 0 {
		duty = math.Max(0, math.Min(1, switchV/inputV))
	}
	conduction := current * current * (duty*parameters["high_side_on_resistance_ohm"] +
		(1-duty)*parameters["low_side_on_resistance_ohm"])
	switching := .5 * inputV * current * parameters["switch_transition_time_s"] *
		parameters["switching_frequency_hz"]
	// Quiescent current is retained here for device temperature even though
	// the empirical electrical efficiency boundary already includes it.
	quiescent := inputV * parameters["quiescent_current_a"]
	loss := conduction + switching + quiescent
	if !finite(loss) {
		return 0, true
	}
	return math.Max(0, loss), true
}

func synchronousBuckDeviceResult(device ResolvedDevice, system mnaSystem, solution []complex128) (complex128, complex128, bool) {
	if device.PrimitiveModel != PrimitiveSynchronousBuckRegulatorV1 {
		return 0, 0, false
	}
	branch, exists := system.branchIndex[device.Component]
	if !exists || branch >= len(solution) {
		return 0, 0, true
	}
	terminals := terminalMap(device)
	voltage := solvedNodeVoltage(system, solution, terminals["PVIN"]) -
		solvedNodeVoltage(system, solution, terminals["PGND"])
	current := solution[branch]
	if !boundedComplex(voltage, maxMNASolutionValue) || !boundedComplex(current, maxMNASolutionValue) ||
		math.IsNaN(cmplx.Abs(current)) {
		return 0, 0, true
	}
	return voltage, current, true
}
