package simmodel

import (
	"fmt"
	"math"
	"strings"
)

const (
	boltzmannConstant               = 1.380649e-23
	electronCharge                  = 1.602176634e-19
	nonlinearMaxIterations          = 60
	nonlinearMaxNodeUpdateV         = 0.25
	nonlinearUpdateTolerance        = 1e-8
	nonlinearCurrentUpdateTolerance = 1e-10
	nonlinearResidualTolerance      = 1e-12
	nonlinearExpLimit               = 40.0
	nonlinearFinalGmin              = 1e-12
)

type continuationStage struct {
	sourceScale float64
	gmin        float64
}

type compiledNonlinearDevice struct {
	primitive  string
	terminals  map[string]string
	parameters map[string]float64
	polarity   float64
}

var nonlinearContinuation = []continuationStage{
	{sourceScale: .05, gmin: 1e-4},
	{sourceScale: .2, gmin: 1e-5},
	{sourceScale: .5, gmin: 1e-6},
	{sourceScale: .8, gmin: 1e-8},
	{sourceScale: 1, gmin: 1e-10},
	{sourceScale: 1, gmin: nonlinearFinalGmin},
}

func solveNonlinearDC(plan Plan, analysis Analysis) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	system, solution, evidence, _, diagnostic := solveNonlinearDCFromState(plan, analysis, nil)
	return system, solution, evidence, diagnostic
}

func solveNonlinearDCFromState(plan Plan, analysis Analysis, initial map[string]float64) (mnaSystem, []complex128, SolverEvidence, map[string]float64, *Diagnostic) {
	comparatorCount := 0
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 {
			comparatorCount++
		}
	}
	states := cloneOpAmpClamps(initial)
	totalEvidence := SolverEvidence{Method: "bounded_newton_comparator_active_set_v1", MaxIterationsPerStep: nonlinearMaxIterations}
	iterationLimit := min(comparatorCount*6+2, maxOpAmpActiveSetIterations)
	if comparatorCount == 0 {
		iterationLimit = 1
	}
	seen := map[string]bool{}
	for iteration := 0; iteration < iterationLimit; iteration++ {
		system, solution, evidence, diagnostic := solveNonlinearDCForComparatorState(plan, analysis, states)
		totalEvidence.SourceStages += evidence.SourceStages
		totalEvidence.Iterations += evidence.Iterations
		totalEvidence.TotalIterations = totalEvidence.Iterations
		totalEvidence.FinalMaxUpdateV = evidence.FinalMaxUpdateV
		totalEvidence.FinalMaxCurrentUpdateA = evidence.FinalMaxCurrentUpdateA
		totalEvidence.FinalMaxResidual = evidence.FinalMaxResidual
		if diagnostic != nil {
			return system, solution, totalEvidence, states, diagnostic
		}
		next, stateKey, diagnostic := resolvedComparatorStates(plan, system, solution)
		if diagnostic != nil {
			return system, solution, totalEvidence, states, diagnostic
		}
		if sameOpAmpClamps(states, next) {
			return system, solution, totalEvidence, states, nil
		}
		if seen[stateKey] {
			return system, solution, totalEvidence, states, &Diagnostic{Path: "devices", Message: "bounded comparator operating-point states did not converge", Suggestion: "correct ambiguous feedback or add a reviewed hysteresis network and loading"}
		}
		seen[stateKey] = true
		states = next
	}
	return mnaSystem{}, nil, totalEvidence, states, &Diagnostic{Path: "devices", Message: "bounded comparator operating-point iteration exceeded its deterministic limit", Suggestion: "correct ambiguous feedback or reduce coupled comparator stages"}
}

