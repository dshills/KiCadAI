package amplifiers

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	CodeAmplifierDCHeadroom      reports.Code = "AMPLIFIER_DC_HEADROOM"
	CodeAmplifierACBandwidth     reports.Code = "AMPLIFIER_AC_BANDWIDTH"
	CodeAmplifierTransientSlew   reports.Code = "AMPLIFIER_TRANSIENT_SLEW"
	CodeAmplifierToleranceCorner reports.Code = "AMPLIFIER_TOLERANCE_CORNER"
	CodeAmplifierStabilityMargin reports.Code = "AMPLIFIER_STABILITY_MARGIN"
	CodeAmplifierThermalLimit    reports.Code = "AMPLIFIER_THERMAL_LIMIT"
	CodeAmplifierSOALimit        reports.Code = "AMPLIFIER_SOA_LIMIT"
)

type AnalysisKind string

const (
	AnalysisDC        AnalysisKind = "dc"
	AnalysisAC        AnalysisKind = "ac"
	AnalysisTransient AnalysisKind = "transient"
	AnalysisTolerance AnalysisKind = "tolerance"
	AnalysisStability AnalysisKind = "stability"
	AnalysisThermal   AnalysisKind = "thermal"
	AnalysisSOA       AnalysisKind = "soa"
)

type OperatingCorner struct {
	Name               string  `json:"name"`
	OutputBiasV        float64 `json:"output_bias_v"`
	QuiescentCurrentA  float64 `json:"quiescent_current_a"`
	DeviceDissipationW float64 `json:"device_dissipation_w"`
	PhaseMarginDeg     float64 `json:"phase_margin_deg"`
}

type ValidationRequest struct {
	Topology                 string                      `json:"topology"`
	SupplyVoltageV           float64                     `json:"supply_voltage_v"`
	NegativeRailVoltageV     float64                     `json:"negative_rail_voltage_v,omitempty"`
	PositiveRailVoltageV     float64                     `json:"positive_rail_voltage_v,omitempty"`
	OutputBiasV              float64                     `json:"output_bias_v"`
	QuiescentCurrentA        float64                     `json:"quiescent_current_a"`
	OutputPeakVoltageV       float64                     `json:"output_peak_voltage_v"`
	OutputPeakCurrentA       float64                     `json:"output_peak_current_a"`
	LoadImpedanceOhm         float64                     `json:"load_impedance_ohm"`
	MaximumSignalFrequencyHz float64                     `json:"maximum_signal_frequency_hz"`
	ClosedLoopGain           float64                     `json:"closed_loop_gain"`
	NoiseGain                float64                     `json:"noise_gain"`
	GainBandwidthHz          float64                     `json:"gain_bandwidth_hz"`
	SlewRateVPerS            float64                     `json:"slew_rate_v_per_s"`
	PhaseMarginDeg           float64                     `json:"phase_margin_deg"`
	DeviceDissipationW       float64                     `json:"device_dissipation_w"`
	AmbientTemperatureC      float64                     `json:"ambient_temperature_c"`
	CaseToSinkCPerW          float64                     `json:"case_to_sink_c_per_w,omitempty"`
	SinkToAmbientCPerW       float64                     `json:"sink_to_ambient_c_per_w,omitempty"`
	SOAVoltageV              float64                     `json:"soa_voltage_v"`
	SOACurrentA              float64                     `json:"soa_current_a"`
	SOATemperatureC          float64                     `json:"soa_temperature_c"`
	Device                   *components.ComponentRecord `json:"device,omitempty"`
	ToleranceCorners         []OperatingCorner           `json:"tolerance_corners,omitempty"`
}

type AnalysisResult struct {
	Kind         AnalysisKind       `json:"kind"`
	Pass         bool               `json:"pass"`
	Measurements map[string]float64 `json:"measurements,omitempty"`
	Issues       []reports.Issue    `json:"issues,omitempty"`
}

type ValidationResult struct {
	Pass     bool             `json:"pass"`
	Analyses []AnalysisResult `json:"analyses"`
	Issues   []reports.Issue  `json:"issues,omitempty"`
}

func ValidateOperatingEnvelope(request ValidationRequest) ValidationResult {
	analyses := []AnalysisResult{
		validateDC(request),
		validateAC(request),
		validateTransient(request),
		validateTolerance(request),
		validateStability(request),
		validateThermal(request),
		validateSOA(request),
	}
	result := ValidationResult{Pass: true, Analyses: analyses}
	for _, analysis := range analyses {
		result.Issues = append(result.Issues, analysis.Issues...)
		result.Pass = result.Pass && analysis.Pass
	}
	return result
}

