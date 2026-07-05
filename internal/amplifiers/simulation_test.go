package amplifiers

import (
	"math"
	"strings"
	"testing"
)

func TestClassABHeadphoneSimulationExpectationDefaults(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	if expectation.ACGain.Nominal != 2 || expectation.LoadImpedanceOhms != 32 || expectation.SupplyVoltage != "9V" {
		t.Fatalf("expectation = %#v, want headphone Class AB defaults", expectation)
	}
	if expectation.StabilityMarginDeg.Min != 45 || expectation.StabilityMarginDeg.Max != 180 {
		t.Fatalf("stability margin = %#v, want bounded stable range", expectation.StabilityMarginDeg)
	}
	if len(expectation.RequiredMeasurements) == 0 {
		t.Fatalf("required measurements empty: %#v", expectation)
	}
}

func TestClassABHeadphoneSimulationNetlistUsesExpectationSupplyVoltage(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.SupplyVoltage = "12V"
	expectation.LoadImpedanceOhms = 64
	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if !strings.Contains(netlist, "VCC vcc 0 DC 12") || !strings.Contains(netlist, "* rails: 0V/12V") {
		t.Fatalf("netlist did not use expectation supply voltage:\n%s", netlist)
	}
	if !strings.Contains(netlist, "RLOAD hp_out 0 64") || !strings.Contains(netlist, "* expected_load_ohms: 64") {
		t.Fatalf("netlist did not use expectation load:\n%s", netlist)
	}
}

func TestClassABHeadphoneSimulationArtifactEmitsStableNetlist(t *testing.T) {
	artifact := BuildClassABHeadphoneSimulationArtifact("demo")
	if artifact.Status != SimulationStatusNotRun || artifact.Format != "spice-like-netlist.v0" {
		t.Fatalf("artifact metadata = %#v", artifact)
	}
	for _, want := range []string{
		"* KiCadAI amplifier simulation artifact: demo",
		"XU1 amp_in feedback driver_out vcc 0 OPAMP",
		"RBIAS_IN amp_in vbias 100k",
		"D1 bias_p driver_out D4148",
		"RE1 e1 hp_drive 0.47",
		"RF hp_drive feedback 10000",
		"COUT hp_drive hp_out 0.000203046",
		"RLOAD hp_out 0 32",
		".subckt OPAMP noninv inv out vcc vee",
		"RPOLE internal out_buf 1k",
		"CPOLE out_buf vee 15.9n",
		"ECLAMP out vee value={limit(V(out_buf), V(vee)+0.2, V(vcc)-0.2)}",
		".model NPN3904 NPN(Is=6.734f",
		".model PNP3906 PNP(Is=1.41f",
		".op",
		".ac dec 20 10 100k",
		".tran 0.1m 50m",
	} {
		if !strings.Contains(artifact.Netlist, want) {
			t.Fatalf("netlist missing %q:\n%s", want, artifact.Netlist)
		}
	}
}

func TestClassABHeadphoneSimulationNetlistDerivesGainAndOutputCoupling(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.ACGain.Nominal = 3
	expectation.LoadImpedanceOhms = 64
	expectation.HighPassCutoffHz = RangeExpectation{Min: 20, Max: 20}

	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if !strings.Contains(netlist, "RF hp_drive feedback 20000") {
		t.Fatalf("netlist did not derive feedback gain:\n%s", netlist)
	}
	if !strings.Contains(netlist, "RG feedback vbias 10000") {
		t.Fatalf("netlist did not include feedback reference resistor:\n%s", netlist)
	}
	if !strings.Contains(netlist, "COUT hp_drive hp_out 0.00012434") {
		t.Fatalf("netlist did not derive output coupling capacitor:\n%s", netlist)
	}
}

func TestClassABHeadphoneSimulationNetlistAllowsUnityGain(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.ACGain.Nominal = 1
	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if !strings.Contains(netlist, "RF hp_drive feedback 1e-06") {
		t.Fatalf("netlist did not allow unity gain feedback:\n%s", netlist)
	}
}

