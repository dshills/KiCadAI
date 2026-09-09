package capabilityexecutorv10

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

func testExecutorV22(t *testing.T, request Request, predecessor Executor) ExecutorV22 {
	t.Helper()
	result := ExecutorV22{predecessor: predecessor, selected: map[string]bool{}, cohort: map[string]string{}}
	for _, input := range request.Cases {
		hash, err := inputBindingV22(input)
		if err != nil {
			t.Fatal(err)
		}
		result.cohort[input.Entry.ID] = hash
	}
	result.successor = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.ElectricalSynthesisResultV22 {
		run := predecessor.synthesize(ctx, requirement, inventory, environment, policy)
		return opentopologysynthesis.ElectricalSynthesisResultV22{PredecessorHash: run.Hash, Synthesis: run, Hash: testDigest("electrical")}
	}
	return result
}

func TestRunV22PreservesFullReplayAndEliminatesDiskSpools(t *testing.T) {
	request := testRequest(t, filepath.Join(t.TempDir(), "v22"))
	base := testExecutor(capabilityfeedback.OutcomeUnsupported)
	executor := testExecutorV22(t, request, base)
	executor.selected[request.Cases[0].Entry.ID] = true
	old := executor.predecessor.synthesize
	oldSuccessor := executor.successor
	baseCalls, successorCalls := 0, 0
	executor.predecessor.synthesize = func(ctx context.Context, r opentopologysynthesis.Requirement, i opentopologysynthesis.PrimitiveInventory, e opentopologysynthesis.SimulationEnvironment, p opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		baseCalls++
		return old(ctx, r, i, e, p)
	}
	executor.successor = func(ctx context.Context, r opentopologysynthesis.Requirement, i opentopologysynthesis.PrimitiveInventory, e opentopologysynthesis.SimulationEnvironment, p opentopologysynthesis.Policy) opentopologysynthesis.ElectricalSynthesisResultV22 {
		successorCalls++
		return oldSuccessor(ctx, r, i, e, p)
	}
	report, err := executor.RunV22(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if baseCalls != 46 || successorCalls != 2 || len(report.Cases) != 24 {
		t.Fatalf("calls: predecessor=%d successor=%d", baseCalls, successorCalls)
	}
	oldRequest := request
	oldRequest.OutputRoot = filepath.Join(t.TempDir(), "v21")
	prior, err := base.RunV21(context.Background(), oldRequest)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range report.Cases {
		if !reflect.DeepEqual(c.ReplaySHA256, prior.Cases[i].ReplaySHA256) || !reflect.DeepEqual(c.Gates, prior.Cases[i].Gates) || !reflect.DeepEqual(c.Case, prior.Cases[i].Case) {
			t.Fatalf("case %d replay or decision differs", i)
		}
	}
	metricsCount, sidecarCount := 0, 0
	err = filepath.WalkDir(request.OutputRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == replaySpoolNameV11 {
			return fmt.Errorf("unexpected synthesis spool")
		}
		if d.Name() == "ELECTRICAL_REPAIR.json" {
			sidecarCount++
		}
		if d.Name() != "METRICS.json" {
			return nil
		}
		metricsCount++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var metrics replayMetricsV22
		if err := json.Unmarshal(data, &metrics); err != nil {
			return err
		}
		if metrics.CanonicalSynthesisBytes <= 0 || metrics.SynthesisSpoolBytes != 0 || metrics.SynthesisNanoseconds < 0 || metrics.HashNanoseconds < 0 {
			return fmt.Errorf("invalid measurement")
		}
		return nil
	})
	if err != nil || metricsCount != 48 || sidecarCount != 48 {
		t.Fatalf("artifacts: metrics=%d sidecars=%d err=%v", metricsCount, sidecarCount, err)
	}
	if _, err := executor.RunV22(context.Background(), request); err == nil {
		t.Fatal("existing root accepted")
	}
}

