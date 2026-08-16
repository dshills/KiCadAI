package opentopologysynthesis

import (
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

func TestV19CausalInvariantsAcceptFeedForwardGraphDeterministically(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph := causalV19FeedForwardGraph(t, requirement)
	inventory := causalV19Inventory(t)
	first := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
	second := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
	if len(first) != 0 {
		t.Fatalf("valid feed-forward graph rejected: %#v", first)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("validation replay differs: first=%#v second=%#v", first, second)
	}
}

func TestV19CausalInvariantsRejectArbitraryCycleAndAcceptTypedPassiveFeedback(t *testing.T) {
	requirement := causalV19Requirement("hysteresis", "dc_sweep")
	graph := causalV19FeedbackGraph(t, requirement)
	inventory := causalV19Inventory(t)
	issues := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
	causalV19RequireIssue(t, issues, "directed causal cycle")

	context := CausalInvariantContextV19{FeedbackPaths: []CausalFeedbackPathV19{{
		FromInstance: "amp", FromTerminal: "OUT",
		ToInstance: "amp", ToTerminal: "IN_MINUS",
		ObligationID: "transfer_bound",
	}}}
	if issues = ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, context); len(issues) != 0 {
		t.Fatalf("valid typed passive feedback rejected: %#v", issues)
	}

	context.FeedbackPaths[0].ObligationID = "unknown_obligation"
	issues = ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, context)
	causalV19RequireIssue(t, issues, "not bound to a feedback-sensitive external obligation")
}

func TestV19CausalInvariantsRejectPushPullContentionAndAcceptRatedOpenCollectorSharing(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	inventory := causalV19Inventory(t)
	pushPull := causalV19FeedForwardGraph(t, requirement)
	second := pushPull.Instances[0]
	second.ID = "amp_b"
	pushPull.Instances = append(pushPull.Instances, second)
	issues := ValidateCausalGraphV19(requirement, pushPull, inventory, GraphLimits{}, CausalInvariantContextV19{})
	causalV19RequireIssue(t, issues, "multiple active push-pull")

	openCollector := causalV19OpenCollectorGraph(t, requirement)
	if issues = ValidateCausalGraphV19(requirement, openCollector, inventory, GraphLimits{}, CausalInvariantContextV19{}); len(issues) != 0 {
		t.Fatalf("rated open-collector sharing rejected: %#v", issues)
	}

	openCollector.Instances = openCollector.Instances[:2]
	issues = ValidateCausalGraphV19(requirement, openCollector, inventory, GraphLimits{}, CausalInvariantContextV19{})
	causalV19RequireIssue(t, issues, "no registry-backed passive pull resource")
}

func TestV19CausalInvariantsRejectDomainAndRatingViolations(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	baseGraph := causalV19FeedForwardGraph(t, requirement)
	baseInventory := causalV19Inventory(t)

	t.Run("external domain binding", func(t *testing.T) {
		graph := CloneGraph(baseGraph)
		for index := range graph.Nodes {
			if graph.Nodes[index].SemanticID == "signal_out" {
				graph.Nodes[index].Domain = "supply"
			}
		}
		issues := ValidateCausalGraphV19(requirement, graph, baseInventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "changes its declared port domain or role")
	})

	t.Run("voltage rating", func(t *testing.T) {
		inventory := causalV19CloneInventory(baseInventory)
		primitive := causalV19Primitive(&inventory, "opamp")
		low := 2.0
		primitive.Ratings = []PrimitiveBound{{Kind: "supply_voltage", Unit: "V", Maximum: &low}, {Kind: "output_current", Unit: "A", Maximum: causalV19Float(.1)}}
		causalV19ReplacePrimitive(&inventory, primitive)
		causalV19SealInventory(t, &inventory)
		issues := ValidateCausalGraphV19(requirement, baseGraph, inventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "does not cover declared")
	})

	t.Run("output current rating", func(t *testing.T) {
		inventory := causalV19CloneInventory(baseInventory)
		primitive := causalV19Primitive(&inventory, "opamp")
		low := .001
		primitive.Ratings = []PrimitiveBound{{Kind: "supply_voltage", Unit: "V", Maximum: causalV19Float(10)}, {Kind: "output_current", Unit: "A", Maximum: &low}}
		causalV19ReplacePrimitive(&inventory, primitive)
		causalV19SealInventory(t, &inventory)
		issues := ValidateCausalGraphV19(requirement, baseGraph, inventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "does not cover declared")
	})
}

