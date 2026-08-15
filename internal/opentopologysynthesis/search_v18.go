package opentopologysynthesis

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

// SearchPrimitiveTopologiesV18 applies the bounded V18 analog extension while
// leaving SearchPrimitiveTopologies and every historical replay path intact.
func SearchPrimitiveTopologiesV18(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	policy Policy,
) TopologySearchResult {
	original := Normalize(requirement)
	originalHash, err := CanonicalHash(original)
	if err != nil {
		return TopologySearchResult{
			Schema: TopologySearchSchema, Version: TopologySearchVersion,
			PolicyVersion: PolicyVersion, Status: TopologySearchFailed,
			Issues: []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash V18 requirement: "+err.Error(), "")},
		}
	}
	searchRequirement := v18ThresholdWindowRequirement(original)
	// Primitive compatibility is derived only from authored observations. The
	// synthetic window-state assertions guide topology construction and must not
	// tighten the component envelope beyond the original contract.
	searchInventory := v18CompatiblePrimitiveInventory(original, inventory)
	var result TopologySearchResult
	if len(multiOutputObligations(searchRequirement)) >= 2 {
		result = searchPrimitiveTopologiesV18MultiOutput(ctx, searchRequirement, searchInventory, policy)
	} else {
		result = SearchPrimitiveTopologies(ctx, searchRequirement, searchInventory, policy)
	}
	inventoryByKey := primitiveInventoryByKey(searchInventory)
	limits := GraphLimits{
		MaxPrimitiveInstances: minPositive(result.Policy.MaxPrimitiveInstances, searchRequirement.Requirements.Constraints.MaxComponents),
		MaxInternalNodes:      result.Policy.MaxInternalNodes,
	}
	isolated := make([]TopologyCandidate, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		state := topologySearchState{
			graph: CloneGraph(candidate.Graph), hash: candidate.Fingerprint,
			score: candidate.Score, operations: cloneGraphOperations(candidate.Operations),
			powerRequirements: deriveTopologyPowerRequirementProfile(searchRequirement),
		}
		state, reason := v18IsolateContendedActiveOutputs(
			searchRequirement, state, searchInventory, inventoryByKey, limits, &result.Consumption,
		)
		if reason != "" {
			continue
		}
		normalized, normalizeErr := NormalizeGraph(state.graph)
		if normalizeErr != nil {
			continue
		}
		topologyHash, topologyErr := TopologyHash(normalized)
		if topologyErr != nil {
			continue
		}
		isolated = append(isolated, TopologyCandidate{
			Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score,
			Graph: normalized, Operations: cloneGraphOperations(state.operations),
		})
	}
	result.Candidates = isolated
	result.RequirementHash = originalHash
	result.InventoryHash = inventory.Hash
	result.Candidates = slices.DeleteFunc(result.Candidates, func(candidate TopologyCandidate) bool {
		return v18TopologyInputImpedanceGap(original, candidate.Graph) != 0
	})
	if len(result.Candidates) == 0 && result.Status == TopologySearchCandidates {
		result.Status = TopologySearchExhausted
		result.Issues = []reports.Issue{graphIssue(
			CodeNoCompleteGraph,
			"search.candidates",
			"V18 rejected every complete topology because it violated a declared input-impedance floor",
			"retain only series input paths or rail shunts at or above the declared impedance",
		)}
	}
	return result
}

