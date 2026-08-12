package opentopologysynthesis

import (
	"context"
	"testing"
)

func TestDCInputImpedanceIntentResolvesCanonicalReferenceAndSource(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	assertion := BehavioralAssertion{
		ID: "input_loading", Metric: "input_impedance", Analysis: "dc_operating_point",
		Observation: Observation{Kind: "port", ID: "input"},
		Min:         graphFloat(5_000), Unit: "ohm",
		OperatingCases: []string{requirement.Requirements.OperatingCases[0].ID},
	}
	requirement.Requirements.BehavioralRequirements = []BehavioralAssertion{assertion}
	requirement.Requirements.OperatingCases[0].Conditions = append(
		requirement.Requirements.OperatingCases[0].Conditions,
		OperatingCondition{Axis: "input_voltage", Target: "input", Min: 1, Max: 1, Unit: "V"},
	)
	requirement = Normalize(requirement)
	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("DC input-impedance requirement issues = %#v", issues)
	}

	result := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, DefaultPolicy())
	if result.Status != SimulationEvaluationPassed || len(result.Diagnoses) != 0 {
		t.Fatalf("DC input-impedance evaluation status=%s diagnoses=%#v attempts=%#v", result.Status, result.Diagnoses, result.Attempts)
	}
	if len(result.Attempts) == 0 || result.Attempts[0].Actual == nil || *result.Attempts[0].Actual < 5_000 {
		t.Fatalf("DC input-impedance evidence = %#v", result.Attempts)
	}
}
