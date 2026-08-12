package opentopologysynthesis

import (
	"reflect"
	"testing"
)

func TestMultiControlObligationsSplitControlsAndPreserveSharedContext(t *testing.T) {
	requirement := realizabilityTestRequirement()
	requirement.Requirements.Ports = append(requirement.Requirements.Ports,
		Port{ID: "enable_in", Kind: "digital", Direction: "sink", Domain: "reference"},
	)
	requirement.Requirements.BehavioralRequirements = append(
		requirement.Requirements.BehavioralRequirements,
		BehavioralAssertion{
			ID: "signal_trip", Metric: "threshold_voltage", Analysis: "dc_sweep",
			Excitation:  &Observation{Kind: "port", ID: "signal_in"},
			Observation: Observation{Kind: "port", ID: "signal_out"},
			Min:         graphFloat(0.4), Max: graphFloat(0.6), Unit: "V", OperatingCases: []string{"nominal"},
		},
		BehavioralAssertion{
			ID: "enable_delay", Metric: "propagation_delay", Analysis: "transient",
			Excitation:  &Observation{Kind: "port", ID: "enable_in"},
			Observation: Observation{Kind: "port", ID: "signal_out"},
			Max:         graphFloat(.001), Unit: "s", OperatingCases: []string{"nominal"},
		},
		BehavioralAssertion{
			ID: "signal_loading", Metric: "input_impedance", Analysis: "dc_operating_point",
			Observation: Observation{Kind: "port", ID: "signal_in"},
			Min:         graphFloat(100_000), Unit: "ohm", OperatingCases: []string{"nominal"},
		},
	)
	requirement = Normalize(requirement)
	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("multi-control requirement issues = %#v", issues)
	}

	obligations := multiControlObligations(requirement)
	if len(obligations) != 2 {
		t.Fatalf("multi-control obligations = %d, want 2", len(obligations))
	}
	if got := []string{obligations[0].controlID, obligations[1].controlID}; !reflect.DeepEqual(got, []string{"enable_in", "signal_in"}) {
		t.Fatalf("control order = %#v", got)
	}
	for _, obligation := range obligations {
		portIDs := map[string]bool{}
		for _, port := range obligation.requirement.Requirements.Ports {
			portIDs[port.ID] = true
		}
		if !portIDs[obligation.controlID] || !portIDs[obligation.outputID] || !portIDs["supply_in"] {
			t.Fatalf("obligation %s lacks control/output/power context: %#v", obligation.controlID, portIDs)
		}
		otherControl := "signal_in"
		if obligation.controlID == otherControl {
			otherControl = "enable_in"
		}
		if portIDs[otherControl] {
			t.Fatalf("obligation %s retained unrelated control %s", obligation.controlID, otherControl)
		}
		assertionIDs := map[string]bool{}
		for _, assertion := range obligation.requirement.Requirements.BehavioralRequirements {
			assertionIDs[assertion.ID] = true
		}
		if !assertionIDs["output_level"] {
			t.Fatalf("obligation %s lost shared output assertion", obligation.controlID)
		}
		if obligation.controlID == "signal_in" && !assertionIDs["signal_loading"] {
			t.Fatal("signal obligation lost control-scoped loading assertion")
		}
		if issues := Validate(obligation.requirement); len(issues) != 0 {
			t.Fatalf("obligation %s issues = %#v", obligation.controlID, issues)
		}
	}
}

func TestMultiControlCombinationCountRemainsGloballyBounded(t *testing.T) {
	limit := 7
	combinations := [][]int{{}}
	for range 5 {
		next := make([][]int, 0, limit)
		for _, combination := range combinations {
			for candidate := range 4 {
				next = append(next, append(append([]int(nil), combination...), candidate))
				if len(next) >= limit {
					break
				}
			}
			if len(next) >= limit {
				break
			}
		}
		combinations = next
		if len(combinations) > limit {
			t.Fatalf("composition materialized %d paths, limit %d", len(combinations), limit)
		}
	}
}
