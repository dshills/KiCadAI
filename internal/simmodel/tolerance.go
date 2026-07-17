package simmodel

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const maxWorstCaseUncertainties = 6

// EvaluateWorstCase evaluates the nominal point, each one-at-a-time endpoint,
// and the complete lower/upper corner set. The fixed enumeration makes the
// result reproducible and deliberately fails closed before unbounded work.
func EvaluateWorstCase(plan Plan) (Report, []Diagnostic) {
	if diagnostics := validateUncertainties(plan.Uncertainties); len(diagnostics) != 0 {
		report, _ := evaluateNominal(planWithoutUncertainties(plan))
		return report, diagnostics
	}
	base := planWithoutUncertainties(plan)
	nominal, nominalDiagnostics := evaluateNominal(base)
	if len(nominalDiagnostics) != 0 {
		return nominal, nominalDiagnostics
	}
	report := nominal
	report.Corners = []CornerResult{{ID: "nominal", Assignments: nominalAssignments(plan.Uncertainties), Assertions: nominal.Assertions, Status: nominal.Status}}

	for _, assignments := range deterministicCorners(plan.Uncertainties) {
		cornerPlan := ClonePlan(base)
		for _, assignment := range assignments {
			if diagnostic := applyUncertainty(&cornerPlan, assignment.Name, assignment.Value); diagnostic != nil {
				return report, []Diagnostic{*diagnostic}
			}
		}
		if cornerPlan.TopologyHash != "" {
			cornerPlan.TopologyHash = topologyHash(cornerPlan.GroundNode, cornerPlan.Nodes, cornerPlan.Devices)
		}
		corner, diagnostics := evaluateNominal(cornerPlan)
		if len(diagnostics) != 0 && len(corner.Assertions) == 0 {
			return report, append([]Diagnostic{{Path: "worst_case", Message: "corner " + cornerID(assignments) + " could not be evaluated", Suggestion: "supply bounded catalog evidence compatible with the trusted model"}}, diagnostics...)
		}
		report.Corners = append(report.Corners, CornerResult{ID: cornerID(assignments), Assignments: assignments, Assertions: corner.Assertions, Status: corner.Status})
	}
	report.Sensitivity = sensitivity(report.Corners, plan.Uncertainties)
	for _, corner := range report.Corners[1:] {
		for _, assertion := range corner.Assertions {
			if !assertion.Pass {
				report.Status = "blocked"
				dominant := dominantSensitivity(report.Sensitivity, assertion)
				message := fmt.Sprintf("worst-case corner %s measured %.12g outside trusted bounds %.12g..%.12g", corner.ID, assertion.Actual, assertion.Min, assertion.Max)
				if dominant.Target != "" {
					message += "; dominant contributor " + dominant.Target + " at " + dominant.Corner
				}
				nominalDiagnostics = append(nominalDiagnostics, Diagnostic{Path: "worst_case." + corner.ID, Message: message, Suggestion: "adjust catalog-backed component values or operating conditions"})
			}
		}
	}
	if len(nominalDiagnostics) != 0 {
		report.Status = "blocked"
	}
	return report, nominalDiagnostics
}

func planWithoutUncertainties(plan Plan) Plan {
	clone := ClonePlan(plan)
	clone.Uncertainties = nil
	return clone
}

func validateUncertainties(uncertainties []Uncertainty) []Diagnostic {
	if len(uncertainties) > maxWorstCaseUncertainties {
		return []Diagnostic{{Path: "uncertainties", Message: fmt.Sprintf("worst-case analysis supports at most %d bounded uncertainties", maxWorstCaseUncertainties), Suggestion: "partition the design or provide a smaller reviewed uncertainty set"}}
	}
	if len(uncertainties) == 0 {
		return []Diagnostic{{Path: "uncertainties", Message: "worst-case analysis requires reviewed bounded uncertainty evidence"}}
	}
	previous := ""
	for index, uncertainty := range uncertainties {
		path := fmt.Sprintf("uncertainties[%d]", index)
		if strings.TrimSpace(uncertainty.Target) == "" || strings.TrimSpace(uncertainty.Source) == "" {
			return []Diagnostic{{Path: path, Message: "uncertainty requires canonical target and reviewed evidence source"}}
		}
		if uncertainty.Target <= previous {
			return []Diagnostic{{Path: path + ".target", Message: "uncertainties must be unique and canonically ordered"}}
		}
		previous = uncertainty.Target
		if !finite(uncertainty.Nominal) || !finite(uncertainty.Minimum) || !finite(uncertainty.Maximum) || uncertainty.Minimum > uncertainty.Nominal || uncertainty.Nominal > uncertainty.Maximum || uncertainty.Minimum == uncertainty.Maximum {
			return []Diagnostic{{Path: path, Message: "uncertainty requires finite minimum <= nominal <= maximum with nonzero span"}}
		}
	}
	return nil
}

