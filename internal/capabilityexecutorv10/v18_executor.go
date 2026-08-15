package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

// NewV18 binds only the explicit V18 constructor. Historical constructors and
// runners remain byte-identical and retain their frozen synthesis functions.
func NewV18() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.SynthesizeV18,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAware,
	}
}

// NewV18WithLegacy binds the V18 extension and the immutable V17 synthesis
// inputs independently. This prevents a V18-only primitive from changing an
// unrelated requirement that must delegate to V17 byte-for-byte.
func NewV18WithLegacy(
	legacyInventory opentopologysynthesis.PrimitiveInventory,
	legacySimulation opentopologysynthesis.SimulationEnvironment,
) Executor {
	executor := NewV18()
	executor.synthesize = func(
		ctx context.Context,
		requirement opentopologysynthesis.Requirement,
		inventory opentopologysynthesis.PrimitiveInventory,
		environment opentopologysynthesis.SimulationEnvironment,
		policy opentopologysynthesis.Policy,
	) opentopologysynthesis.SynthesisRun {
		return opentopologysynthesis.SynthesizeV18WithLegacy(
			ctx,
			requirement,
			inventory,
			environment,
			legacyInventory,
			legacySimulation,
			policy,
		)
	}
	return executor
}
