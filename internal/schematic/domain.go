package schematic

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"kicadai/internal/kiapi"
)

var (
	ErrMissingDocument  = errors.New("document is required")
	ErrMissingLibrary   = errors.New("library id is required")
	ErrMissingReference = errors.New("reference is required")
	ErrMissingValue     = errors.New("value is required")
	ErrMissingText      = errors.New("text is required")
	ErrMissingLabelType = errors.New("label type is required")
	ErrInvalidLabelType = errors.New("invalid label type")
	ErrInvalidWire      = errors.New("wire requires at least two points")
	ErrZeroLengthWire   = errors.New("wire must not have zero length")
	ErrInvalidRotation  = errors.New("rotation must be finite")
)

const (
	OpKindAddSymbol = "add_symbol"
	OpKindAddWire   = "add_wire"
	OpKindAddLabel  = "add_label"
)

type LabelType string

const (
	LabelTypeLocal        LabelType = "local"
	LabelTypeGlobal       LabelType = "global"
	LabelTypeHierarchical LabelType = "hierarchical"
)

type DocumentRef struct {
	Type       kiapi.DocumentType `json:"type"`
	Identifier string             `json:"identifier"`
}

type Point struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

type AddSymbolRequest struct {
	Document        DocumentRef `json:"document"`
	LibraryID       string      `json:"library_id"`
	Reference       string      `json:"reference"`
	Value           string      `json:"value"`
	Position        Point       `json:"position"`
	RotationDegrees float64     `json:"rotation_degrees"`
}

type AddWireRequest struct {
	Document DocumentRef `json:"document"`
	Points   []Point     `json:"points"`
}

type AddLabelRequest struct {
	Document        DocumentRef `json:"document"`
	Text            string      `json:"text"`
	LabelType       LabelType   `json:"label_type"`
	Position        Point       `json:"position"`
	RotationDegrees float64     `json:"rotation_degrees"`
}

type PlannedOperation struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

func ValidateDocument(document DocumentRef) error {
	if document.Type == "" || document.Type == kiapi.DocumentTypeUnknown || strings.TrimSpace(document.Identifier) == "" {
		return ErrMissingDocument
	}
	return nil
}

func ValidateAddSymbol(request AddSymbolRequest) error {
	if err := ValidateDocument(request.Document); err != nil {
		return err
	}
	if strings.TrimSpace(request.LibraryID) == "" {
		return ErrMissingLibrary
	}
	if strings.TrimSpace(request.Reference) == "" {
		return ErrMissingReference
	}
	if strings.TrimSpace(request.Value) == "" {
		return ErrMissingValue
	}
	if !isFinite(request.RotationDegrees) {
		return ErrInvalidRotation
	}
	return nil
}

func ValidateAddWire(request AddWireRequest) error {
	if err := ValidateDocument(request.Document); err != nil {
		return err
	}
	if len(request.Points) < 2 {
		return ErrInvalidWire
	}
	for i := 1; i < len(request.Points); i++ {
		if request.Points[i] == request.Points[i-1] {
			return ErrZeroLengthWire
		}
	}
	return nil
}

func ValidateAddLabel(request AddLabelRequest) error {
	if err := ValidateDocument(request.Document); err != nil {
		return err
	}
	if strings.TrimSpace(request.Text) == "" {
		return ErrMissingText
	}
	if request.LabelType == "" {
		return ErrMissingLabelType
	}
	if !validLabelType(request.LabelType) {
		return ErrInvalidLabelType
	}
	if !isFinite(request.RotationDegrees) {
		return ErrInvalidRotation
	}
	return nil
}

func PlanAddSymbol(request AddSymbolRequest) (PlannedOperation, error) {
	if err := ValidateAddSymbol(request); err != nil {
		return PlannedOperation{}, err
	}
	return PlannedOperation{
		Kind:    OpKindAddSymbol,
		Summary: fmt.Sprintf("place %s %s at %d,%d", request.Reference, request.LibraryID, request.Position.X, request.Position.Y),
	}, nil
}

func PlanAddWire(request AddWireRequest) (PlannedOperation, error) {
	if err := ValidateAddWire(request); err != nil {
		return PlannedOperation{}, err
	}
	return PlannedOperation{
		Kind:    OpKindAddWire,
		Summary: fmt.Sprintf("draw wire with %d points", len(request.Points)),
	}, nil
}

func PlanAddLabel(request AddLabelRequest) (PlannedOperation, error) {
	if err := ValidateAddLabel(request); err != nil {
		return PlannedOperation{}, err
	}
	return PlannedOperation{
		Kind:    OpKindAddLabel,
		Summary: fmt.Sprintf("place %s label %q at %d,%d", request.LabelType, request.Text, request.Position.X, request.Position.Y),
	}, nil
}

func validLabelType(labelType LabelType) bool {
	switch labelType {
	case LabelTypeLocal, LabelTypeGlobal, LabelTypeHierarchical:
		return true
	default:
		return false
	}
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
