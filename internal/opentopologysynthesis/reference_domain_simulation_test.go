package opentopologysynthesis

import "testing"

func TestSimulationMaterializesDeclaredReferenceDomainWithoutReferencePort(t *testing.T) {
	requirement := referenceDomainSimulationRequirement()
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	if got := referenceNodeForDomain(requirement, graph, Observation{Kind: "port", ID: "output"}); got != "domain_ground" {
		t.Fatalf("reference node = %q, want domain_ground", got)
	}
	nodes := simulationNodeEvidence(requirement, graph)
	found := 0
	for _, node := range nodes {
		if node.Name == "domain_ground" && node.Role == "ground" && node.VoltageDomain == "ground" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("reference evidence count = %d, want exactly one: %#v", found, nodes)
	}
	if len(graph.Nodes) != len(requirement.Requirements.Ports)+1 {
		t.Fatal("initial graph did not materialize exactly one declared reference domain")
	}
}

func TestSimulationReferenceDomainPrefersExplicitNodeAndAvoidsCollisions(t *testing.T) {
	requirement := referenceDomainSimulationRequirement()
	requirement.Requirements.Ports = append(requirement.Requirements.Ports, Port{
		ID: "ground_port", Kind: "reference", Direction: "bidirectional", Domain: "ground",
	})
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	if got := referenceNodeForDomain(requirement, graph, Observation{Kind: "port", ID: "output"}); got != "port_ground_port" {
		t.Fatalf("explicit reference node = %q", got)
	}

	for index, node := range graph.Nodes {
		if node.Role == "reference" {
			graph.Nodes = append(graph.Nodes[:index], graph.Nodes[index+1:]...)
			break
		}
	}
	graph.Nodes = append(graph.Nodes, GraphNode{ID: "reference_ground", Scope: "internal", Role: "signal"})
	if got := referenceNodeForDomain(requirement, graph, Observation{Kind: "port", ID: "output"}); got != "reference_ground_001" {
		t.Fatalf("collision-safe reference node = %q", got)
	}
}

func TestSimulationReferenceDomainFollowsObservedPortDomain(t *testing.T) {
	requirement := referenceDomainSimulationRequirement()
	requirement.Requirements.Domains = append(requirement.Requirements.Domains, Domain{ID: "isolated_return", Kind: "reference", Source: "external"})
	requirement.Requirements.Ports[1].Domain = "isolated_return"
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	if got := referenceNodeForDomain(requirement, graph, Observation{Kind: "port", ID: "output"}); got != "domain_isolated_return" {
		t.Fatalf("domain-specific reference node = %q", got)
	}
	if got := referenceNodeForDomain(requirement, graph, Observation{Kind: "circuit", ID: "assembly"}); got != "" {
		t.Fatalf("ambiguous circuit reference = %q, want empty", got)
	}
}

func TestSyntheticReferenceNamesUseOneRequirementWideAllocationOrder(t *testing.T) {
	requirement := referenceDomainSimulationRequirement()
	requirement.Requirements.Domains = append(requirement.Requirements.Domains,
		Domain{ID: "ground_001", Kind: "reference", Source: "external"})
	requirement.Requirements.Ports[1].Domain = "ground_001"
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	filtered := graph.Nodes[:0]
	for _, node := range graph.Nodes {
		if node.Role != "reference" {
			filtered = append(filtered, node)
		}
	}
	graph.Nodes = append(filtered, GraphNode{ID: "reference_ground", Scope: "internal", Role: "signal"})

	const want = "reference_ground_001_001"
	if got := referenceNodeForDomain(requirement, graph, Observation{Kind: "port", ID: "output"}); got != want {
		t.Fatalf("overlapping-domain reference node = %q, want %q", got, want)
	}
	found := false
	for _, node := range simulationNodeEvidence(requirement, graph) {
		if node.VoltageDomain == "ground_001" && node.Role == "ground" {
			found = true
			if node.Name != want {
				t.Fatalf("node evidence reference = %q, want %q", node.Name, want)
			}
		}
	}
	if !found {
		t.Fatal("node evidence omitted overlapping reference domain")
	}
}

func TestInternalReferenceRoleDoesNotSuppressExternalDomainReference(t *testing.T) {
	requirement := referenceDomainSimulationRequirement()
	graph := CandidateGraph{Nodes: []GraphNode{{
		ID: "internal_ground", Scope: "internal", Role: "reference", Domain: "ground",
	}}}
	const want = "reference_ground"
	if got := referenceNodeForDomain(requirement, graph, Observation{Kind: "port", ID: "output"}); got != want {
		t.Fatalf("resolved reference node = %q, want %q", got, want)
	}
	found := false
	for _, node := range simulationNodeEvidence(requirement, graph) {
		if node.Name == want && node.Role == "ground" && node.VoltageDomain == "ground" {
			found = true
		}
	}
	if !found {
		t.Fatal("internal reference role suppressed the external domain reference")
	}
}

func TestRequirementBindingRejectsReferencePortAndDomainDuplicate(t *testing.T) {
	requirement := referenceDomainSimulationRequirement()
	requirement.Requirements.Ports = append(requirement.Requirements.Ports, Port{
		ID: "ground_port", Kind: "reference", Direction: "bidirectional", Domain: "ground",
	})
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	graph.Nodes = append([]GraphNode{{
		ID: "domain_ground", Scope: "external", SemanticKind: "domain",
		SemanticID: "ground", Domain: "ground", Role: "reference",
	}}, graph.Nodes...)
	issues = validateGraphRequirementBinding(graph, requirement)
	if len(issues) != 1 || issues[0].Message != "behavioral reference domain is bound more than once" {
		t.Fatalf("duplicate reference issues = %#v", issues)
	}
}

func referenceDomainSimulationRequirement() Requirement {
	return Requirement{
		Schema: RequirementSchema, Version: RequirementVersion,
		Project: Project{Name: "reference_domain", Title: "Reference domain", Description: "Behavior-only reference-domain test."},
		Requirements: Requirements{
			Domains: []Domain{
				{ID: "ground", Kind: "reference", Source: "external"},
				{ID: "supply", Kind: "supply", Source: "external"},
			},
			Ports: []Port{
				{ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "ground"},
				{ID: "output", Kind: "analog_voltage", Direction: "source", Domain: "ground"},
				{ID: "power", Kind: "power", Direction: "sink", Domain: "supply"},
			},
			OperatingCases: []OperatingCase{{ID: "nominal", Conditions: []OperatingCondition{{Axis: "supply_voltage", Target: "supply", Min: 5, Max: 5, Unit: "V"}}}},
			BehavioralRequirements: []BehavioralAssertion{{
				ID: "output_window", Metric: "output_voltage", Analysis: "dc_operating_point",
				Observation: Observation{Kind: "port", ID: "output"}, Max: referenceDomainFloat(5), Unit: "V", OperatingCases: []string{"nominal"},
			}},
			Constraints: BoardLimits{MaxComponents: 8, MaxWidthMM: 50, MaxHeightMM: 50},
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

func referenceDomainFloat(value float64) *float64 { return &value }
