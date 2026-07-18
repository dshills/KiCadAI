package components

import (
	"testing"

	"kicadai/internal/reports"
)

func TestFabricationProofOpAmpRequiresQuantitativeEvidence(t *testing.T) {
	evidence := validOpAmpEvidence()
	evidence.FabricationCandidateBlocks = false
	evidence.FabricationProof = true

	issues := validateOpAmpEvidence("opamp", evidence)
	assertIssueCode(t, issues, CodeInvalidMetadata)
	assertIssuePath(t, issues, "opamp.supply_voltage")
	assertIssuePath(t, issues, "opamp.input_common_mode")
	assertIssuePath(t, issues, "opamp.output_swing")
	assertIssuePath(t, issues, "opamp.output_current")
	assertIssuePath(t, issues, "opamp.gain_bandwidth")
	assertIssuePath(t, issues, "opamp.slew_rate")
	assertIssuePath(t, issues, "opamp.voltage_noise_density")
	assertIssuePath(t, issues, "opamp.max_junction_temperature_c")
	assertIssuePath(t, issues, "opamp.junction_to_ambient_c_per_w")
}

func TestFabricationProofCapacitorRequiresAppliedEvidence(t *testing.T) {
	evidence := &CapacitorEvidence{
		Dielectric:         "aluminum_electrolytic",
		NominalCapacitance: "220u",
		CapacitanceUnit:    "F",
		VoltageRating:      "16",
		VoltageUnit:        "V",
		FabricationProof:   true,
	}

	issues := validateCapacitorEvidence("capacitor", false, evidence)
	assertIssueCode(t, issues, CodeInvalidMetadata)
	assertIssuePath(t, issues, "capacitor.technology")
	assertIssuePath(t, issues, "capacitor.polarity")
	assertIssuePath(t, issues, "capacitor.capacitance_tolerance_percent")
	assertIssuePath(t, issues, "capacitor.esr")
	assertIssuePath(t, issues, "capacitor.ripple_current")
	assertIssuePath(t, issues, "capacitor.endurance_hours")
	assertIssuePath(t, issues, "capacitor.endurance_temperature_c")
}

func TestPowerSemiconductorEvidenceRejectsMalformedSOA(t *testing.T) {
	evidence := validFabricationBJT()
	evidence.SOA = []SOAEnvelopePoint{
		{VoltageV: 10, CurrentA: 2, DC: true, CaseTemperatureC: 25},
		{VoltageV: 5, CurrentA: 3, DC: true, CaseTemperatureC: 25},
	}
	record := ComponentRecord{Family: "bjt", PowerSemiconductor: evidence}

	issues := validatePowerSemiconductorEvidence("power", &record)
	assertIssueCode(t, issues, CodeInvalidMetadata)
	assertIssuePath(t, issues, "power.soa[1].voltage_v")
	assertIssuePath(t, issues, "power.soa[1].current_a")
}

func TestPowerSemiconductorEvidenceRequiresExactlyOneSOATimeBasis(t *testing.T) {
	duration := 0.001
	evidence := validFabricationBJT()
	evidence.SOA[0].PulseDurationS = &duration

	issues := validatePowerSemiconductorEvidence("power", &ComponentRecord{Family: "bjt", PowerSemiconductor: evidence})
	assertIssueCode(t, issues, CodeInvalidMetadata)
	assertIssuePath(t, issues, "power.soa[0]")
}

func TestPowerSemiconductorEvidenceRejectsDeviceFamilyMismatch(t *testing.T) {
	evidence := validFabricationBJT()

	issues := validatePowerSemiconductorEvidence("power", &ComponentRecord{Family: "mosfet", PowerSemiconductor: evidence})
	assertIssueCode(t, issues, CodeInvalidMetadata)
	assertIssuePath(t, issues, "power.device_class")
}

func TestValidFabricationPowerSemiconductorEvidence(t *testing.T) {
	tests := []struct {
		name   string
		family string
		value  *PowerSemiconductorEvidence
	}{
		{name: "BJT", family: "bjt", value: validFabricationBJT()},
		{name: "MOSFET", family: "mosfet", value: validFabricationMOSFET()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := validatePowerSemiconductorEvidence("power", &ComponentRecord{Family: tt.family, PowerSemiconductor: tt.value})
			for _, issue := range issues {
				if issue.Severity == reports.SeverityBlocked {
					t.Fatalf("unexpected blocked issue: %+v", issue)
				}
			}
		})
	}
}

