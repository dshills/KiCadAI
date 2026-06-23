package placement

import (
	"slices"
	"strings"
)

func normalizeMobilityPolicy(component Component) MobilityPolicy {
	policy := component.Mobility
	policy.Reason = strings.TrimSpace(policy.Reason)
	policy.OwnerScope = strings.TrimSpace(policy.OwnerScope)
	if strings.TrimSpace(policy.GroupID) == "" {
		policy.GroupID = component.GroupID
	}
	policy.GroupID = strings.TrimSpace(policy.GroupID)
	policy.Transforms = mobilityUniqueSortedStrings(policy.Transforms)
	policy.Constraints = mobilityUniqueSortedStrings(policy.Constraints)
	if component.Fixed {
		policy.Class = MobilityFixed
		policy.RouteHandling = RouteHandlingPreserveFixed
	} else if policy.Class == "" {
		policy.Class = MobilityUnowned
	}
	if policy.Reason == "" {
		switch policy.Class {
		case MobilityFixed:
			policy.Reason = "component is fixed"
		case MobilityUnowned:
			policy.Reason = "component has no generated mobility ownership"
		}
	}
	if policy.RouteHandling == "" {
		switch policy.Class {
		case MobilityFixed:
			policy.RouteHandling = RouteHandlingPreserveFixed
		case MobilityGroupTransform:
			policy.RouteHandling = RouteHandlingTransformWithGroup
		case MobilityLocalRebuild, MobilitySoftPreferred:
			policy.RouteHandling = RouteHandlingInvalidateRebuild
		default:
			policy.RouteHandling = RouteHandlingUnsupported
		}
	}
	if len(policy.Transforms) == 0 {
		switch policy.Class {
		case MobilityGroupTransform, MobilityLocalRebuild, MobilitySoftPreferred:
			policy.Transforms = []string{"translate"}
		}
	}
	return policy
}

func MobilitySummaryForComponents(components []Component) MobilitySummary {
	summary := MobilitySummary{Total: len(components), ByClass: map[string]int{}}
	for _, component := range components {
		addMobilityPolicyToSummary(&summary, normalizeMobilityPolicy(component))
	}
	if len(summary.ByClass) == 0 {
		summary.ByClass = nil
	}
	return summary
}

func MobilitySummaryForResults(placements []PlacementResult) MobilitySummary {
	summary := MobilitySummary{Total: len(placements), ByClass: map[string]int{}}
	for _, placed := range placements {
		addMobilityPolicyToSummary(&summary, placed.Mobility)
	}
	if len(summary.ByClass) == 0 {
		summary.ByClass = nil
	}
	return summary
}

func addMobilityPolicyToSummary(summary *MobilitySummary, policy MobilityPolicy) {
	if summary.ByClass == nil {
		summary.ByClass = map[string]int{}
	}
	class := string(policy.Class)
	if class == "" {
		class = string(MobilityUnowned)
	}
	summary.ByClass[class]++
	switch policy.Class {
	case MobilityFixed:
		summary.FixedCount++
	case MobilityUnowned, "":
		summary.UnownedCount++
	case MobilityGroupTransform:
		summary.GroupTransformCount++
		summary.EligibleCount++
	case MobilityLocalRebuild:
		summary.LocalRebuildCount++
		summary.EligibleCount++
	case MobilitySoftPreferred:
		summary.SoftPreferredCount++
		summary.EligibleCount++
	}
	switch policy.RouteHandling {
	case RouteHandlingTransformWithGroup:
		summary.TransformableRouteCnt++
	case RouteHandlingInvalidateRebuild:
		summary.RebuildableRouteCnt++
	case RouteHandlingPreserveFixed:
		summary.PreservedRouteCnt++
	case RouteHandlingUnsupported:
		summary.UnsupportedRouteCnt++
	}
}

func mobilityUniqueSortedStrings(values []string) []string {
	seen := map[string]string{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; !ok {
			seen[trimmed] = trimmed
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
