package opentopologysynthesis

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

func ValidateCompleteGraph(graph CandidateGraph, inventory PrimitiveInventory, limits GraphLimits) []reports.Issue {
	return validateGraph(graph, inventory, limits, true)
}

func ValidatePartialGraph(graph CandidateGraph, inventory PrimitiveInventory, limits GraphLimits) []reports.Issue {
	return validateGraph(graph, inventory, limits, false)
}

func validateGraph(graph CandidateGraph, inventory PrimitiveInventory, limits GraphLimits, complete bool) []reports.Issue {
	validator := graphValidator{
		graph:       graph,
		inventory:   map[string]PrimitiveCandidate{},
		nodes:       map[string]GraphNode{},
		instances:   map[string]GraphInstance{},
		nodeDegrees: map[string]int{},
		complete:    complete,
		limits:      limits,
	}
	for _, primitive := range inventory.Primitives {
		validator.inventory[primitive.Key] = primitive
	}
	validator.header()
	validator.validateNodes()
	validator.validateInstances()
	if complete && len(validator.issues) == 0 {
		validator.validateReachability()
		validator.validateActiveSupplies()
		validator.validateRelevantInstances()
	}
	return reports.SortedIssues(validator.issues)
}

type graphValidator struct {
	graph       CandidateGraph
	inventory   map[string]PrimitiveCandidate
	nodes       map[string]GraphNode
	instances   map[string]GraphInstance
	nodeDegrees map[string]int
	issues      []reports.Issue
	complete    bool
	limits      GraphLimits
}

func (validator *graphValidator) add(path, message, suggestion string) {
	validator.issues = append(validator.issues, graphIssue(CodeNoCompleteGraph, path, message, suggestion))
}

func (validator *graphValidator) header() {
	if validator.graph.Schema != CandidateGraphSchema || validator.graph.Version != CandidateGraphVersion {
		validator.add("schema", fmt.Sprintf("schema/version must be %q/%d", CandidateGraphSchema, CandidateGraphVersion), "rebuild the candidate through the trusted graph kernel")
	}
	if validator.limits.MaxPrimitiveInstances <= 0 {
		validator.limits.MaxPrimitiveInstances = DefaultPolicy().MaxPrimitiveInstances
	}
	if validator.limits.MaxInternalNodes <= 0 {
		validator.limits.MaxInternalNodes = DefaultPolicy().MaxInternalNodes
	}
	if len(validator.graph.Instances) > validator.limits.MaxPrimitiveInstances {
		validator.add("instances", fmt.Sprintf("primitive count %d exceeds limit %d", len(validator.graph.Instances), validator.limits.MaxPrimitiveInstances), "reduce the bounded graph")
	}
}

func (validator *graphValidator) validateNodes() {
	external := 0
	internal := 0
	for index, node := range validator.graph.Nodes {
		path := fmt.Sprintf("nodes[%d]", index)
		if !validGraphID(node.ID) {
			validator.add(path+".id", "node ID must be a normalized semantic identifier", "")
		}
		if _, duplicate := validator.nodes[node.ID]; duplicate {
			validator.add(path+".id", "node ID must be unique", "")
		}
		validator.nodes[node.ID] = node
		switch node.Scope {
		case "external":
			external++
			if node.SemanticKind != "port" || !validGraphID(node.SemanticID) || !validGraphID(node.Domain) {
				validator.add(path, "external node requires a semantic port identity and domain", "")
			}
		case "internal":
			internal++
			if node.SemanticKind != "" || node.SemanticID != "" || node.Domain != "" {
				validator.add(path, "internal nodes may not claim external semantic identity", "")
			}
		default:
			validator.add(path+".scope", "node scope must be external or internal", "")
		}
		if !slices.Contains([]string{"control", "input", "internal", "output", "reference", "supply"}, node.Role) {
			validator.add(path+".role", "unsupported graph-node role", "")
		}
	}
	if external < 2 {
		validator.add("nodes", "candidate graph requires at least two external nodes", "")
	}
	if internal > validator.limits.MaxInternalNodes {
		validator.add("nodes", fmt.Sprintf("internal-node count %d exceeds limit %d", internal, validator.limits.MaxInternalNodes), "reduce the bounded graph")
	}
}