func solveNonlinearDCForComparatorState(plan Plan, analysis Analysis, comparatorStates map[string]float64) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	var guess []complex128
	var finalSystem mnaSystem
	devices := compileNonlinearDevices(plan)
	evidence := SolverEvidence{Method: "bounded_newton_source_gmin_v1", SourceStages: len(nonlinearContinuation)}
	for stageIndex, stage := range nonlinearContinuation {
		baseSystem, diagnostics := buildNonlinearBaseSystem(plan, analysis, stage, comparatorStates)
		if len(diagnostics) != 0 {
			return mnaSystem{}, nil, evidence, &diagnostics[0]
		}
		if guess == nil {
			guess = make([]complex128, len(baseSystem.rhs))
		}
		system := cloneMNASystem(baseSystem)
		converged := false
		largestUpdateLabel := "unknown"
		largestCurrentUpdateLabel := "unknown"
		largestResidualLabel := "unknown"
		for iteration := 1; iteration <= nonlinearMaxIterations; iteration++ {
			resetMNASystem(&system, baseSystem)
			stampCompiledNonlinearDevices(&system, devices, guess)
			if diagnostic := validateMNASystemBounds(system); diagnostic != nil {
				return mnaSystem{}, nil, evidence, diagnostic
			}
			candidate, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Message = fmt.Sprintf("nonlinear continuation stage %d/%d failed: %s", stageIndex+1, len(nonlinearContinuation), diagnostic.Message)
				diagnostic.Suggestion = "add a catalog-backed DC bias path, correct floating nodes or source constraints, or select compatible reviewed nonlinear models"
				return mnaSystem{}, nil, evidence, diagnostic
			}
			for index, value := range candidate {
				if strings.HasPrefix(system.unknownLabels[index], "node:") && math.Abs(imag(value)) > 1e-15 {
					return mnaSystem{}, nil, evidence, &Diagnostic{
						Path:       "unknowns." + system.unknownLabels[index],
						Message:    "nonlinear DC produced a non-real node voltage",
						Suggestion: "correct ill-conditioned connectivity or select catalog models appropriate for DC analysis",
					}
				}
			}
			maxNodeUpdate, maxCurrentUpdate := 0.0, 0.0
			for index := range candidate {
				if strings.HasPrefix(system.unknownLabels[index], "node:") {
					update := math.Abs(real(candidate[index] - guess[index]))
					if update > maxNodeUpdate {
						maxNodeUpdate = update
						largestUpdateLabel = system.unknownLabels[index]
					}
				} else {
					update := math.Abs(real(candidate[index] - guess[index]))
					if update > maxCurrentUpdate {
						maxCurrentUpdate = update
						largestCurrentUpdateLabel = system.unknownLabels[index]
					}
				}
			}
			damping := 1.0
			if maxNodeUpdate > nonlinearMaxNodeUpdateV {
				damping = nonlinearMaxNodeUpdateV / maxNodeUpdate
			}
			maxAppliedUpdate, maxAppliedCurrentUpdate := 0.0, 0.0
			for index := range guess {
				applied := (candidate[index] - guess[index]) * complex(damping, 0)
				guess[index] += applied
				if strings.HasPrefix(system.unknownLabels[index], "node:") {
					maxAppliedUpdate = math.Max(maxAppliedUpdate, math.Abs(real(applied)))
				} else {
					maxAppliedCurrentUpdate = math.Max(maxAppliedCurrentUpdate, math.Abs(real(applied)))
				}
			}
			// Check the accepted damped state against the original nonlinear
			// KCL equations, not against the prior Newton linearization or an
			// undamped candidate that the solver did not accept.
			maxResidual, residualLabel := nonlinearResidual(baseSystem, devices, guess)
			largestResidualLabel = residualLabel
			evidence.Iterations++
			evidence.FinalMaxUpdateV = normalizedMNAFloat(maxAppliedUpdate)
			evidence.FinalMaxCurrentUpdateA = normalizedMNAFloat(maxAppliedCurrentUpdate)
			evidence.FinalMaxResidual = normalizedMNAFloat(maxResidual)
			finalSystem = system
			if maxAppliedUpdate <= nonlinearUpdateTolerance && maxAppliedCurrentUpdate <= nonlinearCurrentUpdateTolerance && maxResidual <= nonlinearResidualTolerance {
				converged = true
				break
			}
		}
		if !converged {
			return mnaSystem{}, nil, evidence, &Diagnostic{
				Path:       "convergence",
				Message:    fmt.Sprintf("nonlinear DC did not converge within %d iterations at continuation stage %d/%d; largest voltage update %s, largest current update %s, largest residual %s", nonlinearMaxIterations, stageIndex+1, len(nonlinearContinuation), largestUpdateLabel, largestCurrentUpdateLabel, largestResidualLabel),
				Suggestion: "add or correct catalog-backed DC bias paths, reduce conflicting source conditions, or select nonlinear models appropriate for the operating range",
			}
		}
	}
	return finalSystem, guess, evidence, nil
}

