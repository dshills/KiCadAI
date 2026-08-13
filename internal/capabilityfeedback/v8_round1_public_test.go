package capabilityfeedback

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

func TestClosedLoopV8SelectedPublicDCSweepCapabilityAdvances(t *testing.T) {
	var selection closedLoopV8SelectionDecision
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8SelectionRoot, "selection.json")), &selection)
	manifestSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, corpuspublication.ManifestFileV8))
	var manifest corpuspublication.ManifestV8
	decodeCorpusStrict(t, manifestSource, &manifest)
	entries := make(map[string]corpuspublication.EntryV8, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		entries[entry.ID] = entry
	}
	var baseline closedLoopV8BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "report.json")), &baseline)
	artifacts := map[string]string{}
	for _, artifact := range baseline.CaseArtifacts {
		artifacts[artifact.CaseID] = artifact.Path
	}
	_, policy := closedLoopV4Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	// The frozen V8 evaluator requires outcome-affecting subprocess concurrency
	// one, so selected cases and their two replays intentionally remain serial.
	for _, caseID := range selection.Selected.CoveredCaseIDs {
		entry, found := entries[caseID]
		if !found {
			t.Fatalf("selected V8 discovery case %s is absent from the authenticated manifest", caseID)
		}
		requirementSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, filepath.FromSlash(entry.StablePath)))
		if corpusHash(requirementSource) != entry.RequirementSHA256 {
			t.Fatalf("selected V8 discovery case %s differs from its raw commitment", caseID)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementSource))
		if len(issues) != 0 {
			t.Fatalf("selected V8 discovery case %s violates the frozen contract: %#v", caseID, issues)
		}
		artifactPath, found := artifacts[caseID]
		if !found {
			t.Fatalf("selected V8 discovery case %s has no committed baseline artifact", caseID)
		}
		artifact := loadClosedLoopV8Round1BaselineArtifact(t, caseID, artifactPath)
		if len(artifact.Replays) != 2 || len(artifact.Replays[0].Search.Candidates) == 0 {
			t.Fatalf("selected V8 discovery case %s has no committed candidate graph", caseID)
		}
		graph := artifact.Replays[0].Search.Candidates[0].Graph
		first, second := evaluateClosedLoopV8CandidateTwice(requirement, graph, inventory, environment, policy)
		if first.Hash == "" {
			t.Fatalf("selected V8 discovery case %s produced an empty evaluation hash", caseID)
		}
		if second.Hash != first.Hash {
			t.Fatalf("selected V8 discovery case %s is not deterministic: %s != %s", caseID, first.Hash, second.Hash)
		}
		dcSweepAttempts := 0
		for _, attempt := range first.Attempts {
			if attempt.Analysis != "dc_sweep" {
				continue
			}
			dcSweepAttempts++
		}
		for _, diagnosis := range first.Diagnoses {
			if diagnosis.Analysis == "dc_sweep" && diagnosis.Code == "SIMULATION_INVALID" {
				t.Fatalf("selected V8 discovery case %s retained an invalid DC sweep: %#v", caseID, diagnosis)
			}
		}
		if dcSweepAttempts == 0 {
			t.Fatalf("selected V8 discovery case %s produced no DC-sweep evidence", caseID)
		}
	}
}

func evaluateClosedLoopV8CandidateTwice(
	requirement ots.Requirement,
	graph ots.CandidateGraph,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
	policy ots.Policy,
) (ots.SimulationEvaluation, ots.SimulationEvaluation) {
	return evaluateClosedLoopV8Candidate(requirement, graph, inventory, environment, policy),
		evaluateClosedLoopV8Candidate(requirement, graph, inventory, environment, policy)
}

func evaluateClosedLoopV8Candidate(
	requirement ots.Requirement,
	graph ots.CandidateGraph,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
	policy ots.Policy,
) ots.SimulationEvaluation {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return ots.EvaluateCandidate(ctx, requirement, graph, nil, inventory, environment, policy)
}

func loadClosedLoopV8Round1BaselineArtifact(t *testing.T, caseID, artifactPath string) closedLoopV8CaseArtifact {
	t.Helper()
	source := mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, filepath.FromSlash(artifactPath)))
	compressed, err := gzip.NewReader(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	var artifact closedLoopV8CaseArtifact
	decoder := json.NewDecoder(compressed)
	decoder.DisallowUnknownFields()
	decodeErr := decoder.Decode(&artifact)
	trailingErr := decoder.Decode(&struct{}{})
	closeErr := compressed.Close()
	if decodeErr != nil {
		t.Fatalf("decode selected V8 discovery case %s baseline evidence: %v", caseID, decodeErr)
	}
	if trailingErr != io.EOF {
		t.Fatalf("selected V8 discovery case %s has trailing baseline evidence", caseID)
	}
	if closeErr != nil {
		t.Fatalf("close selected V8 discovery case %s baseline evidence: %v", caseID, closeErr)
	}
	return artifact
}
