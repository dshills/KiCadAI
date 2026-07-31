package schematiclayout

import (
	"fmt"
	"strings"

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
	anchorIndex := newPinAnchorIndex(pinAnchors(result.Components))
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
			if other.NetName == "" || other.NetName == wire.NetName || !wireSegmentsElectricallyContact(wire, other) {
				continue
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityError,
				Code:     "wire_crossing",
				NetName:  wire.NetName,
				Message:  fmt.Sprintf("wire contacts unrelated net %s", other.NetName),
			})
		}
		if pin, overlaps := unrelatedPinForWireIndexed(wire, wire.NetName, anchorIndex, request); overlaps {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityError,
				Code:     DiagnosticWirePinOverlap,
				Ref:      pin.Ref,
				NetName:  wire.NetName,
				Message:  fmt.Sprintf("wire passes through unrelated pin %s", pin.Ref+"."+pin.Pin),
				Repair:   "reroute the net around the pin or use a label connection",
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
	result.Diagnostics = append(result.Diagnostics, relativePlacementDiagnostics(result.Components, rules)...)
	symbolBodies := symbolValidationBodies(objects)
	textObjects := nonSymbolValidationObjects(objects)
	for _, wire := range result.Wires {
		for _, object := range symbolBodies {
			if SegmentIntersectsRect(wire, object.Box) && !wireLeavesAttachedSymbol(wire, object, result.Components) {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{
					Severity: SeverityError,
					Code:     "wire_symbol_overlap",
					Ref:      object.Ref,
					NetName:  wire.NetName,
					Message:  "wire crosses a symbol body",
				})
			}
		}
		for _, object := range textObjects {
			if object.Kind == "label" || !SegmentIntersectsRect(wire, object.Box) {
				continue
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityWarning,
				Code:     "text_wire_overlap",
				Ref:      object.Ref,
				NetName:  wire.NetName,
				Message:  fmt.Sprintf("wire crosses %s %q", object.Kind, object.Ref),
			})
		}
	}
	return NormalizeResult(result, rules)
}

func relativePlacementDiagnostics(components []PlacedComponent, rules Rules) []Diagnostic {
	byRef := make(map[string]PlacedComponent, len(components))
	for _, component := range components {
		byRef[component.Ref] = component
	}
	var diagnostics []Diagnostic
	for _, component := range components {
		bounds := componentBody(component)
		for _, targetRef := range component.RightOf {
			target, ok := byRef[targetRef]
			if !ok {
				continue
			}
			if bounds.MinX < componentBody(target).MaxX+rules.MinComponentSpacing {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "right_of_violation", Ref: component.Ref, Message: fmt.Sprintf("component must be right of %s", targetRef), Repair: "increase horizontal stage spacing or correct the right_of relation"})
			}
		}
		for _, targetRef := range component.Above {
			target, ok := byRef[targetRef]
			if !ok {
				continue
			}
			if bounds.MaxY > componentBody(target).MinY-rules.MinComponentSpacing {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "above_violation", Ref: component.Ref, Message: fmt.Sprintf("component must be above %s", targetRef), Repair: "increase vertical lane spacing or correct the above relation"})
			}
		}
		for _, targetRef := range component.SameRowAs {
			target, ok := byRef[targetRef]
			if !ok {
				continue
			}
			if component.PlacedAt.Y != target.PlacedAt.Y {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "same_row_violation", Ref: component.Ref, Message: fmt.Sprintf("component must share a row with %s", targetRef), Repair: "align the component anchors on a common row or correct the same_row_as relation"})
			}
		}
		for _, targetRef := range component.SameColumnAs {
			target, ok := byRef[targetRef]
			if !ok {
				continue
			}
			if component.PlacedAt.X != target.PlacedAt.X {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "same_column_violation", Ref: component.Ref, Message: fmt.Sprintf("component must share a column with %s", targetRef), Repair: "align the component anchors on a common column or correct the same_column_as relation"})
			}
		}
		for _, endpoint := range component.SameRowAsPin {
			anchor, ok := placedEndpointAnchor(byRef, endpoint)
			if !ok {
				continue
			}
			if component.PlacedAt.Y != anchor.Y {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "same_pin_row_violation", Ref: component.Ref, Message: fmt.Sprintf("component must share a row with %s.%s", endpoint.Ref, endpoint.Pin), Repair: "align the component anchor with the named pin row or correct the same_row_as_pin relation"})
			}
		}
		for _, endpoint := range component.SameColumnAsPin {
			anchor, ok := placedEndpointAnchor(byRef, endpoint)
			if !ok {
				continue
			}
			if component.PlacedAt.X != anchor.X {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "same_pin_column_violation", Ref: component.Ref, Message: fmt.Sprintf("component must share a column with %s.%s", endpoint.Ref, endpoint.Pin), Repair: "align the component anchor with the named pin column or correct the same_column_as_pin relation"})
			}
		}
		if len(component.CenterBetween) == 2 {
			left, leftOK := byRef[component.CenterBetween[0]]
			right, rightOK := byRef[component.CenterBetween[1]]
			if leftOK && rightOK {
				centerX := SnapIU(left.PlacedAt.X+(right.PlacedAt.X-left.PlacedAt.X)/2, rules.Grid)
				if component.PlacedAt.X != centerX {
					diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "center_between_violation", Ref: component.Ref, Message: fmt.Sprintf("component must be centered between %s and %s", component.CenterBetween[0], component.CenterBetween[1]), Repair: "center the component between the two named columns or correct the center_between relation"})
				}
			}
		}
	}
	return diagnostics
}

