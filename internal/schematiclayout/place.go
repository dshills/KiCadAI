package schematiclayout

import (
	"sort"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func Place(request Request) Result {
	request = Classify(request)
	rules := normalizeRules(request.Rules)
	cells, islandCount, rankCount := planPlacement(request)
	if rules.MaxAuxiliaryPerRank > 0 {
		cells, rankCount = spreadAuxiliaryLaneRanks(request.Components, cells, rules.MaxAuxiliaryPerRank)
	}
	rankX := placementRankX(request.Components, cells, rules)
	positions := placementPositions(request.Components, cells, rankX, rules)
	relationsConverged := enforceRelativePlacement(request.Components, positions, rules)
	overlapRepairs, overlapsConverged := repairPlacementOverlaps(request.Components, positions, rules)
	result := Result{Sheet: request.Sheet, Components: make([]PlacedComponent, 0, len(request.Components))}
	for _, component := range request.Components {
		placed := PlacedComponent{Component: component}
		if component.Fixed {
			placed.PlacedAt = SnapPoint(component.Position, rules.Grid)
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityInfo, Code: "fixed_component", Ref: component.Ref, Message: "fixed schematic coordinate preserved"})
		} else {
			placed.PlacedAt = positions[component.Ref]
		}
		result.Components = append(result.Components, placed)
	}
	if !relationsConverged {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityError, Code: "relative_placement_not_converged", Message: "relative placement constraints did not converge", Repair: "remove relation cycles or increase compatible group/lane spacing"})
	}
	if overlapRepairs > 0 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityInfo, Code: "placement_overlap_repaired", Message: "moved constrained components along a free axis to preserve symbol clearance"})
	}
	if !overlapsConverged {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityError, Code: "placement_overlap_not_repaired", Message: "component overlap could not be repaired without breaking placement constraints", Repair: "relax a row or column alignment constraint or increase compatible lane spacing"})
	}
	var textDiagnostics []Diagnostic
	result.Components, textDiagnostics = placeComponentText(result.Components, rules)
	result.Diagnostics = append(result.Diagnostics, textDiagnostics...)
	if !hasFixedComponent(request.Components) {
		bounds := placementDrawingBounds(result.Components)
		offset := centerOffset(bounds, UsableSheet(request.Sheet), rules.Grid)
		for index := range result.Components {
			result.Components[index].PlacedAt.X += offset.X
			result.Components[index].PlacedAt.Y += offset.Y
		}
		result.Report.CenterOffset = offset
	}
	var anchorDiagnostics []Diagnostic
	result.Components, anchorDiagnostics = resolveCanonicalPinAnchorPositions(result.Components)
	result.Diagnostics = append(result.Diagnostics, anchorDiagnostics...)
	result.Report.IslandCount = islandCount
	result.Report.RankCount = rankCount
	result.Report.OccupiedBounds = placementDrawingBounds(result.Components)
	result = Validate(result, request)
	result.Diagnostics = append(result.Diagnostics, placementDiagnostics(result.Components, request.Sheet)...)
	return NormalizeResult(result, rules)
}

func repairPlacementOverlaps(components []Component, positions map[string]kicadfiles.Point, rules Rules) (int, bool) {
	repairs := 0
	for repairs < rules.MaxSpacingRepairs {
		leftIndex, rightIndex, found := firstPlacementOverlap(components, positions)
		if !found {
			return repairs, true
		}
		moved := false
		for _, pair := range [][2]int{{rightIndex, leftIndex}, {leftIndex, rightIndex}} {
			moving := components[pair[0]]
			if moving.Fixed {
				continue
			}
			obstacle := componentBoundsAt(components[pair[1]], positions[components[pair[1]].Ref])
			for _, candidate := range overlapRepairCandidates(moving, positions[moving.Ref], obstacle, rules) {
				original := positions[moving.Ref]
				positions[moving.Ref] = candidate
				if relativePositionsSatisfied(components, positions, rules) && placementPositionClear(moving.Ref, components, positions) {
					moved = true
					repairs++
					break
				}
				positions[moving.Ref] = original
			}
			if moved {
				break
			}
		}
		if !moved {
			return repairs, false
		}
	}
	_, _, found := firstPlacementOverlap(components, positions)
	return repairs, !found
}

