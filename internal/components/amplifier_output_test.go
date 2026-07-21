package components

import (
	"context"
	"testing"

	"kicadai/internal/reports"
)

func TestSelectAmplifierOutputPairForHeadphoneLoad(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	pair, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		SupplyVoltage:    "9",
		SupplyUnit:       "V",
		LoadImpedance:    "32",
		LoadUnit:         "ohm",
		Acceptance:       AcceptanceConnectivity,
		RequireHeadphone: true,
	})
	if !result.OK {
		t.Fatalf("expected headphone output pair selection to pass: %+v", result.Issues)
	}
	if pair.NPN.Component.ID != "bjt.onsemi.mmbt3904.sot23" {
		t.Fatalf("NPN selection = %s", pair.NPN.Component.ID)
	}
	if pair.PNP.Component.ID != "bjt.onsemi.mmbt3906.sot23" {
		t.Fatalf("PNP selection = %s", pair.PNP.Component.ID)
	}
	if pair.NPN.Component.AmplifierOutput.ComplementaryGroup != "mmbt390x_sot23" || pair.PNP.Component.AmplifierOutput.ComplementaryGroup != "mmbt390x_sot23" {
		t.Fatalf("unexpected complementary groups: NPN=%q PNP=%q", pair.NPN.Component.AmplifierOutput.ComplementaryGroup, pair.PNP.Component.AmplifierOutput.ComplementaryGroup)
	}
	if pair.EstimatedPeakMA != "140.625" {
		t.Fatalf("estimated peak current = %s", pair.EstimatedPeakMA)
	}
}

func TestSelectAmplifierOutputPairBlocksSpeakerLoad(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		SupplyVoltage:    "12",
		SupplyUnit:       "V",
		LoadImpedance:    "8",
		LoadUnit:         "ohm",
		Acceptance:       AcceptanceConnectivity,
		RequireHeadphone: true,
	})
	if result.OK {
		t.Fatal("expected speaker load to block")
	}
	assertIssueCode(t, result.Issues, CodeAmplifierOutputUnsupported)
	assertIssuePath(t, result.Issues, "amplifier_output.load_impedance")
}

func TestSelectAmplifierOutputPairRejectsZeroLoad(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		SupplyVoltage:    "9",
		SupplyUnit:       "V",
		LoadImpedance:    "0",
		LoadUnit:         "ohm",
		Acceptance:       AcceptanceConnectivity,
		RequireHeadphone: true,
	})
	if result.OK {
		t.Fatal("expected zero load to block")
	}
	assertIssueCode(t, result.Issues, reports.CodeInvalidArgument)
	assertIssuePath(t, result.Issues, "amplifier_output.load_impedance")
}

func TestSelectAmplifierOutputPairRejectsZeroSupply(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		SupplyVoltage:    "0",
		SupplyUnit:       "V",
		LoadImpedance:    "32",
		LoadUnit:         "ohm",
		Acceptance:       AcceptanceConnectivity,
		RequireHeadphone: true,
	})
	if result.OK {
		t.Fatal("expected zero supply to block")
	}
	assertIssueCode(t, result.Issues, reports.CodeInvalidArgument)
	assertIssuePath(t, result.Issues, "amplifier_output.supply_voltage")
}

func TestSelectAmplifierOutputPairPassesFabricationEvidenceForBoundedHeadphoneLoad(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	pair, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		SupplyVoltage:    "9",
		SupplyUnit:       "V",
		LoadImpedance:    "32",
		LoadUnit:         "ohm",
		Acceptance:       AcceptanceFabricationCandidate,
		RequireHeadphone: true,
	})
	if !result.OK {
		t.Fatalf("expected bounded headphone pair to pass fabrication evidence: %+v", result.Issues)
	}
	if pair.Upper.Component.PowerSemiconductor == nil || !pair.Upper.Component.PowerSemiconductor.FabricationProof || pair.Lower.Component.PowerSemiconductor == nil || !pair.Lower.Component.PowerSemiconductor.FabricationProof {
		t.Fatalf("pair lacks typed fabrication evidence: %#v", pair)
	}
}

func TestSelectAmplifierOutputPairForPowerBJTLoad(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	pair, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		DeviceClass:   "bjt",
		Application:   "power",
		SupplyVoltage: "50",
		SupplyUnit:    "V",
		LoadImpedance: "8",
		LoadUnit:      "ohm",
		Acceptance:    AcceptanceFabricationCandidate,
	})
	if !result.OK {
		t.Fatalf("expected evidence-backed power BJT pair selection to pass: %+v", result.Issues)
	}
	if pair.Upper.Component.ID != "bjt.onsemi.d44h11g.to220" || pair.Lower.Component.ID != "bjt.onsemi.d45h11g.to220" {
		t.Fatalf("power BJT pair = %s / %s", pair.Upper.Component.ID, pair.Lower.Component.ID)
	}
	if pair.DeviceClass != "bjt" || pair.EstimatedPeakMA != "3125" {
		t.Fatalf("power BJT pair evidence = %#v", pair)
	}
}

func TestSelectAmplifierOutputPairBlocksLinearMOSFETWithoutSOAProof(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		DeviceClass:   "mosfet",
		Application:   "power",
		SupplyVoltage: "40",
		SupplyUnit:    "V",
		LoadImpedance: "8",
		LoadUnit:      "ohm",
		Acceptance:    AcceptanceFabricationCandidate,
	})
	if result.OK {
		t.Fatal("expected switching MOSFET pair to block linear fabrication selection")
	}
	assertIssueCode(t, result.Issues, CodeComponentReviewRequired)
	assertIssuePath(t, result.Issues, "component.mosfet.vishay.irfp240.to247.power_semiconductor_evidence.fabrication_proof")
	assertIssuePath(t, result.Issues, "component.mosfet.vishay.irfp240.to247.power_semiconductor_evidence.linear_mode_status")
	assertIssuePath(t, result.Issues, "component.mosfet.vishay.irfp9240.to247.power_semiconductor_evidence.fabrication_proof")
	assertIssuePath(t, result.Issues, "component.mosfet.vishay.irfp9240.to247.power_semiconductor_evidence.linear_mode_status")
}

func TestSelectAmplifierOutputPairRejectsUnknownDeviceClass(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		DeviceClass:   "tube",
		Application:   "power",
		SupplyVoltage: "40",
		LoadImpedance: "8",
	})
	if result.OK {
		t.Fatal("expected unsupported device class to block")
	}
	assertIssueCode(t, result.Issues, CodeAmplifierOutputUnsupported)
	assertIssuePath(t, result.Issues, "amplifier_output.device_class")
}
