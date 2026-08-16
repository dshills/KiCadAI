package opentopologysynthesis

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

const (
	CausalOperationInsertRoleCompleteStageV19     = "insert_role_complete_stage"
	CausalOperationAllocateObservationConeV19     = "allocate_independent_observation_cone"
	CausalOperationRedirectRoleTerminalV19        = "redirect_role_terminal"
	CausalOperationInsertTypedFeedbackPathV19     = "insert_typed_feedback_path"
	CausalOperationExistingV19                    = "existing_operation"
	CausalMaximumLogicalChangesPerProposalV19     = 2
	causalOperationAddedPrimitiveCostWeightV19    = 1_000
	causalOperationAddedInternalNodeCostWeightV19 = 1
	causalOperationLogicalChangeCostWeightV19     = 1_000_000
)

var (
	ErrCausalOperationRequestV19     = errors.New("invalid V19 causal-operation request")
	ErrCausalOperationBudgetV19      = errors.New("invalid V19 causal-operation budget")
	ErrCausalOperationInventoryV19   = errors.New("unauthenticated V19 primitive inventory")
	ErrCausalOperationCompositionV19 = errors.New("invalid V19 causal-operation composition")
	causalWordSeparatorsV19          = strings.NewReplacer("_", " ", "-", " ", "/", " ", ".", " ")
)

// CausalOperationBudgetV19 is a caller-provided view of the unchanged global
// policy counters. Operation derivation may consume, but never enlarge, it.
type CausalOperationBudgetV19 struct {
	TopologyRepairs int `json:"topology_repairs"`
	GeneratedGraphs int `json:"generated_graphs"`
}

type CausalOperationConsumptionV19 struct {
	TopologyRepairs   int `json:"topology_repairs"`
	GeneratedGraphs   int `json:"generated_graphs"`
	InvariantRejected int `json:"invariant_rejected"`
}

// CausalLogicalOperationV19 adds V19 provenance to the historical operation
// representation without changing GraphOperation's frozen meaning.
type CausalLogicalOperationV19 struct {
	GraphOperation
	ObligationID  string                 `json:"obligation_id"`
	ObservationID string                 `json:"observation_id"`
	UpstreamNode  string                 `json:"upstream_node"`
	InstanceID    string                 `json:"instance_id,omitempty"`
	CreatedNodes  []string               `json:"created_nodes"`
	CanonicalCost int                    `json:"canonical_cost"`
	FeedbackPath  *CausalFeedbackPathV19 `json:"feedback_path,omitempty"`
}

type CausalOperationProposalV19 struct {
	PlannerKind    string                      `json:"planner_kind"`
	Graph          CandidateGraph              `json:"graph"`
	Context        CausalInvariantContextV19   `json:"context"`
	Operations     []CausalLogicalOperationV19 `json:"operations"`
	LogicalChanges int                         `json:"logical_changes"`
	CanonicalKey   string                      `json:"canonical_key"`
}

type CausalOperationBatchV19 struct {
	Proposals   []CausalOperationProposalV19  `json:"proposals"`
	Consumption CausalOperationConsumptionV19 `json:"consumption"`
	Exhausted   bool                          `json:"exhausted"`
}

type RoleCompleteStageRequestV19 struct {
	ObligationID  string `json:"obligation_id"`
	UpstreamNode  string `json:"upstream_node"`
	ObservationID string `json:"observation_id"`
}

type RoleTerminalRedirectRequestV19 struct {
	ObligationID string `json:"obligation_id"`
	InstanceID   string `json:"instance_id"`
	Terminal     string `json:"terminal"`
	Node         string `json:"node"`
}

type TypedFeedbackPathRequestV19 struct {
	ObligationID string `json:"obligation_id"`
	FromInstance string `json:"from_instance"`
	FromTerminal string `json:"from_terminal"`
	ToInstance   string `json:"to_instance"`
	ToTerminal   string `json:"to_terminal"`
}

type causalStagePlanV19 struct {
	request     RoleCompleteStageRequestV19
	primitive   PrimitiveCandidate
	value       *float64
	connections []TerminalConnection
	critical    bool
}

// InsertRoleCompleteStagesV19 derives complete stage candidates only from the
// authenticated inventory and validates every retained graph at the V19 gate.
func InsertRoleCompleteStagesV19(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	limits GraphLimits,
	context CausalInvariantContextV19,
	request RoleCompleteStageRequestV19,
	budget CausalOperationBudgetV19,
) (CausalOperationBatchV19, error) {
	if err := causalValidateOperationInputsV19(requirement, inventory, budget); err != nil {
		return CausalOperationBatchV19{}, err
	}
	if err := causalValidateCanonicalGraphV19(graph); err != nil {
		return CausalOperationBatchV19{}, err
	}
	plans, err := causalStagePlansV19(Normalize(requirement), graph, inventory, request)
	if err != nil {
		return CausalOperationBatchV19{}, err
	}
	batch := CausalOperationBatchV19{}
	for _, plan := range plans {
		if !causalConsumeOperationV19(&batch, budget, 1) {
			break
		}
		proposal, valid := causalApplyStagePlanV19(requirement, graph, inventory, limits, context, plan, 1)
		if !valid {
			batch.Consumption.InvariantRejected++
			continue
		}
		batch.Proposals = append(batch.Proposals, proposal)
	}
	batch.Proposals = causalSortProposalsV19(batch.Proposals)
	return batch, nil
}

// AllocateIndependentObservationConesV19 applies one or two independently
// derived stage insertions as one bounded proposal. Intermediate graphs are
// hashed for replay but are never returned, simulated, or retained.
func AllocateIndependentObservationConesV19(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	limits GraphLimits,
	context CausalInvariantContextV19,
	requests []RoleCompleteStageRequestV19,
	budget CausalOperationBudgetV19,
) (CausalOperationBatchV19, error) {
	if err := causalValidateOperationInputsV19(requirement, inventory, budget); err != nil {
		return CausalOperationBatchV19{}, err
	}
	if err := causalValidateCanonicalGraphV19(graph); err != nil {
		return CausalOperationBatchV19{}, err
	}
	if len(requests) == 0 || len(requests) > CausalMaximumLogicalChangesPerProposalV19 {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: observation-cone allocation requires one or two requests", ErrCausalOperationRequestV19)
	}
	ordered := slices.Clone(requests)
	slices.SortFunc(ordered, func(left, right RoleCompleteStageRequestV19) int {
		return cmp.Or(cmp.Compare(left.ObservationID, right.ObservationID), cmp.Compare(left.ObligationID, right.ObligationID), cmp.Compare(left.UpstreamNode, right.UpstreamNode))
	})
	for index := 1; index < len(ordered); index++ {
		if ordered[index-1].ObservationID == ordered[index].ObservationID {
			return CausalOperationBatchV19{}, fmt.Errorf("%w: observation cones must be independent", ErrCausalOperationRequestV19)
		}
	}
	planSets := make([][]causalStagePlanV19, len(ordered))
	for index, request := range ordered {
		plans, err := causalStagePlansV19(Normalize(requirement), graph, inventory, request)
		if err != nil {
			return CausalOperationBatchV19{}, err
		}
		if len(plans) == 0 {
			return CausalOperationBatchV19{}, nil
		}
		planSets[index] = plans
	}
	batch := CausalOperationBatchV19{}
	var visit func(int, []causalStagePlanV19)
	visit = func(index int, selected []causalStagePlanV19) {
		if batch.Exhausted {
			return
		}
		if index != len(planSets) {
			for _, plan := range planSets[index] {
				visit(index+1, append(selected, plan))
				if batch.Exhausted {
					return
				}
			}
			return
		}
		if !causalConsumeOperationV19(&batch, budget, len(selected)) {
			return
		}
		current := CloneGraph(graph)
		operations := []CausalLogicalOperationV19{}
		valid := true
		for operationIndex, plan := range selected {
			beforeGraph := current
			beforeHash, hashErr := GraphHash(current)
			if hashErr != nil {
				valid = false
				break
			}
			current = AddPrimitive(current, plan.primitive, plan.value, plan.connections)
			var normalizeErr error
			current, normalizeErr = NormalizeGraph(current)
			if normalizeErr != nil {
				valid = false
				break
			}
			afterHash, hashErr := GraphHash(current)
			if hashErr != nil {
				valid = false
				break
			}
			instanceID := causalAddedInstanceIDV19(beforeGraph, current)
			if instanceID == "" {
				valid = false
				break
			}
			normalizedPlan := plan
			normalizedPlan.connections = causalInstanceConnectionsV19(current, instanceID)
			operations = append(operations, causalStageOperationV19(normalizedPlan, operationIndex+1, instanceID, beforeHash, afterHash))
		}
		if !valid {
			batch.Consumption.InvariantRejected++
			return
		}
		if len(ValidateCausalGraphV19(requirement, current, inventory, limits, context)) != 0 {
			batch.Consumption.InvariantRejected++
			return
		}
		proposal := CausalOperationProposalV19{
			PlannerKind: CausalOperationAllocateObservationConeV19,
			Graph:       current, Context: causalCloneContextV19(context), Operations: operations, LogicalChanges: len(operations),
		}
		proposal.CanonicalKey = causalProposalKeyV19(proposal)
		batch.Proposals = append(batch.Proposals, proposal)
	}
	visit(0, nil)
	batch.Proposals = causalSortProposalsV19(batch.Proposals)
	return batch, nil
}