func TestV19CausalInvariantsRejectFloatingAndMergedReferences(t *testing.T) {
	baseRequirement := causalV19Requirement("voltage_gain", "ac_sweep")
	inventory := causalV19Inventory(t)

	t.Run("floating", func(t *testing.T) {
		graph := causalV19FloatingBufferGraph(t, baseRequirement)
		issues := ValidateCausalGraphV19(baseRequirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "floating or multiply merged physical reference")
	})

	t.Run("merged", func(t *testing.T) {
		requirement := causalV19Requirement("voltage_gain", "ac_sweep")
		requirement.Requirements.Domains = append(requirement.Requirements.Domains, Domain{
			ID: "isolated_reference", Kind: "reference", Source: "external",
			MinVoltageV: causalV19Float(0), NominalVoltageV: causalV19Float(0), MaxVoltageV: causalV19Float(0),
		})
		for index := range requirement.Requirements.Ports {
			if requirement.Requirements.Ports[index].ID == "signal_out" {
				requirement.Requirements.Ports[index].Domain = "isolated_reference"
			}
		}
		graph := causalV19FeedForwardGraph(t, requirement)
		value := 10_000.0
		graph.Instances = append(graph.Instances, GraphInstance{
			ID: "isolated_return", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: &value,
			Terminals: []TerminalConnection{{Terminal: "A", Node: "port_signal_out"}, {Terminal: "B", Node: "domain_isolated_reference"}},
		})
		issues := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "lacks a unique declared reference domain")
	})
}

func TestV19CausalInvariantsRejectUnreviewedOrAnalysisIncompletePrimitive(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph := causalV19FeedForwardGraph(t, requirement)
	base := causalV19Inventory(t)

	t.Run("provenance", func(t *testing.T) {
		inventory := causalV19CloneInventory(base)
		primitive := causalV19Primitive(&inventory, "opamp")
		primitive.Models[0].ProvenanceSHA256 = "unreviewed"
		causalV19ReplacePrimitive(&inventory, primitive)
		causalV19SealInventory(t, &inventory)
		issues := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "lacks complete reviewed provenance")
	})

	t.Run("analysis", func(t *testing.T) {
		inventory := causalV19CloneInventory(base)
		primitive := causalV19Primitive(&inventory, "opamp")
		primitive.Models[0].AllowedAnalyses = []string{"transient"}
		causalV19ReplacePrimitive(&inventory, primitive)
		causalV19SealInventory(t, &inventory)
		issues := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "no reviewed model for required analysis")
	})

	t.Run("terminal role", func(t *testing.T) {
		inventory := causalV19CloneInventory(base)
		primitive := causalV19Primitive(&inventory, "opamp")
		for index := range primitive.Terminals {
			if primitive.Terminals[index].Terminal == "OUT" {
				primitive.Terminals[index].Electrical = ""
			}
		}
		causalV19ReplacePrimitive(&inventory, primitive)
		causalV19SealInventory(t, &inventory)
		issues := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
		causalV19RequireIssue(t, issues, "lacks a supported causal electrical role")
	})
}

