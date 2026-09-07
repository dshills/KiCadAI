package opentopologysynthesis

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"kicadai/internal/reports"
)

type topologyProposalV21 struct {
	graph     CandidateGraph
	operation TopologyOperationEvidenceV21
	report    TopologyInvariantReportV21
}

type topologyStateV21 struct {
	graph      CandidateGraph
	graphHash  string
	stateHash  string
	depth      int
	operations []TopologyOperationEvidenceV21
	report     TopologyInvariantReportV21
	ancestors  map[string]bool
}

// PlanTopologyCompletionV21 applies only structural, contract-derived graph
// operations. Numerical evaluation remains a separate admitted step.
func PlanTopologyCompletionV21(ctx context.Context, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, limits TopologyCompletionLimitsV21) TopologyCompletionPlanV21 {
	limits = effectiveTopologyCompletionLimitsV21(limits)
	requirement = Normalize(requirement)
	inventory.Primitives = slices.Clone(inventory.Primitives)
	slices.SortFunc(inventory.Primitives, comparePrimitiveCandidates)
	result := TopologyCompletionPlanV21{
		Schema: TopologyCompletionSchemaV21, Version: TopologyCompletionVersionV21,
		InventoryHash: inventory.Hash, Limits: limits, Candidates: []TopologyCandidateEvidenceV21{},
		Issues: []reports.Issue{}, Status: "failed",
	}
	if ctx.Err() != nil {
		return canceledTopologyCompletionPlanV21(result)
	}
	var err error
	result.RequirementHash, err = CanonicalHash(requirement)
	if err != nil {
		result.Status = TopologyRejectionInvalidV21
		result.Issues = append(result.Issues, graphIssue(CodeTopologyInvalidRepairV21, "topology.v21.requirement", err.Error(), "supply a canonical requirement"))
		return finalizeTopologyCompletionPlanV21(result)
	}
	normalized, err := NormalizeGraph(graph)
	if err != nil {
		result.Status = TopologyRejectionInvalidV21
		result.Issues = append(result.Issues, graphIssue(CodeTopologyInvalidRepairV21, "topology.v21.graph", err.Error(), "supply a canonical candidate graph"))
		return finalizeTopologyCompletionPlanV21(result)
	}
	graph = normalized
	result.InitialGraphHash, err = GraphHash(graph)
	if err != nil {
		result.Status = TopologyRejectionInvalidV21
		result.Issues = append(result.Issues, graphIssue(CodeTopologyInvalidRepairV21, "topology.v21.graph_hash", err.Error(), "supply a canonical candidate graph"))
		return finalizeTopologyCompletionPlanV21(result)
	}
	result.Initial = AnalyzeTopologyV21(requirement, graph, inventory)
	if ctx.Err() != nil {
		return canceledTopologyCompletionPlanV21(result)
	}
	if result.Initial.Contradictory {
		result.Status = TopologyRejectionContradictoryV21
		result.Issues = append(result.Issues, graphIssue(CodeTopologyContradictoryV21, "topology.v21.initial", topologyInvariantIssueV21(result.Initial), "correct the contradictory contract or graph before completion"))
		return finalizeTopologyCompletionPlanV21(result)
	}
	base := topologyStateV21{graph: graph, graphHash: result.InitialGraphHash, depth: 0, report: result.Initial, ancestors: map[string]bool{result.InitialGraphHash: true}}
	base.stateHash = topologyStateHashV21(base)
	if result.Initial.Complete {
		selected := topologyCandidateEvidenceV21(base, "selected", "structurally_complete")
		result.Selected = &selected
		result.Status = "complete"
		return finalizeTopologyCompletionPlanV21(result)
	}

	frontier := []topologyStateV21{base}
	seen := map[string]topologyStateV21{base.graphHash: base}
	result.Consumption.MaximumRetained = 1
	for depth := 1; depth <= limits.MaximumDepth && len(frontier) != 0; depth++ {
		if ctx.Err() != nil {
			return canceledTopologyCompletionPlanV21(result)
		}
		slices.SortFunc(frontier, compareTopologyStatesV21)
		if len(frontier) > limits.MaximumWidth {
			frontier = frontier[:limits.MaximumWidth]
			result.Consumption.BudgetExhausted = true
		}
		result.Consumption.MaximumFrontier = max(result.Consumption.MaximumFrontier, len(frontier))
		next := []topologyStateV21{}
		for _, parent := range frontier {
			if ctx.Err() != nil {
				return canceledTopologyCompletionPlanV21(result)
			}
			if result.Consumption.WorkConsumed >= limits.MaximumWork {
				result.Consumption.BudgetExhausted = true
				break
			}
			result.Consumption.ExpandedStates++
			proposals := topologyProposalsV21(ctx, requirement, parent, inventory)
			if ctx.Err() != nil {
				return canceledTopologyCompletionPlanV21(result)
			}
			if len(proposals) == 0 {
				continue
			}
			remaining := limits.MaximumWork - result.Consumption.WorkConsumed
			if len(proposals) > remaining {
				proposals = proposals[:remaining]
				result.Consumption.BudgetExhausted = true
			}
			topologyAnalyzeProposalsV21(ctx, requirement, inventory, proposals, limits.Workers)
			if ctx.Err() != nil {
				return canceledTopologyCompletionPlanV21(result)
			}
			for _, proposal := range proposals {
				if ctx.Err() != nil {
					return canceledTopologyCompletionPlanV21(result)
				}
				result.Consumption.WorkConsumed++
				result.Consumption.GeneratedCandidates++
				proposal.operation.Number = len(parent.operations) + 1
				proposal.operation.WorkConsumed = result.Consumption.WorkConsumed
				proposal.operation.ParentStateHash = parent.stateHash
				proposal.operation = finalizeTopologyOperationV21(proposal.operation)
				candidateHash, hashErr := GraphHash(proposal.graph)
				state := topologyStateV21{
					graph: proposal.graph, graphHash: candidateHash, depth: depth,
					operations: append(slices.Clone(parent.operations), proposal.operation), report: proposal.report,
					ancestors: topologyCloneSetV21(parent.ancestors),
				}
				state.stateHash = topologyStateHashV21(state)
				disposition, reason := "retained", "strict_structural_improvement"
				switch {
				case hashErr != nil || candidateHash == "" || candidateHash != proposal.operation.AfterGraphHash:
					disposition, reason = "rejected", TopologyRejectionInvalidV21
					result.Consumption.InvalidCandidates++
				case !topologyGraphWithinMemoryV21(proposal.graph, limits.MaximumGraphBytes):
					disposition, reason = "rejected", TopologyRejectionMemoryV21
					result.Consumption.InvalidCandidates++
				case parent.ancestors[candidateHash]:
					disposition, reason = "rejected", TopologyRejectionCycleV21
					result.Consumption.CycleCandidates++
				case seen[candidateHash].graphHash != "":
					disposition, reason = "rejected", TopologyRejectionDuplicateV21
					result.Consumption.DuplicateCandidates++
				case proposal.report.Contradictory:
					disposition, reason = "rejected", TopologyRejectionContradictoryV21
					result.Consumption.InvalidCandidates++
				case !topologyStrictlyImprovesV21(parent, state):
					disposition, reason = "rejected", TopologyRejectionDominatedV21
					result.Consumption.DominatedCandidates++
				}
				proposal.operation.Accepted = disposition == "retained"
				proposal.operation.Reason = reason
				proposal.operation = finalizeTopologyOperationV21(proposal.operation)
				state.operations[len(state.operations)-1] = proposal.operation
				evidence := topologyCandidateEvidenceV21(state, disposition, reason)
				result.Candidates = append(result.Candidates, evidence)
				if disposition != "retained" {
					continue
				}
				seen[candidateHash] = state
				state.ancestors[candidateHash] = true
				if state.report.Complete {
					selected := evidence
					selected.Disposition, selected.Reason = "selected", "structurally_complete"
					selected.Hash = ""
					selected.Hash = causalCrossStageHash(selected)
					result.Selected = &selected
					result.Status = "complete"
					return finalizeTopologyCompletionPlanV21(result)
				}
				next = append(next, state)
			}
		}
		slices.SortFunc(next, compareTopologyStatesV21)
		if len(next) > limits.MaximumWidth {
			next = next[:limits.MaximumWidth]
			result.Consumption.BudgetExhausted = true
		}
		if len(seen) > limits.MaximumRetained {
			result.Consumption.BudgetExhausted = true
			break
		}
		result.Consumption.MaximumRetained = max(result.Consumption.MaximumRetained, len(seen))
		frontier = next
	}
	if len(frontier) != 0 && result.Selected == nil {
		result.Consumption.BudgetExhausted = true
	}
	if result.Consumption.BudgetExhausted || result.Consumption.WorkConsumed >= limits.MaximumWork {
		result.Status = "exhausted"
		result.Issues = append(result.Issues, graphIssue(CodeTopologyBoundV21, "topology.v21.budget", "bounded topology completion exhausted its frozen work limits", "inspect typed obligations or increase limits in a future frozen protocol"))
	} else if result.Consumption.CycleCandidates+result.Consumption.DuplicateCandidates != 0 {
		result.Status = TopologyRejectionCycleV21
		result.Issues = append(result.Issues, graphIssue(CodeTopologyCycleV21, "topology.v21.candidates", "all applicable topology candidates repeated an existing or ancestor state", "inspect canonical operation ordering and the unresolved obligation"))
	} else if result.Consumption.InvalidCandidates != 0 {
		result.Status = TopologyRejectionInvalidV21
		result.Issues = append(result.Issues, graphIssue(CodeTopologyInvalidRepairV21, "topology.v21.candidates", "all applicable topology candidates violated a terminal, domain, observation, or memory invariant", "select a compatible generic operation within the frozen graph bound"))
	} else {
		result.Status = TopologyRejectionNoOperationV21
		result.Issues = append(result.Issues, graphIssue(CodeTopologyNoApplicableV21, "topology.v21.operations", topologyInvariantIssueV21(result.Initial), "provide a compatible reviewed primitive or correct the contradictory requirement"))
	}
	return finalizeTopologyCompletionPlanV21(result)
}

