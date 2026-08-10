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
	PrimitiveFuseI2TClearingV1                  = "mna_fuse_i2t_clearing_v1"
	PrimitiveCapacitorV1                        = "mna_capacitor_v1"
	PrimitiveCapacitorTransientV1               = "mna_capacitor_transient_be_v1"
	PrimitiveInductorTransientV1                = "mna_inductor_transient_be_v1"
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
	PrimitiveFixedBuckModuleV1                  = "mna_fixed_step_down_module_v1"
	PrimitiveSynchronousBuckRegulatorV1         = "mna_synchronous_buck_current_mode_v1"
	PrimitiveFloatingAdjustableRegulatorV1      = "mna_floating_adjustable_regulator_v1"
	PrimitiveProgrammableCurrentSourceV1        = "mna_programmable_current_source_v1"
	PrimitiveShuntVoltageReferenceV1            = "mna_shunt_voltage_reference_v1"
	PrimitiveSingleOutputIsolatedConverterV1    = "mna_single_output_isolated_converter_v1"
	PrimitiveProtectedIsolatedConverterV1       = "mna_protected_isolated_converter_v1"
	PrimitiveDualOutputIsolatedConverterV1      = "mna_dual_output_isolated_converter_v1"
	PrimitiveBidirectionalOpenDrainTranslatorV1 = "mna_bidirectional_open_drain_translator_v1"
	PrimitivePushPullTranslatorV1               = "mna_push_pull_translator_v1"
	PrimitiveDirectionControlledTranslatorV1    = "mna_direction_controlled_translator_v1"
	PrimitiveBidirectionalOpenDrainIsolatorV1   = "mna_bidirectional_open_drain_isolator_v1"
	PrimitivePushPullDigitalIsolatorV1          = "mna_push_pull_digital_isolator_v1"
	PrimitiveReverseBlockingLoadSwitchV1        = "mna_reverse_blocking_load_switch_v1"
	PrimitiveCurrentLimitingEFuseV1             = "mna_current_limiting_efuse_v1"
	PrimitiveBidirectionalTVSV1                 = "mna_bidirectional_tvs_piecewise_linear_v1"
	PrimitiveUnidirectionalZenerV1              = "mna_unidirectional_zener_spice_v1"
	PrimitiveDiodeShockleyV1                    = "mna_diode_shockley_v1"
	PrimitiveNMOSSwitchV1                       = "mna_nmos_guaranteed_switch_v1"
	PrimitivePMOSSwitchV1                       = "mna_pmos_guaranteed_switch_v1"
	PrimitiveBJTNPNV1                           = "mna_bjt_npn_ebers_moll_v1"
	PrimitiveBJTPNPV1                           = "mna_bjt_pnp_ebers_moll_v1"
	PrimitiveRelayClosedV1                      = "mna_relay_closed_state_v1"
	PrimitiveRelayNormallyOpenV1                = "mna_relay_normally_open_v1"
	PrimitiveFixedClockSourceV1                 = "mna_fixed_clock_source_v1"
	PrimitiveResistorProgrammedClockSourceV1    = "mna_resistor_programmed_clock_source_v1"
	PrimitiveCMOSBufferV1                       = "mna_cmos_schmitt_buffer_v1"

	AnalysisDCOperatingPoint = "dc_operating_point"
	AnalysisACSweep          = "ac_sweep"
	AnalysisTransient        = "transient"
	AnalysisNoise            = "noise"
	AnalysisStability        = "stability"
	AnalysisStartup          = "startup"
	AnalysisDistortion       = "distortion"
	AnalysisThermal          = "thermal"
	AnalysisElectrothermal   = "electrothermal"

	QuantityVoltageV                    = "voltage_v"
	QuantityVoltageMagnitudeV           = "voltage_magnitude_v"
	QuantityVoltagePhaseDeg             = "voltage_phase_deg"
	QuantityVoltageDBV                  = "voltage_dbv"
	QuantityRiseTimeS                   = "rise_time_s"
	QuantityFallTimeS                   = "fall_time_s"
	QuantityIntegratedNoiseVRMS         = "integrated_noise_v_rms"
	QuantityPhaseMarginDeg              = "phase_margin_deg"
	QuantityGainMarginDB                = "gain_margin_db"
	QuantityLoopCrossoverHz             = "loop_crossover_frequency_hz"
	QuantityClosedLoopPeakingDB         = "closed_loop_peaking_db"
	QuantityPeakAbsVoltageV             = "peak_abs_voltage_v"
	QuantityPeakAbsDeviceVoltageV       = "peak_abs_device_voltage_v"
	QuantityPeakAbsDeviceCurrentA       = "peak_abs_device_current_a"
	QuantityFinalAbsDeviceCurrentA      = "final_abs_device_current_a"
	QuantityOvershootVoltageV           = "overshoot_voltage_v"
	QuantityOscillationFrequencyHz      = "oscillation_frequency_hz"
	QuantityDutyCyclePct                = "duty_cycle_percent"
	QuantityOutputRippleVPP             = "output_ripple_v_pp"
	QuantityConversionEfficiencyPct     = "conversion_efficiency_percent"
	QuantityTHDPercent                  = "thd_percent"
	QuantityDeviceDissipationW          = "device_dissipation_w"
	QuantityJunctionTemperatureC        = "junction_temperature_c"
	QuantityTransientSOAMargin          = "transient_soa_margin_ratio"
	QuantityMaximumJunctionTemperatureC = "maximum_junction_temperature_c"
	QuantityMinimumTransientSOAMargin   = "minimum_transient_soa_margin_ratio"
	QuantityVoltageGainRatio            = "voltage_gain_ratio"
	QuantityCutoffFrequencyHz           = "cutoff_frequency_hz"
	QuantityBandwidthHz                 = "bandwidth_hz"
	QuantityOutputSwingVPP              = "output_swing_v_pp"
	QuantitySettlingTimeS               = "settling_time_s"
	QuantityResponseTimeS               = "response_time_s"
	QuantityDeviceCurrentA              = "device_current_a"
	QuantityTotalSupplyCurrentA         = "total_supply_current_a"
	QuantityTransimpedanceOhm           = "transimpedance_ohm"
	QuantityInputImpedanceOhm           = "input_impedance_ohm"
	QuantityOutputPowerW                = "output_power_w"
	QuantityThresholdVoltageV           = "threshold_voltage_v"
	QuantityThresholdCurrentA           = "threshold_current_a"
	QuantityHysteresisVoltageV          = "hysteresis_voltage_v"
	QuantityRisingThresholdVoltageV     = "rising_threshold_voltage_v"
	QuantityFallingThresholdVoltageV    = "falling_threshold_voltage_v"
	QuantityLowerThresholdVoltageV      = "lower_threshold_voltage_v"
	QuantityUpperThresholdVoltageV      = "upper_threshold_voltage_v"
	QuantityDCSweepVoltageSpanV         = "dc_sweep_voltage_span_v"
	QuantityDCSweepDeviceCurrentSpanA   = "dc_sweep_device_current_span_a"
	QuantityDCSweepVoltageSlopeVPerV    = "dc_sweep_voltage_slope_v_per_v"
	QuantityDCSweepDeviceSlopeAperV     = "dc_sweep_device_slope_a_per_v"
)