func TestV19CausalInvariantsPermutationAndReplayAreByteStable(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph := causalV19FeedForwardGraph(t, requirement)
	second := graph.Instances[0]
	second.ID = "amp_b"
	graph.Instances = append(graph.Instances, second)
	inventory := causalV19Inventory(t)
	want := ValidateCausalGraphV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{})
	if len(want) == 0 {
		t.Fatal("permutation fixture must contain a deterministic rejection")
	}
	random := rand.New(rand.NewSource(19019))
	for iteration := 0; iteration < 256; iteration++ {
		shuffledRequirement := requirement
		shuffledRequirement.Requirements.Domains = slices.Clone(requirement.Requirements.Domains)
		shuffledRequirement.Requirements.Ports = slices.Clone(requirement.Requirements.Ports)
		shuffledRequirement.Requirements.OperatingCases = slices.Clone(requirement.Requirements.OperatingCases)
		shuffledRequirement.Requirements.BehavioralRequirements = slices.Clone(requirement.Requirements.BehavioralRequirements)
		random.Shuffle(len(shuffledRequirement.Requirements.Domains), func(left, right int) {
			shuffledRequirement.Requirements.Domains[left], shuffledRequirement.Requirements.Domains[right] = shuffledRequirement.Requirements.Domains[right], shuffledRequirement.Requirements.Domains[left]
		})
		random.Shuffle(len(shuffledRequirement.Requirements.Ports), func(left, right int) {
			shuffledRequirement.Requirements.Ports[left], shuffledRequirement.Requirements.Ports[right] = shuffledRequirement.Requirements.Ports[right], shuffledRequirement.Requirements.Ports[left]
		})

		shuffledGraph := CloneGraph(graph)
		random.Shuffle(len(shuffledGraph.Nodes), func(left, right int) {
			shuffledGraph.Nodes[left], shuffledGraph.Nodes[right] = shuffledGraph.Nodes[right], shuffledGraph.Nodes[left]
		})
		random.Shuffle(len(shuffledGraph.Instances), func(left, right int) {
			shuffledGraph.Instances[left], shuffledGraph.Instances[right] = shuffledGraph.Instances[right], shuffledGraph.Instances[left]
		})
		for index := range shuffledGraph.Instances {
			random.Shuffle(len(shuffledGraph.Instances[index].Terminals), func(left, right int) {
				shuffledGraph.Instances[index].Terminals[left], shuffledGraph.Instances[index].Terminals[right] = shuffledGraph.Instances[index].Terminals[right], shuffledGraph.Instances[index].Terminals[left]
			})
		}

		shuffledInventory := causalV19CloneInventory(inventory)
		random.Shuffle(len(shuffledInventory.Primitives), func(left, right int) {
			shuffledInventory.Primitives[left], shuffledInventory.Primitives[right] = shuffledInventory.Primitives[right], shuffledInventory.Primitives[left]
		})
		for index := range shuffledInventory.Primitives {
			random.Shuffle(len(shuffledInventory.Primitives[index].Terminals), func(left, right int) {
				shuffledInventory.Primitives[index].Terminals[left], shuffledInventory.Primitives[index].Terminals[right] = shuffledInventory.Primitives[index].Terminals[right], shuffledInventory.Primitives[index].Terminals[left]
			})
			random.Shuffle(len(shuffledInventory.Primitives[index].Models), func(left, right int) {
				shuffledInventory.Primitives[index].Models[left], shuffledInventory.Primitives[index].Models[right] = shuffledInventory.Primitives[index].Models[right], shuffledInventory.Primitives[index].Models[left]
			})
		}
		causalV19SealInventory(t, &shuffledInventory)
		got := ValidateCausalGraphV19(shuffledRequirement, shuffledGraph, shuffledInventory, GraphLimits{}, CausalInvariantContextV19{})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("permutation %d changed issues:\n got=%#v\nwant=%#v", iteration, got, want)
		}
	}
}

