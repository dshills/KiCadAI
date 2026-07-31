package opentopologysynthesis

import (
	"errors"
	"fmt"
	"slices"
)

var (
	ErrGraphInstanceNotFound       = errors.New("graph instance not found")
	ErrGraphNodeNotFound           = errors.New("graph node not found")
	ErrGraphPrimitiveNotFound      = errors.New("graph primitive not found")
	ErrGraphTerminalNotFound       = errors.New("graph terminal not found")
	ErrGraphTerminalConnected      = errors.New("graph terminal already connected")
	ErrGraphIncompatibleConnection = errors.New("graph terminal and node roles are incompatible")
	ErrGraphIncompatiblePrimitive  = errors.New("graph primitives are not substitution-compatible")
	ErrGraphNodeNotAnonymous       = errors.New("graph node is not anonymous internal")
	ErrGraphNodeInUse              = errors.New("graph node is in use")
	ErrGraphPrimitiveRelevant      = errors.New("graph primitive is in a source-to-observation cone")
	ErrGraphBridgePrimitive        = errors.New("graph bridge requires exactly two primitive terminals")
)

// ConnectPrimitiveTerminal adds one terminal-level connection. It intentionally
// accepts an otherwise incomplete graph so search can build a candidate through
// generic terminal operations; complete validation remains the simulation gate.
func ConnectPrimitiveTerminal(
	graph CandidateGraph,
	inventory PrimitiveInventory,
	instanceID string,
	terminal string,
	nodeID string,
) (CandidateGraph, error) {
	result := CloneGraph(graph)
	node, found := graphNodeByID(result, nodeID)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, nodeID)
	}
	instanceIndex := graphInstanceIndex(result, instanceID)
	if instanceIndex < 0 {
		return graph, fmt.Errorf("%w: %s", ErrGraphInstanceNotFound, instanceID)
	}
	primitive, found := primitiveByKey(inventory, result.Instances[instanceIndex].PrimitiveKey)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphPrimitiveNotFound, result.Instances[instanceIndex].PrimitiveKey)
	}
	contract, found := primitiveTerminalByName(primitive, terminal)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphTerminalNotFound, terminal)
	}
	for _, connection := range result.Instances[instanceIndex].Terminals {
		if connection.Terminal == contract.Terminal {
			return graph, fmt.Errorf("%w: %s.%s", ErrGraphTerminalConnected, instanceID, terminal)
		}
	}
	if !terminalRoleCompatible(primitive.Kind, contract.Terminal, node.Role) {
		return graph, fmt.Errorf(
			"%w: %s.%s -> %s(%s)",
			ErrGraphIncompatibleConnection,
			instanceID,
			terminal,
			nodeID,
			node.Role,
		)
	}
	result.Instances[instanceIndex].Terminals = append(
		result.Instances[instanceIndex].Terminals,
		TerminalConnection{Terminal: contract.Terminal, Node: nodeID},
	)
	slices.SortFunc(result.Instances[instanceIndex].Terminals, compareTerminalConnections)
	return result, nil
}

// RedirectPrimitiveTerminal moves one existing terminal attachment to another
// compatible node. It is the graph-repair counterpart to terminal connection
// and preserves the primitive instance, value, and all other attachments.
func RedirectPrimitiveTerminal(
	graph CandidateGraph,
	inventory PrimitiveInventory,
	instanceID string,
	terminal string,
	nodeID string,
) (CandidateGraph, error) {
	result := CloneGraph(graph)
	node, found := graphNodeByID(result, nodeID)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, nodeID)
	}
	instanceIndex := graphInstanceIndex(result, instanceID)
	if instanceIndex < 0 {
		return graph, fmt.Errorf("%w: %s", ErrGraphInstanceNotFound, instanceID)
	}
	primitive, found := primitiveByKey(inventory, result.Instances[instanceIndex].PrimitiveKey)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphPrimitiveNotFound, result.Instances[instanceIndex].PrimitiveKey)
	}
	contract, found := primitiveTerminalByName(primitive, terminal)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphTerminalNotFound, terminal)
	}
	if !terminalRoleCompatible(primitive.Kind, contract.Terminal, node.Role) {
		return graph, fmt.Errorf("%w: %s.%s -> %s(%s)", ErrGraphIncompatibleConnection, instanceID, terminal, nodeID, node.Role)
	}
	terminalIndex := -1
	for index, connection := range result.Instances[instanceIndex].Terminals {
		if connection.Terminal == contract.Terminal {
			terminalIndex = index
			continue
		}
		if connection.Node == nodeID {
			return graph, fmt.Errorf("%w: redirect would attach multiple terminals to %s", ErrGraphIncompatibleConnection, nodeID)
		}
	}
	if terminalIndex < 0 {
		return graph, fmt.Errorf("%w: %s.%s", ErrGraphTerminalNotFound, instanceID, terminal)
	}
	result.Instances[instanceIndex].Terminals[terminalIndex].Node = nodeID
	slices.SortFunc(result.Instances[instanceIndex].Terminals, compareTerminalConnections)
	return result, nil
}

