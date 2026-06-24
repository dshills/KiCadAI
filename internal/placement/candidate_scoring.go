package placement

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const (
	defaultCandidateScoringPolicy                 = "semantic_v1"
	defaultCandidateScoringVersion                = "candidate-scoring.v1"
	defaultCandidateAlternativesPerComponent      = 3
	defaultCandidateScoreEvidencePerDimension     = 4
	maxCandidateAlternativesPerComponent          = 32
	maxCandidateScoreEvidencePerDimension         = 16
	maxCandidateRejectedSamplesPerComponent       = 64
	candidateScoreFloatPrecision              int = 1000
	candidateScoreMaxEvidenceLength           int = 160
	candidatePlacementCompareEpsilon              = 1e-9
	candidateCongestionPlacedSaturationCount      = 100.0
	candidateFanoutSideCount                      = 4.0
	candidateFanoutPadPressureLimit               = 32.0
	candidateCongestionCellMM                     = 10.0
)

func normalizeCandidateScoringRules(rules CandidateScoringRules) CandidateScoringRules {
	rules.Policy = strings.TrimSpace(rules.Policy)
	if rules.Policy == "" {
		rules.Policy = defaultCandidateScoringPolicy
	}
	if rules.MaxAlternativesPerComponent < 0 {
		rules.MaxAlternativesPerComponent = defaultCandidateAlternativesPerComponent
	}
	if rules.MaxAlternativesPerComponent > maxCandidateAlternativesPerComponent {
		rules.MaxAlternativesPerComponent = maxCandidateAlternativesPerComponent
	}
	if rules.MaxEvidencePerDimension < 0 {
		rules.MaxEvidencePerDimension = defaultCandidateScoreEvidencePerDimension
	}
	if rules.MaxEvidencePerDimension > maxCandidateScoreEvidencePerDimension {
		rules.MaxEvidencePerDimension = maxCandidateScoreEvidencePerDimension
	}
	rules.Weights = normalizeCandidateScoreWeights(rules.Weights)
	return rules
}

func normalizeCandidateScoreWeights(weights CandidateScoreWeights) CandidateScoreWeights {
	weights.HardConstraints = normalizeCandidateScoreWeight(weights.HardConstraints)
	weights.SemanticRole = normalizeCandidateScoreWeight(weights.SemanticRole)
	weights.GroupCohesion = normalizeCandidateScoreWeight(weights.GroupCohesion)
	weights.ElectricalProximity = normalizeCandidateScoreWeight(weights.ElectricalProximity)
	weights.RouteLength = normalizeCandidateScoreWeight(weights.RouteLength)
	weights.Congestion = normalizeCandidateScoreWeight(weights.Congestion)
	weights.Fanout = normalizeCandidateScoreWeight(weights.Fanout)
	weights.Edge = normalizeCandidateScoreWeight(weights.Edge)
	weights.Region = normalizeCandidateScoreWeight(weights.Region)
	weights.Mobility = normalizeCandidateScoreWeight(weights.Mobility)
	weights.Thermal = normalizeCandidateScoreWeight(weights.Thermal)
	weights.HighCurrent = normalizeCandidateScoreWeight(weights.HighCurrent)
	weights.CreepageClearance = normalizeCandidateScoreWeight(weights.CreepageClearance)
	return weights
}

func normalizeCandidateScoreWeight(weight float64) float64 {
	if weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
		return 0
	}
	return weight
}

func NewCandidateScoringReport(rules CandidateScoringRules) *CandidateScoringReport {
	rules = normalizeCandidateScoringRules(rules)
	if !rules.Enabled {
		return nil
	}
	return &CandidateScoringReport{
		Enabled:      true,
		Policy:       rules.Policy,
		ScoreVersion: defaultCandidateScoringVersion,
	}
}

func NormalizeCandidateScoringReport(report CandidateScoringReport, rules CandidateScoringRules) CandidateScoringReport {
	rules = normalizeCandidateScoringRules(rules)
	report.Policy = strings.TrimSpace(report.Policy)
	if report.Policy == "" {
		report.Policy = rules.Policy
	}
	report.ScoreVersion = strings.TrimSpace(report.ScoreVersion)
	if report.ScoreVersion == "" {
		report.ScoreVersion = defaultCandidateScoringVersion
	}
	report.AverageWinningScore = roundCandidateScore(report.AverageWinningScore)
	report.LowestWinningScore = roundCandidateScore(report.LowestWinningScore)
	report.AggregateDimensions = normalizeCandidateScoreDimensions(report.AggregateDimensions, rules.MaxEvidencePerDimension)
	report.TopPenalties = normalizeCandidateScoreDimensions(report.TopPenalties, rules.MaxEvidencePerDimension)
	report.WinningCandidates = normalizeCandidateScores(report.WinningCandidates, rules.MaxEvidencePerDimension)
	if len(report.WinningCandidates) > 0 {
		report.AverageWinningScore, report.LowestWinningScore = candidateWinningScoreSummary(report.WinningCandidates)
	}
	report.AlternativeCandidates = normalizeCandidateScores(report.AlternativeCandidates, rules.MaxEvidencePerDimension)
	report.AlternativeCandidates = limitCandidateScoresPerRef(report.AlternativeCandidates, rules.MaxAlternativesPerComponent)
	report.RejectedByReason = normalizeCandidateReasonCounts(report.RejectedByReason)
	return report
}

