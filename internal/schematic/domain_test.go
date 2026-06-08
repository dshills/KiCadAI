package schematic

import (
	"errors"
	"math"
	"testing"

	"kicadai/internal/kiapi"
)

func TestValidateAddSymbol(t *testing.T) {
	request := AddSymbolRequest{
		Document:  testDocument(),
		LibraryID: "Device:R",
		Reference: "R?",
		Value:     "1k",
		Position:  Point{X: 1000, Y: 2000},
	}

	operation, err := PlanAddSymbol(request)
	if err != nil {
		t.Fatalf("PlanAddSymbol returned error: %v", err)
	}
	if operation.Kind != OpKindAddSymbol {
		t.Fatalf("operation kind = %q", operation.Kind)
	}

	request.LibraryID = ""
	if _, err := PlanAddSymbol(request); !errors.Is(err, ErrMissingLibrary) {
		t.Fatalf("PlanAddSymbol error = %v, want %v", err, ErrMissingLibrary)
	}

	request.LibraryID = "Device:R"
	request.Reference = ""
	if _, err := PlanAddSymbol(request); !errors.Is(err, ErrMissingReference) {
		t.Fatalf("PlanAddSymbol error = %v, want %v", err, ErrMissingReference)
	}

	request.Reference = "R?"
	request.RotationDegrees = math.Inf(1)
	if _, err := PlanAddSymbol(request); !errors.Is(err, ErrInvalidRotation) {
		t.Fatalf("PlanAddSymbol error = %v, want %v", err, ErrInvalidRotation)
	}
}

func TestValidateAddWire(t *testing.T) {
	request := AddWireRequest{
		Document: testDocument(),
		Points:   []Point{{X: 0, Y: 0}, {X: 100, Y: 0}},
	}
	if _, err := PlanAddWire(request); err != nil {
		t.Fatalf("PlanAddWire returned error: %v", err)
	}

	request.Points = request.Points[:1]
	if _, err := PlanAddWire(request); !errors.Is(err, ErrInvalidWire) {
		t.Fatalf("PlanAddWire error = %v, want %v", err, ErrInvalidWire)
	}

	request.Points = []Point{{X: 0, Y: 0}, {X: 0, Y: 0}}
	if _, err := PlanAddWire(request); !errors.Is(err, ErrZeroLengthWire) {
		t.Fatalf("PlanAddWire error = %v, want %v", err, ErrZeroLengthWire)
	}
}

func TestValidateAddLabel(t *testing.T) {
	request := AddLabelRequest{
		Document:  testDocument(),
		Text:      "LED_OUT",
		LabelType: LabelTypeLocal,
		Position:  Point{X: 0, Y: 0},
	}
	if _, err := PlanAddLabel(request); err != nil {
		t.Fatalf("PlanAddLabel returned error: %v", err)
	}

	request.Text = " "
	if _, err := PlanAddLabel(request); !errors.Is(err, ErrMissingText) {
		t.Fatalf("PlanAddLabel error = %v, want %v", err, ErrMissingText)
	}

	request.Text = "LED_OUT"
	request.LabelType = ""
	if _, err := PlanAddLabel(request); !errors.Is(err, ErrMissingLabelType) {
		t.Fatalf("PlanAddLabel error = %v, want %v", err, ErrMissingLabelType)
	}

	request.LabelType = LabelType("bad")
	if _, err := PlanAddLabel(request); !errors.Is(err, ErrInvalidLabelType) {
		t.Fatalf("PlanAddLabel error = %v, want %v", err, ErrInvalidLabelType)
	}
}

func TestValidateDocument(t *testing.T) {
	if err := ValidateDocument(DocumentRef{}); !errors.Is(err, ErrMissingDocument) {
		t.Fatalf("ValidateDocument error = %v, want %v", err, ErrMissingDocument)
	}
}

func testDocument() DocumentRef {
	return DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: "/"}
}
