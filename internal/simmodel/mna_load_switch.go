package simmodel

import (
	"fmt"
	"math"
)

// reverseBlockingLoadSwitchConductance is a bounded, catalog-parameterized
// compact model for an active-high load switch with full-time reverse-current
// blocking. Forward conduction is available only with a valid input supply and
// asserted enable. Once VOUT rises above VIN by the reviewed release threshold,
// the switch presents a leakage-bounded off resistance instead of conducting
// backward through an ordinary bidirectional resistor.
func reverseBlockingLoadSwitchConductance(device compiledNonlinearDevice, system *mnaSystem, solution []complex128) float64 {
	vin := nonlinearNodeVoltage(system, solution, device.terminals["VIN"])
	vout := nonlinearNodeVoltage(system, solution, device.terminals["VOUT"])
	ground := nonlinearNodeVoltage(system, solution, device.terminals["GND"])
	enable := nonlinearNodeVoltage(system, solution, device.terminals["ON"])
	parameters := device.parameters
	offConductance := parameters["reverse_leakage_current_a"] / parameters["max_output_voltage_v"]
	if vin-ground < parameters["input_min_v"] ||
		enable-ground < parameters["enable_high_voltage_v"] ||
		vout-vin > parameters["reverse_blocking_release_voltage_v"] {
		return offConductance
	}
	return 1 / parameters["on_resistance_ohm"]
}

func stampNonlinearReverseBlockingLoadSwitch(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	stampAdmittance(
		system,
		device.terminals["VIN"],
		device.terminals["VOUT"],
		complex(reverseBlockingLoadSwitchConductance(device, system, guess), 0),
	)
}

func addReverseBlockingLoadSwitchResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	conductance := reverseBlockingLoadSwitchConductance(device, &base, solution)
	current := conductance * (nonlinearNodeVoltage(&base, solution, device.terminals["VIN"]) -
		nonlinearNodeVoltage(&base, solution, device.terminals["VOUT"]))
	if index, exists := base.nodeIndex[device.terminals["VIN"]]; exists {
		residuals[index] += complex(current, 0)
	}
	if index, exists := base.nodeIndex[device.terminals["VOUT"]]; exists {
		residuals[index] -= complex(current, 0)
	}
}

func validateReverseBlockingLoadSwitchOperatingLimits(device ResolvedDevice, system mnaSystem, solution []complex128) []Diagnostic {
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	vin := nonlinearNodeVoltage(&system, solution, terminals["VIN"])
	vout := nonlinearNodeVoltage(&system, solution, terminals["VOUT"])
	ground := nonlinearNodeVoltage(&system, solution, terminals["GND"])
	enable := nonlinearNodeVoltage(&system, solution, terminals["ON"])
	compiled := compiledNonlinearDevice{
		component:  device.Component,
		primitive:  device.PrimitiveModel,
		terminals:  terminals,
		parameters: parameters,
		polarity:   1,
	}
	conductance := reverseBlockingLoadSwitchConductance(compiled, &system, solution)
	current := math.Abs((vin - vout) * conductance)
	path := "devices." + device.Component + ".operating_limit"
	tolerance := nonlinearOperatingVoltageTolerance(parameters["max_output_voltage_v"])
	var diagnostics []Diagnostic
	if input := vin - ground; input > parameters["input_max_v"]+tolerance {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"reverse-blocking load-switch input voltage %.12g V exceeds catalog-backed limit %.12g V",
				input, parameters["input_max_v"],
			),
			Suggestion: "reduce the protected supply or select a suitably rated reviewed load switch",
		})
	}
	if output := vout - ground; output < -tolerance || output > parameters["max_output_voltage_v"]+tolerance {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"reverse-blocking load-switch output voltage %.12g V is outside catalog-backed range 0..%.12g V",
				output, parameters["max_output_voltage_v"],
			),
			Suggestion: "keep the protected output inside its reviewed range or select a suitably rated load switch",
		})
	}
	if control := enable - ground; control < -tolerance || control > parameters["input_max_v"]+tolerance {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"reverse-blocking load-switch enable voltage %.12g V is outside catalog-backed range 0..%.12g V",
				control, parameters["input_max_v"],
			),
			Suggestion: "drive the enable from a compatible logic or supply domain",
		})
	}
	if current > parameters["max_output_current_a"] {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"reverse-blocking load-switch current %.12g A exceeds catalog-backed limit %.12g A",
				current, parameters["max_output_current_a"],
			),
			Suggestion: "reduce the load or select a suitably rated reviewed load switch",
		})
	}
	return diagnostics
}