func candidateWinningScoreSummary(scores []CandidateScore) (average float64, lowest float64) {
	if len(scores) == 0 {
		return 0, 0
	}
	lowest = scores[0].Total
	total := lowest
	for _, score := range scores[1:] {
		if score.Total < lowest {
			lowest = score.Total
		}
		total += score.Total
	}
	return roundCandidateScore(total / float64(len(scores))), roundCandidateScore(lowest)
}

func recordCandidateRejection(report *CandidateScoringReport, component Component, refKey string, placement Placement, index int, reason CandidateRejectionReasonName, message string, refs ...string) {
	if report == nil {
		return
	}
	if report.RejectedByReason == nil {
		report.RejectedByReason = map[string]int{}
	}
	if report.rejectedSamplesByRef == nil {
		report.rejectedSamplesByRef = map[string]int{}
	}
	reasonKey := string(reason)
	if reasonKey == "" {
		reasonKey = string(CandidateRejectUnsupportedPolicy)
	}
	report.RejectedByReason[reasonKey]++
	if refKey == "" {
		refKey = component.Ref
	}
	if report.rejectedSamplesByRef[refKey] >= maxCandidateRejectedSamplesPerComponent {
		return
	}
	report.rejectedSamplesByRef[refKey]++
	report.AlternativeCandidates = append(report.AlternativeCandidates, CandidateScore{
		Ref:       component.Ref,
		Role:      component.Role,
		Index:     index,
		Placement: normalizePlacementLayer(placement),
		Rejected:  true,
		Total:     0,
		Reasons: []CandidateRejectionReason{{
			Name:     reason,
			Severity: reports.SeverityError,
			Message:  message,
			Refs:     refs,
		}},
		Evidence: []string{message},
	})
}

func recordCandidateWinner(report *CandidateScoringReport, component Component, placement PlacementResult, candidate placementCandidate) {
	if report == nil {
		return
	}
	report.WinningCandidates = append(report.WinningCandidates, CandidateScore{
		Ref:        component.Ref,
		Role:       component.Role,
		Index:      candidate.Index,
		Placement:  placement.Position,
		Total:      candidate.Total,
		Dimensions: candidate.Dimensions,
		Evidence:   []string{"selected placement candidate"},
	})
}

func weightedCandidateDimensionTotal(dimensions []CandidateScoreDimension) float64 {
	if len(dimensions) == 0 {
		return 0
	}
	totalWeight := 0.0
	total := 0.0
	for _, dimension := range dimensions {
		if dimension.Weight <= 0 {
			continue
		}
		totalWeight += dimension.Weight
		total += dimension.Score * dimension.Weight
	}
	if totalWeight == 0 {
		return 0
	}
	return total / totalWeight
}

func semanticCandidateDimensions(component Component, placement Placement, request Request, anchor Point, hasAnchor bool, groupTarget Point, hasGroupTarget bool) []CandidateScoreDimension {
	weights := request.Rules.CandidateScoring.Weights
	dimensions := make([]CandidateScoreDimension, 0, 2)
	if component.Role != "" && weights.SemanticRole > 0 {
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:     CandidateScoreSemanticRole,
			Score:    1,
			Weight:   weights.SemanticRole,
			Evidence: []string{"role=" + component.Role},
		})
	}
	if weights.GroupCohesion > 0 {
		groupDimension, ok := groupCohesionCandidateDimension(component, placement, request, anchor, hasAnchor, groupTarget, hasGroupTarget, weights.GroupCohesion)
		if ok {
			dimensions = append(dimensions, groupDimension)
		}
	}
	return dimensions
}

type electricalCandidateScoringContext struct {
	Targets             []netScoreTarget
	Weights             CandidateScoreWeights
	ProximityNormalizer float64
	RouteNormalizer     float64
}

func newElectricalCandidateScoringContext(request Request, netTargets []netScoreTarget) electricalCandidateScoringContext {
	weightSum := netTargetWeightSum(netTargets)
	if weightSum <= 0 {
		weightSum = 1
	}
	proximityNormalizer := boardDistance(request.Board.WidthMM, request.Board.HeightMM) * weightSum
	if proximityNormalizer <= 0 {
		proximityNormalizer = 1
	}
	routeNormalizer := (request.Board.WidthMM + request.Board.HeightMM) * weightSum
	if routeNormalizer <= 0 {
		routeNormalizer = 1
	}
	return electricalCandidateScoringContext{
		Targets:             netTargets,
		Weights:             request.Rules.CandidateScoring.Weights,
		ProximityNormalizer: proximityNormalizer,
		RouteNormalizer:     routeNormalizer,
	}
}

