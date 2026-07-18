package amplifiers

import (
	"context"
	"math"
	"slices"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestEngineeringValueAppliesEngineeringPrefixOnce(t *testing.T) {
	tests := []struct {
		raw  string
		unit string
		want float64
		ok   bool
	}{
		{raw: "220u", unit: "F", want: 220e-6, ok: true},
		{raw: "220", unit: "uF", want: 220e-6, ok: true},
		{raw: "220u", unit: "uF", want: 220e-6, ok: true},
		{raw: "47", unit: "pF", want: 47e-12, ok: true},
		{raw: "100", unit: "nF", want: 100e-9, ok: true},
		{raw: "35", unit: "mOhm", want: 35e-3, ok: true},
		{raw: "1.5G", unit: "Ohm", want: 1.5e9, ok: true},
		{raw: "220m", unit: "uF", ok: false},
	}
	for _, test := range tests {
		got, ok := engineeringValue(test.raw, test.unit)
		if ok != test.ok || (ok && math.Abs(got-test.want) > 1e-15) {
			t.Fatalf("engineeringValue(%q, %q) = %.12g, %v; want %.12g, %v", test.raw, test.unit, got, ok, test.want, test.ok)
		}
	}
}

func TestAppliedAudioComponentsPassReviewedEnvelope(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	opamp := amplifierCatalogRecord(t, catalog, "opamp.ti.lmv321.sot23_5")
	if issues := ValidateOpAmpApplication(opamp, OpAmpApplication{SupplyVoltageV: 5, InputMinimumV: 0.5, InputMaximumV: 3.5, OutputMinimumV: 0.2, OutputMaximumV: 4.5, OutputCurrentA: 0.01, RequiredGainBandwidthHz: 400_000, RequiredSlewRateVPerS: 500_000}); len(issues) != 0 {
		t.Fatalf("op-amp application issues: %#v", issues)
	}
	capacitor := amplifierCatalogRecord(t, catalog, "capacitor.panasonic.eeufr1c221.radial")
	if issues := ValidateCapacitorApplication(capacitor, CapacitorApplication{AppliedVoltageV: 9, RippleCurrentA: 0.1, MaximumESROhm: 0.2, RequiredCapacitanceF: 170e-6, ExpectedPolarity: "polarized"}); len(issues) != 0 {
		t.Fatalf("capacitor application issues: %#v", issues)
	}
}

func TestAppliedOpAmpEnvelopeSupportsDualSupplyRails(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	opamp := amplifierCatalogRecord(t, catalog, "opamp.ti.lmv321.sot23_5")
	application := OpAmpApplication{
		SupplyVoltageV:          5,
		NegativeRailVoltageV:    -2.5,
		PositiveRailVoltageV:    2.5,
		InputMinimumV:           -2,
		InputMaximumV:           1,
		OutputMinimumV:          -2.3,
		OutputMaximumV:          2,
		OutputCurrentA:          0.01,
		RequiredGainBandwidthHz: 400_000,
		RequiredSlewRateVPerS:   500_000,
	}
	if issues := ValidateOpAmpApplication(opamp, application); len(issues) != 0 {
		t.Fatalf("dual-supply op-amp application issues: %#v", issues)
	}
	application.OutputMinimumV = -2.5
	if issues := ValidateOpAmpApplication(opamp, application); !issuesContainPath(issues, "amplifier.opamp.output_swing") {
		t.Fatalf("dual-supply output issues = %#v, want negative-rail headroom failure", issues)
	}
}

func TestAppliedAudioComponentsFailClosedOnUnsafeEnvelope(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	opampIssues := ValidateOpAmpApplication(amplifierCatalogRecord(t, catalog, "opamp.ti.lmv321.sot23_5"), OpAmpApplication{SupplyVoltageV: 9, InputMinimumV: -0.1, InputMaximumV: 8.8, OutputMinimumV: 0, OutputMaximumV: 9, OutputCurrentA: 0.2, RequiredGainBandwidthHz: 2e6, RequiredSlewRateVPerS: 2e6, RequireFabricationProof: true})
	for _, path := range []string{"amplifier.opamp.fabrication_proof", "amplifier.opamp.supply_voltage", "amplifier.opamp.input_common_mode", "amplifier.opamp.output_swing", "amplifier.opamp.output_current", "amplifier.opamp.gain_bandwidth", "amplifier.opamp.slew_rate"} {
		if !issuesContainPath(opampIssues, path) {
			t.Fatalf("op-amp issues = %#v, want %s", opampIssues, path)
		}
	}
	capacitor := *amplifierCatalogRecord(t, catalog, "capacitor.panasonic.eeufr1c221.radial")
	capacitorEvidence := *capacitor.Capacitor
	capacitorEvidence.FabricationProof = false
	capacitor.Capacitor = &capacitorEvidence
	capacitorIssues := ValidateCapacitorApplication(&capacitor, CapacitorApplication{AppliedVoltageV: 25, RippleCurrentA: 2, MaximumESROhm: 0.01, RequiredCapacitanceF: 300e-6, ExpectedPolarity: "non_polarized", RequireFabricationProof: true})
	for _, path := range []string{"amplifier.capacitor.fabrication_proof", "amplifier.capacitor.voltage", "amplifier.capacitor.effective_capacitance", "amplifier.capacitor.esr", "amplifier.capacitor.ripple_current", "amplifier.capacitor.polarity"} {
		if !issuesContainPath(capacitorIssues, path) {
			t.Fatalf("capacitor issues = %#v, want %s", capacitorIssues, path)
		}
	}
}

func amplifierCatalogRecord(t *testing.T, catalog *components.Catalog, id string) *components.ComponentRecord {
	t.Helper()
	for index := range catalog.Records {
		if catalog.Records[index].ID == id {
			return &catalog.Records[index]
		}
	}
	t.Fatalf("catalog record %s not found", id)
	return nil
}

func issuesContainPath(issues []reports.Issue, path string) bool {
	return slices.ContainsFunc(issues, func(issue reports.Issue) bool { return issue.Path == path })
}
