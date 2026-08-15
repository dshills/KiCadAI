package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestSynthesisValueTrialV16RetainsNoMaterializedGraph(t *testing.T) {
	typeOf := reflect.TypeOf(synthesisValueTrialV16{})
	graphType := reflect.TypeOf(CandidateGraph{})
	for index := 0; index < typeOf.NumField(); index++ {
		if typeOf.Field(index).Type == graphType {
			t.Fatalf("V16 value-trial scheduler retains a materialized candidate graph in field %q", typeOf.Field(index).Name)
		}
	}
}

func TestSynthesizeV16PreservesFrozenSynthesisEvidence(t *testing.T) {
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
	bounded := SynthesizeV16(context.Background(), requirement, inventory, environment, policy)
	lazyJSON, err := json.Marshal(lazy)
	if err != nil {
		t.Fatal(err)
	}
	boundedJSON, err := json.Marshal(bounded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lazyJSON, boundedJSON) {
		t.Fatal("V16 failure compaction changed V14 synthesis evidence")
	}
}

func TestCompareSynthesisFailureV16MatchesFrozenRanking(t *testing.T) {
	left := synthesisFailure{candidateIndex: 2, penalty: 3, evaluation: SimulationEvaluation{GraphHash: "b"}}
	right := synthesisFailure{candidateIndex: 2, penalty: 3, evaluation: SimulationEvaluation{GraphHash: "c"}}
	if compareSynthesisFailureV16(left, right) >= 0 {
		t.Fatal("V16 failure compaction changed frozen graph-hash tie breaking")
	}
	right.penalty = 2
	if compareSynthesisFailureV16(left, right) <= 0 {
		t.Fatal("V16 failure compaction changed frozen penalty priority")
	}
}

func TestVisitCausalValueCandidatesV16PreservesFrozenPrefix(t *testing.T) {
	requirement, graph, inventory, _ := testSimulationFixture(t)
	policy := DefaultPolicy()
	frozen := causalValueCandidates(requirement, graph, inventory, policy)
	if len(frozen) == 0 {
		t.Fatal("fixture produced no causal value candidates")
	}
	maximum := min(3, len(frozen))
	visited := []causalCandidate{}
	visitCausalValueCandidatesV16(requirement, graph, inventory, policy, func(candidate causalCandidate) bool {
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

func TestHashJSONV16MatchesFrozenRepairHash(t *testing.T) {
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
	if got, want := hashJSONV16(result), hashJSON(result); got != want {
		t.Fatalf("streamed repair hash=%s, want frozen hash %s", got, want)
	}
}

func TestHashJSONV16FailsClosedOnEncodingError(t *testing.T) {
	cycle := []any{nil}
	cycle[0] = cycle
	defer func() {
		if recover() == nil {
			t.Fatal("V16 repair hashing accepted an unencodable cyclic value")
		}
	}()
	_ = hashJSONV16(cycle)
}

func TestFinalizeSynthesisRunV16PreservesFrozenEvidenceAndHash(t *testing.T) {
	run := SynthesisRun{
		Schema:  SynthesisRunSchema,
		Version: SynthesisRunVersion,
		Report: Report{
			Schema:      ReportSchema,
			Version:     ReportVersion,
			Status:      StatusExhausted,
			StopReason:  StopSearchExhausted,
			Candidates:  []CandidateReport{},
			Diagnostics: []Diagnostic{},
		},
		Candidates: []SynthesisCandidateEvidence{},
	}
	frozen := finalizeSynthesisRun(run)
	streamed := finalizeSynthesisRunV16(run)
	frozenJSON, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	streamedJSON, err := json.Marshal(streamed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(streamedJSON, frozenJSON) {
		t.Fatal("V16 streaming finalization changed frozen synthesis evidence")
	}
}
