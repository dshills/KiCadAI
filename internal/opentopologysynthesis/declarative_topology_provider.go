package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"kicadai/internal/simmodel"
)

// declarativeTopologyProvider is a bounded, provider-neutral relationship
// constructor. Providers receive only the normalized behavioral contract,
// reviewed primitive evidence, and explicit graph/search limits. They cannot
// dispatch on project identity or introduce opaque topology payloads.
type declarativeTopologyProvider struct {
	id     string
	expand func(
		context.Context,
		Requirement,
		PrimitiveInventory,
		[]PrimitiveCandidate,
		map[string]PrimitiveCandidate,
		GraphLimits,
		Policy,
		topologySearchState,
	) ([]TopologyCandidate, Consumption, map[string][]string)
}

var declarativeTopologyProviders = []declarativeTopologyProvider{
	{id: "convergent_binary_decisions", expand: topologyConvergentBinaryDecisionSeeds},
}

func topologyDeclarativeProviderSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	retained := map[string]TopologyCandidate{}
	consumption := Consumption{}
	rejections := map[string][]string{}
	for _, provider := range declarativeTopologyProviders {
		if ctx.Err() != nil {
			break
		}
		remaining := policy
		remaining.MaxExpandedStates = max(0, policy.MaxExpandedStates-consumption.ExpandedStates)
		remaining.MaxGeneratedGraphs = max(0, policy.MaxGeneratedGraphs-consumption.GeneratedGraphs)
		if remaining.MaxExpandedStates == 0 || remaining.MaxGeneratedGraphs == 0 {
			consumption.BudgetExhausted = true
			break
		}
		candidates, used, rejected := provider.expand(
			ctx, requirement, inventory, representatives, inventoryByKey,
			limits, remaining, initial,
		)
		addSearchConsumption(&consumption, used)
		for code, samples := range rejected {
			for _, sample := range samples {
				rejections[code] = append(rejections[code], provider.id+":"+sample)
			}
		}
		for _, candidate := range candidates {
			if existing, found := retained[candidate.TopologyHash]; !found ||
				compareTopologyCandidates(candidate, existing) < 0 {
				retained[candidate.TopologyHash] = candidate
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

type declarativeBinaryDecision struct {
	excitation  string
	observation string
}

// topologyConvergentBinaryDecisionSeeds derives wired decision composition
// when independent digital inputs have bounded high/low obligations at one
// digital output. Bounded input resistors converge at one reviewed low-side
// level shifter for a high-side source, with explicit gate and output bias.
// The resulting graph implements composition explicitly instead of treating
// one unrelated active path as if it covered every excitation-to-observation
// relationship.
func topologyConvergentBinaryDecisionSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	groups := declarativeBinaryDecisionGroups(requirement)
	if len(groups) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	var levelShifter PrimitiveCandidate
	var source PrimitiveCandidate
	var endpointAccess PrimitiveCandidate
	for _, primitive := range representatives {
		if primitive.Kind == "npn_bjt" {
			levelShifter = primitive
		}
		if primitive.Kind == "p_channel_mosfet" {
			source = primitive
		}
		if primitive.Kind == "signal_diode" {
			endpointAccess = primitive
		}
	}
	if candidate := selectCurrentRelationshipPrimitiveMatching(
		requirement, inventory, "npn_bjt", false, false,
		func(primitive PrimitiveCandidate) bool {
			return primitiveModelParameter(primitive, simmodel.PrimitiveBJTNPNV1, "forward_beta") >= 100
		},
	); candidate.Key != "" {
		levelShifter = candidate
	}
	if rated := topologyRatedPowerPrimitive(requirement, inventory, "p_channel_mosfet"); rated.Key != "" {
		source = rated
	}
	inputResistance := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 100_000)
	gateBias := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 10_000)
	outputBias := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 2_200)
	if levelShifter.Key == "" || source.Key == "" || endpointAccess.Key == "" || inputResistance.Key == "" ||
		gateBias.Key == "" || outputBias.Key == "" {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"convergent binary decisions require reviewed level-shift, switching, signal-diode, and resistor evidence"},
		}
	}
	references := topologyNodesByRole(initial.graph, "reference")
	if len(references) != 1 {
		return nil, Consumption{}, map[string][]string{
			"relationship_gap": {"convergent binary decisions require one unambiguous reference"},
		}
	}
	results := []TopologyCandidate{}
	consumption := Consumption{}
	rejections := map[string][]string{}
	for _, decisions := range groups {
		if ctx.Err() != nil || len(decisions) < 2 ||
			consumption.ExpandedStates >= policy.MaxExpandedStates {
			break
		}
		consumption.ExpandedStates++
		output := "port_" + decisions[0].observation
		outputRail, ok := declarativeDomainSupplyNode(requirement, initial.graph, output)
		if !ok {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], "output domain lacks one unambiguous supply node")
			continue
		}
		inputRails := map[string]bool{}
		for _, decision := range decisions {
			rail, found := declarativeDomainSupplyNode(
				requirement, initial.graph, "port_"+decision.excitation,
			)
			if !found {
				inputRails = nil
				break
			}
			inputRails[rail] = true
		}
		if inputRails == nil {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], "input domain lacks one unambiguous supply node")
			continue
		}
		estimatedGraphs := 6 + len(decisions) + len(inputRails)
		if consumption.GeneratedGraphs+estimatedGraphs > policy.MaxGeneratedGraphs ||
			len(initial.graph.Instances)+4+len(decisions)+len(inputRails) > limits.MaxPrimitiveInstances ||
			internalNodeCount(initial.graph)+2 > limits.MaxInternalNodes {
			rejections["graph_limit"] = append(rejections["graph_limit"], "convergent binary decision graph exceeds an explicit bound")
			continue
		}
		// Graph mutation helpers clone graph slices and operation history before
		// extending them, so each decision group starts from an isolated value
		// copy of the immutable initial state.
		state := initial
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, outputBias,
			topologyTwoTerminalPlacement(output, references[0]), &consumption,
		)
		rails := make([]string, 0, len(inputRails))
		for rail := range inputRails {
			rails = append(rails, rail)
		}
		slices.Sort(rails)
		for _, rail := range rails {
			state = addRelationshipPrimitive(
				state, requirement, inventoryByKey, endpointAccess,
				[]TerminalConnection{
					{Terminal: "ANODE", Node: references[0]},
					{Terminal: "CATHODE", Node: rail},
				}, &consumption,
			)
		}
		var base, drive string
		state, base = addRelationshipInternalNode(
			state, requirement, inventoryByKey, &consumption,
		)
		state, drive = addRelationshipInternalNode(
			state, requirement, inventoryByKey, &consumption,
		)
		if base == "" || drive == "" {
			return results, consumption, rejections
		}
		for _, decision := range decisions {
			input := "port_" + decision.excitation
			state = addRelationshipPrimitive(
				state, requirement, inventoryByKey, inputResistance,
				topologyTwoTerminalPlacement(input, base), &consumption,
			)
		}
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, gateBias,
			topologyTwoTerminalPlacement(drive, outputRail), &consumption,
		)
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, levelShifter,
			[]TerminalConnection{
				{Terminal: "BASE", Node: base},
				{Terminal: "COLLECTOR", Node: drive},
				{Terminal: "EMITTER", Node: references[0]},
			}, &consumption,
		)
		state = addRelationshipPrimitive(
			state, requirement, inventoryByKey, source,
			[]TerminalConnection{
				{Terminal: "GATE", Node: drive},
				{Terminal: "DRAIN", Node: output},
				{Terminal: "SOURCE", Node: outputRail},
			}, &consumption,
		)
		if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			continue
		}
		if state.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap))
			continue
		}
		normalized, err := NormalizeGraph(state.graph)
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
		results = append(results, TopologyCandidate{
			Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score,
			Graph: normalized, Operations: cloneGraphOperations(state.operations),
		})
	}
	if consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
		consumption.BudgetExhausted = true
	}
	slices.SortFunc(results, compareTopologyCandidates)
	return results, consumption, rejections
}

