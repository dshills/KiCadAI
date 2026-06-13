package placement

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

const (
	groupAnchorScoreWeight     = 5.0
	netConnectivityScoreWeight = 3.0
	seedTieBreakScoreWeight    = 0.0001
	placementCompareEpsilon    = 1e-9
)

func Place(request Request) Result {
	request = NormalizeRequest(request)
	result := Result{
		Status: StatusPlaced,
		Metrics: Metrics{
			ComponentCount: len(request.Components),
		},
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
	placedByRef := map[string]PlacementResult{}
	for _, component := range components {
		placement, ok := placeComponent(component, request, occupancy, placedByRef, padsByRef, rotatedPadsByRef, netsByRef)
		if !ok {
			result.Status = StatusPartial
			result.Metrics.UnplacedCount++
			result.Placements = append(result.Placements, PlacementResult{
				Ref:         component.Ref,
				FootprintID: component.FootprintID,
				Fixed:       component.Fixed,
				GroupID:     component.GroupID,
				Reason:      "no legal placement found",
			})
			result.Issues = append(result.Issues, issue("components."+component.Ref, "no legal placement found for component "+component.Ref))
			continue
		}
		if component.Fixed {
			result.Metrics.FixedCount++
		}
		result.Metrics.PlacedCount++
		occupancy.Add(placement)
		placedByRef[normalizeRef(placement.Ref)] = placement
		result.Placements = append(result.Placements, placement)
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
	return result
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

func placeComponent(component Component, request Request, occupancy *occupancy, placedByRef map[string]PlacementResult, padsByRef map[string]map[string]Point, rotatedPadsByRef map[string]map[int64]map[string]Point, netsByRef map[string][]*normalizedNet) (PlacementResult, bool) {
	if component.Fixed {
		if component.Position == nil {
			return PlacementResult{}, false
		}
		return NewPlacementResult(component, *component.Position, request.Rules)
	}
	for _, placement := range candidatePlacements(component, request, placedByRef, padsByRef, rotatedPadsByRef, netsByRef) {
		if _, conflict := occupancy.FirstConflict(placement); conflict {
			continue
		}
		return placement, true
	}
	return PlacementResult{}, false
}

func candidatePlacements(component Component, request Request, placedByRef map[string]PlacementResult, padsByRef map[string]map[string]Point, rotatedPadsByRef map[string]map[int64]map[string]Point, netsByRef map[string][]*normalizedNet) []PlacementResult {
	usable := BoardUsableRect(request.Board, request.Rules)
	grid := request.Rules.GridMM
	if grid <= 0 {
		grid = DefaultRules().GridMM
	}
	rotations := componentRotations(component)
	layers := candidateLayers(component, request.Rules)
	maxCandidates := request.Rules.MaxCandidatesPerPart
	candidates := make([]PlacementResult, 0, maxCandidates)
	xCount := max(1, int(math.Floor((usable.Max.XMM-usable.Min.XMM)/grid))+1)
	yCount := max(1, int(math.Floor((usable.Max.YMM-usable.Min.YMM)/grid))+1)
	variantsPerPoint := max(1, len(rotations)*len(layers))
	axisSamples := max(7, int(math.Ceil(math.Sqrt(float64(maxCandidates)/float64(variantsPerPoint)))))
	xIndices := sampledIndices(xCount, axisSamples)
	yIndices := sampledIndices(yCount, axisSamples)
	for _, yIndex := range yIndices {
		y := usable.Min.YMM + float64(yIndex)*grid
		for _, xIndex := range xIndices {
			x := usable.Min.XMM + float64(xIndex)*grid
			for _, rotation := range rotations {
				for _, layer := range layers {
					candidate := Placement{XMM: roundToGrid(x, grid), YMM: roundToGrid(y, grid), RotationDeg: rotation, Layer: layer}
					candidateResult, ok := NewPlacementResult(component, candidate, request.Rules)
					if !ok || !usable.Contains(candidateResult.Bounds) {
						continue
					}
					candidates = append(candidates, candidateResult)
				}
			}
		}
	}
	anchor, hasAnchor := groupAnchorPoint(component, request)
	componentRef := normalizeRef(component.Ref)
	netTargets := netScoreTargets(componentRef, netsByRef[componentRef], placedByRef, rotatedPadsByRef)
	seedBase := seedTieBreakBase(request.Seed, component.Ref)
	rotatedPadsByRotation := rotatedPadsByRef[componentRef]
	scored := make([]scoredPlacementCandidate, len(candidates))
	for index, candidate := range candidates {
		scored[index] = scoredPlacementCandidate{
			CandidateIndex: index,
			Score:          placementScore(component, candidate.Position, request, anchor, hasAnchor, netTargets, rotatedPadsByRotation, seedBase),
		}
	}
	sort.Slice(scored, func(i int, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score < scored[j].Score
		}
		return placementLess(candidates[scored[i].CandidateIndex].Position, candidates[scored[j].CandidateIndex].Position)
	})
	ordered := make([]PlacementResult, len(candidates))
	for index, candidate := range scored {
		ordered[index] = candidates[candidate.CandidateIndex]
	}
	candidates = ordered
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	return candidates
}

type scoredPlacementCandidate struct {
	CandidateIndex int
	Score          float64
}

func placementLess(left Placement, right Placement) bool {
	if math.Abs(left.YMM-right.YMM) > placementCompareEpsilon {
		return left.YMM < right.YMM
	}
	if math.Abs(left.XMM-right.XMM) > placementCompareEpsilon {
		return left.XMM < right.XMM
	}
	if math.Abs(left.RotationDeg-right.RotationDeg) > placementCompareEpsilon {
		return left.RotationDeg < right.RotationDeg
	}
	return left.Layer < right.Layer
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

func placementScore(component Component, placement Placement, request Request, anchor Point, hasAnchor bool, netTargets []netScoreTarget, rotatedPadsByRotation map[int64]map[string]Point, seedBase uint64) float64 {
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
	score += netDistanceScore(placement, netTargets, rotatedPadsByRotation[rotationKey(placement.RotationDeg)]) * netConnectivityScoreWeight
	score += seedTieBreak(seedBase, placement) * seedTieBreakScoreWeight
	return score
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
