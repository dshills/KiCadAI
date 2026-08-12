package opentopologysynthesis

import (
	"cmp"
	"slices"
)

const (
	causalRepairChangeCostWeight         = 1_000_000
	causalRepairAddedPrimitiveCostWeight = 1_000
)

// coordinatedTopologyValueCandidates joins one independently authorized
// causal topology change with one independently authorized value change. The
// existing repair search can size a newly inserted primitive and coordinate
// two existing values, but without this bounded bridge it cannot test whether
// an existing bias/value must move together with a causal edge repair.
func coordinatedTopologyValueCandidates(
	base CandidateGraph,
	evaluated []causalEvaluatedCandidate,
	maximum int,
) []causalCandidate {
	if maximum <= 0 {
		return nil
	}
	type costedCandidate struct {
		entry causalEvaluatedCandidate
		cost  int
	}
	eligible := make([]costedCandidate, 0, len(evaluated))
	for _, entry := range evaluated {
		eligible = append(eligible, costedCandidate{entry: entry, cost: causalRepairProposalCost(entry.trial.Repair)})
	}
	slices.SortStableFunc(eligible, func(left, right costedCandidate) int {
		return cmp.Or(
			cmp.Compare(left.cost, right.cost),
			cmp.Compare(right.entry.trial.Improvement, left.entry.trial.Improvement),
			cmp.Compare(left.entry.trial.Hash, right.entry.trial.Hash),
		)
	})
	topology := []causalEvaluatedCandidate{}
	values := []causalEvaluatedCandidate{}
	for _, costed := range eligible {
		entry := costed.entry
		if !entry.trial.Authorized || entry.trial.Improvement <= causalEpsilon || len(entry.trial.Perturbations) != 1 {
			continue
		}
		perturbation := entry.trial.Perturbations[0]
		if causalChangesUseTopology(entry.trial.Repair.Changes) {
			topology = append(topology, entry)
			continue
		}
		if perturbation.Kind == "adjust_value" || perturbation.Kind == "substitute_rated_device" {
			values = append(values, entry)
		}
	}
	if len(topology) > causalMaximumBeamWidth {
		topology = topology[:causalMaximumBeamWidth]
	}
	if len(values) > causalMaximumBeamWidth {
		values = values[:causalMaximumBeamWidth]
	}
	result := []causalCandidate{}
	seen := map[string]struct{}{}
	for _, topologyEntry := range topology {
		for _, valueEntry := range values {
			valuePerturbation := valueEntry.trial.Perturbations[0]
			perturbations := append([]CausalPerturbation(nil), topologyEntry.trial.Perturbations...)
			perturbations = append(perturbations, valuePerturbation)
			if len(perturbations) > causalMaximumChanges {
				continue
			}
			baseIndex := graphInstanceIndex(base, valuePerturbation.InstanceID)
			candidateIndex := graphInstanceIndex(topologyEntry.graph, valuePerturbation.InstanceID)
			if baseIndex < 0 || candidateIndex < 0 ||
				!sameGraphInstanceValue(base.Instances[baseIndex], topologyEntry.graph.Instances[candidateIndex]) {
				continue
			}
			candidate := CloneGraph(topologyEntry.graph)
			if valuePerturbation.ToPrimitiveKey != "" {
				candidate.Instances[candidateIndex].PrimitiveKey = valuePerturbation.ToPrimitiveKey
			}
			candidate.Instances[candidateIndex].ValueSI = cloneInventoryFloat(valuePerturbation.ToValue)
			candidate, err := NormalizeGraph(candidate)
			if err != nil {
				continue
			}
			afterHash, err := GraphHash(candidate)
			if err != nil {
				continue
			}
			if _, duplicate := seen[afterHash]; duplicate {
				continue
			}
			seen[afterHash] = struct{}{}
			valueChange := GraphChange{
				Kind: "set_value", Primitive: valuePerturbation.InstanceID,
				// GraphChange's versioned substitution contract encodes primitive
				// keys in FromNode/ToNode; causalPerturbationsForChanges decodes
				// those same fields back into From/ToPrimitiveKey.
				FromNode: valuePerturbation.FromPrimitiveKey, ToNode: valuePerturbation.ToPrimitiveKey,
				FromValue: cloneInventoryFloat(valuePerturbation.FromValue), ToValue: cloneInventoryFloat(valuePerturbation.ToValue),
			}
			if valuePerturbation.Kind == "substitute_rated_device" {
				valueChange.Kind = "substitute_primitive"
			}
			repair := topologyEntry.trial.Repair
			repair.Operator = "coordinate_topology_and_value"
			repair.AfterGraphHash = afterHash
			repair.Changes = append(append([]GraphChange(nil), repair.Changes...), valueChange)
			result = append(result, causalCandidate{
				graph: candidate, repair: repair, perturbations: perturbations, coordinated: true,
			})
			if len(result) >= maximum {
				return result
			}
		}
	}
	return result
}

func causalRepairProposalCost(repair Repair) int {
	addedPrimitives, addedNodes := 0, 0
	for _, change := range repair.Changes {
		switch change.Kind {
		case "add_primitive", "split_primitive":
			addedPrimitives++
		}
		if change.ToNode != "" && change.FromNode == "" {
			addedNodes++
		}
	}
	return len(repair.Changes)*causalRepairChangeCostWeight +
		addedPrimitives*causalRepairAddedPrimitiveCostWeight +
		addedNodes
}
