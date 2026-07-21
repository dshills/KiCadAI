package simmodel

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	centeredBiasIntervals       = 16
	centeredBiasRailRefinements = 8
)

// CenteredBiasSelection records the deterministic source level chosen from a
// bounded operating-point search. ValueV uses the source primitive's physical
// polarity; Margin is the worst normalized device headroom at that point.
type CenteredBiasSelection struct {
	Source string
	ValueV float64
	Margin float64
}

type centeredBiasWarmState struct {
	activeDeviceStates map[string]float64
	solution           []complex128
}

// SelectFeasibleTransientSourceEdge preserves the requested edge direction
// when its initial endpoint has a valid catalog-backed DC operating point. If
// only the opposite endpoint is feasible, it deterministically reverses the
// edge. This prevents a dynamic workflow from failing before its commanded
// transition solely because the arbitrary initial direction selects an
// unsolved loaded state; both endpoints are still exercised by the transient.
func SelectFeasibleTransientSourceEdge(plan Plan, sourceComponent string, initialV, finalV float64) (float64, float64, []Diagnostic) {
	if !hasTransientEndpointLossDevice(plan) {
		return initialV, finalV, nil
	}
	analysis, ok := centeredBiasAnalysis(plan)
	if !ok {
		return 0, 0, []Diagnostic{{Path: "analyses", Message: "transient edge selection requires one resolved analysis containing the semantic input source"}}
	}
	test := func(value float64) (AnalysisPoint, []Diagnostic) {
		candidate := cloneAnalyses([]Analysis{analysis})[0]
		found := false
		for excitationIndex := range candidate.Excitations {
			excitation := &candidate.Excitations[excitationIndex]
			if excitation.Component == sourceComponent {
				excitation.DCValue = value
				found = true
			}
			excitation.ACMagnitude = 0
			excitation.ACPhaseDeg = 0
			excitation.PulseInitialValue = 0
			excitation.PulseValue = 0
			excitation.PulseDelayS = 0
			excitation.PulseWidthS = 0
			excitation.PulsePeriodS = 0
			excitation.SineAmplitude = 0
			excitation.SineFrequencyHz = 0
			excitation.SinePhaseDeg = 0
		}
		if !found {
			return AnalysisPoint{}, []Diagnostic{{Path: "analyses.excitations." + sourceComponent, Message: "transient edge selection source is absent from the resolved analysis"}}
		}
		candidate.Kind = AnalysisDCOperatingPoint
		candidate.StartFrequencyHz = 0
		candidate.StopFrequencyHz = 0
		candidate.Points = 0
		candidate.DurationS = 0
		candidate.TimeStepS = 0
		candidate.Conditions = nil
		candidate.DCSweep = nil
		return solveCenteredBiasPoint(plan, candidate)
	}
	initialPoint, initialDiagnostics := test(initialV)
	finalPoint, finalDiagnostics := test(finalV)
	if len(initialDiagnostics) == 0 && len(finalDiagnostics) == 0 {
		initialLoss := transientEndpointSemiconductorLoss(plan, initialPoint)
		finalLoss := transientEndpointSemiconductorLoss(plan, finalPoint)
		tolerance := mnaPivotTolerance * math.Max(1, math.Max(initialLoss, finalLoss))
		if finalLoss+tolerance < initialLoss {
			return finalV, initialV, nil
		}
		return initialV, finalV, nil
	}
	if len(initialDiagnostics) == 0 {
		return initialV, finalV, nil
	}
	if len(finalDiagnostics) == 0 {
		return finalV, initialV, nil
	}
	return 0, 0, []Diagnostic{{
		Path:       "analyses.excitations." + sourceComponent,
		Message:    fmt.Sprintf("neither transient edge endpoint has a feasible initial operating point; initial %.12g V: %s; final %.12g V: %s", initialV, initialDiagnostics[0].Message, finalV, finalDiagnostics[0].Message),
		Suggestion: "select a compatible architecture or component, reduce the operating load, or provide a supported initial-state constraint",
	}}
}

func hasTransientEndpointLossDevice(plan Plan) bool {
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case PrimitiveFuseClosedStateV1, PrimitiveBidirectionalTVSV1,
			PrimitiveUnidirectionalZenerV1, PrimitiveDiodeShockleyV1,
			PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1, PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			return true
		}
	}
	return false
}