func nominalAssignments(uncertainties []Uncertainty) []NamedValue {
	assignments := make([]NamedValue, len(uncertainties))
	for index, uncertainty := range uncertainties {
		assignments[index] = NamedValue{Name: uncertainty.Target, Value: uncertainty.Nominal}
	}
	return assignments
}

func deterministicCorners(uncertainties []Uncertainty) [][]NamedValue {
	result := make([][]NamedValue, 0, 2*len(uncertainties)+(1<<len(uncertainties)))
	for index, uncertainty := range uncertainties {
		for _, value := range []float64{uncertainty.Minimum, uncertainty.Maximum} {
			assignments := nominalAssignments(uncertainties)
			assignments[index].Value = value
			result = append(result, assignments)
		}
	}
	for mask := 0; mask < 1<<len(uncertainties); mask++ {
		assignments := make([]NamedValue, len(uncertainties))
		for index, uncertainty := range uncertainties {
			value := uncertainty.Minimum
			if mask&(1<<index) != 0 {
				value = uncertainty.Maximum
			}
			assignments[index] = NamedValue{Name: uncertainty.Target, Value: value}
		}
		result = append(result, assignments)
	}
	return result
}

func cornerID(assignments []NamedValue) string {
	parts := make([]string, len(assignments))
	for index, assignment := range assignments {
		parts[index] = assignment.Name + "=" + fmt.Sprintf("%.12g", assignment.Value)
	}
	return strings.Join(parts, ",")
}

func applyUncertainty(plan *Plan, target string, value float64) *Diagnostic {
	if role, ok := bindingValueTarget(target); ok {
		for index := range plan.Bindings {
			if plan.Bindings[index].Role == role && plan.Bindings[index].ValueSI != nil {
				plan.Bindings[index].ValueSI = floatPointer(value)
				return nil
			}
		}
	}
	if role, parameter, ok := bindingParameterTarget(target); ok {
		for index := range plan.Bindings {
			if plan.Bindings[index].Role == role {
				for parameterIndex := range plan.Bindings[index].ModelParameters {
					if plan.Bindings[index].ModelParameters[parameterIndex].Name == parameter {
						plan.Bindings[index].ModelParameters[parameterIndex].Value = value
						return nil
					}
				}
			}
		}
	}
	if component, ok := deviceValueTarget(target); ok {
		for index := range plan.Devices {
			if plan.Devices[index].Component == component && plan.Devices[index].ValueSI != nil {
				plan.Devices[index].ValueSI = floatPointer(value)
				return nil
			}
		}
	}
	if component, parameter, ok := deviceParameterTarget(target); ok {
		for index := range plan.Devices {
			if plan.Devices[index].Component == component {
				for parameterIndex := range plan.Devices[index].ModelParameters {
					if plan.Devices[index].ModelParameters[parameterIndex].Name == parameter {
						plan.Devices[index].ModelParameters[parameterIndex].Value = value
						return nil
					}
				}
			}
		}
	}
	if analysisID, component, ok := excitationTarget(target); ok {
		for analysis := range plan.Analyses {
			if plan.Analyses[analysis].ID != analysisID {
				continue
			}
			for excitation := range plan.Analyses[analysis].Excitations {
				if plan.Analyses[analysis].Excitations[excitation].Component == component {
					plan.Analyses[analysis].Excitations[excitation].DCValue = value
					return nil
				}
			}
		}
	}
	return &Diagnostic{Path: "uncertainties." + target, Message: "uncertainty target is absent from resolved trusted plan", Suggestion: "resolve the circuit again with compatible reviewed catalog evidence"}
}