func firstPlacementOverlap(components []Component, positions map[string]kicadfiles.Point) (int, int, bool) {
	for left := 0; left < len(components); left++ {
		leftBounds := placementOverlapBounds(components[left], positions[components[left].Ref])
		for right := left + 1; right < len(components); right++ {
			if leftBounds.Intersects(placementOverlapBounds(components[right], positions[components[right].Ref])) {
				return left, right, true
			}
		}
	}
	return 0, 0, false
}

func overlapRepairCandidates(component Component, position kicadfiles.Point, obstacle Rect, rules Rules) []kicadfiles.Point {
	local := TransformRect(DefaultBodyFor(PlacedComponent{Component: component}), component.Rotation, component.Mirror)
	candidates := []kicadfiles.Point{
		{X: position.X, Y: snapAtLeast(obstacle.MaxY+rules.MinComponentSpacing-local.MinY, rules.Grid)},
		{X: position.X, Y: snapAtMost(obstacle.MinY-rules.MinComponentSpacing-local.MaxY, rules.Grid)},
		{X: snapAtLeast(obstacle.MaxX+rules.MinComponentSpacing-local.MinX, rules.Grid), Y: position.Y},
		{X: snapAtMost(obstacle.MinX-rules.MinComponentSpacing-local.MaxX, rules.Grid), Y: position.Y},
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := absIU(candidates[i].X-position.X) + absIU(candidates[i].Y-position.Y)
		right := absIU(candidates[j].X-position.X) + absIU(candidates[j].Y-position.Y)
		return left < right
	})
	return candidates
}

func placementPositionClear(ref string, components []Component, positions map[string]kicadfiles.Point) bool {
	var moving Component
	found := false
	for _, component := range components {
		if component.Ref == ref {
			moving = component
			found = true
			break
		}
	}
	if !found {
		return false
	}
	bounds := componentBoundsAt(moving, positions[ref])
	for _, component := range components {
		if component.Ref != ref && shrinkRect(bounds, kicadfiles.MM(0.5)).Intersects(placementOverlapBounds(component, positions[component.Ref])) {
			return false
		}
	}
	return true
}

func placementOverlapBounds(component Component, position kicadfiles.Point) Rect {
	return shrinkRect(componentBoundsAt(component, position), kicadfiles.MM(0.5))
}

// spreadAuxiliaryLaneRanks prevents dense feedback, bias, rail, and passive
// signal branches from becoming a single tall column. Active devices in the
// main signal lane are deliberately unchanged so complementary pairs and
// parallel gain stages retain their conventional vertical alignment.
func spreadAuxiliaryLaneRanks(components []Component, cells map[string]placementCell, limit int) (map[string]placementCell, int) {
	if limit <= 0 {
		return cells, len(sortedPlacementRanks(cells))
	}
	type bucketKey struct {
		rank int
		lane Lane
	}
	buckets := map[bucketKey][]Component{}
	for _, component := range components {
		if !auxiliaryRankCandidate(component) {
			continue
		}
		cell := cells[component.Ref]
		key := bucketKey{rank: cell.rank, lane: component.Lane}
		buckets[key] = append(buckets[key], component)
	}
	subrankByRef := map[string]int{}
	widthByRank := map[int]int{}
	for key, items := range buckets {
		sort.SliceStable(items, func(i, j int) bool {
			left, right := cells[items[i].Ref], cells[items[j].Ref]
			if left.island != right.island {
				return left.island < right.island
			}
			if left.order != right.order {
				return left.order < right.order
			}
			return items[i].Ref < items[j].Ref
		})
		for index, component := range items {
			subrank := index / limit
			subrankByRef[component.Ref] = subrank
			if subrank+1 > widthByRank[key.rank] {
				widthByRank[key.rank] = subrank + 1
			}
		}
	}
	ranks := sortedPlacementRanks(cells)
	baseByRank := map[int]int{}
	next := 0
	for _, rank := range ranks {
		baseByRank[rank] = next
		width := widthByRank[rank]
		if width < 1 {
			width = 1
		}
		next += width
	}
	spread := make(map[string]placementCell, len(cells))
	for ref, cell := range cells {
		cell.rank = baseByRank[cell.rank] + subrankByRef[ref]
		spread[ref] = cell
	}
	return spread, len(sortedPlacementRanks(spread))
}

