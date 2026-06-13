package placement

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/reports"
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
	for _, component := range components {
		placement, ok := placeComponent(component, request, occupancy)
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

func placeComponent(component Component, request Request, occupancy *occupancy) (PlacementResult, bool) {
	if component.Fixed {
		if component.Position == nil {
			return PlacementResult{}, false
		}
		return NewPlacementResult(component, *component.Position, request.Rules)
	}
	for _, placement := range candidatePlacements(component, request) {
		if _, conflict := occupancy.FirstConflict(placement); conflict {
			continue
		}
		return placement, true
	}
	return PlacementResult{}, false
}

func candidatePlacements(component Component, request Request) []PlacementResult {
	usable := BoardUsableRect(request.Board, request.Rules)
	grid := request.Rules.GridMM
	if grid <= 0 {
		grid = DefaultRules().GridMM
	}
	rotations := component.Rotation.AllowedDeg
	if component.Rotation.FixedDeg != nil {
		rotations = []float64{*component.Rotation.FixedDeg}
	}
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
	sort.SliceStable(candidates, func(i int, j int) bool {
		return placementScore(component, candidates[i].Position, request) < placementScore(component, candidates[j].Position, request)
	})
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	return candidates
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

func placementScore(component Component, placement Placement, request Request) float64 {
	score := placement.XMM + placement.YMM
	switch component.Edge {
	case EdgeLeft:
		score += placement.XMM * 10
	case EdgeRight:
		score += (request.Board.WidthMM - placement.XMM) * 10
	case EdgeTop:
		score += placement.YMM * 10
	case EdgeBottom:
		score += (request.Board.HeightMM - placement.YMM) * 10
	case EdgeAny:
		left := placement.XMM
		right := request.Board.WidthMM - placement.XMM
		top := placement.YMM
		bottom := request.Board.HeightMM - placement.YMM
		score += min(min(left, right), min(top, bottom)) * 10
	}
	if request.Rules.PreferTopLayer && strings.EqualFold(placement.Layer, "B.Cu") {
		score += request.Board.WidthMM + request.Board.HeightMM
	}
	return score
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