const parameterForcedMOSFETState = "__forced_mosfet_state"
const parameterForcedBJTState = "__forced_bjt_state"

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
	ModelID       string                 `json:"model_id"`
	Parameters    []NamedValue           `json:"parameters,omitempty"`
	Uncertainties []Uncertainty          `json:"uncertainties,omitempty"`
	ThermalModel  *ThermalRCNetwork      `json:"thermal_model,omitempty"`
	TransientSOA  []TransientSOAEnvelope `json:"transient_soa,omitempty"`
}

// ThermalRCNetwork is reviewed catalog evidence for a bounded Foster thermal
// impedance model. The evaluator owns the differential equations; catalog
// data supplies only finite physical coefficients and boundary assumptions.
type ThermalRCNetwork struct {
	Reference          string           `json:"reference"`
	Stages             []ThermalRCStage `json:"stages"`
	BoundaryAssumption string           `json:"boundary_assumption"`
}

type ThermalRCStage struct {
	ThermalResistanceCPerW  float64 `json:"thermal_resistance_c_per_w"`
	ThermalCapacitanceJPerC float64 `json:"thermal_capacitance_j_per_c"`
	// CoolingCoupling distinguishes intrinsic/package impedance from the
	// ambient-coupled stage affected by airflow events. Empty preserves the
	// legacy behavior of scaling the complete stage.
	CoolingCoupling string `json:"cooling_coupling,omitempty"`
}

