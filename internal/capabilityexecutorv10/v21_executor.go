package capabilityexecutorv10

import (
	"context"
	"fmt"

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

// NewSelectedV21WithLegacy binds the generic V21 successor only to the
// authenticated public population selected before evaluation. Selection is
// derived from committed case inputs and canonical requirement hashes rather
// than fixture behavior, topology, coordinates, or expected outcomes.
func NewSelectedV21WithLegacy(
	cases []CaseInput,
	selectedCaseIDs []string,
	v20Inventory opentopologysynthesis.PrimitiveInventory,
	v20Simulation opentopologysynthesis.SimulationEnvironment,
	v18Inventory opentopologysynthesis.PrimitiveInventory,
	v18Simulation opentopologysynthesis.SimulationEnvironment,
	legacyInventory opentopologysynthesis.PrimitiveInventory,
	legacySimulation opentopologysynthesis.SimulationEnvironment,
) (Executor, error) {
	v21 := NewV21WithLegacy(v20Inventory, v20Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation)
	v20 := NewV20WithLegacy(v18Inventory, v18Simulation, legacyInventory, legacySimulation)
	return bindSelectedV21(v21, v20, cases, selectedCaseIDs)
}

func bindSelectedV21(v21, v20 Executor, cases []CaseInput, selectedCaseIDs []string) (Executor, error) {
	if v21.synthesize == nil || v20.synthesize == nil || len(cases) == 0 || len(selectedCaseIDs) == 0 {
		return Executor{}, fmt.Errorf("V21 selected evaluator binding is incomplete")
	}
	selectedIDs := make(map[string]bool, len(selectedCaseIDs))
	for _, id := range selectedCaseIDs {
		if id == "" || selectedIDs[id] {
			return Executor{}, fmt.Errorf("V21 selected evaluator contains an empty or duplicate case identity")
		}
		selectedIDs[id] = true
	}
	hashOwners := make(map[string]string, len(cases))
	selectedHashes := make(map[string]bool, len(selectedCaseIDs))
	matched := 0
	for _, input := range cases {
		requirement, err := decodeRequirement(input.RequirementSource)
		if err != nil {
			return Executor{}, fmt.Errorf("decode V21 selected-population requirement: %w", err)
		}
		hash, err := opentopologysynthesis.CanonicalHash(opentopologysynthesis.Normalize(requirement))
		if err != nil {
			return Executor{}, fmt.Errorf("hash V21 selected-population requirement: %w", err)
		}
		if owner, exists := hashOwners[hash]; exists {
			return Executor{}, fmt.Errorf("V21 selected evaluator requirement hash is shared by %q and %q", owner, input.Entry.ID)
		}
		hashOwners[hash] = input.Entry.ID
		if selectedIDs[input.Entry.ID] {
			selectedHashes[hash] = true
			matched++
		}
	}
	if matched != len(selectedIDs) {
		return Executor{}, fmt.Errorf("V21 selected evaluator references a case outside the authenticated public cohort")
	}
	v21Synthesize := v21.synthesize
	v20Synthesize := v20.synthesize
	v21.synthesize = func(
		ctx context.Context,
		requirement opentopologysynthesis.Requirement,
		inventory opentopologysynthesis.PrimitiveInventory,
		environment opentopologysynthesis.SimulationEnvironment,
		policy opentopologysynthesis.Policy,
	) opentopologysynthesis.SynthesisRun {
		// The frozen serial evaluator calls this exactly 48 times. Recomputing
		// the small canonical contract hash avoids mutable identity caches while
		// remaining negligible beside bounded synthesis and simulation.
		hash, err := opentopologysynthesis.CanonicalHash(opentopologysynthesis.Normalize(requirement))
		if err == nil && selectedHashes[hash] {
			return v21Synthesize(ctx, requirement, inventory, environment, policy)
		}
		// Hash failure cannot widen the selected population. Delegate to the
		// exact V20 boundary, which is the fail-closed preservation result.
		return v20Synthesize(ctx, requirement, inventory, environment, policy)
	}
	return v21, nil
}
