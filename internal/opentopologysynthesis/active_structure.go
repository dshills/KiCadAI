package opentopologysynthesis

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// ActiveStructureHash identifies a materially distinct architecture while
// ignoring catalog/value substitutions and cosmetic decomposition of a
// resistive connection. Capacitors, inductors, diodes, and active devices are
// retained because they can change dynamics, energy transfer, or control
// polarity. The hash supplements TopologyHash; it never replaces the complete
// graph evidence used for simulation and physical lowering.
func ActiveStructureHash(graph CandidateGraph) (string, error) {
	if graph.Schema != CandidateGraphSchema || graph.Version != CandidateGraphVersion {
		return "", errors.New("candidate graph schema/version is invalid")
	}
	nodes := slices.Clone(graph.Nodes)
	slices.SortFunc(nodes, func(left, right GraphNode) int {
		return cmp.Compare(left.ID, right.ID)
	})
	nodeIndex := make(map[string]int, len(nodes))
	parents := make([]int, len(nodes))
	for index, node := range nodes {
		if node.ID == "" {
			return "", errors.New("candidate graph node IDs must be nonempty")
		}
		if _, duplicate := nodeIndex[node.ID]; duplicate {
			return "", errors.New("candidate graph node IDs must be unique")
		}
		nodeIndex[node.ID] = index
		parents[index] = index
	}
	find := func(index int) int {
		root := index
		for parents[root] != root {
			root = parents[root]
		}
		for parents[index] != root {
			next := parents[index]
			parents[index] = root
			index = next
		}
		return root
	}
	union := func(left, right int) {
		left, right = find(left), find(right)
		if left == right {
			return
		}
		if left > right {
			left, right = right, left
		}
		parents[right] = left
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" {
			continue
		}
		// CandidateGraph resistors are two-terminal primitives. Ignoring a
		// malformed passive here could make an invalid graph hash-identical to a
		// valid architecture, so active-structure evidence fails closed.
		if len(instance.Terminals) != 2 {
			return "", fmt.Errorf("resistor %q must have exactly two terminals for active-structure contraction", instance.ID)
		}
		left, leftFound := nodeIndex[instance.Terminals[0].Node]
		right, rightFound := nodeIndex[instance.Terminals[1].Node]
		if !leftFound || !rightFound {
			return "", fmt.Errorf("resistor %q refers to unknown nodes %q or %q", instance.ID, instance.Terminals[0].Node, instance.Terminals[1].Node)
		}
		union(left, right)
	}

	groups := map[int][]GraphNode{}
	for index, node := range nodes {
		root := find(index)
		groups[root] = append(groups[root], node)
	}
	roots := make([]int, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	slices.Sort(roots)
	reduced := CandidateGraph{Schema: CandidateGraphSchema, Version: CandidateGraphVersion}
	reducedNodeByOriginal := make(map[string]string, len(nodes))
	for groupIndex, root := range roots {
		members := groups[root]
		node := activeStructureNode(groupIndex, members)
		reduced.Nodes = append(reduced.Nodes, node)
		for _, member := range members {
			reducedNodeByOriginal[member.ID] = node.ID
		}
	}
	for _, source := range graph.Instances {
		if source.Kind == "resistor" {
			continue
		}
		// The reduced graph is consumed only by TopologyHash. Using Kind as its
		// synthetic primitive key intentionally erases catalog identity while
		// retaining the active device class that defines the architecture.
		instance := GraphInstance{ID: source.ID, Kind: source.Kind, PrimitiveKey: source.Kind}
		for _, terminal := range source.Terminals {
			node, found := reducedNodeByOriginal[terminal.Node]
			if !found {
				return "", fmt.Errorf("instance %q terminal %q refers to unknown node %q", source.ID, terminal.Terminal, terminal.Node)
			}
			instance.Terminals = append(instance.Terminals, TerminalConnection{Terminal: terminal.Terminal, Node: node})
		}
		reduced.Instances = append(reduced.Instances, instance)
	}
	return TopologyHash(reduced)
}

func activeStructureNode(index int, members []GraphNode) GraphNode {
	external := []string{}
	domains := []string{}
	roles := []string{}
	for _, member := range members {
		if member.Scope == "external" {
			external = append(external, activeStructureFields(
				member.SemanticKind, member.SemanticID, member.Domain, member.Role,
			))
		}
		if member.Domain != "" {
			domains = append(domains, member.Domain)
		}
		if member.Role != "" {
			roles = append(roles, member.Role)
		}
	}
	slices.Sort(external)
	slices.Sort(domains)
	slices.Sort(roles)
	domains = slices.Compact(domains)
	roles = slices.Compact(roles)
	node := GraphNode{
		ID:     "structure_node_" + strconv.Itoa(index),
		Scope:  "internal",
		Domain: activeStructureFields(domains...),
		Role:   activeStructureFields(roles...),
	}
	if node.Role == "" {
		node.Role = "internal"
	}
	if len(external) != 0 {
		node.Scope = "external"
		node.SemanticKind = "contracted"
		node.SemanticID = activeStructureFields(external...)
	}
	return node
}

// activeStructureFields uses length-prefixed fields so arbitrary semantic IDs,
// domains, and roles cannot collide through delimiter characters.
func activeStructureFields(fields ...string) string {
	capacity := 0
	for _, field := range fields {
		capacity += len(field) + 20
	}
	encoded := make([]byte, 0, capacity)
	for _, field := range fields {
		encoded = strconv.AppendInt(encoded, int64(len(field)), 10)
		encoded = append(encoded, ':')
		encoded = append(encoded, field...)
	}
	return string(encoded)
}