func effectiveTopologyCompletionLimitsV21(limits TopologyCompletionLimitsV21) TopologyCompletionLimitsV21 {
	defaults := DefaultTopologyCompletionLimitsV21()
	if limits.MaximumDepth <= 0 {
		limits.MaximumDepth = defaults.MaximumDepth
	}
	if limits.MaximumWidth <= 0 {
		limits.MaximumWidth = defaults.MaximumWidth
	}
	if limits.MaximumWork <= 0 {
		limits.MaximumWork = defaults.MaximumWork
	}
	if limits.MaximumRetained <= 0 {
		limits.MaximumRetained = defaults.MaximumRetained
	}
	if limits.MaximumGraphBytes <= 0 {
		limits.MaximumGraphBytes = defaults.MaximumGraphBytes
	}
	if limits.Workers <= 0 {
		limits.Workers = defaults.Workers
	}
	limits.Workers = min(limits.Workers, limits.MaximumWork)
	return limits
}

func topologyProposalsV21(ctx context.Context, requirement Requirement, parent topologyStateV21, inventory PrimitiveInventory) []topologyProposalV21 {
	result := []topologyProposalV21{}
	for _, obligation := range parent.report.Obligations {
		if ctx.Err() != nil {
			return result
		}
		switch obligation.Kind {
		case TopologyObligationMissingBindingV21:
			result = append(result, topologyConnectMissingPortV21(requirement, parent, obligation)...)
		case TopologyObligationObservationConeV21, TopologyObligationCausalPathV21,
			TopologyObligationDisconnectedSubgraphV21, TopologyObligationBranchV21,
			TopologyObligationUnreachablePortV21:
			result = append(result, topologyStageProposalsV21(ctx, requirement, parent, inventory, obligation)...)
			result = append(result, topologyRedirectProposalsV21(ctx, parent, inventory, obligation)...)
		case TopologyObligationDirectionV21:
			result = append(result, topologyRedirectProposalsV21(ctx, parent, inventory, obligation)...)
		case TopologyObligationReferenceV21:
			result = append(result, topologyReferenceProposalsV21(requirement, parent, inventory, obligation)...)
		case TopologyObligationIrrelevantFragmentV21:
			result = append(result, topologyRemoveFragmentV21(parent, obligation)...)
		}
	}
	result = append(result, topologyJoinPartialPathsV21(ctx, parent, inventory)...)
	slices.SortFunc(result, compareTopologyProposalsV21)
	deduplicated := result[:0]
	seenGraphs := make(map[string]bool, len(result))
	for _, proposal := range result {
		if seenGraphs[proposal.operation.AfterGraphHash] {
			continue
		}
		seenGraphs[proposal.operation.AfterGraphHash] = true
		deduplicated = append(deduplicated, proposal)
	}
	return deduplicated
}

