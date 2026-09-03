package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

func NewV21() Executor {
	return Executor{
		decode: decodeRequirement, synthesize: opentopologysynthesis.SynthesizeV21,
		promote: opentopologysynthesis.PromoteSynthesisRun, observe: capabilityfeedback.ObserveRealizabilityAwareV21,
	}
}

func NewV21WithLegacy(
	v20Inventory opentopologysynthesis.PrimitiveInventory,
	v20Simulation opentopologysynthesis.SimulationEnvironment,
	v18Inventory opentopologysynthesis.PrimitiveInventory,
	v18Simulation opentopologysynthesis.SimulationEnvironment,
	legacyInventory opentopologysynthesis.PrimitiveInventory,
	legacySimulation opentopologysynthesis.SimulationEnvironment,
) Executor {
	executor := NewV21()
	executor.synthesize = func(ctx context.Context, requirement opentopologysynthesis.Requirement, v21Inventory opentopologysynthesis.PrimitiveInventory, v21Simulation opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		return opentopologysynthesis.SynthesizeV21WithLegacy(
			ctx, requirement,
			v21Inventory, v21Simulation,
			v20Inventory, v20Simulation,
			v18Inventory, v18Simulation,
			legacyInventory, legacySimulation,
			policy,
		)
	}
	return executor
}
