package placement

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

const (
	groupAnchorScoreWeight       = 5.0
	groupKeepTogetherScoreWeight = 8.0
	netConnectivityScoreWeight   = 3.0
	seedTieBreakScoreWeight      = 0.0001
	placementCompareEpsilon      = 1e-9
)

func Place(request Request) Result {
	return PlaceContext(context.Background(), request)
}

func PlaceContext(ctx context.Context, request Request) Result {
	request = NormalizeRequest(request)
	totalComponents := len(request.Components)
	result := Result{
		Status:           StatusPlaced,
		CandidateScoring: NewCandidateScoringReport(request.Rules.CandidateScoring),
		Metrics: Metrics{
			ComponentCount: totalComponents,
		},
	}
	if issue, ok := placementContextIssue(ctx); ok {
		result.Status = StatusBlocked
		result.Issues = []reports.Issue{issue}
		result.Metrics.UnplacedCount = totalComponents
		return result
	}
	if issues := Validate(request); len(issues) > 0 {
		result.Status = StatusBlocked
		result.Issues = issues
		result.Metrics.UnplacedCount = len(request.Components)
		return result
	}

	occupancy := newOccupancy(request)
	components := slicesForPlacement(request.Components)
	padsByRef := componentPadMaps(components)
	rotatedPadsByRef := componentRotatedPadMaps(components, padsByRef)
	netsByRef := netsByComponent(request.Nets)
	advancedRequestContext := newAdvancedPlacementRequestContext(request)
	keepTogetherPeersByRef := keepTogetherPeersByComponent(request)
	placedByRef := make(map[string]PlacementResult, len(components))
	for index, component := range components {
		if issue, ok := placementContextIssue(ctx); ok {
			if result.Metrics.PlacedCount > 0 {
				result.Status = StatusPartial
			} else {
				result.Status = StatusBlocked
			}
			result.Issues = append(result.Issues, issue)
			result.Metrics.UnplacedCount += totalComponents - index
			break
		}
		placement, ok, placementIssues := placeComponent(component, request, occupancy, placedByRef, padsByRef, rotatedPadsByRef, netsByRef, advancedRequestContext, keepTogetherPeersByRef, result.CandidateScoring)
		if !ok {
			result.Status = StatusPartial
			result.Metrics.UnplacedCount++
			result.Placements = append(result.Placements, PlacementResult{
				Ref:         component.Ref,
				FootprintID: component.FootprintID,
				Fixed:       component.Fixed,
				GroupID:     component.GroupID,
				Mobility:    component.Mobility,
				Reason:      "no legal placement found",
			})
			if len(placementIssues) > 0 {
				result.Issues = append(result.Issues, placementIssues...)
			} else {
				result.Issues = append(result.Issues, issue("components."+component.Ref, "no legal placement found for component "+component.Ref))
			}
			continue
		}
		if component.Fixed {
			result.Metrics.FixedCount++
		}
		if estimatedBoundsSource(component.Bounds.Source) {
			result.Metrics.EstimatedBoundsCount++
		}
		result.Metrics.PlacedCount++
		occupancy.Add(placement)
		placedByRef[normalizeRef(placement.Ref)] = placement
		result.Placements = append(result.Placements, placement)
	}
	rigidIssues := preserveRelativeGroupPlacements(request, result.Placements)
	result.Issues = append(result.Issues, rigidIssues...)
	request.Keepouts = TranslatedKeepoutsForPlacements(request, result.Placements)
	if len(rigidIssues) > 0 && result.Status == StatusPlaced {
		result.Status = StatusPartial
	}
	if result.Metrics.UnplacedCount > 0 && result.Metrics.PlacedCount == 0 {
		result.Status = StatusBlocked
	}
	successfulPlacements := successfulPlacementResults(result.Placements)
	geometryIssues := ValidateGeometry(request, successfulPlacements)
	result.Issues = append(result.Issues, geometryIssues...)
	for _, geometryIssue := range geometryIssues {
		if geometryIssue.Code == reports.CodePlacementCollision {
			result.Metrics.CollisionCount++
		}
		if geometryIssue.Code == reports.CodePlacementOutsideBoard {
			result.Metrics.OutsideOutlineCount++
		}
	}
	if len(geometryIssues) > 0 && result.Status == StatusPlaced {
		result.Status = StatusPartial
	}
	groupIssues := ValidateGroups(request, successfulPlacements)
	result.Issues = append(result.Issues, groupIssues...)
	if len(groupIssues) > 0 && result.Status == StatusPlaced {
		result.Status = StatusPartial
	}
	operations, operationIssues := PlacementOperations(request, successfulPlacements)
	result.Operations = operations
	result.Issues = append(result.Issues, operationIssues...)
	if len(operationIssues) > 0 && result.Status == StatusPlaced {
		result.Status = StatusPartial
	}
	result.Metrics.HPWLMM = hpwl(request.Nets, result.Placements)
	if result.CandidateScoring != nil {
		normalized := NormalizeCandidateScoringReport(*result.CandidateScoring, request.Rules.CandidateScoring)
		result.CandidateScoring = &normalized
	}
	return result
}

