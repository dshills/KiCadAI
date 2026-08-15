package opentopologysynthesis

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
)

func TestV18ThresholdOnlyWindowRequirementAddsSearchSemantics(t *testing.T) {
	requirement := testV18Case005(t)
	if _, found := topologyWindowBehaviorEnvelope(requirement); found {
		t.Fatal("legacy search unexpectedly accepts threshold-only window semantics")
	}
	searchRequirement := v18ThresholdWindowRequirement(requirement)
	if issues := Validate(searchRequirement); len(issues) != 0 {
		t.Fatalf("V18 search requirement invalid: %#v", issues)
	}
	envelope, found := topologyWindowBehaviorEnvelope(searchRequirement)
	if !found || envelope.input != "measured_in" || envelope.output != "window_out" {
		t.Fatalf("V18 threshold-window envelope = %#v, found=%t", envelope, found)
	}
	originalHash, err := CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	if transformedHash, hashErr := CanonicalHash(searchRequirement); hashErr != nil || transformedHash == originalHash {
		t.Fatalf("V18 search-only semantics did not produce a distinct normalized requirement: hash=%q err=%v", transformedHash, hashErr)
	}
}

func TestV18InputImpedanceGapRejectsOnlyLowValueRailShunts(t *testing.T) {
	minimum := 1_000_000.0
	requirement := Requirement{Requirements: Requirements{Ports: []Port{{
		ID: "signal", Electrical: Electrical{InputImpedanceMinOhm: &minimum},
	}}}}
	resistance := 100_000.0
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "port_signal", Role: "input"},
			{ID: "port_supply", Role: "supply"},
			{ID: "internal_000", Role: "internal"},
		},
		Instances: []GraphInstance{{
			Kind: "resistor", ValueSI: &resistance,
			Terminals: []TerminalConnection{{Terminal: "A", Node: "port_signal"}, {Terminal: "B", Node: "port_supply"}},
		}},
	}
	if gap := v18TopologyInputImpedanceGap(requirement, graph); gap != 1 {
		t.Fatalf("direct 100 kohm input shunt gap = %d, want 1", gap)
	}
	graph.Instances[0].Terminals[1].Node = "internal_000"
	if gap := v18TopologyInputImpedanceGap(requirement, graph); gap != 0 {
		t.Fatalf("series input path gap = %d, want 0", gap)
	}
	resistance = 2_000_000
	graph.Instances[0].Terminals[1].Node = "port_supply"
	if gap := v18TopologyInputImpedanceGap(requirement, graph); gap != 0 {
		t.Fatalf("2 Mohm rail shunt gap = %d, want 0", gap)
	}
}

func TestV18SearchAdvancesLowVoltageMultiOutputThresholdCaseDeterministically(t *testing.T) {
	requirement := testV18Case005(t)
	inventory, _ := testV18SynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 4_000
	policy.MaxGeneratedGraphs = 20_000
	policy.MaxRetainedCandidates = 16
	first := SearchPrimitiveTopologiesV18(context.Background(), requirement, inventory, policy)
	second := SearchPrimitiveTopologiesV18(context.Background(), requirement, inventory, policy)
	if first.Status != TopologySearchCandidates || len(first.Candidates) == 0 {
		t.Fatalf("V18 search = status=%s issues=%#v rejections=%#v consumption=%#v", first.Status, first.Issues, first.Rejections, first.Consumption)
	}
	if !reflect.DeepEqual(topologyCandidateHashes(first.Candidates), topologyCandidateHashes(second.Candidates)) ||
		!reflect.DeepEqual(first.Consumption, second.Consumption) {
		t.Fatalf("V18 search replay differs: first=%#v second=%#v", first.Consumption, second.Consumption)
	}
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	if first.RequirementHash != requirementHash || first.InventoryHash != inventory.Hash {
		t.Fatalf("V18 search binding = requirement %q inventory %q", first.RequirementHash, first.InventoryHash)
	}
	for _, candidate := range first.Candidates {
		if gap := v18TopologyInputImpedanceGap(requirement, candidate.Graph); gap != 0 {
			t.Fatalf("V18 retained candidate with input-loading gap %d", gap)
		}
		for node, drivers := range testV18OutputDrivers(&candidate.Graph) {
			if len(drivers) > 1 {
				t.Fatalf("V18 retained active-output contention at %s: %v", node, drivers)
			}
		}
	}
	if countV18GraphInstances(first.Candidates[0].Graph, "signal_diode") < 2 {
		t.Fatalf("V18 threshold composition lacks diode isolation: %s", testGraphTopologySummary(first.Candidates[0].Graph))
	}
}

