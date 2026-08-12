package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"slices"
)

type multiControlObligation struct {
	outputID    string
	controlID   string
	requirement Requirement
}

func requirementPortIsIndependentControl(ports map[string]Port, id string) bool {
	port, found := ports[id]
	if !found || port.Direction == "source" {
		return false
	}
	return port.Kind != "power" && port.Kind != "reference"
}

func topologyMultiControlCompositionSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	obligations := multiControlObligations(requirement)
	if len(obligations) < 2 {
		return nil, Consumption{}, map[string][]string{}
	}
	maximumCombinations := max(1, policy.MaxRetainedCandidates*multiOutputCombinationRetentionMultiplier)
	candidateBreadth := multiOutputCandidateBreadth(policy.MaxRetainedCandidates, len(obligations), maximumCombinations)
	consumption := Consumption{}
	rejections := map[string][]string{}
	candidatesByObligation := make([][]TopologyCandidate, 0, len(obligations))
	for obligationIndex, obligation := range obligations {
		if ctx.Err() != nil {
			return nil, consumption, rejections
		}
		remainingSubsearches := len(obligations) - obligationIndex
		if remainingSubsearches <= 0 {
			break
		}
		remainingExpanded := policy.MaxExpandedStates - consumption.ExpandedStates
		remainingGenerated := policy.MaxGeneratedGraphs - consumption.GeneratedGraphs
		if remainingExpanded <= 0 || remainingGenerated <= 0 {
			consumption.BudgetExhausted = true
			return nil, consumption, rejections
		}
		subsearchPolicy := policy
		subsearchPolicy.MaxExpandedStates = max(1, remainingExpanded/remainingSubsearches)
		subsearchPolicy.MaxGeneratedGraphs = max(1, remainingGenerated/remainingSubsearches)
		subsearchPolicy.MaxRetainedCandidates = min(candidateBreadth, policy.MaxRetainedCandidates)
		search := searchPrimitiveTopologies(ctx, obligation.requirement, inventory, subsearchPolicy, topologyCompositionNone)
		addSearchConsumption(&consumption, search.Consumption)
		if len(search.Candidates) == 0 {
			rejections["multi_control_subsearch"] = append(
				rejections["multi_control_subsearch"],
				obligation.outputID+":"+obligation.controlID+":"+string(search.Status),
			)
			return nil, consumption, rejections
		}
		limit := min(candidateBreadth, len(search.Candidates))
		candidatesByObligation = append(candidatesByObligation, append([]TopologyCandidate(nil), search.Candidates[:limit]...))
	}

	combinations := [][]TopologyCandidate{{}}
	for _, candidates := range candidatesByObligation {
		next := make([][]TopologyCandidate, 0, maximumCombinations)
		for _, combination := range combinations {
			for _, candidate := range candidates {
				nextCombination := append(append([]TopologyCandidate(nil), combination...), candidate)
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
	isolatedOutputs := make([]string, len(obligations))
	for index, obligation := range obligations {
		isolatedOutputs[index] = obligation.outputID
	}
	for combinationIndex, combination := range combinations {
		state, ok := mergeOutputTopologyCandidates(
			requirement, initial, combination, isolatedOutputs,
			inventory, inventoryByKey, limits, &consumption,
		)
		if !ok {
			rejections["multi_control_merge"] = append(
				rejections["multi_control_merge"],
				fmt.Sprintf("combination_%03d", combinationIndex),
			)
			continue
		}
		states = append(states, state)
	}
	if len(states) == 0 {
		return nil, consumption, rejections
	}
	completionPolicy := policy
	completionPolicy.MaxExpandedStates = max(1, policy.MaxExpandedStates-consumption.ExpandedStates)
	completionPolicy.MaxGeneratedGraphs = max(1, policy.MaxGeneratedGraphs-consumption.GeneratedGraphs)
	return completeComposedTopologyStates(
		ctx, requirement, inventory, representatives, inventoryByKey, limits,
		completionPolicy, states, &consumption, rejections,
	)
}

func multiControlObligations(requirement Requirement) []multiControlObligation {
	requirement = Normalize(requirement)
	ports := make(map[string]Port, len(requirement.Requirements.Ports))
	sourceOutputs := map[string]bool{}
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
		if port.Direction == "source" {
			sourceOutputs[port.ID] = true
		}
	}
	assertedOutputs := map[string]bool{}
	byOutputControl := map[string]map[string][]BehavioralAssertion{}
	assertionsByOutput := map[string][]BehavioralAssertion{}
	sharedByControl := map[string][]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind != "port" || !sourceOutputs[assertion.Observation.ID] {
			if assertion.Excitation != nil && assertion.Excitation.Kind == "port" &&
				requirementPortIsIndependentControl(ports, assertion.Excitation.ID) {
				sharedByControl[assertion.Excitation.ID] = append(sharedByControl[assertion.Excitation.ID], assertion)
			} else if assertion.Observation.Kind == "port" && requirementPortIsIndependentControl(ports, assertion.Observation.ID) {
				sharedByControl[assertion.Observation.ID] = append(sharedByControl[assertion.Observation.ID], assertion)
			}
			continue
		}
		assertedOutputs[assertion.Observation.ID] = true
		assertionsByOutput[assertion.Observation.ID] = append(assertionsByOutput[assertion.Observation.ID], assertion)
		if assertion.Excitation == nil || assertion.Excitation.Kind != "port" ||
			!requirementPortIsIndependentControl(ports, assertion.Excitation.ID) {
			continue
		}
		if byOutputControl[assertion.Observation.ID] == nil {
			byOutputControl[assertion.Observation.ID] = map[string][]BehavioralAssertion{}
		}
		controlID := assertion.Excitation.ID
		byOutputControl[assertion.Observation.ID][controlID] = append(byOutputControl[assertion.Observation.ID][controlID], assertion)
	}
	// Independent outputs are composed by the outer multi-output layer. A
	// control-only subsearch must never silently discard another source output.
	if len(assertedOutputs) != 1 {
		return nil
	}
	outputIDs := make([]string, 0, len(byOutputControl))
	for outputID, controls := range byOutputControl {
		if len(controls) >= 2 {
			outputIDs = append(outputIDs, outputID)
		}
	}
	slices.Sort(outputIDs)
	result := []multiControlObligation{}
	for _, outputID := range outputIDs {
		controlIDs := make([]string, 0, len(byOutputControl[outputID]))
		for controlID := range byOutputControl[outputID] {
			controlIDs = append(controlIDs, controlID)
		}
		slices.Sort(controlIDs)
		for _, controlID := range controlIDs {
			assertions := append([]BehavioralAssertion(nil), byOutputControl[outputID][controlID]...)
			for _, assertion := range assertionsByOutput[outputID] {
				if assertion.Excitation == nil {
					assertions = append(assertions, assertion)
				}
			}
			assertions = append(assertions, sharedByControl[controlID]...)
			slices.SortFunc(assertions, func(left, right BehavioralAssertion) int {
				return cmp.Compare(left.ID, right.ID)
			})
			assertions = slices.CompactFunc(assertions, func(left, right BehavioralAssertion) bool {
				return left.ID == right.ID
			})
			subRequirement, ok := multiControlSubRequirement(requirement, outputID, controlID, assertions)
			if !ok {
				return nil
			}
			result = append(result, multiControlObligation{outputID: outputID, controlID: controlID, requirement: subRequirement})
		}
	}
	return result
}

