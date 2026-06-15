package placement

import (
	"math"
	"sort"
	"strings"
)

type QualityReport struct {
	Status                   Status               `json:"status"`
	Ready                    bool                 `json:"ready"`
	Metrics                  Metrics              `json:"metrics"`
	EstimatedBoundsRefs      []string             `json:"estimated_bounds_refs,omitempty"`
	FixedRefs                []string             `json:"fixed_refs,omitempty"`
	UnplacedRefs             []string             `json:"unplaced_refs,omitempty"`
	GroupReports             []GroupQualityReport `json:"group_reports,omitempty"`
	EdgeConstraintCount      int                  `json:"edge_constraint_count"`
	EdgeConstraintSatisfied  int                  `json:"edge_constraint_satisfied"`
	SideConstraintCount      int                  `json:"side_constraint_count"`
	SideConstraintSatisfied  int                  `json:"side_constraint_satisfied"`
	KeepoutCount             int                  `json:"keepout_count"`
	GeometryIssueCount       int                  `json:"geometry_issue_count"`
	GroupIssueCount          int                  `json:"group_issue_count"`
	OperationIssueCount      int                  `json:"operation_issue_count"`
	PlacementQualityWarnings []string             `json:"placement_quality_warnings,omitempty"`
}

type GroupQualityReport struct {
	ID               string  `json:"id"`
	PlacedCount      int     `json:"placed_count"`
	RequestedCount   int     `json:"requested_count"`
	KeepTogether     bool    `json:"keep_together"`
	MaxSpreadMM      float64 `json:"max_spread_mm,omitempty"`
	ActualSpreadMM   float64 `json:"actual_spread_mm"`
	SpreadSatisfied  bool    `json:"spread_satisfied"`
	AnchorDistanceMM float64 `json:"anchor_distance_mm,omitempty"`
}

func BuildQualityReport(request Request, result Result) QualityReport {
	request = NormalizeRequest(request)
	successful := successfulPlacementResults(result.Placements)
	validation := ValidateResult(&request, &result)
	placementsByRef := placementResultsByRef(successful)
	componentRefsByGroup := componentRefsByGroupID(request.Components)
	report := QualityReport{
		Status:              result.Status,
		Ready:               validation.Ready,
		Metrics:             result.Metrics,
		KeepoutCount:        len(request.Keepouts),
		GeometryIssueCount:  len(validation.GeometryIssues),
		GroupIssueCount:     len(validation.GroupIssues),
		OperationIssueCount: len(validation.TransactionResult.Issues),
	}
	for _, component := range request.Components {
		ref := normalizeRef(component.Ref)
		placement, placed := placementsByRef[ref]
		if !placed {
			report.UnplacedRefs = append(report.UnplacedRefs, component.Ref)
			continue
		}
		if component.Fixed || placement.Fixed {
			report.FixedRefs = append(report.FixedRefs, component.Ref)
		}
		if estimatedBoundsSource(component.Bounds.Source) {
			report.EstimatedBoundsRefs = append(report.EstimatedBoundsRefs, component.Ref)
		}
		if component.Edge != EdgeNone {
			report.EdgeConstraintCount++
			if edgeConstraintSatisfied(request.Board, component, placement.Position, component.Edge) {
				report.EdgeConstraintSatisfied++
			}
		}
		if component.Side != "" {
			report.SideConstraintCount++
			if sideConstraintSatisfied(component.Side, placement.Position.Layer) {
				report.SideConstraintSatisfied++
			}
		}
	}
	for _, group := range request.Groups {
		report.GroupReports = append(report.GroupReports, groupQualityReport(group, placementsByRef, componentRefsByGroup))
	}
	sort.Strings(report.EstimatedBoundsRefs)
	sort.Strings(report.FixedRefs)
	sort.Strings(report.UnplacedRefs)
	report.PlacementQualityWarnings = placementQualityWarnings(report)
	return report
}

func placementResultsByRef(placements []PlacementResult) map[string]PlacementResult {
	byRef := make(map[string]PlacementResult, len(placements))
	for _, placement := range placements {
		byRef[normalizeRef(placement.Ref)] = placement
	}
	return byRef
}

func componentRefsByGroupID(components []Component) map[string][]string {
	byGroup := map[string][]string{}
	for _, component := range components {
		groupID := strings.ToUpper(strings.TrimSpace(component.GroupID))
		if groupID == "" {
			continue
		}
		byGroup[groupID] = append(byGroup[groupID], normalizeRef(component.Ref))
	}
	return byGroup
}