func topologyStageProposalsV21(ctx context.Context, requirement Requirement, parent topologyStateV21, inventory PrimitiveInventory, obligation TopologyObligationV21) []topologyProposalV21 {
	assertion, found := topologyAssertionV21(requirement, obligation.AssertionID, obligation.ObservationID)
	if !found || obligation.FromNode == "" || obligation.ToNode == "" {
		return nil
	}
	upstream, upstreamFound := graphNodeByID(parent.graph, obligation.FromNode)
	observation, observationFound := graphNodeByID(parent.graph, obligation.ToNode)
	if !upstreamFound || !observationFound || observation.Role != "output" {
		return nil
	}
	result := []topologyProposalV21{}
	for _, primitive := range inventory.Primitives {
		if ctx.Err() != nil {
			return result
		}
		if !topologyPrimitiveSupportsAnalysisV21(primitive, assertion.Analysis) {
			continue
		}
		value, ok := causalPrimitiveDefaultValueV19(primitive)
		if !ok {
			continue
		}
		for _, connections := range causalStageConnectionMapsV19(parent.graph, primitive, upstream, observation) {
			candidate := AddPrimitive(parent.graph, primitive, value, connections)
			candidate, err := NormalizeGraph(candidate)
			if err != nil {
				continue
			}
			beforeHash := parent.graphHash
			afterHash, err := GraphHash(candidate)
			if err != nil || beforeHash == afterHash {
				continue
			}
			instanceID := causalAddedInstanceIDV19(parent.graph, candidate)
			kind := TopologyOperationCompletePathV21
			if obligation.Kind == TopologyObligationObservationConeV21 {
				kind = TopologyOperationExtendConeV21
			}
			if obligation.Kind == TopologyObligationBranchV21 {
				kind = TopologyOperationIntroduceBranchV21
			}
			if obligation.Kind == TopologyObligationUnreachablePortV21 {
				kind = TopologyOperationConnectPortV21
			}
			result = append(result, topologyProposalV21{graph: candidate, operation: TopologyOperationEvidenceV21{
				Kind: kind, Obligation: obligation, AffectedScope: []string{obligation.FromNode, obligation.ToNode, instanceID},
				PrimitiveKey: primitive.Key, InstanceID: instanceID, BeforeGraphHash: beforeHash, AfterGraphHash: afterHash,
			}})
		}
	}
	return result
}

