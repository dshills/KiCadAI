package amplifiers

import (
	"math"
	"strconv"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	CodeAmplifierOpAmpEnvelope     reports.Code = "AMPLIFIER_OPAMP_ENVELOPE"
	CodeAmplifierCapacitorEnvelope reports.Code = "AMPLIFIER_CAPACITOR_ENVELOPE"
)

var engineeringPrefixes = [...]struct {
	suffix     string
	multiplier float64
}{
	{suffix: "p", multiplier: 1e-12},
	{suffix: "n", multiplier: 1e-9},
	{suffix: "u", multiplier: 1e-6},
	{suffix: "µ", multiplier: 1e-6},
	{suffix: "m", multiplier: 1e-3},
	{suffix: "k", multiplier: 1e3},
	{suffix: "K", multiplier: 1e3},
	{suffix: "M", multiplier: 1e6},
	{suffix: "G", multiplier: 1e9},
}

type OpAmpApplication struct {
	// SupplyVoltageV is the total rail-to-rail supply span. For the common
	// single-supply case the rails default to 0 V and SupplyVoltageV. Set both
	// rail voltages to describe a dual-supply application explicitly.
	SupplyVoltageV          float64
	NegativeRailVoltageV    float64
	PositiveRailVoltageV    float64
	InputMinimumV           float64
	InputMaximumV           float64
	OutputMinimumV          float64
	OutputMaximumV          float64
	OutputCurrentA          float64
	RequiredGainBandwidthHz float64
	RequiredSlewRateVPerS   float64
	RequireFabricationProof bool
}

type CapacitorApplication struct {
	AppliedVoltageV         float64
	RippleCurrentA          float64
	MaximumESROhm           float64
	RequiredCapacitanceF    float64
	ExpectedPolarity        string
	RequireFabricationProof bool
}

func ValidateOpAmpApplication(record *components.ComponentRecord, application OpAmpApplication) []reports.Issue {
	if record == nil || record.OpAmp == nil {
		return []reports.Issue{amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.evidence", "typed op-amp evidence is required", "select a concrete op-amp with reviewed quantitative evidence")}
	}
	evidence := record.OpAmp
	var issues []reports.Issue
	negativeRail, positiveRail, railsOK := applicationRails(application)
	supplySpan := positiveRail - negativeRail
	if application.RequireFabricationProof && !evidence.FabricationProof {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.fabrication_proof", "fabrication-oriented use requires explicit op-amp proof", "select a proven device or lower the requested acceptance"))
	}
	if !railsOK || evidence.SupplyVoltage == nil || !rangeContains(evidence.SupplyVoltage, supplySpan) {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.supply_voltage", "requested supply lies outside the reviewed operating range", "select a compatible op-amp or change the supply"))
	}
	if !railsOK || !railRangeContains(evidence.InputCommonMode, negativeRail, positiveRail, application.InputMinimumV, application.InputMaximumV) {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.input_common_mode", "signal common-mode range exceeds the reviewed rail headroom", "rebias the input or select a rail-compatible op-amp"))
	}
	if !railsOK || !railRangeContains(evidence.OutputSwing, negativeRail, positiveRail, application.OutputMinimumV, application.OutputMaximumV) {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.output_swing", "requested output swing exceeds the reviewed rail headroom", "reduce output swing or select a more suitable op-amp"))
	}
	if evidence.OutputCurrent == nil || !positiveFinite(application.OutputCurrentA) || application.OutputCurrentA > evidence.OutputCurrent.Value {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.output_current", "requested output current exceeds the reviewed drive evidence", "buffer the load or select a stronger driver"))
	}
	if evidence.GainBandwidth == nil || !positiveFinite(application.RequiredGainBandwidthHz) || application.RequiredGainBandwidthHz > evidence.GainBandwidth.Value {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.gain_bandwidth", "required gain-bandwidth exceeds the reviewed evidence", "reduce noise gain/bandwidth or select a faster op-amp"))
	}
	if evidence.SlewRate == nil || !positiveFinite(application.RequiredSlewRateVPerS) || application.RequiredSlewRateVPerS > evidence.SlewRate.Value {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierOpAmpEnvelope, "opamp.slew_rate", "required slew rate exceeds the reviewed evidence", "reduce full-power bandwidth or select a faster op-amp"))
	}
	return issues
}