func searchPrimitiveTopologiesV18MultiOutput(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	policy Policy,
) TopologySearchResult {
	result := TopologySearchResult{
		Schema: TopologySearchSchema, Version: TopologySearchVersion,
		PolicyVersion: PolicyVersion, InventoryHash: inventory.Hash,
		Policy: effectiveTopologyPolicy(policy), Status: TopologySearchFailed,
		Candidates: []TopologyCandidate{}, Rejections: []SearchRejection{}, Issues: []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash V18 topology requirement: "+err.Error(), "")}
		return result
	}
	result.RequirementHash = requirementHash
	if issues := Validate(requirement); len(issues) != 0 {
		result.Issues = issues
		return result
	}
	if len(inventory.Primitives) == 0 || len(inventory.Hash) != 64 {
		result.Status = TopologySearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePrimitiveUnavailable, "inventory", "V18 topology search requires a nonempty hash-bound primitive inventory", "build the reviewed V18 primitive inventory")}
		return result
	}
	representatives := topologyRepresentatives(requirement, inventory)
	if len(representatives) == 0 {
		result.Status = TopologySearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePrimitiveUnavailable, "inventory.primitives", "no V18 primitive candidates cover the requested analysis envelope", "onboard compatible reviewed primitive evidence")}
		return result
	}
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		result.Issues = issues
		return result
	}
	initialHash, err := GraphHash(initial)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "initial_graph", "hash V18 initial graph: "+err.Error(), "")}
		return result
	}
	initialTopology, err := TopologyHash(initial)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "initial_graph", "hash V18 initial graph topology: "+err.Error(), "")}
		return result
	}
	inventoryByKey := primitiveInventoryByKey(inventory)
	limits := GraphLimits{
		MaxPrimitiveInstances: minPositive(result.Policy.MaxPrimitiveInstances, requirement.Requirements.Constraints.MaxComponents),
		MaxInternalNodes:      result.Policy.MaxInternalNodes,
	}
	powerRequirements := deriveTopologyPowerRequirementProfile(requirement)
	initialState := topologySearchState{
		graph: initial, hash: initialHash, topology: initialTopology,
		score:             scoreTopologyGraphWithPowerRequirements(requirement, initial, inventoryByKey, initialHash, powerRequirements),
		powerRequirements: powerRequirements,
	}
	rejections := map[string][]string{}
	candidates, consumption, compositionRejections := topologyMultiOutputCompositionSeedsV18(
		ctx, requirement, inventory, representatives, inventoryByKey, limits, result.Policy, initialState,
	)
	result.Consumption = consumption
	for code, samples := range compositionRejections {
		rejections[code] = append(rejections[code], samples...)
	}
	return finalizeComposedTopologySearchResult(result, candidates, rejections)
}

