package capabilityexecutorv10

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

func TestRunV12BoundsAggregateCaseConcurrencyAndResumes(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	base := executor.synthesize
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var blocking atomic.Bool
	blocking.Store(true)
	var active, maximum atomic.Int64
	executor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		current := active.Add(1)
		defer active.Add(-1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		if blocking.Load() {
			entered <- struct{}{}
			<-release
		}
		return base(ctx, requirement, inventory, environment, policy)
	}
	request := testRequest(t, filepath.Join(t.TempDir(), "v12"))
	type runResult struct {
		hash string
		err  error
	}
	result := make(chan runResult, 1)
	go func() {
		report, err := executor.RunV12(context.Background(), request)
		result <- runResult{hash: report.Hash, err: err}
	}()
	for count := 0; count < v12ParallelCaseLimit; count++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("V12 did not start its bounded worker cohort")
		}
	}
	select {
	case <-entered:
		t.Fatal("V12 started more than two simultaneous case syntheses")
	case <-time.After(100 * time.Millisecond):
	}
	blocking.Store(false)
	close(release)
	completed := <-result
	if completed.err != nil || completed.hash == "" || maximum.Load() != v12ParallelCaseLimit {
		t.Fatalf("V12 run failed: hash=%q maximum=%d error=%v", completed.hash, maximum.Load(), completed.err)
	}
	data, err := os.ReadFile(filepath.Join(request.OutputRoot, v12EvaluationRootMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	var marker evaluationRootMarker
	if err := json.Unmarshal(data, &marker); err != nil || marker.Schema != v12EvaluationRootSchema || marker.Version != 12 || marker.ParallelCaseLimit != v12ParallelCaseLimit {
		t.Fatalf("invalid V12 evaluation root: %+v, %v", marker, err)
	}

	var resumeCalls atomic.Int64
	resumeExecutor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	resumeBase := resumeExecutor.synthesize
	resumeExecutor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		resumeCalls.Add(1)
		return resumeBase(ctx, requirement, inventory, environment, policy)
	}
	request.Resume = true
	resumed, err := resumeExecutor.RunV12(context.Background(), request)
	if err != nil || resumed.Hash != completed.hash || resumeCalls.Load() != 0 {
		t.Fatalf("V12 authenticated resume failed: calls=%d hashes=%s/%s error=%v", resumeCalls.Load(), resumed.Hash, completed.hash, err)
	}
}