func placementContextIssue(ctx context.Context) (reports.Issue, bool) {
	if ctx == nil {
		return reports.Issue{}, false
	}
	if err := ctx.Err(); err != nil {
		return reports.Issue{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Path:     "placement.context",
			Message:  err.Error(),
		}, true
	}
	return reports.Issue{}, false
}

func successfulPlacementResults(placements []PlacementResult) []PlacementResult {
	successful := make([]PlacementResult, 0, len(placements))
	for _, placement := range placements {
		if placement.Reason == "" {
			successful = append(successful, placement)
		}
	}
	return successful
}

func slicesForPlacement(components []Component) []Component {
	ordered := append([]Component(nil), components...)
	sort.SliceStable(ordered, func(i int, j int) bool {
		if ordered[i].Fixed != ordered[j].Fixed {
			return ordered[i].Fixed
		}
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority > ordered[j].Priority
		}
		return ordered[i].Ref < ordered[j].Ref
	})
	return ordered
}

func placeComponent(component Component, request Request, occupancy *occupancy, placedByRef map[string]PlacementResult, padsByRef map[string]map[string]Point, rotatedPadsByRef map[string]map[int64]map[string]Point, netsByRef map[string][]*normalizedNet, advancedRequestContext advancedPlacementRequestContext, keepTogetherPeersByRef map[string][]string, scoring *CandidateScoringReport) (PlacementResult, bool, []reports.Issue) {
	componentRef := normalizeRef(component.Ref)
	if component.Fixed {
		if component.Position == nil {
			recordCandidateRejection(scoring, component, componentRef, Placement{}, 0, CandidateRejectMobility, "fixed placement is missing a position")
			return PlacementResult{}, false, nil
		}
		placement, ok := NewPlacementResult(component, *component.Position, request.Rules)
		if !ok {
			recordCandidateRejection(scoring, component, componentRef, *component.Position, 0, CandidateRejectMissingGeometry, "fixed placement bounds are unavailable")
			return PlacementResult{}, false, nil
		}
		physicalBounds, ok := ComponentPhysicalBounds(component, placement.Position)
		if !ok {
			recordCandidateRejection(scoring, component, componentRef, placement.Position, 0, CandidateRejectMissingGeometry, "fixed physical bounds are unavailable")
			return PlacementResult{}, false, nil
		}
		path := "components." + component.Ref + ".position"
		if !BoardUsableRect(request.Board, request.Rules).Contains(physicalBounds) {
			recordCandidateRejection(scoring, component, componentRef, placement.Position, 0, CandidateRejectOutsideBoard, "fixed placement is outside usable board area")
			return PlacementResult{}, false, []reports.Issue{
				geometryIssue(reports.CodePlacementOutsideBoard, path, "fixed placement is outside usable board area"),
			}
		}
		if conflict, ok := occupancy.FirstConflictDetail(placement); ok {
			message := conflict.Message()
			recordCandidateRejection(scoring, component, componentRef, placement.Position, 0, candidateRejectionReasonForConflict(conflict), "fixed placement conflicts with "+message, message)
			return PlacementResult{}, false, []reports.Issue{
				geometryIssue(reports.CodePlacementCollision, path, "fixed placement conflicts with "+message),
			}
		}
		anchor, hasAnchor := groupAnchorPoint(component, request)
		dimensions := semanticCandidateDimensions(component, placement.Position, request, anchor, hasAnchor, Point{}, false)
		recordCandidateWinner(scoring, component, placement, placementCandidate{Placement: placement, Index: 0, Dimensions: dimensions, Total: weightedCandidateDimensionTotal(dimensions)})
		return placement, true, nil
	}
	congestionContext := newCongestionCandidateScoringContext(placedByRef)
	for _, candidate := range candidatePlacements(component, componentRef, request, placedByRef, padsByRef, rotatedPadsByRef, netsByRef, advancedRequestContext, keepTogetherPeersByRef, congestionContext, scoring) {
		placement := candidate.Placement
		if conflict, ok := occupancy.FirstConflictDetail(placement); ok {
			message := conflict.Message()
			recordCandidateRejection(scoring, component, componentRef, placement.Position, candidate.Index, candidateRejectionReasonForConflict(conflict), "candidate conflicts with "+message, message)
			continue
		}
		recordCandidateWinner(scoring, component, placement, candidate)
		return placement, true, nil
	}
	return PlacementResult{}, false, nil
}

