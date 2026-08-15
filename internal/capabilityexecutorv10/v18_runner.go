package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilitybaselinev10"
)

// RunV18 binds the explicit V18 synthesis constructor to the already frozen
// V17 serial two-replay transport. The transport schema remains V17 on purpose:
// the corpus and environment digests provide the versioned evaluator binding,
// while reusing the byte-identical checkpoint and clean-root protocol avoids a
// second mutable execution engine.
func (executor Executor) RunV18(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	return executor.runV17(ctx, request, releaseReplayMemoryV17)
}
