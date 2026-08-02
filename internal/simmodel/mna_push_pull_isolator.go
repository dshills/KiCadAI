package simmodel

import (
	"fmt"
	"math"
)

type pushPullIsolatorChannel struct {
	input, output, inputSupply, inputGround, outputSupply, outputGround, enable string
}

var pushPullIsolatorChannels = []pushPullIsolatorChannel{
	{input: "INA1", output: "OUTB1", inputSupply: "VDD1", inputGround: "GND1", outputSupply: "VDD2", outputGround: "GND2", enable: "EN2"},
	{input: "INA2", output: "OUTB2", inputSupply: "VDD1", inputGround: "GND1", outputSupply: "VDD2", outputGround: "GND2", enable: "EN2"},
	{input: "INA3", output: "OUTB3", inputSupply: "VDD1", inputGround: "GND1", outputSupply: "VDD2", outputGround: "GND2", enable: "EN2"},
	{input: "INB4", output: "OUTA4", inputSupply: "VDD2", inputGround: "GND2", outputSupply: "VDD1", outputGround: "GND1", enable: "EN1"},
}

func pushPullIsolatorOutputState(device compiledNonlinearDevice, system *mnaSystem, solution []complex128, channel pushPullIsolatorChannel) (reference string, resistance float64, active bool, valid bool) {
	inputGround := nonlinearNodeVoltage(system, solution, device.terminals[channel.inputGround])
	outputGround := nonlinearNodeVoltage(system, solution, device.terminals[channel.outputGround])
	inputSupply := nonlinearNodeVoltage(system, solution, device.terminals[channel.inputSupply]) - inputGround
	outputSupply := nonlinearNodeVoltage(system, solution, device.terminals[channel.outputSupply]) - outputGround
	enable := nonlinearNodeVoltage(system, solution, device.terminals[channel.enable]) - outputGround
	if outputSupply < device.parameters["supply_min_v"] {
		return channel.outputGround, device.parameters["output_off_resistance_ohm"], false, true
	}
	if enable < device.parameters["enable_high_ratio"]*outputSupply {
		return channel.outputGround, device.parameters["output_off_resistance_ohm"], false, true
	}
	if inputSupply < device.parameters["supply_min_v"] {
		return channel.outputGround, device.parameters["output_low_resistance_ohm"], true, true
	}
	input := nonlinearNodeVoltage(system, solution, device.terminals[channel.input]) - inputGround
	switch {
	case input <= device.parameters["input_low_ratio"]*inputSupply:
		return channel.outputGround, device.parameters["output_low_resistance_ohm"], true, true
	case input >= device.parameters["input_high_ratio"]*inputSupply:
		return channel.outputSupply, device.parameters["output_high_resistance_ohm"], true, true
	default:
		return channel.outputGround, device.parameters["output_low_resistance_ohm"], true, false
	}
}

func stampNonlinearPushPullIsolator(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	for _, channel := range pushPullIsolatorChannels {
		reference, resistance, _, _ := pushPullIsolatorOutputState(device, system, guess, channel)
		stampAdmittance(system, device.terminals[channel.output], device.terminals[reference], complex(1/resistance, 0))
	}
	for _, side := range []string{"1", "2"} {
		stampBoundedSupplyLoad(system, device.terminals["VDD"+side], device.terminals["GND"+side], device.parameters["supply_min_v"], device.parameters["side_"+side+"_quiescent_current_a"], guess)
	}
}

func addPushPullIsolatorResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	for _, channel := range pushPullIsolatorChannels {
		reference, resistance, _, _ := pushPullIsolatorOutputState(device, &base, solution, channel)
		output := device.terminals[channel.output]
		referenceNode := device.terminals[reference]
		current := (nonlinearNodeVoltage(&base, solution, output) - nonlinearNodeVoltage(&base, solution, referenceNode)) / resistance
		if index, exists := base.nodeIndex[output]; exists {
			residuals[index] += complex(current, 0)
		}
		if index, exists := base.nodeIndex[referenceNode]; exists {
			residuals[index] -= complex(current, 0)
		}
	}
	for _, side := range []string{"1", "2"} {
		addBoundedSupplyLoadResidual(residuals, base, device.terminals["VDD"+side], device.terminals["GND"+side], device.parameters["supply_min_v"], device.parameters["side_"+side+"_quiescent_current_a"], solution)
	}
}