func auxiliaryRankCandidate(component Component) bool {
	if component.Lane != LaneSignal && component.Lane != LaneUnknown {
		return true
	}
	return containsNormalizedRole(
		component.Role,
		"resistor", "capacitor", "inductor", "diode", "protection", "fuse", "tvs",
	)
}

func sortedPlacementRanks(cells map[string]placementCell) []int {
	seen := map[int]struct{}{}
	for _, cell := range cells {
		seen[cell.rank] = struct{}{}
	}
	ranks := make([]int, 0, len(seen))
	for rank := range seen {
		ranks = append(ranks, rank)
	}
	sort.Ints(ranks)
	return ranks
}

func resolveCanonicalPinAnchorPositions(components []PlacedComponent) ([]PlacedComponent, []Diagnostic) {
	resolved := append([]PlacedComponent(nil), components...)
	order := make([]int, len(resolved))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		a, b := resolved[order[left]], resolved[order[right]]
		if a.OriginalOrdinal != b.OriginalOrdinal {
			return a.OriginalOrdinal < b.OriginalOrdinal
		}
		return a.Ref < b.Ref
	})
	occupied := map[kicadfiles.Point]struct{}{}
	var diagnostics []Diagnostic
	for _, index := range order {
		component := &resolved[index]
		offsets := make([]kicadfiles.Point, 0, len(component.Pins))
		for _, pin := range component.Pins {
			offsets = append(offsets, pin.At)
		}
		position, ok := schematic.CollisionFreeSymbolPosition(component.PlacedAt, offsets, component.Rotation, schematic.SymbolMirror(component.Mirror), occupied)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "pin_anchor_collision_unresolved", Ref: component.Ref, Message: "no collision-free canonical pin-anchor position", Repair: "increase component spacing or provide a different relative placement"})
			continue
		}
		if position != component.PlacedAt {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityInfo, Code: "pin_anchor_collision_adjusted", Ref: component.Ref, Message: "moved symbol to preserve distinct canonical pin anchors"})
			component.PlacedAt = position
		}
		for _, offset := range offsets {
			occupied[schematic.CanonicalConnectionAnchor(component.PlacedAt, offset, component.Rotation, schematic.SymbolMirror(component.Mirror))] = struct{}{}
		}
	}
	return resolved, diagnostics
}

