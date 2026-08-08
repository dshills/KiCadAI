package opentopologysynthesis

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedWindowRelationshipPassesElectricalAndSafetyCorners(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "bipolar_magnitude_fault_indicator.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	envelope, found := topologyWindowBehaviorEnvelope(requirement)
	if !found || !envelope.signed || envelope.input != "bipolar_input" ||
		envelope.output != "fault_output" || envelope.lowerV != -1 || envelope.upperV != 1 {
		t.Fatalf("signed window envelope = %#v, found=%t", envelope, found)
	}

	inventory, environment := testHeldOutSynthesisEnvironment(t)
	run := Synthesize(context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy())
	assertNonlinearSwitchingDesignPass(t, requirement, run)
	counts := map[string]int{}
	for _, instance := range run.SelectedGraph.Instances {
		counts[instance.Kind]++
	}
	if counts["comparator"] != 2 || counts["opamp"] != 1 ||
		counts["reference_diode"] != 1 || counts["p_channel_mosfet"] != 1 {
		t.Fatalf("signed window active relationships = %v", counts)
	}
	foundScale := false
	for _, candidate := range run.Candidates {
		if candidate.TopologyHash != run.Report.Selected.TopologyHash {
			continue
		}
		for _, domain := range candidate.ValuePlan.Domains {
			for _, scale := range domain.AnalyticScales {
				foundScale = foundScale || strings.HasPrefix(scale.ID, "topology:signed_window:")
			}
		}
	}
	if !foundScale {
		t.Fatal("selected signed window lacks graph-derived catalog value evidence")
	}
}

func TestSignedWindowStaticHarnessSeparatesInsideAndOutsideStates(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "bipolar_magnitude_fault_indicator.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	var operatingCase OperatingCase
	assertions := map[string]BehavioralAssertion{}
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "magnitude_sweep" {
			operatingCase = candidate
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	for _, test := range []struct {
		assertion string
		want      float64
		quantity  string
	}{
		{assertion: "fault_high", want: 2},
		{assertion: "quiet_low", want: 0},
		{assertion: "negative_fault_bound", quantity: "lower_threshold_voltage_v"},
		{assertion: "positive_fault_bound", quantity: "upper_threshold_voltage_v"},
	} {
		assertion := assertions[test.assertion]
		if test.quantity != "" {
			quantity, _, supported := directSimulationQuantityForRequirement(requirement, assertion)
			if !supported || quantity != test.quantity {
				t.Fatalf("%s quantity = %q, supported=%t", test.assertion, quantity, supported)
			}
			continue
		}
		conditions := simulationHarnessConditions(requirement, assertion, operatingCase)
		found := false
		for _, condition := range conditions {
			if condition.Axis == "input_voltage" && condition.Target == "bipolar_input" {
				found = true
				if condition.Min != test.want || condition.Max != test.want {
					t.Fatalf("%s input condition = %#v, want fixed %g", test.assertion, condition, test.want)
				}
			}
		}
		if !found {
			t.Fatalf("%s lacks a signed-window input condition", test.assertion)
		}
	}
}
