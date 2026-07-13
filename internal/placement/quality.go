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
	scoreWeightCongestion    = 1.0
	scoreWeightFanout        = 1.0

	routingReadinessThresholdMultiplier = 0.75
	routingReadinessWarningCredit       = 0.5
	fanoutWarningConnectedPads          = 8
	fanoutFailConnectedPads             = 16
	fanoutClearanceRuleMultiplier       = 2.0
	fanoutWarningPressure               = 1.0
	fanoutFailPressure                  = 2.0
	fanoutWarningEscapeDemand           = 1.0
	fanoutFailEscapeDemand              = 1.5
	congestionMaxGridCellsPerAxis       = 200
	congestionMinCellMM                 = 2.5
	congestionMaxEndpointsPerNet        = 128
	congestionWarningUtilization        = 1.0
	congestionFailUtilization           = 2.0
	// Congestion capacity estimates usable coarse routing tracks per cell pitch length.
	congestionMinCellCapacity          = 8.0
	congestionCapacityPerLinearPitchMM = 8.0

	scoreStatusPass    = "pass"
	scoreStatusWarning = "warning"
	scoreStatusFail    = "fail"
	netStatusPass      = scoreStatusPass
	netStatusWarning   = scoreStatusWarning
	netStatusFail      = scoreStatusFail
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
	CongestionReports         []CongestionReport    `json:"congestion_reports,omitempty"`
	FanoutReports             []FanoutReport        `json:"fanout_reports,omitempty"`
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

type CongestionReport struct {
	CellID            string  `json:"cell_id"`
	Bounds            Rect    `json:"bounds"`
	WeightedCrossings float64 `json:"weighted_crossings"`
	EstimatedCapacity float64 `json:"estimated_capacity"`
	Utilization       float64 `json:"utilization"`
	Status            string  `json:"status"`
	Evidence          string  `json:"evidence"`
	SuggestedAction   string  `json:"suggested_action,omitempty"`
}