func enforceRelativePlacement(components []Component, positions map[string]kicadfiles.Point, rules Rules) bool {
	byRef := make(map[string]Component, len(components))
	aboveByTarget := map[string][]Component{}
	for _, component := range components {
		byRef[component.Ref] = component
		if component.Fixed {
			continue
		}
		for _, targetRef := range component.Above {
			aboveByTarget[targetRef] = append(aboveByTarget[targetRef], component)
		}
	}
	targetRefs := make([]string, 0, len(aboveByTarget))
	for targetRef := range aboveByTarget {
		targetRefs = append(targetRefs, targetRef)
	}
	sort.Strings(targetRefs)
	// Keep direct Request callers safe even when they bypass schematic IR
	// validation and provide contradictory or cyclic alignment constraints.
	maxIterations := len(components) * 2
	for iteration := 0; iteration < maxIterations; iteration++ {
		changed := false
		for _, component := range components {
			position, ok := positions[component.Ref]
			if !ok || component.Fixed {
				continue
			}
			componentBounds := componentBoundsAt(component, position)
			for _, targetRef := range component.RightOf {
				target, targetOK := byRef[targetRef]
				targetPosition, positionOK := positions[targetRef]
				if !targetOK || !positionOK {
					continue
				}
				targetBounds := componentBoundsAt(target, targetPosition)
				minimumX := targetBounds.MaxX + rules.MinComponentSpacing
				if componentBounds.MinX < minimumX {
					delta := minimumX - componentBounds.MinX
					position.X = snapAtLeast(position.X+delta, rules.Grid)
					positions[component.Ref] = position
					componentBounds = componentBoundsAt(component, position)
					changed = true
				}
			}
		}
		for _, targetRef := range targetRefs {
			target, targetOK := byRef[targetRef]
			targetPosition, positionOK := positions[targetRef]
			if !targetOK || !positionOK {
				continue
			}
			maximumY := componentBoundsAt(target, targetPosition).MinY - rules.MinComponentSpacing
			groupBottom := kicadfiles.IU(-1 << 62)
			for _, component := range aboveByTarget[targetRef] {
				if bounds := componentBoundsAt(component, positions[component.Ref]); bounds.MaxY > groupBottom {
					groupBottom = bounds.MaxY
				}
			}
			if groupBottom <= maximumY {
				continue
			}
			delta := groupBottom - maximumY
			for _, component := range aboveByTarget[targetRef] {
				position := positions[component.Ref]
				position.Y = snapAtMost(position.Y-delta, rules.Grid)
				positions[component.Ref] = position
			}
			changed = true
		}
		for _, component := range components {
			position, ok := positions[component.Ref]
			if !ok || component.Fixed {
				continue
			}
			for _, targetRef := range component.SameRowAs {
				targetPosition, targetOK := positions[targetRef]
				if !targetOK || position.Y == targetPosition.Y {
					continue
				}
				position.Y = targetPosition.Y
				positions[component.Ref] = position
				changed = true
			}
		}
		for _, component := range components {
			position, ok := positions[component.Ref]
			if !ok || component.Fixed {
				continue
			}
			for _, targetRef := range component.SameColumnAs {
				targetPosition, targetOK := positions[targetRef]
				if !targetOK || position.X == targetPosition.X {
					continue
				}
				position.X = targetPosition.X
				positions[component.Ref] = position
				changed = true
			}
		}
		for _, component := range components {
			position, ok := positions[component.Ref]
			if !ok || component.Fixed || len(component.CenterBetween) != 2 {
				continue
			}
			left, leftOK := positions[component.CenterBetween[0]]
			right, rightOK := positions[component.CenterBetween[1]]
			if !leftOK || !rightOK {
				continue
			}
			centerX := SnapIU(left.X+(right.X-left.X)/2, rules.Grid)
			if position.X == centerX {
				continue
			}
			position.X = centerX
			positions[component.Ref] = position
			changed = true
		}
		for _, component := range components {
			position, ok := positions[component.Ref]
			if !ok || component.Fixed {
				continue
			}
			for _, endpoint := range component.SameRowAsPin {
				anchor, anchorOK := relativeEndpointAnchor(byRef, positions, endpoint)
				if !anchorOK || position.Y == anchor.Y {
					continue
				}
				position.Y = anchor.Y
				positions[component.Ref] = position
				changed = true
			}
			for _, endpoint := range component.SameColumnAsPin {
				anchor, anchorOK := relativeEndpointAnchor(byRef, positions, endpoint)
				if !anchorOK || position.X == anchor.X {
					continue
				}
				position.X = anchor.X
				positions[component.Ref] = position
				changed = true
			}
		}
		if !changed {
			return true
		}
	}
	return relativePositionsSatisfied(components, positions, rules)
}

func relativePositionsSatisfied(components []Component, positions map[string]kicadfiles.Point, rules Rules) bool {
	byRef := make(map[string]Component, len(components))
	for _, component := range components {
		byRef[component.Ref] = component
	}
	for _, component := range components {
		bounds := componentBoundsAt(component, positions[component.Ref])
		for _, targetRef := range component.RightOf {
			target, ok := byRef[targetRef]
			if ok && bounds.MinX < componentBoundsAt(target, positions[targetRef]).MaxX+rules.MinComponentSpacing {
				return false
			}
		}
		for _, targetRef := range component.Above {
			target, ok := byRef[targetRef]
			if ok && bounds.MaxY > componentBoundsAt(target, positions[targetRef]).MinY-rules.MinComponentSpacing {
				return false
			}
		}
		for _, targetRef := range component.SameRowAs {
			targetPosition, ok := positions[targetRef]
			if ok && positions[component.Ref].Y != targetPosition.Y {
				return false
			}
		}
		for _, targetRef := range component.SameColumnAs {
			targetPosition, ok := positions[targetRef]
			if ok && positions[component.Ref].X != targetPosition.X {
				return false
			}
		}
		for _, endpoint := range component.SameRowAsPin {
			anchor, ok := relativeEndpointAnchor(byRef, positions, endpoint)
			if ok && positions[component.Ref].Y != anchor.Y {
				return false
			}
		}
		for _, endpoint := range component.SameColumnAsPin {
			anchor, ok := relativeEndpointAnchor(byRef, positions, endpoint)
			if ok && positions[component.Ref].X != anchor.X {
				return false
			}
		}
		if len(component.CenterBetween) == 2 {
			left, leftOK := positions[component.CenterBetween[0]]
			right, rightOK := positions[component.CenterBetween[1]]
			if leftOK && rightOK && positions[component.Ref].X != SnapIU(left.X+(right.X-left.X)/2, rules.Grid) {
				return false
			}
		}
	}
	return true
}

