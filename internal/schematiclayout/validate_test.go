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
