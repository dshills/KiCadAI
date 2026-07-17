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
	if diagnostics := validateTransientOperatingLimits(plan, system, solution); len(diagnostics) != 0 {
		return result, prefixTransientDiagnostics(analysis.ID, 0, 0, diagnostics)
	}
	result.Points = append(result.Points, AnalysisPoint{TimeS: 0, Nodes: nodeResults(plan, system, solution), Solver: &initialEvidence})

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
		if diagnostics := prepareTransientBase(&base, template, plan, analysis, timeS, solution); len(diagnostics) != 0 {
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
		if diagnostics := validateTransientOperatingLimits(plan, system, solution); len(diagnostics) != 0 {
			return result, prefixTransientDiagnostics(analysis.ID, step, timeS, diagnostics)
		}
		result.Points = append(result.Points, AnalysisPoint{TimeS: normalizedMNAFloat(timeS), Nodes: nodeResults(plan, system, solution), Solver: &evidence})
	}
	return result, nil
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
	}
	return dc
}

func transientSourceValue(excitation SourceExcitation, timeS, timeStepS float64) float64 {
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

func prepareTransientBase(base *mnaSystem, template mnaSystem, plan Plan, analysis Analysis, timeS float64, previous []complex128) []Diagnostic {
	resetMNASystem(base, template)
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
		}
	}
	if diagnostic := validateMNASystemBounds(*base); diagnostic != nil {
		return []Diagnostic{*diagnostic}
	}
	return nil
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

func validateTransientOperatingLimits(plan Plan, system mnaSystem, solution []complex128) []Diagnostic {
	diagnostics := validateNonlinearOperatingLimits(plan, system, solution)
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveCapacitorTransientV1 {
			continue
		}
		terminals := terminalMap(device)
		voltage := nonlinearNodeVoltage(&system, solution, terminals["A"]) - nonlinearNodeVoltage(&system, solution, terminals["B"])
		limit := namedValueMap(device.ModelParameters)["max_voltage_v"]
		if math.Abs(voltage) > limit {
			diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".operating_limit", Message: fmt.Sprintf("capacitor voltage %.12g V exceeds catalog-backed limit %.12g V", math.Abs(voltage), limit), Suggestion: "reduce applied voltage or select a suitably rated reviewed capacitor"})
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