func relativeEndpointAnchor(components map[string]Component, positions map[string]kicadfiles.Point, endpoint Endpoint) (kicadfiles.Point, bool) {
	component, ok := components[endpoint.Ref]
	if !ok {
		return kicadfiles.Point{}, false
	}
	position, ok := positions[endpoint.Ref]
	if !ok {
		return kicadfiles.Point{}, false
	}
	for _, pin := range component.Pins {
		if pin.Number != endpoint.Pin {
			continue
		}
		return schematic.CanonicalConnectionAnchor(position, pin.At, component.Rotation, schematic.SymbolMirror(component.Mirror)), true
	}
	return kicadfiles.Point{}, false
}

func snapAtLeast(value, grid kicadfiles.IU) kicadfiles.IU {
	if grid <= 0 {
		return value
	}
	quotient := value / grid
	remainder := value % grid
	if remainder > 0 {
		quotient++
	}
	return quotient * grid
}

func snapAtMost(value, grid kicadfiles.IU) kicadfiles.IU {
	if grid <= 0 {
		return value
	}
	quotient := value / grid
	remainder := value % grid
	if remainder < 0 {
		quotient--
	}
	return quotient * grid
}

func componentBoundsAt(component Component, position kicadfiles.Point) Rect {
	return componentBody(PlacedComponent{Component: component, PlacedAt: position})
}

func placementRankX(components []Component, cells map[string]placementCell, rules Rules) map[int]kicadfiles.IU {
	maxHalfWidth := map[int]kicadfiles.IU{}
	var ranks []int
	seen := map[int]struct{}{}
	for _, component := range components {
		cell := cells[component.Ref]
		body := TransformRect(DefaultBodyFor(PlacedComponent{Component: component}), component.Rotation, component.Mirror)
		halfWidth := body.Width() / 2
		if halfWidth > maxHalfWidth[cell.rank] {
			maxHalfWidth[cell.rank] = halfWidth
		}
		if _, ok := seen[cell.rank]; !ok {
			seen[cell.rank] = struct{}{}
			ranks = append(ranks, cell.rank)
		}
	}
	sort.Ints(ranks)
	positions := map[int]kicadfiles.IU{}
	var previous int
	for index, rank := range ranks {
		if index == 0 {
			positions[rank] = maxHalfWidth[rank]
		} else {
			gap := rules.MinStageSpacing
			if gap < rules.MinComponentSpacing {
				gap = rules.MinComponentSpacing
			}
			positions[rank] = positions[previous] + maxHalfWidth[previous] + gap + maxHalfWidth[rank]
		}
		previous = rank
	}
	return positions
}

func placementPositions(components []Component, cells map[string]placementCell, rankX map[int]kicadfiles.IU, rules Rules) map[string]kicadfiles.Point {
	sharedRankGroups := sharedRankPlacementGroups(components, cells)
	byLane := map[Lane][]Component{}
	for _, component := range components {
		lane := component.Lane
		if lane == LaneUnknown {
			lane = LaneSignal
		}
		byLane[lane] = append(byLane[lane], component)
	}
	laneOrder := []Lane{LanePositiveRail, LaneSignal, LaneReference, LaneGround, LaneNegativeRail}
	positions := map[string]kicadfiles.Point{}
	y := kicadfiles.IU(0)
	for _, lane := range laneOrder {
		items := byLane[lane]
		sort.SliceStable(items, func(i, j int) bool {
			left, right := cells[items[i].Ref], cells[items[j].Ref]
			if left.island != right.island {
				return left.island < right.island
			}
			if left.order != right.order {
				return left.order < right.order
			}
			if left.rank != right.rank {
				return left.rank < right.rank
			}
			return items[i].Ref < items[j].Ref
		})
		if hasGroupBoundary(items, sharedRankGroups) {
			y = placeGroupedRankRows(items, cells, rankX, positions, y, rules, sharedRankGroups)
		} else {
			y = placeLaneRows(items, cells, rankX, positions, y, rules)
		}
		if len(items) != 0 {
			y += rules.MinGroupGutter
		}
	}
	return positions
}