func topologyRedirectProposalsV21(ctx context.Context, parent topologyStateV21, inventory PrimitiveInventory, obligation TopologyObligationV21) []topologyProposalV21 {
	result := []topologyProposalV21{}
	targets := []string{obligation.ToNode, obligation.FromNode}
	for _, instance := range parent.graph.Instances {
		if ctx.Err() != nil {
			return result
		}
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			continue
		}
		contracts := causalTerminalContractsV19(primitive)
		for terminalIndex, connection := range instance.Terminals {
			role := causalTerminalRoleV19(contracts[connection.Terminal])
			for targetIndex, targetID := range targets {
				if targetID == "" || targetID == connection.Node {
					continue
				}
				if targetIndex == 0 && role != "output" && role != "open_collector" && role != "power_output" {
					continue
				}
				if targetIndex == 1 && role != "input" {
					continue
				}
				target, found := graphNodeByID(parent.graph, targetID)
				if !found || !causalTerminalNodeCompatibleV19(contracts[connection.Terminal], target) {
					continue
				}
				candidate := CloneGraph(parent.graph)
				candidate.Instances[graphInstanceIndex(candidate, instance.ID)].Terminals[terminalIndex].Node = targetID
				candidate, err := NormalizeGraph(candidate)
				if err != nil {
					continue
				}
				afterHash, err := GraphHash(candidate)
				if err != nil || afterHash == parent.graphHash {
					continue
				}
				result = append(result, topologyProposalV21{graph: candidate, operation: TopologyOperationEvidenceV21{
					Kind: TopologyOperationRedirectTerminalV21, Obligation: obligation,
					AffectedScope: []string{instance.ID, connection.Terminal, connection.Node, targetID}, PrimitiveKey: primitive.Key,
					InstanceID: instance.ID, Terminal: connection.Terminal, BeforeGraphHash: parent.graphHash, AfterGraphHash: afterHash,
				}})
			}
		}
	}
	return result
}

