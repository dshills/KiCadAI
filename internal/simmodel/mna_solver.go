package simmodel

import (
	"fmt"
	"math"
	"math/cmplx"
	"runtime"
	"slices"
	"strings"
	"sync"
)

const (
	maxMNAUnknowns              = 256
	maxOpAmpActiveSetIterations = 32
	maxMNAMatrixValue           = 1e15
	maxMNASolutionValue         = 1e12
	mnaPivotTolerance           = 1e-12
	mnaResidualTolerance        = 1e-8
	maxMNAAnalysisWorkers       = 8
	mnaUnobservedReferenceS     = 1e-9
)

type mnaSystem struct {
	matrix           [][]complex128
	rhs              []complex128
	unknownLabels    []string
	nodeIndex        map[string]int
	branchIndex      map[string]int
	multiBranchIndex map[mnaBranchKey]int
}

type mnaBranchKey struct {
	component string
	terminal  string
}

type mnaSolveScratch struct {
	matrix        [][]complex128
	matrixStorage []complex128
	rhs           []complex128
	scales        []float64
}

var mnaSolveScratchPool = sync.Pool{New: func() any { return new(mnaSolveScratch) }}

func evaluateMNA(plan Plan, report Report) (Report, []Diagnostic) {
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
			result, diagnostics := solveDCSweepAnalysis(analysisPlan, analysis, model.NonlinearDC)
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
			system, diagnostics := buildMNASystem(analysisPlan, analysis, frequency)
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

type mnaAnalysisEvaluation struct {
	result      AnalysisResult
	diagnostics []Diagnostic
}

func evaluateTransientMNAAnalyses(plan Plan, nonlinearDC bool) []mnaAnalysisEvaluation {
	evaluations := make([]mnaAnalysisEvaluation, len(plan.Analyses))
	if len(plan.Analyses) == 0 {
		return evaluations
	}
	workerCount := min(len(plan.Analyses), maxMNAAnalysisWorkers, max(1, runtime.GOMAXPROCS(0)))
	jobs := make(chan int)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for index := range jobs {
				analysis := plan.Analyses[index]
				analysisPlan := planWithAnalysisOverrides(plan, analysis)
				if !nonlinearDC && len(compileNonlinearDevices(analysisPlan)) == 0 {
					if diagnostics := validateOpAmpStability(analysisPlan, analysis); len(diagnostics) != 0 {
						evaluations[index].diagnostics = diagnostics
						continue
					}
				}
				switch analysis.Kind {
				case AnalysisThermal:
					evaluations[index].result, evaluations[index].diagnostics = solveThermalAnalysis(analysisPlan, analysis)
				case AnalysisStartup:
					evaluations[index].result, evaluations[index].diagnostics = solveStartupAnalysis(analysisPlan, analysis)
				case AnalysisDistortion:
					evaluations[index].result, evaluations[index].diagnostics = solveDistortionAnalysis(analysisPlan, analysis)
				default:
					evaluations[index].result, evaluations[index].diagnostics = solveTransientAnalysis(analysisPlan, analysis)
				}
			}
		}()
	}
	for index := range plan.Analyses {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return evaluations
}

func electricalDeviceResults(plan Plan, analysis Analysis, frequency float64, system mnaSystem, solution []complex128) []DeviceResult {
	return electricalDeviceResultsWithComparatorStates(plan, analysis, frequency, system, solution, nil)
}