// TransientSOAEnvelope is one reviewed voltage/current boundary at either a
// finite pulse duration or DC. Boundaries are interpolated conservatively by
// the trusted evaluator and cannot contain provider-authored expressions.
type TransientSOAEnvelope struct {
	PulseDurationS   *float64            `json:"pulse_duration_s,omitempty"`
	DC               bool                `json:"dc,omitempty"`
	CaseTemperatureC float64             `json:"case_temperature_c"`
	Points           []TransientSOAPoint `json:"points"`
}

type TransientSOAPoint struct {
	VoltageV float64 `json:"voltage_v"`
	CurrentA float64 `json:"current_a"`
}

func CloneCatalogEvidence(source CatalogEvidence) CatalogEvidence {
	clone := CatalogEvidence{
		ModelID:       source.ModelID,
		Parameters:    append([]NamedValue(nil), source.Parameters...),
		Uncertainties: append([]Uncertainty(nil), source.Uncertainties...),
		TransientSOA:  make([]TransientSOAEnvelope, len(source.TransientSOA)),
	}
	if source.ThermalModel != nil {
		thermal := *source.ThermalModel
		thermal.Stages = append([]ThermalRCStage(nil), source.ThermalModel.Stages...)
		clone.ThermalModel = &thermal
	}
	for index, envelope := range source.TransientSOA {
		clone.TransientSOA[index] = envelope
		clone.TransientSOA[index].Points = append([]TransientSOAPoint(nil), envelope.Points...)
		if envelope.PulseDurationS != nil {
			duration := *envelope.PulseDurationS
			clone.TransientSOA[index].PulseDurationS = &duration
		}
	}
	return clone
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
	Metric            string   `json:"metric,omitempty"`
	AnalysisID        string   `json:"analysis_id,omitempty"`
	Node              string   `json:"node,omitempty"`
	Component         string   `json:"component,omitempty"`
	Components        []string `json:"components,omitempty"`
	ReferenceNode     string   `json:"reference_node,omitempty"`
	Quantity          string   `json:"quantity,omitempty"`
	FrequencyHz       float64  `json:"frequency_hz,omitempty"`
	TimeS             float64  `json:"time_s,omitempty"`
	WindowStartS      float64  `json:"window_start_s,omitempty"`
	WindowEndS        float64  `json:"window_end_s,omitempty"`
	ResponseDirection string   `json:"response_direction,omitempty" hash:"omitempty"`
	Min               float64  `json:"min"`
	Max               float64  `json:"max"`
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
	ID                   string                `json:"id"`
	Kind                 string                `json:"kind"`
	StartFrequencyHz     float64               `json:"start_frequency_hz,omitempty"`
	StopFrequencyHz      float64               `json:"stop_frequency_hz,omitempty"`
	Points               int                   `json:"points,omitempty"`
	DurationS            float64               `json:"duration_s,omitempty"`
	TimeStepS            float64               `json:"time_step_s,omitempty"`
	Excitations          []SourceExcitation    `json:"excitations"`
	Conditions           []NamedValue          `json:"conditions,omitempty"`
	DeviceOverrides      []DeviceOverride      `json:"device_overrides,omitempty"`
	SourceValueEvents    []SourceValueEvent    `json:"source_value_events,omitempty"`
	DeviceValueEvents    []DeviceValueEvent    `json:"device_value_events,omitempty"`
	ConditionValueEvents []ConditionValueEvent `json:"condition_value_events,omitempty"`
	DCSweep              *DCSweep              `json:"dc_sweep,omitempty"`
}