func declarativeBinaryDecisionGroups(requirement Requirement) [][]declarativeBinaryDecision {
	type outputLevels struct {
		high bool
		low  bool
	}
	portByID := map[string]Port{}
	for _, port := range requirement.Requirements.Ports {
		portByID[port.ID] = port
	}
	byOutput := map[string]map[string]declarativeBinaryDecision{}
	levelsByOutput := map[string]outputLevels{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Excitation == nil || assertion.Excitation.Kind != "port" ||
			assertion.Observation.Kind != "port" {
			continue
		}
		switch assertion.Metric {
		case "output_high_voltage":
		case "output_low_voltage":
		default:
			continue
		}
		input, inputOK := portByID[assertion.Excitation.ID]
		output, outputOK := portByID[assertion.Observation.ID]
		if !inputOK || !outputOK || input.Kind != "digital" || input.Direction != "sink" ||
			output.Kind != "digital" || output.Direction != "source" {
			continue
		}
		if byOutput[output.ID] == nil {
			byOutput[output.ID] = map[string]declarativeBinaryDecision{}
		}
		levels := levelsByOutput[output.ID]
		levels.high = levels.high || assertion.Metric == "output_high_voltage"
		levels.low = levels.low || assertion.Metric == "output_low_voltage"
		levelsByOutput[output.ID] = levels
		byOutput[output.ID][input.ID] = declarativeBinaryDecision{
			excitation: input.ID, observation: output.ID,
		}
	}
	outputs := make([]string, 0, len(byOutput))
	for output := range byOutput {
		outputs = append(outputs, output)
	}
	slices.Sort(outputs)
	groups := [][]declarativeBinaryDecision{}
	for _, output := range outputs {
		decisions := []declarativeBinaryDecision{}
		for _, decision := range byOutput[output] {
			decisions = append(decisions, decision)
		}
		slices.SortFunc(decisions, func(left, right declarativeBinaryDecision) int {
			return cmp.Compare(left.excitation, right.excitation)
		})
		levels := levelsByOutput[output]
		if len(decisions) >= 2 && levels.high && levels.low {
			groups = append(groups, decisions)
		}
	}
	return groups
}

func declarativeDomainSupplyNode(requirement Requirement, graph CandidateGraph, portNode string) (string, bool) {
	port, found := graphNodeByID(graph, portNode)
	if !found || port.SemanticKind != "port" {
		return "", false
	}
	candidates := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "supply" && node.Domain == port.Domain {
			candidates = append(candidates, node.ID)
		}
	}
	slices.Sort(candidates)
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0], true
}
