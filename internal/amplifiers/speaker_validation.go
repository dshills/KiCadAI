package amplifiers

import (
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"unicode"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const measurementNameMaxLength = 64

const (
	CodeSpeakerComponentEvidence reports.Code = "SPEAKER_AMPLIFIER_COMPONENT_EVIDENCE"
	CodeSpeakerOutputPower       reports.Code = "SPEAKER_AMPLIFIER_OUTPUT_POWER"
	CodeSpeakerReactiveLoad      reports.Code = "SPEAKER_AMPLIFIER_REACTIVE_LOAD"
	CodeSpeakerDriverLimit       reports.Code = "SPEAKER_AMPLIFIER_DRIVER_LIMIT"
	CodeSpeakerDistortion        reports.Code = "SPEAKER_AMPLIFIER_DISTORTION"
	CodeSpeakerElectrothermal    reports.Code = "SPEAKER_AMPLIFIER_ELECTROTHERMAL"
	CodeSpeakerProtection        reports.Code = "SPEAKER_AMPLIFIER_PROTECTION"
	CodeSpeakerLayout            reports.Code = "SPEAKER_AMPLIFIER_LAYOUT"
)

const (
	AnalysisComponents     AnalysisKind = "components"
	AnalysisOutputPower    AnalysisKind = "output_power"
	AnalysisReactiveLoad   AnalysisKind = "reactive_load"
	AnalysisDriver         AnalysisKind = "driver"
	AnalysisDistortion     AnalysisKind = "distortion"
	AnalysisElectrothermal AnalysisKind = "electrothermal"
	AnalysisProtection     AnalysisKind = "protection"
	AnalysisLayout         AnalysisKind = "layout"
)

// SpeakerLoadCase declares the electrical load and the independently derived
// closed-loop phase margin for one resistive or reactive speaker corner.
type SpeakerLoadCase struct {
	Name           string  `json:"name"`
	ResistanceOhm  float64 `json:"resistance_ohm"`
	InductanceH    float64 `json:"inductance_h,omitempty"`
	CapacitanceF   float64 `json:"capacitance_f,omitempty"`
	PhaseMarginDeg float64 `json:"phase_margin_deg"`
}

type SpeakerDistortionBudget struct {
	MaximumTHDPct   float64 `json:"maximum_thd_pct"`
	OpAmpTHDPct     float64 `json:"opamp_thd_pct"`
	CrossoverTHDPct float64 `json:"crossover_thd_pct"`
	FeedbackTHDPct  float64 `json:"feedback_thd_pct"`
	LoadTHDPct      float64 `json:"load_thd_pct"`
}

// SpeakerThermalModel is an applied assembly contract. TransientRJCFactor is
// the reviewed fraction of steady-state junction-to-case resistance at the
// declared short-circuit duration; its non-empty basis makes the approximation
// auditable instead of silently assuming an instantaneous thermal path.
type SpeakerThermalModel struct {
	HeatsinkManufacturer string  `json:"heatsink_manufacturer"`
	HeatsinkMPN          string  `json:"heatsink_mpn"`
	SinkToAmbientCPerW   float64 `json:"sink_to_ambient_c_per_w"`
	CaseToSinkCPerW      float64 `json:"case_to_sink_c_per_w"`
	HotAmbientC          float64 `json:"hot_ambient_c"`
	BlockedAirflowFactor float64 `json:"blocked_airflow_factor"`
	JunctionMarginC      float64 `json:"junction_margin_c"`
	TransientRJCFactor   float64 `json:"transient_rjc_factor"`
	TransientBasis       string  `json:"transient_basis"`
}

type SpeakerProtectionContract struct {
	CurrentLimitA         float64 `json:"current_limit_a"`
	SenseThresholdV       float64 `json:"sense_threshold_v"`
	ShortCircuitDurationS float64 `json:"short_circuit_duration_s"`
	PositiveDCTripV       float64 `json:"positive_dc_trip_v"`
	NegativeDCTripV       float64 `json:"negative_dc_trip_v"`
	MaximumDCTripV        float64 `json:"maximum_dc_trip_v"`
	TurnOnDelayS          float64 `json:"turn_on_delay_s"`
	MinimumTurnOnDelayS   float64 `json:"minimum_turn_on_delay_s"`
	MaximumTurnOnDelayS   float64 `json:"maximum_turn_on_delay_s"`
	ReleaseTimeS          float64 `json:"release_time_s"`
	MaximumReleaseTimeS   float64 `json:"maximum_release_time_s"`
	RelayContactRatingA   float64 `json:"relay_contact_rating_a"`
	RelayNormallyOpen     bool    `json:"relay_normally_open"`
	RelayClampPresent     bool    `json:"relay_clamp_present"`
	SupplyFaultRelease    bool    `json:"supply_fault_release"`
	TurnOffMute           bool    `json:"turn_off_mute"`
	ZobelResistanceOhm    float64 `json:"zobel_resistance_ohm"`
	ZobelCapacitanceF     float64 `json:"zobel_capacitance_f"`
}

type SpeakerLayoutContract struct {
	StarGround                  bool    `json:"star_ground"`
	KelvinFeedback              bool    `json:"kelvin_feedback"`
	KelvinEmitterSense          bool    `json:"kelvin_emitter_sense"`
	LocalRailDecoupling         bool    `json:"local_rail_decoupling"`
	ThermalBiasCoupling         bool    `json:"thermal_bias_coupling"`
	ComplementarySymmetry       bool    `json:"complementary_symmetry"`
	HeatsinkKeepout             bool    `json:"heatsink_keepout"`
	MountingAccess              bool    `json:"mounting_access"`
	CopperWidthMM               float64 `json:"copper_width_mm"`
	MinimumCopperWidthMM        float64 `json:"minimum_copper_width_mm"`
	CopperClearanceMM           float64 `json:"copper_clearance_mm"`
	MinimumClearanceMM          float64 `json:"minimum_clearance_mm"`
	HighCurrentLoopLengthMM     float64 `json:"high_current_loop_length_mm"`
	MaximumLoopLengthMM         float64 `json:"maximum_loop_length_mm"`
	FeedbackSeparationMM        float64 `json:"feedback_separation_mm"`
	MinimumFeedbackSeparationMM float64 `json:"minimum_feedback_separation_mm"`
}

type SpeakerComponentSet struct {
	OpAmp           *components.ComponentRecord `json:"opamp,omitempty"`
	UpperDriver     *components.ComponentRecord `json:"upper_driver,omitempty"`
	LowerDriver     *components.ComponentRecord `json:"lower_driver,omitempty"`
	UpperOutput     *components.ComponentRecord `json:"upper_output,omitempty"`
	LowerOutput     *components.ComponentRecord `json:"lower_output,omitempty"`
	EmitterResistor *components.ComponentRecord `json:"emitter_resistor,omitempty"`
	ZobelResistor   *components.ComponentRecord `json:"zobel_resistor,omitempty"`
	Relay           *components.ComponentRecord `json:"relay,omitempty"`
}

// SpeakerAmplifierRequest contains only reusable electrical and physical
// contracts. It deliberately has no fixture name or absolute PCB coordinate.
type SpeakerAmplifierRequest struct {
	Envelope                    ValidationRequest         `json:"envelope"`
	TargetPowerW                float64                   `json:"target_power_w"`
	TargetLoadOhm               float64                   `json:"target_load_ohm"`
	OutputStageLossV            float64                   `json:"output_stage_loss_v"`
	DriverGainMinimum           float64                   `json:"driver_gain_minimum"`
	OutputGainMinimum           float64                   `json:"output_gain_minimum"`
	OpAmpOutputCurrentA         float64                   `json:"opamp_output_current_a"`
	EmitterResistanceOhm        float64                   `json:"emitter_resistance_ohm"`
	EmitterResistorRatingW      float64                   `json:"emitter_resistor_rating_w"`
	EmitterResistorDerating     float64                   `json:"emitter_resistor_derating"`
	BiasSpreadV                 float64                   `json:"bias_spread_v"`
	RequiredBiasSpreadV         float64                   `json:"required_bias_spread_v"`
	BiasToleranceV              float64                   `json:"bias_tolerance_v"`
	Loads                       []SpeakerLoadCase         `json:"loads"`
	MinimumLoadOhm              float64                   `json:"minimum_load_ohm,omitempty"`
	MaximumLoadOhm              float64                   `json:"maximum_load_ohm,omitempty"`
	RequireStandardLoadCoverage bool                      `json:"require_standard_load_coverage,omitempty"`
	Distortion                  SpeakerDistortionBudget   `json:"distortion"`
	Thermal                     SpeakerThermalModel       `json:"thermal"`
	Protection                  SpeakerProtectionContract `json:"protection"`
	Layout                      SpeakerLayoutContract     `json:"layout"`
	Components                  SpeakerComponentSet       `json:"components"`
}

func ValidateSpeakerAmplifier(request SpeakerAmplifierRequest) ValidationResult {
	base := ValidateOperatingEnvelope(request.Envelope)
	analyses := append([]AnalysisResult{}, base.Analyses...)
	analyses = append(analyses,
		validateSpeakerComponents(request),
		validateSpeakerOutputPower(request),
		validateSpeakerLoads(request),
		validateSpeakerDriver(request),
		validateSpeakerDistortion(request),
		validateSpeakerElectrothermal(request),
		validateSpeakerProtection(request),
		validateSpeakerLayout(request),
	)
	sanitizeAnalysisMeasurements(analyses)
	result := ValidationResult{Pass: true, Analyses: analyses}
	for _, analysis := range analyses {
		result.Pass = result.Pass && analysis.Pass
		result.Issues = append(result.Issues, analysis.Issues...)
	}
	return result
}

// sanitizeAnalysisMeasurements is a final serialization boundary for speaker
// validation. Individual analyzers already reject non-finite inputs and use
// addMeasurement, but this defense keeps future analyzers from leaking values
// that encoding/json cannot represent.
func sanitizeAnalysisMeasurements(analyses []AnalysisResult) {
	for index := range analyses {
		for name, value := range analyses[index].Measurements {
			if !finiteNumber(value) {
				delete(analyses[index].Measurements, name)
			}
		}
	}
}

func validateSpeakerComponents(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisComponents)
	set := request.Components
	for _, item := range []struct {
		path   string
		record *components.ComponentRecord
	}{
		{path: "opamp", record: set.OpAmp},
		{path: "upper_driver", record: set.UpperDriver},
		{path: "lower_driver", record: set.LowerDriver},
		{path: "upper_output", record: set.UpperOutput},
		{path: "lower_output", record: set.LowerOutput},
		{path: "emitter_resistor", record: set.EmitterResistor},
		{path: "zobel_resistor", record: set.ZobelResistor},
		{path: "relay", record: set.Relay},
	} {
		if !concreteVerifiedComponent(item.record) {
			result.addIssue(CodeSpeakerComponentEvidence, item.path, "speaker fabrication candidates require a concrete verified component identity", "select a non-generic procurement-usable record with verified resolver and pin-map evidence")
		}
	}
	if set.OpAmp == nil || set.OpAmp.OpAmp == nil || !set.OpAmp.OpAmp.FabricationProof {
		result.addIssue(CodeSpeakerComponentEvidence, "opamp.opamp_evidence", "fabrication-proven op-amp evidence is required", "select an op-amp with reviewed drive, bandwidth, stability, noise, distortion, and thermal evidence")
	}
	validateSpeakerPair(&result, "driver_pair", set.UpperDriver, set.LowerDriver, false)
	validateSpeakerPair(&result, "output_pair", set.UpperOutput, set.LowerOutput, true)
	return result.finish()
}