// SourceValueEvent applies one bounded, piecewise-constant event to an
// already-resolved independent source. Events cannot introduce sources,
// topology, expressions, or solver policy.
type SourceValueEvent struct {
	ID                   string   `json:"id"`
	Component            string   `json:"component"`
	TriggerTimeS         float64  `json:"trigger_time_s"`
	OriginalTriggerTimeS float64  `json:"original_trigger_time_s,omitempty"`
	DurationS            float64  `json:"duration_s"`
	Initial              float64  `json:"initial"`
	Applied              float64  `json:"applied"`
	Recovered            *float64 `json:"recovered,omitempty"`
}

// DeviceValueEvent applies one bounded, piecewise-constant value to an
// already-resolved value-bearing device such as a load resistor.
type DeviceValueEvent struct {
	ID                   string   `json:"id"`
	Component            string   `json:"component"`
	TriggerTimeS         float64  `json:"trigger_time_s"`
	OriginalTriggerTimeS float64  `json:"original_trigger_time_s,omitempty"`
	DurationS            float64  `json:"duration_s"`
	InitialSI            float64  `json:"initial_si"`
	AppliedSI            float64  `json:"applied_si"`
	RecoveredSI          *float64 `json:"recovered_si,omitempty"`
}

// ConditionValueEvent applies one registered bounded environmental
// condition, currently used for electrothermal boundary changes.
type ConditionValueEvent struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	TriggerTimeS         float64  `json:"trigger_time_s"`
	OriginalTriggerTimeS float64  `json:"original_trigger_time_s,omitempty"`
	DurationS            float64  `json:"duration_s"`
	Initial              float64  `json:"initial"`
	Applied              float64  `json:"applied"`
	Recovered            *float64 `json:"recovered,omitempty"`
}

