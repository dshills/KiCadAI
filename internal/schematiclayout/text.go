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
func reflowTextForWires(components []PlacedComponent, wires []WireSegment, labels []Label, rules Rules, usable Rect) ([]PlacedComponent, []Diagnostic) {
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
		occupied = append(occupied, TextEstimateOriented(label.Text, label.Position, label.Rotation, label.JustifyRight))
	}
	for index := range placed {
		component := &placed[index]
		body := bodyByRef[component.Ref]
		referenceText := component.DisplayRef
		if referenceText == "" {
			referenceText = component.Ref
		}
		var clean bool
		component.ReferenceText, clean = chooseTextPositionWithin(referenceText, component.PlacedAt, body, occupied, rules, true, usable)
		if !clean {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "text_placement_fallback", Ref: component.Ref, Message: "reference field required crowded fallback placement"})
		}
		occupied = append(occupied, component.ReferenceText.Box.Translate(component.PlacedAt))
		if component.Value != "" {
			component.ValueText, clean = chooseTextPositionWithin(component.Value, component.PlacedAt, body, occupied, rules, false, usable)
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
	return chooseTextPositionWithin(text, origin, body, occupied, rules, preferAbove, Rect{})
}

func chooseTextPositionWithin(text string, origin kicadfiles.Point, body Rect, occupied []Rect, rules Rules, preferAbove bool, usable Rect) (TextBox, bool) {
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
	// Fully connected symbols often have wires on every cardinal side. Keep the
	// existing compact cardinal choices preferred, then try deterministic corner
	// positions before accepting an overlap. This is especially useful for
	// active devices whose base/gate, collector/drain, and emitter/source are all
	// locally wired.
	for multiplier := kicadfiles.IU(1); multiplier <= 3; multiplier++ {
		cornerGap := gap * multiplier
		topLeft := kicadfiles.Point{X: body.MinX - cornerGap - width, Y: body.MinY - cornerGap}
		topRight := kicadfiles.Point{X: body.MaxX + cornerGap, Y: body.MinY - cornerGap}
		bottomLeft := kicadfiles.Point{X: body.MinX - cornerGap - width, Y: body.MaxY + cornerGap + height}
		bottomRight := kicadfiles.Point{X: body.MaxX + cornerGap, Y: body.MaxY + cornerGap + height}
		if preferAbove {
			candidates = append(candidates, topLeft, topRight, bottomLeft, bottomRight)
		} else {
			candidates = append(candidates, bottomRight, bottomLeft, topRight, topLeft)
		}
	}
	for _, anchor := range candidates {
		box := TextEstimate(text, anchor, 0, 0)
		if (usable.Empty() || usable.ContainsRect(box)) && !rectIntersectsAny(box, occupied) {
			return localTextBox(text, origin, anchor, box), true
		}
	}
	eligible := make([]kicadfiles.Point, 0, len(candidates))
	for _, anchor := range candidates {
		box := TextEstimate(text, anchor, 0, 0)
		if usable.Empty() || usable.ContainsRect(box) {
			eligible = append(eligible, anchor)
		}
	}
	if len(eligible) == 0 && !usable.Empty() {
		for _, anchor := range candidates {
			fitted := fitTextAnchorToRect(text, anchor, usable)
			if usable.ContainsRect(TextEstimate(text, fitted, 0, 0)) {
				eligible = append(eligible, fitted)
			}
		}
	}
	if len(eligible) == 0 {
		eligible = candidates
	}
	bestIndex := 0
	bestScore := textOverlapScore(TextEstimate(text, eligible[0], 0, 0), occupied)
	for index := 1; index < len(eligible); index++ {
		score := textOverlapScore(TextEstimate(text, eligible[index], 0, 0), occupied)
		if score < bestScore {
			bestIndex = index
			bestScore = score
		}
	}
	anchor := eligible[bestIndex]
	box := TextEstimate(text, anchor, 0, 0)
	return localTextBox(text, origin, anchor, box), false
}

func fitTextAnchorToRect(text string, anchor kicadfiles.Point, usable Rect) kicadfiles.Point {
	box := TextEstimate(text, anchor, 0, 0)
	if box.Width() > usable.Width() || box.Height() > usable.Height() {
		return anchor
	}
	if box.MinX < usable.MinX {
		anchor.X += usable.MinX - box.MinX
	} else if box.MaxX > usable.MaxX {
		anchor.X -= box.MaxX - usable.MaxX
	}
	if box.MinY < usable.MinY {
		anchor.Y += usable.MinY - box.MinY
	} else if box.MaxY > usable.MaxY {
		anchor.Y -= box.MaxY - usable.MaxY
	}
	return anchor
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