func validateDC(request ValidationRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisDC)
	negativeRail, positiveRail, railsOK := validationRails(request)
	lowerHeadroom := request.OutputBiasV - request.OutputPeakVoltageV - negativeRail
	upperHeadroom := positiveRail - request.OutputBiasV - request.OutputPeakVoltageV
	result.addMeasurement("lower_headroom_v", lowerHeadroom)
	result.addMeasurement("upper_headroom_v", upperHeadroom)
	if !railsOK || !positiveFinite(request.QuiescentCurrentA) || request.OutputBiasV <= negativeRail || request.OutputBiasV >= positiveRail {
		result.addIssue(CodeAmplifierDCHeadroom, "operating_point", "DC operating point requires positive supply/current and output bias strictly inside the rails", "reduce the requested swing or rebias the stage")
	}
	if !positiveFinite(lowerHeadroom) || !positiveFinite(upperHeadroom) {
		result.addIssue(CodeAmplifierDCHeadroom, "output_headroom", "requested peak output swing reaches a supply rail", "increase the supply/headroom or reduce output swing")
	}
	return result.finish()
}

func validateAC(request ValidationRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisAC)
	requiredGBW := request.NoiseGain * request.MaximumSignalFrequencyHz * 10
	result.addMeasurement("required_gain_bandwidth_hz", requiredGBW)
	result.addMeasurement("available_gain_bandwidth_hz", request.GainBandwidthHz)
	if !positiveFinite(request.MaximumSignalFrequencyHz) || !positiveFinite(request.ClosedLoopGain) || !positiveFinite(request.NoiseGain) || request.NoiseGain < request.ClosedLoopGain {
		result.addIssue(CodeAmplifierACBandwidth, "gain_contract", "AC validation requires positive frequency/gain and noise gain at least the closed-loop gain", "provide the bounded signal band and noise-gain contract")
	}
	if !positiveFinite(request.GainBandwidthHz) || request.GainBandwidthHz < requiredGBW {
		result.addIssue(CodeAmplifierACBandwidth, "gain_bandwidth", "gain-bandwidth product does not preserve the required tenfold loop-gain margin", "select a faster device or reduce gain/bandwidth")
	}
	return result.finish()
}

func validateTransient(request ValidationRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisTransient)
	requiredSlew := 2 * math.Pi * request.MaximumSignalFrequencyHz * request.OutputPeakVoltageV
	result.addMeasurement("required_slew_rate_v_per_s", requiredSlew)
	result.addMeasurement("available_slew_rate_v_per_s", request.SlewRateVPerS)
	calculatedPeakCurrent := 0.0
	loadOK := positiveFinite(request.LoadImpedanceOhm)
	if loadOK {
		calculatedPeakCurrent = request.OutputPeakVoltageV / request.LoadImpedanceOhm
		result.addMeasurement("calculated_peak_current_a", calculatedPeakCurrent)
	}
	if !positiveFinite(request.SlewRateVPerS) || request.SlewRateVPerS < requiredSlew {
		result.addIssue(CodeAmplifierTransientSlew, "slew_rate", "available slew rate is below the sinusoidal full-power requirement", "reduce output amplitude/bandwidth or select a faster stage")
	}
	if !loadOK || !positiveFinite(request.OutputPeakCurrentA) || !nonNegativeFinite(calculatedPeakCurrent) || calculatedPeakCurrent > request.OutputPeakCurrentA*(1+1e-9) {
		result.addIssue(CodeAmplifierTransientSlew, "output_current", "declared peak-current envelope does not cover the requested load transient", "raise verified drive capability or reduce output voltage/load demand")
	}
	return result.finish()
}

