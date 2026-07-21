package simmodel

import (
	"fmt"
	"math"
	"math/cmplx"
)

const (
	boltzmannConstantJPerK = 1.380649e-23
	noiseReferenceK        = 300.0
	maxReportedMarginDB    = 300.0
)

// solveNoiseAnalysis computes uncorrelated resistor thermal noise and
// catalog-backed op-amp input-voltage noise on the trusted logarithmic grid.
// Independent sources are zeroed by validation, so they act only as their
// ideal small-signal impedances. No provider-supplied equation or spectrum is
// accepted by this evaluator.
func solveNoiseAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisNoise, Points: make([]AnalysisPoint, 0, analysis.Points)}
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		if value := namedValueMap(device.ModelParameters)["input_voltage_noise_density_v_sqrt_hz"]; !finite(value) || value <= 0 {
			return result, []Diagnostic{{
				Path:       "analyses." + analysis.ID + ".devices." + device.Component + ".input_voltage_noise_density_v_sqrt_hz",
				Message:    "noise analysis requires a positive catalog-backed op-amp input-voltage-noise density",
				Suggestion: "select a reviewed component model with noise evidence or omit noise-based promotion",
			}}
		}
	}

	for _, frequency := range sweepFrequencies(analysis) {
		base, diagnostics := buildMNASystem(plan, analysis, frequency)
		if len(diagnostics) != 0 {
			return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
		}
		baseSolution, baseDiagnostic := solveMNA(base)
		if baseDiagnostic != nil {
			baseDiagnostic.Path = "analyses." + analysis.ID + ".noise_baseline." + baseDiagnostic.Path
			baseDiagnostic.Message = "noise baseline solve failed: " + baseDiagnostic.Message
			return result, []Diagnostic{*baseDiagnostic}
		}
		powerSpectralDensity := make(map[string]float64, len(plan.Nodes))
		dominantSource := make(map[string]string, len(plan.Nodes))
		dominantDensity := make(map[string]float64, len(plan.Nodes))
		noiseSources := 0
		for _, device := range plan.Devices {
			system := cloneMNASystem(base)
			terminals := terminalMap(device)
			switch device.PrimitiveModel {
			case PrimitiveResistorV1:
				// Norton current-noise density sqrt(4 k T / R), A/sqrt(Hz).
				density := math.Sqrt(4 * boltzmannConstantJPerK * noiseReferenceK / *device.ValueSI)
				stampCurrentSource(&system, terminals["A"], terminals["B"], complex(density, 0))
			case PrimitiveOpAmpV1:
				parameters := namedValueMap(device.ModelParameters)
				density := parameters["input_voltage_noise_density_v_sqrt_hz"]
				// stampOpAmp writes Vout/Aol-(V+ - V-) = rhs. An
				// input-referred voltage-noise source therefore belongs directly
				// on the equation's right-hand side; the controlled source applies
				// the open-loop gain exactly once while feedback sets the closed-loop
				// noise gain.
				system.rhs[system.branchIndex[device.Component]] += complex(density, 0)
			default:
				continue
			}
			noiseSources++
			solution, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Path = "analyses." + analysis.ID + ".noise_sources." + device.Component + "." + diagnostic.Path
				diagnostic.Message = "noise transfer solve failed: " + diagnostic.Message
				return result, []Diagnostic{*diagnostic}
			}
			for _, node := range plan.Nodes {
				// Some reviewed compact devices have affine operating-point
				// terms in their linearized system. Noise is the incremental
				// response to one physical source, so remove the source-free
				// baseline before accumulating spectral density.
				incremental := solvedNodeVoltage(system, solution, node) - solvedNodeVoltage(base, baseSolution, node)
				magnitude := cmplx.Abs(incremental)
				powerSpectralDensity[node] += magnitude * magnitude
				if magnitude > dominantDensity[node] {
					dominantDensity[node] = magnitude
					dominantSource[node] = device.Component
				}
			}
		}
		if noiseSources == 0 {
			return result, []Diagnostic{{Path: "analyses." + analysis.ID, Message: "noise analysis resolved no trusted physical noise sources", Suggestion: "include a reviewed resistor or active-device noise model"}}
		}
		nodes := make([]NodeResult, 0, len(plan.Nodes))
		for _, node := range plan.Nodes {
			density := normalizedMNAFloat(math.Sqrt(powerSpectralDensity[node]))
			nodes = append(nodes, NodeResult{
				Node: node, Magnitude: density, DominantNoiseSource: dominantSource[node],
				DominantNoiseDensityVSqrtHz: normalizedMNAFloat(dominantDensity[node]),
			})
		}
		result.Points = append(result.Points, AnalysisPoint{FrequencyHz: frequency, Nodes: nodes})
	}
	return result, nil
}

