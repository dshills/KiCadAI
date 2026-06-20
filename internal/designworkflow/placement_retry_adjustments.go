package designworkflow

import (
	"cmp"
	"fmt"
	"slices"

	"kicadai/internal/placement"
)

const (
	placementRetryBaseSpacingDeltaMM = 1.0
	placementRetryMaxSpacingDeltaMM  = 2.0
	placementRetryMaxProximityMM     = 25.0
)

type PlacementRetryAdjustment struct {
	Applied        bool     `json:"applied"`
	Attempt        int      `json:"attempt"`
	SpacingDeltaMM float64  `json:"spacing_delta_mm,omitempty"`
	ProximityRules []string `json:"proximity_rules,omitempty"`
	SkippedReasons []string `json:"skipped_reasons,omitempty"`
}

func BuildPlacementRetryAdjustment(request placement.Request, hints []PlacementRetryHint, attempt int) (placement.Request, PlacementRetryAdjustment) {
	adjusted := placement.CloneRequest(request)
	adjusted = placement.NormalizeRequest(adjusted)
	if attempt < 1 {
		attempt = 1
	}
	adjustment := PlacementRetryAdjustment{Attempt: attempt}
	refsByNet := placementRetryRefsByNet(adjusted)
	complexityByRef := placementRetryComponentComplexity(adjusted.Components)
	spacingDelta := min(placementRetryMaxSpacingDeltaMM, float64(attempt)*placementRetryBaseSpacingDeltaMM)
	orderedHints := slices.Clone(hints)
	for index := range orderedHints {
		orderedHints[index].Refs = sortedUniqueStrings(orderedHints[index].Refs)
		orderedHints[index].Nets = sortedUniqueStrings(orderedHints[index].Nets)
	}
	existingRuleIDs := placementRetryProximityRuleIDs(adjusted.ProximityRules)
	slices.SortFunc(orderedHints, func(a, b PlacementRetryHint) int {
		if compare := cmp.Compare(a.Category, b.Category); compare != 0 {
			return compare
		}
		if compare := slices.Compare(a.Nets, b.Nets); compare != 0 {
			return compare
		}
		return slices.Compare(a.Refs, b.Refs)
	})
	for _, hint := range orderedHints {
		if !hint.RetryEligible {
			adjustment.SkippedReasons = append(adjustment.SkippedReasons, "ineligible:"+string(hint.Category))
			continue
		}
		switch hint.Category {
		case PlacementRetryIncreaseSpacing, PlacementRetryImproveFanout, PlacementRetryMoveFromEdge:
			if spacingDelta > adjustment.SpacingDeltaMM {
				adjustment.SpacingDeltaMM = spacingDelta
			}
		case PlacementRetryReduceDistance:
			added := addRetryProximityRules(&adjusted, hint, refsByNet, complexityByRef, existingRuleIDs)
			if len(added) == 0 {
				adjustment.SkippedReasons = append(adjustment.SkippedReasons, "reduce_distance:no_ref_pair")
				continue
			}
			adjustment.ProximityRules = append(adjustment.ProximityRules, added...)
		default:
			adjustment.SkippedReasons = append(adjustment.SkippedReasons, "unsupported:"+string(hint.Category))
		}
	}
	if adjustment.SpacingDeltaMM > 0 {
		adjusted.Rules.ComponentSpacingMM += adjustment.SpacingDeltaMM
		adjusted.Rules.GroupSpacingMM += adjustment.SpacingDeltaMM
		adjustment.Applied = true
	}
	if len(adjustment.ProximityRules) > 0 {
		adjustment.Applied = true
		slices.Sort(adjustment.ProximityRules)
	}
	slices.Sort(adjustment.SkippedReasons)
	return adjusted, adjustment
}

func addRetryProximityRules(request *placement.Request, hint PlacementRetryHint, refsByNet map[string][]string, complexityByRef map[string]float64, existingRuleIDs map[string]struct{}) []string {
	var added []string
	for _, netName := range hint.Nets {
		refs := refsByNet[netName]
		if len(refs) < 2 {
			continue
		}
		anchor, targets := placementRetryAnchorAndTargets(refs, complexityByRef)
		for _, target := range targets {
			ruleID := "retry_reduce_distance:" + netName + ":" + anchor + ":" + target
			if _, ok := existingRuleIDs[ruleID]; ok {
				continue
			}
			request.ProximityRules = append(request.ProximityRules, placement.ProximityRule{
				ID:            ruleID,
				Source:        "routing_retry",
				AnchorRef:     anchor,
				TargetRefs:    []string{target},
				MaxDistanceMM: placementRetryMaxProximityMM,
				Weight:        1,
			})
			existingRuleIDs[ruleID] = struct{}{}
			added = append(added, ruleID)
		}
	}
	return added
}

func placementRetryComponentComplexity(components []placement.Component) map[string]float64 {
	complexity := map[string]float64{}
	for _, component := range components {
		complexity[component.Ref] = float64(len(component.Pads)) + component.Bounds.WidthMM*component.Bounds.HeightMM
	}
	return complexity
}

func placementRetryAnchorAndTargets(refs []string, complexityByRef map[string]float64) (string, []string) {
	ordered := slices.Clone(refs)
	slices.SortFunc(ordered, func(a, b string) int {
		if complexityByRef[a] != complexityByRef[b] {
			return cmp.Compare(complexityByRef[b], complexityByRef[a])
		}
		return cmp.Compare(a, b)
	})
	anchor := ordered[0]
	targets := slices.Clone(ordered[1:])
	slices.Sort(targets)
	return anchor, targets
}

func placementRetryRefsByNet(request placement.Request) map[string][]string {
	refsByNet := map[string][]string{}
	seenByNet := map[string]map[string]struct{}{}
	for _, net := range request.Nets {
		if seenByNet[net.Name] == nil {
			seenByNet[net.Name] = map[string]struct{}{}
		}
		for _, endpoint := range net.Endpoints {
			ref := endpoint.Ref
			if ref == "" {
				continue
			}
			if _, ok := seenByNet[net.Name][ref]; ok {
				continue
			}
			seenByNet[net.Name][ref] = struct{}{}
			refsByNet[net.Name] = append(refsByNet[net.Name], ref)
		}
	}
	for netName := range refsByNet {
		slices.Sort(refsByNet[netName])
	}
	return refsByNet
}

func placementRetryProximityRuleIDs(rules []placement.ProximityRule) map[string]struct{} {
	ids := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		ids[rule.ID] = struct{}{}
	}
	return ids
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

func PlacementRetryAdjustmentSummary(adjustment PlacementRetryAdjustment) string {
	if !adjustment.Applied {
		return "no safe placement retry adjustment applied"
	}
	return fmt.Sprintf("applied placement retry adjustment: spacing +%.2fmm, proximity rules %d", adjustment.SpacingDeltaMM, len(adjustment.ProximityRules))
}
