package components

import (
	"context"
	"math"
	"testing"
)

func TestAudioPrecisionResistorOptionsMatchCatalogEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	records := make(map[string]ComponentRecord, len(catalog.Records))
	for _, record := range catalog.Records {
		records[record.ID] = record
	}
	for _, option := range AudioPrecisionResistorOptions() {
		record, ok := records[option.ComponentID]
		if !ok || record.Resistor == nil || record.Resistor.NominalResistance == nil || record.Resistor.ResistanceTolerancePct == nil {
			t.Fatalf("precision resistor option %#v has no complete catalog evidence", option)
		}
		if record.Resistor.NominalResistance.Value != option.NominalOhm || *record.Resistor.ResistanceTolerancePct != option.TolerancePercent {
			t.Fatalf("precision resistor option %#v disagrees with catalog evidence %#v", option, record.Resistor)
		}
	}
}

func TestFabricationProofResistorRequiresAppliedDeratingEvidence(t *testing.T) {
	tolerance := 5.0
	maximumTemperature := 250.0
	evidence := &ResistorEvidence{
		Technology:                 "wirewound",
		NominalResistance:          &EvidenceMeasurement{Value: 0.22, Unit: "ohm", Conditions: "ordered value at 20 C"},
		ResistanceTolerancePct:     &tolerance,
		RatedPower:                 &EvidenceMeasurement{Value: 3, Unit: "W", TemperatureC: float64Pointer(40), Conditions: "free-air ambient"},
		DeratedPower:               &EvidenceMeasurement{Value: 2.5, Unit: "W", TemperatureC: float64Pointer(70), Conditions: "datasheet curve"},
		MaximumElementTemperatureC: &maximumTemperature,
		PulseStatus:                reviewStatusNotApplicable,
		FabricationProof:           true,
	}
	if issues := validateResistorEvidence("component.resistor_evidence", false, evidence); len(issues) != 0 {
		t.Fatalf("valid resistor evidence issues = %#v", issues)
	}

	evidence.DeratedPower = nil
	issues := validateResistorEvidence("component.resistor_evidence", false, evidence)
	assertIssuePath(t, issues, "component.resistor_evidence.derated_power")
}

func TestResistorEvidenceRejectsNonfiniteAppliedValue(t *testing.T) {
	tolerance := math.NaN()
	evidence := &ResistorEvidence{ResistanceTolerancePct: &tolerance, PulseStatus: reviewStatusUnknown}
	issues := validateResistorEvidence("component.resistor_evidence", false, evidence)
	assertIssuePath(t, issues, "component.resistor_evidence.resistance_tolerance_percent")
}

func TestThermalDeratingAcceptsTypedResistorFabricationProof(t *testing.T) {
	record := ComponentRecord{
		ID:            "resistor.example.power",
		DeratingRules: []DeratingRule{{Kind: "thermal"}},
		Resistor:      &ResistorEvidence{FabricationProof: true, PulseStatus: reviewStatusNotApplicable},
	}
	if issues := fabricationCandidateReviewIssues(record); len(issues) != 0 {
		t.Fatalf("fabrication review issues = %#v", issues)
	}

	record.Resistor.FabricationProof = false
	issues := fabricationCandidateReviewIssues(record)
	assertIssuePath(t, issues, "component.resistor.example.power.derating_rules.thermal")
}

func float64Pointer(value float64) *float64 {
	return &value
}