func concreteVerifiedComponent(record *components.ComponentRecord) bool {
	return record != nil && !record.Generic && strings.TrimSpace(record.MPN) != "" && speakerLifecycleProcurementUsable(record.Lifecycle) && record.Verification.Confidence == components.ConfidenceVerified && record.Verification.PinMapChecked
}

func speakerLifecycleProcurementUsable(lifecycle string) bool {
	switch strings.ToLower(strings.TrimSpace(lifecycle)) {
	case "active", "preferred", "mature", "nrnd":
		return true
	default:
		return false
	}
}

func validateSpeakerPair(result *AnalysisResult, path string, upper, lower *components.ComponentRecord, requireOutputEvidence bool) {
	if upper == nil || lower == nil || upper.PowerSemiconductor == nil || lower.PowerSemiconductor == nil {
		result.addIssue(CodeSpeakerComponentEvidence, path, "complementary pair requires typed power-semiconductor evidence", "select reviewed complementary driver or output records")
		return
	}
	u, l := upper.PowerSemiconductor, lower.PowerSemiconductor
	if !u.FabricationProof || !l.FabricationProof || len(u.SOA) < 2 || len(l.SOA) < 2 || strings.TrimSpace(u.ComplementaryGroup) == "" || u.ComplementaryGroup != l.ComplementaryGroup || u.Polarity != "npn" || l.Polarity != "pnp" {
		result.addIssue(CodeSpeakerComponentEvidence, path+".pairing", "pair polarity, complementary group, fabrication proof, or DC SOA evidence is incomplete", "select an NPN/PNP pair from one reviewed complementary group")
	}
	if requireOutputEvidence {
		if !speakerOutputEvidenceProven(upper) || !speakerOutputEvidenceProven(lower) {
			result.addIssue(CodeSpeakerComponentEvidence, path+".amplifier_output_evidence", "output devices require reviewed amplifier-use and SOA status", "select fabrication-approved linear audio output devices")
		}
	}
}

