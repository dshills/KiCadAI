package schematiclayout

import "kicadai/internal/kicadfiles"

func Place(request Request) Result {
	request = Classify(request)
	rules := normalizeRules(request.Rules)
	baseX := kicadfiles.MM(30)
	stageSpacing := rules.MinStageSpacing
	if stageSpacing < kicadfiles.MM(25.4) {
		stageSpacing = kicadfiles.MM(25.4)
	}
	laneY := map[Lane]kicadfiles.IU{
		LanePositiveRail: kicadfiles.MM(30),
		LaneSignal:       kicadfiles.MM(65),
		LaneReference:    kicadfiles.MM(90),
		LaneGround:       kicadfiles.MM(115),
		LaneNegativeRail: kicadfiles.MM(140),
	}
	cellCounts := map[Stage]map[Lane]int{}
	result := Result{Components: make([]PlacedComponent, 0, len(request.Components))}
	for _, component := range request.Components {
		placed := PlacedComponent{Component: component}
		if component.Fixed {
			placed.PlacedAt = SnapPoint(component.Position, rules.Grid)
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityInfo, Code: "fixed_component", Ref: component.Ref, Message: "fixed schematic coordinate preserved"})
		} else {
			stage := component.Stage
			if stage == StageUnknown {
				stage = StageProcessing
			}
			lane := component.Lane
			if lane == LaneUnknown {
				lane = LaneSignal
			}
			yBase, ok := laneY[lane]
			if !ok {
				yBase = laneY[LaneSignal]
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "unknown_lane", Ref: component.Ref, Message: "unknown schematic lane defaulted to signal lane"})
			}
			if cellCounts[stage] == nil {
				cellCounts[stage] = map[Lane]int{}
			}
			cellIndex := cellCounts[stage][lane]
			stageIndex := int(stage)
			if stageIndex < 1 {
				stageIndex = int(StageProcessing)
			}
			x := baseX + kicadfiles.IU(stageIndex-1)*stageSpacing
			y := yBase + kicadfiles.IU(cellIndex)*rules.MinGroupGutter
			placed.PlacedAt = SnapPoint(kicadfiles.Point{X: x, Y: y}, rules.Grid)
			cellCounts[stage][lane]++
		}
		placed.ReferenceText = defaultReferenceText(placed)
		placed.ValueText = defaultValueText(placed)
		result.Components = append(result.Components, placed)
	}
	result = Validate(result, request)
	result.Diagnostics = append(result.Diagnostics, placementDiagnostics(result.Components, request.Sheet)...)
	return NormalizeResult(result, rules)
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
