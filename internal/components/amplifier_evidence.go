package components

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
)

func validateEvidenceRange(path string, value *EvidenceRange, requireConditions bool) []reports.Issue {
	if value == nil {
		return nil
	}
	var issues []reports.Issue
	if strings.TrimSpace(value.Unit) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".unit", "quantitative evidence unit is required"))
	}
	if value.Minimum == nil && value.Typical == nil && value.Maximum == nil {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "quantitative evidence requires at least one bound"))
	}
	for suffix, number := range map[string]*float64{"minimum": value.Minimum, "typical": value.Typical, "maximum": value.Maximum} {
		if number != nil && (math.IsNaN(*number) || math.IsInf(*number, 0)) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "quantitative evidence must be finite"))
		}
	}
	if value.Minimum != nil && value.Typical != nil && *value.Minimum > *value.Typical {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".minimum", "minimum must not exceed typical"))
	}
	if value.Typical != nil && value.Maximum != nil && *value.Typical > *value.Maximum {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".typical", "typical must not exceed maximum"))
	}
	if value.Minimum != nil && value.Maximum != nil && *value.Minimum > *value.Maximum {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".minimum", "minimum must not exceed maximum"))
	}
	if requireConditions && strings.TrimSpace(value.Conditions) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".conditions", "fabrication-oriented quantitative evidence requires test conditions"))
	}
	return issues
}

func validateEvidenceMeasurement(path string, value *EvidenceMeasurement, requireConditions bool) []reports.Issue {
	if value == nil {
		return nil
	}
	var issues []reports.Issue
	if math.IsNaN(value.Value) || math.IsInf(value.Value, 0) || value.Value <= 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".value", "evidence measurement must be finite and positive"))
	}
	if strings.TrimSpace(value.Unit) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".unit", "evidence measurement unit is required"))
	}
	if value.FrequencyHz != nil && (math.IsNaN(*value.FrequencyHz) || math.IsInf(*value.FrequencyHz, 0) || *value.FrequencyHz <= 0) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".frequency_hz", "measurement frequency must be finite and positive"))
	}
	if value.TemperatureC != nil && (math.IsNaN(*value.TemperatureC) || math.IsInf(*value.TemperatureC, 0)) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".temperature_c", "measurement temperature must be finite"))
	}
	if requireConditions && strings.TrimSpace(value.Conditions) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".conditions", "fabrication-oriented measurement requires test conditions"))
	}
	return issues
}

func validateRailHeadroom(path string, value *RailHeadroomEvidence, requireConditions bool) []reports.Issue {
	if value == nil {
		return nil
	}
	var issues []reports.Issue
	if value.NegativeRailHeadroomV == nil || value.PositiveRailHeadroomV == nil {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "rail-referenced evidence requires both rail headrooms"))
	}
	fields := []struct {
		suffix string
		number *float64
	}{
		{suffix: "negative_rail_headroom_v", number: value.NegativeRailHeadroomV},
		{suffix: "positive_rail_headroom_v", number: value.PositiveRailHeadroomV},
	}
	for _, field := range fields {
		if field.number != nil && (math.IsNaN(*field.number) || math.IsInf(*field.number, 0) || *field.number < 0) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+field.suffix, "rail headroom must be finite and non-negative"))
		}
	}
	if requireConditions && strings.TrimSpace(value.Conditions) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".conditions", "fabrication-oriented rail headroom requires test conditions"))
	}
	return issues
}