func TestHashReplayV22ExactlyMatchesLegacySpool(t *testing.T) {
	run := testExecutor(capabilityfeedback.OutcomeUnsupported).synthesize(context.Background(), opentopologysynthesis.Requirement{}, opentopologysynthesis.PrimitiveInventory{}, opentopologysynthesis.SimulationEnvironment{}, opentopologysynthesis.DefaultPolicy())
	path := filepath.Join(t.TempDir(), "spool.json")
	oldHash, err := writeReplaySpoolV11(path, &run)
	if err != nil {
		t.Fatal(err)
	}
	hash, count, err := hashReplayV22(context.Background(), &run)
	info, statErr := os.Stat(path)
	if err != nil || statErr != nil || hash != oldHash || count != info.Size() {
		t.Fatalf("hash=%s old=%s bytes=%d err=%v/%v", hash, oldHash, count, err, statErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := hashReplayV22(ctx, &run); err == nil {
		t.Fatal("canceled hash succeeded")
	}
}

func TestRunV22RejectsDriftAndInvalidRequestsWithoutRetry(t *testing.T) {
	for _, mode := range []string{"synthesis", "sidecar", "resume", "metadata", "order", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			request := testRequest(t, filepath.Join(t.TempDir(), "v22"))
			executor := testExecutorV22(t, request, testExecutor(capabilityfeedback.OutcomeUnsupported))
			executor.selected[request.Cases[0].Entry.ID] = true
			calls := 0
			base := executor.successor
			executor.successor = func(ctx context.Context, r opentopologysynthesis.Requirement, i opentopologysynthesis.PrimitiveInventory, e opentopologysynthesis.SimulationEnvironment, p opentopologysynthesis.Policy) opentopologysynthesis.ElectricalSynthesisResultV22 {
				calls++
				run := base(ctx, r, i, e, p)
				if mode == "synthesis" {
					run.Synthesis.Hash = testDigest(fmt.Sprint(calls))
				}
				if mode == "sidecar" {
					run.Hash = testDigest(fmt.Sprint(calls))
				}
				return run
			}
			ctx := context.Background()
			switch mode {
			case "resume":
				request.Resume = true
			case "metadata":
				request.Cases[0].Entry.SafetyImpact = "non_safety"
			case "order":
				request.Cases[0], request.Cases[1] = request.Cases[1], request.Cases[0]
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := executor.RunV22(ctx, request); err == nil {
				t.Fatal("invalid evaluation accepted")
			}
			wantCalls := 0
			if mode == "synthesis" || mode == "sidecar" {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want=%d", calls, wantCalls)
			}
		})
	}
}

func TestV22PublicPopulationBinding(t *testing.T) {
	corpus, err := LoadPublicDiscovery(filepath.Join("..", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest(t, filepath.Join(t.TempDir(), "unused"))
	base := testExecutorV22(t, request, testExecutor(capabilityfeedback.OutcomeUnsupported))
	first := corpus.Cases[0].Entry.ID
	if _, err := bindElectricalPopulationV22(base.predecessor, base.successor, corpus.Cases, []string{first}); err != nil {
		t.Fatal(err)
	}
	for _, selected := range [][]string{nil, {""}, {first, first}, {"unknown"}} {
		if _, err := bindElectricalPopulationV22(base.predecessor, base.successor, corpus.Cases, selected); err == nil {
			t.Fatalf("invalid selection %v admitted", selected)
		}
	}
	duplicate := append([]CaseInput(nil), corpus.Cases...)
	duplicate[1] = duplicate[0]
	if _, err := bindElectricalPopulationV22(base.predecessor, base.successor, duplicate, []string{first}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := NewSelectedV22WithLegacy(corpus.Cases, []string{first}, []string{corpus.Cases[1].Entry.ID}, opentopologysynthesis.PrimitiveInventory{}, opentopologysynthesis.SimulationEnvironment{}, opentopologysynthesis.PrimitiveInventory{}, opentopologysynthesis.SimulationEnvironment{}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("selection widened V21: %v", err)
	}
}

func TestRunV22PromotesEveryPassingReplay(t *testing.T) {
	request := testRequest(t, filepath.Join(t.TempDir(), "v22"))
	base := testExecutor(capabilityfeedback.OutcomePass)
	calls := 0
	base.promote = func(context.Context, opentopologysynthesis.SynthesisRun, opentopologysynthesis.SimulationEnvironment, opentopologysynthesis.PhysicalPromotionOptions) opentopologysynthesis.PhysicalPromotionResult {
		calls++
		return opentopologysynthesis.PhysicalPromotionResult{Status: opentopologysynthesis.PhysicalPromotionPassed, Hash: testDigest("promotion"), ProjectHash: testDigest("project"), ReplayIdentical: true, Runs: []opentopologysynthesis.PhysicalPromotionRun{{Number: 1, ProjectHash: testDigest("project")}, {Number: 2, ProjectHash: testDigest("project")}}}
	}
	executor := testExecutorV22(t, request, base)
	report, err := executor.RunV22(context.Background(), request)
	if err != nil || calls != 48 {
		t.Fatalf("promotions=%d err=%v", calls, err)
	}
	for _, c := range report.Cases {
		if c.Case.Outcome != "pass" || len(c.Promotions) != 2 {
			t.Fatal("missing full promotion")
		}
	}
}
