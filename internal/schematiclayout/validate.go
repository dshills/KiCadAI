package schematiclayout

import (
	"fmt"

	"kicadai/internal/kicadfiles"
)

type ValidationObject struct {
	Ref  string
	Kind string
	Box  Rect
}

func Validate(result Result, request Request) Result {
	rules := normalizeRules(request.Rules)
	result.Diagnostics = append([]Diagnostic(nil), result.Diagnostics...)
	usable := UsableSheet(request.Sheet)
	objects := validationObjects(result)
	for _, wire := range result.Wires {
		if wire.From.X != wire.To.X && wire.From.Y != wire.To.Y {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityError,
				Code:     "diagonal_wire",
				NetName:  wire.NetName,
				Message:  "schematic wire is not horizontal or vertical",
			})
		}
		if !usable.ContainsPoint(wire.From) || !usable.ContainsPoint(wire.To) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityError,
				Code:     "outside_sheet",
				NetName:  wire.NetName,
				Message:  "wire endpoint is outside the usable sheet area",
			})
		}
	}
	for index, wire := range result.Wires {
		if wire.NetName == "" {
			continue
		}
		for otherIndex := index + 1; otherIndex < len(result.Wires); otherIndex++ {
			other := result.Wires[otherIndex]
			if other.NetName == "" || other.NetName == wire.NetName || !wireSegmentsCross(wire, other) {
				continue
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityWarning,
				Code:     "wire_crossing",
				NetName:  wire.NetName,
				Message:  fmt.Sprintf("wire crosses unrelated net %s", other.NetName),
			})
		}
	}
	for index, object := range objects {
		if !usable.ContainsRect(object.Box) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityError,
				Code:     "outside_sheet",
				Ref:      object.Ref,
				Message:  fmt.Sprintf("%s is outside the usable sheet area", object.Kind),
			})
		}
		for otherIndex := index + 1; otherIndex < len(objects); otherIndex++ {
			other := objects[otherIndex]
			if !object.Box.Intersects(other.Box) {
				continue
			}
			result.Diagnostics = append(result.Diagnostics, overlapDiagnostic(object, other))
		}
	}
	symbolBodies := symbolValidationBodies(objects)
	for _, wire := range result.Wires {
		for _, object := range symbolBodies {
			if SegmentIntersectsRect(wire, object.Box) {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{
					Severity: SeverityError,
					Code:     "wire_symbol_overlap",
					Ref:      object.Ref,
					NetName:  wire.NetName,
					Message:  "wire crosses a symbol body",
				})
			}
		}
	}
	return NormalizeResult(result, rules)
}

func symbolValidationBodies(objects []ValidationObject) []ValidationObject {
	symbols := make([]ValidationObject, 0, len(objects))
	for _, object := range objects {
		if object.Kind != "symbol" {
			continue
		}
		object.Box = shrinkRect(object.Box, kicadfiles.MM(0.5))
		symbols = append(symbols, object)
	}
	return symbols
}

func validationObjects(result Result) []ValidationObject {
	var objects []ValidationObject
	for _, component := range result.Components {
		body := componentBody(component)
		objects = append(objects, ValidationObject{Ref: component.Ref, Kind: "symbol", Box: body})
		if !component.ReferenceText.Box.Empty() {
			objects = append(objects, ValidationObject{Ref: component.Ref, Kind: "reference_text", Box: component.ReferenceText.Box.Translate(component.PlacedAt)})
		}
		if !component.ValueText.Box.Empty() {
			objects = append(objects, ValidationObject{Ref: component.Ref, Kind: "value_text", Box: component.ValueText.Box.Translate(component.PlacedAt)})
		}
	}
	for _, label := range result.Labels {
		objects = append(objects, ValidationObject{Ref: label.Text, Kind: "label", Box: TextEstimate(label.Text, label.Position, 0, 0)})
	}
	return objects
}

func overlapDiagnostic(first, second ValidationObject) Diagnostic {
	code := "symbol_overlap"
	severity := SeverityError
	switch {
	case first.Kind == "label" && second.Kind == "label":
		code = "label_overlap"
		severity = SeverityWarning
	case first.Kind != "symbol" || second.Kind != "symbol":
		code = "text_symbol_overlap"
		severity = SeverityWarning
	}
	return Diagnostic{
		Severity: severity,
		Code:     code,
		Ref:      first.Ref,
		Message:  fmt.Sprintf("%s %q overlaps %s %q", first.Kind, first.Ref, second.Kind, second.Ref),
	}
}

func shrinkRect(rect Rect, amount kicadfiles.IU) Rect {
	if amount <= 0 || rect.Width() <= amount*2 || rect.Height() <= amount*2 {
		return rect
	}
	return rect.Inflate(-amount)
}
