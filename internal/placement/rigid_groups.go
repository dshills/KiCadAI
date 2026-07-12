package placement

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/reports"
)

type relativeGroupCandidate struct {
	placements []PlacementResult
}

func preserveRelativeGroupPlacements(request Request, placements []PlacementResult) []reports.Issue {
	components := make(map[string]Component, len(request.Components))
	for _, component := range request.Components {
		components[normalizeRef(component.Ref)] = component
	}
	placementIndexes := make(map[string]int, len(placements))
	for index, result := range placements {
		if result.Reason == "" {
			placementIndexes[normalizeRef(result.Ref)] = index
		}
	}

	var issues []reports.Issue
	for groupIndex, group := range request.Groups {
		if !group.TranslateAsUnit {
			continue
		}
		candidate, ok := findRelativeGroupPlacement(request, group, components, placements, placementIndexes)
		if !ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodePlacementCollision,
				Severity: reports.SeverityError,
				Path:     fmt.Sprintf("groups[%d].translate_as_unit", groupIndex),
				Message:  "no legal translation preserves relative placement for group " + group.ID,
			})
			continue
		}
		for _, result := range candidate.placements {
			placements[placementIndexes[normalizeRef(result.Ref)]] = result
		}
	}
	return issues
}

func findRelativeGroupPlacement(request Request, group Group, components map[string]Component, placements []PlacementResult, placementIndexes map[string]int) (relativeGroupCandidate, bool) {
	anchorRef := normalizeRef(group.Anchor.Ref)
	anchorComponent, ok := components[anchorRef]
	if !ok || anchorComponent.Position == nil {
		return relativeGroupCandidate{}, false
	}
	anchorIndex, ok := placementIndexes[anchorRef]
	if !ok {
		return relativeGroupCandidate{}, false
	}
	target := placements[anchorIndex].Position
	// Authored block-local copper currently supports translation only.
	// Keep the source orientation and side while searching for a legal offset.
	target.RotationDeg = anchorComponent.Position.RotationDeg
	target.Layer = anchorComponent.Position.Layer
	members := make([]Component, 0, len(group.Components))
	memberRefs := make(map[string]struct{}, len(group.Components))
	for _, ref := range group.Components {
		component, exists := components[normalizeRef(ref)]
		if !exists || component.Position == nil {
			return relativeGroupCandidate{}, false
		}
		if _, exists := placementIndexes[normalizeRef(ref)]; !exists {
			return relativeGroupCandidate{}, false
		}
		members = append(members, component)
		memberRefs[normalizeRef(ref)] = struct{}{}
	}
	if _, included := memberRefs[anchorRef]; !included {
		return relativeGroupCandidate{}, false
	}
	existingByLayer := make(map[string][]PlacementResult)
	for _, placed := range placements {
		if placed.Reason != "" {
			continue
		}
		if _, grouped := memberRefs[normalizeRef(placed.Ref)]; grouped {
			continue
		}
		layer := strings.ToUpper(strings.TrimSpace(placed.Position.Layer))
		existingByLayer[layer] = append(existingByLayer[layer], placed)
	}

	grid := request.Rules.GridMM
	if grid <= 0 {
		grid = DefaultRules().GridMM
	}
	maxCandidates := request.Rules.MaxCandidatesPerPart
	if maxCandidates <= 0 {
		maxCandidates = DefaultRules().MaxCandidatesPerPart
	}
	usable := BoardUsableRect(request.Board, request.Rules)
	var found relativeGroupCandidate
	checked := 0
	forEachRelativeGroupAnchorCandidate(usable, grid, target, func(anchor Placement) bool {
		if checked >= maxCandidates {
			return false
		}
		checked++
		candidate, legal := buildRelativeGroupCandidate(request, members, *anchorComponent.Position, anchor, existingByLayer)
		if legal {
			found = candidate
			return false
		}
		return true
	})
	return found, len(found.placements) != 0
}