func topologyConnectMissingPortV21(requirement Requirement, parent topologyStateV21, obligation TopologyObligationV21) []topologyProposalV21 {
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		return nil
	}
	result := []topologyProposalV21{}
	for _, node := range initial.Nodes {
		matchesPort := node.Scope == "external" && node.SemanticKind == "port" && node.SemanticID == obligation.ObservationID
		matchesReference := obligation.Kind == TopologyObligationReferenceV21 && node.Scope == "external" &&
			node.Role == "reference" && node.Domain == obligation.Domain
		if !matchesPort && !matchesReference {
			continue
		}
		candidate := CloneGraph(parent.graph)
		candidate.Nodes = append(candidate.Nodes, node)
		candidate, err := NormalizeGraph(candidate)
		if err != nil {
			continue
		}
		afterHash, err := GraphHash(candidate)
		if err != nil || afterHash == parent.graphHash {
			continue
		}
		kind := TopologyOperationConnectPortV21
		if obligation.Kind == TopologyObligationReferenceV21 {
			kind = TopologyOperationAttachReferenceV21
		}
		result = append(result, topologyProposalV21{graph: candidate, operation: TopologyOperationEvidenceV21{
			Kind: kind, Obligation: obligation, AffectedScope: []string{node.ID},
			BeforeGraphHash: parent.graphHash, AfterGraphHash: afterHash,
		}})
	}
	return result
}

func topologyReferenceProposalsV21(requirement Requirement, parent topologyStateV21, inventory PrimitiveInventory, obligation TopologyObligationV21) []topologyProposalV21 {
	return topologyConnectMissingPortV21(requirement, parent, obligation)
}

func topologyRemoveFragmentV21(parent topologyStateV21, obligation TopologyObligationV21) []topologyProposalV21 {
	index := graphInstanceIndex(parent.graph, obligation.InstanceID)
	if index < 0 {
		return nil
	}
	candidate := CloneGraph(parent.graph)
	candidate.Instances = append(candidate.Instances[:index:index], candidate.Instances[index+1:]...)
	used := map[string]bool{}
	for _, instance := range candidate.Instances {
		for _, connection := range instance.Terminals {
			used[connection.Node] = true
		}
	}
	nodes := candidate.Nodes[:0]
	for _, node := range candidate.Nodes {
		if node.Scope == "external" || used[node.ID] {
			nodes = append(nodes, node)
		}
	}
	candidate.Nodes = nodes
	candidate, err := NormalizeGraph(candidate)
	if err != nil {
		return nil
	}
	afterHash, err := GraphHash(candidate)
	if err != nil || afterHash == parent.graphHash {
		return nil
	}
	return []topologyProposalV21{{graph: candidate, operation: TopologyOperationEvidenceV21{
		Kind: TopologyOperationRemoveIrrelevantV21, Obligation: obligation, AffectedScope: []string{obligation.InstanceID},
		InstanceID: obligation.InstanceID, BeforeGraphHash: parent.graphHash, AfterGraphHash: afterHash,
	}}}
}