func sharedRankPlacementGroups(components []Component, cells map[string]placementCell) map[string]bool {
	groupCounts := map[int]map[string]struct{}{}
	for _, component := range components {
		if component.GroupID == "" {
			continue
		}
		rank := cells[component.Ref].rank
		if groupCounts[rank] == nil {
			groupCounts[rank] = map[string]struct{}{}
		}
		groupCounts[rank][component.GroupID] = struct{}{}
	}
	explicit := map[string]bool{}
	for _, groups := range groupCounts {
		if len(groups) < 2 {
			continue
		}
		for group := range groups {
			explicit[group] = true
		}
	}
	return explicit
}

func hasGroupBoundary(items []Component, sharedRankGroups map[string]bool) bool {
	for _, component := range items {
		if sharedRankGroups[component.GroupID] {
			return true
		}
	}
	return false
}

func placeLaneRows(items []Component, cells map[string]placementCell, rankX map[int]kicadfiles.IU, positions map[string]kicadfiles.Point, y kicadfiles.IU, rules Rules) kicadfiles.IU {
	rowItems, attached := partitionAttachedAnnotations(items, cells)
	rowByRank := map[int]int{}
	rowHeight := map[int]kicadfiles.IU{}
	for _, component := range rowItems {
		rank := cells[component.Ref].rank
		row := rowByRank[rank]
		rowByRank[rank]++
		body := TransformRect(DefaultBodyFor(PlacedComponent{Component: component}), component.Rotation, component.Mirror)
		height := body.Height() + rules.MinTextSpacing*2
		if height > rowHeight[row] {
			rowHeight[row] = height
		}
	}
	rowY := map[int]kicadfiles.IU{}
	rows := 0
	for _, count := range rowByRank {
		if count > rows {
			rows = count
		}
	}
	for row := 0; row < rows; row++ {
		height := rowHeight[row]
		if height == 0 {
			height = kicadfiles.MM(7.62)
		}
		rowY[row] = y + height/2
		y += height + rules.MinComponentSpacing
	}
	usedRows := map[int]int{}
	for _, component := range rowItems {
		rank := cells[component.Ref].rank
		row := usedRows[rank]
		usedRows[rank]++
		positions[component.Ref] = SnapPoint(kicadfiles.Point{X: rankX[rank], Y: rowY[row]}, rules.Grid)
	}
	placeAttachedAnnotations(attached, rowItems, positions, rules)
	return y
}