func appendElectricalCandidateDimensions(dimensions []CandidateScoreDimension, placement Placement, context electricalCandidateScoringContext, rotatedPadsByName map[string]Point) []CandidateScoreDimension {
	if len(context.Targets) == 0 {
		return dimensions
	}
	if context.Weights.ElectricalProximity > 0 {
		proximityDistance := netEuclideanDistanceScore(placement, context.Targets, rotatedPadsByName)
		proximityScore := clampCandidateUnitScore(1 - proximityDistance/context.ProximityNormalizer)
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:   CandidateScoreElectricalProximity,
			Score:  proximityScore,
			Weight: context.Weights.ElectricalProximity,
		})
	}
	if context.Weights.RouteLength > 0 {
		routeDistance := netManhattanDistanceScore(placement, context.Targets, rotatedPadsByName)
		routeScore := clampCandidateUnitScore(1 - routeDistance/context.RouteNormalizer)
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:   CandidateScoreRouteLength,
			Score:  routeScore,
			Weight: context.Weights.RouteLength,
		})
	}
	return dimensions
}

func clampCandidateUnitScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func netEuclideanDistanceScore(placement Placement, targets []netScoreTarget, rotatedPadsByName map[string]Point) float64 {
	total := 0.0
	for _, target := range targets {
		point := absoluteComponentPadPoint(rotatedPadsByName, target.CurrentPin, placement)
		dx := point.XMM - target.Target.XMM
		dy := point.YMM - target.Target.YMM
		total += netTargetEffectiveWeight(target) * boardDistance(dx, dy)
	}
	return total
}

func netTargetWeightSum(targets []netScoreTarget) float64 {
	total := 0.0
	for _, target := range targets {
		total += netTargetEffectiveWeight(target)
	}
	return total
}

func netTargetEffectiveWeight(target netScoreTarget) float64 {
	if target.Weight <= 0 {
		return 1
	}
	return float64(target.Weight)
}

func netManhattanDistanceScore(placement Placement, targets []netScoreTarget, rotatedPadsByName map[string]Point) float64 {
	total := 0.0
	for _, target := range targets {
		point := absoluteComponentPadPoint(rotatedPadsByName, target.CurrentPin, placement)
		total += netTargetEffectiveWeight(target) * (math.Abs(point.XMM-target.Target.XMM) + math.Abs(point.YMM-target.Target.YMM))
	}
	return total
}

type congestionCandidateScoringContext struct {
	Cells map[congestionCellKey]int
}

type congestionCellKey struct {
	Layer string
	X     int
	Y     int
}

func newCongestionCandidateScoringContext(placedByRef map[string]PlacementResult) congestionCandidateScoringContext {
	context := congestionCandidateScoringContext{Cells: make(map[congestionCellKey]int, len(placedByRef))}
	for _, placed := range placedByRef {
		center := placed.Bounds.Center()
		key := congestionCellKey{
			Layer: placed.Position.Layer,
			X:     int(math.Floor(center.XMM / candidateCongestionCellMM)),
			Y:     int(math.Floor(center.YMM / candidateCongestionCellMM)),
		}
		context.Cells[key]++
	}
	return context
}

func appendCongestionFanoutCandidateDimensions(dimensions []CandidateScoreDimension, component Component, placement PlacementResult, request Request, congestionContext congestionCandidateScoringContext) []CandidateScoreDimension {
	weights := request.Rules.CandidateScoring.Weights
	if weights.Congestion > 0 {
		nearby := congestionContext.NearbyCount(placement)
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:   CandidateScoreCongestion,
			Score:  clampCandidateUnitScore(1 - float64(nearby)/candidateCongestionPlacedSaturationCount),
			Weight: weights.Congestion,
		})
	}
	if weights.Fanout > 0 {
		clearance := max(request.Rules.ComponentSpacingMM, request.Rules.ConnectorEdgeClearanceMM)
		availableSides := cheapFanoutAvailableSideCount(request.Board, placement, clearance)
		padCount := max(1, len(component.Pads))
		sideScore := float64(availableSides) / candidateFanoutSideCount
		padPressure := min(1, float64(padCount)/candidateFanoutPadPressureLimit)
		score := clampCandidateUnitScore(sideScore * (1 - padPressure/2))
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:   CandidateScoreFanout,
			Score:  score,
			Weight: weights.Fanout,
		})
	}
	return dimensions
}

type advancedCandidateScoringContext struct {
	ThermalRules           []thermalCandidateRule
	HighCurrentRules       []highCurrentCandidateRule
	CreepageClearanceRules []clearanceCandidateRule
}

type advancedPlacementRequestContext struct {
	ComponentsByRef        map[string]Component
	NetMembershipByRef     map[string]componentNetMembership
	CreepageClearanceRules []preparedClearanceRule
}

type preparedClearanceRule struct {
	Rule    CreepageClearancePlacementRule
	DomainA placementRuleDomainMatch
	DomainB placementRuleDomainMatch
}

type thermalCandidateRule struct {
	Rule           ThermalPlacementRule
	KeepAwayPoints []Point
}

type highCurrentCandidateRule struct {
	Rule         HighCurrentPlacementRule
	TargetPoints []Point
}

type clearanceCandidateRule struct {
	Rule         CreepageClearancePlacementRule
	TargetPoints []Point
}

type placementRuleDomainMatch struct {
	Refs     map[string]struct{}
	Roles    map[string]struct{}
	Nets     map[string]struct{}
	NetRoles map[NetRole]struct{}
}