func pushPullIsolatorMaximumOutput(device compiledNonlinearDevice, system mnaSystem, solution []complex128) (float64, float64) {
	maximumVoltage, maximumCurrent := 0.0, 0.0
	for _, channel := range pushPullIsolatorChannels {
		reference, resistance, _, _ := pushPullIsolatorOutputState(device, &system, solution, channel)
		delta := nonlinearNodeVoltage(&system, solution, device.terminals[channel.output]) - nonlinearNodeVoltage(&system, solution, device.terminals[reference])
		if math.Abs(delta) > math.Abs(maximumVoltage) {
			maximumVoltage = delta
		}
		maximumCurrent = math.Max(maximumCurrent, math.Abs(delta/resistance))
	}
	return maximumVoltage, maximumCurrent
}

func validatePushPullIsolatorOperatingLimits(device ResolvedDevice, system mnaSystem, solution []complex128, allowPowerTransition bool) []Diagnostic {
	parameters := deviceParameterMap(device)
	terminals := terminalMap(device)
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	path := "devices." + device.Component
	var diagnostics []Diagnostic
	for _, side := range []string{"1", "2"} {
		supply := real(solvedNodeVoltage(system, solution, terminals["VDD"+side]) - solvedNodeVoltage(system, solution, terminals["GND"+side]))
		tolerance := nonlinearOperatingVoltageTolerance(parameters["supply_max_v"])
		if supply > parameters["supply_max_v"]+tolerance || supply < -tolerance {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply_" + side, Message: fmt.Sprintf("push-pull isolator side %s supply %.12g V is outside catalog-backed range", side, supply), Suggestion: "adjust the isolated-domain supply or select a compatible reviewed isolator"})
		}
		if !allowPowerTransition && supply+tolerance < parameters["supply_min_v"] {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply_" + side, Message: fmt.Sprintf("push-pull isolator side %s supply is below its operating minimum", side), Suggestion: "provide the operating supply or analyze an explicit supply-loss transition"})
		}
	}
	for index, channel := range pushPullIsolatorChannels {
		_, resistance, active, valid := pushPullIsolatorOutputState(compiled, &system, solution, channel)
		if active && !valid {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.channel_%d_input", path, index+1), Message: "push-pull isolator input is between catalog-backed logic thresholds", Suggestion: "provide a valid source-domain logic level"})
		}
		if !active {
			continue
		}
		reference, _, _, _ := pushPullIsolatorOutputState(compiled, &system, solution, channel)
		current := math.Abs((real(solvedNodeVoltage(system, solution, terminals[channel.output])) - real(solvedNodeVoltage(system, solution, terminals[reference]))) / resistance)
		if current > parameters["max_output_current_a"] {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.channel_%d_current", path, index+1), Message: fmt.Sprintf("push-pull isolator output current %.12g A exceeds catalog-backed maximum %.12g A", current, parameters["max_output_current_a"]), Suggestion: "increase destination impedance or select a suitably rated reviewed isolator"})
		}
	}
	return diagnostics
}

func pushPullIsolatorDissipation(device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if device.PrimitiveModel != PrimitivePushPullDigitalIsolatorV1 {
		return 0, false
	}
	parameters := deviceParameterMap(device)
	terminals := terminalMap(device)
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	dissipation := 0.0
	for _, side := range []string{"1", "2"} {
		supply := real(solvedNodeVoltage(system, solution, terminals["VDD"+side]) - solvedNodeVoltage(system, solution, terminals["GND"+side]))
		current, _ := boundedSupplyLoadCurrentAndGradient(supply, parameters["supply_min_v"], parameters["side_"+side+"_quiescent_current_a"])
		dissipation += math.Abs(supply * current)
	}
	for _, channel := range pushPullIsolatorChannels {
		reference, resistance, active, _ := pushPullIsolatorOutputState(compiled, &system, solution, channel)
		if !active {
			continue
		}
		delta := real(solvedNodeVoltage(system, solution, terminals[channel.output]) - solvedNodeVoltage(system, solution, terminals[reference]))
		dissipation += delta * delta / resistance
	}
	return dissipation, true
}
