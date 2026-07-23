package simmodel

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	boltzmannConstant = 1.380649e-23
	electronCharge    = 1.602176634e-19
	// Catalog saturation-current parameters are characterized at the shared
	// 27 C model reference. The trusted silicon primitive owns the bounded
	// temperature law so providers cannot inject semiconductor equations.
	nonlinearNominalTemperatureK = 300.15
	siliconBandgapEnergyEV       = 1.11
	siliconSaturationExponent    = 3.0
	// The trusted 250 mV update bound needs at least 120 iterations to cross a
	// 30 V dual-rail operating envelope from a clamped continuation seed.
	// Keep deterministic damping and provide enough bounded work for that
	// registered voltage range.
	nonlinearMaxIterations          = 128
	nonlinearMaxNodeUpdateV         = 0.25
	nonlinearUpdateTolerance        = 1e-8
	nonlinearResidualFloorUpdateV   = 1e-7
	nonlinearCurrentUpdateTolerance = 1e-10
	nonlinearResidualTolerance      = 1e-12
	nonlinearExpLimit               = 40.0
	nonlinearFinalGmin              = 1e-12
	nonlinearMinLineSearchDamping   = 1.0 / 4096
	nonlinearMaxContinuationStages  = 24
	nonlinearMinSourceStep          = 1e-2
	nonlinearMinGminRatio           = 3
	nonlinearLineSearchDecrease     = 1e-4
	nonlinearClampConsistencyV      = 1e-3
)

type continuationStage struct {
	sourceScale float64
	gmin        float64
	gainScale   float64
}

type compiledNonlinearDevice struct {
	component  string
	primitive  string
	terminals  map[string]string
	parameters map[string]float64
	polarity   float64
}

var nonlinearContinuation = []continuationStage{
	{sourceScale: .05, gmin: 1e-4, gainScale: 1e-4},
	{sourceScale: .2, gmin: 1e-5, gainScale: 1e-3},
	{sourceScale: .5, gmin: 1e-6, gainScale: 1e-2},
	{sourceScale: .8, gmin: 1e-8, gainScale: .1},
	{sourceScale: 1, gmin: 1e-10, gainScale: .5},
	{sourceScale: 1, gmin: nonlinearFinalGmin, gainScale: 1},
}

func solveNonlinearDC(plan Plan, analysis Analysis) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	system, solution, evidence, _, diagnostic := solveNonlinearDCFromState(plan, analysis, nil)
	return system, solution, evidence, diagnostic
}

func solveNonlinearDCFromState(plan Plan, analysis Analysis, initial map[string]float64) (mnaSystem, []complex128, SolverEvidence, map[string]float64, *Diagnostic) {
	return solveNonlinearDCFromWarmState(plan, analysis, initial, nil)
}

func solveNonlinearDCFromWarmState(plan Plan, analysis Analysis, initial map[string]float64, initialSolution []complex128) (mnaSystem, []complex128, SolverEvidence, map[string]float64, *Diagnostic) {
	activeDeviceCount := 0
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 || device.PrimitiveModel == PrimitiveOpAmpV1 || device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1 {
			activeDeviceCount++
		}
	}
	states := cloneOpAmpClamps(initial)
	if len(initial) == 0 && len(initialSolution) == 0 {
		for _, device := range plan.Devices {
			switch device.PrimitiveModel {
			case PrimitiveComparatorOpenCollectorV1:
				states[device.Component] = 0
			}
		}
	}
	totalEvidence := SolverEvidence{Method: "bounded_newton_comparator_active_set_v1", MaxIterationsPerStep: nonlinearMaxIterations}
	iterationLimit := min(activeDeviceCount*6+2, maxOpAmpActiveSetIterations)
	if activeDeviceCount == 0 {
		iterationLimit = 1
	}
	seen := map[string]bool{}
	lastStateKey := ""
	transitionContext := ""
	warmSolution := append([]complex128(nil), initialSolution...)
	for iteration := 0; iteration < iterationLimit; iteration++ {
		system, solution, evidence, diagnostic := solveNonlinearDCForComparatorStateWithInitial(plan, analysis, states, warmSolution)
		if diagnostic != nil {
			if fallbackSystem, fallbackSolution, fallbackEvidence, ok := solveNonlinearDCByMOSFETActiveSet(plan, analysis, states); ok {
				system, solution, evidence, diagnostic = fallbackSystem, fallbackSolution, fallbackEvidence, nil
			} else if fallbackSystem, fallbackSolution, fallbackEvidence, fallbackStates, ok := solveNonlinearDCByComparatorActiveSet(plan, analysis, states); ok {
				system, solution, evidence, states, diagnostic = fallbackSystem, fallbackSolution, fallbackEvidence, fallbackStates, nil
			}
		}
		totalEvidence.SourceStages += evidence.SourceStages
		totalEvidence.Iterations += evidence.Iterations
		totalEvidence.TotalIterations = totalEvidence.Iterations
		totalEvidence.FinalMaxUpdateV = evidence.FinalMaxUpdateV
		totalEvidence.FinalMaxCurrentUpdateA = evidence.FinalMaxCurrentUpdateA
		totalEvidence.FinalMaxResidual = evidence.FinalMaxResidual
		if diagnostic != nil {
			if transitionContext != "" {
				diagnostic.Message = transitionContext + "; " + diagnostic.Message
			}
			return system, solution, totalEvidence, states, diagnostic
		}
		resolved, resolvedKey, diagnostic := resolvedActiveDeviceStates(plan, system, solution)
		if diagnostic != nil {
			return system, solution, totalEvidence, states, diagnostic
		}
		if sameOpAmpClamps(states, resolved) {
			return system, solution, totalEvidence, states, nil
		}
		if activeDeviceStateSolutionConsistent(plan, system, solution, states, resolved) {
			return system, solution, totalEvidence, resolved, nil
		}
		bisectedSystem, bisectedSolution, bisectedEvidence, bisectedStates, bisected := solveLinearOpAmpByBisection(plan, analysis, states, resolved, system, solution)
		if bisected {
			totalEvidence.SourceStages += bisectedEvidence.SourceStages
			totalEvidence.Iterations += bisectedEvidence.Iterations
			totalEvidence.TotalIterations = totalEvidence.Iterations
			totalEvidence.FinalMaxUpdateV = bisectedEvidence.FinalMaxUpdateV
			totalEvidence.FinalMaxCurrentUpdateA = bisectedEvidence.FinalMaxCurrentUpdateA
			totalEvidence.FinalMaxResidual = bisectedEvidence.FinalMaxResidual
			stateKey := activeDeviceStateKey(plan, bisectedStates)
			if seen[stateKey] {
				return bisectedSystem, bisectedSolution, totalEvidence, states, &Diagnostic{Path: "devices", Message: "bounded active-device operating-point states did not converge after op-amp bisection (current " + activeDeviceStateKey(plan, states) + ", resolved " + resolvedKey + ", cycle " + lastStateKey + " -> " + stateKey + ")", Suggestion: "correct ambiguous feedback or reduce coupled active stages"}
			}
			seen[stateKey] = true
			lastStateKey = stateKey
			states = bisectedStates
			warmSolution = append(warmSolution[:0], bisectedSolution...)
			transitionContext = "released one catalog op-amp clamp by bounded output bisection"
			continue
		}
		transitionContext = activeDeviceTransitionContext(plan, system, solution, states, resolved)
		if bisectedEvidence.Method != "" {
			transitionContext += "; bisection " + bisectedEvidence.Method
		}
		next := advanceActiveDeviceState(plan, states, resolved)
		stateKey := activeDeviceStateKey(plan, next)
		if seen[stateKey] {
			return system, solution, totalEvidence, states, &Diagnostic{Path: "devices", Message: "bounded active-device operating-point states did not converge (current " + activeDeviceStateKey(plan, states) + ", resolved " + resolvedKey + ", cycle " + lastStateKey + " -> " + stateKey + ")", Suggestion: "correct ambiguous feedback or add a reviewed hysteresis network and loading"}
		}
		seen[stateKey] = true
		lastStateKey = stateKey
		states = next
		warmSolution = append(warmSolution[:0], solution...)
	}
	return mnaSystem{}, nil, totalEvidence, states, &Diagnostic{Path: "devices", Message: "bounded comparator operating-point iteration exceeded its deterministic limit", Suggestion: "correct ambiguous feedback or reduce coupled comparator stages"}
}