func validateTolerance(request ValidationRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisTolerance)
	negativeRail, positiveRail, railsOK := validationRails(request)
	result.addMeasurement("corner_count", float64(len(request.ToleranceCorners)))
	if len(request.ToleranceCorners) < 2 {
		result.addIssue(CodeAmplifierToleranceCorner, "corners", "fabrication validation requires at least two named tolerance/temperature corners", "register conservative cold/min and hot/max corners")
		return result.finish()
	}
	seen := map[string]bool{}
	for _, corner := range request.ToleranceCorners {
		name := strings.TrimSpace(corner.Name)
		if name == "" || seen[name] {
			result.addIssue(CodeAmplifierToleranceCorner, "corners", "tolerance corner names must be non-empty and unique", "assign deterministic unique corner names")
			continue
		}
		seen[name] = true
		if !railsOK || corner.OutputBiasV-request.OutputPeakVoltageV <= negativeRail || corner.OutputBiasV+request.OutputPeakVoltageV >= positiveRail || !positiveFinite(corner.QuiescentCurrentA) || !nonNegativeFinite(corner.DeviceDissipationW) || corner.PhaseMarginDeg < 45 {
			result.addIssue(CodeAmplifierToleranceCorner, "corner."+name, "tolerance corner violates headroom, idle-current, dissipation, or phase-margin bounds", "adjust component tolerance/temperature assumptions or operating point")
		}
	}
	return result.finish()
}

func validationRails(request ValidationRequest) (float64, float64, bool) {
	negativeRail := request.NegativeRailVoltageV
	positiveRail := request.PositiveRailVoltageV
	if negativeRail == 0 && positiveRail == 0 {
		positiveRail = request.SupplyVoltageV
	}
	if !finiteNumber(negativeRail) || !finiteNumber(positiveRail) || positiveRail <= negativeRail || !positiveFinite(request.SupplyVoltageV) || math.Abs((positiveRail-negativeRail)-request.SupplyVoltageV) > 1e-9 {
		return 0, 0, false
	}
	return negativeRail, positiveRail, true
}

func validateStability(request ValidationRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisStability)
	result.addMeasurement("phase_margin_deg", request.PhaseMarginDeg)
	if !positiveFinite(request.PhaseMarginDeg) || request.PhaseMarginDeg < 45 {
		result.addIssue(CodeAmplifierStabilityMargin, "phase_margin", "phase margin is below the 45 degree promotion floor", "revise compensation, feedback layout, or capacitive-load isolation")
	}
	return result.finish()
}

func validateThermal(request ValidationRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisThermal)
	evidence := powerEvidence(request.Device)
	if evidence == nil || evidence.MaxJunctionTemperatureC == nil {
		result.addIssue(CodeAmplifierThermalLimit, "device_evidence", "typed junction-temperature and thermal-path evidence is required", "select a fabrication-proven semiconductor record")
		return result.finish()
	}
	theta := 0.0
	if evidence.JunctionToCaseCPerW != nil && request.SinkToAmbientCPerW > 0 {
		theta = *evidence.JunctionToCaseCPerW + request.CaseToSinkCPerW + request.SinkToAmbientCPerW
	} else if evidence.JunctionToAmbientCPerW != nil {
		theta = *evidence.JunctionToAmbientCPerW
	}
	if !positiveFinite(theta) || !nonNegativeFinite(request.DeviceDissipationW) {
		result.addIssue(CodeAmplifierThermalLimit, "thermal_path", "a positive complete thermal resistance and non-negative dissipation are required", "declare board or heatsink thermal path")
		return result.finish()
	}
	junction := request.AmbientTemperatureC + request.DeviceDissipationW*theta
	result.addMeasurement("thermal_resistance_c_per_w", theta)
	result.addMeasurement("junction_temperature_c", junction)
	result.addMeasurement("junction_margin_c", *evidence.MaxJunctionTemperatureC-junction)
	if !finiteNumber(junction) || junction >= *evidence.MaxJunctionTemperatureC {
		result.addIssue(CodeAmplifierThermalLimit, "junction_temperature", "calculated junction temperature reaches or exceeds the device limit", "reduce dissipation or improve the declared thermal path")
	}
	if evidence.PowerDissipation == nil || request.DeviceDissipationW > evidence.PowerDissipation.Value*(1+1e-9) {
		result.addIssue(CodeAmplifierThermalLimit, "device_dissipation", "calculated device dissipation exceeds the catalog rating", "reduce bias/output demand or choose a higher-rated device")
	}
	return result.finish()
}

