package placement

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

const (
	scoreWeightGroupCohesion = 1.0
	scoreWeightProximity     = 1.0
	scoreWeightEdge          = 1.0
	scoreWeightMechanical    = 1.0
	scoreWeightRegion        = 1.0
	scoreWeightRouting       = 1.0

	routingReadinessThresholdMultiplier = 0.75
	routingReadinessWarningCredit       = 0.5

	netStatusPass    = "pass"
	netStatusWarning = "warning"
	netStatusFail    = "fail"
)

type QualityReport struct {
	Status                    Status                `json:"status"`
	Ready                     bool                  `json:"ready"`
	Metrics                   Metrics               `json:"metrics"`
	EstimatedBoundsRefs       []string              `json:"estimated_bounds_refs,omitempty"`
	FixedRefs                 []string              `json:"fixed_refs,omitempty"`
	UnplacedRefs              []string              `json:"unplaced_refs,omitempty"`
	GroupReports              []GroupQualityReport  `json:"group_reports,omitempty"`
	ProximityReports          []ProximityReport     `json:"proximity_reports,omitempty"`
	RegionReports             []RegionReport        `json:"region_reports,omitempty"`
	NetReports                []NetQualityReport    `json:"net_reports,omitempty"`
	KeepoutReports            []KeepoutReport       `json:"keepout_reports,omitempty"`
	Score                     ScoreReport           `json:"score,omitempty"`
	EdgeConstraintCount       int                   `json:"edge_constraint_count"`
	EdgeConstraintSatisfied   int                   `json:"edge_constraint_satisfied"`
	SideConstraintCount       int                   `json:"side_constraint_count"`
	SideConstraintSatisfied   int                   `json:"side_constraint_satisfied"`
	KeepoutCount              int                   `json:"keepout_count"`
	RequiredKeepoutViolations int                   `json:"required_keepout_violations"`
	OptionalKeepoutViolations int                   `json:"optional_keepout_violations"`
	GeometryIssueCount        int                   `json:"geometry_issue_count"`
	GroupIssueCount           int                   `json:"group_issue_count"`
	OperationIssueCount       int                   `json:"operation_issue_count"`
	PlacementQualityWarnings  []string              `json:"placement_quality_warnings,omitempty"`
	Diagnostics               []PlacementDiagnostic `json:"diagnostics,omitempty"`
}

type ProximityReport struct {
	ID            string   `json:"id"`
	Source        string   `json:"source,omitempty"`
	Role          string   `json:"role,omitempty"`
	AnchorRef     string   `json:"anchor_ref"`
	TargetRefs    []string `json:"target_refs,omitempty"`
	MaxDistanceMM float64  `json:"max_distance_mm,omitempty"`
	ActualMM      *float64 `json:"actual_mm,omitempty"`
	Satisfied     bool     `json:"satisfied"`
	Required      bool     `json:"required"`
	Evidence      string   `json:"evidence"`
}

type RegionReport struct {
	ID             string   `json:"id"`
	Source         string   `json:"source,omitempty"`
	Region         string   `json:"region"`
	Refs           []string `json:"refs,omitempty"`
	NetRoles       []string `json:"net_roles,omitempty"`
	Preferred      Rect     `json:"preferred,omitempty"`
	PlacedCount    int      `json:"placed_count"`
	RequestedCount int      `json:"requested_count"`
	OutsideRefs    []string `json:"outside_refs,omitempty"`
	MissingRefs    []string `json:"missing_refs,omitempty"`
	Satisfied      bool     `json:"satisfied"`
	Required       bool     `json:"required"`
}

type NetQualityReport struct {
	Name                string  `json:"name"`
	Role                string  `json:"role,omitempty"`
	EndpointCount       int     `json:"endpoint_count"`
	PlacedEndpointCount int     `json:"placed_endpoint_count"`
	Weight              int     `json:"weight"`
	HPWLMM              float64 `json:"hpwl_mm"`
	WeightedHPWLMM      float64 `json:"weighted_hpwl_mm"`
	Status              string  `json:"status"`
	Message             string  `json:"message,omitempty"`
}

