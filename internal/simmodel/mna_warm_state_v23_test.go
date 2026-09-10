package simmodel

import (
	"math"
	"strings"
	"testing"
)

// An independent historical reproducer: a stale saturation state can oscillate
// between both rails even though the unchanged finite-gain equations have a
// unique in-range negative-feedback solution. V23 must preserve this historical
// entry point while introducing and proving any explicitly versioned correction.
func TestV23ReproducerObsoleteOpAmpRailClamp(t *testing.T) {
	const gain = 100000.0
	amplifier := opAmpEvidence("amplifier", "IN", "OUT", "OUT", "VCC", "GND", .2, .2)
	for i := range amplifier.ModelClaims[0].Parameters {
		if amplifier.ModelClaims[0].Parameters[i].Name == "dc_open_loop_gain" {
			amplifier.ModelClaims[0].Parameters[i].Value = gain
		}
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("power", "VCC", "GND"),
		voltageSourceEvidence("stimulus", "IN", "GND"),
		amplifier,
		resistorEvidence("termination", 22000, "OUT", "GND"),
	}
	plan := centeredBiasTestPlan(t, components,
		[]NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VCC"}},
		[]SourceExcitation{{Component: "power", DCValue: 9}, {Component: "stimulus", DCValue: 2.4}},
	)
	analysis := plan.Analyses[0]
	system, diagnostics := buildMNASystem(plan, analysis, 0)
	if len(diagnostics) != 0 {
		t.Fatalf("build independent operating-point system: %v", diagnostics)
	}
	solution, diagnostic := solveMNA(system)
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	coldSystem, coldSolution, coldClamps, diagnostics := solveBoundedOpAmpDCFromState(plan, analysis, system, solution, nil)
	if len(diagnostics) != 0 || len(coldClamps) != 0 {
		t.Fatalf("finite-gain cold solution refused: %v, %v", coldClamps, diagnostics)
	}
	want := 2.4 * gain / (gain + 1)
	if actual := real(solvedNodeVoltage(coldSystem, coldSolution, "OUT")); math.Abs(actual-want) > 1e-10 {
		t.Fatalf("output %g, want finite-gain solution %g", actual, want)
	}
	for _, rail := range []float64{.2, 8.8} {
		_, _, _, diagnostics := solveBoundedOpAmpDCFromState(plan, analysis, system, solution, map[string]float64{"amplifier": rail})
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "operating-point states did not converge") {
			t.Fatalf("historical stale-rail refusal not reproduced at %g: %v", rail, diagnostics)
		}
	}
}