func topologyMultiOutputCompositionSeedsV18(
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
	maximumCombinations := max(1, policy.MaxRetainedCandidates*multiOutputCombinationRetentionMultiplier)
	candidateBreadth := multiOutputCandidateBreadth(policy.MaxRetainedCandidates, len(obligations), maximumCombinations)
	perObligationPolicy := policy
	perObligationPolicy.MaxExpandedStates = max(1, policy.MaxExpandedStates/(len(obligations)+2))
	perObligationPolicy.MaxGeneratedGraphs = max(1, policy.MaxGeneratedGraphs/(len(obligations)+2))
	// Filter electrically impossible input shunts before reducing each
	// obligation to the Cartesian-combination breadth. The caller's retained
	// candidate bound remains authoritative and the final combination bound is
	// unchanged.
	perObligationPolicy.MaxRetainedCandidates = policy.MaxRetainedCandidates
	candidatesByObligation := make([][]TopologyCandidate, 0, len(obligations))
	for _, obligation := range obligations {
		if ctx.Err() != nil {
			return nil, consumption, rejections
		}
		search := searchPrimitiveTopologies(ctx, obligation.requirement, inventory, perObligationPolicy, topologyCompositionControlsOnly)
		addSearchConsumption(&consumption, search.Consumption)
		compatible := make([]TopologyCandidate, 0, len(search.Candidates))
		for _, candidate := range search.Candidates {
			candidate, polarityReason := v18RepairPositiveFollowerPolarity(
				obligation.requirement, candidate, inventory, inventoryByKey, limits, &consumption,
			)
			if polarityReason != "" {
				rejections["v18_follower_"+polarityReason] = append(
					rejections["v18_follower_"+polarityReason], obligation.outputID,
				)
				continue
			}
			if v18TopologyInputImpedanceGap(obligation.requirement, candidate.Graph) != 0 {
				repaired, repairReason := v18RepairInputAccess(
					obligation.requirement, candidate, inventory, inventoryByKey, limits, &consumption,
				)
				if repairReason != "" {
					rejections["v18_input_access_"+repairReason] = append(
						rejections["v18_input_access_"+repairReason], obligation.outputID,
					)
					continue
				}
				candidate = repaired
			}
			compatible = append(compatible, candidate)
		}
		compatible = slices.DeleteFunc(compatible, func(candidate TopologyCandidate) bool {
			return v18TopologyInputImpedanceGap(obligation.requirement, candidate.Graph) != 0
		})
		if len(compatible) == 0 {
			rejections["multi_output_subsearch"] = append(rejections["multi_output_subsearch"], obligation.outputID+":"+string(search.Status))
			return nil, consumption, rejections
		}
		limit := min(candidateBreadth, len(compatible))
		candidatesByObligation = append(candidatesByObligation, append([]TopologyCandidate(nil), compatible[:limit]...))
	}
	combinations := [][]TopologyCandidate{{}}
	for _, candidates := range candidatesByObligation {
		next := [][]TopologyCandidate{}
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
	for combinationIndex, combination := range combinations {
		state, ok := mergeOutputTopologyCandidates(requirement, initial, combination, nil, inventory, inventoryByKey, limits, &consumption)
		if !ok {
			rejections["multi_output_merge"] = append(rejections["multi_output_merge"], fmt.Sprintf("combination_%03d", combinationIndex))
			continue
		}
		state, isolationReason := v18IsolateContendedActiveOutputs(
			requirement, state, inventory, inventoryByKey, limits, &consumption,
		)
		if isolationReason != "" {
			rejections["v18_output_isolation_"+isolationReason] = append(
				rejections["v18_output_isolation_"+isolationReason], fmt.Sprintf("combination_%03d", combinationIndex),
			)
			continue
		}
		states = append(states, state)
	}
	if len(states) == 0 {
		return nil, consumption, rejections
	}
	return completeComposedTopologyStatesV18(
		ctx, requirement, inventory, representatives, inventoryByKey, limits, policy,
		states, &consumption, rejections,
	)
}

type v18OutputTerminal struct {
	instanceID string
	terminal   string
	kind       string
}

func v18IsolateContendedActiveOutputs(
	requirement Requirement,
	state topologySearchState,
	inventory PrimitiveInventory,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	consumption *Consumption,
) (topologySearchState, string) {
	outputsByNode := map[string][]v18OutputTerminal{}
	for _, instance := range state.graph.Instances {
		primitive, found := inventoryByKey[instance.PrimitiveKey]
		if !found {
			return state, "primitive_unavailable"
		}
		contracts := map[string]PrimitiveTerminal{}
		for _, terminal := range primitive.Terminals {
			contracts[terminal.Terminal] = terminal
		}
		for _, connection := range instance.Terminals {
			contract, found := contracts[connection.Terminal]
			if found && v18ActiveOutputTerminal(instance.Kind, connection.Terminal, contract) {
				outputsByNode[connection.Node] = append(outputsByNode[connection.Node], v18OutputTerminal{
					instanceID: instance.ID, terminal: connection.Terminal, kind: instance.Kind,
				})
			}
		}
	}
	resistor := PrimitiveCandidate{}
	signalDiode := PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if resistor.Key == "" && primitive.Kind == "resistor" && len(primitive.Terminals) == 2 {
			resistor = primitive
		}
		if signalDiode.Key == "" && primitive.Kind == "signal_diode" && len(primitive.Terminals) == 2 {
			signalDiode = primitive
		}
	}
	contendedNodes := []string{}
	for node, outputs := range outputsByNode {
		if len(outputs) > 1 {
			contendedNodes = append(contendedNodes, node)
		}
	}
	if len(contendedNodes) == 0 {
		return state, ""
	}
	if resistor.Key == "" {
		return state, "resistor_unavailable"
	}
	slices.Sort(contendedNodes)
	nodeByID := make(map[string]GraphNode, len(state.graph.Nodes))
	for _, graphNode := range state.graph.Nodes {
		nodeByID[graphNode.ID] = graphNode
	}
	for _, node := range contendedNodes {
		outputs := outputsByNode[node]
		slices.SortFunc(outputs, func(left, right v18OutputTerminal) int {
			return strings.Compare(left.instanceID+"\x00"+left.terminal, right.instanceID+"\x00"+right.terminal)
		})
		useDiodeAND := signalDiode.Key != "" && v18ComparatorPullupNode(state.graph, node, outputs, nodeByID)
		for _, output := range outputs {
			var isolatedNode string
			state, isolatedNode = addRelationshipInternalNode(state, requirement, inventoryByKey, consumption)
			if isolatedNode == "" {
				return state, "node_add_failed"
			}
			var ok bool
			state, ok = v18RedirectTopologyTerminal(
				state, requirement, inventory, inventoryByKey,
				output.instanceID, output.terminal, isolatedNode, consumption,
			)
			if !ok {
				return state, "terminal_redirect_failed"
			}
			if useDiodeAND {
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, signalDiode, []TerminalConnection{
					{Terminal: "ANODE", Node: node},
					{Terminal: "CATHODE", Node: isolatedNode},
				}, consumption)
			} else {
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor, []TerminalConnection{
					{Terminal: resistor.Terminals[0].Terminal, Node: isolatedNode},
					{Terminal: resistor.Terminals[1].Terminal, Node: node},
				}, consumption)
			}
			if state.hash == "" {
				return state, "resistor_add_failed"
			}
		}
	}
	if len(state.graph.Instances) > limits.MaxPrimitiveInstances {
		return state, "component_limit"
	}
	if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
		return state, "complete_graph_invalid"
	}
	return state, ""
}

