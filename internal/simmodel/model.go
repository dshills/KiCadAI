package simmodel

const (
	RegistryVersion = "kicadai.trusted-simulation-registry.v1"
	ReportSchema    = "kicadai.trusted-simulation-report.v2"

	ModelLinearRegulatorIdealV1 = "linear_regulator_ideal_v1"
	ModelResistorDividerDCV1    = "resistor_divider_dc_v1"
	ModelRCLowpassACV1          = "rc_lowpass_ac_v1"
	ModelLinearCircuitMNAV1     = "linear_circuit_mna_v1"
	ModelNonlinearCircuitDCV1   = "nonlinear_circuit_dc_v1"
	ModelTransientCircuitV1     = "transient_circuit_v1"

	PrimitiveResistorV1               = "mna_resistor_v1"
	PrimitiveCapacitorV1              = "mna_capacitor_v1"
	PrimitiveCapacitorTransientV1     = "mna_capacitor_transient_be_v1"
	PrimitiveVoltageSourceV1          = "mna_voltage_source_v1"
	PrimitiveConnectorVoltageSourceV1 = "mna_connector_1x02_voltage_source_pin1_positive_v1"
	PrimitiveCurrentSourceV1          = "mna_current_source_v1"
	PrimitiveOpAmpV1                  = "mna_opamp_single_pole_v1"
	PrimitiveDiodeShockleyV1          = "mna_diode_shockley_v1"
	PrimitiveBJTNPNV1                 = "mna_bjt_npn_ebers_moll_v1"
	PrimitiveBJTPNPV1                 = "mna_bjt_pnp_ebers_moll_v1"

	AnalysisDCOperatingPoint = "dc_operating_point"
	AnalysisACSweep          = "ac_sweep"
	AnalysisTransient        = "transient"

	QuantityVoltageV          = "voltage_v"
	QuantityVoltageMagnitudeV = "voltage_magnitude_v"
	QuantityVoltagePhaseDeg   = "voltage_phase_deg"
	QuantityVoltageDBV        = "voltage_dbv"
	QuantityRiseTimeS         = "rise_time_s"
	QuantityFallTimeS         = "fall_time_s"
)

type NamedValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// Uncertainty is immutable catalog-backed evidence for one bounded scalar in
// a resolved plan. Target is a canonical resolver-owned path; providers never
// provide expressions, sampling policy, or solver controls.
type Uncertainty struct {
	Target  string  `json:"target"`
	Source  string  `json:"source"`
	Nominal float64 `json:"nominal"`
	Minimum float64 `json:"minimum"`
	Maximum float64 `json:"maximum"`
}

type CatalogEvidence struct {
	ModelID       string        `json:"model_id"`
	Parameters    []NamedValue  `json:"parameters,omitempty"`
	Uncertainties []Uncertainty `json:"uncertainties,omitempty"`
}

type Binding struct {
	Role      string `json:"role"`
	Component string `json:"component"`
}