func TestQuantitativeEvidenceRejectsInvertedRange(t *testing.T) {
	minimum, typical, maximum := 5.5, 3.3, 2.7
	issues := validateEvidenceRange("range", &EvidenceRange{
		Minimum: &minimum,
		Typical: &typical,
		Maximum: &maximum,
		Unit:    "V",
	}, false)
	assertIssueCode(t, issues, CodeInvalidMetadata)
	assertIssuePath(t, issues, "range.minimum")
	assertIssuePath(t, issues, "range.typical")
}

func validFabricationBJT() *PowerSemiconductorEvidence {
	maxVoltage := EvidenceMeasurement{Value: 80, Unit: "V", Conditions: "collector-emitter, base open"}
	continuous := EvidenceMeasurement{Value: 15, Unit: "A", Conditions: "case temperature 25 C"}
	peak := EvidenceMeasurement{Value: 30, Unit: "A", Conditions: "bounded pulse"}
	power := EvidenceMeasurement{Value: 150, Unit: "W", Conditions: "case temperature 25 C"}
	maxJunction, junctionToCase, junctionToAmbient := 150.0, 0.83, 35.0
	gain, gainCurrent, transition := 75.0, 3.0, 30e6
	return &PowerSemiconductorEvidence{
		DeviceClass:             "bjt",
		Polarity:                "npn",
		ComplementaryGroup:      "audio_power_pair",
		MaxVoltage:              &maxVoltage,
		ContinuousCurrent:       &continuous,
		PeakCurrent:             &peak,
		PowerDissipation:        &power,
		MaxJunctionTemperatureC: &maxJunction,
		JunctionToCaseCPerW:     &junctionToCase,
		JunctionToAmbientCPerW:  &junctionToAmbient,
		SOA: []SOAEnvelopePoint{
			{VoltageV: 5, CurrentA: 10, DC: true, CaseTemperatureC: 25},
			{VoltageV: 40, CurrentA: 1, DC: true, CaseTemperatureC: 25},
		},
		SecondaryBreakdownStatus: "proven",
		LinearModeStatus:         "not_applicable",
		MountingAssumptions:      "datasheet case-temperature basis with reviewed heatsink",
		BJT: &PowerBJTEvidence{
			GainMinimum:      &gain,
			GainTestCurrentA: &gainCurrent,
			TransitionFreqHz: &transition,
		},
		FabricationProof: true,
	}
}

func validFabricationMOSFET() *PowerSemiconductorEvidence {
	maxVoltage := EvidenceMeasurement{Value: 200, Unit: "V", Conditions: "gate-source shorted"}
	continuous := EvidenceMeasurement{Value: 20, Unit: "A", Conditions: "case temperature 25 C"}
	peak := EvidenceMeasurement{Value: 80, Unit: "A", Conditions: "bounded pulse"}
	power := EvidenceMeasurement{Value: 150, Unit: "W", Conditions: "case temperature 25 C"}
	maxJunction, junctionToCase, junctionToAmbient := 150.0, 0.8, 40.0
	rds, gateV, thresholdMin, thresholdMax := 0.18, 10.0, 2.0, 4.0
	gm, gateCharge, inputCap, reverseCap := 8.0, 70e-9, 1300e-12, 250e-12
	return &PowerSemiconductorEvidence{
		DeviceClass:             "mosfet",
		Polarity:                "n_channel",
		ComplementaryGroup:      "audio_power_fet_pair",
		MaxVoltage:              &maxVoltage,
		ContinuousCurrent:       &continuous,
		PeakCurrent:             &peak,
		PowerDissipation:        &power,
		MaxJunctionTemperatureC: &maxJunction,
		JunctionToCaseCPerW:     &junctionToCase,
		JunctionToAmbientCPerW:  &junctionToAmbient,
		SOA: []SOAEnvelopePoint{
			{VoltageV: 10, CurrentA: 8, DC: true, CaseTemperatureC: 25},
			{VoltageV: 100, CurrentA: 0.5, DC: true, CaseTemperatureC: 25},
		},
		SecondaryBreakdownStatus: "not_applicable",
		LinearModeStatus:         "proven",
		MountingAssumptions:      "datasheet case-temperature basis with reviewed heatsink",
		MOSFET: &PowerMOSFETEvidence{
			RDSOnOhm:             &rds,
			RDSOnGateVoltageV:    &gateV,
			ThresholdMinimumV:    &thresholdMin,
			ThresholdMaximumV:    &thresholdMax,
			TransconductanceS:    &gm,
			TotalGateChargeC:     &gateCharge,
			InputCapacitanceF:    &inputCap,
			ReverseTransferCapF:  &reverseCap,
			BodyDiodeStatus:      "proven",
			GateProtectionStatus: "review_required",
		},
		FabricationProof: true,
	}
}
