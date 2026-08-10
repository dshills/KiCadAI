package simmodel

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	transientGmin = nonlinearFinalGmin
	// A transient switching step can legitimately move a high-voltage node
	// farther than nonlinearMaxIterations*nonlinearMaxNodeUpdateV. Keep the
	// trusted 250 mV damping bound, but give a step enough deterministic work
	// to traverse the full supported 30 V control/load envelope.
	transientMaxNewtonIterations = 256
	// Source continuation also observes the 250 mV bounded update scale for
	// the supported 5 V control envelope. This avoids asking the first
	// continuation stage to cross several nonlinear switch regions at once.
	transientSourceContinuationStages        = 20
	transientMaxNewtonAttemptsPerObservation = 2 * (1 + transientSourceContinuationStages)
	transientActiveLimitContinuationStepV    = 1
	bjtStateSeedDeviceLimit                  = 4
	transientBJTSeedProbeInterval            = 256
	transientPreferredBJTSeedIterations      = 32
	maxClockPhaseCycles                      = 1e9
)

// transientFuseI2TState tracks one contiguous overload pulse. Datasheet
// nominal melting I²t is integral(I² dt), so above-rated samples contribute
// their full current-squared energy. A below-rated sample ends the pulse and
// resets the state; exactly rated current neither heats this pulse model nor
// claims recovery. This deliberate hard boundary means solver chatter below
// the rating also ends the pulse. Long-term cooling, thermal memory, or
// behavior near that boundary requires a separately reviewed time-current or
// thermal fuse model.
type transientFuseI2TState struct {
	integralA2S float64
}

func advanceTransientFuseI2T(state transientFuseI2TState, currentA, ratedCurrentA, timeStepS float64) transientFuseI2TState {
	if currentA < ratedCurrentA {
		return transientFuseI2TState{}
	}
	if currentA > ratedCurrentA && timeStepS > 0 {
		state.integralA2S += currentA * currentA * timeStepS
	}
	return state
}

func solveTransientAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	plan = resolveProgrammedClockFrequencies(plan)
	if diagnostics := validateClockPhaseResolution(plan, analysis); len(diagnostics) != 0 {
		return AnalysisResult{ID: analysis.ID, Kind: AnalysisTransient}, diagnostics
	}
	steps := int(math.Round(analysis.DurationS / analysis.TimeStepS))
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisTransient, Points: make([]AnalysisPoint, 0, steps+1)}
	for _, excitation := range analysis.Excitations {
		if excitation.SineFrequencyHz > 0 {
			result.FundamentalFrequencyHz = excitation.SineFrequencyHz
			break
		}
		periodTolerance := math.Max(1e-15, math.Abs(excitation.PulsePeriodS)*1e-12)
		if excitation.PulsePeriodS > 0 && analysis.DurationS+periodTolerance >= excitation.PulsePeriodS {
			result.FundamentalFrequencyHz = 1 / excitation.PulsePeriodS
			break
		}
	}
	if result.FundamentalFrequencyHz == 0 {
		for _, device := range plan.Devices {
			frequency := transientModelParameter(device.ModelParameters, "switching_frequency_hz")
			if frequency > 0 {
				result.FundamentalFrequencyHz = frequency
				break
			}
		}
	}
	// The point labeled t=0 is the left-hand state at the observation
	// boundary. This preserves an explicit initial sample for events whose
	// trigger is exactly zero; the first positive grid point applies them.
	initialTimeS := -analysis.TimeStepS
	initialAnalysis := transientDCAnalysis(analysis, initialTimeS)
	initialPlan := planWithAnalysisOverrides(plan, initialAnalysis)
	initialPlan = planWithRequestedAnalysis(initialPlan, initialAnalysis)
	explicitZeroPowerStart := transientPowerSourcesZeroAtTime(plan, analysis, initialTimeS)
	autonomousZeroEnergyStart := transientRequiresAutonomousStartup(plan, analysis)
	zeroEnergyInitialState := explicitZeroPowerStart || autonomousZeroEnergyStart
	var system mnaSystem
	var solution []complex128
	var initialEvidence SolverEvidence
	var diagnostic *Diagnostic
	if zeroEnergyInitialState {
		// A power-source event that starts at zero has no energized operating
		// point. Constant-current load harnesses and independent control
		// sources must not drive unpowered rails negative or precharge them.
		var initialDiagnostics []Diagnostic
		system, initialDiagnostics = buildTransientTemplate(plan, analysis)
		if len(initialDiagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, 0, 0, initialDiagnostics)
		}
		solution = make([]complex128, len(system.rhs))
		initialCondition := "explicit_zero_power_source_event"
		if autonomousZeroEnergyStart && !explicitZeroPowerStart {
			initialCondition = "autonomous_zero_energy_startup"
		}
		initialEvidence = SolverEvidence{Method: "zero_energy_transient_v1", InitialCondition: initialCondition}
	} else {
		// The trusted DC initializer applies global source/gmin continuation and
		// ends at nonlinearFinalGmin, exactly the conductance used by later steps.
		system, solution, initialEvidence, diagnostic = solveNonlinearDCForPowerTransition(initialPlan, initialAnalysis)
		if diagnostic != nil {
			diagnostic.Path = "analyses." + analysis.ID + ".initial_condition." + diagnostic.Path
			diagnostic.Message = "deterministic transient initial condition failed: " + diagnostic.Message
			return result, []Diagnostic{*diagnostic}
		}
		initialEvidence.InitialCondition = "bounded_nonlinear_dc_v1"
	}
	if shapeDiagnostic := validateMNASolutionShape(system, solution); shapeDiagnostic != nil {
		shapeDiagnostic.Path = "analyses." + analysis.ID + ".initial_condition." + shapeDiagnostic.Path
		return result, []Diagnostic{*shapeDiagnostic}
	}
	initialEvidence.TotalIterations = initialEvidence.Iterations
	initialEvidence.MaxIterationsPerStep = transientMaxNewtonIterations
	initialEvidence.MaxTotalIterations = maxTransientWork
	initialStates := map[string]float64{}
	if !zeroEnergyInitialState {
		var initialStateDiagnostic *Diagnostic
		initialStates, _, initialStateDiagnostic = resolvedActiveDeviceStatesWithPowerTransition(initialPlan, system, solution, true)
		if initialStateDiagnostic != nil {
			return result, []Diagnostic{*initialStateDiagnostic}
		}
	}
	_, initialLimitDiagnostics := validateTransientOperatingLimits(initialPlan, system, solution, initialStates, true, 0, nil, nil)
	if len(initialLimitDiagnostics) != 0 && !zeroEnergyInitialState {
		if boundedSystem, boundedSolution, boundedEvidence, ok := solveBoundedTransientInitialCondition(
			initialPlan,
			initialAnalysis,
			solution,
		); ok {
			if _, diagnostics := validateTransientOperatingLimits(initialPlan, boundedSystem, boundedSolution, initialStates, true, 0, nil, nil); len(diagnostics) == 0 {
				system, solution = boundedSystem, boundedSolution
				initialEvidence.Method = "bounded_nonlinear_dc_with_operating_limits_v1"
				initialEvidence.Iterations += boundedEvidence.Iterations
				initialEvidence.TotalIterations = initialEvidence.Iterations
				initialEvidence.FinalMaxUpdateV = boundedEvidence.FinalMaxUpdateV
				initialEvidence.FinalMaxCurrentUpdateA = boundedEvidence.FinalMaxCurrentUpdateA
				initialEvidence.FinalMaxResidual = boundedEvidence.FinalMaxResidual
				initialLimitDiagnostics = nil
			}
		}
	}
	if len(initialLimitDiagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 0, 0, initialLimitDiagnostics)
	}
	result.Points = append(result.Points, AnalysisPoint{
		TimeS: 0, Nodes: nodeResults(plan, system, solution),
		Devices: transientObservationDeviceResults(initialPlan, analysis, initialAnalysis, system, solution, nil, nil, 0, 0, solution, nil),
		Solver:  &initialEvidence,
	})
	history := [][]complex128{append([]complex128(nil), solution...)}

	devices := compileNonlinearDevices(plan)
	totalIterations := initialEvidence.Iterations
	template, diagnostics := buildTransientTemplate(plan, analysis)
	if len(diagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 1, analysis.TimeStepS, diagnostics)
	}
	base := cloneMNASystem(template)
	workspace := cloneMNASystem(template)
	fuseI2TStates := map[string]transientFuseI2TState{}
	openFuses := map[string]bool{}
	preferBJTVoltageSeed := false
	preferredBJTVoltageSeedSteps := 0
	preferredBJTVoltageSeed := -1
	for step := 1; step <= steps; step++ {
		// Derive time directly from the integer grid index; never accumulate it.
		timeS := float64(step) * analysis.TimeStepS
		comparatorStates, fixedOutputClamps, diagnostics := prepareTransientBase(&base, template, plan, analysis, step, timeS, solution, history, openFuses)
		if len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		guess := transientInitialGuess(analysis, timeS, solution, history)
		var evidence SolverEvidence
		previousSolution := solution
		predictorValidated := false
		var predictorFuseStates map[string]transientFuseI2TState
		if preferBJTVoltageSeed && preferredBJTVoltageSeedSteps >= transientBJTSeedProbeInterval {
			preferBJTVoltageSeed = false
			preferredBJTVoltageSeedSteps = 0
		}
		if preferBJTVoltageSeed {
			var seeded bool
			var preferredEvidence SolverEvidence
			system, solution, preferredEvidence, seeded, preferredBJTVoltageSeed = solveTransientStepByBJTVoltageSeed(
				base, plan.Devices, devices, previousSolution, previousSolution, &workspace, fixedOutputClamps, preferredBJTVoltageSeed,
			)
			if seeded {
				evidence = preferredEvidence
				preferredBJTVoltageSeedSteps++
				diagnostic = nil
			} else {
				preferBJTVoltageSeed = false
				preferredBJTVoltageSeedSteps = 0
				preferredBJTVoltageSeed = -1
				system, solution, evidence, diagnostic = solveTransientStep(
					base, plan.Devices, devices, previousSolution, guess, &workspace, false, fixedOutputClamps,
				)
				evidence.Iterations += preferredEvidence.Iterations
				evidence.TotalIterations = evidence.Iterations
			}
		} else {
			system, solution, evidence, diagnostic = solveTransientStep(
				base, plan.Devices, devices, previousSolution, guess, &workspace, false, fixedOutputClamps,
			)
		}
		totalIterations += evidence.Iterations
		if diagnostic != nil {
			var seedEvidence SolverEvidence
			var seeded bool
			system, solution, seedEvidence, seeded, preferredBJTVoltageSeed = solveTransientStepByBJTVoltageSeed(
				base, plan.Devices, devices, previousSolution, previousSolution, &workspace, fixedOutputClamps, preferredBJTVoltageSeed,
			)
			if !seeded {
				var stateEvidence SolverEvidence
				system, solution, stateEvidence, seeded = solveTransientStepByBJTStateSeed(
					base, plan.Devices, devices, previousSolution, previousSolution, &workspace, fixedOutputClamps,
				)
				seedEvidence.Iterations += stateEvidence.Iterations
				seedEvidence.TotalIterations = seedEvidence.Iterations
				seedEvidence.FinalMaxUpdateV = stateEvidence.FinalMaxUpdateV
				seedEvidence.FinalMaxCurrentUpdateA = stateEvidence.FinalMaxCurrentUpdateA
				seedEvidence.FinalMaxResidual = stateEvidence.FinalMaxResidual
				if seeded {
					seedEvidence.Method = stateEvidence.Method
				}
			}
			totalIterations += seedEvidence.Iterations
			if seeded {
				evidence = seedEvidence
				preferBJTVoltageSeed = seedEvidence.Method == "backward_euler_bounded_bjt_voltage_seed_v1"
				preferredBJTVoltageSeedSteps = 0
				diagnostic = nil
			}
		}
		if diagnostic != nil {
			priorBase, priorDiagnostics, prepared := prepareAcceptedPriorTransientBase(
				template, plan, analysis, step, timeS, history, openFuses,
			)
			if prepared && len(priorDiagnostics) == 0 {
				var continuationEvidence SolverEvidence
				system, solution, continuationEvidence, diagnostic = solveTransientStepWithSourceContinuation(priorBase, base, plan.Devices, devices, previousSolution, &workspace, fixedOutputClamps)
				totalIterations += continuationEvidence.Iterations
				evidence = continuationEvidence
			}
		}
		if diagnostic != nil && transientAcceptedSubstepsApplicable(plan) {
			var predictorEvidence SolverEvidence
			var predicted bool
			system, solution, predictorEvidence, predictorFuseStates, predicted = solveTransientStepBySubstepPredictor(
				plan, analysis, timeS, plan.Devices, devices, previousSolution,
				openFuses, fuseI2TStates,
			)
			totalIterations += predictorEvidence.Iterations
			if predicted {
				evidence = predictorEvidence
				predictorValidated = true
				diagnostic = nil
			}
		}
		if diagnostic != nil {
			seedAnalysis := transientDCAnalysis(analysis, timeS)
			seedPlan := planWithAnalysisOverrides(plan, seedAnalysis)
			seedPlan = planWithRequestedAnalysis(seedPlan, seedAnalysis)
			_, seed, seedEvidence, seedDiagnostic := solveNonlinearDCForPowerTransition(seedPlan, seedAnalysis)
			totalIterations += seedEvidence.Iterations
			if seedDiagnostic == nil && len(seed) == len(previousSolution) {
				var reseedEvidence SolverEvidence
				system, solution, reseedEvidence, diagnostic = solveTransientStep(
					base, plan.Devices, devices, previousSolution, seed, &workspace, false, fixedOutputClamps,
				)
				totalIterations += reseedEvidence.Iterations
				reseedEvidence.Method = "backward_euler_bounded_dc_reseed_v1"
				evidence = reseedEvidence
				if diagnostic != nil {
					diagnostic.Message = "instantaneous-DC reseed failed to close the bounded dynamic step: " + diagnostic.Message
				}
			} else if seedDiagnostic != nil {
				diagnostic.Message += "; instantaneous-DC reseed unavailable: " + seedDiagnostic.Message
			} else {
				diagnostic.Message += fmt.Sprintf("; instantaneous-DC reseed returned %d unknowns for a %d-unknown dynamic system", len(seed), len(previousSolution))
			}
		}
		evidence.InitialCondition = "previous_accepted_state"
		evidence.TimeSteps = step
		evidence.TotalIterations = totalIterations
		evidence.MaxIterationsPerStep = transientMaxNewtonIterations * transientMaxNewtonAttemptsPerObservation
		evidence.MaxTotalIterations = maxTransientWork
		if diagnostic != nil {
			diagnostic.Path = fmt.Sprintf("analyses.%s.points[%d].%s", analysis.ID, step, diagnostic.Path)
			diagnostic.Message = fmt.Sprintf("transient solve failed at step %d, time %.12g s: %s", step, timeS, diagnostic.Message)
			return result, []Diagnostic{*diagnostic}
		}
		if totalIterations > maxTransientWork {
			return result, []Diagnostic{{Path: fmt.Sprintf("analyses.%s.points[%d].work", analysis.ID, step), Message: fmt.Sprintf("transient solve exceeded bounded total work limit %d", maxTransientWork), Suggestion: "reduce the bounded observation duration or partition the analysis"}}
		}
		candidateFuseStates := cloneTransientFuseI2TStates(fuseI2TStates)
		var openedFuses []string
		diagnostics = nil
		if predictorValidated {
			candidateFuseStates = predictorFuseStates
		} else {
			openedFuses, diagnostics = validateTransientOperatingLimits(plan, system, solution, comparatorStates, true, analysis.TimeStepS, candidateFuseStates, openFuses)
		}
		if len(diagnostics) != 0 && transientAcceptedSubstepsApplicable(plan) {
			var predictorEvidence SolverEvidence
			var predicted bool
			var predictedFuseStates map[string]transientFuseI2TState
			system, solution, predictorEvidence, predictedFuseStates, predicted = solveTransientStepBySubstepPredictor(
				plan, analysis, timeS, plan.Devices, devices, previousSolution,
				openFuses, fuseI2TStates,
			)
			totalIterations += predictorEvidence.Iterations
			if !predicted {
				return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
			}
			evidence = predictorEvidence
			candidateFuseStates = predictedFuseStates
			openedFuses = nil
			// The operating-limit fallback replaces the already-normalized
			// observation evidence. Restore the outer-step counters and bounded
			// work limits so every accepted point carries complete convergence
			// provenance, including work spent by earlier attempts.
			evidence.InitialCondition = "previous_accepted_state"
			evidence.TimeSteps = step
			evidence.TotalIterations = totalIterations
			evidence.MaxIterationsPerStep = transientMaxNewtonIterations * transientMaxNewtonAttemptsPerObservation
			evidence.MaxTotalIterations = maxTransientWork
			if totalIterations > maxTransientWork {
				return result, []Diagnostic{{Path: fmt.Sprintf("analyses.%s.points[%d].work", analysis.ID, step), Message: fmt.Sprintf("transient solve exceeded bounded total work limit %d", maxTransientWork), Suggestion: "reduce the bounded observation duration or partition the analysis"}}
			}
		}
		result.Points = append(result.Points, AnalysisPoint{
			TimeS: normalizedMNAFloat(timeS), Nodes: nodeResults(plan, system, solution),
			Devices: transientObservationDeviceResults(
				plan, analysis, analysis, system, solution, comparatorStates, openFuses,
				step, timeS, previousSolution, history,
			),
			Solver: &evidence,
		})
		fuseI2TStates = commitTransientFuseStep(candidateFuseStates, openFuses, openedFuses)
		history = append(history, append([]complex128(nil), solution...))
	}
	result.PeriodicNodes = synchronousBuckPeriodicNodeResults(plan, result)
	return result, nil
}

