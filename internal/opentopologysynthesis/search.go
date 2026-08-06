package opentopologysynthesis

import (
	"cmp"
	"container/heap"
	"context"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	maxPrimitivePlacementEnumerations = 4_096
	maxPrimitivePlacementsPerKind     = 12
	maxSearchTransitionsPerState      = 64
	searchRejectionSampleLimit        = 24
	topologyControllerGBWReserve      = 20
)

type topologySearchState struct {
	graph      CandidateGraph
	hash       string
	topology   string
	score      TopologyScore
	operations []GraphOperation
}

type topologyFrontier []topologySearchState

func (frontier topologyFrontier) Len() int { return len(frontier) }
func (frontier topologyFrontier) Less(left, right int) bool {
	return compareTopologyScores(frontier[left].score, frontier[right].score) < 0
}
func (frontier topologyFrontier) Swap(left, right int) {
	frontier[left], frontier[right] = frontier[right], frontier[left]
}
func (frontier *topologyFrontier) Push(value any) {
	*frontier = append(*frontier, value.(topologySearchState))
}
func (frontier *topologyFrontier) Pop() any {
	previous := *frontier
	last := previous[len(previous)-1]
	*frontier = previous[:len(previous)-1]
	return last
}

func SearchPrimitiveTopologies(ctx context.Context, requirement Requirement, inventory PrimitiveInventory, policy Policy) TopologySearchResult {
	result := TopologySearchResult{
		Schema:        TopologySearchSchema,
		Version:       TopologySearchVersion,
		PolicyVersion: PolicyVersion,
		InventoryHash: inventory.Hash,
		Policy:        effectiveTopologyPolicy(policy),
		Status:        TopologySearchFailed,
		Candidates:    []TopologyCandidate{},
		Rejections:    []SearchRejection{},
		Issues:        []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash open-topology requirement: "+err.Error(), "")}
		return result
	}
	result.RequirementHash = requirementHash
	if issues := Validate(requirement); len(issues) != 0 {
		result.Issues = issues
		return result
	}
	if len(inventory.Primitives) == 0 || len(inventory.Hash) != 64 {
		result.Status = TopologySearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePrimitiveUnavailable, "inventory", "topology search requires a nonempty hash-bound primitive inventory", "build the reviewed primitive inventory")}
		return result
	}
	representatives := topologyRepresentatives(requirement, inventory)
	if len(representatives) == 0 {
		result.Status = TopologySearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePrimitiveUnavailable, "inventory.primitives", "no primitive candidates cover the requested analysis envelope", "onboard compatible reviewed primitive evidence")}
		return result
	}
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		result.Issues = issues
		return result
	}
	initialHash, err := GraphHash(initial)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "initial_graph", "hash initial graph: "+err.Error(), "")}
		return result
	}
	initialTopology, err := TopologyHash(initial)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "initial_graph", "hash initial graph topology: "+err.Error(), "")}
		return result
	}
	inventoryByKey := primitiveInventoryByKey(inventory)
	limits := GraphLimits{
		MaxPrimitiveInstances: minPositive(result.Policy.MaxPrimitiveInstances, requirement.Requirements.Constraints.MaxComponents),
		MaxInternalNodes:      result.Policy.MaxInternalNodes,
	}
	initialScore := scoreTopologyGraph(requirement, initial, inventoryByKey, initialHash)
	initialState := topologySearchState{graph: initial, hash: initialHash, topology: initialTopology, score: initialScore}
	frontier := &topologyFrontier{initialState}
	heap.Init(frontier)
	visited := map[string]bool{initialHash: true}
	dominantTopology := map[string]topologySearchState{initialTopology: initialState}
	retainedTopology := map[string]TopologyCandidate{}
	rejections := map[string][]string{}
	relationshipCandidates, relationshipConsumption, relationshipRejections := topologyRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, relationshipConsumption)
	for code, samples := range relationshipRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range relationshipCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	analogCandidates, analogConsumption, analogRejections := topologyAnalogRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, analogConsumption)
	for code, samples := range analogRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range analogCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	fullWaveCandidates, fullWaveConsumption, fullWaveRejections := topologyFullWaveRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, fullWaveConsumption)
	for code, samples := range fullWaveRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range fullWaveCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	transimpedanceCandidates, transimpedanceConsumption, transimpedanceRejections := topologyTransimpedanceRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, transimpedanceConsumption)
	for code, samples := range transimpedanceRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range transimpedanceCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	frequencySelectiveCandidates, frequencySelectiveConsumption, frequencySelectiveRejections := topologyFrequencySelectiveRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, frequencySelectiveConsumption)
	for code, samples := range frequencySelectiveRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range frequencySelectiveCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	bandpassCandidates, bandpassConsumption, bandpassRejections := topologyBandpassRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, bandpassConsumption)
	for code, samples := range bandpassRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range bandpassCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	windowCandidates, windowConsumption, windowRejections := topologyWindowRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, windowConsumption)
	for code, samples := range windowRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range windowCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	windowedSwitchCandidates, windowedSwitchConsumption, windowedSwitchRejections := topologyWindowedControlledSwitchRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, windowedSwitchConsumption)
	for code, samples := range windowedSwitchRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range windowedSwitchCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	regulatorCandidates, regulatorConsumption, regulatorRejections := topologyRegulatedVoltageRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, regulatorConsumption)
	for code, samples := range regulatorRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range regulatorCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	powerTransferCandidates, powerTransferConsumption, powerTransferRejections := topologyPowerTransferRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, powerTransferConsumption)
	for code, samples := range powerTransferRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range powerTransferCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	conditionalCandidates, conditionalConsumption, conditionalRejections := topologyConditionalTransferRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, conditionalConsumption)
	for code, samples := range conditionalRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range conditionalCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	switchCandidates, switchConsumption, switchRejections := topologyControlledSwitchRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, switchConsumption)
	for code, samples := range switchRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range switchCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	nonlinearSwitchingCandidates, nonlinearSwitchingConsumption, nonlinearSwitchingRejections := topologyNonlinearSwitchingRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, nonlinearSwitchingConsumption)
	for code, samples := range nonlinearSwitchingRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range nonlinearSwitchingCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	currentCandidates, currentConsumption, currentRejections := topologyTransconductanceRelationshipSeeds(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		result.Policy,
		initialState,
	)
	addSearchConsumption(&result.Consumption, currentConsumption)
	for code, samples := range currentRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	for _, candidate := range currentCandidates {
		if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retainedTopology[candidate.TopologyHash] = candidate
		}
	}
	obligationPolicy := result.Policy
	obligationPolicy.MaxExpandedStates = max(1, result.Policy.MaxExpandedStates/2)
	obligationPolicy.MaxGeneratedGraphs = max(1, result.Policy.MaxGeneratedGraphs/2)
	minimumDistinctTopologies := min(1, result.Policy.MaxRetainedCandidates)
	if len(retainedTopology) < minimumDistinctTopologies {
		obligationCandidates, obligationConsumption, obligationRejections := topologyObligationSeeds(
			ctx,
			requirement,
			inventory,
			representatives,
			inventoryByKey,
			limits,
			obligationPolicy,
			initialState,
		)
		addSearchConsumption(&result.Consumption, obligationConsumption)
		for code, samples := range obligationRejections {
			rejections[code] = append(rejections[code], samples...)
		}
		for _, candidate := range obligationCandidates {
			if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
				compareTopologyCandidates(candidate, existing) < 0 {
				retainedTopology[candidate.TopologyHash] = candidate
			}
		}
	}
	if len(retainedTopology) < minimumDistinctTopologies {
		guidedPolicy := result.Policy
		guidedPolicy.MaxExpandedStates = min(
			max(0, result.Policy.MaxExpandedStates-result.Consumption.ExpandedStates),
			max(1, result.Policy.MaxExpandedStates/4),
		)
		guidedPolicy.MaxGeneratedGraphs = min(
			max(0, result.Policy.MaxGeneratedGraphs-result.Consumption.GeneratedGraphs),
			max(1, result.Policy.MaxGeneratedGraphs/4),
		)
		guidedCandidates, guidedConsumption, guidedRejections := behaviorGuidedTopologySeeds(
			ctx,
			requirement,
			inventory,
			representatives,
			inventoryByKey,
			limits,
			guidedPolicy,
			initialState,
		)
		addSearchConsumption(&result.Consumption, guidedConsumption)
		for code, samples := range guidedRejections {
			rejections[code] = append(rejections[code], samples...)
		}
		for _, candidate := range guidedCandidates {
			if existing, found := retainedTopology[candidate.TopologyHash]; !found ||
				compareTopologyCandidates(candidate, existing) < 0 {
				retainedTopology[candidate.TopologyHash] = candidate
			}
		}
	}
	// Relationship construction retains materially-distinct alternatives when
	// a bounded electrical relationship admits them. Once any complete
	// canonical topology exists, the unconstrained fallback mostly repeats
	// weaker placements at much higher cost. Retain it only while the
	// relationship, obligation, and guided lanes have no complete result.
	if len(retainedTopology) >= minimumDistinctTopologies {
		frontier = &topologyFrontier{}
	}

	for frontier.Len() != 0 {
		if err := ctx.Err(); err != nil {
			result.Status = TopologySearchCanceled
			result.Issues = []reports.Issue{graphIssue(CodeCanceled, "search", "open-topology search canceled", "retry with an active context")}
			break
		}
		if result.Consumption.ExpandedStates >= result.Policy.MaxExpandedStates ||
			result.Consumption.GeneratedGraphs >= result.Policy.MaxGeneratedGraphs {
			result.Status = TopologySearchExhausted
			result.Consumption.BudgetExhausted = true
			result.Issues = []reports.Issue{graphIssue(CodeSearchExhausted, "search.policy", "open-topology graph budget exhausted", "increase the explicit count budget or narrow the behavioral envelope")}
			break
		}
		state := heap.Pop(frontier).(topologySearchState)
		if dominant, found := dominantTopology[state.topology]; found && dominant.hash != state.hash {
			rejections["dominated_topology"] = append(
				rejections["dominated_topology"],
				state.hash+"->"+dominant.hash,
			)
			continue
		}
		result.Consumption.ExpandedStates++
		if frontier.Len() > result.Consumption.MaximumFrontier {
			result.Consumption.MaximumFrontier = frontier.Len()
		}

		completeIssues := ValidateCompleteGraph(state.graph, inventory, limits)
		if len(completeIssues) == 0 && len(state.graph.Instances) != 0 && state.score.BehaviorGap == 0 {
			topologyHash, hashErr := TopologyHash(state.graph)
			if hashErr != nil {
				rejections["canonical_hash_failed"] = append(rejections["canonical_hash_failed"], state.hash)
			} else {
				normalized, normalizeErr := NormalizeGraph(state.graph)
				if normalizeErr != nil {
					rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], state.hash)
				} else {
					result.Consumption.CompleteGraphs++
					candidate := TopologyCandidate{
						Fingerprint:  state.hash,
						TopologyHash: topologyHash,
						Score:        state.score,
						Graph:        normalized,
						Operations:   cloneGraphOperations(state.operations),
					}
					if existing, found := retainedTopology[topologyHash]; !found ||
						compareTopologyCandidates(candidate, existing) < 0 {
						retainedTopology[topologyHash] = candidate
					}
				}
			}
		} else if len(completeIssues) != 0 {
			for _, issue := range completeIssues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
		}

		if len(state.graph.Instances) >= limits.MaxPrimitiveInstances {
			continue
		}
		remainingGraphs := result.Policy.MaxGeneratedGraphs - result.Consumption.GeneratedGraphs
		expansions, generatedGraphs := expandTopologyState(
			state, representatives, limits, requirement, inventory, inventoryByKey,
			remainingGraphs,
		)
		result.Consumption.GeneratedGraphs += generatedGraphs
		for _, expansion := range expansions {
			hash, hashErr := GraphHash(expansion.graph)
			if hashErr != nil {
				rejections["canonical_hash_failed"] = append(rejections["canonical_hash_failed"], hashErr.Error())
				continue
			}
			if visited[hash] {
				rejections["dominated_topology"] = append(rejections["dominated_topology"], hash+"->"+hash)
				continue
			}
			visited[hash] = true
			if partialIssues := ValidatePartialGraph(expansion.graph, inventory, limits); len(partialIssues) != 0 {
				for _, issue := range partialIssues {
					rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
				}
				continue
			}
			expansion.hash = hash
			expansion.score = scoreTopologyGraph(requirement, expansion.graph, inventoryByKey, hash)
			topologyHash, topologyErr := TopologyHash(expansion.graph)
			if topologyErr != nil {
				rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], topologyErr.Error())
				continue
			}
			expansion.topology = topologyHash
			if dominant, found := dominantTopology[topologyHash]; found &&
				compareTopologyScores(dominant.score, expansion.score) <= 0 {
				rejections["dominated_topology"] = append(
					rejections["dominated_topology"],
					expansion.hash+"->"+dominant.hash,
				)
				continue
			}
			dominantTopology[topologyHash] = expansion
			if len(expansion.operations) != 0 {
				last := len(expansion.operations) - 1
				expansion.operations[last].AfterHash = hash
			}
			heap.Push(frontier, expansion)
			if frontier.Len() > result.Consumption.MaximumFrontier {
				result.Consumption.MaximumFrontier = frontier.Len()
			}
		}

		if len(retainedTopology) >= result.Policy.MaxRetainedCandidates &&
			result.Consumption.ExpandedStates >= minPositive(result.Policy.MaxRetainedCandidates*8, result.Policy.MaxExpandedStates) {
			break
		}
	}

	for topologyHash, candidate := range retainedTopology {
		activeHash, err := ActiveStructureHash(candidate.Graph)
		if err != nil {
			rejections["active_structure_hash_failed"] = append(
				rejections["active_structure_hash_failed"], topologyHash+":"+err.Error(),
			)
			delete(retainedTopology, topologyHash)
			continue
		}
		candidate.ActiveStructureHash = activeHash
		retainedTopology[topologyHash] = candidate
	}
	result.Candidates = make([]TopologyCandidate, 0, len(retainedTopology))
	for _, candidate := range retainedTopology {
		result.Candidates = append(result.Candidates, candidate)
	}
	slices.SortFunc(result.Candidates, compareTopologyCandidates)
	if len(result.Candidates) > result.Policy.MaxRetainedCandidates {
		result.Candidates = selectDiverseTopologyCandidates(
			result.Candidates,
			result.Policy.MaxRetainedCandidates,
		)
	}
	result.Rejections = normalizeSearchRejections(rejections)
	if len(result.Candidates) != 0 {
		result.Status = TopologySearchCandidates
		result.Issues = nil
	} else if result.Status != TopologySearchCanceled &&
		(result.Consumption.GeneratedGraphs >= result.Policy.MaxGeneratedGraphs ||
			result.Consumption.ExpandedStates >= result.Policy.MaxExpandedStates) {
		result.Status = TopologySearchExhausted
		result.Consumption.BudgetExhausted = true
		result.Issues = []reports.Issue{graphIssue(CodeSearchExhausted, "search.policy", "open-topology graph budget exhausted", "increase the explicit count budget or narrow the behavioral envelope")}
	} else if result.Status != TopologySearchCanceled && result.Status != TopologySearchExhausted {
		result.Status = TopologySearchExhausted
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "search", "bounded search produced no complete primitive graph", "increase the explicit graph budget or onboard a compatible primitive")}
	}
	return result
}

// topologyFrequencySelectiveRelationshipSeeds recognizes a bounded rejection
// frequency bracketed by preserved lower and upper passbands. It emits
// structurally distinct buffered bridge networks whose ratios can be derived
// from the requested frequency and adjacent passband bounds. The enhanced
// alternative raises the bridge reference through bounded positive feedback
// to narrow the rejection band. No circuit or project identity participates
// in the trigger.
func topologyFrequencySelectiveRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	if _, ok := topologyRejectionFrequency(requirement); !ok {
		return nil, Consumption{}, map[string][]string{}
	}
	var resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	controller := topologyPowerControllerPrimitive(requirement, inventory)
	if resistor.Key == "" || capacitor.Key == "" || controller.Key == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	inputs := topologyNodesByRole(initial.graph, "input", "control")
	outputs := topologyNodesByRole(initial.graph, "output")
	references := topologyNodesByRole(initial.graph, "reference")
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	if len(inputs) == 0 || len(outputs) == 0 || len(references) == 0 || highRail == "" || lowRail == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	addNode := func(state topologySearchState) (topologySearchState, string, bool) {
		if ctx.Err() != nil || consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			return state, "", false
		}
		next, node := addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
		return next, node, node != ""
	}
	add := func(state topologySearchState, primitive PrimitiveCandidate, terminals []TerminalConnection) topologySearchState {
		if ctx.Err() != nil || consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			return state
		}
		return addRelationshipPrimitive(state, requirement, inventoryByKey, primitive, terminals, &consumption)
	}
	retain := func(state topologySearchState) {
		if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
			internalNodeCount(state.graph) > limits.MaxInternalNodes {
			rejections["graph_limit"] = append(rejections["graph_limit"], fmt.Sprintf("instances=%d internal_nodes=%d", len(state.graph.Instances), internalNodeCount(state.graph)))
			return
		}
		if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			return
		}
		if state.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap))
			return
		}
		normalized, err := NormalizeGraph(state.graph)
		if err != nil {
			rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], err.Error())
			return
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], err.Error())
			return
		}
		consumption.CompleteGraphs++
		candidate := TopologyCandidate{
			Fingerprint: state.hash, TopologyHash: topologyHash,
			Score: state.score, Graph: normalized,
			Operations: cloneGraphOperations(state.operations),
		}
		if existing, found := retained[topologyHash]; !found || compareTopologyCandidates(candidate, existing) < 0 {
			retained[topologyHash] = candidate
		}
	}
	for _, input := range inputs {
		for _, output := range outputs {
			reference := references[0]
			for _, enhancedQ := range []bool{false, true} {
				if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates {
					consumption.BudgetExhausted = true
					break
				}
				consumption.ExpandedStates++
				state := initial
				source := input
				nodes := make([]string, 0, 3)
				for len(nodes) < 3 {
					var node string
					var ok bool
					state, node, ok = addNode(state)
					if !ok {
						break
					}
					nodes = append(nodes, node)
				}
				if len(nodes) != 3 {
					continue
				}
				resistiveMid, capacitiveMid, filtered := nodes[0], nodes[1], nodes[2]
				bridgeReference := reference
				outputFeedback := output
				divider, drivenReference := "", ""
				if enhancedQ {
					var ok bool
					for _, target := range []*string{&divider, &drivenReference, &outputFeedback} {
						state, *target, ok = addNode(state)
						if !ok {
							break
						}
					}
					if !ok {
						continue
					}
				}
				feedbackConnection := TerminalConnection{Terminal: "IN_MINUS", Node: outputFeedback}
				var added bool
				state, added = addRelationshipActiveStage(
					state, requirement, inventoryByKey,
					relationshipActiveStage{
						Primitive: controller,
						Input:     TerminalConnection{Terminal: "IN_PLUS", Node: filtered},
						Output:    TerminalConnection{Terminal: "OUT", Node: output},
						Feedback:  &feedbackConnection,
						Bias: []TerminalConnection{
							{Terminal: "V_MINUS", Node: lowRail},
							{Terminal: "V_PLUS", Node: highRail},
						},
					},
					&consumption,
				)
				if !added {
					continue
				}
				if enhancedQ {
					state = add(state, resistor, topologyTwoTerminalPlacement(output, outputFeedback))
					state = add(state, resistor, topologyTwoTerminalPlacement(outputFeedback, reference))
					state = add(state, resistor, topologyTwoTerminalPlacement(output, divider))
					state = add(state, resistor, topologyTwoTerminalPlacement(divider, reference))
					bufferFeedback := TerminalConnection{Terminal: "IN_MINUS", Node: drivenReference}
					state, added = addRelationshipActiveStage(
						state, requirement, inventoryByKey,
						relationshipActiveStage{
							Primitive: controller,
							Input:     TerminalConnection{Terminal: "IN_PLUS", Node: divider},
							Output:    TerminalConnection{Terminal: "OUT", Node: drivenReference},
							Feedback:  &bufferFeedback,
							Bias: []TerminalConnection{
								{Terminal: "V_MINUS", Node: lowRail},
								{Terminal: "V_PLUS", Node: highRail},
							},
						},
						&consumption,
					)
					if !added {
						continue
					}
					bridgeReference = drivenReference
				}
				if enhancedQ {
					armInput, armOutput := "", ""
					var ok bool
					state, armInput, ok = addNode(state)
					if !ok {
						continue
					}
					state, armOutput, ok = addNode(state)
					if !ok {
						continue
					}
					for _, edge := range [][2]string{
						{source, armInput}, {armInput, resistiveMid},
						{resistiveMid, armOutput}, {armOutput, filtered},
						{capacitiveMid, bridgeReference},
					} {
						state = add(state, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]))
					}
				} else {
					for _, edge := range [][2]string{
						{source, resistiveMid}, {resistiveMid, filtered},
						{capacitiveMid, bridgeReference}, {capacitiveMid, bridgeReference},
					} {
						state = add(state, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]))
					}
				}
				for _, edge := range [][2]string{
					{source, capacitiveMid}, {capacitiveMid, filtered},
					{resistiveMid, bridgeReference}, {resistiveMid, bridgeReference},
				} {
					state = add(state, capacitor, topologyTwoTerminalPlacement(edge[0], edge[1]))
				}
				retain(state)
			}
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

type bandpassBehaviorEnvelope struct {
	input, output            Observation
	lowerHz, passHz, upperHz float64
	passMinimum, passMaximum float64
	rejectionMaximum         float64
}

func topologyBandpassBehaviorEnvelope(requirement Requirement) (bandpassBehaviorEnvelope, bool) {
	type response struct {
		assertion  BehavioralAssertion
		excitation Observation
		frequency  float64
		minimum    float64
		maximum    float64
		hasMin     bool
		hasMax     bool
	}
	resolveExcitation := func(assertion BehavioralAssertion) (Observation, bool) {
		// An explicit assertion binding always wins, including requirements
		// with multiple analog inputs. Inference is only for an omitted binding.
		if assertion.Excitation != nil {
			return *assertion.Excitation, true
		}
		inputs := []Observation{}
		for _, port := range requirement.Requirements.Ports {
			if port.Kind == "analog_voltage" && port.Direction == "sink" {
				inputs = append(inputs, Observation{Kind: "port", ID: port.ID})
			}
		}
		if len(inputs) != 1 {
			return Observation{}, false
		}
		return inputs[0], true
	}
	responses := []response{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if !slices.Contains([]string{"voltage_gain", "voltage_gain_at_frequency"}, assertion.Metric) ||
			assertion.FrequencyHz == nil || *assertion.FrequencyHz <= 0 {
			continue
		}
		excitation, found := resolveExcitation(assertion)
		if !found {
			continue
		}
		item := response{assertion: assertion, excitation: excitation, frequency: *assertion.FrequencyHz}
		if assertion.Min != nil {
			item.minimum, item.hasMin = *assertion.Min, true
		}
		if assertion.Max != nil {
			item.maximum, item.hasMax = *assertion.Max, true
		}
		responses = append(responses, item)
	}
	slices.SortFunc(responses, func(left, right response) int {
		return cmp.Or(cmp.Compare(left.frequency, right.frequency), cmp.Compare(left.assertion.ID, right.assertion.ID))
	})
	for _, pass := range responses {
		if !pass.hasMin || pass.minimum <= 0 {
			continue
		}
		var lower, upper *response
		for index := range responses {
			candidate := &responses[index]
			if !candidate.hasMax || candidate.maximum <= 0 || candidate.maximum >= pass.minimum {
				continue
			}
			if candidate.excitation != pass.excitation || candidate.assertion.Observation != pass.assertion.Observation {
				continue
			}
			if candidate.frequency < pass.frequency && (lower == nil || candidate.frequency > lower.frequency) {
				lower = candidate
			}
			if candidate.frequency > pass.frequency && (upper == nil || candidate.frequency < upper.frequency) {
				upper = candidate
			}
		}
		if lower != nil && upper != nil {
			passMaximum := math.Inf(1)
			if pass.hasMax {
				passMaximum = pass.maximum
			}
			return bandpassBehaviorEnvelope{
				input: pass.excitation, output: pass.assertion.Observation,
				lowerHz: lower.frequency, passHz: pass.frequency, upperHz: upper.frequency,
				passMinimum: pass.minimum, passMaximum: passMaximum,
				rejectionMaximum: math.Min(lower.maximum, upper.maximum),
			}, true
		}
	}
	return bandpassBehaviorEnvelope{}, false
}

// topologyBandpassRelationshipSeeds emits the dual of the existing rejected-
// midband bridge: a passive lower-corner/high-corner cascade with catalog
// active isolation between behavior-derived stages. The trigger and terminal
// relationships come only from a preserved gain point bracketed by two
// stricter rejection points.
func topologyBandpassRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	envelope, ok := topologyBandpassBehaviorEnvelope(requirement)
	if !ok || ctx.Err() != nil {
		return nil, Consumption{}, map[string][]string{}
	}
	byKind := map[string]PrimitiveCandidate{}
	for _, primitive := range representatives {
		if _, found := byKind[primitive.Kind]; !found {
			byKind[primitive.Kind] = primitive
		}
	}
	opamp, resistor, capacitor := byKind["opamp"], byKind["resistor"], byKind["capacitor"]
	if opamp.Key == "" || resistor.Key == "" || capacitor.Key == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"bracketed passband requires catalog-backed active, resistive, and reactive relationships"}}
	}
	input := observationNodeID(initial.graph, requirement, envelope.input)
	output := observationNodeID(initial.graph, requirement, envelope.output)
	reference := referenceNodeForDomain(initial.graph, envelope.input)
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	if input == "" || output == "" || reference == "" || highRail == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"bracketed passband interfaces do not resolve to bounded graph endpoints"}}
	}
	if lowRail == "" {
		lowRail = reference
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	retain := func(state topologySearchState) {
		if len(state.graph.Instances) > limits.MaxPrimitiveInstances || internalNodeCount(state.graph) > limits.MaxInternalNodes {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], "bracketed passband graph budget is exhausted")
			return
		}
		if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			return
		}
		if state.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf(
				"%s:gap=%d:analog=%d:hf=%t",
				state.hash,
				state.score.BehaviorGap,
				topologyGraphAnalogStageGap(state.graph, true, false, false),
				topologyGraphHasHighFrequencyAttenuationStage(state.graph),
			))
			return
		}
		normalized, err := NormalizeGraph(state.graph)
		if err != nil {
			rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], err.Error())
			return
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], err.Error())
			return
		}
		consumption.CompleteGraphs++
		retained[topologyHash] = TopologyCandidate{
			Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score,
			Graph: normalized, Operations: cloneGraphOperations(state.operations),
		}
	}

	for _, architecture := range []struct {
		stages       []string
		recoveryGain bool
	}{
		{stages: []string{"lower", "lower", "upper"}, recoveryGain: true},
		{stages: []string{"lower", "lower", "upper", "upper"}},
	} {
		if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates {
			break
		}
		consumption.ExpandedStates++
		state := initial
		previous := input
		complete := true
		for stageIndex, stageKind := range architecture.stages {
			var filterNode string
			state, filterNode = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
			bufferNode := output
			if stageIndex != len(architecture.stages)-1 {
				state, bufferNode = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
			}
			if filterNode == "" || bufferNode == "" {
				complete = false
				break
			}
			if stageKind == "lower" {
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, capacitor, topologyTwoTerminalPlacement(previous, filterNode), &consumption)
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, topologyTwoTerminalPlacement(filterNode, reference), &consumption)
			} else {
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, topologyTwoTerminalPlacement(previous, filterNode), &consumption)
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, capacitor, topologyTwoTerminalPlacement(filterNode, reference), &consumption)
			}
			feedbackNode := bufferNode
			if architecture.recoveryGain && stageIndex == len(architecture.stages)-1 {
				state, feedbackNode = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
				if feedbackNode == "" {
					complete = false
					break
				}
			}
			feedback := TerminalConnection{Terminal: "IN_MINUS", Node: feedbackNode}
			var added bool
			state, added = addRelationshipActiveStage(
				state, requirement, inventoryByKey,
				relationshipActiveStage{
					Primitive: opamp,
					Input:     TerminalConnection{Terminal: "IN_PLUS", Node: filterNode},
					Output:    TerminalConnection{Terminal: "OUT", Node: bufferNode},
					Feedback:  &feedback,
					Bias: []TerminalConnection{
						{Terminal: "V_MINUS", Node: lowRail},
						{Terminal: "V_PLUS", Node: highRail},
					},
				},
				&consumption,
			)
			if !added {
				complete = false
				break
			}
			if feedbackNode != bufferNode {
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, topologyTwoTerminalPlacement(bufferNode, feedbackNode), &consumption)
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, topologyTwoTerminalPlacement(feedbackNode, reference), &consumption)
			}
			previous = bufferNode
		}
		if complete && previous == output {
			retain(state)
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

type windowBehaviorEnvelope struct {
	input  string
	output string
	lowerV float64
	upperV float64
}

func topologyWindowThresholdEnvelope(requirement Requirement) (windowBehaviorEnvelope, bool) {
	envelope := windowBehaviorEnvelope{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Excitation == nil || assertion.Excitation.Kind != "port" ||
			assertion.Observation.Kind != "port" {
			continue
		}
		switch assertion.Metric {
		case "lower_threshold":
			envelope.input = assertion.Excitation.ID
			envelope.output = assertion.Observation.ID
			envelope.lowerV = assertionTarget(assertion)
		case "upper_threshold":
			if envelope.input != "" && (envelope.input != assertion.Excitation.ID ||
				envelope.output != assertion.Observation.ID) {
				return windowBehaviorEnvelope{}, false
			}
			envelope.input = assertion.Excitation.ID
			envelope.output = assertion.Observation.ID
			envelope.upperV = assertionTarget(assertion)
		}
	}
	if envelope.input == "" || envelope.output == "" || envelope.lowerV <= 0 ||
		envelope.upperV <= envelope.lowerV {
		return windowBehaviorEnvelope{}, false
	}
	return envelope, true
}

func topologyWindowBehaviorEnvelope(requirement Requirement) (windowBehaviorEnvelope, bool) {
	envelope, found := topologyWindowThresholdEnvelope(requirement)
	if !found {
		return windowBehaviorEnvelope{}, false
	}
	caseByID := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		caseByID[operatingCase.ID] = operatingCase
	}
	highBelow, highAbove, lowInside := false, false, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind != "port" || assertion.Observation.ID != envelope.output ||
			(assertion.Metric != "output_high_voltage" && assertion.Metric != "output_low_voltage") {
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			operatingCase, found := caseByID[caseID]
			if !found {
				continue
			}
			for _, condition := range operatingCase.Conditions {
				if condition.Axis != "input_voltage" || condition.Target != envelope.input {
					continue
				}
				switch assertion.Metric {
				case "output_high_voltage":
					highBelow = highBelow || condition.Max < envelope.lowerV
					highAbove = highAbove || condition.Min > envelope.upperV
				case "output_low_voltage":
					lowInside = lowInside ||
						(condition.Min > envelope.lowerV && condition.Max < envelope.upperV)
				}
			}
		}
	}
	return envelope, highBelow && highAbove && lowInside
}