func transientEndpointSemiconductorLoss(plan Plan, point AnalysisPoint) float64 {
	models := make(map[string]string, len(plan.Devices))
	for _, device := range plan.Devices {
		models[device.Component] = device.PrimitiveModel
	}
	total := 0.0
	for _, device := range point.Devices {
		switch models[device.Component] {
		case PrimitiveFuseClosedStateV1, PrimitiveBidirectionalTVSV1,
			PrimitiveUnidirectionalZenerV1, PrimitiveDiodeShockleyV1,
			PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1, PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			total += math.Abs(device.VoltageV) * device.CurrentMagnitudeA
		}
	}
	return normalizedMNAFloat(total)
}

// SelectThermallyFeasibleSourceBias chooses the semantic input state that
// leaves every catalog-backed dissipative device inside its reviewed
// steady-state junction-temperature envelope. This is used for circuits whose
// load exists only in an active state (for example a comparator-driven load
// switch), where an arbitrary zero-volt input can force a nonphysical
// continuous flyback-current operating point.
func SelectThermallyFeasibleSourceBias(plan Plan, sourceComponent, sourceNode string) (CenteredBiasSelection, []Diagnostic) {
	polarity, ok := sourceNodePolarity(plan, sourceComponent, sourceNode)
	if !ok {
		return CenteredBiasSelection{}, []Diagnostic{{Path: "analyses.excitations." + sourceComponent, Message: "thermal bias requires a voltage source connected to the resolved semantic input node"}}
	}
	rail, ok := independentRailMagnitude(plan, sourceComponent)
	if !ok {
		return CenteredBiasSelection{}, []Diagnostic{{Path: "analyses.excitations." + sourceComponent, Message: "thermal bias requires a nonzero bounded independent supply rail"}}
	}
	analysis, ok := thermalBiasAnalysis(plan)
	if !ok {
		return CenteredBiasSelection{}, []Diagnostic{{Path: "analyses", Message: "thermal bias requires one resolved thermal analysis containing the semantic input source"}}
	}

	best := CenteredBiasSelection{Source: sourceComponent, Margin: math.Inf(-1)}
	found := false
	failureSet := map[string]bool{}
	var failures []string
	recordFailure := func(key, message string) {
		if message == "" || failureSet[key] || len(failures) >= 4 {
			return
		}
		failureSet[key] = true
		failures = append(failures, message)
	}
	for _, magnitude := range centeredBiasCandidateMagnitudes(rail) {
		candidate := polarity * magnitude
		candidateAnalysis := cloneAnalyses([]Analysis{analysis})[0]
		// Bias selection is deliberately a quiescent feasibility search. A
		// periodically driven thermal contract keeps its sine for the final
		// evidence run, but the candidate search must not retain a time grid
		// after its dynamic source fields are cleared below.
		candidateAnalysis.DurationS = 0
		candidateAnalysis.TimeStepS = 0
		for excitationIndex := range candidateAnalysis.Excitations {
			excitation := &candidateAnalysis.Excitations[excitationIndex]
			if excitation.Component == sourceComponent {
				excitation.DCValue = candidate
			}
			excitation.ACMagnitude = 0
			excitation.ACPhaseDeg = 0
			excitation.PulseInitialValue = 0
			excitation.PulseValue = 0
			excitation.PulseDelayS = 0
			excitation.PulseWidthS = 0
			excitation.PulsePeriodS = 0
			excitation.SineAmplitude = 0
			excitation.SineFrequencyHz = 0
			excitation.SinePhaseDeg = 0
		}
		result, diagnostics := solveThermalAnalysis(plan, candidateAnalysis)
		if len(diagnostics) != 0 {
			failure := diagnostics[0].Path + ": " + diagnostics[0].Message
			recordFailure(failure, failure)
			continue
		}
		margin, constrained := thermalBiasMargin(plan, result)
		if !constrained || !finite(margin) || margin < 0 {
			failure := "resolved thermal operating point has no non-negative catalog-backed junction margin"
			recordFailure(failure, failure)
			continue
		}
		if !found || margin > best.Margin+mnaPivotTolerance || math.Abs(margin-best.Margin) <= mnaPivotTolerance && math.Abs(candidate) < math.Abs(best.ValueV) {
			best.ValueV = normalizedMNAFloat(candidate)
			best.Margin = normalizedMNAFloat(margin)
			found = true
		}
	}
	if !found {
		message := fmt.Sprintf("no thermally feasible operating point exists across the bounded %.12g V input range", rail)
		if len(failures) != 0 {
			message += "; candidate failures: " + strings.Join(failures, "; ")
		}
		return CenteredBiasSelection{}, []Diagnostic{{
			Path:       "analyses.excitations." + sourceComponent,
			Message:    message,
			Suggestion: "select a lower-loss architecture or component, improve the reviewed thermal path, or declare a supported operating-state condition",
		}}
	}
	return best, nil
}