func solveLinearOpAmpByBisection(plan Plan, analysis Analysis, current, resolved map[string]float64, operatingSystem mnaSystem, operatingSolution []complex128) (mnaSystem, []complex128, SolverEvidence, map[string]float64, bool) {
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		currentValue, currentExists := current[device.Component]
		resolvedValue, resolvedExists := resolved[device.Component]
		if !currentExists || resolvedExists && activeDeviceStateValueEqual(currentValue, resolvedValue) {
			continue
		}
		terminals := terminalMap(device)
		parameters := namedValueMap(device.ModelParameters)
		lower := nonlinearNodeVoltage(&operatingSystem, operatingSolution, terminals["V_MINUS"]) + parameters["output_low_margin_v"]
		upper := nonlinearNodeVoltage(&operatingSystem, operatingSolution, terminals["V_PLUS"]) - parameters["output_high_margin_v"]
		if !finite(lower) || !finite(upper) || lower >= upper {
			return mnaSystem{}, nil, SolverEvidence{}, nil, false
		}
		evaluate := func(value float64, warm []complex128) (mnaSystem, []complex128, SolverEvidence, float64, bool) {
			if math.Abs(value-currentValue) <= nonlinearUpdateTolerance {
				differential := nonlinearNodeVoltage(&operatingSystem, operatingSolution, terminals["IN_PLUS"]) - nonlinearNodeVoltage(&operatingSystem, operatingSolution, terminals["IN_MINUS"])
				return operatingSystem, operatingSolution, SolverEvidence{Method: "existing_clamped_operating_point_v1"}, value - parameters["dc_open_loop_gain"]*differential, true
			}
			clamps := cloneOpAmpClamps(current)
			clamps[device.Component] = value
			system, solution, evidence, diagnostic := solveNonlinearDCForComparatorStateWithInitial(plan, analysis, clamps, warm)
			if diagnostic != nil {
				return mnaSystem{}, nil, evidence, 0, false
			}
			differential := nonlinearNodeVoltage(&system, solution, terminals["IN_PLUS"]) - nonlinearNodeVoltage(&system, solution, terminals["IN_MINUS"])
			return system, solution, evidence, value - parameters["dc_open_loop_gain"]*differential, true
		}
		lowerSystem, lowerSolution, lowerEvidence, lowerResidual, lowerOK := evaluate(lower, nil)
		upperSystem, upperSolution, upperEvidence, upperResidual, upperOK := evaluate(upper, nil)
		evidence := SolverEvidence{
			Method:               "bounded_opamp_output_bisection_v1",
			SourceStages:         lowerEvidence.SourceStages + upperEvidence.SourceStages,
			Iterations:           lowerEvidence.Iterations + upperEvidence.Iterations,
			MaxIterationsPerStep: nonlinearMaxIterations,
		}
		if !lowerOK || !upperOK {
			lowerPlus := nonlinearNodeVoltage(&lowerSystem, lowerSolution, terminals["IN_PLUS"])
			lowerMinus := nonlinearNodeVoltage(&lowerSystem, lowerSolution, terminals["IN_MINUS"])
			upperPlus := nonlinearNodeVoltage(&upperSystem, upperSolution, terminals["IN_PLUS"])
			upperMinus := nonlinearNodeVoltage(&upperSystem, upperSolution, terminals["IN_MINUS"])
			evidence.Method = fmt.Sprintf("endpoint bracket lower_ok=%t upper_ok=%t lower_residual=%.12g upper_residual=%.12g lower_inputs=%.12g/%.12g upper_inputs=%.12g/%.12g", lowerOK, upperOK, lowerResidual, upperResidual, lowerPlus, lowerMinus, upperPlus, upperMinus)
			return mnaSystem{}, nil, evidence, nil, false
		}
		if math.Abs(lowerResidual) <= nonlinearClampConsistencyV {
			next := cloneOpAmpClamps(resolved)
			delete(next, device.Component)
			return lowerSystem, lowerSolution, evidence, next, true
		}
		if math.Abs(upperResidual) <= nonlinearClampConsistencyV {
			next := cloneOpAmpClamps(resolved)
			delete(next, device.Component)
			return upperSystem, upperSolution, evidence, next, true
		}
		if math.Signbit(lowerResidual) == math.Signbit(upperResidual) {
			midpoint := (lower + upper) / 2
			midSystem, midSolution, midEvidence, midResidual, midOK := evaluate(midpoint, nil)
			evidence.SourceStages += midEvidence.SourceStages
			evidence.Iterations += midEvidence.Iterations
			if midOK && math.Abs(midResidual) <= nonlinearClampConsistencyV {
				next := cloneOpAmpClamps(resolved)
				delete(next, device.Component)
				evidence.TotalIterations = evidence.Iterations
				evidence.FinalMaxResidual = normalizedMNAFloat(math.Abs(midResidual))
				return midSystem, midSolution, evidence, next, true
			}
			switch {
			case midOK && math.Signbit(lowerResidual) != math.Signbit(midResidual):
				upper, upperResidual, upperSolution = midpoint, midResidual, midSolution
			case midOK && math.Signbit(midResidual) != math.Signbit(upperResidual):
				lower, lowerResidual, lowerSolution = midpoint, midResidual, midSolution
			default:
				evidence.Method = fmt.Sprintf("midpoint bracket midpoint_ok=%t lower_residual=%.12g midpoint_residual=%.12g upper_residual=%.12g", midOK, lowerResidual, midResidual, upperResidual)
				return mnaSystem{}, nil, evidence, nil, false
			}
		}
		bestSystem, bestSolution := lowerSystem, lowerSolution
		for iteration := 0; iteration < 48; iteration++ {
			midpoint := (lower + upper) / 2
			warm := lowerSolution
			if math.Abs(upper-midpoint) < math.Abs(midpoint-lower) {
				warm = upperSolution
			}
			midSystem, midSolution, midEvidence, midResidual, ok := evaluate(midpoint, warm)
			evidence.SourceStages += midEvidence.SourceStages
			evidence.Iterations += midEvidence.Iterations
			if !ok {
				evidence.Method = fmt.Sprintf("midpoint solve at %.12g V", midpoint)
				return mnaSystem{}, nil, evidence, nil, false
			}
			bestSystem, bestSolution = midSystem, midSolution
			if math.Abs(midResidual) <= nonlinearClampConsistencyV {
				next := cloneOpAmpClamps(resolved)
				delete(next, device.Component)
				evidence.TotalIterations = evidence.Iterations
				evidence.FinalMaxResidual = normalizedMNAFloat(math.Abs(midResidual))
				return bestSystem, bestSolution, evidence, next, true
			}
			if math.Signbit(midResidual) == math.Signbit(lowerResidual) {
				lower, lowerResidual, lowerSolution = midpoint, midResidual, midSolution
			} else {
				upper, upperResidual, upperSolution = midpoint, midResidual, midSolution
			}
		}
		evidence.Method = "iteration bound"
		return mnaSystem{}, nil, evidence, nil, false
	}
	return mnaSystem{}, nil, SolverEvidence{}, nil, false
}