func electricalDeviceResultsWithComparatorStates(plan Plan, analysis Analysis, frequency float64, system mnaSystem, solution []complex128, comparatorStates map[string]float64) []DeviceResult {
	results := make([]DeviceResult, 0, len(plan.Devices))
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		var voltage, current complex128
		include := true
		switch device.PrimitiveModel {
		case PrimitiveResistorV1:
			voltage = solvedNodeVoltage(system, solution, terminals["A"]) - solvedNodeVoltage(system, solution, terminals["B"])
			current = voltage / complex(*device.ValueSI, 0)
		case PrimitiveFuseClosedStateV1:
			voltage = solvedNodeVoltage(system, solution, terminals["A"]) - solvedNodeVoltage(system, solution, terminals["B"])
			current = voltage / complex(namedValueMap(device.ModelParameters)["cold_resistance_ohm"], 0)
		case PrimitiveRelayClosedV1, PrimitiveRelayNormallyOpenV1:
			voltage = solvedNodeVoltage(system, solution, terminals["CONTACT_IN"]) - solvedNodeVoltage(system, solution, terminals["CONTACT_OUT"])
			current = voltage / complex(namedValueMap(device.ModelParameters)["contact_on_resistance_ohm"], 0)
		case PrimitiveCapacitorV1:
			voltage = solvedNodeVoltage(system, solution, terminals["A"]) - solvedNodeVoltage(system, solution, terminals["B"])
			if analysis.Kind == AnalysisACSweep {
				current = voltage * complex(0, 2*math.Pi*frequency**device.ValueSI)
			}
		case PrimitiveVoltageSourceV1:
			voltage = solvedNodeVoltage(system, solution, terminals["POSITIVE"]) - solvedNodeVoltage(system, solution, terminals["NEGATIVE"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveConnectorVoltageSourceV1:
			voltage = solvedNodeVoltage(system, solution, terminals["PIN_1"]) - solvedNodeVoltage(system, solution, terminals["PIN_2"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveCurrentSourceV1:
			voltage = solvedNodeVoltage(system, solution, terminals["POSITIVE"]) - solvedNodeVoltage(system, solution, terminals["NEGATIVE"])
			current = excitationValue(analysis, device.Component)
		case PrimitiveOpAmpV1:
			voltage = solvedNodeVoltage(system, solution, terminals["OUT"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveCurrentSenseAmplifierV1:
			voltage = solvedNodeVoltage(system, solution, terminals["OUT"]) - solvedNodeVoltage(system, solution, terminals["GND_A"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveComparatorOpenCollectorV1:
			voltage = solvedNodeVoltage(system, solution, terminals["OUT"]) - solvedNodeVoltage(system, solution, terminals["V_MINUS"])
			parameters := namedValueMap(device.ModelParameters)
			resistance := parameters["output_off_resistance_ohm"]
			on := comparatorOn(device, system, solution)
			if state, exists := comparatorStates[device.Component]; exists {
				on = state >= .5
			}
			if on {
				resistance = parameters["output_on_resistance_ohm"]
			}
			current = voltage / complex(resistance, 0)
		case PrimitiveAdjustableLinearRegulatorV1:
			voltage = solvedNodeVoltage(system, solution, terminals["VOUT"]) - solvedNodeVoltage(system, solution, terminals["GND"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveFixedLinearRegulatorV1:
			voltage = solvedNodeVoltage(system, solution, terminals["VOUT"]) - solvedNodeVoltage(system, solution, terminals["GND"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveFloatingAdjustableRegulatorV1:
			voltage = solvedNodeVoltage(system, solution, terminals["VOUT"]) - solvedNodeVoltage(system, solution, terminals["ADJ"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveProgrammableCurrentSourceV1:
			voltage = solvedNodeVoltage(system, solution, terminals["IN"]) - solvedNodeVoltage(system, solution, terminals["OUT"])
			reference := namedValueMap(device.ModelParameters)["reference_current_a"]
			current = solution[system.branchIndex[device.Component]] - complex(reference, 0)
		case PrimitiveShuntVoltageReferenceV1:
			voltage = solvedNodeVoltage(system, solution, terminals["CATHODE"]) - solvedNodeVoltage(system, solution, terminals["ANODE"])
			current = solution[system.branchIndex[device.Component]]
		case PrimitiveDualOutputIsolatedConverterV1:
			positiveVoltage := solvedNodeVoltage(system, solution, terminals["VOUT_PLUS"]) - solvedNodeVoltage(system, solution, terminals["COMMON"])
			negativeVoltage := solvedNodeVoltage(system, solution, terminals["VOUT_MINUS"]) - solvedNodeVoltage(system, solution, terminals["COMMON"])
			positiveCurrent := solution[system.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_PLUS"}]]
			negativeCurrent := solution[system.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_MINUS"}]]
			voltage, current = positiveVoltage, positiveCurrent
			if cmplx.Abs(negativeVoltage) > cmplx.Abs(positiveVoltage) {
				voltage = negativeVoltage
			}
			if cmplx.Abs(negativeCurrent) > cmplx.Abs(positiveCurrent) {
				current = negativeCurrent
			}
		case PrimitiveBidirectionalOpenDrainTranslatorV1:
			parameters := namedValueMap(device.ModelParameters)
			compiled := compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: parameters}
			maximumVoltage, maximumCurrent := 0.0, 0.0
			for channel := 1; channel <= 2; channel++ {
				a := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("A%d", channel)]))
				b := real(solvedNodeVoltage(system, solution, terminals[fmt.Sprintf("B%d", channel)]))
				resistance := translatorChannelResistance(compiled, &system, solution, channel)
				if delta := a - b; math.Abs(delta) > math.Abs(maximumVoltage) {
					maximumVoltage = delta
				}
				maximumCurrent = math.Max(maximumCurrent, math.Abs((a-b)/resistance))
			}
			voltage, current = complex(maximumVoltage, 0), complex(maximumCurrent, 0)
		case PrimitiveDiodeShockleyV1:
			voltage = solvedNodeVoltage(system, solution, terminals["ANODE"]) - solvedNodeVoltage(system, solution, terminals["CATHODE"])
			value, _ := diodeCurrentAndGradient(real(voltage), namedValueMap(device.ModelParameters))
			current = complex(value, 0)
		case PrimitiveUnidirectionalZenerV1:
			voltage = solvedNodeVoltage(system, solution, terminals["ANODE"]) - solvedNodeVoltage(system, solution, terminals["CATHODE"])
			value, _ := unidirectionalZenerCurrentAndGradient(real(voltage), namedValueMap(device.ModelParameters))
			current = complex(value, 0)
		case PrimitiveBidirectionalTVSV1:
			voltage = solvedNodeVoltage(system, solution, terminals["ANODE"]) - solvedNodeVoltage(system, solution, terminals["CATHODE"])
			value, _ := bidirectionalTVSCurrentAndGradient(real(voltage), namedValueMap(device.ModelParameters))
			current = complex(value, 0)
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			voltage = solvedNodeVoltage(system, solution, terminals["DRAIN"]) - solvedNodeVoltage(system, solution, terminals["SOURCE"])
			conductance := mosfetSwitchConductance(compiledNonlinearDevice{primitive: device.PrimitiveModel, terminals: terminals, parameters: namedValueMap(device.ModelParameters), polarity: mosfetPolarity(device.PrimitiveModel)}, &system, solution)
			current = voltage * complex(conductance, 0)
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			polarity := 1.0
			if device.PrimitiveModel == PrimitiveBJTPNPV1 {
				polarity = -1
			}
			vb := real(solvedNodeVoltage(system, solution, terminals["BASE"]))
			vc := real(solvedNodeVoltage(system, solution, terminals["COLLECTOR"]))
			ve := real(solvedNodeVoltage(system, solution, terminals["EMITTER"]))
			currents, _ := bjtCurrentsAndJacobian(vb, vc, ve, namedValueMap(device.ModelParameters), polarity)
			voltage, current = complex(vc-ve, 0), complex(currents[1], 0)
		default:
			include = false
		}
		if include {
			entry := DeviceResult{Component: device.Component, VoltageV: normalizedMNAFloat(real(voltage)), CurrentA: normalizedMNAFloat(real(current)), CurrentMagnitudeA: normalizedMNAFloat(cmplx.Abs(current))}
			if analysis.Kind == AnalysisThermal {
				if dissipation, dissipative := thermalDeviceDissipation(device, system, solution); dissipative {
					entry.DissipationW = normalizedMNAFloat(dissipation)
				}
			}
			results = append(results, entry)
		}
	}
	return results
}

func planWithAnalysisOverrides(plan Plan, analysis Analysis) Plan {
	if len(analysis.DeviceOverrides) == 0 {
		return plan
	}
	clone := ClonePlan(plan)
	for _, override := range analysis.DeviceOverrides {
		for index := range clone.Devices {
			if clone.Devices[index].Component == override.Component {
				clone.Devices[index] = applyDeviceOverride(clone.Devices[index], override)
				break
			}
		}
	}
	return clone
}

func evaluateMNAAssertions(plan Plan, report Report) (Report, []Diagnostic) {
	var diagnostics []Diagnostic
	for _, assertion := range plan.Assertions {
		actual, diagnostic := assertionValue(report.Analyses, assertion)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			// Preserve positional correspondence with plan.Assertions even when a
			// derived measurement cannot be formed. Higher-level closed-loop
			// evidence links assertion indices, so omission would alias every
			// subsequent result and could turn a failed measurement into another
			// requirement's evidence.
			failedActual := assertion.Min - math.Max(1, math.Abs(assertion.Min))*1e-6
			report.Assertions = append(report.Assertions, AssertionResult{
				AnalysisID: assertion.AnalysisID, Node: assertion.Node, Component: assertion.Component, Components: append([]string(nil), assertion.Components...), ReferenceNode: assertion.ReferenceNode, Quantity: assertion.Quantity, FrequencyHz: assertion.FrequencyHz, TimeS: assertion.TimeS,
				Min: assertion.Min, Max: assertion.Max, Actual: normalizedMNAFloat(failedActual), Pass: false,
			})
			continue
		}
		pass := actual >= assertion.Min && actual <= assertion.Max
		report.Assertions = append(report.Assertions, AssertionResult{
			AnalysisID: assertion.AnalysisID, Node: assertion.Node, Component: assertion.Component, Components: append([]string(nil), assertion.Components...), ReferenceNode: assertion.ReferenceNode, Quantity: assertion.Quantity, FrequencyHz: assertion.FrequencyHz, TimeS: assertion.TimeS,
			Min: assertion.Min, Max: assertion.Max, Actual: actual, Pass: pass,
		})
		if !pass {
			scope := assertion.Node
			if assertion.Component != "" {
				scope = assertion.Component
			} else if len(assertion.Components) != 0 {
				scope = strings.Join(assertion.Components, ",")
			}
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "assertions." + assertion.AnalysisID + "." + scope + "." + assertion.Quantity,
				Message:    fmt.Sprintf("measured %.12g is outside trusted bounds %.12g..%.12g", actual, assertion.Min, assertion.Max),
				Suggestion: "adjust catalog-backed component values, connectivity, or bounded analysis conditions",
			})
		}
	}
	if len(diagnostics) == 0 {
		report.Status = "pass"
	}
	return report, diagnostics
}

func buildMNASystem(plan Plan, analysis Analysis, frequency float64) (mnaSystem, []Diagnostic) {
	return buildMNASystemWithOperatingPoint(plan, analysis, frequency, nil, false)
}

func buildMNASystemWithForcedOpAmp(plan Plan, analysis Analysis, frequency float64, forcedOpAmp string) (mnaSystem, []Diagnostic) {
	clamps := map[string]float64{}
	if forcedOpAmp != "" {
		clamps[forcedOpAmp] = 1
	}
	return buildMNASystemWithOperatingPoint(plan, analysis, frequency, clamps, true)
}

func buildMNASystemWithOperatingPoint(plan Plan, analysis Analysis, frequency float64, forcedStates map[string]float64, zeroSmallSignalSources bool) (mnaSystem, []Diagnostic) {
	smallSignal := analysis
	if zeroSmallSignalSources {
		smallSignal.Excitations = append([]SourceExcitation(nil), analysis.Excitations...)
		for index := range smallSignal.Excitations {
			smallSignal.Excitations[index].ACMagnitude = 0
			smallSignal.Excitations[index].ACPhaseDeg = 0
		}
	}
	if !smallSignalAnalysis(analysis.Kind) || len(compileNonlinearDevices(plan)) == 0 {
		return buildMNASystemWithOpAmpClamps(plan, smallSignal, frequency, forcedStates)
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
	dcSystem, dcSolution, _, diagnostic := solveNonlinearDC(plan, dc)
	if diagnostic != nil {
		diagnostic.Path = "operating_point." + diagnostic.Path
		return mnaSystem{}, []Diagnostic{*diagnostic}
	}
	if diagnostics := validateNonlinearOperatingLimits(plan, dcSystem, dcSolution); len(diagnostics) != 0 {
		return mnaSystem{}, diagnostics
	}
	states, _, stateDiagnostic := resolvedActiveDeviceStates(plan, dcSystem, dcSolution)
	if stateDiagnostic != nil {
		return mnaSystem{}, []Diagnostic{*stateDiagnostic}
	}
	for component, state := range forcedStates {
		states[component] = state
	}
	system, diagnostics := buildMNASystemWithOpAmpClamps(plan, smallSignal, frequency, states)
	if len(diagnostics) != 0 {
		return mnaSystem{}, diagnostics
	}
	stampSmallSignalNonlinearDevices(&system, compileNonlinearDevices(plan), &dcSystem, dcSolution, frequency)
	if diagnostic := validateMNASystemBounds(system); diagnostic != nil {
		return mnaSystem{}, []Diagnostic{*diagnostic}
	}
	return system, nil
}

func buildMNASystemWithOpAmpClamps(plan Plan, analysis Analysis, frequency float64, opAmpClamps map[string]float64) (mnaSystem, []Diagnostic) {
	nodeIndex := make(map[string]int, len(plan.Nodes)-1)
	labels := make([]string, 0, len(plan.Nodes)+len(plan.Devices))
	for _, node := range plan.Nodes {
		if node == plan.GroundNode {
			continue
		}
		nodeIndex[node] = len(labels)
		labels = append(labels, "node:"+node)
	}
	branchIndex := map[string]int{}
	multiBranchIndex := map[mnaBranchKey]int{}
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveVoltageSourceV1 || device.PrimitiveModel == PrimitiveConnectorVoltageSourceV1 || device.PrimitiveModel == PrimitiveOpAmpV1 || device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1 || device.PrimitiveModel == PrimitiveAdjustableLinearRegulatorV1 || device.PrimitiveModel == PrimitiveFixedLinearRegulatorV1 || device.PrimitiveModel == PrimitiveFloatingAdjustableRegulatorV1 || device.PrimitiveModel == PrimitiveProgrammableCurrentSourceV1 || device.PrimitiveModel == PrimitiveShuntVoltageReferenceV1 {
			branchIndex[device.Component] = len(labels)
			labels = append(labels, "branch_current:"+device.Component)
		}
		if device.PrimitiveModel == PrimitiveDualOutputIsolatedConverterV1 {
			for _, terminal := range []string{"VOUT_PLUS", "VOUT_MINUS"} {
				multiBranchIndex[mnaBranchKey{component: device.Component, terminal: terminal}] = len(labels)
				labels = append(labels, "branch_current:"+device.Component+":"+terminal)
			}
		}
	}
	if len(labels) == 0 || len(labels) > maxMNAUnknowns {
		return mnaSystem{}, []Diagnostic{{Path: "topology", Message: fmt.Sprintf("MNA system requires 1..%d unknowns, got %d", maxMNAUnknowns, len(labels))}}
	}
	matrix := make([][]complex128, len(labels))
	for index := range matrix {
		matrix[index] = make([]complex128, len(labels))
	}
	system := mnaSystem{matrix: matrix, rhs: make([]complex128, len(labels)), unknownLabels: labels, nodeIndex: nodeIndex, branchIndex: branchIndex, multiBranchIndex: multiBranchIndex}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveResistorV1:
			conductance := complex(1 / *device.ValueSI, 0)
			stampAdmittance(&system, terminals["A"], terminals["B"], conductance)
		case PrimitiveFuseClosedStateV1:
			conductance := complex(1/namedValueMap(device.ModelParameters)["cold_resistance_ohm"], 0)
			stampAdmittance(&system, terminals["A"], terminals["B"], conductance)
		case PrimitiveRelayClosedV1, PrimitiveRelayNormallyOpenV1:
			parameters := namedValueMap(device.ModelParameters)
			stampAdmittance(&system, terminals["COIL_A"], terminals["COIL_B"], complex(1/parameters["coil_resistance_ohm"], 0))
			stampAdmittance(&system, terminals["CONTACT_IN"], terminals["CONTACT_OUT"], complex(1/parameters["contact_on_resistance_ohm"], 0))
		case PrimitiveCapacitorV1:
			if smallSignalAnalysis(analysis.Kind) {
				stampAdmittance(&system, terminals["A"], terminals["B"], complex(0, 2*math.Pi*frequency**device.ValueSI))
			}
		case PrimitiveCapacitorTransientV1:
			// The trusted transient solver stamps the fixed backward-Euler companion.
		case PrimitiveVoltageSourceV1:
			value := excitationValue(analysis, device.Component)
			stampVoltageSource(&system, device.Component, terminals["POSITIVE"], terminals["NEGATIVE"], value)
		case PrimitiveConnectorVoltageSourceV1:
			value := excitationValue(analysis, device.Component)
			stampVoltageSource(&system, device.Component, terminals["PIN_1"], terminals["PIN_2"], value)
		case PrimitiveCurrentSourceV1:
			value := excitationValue(analysis, device.Component)
			stampCurrentSource(&system, terminals["POSITIVE"], terminals["NEGATIVE"], value)
		case PrimitiveMCUStaticSupplyLoadV1, PrimitiveSensorStaticSupplyLoadV1:
			if !smallSignalAnalysis(analysis.Kind) {
				current := namedValueMap(device.ModelParameters)["maximum_supply_current_a"]
				stampCurrentSource(&system, terminals["POWER"], terminals["GROUND"], complex(current, 0))
			}
		case PrimitiveOpAmpV1:
			if value, clamped := opAmpClamps[device.Component]; clamped {
				stampVoltageSource(&system, device.Component, terminals["OUT"], plan.GroundNode, complex(value, 0))
				continue
			}
			parameters := namedValueMap(device.ModelParameters)
			gain := complex(parameters["dc_open_loop_gain"], 0)
			if smallSignalAnalysis(analysis.Kind) {
				pole := parameters["gain_bandwidth_hz"] / parameters["dc_open_loop_gain"]
				gain /= complex(1, frequency/pole)
			}
			stampOpAmp(&system, device.Component, terminals, gain)
		case PrimitiveCurrentSenseAmplifierV1:
			parameters := namedValueMap(device.ModelParameters)
			if value, clamped := opAmpClamps[device.Component]; clamped {
				stampVoltageSource(&system, device.Component, terminals["OUT"], terminals["GND_A"], complex(value, 0))
				if !smallSignalAnalysis(analysis.Kind) {
					stampCurrentSource(&system, terminals["VCC"], terminals["GND_A"], complex(parameters["quiescent_current_a"], 0))
				}
				continue
			}
			gain := complex(parameters["gain_v_per_v"], 0)
			offset := parameters["input_offset_voltage_v"]
			if smallSignalAnalysis(analysis.Kind) {
				gain /= complex(1, frequency/(parameters["bandwidth_hz"]/parameters["gain_v_per_v"]))
				offset = 0
			}
			stampCurrentSenseAmplifier(&system, device.Component, terminals, gain, complex(offset, 0))
			if !smallSignalAnalysis(analysis.Kind) {
				stampCurrentSource(&system, terminals["VCC"], terminals["GND_A"], complex(parameters["quiescent_current_a"], 0))
			}
		case PrimitiveComparatorOpenCollectorV1:
			parameters := namedValueMap(device.ModelParameters)
			resistance := parameters["output_off_resistance_ohm"]
			if value, decided := opAmpClamps[device.Component]; decided && value >= .5 {
				resistance = parameters["output_on_resistance_ohm"]
			}
			stampAdmittance(&system, terminals["OUT"], terminals["V_MINUS"], complex(1/resistance, 0))
		case PrimitiveAdjustableLinearRegulatorV1:
			parameters := namedValueMap(device.ModelParameters)
			reference := parameters["reference_voltage_v"]
			quiescent := parameters["quiescent_current_a"]
			if smallSignalAnalysis(analysis.Kind) {
				reference = 0
				quiescent = 0
			}
			stampAdjustableLinearRegulator(&system, device.Component, terminals, complex(reference, 0))
			stampCurrentSource(&system, terminals["VIN"], terminals["GND"], complex(quiescent, 0))
		case PrimitiveFixedLinearRegulatorV1:
			parameters := namedValueMap(device.ModelParameters)
			output := parameters["output_voltage_v"]
			quiescent := parameters["quiescent_current_a"]
			if smallSignalAnalysis(analysis.Kind) {
				output = 0
				quiescent = 0
			}
			stampFixedLinearRegulator(&system, device.Component, terminals, complex(output, 0))
			stampCurrentSource(&system, terminals["VIN"], terminals["GND"], complex(quiescent, 0))
		case PrimitiveFloatingAdjustableRegulatorV1:
			parameters := namedValueMap(device.ModelParameters)
			reference := parameters["polarity"] * parameters["reference_voltage_v"]
			adjustmentCurrent := parameters["polarity"] * parameters["adjustment_pin_current_a"]
			if smallSignalAnalysis(analysis.Kind) {
				reference = 0
				adjustmentCurrent = 0
			}
			stampFloatingAdjustableRegulator(&system, device.Component, terminals, complex(reference, 0))
			stampCurrentSource(&system, terminals["VIN"], terminals["ADJ"], complex(adjustmentCurrent, 0))
		case PrimitiveProgrammableCurrentSourceV1:
			parameters := namedValueMap(device.ModelParameters)
			reference := parameters["reference_current_a"]
			offset := parameters["offset_voltage_v"]
			if smallSignalAnalysis(analysis.Kind) {
				reference = 0
				offset = 0
			}
			stampProgrammableCurrentSource(&system, device.Component, terminals, complex(offset, 0))
			stampCurrentSource(&system, terminals["IN"], terminals["SET"], complex(reference, 0))
		case PrimitiveShuntVoltageReferenceV1:
			output := namedValueMap(device.ModelParameters)["output_voltage_v"]
			if smallSignalAnalysis(analysis.Kind) {
				output = 0
			}
			stampVoltageSource(&system, device.Component, terminals["CATHODE"], terminals["ANODE"], complex(output, 0))
		case PrimitiveDualOutputIsolatedConverterV1:
			parameters := namedValueMap(device.ModelParameters)
			positive, negative := parameters["positive_output_voltage_v"], -parameters["negative_output_voltage_v"]
			if smallSignalAnalysis(analysis.Kind) {
				positive, negative = 0, 0
			}
			stampVoltageSourceBranch(&system, system.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_PLUS"}], terminals["VOUT_PLUS"], terminals["COMMON"], complex(positive, 0))
			stampVoltageSourceBranch(&system, system.multiBranchIndex[mnaBranchKey{component: device.Component, terminal: "VOUT_MINUS"}], terminals["VOUT_MINUS"], terminals["COMMON"], complex(negative, 0))
		case PrimitiveBidirectionalOpenDrainTranslatorV1:
			parameters := namedValueMap(device.ModelParameters)
			if !smallSignalAnalysis(analysis.Kind) {
				stampCurrentSource(&system, terminals["VCCA"], terminals["GND"], complex(parameters["vcca_quiescent_current_a"], 0))
				stampCurrentSource(&system, terminals["VCCB"], terminals["GND"], complex(parameters["vccb_quiescent_current_a"], 0))
			}
		case PrimitiveDiodeShockleyV1, PrimitiveUnidirectionalZenerV1, PrimitiveBidirectionalTVSV1, PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1, PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			// Nonlinear devices are stamped by the bounded DC Newton solver.
		default:
			return mnaSystem{}, []Diagnostic{{Path: "devices." + device.Component, Message: "resolved primitive has no trusted MNA stamp"}}
		}
	}
	if smallSignalAnalysis(analysis.Kind) {
		referenceUnobservedMNAComponents(plan, analysis, &system)
	}
	if diagnostic := validateMNASystemBounds(system); diagnostic != nil {
		return mnaSystem{}, []Diagnostic{*diagnostic}
	}
	return system, nil
}

func referenceUnobservedMNAComponents(plan Plan, analysis Analysis, system *mnaSystem) {
	observed := map[int]bool{}
	for _, assertion := range plan.Assertions {
		if assertion.AnalysisID != analysis.ID {
			continue
		}
		if index, exists := system.nodeIndex[assertion.Node]; exists {
			observed[index] = true
		}
		if index, exists := system.branchIndex[assertion.Component]; exists {
			observed[index] = true
		}
		for _, component := range assertion.Components {
			if index, exists := system.branchIndex[component]; exists {
				observed[index] = true
			}
		}
	}
	if len(observed) == 0 {
		return
	}
	visited := make([]bool, len(system.matrix))
	for start := range system.matrix {
		if visited[start] {
			continue
		}
		component := []int{start}
		visited[start] = true
		hasObservation := observed[start]
		for cursor := 0; cursor < len(component); cursor++ {
			row := component[cursor]
			for column := range system.matrix {
				if visited[column] || system.matrix[row][column] == 0 && system.matrix[column][row] == 0 {
					continue
				}
				visited[column] = true
				hasObservation = hasObservation || observed[column]
				component = append(component, column)
			}
		}
		if hasObservation {
			continue
		}
		for _, index := range component {
			if !strings.HasPrefix(system.unknownLabels[index], "node:") {
				continue
			}
			// This algebraic component cannot affect a requested measurement.
			// A single deterministic reference fixes its otherwise arbitrary
			// common mode without loading the observable signal graph.
			system.matrix[index][index] += complex(mnaUnobservedReferenceS, 0)
			break
		}
	}
}

func validateMNASystemBounds(system mnaSystem) *Diagnostic {
	for row := range system.matrix {
		for column := range system.matrix[row] {
			if !boundedComplex(system.matrix[row][column], maxMNAMatrixValue) {
				return &Diagnostic{Path: fmt.Sprintf("matrix[%d][%d]", row, column), Message: "trusted MNA stamp produced a non-finite or unbounded matrix coefficient", Suggestion: "reduce source or component dynamic range, or select catalog models appropriate for the operating range"}
			}
		}
		if !boundedComplex(system.rhs[row], maxMNAMatrixValue) {
			return &Diagnostic{Path: fmt.Sprintf("rhs[%d]", row), Message: "trusted stamp produced a non-finite or unbounded right-hand side", Suggestion: "reduce source or component dynamic range, or select catalog models appropriate for the operating range"}
		}
	}
	return nil
}

// validateOpAmpStability derives each low-frequency feedback factor from the
// compiled linear graph. It forces a one-volt output perturbation with all
// independent sources zeroed, then measures the resulting differential input.
// For the trusted single-pole open-loop model, A0*beta >= 1 places the closed-
// loop pole in the right half-plane and must fail closed.
func validateOpAmpStability(plan Plan, operatingAnalysis Analysis) []Diagnostic {
	var diagnostics []Diagnostic
	loopAnalysis := operatingAnalysis
	loopAnalysis.ID = operatingAnalysis.ID + "_stability_preflight"
	loopAnalysis.Kind = AnalysisStability
	loopAnalysis.StartFrequencyHz, loopAnalysis.StopFrequencyHz, loopAnalysis.Points = 0, 0, 0
	loopAnalysis.DurationS, loopAnalysis.TimeStepS, loopAnalysis.DCSweep = 0, 0, nil
	loopAnalysis.Excitations = append([]SourceExcitation(nil), operatingAnalysis.Excitations...)
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		system, systemDiagnostics := buildMNASystemWithForcedOpAmp(plan, loopAnalysis, 0, device.Component)
		if len(systemDiagnostics) != 0 {
			for _, diagnostic := range systemDiagnostics {
				diagnostic.Path = "devices." + device.Component + ".stability." + diagnostic.Path
				diagnostics = append(diagnostics, diagnostic)
			}
			continue
		}
		solution, diagnostic := solveMNA(system)
		if diagnostic != nil {
			diagnostic.Path = "devices." + device.Component + ".stability." + diagnostic.Path
			diagnostic.Message = "closed-loop stability analysis failed: " + diagnostic.Message
			diagnostics = append(diagnostics, *diagnostic)
			continue
		}
		terminals := terminalMap(device)
		plus := solvedNodeVoltage(system, solution, terminals["IN_PLUS"])
		minus := solvedNodeVoltage(system, solution, terminals["IN_MINUS"])
		beta := plus - minus
		if math.Abs(imag(beta)) > 1e-9 {
			diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".stability", Message: "low-frequency feedback factor is unexpectedly complex", Suggestion: "correct incompatible dynamic or dependent-source connectivity"})
			continue
		}
		gain := namedValueMap(device.ModelParameters)["dc_open_loop_gain"]
		loopGain := gain * real(beta)
		if !finite(loopGain) || loopGain >= 1-mnaPivotTolerance {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "devices." + device.Component + ".stability",
				Message:    fmt.Sprintf("catalog single-pole op-amp has unstable positive feedback (DC loop gain %.12g)", loopGain),
				Suggestion: "reverse the feedback polarity or provide catalog-supported stable compensation",
			})
		}
	}
	return diagnostics
}

func solvedNodeVoltage(system mnaSystem, solution []complex128, node string) complex128 {
	if index, exists := system.nodeIndex[node]; exists {
		return solution[index]
	}
	return 0
}

func solveMNA(system mnaSystem) ([]complex128, *Diagnostic) {
	size := len(system.rhs)
	scratch := mnaSolveScratchPool.Get().(*mnaSolveScratch)
	defer mnaSolveScratchPool.Put(scratch)
	if cap(scratch.matrix) < size {
		scratch.matrix = make([][]complex128, size)
	} else {
		scratch.matrix = scratch.matrix[:size]
	}
	if cap(scratch.matrixStorage) < size*size {
		scratch.matrixStorage = make([]complex128, size*size)
	} else {
		scratch.matrixStorage = scratch.matrixStorage[:size*size]
	}
	if cap(scratch.rhs) < size {
		scratch.rhs = make([]complex128, size)
	} else {
		scratch.rhs = scratch.rhs[:size]
	}
	if cap(scratch.scales) < size {
		scratch.scales = make([]float64, size)
	} else {
		scratch.scales = scratch.scales[:size]
		clear(scratch.scales)
	}
	matrix := scratch.matrix
	matrixStorage := scratch.matrixStorage
	for row := range matrix {
		matrix[row] = matrixStorage[row*size : (row+1)*size]
		copy(matrix[row], system.matrix[row])
	}
	rhs := scratch.rhs
	copy(rhs, system.rhs)
	scales := scratch.scales
	for row := range matrix {
		for _, coefficient := range matrix[row] {
			scales[row] = math.Max(scales[row], cmplx.Abs(coefficient))
		}
	}
	for column := 0; column < size; column++ {
		pivot := -1
		bestRatio := 0.0
		for row := column; row < size; row++ {
			ratio := 0.0
			if scales[row] > 0 {
				ratio = cmplx.Abs(matrix[row][column]) / scales[row]
			}
			if ratio > bestRatio {
				bestRatio = ratio
				pivot = row
			}
		}
		if pivot < 0 || bestRatio < mnaPivotTolerance {
			return nil, &Diagnostic{Path: "unknowns." + system.unknownLabels[column], Message: "MNA matrix is singular or numerically ill-conditioned at this unknown", Suggestion: "connect the floating node, add a catalog-backed DC path, verify source constraints, or correct incompatible feedback"}
		}
		if pivot != column {
			matrix[column], matrix[pivot] = matrix[pivot], matrix[column]
			rhs[column], rhs[pivot] = rhs[pivot], rhs[column]
			scales[column], scales[pivot] = scales[pivot], scales[column]
		}
		for row := column + 1; row < size; row++ {
			factor := matrix[row][column] / matrix[column][column]
			if factor == 0 {
				continue
			}
			matrix[row][column] = 0
			for index := column + 1; index < size; index++ {
				matrix[row][index] -= factor * matrix[column][index]
			}
			rhs[row] -= factor * rhs[column]
		}
	}
	solution := make([]complex128, size)
	for row := size - 1; row >= 0; row-- {
		value := rhs[row]
		for column := row + 1; column < size; column++ {
			value -= matrix[row][column] * solution[column]
		}
		solution[row] = value / matrix[row][row]
		if !boundedComplex(solution[row], maxMNASolutionValue) {
			return nil, &Diagnostic{Path: "unknowns." + system.unknownLabels[row], Message: "MNA solution is non-finite or exceeds the trusted numerical bound", Suggestion: "correct positive feedback, floating nodes, incompatible source constraints, or unrealistic catalog values"}
		}
	}
	maxResidual := 0.0
	matrixNorm := 0.0
	solutionNorm := 0.0
	rhsNorm := 0.0
	for row := 0; row < size; row++ {
		reconstructed := complex(0, 0)
		rowNorm := 0.0
		for column := 0; column < size; column++ {
			reconstructed += system.matrix[row][column] * solution[column]
			rowNorm += cmplx.Abs(system.matrix[row][column])
		}
		maxResidual = math.Max(maxResidual, cmplx.Abs(reconstructed-system.rhs[row]))
		matrixNorm = math.Max(matrixNorm, rowNorm)
		rhsNorm = math.Max(rhsNorm, cmplx.Abs(system.rhs[row]))
	}
	for _, value := range solution {
		solutionNorm = math.Max(solutionNorm, cmplx.Abs(value))
	}
	bound := mnaResidualTolerance * (matrixNorm*solutionNorm + rhsNorm + 1)
	if !finite(maxResidual) || maxResidual > bound {
		return nil, &Diagnostic{Path: "residual", Message: fmt.Sprintf("MNA residual %.12g exceeds deterministic bound %.12g", maxResidual, bound), Suggestion: "reduce component dynamic range or correct an ill-conditioned circuit"}
	}
	return solution, nil
}

func stampAdmittance(system *mnaSystem, positive, negative string, admittance complex128) {
	positiveIndex, positiveKnown := system.nodeIndex[positive]
	negativeIndex, negativeKnown := system.nodeIndex[negative]
	if positiveKnown {
		system.matrix[positiveIndex][positiveIndex] += admittance
	}
	if negativeKnown {
		system.matrix[negativeIndex][negativeIndex] += admittance
	}
	if positiveKnown && negativeKnown {
		system.matrix[positiveIndex][negativeIndex] -= admittance
		system.matrix[negativeIndex][positiveIndex] -= admittance
	}
}

func stampVoltageSource(system *mnaSystem, component, positive, negative string, value complex128) {
	stampVoltageSourceBranch(system, system.branchIndex[component], positive, negative, value)
}

func stampVoltageSourceBranch(system *mnaSystem, branch int, positive, negative string, value complex128) {
	stampBranchKCL(system, branch, positive, negative)
	if index, exists := system.nodeIndex[positive]; exists {
		system.matrix[branch][index] += 1
	}
	if index, exists := system.nodeIndex[negative]; exists {
		system.matrix[branch][index] -= 1
	}
	system.rhs[branch] += value
}

func stampCurrentSource(system *mnaSystem, positive, negative string, value complex128) {
	if index, exists := system.nodeIndex[positive]; exists {
		system.rhs[index] -= value
	}
	if index, exists := system.nodeIndex[negative]; exists {
		system.rhs[index] += value
	}
}

func stampOpAmp(system *mnaSystem, component string, terminals map[string]string, gain complex128) {
	branch := system.branchIndex[component]
	// The controlled source is ground-referenced; V_MINUS is a supply-limit
	// terminal, not the reference of the small-signal transfer equation.
	stampBranchKCL(system, branch, terminals["OUT"], "")
	if index, exists := system.nodeIndex[terminals["OUT"]]; exists {
		system.matrix[branch][index] += 1 / gain
	}
	if index, exists := system.nodeIndex[terminals["IN_PLUS"]]; exists {
		system.matrix[branch][index] -= 1
	}
	if index, exists := system.nodeIndex[terminals["IN_MINUS"]]; exists {
		system.matrix[branch][index] += 1
	}
}

func stampCurrentSenseAmplifier(system *mnaSystem, component string, terminals map[string]string, gain, offset complex128) {
	branch := system.branchIndex[component]
	stampBranchKCL(system, branch, terminals["OUT"], terminals["GND_A"])
	if index, exists := system.nodeIndex[terminals["OUT"]]; exists {
		system.matrix[branch][index] += 1 / gain
	}
	if index, exists := system.nodeIndex[terminals["GND_A"]]; exists {
		system.matrix[branch][index] -= 1 / gain
	}
	if index, exists := system.nodeIndex[terminals["IN_PLUS"]]; exists {
		system.matrix[branch][index] -= 1
	}
	if index, exists := system.nodeIndex[terminals["IN_MINUS"]]; exists {
		system.matrix[branch][index] += 1
	}
	for _, reference := range []string{"REF1", "REF2"} {
		if index, exists := system.nodeIndex[terminals[reference]]; exists {
			system.matrix[branch][index] -= .5 / gain
		}
	}
	system.rhs[branch] += offset
}

// stampAdjustableLinearRegulator models a bounded ideal error amplifier. The
// branch equation holds ADJ at the catalog reference while its branch current
// flows from VIN to VOUT, so the external feedback network determines VOUT and
// upstream source current includes the regulated load. Headroom, current, and
// input-voltage limits are checked against the solved operating point.
func stampAdjustableLinearRegulator(system *mnaSystem, component string, terminals map[string]string, reference complex128) {
	branch := system.branchIndex[component]
	stampBranchKCL(system, branch, terminals["VOUT"], terminals["VIN"])
	if index, exists := system.nodeIndex[terminals["ADJ"]]; exists {
		system.matrix[branch][index] += 1
	}
	if index, exists := system.nodeIndex[terminals["GND"]]; exists {
		system.matrix[branch][index] -= 1
	}
	system.rhs[branch] += reference
}

func stampFixedLinearRegulator(system *mnaSystem, component string, terminals map[string]string, output complex128) {
	branch := system.branchIndex[component]
	stampBranchKCL(system, branch, terminals["VOUT"], terminals["VIN"])
	if index, exists := system.nodeIndex[terminals["VOUT"]]; exists {
		system.matrix[branch][index] += 1
	}
	if index, exists := system.nodeIndex[terminals["GND"]]; exists {
		system.matrix[branch][index] -= 1
	}
	system.rhs[branch] += output
}

func stampFloatingAdjustableRegulator(system *mnaSystem, component string, terminals map[string]string, reference complex128) {
	branch := system.branchIndex[component]
	stampBranchKCL(system, branch, terminals["VOUT"], terminals["VIN"])
	if index, exists := system.nodeIndex[terminals["VOUT"]]; exists {
		system.matrix[branch][index] += 1
	}
	if index, exists := system.nodeIndex[terminals["ADJ"]]; exists {
		system.matrix[branch][index] -= 1
	}
	system.rhs[branch] += reference
}

// stampProgrammableCurrentSource models the power follower in a three-pin,
// two-terminal programmable current source. The catalog-backed reference
// current flows from IN through SET and its external resistor. This branch
// forces OUT to the same voltage as SET while supplying the additional current
// through the external OUT resistor. The physical two-terminal output is the
// shared far end of those two resistors, so their solved ratio determines the
// delivered current.
func stampProgrammableCurrentSource(system *mnaSystem, component string, terminals map[string]string, offset complex128) {
	branch := system.branchIndex[component]
	stampBranchKCL(system, branch, terminals["OUT"], terminals["IN"])
	if index, exists := system.nodeIndex[terminals["OUT"]]; exists {
		system.matrix[branch][index] += 1
	}
	if index, exists := system.nodeIndex[terminals["SET"]]; exists {
		system.matrix[branch][index] -= 1
	}
	system.rhs[branch] += offset
}

func stampBranchKCL(system *mnaSystem, branch int, positive, negative string) {
	if index, exists := system.nodeIndex[positive]; exists {
		system.matrix[index][branch] += 1
	}
	if index, exists := system.nodeIndex[negative]; exists {
		system.matrix[index][branch] -= 1
	}
}

func excitationValue(analysis Analysis, component string) complex128 {
	for _, excitation := range analysis.Excitations {
		if excitation.Component != component {
			continue
		}
		if analysis.Kind == AnalysisDCOperatingPoint {
			return complex(excitation.DCValue, 0)
		}
		phase := excitation.ACPhaseDeg * math.Pi / 180
		return cmplx.Rect(excitation.ACMagnitude, phase)
	}
	return 0
}

func smallSignalAnalysis(kind string) bool {
	switch kind {
	case AnalysisACSweep, AnalysisNoise, AnalysisStability:
		return true
	default:
		return false
	}
}

func terminalMap(device ResolvedDevice) map[string]string {
	terminals := make(map[string]string, len(device.Terminals))
	for _, terminal := range device.Terminals {
		terminals[terminal.Terminal] = terminal.Net
	}
	return terminals
}

func nodeResults(plan Plan, system mnaSystem, solution []complex128) []NodeResult {
	results := make([]NodeResult, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		value := complex(0, 0)
		if index, exists := system.nodeIndex[node]; exists {
			value = solution[index]
		}
		realPart := normalizedMNAFloat(real(value))
		imaginary := normalizedMNAFloat(imag(value))
		magnitude := normalizedMNAFloat(cmplx.Abs(value))
		phase := normalizedMNAFloat(cmplx.Phase(value) * 180 / math.Pi)
		results = append(results, NodeResult{Node: node, Real: realPart, Imaginary: imaginary, Magnitude: magnitude, PhaseDeg: phase})
	}
	return results
}

// solveBoundedOpAmpDC is a fail-closed active-set proof, not a general
// nonlinear approximation. Positive feedback is rejected by the preceding
// stability proof; repeated states, cycles, and iteration exhaustion return a
// structured diagnostic and can never be reported as a suboptimal solution.
func solveBoundedOpAmpDC(plan Plan, analysis Analysis, system mnaSystem, solution []complex128) (mnaSystem, []complex128, []Diagnostic) {
	system, solution, _, diagnostics := solveBoundedOpAmpDCFromState(plan, analysis, system, solution, nil)
	return system, solution, diagnostics
}

func solveBoundedOpAmpDCFromState(plan Plan, analysis Analysis, system mnaSystem, solution []complex128, initial map[string]float64) (mnaSystem, []complex128, map[string]float64, []Diagnostic) {
	opAmpCount := 0
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveOpAmpV1 || device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 || device.PrimitiveModel == PrimitiveCurrentSenseAmplifierV1 {
			opAmpCount++
		}
	}
	if opAmpCount == 0 {
		return system, solution, nil, nil
	}

	clamps := cloneOpAmpClamps(initial)
	if len(clamps) != 0 {
		var diagnostics []Diagnostic
		system, diagnostics = buildMNASystemWithOpAmpClamps(plan, analysis, 0, clamps)
		if len(diagnostics) != 0 {
			return system, nil, clamps, diagnostics
		}
		var diagnostic *Diagnostic
		solution, diagnostic = solveMNA(system)
		if diagnostic != nil {
			diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
			return system, nil, clamps, []Diagnostic{*diagnostic}
		}
	}
	seen := map[string]bool{}
	previousStates := ""
	iterationLimit := min(opAmpCount*6+2, maxOpAmpActiveSetIterations)
	for iteration := 0; iteration < iterationLimit; iteration++ {
		next := map[string]float64{}
		var statesBuilder strings.Builder
		statesBuilder.Grow(opAmpCount * 16)
		var diagnostics []Diagnostic
		for _, device := range plan.Devices {
			if device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 {
				parameters := namedValueMap(device.ModelParameters)
				terminals := terminalMap(device)
				negative := real(solvedNodeVoltage(system, solution, terminals["V_MINUS"]))
				positive := real(solvedNodeVoltage(system, solution, terminals["V_PLUS"]))
				supply := positive - negative
				if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
					diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog comparator"})
					continue
				}
				on := comparatorOn(device, system, solution)
				if on {
					next[device.Component] = 1
					statesBuilder.WriteString(device.Component + ":on;")
				} else {
					next[device.Component] = 0
					statesBuilder.WriteString(device.Component + ":off;")
				}
				continue
			}
			if device.PrimitiveModel != PrimitiveOpAmpV1 {
				if device.PrimitiveModel != PrimitiveCurrentSenseAmplifierV1 {
					continue
				}
				parameters := namedValueMap(device.ModelParameters)
				supply, minimum, maximum, desired := currentSenseOperatingState(device, system, solution)
				if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
					diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog current-sense amplifier"})
					continue
				}
				tolerance := mnaPivotTolerance * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
				switch {
				case desired < minimum-tolerance:
					next[device.Component] = minimum
					statesBuilder.WriteString(device.Component + ":low;")
				case desired > maximum+tolerance:
					next[device.Component] = maximum
					statesBuilder.WriteString(device.Component + ":high;")
				default:
					statesBuilder.WriteString(device.Component + ":linear;")
				}
				continue
			}
			terminals := terminalMap(device)
			parameters := namedValueMap(device.ModelParameters)
			negative := real(solvedNodeVoltage(system, solution, terminals["V_MINUS"]))
			positive := real(solvedNodeVoltage(system, solution, terminals["V_PLUS"]))
			supply := positive - negative
			if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
				diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog op-amp"})
				continue
			}
			minimum := negative + parameters["output_low_margin_v"]
			maximum := positive - parameters["output_high_margin_v"]
			differential := real(solvedNodeVoltage(system, solution, terminals["IN_PLUS"]) - solvedNodeVoltage(system, solution, terminals["IN_MINUS"]))
			desired := parameters["dc_open_loop_gain"] * differential
			tolerance := mnaPivotTolerance * math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum)))
			switch {
			case desired < minimum-tolerance:
				next[device.Component] = minimum
				statesBuilder.WriteString(device.Component)
				statesBuilder.WriteString(":low;")
			case desired > maximum+tolerance:
				next[device.Component] = maximum
				statesBuilder.WriteString(device.Component)
				statesBuilder.WriteString(":high;")
			default:
				statesBuilder.WriteString(device.Component)
				statesBuilder.WriteString(":linear;")
			}
		}
		states := statesBuilder.String()
		if len(diagnostics) != 0 {
			return system, solution, clamps, diagnostics
		}
		if sameOpAmpClamps(clamps, next) {
			return system, solution, clamps, nil
		}
		if states != previousStates && seen[states] {
			return system, solution, clamps, []Diagnostic{{Path: "devices", Message: "bounded op-amp operating-point states did not converge", Suggestion: "correct ambiguous positive feedback or add catalog-backed hysteresis and loading"}}
		}
		seen[states] = true
		previousStates = states
		clamps = next
		// Every active-set state changes dependent-source stamps and sometimes
		// the branch equations themselves, so the MNA system must be rebuilt.
		// The state search and full-system rebuild cost are deterministically
		// bounded by maxOpAmpActiveSetIterations and maxMNAUnknowns.
		var systemDiagnostics []Diagnostic
		system, systemDiagnostics = buildMNASystemWithOpAmpClamps(plan, analysis, 0, clamps)
		if len(systemDiagnostics) != 0 {
			return system, nil, clamps, systemDiagnostics
		}
		var diagnostic *Diagnostic
		solution, diagnostic = solveMNA(system)
		if diagnostic != nil {
			diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
			return system, nil, clamps, []Diagnostic{*diagnostic}
		}
	}
	return system, solution, clamps, []Diagnostic{{Path: "devices", Message: "bounded op-amp operating-point iteration exceeded its deterministic limit", Suggestion: "correct ambiguous positive feedback or reduce coupled comparator stages"}}
}