// placeGroupedRankRows adds a visual gutter only where multiple semantic
// groups share a rank. Different ranks are already separated horizontally;
// applying a vertical gutter between them would distort ordinary signal flow.
func placeGroupedRankRows(items []Component, cells map[string]placementCell, rankX map[int]kicadfiles.IU, positions map[string]kicadfiles.Point, y kicadfiles.IU, rules Rules, sharedRankGroups map[string]bool) kicadfiles.IU {
	byRank := map[int][]Component{}
	var ranks []int
	for _, component := range items {
		rank := cells[component.Ref].rank
		if _, exists := byRank[rank]; !exists {
			ranks = append(ranks, rank)
		}
		byRank[rank] = append(byRank[rank], component)
	}
	sort.Ints(ranks)
	endY := y
	for _, rank := range ranks {
		rankItems := byRank[rank]
		sort.SliceStable(rankItems, func(i, j int) bool {
			leftGroup, rightGroup := groupedRankKey(rankItems[i], sharedRankGroups), groupedRankKey(rankItems[j], sharedRankGroups)
			if leftGroup != rightGroup {
				return leftGroup < rightGroup
			}
			left, right := cells[rankItems[i].Ref], cells[rankItems[j].Ref]
			if left.order != right.order {
				return left.order < right.order
			}
			return rankItems[i].Ref < rankItems[j].Ref
		})
		rowItems, attached := partitionAttachedAnnotations(rankItems, cells)
		rankY := y
		previousGroup := ""
		for index, component := range rowItems {
			group := groupedRankKey(component, sharedRankGroups)
			if index != 0 && group != previousGroup {
				rankY += rules.MinGroupGutter
			}
			body := TransformRect(DefaultBodyFor(PlacedComponent{Component: component}), component.Rotation, component.Mirror)
			height := body.Height() + rules.MinTextSpacing*2
			if height == 0 {
				height = kicadfiles.MM(7.62)
			}
			positions[component.Ref] = SnapPoint(kicadfiles.Point{X: rankX[rank], Y: rankY + height/2}, rules.Grid)
			rankY += height + rules.MinComponentSpacing
			previousGroup = group
		}
		placeAttachedAnnotations(attached, rowItems, positions, rules)
		if rankY > endY {
			endY = rankY
		}
	}
	return endY
}

type attachedAnnotation struct {
	component Component
	targetRef string
}

func partitionAttachedAnnotations(items []Component, cells map[string]placementCell) ([]Component, []attachedAnnotation) {
	byRef := make(map[string]Component, len(items))
	for _, item := range items {
		byRef[item.Ref] = item
	}
	var rows []Component
	var attached []attachedAnnotation
	for _, item := range items {
		isAnnotation := containsNormalizedRole(normalizeRole(item.Role), "annotation")
		if isAnnotation {
			for _, targetRef := range item.Near {
				target, exists := byRef[targetRef]
				if !exists || targetRef == item.Ref || cells[target.Ref].rank != cells[item.Ref].rank {
					continue
				}
				attached = append(attached, attachedAnnotation{component: item, targetRef: targetRef})
				isAnnotation = false
				break
			}
			if !isAnnotation {
				continue
			}
		}
		rows = append(rows, item)
	}
	return rows, attached
}

func placeAttachedAnnotations(attached []attachedAnnotation, rowItems []Component, positions map[string]kicadfiles.Point, rules Rules) {
	components := make(map[string]Component, len(rowItems)+len(attached))
	for _, component := range rowItems {
		components[component.Ref] = component
	}
	for _, annotation := range attached {
		components[annotation.component.Ref] = annotation.component
	}
	leftBounds := map[string]Rect{}
	occupied := make([]Rect, 0, len(rowItems)+len(attached))
	for _, component := range rowItems {
		if position, exists := positions[component.Ref]; exists {
			occupied = append(occupied, componentBoundsAt(component, position))
		}
	}
	for _, annotation := range attached {
		targetPosition, exists := positions[annotation.targetRef]
		if !exists {
			continue
		}
		targetBounds, exists := leftBounds[annotation.targetRef]
		if !exists {
			targetBounds = componentBoundsAt(components[annotation.targetRef], targetPosition)
		}
		localBounds := TransformRect(DefaultBodyFor(PlacedComponent{Component: annotation.component}), annotation.component.Rotation, annotation.component.Mirror)
		preferred := SnapPoint(kicadfiles.Point{
			X: targetBounds.MinX - rules.MinComponentSpacing - localBounds.MaxX,
			Y: targetPosition.Y,
		}, rules.Grid)
		candidates := []kicadfiles.Point{
			preferred,
			SnapPoint(kicadfiles.Point{X: targetBounds.MaxX + rules.MinComponentSpacing - localBounds.MinX, Y: targetPosition.Y}, rules.Grid),
			SnapPoint(kicadfiles.Point{X: targetPosition.X, Y: targetBounds.MinY - rules.MinComponentSpacing - localBounds.MaxY}, rules.Grid),
			SnapPoint(kicadfiles.Point{X: targetPosition.X, Y: targetBounds.MaxY + rules.MinComponentSpacing - localBounds.MinY}, rules.Grid),
		}
		position := preferred
		for _, candidate := range candidates {
			bounds := componentBoundsAt(annotation.component, candidate)
			collides := false
			for _, other := range occupied {
				if bounds.Intersects(other) {
					collides = true
					break
				}
			}
			if !collides {
				position = candidate
				break
			}
		}
		positions[annotation.component.Ref] = position
		leftBounds[annotation.targetRef] = componentBoundsAt(annotation.component, position)
		occupied = append(occupied, leftBounds[annotation.targetRef])
		if leftBounds[annotation.targetRef].MinX > targetBounds.MinX {
			leftBounds[annotation.targetRef] = targetBounds
		}
	}
}