type componentNetMembership struct {
	Names map[string]struct{}
	Roles map[NetRole]struct{}
}

func newAdvancedPlacementRequestContext(request Request) advancedPlacementRequestContext {
	context := advancedPlacementRequestContext{
		ComponentsByRef:    componentsByNormalizedRef(request.Components),
		NetMembershipByRef: advancedNetMembershipByRef(request.Nets),
	}
	for _, rule := range request.AdvancedRules.CreepageClearance {
		context.CreepageClearanceRules = append(context.CreepageClearanceRules, preparedClearanceRule{
			Rule:    rule,
			DomainA: newPlacementRuleDomainMatch(rule.DomainA),
			DomainB: newPlacementRuleDomainMatch(rule.DomainB),
		})
	}
	return context
}

func newAdvancedCandidateScoringContext(component Component, componentRef string, request Request, placedByRef map[string]PlacementResult, requestContext advancedPlacementRequestContext) advancedCandidateScoringContext {
	context := advancedCandidateScoringContext{}
	netMembership := requestContext.NetMembershipByRef[componentRef]
	for _, rule := range request.AdvancedRules.Thermal {
		if thermalRuleAffectsComponent(rule, component, componentRef) {
			context.ThermalRules = append(context.ThermalRules, thermalCandidateRule{
				Rule:           rule,
				KeepAwayPoints: placedCentersForRefs(rule.KeepAwayRefs, placedByRef),
			})
		}
	}
	for _, rule := range request.AdvancedRules.HighCurrent {
		sourceRefs := normalizedRefSet(rule.SourceRefs)
		sinkRefs := normalizedRefSet(rule.SinkRefs)
		netNames := upperStringSet(rule.Nets)
		if !highCurrentRuleAffectsComponent(rule, componentRef, netMembership, sourceRefs, sinkRefs, netNames) {
			continue
		}
		context.HighCurrentRules = append(context.HighCurrentRules, highCurrentCandidateRule{
			Rule:         rule,
			TargetPoints: placedCentersForRefs(highCurrentTargetRefs(rule, componentRef, sourceRefs, sinkRefs), placedByRef),
		})
	}
	for _, prepared := range requestContext.CreepageClearanceRules {
		rule := prepared.Rule
		domainA := prepared.DomainA
		domainB := prepared.DomainB
		if domainAffectsComponent(domainA, component, componentRef, netMembership) {
			context.CreepageClearanceRules = append(context.CreepageClearanceRules, clearanceCandidateRule{
				Rule:         rule,
				TargetPoints: placedCentersForDomain(domainB, placedByRef, requestContext.ComponentsByRef, requestContext.NetMembershipByRef),
			})
			continue
		}
		if domainAffectsComponent(domainB, component, componentRef, netMembership) {
			context.CreepageClearanceRules = append(context.CreepageClearanceRules, clearanceCandidateRule{
				Rule:         rule,
				TargetPoints: placedCentersForDomain(domainA, placedByRef, requestContext.ComponentsByRef, requestContext.NetMembershipByRef),
			})
		}
	}
	return context
}

func newPlacementRuleDomainMatch(domain PlacementRuleDomain) placementRuleDomainMatch {
	match := placementRuleDomainMatch{
		Refs:     normalizedRefSet(domain.Refs),
		Roles:    lowerStringSet(domain.Roles),
		Nets:     upperStringSet(domain.Nets),
		NetRoles: map[NetRole]struct{}{},
	}
	for _, role := range domain.NetRoles {
		match.NetRoles[role] = struct{}{}
	}
	return match
}

func advancedNetMembershipByRef(nets []Net) map[string]componentNetMembership {
	byRef := map[string]componentNetMembership{}
	for _, net := range nets {
		netName := strings.ToUpper(strings.TrimSpace(net.Name))
		for _, endpoint := range net.Endpoints {
			ref := normalizeRef(endpoint.Ref)
			if ref == "" {
				continue
			}
			membership := byRef[ref]
			if membership.Names == nil {
				membership.Names = map[string]struct{}{}
			}
			if membership.Roles == nil {
				membership.Roles = map[NetRole]struct{}{}
			}
			if netName != "" {
				membership.Names[netName] = struct{}{}
			}
			membership.Roles[net.Role] = struct{}{}
			byRef[ref] = membership
		}
	}
	return byRef
}

func appendThermalHighCurrentCandidateDimensions(dimensions []CandidateScoreDimension, component Component, placement PlacementResult, request Request, placedByRef map[string]PlacementResult, advancedContext advancedCandidateScoringContext) []CandidateScoreDimension {
	weights := request.Rules.CandidateScoring.Weights
	if weights.Thermal > 0 {
		if dimension, ok := thermalCandidateDimension(placement, request.Board, advancedContext.ThermalRules, weights.Thermal); ok {
			dimensions = append(dimensions, dimension)
		}
	}
	if weights.HighCurrent > 0 {
		if dimension, ok := highCurrentCandidateDimension(placement, request, placedByRef, advancedContext.HighCurrentRules, weights.HighCurrent); ok {
			dimensions = append(dimensions, dimension)
		}
	}
	if weights.CreepageClearance > 0 {
		if dimension, ok := clearanceCandidateDimension(placement, request, advancedContext.CreepageClearanceRules, weights.CreepageClearance); ok {
			dimensions = append(dimensions, dimension)
		}
	}
	return dimensions
}

