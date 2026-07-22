package components

import (
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestRegulatorDynamicEvidenceSupportsFabricationProof(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Regulator = validDynamicRegulatorEvidence()
	if result := ValidateCatalog(&catalog); !result.OK {
		t.Fatalf("valid regulator dynamic evidence issues = %#v", result.Issues)
	}
}

func TestRegulatorDynamicEvidenceFailsClosedWithoutStartup(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Regulator = validDynamicRegulatorEvidence()
	catalog.Records[0].Regulator.StartupTime = nil
	result := ValidateCatalog(&catalog)
	if !slices.ContainsFunc(result.Issues, func(issue reports.Issue) bool {
		return issue.Path == "records[0].regulator_evidence.startup_time" && issue.Severity == reports.SeverityBlocked
	}) {
		t.Fatalf("incomplete regulator evidence issues = %#v", result.Issues)
	}
}

func validDynamicRegulatorEvidence() *RegulatorEvidence {
	return &RegulatorEvidence{
		OutputCapacitor: &RegulatorCapacitorStability{
			Kind: "ceramic_stable", MinCapacitance: "1", MaxCapacitance: "22", CapacitanceUnit: "uF",
			AcceptedDielectrics: []string{"X7R"}, ProofStatus: "proven",
		},
		StartupTime:                &EvidenceMeasurement{Value: 1e-3, Unit: "s", Conditions: "rated input and load"},
		SoftStartStatus:            "proven",
		StartupMonotonicStatus:     "proven",
		MaximumInrushCurrent:       &EvidenceMeasurement{Value: .2, Unit: "A", Conditions: "rated input and output capacitor"},
		QuiescentCurrent:           &EvidenceMeasurement{Value: 100e-6, Unit: "A", Conditions: "enabled, no load"},
		Efficiency:                 &EvidenceRange{Minimum: float64Pointer(.8), Maximum: float64Pointer(.95), Unit: "ratio", Conditions: "rated input and load range"},
		DropoutVoltage:             &EvidenceMeasurement{Value: .25, Unit: "V", Conditions: "maximum rated load"},
		LoadTransientRecoveryTime:  &EvidenceMeasurement{Value: 50e-6, Unit: "s", Conditions: "reviewed load step"},
		LoadTransientPeakDeviation: &EvidenceMeasurement{Value: .1, Unit: "V", Conditions: "reviewed load step"},
		ThermalReview:              "proven",
		FabricationProof:           true,
	}
}
