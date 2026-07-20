package simmodel

import "fmt"

const (
	dcSweepForward = "forward"
	dcSweepReverse = "reverse"
)

func solveDCSweepAnalysis(plan Plan, analysis Analysis, nonlinear bool) (AnalysisResult, []Diagnostic) {
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
	var clamps map[string]float64
	for _, pass := range passes {
		for _, value := range pass.values {
			pointAnalysis := analysisWithDCSweepValue(analysis, sweep.Component, value)
			if nonlinear {
				system, solution, evidence, next, diagnostic := solveNonlinearDCFromState(plan, pointAnalysis, clamps)
				if diagnostic != nil {
					diagnostic.Path = fmt.Sprintf("analyses.%s.dc_sweep.%s.%.12g.%s", analysis.ID, pass.direction, value, diagnostic.Path)
					return AnalysisResult{}, []Diagnostic{*diagnostic}
				}
				clamps = next
				if diagnostics := validateNonlinearOperatingLimits(plan, system, solution); len(diagnostics) != 0 {
					return AnalysisResult{}, diagnostics
				}
				result.Points = append(result.Points, AnalysisPoint{SweepValue: normalizedMNAFloat(value), Sweep: pass.direction, Nodes: nodeResults(plan, system, solution), Devices: electricalDeviceResults(plan, pointAnalysis, 0, system, solution), Solver: &evidence})
				continue
			}

			system, diagnostics := buildMNASystem(plan, pointAnalysis, 0)
			if len(diagnostics) != 0 {
				return AnalysisResult{}, diagnostics
			}
			solution, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Path = fmt.Sprintf("analyses.%s.dc_sweep.%s.%.12g.%s", analysis.ID, pass.direction, value, diagnostic.Path)
				return AnalysisResult{}, []Diagnostic{*diagnostic}
			}
			system, solution, clamps, diagnostics = solveBoundedOpAmpDCFromState(plan, pointAnalysis, system, solution, clamps)
			if len(diagnostics) != 0 {
				return AnalysisResult{}, diagnostics
			}
			result.Points = append(result.Points, AnalysisPoint{SweepValue: normalizedMNAFloat(value), Sweep: pass.direction, Nodes: nodeResults(plan, system, solution), Devices: electricalDeviceResults(plan, pointAnalysis, 0, system, solution)})
		}
	}
	return result, nil
}

func dcSweepValues(sweep DCSweep) []float64 {
	values := make([]float64, sweep.Points)
	span := sweep.StopValue - sweep.StartValue
	for index := range values {
		values[index] = normalizedMNAFloat(sweep.StartValue + span*float64(index)/float64(sweep.Points-1))
	}
	return values
}

func analysisWithDCSweepValue(source Analysis, component string, value float64) Analysis {
	analysis := source
	analysis.DCSweep = nil
	analysis.Excitations = append([]SourceExcitation(nil), source.Excitations...)
	for index := range analysis.Excitations {
		if analysis.Excitations[index].Component == component {
			analysis.Excitations[index].DCValue = value
			break
		}
	}
	return analysis
}