func activeDeviceTransitionContext(plan Plan, system mnaSystem, solution []complex128, current, resolved map[string]float64) string {
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 && device.PrimitiveModel != PrimitiveCurrentSenseAmplifierV1 {
			continue
		}
		currentValue, currentExists := current[device.Component]
		_, resolvedExists := resolved[device.Component]
		if !currentExists || resolvedExists {
			continue
		}
		desired := 0.0
		if device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1 {
			_, _, _, desired = currentSenseOperatingState(device, system, solution)
		} else {
			terminals := terminalMap(device)
			gain := namedValueMap(device.ModelParameters)["dc_open_loop_gain"]
			differential := nonlinearNodeVoltage(&system, solution, terminals["IN_PLUS"]) - nonlinearNodeVoltage(&system, solution, terminals["IN_MINUS"])
			desired = gain * differential
		}
		return fmt.Sprintf("released %s clamp %.12g V toward finite-gain target %.12g V", device.Component, currentValue, desired)
	}
	return ""
}

func activeDeviceStateSolutionConsistent(plan Plan, system mnaSystem, solution []complex128, current, resolved map[string]float64) bool {
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 && device.PrimitiveModel != PrimitiveComparatorOpenCollectorV1 && device.PrimitiveModel != PrimitiveCurrentSenseAmplifierV1 {
			continue
		}
		currentValue, currentExists := current[device.Component]
		resolvedValue, resolvedExists := resolved[device.Component]
		if currentExists == resolvedExists {
			if !currentExists || activeDeviceStateValueEqual(currentValue, resolvedValue) {
				continue
			}
			return false
		}
		if device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 {
			return false
		}
		if !currentExists || resolvedExists {
			return false
		}
		desired := 0.0
		if device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1 {
			_, _, _, desired = currentSenseOperatingState(device, system, solution)
		} else {
			terminals := terminalMap(device)
			gain := namedValueMap(device.ModelParameters)["dc_open_loop_gain"]
			differential := nonlinearNodeVoltage(&system, solution, terminals["IN_PLUS"]) - nonlinearNodeVoltage(&system, solution, terminals["IN_MINUS"])
			desired = gain * differential
		}
		tolerance := nonlinearClampConsistencyV * math.Max(1, math.Abs(currentValue))
		if math.Abs(desired-currentValue) > tolerance {
			return false
		}
	}
	return true
}

func advanceActiveDeviceState(plan Plan, current, resolved map[string]float64) map[string]float64 {
	next := cloneOpAmpClamps(current)
	for priority := 0; priority < 2; priority++ {
		for _, device := range plan.Devices {
			linearOutput := device.PrimitiveModel == PrimitiveOpAmpV1 || device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1
			if priority == 0 && !linearOutput || priority == 1 && device.PrimitiveModel != PrimitiveComparatorOpenCollectorV1 {
				continue
			}
			currentValue, currentExists := current[device.Component]
			resolvedValue, resolvedExists := resolved[device.Component]
			if currentExists == resolvedExists && (!currentExists || activeDeviceStateValueEqual(currentValue, resolvedValue)) {
				continue
			}
			if linearOutput && currentExists && resolvedExists {
				// A constrained feedback solve can make the opposite rail appear
				// immediately active. Release the current clamp first so the trusted
				// linear equation decides whether that opposite clamp is real. Do
				// this before changing downstream comparator decisions.
				delete(next, device.Component)
				return next
			}
			if resolvedExists {
				next[device.Component] = resolvedValue
			} else {
				delete(next, device.Component)
			}
			return next
		}
	}
	return next
}

func activeDeviceStateValueEqual(left, right float64) bool {
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= mnaPivotTolerance*scale
}

func activeDeviceStateKey(plan Plan, states map[string]float64) string {
	var key strings.Builder
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveComparatorOpenCollectorV1 && device.PrimitiveModel != PrimitiveOpAmpV1 && device.PrimitiveModel != PrimitiveCurrentSenseAmplifierV1 {
			continue
		}
		value, exists := states[device.Component]
		if !exists {
			key.WriteString(device.Component + ":linear;")
			continue
		}
		key.WriteString(device.Component + ":" + fmt.Sprintf("%.12g", value) + ";")
	}
	return key.String()
}

func solveNonlinearDCForComparatorState(plan Plan, analysis Analysis, comparatorStates map[string]float64) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	return solveNonlinearDCForComparatorStateWithInitial(plan, analysis, comparatorStates, nil)
}