func transientRequiresAutonomousStartup(plan Plan, analysis Analysis) bool {
	periodicallyDriven := false
	for _, excitation := range analysis.Excitations {
		periodicallyDriven = periodicallyDriven || excitation.SineFrequencyHz > 0 || excitation.PulsePeriodS > 0
	}
	if periodicallyDriven {
		return false
	}
	for _, assertion := range plan.Assertions {
		if assertion.AnalysisID != analysis.ID {
			continue
		}
		switch assertion.Quantity {
		case QuantityOscillationFrequencyHz, QuantityDutyCyclePct:
			return true
		}
	}
	return false
}

func prepareAcceptedPriorTransientBase(
	template mnaSystem,
	plan Plan,
	analysis Analysis,
	step int,
	timeS float64,
	history [][]complex128,
	openFuses map[string]bool,
) (mnaSystem, []Diagnostic, bool) {
	if step <= 1 || len(history) < 2 {
		return mnaSystem{}, nil, false
	}
	priorBase := cloneMNASystem(template)
	priorState := history[len(history)-2]
	priorHistory := history[:len(history)-1]
	_, _, diagnostics := prepareTransientBase(
		&priorBase, template, plan, analysis, step-1, timeS-analysis.TimeStepS,
		priorState, priorHistory, openFuses,
	)
	return priorBase, diagnostics, true
}

func transientObservationDeviceResults(
	plan Plan,
	observation Analysis,
	evaluation Analysis,
	system mnaSystem,
	solution []complex128,
	comparatorStates map[string]float64,
	openFuses map[string]bool,
	step int,
	timeS float64,
	previous []complex128,
	history [][]complex128,
) []DeviceResult {
	if observation.Kind == AnalysisDistortion {
		needsDeviceEvidence := false
		for _, assertion := range plan.Assertions {
			if assertion.AnalysisID == observation.ID && assertion.Quantity == QuantityOutputPowerW {
				needsDeviceEvidence = true
				break
			}
		}
		if !needsDeviceEvidence {
			return nil
		}
	}
	var digitalOutputStates map[string]bool
	var digitalOutputEnabled map[string]bool
	switch evaluation.Kind {
	case AnalysisTransient, AnalysisElectrothermal, AnalysisStartup:
		digitalOutputStates, digitalOutputEnabled = transientDigitalOutputStates(
			plan, evaluation, &system, step, timeS, previous, history,
		)
	}
	if evaluation.Kind == AnalysisTransient || evaluation.Kind == AnalysisElectrothermal {
		plan = planWithTransientValueEvents(plan, evaluation, timeS)
	}
	results := electricalDeviceResultsWithComparatorStates(
		plan, evaluation, 0, system, solution, comparatorStates, digitalOutputStates, digitalOutputEnabled, openFuses,
	)
	sourceCurrents := make(map[string]float64)
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveCurrentSourceV1 {
			sourceCurrents[device.Component] = transientIndependentSourceValue(evaluation, device.Component, timeS)
		}
	}
	for index := range results {
		current, ok := sourceCurrents[results[index].Component]
		if !ok {
			continue
		}
		results[index].CurrentA = normalizedMNAFloat(current)
		results[index].CurrentMagnitudeA = normalizedMNAFloat(math.Abs(current))
	}
	return results
}

func transientDigitalOutputStates(
	plan Plan,
	analysis Analysis,
	system *mnaSystem,
	step int,
	timeS float64,
	previous []complex128,
	history [][]complex128,
) (map[string]bool, map[string]bool) {
	states := map[string]bool{}
	enabled := map[string]bool{}
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case PrimitiveFixedClockSourceV1, PrimitiveResistorProgrammedClockSourceV1:
			enabled[device.Component] = clockSourceEnabled(device, system, previous)
			states[device.Component] = clockSourceHigh(device, system, analysis, timeS, previous)
		case PrimitiveCMOSBufferV1:
			enabled[device.Component] = true
			states[device.Component] = clockBufferHigh(device, system, analysis, step, previous, history)
		}
	}
	return states, enabled
}

func transientPowerSourcesZeroAtTime(plan Plan, analysis Analysis, timeS float64) bool {
	if len(analysis.Excitations) == 0 {
		return false
	}
	allSourcesZero := true
	for _, excitation := range analysis.Excitations {
		if math.Abs(transientExcitationValue(analysis, excitation.Component, timeS)) > 1e-15 {
			allSourcesZero = false
			break
		}
	}
	if allSourcesZero {
		return true
	}
	powerInputs := transientPowerInputNets(plan)
	if len(powerInputs) == 0 {
		return false
	}
	found := false
	for _, excitation := range analysis.Excitations {
		deviceIndex := slices.IndexFunc(plan.Devices, func(device ResolvedDevice) bool {
			return device.Component == excitation.Component
		})
		if deviceIndex < 0 {
			continue
		}
		device := plan.Devices[deviceIndex]
		terminals := terminalMap(device)
		positive, negative := "", ""
		switch device.PrimitiveModel {
		case PrimitiveVoltageSourceV1:
			positive, negative = terminals["POSITIVE"], terminals["NEGATIVE"]
		case PrimitiveConnectorVoltageSourceV1:
			positive, negative = terminals["PIN_1"], terminals["PIN_2"]
		default:
			continue
		}
		if !powerInputs[positive] && !powerInputs[negative] {
			continue
		}
		found = true
		if math.Abs(transientExcitationValue(analysis, excitation.Component, timeS)) > 1e-15 {
			return false
		}
	}
	return found
}

func transientPowerInputNets(plan Plan) map[string]bool {
	nets := map[string]bool{}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		var names []string
		switch device.PrimitiveModel {
		case PrimitiveSynchronousBuckRegulatorV1:
			names = []string{"PVIN"}
		case PrimitiveAdjustableLinearRegulatorV1, PrimitiveFixedLinearRegulatorV1, PrimitiveFixedBuckModuleV1:
			names = []string{"VIN"}
		case PrimitiveFloatingAdjustableRegulatorV1, PrimitiveProgrammableCurrentSourceV1:
			names = []string{"VIN", "IN"}
		case PrimitiveOpAmpV1, PrimitiveComparatorOpenCollectorV1:
			names = []string{"V_PLUS"}
		case PrimitiveCurrentSenseAmplifierV1:
			names = []string{"VCC"}
		case PrimitiveSingleOutputIsolatedConverterV1, PrimitiveProtectedIsolatedConverterV1, PrimitiveDualOutputIsolatedConverterV1:
			names = []string{"VIN"}
		case PrimitiveMCUStaticSupplyLoadV1, PrimitiveSensorStaticSupplyLoadV1:
			names = []string{"POWER"}
		case PrimitiveFixedClockSourceV1, PrimitiveResistorProgrammedClockSourceV1:
			names = []string{"VDD"}
		case PrimitiveCMOSBufferV1:
			names = []string{"VCC"}
		}
		for _, name := range names {
			if net := terminals[name]; net != "" && net != plan.GroundNode {
				nets[net] = true
			}
		}
	}
	return nets
}

// solveStartupAnalysis applies every bounded DC source after a canonical
// zero-energy point. Unlike ordinary transient analysis, it deliberately does
// not solve a steady-state operating point first: capacitor voltages and all
// algebraic unknowns begin at zero, making power-up overshoot reproducible.
func solveStartupAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	plan = resolveProgrammedClockFrequencies(plan)
	if diagnostics := validateClockPhaseResolution(plan, analysis); len(diagnostics) != 0 {
		return AnalysisResult{ID: analysis.ID, Kind: AnalysisStartup}, diagnostics
	}
	steps := int(math.Round(analysis.DurationS / analysis.TimeStepS))
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisStartup, Points: make([]AnalysisPoint, 0, steps+1)}
	template, diagnostics := buildTransientTemplate(plan, analysis)
	if len(diagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 0, 0, diagnostics)
	}
	previous := make([]complex128, len(template.rhs))
	history := [][]complex128{append([]complex128(nil), previous...)}
	initialEvidence := SolverEvidence{
		Method: "zero_energy_startup_v1", InitialCondition: "all_dynamic_and_algebraic_unknowns_zero",
		MaxIterationsPerStep: transientMaxNewtonIterations, MaxTotalIterations: maxTransientWork,
	}
	result.Points = append(result.Points, AnalysisPoint{
		Nodes: nodeResults(plan, template, previous),
		Devices: transientObservationDeviceResults(
			plan, analysis, analysis, template, previous, nil, nil, 0, 0, previous, history,
		),
		Solver: &initialEvidence,
	})

	devices := compileNonlinearDevices(plan)
	base := cloneMNASystem(template)
	workspace := cloneMNASystem(template)
	totalIterations := 0
	fuseI2TStates := map[string]transientFuseI2TState{}
	openFuses := map[string]bool{}
	for step := 1; step <= steps; step++ {
		timeS := float64(step) * analysis.TimeStepS
		comparatorStates, fixedOutputClamps, diagnostics := prepareTransientBase(&base, template, plan, analysis, step, timeS, previous, history, openFuses)
		if len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		guess := transientInitialGuess(analysis, timeS, previous, history)
		system, solution, evidence, diagnostic := solveTransientStep(base, plan.Devices, devices, previous, guess, &workspace, false, fixedOutputClamps)
		totalIterations += evidence.Iterations
		evidence.InitialCondition = "previous_accepted_startup_state"
		evidence.TimeSteps = step
		evidence.TotalIterations = totalIterations
		evidence.MaxIterationsPerStep = transientMaxNewtonIterations
		evidence.MaxTotalIterations = maxTransientWork
		if diagnostic != nil {
			diagnostic.Path = fmt.Sprintf("analyses.%s.points[%d].%s", analysis.ID, step, diagnostic.Path)
			diagnostic.Message = fmt.Sprintf("startup solve failed at step %d, time %.12g s: %s", step, timeS, diagnostic.Message)
			return result, []Diagnostic{*diagnostic}
		}
		candidateFuseStates := cloneTransientFuseI2TStates(fuseI2TStates)
		openedFuses, diagnostics := validateTransientOperatingLimits(plan, system, solution, comparatorStates, true, analysis.TimeStepS, candidateFuseStates, openFuses)
		if len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		result.Points = append(result.Points, AnalysisPoint{
			TimeS: normalizedMNAFloat(timeS), Nodes: nodeResults(plan, system, solution),
			Devices: transientObservationDeviceResults(
				plan, analysis, analysis, system, solution, comparatorStates, openFuses,
				step, timeS, previous, history,
			),
			Solver: &evidence,
		})
		fuseI2TStates = commitTransientFuseStep(candidateFuseStates, openFuses, openedFuses)
		previous = solution
		history = append(history, append([]complex128(nil), solution...))
	}
	return result, nil
}

func solveBoundedTransientInitialCondition(
	plan Plan,
	analysis Analysis,
	initialSolution []complex128,
) (mnaSystem, []complex128, SolverEvidence, bool) {
	stage := nonlinearContinuation[len(nonlinearContinuation)-1]
	base, diagnostics := buildNonlinearBaseSystem(plan, analysis, stage, nil)
	if len(diagnostics) != 0 {
		return mnaSystem{}, nil, SolverEvidence{}, false
	}
	workspace := cloneMNASystem(base)
	guess := append([]complex128(nil), initialSolution...)
	system, solution, evidence, diagnostic := solveTransientStep(
		base,
		plan.Devices,
		compileNonlinearDevices(plan),
		initialSolution,
		guess,
		&workspace,
		false,
		nil,
	)
	if diagnostic != nil {
		return mnaSystem{}, nil, evidence, false
	}
	return system, solution, evidence, true
}

func peakAbsVoltage(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	peak := 0.0
	found := false
	points := result.Points
	if result.Kind == AnalysisTransient {
		points = periodicSteadyStatePoints(result)
	}
	for _, point := range points {
		for _, node := range point.Nodes {
			if node.Node != assertion.Node {
				continue
			}
			peak = math.Max(peak, math.Abs(node.Real))
			found = true
			break
		}
	}
	if !found {
		return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node, Message: "startup peak assertion did not resolve to a solved node waveform"}
	}
	return normalizedMNAFloat(peak), nil
}

func periodicSteadyStatePoints(result AnalysisResult) []AnalysisPoint {
	if result.FundamentalFrequencyHz <= 0 || len(result.Points) < 2 {
		return result.Points
	}
	timeStepS := result.Points[1].TimeS - result.Points[0].TimeS
	if timeStepS <= 0 {
		return result.Points
	}
	samplesPerCycle := int(math.Round(1 / (result.FundamentalFrequencyHz * timeStepS)))
	window := 2 * samplesPerCycle
	if samplesPerCycle <= 0 || len(result.Points)-1 < window {
		return result.Points
	}
	start := len(result.Points) - 1 - window
	return result.Points[start : start+window]
}

func transientDCAnalysis(analysis Analysis, timeS float64) Analysis {
	dc := Analysis{ID: analysis.ID, Kind: AnalysisDCOperatingPoint, Excitations: append([]SourceExcitation(nil), analysis.Excitations...)}
	dc.Conditions = append([]NamedValue(nil), analysis.Conditions...)
	dc.DeviceOverrides = append([]DeviceOverride(nil), analysis.DeviceOverrides...)
	for index := range dc.Excitations {
		// A periodic source has no unique instantaneous DC operating point.
		// Initialize its dynamic waveform around the declared DC bias instead
		// of freezing the sample immediately before the observation boundary
		// into capacitor and inductor history. Pulsed sources still resolve
		// their left-hand boundary value so power and event transitions retain
		// their deterministic pre-trigger state.
		if dc.Excitations[index].SineFrequencyHz <= 0 {
			dc.Excitations[index].DCValue = transientSourceValue(dc.Excitations[index], timeS, analysis.TimeStepS)
		}
		dc.Excitations[index].DCValue = transientSourceEventValue(
			analysis.SourceValueEvents,
			dc.Excitations[index].Component,
			timeS,
			analysis.TimeStepS,
			dc.Excitations[index].DCValue,
		)
		dc.Excitations[index].ACMagnitude = 0
		dc.Excitations[index].ACPhaseDeg = 0
		dc.Excitations[index].PulseInitialValue = 0
		dc.Excitations[index].PulseValue = 0
		dc.Excitations[index].PulseDelayS = 0
		dc.Excitations[index].PulseWidthS = 0
		dc.Excitations[index].PulsePeriodS = 0
		dc.Excitations[index].SineAmplitude = 0
		dc.Excitations[index].SineFrequencyHz = 0
		dc.Excitations[index].SinePhaseDeg = 0
	}
	for _, event := range analysis.DeviceValueEvents {
		value, applies := transientDeviceEventValue(event, timeS, analysis.TimeStepS)
		if !applies {
			continue
		}
		override := DeviceOverride{Component: event.Component, ValueSI: &value}
		for _, existing := range dc.DeviceOverrides {
			if existing.Component == event.Component {
				override.ModelParameters = append([]NamedValue(nil), existing.ModelParameters...)
				break
			}
		}
		dc.DeviceOverrides = setTransientDeviceOverride(dc.DeviceOverrides, override)
	}
	for _, event := range analysis.ConditionValueEvents {
		value, applies := transientConditionEventValue(event, timeS, analysis.TimeStepS)
		if applies {
			dc.Conditions = setTransientCondition(dc.Conditions, event.Name, value)
		}
	}
	return dc
}

