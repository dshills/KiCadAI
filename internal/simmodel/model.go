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

	PrimitiveResistorV1                         = "mna_resistor_v1"
	PrimitiveFuseClosedStateV1                  = "mna_fuse_closed_state_v1"
	PrimitiveCapacitorV1                        = "mna_capacitor_v1"
	PrimitiveCapacitorTransientV1               = "mna_capacitor_transient_be_v1"
	PrimitiveVoltageSourceV1                    = "mna_voltage_source_v1"
	PrimitiveConnectorVoltageSourceV1           = "mna_connector_1x02_voltage_source_pin1_positive_v1"
	PrimitiveCurrentSourceV1                    = "mna_current_source_v1"
	PrimitiveMCUStaticSupplyLoadV1              = "mna_mcu_static_supply_load_v1"
	PrimitiveSensorStaticSupplyLoadV1           = "mna_sensor_static_supply_load_v1"
	PrimitiveOpAmpV1                            = "mna_opamp_single_pole_v1"
	PrimitiveComparatorOpenCollectorV1          = "mna_comparator_open_collector_v1"
	PrimitiveCurrentSenseAmplifierV1            = "mna_current_sense_amplifier_single_pole_v1"
	PrimitiveAdjustableLinearRegulatorV1        = "mna_adjustable_linear_regulator_v1"
	PrimitiveFixedLinearRegulatorV1             = "mna_fixed_linear_regulator_v1"
	PrimitiveFloatingAdjustableRegulatorV1      = "mna_floating_adjustable_regulator_v1"
	PrimitiveProgrammableCurrentSourceV1        = "mna_programmable_current_source_v1"
	PrimitiveShuntVoltageReferenceV1            = "mna_shunt_voltage_reference_v1"
	PrimitiveDualOutputIsolatedConverterV1      = "mna_dual_output_isolated_converter_v1"
	PrimitiveBidirectionalOpenDrainTranslatorV1 = "mna_bidirectional_open_drain_translator_v1"
	PrimitiveBidirectionalTVSV1                 = "mna_bidirectional_tvs_piecewise_linear_v1"
	PrimitiveUnidirectionalZenerV1              = "mna_unidirectional_zener_spice_v1"
	PrimitiveDiodeShockleyV1                    = "mna_diode_shockley_v1"
	PrimitiveNMOSSwitchV1                       = "mna_nmos_guaranteed_switch_v1"
	PrimitivePMOSSwitchV1                       = "mna_pmos_guaranteed_switch_v1"
	PrimitiveBJTNPNV1                           = "mna_bjt_npn_ebers_moll_v1"
	PrimitiveBJTPNPV1                           = "mna_bjt_pnp_ebers_moll_v1"
	PrimitiveRelayClosedV1                      = "mna_relay_closed_state_v1"
	PrimitiveRelayNormallyOpenV1                = "mna_relay_normally_open_v1"

	AnalysisDCOperatingPoint = "dc_operating_point"
	AnalysisACSweep          = "ac_sweep"
	AnalysisTransient        = "transient"
	AnalysisNoise            = "noise"
	AnalysisStability        = "stability"
	AnalysisStartup          = "startup"
	AnalysisDistortion       = "distortion"
	AnalysisThermal          = "thermal"

	QuantityVoltageV             = "voltage_v"
	QuantityVoltageMagnitudeV    = "voltage_magnitude_v"
	QuantityVoltagePhaseDeg      = "voltage_phase_deg"
	QuantityVoltageDBV           = "voltage_dbv"
	QuantityRiseTimeS            = "rise_time_s"
	QuantityFallTimeS            = "fall_time_s"
	QuantityIntegratedNoiseVRMS  = "integrated_noise_v_rms"
	QuantityPhaseMarginDeg       = "phase_margin_deg"
	QuantityGainMarginDB         = "gain_margin_db"
	QuantityPeakAbsVoltageV      = "peak_abs_voltage_v"
	QuantityTHDPercent           = "thd_percent"
	QuantityDeviceDissipationW   = "device_dissipation_w"
	QuantityJunctionTemperatureC = "junction_temperature_c"
	QuantityVoltageGainRatio     = "voltage_gain_ratio"
	QuantityCutoffFrequencyHz    = "cutoff_frequency_hz"
	QuantityBandwidthHz          = "bandwidth_hz"
	QuantityOutputSwingVPP       = "output_swing_v_pp"
	QuantitySettlingTimeS        = "settling_time_s"
	QuantityResponseTimeS        = "response_time_s"
	QuantityDeviceCurrentA       = "device_current_a"
	QuantityTotalSupplyCurrentA  = "total_supply_current_a"
	QuantityTransimpedanceOhm    = "transimpedance_ohm"
	QuantityOutputPowerW         = "output_power_w"
	QuantityThresholdVoltageV    = "threshold_voltage_v"
	QuantityThresholdCurrentA    = "threshold_current_a"
	QuantityHysteresisVoltageV   = "hysteresis_voltage_v"
)

const parameterForcedMOSFETState = "__forced_mosfet_state"

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

// ModelProvenance is catalog-owned trust evidence. It is never accepted from
// provider-authored simulation intent.
type ModelProvenance struct {
	Source          string   `json:"source"`
	Revision        string   `json:"revision"`
	SHA256          string   `json:"sha256"`
	ReviewStatus    string   `json:"review_status"`
	AllowedAnalyses []string `json:"allowed_analyses"`
	MinTemperatureC *float64 `json:"min_temperature_c,omitempty"`
	MaxTemperatureC *float64 `json:"max_temperature_c,omitempty"`
}

type Binding struct {
	Role      string `json:"role"`
	Component string `json:"component"`
}