// RedirectRoleTerminalV19 rebinds exactly one inventory-declared terminal and
// retains the primitive's complete contract. It never calls the historical,
// terminal-name-based compatibility helper.
func RedirectRoleTerminalV19(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	limits GraphLimits,
	context CausalInvariantContextV19,
	request RoleTerminalRedirectRequestV19,
	budget CausalOperationBudgetV19,
) (CausalOperationBatchV19, error) {
	if err := causalValidateOperationInputsV19(requirement, inventory, budget); err != nil {
		return CausalOperationBatchV19{}, err
	}
	if err := causalValidateCanonicalGraphV19(graph); err != nil {
		return CausalOperationBatchV19{}, err
	}
	assertion, err := causalOperationAssertionV19(Normalize(requirement), request.ObligationID, "")
	if err != nil {
		return CausalOperationBatchV19{}, err
	}
	instanceIndex := graphInstanceIndex(graph, request.InstanceID)
	if instanceIndex < 0 {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: unknown instance %q", ErrCausalOperationRequestV19, request.InstanceID)
	}
	primitive, found := primitiveByKey(inventory, graph.Instances[instanceIndex].PrimitiveKey)
	if !found {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: missing primitive %q", ErrCausalOperationInventoryV19, graph.Instances[instanceIndex].PrimitiveKey)
	}
	terminal, found := primitiveTerminalByName(primitive, request.Terminal)
	if !found {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: unknown terminal %q", ErrCausalOperationRequestV19, request.Terminal)
	}
	node, found := graphNodeByID(graph, request.Node)
	if !found || !causalTerminalNodeCompatibleV19(terminal, node) {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: incompatible terminal redirection", ErrCausalOperationRequestV19)
	}
	oldNode := GraphNode{}
	terminalIndex := -1
	for index, connection := range graph.Instances[instanceIndex].Terminals {
		if connection.Terminal == terminal.Terminal {
			terminalIndex = index
			oldNode, _ = graphNodeByID(graph, connection.Node)
			break
		}
	}
	if terminalIndex < 0 || graph.Instances[instanceIndex].Terminals[terminalIndex].Node == request.Node {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: terminal is absent or already bound", ErrCausalOperationRequestV19)
	}
	role := causalTerminalRoleV19(terminal)
	if oldNode.Domain != "" && node.Domain != "" && oldNode.Domain != node.Domain && role != "power_input" {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: redirection would invent a domain bridge", ErrCausalOperationRequestV19)
	}
	batch := CausalOperationBatchV19{}
	if !causalConsumeOperationV19(&batch, budget, 1) {
		return batch, nil
	}
	beforeHash, hashErr := GraphHash(graph)
	if hashErr != nil {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: graph hash failed", ErrCausalOperationRequestV19)
	}
	candidate := CloneGraph(graph)
	candidate.Instances[instanceIndex].Terminals[terminalIndex].Node = request.Node
	slices.SortFunc(candidate.Instances[instanceIndex].Terminals, compareTerminalConnections)
	candidate, err = NormalizeGraph(candidate)
	if err != nil {
		batch.Consumption.InvariantRejected++
		return batch, nil
	}
	afterHash, hashErr := GraphHash(candidate)
	if hashErr != nil {
		batch.Consumption.InvariantRejected++
		return batch, nil
	}
	if len(ValidateCausalGraphV19(requirement, candidate, inventory, limits, context)) != 0 {
		batch.Consumption.InvariantRejected++
		return batch, nil
	}
	operation := CausalLogicalOperationV19{
		GraphOperation: GraphOperation{Number: 1, Kind: CausalOperationRedirectRoleTerminalV19, PrimitiveKey: primitive.Key, PrimitiveKind: primitive.Kind, Node: request.Node,
			Connections: []TerminalConnection{{Terminal: terminal.Terminal, Node: request.Node}}, BeforeHash: beforeHash, AfterHash: afterHash},
		ObligationID: request.ObligationID, ObservationID: assertion.Observation.ID, InstanceID: request.InstanceID,
		CreatedNodes: []string{}, CanonicalCost: causalOperationLogicalChangeCostWeightV19,
	}
	proposal := CausalOperationProposalV19{PlannerKind: CausalOperationRedirectRoleTerminalV19, Graph: candidate, Context: causalCloneContextV19(context), Operations: []CausalLogicalOperationV19{operation}, LogicalChanges: 1}
	proposal.CanonicalKey = causalProposalKeyV19(proposal)
	batch.Proposals = []CausalOperationProposalV19{proposal}
	return batch, nil
}

