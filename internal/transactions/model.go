package transactions

import "encoding/json"

type OperationKind string

const (
	OpCreateProject    OperationKind = "create_project"
	OpSetBoardOutline  OperationKind = "set_board_outline"
	OpAddSymbol        OperationKind = "add_symbol"
	OpAddLabel         OperationKind = "add_label"
	OpAddBus           OperationKind = "add_bus"
	OpAddBusEntry      OperationKind = "add_bus_entry"
	OpAddSchematicWire OperationKind = "add_schematic_wire"
	OpConnect          OperationKind = "connect"
	OpAssignFootprint  OperationKind = "assign_footprint"
	OpPlaceFootprint   OperationKind = "place_footprint"
	OpRoute            OperationKind = "route"
	OpAddZone          OperationKind = "add_zone"
	OpAddNoConnect     OperationKind = "add_no_connect"
	OpWriteProject     OperationKind = "write_project"
	OpRemoveSymbol     OperationKind = "remove_symbol"
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
	Ref   string          `json:"-"`
	Net   string          `json:"-"`
	// SnapExempt is in-memory routing-stage metadata. It is set and consumed
	// during a single RoutePlacement call before operations are serialized.
	SnapExempt bool `json:"-"`
}

func (tx Transaction) Clone() Transaction {
	clone := tx
	clone.Operations = append([]Operation(nil), tx.Operations...)
	for index := range clone.Operations {
		clone.Operations[index] = clone.Operations[index].Clone()
	}
	return clone
}

func (op Operation) Clone() Operation {
	// Operation currently stores scalar metadata plus the raw operation JSON.
	// The raw payload is the only mutable backing store that needs copying.
	clone := op
	clone.Raw = append(json.RawMessage(nil), op.Raw...)
	return clone
}

