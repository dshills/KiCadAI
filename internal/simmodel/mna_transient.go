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
	transientMaxNewtonIterations             = 512
	transientSourceContinuationStages        = 4
	transientMaxNewtonAttemptsPerObservation = 2 * (1 + transientSourceContinuationStages)
	transientActiveLimitContinuationStepV    = 1
)

func solveTransientAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	steps := int(math.Round(analysis.DurationS / analysis.TimeStepS))
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisTransient, Points: make([]AnalysisPoint, 0, steps+1)}
	for _, excitation := range analysis.Excitations {
		if excitation.SineFrequencyHz > 0 {
			result.FundamentalFrequencyHz = excitation.SineFrequencyHz
			break
		}
	}
	initialAnalysis := transientDCAnalysis(analysis, 0)
	// The trusted DC initializer applies global source/gmin continuation and
	// ends at nonlinearFinalGmin, exactly the conductance used by later steps.
	system, solution, initialEvidence, diagnostic := solveNonlinearDC(plan, initialAnalysis)
	if diagnostic != nil {
		diagnostic.Path = "analyses." + analysis.ID + ".initial_condition." + diagnostic.Path
		diagnostic.Message = "deterministic transient initial condition failed: " + diagnostic.Message
		return result, []Diagnostic{*diagnostic}
	}
	initialEvidence.InitialCondition = "bounded_nonlinear_dc_v1"
	if transientSourcesInitiallyZero(analysis) {
		// A pulsed power-up workflow has no energized DC operating point at t=0.
		// Preserve the resolved matrix/labels but start all algebraic and dynamic
		// unknowns at zero so regulated rails cannot be precharged by a model's
		// nominal reference before its input source turns on.
		solution = make([]complex128, len(solution))
		initialEvidence.InitialCondition = "zero_energy_transient_v1"
	}
	initialEvidence.TotalIterations = initialEvidence.Iterations
	initialEvidence.MaxIterationsPerStep = transientMaxNewtonIterations
	initialEvidence.MaxTotalIterations = maxTransientWork
	initialStates, _, initialStateDiagnostic := resolvedActiveDeviceStates(plan, system, solution)
	if initialStateDiagnostic != nil {
		return result, []Diagnostic{*initialStateDiagnostic}
	}
	if diagnostics := validateTransientOperatingLimits(plan, system, solution, initialStates, transientSourcesInitiallyZero(analysis), 0, nil); len(diagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 0, 0, diagnostics)
	}
	result.Points = append(result.Points, AnalysisPoint{TimeS: 0, Nodes: nodeResults(plan, system, solution), Devices: transientObservationDeviceResults(plan, analysis, initialAnalysis, system, solution, nil), Solver: &initialEvidence})
	history := [][]complex128{append([]complex128(nil), solution...)}

	devices := compileNonlinearDevices(plan)
	totalIterations := initialEvidence.Iterations
	template, diagnostics := buildTransientTemplate(plan, analysis)
	if len(diagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 1, analysis.TimeStepS, diagnostics)
	}
	base := cloneMNASystem(template)
	workspace := cloneMNASystem(template)
	fuseSurgeI2t := map[string]float64{}
	for step := 1; step <= steps; step++ {
		// Derive time directly from the integer grid index; never accumulate it.
		timeS := float64(step) * analysis.TimeStepS
		comparatorStates, fixedOutputClamps, diagnostics := prepareTransientBase(&base, template, plan, analysis, step, timeS, solution, history)
		if len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		guess := transientInitialGuess(analysis, timeS, solution, history)
		var evidence SolverEvidence
		previousSolution := solution
		system, solution, evidence, diagnostic = solveTransientStep(base, plan.Devices, devices, previousSolution, guess, &workspace, false, fixedOutputClamps)
		totalIterations += evidence.Iterations
		if diagnostic != nil {
			priorBase := cloneMNASystem(template)
			_, _, priorDiagnostics := prepareTransientBase(&priorBase, template, plan, analysis, step-1, timeS-analysis.TimeStepS, previousSolution, history)
			if len(priorDiagnostics) == 0 {
				var continuationEvidence SolverEvidence
				system, solution, continuationEvidence, diagnostic = solveTransientStepWithSourceContinuation(priorBase, base, plan.Devices, devices, previousSolution, &workspace, fixedOutputClamps)
				totalIterations += continuationEvidence.Iterations
				evidence = continuationEvidence
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
		if diagnostics := validateTransientOperatingLimits(plan, system, solution, comparatorStates, transientSourcesZeroAtTime(analysis, timeS), analysis.TimeStepS, fuseSurgeI2t); len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		result.Points = append(result.Points, AnalysisPoint{TimeS: normalizedMNAFloat(timeS), Nodes: nodeResults(plan, system, solution), Devices: transientObservationDeviceResults(plan, analysis, analysis, system, solution, comparatorStates), Solver: &evidence})
		history = append(history, append([]complex128(nil), solution...))
	}
	return result, nil
}

func transientObservationDeviceResults(plan Plan, observation, evaluation Analysis, system mnaSystem, solution []complex128, comparatorStates map[string]float64) []DeviceResult {
	if observation.Kind == AnalysisDistortion {
		return nil
	}
	return electricalDeviceResultsWithComparatorStates(plan, evaluation, 0, system, solution, comparatorStates)
}

func transientSourcesInitiallyZero(analysis Analysis) bool {
	return transientSourcesZeroAtTime(analysis, 0)
}

func transientSourcesZeroAtTime(analysis Analysis, timeS float64) bool {
	if len(analysis.Excitations) == 0 {
		return false
	}
	for _, excitation := range analysis.Excitations {
		if math.Abs(transientExcitationValue(analysis, excitation.Component, timeS)) > 1e-15 {
			return false
		}
	}
	return true
}

// solveStartupAnalysis applies every bounded DC source after a canonical
// zero-energy point. Unlike ordinary transient analysis, it deliberately does
// not solve a steady-state operating point first: capacitor voltages and all
// algebraic unknowns begin at zero, making power-up overshoot reproducible.
func solveStartupAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
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
	result.Points = append(result.Points, AnalysisPoint{Nodes: nodeResults(plan, template, previous), Devices: electricalDeviceResults(plan, analysis, 0, template, previous), Solver: &initialEvidence})

	devices := compileNonlinearDevices(plan)
	base := cloneMNASystem(template)
	workspace := cloneMNASystem(template)
	totalIterations := 0
	fuseSurgeI2t := map[string]float64{}
	for step := 1; step <= steps; step++ {
		timeS := float64(step) * analysis.TimeStepS
		comparatorStates, fixedOutputClamps, diagnostics := prepareTransientBase(&base, template, plan, analysis, step, timeS, previous, history)
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
		if diagnostics := validateTransientOperatingLimits(plan, system, solution, comparatorStates, true, analysis.TimeStepS, fuseSurgeI2t); len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		result.Points = append(result.Points, AnalysisPoint{TimeS: normalizedMNAFloat(timeS), Nodes: nodeResults(plan, system, solution), Devices: electricalDeviceResultsWithComparatorStates(plan, analysis, 0, system, solution, comparatorStates), Solver: &evidence})
		previous = solution
		history = append(history, append([]complex128(nil), solution...))
	}
	return result, nil
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
	for index := range dc.Excitations {
		dc.Excitations[index].DCValue = transientSourceValue(dc.Excitations[index], timeS, analysis.TimeStepS)
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
	return dc
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
		case PrimitiveBidirectionalTVSV1:
			conductance := namedValueMap(device.ModelParameters)["junction_capacitance_f"] / analysis.TimeStepS
			stampAdmittance(&system, terminals["ANODE"], terminals["CATHODE"], complex(conductance, 0))
		case PrimitiveOpAmpV1:
			parameters := namedValueMap(device.ModelParameters)
			gain := parameters["dc_open_loop_gain"]
			poleHz := parameters["gain_bandwidth_hz"] / gain
			historyCoefficient := 1 / (2 * math.Pi * poleHz * gain * analysis.TimeStepS)
			if outputIndex, exists := system.nodeIndex[terminals["OUT"]]; exists {
				system.matrix[system.branchIndex[device.Component]][outputIndex] += complex(historyCoefficient, 0)
			}
		case PrimitiveCurrentSenseAmplifierV1:
			parameters := namedValueMap(device.ModelParameters)
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
			parameters := namedValueMap(device.ModelParameters)
			system.rhs[system.branchIndex[device.Component]] -= complex(parameters["reference_voltage_v"], 0)
			stampCurrentSource(&system, terminals["VIN"], terminals["GND"], complex(-parameters["quiescent_current_a"], 0))
		case PrimitiveFixedLinearRegulatorV1:
			parameters := namedValueMap(device.ModelParameters)
			system.rhs[system.branchIndex[device.Component]] -= complex(parameters["output_voltage_v"], 0)
			stampCurrentSource(&system, terminals["VIN"], terminals["GND"], complex(-parameters["quiescent_current_a"], 0))
		case PrimitiveFloatingAdjustableRegulatorV1:
			parameters := namedValueMap(device.ModelParameters)
			reference := parameters["polarity"] * parameters["reference_voltage_v"]
			adjustmentCurrent := parameters["polarity"] * parameters["adjustment_pin_current_a"]
			system.rhs[system.branchIndex[device.Component]] -= complex(reference, 0)
			stampCurrentSource(&system, terminals["VIN"], terminals["ADJ"], complex(-adjustmentCurrent, 0))
		case PrimitiveProgrammableCurrentSourceV1:
			parameters := namedValueMap(device.ModelParameters)
			system.rhs[system.branchIndex[device.Component]] -= complex(parameters["offset_voltage_v"], 0)
			stampCurrentSource(&system, terminals["IN"], terminals["SET"], complex(-parameters["reference_current_a"], 0))
		case PrimitiveShuntVoltageReferenceV1:
			system.rhs[system.branchIndex[device.Component]] -= complex(namedValueMap(device.ModelParameters)["output_voltage_v"], 0)
		case PrimitiveDualOutputIsolatedConverterV1:
			parameters := namedValueMap(device.ModelParameters)
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

func prepareTransientBase(base *mnaSystem, template mnaSystem, plan Plan, analysis Analysis, step int, timeS float64, previous []complex128, history [][]complex128) (map[string]float64, map[string]bool, []Diagnostic) {
	resetMNASystem(base, template)
	comparatorStates := map[string]float64{}
	fixedOutputClamps := map[string]bool{}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveVoltageSourceV1, PrimitiveConnectorVoltageSourceV1:
			value := transientExcitationValue(analysis, device.Component, timeS)
			if analysis.Kind == AnalysisStartup {
				value *= startupSourceRampScale(analysis, timeS)
			}
			base.rhs[base.branchIndex[device.Component]] += complex(value, 0)
		case PrimitiveCurrentSourceV1:
			value := transientExcitationValue(analysis, device.Component, timeS)
			if analysis.Kind == AnalysisStartup {
				value *= startupSourceRampScale(analysis, timeS)
			}
			stampCurrentSource(base, terminals["POSITIVE"], terminals["NEGATIVE"], complex(value, 0))
		case PrimitiveCapacitorTransientV1:
			conductance := *device.ValueSI / analysis.TimeStepS
			previousVoltage := nonlinearNodeVoltage(base, previous, terminals["A"]) - nonlinearNodeVoltage(base, previous, terminals["B"])
			stampCurrentSource(base, terminals["A"], terminals["B"], complex(-conductance*previousVoltage, 0))
		case PrimitiveRelayNormallyOpenV1:
			parameters := namedValueMap(device.ModelParameters)
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
			conductance := namedValueMap(device.ModelParameters)["junction_capacitance_f"] / analysis.TimeStepS
			previousVoltage := nonlinearNodeVoltage(base, previous, terminals["ANODE"]) - nonlinearNodeVoltage(base, previous, terminals["CATHODE"])
			stampCurrentSource(base, terminals["ANODE"], terminals["CATHODE"], complex(-conductance*previousVoltage, 0))
		case PrimitiveComparatorOpenCollectorV1:
			parameters := namedValueMap(device.ModelParameters)
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
			parameters := namedValueMap(device.ModelParameters)
			gain := parameters["dc_open_loop_gain"]
			poleHz := parameters["gain_bandwidth_hz"] / gain
			historyCoefficient := 1 / (2 * math.Pi * poleHz * gain * analysis.TimeStepS)
			base.rhs[base.branchIndex[device.Component]] += complex(historyCoefficient*nonlinearNodeVoltage(base, previous, terminals["OUT"]), 0)
			if analysis.Kind != AnalysisStartup {
				continue
			}
			positive, positiveKnown := transientKnownNodeVoltage(plan, analysis, terminals["V_PLUS"], timeS)
			negative, negativeKnown := transientKnownNodeVoltage(plan, analysis, terminals["V_MINUS"], timeS)
			if startupSourceRampScale(analysis, timeS) < 1 || positiveKnown && negativeKnown && positive-negative < parameters["supply_min_v"] {
				stampTransientOpAmpClamp(base, device.Component, terminals["OUT"], 0)
				fixedOutputClamps[device.Component] = true
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
			parameters := namedValueMap(device.ModelParameters)
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
			parameters := namedValueMap(device.ModelParameters)
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
			parameters := namedValueMap(device.ModelParameters)
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
		case PrimitiveFloatingAdjustableRegulatorV1:
			parameters := namedValueMap(device.ModelParameters)
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
			parameters := namedValueMap(device.ModelParameters)
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
			output := namedValueMap(device.ModelParameters)["output_voltage_v"]
			if analysis.Kind == AnalysisStartup {
				output *= startupSourceRampScale(analysis, timeS)
			}
			base.rhs[base.branchIndex[device.Component]] += complex(output, 0)
		case PrimitiveDualOutputIsolatedConverterV1:
			parameters := namedValueMap(device.ModelParameters)
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
			return transientSourceValue(excitation, timeS, analysis.TimeStepS)
		}
	}
	return 0
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

func solveTransientStepWithSourceContinuation(priorBase, finalBase mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous []complex128, workspace *mnaSystem, fixedOutputClamps map[string]bool) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	guess := append([]complex128(nil), previous...)
	total := SolverEvidence{Method: "backward_euler_bounded_source_continuation_v1"}
	var system mnaSystem
	for stage := 1; stage <= transientSourceContinuationStages; stage++ {
		scale := float64(stage) / transientSourceContinuationStages
		stageBase := cloneMNASystem(finalBase)
		for index := range stageBase.rhs {
			stageBase.rhs[index] = priorBase.rhs[index] + (finalBase.rhs[index]-priorBase.rhs[index])*complex(scale, 0)
		}
		var evidence SolverEvidence
		var diagnostic *Diagnostic
		system, guess, evidence, diagnostic = solveTransientStep(stageBase, resolvedDevices, devices, previous, guess, workspace, false, fixedOutputClamps)
		total.Iterations += evidence.Iterations
		total.TotalIterations = total.Iterations
		total.FinalMaxUpdateV = evidence.FinalMaxUpdateV
		total.FinalMaxCurrentUpdateA = evidence.FinalMaxCurrentUpdateA
		total.FinalMaxResidual = evidence.FinalMaxResidual
		if diagnostic != nil {
			diagnostic.Message = fmt.Sprintf("source-continuation stage %d/%d failed: %s", stage, transientSourceContinuationStages, diagnostic.Message)
			return mnaSystem{}, nil, total, diagnostic
		}
	}
	return system, guess, total, nil
}

func solveTransientStepInternal(base mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous, guess []complex128, workspace *mnaSystem, selectiveNodeDamping bool, fixedOutputClamps map[string]bool, allowMOSFETActiveSet bool) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	system := workspace
	constrainedBase := cloneMNASystem(base)
	outputLimits := map[string]transientOutputLimitState{}
	branchLimits := map[int]float64{}
	deferredOutputLimits := map[string]bool{}
	deferredBranchLimits := map[int]bool{}
	seenLimitStates := map[string]bool{transientActiveLimitSolverStateKey(resolvedDevices, outputLimits, branchLimits, deferredOutputLimits, deferredBranchLimits): true}
	evidence := SolverEvidence{Method: "backward_euler_bounded_newton_v1"}
	largestUpdateLabel, largestCurrentUpdateLabel, largestResidualLabel := "unknown", "unknown", "unknown"
	for iteration := 1; iteration <= transientMaxNewtonIterations; iteration++ {
		resetMNASystem(&constrainedBase, base)
		applyTransientActiveLimits(&constrainedBase, resolvedDevices, outputLimits, branchLimits)
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
			priorResidual, _ := nonlinearResidual(constrainedBase, devices, guess)
			trial := make([]complex128, len(guess))
			for {
				for index := range guess {
					trial[index] = guess[index] + (candidate[index]-guess[index])*complex(damping, 0)
				}
				trialResidual, _ := nonlinearResidual(constrainedBase, devices, trial)
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
		maxResidual, label := nonlinearResidual(constrainedBase, devices, guess)
		largestResidualLabel = label
		evidence.Iterations++
		evidence.FinalMaxUpdateV = normalizedMNAFloat(maxAppliedUpdate)
		evidence.FinalMaxCurrentUpdateA = normalizedMNAFloat(maxAppliedCurrentUpdate)
		evidence.FinalMaxResidual = normalizedMNAFloat(maxResidual)
		if nonlinearIterationConverged(maxAppliedUpdate, maxAppliedCurrentUpdate, maxResidual) {
			nextOutputLimits, nextBranchLimits, activeLimitChanged := advanceTransientActiveLimitState(base, resolvedDevices, guess, outputLimits, branchLimits, fixedOutputClamps)
			if activeLimitChanged {
				for component := range outputLimits {
					if _, remainsLimited := nextOutputLimits[component]; !remainsLimited {
						deferredOutputLimits[component] = true
					}
				}
				for branch := range branchLimits {
					if _, remainsLimited := nextBranchLimits[branch]; !remainsLimited {
						deferredBranchLimits[branch] = true
					}
				}
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
	diagnostic := &Diagnostic{Path: "convergence", Message: fmt.Sprintf("fixed backward-Euler Newton solve did not converge within %d iterations; active limits %s; largest voltage update %s %.12g V, largest current update %s %.12g A, largest normalized residual %s %.12g", transientMaxNewtonIterations, transientActiveLimitStateKey(resolvedDevices, outputLimits, branchLimits), largestUpdateLabel, evidence.FinalMaxUpdateV, largestCurrentUpdateLabel, evidence.FinalMaxCurrentUpdateA, largestResidualLabel, evidence.FinalMaxResidual), Suggestion: "reduce the bounded observation step, add a catalog-backed bias path, or correct incompatible source and switching conditions"}
	if allowMOSFETActiveSet {
		if activeSystem, activeSolution, activeEvidence, ok := solveTransientStepByMOSFETActiveSet(base, resolvedDevices, devices, previous, workspace, selectiveNodeDamping, fixedOutputClamps); ok {
			activeEvidence.Iterations += evidence.Iterations
			activeEvidence.TotalIterations = activeEvidence.Iterations
			return activeSystem, activeSolution, activeEvidence, nil
		}
	}
	return mnaSystem{}, nil, evidence, diagnostic
}

func solveTransientStepByMOSFETActiveSet(base mnaSystem, resolvedDevices []ResolvedDevice, devices []compiledNonlinearDevice, previous []complex128, workspace *mnaSystem, selectiveNodeDamping bool, fixedOutputClamps map[string]bool) (mnaSystem, []complex128, SolverEvidence, bool) {
	var switches []string
	for _, device := range devices {
		if device.primitive == PrimitiveNMOSSwitchV1 || device.primitive == PrimitivePMOSSwitchV1 {
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
			return next, branchLimits, true
		case output > maximum+tolerance:
			next := cloneTransientOutputLimits(outputLimits)
			next[device.Component] = transientOutputLimitState{side: 1, value: maximum}
			return next, branchLimits, true
		}
	}
	for _, device := range devices {
		for _, candidate := range transientBranchLimitCandidates(base, device) {
			if candidate.limit <= 0 || candidate.branch >= len(solution) || deferredBranchLimits[candidate.branch] {
				continue
			}
			if _, limited := branchLimits[candidate.branch]; limited {
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

func advanceTransientActiveLimitState(base mnaSystem, devices []ResolvedDevice, solution []complex128, outputLimits map[string]transientOutputLimitState, branchLimits map[int]float64, fixedOutputClamps map[string]bool) (map[string]transientOutputLimitState, map[int]float64, bool) {
	for _, device := range devices {
		if fixedOutputClamps[device.Component] {
			continue
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
					continue
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
		for _, candidate := range transientBranchLimitCandidates(base, device) {
			if candidate.limit <= 0 || candidate.branch >= len(solution) {
				continue
			}
			state, limited := branchLimits[candidate.branch]
			if limited {
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
	terminals := terminalMap(device)
	parameters := namedValueMap(device.ModelParameters)
	switch device.PrimitiveModel {
	case PrimitiveOpAmpV1:
		positive := nonlinearNodeVoltage(&base, solution, terminals["V_PLUS"])
		negative := nonlinearNodeVoltage(&base, solution, terminals["V_MINUS"])
		return negative + parameters["output_low_margin_v"], positive - parameters["output_high_margin_v"], nonlinearNodeVoltage(&base, solution, terminals["OUT"]), nonlinearClampConsistencyV, true
	case PrimitiveCurrentSenseAmplifierV1:
		ground := nonlinearNodeVoltage(&base, solution, terminals["GND_A"])
		supply := nonlinearNodeVoltage(&base, solution, terminals["VCC"]) - ground
		return parameters["output_low_margin_v"], supply - parameters["output_high_margin_v"], nonlinearNodeVoltage(&base, solution, terminals["OUT"]) - ground, mnaPivotTolerance, true
	default:
		return 0, 0, 0, 0, false
	}
}

type transientBranchLimitCandidate struct {
	branch int
	limit  float64
}

func transientBranchLimitCandidates(base mnaSystem, device ResolvedDevice) []transientBranchLimitCandidate {
	parameters := namedValueMap(device.ModelParameters)
	switch device.PrimitiveModel {
	case PrimitiveAdjustableLinearRegulatorV1, PrimitiveFixedLinearRegulatorV1, PrimitiveFloatingAdjustableRegulatorV1:
		branch, exists := base.branchIndex[device.Component]
		if !exists {
			return nil
		}
		return []transientBranchLimitCandidate{{branch: branch, limit: parameters["max_load_current_a"]}}
	case PrimitiveDualOutputIsolatedConverterV1:
		return []transientBranchLimitCandidate{
			{branch: base.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_PLUS"}], limit: parameters["positive_max_output_current_a"]},
			{branch: base.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_MINUS"}], limit: parameters["negative_max_output_current_a"]},
		}
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

func validateTransientOperatingLimits(plan Plan, system mnaSystem, solution []complex128, comparatorStates map[string]float64, allowPowerTransition bool, timeStepS float64, fuseSurgeI2t map[string]float64) []Diagnostic {
	diagnostics := validateNonlinearOperatingLimitsWithComparatorStates(plan, system, solution, comparatorStates, allowPowerTransition)
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		parameters := namedValueMap(device.ModelParameters)
		switch device.PrimitiveModel {
		case PrimitiveFuseClosedStateV1:
			meltingI2t, hasSurgeEvidence := parameters["nominal_melting_i2t_a2s"]
			if !hasSurgeEvidence || meltingI2t <= 0 || timeStepS <= 0 || fuseSurgeI2t == nil {
				continue
			}
			voltage := nonlinearNodeVoltage(&system, solution, terminals["A"]) - nonlinearNodeVoltage(&system, solution, terminals["B"])
			current := math.Abs(voltage / parameters["cold_resistance_ohm"])
			rated := parameters["rated_current_a"]
			if current > rated {
				fuseSurgeI2t[device.Component] += (current*current - rated*rated) * timeStepS
			}
			path := "devices." + device.Component + ".operating_limit"
			diagnostics = slices.DeleteFunc(diagnostics, func(diagnostic Diagnostic) bool {
				return diagnostic.Path == path && strings.HasPrefix(diagnostic.Message, "fuse current ")
			})
			if fuseSurgeI2t[device.Component] > meltingI2t {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("fuse excess-current integral %.12g A^2s exceeds catalog-backed nominal melting I2t %.12g A^2s", fuseSurgeI2t[device.Component], meltingI2t), Suggestion: "reduce transient current, select a fuse with sufficient reviewed surge capacity, or use a registered time-current clearing model"})
			}
		case PrimitiveCapacitorTransientV1:
			voltage := nonlinearNodeVoltage(&system, solution, terminals["A"]) - nonlinearNodeVoltage(&system, solution, terminals["B"])
			limit := parameters["max_voltage_v"]
			if math.Abs(voltage) > limit {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("capacitor voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(voltage), limit), Suggestion: "reduce applied voltage or select a suitably rated reviewed capacitor"})
			}
		}
	}
	return diagnostics
}

func prefixTransientDiagnostics(analysisID string, step int, timeS float64, diagnostics []Diagnostic) []Diagnostic {
	for index := range diagnostics {
		diagnostics[index].Path = fmt.Sprintf("analyses.%s.points[%d].%s", analysisID, step, diagnostics[index].Path)
		diagnostics[index].Message = fmt.Sprintf("transient operating point at %.12g s: %s", timeS, diagnostics[index].Message)
	}
	return diagnostics
}
