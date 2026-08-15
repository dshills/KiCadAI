package capabilityexecutorv10

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

func TestRunV18UsesFrozenSerialTwoReplayTransport(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	base := executor.synthesize
	var active, maximum, calls atomic.Int64
	executor.synthesize = func(
		ctx context.Context,
		requirement opentopologysynthesis.Requirement,
		inventory opentopologysynthesis.PrimitiveInventory,
		environment opentopologysynthesis.SimulationEnvironment,
		policy opentopologysynthesis.Policy,
	) opentopologysynthesis.SynthesisRun {
		current := active.Add(1)
		defer active.Add(-1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		calls.Add(1)
		return base(ctx, requirement, inventory, environment, policy)
	}
	request := testRequest(t, filepath.Join(t.TempDir(), "v18"))
	report, err := executor.RunV18(context.Background(), request)
	if err != nil || report.Hash == "" || maximum.Load() != 1 || calls.Load() != 48 {
		t.Fatalf("V18 transport = hash=%q maximum=%d calls=%d error=%v", report.Hash, maximum.Load(), calls.Load(), err)
	}
}
