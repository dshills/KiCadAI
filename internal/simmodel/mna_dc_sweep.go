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
	var previousSolution []complex128
	for _, pass := range passes {
		for _, value := range pass.values {
			pointPlan := plan
			if sweep.DeviceValue {
				deviceValue = value
				pointPlan = planWithAnalysisOverrides(plan, pointAnalysis)
			} else if excitationIndex >= 0 {
				pointAnalysis.Excitations[excitationIndex].DCValue = value * dcSweepExcitationScale(*sweep)
			}
			if nonlinear {
				system, solution, evidence, next, diagnostic := solveNonlinearDCFromWarmState(pointPlan, pointAnalysis, clamps, previousSolution)
				if diagnostic != nil {
					diagnostic.Path = fmt.Sprintf("analyses.%s.dc_sweep.%s.%.12g.%s", analysis.ID, pass.direction, value, diagnostic.Path)
					return AnalysisResult{}, []Diagnostic{*diagnostic}
				}
				clamps = next
				previousSolution = append(previousSolution[:0], solution...)
				if diagnostics := validateNonlinearOperatingLimits(pointPlan, system, solution); len(diagnostics) != 0 {
					return AnalysisResult{}, diagnostics
				}
				result.Points = append(result.Points, AnalysisPoint{SweepValue: normalizedMNAFloat(value), Sweep: pass.direction, Nodes: nodeResults(pointPlan, system, solution), Devices: electricalDeviceResults(pointPlan, pointAnalysis, 0, system, solution), Solver: &evidence})
				continue
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
			system, solution, clamps, diagnostics = solveBoundedOpAmpDCFromState(pointPlan, pointAnalysis, system, solution, clamps)
			if len(diagnostics) != 0 {
				return AnalysisResult{}, diagnostics
			}
			result.Points = append(result.Points, AnalysisPoint{SweepValue: normalizedMNAFloat(value), Sweep: pass.direction, Nodes: nodeResults(pointPlan, system, solution), Devices: electricalDeviceResults(pointPlan, pointAnalysis, 0, system, solution)})
		}
	}
	return result, nil
}

func dcSweepExcitationScale(sweep DCSweep) float64 {
	if sweep.ExcitationScale == 0 {
		return 1
	}
	return sweep.ExcitationScale
}

func dcSweepValues(sweep DCSweep) []float64 {
	values := make([]float64, sweep.Points)
	span := sweep.StopValue - sweep.StartValue
	for index := range values {
		values[index] = normalizedMNAFloat(sweep.StartValue + span*float64(index)/float64(sweep.Points-1))
	}
	return values
}