// topologyWindowRelationshipSeeds constructs an outside-window assertion from
// the declared pair of thresholds. Two open-collector decisions share a
// pulled-up inside-window node; a third decision converts that node to the
// requested outside-high polarity. A reviewed shunt reference and two generic
// non-inverting gain relationships keep both thresholds independent of line
// variation. Values are derived later from graph relationships and bounds.
func topologyWindowRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	envelope, required := topologyWindowBehaviorEnvelope(requirement)
	if !required {
		return nil, Consumption{}, map[string][]string{}
	}
	var comparator, opamp, resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "comparator":
			comparator = primitive
		case "opamp":
			opamp = primitive
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	stableReference := topologyStableReferencePrimitive(requirement, inventory)
	referenceVoltage, referenceVoltageKnown := topologyPrimitiveReferenceVoltage(stableReference)
	if comparator.Key == "" || opamp.Key == "" || resistor.Key == "" || capacitor.Key == "" || stableReference.Key == "" {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"outside-window transfer requires catalog-backed comparator, amplifier, reference, and resistor relationships"},
		}
	}
	input, output := "port_"+envelope.input, "port_"+envelope.output
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(supplies) != 1 || len(references) != 1 {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"outside-window transfer requires one bounded supply and reference relationship"},
		}
	}
	supplyNode, referenceNode := supplies[0], references[0]
	consumption := Consumption{ExpandedStates: 1}
	rejections := map[string][]string{}
	state := initial
	internal := make([]string, 0, 8)
	for range 8 {
		if ctx.Err() != nil || consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			break
		}
		var node string
		state, node = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
		if node == "" {
			break
		}
		internal = append(internal, node)
	}
	if len(internal) != 8 || internalNodeCount(state.graph) > limits.MaxInternalNodes {
		return nil, consumption, rejections
	}
	absoluteReference := internal[0]
	lowerReference, lowerGain, lowerJoin := internal[1], internal[2], internal[3]
	upperReference, upperGain, upperJoin := internal[4], internal[5], internal[6]
	insideNode := internal[7]
	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, stableReference,
		[]TerminalConnection{
			{Terminal: "ANODE", Node: referenceNode},
			{Terminal: "CATHODE", Node: absoluteReference},
		}, &consumption,
	)
	lowerAmplifier := []TerminalConnection{
		{Terminal: "IN_PLUS", Node: absoluteReference},
		{Terminal: "IN_MINUS", Node: lowerGain},
		{Terminal: "OUT", Node: lowerReference},
		{Terminal: "V_MINUS", Node: referenceNode},
		{Terminal: "V_PLUS", Node: supplyNode},
	}
	lowerEdges := [][2]string{
		{lowerGain, referenceNode},
		{lowerReference, lowerJoin}, {lowerJoin, lowerGain},
	}
	if referenceVoltageKnown && envelope.lowerV < referenceVoltage {
		// A threshold below the reviewed reference cannot be realized by a
		// non-inverting gain stage. Divide the absolute reference first, then
		// buffer the high-impedance tap so threshold accuracy remains isolated
		// from the comparator input and pull-up network.
		lowerAmplifier = []TerminalConnection{
			{Terminal: "IN_PLUS", Node: lowerGain},
			{Terminal: "IN_MINUS", Node: lowerJoin},
			{Terminal: "OUT", Node: lowerReference},
			{Terminal: "V_MINUS", Node: referenceNode},
			{Terminal: "V_PLUS", Node: supplyNode},
		}
		lowerEdges = [][2]string{
			{absoluteReference, lowerGain}, {lowerGain, referenceNode},
			{lowerReference, lowerJoin},
		}
	}
	for _, placement := range [][]TerminalConnection{
		lowerAmplifier,
		{
			{Terminal: "IN_PLUS", Node: absoluteReference},
			{Terminal: "IN_MINUS", Node: upperGain},
			{Terminal: "OUT", Node: upperReference},
			{Terminal: "V_MINUS", Node: referenceNode},
			{Terminal: "V_PLUS", Node: supplyNode},
		},
	} {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, opamp, placement, &consumption)
	}
	for _, placement := range [][]TerminalConnection{
		{
			{Terminal: "IN_PLUS", Node: input},
			{Terminal: "IN_MINUS", Node: lowerReference},
			{Terminal: "OUT", Node: insideNode},
			{Terminal: "V_MINUS", Node: referenceNode},
			{Terminal: "V_PLUS", Node: supplyNode},
		},
		{
			{Terminal: "IN_PLUS", Node: upperReference},
			{Terminal: "IN_MINUS", Node: input},
			{Terminal: "OUT", Node: insideNode},
			{Terminal: "V_MINUS", Node: referenceNode},
			{Terminal: "V_PLUS", Node: supplyNode},
		},
		{
			{Terminal: "IN_PLUS", Node: absoluteReference},
			{Terminal: "IN_MINUS", Node: insideNode},
			{Terminal: "OUT", Node: output},
			{Terminal: "V_MINUS", Node: referenceNode},
			{Terminal: "V_PLUS", Node: supplyNode},
		},
	} {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, comparator, placement, &consumption)
	}
	edges := [][2]string{
		{supplyNode, absoluteReference},
		{upperGain, referenceNode},
		{upperReference, upperJoin}, {upperJoin, upperGain},
		{supplyNode, insideNode}, {supplyNode, output},
	}
	edges = append(edges, lowerEdges...)
	for _, edge := range edges {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, resistor,
			topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption,
		)
	}
	states := []topologySearchState{state}
	bypassed := addRelationshipPrimitive(
		state,
		requirement,
		inventoryByKey,
		capacitor,
		topologyTwoTerminalPlacement(supplyNode, referenceNode),
		&consumption,
	)
	if bypassed.hash != state.hash {
		states = append(states, bypassed)
	}
	result := make([]TopologyCandidate, 0, len(states))
	for _, candidateState := range states {
		if len(candidateState.graph.Instances) > limits.MaxPrimitiveInstances {
			rejections["graph_limit"] = append(rejections["graph_limit"], candidateState.hash+":primitive instance limit exceeded")
			continue
		}
		if issues := ValidateCompleteGraph(candidateState.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			continue
		}
		if candidateState.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", candidateState.hash, candidateState.score.BehaviorGap))
			continue
		}
		normalized, err := NormalizeGraph(candidateState.graph)
		if err != nil {
			rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], err.Error())
			continue
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], err.Error())
			continue
		}
		consumption.CompleteGraphs++
		result = append(result, TopologyCandidate{
			Fingerprint: candidateState.hash, TopologyHash: topologyHash, Score: candidateState.score,
			Graph: normalized, Operations: cloneGraphOperations(candidateState.operations),
		})
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

// topologyWindowedControlledSwitchRelationshipSeeds composes two ordered
// threshold decisions with a protected high-side power path. The shared
// open-collector node is high only while both threshold predicates are true;
// independent passive feedback branches preserve state near either boundary.
// Selection is driven only by lower/upper-threshold and controlled-load
// obligations, so the relationship applies to any bounded analog control
// input and source-oriented switched output.
func topologyWindowedControlledSwitchRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	envelope, hasWindow := topologyWindowThresholdEnvelope(requirement)
	if !hasWindow || !topologyControlledSwitchRequired(requirement) {
		return nil, Consumption{}, map[string][]string{}
	}
	input, output := "port_"+envelope.input, "port_"+envelope.output
	highSide := topologyControlledSwitchHighSideOutput(requirement, initial.graph, output)
	for _, port := range requirement.Requirements.Ports {
		if port.ID == envelope.output && port.Kind == "controlled_current" && port.Direction == "source" {
			highSide = true
			break
		}
	}
	var comparator, opamp, nmos, pmos, resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "comparator":
			comparator = primitive
		case "opamp":
			opamp = primitive
		case "p_channel_mosfet":
			pmos = primitive
		case "n_channel_mosfet":
			nmos = primitive
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	mosfet := nmos
	mosfetKind := "n_channel_mosfet"
	if highSide {
		mosfet, mosfetKind = pmos, "p_channel_mosfet"
	}
	if rated := topologyRatedPowerPrimitive(requirement, inventory, mosfetKind); rated.Key != "" {
		mosfet = rated
	}
	stableReference := topologyStableReferencePrimitive(requirement, inventory)
	referenceVoltage, referenceKnown := topologyPrimitiveReferenceVoltage(stableReference)
	levelShifter := selectCurrentRelationshipPrimitiveMatching(
		requirement, inventory, "npn_bjt", false, false,
		func(candidate PrimitiveCandidate) bool {
			return primitiveModelParameter(candidate, simmodel.PrimitiveBJTNPNV1, "forward_beta") >= 100
		},
	)
	flyback := topologyFlybackDiodePrimitive(requirement, inventory)
	if comparator.Key == "" || opamp.Key == "" || mosfet.Key == "" ||
		resistor.Key == "" || capacitor.Key == "" || stableReference.Key == "" ||
		(highSide && levelShifter.Key == "") || flyback.Key == "" || !referenceKnown {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"windowed controlled power requires reviewed comparator, amplifier, reference, level-shifter, switch, passive, and protection relationships"},
		}
	}
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(supplies) < 2 || len(references) != 1 {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"windowed controlled power requires distinct bounded control and load supplies with one reference"},
		}
	}
	loadSupply, loadSupplyFound := topologyControlledSwitchLoadSupply(
		requirement, initial.graph, output, supplies,
	)
	if !loadSupplyFound {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {output + ": windowed controlled power lacks one unambiguous load supply"},
		}
	}
	regulatedLoad, hasRegulatedLoad := topologyRegulatedLoadRail(
		requirement, initial.graph, output, loadSupply, inventory,
	)
	maximumGateSpan := 0.0
	if _, maximum, found := topologyDeclaredNodeVoltageRange(
		requirement, initial.graph, loadSupply,
	); found {
		maximumGateSpan = maximum
	}
	if hasRegulatedLoad && regulatedLoad.outputVoltageV > 0 {
		maximumGateSpan = regulatedLoad.outputVoltageV
	}
	gateClamp, gateClampRequired := topologyMOSFETGateClampPrimitive(
		inventory, mosfet, maximumGateSpan,
	)
	if highSide && gateClampRequired && gateClamp.Key == "" {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"windowed high-side power switch lacks a reviewed gate clamp with sufficient turn-on and rating margin"},
		}
	}
	driveSupply := ""
	driveVoltage := math.Inf(1)
	for _, supply := range supplies {
		if supply == loadSupply {
			continue
		}
		nominal, known := topologyNodeNominalVoltage(requirement, initial.graph, supply)
		if !known || nominal <= envelope.upperV || nominal <= referenceVoltage {
			continue
		}
		if nominal < driveVoltage || (nominal == driveVoltage && (driveSupply == "" || supply < driveSupply)) {
			driveSupply, driveVoltage = supply, nominal
		}
	}
	if driveSupply == "" {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"windowed controlled power lacks a control rail above both requested thresholds"},
		}
	}

	consumption := Consumption{ExpandedStates: 1}
	rejections := map[string][]string{}
	state := initial
	nextNode := func() string {
		if ctx.Err() != nil || consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			return ""
		}
		var node string
		state, node = addRelationshipInternalNode(
			state, requirement, inventoryByKey, &consumption,
		)
		return node
	}
	absoluteReference := nextNode()
	lowerReference, lowerSignal := nextNode(), nextNode()
	upperThreshold, insideNode := nextNode(), nextNode()
	nodes := []string{
		absoluteReference, lowerReference, lowerSignal, upperThreshold, insideNode,
	}
	for _, node := range nodes {
		if node == "" {
			return nil, consumption, rejections
		}
	}
	if internalNodeCount(state.graph) > limits.MaxInternalNodes {
		return nil, consumption, map[string][]string{
			"graph_limit": {"windowed controlled power exceeds the bounded internal-node limit"},
		}
	}

	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, stableReference,
		[]TerminalConnection{
			{Terminal: "ANODE", Node: references[0]},
			{Terminal: "CATHODE", Node: absoluteReference},
		}, &consumption,
	)
	for _, edge := range [][2]string{
		{driveSupply, absoluteReference},
		{absoluteReference, insideNode},
		{absoluteReference, lowerReference},
		{lowerReference, references[0]},
	} {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, resistor,
			topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption,
		)
	}

	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, comparator,
		[]TerminalConnection{
			{Terminal: "IN_PLUS", Node: lowerSignal},
			{Terminal: "IN_MINUS", Node: lowerReference},
			{Terminal: "OUT", Node: insideNode},
			{Terminal: "V_MINUS", Node: references[0]},
			{Terminal: "V_PLUS", Node: driveSupply},
		}, &consumption,
	)
	for _, edge := range [][2]string{
		{input, lowerSignal},
		{insideNode, lowerSignal},
	} {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, resistor,
			topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption,
		)
	}

	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, comparator,
		[]TerminalConnection{
			{Terminal: "IN_PLUS", Node: upperThreshold},
			{Terminal: "IN_MINUS", Node: input},
			{Terminal: "OUT", Node: insideNode},
			{Terminal: "V_MINUS", Node: references[0]},
			{Terminal: "V_PLUS", Node: driveSupply},
		}, &consumption,
	)
	for _, edge := range [][2]string{
		{driveSupply, upperThreshold},
		{upperThreshold, references[0]},
		{insideNode, upperThreshold},
	} {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, resistor,
			topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption,
		)
	}

	powerRail := loadSupply
	if hasRegulatedLoad && regulatedLoad.seriesCount > 0 && regulatedLoad.parallelCount > 0 {
		powerRail = nextNode()
		if powerRail == "" {
			return nil, consumption, rejections
		}
		for branch := 0; branch < regulatedLoad.parallelCount; branch++ {
			previous := references[0]
			for stage := 0; stage < regulatedLoad.seriesCount; stage++ {
				converterOutput := nextNode()
				if converterOutput == "" {
					return nil, consumption, rejections
				}
				state = addRelationshipPrimitive(
					state, requirement, inventoryByKey, regulatedLoad.converter,
					[]TerminalConnection{
						{Terminal: "VIN_PLUS", Node: loadSupply},
						{Terminal: "VIN_MINUS", Node: references[0]},
						{Terminal: "VOUT_PLUS", Node: converterOutput},
						{Terminal: "VOUT_MINUS", Node: previous},
					}, &consumption,
				)
				previous = converterOutput
			}
			switch len(regulatedLoad.ballast) {
			case 1:
				state = addRelationshipPrimitiveAtValue(
					state, requirement, inventoryByKey, regulatedLoad.ballast[0],
					regulatedLoad.ballastValueSI[0],
					topologyTwoTerminalPlacement(previous, powerRail), &consumption,
				)
			case 2:
				ballastMiddle := nextNode()
				if ballastMiddle == "" {
					return nil, consumption, rejections
				}
				state = addRelationshipPrimitiveAtValue(
					state, requirement, inventoryByKey, regulatedLoad.ballast[0],
					regulatedLoad.ballastValueSI[0],
					topologyTwoTerminalPlacement(previous, ballastMiddle), &consumption,
				)
				state = addRelationshipPrimitiveAtValue(
					state, requirement, inventoryByKey, regulatedLoad.ballast[1],
					regulatedLoad.ballastValueSI[1],
					topologyTwoTerminalPlacement(ballastMiddle, powerRail), &consumption,
				)
			default:
				return nil, consumption, rejections
			}
		}
	}

	gateNode := nextNode()
	baseNode := ""
	levelDriveNode := ""
	senseNode := references[0]
	currentFeedbackNode := ""
	if highSide {
		baseNode = nextNode()
		levelDriveNode = nextNode()
	} else {
		senseNode = nextNode()
		currentFeedbackNode = nextNode()
	}
	if gateNode == "" || (highSide && (baseNode == "" || levelDriveNode == "")) || senseNode == "" ||
		(!highSide && currentFeedbackNode == "") {
		return nil, consumption, rejections
	}
	if highSide {
		// Buffer the wired decision node before the bipolar level shifter. The
		// comparators then see only a high-impedance feedback load, while the
		// reviewed amplifier supplies the base current needed to discharge a
		// high-side switch gate. This keeps threshold and hysteresis values
		// independent of transistor beta and collector-base loading.
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, opamp,
			[]TerminalConnection{
				{Terminal: "IN_PLUS", Node: insideNode},
				{Terminal: "IN_MINUS", Node: levelDriveNode},
				{Terminal: "OUT", Node: levelDriveNode},
				{Terminal: "V_MINUS", Node: references[0]},
				{Terminal: "V_PLUS", Node: driveSupply},
			}, &consumption,
		)
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, levelShifter,
			[]TerminalConnection{
				{Terminal: "BASE", Node: baseNode},
				{Terminal: "COLLECTOR", Node: gateNode},
				{Terminal: "EMITTER", Node: references[0]},
			}, &consumption,
		)
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, mosfet,
			[]TerminalConnection{
				{Terminal: "DRAIN", Node: output},
				{Terminal: "GATE", Node: gateNode},
				{Terminal: "SOURCE", Node: powerRail},
			}, &consumption,
		)
		if gateClampRequired {
			state = addRelationshipPrimitive(
				state, requirement, inventoryByKey, gateClamp,
				[]TerminalConnection{
					{Terminal: "ANODE", Node: gateNode},
					{Terminal: "CATHODE", Node: powerRail},
				}, &consumption,
			)
		}
	} else {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, opamp,
			[]TerminalConnection{
				{Terminal: "IN_PLUS", Node: insideNode},
				{Terminal: "IN_MINUS", Node: currentFeedbackNode},
				{Terminal: "OUT", Node: gateNode},
				{Terminal: "V_MINUS", Node: references[0]},
				{Terminal: "V_PLUS", Node: driveSupply},
			}, &consumption,
		)
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, mosfet,
			[]TerminalConnection{
				{Terminal: "DRAIN", Node: output},
				{Terminal: "GATE", Node: gateNode},
				{Terminal: "SOURCE", Node: senseNode},
			}, &consumption,
		)
	}
	edges := [][2]string{}
	gateReference := references[0]
	if highSide {
		edges = append(edges, [2]string{levelDriveNode, baseNode}, [2]string{powerRail, gateNode})
		gateReference = powerRail
	} else {
		edges = append(
			edges,
			[2]string{gateNode, references[0]},
			[2]string{senseNode, references[0]},
			[2]string{senseNode, currentFeedbackNode},
			[2]string{absoluteReference, currentFeedbackNode},
		)
	}
	for _, edge := range edges {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, resistor,
			topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption,
		)
	}
	capacitorEdges := [][2]string{{driveSupply, references[0]}}
	if !highSide {
		capacitorEdges = append(capacitorEdges, [2]string{gateReference, gateNode})
	}
	for _, edge := range capacitorEdges {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, capacitor,
			topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption,
		)
	}
	flybackAnode, flybackCathode := output, powerRail
	if highSide {
		flybackAnode, flybackCathode = references[0], output
	}
	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, flyback,
		[]TerminalConnection{
			{Terminal: "ANODE", Node: flybackAnode},
			{Terminal: "CATHODE", Node: flybackCathode},
		}, &consumption,
	)
	if len(state.graph.Instances) > limits.MaxPrimitiveInstances {
		return nil, consumption, map[string][]string{
			"graph_limit": {"windowed controlled power exceeds the bounded primitive limit"},
		}
	}
	if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
		for _, issue := range issues {
			rejections[string(issue.Code)] = append(
				rejections[string(issue.Code)], issue.Path+":"+issue.Message,
			)
		}
		return nil, consumption, rejections
	}
	if state.score.BehaviorGap != 0 {
		rejections["relationship_gap"] = append(
			rejections["relationship_gap"], fmt.Sprintf(
				"%s:gap=%d:decision=%d:switched_load=%d:absolute=%t",
				state.hash,
				state.score.BehaviorGap,
				topologyDecisionStageGap(state.graph, 2, true, topologyDecisionPolarity(requirement)),
				topologySwitchedLoadEnvelopeGap(requirement, state.graph, inventoryByKey),
				topologyGraphHasAbsoluteDecisionReference(state.graph),
			),
		)
		return nil, consumption, rejections
	}
	normalized, err := NormalizeGraph(state.graph)
	if err != nil {
		return nil, consumption, map[string][]string{"canonical_normalization_failed": {err.Error()}}
	}
	topologyHash, err := TopologyHash(normalized)
	if err != nil {
		return nil, consumption, map[string][]string{"canonical_topology_hash_failed": {err.Error()}}
	}
	consumption.CompleteGraphs++
	return []TopologyCandidate{{
		Fingerprint:  state.hash,
		TopologyHash: topologyHash,
		Score:        state.score,
		Graph:        normalized,
		Operations:   cloneGraphOperations(state.operations),
	}}, consumption, rejections
}

// topologyMOSFETGateClampPrimitive requires gate protection when the declared
// source-to-gate span consumes the design margin reserved below the absolute
// catalog VGS limit. A reviewed bidirectional clamp is eligible only when its
// modeled breakdown both exceeds the gate-on requirement and remains below the
// reserved limit after the pull resistor's maximum available clamp current.
func topologyMOSFETGateClampPrimitive(
	inventory PrimitiveInventory,
	mosfet PrimitiveCandidate,
	maximumGateSpan float64,
) (PrimitiveCandidate, bool) {
	modelID := simmodel.PrimitiveNMOSSwitchV1
	if mosfet.Kind == "p_channel_mosfet" {
		modelID = simmodel.PrimitivePMOSSwitchV1
	}
	gateLimit := primitiveModelParameter(mosfet, modelID, "max_gate_source_voltage_v")
	gateOn := primitiveModelParameter(mosfet, modelID, "gate_on_voltage_v")
	if gateLimit <= 0 || gateOn <= 0 || maximumGateSpan <= .75*gateLimit {
		return PrimitiveCandidate{}, false
	}

	const gatePullResistanceOhm = 10_000.0
	type candidate struct {
		primitive PrimitiveCandidate
		breakdown float64
	}
	candidates := []candidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "clamp_diode" ||
			!primitiveHasModel(primitive, simmodel.PrimitiveBidirectionalTVSV1) {
			continue
		}
		breakdown := primitiveModelParameter(
			primitive, simmodel.PrimitiveBidirectionalTVSV1, "breakdown_voltage_v",
		)
		dynamicResistance := primitiveModelParameter(
			primitive, simmodel.PrimitiveBidirectionalTVSV1, "dynamic_resistance_ohm",
		)
		pulseLimit := primitiveModelParameter(
			primitive, simmodel.PrimitiveBidirectionalTVSV1, "max_pulse_current_a",
		)
		maximumClampCurrent := maximumGateSpan / gatePullResistanceOhm
		clampedVoltage := breakdown + maximumClampCurrent*dynamicResistance
		if breakdown < 1.25*gateOn || dynamicResistance <= 0 ||
			pulseLimit < maximumClampCurrent || clampedVoltage > .85*gateLimit {
			continue
		}
		candidates = append(candidates, candidate{primitive: primitive, breakdown: breakdown})
	}
	slices.SortFunc(candidates, func(left, right candidate) int {
		return cmp.Or(
			cmp.Compare(
				math.Abs(left.breakdown-.75*gateLimit),
				math.Abs(right.breakdown-.75*gateLimit),
			),
			cmp.Compare(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}, true
	}
	return candidates[0].primitive, true
}

func topologyPrimitiveReferenceVoltage(primitive PrimitiveCandidate) (float64, bool) {
	for _, model := range primitive.Models {
		if model.ModelID != simmodel.PrimitiveShuntVoltageReferenceV1 {
			continue
		}
		for _, parameter := range model.Parameters {
			if parameter.Name == "output_voltage_v" && parameter.Value > 0 {
				return parameter.Value, true
			}
		}
	}
	return 0, false
}

func topologySimpleRegulatedVoltageRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	outputTarget := 0.0
	outputID := ""
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "output_voltage":
			if assertion.Observation.Kind == "port" {
				outputID = assertion.Observation.ID
				outputTarget = assertionTarget(assertion)
			}
		}
	}
	if outputTarget <= 0 || outputID == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	var opamp, resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "opamp":
			opamp = primitive
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	stableReference := topologyStableReferencePrimitive(requirement, inventory)
	passDevice := topologyRatedPowerPrimitive(requirement, inventory, "npn_bjt")
	driveBuffer := selectCurrentRelationshipPrimitive(requirement, inventory, "pnp_bjt", false, false)
	if opamp.Key == "" || resistor.Key == "" || capacitor.Key == "" ||
		stableReference.Key == "" || passDevice.Key == "" {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"regulated output requires catalog-backed controller, reference, pass-device, resistor, and capacitor relationships"},
		}
	}
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	controls := topologyNodesByRole(initial.graph, "control", "input")
	if len(supplies) != 1 || len(references) != 1 {
		return nil, Consumption{}, map[string][]string{}
	}
	output := "port_" + outputID
	consumption := Consumption{}
	rejections := map[string][]string{}
	states := []topologySearchState{}
	driveModes := []bool{false}
	if driveBuffer.Key != "" {
		driveModes = append(driveModes, true)
	}
	for _, bufferedDrive := range driveModes {
		if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates ||
			consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			break
		}
		consumption.ExpandedStates++
		state := initial
		state.graph = CloneGraph(initial.graph)
		state.operations = cloneGraphOperations(initial.operations)
		internalCount := 4
		if bufferedDrive {
			internalCount++
		}
		internal := make([]string, 0, internalCount)
		for len(internal) < internalCount {
			var node string
			state, node = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
			if node == "" {
				break
			}
			internal = append(internal, node)
		}
		if len(internal) != internalCount || internalNodeCount(state.graph) > limits.MaxInternalNodes {
			continue
		}
		absoluteReference, feedback, controllerOutput, passBase := internal[0], internal[1], internal[2], internal[3]
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, stableReference, []TerminalConnection{
			{Terminal: "ANODE", Node: references[0]},
			{Terminal: "CATHODE", Node: absoluteReference},
		}, &consumption)
		positiveInput, negativeInput := absoluteReference, feedback
		if bufferedDrive {
			positiveInput, negativeInput = feedback, absoluteReference
		}
		controllerFeedback := TerminalConnection{Terminal: "IN_MINUS", Node: negativeInput}
		var added bool
		state, added = addRelationshipActiveStage(
			state, requirement, inventoryByKey,
			relationshipActiveStage{
				Primitive: opamp,
				Input:     TerminalConnection{Terminal: "IN_PLUS", Node: positiveInput},
				Output:    TerminalConnection{Terminal: "OUT", Node: controllerOutput},
				Feedback:  &controllerFeedback,
				Bias: []TerminalConnection{
					{Terminal: "V_MINUS", Node: references[0]},
					{Terminal: "V_PLUS", Node: supplies[0]},
				},
			},
			&consumption,
		)
		if !added {
			continue
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, passDevice, []TerminalConnection{
			{Terminal: "BASE", Node: passBase},
			{Terminal: "COLLECTOR", Node: supplies[0]},
			{Terminal: "EMITTER", Node: output},
		}, &consumption)
		for _, edge := range [][2]string{
			{supplies[0], absoluteReference},
			{output, feedback}, {feedback, references[0]},
		} {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
		}
		if bufferedDrive {
			driverBase := internal[4]
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(controllerOutput, driverBase), &consumption)
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(supplies[0], driverBase), &consumption)
			state, added = addRelationshipActiveStage(
				state, requirement, inventoryByKey,
				relationshipActiveStage{
					Primitive: driveBuffer,
					Input:     TerminalConnection{Terminal: "BASE", Node: driverBase},
					Output:    TerminalConnection{Terminal: "COLLECTOR", Node: passBase},
					Bias:      []TerminalConnection{{Terminal: "EMITTER", Node: supplies[0]}},
				},
				&consumption,
			)
			if !added {
				continue
			}
		} else {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(controllerOutput, passBase), &consumption)
		}
		for _, control := range controls {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, topologyTwoTerminalPlacement(supplies[0], control), &consumption)
		}
		for _, edge := range [][2]string{
			{supplies[0], references[0]},
			{output, references[0]},
			{controllerOutput, feedback},
		} {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, capacitor, topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
		}
		states = append(states, state)
		bleeder := addRelationshipPrimitive(
			state,
			requirement,
			inventoryByKey,
			resistor,
			topologyTwoTerminalPlacement(passBase, output),
			&consumption,
		)
		if bleeder.hash != state.hash {
			states = append(states, bleeder)
		}
	}
	result := make([]TopologyCandidate, 0, len(states))
	for _, candidateState := range states {
		if len(candidateState.graph.Instances) > limits.MaxPrimitiveInstances {
			rejections["graph_limit"] = append(rejections["graph_limit"], candidateState.hash+":primitive instance limit exceeded")
			continue
		}
		if issues := ValidateCompleteGraph(candidateState.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			continue
		}
		if candidateState.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", candidateState.hash, candidateState.score.BehaviorGap))
			continue
		}
		normalized, err := NormalizeGraph(candidateState.graph)
		if err != nil {
			rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], err.Error())
			continue
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], err.Error())
			continue
		}
		consumption.CompleteGraphs++
		result = append(result, TopologyCandidate{
			Fingerprint: candidateState.hash, TopologyHash: topologyHash, Score: candidateState.score,
			Graph: normalized, Operations: cloneGraphOperations(candidateState.operations),
		})
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func topologyRejectionFrequency(requirement Requirement) (float64, bool) {
	type responseBound struct {
		frequency float64
		minimum   float64
		maximum   float64
		hasMin    bool
		hasMax    bool
	}
	responses := []responseBound{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "voltage_gain_at_frequency" ||
			assertion.FrequencyHz == nil || *assertion.FrequencyHz <= 0 {
			continue
		}
		response := responseBound{frequency: *assertion.FrequencyHz}
		if assertion.Min != nil {
			response.minimum, response.hasMin = *assertion.Min, true
		}
		if assertion.Max != nil {
			response.maximum, response.hasMax = *assertion.Max, true
		}
		responses = append(responses, response)
	}
	for _, rejected := range responses {
		if !rejected.hasMax {
			continue
		}
		lowerPass, upperPass := false, false
		for _, pass := range responses {
			if !pass.hasMin || pass.minimum <= rejected.maximum {
				continue
			}
			lowerPass = lowerPass || pass.frequency < rejected.frequency
			upperPass = upperPass || pass.frequency > rejected.frequency
		}
		if lowerPass && upperPass {
			return rejected.frequency, true
		}
	}
	return 0, false
}

// topologyRelationshipSeeds constructs only the electrical relationships
// implied directly by behavior: an observed decision output, an externally
// driven signal, a rail-derived reference, and optional positive feedback.
// It does not select a named circuit family or use corpus identities. The
// ordinary bounded graph search remains the fallback for behavior that does
// not imply these relationships or cannot be completed within the limits.
func topologyRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	minimumStages, requireFeedback := topologyDecisionObligation(requirement)
	if minimumStages != 1 {
		return nil, Consumption{}, map[string][]string{}
	}
	var resistor PrimitiveCandidate
	active := []PrimitiveCandidate{}
	referenceDiodes := []PrimitiveCandidate{}
	conditioners := []PrimitiveCandidate{}
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "resistor":
			resistor = primitive
		case "comparator":
			active = append(active, primitive)
		case "opamp":
			active = append(active, primitive)
			conditioners = append(conditioners, primitive)
		case "reference_diode":
			referenceDiodes = append(referenceDiodes, primitive)
		}
	}
	if resistor.Key == "" || len(active) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	slices.SortFunc(active, func(left, right PrimitiveCandidate) int {
		return cmp.Compare(left.Key, right.Key)
	})
	slices.SortFunc(referenceDiodes, func(left, right PrimitiveCandidate) int {
		return cmp.Compare(left.Key, right.Key)
	})
	slices.SortFunc(conditioners, func(left, right PrimitiveCandidate) int {
		return cmp.Compare(left.Key, right.Key)
	})
	inputs := topologyNodesByRole(initial.graph, "input", "control")
	outputs := topologyNodesByRole(initial.graph, "output")
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(inputs) == 0 || len(outputs) == 0 ||
		len(supplies) == 0 || len(references) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	polarities := []int{-1, 1}
	if polarity := topologyDecisionPolarity(requirement); polarity != 0 {
		polarities = []int{polarity}
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, primitive := range active {
		for _, input := range inputs {
			for _, output := range outputs {
				for _, supply := range supplies {
					for _, referenceRail := range references {
						for _, polarity := range polarities {
							if ctx.Err() != nil ||
								consumption.ExpandedStates >= policy.MaxExpandedStates ||
								consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
								break
							}
							consumption.ExpandedStates++
							state := initial
							referenceNode := ""
							state, referenceNode = addRelationshipInternalNode(
								state, requirement, inventoryByKey, &consumption,
							)
							if referenceNode == "" {
								continue
							}
							signalNode := input
							if requireFeedback {
								state, signalNode = addRelationshipInternalNode(
									state, requirement, inventoryByKey, &consumption,
								)
								if signalNode == "" {
									continue
								}
							}
							if internalNodeCount(state.graph) > limits.MaxInternalNodes {
								continue
							}
							inPlus, inMinus := referenceNode, signalNode
							if polarity > 0 {
								inPlus, inMinus = signalNode, referenceNode
							}
							state = addRelationshipPrimitive(
								state,
								requirement,
								inventoryByKey,
								primitive,
								[]TerminalConnection{
									{Terminal: "IN_MINUS", Node: inMinus},
									{Terminal: "IN_PLUS", Node: inPlus},
									{Terminal: "OUT", Node: output},
									{Terminal: "V_MINUS", Node: referenceRail},
									{Terminal: "V_PLUS", Node: supply},
								},
								&consumption,
							)
							if topologyPrimitiveRequiresPullup(primitive) {
								state = addRelationshipPrimitive(
									state,
									requirement,
									inventoryByKey,
									resistor,
									topologyTwoTerminalPlacement(output, supply),
									&consumption,
								)
							}
							for _, edge := range [][2]string{
								{referenceNode, referenceRail},
								{referenceNode, supply},
							} {
								state = addRelationshipPrimitive(
									state,
									requirement,
									inventoryByKey,
									resistor,
									topologyTwoTerminalPlacement(edge[0], edge[1]),
									&consumption,
								)
							}
							if requireFeedback {
								feedbackNode := signalNode
								if polarity < 0 {
									feedbackNode = referenceNode
								}
								for _, edge := range [][2]string{
									{signalNode, input},
									{feedbackNode, output},
								} {
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										resistor,
										topologyTwoTerminalPlacement(edge[0], edge[1]),
										&consumption,
									)
								}
							}
							if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
								consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
								continue
							}
							if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
								for _, issue := range issues {
									rejections[string(issue.Code)] = append(
										rejections[string(issue.Code)],
										issue.Path+":"+issue.Message,
									)
								}
								continue
							}
							if state.score.BehaviorGap != 0 {
								rejections["relationship_gap"] = append(
									rejections["relationship_gap"],
									fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap),
								)
								continue
							}
							normalized, err := NormalizeGraph(state.graph)
							if err != nil {
								rejections["canonical_normalization_failed"] = append(
									rejections["canonical_normalization_failed"], err.Error(),
								)
								continue
							}
							topologyHash, err := TopologyHash(normalized)
							if err != nil {
								rejections["canonical_topology_hash_failed"] = append(
									rejections["canonical_topology_hash_failed"], err.Error(),
								)
								continue
							}
							consumption.CompleteGraphs++
							candidate := TopologyCandidate{
								Fingerprint:  state.hash,
								TopologyHash: topologyHash,
								Score:        state.score,
								Graph:        normalized,
								Operations:   cloneGraphOperations(state.operations),
							}
							if existing, found := retained[topologyHash]; !found ||
								compareTopologyCandidates(candidate, existing) < 0 {
								retained[topologyHash] = candidate
							}
						}
					}
				}
			}
		}
	}
	for _, primitive := range active {
		for _, conditioner := range conditioners {
			for _, referencePrimitive := range referenceDiodes {
				for _, input := range inputs {
					for _, output := range outputs {
						for _, supply := range supplies {
							for _, referenceRail := range references {
								for _, polarity := range polarities {
									if ctx.Err() != nil ||
										consumption.ExpandedStates >= policy.MaxExpandedStates ||
										consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
										break
									}
									consumption.ExpandedStates++
									state := initial
									internal := make([]string, 0, 5)
									for index := 0; index < 4; index++ {
										var node string
										state, node = addRelationshipInternalNode(
											state, requirement, inventoryByKey, &consumption,
										)
										if node == "" {
											break
										}
										internal = append(internal, node)
									}
									if len(internal) != 4 {
										continue
									}
									absoluteReference := internal[0]
									gainNode := internal[1]
									conditionedReference := internal[2]
									signalNode := internal[3]
									decisionReference := conditionedReference
									if polarity < 0 {
										var node string
										state, node = addRelationshipInternalNode(
											state, requirement, inventoryByKey, &consumption,
										)
										if node == "" {
											continue
										}
										decisionReference = node
									}
									if internalNodeCount(state.graph) > limits.MaxInternalNodes {
										continue
									}
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										referencePrimitive,
										[]TerminalConnection{
											{Terminal: "ANODE", Node: referenceRail},
											{Terminal: "CATHODE", Node: absoluteReference},
										},
										&consumption,
									)
									if topologyPrimitiveRequiresPullup(primitive) {
										state = addRelationshipPrimitive(
											state,
											requirement,
											inventoryByKey,
											resistor,
											topologyTwoTerminalPlacement(output, supply),
											&consumption,
										)
									}
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										resistor,
										topologyTwoTerminalPlacement(supply, absoluteReference),
										&consumption,
									)
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										conditioner,
										[]TerminalConnection{
											{Terminal: "IN_MINUS", Node: gainNode},
											{Terminal: "IN_PLUS", Node: absoluteReference},
											{Terminal: "OUT", Node: conditionedReference},
											{Terminal: "V_MINUS", Node: referenceRail},
											{Terminal: "V_PLUS", Node: supply},
										},
										&consumption,
									)
									for _, edge := range [][2]string{
										{gainNode, referenceRail},
										{gainNode, conditionedReference},
									} {
										state = addRelationshipPrimitive(
											state,
											requirement,
											inventoryByKey,
											resistor,
											topologyTwoTerminalPlacement(edge[0], edge[1]),
											&consumption,
										)
									}
									inPlus, inMinus := decisionReference, signalNode
									if polarity > 0 {
										inPlus, inMinus = signalNode, decisionReference
									}
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										primitive,
										[]TerminalConnection{
											{Terminal: "IN_MINUS", Node: inMinus},
											{Terminal: "IN_PLUS", Node: inPlus},
											{Terminal: "OUT", Node: output},
											{Terminal: "V_MINUS", Node: referenceRail},
											{Terminal: "V_PLUS", Node: supply},
										},
										&consumption,
									)
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										resistor,
										topologyTwoTerminalPlacement(signalNode, input),
										&consumption,
									)
									feedbackNode := signalNode
									if polarity < 0 {
										state = addRelationshipPrimitive(
											state,
											requirement,
											inventoryByKey,
											resistor,
											topologyTwoTerminalPlacement(
												conditionedReference,
												decisionReference,
											),
											&consumption,
										)
										feedbackNode = decisionReference
									}
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										resistor,
										topologyTwoTerminalPlacement(feedbackNode, output),
										&consumption,
									)
									if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
										consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
										continue
									}
									if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
										for _, issue := range issues {
											rejections[string(issue.Code)] = append(
												rejections[string(issue.Code)],
												issue.Path+":"+issue.Message,
											)
										}
										continue
									}
									if state.score.BehaviorGap != 0 {
										rejections["relationship_gap"] = append(
											rejections["relationship_gap"],
											fmt.Sprintf(
												"%s:gap=%d:decision_gap=%d:derived_reference=%t",
												state.hash,
												state.score.BehaviorGap,
												topologyDecisionStageGap(
													state.graph,
													minimumStages,
													requireFeedback,
													polarity,
												),
												topologyNodeHasDerivedReference(
													state.graph,
													decisionReference,
													supplies,
													references,
												),
											),
										)
										continue
									}
									normalized, err := NormalizeGraph(state.graph)
									if err != nil {
										rejections["canonical_normalization_failed"] = append(
											rejections["canonical_normalization_failed"], err.Error(),
										)
										continue
									}
									topologyHash, err := TopologyHash(normalized)
									if err != nil {
										rejections["canonical_topology_hash_failed"] = append(
											rejections["canonical_topology_hash_failed"], err.Error(),
										)
										continue
									}
									consumption.CompleteGraphs++
									candidate := TopologyCandidate{
										Fingerprint:  state.hash,
										TopologyHash: topologyHash,
										Score:        state.score,
										Graph:        normalized,
										Operations:   cloneGraphOperations(state.operations),
									}
									if existing, found := retained[topologyHash]; !found ||
										compareTopologyCandidates(candidate, existing) < 0 {
										retained[topologyHash] = candidate
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
		consumption.BudgetExhausted = true
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func topologyDecisionObligation(requirement Requirement) (int, bool) {
	minimumStages := 0
	requireFeedback := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "hysteresis":
			minimumStages = max(minimumStages, 1)
			requireFeedback = true
		case "rising_threshold", "falling_threshold", "threshold_voltage", "threshold_current":
			minimumStages = max(minimumStages, 1)
		case "lower_threshold", "upper_threshold":
			minimumStages = max(minimumStages, 2)
		}
	}
	return minimumStages, requireFeedback
}