func (op *Operation) UnmarshalJSON(data []byte) error {
	var head struct {
		Op      OperationKind   `json:"op"`
		Ref     json.RawMessage `json:"ref"`
		NetName json.RawMessage `json:"net_name"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	op.Op = head.Op
	op.Raw = append([]byte(nil), data...)
	op.Ref = operationRefFromRawValue(head.Ref)
	op.Net = operationStringFromRawValue(head.NetName)
	return nil
}

func (op Operation) MarshalJSON() ([]byte, error) {
	if len(op.Raw) > 0 {
		return op.Raw, nil
	}
	type alias Operation
	return json.Marshal(alias(op))
}

// NewOperation wraps a complete operation JSON object. The raw payload should
// include the same "op" field that would be present after unmarshalling a
// transaction file.
func NewOperation(kind OperationKind, raw json.RawMessage) Operation {
	metadata := operationMetadataFromRaw(raw)
	return newOperationWithMetadata(kind, raw, metadata.Ref, metadata.NetName)
}

// NewOperationWithRef wraps a complete operation JSON object and attaches
// already-known reference metadata for callers that constructed the payload.
func NewOperationWithRef(kind OperationKind, raw json.RawMessage, ref string) Operation {
	return newOperationWithMetadata(kind, raw, ref, "")
}

// NewOperationWithMetadata wraps a complete operation JSON object and attaches
// already-known metadata for callers that constructed the payload.
func NewOperationWithMetadata(kind OperationKind, raw json.RawMessage, ref string, netName string) Operation {
	return newOperationWithMetadata(kind, raw, ref, netName)
}

func newOperationWithMetadata(kind OperationKind, raw json.RawMessage, ref string, netName string) Operation {
	return Operation{Op: kind, Raw: append([]byte(nil), raw...), Ref: ref, Net: netName}
}

func operationRefFromRawValue(raw json.RawMessage) string {
	return operationStringFromRawValue(raw)
}

func operationStringFromRawValue(raw json.RawMessage) string {
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		return ""
	}
	return ref
}

type operationMetadata struct {
	Ref     string `json:"ref"`
	NetName string `json:"net_name"`
}

func operationMetadataFromRaw(data json.RawMessage) operationMetadata {
	var metadata operationMetadata
	if len(data) > 0 {
		_ = json.Unmarshal(data, &metadata)
	}
	return metadata
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
	Ref  string `json:"ref"`
	Pin  string `json:"pin"`
	Unit int    `json:"unit,omitempty"`
}

type PinSpec struct {
	Number         string  `json:"number"`
	XMM            float64 `json:"x_mm,omitempty"`
	YMM            float64 `json:"y_mm,omitempty"`
	ExplicitOffset bool    `json:"explicit_offset,omitempty"`
}

type PadSpec struct {
	Name        string  `json:"name"`
	Type        string  `json:"type,omitempty"`
	Shape       string  `json:"shape,omitempty"`
	XMM         float64 `json:"x_mm,omitempty"`
	YMM         float64 `json:"y_mm,omitempty"`
	RotationDeg float64 `json:"rotation_deg,omitempty"`
	WidthMM     float64 `json:"width_mm,omitempty"`
	HeightMM    float64 `json:"height_mm,omitempty"`
	DrillMM     float64 `json:"drill_mm,omitempty"`
	Net         *string `json:"net,omitempty"`
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
	Op                   OperationKind    `json:"op"`
	Ref                  string           `json:"ref"`
	Unit                 int              `json:"unit,omitempty"`
	Role                 string           `json:"role,omitempty"`
	Value                string           `json:"value,omitempty"`
	LibraryID            string           `json:"library_id"`
	At                   Point            `json:"at"`
	Rotation             float64          `json:"rotation_deg,omitempty"`
	Mirror               string           `json:"mirror,omitempty"`
	Pins                 []PinSpec        `json:"pins,omitempty"`
	Properties           []SymbolProperty `json:"properties,omitempty"`
	PreferResolverSymbol bool             `json:"prefer_resolver_symbol,omitempty"`
}

type SymbolProperty struct {
	Name           string   `json:"name"`
	Value          string   `json:"value"`
	Private        bool     `json:"private,omitempty"`
	Hidden         bool     `json:"hidden,omitempty"`
	ShowName       *bool    `json:"show_name,omitempty"`
	DoNotAutoplace *bool    `json:"do_not_autoplace,omitempty"`
	At             *Point   `json:"at,omitempty"`
	Rotation       *float64 `json:"rotation_deg,omitempty"`
}

type ConnectOperation struct {
	Op                 OperationKind `json:"op"`
	From               Endpoint      `json:"from"`
	To                 Endpoint      `json:"to"`
	NetName            string        `json:"net_name"`
	UseLabels          *bool         `json:"use_labels,omitempty"`
	SuppressBendLabels bool          `json:"suppress_bend_labels,omitempty"`
	SkipFromLabel      bool          `json:"skip_from_label,omitempty"`
	SkipToLabel        bool          `json:"skip_to_label,omitempty"`
	Waypoints          []Point       `json:"waypoints,omitempty"`
	FromLabelAt        *Point        `json:"from_label_at,omitempty"`
	ToLabelAt          *Point        `json:"to_label_at,omitempty"`
}

type AddLabelOperation struct {
	Op          OperationKind `json:"op"`
	Text        string        `json:"text"`
	At          Point         `json:"at"`
	Kind        string        `json:"kind,omitempty"`
	RotationDeg float64       `json:"rotation_deg,omitempty"`
	Shape       string        `json:"shape,omitempty"`
}

type AddBusOperation struct {
	Op     OperationKind `json:"op"`
	Points []Point       `json:"points"`
}

type AddBusEntryOperation struct {
	Op   OperationKind `json:"op"`
	At   Point         `json:"at"`
	Size Point         `json:"size"`
}

type AddSchematicWireOperation struct {
	Op          OperationKind `json:"op"`
	NetName     string        `json:"net_name"`
	Points      []Point       `json:"points"`
	Label       string        `json:"label,omitempty"`
	LabelAt     *Point        `json:"label_at,omitempty"`
	LabelRotate float64       `json:"label_rotation_deg,omitempty"`
}

type AddNoConnectOperation struct {
	Op       OperationKind `json:"op"`
	Endpoint Endpoint      `json:"endpoint"`
}

type AssignFootprintOperation struct {
	Op          OperationKind `json:"op"`
	Ref         string        `json:"ref"`
	Role        string        `json:"role,omitempty"`
	FootprintID string        `json:"footprint_id"`
}

type PlaceFootprintOperation struct {
	Op                            OperationKind `json:"op"`
	Ref                           string        `json:"ref"`
	Role                          string        `json:"role,omitempty"`
	FootprintID                   string        `json:"footprint_id,omitempty"`
	Value                         string        `json:"value,omitempty"`
	At                            Point         `json:"at"`
	Rotation                      float64       `json:"rotation_deg,omitempty"`
	Layer                         string        `json:"layer,omitempty"`
	Pads                          []PadSpec     `json:"pads,omitempty"`
	AllowUnmatchedUnconnectedPads bool          `json:"allow_unmatched_unconnected_pads"`
	// HideDefaultFootprintText hides generated KiCad Reference and Value properties.
	HideDefaultFootprintText bool `json:"hide_default_footprint_text,omitempty"`
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
	Op          OperationKind `json:"op"`
	Name        string        `json:"name,omitempty"`
	NetName     *string       `json:"net_name"`
	Layers      []string      `json:"layers,omitempty"`
	Polygon     []Point       `json:"polygon"`
	ClearanceMM float64       `json:"clearance_mm,omitempty"`
}

type WriteProjectOperation struct {
	Op                          OperationKind       `json:"op"`
	OutputDir                   string              `json:"output_dir,omitempty"`
	Overwrite                   bool                `json:"overwrite,omitempty"`
	SchematicOnly               bool                `json:"schematic_only,omitempty"`
	RequireSchematicReadability bool                `json:"require_schematic_readability,omitempty"`
	Hierarchy                   *SchematicHierarchy `json:"hierarchy,omitempty"`
}

type SchematicHierarchy struct {
	Sheets         []SchematicHierarchySheet `json:"sheets"`
	CrossSheetNets []SchematicCrossSheetNet  `json:"cross_sheet_nets,omitempty"`
	Buses          []SchematicHierarchyBus   `json:"buses,omitempty"`
}

type SchematicHierarchySheet struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Filename   string               `json:"filename"`
	References []string             `json:"references"`
	Symbols    []SchematicSymbolRef `json:"symbols,omitempty"`
}

type SchematicSymbolRef struct {
	Ref  string `json:"ref"`
	Unit int    `json:"unit,omitempty"`
}

type SchematicCrossSheetNet struct {
	Name      string     `json:"name"`
	Endpoints []Endpoint `json:"endpoints"`
}

type SchematicHierarchyBus struct {
	ID      string                    `json:"id"`
	Name    string                    `json:"name"`
	Points  []Point                   `json:"points"`
	Entries []SchematicHierarchyEntry `json:"entries"`
}

type SchematicHierarchyEntry struct {
	Member   string   `json:"member"`
	Label    string   `json:"label,omitempty"`
	Endpoint Endpoint `json:"endpoint"`
	At       Point    `json:"at"`
	Size     Point    `json:"size"`
}
