package simmodel

import (
	"fmt"
	"math"
)

const directionControlledTranslatorChannels = 8

func directionControlledTranslatorPowered(device compiledNonlinearDevice, system *mnaSystem, solution []complex128) bool {
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	vcca := nonlinearNodeVoltage(system, solution, device.terminals["VCCA"]) - ground
	vccb := nonlinearNodeVoltage(system, solution, device.terminals["VCCB"]) - ground
	return vcca >= device.parameters["vcca_min_v"] && vccb >= device.parameters["vccb_min_v"]
}

func directionControlledTranslatorEnabled(device compiledNonlinearDevice, system *mnaSystem, solution []complex128) bool {
	if !directionControlledTranslatorPowered(device, system, solution) {
		return false
	}
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	vcca := nonlinearNodeVoltage(system, solution, device.terminals["VCCA"]) - ground
	oe := nonlinearNodeVoltage(system, solution, device.terminals["OE"]) - ground
	return oe <= device.parameters["control_low_ratio"]*vcca
}

func directionControlledTranslatorDirection(device compiledNonlinearDevice, system *mnaSystem, solution []complex128, channel int) (inputPrefix, outputPrefix, outputSupply string, valid bool) {
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	vcca := nonlinearNodeVoltage(system, solution, device.terminals["VCCA"]) - ground
	control := "DIR1"
	if channel > 4 {
		control = "DIR2"
	}
	direction := nonlinearNodeVoltage(system, solution, device.terminals[control]) - ground
	if direction <= device.parameters["control_low_ratio"]*vcca {
		return "B", "A", "VCCA", true
	}
	if direction >= device.parameters["control_high_ratio"]*vcca {
		return "A", "B", "VCCB", true
	}
	return "", "", "", false
}

func directionControlledTranslatorInputState(device compiledNonlinearDevice, system *mnaSystem, solution []complex128, channel int) (high, valid bool) {
	inputPrefix, _, _, directionValid := directionControlledTranslatorDirection(device, system, solution, channel)
	if !directionValid {
		return false, false
	}
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	inputSupply := nonlinearNodeVoltage(system, solution, device.terminals["VCCA"]) - ground
	if inputPrefix == "B" {
		inputSupply = nonlinearNodeVoltage(system, solution, device.terminals["VCCB"]) - ground
	}
	input := nonlinearNodeVoltage(system, solution, device.terminals[fmt.Sprintf("%s%d", inputPrefix, channel)]) - ground
	if input <= device.parameters["input_low_ratio"]*inputSupply {
		return false, true
	}
	if input >= device.parameters["input_high_ratio"]*inputSupply {
		return true, true
	}
	return false, false
}

func directionControlledTranslatorOutputState(device compiledNonlinearDevice, system *mnaSystem, solution []complex128, channel int) (output, reference string, resistance float64, active bool) {
	inputPrefix, outputPrefix, outputSupply, directionValid := directionControlledTranslatorDirection(device, system, solution, channel)
	_ = inputPrefix
	if !directionControlledTranslatorEnabled(device, system, solution) || !directionValid {
		return "", "", device.parameters["output_off_resistance_ohm"], false
	}
	high, inputValid := directionControlledTranslatorInputState(device, system, solution, channel)
	if !inputValid {
		return "", "", device.parameters["output_off_resistance_ohm"], false
	}
	output = device.terminals[fmt.Sprintf("%s%d", outputPrefix, channel)]
	reference = device.terminals["GND"]
	resistance = device.parameters["output_low_resistance_ohm"]
	if high {
		reference = device.terminals[outputSupply]
		resistance = device.parameters["output_high_resistance_ohm"]
	}
	return output, reference, resistance, true
}

func stampNonlinearDirectionControlledTranslator(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		output, reference, resistance, active := directionControlledTranslatorOutputState(device, system, guess, channel)
		if active {
			stampAdmittance(system, output, reference, complex(1/resistance, 0))
			continue
		}
		for _, prefix := range []string{"A", "B"} {
			stampAdmittance(system, device.terminals[fmt.Sprintf("%s%d", prefix, channel)], device.terminals["GND"], complex(1/resistance, 0))
		}
	}
	stampBoundedSupplyLoad(system, device.terminals["VCCA"], device.terminals["GND"], device.parameters["vcca_min_v"], device.parameters["vcca_quiescent_current_a"], guess)
	stampBoundedSupplyLoad(system, device.terminals["VCCB"], device.terminals["GND"], device.parameters["vccb_min_v"], device.parameters["vccb_quiescent_current_a"], guess)
}

func addDirectionControlledTranslatorResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		output, reference, resistance, active := directionControlledTranslatorOutputState(device, &base, solution, channel)
		pairs := [][2]string{{output, reference}}
		if !active {
			pairs = pairs[:0]
			for _, prefix := range []string{"A", "B"} {
				pairs = append(pairs, [2]string{device.terminals[fmt.Sprintf("%s%d", prefix, channel)], device.terminals["GND"]})
			}
		}
		for _, pair := range pairs {
			current := (nonlinearNodeVoltage(&base, solution, pair[0]) - nonlinearNodeVoltage(&base, solution, pair[1])) / resistance
			if index, exists := base.nodeIndex[pair[0]]; exists {
				residuals[index] += complex(current, 0)
			}
			if index, exists := base.nodeIndex[pair[1]]; exists {
				residuals[index] -= complex(current, 0)
			}
		}
	}
	addBoundedSupplyLoadResidual(residuals, base, device.terminals["VCCA"], device.terminals["GND"], device.parameters["vcca_min_v"], device.parameters["vcca_quiescent_current_a"], solution)
	addBoundedSupplyLoadResidual(residuals, base, device.terminals["VCCB"], device.terminals["GND"], device.parameters["vccb_min_v"], device.parameters["vccb_quiescent_current_a"], solution)
}

