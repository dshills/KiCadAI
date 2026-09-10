package simmodel

import "fmt"

// Version-isolated copies retain the historical analysis dispatch and sweep
// ordering. Only the linear DC sweep invokes the frozen V23 clamp recovery.
func evaluateMNAWithSolverV23(plan Plan, report Report) (Report, []Diagnostic) {
	model, _ := definitionByID(plan.ModelID)
	analysisResults := make([]AnalysisResult, 0, len(plan.Analyses))
	if model.Transient {
		for _, evaluation := range evaluateTransientMNAAnalyses(plan, model.NonlinearDC) {
			if len(evaluation.diagnostics) != 0 {
				return report, evaluation.diagnostics
			}
			analysisResults = append(analysisResults, evaluation.result)
		}
		report.Analyses = analysisResults
		return evaluateMNAAssertions(plan, report)
	}
	for _, analysis := range plan.Analyses {
		analysisPlan := planWithAnalysisOverrides(plan, analysis)
		if !model.NonlinearDC && analysis.DCSweep == nil && len(compileNonlinearDevices(analysisPlan)) == 0 {
			if diagnostics := validateOpAmpStability(analysisPlan, analysis); len(diagnostics) != 0 {
				return report, diagnostics
			}
		}
		switch analysis.Kind {
		case AnalysisThermal:
			result, diagnostics := solveThermalAnalysis(analysisPlan, analysis)
			if len(diagnostics) != 0 {
				return report, diagnostics
			}
			analysisResults = append(analysisResults, result)
			continue
		case AnalysisNoise:
			result, diagnostics := solveNoiseAnalysis(analysisPlan, analysis)
			if len(diagnostics) != 0 {
				return report, diagnostics
			}
			analysisResults = append(analysisResults, result)
			continue
		case AnalysisStability:
			result, diagnostics := solveStabilityAnalysis(analysisPlan, analysis)
			if len(diagnostics) != 0 {
				return report, diagnostics
			}
			analysisResults = append(analysisResults, result)
			continue
		}
		if analysis.Kind == AnalysisDCOperatingPoint && analysis.DCSweep != nil {
			result, diagnostics := solveDCSweepAnalysisV23(analysisPlan, analysis, model.NonlinearDC)
			if len(diagnostics) != 0 {
				return report, diagnostics
			}
			analysisResults = append(analysisResults, result)
			continue
		}
		frequencies := []float64{0}
		if analysis.Kind == AnalysisACSweep {
			frequencies = sweepFrequencies(analysis)
		}
		var operatingPoint *smallSignalOperatingPoint
		if !model.NonlinearDC && smallSignalAnalysis(analysis.Kind) &&
			len(compileNonlinearDevices(analysisPlan)) != 0 {
			var diagnostics []Diagnostic
			operatingPoint, diagnostics = prepareSmallSignalOperatingPoint(analysisPlan, analysis)
			if len(diagnostics) != 0 {
				return report, diagnostics
			}
		}
		result := AnalysisResult{ID: analysis.ID, Kind: analysis.Kind, Points: make([]AnalysisPoint, 0, len(frequencies))}
		for _, frequency := range frequencies {
			if model.NonlinearDC {
				system, solution, evidence, diagnostic := solveNonlinearDC(analysisPlan, analysis)
				if diagnostic != nil {
					diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
					return report, []Diagnostic{*diagnostic}
				}
				point := AnalysisPoint{Nodes: nodeResults(analysisPlan, system, solution), Devices: electricalDeviceResults(analysisPlan, analysis, 0, system, solution), Solver: &evidence}
				if diagnostics := validateNonlinearOperatingLimits(analysisPlan, system, solution); len(diagnostics) != 0 {
					return report, diagnostics
				}
				result.Points = append(result.Points, point)
				continue
			}
			var system mnaSystem
			var diagnostics []Diagnostic
			if operatingPoint != nil {
				system, diagnostics = buildMNASystemWithPreparedOperatingPoint(
					analysisPlan,
					analysis,
					frequency,
					operatingPoint,
					nil,
				)
			} else {
				system, diagnostics = buildMNASystem(analysisPlan, analysis, frequency)
			}
			if len(diagnostics) != 0 {
				return report, diagnostics
			}
			solution, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
				return report, []Diagnostic{*diagnostic}
			}
			point := AnalysisPoint{FrequencyHz: frequency, Nodes: nodeResults(analysisPlan, system, solution), Devices: electricalDeviceResults(analysisPlan, analysis, frequency, system, solution)}
			if analysis.Kind == AnalysisDCOperatingPoint {
				system, solution, diagnostics = solveBoundedOpAmpDC(analysisPlan, analysis, system, solution)
				if len(diagnostics) != 0 {
					return report, diagnostics
				}
				if diagnostics = validateResolvedOperatingLimits(analysisPlan, system, solution, false); len(diagnostics) != 0 {
					return report, diagnostics
				}
				point.Nodes = nodeResults(analysisPlan, system, solution)
				point.Devices = electricalDeviceResults(analysisPlan, analysis, frequency, system, solution)
			}
			result.Points = append(result.Points, point)
		}
		analysisResults = append(analysisResults, result)
	}
	report.Analyses = analysisResults
	return evaluateMNAAssertions(plan, report)
}

