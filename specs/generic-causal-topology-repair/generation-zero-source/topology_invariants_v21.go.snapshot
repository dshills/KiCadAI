package opentopologysynthesis

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// AnalyzeTopologyV21 evaluates causal structure independently from model and
// solver admission. It intentionally does not require every primitive model to
// implement every analysis in the request; V20 admission owns that decision.
func AnalyzeTopologyV21(requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory) TopologyInvariantReportV21 {
	requirement = Normalize(requirement)
	report := TopologyInvariantReportV21{
		Schema: TopologyCompletionSchemaV21, Version: TopologyCompletionVersionV21,
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
	return finalizeTopologyInvariantV21(report)
}

func topologyObligationV21(kind, assertion, observation, from, to, domain, instance string, critical bool, terminal ...string) TopologyObligationV21 {
	result := TopologyObligationV21{Kind: kind, AssertionID: assertion, ObservationID: observation, FromNode: from, ToNode: to, Domain: domain, InstanceID: instance, Critical: critical}
	if len(terminal) != 0 {
		result.Terminal = terminal[0]
	}
	result.EvidenceHash = causalCrossStageHash(result)
	result.ID = kind + "_" + result.EvidenceHash[:12]
	return result
}

func finalizeTopologyInvariantV21(report TopologyInvariantReportV21) TopologyInvariantReportV21 {
	slices.SortFunc(report.Obligations, func(left, right TopologyObligationV21) int {
		return cmp.Or(cmp.Compare(topologyCriticalRankV21(left.Critical), topologyCriticalRankV21(right.Critical)), cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.AssertionID, right.AssertionID), cmp.Compare(left.ObservationID, right.ObservationID), cmp.Compare(left.FromNode, right.FromNode), cmp.Compare(left.ToNode, right.ToNode), cmp.Compare(left.InstanceID, right.InstanceID), cmp.Compare(left.Terminal, right.Terminal))
	})
	report.Obligations = slices.CompactFunc(report.Obligations, func(left, right TopologyObligationV21) bool { return left.EvidenceHash == right.EvidenceHash })
	report.Complete = len(report.Obligations) == 0
	report.Hash = ""
	report.Hash = causalCrossStageHash(report)
	return report
}

func topologyCriticalRankV21(critical bool) int {
	if critical {
		return 0
	}
	return 1
}

func topologyFirstV21(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func topologyUndirectedAdjacencyV21(graph CandidateGraph) map[string][]string {
	result := map[string][]string{}
	for _, instance := range graph.Instances {
		instanceVertex := "instance:" + instance.ID
		for _, connection := range instance.Terminals {
			nodeVertex := "node:" + connection.Node
			result[instanceVertex] = append(result[instanceVertex], nodeVertex)
			result[nodeVertex] = append(result[nodeVertex], instanceVertex)
		}
	}
	for vertex := range result {
		slices.Sort(result[vertex])
		result[vertex] = slices.Compact(result[vertex])
	}
	return result
}

func topologyDriverOwnersV21(graph CandidateGraph, inventory PrimitiveInventory, nodeID string) []string {
	result := []string{}
	for _, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			continue
		}
		contracts := causalTerminalContractsV19(primitive)
		for _, connection := range instance.Terminals {
			role := causalTerminalRoleV19(contracts[connection.Terminal])
			if connection.Node == nodeID && (role == "output" || role == "open_collector" || role == "power_output") {
				result = append(result, instance.ID+":"+connection.Terminal)
			}
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func topologyReferenceNodesV21(graph CandidateGraph) []string {
	result := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "reference" {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func topologyObligationSummaryV21(obligation TopologyObligationV21) string {
	return strings.Join([]string{obligation.Kind, obligation.AssertionID, obligation.ObservationID, obligation.FromNode, obligation.ToNode, obligation.Domain, obligation.InstanceID, obligation.Terminal}, ":")
}

func topologyInvariantIssueV21(report TopologyInvariantReportV21) string {
	if report.Complete {
		return ""
	}
	if len(report.Obligations) == 0 {
		return "topology invariant evaluation failed"
	}
	return fmt.Sprintf("%d unresolved topology obligations; first=%s", len(report.Obligations), topologyObligationSummaryV21(report.Obligations[0]))
}
