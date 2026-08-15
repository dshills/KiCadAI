package capabilityexecutorv10

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

func TestRunV11PreservesFrozenSyntheticEvidenceWithoutRetainingSpools(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	request := testRequest(t, filepath.Join(t.TempDir(), "first"))
	first, err := executor.RunV11(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.OutputRoot = filepath.Join(t.TempDir(), "second")
	second, err := executor.RunV11(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	const frozenV11SyntheticHash = "f07d6fba4f396a67b0de2026799c7279544a3cfa15ef81f9038db111266c049d"
	if first.Hash != frozenV11SyntheticHash || second.Hash != frozenV11SyntheticHash {
		t.Fatalf("V11 evidence is nondeterministic: %s %s", first.Hash, second.Hash)
	}
	legacyRequest := testRequest(t, filepath.Join(t.TempDir(), "legacy"))
	legacy, err := executor.Run(context.Background(), legacyRequest)
	if err != nil {
		t.Fatal(err)
	}
	for index := range first.Cases {
		current, prior := first.Cases[index], legacy.Cases[index]
		if !reflect.DeepEqual(current.Case, prior.Case) || current.RequirementSHA256 != prior.RequirementSHA256 ||
			current.EnvironmentSHA256 != prior.EnvironmentSHA256 || current.EvaluatorManifestSHA256 != prior.EvaluatorManifestSHA256 ||
			!reflect.DeepEqual(current.ReplaySHA256, prior.ReplaySHA256) || !reflect.DeepEqual(current.Gates, prior.Gates) ||
			!reflect.DeepEqual(current.Promotions, prior.Promotions) {
			t.Fatalf("V11 semantic evidence differs from V10 for case %d", index+1)
		}
	}
	marker := filepath.Join(request.OutputRoot, v11EvaluationRootMarkerName)
	var root evaluationRootMarker
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &root); err != nil || root.Schema != v11EvaluationRootSchema || root.Version != 11 {
		t.Fatalf("invalid V11 evaluation root: %+v, %v", root, err)
	}
	if _, err := os.Lstat(filepath.Join(request.OutputRoot, evaluationRootMarkerName)); !os.IsNotExist(err) {
		t.Fatalf("V11 emitted a V10 root marker: %v", err)
	}
	for replay := 1; replay <= 2; replay++ {
		spool := filepath.Join(request.OutputRoot, "v10_case_001", fmt.Sprintf("replay-%d", replay), replaySpoolNameV11)
		if _, err := os.Lstat(spool); !os.IsNotExist(err) {
			t.Fatalf("completed V11 replay spool %d remains: %v", replay, err)
		}
	}
}

func TestV11ReplaySpoolMatchesJSONMarshalAndRefusesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), replaySpoolNameV11)
	run := opentopologysynthesis.SynthesisRun{
		Schema: opentopologysynthesis.SynthesisRunSchema, Version: opentopologysynthesis.SynthesisRunVersion,
		Hash: testDigest("streamed-run"), Report: opentopologysynthesis.Report{
			Status:      opentopologysynthesis.StatusUnsupported,
			Consumption: opentopologysynthesis.Consumption{CandidateSimulations: 3},
		},
		Candidates: []opentopologysynthesis.SynthesisCandidateEvidence{},
	}
	want, err := json.Marshal(&run)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := writeReplaySpoolV11(path, &run)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o444 || string(data) != string(want) || digest != testRawDigest(want) {
		t.Fatalf("invalid V11 replay spool: mode=%v digest=%s", info.Mode(), digest)
	}
	if _, err := writeReplaySpoolV11(path, &run); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("V11 replay spool replacement error = %v", err)
	}
}

func TestRunV11ReleasesFirstReplayBeforeSecondSynthesis(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	output := filepath.Join(t.TempDir(), "sequenced")
	request := testRequest(t, output)
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	environmentSHA256, err := validateEnvironment(request.Environment)
	if err != nil {
		t.Fatal(err)
	}
	baseSynthesize, baseObserve := executor.synthesize, executor.observe
	var synthesisCalls atomic.Int64
	executor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		synthesisCalls.Add(1)
		return baseSynthesize(ctx, requirement, inventory, environment, policy)
	}
	executor.observe = func(meta capabilityfeedback.CaseMeta, requirement opentopologysynthesis.Requirement, run opentopologysynthesis.SynthesisRun, promotion *opentopologysynthesis.PhysicalPromotionResult) (capabilityfeedback.CaseEvidence, error) {
		if calls := synthesisCalls.Load(); calls != 1 {
			return capabilityfeedback.CaseEvidence{}, fmt.Errorf("first replay remained live until after second synthesis: calls=%d", calls)
		}
		return baseObserve(meta, requirement, run, promotion)
	}
	result, err := executor.runCaseV11(context.Background(), output, request.CorpusManifestSHA256, request.Cases[0], request.Environment, environmentSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if synthesisCalls.Load() != 2 {
		t.Fatalf("V11 synthesis calls = %d, want 2", synthesisCalls.Load())
	}
	if err := removeReplaySpoolsV11(result.replaySpools); err != nil {
		t.Fatal(err)
	}
}

func TestRunV11ResumesOnlyAuthenticatedCheckpoints(t *testing.T) {
	root := filepath.Join(t.TempDir(), "resume")
	request := testRequest(t, root)
	first, err := testExecutor(capabilityfeedback.OutcomeUnsupported).RunV11(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	base := executor.synthesize
	executor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		calls.Add(1)
		return base(ctx, requirement, inventory, environment, policy)
	}
	request.Resume = true
	resumed, err := executor.RunV11(context.Background(), request)
	if err != nil || calls.Load() != 0 || resumed.Hash != first.Hash {
		t.Fatalf("V11 full resume calls=%d hashes=%s/%s error=%v", calls.Load(), resumed.Hash, first.Hash, err)
	}
	if err := os.Remove(caseCheckpointPath(root, "v10_case_007")); err != nil {
		t.Fatal(err)
	}
	calls.Store(0)
	partial, err := executor.RunV11(context.Background(), request)
	if err != nil || calls.Load() != 2 || partial.Hash != first.Hash {
		t.Fatalf("V11 partial resume calls=%d hashes=%s/%s error=%v", calls.Load(), partial.Hash, first.Hash, err)
	}
}
