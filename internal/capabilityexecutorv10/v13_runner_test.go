package capabilityexecutorv10

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

func TestRunV13SerializesCasesReleasesEveryReplayAndResumes(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	base := executor.synthesize
	var active, maximum, synthesisCalls, releaseCalls atomic.Int64
	executor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		current := active.Add(1)
		defer active.Add(-1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		synthesisCalls.Add(1)
		return base(ctx, requirement, inventory, environment, policy)
	}
	originalRelease := releaseReplayMemoryV13
	releaseReplayMemoryV13 = func() { releaseCalls.Add(1) }
	t.Cleanup(func() { releaseReplayMemoryV13 = originalRelease })

	request := testRequest(t, filepath.Join(t.TempDir(), "v13"))
	report, err := executor.RunV13(context.Background(), request)
	if err != nil || report.Hash == "" || maximum.Load() != v13ParallelCaseLimit || synthesisCalls.Load() != 48 || releaseCalls.Load() != 48 {
		t.Fatalf("V13 run failed: hash=%q maximum=%d syntheses=%d releases=%d error=%v", report.Hash, maximum.Load(), synthesisCalls.Load(), releaseCalls.Load(), err)
	}
	data, err := os.ReadFile(filepath.Join(request.OutputRoot, v13EvaluationRootMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	var marker evaluationRootMarker
	if err := json.Unmarshal(data, &marker); err != nil || marker.Schema != v13EvaluationRootSchema || marker.Version != 13 || marker.ParallelCaseLimit != v13ParallelCaseLimit {
		t.Fatalf("invalid V13 evaluation root: %+v, %v", marker, err)
	}

	var resumeCalls atomic.Int64
	resumeExecutor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	resumeBase := resumeExecutor.synthesize
	resumeExecutor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		resumeCalls.Add(1)
		return resumeBase(ctx, requirement, inventory, environment, policy)
	}
	request.Resume = true
	resumed, err := resumeExecutor.RunV13(context.Background(), request)
	if err != nil || resumed.Hash != report.Hash || resumeCalls.Load() != 0 {
		t.Fatalf("V13 authenticated resume failed: calls=%d hashes=%s/%s error=%v", resumeCalls.Load(), resumed.Hash, report.Hash, err)
	}
}
