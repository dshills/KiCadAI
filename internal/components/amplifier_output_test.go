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

func TestSelectAmplifierOutputPairBlocksFabricationCandidateReviewGaps(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		SupplyVoltage:    "9",
		SupplyUnit:       "V",
		LoadImpedance:    "32",
		LoadUnit:         "ohm",
		Acceptance:       AcceptanceFabricationCandidate,
		RequireHeadphone: true,
	})
	if result.OK {
		t.Fatal("expected fabrication-candidate pair selection to block on review evidence")
	}
	assertIssueCode(t, result.Issues, CodeComponentReviewRequired)
	assertIssuePath(t, result.Issues, "component.bjt.onsemi.mmbt3904.sot23.amplifier_output_evidence.thermal_review")
	assertIssuePath(t, result.Issues, "component.bjt.onsemi.mmbt3906.sot23.amplifier_output_evidence.thermal_review")
}