func transientSourceEventValue(events []SourceValueEvent, component string, timeS, timeStepS, fallback float64) float64 {
	value := fallback
	for _, event := range events {
		if event.Component != component {
			continue
		}
		eventValue, terminal := transientEventValue(timeS, timeStepS, event.TriggerTimeS, event.DurationS, event.Initial, event.Applied, event.Recovered)
		value = eventValue
		if terminal {
			break
		}
	}
	return value
}

func transientDeviceValue(events []DeviceValueEvent, component string, timeS, timeStepS, fallback float64) float64 {
	value := fallback
	for _, event := range events {
		if event.Component != component {
			continue
		}
		eventValue, terminal := transientEventValue(timeS, timeStepS, event.TriggerTimeS, event.DurationS, event.InitialSI, event.AppliedSI, event.RecoveredSI)
		value = eventValue
		if terminal {
			break
		}
	}
	return value
}

func transientDeviceEventValue(event DeviceValueEvent, timeS, timeStepS float64) (float64, bool) {
	value, _ := transientEventValue(timeS, timeStepS, event.TriggerTimeS, event.DurationS, event.InitialSI, event.AppliedSI, event.RecoveredSI)
	return value, true
}

func transientConditionEventValue(event ConditionValueEvent, timeS, timeStepS float64) (float64, bool) {
	value, _ := transientEventValue(timeS, timeStepS, event.TriggerTimeS, event.DurationS, event.Initial, event.Applied, event.Recovered)
	return value, true
}

// transientEventValue returns terminal when the requested time is inside or
// before this event. After a completed event, callers continue so a later
// non-overlapping event on the same target can take precedence.
func transientEventValue(timeS, timeStepS, trigger, duration, initial, applied float64, recovered *float64) (value float64, terminal bool) {
	tolerance := math.Max(timeStepS, math.Abs(timeS)) * 1e-12
	if timeS+tolerance < trigger {
		return initial, true
	}
	if timeS < trigger+duration-tolerance {
		return applied, true
	}
	if recovered != nil {
		return *recovered, false
	}
	return applied, false
}

func setTransientDeviceOverride(overrides []DeviceOverride, replacement DeviceOverride) []DeviceOverride {
	result := append([]DeviceOverride(nil), overrides...)
	for index := range result {
		if result[index].Component == replacement.Component {
			result[index] = replacement
			return result
		}
	}
	result = append(result, replacement)
	slices.SortStableFunc(result, func(left, right DeviceOverride) int { return strings.Compare(left.Component, right.Component) })
	return result
}

func setTransientCondition(conditions []NamedValue, name string, value float64) []NamedValue {
	result := append([]NamedValue(nil), conditions...)
	for index := range result {
		if result[index].Name == name {
			result[index].Value = value
			return result
		}
	}
	result = append(result, NamedValue{Name: name, Value: value})
	return normalizeNamedValues(result)
}

func planWithTransientValueEvents(plan Plan, analysis Analysis, timeS float64) Plan {
	if len(analysis.DeviceValueEvents) == 0 {
		return plan
	}
	// Simulation plans are immutable during evaluation. Device value events
	// only replace ValueSI pointers, so copy exactly the Devices slice and
	// retain the plan's read-only topology, analyses, and evidence storage.
	// Nested device evidence is likewise read-only; mutation paths must make
	// their own copy before changing it.
	clone := plan
	clone.Devices = append([]ResolvedDevice(nil), plan.Devices...)
	for index := range clone.Devices {
		device := &clone.Devices[index]
		if device.ValueSI == nil {
			continue
		}
		value := transientDeviceValue(analysis.DeviceValueEvents, device.Component, timeS, analysis.TimeStepS, *device.ValueSI)
		device.ValueSI = &value
	}
	return clone
}

func transientSourceValue(excitation SourceExcitation, timeS, timeStepS float64) float64 {
	if excitation.SineFrequencyHz > 0 {
		phase := excitation.SinePhaseDeg * math.Pi / 180
		return excitation.DCValue + excitation.SineAmplitude*math.Sin(2*math.Pi*excitation.SineFrequencyHz*timeS+phase)
	}
	if excitation.PulsePeriodS <= 0 {
		return excitation.DCValue
	}
	tolerance := math.Max(timeStepS, math.Abs(timeS)) * 1e-12
	if timeS+tolerance < excitation.PulseDelayS {
		return excitation.PulseInitialValue
	}
	phase := math.Mod(timeS-excitation.PulseDelayS, excitation.PulsePeriodS)
	if phase < 0 {
		phase += excitation.PulsePeriodS
	}
	if phase+tolerance < excitation.PulseWidthS {
		return excitation.PulseValue
	}
	return excitation.PulseInitialValue
}

func solveDistortionAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	result, diagnostics := solveTransientAnalysis(plan, analysis)
	result.Kind = AnalysisDistortion
	for _, excitation := range analysis.Excitations {
		if excitation.SineFrequencyHz > 0 {
			result.FundamentalFrequencyHz = excitation.SineFrequencyHz
			break
		}
	}
	return result, diagnostics
}

func totalHarmonicDistortion(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	frequency := result.FundamentalFrequencyHz
	if frequency <= 0 {
		return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID, Message: "distortion assertion has no resolved sine fundamental"}
	}
	if len(result.Points) < 2 || result.Points[1].TimeS <= 0 {
		return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID, Message: "distortion waveform has no positive observation step"}
	}
	samplesPerCycle := int(math.Round(1 / (frequency * result.Points[1].TimeS)))
	window := 2 * samplesPerCycle
	if len(result.Points)-1 < window {
		return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID, Message: "distortion waveform does not contain the trusted two-cycle measurement window"}
	}
	start := len(result.Points) - 1 - window
	values := make([]float64, 0, window)
	for _, point := range result.Points[start : start+window] {
		found := false
		for _, node := range point.Nodes {
			if node.Node == assertion.Node {
				values = append(values, node.Real)
				found = true
				break
			}
		}
		if !found {
			return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node, Message: "distortion assertion did not resolve to a solved waveform"}
		}
	}
	fundamentalBin := 2
	fundamental := dftMagnitude(values, fundamentalBin)
	if fundamental <= 1e-15 || !finite(fundamental) {
		return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node, Message: "distortion fundamental is zero or numerically unresolved", Suggestion: "increase the bounded source amplitude or correct circuit transfer"}
	}
	harmonicPower := 0.0
	for harmonic := 2; harmonic <= 5; harmonic++ {
		bin := fundamentalBin * harmonic
		if bin >= len(values)/2 {
			break
		}
		magnitude := dftMagnitude(values, bin)
		harmonicPower += magnitude * magnitude
	}
	return normalizedMNAFloat(100 * math.Sqrt(harmonicPower) / fundamental), nil
}

func dftMagnitude(values []float64, bin int) float64 {
	realPart, imaginary := 0.0, 0.0
	for index, value := range values {
		angle := 2 * math.Pi * float64(bin*index) / float64(len(values))
		realPart += value * math.Cos(angle)
		imaginary -= value * math.Sin(angle)
	}
	return 2 * math.Hypot(realPart, imaginary) / float64(len(values))
}

func buildTransientTemplate(plan Plan, analysis Analysis) (mnaSystem, []Diagnostic) {
	zero := transientDCAnalysis(analysis, 0)
	for index := range zero.Excitations {
		zero.Excitations[index].DCValue = 0
	}
	system, diagnostics := buildMNASystem(plan, zero, 0)
	if len(diagnostics) != 0 {
		return system, diagnostics
	}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveCapacitorTransientV1:
			conductance := *device.ValueSI / analysis.TimeStepS
			stampAdmittance(&system, terminals["A"], terminals["B"], complex(conductance, 0))
		case PrimitiveInductorTransientV1:
			branch := system.branchIndex[device.Component]
			system.matrix[branch][branch] -= complex(*device.ValueSI/analysis.TimeStepS, 0)
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			for _, capacitor := range mosfetDynamicCapacitors(device, analysis.Kind) {
				stampAdmittance(&system, capacitor.a, capacitor.b, complex(capacitor.capacitanceF/analysis.TimeStepS, 0))
			}
		case PrimitiveBidirectionalTVSV1:
			conductance := deviceParameterMap(device)["junction_capacitance_f"] / analysis.TimeStepS
			stampAdmittance(&system, terminals["ANODE"], terminals["CATHODE"], complex(conductance, 0))
		case PrimitiveOpAmpV1:
			parameters := deviceParameterMap(device)
			gain := parameters["dc_open_loop_gain"]
			poleHz := parameters["gain_bandwidth_hz"] / gain
			historyCoefficient := 1 / (2 * math.Pi * poleHz * gain * analysis.TimeStepS)
			if outputIndex, exists := system.nodeIndex[terminals["OUT"]]; exists {
				system.matrix[system.branchIndex[device.Component]][outputIndex] += complex(historyCoefficient, 0)
			}
		case PrimitiveCurrentSenseAmplifierV1:
			parameters := deviceParameterMap(device)
			gain := parameters["gain_v_per_v"]
			poleHz := parameters["bandwidth_hz"] / gain
			historyCoefficient := 1 / (2 * math.Pi * poleHz * gain * analysis.TimeStepS)
			branch := system.branchIndex[device.Component]
			if outputIndex, exists := system.nodeIndex[terminals["OUT"]]; exists {
				system.matrix[branch][outputIndex] += complex(historyCoefficient, 0)
			}
			if groundIndex, exists := system.nodeIndex[terminals["GND_A"]]; exists {
				system.matrix[branch][groundIndex] -= complex(historyCoefficient, 0)
			}
			stampCurrentSource(&system, terminals["VCC"], terminals["GND_A"], complex(-parameters["quiescent_current_a"], 0))
		case PrimitiveAdjustableLinearRegulatorV1:
			parameters := deviceParameterMap(device)
			system.rhs[system.branchIndex[device.Component]] -= complex(parameters["reference_voltage_v"], 0)
			stampCurrentSource(&system, terminals["VIN"], terminals["GND"], complex(-parameters["quiescent_current_a"], 0))
		case PrimitiveFixedLinearRegulatorV1:
			parameters := deviceParameterMap(device)
			system.rhs[system.branchIndex[device.Component]] -= complex(parameters["output_voltage_v"], 0)
			stampCurrentSource(&system, terminals["VIN"], terminals["GND"], complex(-parameters["quiescent_current_a"], 0))
		case PrimitiveFixedBuckModuleV1:
			parameters := deviceParameterMap(device)
			system.rhs[system.branchIndex[device.Component]] -= complex(parameters["output_voltage_v"], 0)
		case PrimitiveSynchronousBuckRegulatorV1:
			parameters := deviceParameterMap(device)
			branch := system.branchIndex[device.Component]
			// The DC builder may legitimately disable an unpowered buck. A
			// transient template still needs the catalog controller equation
			// because later event/startup steps can energize it.
			resetSynchronousBuckControlEquation(
				&system,
				device.Component,
				terminals,
				synchronousBuckTransconductance(parameters, 0),
				parameters["reference_voltage_v"],
			)
			system.rhs[branch] -= complex(parameters["reference_voltage_v"], 0)
			historyCoefficient := 1 / (2 * math.Pi * parameters["control_pole_hz"] * parameters["control_transconductance_s"] * analysis.TimeStepS)
			system.matrix[branch][branch] += complex(historyCoefficient, 0)
			// The empirical conversion-efficiency branch owns controller
			// operating current; no independent electrical source is removed
			// from this reusable transient template.
		case PrimitiveFloatingAdjustableRegulatorV1:
			parameters := deviceParameterMap(device)
			reference := parameters["polarity"] * parameters["reference_voltage_v"]
			adjustmentCurrent := parameters["polarity"] * parameters["adjustment_pin_current_a"]
			system.rhs[system.branchIndex[device.Component]] -= complex(reference, 0)
			stampCurrentSource(&system, terminals["VIN"], terminals["ADJ"], complex(-adjustmentCurrent, 0))
		case PrimitiveProgrammableCurrentSourceV1:
			parameters := deviceParameterMap(device)
			system.rhs[system.branchIndex[device.Component]] -= complex(parameters["offset_voltage_v"], 0)
			stampCurrentSource(&system, terminals["IN"], terminals["SET"], complex(-parameters["reference_current_a"], 0))
		case PrimitiveShuntVoltageReferenceV1:
			system.rhs[system.branchIndex[device.Component]] -= complex(deviceParameterMap(device)["output_voltage_v"], 0)
		case PrimitiveSingleOutputIsolatedConverterV1, PrimitiveProtectedIsolatedConverterV1:
			system.rhs[system.branchIndex[device.Component]] -= complex(deviceParameterMap(device)["output_voltage_v"], 0)
		case PrimitiveDualOutputIsolatedConverterV1:
			parameters := deviceParameterMap(device)
			positiveBranch := system.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_PLUS"}]
			negativeBranch := system.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_MINUS"}]
			system.rhs[positiveBranch] -= complex(parameters["positive_output_voltage_v"], 0)
			system.rhs[negativeBranch] += complex(parameters["negative_output_voltage_v"], 0)
		}
	}
	for _, node := range plan.Nodes {
		if index, exists := system.nodeIndex[node]; exists {
			// Fixed global gmin gives every resolved node the same deterministic
			// numerical reference. It is deliberately trusted and tiny rather
			// than provider-configurable, and remains part of the work contract.
			system.matrix[index][index] += complex(transientGmin, 0)
		}
	}
	if diagnostic := validateMNASystemBounds(system); diagnostic != nil {
		return mnaSystem{}, []Diagnostic{*diagnostic}
	}
	return system, nil
}

