package opentopologysynthesis

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func TestCanonicalGraphHashIgnoresNamesOrderAndPassiveOrientation(t *testing.T) {
	first := testRCGraph()
	second := CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "z_ref", Scope: "external", SemanticKind: "port", SemanticID: "ground", Domain: "ground", Role: "reference"},
			{ID: "z_out", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "ground", Role: "output"},
			{ID: "z_in", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "ground", Role: "input"},
		},
		Instances: []GraphInstance{
			{ID: "part_b", PrimitiveKey: "capacitor|0603", Kind: "capacitor", ValueSI: graphFloat(1e-6), Terminals: []TerminalConnection{{Terminal: "B", Node: "z_ref"}, {Terminal: "A", Node: "z_out"}}},
			{ID: "part_a", PrimitiveKey: "resistor|0603", Kind: "resistor", ValueSI: graphFloat(1000), Terminals: []TerminalConnection{{Terminal: "B", Node: "z_in"}, {Terminal: "A", Node: "z_out"}}},
		},
	}
	firstHash := mustGraphHash(t, first, false)
	secondHash := mustGraphHash(t, second, false)
	if firstHash != secondHash {
		t.Fatalf("full graph hash changed under names/order/orientation: %s != %s", firstHash, secondHash)
	}
	firstTopology := mustGraphHash(t, first, true)
	secondTopology := mustGraphHash(t, second, true)
	if firstTopology != secondTopology {
		t.Fatalf("topology hash changed under names/order/orientation: %s != %s", firstTopology, secondTopology)
	}
	firstJSON, err := CanonicalGraphJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := CanonicalGraphJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical graph JSON differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestTopologyHashSeparatesStructureButNotCatalogOrValue(t *testing.T) {
	base := testRCGraph()
	alternative := CloneGraph(base)
	alternative.Instances[0].PrimitiveKey = "resistor|0805"
	alternative.Instances[0].ValueSI = graphFloat(2200)
	if mustGraphHash(t, base, false) == mustGraphHash(t, alternative, false) {
		t.Fatal("full graph hash ignored catalog/value change")
	}
	if mustGraphHash(t, base, true) != mustGraphHash(t, alternative, true) {
		t.Fatal("topology hash changed for catalog/value-only alternative")
	}

	rewired := CloneGraph(base)
	rewired.Instances[1].Terminals[0].Node = "input"
	if mustGraphHash(t, base, true) == mustGraphHash(t, rewired, true) {
		t.Fatal("topology hash did not distinguish rewired graph")
	}
}

func TestNormalizeGraphDoesNotMutateNonCanonicalTerminalBindings(t *testing.T) {
	graph := testDiamondGraph("left_branch", "right_branch")
	before, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := NormalizeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("normalization mutated its input:\n%s\n%s", before, after)
	}
	if _, err := GraphHash(normalized); err != nil {
		t.Fatalf("normalized graph is invalid: %v", err)
	}
}

func TestCanonicalGraphHashHandlesSymmetricInternalNodes(t *testing.T) {
	first := testDiamondGraph("left", "right")
	second := testDiamondGraph("right_renamed", "left_renamed")
	slices.Reverse(second.Nodes)
	slices.Reverse(second.Instances)
	for index := range second.Instances {
		slices.Reverse(second.Instances[index].Terminals)
	}
	if mustGraphHash(t, first, false) != mustGraphHash(t, second, false) {
		t.Fatal("symmetric diamond graph hash depends on internal naming or ordering")
	}
	normalized, err := NormalizeGraph(first)
	if err != nil {
		t.Fatal(err)
	}
	again, err := NormalizeGraph(normalized)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(normalized)
	secondJSON, _ := json.Marshal(again)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("graph normalization is not idempotent:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestDirectedPrimitiveTerminalOrientationAffectsTopology(t *testing.T) {
	first := testDiodeGraph("ANODE", "CATHODE")
	second := testDiodeGraph("CATHODE", "ANODE")
	if mustGraphHash(t, first, true) == mustGraphHash(t, second, true) {
		t.Fatal("directed diode orientation was treated as symmetric")
	}
}

func TestCompleteGraphValidationRejectsInvalidAndAcceptsConnectedGraphs(t *testing.T) {
	inventory := testGraphInventory()
	limits := GraphLimits{MaxPrimitiveInstances: 8, MaxInternalNodes: 8}
	valid := testRCGraph()
	if issues := ValidateCompleteGraph(valid, inventory, limits); len(issues) != 0 {
		t.Fatalf("valid RC graph issues: %#v", issues)
	}

	tests := []struct {
		name   string
		mutate func(CandidateGraph) CandidateGraph
	}{
		{"unknown_primitive", func(graph CandidateGraph) CandidateGraph {
			graph.Instances[0].PrimitiveKey = "missing"
			return graph
		}},
		{"missing_terminal", func(graph CandidateGraph) CandidateGraph {
			graph.Instances[0].Terminals = graph.Instances[0].Terminals[:1]
			return graph
		}},
		{"unknown_node", func(graph CandidateGraph) CandidateGraph {
			graph.Instances[0].Terminals[0].Node = "missing"
			return graph
		}},
		{"disconnected_external", func(graph CandidateGraph) CandidateGraph {
			graph.Instances = graph.Instances[:1]
			return graph
		}},
		{"dangling_internal", func(graph CandidateGraph) CandidateGraph {
			graph.Nodes = append(graph.Nodes, GraphNode{ID: "dangling", Scope: "internal", Role: "internal"})
			return graph
		}},
		{"value_out_of_domain", func(graph CandidateGraph) CandidateGraph {
			graph.Instances[0].ValueSI = graphFloat(2e9)
			return graph
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := test.mutate(CloneGraph(valid))
			if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) == 0 {
				t.Fatal("invalid graph passed validation")
			}
		})
	}
}

