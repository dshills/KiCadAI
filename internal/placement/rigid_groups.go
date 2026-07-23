package placement

import (
	"math"
	"sort"
	"strings"
)

type relativeGroupCandidate struct {
	placements []PlacementResult
}

func relativeGroupIndexesByMember(groups []Group) map[string]int {
	result := map[string]int{}
	for groupIndex, group := range groups {
		if !group.TranslateAsUnit {
			continue
		}
		for _, ref := range group.Components {
			result[normalizeRef(ref)] = groupIndex
		}
	}
	return result
}

func relativeGroupOrder(groups []Group, components map[string]Component) []int {
	groupOrder := make([]int, 0, len(groups))
	for groupIndex, group := range groups {
		if group.TranslateAsUnit {
			groupOrder = append(groupOrder, groupIndex)
		}
	}
	sort.SliceStable(groupOrder, func(left, right int) bool {
		leftGroup := groups[groupOrder[left]]
		rightGroup := groups[groupOrder[right]]
		if leftGroup.Priority != rightGroup.Priority {
			return leftGroup.Priority > rightGroup.Priority
		}
		leftEdge := relativeGroupHasEdgeConstraint(leftGroup, components)
		rightEdge := relativeGroupHasEdgeConstraint(rightGroup, components)
		if leftEdge != rightEdge {
			return leftEdge
		}
		return leftGroup.ID < rightGroup.ID
	})
	return groupOrder
}

func relativeGroupHasEdgeConstraint(group Group, components map[string]Component) bool {
	for _, ref := range group.Components {
		if component, ok := components[normalizeRef(ref)]; ok && component.Edge != EdgeNone {
			return true
		}
	}
	return false
}

func placeRelativeGroup(request Request, group Group, components map[string]Component, existing []PlacementResult) (relativeGroupCandidate, bool) {
	var found relativeGroupCandidate
	forEachRelativeGroupPlacement(request, group, components, existing, func(candidate relativeGroupCandidate) bool {
		found = candidate
		return false
	})
	return found, len(found.placements) != 0
}

func placeRelativeGroupSet(request Request, groupOrder []int, components map[string]Component, existing []PlacementResult) ([]relativeGroupCandidate, *RigidGroupSearchReport) {
	report := &RigidGroupSearchReport{Enabled: true, GroupCount: len(groupOrder), RejectedByReason: map[string]int{}}
	if len(groupOrder) == 0 {
		report.Complete = true
		return nil, report
	}
	maxBranches := request.Rules.MaxCandidatesPerPart
	if maxBranches <= 0 {
		maxBranches = DefaultRules().MaxCandidatesPerPart
	}
	maxBranches *= len(groupOrder)
	report.BranchBudget = maxBranches
	branches := 0
	result := make([]relativeGroupCandidate, len(groupOrder))
	var search func(int, []PlacementResult) bool
	search = func(orderIndex int, placed []PlacementResult) bool {
		if orderIndex == len(groupOrder) {
			return true
		}
		group := request.Groups[groupOrder[orderIndex]]
		completed := false
		legalCandidates := 0
		forEachRelativeGroupPlacement(request, group, components, placed, func(candidate relativeGroupCandidate) bool {
			legalCandidates++
			branches++
			report.ExploredBranches = branches
			if branches > maxBranches {
				report.BudgetExhausted = true
				report.RejectedByReason["search_budget"]++
				return false
			}
			next := make([]PlacementResult, 0, len(placed)+len(candidate.placements))
			next = append(next, placed...)
			next = append(next, candidate.placements...)
			if search(orderIndex+1, next) {
				result[orderIndex] = candidate
				completed = true
				return false
			}
			report.Backtracks++
			report.RejectedByReason["downstream_infeasible"]++
			return branches <= maxBranches
		})
		if legalCandidates == 0 {
			report.RejectedByReason["no_legal_transform"]++
		}
		return completed
	}
	if !search(0, append([]PlacementResult(nil), existing...)) {
		return nil, report
	}
	report.Complete = true
	for orderIndex, candidate := range result {
		group := request.Groups[groupOrder[orderIndex]]
		anchorRef := normalizeRef(group.Anchor.Ref)
		for _, placed := range candidate.placements {
			if normalizeRef(placed.Ref) == anchorRef {
				report.Selected = append(report.Selected, RigidGroupSelectedPlacement{GroupID: group.ID, Anchor: placed.Ref, At: placed.Position})
				break
			}
		}
	}
	if len(report.RejectedByReason) == 0 {
		report.RejectedByReason = nil
	}
	return result, report
}

