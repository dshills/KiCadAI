package opentopologysynthesis

import "slices"

// Version-isolated structural derivation preserves all V21 checks except that
// explained feedback cycles receive a separate, authenticated binding path.
// The historical V21 function and its sealed behavior remain unchanged.
func analyzeElectricalTopologyV22(requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, bindings []FeedbackBindingV22) TopologyInvariantReportV21 {
	requirement = Normalize(requirement)
	report := TopologyInvariantReportV21{
		Schema: "kicadai.electrical-topology.v22", Version: 22,
		InventoryHash: inventory.Hash, Obligations: []TopologyObligationV21{},
	}
	var hashErr error
	report.RequirementHash, hashErr = CanonicalHash(requirement)
	if hashErr != nil {
		report.Contradictory = true
		report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationInvalidEvidenceV21, "", "", "", "", "", "", false))
		return finalizeTopologyInvariantV21(report)
	}
	graph, normalizeErr := NormalizeGraph(graph)
	if normalizeErr == nil {
		report.GraphHash, hashErr = GraphHash(graph)
	}
	limits := GraphLimits{MaxPrimitiveInstances: DefaultPolicy().MaxPrimitiveInstances, MaxInternalNodes: DefaultPolicy().MaxInternalNodes}
	if normalizeErr != nil || hashErr != nil || len(ValidatePartialGraph(graph, inventory, limits)) != 0 {
		report.Contradictory = true
		kind := TopologyObligationDirectionV21
		if hashErr != nil {
			kind = TopologyObligationInvalidEvidenceV21
		}
		report.Obligations = append(report.Obligations, topologyObligationV21(kind, "", "", "", "", "", "", false))
		return finalizeTopologyInvariantV21(report)
	}

	ports := map[string]Port{}
	nodes := map[string]GraphNode{}
	portNodes := map[string]string{}
	degree := map[string]int{}
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
		if node.Scope == "external" && node.SemanticKind == "port" {
			portNodes[node.SemanticID] = node.ID
		}
	}
	for _, instance := range graph.Instances {
		for _, connection := range instance.Terminals {
			degree[connection.Node]++
		}
	}
	for _, port := range requirement.Requirements.Ports {
		nodeID := portNodes[port.ID]
		if nodeID == "" {
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationMissingBindingV21, "", port.ID, "", "", port.Domain, "", true))
			continue
		}
		if degree[nodeID] == 0 {
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationUnreachablePortV21, "", port.ID, "", nodeID, port.Domain, "", true))
		}
	}

	referenceCount := map[string]int{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "reference" {
			referenceCount[node.Domain]++
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind == "reference" && referenceCount[domain.ID] != 1 {
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationReferenceV21, "", "", "", "", domain.ID, "", true))
		}
	}

	drivers, directed := causalGraphReachabilityV19(graph, inventory)
	undirected := topologyUndirectedAdjacencyV21(graph)
	defaultStarts := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && (node.Role == "input" || node.Role == "control" || node.Role == "supply") {
			defaultStarts = append(defaultStarts, node.ID)
		}
	}
	slices.Sort(defaultStarts)
	assertionsByObservation := map[string][]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind != "port" {
			continue
		}
		assertionsByObservation[assertion.Observation.ID] = append(assertionsByObservation[assertion.Observation.ID], assertion)
		observationNode := portNodes[assertion.Observation.ID]
		if observationNode == "" {
			continue
		}
		starts := slices.Clone(defaultStarts)
		if assertion.Excitation != nil && assertion.Excitation.Kind == "port" {
			if excitationNode := portNodes[assertion.Excitation.ID]; excitationNode != "" {
				starts = []string{excitationNode}
			}
		}
		reachable := causalReachableNodesV19(starts, directed)
		if !drivers[observationNode] {
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationObservationConeV21, assertion.ID, assertion.Observation.ID, topologyFirstV21(starts), observationNode, ports[assertion.Observation.ID].Domain, "", assertion.Critical))
		}
		if !reachable[observationNode] {
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationCausalPathV21, assertion.ID, assertion.Observation.ID, topologyFirstV21(starts), observationNode, ports[assertion.Observation.ID].Domain, "", assertion.Critical))
		}
		connected := false
		for _, start := range starts {
			if graphPathExists(undirected, "node:"+start, "node:"+observationNode) {
				connected = true
				break
			}
		}
		if !connected {
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationDisconnectedSubgraphV21, assertion.ID, assertion.Observation.ID, topologyFirstV21(starts), observationNode, ports[assertion.Observation.ID].Domain, "", assertion.Critical))
		}
	}

	for _, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			continue
		}
		contracts := causalTerminalContractsV19(primitive)
		for _, connection := range instance.Terminals {
			role := causalTerminalRoleV19(contracts[connection.Terminal])
			node := nodes[connection.Node]
			if (role == "output" || role == "open_collector" || role == "power_output") && node.Role == "reference" {
				report.Contradictory = true
				report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationDirectionV21, "", node.SemanticID, "", connection.Node, node.Domain, instance.ID, true, connection.Terminal))
			}
		}
	}

	observations := make([]string, 0, len(assertionsByObservation))
	for observation := range assertionsByObservation {
		observations = append(observations, observation)
	}
	slices.Sort(observations)
	if len(observations) > 1 {
		driverOwners := map[string]bool{}
		for _, observation := range observations {
			nodeID := portNodes[observation]
			for _, owner := range topologyDriverOwnersV21(graph, inventory, nodeID) {
				driverOwners[owner] = true
			}
		}
		if len(driverOwners) < len(observations) {
			for _, observation := range observations {
				assertion := assertionsByObservation[observation][0]
				report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationBranchV21, assertion.ID, observation, topologyFirstV21(defaultStarts), portNodes[observation], ports[observation].Domain, "", assertion.Critical))
			}
		}
	}

	sourceVertices := append(slices.Clone(defaultStarts), topologyReferenceNodesV21(graph)...)
	for index := range sourceVertices {
		sourceVertices[index] = "node:" + sourceVertices[index]
	}
	observationVertices := make([]string, 0, len(observations))
	for _, observation := range observations {
		if nodeID := portNodes[observation]; nodeID != "" {
			observationVertices = append(observationVertices, "node:"+nodeID)
		}
	}
	fromSource := causalReachableNodesV19(sourceVertices, undirected)
	toObservation := causalReachableNodesV19(observationVertices, undirected)
	for _, instance := range graph.Instances {
		vertex := "instance:" + instance.ID
		if !fromSource[vertex] || !toObservation[vertex] {
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationIrrelevantFragmentV21, "", "", "", "", "", instance.ID, false))
		}
	}
	if len(report.Obligations) == 0 {
		// Reachability alone cannot certify electrical structure. Reuse the
		// established structural checks without the V19 model/analysis gate:
		// exact model and solver admission remains V20's responsibility.
		validator := newCausalInvariantValidatorV19(requirement, graph, inventory, CausalInvariantContextV19{})
		validator.validateRequirementBindings()
		validator.validateTerminalDomainsAndRatings()
		validator.validateActiveOutputContention()
		validator.validateReferenceClosure()
		// Every active cycle must be explained by exact role-bound feedback
		// evidence. This is structural permission to evaluate, not a pass.
		validateFeedbackBindingsV22(validator, bindings)
		issues := ValidateCompleteGraph(graph, inventory, limits)
		issues = append(issues, validator.issues...)
		if len(issues) != 0 {
			report.Contradictory = true
			report.Issues = issues
			report.Obligations = append(report.Obligations, topologyObligationV21(TopologyObligationDirectionV21, "", "", "", "", "", "", true))
		}
	}
	return finalizeTopologyInvariantV21(report)
}