func (validator *graphValidator) validateInstances() {
	for index, instance := range validator.graph.Instances {
		path := fmt.Sprintf("instances[%d]", index)
		if !validGraphID(instance.ID) {
			validator.add(path+".id", "instance ID must be a normalized semantic identifier", "")
		}
		if _, duplicate := validator.instances[instance.ID]; duplicate {
			validator.add(path+".id", "instance ID must be unique", "")
		}
		validator.instances[instance.ID] = instance
		primitive, exists := validator.inventory[instance.PrimitiveKey]
		if !exists {
			validator.add(path+".primitive_key", "instance primitive is absent from the bound inventory", "rebuild search against the current primitive inventory")
			continue
		}
		if instance.Kind != primitive.Kind {
			validator.add(path+".kind", "instance kind does not match the bound primitive", "")
		}
		if primitive.ValueDomain != nil {
			if instance.ValueSI == nil || !finite(*instance.ValueSI) || *instance.ValueSI <= 0 {
				validator.add(path+".value_si", "value-bearing primitive requires a finite positive value", "")
			} else if !valueWithinPrimitiveDomain(*instance.ValueSI, *primitive.ValueDomain) {
				validator.add(path+".value_si", "primitive value is outside its catalog domain", "select a value from the frozen primitive domain")
			}
		} else if instance.ValueSI != nil {
			validator.add(path+".value_si", "fixed primitive may not carry a free component value", "")
		}
		expectedTerminals := map[string]PrimitiveTerminal{}
		for _, terminal := range primitive.Terminals {
			expectedTerminals[terminal.Terminal] = terminal
		}
		seenTerminals := map[string]bool{}
		distinctNodes := map[string]bool{}
		for terminalIndex, connection := range instance.Terminals {
			connectionPath := fmt.Sprintf("%s.terminals[%d]", path, terminalIndex)
			if _, exists := expectedTerminals[connection.Terminal]; !exists {
				validator.add(connectionPath+".terminal", "terminal is not declared by the bound primitive", "")
			}
			if seenTerminals[connection.Terminal] {
				validator.add(connectionPath+".terminal", "primitive terminal must be connected exactly once", "")
			}
			seenTerminals[connection.Terminal] = true
			if _, exists := validator.nodes[connection.Node]; !exists {
				validator.add(connectionPath+".node", "terminal refers to an unknown graph node", "")
			} else {
				validator.nodeDegrees[connection.Node]++
				distinctNodes[connection.Node] = true
			}
		}
		for terminal, contract := range expectedTerminals {
			if validator.complete && !seenTerminals[terminal] && contract.DefaultNet == "" {
				validator.add(path+".terminals", "required primitive terminal "+terminal+" is unconnected", "")
			}
		}
		if validator.complete && len(distinctNodes) < 2 {
			validator.add(path+".terminals", "primitive must span at least two distinct nodes", "")
		}
	}
	if validator.complete {
		for _, node := range validator.graph.Nodes {
			if node.Scope == "internal" && validator.nodeDegrees[node.ID] < 2 {
				validator.add("nodes."+node.ID, "complete graph contains a dangling internal node", "remove the node or complete its electrical path")
			}
			if node.Scope == "external" && validator.nodeDegrees[node.ID] == 0 {
				validator.add("nodes."+node.ID, "required external node is disconnected", "connect every required external interface")
			}
		}
	}
}

func (validator *graphValidator) validateReachability() {
	adjacency := validator.adjacency()
	starts := []string{}
	outputs := []string{}
	for _, node := range validator.graph.Nodes {
		switch node.Role {
		case "input", "control", "supply":
			starts = append(starts, "node:"+node.ID)
		case "output":
			outputs = append(outputs, "node:"+node.ID)
		}
	}
	for _, output := range outputs {
		reachable := false
		for _, start := range starts {
			if graphPathExists(adjacency, start, output) {
				reachable = true
				break
			}
		}
		if !reachable {
			validator.add("nodes."+strings.TrimPrefix(output, "node:"), "output is unreachable from every excitation or supply", "add a source-to-observation path")
		}
	}
}

func (validator *graphValidator) validateActiveSupplies() {
	for _, instance := range validator.graph.Instances {
		if instance.Kind != "opamp" && instance.Kind != "comparator" {
			continue
		}
		connections := map[string]GraphNode{}
		for _, terminal := range instance.Terminals {
			if node, exists := validator.nodes[terminal.Node]; exists {
				connections[terminal.Terminal] = node
			}
		}
		positive, positiveOK := connections["V_PLUS"]
		negative, negativeOK := connections["V_MINUS"]
		if !positiveOK || positive.Role != "supply" {
			validator.add("instances."+instance.ID, "active primitive positive supply is not bound to an external supply", "connect V_PLUS to a declared supply")
		}
		if !negativeOK || (negative.Role != "reference" && negative.Role != "supply") {
			validator.add("instances."+instance.ID, "active primitive negative supply is not bound to a reference or supply", "connect V_MINUS to a declared reference or negative supply")
		}
		if positiveOK && negativeOK && positive.ID == negative.ID {
			validator.add("instances."+instance.ID, "active primitive supply terminals are shorted", "")
		}
	}
}

func (validator *graphValidator) validateRelevantInstances() {
	adjacency := validator.adjacency()
	starts := []string{}
	outputs := []string{}
	for _, node := range validator.graph.Nodes {
		switch node.Role {
		case "input", "control", "supply", "reference":
			starts = append(starts, "node:"+node.ID)
		case "output":
			outputs = append(outputs, "node:"+node.ID)
		}
	}
	for _, instance := range validator.graph.Instances {
		vertex := "instance:" + instance.ID
		fromSource := false
		toOutput := false
		for _, start := range starts {
			if graphPathExists(adjacency, start, vertex) {
				fromSource = true
				break
			}
		}
		for _, output := range outputs {
			if graphPathExists(adjacency, vertex, output) {
				toOutput = true
				break
			}
		}
		if !fromSource || !toOutput {
			validator.add("instances."+instance.ID, "primitive is outside every source-to-observation cone", "remove the irrelevant primitive")
		}
	}
}

func (validator *graphValidator) adjacency() map[string][]string {
	result := map[string][]string{}
	for _, instance := range validator.graph.Instances {
		instanceVertex := "instance:" + instance.ID
		for _, terminal := range instance.Terminals {
			nodeVertex := "node:" + terminal.Node
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

func graphPathExists(adjacency map[string][]string, start, target string) bool {
	if start == target {
		return true
	}
	visited := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if next == target {
				return true
			}
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

func valueWithinPrimitiveDomain(value float64, domain PrimitiveValueDomain) bool {
	if domain.Minimum == nil && domain.Maximum == nil && domain.Nominal != nil {
		scale := math.Max(1, math.Abs(*domain.Nominal))
		return math.Abs(value-*domain.Nominal) <= scale*1e-12
	}
	if domain.Minimum != nil && value < *domain.Minimum {
		return false
	}
	if domain.Maximum != nil && value > *domain.Maximum {
		return false
	}
	return true
}
