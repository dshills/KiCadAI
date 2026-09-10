package simmodel

import (
	"fmt"
	"kicadai/internal/runtimebudget"
	"slices"
	"sync"
)

// V23 retains exactly the historical corner selection, worker bound, and
// fail-closed aggregation while explicitly selecting its nominal solver.
func evaluateWorstCaseV23(plan Plan) (Report, []Diagnostic) {
	if diagnostics := validateUncertainties(plan.Uncertainties); len(diagnostics) != 0 {
		report, _ := evaluateNominalV23(planWithoutUncertainties(plan))
		return report, diagnostics
	}
	base := planWithoutUncertainties(plan)
	nominal, nominalDiagnostics := evaluateNominalV23(base)
	if len(nominalDiagnostics) != 0 {
		return nominal, nominalDiagnostics
	}
	report := nominal
	report.Corners = []CornerResult{normalizedWorstCaseCorner("nominal", nominalAssignments(plan.Uncertainties), nominal.Assertions)}

	corners := deterministicCorners(plan.Uncertainties)
	evaluations := evaluateWorstCaseCornersV23(base, corners)
	for index, evaluation := range evaluations {
		assignments := corners[index]
		if hasCornerEvaluationFailure(evaluation.diagnostics) {
			id := cornerID(assignments)
			report.Corners = append(report.Corners, CornerResult{ID: id, Assignments: assignments, Assertions: evaluation.report.Assertions, Status: "blocked"})
			return report, append([]Diagnostic{{Path: "worst_case", Message: "corner " + id + " could not be evaluated", Suggestion: "supply bounded catalog evidence compatible with the trusted model"}}, evaluation.diagnostics...)
		}
		report.Corners = append(report.Corners, normalizedWorstCaseCorner(cornerID(assignments), assignments, evaluation.report.Assertions))
	}
	if len(groupedUncertainties(plan.Uncertainties)) > maxExhaustiveWorstCaseGroups {
		directed := assertionDirectedCorners(report.Corners, plan.Uncertainties)
		existing := make(map[string]struct{}, len(report.Corners))
		for _, corner := range report.Corners {
			existing[corner.ID] = struct{}{}
		}
		directed = slices.DeleteFunc(directed, func(assignments []NamedValue) bool {
			id := cornerID(assignments)
			if _, ok := existing[id]; ok {
				return true
			}
			existing[id] = struct{}{}
			return false
		})
		directedEvaluations := evaluateWorstCaseCornersV23(base, directed)
		for index, evaluation := range directedEvaluations {
			assignments := directed[index]
			if hasCornerEvaluationFailure(evaluation.diagnostics) {
				id := cornerID(assignments)
				report.Corners = append(report.Corners, CornerResult{ID: id, Assignments: assignments, Assertions: evaluation.report.Assertions, Status: "blocked"})
				return report, append([]Diagnostic{{Path: "worst_case", Message: "directed corner " + id + " could not be evaluated", Suggestion: "supply bounded catalog evidence compatible with the trusted model"}}, evaluation.diagnostics...)
			}
			report.Corners = append(report.Corners, normalizedWorstCaseCorner(cornerID(assignments), assignments, evaluation.report.Assertions))
		}
	}
	report.Sensitivity = sensitivity(report.Corners, plan.Uncertainties)
	for _, corner := range report.Corners[1:] {
		for _, assertion := range corner.Assertions {
			if !assertionWithinBounds(assertion.Actual, assertion.Min, assertion.Max) {
				report.Status = "blocked"
				dominant := dominantSensitivity(report.Sensitivity, assertion)
				message := fmt.Sprintf("worst-case corner %s measured %.12g outside trusted bounds %.12g..%.12g", corner.ID, assertion.Actual, assertion.Min, assertion.Max)
				if dominant.Target != "" {
					message += "; dominant contributor " + dominant.Target + " at " + dominant.Corner
				}
				nominalDiagnostics = append(nominalDiagnostics, Diagnostic{Code: DiagnosticAssertionOutOfBounds, Path: "worst_case." + corner.ID, Message: message, Suggestion: "adjust catalog-backed component values or operating conditions"})
			}
		}
	}
	if len(nominalDiagnostics) != 0 {
		report.Status = "blocked"
	}
	return report, nominalDiagnostics
}
func evaluateWorstCaseCornersV23(base Plan, corners [][]NamedValue) []worstCaseCornerEvaluation {
	results := make([]worstCaseCornerEvaluation, len(corners))
	if len(corners) == 0 {
		return results
	}
	uniqueCorners, resultIndex := uniqueCornerEvaluationPlan(corners)
	uniqueResults := make([]worstCaseCornerEvaluation, len(uniqueCorners))
	workerCount := worstCaseCornerWorkerCount(len(uniqueCorners), len(base.Analyses), runtimebudget.Capacity())
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
				uniqueResults[index].report, uniqueResults[index].diagnostics = evaluateNominalV23(cornerPlan)
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