func TestNominalOnlyPrimitiveDomainRejectsRelabeledValues(t *testing.T) {
	domain := PrimitiveValueDomain{
		Kind:    "resistance",
		Unit:    "ohm",
		Nominal: graphFloat(47),
	}
	if !valueWithinPrimitiveDomain(47, domain) {
		t.Fatal("nominal value was rejected")
	}
	if valueWithinPrimitiveDomain(4_990, domain) {
		t.Fatal("nominal-only component accepted a different written value")
	}
}

func TestActivePrimitiveRequiresExplicitSupplyAndReference(t *testing.T) {
	inventory := testGraphInventory()
	graph := CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "ground", Scope: "external", SemanticKind: "port", SemanticID: "ground", Domain: "ground", Role: "reference"},
			{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "ground", Role: "input"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "ground", Role: "output"},
			{ID: "supply", Scope: "external", SemanticKind: "port", SemanticID: "power", Domain: "supply", Role: "supply"},
		},
		Instances: []GraphInstance{{
			ID: "amplifier", PrimitiveKey: "opamp|single", Kind: "opamp",
			Terminals: []TerminalConnection{
				{Terminal: "IN_PLUS", Node: "input"},
				{Terminal: "IN_MINUS", Node: "output"},
				{Terminal: "OUT", Node: "output"},
				{Terminal: "V_PLUS", Node: "supply"},
				{Terminal: "V_MINUS", Node: "ground"},
			},
		}},
	}
	limits := GraphLimits{MaxPrimitiveInstances: 8, MaxInternalNodes: 8}
	if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) != 0 {
		t.Fatalf("valid active graph issues: %#v", issues)
	}
	graph.Instances[0].Terminals[3].Node = "ground"
	if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) == 0 {
		t.Fatal("active graph with invalid positive supply passed")
	}
}

func TestInitialGraphContainsOnlySemanticExternalPorts(t *testing.T) {
	data := mustRead(t, filepath.Join(frozenCorpusRoot(), "powered_lowpass.json"))
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("decode issues: %#v", issues)
	}
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	if len(graph.Nodes) != len(requirement.Requirements.Ports) || len(graph.Instances) != 0 {
		t.Fatalf("initial graph = %#v", graph)
	}
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.SemanticKind != "port" {
			t.Fatalf("non-semantic initial node: %#v", node)
		}
	}
	if node, ok := ExternalNodeForObservation(graph, requirement, Observation{Kind: "domain", ID: "supply"}); !ok || node != "port_power" {
		t.Fatalf("supply-domain observation binding = %q/%t", node, ok)
	}
}