func solveNonlinearDCForComparatorStateWithInitial(plan Plan, analysis Analysis, comparatorStates map[string]float64, initialSolution []complex128) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	guess := append([]complex128(nil), initialSolution...)
	var finalSystem mnaSystem
	devices := compileNonlinearDevicesWithStates(plan, comparatorStates)
	if len(devices) == 0 {
		system, diagnostics := buildNonlinearBaseSystem(plan, analysis, continuationStage{sourceScale: 1, gmin: nonlinearFinalGmin}, comparatorStates)
		evidence := SolverEvidence{Method: "pivoted_piecewise_linear_active_set_v1", Iterations: 1, SourceStages: 1, TotalIterations: 1}
		if len(diagnostics) != 0 {
			return mnaSystem{}, nil, evidence, &diagnostics[0]
		}
		solution, diagnostic := solveMNA(system)
		if diagnostic != nil {
			return system, nil, evidence, diagnostic
		}
		return system, solution, evidence, nil
	}
	stages := nonlinearContinuation
	method := "bounded_newton_source_gmin_v1"
	warmStart := len(guess) != 0
	warmFallbackUsed := false
	stageAttemptOffset := 0
	if len(guess) != 0 {
		stages = []continuationStage{{sourceScale: 1, gmin: nonlinearFinalGmin, gainScale: 1}}
		method = "bounded_newton_direct_warm_start_v1"
	}
	evidence := SolverEvidence{Method: method, SourceStages: len(stages)}
	stages = append([]continuationStage(nil), stages...)
	for stageIndex := 0; stageIndex < len(stages); {
		stage := stages[stageIndex]
		baseSystem, diagnostics := buildNonlinearBaseSystem(plan, analysis, stage, comparatorStates)
		if len(diagnostics) != 0 {
			return mnaSystem{}, nil, evidence, &diagnostics[0]
		}
		if len(guess) == 0 {
			guess = make([]complex128, len(baseSystem.rhs))
		} else if len(guess) != len(baseSystem.rhs) {
			return mnaSystem{}, nil, evidence, &Diagnostic{Path: "initial_state", Message: "nonlinear warm-start state size differs from the resolved MNA system"}
		}
		stageInitialGuess := append([]complex128(nil), guess...)
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
				diagnostic.Message = fmt.Sprintf("nonlinear continuation stage %d/%d failed: %s", stageIndex+1, len(stages), diagnostic.Message)
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
			if maxNodeUpdate > nonlinearMaxNodeUpdateV && !piecewiseLinearRegionStable(devices, &system, guess, candidate) {
				damping = nonlinearMaxNodeUpdateV / maxNodeUpdate
			}
			if requiresNonlinearLineSearch(devices) {
				priorResidual, _ := nonlinearResidual(baseSystem, devices, guess)
				trial := make([]complex128, len(guess))
				for {
					for index := range guess {
						trial[index] = guess[index] + (candidate[index]-guess[index])*complex(damping, 0)
					}
					trialResidual, _ := nonlinearResidual(baseSystem, devices, trial)
					if trialResidual <= priorResidual*(1-nonlinearLineSearchDecrease*damping) || damping <= nonlinearMinLineSearchDamping {
						break
					}
					damping *= .5
				}
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
			if nonlinearIterationConverged(maxAppliedUpdate, maxAppliedCurrentUpdate, maxResidual) {
				converged = true
				break
			}
		}
		if !converged {
			if warmStart && !warmFallbackUsed {
				// A nearby sweep or transient operating point is normally the best
				// initial state at the physical gmin. If it does not converge, discard
				// it and retain the complete cold source/gmin continuation as a
				// deterministic bounded fallback.
				warmFallbackUsed = true
				stageAttemptOffset = 1
				stages = append([]continuationStage(nil), nonlinearContinuation...)
				guess = nil
				stageIndex = 0
				evidence.Method = "bounded_newton_direct_warm_start_with_source_gmin_fallback_v1"
				evidence.SourceStages = stageAttemptOffset + len(stages)
				continue
			}
			if midpoint, refinable := nonlinearContinuationMidpoint(stages, stageIndex); refinable && len(stages) < nonlinearMaxContinuationStages {
				stages = append(stages, continuationStage{})
				copy(stages[stageIndex+1:], stages[stageIndex:])
				stages[stageIndex] = midpoint
				guess = stageInitialGuess
				evidence.SourceStages = stageAttemptOffset + len(stages)
				continue
			}
			voltageContext := nonlinearUnknownContext(plan, largestUpdateLabel)
			residualContext := nonlinearUnknownContext(plan, largestResidualLabel)
			activeState := activeDeviceStateKey(plan, comparatorStates)
			return mnaSystem{}, nil, evidence, &Diagnostic{
				Path:       "convergence",
				Message:    fmt.Sprintf("nonlinear DC did not converge within %d iterations at continuation stage %d/%d (source %.12g, gmin %.12g, op-amp gain %.12g) with active-device state %s; largest voltage update %s%s %.12g V, largest current update %s %.12g A, largest normalized residual %s%s %.12g", nonlinearMaxIterations, stageIndex+1, len(stages), stage.sourceScale, stage.gmin, stage.gainScale, activeState, largestUpdateLabel, voltageContext, evidence.FinalMaxUpdateV, largestCurrentUpdateLabel, evidence.FinalMaxCurrentUpdateA, largestResidualLabel, residualContext, evidence.FinalMaxResidual),
				Suggestion: "add or correct catalog-backed DC bias paths, reduce conflicting source conditions, or select nonlinear models appropriate for the operating range",
			}
		}
		stageIndex++
	}
	return finalSystem, guess, evidence, nil
}

func nonlinearIterationConverged(maxVoltageUpdateV, maxCurrentUpdateA, maxResidual float64) bool {
	if maxCurrentUpdateA > nonlinearCurrentUpdateTolerance || maxResidual > nonlinearResidualTolerance {
		return false
	}
	return maxVoltageUpdateV <= nonlinearResidualFloorUpdateV
}

func solveNonlinearDCByMOSFETActiveSet(plan Plan, analysis Analysis, activeStates map[string]float64) (mnaSystem, []complex128, SolverEvidence, bool) {
	var switches []string
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveNMOSSwitchV1 || device.PrimitiveModel == PrimitivePMOSSwitchV1 {
			switches = append(switches, device.Component)
		}
	}
	slices.Sort(switches)
	if len(switches) == 0 || len(switches) > 4 {
		return mnaSystem{}, nil, SolverEvidence{}, false
	}
	var total SolverEvidence
	for mask := 0; mask < 1<<len(switches); mask++ {
		states := cloneOpAmpClamps(activeStates)
		for index, component := range switches {
			states[component] = float64((mask >> index) & 1)
		}
		system, solution, evidence, diagnostic := solveNonlinearDCForComparatorStateWithInitial(plan, analysis, states, nil)
		total.SourceStages += evidence.SourceStages
		total.Iterations += evidence.Iterations
		if diagnostic != nil || !mosfetActiveSetConsistent(plan, system, solution, states) {
			continue
		}
		evidence.Method = "bounded_newton_mosfet_active_set_v1"
		evidence.SourceStages = total.SourceStages
		evidence.Iterations = total.Iterations
		evidence.TotalIterations = total.Iterations
		return system, solution, evidence, true
	}
	return mnaSystem{}, nil, total, false
}

