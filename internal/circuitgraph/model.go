package circuitgraph

import (
	"encoding/json"

	"kicadai/internal/components"
)

const (
	SchemaID           = "kicadai.circuit-graph.v1"
	Version            = 1
	ProviderProfileID  = "generic-circuit-v1"
	ProviderSchemaName = "kicadai_generic_circuit_graph_v1"
)

const (
	MaxDocumentBytes     = 4 << 20
	MaxComponents        = 512
	MaxUnitsPerComponent = 32
	MaxNets              = 1024
	MaxEndpointsPerNet   = 512
	MaxTotalEndpoints    = 32768
	MaxNoConnects        = 16384
	MaxBuses             = 128
	MaxPowerFlags        = 64
	MaxStringBytes       = 256
	MaxDescriptionBytes  = 2048
	MaxBoardDimensionMM  = 1000
)

type Document struct {
	Schema     string                     `json:"schema"`
	Version    int                        `json:"version"`
	Project    Project                    `json:"project"`
	Components []Component                `json:"components"`
	Nets       []Net                      `json:"nets"`
	NoConnects []Endpoint                 `json:"no_connects"`
	PowerFlags []PowerFlag                `json:"power_flags,omitempty"`
	Buses      []Bus                      `json:"buses"`
	Schematic  SchematicIntent            `json:"schematic"`
	PCB        PCBIntent                  `json:"pcb"`
	Policy     Policy                     `json:"policy"`
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// PowerFlag declares that an existing power or return net is driven by a
// source outside the modeled circuit. It lowers to a schematic-only PWR_FLAG.
type PowerFlag struct {
	Net string `json:"net"`
}

type Project struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Acceptance  AcceptanceLevel `json:"acceptance"`
	Board       Board           `json:"board"`
}

type AcceptanceLevel string

const (
	AcceptanceStructural           AcceptanceLevel = "structural"
	AcceptanceConnectivity         AcceptanceLevel = "connectivity"
	AcceptanceERCDRC               AcceptanceLevel = "erc-drc"
	AcceptanceFabricationCandidate AcceptanceLevel = "fabrication-candidate"
)

type Board struct {
	WidthMM         float64 `json:"width_mm"`
	HeightMM        float64 `json:"height_mm"`
	Layers          int     `json:"layers"`
	EdgeClearanceMM float64 `json:"edge_clearance_mm,omitempty"`
}

type Component struct {
	ID                string                     `json:"id"`
	Reference         string                     `json:"reference,omitempty"`
	Role              ComponentRole              `json:"role"`
	Units             []ComponentUnit            `json:"units,omitempty"`
	ComponentID       string                     `json:"component_id,omitempty"`
	VariantID         string                     `json:"variant_id,omitempty"`
	Query             *ComponentQuery            `json:"query,omitempty"`
	Value             string                     `json:"value,omitempty"`
	Parameters        []Parameter                `json:"parameters,omitempty"`
	Symbol            *LibraryConstraint         `json:"symbol,omitempty"`
	Footprint         *LibraryConstraint         `json:"footprint,omitempty"`
	RequiredRatings   []RequiredRating           `json:"required_ratings,omitempty"`
	RequiredFunctions []string                   `json:"required_functions,omitempty"`
	Manufacturer      string                     `json:"manufacturer,omitempty"`
	MPN               string                     `json:"mpn,omitempty"`
	Population        Population                 `json:"population"`
	Properties        []Property                 `json:"properties,omitempty"`
	Extensions        map[string]json.RawMessage `json:"extensions,omitempty"`
}

type ComponentUnit struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type ComponentRole string