type Assertion struct {
	Metric      string  `json:"metric,omitempty"`
	AnalysisID  string  `json:"analysis_id,omitempty"`
	Node        string  `json:"node,omitempty"`
	Quantity    string  `json:"quantity,omitempty"`
	FrequencyHz float64 `json:"frequency_hz,omitempty"`
	TimeS       float64 `json:"time_s,omitempty"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
}

// SourceExcitation is a bounded operating condition for a catalog-resolved
// independent source. Primitive kind and terminal orientation remain trusted
// catalog/registry data rather than provider input.
type SourceExcitation struct {
	Component         string  `json:"component"`
	DCValue           float64 `json:"dc_value,omitempty"`
	ACMagnitude       float64 `json:"ac_magnitude,omitempty"`
	ACPhaseDeg        float64 `json:"ac_phase_deg,omitempty"`
	PulseInitialValue float64 `json:"pulse_initial_value,omitempty"`
	PulseValue        float64 `json:"pulse_value,omitempty"`
	PulseDelayS       float64 `json:"pulse_delay_s,omitempty"`
	PulseWidthS       float64 `json:"pulse_width_s,omitempty"`
	PulsePeriodS      float64 `json:"pulse_period_s,omitempty"`
}

// Analysis requests a trusted analysis algorithm. It contains no equation,
// matrix, expression, executable, include, path, or topology field.
type Analysis struct {
	ID               string             `json:"id"`
	Kind             string             `json:"kind"`
	StartFrequencyHz float64            `json:"start_frequency_hz,omitempty"`
	StopFrequencyHz  float64            `json:"stop_frequency_hz,omitempty"`
	Points           int                `json:"points,omitempty"`
	DurationS        float64            `json:"duration_s,omitempty"`
	TimeStepS        float64            `json:"time_step_s,omitempty"`
	Excitations      []SourceExcitation `json:"excitations"`
}

// Intent contains only trusted model selection, component bindings, bounded
// scalar operating conditions, and assertions. It deliberately has no model
// text, include path, command, expression, or executable field.
type Intent struct {
	ModelID    string       `json:"model_id"`
	Bindings   []Binding    `json:"bindings"`
	Inputs     []NamedValue `json:"inputs"`
	Analyses   []Analysis   `json:"analyses,omitempty"`
	Assertions []Assertion  `json:"assertions"`
	WorstCase  bool         `json:"worst_case,omitempty"`
}

type ConnectionEvidence struct {
	Function string
	UnitID   string
	Net      string
}

type ComponentEvidence struct {
	InstanceID        string
	PhysicalComponent string
	CatalogID         string
	Family            string
	ValueSI           float64
	HasValueSI        bool
	ModelClaims       []CatalogEvidence
	Connections       []ConnectionEvidence
	Uncertainties     []Uncertainty
}

type ResolvedBinding struct {
	Role            string       `json:"role"`
	Component       string       `json:"component"`
	CatalogID       string       `json:"catalog_id"`
	Family          string       `json:"family"`
	ValueSI         *float64     `json:"value_si,omitempty"`
	ModelParameters []NamedValue `json:"model_parameters,omitempty"`
}

type TerminalBinding struct {
	Terminal string `json:"terminal"`
	Net      string `json:"net"`
}

type ResolvedDevice struct {
	Component         string            `json:"component"`
	PhysicalComponent string            `json:"physical_component,omitempty"`
	CatalogID         string            `json:"catalog_id"`
	Family            string            `json:"family"`
	PrimitiveModel    string            `json:"primitive_model"`
	ValueSI           *float64          `json:"value_si,omitempty"`
	ModelParameters   []NamedValue      `json:"model_parameters,omitempty"`
	Terminals         []TerminalBinding `json:"terminals"`
}

type Plan struct {
	RegistryVersion string            `json:"registry_version"`
	RegistryHash    string            `json:"registry_hash"`
	CatalogID       string            `json:"catalog_id"`
	CatalogHash     string            `json:"catalog_hash"`
	ModelID         string            `json:"model_id"`
	Bindings        []ResolvedBinding `json:"bindings"`
	Inputs          []NamedValue      `json:"inputs"`
	GroundNode      string            `json:"ground_node,omitempty"`
	Nodes           []string          `json:"nodes,omitempty"`
	Devices         []ResolvedDevice  `json:"devices,omitempty"`
	TopologyHash    string            `json:"topology_hash,omitempty"`
	Analyses        []Analysis        `json:"analyses,omitempty"`
	Assertions      []Assertion       `json:"assertions"`
	Uncertainties   []Uncertainty     `json:"uncertainties,omitempty"`
	WorstCase       bool              `json:"worst_case,omitempty"`
}

func ClonePlan(source Plan) Plan {
	clone := source
	clone.Bindings = append([]ResolvedBinding(nil), source.Bindings...)
	for index := range clone.Bindings {
		clone.Bindings[index].ModelParameters = append([]NamedValue(nil), source.Bindings[index].ModelParameters...)
		if source.Bindings[index].ValueSI != nil {
			value := *source.Bindings[index].ValueSI
			clone.Bindings[index].ValueSI = &value
		}
	}
	clone.Inputs = append([]NamedValue(nil), source.Inputs...)
	clone.Nodes = append([]string(nil), source.Nodes...)
	clone.Devices = append([]ResolvedDevice(nil), source.Devices...)
	for index := range clone.Devices {
		clone.Devices[index].ModelParameters = append([]NamedValue(nil), source.Devices[index].ModelParameters...)
		clone.Devices[index].Terminals = append([]TerminalBinding(nil), source.Devices[index].Terminals...)
		if source.Devices[index].ValueSI != nil {
			value := *source.Devices[index].ValueSI
			clone.Devices[index].ValueSI = &value
		}
	}
	clone.Analyses = cloneAnalyses(source.Analyses)
	clone.Assertions = append([]Assertion(nil), source.Assertions...)
	clone.Uncertainties = append([]Uncertainty(nil), source.Uncertainties...)
	return clone
}

func cloneAnalyses(source []Analysis) []Analysis {
	clone := append([]Analysis(nil), source...)
	for index := range clone {
		clone[index].Excitations = append([]SourceExcitation(nil), source[index].Excitations...)
	}
	return clone
}

type Diagnostic struct {
	Path       string `json:"path"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type Measurement struct {
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
}

type AssertionResult struct {
	Metric      string  `json:"metric,omitempty"`
	AnalysisID  string  `json:"analysis_id,omitempty"`
	Node        string  `json:"node,omitempty"`
	Quantity    string  `json:"quantity,omitempty"`
	FrequencyHz float64 `json:"frequency_hz,omitempty"`
	TimeS       float64 `json:"time_s,omitempty"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Actual      float64 `json:"actual"`
	Pass        bool    `json:"pass"`
}

type NodeResult struct {
	Node      string  `json:"node"`
	Real      float64 `json:"real"`
	Imaginary float64 `json:"imaginary"`
	Magnitude float64 `json:"magnitude"`
	PhaseDeg  float64 `json:"phase_deg"`
}

type AnalysisPoint struct {
	FrequencyHz float64         `json:"frequency_hz,omitempty"`
	TimeS       float64         `json:"time_s,omitempty"`
	Nodes       []NodeResult    `json:"nodes"`
	Solver      *SolverEvidence `json:"solver,omitempty"`
}

// SolverEvidence records bounded deterministic nonlinear work without
// exposing or accepting solver controls in provider-authored intent.
type SolverEvidence struct {
	Method                 string  `json:"method"`
	Iterations             int     `json:"iterations"`
	SourceStages           int     `json:"source_stages"`
	FinalMaxUpdateV        float64 `json:"final_max_update_v"`
	FinalMaxCurrentUpdateA float64 `json:"final_max_current_update_a,omitempty"`
	FinalMaxResidual       float64 `json:"final_max_residual"`
	InitialCondition       string  `json:"initial_condition,omitempty"`
	TimeSteps              int     `json:"time_steps,omitempty"`
	TotalIterations        int     `json:"total_iterations,omitempty"`
	MaxIterationsPerStep   int     `json:"max_iterations_per_step,omitempty"`
	MaxTotalIterations     int     `json:"max_total_iterations,omitempty"`
}

type AnalysisResult struct {
	ID     string          `json:"id"`
	Kind   string          `json:"kind"`
	Points []AnalysisPoint `json:"points"`
}

type CornerResult struct {
	ID          string            `json:"id"`
	Assignments []NamedValue      `json:"assignments"`
	Assertions  []AssertionResult `json:"assertions"`
	Status      string            `json:"status"`
}

type SensitivityResult struct {
	Assertion string  `json:"assertion"`
	Target    string  `json:"target"`
	Corner    string  `json:"corner"`
	Margin    float64 `json:"margin"`
}

type Report struct {
	Schema          string              `json:"schema"`
	RegistryVersion string              `json:"registry_version"`
	RegistryHash    string              `json:"registry_hash"`
	CatalogID       string              `json:"catalog_id"`
	CatalogHash     string              `json:"catalog_hash"`
	ModelID         string              `json:"model_id"`
	Bindings        []ResolvedBinding   `json:"bindings"`
	Inputs          []NamedValue        `json:"inputs"`
	GroundNode      string              `json:"ground_node,omitempty"`
	Nodes           []string            `json:"nodes,omitempty"`
	Devices         []ResolvedDevice    `json:"devices,omitempty"`
	TopologyHash    string              `json:"topology_hash,omitempty"`
	Analyses        []AnalysisResult    `json:"analyses,omitempty"`
	Measurements    []Measurement       `json:"measurements"`
	Assertions      []AssertionResult   `json:"assertions"`
	Corners         []CornerResult      `json:"corners,omitempty"`
	Sensitivity     []SensitivityResult `json:"sensitivity,omitempty"`
	Status          string              `json:"status"`
}