func thermalBiasAnalysis(plan Plan) (Analysis, bool) {
	for _, analysis := range plan.Analyses {
		if analysis.Kind == AnalysisThermal {
			return analysis, true
		}
	}
	return Analysis{}, false
}

func thermalBiasMargin(plan Plan, result AnalysisResult) (float64, bool) {
	if len(result.Points) != 1 {
		return 0, false
	}
	devices := make(map[string]ResolvedDevice, len(plan.Devices))
	for _, device := range plan.Devices {
		devices[device.Component] = device
	}
	margin := math.Inf(1)
	constrained := false
	for _, resultDevice := range result.Points[0].Devices {
		if resultDevice.JunctionTemperatureC == nil {
			continue
		}
		device, exists := devices[resultDevice.Component]
		if !exists {
			continue
		}
		maximum, exists := namedValue(namedValueMap(device.ModelParameters), "max_temperature_c")
		if !exists || maximum <= 0 {
			continue
		}
		candidate := (maximum - *resultDevice.JunctionTemperatureC) / maximum
		margin = math.Min(margin, candidate)
		constrained = true
	}
	return margin, constrained
}

// SelectCenteredSourceBias chooses a feasible DC bias for one semantic voltage
// input. Every candidate is solved against the resolved graph, and candidates
// that violate catalog-backed operating limits are discarded. The remaining
// point with the greatest worst normalized active/protection-device headroom is
// selected, with the lowest source magnitude as the deterministic tie-break.
func SelectCenteredSourceBias(plan Plan, sourceComponent, sourceNode string) (CenteredBiasSelection, []Diagnostic) {
	polarity, ok := sourceNodePolarity(plan, sourceComponent, sourceNode)
	if !ok {
		return CenteredBiasSelection{}, []Diagnostic{{Path: "analyses.excitations." + sourceComponent, Message: "centered bias requires a voltage source connected to the resolved semantic input node"}}
	}
	rail, ok := independentRailMagnitude(plan, sourceComponent)
	if !ok {
		return CenteredBiasSelection{}, []Diagnostic{{Path: "analyses.excitations." + sourceComponent, Message: "centered bias requires a nonzero bounded independent supply rail"}}
	}
	analysis, ok := centeredBiasAnalysis(plan)
	if !ok {
		return CenteredBiasSelection{}, []Diagnostic{{Path: "analyses", Message: "centered bias requires one resolved analysis containing the semantic input source"}}
	}

	best := CenteredBiasSelection{Source: sourceComponent, Margin: math.Inf(-1)}
	found := false
	failureSet := map[string]bool{}
	var failures []string
	recordFailure := func(key, message string) {
		if message == "" || failureSet[key] || len(failures) >= 4 {
			return
		}
		failureSet[key] = true
		failures = append(failures, message)
	}
	bestRejected := CenteredBiasSelection{Source: sourceComponent, Margin: math.Inf(-1)}
	bestRejectedLimiter := ""
	warmState := centeredBiasWarmState{}
	for _, magnitude := range centeredBiasCandidateMagnitudes(rail) {
		candidate := polarity * magnitude
		candidateAnalysis := cloneAnalyses([]Analysis{analysis})[0]
		for excitationIndex := range candidateAnalysis.Excitations {
			excitation := &candidateAnalysis.Excitations[excitationIndex]
			if excitation.Component == sourceComponent {
				excitation.DCValue = candidate
			}
			excitation.ACMagnitude = 0
			excitation.ACPhaseDeg = 0
			excitation.PulseInitialValue = 0
			excitation.PulseValue = 0
			excitation.PulseDelayS = 0
			excitation.PulseWidthS = 0
			excitation.PulsePeriodS = 0
			excitation.SineAmplitude = 0
			excitation.SineFrequencyHz = 0
			excitation.SinePhaseDeg = 0
		}
		candidateAnalysis.Kind = AnalysisDCOperatingPoint
		candidateAnalysis.StartFrequencyHz = 0
		candidateAnalysis.StopFrequencyHz = 0
		candidateAnalysis.Points = 0
		candidateAnalysis.DurationS = 0
		candidateAnalysis.TimeStepS = 0
		candidateAnalysis.Conditions = nil
		candidateAnalysis.DCSweep = nil

		point, nextWarmState, diagnostics := solveCenteredBiasPointWarm(plan, candidateAnalysis, warmState)
		if len(nextWarmState.solution) != 0 {
			warmState = nextWarmState
		}
		if len(diagnostics) != 0 {
			recordFailure(diagnostics[0].Path+"\x00"+diagnostics[0].Message, fmt.Sprintf("source %.12g V: %s: %s", candidate, diagnostics[0].Path, diagnostics[0].Message))
			continue
		}
		margin, constrained, limiter := centeredBiasMargin(plan, point)
		if !constrained || !finite(margin) || margin <= mnaPivotTolerance {
			if constrained && finite(margin) && margin > bestRejected.Margin {
				bestRejected.ValueV = normalizedMNAFloat(candidate)
				bestRejected.Margin = normalizedMNAFloat(margin)
				bestRejectedLimiter = limiter
			}
			failure := fmt.Sprintf("source %.12g V: resolved active/protection-device headroom %.12g is not positive", candidate, margin)
			if limiter != "" {
				failure += "; limiting device: " + limiter
			}
			limiterKey := limiter
			if fields := strings.Fields(limiter); len(fields) != 0 {
				limiterKey = fields[0]
			}
			recordFailure("headroom\x00"+limiterKey, failure)
			continue
		}
		if !found || margin > best.Margin+mnaPivotTolerance || math.Abs(margin-best.Margin) <= mnaPivotTolerance && math.Abs(candidate) < math.Abs(best.ValueV) {
			best.ValueV = normalizedMNAFloat(candidate)
			best.Margin = normalizedMNAFloat(margin)
			found = true
		}
	}
	if !found {
		message := fmt.Sprintf("no feasible centered operating point exists across the bounded %.12g V input range", rail)
		if len(failures) != 0 {
			message += "; candidate failures: " + strings.Join(failures, "; ")
		}
		if finite(bestRejected.Margin) {
			message += fmt.Sprintf("; best rejected source %.12g V had normalized margin %.12g", bestRejected.ValueV, bestRejected.Margin)
			if bestRejectedLimiter != "" {
				message += " at " + bestRejectedLimiter
			}
		}
		return CenteredBiasSelection{}, []Diagnostic{{
			Path:       "analyses.excitations." + sourceComponent,
			Message:    message,
			Suggestion: "select an architecture or catalog component with compatible input/output headroom and protection limits, or declare a supported external bias condition",
		}}
	}
	return best, nil
}

