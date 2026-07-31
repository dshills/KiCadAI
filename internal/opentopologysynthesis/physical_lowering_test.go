package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/circuitgraph"
)

func TestPassingPrimitiveGraphLowersToResolvedDesignRequest(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	evaluation := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, DefaultPolicy())
	if evaluation.Status != SimulationEvaluationPassed {
		t.Fatalf("simulation prerequisite = status=%s issues=%#v diagnoses=%#v", evaluation.Status, evaluation.Issues, evaluation.Diagnoses)
	}
	first := LowerPassingCandidate(context.Background(), requirement, graph, evaluation, inventory, environment)
	if first.Status != PhysicalLoweringReady || len(first.Issues) != 0 ||
		len(first.Document.Components) == 0 || len(first.Document.Nets) == 0 ||
		len(first.Resolved.Components) != len(first.Document.Components) ||
		first.DesignRequest.ExplicitCircuit == nil ||
		len(first.DesignRequest.ExplicitCircuit.Components) != len(first.Document.Components) ||
		len(first.Bindings) == 0 || len(first.Hash) != 64 {
		t.Fatalf("physical lowering = status=%s issues=%#v components=%d/%d request=%#v bindings=%#v",
			first.Status, first.Issues, len(first.Resolved.Components), len(first.Document.Components), first.DesignRequest, first.Bindings)
	}
	if issues := circuitgraph.Validate(first.Document); len(issues) != 0 {
		t.Fatalf("lowered circuit graph issues: %#v", issues)
	}
	for _, component := range first.Document.Components {
		if component.ComponentID == "" || component.VariantID == "" {
			t.Fatalf("component lacks deterministic physical selection: %#v", component)
		}
	}

	second := LowerPassingCandidate(context.Background(), requirement, graph, evaluation, inventory, environment)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("physical lowering replay differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestPhysicalLoweringRejectsUnprovenGraph(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	result := LowerPassingCandidate(context.Background(), requirement, graph, SimulationEvaluation{}, inventory, environment)
	if result.Status != PhysicalLoweringUnsupported || len(result.Issues) != 1 ||
		result.Issues[0].Code != CodePhysicalPromotionFailed {
		t.Fatalf("unproven lowering = status=%s issues=%#v", result.Status, result.Issues)
	}
}
