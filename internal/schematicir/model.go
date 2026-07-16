package schematicir

import (
	"encoding/json"
	"strings"

	"kicadai/internal/domain"
)

const (
	SchemaID = "kicadai.schematic.ir.v1"
	Version  = 1

	DefaultPaper                 = "A4"
	DefaultMinGroupSpacingMM     = 12.7
	DefaultMinComponentSpacingMM = 7.62
)

type Document struct {
	Schema     string                     `json:"schema"`
	Version    int                        `json:"version"`
	Metadata   Metadata                   `json:"metadata"`
	Circuit    Circuit                    `json:"circuit"`
	Layout     Layout                     `json:"layout"`
	Policy     Policy                     `json:"policy"`
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

type Metadata struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Seed        string `json:"seed,omitempty"`
	Paper       string `json:"paper,omitempty"`
}

type Circuit struct {
	Components []Component                `json:"components"`
	Nets       []Net                      `json:"nets"`
	Ports      []Port                     `json:"ports,omitempty"`
	Buses      []Bus                      `json:"buses,omitempty"`
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

type Component struct {
	// ID is the document-local component identifier. It must be unique and cannot contain dots.
	ID         string            `json:"id"`
	Ref        string            `json:"ref,omitempty"`
	Unit       string            `json:"unit,omitempty"`
	Role       ComponentRole     `json:"role"`
	Symbol     string            `json:"symbol"`
	Value      string            `json:"value,omitempty"`
	Footprint  string            `json:"footprint,omitempty"`
	Pins       []Pin             `json:"pins,omitempty"`
	Body       *BodyGeometry     `json:"body,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// BodyGeometry describes the symbol graphics bounds relative to the symbol
// origin, in millimetres. It lets arbitrary or externally resolved symbols
// participate in obstacle-aware placement and routing without role heuristics.
type BodyGeometry struct {
	MinXMM float64 `json:"min_x_mm"`
	MinYMM float64 `json:"min_y_mm"`
	MaxXMM float64 `json:"max_x_mm"`
	MaxYMM float64 `json:"max_y_mm"`
}

type ComponentRole = domain.ComponentRole

const (
	// Package-local names remain source-compatible aliases to the shared
	// vocabulary; schematic IR retains bus and no-connect values.
	ComponentRoleConnector           = domain.ComponentRoleConnector
	ComponentRoleInputConnector      = domain.ComponentRoleInputConnector
	ComponentRoleOutputConnector     = domain.ComponentRoleOutputConnector
	ComponentRoleResistor            = domain.ComponentRoleResistor
	ComponentRoleCurrentLimiter      = domain.ComponentRoleCurrentLimiter
	ComponentRolePullup              = domain.ComponentRolePullup
	ComponentRoleCapacitor           = domain.ComponentRoleCapacitor
	ComponentRoleDecouplingCapacitor = domain.ComponentRoleDecouplingCapacitor
	ComponentRoleBulkCapacitor       = domain.ComponentRoleBulkCapacitor
	ComponentRoleInductor            = domain.ComponentRoleInductor
	ComponentRoleDiode               = domain.ComponentRoleDiode
	ComponentRoleIndicatorLED        = domain.ComponentRoleIndicatorLED
	ComponentRoleIC                  = domain.ComponentRoleIC
	ComponentRoleSensor              = domain.ComponentRoleSensor
	ComponentRoleRegulator           = domain.ComponentRoleRegulator
	ComponentRoleTransistor          = domain.ComponentRoleTransistor
	ComponentRoleBJT                 = domain.ComponentRoleBJT
	ComponentRoleMOSFET              = domain.ComponentRoleMOSFET
	ComponentRoleSwitch              = domain.ComponentRoleSwitch
	ComponentRoleCrystal             = domain.ComponentRoleCrystal
	ComponentRoleOscillator          = domain.ComponentRoleOscillator
	ComponentRoleProtection          = domain.ComponentRoleProtection
	ComponentRoleFuse                = domain.ComponentRoleFuse
	ComponentRoleTVS                 = domain.ComponentRoleTVS
	ComponentRolePowerSymbol         = domain.ComponentRolePowerSymbol
	ComponentRoleGroundSymbol        = domain.ComponentRoleGroundSymbol
	ComponentRoleTestpoint           = domain.ComponentRoleTestpoint
	ComponentRoleGeneric             = domain.ComponentRoleGeneric
)

type Pin struct {
	Number    string   `json:"number"`
	Name      string   `json:"name,omitempty"`
	Role      PinRole  `json:"role,omitempty"`
	NoConnect bool     `json:"no_connect,omitempty"`
	OffsetXMM *float64 `json:"offset_x_mm,omitempty"`
	OffsetYMM *float64 `json:"offset_y_mm,omitempty"`
}

type PinRole string

const (
	PinRoleInput         PinRole = "input"
	PinRoleOutput        PinRole = "output"
	PinRolePower         PinRole = "power"
	PinRoleGround        PinRole = "ground"
	PinRolePassive       PinRole = "passive"
	PinRoleBidirectional PinRole = "bidirectional"
)

type Net struct {
	// Name is the normalized circuit-scope net name.
	Name string  `json:"name"`
	Role NetRole `json:"role"`
	// Connect contains endpoints in the format component_id.pin_selector, where
	// pin_selector resolves to Pin.Number or symbol-library pin metadata.
	// The first dot is the separator; component IDs cannot contain dots.
	Connect  []EndpointRef `json:"connect"`
	Label    string        `json:"label,omitempty"`
	UseLabel *bool         `json:"use_label,omitempty"`
}

// Bus declares a KiCad vector bus whose members remain ordinary scalar nets.
// Geometry is intentionally kept in Layout so circuit intent is independent
// from drawing coordinates.
type Bus struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Members []BusMember `json:"members"`
}

type BusMember struct {
	Net   string `json:"net"`
	Label string `json:"label"`
}

type EndpointRef string

func (ref EndpointRef) Split() (componentID string, pinSelector string, ok bool) {
	componentID, pinSelector, ok = strings.Cut(string(ref), ".")
	return componentID, pinSelector, ok && componentID != "" && pinSelector != ""
}

type NetRole = domain.NetRole

const (
	// Net-role aliases preserve schematic IR API names and wire values.
	NetRoleSignal    = domain.NetRoleSignal
	NetRolePower     = domain.NetRolePower
	NetRolePowerPos  = domain.NetRolePowerPos
	NetRolePowerNeg  = domain.NetRolePowerNeg
	NetRoleGround    = domain.NetRoleGround
	NetRoleReturn    = domain.NetRoleReturn
	NetRoleFeedback  = domain.NetRoleFeedback
	NetRoleBias      = domain.NetRoleBias
	NetRoleShield    = domain.NetRoleShield
	NetRoleBus       = domain.NetRoleBus
	NetRoleNoConnect = domain.NetRoleNoConnect
)

type Port struct {
	Name           string         `json:"name"`
	Direction      PortDirection  `json:"direction"`
	ElectricalType ElectricalType `json:"electrical_type,omitempty"`
	// Net references a normalized circuit-scope circuit.nets[].name value.
	// Strict referential checks live in validation.
	Net  string `json:"net"`
	Side Side   `json:"side"`
}

type PortDirection string

const (
	PortDirectionInput         PortDirection = "input"
	PortDirectionOutput        PortDirection = "output"
	PortDirectionBidirectional PortDirection = "bidirectional"
	PortDirectionPassive       PortDirection = "passive"
	PortDirectionTriState      PortDirection = "tri_state"
	PortDirectionUnspecified   PortDirection = "unspecified"
)

type ElectricalType string

const (
	ElectricalTypeInput         ElectricalType = "input"
	ElectricalTypeOutput        ElectricalType = "output"
	ElectricalTypeBidirectional ElectricalType = "bidirectional"
	ElectricalTypeTriState      ElectricalType = "tri_state"
	ElectricalTypePassive       ElectricalType = "passive"
	ElectricalTypeUnspecified   ElectricalType = "unspecified"
	ElectricalTypePowerInput    ElectricalType = "power_input"
	ElectricalTypePowerOutput   ElectricalType = "power_output"
	ElectricalTypeOpenCollector ElectricalType = "open_collector"
	ElectricalTypeOpenEmitter   ElectricalType = "open_emitter"
	ElectricalTypeNoConnect     ElectricalType = "no_connect"
)

type Layout struct {
	Flow                  Flow        `json:"flow"`
	Origin                Origin      `json:"origin"`
	Groups                []Group     `json:"groups,omitempty"`
	Lanes                 Lanes       `json:"lanes"`
	Rules                 LayoutRules `json:"rules"`
	MaxComponentsPerSheet int         `json:"max_components_per_sheet,omitempty"`
	Placements            []Placement `json:"placements,omitempty"`
	Buses                 []BusLayout `json:"buses,omitempty"`
}

type Flow string

const (
	FlowLeftToRight Flow = "left_to_right"
)

type Origin string

const (
	OriginCentered      Origin = "centered"
	OriginPageUpperLeft Origin = "page_upper_left"
)

type Group struct {
	ID    string    `json:"id"`
	Label string    `json:"label,omitempty"`
	Role  GroupRole `json:"role,omitempty"`
	// Members contains component IDs belonging to this group.
	Members []string `json:"members"`
	Rank    int      `json:"rank"`
	Side    Side     `json:"side,omitempty"`
	// Inferred is runtime-only normalization evidence. Explicit groups are hard
	// rank constraints; inferred groups remain graph-layout hints.
	Inferred bool `json:"-"`
}

type GroupRole string

const (
	GroupRoleInputStage       GroupRole = "input_stage"
	GroupRolePowerStage       GroupRole = "power_stage"
	GroupRoleRegulatorStage   GroupRole = "regulator_stage"
	GroupRoleProcessingStage  GroupRole = "processing_stage"
	GroupRoleOutputStage      GroupRole = "output_stage"
	GroupRoleProtectionStage  GroupRole = "protection_stage"
	GroupRoleConnectorStage   GroupRole = "connector_stage"
	GroupRoleDecouplingStage  GroupRole = "decoupling_stage"
	GroupRoleUnspecifiedStage GroupRole = "unspecified_stage"
)

type Side string

const (
	SideLeft   Side = "left"
	SideRight  Side = "right"
	SideTop    Side = "top"
	SideBottom Side = "bottom"
)

type Lanes struct {
	Power         LanePosition `json:"power"`
	PowerNegative LanePosition `json:"power_negative,omitempty"`
	Ground        LanePosition `json:"ground"`
	Signals       LanePosition `json:"signals"`
}

type LanePosition string

const (
	LanePositionNone   LanePosition = ""
	LanePositionTop    LanePosition = "top"
	LanePositionMiddle LanePosition = "middle"
	LanePositionLower  LanePosition = "lower"
	LanePositionBottom LanePosition = "bottom"
)

type LayoutRules struct {
	PositivePowerTop        *bool    `json:"positive_power_top,omitempty"`
	GroundBottom            *bool    `json:"ground_bottom,omitempty"`
	CenterOnPage            *bool    `json:"center_on_page,omitempty"`
	PreferLabelsForLongNets *bool    `json:"prefer_labels_for_long_nets,omitempty"`
	AvoidWireCrossings      *bool    `json:"avoid_wire_crossings,omitempty"`
	MinGroupSpacingMM       *float64 `json:"min_group_spacing_mm,omitempty"`
	MinComponentSpacingMM   *float64 `json:"min_component_spacing_mm,omitempty"`
}

type Placement struct {
	Target      string      `json:"target"`
	Group       string      `json:"group,omitempty"`
	Near        []string    `json:"near,omitempty"`
	Above       []string    `json:"above,omitempty"`
	RightOf     []string    `json:"right_of,omitempty"`
	Orientation Orientation `json:"orientation,omitempty"`
	Mirror      Mirror      `json:"mirror,omitempty"`
}

type BusLayout struct {
	Bus     string           `json:"bus"`
	Points  []LayoutPoint    `json:"points"`
	Entries []BusEntryLayout `json:"entries"`
}

type BusEntryLayout struct {
	Member   string      `json:"member"`
	Endpoint EndpointRef `json:"endpoint"`
	At       LayoutPoint `json:"at"`
	Size     LayoutPoint `json:"size"`
}

type LayoutPoint struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type Orientation string

const (
	OrientationNormal     Orientation = "normal"
	OrientationRotated    Orientation = "rotated" // Backward-compatible 90 degrees.
	OrientationRotated90  Orientation = "rotated_90"
	OrientationRotated180 Orientation = "rotated_180"
	OrientationRotated270 Orientation = "rotated_270"
)

// Mirror uses KiCad's symbol-instance axes. "x" reflects across the X axis
// and "y" reflects across the Y axis before the instance rotation.
type Mirror string

const (
	MirrorNone Mirror = ""
	MirrorX    Mirror = "x"
	MirrorY    Mirror = "y"
)

type Policy struct {
	Validation map[string]IssueAction `json:"validation,omitempty"`
	Repair     RepairPolicy           `json:"repair"`
	Acceptance AcceptanceLevel        `json:"acceptance"`
}

type IssueAction string

const (
	IssueActionError   IssueAction = "error"
	IssueActionWarning IssueAction = "warning"
	IssueActionIgnore  IssueAction = "ignore"
)

type RepairPolicy struct {
	AllowRefAssignment          bool `json:"allow_ref_assignment"`
	AllowLabelInsertion         bool `json:"allow_label_insertion"`
	AllowGroupSpacingAdjustment bool `json:"allow_group_spacing_adjustment"`
	AllowSymbolSubstitution     bool `json:"allow_symbol_substitution"`
	AllowPinGuessing            bool `json:"allow_pin_guessing"`
}

type AcceptanceLevel string

const (
	AcceptanceStructural AcceptanceLevel = "structural"
	AcceptanceERCClean   AcceptanceLevel = "erc_clean"
	AcceptanceReadable   AcceptanceLevel = "readable"
)

func NewDocument() *Document {
	return &Document{
		Schema:  SchemaID,
		Version: Version,
		Metadata: Metadata{
			Name:  "untitled",
			Paper: DefaultPaper,
		},
		Circuit: Circuit{
			Components: []Component{},
			Nets:       []Net{},
			Buses:      []Bus{},
		},
		Layout: Layout{
			Flow:   FlowLeftToRight,
			Origin: OriginCentered,
			Lanes: Lanes{
				Power:   LanePositionTop,
				Ground:  LanePositionBottom,
				Signals: LanePositionMiddle,
			},
			Rules: LayoutRules{
				PositivePowerTop:        boolPtr(true),
				GroundBottom:            boolPtr(true),
				PreferLabelsForLongNets: boolPtr(true),
				AvoidWireCrossings:      boolPtr(true),
				MinGroupSpacingMM:       floatPtr(DefaultMinGroupSpacingMM),
				MinComponentSpacingMM:   floatPtr(DefaultMinComponentSpacingMM),
			},
		},
		Policy: Policy{
			Repair: RepairPolicy{
				AllowRefAssignment:          true,
				AllowLabelInsertion:         true,
				AllowGroupSpacingAdjustment: true,
				AllowSymbolSubstitution:     false,
				AllowPinGuessing:            false,
			},
			Acceptance: AcceptanceStructural,
		},
	}
}