func validatePowerSemiconductorEvidence(path string, record *ComponentRecord) []reports.Issue {
	evidence := record.PowerSemiconductor
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	switch evidence.DeviceClass {
	case "bjt":
		if record.Family != "bjt" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".device_class", "BJT power evidence requires a bjt-family record"))
		}
		if evidence.BJT == nil || evidence.MOSFET != nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".bjt", "BJT power evidence requires only BJT-specific evidence"))
		}
	case "mosfet":
		if record.Family != "mosfet" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".device_class", "MOSFET power evidence requires a mosfet-family record"))
		}
		if evidence.MOSFET == nil || evidence.BJT != nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".mosfet", "MOSFET power evidence requires only MOSFET-specific evidence"))
		}
	default:
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".device_class", "power semiconductor device_class must be bjt or mosfet"))
	}
	validPolarity := (evidence.DeviceClass == "bjt" && (evidence.Polarity == "npn" || evidence.Polarity == "pnp")) ||
		(evidence.DeviceClass == "mosfet" && (evidence.Polarity == "n_channel" || evidence.Polarity == "p_channel"))
	if !validPolarity {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".polarity", "power semiconductor polarity is incompatible with device_class"))
	}
	require := evidence.FabricationProof
	for suffix, measurement := range map[string]*EvidenceMeasurement{
		"max_voltage": evidence.MaxVoltage, "continuous_current": evidence.ContinuousCurrent,
		"peak_current": evidence.PeakCurrent, "power_dissipation": evidence.PowerDissipation,
	} {
		issues = append(issues, validateEvidenceMeasurement(path+"."+suffix, measurement, require)...)
		if require && measurement == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented power semiconductor evidence requires this rating"))
		}
	}
	for suffix, number := range map[string]*float64{
		"max_junction_temperature_c": evidence.MaxJunctionTemperatureC,
	} {
		if number != nil && (math.IsNaN(*number) || math.IsInf(*number, 0) || *number <= 0) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "thermal evidence must be finite and positive"))
		}
		if require && number == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented power semiconductor evidence requires this thermal limit"))
		}
	}
	if require && evidence.JunctionToCaseCPerW == nil && evidence.JunctionToAmbientCPerW == nil {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".thermal_resistance", "fabrication-oriented power semiconductor evidence requires junction-to-case or junction-to-ambient thermal resistance"))
	}
	if evidence.JunctionToCaseCPerW != nil && (math.IsNaN(*evidence.JunctionToCaseCPerW) || math.IsInf(*evidence.JunctionToCaseCPerW, 0) || *evidence.JunctionToCaseCPerW <= 0) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".junction_to_case_c_per_w", "thermal evidence must be finite and positive"))
	}
	if evidence.JunctionToAmbientCPerW != nil && (math.IsNaN(*evidence.JunctionToAmbientCPerW) || math.IsInf(*evidence.JunctionToAmbientCPerW, 0) || *evidence.JunctionToAmbientCPerW <= 0) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".junction_to_ambient_c_per_w", "thermal evidence must be finite and positive"))
	}
	if require && strings.TrimSpace(evidence.ComplementaryGroup) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".complementary_group", "fabrication-oriented amplifier output evidence requires a complementary group"))
	}
	if require && strings.TrimSpace(evidence.MountingAssumptions) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".mounting_assumptions", "fabrication-oriented thermal evidence requires mounting assumptions"))
	}
	for suffix, status := range map[string]string{
		"secondary_breakdown_status": evidence.SecondaryBreakdownStatus,
		"linear_mode_status":         evidence.LinearModeStatus,
	} {
		issues = append(issues, validateReviewStatus(path+"."+suffix, status, strings.ReplaceAll(suffix, "_", " "))...)
		if require && strings.TrimSpace(status) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented power semiconductor evidence requires this review status"))
		}
	}
	if evidence.JunctionToCaseCPerW != nil && evidence.JunctionToAmbientCPerW != nil && *evidence.JunctionToCaseCPerW > *evidence.JunctionToAmbientCPerW {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".junction_to_case_c_per_w", "junction-to-case thermal resistance must not exceed junction-to-ambient resistance"))
	}
	if record.AmplifierOutput != nil {
		if evidence.DeviceClass != record.AmplifierOutput.DeviceClass {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".device_class", "power semiconductor device_class must match amplifier output evidence"))
		}
		if evidence.Polarity != record.AmplifierOutput.Polarity {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".polarity", "power semiconductor polarity must match amplifier output evidence"))
		}
		if evidence.ComplementaryGroup != "" && record.AmplifierOutput.ComplementaryGroup != "" && evidence.ComplementaryGroup != record.AmplifierOutput.ComplementaryGroup {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".complementary_group", "power semiconductor complementary group must match amplifier output evidence"))
		}
	}
	issues = append(issues, validateSOA(path+".soa", evidence.SOA, require)...)
	issues = append(issues, validatePowerBJT(path+".bjt", evidence.BJT, require && evidence.DeviceClass == "bjt")...)
	issues = append(issues, validatePowerMOSFET(path+".mosfet", evidence.MOSFET, require && evidence.DeviceClass == "mosfet")...)
	return issues
}

