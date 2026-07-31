package opentopologysynthesis

import (
	"encoding/json"

	"kicadai/internal/reports"
)

const (
	RequirementSchema  = "kicadai.open-topology-requirement.v1"
	RequirementVersion = 1
	ReportSchema       = "kicadai.open-topology-synthesis-report.v1"
	ReportVersion      = 1
	PolicyVersion      = "open-topology-policy-v1"

	MaxRequirementBytes = 256 * 1024
	MaxDomains          = 16
	MaxPorts            = 24
	MaxOperatingCases   = 24
	MaxConditions       = 16
	MaxEvents           = 16
	MaxAssertions       = 64
	MaxComponents       = 32
	MaxBoardDimensionMM = 500
	MaxTextBytes        = 4096
)

const (
	CodeRequirementInvalid      reports.Code = "OPEN_TOPOLOGY_REQUIREMENT_INVALID"
	CodePrimitiveUnavailable    reports.Code = "OPEN_TOPOLOGY_PRIMITIVE_UNAVAILABLE"
	CodeModelUnavailable        reports.Code = "OPEN_TOPOLOGY_MODEL_UNAVAILABLE"
	CodeSearchExhausted         reports.Code = "OPEN_TOPOLOGY_SEARCH_EXHAUSTED"
	CodeNoCompleteGraph         reports.Code = "OPEN_TOPOLOGY_NO_COMPLETE_GRAPH"
	CodeNoPassingGraph          reports.Code = "OPEN_TOPOLOGY_NO_PASSING_GRAPH"
	CodeValueExhausted          reports.Code = "OPEN_TOPOLOGY_VALUE_EXHAUSTED"
	CodeRepairUnsupported       reports.Code = "OPEN_TOPOLOGY_REPAIR_UNSUPPORTED"
	CodeRepairExhausted         reports.Code = "OPEN_TOPOLOGY_REPAIR_EXHAUSTED"
	CodeCanceled                reports.Code = "OPEN_TOPOLOGY_CANCELED"
	CodePhysicalPromotionFailed reports.Code = "OPEN_TOPOLOGY_PHYSICAL_PROMOTION_FAILED"
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
	Domains                []Domain              `json:"domains"`
	Ports                  []Port                `json:"ports"`
	OperatingCases         []OperatingCase       `json:"operating_cases"`
	BehavioralRequirements []BehavioralAssertion `json:"behavioral_requirements"`
	Constraints            BoardLimits           `json:"constraints"`
}

type Domain struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	MinVoltageV     *float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV *float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV     *float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA     *float64 `json:"max_current_a,omitempty"`
	Source          string   `json:"source"`
}

type Port struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Direction  string     `json:"direction"`
	Domain     string     `json:"domain"`
	Electrical Electrical `json:"electrical,omitempty"`
}

type Electrical struct {
	MinVoltageV          *float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV      *float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV          *float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA          *float64 `json:"max_current_a,omitempty"`
	InputImpedanceMinOhm *float64 `json:"input_impedance_min_ohm,omitempty"`
	DefaultState         string   `json:"default_state,omitempty"`
}

type OperatingCase struct {
	ID         string               `json:"id"`
	Conditions []OperatingCondition `json:"conditions"`
	Events     []OperatingEvent     `json:"events,omitempty"`
}