func v18ComparatorPullupNode(
	graph CandidateGraph,
	node string,
	outputs []v18OutputTerminal,
	nodes map[string]GraphNode,
) bool {
	for _, output := range outputs {
		if output.kind != "comparator" {
			return false
		}
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
			continue
		}
		for index, terminal := range instance.Terminals {
			if terminal.Node == node && nodes[instance.Terminals[1-index].Node].Role == "supply" {
				return true
			}
		}
	}
	return false
}

func v18ActiveOutputTerminal(kind, terminal string, contract PrimitiveTerminal) bool {
	electrical := strings.ToLower(strings.TrimSpace(contract.Electrical))
	if electrical == "output" || electrical == "open_collector" ||
		electrical == "open_emitter" || electrical == "tri_state" {
		return true
	}
	if kind != "opamp" && kind != "comparator" {
		return false
	}
	function := strings.ToUpper(strings.TrimSpace(contract.Function))
	return strings.EqualFold(strings.TrimSpace(terminal), "OUT") ||
		function == "OUT" || function == "OUTPUT"
}

func v18RepairPositiveFollowerPolarity(
	requirement Requirement,
	candidate TopologyCandidate,
	inventory PrimitiveInventory,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	consumption *Consumption,
) (TopologyCandidate, string) {
	state := topologySearchState{
		graph: CloneGraph(candidate.Graph), hash: candidate.Fingerprint,
		score: candidate.Score, operations: cloneGraphOperations(candidate.Operations),
		powerRequirements: deriveTopologyPowerRequirementProfile(requirement),
	}
	changed := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "voltage_gain" || assertion.Excitation == nil ||
			assertion.Excitation.Kind != "port" || assertion.Observation.Kind != "port" ||
			assertion.Max != nil && *assertion.Max <= 0 || assertion.Min != nil && *assertion.Min < 0 {
			continue
		}
		inputNode := "port_" + assertion.Excitation.ID
		outputNode := "port_" + assertion.Observation.ID
		for _, instance := range append([]GraphInstance(nil), state.graph.Instances...) {
			if instance.Kind != "opamp" {
				continue
			}
			terminalNode := map[string]string{}
			for _, connection := range instance.Terminals {
				terminalNode[connection.Terminal] = connection.Node
			}
			if terminalNode["OUT"] != outputNode {
				continue
			}
			switch {
			case terminalNode["IN_PLUS"] == outputNode && terminalNode["IN_MINUS"] == inputNode:
				var ok bool
				state, ok = v18RedirectTopologyTerminal(
					state, requirement, inventory, inventoryByKey,
					instance.ID, "IN_MINUS", outputNode, consumption,
				)
				if !ok {
					return TopologyCandidate{}, "negative_input_redirect_failed"
				}
				state, ok = v18RedirectTopologyTerminal(
					state, requirement, inventory, inventoryByKey,
					instance.ID, "IN_PLUS", inputNode, consumption,
				)
				if !ok {
					return TopologyCandidate{}, "positive_input_redirect_failed"
				}
			case terminalNode["IN_PLUS"] == inputNode && terminalNode["IN_MINUS"] != outputNode &&
				v18UnityGainAssertion(assertion):
				var ok bool
				state, ok = v18RedirectTopologyTerminal(
					state, requirement, inventory, inventoryByKey,
					instance.ID, "IN_MINUS", outputNode, consumption,
				)
				if !ok {
					return TopologyCandidate{}, "follower_feedback_redirect_failed"
				}
			default:
				continue
			}
			changed = true
		}
	}
	if !changed {
		return candidate, ""
	}
	if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
		return TopologyCandidate{}, "complete_graph_invalid"
	}
	normalized, err := NormalizeGraph(state.graph)
	if err != nil {
		return TopologyCandidate{}, "normalize_failed"
	}
	topologyHash, err := TopologyHash(normalized)
	if err != nil {
		return TopologyCandidate{}, "topology_hash_failed"
	}
	state.score = scoreTopologyGraphWithPowerRequirements(requirement, normalized, inventoryByKey, state.hash, state.powerRequirements)
	return TopologyCandidate{
		Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score,
		Graph: normalized, Operations: cloneGraphOperations(state.operations),
	}, ""
}