func speakerOutputEvidenceProven(record *components.ComponentRecord) bool {
	return record != nil && record.AmplifierOutput != nil && record.AmplifierOutput.SafeOperatingAreaStatus == "proven"
}

func validateSpeakerOutputPower(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisOutputPower)
	negativeRail, positiveRail, railsOK := validationRails(request.Envelope)
	railSwing := math.Min(positiveRail-request.Envelope.OutputBiasV, request.Envelope.OutputBiasV-negativeRail)
	availablePeakV := math.Min(railSwing-request.OutputStageLossV, request.Protection.CurrentLimitA*request.TargetLoadOhm)
	availablePeakV = math.Max(0, availablePeakV)
	powerW := availablePeakV * availablePeakV / (2 * request.TargetLoadOhm)
	clippingInputPeakV := availablePeakV / request.Envelope.ClosedLoopGain
	result.addMeasurement("available_peak_output_v", availablePeakV)
	result.addMeasurement("available_rms_output_v", availablePeakV/math.Sqrt2)
	result.addMeasurement("available_output_power_w", powerW)
	result.addMeasurement("clipping_input_peak_v", clippingInputPeakV)
	if !railsOK || !positiveFinite(request.TargetPowerW) || !positiveFinite(request.TargetLoadOhm) || !nonNegativeFinite(request.OutputStageLossV) || !positiveFinite(availablePeakV) || !positiveFinite(powerW) || powerW < request.TargetPowerW {
		result.addIssue(CodeSpeakerOutputPower, "target_power", "available unclipped output does not satisfy the declared RMS power target", "increase rail headroom, reduce stage loss, or lower the target load/power")
	}
	return result.finish()
}

