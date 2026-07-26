package closedloopsynthesis

import (
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
)

func evaluateHierarchyVerification(requirement architecturesearch.Requirement, systemPlan *architecturesearch.SystemPlan, assertions []AssertionResult) (*HierarchyVerificationEvidence, []Diagnostic) {
	if !hierarchicalRequirement(requirement) {
		return nil, nil
	}
	if systemPlan == nil {
		return nil, []Diagnostic{{Path: "evaluation.hierarchy", Message: "hierarchical evaluation requires its generated system plan"}}
	}
	if err := architecturesearch.ValidateSystemPlan(requirement, systemPlan.CandidateFingerprint, *systemPlan); err != nil {
		return nil, []Diagnostic{{Path: "evaluation.hierarchy", Message: err.Error()}}
	}
	byBehavior := map[string][]AssertionResult{}
	for _, assertion := range assertions {
		byBehavior[assertion.RequirementID] = append(byBehavior[assertion.RequirementID], assertion)
	}
	evidence := &HierarchyVerificationEvidence{SystemPlanHash: systemPlan.PlanHash, Status: "pass"}
	var diagnostics []Diagnostic
	for _, block := range systemPlan.Hierarchy.Blocks {
		result := BlockVerificationResult{
			BlockID: block.ID, BehaviorIDs: append([]string(nil), block.RequiredBehaviorIDs...),
			InterfaceIDs: append([]string(nil), block.InterfaceIDs...), Pass: true,
		}
		for _, behaviorID := range block.RequiredBehaviorIDs {
			results := byBehavior[behaviorID]
			if len(results) == 0 {
				diagnostics = append(diagnostics, Diagnostic{
					Path:    "evaluation.hierarchy.blocks." + block.ID,
					Message: "missing block-scoped behavioral evidence for " + behaviorID,
				})
				result.Pass = false
				continue
			}
			for _, assertion := range results {
				result.Pass = result.Pass && assertion.Pass
			}
		}
		for _, interfaceID := range block.InterfaceIDs {
			if !slices.ContainsFunc(systemPlan.Interfaces, func(connection architecturesearch.InterfaceContractPlan) bool {
				return connection.ID == interfaceID && connection.Status == "pass"
			}) {
				diagnostics = append(diagnostics, Diagnostic{
					Path:    "evaluation.hierarchy.blocks." + block.ID,
					Message: "missing proven block interface contract " + interfaceID,
				})
				result.Pass = false
			}
		}
		evidence.Status = hierarchyStatus(evidence.Status, result.Pass)
		evidence.Blocks = append(evidence.Blocks, result)
	}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		result := EndToEndVerificationResult{
			RequirementID:  behavior.ID,
			OperatingCases: append([]string(nil), behavior.OperatingCases...),
			Pass:           true,
		}
		results := byBehavior[behavior.ID]
		if len(results) != len(behavior.OperatingCases) {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    "evaluation.hierarchy.end_to_end." + behavior.ID,
				Message: "end-to-end evidence does not cover every declared operating case",
			})
			result.Pass = false
		}
		for _, caseID := range behavior.OperatingCases {
			found := false
			for _, assertion := range results {
				if assertion.OperatingCase == caseID {
					found = true
					result.Pass = result.Pass && assertion.Pass
				}
			}
			if !found {
				result.Pass = false
			}
		}
		evidence.Status = hierarchyStatus(evidence.Status, result.Pass)
		evidence.EndToEnd = append(evidence.EndToEnd, result)
	}
	slices.SortStableFunc(evidence.Blocks, func(left, right BlockVerificationResult) int {
		return strings.Compare(left.BlockID, right.BlockID)
	})
	slices.SortStableFunc(evidence.EndToEnd, func(left, right EndToEndVerificationResult) int {
		return strings.Compare(left.RequirementID, right.RequirementID)
	})
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return evidence, diagnostics
}

func hierarchyStatus(current string, pass bool) string {
	if current == "fail" || !pass {
		return "fail"
	}
	return "pass"
}