func directionControlledTranslatorMaximumOutput(device compiledNonlinearDevice, system mnaSystem, solution []complex128) (float64, float64) {
	maximumVoltage, maximumCurrent := 0.0, 0.0
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		output, reference, resistance, active := directionControlledTranslatorOutputState(device, &system, solution, channel)
		if !active {
			continue
		}
		delta := nonlinearNodeVoltage(&system, solution, output) - nonlinearNodeVoltage(&system, solution, reference)
		if math.Abs(delta) > math.Abs(maximumVoltage) {
			maximumVoltage = delta
		}
		maximumCurrent = math.Max(maximumCurrent, math.Abs(delta/resistance))
	}
	return maximumVoltage, maximumCurrent
}

func validateDirectionControlledTranslatorOperatingLimits(device ResolvedDevice, system mnaSystem, solution []complex128, allowPowerTransition bool) []Diagnostic {
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	ground := real(solvedNodeVoltage(system, solution, terminals["GND"]))
	vcca := real(solvedNodeVoltage(system, solution, terminals["VCCA"])) - ground
	vccb := real(solvedNodeVoltage(system, solution, terminals["VCCB"])) - ground
	path := "devices." + device.Component
	var diagnostics []Diagnostic
	for _, supply := range []struct {
		name, terminal string
		voltage        float64
	}{
		{"vcca", "VCCA", vcca},
		{"vccb", "VCCB", vccb},
	} {
		tolerance := nonlinearOperatingVoltageTolerance(parameters[supply.name+"_max_v"])
		if supply.voltage > parameters[supply.name+"_max_v"]+tolerance || supply.voltage < -tolerance {
			diagnostics = append(diagnostics, Diagnostic{Path: path + "." + supply.name, Message: fmt.Sprintf("direction-controlled translator %s %.12g V is outside catalog-backed range 0..%.12g V", supply.terminal, supply.voltage, parameters[supply.name+"_max_v"]), Suggestion: "adjust the supply or select a compatible reviewed translator"})
		}
	}
	fullyPowered := vcca+nonlinearOperatingVoltageTolerance(parameters["vcca_min_v"]) >= parameters["vcca_min_v"] &&
		vccb+nonlinearOperatingVoltageTolerance(parameters["vccb_min_v"]) >= parameters["vccb_min_v"]
	if !allowPowerTransition && !fullyPowered {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply", Message: "direction-controlled translator requires both supplies for active operation", Suggestion: "provide both operating supplies or analyze an explicit partial-power-down transition"})
	}
	if !directionControlledTranslatorEnabled(compiled, &system, solution) {
		return diagnostics
	}
	for group, channel := range []int{1, 5} {
		if _, _, _, valid := directionControlledTranslatorDirection(compiled, &system, solution, channel); !valid {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.dir%d", path, group+1), Message: "direction-control input is between catalog-backed logic thresholds", Suggestion: "drive the direction control to a valid VCCA-referenced logic level while outputs are disabled"})
		}
	}
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		_, _, _, directionValid := directionControlledTranslatorDirection(compiled, &system, solution, channel)
		if !directionValid {
			continue
		}
		if _, valid := directionControlledTranslatorInputState(compiled, &system, solution, channel); !valid {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.channel_%d_input", path, channel), Message: "direction-controlled translator input is between catalog-backed logic thresholds", Suggestion: "provide a valid digital input level"})
			continue
		}
		output, reference, resistance, active := directionControlledTranslatorOutputState(compiled, &system, solution, channel)
		if !active {
			continue
		}
		current := math.Abs((real(solvedNodeVoltage(system, solution, output)) - real(solvedNodeVoltage(system, solution, reference))) / resistance)
		high, _ := directionControlledTranslatorInputState(compiled, &system, solution, channel)
		limit := parameters["max_sink_current_a"]
		if high {
			limit = parameters["max_source_current_a"]
		}
		if current > limit {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.channel_%d_current", path, channel), Message: fmt.Sprintf("direction-controlled translator channel current %.12g A exceeds catalog-backed limit %.12g A", current, limit), Suggestion: "increase destination impedance or select a suitably rated reviewed translator"})
		}
	}
	return diagnostics
}

func directionControlledTranslatorDissipation(device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if device.PrimitiveModel != PrimitiveDirectionControlledTranslatorV1 {
		return 0, false
	}
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	ground := real(solvedNodeVoltage(system, solution, terminals["GND"]))
	vcca := real(solvedNodeVoltage(system, solution, terminals["VCCA"])) - ground
	vccb := real(solvedNodeVoltage(system, solution, terminals["VCCB"])) - ground
	vccaCurrent, _ := boundedSupplyLoadCurrentAndGradient(vcca, parameters["vcca_min_v"], parameters["vcca_quiescent_current_a"])
	vccbCurrent, _ := boundedSupplyLoadCurrentAndGradient(vccb, parameters["vccb_min_v"], parameters["vccb_quiescent_current_a"])
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	dissipation := math.Abs(vcca*vccaCurrent) + math.Abs(vccb*vccbCurrent)
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		output, reference, resistance, active := directionControlledTranslatorOutputState(compiled, &system, solution, channel)
		if !active {
			continue
		}
		delta := real(solvedNodeVoltage(system, solution, output)) - real(solvedNodeVoltage(system, solution, reference))
		dissipation += delta * delta / resistance
	}
	return dissipation, true
}