func validateSpeakerLoads(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisReactiveLoad)
	result.addMeasurement("load_case_count", float64(len(request.Loads)))
	minimumLoadOhm, maximumLoadOhm, validLoadBounds := speakerLoadBounds(request)
	result.addMeasurement("minimum_validated_load_ohm", minimumLoadOhm)
	result.addMeasurement("maximum_validated_load_ohm", maximumLoadOhm)
	if !validLoadBounds {
		result.addIssue(CodeSpeakerReactiveLoad, "loads.envelope", "minimum_load_ohm and maximum_load_ohm must define a finite positive impedance envelope", "declare ordered positive load bounds; omitted bounds default to 2-16 ohms")
	}
	hasFourOhm := false
	hasEightOhm := false
	hasReactive := false
	seen := map[string]bool{}
	seenMeasurementNames := map[string]bool{}
	availablePeakV := availableSpeakerPeakV(request)
	for _, load := range request.Loads {
		name := strings.TrimSpace(load.Name)
		if name == "" || seen[name] {
			result.addIssue(CodeSpeakerReactiveLoad, "loads", "load-case names must be non-empty and unique", "declare deterministic names for each resistive and reactive load")
			continue
		}
		seen[name] = true
		measurementKey := measurementName(name)
		if measurementKey == "" || seenMeasurementNames[measurementKey] {
			result.addIssue(CodeSpeakerReactiveLoad, "loads."+name, "load-case names must remain unique after measurement-key normalization", "choose names that differ by letters or digits, not only punctuation")
			continue
		}
		seenMeasurementNames[measurementKey] = true
		if !positiveFinite(load.ResistanceOhm) || load.ResistanceOhm < minimumLoadOhm || load.ResistanceOhm > maximumLoadOhm || !nonNegativeFinite(load.InductanceH) || !nonNegativeFinite(load.CapacitanceF) || !positiveFinite(load.PhaseMarginDeg) || load.PhaseMarginDeg < 45 {
			result.addIssue(CodeSpeakerReactiveLoad, "loads."+name, "load must be finite, inside the declared impedance envelope, and have at least 45 degrees phase margin", "revise compensation or constrain the declared load envelope")
			continue
		}
		hasFourOhm = hasFourOhm || math.Abs(load.ResistanceOhm-4) <= 1e-9
		hasEightOhm = hasEightOhm || math.Abs(load.ResistanceOhm-8) <= 1e-9
		hasReactive = hasReactive || load.InductanceH > 0 || load.CapacitanceF > 0
		peakCurrent := math.Min(availablePeakV/load.ResistanceOhm, request.Protection.CurrentLimitA)
		result.addMeasurement("load_"+measurementKey+"_peak_current_a", peakCurrent)
		result.addMeasurement("load_"+measurementKey+"_power_w", math.Pow(peakCurrent*load.ResistanceOhm, 2)/(2*load.ResistanceOhm))
	}
	if len(request.Loads) == 0 {
		result.addIssue(CodeSpeakerReactiveLoad, "loads.coverage", "validation requires at least one load case inside the declared impedance envelope", "add a load case with its reviewed stability margin")
	} else if request.RequireStandardLoadCoverage && (len(request.Loads) < 3 || !hasFourOhm || !hasEightOhm || !hasReactive) {
		result.addIssue(CodeSpeakerReactiveLoad, "loads.coverage", "validation requires multiple 4-8 ohm cases including a representative reactive load", "add 8 ohm, 4 ohm, and inductive/capacitive speaker corners")
	}
	return result.finish()
}

