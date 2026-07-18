package placement

import (
	"fmt"
	"strings"

	"kicadai/internal/reports"
)

// ValidateRequiredProximity fail-closes placement when a declared required
// electrical-proximity relationship is not satisfied by final geometry.
func ValidateRequiredProximity(request Request, placements []PlacementResult) []reports.Issue {
	request = NormalizeRequest(request)
	placementsByRef := make(map[string]PlacementResult, len(placements))
	for _, placed := range placements {
		if placed.Reason == "" {
			placementsByRef[normalizeRef(placed.Ref)] = placed
		}
	}
	componentsByRef := componentsByNormalizedRef(request.Components)
	toleranceMM := request.Rules.GridMM
	if toleranceMM <= 0 {
		toleranceMM = DefaultRules().GridMM
	}
	var issues []reports.Issue
	for ruleIndex, rule := range request.ProximityRules {
		if !rule.Required {
			continue
		}
		anchorRef := normalizeRef(rule.AnchorRef)
		anchorPlacement, anchorOK := placementsByRef[anchorRef]
		anchorComponent, componentOK := componentsByRef[anchorRef]
		if !anchorOK || !componentOK {
			issues = append(issues, requiredProximityIssue(ruleIndex, rule, rule.AnchorRef, "required proximity anchor is not placed"))
			continue
		}
		for _, targetRefRaw := range rule.TargetRefs {
			targetRef := normalizeRef(targetRefRaw)
			targetPlacement, targetOK := placementsByRef[targetRef]
			targetComponent, targetComponentOK := componentsByRef[targetRef]
			if !targetOK || !targetComponentOK {
				issues = append(issues, requiredProximityIssue(ruleIndex, rule, targetRefRaw, "required proximity target is not placed"))
				continue
			}
			distance, _ := proximityDistance(anchorComponent, anchorPlacement, rule.AnchorPins, targetComponent, targetPlacement, rule.TargetPins)
			if rule.MaxDistanceMM > 0 && distance > rule.MaxDistanceMM+toleranceMM {
				issues = append(issues, requiredProximityIssue(
					ruleIndex,
					rule,
					targetRefRaw,
					fmt.Sprintf("required proximity %.2fmm exceeds %.2fmm", distance, rule.MaxDistanceMM),
				))
			}
		}
	}
	return issues
}

func requiredProximityIssue(ruleIndex int, rule ProximityRule, targetRef string, message string) reports.Issue {
	refs := []string{strings.TrimSpace(rule.AnchorRef), strings.TrimSpace(targetRef)}
	compact := refs[:0]
	for _, ref := range refs {
		if ref != "" {
			compact = append(compact, ref)
		}
	}
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityError,
		Path:       fmt.Sprintf("proximity_rules[%d].max_distance_mm", ruleIndex),
		Message:    "proximity rule " + rule.ID + ": " + message,
		Refs:       compact,
		Suggestion: "move the required components closer or revise the declared proximity limit",
	}
}