func solveDCSweepAnalysisV23(plan Plan, analysis Analysis, nonlinear bool) (AnalysisResult, []Diagnostic) {
	if nonlinear {
		return solveDCSweepAnalysis(plan, analysis, true)
	}
	sweep := analysis.DCSweep
	if sweep == nil {
		return AnalysisResult{}, []Diagnostic{{Path: "analyses." + analysis.ID + ".dc_sweep", Message: "bounded DC sweep configuration is missing"}}
	}
	values := dcSweepValues(*sweep)
	passes := []struct {
		direction string
		values    []float64
	}{{direction: dcSweepForward, values: values}}
	if sweep.Bidirectional {
		reverse := append([]float64(nil), values...)
		for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
			reverse[left], reverse[right] = reverse[right], reverse[left]
		}
		passes = append(passes, struct {
			direction string
			values    []float64
		}{direction: dcSweepReverse, values: reverse})
	}

	result := AnalysisResult{ID: analysis.ID, Kind: analysis.Kind, Points: make([]AnalysisPoint, 0, sweep.Points*len(passes))}
	pointAnalysis := analysis
	pointAnalysis.DCSweep = nil
	pointAnalysis.Excitations = append([]SourceExcitation(nil), analysis.Excitations...)
	pointAnalysis.DeviceOverrides = append([]DeviceOverride(nil), analysis.DeviceOverrides...)
	excitationIndex, deviceOverrideIndex := -1, -1
	deviceValue := 0.0
	if sweep.DeviceValue {
		for index := range pointAnalysis.DeviceOverrides {
			if pointAnalysis.DeviceOverrides[index].Component == sweep.Component {
				deviceOverrideIndex = index
				break
			}
		}
		if deviceOverrideIndex < 0 {
			pointAnalysis.DeviceOverrides = append(pointAnalysis.DeviceOverrides, DeviceOverride{Component: sweep.Component})
			deviceOverrideIndex = len(pointAnalysis.DeviceOverrides) - 1
		}
		pointAnalysis.DeviceOverrides[deviceOverrideIndex].ValueSI = &deviceValue
	} else {
		for index := range pointAnalysis.Excitations {
			if pointAnalysis.Excitations[index].Component == sweep.Component {
				excitationIndex = index
				break
			}
		}
	}
	var clamps map[string]float64
	for _, pass := range passes {
		for _, value := range pass.values {
			pointPlan := plan
			if sweep.DeviceValue {
				deviceValue = value
				pointPlan = planWithAnalysisOverrides(plan, pointAnalysis)
			} else if excitationIndex >= 0 {
				pointAnalysis.Excitations[excitationIndex].DCValue = value * dcSweepExcitationScale(*sweep)
			}
			system, diagnostics := buildMNASystem(pointPlan, pointAnalysis, 0)
			if len(diagnostics) != 0 {
				return AnalysisResult{}, diagnostics
			}
			solution, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Path = fmt.Sprintf("analyses.%s.dc_sweep.%s.%.12g.%s", analysis.ID, pass.direction, value, diagnostic.Path)
				return AnalysisResult{}, []Diagnostic{*diagnostic}
			}
			var recovery activeStateRecoveryV23
			system, solution, clamps, recovery, diagnostics = solveBoundedOpAmpDCFromStateV23(pointPlan, pointAnalysis, system, solution, clamps)
			if len(diagnostics) != 0 {
				return AnalysisResult{}, diagnostics
			}
			method := "bounded_opamp_active_set_v23"
			if recovery.accepted {
				method += "_clamp_release"
			}
			result.Points = append(result.Points, AnalysisPoint{SweepValue: normalizedMNAFloat(value), Sweep: pass.direction, Nodes: nodeResults(pointPlan, system, solution), Devices: electricalDeviceResults(pointPlan, pointAnalysis, 0, system, solution), Solver: &SolverEvidence{Method: method}})
		}
	}
	return result, nil
}
