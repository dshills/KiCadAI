package capabilityexecutorv10

import (
	"context"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

// NewV19 binds only the explicit V19 synthesis adapter. No historical
// constructor or runner is mutated.
func NewV19() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.SynthesizeV19,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAware,
	}
}

// NewV19WithLegacy binds the V19 extension, exact V18 environment, and exact
// V17 legacy environment independently. The request environment remains the
// V19 environment used only after the exact V18-first eligibility gate.
func NewV19WithLegacy(
	v18Inventory opentopologysynthesis.PrimitiveInventory,
	v18Simulation opentopologysynthesis.SimulationEnvironment,
	legacyInventory opentopologysynthesis.PrimitiveInventory,
	legacySimulation opentopologysynthesis.SimulationEnvironment,
) Executor {
	executor := NewV19()
	executor.synthesize = func(
		ctx context.Context,
		requirement opentopologysynthesis.Requirement,
		v19Inventory opentopologysynthesis.PrimitiveInventory,
		v19Simulation opentopologysynthesis.SimulationEnvironment,
		policy opentopologysynthesis.Policy,
	) opentopologysynthesis.SynthesisRun {
		return opentopologysynthesis.SynthesizeV19WithLegacy(
			ctx, requirement,
			v19Inventory, v19Simulation,
			v18Inventory, v18Simulation,
			legacyInventory, legacySimulation,
			policy,
		)
	}
	return executor
}