func compileNonlinearDevices(plan Plan) []compiledNonlinearDevice {
	var devices []compiledNonlinearDevice
	for _, device := range plan.Devices {
		polarity := 0.0
		switch device.PrimitiveModel {
		case PrimitiveDiodeShockleyV1:
			polarity = 1
		case PrimitiveBidirectionalTVSV1:
			polarity = 1
		case PrimitiveBJTNPNV1:
			polarity = 1
		case PrimitiveBJTPNPV1:
			polarity = -1
		default:
			continue
		}
		devices = append(devices, compiledNonlinearDevice{
			primitive: device.PrimitiveModel,
			terminals: terminalMap(device), parameters: namedValueMap(device.ModelParameters), polarity: polarity,
		})
	}
	return devices
}

func buildNonlinearBaseSystem(plan Plan, analysis Analysis, stage continuationStage, comparatorStates map[string]float64) (mnaSystem, []Diagnostic) {
	scaled := analysis
	scaled.Excitations = append([]SourceExcitation(nil), analysis.Excitations...)
	for index := range scaled.Excitations {
		scaled.Excitations[index].DCValue *= stage.sourceScale
	}
	system, diagnostics := buildMNASystemWithOpAmpClamps(plan, scaled, 0, comparatorStates)
	if len(diagnostics) != 0 {
		return system, diagnostics
	}
	for _, node := range plan.Nodes {
		if index, exists := system.nodeIndex[node]; exists {
			system.matrix[index][index] += complex(stage.gmin, 0)
		}
	}
	if diagnostic := validateMNASystemBounds(system); diagnostic != nil {
		return mnaSystem{}, []Diagnostic{*diagnostic}
	}
	return system, nil
}

func resolvedComparatorStates(plan Plan, system mnaSystem, solution []complex128) (map[string]float64, string, *Diagnostic) {
	states := map[string]float64{}
	var key strings.Builder
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveComparatorOpenCollectorV1 {
			continue
		}
		parameters := namedValueMap(device.ModelParameters)
		terminals := terminalMap(device)
		supply := nonlinearNodeVoltage(&system, solution, terminals["V_PLUS"]) - nonlinearNodeVoltage(&system, solution, terminals["V_MINUS"])
		if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
			return states, key.String(), &Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog comparator"}
		}
		if comparatorOn(device, system, solution) {
			states[device.Component] = 1
			key.WriteString(device.Component + ":on;")
		} else {
			states[device.Component] = 0
			key.WriteString(device.Component + ":off;")
		}
	}
	return states, key.String(), nil
}

func cloneMNASystem(source mnaSystem) mnaSystem {
	clone := source
	clone.matrix = make([][]complex128, len(source.matrix))
	for row := range source.matrix {
		clone.matrix[row] = append([]complex128(nil), source.matrix[row]...)
	}
	clone.rhs = append([]complex128(nil), source.rhs...)
	return clone
}

func resetMNASystem(target *mnaSystem, source mnaSystem) {
	for row := range source.matrix {
		copy(target.matrix[row], source.matrix[row])
	}
	copy(target.rhs, source.rhs)
}

