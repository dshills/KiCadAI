package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"slices"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
)

func TestCombinationCountWithinBudget(t *testing.T) {
	for _, test := range []struct {
		name                  string
		values, branches, max int
		want                  bool
	}{
		{name: "single branch", values: 64, branches: 1, max: 64, want: true},
		{name: "repeated selection", values: 3, branches: 2, max: 6, want: true},
		{name: "over budget", values: 100, branches: 2, max: 4_096, want: false},
		{name: "empty catalog", values: 0, branches: 2, max: 4_096, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := combinationCountWithinBudget(test.values, test.branches, test.max); got != test.want {
				t.Fatalf("combinationCountWithinBudget(%d, %d, %d) = %v, want %v", test.values, test.branches, test.max, got, test.want)
			}
		})
	}
}

func TestValueDomainsAreCatalogBoundedAnalyticAndDeterministic(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	inventory := testSearchInventory()
	graph := testPoweredRCGraph(t, requirement, inventory)
	policy := DefaultPolicy()
	policy.MaxValueTrials = 5

	first := BuildValueSearchPlan(requirement, graph, inventory, policy)
	if first.Status != ValuePlanReady || len(first.Domains) != len(graph.Instances) || first.CandidateValues <= len(first.Domains) {
		t.Fatalf("value plan = status=%s domains=%d candidates=%d issues=%#v rejections=%#v", first.Status, len(first.Domains), first.CandidateValues, first.Issues, first.Rejections)
	}
	quantities := map[string]bool{}
	for _, domain := range first.Domains {
		if len(domain.Candidates) == 0 {
			t.Fatalf("empty instance value domain: %#v", domain)
		}
		quantities[domain.Quantity] = true
		for index, candidate := range domain.Candidates {
			if candidate.Rank != index+1 || len(candidate.Hash) != 64 || candidate.PrimitiveKey == "" ||
				len(candidate.ModelEvidenceSHA256s) == 0 {
				t.Fatalf("candidate lacks rank/provenance: %#v", candidate)
			}
			primitive, found := primitiveByKey(inventory, candidate.PrimitiveKey)
			if !found {
				t.Fatalf("candidate primitive %q is outside inventory", candidate.PrimitiveKey)
			}
			if primitive.ValueDomain != nil {
				if candidate.ValueSI == nil || !valueWithinPrimitiveDomain(*candidate.ValueSI, *primitive.ValueDomain) ||
					!candidate.ToleranceProven || candidate.PreferredSeries == "" ||
					candidate.CornerMinimumSI == nil || candidate.CornerMaximumSI == nil {
					t.Fatalf("value candidate is not bounded and corner-complete: %#v", candidate)
				}
			}
		}
	}
	if !quantities["resistance"] || !quantities["capacitance"] {
		t.Fatalf("plan quantities = %#v", quantities)
	}
	if !planHasAnalyticScale(first, "resistance") || !planHasAnalyticScale(first, "capacitance") {
		t.Fatalf("plan lacks resistance/capacitance analytic scales: %#v", first.Domains)
	}

	second := BuildValueSearchPlan(requirement, graph, inventory, policy)
	reordered := inventory
	reordered.Primitives = slices.Clone(inventory.Primitives)
	slices.Reverse(reordered.Primitives)
	third := BuildValueSearchPlan(requirement, graph, reordered, policy)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	thirdJSON, _ := json.Marshal(third)
	if !bytes.Equal(firstJSON, secondJSON) || !bytes.Equal(firstJSON, thirdJSON) {
		t.Fatalf("value-domain ordering is not deterministic:\n%s\n%s\n%s", firstJSON, secondJSON, thirdJSON)
	}

	enumeration := EnumerateValueTrials(first, policy.MaxValueTrials)
	if len(enumeration.Trials) != policy.MaxValueTrials || !enumeration.Exhausted ||
		enumeration.TotalCombinations <= uint64(len(enumeration.Trials)) {
		t.Fatalf("bounded enumeration = %#v", enumeration)
	}
	hashes := map[string]bool{}
	for index, trial := range enumeration.Trials {
		if trial.Number != index+1 || len(trial.Hash) != 64 || hashes[trial.Hash] {
			t.Fatalf("invalid or duplicate trial: %#v", trial)
		}
		hashes[trial.Hash] = true
		applied, err := ApplyValueTrial(graph, trial, inventory)
		if err != nil {
			t.Fatal(err)
		}
		if issues := ValidateCompleteGraph(applied, inventory, GraphLimits{MaxPrimitiveInstances: 8, MaxInternalNodes: 8}); len(issues) != 0 {
			t.Fatalf("applied trial graph issues: %#v", issues)
		}
	}
}