// InsertTypedFeedbackPathsV19 is the only Phase-2 operation that may record a
// directed back-edge. The physical edge is always a reviewed passive primitive.
func InsertTypedFeedbackPathsV19(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	limits GraphLimits,
	context CausalInvariantContextV19,
	request TypedFeedbackPathRequestV19,
	budget CausalOperationBudgetV19,
) (CausalOperationBatchV19, error) {
	if err := causalValidateOperationInputsV19(requirement, inventory, budget); err != nil {
		return CausalOperationBatchV19{}, err
	}
	if err := causalValidateCanonicalGraphV19(graph); err != nil {
		return CausalOperationBatchV19{}, err
	}
	normalizedRequirement := Normalize(requirement)
	assertion, err := causalOperationAssertionV19(normalizedRequirement, request.ObligationID, "")
	if err != nil || !causalFeedbackSensitiveMetricV19(assertion.Metric) {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: obligation is not feedback-sensitive", ErrCausalOperationRequestV19)
	}
	instances := map[string]GraphInstance{}
	for _, instance := range graph.Instances {
		instances[instance.ID] = instance
	}
	fromInstance, fromOK := instances[request.FromInstance]
	toInstance, toOK := instances[request.ToInstance]
	if !fromOK || !toOK {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: feedback endpoint instance is absent", ErrCausalOperationRequestV19)
	}
	fromConnection, fromTerminal, fromOK := causalOperationTerminalV19(fromInstance, inventory, request.FromTerminal)
	toConnection, toTerminal, toOK := causalOperationTerminalV19(toInstance, inventory, request.ToTerminal)
	fromRole := causalTerminalRoleV19(fromTerminal)
	if !fromOK || !toOK || (fromRole != "output" && fromRole != "open_collector" && fromRole != "power_output") || causalTerminalRoleV19(toTerminal) != "input" {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: feedback must run from output to input", ErrCausalOperationRequestV19)
	}
	fromNode, _ := graphNodeByID(graph, fromConnection.Node)
	toNode, _ := graphNodeByID(graph, toConnection.Node)
	if fromNode.Role == "supply" || fromNode.Role == "reference" || toNode.Role == "supply" || toNode.Role == "reference" {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: feedback may not use supply/reference endpoints", ErrCausalOperationRequestV19)
	}
	feedback := CausalFeedbackPathV19{FromInstance: request.FromInstance, FromTerminal: request.FromTerminal, ToInstance: request.ToInstance, ToTerminal: request.ToTerminal, ObligationID: request.ObligationID}
	beforeHash, hashErr := GraphHash(graph)
	if hashErr != nil {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: graph hash failed", ErrCausalOperationRequestV19)
	}
	primitives := slices.Clone(inventory.Primitives)
	slices.SortFunc(primitives, comparePrimitiveCandidates)
	batch := CausalOperationBatchV19{}
	for _, primitive := range primitives {
		contracts := causalTerminalContractsV19(primitive)
		if len(primitive.Terminals) != 2 || !causalPurePassiveV19(contracts, GraphInstance{Terminals: []TerminalConnection{{Terminal: primitive.Terminals[0].Terminal}, {Terminal: primitive.Terminals[1].Terminal}}}) || !causalPrimitiveAnalysesV19(primitive, causalRequiredAnalysesV19(normalizedRequirement)) {
			continue
		}
		value, ok := causalPrimitiveDefaultValueV19(primitive)
		if !ok {
			continue
		}
		if !causalConsumeOperationV19(&batch, budget, 1) {
			break
		}
		connections := []TerminalConnection{{Terminal: primitive.Terminals[0].Terminal, Node: fromConnection.Node}, {Terminal: primitive.Terminals[1].Terminal, Node: toConnection.Node}}
		slices.SortFunc(connections, compareTerminalConnections)
		candidate := AddPrimitive(graph, primitive, value, connections)
		candidate, normalizeErr := NormalizeGraph(candidate)
		if normalizeErr != nil {
			batch.Consumption.InvariantRejected++
			continue
		}
		afterHash, hashErr := GraphHash(candidate)
		if hashErr != nil {
			batch.Consumption.InvariantRejected++
			continue
		}
		candidateContext := causalCloneContextV19(context)
		candidateContext.FeedbackPaths = append(candidateContext.FeedbackPaths, feedback)
		if len(ValidateCausalGraphV19(requirement, candidate, inventory, limits, candidateContext)) != 0 {
			batch.Consumption.InvariantRejected++
			continue
		}
		instanceID := causalAddedInstanceIDV19(graph, candidate)
		if instanceID == "" {
			batch.Consumption.InvariantRejected++
			continue
		}
		connections = causalInstanceConnectionsV19(candidate, instanceID)
		operation := CausalLogicalOperationV19{
			GraphOperation: GraphOperation{Number: 1, Kind: CausalOperationInsertTypedFeedbackPathV19, PrimitiveKey: primitive.Key, PrimitiveKind: primitive.Kind, Connections: connections, ValueSI: cloneInventoryFloat(value), BeforeHash: beforeHash, AfterHash: afterHash},
			ObligationID:   request.ObligationID, ObservationID: assertion.Observation.ID, UpstreamNode: fromConnection.Node, InstanceID: instanceID,
			CreatedNodes: []string{}, CanonicalCost: causalOperationLogicalChangeCostWeightV19 + causalOperationAddedPrimitiveCostWeightV19, FeedbackPath: &feedback,
		}
		proposal := CausalOperationProposalV19{PlannerKind: CausalOperationInsertTypedFeedbackPathV19, Graph: candidate, Context: candidateContext, Operations: []CausalLogicalOperationV19{operation}, LogicalChanges: 1}
		proposal.CanonicalKey = causalProposalKeyV19(proposal)
		batch.Proposals = append(batch.Proposals, proposal)
	}
	batch.Proposals = causalSortProposalsV19(batch.Proposals)
	return batch, nil
}

// RecordExistingCausalOperationV19 authenticates one historical value,
// polarity, passive, substitution, or passive-terminal operation without
// redefining how that operation mutates a graph.
func RecordExistingCausalOperationV19(
	requirement Requirement,
	before CandidateGraph,
	after CandidateGraph,
	inventory PrimitiveInventory,
	limits GraphLimits,
	context CausalInvariantContextV19,
	operation GraphOperation,
	obligationID string,
) (CausalOperationProposalV19, error) {
	if err := causalValidateInventoryV19(inventory); err != nil {
		return CausalOperationProposalV19{}, err
	}
	if err := causalValidateCanonicalGraphV19(before); err != nil {
		return CausalOperationProposalV19{}, err
	}
	if err := causalValidateCanonicalGraphV19(after); err != nil {
		return CausalOperationProposalV19{}, err
	}
	assertion, err := causalOperationAssertionV19(Normalize(requirement), obligationID, "")
	if err != nil {
		return CausalOperationProposalV19{}, err
	}
	beforeHash, beforeErr := GraphHash(before)
	afterHash, afterErr := GraphHash(after)
	if beforeErr != nil || afterErr != nil || operation.BeforeHash != beforeHash || operation.AfterHash != afterHash {
		return CausalOperationProposalV19{}, fmt.Errorf("%w: historical operation hashes do not match graph bytes", ErrCausalOperationCompositionV19)
	}
	kind, instanceID, addedPrimitives, addedNodes, ok := causalExistingDeltaV19(before, after, inventory)
	if !ok || kind != operation.Kind || !causalExistingOperationEvidenceV19(operation, before, after, instanceID) {
		return CausalOperationProposalV19{}, fmt.Errorf("%w: historical operation metadata does not match its graph delta", ErrCausalOperationCompositionV19)
	}
	normalized, err := NormalizeGraph(after)
	if err != nil || len(ValidateCausalGraphV19(requirement, normalized, inventory, limits, context)) != 0 {
		return CausalOperationProposalV19{}, fmt.Errorf("%w: historical operation result fails V19 invariants", ErrCausalOperationCompositionV19)
	}
	logical := CausalLogicalOperationV19{
		GraphOperation: operation, ObligationID: obligationID, ObservationID: assertion.Observation.ID, InstanceID: instanceID,
		CreatedNodes:  causalAddedInternalNodesV19(before, after),
		CanonicalCost: causalOperationLogicalChangeCostWeightV19 + addedPrimitives*causalOperationAddedPrimitiveCostWeightV19 + addedNodes*causalOperationAddedInternalNodeCostWeightV19,
	}
	logical.Number = 1
	proposal := CausalOperationProposalV19{PlannerKind: CausalOperationExistingV19, Graph: normalized, Context: causalCloneContextV19(context), Operations: []CausalLogicalOperationV19{logical}, LogicalChanges: 1}
	proposal.CanonicalKey = causalProposalKeyV19(proposal)
	return proposal, nil
}