func stampCompiledNonlinearDevices(system *mnaSystem, devices []compiledNonlinearDevice, guess []complex128) {
	for _, device := range devices {
		switch device.primitive {
		case PrimitiveDiodeShockleyV1:
			stampNonlinearDiode(system, device, guess)
		case PrimitiveBidirectionalTVSV1:
			stampNonlinearTVS(system, device, guess)
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			stampNonlinearBJT(system, device, guess)
		}
	}
}

func nonlinearNodeVoltage(system *mnaSystem, solution []complex128, node string) float64 {
	if index, exists := system.nodeIndex[node]; exists && index < len(solution) {
		return real(solution[index])
	}
	return 0
}

func bidirectionalTVSCurrentAndGradient(voltage float64, parameters map[string]float64) (float64, float64) {
	breakdown := parameters["breakdown_voltage_v"]
	offConductance := 1 / parameters["off_resistance_ohm"]
	onConductance := 1 / parameters["dynamic_resistance_ohm"]
	switch {
	case voltage > breakdown:
		return breakdown*offConductance + (voltage-breakdown)*onConductance, onConductance
	case voltage < -breakdown:
		return -breakdown*offConductance + (voltage+breakdown)*onConductance, onConductance
	default:
		return voltage * offConductance, offConductance
	}
}

func stampNonlinearTVS(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	anode, cathode := device.terminals["ANODE"], device.terminals["CATHODE"]
	voltage := nonlinearNodeVoltage(system, guess, anode) - nonlinearNodeVoltage(system, guess, cathode)
	current, gradient := bidirectionalTVSCurrentAndGradient(voltage, device.parameters)
	stampAdmittance(system, anode, cathode, complex(gradient, 0))
	stampCurrentSource(system, anode, cathode, complex(current-gradient*voltage, 0))
}

func thermalVoltage(parameters map[string]float64) float64 {
	return boltzmannConstant * parameters["junction_temperature_k"] / electronCharge
}

func boundedExponential(argument float64) (float64, float64) {
	if argument > nonlinearExpLimit {
		base := math.Exp(nonlinearExpLimit)
		return base * (1 + argument - nonlinearExpLimit), base
	}
	if argument < -nonlinearExpLimit {
		return math.Exp(-nonlinearExpLimit), 0
	}
	value := math.Exp(argument)
	return value, value
}

func diodeCurrentAndGradient(voltage float64, parameters map[string]float64) (float64, float64) {
	scale := parameters["emission_coefficient"] * thermalVoltage(parameters)
	exponential, derivative := boundedExponential(voltage / scale)
	return parameters["saturation_current_a"] * (exponential - 1), parameters["saturation_current_a"] * derivative / scale
}

func stampNonlinearDiode(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	voltage := nonlinearNodeVoltage(system, guess, device.terminals["ANODE"]) - nonlinearNodeVoltage(system, guess, device.terminals["CATHODE"])
	current, conductance := diodeCurrentAndGradient(voltage, device.parameters)
	stampAdmittance(system, device.terminals["ANODE"], device.terminals["CATHODE"], complex(conductance, 0))
	stampCurrentSource(system, device.terminals["ANODE"], device.terminals["CATHODE"], complex(current-conductance*voltage, 0))
}

func bjtCurrentsAndJacobian(vb, vc, ve float64, parameters map[string]float64, polarity float64) ([3]float64, [3][3]float64) {
	vt := parameters["emission_coefficient"] * thermalVoltage(parameters)
	is := parameters["saturation_current_a"]
	forwardExp, forwardDerivative := boundedExponential(polarity * (vb - ve) / vt)
	reverseExp, reverseDerivative := boundedExponential(polarity * (vb - vc) / vt)
	forward := is * (forwardExp - 1)
	reverse := is * (reverseExp - 1)
	gForward := is * forwardDerivative / vt
	gReverse := is * reverseDerivative / vt
	alphaForward := parameters["forward_beta"] / (parameters["forward_beta"] + 1)
	alphaReverse := parameters["reverse_beta"] / (parameters["reverse_beta"] + 1)
	currents := [3]float64{
		polarity * ((1-alphaForward)*forward + (1-alphaReverse)*reverse),
		polarity * (alphaForward*forward - reverse),
		polarity * (-forward + alphaReverse*reverse),
	}
	jacobian := [3][3]float64{
		{(1-alphaForward)*gForward + (1-alphaReverse)*gReverse, -(1 - alphaReverse) * gReverse, -(1 - alphaForward) * gForward},
		{alphaForward*gForward - gReverse, gReverse, -alphaForward * gForward},
		{-gForward + alphaReverse*gReverse, -alphaReverse * gReverse, gForward},
	}
	return currents, jacobian
}