// topologyAnalogRelationshipSeeds constructs active-transfer candidates with a
// real input time constant when the requested behavior simultaneously requires
// gain above unity and a bounded cutoff. It retains both an incomplete
// feedback relationship for diagnosis-driven graph repair and the corresponding
// closed-loop relationship; both come only from gain and frequency obligations.
func topologyAnalogRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	requireGain, requireCutoff := false, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "voltage_gain":
			requireGain = requireGain ||
				(assertion.Min != nil && *assertion.Min > 1)
		case "cutoff_frequency":
			requireCutoff = true
		}
	}
	if !requireGain || !requireCutoff {
		return nil, Consumption{}, map[string][]string{}
	}
	var opamp, resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "opamp":
			opamp = primitive
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	if opamp.Key == "" || resistor.Key == "" || capacitor.Key == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	inputs := topologyNodesByRole(initial.graph, "input")
	outputs := topologyNodesByRole(initial.graph, "output")
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, input := range inputs {
		for _, output := range outputs {
			for _, supply := range supplies {
				for _, reference := range references {
					if ctx.Err() != nil ||
						consumption.ExpandedStates >= policy.MaxExpandedStates ||
						consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
						break
					}
					consumption.ExpandedStates++
					state := initial
					var filterNode, feedbackNode string
					state, filterNode = addRelationshipInternalNode(
						state, requirement, inventoryByKey, &consumption,
					)
					state, feedbackNode = addRelationshipInternalNode(
						state, requirement, inventoryByKey, &consumption,
					)
					if filterNode == "" || feedbackNode == "" ||
						internalNodeCount(state.graph) > limits.MaxInternalNodes {
						continue
					}
					state = addRelationshipPrimitive(
						state,
						requirement,
						inventoryByKey,
						opamp,
						[]TerminalConnection{
							{Terminal: "IN_MINUS", Node: feedbackNode},
							{Terminal: "IN_PLUS", Node: filterNode},
							{Terminal: "OUT", Node: output},
							{Terminal: "V_MINUS", Node: reference},
							{Terminal: "V_PLUS", Node: supply},
						},
						&consumption,
					)
					for _, edge := range [][2]string{
						{input, filterNode},
						{feedbackNode, reference},
					} {
						state = addRelationshipPrimitive(
							state,
							requirement,
							inventoryByKey,
							resistor,
							topologyTwoTerminalPlacement(edge[0], edge[1]),
							&consumption,
						)
					}
					state = addRelationshipPrimitive(
						state,
						requirement,
						inventoryByKey,
						capacitor,
						topologyTwoTerminalPlacement(filterNode, reference),
						&consumption,
					)
					// Retain the complete active/time-constant graph before
					// closing its loop as an explicit repair candidate. It is
					// electrically complete but still has a scored behavioral
					// gap; trusted simulation and the generic repair lane must
					// discover whether a feedback edge resolves that gap.
					type analogRelationshipState struct {
						state      topologySearchState
						repairable bool
					}
					relationshipStates := []analogRelationshipState{{
						state: state, repairable: true,
					}}
					closedLoop := addRelationshipPrimitive(
						state,
						requirement,
						inventoryByKey,
						resistor,
						topologyTwoTerminalPlacement(feedbackNode, output),
						&consumption,
					)
					if closedLoop.hash != state.hash {
						relationshipStates = append(
							relationshipStates,
							analogRelationshipState{state: closedLoop},
						)
					}
					if len(closedLoop.graph.Instances) < limits.MaxPrimitiveInstances &&
						consumption.GeneratedGraphs < policy.MaxGeneratedGraphs {
						secondPole := addRelationshipPrimitive(
							closedLoop,
							requirement,
							inventoryByKey,
							capacitor,
							topologyTwoTerminalPlacement(output, reference),
							&consumption,
						)
						if secondPole.hash != closedLoop.hash {
							relationshipStates = append(
								relationshipStates,
								analogRelationshipState{state: secondPole},
							)
						}
					}
					for _, relationship := range relationshipStates {
						relationshipState := relationship.state
						if len(relationshipState.graph.Instances) > limits.MaxPrimitiveInstances ||
							consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
							continue
						}
						if issues := ValidateCompleteGraph(relationshipState.graph, inventory, limits); len(issues) != 0 {
							for _, issue := range issues {
								rejections[string(issue.Code)] = append(
									rejections[string(issue.Code)],
									issue.Path+":"+issue.Message,
								)
							}
							continue
						}
						if relationshipState.score.BehaviorGap != 0 &&
							!relationship.repairable {
							rejections["relationship_gap"] = append(
								rejections["relationship_gap"],
								fmt.Sprintf("%s:gap=%d", relationshipState.hash, relationshipState.score.BehaviorGap),
							)
							continue
						}
						normalized, err := NormalizeGraph(relationshipState.graph)
						if err != nil {
							rejections["canonical_normalization_failed"] = append(
								rejections["canonical_normalization_failed"], err.Error(),
							)
							continue
						}
						topologyHash, err := TopologyHash(normalized)
						if err != nil {
							rejections["canonical_topology_hash_failed"] = append(
								rejections["canonical_topology_hash_failed"], err.Error(),
							)
							continue
						}
						consumption.CompleteGraphs++
						candidate := TopologyCandidate{
							Fingerprint:  relationshipState.hash,
							TopologyHash: topologyHash,
							Repairable:   relationship.repairable,
							Score:        relationshipState.score,
							Graph:        normalized,
							Operations:   cloneGraphOperations(relationshipState.operations),
						}
						if existing, found := retained[topologyHash]; !found ||
							compareTopologyCandidates(candidate, existing) < 0 {
							retained[topologyHash] = candidate
						}
					}
				}
			}
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

type fullWaveBehaviorEnvelope struct {
	input  string
	output string
}

func topologyFullWaveBehaviorEnvelope(requirement Requirement) (fullWaveBehaviorEnvelope, bool) {
	cases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	envelope := fullWaveBehaviorEnvelope{}
	positive, negative := false, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "output_voltage" || assertion.Analysis != "dc_operating_point" ||
			assertion.Observation.Kind != "port" || assertionTarget(assertion) <= 0 {
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			for _, condition := range cases[caseID].Conditions {
				if condition.Axis != "input_voltage" || condition.Min != condition.Max || condition.Min == 0 {
					continue
				}
				if envelope.input != "" && (envelope.input != condition.Target || envelope.output != assertion.Observation.ID) {
					return fullWaveBehaviorEnvelope{}, false
				}
				envelope.input, envelope.output = condition.Target, assertion.Observation.ID
				positive = positive || condition.Min > 0
				negative = negative || condition.Min < 0
			}
		}
	}
	return envelope, envelope.input != "" && positive && negative
}

func topologyFullWaveRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	envelope, required := topologyFullWaveBehaviorEnvelope(requirement)
	if !required {
		return nil, Consumption{}, map[string][]string{}
	}
	var opamp, resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "opamp":
			opamp = primitive
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	diode := topologyPrecisionSignalDiodePrimitive(requirement, inventory)
	if opamp.Key == "" || diode.Key == "" || resistor.Key == "" || capacitor.Key == "" {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"full-wave magnitude transfer requires catalog-backed amplifier, signal-diode, and resistor relationships"},
		}
	}
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	references := topologyNodesByRole(initial.graph, "reference")
	if highRail == "" || lowRail == "" || len(references) != 1 {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"full-wave magnitude transfer requires ordered positive and negative rails plus one reference node"},
		}
	}
	input, output := "port_"+envelope.input, "port_"+envelope.output
	consumption := Consumption{ExpandedStates: 1}
	rejections := map[string][]string{}
	state := initial
	internal := make([]string, 0, 7)
	for range 7 {
		if ctx.Err() != nil || consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			break
		}
		var node string
		state, node = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
		if node == "" {
			break
		}
		internal = append(internal, node)
	}
	if len(internal) != 7 || internalNodeCount(state.graph) > limits.MaxInternalNodes {
		rejections["graph_limit"] = []string{fmt.Sprintf(
			"full-wave magnitude relationship requires 7 internal nodes; created=%d limit=%d",
			len(internal), limits.MaxInternalNodes,
		)}
		return nil, consumption, rejections
	}
	halfSum, halfDrive, halfOutput := internal[0], internal[1], internal[2]
	mixJoin, mixNode := internal[3], internal[4]
	feedbackJoin, gainFeedback := internal[5], internal[6]
	addOpamp := func(plus, minus, out string) {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, opamp, []TerminalConnection{
			{Terminal: "IN_PLUS", Node: plus},
			{Terminal: "IN_MINUS", Node: minus},
			{Terminal: "OUT", Node: out},
			{Terminal: "V_MINUS", Node: lowRail},
			{Terminal: "V_PLUS", Node: highRail},
		}, &consumption)
	}
	addOpamp(references[0], halfSum, halfDrive)
	addOpamp(mixNode, gainFeedback, output)
	for _, placement := range [][]TerminalConnection{
		{{Terminal: "ANODE", Node: halfDrive}, {Terminal: "CATHODE", Node: halfOutput}},
		{{Terminal: "ANODE", Node: halfSum}, {Terminal: "CATHODE", Node: halfDrive}},
	} {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, diode, placement, &consumption)
	}
	for _, edge := range [][2]string{
		{input, halfSum}, {halfOutput, halfSum}, {halfOutput, references[0]},
		{input, mixJoin}, {mixJoin, mixNode}, {halfOutput, mixNode},
		{output, feedbackJoin}, {feedbackJoin, gainFeedback}, {gainFeedback, references[0]},
	} {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
	}
	states := []topologySearchState{state}
	compensated := addRelationshipPrimitive(
		state,
		requirement,
		inventoryByKey,
		capacitor,
		topologyTwoTerminalPlacement(output, gainFeedback),
		&consumption,
	)
	if compensated.hash != state.hash {
		states = append(states, compensated)
	}
	result := make([]TopologyCandidate, 0, len(states))
	for _, candidateState := range states {
		if len(candidateState.graph.Instances) > limits.MaxPrimitiveInstances {
			rejections["graph_limit"] = append(rejections["graph_limit"], fmt.Sprintf(
				"full-wave magnitude relationship requires %d primitive instances; limit=%d",
				len(candidateState.graph.Instances), limits.MaxPrimitiveInstances,
			))
			continue
		}
		if issues := ValidateCompleteGraph(candidateState.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			continue
		}
		if candidateState.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", candidateState.hash, candidateState.score.BehaviorGap))
			continue
		}
		normalized, err := NormalizeGraph(candidateState.graph)
		if err != nil {
			rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], err.Error())
			continue
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], err.Error())
			continue
		}
		consumption.CompleteGraphs++
		result = append(result, TopologyCandidate{
			Fingerprint: candidateState.hash, TopologyHash: topologyHash, Score: candidateState.score,
			Graph: normalized, Operations: cloneGraphOperations(candidateState.operations),
		})
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

// topologyTransimpedanceRelationshipSeeds closes the terminal relationships
// implied by a bounded current-to-voltage transfer: the current excitation is
// observed at an inverting active input, the non-inverting input is tied to its
// semantic reference, and a catalog impedance feeds the observed voltage back
// to the current-summing node. An optional parallel reactance gives trusted
// simulation a distinct compensated alternative when the inventory supports
// one. No named implementation family or requirement identity participates.
func topologyTransimpedanceRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	type relationship struct {
		input, output, reference string
	}
	relationships := []relationship{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "transimpedance" || assertion.Excitation == nil ||
			assertionTarget(assertion) <= 0 {
			continue
		}
		input := observationNodeID(initial.graph, requirement, *assertion.Excitation)
		output := observationNodeID(initial.graph, requirement, assertion.Observation)
		reference := referenceNodeForDomain(initial.graph, *assertion.Excitation)
		if input == "" || output == "" || reference == "" || input == output {
			continue
		}
		relationships = append(relationships, relationship{input: input, output: output, reference: reference})
	}
	slices.SortFunc(relationships, func(left, right relationship) int {
		return cmp.Or(
			cmp.Compare(left.input, right.input),
			cmp.Compare(left.output, right.output),
			cmp.Compare(left.reference, right.reference),
		)
	})
	relationships = slices.Compact(relationships)
	if len(relationships) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	byKind := map[string]PrimitiveCandidate{}
	for _, primitive := range representatives {
		if _, found := byKind[primitive.Kind]; !found {
			byKind[primitive.Kind] = primitive
		}
	}
	opamp, resistor, capacitor := byKind["opamp"], byKind["resistor"], byKind["capacitor"]
	if opamp.Key == "" || resistor.Key == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"current-to-voltage transfer requires catalog-backed active and resistive relationships"}}
	}
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	if highRail == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"current-to-voltage transfer requires a bounded supply relationship"}}
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	retain := func(state topologySearchState) {
		if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
			internalNodeCount(state.graph) > limits.MaxInternalNodes {
			return
		}
		if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			return
		}
		if state.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap))
			return
		}
		normalized, err := NormalizeGraph(state.graph)
		if err != nil {
			rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], err.Error())
			return
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], err.Error())
			return
		}
		consumption.CompleteGraphs++
		candidate := TopologyCandidate{
			Fingerprint: state.hash, TopologyHash: topologyHash,
			Score: state.score, Graph: normalized,
			Operations: cloneGraphOperations(state.operations),
		}
		if existing, found := retained[topologyHash]; !found || compareTopologyCandidates(candidate, existing) < 0 {
			retained[topologyHash] = candidate
		}
	}
	for _, relation := range relationships {
		if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates ||
			consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			break
		}
		consumption.ExpandedStates++
		negativeRail := lowRail
		if negativeRail == "" {
			negativeRail = relation.reference
		}
		activeState := addRelationshipPrimitive(
			initial, requirement, inventoryByKey, opamp,
			[]TerminalConnection{
				{Terminal: "IN_MINUS", Node: relation.input},
				{Terminal: "IN_PLUS", Node: relation.reference},
				{Terminal: "OUT", Node: relation.output},
				{Terminal: "V_MINUS", Node: negativeRail},
				{Terminal: "V_PLUS", Node: highRail},
			},
			&consumption,
		)
		direct := addRelationshipPrimitive(
			activeState, requirement, inventoryByKey, resistor,
			topologyTwoTerminalPlacement(relation.input, relation.output),
			&consumption,
		)
		retain(direct)
		if capacitor.Key != "" && consumption.GeneratedGraphs < policy.MaxGeneratedGraphs {
			compensated := addRelationshipPrimitive(
				direct, requirement, inventoryByKey, capacitor,
				topologyTwoTerminalPlacement(relation.input, relation.output),
				&consumption,
			)
			if compensated.hash != direct.hash {
				retain(compensated)
			}
		}
		series, midpoint := addRelationshipInternalNode(
			activeState, requirement, inventoryByKey, &consumption,
		)
		if midpoint != "" && internalNodeCount(series.graph) <= limits.MaxInternalNodes {
			series = addRelationshipPrimitive(
				series, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(relation.input, midpoint),
				&consumption,
			)
			series = addRelationshipPrimitive(
				series, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(midpoint, relation.output),
				&consumption,
			)
			retain(series)
			if capacitor.Key != "" && consumption.GeneratedGraphs < policy.MaxGeneratedGraphs {
				compensatedSeries := addRelationshipPrimitive(
					series, requirement, inventoryByKey, capacitor,
					topologyTwoTerminalPlacement(relation.input, relation.output),
					&consumption,
				)
				if compensatedSeries.hash != series.hash {
					retain(compensatedSeries)
				}
			}
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

// topologyPowerTransferRelationshipSeeds expands a bounded voltage-transfer
// obligation into several electrically distinct active paths when the same
// contract also requires meaningful load drive. The trigger is entirely
// behavioral: gain plus output power/current/swing and dynamic quality. It
// deliberately emits direct, single-ended, and complementary alternatives so
// trusted simulation can reject or rank them; no project identity or named
// amplifier class participates in generation.
func topologyPowerTransferRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	if !topologyRequiresPowerTransfer(requirement) {
		return nil, Consumption{}, map[string][]string{}
	}
	byKind := map[string]PrimitiveCandidate{}
	for _, primitive := range representatives {
		byKind[primitive.Kind] = primitive
	}
	resistor := byKind["resistor"]
	capacitor := byKind["capacitor"]
	opamp := byKind["opamp"]
	stableReference := topologyStableReferencePrimitive(requirement, inventory)
	powerController := topologyPowerControllerPrimitive(requirement, inventory)
	if powerController.Key == "" {
		powerController = opamp
	}
	if resistor.Key == "" || capacitor.Key == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	for _, kind := range []string{
		"npn_bjt", "pnp_bjt", "n_channel_mosfet", "p_channel_mosfet",
	} {
		if primitive := topologyRatedPowerPrimitive(requirement, inventory, kind); primitive.Key != "" {
			byKind[kind] = primitive
		}
	}
	inputs := topologyNodesByRole(initial.graph, "input", "control")
	outputs := topologyNodesByRole(initial.graph, "output")
	supplyNodes := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(inputs) == 0 || len(outputs) == 0 || len(references) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	if highRail == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	if lowRail == "" {
		lowRail = references[0]
	}

	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	retain := func(state topologySearchState) {
		if state.hash == "" {
			rejections["invalid_graph_hash"] = append(rejections["invalid_graph_hash"], "candidate graph has no deterministic hash")
			return
		}
		if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
			internalNodeCount(state.graph) > limits.MaxInternalNodes {
			rejections["graph_limit"] = append(
				rejections["graph_limit"],
				fmt.Sprintf(
					"instances=%d/%d internal_nodes=%d/%d",
					len(state.graph.Instances), limits.MaxPrimitiveInstances,
					internalNodeCount(state.graph), limits.MaxInternalNodes,
				),
			)
			return
		}
		if consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
			consumption.BudgetExhausted = true
			return
		}
		if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(
					rejections[string(issue.Code)], issue.Path+":"+issue.Message,
				)
			}
			return
		}
		if state.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(
				rejections["relationship_gap"],
				fmt.Sprintf("%s:gap=%d score=%#v", state.hash, state.score.BehaviorGap, state.score),
			)
			return
		}
		normalized, err := NormalizeGraph(CloneGraph(state.graph))
		if err != nil {
			rejections["canonical_normalization_failed"] = append(
				rejections["canonical_normalization_failed"], err.Error(),
			)
			return
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(
				rejections["canonical_topology_hash_failed"], err.Error(),
			)
			return
		}
		consumption.CompleteGraphs++
		candidate := TopologyCandidate{
			Fingerprint:  state.hash,
			TopologyHash: topologyHash,
			Score:        state.score,
			Graph:        normalized,
			Operations:   cloneGraphOperations(state.operations),
		}
		if existing, found := retained[topologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retained[topologyHash] = candidate
		}
	}
	addNode := func(state topologySearchState) (topologySearchState, string, bool) {
		if ctx.Err() != nil || consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			return state, "", false
		}
		next, node := addRelationshipInternalNode(
			state, requirement, inventoryByKey, &consumption,
		)
		return next, node, node != ""
	}
	add := func(state topologySearchState, primitive PrimitiveCandidate, terminals []TerminalConnection) topologySearchState {
		if primitive.Key == "" || ctx.Err() != nil ||
			consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			return state
		}
		return addRelationshipPrimitive(
			state, requirement, inventoryByKey, primitive, terminals, &consumption,
		)
	}
	redirect := func(
		state topologySearchState,
		instanceID string,
		terminal string,
		node string,
	) topologySearchState {
		graph, err := RedirectPrimitiveTerminal(
			state.graph, inventory, instanceID, terminal, node,
		)
		if err != nil {
			return state
		}
		hash, err := GraphHash(graph)
		if err != nil {
			return state
		}
		primitive := GraphInstance{}
		for _, instance := range state.graph.Instances {
			if instance.ID == instanceID {
				primitive = instance
				break
			}
		}
		operations := cloneGraphOperations(state.operations)
		operations = append(operations, GraphOperation{
			Number:        len(operations) + 1,
			Kind:          "redirect_terminal",
			PrimitiveKey:  primitive.PrimitiveKey,
			PrimitiveKind: primitive.Kind,
			Node:          node,
			Connections: []TerminalConnection{{
				Terminal: terminal,
				Node:     node,
			}},
			BeforeHash: state.hash,
			AfterHash:  hash,
		})
		consumption.GeneratedGraphs++
		return topologySearchState{
			graph: graph, hash: hash,
			score:      scoreTopologyGraph(requirement, graph, inventoryByKey, hash),
			operations: operations,
		}
	}

	for _, input := range inputs {
		for _, output := range outputs {
			if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates ||
				consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
				consumption.BudgetExhausted = true
				break
			}
			consumption.ExpandedStates++
			reference := references[0]

			// A direct closed-loop active path is an important low-complexity
			// alternative even when it will later fail load-current or thermal
			// evidence. Retaining it makes that rejection explicit.
			if opamp.Key != "" {
				state := initial
				var feedback string
				var ok bool
				state, feedback, ok = addNode(state)
				if ok {
					state = add(state, opamp, []TerminalConnection{
						{Terminal: "IN_MINUS", Node: feedback},
						{Terminal: "IN_PLUS", Node: input},
						{Terminal: "OUT", Node: output},
						{Terminal: "V_MINUS", Node: lowRail},
						{Terminal: "V_PLUS", Node: highRail},
					})
					state = add(state, resistor, topologyTwoTerminalPlacement(feedback, reference))
					state = add(state, resistor, topologyTwoTerminalPlacement(feedback, output))
					retain(state)
					compensated := add(
						state,
						capacitor,
						topologyTwoTerminalPlacement(feedback, output),
					)
					retain(compensated)
				}
			}

			// Single-ended transconductance path with bias, degeneration, and
			// AC endpoint access. Its standing current and gain are sized later
			// from the behavioral bounds rather than fixed here.
			if npn := byKind["npn_bjt"]; npn.Key != "" {
				lowSideController := topologyLowSideCurrentControllerPrimitive(requirement, inventory, npn)
				if lowSideController.Key == "" {
					rejections["current_controller_unavailable"] = append(rejections["current_controller_unavailable"], npn.Key)
				}
				state := initial
				nodes := make([]string, 0, 3)
				for len(nodes) < 3 {
					var node string
					var ok bool
					state, node, ok = addNode(state)
					if !ok {
						break
					}
					nodes = append(nodes, node)
				}
				if len(nodes) == 3 {
					base, emitter, collector := nodes[0], nodes[1], nodes[2]
					state = add(state, npn, []TerminalConnection{
						{Terminal: "BASE", Node: base},
						{Terminal: "COLLECTOR", Node: collector},
						{Terminal: "EMITTER", Node: emitter},
					})
					for _, edge := range [][2]string{
						{highRail, collector},
						{emitter, reference},
						{collector, base},
					} {
						state = add(state, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]))
					}
					state = add(state, capacitor, topologyTwoTerminalPlacement(input, base))
					state = add(state, capacitor, topologyTwoTerminalPlacement(collector, output))
					retain(state)

					bypassed := add(
						state,
						capacitor,
						topologyTwoTerminalPlacement(emitter, reference),
					)
					retain(bypassed)

					parallelLoad := add(
						state,
						resistor,
						topologyTwoTerminalPlacement(highRail, collector),
					)
					retain(parallelLoad)
					parallelBypassed := add(
						parallelLoad,
						capacitor,
						topologyTwoTerminalPlacement(emitter, reference),
					)
					retain(parallelBypassed)
				}

				// A high-input-impedance controller can close the signal and DC
				// bias loop around the same single-ended power device. This is a
				// distinct architecture derived from the input-impedance and gain
				// obligations, not from a named amplifier class.
				if powerController.Key != "" && lowSideController.Key != "" && len(supplyNodes) == 1 {
					// A controller-biased emitter follower is the non-inverting
					// single-ended alternative. Two parallel standing-current
					// current sink lets catalog power/value constraints realize a
					// continuous-conduction load without naming an amplifier class.
					follower := initial
					followerNodes := make([]string, 0, 8)
					for len(followerNodes) < 8 {
						var node string
						var ok bool
						follower, node, ok = addNode(follower)
						if !ok {
							break
						}
						followerNodes = append(followerNodes, node)
					}
					if len(followerNodes) == 8 {
						midRail, biasedInput, feedback := followerNodes[0], followerNodes[1], followerNodes[2]
						emitter, drive := followerNodes[3], followerNodes[4]
						sinkSense, sinkDrive, sinkReference := followerNodes[5], followerNodes[6], followerNodes[7]
						follower = add(follower, powerController, []TerminalConnection{
							{Terminal: "IN_MINUS", Node: feedback},
							{Terminal: "IN_PLUS", Node: biasedInput},
							{Terminal: "OUT", Node: drive},
							{Terminal: "V_MINUS", Node: reference},
							{Terminal: "V_PLUS", Node: highRail},
						})
						follower = add(follower, npn, []TerminalConnection{
							{Terminal: "BASE", Node: drive},
							{Terminal: "COLLECTOR", Node: highRail},
							{Terminal: "EMITTER", Node: emitter},
						})
						follower = add(follower, lowSideController, []TerminalConnection{
							{Terminal: "IN_MINUS", Node: sinkSense},
							{Terminal: "IN_PLUS", Node: sinkReference},
							{Terminal: "OUT", Node: sinkDrive},
							{Terminal: "V_MINUS", Node: reference},
							{Terminal: "V_PLUS", Node: highRail},
						})
						follower = add(follower, npn, []TerminalConnection{
							{Terminal: "BASE", Node: sinkDrive},
							{Terminal: "COLLECTOR", Node: emitter},
							{Terminal: "EMITTER", Node: sinkSense},
						})
						resistorEdges := [][2]string{
							{highRail, midRail},
							{midRail, reference},
							{midRail, biasedInput},
							{midRail, feedback},
							{emitter, feedback},
							{sinkSense, reference},
							{sinkSense, reference},
						}
						if stableReference.Key != "" {
							var referenceCathode string
							var ok bool
							follower, referenceCathode, ok = addNode(follower)
							if ok {
								follower = add(follower, stableReference, []TerminalConnection{
									{Terminal: "ANODE", Node: reference},
									{Terminal: "CATHODE", Node: referenceCathode},
								})
								resistorEdges = append(resistorEdges,
									[2]string{highRail, referenceCathode},
									[2]string{referenceCathode, sinkReference},
									[2]string{sinkReference, reference},
								)
							}
						} else {
							resistorEdges = append(resistorEdges,
								[2]string{highRail, sinkReference},
								[2]string{sinkReference, reference},
								[2]string{sinkReference, reference},
							)
						}
						for _, edge := range resistorEdges {
							follower = add(follower, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]))
						}
						follower = add(follower, capacitor, topologyTwoTerminalPlacement(midRail, reference))
						follower = add(follower, capacitor, topologyTwoTerminalPlacement(sinkReference, reference))
						follower = add(follower, capacitor, topologyTwoTerminalPlacement(input, biasedInput))
						follower = add(follower, capacitor, topologyTwoTerminalPlacement(emitter, output))
						retain(follower)
					}

					controlled := initial
					nodes := make([]string, 0, 6)
					for len(nodes) < 6 {
						var node string
						var ok bool
						controlled, node, ok = addNode(controlled)
						if !ok {
							break
						}
						nodes = append(nodes, node)
					}
					if len(nodes) == 6 {
						midRail, biasedInput, feedback := nodes[0], nodes[1], nodes[2]
						emitter, collector, drive := nodes[3], nodes[4], nodes[5]
						controlled = add(controlled, powerController, []TerminalConnection{
							{Terminal: "IN_MINUS", Node: biasedInput},
							{Terminal: "IN_PLUS", Node: feedback},
							{Terminal: "OUT", Node: drive},
							{Terminal: "V_MINUS", Node: reference},
							{Terminal: "V_PLUS", Node: highRail},
						})
						controlled = add(controlled, npn, []TerminalConnection{
							{Terminal: "BASE", Node: drive},
							{Terminal: "COLLECTOR", Node: collector},
							{Terminal: "EMITTER", Node: emitter},
						})
						for _, edge := range [][2]string{
							{highRail, midRail},
							{midRail, reference},
							{midRail, biasedInput},
							{collector, feedback},
							{feedback, midRail},
							{highRail, collector},
							{emitter, reference},
						} {
							controlled = add(controlled, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]))
						}
						controlled = add(controlled, capacitor, topologyTwoTerminalPlacement(input, biasedInput))
						controlled = add(controlled, capacitor, topologyTwoTerminalPlacement(collector, output))
						retain(controlled)

						controlledBypassed := add(
							controlled,
							capacitor,
							topologyTwoTerminalPlacement(emitter, reference),
						)
						retain(controlledBypassed)
						controlledParallel := add(
							controlled,
							resistor,
							topologyTwoTerminalPlacement(highRail, collector),
						)
						retain(controlledParallel)
						controlledParallelBypassed := add(
							controlledParallel,
							capacitor,
							topologyTwoTerminalPlacement(emitter, reference),
						)
						retain(controlledParallelBypassed)
					}
				}
			}

			if powerController.Key != "" && len(supplyNodes) >= 2 && highRail != lowRail {
				for _, pair := range [][2]string{
					{"npn_bjt", "pnp_bjt"},
					{"n_channel_mosfet", "p_channel_mosfet"},
				} {
					upper, lower := byKind[pair[0]], byKind[pair[1]]
					if upper.Key == "" || lower.Key == "" {
						continue
					}
					state := initial
					nodes := make([]string, 0, 6)
					for len(nodes) < 6 {
						var node string
						var ok bool
						state, node, ok = addNode(state)
						if !ok {
							break
						}
						nodes = append(nodes, node)
					}
					if len(nodes) != 6 {
						continue
					}
					feedback, drive, upperControl, lowerControl := nodes[0], nodes[1], nodes[2], nodes[3]
					upperOutput, lowerOutput := nodes[4], nodes[5]
					controllerOutput := drive
					state = add(state, powerController, []TerminalConnection{
						{Terminal: "IN_MINUS", Node: feedback},
						{Terminal: "IN_PLUS", Node: input},
						{Terminal: "OUT", Node: controllerOutput},
						{Terminal: "V_MINUS", Node: lowRail},
						{Terminal: "V_PLUS", Node: highRail},
					})
					state = add(state, resistor, topologyTwoTerminalPlacement(feedback, reference))
					state = add(state, resistor, topologyTwoTerminalPlacement(feedback, output))
					for _, edge := range [][2]string{
						{upperOutput, output},
						{lowerOutput, output},
					} {
						state = add(state, resistor, topologyTwoTerminalPlacement(edge[0], edge[1]))
					}
					switch pair[0] {
					case "npn_bjt":
						// Retain a direct complementary follower. Global feedback
						// can make this low-complexity Class-B alternative useful,
						// while trusted distortion and idle-current evidence decide
						// whether explicit bias spreading is required.
						direct := state
						direct = add(direct, resistor, topologyTwoTerminalPlacement(drive, upperControl))
						direct = add(direct, resistor, topologyTwoTerminalPlacement(drive, lowerControl))
						direct = add(direct, upper, []TerminalConnection{
							{Terminal: "BASE", Node: upperControl},
							{Terminal: "COLLECTOR", Node: highRail},
							{Terminal: "EMITTER", Node: upperOutput},
						})
						direct = add(direct, lower, []TerminalConnection{
							{Terminal: "BASE", Node: lowerControl},
							{Terminal: "COLLECTOR", Node: lowRail},
							{Terminal: "EMITTER", Node: lowerOutput},
						})
						retain(direct)
						// A controller-to-load ballast path turns the same pair into
						// a current booster: the controller owns the zero-crossing and
						// the power devices take over once roughly one base-emitter
						// drop develops across the ballast resistor. This supplies a
						// small-signal path without requiring a fixture-named class.
						boosted := add(
							direct,
							resistor,
							topologyTwoTerminalPlacement(drive, output),
						)
						retain(boosted)
						directCompensated := add(
							direct,
							capacitor,
							topologyTwoTerminalPlacement(controllerOutput, feedback),
						)
						retain(directCompensated)
						boostedCompensated := add(
							boosted,
							capacitor,
							topologyTwoTerminalPlacement(controllerOutput, feedback),
						)
						retain(boostedCompensated)

						// A compound complementary follower trades two additional
						// active devices and two junction drops for beta multiplication.
						// Its rail-fed chain carries only driver-base current; the
						// driver emitters furnish the power-device base current.
						compound := state
						compoundNodes := make([]string, 0, 4)
						for len(compoundNodes) < 4 {
							var node string
							var compoundOK bool
							compound, node, compoundOK = addNode(compound)
							if !compoundOK {
								break
							}
							compoundNodes = append(compoundNodes, node)
						}
						if len(compoundNodes) == 4 {
							upperMid, lowerMid := compoundNodes[0], compoundNodes[1]
							upperPowerBase, lowerPowerBase := compoundNodes[2], compoundNodes[3]
							if upper.Key != "" && lower.Key != "" {
								compound = add(compound, resistor, topologyTwoTerminalPlacement(highRail, upperControl))
								for _, edge := range [][2]string{
									{upperControl, upperMid},
									{upperMid, drive},
								} {
									compound = add(compound, upper, []TerminalConnection{
										{Terminal: "BASE", Node: edge[0]},
										{Terminal: "COLLECTOR", Node: edge[0]},
										{Terminal: "EMITTER", Node: edge[1]},
									})
								}
								for _, edge := range [][2]string{
									{drive, lowerMid},
									{lowerMid, lowerControl},
								} {
									compound = add(compound, lower, []TerminalConnection{
										{Terminal: "EMITTER", Node: edge[0]},
										{Terminal: "BASE", Node: edge[1]},
										{Terminal: "COLLECTOR", Node: edge[1]},
									})
								}
								compound = add(compound, resistor, topologyTwoTerminalPlacement(lowerControl, lowRail))
								compound = add(compound, upper, []TerminalConnection{
									{Terminal: "BASE", Node: upperControl},
									{Terminal: "COLLECTOR", Node: highRail},
									{Terminal: "EMITTER", Node: upperPowerBase},
								})
								compound = add(compound, upper, []TerminalConnection{
									{Terminal: "BASE", Node: upperPowerBase},
									{Terminal: "COLLECTOR", Node: highRail},
									{Terminal: "EMITTER", Node: upperOutput},
								})
								compound = add(compound, lower, []TerminalConnection{
									{Terminal: "BASE", Node: lowerControl},
									{Terminal: "COLLECTOR", Node: lowRail},
									{Terminal: "EMITTER", Node: lowerPowerBase},
								})
								compound = add(compound, lower, []TerminalConnection{
									{Terminal: "BASE", Node: lowerPowerBase},
									{Terminal: "COLLECTOR", Node: lowRail},
									{Terminal: "EMITTER", Node: lowerOutput},
								})
								compound = add(compound, resistor, topologyTwoTerminalPlacement(upperPowerBase, upperOutput))
								compound = add(compound, resistor, topologyTwoTerminalPlacement(lowerPowerBase, lowerOutput))
								retain(compound)
								compoundCompensated := add(
									compound,
									capacitor,
									topologyTwoTerminalPlacement(controllerOutput, feedback),
								)
								retain(compoundCompensated)
							}
						}

						// The biased alternative is symmetric around the controller
						// output: rail-fed diode drops establish the two base voltages
						// and both shift with the midpoint drive. This preserves access
						// to both output polarities.
						var upperBase, lowerBase string
						var ok bool
						state, upperBase, ok = addNode(state)
						if !ok {
							continue
						}
						state, lowerBase, ok = addNode(state)
						if !ok {
							continue
						}
						if diode := byKind["signal_diode"]; diode.Key != "" {
							state = add(state, resistor, topologyTwoTerminalPlacement(highRail, upperControl))
							state = add(state, diode, []TerminalConnection{
								{Terminal: "ANODE", Node: upperControl},
								{Terminal: "CATHODE", Node: drive},
							})
							state = add(state, diode, []TerminalConnection{
								{Terminal: "ANODE", Node: drive},
								{Terminal: "CATHODE", Node: lowerControl},
							})
							state = add(state, resistor, topologyTwoTerminalPlacement(lowerControl, lowRail))
						} else {
							state = add(state, resistor, topologyTwoTerminalPlacement(drive, upperControl))
							state = add(state, resistor, topologyTwoTerminalPlacement(drive, lowerControl))
						}
						state = add(state, resistor, topologyTwoTerminalPlacement(upperControl, upperBase))
						state = add(state, resistor, topologyTwoTerminalPlacement(lowerControl, lowerBase))
						state = add(state, upper, []TerminalConnection{
							{Terminal: "BASE", Node: upperBase},
							{Terminal: "COLLECTOR", Node: highRail},
							{Terminal: "EMITTER", Node: upperOutput},
						})
						state = add(state, lower, []TerminalConnection{
							{Terminal: "BASE", Node: lowerBase},
							{Terminal: "COLLECTOR", Node: lowRail},
							{Terminal: "EMITTER", Node: lowerOutput},
						})
					case "n_channel_mosfet":
						state = add(state, resistor, topologyTwoTerminalPlacement(drive, upperControl))
						state = add(state, resistor, topologyTwoTerminalPlacement(drive, lowerControl))
						state = add(state, upper, []TerminalConnection{
							{Terminal: "DRAIN", Node: highRail},
							{Terminal: "GATE", Node: upperControl},
							{Terminal: "SOURCE", Node: upperOutput},
						})
						state = add(state, lower, []TerminalConnection{
							{Terminal: "DRAIN", Node: lowRail},
							{Terminal: "GATE", Node: lowerControl},
							{Terminal: "SOURCE", Node: lowerOutput},
						})
					}
					retain(state)
					compensated := add(
						state,
						capacitor,
						topologyTwoTerminalPlacement(controllerOutput, feedback),
					)
					retain(compensated)

					series := state
					var upperJoin, lowerJoin string
					var ok bool
					series, upperJoin, ok = addNode(series)
					if !ok {
						continue
					}
					series, lowerJoin, ok = addNode(series)
					if !ok {
						continue
					}
					upperResistor, upperTerminal := "", ""
					lowerResistor, lowerTerminal := "", ""
					for _, instance := range series.graph.Instances {
						if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
							continue
						}
						for _, terminal := range instance.Terminals {
							other := instance.Terminals[0].Node
							if other == terminal.Node {
								other = instance.Terminals[1].Node
							}
							if terminal.Node != output {
								continue
							}
							switch other {
							case upperOutput:
								upperResistor, upperTerminal = instance.ID, terminal.Terminal
							case lowerOutput:
								lowerResistor, lowerTerminal = instance.ID, terminal.Terminal
							}
						}
					}
					if upperResistor != "" && lowerResistor != "" {
						series = redirect(series, upperResistor, upperTerminal, upperJoin)
						series = redirect(series, lowerResistor, lowerTerminal, lowerJoin)
						series = add(series, resistor, topologyTwoTerminalPlacement(upperJoin, output))
						series = add(series, resistor, topologyTwoTerminalPlacement(lowerJoin, output))
						retain(series)
						seriesCompensated := add(
							series,
							capacitor,
							topologyTwoTerminalPlacement(controllerOutput, feedback),
						)
						retain(seriesCompensated)
					}
				}
			}
		}
	}
	if consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
		consumption.BudgetExhausted = true
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func topologyRequiresPowerTransfer(requirement Requirement) bool {
	requireGain := false
	requireLoadDrive := false
	requireDynamicQuality := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "voltage_gain":
			// Unity-gain followers still require a power-transfer stage when
			// paired with explicit load-drive and dynamic-quality obligations.
			requireGain = requireGain ||
				(assertion.Min != nil && *assertion.Min >= 1)
		case "output_current", "output_power", "output_swing", "peak_current", "peak_voltage":
			requireLoadDrive = true
		case "bandwidth", "cutoff_frequency", "phase_margin", "thd", "total_harmonic_distortion":
			requireDynamicQuality = true
		}
	}
	return requireGain && requireLoadDrive && requireDynamicQuality
}

func topologyRatedPowerPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	kind string,
) PrimitiveCandidate {
	requireThermal := false
	requireSOA := false
	requiredSOAMargin := 1.0
	requiredAnalyses := requirementAnalysisSet(requirement)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		requireThermal = requireThermal || assertion.Metric == "junction_temperature"
		requireSOA = requireSOA || assertion.Metric == "soa_margin"
		if assertion.Metric == "soa_margin" && assertion.Min != nil && *assertion.Min > 0 {
			requiredSOAMargin = math.Max(requiredSOAMargin, *assertion.Min)
		}
	}
	type ratedPowerCandidate struct {
		primitive PrimitiveCandidate
		soaMargin float64
		soaKnown  bool
	}
	candidates := []ratedPowerCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != kind || !ratingsCoverRequirement(requirement, primitive) ||
			(requireThermal && !primitiveHasThermalEvidence(primitive)) ||
			(requireSOA && !primitiveHasSOAEvidence(primitive)) {
			continue
		}
		soaMargin := 0.0
		soaKnown := false
		if requireSOA {
			soaMargin, soaKnown = topologyPowerDCSoaMargin(requirement, primitive)
		}
		candidates = append(candidates, ratedPowerCandidate{primitive: primitive, soaMargin: soaMargin, soaKnown: soaKnown})
	}
	slices.SortFunc(candidates, func(left, right ratedPowerCandidate) int {
		if requireSOA {
			if left.soaKnown != right.soaKnown {
				if left.soaKnown {
					return -1
				}
				return 1
			}
			if !left.soaKnown {
				return compareRepresentativePrimitives(left.primitive, right.primitive, requiredAnalyses)
			}
			leftPass := left.soaMargin >= requiredSOAMargin
			rightPass := right.soaMargin >= requiredSOAMargin
			if leftPass != rightPass {
				if leftPass {
					return -1
				}
				return 1
			}
			if comparison := cmp.Compare(left.soaMargin, right.soaMargin); comparison != 0 {
				if !leftPass {
					return -comparison
				}
				return comparison
			}
		}
		return compareRepresentativePrimitives(left.primitive, right.primitive, requiredAnalyses)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

// topologyPowerDCSoaMargin estimates whether a power-transfer primitive has a
// reviewed continuous SOA boundary wide enough for the declared output
// envelope. It is a topology-independent selection prior: trusted simulation
// remains authoritative for simultaneous device voltage/current waveforms.
func topologyPowerDCSoaMargin(requirement Requirement, primitive PrimitiveCandidate) (float64, bool) {
	targets := derivePowerTransferSizingTargets(requirement)
	minimumLoadResistance := targets.loadResistance
	maximumOutputPeak := targets.outputPeakVoltage
	maximumAmbientC := math.Inf(-1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "peak_voltage" && assertion.Max != nil {
			maximumOutputPeak = math.Max(maximumOutputPeak, math.Abs(*assertion.Max))
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			switch condition.Axis {
			case "load_resistance":
				if condition.Min > 0 && (minimumLoadResistance <= 0 || condition.Min < minimumLoadResistance) {
					minimumLoadResistance = condition.Min
				}
			case "ambient_temperature", "temperature":
				maximumAmbientC = math.Max(maximumAmbientC, condition.Max)
			}
		}
	}
	stressCurrentA := targets.quiescentCurrent
	if maximumOutputPeak > 0 && minimumLoadResistance > 0 {
		stressCurrentA = math.Max(stressCurrentA, maximumOutputPeak/minimumLoadResistance)
	}
	stressVoltageV := protectedVoltageMaximumSupply(requirement)
	if stressCurrentA <= 0 || stressVoltageV <= 0 {
		return 0, false
	}
	allowedCurrentA := math.Inf(1)
	found := false
	for _, model := range primitive.Models {
		maximumTemperatureC := 0.0
		for _, parameter := range model.Parameters {
			if parameter.Name == "max_temperature_c" {
				maximumTemperatureC = math.Max(maximumTemperatureC, parameter.Value)
			}
		}
		for _, envelope := range model.TransientSOA {
			if !envelope.DC {
				continue
			}
			currentA, covered := protectedVoltageSOACurrent(envelope.Points, stressVoltageV)
			if !covered || currentA <= 0 {
				continue
			}
			if finite(maximumAmbientC) && maximumAmbientC > envelope.CaseTemperatureC {
				span := maximumTemperatureC - envelope.CaseTemperatureC
				if span <= 0 || maximumAmbientC >= maximumTemperatureC {
					return 0, false
				}
				currentA *= (maximumTemperatureC - maximumAmbientC) / span
			}
			allowedCurrentA = math.Min(allowedCurrentA, currentA)
			found = true
		}
	}
	if !found || !finite(allowedCurrentA) || allowedCurrentA <= 0 {
		return 0, false
	}
	return allowedCurrentA / stressCurrentA, true
}

func topologyPowerControllerPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
) PrimitiveCandidate {
	targetGain := 0.0
	targetBandwidth := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "voltage_gain":
			targetGain = math.Max(targetGain, assertionTarget(assertion))
		case "bandwidth", "cutoff_frequency":
			targetBandwidth = math.Max(targetBandwidth, assertionTarget(assertion))
		}
	}
	// Discrete power stages add device poles and consume loop gain well before
	// the requested closed-loop corner. Reserve twenty times the gain-bandwidth
	// product so the controller still has corrective authority at that corner.
	targetGBW := targetGain * targetBandwidth * topologyControllerGBWReserve
	type scoredController struct {
		primitive     PrimitiveCandidate
		gbw           float64
		outputMargin  float64
		belowRequired bool
		distance      float64
	}
	candidates := []scoredController{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "opamp" || !ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		gbw := 0.0
		outputHighMargin := math.Inf(1)
		outputLowMargin := math.Inf(1)
		for _, model := range primitive.Models {
			for _, parameter := range model.Parameters {
				switch parameter.Name {
				case "gain_bandwidth_hz":
					gbw = math.Max(gbw, parameter.Value)
				case "output_high_margin_v":
					outputHighMargin = math.Min(outputHighMargin, parameter.Value)
				case "output_low_margin_v":
					outputLowMargin = math.Min(outputLowMargin, parameter.Value)
				}
			}
		}
		if gbw <= 0 || !finite(outputHighMargin) || !finite(outputLowMargin) {
			continue
		}
		distance := 0.0
		if targetGBW > 0 {
			distance = math.Abs(math.Log(gbw / targetGBW))
		}
		candidates = append(candidates, scoredController{
			primitive:     primitive,
			gbw:           gbw,
			outputMargin:  math.Max(outputHighMargin, outputLowMargin),
			belowRequired: targetGBW > 0 && gbw < targetGBW,
			distance:      distance,
		})
	}
	slices.SortFunc(candidates, func(left, right scoredController) int {
		if left.belowRequired != right.belowRequired {
			if left.belowRequired {
				return 1
			}
			return -1
		}
		return cmp.Or(
			cmp.Compare(left.outputMargin, right.outputMargin),
			cmp.Compare(left.distance, right.distance),
			cmp.Compare(right.gbw, left.gbw),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

func topologyStableReferencePrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
) PrimitiveCandidate {
	type scoredReference struct {
		primitive PrimitiveCandidate
		voltage   float64
	}
	candidates := []scoredReference{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "reference_diode" ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		voltage := 0.0
		validModel := false
		for _, model := range primitive.Models {
			if model.ModelID != simmodel.PrimitiveShuntVoltageReferenceV1 {
				continue
			}
			validModel = true
			for _, parameter := range model.Parameters {
				if parameter.Name == "output_voltage_v" {
					voltage = parameter.Value
				}
			}
		}
		if validModel && voltage > 0 {
			candidates = append(candidates, scoredReference{primitive: primitive, voltage: voltage})
		}
	}
	slices.SortFunc(candidates, func(left, right scoredReference) int {
		return cmp.Or(
			cmp.Compare(left.voltage, right.voltage),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

func topologyPrecisionSignalDiodePrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
) PrimitiveCandidate {
	type scoredDiode struct {
		primitive PrimitiveCandidate
		leakage   float64
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	minimumRail, maximumRail := 0.0, 0.0
	for _, domain := range requirement.Requirements.Domains {
		if domain.MinVoltageV != nil {
			minimumRail = math.Min(minimumRail, *domain.MinVoltageV)
		}
		if domain.MaxVoltageV != nil {
			maximumRail = math.Max(maximumRail, *domain.MaxVoltageV)
		}
	}
	requiredReverseVoltage := maximumRail - minimumRail
	candidates := []scoredDiode{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "signal_diode" ||
			!ratingsCoverRequirement(requirement, primitive) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) {
			continue
		}
		leakage := math.Inf(1)
		reverseVoltage := 0.0
		for _, model := range primitive.Models {
			if model.ModelID != simmodel.PrimitiveDiodeShockleyV1 {
				continue
			}
			for _, parameter := range model.Parameters {
				if parameter.Name == "saturation_current_a" && parameter.Value > 0 {
					leakage = math.Min(leakage, parameter.Value)
				}
				if parameter.Name == "max_reverse_voltage_v" {
					reverseVoltage = math.Max(reverseVoltage, parameter.Value)
				}
			}
		}
		if finite(leakage) && reverseVoltage >= requiredReverseVoltage {
			candidates = append(candidates, scoredDiode{primitive: primitive, leakage: leakage})
		}
	}
	slices.SortFunc(candidates, func(left, right scoredDiode) int {
		return cmp.Or(
			cmp.Compare(left.leakage, right.leakage),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

func topologyFlybackDiodePrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
) PrimitiveCandidate {
	type scoredDiode struct {
		primitive      PrimitiveCandidate
		forwardVoltage float64
		thermalExcess  float64
		thermalRise    float64
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	requiredCurrent := requirementMaximumMetric(requirement, "peak_current")
	minimumVoltage, maximumVoltage := 0.0, 0.0
	for _, port := range requirement.Requirements.Ports {
		if port.Electrical.MaxCurrentA != nil {
			requiredCurrent = math.Max(requiredCurrent, *port.Electrical.MaxCurrentA)
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		for _, voltage := range []*float64{domain.MinVoltageV, domain.NominalVoltageV, domain.MaxVoltageV} {
			if voltage != nil {
				minimumVoltage = math.Min(minimumVoltage, *voltage)
				maximumVoltage = math.Max(maximumVoltage, *voltage)
			}
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "supply_voltage" || condition.Axis == "input_voltage" {
				minimumVoltage = math.Min(minimumVoltage, math.Min(condition.Min, condition.Max))
				maximumVoltage = math.Max(maximumVoltage, math.Max(condition.Min, condition.Max))
			}
		}
	}
	if requiredCurrent <= 0 {
		requiredCurrent = 1e-3
	}
	requiredReverseVoltage := maximumVoltage - minimumVoltage
	candidates := []scoredDiode{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "signal_diode" ||
			!ratingsCoverRequirement(requirement, primitive) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			(requiredAnalyses["electrothermal"] && !primitiveHasThermalEvidence(primitive)) {
			continue
		}
		for _, model := range primitive.Models {
			if model.ModelID != simmodel.PrimitiveDiodeShockleyV1 {
				continue
			}
			parameters := map[string]float64{}
			for _, parameter := range model.Parameters {
				parameters[parameter.Name] = parameter.Value
			}
			if parameters["max_forward_current_a"] < requiredCurrent ||
				parameters["max_reverse_voltage_v"] < requiredReverseVoltage ||
				parameters["saturation_current_a"] <= 0 ||
				parameters["emission_coefficient"] <= 0 ||
				parameters["junction_temperature_k"] <= 0 {
				continue
			}
			forwardVoltage := parameters["emission_coefficient"] * 8.617333262e-5 *
				parameters["junction_temperature_k"] * math.Log1p(requiredCurrent/parameters["saturation_current_a"])
			if finite(forwardVoltage) && forwardVoltage > 0 {
				thermalExcess, thermalRise := topologyFlybackDiodeThermalScore(
					requirement, model, requiredCurrent, forwardVoltage,
				)
				candidates = append(candidates, scoredDiode{
					primitive: primitive, forwardVoltage: forwardVoltage,
					thermalExcess: thermalExcess, thermalRise: thermalRise,
				})
			}
		}
	}
	slices.SortFunc(candidates, func(left, right scoredDiode) int {
		return cmp.Or(
			cmp.Compare(left.thermalExcess, right.thermalExcess),
			cmp.Compare(left.thermalRise, right.thermalRise),
			cmp.Compare(left.forwardVoltage, right.forwardVoltage),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

func topologyFlybackDiodeThermalScore(
	requirement Requirement,
	model PrimitiveModelContract,
	requiredCurrent float64,
	forwardVoltage float64,
) (float64, float64) {
	parameters := map[string]float64{}
	for _, parameter := range model.Parameters {
		parameters[parameter.Name] = parameter.Value
	}
	theta := parameters["thermal_resistance_c_per_w"]
	if theta <= 0 {
		theta = parameters["junction_to_ambient_c_per_w"]
	}
	if theta <= 0 && model.ThermalModel != nil && model.ThermalModel.Reference == "junction_to_ambient" {
		for _, stage := range model.ThermalModel.Stages {
			theta += stage.ThermalResistanceCPerW
		}
	}
	maximumTemperature := parameters["max_temperature_c"]
	requestedMaximum := requirementMaximumMetric(requirement, "junction_temperature")
	if requestedMaximum > 0 && (maximumTemperature <= 0 || requestedMaximum < maximumTemperature) {
		maximumTemperature = requestedMaximum
	}
	if theta <= 0 || maximumTemperature <= 0 {
		return math.MaxFloat64, math.MaxFloat64
	}
	maximumAmbient := 25.0
	foundAmbient := false
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "ambient_temperature" && condition.Axis != "temperature" {
				continue
			}
			if !foundAmbient || condition.Max > maximumAmbient {
				maximumAmbient = condition.Max
			}
			foundAmbient = true
		}
	}
	thermalRise := requiredCurrent * forwardVoltage * theta
	predicted := maximumAmbient + thermalRise
	return math.Max(0, predicted-maximumTemperature), thermalRise
}

// topologyLowSideCurrentControllerPrimitive selects a controller for a
// feedback-regulated transistor current sink. Unlike the signal controller,
// this role is dominated by output-current evidence and low-rail swing: the
// controller must source the selected transistor's base current while sensing
// a small voltage above the reference rail.
func topologyLowSideCurrentControllerPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	driven PrimitiveCandidate,
) PrimitiveCandidate {
	targets := derivePowerTransferSizingTargets(requirement)
	standingCurrent := targets.quiescentCurrent
	if targets.outputPeakVoltage > 0 && targets.loadResistance > 0 {
		standingCurrent = math.Max(
			standingCurrent,
			1.05*targets.outputPeakVoltage/targets.loadResistance,
		)
	}
	minimumBeta := math.Inf(1)
	for _, model := range driven.Models {
		for _, parameter := range model.Parameters {
			if parameter.Name == "forward_beta" && parameter.Value > 0 {
				minimumBeta = math.Min(minimumBeta, parameter.Value)
			}
		}
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == "model_parameters.forward_beta" && uncertainty.Minimum > 0 {
				minimumBeta = math.Min(minimumBeta, uncertainty.Minimum)
			}
		}
	}
	if !finite(minimumBeta) || minimumBeta <= 0 {
		return PrimitiveCandidate{}
	}
	requiredOutputCurrent := 1.25 * standingCurrent / minimumBeta
	type scoredController struct {
		primitive PrimitiveCandidate
		lowMargin float64
		gbw       float64
	}
	candidates := []scoredController{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "opamp" || !ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		outputCurrent := 0.0
		for _, rating := range primitive.Ratings {
			if rating.Kind == "output_current" {
				outputCurrent = math.Max(outputCurrent, boundMaximum(rating))
			}
		}
		if outputCurrent < requiredOutputCurrent {
			continue
		}
		lowMargin := math.Inf(1)
		gbw := 0.0
		for _, model := range primitive.Models {
			for _, parameter := range model.Parameters {
				switch parameter.Name {
				case "output_low_margin_v":
					lowMargin = math.Min(lowMargin, parameter.Value)
				case "gain_bandwidth_hz":
					gbw = math.Max(gbw, parameter.Value)
				}
			}
		}
		if !finite(lowMargin) || gbw <= 0 {
			continue
		}
		candidates = append(candidates, scoredController{
			primitive: primitive,
			lowMargin: lowMargin,
			gbw:       gbw,
		})
	}
	slices.SortFunc(candidates, func(left, right scoredController) int {
		return cmp.Or(
			cmp.Compare(left.lowMargin, right.lowMargin),
			cmp.Compare(right.gbw, left.gbw),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

// topologyCurrentSenseAmplifierPrimitive selects the observation amplifier for
// a regulated current relationship. Its output represents the commanded
// current as a voltage, so it must reproduce the active command range without
// clipping at either rail and retain enough bandwidth for the requested
// settling time. This is intentionally distinct from selecting the amplifier
// that drives a high-side pass device: the two roles have different swing and
// drive obligations.
func topologyCurrentSenseAmplifierPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
) PrimitiveCandidate {
	commandMinimum, commandMaximum, commandFound := regulatedCurrentCommandVoltageRange(requirement)
	minimumSupply := minimumTransconductanceSupplyVoltage(requirement)
	if !commandFound || minimumSupply <= commandMaximum {
		return PrimitiveCandidate{}
	}
	requiredGBW := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "settling_time" && assertion.Max != nil && *assertion.Max > 0 {
			requiredGBW = math.Max(
				requiredGBW,
				topologyControllerGBWReserve / *assertion.Max,
			)
		}
	}
	type scoredAmplifier struct {
		primitive  PrimitiveCandidate
		lowMargin  float64
		highMargin float64
		gbw        float64
	}
	candidates := []scoredAmplifier{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "opamp" || !ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		lowMargin, highMargin, gbw, found := primitiveOpAmpCapabilities(primitive)
		if !found || lowMargin > commandMinimum ||
			highMargin > minimumSupply-commandMaximum || gbw < requiredGBW {
			continue
		}
		candidates = append(candidates, scoredAmplifier{
			primitive: primitive, lowMargin: lowMargin, highMargin: highMargin, gbw: gbw,
		})
	}
	slices.SortFunc(candidates, func(left, right scoredAmplifier) int {
		return cmp.Or(
			cmp.Compare(left.lowMargin, right.lowMargin),
			cmp.Compare(left.highMargin, right.highMargin),
			cmp.Compare(right.gbw, left.gbw),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

// topologyHighSideCurrentControllerPrimitive selects an amplifier capable of
// driving a high-side bipolar pass stage. Fault-safe turn-off requires its
// output to approach the positive rail within a conservative base-emitter
// junction drop, while rated operation requires reviewed output-current
// capacity for the worst-corner base current.
func topologyHighSideCurrentControllerPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	driven PrimitiveCandidate,
	parallelDevices int,
) PrimitiveCandidate {
	const (
		baseDriveReserve     = 2.0
		turnOffRailHeadroomV = 1.0
	)
	minimumBeta := primitiveMinimumForwardBeta(driven)
	requiredCurrent := requiredTransconductanceOutputCurrent(requirement)
	if minimumBeta <= 0 || requiredCurrent <= 0 || parallelDevices <= 0 {
		return PrimitiveCandidate{}
	}
	requiredOutputCurrent := baseDriveReserve * requiredCurrent /
		(minimumBeta * float64(parallelDevices))
	type scoredController struct {
		primitive     PrimitiveCandidate
		highMargin    float64
		lowMargin     float64
		outputCurrent float64
		gbw           float64
	}
	candidates := []scoredController{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "opamp" || !ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		outputCurrent := 0.0
		for _, rating := range primitive.Ratings {
			if rating.Kind == "output_current" {
				outputCurrent = math.Max(outputCurrent, boundMaximum(rating))
			}
		}
		lowMargin, highMargin, gbw, found := primitiveOpAmpCapabilities(primitive)
		if !found || outputCurrent < requiredOutputCurrent || highMargin > turnOffRailHeadroomV {
			continue
		}
		candidates = append(candidates, scoredController{
			primitive: primitive, highMargin: highMargin, lowMargin: lowMargin,
			outputCurrent: outputCurrent, gbw: gbw,
		})
	}
	slices.SortFunc(candidates, func(left, right scoredController) int {
		return cmp.Or(
			cmp.Compare(left.highMargin, right.highMargin),
			cmp.Compare(left.lowMargin, right.lowMargin),
			cmp.Compare(right.outputCurrent, left.outputCurrent),
			cmp.Compare(right.gbw, left.gbw),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

func regulatedCurrentCommandVoltageRange(requirement Requirement) (float64, float64, bool) {
	cases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	minimum := math.Inf(1)
	maximum := math.Inf(-1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "transconductance" || assertion.Excitation == nil ||
			assertion.Excitation.Kind != "port" {
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			for _, condition := range cases[caseID].Conditions {
				if condition.Axis != "input_voltage" || condition.Target != assertion.Excitation.ID {
					continue
				}
				minimum = math.Min(minimum, condition.Min)
				maximum = math.Max(maximum, condition.Max)
			}
		}
	}
	if math.IsInf(minimum, 1) || math.IsInf(maximum, -1) || minimum < 0 || maximum < minimum {
		return 0, 0, false
	}
	return minimum, maximum, true
}

func primitiveOpAmpCapabilities(primitive PrimitiveCandidate) (float64, float64, float64, bool) {
	lowMargin := math.Inf(1)
	highMargin := math.Inf(1)
	gbw := 0.0
	for _, model := range primitive.Models {
		if model.ModelID != simmodel.PrimitiveOpAmpV1 {
			continue
		}
		for _, parameter := range model.Parameters {
			switch parameter.Name {
			case "output_low_margin_v":
				lowMargin = math.Min(lowMargin, parameter.Value)
			case "output_high_margin_v":
				highMargin = math.Min(highMargin, parameter.Value)
			case "gain_bandwidth_hz":
				gbw = math.Max(gbw, parameter.Value)
			}
		}
	}
	return lowMargin, highMargin, gbw,
		finite(lowMargin) && finite(highMargin) && lowMargin >= 0 && highMargin >= 0 && gbw > 0
}

func primitiveHasSOAEvidence(primitive PrimitiveCandidate) bool {
	for _, model := range primitive.Models {
		if len(model.TransientSOA) != 0 {
			return true
		}
	}
	return false
}

func topologyPowerRails(requirement Requirement, graph CandidateGraph) (string, string) {
	type rail struct {
		id      string
		voltage float64
	}
	rails := []rail{}
	for _, id := range topologyNodesByRole(graph, "supply") {
		if voltage, ok := topologyNodeNominalVoltage(requirement, graph, id); ok {
			rails = append(rails, rail{id: id, voltage: voltage})
		}
	}
	slices.SortFunc(rails, func(left, right rail) int {
		if order := cmp.Compare(right.voltage, left.voltage); order != 0 {
			return order
		}
		return cmp.Compare(left.id, right.id)
	})
	if len(rails) == 0 {
		return "", ""
	}
	if len(rails) == 1 {
		return rails[0].id, ""
	}
	return rails[0].id, rails[len(rails)-1].id
}

type conditionalTransferRelationship struct {
	input   string
	output  string
	control string
}

// topologyConditionalTransferRelationshipSeeds constructs a buffered analog
// path with a control-driven shunt when the same excitation and observation
// require materially different bounded gains in non-overlapping control
// ranges. It derives only terminal relationships from the behavioral contract:
// a high-impedance follower preserves the enabled transfer, a series branch
// isolates its driver, and a switching primitive attenuates the observed node.
func topologyConditionalTransferRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	relationships := topologyConditionalTransferRelationships(requirement)
	if len(relationships) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	var opamp, mosfet, resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "opamp":
			opamp = primitive
		case "n_channel_mosfet":
			mosfet = primitive
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	if opamp.Key == "" || mosfet.Key == "" ||
		resistor.Key == "" || capacitor.Key == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	const (
		defaultBiasResistance = 47_000.0
		couplingCapacitance   = 4.7e-6
	)
	biasResistor := topologyPrimitiveClosestValue(
		inventory.Primitives,
		"resistor",
		defaultBiasResistance,
	)
	couplingCapacitor := topologyPrimitiveClosestValue(
		inventory.Primitives,
		"capacitor",
		couplingCapacitance,
	)
	if biasResistor.Key == "" {
		biasResistor = resistor
	}
	if couplingCapacitor.Key == "" {
		couplingCapacitor = capacitor
	}
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(supplies) == 0 || len(references) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	nodeExists := map[string]bool{}
	for _, node := range initial.graph.Nodes {
		nodeExists[node.ID] = true
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, relationship := range relationships {
		input := "port_" + relationship.input
		output := "port_" + relationship.output
		control := "port_" + relationship.control
		if !nodeExists[input] || !nodeExists[output] || !nodeExists[control] {
			continue
		}
		for _, supply := range supplies {
			for _, reference := range references {
				if ctx.Err() != nil ||
					consumption.ExpandedStates >= policy.MaxExpandedStates ||
					consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
					break
				}
				consumption.ExpandedStates++
				state := initial
				var biasNode, driverNode string
				state, biasNode = addRelationshipInternalNode(
					state, requirement, inventoryByKey, &consumption,
				)
				state, driverNode = addRelationshipInternalNode(
					state, requirement, inventoryByKey, &consumption,
				)
				if biasNode == "" || driverNode == "" ||
					internalNodeCount(state.graph) > limits.MaxInternalNodes {
					continue
				}
				state = addRelationshipPrimitive(
					state,
					requirement,
					inventoryByKey,
					opamp,
					[]TerminalConnection{
						{Terminal: "IN_MINUS", Node: driverNode},
						{Terminal: "IN_PLUS", Node: biasNode},
						{Terminal: "OUT", Node: driverNode},
						{Terminal: "V_MINUS", Node: reference},
						{Terminal: "V_PLUS", Node: supply},
					},
					&consumption,
				)
				state = addRelationshipPrimitiveAtValue(
					state,
					requirement,
					inventoryByKey,
					couplingCapacitor,
					couplingCapacitance,
					topologyTwoTerminalPlacement(input, biasNode),
					&consumption,
				)
				state = addRelationshipPrimitiveAtValue(
					state,
					requirement,
					inventoryByKey,
					couplingCapacitor,
					couplingCapacitance,
					topologyTwoTerminalPlacement(driverNode, output),
					&consumption,
				)
				for _, edge := range [][2]string{
					{supply, biasNode},
					{biasNode, reference},
					{output, reference},
				} {
					state = addRelationshipPrimitive(
						state,
						requirement,
						inventoryByKey,
						biasResistor,
						topologyTwoTerminalPlacement(edge[0], edge[1]),
						&consumption,
					)
				}
				state = addRelationshipPrimitive(
					state,
					requirement,
					inventoryByKey,
					mosfet,
					[]TerminalConnection{
						{Terminal: "DRAIN", Node: output},
						{Terminal: "GATE", Node: control},
						{Terminal: "SOURCE", Node: reference},
					},
					&consumption,
				)
				if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
					consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
					continue
				}
				if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
					for _, issue := range issues {
						rejections[string(issue.Code)] = append(
							rejections[string(issue.Code)],
							issue.Path+":"+issue.Message,
						)
					}
					continue
				}
				if state.score.BehaviorGap != 0 {
					rejections["relationship_gap"] = append(
						rejections["relationship_gap"],
						fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap),
					)
					continue
				}
				normalized, err := NormalizeGraph(state.graph)
				if err != nil {
					rejections["canonical_normalization_failed"] = append(
						rejections["canonical_normalization_failed"], err.Error(),
					)
					continue
				}
				topologyHash, err := TopologyHash(normalized)
				if err != nil {
					rejections["canonical_topology_hash_failed"] = append(
						rejections["canonical_topology_hash_failed"], err.Error(),
					)
					continue
				}
				consumption.CompleteGraphs++
				candidate := TopologyCandidate{
					Fingerprint:  state.hash,
					TopologyHash: topologyHash,
					Score:        state.score,
					Graph:        normalized,
					Operations:   cloneGraphOperations(state.operations),
				}
				if existing, found := retained[topologyHash]; !found ||
					compareTopologyCandidates(candidate, existing) < 0 {
					retained[topologyHash] = candidate
				}
			}
		}
	}
	if consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
		consumption.BudgetExhausted = true
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func topologyConditionalTransferRelationships(
	requirement Requirement,
) []conditionalTransferRelationship {
	type transferBound struct {
		excitation  string
		observation string
		control     string
		controlMin  float64
		controlMax  float64
		gainMin     float64
		gainMax     float64
		hasGainMin  bool
		hasGainMax  bool
	}
	operatingCaseByID := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		operatingCaseByID[operatingCase.ID] = operatingCase
	}
	bounds := []transferBound{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "voltage_gain" ||
			assertion.Excitation == nil ||
			assertion.Excitation.Kind != "port" ||
			assertion.Observation.Kind != "port" {
			continue
		}
		for _, operatingCaseID := range assertion.OperatingCases {
			operatingCase, found := operatingCaseByID[operatingCaseID]
			if !found {
				continue
			}
			for _, condition := range operatingCase.Conditions {
				if condition.Axis != "input_voltage" ||
					!requirementPortIsControl(requirement, condition.Target) {
					continue
				}
				bound := transferBound{
					excitation:  assertion.Excitation.ID,
					observation: assertion.Observation.ID,
					control:     condition.Target,
					controlMin:  condition.Min,
					controlMax:  condition.Max,
				}
				if assertion.Min != nil {
					bound.gainMin, bound.hasGainMin = *assertion.Min, true
				}
				if assertion.Max != nil {
					bound.gainMax, bound.hasGainMax = *assertion.Max, true
				}
				bounds = append(bounds, bound)
			}
		}
	}
	unique := map[string]conditionalTransferRelationship{}
	for _, passBound := range bounds {
		if !passBound.hasGainMin || passBound.gainMin <= 0 {
			continue
		}
		for _, attenuatedBound := range bounds {
			if passBound.excitation != attenuatedBound.excitation ||
				passBound.observation != attenuatedBound.observation ||
				passBound.control != attenuatedBound.control ||
				!attenuatedBound.hasGainMax ||
				attenuatedBound.gainMax >= passBound.gainMin {
				continue
			}
			controlRangesSeparate :=
				passBound.controlMax < attenuatedBound.controlMin ||
					attenuatedBound.controlMax < passBound.controlMin
			if !controlRangesSeparate {
				continue
			}
			relationship := conditionalTransferRelationship{
				input:   passBound.excitation,
				output:  passBound.observation,
				control: passBound.control,
			}
			key := relationship.input + "|" + relationship.output + "|" + relationship.control
			unique[key] = relationship
		}
	}
	result := make([]conditionalTransferRelationship, 0, len(unique))
	for _, relationship := range unique {
		result = append(result, relationship)
	}
	slices.SortFunc(result, func(left, right conditionalTransferRelationship) int {
		return cmp.Or(
			cmp.Compare(left.input, right.input),
			cmp.Compare(left.output, right.output),
			cmp.Compare(left.control, right.control),
		)
	})
	return result
}

func topologyConditionalTransferPassGain(requirement Requirement) float64 {
	passGain := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "voltage_gain" ||
			assertion.Min == nil ||
			*assertion.Min <= 0 {
			continue
		}
		target := *assertion.Min
		if assertion.Max != nil && *assertion.Max > target {
			target = *assertion.Max
		}
		if target > passGain {
			passGain = target
		}
	}
	return passGain
}

func topologyPrimitiveClosestValue(
	primitives []PrimitiveCandidate,
	kind string,
	target float64,
) PrimitiveCandidate {
	if target <= 0 || !finite(target) {
		return PrimitiveCandidate{}
	}
	best := PrimitiveCandidate{}
	bestDistance := math.Inf(1)
	for _, primitive := range primitives {
		if primitive.Kind != kind {
			continue
		}
		value := seedPrimitiveValue(primitive)
		if value == nil || *value <= 0 || !finite(*value) {
			continue
		}
		distance := math.Abs(math.Log(*value / target))
		if distance < bestDistance ||
			(distance == bestDistance &&
				(best.Key == "" || primitive.Key < best.Key)) {
			best = primitive
			bestDistance = distance
		}
	}
	return best
}

// topologyControlledSwitchRelationshipSeeds constructs a bounded level-shifted
// switching relationship when behavior couples an external control to
// on/off-state observations. A reviewed supply establishes the decision
// reference and may also drive the switching device when its modeled gate-on
// voltage permits that simpler, supply-stable assignment. Both control
// polarities and all sufficient supply assignments are enumerated
// deterministically.
func topologyControlledSwitchRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	if !topologyControlledSwitchRequired(requirement) {
		return nil, Consumption{}, map[string][]string{}
	}
	decisionStages, requireDecisionFeedback := topologyDecisionObligation(requirement)
	if decisionStages > 1 {
		return nil, Consumption{}, map[string][]string{}
	}
	var comparator, nmos, pmos, resistor, capacitor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "comparator":
			comparator = primitive
		case "n_channel_mosfet":
			nmos = primitive
		case "p_channel_mosfet":
			pmos = primitive
		case "resistor":
			resistor = primitive
		case "capacitor":
			capacitor = primitive
		}
	}
	flyback := topologyFlybackDiodePrimitive(requirement, inventory)
	stableReference := topologyStableReferencePrimitive(requirement, inventory)
	levelShifter := selectCurrentRelationshipPrimitiveMatching(
		requirement, inventory, "npn_bjt", false, false,
		func(candidate PrimitiveCandidate) bool {
			return primitiveModelParameter(candidate, simmodel.PrimitiveBJTNPNV1, "forward_beta") >= 100
		},
	)
	if comparator.Key == "" || (nmos.Key == "" && pmos.Key == "") ||
		resistor.Key == "" || flyback.Key == "" {
		return nil, Consumption{}, map[string][]string{}
	}
	controls := topologyControlNodes(requirement, initial.graph)
	outputs := topologyNodesByRole(initial.graph, "output")
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(controls) == 0 || len(outputs) == 0 ||
		len(supplies) == 0 || len(references) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, control := range controls {
		for _, output := range outputs {
			highSide := topologyControlledSwitchHighSideOutput(requirement, initial.graph, output)
			mosfet := nmos
			modelID := simmodel.PrimitiveNMOSSwitchV1
			if highSide {
				mosfet = pmos
				modelID = simmodel.PrimitivePMOSSwitchV1
			}
			if rated := topologyRatedPowerPrimitive(requirement, inventory, mosfet.Kind); rated.Key != "" {
				mosfet = rated
			}
			if mosfet.Key == "" {
				rejections["relationship_gap"] = append(
					rejections["relationship_gap"],
					output+": controlled power transfer lacks a reviewed switch for the required source/sink orientation",
				)
				continue
			}
			minimumGateDrive := primitiveModelParameter(mosfet, modelID, "gate_on_voltage_v")
			loadSupply, loadSupplyFound := topologyControlledSwitchLoadSupply(requirement, initial.graph, output, supplies)
			if !loadSupplyFound {
				rejections["relationship_gap"] = append(
					rejections["relationship_gap"],
					output+": controlled inductive switching requires one unambiguous load supply covering the output voltage and current envelope",
				)
				continue
			}
			for _, driveSupply := range supplies {
				for _, referenceSupply := range supplies {
					driveVoltage, driveVoltageKnown := topologyNodeNominalVoltage(
						requirement,
						initial.graph,
						driveSupply,
					)
					referenceVoltage, referenceVoltageKnown := topologyNodeNominalVoltage(
						requirement,
						initial.graph,
						referenceSupply,
					)
					if minimumGateDrive > 0 && driveVoltageKnown &&
						driveVoltage < minimumGateDrive {
						continue
					}
					if driveSupply != referenceSupply &&
						driveVoltageKnown && referenceVoltageKnown &&
						driveVoltage <= referenceVoltage {
						continue
					}
					for _, reference := range references {
						if highSide && stableReference.Key != "" && levelShifter.Key != "" && capacitor.Key != "" {
							type highSideSupplyState struct {
								state  topologySearchState
								supply string
							}
							supplyStates := []highSideSupplyState{{state: initial, supply: loadSupply}}
							if regulated, found := topologyRegulatedLoadRail(
								requirement, initial.graph, output, loadSupply, inventory,
							); found && len(regulated.ballast) > 0 &&
								len(regulated.ballast) == len(regulated.ballastValueSI) {
								adapted, converterRail := addRelationshipInternalNode(
									initial, requirement, inventoryByKey, &consumption,
								)
								adapted, regulatedRail := addRelationshipInternalNode(
									adapted, requirement, inventoryByKey, &consumption,
								)
								ballastMiddle := ""
								if len(regulated.ballast) == 2 {
									adapted, ballastMiddle = addRelationshipInternalNode(
										adapted, requirement, inventoryByKey, &consumption,
									)
								}
								if converterRail != "" && regulatedRail != "" &&
									(len(regulated.ballast) == 1 || ballastMiddle != "") {
									adapted = addRelationshipPrimitive(
										adapted, requirement, inventoryByKey, regulated.converter,
										[]TerminalConnection{
											{Terminal: "VIN_PLUS", Node: loadSupply},
											{Terminal: "VIN_MINUS", Node: reference},
											{Terminal: "VOUT_PLUS", Node: converterRail},
											{Terminal: "VOUT_MINUS", Node: reference},
										}, &consumption,
									)
									if len(regulated.ballast) == 1 {
										adapted = addRelationshipPrimitiveAtValue(
											adapted, requirement, inventoryByKey, regulated.ballast[0],
											regulated.ballastValueSI[0], topologyTwoTerminalPlacement(converterRail, regulatedRail),
											&consumption,
										)
									} else {
										adapted = addRelationshipPrimitiveAtValue(
											adapted, requirement, inventoryByKey, regulated.ballast[0],
											regulated.ballastValueSI[0], topologyTwoTerminalPlacement(converterRail, ballastMiddle),
											&consumption,
										)
										adapted = addRelationshipPrimitiveAtValue(
											adapted, requirement, inventoryByKey, regulated.ballast[1],
											regulated.ballastValueSI[1], topologyTwoTerminalPlacement(ballastMiddle, regulatedRail),
											&consumption,
										)
									}
									supplyStates = append(supplyStates, highSideSupplyState{state: adapted, supply: regulatedRail})
								}
							}
							for _, supplyState := range supplyStates {
								candidateState, ok := topologyHighSideLevelShiftedSwitchState(
									requirement, inventoryByKey, supplyState.state,
									comparator, mosfet, levelShifter, stableReference, resistor, capacitor,
									control, output, driveSupply, supplyState.supply, reference,
									&consumption,
								)
								if !ok || internalNodeCount(candidateState.graph) > limits.MaxInternalNodes {
									continue
								}
								candidateState = addRelationshipPrimitive(
									candidateState, requirement, inventoryByKey, flyback,
									[]TerminalConnection{
										{Terminal: "ANODE", Node: reference},
										{Terminal: "CATHODE", Node: output},
									}, &consumption,
								)
								retainControlledSwitchRelationshipCandidate(
									candidateState, inventory, limits, policy,
									&consumption, retained, rejections,
								)
							}
						}
						for _, polarity := range []int{-1, 1} {
							if ctx.Err() != nil ||
								consumption.ExpandedStates >= policy.MaxExpandedStates ||
								consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
								break
							}
							consumption.ExpandedStates++
							state := initial
							var gateNode, thresholdNode, signalNode string
							state, gateNode = addRelationshipInternalNode(
								state, requirement, inventoryByKey, &consumption,
							)
							state, thresholdNode = addRelationshipInternalNode(
								state, requirement, inventoryByKey, &consumption,
							)
							signalNode = control
							if requireDecisionFeedback {
								state, signalNode = addRelationshipInternalNode(
									state, requirement, inventoryByKey, &consumption,
								)
							}
							if gateNode == "" || thresholdNode == "" || signalNode == "" ||
								internalNodeCount(state.graph) > limits.MaxInternalNodes {
								continue
							}
							inPlus, inMinus := thresholdNode, signalNode
							if polarity > 0 {
								inPlus, inMinus = signalNode, thresholdNode
							}
							switchSource := reference
							if highSide {
								switchSource = loadSupply
							}
							state = addRelationshipPrimitive(
								state,
								requirement,
								inventoryByKey,
								comparator,
								[]TerminalConnection{
									{Terminal: "IN_MINUS", Node: inMinus},
									{Terminal: "IN_PLUS", Node: inPlus},
									{Terminal: "OUT", Node: gateNode},
									{Terminal: "V_MINUS", Node: reference},
									{Terminal: "V_PLUS", Node: driveSupply},
								},
								&consumption,
							)
							state = addRelationshipPrimitive(
								state,
								requirement,
								inventoryByKey,
								mosfet,
								[]TerminalConnection{
									{Terminal: "DRAIN", Node: output},
									{Terminal: "GATE", Node: gateNode},
									{Terminal: "SOURCE", Node: switchSource},
								},
								&consumption,
							)
							gatePullupSupply := driveSupply
							if highSide {
								gatePullupSupply = loadSupply
							}
							edges := [][2]string{
								{referenceSupply, thresholdNode},
								{thresholdNode, reference},
								{gatePullupSupply, gateNode},
							}
							if !highSide {
								edges = append(edges, [2]string{gateNode, reference})
							}
							for _, edge := range edges {
								state = addRelationshipPrimitive(
									state,
									requirement,
									inventoryByKey,
									resistor,
									topologyTwoTerminalPlacement(edge[0], edge[1]),
									&consumption,
								)
							}
							if requireDecisionFeedback {
								feedbackNode := signalNode
								if polarity < 0 {
									feedbackNode = thresholdNode
								}
								for _, edge := range [][2]string{
									{signalNode, control},
									{feedbackNode, gateNode},
								} {
									state = addRelationshipPrimitive(
										state,
										requirement,
										inventoryByKey,
										resistor,
										topologyTwoTerminalPlacement(edge[0], edge[1]),
										&consumption,
									)
								}
							}
							type loadRailState struct {
								state topologySearchState
								rail  string
							}
							railStates := []loadRailState{{state: state, rail: loadSupply}}
							if regulated, found := topologyRegulatedLoadRail(
								requirement, state.graph, output, loadSupply, inventory,
							); found && len(regulated.ballast) > 0 &&
								len(regulated.ballast) == len(regulated.ballastValueSI) {
								adapted, converterRail := addRelationshipInternalNode(
									state, requirement, inventoryByKey, &consumption,
								)
								adapted, regulatedRail := addRelationshipInternalNode(
									adapted, requirement, inventoryByKey, &consumption,
								)
								ballastMiddle := ""
								if len(regulated.ballast) == 2 {
									adapted, ballastMiddle = addRelationshipInternalNode(
										adapted, requirement, inventoryByKey, &consumption,
									)
								}
								if converterRail != "" && regulatedRail != "" &&
									(len(regulated.ballast) == 1 || ballastMiddle != "") &&
									internalNodeCount(adapted.graph) <= limits.MaxInternalNodes {
									adapted = addRelationshipPrimitive(
										adapted,
										requirement,
										inventoryByKey,
										regulated.converter,
										[]TerminalConnection{
											{Terminal: "VIN_PLUS", Node: loadSupply},
											{Terminal: "VIN_MINUS", Node: reference},
											{Terminal: "VOUT_PLUS", Node: converterRail},
											{Terminal: "VOUT_MINUS", Node: reference},
										},
										&consumption,
									)
									if len(regulated.ballast) == 1 {
										adapted = addRelationshipPrimitiveAtValue(
											adapted, requirement, inventoryByKey, regulated.ballast[0],
											regulated.ballastValueSI[0], topologyTwoTerminalPlacement(converterRail, regulatedRail),
											&consumption,
										)
									} else {
										adapted = addRelationshipPrimitiveAtValue(
											adapted, requirement, inventoryByKey, regulated.ballast[0],
											regulated.ballastValueSI[0], topologyTwoTerminalPlacement(converterRail, ballastMiddle),
											&consumption,
										)
										adapted = addRelationshipPrimitiveAtValue(
											adapted, requirement, inventoryByKey, regulated.ballast[1],
											regulated.ballastValueSI[1], topologyTwoTerminalPlacement(ballastMiddle, regulatedRail),
											&consumption,
										)
									}
									railStates = append(railStates, loadRailState{state: adapted, rail: regulatedRail})
								}
							}
							for _, railState := range railStates {
								flybackAnode, flybackCathode := output, railState.rail
								if highSide {
									flybackAnode, flybackCathode = reference, output
								}
								candidateState := addRelationshipPrimitive(
									railState.state,
									requirement,
									inventoryByKey,
									flyback,
									[]TerminalConnection{
										{Terminal: "ANODE", Node: flybackAnode},
										{Terminal: "CATHODE", Node: flybackCathode},
									},
									&consumption,
								)
								retainControlledSwitchRelationshipCandidate(
									candidateState,
									inventory,
									limits,
									policy,
									&consumption,
									retained,
									rejections,
								)
							}
						}
					}
				}
			}
		}
	}
	if consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
		consumption.BudgetExhausted = true
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func topologyHighSideLevelShiftedSwitchState(
	requirement Requirement,
	inventoryByKey map[string]PrimitiveCandidate,
	initial topologySearchState,
	comparator PrimitiveCandidate,
	mosfet PrimitiveCandidate,
	levelShifter PrimitiveCandidate,
	stableReference PrimitiveCandidate,
	resistor PrimitiveCandidate,
	capacitor PrimitiveCandidate,
	control string,
	output string,
	driveSupply string,
	loadSupply string,
	reference string,
	consumption *Consumption,
) (topologySearchState, bool) {
	const internalNodeCount = 8
	state := initial
	nodes := make([]string, 0, internalNodeCount)
	for index := 0; index < internalNodeCount; index++ {
		var node string
		state, node = addRelationshipInternalNode(
			state, requirement, inventoryByKey, consumption,
		)
		if node == "" {
			return topologySearchState{}, false
		}
		nodes = append(nodes, node)
	}
	gateNode, signalNode, thresholdNode := nodes[0], nodes[1], nodes[2]
	referenceNode, decisionNode := nodes[3], nodes[4]
	feedbackMiddle, baseNode, biasMiddle := nodes[5], nodes[6], nodes[7]

	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, stableReference,
		[]TerminalConnection{
			{Terminal: "ANODE", Node: reference},
			{Terminal: "CATHODE", Node: referenceNode},
		}, consumption,
	)
	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, comparator,
		[]TerminalConnection{
			{Terminal: "IN_MINUS", Node: thresholdNode},
			{Terminal: "IN_PLUS", Node: signalNode},
			{Terminal: "OUT", Node: decisionNode},
			{Terminal: "V_MINUS", Node: reference},
			{Terminal: "V_PLUS", Node: driveSupply},
		}, consumption,
	)
	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, levelShifter,
		[]TerminalConnection{
			{Terminal: "BASE", Node: baseNode},
			{Terminal: "COLLECTOR", Node: gateNode},
			{Terminal: "EMITTER", Node: reference},
		}, consumption,
	)
	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, mosfet,
		[]TerminalConnection{
			{Terminal: "DRAIN", Node: output},
			{Terminal: "GATE", Node: gateNode},
			{Terminal: "SOURCE", Node: loadSupply},
		}, consumption,
	)
	for _, edge := range [][2]string{
		{driveSupply, referenceNode},
		{referenceNode, thresholdNode},
		{thresholdNode, reference},
		{control, signalNode},
		{signalNode, reference},
		{referenceNode, decisionNode},
		{decisionNode, baseNode},
		{decisionNode, feedbackMiddle},
		// Two independently valued parallel legs after the series feedback
		// branch let a sparse reviewed catalog realize the derived impedance.
		{feedbackMiddle, signalNode},
		{feedbackMiddle, signalNode},
		{referenceNode, biasMiddle},
		{biasMiddle, signalNode},
		{loadSupply, gateNode},
	} {
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, resistor,
			topologyTwoTerminalPlacement(edge[0], edge[1]), consumption,
		)
	}
	state = addRelationshipPrimitive(
		state, requirement, inventoryByKey, capacitor,
		topologyTwoTerminalPlacement(loadSupply, gateNode), consumption,
	)
	if state.hash == "" {
		return topologySearchState{}, false
	}
	return state, true
}

func retainControlledSwitchRelationshipCandidate(
	state topologySearchState,
	inventory PrimitiveInventory,
	limits GraphLimits,
	policy Policy,
	consumption *Consumption,
	retained map[string]TopologyCandidate,
	rejections map[string][]string,
) {
	if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
		consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
		return
	}
	if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
		for _, issue := range issues {
			rejections[string(issue.Code)] = append(
				rejections[string(issue.Code)],
				issue.Path+":"+issue.Message,
			)
		}
		return
	}
	if state.score.BehaviorGap != 0 {
		rejections["relationship_gap"] = append(
			rejections["relationship_gap"],
			fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap),
		)
		return
	}
	normalized, err := NormalizeGraph(state.graph)
	if err != nil {
		rejections["canonical_normalization_failed"] = append(
			rejections["canonical_normalization_failed"], err.Error(),
		)
		return
	}
	topologyHash, err := TopologyHash(normalized)
	if err != nil {
		rejections["canonical_topology_hash_failed"] = append(
			rejections["canonical_topology_hash_failed"], err.Error(),
		)
		return
	}
	consumption.CompleteGraphs++
	candidate := TopologyCandidate{
		Fingerprint:  state.hash,
		TopologyHash: topologyHash,
		Score:        state.score,
		Graph:        normalized,
		Operations:   cloneGraphOperations(state.operations),
	}
	if existing, found := retained[topologyHash]; !found ||
		compareTopologyCandidates(candidate, existing) < 0 {
		retained[topologyHash] = candidate
	}
}

// topologyControlledSwitchHighSideOutput derives switch orientation from the
// external electrical contract. A power port that sources energy must be
// disconnected and connected on its supply side; signal/current-return ports
// retain the low-side controlled path.
func topologyControlledSwitchHighSideOutput(
	requirement Requirement,
	graph CandidateGraph,
	output string,
) bool {
	semanticID := ""
	for _, node := range graph.Nodes {
		if node.ID == output && node.Scope == "external" && node.SemanticKind == "port" {
			semanticID = node.SemanticID
			break
		}
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID == semanticID {
			return port.Kind == "power" && port.Direction == "source"
		}
	}
	return false
}

func topologyControlledSwitchLoadSupply(
	requirement Requirement,
	graph CandidateGraph,
	output string,
	supplies []string,
) (string, bool) {
	outputSemanticID := ""
	for _, node := range graph.Nodes {
		if node.ID == output && node.SemanticKind == "port" {
			outputSemanticID = node.SemanticID
			break
		}
	}
	requiredVoltage, requiredCurrent := 0.0, 0.0
	for _, port := range requirement.Requirements.Ports {
		if port.ID != outputSemanticID {
			continue
		}
		if port.Electrical.MaxVoltageV != nil {
			requiredVoltage = math.Max(requiredVoltage, *port.Electrical.MaxVoltageV)
		}
		if port.Electrical.NominalVoltageV != nil {
			requiredVoltage = math.Max(requiredVoltage, *port.Electrical.NominalVoltageV)
		}
		if port.Electrical.MaxCurrentA != nil {
			requiredCurrent = *port.Electrical.MaxCurrentA
		}
		break
	}
	if requiredVoltage <= 0 || !finite(requiredVoltage) {
		requiredVoltage = 0
	}
	if requiredCurrent <= 0 || !finite(requiredCurrent) {
		requiredCurrent = 0
	}
	if requiredVoltage == 0 && requiredCurrent == 0 {
		return "", false
	}

	selected := ""
	selectedVoltageHeadroom := math.Inf(1)
	selectedCurrentHeadroom := math.Inf(1)
	ambiguous := false
	for _, supply := range supplies {
		voltageHeadroom := 0.0
		if requiredVoltage > 0 {
			_, maximumVoltage, found := topologyNodeVoltageRange(requirement, graph, supply)
			if !found || maximumVoltage < requiredVoltage || !finite(maximumVoltage) {
				continue
			}
			voltageHeadroom = maximumVoltage - requiredVoltage
		}
		domainID := ""
		for _, node := range graph.Nodes {
			if node.ID == supply {
				domainID = node.Domain
				break
			}
		}
		currentHeadroom := 0.0
		currentCovered := requiredCurrent <= 0
		for _, domain := range requirement.Requirements.Domains {
			if domain.ID == domainID && requiredCurrent > 0 && domain.MaxCurrentA != nil && finite(*domain.MaxCurrentA) {
				currentCovered = *domain.MaxCurrentA >= requiredCurrent
				currentHeadroom = *domain.MaxCurrentA - requiredCurrent
				break
			}
		}
		if !currentCovered {
			continue
		}
		if voltageHeadroom < selectedVoltageHeadroom ||
			(voltageHeadroom == selectedVoltageHeadroom && currentHeadroom < selectedCurrentHeadroom) {
			selected = supply
			selectedVoltageHeadroom = voltageHeadroom
			selectedCurrentHeadroom = currentHeadroom
			ambiguous = false
		} else if voltageHeadroom == selectedVoltageHeadroom &&
			currentHeadroom == selectedCurrentHeadroom && supply != selected {
			ambiguous = true
		}
	}
	return selected, selected != "" && !ambiguous
}

func topologyControlledSwitchRequired(requirement Requirement) bool {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "off_state_current", "on_state_voltage", "propagation_delay", "startup_current":
			if assertion.Excitation != nil &&
				assertion.Excitation.Kind == "port" &&
				(requirementPortIsControl(requirement, assertion.Excitation.ID) ||
					requirementPortDrivesDecision(requirement, assertion.Excitation.ID)) {
				return true
			}
			for _, operatingCaseID := range assertion.OperatingCases {
				for _, operatingCase := range requirement.Requirements.OperatingCases {
					if operatingCase.ID != operatingCaseID {
						continue
					}
					for _, condition := range operatingCase.Conditions {
						if condition.Axis == "input_voltage" &&
							(requirementPortIsControl(requirement, condition.Target) ||
								requirementPortDrivesDecision(requirement, condition.Target)) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// topologyHighSideTransconductanceRelationshipSeeds constructs a high-side controlled
// current path from only the relationships implied by a voltage-to-current
// transfer: a series current-sense impedance, a differential observation, a
// feedback controller, a bounded pass-device bias path, and a thermally
// reviewed analog pass device.
func topologyHighSideTransconductanceRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	relationships := regulatedCurrentRelationships(requirement)
	if len(relationships) == 0 {
		return topologyUnprotectedTransconductanceRelationshipSeeds(
			ctx, requirement, inventory, representatives, inventoryByKey, limits, policy, initial,
		)
	}
	requireTransconductance := len(relationships) != 0
	requireProtectedControl := false
	for _, relationship := range relationships {
		requireProtectedControl = requireProtectedControl ||
			(relationship.activation != "" && relationship.fault != "")
	}
	requireThermal := false
	requireSOA := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		requireThermal = requireThermal ||
			assertion.Metric == "junction_temperature"
		requireSOA = requireSOA || assertion.Metric == "soa_margin"
	}
	if !requireTransconductance {
		return nil, Consumption{}, map[string][]string{}
	}
	var resistor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "resistor":
			resistor = primitive
		}
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	passCandidates := []PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "pnp_bjt" ||
			(requireThermal && !primitiveHasThermalEvidence(primitive)) ||
			(requireSOA && !primitiveHasSOAEvidence(primitive)) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		passCandidates = append(passCandidates, primitive)
	}
	slices.SortFunc(passCandidates, func(left, right PrimitiveCandidate) int {
		return compareRepresentativePrimitives(left, right, requiredAnalyses)
	})
	var passDevice PrimitiveCandidate
	if len(passCandidates) != 0 {
		passDevice = passCandidates[0]
	}
	passDeviceCount := 1
	if requireThermal {
		passDeviceCount = 2
	}
	senseAmplifier := topologyCurrentSenseAmplifierPrimitive(requirement, inventory)
	controller := topologyHighSideCurrentControllerPrimitive(
		requirement, inventory, passDevice, passDeviceCount,
	)
	powerSwitch := selectSupplyDrivenCurrentRelationshipSwitchPrimitive(
		requirement, inventory, "p_channel_mosfet", false,
	)
	controlDevice := selectCurrentRelationshipPrimitive(
		requirement, inventory, "npn_bjt", false, false,
	)
	bufferedDriveResistor := topologyPrimitiveClosestValue(
		inventory.Primitives, "resistor", 10_000,
	)
	transconductance := requirementTransconductance(requirement)
	if transconductance <= 0 || !finite(transconductance) {
		return nil, Consumption{}, map[string][]string{}
	}
	senseValues, senseValuesFound := currentSenseDifferentialComposition(
		ctx, requirement, inventory, 1/transconductance,
	)
	if senseAmplifier.Key == "" || controller.Key == "" || passDevice.Key == "" || resistor.Key == "" ||
		!senseValuesFound || (requireProtectedControl && (powerSwitch.Key == "" || controlDevice.Key == "")) {
		return nil, Consumption{}, map[string][]string{}
	}
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, relationship := range relationships {
		protectedControl := relationship.activation != "" && relationship.fault != ""
		if relationship.direction != "source" ||
			((relationship.activation == "") != (relationship.fault == "")) {
			continue
		}
		input := externalRelationshipNode(initial.graph, relationship.input)
		output := externalRelationshipNode(initial.graph, relationship.output)
		activation, fault := "", ""
		if protectedControl {
			activation = externalRelationshipNode(initial.graph, relationship.activation)
			fault = externalRelationshipNode(initial.graph, relationship.fault)
		}
		if input == "" || output == "" ||
			(protectedControl && (activation == "" || fault == "")) {
			continue
		}
		for _, supply := range supplies {
			for _, reference := range references {
				driveModes := []bool{false}
				if controlDevice.Key != "" && bufferedDriveResistor.Key != "" {
					driveModes = append(driveModes, true)
				}
				for _, bufferedDrive := range driveModes {
					if ctx.Err() != nil ||
						consumption.ExpandedStates >= policy.MaxExpandedStates ||
						consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
						break
					}
					consumption.ExpandedStates++
					state := initial
					state.graph = CloneGraph(initial.graph)
					state.operations = cloneGraphOperations(initial.operations)
					nextNode := func() string {
						var node string
						state, node = addRelationshipInternalNode(
							state, requirement, inventoryByKey, &consumption,
						)
						return node
					}
					switchedSupply := supply
					if protectedControl {
						switchGate := nextNode()
						if switchGate == "" {
							continue
						}
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
							topologyTwoTerminalPlacement(supply, switchGate), &consumption)
						switchedSupply = nextNode()
						if switchedSupply == "" {
							continue
						}
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, powerSwitch, []TerminalConnection{
							{Terminal: "GATE", Node: switchGate},
							{Terminal: "DRAIN", Node: switchedSupply},
							{Terminal: "SOURCE", Node: supply},
						}, &consumption)
						permitBase := nextNode()
						if permitBase == "" {
							continue
						}
						for _, edge := range [][2]string{{activation, permitBase}, {permitBase, reference}} {
							state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
								topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
						}
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, controlDevice, []TerminalConnection{
							{Terminal: "BASE", Node: permitBase},
							{Terminal: "COLLECTOR", Node: switchGate},
							{Terminal: "EMITTER", Node: reference},
						}, &consumption)
						faultBase := nextNode()
						if faultBase == "" {
							continue
						}
						for _, edge := range [][2]string{{fault, faultBase}, {faultBase, reference}} {
							state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
								topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
						}
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, controlDevice, []TerminalConnection{
							{Terminal: "BASE", Node: faultBase},
							{Terminal: "COLLECTOR", Node: permitBase},
							{Terminal: "EMITTER", Node: reference},
						}, &consumption)
					}
					passBase := nextNode()
					if passBase == "" {
						continue
					}
					emitterNodes := make([]string, 0, passDeviceCount)
					for index := 0; index < passDeviceCount; index++ {
						emitter := nextNode()
						if emitter == "" {
							break
						}
						emitterNodes = append(emitterNodes, emitter)
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
							topologyTwoTerminalPlacement(switchedSupply, emitter), &consumption)
					}
					if len(emitterNodes) != passDeviceCount {
						continue
					}
					passCollector := nextNode()
					if passCollector == "" {
						continue
					}
					for _, emitter := range emitterNodes {
						state = addRelationshipPrimitive(
							state,
							requirement,
							inventoryByKey,
							passDevice,
							[]TerminalConnection{
								{Terminal: "BASE", Node: passBase},
								{Terminal: "COLLECTOR", Node: passCollector},
								{Terminal: "EMITTER", Node: emitter},
							},
							&consumption,
						)
					}
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, senseValues.shunt,
						topologyTwoTerminalPlacement(passCollector, output), &consumption)
					senseMinus := nextNode()
					if senseMinus == "" {
						continue
					}
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, senseValues.input,
						topologyTwoTerminalPlacement(output, senseMinus), &consumption)
					sensePlus := nextNode()
					if sensePlus == "" {
						continue
					}
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, senseValues.input,
						topologyTwoTerminalPlacement(passCollector, sensePlus), &consumption)
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, senseValues.feedback,
						topologyTwoTerminalPlacement(sensePlus, reference), &consumption)
					senseOutput := nextNode()
					if senseOutput == "" {
						continue
					}
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, senseValues.feedback,
						topologyTwoTerminalPlacement(senseOutput, senseMinus), &consumption)
					controlOutput := nextNode()
					if controlOutput == "" {
						continue
					}
					controllerMinus, controllerPlus := input, senseOutput
					if bufferedDrive {
						driverBase := nextNode()
						if driverBase == "" {
							continue
						}
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, bufferedDriveResistor,
							topologyTwoTerminalPlacement(controlOutput, driverBase), &consumption)
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, bufferedDriveResistor,
							topologyTwoTerminalPlacement(switchedSupply, passBase), &consumption)
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, controlDevice, []TerminalConnection{
							{Terminal: "BASE", Node: driverBase},
							{Terminal: "COLLECTOR", Node: passBase},
							{Terminal: "EMITTER", Node: reference},
						}, &consumption)
						controllerMinus, controllerPlus = senseOutput, input
					} else {
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
							topologyTwoTerminalPlacement(controlOutput, passBase), &consumption)
					}
					state = addRelationshipPrimitive(
						state,
						requirement,
						inventoryByKey,
						senseAmplifier,
						[]TerminalConnection{
							{Terminal: "IN_MINUS", Node: senseMinus},
							{Terminal: "IN_PLUS", Node: sensePlus},
							{Terminal: "OUT", Node: senseOutput},
							{Terminal: "V_MINUS", Node: reference},
							{Terminal: "V_PLUS", Node: supply},
						},
						&consumption,
					)
					state = addRelationshipPrimitive(
						state,
						requirement,
						inventoryByKey,
						controller,
						[]TerminalConnection{
							{Terminal: "IN_MINUS", Node: controllerMinus},
							{Terminal: "IN_PLUS", Node: controllerPlus},
							{Terminal: "OUT", Node: controlOutput},
							{Terminal: "V_MINUS", Node: reference},
							{Terminal: "V_PLUS", Node: supply},
						},
						&consumption,
					)
					if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
						consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
						continue
					}
					if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
						for _, issue := range issues {
							rejections[string(issue.Code)] = append(
								rejections[string(issue.Code)],
								issue.Path+":"+issue.Message,
							)
						}
						continue
					}
					if state.score.BehaviorGap != 0 {
						rejections["relationship_gap"] = append(
							rejections["relationship_gap"],
							fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap),
						)
						continue
					}
					normalized, err := NormalizeGraph(state.graph)
					if err != nil {
						rejections["canonical_normalization_failed"] = append(
							rejections["canonical_normalization_failed"], err.Error(),
						)
						continue
					}
					topologyHash, err := TopologyHash(normalized)
					if err != nil {
						rejections["canonical_topology_hash_failed"] = append(
							rejections["canonical_topology_hash_failed"], err.Error(),
						)
						continue
					}
					consumption.CompleteGraphs++
					candidate := TopologyCandidate{
						Fingerprint:  state.hash,
						TopologyHash: topologyHash,
						Score:        state.score,
						Graph:        normalized,
						Operations:   cloneGraphOperations(state.operations),
					}
					if existing, found := retained[topologyHash]; !found ||
						compareTopologyCandidates(candidate, existing) < 0 {
						retained[topologyHash] = candidate
					}
				}
			}
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func primitiveHasThermalEvidence(primitive PrimitiveCandidate) bool {
	for _, model := range primitive.Models {
		if model.ThermalModel != nil || namedThermalParametersComplete(model.Parameters) {
			return true
		}
	}
	return false
}