func v18UnityGainAssertion(assertion BehavioralAssertion) bool {
	if assertion.Min == nil || assertion.Max == nil || *assertion.Min > 1 || *assertion.Max < 1 {
		return false
	}
	return *assertion.Max-*assertion.Min <= 0.2
}

func v18RepairInputAccess(
	requirement Requirement,
	candidate TopologyCandidate,
	inventory PrimitiveInventory,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	consumption *Consumption,
) (TopologyCandidate, string) {
	minimumByNode := v18TopologyMinimumInputImpedanceByNode(requirement)
	if len(minimumByNode) == 0 {
		return candidate, ""
	}
	opamp := PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind == "opamp" {
			opamp = primitive
			break
		}
	}
	positiveSupply, _ := topologyPowerRails(requirement, candidate.Graph)
	references := topologyNodesByRole(candidate.Graph, "reference")
	if opamp.Key == "" {
		return TopologyCandidate{}, "buffer_primitive_unavailable"
	}
	if positiveSupply == "" {
		return TopologyCandidate{}, "positive_supply_unavailable"
	}
	if len(references) == 0 {
		return TopologyCandidate{}, "reference_unavailable"
	}
	reference := references[0]
	state := topologySearchState{
		graph: CloneGraph(candidate.Graph), hash: candidate.Fingerprint,
		score: candidate.Score, operations: cloneGraphOperations(candidate.Operations),
		powerRequirements: deriveTopologyPowerRequirementProfile(requirement),
	}
	nodeByID := make(map[string]GraphNode, len(state.graph.Nodes))
	for _, node := range state.graph.Nodes {
		nodeByID[node.ID] = node
	}
	inputNodes := make([]string, 0, len(minimumByNode))
	for inputNode := range minimumByNode {
		inputNodes = append(inputNodes, inputNode)
	}
	slices.Sort(inputNodes)
	for _, inputNode := range inputNodes {
		type redirectTarget struct {
			instanceID string
			terminal   string
		}
		redirects := []redirectTarget{}
		conductance := 0.0
		for _, instance := range state.graph.Instances {
			if instance.Kind != "resistor" || instance.ValueSI == nil || *instance.ValueSI <= 0 || len(instance.Terminals) != 2 {
				continue
			}
			for index, terminal := range instance.Terminals {
				if terminal.Node != inputNode || !topologyRailRole(nodeByID[instance.Terminals[1-index].Node].Role) {
					continue
				}
				conductance += 1 / *instance.ValueSI
				redirects = append(redirects, redirectTarget{instanceID: instance.ID, terminal: terminal.Terminal})
			}
		}
		if conductance == 0 || 1/conductance >= minimumByNode[inputNode] {
			continue
		}
		var bufferedNode string
		state, bufferedNode = addRelationshipInternalNode(state, requirement, inventoryByKey, consumption)
		if bufferedNode == "" {
			return TopologyCandidate{}, "node_add_failed"
		}
		for _, target := range redirects {
			var ok bool
			state, ok = v18RedirectTopologyTerminal(
				state, requirement, inventory, inventoryByKey,
				target.instanceID, target.terminal, bufferedNode, consumption,
			)
			if !ok {
				return TopologyCandidate{}, "terminal_redirect_failed"
			}
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, opamp, []TerminalConnection{
			{Terminal: "IN_PLUS", Node: inputNode},
			{Terminal: "IN_MINUS", Node: bufferedNode},
			{Terminal: "OUT", Node: bufferedNode},
			{Terminal: "V_PLUS", Node: positiveSupply},
			{Terminal: "V_MINUS", Node: reference},
		}, consumption)
		if state.hash == "" {
			return TopologyCandidate{}, "buffer_add_failed"
		}
	}
	if len(state.graph.Instances) > limits.MaxPrimitiveInstances {
		return TopologyCandidate{}, "component_limit"
	}
	if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
		return TopologyCandidate{}, "complete_graph_invalid"
	}
	if v18TopologyInputImpedanceGap(requirement, state.graph) != 0 {
		return TopologyCandidate{}, "impedance_unresolved"
	}
	normalized, err := NormalizeGraph(state.graph)
	if err != nil {
		return TopologyCandidate{}, "normalize_failed"
	}
	topologyHash, err := TopologyHash(normalized)
	if err != nil {
		return TopologyCandidate{}, "topology_hash_failed"
	}
	state.score = scoreTopologyGraphWithPowerRequirements(requirement, normalized, inventoryByKey, state.hash, state.powerRequirements)
	return TopologyCandidate{
		Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score,
		Graph: normalized, Operations: cloneGraphOperations(state.operations),
	}, ""
}