func prepareTransientBase(base *mnaSystem, template mnaSystem, plan Plan, analysis Analysis, step int, timeS float64, previous []complex128, history [][]complex128, openFuses map[string]bool) (map[string]float64, map[string]bool, []Diagnostic) {
	resetMNASystem(base, &template)
	comparatorStates := map[string]float64{}
	fixedOutputClamps := map[string]bool{}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveResistorV1:
			value := transientDeviceValue(analysis.DeviceValueEvents, device.Component, timeS, analysis.TimeStepS, *device.ValueSI)
			if value != *device.ValueSI {
				delta := 1/value - 1/(*device.ValueSI)
				stampAdmittance(base, terminals["A"], terminals["B"], complex(delta, 0))
			}
		case PrimitiveFuseI2TClearingV1:
			if openFuses[device.Component] {
				parameters := deviceParameterMap(device)
				delta := 1/parameters["open_resistance_ohm"] - 1/parameters["cold_resistance_ohm"]
				stampAdmittance(base, terminals["A"], terminals["B"], complex(delta, 0))
			}
		case PrimitiveVoltageSourceV1, PrimitiveConnectorVoltageSourceV1:
			value := transientIndependentSourceValue(analysis, device.Component, timeS)
			base.rhs[base.branchIndex[device.Component]] += complex(value, 0)
		case PrimitiveCurrentSourceV1:
			value := transientIndependentSourceValue(analysis, device.Component, timeS)
			stampCurrentSource(base, terminals["POSITIVE"], terminals["NEGATIVE"], complex(value, 0))
		case PrimitiveFixedClockSourceV1, PrimitiveResistorProgrammedClockSourceV1:
			parameters := deviceParameterMap(device)
			conductance := complex(1/parameters["output_resistance_ohm"], 0)
			dcEnabled := clockSourceDCEnabled(device)
			dcReference := terminals["GND"]
			if parameters["dc_output_high"] >= .5 {
				dcReference = terminals["VDD"]
			}
			if dcEnabled {
				stampAdmittance(base, terminals["OUT"], dcReference, -conductance)
			}
			transientEnabled := clockSourceEnabled(device, base, previous)
			if transientEnabled != dcEnabled {
				supplyDelta := parameters["supply_current_a"]
				if !transientEnabled {
					supplyDelta = -supplyDelta
				}
				stampCurrentSource(base, terminals["VDD"], terminals["GND"], complex(supplyDelta, 0))
			}
			transientHigh := clockSourceHigh(device, base, analysis, timeS, previous)
			if transientEnabled {
				transientReference := terminals["GND"]
				if transientHigh {
					transientReference = terminals["VDD"]
				}
				stampAdmittance(base, terminals["OUT"], transientReference, conductance)
			}
		case PrimitiveCMOSBufferV1:
			if !clockBufferHigh(device, base, analysis, step, previous, history) {
				resistance := deviceParameterMap(device)["output_resistance_ohm"]
				conductance := complex(1/resistance, 0)
				stampAdmittance(base, terminals["OUT"], terminals["VCC"], -conductance)
				stampAdmittance(base, terminals["OUT"], terminals["GND"], conductance)
			}
		case PrimitiveCapacitorTransientV1:
			conductance := *device.ValueSI / analysis.TimeStepS
			previousVoltage := nonlinearNodeVoltage(base, previous, terminals["A"]) - nonlinearNodeVoltage(base, previous, terminals["B"])
			stampCurrentSource(base, terminals["A"], terminals["B"], complex(-conductance*previousVoltage, 0))
		case PrimitiveInductorTransientV1:
			branch := base.branchIndex[device.Component]
			previousCurrent := real(previous[branch])
			base.rhs[branch] -= complex(*device.ValueSI/analysis.TimeStepS*previousCurrent, 0)
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			for _, capacitor := range mosfetDynamicCapacitors(device, analysis.Kind) {
				conductance := capacitor.capacitanceF / analysis.TimeStepS
				previousVoltage := nonlinearNodeVoltage(base, previous, capacitor.a) - nonlinearNodeVoltage(base, previous, capacitor.b)
				stampCurrentSource(base, capacitor.a, capacitor.b, complex(-conductance*previousVoltage, 0))
			}
		case PrimitiveRelayNormallyOpenV1:
			parameters := deviceParameterMap(device)
			coilVoltage := nonlinearNodeVoltage(base, previous, terminals["COIL_A"]) - nonlinearNodeVoltage(base, previous, terminals["COIL_B"])
			energized := math.Abs(coilVoltage)/parameters["coil_resistance_ohm"] >= parameters["operate_current_a"]*(1-1e-12)
			closed := energized
			if analysis.Kind == AnalysisStartup {
				closed = energized && timeS >= parameters["operate_delay_s"]
			}
			if !closed {
				delta := 1/parameters["contact_off_resistance_ohm"] - 1/parameters["contact_on_resistance_ohm"]
				stampAdmittance(base, terminals["CONTACT_IN"], terminals["CONTACT_OUT"], complex(delta, 0))
			}
		case PrimitiveBidirectionalTVSV1:
			conductance := deviceParameterMap(device)["junction_capacitance_f"] / analysis.TimeStepS
			previousVoltage := nonlinearNodeVoltage(base, previous, terminals["ANODE"]) - nonlinearNodeVoltage(base, previous, terminals["CATHODE"])
			stampCurrentSource(base, terminals["ANODE"], terminals["CATHODE"], complex(-conductance*previousVoltage, 0))
		case PrimitiveComparatorOpenCollectorV1:
			parameters := deviceParameterMap(device)
			delaySteps := int(math.Ceil(parameters["propagation_delay_s"]/analysis.TimeStepS - 1e-12))
			decisionIndex := step - delaySteps
			if decisionIndex < 0 {
				decisionIndex = 0
			}
			if decisionIndex >= len(history) {
				decisionIndex = len(history) - 1
			}
			if comparatorOn(device, *base, history[decisionIndex]) {
				comparatorStates[device.Component] = 1
				onConductance := 1 / parameters["output_on_resistance_ohm"]
				offConductance := 1 / parameters["output_off_resistance_ohm"]
				stampAdmittance(base, terminals["OUT"], terminals["V_MINUS"], complex(onConductance-offConductance, 0))
			} else {
				comparatorStates[device.Component] = 0
			}
		case PrimitiveOpAmpV1:
			parameters := deviceParameterMap(device)
			gain := parameters["dc_open_loop_gain"]
			poleHz := parameters["gain_bandwidth_hz"] / gain
			historyCoefficient := 1 / (2 * math.Pi * poleHz * gain * analysis.TimeStepS)
			base.rhs[base.branchIndex[device.Component]] += complex(historyCoefficient*nonlinearNodeVoltage(base, previous, terminals["OUT"]), 0)
			positive, positiveKnown := transientKnownNodeVoltage(plan, analysis, terminals["V_PLUS"], timeS)
			negative, negativeKnown := transientKnownNodeVoltage(plan, analysis, terminals["V_MINUS"], timeS)
			underpowered := positiveKnown && negativeKnown && positive-negative < parameters["supply_min_v"]
			startupRamping := analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if underpowered || startupRamping {
				stampTransientRelativeOutputClamp(base, device.Component, terminals["OUT"], terminals["V_MINUS"], 0)
				fixedOutputClamps[device.Component] = true
				continue
			}
			if analysis.Kind != AnalysisStartup {
				continue
			}
			if !positiveKnown || !negativeKnown {
				continue
			}
			differential := nonlinearNodeVoltage(base, previous, terminals["IN_PLUS"]) - nonlinearNodeVoltage(base, previous, terminals["IN_MINUS"])
			desired := parameters["dc_open_loop_gain"] * differential
			minimum := negative + parameters["output_low_margin_v"]
			maximum := positive - parameters["output_high_margin_v"]
			previousOutput := nonlinearNodeVoltage(base, previous, terminals["OUT"])
			switch {
			case desired < minimum:
				stampTransientOpAmpClamp(base, device.Component, terminals["OUT"], boundedTransientClamp(previousOutput, minimum))
				fixedOutputClamps[device.Component] = true
			case desired > maximum:
				stampTransientOpAmpClamp(base, device.Component, terminals["OUT"], boundedTransientClamp(previousOutput, maximum))
				fixedOutputClamps[device.Component] = true
			}
		case PrimitiveCurrentSenseAmplifierV1:
			parameters := deviceParameterMap(device)
			gain := parameters["gain_v_per_v"]
			poleHz := parameters["bandwidth_hz"] / gain
			historyCoefficient := 1 / (2 * math.Pi * poleHz * gain * analysis.TimeStepS)
			previousOutput := nonlinearNodeVoltage(base, previous, terminals["OUT"]) - nonlinearNodeVoltage(base, previous, terminals["GND_A"])
			base.rhs[base.branchIndex[device.Component]] += complex(historyCoefficient*previousOutput, 0)
			if analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1 {
				stampTransientRelativeOutputClamp(base, device.Component, terminals["OUT"], terminals["GND_A"], 0)
				fixedOutputClamps[device.Component] = true
				continue
			}
			stampCurrentSource(base, terminals["VCC"], terminals["GND_A"], complex(parameters["quiescent_current_a"], 0))
		case PrimitiveAdjustableLinearRegulatorV1:
			parameters := deviceParameterMap(device)
			reference := parameters["reference_voltage_v"]
			powerTransition := analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if powerTransition {
				reference = 0
			} else if analysis.Kind == AnalysisStartup && parameters["soft_start_time_s"] > 0 {
				reference *= math.Min(1, timeS/parameters["soft_start_time_s"])
			}
			base.rhs[base.branchIndex[device.Component]] += complex(reference, 0)
			if !powerTransition {
				stampCurrentSource(base, terminals["VIN"], terminals["GND"], complex(parameters["quiescent_current_a"], 0))
			}
		case PrimitiveFixedLinearRegulatorV1:
			parameters := deviceParameterMap(device)
			output := parameters["output_voltage_v"]
			powerTransition := analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if powerTransition {
				output = 0
			} else if analysis.Kind == AnalysisStartup && parameters["soft_start_time_s"] > 0 {
				output *= math.Min(1, timeS/parameters["soft_start_time_s"])
			}
			base.rhs[base.branchIndex[device.Component]] += complex(output, 0)
			if !powerTransition {
				stampCurrentSource(base, terminals["VIN"], terminals["GND"], complex(parameters["quiescent_current_a"], 0))
			}
		case PrimitiveFixedBuckModuleV1:
			parameters := deviceParameterMap(device)
			input := nonlinearNodeVoltage(base, previous, terminals["VIN"]) -
				nonlinearNodeVoltage(base, previous, terminals["GND"])
			output := parameters["output_voltage_v"]
			powerTransition := input < parameters["input_min_v"] ||
				analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if powerTransition {
				disableTransientBranch(base, device.Component)
				continue
			} else if analysis.Kind == AnalysisStartup && parameters["soft_start_time_s"] > 0 {
				output *= math.Min(1, timeS/parameters["soft_start_time_s"])
			}
			base.rhs[base.branchIndex[device.Component]] += complex(output, 0)
		case PrimitiveSynchronousBuckRegulatorV1:
			parameters := deviceParameterMap(device)
			branch := base.branchIndex[device.Component]
			templateRatio := parameters["nominal_output_voltage_v"] /
				parameters["nominal_input_voltage_v"] /
				parameters["conversion_efficiency_fraction"]
			outputV := parameters["nominal_output_voltage_v"]
			if outputNet, ok := synchronousBuckOutputNet(plan, device); ok {
				outputV = math.Abs(
					nonlinearNodeVoltage(base, previous, outputNet) -
						nonlinearNodeVoltage(base, previous, terminals["PGND"]),
				)
			}
			actualRatio := synchronousBuckInputCurrentRatioForOutput(plan, analysis, device, timeS, outputV)
			adjustSynchronousBuckInputCurrentRatio(base, device, actualRatio-templateRatio)
			inputV := synchronousBuckInputVoltage(plan, analysis, device, timeS)
			enableV := nonlinearNodeVoltage(base, previous, terminals["EN"]) -
				nonlinearNodeVoltage(base, previous, terminals["AGND"])
			if enable, enableKnown := transientKnownNodeVoltage(plan, analysis, terminals["EN"], timeS); enableKnown {
				if ground, groundKnown := transientKnownNodeVoltage(plan, analysis, terminals["AGND"], timeS); groundKnown {
					enableV = enable - ground
				}
			}
			powerTransition := inputV < parameters["min_input_voltage_v"] || enableV < parameters["enable_threshold_v"]
			if powerTransition {
				disableTransientBranch(base, device.Component)
				continue
			}
			reference := parameters["reference_voltage_v"] * synchronousBuckSoftStartScale(plan, analysis, device, timeS)
			historyCoefficient := 1 / (2 * math.Pi * parameters["control_pole_hz"] * parameters["control_transconductance_s"] * analysis.TimeStepS)
			base.rhs[branch] += complex(reference+historyCoefficient*real(previous[branch]), 0)
		case PrimitiveFloatingAdjustableRegulatorV1:
			parameters := deviceParameterMap(device)
			reference := parameters["polarity"] * parameters["reference_voltage_v"]
			powerTransition := analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if powerTransition {
				reference = 0
			} else if analysis.Kind == AnalysisStartup && parameters["soft_start_time_s"] > 0 {
				reference *= math.Min(1, timeS/parameters["soft_start_time_s"])
			}
			base.rhs[base.branchIndex[device.Component]] += complex(reference, 0)
			if !powerTransition {
				adjustmentCurrent := parameters["polarity"] * parameters["adjustment_pin_current_a"]
				stampCurrentSource(base, terminals["VIN"], terminals["ADJ"], complex(adjustmentCurrent, 0))
			}
		case PrimitiveProgrammableCurrentSourceV1:
			parameters := deviceParameterMap(device)
			reference := parameters["reference_current_a"]
			offset := parameters["offset_voltage_v"]
			headroom := nonlinearNodeVoltage(base, previous, terminals["IN"]) - nonlinearNodeVoltage(base, previous, terminals["OUT"])
			powerTransition := headroom < parameters["min_headroom_v"] || analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if powerTransition {
				disableTransientBranch(base, device.Component)
			} else if analysis.Kind == AnalysisStartup && parameters["soft_start_time_s"] > 0 {
				scale := math.Min(1, timeS/parameters["soft_start_time_s"])
				reference *= scale
				offset *= scale
			}
			if !powerTransition {
				base.rhs[base.branchIndex[device.Component]] += complex(offset, 0)
				stampCurrentSource(base, terminals["IN"], terminals["SET"], complex(reference, 0))
			}
		case PrimitiveShuntVoltageReferenceV1:
			output := deviceParameterMap(device)["output_voltage_v"]
			if analysis.Kind == AnalysisStartup {
				output *= startupSourceRampScale(analysis, timeS)
			}
			base.rhs[base.branchIndex[device.Component]] += complex(output, 0)
		case PrimitiveDualOutputIsolatedConverterV1:
			parameters := deviceParameterMap(device)
			positive := parameters["positive_output_voltage_v"]
			negative := -parameters["negative_output_voltage_v"]
			powerTransition := analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if powerTransition {
				positive, negative = 0, 0
			} else if analysis.Kind == AnalysisStartup && parameters["soft_start_time_s"] > 0 {
				scale := math.Min(1, timeS/parameters["soft_start_time_s"])
				positive *= scale
				negative *= scale
			}
			base.rhs[base.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_PLUS"}]] += complex(positive, 0)
			base.rhs[base.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_MINUS"}]] += complex(negative, 0)
		case PrimitiveSingleOutputIsolatedConverterV1, PrimitiveProtectedIsolatedConverterV1:
			parameters := deviceParameterMap(device)
			input := nonlinearNodeVoltage(base, previous, terminals["VIN_PLUS"]) -
				nonlinearNodeVoltage(base, previous, terminals["VIN_MINUS"])
			output := parameters["output_voltage_v"]
			powerTransition := input < parameters["input_min_v"] ||
				analysis.Kind == AnalysisStartup && startupSourceRampScale(analysis, timeS) < 1
			if powerTransition {
				disableTransientBranch(base, device.Component)
				continue
			} else if analysis.Kind == AnalysisStartup && parameters["soft_start_time_s"] > 0 {
				output *= math.Min(1, timeS/parameters["soft_start_time_s"])
			}
			base.rhs[base.branchIndex[device.Component]] += complex(output, 0)
		}
	}
	referenceUnobservedMNAComponents(plan, analysis, base)
	if diagnostic := validateMNASystemBounds(*base); diagnostic != nil {
		return comparatorStates, fixedOutputClamps, []Diagnostic{*diagnostic}
	}
	return comparatorStates, fixedOutputClamps, nil
}

func boundedTransientClamp(previous, target float64) float64 {
	delta := target - previous
	if math.Abs(delta) <= nonlinearMaxNodeUpdateV {
		return target
	}
	return previous + math.Copysign(nonlinearMaxNodeUpdateV, delta)
}

func transientInitialGuess(analysis Analysis, timeS float64, previous []complex128, history [][]complex128) []complex128 {
	guess := append([]complex128(nil), previous...)
	if len(history) == 0 || !transientPeriodicZeroCrossing(analysis, timeS) {
		return guess
	}
	if len(history[0]) == len(previous) {
		copy(guess, history[0])
	}
	return guess
}

func transientPeriodicZeroCrossing(analysis Analysis, timeS float64) bool {
	found := false
	for _, excitation := range analysis.Excitations {
		if excitation.SineAmplitude == 0 || excitation.SineFrequencyHz <= 0 {
			continue
		}
		found = true
		phase := 2*math.Pi*excitation.SineFrequencyHz*timeS + excitation.SinePhaseDeg*math.Pi/180
		if math.Abs(math.Sin(phase)) > 1e-12 {
			return false
		}
	}
	return found
}

func stampTransientOpAmpClamp(system *mnaSystem, component, output string, value float64) {
	stampTransientRelativeOutputClamp(system, component, output, "", value)
}

func stampTransientRelativeOutputClamp(system *mnaSystem, component, output, reference string, value float64) {
	branch := system.branchIndex[component]
	for column := range system.matrix[branch] {
		system.matrix[branch][column] = 0
	}
	if outputIndex, exists := system.nodeIndex[output]; exists {
		system.matrix[branch][outputIndex] = 1
	}
	if referenceIndex, exists := system.nodeIndex[reference]; exists {
		system.matrix[branch][referenceIndex] = -1
	}
	system.rhs[branch] = complex(value, 0)
}

