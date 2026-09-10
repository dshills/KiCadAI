package capabilityexecutorv10

import (
	"context"
	"fmt"

	"kicadai/internal/opentopologysynthesis"
)

type electricalSynthesisFuncV23 func(context.Context, opentopologysynthesis.Requirement, opentopologysynthesis.PrimitiveInventory, opentopologysynthesis.SimulationEnvironment, opentopologysynthesis.Policy) opentopologysynthesis.ElectricalSynthesisResultV23

// ExecutorV23 is a public-evaluation adapter, not a v1 CLI admission path.
// Selection belongs to the frozen evaluator; the repair itself has no case IDs.
type ExecutorV23 struct {
	predecessor Executor
	successor   electricalSynthesisFuncV23
	selected    map[string]bool
	cohort      map[string]string
}

func NewSelectedV23WithLegacy(
	cases []CaseInput, v21SelectedIDs, electricalSelectedIDs []string,
	v18Inventory opentopologysynthesis.PrimitiveInventory,
	v18Simulation opentopologysynthesis.SimulationEnvironment,
	legacyInventory opentopologysynthesis.PrimitiveInventory,
	legacySimulation opentopologysynthesis.SimulationEnvironment,
) (ExecutorV23, error) {
	predecessor, err := NewSelectedV21WithLegacy(cases, v21SelectedIDs, v18Inventory, v18Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation)
	if err != nil {
		return ExecutorV23{}, err
	}
	priorSelection := make(map[string]bool, len(v21SelectedIDs))
	for _, id := range v21SelectedIDs {
		priorSelection[id] = true
	}
	for _, id := range electricalSelectedIDs {
		if !priorSelection[id] {
			return ExecutorV23{}, fmt.Errorf("V23 electrical selection is outside the frozen V21 selection")
		}
	}
	successor := func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, simulation opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.ElectricalSynthesisResultV23 {
		return opentopologysynthesis.SynthesizeV23WithLegacy(ctx, requirement, inventory, simulation, v18Inventory, v18Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation, policy, opentopologysynthesis.DefaultElectricalRepairLimitsV22())
	}
	return bindElectricalPopulationV23(predecessor, successor, cases, electricalSelectedIDs)
}

func bindElectricalPopulationV23(predecessor Executor, successor electricalSynthesisFuncV23, cases []CaseInput, selectedIDs []string) (ExecutorV23, error) {
	result := ExecutorV23{predecessor: predecessor, successor: successor, selected: map[string]bool{}, cohort: map[string]string{}}
	if successor == nil || predecessor.synthesize == nil || len(cases) == 0 || len(selectedIDs) == 0 {
		return ExecutorV23{}, fmt.Errorf("V23 evaluator binding is incomplete")
	}
	for _, id := range selectedIDs {
		if id == "" || result.selected[id] {
			return ExecutorV23{}, fmt.Errorf("V23 selected identities must be nonempty and unique")
		}
		result.selected[id] = true
	}
	owners := map[string]bool{}
	for _, input := range cases {
		if err := validateEntry(input); err != nil {
			return ExecutorV23{}, err
		}
		requirement, err := decodeRequirement(input.RequirementSource)
		if err != nil {
			return ExecutorV23{}, err
		}
		hash, err := opentopologysynthesis.CanonicalHash(opentopologysynthesis.Normalize(requirement))
		if err != nil || owners[hash] || result.cohort[input.Entry.ID] != "" {
			return ExecutorV23{}, fmt.Errorf("V23 cohort contains invalid or duplicate identities")
		}
		owners[hash] = true
		binding, err := inputBindingV22(input)
		if err != nil {
			return ExecutorV23{}, err
		}
		result.cohort[input.Entry.ID] = binding
	}
	for id := range result.selected {
		if result.cohort[id] == "" {
			return ExecutorV23{}, fmt.Errorf("V23 selected identity is outside the authenticated public cohort")
		}
	}
	return result, nil
}