func v18RedirectTopologyTerminal(
	state topologySearchState,
	requirement Requirement,
	inventory PrimitiveInventory,
	inventoryByKey map[string]PrimitiveCandidate,
	instanceID, terminal, node string,
	consumption *Consumption,
) (topologySearchState, bool) {
	graph, err := RedirectPrimitiveTerminal(state.graph, inventory, instanceID, terminal, node)
	if err != nil {
		return state, false
	}
	hash, err := GraphHash(graph)
	if err != nil {
		return state, false
	}
	primitive := GraphInstance{}
	for _, instance := range state.graph.Instances {
		if instance.ID == instanceID {
			primitive = instance
			break
		}
	}
	if primitive.ID == "" {
		return state, false
	}
	operations := cloneGraphOperations(state.operations)
	operations = append(operations, GraphOperation{
		Number: len(operations) + 1, Kind: "redirect_terminal",
		PrimitiveKey: primitive.PrimitiveKey, PrimitiveKind: primitive.Kind, Node: node,
		Connections: []TerminalConnection{{Terminal: terminal, Node: node}},
		BeforeHash:  state.hash, AfterHash: hash,
	})
	consumption.GeneratedGraphs++
	return topologySearchState{
		graph: graph, hash: hash,
		score:      scoreTopologyGraphWithPowerRequirements(requirement, graph, inventoryByKey, hash, state.powerRequirements),
		operations: operations, powerRequirements: state.powerRequirements,
	}, true
}

