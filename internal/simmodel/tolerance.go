package simmodel

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
)

const (
	maxWorstCaseUncertainties = 6
	maxWorstCaseWorkers       = 8
)

// EvaluateWorstCase evaluates the nominal point, each one-at-a-time endpoint,
// and the complete lower/upper corner set across independent uncertainty
// groups. Catalog temperature ranges are one correlated environmental group,
// even when many devices carry the same operating-temperature evidence. The
// fixed enumeration makes the result reproducible and deliberately fails
// closed before unbounded work.
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

	corners := deterministicCorners(plan.Uncertainties)
	evaluations := evaluateWorstCaseCorners(base, corners)
	for index, evaluation := range evaluations {
		assignments := corners[index]
		if hasCornerEvaluationFailure(evaluation.diagnostics) {
			id := cornerID(assignments)
			report.Corners = append(report.Corners, CornerResult{ID: id, Assignments: assignments, Assertions: evaluation.report.Assertions, Status: "blocked"})
			return report, append([]Diagnostic{{Path: "worst_case", Message: "corner " + id + " could not be evaluated", Suggestion: "supply bounded catalog evidence compatible with the trusted model"}}, evaluation.diagnostics...)
		}
		report.Corners = append(report.Corners, CornerResult{ID: cornerID(assignments), Assignments: assignments, Assertions: evaluation.report.Assertions, Status: evaluation.report.Status})
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

func hasCornerEvaluationFailure(diagnostics []Diagnostic) bool {
	return slices.ContainsFunc(diagnostics, func(diagnostic Diagnostic) bool {
		return !strings.HasPrefix(diagnostic.Path, "assertions.")
	})
}

type worstCaseCornerEvaluation struct {
	report      Report
	diagnostics []Diagnostic
}

func evaluateWorstCaseCorners(base Plan, corners [][]NamedValue) []worstCaseCornerEvaluation {
	results := make([]worstCaseCornerEvaluation, len(corners))
	if len(corners) == 0 {
		return results
	}
	uniqueCorners, resultIndex := uniqueCornerEvaluationPlan(corners)
	uniqueResults := make([]worstCaseCornerEvaluation, len(uniqueCorners))
	workerCount := min(len(uniqueCorners), maxWorstCaseWorkers, max(1, runtime.GOMAXPROCS(0)))
	jobs := make(chan int)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for index := range jobs {
				cornerPlan := ClonePlan(base)
				for _, assignment := range uniqueCorners[index] {
					if diagnostic := applyUncertainty(&cornerPlan, assignment.Name, assignment.Value); diagnostic != nil {
						uniqueResults[index].diagnostics = []Diagnostic{*diagnostic}
						break
					}
				}
				if len(uniqueResults[index].diagnostics) != 0 {
					continue
				}
				if cornerPlan.TopologyHash != "" {
					cornerPlan.TopologyHash = topologyHash(cornerPlan.GroundNode, cornerPlan.Nodes, cornerPlan.Devices)
				}
				uniqueResults[index].report, uniqueResults[index].diagnostics = evaluateNominal(cornerPlan)
			}
		}()
	}
	for index := range uniqueCorners {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	for index, uniqueIndex := range resultIndex {
		results[index] = uniqueResults[uniqueIndex]
	}
	return results
}

func uniqueCornerEvaluationPlan(corners [][]NamedValue) ([][]NamedValue, []int) {
	unique := make([][]NamedValue, 0, len(corners))
	indices := make([]int, len(corners))
	seen := make(map[string]int, len(corners))
	for index, corner := range corners {
		id := cornerID(corner)
		if uniqueIndex, exists := seen[id]; exists {
			indices[index] = uniqueIndex
			continue
		}
		uniqueIndex := len(unique)
		seen[id] = uniqueIndex
		indices[index] = uniqueIndex
		unique = append(unique, corner)
	}
	return unique, indices
}

func planWithoutUncertainties(plan Plan) Plan {
	clone := ClonePlan(plan)
	clone.Uncertainties = nil
	return clone
}