func disableTransientBranch(system *mnaSystem, component string) {
	branch := system.branchIndex[component]
	for column := range system.matrix[branch] {
		system.matrix[branch][column] = 0
	}
	system.matrix[branch][branch] = 1
	system.rhs[branch] = 0
}

func transientKnownNodeVoltage(plan Plan, analysis Analysis, node string, timeS float64) (float64, bool) {
	if node == plan.GroundNode {
		return 0, true
	}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		positive, negative := "", ""
		switch device.PrimitiveModel {
		case PrimitiveVoltageSourceV1:
			positive, negative = terminals["POSITIVE"], terminals["NEGATIVE"]
		case PrimitiveConnectorVoltageSourceV1:
			positive, negative = terminals["PIN_1"], terminals["PIN_2"]
		default:
			continue
		}
		value := transientExcitationValue(analysis, device.Component, timeS)
		if analysis.Kind == AnalysisStartup {
			value *= startupSourceRampScale(analysis, timeS)
		}
		switch {
		case positive == node && negative == plan.GroundNode:
			return value, true
		case negative == node && positive == plan.GroundNode:
			return -value, true
		}
	}
	return 0, false
}

func startupSourceRampScale(analysis Analysis, timeS float64) float64 {
	rampDuration := math.Max(analysis.TimeStepS, math.Min(10e-6, analysis.DurationS/10))
	return math.Max(0, math.Min(1, timeS/rampDuration))
}

func transientExcitationValue(analysis Analysis, component string, timeS float64) float64 {
	for _, excitation := range analysis.Excitations {
		if excitation.Component == component {
			return transientSourceEventValue(
				analysis.SourceValueEvents,
				component,
				timeS,
				analysis.TimeStepS,
				transientSourceValue(excitation, timeS, analysis.TimeStepS),
			)
		}
	}
	return 0
}

func transientIndependentSourceValue(analysis Analysis, component string, timeS float64) float64 {
	value := transientExcitationValue(analysis, component, timeS)
	if analysis.Kind == AnalysisStartup {
		value *= startupSourceRampScale(analysis, timeS)
	}
	return value
}

func clockSourceHigh(device ResolvedDevice, system *mnaSystem, analysis Analysis, timeS float64, state []complex128) bool {
	parameters := deviceParameterMap(device)
	frequency := parameters["frequency_hz"]
	dutyCycle := parameters["duty_cycle_fraction"]
	if !finite(frequency) || frequency <= 0 || !finite(dutyCycle) ||
		dutyCycle <= 0 || dutyCycle >= 1 || !finite(timeS) {
		return false
	}
	if !clockSourceEnabled(device, system, state) {
		return false
	}
	phaseTime := timeS
	if analysis.Kind == AnalysisStartup {
		startup := parameters["startup_time_s"]
		if device.PrimitiveModel == PrimitiveResistorProgrammedClockSourceV1 {
			startup = parameters["startup_fixed_s"] + parameters["startup_cycles"]/frequency
		}
		if !finite(startup) || startup < 0 || timeS < startup {
			return false
		}
		phaseTime -= startup
	}
	period := 1 / frequency
	if !finite(period) || period <= 0 || !finite(phaseTime) {
		return false
	}
	elapsedCycles := math.Max(phaseTime, 0) * frequency
	if !finite(elapsedCycles) || elapsedCycles > maxClockPhaseCycles {
		return false
	}
	phaseFraction := elapsedCycles - math.Floor(elapsedCycles)
	return finite(phaseFraction) && phaseFraction < dutyCycle
}

func clockSourceDCEnabled(device ResolvedDevice) bool {
	terminals := terminalMap(device)
	enable := terminals["ENABLE"]
	return enable == "" || enable == terminals["VDD"]
}

func clockSourceEnabled(device ResolvedDevice, system *mnaSystem, state []complex128) bool {
	terminals := terminalMap(device)
	enable := terminals["ENABLE"]
	if enable == "" {
		return true
	}
	if system == nil {
		return false
	}
	groundV := nonlinearNodeVoltage(system, state, terminals["GND"])
	supplyV := nonlinearNodeVoltage(system, state, terminals["VDD"]) - groundV
	enableV := nonlinearNodeVoltage(system, state, enable) - groundV
	threshold := deviceParameterMap(device)["enable_high_ratio"] * supplyV
	return finite(supplyV) && supplyV > 0 && finite(enableV) && enableV >= threshold
}

func validateClockPhaseResolution(plan Plan, analysis Analysis) []Diagnostic {
	var diagnostics []Diagnostic
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveFixedClockSourceV1 &&
			device.PrimitiveModel != PrimitiveResistorProgrammedClockSourceV1 {
			continue
		}
		frequency := deviceParameterMap(device)["frequency_hz"]
		cycles := analysis.DurationS * frequency
		if !finite(frequency) || frequency <= 0 || !finite(cycles) || cycles > maxClockPhaseCycles {
			diagnostics = append(diagnostics, Diagnostic{
				Path: "analyses." + analysis.ID + ".duration_s",
				Message: fmt.Sprintf(
					"clock source %s would require %.12g elapsed cycles; deterministic phase limit is %.12g",
					device.Component, cycles, maxClockPhaseCycles,
				),
				Suggestion: "shorten the analysis duration or use a lower clock frequency",
			})
		}
	}
	return diagnostics
}

func resolveProgrammedClockFrequencies(plan Plan) Plan {
	resolved := ClonePlan(plan)
	for index := range resolved.Devices {
		device := &resolved.Devices[index]
		if device.PrimitiveModel != PrimitiveResistorProgrammedClockSourceV1 {
			continue
		}
		frequency := programmedClockFrequency(plan, *device)
		if !finite(frequency) || frequency <= 0 {
			continue
		}
		found := false
		for parameterIndex := range device.ModelParameters {
			if device.ModelParameters[parameterIndex].Name == "frequency_hz" {
				device.ModelParameters[parameterIndex].Value = frequency
				found = true
				break
			}
		}
		if !found {
			device.ModelParameters = append(device.ModelParameters, NamedValue{Name: "frequency_hz", Value: frequency})
			slices.SortFunc(device.ModelParameters, func(left, right NamedValue) int {
				return strings.Compare(left.Name, right.Name)
			})
		}
		device.parameterIndex = namedValueMap(device.ModelParameters)
	}
	return resolved
}

func programmedClockFrequency(plan Plan, source ResolvedDevice) float64 {
	parameters := namedValueMap(source.ModelParameters)
	terminals := terminalMap(source)
	setNode := terminals["SET"]
	groundNode := terminals["GND"]
	scale := parameters["frequency_scale_hz_ohm"]
	divider := parameters["divider_ratio"]
	if setNode == "" || groundNode == "" || setNode == groundNode ||
		!finite(scale) || scale <= 0 || !finite(divider) || divider <= 0 {
		return 0
	}
	frequency := 0.0
	for _, candidate := range plan.Devices {
		if candidate.PrimitiveModel != PrimitiveResistorV1 || candidate.Usage != "timing_resistor" || candidate.ValueSI == nil ||
			!finite(*candidate.ValueSI) || *candidate.ValueSI <= 0 {
			continue
		}
		resistorTerminals := terminalMap(candidate)
		aNode := resistorTerminals["A"]
		bNode := resistorTerminals["B"]
		if !((aNode == setNode && bNode == groundNode) || (aNode == groundNode && bNode == setNode)) {
			continue
		}
		if frequency != 0 {
			return 0
		}
		denominator := divider * *candidate.ValueSI
		if !finite(denominator) || denominator <= 0 {
			return 0
		}
		frequency = scale / denominator
		if !finite(frequency) || frequency <= 0 {
			return 0
		}
	}
	return frequency
}

func clockBufferHigh(device ResolvedDevice, system *mnaSystem, analysis Analysis, step int, previous []complex128, history [][]complex128) bool {
	parameters := deviceParameterMap(device)
	terminals := terminalMap(device)
	delaySteps := int(math.Ceil(parameters["propagation_delay_s"]/analysis.TimeStepS - 1e-12))
	decision := previous
	if len(history) != 0 {
		index := step - delaySteps
		if index < 0 {
			index = 0
		}
		if index >= len(history) {
			index = len(history) - 1
		}
		decision = history[index]
	}
	input := nonlinearNodeVoltage(system, decision, terminals["IN"])
	if input >= parameters["input_high_min_v"] {
		return true
	}
	if input <= parameters["input_low_max_v"] {
		return false
	}
	return nonlinearNodeVoltage(system, previous, terminals["OUT"]) >
		.5*(nonlinearNodeVoltage(system, previous, terminals["VCC"])+nonlinearNodeVoltage(system, previous, terminals["GND"]))
}

type transientOutputLimitState struct {
	side          int
	value         float64
	lower         float64
	upper         float64
	lowerResidual float64
	upperResidual float64
	lowerSolution []complex128
	upperSolution []complex128
}

func solveTransientStep(base mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous, guess []complex128, workspace *mnaSystem, selectiveNodeDamping bool, fixedOutputClamps map[string]bool) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	retryGuess := append([]complex128(nil), guess...)
	system, solution, evidence, diagnostic := solveTransientStepInternal(base, resolvedDevices, devices, previous, guess, workspace, selectiveNodeDamping, fixedOutputClamps, true)
	if diagnostic == nil || selectiveNodeDamping {
		return system, solution, evidence, diagnostic
	}
	system, solution, retryEvidence, retryDiagnostic := solveTransientStepInternal(base, resolvedDevices, devices, previous, retryGuess, workspace, true, fixedOutputClamps, true)
	retryEvidence.Iterations += evidence.Iterations
	retryEvidence.TotalIterations = retryEvidence.Iterations
	if retryDiagnostic == nil {
		retryEvidence.Method = "backward_euler_bounded_selective_damping_fallback_v1"
		return system, solution, retryEvidence, nil
	}
	return mnaSystem{}, nil, retryEvidence, retryDiagnostic
}

func solveTransientStepByBJTStateSeed(
	base mnaSystem,
	resolvedDevices []ResolvedDevice,
	devices []compiledNonlinearDevice,
	previous []complex128,
	initial []complex128,
	workspace *mnaSystem,
	fixedOutputClamps map[string]bool,
) (mnaSystem, []complex128, SolverEvidence, bool) {
	var components []string
	for _, device := range devices {
		if device.primitive == PrimitiveBJTNPNV1 || device.primitive == PrimitiveBJTPNPV1 {
			components = append(components, device.component)
		}
	}
	slices.Sort(components)
	if len(components) == 0 || len(components) > bjtStateSeedDeviceLimit || len(initial) != len(base.rhs) {
		return mnaSystem{}, nil, SolverEvidence{}, false
	}
	total := SolverEvidence{Method: "backward_euler_bounded_bjt_state_seed_v1"}
	for mask := 0; mask < 1<<len(components); mask++ {
		states := make(map[string]float64, len(components))
		for index, component := range components {
			states[component] = float64((mask >> index) & 1)
		}
		forced := compiledDevicesWithForcedBJTStates(devices, states)
		forcedGuess := append([]complex128(nil), initial...)
		_, seed, seedEvidence, seedDiagnostic := solveTransientStepInternal(
			base, resolvedDevices, forced, previous, forcedGuess, workspace, false, fixedOutputClamps, true,
		)
		total.Iterations += seedEvidence.Iterations
		if seedDiagnostic != nil {
			continue
		}
		system, solution, evidence, diagnostic := solveTransientStep(
			base, resolvedDevices, devices, previous, seed, workspace, false, fixedOutputClamps,
		)
		total.Iterations += evidence.Iterations
		total.TotalIterations = total.Iterations
		total.FinalMaxUpdateV = evidence.FinalMaxUpdateV
		total.FinalMaxCurrentUpdateA = evidence.FinalMaxCurrentUpdateA
		total.FinalMaxResidual = evidence.FinalMaxResidual
		if diagnostic == nil {
			return system, solution, total, true
		}
	}
	total.TotalIterations = total.Iterations
	return mnaSystem{}, nil, total, false
}

func compiledDevicesWithForcedBJTStates(devices []compiledNonlinearDevice, states map[string]float64) []compiledNonlinearDevice {
	clone := make([]compiledNonlinearDevice, len(devices))
	for index, device := range devices {
		clone[index] = device
		clone[index].parameters = make(map[string]float64, len(device.parameters)+1)
		for name, value := range device.parameters {
			clone[index].parameters[name] = value
		}
		if device.primitive == PrimitiveBJTNPNV1 || device.primitive == PrimitiveBJTPNPV1 {
			clone[index].parameters[parameterForcedBJTState] = states[device.component]
		}
	}
	return clone
}

func solveTransientStepByBJTVoltageSeed(
	base mnaSystem,
	resolvedDevices []ResolvedDevice,
	devices []compiledNonlinearDevice,
	previous []complex128,
	initial []complex128,
	workspace *mnaSystem,
	fixedOutputClamps map[string]bool,
	preferredSeed int,
) (mnaSystem, []complex128, SolverEvidence, bool, int) {
	total := SolverEvidence{Method: "backward_euler_bounded_bjt_voltage_seed_v1"}
	seeds := transientBJTVoltageSeeds(resolvedDevices, &base, initial)
	order := make([]int, 0, len(seeds))
	if preferredSeed >= 0 && preferredSeed < len(seeds) {
		order = append(order, preferredSeed)
	}
	for index := range seeds {
		if index != preferredSeed {
			order = append(order, index)
		}
	}
	if preferredSeed >= 0 && preferredSeed < len(seeds) && len(seeds) > 1 {
		order = append(order, preferredSeed)
	}
	for orderIndex, seedIndex := range order {
		seed := seeds[seedIndex]
		maxIterations := transientMaxNewtonIterations
		if seedIndex == preferredSeed && orderIndex == 0 && len(seeds) > 1 {
			maxIterations = transientPreferredBJTSeedIterations
		}
		system, solution, evidence, diagnostic := solveTransientStepInternalWithLimit(
			base, resolvedDevices, devices, previous, seed, workspace, true, fixedOutputClamps, true, maxIterations,
		)
		total.Iterations += evidence.Iterations
		total.TotalIterations = total.Iterations
		total.FinalMaxUpdateV = evidence.FinalMaxUpdateV
		total.FinalMaxCurrentUpdateA = evidence.FinalMaxCurrentUpdateA
		total.FinalMaxResidual = evidence.FinalMaxResidual
		if diagnostic == nil {
			return system, solution, total, true, seedIndex
		}
	}
	return mnaSystem{}, nil, total, false, -1
}

func transientBJTVoltageSeeds(resolvedDevices []ResolvedDevice, system *mnaSystem, previous []complex128) [][]complex128 {
	var devices []ResolvedDevice
	for _, device := range resolvedDevices {
		if device.PrimitiveModel == PrimitiveBJTNPNV1 || device.PrimitiveModel == PrimitiveBJTPNPV1 {
			devices = append(devices, device)
		}
	}
	if len(devices) == 0 || len(devices) > bjtStateSeedDeviceLimit || system == nil || len(previous) != len(system.rhs) {
		return nil
	}
	slices.SortStableFunc(devices, func(left, right ResolvedDevice) int {
		return strings.Compare(left.Component, right.Component)
	})
	seeds := make([][]complex128, 0, 1<<len(devices))
	for state := 0; state < 1<<len(devices); state++ {
		seed := append([]complex128(nil), previous...)
		for index, device := range devices {
			terminals := terminalMap(device)
			emitter := nonlinearNodeVoltage(system, seed, terminals["EMITTER"])
			base, collector := emitter, nonlinearNodeVoltage(system, seed, terminals["COLLECTOR"])
			if state&(1<<index) != 0 {
				polarity := 1.0
				if device.PrimitiveModel == PrimitiveBJTPNPV1 {
					polarity = -1
				}
				base = emitter + polarity*.72
				collector = emitter + polarity*.1
			}
			setTransientNodeSeed(system, seed, terminals["BASE"], base)
			setTransientNodeSeed(system, seed, terminals["COLLECTOR"], collector)
		}
		seeds = append(seeds, seed)
	}
	return seeds
}

func setTransientNodeSeed(system *mnaSystem, solution []complex128, node string, value float64) {
	if system == nil {
		return
	}
	if index, exists := system.nodeIndex[node]; exists && index >= 0 && index < len(solution) {
		solution[index] = complex(value, 0)
	}
}

