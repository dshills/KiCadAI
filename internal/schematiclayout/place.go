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
	rankX := placementRankX(request.Components, cells, rules)
	positions := placementPositions(request.Components, cells, rankX, rules)
	relationsConverged := enforceRelativePlacement(request.Components, positions, rules)
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
	}
	return true
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
	rowByRank := map[int]int{}
	rowHeight := map[int]kicadfiles.IU{}
	for _, component := range items {
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
	for _, component := range items {
		rank := cells[component.Ref].rank
		row := usedRows[rank]
		usedRows[rank]++
		positions[component.Ref] = SnapPoint(kicadfiles.Point{X: rankX[rank], Y: rowY[row]}, rules.Grid)
	}
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
		rankY := y
		previousGroup := ""
		for index, component := range rankItems {
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
		if rankY > endY {
			endY = rankY
		}
	}
	return endY
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
