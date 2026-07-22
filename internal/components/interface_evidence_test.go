package components

import (
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestInterfaceTranslatorAndADCEvidenceNormalizeDeterministically(t *testing.T) {
	catalog := validCatalog()
	record := &catalog.Records[0]
	record.Interface = validInterfaceEvidence()
	record.Interface.SignalingModes = []string{"push_pull", "open_drain"}
	record.Interface.Directions = []string{"source", "bidirectional"}
	record.Translator = validTranslatorEvidence()
	record.Translator.SignalingModes = []string{"push_pull", "open_drain"}
	record.Translator.Directions = []string{"unidirectional", "bidirectional"}
	record.ADC = validADCEvidence()
	SortCatalog(&catalog)
	if !slices.Equal(record.Interface.SignalingModes, []string{"open_drain", "push_pull"}) ||
		!slices.Equal(record.Interface.Directions, []string{"bidirectional", "source"}) ||
		!slices.Equal(record.Translator.SignalingModes, []string{"open_drain", "push_pull"}) ||
		!slices.Equal(record.Translator.Directions, []string{"bidirectional", "unidirectional"}) {
		t.Fatalf("interface evidence was not normalized: %#v %#v", record.Interface, record.Translator)
	}
	if result := ValidateCatalog(&catalog); !result.OK {
		t.Fatalf("valid interface evidence issues = %#v", result.Issues)
	}
}

func TestFabricationInterfaceEvidenceFailsClosedWhenIncomplete(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Interface = validInterfaceEvidence()
	catalog.Records[0].Interface.InputCapacitance = nil
	result := ValidateCatalog(&catalog)
	if !slices.ContainsFunc(result.Issues, func(issue reports.Issue) bool {
		return issue.Path == "records[0].interface_evidence.input_capacitance" && issue.Severity == reports.SeverityBlocked
	}) {
		t.Fatalf("incomplete interface evidence issues = %#v", result.Issues)
	}
}

func TestTranslatorAndADCEvidenceRejectMissingRequiredFacts(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Translator = validTranslatorEvidence()
	catalog.Records[0].Translator.MaximumFrequency = nil
	catalog.Records[0].ADC = validADCEvidence()
	catalog.Records[0].ADC.AcquisitionTime = nil
	result := ValidateCatalog(&catalog)
	for _, path := range []string{"records[0].translator_evidence", "records[0].adc_evidence.acquisition_time"} {
		if !slices.ContainsFunc(result.Issues, func(issue reports.Issue) bool { return issue.Path == path }) {
			t.Fatalf("missing required issue %s in %#v", path, result.Issues)
		}
	}
}

func validInterfaceEvidence() *InterfaceEvidence {
	return &InterfaceEvidence{
		ProofStatus: "proven", SignalingModes: []string{"open_drain"}, Directions: []string{"bidirectional"},
		Voltage:           &EvidenceRange{Minimum: float64Pointer(1.65), Maximum: float64Pointer(5.5), Unit: "V", Conditions: "rated supplies"},
		OutputLowMaximumV: float64Pointer(.4), OutputHighMinimumV: float64Pointer(1.2),
		InputLowMaximumV: float64Pointer(.5), InputHighMinimumV: float64Pointer(1.1),
		OutputImpedance:  &EvidenceMeasurement{Value: 400, Unit: "Ohm", Conditions: "channel enabled"},
		OutputCurrent:    &EvidenceMeasurement{Value: .05, Unit: "A", Conditions: "rated supplies"},
		InputCapacitance: &EvidenceMeasurement{Value: 10e-12, Unit: "F", Conditions: "channel disabled"},
		InputLeakage:     &EvidenceMeasurement{Value: 2e-6, Unit: "A", Conditions: "partial power down"},
		EdgeTime:         &EvidenceRange{Minimum: float64Pointer(1e-9), Maximum: float64Pointer(1e-6), Unit: "s", Conditions: "rated load"},
		MaximumFrequency: &EvidenceMeasurement{Value: 2e6, Unit: "Hz", Conditions: "open-drain signaling"},
		StartupTime:      &EvidenceMeasurement{Value: 100e-9, Unit: "s", Conditions: "OE assertion"},
		FabricationProof: true,
	}
}

func validTranslatorEvidence() *TranslatorEvidence {
	return &TranslatorEvidence{
		ProofStatus: "proven", SignalingModes: []string{"open_drain"}, Directions: []string{"bidirectional"}, ChannelCount: 2,
		SideAVoltage:     &EvidenceRange{Minimum: float64Pointer(1.65), Maximum: float64Pointer(3.6), Unit: "V", Conditions: "rated operation"},
		SideBVoltage:     &EvidenceRange{Minimum: float64Pointer(2.3), Maximum: float64Pointer(5.5), Unit: "V", Conditions: "rated operation"},
		MaximumFrequency: &EvidenceMeasurement{Value: 2e6, Unit: "Hz", Conditions: "open-drain signaling"},
		StartupTime:      &EvidenceMeasurement{Value: 100e-9, Unit: "s", Conditions: "OE assertion"},
		PartialPowerDown: true, StartupState: "high_impedance", FabricationProof: true,
	}
}

func validADCEvidence() *ADCEvidence {
	return &ADCEvidence{
		ProofStatus:            "proven",
		AcquisitionCapacitance: &EvidenceMeasurement{Value: 20e-12, Unit: "F", Conditions: "sample switch enabled"},
		AcquisitionTime:        &EvidenceMeasurement{Value: 1e-6, Unit: "s", Conditions: "rated clock"},
		MaximumSourceImpedance: &EvidenceMeasurement{Value: 10e3, Unit: "Ohm", Conditions: "one-half LSB settling"},
		InputLeakage:           &EvidenceMeasurement{Value: 1e-6, Unit: "A", Conditions: "rated temperature"},
		MaximumFrequency:       &EvidenceMeasurement{Value: 1e6, Unit: "Hz", Conditions: "rated resolution"},
		FabricationProof:       true,
	}
}
