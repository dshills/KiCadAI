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

func TestValidateRejectsDifferentNetEndpointContactAndCollinearOverlap(t *testing.T) {
	tests := []struct {
		name  string
		wires []WireSegment
	}{
		{
			name: "shared endpoint",
			wires: []WireSegment{
				{NetName: "A", From: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}, To: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)}},
				{NetName: "B", From: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)}, To: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(40)}},
			},
		},
		{
			name: "collinear overlap",
			wires: []WireSegment{
				{NetName: "A", From: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}, To: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}},
				{NetName: "B", From: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)}, To: kicadfiles.Point{X: kicadfiles.MM(60), Y: kicadfiles.MM(20)}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validated := Validate(Result{Wires: test.wires}, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
			if !hasDiagnostic(validated.Diagnostics, "wire_crossing", SeverityError) {
				t.Fatalf("diagnostics = %#v, want electrical-contact error", validated.Diagnostics)
			}
		})
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

func TestValidateAcceptsWireEndingAtFallbackPinAnchor(t *testing.T) {
	anchor := kicadfiles.Point{X: kicadfiles.MM(50.8), Y: kicadfiles.MM(50.8)}
	result := Result{
		Components: []PlacedComponent{{
			Component: Component{
				Ref:       "#PWR01",
				LibraryID: "power:GND",
				Body:      Rect{MinX: -kicadfiles.MM(2), MinY: 0, MaxX: kicadfiles.MM(2), MaxY: kicadfiles.MM(3)},
				BodyKnown: true,
			},
			PlacedAt: anchor,
		}},
		Wires: []WireSegment{{
			NetName: "GND",
			From:    kicadfiles.Point{X: anchor.X, Y: kicadfiles.MM(40.64)},
			To:      anchor,
		}},
	}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
	if hasDiagnostic(validated.Diagnostics, "wire_symbol_overlap", SeverityError) {
		t.Fatalf("diagnostics = %#v, fallback pin anchor should accept the attached wire", validated.Diagnostics)
	}
}

func TestValidateAcceptsWireEndingAtPowerGlyphPin(t *testing.T) {
	anchor := kicadfiles.Point{X: kicadfiles.MM(50.8), Y: kicadfiles.MM(50.8)}
	result := Result{
		Components: []PlacedComponent{{
			Component: Component{
				Ref:       "#PWR01",
				LibraryID: "power:GND",
				Body:      Rect{MinX: -kicadfiles.MM(2), MinY: -kicadfiles.MM(3), MaxX: kicadfiles.MM(2), MaxY: 0},
				BodyKnown: true,
				Pins:      []Pin{{Number: "1"}},
			},
			PlacedAt: anchor,
		}},
		Wires: []WireSegment{{
			NetName: "GND",
			From:    kicadfiles.Point{X: anchor.X, Y: kicadfiles.MM(40.64)},
			To:      anchor,
		}},
	}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
	if hasDiagnostic(validated.Diagnostics, "wire_symbol_overlap", SeverityError) {
		t.Fatalf("diagnostics = %#v, power glyph pin should accept its attached wire", validated.Diagnostics)
	}
}

func TestValidateAcceptsWireEndingAtRoleIdentifiedCustomPowerGlyph(t *testing.T) {
	anchor := kicadfiles.Point{X: kicadfiles.MM(50.8), Y: kicadfiles.MM(50.8)}
	result := Result{
		Components: []PlacedComponent{{
			Component: Component{
				Ref:       "#PWR01",
				LibraryID: "Custom:GND",
				Role:      "ground_symbol",
				Body:      Rect{MinX: -kicadfiles.MM(2), MinY: -kicadfiles.MM(3), MaxX: kicadfiles.MM(2), MaxY: 0},
				BodyKnown: true,
				Pins:      []Pin{{Number: "1"}},
			},
			PlacedAt: anchor,
		}},
		Wires: []WireSegment{{
			NetName: "GND",
			From:    kicadfiles.Point{X: anchor.X, Y: kicadfiles.MM(40.64)},
			To:      anchor,
		}},
	}
	validated := Validate(result, Request{Sheet: testSheet(), Rules: DefaultRules(ProfileBasic)})
	if hasDiagnostic(validated.Diagnostics, "wire_symbol_overlap", SeverityError) {
		t.Fatalf("diagnostics = %#v, role-identified custom power glyph should accept its attached wire", validated.Diagnostics)
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
	placed, diagnostics := reflowTextForWires([]PlacedComponent{component}, wires, nil, DefaultRules(ProfileStandard), UsableSheet(testSheet()))
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

func TestReflowTextForWiresKeepsGeneratedFieldsInsideUsableSheet(t *testing.T) {
	component := PlacedComponent{
		Component: Component{Ref: "Q1", Value: "LONG_COMPONENT_VALUE", Role: "transistor"},
		PlacedAt:  kicadfiles.Point{X: kicadfiles.MM(282), Y: kicadfiles.MM(100)},
	}
	placed, _ := reflowTextForWires(
		[]PlacedComponent{component},
		nil,
		nil,
		DefaultRules(ProfileStandard),
		UsableSheet(testSheet()),
	)
	if len(placed) != 1 {
		t.Fatalf("placed = %#v", placed)
	}
	usable := UsableSheet(testSheet())
	for name, field := range map[string]TextBox{
		"reference": placed[0].ReferenceText,
		"value":     placed[0].ValueText,
	} {
		if !usable.ContainsRect(field.Box.Translate(placed[0].PlacedAt)) {
			t.Fatalf("%s field is outside usable sheet: %#v usable=%#v", name, field, usable)
		}
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