func topologyJoinPartialPathsV21(ctx context.Context, parent topologyStateV21, inventory PrimitiveInventory) []topologyProposalV21 {
	result := []topologyProposalV21{}
	for _, source := range parent.graph.Instances {
		if ctx.Err() != nil {
			return result
		}
		sourcePrimitive, found := primitiveByKey(inventory, source.PrimitiveKey)
		if !found {
			continue
		}
		sourceContracts := causalTerminalContractsV19(sourcePrimitive)
		for _, output := range source.Terminals {
			role := causalTerminalRoleV19(sourceContracts[output.Terminal])
			if role != "output" && role != "open_collector" && role != "power_output" {
				continue
			}
			outputNode, found := graphNodeByID(parent.graph, output.Node)
			if !found || outputNode.Scope != "internal" {
				continue
			}
			for _, target := range parent.graph.Instances {
				if target.ID == source.ID {
					continue
				}
				targetPrimitive, found := primitiveByKey(inventory, target.PrimitiveKey)
				if !found {
					continue
				}
				targetContracts := causalTerminalContractsV19(targetPrimitive)
				for terminalIndex, input := range target.Terminals {
					if causalTerminalRoleV19(targetContracts[input.Terminal]) != "input" || input.Node == output.Node {
						continue
					}
					inputNode, found := graphNodeByID(parent.graph, input.Node)
					if !found || inputNode.Scope != "internal" {
						continue
					}
					candidate := CloneGraph(parent.graph)
					candidate.Instances[graphInstanceIndex(candidate, target.ID)].Terminals[terminalIndex].Node = output.Node
					candidate, err := NormalizeGraph(candidate)
					if err != nil {
						continue
					}
					afterHash, err := GraphHash(candidate)
					if err != nil || afterHash == parent.graphHash {
						continue
					}
					obligation := topologyObligationV21(TopologyObligationCausalPathV21, "", "", output.Node, input.Node, "", target.ID, false, input.Terminal)
					result = append(result, topologyProposalV21{graph: candidate, operation: TopologyOperationEvidenceV21{
						Kind: TopologyOperationJoinPathsV21, Obligation: obligation, AffectedScope: []string{source.ID, output.Node, target.ID, input.Terminal},
						PrimitiveKey: target.PrimitiveKey, InstanceID: target.ID, Terminal: input.Terminal, BeforeGraphHash: parent.graphHash, AfterGraphHash: afterHash,
					}})
				}
			}
		}
	}
	return result
}

func topologyAnalyzeProposalsV21(ctx context.Context, requirement Requirement, inventory PrimitiveInventory, proposals []topologyProposalV21, workers int) {
	workers = max(1, min(workers, len(proposals)))
	jobs := make(chan int)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				if ctx.Err() == nil {
					proposals[index].report = AnalyzeTopologyV21(requirement, proposals[index].graph, inventory)
				}
			}
		}()
	}
	for index := range proposals {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			group.Wait()
			return
		}
	}
	close(jobs)
	group.Wait()
}

func topologyAssertionV21(requirement Requirement, assertionID, observationID string) (BehavioralAssertion, bool) {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.ID == assertionID && assertion.Observation.Kind == "port" && assertion.Observation.ID == observationID {
			return assertion, true
		}
	}
	return BehavioralAssertion{}, false
}

func topologyPrimitiveSupportsAnalysisV21(primitive PrimitiveCandidate, analysis string) bool {
	if primitive.Key == "" || len(primitive.Models) == 0 {
		return false
	}
	for _, model := range primitive.Models {
		if reviewedPrimitiveModelSupportsCircuitAnalysis(model, analysis) {
			return true
		}
	}
	return false
}

func topologyStrictlyImprovesV21(parent, child topologyStateV21) bool {
	parentCritical, childCritical := 0, 0
	for _, obligation := range parent.report.Obligations {
		if obligation.Critical {
			parentCritical++
		}
	}
	for _, obligation := range child.report.Obligations {
		if obligation.Critical {
			childCritical++
		}
	}
	return cmp.Or(cmp.Compare(childCritical, parentCritical), cmp.Compare(len(child.report.Obligations), len(parent.report.Obligations)), cmp.Compare(len(child.graph.Instances), len(parent.graph.Instances))) < 0
}

func compareTopologyStatesV21(left, right topologyStateV21) int {
	leftCritical, rightCritical := 0, 0
	for _, obligation := range left.report.Obligations {
		if obligation.Critical {
			leftCritical++
		}
	}
	for _, obligation := range right.report.Obligations {
		if obligation.Critical {
			rightCritical++
		}
	}
	return cmp.Or(cmp.Compare(leftCritical, rightCritical), cmp.Compare(len(left.report.Obligations), len(right.report.Obligations)), cmp.Compare(len(left.graph.Instances), len(right.graph.Instances)), cmp.Compare(left.graphHash, right.graphHash))
}