func validateUncertainties(uncertainties []Uncertainty) []Diagnostic {
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
	groups := groupedUncertainties(uncertainties)
	for _, group := range groups {
		if group.correlated && group.minimum > group.maximum {
			return []Diagnostic{{Path: "uncertainties", Message: "correlated environmental temperature ranges do not overlap", Suggestion: "select components qualified for one shared operating-temperature range"}}
		}
	}
	if len(groups) > maxWorstCaseUncertainties {
		return []Diagnostic{{Path: "uncertainties", Message: fmt.Sprintf("worst-case analysis supports at most %d independent bounded uncertainty groups", maxWorstCaseUncertainties), Suggestion: "partition the design or provide a smaller reviewed uncertainty set"}}
	}
	return nil
}

type uncertaintyGroup struct {
	key        string
	target     string
	members    []int
	minimum    float64
	maximum    float64
	correlated bool
}

func groupedUncertainties(uncertainties []Uncertainty) []uncertaintyGroup {
	groups := []uncertaintyGroup{}
	indices := map[string]int{}
	for index, uncertainty := range uncertainties {
		key, target := uncertainty.Target, uncertainty.Target
		correlated := false
		if _, parameter, ok := deviceParameterTarget(uncertainty.Target); ok && parameter == "junction_temperature_k" && strings.HasSuffix(uncertainty.Source, ":temperature") {
			key, target = "environment.temperature", "environment.temperature"
			correlated = true
		}
		groupIndex, exists := indices[key]
		if !exists {
			groupIndex = len(groups)
			indices[key] = groupIndex
			groups = append(groups, uncertaintyGroup{key: key, target: target, minimum: uncertainty.Minimum, maximum: uncertainty.Maximum, correlated: correlated})
		} else if correlated {
			groups[groupIndex].minimum = math.Max(groups[groupIndex].minimum, uncertainty.Minimum)
			groups[groupIndex].maximum = math.Min(groups[groupIndex].maximum, uncertainty.Maximum)
		}
		groups[groupIndex].members = append(groups[groupIndex].members, index)
	}
	return groups
}

func nominalAssignments(uncertainties []Uncertainty) []NamedValue {
	assignments := make([]NamedValue, len(uncertainties))
	for index, uncertainty := range uncertainties {
		assignments[index] = NamedValue{Name: uncertainty.Target, Value: uncertainty.Nominal}
	}
	return assignments
}

func deterministicCorners(uncertainties []Uncertainty) [][]NamedValue {
	groups := groupedUncertainties(uncertainties)
	result := make([][]NamedValue, 0, 2*len(groups)+(1<<len(groups)))
	for _, group := range groups {
		for _, maximum := range []bool{false, true} {
			assignments := nominalAssignments(uncertainties)
			for _, index := range group.members {
				if group.correlated {
					assignments[index].Value = group.minimum
					if maximum {
						assignments[index].Value = group.maximum
					}
				} else {
					assignments[index].Value = uncertainties[index].Minimum
					if maximum {
						assignments[index].Value = uncertainties[index].Maximum
					}
				}
			}
			result = append(result, assignments)
		}
	}
	for mask := 0; mask < 1<<len(groups); mask++ {
		assignments := make([]NamedValue, len(uncertainties))
		for groupIndex, group := range groups {
			for _, index := range group.members {
				uncertainty := uncertainties[index]
				value := group.minimum
				if !group.correlated {
					value = uncertainty.Minimum
				}
				if mask&(1<<groupIndex) != 0 {
					value = group.maximum
					if !group.correlated {
						value = uncertainty.Maximum
					}
				}
				assignments[index] = NamedValue{Name: uncertainty.Target, Value: value}
			}
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
	return assertion.AnalysisID + "." + assertion.Node + "." + assertion.Component + "." + strings.Join(assertion.Components, "\x1f") + "." + assertion.Quantity + fmt.Sprintf("@%.12g/%.12g", assertion.FrequencyHz, assertion.TimeS)
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
	for index, group := range groupedUncertainties(uncertainties) {
		best := SensitivityResult{Target: group.target, Margin: math.Inf(1)}
		for _, corner := range corners[1+2*index : 1+2*index+2] {
			for _, assertion := range corner.Assertions {
				id := assertionID(assertion)
				if _, exists := nominal[id]; !exists {
					continue
				}
				if assertionMargin(assertion) < best.Margin {
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