// ComposeCausalProposalsV19 joins two authenticated one-change proposals. It
// rejects two V19-new operations, stale hash chains, and all budget overflow.
func ComposeCausalProposalsV19(
	requirement Requirement,
	inventory PrimitiveInventory,
	limits GraphLimits,
	first CausalOperationProposalV19,
	second CausalOperationProposalV19,
	budget CausalOperationBudgetV19,
) (CausalOperationBatchV19, error) {
	if err := causalValidateOperationInputsV19(requirement, inventory, budget); err != nil {
		return CausalOperationBatchV19{}, err
	}
	if !causalProposalEvidenceValidV19(first) || !causalProposalEvidenceValidV19(second) {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: proposal evidence is not self-authenticating", ErrCausalOperationCompositionV19)
	}
	operations := append(causalCloneOperationsV19(first.Operations), causalCloneOperationsV19(second.Operations)...)
	if len(operations) == 0 || len(operations) > CausalMaximumLogicalChangesPerProposalV19 {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: proposal must contain one or two logical changes", ErrCausalOperationCompositionV19)
	}
	newCount := 0
	for _, operation := range operations {
		if causalNewOperationKindV19(operation.Kind) {
			newCount++
		} else if !causalExistingOperationKindV19(operation.Kind) {
			return CausalOperationBatchV19{}, fmt.Errorf("%w: unsupported operation kind %q", ErrCausalOperationCompositionV19, operation.Kind)
		}
	}
	if newCount > 1 {
		return CausalOperationBatchV19{}, fmt.Errorf("%w: at most one V19-new operation is allowed", ErrCausalOperationCompositionV19)
	}
	for index := 1; index < len(operations); index++ {
		if operations[index-1].AfterHash != operations[index].BeforeHash {
			return CausalOperationBatchV19{}, fmt.Errorf("%w: operation hash chain is discontinuous", ErrCausalOperationCompositionV19)
		}
	}
	batch := CausalOperationBatchV19{}
	if !causalConsumeOperationV19(&batch, budget, len(operations)) {
		return batch, nil
	}
	context := causalMergeContextsV19(first.Context, second.Context)
	if len(ValidateCausalGraphV19(requirement, second.Graph, inventory, limits, context)) != 0 {
		batch.Consumption.InvariantRejected++
		return batch, nil
	}
	for index := range operations {
		operations[index].Number = index + 1
	}
	proposal := CausalOperationProposalV19{PlannerKind: "coordinated_pair", Graph: CloneGraph(second.Graph), Context: context, Operations: operations, LogicalChanges: len(operations)}
	proposal.CanonicalKey = causalProposalKeyV19(proposal)
	batch.Proposals = []CausalOperationProposalV19{proposal}
	return batch, nil
}

func causalValidateOperationInputsV19(requirement Requirement, inventory PrimitiveInventory, budget CausalOperationBudgetV19) error {
	if issues := Validate(Normalize(requirement)); len(issues) != 0 {
		return fmt.Errorf("%w: requirement contract failed", ErrCausalOperationRequestV19)
	}
	if err := causalValidateInventoryV19(inventory); err != nil {
		return err
	}
	policy := DefaultPolicy()
	if budget.TopologyRepairs < 0 || budget.GeneratedGraphs < 0 || budget.TopologyRepairs > policy.MaxTopologyRepairs || budget.GeneratedGraphs > policy.MaxGeneratedGraphs {
		return fmt.Errorf("%w: remaining counters exceed the inherited policy", ErrCausalOperationBudgetV19)
	}
	return nil
}

func causalValidateCanonicalGraphV19(graph CandidateGraph) error {
	if graph.Schema != CandidateGraphSchema || graph.Version != CandidateGraphVersion || !graphUsesCanonicalIDs(graph) {
		return fmt.Errorf("%w: operation graph must use canonical normalized IDs", ErrCausalOperationRequestV19)
	}
	return nil
}

func causalValidateInventoryV19(inventory PrimitiveInventory) error {
	if inventory.Schema != PrimitiveInventorySchema || inventory.Version != PrimitiveInventoryVersion ||
		!causalSHA256V19(inventory.CatalogHash) || !causalSHA256V19(inventory.ModelRegistryHash) ||
		!causalSHA256V19(inventory.PrimitiveRegistry) || !causalSHA256V19(inventory.Hash) ||
		inventory.PrimitiveRegistry != primitiveRegistryHash() {
		return ErrCausalOperationInventoryV19
	}
	computed, err := primitiveInventoryHash(inventory)
	if err != nil || computed != inventory.Hash {
		return ErrCausalOperationInventoryV19
	}
	keys := map[string]bool{}
	for _, primitive := range inventory.Primitives {
		if primitive.Key == "" || keys[primitive.Key] {
			return ErrCausalOperationInventoryV19
		}
		keys[primitive.Key] = true
	}
	return nil
}

func causalConsumeOperationV19(batch *CausalOperationBatchV19, budget CausalOperationBudgetV19, changes int) bool {
	if changes <= 0 || changes > CausalMaximumLogicalChangesPerProposalV19 ||
		batch.Consumption.TopologyRepairs+changes > budget.TopologyRepairs ||
		batch.Consumption.GeneratedGraphs+1 > budget.GeneratedGraphs {
		batch.Exhausted = true
		return false
	}
	batch.Consumption.TopologyRepairs += changes
	batch.Consumption.GeneratedGraphs++
	return true
}

func causalStagePlansV19(requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, request RoleCompleteStageRequestV19) ([]causalStagePlanV19, error) {
	assertion, err := causalOperationAssertionV19(requirement, request.ObligationID, request.ObservationID)
	if err != nil {
		return nil, err
	}
	upstream, found := graphNodeByID(graph, request.UpstreamNode)
	if !found || upstream.Role == "output" || upstream.Role == "reference" {
		return nil, fmt.Errorf("%w: upstream node is absent or cannot excite a stage", ErrCausalOperationRequestV19)
	}
	observationNode, found := ExternalNodeForObservation(graph, requirement, assertion.Observation)
	if !found {
		return nil, fmt.Errorf("%w: observation has no external graph node", ErrCausalOperationRequestV19)
	}
	observation, _ := graphNodeByID(graph, observationNode)
	if observation.Role != "output" {
		return nil, fmt.Errorf("%w: observation is not a source output", ErrCausalOperationRequestV19)
	}
	primitives := slices.Clone(inventory.Primitives)
	slices.SortFunc(primitives, comparePrimitiveCandidates)
	analyses := causalRequiredAnalysesV19(requirement)
	plans := []causalStagePlanV19{}
	for _, primitive := range primitives {
		if !causalPrimitiveAnalysesV19(primitive, analyses) {
			continue
		}
		value, ok := causalPrimitiveDefaultValueV19(primitive)
		if !ok {
			continue
		}
		for _, connections := range causalStageConnectionMapsV19(graph, primitive, upstream, observation) {
			plans = append(plans, causalStagePlanV19{request: request, primitive: primitive, value: value, connections: connections, critical: assertion.Critical})
		}
	}
	slices.SortFunc(plans, causalCompareStagePlansV19)
	return plans, nil
}