func comparatorOn(device ResolvedDevice, system mnaSystem, solution []complex128) bool {
	terminals := terminalMap(device)
	parameters := namedValueMap(device.ModelParameters)
	plus := real(solvedNodeVoltage(system, solution, terminals["IN_PLUS"]))
	minus := real(solvedNodeVoltage(system, solution, terminals["IN_MINUS"]))
	return plus-minus < parameters["input_offset_v"]
}

func currentSenseOperatingState(device ResolvedDevice, system mnaSystem, solution []complex128) (supply, minimum, maximum, desired float64) {
	terminals := terminalMap(device)
	parameters := namedValueMap(device.ModelParameters)
	ground := real(solvedNodeVoltage(system, solution, terminals["GND_A"]))
	supply = real(solvedNodeVoltage(system, solution, terminals["VCC"])) - ground
	minimum = parameters["output_low_margin_v"]
	maximum = supply - parameters["output_high_margin_v"]
	inputDifferential := real(solvedNodeVoltage(system, solution, terminals["IN_PLUS"]) - solvedNodeVoltage(system, solution, terminals["IN_MINUS"]))
	reference := .5*(real(solvedNodeVoltage(system, solution, terminals["REF1"]))+real(solvedNodeVoltage(system, solution, terminals["REF2"]))) - ground
	desired = reference + parameters["gain_v_per_v"]*(inputDifferential+parameters["input_offset_voltage_v"])
	return supply, minimum, maximum, desired
}

