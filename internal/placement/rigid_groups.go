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
	authoredKeepouts := append([]Keepout(nil), request.Keepouts...)
	components := make(map[string]Component, len(request.Components))
	for _, component := range request.Components {
		components[normalizeRef(component.Ref)] = component
	}
	translatedMemberRefs := map[string]struct{}{}
	groupOrder := make([]int, 0, len(request.Groups))
	for groupIndex, group := range request.Groups {
		if !group.TranslateAsUnit {
			continue
		}
		groupOrder = append(groupOrder, groupIndex)
		for _, ref := range group.Components {
			translatedMemberRefs[normalizeRef(ref)] = struct{}{}
		}
	}
	sort.SliceStable(groupOrder, func(left, right int) bool {
		leftGroup := request.Groups[groupOrder[left]]
		rightGroup := request.Groups[groupOrder[right]]
		if leftGroup.Priority != rightGroup.Priority {
			return leftGroup.Priority > rightGroup.Priority
		}
		if len(leftGroup.Components) != len(rightGroup.Components) {
			return len(leftGroup.Components) > len(rightGroup.Components)
		}
		return leftGroup.ID < rightGroup.ID
	})
	hardRefs := map[string]struct{}{}
	for ref, component := range components {
		_, translatedMember := translatedMemberRefs[ref]
		if !translatedMember || component.Fixed || component.Mobility.Class == MobilityFixed {
			hardRefs[ref] = struct{}{}
		}
	}
	placementIndexes := make(map[string]int, len(placements))
	for index, result := range placements {
		if result.Reason == "" {
			placementIndexes[normalizeRef(result.Ref)] = index
		}
	}

	var issues []reports.Issue
	for _, groupIndex := range groupOrder {
		group := request.Groups[groupIndex]
		candidate, ok := findRelativeGroupPlacement(request, group, components, placements, placementIndexes, hardRefs)
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
			hardRefs[normalizeRef(result.Ref)] = struct{}{}
		}
		keepoutRequest := request
		keepoutRequest.Keepouts = authoredKeepouts
		request.Keepouts = TranslatedKeepoutsForPlacements(keepoutRequest, placements)
	}
	return issues
}

func findRelativeGroupPlacement(request Request, group Group, components map[string]Component, placements []PlacementResult, placementIndexes map[string]int, hardRefs map[string]struct{}) (relativeGroupCandidate, bool) {
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
		if _, hard := hardRefs[normalizeRef(placed.Ref)]; !hard {
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
	// A rigid group searches one shared transform on behalf of every member.
	// Scale the per-part budget so larger authored groups are not starved after
	// a few nearby transforms that collide with already placed components.
	maxCandidates *= max(1, len(members))
	usable := BoardUsableRect(request.Board, request.Rules)
	var found relativeGroupCandidate
	checked := 0
	forEachRelativeGroupAnchorCandidate(usable, grid, target, func(anchor Placement) bool {
		if anchorComponent.Edge != EdgeNone && !edgeConstraintSatisfied(request.Board, anchorComponent, anchor, anchorComponent.Edge, edgeConstraintTolerance(request.Board, request.Rules)) {
			return true
		}
		if checked >= maxCandidates {
			return false
		}
		checked++
		candidate, legal := buildRelativeGroupCandidate(request, group.ID, members, *anchorComponent.Position, anchor, existingByLayer)
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

func buildRelativeGroupCandidate(request Request, groupID string, members []Component, authoredAnchor Placement, anchor Placement, existingByLayer map[string][]PlacementResult) (relativeGroupCandidate, bool) {
	usable := BoardUsableRect(request.Board, request.Rules)
	for _, group := range request.Groups {
		if !strings.EqualFold(strings.TrimSpace(group.ID), strings.TrimSpace(groupID)) || group.Bounds == nil {
			continue
		}
		bounds := translatedGroupBounds(*group.Bounds, authoredAnchor, anchor)
		if !usable.Contains(bounds) {
			return relativeGroupCandidate{}, false
		}
		break
	}
	edgeTolerance := edgeConstraintTolerance(request.Board, request.Rules)
	translatedKeepouts := translatedKeepoutsForGroup(request.Keepouts, groupID, authoredAnchor, anchor)
	keepouts := &occupancy{keepouts: translatedKeepouts}
	for _, existing := range existingByLayer {
		for _, placed := range existing {
			for _, keepout := range translatedKeepouts {
				if !strings.EqualFold(strings.TrimSpace(keepout.GroupID), strings.TrimSpace(groupID)) || keepout.Optional || keepoutExemptsRef(keepout, placed.Ref) {
					continue
				}
				if keepoutAppliesToLayer(keepout, placed.Position.Layer) && keepout.Bounds.Intersects(placed.Bounds) {
					return relativeGroupCandidate{}, false
				}
			}
		}
	}
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
		if component.Edge != EdgeNone && !edgeConstraintSatisfied(request.Board, component, placement.Position, component.Edge, edgeTolerance) {
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

func translatedGroupBounds(bounds Rect, authoredAnchor Placement, placedAnchor Placement) Rect {
	deltaX := placedAnchor.XMM - authoredAnchor.XMM
	deltaY := placedAnchor.YMM - authoredAnchor.YMM
	bounds.Min.XMM += deltaX
	bounds.Max.XMM += deltaX
	bounds.Min.YMM += deltaY
	bounds.Max.YMM += deltaY
	return bounds
}

// TranslatedKeepoutsForPlacements returns keepouts in the same coordinate
// frame as their final translated placement groups. Unowned keepouts and
// groups without a completed anchor placement remain unchanged.
func TranslatedKeepoutsForPlacements(request Request, placements []PlacementResult) []Keepout {
	keepouts := append([]Keepout(nil), request.Keepouts...)
	components := make(map[string]Component, len(request.Components))
	for _, component := range request.Components {
		components[normalizeRef(component.Ref)] = component
	}
	placedByRef := make(map[string]PlacementResult, len(placements))
	for _, placed := range placements {
		if placed.Reason == "" {
			placedByRef[normalizeRef(placed.Ref)] = placed
		}
	}
	for _, group := range request.Groups {
		anchorRef := normalizeRef(group.Anchor.Ref)
		component, componentOK := components[anchorRef]
		placed, placedOK := placedByRef[anchorRef]
		if !componentOK || component.Position == nil || !placedOK {
			continue
		}
		keepouts = translatedKeepoutsForGroup(keepouts, group.ID, *component.Position, placed.Position)
	}
	return keepouts
}

func translatedKeepoutsForGroup(keepouts []Keepout, groupID string, authoredAnchor Placement, placedAnchor Placement) []Keepout {
	translated := append([]Keepout(nil), keepouts...)
	deltaX := placedAnchor.XMM - authoredAnchor.XMM
	deltaY := placedAnchor.YMM - authoredAnchor.YMM
	for index := range translated {
		if !strings.EqualFold(strings.TrimSpace(translated[index].GroupID), strings.TrimSpace(groupID)) {
			continue
		}
		translated[index].Bounds.Min.XMM += deltaX
		translated[index].Bounds.Max.XMM += deltaX
		translated[index].Bounds.Min.YMM += deltaY
		translated[index].Bounds.Max.YMM += deltaY
	}
	return translated
}

func firstNonEmptyPlacementLayer(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return defaultPlacementLayer
}