func TestValueDomainsFailClosedOnToleranceRatingAndSemanticBinding(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	inventory := testSearchInventory()
	graph := testPoweredRCGraph(t, requirement, inventory)

	unproven := inventory
	unproven.Primitives = slices.Clone(inventory.Primitives)
	for index := range unproven.Primitives {
		if unproven.Primitives[index].Kind == "capacitor" {
			unproven.Primitives[index].Tolerances = nil
		}
	}
	tolerancePlan := BuildValueSearchPlan(requirement, graph, unproven, DefaultPolicy())
	if tolerancePlan.Status != ValuePlanExhausted || !hasSearchRejection(tolerancePlan.Rejections, "tolerance_unproven") {
		t.Fatalf("unproven tolerance plan = status=%s rejections=%#v issues=%#v", tolerancePlan.Status, tolerancePlan.Rejections, tolerancePlan.Issues)
	}

	underrated := inventory
	underrated.Primitives = slices.Clone(inventory.Primitives)
	for index := range underrated.Primitives {
		if underrated.Primitives[index].Kind == "capacitor" {
			underrated.Primitives[index].Ratings = []PrimitiveBound{{
				Kind: "voltage", Unit: "V", Maximum: graphFloat(5),
			}}
		}
	}
	ratingPlan := BuildValueSearchPlan(requirement, graph, underrated, DefaultPolicy())
	if ratingPlan.Status != ValuePlanExhausted || !hasSearchRejection(ratingPlan.Rejections, "rating_envelope") {
		t.Fatalf("underrated plan = status=%s rejections=%#v issues=%#v", ratingPlan.Status, ratingPlan.Rejections, ratingPlan.Issues)
	}

	misbound := CloneGraph(graph)
	for index := range misbound.Nodes {
		if misbound.Nodes[index].SemanticID == "power" {
			misbound.Nodes[index].SemanticID = "invented"
			break
		}
	}
	bindingPlan := BuildValueSearchPlan(requirement, misbound, inventory, DefaultPolicy())
	if bindingPlan.Status != ValuePlanFailed || len(bindingPlan.Issues) == 0 {
		t.Fatalf("misbound graph plan = status=%s issues=%#v", bindingPlan.Status, bindingPlan.Issues)
	}
}

func TestDefaultCatalogProducesProvenanceCompleteValueDomains(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model-provenance diagnostics: %#v", diagnostics)
	}
	inventory, issues := BuildPrimitiveInventory(
		catalog,
		circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash(),
		registry,
	)
	if len(issues) != 0 {
		t.Fatalf("primitive inventory issues: %#v", issues)
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	resistor, resistorFound := firstValuePrimitive(inventory, requirement, requiredAnalyses, "resistor")
	capacitor, capacitorFound := firstValuePrimitive(inventory, requirement, requiredAnalyses, "capacitor")
	if !resistorFound || !capacitorFound {
		t.Fatalf("default inventory lacks usable resistor/capacitor: resistor=%t capacitor=%t", resistorFound, capacitorFound)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	graph = AddPrimitive(graph, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
		{Terminal: "A", Node: "port_input"},
		{Terminal: "B", Node: "port_output"},
	})
	graph = AddPrimitive(graph, capacitor, seedPrimitiveValue(capacitor), []TerminalConnection{
		{Terminal: "A", Node: "port_output"},
		{Terminal: "B", Node: "port_ground"},
	})
	graph = AddPrimitive(graph, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
		{Terminal: "A", Node: "port_power"},
		{Terminal: "B", Node: "port_ground"},
	})
	plan := BuildValueSearchPlan(requirement, graph, inventory, DefaultPolicy())
	if plan.Status != ValuePlanReady || plan.CandidateValues == 0 {
		t.Fatalf("default-catalog value plan = status=%s candidates=%d issues=%#v rejections=%#v", plan.Status, plan.CandidateValues, plan.Issues, plan.Rejections)
	}
}

