package simmodel

import (
	"fmt"
	"math"
)

func validateResolvedOperatingLimits(plan Plan, system mnaSystem, solution []complex128, allowPowerTransition bool) []Diagnostic {
	var diagnostics []Diagnostic
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveFuseClosedStateV1 {
			parameters := namedValueMap(device.ModelParameters)
			terminals := terminalMap(device)
			voltage := math.Abs(real(solvedNodeVoltage(system, solution, terminals["A"]) - solvedNodeVoltage(system, solution, terminals["B"])))
			current := voltage / parameters["cold_resistance_ohm"]
			path := "devices." + device.Component + ".operating_limit"
			if voltage > parameters["max_voltage_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("fuse voltage %.12g V exceeds catalog-backed limit %.12g V", voltage, parameters["max_voltage_v"]), Suggestion: "reduce the applied voltage or select a suitably rated reviewed fuse"})
			}
			if current > parameters["rated_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("fuse current %.12g A exceeds the reviewed closed-state model limit %.12g A", current, parameters["rated_current_a"]), Suggestion: "reduce steady-state current, select a suitably rated reviewed fuse, or use a registered time-current clearing model"})
			}
			continue
		}
		if device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1 {
			parameters := namedValueMap(device.ModelParameters)
			terminals := terminalMap(device)
			groundA := real(solvedNodeVoltage(system, solution, terminals["GND_A"]))
			groundB := real(solvedNodeVoltage(system, solution, terminals["GND_B"]))
			ground := (groundA + groundB) / 2
			supply := real(solvedNodeVoltage(system, solution, terminals["VCC"])) - ground
			inputPlus := real(solvedNodeVoltage(system, solution, terminals["IN_PLUS"]))
			inputMinus := real(solvedNodeVoltage(system, solution, terminals["IN_MINUS"]))
			commonMode := (inputPlus+inputMinus)/2 - ground
			output := real(solvedNodeVoltage(system, solution, terminals["OUT"])) - ground
			path := "devices." + device.Component
			powerTransition := allowPowerTransition && supply < parameters["supply_min_v"]
			if !powerTransition && (supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"]) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".supply", Message: fmt.Sprintf("current-sense amplifier supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust the source conditions or select a compatible reviewed current sensor"})
			}
			if !powerTransition && (commonMode < parameters["common_mode_min_v"] || commonMode > parameters["common_mode_max_v"]) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".common_mode", Message: fmt.Sprintf("current-sense amplifier common-mode voltage %.12g V is outside catalog-backed range %.12g..%.12g V", commonMode, parameters["common_mode_min_v"], parameters["common_mode_max_v"]), Suggestion: "adjust the sensed rail or select a compatible reviewed current sensor"})
			}
			minimumOutput := parameters["output_low_margin_v"]
			maximumOutput := supply - parameters["output_high_margin_v"]
			if !powerTransition && (output < minimumOutput || output > maximumOutput) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".output", Message: fmt.Sprintf("current-sense amplifier output %.12g V is outside catalog-backed range %.12g..%.12g V", output, minimumOutput, maximumOutput), Suggestion: "reduce shunt voltage or gain, adjust reference bias, or select a compatible reviewed current sensor"})
			}
			continue
		}
		if device.PrimitiveModel == PrimitiveFloatingAdjustableRegulatorV1 {
			parameters := namedValueMap(device.ModelParameters)
			terminals := terminalMap(device)
			input := real(solvedNodeVoltage(system, solution, terminals["VIN"]))
			output := real(solvedNodeVoltage(system, solution, terminals["VOUT"]))
			differential := math.Abs(input - output)
			path := "devices." + device.Component
			powerTransition := allowPowerTransition && differential < parameters["min_headroom_v"]
			if !powerTransition && differential < parameters["min_headroom_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".headroom", Message: fmt.Sprintf("floating adjustable regulator headroom %.12g V is below catalog-backed minimum %.12g V", differential, parameters["min_headroom_v"]), Suggestion: "increase input-output differential or select a lower-dropout reviewed regulator"})
			}
			if differential > parameters["max_input_output_voltage_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".input_output_voltage", Message: fmt.Sprintf("floating adjustable regulator input-output differential %.12g V exceeds catalog-backed maximum %.12g V", differential, parameters["max_input_output_voltage_v"]), Suggestion: "reduce input-output differential or select a compatible reviewed regulator"})
			}
			branch, exists := system.branchIndex[device.Component]
			if !exists || branch >= len(solution) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".output_current", Message: "floating adjustable regulator output-current branch is absent from the solved topology"})
			} else if current := math.Abs(real(solution[branch])); !powerTransition && current > parameters["max_load_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".output_current", Message: fmt.Sprintf("floating adjustable regulator load current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_load_current_a"]), Suggestion: "reduce load current or select a compatible reviewed regulator"})
			}
			continue
		}
		if device.PrimitiveModel == PrimitiveDualOutputIsolatedConverterV1 {
			parameters := namedValueMap(device.ModelParameters)
			terminals := terminalMap(device)
			input := real(solvedNodeVoltage(system, solution, terminals["VIN_PLUS"]) - solvedNodeVoltage(system, solution, terminals["VIN_MINUS"]))
			path := "devices." + device.Component
			powerTransition := allowPowerTransition && input < parameters["input_min_v"]
			if !powerTransition && (input < parameters["input_min_v"] || input > parameters["input_max_v"]) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".input_voltage", Message: fmt.Sprintf("dual-output isolated converter input %.12g V is outside catalog-backed range %.12g..%.12g V", input, parameters["input_min_v"], parameters["input_max_v"]), Suggestion: "adjust the source conditions or select a compatible reviewed isolated converter"})
			}
			for _, output := range []struct {
				terminal string
				limit    string
				label    string
			}{
				{terminal: "VOUT_PLUS", limit: "positive_max_output_current_a", label: "positive"},
				{terminal: "VOUT_MINUS", limit: "negative_max_output_current_a", label: "negative"},
			} {
				branch, exists := system.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: output.terminal}]
				if !exists || branch >= len(solution) {
					diagnostics = append(diagnostics, Diagnostic{Path: path + "." + output.label + "_output_current", Message: "dual-output isolated converter output-current branch is absent from the solved topology"})
					continue
				}
				if current := math.Abs(real(solution[branch])); !powerTransition && current > parameters[output.limit] {
					diagnostics = append(diagnostics, Diagnostic{Path: path + "." + output.label + "_output_current", Message: fmt.Sprintf("dual-output isolated converter %s output current %.12g A exceeds catalog-backed limit %.12g A", output.label, current, parameters[output.limit]), Suggestion: "reduce load current or select a compatible reviewed isolated converter"})
				}
			}
			continue
		}
		if device.PrimitiveModel != PrimitiveAdjustableLinearRegulatorV1 && device.PrimitiveModel != PrimitiveFixedLinearRegulatorV1 {
			continue
		}
		parameters := namedValueMap(device.ModelParameters)
		terminals := terminalMap(device)
		ground := real(solvedNodeVoltage(system, solution, terminals["GND"]))
		input := real(solvedNodeVoltage(system, solution, terminals["VIN"])) - ground
		output := real(solvedNodeVoltage(system, solution, terminals["VOUT"])) - ground
		path := "devices." + device.Component
		powerTransition := allowPowerTransition && input < parameters["min_input_voltage_v"]
		if !powerTransition && (input < parameters["min_input_voltage_v"] || input > parameters["max_input_voltage_v"]) {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       path + ".input_voltage",
				Message:    fmt.Sprintf("regulator input %.12g V is outside catalog-backed range %.12g..%.12g V", input, parameters["min_input_voltage_v"], parameters["max_input_voltage_v"]),
				Suggestion: "adjust the source conditions or select a compatible reviewed regulator",
			})
		}
		headroom := input - output
		if !powerTransition && headroom < parameters["min_headroom_v"] {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       path + ".headroom",
				Message:    fmt.Sprintf("regulator headroom %.12g V is below catalog-backed minimum %.12g V", headroom, parameters["min_headroom_v"]),
				Suggestion: "increase input voltage, reduce the requested output, or select a lower-dropout reviewed regulator",
			})
		}
		branch, exists := system.branchIndex[device.Component]
		if !exists || branch >= len(solution) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".output_current", Message: "regulator output-current branch is absent from the solved topology"})
			continue
		}
		current := math.Abs(real(solution[branch]))
		if !powerTransition && current > parameters["max_load_current_a"] {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       path + ".output_current",
				Message:    fmt.Sprintf("regulator load current %.12g A at input %.12g V and output %.12g V exceeds catalog-backed limit %.12g A", current, input, output, parameters["max_load_current_a"]),
				Suggestion: "reduce load current or select a compatible reviewed regulator",
			})
		}
	}
	return diagnostics
}