// solveStabilityAnalysis breaks each catalog op-amp loop at its output and,
// when no op-amp exists, evaluates the local emitter-degeneration return ratio
// of catalog BJTs. Assertions derive phase and gain margins from these bounded
// trusted loop definitions; the provider cannot choose an equation or hidden
// loop-breaking node.
func solveStabilityAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisStability, Points: make([]AnalysisPoint, 0, analysis.Points)}
	opAmps := make([]ResolvedDevice, 0)
	outputs := map[string]string{}
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		output := terminalMap(device)["OUT"]
		if previous, exists := outputs[output]; exists {
			return result, []Diagnostic{{Path: "analyses." + analysis.ID + ".nodes." + output, Message: fmt.Sprintf("stability output is shared by op-amps %s and %s", previous, device.Component), Suggestion: "use a topology with an unambiguous catalog op-amp output loop"}}
		}
		outputs[output] = device.Component
		opAmps = append(opAmps, device)
	}
	if len(opAmps) == 0 {
		return solveBJTEmitterDegenerationStability(plan, analysis)
	}

	for _, frequency := range sweepFrequencies(analysis) {
		nodes := make([]NodeResult, 0, len(opAmps))
		for _, device := range opAmps {
			system, diagnostics := buildMNASystemWithForcedOpAmp(plan, analysis, frequency, device.Component)
			if len(diagnostics) != 0 {
				return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
			}
			solution, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Path = "analyses." + analysis.ID + ".devices." + device.Component + "." + diagnostic.Path
				diagnostic.Message = "loop-return solve failed: " + diagnostic.Message
				return result, []Diagnostic{*diagnostic}
			}
			terminals := terminalMap(device)
			beta := solvedNodeVoltage(system, solution, terminals["IN_PLUS"]) - solvedNodeVoltage(system, solution, terminals["IN_MINUS"])
			loop := -opAmpOpenLoopGain(namedValueMap(device.ModelParameters), frequency) * beta
			if !boundedComplex(loop, maxMNASolutionValue) {
				return result, []Diagnostic{{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: "trusted loop-return ratio is non-finite or exceeds the numerical bound"}}
			}
			nodes = append(nodes, NodeResult{
				Node: terminalMap(device)["OUT"], Real: normalizedMNAFloat(real(loop)), Imaginary: normalizedMNAFloat(imag(loop)),
				Magnitude: normalizedMNAFloat(cmplx.Abs(loop)), PhaseDeg: normalizedMNAFloat(cmplx.Phase(loop) * 180 / math.Pi),
			})
		}
		result.Points = append(result.Points, AnalysisPoint{FrequencyHz: frequency, Nodes: nodes})
	}
	return result, nil
}

