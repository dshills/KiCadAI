package opentopologysynthesis

import (
	"context"
	"fmt"
)

// applyControlChangeV22 replays only a catalog-input attachment. It cannot
// smuggle value, component, supply, output-driver, or external-interface edits.
func applyControlChangeV22(before CandidateGraph, inventory PrimitiveInventory, change GraphChange) (CandidateGraph, error) {
	after := CloneGraph(before)
	for i := range after.Instances {
		if after.Instances[i].ID != change.Primitive {
			continue
		}
		for j, terminal := range after.Instances[i].Terminals {
			if terminal.Terminal == change.Terminal && terminal.Node == change.FromNode {
				after.Instances[i].Terminals[j].Node = change.ToNode
				break
			}
		}
		break
	}
	if err := checkControlChangeV22(before, after, inventory, change); err != nil {
		return CandidateGraph{}, err
	}
	return NormalizeGraph(after)
}

func validateControlPathV22(ctx context.Context, requirement Requirement, before, after CandidateGraph, inventory PrimitiveInventory, changes []GraphChange) ([]string, error) {
	if len(changes) == 0 {
		return nil, fmt.Errorf("empty control repair path")
	}
	current := before
	hash, err := GraphHash(before)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{hash: true}
	hashes := make([]string, 0, len(changes))
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current, err = applyControlChangeV22(current, inventory, change)
		if err != nil {
			return nil, err
		}
		structure := analyzeElectricalTopologyV22(requirement, current, inventory, deriveFeedbackBindingsV22(requirement, current, inventory))
		if !structure.Complete || structure.Contradictory || seen[structure.GraphHash] {
			return nil, fmt.Errorf("control repair path is incomplete, contradictory, or repeated")
		}
		seen[structure.GraphHash] = true
		hashes = append(hashes, structure.GraphHash)
	}
	want, err := GraphHash(after)
	if err != nil || hashes[len(hashes)-1] != want {
		return nil, fmt.Errorf("control repair path does not produce final graph")
	}
	return hashes, nil
}
