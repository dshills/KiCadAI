package simmodel

import (
	"fmt"
	"reflect"
)

// VerifyPassingCornersV23 checks declared assertion identities and the exact
// existing corner schedule, without rerunning numerical work. Execution binding
// and source/replay authentication remain separate, required checks.
func VerifyPassingCornersV23(plan Plan, report Report) error {
	if len(ValidatePlan(plan)) != 0 || report.Status != "pass" || !passingAssertionsV23(plan.Assertions, report.Assertions) {
		return fmt.Errorf("V23 passing report assertion evidence differs")
	}
	if !plan.WorstCase {
		if len(report.Corners) != 0 {
			return fmt.Errorf("V23 non-worst-case report has unexpected corners")
		}
		return nil
	}
	if len(validateUncertainties(plan.Uncertainties)) != 0 || len(report.Corners) == 0 {
		return fmt.Errorf("V23 worst-case report lacks required uncertainty evidence")
	}
	nominal := normalizedWorstCaseCorner("nominal", nominalAssignments(plan.Uncertainties), report.Assertions)
	if !reflect.DeepEqual(report.Corners[0], nominal) {
		return fmt.Errorf("V23 nominal corner differs from the nominal report")
	}
	expected := []CornerResult{{ID: "nominal", Assignments: nominalAssignments(plan.Uncertainties)}}
	for _, assignments := range deterministicCorners(plan.Uncertainties) {
		expected = append(expected, CornerResult{ID: cornerID(assignments), Assignments: assignments})
	}
	verifyPrefix := func() error {
		if len(report.Corners) < len(expected) {
			return fmt.Errorf("V23 worst-case report omitted a required corner")
		}
		for i, corner := range expected {
			actual := report.Corners[i]
			if actual.ID != corner.ID || !reflect.DeepEqual(actual.Assignments, corner.Assignments) || actual.Status != "pass" || !passingAssertionsV23(plan.Assertions, actual.Assertions) {
				return fmt.Errorf("V23 worst-case corner assignment or assertion differs")
			}
		}
		return nil
	}
	if err := verifyPrefix(); err != nil {
		return err
	}
	if len(groupedUncertainties(plan.Uncertainties)) > maxExhaustiveWorstCaseGroups {
		existing := map[string]bool{}
		for _, corner := range expected {
			existing[corner.ID] = true
		}
		for _, assignments := range assertionDirectedCorners(report.Corners[:len(expected)], plan.Uncertainties) {
			id := cornerID(assignments)
			if !existing[id] {
				existing[id] = true
				expected = append(expected, CornerResult{ID: id, Assignments: assignments})
			}
		}
	}
	if len(report.Corners) != len(expected) {
		return fmt.Errorf("V23 worst-case report has a missing or extra corner")
	}
	return verifyPrefix()
}

func passingAssertionsV23(requested []Assertion, actual []AssertionResult) bool {
	if len(requested) == 0 || len(requested) != len(actual) {
		return false
	}
	for i, assertion := range requested {
		observed := actual[i]
		if !observed.Pass || !finite(observed.Actual) || !assertionWithinBounds(observed.Actual, assertion.Min, assertion.Max) {
			return false
		}
		want := AssertionResult{Metric: assertion.Metric, AnalysisID: assertion.AnalysisID, Node: assertion.Node, Component: assertion.Component, Components: assertion.Components, ReferenceNode: assertion.ReferenceNode, Quantity: assertion.Quantity, FrequencyHz: assertion.FrequencyHz, TimeS: assertion.TimeS, Min: assertion.Min, Max: assertion.Max, Actual: observed.Actual, Pass: true}
		if !reflect.DeepEqual(want, observed) {
			return false
		}
	}
	return true
}