const (
	RoleConnector           ComponentRole = "connector"
	RoleInputConnector      ComponentRole = "input_connector"
	RoleOutputConnector     ComponentRole = "output_connector"
	RoleResistor            ComponentRole = "resistor"
	RoleCurrentLimiter      ComponentRole = "current_limiter"
	RolePullup              ComponentRole = "pullup"
	RoleCapacitor           ComponentRole = "capacitor"
	RoleDecouplingCapacitor ComponentRole = "decoupling_capacitor"
	RoleBulkCapacitor       ComponentRole = "bulk_capacitor"
	RoleInductor            ComponentRole = "inductor"
	RoleDiode               ComponentRole = "diode"
	RoleIndicatorLED        ComponentRole = "indicator_led"
	RoleIC                  ComponentRole = "ic"
	RoleSensor              ComponentRole = "sensor"
	RoleRegulator           ComponentRole = "regulator"
	RoleTransistor          ComponentRole = "transistor"
	RoleBJT                 ComponentRole = "bjt"
	RoleMOSFET              ComponentRole = "mosfet"
	RoleSwitch              ComponentRole = "switch"
	RoleCrystal             ComponentRole = "crystal"
	RoleOscillator          ComponentRole = "oscillator"
	RoleProtection          ComponentRole = "protection"
	RoleFuse                ComponentRole = "fuse"
	RoleTVS                 ComponentRole = "tvs"
	RolePowerSymbol         ComponentRole = "power_symbol"
	RoleGroundSymbol        ComponentRole = "ground_symbol"
	RoleTestpoint           ComponentRole = "testpoint"
	RoleGeneric             ComponentRole = "generic"
)

type ComponentQuery struct {
	Text              string                     `json:"text,omitempty"`
	Family            string                     `json:"family,omitempty"`
	Package           string                     `json:"package,omitempty"`
	ValueKind         string                     `json:"value_kind,omitempty"`
	Value             string                     `json:"value,omitempty"`
	MinVoltageV       float64                    `json:"min_voltage_v,omitempty"`
	MinimumConfidence components.ConfidenceLevel `json:"minimum_confidence,omitempty"`
}

type LibraryConstraint struct {
	LibraryID string `json:"library_id"`
}

type RequiredRating struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

type Parameter struct {
	Name  string         `json:"name"`
	Value ParameterValue `json:"value"`
}

type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ParameterValue struct {
	String *string  `json:"-"`
	Number *float64 `json:"-"`
	Bool   *bool    `json:"-"`
	List   []string `json:"-"`
}

type Population string

const (
	PopulationPopulate      Population = "populate"
	PopulationDoNotPopulate Population = "do_not_populate"
)

type Net struct {
	Name             string     `json:"name"`
	Role             NetRole    `json:"role"`
	Required         *bool      `json:"required,omitempty"`
	VoltageDomain    string     `json:"voltage_domain,omitempty"`
	NetClass         string     `json:"net_class,omitempty"`
	CurrentMA        float64    `json:"current_ma,omitempty"`
	WidthMM          float64    `json:"width_mm,omitempty"`
	ClearanceMM      float64    `json:"clearance_mm,omitempty"`
	DifferentialPair string     `json:"differential_pair,omitempty"`
	Endpoints        []Endpoint `json:"endpoints"`
}

type NetRole string

const (
	NetRoleSignal   NetRole = "signal"
	NetRolePower    NetRole = "power"
	NetRolePowerPos NetRole = "power_pos"
	NetRolePowerNeg NetRole = "power_neg"
	NetRoleGround   NetRole = "ground"
	NetRoleReturn   NetRole = "return"
	NetRoleFeedback NetRole = "feedback"
	NetRoleBias     NetRole = "bias"
	NetRoleShield   NetRole = "shield"
)

type Endpoint struct {
	Component    string       `json:"component"`
	Unit         string       `json:"unit,omitempty"`
	SelectorKind SelectorKind `json:"selector_kind"`
	Selector     string       `json:"selector"`
}

type SelectorKind string

const (
	SelectorFunction  SelectorKind = "function"
	SelectorAlias     SelectorKind = "alias"
	SelectorSymbolPin SelectorKind = "symbol_pin"
)

type Bus struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Members []BusMember `json:"members"`
}

type BusMember struct {
	Net      string `json:"net"`
	Label    string `json:"label"`
	Polarity string `json:"polarity,omitempty"`
}

type SchematicIntent struct {
	Flow       Flow                 `json:"flow"`
	Origin     Origin               `json:"origin"`
	Groups     []SchematicGroup     `json:"groups"`
	Lanes      SchematicLanes       `json:"lanes"`
	Placements []SchematicPlacement `json:"placements"`
	Rules      SchematicRules       `json:"rules"`
	Hierarchy  HierarchyPolicy      `json:"hierarchy"`
}