func topologyNodeNominalVoltage(
	requirement Requirement,
	graph CandidateGraph,
	nodeID string,
) (float64, bool) {
	domainID := ""
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			domainID = node.Domain
			break
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID != domainID {
			continue
		}
		if domain.NominalVoltageV != nil {
			return *domain.NominalVoltageV, true
		}
		if domain.MinVoltageV != nil && domain.MaxVoltageV != nil {
			return (*domain.MinVoltageV + *domain.MaxVoltageV) / 2, true
		}
		if domain.MaxVoltageV != nil {
			return *domain.MaxVoltageV, true
		}
		if domain.MinVoltageV != nil {
			return *domain.MinVoltageV, true
		}
	}
	return 0, false
}

func topologyNodeVoltageRange(
	requirement Requirement,
	graph CandidateGraph,
	nodeID string,
) (float64, float64, bool) {
	var node GraphNode
	found := false
	for _, candidate := range graph.Nodes {
		if candidate.ID == nodeID {
			node = candidate
			found = true
			break
		}
	}
	if !found {
		return 0, 0, false
	}
	minimum, maximum := 0.0, 0.0
	haveValue := false
	include := func(value float64) {
		if !haveValue {
			minimum, maximum, haveValue = value, value, true
			return
		}
		minimum = math.Min(minimum, value)
		maximum = math.Max(maximum, value)
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID != node.Domain {
			continue
		}
		if domain.MinVoltageV != nil {
			include(*domain.MinVoltageV)
		}
		if domain.NominalVoltageV != nil {
			include(*domain.NominalVoltageV)
		}
		if domain.MaxVoltageV != nil {
			include(*domain.MaxVoltageV)
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if (condition.Axis != "supply_voltage" && condition.Axis != "input_voltage") ||
				(condition.Target != node.Domain && condition.Target != node.SemanticID) {
				continue
			}
			include(condition.Min)
			include(condition.Max)
		}
	}
	return minimum, maximum, haveValue
}