func TestClassABHeadphoneSimulationNetlistDefaultsNonFiniteOutputCoupling(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.HighPassCutoffHz = RangeExpectation{Min: math.SmallestNonzeroFloat64, Max: math.SmallestNonzeroFloat64}
	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if strings.Contains(netlist, "Inf") || strings.Contains(netlist, "NaN") {
		t.Fatalf("netlist emitted non-finite capacitor value:\n%s", netlist)
	}
	if !strings.Contains(netlist, "COUT hp_drive hp_out 0.00022") {
		t.Fatalf("netlist did not use fallback output capacitor:\n%s", netlist)
	}
}

func TestSimulationExpectationValidateReportsInvalidRanges(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.ACGain.Nominal = 3
	expectation.OutputSwingVPP = RangeExpectation{Min: 10, Max: 1}
	expectation.LoadImpedanceOhms = -1

	issues := expectation.Validate()
	for _, want := range []string{
		"ac_gain must satisfy min <= nominal <= max",
		"output_swing_vpp min exceeds max",
		"load_impedance_ohms must be positive",
	} {
		if !stringSliceContains(issues, want) {
			t.Fatalf("issues = %#v, want %q", issues, want)
		}
	}
	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if !strings.Contains(netlist, "* expected_load_ohms: 32") {
		t.Fatalf("netlist did not report defaulted load:\n%s", netlist)
	}
	if !strings.Contains(netlist, "* expectation_issue: output_swing_vpp min exceeds max") {
		t.Fatalf("netlist did not include expectation issue:\n%s", netlist)
	}
}

func TestSimulationExpectationValidateReportsNonFiniteValues(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.ACGain.Nominal = math.NaN()
	expectation.StabilityMarginDeg = RangeExpectation{Min: 45, Max: math.Inf(1)}

	issues := expectation.Validate()
	for _, want := range []string{
		"ac_gain values must be finite",
		"stability_margin_deg bounds must be finite",
	} {
		if !stringSliceContains(issues, want) {
			t.Fatalf("issues = %#v, want %q", issues, want)
		}
	}
}

func TestClassABHeadphoneSimulationNetlistSanitizesReference(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.SupplyVoltage = "9\n.include bad.lib"
	netlist := ClassABHeadphoneSPICENetlist("demo\n.include bad.lib", expectation)
	if strings.Contains(netlist, "\n.include bad.lib") {
		t.Fatalf("netlist reference was not sanitized:\n%s", netlist)
	}
	if !strings.Contains(netlist, "* KiCadAI amplifier simulation artifact: demo .include bad.lib") {
		t.Fatalf("netlist missing sanitized reference:\n%s", netlist)
	}
	if !strings.Contains(netlist, "\nVCC vcc 0 DC 9\n") || strings.Contains(netlist, "VCC vcc 0 DC 9 .include") {
		t.Fatalf("netlist supply voltage was not sanitized:\n%s", netlist)
	}
}

func TestClassABHeadphoneSimulationNetlistDefaultsInvalidSupplyVoltage(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.SupplyVoltage = ".bad"
	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if strings.Contains(netlist, "VCC vcc 0 DC .") {
		t.Fatalf("netlist used invalid SPICE voltage:\n%s", netlist)
	}
	if !strings.Contains(netlist, "VCC vcc 0 DC 9") {
		t.Fatalf("netlist did not use fallback voltage:\n%s", netlist)
	}
}

func TestClassABHeadphoneSimulationNetlistParsesSupplyVoltageSuffixes(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.SupplyVoltage = "500 mV"
	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if !strings.Contains(netlist, "VCC vcc 0 DC 0.5") {
		t.Fatalf("netlist did not parse mV suffix:\n%s", netlist)
	}
}

func TestClassABHeadphoneSimulationNetlistParsesWordVoltageSuffix(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.SupplyVoltage = "9 volts"
	netlist := ClassABHeadphoneSPICENetlist("demo", expectation)
	if !strings.Contains(netlist, "VCC vcc 0 DC 9") {
		t.Fatalf("netlist did not parse word voltage suffix:\n%s", netlist)
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