// DCSweep requests a bounded deterministic sweep of one already resolved
// independent source or one reviewed scalar device value. Bidirectional
// sweeps preserve the converged active-device state between adjacent points so
// thresholds, hysteresis, and load regulation come from circuit equations.
type DCSweep struct {
	Component       string  `json:"component"`
	DeviceValue     bool    `json:"device_value,omitempty"`
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
	Usage             string `json:"usage,omitempty"`
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
	Component         string                 `json:"component"`
	PhysicalComponent string                 `json:"physical_component,omitempty"`
	CatalogID         string                 `json:"catalog_id"`
	Family            string                 `json:"family"`
	Usage             string                 `json:"usage,omitempty"`
	PrimitiveModel    string                 `json:"primitive_model"`
	ValueSI           *float64               `json:"value_si,omitempty"`
	ModelParameters   []NamedValue           `json:"model_parameters,omitempty"`
	ThermalModel      *ThermalRCNetwork      `json:"thermal_model,omitempty"`
	TransientSOA      []TransientSOAEnvelope `json:"transient_soa,omitempty"`
	Terminals         []TerminalBinding      `json:"terminals"`
	// Runtime indexes are immutable after indexMNAPlanDevices builds them.
	// Mutation paths must copy them and replace the owning device index.
	parameterIndex map[string]float64
	terminalIndex  map[string]string
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
		clone.Devices[index].ThermalModel = cloneThermalRCNetwork(source.Devices[index].ThermalModel)
		clone.Devices[index].TransientSOA = cloneTransientSOA(source.Devices[index].TransientSOA)
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

func cloneThermalRCNetwork(source *ThermalRCNetwork) *ThermalRCNetwork {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Stages = append([]ThermalRCStage(nil), source.Stages...)
	return &clone
}

func cloneTransientSOA(source []TransientSOAEnvelope) []TransientSOAEnvelope {
	clone := make([]TransientSOAEnvelope, len(source))
	for index, envelope := range source {
		clone[index] = envelope
		clone[index].Points = append([]TransientSOAPoint(nil), envelope.Points...)
		if envelope.PulseDurationS != nil {
			duration := *envelope.PulseDurationS
			clone[index].PulseDurationS = &duration
		}
	}
	return clone
}

func cloneAnalyses(source []Analysis) []Analysis {
	clone := append([]Analysis(nil), source...)
	for index := range clone {
		clone[index].Excitations = append([]SourceExcitation(nil), source[index].Excitations...)
		clone[index].Conditions = append([]NamedValue(nil), source[index].Conditions...)
		clone[index].DeviceOverrides = append([]DeviceOverride(nil), source[index].DeviceOverrides...)
		clone[index].SourceValueEvents = append([]SourceValueEvent(nil), source[index].SourceValueEvents...)
		clone[index].DeviceValueEvents = append([]DeviceValueEvent(nil), source[index].DeviceValueEvents...)
		clone[index].ConditionValueEvents = append([]ConditionValueEvent(nil), source[index].ConditionValueEvents...)
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
		for eventIndex := range clone[index].SourceValueEvents {
			if source[index].SourceValueEvents[eventIndex].Recovered != nil {
				value := *source[index].SourceValueEvents[eventIndex].Recovered
				clone[index].SourceValueEvents[eventIndex].Recovered = &value
			}
		}
		for eventIndex := range clone[index].DeviceValueEvents {
			if source[index].DeviceValueEvents[eventIndex].RecoveredSI != nil {
				value := *source[index].DeviceValueEvents[eventIndex].RecoveredSI
				clone[index].DeviceValueEvents[eventIndex].RecoveredSI = &value
			}
		}
		for eventIndex := range clone[index].ConditionValueEvents {
			if source[index].ConditionValueEvents[eventIndex].Recovered != nil {
				value := *source[index].ConditionValueEvents[eventIndex].Recovered
				clone[index].ConditionValueEvents[eventIndex].Recovered = &value
			}
		}
	}
	return clone
}

const DiagnosticAssertionOutOfBounds = "assertion_out_of_bounds"

type Diagnostic struct {
	Code       string `json:"code,omitempty"`
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
	Component             string   `json:"component"`
	VoltageV              float64  `json:"voltage_v,omitempty"`
	CurrentA              float64  `json:"current_a,omitempty"`
	CurrentMagnitudeA     float64  `json:"current_magnitude_a,omitempty"`
	DissipationW          float64  `json:"dissipation_w"`
	JunctionTemperatureC  *float64 `json:"junction_temperature_c,omitempty"`
	TransientSOAMargin    float64  `json:"transient_soa_margin_ratio,omitempty"`
	TransientSOAEvaluated bool     `json:"transient_soa_evaluated,omitempty"`
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
	AcceptedSubsteps       int     `json:"accepted_substeps,omitempty"`
}

type AnalysisResult struct {
	ID                     string               `json:"id"`
	Kind                   string               `json:"kind"`
	FundamentalFrequencyHz float64              `json:"fundamental_frequency_hz,omitempty"`
	ControlLoops           []ControlLoop        `json:"control_loops,omitempty"`
	PeriodicNodes          []PeriodicNodeResult `json:"periodic_nodes,omitempty"`
	Points                 []AnalysisPoint      `json:"points"`
}

// PeriodicNodeResult records a trusted model-derived steady-state envelope
// when an averaged switching primitive intentionally does not expose carrier
// cycles on the transient observation grid.
type PeriodicNodeResult struct {
	Node             string  `json:"node"`
	Component        string  `json:"component"`
	VoltageRippleVPP float64 `json:"voltage_ripple_v_pp"`
	Method           string  `json:"method"`
}

// ControlLoop records a loop derived from resolved primitive connectivity.
// Provider-authored intent cannot supply any of these fields.
type ControlLoop struct {
	ID                       string   `json:"id"`
	ActiveComponent          string   `json:"active_component"`
	PrimitiveModel           string   `json:"primitive_model"`
	InjectionTerminal        string   `json:"injection_terminal"`
	InjectionNet             string   `json:"injection_net"`
	ObservationNet           string   `json:"observation_net"`
	FeedbackTerminal         string   `json:"feedback_terminal"`
	FeedbackNet              string   `json:"feedback_net"`
	Polarity                 string   `json:"polarity"`
	Members                  []string `json:"members"`
	NetPath                  []string `json:"net_path"`
	DCPreserved              bool     `json:"dc_preserved"`
	CrossoverFrequencyHz     float64  `json:"crossover_frequency_hz"`
	PhaseMarginDeg           float64  `json:"phase_margin_deg"`
	GainMarginDB             float64  `json:"gain_margin_db"`
	ClosedLoopPeakingDB      float64  `json:"closed_loop_peaking_db"`
	ReturnRatioSamplesSHA256 string   `json:"return_ratio_samples_sha256"`
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

// CloneReport returns a deep copy of one simulation report. Keep this
// implementation adjacent to Report so schema changes also update the
// persistence-safe clone boundary.
func CloneReport(source Report) Report {
	return CloneReportWithAnalysisPointLimit(source, 0)
}

// CloneReportWithAnalysisPointLimit returns a deep copy while retaining at
// most pointLimit uniformly spaced points per analysis, including both
// endpoints. A non-positive limit preserves every point.
func CloneReportWithAnalysisPointLimit(source Report, pointLimit int) Report {
	clone := Report{
		Schema:          source.Schema,
		RegistryVersion: source.RegistryVersion,
		RegistryHash:    source.RegistryHash,
		CatalogID:       source.CatalogID,
		CatalogHash:     source.CatalogHash,
		ModelID:         source.ModelID,
		GroundNode:      source.GroundNode,
		TopologyHash:    source.TopologyHash,
		Status:          source.Status,
	}
	plan := ClonePlan(Plan{
		Bindings: source.Bindings,
		Inputs:   source.Inputs,
		Nodes:    source.Nodes,
		Devices:  source.Devices,
	})
	clone.Bindings = plan.Bindings
	clone.Inputs = plan.Inputs
	clone.Nodes = plan.Nodes
	clone.Devices = plan.Devices
	for index := range clone.Devices {
		// Runtime lookup indexes are rebuilt when a report is evaluated and do
		// not belong in the persistence-safe report clone boundary.
		clone.Devices[index].parameterIndex = nil
		clone.Devices[index].terminalIndex = nil
	}
	if source.Analyses != nil {
		clone.Analyses = make([]AnalysisResult, len(source.Analyses))
	}
	for analysisIndex := range clone.Analyses {
		clone.Analyses[analysisIndex] = AnalysisResult{
			ID:                     source.Analyses[analysisIndex].ID,
			Kind:                   source.Analyses[analysisIndex].Kind,
			FundamentalFrequencyHz: source.Analyses[analysisIndex].FundamentalFrequencyHz,
		}
		if source.Analyses[analysisIndex].ControlLoops != nil {
			clone.Analyses[analysisIndex].ControlLoops = make([]ControlLoop, len(source.Analyses[analysisIndex].ControlLoops))
		}
		for loopIndex := range clone.Analyses[analysisIndex].ControlLoops {
			clone.Analyses[analysisIndex].ControlLoops[loopIndex] = source.Analyses[analysisIndex].ControlLoops[loopIndex]
			clone.Analyses[analysisIndex].ControlLoops[loopIndex].Members = append([]string(nil), source.Analyses[analysisIndex].ControlLoops[loopIndex].Members...)
			clone.Analyses[analysisIndex].ControlLoops[loopIndex].NetPath = append([]string(nil), source.Analyses[analysisIndex].ControlLoops[loopIndex].NetPath...)
		}
		clone.Analyses[analysisIndex].PeriodicNodes = append(
			[]PeriodicNodeResult(nil),
			source.Analyses[analysisIndex].PeriodicNodes...,
		)
		clone.Analyses[analysisIndex].Points = cloneAnalysisPoints(
			source.Analyses[analysisIndex].Points,
			pointLimit,
		)
	}
	// Measurement and SensitivityResult are value-only schema records. The
	// clone-schema guard test fails if either gains a reference field.
	clone.Measurements = append([]Measurement(nil), source.Measurements...)
	clone.Assertions = cloneAssertionResults(source.Assertions)
	if source.Corners != nil {
		clone.Corners = make([]CornerResult, len(source.Corners))
	}
	for cornerIndex := range clone.Corners {
		clone.Corners[cornerIndex] = CornerResult{
			ID:     source.Corners[cornerIndex].ID,
			Status: source.Corners[cornerIndex].Status,
		}
		clone.Corners[cornerIndex].Assignments = append([]NamedValue(nil), source.Corners[cornerIndex].Assignments...)
		clone.Corners[cornerIndex].Assertions = cloneAssertionResults(source.Corners[cornerIndex].Assertions)
	}
	clone.Sensitivity = append([]SensitivityResult(nil), source.Sensitivity...)
	return clone
}

func cloneAnalysisPoints(source []AnalysisPoint, limit int) []AnalysisPoint {
	if source == nil {
		return nil
	}
	count := len(source)
	if limit > 0 {
		count = min(count, limit)
	}
	clone := make([]AnalysisPoint, count)
	last := len(source) - 1
	for index := range clone {
		sourceIndex := index
		if len(source) > count && count > 1 {
			sourceIndex = int(int64(index) * int64(last) / int64(count-1))
		}
		clone[index] = AnalysisPoint{
			FrequencyHz: source[sourceIndex].FrequencyHz,
			TimeS:       source[sourceIndex].TimeS,
			SweepValue:  source[sourceIndex].SweepValue,
			Sweep:       source[sourceIndex].Sweep,
		}
		clone[index].Nodes = append([]NodeResult(nil), source[sourceIndex].Nodes...)
		if source[sourceIndex].Devices != nil {
			clone[index].Devices = make([]DeviceResult, len(source[sourceIndex].Devices))
		}
		for deviceIndex := range clone[index].Devices {
			sourceDevice := source[sourceIndex].Devices[deviceIndex]
			clone[index].Devices[deviceIndex] = DeviceResult{
				Component:             sourceDevice.Component,
				VoltageV:              sourceDevice.VoltageV,
				CurrentA:              sourceDevice.CurrentA,
				CurrentMagnitudeA:     sourceDevice.CurrentMagnitudeA,
				DissipationW:          sourceDevice.DissipationW,
				TransientSOAMargin:    sourceDevice.TransientSOAMargin,
				TransientSOAEvaluated: sourceDevice.TransientSOAEvaluated,
			}
			if source[sourceIndex].Devices[deviceIndex].JunctionTemperatureC != nil {
				value := *source[sourceIndex].Devices[deviceIndex].JunctionTemperatureC
				clone[index].Devices[deviceIndex].JunctionTemperatureC = &value
			}
		}
		if source[sourceIndex].Solver != nil {
			solver := *source[sourceIndex].Solver
			clone[index].Solver = &solver
		}
	}
	return clone
}

func cloneAssertionResults(source []AssertionResult) []AssertionResult {
	if source == nil {
		return nil
	}
	clone := make([]AssertionResult, len(source))
	for index := range clone {
		clone[index] = AssertionResult{
			Metric:        source[index].Metric,
			AnalysisID:    source[index].AnalysisID,
			Node:          source[index].Node,
			Component:     source[index].Component,
			ReferenceNode: source[index].ReferenceNode,
			Quantity:      source[index].Quantity,
			FrequencyHz:   source[index].FrequencyHz,
			TimeS:         source[index].TimeS,
			Min:           source[index].Min,
			Max:           source[index].Max,
			Actual:        source[index].Actual,
			Pass:          source[index].Pass,
		}
		clone[index].Components = append([]string(nil), source[index].Components...)
	}
	return clone
}