func thermalCandidateDimension(placement PlacementResult, board BoardPlacementArea, rules []thermalCandidateRule, weight float64) (CandidateScoreDimension, bool) {
	if len(rules) == 0 {
		return CandidateScoreDimension{}, false
	}
	scoreTotal := 0.0
	scoreCount := 0.0
	evidence := []string{}
	for _, candidateRule := range rules {
		rule := candidateRule.Rule
		ruleScore := 0.0
		ruleCount := 0.0
		if rule.PreferredEdge != "" && rule.PreferredEdge != EdgeNone {
			ruleScore += thermalPreferredEdgeScore(rule.PreferredEdge, placement, board)
			ruleCount++
			evidence = append(evidence, "rule="+rule.ID+" preferred_edge="+string(rule.PreferredEdge))
		}
		if rule.MinDistanceMM > 0 && len(candidateRule.KeepAwayPoints) > 0 {
			if distanceSq, ok := nearestPointDistanceSquared(placement, candidateRule.KeepAwayPoints); ok {
				ruleScore += clampCandidateUnitScore(distanceSq / square(rule.MinDistanceMM))
				ruleCount++
				evidence = append(evidence, "rule="+rule.ID+" thermal_keepaway_checked")
			}
		}
		if ruleCount == 0 {
			ruleScore = 0.5
			ruleCount = 1
			evidence = append(evidence, "rule="+rule.ID+" thermal_metadata_incomplete")
		}
		scoreTotal += ruleScore / ruleCount
		scoreCount++
	}
	if scoreCount == 0 {
		return CandidateScoreDimension{}, false
	}
	return CandidateScoreDimension{
		Name:     CandidateScoreThermal,
		Score:    scoreTotal / scoreCount,
		Weight:   weight,
		Evidence: evidence,
	}, true
}

func highCurrentCandidateDimension(placement PlacementResult, request Request, placedByRef map[string]PlacementResult, rules []highCurrentCandidateRule, weight float64) (CandidateScoreDimension, bool) {
	if len(rules) == 0 {
		return CandidateScoreDimension{}, false
	}
	boardDiagonal := boardDistance(request.Board.WidthMM, request.Board.HeightMM)
	if boardDiagonal <= 0 {
		boardDiagonal = 1
	}
	scoreTotal := 0.0
	scoreCount := 0.0
	evidence := []string{}
	for _, candidateRule := range rules {
		rule := candidateRule.Rule
		if len(candidateRule.TargetPoints) == 0 {
			scoreTotal += 0.5
			scoreCount++
			evidence = append(evidence, "rule="+rule.ID+" high_current_endpoint_metadata_incomplete")
			continue
		}
		if distanceSq, ok := nearestPointDistanceSquared(placement, candidateRule.TargetPoints); ok {
			normalizer := boardDiagonal
			if rule.MaxPreferredLengthMM > 0 {
				normalizer = rule.MaxPreferredLengthMM
			}
			scoreTotal += clampCandidateUnitScore(1 - distanceSq/square(normalizer))
			scoreCount++
			evidence = append(evidence, "rule="+rule.ID+" high_current_distance_checked")
		}
	}
	if scoreCount == 0 {
		return CandidateScoreDimension{}, false
	}
	return CandidateScoreDimension{
		Name:     CandidateScoreHighCurrent,
		Score:    scoreTotal / scoreCount,
		Weight:   weight,
		Evidence: evidence,
	}, true
}

func clearanceCandidateDimension(placement PlacementResult, request Request, rules []clearanceCandidateRule, weight float64) (CandidateScoreDimension, bool) {
	if len(rules) == 0 {
		return CandidateScoreDimension{}, false
	}
	boardDiagonal := boardDistance(request.Board.WidthMM, request.Board.HeightMM)
	if boardDiagonal <= 0 {
		boardDiagonal = 1
	}
	scoreTotal := 0.0
	scoreCount := 0.0
	evidence := []string{}
	for _, candidateRule := range rules {
		rule := candidateRule.Rule
		if rule.MinCreepageMM > 0 {
			evidence = append(evidence, "rule="+rule.ID+" creepage_proof_unsupported")
		}
		if len(candidateRule.TargetPoints) == 0 {
			scoreTotal += 0.5
			scoreCount++
			evidence = append(evidence, "rule="+rule.ID+" clearance_target_not_placed")
			continue
		}
		if distanceSq, ok := nearestPointDistanceSquared(placement, candidateRule.TargetPoints); ok {
			normalizer := boardDiagonal
			if rule.MinClearanceMM > 0 {
				normalizer = rule.MinClearanceMM
			}
			scoreTotal += clampCandidateUnitScore(distanceSq / square(normalizer))
			scoreCount++
			evidence = append(evidence, "rule="+rule.ID+" clearance_checked")
		}
	}
	if scoreCount == 0 {
		return CandidateScoreDimension{}, false
	}
	return CandidateScoreDimension{
		Name:     CandidateScoreCreepageClearance,
		Score:    scoreTotal / scoreCount,
		Weight:   weight,
		Evidence: evidence,
	}, true
}