func TestOriginalPrimitiveValueDomainKeepsTopologySeedFirst(t *testing.T) {
	candidates := []ComponentValueCandidate{
		{PrimitiveKey: "active.alpha"},
		{PrimitiveKey: "active.original"},
		{PrimitiveKey: "active.zeta"},
	}
	prioritized := prioritizeOriginalCandidate(
		candidates,
		"active.original",
	)
	if prioritized[0].PrimitiveKey != "active.original" ||
		prioritized[1].PrimitiveKey != "active.alpha" ||
		prioritized[2].PrimitiveKey != "active.zeta" {
		t.Fatalf("original primitive priority = %#v", prioritized)
	}
	if candidates[0].PrimitiveKey != "active.alpha" {
		t.Fatal("original primitive priority mutated its input")
	}
}

func TestValueCandidateRankingUsesWorstCaseAnalyticError(t *testing.T) {
	candidates := []ComponentValueCandidate{
		{
			PrimitiveKey:     "loose_exact",
			ValueSI:          graphFloat(10),
			AnalyticPriority: 1,
			RelativeError:    0,
			TolerancePercent: 5,
			ToleranceProven:  true,
		},
		{
			PrimitiveKey:     "precise_near",
			ValueSI:          graphFloat(9.99),
			AnalyticPriority: 1,
			RelativeError:    0.001,
			TolerancePercent: 0.1,
			ToleranceProven:  true,
		},
	}
	slices.SortFunc(candidates, compareComponentValueCandidates)
	if candidates[0].PrimitiveKey != "precise_near" {
		t.Fatalf("first value candidate = %q; want bounded worst-case error", candidates[0].PrimitiveKey)
	}
}

func TestReviewedTransientPrimitiveParticipatesInElectrothermalCircuit(t *testing.T) {
	transientOnly := PrimitiveCandidate{
		Models: []PrimitiveModelContract{{
			AllowedAnalyses: []string{"dc_operating_point", "transient"},
		}},
	}
	if !primitiveCoversAllAnalyses(
		transientOnly,
		map[string]bool{"dc_operating_point": true, "electrothermal": true},
	) {
		t.Fatal("reviewed transient electrical model was excluded from an electrothermal circuit")
	}
	if primitiveCoversAllAnalyses(
		transientOnly,
		map[string]bool{"ac_sweep": true},
	) {
		t.Fatal("analysis compatibility widened beyond the electrothermal circuit workflow")
	}
}

func TestTopologyAnalyticScalesRankBoundedDecisionRatios(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "hysteretic_detector.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	byKind := map[string]PrimitiveCandidate{}
	for _, primitive := range topologyRepresentatives(requirement, inventory) {
		byKind[primitive.Kind] = primitive
	}
	comparator, comparatorFound := byKind["comparator"]
	resistor, resistorFound := byKind["resistor"]
	if !comparatorFound || !resistorFound {
		t.Fatalf("missing decision primitives: comparator=%t resistor=%t", comparatorFound, resistorFound)
	}
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	graph = AddInternalNode(graph, "internal")
	referenceNode := graph.Nodes[len(graph.Nodes)-1].ID
	graph = AddInternalNode(graph, "internal")
	decisionNode := graph.Nodes[len(graph.Nodes)-1].ID
	graph = AddPrimitive(graph, comparator, nil, []TerminalConnection{
		{Terminal: "IN_MINUS", Node: referenceNode},
		{Terminal: "IN_PLUS", Node: decisionNode},
		{Terminal: "OUT", Node: "port_indication"},
		{Terminal: "V_MINUS", Node: "port_ground"},
		{Terminal: "V_PLUS", Node: "port_power"},
	})
	for _, nodes := range [][2]string{
		{referenceNode, "port_ground"},
		{referenceNode, "port_power"},
		{decisionNode, "port_input"},
		{decisionNode, "port_indication"},
	} {
		graph = AddPrimitive(graph, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
			{Terminal: "A", Node: nodes[0]},
			{Terminal: "B", Node: nodes[1]},
		})
	}
	expected := map[string]float64{
		"primitive_001": 14_117.488547561301,
		"primitive_002": 10_000,
		"primitive_003": 10_000,
		"primitive_004": 169_000,
	}
	for _, instance := range graph.Instances[1:] {
		scales := deriveTopologyAnalyticScales(
			requirement,
			graph,
			instance,
			primitiveInventoryByKey(inventory),
		)
		if len(scales) != 1 || scales[0].Priority != 1 ||
			math.Abs(scales[0].ValueSI-expected[instance.ID]) > expected[instance.ID]*1e-12 {
			t.Fatalf("%s topology scales = %#v; want %.12g ohm", instance.ID, scales, expected[instance.ID])
		}
	}
}

