package opentopologysynthesis

import (
	"context"
	"reflect"
	"testing"
)

func TestMultiOutputCandidateBreadthUsesCombinationBudget(t *testing.T) {
	tests := []struct {
		name                string
		maxRetained         int
		obligations         int
		maximumCombinations int
		want                int
	}{
		{name: "default two outputs", maxRetained: 16, obligations: 2, maximumCombinations: 32, want: 5},
		{name: "default three outputs", maxRetained: 16, obligations: 3, maximumCombinations: 32, want: 3},
		{name: "small search", maxRetained: 2, obligations: 2, maximumCombinations: 4, want: 2},
		{name: "single retained candidate", maxRetained: 1, obligations: 2, maximumCombinations: 2, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := multiOutputCandidateBreadth(
				test.maxRetained,
				test.obligations,
				test.maximumCombinations,
			)
			if got != test.want {
				t.Fatalf("multi-output candidate breadth = %d, want %d", got, test.want)
			}
		})
	}
}

func TestMultiOutputCompositionMergesIndependentBehaviorConesDeterministically(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	var primaryOutput Port
	for _, port := range requirement.Requirements.Ports {
		if port.ID == "output" {
			primaryOutput = port
			break
		}
	}
	if primaryOutput.ID == "" {
		t.Fatal("powered low-pass requirement lacks its declared output")
	}
	secondaryOutput := Port{
		ID:        "secondary_output",
		Kind:      "analog_voltage",
		Direction: "source",
		Domain:    "ground",
		Electrical: Electrical{
			MinVoltageV: primaryOutput.Electrical.MinVoltageV,
			MaxVoltageV: primaryOutput.Electrical.MaxVoltageV,
		},
	}
	requirement.Requirements.Ports = append(requirement.Requirements.Ports, secondaryOutput)
	secondaryAssertions := make([]BehavioralAssertion, 0, len(requirement.Requirements.BehavioralRequirements))
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		secondary := assertion
		secondary.ID = "secondary_" + assertion.ID
		secondary.Observation = Observation{Kind: "port", ID: secondaryOutput.ID}
		secondaryAssertions = append(secondaryAssertions, secondary)
	}
	requirement.Requirements.BehavioralRequirements = append(
		requirement.Requirements.BehavioralRequirements,
		secondaryAssertions...,
	)
	requirement = Normalize(requirement)
	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("multi-output requirement issues: %#v", issues)
	}

	policy := DefaultPolicy()
	policy.MaxRetainedCandidates = 2
	inventory := testSearchInventory()
	first := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	second := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	if first.Status != TopologySearchCandidates || len(first.Candidates) == 0 {
		t.Fatalf("multi-output topology search = status=%s issues=%#v rejections=%#v consumption=%#v", first.Status, first.Issues, first.Rejections, first.Consumption)
	}
	if !reflect.DeepEqual(topologyCandidateHashes(first.Candidates), topologyCandidateHashes(second.Candidates)) ||
		!reflect.DeepEqual(first.Consumption, second.Consumption) {
		t.Fatalf("multi-output topology search was not deterministic: first=%#v second=%#v", first, second)
	}
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("multi-output initial graph issues: %#v", issues)
	}
	previousHash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	for index, operation := range first.Candidates[0].Operations {
		if operation.Number != index+1 || operation.BeforeHash != previousHash || operation.AfterHash == "" {
			t.Fatalf("composition operation %d breaks replay chain: %#v", index, operation)
		}
		previousHash = operation.AfterHash
	}
	if previousHash != first.Candidates[0].Fingerprint {
		t.Fatalf("composition operation terminus = %q, want %q", previousHash, first.Candidates[0].Fingerprint)
	}
	replayed := initial
	for index, operation := range first.Candidates[0].Operations {
		beforeHash, hashErr := GraphHash(replayed)
		if hashErr != nil || beforeHash != operation.BeforeHash {
			t.Fatalf("composition replay operation %d starts at %q, want %q: %v", index, beforeHash, operation.BeforeHash, hashErr)
		}
		switch operation.Kind {
		case "add_internal_node":
			var nodeID string
			replayed, nodeID = addInternalNode(replayed, "internal")
			if nodeID != operation.Node {
				t.Fatalf("composition replay node %d = %q, want %q", index, nodeID, operation.Node)
			}
		case "add_primitive":
			primitive, found := primitiveByKey(inventory, operation.PrimitiveKey)
			if !found {
				t.Fatalf("composition replay primitive %d = %q is absent", index, operation.PrimitiveKey)
			}
			replayed = AddPrimitive(replayed, primitive, operation.ValueSI, operation.Connections)
		default:
			t.Fatalf("composition replay operation %d has unsupported kind %q", index, operation.Kind)
		}
		afterHash, hashErr := GraphHash(replayed)
		if hashErr != nil || afterHash != operation.AfterHash {
			t.Fatalf("composition replay operation %d ends at %q, want %q: %v", index, afterHash, operation.AfterHash, hashErr)
		}
	}
	for _, outputID := range []string{"output", secondaryOutput.ID} {
		nodeID := "port_" + outputID
		if !graphNodeHasConnection(first.Candidates[0].Graph, nodeID) {
			t.Fatalf("composed topology left output %s disconnected: %#v", outputID, first.Candidates[0].Graph)
		}
	}
}

func topologyCandidateHashes(candidates []TopologyCandidate) []string {
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.TopologyHash)
	}
	return result
}

func graphNodeHasConnection(graph CandidateGraph, nodeID string) bool {
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			if terminal.Node == nodeID {
				return true
			}
		}
	}
	return false
}