func speakerLoadBounds(request SpeakerAmplifierRequest) (float64, float64, bool) {
	minimumLoadOhm := request.MinimumLoadOhm
	maximumLoadOhm := request.MaximumLoadOhm
	if minimumLoadOhm == 0 {
		minimumLoadOhm = 2
	}
	if maximumLoadOhm == 0 {
		maximumLoadOhm = 16
	}
	return minimumLoadOhm, maximumLoadOhm, positiveFinite(minimumLoadOhm) && positiveFinite(maximumLoadOhm) && minimumLoadOhm <= maximumLoadOhm
}

func validateSpeakerDriver(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisDriver)
	minLoad := minimumLoadResistance(request.Loads)
	availablePeakV := availableSpeakerPeakV(request)
	peakCurrent := 0.0
	if positiveFinite(minLoad) {
		peakCurrent = math.Min(availablePeakV/minLoad, request.Protection.CurrentLimitA)
	}
	requiredOpAmpCurrent := peakCurrent / (request.DriverGainMinimum * request.OutputGainMinimum)
	biasError := math.Abs(request.BiasSpreadV - request.RequiredBiasSpreadV)
	emitterPower := peakCurrent * peakCurrent * request.EmitterResistanceOhm / 4
	emitterLimit := request.EmitterResistorRatingW * request.EmitterResistorDerating
	result.addMeasurement("worst_peak_output_current_a", peakCurrent)
	result.addMeasurement("required_opamp_output_current_a", requiredOpAmpCurrent)
	result.addMeasurement("available_opamp_output_current_a", request.OpAmpOutputCurrentA)
	result.addMeasurement("bias_spread_error_v", biasError)
	result.addMeasurement("emitter_resistor_dissipation_w", emitterPower)
	result.addMeasurement("emitter_resistor_applied_limit_w", emitterLimit)
	if !positiveFinite(request.DriverGainMinimum) || !positiveFinite(request.OutputGainMinimum) || !positiveFinite(request.OpAmpOutputCurrentA) || !nonNegativeFinite(requiredOpAmpCurrent) || requiredOpAmpCurrent > request.OpAmpOutputCurrentA {
		result.addIssue(CodeSpeakerDriverLimit, "base_drive", "op-amp and driver gain do not cover worst-case output base current", "increase verified driver gain/current or reduce the output-current envelope")
	}
	if !positiveFinite(request.BiasSpreadV) || !positiveFinite(request.RequiredBiasSpreadV) || !nonNegativeFinite(request.BiasToleranceV) || biasError > request.BiasToleranceV {
		result.addIssue(CodeSpeakerDriverLimit, "bias_spread", "bias spread falls outside the declared cold/hot tracking tolerance", "revise the thermally coupled bias spreader or tolerance allocation")
	}
	if !positiveFinite(request.EmitterResistanceOhm) || !positiveFinite(request.EmitterResistorRatingW) || !positiveFinite(request.EmitterResistorDerating) || request.EmitterResistorDerating > 1 || !nonNegativeFinite(emitterPower) || emitterPower > emitterLimit {
		result.addIssue(CodeSpeakerDriverLimit, "emitter_resistor", "emitter resistor exceeds its applied derated dissipation limit", "increase resistance power rating or reduce current")
	}
	return result.finish()
}

func validateSpeakerDistortion(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisDistortion)
	d := request.Distortion
	total := d.OpAmpTHDPct + d.CrossoverTHDPct + d.FeedbackTHDPct + d.LoadTHDPct
	result.addMeasurement("calculated_thd_pct", total)
	result.addMeasurement("maximum_thd_pct", d.MaximumTHDPct)
	if !positiveFinite(d.MaximumTHDPct) || !nonNegativeFinite(d.OpAmpTHDPct) || !nonNegativeFinite(d.CrossoverTHDPct) || !nonNegativeFinite(d.FeedbackTHDPct) || !nonNegativeFinite(d.LoadTHDPct) || !finiteNumber(total) || total > d.MaximumTHDPct {
		result.addIssue(CodeSpeakerDistortion, "thd_budget", "summed op-amp, crossover, feedback, and load distortion exceeds the declared THD budget", "improve bias/feedback linearity or relax the explicitly reviewed THD target")
	}
	return result.finish()
}

