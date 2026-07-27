package simmodel

import (
	"fmt"
	"math"
)

const pushPullTranslatorChannels = 4

func pushPullTranslatorDirection(device compiledNonlinearDevice) (inputPrefix, outputPrefix, outputSupply string) {
	if device.parameters["direction"] < 0 {
		return "B", "A", "VCCA"
	}
	return "A", "B", "VCCB"
}

func pushPullTranslatorInputHeadroom(parameters map[string]float64, inputPrefix string) float64 {
	name := "input_high_headroom_a_v"
	if inputPrefix == "B" {
		name = "input_high_headroom_b_v"
	}
	if headroom := parameters[name]; headroom > 0 {
		return headroom
	}
	return parameters["input_high_headroom_v"]
}

func pushPullTranslatorEnabled(device compiledNonlinearDevice, system *mnaSystem, solution []complex128) bool {
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	vcca := nonlinearNodeVoltage(system, solution, device.terminals["VCCA"]) - ground
	vccb := nonlinearNodeVoltage(system, solution, device.terminals["VCCB"]) - ground
	oe := nonlinearNodeVoltage(system, solution, device.terminals["OE"]) - ground
	return vcca >= device.parameters["vcca_min_v"] &&
		vccb >= device.parameters["vccb_min_v"] &&
		oe >= device.parameters["enable_high_ratio"]*vcca
}

func pushPullTranslatorInputState(device compiledNonlinearDevice, system *mnaSystem, solution []complex128, channel int) (high, valid bool) {
	inputPrefix, _, _ := pushPullTranslatorDirection(device)
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	inputSupply := nonlinearNodeVoltage(system, solution, device.terminals["VCCA"]) - ground
	if inputPrefix == "B" {
		inputSupply = nonlinearNodeVoltage(system, solution, device.terminals["VCCB"]) - ground
	}
	input := nonlinearNodeVoltage(system, solution, device.terminals[fmt.Sprintf("%s%d", inputPrefix, channel)]) - ground
	if input <= device.parameters["input_low_max_v"] {
		return false, true
	}
	if input >= inputSupply-pushPullTranslatorInputHeadroom(device.parameters, inputPrefix) {
		return true, true
	}
	return false, false
}

func stampNonlinearPushPullTranslator(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	_, outputPrefix, outputSupply := pushPullTranslatorDirection(device)
	enabled := pushPullTranslatorEnabled(device, system, guess)
	for channel := 1; channel <= pushPullTranslatorChannels; channel++ {
		output := device.terminals[fmt.Sprintf("%s%d", outputPrefix, channel)]
		resistance := device.parameters["output_off_resistance_ohm"]
		reference := device.terminals["GND"]
		if high, valid := pushPullTranslatorInputState(device, system, guess, channel); enabled && valid {
			resistance = device.parameters["output_low_resistance_ohm"]
			if high {
				reference = device.terminals[outputSupply]
				resistance = device.parameters["output_high_resistance_ohm"]
			}
		}
		stampAdmittance(system, output, reference, complex(1/resistance, 0))
	}
	stampBoundedSupplyLoad(system, device.terminals["VCCA"], device.terminals["GND"], device.parameters["vcca_min_v"], device.parameters["vcca_quiescent_current_a"], guess)
	stampBoundedSupplyLoad(system, device.terminals["VCCB"], device.terminals["GND"], device.parameters["vccb_min_v"], device.parameters["vccb_quiescent_current_a"], guess)
}

func addPushPullTranslatorResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	_, outputPrefix, outputSupply := pushPullTranslatorDirection(device)
	enabled := pushPullTranslatorEnabled(device, &base, solution)
	for channel := 1; channel <= pushPullTranslatorChannels; channel++ {
		output := device.terminals[fmt.Sprintf("%s%d", outputPrefix, channel)]
		reference := device.terminals["GND"]
		resistance := device.parameters["output_off_resistance_ohm"]
		if high, valid := pushPullTranslatorInputState(device, &base, solution, channel); enabled && valid {
			resistance = device.parameters["output_low_resistance_ohm"]
			if high {
				reference = device.terminals[outputSupply]
				resistance = device.parameters["output_high_resistance_ohm"]
			}
		}
		current := (nonlinearNodeVoltage(&base, solution, output) - nonlinearNodeVoltage(&base, solution, reference)) / resistance
		if index, exists := base.nodeIndex[output]; exists {
			residuals[index] += complex(current, 0)
		}
		if index, exists := base.nodeIndex[reference]; exists {
			residuals[index] -= complex(current, 0)
		}
	}
	addBoundedSupplyLoadResidual(residuals, base, device.terminals["VCCA"], device.terminals["GND"], device.parameters["vcca_min_v"], device.parameters["vcca_quiescent_current_a"], solution)
	addBoundedSupplyLoadResidual(residuals, base, device.terminals["VCCB"], device.terminals["GND"], device.parameters["vccb_min_v"], device.parameters["vccb_quiescent_current_a"], solution)
}

func pushPullTranslatorMaximumOutput(device compiledNonlinearDevice, system mnaSystem, solution []complex128) (float64, float64) {
	_, outputPrefix, outputSupply := pushPullTranslatorDirection(device)
	enabled := pushPullTranslatorEnabled(device, &system, solution)
	maximumVoltage, maximumCurrent := 0.0, 0.0
	for channel := 1; channel <= pushPullTranslatorChannels; channel++ {
		output := device.terminals[fmt.Sprintf("%s%d", outputPrefix, channel)]
		reference := device.terminals["GND"]
		resistance := device.parameters["output_off_resistance_ohm"]
		if high, valid := pushPullTranslatorInputState(device, &system, solution, channel); enabled && valid {
			resistance = device.parameters["output_low_resistance_ohm"]
			if high {
				reference = device.terminals[outputSupply]
				resistance = device.parameters["output_high_resistance_ohm"]
			}
		}
		delta := nonlinearNodeVoltage(&system, solution, output) - nonlinearNodeVoltage(&system, solution, reference)
		if math.Abs(delta) > math.Abs(maximumVoltage) {
			maximumVoltage = delta
		}
		maximumCurrent = math.Max(maximumCurrent, math.Abs(delta/resistance))
	}
	return maximumVoltage, maximumCurrent
}