type OperatingCondition struct {
	Axis   string  `json:"axis"`
	Target string  `json:"target"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Unit   string  `json:"unit"`
}

type OperatingEvent struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Target       string  `json:"target"`
	TriggerTimeS float64 `json:"trigger_time_s"`
	Initial      float64 `json:"initial"`
	Applied      float64 `json:"applied"`
	Unit         string  `json:"unit"`
}

type BehavioralAssertion struct {
	ID             string       `json:"id"`
	Metric         string       `json:"metric"`
	Analysis       string       `json:"analysis"`
	Excitation     *Observation `json:"excitation,omitempty"`
	Observation    Observation  `json:"observation"`
	Min            *float64     `json:"min,omitempty"`
	Max            *float64     `json:"max,omitempty"`
	Unit           string       `json:"unit"`
	FrequencyHz    *float64     `json:"frequency_hz,omitempty"`
	OperatingCases []string     `json:"operating_cases"`
	Critical       bool         `json:"critical,omitempty"`
}

type Observation struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type BoardLimits struct {
	MaxComponents int     `json:"max_components"`
	MaxWidthMM    float64 `json:"max_width_mm"`
	MaxHeightMM   float64 `json:"max_height_mm"`
}

type Acceptance struct {
	RequirePrimitiveOnly       bool `json:"require_primitive_only"`
	RequireTopologySearch      bool `json:"require_topology_search"`
	RequireSimulation          bool `json:"require_simulation"`
	RequireAllCorners          bool `json:"require_all_corners"`
	RequireModelProvenance     bool `json:"require_model_provenance"`
	RequireClosedLoopEvidence  bool `json:"require_closed_loop_evidence"`
	RequireCompleteRouting     bool `json:"require_complete_routing"`
	RequireConnectivity        bool `json:"require_connectivity"`
	RequireWriterCorrectness   bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff   bool `json:"require_round_trip_zero_diff"`
	RequireERC                 bool `json:"require_erc"`
	RequireStrictDRC           bool `json:"require_strict_drc"`
	RequireDeterministicReplay bool `json:"require_deterministic_replay"`
	RequireFailClosed          bool `json:"require_fail_closed"`
}

type Policy struct {
	MaxExpandedStates       int `json:"max_expanded_states"`
	MaxGeneratedGraphs      int `json:"max_generated_graphs"`
	MaxPrimitiveInstances   int `json:"max_primitive_instances"`
	MaxInternalNodes        int `json:"max_internal_nodes"`
	MaxCandidateSimulations int `json:"max_candidate_simulations"`
	MaxCornerEvaluations    int `json:"max_corner_evaluations"`
	MaxValueTrials          int `json:"max_value_trials"`
	MaxTopologyRepairs      int `json:"max_topology_repairs"`
	MaxRetainedCandidates   int `json:"max_retained_candidates"`
	MaxDiagnosticSamples    int `json:"max_diagnostic_samples"`
}

func DefaultPolicy() Policy {
	return Policy{
		MaxExpandedStates:       20_000,
		MaxGeneratedGraphs:      50_000,
		MaxPrimitiveInstances:   20,
		MaxInternalNodes:        24,
		MaxCandidateSimulations: 512,
		MaxCornerEvaluations:    4_096,
		MaxValueTrials:          4_096,
		MaxTopologyRepairs:      128,
		MaxRetainedCandidates:   16,
		MaxDiagnosticSamples:    32,
	}
}

type Status string

const (
	StatusPassed      Status = "passed"
	StatusFailed      Status = "failed"
	StatusExhausted   Status = "exhausted"
	StatusCanceled    Status = "canceled"
	StatusInvalid     Status = "invalid"
	StatusUnsupported Status = "unsupported"
)

type StopReason string

const (
	StopPassed                  StopReason = "passed"
	StopRequirementInvalid      StopReason = "requirement_invalid"
	StopPrimitiveUnavailable    StopReason = "primitive_unavailable"
	StopModelUnavailable        StopReason = "model_unavailable"
	StopSearchExhausted         StopReason = "search_exhausted"
	StopNoCompleteGraph         StopReason = "no_complete_graph"
	StopNoPassingGraph          StopReason = "no_passing_graph"
	StopValueExhausted          StopReason = "value_exhausted"
	StopRepairUnsupported       StopReason = "repair_unsupported"
	StopRepairExhausted         StopReason = "repair_exhausted"
	StopCanceled                StopReason = "canceled"
	StopPhysicalPromotionFailed StopReason = "physical_promotion_failed"
)

type Report struct {
	Schema                 string            `json:"schema"`
	Version                int               `json:"version"`
	PolicyVersion          string            `json:"policy_version"`
	PolicyHash             string            `json:"policy_hash"`
	RequirementHash        string            `json:"requirement_hash"`
	PrimitiveInventoryHash string            `json:"primitive_inventory_hash,omitempty"`
	CatalogHash            string            `json:"catalog_hash,omitempty"`
	ModelRegistryHash      string            `json:"model_registry_hash,omitempty"`
	Policy                 Policy            `json:"policy"`
	Status                 Status            `json:"status"`
	StopReason             StopReason        `json:"stop_reason"`
	Consumption            Consumption       `json:"consumption"`
	Candidates             []CandidateReport `json:"candidates"`
	Selected               *SelectedResult   `json:"selected,omitempty"`
	Diagnostics            []Diagnostic      `json:"diagnostics"`
}

type Consumption struct {
	ExpandedStates       int  `json:"expanded_states"`
	GeneratedGraphs      int  `json:"generated_graphs"`
	CompleteGraphs       int  `json:"complete_graphs"`
	CandidateSimulations int  `json:"candidate_simulations"`
	CornerEvaluations    int  `json:"corner_evaluations"`
	ValueTrials          int  `json:"value_trials"`
	TopologyRepairs      int  `json:"topology_repairs"`
	MaximumFrontier      int  `json:"maximum_frontier"`
	BudgetExhausted      bool `json:"budget_exhausted"`
}

type CandidateReport struct {
	Fingerprint    string    `json:"fingerprint"`
	TopologyHash   string    `json:"topology_hash"`
	ComponentCount int       `json:"component_count"`
	InternalNodes  int       `json:"internal_nodes"`
	Status         Status    `json:"status"`
	Attempts       []Attempt `json:"attempts"`
}

type Attempt struct {
	Number         int         `json:"number"`
	GraphHash      string      `json:"graph_hash"`
	ValueHash      string      `json:"value_hash"`
	EvaluationHash string      `json:"evaluation_hash,omitempty"`
	Diagnoses      []Diagnosis `json:"diagnoses"`
	Repair         *Repair     `json:"repair,omitempty"`
	Status         Status      `json:"status"`
}

type Diagnosis struct {
	Code             string   `json:"code"`
	RequirementID    string   `json:"requirement_id"`
	OperatingCase    string   `json:"operating_case"`
	Analysis         string   `json:"analysis"`
	Metric           string   `json:"metric"`
	Direction        string   `json:"direction"`
	Actual           *float64 `json:"actual,omitempty"`
	RequiredMin      *float64 `json:"required_min,omitempty"`
	RequiredMax      *float64 `json:"required_max,omitempty"`
	AffectedConeHash string   `json:"affected_cone_hash,omitempty"`
	EvidenceHash     string   `json:"evidence_hash,omitempty"`
	Message          string   `json:"message"`
}

type Repair struct {
	Number            int           `json:"number"`
	Operator          string        `json:"operator"`
	DiagnosisCode     string        `json:"diagnosis_code"`
	BeforeGraphHash   string        `json:"before_graph_hash"`
	AfterGraphHash    string        `json:"after_graph_hash"`
	ExpectedDirection string        `json:"expected_direction"`
	Changes           []GraphChange `json:"changes"`
}

type GraphChange struct {
	Kind      string   `json:"kind"`
	Primitive string   `json:"primitive,omitempty"`
	Terminal  string   `json:"terminal,omitempty"`
	FromNode  string   `json:"from_node,omitempty"`
	ToNode    string   `json:"to_node,omitempty"`
	FromValue *float64 `json:"from_value,omitempty"`
	ToValue   *float64 `json:"to_value,omitempty"`
}

type SelectedResult struct {
	Fingerprint      string `json:"fingerprint"`
	TopologyHash     string `json:"topology_hash"`
	EvaluationHash   string `json:"evaluation_hash"`
	PhysicalHash     string `json:"physical_hash,omitempty"`
	SelectionSummary string `json:"selection_summary"`
}

type Diagnostic struct {
	Code       reports.Code `json:"code"`
	Path       string       `json:"path"`
	Message    string       `json:"message"`
	Suggestion string       `json:"suggestion,omitempty"`
}

type ReplayEnvelope struct {
	Requirement json.RawMessage `json:"requirement"`
	Report      Report          `json:"report"`
}
