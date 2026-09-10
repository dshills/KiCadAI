package capabilityexecutorv10

import (
	"context"
	"encoding/json"
	"fmt"

	"kicadai/internal/opentopologysynthesis"
)

type electricalSynthesisFuncV22 func(context.Context, opentopologysynthesis.Requirement, opentopologysynthesis.PrimitiveInventory, opentopologysynthesis.SimulationEnvironment, opentopologysynthesis.Policy) opentopologysynthesis.ElectricalSynthesisResultV22

// ExecutorV22 is a public-evaluation adapter, not a v1 CLI admission path.
// Selection belongs to the frozen evaluator; the repair itself has no case IDs.
type ExecutorV22 struct {
	predecessor Executor
	successor   electricalSynthesisFuncV22
	selected    map[string]bool
	cohort      map[string]string
}

func NewSelectedV22WithLegacy(
	cases []CaseInput, v21SelectedIDs, electricalSelectedIDs []string,
	v18Inventory opentopologysynthesis.PrimitiveInventory,
	v18Simulation opentopologysynthesis.SimulationEnvironment,
	legacyInventory opentopologysynthesis.PrimitiveInventory,
	legacySimulation opentopologysynthesis.SimulationEnvironment,
) (ExecutorV22, error) {
	predecessor, err := NewSelectedV21WithLegacy(cases, v21SelectedIDs, v18Inventory, v18Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation)
	if err != nil {
		return ExecutorV22{}, err
	}
	priorSelection := make(map[string]bool, len(v21SelectedIDs))
	for _, id := range v21SelectedIDs {
		priorSelection[id] = true
	}
	for _, id := range electricalSelectedIDs {
		if !priorSelection[id] {
			return ExecutorV22{}, fmt.Errorf("V22 electrical selection is outside the frozen V21 selection")
		}
	}
	successor := func(ctx context.Context, requirement opentopologysynthesis.Requirement, inventory opentopologysynthesis.PrimitiveInventory, simulation opentopologysynthesis.SimulationEnvironment, policy opentopologysynthesis.Policy) opentopologysynthesis.ElectricalSynthesisResultV22 {
		return opentopologysynthesis.SynthesizeV22WithLegacy(ctx, requirement, inventory, simulation, v18Inventory, v18Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation, policy, opentopologysynthesis.DefaultElectricalRepairLimitsV22())
	}
	return bindElectricalPopulationV22(predecessor, successor, cases, electricalSelectedIDs)
}

func bindElectricalPopulationV22(predecessor Executor, successor electricalSynthesisFuncV22, cases []CaseInput, selectedIDs []string) (ExecutorV22, error) {
	result := ExecutorV22{predecessor: predecessor, successor: successor, selected: map[string]bool{}, cohort: map[string]string{}}
	if successor == nil || predecessor.synthesize == nil || len(cases) == 0 || len(selectedIDs) == 0 {
		return ExecutorV22{}, fmt.Errorf("V22 evaluator binding is incomplete")
	}
	for _, id := range selectedIDs {
		if id == "" || result.selected[id] {
			return ExecutorV22{}, fmt.Errorf("V22 selected identities must be nonempty and unique")
		}
		result.selected[id] = true
	}
	owners := map[string]bool{}
	for _, input := range cases {
		if err := validateEntry(input); err != nil {
			return ExecutorV22{}, err
		}
		requirement, err := decodeRequirement(input.RequirementSource)
		if err != nil {
			return ExecutorV22{}, err
		}
		hash, err := opentopologysynthesis.CanonicalHash(opentopologysynthesis.Normalize(requirement))
		if err != nil || owners[hash] || result.cohort[input.Entry.ID] != "" {
			return ExecutorV22{}, fmt.Errorf("V22 cohort contains invalid or duplicate identities")
		}
		owners[hash] = true
		binding, err := inputBindingV22(input)
		if err != nil {
			return ExecutorV22{}, err
		}
		result.cohort[input.Entry.ID] = binding
	}
	for id := range result.selected {
		if result.cohort[id] == "" {
			return ExecutorV22{}, fmt.Errorf("V22 selected identity is outside the authenticated public cohort")
		}
	}
	return result, nil
}

func inputBindingV22(input CaseInput) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}
