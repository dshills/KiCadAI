package schematiclayout

import (
	"sort"

	"kicadai/internal/kicadfiles"
)

func Place(request Request) Result {
	request = Classify(request)
	rules := normalizeRules(request.Rules)
	cells, islandCount, rankCount := planPlacement(request)
	rankX := placementRankX(request.Components, cells, rules)
	positions := placementPositions(request.Components, cells, rankX, rules)
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
	result.Report.IslandCount = islandCount
	result.Report.RankCount = rankCount
	result.Report.OccupiedBounds = placementDrawingBounds(result.Components)
	result = Validate(result, request)
	result.Diagnostics = append(result.Diagnostics, placementDiagnostics(result.Components, request.Sheet)...)
	return NormalizeResult(result, rules)
}

func placementRankX(components []Component, cells map[string]placementCell, rules Rules) map[int]kicadfiles.IU {
	maxHalfWidth := map[int]kicadfiles.IU{}
	var ranks []int
	seen := map[int]struct{}{}
	for _, component := range components {
		cell := cells[component.Ref]
		body := RotateRect(DefaultBodyFor(PlacedComponent{Component: component}), component.Rotation)
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
		rowByRank := map[int]int{}
		rowHeight := map[int]kicadfiles.IU{}
		for _, component := range items {
			rank := cells[component.Ref].rank
			row := rowByRank[rank]
			rowByRank[rank]++
			body := RotateRect(DefaultBodyFor(PlacedComponent{Component: component}), component.Rotation)
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
		if len(items) != 0 {
			y += rules.MinGroupGutter
		}
	}
	return positions
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
	return TextBox{
		Text: component.Ref,
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
		if !usable.ContainsRect(componentBody(component)) {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "page_overflow", Ref: component.Ref, Message: "placed component is outside the preferred readable area"})
		}
	}
	return diagnostics
}
