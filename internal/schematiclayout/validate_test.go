package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestValidateRejectsSymbolOverlap(t *testing.T) {
	result := Result{Components: []PlacedComponent{
		{Component: Component{Ref: "R1", Role: "resistor"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}},
		{Component: Component{Ref: "R2", Role: "resistor"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(52), Y: kicadfiles.MM(50)}},
	}}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
	if !hasDiagnostic(validated.Diagnostics, "symbol_overlap", SeverityError) {
		t.Fatalf("diagnostics = %#v, want symbol overlap error", validated.Diagnostics)
	}
}

func TestValidateRejectsDiagonalWire(t *testing.T) {
	result := Result{Wires: []WireSegment{{
		NetName: "SIG",
		From:    kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
		To:      kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(30)},
	}}}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
	if !hasDiagnostic(validated.Diagnostics, "diagonal_wire", SeverityError) {
		t.Fatalf("diagnostics = %#v, want diagonal wire error", validated.Diagnostics)
	}
}

func TestValidateRejectsWireThroughSymbol(t *testing.T) {
	result := Result{
		Components: []PlacedComponent{{Component: Component{Ref: "U1", Role: "opamp"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}}},
		Wires: []WireSegment{{
			NetName: "SIG",
			From:    kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(50)},
			To:      kicadfiles.Point{X: kicadfiles.MM(70), Y: kicadfiles.MM(50)},
		}},
	}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
	if !hasDiagnostic(validated.Diagnostics, "wire_symbol_overlap", SeverityError) {
		t.Fatalf("diagnostics = %#v, want wire/symbol overlap error", validated.Diagnostics)
	}
}

func TestValidateWarnsForTextOverlap(t *testing.T) {
	result := Result{Components: []PlacedComponent{{
		Component: Component{
			Ref:           "R1",
			Role:          "resistor",
			ReferenceText: TextBox{Text: "R1", Box: Rect{MinX: -kicadfiles.MM(2), MinY: -kicadfiles.MM(2), MaxX: kicadfiles.MM(2), MaxY: kicadfiles.MM(2)}},
		},
		PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)},
	}}}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
	if !hasDiagnostic(validated.Diagnostics, "text_symbol_overlap", SeverityWarning) {
		t.Fatalf("diagnostics = %#v, want text overlap warning", validated.Diagnostics)
	}
}

func TestValidateAcceptsSpacedObjects(t *testing.T) {
	result := Result{Components: []PlacedComponent{
		{Component: Component{Ref: "R1", Role: "resistor"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(40)}},
		{Component: Component{Ref: "R2", Role: "resistor"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(40)}},
	}}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileStrict)})
	if !validated.Report.Passed {
		t.Fatalf("report = %#v diagnostics=%#v, want pass", validated.Report, validated.Diagnostics)
	}
}

func TestReflowTextForWiresAvoidsWireOverlap(t *testing.T) {
	component := PlacedComponent{Component: Component{Ref: "F1", Value: "500mA", Role: "fuse"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}}
	wires := []WireSegment{{From: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(55)}, To: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(70)}}}
	placed, diagnostics := reflowTextForWires([]PlacedComponent{component}, wires, nil, DefaultRules(ProfileStandard))
	if len(placed) != 1 {
		t.Fatalf("placed = %#v", placed)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if placed[0].ValueText.Box.Translate(placed[0].PlacedAt).Intersects(Rect{MinX: kicadfiles.MM(49), MinY: kicadfiles.MM(55), MaxX: kicadfiles.MM(51), MaxY: kicadfiles.MM(70)}) {
		t.Fatalf("value text overlaps route: %#v", placed[0].ValueText)
	}
}

func testSheet() Sheet {
	return Sheet{Width: kicadfiles.MM(297), Height: kicadfiles.MM(210), Margin: kicadfiles.MM(10)}
}

func hasDiagnostic(diagnostics []Diagnostic, code string, severity Severity) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && diagnostic.Severity == severity {
			return true
		}
	}
	return false
}
