package simmodel

import (
	"fmt"
	"math"
)

var isolatedOpenDrainChannels = [][2]string{
	{"SDA1", "SDA2"},
	{"SCL1", "SCL2"},
}

func isolatedOpenDrainDriverParameter(side int, channel int) string {
	return fmt.Sprintf("resolved_driver_side_%d_channel_%d", side, channel+1)
}

func isolatedOpenDrainSourceTerminals(device ResolvedDevice) (string, string, bool) {
	terminals := terminalMap(device)
	switch device.PrimitiveModel {
	case PrimitiveVoltageSourceV1, PrimitiveCurrentSourceV1:
		return terminals["POSITIVE"], terminals["NEGATIVE"], true
	case PrimitiveConnectorVoltageSourceV1:
		return terminals["PIN_1"], terminals["PIN_2"], true
	default:
		return "", "", false
	}
}

func isolatedOpenDrainSignalDriver(plan Plan, component string, lowThresholdV float64) bool {
	for _, analysis := range plan.Analyses {
		if analysis.DCSweep != nil && analysis.DCSweep.Component == component {
			return true
		}
		for _, excitation := range analysis.Excitations {
			if excitation.Component != component {
				continue
			}
			if excitation.PulsePeriodS > 0 || excitation.SineFrequencyHz > 0 || excitation.ACMagnitude != 0 ||
				math.Abs(excitation.DCValue) <= lowThresholdV {
				return true
			}
		}
	}
	return false
}

func isolatedOpenDrainDriverDistances(plan Plan, lowThresholdV float64) (map[string]int, map[string]bool) {
	referenceNodes := map[string]bool{plan.GroundNode: true}
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveBidirectionalOpenDrainIsolatorV1 {
			continue
		}
		terminals := terminalMap(device)
		referenceNodes[terminals["GND1"]] = true
		referenceNodes[terminals["GND2"]] = true
	}
	supplyNodes := map[string]bool{}
	driverNodes := map[string]bool{}
	for _, device := range plan.Devices {
		positive, negative, source := isolatedOpenDrainSourceTerminals(device)
		if !source {
			continue
		}
		driver := isolatedOpenDrainSignalDriver(plan, device.Component, lowThresholdV)
		if negative != "" && referenceNodes[negative] {
			if driver {
				driverNodes[positive] = true
			} else {
				supplyNodes[positive] = true
			}
		}
		if positive != "" && referenceNodes[positive] {
			if driver {
				driverNodes[negative] = true
			} else {
				supplyNodes[negative] = true
			}
		}
	}
	adjacent := map[string][]string{}
	appendEdge := func(left, right string) {
		if left == "" || right == "" || left == right {
			return
		}
		adjacent[left] = append(adjacent[left], right)
		adjacent[right] = append(adjacent[right], left)
	}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveResistorV1:
			a, b := terminals["A"], terminals["B"]
			if referenceNodes[a] || referenceNodes[b] || supplyNodes[a] || supplyNodes[b] {
				continue
			}
			appendEdge(a, b)
		case PrimitiveBidirectionalOpenDrainIsolatorV1:
			for _, pair := range isolatedOpenDrainChannels {
				appendEdge(terminals[pair[0]], terminals[pair[1]])
			}
		}
	}
	distance := map[string]int{}
	queue := make([]string, 0, len(driverNodes))
	for node := range driverNodes {
		distance[node] = 0
		queue = append(queue, node)
	}
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		for _, neighbor := range adjacent[node] {
			if _, seen := distance[neighbor]; seen {
				continue
			}
			distance[neighbor] = distance[node] + 1
			queue = append(queue, neighbor)
		}
	}
	return distance, driverNodes
}

