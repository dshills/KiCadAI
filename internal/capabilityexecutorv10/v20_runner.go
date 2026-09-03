package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilitybaselinev10"
)

// RunV20 reuses the serial two-replay transport. Versioning is bound by the
// separately frozen V20 evaluator and environment digests.
func (executor Executor) RunV20(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	return executor.runV17(ctx, request, releaseReplayMemoryV17)
}