func solveTransientStepWithSourceContinuation(priorBase, finalBase mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous []complex128, workspace *mnaSystem, fixedOutputClamps map[string]bool) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	guess := append([]complex128(nil), previous...)
	total := SolverEvidence{Method: "backward_euler_bounded_system_continuation_v1"}
	usedBJTSeed := false
	var system mnaSystem
	for stage := 1; stage <= transientSourceContinuationStages; stage++ {
		fraction := float64(stage) / transientSourceContinuationStages
		scale := fraction
		stageBase := cloneMNASystem(finalBase)
		for row := range stageBase.matrix {
			for column := range stageBase.matrix[row] {
				stageBase.matrix[row][column] = priorBase.matrix[row][column] +
					(finalBase.matrix[row][column]-priorBase.matrix[row][column])*complex(scale, 0)
			}
		}
		for index := range stageBase.rhs {
			stageBase.rhs[index] = priorBase.rhs[index] + (finalBase.rhs[index]-priorBase.rhs[index])*complex(scale, 0)
		}
		var evidence SolverEvidence
		var diagnostic *Diagnostic
		stageGuess := append([]complex128(nil), guess...)
		system, guess, evidence, diagnostic = solveTransientStep(stageBase, resolvedDevices, devices, previous, stageGuess, workspace, false, fixedOutputClamps)
		total.Iterations += evidence.Iterations
		total.TotalIterations = total.Iterations
		total.FinalMaxUpdateV = evidence.FinalMaxUpdateV
		total.FinalMaxCurrentUpdateA = evidence.FinalMaxCurrentUpdateA
		total.FinalMaxResidual = evidence.FinalMaxResidual
		if diagnostic != nil {
			var seeded bool
			system, guess, evidence, seeded, _ = solveTransientStepByBJTVoltageSeed(
				stageBase, resolvedDevices, devices, previous, stageGuess, workspace, fixedOutputClamps, -1,
			)
			if !seeded {
				var stateEvidence SolverEvidence
				system, guess, stateEvidence, seeded = solveTransientStepByBJTStateSeed(
					stageBase, resolvedDevices, devices, previous, stageGuess, workspace, fixedOutputClamps,
				)
				evidence.Iterations += stateEvidence.Iterations
				evidence.TotalIterations = evidence.Iterations
				evidence.FinalMaxUpdateV = stateEvidence.FinalMaxUpdateV
				evidence.FinalMaxCurrentUpdateA = stateEvidence.FinalMaxCurrentUpdateA
				evidence.FinalMaxResidual = stateEvidence.FinalMaxResidual
				if seeded {
					evidence.Method = stateEvidence.Method
				}
			}
			total.Iterations += evidence.Iterations
			total.TotalIterations = total.Iterations
			total.FinalMaxUpdateV = evidence.FinalMaxUpdateV
			total.FinalMaxCurrentUpdateA = evidence.FinalMaxCurrentUpdateA
			total.FinalMaxResidual = evidence.FinalMaxResidual
			if seeded {
				usedBJTSeed = true
				diagnostic = nil
			}
		}
		if diagnostic != nil {
			diagnostic.Message = fmt.Sprintf("system-continuation stage %d/%d failed: %s", stage, transientSourceContinuationStages, diagnostic.Message)
			return mnaSystem{}, nil, total, diagnostic
		}
	}
	if usedBJTSeed {
		total.Method = "backward_euler_bounded_system_continuation_bjt_state_seed_v1"
	}
	return system, guess, total, nil
}

func solveTransientStepBySubstepPredictor(
	plan Plan,
	analysis Analysis,
	timeS float64,
	resolvedDevices []ResolvedDevice,
	devices []compiledNonlinearDevice,
	previous []complex128,
	openFuses map[string]bool,
	acceptedFuseStates map[string]transientFuseI2TState,
) (mnaSystem, []complex128, SolverEvidence, map[string]transientFuseI2TState, bool) {
	total := SolverEvidence{
		Method:               "backward_euler_bounded_accepted_substeps_v1",
		MaxIterationsPerStep: transientMaxNewtonIterations * transientMaxNewtonAttemptsPerObservation,
		MaxTotalIterations:   maxTransientWork,
	}
	for _, divisions := range []int{16, 32, 64} {
		predictorAnalysis := analysis
		predictorAnalysis.TimeStepS = analysis.TimeStepS / float64(divisions)
		template, diagnostics := buildTransientTemplate(plan, predictorAnalysis)
		if len(diagnostics) != 0 {
			continue
		}
		predictorPrevious := append([]complex128(nil), previous...)
		predictorHistory := [][]complex128{append([]complex128(nil), previous...)}
		predictorWorkspace := cloneMNASystem(template)
		var predictorSystem mnaSystem
		predicted := true
		predictorFuseStates := cloneTransientFuseI2TStates(acceptedFuseStates)
		predictorOpenFuses := cloneTransientOpenFuseStates(openFuses)
		for substep := 1; substep <= divisions; substep++ {
			substepTime := timeS - analysis.TimeStepS + float64(substep)*predictorAnalysis.TimeStepS
			base := cloneMNASystem(template)
			predictorStates, predictorClamps, prepareDiagnostics := prepareTransientBase(
				&base, template, plan, predictorAnalysis, substep, substepTime,
				predictorPrevious, predictorHistory, predictorOpenFuses,
			)
			if len(prepareDiagnostics) != 0 {
				predicted = false
				break
			}
			guess := append([]complex128(nil), predictorPrevious...)
			predictorSystem, predictorSolution, evidence, diagnostic := solveTransientStep(
				base, resolvedDevices, devices, predictorPrevious, guess,
				&predictorWorkspace, false, predictorClamps,
			)
			total.Iterations += evidence.Iterations
			if diagnostic != nil {
				var seeded bool
				predictorSystem, predictorSolution, evidence, seeded, _ = solveTransientStepByBJTVoltageSeed(
					base, resolvedDevices, devices, predictorPrevious, predictorPrevious,
					&predictorWorkspace, predictorClamps, -1,
				)
				total.Iterations += evidence.Iterations
				if !seeded {
					predicted = false
					break
				}
			}
			opened, limitDiagnostics := validateTransientOperatingLimits(
				plan, predictorSystem, predictorSolution, predictorStates, true,
				predictorAnalysis.TimeStepS, predictorFuseStates, predictorOpenFuses,
			)
			if len(limitDiagnostics) != 0 || len(opened) != 0 {
				predicted = false
				break
			}
			predictorPrevious = predictorSolution
			predictorHistory = append(predictorHistory, append([]complex128(nil), predictorSolution...))
		}
		if !predicted {
			continue
		}
		total.TotalIterations = total.Iterations
		total.TimeSteps = divisions
		total.AcceptedSubsteps = divisions
		return predictorSystem, predictorPrevious, total, predictorFuseStates, true
	}
	total.TotalIterations = total.Iterations
	return mnaSystem{}, nil, total, nil, false
}

func transientAcceptedSubstepsApplicable(plan Plan) bool {
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case PrimitiveOpAmpV1, PrimitiveCurrentSenseAmplifierV1, PrimitiveSynchronousBuckRegulatorV1:
			return true
		}
	}
	return false
}

func cloneTransientOpenFuseStates(source map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(source))
	for component, open := range source {
		clone[component] = open
	}
	return clone
}

func cloneTransientFuseI2TStates(source map[string]transientFuseI2TState) map[string]transientFuseI2TState {
	clone := make(map[string]transientFuseI2TState, len(source))
	for component, state := range source {
		clone[component] = state
	}
	return clone
}

// commitTransientFuseStep is called only after a solved and validated point is
// recorded. It advances both accepted fuse history and the topology used by
// the next point; rejected coarse steps and predictor attempts never call it.
func commitTransientFuseStep(candidate map[string]transientFuseI2TState, openFuses map[string]bool, openedFuses []string) map[string]transientFuseI2TState {
	for _, component := range openedFuses {
		openFuses[component] = true
	}
	return candidate
}

func solveTransientStepInternal(base mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous, guess []complex128, workspace *mnaSystem, selectiveNodeDamping bool, fixedOutputClamps map[string]bool, allowMOSFETActiveSet bool) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	return solveTransientStepInternalWithLimit(
		base, resolvedDevices, devices, previous, guess, workspace, selectiveNodeDamping, fixedOutputClamps, allowMOSFETActiveSet, transientMaxNewtonIterations,
	)
}

func solveTransientStepInternalWithLimit(base mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous, guess []complex128, workspace *mnaSystem, selectiveNodeDamping bool, fixedOutputClamps map[string]bool, allowMOSFETActiveSet bool, maxIterations int) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	system := workspace
	constrainedScratch := acquireReusableMNASystemClone(&base)
	defer releaseReusableMNASystemClone(constrainedScratch)
	constrainedBase := &constrainedScratch.system
	outputLimits := transientPriorOutputLimits(base, resolvedDevices, previous, fixedOutputClamps)
	branchLimits := map[int]float64{}
	deferredOutputLimits := map[string]bool{}
	deferredBranchLimits := map[int]bool{}
	stickyBranchLimits := map[int]bool{}
	seenLimitStates := map[string]bool{transientActiveLimitSolverStateKey(resolvedDevices, outputLimits, branchLimits, deferredOutputLimits, deferredBranchLimits): true}
	evidence := SolverEvidence{Method: "backward_euler_bounded_newton_v1"}
	largestUpdateLabel, largestCurrentUpdateLabel, largestResidualLabel := "unknown", "unknown", "unknown"
	for iteration := 1; iteration <= maxIterations; iteration++ {
		resetMNASystem(constrainedBase, &base)
		applyTransientActiveLimits(constrainedBase, resolvedDevices, outputLimits, branchLimits)
		resetMNASystem(system, constrainedBase)
		stampCompiledNonlinearDevices(system, devices, guess)
		if diagnostic := validateMNASystemBounds(*system); diagnostic != nil {
			return mnaSystem{}, nil, evidence, diagnostic
		}
		candidate, diagnostic := solveMNA(*system)
		if diagnostic != nil {
			diagnostic.Suggestion = "correct floating nodes or conflicting source constraints, reduce the fixed observation step, or select compatible reviewed dynamic models"
			return mnaSystem{}, nil, evidence, diagnostic
		}
		nextOutputLimits, nextBranchLimits, activeLimitAdded := addViolatedTransientActiveLimit(base, resolvedDevices, candidate, outputLimits, branchLimits, fixedOutputClamps, deferredOutputLimits, deferredBranchLimits)
		if activeLimitAdded {
			evidence.Iterations++
			stateKey := transientActiveLimitSolverStateKey(resolvedDevices, nextOutputLimits, nextBranchLimits, deferredOutputLimits, deferredBranchLimits)
			if seenLimitStates[stateKey] {
				return mnaSystem{}, nil, evidence, &Diagnostic{Path: "devices", Message: "bounded transient output/current-limit states repeated before the unconstrained equations converged (repeated " + stateKey + ")", Suggestion: "correct ambiguous feedback, reduce the bounded observation step, or select compatible reviewed dynamic models"}
			}
			seenLimitStates[stateKey] = true
			seedTransientOutputLimitGuess(base, resolvedDevices, guess, outputLimits, nextOutputLimits)
			outputLimits, branchLimits = nextOutputLimits, nextBranchLimits
			continue
		}
		maxNodeUpdate, maxCurrentUpdate := 0.0, 0.0
		for index, value := range candidate {
			imaginaryTolerance := 1e-12 * math.Max(1, math.Abs(real(value)))
			if math.Abs(imag(value)) > imaginaryTolerance {
				return mnaSystem{}, nil, evidence, &Diagnostic{Path: "unknowns." + system.unknownLabels[index], Message: "transient analysis produced a non-real solution value", Suggestion: "correct ill-conditioned connectivity or select compatible reviewed dynamic models"}
			}
			if strings.HasPrefix(system.unknownLabels[index], "node:") {
				update := math.Abs(real(value - guess[index]))
				if update > maxNodeUpdate {
					maxNodeUpdate, largestUpdateLabel = update, system.unknownLabels[index]
				}
			} else {
				update := math.Abs(real(value - guess[index]))
				if update > maxCurrentUpdate {
					maxCurrentUpdate, largestCurrentUpdateLabel = update, system.unknownLabels[index]
				}
			}
		}
		damping := 1.0
		stableRegion := piecewiseLinearRegionStable(devices, system, guess, candidate)
		maxControlVoltageUpdate := maxNonlinearControlVoltageUpdate(devices, system, guess, candidate)
		if !selectiveNodeDamping && maxControlVoltageUpdate > nonlinearMaxNodeUpdateV && !stableRegion {
			damping = nonlinearMaxNodeUpdateV / maxControlVoltageUpdate
		}
		if !selectiveNodeDamping && requiresNonlinearLineSearch(devices) {
			priorResidual, _ := nonlinearResidual(*constrainedBase, devices, guess)
			trial := make([]complex128, len(guess))
			for {
				for index := range guess {
					trial[index] = guess[index] + (candidate[index]-guess[index])*complex(damping, 0)
				}
				trialResidual, _ := nonlinearResidual(*constrainedBase, devices, trial)
				if trialResidual <= priorResidual*(1+1e-12) || damping <= nonlinearMinLineSearchDamping {
					break
				}
				damping *= .5
			}
		}
		maxAppliedUpdate, maxAppliedCurrentUpdate := 0.0, 0.0
		for index := range guess {
			applied := (candidate[index] - guess[index]) * complex(damping, 0)
			if selectiveNodeDamping && strings.HasPrefix(system.unknownLabels[index], "node:") && math.Abs(real(applied)) > nonlinearMaxNodeUpdateV && !stableRegion {
				applied = complex(math.Copysign(nonlinearMaxNodeUpdateV, real(applied)), 0)
			}
			guess[index] += applied
			if strings.HasPrefix(system.unknownLabels[index], "node:") {
				maxAppliedUpdate = math.Max(maxAppliedUpdate, math.Abs(real(applied)))
			} else {
				maxAppliedCurrentUpdate = math.Max(maxAppliedCurrentUpdate, math.Abs(real(applied)))
			}
		}
		maxResidual, label := nonlinearResidual(*constrainedBase, devices, guess)
		largestResidualLabel = label
		evidence.Iterations++
		evidence.FinalMaxUpdateV = normalizedMNAFloat(maxAppliedUpdate)
		evidence.FinalMaxCurrentUpdateA = normalizedMNAFloat(maxAppliedCurrentUpdate)
		evidence.FinalMaxResidual = normalizedMNAFloat(maxResidual)
		if nonlinearIterationConverged(maxAppliedUpdate, maxAppliedCurrentUpdate, maxResidual) {
			nextOutputLimits, nextBranchLimits, activeLimitChanged := advanceTransientActiveLimitState(
				base, resolvedDevices, guess, outputLimits, branchLimits, fixedOutputClamps, stickyBranchLimits,
			)
			if activeLimitChanged {
				recordReleasedTransientActiveLimits(
					base,
					outputLimits, nextOutputLimits,
					branchLimits, nextBranchLimits,
					deferredOutputLimits, deferredBranchLimits,
				)
				for component := range nextOutputLimits {
					if _, wasLimited := outputLimits[component]; !wasLimited {
						delete(deferredOutputLimits, component)
					}
				}
				for branch := range nextBranchLimits {
					if _, wasLimited := branchLimits[branch]; !wasLimited {
						delete(deferredBranchLimits, branch)
					}
				}
				currentStateKey := transientActiveLimitStateKey(resolvedDevices, outputLimits, branchLimits)
				stateKey := transientActiveLimitSolverStateKey(resolvedDevices, nextOutputLimits, nextBranchLimits, deferredOutputLimits, deferredBranchLimits)
				if seenLimitStates[stateKey] {
					if len(branchLimits) == 0 && transientInteriorOutputRootsConsistent(base, resolvedDevices, guess, outputLimits) {
						evidence.Method = "backward_euler_bounded_interior_root_v1"
						evidence.TotalIterations = evidence.Iterations
						return *system, guess, evidence, nil
					}
					stabilizedBranch := false
					for branch := range nextBranchLimits {
						if _, wasLimited := branchLimits[branch]; wasLimited {
							continue
						}
						stickyBranchLimits[branch] = true
						stabilizedBranch = true
					}
					if stabilizedBranch {
						seedTransientOutputLimitGuess(base, resolvedDevices, guess, outputLimits, nextOutputLimits)
						outputLimits, branchLimits = nextOutputLimits, nextBranchLimits
						continue
					}
					return mnaSystem{}, nil, evidence, &Diagnostic{Path: "devices", Message: "bounded transient output/current-limit states did not converge (current " + currentStateKey + ", repeated " + stateKey + ")", Suggestion: "correct ambiguous feedback, reduce the bounded observation step, or select compatible reviewed dynamic models"}
				}
				seenLimitStates[stateKey] = true
				seedTransientOutputLimitGuess(base, resolvedDevices, guess, outputLimits, nextOutputLimits)
				outputLimits, branchLimits = nextOutputLimits, nextBranchLimits
				continue
			}
			return *system, guess, evidence, nil
		}
	}
	diagnostic := &Diagnostic{Path: "convergence", Message: fmt.Sprintf("fixed backward-Euler Newton solve did not converge within %d iterations; active limits %s; largest voltage update %s %.12g V, largest current update %s %.12g A, largest normalized residual %s %.12g", maxIterations, transientActiveLimitStateKey(resolvedDevices, outputLimits, branchLimits), largestUpdateLabel, evidence.FinalMaxUpdateV, largestCurrentUpdateLabel, evidence.FinalMaxCurrentUpdateA, largestResidualLabel, evidence.FinalMaxResidual), Suggestion: "reduce the bounded observation step, add a catalog-backed bias path, or correct incompatible source and switching conditions"}
	if allowMOSFETActiveSet {
		if activeSystem, activeSolution, activeEvidence, ok := solveTransientStepByMOSFETActiveSet(base, resolvedDevices, devices, previous, workspace, selectiveNodeDamping, fixedOutputClamps); ok {
			activeEvidence.Iterations += evidence.Iterations
			activeEvidence.TotalIterations = activeEvidence.Iterations
			return activeSystem, activeSolution, activeEvidence, nil
		}
	}
	return mnaSystem{}, nil, evidence, diagnostic
}

