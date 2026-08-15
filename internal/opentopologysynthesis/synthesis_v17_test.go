package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestSynthesisValueTrialV17RetainsNoMaterializedGraph(t *testing.T) {
	typeOf := reflect.TypeOf(synthesisValueTrialV17{})
	graphType := reflect.TypeOf(CandidateGraph{})
	for index := 0; index < typeOf.NumField(); index++ {
		if typeOf.Field(index).Type == graphType {
			t.Fatalf("V17 value-trial scheduler retains a materialized candidate graph in field %q", typeOf.Field(index).Name)
		}
	}
}

func TestSynthesizeV17PreservesFrozenSynthesisOutcomeWithBoundedEvidence(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 1_000
	policy.MaxGeneratedGraphs = 5_000
	policy.MaxRetainedCandidates = 4
	policy.MaxValueTrials = 4
	policy.MaxTopologyRepairs = 2
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1_024

	legacy := SynthesizeV16(context.Background(), requirement, inventory, environment, policy)
	bounded := SynthesizeV17(context.Background(), requirement, inventory, environment, policy)
	if bounded.Report.Status != legacy.Report.Status || bounded.Report.StopReason != legacy.Report.StopReason {
		t.Fatalf("V17 outcome changed: got %s/%s want %s/%s", bounded.Report.Status, bounded.Report.StopReason, legacy.Report.Status, legacy.Report.StopReason)
	}
	if (bounded.SelectedGraph == nil) != (legacy.SelectedGraph == nil) {
		t.Fatal("V17 changed whether a graph was selected")
	}
	if bounded.SelectedGraph != nil {
		got, gotErr := GraphHash(*bounded.SelectedGraph)
		want, wantErr := GraphHash(*legacy.SelectedGraph)
		if gotErr != nil || wantErr != nil || got != want {
			t.Fatalf("V17 selected graph changed: got %s/%v want %s/%v", got, gotErr, want, wantErr)
		}
	}
}

func TestCompareSynthesisFailureV17MatchesFrozenRanking(t *testing.T) {
	left := synthesisFailure{candidateIndex: 2, penalty: 3, evaluation: SimulationEvaluation{GraphHash: "b"}}
	right := synthesisFailure{candidateIndex: 2, penalty: 3, evaluation: SimulationEvaluation{GraphHash: "c"}}
	if compareSynthesisFailureV17(left, right) >= 0 {
		t.Fatal("V17 failure compaction changed frozen graph-hash tie breaking")
	}
	right.penalty = 2
	if compareSynthesisFailureV17(left, right) <= 0 {
		t.Fatal("V17 failure compaction changed frozen penalty priority")
	}
}

func TestVisitCausalValueCandidatesV17PreservesFrozenPrefix(t *testing.T) {
	requirement, graph, inventory, _ := testSimulationFixture(t)
	policy := DefaultPolicy()
	frozen := causalValueCandidates(requirement, graph, inventory, policy)
	if len(frozen) == 0 {
		t.Fatal("fixture produced no causal value candidates")
	}
	maximum := min(3, len(frozen))
	visited := []causalCandidate{}
	visitCausalValueCandidatesV17(requirement, graph, inventory, policy, func(candidate causalCandidate) bool {
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

func TestHashJSONV17MatchesFrozenRepairHash(t *testing.T) {
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
	if got, want := hashJSONV17(result), hashJSON(result); got != want {
		t.Fatalf("streamed repair hash=%s, want frozen hash %s", got, want)
	}
}

func TestHashJSONV17FailsClosedOnEncodingError(t *testing.T) {
	cycle := []any{nil}
	cycle[0] = cycle
	defer func() {
		if recover() == nil {
			t.Fatal("V17 repair hashing accepted an unencodable cyclic value")
		}
	}()
	_ = hashJSONV17(cycle)
}

func TestFinalizeSynthesisRunV17PreservesFrozenEvidenceAndHash(t *testing.T) {
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
	streamed := finalizeSynthesisRunV17(run)
	frozenJSON, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	streamedJSON, err := json.Marshal(streamed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(streamedJSON, frozenJSON) {
		t.Fatal("V17 streaming finalization changed frozen synthesis evidence")
	}
}
