package simmodel

import (
	"fmt"
)

// staticSupplyLoadCurrentAndGradient preserves the catalog maximum-current
// envelope once a device reaches its minimum operating voltage, while smoothly
// releasing the load as an unpowered rail approaches its local reference.
// This prevents a constant-current abstraction from manufacturing negative
// supply rails during startup without understating the powered worst case.
func staticSupplyLoadCurrentAndGradient(voltage float64, parameters map[string]float64) (float64, float64) {
	return boundedSupplyLoadCurrentAndGradient(
		voltage,
		parameters["minimum_supply_voltage_v"],
		parameters["maximum_supply_current_a"],
	)
}

func boundedSupplyLoadCurrentAndGradient(voltage, minimum, maximumCurrent float64) (float64, float64) {
	if voltage < minimum {
		conductance := maximumCurrent / minimum
		return voltage * conductance, conductance
	}
	return maximumCurrent, 0
}

func stampBoundedSupplyLoad(system *mnaSystem, power, ground string, minimum, maximumCurrent float64, guess []complex128) {
	voltage := nonlinearNodeVoltage(system, guess, power) - nonlinearNodeVoltage(system, guess, ground)
	current, conductance := boundedSupplyLoadCurrentAndGradient(voltage, minimum, maximumCurrent)
	stampAdmittance(system, power, ground, complex(conductance, 0))
	stampCurrentSource(system, power, ground, complex(current-conductance*voltage, 0))
}

func addBoundedSupplyLoadResidual(residuals []complex128, base mnaSystem, power, ground string, minimum, maximumCurrent float64, solution []complex128) {
	voltage := nonlinearNodeVoltage(&base, solution, power) - nonlinearNodeVoltage(&base, solution, ground)
	current, _ := boundedSupplyLoadCurrentAndGradient(voltage, minimum, maximumCurrent)
	if index, exists := base.nodeIndex[power]; exists {
		residuals[index] += complex(current, 0)
	}
	if index, exists := base.nodeIndex[ground]; exists {
		residuals[index] -= complex(current, 0)
	}
}

func stampNonlinearStaticSupplyLoad(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	stampBoundedSupplyLoad(
		system, device.terminals["POWER"], device.terminals["GROUND"],
		device.parameters["minimum_supply_voltage_v"], device.parameters["maximum_supply_current_a"], guess,
	)
}

func addStaticSupplyLoadResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	addBoundedSupplyLoadResidual(
		residuals, base, device.terminals["POWER"], device.terminals["GROUND"],
		device.parameters["minimum_supply_voltage_v"], device.parameters["maximum_supply_current_a"], solution,
	)
}

func validateStaticSupplyLoadOperatingLimits(device ResolvedDevice, system mnaSystem, solution []complex128, allowPowerTransition bool) []Diagnostic {
	parameters := namedValueMap(device.ModelParameters)
	terminals := terminalMap(device)
	voltage := nonlinearNodeVoltage(&system, solution, terminals["POWER"]) -
		nonlinearNodeVoltage(&system, solution, terminals["GROUND"])
	tolerance := nonlinearOperatingVoltageTolerance(parameters["maximum_supply_voltage_v"])
	path := "devices." + device.Component + ".supply"
	if voltage > parameters["maximum_supply_voltage_v"]+tolerance || voltage < -tolerance {
		return []Diagnostic{{
			Path: path,
			Message: fmt.Sprintf(
				"static supply load voltage %.12g V is outside catalog-backed range 0..%.12g V",
				voltage, parameters["maximum_supply_voltage_v"],
			),
			Suggestion: "provide a compatible supply domain or select a suitably rated reviewed device",
		}}
	}
	if !allowPowerTransition && voltage+tolerance < parameters["minimum_supply_voltage_v"] {
		return []Diagnostic{{
			Path: path,
			Message: fmt.Sprintf(
				"static supply load voltage %.12g V is below catalog-backed operating minimum %.12g V",
				voltage, parameters["minimum_supply_voltage_v"],
			),
			Suggestion: "provide the required operating rail or verify the device only during an explicit power transition",
		}}
	}
	return nil
}
