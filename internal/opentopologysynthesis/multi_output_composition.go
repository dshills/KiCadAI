package opentopologysynthesis

import (
	"container/heap"
	"context"
	"fmt"
	"slices"
)

const multiOutputCombinationRetentionMultiplier = 2

type multiOutputObligation struct {
	outputID    string
	requirement Requirement
}

func topologyMultiOutputCompositionSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	obligations := multiOutputObligations(requirement)
	if len(obligations) < 2 {
		return nil, Consumption{}, map[string][]string{}
	}

	consumption := Consumption{}
	rejections := map[string][]string{}
	maximumCombinations := max(
		1,
		policy.MaxRetainedCandidates*multiOutputCombinationRetentionMultiplier,
	)
	candidateBreadth := multiOutputCandidateBreadth(
		policy.MaxRetainedCandidates,
		len(obligations),
		maximumCombinations,
	)
	perObligationPolicy := policy
	perObligationPolicy.MaxExpandedStates = max(1, policy.MaxExpandedStates/(len(obligations)+2))
	perObligationPolicy.MaxGeneratedGraphs = max(1, policy.MaxGeneratedGraphs/(len(obligations)+2))
	perObligationPolicy.MaxRetainedCandidates = min(
		candidateBreadth,
		policy.MaxRetainedCandidates,
	)
	candidatesByObligation := make([][]TopologyCandidate, 0, len(obligations))
	for _, obligation := range obligations {
		if ctx.Err() != nil {
			return nil, consumption, rejections
		}
		search := searchPrimitiveTopologies(
			ctx,
			obligation.requirement,
			inventory,
			perObligationPolicy,
			false,
		)
		addSearchConsumption(&consumption, search.Consumption)
		if len(search.Candidates) == 0 {
			rejections["multi_output_subsearch"] = append(
				rejections["multi_output_subsearch"],
				obligation.outputID+":"+string(search.Status),
			)
			return nil, consumption, rejections
		}
		limit := min(candidateBreadth, len(search.Candidates))
		candidatesByObligation = append(
			candidatesByObligation,
			append([]TopologyCandidate(nil), search.Candidates[:limit]...),
		)
	}

	combinations := [][]TopologyCandidate{{}}
	for _, candidates := range candidatesByObligation {
		next := [][]TopologyCandidate{}
		for _, combination := range combinations {
			for _, candidate := range candidates {
				nextCombination := append([]TopologyCandidate(nil), combination...)
				nextCombination = append(nextCombination, candidate)
				next = append(next, nextCombination)
				if len(next) >= maximumCombinations {
					break
				}
			}
			if len(next) >= maximumCombinations {
				break
			}
		}
		combinations = next
	}

	states := []topologySearchState{}
	for combinationIndex, combination := range combinations {
		state, ok := mergeOutputTopologyCandidates(
			requirement,
			initial,
			combination,
			inventory,
			inventoryByKey,
			limits,
			&consumption,
		)
		if !ok {
			rejections["multi_output_merge"] = append(
				rejections["multi_output_merge"],
				fmt.Sprintf("combination_%03d", combinationIndex),
			)
			continue
		}
		states = append(states, state)
	}
	if len(states) == 0 {
		return nil, consumption, rejections
	}

	return completeComposedTopologyStates(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		policy,
		states,
		&consumption,
		rejections,
	)
}

func multiOutputCandidateBreadth(maxRetainedCandidates, obligationCount, maximumCombinations int) int {
	maximumBreadth := max(1, maxRetainedCandidates)
	maximumCombinations = max(1, maximumCombinations)
	obligationCount = max(1, obligationCount)
	breadth := 1
	for breadth < maximumBreadth && multiOutputCombinationCountFits(
		breadth+1,
		obligationCount,
		maximumCombinations,
	) {
		breadth++
	}
	return breadth
}

func multiOutputCombinationCountFits(breadth, obligationCount, limit int) bool {
	combinations := 1
	for range obligationCount {
		if combinations > limit/breadth {
			return false
		}
		combinations *= breadth
	}
	return true
}