func causalV19Requirement(metric, analysis string) Requirement {
	minimum, maximum := .9, 1.1
	unit := "ratio"
	if strings.Contains(metric, "hysteresis") {
		minimum, maximum, unit = .1, .2, "V"
	}
	return Requirement{
		Schema: RequirementSchema, Version: RequirementVersion,
		Project: Project{Name: "causal_invariant_fixture", Title: "Causal invariant fixture", Description: "Bounded public behavior for generic causal graph validation."},
		Requirements: Requirements{
			Domains: []Domain{
				{ID: "ground", Kind: "reference", Source: "external", MinVoltageV: causalV19Float(0), NominalVoltageV: causalV19Float(0), MaxVoltageV: causalV19Float(0), MaxCurrentA: causalV19Float(.2)},
				{ID: "supply", Kind: "supply", Source: "external", MinVoltageV: causalV19Float(3), NominalVoltageV: causalV19Float(4), MaxVoltageV: causalV19Float(5), MaxCurrentA: causalV19Float(.2)},
			},
			Ports: []Port{
				{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "ground", Electrical: Electrical{NominalVoltageV: causalV19Float(0), MaxCurrentA: causalV19Float(.2)}},
				{ID: "vcc", Kind: "power", Direction: "sink", Domain: "supply", Electrical: Electrical{MinVoltageV: causalV19Float(3), NominalVoltageV: causalV19Float(4), MaxVoltageV: causalV19Float(5), MaxCurrentA: causalV19Float(.2)}},
				{ID: "signal_in", Kind: "analog_voltage", Direction: "sink", Domain: "ground", Electrical: Electrical{MinVoltageV: causalV19Float(0), NominalVoltageV: causalV19Float(.5), MaxVoltageV: causalV19Float(1), InputImpedanceMinOhm: causalV19Float(100_000)}},
				{ID: "signal_out", Kind: "analog_voltage", Direction: "source", Domain: "ground", Electrical: Electrical{MinVoltageV: causalV19Float(0), NominalVoltageV: causalV19Float(.5), MaxVoltageV: causalV19Float(1), MaxCurrentA: causalV19Float(.01)}},
			},
			OperatingCases: []OperatingCase{{ID: "bounded", Conditions: []OperatingCondition{
				{Axis: "supply_voltage", Target: "supply", Min: 3, Max: 5, Unit: "V"},
				{Axis: "input_voltage", Target: "signal_in", Min: 0, Max: 1, Unit: "V"},
			}}},
			BehavioralRequirements: []BehavioralAssertion{{
				ID: "transfer_bound", Metric: metric, Analysis: analysis,
				Excitation: &Observation{Kind: "port", ID: "signal_in"}, Observation: Observation{Kind: "port", ID: "signal_out"},
				Min: &minimum, Max: &maximum, Unit: unit, OperatingCases: []string{"bounded"},
			}},
			Constraints: BoardLimits{MaxComponents: 16, MaxWidthMM: 40, MaxHeightMM: 30},
		},
		Acceptance: Acceptance{
			RequirePrimitiveOnly: true, RequireTopologySearch: true, RequireSimulation: true,
			RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true,
			RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true,
			RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true,
			RequireDeterministicReplay: true, RequireFailClosed: true,
		},
	}
}

func causalV19FeedForwardGraph(t *testing.T, requirement Requirement) CandidateGraph {
	t.Helper()
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph: %#v", issues)
	}
	graph.Instances = []GraphInstance{{
		ID: "amp", PrimitiveKey: "opamp", Kind: "opamp",
		Terminals: []TerminalConnection{
			{Terminal: "IN_PLUS", Node: "port_signal_in"},
			{Terminal: "IN_MINUS", Node: "port_ground"},
			{Terminal: "OUT", Node: "port_signal_out"},
			{Terminal: "V_PLUS", Node: "port_vcc"},
			{Terminal: "V_MINUS", Node: "port_ground"},
		},
	}}
	return graph
}

func causalV19FeedbackGraph(t *testing.T, requirement Requirement) CandidateGraph {
	t.Helper()
	graph := causalV19FeedForwardGraph(t, requirement)
	graph.Nodes = append(graph.Nodes, GraphNode{ID: "feedback_node", Scope: "internal", Role: "internal"})
	for index := range graph.Instances[0].Terminals {
		if graph.Instances[0].Terminals[index].Terminal == "IN_MINUS" {
			graph.Instances[0].Terminals[index].Node = "feedback_node"
		}
	}
	feedback, bias := 10_000.0, 100_000.0
	graph.Instances = append(graph.Instances,
		GraphInstance{ID: "feedback", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: &feedback, Terminals: []TerminalConnection{{Terminal: "A", Node: "port_signal_out"}, {Terminal: "B", Node: "feedback_node"}}},
		GraphInstance{ID: "bias", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: &bias, Terminals: []TerminalConnection{{Terminal: "A", Node: "feedback_node"}, {Terminal: "B", Node: "port_ground"}}},
	)
	return graph
}