func validateSpeakerElectrothermal(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisElectrothermal)
	thermal := request.Thermal
	upper := powerEvidence(request.Components.UpperOutput)
	lower := powerEvidence(request.Components.LowerOutput)
	if upper == nil || lower == nil || upper.JunctionToCaseCPerW == nil || lower.JunctionToCaseCPerW == nil || upper.MaxJunctionTemperatureC == nil || lower.MaxJunctionTemperatureC == nil {
		result.addIssue(CodeSpeakerElectrothermal, "device_evidence", "both output devices require junction-to-case and maximum-junction evidence", "select fabrication-proven output devices")
		return result.finish()
	}
	if strings.TrimSpace(thermal.HeatsinkManufacturer) == "" || strings.TrimSpace(thermal.HeatsinkMPN) == "" || !positiveFinite(thermal.SinkToAmbientCPerW) || !nonNegativeFinite(thermal.CaseToSinkCPerW) || !positiveFinite(thermal.BlockedAirflowFactor) || thermal.BlockedAirflowFactor < 1 || !positiveFinite(thermal.JunctionMarginC) || !positiveFinite(thermal.TransientRJCFactor) || thermal.TransientRJCFactor > 1 || strings.TrimSpace(thermal.TransientBasis) == "" {
		result.addIssue(CodeSpeakerElectrothermal, "thermal_model", "complete concrete heatsink, interface, blocked-airflow, junction-margin, and transient-impedance evidence is required", "declare a reviewed heatsink assembly and bounded transient thermal factor")
		return result.finish()
	}
	rail := speakerRailMagnitude(request.Envelope)
	minLoad := minimumLoadResistance(request.Loads)
	peakAtWorstDissipation := math.Min(2*rail/(math.Pi*minLoad), request.Protection.CurrentLimitA)
	perDeviceW := rail*peakAtWorstDissipation/math.Pi - peakAtWorstDissipation*peakAtWorstDissipation*minLoad/4 + rail*request.Envelope.QuiescentCurrentA
	totalSinkW := 2 * perDeviceW
	hotSinkC := thermal.HotAmbientC + totalSinkW*thermal.SinkToAmbientCPerW
	blockedSinkC := thermal.HotAmbientC + totalSinkW*thermal.SinkToAmbientCPerW*thermal.BlockedAirflowFactor
	upperBlockedCaseC := blockedSinkC + perDeviceW*thermal.CaseToSinkCPerW
	lowerBlockedCaseC := blockedSinkC + perDeviceW*thermal.CaseToSinkCPerW
	upperJunctionToCaseCPerW := *upper.JunctionToCaseCPerW
	lowerJunctionToCaseCPerW := *lower.JunctionToCaseCPerW
	upperBlockedJunctionC := upperBlockedCaseC + perDeviceW*upperJunctionToCaseCPerW
	lowerBlockedJunctionC := lowerBlockedCaseC + perDeviceW*lowerJunctionToCaseCPerW
	junctionLimitC := math.Min(*upper.MaxJunctionTemperatureC, *lower.MaxJunctionTemperatureC) - thermal.JunctionMarginC
	shortPowerW := rail * request.Protection.CurrentLimitA
	worstJunctionToCaseCPerW := math.Max(*upper.JunctionToCaseCPerW, *lower.JunctionToCaseCPerW)
	shortJunctionC := blockedSinkC + shortPowerW*(thermal.CaseToSinkCPerW+worstJunctionToCaseCPerW)*thermal.TransientRJCFactor
	result.addMeasurement("worst_per_device_dissipation_w", perDeviceW)
	result.addMeasurement("hot_heatsink_temperature_c", hotSinkC)
	result.addMeasurement("blocked_heatsink_temperature_c", blockedSinkC)
	result.addMeasurement("upper_blocked_case_temperature_c", upperBlockedCaseC)
	result.addMeasurement("lower_blocked_case_temperature_c", lowerBlockedCaseC)
	result.addMeasurement("upper_blocked_junction_temperature_c", upperBlockedJunctionC)
	result.addMeasurement("lower_blocked_junction_temperature_c", lowerBlockedJunctionC)
	result.addMeasurement("junction_design_limit_c", junctionLimitC)
	result.addMeasurement("short_transient_junction_temperature_c", shortJunctionC)
	if !positiveFinite(rail) || !positiveFinite(minLoad) || !positiveFinite(thermal.HotAmbientC) || !nonNegativeFinite(perDeviceW) || upperBlockedJunctionC >= junctionLimitC || lowerBlockedJunctionC >= junctionLimitC || shortJunctionC >= junctionLimitC {
		result.addIssue(CodeSpeakerElectrothermal, "junction_temperature", "hot, blocked-airflow, or short-transient junction temperature reaches the design limit", "improve the heatsink/interface, reduce current, or increase device thermal margin")
	}
	validateSpeakerSOA(&result, "upper_output", upper, rail, minLoad, upperBlockedCaseC, request.Protection.CurrentLimitA)
	validateSpeakerSOA(&result, "lower_output", lower, rail, minLoad, lowerBlockedCaseC, request.Protection.CurrentLimitA)
	return result.finish()
}