func TestTopologyAnalyticScalesRankSupplyInvariantDecisionReference(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "hysteretic_detector.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	search := SearchPrimitiveTopologies(
		context.Background(),
		requirement,
		inventory,
		policy,
	)
	var graph CandidateGraph
	for _, candidate := range search.Candidates {
		if candidate.Score.InternalNodeCount == 4 &&
			len(candidate.Graph.Instances) == 8 {
			graph = candidate.Graph
			break
		}
	}
	if len(graph.Instances) == 0 {
		t.Fatal("search omitted the supply-invariant decision relationship")
	}
	byKey := primitiveInventoryByKey(inventory)
	values := []float64{}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" {
			continue
		}
		scales := deriveTopologyAnalyticScales(
			requirement,
			graph,
			instance,
			byKey,
		)
		if len(scales) != 1 || scales[0].Priority != 1 {
			t.Fatalf("%s invariant-reference scales = %#v", instance.ID, scales)
		}
		values = append(values, scales[0].ValueSI)
	}
	slices.Sort(values)
	expected := []float64{
		10_000,
		10_000,
		10_000,
		13_548.603351955304,
		169_000,
	}
	if len(values) != len(expected) {
		t.Fatalf("invariant-reference scale count = %d; want %d", len(values), len(expected))
	}
	for index := range expected {
		if math.Abs(values[index]-expected[index]) > expected[index]*1e-12 {
			t.Fatalf("invariant-reference scales = %#v; want %#v", values, expected)
		}
	}
}

func TestTopologyAnalyticScalesComposeSplitDecisionFeedbackFromCatalog(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "hysteretic_detector.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	var graph CandidateGraph
	for _, candidate := range search.Candidates {
		if len(candidate.Graph.Instances) == 8 {
			graph = candidate.Graph
			break
		}
	}
	if len(graph.Instances) == 0 {
		t.Fatal("search omitted the absolute-reference hysteretic topology")
	}
	var err error
	graph, err = SubstitutePrimitive(
		graph, inventory, "primitive_007", "resistor.vishay.tnpw0805.90k9.0p1|0805",
	)
	if err != nil {
		t.Fatal(err)
	}
	inserted, found := primitiveByKey(inventory, "resistor.vishay.ac03.0r22.axial|axial_ac03")
	if !found {
		t.Fatal("inventory lacks the deterministic neutral repair resistor")
	}
	graph, err = SplitPrimitiveInSeries(
		graph, inventory, "primitive_007", inserted, seedPrimitiveValue(inserted),
	)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	want := map[string]float64{"primitive_007": 90_900, "primitive_008": 47_000}
	for instanceID, expected := range want {
		instance := graph.Instances[graphInstanceIndex(graph, instanceID)]
		scales := deriveTopologyAnalyticScales(requirement, graph, instance, byKey)
		if len(scales) != 1 || scales[0].Priority != 1 || scales[0].ValueSI != expected {
			t.Fatalf("%s split-feedback scales = %#v; want %.12g ohm", instanceID, scales, expected)
		}
	}
}

func TestCatalogSeriesResistancePairPreservesExistingBranch(t *testing.T) {
	requirement := testMultiBranchAnalogRequirement(t, "precision_low_voltage_rail.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	left := GraphInstance{ID: "left", Kind: "resistor", ValueSI: graphFloat(15_000)}
	right := GraphInstance{ID: "right", Kind: "resistor", ValueSI: graphFloat(0.22)}
	leftValue, rightValue, found := catalogSeriesResistancePairPreservingBranch(
		requirement,
		primitiveInventoryByKey(inventory),
		16_400,
		left,
		right,
	)
	if !found || leftValue != 15_000 || rightValue != 1_000 {
		t.Fatalf("preserving series pair = %.12g + %.12g, found=%t", leftValue, rightValue, found)
	}
}

func TestCatalogSeriesParallelResistanceTripletUsesReviewedFixedValues(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, multiStageOODCorpusRoot()+"/undervoltage_load_permission.json"),
		&requirement,
	)
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	const target = 119_700.0
	byKey := primitiveInventoryByKey(inventory)
	series, parallelA, parallelB, found := catalogSeriesParallelResistanceTriplet(
		requirement,
		byKey,
		target,
	)
	if !found {
		t.Fatal("catalog series-parallel resistance triplet was not found")
	}
	realized := series + 1/(1/parallelA+1/parallelB)
	if math.Abs(realized-target)/target > .01 {
		t.Fatalf(
			"catalog series-parallel resistance triplet = %.12g + (%.12g || %.12g) = %.12g; want within 1%% of %.12g",
			series, parallelA, parallelB, realized, target,
		)
	}
}