func resolveIsolatedOpenDrainDrivers(plan Plan, device ResolvedDevice, parameters map[string]float64) {
	terminals := terminalMap(device)
	distance, direct := isolatedOpenDrainDriverDistances(plan, parameters["low_level_threshold_v"])
	for channel, pair := range isolatedOpenDrainChannels {
		side1, side2 := terminals[pair[0]], terminals[pair[1]]
		distance1, reachable1 := distance[side1]
		distance2, reachable2 := distance[side2]
		switch {
		case direct[side1] && direct[side2]:
			parameters[isolatedOpenDrainDriverParameter(1, channel)] = 1
			parameters[isolatedOpenDrainDriverParameter(2, channel)] = 1
		case reachable1 && (!reachable2 || distance1 < distance2):
			parameters[isolatedOpenDrainDriverParameter(1, channel)] = 1
		case reachable2 && (!reachable1 || distance2 < distance1):
			parameters[isolatedOpenDrainDriverParameter(2, channel)] = 1
		}
	}
}

func isolatedOpenDrainResistance(device compiledNonlinearDevice, system *mnaSystem, solution []complex128, side int, channel int) float64 {
	parameters := device.parameters
	localGround := device.terminals[fmt.Sprintf("GND%d", side)]
	localSupply := device.terminals[fmt.Sprintf("VDD%d", side)]
	remoteSide := 3 - side
	remoteGround := device.terminals[fmt.Sprintf("GND%d", remoteSide)]
	remoteSignal := device.terminals[isolatedOpenDrainChannels[channel][remoteSide-1]]
	localVoltage := nonlinearNodeVoltage(system, solution, localSupply) - nonlinearNodeVoltage(system, solution, localGround)
	remoteVoltage := nonlinearNodeVoltage(system, solution, device.terminals[fmt.Sprintf("VDD%d", remoteSide)]) -
		nonlinearNodeVoltage(system, solution, remoteGround)
	if localVoltage < parameters[fmt.Sprintf("side_%c_min_v", 'a'+rune(side-1))] ||
		remoteVoltage < parameters[fmt.Sprintf("side_%c_min_v", 'a'+rune(remoteSide-1))] {
		return parameters["output_off_resistance_ohm"]
	}
	localDriven := parameters[isolatedOpenDrainDriverParameter(side, channel)] > 0
	remoteDriven := parameters[isolatedOpenDrainDriverParameter(remoteSide, channel)] > 0
	if (localDriven || remoteDriven) && !remoteDriven {
		return parameters["output_off_resistance_ohm"]
	}
	remoteLevel := nonlinearNodeVoltage(system, solution, remoteSignal) - nonlinearNodeVoltage(system, solution, remoteGround)
	if remoteLevel <= parameters["low_level_threshold_v"] {
		return parameters["output_on_resistance_ohm"]
	}
	return parameters["output_off_resistance_ohm"]
}

func stampNonlinearOpenDrainIsolator(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	stampAdmittance(system, device.terminals["GND1"], device.terminals["GND2"], complex(1/device.parameters["isolation_resistance_ohm"], 0))
	for channel := range isolatedOpenDrainChannels {
		for side := 1; side <= 2; side++ {
			signal := device.terminals[isolatedOpenDrainChannels[channel][side-1]]
			ground := device.terminals[fmt.Sprintf("GND%d", side)]
			resistance := isolatedOpenDrainResistance(device, system, guess, side, channel)
			stampAdmittance(system, signal, ground, complex(1/resistance, 0))
		}
	}
}

func addOpenDrainIsolatorResidual(residuals []complex128, base mnaSystem, device compiledNonlinearDevice, solution []complex128) {
	isolationCurrent := (nonlinearNodeVoltage(&base, solution, device.terminals["GND1"]) -
		nonlinearNodeVoltage(&base, solution, device.terminals["GND2"])) / device.parameters["isolation_resistance_ohm"]
	if index, exists := base.nodeIndex[device.terminals["GND1"]]; exists {
		residuals[index] += complex(isolationCurrent, 0)
	}
	if index, exists := base.nodeIndex[device.terminals["GND2"]]; exists {
		residuals[index] -= complex(isolationCurrent, 0)
	}
	for channel := range isolatedOpenDrainChannels {
		for side := 1; side <= 2; side++ {
			signal := device.terminals[isolatedOpenDrainChannels[channel][side-1]]
			ground := device.terminals[fmt.Sprintf("GND%d", side)]
			resistance := isolatedOpenDrainResistance(device, &base, solution, side, channel)
			current := (nonlinearNodeVoltage(&base, solution, signal) - nonlinearNodeVoltage(&base, solution, ground)) / resistance
			if index, exists := base.nodeIndex[signal]; exists {
				residuals[index] += complex(current, 0)
			}
			if index, exists := base.nodeIndex[ground]; exists {
				residuals[index] -= complex(current, 0)
			}
		}
	}
}