func groupedRankKey(component Component, sharedRankGroups map[string]bool) string {
	if sharedRankGroups[component.GroupID] {
		return component.GroupID
	}
	return "\xffungrouped"
}

func placementBounds(components []PlacedComponent) Rect {
	var bounds Rect
	for _, component := range components {
		bounds = unionRect(bounds, componentBody(component))
	}
	return bounds
}

func placementDrawingBounds(components []PlacedComponent) Rect {
	bounds := placementBounds(components)
	for _, component := range components {
		if !component.ReferenceText.Box.Empty() {
			bounds = unionRect(bounds, component.ReferenceText.Box.Translate(component.PlacedAt))
		}
		if !component.ValueText.Box.Empty() {
			bounds = unionRect(bounds, component.ValueText.Box.Translate(component.PlacedAt))
		}
	}
	return bounds
}

func unionRect(first, second Rect) Rect {
	if first.Empty() {
		return second
	}
	if second.Empty() {
		return first
	}
	return Rect{
		MinX: minIU(first.MinX, second.MinX),
		MinY: minIU(first.MinY, second.MinY),
		MaxX: maxIU(first.MaxX, second.MaxX),
		MaxY: maxIU(first.MaxY, second.MaxY),
	}
}

func centerOffset(bounds, usable Rect, grid kicadfiles.IU) kicadfiles.Point {
	if bounds.Empty() || usable.Empty() {
		return kicadfiles.Point{}
	}
	return SnapPoint(kicadfiles.Point{
		X: (usable.MinX+usable.MaxX)/2 - (bounds.MinX+bounds.MaxX)/2,
		Y: (usable.MinY+usable.MaxY)/2 - (bounds.MinY+bounds.MaxY)/2,
	}, grid)
}

func hasFixedComponent(components []Component) bool {
	for _, component := range components {
		if component.Fixed {
			return true
		}
	}
	return false
}

func minIU(first, second kicadfiles.IU) kicadfiles.IU {
	if first < second {
		return first
	}
	return second
}

func maxIU(first, second kicadfiles.IU) kicadfiles.IU {
	if first > second {
		return first
	}
	return second
}

func defaultReferenceText(component PlacedComponent) TextBox {
	if !component.ReferenceText.Box.Empty() {
		return component.ReferenceText
	}
	body := componentBody(component)
	text := component.DisplayRef
	if text == "" {
		text = component.Ref
	}
	return TextBox{
		Text: text,
		Box:  Rect{MinX: body.MinX - component.PlacedAt.X, MinY: body.MinY - component.PlacedAt.Y - kicadfiles.MM(2.54), MaxX: body.MaxX - component.PlacedAt.X, MaxY: body.MinY - component.PlacedAt.Y},
	}
}

func defaultValueText(component PlacedComponent) TextBox {
	if !component.ValueText.Box.Empty() {
		return component.ValueText
	}
	body := componentBody(component)
	return TextBox{
		Text: component.Value,
		Box:  Rect{MinX: body.MinX - component.PlacedAt.X, MinY: body.MaxY - component.PlacedAt.Y, MaxX: body.MaxX - component.PlacedAt.X, MaxY: body.MaxY - component.PlacedAt.Y + kicadfiles.MM(2.54)},
	}
}

func placementDiagnostics(components []PlacedComponent, sheet Sheet) []Diagnostic {
	usable := UsableSheet(sheet)
	var diagnostics []Diagnostic
	for _, component := range components {
		body := componentBody(component)
		if component.BodyKnown && body.Empty() {
			continue
		}
		if !usable.ContainsRect(body) {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "page_overflow", Ref: component.Ref, Message: "placed component is outside the preferred readable area"})
		}
	}
	return diagnostics
}