func validatePushPullTranslatorOperatingLimits(device ResolvedDevice, system mnaSystem, solution []complex128, allowPowerTransition, allowTransientRatings bool) []Diagnostic {
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	ground := real(solvedNodeVoltage(system, solution, terminals["GND"]))
	vcca := real(solvedNodeVoltage(system, solution, terminals["VCCA"])) - ground
	vccb := real(solvedNodeVoltage(system, solution, terminals["VCCB"])) - ground
	path := "devices." + device.Component
	var diagnostics []Diagnostic
	vccaTolerance := nonlinearOperatingVoltageTolerance(parameters["vcca_max_v"])
	vccbTolerance := nonlinearOperatingVoltageTolerance(parameters["vccb_max_v"])
	if vcca > parameters["vcca_max_v"]+vccaTolerance || vcca < -vccaTolerance {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".vcca", Message: fmt.Sprintf("push-pull translator VCCA %.12g V is outside catalog-backed range 0..%.12g V", vcca, parameters["vcca_max_v"]), Suggestion: "adjust the lower-voltage supply or select a compatible reviewed translator"})
	}
	if vccb > parameters["vccb_max_v"]+vccbTolerance || vccb < -vccbTolerance {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".vccb", Message: fmt.Sprintf("push-pull translator VCCB %.12g V is outside catalog-backed range 0..%.12g V", vccb, parameters["vccb_max_v"]), Suggestion: "adjust the higher-voltage supply or select a compatible reviewed translator"})
	}
	fullyPowered := vcca+vccaTolerance >= parameters["vcca_min_v"] && vccb+vccbTolerance >= parameters["vccb_min_v"]
	if !allowPowerTransition && !fullyPowered {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply", Message: fmt.Sprintf("push-pull translator supplies %.12g V/%.12g V are below catalog-backed operating minima %.12g V/%.12g V", vcca, vccb, parameters["vcca_min_v"], parameters["vccb_min_v"]), Suggestion: "provide both operating supplies or analyze an explicit partial-power-down transition"})
	}
	if fullyPowered && vcca > vccb+math.Max(vccaTolerance, vccbTolerance) {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply_order", Message: fmt.Sprintf("push-pull translator VCCA %.12g V exceeds VCCB %.12g V", vcca, vccb), Suggestion: "bind the lower-voltage domain to VCCA and the higher-voltage domain to VCCB"})
	}
	if pushPullTranslatorEnabled(compiled, &system, solution) {
		inputPrefix, _, _ := pushPullTranslatorDirection(compiled)
		inputSupply := vcca
		if inputPrefix == "B" {
			inputSupply = vccb
		}
		for channel := 1; channel <= pushPullTranslatorChannels; channel++ {
			input := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("%s%d", inputPrefix, channel)])) - ground
			if _, valid := pushPullTranslatorInputState(compiled, &system, solution, channel); !valid {
				headroom := pushPullTranslatorInputHeadroom(parameters, inputPrefix)
				diagnostics = append(diagnostics, Diagnostic{
					Path:       fmt.Sprintf("%s.channel_%d_input", path, channel),
					Message:    fmt.Sprintf("push-pull translator input %.12g V is between catalog-backed logic thresholds %.12g..%.12g V", input, parameters["input_low_max_v"], inputSupply-headroom),
					Suggestion: "provide a valid digital input level or select a translator with compatible thresholds",
				})
			}
		}
		_, outputPrefix, outputSupply := pushPullTranslatorDirection(compiled)
		for channel := 1; channel <= pushPullTranslatorChannels; channel++ {
			high, valid := pushPullTranslatorInputState(compiled, &system, solution, channel)
			if !valid {
				continue
			}
			output := terminals[fmt.Sprintf("%s%d", outputPrefix, channel)]
			reference := terminals["GND"]
			resistance := parameters["output_low_resistance_ohm"]
			limit := parameters["max_sink_current_a"]
			if high {
				reference = terminals[outputSupply]
				resistance = parameters["output_high_resistance_ohm"]
				limit = parameters["max_source_current_a"]
				if transient := parameters["max_transient_source_current_a"]; allowTransientRatings && transient > limit {
					limit = transient
				}
			} else if transient := parameters["max_transient_sink_current_a"]; allowTransientRatings && transient > limit {
				limit = transient
			}
			current := math.Abs((real(solvedNodeVoltage(system, solution, output)) - real(solvedNodeVoltage(system, solution, reference))) / resistance)
			if current > limit {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.channel_%d_current", path, channel), Message: fmt.Sprintf("push-pull translator channel current %.12g A exceeds catalog-backed limit %.12g A", current, limit), Suggestion: "increase destination impedance or select a suitably rated reviewed translator"})
			}
		}
	}
	return diagnostics
}

func pushPullTranslatorDissipation(device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if device.PrimitiveModel != PrimitivePushPullTranslatorV1 {
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
	_, outputPrefix, outputSupply := pushPullTranslatorDirection(compiled)
	dissipation := math.Abs(vcca*vccaCurrent) + math.Abs(vccb*vccbCurrent)
	if pushPullTranslatorEnabled(compiled, &system, solution) {
		for channel := 1; channel <= pushPullTranslatorChannels; channel++ {
			high, valid := pushPullTranslatorInputState(compiled, &system, solution, channel)
			if !valid {
				continue
			}
			output := terminals[fmt.Sprintf("%s%d", outputPrefix, channel)]
			reference := terminals["GND"]
			resistance := parameters["output_low_resistance_ohm"]
			if high {
				reference = terminals[outputSupply]
				resistance = parameters["output_high_resistance_ohm"]
			}
			delta := real(solvedNodeVoltage(system, solution, output)) - real(solvedNodeVoltage(system, solution, reference))
			dissipation += delta * delta / resistance
		}
	}
	return dissipation, true
}