func multiOutputObligations(requirement Requirement) []multiOutputObligation {
	requirement = Normalize(requirement)
	sourceOutputs := map[string]bool{}
	for _, port := range requirement.Requirements.Ports {
		if port.Direction == "source" {
			sourceOutputs[port.ID] = true
		}
	}
	assertionsByOutput := map[string][]BehavioralAssertion{}
	sharedAssertions := []BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind == "port" && sourceOutputs[assertion.Observation.ID] {
			assertionsByOutput[assertion.Observation.ID] = append(
				assertionsByOutput[assertion.Observation.ID],
				assertion,
			)
			continue
		}
		sharedAssertions = append(sharedAssertions, assertion)
	}
	if len(assertionsByOutput) < 2 {
		return nil
	}

	outputIDs := make([]string, 0, len(assertionsByOutput))
	for outputID := range assertionsByOutput {
		outputIDs = append(outputIDs, outputID)
	}
	slices.Sort(outputIDs)
	result := make([]multiOutputObligation, 0, len(outputIDs))
	for _, outputID := range outputIDs {
		assertions := append([]BehavioralAssertion(nil), assertionsByOutput[outputID]...)
		assertions = append(assertions, sharedAssertions...)
		portIDs := map[string]bool{outputID: true}
		domainIDs := map[string]bool{}
		caseIDs := map[string]bool{}
		for _, assertion := range assertions {
			if assertion.Excitation != nil {
				switch assertion.Excitation.Kind {
				case "port":
					portIDs[assertion.Excitation.ID] = true
				case "domain":
					domainIDs[assertion.Excitation.ID] = true
				}
			}
			switch assertion.Observation.Kind {
			case "port":
				portIDs[assertion.Observation.ID] = true
			case "domain":
				domainIDs[assertion.Observation.ID] = true
			}
			for _, caseID := range assertion.OperatingCases {
				caseIDs[caseID] = true
			}
		}
		for _, port := range requirement.Requirements.Ports {
			if port.Kind == "power" || port.Kind == "reference" {
				portIDs[port.ID] = true
			}
		}

		ports := []Port{}
		for _, port := range requirement.Requirements.Ports {
			if !portIDs[port.ID] {
				continue
			}
			ports = append(ports, port)
			domainIDs[port.Domain] = true
		}
		domains := []Domain{}
		for _, domain := range requirement.Requirements.Domains {
			if domainIDs[domain.ID] {
				domains = append(domains, domain)
			}
		}
		operatingCases := []OperatingCase{}
		for _, operatingCase := range requirement.Requirements.OperatingCases {
			if !caseIDs[operatingCase.ID] {
				continue
			}
			filtered := operatingCase
			filtered.Conditions = slices.DeleteFunc(
				append([]OperatingCondition(nil), operatingCase.Conditions...),
				func(condition OperatingCondition) bool {
					return !portIDs[condition.Target] && !domainIDs[condition.Target]
				},
			)
			filtered.Events = slices.DeleteFunc(
				append([]OperatingEvent(nil), operatingCase.Events...),
				func(event OperatingEvent) bool { return !portIDs[event.Target] },
			)
			operatingCases = append(operatingCases, filtered)
		}

		subRequirement := requirement
		subRequirement.Requirements.Domains = domains
		subRequirement.Requirements.Ports = ports
		subRequirement.Requirements.OperatingCases = operatingCases
		subRequirement.Requirements.BehavioralRequirements = assertions
		subRequirement = Normalize(subRequirement)
		if len(Validate(subRequirement)) != 0 {
			return nil
		}
		result = append(result, multiOutputObligation{
			outputID:    outputID,
			requirement: subRequirement,
		})
	}
	return result
}