func addRelationshipInternalNode(
	state topologySearchState,
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	consumption *Consumption,
) (topologySearchState, string) {
	graph := AddInternalNode(state.graph, "internal")
	hash, err := GraphHash(graph)
	if err != nil {
		return state, ""
	}
	node := graph.Nodes[len(graph.Nodes)-1].ID
	operations := cloneGraphOperations(state.operations)
	operations = append(operations, GraphOperation{
		Number:     len(operations) + 1,
		Kind:       "add_internal_node",
		Node:       node,
		BeforeHash: state.hash,
		AfterHash:  hash,
	})
	consumption.GeneratedGraphs++
	return topologySearchState{
		graph:      graph,
		hash:       hash,
		score:      scoreTopologyGraph(requirement, graph, inventory, hash),
		operations: operations,
	}, node
}

func addRelationshipPrimitive(
	state topologySearchState,
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	primitive PrimitiveCandidate,
	placement []TerminalConnection,
	consumption *Consumption,
) topologySearchState {
	return addRelationshipPrimitiveWithValue(
		state,
		requirement,
		inventory,
		primitive,
		seedPrimitiveValue(primitive),
		placement,
		consumption,
	)
}

func addRelationshipPrimitiveAtValue(
	state topologySearchState,
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	primitive PrimitiveCandidate,
	value float64,
	placement []TerminalConnection,
	consumption *Consumption,
) topologySearchState {
	if primitive.ValueDomain == nil ||
		!valueWithinPrimitiveDomain(value, *primitive.ValueDomain) {
		return state
	}
	return addRelationshipPrimitiveWithValue(
		state,
		requirement,
		inventory,
		primitive,
		&value,
		placement,
		consumption,
	)
}

func addRelationshipPrimitiveWithValue(
	state topologySearchState,
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	primitive PrimitiveCandidate,
	value *float64,
	placement []TerminalConnection,
	consumption *Consumption,
) topologySearchState {
	graph := AddPrimitive(
		state.graph,
		primitive,
		value,
		placement,
	)
	hash, err := GraphHash(graph)
	if err != nil {
		return state
	}
	operations := cloneGraphOperations(state.operations)
	operations = append(operations, GraphOperation{
		Number:        len(operations) + 1,
		Kind:          "add_primitive",
		PrimitiveKey:  primitive.Key,
		PrimitiveKind: primitive.Kind,
		Connections:   append([]TerminalConnection(nil), placement...),
		ValueSI:       cloneInventoryFloat(value),
		BeforeHash:    state.hash,
		AfterHash:     hash,
	})
	consumption.GeneratedGraphs++
	return topologySearchState{
		graph:      graph,
		hash:       hash,
		score:      scoreTopologyGraph(requirement, graph, inventory, hash),
		operations: operations,
	}
}

func topologyTwoTerminalPlacement(left, right string) []TerminalConnection {
	if left > right {
		left, right = right, left
	}
	return []TerminalConnection{
		{Terminal: "A", Node: left},
		{Terminal: "B", Node: right},
	}
}

func topologyObligationSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	const (
		maximumKindSequences = 128
		obligationBeamWidth  = 192
	)
	sequences := topologyObligationKindSequences(
		requirement, representatives, maximumKindSequences,
	)
	if len(sequences) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	dominantTopology := map[string]topologySearchState{initial.topology: initial}
	baseStates := []topologySearchState{initial}
	if limits.MaxInternalNodes > 0 && policy.MaxGeneratedGraphs > 0 {
		base := initial
		for index := 0; index < min(limits.MaxInternalNodes, 3); index++ {
			graph := AddInternalNode(base.graph, "internal")
			hash, err := GraphHash(graph)
			if err != nil {
				break
			}
			operation := GraphOperation{
				Number: len(base.operations) + 1, Kind: "add_internal_node",
				Node:       graph.Nodes[len(graph.Nodes)-1].ID,
				BeforeHash: base.hash, AfterHash: hash,
			}
			topologyHash, _ := TopologyHash(graph)
			base = topologySearchState{
				graph: graph, hash: hash, topology: topologyHash,
				score:      scoreTopologyGraph(requirement, graph, inventoryByKey, hash),
				operations: append(cloneGraphOperations(base.operations), operation),
			}
			baseStates = append(baseStates, base)
			consumption.GeneratedGraphs++
		}
	}
	for _, sequence := range sequences {
		if ctx.Err() != nil ||
			consumption.ExpandedStates >= policy.MaxExpandedStates ||
			consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			break
		}
		beam := append([]topologySearchState(nil), baseStates...)
		for _, primitive := range sequence {
			next := []topologySearchState{}
			for _, state := range beam {
				if consumption.ExpandedStates >= policy.MaxExpandedStates ||
					consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
					break
				}
				consumption.ExpandedStates++
				for _, placement := range primitivePlacements(
					requirement, state.graph, primitive, maxPrimitivePlacementsPerKind,
				) {
					if consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
						break
					}
					graph := AddPrimitive(
						state.graph, primitive, seedPrimitiveValue(primitive), placement,
					)
					consumption.GeneratedGraphs++
					score := scoreTopologyGraph(requirement, graph, inventoryByKey, "")
					operations := cloneGraphOperations(state.operations)
					operations = append(operations, GraphOperation{
						Number: len(operations) + 1, Kind: "add_primitive",
						PrimitiveKey: primitive.Key, PrimitiveKind: primitive.Kind,
						Connections: append([]TerminalConnection(nil), placement...),
						ValueSI:     seedPrimitiveValue(primitive), BeforeHash: state.hash,
					})
					next = append(next, topologySearchState{
						graph: graph, score: score, operations: operations,
					})
				}
			}
			slices.SortFunc(next, func(left, right topologySearchState) int {
				if comparison := compareTopologyScores(left.score, right.score); comparison != 0 {
					return comparison
				}
				return compareGraphOperations(
					left.operations[len(left.operations)-1],
					right.operations[len(right.operations)-1],
				)
			})
			if len(next) > obligationBeamWidth*4 {
				next = next[:obligationBeamWidth*4]
			}
			candidateBeam := make([]topologySearchState, 0, obligationBeamWidth*4)
			seen := map[string]bool{}
			for _, state := range next {
				hash, err := GraphHash(state.graph)
				if err != nil {
					rejections["canonical_hash_failed"] = append(
						rejections["canonical_hash_failed"], err.Error(),
					)
					continue
				}
				if seen[hash] {
					rejections["dominated_topology"] = append(
						rejections["dominated_topology"], hash+"->"+hash,
					)
					continue
				}
				seen[hash] = true
				if issues := ValidatePartialGraph(state.graph, inventory, limits); len(issues) != 0 {
					continue
				}
				state.hash = hash
				state.score.Fingerprint = hash
				state.topology, _ = TopologyHash(state.graph)
				if dominant, found := dominantTopology[state.topology]; found &&
					compareTopologyScores(dominant.score, state.score) <= 0 {
					rejections["dominated_topology"] = append(
						rejections["dominated_topology"], hash+"->"+dominant.hash,
					)
					continue
				}
				dominantTopology[state.topology] = state
				state.operations[len(state.operations)-1].AfterHash = hash
				candidateBeam = append(candidateBeam, state)
				if len(candidateBeam) >= obligationBeamWidth*4 {
					break
				}
			}
			beam = selectDiverseTopologyStates(candidateBeam, obligationBeamWidth)
			consumption.MaximumFrontier = max(consumption.MaximumFrontier, len(beam))
			if len(beam) == 0 {
				break
			}
		}
		for _, state := range beam {
			if state.score.BehaviorGap != 0 ||
				len(ValidateCompleteGraph(state.graph, inventory, limits)) != 0 {
				continue
			}
			normalized, err := NormalizeGraph(state.graph)
			if err != nil {
				continue
			}
			consumption.CompleteGraphs++
			candidate := TopologyCandidate{
				Fingerprint: state.hash, TopologyHash: state.topology,
				Score: state.score, Graph: normalized,
				Operations: cloneGraphOperations(state.operations),
			}
			if existing, found := retained[state.topology]; !found ||
				compareTopologyCandidates(candidate, existing) < 0 {
				retained[state.topology] = candidate
			}
		}
		if len(retained) >= policy.MaxRetainedCandidates {
			break
		}
	}
	if consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
		consumption.BudgetExhausted = true
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func topologyPrimitiveRequiresPullup(primitive PrimitiveCandidate) bool {
	for _, model := range primitive.Models {
		if model.ModelID == simmodel.PrimitiveComparatorOpenCollectorV1 {
			return true
		}
	}
	return false
}

func topologyObligationKindSequences(
	requirement Requirement,
	representatives []PrimitiveCandidate,
	maximum int,
) [][]PrimitiveCandidate {
	if maximum <= 0 {
		return nil
	}
	needAnalog := false
	needDecision := 0
	needSwitch := false
	needResistance := 0
	needReactance := false
	needActive := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "cutoff_frequency":
			needResistance = max(needResistance, 1)
			needReactance = true
		case "bandwidth":
			needReactance = true
		case "phase_margin", "gain_margin", "voltage_gain", "voltage_gain_at_frequency",
			"output_noise_rms", "thd", "total_harmonic_distortion":
			needAnalog = true
			if assertion.Metric == "voltage_gain" &&
				assertion.Min != nil && *assertion.Min > 1 {
				needResistance = max(needResistance, 2)
			}
		case "transimpedance":
			needAnalog = true
			needResistance = max(needResistance, 1)
		case "hysteresis":
			needDecision = max(needDecision, 1)
			// A bounded hysteretic threshold requires a rail-referenced
			// decision level, input isolation, and positive feedback. Four
			// primitive two-pin impedances cover either comparator polarity
			// without assuming the external source has nonzero impedance.
			needResistance = max(needResistance, 4)
		case "rising_threshold", "falling_threshold", "threshold_voltage", "threshold_current":
			needDecision = max(needDecision, 1)
			// Thresholds must be established by a reference network rather
			// than an unconstrained active input tied directly to a rail.
			needResistance = max(needResistance, 2)
		case "lower_threshold", "upper_threshold":
			needDecision = max(needDecision, 2)
			// Two independently bounded decision levels require two
			// independently tunable primitive dividers.
			needResistance = max(needResistance, 4)
		case "output_current", "transconductance":
			needActive = true
			needResistance = max(needResistance, 1)
		case "line_regulation", "load_regulation":
			needActive = true
			needResistance = max(needResistance, 2)
		case
			"off_state_current", "on_state_voltage", "propagation_delay", "startup_current",
			"junction_temperature", "soa_margin":
			needActive = true
		}
		if assertion.Excitation != nil && assertion.Excitation.Kind == "port" &&
			requirementPortIsControl(requirement, assertion.Excitation.ID) {
			needSwitch = true
			// A discrete controlled input needs a deterministic inactive
			// state even when its external driver is high impedance.
			needResistance = max(needResistance, 1)
		}
	}
	byKind := map[string]PrimitiveCandidate{}
	for _, primitive := range representatives {
		byKind[primitive.Kind] = primitive
	}
	groups := [][]PrimitiveCandidate{}
	appendChoices := func(kinds ...string) {
		choices := []PrimitiveCandidate{}
		for _, kind := range kinds {
			if primitive, found := byKind[kind]; found {
				choices = append(choices, primitive)
			}
		}
		if len(choices) != 0 {
			groups = append(groups, choices)
		}
	}
	if needAnalog {
		appendChoices("opamp", "n_channel_mosfet", "npn_bjt", "p_channel_mosfet", "pnp_bjt")
	}
	for index := 0; index < needDecision; index++ {
		appendChoices("comparator", "opamp")
	}
	if needSwitch {
		appendChoices("n_channel_mosfet", "npn_bjt", "p_channel_mosfet", "pnp_bjt", "comparator")
	}
	if needActive && !needAnalog && needDecision == 0 && !needSwitch {
		appendChoices(
			"opamp", "comparator", "n_channel_mosfet", "npn_bjt",
			"p_channel_mosfet", "pnp_bjt",
		)
	}
	for index := 0; index < needResistance; index++ {
		appendChoices("resistor")
	}
	if needReactance {
		appendChoices("capacitor", "inductor")
	}
	if len(groups) == 0 {
		return nil
	}
	combinations := [][]PrimitiveCandidate{{}}
	for _, group := range groups {
		next := [][]PrimitiveCandidate{}
		for _, prefix := range combinations {
			for _, primitive := range group {
				next = append(next, append(
					append([]PrimitiveCandidate(nil), prefix...), primitive,
				))
			}
		}
		combinations = next
	}
	sequences := [][]PrimitiveCandidate{}
	seen := map[string]bool{}
	var permute func([]PrimitiveCandidate, int)
	permute = func(values []PrimitiveCandidate, index int) {
		if len(sequences) >= maximum {
			return
		}
		if index == len(values) {
			kinds := make([]string, len(values))
			for position, primitive := range values {
				kinds[position] = primitive.Kind
			}
			key := fmt.Sprint(kinds)
			if !seen[key] {
				seen[key] = true
				sequences = append(sequences, append([]PrimitiveCandidate(nil), values...))
			}
			return
		}
		for candidate := index; candidate < len(values); candidate++ {
			values[index], values[candidate] = values[candidate], values[index]
			permute(values, index+1)
			values[index], values[candidate] = values[candidate], values[index]
		}
	}
	for _, combination := range combinations {
		permute(combination, 0)
		if len(sequences) >= maximum {
			break
		}
	}
	return sequences
}

func behaviorGuidedTopologySeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	const maximumDepth = 8
	beamWidth := max(16, policy.MaxRetainedCandidates*2)
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	beam := []topologySearchState{initial}
	seen := map[string]bool{initial.hash: true}
	dominantTopology := map[string]topologySearchState{initial.topology: initial}

	for depth := 0; depth < maximumDepth && len(beam) != 0; depth++ {
		if ctx.Err() != nil ||
			consumption.ExpandedStates >= policy.MaxExpandedStates ||
			consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			break
		}
		next := []topologySearchState{}
		for _, state := range beam {
			if consumption.ExpandedStates >= policy.MaxExpandedStates ||
				consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
				break
			}
			consumption.ExpandedStates++
			remaining := policy.MaxGeneratedGraphs - consumption.GeneratedGraphs
			expansions, generated := expandTopologyState(
				state,
				representatives,
				limits,
				requirement,
				inventory,
				inventoryByKey,
				remaining,
			)
			consumption.GeneratedGraphs += generated
			next = append(next, expansions...)
		}
		slices.SortFunc(next, func(left, right topologySearchState) int {
			if comparison := compareTopologyScores(left.score, right.score); comparison != 0 {
				return comparison
			}
			return compareGraphOperations(
				left.operations[len(left.operations)-1],
				right.operations[len(right.operations)-1],
			)
		})
		// Hash only a bounded oversample. Canonical hashing is part of the
		// explicit work budget, while the oversample leaves room for invalid
		// and canonically duplicate transitions.
		if len(next) > beamWidth*4 {
			next = next[:beamWidth*4]
		}
		candidateBeam := make([]topologySearchState, 0, beamWidth*4)
		for _, state := range next {
			hash, err := GraphHash(state.graph)
			if err != nil {
				rejections["canonical_hash_failed"] = append(rejections["canonical_hash_failed"], err.Error())
				continue
			}
			if seen[hash] {
				rejections["dominated_topology"] = append(rejections["dominated_topology"], hash+"->"+hash)
				continue
			}
			seen[hash] = true
			if issues := ValidatePartialGraph(state.graph, inventory, limits); len(issues) != 0 {
				for _, issue := range issues {
					rejections[string(issue.Code)] = append(
						rejections[string(issue.Code)], issue.Path+":"+issue.Message,
					)
				}
				continue
			}
			state.hash = hash
			state.score.Fingerprint = hash
			topologyHash, err := TopologyHash(state.graph)
			if err != nil {
				rejections["canonical_topology_hash_failed"] = append(
					rejections["canonical_topology_hash_failed"], err.Error(),
				)
				continue
			}
			state.topology = topologyHash
			if dominant, found := dominantTopology[topologyHash]; found &&
				compareTopologyScores(dominant.score, state.score) <= 0 {
				rejections["dominated_topology"] = append(
					rejections["dominated_topology"], hash+"->"+dominant.hash,
				)
				continue
			}
			dominantTopology[topologyHash] = state
			if len(state.operations) != 0 {
				state.operations[len(state.operations)-1].AfterHash = hash
			}
			if len(ValidateCompleteGraph(state.graph, inventory, limits)) == 0 &&
				len(state.graph.Instances) != 0 &&
				state.score.BehaviorGap == 0 {
				normalized, normalizeErr := NormalizeGraph(state.graph)
				if normalizeErr == nil {
					consumption.CompleteGraphs++
					candidate := TopologyCandidate{
						Fingerprint: hash, TopologyHash: topologyHash,
						Score: state.score, Graph: normalized,
						Operations: cloneGraphOperations(state.operations),
					}
					if existing, found := retained[topologyHash]; !found ||
						compareTopologyCandidates(candidate, existing) < 0 {
						retained[topologyHash] = candidate
					}
				}
			}
			candidateBeam = append(candidateBeam, state)
			if len(candidateBeam) >= beamWidth*4 {
				break
			}
		}
		beam = selectDiverseTopologyStates(candidateBeam, beamWidth)
		consumption.MaximumFrontier = max(consumption.MaximumFrontier, len(beam))
		if len(retained) >= policy.MaxRetainedCandidates {
			break
		}
	}
	if consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
		consumption.BudgetExhausted = true
	}
	if len(retained) == 0 {
		for _, state := range beam {
			rejections["guided_frontier"] = append(
				rejections["guided_frontier"],
				fmt.Sprintf(
					"%s:gap=%d:redundant_active=%d:primitives=%d:internal=%d",
					state.hash,
					state.score.BehaviorGap,
					state.score.RedundantActive,
					state.score.PrimitiveCount,
					state.score.InternalNodeCount,
				),
			)
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func selectDiverseTopologyCandidates(
	candidates []TopologyCandidate,
	limit int,
) []TopologyCandidate {
	if limit <= 0 || len(candidates) <= limit {
		return append([]TopologyCandidate(nil), candidates...)
	}
	features := make([]map[string]int, len(candidates))
	for index, candidate := range candidates {
		features[index] = topologyCandidateFeatures(candidate.Graph)
	}
	selected := make([]int, 0, limit)
	selectedSet := map[int]bool{}
	selected = append(selected, 0)
	selectedSet[0] = true
	for len(selected) < limit {
		bestIndex := -1
		bestDistance := -1
		for candidateIndex := range candidates {
			if selectedSet[candidateIndex] {
				continue
			}
			minimumDistance := int(^uint(0) >> 1)
			for _, selectedIndex := range selected {
				minimumDistance = min(
					minimumDistance,
					topologyFeatureDistance(
						features[candidateIndex],
						features[selectedIndex],
					),
				)
			}
			if minimumDistance > bestDistance ||
				(minimumDistance == bestDistance &&
					(bestIndex < 0 ||
						compareTopologyCandidates(
							candidates[candidateIndex],
							candidates[bestIndex],
						) < 0)) {
				bestIndex = candidateIndex
				bestDistance = minimumDistance
			}
		}
		if bestIndex < 0 {
			break
		}
		selected = append(selected, bestIndex)
		selectedSet[bestIndex] = true
	}
	result := make([]TopologyCandidate, 0, len(selected))
	for _, index := range selected {
		result = append(result, candidates[index])
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result
}

func selectDiverseTopologyStates(
	states []topologySearchState,
	limit int,
) []topologySearchState {
	if limit <= 0 || len(states) <= limit {
		return append([]topologySearchState(nil), states...)
	}
	features := make([]map[string]int, len(states))
	for index, state := range states {
		features[index] = topologyCandidateFeatures(state.graph)
	}
	selected := make([]int, 0, limit)
	selectedSet := map[int]bool{}
	selected = append(selected, 0)
	selectedSet[0] = true
	for len(selected) < limit {
		bestIndex := -1
		bestDistance := -1
		for stateIndex := range states {
			if selectedSet[stateIndex] {
				continue
			}
			minimumDistance := int(^uint(0) >> 1)
			for _, selectedIndex := range selected {
				minimumDistance = min(
					minimumDistance,
					topologyFeatureDistance(
						features[stateIndex],
						features[selectedIndex],
					),
				)
			}
			if minimumDistance > bestDistance ||
				(minimumDistance == bestDistance &&
					(bestIndex < 0 ||
						compareTopologyScores(
							states[stateIndex].score,
							states[bestIndex].score,
						) < 0)) {
				bestIndex = stateIndex
				bestDistance = minimumDistance
			}
		}
		if bestIndex < 0 {
			break
		}
		selected = append(selected, bestIndex)
		selectedSet[bestIndex] = true
	}
	result := make([]topologySearchState, 0, len(selected))
	for _, index := range selected {
		result = append(result, states[index])
	}
	slices.SortFunc(result, func(left, right topologySearchState) int {
		return compareTopologyScores(left.score, right.score)
	})
	return result
}

func topologyCandidateFeatures(graph CandidateGraph) map[string]int {
	nodes := map[string]GraphNode{}
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
	}
	features := map[string]int{}
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			node := nodes[terminal.Node]
			features[fmt.Sprintf(
				"%s:%s=%s:%s",
				instance.Kind,
				terminal.Terminal,
				node.Scope,
				node.Role,
			)]++
		}
	}
	return features
}

func topologyFeatureDistance(left, right map[string]int) int {
	distance := 0
	for feature, leftCount := range left {
		rightCount := right[feature]
		if leftCount > rightCount {
			distance += leftCount - rightCount
		} else {
			distance += rightCount - leftCount
		}
	}
	for feature, rightCount := range right {
		if _, found := left[feature]; !found {
			distance += rightCount
		}
	}
	return distance
}

func addSearchConsumption(target *Consumption, value Consumption) {
	target.ExpandedStates += value.ExpandedStates
	target.GeneratedGraphs += value.GeneratedGraphs
	target.CompleteGraphs += value.CompleteGraphs
	target.MaximumFrontier = max(target.MaximumFrontier, value.MaximumFrontier)
}

func expandTopologyState(
	state topologySearchState,
	representatives []PrimitiveCandidate,
	limits GraphLimits,
	requirement Requirement,
	primitiveInventory PrimitiveInventory,
	inventory map[string]PrimitiveCandidate,
	maximumGenerated int,
) ([]topologySearchState, int) {
	if maximumGenerated <= 0 {
		return nil, 0
	}
	result := []topologySearchState{}
	generated := 0
primitiveExpansion:
	for _, primitive := range representatives {
		placements := primitivePlacements(requirement, state.graph, primitive, maxPrimitivePlacementsPerKind)
		for _, placement := range placements {
			if generated >= maximumGenerated {
				break primitiveExpansion
			}
			nextGraph := AddPrimitive(state.graph, primitive, seedPrimitiveValue(primitive), placement)
			generated++
			before := state.hash
			if before == "" {
				before, _ = GraphHash(state.graph)
			}
			operations := cloneGraphOperations(state.operations)
			operations = append(operations, GraphOperation{
				Number:        len(operations) + 1,
				Kind:          "add_primitive",
				PrimitiveKey:  primitive.Key,
				PrimitiveKind: primitive.Kind,
				Connections:   append([]TerminalConnection(nil), placement...),
				ValueSI:       seedPrimitiveValue(primitive),
				BeforeHash:    before,
			})
			result = append(result, topologySearchState{
				graph: nextGraph, operations: operations,
				score: scoreTopologyGraph(requirement, nextGraph, inventory, ""),
			})
		}
	}
	for _, instance := range state.graph.Instances {
		for _, connection := range instance.Terminals {
			if !searchRedirectableTerminal(instance.Kind, connection.Terminal) {
				continue
			}
			source, found := graphNodeByID(state.graph, connection.Node)
			if !found || source.Scope != "external" {
				continue
			}
			for _, node := range state.graph.Nodes {
				if generated >= maximumGenerated {
					break
				}
				if node.Scope != "internal" || node.ID == connection.Node {
					continue
				}
				nextGraph, err := RedirectPrimitiveTerminal(
					state.graph,
					primitiveInventory,
					instance.ID,
					connection.Terminal,
					node.ID,
				)
				if err != nil {
					continue
				}
				generated++
				before := state.hash
				if before == "" {
					before, _ = GraphHash(state.graph)
				}
				operations := cloneGraphOperations(state.operations)
				operations = append(operations, GraphOperation{
					Number:        len(operations) + 1,
					Kind:          "redirect_terminal",
					PrimitiveKey:  instance.PrimitiveKey,
					PrimitiveKind: instance.Kind,
					Node:          node.ID,
					Connections: []TerminalConnection{{
						Terminal: connection.Terminal,
						Node:     node.ID,
					}},
					BeforeHash: before,
				})
				result = append(result, topologySearchState{
					graph: nextGraph, operations: operations,
					score: scoreTopologyGraph(requirement, nextGraph, inventory, ""),
				})
			}
		}
	}
	if internalNodeCount(state.graph) < limits.MaxInternalNodes &&
		!hasDanglingInternalNode(state.graph) &&
		generated < maximumGenerated {
		nextGraph := AddInternalNode(state.graph, "internal")
		generated++
		node := nextGraph.Nodes[len(nextGraph.Nodes)-1].ID
		before := state.hash
		if before == "" {
			before, _ = GraphHash(state.graph)
		}
		operations := cloneGraphOperations(state.operations)
		operations = append(operations, GraphOperation{
			Number:     len(operations) + 1,
			Kind:       "add_internal_node",
			Node:       node,
			BeforeHash: before,
		})
		result = append(result, topologySearchState{
			graph: nextGraph, operations: operations,
			score: scoreTopologyGraph(requirement, nextGraph, inventory, ""),
		})
	}
	slices.SortFunc(result, func(left, right topologySearchState) int {
		if comparison := compareTopologyScores(left.score, right.score); comparison != 0 {
			return comparison
		}
		leftOperation := left.operations[len(left.operations)-1]
		rightOperation := right.operations[len(right.operations)-1]
		return compareGraphOperations(leftOperation, rightOperation)
	})
	if len(result) > maxSearchTransitionsPerState {
		result = result[:maxSearchTransitionsPerState]
	}
	return result, generated
}

func searchRedirectableTerminal(kind, terminal string) bool {
	switch kind {
	case "opamp", "comparator":
		return terminal == "IN_PLUS" || terminal == "IN_MINUS" || terminal == "OUT"
	case "n_channel_mosfet", "p_channel_mosfet":
		return terminal == "GATE" || terminal == "DRAIN" || terminal == "SOURCE"
	case "npn_bjt", "pnp_bjt":
		return terminal == "BASE" || terminal == "COLLECTOR" || terminal == "EMITTER"
	default:
		return false
	}
}

