package opentopologysynthesis

import (
	"errors"
	"reflect"
	"testing"
)

func TestTerminalLevelGraphOperationsAreGenericAndValidated(t *testing.T) {
	inventory := testGraphInventory()
	resistor, found := primitiveByKey(inventory, "resistor|0603")
	if !found {
		t.Fatal("test resistor is missing")
	}
	graph := CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "ground", Role: "input"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "ground", Role: "output"},
			{ID: "ground", Scope: "external", SemanticKind: "port", SemanticID: "ground", Domain: "ground", Role: "reference"},
		},
		Instances: []GraphInstance{{
			ID:           "candidate",
			PrimitiveKey: resistor.Key,
			Kind:         resistor.Kind,
			ValueSI:      graphFloat(1000),
			Terminals:    []TerminalConnection{},
		}},
	}
	limits := GraphLimits{MaxPrimitiveInstances: 8, MaxInternalNodes: 8}
	if issues := ValidatePartialGraph(graph, inventory, limits); len(issues) != 0 {
		t.Fatalf("terminal-level partial graph issues: %#v", issues)
	}
	if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) == 0 {
		t.Fatal("incomplete terminal-level graph passed complete validation")
	}

	var err error
	graph, err = ConnectPrimitiveTerminal(graph, inventory, "candidate", "A", "input")
	if err != nil {
		t.Fatal(err)
	}
	graph, err = ConnectPrimitiveTerminal(graph, inventory, "candidate", "B", "output")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConnectPrimitiveTerminal(graph, inventory, "candidate", "B", "ground"); !errors.Is(err, ErrGraphTerminalConnected) {
		t.Fatalf("duplicate terminal connection error = %v", err)
	}

	graph, err = BridgeNodesWithPrimitive(graph, resistor, graphFloat(10_000), "output", "ground")
	if err != nil {
		t.Fatal(err)
	}
	graph, err = RedirectPrimitiveTerminal(graph, inventory, "candidate", "B", "ground")
	if err != nil {
		t.Fatal(err)
	}
	if graph.Instances[0].Terminals[1].Node != "ground" {
		t.Fatalf("redirected terminal = %#v", graph.Instances[0].Terminals)
	}
	graph, err = RedirectPrimitiveTerminal(graph, inventory, "candidate", "B", "output")
	if err != nil {
		t.Fatal(err)
	}
	if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) != 0 {
		t.Fatalf("connected graph issues: %#v", issues)
	}
}

func TestJoinSubstituteAndGuardedRemoveGraphOperations(t *testing.T) {
	inventory := testGraphInventory()
	graph := testDiamondGraph("branch_a", "branch_b")
	graph.Nodes = append(graph.Nodes, GraphNode{ID: "unused", Scope: "internal", Role: "internal"})
	var err error
	graph, err = RemoveUnusedInternalNode(graph, "unused")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveUnusedInternalNode(graph, "branch_a"); !errors.Is(err, ErrGraphNodeInUse) {
		t.Fatalf("used internal-node removal error = %v", err)
	}

	graph, err = JoinAnonymousNodes(graph, "branch_a", "branch_b")
	if err != nil {
		t.Fatal(err)
	}
	if _, found := graphNodeByID(graph, "branch_b"); found {
		t.Fatal("joined node remains in graph")
	}

	graph, err = SubstitutePrimitive(graph, inventory, "r1", "resistor|0805")
	if err != nil {
		t.Fatal(err)
	}
	if graph.Instances[0].PrimitiveKey != "resistor|0805" {
		t.Fatalf("substitution did not update primitive: %#v", graph.Instances[0])
	}
	if _, err := SubstitutePrimitive(graph, inventory, "r1", "capacitor|0603"); !errors.Is(err, ErrGraphIncompatiblePrimitive) {
		t.Fatalf("incompatible substitution error = %v", err)
	}

	if _, err := RemoveIrrelevantPrimitive(graph, "r1"); !errors.Is(err, ErrGraphPrimitiveRelevant) {
		t.Fatalf("relevant primitive removal error = %v", err)
	}
	graph.Instances = append(graph.Instances, GraphInstance{
		ID: "irrelevant", PrimitiveKey: "resistor|0603", Kind: "resistor", ValueSI: graphFloat(1000),
		Terminals: []TerminalConnection{{Terminal: "A", Node: "input"}, {Terminal: "B", Node: "input"}},
	})
	graph, err = RemoveIrrelevantPrimitive(graph, "irrelevant")
	if err != nil {
		t.Fatal(err)
	}
	if graphInstanceIndex(graph, "irrelevant") >= 0 {
		t.Fatal("irrelevant primitive remains in graph")
	}
}