func stampNonlinearBJT(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	nodes := [3]string{device.terminals["BASE"], device.terminals["COLLECTOR"], device.terminals["EMITTER"]}
	voltages := [3]float64{}
	for index, node := range nodes {
		voltages[index] = nonlinearNodeVoltage(system, guess, node)
	}
	currents, jacobian := bjtCurrentsAndJacobian(voltages[0], voltages[1], voltages[2], device.parameters, device.polarity)
	for row, rowNode := range nodes {
		rowIndex, rowKnown := system.nodeIndex[rowNode]
		if !rowKnown {
			continue
		}
		rhs := -currents[row]
		for column, columnNode := range nodes {
			rhs += jacobian[row][column] * voltages[column]
			if columnIndex, columnKnown := system.nodeIndex[columnNode]; columnKnown {
				system.matrix[rowIndex][columnIndex] += complex(jacobian[row][column], 0)
			}
		}
		system.rhs[rowIndex] += complex(rhs, 0)
	}
}

func nonlinearResidual(base mnaSystem, devices []compiledNonlinearDevice, solution []complex128) (float64, string) {
	residuals := make([]complex128, len(base.rhs))
	for row := range base.rhs {
		residuals[row] = -base.rhs[row]
		for column := range solution {
			residuals[row] += base.matrix[row][column] * solution[column]
		}
	}
	for _, device := range devices {
		switch device.primitive {
		case PrimitiveDiodeShockleyV1:
			voltage := nonlinearNodeVoltage(&base, solution, device.terminals["ANODE"]) - nonlinearNodeVoltage(&base, solution, device.terminals["CATHODE"])
			current, _ := diodeCurrentAndGradient(voltage, device.parameters)
			if index, exists := base.nodeIndex[device.terminals["ANODE"]]; exists {
				residuals[index] += complex(current, 0)
			}
			if index, exists := base.nodeIndex[device.terminals["CATHODE"]]; exists {
				residuals[index] -= complex(current, 0)
			}
		case PrimitiveBidirectionalTVSV1:
			voltage := nonlinearNodeVoltage(&base, solution, device.terminals["ANODE"]) - nonlinearNodeVoltage(&base, solution, device.terminals["CATHODE"])
			current, _ := bidirectionalTVSCurrentAndGradient(voltage, device.parameters)
			if index, exists := base.nodeIndex[device.terminals["ANODE"]]; exists {
				residuals[index] += complex(current, 0)
			}
			if index, exists := base.nodeIndex[device.terminals["CATHODE"]]; exists {
				residuals[index] -= complex(current, 0)
			}
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			nodes := [3]string{device.terminals["BASE"], device.terminals["COLLECTOR"], device.terminals["EMITTER"]}
			voltages := [3]float64{}
			for index, node := range nodes {
				voltages[index] = nonlinearNodeVoltage(&base, solution, node)
			}
			currents, _ := bjtCurrentsAndJacobian(voltages[0], voltages[1], voltages[2], device.parameters, device.polarity)
			for index, node := range nodes {
				if row, exists := base.nodeIndex[node]; exists {
					residuals[row] += complex(currents[index], 0)
				}
			}
		}
	}
	maximum, label := 0.0, "unknown"
	for row, residual := range residuals {
		if magnitude := math.Abs(real(residual)); magnitude > maximum {
			maximum, label = magnitude, base.unknownLabels[row]
		}
	}
	return maximum, label
}