func validateSOA(path string, points []SOAEnvelopePoint, required bool) []reports.Issue {
	var issues []reports.Issue
	type boundaryPoint struct {
		voltage float64
		current float64
		index   int
	}
	lastByBasis := map[string]boundaryPoint{}
	if required && len(points) < 2 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "fabrication-oriented SOA evidence requires at least two boundary points"))
	}
	for i, point := range points {
		pointPath := fmt.Sprintf("%s[%d]", path, i)
		if !isFinitePositive(point.VoltageV) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pointPath+".voltage_v", "SOA voltage must be finite and positive"))
		}
		if !isFinitePositive(point.CurrentA) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pointPath+".current_a", "SOA current must be finite and positive"))
		}
		if point.DC == (point.PulseDurationS != nil) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pointPath, "SOA point requires exactly one of dc or pulse_duration_s"))
		}
		if point.PulseDurationS != nil && !isFinitePositive(*point.PulseDurationS) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pointPath+".pulse_duration_s", "SOA pulse duration must be finite and positive"))
		}
		if math.IsNaN(point.CaseTemperatureC) || math.IsInf(point.CaseTemperatureC, 0) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pointPath+".case_temperature_c", "SOA case temperature must be finite"))
		}
		basis := "dc"
		if point.PulseDurationS != nil {
			basis = fmt.Sprintf("pulse:%g", *point.PulseDurationS)
		}
		if previous, ok := lastByBasis[basis]; ok {
			if point.VoltageV <= previous.voltage {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pointPath+".voltage_v", fmt.Sprintf("SOA boundary voltage must increase within %s points after %s[%d]", basis, path, previous.index)))
			}
			if point.CurrentA > previous.current {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pointPath+".current_a", fmt.Sprintf("SOA boundary current must not increase with voltage within %s points", basis)))
			}
		}
		lastByBasis[basis] = boundaryPoint{voltage: point.VoltageV, current: point.CurrentA, index: i}
	}
	return issues
}

func validatePowerBJT(path string, evidence *PowerBJTEvidence, required bool) []reports.Issue {
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	for suffix, number := range map[string]*float64{"gain_minimum": evidence.GainMinimum, "gain_test_current_a": evidence.GainTestCurrentA, "transition_frequency_hz": evidence.TransitionFreqHz} {
		if number != nil && !isFinitePositive(*number) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "BJT evidence must be finite and positive"))
		}
		if required && number == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented BJT evidence requires this value"))
		}
	}
	return issues
}

func validatePowerMOSFET(path string, evidence *PowerMOSFETEvidence, required bool) []reports.Issue {
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	values := map[string]*float64{
		"rds_on_ohm": evidence.RDSOnOhm, "rds_on_gate_voltage_v": evidence.RDSOnGateVoltageV,
		"threshold_minimum_v": evidence.ThresholdMinimumV, "threshold_maximum_v": evidence.ThresholdMaximumV,
		"transconductance_s": evidence.TransconductanceS, "total_gate_charge_c": evidence.TotalGateChargeC,
		"input_capacitance_f": evidence.InputCapacitanceF, "reverse_transfer_capacitance_f": evidence.ReverseTransferCapF,
	}
	for suffix, number := range values {
		if number != nil && !isFinitePositive(*number) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "MOSFET evidence must be finite and positive"))
		}
		if required && number == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented MOSFET evidence requires this value"))
		}
	}
	if evidence.ThresholdMinimumV != nil && evidence.ThresholdMaximumV != nil && *evidence.ThresholdMinimumV > *evidence.ThresholdMaximumV {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".threshold_minimum_v", "MOSFET threshold minimum must not exceed maximum"))
	}
	for suffix, status := range map[string]string{"body_diode_status": evidence.BodyDiodeStatus, "gate_protection_status": evidence.GateProtectionStatus} {
		issues = append(issues, validateReviewStatus(path+"."+suffix, status, strings.ReplaceAll(suffix, "_", " "))...)
		if required && strings.TrimSpace(status) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented MOSFET evidence requires this review status"))
		}
	}
	return issues
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