func TestSplitPrimitiveInSeriesPreservesEndpointsAndAddsAnonymousNode(t *testing.T) {
	inventory := testGraphInventory()
	resistor, found := primitiveByKey(inventory, "resistor|0603")
	if !found {
		t.Fatal("test resistor is missing")
	}
	graph := CandidateGraph{
		Schema: CandidateGraphSchema, Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "signal", Role: "input"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "signal", Role: "output"},
		},
		Instances: []GraphInstance{{
			ID: "edge", PrimitiveKey: resistor.Key, Kind: resistor.Kind,
			ValueSI:   graphFloat(1000),
			Terminals: []TerminalConnection{{Terminal: "A", Node: "input"}, {Terminal: "B", Node: "output"}},
		}},
	}

	original := CloneGraph(graph)
	result, err := SplitPrimitiveInSeries(graph, inventory, "edge", resistor, graphFloat(2200))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(graph, original) {
		t.Fatalf("successful split mutated its input graph:\noriginal=%#v\nmutated=%#v", original, graph)
	}
	if len(result.Nodes) != 3 || len(result.Instances) != 2 {
		t.Fatalf("split graph dimensions = %d nodes, %d instances", len(result.Nodes), len(result.Instances))
	}
	seriesNode := result.Nodes[2]
	if seriesNode.Scope != "internal" || seriesNode.SemanticKind != "" || seriesNode.SemanticID != "" {
		t.Fatalf("series node is not anonymous internal: %#v", seriesNode)
	}
	if got := result.Instances[0].Terminals; got[0].Node != "input" || got[1].Node != seriesNode.ID {
		t.Fatalf("original edge endpoints = %#v", got)
	}
	if got := result.Instances[1].Terminals; got[0].Node != seriesNode.ID || got[1].Node != "output" {
		t.Fatalf("inserted edge endpoints = %#v", got)
	}
	if result.Instances[1].ValueSI == nil || *result.Instances[1].ValueSI != 2200 {
		t.Fatalf("inserted edge value = %#v", result.Instances[1].ValueSI)
	}
	if issues := ValidateCompleteGraph(result, inventory, GraphLimits{MaxPrimitiveInstances: 4, MaxInternalNodes: 2}); len(issues) != 0 {
		t.Fatalf("split complete graph issues: %#v", issues)
	}
}

func TestSplitPrimitiveInSeriesErrorDoesNotMutateInput(t *testing.T) {
	inventory := testGraphInventory()
	resistor, found := primitiveByKey(inventory, "resistor|0603")
	if !found {
		t.Fatal("test resistor is missing")
	}
	graph := CandidateGraph{
		Schema: CandidateGraphSchema, Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", Role: "input"},
			{ID: "output", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{{
			ID: "edge", PrimitiveKey: resistor.Key, Kind: resistor.Kind,
			ValueSI: graphFloat(1000),
			Terminals: []TerminalConnection{
				{Terminal: "A", Node: "input"},
				{Terminal: "UNKNOWN", Node: "output"},
			},
		}},
	}
	original := CloneGraph(graph)
	if _, err := SplitPrimitiveInSeries(graph, inventory, "edge", resistor, graphFloat(2200)); !errors.Is(err, ErrGraphTerminalNotFound) {
		t.Fatalf("split error = %v, want %v", err, ErrGraphTerminalNotFound)
	}
	if !reflect.DeepEqual(graph, original) {
		t.Fatalf("failed split mutated its input graph:\noriginal=%#v\nmutated=%#v", original, graph)
	}
}
