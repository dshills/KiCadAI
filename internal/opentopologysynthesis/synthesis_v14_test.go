package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestSynthesisValueTrialV14RetainsNoMaterializedGraph(t *testing.T) {
	typeOf := reflect.TypeOf(synthesisValueTrialV14{})
	graphType := reflect.TypeOf(CandidateGraph{})
	for index := 0; index < typeOf.NumField(); index++ {
		if typeOf.Field(index).Type == graphType {
			t.Fatalf("V14 value-trial scheduler retains a materialized candidate graph in field %q", typeOf.Field(index).Name)
		}
	}
}

func TestSynthesizeV14PreservesFrozenSynthesisEvidence(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 1_000
	policy.MaxGeneratedGraphs = 5_000
	policy.MaxRetainedCandidates = 4
	policy.MaxValueTrials = 4
	policy.MaxTopologyRepairs = 2
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1_024

	frozen := Synthesize(context.Background(), requirement, inventory, environment, policy)
	lazy := SynthesizeV14(context.Background(), requirement, inventory, environment, policy)
	frozenJSON, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	lazyJSON, err := json.Marshal(lazy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frozenJSON, lazyJSON) {
		t.Fatal("V14 lazy materialization changed frozen synthesis evidence")
	}
}