func testPoweredRCGraph(t *testing.T, requirement Requirement, inventory PrimitiveInventory) CandidateGraph {
	t.Helper()
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	resistor, found := primitiveByKey(inventory, "resistor|0603")
	if !found {
		t.Fatal("test resistor is missing")
	}
	capacitor, found := primitiveByKey(inventory, "capacitor|0603")
	if !found {
		t.Fatal("test capacitor is missing")
	}
	graph = AddPrimitive(graph, resistor, graphFloat(10_000), []TerminalConnection{
		{Terminal: "A", Node: "port_input"},
		{Terminal: "B", Node: "port_output"},
	})
	graph = AddPrimitive(graph, capacitor, graphFloat(10e-9), []TerminalConnection{
		{Terminal: "A", Node: "port_output"},
		{Terminal: "B", Node: "port_ground"},
	})
	graph = AddPrimitive(graph, resistor, graphFloat(100_000), []TerminalConnection{
		{Terminal: "A", Node: "port_power"},
		{Terminal: "B", Node: "port_ground"},
	})
	if issues := ValidateCompleteGraph(graph, inventory, GraphLimits{MaxPrimitiveInstances: 8, MaxInternalNodes: 8}); len(issues) != 0 {
		t.Fatalf("powered RC graph issues: %#v", issues)
	}
	return graph
}

func planHasAnalyticScale(plan ValueSearchPlan, kind string) bool {
	for _, domain := range plan.Domains {
		for _, scale := range domain.AnalyticScales {
			if scale.Kind == kind {
				return true
			}
		}
	}
	return false
}

func hasSearchRejection(rejections []SearchRejection, code string) bool {
	for _, rejection := range rejections {
		if rejection.Code == code && rejection.Count > 0 {
			return true
		}
	}
	return false
}

func firstValuePrimitive(
	inventory PrimitiveInventory,
	requirement Requirement,
	requiredAnalyses map[string]bool,
	kind string,
) (PrimitiveCandidate, bool) {
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != kind || primitive.ValueDomain == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		if _, proven := primitiveTolerancePercent(primitive, primitive.ValueDomain.Kind); !proven {
			continue
		}
		return primitive, true
	}
	return PrimitiveCandidate{}, false
}

func TestRatingsCoverRequirementRejectsPropagationDelayOutsideDynamicEnvelope(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	maximumRiseTimeS := 5e-9
	minimumFrequencyHz := 50e6
	requirement.Requirements.BehavioralRequirements = []BehavioralAssertion{
		{ID: "edge", Metric: "rise_time", Max: &maximumRiseTimeS, Unit: "s"},
		{ID: "rate", Metric: "oscillation_frequency", Min: &minimumFrequencyHz, Unit: "Hz"},
	}
	maximumDelayUS := 1.0
	primitive := PrimitiveCandidate{Ratings: []PrimitiveBound{{
		Kind:    "propagation_delay",
		Unit:    "us",
		Maximum: &maximumDelayUS,
	}}}
	if ratingsCoverRequirement(requirement, primitive) {
		t.Fatal("one-microsecond primitive covered a five-nanosecond, fifty-megahertz dynamic envelope")
	}

	maximumDelayNS := 2.0
	primitive.Ratings[0].Unit = "ns"
	primitive.Ratings[0].Maximum = &maximumDelayNS
	if !ratingsCoverRequirement(requirement, primitive) {
		t.Fatal("two-nanosecond primitive did not cover a five-nanosecond, fifty-megahertz dynamic envelope")
	}
}