func forEachRelativeGroupPlacement(request Request, group Group, components map[string]Component, existing []PlacementResult, visit func(relativeGroupCandidate) bool) {
	request.Keepouts = keepoutsForRelativeGroupSearch(request, group.ID, existing, components)
	workingPlacements := make([]PlacementResult, 0, len(existing)+len(group.Components))
	workingPlacements = append(workingPlacements, existing...)
	placementIndexes := make(map[string]int, len(existing)+len(group.Components))
	hardRefs := make(map[string]struct{}, len(existing))
	for index, placement := range workingPlacements {
		ref := normalizeRef(placement.Ref)
		placementIndexes[ref] = index
		hardRefs[ref] = struct{}{}
	}
	for _, ref := range group.Components {
		componentRef := normalizeRef(ref)
		component, ok := components[componentRef]
		if !ok || component.Position == nil {
			return
		}
		placement, ok := NewPlacementResult(component, *component.Position, request.Rules)
		if !ok {
			return
		}
		placementIndexes[componentRef] = len(workingPlacements)
		workingPlacements = append(workingPlacements, placement)
	}
	visitRelativeGroupPlacements(request, group, components, workingPlacements, placementIndexes, hardRefs, visit)
}

func keepoutsForRelativeGroupSearch(request Request, groupID string, existing []PlacementResult, components map[string]Component) []Keepout {
	keepouts := translatedKeepoutsForPlacements(request, existing, components)
	for _, keepout := range request.Keepouts {
		if strings.EqualFold(strings.TrimSpace(keepout.GroupID), strings.TrimSpace(groupID)) {
			keepouts = append(keepouts, keepout)
		}
	}
	return keepouts
}

func findRelativeGroupPlacement(request Request, group Group, components map[string]Component, placements []PlacementResult, placementIndexes map[string]int, hardRefs map[string]struct{}) (relativeGroupCandidate, bool) {
	var found relativeGroupCandidate
	visitRelativeGroupPlacements(request, group, components, placements, placementIndexes, hardRefs, func(candidate relativeGroupCandidate) bool {
		found = candidate
		return false
	})
	return found, len(found.placements) != 0
}

func visitRelativeGroupPlacements(request Request, group Group, components map[string]Component, placements []PlacementResult, placementIndexes map[string]int, hardRefs map[string]struct{}, visit func(relativeGroupCandidate) bool) {
	anchorRef := normalizeRef(group.Anchor.Ref)
	anchorComponent, ok := components[anchorRef]
	if !ok || anchorComponent.Position == nil {
		return
	}
	anchorIndex, ok := placementIndexes[anchorRef]
	if !ok {
		return
	}
	target := placements[anchorIndex].Position
	if group.Anchor.At != nil {
		target.XMM = group.Anchor.At.XMM
		target.YMM = group.Anchor.At.YMM
	}
	// Authored block-local copper currently supports translation only.
	// Keep the source orientation and side while searching for a legal offset.
	target.RotationDeg = anchorComponent.Position.RotationDeg
	target.Layer = anchorComponent.Position.Layer
	members := make([]Component, 0, len(group.Components))
	memberRefs := make(map[string]struct{}, len(group.Components))
	for _, ref := range group.Components {
		component, exists := components[normalizeRef(ref)]
		if !exists || component.Position == nil {
			return
		}
		if _, exists := placementIndexes[normalizeRef(ref)]; !exists {
			return
		}
		members = append(members, component)
		memberRefs[normalizeRef(ref)] = struct{}{}
	}
	if _, included := memberRefs[anchorRef]; !included {
		return
	}
	sort.SliceStable(members, func(left, right int) bool {
		return normalizeRef(members[left].Ref) == anchorRef && normalizeRef(members[right].Ref) != anchorRef
	})
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
	boardCandidateCount := max(1, int(math.Floor((usable.Max.XMM-usable.Min.XMM)/grid))+1) * max(1, int(math.Floor((usable.Max.YMM-usable.Min.YMM)/grid))+1)
	// A rigid group has no legal partial result: either one shared transform is
	// found or every member loses its authored copper relationship. Permit the
	// deterministic ring search to cover the finite board before declaring that
	// no transform exists.
	maxCandidates = max(maxCandidates, boardCandidateCount)
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
			return visit(candidate)
		}
		return true
	})
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
	anchorRef := normalizeRef(requestGroupAnchorRef(request, groupID))
	anchorComponent := Component{}
	for _, member := range members {
		if normalizeRef(member.Ref) == anchorRef {
			anchorComponent = member
			break
		}
	}
	for _, group := range request.Groups {
		if !strings.EqualFold(strings.TrimSpace(group.ID), strings.TrimSpace(groupID)) || group.Bounds == nil {
			continue
		}
		bounds := translatedGroupBounds(*group.Bounds, authoredAnchor, anchor)
		if !relativeGroupBoundsContained(request, anchorComponent, anchor, bounds) {
			return relativeGroupCandidate{}, false
		}
		break
	}
	edgeTolerance := edgeConstraintTolerance(request.Board, request.Rules)
	translatedKeepouts := translatedKeepoutsForGroup(request.Keepouts, groupID, authoredAnchor, anchor)
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
		if normalizeRef(component.Ref) != normalizeRef(requestGroupAnchorRef(request, groupID)) {
			placement, ok = legalizeRelativeGroupMember(component, placement, result.placements, anchor, request.Rules)
			if !ok {
				return relativeGroupCandidate{}, false
			}
		}
		physicalBounds, ok := ComponentPhysicalBounds(component, placement.Position)
		if !ok || !usable.Contains(physicalBounds) {
			return relativeGroupCandidate{}, false
		}
		if component.Edge != EdgeNone && !edgeConstraintSatisfied(request.Board, component, placement.Position, component.Edge, edgeTolerance) {
			return relativeGroupCandidate{}, false
		}
		if rigidGroupKeepoutConflict(translatedKeepouts, groupID, placement) {
			return relativeGroupCandidate{}, false
		}
		layer := strings.ToUpper(strings.TrimSpace(placement.Position.Layer))
		for _, placed := range existingByLayer[layer] {
			if placed.Bounds.Intersects(placement.Bounds) {
				return relativeGroupCandidate{}, false
			}
		}
		result.placements = append(result.placements, placement)
	}
	return result, true
}