func validateSpeakerSOA(result *AnalysisResult, path string, evidence *components.PowerSemiconductorEvidence, rail, load, temperature, currentLimit float64) {
	normalVoltage := rail / 2
	normalCurrent := math.Min(rail/(2*load), currentLimit)
	allowedNormal, okNormal := temperatureDeratedSOA(evidence, normalVoltage, temperature)
	allowedShort, okShort := temperatureDeratedSOA(evidence, rail, temperature)
	result.addMeasurement(path+"_normal_soa_allowed_a", allowedNormal)
	result.addMeasurement(path+"_short_soa_allowed_a", allowedShort)
	if !okNormal || allowedNormal < normalCurrent || !okShort || allowedShort < currentLimit {
		result.addIssue(CodeSpeakerElectrothermal, path+".soa", "normal-load or short-circuit operating point exceeds the temperature-derated DC SOA", "lower the current limit or select an output device with greater reviewed linear SOA")
	}
}

func temperatureDeratedSOA(evidence *components.PowerSemiconductorEvidence, voltage, temperature float64) (float64, bool) {
	if evidence == nil || evidence.MaxJunctionTemperatureC == nil {
		return 0, false
	}
	allowed, basis, ok := interpolateDCAllowedCurrent(evidence.SOA, voltage)
	if !ok {
		return 0, false
	}
	if temperature > basis {
		span := *evidence.MaxJunctionTemperatureC - basis
		if span <= 0 {
			return 0, false
		}
		allowed *= math.Max(0, (*evidence.MaxJunctionTemperatureC-temperature)/span)
	}
	return allowed, positiveFinite(allowed)
}

func validateSpeakerProtection(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisProtection)
	p := request.Protection
	relayEvidenceRatingA, relayEvidenceOK := componentMaximumRating(request.Components.Relay, "contact_current_dc", "A")
	result.addMeasurement("relay_evidence_contact_rating_a", relayEvidenceRatingA)
	currentLimitInputsValid := positiveFinite(p.CurrentLimitA) && positiveFinite(p.SenseThresholdV) && positiveFinite(request.EmitterResistanceOhm)
	sensedLimit := 0.0
	if currentLimitInputsValid {
		sensedLimit = p.SenseThresholdV / request.EmitterResistanceOhm
	}
	result.addMeasurement("calculated_current_limit_a", sensedLimit)
	result.addMeasurement("declared_current_limit_a", p.CurrentLimitA)
	if !currentLimitInputsValid || math.Abs(sensedLimit-p.CurrentLimitA) > 0.1*p.CurrentLimitA || !positiveFinite(p.ShortCircuitDurationS) {
		result.addIssue(CodeSpeakerProtection, "current_limit", "emitter-sense threshold does not establish the declared bounded short-circuit current", "align sense resistance/threshold and declare a finite short duration")
	}
	if !positiveFinite(p.PositiveDCTripV) || !positiveFinite(p.NegativeDCTripV) || !positiveFinite(p.MaximumDCTripV) || p.PositiveDCTripV > p.MaximumDCTripV || p.NegativeDCTripV > p.MaximumDCTripV {
		result.addIssue(CodeSpeakerProtection, "dc_detection", "positive and negative DC trip thresholds must be finite and below the declared speaker-safe maximum", "lower or balance the DC detector thresholds")
	}
	if !positiveFinite(p.TurnOnDelayS) || !positiveFinite(p.MinimumTurnOnDelayS) || !positiveFinite(p.MaximumTurnOnDelayS) || p.TurnOnDelayS < p.MinimumTurnOnDelayS || p.TurnOnDelayS > p.MaximumTurnOnDelayS {
		result.addIssue(CodeSpeakerProtection, "turn_on_mute", "relay engagement lies outside the reviewed turn-on mute window", "adjust the deterministic delay network")
	}
	if !positiveFinite(p.ReleaseTimeS) || !positiveFinite(p.MaximumReleaseTimeS) || p.ReleaseTimeS > p.MaximumReleaseTimeS || !p.SupplyFaultRelease || !p.TurnOffMute {
		result.addIssue(CodeSpeakerProtection, "release", "turn-off or supply-fault relay release is missing or too slow", "add supply-loss detection and meet the release-time limit")
	}
	if !p.RelayNormallyOpen || !p.RelayClampPresent || !positiveFinite(p.RelayContactRatingA) || !relayEvidenceOK || p.RelayContactRatingA > relayEvidenceRatingA || math.Min(p.RelayContactRatingA, relayEvidenceRatingA) < p.CurrentLimitA {
		result.addIssue(CodeSpeakerProtection, "relay", "speaker relay must be normally open, clamped, and have catalog contact-current evidence above the declared current limit", "select a sufficiently rated normally-open relay, declare no more than its catalog rating, and add its coil clamp")
	}
	if !positiveFinite(p.ZobelResistanceOhm) || !positiveFinite(p.ZobelCapacitanceF) {
		result.addIssue(CodeSpeakerProtection, "zobel", "a finite output Zobel resistance and capacitance are required", "add the reviewed output damping network")
	}
	return result.finish()
}

