package simmodel

import (
	"fmt"
	"math"
)

func translatorChannelResistance(device compiledNonlinearDevice, system *mnaSystem, solution []complex128, channel int) float64 {
	parameters := device.parameters
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	vcca := nonlinearNodeVoltage(system, solution, device.terminals["VCCA"]) - ground
	vccb := nonlinearNodeVoltage(system, solution, device.terminals["VCCB"]) - ground
	oe := nonlinearNodeVoltage(system, solution, device.terminals["OE"]) - ground
	if vcca < parameters["vcca_min_v"] || vccb < parameters["vccb_min_v"] || oe < parameters["enable_high_ratio"]*vcca {
		return parameters["channel_off_resistance_ohm"]
	}
	a := nonlinearNodeVoltage(system, solution, device.terminals[fmt.Sprintf("A%d", channel)]) - ground
	b := nonlinearNodeVoltage(system, solution, device.terminals[fmt.Sprintf("B%d", channel)]) - ground
	if math.Min(a, b) <= parameters["low_level_threshold_v"] {
		return parameters["channel_on_resistance_ohm"]
	}
	return parameters["channel_off_resistance_ohm"]
}

func stampNonlinearOpenDrainTranslator(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	for channel := 1; channel <= 2; channel++ {
		resistance := translatorChannelResistance(device, system, guess, channel)
		stampAdmittance(system, device.terminals[fmt.Sprintf("A%d", channel)], device.terminals[fmt.Sprintf("B%d", channel)], complex(1/resistance, 0))
	}
}

func addOpenDrainTranslatorResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	for channel := 1; channel <= 2; channel++ {
		a := device.terminals[fmt.Sprintf("A%d", channel)]
		b := device.terminals[fmt.Sprintf("B%d", channel)]
		resistance := translatorChannelResistance(device, &base, solution, channel)
		current := (nonlinearNodeVoltage(&base, solution, a) - nonlinearNodeVoltage(&base, solution, b)) / resistance
		if index, exists := base.nodeIndex[a]; exists {
			residuals[index] += complex(current, 0)
		}
		if index, exists := base.nodeIndex[b]; exists {
			residuals[index] -= complex(current, 0)
		}
	}
}

func validateOpenDrainTranslatorOperatingLimits(device ResolvedDevice, system mnaSystem, solution []complex128, allowPowerTransition bool) []Diagnostic {
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	ground := real(solvedNodeVoltage(system, solution, terminals["GND"]))
	vcca := real(solvedNodeVoltage(system, solution, terminals["VCCA"])) - ground
	vccb := real(solvedNodeVoltage(system, solution, terminals["VCCB"])) - ground
	oe := real(solvedNodeVoltage(system, solution, terminals["OE"])) - ground
	path := "devices." + device.Component
	var diagnostics []Diagnostic
	vccaTolerance := 1e-9 * math.Max(1, math.Abs(parameters["vcca_max_v"]))
	vccbTolerance := 1e-9 * math.Max(1, math.Abs(parameters["vccb_max_v"]))
	if vcca > parameters["vcca_max_v"]+vccaTolerance || vcca < -vccaTolerance {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".vcca", Message: fmt.Sprintf("translator VCCA %.12g V is outside catalog-backed range 0..%.12g V", vcca, parameters["vcca_max_v"]), Suggestion: "adjust supply conditions or select a compatible reviewed translator"})
	}
	if vccb > parameters["vccb_max_v"]+vccbTolerance || vccb < -vccbTolerance {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".vccb", Message: fmt.Sprintf("translator VCCB %.12g V is outside catalog-backed range 0..%.12g V", vccb, parameters["vccb_max_v"]), Suggestion: "adjust supply conditions or select a compatible reviewed translator"})
	}
	fullyPowered := vcca+vccaTolerance >= parameters["vcca_min_v"] && vccb+vccbTolerance >= parameters["vccb_min_v"]
	if !allowPowerTransition && !fullyPowered {
		diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply", Message: fmt.Sprintf("translator supplies %.12g V/%.12g V are below catalog-backed operating minima %.12g V/%.12g V", vcca, vccb, parameters["vcca_min_v"], parameters["vccb_min_v"]), Suggestion: "provide both operating supplies or use the explicit partial-power-down state"})
	}
	if fullyPowered {
		if vcca > vccb+1e-9 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply_order", Message: fmt.Sprintf("translator VCCA %.12g V exceeds VCCB %.12g V", vcca, vccb), Suggestion: "bind the lower-voltage domain to VCCA and the higher-voltage domain to VCCB"})
		}
		if oe < parameters["enable_high_ratio"]*vcca {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".enable", Message: fmt.Sprintf("translator OE %.12g V is below the catalog-backed enable threshold %.12g V", oe, parameters["enable_high_ratio"]*vcca), Suggestion: "drive OE from a valid VCCA-referenced enable source"})
		}
	}
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	for channel := 1; channel <= 2; channel++ {
		a := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("A%d", channel)]))
		b := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("B%d", channel)]))
		current := math.Abs((a - b) / translatorChannelResistance(compiled, &system, solution, channel))
		if current > parameters["max_channel_current_a"] {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.channel_%d_current", path, channel), Message: fmt.Sprintf("translator channel current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_channel_current_a"]), Suggestion: "increase pull-up or driver impedance or select a suitably rated reviewed translator"})
		}
	}
	return diagnostics
}

func openDrainTranslatorDissipation(device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if device.PrimitiveModel != PrimitiveBidirectionalOpenDrainTranslatorV1 {
		return 0, false
	}
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	ground := real(solvedNodeVoltage(system, solution, terminals["GND"]))
	vcca := real(solvedNodeVoltage(system, solution, terminals["VCCA"])) - ground
	vccb := real(solvedNodeVoltage(system, solution, terminals["VCCB"])) - ground
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	dissipation := math.Abs(vcca)*parameters["vcca_quiescent_current_a"] + math.Abs(vccb)*parameters["vccb_quiescent_current_a"]
	for channel := 1; channel <= 2; channel++ {
		a := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("A%d", channel)]))
		b := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("B%d", channel)]))
		resistance := translatorChannelResistance(compiled, &system, solution, channel)
		dissipation += (a - b) * (a - b) / resistance
	}
	return dissipation, true
}