func solveBJTEmitterDegenerationStability(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisStability, Points: make([]AnalysisPoint, 0, analysis.Points)}
	var transistors []ResolvedDevice
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveBJTNPNV1 && device.PrimitiveModel != PrimitiveBJTPNPV1 {
			continue
		}
		parameters := namedValueMap(device.ModelParameters)
		terminals := terminalMap(device)
		if terminals["EMITTER"] == plan.GroundNode || parameters["transition_frequency_hz"] <= 0 {
			continue
		}
		transistors = append(transistors, device)
	}
	if len(transistors) == 0 {
		return result, []Diagnostic{{Path: "analyses." + analysis.ID, Message: "stability analysis requires a trusted op-amp loop or a catalog BJT with emitter degeneration and transition-frequency evidence"}}
	}

	dc := analysis
	dc.ID = analysis.ID + "_operating_point"
	dc.Kind = AnalysisDCOperatingPoint
	dc.StartFrequencyHz, dc.StopFrequencyHz, dc.Points = 0, 0, 0
	dc.DurationS, dc.TimeStepS, dc.DCSweep = 0, 0, nil
	dc.Excitations = append([]SourceExcitation(nil), analysis.Excitations...)
	for index := range dc.Excitations {
		dc.Excitations[index].ACMagnitude = 0
		dc.Excitations[index].ACPhaseDeg = 0
	}
	dcSystem, operatingPoint, _, diagnostic := solveNonlinearDC(plan, dc)
	if diagnostic != nil {
		diagnostic.Path = "analyses." + analysis.ID + ".operating_point." + diagnostic.Path
		return result, []Diagnostic{*diagnostic}
	}
	if diagnostics := validateNonlinearOperatingLimits(plan, dcSystem, operatingPoint); len(diagnostics) != 0 {
		return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
	}
	compiled := compileNonlinearDevices(plan)
	for _, frequency := range sweepFrequencies(analysis) {
		nodes := make([]NodeResult, 0, len(transistors))
		for _, device := range transistors {
			system, diagnostics := buildMNASystemWithOpAmpClamps(plan, analysis, frequency, nil)
			if len(diagnostics) != 0 {
				return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
			}
			stampSmallSignalNonlinearDevicesExcept(&system, compiled, &dcSystem, operatingPoint, frequency, device.Component)
			terminals := terminalMap(device)
			stampCurrentSource(&system, plan.GroundNode, terminals["EMITTER"], 1)
			if diagnostic := validateMNASystemBounds(system); diagnostic != nil {
				return result, prefixAnalysisDiagnostics(analysis.ID, []Diagnostic{*diagnostic})
			}
			solution, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Path = "analyses." + analysis.ID + ".devices." + device.Component + "." + diagnostic.Path
				diagnostic.Message = "BJT emitter-loop return solve failed: " + diagnostic.Message
				return result, []Diagnostic{*diagnostic}
			}
			parameters := namedValueMap(device.ModelParameters)
			polarity := 1.0
			if device.PrimitiveModel == PrimitiveBJTPNPV1 {
				polarity = -1
			}
			_, jacobian := bjtCurrentsAndJacobian(
				nonlinearNodeVoltage(&dcSystem, operatingPoint, terminals["BASE"]),
				nonlinearNodeVoltage(&dcSystem, operatingPoint, terminals["COLLECTOR"]),
				nonlinearNodeVoltage(&dcSystem, operatingPoint, terminals["EMITTER"]),
				parameters, polarity,
			)
			emitterTransconductance := math.Abs(jacobian[2][0])
			beta := parameters["forward_beta"]
			betaPole := parameters["transition_frequency_hz"] / beta
			emitterImpedance := solvedNodeVoltage(system, solution, terminals["EMITTER"])
			rawLoop := complex(emitterTransconductance, 0) * emitterImpedance / complex(1, frequency/betaPole)
			loop := rawLoop / (1 + rawLoop/complex(beta, 0))
			if !boundedComplex(loop, maxMNASolutionValue) || cmplx.Abs(loop) <= mnaPivotTolerance {
				return result, []Diagnostic{{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: "trusted BJT emitter-degeneration return ratio is absent, non-finite, or exceeds the numerical bound"}}
			}
			nodes = append(nodes, NodeResult{
				Node: terminals["COLLECTOR"], Real: normalizedMNAFloat(real(loop)), Imaginary: normalizedMNAFloat(imag(loop)),
				Magnitude: normalizedMNAFloat(cmplx.Abs(loop)), PhaseDeg: normalizedMNAFloat(cmplx.Phase(loop) * 180 / math.Pi),
			})
		}
		result.Points = append(result.Points, AnalysisPoint{FrequencyHz: frequency, Nodes: nodes})
	}
	return result, nil
}

func opAmpOpenLoopGain(parameters map[string]float64, frequency float64) complex128 {
	gain := complex(parameters["dc_open_loop_gain"], 0)
	pole := parameters["gain_bandwidth_hz"] / parameters["dc_open_loop_gain"]
	return gain / complex(1, frequency/pole)
}

func prefixAnalysisDiagnostics(analysisID string, diagnostics []Diagnostic) []Diagnostic {
	for index := range diagnostics {
		diagnostics[index].Path = "analyses." + analysisID + "." + diagnostics[index].Path
	}
	return diagnostics
}

