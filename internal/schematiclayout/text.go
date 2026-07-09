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
		if component.ReferenceText.Box.Empty() {
			field, clean := chooseTextPosition(component.Ref, component.PlacedAt, bodyByRef[component.Ref], occupied, rules, true)
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
	for _, anchor := range candidates {
		box := TextEstimate(text, anchor, 0, 0)
		if !rectIntersectsAny(box, occupied) {
			return localTextBox(text, origin, anchor, box), true
		}
	}
	anchor := candidates[0]
	box := TextEstimate(text, anchor, 0, 0)
	return localTextBox(text, origin, anchor, box), false
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