func groupQualityReport(group Group, placementsByRef map[string]PlacementResult, componentRefsByGroup map[string][]string) GroupQualityReport {
	members := normalizedGroupMembers(group, componentRefsByGroup)
	centers := make([]Point, 0, len(members))
	for _, member := range members {
		if placement, ok := placementsByRef[member]; ok {
			centers = append(centers, placement.Bounds.Center())
		}
	}
	report := GroupQualityReport{
		ID:             group.ID,
		PlacedCount:    len(centers),
		RequestedCount: len(members),
		KeepTogether:   group.KeepTogether,
		MaxSpreadMM:    group.MaxSpreadMM,
	}
	spread := maxPointSpread(centers)
	report.ActualSpreadMM = spread
	report.SpreadSatisfied = group.MaxSpreadMM <= 0 || spread <= group.MaxSpreadMM
	if group.Anchor.At != nil && len(centers) > 0 {
		center := centroid(centers)
		report.AnchorDistanceMM = boardDistance(center.XMM-group.Anchor.At.XMM, center.YMM-group.Anchor.At.YMM)
	}
	return report
}

func normalizedGroupMembers(group Group, componentRefsByGroup map[string][]string) []string {
	seen := map[string]struct{}{}
	members := make([]string, 0, len(group.Components))
	for _, ref := range group.Components {
		normalized := normalizeRef(ref)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		members = append(members, normalized)
	}
	for _, ref := range componentRefsByGroup[strings.ToUpper(strings.TrimSpace(group.ID))] {
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		members = append(members, ref)
	}
	sort.Strings(members)
	return members
}

func maxPointSpread(points []Point) float64 {
	var spread float64
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			spread = math.Max(spread, boardDistance(points[i].XMM-points[j].XMM, points[i].YMM-points[j].YMM))
		}
	}
	return spread
}

func centroid(points []Point) Point {
	if len(points) == 0 {
		return Point{}
	}
	var center Point
	for _, point := range points {
		center.XMM += point.XMM
		center.YMM += point.YMM
	}
	center.XMM /= float64(len(points))
	center.YMM /= float64(len(points))
	return center
}

func edgeConstraintSatisfied(board BoardPlacementArea, component Component, placement Placement, edge EdgeConstraint) bool {
	toleranceMM := math.Min(2.0, math.Min(board.WidthMM, board.HeightMM)*0.1)
	bounds, ok := ComponentPhysicalBounds(component, placement)
	if !ok {
		return false
	}
	switch edge {
	case EdgeLeft:
		return withinEdgeTolerance(bounds.Min.XMM-board.Origin.XMM, toleranceMM)
	case EdgeRight:
		return withinEdgeTolerance(board.Origin.XMM+board.WidthMM-bounds.Max.XMM, toleranceMM)
	case EdgeTop:
		return withinEdgeTolerance(bounds.Min.YMM-board.Origin.YMM, toleranceMM)
	case EdgeBottom:
		return withinEdgeTolerance(board.Origin.YMM+board.HeightMM-bounds.Max.YMM, toleranceMM)
	case EdgeAny:
		left := bounds.Min.XMM - board.Origin.XMM
		right := board.Origin.XMM + board.WidthMM - bounds.Max.XMM
		top := bounds.Min.YMM - board.Origin.YMM
		bottom := board.Origin.YMM + board.HeightMM - bounds.Max.YMM
		return withinEdgeTolerance(math.Min(math.Min(left, right), math.Min(top, bottom)), toleranceMM)
	default:
		return true
	}
}

func withinEdgeTolerance(distanceMM float64, toleranceMM float64) bool {
	return distanceMM >= 0 && distanceMM <= toleranceMM
}

func sideConstraintSatisfied(side SideConstraint, layer string) bool {
	layer = normalizeLayer(layer)
	switch side {
	case SideBottom:
		return strings.EqualFold(layer, "B.Cu")
	case SideTop:
		return strings.EqualFold(layer, "F.Cu")
	default:
		return true
	}
}

func placementQualityWarnings(report QualityReport) []string {
	warnings := []string{}
	if len(report.EstimatedBoundsRefs) > 0 {
		warnings = append(warnings, "placement uses estimated or pad-derived bounds for "+strings.Join(report.EstimatedBoundsRefs, ", "))
	}
	if len(report.UnplacedRefs) > 0 {
		warnings = append(warnings, "placement left components unplaced: "+strings.Join(report.UnplacedRefs, ", "))
	}
	if report.EdgeConstraintCount > report.EdgeConstraintSatisfied {
		warnings = append(warnings, "one or more edge constraints were not satisfied")
	}
	if report.SideConstraintCount > report.SideConstraintSatisfied {
		warnings = append(warnings, "one or more side constraints were not satisfied")
	}
	for _, group := range report.GroupReports {
		if group.MaxSpreadMM > 0 && !group.SpreadSatisfied {
			warnings = append(warnings, "group "+group.ID+" exceeds max spread")
		}
	}
	return warnings
}
