package simmodel

import (
	"fmt"
	"math"
	"math/cmplx"
	"slices"
)

const (
	maxMNAUnknowns       = 256
	maxMNAMatrixValue    = 1e15
	maxMNASolutionValue  = 1e12
	mnaPivotTolerance    = 1e-12
	mnaResidualTolerance = 1e-8
)

type mnaSystem struct {
	matrix        [][]complex128
	rhs           []complex128
	unknownLabels []string
	nodeIndex     map[string]int
	branchIndex   map[string]int
}

func evaluateMNA(plan Plan, report Report) (Report, []Diagnostic) {
	model, _ := definitionByID(plan.ModelID)
	if !model.NonlinearDC {
		if diagnostics := validateOpAmpStability(plan); len(diagnostics) != 0 {
			return report, diagnostics
		}
	}
	analysisResults := make([]AnalysisResult, 0, len(plan.Analyses))
	for _, analysis := range plan.Analyses {
		frequencies := []float64{0}
		if analysis.Kind == AnalysisACSweep {
			frequencies = sweepFrequencies(analysis)
		}
		result := AnalysisResult{ID: analysis.ID, Kind: analysis.Kind, Points: make([]AnalysisPoint, 0, len(frequencies))}
		for _, frequency := range frequencies {
			if model.NonlinearDC {
				system, solution, evidence, diagnostic := solveNonlinearDC(plan, analysis)
				if diagnostic != nil {
					diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
					return report, []Diagnostic{*diagnostic}
				}
				point := AnalysisPoint{Nodes: nodeResults(plan, system, solution), Solver: &evidence}
				if diagnostics := validateNonlinearOperatingLimits(plan, system, solution); len(diagnostics) != 0 {
					return report, diagnostics
				}
				result.Points = append(result.Points, point)
				continue
			}
			system, diagnostics := buildMNASystem(plan, analysis, frequency)
			if len(diagnostics) != 0 {
				return report, diagnostics
			}
			solution, diagnostic := solveMNA(system)
			if diagnostic != nil {
				diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
				return report, []Diagnostic{*diagnostic}
			}
			point := AnalysisPoint{FrequencyHz: frequency, Nodes: nodeResults(plan, system, solution)}
			if analysis.Kind == AnalysisDCOperatingPoint {
				if diagnostics := validateOpAmpOperatingPoints(plan, point); len(diagnostics) != 0 {
					return report, diagnostics
				}
			}
			result.Points = append(result.Points, point)
		}
		analysisResults = append(analysisResults, result)
	}
	report.Analyses = analysisResults
	var diagnostics []Diagnostic
	for _, assertion := range plan.Assertions {
		actual, diagnostic := assertionValue(report.Analyses, assertion)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			continue
		}
		pass := actual >= assertion.Min && actual <= assertion.Max
		report.Assertions = append(report.Assertions, AssertionResult{
			AnalysisID: assertion.AnalysisID, Node: assertion.Node, Quantity: assertion.Quantity, FrequencyHz: assertion.FrequencyHz,
			Min: assertion.Min, Max: assertion.Max, Actual: actual, Pass: pass,
		})
		if !pass {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "assertions." + assertion.AnalysisID + "." + assertion.Node + "." + assertion.Quantity,
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
	return buildMNASystemWithForcedOpAmp(plan, analysis, frequency, "")
}

func buildMNASystemWithForcedOpAmp(plan Plan, analysis Analysis, frequency float64, forcedOpAmp string) (mnaSystem, []Diagnostic) {
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
	for _, device := range plan.Devices {
		if device.PrimitiveModel == PrimitiveVoltageSourceV1 || device.PrimitiveModel == PrimitiveOpAmpV1 {
			branchIndex[device.Component] = len(labels)
			labels = append(labels, "branch_current:"+device.Component)
		}
	}
	if len(labels) == 0 || len(labels) > maxMNAUnknowns {
		return mnaSystem{}, []Diagnostic{{Path: "topology", Message: fmt.Sprintf("MNA system requires 1..%d unknowns, got %d", maxMNAUnknowns, len(labels))}}
	}
	matrix := make([][]complex128, len(labels))
	for index := range matrix {
		matrix[index] = make([]complex128, len(labels))
	}
	system := mnaSystem{matrix: matrix, rhs: make([]complex128, len(labels)), unknownLabels: labels, nodeIndex: nodeIndex, branchIndex: branchIndex}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveResistorV1:
			conductance := complex(1 / *device.ValueSI, 0)
			stampAdmittance(&system, terminals["A"], terminals["B"], conductance)
		case PrimitiveCapacitorV1:
			if analysis.Kind == AnalysisACSweep {
				stampAdmittance(&system, terminals["A"], terminals["B"], complex(0, 2*math.Pi*frequency**device.ValueSI))
			}
		case PrimitiveVoltageSourceV1:
			value := excitationValue(analysis, device.Component)
			stampVoltageSource(&system, device.Component, terminals["POSITIVE"], terminals["NEGATIVE"], value)
		case PrimitiveCurrentSourceV1:
			value := excitationValue(analysis, device.Component)
			stampCurrentSource(&system, terminals["POSITIVE"], terminals["NEGATIVE"], value)
		case PrimitiveOpAmpV1:
			if device.Component == forcedOpAmp {
				stampVoltageSource(&system, device.Component, terminals["OUT"], plan.GroundNode, 1)
				continue
			}
			parameters := namedValueMap(device.ModelParameters)
			gain := complex(parameters["dc_open_loop_gain"], 0)
			if analysis.Kind == AnalysisACSweep {
				pole := parameters["gain_bandwidth_hz"] / parameters["dc_open_loop_gain"]
				gain /= complex(1, frequency/pole)
			}
			stampOpAmp(&system, device.Component, terminals, gain)
		case PrimitiveDiodeShockleyV1, PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			// Nonlinear devices are stamped by the bounded DC Newton solver.
		default:
			return mnaSystem{}, []Diagnostic{{Path: "devices." + device.Component, Message: "resolved primitive has no trusted MNA stamp"}}
		}
	}
	if diagnostic := validateMNASystemBounds(system); diagnostic != nil {
		return mnaSystem{}, []Diagnostic{*diagnostic}
	}
	return system, nil
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
func validateOpAmpStability(plan Plan) []Diagnostic {
	var diagnostics []Diagnostic
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		analysis := Analysis{ID: "stability", Kind: AnalysisDCOperatingPoint}
		system, systemDiagnostics := buildMNASystemWithForcedOpAmp(plan, analysis, 0, device.Component)
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
	matrix := make([][]complex128, size)
	original := make([][]complex128, size)
	for row := range matrix {
		matrix[row] = append([]complex128(nil), system.matrix[row]...)
		original[row] = append([]complex128(nil), system.matrix[row]...)
	}
	rhs := append([]complex128(nil), system.rhs...)
	originalRHS := append([]complex128(nil), system.rhs...)
	scales := make([]float64, size)
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
			reconstructed += original[row][column] * solution[column]
			rowNorm += cmplx.Abs(original[row][column])
		}
		maxResidual = math.Max(maxResidual, cmplx.Abs(reconstructed-originalRHS[row]))
		matrixNorm = math.Max(matrixNorm, rowNorm)
		rhsNorm = math.Max(rhsNorm, cmplx.Abs(originalRHS[row]))
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
	branch := system.branchIndex[component]
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
		system.matrix[branch][index] += 1
	}
	if index, exists := system.nodeIndex[terminals["IN_PLUS"]]; exists {
		system.matrix[branch][index] -= gain
	}
	if index, exists := system.nodeIndex[terminals["IN_MINUS"]]; exists {
		system.matrix[branch][index] += gain
	}
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