func validateOpenDrainIsolatorOperatingLimits(plan Plan, device ResolvedDevice, system mnaSystem, solution []complex128, allowPowerTransition bool) []Diagnostic {
	parameters := mutableDeviceParameterMap(device)
	resolveIsolatedOpenDrainDrivers(plan, device, parameters)
	terminals := terminalMap(device)
	path := "devices." + device.Component
	var diagnostics []Diagnostic
	for side := 1; side <= 2; side++ {
		ground := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("GND%d", side)]))
		supply := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("VDD%d", side)])) - ground
		label := string(rune('a' + side - 1))
		minimum, maximum := parameters["side_"+label+"_min_v"], parameters["side_"+label+"_max_v"]
		tolerance := 1e-9 * math.Max(1, math.Abs(maximum))
		if supply > maximum+tolerance || supply < -tolerance {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       path + ".side_" + label + "_supply",
				Message:    fmt.Sprintf("isolator side %s supply %.12g V is outside catalog-backed range 0..%.12g V", label, supply, maximum),
				Suggestion: "adjust supply conditions or select a compatible reviewed isolator",
			})
		}
		if !allowPowerTransition && supply+tolerance < minimum {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       path + ".side_" + label + "_supply",
				Message:    fmt.Sprintf("isolator side %s supply %.12g V is below catalog-backed minimum %.12g V", label, supply, minimum),
				Suggestion: "provide both operating supplies or verify the explicit unpowered state",
			})
		}
	}
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	for channel := range isolatedOpenDrainChannels {
		for side := 1; side <= 2; side++ {
			signal := terminals[isolatedOpenDrainChannels[channel][side-1]]
			ground := terminals[fmt.Sprintf("GND%d", side)]
			resistance := isolatedOpenDrainResistance(compiled, &system, solution, side, channel)
			current := math.Abs((real(solvedNodeVoltage(system, solution, signal)) - real(solvedNodeVoltage(system, solution, ground))) / resistance)
			if current > parameters["max_output_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{
					Path:       fmt.Sprintf("%s.channel_%d_side_%d_current", path, channel+1, side),
					Message:    fmt.Sprintf("isolator output current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_output_current_a"]),
					Suggestion: "increase pull-up impedance or select a suitably rated reviewed isolator",
				})
			}
		}
	}
	return diagnostics
}

func openDrainIsolatorDissipation(plan Plan, device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if device.PrimitiveModel != PrimitiveBidirectionalOpenDrainIsolatorV1 {
		return 0, false
	}
	parameters := mutableDeviceParameterMap(device)
	resolveIsolatedOpenDrainDrivers(plan, device, parameters)
	terminals := terminalMap(device)
	compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
	dissipation := 0.0
	for side := 1; side <= 2; side++ {
		ground := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("GND%d", side)]))
		supply := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("VDD%d", side)])) - ground
		label := string(rune('a' + side - 1))
		dissipation += math.Abs(supply) * parameters["side_"+label+"_quiescent_current_a"]
		for channel := range isolatedOpenDrainChannels {
			signal := real(solvedNodeVoltage(system, solution, terminals[isolatedOpenDrainChannels[channel][side-1]])) - ground
			resistance := isolatedOpenDrainResistance(compiled, &system, solution, side, channel)
			dissipation += signal * signal / resistance
		}
	}
	return dissipation, true
}