func TestV18EvaluationPassesLowVoltageMultiOutputThresholdCase(t *testing.T) {
	requirement := testV18Case005(t)
	inventory, environment := testV18SynthesisEnvironment(t)
	policy := DefaultPolicy()
	search := SearchPrimitiveTopologiesV18(context.Background(), requirement, inventory, policy)
	if len(search.Candidates) == 0 {
		t.Fatalf("V18 search produced no candidates: %#v", search)
	}
	failureSummaries := []string{}
	firstTopology := ""
	for _, candidate := range search.Candidates {
		if firstTopology == "" {
			firstTopology = testGraphTopologySummary(candidate.Graph)
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		enumeration := EnumerateValueTrialsV18(plan, requirement, candidate.Graph, inventory, 16)
		for _, trial := range enumeration.Trials {
			graph, err := ApplyValueTrial(candidate.Graph, trial, inventory)
			if err != nil {
				continue
			}
			evaluation := EvaluateCandidateV18(
				context.Background(), requirement, graph, nil,
				inventory, environment, policy,
			)
			if len(failureSummaries) == 0 {
				for _, attempt := range evaluation.Attempts {
					if !attempt.AssertionPass {
						actual := 0.0
						if attempt.Actual != nil {
							actual = *attempt.Actual
						}
						failureSummaries = append(failureSummaries, fmt.Sprintf(
							"%s/%s=%g want[%v,%v]", attempt.RequirementID, attempt.Metric,
							actual, attempt.RequiredMin, attempt.RequiredMax,
						))
					}
				}
			}
			if evaluation.Status == SimulationEvaluationPassed {
				requirementHash, err := CanonicalHash(requirement)
				if err != nil {
					t.Fatal(err)
				}
				if evaluation.RequirementHash != requirementHash {
					t.Fatalf("V18 evaluation requirement hash = %q, want %q", evaluation.RequirementHash, requirementHash)
				}
				physical := LowerPassingCandidate(
					context.Background(), requirement, graph, evaluation,
					inventory, environment,
				)
				if physical.Status != PhysicalLoweringReady {
					t.Fatalf("V18 physical lowering = status=%s issues=%#v", physical.Status, physical.Issues)
				}
				return
			}
		}
	}
	t.Fatalf("V18 produced no passing value trial; first failures=%v topology=%s", failureSummaries, firstTopology)
}

func TestSynthesizeV18PromotesLowVoltageMultiOutputThresholdCase(t *testing.T) {
	requirement := testV18Case005(t)
	inventory, environment := testV18SynthesisEnvironment(t)
	first := SynthesizeV18(context.Background(), requirement, inventory, environment, DefaultPolicy())
	second := SynthesizeV18(context.Background(), requirement, inventory, environment, DefaultPolicy())
	if first.Report.Status != StatusPassed || first.Physical == nil || first.Physical.Status != PhysicalLoweringReady {
		t.Fatalf("V18 synthesis = status=%s stop=%s diagnostics=%#v", first.Report.Status, first.Report.StopReason, first.Report.Diagnostics)
	}
	if first.Hash != second.Hash {
		t.Fatalf("V18 synthesis replay hash = %q, want %q", second.Hash, first.Hash)
	}
}

func TestV18LowVoltageMultiOutputThresholdOptionalKiCadPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 for installed-KiCad promotion")
	}
	requirement := testV18Case005(t)
	inventory, environment := testV18SynthesisEnvironment(t)
	run := SynthesizeV18(context.Background(), requirement, inventory, environment, DefaultPolicy())
	if run.Report.Status != StatusPassed {
		t.Fatalf("V18 synthesis = status=%s diagnostics=%#v", run.Report.Status, run.Report.Diagnostics)
	}
	index, _ := libraryresolver.Load(
		context.Background(),
		libraryresolver.LibraryRoots{
			SymbolsRoot:    openTopologyLibraryRoot(t, libraryresolver.EnvSymbolsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols"),
			FootprintsRoot: openTopologyLibraryRoot(t, libraryresolver.EnvFootprintsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints"),
			TemplatesRoot:  strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
		},
		libraryresolver.LoadOptions{},
	)
	outputRoot := t.TempDir()
	if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
		outputRoot = filepath.Join(retained, "v18_low_voltage_multi_output_threshold")
	}
	promotion := PromoteSynthesisRun(context.Background(), run, environment, PhysicalPromotionOptions{
		OutputRoot: outputRoot, KiCadCLI: openTopologyKiCadCLI(t), LibraryIndex: &index,
		Timeout: 3 * time.Minute, KeepArtifacts: true,
	})
	if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical || len(promotion.Runs) != 2 {
		t.Fatalf("V18 installed-KiCad promotion = status=%s replay=%t runs=%d output_drivers=%#v issues=%#v", promotion.Status, promotion.ReplayIdentical, len(promotion.Runs), testV18OutputDrivers(run.SelectedGraph), promotion.Issues)
	}
}