func advancedPlacementHardRejection(component Component, placement PlacementResult, advancedContext advancedCandidateScoringContext) (CandidateRejectionReasonName, string, []string, bool) {
	for _, candidateRule := range advancedContext.ThermalRules {
		rule := candidateRule.Rule
		if rule.Enforcement != AdvancedRuleHard || rule.MinDistanceMM <= 0 {
			continue
		}
		if distanceSq, ok := nearestPointDistanceSquared(placement, candidateRule.KeepAwayPoints); ok && distanceSq < square(rule.MinDistanceMM) {
			message := fmt.Sprintf("candidate violates thermal rule %s: squared distance %.3f is below limit %.3f", rule.ID, distanceSq, square(rule.MinDistanceMM))
			return CandidateRejectAdvancedRule, message, []string{component.Ref}, true
		}
	}
	for _, candidateRule := range advancedContext.HighCurrentRules {
		rule := candidateRule.Rule
		if rule.Enforcement != AdvancedRuleHard || rule.MaxPreferredLengthMM <= 0 || len(candidateRule.TargetPoints) == 0 {
			continue
		}
		if distanceSq, ok := nearestPointDistanceSquared(placement, candidateRule.TargetPoints); ok && distanceSq > square(rule.MaxPreferredLengthMM) {
			message := fmt.Sprintf("candidate violates high-current rule %s: squared distance %.3f exceeds limit %.3f", rule.ID, distanceSq, square(rule.MaxPreferredLengthMM))
			return CandidateRejectAdvancedRule, message, []string{component.Ref}, true
		}
	}
	for _, candidateRule := range advancedContext.CreepageClearanceRules {
		rule := candidateRule.Rule
		if rule.Enforcement != AdvancedRuleHard || rule.MinClearanceMM <= 0 || len(candidateRule.TargetPoints) == 0 {
			continue
		}
		if distanceSq, ok := nearestPointDistanceSquared(placement, candidateRule.TargetPoints); ok && distanceSq < square(rule.MinClearanceMM) {
			message := fmt.Sprintf("candidate violates clearance rule %s: squared distance %.3f is below limit %.3f", rule.ID, distanceSq, square(rule.MinClearanceMM))
			return CandidateRejectAdvancedRule, message, []string{component.Ref}, true
		}
	}
	return "", "", nil, false
}

func thermalRuleAffectsComponent(rule ThermalPlacementRule, component Component, componentRef string) bool {
	if stringSliceContainsFold(rule.Refs, componentRef) {
		return true
	}
	for _, role := range rule.Roles {
		if strings.EqualFold(role, component.Role) {
			return true
		}
	}
	return false
}

func highCurrentRuleAffectsComponent(rule HighCurrentPlacementRule, componentRef string, membership componentNetMembership, sourceRefs map[string]struct{}, sinkRefs map[string]struct{}, netNames map[string]struct{}) bool {
	if _, ok := sourceRefs[componentRef]; ok {
		return true
	}
	if _, ok := sinkRefs[componentRef]; ok {
		return true
	}
	if len(rule.Nets) == 0 && len(rule.NetRoles) == 0 {
		return false
	}
	nameMatches := len(rule.Nets) == 0
	for net := range netNames {
		if _, ok := membership.Names[net]; ok {
			nameMatches = true
			break
		}
	}
	roleMatches := len(rule.NetRoles) == 0
	for _, role := range rule.NetRoles {
		if _, ok := membership.Roles[role]; ok {
			roleMatches = true
			break
		}
	}
	return nameMatches && roleMatches
}

func highCurrentTargetRefs(rule HighCurrentPlacementRule, componentRef string, sourceRefs map[string]struct{}, sinkRefs map[string]struct{}) []string {
	var targets []string
	if _, ok := sourceRefs[componentRef]; ok {
		targets = append(targets, rule.SinkRefs...)
	}
	if _, ok := sinkRefs[componentRef]; ok {
		targets = append(targets, rule.SourceRefs...)
	}
	if len(targets) > 0 {
		return uniqueSortedStrings(targets)
	}
	targets = append([]string(nil), rule.SourceRefs...)
	targets = append(targets, rule.SinkRefs...)
	return uniqueSortedStrings(targets)
}

func domainAffectsComponent(domain placementRuleDomainMatch, component Component, componentRef string, membership componentNetMembership) bool {
	if _, ok := domain.Refs[componentRef]; ok {
		return true
	}
	if _, ok := domain.Roles[strings.ToLower(strings.TrimSpace(component.Role))]; ok {
		return true
	}
	for net := range domain.Nets {
		if _, ok := membership.Names[net]; ok {
			return true
		}
	}
	for role := range domain.NetRoles {
		if _, ok := membership.Roles[role]; ok {
			return true
		}
	}
	return false
}

func placedCentersForDomain(domain placementRuleDomainMatch, placedByRef map[string]PlacementResult, componentsByRef map[string]Component, netMembershipByRef map[string]componentNetMembership) []Point {
	points := make([]Point, 0, len(placedByRef))
	for ref, placement := range placedByRef {
		component := componentsByRef[ref]
		if domainAffectsComponent(domain, component, ref, netMembershipByRef[ref]) {
			points = append(points, placement.Bounds.Center())
		}
	}
	return points
}

func normalizedRefSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if ref := normalizeRef(value); ref != "" {
			out[ref] = struct{}{}
		}
	}
	return out
}

func upperStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func lowerStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func placedCentersForRefs(refs []string, placedByRef map[string]PlacementResult) []Point {
	points := make([]Point, 0, len(refs))
	for _, ref := range refs {
		if placed, ok := placedByRef[normalizeRef(ref)]; ok {
			points = append(points, placed.Bounds.Center())
		}
	}
	return points
}

func nearestPointDistanceSquared(placement PlacementResult, points []Point) (float64, bool) {
	candidateCenter := placement.Bounds.Center()
	best := 0.0
	found := false
	for _, point := range points {
		distance := square(candidateCenter.XMM-point.XMM) + square(candidateCenter.YMM-point.YMM)
		if !found || distance < best {
			best = distance
			found = true
		}
	}
	return best, found
}

func square(value float64) float64 {
	return value * value
}

func thermalPreferredEdgeScore(edge EdgeConstraint, placement PlacementResult, board BoardPlacementArea) float64 {
	center := placement.Bounds.Center()
	switch edge {
	case EdgeLeft:
		return clampCandidateUnitScore(1 - (center.XMM-board.Origin.XMM)/max(1, board.WidthMM))
	case EdgeRight:
		return clampCandidateUnitScore(1 - (board.Origin.XMM+board.WidthMM-center.XMM)/max(1, board.WidthMM))
	case EdgeTop:
		return clampCandidateUnitScore(1 - (center.YMM-board.Origin.YMM)/max(1, board.HeightMM))
	case EdgeBottom:
		return clampCandidateUnitScore(1 - (board.Origin.YMM+board.HeightMM-center.YMM)/max(1, board.HeightMM))
	case EdgeAny:
		left := center.XMM - board.Origin.XMM
		right := board.Origin.XMM + board.WidthMM - center.XMM
		top := center.YMM - board.Origin.YMM
		bottom := board.Origin.YMM + board.HeightMM - center.YMM
		return clampCandidateUnitScore(1 - min(min(left, right), min(top, bottom))/max(1, min(board.WidthMM, board.HeightMM)))
	default:
		return 0.5
	}
}

func stringSliceContainsFold(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func (context congestionCandidateScoringContext) NearbyCount(candidate PlacementResult) int {
	if len(context.Cells) == 0 {
		return 0
	}
	center := candidate.Bounds.Center()
	baseX := int(math.Floor(center.XMM / candidateCongestionCellMM))
	baseY := int(math.Floor(center.YMM / candidateCongestionCellMM))
	count := 0
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			count += context.Cells[congestionCellKey{Layer: candidate.Position.Layer, X: baseX + dx, Y: baseY + dy}]
		}
	}
	return count
}

func cheapFanoutAvailableSideCount(board BoardPlacementArea, placement PlacementResult, clearance float64) int {
	count := 0
	if placement.Bounds.Min.XMM-board.Origin.XMM >= clearance {
		count++
	}
	if board.Origin.XMM+board.WidthMM-placement.Bounds.Max.XMM >= clearance {
		count++
	}
	if placement.Bounds.Min.YMM-board.Origin.YMM >= clearance {
		count++
	}
	if board.Origin.YMM+board.HeightMM-placement.Bounds.Max.YMM >= clearance {
		count++
	}
	return count
}

func groupCohesionCandidateDimension(component Component, placement Placement, request Request, anchor Point, hasAnchor bool, groupTarget Point, hasGroupTarget bool, weight float64) (CandidateScoreDimension, bool) {
	if component.GroupID == "" || (!hasAnchor && !hasGroupTarget) {
		return CandidateScoreDimension{}, false
	}
	boardDiagonal := boardDistance(request.Board.WidthMM, request.Board.HeightMM)
	if boardDiagonal <= 0 {
		boardDiagonal = 1
	}
	score := 0.0
	evidence := []string{"group=" + component.GroupID}
	count := 0.0
	if hasAnchor {
		distance := boardDistance(placement.XMM-anchor.XMM, placement.YMM-anchor.YMM)
		score += 1 - min(1, distance/boardDiagonal)
		count++
		evidence = append(evidence, fmt.Sprintf("anchor_distance_mm=%.3f", distance))
	}
	if hasGroupTarget {
		distance := boardDistance(placement.XMM-groupTarget.XMM, placement.YMM-groupTarget.YMM)
		score += 1 - min(1, distance/boardDiagonal)
		count++
		evidence = append(evidence, fmt.Sprintf("peer_distance_mm=%.3f", distance))
	}
	if count == 0 {
		return CandidateScoreDimension{}, false
	}
	return CandidateScoreDimension{
		Name:     CandidateScoreGroupCohesion,
		Score:    score / count,
		Weight:   weight,
		Evidence: evidence,
	}, true
}

