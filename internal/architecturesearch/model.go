// Package architecturesearch defines behavioral electrical requirements and
// deterministic bounded circuit-architecture search.
package architecturesearch

import "encoding/json"

const (
	SchemaID        = "kicadai.open-set-requirement.v1"
	Version         = 1
	SchemaIDV2      = "kicadai.open-set-requirement.v2"
	VersionV2       = 2
	SchemaIDV3      = "kicadai.open-set-requirement.v3"
	VersionV3       = 3
	PolicyVersion   = "architecture-search-policy-v1"
	PolicyVersionV2 = "architecture-search-policy-v2"
	PolicyVersionV3 = "architecture-search-policy-v3"

	MaxRequirementBytes       = 256 * 1024
	MaxDomains                = 16
	MaxPorts                  = 64
	MaxSignals                = 64
	MaxParticipants           = 16
	MaxParticipantPorts       = 32
	MaxObjectives             = 32
	MaxBindings               = 64
	MaxConstraints            = 64
	MaxOperatingCases         = 16
	MaxCaseConditions         = 16
	MaxBehavioralRequirements = 64
	MaxComponents             = 64
	MaxBoardDimensionMM       = 200.0
)

type Requirement struct {
	Schema       string       `json:"schema"`
	Version      int          `json:"version"`
	Project      Project      `json:"project"`
	Requirements Requirements `json:"requirements"`
	Acceptance   Acceptance   `json:"acceptance"`
}

type Project struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type Requirements struct {
	Domains                []Domain                `json:"domains"`
	Ports                  []Port                  `json:"ports"`
	Signals                []Signal                `json:"signals,omitempty"`
	Participants           []Participant           `json:"participants,omitempty"`
	Objectives             []Objective             `json:"objectives"`
	SystemConstraints      []Constraint            `json:"system_constraints,omitempty"`
	OperatingCases         []OperatingCase         `json:"operating_cases,omitempty"`
	BehavioralRequirements []BehavioralRequirement `json:"behavioral_requirements,omitempty"`
	Constraints            BoardLimits             `json:"constraints"`
}

type Domain struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	MinVoltageV     *float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV float64  `json:"nominal_voltage_v"`
	MaxVoltageV     *float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA     *float64 `json:"max_current_a,omitempty"`
	Source          string   `json:"source"`
}

type Port struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Direction  string      `json:"direction"`
	Domain     string      `json:"domain"`
	Electrical *Electrical `json:"electrical,omitempty"`
	Protocol   *Protocol   `json:"protocol,omitempty"`
}

// Signal is a behavior-level interface between objectives. It deliberately
// omits implementation concepts such as nets, pins, and topology. Direction is
// declared by each binding so a producer and consumers can share one contract.
type Signal struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Domain     string      `json:"domain"`
	Electrical *Electrical `json:"electrical,omitempty"`
	Protocol   *Protocol   `json:"protocol,omitempty"`
}

type Electrical struct {
	MinVoltageV          *float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV      *float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV          *float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA          *float64 `json:"max_current_a,omitempty"`
	MaxSourceCurrentMA   *float64 `json:"max_source_current_ma,omitempty"`
	InputImpedanceMinOhm *float64 `json:"input_impedance_min_ohm,omitempty"`
	FrequencyMaxHz       *float64 `json:"frequency_max_hz,omitempty"`
	DefaultState         string   `json:"default_state,omitempty"`
}

type Protocol struct {
	Name           string  `json:"name"`
	Mode           string  `json:"mode"`
	MaxFrequencyHz float64 `json:"max_frequency_hz"`
}

type Participant struct {
	ID            string            `json:"id"`
	Capability    string            `json:"capability"`
	Domain        string            `json:"domain"`
	RequiredPorts []ParticipantPort `json:"required_ports"`
	Constraints   []Constraint      `json:"constraints"`
}

type ParticipantPort struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Direction string    `json:"direction"`
	Protocol  *Protocol `json:"protocol,omitempty"`
}

type Objective struct {
	ID          string       `json:"id"`
	Capability  string       `json:"capability"`
	Bindings    []Binding    `json:"bindings"`
	Constraints []Constraint `json:"constraints"`
}

// Binding is a strict tagged union. Port identifies an external requirement
// port. Participant and ParticipantPort together identify an abstract
// participant port. Signal plus Direction identifies one endpoint of an
// internal behavior-level signal. Exactly one form must be present.
type Binding struct {
	Role            string `json:"role"`
	Port            string `json:"port,omitempty"`
	Signal          string `json:"signal,omitempty"`
	Direction       string `json:"direction,omitempty"`
	Participant     string `json:"participant,omitempty"`
	ParticipantPort string `json:"participant_port,omitempty"`
}

// Constraint is deliberately capability-neutral. Providers consume named
// constraints through a shared normalized relation/value/unit contract rather
// than defining topology-specific input schemas.
type Constraint struct {
	Name             string          `json:"name"`
	Relation         string          `json:"relation"`
	Value            json.RawMessage `json:"value"`
	Unit             string          `json:"unit,omitempty"`
	TolerancePercent *float64        `json:"tolerance_percent,omitempty"`
}

type BoardLimits struct {
	MaxComponents int     `json:"max_components"`
	MaxWidthMM    float64 `json:"max_width_mm"`
	MaxHeightMM   float64 `json:"max_height_mm"`
}

// OperatingCase declares bounded environmental and loading axes using only
// semantic requirement identities. It deliberately cannot name components,
// nets, models, or solver controls.
type OperatingCase struct {
	ID         string               `json:"id"`
	Conditions []OperatingCondition `json:"conditions"`
}

type OperatingCondition struct {
	Axis      string   `json:"axis"`
	Target    string   `json:"target"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Selection string   `json:"selection,omitempty"`
}

// BehavioralRequirement is a measurable, topology-neutral assertion. The
// observation is bound to resolved graph evidence only after architecture
// selection and lowering.
type BehavioralRequirement struct {
	ID             string      `json:"id"`
	Metric         string      `json:"metric"`
	Analysis       string      `json:"analysis"`
	Observation    Observation `json:"observation"`
	Min            *float64    `json:"min,omitempty"`
	Max            *float64    `json:"max,omitempty"`
	Unit           string      `json:"unit"`
	OperatingCases []string    `json:"operating_cases"`
	Critical       bool        `json:"critical,omitempty"`
}

type Observation struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Acceptance struct {
	RequireERC                 bool `json:"require_erc"`
	RequireStrictDRC           bool `json:"require_strict_drc"`
	RequireCompleteRouting     bool `json:"require_complete_routing"`
	RequireConnectivity        bool `json:"require_connectivity"`
	RequireWriterCorrectness   bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff   bool `json:"require_round_trip_zero_diff"`
	RequireDeterministicReplay bool `json:"require_deterministic_replay"`
	RequireContractComposition bool `json:"require_contract_composition,omitempty"`
	RequireGlobalReasoning     bool `json:"require_global_reasoning,omitempty"`
	RequireCoverageAccounting  bool `json:"require_coverage_accounting,omitempty"`
	RequireAlternatives        bool `json:"require_alternatives,omitempty"`
	RequireFailClosed          bool `json:"require_fail_closed,omitempty"`
	RequireSimulation          bool `json:"require_simulation,omitempty"`
	RequireAllCorners          bool `json:"require_all_corners,omitempty"`
	RequireModelProvenance     bool `json:"require_model_provenance,omitempty"`
	RequireClosedLoopEvidence  bool `json:"require_closed_loop_evidence,omitempty"`
}
