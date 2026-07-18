package components

import (
	"math"
	"strings"

	"kicadai/internal/reports"
)

type PrecisionResistorOption struct {
	ComponentID      string
	NominalOhm       float64
	TolerancePercent float64
}

var audioPrecisionResistorOptions = []PrecisionResistorOption{
	{ComponentID: "resistor.vishay.tnpw0805.10k0.0p1", NominalOhm: 10000, TolerancePercent: 0.1},
	{ComponentID: "resistor.vishay.tnpw0805.11k7.0p1", NominalOhm: 11700, TolerancePercent: 0.1},
	{ComponentID: "resistor.vishay.tnpw0805.12k5.0p1", NominalOhm: 12500, TolerancePercent: 0.1},
	{ComponentID: "resistor.vishay.tnpw0805.47k0.0p1", NominalOhm: 47000, TolerancePercent: 0.1},
}

// AudioPrecisionResistorOptions is the shared inventory contract for concrete
// precision resistors used by audio blocks. Callers receive a copy so block
// selection and catalog validation cannot mutate one another's evidence.
func AudioPrecisionResistorOptions() []PrecisionResistorOption {
	return append([]PrecisionResistorOption(nil), audioPrecisionResistorOptions...)
}

func validateResistorEvidence(path string, generic bool, evidence *ResistorEvidence) []reports.Issue {
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	issues = append(issues, validateEvidenceMeasurement(path+".nominal_resistance", evidence.NominalResistance, evidence.FabricationProof)...)
	issues = append(issues, validateEvidenceMeasurement(path+".rated_power", evidence.RatedPower, evidence.FabricationProof)...)
	issues = append(issues, validateEvidenceMeasurement(path+".derated_power", evidence.DeratedPower, evidence.FabricationProof)...)
	if evidence.ResistanceTolerancePct != nil && (!finitePositive(*evidence.ResistanceTolerancePct) || *evidence.ResistanceTolerancePct > 100) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".resistance_tolerance_percent", "resistance tolerance must be finite, greater than zero, and at most 100 percent"))
	}
	if evidence.MaximumElementTemperatureC != nil && (math.IsNaN(*evidence.MaximumElementTemperatureC) || math.IsInf(*evidence.MaximumElementTemperatureC, 0) || *evidence.MaximumElementTemperatureC <= -273.15) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".maximum_element_temperature_c", "maximum element temperature must be finite and above absolute zero"))
	}
	issues = append(issues, validateReviewStatus(path+".pulse_status", evidence.PulseStatus, "resistor pulse")...)
	if generic && evidence.FabricationProof {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".fabrication_proof", "generic resistor records cannot carry fabrication proof"))
	}
	if evidence.FabricationProof {
		if strings.TrimSpace(evidence.Technology) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".technology", "fabrication-proof resistor evidence requires technology"))
		}
		for suffix, missing := range map[string]bool{
			"nominal_resistance":            evidence.NominalResistance == nil,
			"resistance_tolerance_percent":  evidence.ResistanceTolerancePct == nil,
			"rated_power":                   evidence.RatedPower == nil,
			"derated_power":                 evidence.DeratedPower == nil,
			"maximum_element_temperature_c": evidence.MaximumElementTemperatureC == nil,
		} {
			if missing {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-proof resistor evidence requires "+strings.ReplaceAll(suffix, "_", " ")))
			}
		}
	}
	return issues
}

func finitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