func causalV19OpenCollectorGraph(t *testing.T, requirement Requirement) CandidateGraph {
	t.Helper()
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph: %#v", issues)
	}
	comparator := func(id string) GraphInstance {
		return GraphInstance{ID: id, PrimitiveKey: "comparator", Kind: "comparator", Terminals: []TerminalConnection{
			{Terminal: "IN_PLUS", Node: "port_signal_in"}, {Terminal: "IN_MINUS", Node: "port_ground"},
			{Terminal: "OUT", Node: "port_signal_out"}, {Terminal: "V_PLUS", Node: "port_vcc"}, {Terminal: "V_MINUS", Node: "port_ground"},
		}}
	}
	pull := 10_000.0
	graph.Instances = []GraphInstance{
		comparator("compare_a"), comparator("compare_b"),
		{ID: "pull", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: &pull, Terminals: []TerminalConnection{{Terminal: "A", Node: "port_signal_out"}, {Terminal: "B", Node: "port_vcc"}}},
	}
	return graph
}

func causalV19FloatingBufferGraph(t *testing.T, requirement Requirement) CandidateGraph {
	t.Helper()
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph: %#v", issues)
	}
	link := 10_000.0
	graph.Instances = []GraphInstance{
		{ID: "buffer", PrimitiveKey: "buffer", Kind: "logic_buffer", Terminals: []TerminalConnection{
			{Terminal: "IN", Node: "port_signal_in"}, {Terminal: "OUT", Node: "port_signal_out"},
			{Terminal: "VCC", Node: "port_vcc"}, {Terminal: "GND", Node: "port_vcc"},
		}},
		{ID: "reference_link", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: &link, Terminals: []TerminalConnection{{Terminal: "A", Node: "port_ground"}, {Terminal: "B", Node: "port_vcc"}}},
	}
	return graph
}

func causalV19Inventory(t *testing.T) PrimitiveInventory {
	t.Helper()
	model := func(id string) PrimitiveModelContract {
		return PrimitiveModelContract{
			ModelID: id, Family: "test", AllowedAnalyses: []string{
				"ac_sweep", "dc_operating_point", "dc_sweep", "distortion", "noise", "stability", "startup", "thermal", "transient",
			},
			ProvenanceSource: "https://example.invalid/reviewed", ProvenanceRevision: "reviewed-v1", ProvenanceSHA256: strings.Repeat("d", 64),
		}
	}
	terminal := func(id, electrical string) PrimitiveTerminal {
		return PrimitiveTerminal{Terminal: id, Function: id, SymbolID: "symbol", SymbolPin: id, Pad: id, Electrical: electrical, Required: true}
	}
	voltage, current := 10.0, .1
	resistanceMin, resistanceNominal, resistanceMax := 1.0, 10_000.0, 1_000_000.0
	physical := func(key, kind, family, modelID string, terminals []PrimitiveTerminal) PrimitiveCandidate {
		return PrimitiveCandidate{
			Key: key, CatalogID: "catalog." + key, VariantID: "variant", Kind: kind, Family: family,
			Evidence: "verified", SymbolIDs: []string{"symbol"}, FootprintID: "footprint", PackageType: "test",
			Terminals: terminals, Models: []PrimitiveModelContract{model(modelID)},
			Ratings: []PrimitiveBound{{Kind: "working_voltage", Unit: "V", Maximum: &voltage}, {Kind: "output_current", Unit: "A", Maximum: &current}},
		}
	}
	inventory := PrimitiveInventory{
		Schema: PrimitiveInventorySchema, Version: PrimitiveInventoryVersion,
		CatalogHash: strings.Repeat("a", 64), ModelRegistryHash: strings.Repeat("b", 64), PrimitiveRegistry: primitiveRegistryHash(),
		Primitives: []PrimitiveCandidate{
			physical("opamp", "opamp", "opamp", simmodel.PrimitiveOpAmpV1, []PrimitiveTerminal{
				terminal("IN_PLUS", "input"), terminal("IN_MINUS", "input"), terminal("OUT", "output"), terminal("V_PLUS", "power_in"), terminal("V_MINUS", "power_in"),
			}),
			physical("comparator", "comparator", "comparator", simmodel.PrimitiveComparatorOpenCollectorV1, []PrimitiveTerminal{
				terminal("IN_PLUS", "input"), terminal("IN_MINUS", "input"), terminal("OUT", "open_collector"), terminal("V_PLUS", "power_in"), terminal("V_MINUS", "power_in"),
			}),
			physical("buffer", "logic_buffer", "logic_buffer", simmodel.PrimitiveCMOSBufferV1, []PrimitiveTerminal{
				terminal("IN", "input"), terminal("OUT", "output"), terminal("VCC", "power_in"), terminal("GND", "power_in"),
			}),
			physical("resistor", "resistor", "resistor", simmodel.PrimitiveResistorV1, []PrimitiveTerminal{terminal("A", "passive"), terminal("B", "passive")}),
		},
		Stats: InventoryStats{CatalogRecords: 4, PhysicalVariants: 4, PrimitiveCandidates: 4, PrimitiveModelClaims: 4},
	}
	for index := range inventory.Primitives {
		if inventory.Primitives[index].Key == "resistor" {
			inventory.Primitives[index].ValueDomain = &PrimitiveValueDomain{Kind: "resistance", Unit: "ohm", Minimum: &resistanceMin, Nominal: &resistanceNominal, Maximum: &resistanceMax}
		}
	}
	causalV19SealInventory(t, &inventory)
	return inventory
}