func testV18OutputDrivers(graph *CandidateGraph) map[string][]string {
	result := map[string][]string{}
	if graph == nil {
		return result
	}
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			if (instance.Kind == "opamp" || instance.Kind == "comparator") && terminal.Terminal == "OUT" {
				result[terminal.Node] = append(result[terminal.Node], instance.ID)
			}
		}
	}
	return result
}

func countV18GraphInstances(graph CandidateGraph, kind string) int {
	count := 0
	for _, instance := range graph.Instances {
		if instance.Kind == kind {
			count++
		}
	}
	return count
}

func TestSynthesizeV18DelegatesNoneligibleRequirementByteForByte(t *testing.T) {
	requirement := testV18Case005(t)
	requirement.Requirements.BehavioralRequirements = requirement.Requirements.BehavioralRequirements[:1]
	requirement.Version = 0
	inventory, environment := testV18SynthesisEnvironment(t)
	want := SynthesizeV17(context.Background(), requirement, inventory, environment, DefaultPolicy())
	got := SynthesizeV18(context.Background(), requirement, inventory, environment, DefaultPolicy())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("V18 noneligible delegation differs: got=%#v want=%#v", got, want)
	}
}

func testV18Case005(t *testing.T) Requirement {
	t.Helper()
	data := mustRead(t, filepath.Join("..", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus", "discovery", "v10_case_005.json"))
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("decode V18 public replay case: %#v", issues)
	}
	return requirement
}

func testV18SynthesisEnvironment(t *testing.T) (PrimitiveInventory, SimulationEnvironment) {
	t.Helper()
	catalog, err := components.LoadCatalogV18(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := modelprovenance.LoadV18()
	if len(diagnostics) != 0 {
		t.Fatalf("V18 model provenance diagnostics: %#v", diagnostics)
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	inventory, issues := BuildPrimitiveInventory(catalog, catalogHash, registry)
	if len(issues) != 0 {
		t.Fatalf("V18 primitive inventory issues: %#v", issues)
	}
	return inventory, SimulationEnvironment{Catalog: catalog, CatalogHash: catalogHash, ModelRegistry: registry}
}