func compareTopologyProposalsV21(left, right topologyProposalV21) int {
	return cmp.Or(cmp.Compare(left.operation.Kind, right.operation.Kind), cmp.Compare(left.operation.Obligation.ID, right.operation.Obligation.ID), cmp.Compare(left.operation.PrimitiveKey, right.operation.PrimitiveKey), cmp.Compare(left.operation.InstanceID, right.operation.InstanceID), cmp.Compare(left.operation.Terminal, right.operation.Terminal), cmp.Compare(left.operation.AfterGraphHash, right.operation.AfterGraphHash))
}

func topologyStateHashV21(state topologyStateV21) string {
	return causalCrossStageHash(struct {
		GraphHash     string                         `json:"graph_sha256"`
		Depth         int                            `json:"depth"`
		Operations    []TopologyOperationEvidenceV21 `json:"operations"`
		InvariantHash string                         `json:"invariant_sha256"`
	}{state.graphHash, state.depth, state.operations, state.report.Hash})
}

func topologyCandidateEvidenceV21(state topologyStateV21, disposition, reason string) TopologyCandidateEvidenceV21 {
	result := TopologyCandidateEvidenceV21{Graph: state.graph, GraphHash: state.graphHash, StateHash: state.stateHash, Depth: state.depth, Operations: slices.Clone(state.operations), Invariant: state.report, Disposition: disposition, Reason: reason}
	if len(state.operations) != 0 {
		result.ParentHash = state.operations[len(state.operations)-1].ParentStateHash
	}
	result.Hash = causalCrossStageHash(result)
	return result
}

func finalizeTopologyOperationV21(operation TopologyOperationEvidenceV21) TopologyOperationEvidenceV21 {
	slices.Sort(operation.AffectedScope)
	operation.AffectedScope = slices.Compact(operation.AffectedScope)
	operation.Hash = ""
	operation.Hash = causalCrossStageHash(operation)
	return operation
}

func finalizeTopologyCompletionPlanV21(result TopologyCompletionPlanV21) TopologyCompletionPlanV21 {
	if result.Candidates == nil {
		result.Candidates = []TopologyCandidateEvidenceV21{}
	}
	if result.Issues == nil {
		result.Issues = []reports.Issue{}
	}
	result.Hash = ""
	result.Hash = causalCrossStageHash(result)
	return result
}

func canceledTopologyCompletionPlanV21(result TopologyCompletionPlanV21) TopologyCompletionPlanV21 {
	result.Status = string(StatusCanceled)
	result.Issues = []reports.Issue{graphIssue(CodeCanceled, "topology.v21", "V21 topology completion canceled", "retry with an active context")}
	return finalizeTopologyCompletionPlanV21(result)
}

func topologyCloneSetV21(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func topologyGraphWithinMemoryV21(graph CandidateGraph, maximum int) bool {
	if maximum <= 0 {
		return false
	}
	// Conservatively bound the canonical JSON representation without allocating
	// and serializing the complete graph for every generated proposal. A JSON
	// string byte can expand to at most six bytes when escaped.
	remaining := maximum
	consume := func(bytes int) bool {
		if bytes < 0 || bytes > remaining {
			return false
		}
		remaining -= bytes
		return true
	}
	consumeString := func(value string) bool {
		if len(value) > remaining/6 {
			return false
		}
		return consume(6 * len(value))
	}
	if !consume(128) || !consumeString(graph.Schema) {
		return false
	}
	for _, node := range graph.Nodes {
		if !consume(96) || !consumeString(node.ID) || !consumeString(node.Scope) ||
			!consumeString(node.SemanticKind) || !consumeString(node.SemanticID) ||
			!consumeString(node.Domain) || !consumeString(node.Role) {
			return false
		}
	}
	for _, instance := range graph.Instances {
		if !consume(112) || !consumeString(instance.ID) || !consumeString(instance.PrimitiveKey) || !consumeString(instance.Kind) {
			return false
		}
		if instance.ValueSI != nil && !consume(32) {
			return false
		}
		for _, terminal := range instance.Terminals {
			if !consume(48) || !consumeString(terminal.Terminal) || !consumeString(terminal.Node) {
				return false
			}
		}
	}
	return true
}