func mergeOutputTopologyCandidates(
	requirement Requirement,
	initial topologySearchState,
	candidates []TopologyCandidate,
	inventory PrimitiveInventory,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	consumption *Consumption,
) (topologySearchState, bool) {
	state := initial
	state.graph = CloneGraph(initial.graph)
	state.operations = cloneGraphOperations(initial.operations)
	type externalNodeIdentity struct {
		semanticKind string
		semanticID   string
		domain       string
		role         string
	}
	externalNodes := map[externalNodeIdentity]string{}
	for _, node := range state.graph.Nodes {
		if node.Scope == "external" {
			identity := externalNodeIdentity{
				semanticKind: node.SemanticKind,
				semanticID:   node.SemanticID,
				domain:       node.Domain,
				role:         node.Role,
			}
			if _, duplicate := externalNodes[identity]; duplicate {
				return topologySearchState{}, false
			}
			externalNodes[identity] = node.ID
		}
	}
	for _, candidate := range candidates {
		nodeMapping := map[string]string{}
		for _, node := range candidate.Graph.Nodes {
			if node.Scope == "external" {
				identity := externalNodeIdentity{
					semanticKind: node.SemanticKind,
					semanticID:   node.SemanticID,
					domain:       node.Domain,
					role:         node.Role,
				}
				mappedNode, found := externalNodes[identity]
				if !found {
					return topologySearchState{}, false
				}
				nodeMapping[node.ID] = mappedNode
				continue
			}
			var generatedNode string
			state, generatedNode = addRelationshipInternalNode(
				state,
				requirement,
				inventoryByKey,
				consumption,
			)
			if generatedNode == "" {
				return topologySearchState{}, false
			}
			nodeMapping[node.ID] = generatedNode
		}
		for _, instance := range candidate.Graph.Instances {
			primitive, found := inventoryByKey[instance.PrimitiveKey]
			if !found {
				return topologySearchState{}, false
			}
			connections := make([]TerminalConnection, 0, len(instance.Terminals))
			for _, connection := range instance.Terminals {
				node, found := nodeMapping[connection.Node]
				if !found {
					return topologySearchState{}, false
				}
				connections = append(connections, TerminalConnection{
					Terminal: connection.Terminal,
					Node:     node,
				})
			}
			state = addRelationshipPrimitiveWithValue(
				state,
				requirement,
				inventoryByKey,
				primitive,
				instance.ValueSI,
				connections,
				consumption,
			)
		}
	}
	if len(ValidatePartialGraph(state.graph, inventory, limits)) != 0 {
		return topologySearchState{}, false
	}
	graphHash, err := GraphHash(state.graph)
	if err != nil {
		return topologySearchState{}, false
	}
	state.hash = graphHash
	if len(state.operations) != 0 {
		state.operations[len(state.operations)-1].AfterHash = graphHash
	}
	topologyHash, err := TopologyHash(state.graph)
	if err != nil {
		return topologySearchState{}, false
	}
	state.topology = topologyHash
	state.score = scoreTopologyGraphWithPowerRequirements(
		requirement, state.graph, inventoryByKey, state.hash, state.powerRequirements,
	)
	return state, true
}

func completeComposedTopologyStates(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	states []topologySearchState,
	consumption *Consumption,
	rejections map[string][]string,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	frontier := &topologyFrontier{}
	heapInit := false
	visited := map[string]bool{}
	dominantTopology := map[string]topologySearchState{}
	for _, state := range states {
		if visited[state.hash] {
			continue
		}
		visited[state.hash] = true
		dominantTopology[state.topology] = state
		*frontier = append(*frontier, state)
		heapInit = true
	}
	if heapInit {
		heap.Init(frontier)
	}
	retained := map[string]TopologyCandidate{}
	exploreTopologyFrontier(
		ctx,
		requirement,
		inventory,
		representatives,
		inventoryByKey,
		limits,
		policy,
		frontier,
		visited,
		dominantTopology,
		retained,
		rejections,
		consumption,
	)
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, *consumption, rejections
}

func finalizeComposedTopologySearchResult(
	result TopologySearchResult,
	candidates []TopologyCandidate,
	rejections map[string][]string,
) TopologySearchResult {
	result.Candidates = finalizeTopologyCandidates(
		candidates,
		result.Policy.MaxRetainedCandidates,
		rejections,
	)
	result.Rejections = normalizeSearchRejections(rejections)
	if len(result.Candidates) != 0 {
		result.Status = TopologySearchCandidates
		result.Issues = nil
		return result
	}
	result.Status = TopologySearchExhausted
	return result
}
