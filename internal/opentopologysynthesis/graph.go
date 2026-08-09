package opentopologysynthesis

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const (
	CandidateGraphSchema  = "kicadai.open-topology-candidate-graph.v1"
	CandidateGraphVersion = 1
)

type CandidateGraph struct {
	Schema    string          `json:"schema"`
	Version   int             `json:"version"`
	Nodes     []GraphNode     `json:"nodes"`
	Instances []GraphInstance `json:"instances"`
}

type GraphNode struct {
	ID           string `json:"id"`
	Scope        string `json:"scope"`
	SemanticKind string `json:"semantic_kind,omitempty"`
	SemanticID   string `json:"semantic_id,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Role         string `json:"role"`
}

type GraphInstance struct {
	ID           string               `json:"id"`
	PrimitiveKey string               `json:"primitive_key"`
	Kind         string               `json:"kind"`
	ValueSI      *float64             `json:"value_si,omitempty"`
	Terminals    []TerminalConnection `json:"terminals"`
}

type TerminalConnection struct {
	Terminal string `json:"terminal"`
	Node     string `json:"node"`
}

type GraphLimits struct {
	MaxPrimitiveInstances int
	MaxInternalNodes      int
}

func InitialGraph(requirement Requirement) (CandidateGraph, []reports.Issue) {
	if issues := Validate(requirement); len(issues) != 0 {
		return CandidateGraph{}, issues
	}
	graph := CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes:   make([]GraphNode, 0, len(requirement.Requirements.Ports)+len(requirement.Requirements.Domains)),
	}
	representedReferenceDomains := map[string]bool{}
	for _, port := range requirement.Requirements.Ports {
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:           "port_" + port.ID,
			Scope:        "external",
			SemanticKind: "port",
			SemanticID:   port.ID,
			Domain:       port.Domain,
			Role:         graphRoleForPort(port),
		})
		if port.Kind == "reference" {
			representedReferenceDomains[port.Domain] = true
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "reference" || representedReferenceDomains[domain.ID] {
			continue
		}
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:           "domain_" + domain.ID,
			Scope:        "external",
			SemanticKind: "domain",
			SemanticID:   domain.ID,
			Domain:       domain.ID,
			Role:         "reference",
		})
	}
	slices.SortFunc(graph.Nodes, compareGraphNodes)
	return graph, nil
}

func ExternalNodeForObservation(graph CandidateGraph, requirement Requirement, observation Observation) (string, bool) {
	if observation.Kind == "port" {
		for _, node := range graph.Nodes {
			if node.Scope == "external" && node.SemanticKind == "port" && node.SemanticID == observation.ID {
				return node.ID, true
			}
		}
		return "", false
	}
	if observation.Kind != "domain" {
		return "", false
	}
	candidates := []GraphNode{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Domain == observation.ID &&
			(node.Role == "supply" || node.Role == "reference") {
			candidates = append(candidates, node)
		}
	}
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0].ID, true
}

func CloneGraph(graph CandidateGraph) CandidateGraph {
	result := graph
	result.Nodes = slices.Clone(graph.Nodes)
	result.Instances = make([]GraphInstance, len(graph.Instances))
	for index, instance := range graph.Instances {
		result.Instances[index] = instance
		result.Instances[index].Terminals = slices.Clone(instance.Terminals)
		if instance.ValueSI != nil {
			value := *instance.ValueSI
			result.Instances[index].ValueSI = &value
		}
	}
	return result
}

func AddInternalNode(graph CandidateGraph, role string) CandidateGraph {
	result, _ := addInternalNode(graph, role)
	return result
}

// addInternalNode returns the generated identifier with the cloned graph so
// compound graph operations never have to infer it from slice position.
func addInternalNode(graph CandidateGraph, role string) (CandidateGraph, string) {
	result := CloneGraph(graph)
	used := map[string]bool{}
	for _, node := range result.Nodes {
		used[node.ID] = true
	}
	index := 0
	for _, node := range result.Nodes {
		if node.Scope == "internal" {
			index++
		}
	}
	id := ""
	for id == "" || used[id] {
		id = fmt.Sprintf("internal_%03d", index)
		index++
	}
	result.Nodes = append(result.Nodes, GraphNode{
		ID:    id,
		Scope: "internal",
		Role:  canonicalIdentifier(role),
	})
	return result, id
}

func AddPrimitive(graph CandidateGraph, primitive PrimitiveCandidate, value *float64, connections []TerminalConnection) CandidateGraph {
	result := CloneGraph(graph)
	used := map[string]bool{}
	for _, instance := range result.Instances {
		used[instance.ID] = true
	}
	index := len(result.Instances)
	id := ""
	for id == "" || used[id] {
		id = fmt.Sprintf("primitive_%03d", index)
		index++
	}
	instance := GraphInstance{
		ID:           id,
		PrimitiveKey: primitive.Key,
		Kind:         primitive.Kind,
		ValueSI:      cloneInventoryFloat(value),
		Terminals:    append([]TerminalConnection(nil), connections...),
	}
	slices.SortFunc(instance.Terminals, compareTerminalConnections)
	result.Instances = append(result.Instances, instance)
	return result
}

func graphRoleForPort(port Port) string {
	switch {
	case port.Kind == "reference":
		return "reference"
	case port.Kind == "power" && port.Direction == "sink":
		return "supply"
	case port.Direction == "source" || port.Kind == "controlled_current":
		return "output"
	case port.Kind == "digital":
		return "control"
	default:
		return "input"
	}
}

func compareGraphNodes(left, right GraphNode) int {
	scopeRank := func(scope string) int {
		if scope == "external" {
			return 0
		}
		return 1
	}
	return cmp.Or(
		cmp.Compare(scopeRank(left.Scope), scopeRank(right.Scope)),
		cmp.Compare(left.SemanticKind, right.SemanticKind),
		cmp.Compare(left.SemanticID, right.SemanticID),
		cmp.Compare(left.Domain, right.Domain),
		cmp.Compare(left.Role, right.Role),
		cmp.Compare(left.ID, right.ID),
	)
}

func compareTerminalConnections(left, right TerminalConnection) int {
	return cmp.Or(
		cmp.Compare(left.Terminal, right.Terminal),
		cmp.Compare(left.Node, right.Node),
	)
}

func graphIssue(code reports.Code, path, message, suggestion string) reports.Issue {
	return reports.Issue{
		Code:       code,
		Severity:   reports.SeverityError,
		Stage:      "open_topology_graph",
		Path:       path,
		Message:    message,
		Suggestion: suggestion,
	}
}

func validGraphID(value string) bool {
	return semanticIDPattern.MatchString(strings.ToLower(strings.TrimSpace(value)))
}
