package electricaldiagnostics

import (
	"encoding/json"
	"strings"
	"testing"

	ot "kicadai/internal/opentopologysynthesis"
)

func certifiedRun(t *testing.T, operations bool) ot.SynthesisRun {
	t.Helper()
	graph := ot.CandidateGraph{Schema: ot.CandidateGraphSchema, Version: ot.CandidateGraphVersion, Nodes: []ot.GraphNode{}, Instances: []ot.GraphInstance{}}
	graphHash, err := ot.GraphHash(graph)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := signedEvaluation(t, ot.SimulationAttempt{Status: ot.SimulationEvaluationUnsupported, Diagnostics: []ot.SimulationDiagnostic{{Code: "MISSING_MODEL"}}})
	evaluation.GraphHash = graphHash
	sealEvaluation(t, &evaluation)
	invariant := ot.TopologyInvariantReportV21{RequirementHash: evaluation.RequirementHash, InventoryHash: evaluation.InventoryHash, GraphHash: graphHash, Complete: true, Obligations: []ot.TopologyObligationV21{}}
	invariant.Hash, err = hash(invariant)
	if err != nil {
		t.Fatal(err)
	}
	selected := ot.TopologyCandidateEvidenceV21{Graph: graph, GraphHash: graphHash, Invariant: invariant, Operations: []ot.TopologyOperationEvidenceV21{}}
	repair := ot.RepairSearchResult{InitialEvaluationHash: evaluation.Hash, TopologyCompletionV21: &ot.TopologyCompletionPlanV21{Status: "complete", Selected: &selected}}
	if operations {
		selected.Operations = []ot.TopologyOperationEvidenceV21{{Kind: ot.TopologyOperationConnectPortV21}}
		repair.InitialEvaluationHash = strings.Repeat("9", 64)
		repair.Attempts = []ot.RepairAttempt{{GraphHash: graphHash, Evaluation: evaluation}}
	}
	run := ot.SynthesisRun{Schema: ot.SynthesisRunSchema, Version: ot.SynthesisRunVersion,
		Report:     ot.Report{RequirementHash: evaluation.RequirementHash, PrimitiveInventoryHash: evaluation.InventoryHash, Status: ot.StatusFailed, StopReason: ot.StopNoPassingGraph},
		Candidates: []ot.SynthesisCandidateEvidence{{Fingerprint: "candidate", Repair: &repair, Evaluations: []ot.SimulationEvaluation{evaluation}}},
	}
	sealRun(t, &run)
	return run
}

func sealRun(t *testing.T, run *ot.SynthesisRun) {
	t.Helper()
	run.Hash = ""
	var err error
	run.Hash, _, err = streamedHash(*run)
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectSynthesisSelectsExactCertificateEvaluation(t *testing.T) {
	for _, operations := range []bool{false, true} {
		run := certifiedRun(t, operations)
		projection, err := ProjectSynthesis(run)
		if err != nil {
			t.Fatal(err)
		}
		if len(projection.CertifiedCandidates) != 1 || projection.CertifiedCandidates[0].Evaluation.FirstFailure.Stage != StageAdmission {
			t.Fatalf("unexpected projection %+v", projection)
		}
		data, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		expectedHash, _ := hash(run)
		if projection.RawSerializedBytes != int64(len(data)) || projection.ReplayHash != expectedHash {
			t.Fatal("streamed measurement differs from frozen JSON bytes")
		}
		first, _ := Marshal(projection)
		replayed, err := ProjectSynthesis(run)
		if err != nil {
			t.Fatal(err)
		}
		second, _ := Marshal(replayed)
		if string(first) != string(second) {
			t.Fatal("trace does not replay")
		}
	}
}

func TestProjectSynthesisRejectsMissingOrMismatchedBindings(t *testing.T) {
	for _, mutate := range []func(*ot.SynthesisRun){
		func(run *ot.SynthesisRun) { run.Candidates[0].Repair.InitialEvaluationHash = "missing" },
		func(run *ot.SynthesisRun) {
			run.Candidates[0].Repair.TopologyCompletionV21.Selected.Invariant.GraphHash = "wrong"
		},
		func(run *ot.SynthesisRun) {
			run.Candidates[0].Repair.TopologyCompletionV21.Selected.Invariant.Hash = "wrong"
		},
		func(run *ot.SynthesisRun) {
			run.Candidates[0].Repair.TopologyCompletionV21.Selected.Invariant.Contradictory = true
		},
		func(run *ot.SynthesisRun) { run.Candidates[0].Evaluations = nil },
	} {
		run := certifiedRun(t, false)
		mutate(&run)
		sealRun(t, &run)
		if _, err := ProjectSynthesis(run); err == nil {
			t.Fatal("inconsistent binding accepted")
		}
	}
	run := certifiedRun(t, false)
	run.Report.Status = ot.StatusPassed
	if _, err := ProjectSynthesis(run); err == nil {
		t.Fatal("tampered run accepted")
	}
}

func TestTraceBoundRefusesWithoutTruncation(t *testing.T) {
	if _, err := Marshal(Synthesis{Hash: strings.Repeat("x", MaximumTraceBytes)}); err == nil {
		t.Fatal("oversized trace accepted")
	}
}