func validateOpAmpOperatingPoints(plan Plan, point AnalysisPoint) []Diagnostic {
	values := make(map[string]float64, len(point.Nodes))
	for _, node := range point.Nodes {
		values[node.Node] = node.Real
	}
	var diagnostics []Diagnostic
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		terminals := terminalMap(device)
		parameters := namedValueMap(device.ModelParameters)
		negative := values[terminals["V_MINUS"]]
		positive := values[terminals["V_PLUS"]]
		output := values[terminals["OUT"]]
		supply := positive - negative
		if supply < parameters["supply_min_v"] || supply > parameters["supply_max_v"] {
			diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".supply", Message: fmt.Sprintf("DC supply %.12g V is outside catalog-backed range %.12g..%.12g V", supply, parameters["supply_min_v"], parameters["supply_max_v"]), Suggestion: "adjust source conditions or select a compatible catalog op-amp"})
			continue
		}
		minimum := negative + parameters["output_low_margin_v"]
		maximum := positive - parameters["output_high_margin_v"]
		if output < minimum || output > maximum {
			diagnostics = append(diagnostics, Diagnostic{Path: "devices." + device.Component + ".output", Message: fmt.Sprintf("DC output %.12g V is outside catalog-backed linear range %.12g..%.12g V", output, minimum, maximum), Suggestion: "correct bias/feedback, reduce requested operating level, or select a compatible op-amp"})
		}
	}
	return diagnostics
}

func assertionValue(results []AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	for _, result := range results {
		if result.ID != assertion.AnalysisID {
			continue
		}
		for _, point := range result.Points {
			if result.Kind == AnalysisACSweep && math.Abs(point.FrequencyHz-assertion.FrequencyHz) > math.Max(1, math.Abs(point.FrequencyHz))*1e-12 {
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
