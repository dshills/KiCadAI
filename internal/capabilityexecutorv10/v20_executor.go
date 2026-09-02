package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

// NewV20 binds only the explicit generic analysis/model/solver admission
// successor. Historical constructors and runners remain unchanged.
func NewV20() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.SynthesizeV20,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAwareV20,
	}
}

func NewV20WithLegacy(
	v18Inventory opentopologysynthesis.PrimitiveInventory,
	v18Simulation opentopologysynthesis.SimulationEnvironment,
	legacyInventory opentopologysynthesis.PrimitiveInventory,
	legacySimulation opentopologysynthesis.SimulationEnvironment,
) Executor {
	executor := NewV20()
	executor.synthesize = func(
		ctx context.Context,
		requirement opentopologysynthesis.Requirement,
		v20Inventory opentopologysynthesis.PrimitiveInventory,
		v20Simulation opentopologysynthesis.SimulationEnvironment,
		policy opentopologysynthesis.Policy,
	) opentopologysynthesis.SynthesisRun {
		return opentopologysynthesis.SynthesizeV20WithLegacy(
			ctx, requirement,
			v20Inventory, v20Simulation,
			v18Inventory, v18Simulation,
			legacyInventory, legacySimulation,
			policy,
		)
	}
	return executor
}