// BridgeNodesWithPrimitive creates one generic two-terminal graph edge. The
// primitive terminal order defines polarity for directed devices; canonical
// hashing removes orientation for symmetric passives.
func BridgeNodesWithPrimitive(
	graph CandidateGraph,
	primitive PrimitiveCandidate,
	value *float64,
	fromNode string,
	toNode string,
) (CandidateGraph, error) {
	if len(primitive.Terminals) != 2 {
		return graph, fmt.Errorf("%w: %s has %d terminals", ErrGraphBridgePrimitive, primitive.Key, len(primitive.Terminals))
	}
	from, fromFound := graphNodeByID(graph, fromNode)
	to, toFound := graphNodeByID(graph, toNode)
	if !fromFound {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, fromNode)
	}
	if !toFound {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, toNode)
	}
	if fromNode == toNode ||
		!terminalRoleCompatible(primitive.Kind, primitive.Terminals[0].Terminal, from.Role) ||
		!terminalRoleCompatible(primitive.Kind, primitive.Terminals[1].Terminal, to.Role) {
		return graph, fmt.Errorf("%w: %s -> %s", ErrGraphIncompatibleConnection, fromNode, toNode)
	}
	connections := []TerminalConnection{
		{Terminal: primitive.Terminals[0].Terminal, Node: fromNode},
		{Terminal: primitive.Terminals[1].Terminal, Node: toNode},
	}
	return AddPrimitive(graph, primitive, value, connections), nil
}

// JoinAnonymousNodes electrically merges two anonymous internal nodes.
func JoinAnonymousNodes(graph CandidateGraph, keepNode string, removeNode string) (CandidateGraph, error) {
	if keepNode == removeNode {
		return graph, fmt.Errorf("%w: identical node %s", ErrGraphNodeNotAnonymous, keepNode)
	}
	keep, keepFound := graphNodeByID(graph, keepNode)
	remove, removeFound := graphNodeByID(graph, removeNode)
	if !keepFound {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, keepNode)
	}
	if !removeFound {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, removeNode)
	}
	if keep.Scope != "internal" || remove.Scope != "internal" ||
		keep.SemanticKind != "" || remove.SemanticKind != "" ||
		keep.SemanticID != "" || remove.SemanticID != "" {
		return graph, fmt.Errorf("%w: %s,%s", ErrGraphNodeNotAnonymous, keepNode, removeNode)
	}
	result := CloneGraph(graph)
	for instanceIndex := range result.Instances {
		for terminalIndex := range result.Instances[instanceIndex].Terminals {
			if result.Instances[instanceIndex].Terminals[terminalIndex].Node == removeNode {
				result.Instances[instanceIndex].Terminals[terminalIndex].Node = keepNode
			}
		}
		slices.SortFunc(result.Instances[instanceIndex].Terminals, compareTerminalConnections)
	}
	for index, node := range result.Nodes {
		if node.ID == removeNode {
			result.Nodes = slices.Delete(result.Nodes, index, index+1)
			break
		}
	}
	return result, nil
}

// SubstitutePrimitive replaces an instance with a catalog alternative having
// the same primitive kind and terminal contract.
func SubstitutePrimitive(
	graph CandidateGraph,
	inventory PrimitiveInventory,
	instanceID string,
	replacementKey string,
) (CandidateGraph, error) {
	result := CloneGraph(graph)
	instanceIndex := graphInstanceIndex(result, instanceID)
	if instanceIndex < 0 {
		return graph, fmt.Errorf("%w: %s", ErrGraphInstanceNotFound, instanceID)
	}
	current, found := primitiveByKey(inventory, result.Instances[instanceIndex].PrimitiveKey)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphPrimitiveNotFound, result.Instances[instanceIndex].PrimitiveKey)
	}
	replacement, found := primitiveByKey(inventory, replacementKey)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphPrimitiveNotFound, replacementKey)
	}
	if current.Kind != replacement.Kind || !samePrimitiveTerminalContract(current, replacement) {
		return graph, fmt.Errorf("%w: %s -> %s", ErrGraphIncompatiblePrimitive, current.Key, replacement.Key)
	}
	instance := &result.Instances[instanceIndex]
	instance.PrimitiveKey = replacement.Key
	instance.Kind = replacement.Kind
	switch {
	case replacement.ValueDomain == nil:
		instance.ValueSI = nil
	case instance.ValueSI == nil || !valueWithinPrimitiveDomain(*instance.ValueSI, *replacement.ValueDomain):
		instance.ValueSI = seedPrimitiveValue(replacement)
	}
	return result, nil
}

func RemovePrimitive(graph CandidateGraph, instanceID string) (CandidateGraph, error) {
	index := graphInstanceIndex(graph, instanceID)
	if index < 0 {
		return graph, fmt.Errorf("%w: %s", ErrGraphInstanceNotFound, instanceID)
	}
	result := CloneGraph(graph)
	result.Instances = slices.Delete(result.Instances, index, index+1)
	return result, nil
}