type Assertion struct {
	Metric        string   `json:"metric,omitempty"`
	AnalysisID    string   `json:"analysis_id,omitempty"`
	Node          string   `json:"node,omitempty"`
	Component     string   `json:"component,omitempty"`
	Components    []string `json:"components,omitempty"`
	ReferenceNode string   `json:"reference_node,omitempty"`
	Quantity      string   `json:"quantity,omitempty"`
	FrequencyHz   float64  `json:"frequency_hz,omitempty"`
	TimeS         float64  `json:"time_s,omitempty"`
	Min           float64  `json:"min"`
	Max           float64  `json:"max"`
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
	SineAmplitude     float64 `json:"sine_amplitude,omitempty"`
	SineFrequencyHz   float64 `json:"sine_frequency_hz,omitempty"`
	SinePhaseDeg      float64 `json:"sine_phase_deg,omitempty"`
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
	Conditions       []NamedValue       `json:"conditions,omitempty"`
	DeviceOverrides  []DeviceOverride   `json:"device_overrides,omitempty"`
	DCSweep          *DCSweep           `json:"dc_sweep,omitempty"`
}

// DCSweep requests a bounded deterministic sweep of one already resolved
// independent source. Bidirectional sweeps preserve the converged comparator
// state between adjacent points so threshold and hysteresis are measured from
// the circuit equations rather than inferred from a provider formula.
type DCSweep struct {
	Component       string  `json:"component"`
	StartValue      float64 `json:"start_value"`
	StopValue       float64 `json:"stop_value"`
	Points          int     `json:"points"`
	Bidirectional   bool    `json:"bidirectional,omitempty"`
	ExcitationScale float64 `json:"excitation_scale,omitempty"`
}

// DeviceOverride applies bounded scalar corner values to an already resolved
// catalog device for one analysis. It cannot change identity, primitive kind,
// terminals, or topology.
type DeviceOverride struct {
	Component       string       `json:"component"`
	ValueSI         *float64     `json:"value_si,omitempty"`
	ModelParameters []NamedValue `json:"model_parameters,omitempty"`
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
	for index := range clone.Assertions {
		clone.Assertions[index].Components = append([]string(nil), source.Assertions[index].Components...)
	}
	clone.Uncertainties = append([]Uncertainty(nil), source.Uncertainties...)
	return clone
}

func cloneAnalyses(source []Analysis) []Analysis {
	clone := append([]Analysis(nil), source...)
	for index := range clone {
		clone[index].Excitations = append([]SourceExcitation(nil), source[index].Excitations...)
		clone[index].Conditions = append([]NamedValue(nil), source[index].Conditions...)
		clone[index].DeviceOverrides = append([]DeviceOverride(nil), source[index].DeviceOverrides...)
		if source[index].DCSweep != nil {
			sweep := *source[index].DCSweep
			clone[index].DCSweep = &sweep
		}
		for overrideIndex := range clone[index].DeviceOverrides {
			clone[index].DeviceOverrides[overrideIndex].ModelParameters = append([]NamedValue(nil), source[index].DeviceOverrides[overrideIndex].ModelParameters...)
			if source[index].DeviceOverrides[overrideIndex].ValueSI != nil {
				value := *source[index].DeviceOverrides[overrideIndex].ValueSI
				clone[index].DeviceOverrides[overrideIndex].ValueSI = &value
			}
		}
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
	Metric        string   `json:"metric,omitempty"`
	AnalysisID    string   `json:"analysis_id,omitempty"`
	Node          string   `json:"node,omitempty"`
	Component     string   `json:"component,omitempty"`
	Components    []string `json:"components,omitempty"`
	ReferenceNode string   `json:"reference_node,omitempty"`
	Quantity      string   `json:"quantity,omitempty"`
	FrequencyHz   float64  `json:"frequency_hz,omitempty"`
	TimeS         float64  `json:"time_s,omitempty"`
	Min           float64  `json:"min"`
	Max           float64  `json:"max"`
	Actual        float64  `json:"actual"`
	Pass          bool     `json:"pass"`
}

type NodeResult struct {
	Node                        string  `json:"node"`
	Real                        float64 `json:"real"`
	Imaginary                   float64 `json:"imaginary"`
	Magnitude                   float64 `json:"magnitude"`
	PhaseDeg                    float64 `json:"phase_deg"`
	DominantNoiseSource         string  `json:"dominant_noise_source,omitempty"`
	DominantNoiseDensityVSqrtHz float64 `json:"dominant_noise_density_v_sqrt_hz,omitempty"`
}

type AnalysisPoint struct {
	FrequencyHz float64         `json:"frequency_hz,omitempty"`
	TimeS       float64         `json:"time_s,omitempty"`
	SweepValue  float64         `json:"sweep_value,omitempty"`
	Sweep       string          `json:"sweep,omitempty"`
	Nodes       []NodeResult    `json:"nodes"`
	Devices     []DeviceResult  `json:"devices,omitempty"`
	Solver      *SolverEvidence `json:"solver,omitempty"`
}

type DeviceResult struct {
	Component            string   `json:"component"`
	VoltageV             float64  `json:"voltage_v,omitempty"`
	CurrentA             float64  `json:"current_a,omitempty"`
	CurrentMagnitudeA    float64  `json:"current_magnitude_a,omitempty"`
	DissipationW         float64  `json:"dissipation_w"`
	JunctionTemperatureC *float64 `json:"junction_temperature_c,omitempty"`
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
	ID                     string          `json:"id"`
	Kind                   string          `json:"kind"`
	FundamentalFrequencyHz float64         `json:"fundamental_frequency_hz,omitempty"`
	Points                 []AnalysisPoint `json:"points"`
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