func validateSOA(request ValidationRequest) AnalysisResult {
	result := newAnalysisResult(AnalysisSOA)
	evidence := powerEvidence(request.Device)
	if evidence == nil || !evidence.FabricationProof || len(evidence.SOA) < 2 {
		result.addIssue(CodeAmplifierSOALimit, "device_evidence", "fabrication-proven semiconductor SOA evidence is required", "select a device with a reviewed machine-readable SOA boundary")
		return result.finish()
	}
	allowed, basisTemperature, ok := interpolateDCAllowedCurrent(evidence.SOA, request.SOAVoltageV)
	if !ok {
		result.addIssue(CodeAmplifierSOALimit, "voltage", "SOA operating voltage lies outside the reviewed DC boundary", "reduce device voltage or extend reviewed device evidence")
		return result.finish()
	}
	if evidence.MaxJunctionTemperatureC != nil && request.SOATemperatureC > basisTemperature {
		denominator := *evidence.MaxJunctionTemperatureC - basisTemperature
		if denominator <= 0 {
			allowed = 0
		} else {
			allowed *= math.Max(0, (*evidence.MaxJunctionTemperatureC-request.SOATemperatureC)/denominator)
		}
	}
	result.addMeasurement("allowed_current_a", allowed)
	result.addMeasurement("requested_current_a", request.SOACurrentA)
	if request.SOACurrentA > 0 {
		result.addMeasurement("soa_current_margin_ratio", allowed/request.SOACurrentA)
	}
	if !positiveFinite(request.SOACurrentA) || allowed < request.SOACurrentA*(1-1e-9) {
		result.addIssue(CodeAmplifierSOALimit, "current", "requested device current exceeds the temperature-derated DC SOA boundary", "reduce voltage/current or select a device with greater linear SOA")
	}
	return result.finish()
}

func interpolateDCAllowedCurrent(points []components.SOAEnvelopePoint, voltage float64) (float64, float64, bool) {
	dc := make([]components.SOAEnvelopePoint, 0, len(points))
	for _, point := range points {
		if point.DC {
			dc = append(dc, point)
		}
	}
	sort.Slice(dc, func(i, j int) bool { return dc[i].VoltageV < dc[j].VoltageV })
	if len(dc) < 2 || !positiveFinite(voltage) || voltage < dc[0].VoltageV || voltage > dc[len(dc)-1].VoltageV {
		return 0, 0, false
	}
	for i := 1; i < len(dc); i++ {
		if voltage > dc[i].VoltageV {
			continue
		}
		left, right := dc[i-1], dc[i]
		if !positiveFinite(left.VoltageV) || !positiveFinite(right.VoltageV) || right.VoltageV <= left.VoltageV || !positiveFinite(left.CurrentA) || !positiveFinite(right.CurrentA) {
			return 0, 0, false
		}
		logVoltageSpan := math.Log(right.VoltageV / left.VoltageV)
		if !positiveFinite(logVoltageSpan) {
			return 0, 0, false
		}
		fraction := math.Log(voltage/left.VoltageV) / logVoltageSpan
		current := math.Exp(math.Log(left.CurrentA) + fraction*(math.Log(right.CurrentA)-math.Log(left.CurrentA)))
		temperature := left.CaseTemperatureC + fraction*(right.CaseTemperatureC-left.CaseTemperatureC)
		return current, temperature, finiteNumber(current) && current > 0
	}
	return dc[len(dc)-1].CurrentA, dc[len(dc)-1].CaseTemperatureC, true
}

func powerEvidence(device *components.ComponentRecord) *components.PowerSemiconductorEvidence {
	if device == nil {
		return nil
	}
	return device.PowerSemiconductor
}

func newAnalysisResult(kind AnalysisKind) AnalysisResult {
	return AnalysisResult{Kind: kind, Pass: true, Measurements: map[string]float64{}}
}

func (result *AnalysisResult) addMeasurement(name string, value float64) {
	if result == nil || !finiteNumber(value) {
		return
	}
	result.Measurements[name] = value
}

func (result *AnalysisResult) addIssue(code reports.Code, path string, message string, suggestion string) {
	result.Pass = false
	result.Issues = append(result.Issues, reports.Issue{Code: code, Severity: reports.SeverityError, Path: "amplifier." + string(result.Kind) + "." + path, Message: message, Suggestion: suggestion})
}

func (result AnalysisResult) finish() AnalysisResult {
	result.Pass = len(result.Issues) == 0
	return result
}

func positiveFinite(value float64) bool {
	return finiteNumber(value) && value > 0
}

func nonNegativeFinite(value float64) bool {
	return finiteNumber(value) && value >= 0
}

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func FormatValidationFailure(result ValidationResult) string {
	if result.Pass {
		return "pass"
	}
	parts := make([]string, 0, len(result.Issues))
	for _, issue := range result.Issues {
		parts = append(parts, fmt.Sprintf("%s:%s", issue.Code, issue.Path))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