func integratedNoise(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	if len(result.Points) < 2 {
		return 0, advancedAssertionDiagnostic(assertion, "integrated noise requires at least two solved frequency points")
	}
	integral := 0.0
	previousFrequency, previousDensity, foundPrevious := 0.0, 0.0, false
	for _, point := range result.Points {
		density, found := analysisNodeMagnitude(point, assertion.Node)
		if !found || !finite(density) || density < 0 {
			return 0, advancedAssertionDiagnostic(assertion, "integrated-noise assertion did not resolve to a finite node-noise density")
		}
		if foundPrevious {
			integral += .5 * (previousDensity*previousDensity + density*density) * (point.FrequencyHz - previousFrequency)
		}
		previousFrequency, previousDensity, foundPrevious = point.FrequencyHz, density, true
	}
	if !finite(integral) || integral < 0 {
		return 0, advancedAssertionDiagnostic(assertion, "integrated node-noise power is non-finite")
	}
	return normalizedMNAFloat(math.Sqrt(integral)), nil
}

func stabilityMargin(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	frequencies, magnitudes, phases, diagnostic := loopSeries(result, assertion)
	if diagnostic != nil {
		return 0, diagnostic
	}
	if assertion.Quantity == QuantityPhaseMarginDeg {
		for index := 1; index < len(magnitudes); index++ {
			if magnitudes[index-1] >= 1 && magnitudes[index] <= 1 {
				fraction := logarithmicCrossingFraction(magnitudes[index-1], magnitudes[index], 1)
				phase := phases[index-1] + fraction*(phases[index]-phases[index-1])
				return normalizedMNAFloat(180 + phase), nil
			}
		}
		return 0, advancedAssertionDiagnostic(assertion, fmt.Sprintf("stability sweep %.12g..%.12g Hz does not bracket the unity loop-gain crossing", frequencies[0], frequencies[len(frequencies)-1]))
	}
	for index := 1; index < len(phases); index++ {
		if phases[index-1] > -180 && phases[index] <= -180 {
			fraction := linearCrossingFraction(phases[index-1], phases[index], -180)
			logMagnitude := math.Log(magnitudes[index-1]) + fraction*(math.Log(magnitudes[index])-math.Log(magnitudes[index-1]))
			margin := -20 * logMagnitude / math.Ln10
			return normalizedMNAFloat(math.Max(-maxReportedMarginDB, math.Min(maxReportedMarginDB, margin))), nil
		}
	}
	// A return ratio that never reaches -180 degrees on the complete bounded
	// sweep has at least this clipped margin in the trusted v1 report.
	return maxReportedMarginDB, nil
}

func loopSeries(result AnalysisResult, assertion Assertion) ([]float64, []float64, []float64, *Diagnostic) {
	if len(result.Points) < 2 {
		return nil, nil, nil, advancedAssertionDiagnostic(assertion, "stability margin requires at least two solved frequency points")
	}
	frequencies := make([]float64, 0, len(result.Points))
	magnitudes := make([]float64, 0, len(result.Points))
	phases := make([]float64, 0, len(result.Points))
	for _, point := range result.Points {
		var selected *NodeResult
		for index := range point.Nodes {
			if point.Nodes[index].Node == assertion.Node {
				selected = &point.Nodes[index]
				break
			}
		}
		if selected == nil || !finite(selected.Magnitude) || selected.Magnitude <= 0 || !finite(selected.PhaseDeg) {
			return nil, nil, nil, advancedAssertionDiagnostic(assertion, "stability assertion did not resolve to a finite positive loop-return ratio")
		}
		phase := selected.PhaseDeg
		if len(phases) != 0 {
			for phase-phases[len(phases)-1] > 180 {
				phase -= 360
			}
			for phase-phases[len(phases)-1] < -180 {
				phase += 360
			}
		}
		frequencies = append(frequencies, point.FrequencyHz)
		magnitudes = append(magnitudes, selected.Magnitude)
		phases = append(phases, phase)
	}
	return frequencies, magnitudes, phases, nil
}

func analysisNodeMagnitude(point AnalysisPoint, node string) (float64, bool) {
	for _, result := range point.Nodes {
		if result.Node == node {
			return result.Magnitude, true
		}
	}
	return 0, false
}

func logarithmicCrossingFraction(a, b, target float64) float64 {
	return linearCrossingFraction(math.Log(a), math.Log(b), math.Log(target))
}

func linearCrossingFraction(a, b, target float64) float64 {
	if a == b {
		return 0
	}
	return (target - a) / (b - a)
}

func advancedAssertionDiagnostic(assertion Assertion, message string) *Diagnostic {
	return &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node + "." + assertion.Quantity, Message: message}
}