type KeepoutReport struct {
	ID       string   `json:"id,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Optional bool     `json:"optional,omitempty"`
	Refs     []string `json:"refs,omitempty"`
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
	keepoutReports := keepoutViolationReports(request.Keepouts, successful)
	requiredKeepoutViolations, optionalKeepoutViolations := keepoutViolationCounts(keepoutReports)
	report := QualityReport{
		Status:                    result.Status,
		Ready:                     validation.Ready,
		Metrics:                   result.Metrics,
		KeepoutCount:              len(request.Keepouts),
		RequiredKeepoutViolations: requiredKeepoutViolations,
		OptionalKeepoutViolations: optionalKeepoutViolations,
		GeometryIssueCount:        len(validation.GeometryIssues),
		GroupIssueCount:           len(validation.GroupIssues),
		OperationIssueCount:       len(validation.TransactionResult.Issues),
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
	report.ProximityReports = proximityReports(request, placementsByRef)
	report.RegionReports = regionReports(request, placementsByRef)
	report.NetReports = netQualityReports(request, placementsByRef)
	report.KeepoutReports = keepoutReports
	report.Score = placementScoreReport(report)
	sort.Strings(report.EstimatedBoundsRefs)
	sort.Strings(report.FixedRefs)
	sort.Strings(report.UnplacedRefs)
	report.PlacementQualityWarnings = placementQualityWarnings(report)
	report.Diagnostics = DiagnosticsForQuality(request, result, report)
	return report
}

func proximityReports(request Request, placementsByRef map[string]PlacementResult) []ProximityReport {
	componentsByRef := componentsByNormalizedRef(request.Components)
	reports := make([]ProximityReport, 0, len(request.ProximityRules))
	for _, rule := range request.ProximityRules {
		anchorRef := normalizeRef(rule.AnchorRef)
		anchorPlacement, ok := placementsByRef[anchorRef]
		if !ok {
			reports = append(reports, ProximityReport{
				ID:            rule.ID,
				Source:        rule.Source,
				Role:          string(rule.Role),
				AnchorRef:     rule.AnchorRef,
				TargetRefs:    append([]string(nil), rule.TargetRefs...),
				MaxDistanceMM: rule.MaxDistanceMM,
				Required:      rule.Required,
				Evidence:      "missing_anchor",
			})
			continue
		}
		anchorComponent := componentsByRef[anchorRef]
		report := ProximityReport{
			ID:            rule.ID,
			Source:        rule.Source,
			Role:          string(rule.Role),
			AnchorRef:     rule.AnchorRef,
			TargetRefs:    append([]string(nil), rule.TargetRefs...),
			MaxDistanceMM: rule.MaxDistanceMM,
			Required:      rule.Required,
			Evidence:      "center",
		}
		bestDistance := math.Inf(1)
		for _, targetRefRaw := range rule.TargetRefs {
			targetRef := normalizeRef(targetRefRaw)
			targetPlacement, ok := placementsByRef[targetRef]
			if !ok {
				continue
			}
			targetComponent := componentsByRef[targetRef]
			distance, evidence := proximityDistance(anchorComponent, anchorPlacement, rule.AnchorPins, targetComponent, targetPlacement, rule.TargetPins)
			if distance < bestDistance {
				bestDistance = distance
				report.Evidence = evidence
			}
		}
		if math.IsInf(bestDistance, 1) {
			report.Evidence = "missing_target"
		} else {
			report.ActualMM = &bestDistance
		}
		report.Satisfied = report.ActualMM != nil && (rule.MaxDistanceMM <= 0 || *report.ActualMM <= rule.MaxDistanceMM)
		reports = append(reports, report)
	}
	sort.SliceStable(reports, func(i, j int) bool {
		return reports[i].ID < reports[j].ID
	})
	return reports
}

func regionReports(request Request, placementsByRef map[string]PlacementResult) []RegionReport {
	if len(request.RegionRules) == 0 {
		return nil
	}
	reports := make([]RegionReport, 0, len(request.RegionRules))
	refsByRole := map[NetRole][]string{}
	if regionRulesUseNetRoles(request.RegionRules) {
		refsByRole = netRefsByRole(request.Nets)
	}
	for _, rule := range request.RegionRules {
		refs := regionRuleRefs(rule, refsByRole)
		report := RegionReport{
			ID:             rule.ID,
			Source:         rule.Source,
			Region:         rule.Region,
			Refs:           refs,
			NetRoles:       netRoleStrings(rule.NetRoles),
			Preferred:      rule.Preferred,
			RequestedCount: len(refs),
			Required:       rule.Required,
			Satisfied:      true,
		}
		if len(refs) == 0 {
			reports = append(reports, report)
			continue
		}
		for _, ref := range refs {
			placement, ok := placementsByRef[ref]
			if !ok {
				report.MissingRefs = append(report.MissingRefs, ref)
				report.Satisfied = false
				continue
			}
			report.PlacedCount++
			if !rule.Preferred.IsZero() && !placementSatisfiesRegion(placement, rule.Preferred) {
				report.OutsideRefs = append(report.OutsideRefs, ref)
				report.Satisfied = false
			}
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].ID < reports[j].ID
	})
	return reports
}

func placementSatisfiesRegion(placement PlacementResult, preferred Rect) bool {
	return preferred.Contains(placement.Bounds)
}

func regionRuleRefs(rule RegionRule, refsByRole map[NetRole][]string) []string {
	seen := map[string]struct{}{}
	refs := make([]string, 0, len(rule.Refs))
	for _, ref := range rule.Refs {
		refs = addNormalizedRef(normalizeRef(ref), seen, refs)
	}
	for _, role := range rule.NetRoles {
		for _, ref := range refsByRole[role] {
			refs = addNormalizedRef(ref, seen, refs)
		}
	}
	sort.Strings(refs)
	return refs
}

func addNormalizedRef(normalized string, seen map[string]struct{}, refs []string) []string {
	if normalized == "" {
		return refs
	}
	if _, ok := seen[normalized]; ok {
		return refs
	}
	seen[normalized] = struct{}{}
	return append(refs, normalized)
}

func netRefsByRole(nets []Net) map[NetRole][]string {
	seenByRole := map[NetRole]map[string]struct{}{}
	refsByRole := map[NetRole][]string{}
	for _, net := range nets {
		if net.Role == "" || net.Role == NetUnknown {
			continue
		}
		for _, endpoint := range net.Endpoints {
			ref := normalizeRef(endpoint.Ref)
			if ref == "" {
				continue
			}
			if seenByRole[net.Role] == nil {
				seenByRole[net.Role] = map[string]struct{}{}
			}
			if _, ok := seenByRole[net.Role][ref]; ok {
				continue
			}
			seenByRole[net.Role][ref] = struct{}{}
			refsByRole[net.Role] = append(refsByRole[net.Role], ref)
		}
	}
	return refsByRole
}

func regionRulesUseNetRoles(rules []RegionRule) bool {
	for _, rule := range rules {
		if len(rule.NetRoles) > 0 {
			return true
		}
	}
	return false
}

func netRoleStrings(roles []NetRole) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		value := string(role)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func netQualityReports(request Request, placementsByRef map[string]PlacementResult) []NetQualityReport {
	reports := make([]NetQualityReport, 0, len(request.Nets))
	longThreshold := routingReadinessLongThreshold(request.Board)
	for _, net := range request.Nets {
		if len(net.Endpoints) < 2 {
			continue
		}
		weight := net.Weight
		if weight <= 0 {
			weight = 1
		}
		report := NetQualityReport{
			Name:          net.Name,
			Role:          string(net.Role),
			EndpointCount: len(net.Endpoints),
			Weight:        weight,
			Status:        netStatusPass,
		}
		minX, minY := 0.0, 0.0
		maxX, maxY := 0.0, 0.0
		for _, endpoint := range net.Endpoints {
			ref := normalizeRef(endpoint.Ref)
			if ref == "" {
				continue
			}
			placement, ok := placementsByRef[ref]
			if !ok {
				continue
			}
			point := placement.Bounds.Center()
			if report.PlacedEndpointCount == 0 {
				minX, maxX = point.XMM, point.XMM
				minY, maxY = point.YMM, point.YMM
			} else {
				minX = min(minX, point.XMM)
				maxX = max(maxX, point.XMM)
				minY = min(minY, point.YMM)
				maxY = max(maxY, point.YMM)
			}
			report.PlacedEndpointCount++
		}
		if report.PlacedEndpointCount < report.EndpointCount {
			report.Status = netStatusFail
			report.Message = "one or more net endpoints are unplaced"
		}
		if report.PlacedEndpointCount > 1 {
			report.HPWLMM = (maxX - minX) + (maxY - minY)
			report.WeightedHPWLMM = float64(weight) * report.HPWLMM
			if report.Status == netStatusPass && longThreshold > 0 && report.HPWLMM > longThreshold {
				report.Status = netStatusWarning
				report.Message = "net HPWL exceeds routing-readiness threshold"
			}
		}
		reports = append(reports, report)
	}
	slices.SortFunc(reports, func(a, b NetQualityReport) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return reports
}

func routingReadinessLongThreshold(board BoardPlacementArea) float64 {
	if board.WidthMM <= 0 || board.HeightMM <= 0 {
		return 0
	}
	return max(board.WidthMM, board.HeightMM) * routingReadinessThresholdMultiplier
}

func componentsByNormalizedRef(components []Component) map[string]Component {
	byRef := make(map[string]Component, len(components))
	for _, component := range components {
		byRef[normalizeRef(component.Ref)] = component
	}
	return byRef
}

func proximityDistance(anchor Component, anchorPlacement PlacementResult, anchorPins []string, target Component, targetPlacement PlacementResult, targetPins []string) (float64, string) {
	anchorPoint, anchorEvidence := proximityPoint(anchor, anchorPlacement, anchorPins)
	targetPoint, targetEvidence := proximityPoint(target, targetPlacement, targetPins)
	evidence := "center"
	if anchorEvidence == "pad" && targetEvidence == "pad" {
		evidence = "pad"
	}
	return boardDistance(anchorPoint.XMM-targetPoint.XMM, anchorPoint.YMM-targetPoint.YMM), evidence
}

func proximityPoint(component Component, placement PlacementResult, pins []string) (Point, string) {
	padsByName := map[string]PadSummary{}
	for _, pad := range component.Pads {
		padsByName[strings.ToUpper(strings.TrimSpace(pad.Name))] = pad
	}
	for _, pin := range pins {
		pad, ok := padsByName[strings.ToUpper(strings.TrimSpace(pin))]
		if ok {
			rotated := rotatePoint(Point{XMM: pad.XMM, YMM: pad.YMM}, placement.Position.RotationDeg)
			return Point{XMM: placement.Position.XMM + rotated.XMM, YMM: placement.Position.YMM + rotated.YMM}, "pad"
		}
	}
	return placement.Bounds.Center(), "center"
}

func placementScoreReport(report QualityReport) ScoreReport {
	score := ScoreReport{}
	add := func(dimension ScoreDimension) {
		score.Dimensions = append(score.Dimensions, dimension)
		score.Total += dimension.Score * dimension.Weight
	}
	groupScore := 1.0
	groupStatus := "pass"
	for _, group := range report.GroupReports {
		if group.RequestedCount > 0 && group.PlacedCount < group.RequestedCount {
			groupScore = 0
			groupStatus = "fail"
			break
		}
		if group.MaxSpreadMM > 0 && !group.SpreadSatisfied {
			groupScore = 0
			groupStatus = "fail"
			break
		}
	}
	if len(report.GroupReports) > 0 {
		add(ScoreDimension{Name: "group_cohesion", Score: groupScore, Weight: scoreWeightGroupCohesion, Status: groupStatus, Message: "placement group cohesion"})
	}
	if report.EdgeConstraintCount > 0 {
		edgeScore := float64(report.EdgeConstraintSatisfied) / float64(report.EdgeConstraintCount)
		edgeStatus := "pass"
		if report.EdgeConstraintSatisfied < report.EdgeConstraintCount {
			edgeStatus = "fail"
		}
		add(ScoreDimension{Name: "edge_constraints", Score: edgeScore, Weight: scoreWeightEdge, Status: edgeStatus, Message: "edge placement constraint satisfaction"})
	}
	if report.KeepoutCount > 0 {
		mechanicalScore := 1.0
		mechanicalStatus := "pass"
		if report.RequiredKeepoutViolations > 0 {
			mechanicalScore = 0
			mechanicalStatus = "fail"
		} else if report.OptionalKeepoutViolations > 0 {
			mechanicalScore = 0.5
			mechanicalStatus = "warning"
		}
		add(ScoreDimension{Name: "mechanical", Score: mechanicalScore, Weight: scoreWeightMechanical, Status: mechanicalStatus, Message: "mechanical keepout and board-fit satisfaction"})
	}
	regionSatisfied := 0
	regionStatus := "pass"
	for _, region := range report.RegionReports {
		if region.Satisfied {
			regionSatisfied++
			continue
		}
		if region.Required {
			regionStatus = "fail"
		} else if regionStatus != "fail" {
			regionStatus = "warning"
		}
	}
	if len(report.RegionReports) > 0 {
		regionScore := float64(regionSatisfied) / float64(len(report.RegionReports))
		add(ScoreDimension{Name: "regions", Score: regionScore, Weight: scoreWeightRegion, Status: regionStatus, Message: "placement region preference satisfaction"})
	}
	if len(report.NetReports) > 0 {
		routingScore, routingStatus := routingReadinessScore(report.NetReports)
		add(ScoreDimension{Name: "routing_readiness", Score: routingScore, Weight: scoreWeightRouting, Status: routingStatus, Message: "placement net HPWL and endpoint readiness"})
	}
	proximityScore := 1.0
	proximityStatus := "pass"
	for _, proximity := range report.ProximityReports {
		if !proximity.Satisfied {
			proximityScore = 0
			if proximity.Required {
				proximityStatus = "fail"
				break
			}
			proximityStatus = "warning"
		}
	}
	if len(report.ProximityReports) > 0 {
		add(ScoreDimension{Name: "proximity", Score: proximityScore, Weight: scoreWeightProximity, Status: proximityStatus, Message: "electrical proximity rule satisfaction"})
	}
	return score
}

func placementResultsByRef(placements []PlacementResult) map[string]PlacementResult {
	byRef := make(map[string]PlacementResult, len(placements))
	for _, placement := range placements {
		byRef[normalizeRef(placement.Ref)] = placement
	}
	return byRef
}

func routingReadinessScore(reports []NetQualityReport) (float64, string) {
	if len(reports) == 0 {
		return 1, netStatusPass
	}
	score := 0.0
	var totalWeight int64
	status := netStatusPass
	for _, report := range reports {
		weight := report.Weight
		totalWeight += int64(weight)
		switch report.Status {
		case netStatusPass:
			score += float64(weight)
		case netStatusWarning:
			score += float64(weight) * routingReadinessWarningCredit
			if status != netStatusFail {
				status = netStatusWarning
			}
		case netStatusFail:
			status = netStatusFail
		}
	}
	if totalWeight == 0 {
		return 1, status
	}
	return score / float64(totalWeight), status
}

func keepoutViolationReports(keepouts []Keepout, placements []PlacementResult) []KeepoutReport {
	reports := []KeepoutReport{}
	for _, keepout := range keepouts {
		report := KeepoutReport{ID: keepout.ID, Reason: keepout.Reason, Optional: keepout.Optional}
		for _, placement := range placements {
			if keepoutAppliesToLayer(keepout, placement.Position.Layer) && keepout.Bounds.Intersects(placement.Bounds) {
				report.Refs = append(report.Refs, placement.Ref)
			}
		}
		if len(report.Refs) > 0 {
			sort.Strings(report.Refs)
			reports = append(reports, report)
		}
	}
	slices.SortFunc(reports, func(a, b KeepoutReport) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return reports
}

func keepoutViolationCounts(reports []KeepoutReport) (required int, optional int) {
	for _, report := range reports {
		if report.Optional {
			optional++
		} else {
			required++
		}
	}
	return required, optional
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
	if report.OptionalKeepoutViolations > 0 {
		warnings = append(warnings, "one or more optional keepouts were occupied")
	}
	for _, region := range report.RegionReports {
		if !region.Satisfied {
			if region.Required {
				warnings = append(warnings, "region "+region.ID+" required placement was not satisfied")
			} else {
				warnings = append(warnings, "region "+region.ID+" placement preference was not satisfied")
			}
		}
	}
	failedNets, warningNets := routingReadinessIssueCounts(report.NetReports)
	if failedNets > 0 {
		warnings = append(warnings, fmt.Sprintf("%d nets have failed routing readiness", failedNets))
	}
	if warningNets > 0 {
		warnings = append(warnings, fmt.Sprintf("%d nets have routing-readiness warnings", warningNets))
	}
	for _, group := range report.GroupReports {
		if group.MaxSpreadMM > 0 && !group.SpreadSatisfied {
			warnings = append(warnings, "group "+group.ID+" exceeds max spread")
		}
	}
	return warnings
}

func routingReadinessIssueCounts(reports []NetQualityReport) (failed int, warning int) {
	for _, report := range reports {
		switch report.Status {
		case netStatusFail:
			failed++
		case netStatusWarning:
			warning++
		}
	}
	return failed, warning
}