func candidatePlacements(component Component, componentRef string, request Request, placedByRef map[string]PlacementResult, padsByRef map[string]map[string]Point, rotatedPadsByRef map[string]map[int64]map[string]Point, netsByRef map[string][]*normalizedNet, advancedRequestContext advancedPlacementRequestContext, keepTogetherPeersByRef map[string][]string, congestionContext congestionCandidateScoringContext, scoring *CandidateScoringReport) []placementCandidate {
	usable := BoardUsableRect(request.Board, request.Rules)
	edgeTolerance := edgeConstraintTolerance(request.Board, request.Rules)
	edgeInset := edgeCandidateInset(request.Board, request.Rules)
	edgeSpan := max(0, connectorEdgeProximity(request.Rules)-edgeInset)
	grid := request.Rules.GridMM
	if grid <= 0 {
		grid = DefaultRules().GridMM
	}
	rotations := componentRotations(component)
	layers := candidateLayers(component, request.Rules)
	maxCandidates := request.Rules.MaxCandidatesPerPart
	advancedContext := newAdvancedCandidateScoringContext(component, componentRef, request, placedByRef, advancedRequestContext)
	candidates := make([]placementCandidate, 0, maxCandidates)
	xCount := max(1, int(math.Floor((usable.Max.XMM-usable.Min.XMM)/grid))+1)
	yCount := max(1, int(math.Floor((usable.Max.YMM-usable.Min.YMM)/grid))+1)
	variantsPerPoint := max(1, len(rotations)*len(layers))
	axisSamples := max(7, int(math.Ceil(math.Sqrt(float64(maxCandidates)/float64(variantsPerPoint)))))
	xIndices := edgeAwareSampledIndices(component, rotations, component.Edge, true, usable.Min.XMM, usable.Max.XMM, edgeInset, edgeSpan, grid, xCount, axisSamples)
	yIndices := edgeAwareSampledIndices(component, rotations, component.Edge, false, usable.Min.YMM, usable.Max.YMM, edgeInset, edgeSpan, grid, yCount, axisSamples)
	candidateIndex := 0
	for _, yIndex := range yIndices {
		y := usable.Min.YMM + float64(yIndex)*grid
		for _, xIndex := range xIndices {
			x := usable.Min.XMM + float64(xIndex)*grid
			for _, rotation := range rotations {
				for _, layer := range layers {
					index := candidateIndex
					candidateIndex++
					candidate := Placement{XMM: roundToGrid(x, grid), YMM: roundToGrid(y, grid), RotationDeg: rotation, Layer: layer}
					candidateResult, ok := NewPlacementResult(component, candidate, request.Rules)
					if !ok {
						recordCandidateRejection(scoring, component, componentRef, candidate, index, CandidateRejectMissingGeometry, "candidate bounds are unavailable")
						continue
					}
					physicalBounds, ok := ComponentPhysicalBounds(component, candidateResult.Position)
					if !ok {
						recordCandidateRejection(scoring, component, componentRef, candidate, index, CandidateRejectMissingGeometry, "candidate physical bounds are unavailable")
						continue
					}
					if !usable.Contains(physicalBounds) {
						recordCandidateRejection(scoring, component, componentRef, candidateResult.Position, index, CandidateRejectOutsideBoard, "candidate is outside usable board area")
						continue
					}
					if !component.Fixed && component.Edge != EdgeNone && !edgeConstraintSatisfied(request.Board, component, candidateResult.Position, component.Edge, edgeTolerance) {
						recordCandidateRejection(scoring, component, componentRef, candidateResult.Position, index, CandidateRejectEdge, "candidate does not satisfy edge constraint")
						continue
					}
					if reason, message, refs, rejected := advancedPlacementHardRejection(component, candidateResult, advancedContext); rejected {
						recordCandidateRejection(scoring, component, componentRef, candidateResult.Position, index, reason, message, refs...)
						continue
					}
					candidates = append(candidates, placementCandidate{Placement: candidateResult, Index: index})
				}
			}
		}
	}
	anchor, hasAnchor := groupAnchorPoint(component, request)
	groupTarget, hasGroupTarget := groupKeepTogetherTarget(componentRef, keepTogetherPeersByRef, placedByRef)
	netTargets := netScoreTargets(componentRef, netsByRef[componentRef], placedByRef, rotatedPadsByRef)
	electricalContext := newElectricalCandidateScoringContext(request, netTargets)
	timingContext := newTimingSensitiveCandidateScoringContext(componentRef, request, placedByRef)
	seedBase := seedTieBreakBase(request.Seed, component.Ref)
	rotatedPadsByRotation := rotatedPadsByRef[componentRef]
	scored := make([]scoredPlacementCandidate, len(candidates))
	for index := range candidates {
		candidate := candidates[index]
		dimensions := semanticCandidateDimensions(component, candidate.Placement.Position, request, anchor, hasAnchor, groupTarget, hasGroupTarget)
		dimensions = appendElectricalCandidateDimensions(dimensions, candidate.Placement.Position, electricalContext, rotatedPadsByRotation[rotationKey(candidate.Placement.Position.RotationDeg)])
		dimensions = appendCongestionFanoutCandidateDimensions(dimensions, component, candidate.Placement, request, congestionContext)
		dimensions = appendAdvancedCandidateDimensions(dimensions, component, candidate.Placement, request, placedByRef, advancedContext)
		dimensions = appendTimingSensitiveCandidateDimensions(dimensions, candidate.Placement, timingContext)
		total := weightedCandidateDimensionTotal(dimensions)
		scored[index] = scoredPlacementCandidate{
			CandidateIndex: index,
			LegacyCost:     placementScore(component, candidate.Placement.Position, request, anchor, hasAnchor, groupTarget, hasGroupTarget, netTargets, rotatedPadsByRotation, seedBase),
			Total:          total,
			Dimensions:     dimensions,
		}
	}
	scoringEnabled := request.Rules.CandidateScoring.Enabled
	sort.Slice(scored, func(i int, j int) bool {
		if scoringEnabled && scored[i].Total != scored[j].Total {
			return scored[i].Total > scored[j].Total
		}
		if scored[i].LegacyCost != scored[j].LegacyCost {
			return scored[i].LegacyCost < scored[j].LegacyCost
		}
		if comparison := placementCompare(candidates[scored[i].CandidateIndex].Placement.Position, candidates[scored[j].CandidateIndex].Placement.Position); comparison != 0 {
			return comparison < 0
		}
		return candidates[scored[i].CandidateIndex].Index < candidates[scored[j].CandidateIndex].Index
	})
	ordered := make([]placementCandidate, len(candidates))
	for index, candidate := range scored {
		ordered[index] = candidates[candidate.CandidateIndex]
		ordered[index].Total = candidate.Total
		ordered[index].Dimensions = candidate.Dimensions
	}
	candidates = ordered
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	return candidates
}