// transientPriorOutputLimits carries a saturated dynamic output into the next
// backward-Euler solve. A rail-limited op-amp in a Schmitt or other bistable
// network has multiple algebraically valid roots; restarting every observation
// from an unconstrained active set can alternate roots even though the stored
// energy has not crossed the physical switching threshold. The ordinary
// residual-driven active-limit advancement remains authoritative and releases
// the carried state as soon as the dynamic equation supports an interior or
// opposite-rail solution.
func transientPriorOutputLimits(
	base mnaSystem,
	devices []ResolvedDevice,
	previous []complex128,
	fixedOutputClamps map[string]bool,
) map[string]transientOutputLimitState {
	result := map[string]transientOutputLimitState{}
	for _, device := range devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 || fixedOutputClamps[device.Component] {
			continue
		}
		minimum, maximum, output, clampTolerance, ok := transientOutputLimitObservation(base, device, previous)
		if !ok || minimum >= maximum {
			continue
		}
		tolerance := 10 * clampTolerance * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
		switch {
		case output <= minimum+tolerance:
			result[device.Component] = transientOutputLimitState{side: -1, value: minimum}
		case output >= maximum-tolerance:
			result[device.Component] = transientOutputLimitState{side: 1, value: maximum}
		}
	}
	return result
}

func transientInteriorOutputRootsConsistent(base mnaSystem, devices []ResolvedDevice, solution []complex128, outputLimits map[string]transientOutputLimitState) bool {
	if len(outputLimits) == 0 {
		return false
	}
	for _, device := range devices {
		state, limited := outputLimits[device.Component]
		if !limited {
			continue
		}
		if state.side != 0 {
			return false
		}
		minimum, maximum, _, _, ok := transientOutputLimitObservation(base, device, solution)
		branch, hasBranch := base.branchIndex[device.Component]
		if !ok || !hasBranch {
			return false
		}
		residual := transientBranchEquationResidual(base, branch, solution)
		residualTolerance := nonlinearClampConsistencyV * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
		if math.Abs(residual) > residualTolerance {
			return false
		}
	}
	return true
}

func recordReleasedTransientActiveLimits(
	base mnaSystem,
	outputLimits, nextOutputLimits map[string]transientOutputLimitState,
	branchLimits, nextBranchLimits map[int]float64,
	deferredOutputLimits map[string]bool,
	deferredBranchLimits map[int]bool,
) {
	for component, state := range outputLimits {
		if _, remainsLimited := nextOutputLimits[component]; remainsLimited {
			continue
		}
		// A physical rail clamp is deferred after release so it cannot be
		// selected again before the restored control equation moves inward.
		// An interior continuation root is only a synthetic equation-solving
		// clamp: releasing it must leave the physical output envelope eligible.
		if state.side != 0 {
			deferredOutputLimits[component] = true
		} else {
			delete(deferredOutputLimits, component)
		}
		if branch, exists := base.branchIndex[component]; exists {
			// A branch limit deferred only to let the same controlled output
			// reach a physical rail becomes eligible again as soon as that
			// output clamp releases. Carrying the deferral forward would
			// permit a later above-limit solution.
			delete(deferredBranchLimits, branch)
		}
	}
	for branch := range branchLimits {
		if _, remainsLimited := nextBranchLimits[branch]; !remainsLimited {
			deferredBranchLimits[branch] = true
		}
	}
}

func solveTransientStepByMOSFETActiveSet(base mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous []complex128, workspace *mnaSystem, selectiveNodeDamping bool, fixedOutputClamps map[string]bool) (mnaSystem, []complex128, SolverEvidence, bool) {
	var switches []string
	for _, device := range devices {
		if device.primitive == PrimitiveNMOSSwitchV1 || device.primitive == PrimitivePMOSSwitchV1 {
			if device.parameters["gate_threshold_max_v"] > 0 {
				continue
			}
			switches = append(switches, device.component)
		}
	}
	slices.Sort(switches)
	if len(switches) == 0 || len(switches) > 4 {
		return mnaSystem{}, nil, SolverEvidence{}, false
	}
	var total SolverEvidence
	for mask := 0; mask < 1<<len(switches); mask++ {
		states := make(map[string]float64, len(switches))
		for index, component := range switches {
			states[component] = float64((mask >> index) & 1)
		}
		fixed := compiledDevicesWithForcedMOSFETStates(devices, states)
		trialGuess := make([]complex128, len(previous))
		copy(trialGuess, previous)
		system, solution, evidence, diagnostic := solveTransientStepInternal(base, resolvedDevices, fixed, previous, trialGuess, workspace, selectiveNodeDamping, fixedOutputClamps, false)
		total.Iterations += evidence.Iterations
		if diagnostic != nil || !compiledMOSFETActiveSetConsistent(fixed, &system, solution, states) {
			continue
		}
		evidence.Method = "backward_euler_bounded_mosfet_active_set_v1"
		evidence.Iterations = total.Iterations
		evidence.TotalIterations = total.Iterations
		return system, solution, evidence, true
	}
	return mnaSystem{}, nil, total, false
}

func compiledDevicesWithForcedMOSFETStates(devices []compiledNonlinearDevice, states map[string]float64) []compiledNonlinearDevice {
	clone := make([]compiledNonlinearDevice, len(devices))
	for index, device := range devices {
		clone[index] = device
		clone[index].parameters = make(map[string]float64, len(device.parameters)+1)
		for name, value := range device.parameters {
			clone[index].parameters[name] = value
		}
		if device.primitive == PrimitiveNMOSSwitchV1 || device.primitive == PrimitivePMOSSwitchV1 {
			clone[index].parameters[parameterForcedMOSFETState] = states[device.component]
		}
	}
	return clone
}

func compiledMOSFETActiveSetConsistent(devices []compiledNonlinearDevice, system *mnaSystem, solution []complex128, states map[string]float64) bool {
	for _, device := range devices {
		if device.primitive != PrimitiveNMOSSwitchV1 && device.primitive != PrimitivePMOSSwitchV1 {
			continue
		}
		gate := nonlinearNodeVoltage(system, solution, device.terminals["GATE"])
		source := nonlinearNodeVoltage(system, solution, device.terminals["SOURCE"])
		resolvedOn := device.polarity*(gate-source) >= device.parameters["gate_on_voltage_v"]
		if resolvedOn != (states[device.component] >= .5) {
			return false
		}
	}
	return true
}

func applyTransientActiveLimits(system *mnaSystem, devices []ResolvedDevice, outputLimits map[string]transientOutputLimitState, branchLimits map[int]float64) {
	for _, device := range devices {
		state, limited := outputLimits[device.Component]
		if !limited {
			continue
		}
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveOpAmpV1:
			stampTransientRelativeOutputClamp(system, device.Component, terminals["OUT"], "", state.value)
		case PrimitiveCurrentSenseAmplifierV1:
			stampTransientRelativeOutputClamp(system, device.Component, terminals["OUT"], terminals["GND_A"], state.value)
		case PrimitiveSynchronousBuckRegulatorV1:
			stampTransientRelativeOutputClamp(system, device.Component, terminals["SW"], terminals["PGND"], state.value)
		}
	}
	branches := make([]int, 0, len(branchLimits))
	for branch := range branchLimits {
		branches = append(branches, branch)
	}
	slices.Sort(branches)
	for _, branch := range branches {
		for column := range system.matrix[branch] {
			system.matrix[branch][column] = 0
		}
		system.matrix[branch][branch] = 1
		system.rhs[branch] = complex(branchLimits[branch], 0)
	}
}

func addViolatedTransientActiveLimit(base mnaSystem, devices []ResolvedDevice, solution []complex128, outputLimits map[string]transientOutputLimitState, branchLimits map[int]float64, fixedOutputClamps, deferredOutputLimits map[string]bool, deferredBranchLimits map[int]bool) (map[string]transientOutputLimitState, map[int]float64, bool) {
	for _, device := range devices {
		if fixedOutputClamps[device.Component] || deferredOutputLimits[device.Component] {
			continue
		}
		branch := -1
		branchLimited := false
		if candidateBranch, exists := base.branchIndex[device.Component]; exists {
			branch = candidateBranch
			_, branchLimited = branchLimits[branch]
		}
		if _, limited := outputLimits[device.Component]; limited {
			continue
		}
		minimum, maximum, output, clampTolerance, outputDevice := transientOutputLimitObservation(base, device, solution)
		if !outputDevice || minimum >= maximum {
			continue
		}
		tolerance := clampTolerance * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
		switch {
		case output < minimum-tolerance:
			next := cloneTransientOutputLimits(outputLimits)
			next[device.Component] = transientOutputLimitState{side: -1, value: minimum}
			nextBranches := branchLimits
			if branchLimited {
				nextBranches = cloneTransientBranchLimits(branchLimits)
				delete(nextBranches, branch)
				if deferredBranchLimits != nil {
					deferredBranchLimits[branch] = true
				}
			}
			return next, nextBranches, true
		case output > maximum+tolerance:
			next := cloneTransientOutputLimits(outputLimits)
			next[device.Component] = transientOutputLimitState{side: 1, value: maximum}
			nextBranches := branchLimits
			if branchLimited {
				nextBranches = cloneTransientBranchLimits(branchLimits)
				delete(nextBranches, branch)
				if deferredBranchLimits != nil {
					deferredBranchLimits[branch] = true
				}
			}
			return next, nextBranches, true
		}
	}
	for _, device := range devices {
		mainBranch, hasMainBranch := base.branchIndex[device.Component]
		_, outputLimited := outputLimits[device.Component]
		for _, candidate := range transientBranchLimitCandidates(base, devices, device) {
			if candidate.limit <= 0 || candidate.branch >= len(solution) || deferredBranchLimits[candidate.branch] {
				continue
			}
			if _, limited := branchLimits[candidate.branch]; limited {
				continue
			}
			current := real(solution[candidate.branch])
			if math.Abs(current) > candidate.limit*(1+1e-9) {
				if outputLimited && hasMainBranch && candidate.branch == mainBranch {
					state := outputLimits[device.Component]
					if device.PrimitiveModel == PrimitiveSynchronousBuckRegulatorV1 && state.value <= mnaPivotTolerance {
						if deferredBranchLimits != nil {
							deferredBranchLimits[candidate.branch] = true
						}
						continue
					}
				}
				nextOutputLimits := outputLimits
				if outputLimited && hasMainBranch && candidate.branch == mainBranch {
					nextOutputLimits = cloneTransientOutputLimits(outputLimits)
					delete(nextOutputLimits, device.Component)
				}
				next := cloneTransientBranchLimits(branchLimits)
				next[candidate.branch] = math.Copysign(candidate.limit, current)
				return nextOutputLimits, next, true
			}
		}
	}
	return outputLimits, branchLimits, false
}