func cloneOpAmpClamps(source map[string]float64) map[string]float64 {
	clone := make(map[string]float64, len(source))
	for component, value := range source {
		clone[component] = value
	}
	return clone
}

func sameOpAmpClamps(left, right map[string]float64) bool {
	if len(left) != len(right) {
		return false
	}
	for component, value := range left {
		rightValue := right[component]
		tolerance := 1e-9 * math.Max(1, math.Max(math.Abs(value), math.Abs(rightValue)))
		if math.Abs(rightValue-value) > tolerance {
			return false
		}
	}
	return true
}

func assertionValue(results []AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	for _, result := range results {
		if result.ID != assertion.AnalysisID {
			continue
		}
		if result.Kind == AnalysisNoise && assertion.Quantity == QuantityIntegratedNoiseVRMS {
			return integratedNoise(result, assertion)
		}
		if result.Kind == AnalysisStability && (assertion.Quantity == QuantityPhaseMarginDeg || assertion.Quantity == QuantityGainMarginDB) {
			return stabilityMargin(result, assertion)
		}
		if result.Kind == AnalysisTransient && (assertion.Quantity == QuantityRiseTimeS || assertion.Quantity == QuantityFallTimeS) {
			return transientEdgeTime(result, assertion)
		}
		if result.Kind == AnalysisStartup && assertion.Quantity == QuantityPeakAbsVoltageV {
			return peakAbsVoltage(result, assertion)
		}
		if result.Kind == AnalysisDistortion && assertion.Quantity == QuantityTHDPercent {
			return totalHarmonicDistortion(result, assertion)
		}
		if result.Kind == AnalysisThermal && (assertion.Quantity == QuantityDeviceDissipationW || assertion.Quantity == QuantityJunctionTemperatureC) {
			return thermalAssertionValue(result, assertion)
		}
		if result.Kind == AnalysisACSweep && (assertion.Quantity == QuantityVoltageGainRatio || assertion.Quantity == QuantityCutoffFrequencyHz || assertion.Quantity == QuantityBandwidthHz) {
			return acDerivedValue(result, assertion)
		}
		if result.Kind == AnalysisTransient && (assertion.Quantity == QuantityPeakAbsVoltageV || assertion.Quantity == QuantityOutputSwingVPP || assertion.Quantity == QuantitySettlingTimeS || assertion.Quantity == QuantityResponseTimeS || assertion.Quantity == QuantityOutputPowerW) {
			return transientDerivedValue(result, assertion)
		}
		if result.Kind == AnalysisDCOperatingPoint && (assertion.Quantity == QuantityDeviceCurrentA || assertion.Quantity == QuantityTotalSupplyCurrentA || assertion.Quantity == QuantityTransimpedanceOhm) {
			return dcDeviceValue(result, assertion)
		}
		if result.Kind == AnalysisDCOperatingPoint && (assertion.Quantity == QuantityThresholdVoltageV || assertion.Quantity == QuantityThresholdCurrentA || assertion.Quantity == QuantityHysteresisVoltageV) {
			return dcSweepDerivedValue(result, assertion)
		}
		for _, point := range result.Points {
			if result.Kind == AnalysisACSweep && math.Abs(point.FrequencyHz-assertion.FrequencyHz) > math.Max(1, math.Abs(point.FrequencyHz))*1e-12 {
				continue
			}
			if result.Kind == AnalysisTransient && math.Abs(point.TimeS-assertion.TimeS) > math.Max(1, math.Abs(point.TimeS))*1e-12 {
				continue
			}
			for _, node := range point.Nodes {
				if node.Node != assertion.Node {
					continue
				}
				switch assertion.Quantity {
				case QuantityVoltageV:
					return node.Real, nil
				case QuantityVoltageMagnitudeV:
					return node.Magnitude, nil
				case QuantityVoltagePhaseDeg:
					return node.PhaseDeg, nil
				case QuantityVoltageDBV:
					if node.Magnitude <= 0 {
						return math.Inf(-1), &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node, Message: "dBV assertion is undefined for a zero-magnitude node"}
					}
					return 20 * math.Log10(node.Magnitude), nil
				}
			}
		}
	}
	return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node, Message: "structured assertion did not resolve to a solved analysis point"}
}