func completeComposedTopologyStatesV18(
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
	visited := map[string]bool{}
	dominantTopology := map[string]topologySearchState{}
	retained := map[string]TopologyCandidate{}
	for _, state := range states {
		if visited[state.hash] {
			continue
		}
		visited[state.hash] = true
		if len(state.graph.Instances) != 0 && state.score.BehaviorGap == 0 &&
			len(ValidateCompleteGraph(state.graph, inventory, limits)) == 0 {
			normalized, normalizeErr := NormalizeGraph(state.graph)
			topologyHash, topologyErr := TopologyHash(state.graph)
			if normalizeErr == nil && topologyErr == nil {
				consumption.CompleteGraphs++
				candidate := TopologyCandidate{
					Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score,
					Graph: normalized, Operations: cloneGraphOperations(state.operations),
				}
				if existing, found := retained[topologyHash]; !found || compareTopologyCandidates(candidate, existing) < 0 {
					retained[topologyHash] = candidate
				}
				continue
			}
		}
		dominantTopology[state.topology] = state
		*frontier = append(*frontier, state)
	}
	if frontier.Len() != 0 {
		heap.Init(frontier)
		exploreTopologyFrontier(
			ctx, requirement, inventory, representatives, inventoryByKey, limits, policy,
			frontier, visited, dominantTopology, retained, rejections, consumption,
		)
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, *consumption, rejections
}

func v18CompatiblePrimitiveInventory(requirement Requirement, inventory PrimitiveInventory) PrimitiveInventory {
	filtered := inventory
	filtered.Primitives = make([]PrimitiveCandidate, 0, len(inventory.Primitives))
	for _, primitive := range inventory.Primitives {
		if primitive.Kind == "opamp" {
			supplyPenalty, headroomPenalty := v18OpAmpSelectionPenalty(requirement, primitive)
			if supplyPenalty != 0 || headroomPenalty != 0 {
				continue
			}
		}
		filtered.Primitives = append(filtered.Primitives, primitive)
	}
	return filtered
}

func v18OpAmpSelectionPenalty(requirement Requirement, primitive PrimitiveCandidate) (int, int) {
	if primitive.Kind != "opamp" {
		return 0, 0
	}
	requiredSupplySpan := maximumDeclaredSupplySpan(requirement)
	for _, model := range primitive.Models {
		if model.ModelID != simmodel.PrimitiveOpAmpV1 {
			continue
		}
		parameters := map[string]float64{}
		for _, parameter := range model.Parameters {
			parameters[parameter.Name] = parameter.Value
		}
		supplyMinimum, haveSupplyMinimum := parameters["supply_min_v"]
		supplyMaximum, haveSupplyMaximum := parameters["supply_max_v"]
		_, haveLowMargin := parameters["output_low_margin_v"]
		highMargin, haveHighMargin := parameters["output_high_margin_v"]
		if !haveSupplyMinimum || !haveSupplyMaximum || !haveLowMargin || !haveHighMargin {
			continue
		}
		supplyPenalty := 0
		if requiredSupplySpan > 0 && (requiredSupplySpan < supplyMinimum || requiredSupplySpan > supplyMaximum) {
			supplyPenalty = 1
		}
		minimumPositiveSupply := math.Inf(1)
		for _, domain := range requirement.Requirements.Domains {
			if domain.Kind == "supply" && domain.MinVoltageV != nil && *domain.MinVoltageV > 0 {
				minimumPositiveSupply = math.Min(minimumPositiveSupply, *domain.MinVoltageV)
			}
		}
		requiredOutputs := map[string]bool{}
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			if assertion.Observation.Kind == "port" && slices.Contains([]string{
				"dc_voltage", "output_high_voltage", "output_low_voltage", "output_noise_rms",
				"output_swing", "output_voltage", "phase_margin", "startup_output_voltage",
				"thd", "total_harmonic_distortion", "voltage_gain", "voltage_gain_at_frequency",
			}, assertion.Metric) {
				requiredOutputs[assertion.Observation.ID] = true
			}
		}
		for _, port := range requirement.Requirements.Ports {
			if requiredOutputs[port.ID] && port.Direction == "source" && port.Electrical.MaxVoltageV != nil &&
				finite(minimumPositiveSupply) && *port.Electrical.MaxVoltageV > minimumPositiveSupply-highMargin {
				return supplyPenalty, 1
			}
		}
		return supplyPenalty, 0
	}
	return 2, 1
}