// currentLimitingEFuseCurrentAndGradient models a reviewed active-current-limit
// eFuse as a continuous piecewise-linear two-terminal device. Below the
// programmed limit it presents the catalog on-resistance. Above that limit it
// regulates forward current instead of pretending that the protected source
// can deliver an arbitrarily large surge. Disabled, unpowered, and reverse
// states retain only the catalog leakage path.
func currentLimitingEFuseCurrentAndGradient(device compiledNonlinearDevice, system *mnaSystem, solution []complex128) (float64, float64) {
	vin := nonlinearNodeVoltage(system, solution, device.terminals["VIN"])
	vout := nonlinearNodeVoltage(system, solution, device.terminals["VOUT"])
	reference := nonlinearNodeVoltage(system, solution, device.terminals["RTN"])
	enable := nonlinearNodeVoltage(system, solution, device.terminals["SHDN"])
	parameters := device.parameters
	voltage := vin - vout
	offConductance := parameters["reverse_leakage_current_a"] / parameters["max_output_voltage_v"]
	if vin-reference < parameters["input_min_v"] ||
		enable-reference < parameters["enable_high_voltage_v"] ||
		voltage <= 0 {
		return voltage * offConductance, offConductance
	}
	onConductance := 1 / parameters["on_resistance_ohm"]
	unlimited := voltage * onConductance
	if unlimited <= parameters["programmed_current_limit_a"] {
		return unlimited, onConductance
	}
	return parameters["programmed_current_limit_a"], 0
}

func stampNonlinearCurrentLimitingEFuse(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	vin, vout := device.terminals["VIN"], device.terminals["VOUT"]
	voltage := nonlinearNodeVoltage(system, guess, vin) - nonlinearNodeVoltage(system, guess, vout)
	current, gradient := currentLimitingEFuseCurrentAndGradient(device, system, guess)
	stampAdmittance(system, vin, vout, complex(gradient, 0))
	stampCurrentSource(system, vin, vout, complex(current-gradient*voltage, 0))
}

func addCurrentLimitingEFuseResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	current, _ := currentLimitingEFuseCurrentAndGradient(device, &base, solution)
	if index, exists := base.nodeIndex[device.terminals["VIN"]]; exists {
		residuals[index] += complex(current, 0)
	}
	if index, exists := base.nodeIndex[device.terminals["VOUT"]]; exists {
		residuals[index] -= complex(current, 0)
	}
}

func validateCurrentLimitingEFuseOperatingLimits(device ResolvedDevice, system mnaSystem, solution []complex128) []Diagnostic {
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	vin := nonlinearNodeVoltage(&system, solution, terminals["VIN"])
	vout := nonlinearNodeVoltage(&system, solution, terminals["VOUT"])
	reference := nonlinearNodeVoltage(&system, solution, terminals["RTN"])
	enable := nonlinearNodeVoltage(&system, solution, terminals["SHDN"])
	compiled := compiledNonlinearDevice{
		component: device.Component, primitive: device.PrimitiveModel,
		terminals: terminals, parameters: parameters, polarity: 1,
	}
	current, _ := currentLimitingEFuseCurrentAndGradient(compiled, &system, solution)
	path := "devices." + device.Component + ".operating_limit"
	tolerance := nonlinearOperatingVoltageTolerance(parameters["max_output_voltage_v"])
	var diagnostics []Diagnostic
	if input := vin - reference; input > parameters["input_max_v"]+tolerance {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"current-limiting eFuse input voltage %.12g V exceeds catalog-backed limit %.12g V",
				input, parameters["input_max_v"],
			),
			Suggestion: "reduce the protected supply or select a suitably rated reviewed eFuse",
		})
	}
	if output := vout - reference; output < -tolerance || output > parameters["max_output_voltage_v"]+tolerance {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"current-limiting eFuse output voltage %.12g V is outside catalog-backed range 0..%.12g V",
				output, parameters["max_output_voltage_v"],
			),
			Suggestion: "keep the protected output inside its reviewed range or select a suitably rated eFuse",
		})
	}
	if control := enable - reference; control < -tolerance || control > parameters["input_max_v"]+tolerance {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"current-limiting eFuse shutdown-control voltage %.12g V is outside catalog-backed range 0..%.12g V",
				control, parameters["input_max_v"],
			),
			Suggestion: "drive shutdown from a compatible control or supply domain",
		})
	}
	if math.Abs(current) > parameters["maximum_current_limit_a"]+mnaOperatingCurrentTolerance(parameters["maximum_current_limit_a"]) {
		diagnostics = append(diagnostics, Diagnostic{
			Path: path,
			Message: fmt.Sprintf(
				"current-limiting eFuse current %.12g A exceeds catalog-backed maximum limit %.12g A",
				math.Abs(current), parameters["maximum_current_limit_a"],
			),
			Suggestion: "increase the programmed resistance or select a suitably rated reviewed eFuse",
		})
	}
	return diagnostics
}
