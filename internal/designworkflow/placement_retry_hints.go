package designworkflow

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

type PlacementRetryHintCategory string

const (
	PlacementRetryReduceDistance  PlacementRetryHintCategory = "reduce_distance"
	PlacementRetryIncreaseSpacing PlacementRetryHintCategory = "increase_spacing"
	PlacementRetryImproveFanout   PlacementRetryHintCategory = "improve_fanout"
	PlacementRetryMoveFromEdge    PlacementRetryHintCategory = "move_from_edge"
	PlacementRetryRelaxRules      PlacementRetryHintCategory = "relax_rules"
	PlacementRetryUnsupported     PlacementRetryHintCategory = "unsupported"
)

type PlacementRetryHint struct {
	Category          PlacementRetryHintCategory `json:"category"`
	SourceCategory    routing.RepairCategory     `json:"source_category"`
	SourceAction      routing.RepairAction       `json:"source_action"`
	Severity          reports.Severity           `json:"severity"`
	Refs              []string                   `json:"refs,omitempty"`
	Nets              []string                   `json:"nets,omitempty"`
	SuggestedAction   string                     `json:"suggested_action"`
	RetryEligible     bool                       `json:"retry_eligible"`
	PlacementEvidence []string                   `json:"placement_evidence,omitempty"`
}

const maxPlacementRetryEvidenceItems = 8

func BuildPlacementRetryHints(diagnostics []routing.RepairDiagnostic, quality *placement.QualityReport) []PlacementRetryHint {
	hints := make([]PlacementRetryHint, 0, len(diagnostics))
	fanoutEvidenceByRef := placementFanoutEvidenceByRef(quality)
	congestionEvidence := placementCongestionEvidence(quality)
	seen := map[string]struct{}{}
	for _, diagnostic := range diagnostics {
		category, eligible := placementRetryHintCategory(diagnostic)
		hint := PlacementRetryHint{
			Category:          category,
			SourceCategory:    diagnostic.Category,
			SourceAction:      diagnostic.Action,
			Severity:          diagnostic.Severity,
			Refs:              sortedStringsCopy(diagnostic.Refs),
			Nets:              sortedStringsCopy(diagnostic.Nets),
			SuggestedAction:   placementRetryHintAction(category),
			RetryEligible:     eligible,
			PlacementEvidence: placementRetryEvidence(diagnostic, category, fanoutEvidenceByRef, congestionEvidence),
		}
		key := placementRetryHintKey(hint)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		hints = append(hints, hint)
	}
	slices.SortFunc(hints, func(a, b PlacementRetryHint) int {
		if compare := cmp.Compare(a.Category, b.Category); compare != 0 {
			return compare
		}
		if compare := cmp.Compare(a.SourceCategory, b.SourceCategory); compare != 0 {
			return compare
		}
		if compare := cmp.Compare(a.SourceAction, b.SourceAction); compare != 0 {
			return compare
		}
		if compare := cmp.Compare(a.Severity, b.Severity); compare != 0 {
			return compare
		}
		if compare := compareBool(a.RetryEligible, b.RetryEligible); compare != 0 {
			return compare
		}
		if compare := slices.Compare(a.Nets, b.Nets); compare != 0 {
			return compare
		}
		return slices.Compare(a.Refs, b.Refs)
	})
	return hints
}

func compareBool(a bool, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return -1
	}
	return 1
}

func placementRetryHintKey(hint PlacementRetryHint) string {
	return strings.Join([]string{
		string(hint.Category),
		string(hint.SourceCategory),
		string(hint.SourceAction),
		string(hint.Severity),
		strings.Join(hint.Refs, ","),
		strings.Join(hint.Nets, ","),
	}, "|")
}

func placementRetryHintCategory(diagnostic routing.RepairDiagnostic) (PlacementRetryHintCategory, bool) {
	switch diagnostic.Category {
	case routing.RepairRouteSearch, routing.RepairClearance:
		return PlacementRetryIncreaseSpacing, true
	case routing.RepairLengthPolicy:
		return PlacementRetryReduceDistance, true
	case routing.RepairPadAccess:
		return PlacementRetryImproveFanout, true
	case routing.RepairBoardBoundary:
		return PlacementRetryMoveFromEdge, true
	case routing.RepairLayerAccess, routing.RepairViaPolicy, routing.RepairRoutingRules:
		return PlacementRetryRelaxRules, false
	case routing.RepairZonePolicy, routing.RepairInputModel, routing.RepairExternalCheck, routing.RepairConnectivity:
		return PlacementRetryUnsupported, false
	default:
		return PlacementRetryUnsupported, false
	}
}

func placementRetryHintAction(category PlacementRetryHintCategory) string {
	switch category {
	case PlacementRetryReduceDistance:
		return "move connected components closer or emphasize proximity for affected nets"
	case PlacementRetryIncreaseSpacing:
		return "increase local spacing around affected refs or congested routing channels"
	case PlacementRetryImproveFanout:
		return "increase escape clearance around affected components"
	case PlacementRetryMoveFromEdge:
		return "move affected refs away from blocked board edges when not fixed"
	case PlacementRetryRelaxRules:
		return "routing rules need review before placement retry is useful"
	default:
		return "manual or upstream repair required before placement retry"
	}
}

func placementFanoutEvidenceByRef(quality *placement.QualityReport) map[string]string {
	evidence := map[string]string{}
	if quality == nil {
		return evidence
	}
	for _, report := range quality.FanoutReports {
		if report.Status == "pass" {
			continue
		}
		evidence[report.Ref] = "fanout:" + report.Ref + ":" + report.Status
	}
	return evidence
}

func placementCongestionEvidence(quality *placement.QualityReport) []string {
	if quality == nil {
		return nil
	}
	reports := slices.Clone(quality.CongestionReports)
	slices.SortFunc(reports, func(a, b placement.CongestionReport) int {
		if a.Utilization != b.Utilization {
			return cmp.Compare(b.Utilization, a.Utilization)
		}
		return cmp.Compare(a.CellID, b.CellID)
	})
	var evidence []string
	for _, report := range reports {
		if report.Status == "pass" {
			continue
		}
		evidence = append(evidence, fmt.Sprintf("congestion:%s:%s:utilization=%.3f", report.CellID, report.Status, report.Utilization))
		if len(evidence) >= maxPlacementRetryEvidenceItems {
			return evidence
		}
	}
	return evidence
}

func placementRetryEvidence(diagnostic routing.RepairDiagnostic, category PlacementRetryHintCategory, fanoutEvidenceByRef map[string]string, congestionEvidence []string) []string {
	if category == PlacementRetryIncreaseSpacing {
		return append([]string(nil), congestionEvidence...)
	}
	if category != PlacementRetryImproveFanout || len(fanoutEvidenceByRef) == 0 {
		return nil
	}
	var evidence []string
	for _, ref := range diagnostic.Refs {
		if item, ok := fanoutEvidenceByRef[ref]; ok {
			evidence = append(evidence, item)
		}
	}
	slices.Sort(evidence)
	if len(evidence) > maxPlacementRetryEvidenceItems {
		return evidence[:maxPlacementRetryEvidenceItems]
	}
	return evidence
}

func sortedStringsCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	slices.Sort(out)
	return out
}