func placedEndpointAnchor(components map[string]PlacedComponent, endpoint Endpoint) (kicadfiles.Point, bool) {
	component, ok := components[endpoint.Ref]
	if !ok {
		return kicadfiles.Point{}, false
	}
	anchor, ok := pinAnchors([]PlacedComponent{component})[endpoint]
	return anchor, ok
}

// wireLeavesAttachedSymbol permits the short wire stub that exits a symbol
// pin. The validator must still reject a wire that enters or crosses a body;
// only an axis-aligned segment that starts at a known pin and immediately
// continues away from that body's bounds is exempt.
func wireLeavesAttachedSymbol(wire WireSegment, object ValidationObject, components []PlacedComponent) bool {
	var component *PlacedComponent
	for index := range components {
		if components[index].Ref == object.Ref {
			component = &components[index]
			break
		}
	}
	if component == nil {
		return false
	}
	anchors := pinAnchors([]PlacedComponent{*component})
	for _, endpoint := range []kicadfiles.Point{wire.From, wire.To} {
		attached := false
		for _, anchor := range anchors {
			if anchor == endpoint {
				attached = true
				break
			}
		}
		if !attached {
			continue
		}
		// KiCad power glyphs are single-pin terminals. Some library bodies place
		// the visible rail or ground graphic on the wire-facing side of the pin
		// anchor, so a legal connection can geometrically overlap that glyph.
		// Requiring the exact parsed pin anchor keeps unrelated pass-through wires
		// subject to the normal body-crossing rule.
		if isPowerTerminalComponent(component) {
			return true
		}
		other := wire.To
		if endpoint == wire.To {
			other = wire.From
		}
		if object.Box.ContainsPoint(endpoint) && !object.Box.ContainsPoint(other) {
			return true
		}
		if wire.From.X == wire.To.X && (endpoint.Y <= object.Box.MinY && other.Y <= object.Box.MinY || endpoint.Y >= object.Box.MaxY && other.Y >= object.Box.MaxY) {
			return true
		}
		if wire.From.Y == wire.To.Y && (endpoint.X <= object.Box.MinX && other.X <= object.Box.MinX || endpoint.X >= object.Box.MaxX && other.X >= object.Box.MaxX) {
			return true
		}
	}
	return false
}

func isPowerTerminalComponent(component *PlacedComponent) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(component.LibraryID)), "power:") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(component.Role)) {
	case "power_symbol", "ground_symbol", "positive_rail", "negative_rail", "ground":
		return true
	default:
		return false
	}
}

func nonSymbolValidationObjects(objects []ValidationObject) []ValidationObject {
	var out []ValidationObject
	for _, object := range objects {
		if object.Kind != "symbol" {
			out = append(out, object)
		}
	}
	return out
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
		objects = append(objects, ValidationObject{Ref: label.Text, Kind: "label", Box: TextEstimateRotated(label.Text, label.Position, label.Rotation)})
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
