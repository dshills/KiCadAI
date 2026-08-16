package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilitybaselinev10"
)

// RunV19 keeps the frozen V17 serial two-replay transport. Versioning is bound
// by the V19 evaluator and environment digests; the checkpoint and clean-root
// protocol remains byte-identical and never opens a public outcome itself.
func (executor Executor) RunV19(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	return executor.runV17(ctx, request, releaseReplayMemoryV17)
}
