package transactions

import "encoding/json"

type OperationKind string

const (
	OpCreateProject   OperationKind = "create_project"
	OpSetBoardOutline OperationKind = "set_board_outline"
	OpAddSymbol       OperationKind = "add_symbol"
	OpConnect         OperationKind = "connect"
	OpAssignFootprint OperationKind = "assign_footprint"
	OpPlaceFootprint  OperationKind = "place_footprint"
	OpRoute           OperationKind = "route"
	OpAddZone         OperationKind = "add_zone"
	OpWriteProject    OperationKind = "write_project"
	OpRemoveSymbol    OperationKind = "remove_symbol"
)

type Transaction struct {
	Name       string      `json:"name,omitempty"`
	Project    string      `json:"project,omitempty"`
	Operations []Operation `json:"operations"`
}

type Operation struct {
	Op    OperationKind   `json:"op"`
	Index int             `json:"-"`
	Raw   json.RawMessage `json:"-"`
}

func (op *Operation) UnmarshalJSON(data []byte) error {
	var head struct {
		Op OperationKind `json:"op"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	op.Op = head.Op
	op.Raw = append([]byte(nil), data...)
	return nil
}

func (op Operation) MarshalJSON() ([]byte, error) {
	if len(op.Raw) > 0 {
		return op.Raw, nil
	}
	type alias Operation
	return json.Marshal(alias(op))
}

type Point struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type BoardSize struct {
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

type Endpoint struct {
	Ref string `json:"ref"`
	Pin string `json:"pin"`
}

type PinSpec struct {
	Number string  `json:"number"`
	XMM    float64 `json:"x_mm,omitempty"`
	YMM    float64 `json:"y_mm,omitempty"`
}

type PadSpec struct {
	Name     string  `json:"name"`
	Type     string  `json:"type,omitempty"`
	Shape    string  `json:"shape,omitempty"`
	XMM      float64 `json:"x_mm,omitempty"`
	YMM      float64 `json:"y_mm,omitempty"`
	WidthMM  float64 `json:"width_mm,omitempty"`
	HeightMM float64 `json:"height_mm,omitempty"`
	DrillMM  float64 `json:"drill_mm,omitempty"`
	Net      *string `json:"net,omitempty"`
}

type CreateProjectOperation struct {
	Op    OperationKind `json:"op"`
	Name  string        `json:"name"`
	Paper string        `json:"paper,omitempty"`
}

type SetBoardOutlineOperation struct {
	Op     OperationKind `json:"op"`
	Board  *BoardSize    `json:"board,omitempty"`
	Points []Point       `json:"points,omitempty"`
}

type AddSymbolOperation struct {
	Op        OperationKind `json:"op"`
	Ref       string        `json:"ref"`
	Value     string        `json:"value,omitempty"`
	LibraryID string        `json:"library_id"`
	At        Point         `json:"at"`
	Pins      []PinSpec     `json:"pins,omitempty"`
}

type ConnectOperation struct {
	Op      OperationKind `json:"op"`
	From    Endpoint      `json:"from"`
	To      Endpoint      `json:"to"`
	NetName string        `json:"net_name"`
}

type AssignFootprintOperation struct {
	Op          OperationKind `json:"op"`
	Ref         string        `json:"ref"`
	FootprintID string        `json:"footprint_id"`
}

type PlaceFootprintOperation struct {
	Op          OperationKind `json:"op"`
	Ref         string        `json:"ref"`
	FootprintID string        `json:"footprint_id,omitempty"`
	Value       string        `json:"value,omitempty"`
	At          Point         `json:"at"`
	Rotation    float64       `json:"rotation_deg,omitempty"`
	Layer       string        `json:"layer,omitempty"`
	Pads        []PadSpec     `json:"pads,omitempty"`
}

type RouteOperation struct {
	Op      OperationKind  `json:"op"`
	NetName string         `json:"net_name"`
	Layer   string         `json:"layer,omitempty"`
	WidthMM float64        `json:"width_mm,omitempty"`
	Points  []Point        `json:"points"`
	Vias    []RouteViaSpec `json:"vias,omitempty"`
}

type RouteViaSpec struct {
	At         Point    `json:"at"`
	DiameterMM float64  `json:"diameter_mm"`
	DrillMM    float64  `json:"drill_mm"`
	Layers     []string `json:"layers,omitempty"`
}

type AddZoneOperation struct {
	Op      OperationKind `json:"op"`
	Name    string        `json:"name,omitempty"`
	NetName *string       `json:"net_name"`
	Layers  []string      `json:"layers,omitempty"`
	Polygon []Point       `json:"polygon"`
}

type WriteProjectOperation struct {
	Op        OperationKind `json:"op"`
	OutputDir string        `json:"output_dir,omitempty"`
	Overwrite bool          `json:"overwrite,omitempty"`
}