func transientEdgeTime(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	times := make([]float64, 0, len(result.Points))
	values := make([]float64, 0, len(result.Points))
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, point := range result.Points {
		for _, node := range point.Nodes {
			if node.Node == assertion.Node {
				times = append(times, point.TimeS)
				values = append(values, node.Real)
				minimum = math.Min(minimum, node.Real)
				maximum = math.Max(maximum, node.Real)
				break
			}
		}
	}
	if len(values) < 2 || !finite(minimum) || !finite(maximum) || maximum-minimum <= 1e-12 {
		return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node, Message: "trusted edge-time assertion requires a nonconstant solved waveform"}
	}
	// V1 intentionally defines thresholds from the global extrema of the
	// complete bounded analysis duration, making multi-pulse behavior explicit
	// and deterministic without provider-selected transition windows.
	low, high := minimum+.1*(maximum-minimum), minimum+.9*(maximum-minimum)
	first, second := low, high
	rising := assertion.Quantity == QuantityRiseTimeS
	if !rising {
		first, second = high, low
	}
	firstTime, foundFirst := 0.0, false
	for index := 1; index < len(values); index++ {
		if !foundFirst && crosses(values[index-1], values[index], first, rising) {
			firstTime = interpolateCrossing(times[index-1], times[index], values[index-1], values[index], first)
			foundFirst = true
		}
		if foundFirst && crosses(values[index-1], values[index], second, rising) {
			secondTime := interpolateCrossing(times[index-1], times[index], values[index-1], values[index], second)
			if secondTime >= firstTime {
				return normalizedMNAFloat(secondTime - firstTime), nil
			}
		}
	}
	return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Node, Message: "trusted waveform does not contain a complete 10%-90% " + assertion.Quantity, Suggestion: "extend the bounded duration or correct the catalog-backed switching circuit"}
}