func centeredBiasCandidateMagnitudes(rail float64) []float64 {
	candidates := make([]float64, 0, centeredBiasIntervals+1+centeredBiasRailRefinements*2)
	seen := make(map[float64]bool, cap(candidates))
	appendCandidate := func(value float64) {
		value = normalizedMNAFloat(value)
		if value < 0 || value > rail || seen[value] {
			return
		}
		seen[value] = true
		candidates = append(candidates, value)
	}
	for index := 0; index <= centeredBiasIntervals; index++ {
		appendCandidate(rail * float64(index) / centeredBiasIntervals)
	}
	for refinement := 1; refinement <= centeredBiasRailRefinements; refinement++ {
		delta := rail / math.Exp2(float64(refinement))
		appendCandidate(delta)
		appendCandidate(rail - delta)
	}
	slices.Sort(candidates)
	return candidates
}

func sourceNodePolarity(plan Plan, component, node string) (float64, bool) {
	device, exists := resolvedDeviceByComponent(plan.Devices, component)
	if !exists {
		return 0, false
	}
	terminals := terminalMap(device)
	switch device.PrimitiveModel {
	case PrimitiveVoltageSourceV1:
		if terminals["POSITIVE"] == node {
			return 1, true
		}
		if terminals["NEGATIVE"] == node {
			return -1, true
		}
	case PrimitiveConnectorVoltageSourceV1:
		if terminals["PIN_1"] == node {
			return 1, true
		}
		if terminals["PIN_2"] == node {
			return -1, true
		}
	}
	return 0, false
}

func independentRailMagnitude(plan Plan, excluded string) (float64, bool) {
	maximum := 0.0
	for _, analysis := range plan.Analyses {
		for _, excitation := range analysis.Excitations {
			if excitation.Component == excluded {
				continue
			}
			device, exists := resolvedDeviceByComponent(plan.Devices, excitation.Component)
			if !exists || device.PrimitiveModel != PrimitiveVoltageSourceV1 && device.PrimitiveModel != PrimitiveConnectorVoltageSourceV1 {
				continue
			}
			maximum = math.Max(maximum, math.Abs(excitation.DCValue))
		}
	}
	return maximum, maximum > 0 && finite(maximum)
}

func centeredBiasAnalysis(plan Plan) (Analysis, bool) {
	for _, analysis := range plan.Analyses {
		if analysis.DCSweep == nil {
			return analysis, true
		}
	}
	return Analysis{}, false
}