func relativeGroupBoundsContained(request Request, anchor Component, placement Placement, bounds Rect) bool {
	limit := BoardUsableRect(request.Board, request.Rules)
	if anchor.Edge == EdgeNone {
		return limit.Contains(bounds)
	}
	limit = Rect{
		Min: request.Board.Origin,
		Max: Point{XMM: request.Board.Origin.XMM + request.Board.WidthMM, YMM: request.Board.Origin.YMM + request.Board.HeightMM},
	}
	tolerance := edgeConstraintTolerance(request.Board, request.Rules)
	allowLeft := anchor.Edge == EdgeLeft || anchor.Edge == EdgeAny && edgeConstraintSatisfied(request.Board, anchor, placement, EdgeLeft, tolerance)
	allowRight := anchor.Edge == EdgeRight || anchor.Edge == EdgeAny && edgeConstraintSatisfied(request.Board, anchor, placement, EdgeRight, tolerance)
	allowTop := anchor.Edge == EdgeTop || anchor.Edge == EdgeAny && edgeConstraintSatisfied(request.Board, anchor, placement, EdgeTop, tolerance)
	allowBottom := anchor.Edge == EdgeBottom || anchor.Edge == EdgeAny && edgeConstraintSatisfied(request.Board, anchor, placement, EdgeBottom, tolerance)
	return (allowLeft || bounds.Min.XMM >= limit.Min.XMM) &&
		(allowRight || bounds.Max.XMM <= limit.Max.XMM) &&
		(allowTop || bounds.Min.YMM >= limit.Min.YMM) &&
		(allowBottom || bounds.Max.YMM <= limit.Max.YMM)
}

