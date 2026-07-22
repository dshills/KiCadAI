package components

import (
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestClockEvidenceNormalizesAndValidatesDeterministically(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Clock = validClockEvidence()
	catalog.Records[0].Clock.SignalingModes = []string{"lvttl", "lvcmos"}
	SortCatalog(&catalog)
	if got := catalog.Records[0].Clock.SignalingModes; !slices.Equal(got, []string{"lvcmos", "lvttl"}) {
		t.Fatalf("clock signaling modes = %v", got)
	}
	if result := ValidateCatalog(&catalog); !result.OK {
		t.Fatalf("valid clock evidence issues = %#v", result.Issues)
	}
}

func TestClockFabricationEvidenceFailsClosedWhenIncomplete(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Clock = validClockEvidence()
	catalog.Records[0].Clock.RMSJitter = nil
	result := ValidateCatalog(&catalog)
	if !slices.ContainsFunc(result.Issues, func(issue reports.Issue) bool {
		return issue.Path == "records[0].clock_evidence.rms_jitter" && issue.Severity == reports.SeverityBlocked
	}) {
		t.Fatalf("incomplete clock evidence issues = %#v", result.Issues)
	}
}

func validClockEvidence() *ClockEvidence {
	return &ClockEvidence{
		ProofStatus:           "proven",
		SignalingModes:        []string{"lvcmos"},
		Amplitude:             &EvidenceRange{Minimum: float64Pointer(3), Maximum: float64Pointer(3.3), Unit: "V", Conditions: "rated supply and load"},
		CommonMode:            &EvidenceRange{Minimum: float64Pointer(1.5), Maximum: float64Pointer(1.65), Unit: "V", Conditions: "rated supply and load"},
		EdgeTime:              &EvidenceRange{Minimum: float64Pointer(1e-9), Maximum: float64Pointer(4e-9), Unit: "s", Conditions: "rated capacitive load"},
		RMSJitter:             &EvidenceMeasurement{Value: 5e-12, Unit: "s", Conditions: "integrated phase jitter"},
		StartupTime:           &EvidenceMeasurement{Value: 2e-3, Unit: "s", Conditions: "rated supply ramp"},
		MaximumFrequency:      &EvidenceMeasurement{Value: 50e6, Unit: "Hz", Conditions: "rated supply and load"},
		OutputImpedance:       &EvidenceMeasurement{Value: 15, Unit: "Ohm", Conditions: "linearized output impedance"},
		OutputCurrent:         &EvidenceMeasurement{Value: 12e-3, Unit: "A", Conditions: "rated supply"},
		MaximumCapacitiveLoad: &EvidenceMeasurement{Value: 25e-12, Unit: "F", Conditions: "edge-time limit"},
		FabricationProof:      true,
	}
}