func testGraphInventory() PrimitiveInventory {
	return PrimitiveInventory{
		Schema:  PrimitiveInventorySchema,
		Version: PrimitiveInventoryVersion,
		Primitives: []PrimitiveCandidate{
			testPrimitive("resistor|0603", "resistor", []string{"A", "B"}, &PrimitiveValueDomain{Kind: "resistance", Unit: "ohm", Minimum: graphFloat(1), Maximum: graphFloat(1e9)}),
			testPrimitive("resistor|0805", "resistor", []string{"A", "B"}, &PrimitiveValueDomain{Kind: "resistance", Unit: "ohm", Minimum: graphFloat(1), Maximum: graphFloat(1e9)}),
			testPrimitive("capacitor|0603", "capacitor", []string{"A", "B"}, &PrimitiveValueDomain{Kind: "capacitance", Unit: "F", Minimum: graphFloat(1e-12), Maximum: graphFloat(1)}),
			testPrimitive("diode|sod", "signal_diode", []string{"ANODE", "CATHODE"}, nil),
			testPrimitive("opamp|single", "opamp", []string{"IN_PLUS", "IN_MINUS", "OUT", "V_PLUS", "V_MINUS"}, nil),
		},
	}
}

func testPrimitive(key, kind string, terminalNames []string, domain *PrimitiveValueDomain) PrimitiveCandidate {
	terminals := make([]PrimitiveTerminal, 0, len(terminalNames))
	for index, terminal := range terminalNames {
		terminals = append(terminals, PrimitiveTerminal{
			Terminal:  terminal,
			Function:  terminal,
			SymbolID:  "Test:" + kind,
			SymbolPin: graphIndex(index + 1),
			Pad:       graphIndex(index + 1),
			Required:  true,
		})
	}
	result := PrimitiveCandidate{
		Key: key, Kind: kind, Family: kind, CatalogID: key, VariantID: "test",
		Evidence: "verified", Terminals: terminals, ValueDomain: domain,
		Models: []PrimitiveModelContract{{
			ModelID: "test_" + kind,
			AllowedAnalyses: []string{
				"ac_sweep", "dc_operating_point", "dc_sweep", "distortion", "electrothermal",
				"noise", "stability", "startup", "thermal", "transient",
			},
			ProvenanceSource:   "test",
			ProvenanceRevision: "test",
			ProvenanceSHA256:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
	}
	if domain != nil {
		result.Tolerances = []PrimitiveBound{{
			Kind: domain.Kind, Unit: "%", Maximum: graphFloat(1),
		}}
	}
	return result
}

func testRCGraph() CandidateGraph {
	return CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "ground", Role: "input"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "ground", Role: "output"},
			{ID: "ground", Scope: "external", SemanticKind: "port", SemanticID: "ground", Domain: "ground", Role: "reference"},
		},
		Instances: []GraphInstance{
			{ID: "r1", PrimitiveKey: "resistor|0603", Kind: "resistor", ValueSI: graphFloat(1000), Terminals: []TerminalConnection{{Terminal: "A", Node: "input"}, {Terminal: "B", Node: "output"}}},
			{ID: "c1", PrimitiveKey: "capacitor|0603", Kind: "capacitor", ValueSI: graphFloat(1e-6), Terminals: []TerminalConnection{{Terminal: "A", Node: "output"}, {Terminal: "B", Node: "ground"}}},
		},
	}
}

func testDiamondGraph(left, right string) CandidateGraph {
	return CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "ground", Role: "input"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "ground", Role: "output"},
			{ID: left, Scope: "internal", Role: "internal"},
			{ID: right, Scope: "internal", Role: "internal"},
		},
		Instances: []GraphInstance{
			testResistorInstance("r1", "input", left),
			testResistorInstance("r2", left, "output"),
			testResistorInstance("r3", "input", right),
			testResistorInstance("r4", right, "output"),
		},
	}
}

func testResistorInstance(id, left, right string) GraphInstance {
	return GraphInstance{ID: id, PrimitiveKey: "resistor|0603", Kind: "resistor", ValueSI: graphFloat(1000), Terminals: []TerminalConnection{{Terminal: "A", Node: left}, {Terminal: "B", Node: right}}}
}

func testDiodeGraph(inputTerminal, outputTerminal string) CandidateGraph {
	return CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "ground", Role: "input"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "ground", Role: "output"},
		},
		Instances: []GraphInstance{{
			ID: "d1", PrimitiveKey: "diode|sod", Kind: "signal_diode",
			Terminals: []TerminalConnection{{Terminal: inputTerminal, Node: "input"}, {Terminal: outputTerminal, Node: "output"}},
		}},
	}
}

func mustGraphHash(t *testing.T, graph CandidateGraph, topology bool) string {
	t.Helper()
	var (
		hash string
		err  error
	)
	if topology {
		hash, err = TopologyHash(graph)
	} else {
		hash, err = GraphHash(graph)
	}
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func graphFloat(value float64) *float64 {
	return &value
}

func graphIndex(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(digits[value%10]) + result
		value /= 10
	}
	return result
}