func v18TopologyMinimumInputImpedanceByNode(requirement Requirement) map[string]float64 {
	result := map[string]float64{}
	for _, port := range requirement.Requirements.Ports {
		if port.Electrical.InputImpedanceMinOhm != nil && *port.Electrical.InputImpedanceMinOhm > 0 {
			result["port_"+port.ID] = *port.Electrical.InputImpedanceMinOhm
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "input_impedance" && assertion.Observation.Kind == "port" && assertion.Min != nil && *assertion.Min > 0 {
			node := "port_" + assertion.Observation.ID
			result[node] = math.Max(result[node], *assertion.Min)
		}
	}
	return result
}

func v18TopologyInputImpedanceGap(requirement Requirement, graph CandidateGraph) int {
	minimumByNode := v18TopologyMinimumInputImpedanceByNode(requirement)
	if len(minimumByNode) == 0 {
		return 0
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	conductanceByInput := map[string]float64{}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || instance.ValueSI == nil || *instance.ValueSI <= 0 || len(instance.Terminals) != 2 {
			continue
		}
		for index, terminal := range instance.Terminals {
			if minimumByNode[terminal.Node] <= 0 {
				continue
			}
			if topologyRailRole(nodeByID[instance.Terminals[1-index].Node].Role) {
				conductanceByInput[terminal.Node] += 1 / *instance.ValueSI
			}
		}
	}
	gap := 0
	for input, conductance := range conductanceByInput {
		if conductance > 0 && 1/conductance < minimumByNode[input] {
			gap++
		}
	}
	return gap
}

func v18ThresholdWindowRequirement(requirement Requirement) Requirement {
	result := cloneRequirement(Normalize(requirement))
	envelope, found := topologyWindowThresholdEnvelope(result)
	if !found {
		return result
	}
	for _, assertion := range result.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind == "port" && assertion.Observation.ID == envelope.output &&
			(assertion.Metric == "output_high_voltage" || assertion.Metric == "output_low_voltage") {
			return result
		}
	}
	inputMinimum, inputMaximum, outputMinimum, outputMaximum, boundsFound := v18WindowPortBounds(result, envelope)
	if !boundsFound {
		return result
	}
	span := envelope.upperV - envelope.lowerV
	below := math.Max(inputMinimum, envelope.lowerV-span/4)
	above := math.Min(inputMaximum, envelope.upperV+span/4)
	inside := (envelope.lowerV + envelope.upperV) / 2
	if below >= envelope.lowerV || above <= envelope.upperV || inside <= envelope.lowerV || inside >= envelope.upperV {
		return result
	}
	caseIDs := v18UniqueWindowIDs(result)
	points := []float64{below, inside, above}
	for index, point := range points {
		result.Requirements.OperatingCases = append(result.Requirements.OperatingCases, OperatingCase{
			ID:         caseIDs[index],
			Conditions: []OperatingCondition{{Axis: "input_voltage", Target: envelope.input, Min: point, Max: point, Unit: "V"}},
		})
	}
	highMinimum := outputMinimum + .75*(outputMaximum-outputMinimum)
	lowMaximum := outputMinimum + .25*(outputMaximum-outputMinimum)
	for index, metric := range []string{"output_high_voltage", "output_low_voltage", "output_high_voltage"} {
		assertion := BehavioralAssertion{
			ID: caseIDs[index] + "_assertion", Metric: metric, Analysis: simmodel.AnalysisDCOperatingPoint,
			Excitation:  &Observation{Kind: "port", ID: envelope.input},
			Observation: Observation{Kind: "port", ID: envelope.output}, Unit: "V",
			OperatingCases: []string{caseIDs[index]},
		}
		if metric == "output_high_voltage" {
			assertion.Min = floatPointer(highMinimum)
		} else {
			assertion.Max = floatPointer(lowMaximum)
		}
		result.Requirements.BehavioralRequirements = append(result.Requirements.BehavioralRequirements, assertion)
	}
	return Normalize(result)
}

func v18WindowPortBounds(requirement Requirement, envelope windowBehaviorEnvelope) (float64, float64, float64, float64, bool) {
	inputMinimum, inputMaximum, outputMinimum, outputMaximum := 0.0, 0.0, 0.0, 0.0
	inputFound, outputFound := false, false
	for _, port := range requirement.Requirements.Ports {
		switch port.ID {
		case envelope.input:
			if port.Electrical.MinVoltageV != nil && port.Electrical.MaxVoltageV != nil {
				inputMinimum, inputMaximum, inputFound = *port.Electrical.MinVoltageV, *port.Electrical.MaxVoltageV, true
			}
		case envelope.output:
			if port.Electrical.MinVoltageV != nil && port.Electrical.MaxVoltageV != nil {
				outputMinimum, outputMaximum, outputFound = *port.Electrical.MinVoltageV, *port.Electrical.MaxVoltageV, true
			}
		}
	}
	return inputMinimum, inputMaximum, outputMinimum, outputMaximum,
		inputFound && outputFound && inputMinimum < envelope.lowerV && inputMaximum > envelope.upperV && outputMinimum < outputMaximum
}

func v18UniqueWindowIDs(requirement Requirement) [3]string {
	existing := map[string]bool{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		existing[operatingCase.ID] = true
	}
	for suffix := 0; ; suffix++ {
		ids := [3]string{
			fmt.Sprintf("v18_window_below_%02d", suffix),
			fmt.Sprintf("v18_window_inside_%02d", suffix),
			fmt.Sprintf("v18_window_above_%02d", suffix),
		}
		if !existing[ids[0]] && !existing[ids[1]] && !existing[ids[2]] {
			return ids
		}
	}
}
