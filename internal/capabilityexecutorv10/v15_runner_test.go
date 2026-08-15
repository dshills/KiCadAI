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

func TestRunV15SerializesCasesReleasesEveryReplayAndResumes(t *testing.T) {
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
	request := testRequest(t, filepath.Join(t.TempDir(), "v15"))
	report, err := executor.runV15(context.Background(), request, func() { releaseCalls.Add(1) })
	if err != nil || report.Hash == "" || maximum.Load() != v15ParallelCaseLimit || synthesisCalls.Load() != 48 || releaseCalls.Load() != 48 {
		t.Fatalf("V15 run failed: hash=%q maximum=%d syntheses=%d releases=%d error=%v", report.Hash, maximum.Load(), synthesisCalls.Load(), releaseCalls.Load(), err)
	}
	data, err := os.ReadFile(filepath.Join(request.OutputRoot, v15EvaluationRootMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	var marker evaluationRootMarker
	if err := json.Unmarshal(data, &marker); err != nil || marker.Schema != v15EvaluationRootSchema || marker.Version != 15 || marker.ParallelCaseLimit != v15ParallelCaseLimit {
		t.Fatalf("invalid V15 evaluation root: %+v, %v", marker, err)
	}

	var resumeCalls atomic.Int64
	resumeExecutor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	resumeBase := resumeExecutor.synthesize
	resumeExecutor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, environment opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		resumeCalls.Add(1)
		return resumeBase(ctx, requirement, inventory, environment, policy)
	}
	request.Resume = true
	resumed, err := resumeExecutor.runV15(context.Background(), request, func() { releaseCalls.Add(1) })
	if err != nil || resumed.Hash != report.Hash || resumeCalls.Load() != 0 {
		t.Fatalf("V15 authenticated resume failed: calls=%d hashes=%s/%s error=%v", resumeCalls.Load(), resumed.Hash, report.Hash, err)
	}
}
