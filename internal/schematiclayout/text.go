package schematiclayout

import "kicadai/internal/kicadfiles"

func placeComponentText(components []PlacedComponent, rules Rules) ([]PlacedComponent, []Diagnostic) {
	placed := append([]PlacedComponent(nil), components...)
	bodyByRef := map[string]Rect{}
	for _, component := range placed {
		bodyByRef[component.Ref] = componentBody(component)
	}
	var occupied []Rect
	for _, component := range placed {
		occupied = append(occupied, bodyByRef[component.Ref])
	}
	var diagnostics []Diagnostic
	for index := range placed {
		component := &placed[index]
		referenceText := component.DisplayRef
		if referenceText == "" {
			referenceText = component.Ref
		}
		if component.ReferenceText.Box.Empty() {
			field, clean := chooseTextPosition(referenceText, component.PlacedAt, bodyByRef[component.Ref], occupied, rules, true)
			component.ReferenceText = field
			occupied = append(occupied, field.Box.Translate(component.PlacedAt))
			if !clean {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "text_placement_fallback", Ref: component.Ref, Message: "reference field required crowded fallback placement"})
			}
		}
		if component.Value != "" && component.ValueText.Box.Empty() {
			field, clean := chooseTextPosition(component.Value, component.PlacedAt, bodyByRef[component.Ref], occupied, rules, false)
			component.ValueText = field
			occupied = append(occupied, field.Box.Translate(component.PlacedAt))
			if !clean {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "text_placement_fallback", Ref: component.Ref, Message: "value field required crowded fallback placement"})
			}
		}
	}
	return placed, diagnostics
}

// reflowTextForWires recomputes generated symbol fields after routing. Text
// placement happens before routes exist, so a field that was clear of symbols
// can otherwise end up on a power or ground wire.
func reflowTextForWires(components []PlacedComponent, wires []WireSegment, labels []Label, rules Rules) ([]PlacedComponent, []Diagnostic) {
	placed := append([]PlacedComponent(nil), components...)
	var diagnostics []Diagnostic
	bodyByRef := map[string]Rect{}
	occupied := make([]Rect, 0, len(placed)+len(wires))
	for _, component := range placed {
		body := componentBody(component)
		bodyByRef[component.Ref] = body
		occupied = append(occupied, body)
	}
	wireGap := rules.MinTextSpacing / 2
	if wireGap <= 0 {
		wireGap = kicadfiles.MM(1.27)
	}
	for _, wire := range wires {
		minX, maxX := orderedIU(wire.From.X, wire.To.X)
		minY, maxY := orderedIU(wire.From.Y, wire.To.Y)
		occupied = append(occupied, (Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}).Inflate(wireGap))
	}
	for _, label := range labels {
		occupied = append(occupied, TextEstimateRotated(label.Text, label.Position, label.Rotation))
	}
	for index := range placed {
		component := &placed[index]
		body := bodyByRef[component.Ref]
		referenceText := component.DisplayRef
		if referenceText == "" {
			referenceText = component.Ref
		}
		var clean bool
		component.ReferenceText, clean = chooseTextPosition(referenceText, component.PlacedAt, body, occupied, rules, true)
		if !clean {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "text_placement_fallback", Ref: component.Ref, Message: "reference field required crowded fallback placement"})
		}
		occupied = append(occupied, component.ReferenceText.Box.Translate(component.PlacedAt))
		if component.Value != "" {
			component.ValueText, clean = chooseTextPosition(component.Value, component.PlacedAt, body, occupied, rules, false)
			if !clean {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "text_placement_fallback", Ref: component.Ref, Message: "value field required crowded fallback placement"})
			}
			occupied = append(occupied, component.ValueText.Box.Translate(component.PlacedAt))
		} else {
			component.ValueText = TextBox{}
		}
	}
	return placed, diagnostics
}

func chooseTextPosition(text string, origin kicadfiles.Point, body Rect, occupied []Rect, rules Rules, preferAbove bool) (TextBox, bool) {
	gap := rules.MinTextSpacing
	if gap <= 0 {
		gap = kicadfiles.MM(2.54)
	}
	estimate := TextEstimate(text, kicadfiles.Point{}, 0, 0)
	width := estimate.Width()
	height := estimate.Height()
	centerX := (body.MinX + body.MaxX) / 2
	centerY := (body.MinY + body.MaxY) / 2
	top := kicadfiles.Point{X: centerX - width/2, Y: body.MinY - gap}
	bottom := kicadfiles.Point{X: centerX - width/2, Y: body.MaxY + gap + height}
	left := kicadfiles.Point{X: body.MinX - gap - width, Y: centerY + height/2}
	right := kicadfiles.Point{X: body.MaxX + gap, Y: centerY + height/2}
	candidates := []kicadfiles.Point{top, bottom, left, right}
	if !preferAbove {
		candidates = []kicadfiles.Point{bottom, top, right, left}
	}
	for multiplier := kicadfiles.IU(2); multiplier <= 3; multiplier++ {
		wideGap := gap * multiplier
		wideTop := kicadfiles.Point{X: centerX - width/2, Y: body.MinY - wideGap - height}
		wideBottom := kicadfiles.Point{X: centerX - width/2, Y: body.MaxY + wideGap}
		wideLeft := kicadfiles.Point{X: body.MinX - wideGap - width, Y: centerY + height/2}
		wideRight := kicadfiles.Point{X: body.MaxX + wideGap, Y: centerY + height/2}
		if preferAbove {
			candidates = append(candidates, wideTop, wideBottom, wideLeft, wideRight)
		} else {
			candidates = append(candidates, wideBottom, wideTop, wideRight, wideLeft)
		}
	}
	for _, anchor := range candidates {
		box := TextEstimate(text, anchor, 0, 0)
		if !rectIntersectsAny(box, occupied) {
			return localTextBox(text, origin, anchor, box), true
		}
	}
	bestIndex := 0
	bestScore := textOverlapScore(TextEstimate(text, candidates[0], 0, 0), occupied)
	for index := 1; index < len(candidates); index++ {
		score := textOverlapScore(TextEstimate(text, candidates[index], 0, 0), occupied)
		if score < bestScore {
			bestIndex = index
			bestScore = score
		}
	}
	anchor := candidates[bestIndex]
	box := TextEstimate(text, anchor, 0, 0)
	return localTextBox(text, origin, anchor, box), false
}

func textOverlapScore(candidate Rect, occupied []Rect) int {
	score := 0
	for _, object := range occupied {
		if candidate.Intersects(object) {
			score++
		}
	}
	return score
}

func localTextBox(text string, origin, anchor kicadfiles.Point, box Rect) TextBox {
	return TextBox{
		Text: text,
		At:   kicadfiles.Point{X: anchor.X - origin.X, Y: anchor.Y - origin.Y},
		Box:  Rect{MinX: box.MinX - origin.X, MinY: box.MinY - origin.Y, MaxX: box.MaxX - origin.X, MaxY: box.MaxY - origin.Y},
	}
}

func rectIntersectsAny(candidate Rect, occupied []Rect) bool {
	for _, object := range occupied {
		if candidate.Intersects(object) {
			return true
		}
	}
	return false
}