func validateNonlinearOperatingLimits(plan Plan, system mnaSystem, solution []complex128) []Diagnostic {
	return validateNonlinearOperatingLimitsWithComparatorStates(plan, system, solution, nil)
}

func validateNonlinearOperatingLimitsWithComparatorStates(plan Plan, system mnaSystem, solution []complex128, comparatorStates map[string]float64) []Diagnostic {
	var diagnostics []Diagnostic
	for _, device := range plan.Devices {
		parameters := namedValueMap(device.ModelParameters)
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveDiodeShockleyV1:
			voltage := nonlinearNodeVoltage(&system, solution, terminals["ANODE"]) - nonlinearNodeVoltage(&system, solution, terminals["CATHODE"])
			current, _ := diodeCurrentAndGradient(voltage, parameters)
			if current > parameters["max_forward_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("diode forward current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_forward_current_a"]), Suggestion: "increase series resistance, reduce source voltage, or select a suitably rated reviewed diode"})
			}
			if -voltage > parameters["max_reverse_voltage_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("diode reverse voltage %.12g V exceeds catalog-backed limit %.12g V", -voltage, parameters["max_reverse_voltage_v"]), Suggestion: "reduce reverse voltage or select a suitably rated reviewed diode"})
			}
		case PrimitiveBidirectionalTVSV1:
			voltage := nonlinearNodeVoltage(&system, solution, terminals["ANODE"]) - nonlinearNodeVoltage(&system, solution, terminals["CATHODE"])
			current, _ := bidirectionalTVSCurrentAndGradient(voltage, parameters)
			if math.Abs(current) > parameters["max_pulse_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("TVS pulse current %.12g A exceeds catalog-backed limit %.12g A", math.Abs(current), parameters["max_pulse_current_a"]), Suggestion: "increase source impedance or select a compatible reviewed protection device"})
			}
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			polarity := 1.0
			if device.PrimitiveModel == PrimitiveBJTPNPV1 {
				polarity = -1
			}
			vb := nonlinearNodeVoltage(&system, solution, terminals["BASE"])
			vc := nonlinearNodeVoltage(&system, solution, terminals["COLLECTOR"])
			ve := nonlinearNodeVoltage(&system, solution, terminals["EMITTER"])
			currents, _ := bjtCurrentsAndJacobian(vb, vc, ve, parameters, polarity)
			if math.Abs(currents[1]) > parameters["max_collector_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("BJT collector current %.12g A exceeds catalog-backed limit %.12g A", math.Abs(currents[1]), parameters["max_collector_current_a"]), Suggestion: "increase current-limiting resistance, reduce drive, or select a suitably rated reviewed transistor"})
			}
			if math.Abs(vc-ve) > parameters["max_collector_emitter_voltage_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("BJT collector-emitter voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(vc-ve), parameters["max_collector_emitter_voltage_v"]), Suggestion: "reduce supply voltage or select a suitably rated reviewed transistor"})
			}
		case PrimitiveComparatorOpenCollectorV1:
			supply := nonlinearNodeVoltage(&system, solution, terminals["V_PLUS"]) - nonlinearNodeVoltage(&system, solution, terminals["V_MINUS"])
			if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog comparator"})
			}
			on := comparatorOn(device, system, solution)
			if state, exists := comparatorStates[device.Component]; exists {
				on = state >= .5
			}
			if on {
				voltage := nonlinearNodeVoltage(&system, solution, terminals["OUT"]) - nonlinearNodeVoltage(&system, solution, terminals["V_MINUS"])
				current := math.Abs(voltage / parameters["output_on_resistance_ohm"])
				if current > parameters["max_sink_current_a"] {
					diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("comparator sink current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_sink_current_a"]), Suggestion: "increase output pull-up resistance or select a compatible reviewed comparator"})
				}
			}
		}
	}
	return diagnostics
}
