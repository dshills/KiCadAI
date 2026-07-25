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