// RemoveIrrelevantPrimitive is the guarded remove operation used by repair. It
// refuses to remove any instance that participates in a path from an external
// excitation, supply, or reference to an external observation.
func RemoveIrrelevantPrimitive(graph CandidateGraph, instanceID string) (CandidateGraph, error) {
	if graphInstanceIndex(graph, instanceID) < 0 {
		return graph, fmt.Errorf("%w: %s", ErrGraphInstanceNotFound, instanceID)
	}
	if primitiveInSourceObservationCone(graph, instanceID) {
		return graph, fmt.Errorf("%w: %s", ErrGraphPrimitiveRelevant, instanceID)
	}
	return RemovePrimitive(graph, instanceID)
}

func RemoveUnusedInternalNode(graph CandidateGraph, nodeID string) (CandidateGraph, error) {
	node, found := graphNodeByID(graph, nodeID)
	if !found {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, nodeID)
	}
	if node.Scope != "internal" || node.SemanticKind != "" || node.SemanticID != "" {
		return graph, fmt.Errorf("%w: %s", ErrGraphNodeNotAnonymous, nodeID)
	}
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			if terminal.Node == nodeID {
				return graph, fmt.Errorf("%w: %s", ErrGraphNodeInUse, nodeID)
			}
		}
	}
	result := CloneGraph(graph)
	for index, candidate := range result.Nodes {
		if candidate.ID == nodeID {
			result.Nodes = slices.Delete(result.Nodes, index, index+1)
			break
		}
	}
	return result, nil
}

func graphNodeByID(graph CandidateGraph, nodeID string) (GraphNode, bool) {
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return GraphNode{}, false
}

func graphInstanceIndex(graph CandidateGraph, instanceID string) int {
	for index, instance := range graph.Instances {
		if instance.ID == instanceID {
			return index
		}
	}
	return -1
}

func primitiveByKey(inventory PrimitiveInventory, key string) (PrimitiveCandidate, bool) {
	for _, primitive := range inventory.Primitives {
		if primitive.Key == key {
			return primitive, true
		}
	}
	return PrimitiveCandidate{}, false
}

func primitiveTerminalByName(primitive PrimitiveCandidate, terminal string) (PrimitiveTerminal, bool) {
	for _, candidate := range primitive.Terminals {
		if candidate.Terminal == terminal {
			return candidate, true
		}
	}
	return PrimitiveTerminal{}, false
}

func samePrimitiveTerminalContract(left PrimitiveCandidate, right PrimitiveCandidate) bool {
	leftNames := make([]string, 0, len(left.Terminals))
	rightNames := make([]string, 0, len(right.Terminals))
	for _, terminal := range left.Terminals {
		leftNames = append(leftNames, terminal.Terminal)
	}
	for _, terminal := range right.Terminals {
		rightNames = append(rightNames, terminal.Terminal)
	}
	slices.Sort(leftNames)
	slices.Sort(rightNames)
	return slices.Equal(leftNames, rightNames)
}

func primitiveInSourceObservationCone(graph CandidateGraph, instanceID string) bool {
	adjacency := map[string][]string{}
	for _, instance := range graph.Instances {
		instanceVertex := "instance:" + instance.ID
		for _, terminal := range instance.Terminals {
			nodeVertex := "node:" + terminal.Node
			adjacency[instanceVertex] = append(adjacency[instanceVertex], nodeVertex)
			adjacency[nodeVertex] = append(adjacency[nodeVertex], instanceVertex)
		}
	}
	target := "instance:" + instanceID
	neighbors := slices.Clone(adjacency[target])
	delete(adjacency, target)
	for vertex, connected := range adjacency {
		adjacency[vertex] = slices.DeleteFunc(connected, func(candidate string) bool {
			return candidate == target
		})
	}
	sourceReachable := map[string]bool{}
	observationReachable := map[string]bool{}
	for _, node := range graph.Nodes {
		vertex := "node:" + node.ID
		if node.Scope != "external" {
			continue
		}
		if node.Role == "input" || node.Role == "control" || node.Role == "supply" || node.Role == "reference" {
			markReachableVertices(adjacency, vertex, sourceReachable)
		}
		if node.Role == "output" {
			markReachableVertices(adjacency, vertex, observationReachable)
		}
	}
	for _, from := range neighbors {
		if !sourceReachable[from] {
			continue
		}
		for _, to := range neighbors {
			if from != to && observationReachable[to] {
				return true
			}
		}
	}
	return false
}

func markReachableVertices(adjacency map[string][]string, start string, reached map[string]bool) {
	if reached[start] {
		return
	}
	reached[start] = true
	queue := []string{start}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if reached[next] {
				continue
			}
			reached[next] = true
			queue = append(queue, next)
		}
	}
}