func solveNonlinearDCByComparatorActiveSet(plan Plan, analysis Analysis, activeStates map[string]float64) (mnaSystem, []complex128, SolverEvidence, map[string]float64, bool) {
	var comparators []string
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 {
			comparators = append(comparators, device.Component)
		}
	}
	slices.Sort(comparators)
	if len(comparators) == 0 || len(comparators) > 4 {
		return mnaSystem{}, nil, SolverEvidence{}, nil, false
	}
	var total SolverEvidence
	for mask := 0; mask < 1<<len(comparators); mask++ {
		states := cloneOpAmpClamps(activeStates)
		differs := false
		for index, component := range comparators {
			value := float64((mask >> index) & 1)
			if current, exists := activeStates[component]; !exists || !activeDeviceStateValueEqual(current, value) {
				differs = true
			}
			states[component] = value
		}
		if !differs {
			continue
		}
		system, solution, evidence, diagnostic := solveNonlinearDCForComparatorStateWithInitial(plan, analysis, states, nil)
		if diagnostic != nil {
			if mosfetSystem, mosfetSolution, mosfetEvidence, ok := solveNonlinearDCByMOSFETActiveSet(plan, analysis, states); ok {
				system, solution, evidence, diagnostic = mosfetSystem, mosfetSolution, mosfetEvidence, nil
			}
		}
		total.SourceStages += evidence.SourceStages
		total.Iterations += evidence.Iterations
		if diagnostic != nil {
			continue
		}
		evidence.Method = "bounded_newton_comparator_active_set_fallback_v1"
		evidence.SourceStages = total.SourceStages
		evidence.Iterations = total.Iterations
		evidence.TotalIterations = total.Iterations
		return system, solution, evidence, states, true
	}
	return mnaSystem{}, nil, total, nil, false
}

func mosfetActiveSetConsistent(plan Plan, system mnaSystem, solution []complex128, states map[string]float64) bool {
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveNMOSSwitchV1 && device.PrimitiveModel != PrimitivePMOSSwitchV1 {
			continue
		}
		state, exists := states[device.Component]
		if !exists {
			return false
		}
		terminals := terminalMap(device)
		parameters := namedValueMap(device.ModelParameters)
		gate := nonlinearNodeVoltage(&system, solution, terminals["GATE"])
		source := nonlinearNodeVoltage(&system, solution, terminals["SOURCE"])
		resolvedOn := mosfetSwitchOn(device.PrimitiveModel, gate, source, parameters["gate_on_voltage_v"])
		if resolvedOn != (state >= .5) {
			return false
		}
	}
	return true
}

func nonlinearContinuationMidpoint(stages []continuationStage, stageIndex int) (continuationStage, bool) {
	if stageIndex <= 0 || stageIndex >= len(stages) {
		return continuationStage{}, false
	}
	previous, target := stages[stageIndex-1], stages[stageIndex]
	if previous.gainScale > 0 && target.gainScale/previous.gainScale > nonlinearMinGminRatio {
		return continuationStage{sourceScale: target.sourceScale, gmin: target.gmin, gainScale: math.Sqrt(previous.gainScale * target.gainScale)}, true
	}
	if target.sourceScale-previous.sourceScale > nonlinearMinSourceStep {
		return continuationStage{sourceScale: (previous.sourceScale + target.sourceScale) / 2, gmin: previous.gmin, gainScale: previous.gainScale}, true
	}
	if previous.sourceScale == target.sourceScale && previous.gmin/target.gmin > nonlinearMinGminRatio {
		return continuationStage{sourceScale: target.sourceScale, gmin: math.Sqrt(previous.gmin * target.gmin), gainScale: target.gainScale}, true
	}
	return continuationStage{}, false
}

func requiresNonlinearLineSearch(devices []compiledNonlinearDevice) bool {
	for _, device := range devices {
		switch device.primitive {
		case PrimitiveDiodeShockleyV1, PrimitiveUnidirectionalZenerV1, PrimitiveBidirectionalTVSV1, PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			return true
		}
	}
	return false
}

// maxNonlinearControlVoltageUpdate measures Newton movement in the terminal
// differences that control nonlinear device current. Large common-mode node
// movement is harmless to an exponential junction and must not consume the
// bounded iteration budget one absolute-node increment at a time.
func maxNonlinearControlVoltageUpdate(devices []compiledNonlinearDevice, system *mnaSystem, before, after []complex128) float64 {
	maximum := 0.0
	voltageUpdate := func(first, second string) {
		beforeVoltage := nonlinearNodeVoltage(system, before, first) - nonlinearNodeVoltage(system, before, second)
		afterVoltage := nonlinearNodeVoltage(system, after, first) - nonlinearNodeVoltage(system, after, second)
		maximum = math.Max(maximum, math.Abs(afterVoltage-beforeVoltage))
	}
	for _, device := range devices {
		switch device.primitive {
		case PrimitiveDiodeShockleyV1, PrimitiveUnidirectionalZenerV1, PrimitiveBidirectionalTVSV1:
			voltageUpdate(device.terminals["ANODE"], device.terminals["CATHODE"])
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			voltageUpdate(device.terminals["BASE"], device.terminals["EMITTER"])
			voltageUpdate(device.terminals["BASE"], device.terminals["COLLECTOR"])
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			voltageUpdate(device.terminals["GATE"], device.terminals["SOURCE"])
		}
	}
	return maximum
}

func nonlinearUnknownContext(plan Plan, label string) string {
	if !strings.HasPrefix(label, "node:") {
		return ""
	}
	node := strings.TrimPrefix(label, "node:")
	var terminals []string
	for _, device := range plan.Devices {
		for _, terminal := range device.Terminals {
			if terminal.Net == node {
				terminals = append(terminals, device.Component+"."+terminal.Terminal)
			}
		}
	}
	slices.Sort(terminals)
	if len(terminals) == 0 {
		return ""
	}
	return "[" + strings.Join(terminals, ",") + "]"
}