func causalOperationAssertionV19(requirement Requirement, obligationID, observationID string) (BehavioralAssertion, error) {
	if strings.TrimSpace(obligationID) == "" {
		return BehavioralAssertion{}, fmt.Errorf("%w: diagnosed obligation is required", ErrCausalOperationRequestV19)
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.ID != obligationID {
			continue
		}
		if assertion.Observation.Kind != "port" || (observationID != "" && assertion.Observation.ID != observationID) {
			return BehavioralAssertion{}, fmt.Errorf("%w: obligation observation mismatch", ErrCausalOperationRequestV19)
		}
		return assertion, nil
	}
	return BehavioralAssertion{}, fmt.Errorf("%w: unknown diagnosed obligation", ErrCausalOperationRequestV19)
}

func causalPrimitiveAnalysesV19(primitive PrimitiveCandidate, analyses []string) bool {
	if primitive.Key == "" || primitive.CatalogID == "" || primitive.VariantID == "" || primitive.Kind == "" || primitive.Evidence == "" || primitive.FootprintID == "" || primitive.PackageType == "" || len(primitive.SymbolIDs) == 0 || len(primitive.Models) == 0 {
		return false
	}
	for _, analysis := range analyses {
		supported := false
		for _, model := range primitive.Models {
			if model.ModelID != "" && model.Family != "" && model.ProvenanceSource != "" && model.ProvenanceRevision != "" && causalSHA256V19(model.ProvenanceSHA256) && reviewedPrimitiveModelSupportsCircuitAnalysis(model, analysis) {
				supported = true
				break
			}
		}
		if !supported {
			return false
		}
	}
	return true
}

func causalPrimitiveDefaultValueV19(primitive PrimitiveCandidate) (*float64, bool) {
	if primitive.ValueDomain == nil {
		return nil, true
	}
	if primitive.ValueDomain.Nominal == nil || !finite(*primitive.ValueDomain.Nominal) || *primitive.ValueDomain.Nominal <= 0 || !valueWithinPrimitiveDomain(*primitive.ValueDomain.Nominal, *primitive.ValueDomain) {
		return nil, false
	}
	return cloneInventoryFloat(primitive.ValueDomain.Nominal), true
}

func causalStageConnectionMapsV19(graph CandidateGraph, primitive PrimitiveCandidate, upstream, observation GraphNode) [][]TerminalConnection {
	terminals := slices.Clone(primitive.Terminals)
	slices.SortFunc(terminals, func(left, right PrimitiveTerminal) int { return cmp.Compare(left.Terminal, right.Terminal) })
	inputs := []PrimitiveTerminal{}
	outputs := []PrimitiveTerminal{}
	powerInputs := []PrimitiveTerminal{}
	for _, terminal := range terminals {
		switch causalTerminalRoleV19(terminal) {
		case "input":
			inputs = append(inputs, terminal)
		case "output", "open_collector", "power_output":
			outputs = append(outputs, terminal)
		case "power_input":
			powerInputs = append(powerInputs, terminal)
		}
	}
	if len(inputs) == 0 && len(outputs) == 0 && len(terminals) == 2 {
		for _, terminal := range terminals {
			if causalTerminalRoleV19(terminal) != "passive" {
				return nil
			}
		}
		result := [][]TerminalConnection{
			{{Terminal: terminals[0].Terminal, Node: upstream.ID}, {Terminal: terminals[1].Terminal, Node: observation.ID}},
			{{Terminal: terminals[0].Terminal, Node: observation.ID}, {Terminal: terminals[1].Terminal, Node: upstream.ID}},
		}
		for index := range result {
			slices.SortFunc(result[index], compareTerminalConnections)
		}
		slices.SortFunc(result, causalCompareConnectionMapsV19)
		return slices.CompactFunc(result, func(left, right []TerminalConnection) bool { return causalCompareConnectionMapsV19(left, right) == 0 })
	}
	if len(outputs) != 1 || (len(inputs) == 0 && (upstream.Role != "supply" || len(powerInputs) == 0)) {
		return nil
	}
	primaryInputs := inputs
	if upstream.Role == "supply" && len(powerInputs) != 0 {
		primaryInputs = nil
		for _, terminal := range powerInputs {
			if !causalNegativePowerTerminalV19(terminal) {
				primaryInputs = append(primaryInputs, terminal)
			}
		}
	}
	if len(primaryInputs) == 0 {
		return nil
	}
	result := [][]TerminalConnection{}
	for _, primary := range primaryInputs {
		options := make([][]string, len(terminals))
		valid := true
		for index, terminal := range terminals {
			role := causalTerminalRoleV19(terminal)
			switch {
			case terminal.Terminal == primary.Terminal:
				options[index] = []string{upstream.ID}
			case role == "output" || role == "open_collector" || role == "power_output":
				options[index] = []string{observation.ID}
			case role == "input":
				options[index] = causalReferenceNodesV19(graph, upstream.Domain, observation.Domain)
			case role == "power_input":
				options[index] = causalPowerNodesForTerminalV19(graph, terminal)
			case role == "passive":
				options[index] = causalReferenceNodesV19(graph, upstream.Domain, observation.Domain)
			case role == "ignore" && terminal.DefaultNet != "":
				options[index] = causalDefaultNetNodesV19(graph, terminal.DefaultNet)
			default:
				if terminal.DefaultNet != "" {
					options[index] = causalDefaultNetNodesV19(graph, terminal.DefaultNet)
				} else {
					valid = false
				}
			}
			if len(options[index]) == 0 {
				valid = false
			}
		}
		if !valid {
			continue
		}
		var expand func(int, []TerminalConnection, map[string]string)
		expand = func(index int, connections []TerminalConnection, powerBindings map[string]string) {
			if index == len(terminals) {
				if len(connections) < 2 {
					return
				}
				ordered := slices.Clone(connections)
				slices.SortFunc(ordered, compareTerminalConnections)
				result = append(result, ordered)
				return
			}
			terminal := terminals[index]
			for _, nodeID := range options[index] {
				role := causalTerminalRoleV19(terminal)
				powerClass := ""
				if role == "power_input" {
					powerClass = causalPowerTerminalClassV19(terminal)
					if prior, exists := powerBindings[nodeID]; exists && prior != powerClass {
						continue
					}
				}
				nextBindings := powerBindings
				if role == "power_input" {
					nextBindings = make(map[string]string, len(powerBindings)+1)
					for key, value := range powerBindings {
						nextBindings[key] = value
					}
					nextBindings[nodeID] = powerClass
				}
				expand(index+1, append(connections, TerminalConnection{Terminal: terminal.Terminal, Node: nodeID}), nextBindings)
			}
		}
		expand(0, nil, map[string]string{})
	}
	slices.SortFunc(result, causalCompareConnectionMapsV19)
	result = slices.CompactFunc(result, func(left, right []TerminalConnection) bool { return causalCompareConnectionMapsV19(left, right) == 0 })
	return result
}