func componentMaximumRating(record *components.ComponentRecord, kind, unit string) (float64, bool) {
	if record == nil {
		return 0, false
	}
	for _, rating := range record.Ratings {
		if !strings.EqualFold(strings.TrimSpace(rating.Kind), kind) || !strings.EqualFold(strings.TrimSpace(rating.Unit), unit) {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(rating.Max), 64)
		if err == nil && positiveFinite(value) {
			return value, true
		}
	}
	return 0, false
}

func validateSpeakerLayout(request SpeakerAmplifierRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisLayout)
	l := request.Layout
	if !l.StarGround || !l.KelvinFeedback || !l.KelvinEmitterSense || !l.LocalRailDecoupling {
		result.addIssue(CodeSpeakerLayout, "return_and_sense", "star return, Kelvin feedback/sense, and local rail decoupling are mandatory", "apply the speaker-power layout profile")
	}
	if !l.ThermalBiasCoupling || !l.ComplementarySymmetry || !l.HeatsinkKeepout || !l.MountingAccess {
		result.addIssue(CodeSpeakerLayout, "thermal_and_mechanical", "thermal coupling, complementary symmetry, heatsink keepout, and mounting access are mandatory", "declare quantified placement groups and mechanical keepouts")
	}
	if !positiveFinite(l.CopperWidthMM) || !positiveFinite(l.MinimumCopperWidthMM) || l.CopperWidthMM < l.MinimumCopperWidthMM || !positiveFinite(l.CopperClearanceMM) || !positiveFinite(l.MinimumClearanceMM) || l.CopperClearanceMM < l.MinimumClearanceMM {
		result.addIssue(CodeSpeakerLayout, "power_copper", "high-current copper width or clearance is below its calculated minimum", "increase the applied net-class width/clearance")
	}
	if !positiveFinite(l.HighCurrentLoopLengthMM) || !positiveFinite(l.MaximumLoopLengthMM) || l.HighCurrentLoopLengthMM > l.MaximumLoopLengthMM || !positiveFinite(l.FeedbackSeparationMM) || !positiveFinite(l.MinimumFeedbackSeparationMM) || l.FeedbackSeparationMM < l.MinimumFeedbackSeparationMM {
		result.addIssue(CodeSpeakerLayout, "loop_geometry", "high-current loop length or feedback separation violates the declared geometry contract", "shorten the power loop or increase input/feedback separation")
	}
	return result.finish()
}

func availableSpeakerPeakV(request SpeakerAmplifierRequest) float64 {
	negativeRail, positiveRail, ok := validationRails(request.Envelope)
	if !ok {
		return math.NaN()
	}
	railLimited := math.Min(positiveRail-request.Envelope.OutputBiasV, request.Envelope.OutputBiasV-negativeRail) - request.OutputStageLossV
	return math.Min(railLimited, request.Protection.CurrentLimitA*request.TargetLoadOhm)
}

func speakerRailMagnitude(request ValidationRequest) float64 {
	negativeRail, positiveRail, ok := validationRails(request)
	if !ok {
		return math.NaN()
	}
	return math.Min(positiveRail-request.OutputBiasV, request.OutputBiasV-negativeRail)
}

func minimumLoadResistance(loads []SpeakerLoadCase) float64 {
	minimum := math.Inf(1)
	for _, load := range loads {
		if positiveFinite(load.ResistanceOhm) && load.ResistanceOhm < minimum {
			minimum = load.ResistanceOhm
		}
	}
	return minimum
}

func measurementName(name string) string {
	var b strings.Builder
	lastWasUnderscore := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastWasUnderscore = false
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !lastWasUnderscore {
				b.WriteByte('_')
			}
			b.WriteByte('u')
			b.WriteString(strconv.FormatInt(int64(r), 16))
			b.WriteByte('_')
			lastWasUnderscore = true
		} else if !lastWasUnderscore {
			b.WriteByte('_')
			lastWasUnderscore = true
		}
	}
	normalized := strings.Trim(b.String(), "_")
	if len(normalized) <= measurementNameMaxLength {
		return normalized
	}
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(normalized))
	suffix := "_" + strconv.FormatUint(digest.Sum64(), 16)
	return normalized[:measurementNameMaxLength-len(suffix)] + suffix
}