func causalV19CloneInventory(inventory PrimitiveInventory) PrimitiveInventory {
	result := inventory
	result.Primitives = make([]PrimitiveCandidate, len(inventory.Primitives))
	for index, primitive := range inventory.Primitives {
		result.Primitives[index] = primitive
		result.Primitives[index].SymbolIDs = slices.Clone(primitive.SymbolIDs)
		result.Primitives[index].Terminals = slices.Clone(primitive.Terminals)
		result.Primitives[index].Models = make([]PrimitiveModelContract, len(primitive.Models))
		for modelIndex, model := range primitive.Models {
			result.Primitives[index].Models[modelIndex] = model
			result.Primitives[index].Models[modelIndex].AllowedAnalyses = slices.Clone(model.AllowedAnalyses)
		}
		result.Primitives[index].Ratings = slices.Clone(primitive.Ratings)
		if primitive.ValueDomain != nil {
			value := *primitive.ValueDomain
			result.Primitives[index].ValueDomain = &value
		}
	}
	return result
}

func causalV19Primitive(inventory *PrimitiveInventory, key string) PrimitiveCandidate {
	for _, primitive := range inventory.Primitives {
		if primitive.Key == key {
			return primitive
		}
	}
	return PrimitiveCandidate{}
}

func causalV19ReplacePrimitive(inventory *PrimitiveInventory, replacement PrimitiveCandidate) {
	for index := range inventory.Primitives {
		if inventory.Primitives[index].Key == replacement.Key {
			inventory.Primitives[index] = replacement
			return
		}
	}
}

func causalV19SealInventory(t *testing.T, inventory *PrimitiveInventory) {
	t.Helper()
	inventory.Hash = ""
	hash, err := primitiveInventoryHash(*inventory)
	if err != nil {
		t.Fatal(err)
	}
	inventory.Hash = hash
}

func causalV19RequireIssue(t *testing.T, issues []reports.Issue, message string) {
	t.Helper()
	for _, issue := range issues {
		if strings.Contains(issue.Message, message) {
			return
		}
	}
	t.Fatalf("missing issue containing %q: %#v", message, issues)
}

func causalV19Float(value float64) *float64 { return &value }