func adjustableLinearRegulatorDissipation(device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if device.PrimitiveModel != PrimitiveAdjustableLinearRegulatorV1 && device.PrimitiveModel != PrimitiveFixedLinearRegulatorV1 && device.PrimitiveModel != PrimitiveFloatingAdjustableRegulatorV1 {
		return 0, false
	}
	branch, exists := system.branchIndex[device.Component]
	if !exists || branch >= len(solution) {
		return 0, true
	}
	terminals := terminalMap(device)
	if device.PrimitiveModel == PrimitiveFloatingAdjustableRegulatorV1 {
		input := real(solvedNodeVoltage(system, solution, terminals["VIN"]))
		output := real(solvedNodeVoltage(system, solution, terminals["VOUT"]))
		adjust := real(solvedNodeVoltage(system, solution, terminals["ADJ"]))
		loadCurrent := math.Abs(real(solution[branch]))
		quiescent := namedValueMap(device.ModelParameters)["quiescent_current_a"]
		return math.Abs(input-output)*loadCurrent + math.Abs(input-adjust)*quiescent, true
	}
	ground := real(solvedNodeVoltage(system, solution, terminals["GND"]))
	input := real(solvedNodeVoltage(system, solution, terminals["VIN"])) - ground
	output := real(solvedNodeVoltage(system, solution, terminals["VOUT"])) - ground
	loadCurrent := math.Abs(real(solution[branch]))
	quiescent := namedValueMap(device.ModelParameters)["quiescent_current_a"]
	return math.Abs(input-output)*loadCurrent + math.Abs(input)*quiescent, true
}
