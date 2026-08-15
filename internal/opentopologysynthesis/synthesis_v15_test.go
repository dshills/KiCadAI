package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestSynthesisValueTrialV15RetainsNoMaterializedGraph(t *testing.T) {
	typeOf := reflect.TypeOf(synthesisValueTrialV15{})
	graphType := reflect.TypeOf(CandidateGraph{})
	for index := 0; index < typeOf.NumField(); index++ {
		if typeOf.Field(index).Type == graphType {
			t.Fatalf("V15 value-trial scheduler retains a materialized candidate graph in field %q", typeOf.Field(index).Name)
		}
	}
}

func TestSynthesizeV15PreservesFrozenSynthesisEvidence(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 1_000
	policy.MaxGeneratedGraphs = 5_000
	policy.MaxRetainedCandidates = 4
	policy.MaxValueTrials = 4
	policy.MaxTopologyRepairs = 2
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1_024

	lazy := SynthesizeV14(context.Background(), requirement, inventory, environment, policy)
	bounded := SynthesizeV15(context.Background(), requirement, inventory, environment, policy)
	lazyJSON, err := json.Marshal(lazy)
	if err != nil {
		t.Fatal(err)
	}
	boundedJSON, err := json.Marshal(bounded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lazyJSON, boundedJSON) {
		t.Fatal("V15 failure compaction changed V14 synthesis evidence")
	}
}

func TestCompareSynthesisFailureV15MatchesFrozenRanking(t *testing.T) {
	left := synthesisFailure{candidateIndex: 2, penalty: 3, evaluation: SimulationEvaluation{GraphHash: "b"}}
	right := synthesisFailure{candidateIndex: 2, penalty: 3, evaluation: SimulationEvaluation{GraphHash: "c"}}
	if compareSynthesisFailureV15(left, right) >= 0 {
		t.Fatal("V15 failure compaction changed frozen graph-hash tie breaking")
	}
	right.penalty = 2
	if compareSynthesisFailureV15(left, right) <= 0 {
		t.Fatal("V15 failure compaction changed frozen penalty priority")
	}
}

func TestVisitCausalValueCandidatesV15PreservesFrozenPrefix(t *testing.T) {
	requirement, graph, inventory, _ := testSimulationFixture(t)
	policy := DefaultPolicy()
	frozen := causalValueCandidates(requirement, graph, inventory, policy)
	if len(frozen) == 0 {
		t.Fatal("fixture produced no causal value candidates")
	}
	maximum := min(3, len(frozen))
	visited := []causalCandidate{}
	visitCausalValueCandidatesV15(requirement, graph, inventory, policy, func(candidate causalCandidate) bool {
		visited = append(visited, candidate)
		return len(visited) < maximum
	})
	if len(visited) != maximum {
		t.Fatalf("visited candidates=%d, want %d", len(visited), maximum)
	}
	for index := range visited {
		gotHash, gotErr := GraphHash(visited[index].graph)
		wantHash, wantErr := GraphHash(frozen[index].graph)
		if gotErr != nil || wantErr != nil || gotHash != wantHash ||
			causalPerturbationKey(visited[index].perturbations) != causalPerturbationKey(frozen[index].perturbations) ||
			visited[index].repair.AfterGraphHash != frozen[index].repair.AfterGraphHash {
			t.Fatalf("visited candidate %d changed frozen generation order", index)
		}
	}
}

func TestHashJSONV15MatchesFrozenRepairHash(t *testing.T) {
	result := RepairSearchResult{
		Schema: RepairSearchSchema, Version: RepairSearchVersion,
		Status: RepairSearchExhausted,
		Attempts: []RepairAttempt{{
			Number: 1, GraphHash: "graph", TopologyHash: "topology",
			Status: RepairSearchFailed,
		}},
		CausalAnalyses: []CausalRepairAnalysis{},
		Issues:         []reports.Issue{},
	}
	if got, want := hashJSONV15(result), hashJSON(result); got != want {
		t.Fatalf("streamed repair hash=%s, want frozen hash %s", got, want)
	}
}

func TestHashJSONV15FailsClosedOnEncodingError(t *testing.T) {
	cycle := []any{nil}
	cycle[0] = cycle
	defer func() {
		if recover() == nil {
			t.Fatal("V15 repair hashing accepted an unencodable cyclic value")
		}
	}()
	_ = hashJSONV15(cycle)
}