func primitivePlacements(requirement Requirement, graph CandidateGraph, primitive PrimitiveCandidate, limit int) [][]TerminalConnection {
	if limit <= 0 {
		return nil
	}
	terminalCandidates := make([][]string, len(primitive.Terminals))
	for index, terminal := range primitive.Terminals {
		candidates := compatibleNodesForTerminal(graph.Nodes, primitive.Kind, terminal.Terminal)
		if len(candidates) == 0 {
			return nil
		}
		terminalCandidates[index] = candidates
	}
	result := [][]TerminalConnection{}
	current := make([]TerminalConnection, len(primitive.Terminals))
	var expand func(int)
	expand = func(index int) {
		if len(result) >= maxPrimitivePlacementEnumerations {
			return
		}
		if index == len(primitive.Terminals) {
			if validPrimitivePlacement(primitive.Kind, current) &&
				validPrimitiveSupplyOrder(requirement, graph, current) &&
				!graphHasPrimitivePlacement(graph, primitive.Key, current) {
				placement := append([]TerminalConnection(nil), current...)
				slices.SortFunc(placement, compareTerminalConnections)
				result = append(result, placement)
			}
			return
		}
		terminal := primitive.Terminals[index].Terminal
		for _, node := range terminalCandidates[index] {
			current[index] = TerminalConnection{Terminal: terminal, Node: node}
			expand(index + 1)
			if len(result) >= maxPrimitivePlacementEnumerations {
				return
			}
		}
	}
	expand(0)
	slices.SortFunc(result, func(left, right []TerminalConnection) int {
		return cmp.Or(
			cmp.Compare(
				-topologyPlacementPreference(graph, primitive, left),
				-topologyPlacementPreference(graph, primitive, right),
			),
			comparePlacements(left, right),
		)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func validPrimitiveSupplyOrder(
	requirement Requirement,
	graph CandidateGraph,
	connections []TerminalConnection,
) bool {
	byTerminal := map[string]string{}
	for _, connection := range connections {
		byTerminal[connection.Terminal] = connection.Node
	}
	if byTerminal["V_PLUS"] == "" || byTerminal["V_MINUS"] == "" {
		return true
	}
	positive, positiveKnown := topologyNodeNominalVoltage(requirement, graph, byTerminal["V_PLUS"])
	negative, negativeKnown := topologyNodeNominalVoltage(requirement, graph, byTerminal["V_MINUS"])
	return !positiveKnown || !negativeKnown || positive > negative
}

func topologyPlacementPreference(
	graph CandidateGraph,
	primitive PrimitiveCandidate,
	placement []TerminalConnection,
) int {
	degrees := map[string]int{}
	nodeByID := map[string]GraphNode{}
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			degrees[terminal.Node]++
		}
	}
	byTerminal := map[string]string{}
	score := 0
	for _, connection := range placement {
		byTerminal[connection.Terminal] = connection.Node
		node := nodeByID[connection.Node]
		if node.Scope == "external" && degrees[node.ID] == 0 {
			score += 2
		}
		if node.Scope == "internal" && searchRedirectableTerminal(primitive.Kind, connection.Terminal) {
			score += 6
		}
		switch connection.Terminal {
		case "OUT":
			if node.Role == "output" {
				score += 8
			}
		case "IN_PLUS", "IN_MINUS", "GATE", "BASE":
			if node.Role == "input" || node.Role == "control" {
				score += 5
			}
		case "DRAIN", "COLLECTOR":
			if node.Role == "output" {
				score += 6
			}
		}
	}
	if byTerminal["OUT"] != "" && byTerminal["OUT"] == byTerminal["IN_MINUS"] &&
		primitive.Kind == "opamp" {
		score += 7
	}
	if primitive.Kind == "opamp" {
		inMinus := nodeByID[byTerminal["IN_MINUS"]]
		inPlus := nodeByID[byTerminal["IN_PLUS"]]
		output := nodeByID[byTerminal["OUT"]]
		if inMinus.Scope == "internal" &&
			topologyRailRole(inPlus.Role) &&
			output.Role == "output" {
			score += 7
		}
	}
	if len(primitive.Terminals) == 2 {
		left := nodeByID[placement[0].Node]
		right := nodeByID[placement[1].Node]
		if topologyPlacementBridgesActiveFeedback(graph, placement) {
			score += 6
		}
		internalCount := 0
		if left.Scope == "internal" {
			internalCount++
		}
		if right.Scope == "internal" {
			internalCount++
		}
		if internalCount == 1 {
			score += 5
			external := left
			if external.Scope == "internal" {
				external = right
			}
			switch primitive.Kind {
			case "resistor":
				if external.Role == "input" || external.Role == "control" || external.Role == "output" {
					score += 5
				} else if topologyRailRole(external.Role) {
					// Internal-to-rail resistors establish bias, references,
					// and inactive states for primitive active stages.
					score += 5
				}
			case "capacitor", "inductor":
				if external.Role == "reference" || external.Role == "supply" {
					score += 5
				}
			}
		}
	}
	return score
}

func topologyPlacementBridgesActiveFeedback(
	graph CandidateGraph,
	placement []TerminalConnection,
) bool {
	if len(placement) != 2 || placement[0].Node == placement[1].Node {
		return false
	}
	left, right := placement[0].Node, placement[1].Node
	for _, instance := range graph.Instances {
		inputTerminals, outputTerminals := topologyActiveSignalTerminals(instance.Kind)
		if len(inputTerminals) == 0 || len(outputTerminals) == 0 {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		for _, inputTerminal := range inputTerminals {
			inputNode := nodes[inputTerminal]
			for _, outputTerminal := range outputTerminals {
				outputNode := nodes[outputTerminal]
				if inputNode != "" && outputNode != "" &&
					((left == inputNode && right == outputNode) ||
						(left == outputNode && right == inputNode)) {
					return true
				}
			}
		}
	}
	return false
}

func compatibleNodesForTerminal(nodes []GraphNode, primitiveKind, terminal string) []string {
	result := []string{}
	for _, node := range nodes {
		if terminalRoleCompatible(primitiveKind, terminal, node.Role) {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return result
}

func terminalRoleCompatible(primitiveKind, terminal, nodeRole string) bool {
	switch terminal {
	case "V_PLUS", "VCC":
		return nodeRole == "supply"
	case "V_MINUS", "GND":
		return nodeRole == "reference" || nodeRole == "supply"
	case "VIN":
		return nodeRole == "supply" || nodeRole == "input" || nodeRole == "internal"
	case "VOUT":
		return nodeRole == "output" || nodeRole == "internal"
	case "ADJ":
		return nodeRole == "reference" || nodeRole == "internal"
	case "OUT":
		return nodeRole == "output" || nodeRole == "internal"
	case "IN_PLUS", "IN_MINUS":
		return nodeRole == "input" || nodeRole == "control" || nodeRole == "output" || nodeRole == "internal" || nodeRole == "reference"
	case "GATE", "BASE":
		return nodeRole == "input" || nodeRole == "control" || nodeRole == "output" || nodeRole == "internal" || nodeRole == "reference"
	case "DRAIN", "COLLECTOR":
		return nodeRole == "supply" || nodeRole == "output" || nodeRole == "internal"
	case "SOURCE", "EMITTER":
		return nodeRole == "reference" || nodeRole == "supply" || nodeRole == "output" || nodeRole == "internal"
	case "ANODE", "CATHODE", "A", "B":
		return nodeRole != ""
	default:
		return primitiveKind != "" && nodeRole != ""
	}
}

func validPrimitivePlacement(kind string, connections []TerminalConnection) bool {
	nodes := map[string]bool{}
	byTerminal := map[string]string{}
	for _, connection := range connections {
		nodes[connection.Node] = true
		byTerminal[connection.Terminal] = connection.Node
	}
	if len(nodes) < 2 {
		return false
	}
	if byTerminal["V_PLUS"] != "" && byTerminal["V_PLUS"] == byTerminal["V_MINUS"] {
		return false
	}
	if slices.Contains([]string{"capacitor", "inductor", "resistor"}, kind) {
		return byTerminal["A"] < byTerminal["B"]
	}
	return true
}

func graphHasPrimitivePlacement(graph CandidateGraph, primitiveKey string, placement []TerminalConnection) bool {
	for _, instance := range graph.Instances {
		if instance.PrimitiveKey != primitiveKey || len(instance.Terminals) != len(placement) {
			continue
		}
		existing := append([]TerminalConnection(nil), instance.Terminals...)
		slices.SortFunc(existing, compareTerminalConnections)
		if slices.Equal(existing, placement) {
			return true
		}
	}
	return false
}

func topologyRepresentatives(requirement Requirement, inventory PrimitiveInventory) []PrimitiveCandidate {
	requiredAnalyses := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		requiredAnalyses[trustedModelAnalysisKind(assertion.Analysis)] = true
	}
	byKind := map[string][]PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitiveCoversAnyAnalysis(primitive, requiredAnalyses) {
			byKind[primitive.Kind] = append(byKind[primitive.Kind], primitive)
		}
	}
	kinds := make([]string, 0, len(byKind))
	for kind := range byKind {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	result := []PrimitiveCandidate{}
	for _, kind := range kinds {
		candidates := byKind[kind]
		slices.SortFunc(candidates, func(left, right PrimitiveCandidate) int {
			return compareRepresentativePrimitives(left, right, requiredAnalyses)
		})
		if len(candidates) != 0 {
			result = append(result, candidates[0])
		}
	}
	return result
}

func primitiveCoversAnyAnalysis(primitive PrimitiveCandidate, required map[string]bool) bool {
	if len(required) == 0 {
		return true
	}
	for _, model := range primitive.Models {
		for analysis := range required {
			if reviewedPrimitiveModelSupportsCircuitAnalysis(model, analysis) {
				return true
			}
		}
	}
	return false
}

func compareRepresentativePrimitives(left, right PrimitiveCandidate, required map[string]bool) int {
	return cmp.Or(
		cmp.Compare(primitiveEvidencePenalty(left.Evidence), primitiveEvidencePenalty(right.Evidence)),
		cmp.Compare(-primitiveAnalysisCoverage(left, required), -primitiveAnalysisCoverage(right, required)),
		comparePositiveArea(left.AreaMM2, right.AreaMM2),
		cmp.Compare(left.Key, right.Key),
	)
}

func primitiveAnalysisCoverage(primitive PrimitiveCandidate, required map[string]bool) int {
	covered := map[string]bool{}
	for _, model := range primitive.Models {
		for analysis := range required {
			if reviewedPrimitiveModelSupportsCircuitAnalysis(model, analysis) {
				covered[analysis] = true
			}
		}
	}
	return len(covered)
}

func seedPrimitiveValue(primitive PrimitiveCandidate) *float64 {
	if primitive.ValueDomain == nil {
		return nil
	}
	domain := primitive.ValueDomain
	if domain.Nominal != nil && *domain.Nominal > 0 {
		return cloneInventoryFloat(domain.Nominal)
	}
	if domain.Minimum != nil && domain.Maximum != nil && *domain.Minimum > 0 && *domain.Maximum > 0 {
		value := geometricMean(*domain.Minimum, *domain.Maximum)
		return &value
	}
	if domain.Minimum != nil && *domain.Minimum > 0 {
		return cloneInventoryFloat(domain.Minimum)
	}
	if domain.Maximum != nil && *domain.Maximum > 0 {
		value := *domain.Maximum / 10
		if value <= 0 {
			value = *domain.Maximum
		}
		return &value
	}
	return nil
}

func scoreTopologyGraph(requirement Requirement, graph CandidateGraph, inventory map[string]PrimitiveCandidate, fingerprint string) TopologyScore {
	degrees := map[string]int{}
	adjacency := map[string][]string{}
	evidencePenalty := 0
	area := 0.0
	for _, instance := range graph.Instances {
		primitive := inventory[instance.PrimitiveKey]
		evidencePenalty += primitiveEvidencePenalty(primitive.Evidence)
		area += primitive.AreaMM2
		instanceVertex := "instance:" + instance.ID
		for _, terminal := range instance.Terminals {
			degrees[terminal.Node]++
			nodeVertex := "node:" + terminal.Node
			adjacency[instanceVertex] = append(adjacency[instanceVertex], nodeVertex)
			adjacency[nodeVertex] = append(adjacency[nodeVertex], instanceVertex)
		}
	}
	score := TopologyScore{
		BehaviorGap:       topologyBehaviorGap(requirement, graph, inventory),
		RedundantActive:   topologyRedundantActiveCount(requirement, graph),
		EndpointAccess:    topologyEndpointAccessPenalty(requirement, graph),
		PrimitiveCount:    len(graph.Instances),
		InternalNodeCount: internalNodeCount(graph),
		EvidencePenalty:   evidencePenalty,
		AreaMM2:           quantizeInventory(area),
		Fingerprint:       fingerprint,
	}
	starts := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && degrees[node.ID] == 0 {
			score.UnconnectedExternal++
		}
		if node.Scope == "internal" && degrees[node.ID] < 2 {
			score.DanglingInternal++
		}
		if node.Role == "input" || node.Role == "control" || node.Role == "supply" {
			starts = append(starts, "node:"+node.ID)
		}
	}
	for _, node := range graph.Nodes {
		if node.Role != "output" {
			continue
		}
		reachable := false
		for _, start := range starts {
			if graphPathExists(adjacency, start, "node:"+node.ID) {
				reachable = true
				break
			}
		}
		if !reachable {
			score.UnreachableOutputs++
		}
	}
	return score
}

func topologyEndpointAccessPenalty(requirement Requirement, graph CandidateGraph) int {
	requireAnalog := false
	requireFeedback := false
	requireFeedbackImpedance := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "phase_margin", "gain_margin":
			requireAnalog = true
			requireFeedback = true
		case "transimpedance":
			requireAnalog = true
			requireFeedback = true
		case "voltage_gain", "voltage_gain_at_frequency", "cutoff_frequency",
			"bandwidth", "output_noise_rms", "thd", "total_harmonic_distortion":
			requireAnalog = true
		}
	}
	if !requireAnalog {
		return 0
	}
	adjacency := topologyPassiveNodeAdjacency(graph, false)
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	inputs := topologyNodesByRole(graph, "input", "control")
	outputs := topologyNodesByRole(graph, "output")
	if topologyGraphHasCompositeAnalogStage(
		graph, requireFeedback, false, requireFeedbackImpedance,
	) {
		return 0
	}
	best := 1_000
	for _, instance := range graph.Instances {
		inputTerminals, outputTerminals := topologyActiveSignalTerminals(instance.Kind)
		if len(inputTerminals) == 0 || len(outputTerminals) == 0 {
			continue
		}
		inputTerminals = topologyEffectiveInputTerminals(instance, inputTerminals)
		terminalNodes := topologyTerminalNodes(instance)
		inputDistance := topologyMinimumTerminalDistance(
			terminalNodes, inputTerminals, inputs, adjacency,
		)
		outputDistance := topologyMinimumTerminalDistance(
			terminalNodes, outputTerminals, outputs, adjacency,
		)
		penalty := 0
		if inputDistance >= 100 {
			penalty += 100
		} else if inputDistance > 1 {
			penalty += inputDistance - 1
		}
		if outputDistance >= 100 {
			penalty += 100
		} else if outputDistance > 1 {
			penalty += outputDistance - 1
		}
		if requireFeedback {
			if instance.Kind != "opamp" {
				penalty += 100
			} else {
				if requireFeedbackImpedance &&
					nodeByID[terminalNodes["IN_MINUS"]].Scope != "internal" {
					penalty += 100
				}
				feedbackDistance := topologyNodeDistance(
					dcAdjacency, terminalNodes["OUT"], terminalNodes["IN_MINUS"],
				)
				if feedbackDistance >= 100 ||
					(requireFeedbackImpedance && feedbackDistance == 0) {
					penalty += 100
				} else if feedbackDistance > 1 {
					penalty += feedbackDistance - 1
				}
			}
		}
		best = min(best, penalty)
	}
	return best
}

func topologyRedundantActiveCount(requirement Requirement, graph CandidateGraph) int {
	minimumActive := 0
	minimumDecision := 0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "lower_threshold", "upper_threshold":
			minimumDecision = max(minimumDecision, 2)
		case "rising_threshold", "falling_threshold", "threshold_voltage", "threshold_current",
			"hysteresis":
			minimumDecision = max(minimumDecision, 1)
		case "phase_margin", "gain_margin", "voltage_gain", "voltage_gain_at_frequency",
			"output_noise_rms",
			"thd", "total_harmonic_distortion", "output_current", "transconductance", "transimpedance",
			"line_regulation", "load_regulation", "off_state_current", "on_state_voltage",
			"propagation_delay", "startup_current", "junction_temperature", "soa_margin":
			minimumActive = max(minimumActive, 1)
		}
		if assertion.Excitation != nil && assertion.Excitation.Kind == "port" &&
			requirementPortIsControl(requirement, assertion.Excitation.ID) {
			minimumActive = max(minimumActive, 1)
		}
	}
	minimumActive = max(minimumActive, minimumDecision)
	active := 0
	for _, instance := range graph.Instances {
		if topologyActiveKind(instance.Kind) {
			active++
		}
	}
	return max(0, active-minimumActive)
}

func topologyBehaviorGap(
	requirement Requirement,
	graph CandidateGraph,
	inventory map[string]PrimitiveCandidate,
) int {
	_, boundedBipolarTransfer := topologyBoundedBipolarEnvelope(requirement)
	requireActive := false
	requireReactive := false
	requireTimeConstant := false
	requireFeedback := false
	requireFeedbackImpedance := false
	requireHighFrequencyAttenuation := false
	requireThermal := false
	requireSOA := false
	requireAnalogTransfer := false
	requireInternalTransfer := false
	requireControlledSwitch := false
	requireDecisionReference := false
	requireAbsoluteDecisionReference := topologyRequiresAbsoluteDecisionReference(requirement)
	decisionPolarity := topologyDecisionPolarity(requirement)
	minimumResistors := 0
	minimumDecisionStages := 0
	minimumPassbandGain := 0.0
	maximumHighFrequencyGain := 0.0
	hasPassbandGain := false
	hasHighFrequencyGain := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "cutoff_frequency":
			requireReactive = true
			requireTimeConstant = true
			minimumResistors = max(minimumResistors, 1)
		case "phase_margin", "gain_margin", "hysteresis":
			requireActive = true
			requireFeedback = true
			if assertion.Metric == "hysteresis" {
				minimumResistors = max(minimumResistors, 4)
			} else {
				requireAnalogTransfer = true
			}
		case "rising_threshold", "falling_threshold", "threshold_voltage", "threshold_current":
			requireActive = true
			requireDecisionReference = true
			minimumDecisionStages = max(minimumDecisionStages, 1)
			minimumResistors = max(minimumResistors, 2)
		case "lower_threshold", "upper_threshold":
			requireActive = true
			requireDecisionReference = true
			minimumDecisionStages = max(minimumDecisionStages, 2)
			minimumResistors = max(minimumResistors, 4)
		case "output_current", "transconductance":
			requireActive = true
			minimumResistors = max(minimumResistors, 1)
		case "transimpedance":
			requireActive = true
			requireAnalogTransfer = true
			requireFeedback = true
			minimumResistors = max(minimumResistors, 1)
		case "line_regulation", "load_regulation":
			requireActive = true
			minimumResistors = max(minimumResistors, 2)
		case
			"off_state_current", "on_state_voltage", "propagation_delay", "startup_current":
			requireActive = true
		case "junction_temperature":
			requireActive = true
			requireThermal = true
		case "soa_margin":
			requireActive = true
			requireSOA = true
		case "thd", "total_harmonic_distortion":
			requireActive = true
			requireAnalogTransfer = true
		case "voltage_gain", "voltage_gain_at_frequency", "output_noise_rms":
			if !boundedBipolarTransfer {
				requireActive = true
				requireAnalogTransfer = true
			}
			if assertion.Metric == "voltage_gain" && assertion.Min != nil {
				if !hasPassbandGain || *assertion.Min > minimumPassbandGain {
					minimumPassbandGain = *assertion.Min
				}
				hasPassbandGain = true
			}
			if assertion.Metric == "voltage_gain_at_frequency" && assertion.Max != nil {
				if !hasHighFrequencyGain || *assertion.Max < maximumHighFrequencyGain {
					maximumHighFrequencyGain = *assertion.Max
				}
				hasHighFrequencyGain = true
			}
			if assertion.Metric == "voltage_gain" &&
				assertion.Min != nil && *assertion.Min > 1 {
				minimumResistors = max(minimumResistors, 2)
			}
		case "oscillation_frequency", "duty_cycle":
			if assertion.Excitation == nil {
				requireActive = true
				requireReactive = true
				requireTimeConstant = true
				requireFeedback = true
				requireDecisionReference = true
				minimumDecisionStages = max(minimumDecisionStages, 1)
				minimumResistors = max(minimumResistors, 3)
			}
		case "output_ripple":
			requireReactive = true
		case "conversion_efficiency":
			requireActive = true
		}
		if assertion.Excitation != nil && assertion.Excitation.Kind == "port" &&
			requirementPortIsControl(requirement, assertion.Excitation.ID) {
			requireActive = true
			requireControlledSwitch = true
			minimumResistors = max(minimumResistors, 1)
		}
	}
	requireHighFrequencyAttenuation = hasPassbandGain &&
		hasHighFrequencyGain &&
		maximumHighFrequencyGain < minimumPassbandGain
	requireInternalTransfer = requireReactive && requireAnalogTransfer
	activeCount, analogActiveCount, switchingActiveCount, decisionCount, resistorCount := 0, 0, 0, 0, 0
	hasReactive, hasResistor, hasThermal, hasSOA := false, false, false, false
	for _, instance := range graph.Instances {
		primitive := inventory[instance.PrimitiveKey]
		if topologyActiveKind(instance.Kind) {
			activeCount++
		}
		if topologyAnalogActiveKind(instance.Kind) {
			analogActiveCount++
		}
		if topologySwitchingActiveKind(instance.Kind) {
			switchingActiveCount++
		}
		if instance.Kind == "opamp" || instance.Kind == "comparator" {
			decisionCount++
		}
		if instance.Kind == "capacitor" || instance.Kind == "inductor" {
			hasReactive = true
		}
		if instance.Kind == "resistor" {
			hasResistor = true
			resistorCount++
		}
		hasThermal = hasThermal || primitiveHasThermalEvidence(primitive)
		for _, model := range primitive.Models {
			hasSOA = hasSOA || len(model.TransientSOA) != 0
		}
	}
	gap := 0
	if requireActive && activeCount == 0 {
		gap++
	}
	if requireReactive && !hasReactive {
		gap++
	}
	if resistorCount < minimumResistors {
		gap += minimumResistors - resistorCount
	}
	if requireTimeConstant && !requireAnalogTransfer && !topologyGraphHasPassiveTimeConstant(graph) {
		gap++
	}
	if requireAnalogTransfer && analogActiveCount == 0 {
		gap++
	}
	if requireAnalogTransfer {
		gap += topologyGraphAnalogStageGap(
			graph, requireFeedback, requireTimeConstant, requireFeedbackImpedance,
		)
	}
	if requireHighFrequencyAttenuation &&
		!topologyGraphHasHighFrequencyAttenuationStage(graph) {
		gap++
	}
	if requireInternalTransfer && internalNodeCount(graph) == 0 {
		gap++
	}
	if requireControlledSwitch && switchingActiveCount == 0 {
		gap++
	}
	if requireDecisionReference {
		gap += topologyDecisionStageGap(
			graph,
			minimumDecisionStages,
			requireFeedback,
			decisionPolarity,
		)
		if requireAbsoluteDecisionReference && !topologyGraphHasAbsoluteDecisionReference(graph) {
			gap++
		}
	} else if requireFeedback && !requireAnalogTransfer &&
		(!hasResistor || !topologyGraphHasDecisionFeedback(graph)) {
		gap++
	}
	if requireThermal && !hasThermal {
		gap++
	}
	if requireSOA && !hasSOA {
		gap++
	}
	if decisionCount < minimumDecisionStages {
		gap += minimumDecisionStages - decisionCount
	}
	gap += topologySwitchedLoadEnvelopeGap(requirement, graph, inventory)
	return gap
}

func topologyRequiresAbsoluteDecisionReference(requirement Requirement) bool {
	caseByID := make(map[string]OperatingCase, len(requirement.Requirements.OperatingCases))
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		caseByID[operatingCase.ID] = operatingCase
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if !slices.Contains(
			[]string{"falling_threshold", "rising_threshold", "threshold_voltage", "lower_threshold", "upper_threshold"},
			assertion.Metric,
		) || assertion.Unit != "V" {
			continue
		}
		thresholdMinimum, thresholdMaximum := 0.0, 0.0
		if assertion.Min != nil {
			thresholdMinimum = *assertion.Min
		}
		if assertion.Max != nil {
			thresholdMaximum = *assertion.Max
		}
		if thresholdMinimum == 0 {
			thresholdMinimum = thresholdMaximum
		}
		if thresholdMaximum == 0 {
			thresholdMaximum = thresholdMinimum
		}
		for _, caseID := range assertion.OperatingCases {
			for _, condition := range caseByID[caseID].Conditions {
				if condition.Axis == "supply_voltage" && condition.Min < condition.Max &&
					thresholdMaximum >= condition.Min && thresholdMinimum <= condition.Max {
					return true
				}
			}
		}
	}
	return false
}

func topologyGraphHasAbsoluteDecisionReference(graph CandidateGraph) bool {
	references := topologyNodesByRole(graph, "reference")
	if len(references) == 0 {
		return false
	}
	passive := topologyPassiveNodeAdjacencyWithRailAccess(graph, true, true)
	for _, source := range graph.Instances {
		if source.Kind != "reference_diode" {
			continue
		}
		terminals := topologyTerminalNodes(source)
		cathode, anode := terminals["CATHODE"], terminals["ANODE"]
		if cathode == "" || anode == "" {
			continue
		}
		anodeReferenced := false
		for _, reference := range references {
			if anode == reference || topologyNodePathExists(passive, anode, reference) {
				anodeReferenced = true
				break
			}
		}
		if !anodeReferenced {
			continue
		}
		for _, decision := range graph.Instances {
			if decision.Kind != "comparator" && decision.Kind != "opamp" {
				continue
			}
			decisionTerminals := topologyTerminalNodes(decision)
			for _, input := range []string{"IN_PLUS", "IN_MINUS"} {
				if node := decisionTerminals[input]; node != "" &&
					(cathode == node || topologyNodePathExists(passive, cathode, node)) {
					return true
				}
			}
		}
	}
	return false
}

func topologyGraphHasHighFrequencyAttenuationStage(graph CandidateGraph) bool {
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	reactiveEdges := [][2]string{}
	for _, instance := range graph.Instances {
		if instance.Kind != "capacitor" && instance.Kind != "inductor" {
			continue
		}
		if len(instance.Terminals) != 2 ||
			instance.Terminals[0].Node == instance.Terminals[1].Node {
			continue
		}
		reactiveEdges = append(reactiveEdges, [2]string{
			instance.Terminals[0].Node,
			instance.Terminals[1].Node,
		})
	}
	if len(reactiveEdges) == 0 {
		return false
	}
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	externalInputs := topologyNodesByRole(graph, "input", "control")
	for _, instance := range graph.Instances {
		inputTerminals, outputTerminals := topologyActiveSignalTerminals(instance.Kind)
		if len(inputTerminals) == 0 || len(outputTerminals) == 0 {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		for _, inputTerminal := range inputTerminals {
			inputNode := nodes[inputTerminal]
			if inputNode == "" {
				continue
			}
			inputAccessible := topologySignalFlowReachable(graph, externalInputs, inputNode)
			if !inputAccessible {
				continue
			}
			isFeedbackInput := false
			for _, outputTerminal := range outputTerminals {
				outputNode := nodes[outputTerminal]
				isFeedbackInput = isFeedbackInput ||
					topologyNodePathExists(dcAdjacency, outputNode, inputNode)
				for _, edge := range reactiveEdges {
					if (edge[0] == inputNode && edge[1] == outputNode) ||
						(edge[0] == outputNode && edge[1] == inputNode) {
						if instance.Kind != "opamp" ||
							inputTerminal == "IN_MINUS" {
							return true
						}
					}
				}
			}
			if nodeByID[inputNode].Scope != "internal" || isFeedbackInput {
				continue
			}
			for _, edge := range reactiveEdges {
				other := ""
				if edge[0] == inputNode {
					other = edge[1]
				} else if edge[1] == inputNode {
					other = edge[0]
				}
				if other != "" && topologyRailRole(nodeByID[other].Role) {
					return true
				}
			}
		}
	}
	return false
}

func topologySignalFlowReachable(graph CandidateGraph, starts []string, target string) bool {
	adjacency := topologyPassiveNodeAdjacency(graph, false)
	for _, instance := range graph.Instances {
		inputTerminals, outputTerminals := topologyActiveSignalTerminals(instance.Kind)
		if len(inputTerminals) == 0 || len(outputTerminals) == 0 {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		for _, inputTerminal := range topologyEffectiveInputTerminals(instance, inputTerminals) {
			input := nodes[inputTerminal]
			if input == "" {
				continue
			}
			for _, outputTerminal := range outputTerminals {
				if output := nodes[outputTerminal]; output != "" {
					adjacency[input] = append(adjacency[input], output)
				}
			}
		}
	}
	visited := map[string]bool{}
	queue := append([]string(nil), starts...)
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		if visited[node] {
			continue
		}
		if node == target {
			return true
		}
		visited[node] = true
		queue = append(queue, adjacency[node]...)
	}
	return false
}

func topologyNodeIsInternal(graph CandidateGraph, nodeID string) bool {
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			return node.Scope == "internal"
		}
	}
	return false
}

func topologyAnalogActiveKind(kind string) bool {
	switch kind {
	case "opamp", "n_channel_mosfet", "p_channel_mosfet", "npn_bjt", "pnp_bjt",
		"fixed_voltage_regulator", "adjustable_voltage_regulator":
		return true
	default:
		return false
	}
}

func topologySwitchingActiveKind(kind string) bool {
	switch kind {
	case "comparator", "n_channel_mosfet", "p_channel_mosfet", "npn_bjt", "pnp_bjt", "synchronous_buck_regulator":
		return true
	default:
		return false
	}
}

func topologyActiveKind(kind string) bool {
	switch kind {
	case "opamp", "comparator", "n_channel_mosfet", "p_channel_mosfet", "npn_bjt", "pnp_bjt",
		"fixed_voltage_regulator", "adjustable_voltage_regulator", "synchronous_buck_regulator", "signal_diode":
		return true
	default:
		return false
	}
}

func requirementPortIsControl(requirement Requirement, id string) bool {
	for _, port := range requirement.Requirements.Ports {
		if port.ID == id {
			return port.Kind == "digital" || port.Kind == "control"
		}
	}
	return false
}

func requirementPortDrivesDecision(requirement Requirement, id string) bool {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Excitation == nil || assertion.Excitation.Kind != "port" ||
			assertion.Excitation.ID != id {
			continue
		}
		switch assertion.Metric {
		case "hysteresis", "rising_threshold", "falling_threshold",
			"threshold_voltage", "threshold_current", "lower_threshold", "upper_threshold":
			return true
		}
	}
	return false
}