func causalReferenceNodesV19(graph CandidateGraph, domains ...string) []string {
	allowed := map[string]bool{}
	for _, domain := range domains {
		if domain != "" {
			allowed[domain] = true
		}
	}
	result := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "reference" && (len(allowed) == 0 || allowed[node.Domain]) {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func causalPowerNodesForTerminalV19(graph CandidateGraph, terminal PrimitiveTerminal) []string {
	powerClass := causalPowerTerminalClassV19(terminal)
	result := []string{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" {
			continue
		}
		switch {
		case powerClass == "negative":
			if node.Role == "reference" || node.Role == "supply" {
				result = append(result, node.ID)
			}
		case powerClass == "positive":
			if node.Role == "supply" {
				result = append(result, node.ID)
			}
		default:
			if node.Role == "supply" || node.Role == "reference" {
				result = append(result, node.ID)
			}
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func causalNegativePowerTerminalV19(terminal PrimitiveTerminal) bool {
	return causalPowerTerminalClassV19(terminal) == "negative"
}

func causalPowerTerminalClassV19(terminal PrimitiveTerminal) string {
	description := strings.ToLower(strings.Join([]string{terminal.Function, terminal.Polarity, terminal.Terminal}, " "))
	switch {
	case causalContainsWordV19(description, "negative", "minus", "return", "ground", "gnd", "vss"):
		return "negative"
	case causalContainsWordV19(description, "positive", "plus", "vcc", "vdd"):
		return "positive"
	default:
		return "unspecified:" + strings.ToLower(strings.TrimSpace(terminal.Terminal))
	}
}

func causalContainsWordV19(value string, words ...string) bool {
	parts := strings.Fields(causalWordSeparatorsV19.Replace(value))
	for _, part := range parts {
		for len(part) != 0 && part[len(part)-1] >= '0' && part[len(part)-1] <= '9' {
			part = part[:len(part)-1]
		}
		for _, word := range words {
			if part == word {
				return true
			}
		}
	}
	return false
}

func causalDefaultNetNodesV19(graph CandidateGraph, defaultNet string) []string {
	defaultNet = canonicalIdentifier(defaultNet)
	result := []string{}
	for _, node := range graph.Nodes {
		if canonicalIdentifier(node.ID) == defaultNet || canonicalIdentifier(node.SemanticID) == defaultNet || canonicalIdentifier(node.Domain) == defaultNet {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func causalTerminalNodeCompatibleV19(terminal PrimitiveTerminal, node GraphNode) bool {
	switch causalTerminalRoleV19(terminal) {
	case "input":
		return slices.Contains([]string{"input", "control", "output", "internal", "reference"}, node.Role)
	case "output", "open_collector", "power_output":
		return node.Role == "output" || node.Role == "internal"
	case "power_input":
		return node.Scope == "external" && (node.Role == "supply" || node.Role == "reference")
	case "passive":
		return node.Role != ""
	default:
		return false
	}
}

func causalApplyStagePlanV19(requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, limits GraphLimits, context CausalInvariantContextV19, plan causalStagePlanV19, number int) (CausalOperationProposalV19, bool) {
	beforeHash, beforeErr := GraphHash(graph)
	candidate := AddPrimitive(graph, plan.primitive, plan.value, plan.connections)
	candidate, normalizeErr := NormalizeGraph(candidate)
	if beforeErr != nil || normalizeErr != nil {
		return CausalOperationProposalV19{}, false
	}
	afterHash, afterErr := GraphHash(candidate)
	if afterErr != nil || len(ValidateCausalGraphV19(requirement, candidate, inventory, limits, context)) != 0 {
		return CausalOperationProposalV19{}, false
	}
	instanceID := causalAddedInstanceIDV19(graph, candidate)
	if instanceID == "" {
		return CausalOperationProposalV19{}, false
	}
	normalizedPlan := plan
	normalizedPlan.connections = causalInstanceConnectionsV19(candidate, instanceID)
	operation := causalStageOperationV19(normalizedPlan, number, instanceID, beforeHash, afterHash)
	proposal := CausalOperationProposalV19{PlannerKind: CausalOperationInsertRoleCompleteStageV19, Graph: candidate, Context: causalCloneContextV19(context), Operations: []CausalLogicalOperationV19{operation}, LogicalChanges: 1}
	proposal.CanonicalKey = causalProposalKeyV19(proposal)
	return proposal, true
}

func causalStageOperationV19(plan causalStagePlanV19, number int, instanceID, beforeHash, afterHash string) CausalLogicalOperationV19 {
	return CausalLogicalOperationV19{
		GraphOperation: GraphOperation{Number: number, Kind: CausalOperationInsertRoleCompleteStageV19, PrimitiveKey: plan.primitive.Key, PrimitiveKind: plan.primitive.Kind,
			Connections: slices.Clone(plan.connections), ValueSI: cloneInventoryFloat(plan.value), BeforeHash: beforeHash, AfterHash: afterHash},
		ObligationID: plan.request.ObligationID, ObservationID: plan.request.ObservationID, UpstreamNode: plan.request.UpstreamNode, InstanceID: instanceID,
		CreatedNodes: []string{}, CanonicalCost: causalOperationLogicalChangeCostWeightV19 + causalOperationAddedPrimitiveCostWeightV19,
	}
}

func causalAddedInstanceIDV19(before, after CandidateGraph) string {
	if !graphUsesCanonicalIDs(before) || !graphUsesCanonicalIDs(after) || len(after.Instances) != len(before.Instances)+1 {
		return ""
	}
	afterByID := make(map[string]GraphInstance, len(after.Instances))
	for _, instance := range after.Instances {
		afterByID[instance.ID] = causalCanonicalInstanceEvidenceV19(instance)
	}
	for _, instance := range before.Instances {
		preserved, found := afterByID[instance.ID]
		if !found || !reflect.DeepEqual(causalCanonicalInstanceEvidenceV19(instance), preserved) {
			return ""
		}
		delete(afterByID, instance.ID)
	}
	if len(afterByID) != 1 {
		return ""
	}
	for instanceID := range afterByID {
		return instanceID
	}
	return ""
}

func causalCanonicalInstanceEvidenceV19(source GraphInstance) GraphInstance {
	instance := source
	instance.ValueSI = cloneInventoryFloat(source.ValueSI)
	instance.Terminals = slices.Clone(source.Terminals)
	canonicalizeSymmetricTerminals(&instance)
	slices.SortFunc(instance.Terminals, compareTerminalConnections)
	return instance
}

func causalInstanceConnectionsV19(graph CandidateGraph, instanceID string) []TerminalConnection {
	for _, instance := range graph.Instances {
		if instance.ID == instanceID {
			return slices.Clone(instance.Terminals)
		}
	}
	return nil
}

func causalOperationTerminalV19(instance GraphInstance, inventory PrimitiveInventory, terminalID string) (TerminalConnection, PrimitiveTerminal, bool) {
	primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
	if !found {
		return TerminalConnection{}, PrimitiveTerminal{}, false
	}
	terminal, found := primitiveTerminalByName(primitive, terminalID)
	if !found {
		return TerminalConnection{}, PrimitiveTerminal{}, false
	}
	for _, connection := range instance.Terminals {
		if connection.Terminal == terminal.Terminal {
			return connection, terminal, true
		}
	}
	return TerminalConnection{}, PrimitiveTerminal{}, false
}

func causalExistingDeltaV19(before, after CandidateGraph, inventory PrimitiveInventory) (string, string, int, int, bool) {
	beforeNodes, afterNodes := map[string]GraphNode{}, map[string]GraphNode{}
	for _, node := range before.Nodes {
		beforeNodes[node.ID] = node
	}
	for _, node := range after.Nodes {
		afterNodes[node.ID] = node
	}
	addedNodes := []GraphNode{}
	for id, node := range afterNodes {
		if _, exists := beforeNodes[id]; !exists {
			addedNodes = append(addedNodes, node)
		}
	}
	for id, node := range beforeNodes {
		if current, exists := afterNodes[id]; !exists || !reflect.DeepEqual(node, current) {
			return "", "", 0, 0, false
		}
	}
	beforeInstances, afterInstances := map[string]GraphInstance{}, map[string]GraphInstance{}
	for _, instance := range before.Instances {
		beforeInstances[instance.ID] = instance
	}
	for _, instance := range after.Instances {
		afterInstances[instance.ID] = instance
	}
	addedInstances := []GraphInstance{}
	changed := []string{}
	for id, instance := range afterInstances {
		prior, exists := beforeInstances[id]
		if !exists {
			addedInstances = append(addedInstances, instance)
		} else if !reflect.DeepEqual(prior, instance) {
			changed = append(changed, id)
		}
	}
	for id := range beforeInstances {
		if _, exists := afterInstances[id]; !exists {
			return "", "", 0, 0, false
		}
	}
	slices.Sort(changed)
	slices.SortFunc(addedInstances, func(left, right GraphInstance) int { return cmp.Compare(left.ID, right.ID) })
	if len(addedNodes) == 0 && len(addedInstances) == 0 && len(changed) == 1 {
		id := changed[0]
		prior, current := beforeInstances[id], afterInstances[id]
		if causalSameInstanceExceptValueV19(prior, current) {
			return "set_value", id, 0, 0, true
		}
		if causalSameInstanceExceptPrimitiveV19(prior, current, inventory) {
			return "substitute_primitive", id, 0, 0, true
		}
		if causalPolaritySwapV19(prior, current) {
			return "correct_polarity", id, 0, 0, true
		}
		if causalSingleTerminalRedirectV19(prior, current) {
			primitive, found := primitiveByKey(inventory, prior.PrimitiveKey)
			if found && causalPurePassiveV19(causalTerminalContractsV19(primitive), prior) {
				return "redirect_terminal", id, 0, 0, true
			}
		}
	}
	if len(addedNodes) == 0 && len(addedInstances) == 1 && len(changed) == 0 {
		primitive, found := primitiveByKey(inventory, addedInstances[0].PrimitiveKey)
		if found && causalPurePassiveV19(causalTerminalContractsV19(primitive), addedInstances[0]) {
			return "add_primitive", addedInstances[0].ID, 1, 0, true
		}
	}
	if len(addedNodes) == 1 && addedNodes[0].Scope == "internal" && len(addedInstances) == 1 && len(changed) == 1 {
		primitive, found := primitiveByKey(inventory, addedInstances[0].PrimitiveKey)
		if found && causalPurePassiveV19(causalTerminalContractsV19(primitive), addedInstances[0]) && causalSingleTerminalRedirectV19(beforeInstances[changed[0]], afterInstances[changed[0]]) {
			return "split_primitive", addedInstances[0].ID, 1, 1, true
		}
	}
	return "", "", 0, 0, false
}

func causalExistingOperationEvidenceV19(operation GraphOperation, before, after CandidateGraph, instanceID string) bool {
	beforeHash, beforeErr := GraphHash(before)
	afterHash, afterErr := GraphHash(after)
	if beforeErr != nil || afterErr != nil || operation.BeforeHash != beforeHash || operation.AfterHash != afterHash || instanceID == "" {
		return false
	}
	beforeInstance, beforeFound := causalGraphInstanceV19(before, instanceID)
	afterInstance, afterFound := causalGraphInstanceV19(after, instanceID)
	switch operation.Kind {
	case "set_value":
		return beforeFound && afterFound && operation.Node == instanceID && causalEqualFloatV19(operation.ValueSI, afterInstance.ValueSI)
	case "correct_polarity", "redirect_terminal":
		if !beforeFound || !afterFound || operation.Node != instanceID {
			return false
		}
		changed := causalChangedTerminalConnectionsV19(beforeInstance, afterInstance)
		return causalCompareConnectionMapsV19(causalOrderedConnectionsV19(operation.Connections), changed) == 0
	case "add_primitive", "split_primitive":
		if beforeFound || !afterFound || operation.PrimitiveKey != afterInstance.PrimitiveKey || operation.PrimitiveKind != afterInstance.Kind || !causalEqualFloatV19(operation.ValueSI, afterInstance.ValueSI) {
			return false
		}
		return causalCompareConnectionMapsV19(causalOrderedConnectionsV19(operation.Connections), causalOrderedConnectionsV19(afterInstance.Terminals)) == 0
	case "substitute_primitive":
		return beforeFound && afterFound && operation.Node == instanceID && operation.PrimitiveKey == afterInstance.PrimitiveKey && operation.PrimitiveKind == afterInstance.Kind
	default:
		return false
	}
}

func causalGraphInstanceV19(graph CandidateGraph, instanceID string) (GraphInstance, bool) {
	for _, instance := range graph.Instances {
		if instance.ID == instanceID {
			return instance, true
		}
	}
	return GraphInstance{}, false
}

func causalChangedTerminalConnectionsV19(before, after GraphInstance) []TerminalConnection {
	prior := map[string]string{}
	for _, connection := range before.Terminals {
		prior[connection.Terminal] = connection.Node
	}
	result := []TerminalConnection{}
	for _, connection := range after.Terminals {
		if prior[connection.Terminal] != connection.Node {
			result = append(result, connection)
		}
	}
	return causalOrderedConnectionsV19(result)
}

func causalOrderedConnectionsV19(source []TerminalConnection) []TerminalConnection {
	result := slices.Clone(source)
	slices.SortFunc(result, compareTerminalConnections)
	return result
}

func causalSameInstanceExceptValueV19(left, right GraphInstance) bool {
	if left.ID != right.ID || left.PrimitiveKey != right.PrimitiveKey || left.Kind != right.Kind || !reflect.DeepEqual(left.Terminals, right.Terminals) || left.ValueSI == nil || right.ValueSI == nil || *left.ValueSI == *right.ValueSI {
		return false
	}
	copyLeft, copyRight := left, right
	copyLeft.ValueSI, copyRight.ValueSI = nil, nil
	return reflect.DeepEqual(copyLeft, copyRight)
}

func causalSameInstanceExceptPrimitiveV19(left, right GraphInstance, inventory PrimitiveInventory) bool {
	if left.ID != right.ID || left.PrimitiveKey == right.PrimitiveKey || !causalEqualFloatV19(left.ValueSI, right.ValueSI) || !reflect.DeepEqual(left.Terminals, right.Terminals) {
		return false
	}
	prior, priorOK := primitiveByKey(inventory, left.PrimitiveKey)
	replacement, replacementOK := primitiveByKey(inventory, right.PrimitiveKey)
	return priorOK && replacementOK && samePrimitiveTerminalContract(prior, replacement)
}

func causalSingleTerminalRedirectV19(left, right GraphInstance) bool {
	if left.ID != right.ID || left.PrimitiveKey != right.PrimitiveKey || left.Kind != right.Kind || !causalEqualFloatV19(left.ValueSI, right.ValueSI) || len(left.Terminals) != len(right.Terminals) {
		return false
	}
	leftByTerminal := map[string]string{}
	for _, terminal := range left.Terminals {
		leftByTerminal[terminal.Terminal] = terminal.Node
	}
	changes := 0
	for _, terminal := range right.Terminals {
		prior, exists := leftByTerminal[terminal.Terminal]
		if !exists {
			return false
		}
		if prior != terminal.Node {
			changes++
		}
	}
	return changes == 1
}

func causalPolaritySwapV19(left, right GraphInstance) bool {
	if left.ID != right.ID || left.PrimitiveKey != right.PrimitiveKey || left.Kind != right.Kind || !causalEqualFloatV19(left.ValueSI, right.ValueSI) || len(left.Terminals) != len(right.Terminals) {
		return false
	}
	leftByTerminal := map[string]string{}
	for _, terminal := range left.Terminals {
		leftByTerminal[terminal.Terminal] = terminal.Node
	}
	changed := []TerminalConnection{}
	for _, terminal := range right.Terminals {
		prior, exists := leftByTerminal[terminal.Terminal]
		if !exists {
			return false
		}
		if prior != terminal.Node {
			changed = append(changed, terminal)
		}
	}
	return len(changed) == 2 && leftByTerminal[changed[0].Terminal] == changed[1].Node && leftByTerminal[changed[1].Terminal] == changed[0].Node
}

func causalEqualFloatV19(left, right *float64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func causalAddedInternalNodesV19(before, after CandidateGraph) []string {
	seen := map[string]bool{}
	for _, node := range before.Nodes {
		seen[node.ID] = true
	}
	result := []string{}
	for _, node := range after.Nodes {
		if !seen[node.ID] && node.Scope == "internal" {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return result
}

func causalNewOperationKindV19(kind string) bool {
	return slices.Contains([]string{CausalOperationInsertRoleCompleteStageV19, CausalOperationAllocateObservationConeV19, CausalOperationRedirectRoleTerminalV19, CausalOperationInsertTypedFeedbackPathV19}, kind)
}

func causalExistingOperationKindV19(kind string) bool {
	return slices.Contains([]string{"set_value", "correct_polarity", "redirect_terminal", "add_primitive", "split_primitive", "substitute_primitive"}, kind)
}

func causalCloneContextV19(context CausalInvariantContextV19) CausalInvariantContextV19 {
	return CausalInvariantContextV19{FeedbackPaths: slices.Clone(context.FeedbackPaths)}
}

func causalMergeContextsV19(left, right CausalInvariantContextV19) CausalInvariantContextV19 {
	paths := append(slices.Clone(left.FeedbackPaths), right.FeedbackPaths...)
	slices.SortFunc(paths, func(left, right CausalFeedbackPathV19) int {
		return cmp.Or(cmp.Compare(left.FromInstance, right.FromInstance), cmp.Compare(left.FromTerminal, right.FromTerminal), cmp.Compare(left.ToInstance, right.ToInstance), cmp.Compare(left.ToTerminal, right.ToTerminal), cmp.Compare(left.ObligationID, right.ObligationID))
	})
	paths = slices.Compact(paths)
	return CausalInvariantContextV19{FeedbackPaths: paths}
}

func causalCloneOperationsV19(source []CausalLogicalOperationV19) []CausalLogicalOperationV19 {
	result := make([]CausalLogicalOperationV19, len(source))
	for index, operation := range source {
		result[index] = operation
		result[index].Connections = slices.Clone(operation.Connections)
		result[index].CreatedNodes = slices.Clone(operation.CreatedNodes)
		result[index].ValueSI = cloneInventoryFloat(operation.ValueSI)
		if operation.FeedbackPath != nil {
			feedback := *operation.FeedbackPath
			result[index].FeedbackPath = &feedback
		}
	}
	return result
}

func causalProposalEvidenceValidV19(proposal CausalOperationProposalV19) bool {
	if proposal.LogicalChanges != len(proposal.Operations) || proposal.LogicalChanges == 0 || proposal.LogicalChanges > CausalMaximumLogicalChangesPerProposalV19 || proposal.CanonicalKey != causalProposalKeyV19(proposal) {
		return false
	}
	graphHash, err := GraphHash(proposal.Graph)
	if err != nil || proposal.Operations[len(proposal.Operations)-1].AfterHash != graphHash {
		return false
	}
	for index, operation := range proposal.Operations {
		if operation.Number != index+1 || operation.BeforeHash == "" || operation.AfterHash == "" || operation.ObligationID == "" || operation.ObservationID == "" || operation.CanonicalCost <= 0 || operation.InstanceID == "" {
			return false
		}
		if index > 0 && proposal.Operations[index-1].AfterHash != operation.BeforeHash {
			return false
		}
		if operation.FeedbackPath != nil && !slices.Contains(proposal.Context.FeedbackPaths, *operation.FeedbackPath) {
			return false
		}
	}
	return true
}

func causalCompareStagePlansV19(left, right causalStagePlanV19) int {
	return cmp.Or(
		cmp.Compare(causalBoolRankV19(!left.critical), causalBoolRankV19(!right.critical)),
		cmp.Compare(left.request.ObservationID, right.request.ObservationID),
		cmp.Compare(left.request.UpstreamNode, right.request.UpstreamNode),
		cmp.Compare(left.primitive.Kind, right.primitive.Kind),
		cmp.Compare(left.primitive.Key, right.primitive.Key),
		causalCompareConnectionMapsV19(left.connections, right.connections),
		causalCompareStageValuesV19(left.value, right.value),
	)
}

func causalCompareStageValuesV19(left, right *float64) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	default:
		return cmp.Compare(*left, *right)
	}
}

func causalCompareConnectionMapsV19(left, right []TerminalConnection) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if order := compareTerminalConnections(left[index], right[index]); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(left), len(right))
}

func causalSortProposalsV19(proposals []CausalOperationProposalV19) []CausalOperationProposalV19 {
	slices.SortFunc(proposals, func(left, right CausalOperationProposalV19) int {
		return cmp.Compare(left.CanonicalKey, right.CanonicalKey)
	})
	if len(proposals) == 0 {
		return proposals
	}
	seen := map[string]bool{}
	write := 0
	for read := range proposals {
		hash, err := GraphHash(proposals[read].Graph)
		if err != nil {
			continue
		}
		if seen[hash] {
			continue
		}
		seen[hash] = true
		proposals[write] = proposals[read]
		write++
	}
	clear(proposals[write:])
	return proposals[:write]
}

func causalProposalKeyV19(proposal CausalOperationProposalV19) string {
	fields := []string{proposal.PlannerKind, strconv.Itoa(proposal.LogicalChanges)}
	for _, operation := range proposal.Operations {
		feedbackKey := "-"
		if operation.FeedbackPath != nil {
			feedbackKey = causalLengthDelimitedV19([]string{operation.FeedbackPath.FromInstance, operation.FeedbackPath.FromTerminal, operation.FeedbackPath.ToInstance, operation.FeedbackPath.ToTerminal, operation.FeedbackPath.ObligationID})
		}
		fields = append(fields, operation.Kind, operation.ObligationID, operation.ObservationID, operation.UpstreamNode, operation.PrimitiveKind, operation.PrimitiveKey, operation.InstanceID,
			causalLengthDelimitedV19(operation.CreatedNodes), causalConnectionsKeyV19(operation.Connections), causalCanonicalValueV19(operation.ValueSI), feedbackKey, operation.BeforeHash, operation.AfterHash)
	}
	return causalLengthDelimitedV19(fields)
}

func causalConnectionsKeyV19(connections []TerminalConnection) string {
	ordered := slices.Clone(connections)
	slices.SortFunc(ordered, compareTerminalConnections)
	fields := []string{}
	for _, connection := range ordered {
		fields = append(fields, connection.Terminal, connection.Node)
	}
	return causalLengthDelimitedV19(fields)
}

func causalLengthDelimitedV19(fields []string) string {
	var builder strings.Builder
	for _, field := range fields {
		builder.WriteString(strconv.Itoa(len(field)))
		builder.WriteByte(':')
		builder.WriteString(field)
	}
	return builder.String()
}

func causalCanonicalValueV19(value *float64) string {
	if value == nil {
		return "-"
	}
	if !finite(*value) || math.Signbit(*value) && *value == 0 {
		return "!"
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}

func causalBoolRankV19(value bool) int {
	if value {
		return 1
	}
	return 0
}