func legalizeRelativeGroupMember(component Component, placement PlacementResult, grouped []PlacementResult, anchor Placement, rules Rules) (PlacementResult, bool) {
	grid := rules.GridMM
	if grid <= 0 {
		grid = DefaultRules().GridMM
	}
	directionX := placement.Position.XMM - anchor.XMM
	directionY := placement.Position.YMM - anchor.YMM
	if math.Abs(directionX) <= placementCompareEpsilon && math.Abs(directionY) <= placementCompareEpsilon {
		for _, existing := range grouped {
			if strings.EqualFold(existing.Position.Layer, placement.Position.Layer) && existing.Bounds.Intersects(placement.Bounds) {
				return PlacementResult{}, false
			}
		}
		return placement, true
	}
	maximumAttempts := max(4, len(grouped)*4)
	for attempt := 0; attempt < maximumAttempts; attempt++ {
		conflict := PlacementResult{}
		found := false
		for _, existing := range grouped {
			if strings.EqualFold(existing.Position.Layer, placement.Position.Layer) && existing.Bounds.Intersects(placement.Bounds) {
				conflict = existing
				found = true
				break
			}
		}
		if !found {
			return placement, true
		}
		position := placement.Position
		if math.Abs(directionX) >= math.Abs(directionY) {
			if directionX >= 0 {
				position.XMM += conflict.Bounds.Max.XMM - placement.Bounds.Min.XMM + grid
			} else {
				position.XMM += conflict.Bounds.Min.XMM - placement.Bounds.Max.XMM - grid
			}
		} else if directionY >= 0 {
			position.YMM += conflict.Bounds.Max.YMM - placement.Bounds.Min.YMM + grid
		} else {
			position.YMM += conflict.Bounds.Min.YMM - placement.Bounds.Max.YMM - grid
		}
		var ok bool
		placement, ok = NewPlacementResult(component, position, rules)
		if !ok {
			return PlacementResult{}, false
		}
	}
	return PlacementResult{}, false
}

func requestGroupAnchorRef(request Request, groupID string) string {
	for _, group := range request.Groups {
		if strings.EqualFold(strings.TrimSpace(group.ID), strings.TrimSpace(groupID)) {
			return group.Anchor.Ref
		}
	}
	return ""
}

func rigidGroupKeepoutConflict(keepouts []Keepout, groupID string, placement PlacementResult) bool {
	ref := normalizeRef(placement.Ref)
	layer := normalizePlacementLayer(placement.Position).Layer
	for _, keepout := range keepouts {
		// A group-owned keepout reserves the translated authored cluster from
		// external components. Its members already have an immutable reviewed
		// relationship to that envelope, so applying it internally double-counts
		// coarse component bounds and can make every shared transform impossible.
		if strings.EqualFold(strings.TrimSpace(keepout.GroupID), strings.TrimSpace(groupID)) {
			continue
		}
		if keepout.Optional || keepoutExemptsNormalizedRef(keepout, ref) || !keepoutAppliesToLayer(keepout, layer) {
			continue
		}
		if keepout.Bounds.Intersects(placement.Bounds) {
			return true
		}
	}
	return false
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
// keepouts owned by non-translated semantic groups remain active at authored
// coordinates until their anchor is placed. Keepouts owned by translated groups
// remain inactive until the group's anchor has a successful placement,
// preventing stale authored coordinates from constraining groups that have not
// yet been searched. Once any group anchor is placed, its keepouts follow it.
func TranslatedKeepoutsForPlacements(request Request, placements []PlacementResult) []Keepout {
	return translatedKeepoutsForPlacements(request, placements, nil)
}

func translatedKeepoutsForPlacements(request Request, placements []PlacementResult, components map[string]Component) []Keepout {
	if components == nil {
		components = make(map[string]Component, len(request.Components))
		for _, component := range request.Components {
			components[normalizeRef(component.Ref)] = component
		}
	}
	placedByRef := make(map[string]PlacementResult, len(placements))
	for _, placed := range placements {
		if placed.Reason == "" {
			placedByRef[normalizeRef(placed.Ref)] = placed
		}
	}
	type groupTransform struct {
		authored Placement
		placed   Placement
	}
	transforms := make(map[string]groupTransform, len(request.Groups))
	translatedGroups := make(map[string]struct{}, len(request.Groups))
	for _, group := range request.Groups {
		groupID := normalizeRef(group.ID)
		if group.TranslateAsUnit {
			translatedGroups[groupID] = struct{}{}
		}
		anchorRef := normalizeRef(group.Anchor.Ref)
		component, componentOK := components[anchorRef]
		placed, placedOK := placedByRef[anchorRef]
		if !componentOK || component.Position == nil || !placedOK {
			continue
		}
		transforms[groupID] = groupTransform{authored: *component.Position, placed: placed.Position}
	}
	keepouts := make([]Keepout, 0, len(request.Keepouts))
	for _, keepout := range request.Keepouts {
		groupID := normalizeRef(keepout.GroupID)
		if groupID == "" {
			keepouts = append(keepouts, keepout)
			continue
		}
		transform, ok := transforms[groupID]
		if ok {
			translated := translatedKeepoutsForGroup([]Keepout{keepout}, keepout.GroupID, transform.authored, transform.placed)
			keepouts = append(keepouts, translated[0])
			continue
		}
		if _, translated := translatedGroups[groupID]; translated {
			continue
		}
		keepouts = append(keepouts, keepout)
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
