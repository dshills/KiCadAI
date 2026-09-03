package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilitybaselinev10"
)

// RunV21 retains the frozen serial two-replay transport. Structural proposal
// analysis may use workers internally, but case order and evidence do not.
func (executor Executor) RunV21(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	return executor.runV17(ctx, request, releaseReplayMemoryV17)
}