// piecewiseLinearRegionStable identifies a Newton candidate that is already
// the exact solution of the same catalog-backed affine device regions used to
// build its matrix. Damping such a step only magnifies high-gain branch-current
// updates and can prevent an otherwise exact op-amp/TVS operating point from
// reaching the bounded residual tolerance. Smooth exponential devices remain
// on the conservative damped path.
func piecewiseLinearRegionStable(devices []compiledNonlinearDevice, system *mnaSystem, before, after []complex128) bool {
	if len(devices) == 0 || len(before) != len(after) {
		return false
	}
	for _, device := range devices {
		switch device.primitive {
		case PrimitiveBidirectionalTVSV1:
			region := func(solution []complex128) int {
				voltage := nonlinearNodeVoltage(system, solution, device.terminals["ANODE"]) - nonlinearNodeVoltage(system, solution, device.terminals["CATHODE"])
				breakdown := device.parameters["breakdown_voltage_v"]
				if voltage > breakdown {
					return 1
				}
				if voltage < -breakdown {
					return -1
				}
				return 0
			}
			if region(before) != region(after) {
				return false
			}
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			if _, forced := device.parameters[parameterForcedMOSFETState]; forced {
				continue
			}
			isOn := func(solution []complex128) bool {
				gate := nonlinearNodeVoltage(system, solution, device.terminals["GATE"])
				source := nonlinearNodeVoltage(system, solution, device.terminals["SOURCE"])
				return device.polarity*(gate-source) >= device.parameters["gate_on_voltage_v"]
			}
			if isOn(before) != isOn(after) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func compileNonlinearDevices(plan Plan) []compiledNonlinearDevice {
	return compileNonlinearDevicesWithStates(plan, nil)
}

func compileNonlinearDevicesWithStates(plan Plan, states map[string]float64) []compiledNonlinearDevice {
	var devices []compiledNonlinearDevice
	for _, device := range plan.Devices {
		polarity := 0.0
		switch device.PrimitiveModel {
		case PrimitiveDiodeShockleyV1:
			polarity = 1
		case PrimitiveUnidirectionalZenerV1:
			polarity = 1
		case PrimitiveNMOSSwitchV1:
			polarity = 1
		case PrimitivePMOSSwitchV1:
			polarity = -1
		case PrimitiveBidirectionalTVSV1:
			polarity = 1
		case PrimitiveBJTNPNV1:
			polarity = 1
		case PrimitiveBJTPNPV1:
			polarity = -1
		case PrimitiveBidirectionalOpenDrainTranslatorV1:
			polarity = 1
		default:
			continue
		}
		parameters := namedValueMap(device.ModelParameters)
		if device.PrimitiveModel == PrimitiveNMOSSwitchV1 || device.PrimitiveModel == PrimitivePMOSSwitchV1 {
			if state, exists := states[device.Component]; exists {
				parameters[parameterForcedMOSFETState] = state
			}
		}
		devices = append(devices, compiledNonlinearDevice{
			component: device.Component, primitive: device.PrimitiveModel,
			terminals: terminalMap(device), parameters: parameters, polarity: polarity,
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
	// Independent voltage/current-source primitives are driven exclusively by
	// Analysis.Excitations, which were scaled above. Only intrinsic sources
	// encoded in device model parameters need a cloned plan.
	scaledPlan := planWithIntrinsicSourceContinuationScale(plan, stage.sourceScale)
	scaledPlan = planWithOpAmpGainScale(scaledPlan, stage.gainScale)
	system, diagnostics := buildMNASystemWithOpAmpClamps(scaledPlan, scaled, 0, comparatorStates)
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

func planWithIntrinsicSourceContinuationScale(plan Plan, scale float64) Plan {
	if scale <= 0 || scale == 1 {
		return plan
	}
	clone := ClonePlan(plan)
	for deviceIndex := range clone.Devices {
		device := &clone.Devices[deviceIndex]
		switch device.PrimitiveModel {
		case PrimitiveProgrammableCurrentSourceV1:
			for parameterIndex := range device.ModelParameters {
				switch device.ModelParameters[parameterIndex].Name {
				case "reference_current_a", "offset_voltage_v":
					device.ModelParameters[parameterIndex].Value *= scale
				}
			}
		case PrimitiveShuntVoltageReferenceV1:
			for parameterIndex := range device.ModelParameters {
				if device.ModelParameters[parameterIndex].Name == "output_voltage_v" {
					device.ModelParameters[parameterIndex].Value *= scale
				}
			}
		}
	}
	return clone
}

func planWithOpAmpGainScale(plan Plan, scale float64) Plan {
	if scale <= 0 || scale == 1 {
		return plan
	}
	clone := ClonePlan(plan)
	for deviceIndex := range clone.Devices {
		device := &clone.Devices[deviceIndex]
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		for parameterIndex := range device.ModelParameters {
			if device.ModelParameters[parameterIndex].Name == "dc_open_loop_gain" {
				device.ModelParameters[parameterIndex].Value *= scale
			}
		}
	}
	return clone
}

func resolvedActiveDeviceStates(plan Plan, system mnaSystem, solution []complex128) (map[string]float64, string, *Diagnostic) {
	states := map[string]float64{}
	var key strings.Builder
	for _, device := range plan.Devices {
		parameters := namedValueMap(device.ModelParameters)
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveComparatorOpenCollectorV1:
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
		case PrimitiveOpAmpV1:
			negative := nonlinearNodeVoltage(&system, solution, terminals["V_MINUS"])
			positive := nonlinearNodeVoltage(&system, solution, terminals["V_PLUS"])
			supply := positive - negative
			if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
				return states, key.String(), &Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog op-amp"}
			}
			minimum := negative + parameters["output_low_margin_v"]
			maximum := positive - parameters["output_high_margin_v"]
			differential := nonlinearNodeVoltage(&system, solution, terminals["IN_PLUS"]) - nonlinearNodeVoltage(&system, solution, terminals["IN_MINUS"])
			desired := parameters["dc_open_loop_gain"] * differential
			tolerance := mnaPivotTolerance * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
			switch {
			case desired < minimum-tolerance:
				states[device.Component] = minimum
				key.WriteString(device.Component + ":low;")
			case desired > maximum+tolerance:
				states[device.Component] = maximum
				key.WriteString(device.Component + ":high;")
			default:
				key.WriteString(device.Component + ":linear;")
			}
		case PrimitiveCurrentSenseAmplifierV1:
			supply, minimum, maximum, desired := currentSenseOperatingState(device, system, solution)
			if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
				return states, key.String(), &Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog current-sense amplifier"}
			}
			tolerance := mnaPivotTolerance * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
			switch {
			case desired < minimum-tolerance:
				states[device.Component] = minimum
				key.WriteString(device.Component + ":low;")
			case desired > maximum+tolerance:
				states[device.Component] = maximum
				key.WriteString(device.Component + ":high;")
			default:
				key.WriteString(device.Component + ":linear;")
			}
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
		case PrimitiveUnidirectionalZenerV1:
			stampNonlinearZener(system, device, guess)
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			stampNonlinearMOSFETSwitch(system, device, guess)
		case PrimitiveBidirectionalTVSV1:
			stampNonlinearTVS(system, device, guess)
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			stampNonlinearBJT(system, device, guess)
		case PrimitiveBidirectionalOpenDrainTranslatorV1:
			stampNonlinearOpenDrainTranslator(system, device, guess)
		}
	}
}

func stampSmallSignalNonlinearDevices(system *mnaSystem, devices []compiledNonlinearDevice, operatingSystem *mnaSystem, operatingPoint []complex128, frequency float64) {
	stampSmallSignalNonlinearDevicesExcept(system, devices, operatingSystem, operatingPoint, frequency, "")
}

func stampSmallSignalNonlinearDevicesExcept(system *mnaSystem, devices []compiledNonlinearDevice, operatingSystem *mnaSystem, operatingPoint []complex128, frequency float64, excludedComponent string) {
	for _, device := range devices {
		if device.component == excludedComponent {
			continue
		}
		switch device.primitive {
		case PrimitiveDiodeShockleyV1:
			voltage := nonlinearNodeVoltage(operatingSystem, operatingPoint, device.terminals["ANODE"]) - nonlinearNodeVoltage(operatingSystem, operatingPoint, device.terminals["CATHODE"])
			_, conductance := diodeCurrentAndGradient(voltage, device.parameters)
			stampAdmittance(system, device.terminals["ANODE"], device.terminals["CATHODE"], complex(conductance, 0))
		case PrimitiveUnidirectionalZenerV1:
			voltage := nonlinearNodeVoltage(operatingSystem, operatingPoint, device.terminals["ANODE"]) - nonlinearNodeVoltage(operatingSystem, operatingPoint, device.terminals["CATHODE"])
			_, conductance := unidirectionalZenerCurrentAndGradient(voltage, device.parameters)
			stampAdmittance(system, device.terminals["ANODE"], device.terminals["CATHODE"], complex(conductance, 0))
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			conductance := mosfetSwitchConductance(device, operatingSystem, operatingPoint)
			stampAdmittance(system, device.terminals["DRAIN"], device.terminals["SOURCE"], complex(conductance, 0))
		case PrimitiveBidirectionalTVSV1:
			voltage := nonlinearNodeVoltage(operatingSystem, operatingPoint, device.terminals["ANODE"]) - nonlinearNodeVoltage(operatingSystem, operatingPoint, device.terminals["CATHODE"])
			_, conductance := bidirectionalTVSCurrentAndGradient(voltage, device.parameters)
			capacitance := device.parameters["junction_capacitance_f"]
			stampAdmittance(system, device.terminals["ANODE"], device.terminals["CATHODE"], complex(conductance, 2*math.Pi*frequency*capacitance))
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			nodes := [3]string{device.terminals["BASE"], device.terminals["COLLECTOR"], device.terminals["EMITTER"]}
			_, jacobian := bjtCurrentsAndJacobian(
				nonlinearNodeVoltage(operatingSystem, operatingPoint, nodes[0]),
				nonlinearNodeVoltage(operatingSystem, operatingPoint, nodes[1]),
				nonlinearNodeVoltage(operatingSystem, operatingPoint, nodes[2]),
				device.parameters, device.polarity,
			)
			baseControl := complex(1, 0)
			if transition := device.parameters["transition_frequency_hz"]; transition > 0 && frequency > 0 {
				betaPole := transition / device.parameters["forward_beta"]
				baseControl /= complex(1, frequency/betaPole)
			}
			for row, rowNode := range nodes {
				rowIndex, rowKnown := system.nodeIndex[rowNode]
				if !rowKnown {
					continue
				}
				for column, columnNode := range nodes {
					if columnIndex, columnKnown := system.nodeIndex[columnNode]; columnKnown {
						value := complex(jacobian[row][column], 0)
						if column == 0 {
							value *= baseControl
						}
						system.matrix[rowIndex][columnIndex] += value
					}
				}
			}
		case PrimitiveBidirectionalOpenDrainTranslatorV1:
			for channel := 1; channel <= 2; channel++ {
				resistance := translatorChannelResistance(device, operatingSystem, operatingPoint, channel)
				stampAdmittance(system, device.terminals[fmt.Sprintf("A%d", channel)], device.terminals[fmt.Sprintf("B%d", channel)], complex(1/resistance, 0))
			}
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

func unidirectionalZenerCurrentAndGradient(voltage float64, parameters map[string]float64) (float64, float64) {
	forwardCurrent, forwardGradient := seriesDiodeCurrentAndGradient(
		voltage,
		parameters["forward_saturation_current_a"],
		parameters["forward_emission_coefficient"],
		parameters["junction_temperature_k"],
		parameters["forward_series_resistance_ohm"],
	)
	reverseVoltage := -voltage - parameters["zener_offset_voltage_v"]
	reverseCurrent, reverseGradient := seriesDiodeCurrentAndGradient(
		reverseVoltage,
		parameters["reverse_saturation_current_a"],
		parameters["reverse_emission_coefficient"],
		parameters["junction_temperature_k"],
		parameters["reverse_series_resistance_ohm"],
	)
	return forwardCurrent - reverseCurrent, forwardGradient + reverseGradient
}

func seriesDiodeCurrentAndGradient(voltage, saturationCurrent, emissionCoefficient, temperatureK, seriesResistance float64) (float64, float64) {
	thermal := boltzmannConstant * temperatureK / electronCharge
	saturationCurrent = siliconSaturationCurrentAtTemperature(saturationCurrent, emissionCoefficient, temperatureK)
	junctionVoltage := voltage
	if seriesResistance > 0 {
		lower, upper := math.Min(voltage, 0), math.Max(voltage, 0)
		for iteration := 0; iteration < 64; iteration++ {
			junctionVoltage = .5 * (lower + upper)
			exponential, _ := boundedExponential(junctionVoltage / (emissionCoefficient * thermal))
			current := saturationCurrent * (exponential - 1)
			if junctionVoltage+seriesResistance*current < voltage {
				lower = junctionVoltage
			} else {
				upper = junctionVoltage
			}
		}
	}
	exponential, derivative := boundedExponential(junctionVoltage / (emissionCoefficient * thermal))
	current := saturationCurrent * (exponential - 1)
	junctionGradient := saturationCurrent * derivative / (emissionCoefficient * thermal)
	return current, junctionGradient / (1 + seriesResistance*junctionGradient)
}

func stampNonlinearZener(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	anode, cathode := device.terminals["ANODE"], device.terminals["CATHODE"]
	voltage := nonlinearNodeVoltage(system, guess, anode) - nonlinearNodeVoltage(system, guess, cathode)
	current, gradient := unidirectionalZenerCurrentAndGradient(voltage, device.parameters)
	stampAdmittance(system, anode, cathode, complex(gradient, 0))
	stampCurrentSource(system, anode, cathode, complex(current-gradient*voltage, 0))
}

func mosfetSwitchConductance(device compiledNonlinearDevice, system *mnaSystem, solution []complex128) float64 {
	if state, forced := device.parameters[parameterForcedMOSFETState]; forced {
		if state >= .5 {
			return 1 / device.parameters["on_resistance_ohm"]
		}
		return 0
	}
	gate := nonlinearNodeVoltage(system, solution, device.terminals["GATE"])
	source := nonlinearNodeVoltage(system, solution, device.terminals["SOURCE"])
	if device.polarity*(gate-source) < device.parameters["gate_on_voltage_v"] {
		return 0
	}
	return 1 / device.parameters["on_resistance_ohm"]
}

func mosfetPolarity(primitive string) float64 {
	if primitive == PrimitivePMOSSwitchV1 {
		return -1
	}
	return 1
}

func mosfetSwitchOn(primitive string, gate, source, threshold float64) bool {
	return mosfetPolarity(primitive)*(gate-source) >= threshold
}

func stampNonlinearMOSFETSwitch(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	conductance := mosfetSwitchConductance(device, system, guess)
	if conductance == 0 {
		return
	}
	stampAdmittance(system, device.terminals["DRAIN"], device.terminals["SOURCE"], complex(conductance, 0))
}

func thermalVoltage(parameters map[string]float64) float64 {
	return boltzmannConstant * parameters["junction_temperature_k"] / electronCharge
}

func siliconSaturationCurrentAtTemperature(nominal, emissionCoefficient, temperatureK float64) float64 {
	ratio := temperatureK / nonlinearNominalTemperatureK
	boltzmannEVPerK := boltzmannConstant / electronCharge
	activation := -siliconBandgapEnergyEV / (emissionCoefficient * boltzmannEVPerK) * (1/temperatureK - 1/nonlinearNominalTemperatureK)
	return nominal * math.Pow(ratio, siliconSaturationExponent/emissionCoefficient) * math.Exp(activation)
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
	saturationCurrent := siliconSaturationCurrentAtTemperature(parameters["saturation_current_a"], parameters["emission_coefficient"], parameters["junction_temperature_k"])
	return saturationCurrent * (exponential - 1), saturationCurrent * derivative / scale
}

func stampNonlinearDiode(system *mnaSystem, device compiledNonlinearDevice, guess []complex128) {
	voltage := nonlinearNodeVoltage(system, guess, device.terminals["ANODE"]) - nonlinearNodeVoltage(system, guess, device.terminals["CATHODE"])
	current, conductance := diodeCurrentAndGradient(voltage, device.parameters)
	stampAdmittance(system, device.terminals["ANODE"], device.terminals["CATHODE"], complex(conductance, 0))
	stampCurrentSource(system, device.terminals["ANODE"], device.terminals["CATHODE"], complex(current-conductance*voltage, 0))
}

func bjtCurrentsAndJacobian(vb, vc, ve float64, parameters map[string]float64, polarity float64) ([3]float64, [3][3]float64) {
	vt := parameters["emission_coefficient"] * thermalVoltage(parameters)
	is := siliconSaturationCurrentAtTemperature(parameters["saturation_current_a"], parameters["emission_coefficient"], parameters["junction_temperature_k"])
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
	scales := make([]float64, len(base.rhs))
	for row := range base.rhs {
		residuals[row] = -base.rhs[row]
		scales[row] = math.Abs(real(base.rhs[row]))
		for column := range solution {
			term := base.matrix[row][column] * solution[column]
			residuals[row] += term
			scales[row] += math.Abs(real(term))
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
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			conductance := mosfetSwitchConductance(device, &base, solution)
			current := conductance * (nonlinearNodeVoltage(&base, solution, device.terminals["DRAIN"]) - nonlinearNodeVoltage(&base, solution, device.terminals["SOURCE"]))
			if index, exists := base.nodeIndex[device.terminals["DRAIN"]]; exists {
				residuals[index] += complex(current, 0)
			}
			if index, exists := base.nodeIndex[device.terminals["SOURCE"]]; exists {
				residuals[index] -= complex(current, 0)
			}
		case PrimitiveUnidirectionalZenerV1:
			voltage := nonlinearNodeVoltage(&base, solution, device.terminals["ANODE"]) - nonlinearNodeVoltage(&base, solution, device.terminals["CATHODE"])
			current, _ := unidirectionalZenerCurrentAndGradient(voltage, device.parameters)
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
		case PrimitiveBidirectionalOpenDrainTranslatorV1:
			addOpenDrainTranslatorResidual(residuals, base, device, solution)
		}
	}
	maximum, label := 0.0, "unknown"
	for row, residual := range residuals {
		scale := math.Max(1, scales[row])
		if magnitude := math.Abs(real(residual)) / scale; magnitude > maximum {
			maximum, label = magnitude, base.unknownLabels[row]
		}
	}
	return maximum, label
}

func validateNonlinearOperatingLimits(plan Plan, system mnaSystem, solution []complex128) []Diagnostic {
	return validateNonlinearOperatingLimitsWithComparatorStates(plan, system, solution, nil, false)
}

func validateNonlinearOperatingLimitsWithComparatorStates(plan Plan, system mnaSystem, solution []complex128, comparatorStates map[string]float64, allowPowerTransition bool) []Diagnostic {
	diagnostics := validateResolvedOperatingLimits(plan, system, solution, allowPowerTransition)
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
		case PrimitiveUnidirectionalZenerV1:
			voltage := nonlinearNodeVoltage(&system, solution, terminals["ANODE"]) - nonlinearNodeVoltage(&system, solution, terminals["CATHODE"])
			current, _ := unidirectionalZenerCurrentAndGradient(voltage, parameters)
			if math.Abs(current) > parameters["max_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("Zener current %.12g A exceeds catalog-backed limit %.12g A", math.Abs(current), parameters["max_current_a"]), Suggestion: "increase series resistance, reduce source voltage, or select a suitably rated reviewed Zener"})
			}
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			gate := nonlinearNodeVoltage(&system, solution, terminals["GATE"])
			drain := nonlinearNodeVoltage(&system, solution, terminals["DRAIN"])
			source := nonlinearNodeVoltage(&system, solution, terminals["SOURCE"])
			conductance := mosfetSwitchConductance(compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters, polarity: mosfetPolarity(device.PrimitiveModel)}, &system, solution)
			current := math.Abs((drain - source) * conductance)
			if current > parameters["max_drain_current_a"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("MOSFET drain current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_drain_current_a"]), Suggestion: "increase load impedance, reduce drive, or select a suitably rated reviewed MOSFET"})
			}
			if math.Abs(drain-source) > parameters["max_drain_source_voltage_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("MOSFET drain-source voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(drain-source), parameters["max_drain_source_voltage_v"]), Suggestion: "reduce supply voltage or select a suitably rated reviewed MOSFET"})
			}
			if math.Abs(gate-source) > parameters["max_gate_source_voltage_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("MOSFET gate-source voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(gate-source), parameters["max_gate_source_voltage_v"]), Suggestion: "reduce gate drive or select a suitably rated reviewed MOSFET"})
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
			powerTransition := allowPowerTransition && supply < parameters["supply_min_v"]
			if !powerTransition && (supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"]) {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog comparator"})
			}
			on := comparatorOn(device, system, solution)
			if state, exists := comparatorStates[device.Component]; exists {
				on = state >= .5
			}
			if on && !powerTransition {
				voltage := nonlinearNodeVoltage(&system, solution, terminals["OUT"]) - nonlinearNodeVoltage(&system, solution, terminals["V_MINUS"])
				current := math.Abs(voltage / parameters["output_on_resistance_ohm"])
				if current > parameters["max_sink_current_a"] {
					diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("comparator sink current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_sink_current_a"]), Suggestion: "increase output pull-up resistance or select a compatible reviewed comparator"})
				}
			}
		case PrimitiveBidirectionalOpenDrainTranslatorV1:
			diagnostics = append(diagnostics, validateOpenDrainTranslatorOperatingLimits(device, system, solution, allowPowerTransition)...)
		}
	}
	return diagnostics
}
