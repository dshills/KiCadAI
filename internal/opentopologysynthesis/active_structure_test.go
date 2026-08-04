package opentopologysynthesis

import "testing"

func TestActiveStructureHashIgnoresResistorDecompositionButSeparatesBufferedControl(t *testing.T) {
	direct := activeStructureTestGraph(false, false)
	split := activeStructureTestGraph(true, false)
	buffered := activeStructureTestGraph(false, true)
	directTopology, err := TopologyHash(direct)
	if err != nil {
		t.Fatal(err)
	}
	splitTopology, err := TopologyHash(split)
	if err != nil {
		t.Fatal(err)
	}
	if directTopology == splitTopology {
		t.Fatal("complete topology hash ignored resistor decomposition")
	}
	directActive, err := ActiveStructureHash(direct)
	if err != nil {
		t.Fatal(err)
	}
	splitActive, err := ActiveStructureHash(split)
	if err != nil {
		t.Fatal(err)
	}
	bufferedActive, err := ActiveStructureHash(buffered)
	if err != nil {
		t.Fatal(err)
	}
	if directActive == "" || directActive != splitActive {
		t.Fatalf("direct/split active hashes = %q/%q", directActive, splitActive)
	}
	if bufferedActive == directActive {
		t.Fatalf("buffered active hash = direct hash %q", directActive)
	}
}

func TestActiveStructureHashSeparatesDelimiterBearingSemantics(t *testing.T) {
	first := activeStructureTestGraph(false, false)
	second := activeStructureTestGraph(false, false)
	first.Nodes[0].SemanticKind = "port:power"
	first.Nodes[0].SemanticID = "supply"
	second.Nodes[0].SemanticKind = "port"
	second.Nodes[0].SemanticID = "power:supply"
	firstHash, err := ActiveStructureHash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := ActiveStructureHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatalf("delimiter-bearing semantics collided at %q", firstHash)
	}
}

func TestActiveStructureHashRejectsMalformedResistor(t *testing.T) {
	graph := activeStructureTestGraph(false, false)
	graph.Instances[len(graph.Instances)-1].Terminals = graph.Instances[len(graph.Instances)-1].Terminals[:1]
	if _, err := ActiveStructureHash(graph); err == nil {
		t.Fatal("malformed one-terminal resistor produced active-structure evidence")
	}
}

func TestActiveStructureHashIsIndependentOfInputNodeOrder(t *testing.T) {
	ordered := activeStructureTestGraph(false, true)
	reversed := CloneGraph(ordered)
	for left, right := 0, len(reversed.Nodes)-1; left < right; left, right = left+1, right-1 {
		reversed.Nodes[left], reversed.Nodes[right] = reversed.Nodes[right], reversed.Nodes[left]
	}
	orderedHash, err := ActiveStructureHash(ordered)
	if err != nil {
		t.Fatal(err)
	}
	reversedHash, err := ActiveStructureHash(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if orderedHash != reversedHash {
		t.Fatalf("node-order-dependent active hashes = %q/%q", orderedHash, reversedHash)
	}
}

func activeStructureTestGraph(splitDrive, buffered bool) CandidateGraph {
	graph := CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "supply", Scope: "external", SemanticKind: "port", SemanticID: "supply", Role: "supply"},
			{ID: "reference", Scope: "external", SemanticKind: "port", SemanticID: "reference", Role: "reference"},
			{ID: "command", Scope: "external", SemanticKind: "port", SemanticID: "command", Role: "control"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Role: "output"},
			{ID: "drive", Scope: "internal", Role: "control"},
			{ID: "base", Scope: "internal", Role: "control"},
		},
		Instances: []GraphInstance{
			{ID: "controller", PrimitiveKey: "opamp.a", Kind: "opamp", Terminals: []TerminalConnection{
				{Terminal: "IN_PLUS", Node: "command"}, {Terminal: "IN_MINUS", Node: "output"},
				{Terminal: "OUT", Node: "drive"}, {Terminal: "V_PLUS", Node: "supply"},
				{Terminal: "V_MINUS", Node: "reference"},
			}},
			{ID: "pass", PrimitiveKey: "pnp.a", Kind: "pnp_bjt", Terminals: []TerminalConnection{
				{Terminal: "BASE", Node: "base"}, {Terminal: "COLLECTOR", Node: "output"},
				{Terminal: "EMITTER", Node: "supply"},
			}},
		},
	}
	switch {
	case buffered:
		graph.Nodes = append(graph.Nodes, GraphNode{ID: "driver_base", Scope: "internal", Role: "control"})
		graph.Instances = append(graph.Instances,
			GraphInstance{ID: "drive_resistor", PrimitiveKey: "resistor.10k", Kind: "resistor", Terminals: topologyTwoTerminalPlacement("drive", "driver_base")},
			GraphInstance{ID: "pullup", PrimitiveKey: "resistor.10k", Kind: "resistor", Terminals: topologyTwoTerminalPlacement("supply", "base")},
			GraphInstance{ID: "driver", PrimitiveKey: "npn.a", Kind: "npn_bjt", Terminals: []TerminalConnection{
				{Terminal: "BASE", Node: "driver_base"}, {Terminal: "COLLECTOR", Node: "base"},
				{Terminal: "EMITTER", Node: "reference"},
			}},
		)
	case splitDrive:
		graph.Nodes = append(graph.Nodes, GraphNode{ID: "mid", Scope: "internal", Role: "control"})
		graph.Instances = append(graph.Instances,
			GraphInstance{ID: "drive_resistor_a", PrimitiveKey: "resistor.4k7", Kind: "resistor", Terminals: topologyTwoTerminalPlacement("drive", "mid")},
			GraphInstance{ID: "drive_resistor_b", PrimitiveKey: "resistor.5k3", Kind: "resistor", Terminals: topologyTwoTerminalPlacement("mid", "base")},
		)
	default:
		graph.Instances = append(graph.Instances,
			GraphInstance{ID: "drive_resistor", PrimitiveKey: "resistor.10k", Kind: "resistor", Terminals: topologyTwoTerminalPlacement("drive", "base")},
		)
	}
	return graph
}