func candidateRejectionReasonForConflict(conflict occupancyConflict) CandidateRejectionReasonName {
	if conflict.Kind == occupancyConflictKeepout {
		return CandidateRejectKeepout
	}
	return CandidateRejectCollision
}

func limitCandidateScoresPerRef(scores []CandidateScore, limit int) []CandidateScore {
	if limit < 0 {
		return scores
	}
	counts := make(map[string]int, len(scores))
	out := make([]CandidateScore, 0, len(scores))
	for _, score := range scores {
		ref := score.Ref
		if ref == "" {
			ref = "<unknown>"
		}
		if counts[ref] >= limit {
			continue
		}
		counts[ref]++
		out = append(out, score)
	}
	return out
}

func normalizeCandidateScores(scores []CandidateScore, evidenceLimit int) []CandidateScore {
	out := slices.Clone(scores)
	for index := range out {
		out[index].Ref = strings.TrimSpace(out[index].Ref)
		out[index].Role = strings.TrimSpace(out[index].Role)
		out[index].Total = roundCandidateScore(out[index].Total)
		out[index].Evidence = normalizeCandidateEvidence(out[index].Evidence, evidenceLimit)
		out[index].Dimensions = normalizeCandidateScoreDimensions(out[index].Dimensions, evidenceLimit)
		out[index].Reasons = normalizeCandidateRejectionReasons(out[index].Reasons)
	}
	slices.SortStableFunc(out, func(left, right CandidateScore) int {
		if candidateScoreLess(left, right) {
			return -1
		}
		if candidateScoreLess(right, left) {
			return 1
		}
		return 0
	})
	return out
}

func normalizeCandidateScoreDimensions(dimensions []CandidateScoreDimension, evidenceLimit int) []CandidateScoreDimension {
	out := slices.Clone(dimensions)
	for index := range out {
		out[index].Name = CandidateScoreDimensionName(strings.TrimSpace(string(out[index].Name)))
		out[index].Score = roundCandidateScore(out[index].Score)
		out[index].Weight = roundCandidateScore(out[index].Weight)
		out[index].Evidence = normalizeCandidateEvidence(out[index].Evidence, evidenceLimit)
	}
	slices.SortStableFunc(out, func(left, right CandidateScoreDimension) int {
		if left.Name != right.Name {
			return compareOrdered(left.Name, right.Name)
		}
		if left.Score != right.Score {
			return compareOrdered(right.Score, left.Score)
		}
		return compareOrdered(right.Weight, left.Weight)
	})
	return out
}

func normalizeCandidateRejectionReasons(reasons []CandidateRejectionReason) []CandidateRejectionReason {
	out := slices.Clone(reasons)
	for index := range out {
		out[index].Name = CandidateRejectionReasonName(strings.TrimSpace(string(out[index].Name)))
		out[index].Message = strings.TrimSpace(out[index].Message)
		out[index].Refs = normalizeCandidateReasonRefs(out[index].Refs)
	}
	slices.SortStableFunc(out, func(left, right CandidateRejectionReason) int {
		if left.Name != right.Name {
			return compareOrdered(left.Name, right.Name)
		}
		if left.Severity != right.Severity {
			return compareOrdered(left.Severity, right.Severity)
		}
		return compareOrdered(left.Message, right.Message)
	})
	return out
}

func normalizeCandidateReasonCounts(counts map[string]int) map[string]int {
	if len(counts) == 0 {
		return nil
	}
	out := make(map[string]int, len(counts))
	for key, value := range counts {
		key = strings.TrimSpace(key)
		if key == "" || value <= 0 {
			continue
		}
		out[key] += value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeCandidateReasonRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(refs))
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	slices.Sort(out)
	return out
}

func normalizeCandidateEvidence(evidence []string, limit int) []string {
	if limit <= 0 || len(evidence) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(evidence))
	out := make([]string, 0, len(evidence))
	for _, item := range evidence {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		item = truncateRunes(item, candidateScoreMaxEvidenceLength)
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	slices.Sort(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for index := range value {
		if count == limit {
			return value[:index]
		}
		count++
	}
	return value
}

func candidateScoreLess(left CandidateScore, right CandidateScore) bool {
	if left.Ref != right.Ref {
		return left.Ref < right.Ref
	}
	if left.Rejected != right.Rejected {
		return !left.Rejected
	}
	if left.Total != right.Total {
		return left.Total > right.Total
	}
	if left.Index != right.Index {
		return left.Index < right.Index
	}
	return candidatePlacementLess(left.Placement, right.Placement)
}

func candidatePlacementLess(left Placement, right Placement) bool {
	if math.Abs(left.YMM-right.YMM) > candidatePlacementCompareEpsilon {
		return left.YMM < right.YMM
	}
	if math.Abs(left.XMM-right.XMM) > candidatePlacementCompareEpsilon {
		return left.XMM < right.XMM
	}
	if math.Abs(left.RotationDeg-right.RotationDeg) > candidatePlacementCompareEpsilon {
		return left.RotationDeg < right.RotationDeg
	}
	return left.Layer < right.Layer
}

func compareOrdered[T ~string | ~int | ~float64](left, right T) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func roundCandidateScore(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*float64(candidateScoreFloatPrecision)) / float64(candidateScoreFloatPrecision)
}