type Flow string

const FlowLeftToRight Flow = "left_to_right"

type Origin string

const OriginCentered Origin = "centered"

type SchematicGroup struct {
	ID      string   `json:"id"`
	Label   string   `json:"label,omitempty"`
	Role    string   `json:"role,omitempty"`
	Members []string `json:"members"`
	Rank    int      `json:"rank"`
	Side    Side     `json:"side,omitempty"`
}

type Side string

const (
	SideLeft   Side = "left"
	SideRight  Side = "right"
	SideTop    Side = "top"
	SideBottom Side = "bottom"
)

type SchematicLanes struct {
	Power         Lane  `json:"power"`
	PowerNegative *Lane `json:"power_negative"`
	Signals       Lane  `json:"signals"`
	Ground        Lane  `json:"ground"`
}

type Lane string

const (
	LaneTop    Lane = "top"
	LaneLower  Lane = "lower"
	LaneMiddle Lane = "middle"
	LaneBottom Lane = "bottom"
)

type SchematicPlacement struct {
	Component   string `json:"component"`
	Unit        string `json:"unit,omitempty"`
	Group       string `json:"group,omitempty"`
	Near        string `json:"near,omitempty"`
	NearUnit    string `json:"near_unit,omitempty"`
	Above       string `json:"above,omitempty"`
	AboveUnit   string `json:"above_unit,omitempty"`
	RightOf     string `json:"right_of,omitempty"`
	RightOfUnit string `json:"right_of_unit,omitempty"`
	Orientation string `json:"orientation,omitempty"`
	Mirror      string `json:"mirror,omitempty"`
}

type SchematicRules struct {
	PositivePowerTop        *bool   `json:"positive_power_top"`
	GroundBottom            *bool   `json:"ground_bottom"`
	CenterOnPage            *bool   `json:"center_on_page"`
	PreferLabelsForLongNets *bool   `json:"prefer_labels_for_long_nets"`
	AvoidWireCrossings      *bool   `json:"avoid_wire_crossings"`
	MinGroupSpacingMM       float64 `json:"min_group_spacing_mm"`
	MinComponentSpacingMM   float64 `json:"min_component_spacing_mm"`
}

type HierarchyPolicy struct {
	Mode                  string `json:"mode"`
	MaxComponentsPerSheet int    `json:"max_components_per_sheet,omitempty"`
}

type PCBIntent struct {
	Regions    []PCBRegion    `json:"regions"`
	Placements []PCBPlacement `json:"placements"`
	Keepouts   []PCBKeepout   `json:"keepouts"`
	Zones      []PCBZone      `json:"zones"`
}

type Bounds struct {
	XMM      float64 `json:"x_mm"`
	YMM      float64 `json:"y_mm"`
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

type PCBRegion struct {
	ID     string `json:"id"`
	Role   string `json:"role,omitempty"`
	Bounds Bounds `json:"bounds"`
}

type PCBPlacement struct {
	Component     string  `json:"component"`
	Region        string  `json:"region,omitempty"`
	Near          string  `json:"near,omitempty"`
	Edge          Side    `json:"edge,omitempty"`
	Priority      int     `json:"priority,omitempty"`
	MaxDistanceMM float64 `json:"max_distance_mm,omitempty"`
}

type PCBKeepout struct {
	ID     string   `json:"id"`
	Bounds Bounds   `json:"bounds"`
	Layers []string `json:"layers"`
}

type PCBZone struct {
	Net         string   `json:"net"`
	Layers      []string `json:"layers"`
	ClearanceMM float64  `json:"clearance_mm,omitempty"`
}

type Policy struct {
	AllowReferenceAssignment *bool `json:"allow_reference_assignment"`
	AllowValueNormalization  *bool `json:"allow_value_normalization"`
	AllowLayoutInference     *bool `json:"allow_layout_inference"`
	AllowSpacingAdjustment   *bool `json:"allow_spacing_adjustment"`
	AllowLabelInsertion      *bool `json:"allow_label_insertion"`
	AllowPlacementAdjustment *bool `json:"allow_placement_adjustment"`
	AllowRouteRetry          *bool `json:"allow_route_retry"`
}
