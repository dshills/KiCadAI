package simmodel

import (
	"fmt"
	"math"
	"strings"
)

const transientGmin = nonlinearFinalGmin

func solveTransientAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	steps := int(math.Round(analysis.DurationS / analysis.TimeStepS))
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisTransient, Points: make([]AnalysisPoint, 0, steps+1)}
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
	initialEvidence.TotalIterations = initialEvidence.Iterations
	initialEvidence.MaxIterationsPerStep = nonlinearMaxIterations
	initialEvidence.MaxTotalIterations = maxTransientWork
	initialStates, _, initialStateDiagnostic := resolvedComparatorStates(plan, system, solution)
	if initialStateDiagnostic != nil {
		return result, []Diagnostic{*initialStateDiagnostic}
	}
	if diagnostics := validateTransientOperatingLimits(plan, system, solution, initialStates); len(diagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 0, 0, diagnostics)
	}
	result.Points = append(result.Points, AnalysisPoint{TimeS: 0, Nodes: nodeResults(plan, system, solution), Devices: electricalDeviceResults(plan, initialAnalysis, 0, system, solution), Solver: &initialEvidence})
	history := [][]complex128{append([]complex128(nil), solution...)}

	devices := compileNonlinearDevices(plan)
	totalIterations := initialEvidence.Iterations
	template, diagnostics := buildTransientTemplate(plan, analysis)
	if len(diagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 1, analysis.TimeStepS, diagnostics)
	}
	base := cloneMNASystem(template)
	workspace := cloneMNASystem(template)
	guess := make([]complex128, len(solution))
	for step := 1; step <= steps; step++ {
		// Derive time directly from the integer grid index; never accumulate it.
		timeS := float64(step) * analysis.TimeStepS
		comparatorStates, diagnostics := prepareTransientBase(&base, template, plan, analysis, step, timeS, solution, history)
		if len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		var evidence SolverEvidence
		system, solution, evidence, diagnostic = solveTransientStep(base, devices, solution, guess, &workspace)
		totalIterations += evidence.Iterations
		evidence.InitialCondition = "previous_accepted_state"
		evidence.TimeSteps = step
		evidence.TotalIterations = totalIterations
		evidence.MaxIterationsPerStep = nonlinearMaxIterations
		evidence.MaxTotalIterations = maxTransientWork
		if diagnostic != nil {
			diagnostic.Path = fmt.Sprintf("analyses.%s.points[%d].%s", analysis.ID, step, diagnostic.Path)
			diagnostic.Message = fmt.Sprintf("transient solve failed at step %d, time %.12g s: %s", step, timeS, diagnostic.Message)
			return result, []Diagnostic{*diagnostic}
		}
		if diagnostics := validateTransientOperatingLimits(plan, system, solution, comparatorStates); len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		result.Points = append(result.Points, AnalysisPoint{TimeS: normalizedMNAFloat(timeS), Nodes: nodeResults(plan, system, solution), Devices: electricalDeviceResultsWithComparatorStates(plan, analysis, 0, system, solution, comparatorStates), Solver: &evidence})
		history = append(history, append([]complex128(nil), solution...))
	}
	return result, nil
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
		MaxIterationsPerStep: nonlinearMaxIterations, MaxTotalIterations: maxTransientWork,
	}
	result.Points = append(result.Points, AnalysisPoint{Nodes: nodeResults(plan, template, previous), Devices: electricalDeviceResults(plan, analysis, 0, template, previous), Solver: &initialEvidence})

	devices := compileNonlinearDevices(plan)
	base := cloneMNASystem(template)
	workspace := cloneMNASystem(template)
	guess := make([]complex128, len(previous))
	totalIterations := 0
	for step := 1; step <= steps; step++ {
		timeS := float64(step) * analysis.TimeStepS
		comparatorStates, diagnostics := prepareTransientBase(&base, template, plan, analysis, step, timeS, previous, history)
		if len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		system, solution, evidence, diagnostic := solveTransientStep(base, devices, previous, guess, &workspace)
		totalIterations += evidence.Iterations
		evidence.InitialCondition = "previous_accepted_startup_state"
		evidence.TimeSteps = step
		evidence.TotalIterations = totalIterations
		evidence.MaxIterationsPerStep = nonlinearMaxIterations
		evidence.MaxTotalIterations = maxTransientWork
		if diagnostic != nil {
			diagnostic.Path = fmt.Sprintf("analyses.%s.points[%d].%s", analysis.ID, step, diagnostic.Path)
			diagnostic.Message = fmt.Sprintf("startup solve failed at step %d, time %.12g s: %s", step, timeS, diagnostic.Message)
			return result, []Diagnostic{*diagnostic}
		}
		if diagnostics := validateTransientOperatingLimits(plan, system, solution, comparatorStates); len(diagnostics) != 0 {
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
	for _, point := range result.Points {
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
		if device.PrimitiveModel != PrimitiveCapacitorTransientV1 {
			continue
		}
		terminals := terminalMap(device)
		conductance := *device.ValueSI / analysis.TimeStepS
		stampAdmittance(&system, terminals["A"], terminals["B"], complex(conductance, 0))
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

func prepareTransientBase(base *mnaSystem, template mnaSystem, plan Plan, analysis Analysis, step int, timeS float64, previous []complex128, history [][]complex128) (map[string]float64, []Diagnostic) {
	resetMNASystem(base, template)
	comparatorStates := map[string]float64{}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveVoltageSourceV1, PrimitiveConnectorVoltageSourceV1:
			base.rhs[base.branchIndex[device.Component]] += complex(transientExcitationValue(analysis, device.Component, timeS), 0)
		case PrimitiveCurrentSourceV1:
			stampCurrentSource(base, terminals["POSITIVE"], terminals["NEGATIVE"], complex(transientExcitationValue(analysis, device.Component, timeS), 0))
		case PrimitiveCapacitorTransientV1:
			conductance := *device.ValueSI / analysis.TimeStepS
			previousVoltage := nonlinearNodeVoltage(base, previous, terminals["A"]) - nonlinearNodeVoltage(base, previous, terminals["B"])
			stampCurrentSource(base, terminals["A"], terminals["B"], complex(-conductance*previousVoltage, 0))
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
		}
	}
	if diagnostic := validateMNASystemBounds(*base); diagnostic != nil {
		return comparatorStates, []Diagnostic{*diagnostic}
	}
	return comparatorStates, nil
}

func transientExcitationValue(analysis Analysis, component string, timeS float64) float64 {
	for _, excitation := range analysis.Excitations {
		if excitation.Component == component {
			return transientSourceValue(excitation, timeS, analysis.TimeStepS)
		}
	}
	return 0
}

func solveTransientStep(base mnaSystem, devices []compiledNonlinearDevice, previous, guess []complex128, workspace *mnaSystem) (mnaSystem, []complex128, SolverEvidence, *Diagnostic) {
	copy(guess, previous)
	system := workspace
	evidence := SolverEvidence{Method: "backward_euler_bounded_newton_v1"}
	largestUpdateLabel, largestCurrentUpdateLabel, largestResidualLabel := "unknown", "unknown", "unknown"
	for iteration := 1; iteration <= nonlinearMaxIterations; iteration++ {
		resetMNASystem(system, base)
		stampCompiledNonlinearDevices(system, devices, guess)
		if diagnostic := validateMNASystemBounds(*system); diagnostic != nil {
			return mnaSystem{}, nil, evidence, diagnostic
		}
		candidate, diagnostic := solveMNA(*system)
		if diagnostic != nil {
			diagnostic.Suggestion = "correct floating nodes or conflicting source constraints, reduce the fixed observation step, or select compatible reviewed dynamic models"
			return mnaSystem{}, nil, evidence, diagnostic
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
		maxResidual, label := nonlinearResidual(base, devices, guess)
		largestResidualLabel = label
		evidence.Iterations++
		evidence.FinalMaxUpdateV = normalizedMNAFloat(maxAppliedUpdate)
		evidence.FinalMaxCurrentUpdateA = normalizedMNAFloat(maxAppliedCurrentUpdate)
		evidence.FinalMaxResidual = normalizedMNAFloat(maxResidual)
		if maxAppliedUpdate <= nonlinearUpdateTolerance && maxAppliedCurrentUpdate <= nonlinearCurrentUpdateTolerance && maxResidual <= nonlinearResidualTolerance {
			return *system, guess, evidence, nil
		}
	}
	return mnaSystem{}, nil, evidence, &Diagnostic{Path: "convergence", Message: fmt.Sprintf("fixed backward-Euler Newton solve did not converge within %d iterations; largest voltage update %s, largest current update %s, largest residual %s", nonlinearMaxIterations, largestUpdateLabel, largestCurrentUpdateLabel, largestResidualLabel), Suggestion: "reduce the bounded observation step, add a catalog-backed bias path, or correct incompatible source and switching conditions"}
}

func validateTransientOperatingLimits(plan Plan, system mnaSystem, solution []complex128, comparatorStates map[string]float64) []Diagnostic {
	diagnostics := validateNonlinearOperatingLimitsWithComparatorStates(plan, system, solution, comparatorStates)
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		parameters := namedValueMap(device.ModelParameters)
		switch device.PrimitiveModel {
		case PrimitiveCapacitorTransientV1:
			voltage := nonlinearNodeVoltage(&system, solution, terminals["A"]) - nonlinearNodeVoltage(&system, solution, terminals["B"])
			limit := parameters["max_voltage_v"]
			if math.Abs(voltage) > limit {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("capacitor voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(voltage), limit), Suggestion: "reduce applied voltage or select a suitably rated reviewed capacitor"})
			}
		case PrimitiveComparatorOpenCollectorV1:
			supply := nonlinearNodeVoltage(&system, solution, terminals["V_PLUS"]) - nonlinearNodeVoltage(&system, solution, terminals["V_MINUS"])
			if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("transient supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog comparator"})
			}
			if comparatorStates[device.Component] >= .5 {
				current := math.Abs((nonlinearNodeVoltage(&system, solution, terminals["OUT"]) - nonlinearNodeVoltage(&system, solution, terminals["V_MINUS"])) / parameters["output_on_resistance_ohm"])
				if current > parameters["max_sink_current_a"] {
					diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("comparator sink current %.12g A exceeds catalog-backed limit %.12g A", current, parameters["max_sink_current_a"]), Suggestion: "increase output pull-up resistance or select a compatible reviewed comparator"})
				}
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