type placementCandidate struct {
	Placement  PlacementResult
	Index      int
	Total      float64
	Dimensions []CandidateScoreDimension
}

type scoredPlacementCandidate struct {
	CandidateIndex int
	LegacyCost     float64
	Total          float64
	Dimensions     []CandidateScoreDimension
}

func placementLess(left Placement, right Placement) bool {
	return placementCompare(left, right) < 0
}

func placementCompare(left Placement, right Placement) int {
	if math.Abs(left.YMM-right.YMM) > placementCompareEpsilon {
		return compareFloat64(left.YMM, right.YMM)
	}
	if math.Abs(left.XMM-right.XMM) > placementCompareEpsilon {
		return compareFloat64(left.XMM, right.XMM)
	}
	if math.Abs(left.RotationDeg-right.RotationDeg) > placementCompareEpsilon {
		return compareFloat64(left.RotationDeg, right.RotationDeg)
	}
	if left.Layer < right.Layer {
		return -1
	}
	if left.Layer > right.Layer {
		return 1
	}
	return 0
}

func compareFloat64(left float64, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func candidateLayers(component Component, rules Rules) []string {
	switch component.Side {
	case SideBottom:
		return []string{"B.Cu"}
	case SideAny:
		if rules.AllowBackLayer {
			return []string{defaultPlacementLayer, "B.Cu"}
		}
		return []string{defaultPlacementLayer}
	default:
		return []string{defaultPlacementLayer}
	}
}

func componentRotations(component Component) []float64 {
	if component.Rotation.FixedDeg != nil {
		return []float64{*component.Rotation.FixedDeg}
	}
	return component.Rotation.AllowedDeg
}

func sampledIndices(count int, target int) []int {
	if count <= 0 {
		return nil
	}
	if target >= count {
		indices := make([]int, count)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	if target < 2 {
		target = 2
	}
	indices := make([]int, 0, target)
	seen := map[int]struct{}{}
	for i := 0; i < target; i++ {
		index := int(math.Round(float64(i) * float64(count-1) / float64(target-1)))
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		indices = append(indices, index)
	}
	return indices
}

func edgeAwareSampledIndices(component Component, rotations []float64, edge EdgeConstraint, horizontal bool, usableMin, usableMax, edgeInset, edgeSpan, grid float64, count, target int) []int {
	if edge == EdgeNone {
		return sampledIndices(count, target)
	}
	edgeIndices := map[int]struct{}{}
	inwardSteps := max(0, int(math.Floor(edgeSpan/grid)))
	addIndices := func(position float64, lowerBound bool) {
		const gridIndexEpsilon = 1e-9
		value := (position - usableMin) / grid
		center := int(math.Floor(value + gridIndexEpsilon))
		start, end := center-inwardSteps, center
		if lowerBound {
			center = int(math.Ceil(value - gridIndexEpsilon))
			start, end = center, center+inwardSteps
		}
		for index := start; index <= end; index++ {
			if index >= 0 && index < count {
				edgeIndices[index] = struct{}{}
			}
		}
	}
	for _, rotation := range rotations {
		bounds, ok := ComponentPhysicalBounds(component, Placement{RotationDeg: rotation})
		if !ok {
			continue
		}
		if horizontal && (edge == EdgeLeft || edge == EdgeAny) {
			addIndices(usableMin+edgeInset-bounds.Min.XMM, true)
		}
		if !horizontal && (edge == EdgeTop || edge == EdgeAny) {
			addIndices(usableMin+edgeInset-bounds.Min.YMM, true)
		}
		if horizontal && (edge == EdgeRight || edge == EdgeAny) {
			addIndices(usableMax-edgeInset-bounds.Max.XMM, false)
		}
		if !horizontal && (edge == EdgeBottom || edge == EdgeAny) {
			addIndices(usableMax-edgeInset-bounds.Max.YMM, false)
		}
	}
	if len(edgeIndices) == 0 {
		return sampledIndices(count, target)
	}
	if edge == EdgeAny {
		for _, index := range sampledIndices(count, target) {
			edgeIndices[index] = struct{}{}
		}
	}
	edgeSamples := make([]int, 0, len(edgeIndices))
	for index := range edgeIndices {
		edgeSamples = append(edgeSamples, index)
	}
	sort.Ints(edgeSamples)
	if len(edgeSamples) >= target {
		selected := sampledIndices(len(edgeSamples), target)
		for index := range selected {
			selected[index] = edgeSamples[selected[index]]
		}
		return selected
	}
	return edgeSamples
}

func placementScore(component Component, placement Placement, request Request, anchor Point, hasAnchor bool, groupTarget Point, hasGroupTarget bool, netTargets []netScoreTarget, rotatedPadsByRotation map[int64]map[string]Point, seedBase uint64) float64 {
	score := 0.0
	switch component.Edge {
	case EdgeLeft:
		score += placement.XMM*10 + placement.YMM
	case EdgeRight:
		score += (request.Board.WidthMM-placement.XMM)*10 + placement.YMM
	case EdgeTop:
		score += placement.YMM*10 + placement.XMM
	case EdgeBottom:
		score += (request.Board.HeightMM-placement.YMM)*10 + placement.XMM
	case EdgeAny:
		left := placement.XMM
		right := request.Board.WidthMM - placement.XMM
		top := placement.YMM
		bottom := request.Board.HeightMM - placement.YMM
		score += min(min(left, right), min(top, bottom))*10 + placement.XMM + placement.YMM
	default:
		score += placement.XMM + placement.YMM
	}
	if request.Rules.PreferTopLayer && strings.EqualFold(placement.Layer, "B.Cu") {
		score += request.Board.WidthMM + request.Board.HeightMM
	}
	if hasAnchor {
		score += boardDistance(placement.XMM-anchor.XMM, placement.YMM-anchor.YMM) * groupAnchorScoreWeight
	}
	if hasGroupTarget {
		score += boardDistance(placement.XMM-groupTarget.XMM, placement.YMM-groupTarget.YMM) * groupKeepTogetherScoreWeight
	}
	score += netDistanceScore(placement, netTargets, rotatedPadsByRotation[rotationKey(placement.RotationDeg)]) * netConnectivityScoreWeight
	score += seedTieBreak(seedBase, placement) * seedTieBreakScoreWeight
	return score
}

func keepTogetherPeersByComponent(request Request) map[string][]string {
	peersByRef := map[string][]string{}
	componentGroups := map[string][]string{}
	for _, component := range request.Components {
		groupID := strings.ToUpper(strings.TrimSpace(component.GroupID))
		if groupID == "" {
			continue
		}
		componentGroups[groupID] = append(componentGroups[groupID], normalizeRef(component.Ref))
	}
	for _, group := range request.Groups {
		if !group.KeepTogether {
			continue
		}
		groupID := strings.ToUpper(strings.TrimSpace(group.ID))
		seen := map[string]struct{}{}
		members := make([]string, 0, len(group.Components)+len(componentGroups[groupID]))
		for _, ref := range componentGroups[groupID] {
			if ref == "" {
				continue
			}
			seen[ref] = struct{}{}
			members = append(members, ref)
		}
		for _, ref := range group.Components {
			normalizedRef := normalizeRef(ref)
			if normalizedRef == "" {
				continue
			}
			if _, ok := seen[normalizedRef]; ok {
				continue
			}
			seen[normalizedRef] = struct{}{}
			members = append(members, normalizedRef)
		}
		for _, member := range members {
			for _, peer := range members {
				if peer == member {
					continue
				}
				peersByRef[member] = append(peersByRef[member], peer)
			}
		}
	}
	for ref, peers := range peersByRef {
		seen := map[string]struct{}{}
		unique := peers[:0]
		for _, peer := range peers {
			if _, ok := seen[peer]; ok {
				continue
			}
			seen[peer] = struct{}{}
			unique = append(unique, peer)
		}
		peersByRef[ref] = unique
	}
	return peersByRef
}

func groupKeepTogetherTarget(componentRef string, keepTogetherPeersByRef map[string][]string, placedByRef map[string]PlacementResult) (Point, bool) {
	if len(placedByRef) == 0 {
		return Point{}, false
	}
	peerRefs := keepTogetherPeersByRef[componentRef]
	if len(peerRefs) == 0 {
		return Point{}, false
	}
	var centroid Point
	count := 0
	for _, peerRef := range peerRefs {
		placement, ok := placedByRef[peerRef]
		if !ok {
			continue
		}
		center := placement.Bounds.Center()
		centroid.XMM += center.XMM
		centroid.YMM += center.YMM
		count++
	}
	if count == 0 {
		return Point{}, false
	}
	centroid.XMM /= float64(count)
	centroid.YMM /= float64(count)
	return centroid, true
}

func estimatedBoundsSource(source BoundsSource) bool {
	switch source {
	case BoundsEstimated, BoundsGeneratedPads, BoundsLibraryPads:
		return true
	default:
		return false
	}
}

type netScoreTarget struct {
	CurrentPin string
	Target     Point
	Weight     int
}

func netScoreTargets(componentRef string, nets []*normalizedNet, placedByRef map[string]PlacementResult, rotatedPadsByRef map[string]map[int64]map[string]Point) []netScoreTarget {
	if len(placedByRef) == 0 || len(nets) == 0 {
		return nil
	}
	targets := []netScoreTarget{}
	for _, net := range nets {
		currentPins := []string{}
		for _, endpoint := range net.Endpoints {
			if endpoint.Ref == componentRef {
				currentPins = append(currentPins, endpoint.Pin)
			}
		}
		if len(currentPins) == 0 {
			continue
		}
		weight := net.Weight
		if weight <= 0 {
			weight = 1
		}
		for _, endpoint := range net.Endpoints {
			other, ok := placedByRef[endpoint.Ref]
			if !ok {
				continue
			}
			rotatedPadsByRotation, ok := rotatedPadsByRef[endpoint.Ref]
			if !ok {
				continue
			}
			target := absolutePlacedPadPoint(rotatedPadsByRotation[rotationKey(other.Position.RotationDeg)], endpoint.Pin, other, other.Bounds.Center())
			for _, pin := range currentPins {
				targets = append(targets, netScoreTarget{CurrentPin: pin, Target: target, Weight: weight})
			}
		}
	}
	return targets
}

func netDistanceScore(placement Placement, targets []netScoreTarget, rotatedPadsByName map[string]Point) float64 {
	var total float64
	for _, target := range targets {
		point := absoluteComponentPadPoint(rotatedPadsByName, target.CurrentPin, placement)
		dx := point.XMM - target.Target.XMM
		dy := point.YMM - target.Target.YMM
		total += float64(target.Weight) * boardDistance(dx, dy)
	}
	return total
}

func absolutePlacedPadPoint(rotatedPadsByName map[string]Point, pin string, placement PlacementResult, fallback Point) Point {
	if local, ok := rotatedPadsByName[pin]; ok {
		return Point{XMM: placement.Position.XMM + local.XMM, YMM: placement.Position.YMM + local.YMM}
	}
	return fallback
}

func boardDistance(dx float64, dy float64) float64 {
	return math.Sqrt(dx*dx + dy*dy)
}

func absoluteComponentPadPoint(rotatedPadsByName map[string]Point, pin string, placement Placement) Point {
	if local, ok := rotatedPadsByName[pin]; ok {
		return Point{XMM: placement.XMM + local.XMM, YMM: placement.YMM + local.YMM}
	}
	return Point{XMM: placement.XMM, YMM: placement.YMM}
}

func padPointMap(component Component) map[string]Point {
	padsByName := make(map[string]Point, len(component.Pads))
	for _, pad := range component.Pads {
		name := normalizePin(pad.Name)
		if name != "" {
			padsByName[name] = Point{XMM: pad.XMM, YMM: pad.YMM}
		}
	}
	return padsByName
}

func rotatedPadMaps(padsByName map[string]Point, rotations []float64) map[int64]map[string]Point {
	byRotation := make(map[int64]map[string]Point, len(rotations))
	for _, rotation := range rotations {
		rotated := make(map[string]Point, len(padsByName))
		for pin, point := range padsByName {
			rotated[pin] = rotatePoint(point, rotation)
		}
		byRotation[rotationKey(rotation)] = rotated
	}
	return byRotation
}

func rotationKey(rotation float64) int64 {
	rotation = math.Mod(rotation, 360)
	if rotation < 0 {
		rotation += 360
	}
	return int64(math.Round(rotation*10)) % 3600
}

func seedTieBreakBase(seed string, ref string) uint64 {
	if seed == "" {
		return 0
	}
	value := uint64(1469598103934665603)
	for _, ch := range seed + "|" + ref {
		value ^= uint64(ch)
		value *= 1099511628211
	}
	return value
}

func seedTieBreak(base uint64, placement Placement) float64 {
	if base == 0 {
		return 0
	}
	value := base
	value ^= math.Float64bits(placement.XMM) + 0x9e3779b97f4a7c15 + (value << 6) + (value >> 2)
	value ^= math.Float64bits(placement.YMM) + 0x9e3779b97f4a7c15 + (value << 6) + (value >> 2)
	value ^= math.Float64bits(placement.RotationDeg) + 0x9e3779b97f4a7c15 + (value << 6) + (value >> 2)
	for _, ch := range placement.Layer {
		value ^= uint64(ch) + 0x9e3779b97f4a7c15 + (value << 6) + (value >> 2)
	}
	return float64(value%1000) / 1000
}

func componentPadMaps(components []Component) map[string]map[string]Point {
	byRef := make(map[string]map[string]Point, len(components))
	byFootprintOrGeometry := map[string]map[string]Point{}
	for _, component := range components {
		cacheKey := componentPadMapCacheKey(component)
		if padsByName, ok := byFootprintOrGeometry[cacheKey]; ok {
			byRef[normalizeRef(component.Ref)] = padsByName
			continue
		}
		padsByName := padPointMap(component)
		byFootprintOrGeometry[cacheKey] = padsByName
		byRef[normalizeRef(component.Ref)] = padsByName
	}
	return byRef
}

func componentRotatedPadMaps(components []Component, padsByRef map[string]map[string]Point) map[string]map[int64]map[string]Point {
	byRef := make(map[string]map[int64]map[string]Point, len(components))
	byFootprintGeometryAndRotation := map[string]map[int64]map[string]Point{}
	for _, component := range components {
		ref := normalizeRef(component.Ref)
		cacheKey := componentPadMapCacheKey(component) + "|rot:" + rotationSetKey(componentRotations(component))
		if rotatedPadsByRotation, ok := byFootprintGeometryAndRotation[cacheKey]; ok {
			byRef[ref] = rotatedPadsByRotation
			continue
		}
		rotatedPadsByRotation := rotatedPadMaps(padsByRef[ref], componentRotations(component))
		byFootprintGeometryAndRotation[cacheKey] = rotatedPadsByRotation
		byRef[ref] = rotatedPadsByRotation
	}
	return byRef
}

func componentPadMapCacheKey(component Component) string {
	footprintID := strings.TrimSpace(component.FootprintID)
	if footprintID != "" {
		return "fp:" + footprintID
	}
	type cachePad struct {
		Name string
		XMM  float64
		YMM  float64
	}
	pads := make([]cachePad, 0, len(component.Pads))
	for _, pad := range component.Pads {
		pads = append(pads, cachePad{
			Name: normalizePin(pad.Name),
			XMM:  pad.XMM,
			YMM:  pad.YMM,
		})
	}
	sort.Slice(pads, func(i int, j int) bool {
		if pads[i].Name != pads[j].Name {
			return pads[i].Name < pads[j].Name
		}
		if pads[i].XMM != pads[j].XMM {
			return pads[i].XMM < pads[j].XMM
		}
		return pads[i].YMM < pads[j].YMM
	})
	var builder strings.Builder
	builder.WriteString("pads:")
	for _, pad := range pads {
		builder.WriteString(pad.Name)
		builder.WriteByte('@')
		builder.WriteString(strconv.FormatFloat(pad.XMM, 'f', 6, 64))
		builder.WriteByte(',')
		builder.WriteString(strconv.FormatFloat(pad.YMM, 'f', 6, 64))
	}
	return builder.String()
}

func rotationSetKey(rotations []float64) string {
	keys := make([]int64, 0, len(rotations))
	for _, rotation := range rotations {
		keys = append(keys, rotationKey(rotation))
	}
	sort.Slice(keys, func(i int, j int) bool {
		return keys[i] < keys[j]
	})
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(strconv.FormatInt(key, 10))
		builder.WriteByte(',')
	}
	return builder.String()
}

type normalizedNet struct {
	Weight    int
	Endpoints []normalizedNetEndpoint
}

type normalizedNetEndpoint struct {
	Ref string
	Pin string
}

func netsByComponent(nets []Net) map[string][]*normalizedNet {
	byRef := map[string][]*normalizedNet{}
	normalizedNets := make([]normalizedNet, len(nets))
	for index := range nets {
		net := &normalizedNets[index]
		net.Weight = nets[index].Weight
		net.Endpoints = make([]normalizedNetEndpoint, 0, len(nets[index].Endpoints))
		for _, endpoint := range nets[index].Endpoints {
			net.Endpoints = append(net.Endpoints, normalizedNetEndpoint{
				Ref: normalizeRef(endpoint.Ref),
				Pin: normalizePin(endpoint.Pin),
			})
		}
		seen := map[string]struct{}{}
		for _, endpoint := range net.Endpoints {
			ref := endpoint.Ref
			if ref == "" {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			byRef[ref] = append(byRef[ref], net)
		}
	}
	return byRef
}

func normalizeRef(ref string) string {
	return strings.ToUpper(strings.TrimSpace(ref))
}

func normalizePin(pin string) string {
	return strings.ToUpper(strings.TrimSpace(pin))
}

func groupAnchorPoint(component Component, request Request) (Point, bool) {
	if component.GroupID == "" {
		return Point{}, false
	}
	for _, group := range request.Groups {
		if !strings.EqualFold(group.ID, component.GroupID) {
			continue
		}
		if group.Anchor.At != nil {
			return *group.Anchor.At, true
		}
	}
	return Point{}, false
}

func roundToGrid(value float64, grid float64) float64 {
	return math.Round(value/grid) * grid
}

func hpwl(nets []Net, placements []PlacementResult) float64 {
	byRef := map[string]PlacementResult{}
	for _, placement := range placements {
		if placement.Reason == "" {
			byRef[strings.ToUpper(placement.Ref)] = placement
		}
	}
	endpointRefs := make([][]string, len(nets))
	for i, net := range nets {
		endpointRefs[i] = make([]string, len(net.Endpoints))
		for j, endpoint := range net.Endpoints {
			endpointRefs[i][j] = strings.ToUpper(endpoint.Ref)
		}
	}
	var total float64
	for netIndex, net := range nets {
		if len(net.Endpoints) < 2 {
			continue
		}
		minX, minY := 0.0, 0.0
		maxX, maxY := 0.0, 0.0
		seen := 0
		for _, endpointRef := range endpointRefs[netIndex] {
			placement, ok := byRef[endpointRef]
			if !ok {
				continue
			}
			point := placement.Bounds.Center()
			if seen == 0 {
				minX, maxX = point.XMM, point.XMM
				minY, maxY = point.YMM, point.YMM
			} else {
				minX = min(minX, point.XMM)
				maxX = max(maxX, point.XMM)
				minY = min(minY, point.YMM)
				maxY = max(maxY, point.YMM)
			}
			seen++
		}
		if seen > 1 {
			weight := net.Weight
			if weight <= 0 {
				weight = 1
			}
			total += float64(weight) * ((maxX - minX) + (maxY - minY))
		}
	}
	return total
}

func placementSummary(result Result) string {
	return fmt.Sprintf("%s: %d placed, %d unplaced", result.Status, result.Metrics.PlacedCount, result.Metrics.UnplacedCount)
}