func bindingValueTarget(target string) (string, bool) {
	return trimTarget(target, "bindings.", ".value_si")
}
func deviceValueTarget(target string) (string, bool) {
	return trimTarget(target, "devices.", ".value_si")
}

func bindingParameterTarget(target string) (string, string, bool) {
	return parameterTarget(target, "bindings.")
}
func deviceParameterTarget(target string) (string, string, bool) {
	return parameterTarget(target, "devices.")
}

func parameterTarget(target, prefix string) (string, string, bool) {
	remainder := strings.TrimPrefix(target, prefix)
	if remainder == target {
		return "", "", false
	}
	separator := ".model_parameters."
	index := strings.LastIndex(remainder, separator)
	if index <= 0 || index+len(separator) >= len(remainder) {
		return "", "", false
	}
	return remainder[:index], remainder[index+len(separator):], true
}

func excitationTarget(target string) (string, string, bool) {
	remainder := strings.TrimPrefix(target, "analyses.")
	if remainder == target || !strings.HasSuffix(remainder, ".dc_value") {
		return "", "", false
	}
	remainder = strings.TrimSuffix(remainder, ".dc_value")
	separator := ".excitations."
	index := strings.LastIndex(remainder, separator)
	if index <= 0 || index+len(separator) >= len(remainder) {
		return "", "", false
	}
	return remainder[:index], remainder[index+len(separator):], true
}

func trimTarget(target, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(target, prefix) || !strings.HasSuffix(target, suffix) {
		return "", false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(target, prefix), suffix)
	return value, value != ""
}

func floatPointer(value float64) *float64 { return &value }

func assertionID(assertion AssertionResult) string {
	if assertion.Metric != "" {
		return assertion.Metric
	}
	return assertion.AnalysisID + "." + assertion.Node + "." + assertion.Quantity + fmt.Sprintf("@%.12g/%.12g", assertion.FrequencyHz, assertion.TimeS)
}

func assertionMargin(assertion AssertionResult) float64 {
	return math.Min(assertion.Actual-assertion.Min, assertion.Max-assertion.Actual)
}

func sensitivity(corners []CornerResult, uncertainties []Uncertainty) []SensitivityResult {
	if len(corners) == 0 {
		return nil
	}
	nominal := map[string]AssertionResult{}
	for _, assertion := range corners[0].Assertions {
		nominal[assertionID(assertion)] = assertion
	}
	result := []SensitivityResult{}
	for index, uncertainty := range uncertainties {
		best := SensitivityResult{Target: uncertainty.Target, Margin: math.Inf(1)}
		for _, corner := range corners[1+2*index : 1+2*index+2] {
			for _, assertion := range corner.Assertions {
				id := assertionID(assertion)
				if _, exists := nominal[id]; !exists {
					continue
				}
				if _, present := assignmentFor(corner.Assignments, uncertainty.Target); present && assertionMargin(assertion) < best.Margin {
					best.Assertion, best.Corner, best.Margin = id, corner.ID, assertionMargin(assertion)
				}
			}
		}
		if best.Assertion != "" {
			result = append(result, best)
		}
	}
	slices.SortFunc(result, func(a, b SensitivityResult) int {
		if a.Assertion != b.Assertion {
			return strings.Compare(a.Assertion, b.Assertion)
		}
		if a.Margin != b.Margin {
			return int(math.Copysign(1, a.Margin-b.Margin))
		}
		return strings.Compare(a.Target, b.Target)
	})
	return result
}

func sameUncertaintyValue(left, right float64) bool {
	return math.Abs(left-right) <= 1e-12*math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
}

func assignmentFor(assignments []NamedValue, target string) (float64, bool) {
	for _, assignment := range assignments {
		if assignment.Name == target {
			return assignment.Value, true
		}
	}
	return 0, false
}

func dominantSensitivity(results []SensitivityResult, assertion AssertionResult) SensitivityResult {
	id := assertionID(assertion)
	for _, result := range results {
		if result.Assertion == id {
			return result
		}
	}
	return SensitivityResult{}
}
