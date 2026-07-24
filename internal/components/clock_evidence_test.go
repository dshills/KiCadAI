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

func TestClockFabricationEvidenceRequiresObservableBehaviorAndArchitectureClass(t *testing.T) {
	cases := []struct {
		name string
		drop func(*ClockEvidence)
		path string
	}{
		{name: "architecture class", drop: func(e *ClockEvidence) { e.ArchitectureClass = "" }, path: "records[0].clock_evidence.architecture_class"},
		{name: "frequency", drop: func(e *ClockEvidence) { e.Frequency = nil }, path: "records[0].clock_evidence.frequency"},
		{name: "frequency accuracy", drop: func(e *ClockEvidence) { e.FrequencyAccuracy = nil }, path: "records[0].clock_evidence.frequency_accuracy"},
		{name: "duty cycle", drop: func(e *ClockEvidence) { e.DutyCycle = nil }, path: "records[0].clock_evidence.duty_cycle"},
		{name: "supply voltage", drop: func(e *ClockEvidence) { e.SupplyVoltage = nil }, path: "records[0].clock_evidence.supply_voltage"},
		{name: "supply current", drop: func(e *ClockEvidence) { e.SupplyCurrent = nil }, path: "records[0].clock_evidence.supply_current"},
		{name: "output high", drop: func(e *ClockEvidence) { e.OutputHighVoltage = nil }, path: "records[0].clock_evidence.output_high_voltage"},
		{name: "output low", drop: func(e *ClockEvidence) { e.OutputLowVoltage = nil }, path: "records[0].clock_evidence.output_low_voltage"},
		{name: "fanout", drop: func(e *ClockEvidence) { e.MaximumFanout = nil }, path: "records[0].clock_evidence.maximum_fanout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := validCatalog()
			catalog.Records[0].Clock = validClockEvidence()
			tc.drop(catalog.Records[0].Clock)
			result := ValidateCatalog(&catalog)
			if !slices.ContainsFunc(result.Issues, func(issue reports.Issue) bool {
				return issue.Path == tc.path && issue.Severity == reports.SeverityBlocked
			}) {
				t.Fatalf("incomplete clock evidence issues = %#v", result.Issues)
			}
		})
	}
}

func validClockEvidence() *ClockEvidence {
	return &ClockEvidence{
		ArchitectureClass:     "packaged_oscillator",
		ProofStatus:           "proven",
		SignalingModes:        []string{"lvcmos"},
		Frequency:             &EvidenceRange{Minimum: float64Pointer(1e6), Maximum: float64Pointer(50e6), Unit: "Hz", Conditions: "ordered frequency range"},
		FrequencyAccuracy:     &EvidenceMeasurement{Value: 50, Unit: "ppm", Conditions: "supply, load, temperature, and aging"},
		DutyCycle:             &EvidenceRange{Minimum: float64Pointer(45), Maximum: float64Pointer(55), Unit: "%", Conditions: "rated supply and load"},
		SupplyVoltage:         &EvidenceRange{Minimum: float64Pointer(3), Maximum: float64Pointer(3.6), Unit: "V", Conditions: "recommended operation"},
		SupplyCurrent:         &EvidenceMeasurement{Value: 5e-3, Unit: "A", Conditions: "maximum operating current"},
		Amplitude:             &EvidenceRange{Minimum: float64Pointer(3), Maximum: float64Pointer(3.3), Unit: "V", Conditions: "rated supply and load"},
		CommonMode:            &EvidenceRange{Minimum: float64Pointer(1.5), Maximum: float64Pointer(1.65), Unit: "V", Conditions: "rated supply and load"},
		OutputHighVoltage:     &EvidenceMeasurement{Value: 2.7, Unit: "V", Conditions: "rated source current"},
		OutputLowVoltage:      &EvidenceMeasurement{Value: .3, Unit: "V", Conditions: "rated sink current"},
		EdgeTime:              &EvidenceRange{Minimum: float64Pointer(1e-9), Maximum: float64Pointer(4e-9), Unit: "s", Conditions: "rated capacitive load"},
		RMSJitter:             &EvidenceMeasurement{Value: 5e-12, Unit: "s", Conditions: "integrated phase jitter"},
		StartupTime:           &EvidenceMeasurement{Value: 2e-3, Unit: "s", Conditions: "rated supply ramp"},
		MaximumFrequency:      &EvidenceMeasurement{Value: 50e6, Unit: "Hz", Conditions: "rated supply and load"},
		OutputImpedance:       &EvidenceMeasurement{Value: 15, Unit: "Ohm", Conditions: "linearized output impedance"},
		OutputCurrent:         &EvidenceMeasurement{Value: 12e-3, Unit: "A", Conditions: "rated supply"},
		MaximumCapacitiveLoad: &EvidenceMeasurement{Value: 25e-12, Unit: "F", Conditions: "edge-time limit"},
		MaximumFanout:         &EvidenceMeasurement{Value: 4, Unit: "count", Conditions: "within rated capacitive load"},
		FabricationProof:      true,
	}
}