type FanoutReport struct {
	Ref               string   `json:"ref"`
	PadCount          int      `json:"pad_count"`
	ConnectedPadCount int      `json:"connected_pad_count"`
	LocalNetCount     int      `json:"local_net_count"`
	AvailableSides    []string `json:"available_sides,omitempty"`
	EdgePressure      float64  `json:"edge_pressure"`
	KeepoutPressure   float64  `json:"keepout_pressure"`
	NeighborPressure  float64  `json:"neighbor_pressure"`
	EscapeDemand      float64  `json:"escape_demand"`
	Status            string   `json:"status"`
	Evidence          string   `json:"evidence"`
	SuggestedAction   string   `json:"suggested_action,omitempty"`
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
	edgeTolerance := edgeConstraintTolerance(request.Board, request.Rules)
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
			if edgeConstraintSatisfied(request.Board, component, placement.Position, component.Edge, edgeTolerance) {
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
	report.CongestionReports = congestionReports(request, placementsByRef)
	report.FanoutReports = fanoutReports(request, placementsByRef)
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
	if len(report.CongestionReports) > 0 {
		congestionScore, congestionStatus := congestionScore(report.CongestionReports)
		add(ScoreDimension{Name: "congestion", Score: congestionScore, Weight: scoreWeightCongestion, Status: congestionStatus, Message: "coarse placement congestion estimate"})
	}
	if len(report.FanoutReports) > 0 {
		fanoutScore, fanoutStatus := fanoutScore(report.FanoutReports)
		add(ScoreDimension{Name: "fanout", Score: fanoutScore, Weight: scoreWeightFanout, Status: fanoutStatus, Message: "component escape readiness estimate"})
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

func fanoutReports(request Request, placementsByRef map[string]PlacementResult) []FanoutReport {
	if len(request.Components) == 0 || request.Board.WidthMM <= 0 || request.Board.HeightMM <= 0 {
		return nil
	}
	connectedPinsByRef, localNetsByRef := fanoutConnectivity(request.Nets)
	componentsByRef := componentsByNormalizedRef(request.Components)
	clearance := max(0.5, request.Rules.ComponentSpacingMM*fanoutClearanceRuleMultiplier)
	neighborPressureByRef := fanoutNeighborPressures(request.Board, placementsByRef, clearance)
	reports := make([]FanoutReport, 0, len(placementsByRef))
	for ref, placement := range placementsByRef {
		component, ok := componentsByRef[ref]
		if !ok {
			continue
		}
		padCount := len(component.Pads)
		connectedPadCount := len(connectedPinsByRef[ref])
		if padCount == 0 {
			padCount = connectedPadCount
		}
		localNetCount := len(localNetsByRef[ref])
		if connectedPadCount == 0 && localNetCount == 0 {
			continue
		}
		availableSides := fanoutAvailableSides(request.Board, request.Keepouts, placement, clearance)
		edgePressure := fanoutEdgePressure(request.Board, placement.Bounds, clearance)
		keepoutPressure := fanoutKeepoutPressure(request.Keepouts, placement, clearance)
		neighborPressure := neighborPressureByRef[ref]
		perimeter := max(0.1, 2*(placement.Bounds.WidthMM()+placement.Bounds.HeightMM()))
		escapeDemand := float64(max(connectedPadCount, localNetCount)) / perimeter
		status := scoreStatusPass
		action := ""
		pressure := edgePressure + keepoutPressure + neighborPressure
		if connectedPadCount >= fanoutFailConnectedPads && (len(availableSides) <= 1 || pressure >= fanoutFailPressure || escapeDemand >= fanoutFailEscapeDemand && pressure >= fanoutWarningPressure) {
			status = scoreStatusFail
			action = "increase escape clearance or move component away from blocked sides"
		} else if connectedPadCount >= fanoutWarningConnectedPads && (len(availableSides) <= 2 || pressure >= fanoutWarningPressure || escapeDemand >= fanoutWarningEscapeDemand) {
			status = scoreStatusWarning
			action = "review component fanout spacing before routing"
		}
		if (strings.EqualFold(component.Role, string(IntentConnector)) || component.Edge != EdgeNone) && len(availableSides) >= 1 && status != scoreStatusPass {
			status = scoreStatusPass
			action = ""
		}
		if status == scoreStatusPass && connectedPadCount < fanoutWarningConnectedPads {
			continue
		}
		reports = append(reports, FanoutReport{
			Ref:               component.Ref,
			PadCount:          padCount,
			ConnectedPadCount: connectedPadCount,
			LocalNetCount:     localNetCount,
			AvailableSides:    availableSides,
			EdgePressure:      roundPlacementMetric(edgePressure),
			KeepoutPressure:   roundPlacementMetric(keepoutPressure),
			NeighborPressure:  roundPlacementMetric(neighborPressure),
			EscapeDemand:      roundPlacementMetric(escapeDemand),
			Status:            status,
			Evidence:          "bounds_edge_keepout_neighbor_escape",
			SuggestedAction:   action,
		})
	}
	slices.SortFunc(reports, func(a, b FanoutReport) int {
		return cmp.Compare(a.Ref, b.Ref)
	})
	return reports
}

func fanoutConnectivity(nets []Net) (map[string]map[string]struct{}, map[string]map[string]struct{}) {
	pinsByRef := map[string]map[string]struct{}{}
	netsByRef := map[string]map[string]struct{}{}
	for _, net := range nets {
		for endpointIndex, endpoint := range net.Endpoints {
			ref := normalizeRef(endpoint.Ref)
			if ref == "" {
				continue
			}
			if pinsByRef[ref] == nil {
				pinsByRef[ref] = map[string]struct{}{}
			}
			if netsByRef[ref] == nil {
				netsByRef[ref] = map[string]struct{}{}
			}
			pin := strings.TrimSpace(endpoint.Pin)
			if pin == "" {
				pin = fmt.Sprintf("%s#%d", net.Name, endpointIndex)
			}
			pinsByRef[ref][pin] = struct{}{}
			netsByRef[ref][net.Name] = struct{}{}
		}
	}
	return pinsByRef, netsByRef
}

func fanoutAvailableSides(board BoardPlacementArea, keepouts []Keepout, placement PlacementResult, clearance float64) []string {
	boardMinX := board.Origin.XMM
	boardMinY := board.Origin.YMM
	boardMaxX := board.Origin.XMM + board.WidthMM
	boardMaxY := board.Origin.YMM + board.HeightMM
	candidates := []struct {
		name  string
		space float64
	}{
		{name: "left", space: placement.Bounds.Min.XMM - boardMinX},
		{name: "right", space: boardMaxX - placement.Bounds.Max.XMM},
		{name: "bottom", space: placement.Bounds.Min.YMM - boardMinY},
		{name: "top", space: boardMaxY - placement.Bounds.Max.YMM},
	}
	available := []string{}
	for _, candidate := range candidates {
		if candidate.space < clearance {
			continue
		}
		if fanoutSideBlockedByKeepout(candidate.name, placement, keepouts, clearance) {
			continue
		}
		available = append(available, candidate.name)
	}
	return available
}

func fanoutSideBlockedByKeepout(side string, placement PlacementResult, keepouts []Keepout, clearance float64) bool {
	bounds := placement.Bounds
	placementRef := strings.ToUpper(strings.TrimSpace(placement.Ref))
	probe := expandedRect(bounds, clearance)
	switch side {
	case "left":
		probe.Max.XMM = bounds.Min.XMM
	case "right":
		probe.Min.XMM = bounds.Max.XMM
	case "bottom":
		probe.Max.YMM = bounds.Min.YMM
	case "top":
		probe.Min.YMM = bounds.Max.YMM
	}
	for _, keepout := range keepouts {
		if keepoutExemptsNormalizedRef(keepout, placementRef) {
			continue
		}
		if !keepout.Optional && keepout.Bounds.Intersects(probe) {
			return true
		}
	}
	return false
}

func fanoutEdgePressure(board BoardPlacementArea, bounds Rect, clearance float64) float64 {
	boardMinX := board.Origin.XMM
	boardMinY := board.Origin.YMM
	boardMaxX := board.Origin.XMM + board.WidthMM
	boardMaxY := board.Origin.YMM + board.HeightMM
	pressure := 0.0
	for _, distance := range []float64{bounds.Min.XMM - boardMinX, boardMaxX - bounds.Max.XMM, bounds.Min.YMM - boardMinY, boardMaxY - bounds.Max.YMM} {
		if distance < clearance {
			pressure++
		}
	}
	return pressure
}

func fanoutKeepoutPressure(keepouts []Keepout, placement PlacementResult, clearance float64) float64 {
	pressure := 0.0
	for _, side := range []string{"left", "right", "bottom", "top"} {
		if fanoutSideBlockedByKeepout(side, placement, keepouts, clearance) {
			pressure++
		}
	}
	return pressure
}

func expandedRect(bounds Rect, amount float64) Rect {
	return Rect{
		Min: Point{XMM: bounds.Min.XMM - amount, YMM: bounds.Min.YMM - amount},
		Max: Point{XMM: bounds.Max.XMM + amount, YMM: bounds.Max.YMM + amount},
	}
}

func fanoutNeighborPressures(board BoardPlacementArea, placementsByRef map[string]PlacementResult, clearance float64) map[string]float64 {
	if len(placementsByRef) == 0 || board.WidthMM <= 0 || board.HeightMM <= 0 || clearance <= 0 {
		return nil
	}
	cellSize := max(1, clearance)
	cols := max(1, int(math.Ceil(board.WidthMM/cellSize)))
	rows := max(1, int(math.Ceil(board.HeightMM/cellSize)))
	for cols*rows > congestionMaxGridCellsPerAxis*congestionMaxGridCellsPerAxis {
		cellSize *= 2
		cols = max(1, int(math.Ceil(board.WidthMM/cellSize)))
		rows = max(1, int(math.Ceil(board.HeightMM/cellSize)))
	}
	rangesByRef := map[string]fanoutGridRanges{}
	for ref, placement := range placementsByRef {
		rangesByRef[ref] = fanoutGridRanges{
			bounds: computeFanoutGridRange(board, placement.Bounds, cellSize, cols, rows),
			probe:  computeFanoutGridRange(board, expandedRect(placement.Bounds, clearance), cellSize, cols, rows),
		}
	}
	grid := make([][]string, cols*rows)
	for ref := range placementsByRef {
		cellRange := rangesByRef[ref].bounds
		for row := cellRange.startRow; row <= cellRange.endRow; row++ {
			for col := cellRange.startCol; col <= cellRange.endCol; col++ {
				grid[row*cols+col] = append(grid[row*cols+col], ref)
			}
		}
	}
	pressures := map[string]float64{}
	seen := map[string]struct{}{}
	for ref, placement := range placementsByRef {
		probe := expandedRect(placement.Bounds, clearance)
		cellRange := rangesByRef[ref].probe
		clear(seen)
		for row := cellRange.startRow; row <= cellRange.endRow; row++ {
			for col := cellRange.startCol; col <= cellRange.endCol; col++ {
				for _, otherRef := range grid[row*cols+col] {
					if otherRef == ref {
						continue
					}
					if _, ok := seen[otherRef]; ok {
						continue
					}
					seen[otherRef] = struct{}{}
					if probe.Intersects(placementsByRef[otherRef].Bounds) {
						pressures[ref] += fanoutRelativeNeighborPressure(placement.Bounds, placementsByRef[otherRef].Bounds)
					}
				}
			}
		}
	}
	return pressures
}

type fanoutGridRanges struct {
	bounds fanoutGridRange
	probe  fanoutGridRange
}

type fanoutGridRange struct {
	startCol int
	endCol   int
	startRow int
	endRow   int
}

func computeFanoutGridRange(board BoardPlacementArea, bounds Rect, cellSize float64, cols int, rows int) fanoutGridRange {
	return fanoutGridRange{
		startCol: clampInt(int(math.Floor((bounds.Min.XMM-board.Origin.XMM)/cellSize)), 0, cols-1),
		endCol:   clampInt(int(math.Floor((bounds.Max.XMM-board.Origin.XMM)/cellSize)), 0, cols-1),
		startRow: clampInt(int(math.Floor((bounds.Min.YMM-board.Origin.YMM)/cellSize)), 0, rows-1),
		endRow:   clampInt(int(math.Floor((bounds.Max.YMM-board.Origin.YMM)/cellSize)), 0, rows-1),
	}
}

func fanoutRelativeNeighborPressure(bounds Rect, other Rect) float64 {
	area := max(1e-6, bounds.WidthMM()*bounds.HeightMM())
	otherArea := max(0, other.WidthMM()*other.HeightMM())
	return min(1, otherArea/area)
}

func fanoutScore(reports []FanoutReport) (float64, string) {
	if len(reports) == 0 {
		return 1, scoreStatusPass
	}
	worstScore := 1.0
	status := scoreStatusPass
	for _, report := range reports {
		score := 1.0
		if report.Status == scoreStatusFail {
			score = 0
			status = scoreStatusFail
		} else if report.Status == scoreStatusWarning {
			score = 0.5
			if status != scoreStatusFail {
				status = scoreStatusWarning
			}
		}
		worstScore = min(worstScore, score)
	}
	return worstScore, status
}

type congestionCell struct {
	bounds            Rect
	weightedCrossings float64
}

func congestionReports(request Request, placementsByRef map[string]PlacementResult) []CongestionReport {
	if request.Board.WidthMM <= 0 || request.Board.HeightMM <= 0 || len(request.Nets) == 0 {
		return nil
	}
	cols := congestionAxisCells(request.Board.WidthMM, len(request.Components))
	rows := congestionAxisCells(request.Board.HeightMM, len(request.Components))
	if cols <= 0 || rows <= 0 {
		return nil
	}
	cellWidth := request.Board.WidthMM / float64(cols)
	cellHeight := request.Board.HeightMM / float64(rows)
	cells := make([]*congestionCell, rows*cols)
	for _, net := range request.Nets {
		points := placedEndpointPoints(net, placementsByRef)
		if len(points) < 2 {
			continue
		}
		points = sampleCongestionPoints(points, congestionMaxEndpointsPerNet)
		weight := net.Weight
		if weight <= 0 {
			weight = 1
		}
		if net.Role == NetPower || net.Role == NetGround || net.Role == NetClock || net.Role == NetDifferential {
			weight *= 2
		}
		for _, edge := range congestionMSTEdges(points) {
			horizontalFirstMid := Point{XMM: edge.end.point.XMM, YMM: edge.start.point.YMM}
			verticalFirstMid := Point{XMM: edge.start.point.XMM, YMM: edge.end.point.YMM}
			splitWeight := float64(weight) / 2
			addCongestionSegment(cells, request.Board, cols, rows, cellWidth, cellHeight, edge.start, endpointPoint{point: horizontalFirstMid}, splitWeight)
			addCongestionSegment(cells, request.Board, cols, rows, cellWidth, cellHeight, endpointPoint{point: horizontalFirstMid}, edge.end, splitWeight)
			addCongestionSegment(cells, request.Board, cols, rows, cellWidth, cellHeight, edge.start, endpointPoint{point: verticalFirstMid}, splitWeight)
			addCongestionSegment(cells, request.Board, cols, rows, cellWidth, cellHeight, endpointPoint{point: verticalFirstMid}, edge.end, splitWeight)
		}
	}
	reports := make([]CongestionReport, 0, len(cells))
	for index, cell := range cells {
		if cell == nil {
			continue
		}
		row := index / cols
		col := index % cols
		id := congestionCellID(row, col)
		capacity := congestionCapacity(cell.bounds)
		utilization := 0.0
		if capacity > 0 {
			utilization = cell.weightedCrossings / capacity
		}
		status := scoreStatusPass
		action := ""
		if utilization > congestionFailUtilization {
			status = scoreStatusFail
			action = "spread components or reduce net crossings through this area"
		} else if utilization >= congestionWarningUtilization {
			status = scoreStatusWarning
			action = "review placement density around this area"
		}
		reports = append(reports, CongestionReport{
			CellID:            id,
			Bounds:            cell.bounds,
			WeightedCrossings: roundPlacementMetric(cell.weightedCrossings),
			EstimatedCapacity: roundPlacementMetric(capacity),
			Utilization:       roundPlacementMetric(utilization),
			Status:            status,
			Evidence:          "centerpoint_horizontal_then_vertical",
			SuggestedAction:   action,
		})
	}
	slices.SortFunc(reports, func(a, b CongestionReport) int {
		return cmp.Compare(a.CellID, b.CellID)
	})
	return reports
}

type endpointPoint struct {
	ref   string
	point Point
}

type congestionEdge struct {
	start endpointPoint
	end   endpointPoint
}

func placedEndpointPoints(net Net, placementsByRef map[string]PlacementResult) []endpointPoint {
	points := []endpointPoint{}
	for _, endpoint := range net.Endpoints {
		ref := normalizeRef(endpoint.Ref)
		placement, ok := placementsByRef[ref]
		if !ok {
			continue
		}
		points = append(points, endpointPoint{ref: ref, point: placement.Bounds.Center()})
	}
	slices.SortFunc(points, func(a, b endpointPoint) int {
		return cmp.Compare(a.ref, b.ref)
	})
	return points
}

func sampleCongestionPoints(points []endpointPoint, limit int) []endpointPoint {
	if limit <= 0 || len(points) <= limit {
		return points
	}
	if limit == 1 {
		return points[:1]
	}
	spatial := slices.Clone(points)
	slices.SortFunc(spatial, func(a, b endpointPoint) int {
		if a.point.XMM != b.point.XMM {
			return cmp.Compare(a.point.XMM, b.point.XMM)
		}
		if a.point.YMM != b.point.YMM {
			return cmp.Compare(a.point.YMM, b.point.YMM)
		}
		return cmp.Compare(a.ref, b.ref)
	})
	sampled := make([]endpointPoint, 0, limit)
	lastIndex := -1
	for sampleIndex := 0; sampleIndex < limit; sampleIndex++ {
		pointIndex := int(math.Round(float64(sampleIndex) * float64(len(spatial)-1) / float64(limit-1)))
		if pointIndex == lastIndex {
			continue
		}
		sampled = append(sampled, spatial[pointIndex])
		lastIndex = pointIndex
	}
	return sampled
}

func congestionMSTEdges(points []endpointPoint) []congestionEdge {
	if len(points) < 2 {
		return nil
	}
	used := make([]bool, len(points))
	bestDistance := make([]float64, len(points))
	bestParent := make([]int, len(points))
	for index := range points {
		bestDistance[index] = math.Inf(1)
		bestParent[index] = -1
	}
	used[0] = true
	for index := 1; index < len(points); index++ {
		bestDistance[index] = manhattanDistance(points[0].point, points[index].point)
		bestParent[index] = 0
	}
	edges := make([]congestionEdge, 0, len(points)-1)
	for len(edges) < len(points)-1 {
		bestTo := -1
		for index, isUsed := range used {
			if isUsed || bestParent[index] < 0 {
				continue
			}
			if bestTo < 0 || bestDistance[index] < bestDistance[bestTo] || (bestDistance[index] == bestDistance[bestTo] && congestionEdgeLess(points[bestParent[index]], points[index], points[bestParent[bestTo]], points[bestTo])) {
				bestTo = index
			}
		}
		if bestTo < 0 || bestParent[bestTo] < 0 {
			break
		}
		edges = append(edges, congestionEdge{start: points[bestParent[bestTo]], end: points[bestTo]})
		used[bestTo] = true
		for index, isUsed := range used {
			if isUsed {
				continue
			}
			distance := manhattanDistance(points[bestTo].point, points[index].point)
			if distance < bestDistance[index] || (distance == bestDistance[index] && congestionEdgeLess(points[bestTo], points[index], points[bestParent[index]], points[index])) {
				bestDistance[index] = distance
				bestParent[index] = bestTo
			}
		}
	}
	return edges
}

func congestionEdgeLess(from endpointPoint, to endpointPoint, bestFrom endpointPoint, bestTo endpointPoint) bool {
	if bestFrom.ref == "" && bestTo.ref == "" {
		return true
	}
	if compare := cmp.Compare(from.ref, bestFrom.ref); compare != 0 {
		return compare < 0
	}
	if compare := cmp.Compare(to.ref, bestTo.ref); compare != 0 {
		return compare < 0
	}
	if from.point.XMM != bestFrom.point.XMM {
		return from.point.XMM < bestFrom.point.XMM
	}
	return to.point.XMM < bestTo.point.XMM
}

func manhattanDistance(a Point, b Point) float64 {
	return math.Abs(a.XMM-b.XMM) + math.Abs(a.YMM-b.YMM)
}

func addCongestionSegment(cells []*congestionCell, board BoardPlacementArea, cols int, rows int, cellWidth float64, cellHeight float64, start endpointPoint, end endpointPoint, weight float64) {
	minX := min(start.point.XMM, end.point.XMM)
	maxX := max(start.point.XMM, end.point.XMM)
	minY := min(start.point.YMM, end.point.YMM)
	maxY := max(start.point.YMM, end.point.YMM)
	startCol := clampInt(int(math.Floor((minX-board.Origin.XMM)/cellWidth)), 0, cols-1)
	endCol := clampInt(int(math.Floor((maxX-board.Origin.XMM)/cellWidth)), 0, cols-1)
	startRow := clampInt(int(math.Floor((minY-board.Origin.YMM)/cellHeight)), 0, rows-1)
	endRow := clampInt(int(math.Floor((maxY-board.Origin.YMM)/cellHeight)), 0, rows-1)
	for row := startRow; row <= endRow; row++ {
		for col := startCol; col <= endCol; col++ {
			cellMinX := board.Origin.XMM + float64(col)*cellWidth
			cellMinY := board.Origin.YMM + float64(row)*cellHeight
			cellMaxX := cellMinX + cellWidth
			cellMaxY := cellMinY + cellHeight
			demand := weight * congestionDemandUnits(cellMinX, cellMinY, cellMaxX, cellMaxY, minX, maxX, minY, maxY)
			if demand <= 0 {
				continue
			}
			id := row*cols + col
			cell := cells[id]
			if cell == nil {
				cell = &congestionCell{
					bounds: Rect{
						Min: Point{XMM: cellMinX, YMM: cellMinY},
						Max: Point{XMM: cellMaxX, YMM: cellMaxY},
					},
				}
				cells[id] = cell
			}
			cell.weightedCrossings += demand
		}
	}
}

func congestionDemandUnits(cellMinX float64, cellMinY float64, cellMaxX float64, cellMaxY float64, minX float64, maxX float64, minY float64, maxY float64) float64 {
	xOverlap := max(0, min(maxX, cellMaxX)-max(minX, cellMinX))
	yOverlap := max(0, min(maxY, cellMaxY)-max(minY, cellMinY))
	length := max(xOverlap, yOverlap)
	if length <= 0 {
		return 0
	}
	return max(0.25, length/congestionMinCellMM)
}

func congestionAxisCells(lengthMM float64, componentCount int) int {
	if lengthMM <= 0 {
		return 0
	}
	bySize := int(math.Ceil(lengthMM / congestionMinCellMM))
	byDensity := int(math.Ceil(math.Sqrt(float64(max(1, componentCount))) * 4))
	return clampInt(max(1, min(bySize, byDensity)), 1, congestionMaxGridCellsPerAxis)
}

func congestionCapacity(bounds Rect) float64 {
	return max(congestionMinCellCapacity, (bounds.WidthMM()+bounds.HeightMM())/congestionMinCellMM*congestionCapacityPerLinearPitchMM)
}

func congestionScore(reports []CongestionReport) (float64, string) {
	if len(reports) == 0 {
		return 1, scoreStatusPass
	}
	worstScore := 1.0
	status := scoreStatusPass
	for _, report := range reports {
		score := 1.0
		if report.Status == scoreStatusFail {
			score = 0
			status = scoreStatusFail
		} else if report.Status == scoreStatusWarning {
			score = 0.5
			if status != scoreStatusFail {
				status = scoreStatusWarning
			}
		}
		worstScore = min(worstScore, score)
	}
	return worstScore, status
}

func congestionCellID(row int, col int) string {
	return fmt.Sprintf("r%03d_c%03d", row, col)
}

func clampInt(value int, minValue int, maxValue int) int {
	return max(minValue, min(value, maxValue))
}

func roundPlacementMetric(value float64) float64 {
	return math.Round(value*1000) / 1000
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
			if keepoutExemptsRef(keepout, placement.Ref) {
				continue
			}
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

func edgeConstraintTolerance(board BoardPlacementArea, rules Rules) float64 {
	clearance := max(board.MarginMM, rules.BoardEdgeClearanceMM)
	return clearance + connectorEdgeProximity(rules)
}

func connectorEdgeProximity(rules Rules) float64 {
	if rules.ConnectorEdgeClearanceMM > 0 {
		return rules.ConnectorEdgeClearanceMM
	}
	return DefaultRules().ConnectorEdgeClearanceMM
}

func edgeCandidateInset(board BoardPlacementArea, rules Rules) float64 {
	const generatedBoardCopperEdgeClearanceMM = 0.5
	clearance := max(board.MarginMM, rules.BoardEdgeClearanceMM)
	return min(connectorEdgeProximity(rules), max(0, generatedBoardCopperEdgeClearanceMM-clearance))
}

func edgeConstraintSatisfied(board BoardPlacementArea, component Component, placement Placement, edge EdgeConstraint, toleranceMM float64) bool {
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