func ValidateCapacitorApplication(record *components.ComponentRecord, application CapacitorApplication) []reports.Issue {
	if record == nil || record.Capacitor == nil {
		return []reports.Issue{amplifierApplicationIssue(CodeAmplifierCapacitorEnvelope, "capacitor.evidence", "typed capacitor evidence is required", "select a concrete capacitor with applied voltage, ESR, ripple, and polarity evidence")}
	}
	evidence := record.Capacitor
	var issues []reports.Issue
	if application.RequireFabricationProof && !evidence.FabricationProof {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierCapacitorEnvelope, "capacitor.fabrication_proof", "fabrication-oriented use requires explicit capacitor proof", "select a proven capacitor or lower the requested acceptance"))
	}
	voltageRating, voltageOK := engineeringValue(evidence.VoltageRating, evidence.VoltageUnit)
	if !voltageOK || !nonNegativeFinite(application.AppliedVoltageV) || application.AppliedVoltageV > voltageRating {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierCapacitorEnvelope, "capacitor.voltage", "applied voltage exceeds the reviewed rating", "increase voltage rating or reduce applied voltage"))
	}
	capacitance, capacitanceOK := engineeringValue(evidence.NominalCapacitance, evidence.CapacitanceUnit)
	if !capacitanceOK || !positiveFinite(application.RequiredCapacitanceF) || capacitance*(1-math.Abs(pointerValue(evidence.CapacitanceTolerancePct))/100) < application.RequiredCapacitanceF {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierCapacitorEnvelope, "capacitor.effective_capacitance", "worst-case nominal capacitance does not cover the required value", "increase capacitance or use verified effective-capacitance evidence"))
	}
	if evidence.ESR == nil || !nonNegativeFinite(application.MaximumESROhm) || evidence.ESR.Value > application.MaximumESROhm {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierCapacitorEnvelope, "capacitor.esr", "reviewed ESR exceeds the application limit", "select a lower-ESR capacitor or relax the circuit requirement with stability evidence"))
	}
	if evidence.RippleCurrent == nil || !nonNegativeFinite(application.RippleCurrentA) || application.RippleCurrentA > evidence.RippleCurrent.Value {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierCapacitorEnvelope, "capacitor.ripple_current", "application ripple current exceeds the reviewed rating", "select a higher-ripple capacitor or reduce ripple current"))
	}
	expectedPolarity := strings.TrimSpace(application.ExpectedPolarity)
	if expectedPolarity != "" && strings.TrimSpace(evidence.Polarity) != expectedPolarity {
		issues = append(issues, amplifierApplicationIssue(CodeAmplifierCapacitorEnvelope, "capacitor.polarity", "capacitor polarity contract does not match the application", "select the correct polarized or non-polarized technology"))
	}
	return issues
}

func amplifierApplicationIssue(code reports.Code, path, message, suggestion string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityError, Path: "amplifier." + path, Message: message, Suggestion: suggestion}
}

func rangeContains(value *components.EvidenceRange, target float64) bool {
	if value == nil || !positiveFinite(target) {
		return false
	}
	return (value.Minimum == nil || target >= *value.Minimum) && (value.Maximum == nil || target <= *value.Maximum)
}

func applicationRails(application OpAmpApplication) (float64, float64, bool) {
	negativeRail := application.NegativeRailVoltageV
	positiveRail := application.PositiveRailVoltageV
	if negativeRail == 0 && positiveRail == 0 {
		positiveRail = application.SupplyVoltageV
	}
	if !finiteNumber(negativeRail) || !finiteNumber(positiveRail) || positiveRail <= negativeRail {
		return 0, 0, false
	}
	return negativeRail, positiveRail, true
}

func railRangeContains(value *components.RailHeadroomEvidence, negativeRail, positiveRail, minimum, maximum float64) bool {
	if value == nil || value.NegativeRailHeadroomV == nil || value.PositiveRailHeadroomV == nil || !finiteNumber(negativeRail) || !finiteNumber(positiveRail) || positiveRail <= negativeRail || !finiteNumber(minimum) || !finiteNumber(maximum) || minimum > maximum {
		return false
	}
	return minimum >= negativeRail+*value.NegativeRailHeadroomV && maximum <= positiveRail-*value.PositiveRailHeadroomV
}

func pointerValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func engineeringValue(raw, unit string) (float64, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	valueMultiplier := 1.0
	hasValuePrefix := false
	for _, prefix := range engineeringPrefixes {
		if strings.HasSuffix(value, prefix.suffix) {
			value = strings.TrimSuffix(value, prefix.suffix)
			valueMultiplier = prefix.multiplier
			hasValuePrefix = true
			break
		}
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || !positiveFinite(number) {
		return 0, false
	}
	unitMultiplier := 1.0
	switch strings.TrimSpace(unit) {
	case "pV", "pA", "pF", "pOhm", "pohm", "pΩ":
		unitMultiplier = 1e-12
	case "nV", "nA", "nF", "nOhm", "nohm", "nΩ":
		unitMultiplier = 1e-9
	case "mV", "mA", "mF", "mOhm", "mohm", "mΩ":
		unitMultiplier = 1e-3
	case "uV", "uA", "uF", "uOhm", "uohm", "uΩ", "µV", "µA", "µF", "µOhm", "µohm", "µΩ":
		unitMultiplier = 1e-6
	case "kV", "kA", "kOhm", "kohm", "kΩ":
		unitMultiplier = 1e3
	case "MV", "MA", "MOhm", "MΩ":
		unitMultiplier = 1e6
	case "GV", "GA", "GOhm", "GΩ":
		unitMultiplier = 1e9
	}
	// Catalog values normally put the engineering prefix in either the raw
	// value ("220u" + "F") or the unit ("220" + "uF"). Accept a matching
	// redundant prefix once, but reject conflicting prefixes as ambiguous.
	if hasValuePrefix && unitMultiplier != 1 {
		if valueMultiplier != unitMultiplier {
			return 0, false
		}
		unitMultiplier = 1
	}
	return number * valueMultiplier * unitMultiplier, true
}