func crosses(a, b, threshold float64, rising bool) bool {
	if rising {
		return (a < threshold && b >= threshold) || (a == threshold && b > threshold)
	}
	return (a > threshold && b <= threshold) || (a == threshold && b < threshold)
}

func interpolateCrossing(t0, t1, v0, v1, threshold float64) float64 {
	if math.Abs(v1-v0) < 1e-15 {
		return t1
	}
	return t0 + (threshold-v0)*(t1-t0)/(v1-v0)
}

func sweepFrequencies(analysis Analysis) []float64 {
	frequencies := make([]float64, analysis.Points)
	if analysis.Points == 0 {
		return frequencies
	}
	if analysis.Points == 1 || analysis.StartFrequencyHz == analysis.StopFrequencyHz {
		for index := range frequencies {
			frequencies[index] = analysis.StartFrequencyHz
		}
		return frequencies
	}
	start := math.Log10(analysis.StartFrequencyHz)
	stop := math.Log10(analysis.StopFrequencyHz)
	for index := range frequencies {
		fraction := float64(index) / float64(analysis.Points-1)
		frequencies[index] = math.Pow(10, start+(stop-start)*fraction)
	}
	frequencies[0] = analysis.StartFrequencyHz
	frequencies[len(frequencies)-1] = analysis.StopFrequencyHz
	return frequencies
}

func boundedComplex(value complex128, maximum float64) bool {
	return finite(real(value)) && finite(imag(value)) && cmplx.Abs(value) <= maximum
}

func normalizedMNAFloat(value float64) float64 {
	if math.Abs(value) < 1e-15 {
		return 0
	}
	return value
}

func sortedNodeNames(results []NodeResult) []string {
	names := make([]string, 0, len(results))
	for _, result := range results {
		names = append(names, result.Node)
	}
	slices.Sort(names)
	return names
}