func multiControlSubRequirement(
	requirement Requirement,
	outputID string,
	controlID string,
	assertions []BehavioralAssertion,
) (Requirement, bool) {
	portIDs := map[string]bool{outputID: true, controlID: true}
	domainIDs := map[string]bool{}
	caseIDs := map[string]bool{}
	for _, assertion := range assertions {
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
		if portIDs[port.ID] {
			ports = append(ports, port)
			domainIDs[port.Domain] = true
		}
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
		filtered.Conditions = slices.DeleteFunc(append([]OperatingCondition(nil), operatingCase.Conditions...), func(condition OperatingCondition) bool {
			return !portIDs[condition.Target] && !domainIDs[condition.Target]
		})
		filtered.Events = slices.DeleteFunc(append([]OperatingEvent(nil), operatingCase.Events...), func(event OperatingEvent) bool {
			return !portIDs[event.Target]
		})
		operatingCases = append(operatingCases, filtered)
	}
	subRequirement := requirement
	subRequirement.Requirements.Domains = domains
	subRequirement.Requirements.Ports = ports
	subRequirement.Requirements.OperatingCases = operatingCases
	subRequirement.Requirements.BehavioralRequirements = append([]BehavioralAssertion(nil), assertions...)
	subRequirement = Normalize(subRequirement)
	return subRequirement, len(Validate(subRequirement)) == 0
}
