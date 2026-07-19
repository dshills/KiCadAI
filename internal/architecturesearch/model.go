// Package architecturesearch defines behavioral electrical requirements and
// deterministic bounded circuit-architecture search.
package architecturesearch

import "encoding/json"

const (
	SchemaID      = "kicadai.open-set-requirement.v1"
	Version       = 1
	PolicyVersion = "architecture-search-policy-v1"

	MaxRequirementBytes = 256 * 1024
	MaxDomains          = 16
	MaxPorts            = 64
	MaxParticipants     = 16
	MaxParticipantPorts = 32
	MaxObjectives       = 32
	MaxBindings         = 64
	MaxConstraints      = 64
	MaxComponents       = 64
	MaxBoardDimensionMM = 200.0
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
	Domains      []Domain      `json:"domains"`
	Ports        []Port        `json:"ports"`
	Participants []Participant `json:"participants,omitempty"`
	Objectives   []Objective   `json:"objectives"`
	Constraints  BoardLimits   `json:"constraints"`
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
// participant port. Exactly one form must be present.
type Binding struct {
	Role            string `json:"role"`
	Port            string `json:"port,omitempty"`
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

type Acceptance struct {
	RequireERC                 bool `json:"require_erc"`
	RequireStrictDRC           bool `json:"require_strict_drc"`
	RequireCompleteRouting     bool `json:"require_complete_routing"`
	RequireConnectivity        bool `json:"require_connectivity"`
	RequireWriterCorrectness   bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff   bool `json:"require_round_trip_zero_diff"`
	RequireDeterministicReplay bool `json:"require_deterministic_replay"`
}