func forEachRelativeGroupAnchorCandidate(usable Rect, grid float64, target Placement, visit func(Placement) bool) {
	xCount := max(1, int(math.Floor((usable.Max.XMM-usable.Min.XMM)/grid))+1)
	yCount := max(1, int(math.Floor((usable.Max.YMM-usable.Min.YMM)/grid))+1)
	targetX := min(max(int(math.Round((target.XMM-usable.Min.XMM)/grid)), 0), xCount-1)
	targetY := min(max(int(math.Round((target.YMM-usable.Min.YMM)/grid)), 0), yCount-1)
	maxRadius := max(max(targetX, xCount-1-targetX), max(targetY, yCount-1-targetY))
	for radius := 0; radius <= maxRadius; radius++ {
		candidates := relativeGroupAnchorRing(usable, grid, target, targetX, targetY, radius, xCount, yCount)
		for _, candidate := range candidates {
			if !visit(candidate) {
				return
			}
		}
	}
}

func relativeGroupAnchorRing(usable Rect, grid float64, target Placement, targetX int, targetY int, radius int, xCount int, yCount int) []Placement {
	minX := max(0, targetX-radius)
	maxX := min(xCount-1, targetX+radius)
	minY := max(0, targetY-radius)
	maxY := min(yCount-1, targetY+radius)
	candidates := make([]Placement, 0, max(1, 8*radius))
	appendCandidate := func(xIndex int, yIndex int) {
		candidates = append(candidates, Placement{
			XMM:         roundToGrid(usable.Min.XMM+float64(xIndex)*grid, grid),
			YMM:         roundToGrid(usable.Min.YMM+float64(yIndex)*grid, grid),
			RotationDeg: target.RotationDeg,
			Layer:       target.Layer,
		})
	}
	if radius == 0 {
		appendCandidate(targetX, targetY)
	} else {
		for xIndex := minX; xIndex <= maxX; xIndex++ {
			appendCandidate(xIndex, minY)
			if maxY != minY {
				appendCandidate(xIndex, maxY)
			}
		}
		for yIndex := minY + 1; yIndex < maxY; yIndex++ {
			appendCandidate(minX, yIndex)
			if maxX != minX {
				appendCandidate(maxX, yIndex)
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftDistance := boardDistance(candidates[i].XMM-target.XMM, candidates[i].YMM-target.YMM)
		rightDistance := boardDistance(candidates[j].XMM-target.XMM, candidates[j].YMM-target.YMM)
		if math.Abs(leftDistance-rightDistance) > placementCompareEpsilon {
			return leftDistance < rightDistance
		}
		return placementLess(candidates[i], candidates[j])
	})
	return candidates
}

func buildRelativeGroupCandidate(request Request, members []Component, authoredAnchor Placement, anchor Placement, existingByLayer map[string][]PlacementResult) (relativeGroupCandidate, bool) {
	usable := BoardUsableRect(request.Board, request.Rules)
	keepouts := &occupancy{keepouts: request.Keepouts}
	result := relativeGroupCandidate{}
	for _, component := range members {
		if component.Position == nil {
			return relativeGroupCandidate{}, false
		}
		position := *component.Position
		position.XMM = anchor.XMM + component.Position.XMM - authoredAnchor.XMM
		position.YMM = anchor.YMM + component.Position.YMM - authoredAnchor.YMM
		position.Layer = firstNonEmptyPlacementLayer(component.Position.Layer, anchor.Layer)
		placement, ok := NewPlacementResult(component, position, request.Rules)
		if !ok {
			return relativeGroupCandidate{}, false
		}
		physicalBounds, ok := ComponentPhysicalBounds(component, placement.Position)
		if !ok || !usable.Contains(physicalBounds) {
			return relativeGroupCandidate{}, false
		}
		if _, conflict := keepouts.FirstConflictDetail(placement); conflict {
			return relativeGroupCandidate{}, false
		}
		layer := strings.ToUpper(strings.TrimSpace(placement.Position.Layer))
		for _, placed := range existingByLayer[layer] {
			if placed.Bounds.Intersects(placement.Bounds) {
				return relativeGroupCandidate{}, false
			}
		}
		for _, grouped := range result.placements {
			if strings.EqualFold(grouped.Position.Layer, placement.Position.Layer) && grouped.Bounds.Intersects(placement.Bounds) {
				return relativeGroupCandidate{}, false
			}
		}
		result.placements = append(result.placements, placement)
	}
	return result, true
}

func firstNonEmptyPlacementLayer(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return defaultPlacementLayer
}