func topologyControlNodes(requirement Requirement, graph CandidateGraph) []string {
	result := []string{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" || (node.Role != "control" && node.Role != "input") {
			continue
		}
		if node.Role == "control" || requirementPortIsControl(requirement, node.SemanticID) ||
			requirementPortDrivesDecision(requirement, node.SemanticID) {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return result
}

func topologyGraphHasAnalogTransfer(graph CandidateGraph) bool {
	signalAdjacency := topologyPassiveNodeAdjacency(graph, false)
	inputs := topologyNodesByRole(graph, "input", "control")
	outputs := topologyNodesByRole(graph, "output")
	for _, instance := range graph.Instances {
		inputTerminals, outputTerminals := topologyActiveSignalTerminals(instance.Kind)
		if len(inputTerminals) == 0 || len(outputTerminals) == 0 {
			continue
		}
		terminalNodes := topologyTerminalNodes(instance)
		inputTerminals = topologyEffectiveInputTerminals(instance, inputTerminals)
		if topologyAnyTerminalReachable(terminalNodes, inputTerminals, inputs, signalAdjacency) &&
			topologyAnyTerminalReachable(terminalNodes, outputTerminals, outputs, signalAdjacency) {
			return true
		}
	}
	return false
}

func topologyGraphHasAnalogStage(
	graph CandidateGraph,
	requireFeedback bool,
	requireTimeConstant bool,
	requireFeedbackImpedance bool,
) bool {
	signalAdjacency := topologyPassiveNodeAdjacency(graph, false)
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	inputs := topologyNodesByRole(graph, "input", "control")
	outputs := topologyNodesByRole(graph, "output")
	for _, instance := range graph.Instances {
		inputTerminals, outputTerminals := topologyActiveSignalTerminals(instance.Kind)
		if len(inputTerminals) == 0 || len(outputTerminals) == 0 {
			continue
		}
		terminalNodes := topologyTerminalNodes(instance)
		inputTerminals = topologyEffectiveInputTerminals(instance, inputTerminals)
		if !topologyAnyTerminalReachable(terminalNodes, inputTerminals, inputs, signalAdjacency) ||
			!topologyAnyTerminalReachable(terminalNodes, outputTerminals, outputs, signalAdjacency) {
			continue
		}
		if requireFeedback {
			if instance.Kind != "opamp" ||
				!topologyNodePathExists(dcAdjacency, terminalNodes["OUT"], terminalNodes["IN_MINUS"]) ||
				(requireFeedbackImpedance &&
					(terminalNodes["OUT"] == terminalNodes["IN_MINUS"] ||
						!topologyNodeIsInternal(graph, terminalNodes["IN_MINUS"]))) {
				continue
			}
		}
		if requireTimeConstant && !topologyInstanceHasPassiveTimeConstant(graph, instance) {
			continue
		}
		return true
	}
	return false
}

func topologyGraphAnalogStageGap(
	graph CandidateGraph,
	requireFeedback bool,
	requireTimeConstant bool,
	requireFeedbackImpedance bool,
) int {
	if requireFeedback && topologyGraphHasReferenceRegulatedOutput(graph, requireTimeConstant) {
		return 0
	}
	if topologyGraphHasCompositeAnalogStage(
		graph, requireFeedback, requireTimeConstant, requireFeedbackImpedance,
	) {
		return 0
	}
	signalAdjacency := topologyPassiveNodeAdjacency(graph, false)
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	inputs := topologyNodesByRole(graph, "input", "control")
	outputs := topologyNodesByRole(graph, "output")
	best := int(^uint(0) >> 1)
	found := false
	for _, instance := range graph.Instances {
		inputTerminals, outputTerminals := topologyActiveSignalTerminals(instance.Kind)
		if len(inputTerminals) == 0 || len(outputTerminals) == 0 {
			continue
		}
		found = true
		terminalNodes := topologyTerminalNodes(instance)
		inputTerminals = topologyEffectiveInputTerminals(instance, inputTerminals)
		stageGap := 0
		if !topologyAnyTerminalReachable(terminalNodes, inputTerminals, inputs, signalAdjacency) ||
			!topologyAnyTerminalReachable(terminalNodes, outputTerminals, outputs, signalAdjacency) {
			stageGap++
		}
		if requireFeedback &&
			(instance.Kind != "opamp" ||
				!topologyNodePathExists(dcAdjacency, terminalNodes["OUT"], terminalNodes["IN_MINUS"]) ||
				(requireFeedbackImpedance &&
					(terminalNodes["OUT"] == terminalNodes["IN_MINUS"] ||
						!topologyNodeIsInternal(graph, terminalNodes["IN_MINUS"])))) {
			stageGap++
		}
		if requireTimeConstant {
			if !topologyInstanceHasInternalSignalTerminal(graph, instance) {
				// Endpoint access is a prerequisite for constructing a real
				// internal time constant, so rank it ahead of incidental
				// passives attached only to external ideal sources.
				stageGap += 2
			}
			if !topologyInstanceHasPassiveTimeConstant(graph, instance) {
				stageGap++
			}
		}
		best = min(best, stageGap)
	}
	if !found {
		return 1
	}
	return best
}

func topologyGraphHasReferenceRegulatedOutput(
	graph CandidateGraph,
	requireTimeConstant bool,
) bool {
	supplies := topologyNodesByRole(graph, "supply")
	references := topologyNodesByRole(graph, "reference")
	commands := topologyNodesByRole(graph, "input", "control")
	outputs := topologyNodesByRole(graph, "output")
	if len(supplies) == 0 || len(references) == 0 || len(outputs) == 0 {
		return false
	}
	passive := topologyPassiveNodeAdjacency(graph, true)
	if requireTimeConstant && !topologyGraphHasPassiveTimeConstant(graph) {
		return false
	}
	for _, controller := range graph.Instances {
		if controller.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(controller)
		for _, invertedDrive := range []bool{false, true} {
			setpointTerminal, feedbackTerminal := terminals["IN_PLUS"], terminals["IN_MINUS"]
			if invertedDrive {
				setpointTerminal, feedbackTerminal = feedbackTerminal, setpointTerminal
			}
			setpointDriven := false
			for _, reference := range graph.Instances {
				if reference.Kind != "reference_diode" {
					continue
				}
				referenceTerminals := topologyTerminalNodes(reference)
				if referenceTerminals["CATHODE"] == setpointTerminal &&
					slices.Contains(references, referenceTerminals["ANODE"]) {
					setpointDriven = true
					break
				}
			}
			for _, command := range commands {
				setpointDriven = setpointDriven || topologyNodePathExists(passive, command, setpointTerminal)
			}
			if !setpointDriven || !topologyNodeIsInternal(graph, feedbackTerminal) {
				continue
			}
			feedback := false
			for _, output := range outputs {
				feedback = feedback || topologyNodePathExists(passive, output, feedbackTerminal)
			}
			if !feedback {
				continue
			}
			for _, passDevice := range graph.Instances {
				if passDevice.Kind != "npn_bjt" {
					continue
				}
				passTerminals := topologyTerminalNodes(passDevice)
				driven := topologyNodePathExists(passive, terminals["OUT"], passTerminals["BASE"])
				if invertedDrive {
					driven = false
					for _, driver := range graph.Instances {
						if driver.Kind != "pnp_bjt" {
							continue
						}
						driverTerminals := topologyTerminalNodes(driver)
						if slices.Contains(supplies, driverTerminals["EMITTER"]) &&
							topologyNodePathExists(passive, terminals["OUT"], driverTerminals["BASE"]) &&
							topologyNodePathExists(passive, driverTerminals["COLLECTOR"], passTerminals["BASE"]) {
							driven = true
							break
						}
					}
				}
				if !driven || !slices.Contains(supplies, passTerminals["COLLECTOR"]) {
					continue
				}
				for _, output := range outputs {
					if topologyNodePathExists(passive, passTerminals["EMITTER"], output) {
						return true
					}
				}
			}
		}
	}
	return false
}

// topologyGraphHasCompositeAnalogStage recognizes a signal path spanning more
// than one active primitive. Ordinary endpoint checks intentionally reason
// about one primitive at a time; that is insufficient for a controller driving
// a discrete output stage. This graph is directional across active terminals
// and excludes rail-connected passive edges, preventing power nets from being
// mistaken for signal continuity.
func topologyGraphHasCompositeAnalogStage(
	graph CandidateGraph,
	requireFeedback bool,
	requireTimeConstant bool,
	requireFeedbackImpedance bool,
) bool {
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	adjacency := map[string][]string{}
	addEdge := func(from, to string) {
		if from == "" || to == "" || from == to {
			return
		}
		adjacency[from] = append(adjacency[from], to)
	}
	for _, instance := range graph.Instances {
		terminals := topologyTerminalNodes(instance)
		switch instance.Kind {
		case "resistor", "capacitor", "inductor":
			if len(instance.Terminals) != 2 {
				continue
			}
			left := instance.Terminals[0].Node
			right := instance.Terminals[1].Node
			if topologyRailRole(nodeByID[left].Role) ||
				topologyRailRole(nodeByID[right].Role) {
				continue
			}
			addEdge(left, right)
			addEdge(right, left)
		case "signal_diode", "reference_diode":
			if topologyRailRole(nodeByID[terminals["ANODE"]].Role) ||
				topologyRailRole(nodeByID[terminals["CATHODE"]].Role) {
				continue
			}
			addEdge(terminals["ANODE"], terminals["CATHODE"])
			addEdge(terminals["CATHODE"], terminals["ANODE"])
		case "opamp", "comparator":
			addEdge(terminals["IN_PLUS"], terminals["OUT"])
			addEdge(terminals["IN_MINUS"], terminals["OUT"])
		case "npn_bjt", "pnp_bjt":
			if terminals["BASE"] != "" && terminals["BASE"] == terminals["COLLECTOR"] {
				// A diode-connected BJT is a two-terminal incremental
				// junction. Bias midpoint motion propagates through it in
				// either small-signal direction even though its DC current is
				// polarity constrained by the nonlinear device model.
				addEdge(terminals["BASE"], terminals["EMITTER"])
				addEdge(terminals["EMITTER"], terminals["BASE"])
				continue
			}
			addEdge(terminals["BASE"], terminals["COLLECTOR"])
			addEdge(terminals["BASE"], terminals["EMITTER"])
		case "n_channel_mosfet", "p_channel_mosfet":
			addEdge(terminals["GATE"], terminals["DRAIN"])
			addEdge(terminals["GATE"], terminals["SOURCE"])
		}
	}
	pathExists := func(starts []string, target string) bool {
		visited := map[string]bool{}
		queue := append([]string(nil), starts...)
		for len(queue) != 0 {
			current := queue[0]
			queue = queue[1:]
			if current == target {
				return true
			}
			if visited[current] {
				continue
			}
			visited[current] = true
			for _, next := range adjacency[current] {
				if !visited[next] {
					queue = append(queue, next)
				}
			}
		}
		return false
	}
	inputs := topologyNodesByRole(graph, "input", "control")
	outputs := topologyNodesByRole(graph, "output")
	forward := false
	for _, output := range outputs {
		forward = forward || pathExists(inputs, output)
	}
	if !forward {
		return false
	}
	if requireTimeConstant {
		hasReactive := false
		for _, instance := range graph.Instances {
			hasReactive = hasReactive ||
				instance.Kind == "capacitor" || instance.Kind == "inductor"
		}
		if !hasReactive {
			return false
		}
	}
	if !requireFeedback {
		return true
	}
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if !pathExists(inputs, terminals["IN_PLUS"]) {
			continue
		}
		forwardOutput := false
		for _, output := range outputs {
			forwardOutput = forwardOutput || pathExists([]string{terminals["OUT"]}, output)
		}
		if !forwardOutput {
			continue
		}
		for _, output := range outputs {
			if !topologyNodePathExists(dcAdjacency, output, terminals["IN_MINUS"]) {
				continue
			}
			if requireFeedbackImpedance &&
				(output == terminals["IN_MINUS"] ||
					!topologyNodeIsInternal(graph, terminals["IN_MINUS"])) {
				continue
			}
			return true
		}
	}
	return false
}

func topologyInstanceHasInternalSignalTerminal(graph CandidateGraph, instance GraphInstance) bool {
	internal := map[string]bool{}
	for _, node := range graph.Nodes {
		internal[node.ID] = node.Scope == "internal"
	}
	inputs, outputs := topologyActiveSignalTerminals(instance.Kind)
	signalTerminals := append(append([]string(nil), inputs...), outputs...)
	terminalNodes := topologyTerminalNodes(instance)
	for _, terminal := range signalTerminals {
		if internal[terminalNodes[terminal]] {
			return true
		}
	}
	return false
}

func topologyGraphHasPassiveTimeConstant(graph CandidateGraph) bool {
	type passiveEdge struct {
		left     string
		right    string
		resistor bool
		reactive bool
	}
	edges := []passiveEdge{}
	byNode := map[string][]int{}
	nodeRoles := map[string]string{}
	nodeScopes := map[string]string{}
	for _, node := range graph.Nodes {
		nodeRoles[node.ID] = node.Role
		nodeScopes[node.ID] = node.Scope
	}
	for _, instance := range graph.Instances {
		if len(instance.Terminals) != 2 {
			continue
		}
		edge := passiveEdge{
			left:  instance.Terminals[0].Node,
			right: instance.Terminals[1].Node,
		}
		switch instance.Kind {
		case "resistor":
			edge.resistor = true
		case "capacitor", "inductor":
			edge.reactive = true
		default:
			continue
		}
		index := len(edges)
		edges = append(edges, edge)
		byNode[edge.left] = append(byNode[edge.left], index)
		byNode[edge.right] = append(byNode[edge.right], index)
	}
	for start := range byNode {
		hasResistor, hasReactive := false, false
		hasInput, hasOutput := false, false
		visitedNodes := map[string]bool{start: true}
		queue := []string{start}
		for len(queue) != 0 {
			node := queue[0]
			queue = queue[1:]
			if nodeScopes[node] == "external" {
				hasInput = hasInput ||
					nodeRoles[node] == "input" || nodeRoles[node] == "control"
				hasOutput = hasOutput || nodeRoles[node] == "output"
			}
			for _, edgeIndex := range byNode[node] {
				edge := edges[edgeIndex]
				hasResistor = hasResistor || edge.resistor
				hasReactive = hasReactive || edge.reactive
				next := edge.left
				if next == node {
					next = edge.right
				}
				if !visitedNodes[next] {
					visitedNodes[next] = true
					queue = append(queue, next)
				}
			}
		}
		if hasResistor && hasReactive && hasInput && hasOutput {
			return true
		}
	}
	for _, instance := range graph.Instances {
		if topologyInstanceHasPassiveTimeConstant(graph, instance) {
			return true
		}
	}
	return false
}

func topologyInstanceHasPassiveTimeConstant(graph CandidateGraph, active GraphInstance) bool {
	type passiveEdge struct {
		left     string
		right    string
		resistor bool
		reactive bool
	}
	externalSignals := map[string]bool{}
	internalNodes := map[string]bool{}
	nodeRoles := map[string]string{}
	for _, node := range graph.Nodes {
		nodeRoles[node.ID] = node.Role
		if node.Scope == "external" && (node.Role == "input" || node.Role == "output") {
			externalSignals[node.ID] = true
		}
		if node.Scope == "internal" {
			internalNodes[node.ID] = true
		}
	}
	edges := []passiveEdge{}
	adjacency := map[string][]int{}
	for _, instance := range graph.Instances {
		if len(instance.Terminals) != 2 {
			continue
		}
		edge := passiveEdge{
			left:  instance.Terminals[0].Node,
			right: instance.Terminals[1].Node,
		}
		switch instance.Kind {
		case "resistor":
			edge.resistor = true
		case "capacitor", "inductor":
			leftRole := nodeRoles[edge.left]
			rightRole := nodeRoles[edge.right]
			edge.reactive =
				leftRole == "internal" || leftRole == "output" ||
					rightRole == "internal" || rightRole == "output"
		default:
			continue
		}
		index := len(edges)
		edges = append(edges, edge)
		adjacency[edge.left] = append(adjacency[edge.left], index)
		adjacency[edge.right] = append(adjacency[edge.right], index)
	}
	inputs, outputs := topologyActiveSignalTerminals(active.Kind)
	if len(inputs) == 0 && len(outputs) == 0 {
		return false
	}
	{
		signalTerminals := append(append([]string(nil), inputs...), outputs...)
		terminalNodes := topologyTerminalNodes(active)
		for _, terminal := range signalTerminals {
			start := terminalNodes[terminal]
			if start == "" || !internalNodes[start] {
				continue
			}
			visitedNodes := map[string]bool{start: true}
			visitedEdges := map[int]bool{}
			queue := []string{start}
			hasResistor, hasReactive, hasExternalSignal := false, false, externalSignals[start]
			for len(queue) != 0 {
				node := queue[0]
				queue = queue[1:]
				for _, edgeIndex := range adjacency[node] {
					if visitedEdges[edgeIndex] {
						continue
					}
					visitedEdges[edgeIndex] = true
					edge := edges[edgeIndex]
					hasResistor = hasResistor || edge.resistor
					hasReactive = hasReactive || edge.reactive
					next := edge.left
					if next == node {
						next = edge.right
					}
					hasExternalSignal = hasExternalSignal || externalSignals[next]
					if topologyRailRole(nodeRoles[next]) {
						continue
					}
					if !visitedNodes[next] {
						visitedNodes[next] = true
						queue = append(queue, next)
					}
				}
			}
			if hasResistor && hasReactive && hasExternalSignal {
				return true
			}
		}
	}
	return false
}

func topologyGraphHasAnalogFeedback(graph CandidateGraph) bool {
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if topologyNodePathExists(dcAdjacency, terminals["OUT"], terminals["IN_MINUS"]) {
			return true
		}
	}
	return false
}

func topologyGraphHasDecisionFeedback(graph CandidateGraph) bool {
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" && instance.Kind != "comparator" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if (terminals["OUT"] != terminals["IN_PLUS"] &&
			topologyNodePathExists(dcAdjacency, terminals["OUT"], terminals["IN_PLUS"])) ||
			(terminals["OUT"] != terminals["IN_MINUS"] &&
				topologyNodePathExists(dcAdjacency, terminals["OUT"], terminals["IN_MINUS"])) {
			return true
		}
	}
	return false
}

// topologyDecisionStageGap recognizes only the primitive connectivity implied
// by bounded thresholds: an observed decision output, an externally driven
// input, and a rail-referenced decision level. Hysteretic behavior additionally
// requires a passive path from output back to an input. No named circuit family
// or preferred threshold equation participates in this structural ranking.
func topologyDecisionStageGap(
	graph CandidateGraph,
	minimumStages int,
	requireFeedback bool,
	requiredPolarity int,
) int {
	if minimumStages <= 0 {
		return 0
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	inputs := []string{}
	outputs := []string{}
	supplies := []string{}
	references := []string{}
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
		switch node.Role {
		case "input", "control":
			inputs = append(inputs, node.ID)
		case "output":
			outputs = append(outputs, node.ID)
		case "supply":
			supplies = append(supplies, node.ID)
		case "reference":
			references = append(references, node.ID)
		}
	}
	signalAdjacency := topologyPassiveNodeAdjacency(graph, true)
	decisionAdjacency := topologyDecisionCompositionAdjacency(graph, signalAdjacency)
	railAdjacency := topologyPassiveNodeAdjacencyWithRailAccess(graph, true, true)
	reachable := func(adjacency map[string][]string, start string, targets []string) bool {
		for _, target := range targets {
			if topologyNodePathExists(adjacency, start, target) {
				return true
			}
		}
		return false
	}
	const baseDecisionObligations = 5
	stageGaps := []int{}
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" && instance.Kind != "comparator" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		output := terminals["OUT"]
		bestGap := baseDecisionObligations
		if requireFeedback {
			bestGap++
		}
		for _, signalTerminal := range []string{"IN_PLUS", "IN_MINUS"} {
			if (requiredPolarity > 0 && signalTerminal != "IN_PLUS") ||
				(requiredPolarity < 0 && signalTerminal != "IN_MINUS") {
				continue
			}
			referenceTerminal := "IN_MINUS"
			if signalTerminal == "IN_MINUS" {
				referenceTerminal = "IN_PLUS"
			}
			signalNode := terminals[signalTerminal]
			referenceNode := terminals[referenceTerminal]
			reference := nodeByID[referenceNode]
			gap := 0
			if !reachable(decisionAdjacency, output, outputs) {
				gap++
			}
			signalValid := signalNode != "" &&
				referenceNode != "" &&
				signalNode != referenceNode &&
				(!requireFeedback || signalTerminal != "IN_PLUS" ||
					nodeByID[signalNode].Scope == "internal") &&
				(reference.Scope == "internal" ||
					reference.Role == "input" ||
					reference.Role == "control") &&
				(reference.Scope != "external" ||
					!topologyNodePathExists(signalAdjacency, signalNode, referenceNode))
			if !signalValid || !reachable(decisionAdjacency, signalNode, inputs) {
				gap++
			}
			if !signalValid {
				gap += 2
			} else if !topologyNodeHasDerivedReference(
				graph,
				referenceNode,
				supplies,
				references,
			) {
				if !reachable(railAdjacency, referenceNode, supplies) {
					gap++
				}
				if !reachable(railAdjacency, referenceNode, references) {
					gap++
				}
			}
			if requireFeedback &&
				(output == signalNode || output == referenceNode ||
					(signalTerminal == "IN_PLUS" &&
						!topologyNodePathExists(signalAdjacency, output, signalNode)) ||
					(signalTerminal == "IN_MINUS" &&
						!topologyNodePathExists(signalAdjacency, output, referenceNode))) {
				gap++
			}
			bestGap = min(bestGap, gap)
		}
		stageGaps = append(stageGaps, bestGap)
	}
	slices.Sort(stageGaps)
	missingStageGap := baseDecisionObligations
	if requireFeedback {
		missingStageGap++
	}
	gap := 0
	for index := 0; index < minimumStages; index++ {
		if index >= len(stageGaps) {
			gap += missingStageGap
		} else {
			gap += stageGaps[index]
		}
	}
	return gap
}

// topologyDecisionCompositionAdjacency augments passive connectivity with
// active causal paths so a decision feeding another decision or a controlled
// power device is recognized as a composed behavioral path. The composition
// view is intentionally undirected because topologyDecisionStageGap queries it
// from both external boundaries: forward from a stage output and backward from
// a stage input. Terminal polarity is checked per decision stage, while
// feedback validation remains on the passive-only graph and therefore still
// requires an explicit branch.
func topologyDecisionCompositionAdjacency(
	graph CandidateGraph,
	passive map[string][]string,
) map[string][]string {
	result := make(map[string][]string, len(passive))
	for node, neighbors := range passive {
		result[node] = append([]string(nil), neighbors...)
	}
	add := func(left, right string) {
		if left == "" || right == "" || left == right {
			return
		}
		result[left] = append(result[left], right)
		result[right] = append(result[right], left)
	}
	for _, instance := range graph.Instances {
		terminals := topologyTerminalNodes(instance)
		switch instance.Kind {
		case "comparator", "opamp":
			add(terminals["IN_PLUS"], terminals["OUT"])
			add(terminals["IN_MINUS"], terminals["OUT"])
		case "n_channel_mosfet", "p_channel_mosfet":
			add(terminals["GATE"], terminals["DRAIN"])
			add(terminals["GATE"], terminals["SOURCE"])
		case "npn_bjt", "pnp_bjt":
			add(terminals["BASE"], terminals["COLLECTOR"])
			add(terminals["BASE"], terminals["EMITTER"])
		}
	}
	for node, neighbors := range result {
		slices.Sort(neighbors)
		result[node] = slices.Compact(neighbors)
	}
	return result
}

// topologyNodeHasDerivedReference recognizes a rail-powered active stage whose
// input is established by a reviewed two-terminal reference primitive and
// whose feedback network is returned to the reference rail. This permits a
// decision threshold to be supply-invariant without treating an arbitrary
// active output as a trusted reference.
func topologyNodeHasDerivedReference(
	graph CandidateGraph,
	node string,
	supplies []string,
	references []string,
) bool {
	passive := topologyPassiveNodeAdjacencyWithRailAccess(graph, true, true)
	referenceSources := []string{}
	for _, instance := range graph.Instances {
		if instance.Kind != "reference_diode" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if terminals["CATHODE"] == "" || terminals["ANODE"] == "" {
			continue
		}
		for _, referenceNode := range references {
			if terminals["ANODE"] == referenceNode ||
				topologyNodePathExists(passive, terminals["ANODE"], referenceNode) {
				referenceSources = append(referenceSources, terminals["CATHODE"])
				break
			}
		}
	}
	if len(referenceSources) == 0 {
		return false
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if terminals["OUT"] != node ||
			!topologyAnyTerminalReachable(
				terminals,
				[]string{"V_PLUS"},
				supplies,
				passive,
			) ||
			!topologyAnyTerminalReachable(
				terminals,
				[]string{"V_MINUS"},
				references,
				passive,
			) ||
			!topologyAnyTerminalReachable(
				terminals,
				[]string{"IN_PLUS"},
				referenceSources,
				passive,
			) ||
			!topologyNodePathExists(
				passive,
				terminals["IN_MINUS"],
				terminals["OUT"],
			) {
			continue
		}
		for _, referenceNode := range references {
			if topologyNodePathExists(passive, terminals["IN_MINUS"], referenceNode) {
				return true
			}
		}
	}
	return false
}

// topologyDecisionPolarity derives comparator polarity only when bounded
// operating behavior is unambiguous: a low/high output requirement is paired
// with an input range wholly below or above every declared decision threshold.
// Conflicting or incomplete evidence deliberately leaves polarity unconstrained.
func topologyDecisionPolarity(requirement Requirement) int {
	threshold := math.Inf(1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "rising_threshold", "falling_threshold", "threshold_voltage",
			"lower_threshold", "upper_threshold":
			if target := assertionTarget(assertion); target > 0 {
				threshold = math.Min(threshold, target)
			}
		}
	}
	supply := nominalSupplyVoltage(requirement)
	if !finite(threshold) || supply <= 0 {
		return 0
	}
	cases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	polarity := 0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		outputLow := assertion.Metric == "output_low_voltage" ||
			assertion.Metric == "startup_output_voltage"
		outputHigh := assertion.Metric == "output_high_voltage"
		if outputLow {
			outputLow = assertion.Max != nil && *assertion.Max < supply/2
		}
		if outputHigh {
			outputHigh = assertion.Min != nil && *assertion.Min > supply/2
		}
		if !outputLow && !outputHigh {
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			for _, condition := range cases[caseID].Conditions {
				if condition.Axis != "input_voltage" {
					continue
				}
				candidate := 0
				switch {
				case condition.Max < threshold && outputLow:
					candidate = 1
				case condition.Max < threshold && outputHigh:
					candidate = -1
				case condition.Min > threshold && outputHigh:
					candidate = 1
				case condition.Min > threshold && outputLow:
					candidate = -1
				}
				if candidate == 0 {
					continue
				}
				if polarity != 0 && polarity != candidate {
					return 0
				}
				polarity = candidate
			}
		}
	}
	return polarity
}

func topologyPassiveNodeAdjacency(graph CandidateGraph, dcOnly bool) map[string][]string {
	return topologyPassiveNodeAdjacencyWithRailAccess(graph, dcOnly, false)
}

func topologyPassiveNodeAdjacencyWithRailAccess(
	graph CandidateGraph,
	dcOnly bool,
	includeRails bool,
) map[string][]string {
	adjacencySets := map[string]map[string]struct{}{}
	nodeRoles := make(map[string]string, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeRoles[node.ID] = node.Role
	}
	for _, instance := range graph.Instances {
		if len(instance.Terminals) != 2 {
			continue
		}
		switch instance.Kind {
		case "resistor", "inductor", "diode", "signal_diode", "clamp_diode":
		case "capacitor":
			if dcOnly {
				continue
			}
		default:
			continue
		}
		left := instance.Terminals[0].Node
		right := instance.Terminals[1].Node
		if left == right {
			continue
		}
		if !includeRails &&
			(topologyRailRole(nodeRoles[left]) || topologyRailRole(nodeRoles[right])) {
			continue
		}
		if adjacencySets[left] == nil {
			adjacencySets[left] = map[string]struct{}{}
		}
		if adjacencySets[right] == nil {
			adjacencySets[right] = map[string]struct{}{}
		}
		adjacencySets[left][right] = struct{}{}
		adjacencySets[right][left] = struct{}{}
	}
	adjacency := make(map[string][]string, len(adjacencySets))
	for node, neighbors := range adjacencySets {
		for neighbor := range neighbors {
			adjacency[node] = append(adjacency[node], neighbor)
		}
		slices.Sort(adjacency[node])
	}
	return adjacency
}

func topologyNodesByRole(graph CandidateGraph, roles ...string) []string {
	roleSet := map[string]bool{}
	for _, role := range roles {
		roleSet[role] = true
	}
	result := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && roleSet[node.Role] {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return result
}

func topologyRailRole(role string) bool {
	return role == "supply" || role == "reference"
}

func topologyActiveSignalTerminals(kind string) ([]string, []string) {
	switch kind {
	case "opamp":
		return []string{"IN_PLUS", "IN_MINUS"}, []string{"OUT"}
	case "fixed_voltage_regulator":
		return []string{"VIN"}, []string{"VOUT"}
	case "adjustable_voltage_regulator":
		return []string{"VIN", "ADJ"}, []string{"VOUT"}
	case "n_channel_mosfet", "p_channel_mosfet":
		return []string{"GATE"}, []string{"DRAIN", "SOURCE"}
	case "npn_bjt", "pnp_bjt":
		return []string{"BASE"}, []string{"COLLECTOR", "EMITTER"}
	default:
		return nil, nil
	}
}

func topologyEffectiveInputTerminals(
	instance GraphInstance,
	inputTerminals []string,
) []string {
	if instance.Kind != "opamp" {
		return inputTerminals
	}
	terminals := topologyTerminalNodes(instance)
	if terminals["OUT"] != "" && terminals["OUT"] == terminals["IN_MINUS"] {
		return []string{"IN_PLUS"}
	}
	return inputTerminals
}

func topologyTerminalNodes(instance GraphInstance) map[string]string {
	result := make(map[string]string, len(instance.Terminals))
	for _, terminal := range instance.Terminals {
		result[terminal.Terminal] = terminal.Node
	}
	return result
}

func topologyAnyTerminalReachable(
	terminalNodes map[string]string,
	terminals []string,
	targets []string,
	adjacency map[string][]string,
) bool {
	for _, terminal := range terminals {
		for _, target := range targets {
			if topologyNodePathExists(adjacency, terminalNodes[terminal], target) {
				return true
			}
		}
	}
	return false
}

func topologyMinimumTerminalDistance(
	terminalNodes map[string]string,
	terminals []string,
	targets []string,
	adjacency map[string][]string,
) int {
	best := 100
	for _, terminal := range terminals {
		start := terminalNodes[terminal]
		for _, target := range targets {
			best = min(best, topologyNodeDistance(adjacency, start, target))
		}
	}
	return best
}

func topologyNodeDistance(adjacency map[string][]string, start, target string) int {
	if start == "" || target == "" {
		return 100
	}
	if start == target {
		return 0
	}
	distances := map[string]int{start: 0}
	queue := []string{start}
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		nextDistance := distances[node] + 1
		for _, neighbor := range adjacency[node] {
			if _, visited := distances[neighbor]; visited {
				continue
			}
			if neighbor == target {
				return nextDistance
			}
			distances[neighbor] = nextDistance
			queue = append(queue, neighbor)
		}
	}
	return 100
}

func topologyNodePathExists(adjacency map[string][]string, start, target string) bool {
	if start == "" || target == "" {
		return false
	}
	if start == target {
		return true
	}
	visited := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		for _, neighbor := range adjacency[node] {
			if neighbor == target {
				return true
			}
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	return false
}

func compareTopologyScores(left, right TopologyScore) int {
	return cmp.Or(
		cmp.Compare(left.RedundantActive, right.RedundantActive),
		cmp.Compare(left.BehaviorGap, right.BehaviorGap),
		cmp.Compare(left.EndpointAccess, right.EndpointAccess),
		cmp.Compare(left.UnconnectedExternal, right.UnconnectedExternal),
		cmp.Compare(left.UnreachableOutputs, right.UnreachableOutputs),
		cmp.Compare(left.DanglingInternal, right.DanglingInternal),
		cmp.Compare(left.PrimitiveCount, right.PrimitiveCount),
		cmp.Compare(left.InternalNodeCount, right.InternalNodeCount),
		cmp.Compare(left.EvidencePenalty, right.EvidencePenalty),
		cmp.Compare(left.AreaMM2, right.AreaMM2),
		cmp.Compare(left.Fingerprint, right.Fingerprint),
	)
}

func compareTopologyCandidates(left, right TopologyCandidate) int {
	return cmp.Or(
		compareTopologyScores(left.Score, right.Score),
		cmp.Compare(left.TopologyHash, right.TopologyHash),
		cmp.Compare(left.Fingerprint, right.Fingerprint),
	)
}

func compareGraphOperations(left, right GraphOperation) int {
	if comparison := cmp.Or(
		cmp.Compare(left.Kind, right.Kind),
		cmp.Compare(left.PrimitiveKind, right.PrimitiveKind),
		cmp.Compare(left.PrimitiveKey, right.PrimitiveKey),
		cmp.Compare(left.Node, right.Node),
		cmp.Compare(canonicalOptionalFloat(left.ValueSI), canonicalOptionalFloat(right.ValueSI)),
		cmp.Compare(len(left.Connections), len(right.Connections)),
	); comparison != 0 {
		return comparison
	}
	for index := range left.Connections {
		if comparison := compareTerminalConnections(left.Connections[index], right.Connections[index]); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func primitiveEvidencePenalty(value string) int {
	switch value {
	case "verified":
		return 0
	case "library_derived":
		return 1
	case "rule_inferred":
		return 2
	default:
		return 10
	}
}

func comparePositiveArea(left, right float64) int {
	if left <= 0 && right > 0 {
		return 1
	}
	if right <= 0 && left > 0 {
		return -1
	}
	return cmp.Compare(left, right)
}

func effectiveTopologyPolicy(policy Policy) Policy {
	defaults := DefaultPolicy()
	if policy.MaxExpandedStates <= 0 {
		policy.MaxExpandedStates = defaults.MaxExpandedStates
	}
	if policy.MaxGeneratedGraphs <= 0 {
		policy.MaxGeneratedGraphs = defaults.MaxGeneratedGraphs
	}
	if policy.MaxPrimitiveInstances <= 0 {
		policy.MaxPrimitiveInstances = defaults.MaxPrimitiveInstances
	}
	if policy.MaxInternalNodes <= 0 {
		policy.MaxInternalNodes = defaults.MaxInternalNodes
	}
	if policy.MaxRetainedCandidates <= 0 {
		policy.MaxRetainedCandidates = defaults.MaxRetainedCandidates
	}
	if policy.MaxDiagnosticSamples <= 0 {
		policy.MaxDiagnosticSamples = defaults.MaxDiagnosticSamples
	}
	return policy
}

func primitiveInventoryByKey(inventory PrimitiveInventory) map[string]PrimitiveCandidate {
	result := make(map[string]PrimitiveCandidate, len(inventory.Primitives))
	for _, primitive := range inventory.Primitives {
		result[primitive.Key] = primitive
	}
	return result
}

func internalNodeCount(graph CandidateGraph) int {
	count := 0
	for _, node := range graph.Nodes {
		if node.Scope == "internal" {
			count++
		}
	}
	return count
}

func hasDanglingInternalNode(graph CandidateGraph) bool {
	degrees := map[string]int{}
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			degrees[terminal.Node]++
		}
	}
	for _, node := range graph.Nodes {
		if node.Scope == "internal" && degrees[node.ID] < 2 {
			return true
		}
	}
	return false
}

func normalizeSearchRejections(rejections map[string][]string) []SearchRejection {
	codes := make([]string, 0, len(rejections))
	for code := range rejections {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	result := make([]SearchRejection, 0, len(codes))
	for _, code := range codes {
		samples := append([]string(nil), rejections[code]...)
		slices.Sort(samples)
		samples = slices.Compact(samples)
		count := len(samples)
		if len(samples) > searchRejectionSampleLimit {
			samples = samples[:searchRejectionSampleLimit]
		}
		result = append(result, SearchRejection{Code: code, Count: count, Samples: samples})
	}
	return result
}

func comparePlacements(left, right []TerminalConnection) int {
	if comparison := cmp.Compare(len(left), len(right)); comparison != 0 {
		return comparison
	}
	for index := range left {
		if comparison := compareTerminalConnections(left[index], right[index]); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func cloneGraphOperations(source []GraphOperation) []GraphOperation {
	result := make([]GraphOperation, len(source))
	for index, operation := range source {
		result[index] = operation
		result[index].Connections = append([]TerminalConnection(nil), operation.Connections...)
		result[index].ValueSI = cloneInventoryFloat(operation.ValueSI)
	}
	return result
}

func minPositive(left, right int) int {
	switch {
	case left <= 0:
		return right
	case right <= 0:
		return left
	case left < right:
		return left
	default:
		return right
	}
}

func geometricMean(left, right float64) float64 {
	if left <= 0 || right <= 0 {
		return 0
	}
	return quantizeInventory(sqrtProduct(left, right))
}

func sqrtProduct(left, right float64) float64 {
	// Scaling avoids overflow while retaining deterministic IEEE-754 behavior.
	if left > right {
		left, right = right, left
	}
	return right * deterministicSqrt(left/right)
}

func deterministicSqrt(value float64) float64 {
	if value <= 0 {
		return 0
	}
	guess := value
	if guess < 1 {
		guess = 1
	}
	for range 24 {
		guess = 0.5 * (guess + value/guess)
	}
	return guess
}