func advanceTransientActiveLimitState(base mnaSystem, devices []ResolvedDevice, solution []complex128, outputLimits map[string]transientOutputLimitState, branchLimits map[int]float64, fixedOutputClamps map[string]bool, stickyBranchLimits map[int]bool) (map[string]transientOutputLimitState, map[int]float64, bool) {
	for _, device := range devices {
		if fixedOutputClamps[device.Component] {
			continue
		}
		if branch, exists := base.branchIndex[device.Component]; exists {
			if _, limited := branchLimits[branch]; limited {
				continue
			}
		}
		minimum, maximum, output, clampTolerance, outputDevice := transientOutputLimitObservation(base, device, solution)
		if !outputDevice || minimum >= maximum {
			continue
		}
		tolerance := clampTolerance * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
		state, limited := outputLimits[device.Component]
		if limited {
			residual := transientBranchEquationResidual(base, base.branchIndex[device.Component], solution)
			residualTolerance := nonlinearClampConsistencyV * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
			switch state.side {
			case -1, 1:
				target := maximum
				if state.side < 0 {
					target = minimum
				}
				if math.Abs(state.value-target) > tolerance {
					next := cloneTransientOutputLimits(outputLimits)
					next[device.Component] = transientOutputLimitState{side: state.side, value: target}
					return next, branchLimits, true
				}
				// At the high rail the unconstrained equation points outward with a
				// negative residual; at the low rail it points outward with a
				// positive residual. When it points inward, probe the opposite rail
				// to deterministically bracket the valid nonlinear operating point.
				if residual*float64(state.side) > residualTolerance {
					next := cloneTransientOutputLimits(outputLimits)
					if state.side > 0 {
						next[device.Component] = transientOutputLimitState{side: 2, value: math.Max(minimum, maximum-transientActiveLimitContinuationStepV), lower: minimum, upper: maximum, upperResidual: residual, upperSolution: cloneComplexSolution(solution)}
					} else {
						next[device.Component] = transientOutputLimitState{side: -2, value: math.Min(maximum, minimum+transientActiveLimitContinuationStepV), lower: minimum, upper: maximum, lowerResidual: residual, lowerSolution: cloneComplexSolution(solution)}
					}
					return next, branchLimits, true
				}
			case 2:
				if residual > residualTolerance {
					if state.value <= minimum+tolerance {
						next := cloneTransientOutputLimits(outputLimits)
						next[device.Component] = transientOutputLimitState{side: -1, value: minimum}
						return next, branchLimits, true
					}
					step := math.Max(transientActiveLimitContinuationStepV, state.upper-state.value)
					next := cloneTransientOutputLimits(outputLimits)
					next[device.Component] = transientOutputLimitState{side: 2, value: math.Max(minimum, state.value-2*step), lower: minimum, upper: state.value, upperResidual: residual, upperSolution: cloneComplexSolution(solution)}
					return next, branchLimits, true
				}
				if math.Abs(residual) <= residualTolerance {
					next := cloneTransientOutputLimits(outputLimits)
					next[device.Component] = transientOutputLimitState{value: state.value, lower: state.value, upper: state.value, lowerResidual: residual, upperResidual: residual, lowerSolution: cloneComplexSolution(solution), upperSolution: cloneComplexSolution(solution)}
					return next, branchLimits, true
				}
				next := cloneTransientOutputLimits(outputLimits)
				lower, upper := state.value, state.upper
				next[device.Component] = transientOutputLimitState{value: (lower + upper) / 2, lower: lower, upper: upper, lowerResidual: residual, upperResidual: state.upperResidual, lowerSolution: cloneComplexSolution(solution), upperSolution: cloneComplexSolution(state.upperSolution)}
				return next, branchLimits, true
			case -2:
				if residual < -residualTolerance {
					if state.value >= maximum-tolerance {
						next := cloneTransientOutputLimits(outputLimits)
						next[device.Component] = transientOutputLimitState{side: 1, value: maximum}
						return next, branchLimits, true
					}
					step := math.Max(transientActiveLimitContinuationStepV, state.value-state.lower)
					next := cloneTransientOutputLimits(outputLimits)
					next[device.Component] = transientOutputLimitState{side: -2, value: math.Min(maximum, state.value+2*step), lower: state.value, upper: maximum, lowerResidual: residual, lowerSolution: cloneComplexSolution(solution)}
					return next, branchLimits, true
				}
				if math.Abs(residual) <= residualTolerance {
					next := cloneTransientOutputLimits(outputLimits)
					next[device.Component] = transientOutputLimitState{value: state.value, lower: state.value, upper: state.value, lowerResidual: residual, upperResidual: residual, lowerSolution: cloneComplexSolution(solution), upperSolution: cloneComplexSolution(solution)}
					return next, branchLimits, true
				}
				next := cloneTransientOutputLimits(outputLimits)
				lower, upper := state.lower, state.value
				next[device.Component] = transientOutputLimitState{value: (lower + upper) / 2, lower: lower, upper: upper, lowerResidual: state.lowerResidual, upperResidual: residual, lowerSolution: cloneComplexSolution(state.lowerSolution), upperSolution: cloneComplexSolution(solution)}
				return next, branchLimits, true
			case 0:
				if math.Abs(residual) <= residualTolerance {
					// The continuation clamp has located an interior root of
					// the original control equation. Release the synthetic
					// voltage clamp so the restored equation can expose and
					// enforce any independent branch-current limit.
					next := cloneTransientOutputLimits(outputLimits)
					delete(next, device.Component)
					return next, branchLimits, true
				}
				lower, upper := state.lower, state.upper
				lowerResidual, upperResidual := state.lowerResidual, state.upperResidual
				lowerSolution, upperSolution := state.lowerSolution, state.upperSolution
				if residual < 0 {
					lower, lowerResidual = state.value, residual
					lowerSolution = cloneComplexSolution(solution)
				} else {
					upper, upperResidual = state.value, residual
					upperSolution = cloneComplexSolution(solution)
				}
				if upper-lower <= nonlinearClampConsistencyV {
					continue
				}
				next := cloneTransientOutputLimits(outputLimits)
				next[device.Component] = transientOutputLimitState{value: (lower + upper) / 2, lower: lower, upper: upper, lowerResidual: lowerResidual, upperResidual: upperResidual, lowerSolution: cloneComplexSolution(lowerSolution), upperSolution: cloneComplexSolution(upperSolution)}
				return next, branchLimits, true
			}
			continue
		}
		switch {
		case output < minimum-tolerance:
			next := cloneTransientOutputLimits(outputLimits)
			next[device.Component] = transientOutputLimitState{side: -1, value: minimum}
			return next, branchLimits, true
		case output > maximum+tolerance:
			next := cloneTransientOutputLimits(outputLimits)
			next[device.Component] = transientOutputLimitState{side: 1, value: maximum}
			return next, branchLimits, true
		}
	}

	for _, device := range devices {
		mainBranch, hasMainBranch := base.branchIndex[device.Component]
		_, outputLimited := outputLimits[device.Component]
		for _, candidate := range transientBranchLimitCandidates(base, devices, device) {
			if candidate.limit <= 0 || candidate.branch >= len(solution) {
				continue
			}
			if outputLimited && hasMainBranch && candidate.branch == mainBranch {
				continue
			}
			state, limited := branchLimits[candidate.branch]
			if limited {
				if stickyBranchLimits[candidate.branch] {
					continue
				}
				residual := transientBranchEquationResidual(base, candidate.branch, solution)
				if residual*math.Copysign(1, state) < -nonlinearClampConsistencyV {
					next := cloneTransientBranchLimits(branchLimits)
					delete(next, candidate.branch)
					return outputLimits, next, true
				}
				continue
			}
			current := real(solution[candidate.branch])
			if math.Abs(current) > candidate.limit*(1+1e-9) {
				next := cloneTransientBranchLimits(branchLimits)
				next[candidate.branch] = math.Copysign(candidate.limit, current)
				return outputLimits, next, true
			}
		}
	}
	return outputLimits, branchLimits, false
}

func transientOutputLimitObservation(base mnaSystem, device ResolvedDevice, solution []complex128) (minimum, maximum, output, clampTolerance float64, ok bool) {
	switch device.PrimitiveModel {
	case PrimitiveOpAmpV1:
		terminals := terminalMap(device)
		positive := nonlinearNodeVoltage(&base, solution, terminals["V_PLUS"])
		negative := nonlinearNodeVoltage(&base, solution, terminals["V_MINUS"])
		return negative + transientModelParameter(device.ModelParameters, "output_low_margin_v"),
			positive - transientModelParameter(device.ModelParameters, "output_high_margin_v"),
			nonlinearNodeVoltage(&base, solution, terminals["OUT"]), nonlinearClampConsistencyV, true
	case PrimitiveCurrentSenseAmplifierV1:
		terminals := terminalMap(device)
		ground := nonlinearNodeVoltage(&base, solution, terminals["GND_A"])
		supply := nonlinearNodeVoltage(&base, solution, terminals["VCC"]) - ground
		return transientModelParameter(device.ModelParameters, "output_low_margin_v"),
			supply - transientModelParameter(device.ModelParameters, "output_high_margin_v"),
			nonlinearNodeVoltage(&base, solution, terminals["OUT"]) - ground, mnaPivotTolerance, true
	case PrimitiveSynchronousBuckRegulatorV1:
		terminals := terminalMap(device)
		ground := nonlinearNodeVoltage(&base, solution, terminals["PGND"])
		input := nonlinearNodeVoltage(&base, solution, terminals["PVIN"]) - ground
		return 0, math.Max(0, input), nonlinearNodeVoltage(&base, solution, terminals["SW"]) - ground, mnaPivotTolerance, true
	default:
		return 0, 0, 0, 0, false
	}
}

func transientModelParameter(parameters []NamedValue, name string) float64 {
	for _, parameter := range parameters {
		if parameter.Name == name {
			return parameter.Value
		}
	}
	return 0
}

type transientBranchLimitCandidate struct {
	branch int
	limit  float64
}

func transientBranchLimitCandidates(base mnaSystem, devices []ResolvedDevice, device ResolvedDevice) []transientBranchLimitCandidate {
	switch device.PrimitiveModel {
	case PrimitiveAdjustableLinearRegulatorV1, PrimitiveFixedLinearRegulatorV1, PrimitiveFloatingAdjustableRegulatorV1:
		branch, exists := base.branchIndex[device.Component]
		if !exists {
			return nil
		}
		return []transientBranchLimitCandidate{{branch: branch, limit: transientModelParameter(device.ModelParameters, "max_load_current_a")}}
	case PrimitiveFixedBuckModuleV1:
		branch, exists := base.branchIndex[device.Component]
		if !exists {
			return nil
		}
		return []transientBranchLimitCandidate{{branch: branch, limit: transientModelParameter(device.ModelParameters, "max_output_current_a")}}
	case PrimitiveSynchronousBuckRegulatorV1:
		branch, exists := base.branchIndex[device.Component]
		if !exists {
			return nil
		}
		return []transientBranchLimitCandidate{{branch: branch, limit: transientModelParameter(device.ModelParameters, "peak_current_limit_a")}}
	case PrimitiveDualOutputIsolatedConverterV1:
		return []transientBranchLimitCandidate{
			{branch: base.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_PLUS"}], limit: transientModelParameter(device.ModelParameters, "positive_max_output_current_a")},
			{branch: base.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_MINUS"}], limit: transientModelParameter(device.ModelParameters, "negative_max_output_current_a")},
		}
	case PrimitiveSingleOutputIsolatedConverterV1, PrimitiveProtectedIsolatedConverterV1:
		return []transientBranchLimitCandidate{{branch: base.branchIndex[device.Component], limit: transientModelParameter(device.ModelParameters, "max_output_current_a")}}
	default:
		return nil
	}
}

func transientBranchEquationResidual(base mnaSystem, branch int, solution []complex128) float64 {
	residual := -base.rhs[branch]
	for column, coefficient := range base.matrix[branch] {
		residual += coefficient * solution[column]
	}
	return real(residual)
}

func cloneTransientOutputLimits(source map[string]transientOutputLimitState) map[string]transientOutputLimitState {
	clone := make(map[string]transientOutputLimitState, len(source)+1)
	for component, state := range source {
		clone[component] = state
	}
	return clone
}

func cloneTransientBranchLimits(source map[int]float64) map[int]float64 {
	clone := make(map[int]float64, len(source)+1)
	for branch, value := range source {
		clone[branch] = value
	}
	return clone
}

func cloneComplexSolution(source []complex128) []complex128 {
	return append([]complex128(nil), source...)
}

func seedTransientOutputLimitGuess(base mnaSystem, devices []ResolvedDevice, guess []complex128, current, next map[string]transientOutputLimitState) {
	for _, device := range devices {
		nextState, limited := next[device.Component]
		if !limited {
			continue
		}
		currentState, wasLimited := current[device.Component]
		if wasLimited && sameTransientOutputLimitState(currentState, nextState) {
			continue
		}
		if len(nextState.lowerSolution) == len(guess) && len(nextState.upperSolution) == len(guess) && nextState.upper > nextState.lower {
			fraction := (nextState.value - nextState.lower) / (nextState.upper - nextState.lower)
			fraction = math.Max(0, math.Min(1, fraction))
			for index := range guess {
				guess[index] = nextState.lowerSolution[index]*(1-complex(fraction, 0)) + nextState.upperSolution[index]*complex(fraction, 0)
			}
		}
		terminals := terminalMap(device)
		outputIndex, exists := base.nodeIndex[terminals["OUT"]]
		if !exists || outputIndex >= len(guess) {
			continue
		}
		value := nextState.value
		if device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1 {
			value += nonlinearNodeVoltage(&base, guess, terminals["GND_A"])
		}
		guess[outputIndex] = complex(value, 0)
	}
}

func sameTransientOutputLimitState(left, right transientOutputLimitState) bool {
	return left.side == right.side && left.value == right.value && left.lower == right.lower && left.upper == right.upper && left.lowerResidual == right.lowerResidual && left.upperResidual == right.upperResidual
}

func transientActiveLimitStateKey(devices []ResolvedDevice, outputLimits map[string]transientOutputLimitState, branchLimits map[int]float64) string {
	var key strings.Builder
	for _, device := range devices {
		if state, exists := outputLimits[device.Component]; exists {
			fmt.Fprintf(&key, "output:%s:%d:%.12g:%.12g:%.12g:%.12g:%.12g;", device.Component, state.side, state.value, state.lower, state.upper, state.lowerResidual, state.upperResidual)
		}
	}
	branches := make([]int, 0, len(branchLimits))
	for branch := range branchLimits {
		branches = append(branches, branch)
	}
	slices.Sort(branches)
	for _, branch := range branches {
		fmt.Fprintf(&key, "branch:%d:%.12g;", branch, branchLimits[branch])
	}
	return key.String()
}

func transientActiveLimitSolverStateKey(devices []ResolvedDevice, outputLimits map[string]transientOutputLimitState, branchLimits map[int]float64, deferredOutputLimits map[string]bool, deferredBranchLimits map[int]bool) string {
	var key strings.Builder
	key.WriteString(transientActiveLimitStateKey(devices, outputLimits, branchLimits))
	for _, device := range devices {
		if deferredOutputLimits[device.Component] {
			key.WriteString("deferred-output:" + device.Component + ";")
		}
	}
	branches := make([]int, 0, len(deferredBranchLimits))
	for branch := range deferredBranchLimits {
		branches = append(branches, branch)
	}
	slices.Sort(branches)
	for _, branch := range branches {
		fmt.Fprintf(&key, "deferred-branch:%d;", branch)
	}
	return key.String()
}

func validateTransientOperatingLimits(
	plan Plan,
	system mnaSystem,
	solution []complex128,
	comparatorStates map[string]float64,
	allowPowerTransition bool,
	timeStepS float64,
	fuseI2TStates map[string]transientFuseI2TState,
	openFuses map[string]bool,
) ([]string, []Diagnostic) {
	diagnostics := validateNonlinearOperatingLimitsWithComparatorStates(plan, system, solution, comparatorStates, allowPowerTransition, true)
	var openedFuses []string
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		parameters := deviceParameterMap(device)
		switch device.PrimitiveModel {
		case PrimitiveFuseClosedStateV1:
			meltingI2t, hasSurgeEvidence := parameters["nominal_melting_i2t_a2s"]
			if !hasSurgeEvidence || meltingI2t <= 0 || timeStepS <= 0 || fuseI2TStates == nil {
				continue
			}
			voltage := nonlinearNodeVoltage(&system, solution, terminals["A"]) - nonlinearNodeVoltage(&system, solution, terminals["B"])
			current := math.Abs(voltage / parameters["cold_resistance_ohm"])
			rated := parameters["rated_current_a"]
			state := advanceTransientFuseI2T(fuseI2TStates[device.Component], current, rated, timeStepS)
			fuseI2TStates[device.Component] = state
			path := "devices." + device.Component + ".operating_limit"
			diagnostics = slices.DeleteFunc(diagnostics, func(diagnostic Diagnostic) bool {
				return diagnostic.Path == path && strings.HasPrefix(diagnostic.Message, "fuse current ")
			})
			if state.integralA2S >= meltingI2t {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("fuse current-squared integral %.12g A^2s exceeds catalog-backed nominal melting I2t %.12g A^2s", state.integralA2S, meltingI2t), Suggestion: "reduce transient current, select a fuse with sufficient reviewed surge capacity, or use a registered time-current clearing model"})
			}
		case PrimitiveFuseI2TClearingV1:
			voltage := nonlinearNodeVoltage(&system, solution, terminals["A"]) - nonlinearNodeVoltage(&system, solution, terminals["B"])
			if math.Abs(voltage) > parameters["max_voltage_v"] {
				diagnostics = append(diagnostics, Diagnostic{
					Path:       "devices." + device.Component + ".operating_limit",
					Message:    fmt.Sprintf("cleared fuse voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(voltage), parameters["max_voltage_v"]),
					Suggestion: "reduce the applied voltage or select a fuse with a sufficient reviewed interrupting-voltage rating",
				})
			}
			if openFuses[device.Component] || timeStepS <= 0 || fuseI2TStates == nil {
				continue
			}
			current := math.Abs(voltage / parameters["cold_resistance_ohm"])
			rated := parameters["rated_current_a"]
			state := advanceTransientFuseI2T(fuseI2TStates[device.Component], current, rated, timeStepS)
			fuseI2TStates[device.Component] = state
			if state.integralA2S >= parameters["nominal_melting_i2t_a2s"] {
				openedFuses = append(openedFuses, device.Component)
			}
		case PrimitiveCapacitorTransientV1:
			voltage := nonlinearNodeVoltage(&system, solution, terminals["A"]) - nonlinearNodeVoltage(&system, solution, terminals["B"])
			limit := parameters["max_voltage_v"]
			if math.Abs(voltage) > limit {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("capacitor voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(voltage), limit), Suggestion: "reduce applied voltage or select a suitably rated reviewed capacitor"})
			}
		case PrimitiveInductorTransientV1:
			current := math.Abs(real(solution[system.branchIndex[device.Component]]))
			rated := parameters["rated_current_a"]
			saturation := parameters["saturation_current_a"]
			if current > saturation {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("inductor current %.12g A exceeds catalog-backed saturation current %.12g A", current, saturation), Suggestion: "reduce peak current or select a reviewed inductor with sufficient saturation current"})
			} else if current > rated {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("inductor current %.12g A exceeds catalog-backed rated current %.12g A", current, rated), Suggestion: "reduce RMS current or select a reviewed inductor with sufficient current rating"})
			}
		}
	}
	slices.Sort(openedFuses)
	return slices.Compact(openedFuses), diagnostics
}

func prefixTransientDiagnostics(analysisID string, step int, timeS float64, diagnostics []Diagnostic) []Diagnostic {
	for index := range diagnostics {
		diagnostics[index].Path = fmt.Sprintf("analyses.%s.points[%d].%s", analysisID, step, diagnostics[index].Path)
		diagnostics[index].Message = fmt.Sprintf("transient operating point at %.12g s: %s", timeS, diagnostics[index].Message)
	}
	return diagnostics
}