func solveCenteredBiasPoint(plan Plan, analysis Analysis) (AnalysisPoint, []Diagnostic) {
	point, _, diagnostics := solveCenteredBiasPointWarm(plan, analysis, centeredBiasWarmState{})
	return point, diagnostics
}

func solveCenteredBiasPointWarm(plan Plan, analysis Analysis, warmState centeredBiasWarmState) (AnalysisPoint, centeredBiasWarmState, []Diagnostic) {
	analysisPlan := planWithAnalysisOverrides(plan, analysis)
	if len(compileNonlinearDevices(analysisPlan)) != 0 {
		system, solution, evidence, activeDeviceStates, diagnostic := solveNonlinearDCFromWarmState(analysisPlan, analysis, warmState.activeDeviceStates, warmState.solution)
		if diagnostic != nil {
			return AnalysisPoint{}, centeredBiasWarmState{}, []Diagnostic{*diagnostic}
		}
		nextWarmState := centeredBiasWarmState{
			activeDeviceStates: cloneOpAmpClamps(activeDeviceStates),
			solution:           append([]complex128(nil), solution...),
		}
		if diagnostics := validateNonlinearOperatingLimits(analysisPlan, system, solution); len(diagnostics) != 0 {
			return AnalysisPoint{}, nextWarmState, diagnostics
		}
		return AnalysisPoint{Nodes: nodeResults(analysisPlan, system, solution), Devices: electricalDeviceResults(analysisPlan, analysis, 0, system, solution), Solver: &evidence}, nextWarmState, nil
	}
	system, diagnostics := buildMNASystem(analysisPlan, analysis, 0)
	if len(diagnostics) != 0 {
		return AnalysisPoint{}, centeredBiasWarmState{}, diagnostics
	}
	solution, diagnostic := solveMNA(system)
	if diagnostic != nil {
		return AnalysisPoint{}, centeredBiasWarmState{}, []Diagnostic{*diagnostic}
	}
	system, solution, diagnostics = solveBoundedOpAmpDC(analysisPlan, analysis, system, solution)
	if len(diagnostics) != 0 {
		return AnalysisPoint{}, centeredBiasWarmState{}, diagnostics
	}
	if diagnostics = validateResolvedOperatingLimits(analysisPlan, system, solution, false); len(diagnostics) != 0 {
		return AnalysisPoint{}, centeredBiasWarmState{}, diagnostics
	}
	return AnalysisPoint{Nodes: nodeResults(analysisPlan, system, solution), Devices: electricalDeviceResults(analysisPlan, analysis, 0, system, solution)}, centeredBiasWarmState{}, nil
}

func centeredBiasMargin(plan Plan, point AnalysisPoint) (float64, bool, string) {
	nodes := make(map[string]float64, len(point.Nodes))
	for _, node := range point.Nodes {
		nodes[node.Node] = node.Real
	}
	devices := make(map[string]DeviceResult, len(point.Devices))
	for _, device := range point.Devices {
		devices[device.Component] = device
	}
	margin := math.Inf(1)
	constrained := false
	limiter := ""
	record := func(component string, candidate float64) {
		if !constrained || candidate < margin {
			margin = candidate
			limiter = component
		}
		constrained = true
	}
	for _, device := range plan.Devices {
		parameters := namedValueMap(device.ModelParameters)
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveOpAmpV1:
			negative, positive, output := nodes[terminals["V_MINUS"]], nodes[terminals["V_PLUS"]], nodes[terminals["OUT"]]
			span := positive - negative
			if span <= 0 {
				return math.Inf(-1), true, device.Component
			}
			low := (output - (negative + parameters["output_low_margin_v"])) / span
			high := ((positive - parameters["output_high_margin_v"]) - output) / span
			record(fmt.Sprintf("%s (output %.12g V, allowed %.12g..%.12g V)", device.Component, output, negative+parameters["output_low_margin_v"], positive-parameters["output_high_margin_v"]), math.Min(low, high))
		case PrimitiveFuseClosedStateV1:
			result, exists := devices[device.Component]
			if !exists {
				continue
			}
			currentMargin := (parameters["rated_current_a"] - result.CurrentMagnitudeA) / parameters["rated_current_a"]
			voltageMargin := (parameters["max_voltage_v"] - math.Abs(result.VoltageV)) / parameters["max_voltage_v"]
			record(device.Component, math.Min(currentMargin, voltageMargin))
		case PrimitiveBidirectionalTVSV1:
			result, exists := devices[device.Component]
			if !exists {
				continue
			}
			breakdown := parameters["breakdown_voltage_v"]
			record(device.Component, (breakdown-math.Abs(result.VoltageV))/breakdown)
		}
	}
	return margin, constrained, limiter
}
