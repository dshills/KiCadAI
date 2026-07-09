package schematicir

import (
	"encoding/json"
	"strings"
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
	Properties map[string]string `json:"properties,omitempty"`
}

type ComponentRole string

const (
	ComponentRoleConnector           ComponentRole = "connector"
	ComponentRoleInputConnector      ComponentRole = "input_connector"
	ComponentRoleOutputConnector     ComponentRole = "output_connector"
	ComponentRoleResistor            ComponentRole = "resistor"
	ComponentRoleCurrentLimiter      ComponentRole = "current_limiter"
	ComponentRolePullup              ComponentRole = "pullup"
	ComponentRoleCapacitor           ComponentRole = "capacitor"
	ComponentRoleDecouplingCapacitor ComponentRole = "decoupling_capacitor"
	ComponentRoleBulkCapacitor       ComponentRole = "bulk_capacitor"
	ComponentRoleInductor            ComponentRole = "inductor"
	ComponentRoleDiode               ComponentRole = "diode"
	ComponentRoleIndicatorLED        ComponentRole = "indicator_led"
	ComponentRoleIC                  ComponentRole = "ic"
	ComponentRoleSensor              ComponentRole = "sensor"
	ComponentRoleRegulator           ComponentRole = "regulator"
	ComponentRoleTransistor          ComponentRole = "transistor"
	ComponentRoleBJT                 ComponentRole = "bjt"
	ComponentRoleMOSFET              ComponentRole = "mosfet"
	ComponentRoleSwitch              ComponentRole = "switch"
	ComponentRoleCrystal             ComponentRole = "crystal"
	ComponentRoleOscillator          ComponentRole = "oscillator"
	ComponentRoleProtection          ComponentRole = "protection"
	ComponentRoleFuse                ComponentRole = "fuse"
	ComponentRoleTVS                 ComponentRole = "tvs"
	ComponentRolePowerSymbol         ComponentRole = "power_symbol"
	ComponentRoleGroundSymbol        ComponentRole = "ground_symbol"
	ComponentRoleTestpoint           ComponentRole = "testpoint"
	ComponentRoleGeneric             ComponentRole = "generic"
)

type Pin struct {
	Number    string  `json:"number"`
	Name      string  `json:"name,omitempty"`
	Role      PinRole `json:"role,omitempty"`
	NoConnect bool    `json:"no_connect,omitempty"`
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

type EndpointRef string

func (ref EndpointRef) Split() (componentID string, pinSelector string, ok bool) {
	componentID, pinSelector, ok = strings.Cut(string(ref), ".")
	return componentID, pinSelector, ok && componentID != "" && pinSelector != ""
}

type NetRole string

const (
	NetRoleSignal    NetRole = "signal"
	NetRolePower     NetRole = "power"
	NetRolePowerPos  NetRole = "power_pos"
	NetRolePowerNeg  NetRole = "power_neg"
	NetRoleGround    NetRole = "ground"
	NetRoleReturn    NetRole = "return"
	NetRoleFeedback  NetRole = "feedback"
	NetRoleBias      NetRole = "bias"
	NetRoleShield    NetRole = "shield"
	NetRoleNoConnect NetRole = "no_connect"
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
	Flow       Flow        `json:"flow"`
	Origin     Origin      `json:"origin"`
	Groups     []Group     `json:"groups,omitempty"`
	Lanes      Lanes       `json:"lanes"`
	Rules      LayoutRules `json:"rules"`
	Placements []Placement `json:"placements,omitempty"`
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
	Orientation Orientation `json:"orientation,omitempty"`
}

type Orientation string

const (
	OrientationNormal  Orientation = "normal"
	OrientationRotated Orientation = "rotated"
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
