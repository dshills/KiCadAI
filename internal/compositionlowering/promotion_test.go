package compositionlowering

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/capabilitygate"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
	"kicadai/internal/writercorrectness"
)

func TestFrozenOpenSetCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotion(t, "open_set_composition_corpus", 5, "KICADAI_OPEN_SET_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestNeutralMCUSynthesisCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "mcu_synthesis_corpus"), 3, "KICADAI_MCU_SYNTHESIS_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestNeutralMCUSynthesisCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed MCU synthesis corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "mcu_synthesis_corpus"), 3, "KICADAI_MCU_SYNTHESIS_ARTIFACT_DIR", cli, index)
}

func TestPowerInterfaceSynthesisCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "power_interface_synthesis_corpus"), 4, "KICADAI_POWER_INTERFACE_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestPowerInterfaceSynthesisCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed power/interface synthesis corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "power_interface_synthesis_corpus"), 4, "KICADAI_POWER_INTERFACE_ARTIFACT_DIR", cli, index)
}

func TestFrozenOpenSetCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed open-set corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotion(t, "open_set_composition_corpus", 5, "KICADAI_OPEN_SET_ARTIFACT_DIR", cli, index)
}

func TestFrozenAdversarialMultiFunctionCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotion(t, "adversarial_multi_function_composition_corpus", 10, "KICADAI_ADVERSARIAL_MULTI_FUNCTION_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestFrozenAdversarialMultiFunctionCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed adversarial multi-function corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotion(t, "adversarial_multi_function_composition_corpus", 10, "KICADAI_ADVERSARIAL_MULTI_FUNCTION_ARTIFACT_DIR", cli, index)
}

func TestFrozenSimulationGroundedCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "simulation_grounded_closed_loop_corpus"), 10, "KICADAI_SIMULATION_GROUNDED_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{}, filepath.Join("..", "architecturesearch", "testdata", "control_behavior_corpus"))
}

func TestFrozenSimulationGroundedCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed simulation-grounded corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "simulation_grounded_closed_loop_corpus"), 10, "KICADAI_SIMULATION_GROUNDED_ARTIFACT_DIR", cli, index, filepath.Join("..", "architecturesearch", "testdata", "control_behavior_corpus"))
}

func TestDynamicElectrothermalControlLoopCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(
		t,
		filepath.Join("..", "architecturesearch", "testdata", "dynamic_electrothermal_control_loop_corpus"),
		6,
		"KICADAI_DYNAMIC_ELECTROTHERMAL_ARTIFACT_DIR",
		"",
		libraryresolver.LibraryIndex{},
	)
}

func TestDynamicElectrothermalControlLoopCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed dynamic electrothermal/control-loop corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(
		t,
		filepath.Join("..", "architecturesearch", "testdata", "dynamic_electrothermal_control_loop_corpus"),
		6,
		"KICADAI_DYNAMIC_ELECTROTHERMAL_ARTIFACT_DIR",
		cli,
		index,
	)
}

func TestOpenWorldCapabilityPromotionCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(
		t,
		filepath.Join("..", "architecturesearch", "testdata", "open_world_capability_promotion"),
		5,
		"KICADAI_OPEN_WORLD_CAPABILITY_ARTIFACT_DIR",
		"",
		libraryresolver.LibraryIndex{},
	)
}

func TestOpenWorldCapabilityPromotionCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed open-world capability corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(
		t,
		filepath.Join("..", "architecturesearch", "testdata", "open_world_capability_promotion"),
		5,
		"KICADAI_OPEN_WORLD_CAPABILITY_ARTIFACT_DIR",
		cli,
		index,
	)
}

func TestHeldOutConstantCurrentCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	corpusRoot, count := heldOutCapabilityFamilyCorpus(t, "constant_current_regulation")
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_CONSTANT_CURRENT_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestHeldOutConstantCurrentCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed constant-current corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	corpusRoot, count := heldOutCapabilityFamilyCorpus(t, "constant_current_regulation")
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_CONSTANT_CURRENT_ARTIFACT_DIR", cli, index)
}

func TestHeldOutPrecisionRectificationCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	corpusRoot, count := heldOutCapabilityFamilyCorpus(t, "precision_rectification")
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_PRECISION_RECTIFICATION_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestHeldOutPrecisionRectificationCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed precision-rectification corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	corpusRoot, count := heldOutCapabilityFamilyCorpus(t, "precision_rectification")
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_PRECISION_RECTIFICATION_ARTIFACT_DIR", cli, index)
}

func TestStandaloneClockGenerationCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "standalone_clock_generation_corpus"), 2, "KICADAI_CLOCK_GENERATION_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestStandaloneClockGenerationCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed standalone clock-generation corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "standalone_clock_generation_corpus"), 2, "KICADAI_CLOCK_GENERATION_ARTIFACT_DIR", cli, index)
}

func TestProtocolAwareBusCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "protocol_aware_bus_corpus"), 4, "KICADAI_PROTOCOL_BUS_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestProtocolAwareBusCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed protocol-aware bus corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "protocol_aware_bus_corpus"), 4, "KICADAI_PROTOCOL_BUS_ARTIFACT_DIR", cli, index)
}

func TestHeldOutClockGenerationCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	corpusRoot, count := heldOutCapabilityFamilyCorpus(t, "clock_generation")
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_CLOCK_GENERATION_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestHeldOutClockGenerationCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed held-out clock-generation corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	corpusRoot, count := heldOutCapabilityFamilyCorpus(t, "clock_generation")
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_CLOCK_GENERATION_ARTIFACT_DIR", cli, index)
}

func TestClockProgrammingSynthesisCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	corpusRoot, count := clockProgrammingSupportedCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_CLOCK_PROGRAMMING_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestClockProgrammingSynthesisCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed clock/programming synthesis corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	corpusRoot, count := clockProgrammingSupportedCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_CLOCK_PROGRAMMING_ARTIFACT_DIR", cli, index)
}

func TestMCUPowerIntegritySynthesisCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	corpusRoot, count := mcuPowerIntegritySupportedCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_MCU_POWER_INTEGRITY_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestMCUPowerIntegritySynthesisCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed MCU power-integrity synthesis corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	corpusRoot, count := mcuPowerIntegritySupportedCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_MCU_POWER_INTEGRITY_ARTIFACT_DIR", cli, index)
}

func TestFrozenBehavioralIntentHeldOutReadyCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	corpusRoot, count := behavioralIntentHeldOutReadyCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_BEHAVIORAL_INTENT_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{}, filepath.Join("..", "architecturesearch", "testdata", "control_behavior_corpus"))
}

func TestFrozenBehavioralIntentHeldOutReadyCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed behavioral-intent corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	corpusRoot, count := behavioralIntentHeldOutReadyCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_BEHAVIORAL_INTENT_ARTIFACT_DIR", cli, index, filepath.Join("..", "architecturesearch", "testdata", "control_behavior_corpus"))
}

func TestHierarchicalMultiDomainCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(
		t,
		filepath.Join("..", "architecturesearch", "testdata", "hierarchical_multi_domain_corpus"),
		6,
		"KICADAI_HIERARCHICAL_MULTI_DOMAIN_ARTIFACT_DIR",
		"",
		libraryresolver.LibraryIndex{},
		filepath.Join("..", "architecturesearch", "testdata", "hierarchical_control_behavior_overlay"),
	)
}

func TestHierarchicalMultiDomainCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed hierarchical multi-domain corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(
		t,
		filepath.Join("..", "architecturesearch", "testdata", "hierarchical_multi_domain_corpus"),
		6,
		"KICADAI_HIERARCHICAL_MULTI_DOMAIN_ARTIFACT_DIR",
		cli,
		index,
		filepath.Join("..", "architecturesearch", "testdata", "hierarchical_control_behavior_overlay"),
	)
}

func clockProgrammingSupportedCorpus(t *testing.T) (string, int) {
	t.Helper()
	type manifestCase struct {
		File           string `json:"file"`
		ExpectedStatus string `json:"expected_status"`
	}
	var manifest struct {
		Cases []manifestCase `json:"cases"`
	}
	sourceRoot := filepath.Join("..", "architecturesearch", "testdata", "clock_programming_synthesis_corpus")
	data, err := os.ReadFile(filepath.Join(sourceRoot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	count := 0
	for _, fixture := range manifest.Cases {
		if fixture.ExpectedStatus != string(architecturesearch.SearchSelected) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(sourceRoot, fixture.File))
		if err != nil {
			t.Fatalf("read clock/programming requirement %s: %v", fixture.File, err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.Base(fixture.File)), body, 0o644); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count == 0 {
		t.Fatal("clock/programming synthesis corpus has no supported fixtures")
	}
	return root, count
}

func mcuPowerIntegritySupportedCorpus(t *testing.T) (string, int) {
	t.Helper()
	type manifestCase struct {
		File           string `json:"file"`
		ExpectedStatus string `json:"expected_status"`
	}
	var manifest struct {
		Cases []manifestCase `json:"cases"`
	}
	sourceRoot := filepath.Join("..", "architecturesearch", "testdata", "mcu_power_integrity_synthesis_corpus")
	data, err := os.ReadFile(filepath.Join(sourceRoot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	count := 0
	for _, fixture := range manifest.Cases {
		if fixture.ExpectedStatus != string(architecturesearch.SearchSelected) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(sourceRoot, fixture.File))
		if err != nil {
			t.Fatalf("read MCU power-integrity requirement %s: %v", fixture.File, err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.Base(fixture.File)), body, 0o644); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count == 0 {
		t.Fatal("MCU power-integrity synthesis corpus has no supported fixtures")
	}
	return root, count
}

func heldOutCapabilityFamilyCorpus(t *testing.T, family string) (string, int) {
	t.Helper()
	type manifestCase struct {
		Family          string `json:"family"`
		RequirementFile string `json:"requirement_file"`
	}
	var manifest struct {
		Cases []manifestCase `json:"cases"`
	}
	sourceRoot := filepath.Join("..", "architecturesearch", "testdata", "held_out_capability_expansion_corpus")
	data, err := os.ReadFile(filepath.Join(sourceRoot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	count := 0
	for _, fixture := range manifest.Cases {
		if fixture.Family != family {
			continue
		}
		body, err := os.ReadFile(filepath.Join(sourceRoot, fixture.RequirementFile))
		if err != nil {
			t.Fatalf("read held-out requirement %s: %v", fixture.RequirementFile, err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.Base(fixture.RequirementFile)), body, 0o644); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count == 0 {
		t.Fatalf("held-out capability family %q has no fixtures", family)
	}
	return root, count
}

func behavioralIntentHeldOutReadyCorpus(t *testing.T) (string, int) {
	t.Helper()
	type manifestCase struct {
		Outcome         string `json:"outcome"`
		RequirementFile string `json:"requirement_file"`
	}
	var manifest struct {
		Cases []manifestCase `json:"cases"`
	}
	manifestPath := filepath.Join("..", "behavioralintent", "testdata", "held_out_corpus", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	unique := map[string]bool{}
	for _, fixture := range manifest.Cases {
		if fixture.Outcome == "ready" {
			unique[fixture.RequirementFile] = true
		}
	}
	paths := make([]string, 0, len(unique))
	for path := range unique {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	root := t.TempDir()
	for _, path := range paths {
		if strings.HasPrefix(path, "testdata/") {
			path = filepath.Join("..", "behavioralintent", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read held-out requirement %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.Base(path)), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, len(paths)
}

func requireLongPromotionTest(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping frozen end-to-end promotion corpus in short mode")
	}
	runPattern := ""
	if run := flag.Lookup("test.run"); run != nil {
		runPattern = run.Value.String()
	}
	// A non-empty -run expression is an explicit request for a focused test
	// lane (the Makefile/CI test-one contract always supplies one). Keep those
	// historical commands working while preventing an unqualified unit-suite
	// invocation from serially executing every multi-minute promotion corpus.
	if longPromotionRequested(os.Getenv("KICADAI_RUN_PROMOTION_CORPORA"), runPattern) {
		return
	}
	t.Skip("set KICADAI_RUN_PROMOTION_CORPORA=1 or use -run to execute frozen end-to-end promotion corpora")
}

func longPromotionRequested(environment, runPattern string) bool {
	return environment == "1" || strings.TrimSpace(runPattern) != ""
}

func TestLongPromotionRequiresExplicitCampaignOrFocusedRun(t *testing.T) {
	for _, test := range []struct {
		environment string
		pattern     string
		want        bool
	}{
		{want: false},
		{environment: "0", want: false},
		{environment: "1", want: true},
		{pattern: "^TestFrozen", want: true},
	} {
		if got := longPromotionRequested(test.environment, test.pattern); got != test.want {
			t.Fatalf("longPromotionRequested(%q, %q) = %t, want %t", test.environment, test.pattern, got, test.want)
		}
	}
}

func TestPromotionShardSelectionIsCompleteAndNonOverlapping(t *testing.T) {
	paths := []string{"00", "01", "02", "03", "04", "05", "06", "07", "08", "09"}
	const shardCount = 3
	seen := make(map[string]int, len(paths))
	for shard := range shardCount {
		spec := fmt.Sprintf("%d/%d", shard, shardCount)
		selected, err := selectPromotionShard(paths, spec)
		if err != nil {
			t.Fatalf("select shard %s: %v", spec, err)
		}
		var want []string
		for index := shard; index < len(paths); index += shardCount {
			want = append(want, paths[index])
		}
		if !slices.Equal(selected, want) {
			t.Fatalf("shard %s = %#v, want %#v", spec, selected, want)
		}
		for _, path := range selected {
			seen[path]++
		}
	}
	for _, path := range paths {
		if seen[path] != 1 {
			t.Fatalf("path %s appeared in %d shards, want exactly one", path, seen[path])
		}
	}
	unsharded, err := selectPromotionShard(paths, "")
	if err != nil || !slices.Equal(unsharded, paths) {
		t.Fatalf("unsharded paths = %#v, %v", unsharded, err)
	}
	spaced, err := selectPromotionShard(paths, " 0 / 3 ")
	if err != nil {
		t.Fatalf("select spaced shard: %v", err)
	}
	if want := []string{"00", "03", "06", "09"}; !slices.Equal(spaced, want) {
		t.Fatalf("spaced shard = %#v, want %#v", spaced, want)
	}
	for _, spec := range []string{"0", "0/3/4", "x/3", "0/x", "-1/3", "3/3", "0/0", "10/11"} {
		if selected, err := selectPromotionShard(paths, spec); err == nil {
			t.Errorf("invalid shard %q selected %#v", spec, selected)
		}
	}
}

func TestPromotionCorpusOverlayReplacesOnlyDeclaredBaselineCase(t *testing.T) {
	base := filepath.Join("..", "architecturesearch", "testdata", "simulation_grounded_closed_loop_corpus")
	overlay := filepath.Join("..", "architecturesearch", "testdata", "control_behavior_corpus")
	paths, expected, err := promotionCorpusPaths(base, overlay)
	if err != nil || len(paths) != 10 || len(expected) != 0 {
		t.Fatalf("overlay paths=%#v expected=%#v err=%v", paths, expected, err)
	}
	for _, path := range paths {
		switch filepath.Base(path) {
		case "current_sense_protection.json":
			if filepath.Dir(path) != overlay {
				t.Fatalf("current-sense overlay = %q %#v", path, expected[path])
			}
		case "mixed_function_control_power.json":
			if filepath.Dir(path) != overlay {
				t.Fatalf("mixed-function overlay = %q %#v", path, expected[path])
			}
		}
	}
}

func TestHierarchicalControlBehaviorOverlayReplacesOnlyLegacyCase(t *testing.T) {
	base := filepath.Join("..", "architecturesearch", "testdata", "hierarchical_multi_domain_corpus")
	overlay := filepath.Join("..", "architecturesearch", "testdata", "hierarchical_control_behavior_overlay")
	paths, expected, err := promotionCorpusPaths(base, overlay)
	if err != nil || len(paths) != 6 || len(expected) != 0 {
		t.Fatalf("overlay paths=%#v expected=%#v err=%v", paths, expected, err)
	}
	for _, path := range paths {
		if filepath.Base(path) == "current_limited_switched_load_system.json" && filepath.Dir(path) != overlay {
			t.Fatalf("current-limited overlay = %q", path)
		}
		if filepath.Base(path) != "current_limited_switched_load_system.json" && filepath.Dir(path) != base {
			t.Fatalf("unexpected hierarchical overlay = %q", path)
		}
	}
}

// selectPromotionShard parses the shard specification once, then partitions
// the complete corpus path list in a single pass before any cases run.
func selectPromotionShard(paths []string, spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return paths, nil
	}
	parts := strings.Split(spec, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("promotion shard %q must use index/count syntax", spec)
	}
	indexText := strings.TrimSpace(parts[0])
	index, err := strconv.Atoi(indexText)
	if err != nil {
		return nil, fmt.Errorf("promotion shard %q has invalid index %q: %w", spec, indexText, err)
	}
	countText := strings.TrimSpace(parts[1])
	count, err := strconv.Atoi(countText)
	if err != nil {
		return nil, fmt.Errorf("promotion shard %q has invalid count %q: %w", spec, countText, err)
	}
	if count <= 0 || index < 0 || index >= count {
		return nil, fmt.Errorf("promotion shard %q must have 0 <= index < count", spec)
	}
	selected := make([]string, 0, (len(paths)+count-1)/count)
	for pathIndex, path := range paths {
		if pathIndex%count == index {
			selected = append(selected, path)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("promotion shard %q selects no cases from a %d-case corpus", spec, len(paths))
	}
	return selected, nil
}

func runFrozenPromotion(t *testing.T, corpusDir string, expectedCount int, artifactEnv string, cli string, installedIndex libraryresolver.LibraryIndex) {
	t.Helper()
	runFrozenPromotionAt(t, filepath.Join("..", "circuitgraph", "testdata", corpusDir), expectedCount, artifactEnv, cli, installedIndex)
}

type promotionExpectedRejection struct {
	Code    reports.Code
	Path    string
	Message string
}

type promotionOverlayManifest struct {
	Fixtures []struct {
		File             string `json:"file"`
		ExpectedStatus   string `json:"expected_status"`
		ExpectedCode     string `json:"expected_code"`
		ExpectedPath     string `json:"expected_path"`
		ExpectedMessage  string `json:"expected_message"`
		PromotionOverlay bool   `json:"promotion_overlay"`
	} `json:"fixtures"`
}

func promotionCorpusPaths(corpusRoot string, overlayRoots ...string) ([]string, map[string]promotionExpectedRejection, error) {
	basePaths, err := filepath.Glob(filepath.Join(corpusRoot, "*.json"))
	if err != nil {
		return nil, nil, err
	}
	pathsByName := make(map[string]string, len(basePaths))
	for _, path := range basePaths {
		name := filepath.Base(path)
		if name != "manifest.json" {
			pathsByName[name] = path
		}
	}
	expected := map[string]promotionExpectedRejection{}
	for _, root := range overlayRoots {
		data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
		if err != nil {
			return nil, nil, err
		}
		var manifest promotionOverlayManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, nil, err
		}
		applied := 0
		for _, fixture := range manifest.Fixtures {
			if !fixture.PromotionOverlay {
				continue
			}
			if _, exists := pathsByName[fixture.File]; !exists {
				continue
			}
			path := filepath.Join(root, fixture.File)
			pathsByName[fixture.File] = path
			if fixture.ExpectedStatus == "reject" || (fixture.ExpectedStatus == "" && fixture.ExpectedCode != "") {
				expected[path] = promotionExpectedRejection{Code: reports.Code(fixture.ExpectedCode), Path: fixture.ExpectedPath, Message: fixture.ExpectedMessage}
			}
			applied++
		}
		if applied == 0 {
			return nil, nil, fmt.Errorf("promotion overlay %q does not replace any baseline requirement", root)
		}
	}
	paths := make([]string, 0, len(pathsByName))
	for _, path := range pathsByName {
		paths = append(paths, path)
	}
	slices.SortFunc(paths, func(left, right string) int { return strings.Compare(filepath.Base(left), filepath.Base(right)) })
	return paths, expected, nil
}

func runFrozenPromotionAt(t *testing.T, corpusRoot string, expectedCount int, artifactEnv string, cli string, installedIndex libraryresolver.LibraryIndex, overlayRoots ...string) {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance diagnostics = %#v", provenanceDiagnostics)
	}
	modelRegistryHash, err := modelprovenance.Hash(provenance)
	if err != nil {
		t.Fatal(err)
	}
	paths, expectedRejections, err := promotionCorpusPaths(corpusRoot, overlayRoots...)
	if err != nil || len(paths) != expectedCount {
		t.Fatalf("corpus paths = %#v, %v", paths, err)
	}
	paths, err = selectPromotionShard(paths, os.Getenv("KICADAI_PROMOTION_SHARD"))
	if err != nil {
		t.Fatalf("select promotion shard: %v", err)
	}
	// go.mod requires Go 1.23, so each range iteration has its own path
	// variable even when the subtest closure captures it.
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			// Each plan already parallelizes its bounded worst-case corner
			// solves. Running whole plans in parallel here multiplies that CPU
			// budget and can starve every nonlinear solve on small runners.
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
			if expected, exists := expectedRejections[path]; exists {
				if len(decodeIssues) != 1 || decodeIssues[0].Code != expected.Code || decodeIssues[0].Path != expected.Path || decodeIssues[0].Message != expected.Message {
					t.Fatalf("precise promotion rejection = %#v, want %#v", decodeIssues, expected)
				}
				return
			}
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
			if search.Status != architecturesearch.SearchSelected {
				t.Fatalf("search status = %s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			assessment, assessmentErr := capabilitygate.AssessArchitecture(requirement, search, false)
			if assessmentErr != nil {
				t.Fatalf("capability assessment: %v", assessmentErr)
			}
			if err := capabilitygate.Validate(assessment); err != nil {
				t.Fatalf("validate capability assessment: %v", err)
			}
			if assessment.Classification != capabilitygate.ClassificationSupported &&
				assessment.Classification != capabilitygate.ClassificationExperimental {
				t.Fatalf("architecture capability = %s fabrication_eligible=%t; gaps=%#v risks=%#v evidence=%#v", assessment.Classification, assessment.FabricationReadyEligible, assessment.Gaps, assessment.Risks, assessment.Evidence)
			}
			var request designworkflow.Request
			var resolved circuitgraph.ResolvedDocument
			if requirement.Version == architecturesearch.VersionV3 || requirement.Version == architecturesearch.VersionV4 || requirement.Version == architecturesearch.VersionV5 {
				promotion, promotionIssues := SynthesizeClosedLoop(context.Background(), requirement, search, ArchitectureSimulationPlanResolver{
					GraphResolver: resolver, ProvenanceRegistry: provenance,
				}, modelRegistryHash, nil, closedloopsynthesis.DefaultPolicy())
				if reports.HasBlockingIssue(promotionIssues) || promotion.Report.Status != "pass" {
					t.Fatalf("closed-loop promotion issues=%#v\n%s\n%s", promotionIssues, closedLoopFailureSummary(promotion.Report), closedLoopResolutionFailureSummary(context.Background(), requirement, search, resolver, provenance))
				}
				request = promotion.Request
				resolved = promotion.Resolved
				if request.ExplicitCircuit == nil || request.ExplicitCircuit.ClosedLoop == nil || request.ExplicitCircuit.ClosedLoop.SelectedCircuitHash != request.ExplicitCircuit.ResolutionHash {
					t.Fatalf("closed-loop request is not bound to selected resolved circuit: %#v", request.ExplicitCircuit)
				}
				if request.ExplicitCircuit.RoutingPolicy != designworkflow.ExplicitRoutingPolicyConstrainedEndpointAccessV1 {
					t.Fatalf("closed-loop synthesized routing policy = %q", request.ExplicitCircuit.RoutingPolicy)
				}
			} else {
				lowered, lowerIssues := Lower(requirement, search)
				if len(lowerIssues) != 0 {
					t.Fatalf("lower issues = %#v", lowerIssues)
				}
				var resolveIssues []reports.Issue
				resolved, resolveIssues = resolver.Resolve(context.Background(), lowered.Document)
				if reports.HasBlockingIssue(resolveIssues) {
					t.Fatalf("resolve issues = %#v", resolveIssues)
				}
				var requestIssues []reports.Issue
				request, requestIssues = circuitgraph.ToDesignRequest(resolved)
				if reports.HasBlockingIssue(requestIssues) {
					t.Fatalf("design request issues = %#v", requestIssues)
				}
			}

			index := installedIndex
			if cli == "" {
				index = openSetSyntheticLibraryIndex(resolved)
				request.Validation.RequireERC = false
				request.Validation.RequireDRC = false
			}
			artifactRoot := t.TempDir()
			if configured := os.Getenv(artifactEnv); configured != "" {
				artifactRoot = filepath.Join(configured, filepath.Base(path))
				if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
					t.Fatal(err)
				}
				requestData, err := json.MarshalIndent(request, "", "  ")
				if err != nil {
					t.Fatalf("marshal captured workflow request: %v", err)
				}
				if err := os.WriteFile(filepath.Join(artifactRoot, "workflow_request.json"), append(requestData, '\n'), 0o644); err != nil {
					t.Fatalf("write captured workflow request: %v", err)
				}
				indexData, err := json.MarshalIndent(index, "", "  ")
				if err != nil {
					t.Fatalf("marshal captured library index: %v", err)
				}
				if err := os.WriteFile(filepath.Join(artifactRoot, "library_index.json"), append(indexData, '\n'), 0o644); err != nil {
					t.Fatalf("write captured library index: %v", err)
				}
			}
			firstDir := filepath.Join(artifactRoot, "first")
			secondDir := filepath.Join(artifactRoot, "second")
			runOpenSetWorkflow(t, request, index, cli, firstDir)
			runOpenSetWorkflow(t, request, index, cli, secondDir)
			for _, suffix := range []string{".kicad_sch", ".kicad_pcb"} {
				firstBytes, err := os.ReadFile(filepath.Join(firstDir, request.Name+suffix))
				if err != nil {
					t.Fatal(err)
				}
				secondBytes, err := os.ReadFile(filepath.Join(secondDir, request.Name+suffix))
				if err != nil {
					t.Fatal(err)
				}
				if roundtrip.NormalizeBytes(firstBytes) != roundtrip.NormalizeBytes(secondBytes) {
					t.Fatalf("normalized %s differs across deterministic replay", suffix)
				}
			}
			project, err := design.ReadProjectDirectory(firstDir)
			if err != nil || project.Schematic == nil || project.PCB == nil || len(project.PCB.Tracks) == 0 {
				t.Fatalf("written project is incomplete: project=%#v err=%v", project, err)
			}
		})
	}
}

func closedLoopFailureSummary(report closedloopsynthesis.Report) string {
	var lines []string
	for _, candidate := range report.Candidates {
		if len(candidate.Attempts) == 0 {
			lines = append(lines, candidate.Fingerprint+" no attempts")
			continue
		}
		attempt := candidate.Attempts[0]
		for _, candidateAttempt := range candidate.Attempts {
			if sameClosedLoopState(candidateAttempt.State, candidate.FinalState) {
				attempt = candidateAttempt
			}
		}
		for _, repair := range candidate.Repairs {
			lines = append(lines, fmt.Sprintf("%s repair %d %s %.12g->%.12g for %s/%s", candidate.Fingerprint, repair.Number, repair.Variable, repair.From, repair.To, repair.Analysis, repair.Metric))
		}
		for _, candidateAttempt := range candidate.Attempts {
			if candidateAttempt.Status == "pass" {
				continue
			}
			values := make([]string, 0, len(candidateAttempt.State.Variables))
			for _, variable := range candidateAttempt.State.Variables {
				values = append(values, fmt.Sprintf("%s=%.6g", variable.ID, variable.Value))
			}
			failures := make([]string, 0)
			for _, assertion := range candidateAttempt.Assertions {
				if !assertion.Pass {
					failures = append(failures, fmt.Sprintf("%s=%.6g", assertion.Metric, assertion.Actual))
				}
			}
			lines = append(lines, fmt.Sprintf("%s attempt %d [%s] failed [%s]", candidate.Fingerprint, candidateAttempt.Number, strings.Join(values, ","), strings.Join(failures, ",")))
			for _, diagnostic := range candidateAttempt.Diagnostics {
				lines = append(lines, fmt.Sprintf("%s attempt %d %s: %s", candidate.Fingerprint, candidateAttempt.Number, diagnostic.Path, diagnostic.Message))
			}
		}
		for _, variable := range attempt.State.Variables {
			lines = append(lines, fmt.Sprintf("%s final variable %s=%.12g", candidate.Fingerprint, variable.ID, variable.Value))
		}
		if attempt.Simulation != nil {
			for planIndex, plan := range attempt.Simulation.Resolution.Plans {
				kinds := make([]string, 0, len(plan.Analyses))
				for _, analysis := range plan.Analyses {
					kinds = append(kinds, analysis.Kind)
					if len(analysis.SourceValueEvents) != 0 {
						lines = append(lines, fmt.Sprintf("%s plan %d analysis %s dt=%.12g source_events=%v", candidate.Fingerprint, planIndex, analysis.ID, analysis.TimeStepS, analysis.SourceValueEvents))
					}
				}
				lines = append(lines, fmt.Sprintf("%s plan %d model=%s analyses=%v", candidate.Fingerprint, planIndex, plan.ModelID, kinds))
			}
		}
		for _, decision := range attempt.ModelDecisions {
			lines = append(lines, fmt.Sprintf("%s model %s/%s %s status=%s analyses=%v reason=%s parameters=%v", candidate.Fingerprint, decision.Component, decision.Family, decision.Claim.ModelID, decision.Status, decision.RequiredAnalyses, decision.Reason, decision.Claim.Parameters))
		}
		for _, diagnostic := range attempt.Diagnostics {
			lines = append(lines, fmt.Sprintf("%s %s: %s", candidate.Fingerprint, diagnostic.Path, diagnostic.Message))
		}
		for _, assertion := range attempt.Assertions {
			if !assertion.Pass {
				lines = append(lines, fmt.Sprintf("%s %s/%s %s actual=%.12g margin=%.12g", candidate.Fingerprint, assertion.RequirementID, assertion.OperatingCase, assertion.Metric, assertion.Actual, assertion.Margin))
				if assertion.Metric == "integrated_output_noise" && attempt.Simulation != nil {
					lines = append(lines, dominantNoiseSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if assertion.Metric == "bandwidth" && attempt.Simulation != nil {
					lines = append(lines, acSweepRangeSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if assertion.Metric == "transimpedance" && attempt.Simulation != nil {
					lines = append(lines, transimpedanceSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if (assertion.Metric == "quiescent_current" || assertion.Metric == "dc_current") && attempt.Simulation != nil {
					lines = append(lines, operatingPointSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if (assertion.Metric == "threshold_voltage" || assertion.Metric == "threshold_current") && attempt.Simulation != nil {
					lines = append(lines, thresholdSweepSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase, assertion.Actual))
				}
				if (assertion.Metric == "protection_response_time" || assertion.Metric == "protection_recovery_time") && attempt.Simulation != nil {
					lines = append(lines, responseTimeSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func closedLoopResolutionFailureSummary(
	ctx context.Context,
	requirement architecturesearch.Requirement,
	search architecturesearch.SearchResult,
	resolver *circuitgraph.Resolver,
	provenance modelprovenance.Registry,
) string {
	retained := []architecturesearch.CandidateResult{}
	if search.Selected != nil {
		retained = append(retained, *search.Selected)
	}
	retained = append(retained, search.Alternatives...)
	slices.SortStableFunc(retained, func(left, right architecturesearch.CandidateResult) int {
		return strings.Compare(left.Fingerprint, right.Fingerprint)
	})
	lines := make([]string, 0, len(retained))
	for _, candidate := range retained {
		resolved, err := (ArchitectureSimulationPlanResolver{
			Requirement: requirement, Search: search, GraphResolver: resolver, ProvenanceRegistry: provenance,
		}).resolveArchitectureCandidate(ctx, closedloopsynthesis.CandidateState{Fingerprint: candidate.Fingerprint})
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s resolution: %v", candidate.Fingerprint, err))
			continue
		}
		harnessSummary := ""
		if harness, harnessErr := operatingHarnessDevices(requirement, resolved.Lowered.Evidence.SemanticBindings, resolved.Resolved.Simulation, simmodel.AnalysisTransient); harnessErr != nil {
			harnessSummary = "error:" + harnessErr.Error()
		} else {
			harnessSummary = closedLoopOperatingHarnessSummary(harness)
		}
		lines = append(lines, fmt.Sprintf(
			"%s synthesis simulation: status=%s model=%s reason=%s resolved_plan=%t claims=%s devices=%s excitations=%s harness=%s",
			candidate.Fingerprint,
			resolved.SynthesisReport.Simulation.Status,
			resolved.SynthesisReport.Simulation.ModelID,
			resolved.SynthesisReport.Simulation.Reason,
			resolved.Resolved.Simulation != nil,
			closedLoopComponentClaimSummary(resolved.Resolved),
			closedLoopResolvedDeviceSummary(resolved.Resolved),
			closedLoopExcitationSummary(resolved.Resolved),
			harnessSummary,
		))
	}
	return strings.Join(lines, "\n")
}

func closedLoopExcitationSummary(resolved circuitgraph.ResolvedDocument) string {
	if resolved.Simulation == nil {
		return ""
	}
	var entries []string
	for _, analysis := range resolved.Simulation.Analyses {
		for _, excitation := range analysis.Excitations {
			entries = append(entries, fmt.Sprintf(
				"%s:%s:dc=%.9g:pulse=%.9g->%.9g",
				analysis.ID, excitation.Component, excitation.DCValue, excitation.PulseInitialValue, excitation.PulseValue,
			))
		}
	}
	slices.Sort(entries)
	return strings.Join(entries, ",")
}

func closedLoopOperatingHarnessSummary(harness []operatingHarnessDevice) string {
	var entries []string
	for _, entry := range harness {
		var connections []string
		for _, connection := range entry.Device.Connections {
			connections = append(connections, connection.Function+"="+connection.Net)
		}
		entries = append(entries, fmt.Sprintf(
			"%s:%s:source=%t:default=%.9g:edge=%t:%v",
			entry.Device.InstanceID, entry.Device.CatalogID, entry.Source, entry.DefaultValue, entry.TransientEdge, connections,
		))
	}
	slices.Sort(entries)
	return strings.Join(entries, ",")
}

func closedLoopResolvedDeviceSummary(resolved circuitgraph.ResolvedDocument) string {
	if resolved.Simulation == nil {
		return ""
	}
	var devices []string
	for _, device := range resolved.Simulation.Devices {
		terminals := make([]string, 0, len(device.Terminals))
		for _, terminal := range device.Terminals {
			terminals = append(terminals, terminal.Terminal+"="+terminal.Net)
		}
		value := ""
		if device.ValueSI != nil {
			value = fmt.Sprintf(":value=%.9g", *device.ValueSI)
		}
		devices = append(devices, fmt.Sprintf("%s:%s%s:%v", device.Component, device.PrimitiveModel, value, terminals))
	}
	slices.Sort(devices)
	return strings.Join(devices, ",")
}

func closedLoopComponentClaimSummary(resolved circuitgraph.ResolvedDocument) string {
	claims := make([]string, 0, len(resolved.Components))
	for _, component := range resolved.Components {
		models := make([]string, 0, len(component.Record.SimulationModels))
		for _, model := range component.Record.SimulationModels {
			models = append(models, model.ModelID)
		}
		if len(models) > 1 || strings.Contains(component.Instance.ID, "current_shunt") {
			claims = append(claims, fmt.Sprintf("%s:%s:%v", component.Instance.ID, component.ComponentID, models))
		}
	}
	slices.Sort(claims)
	return strings.Join(claims, ",")
}

func acSweepRangeSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		assertionIndexes := append([]int(nil), link.Assertions...)
		if len(assertionIndexes) == 0 {
			assertionIndexes = append(assertionIndexes, link.Assertion)
		}
		for _, assertionIndex := range assertionIndexes {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.Kind != simmodel.AnalysisACSweep || len(analysis.Points) == 0 {
					continue
				}
				gains := make([]float64, 0, len(analysis.Points))
				for _, point := range analysis.Points {
					output, outputOK := testAnalysisNodeMagnitude(point, assertion.Node)
					reference, referenceOK := testAnalysisNodeMagnitude(point, assertion.ReferenceNode)
					if !outputOK || !referenceOK || reference <= 0 {
						return "AC sweep nodes are absent from the recorded evidence"
					}
					gains = append(gains, output/reference)
				}
				threshold := gains[0] / math.Sqrt2
				crossing := len(gains) - 1
				for index := 1; index < len(gains); index++ {
					if gains[index-1] >= threshold && gains[index] <= threshold {
						crossing = index
						break
					}
				}
				start, stop := max(0, crossing-2), min(len(gains), crossing+2)
				points := make([]string, 0, stop-start)
				for index := start; index < stop; index++ {
					points = append(points, fmt.Sprintf("%.6gHz=%.6g", analysis.Points[index].FrequencyHz, gains[index]))
				}
				return fmt.Sprintf("AC sweep range %.12g..%.12g Hz (%d points), passband %.6g threshold %.6g crossing [%s]", analysis.Points[0].FrequencyHz, analysis.Points[len(analysis.Points)-1].FrequencyHz, len(analysis.Points), gains[0], threshold, strings.Join(points, ", "))
			}
		}
	}
	return "AC sweep evidence unavailable"
}

func testAnalysisNodeMagnitude(point simmodel.AnalysisPoint, node string) (float64, bool) {
	for _, result := range point.Nodes {
		if result.Node == node {
			return result.Magnitude, true
		}
	}
	return 0, false
}

func responseTimeSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase {
			continue
		}
		sets := append([]closedloopsynthesis.SimulationAssertionSet(nil), link.Evidence...)
		if len(sets) == 0 {
			indices := append([]int(nil), link.Assertions...)
			if len(indices) == 0 {
				indices = []int{link.Assertion}
			}
			sets = []closedloopsynthesis.SimulationAssertionSet{{Plan: link.Plan, Assertions: indices}}
		}
		for _, set := range sets {
			if set.Plan < 0 || set.Plan >= len(plans) || set.Plan >= len(evidence.Reports) {
				continue
			}
			for _, assertionIndex := range set.Assertions {
				if assertionIndex < 0 || assertionIndex >= len(plans[set.Plan].Assertions) {
					continue
				}
				assertion := plans[set.Plan].Assertions[assertionIndex]
				for _, analysis := range evidence.Reports[set.Plan].Analyses {
					if analysis.ID != assertion.AnalysisID || len(analysis.Points) == 0 {
						continue
					}
					targetTimes := []float64{
						math.Max(0, assertion.WindowStartS-2e-4),
						assertion.WindowStartS,
						assertion.WindowStartS + 5e-4,
						assertion.WindowStartS + 1e-3,
						assertion.WindowStartS + 5e-3,
						assertion.WindowStartS + 15e-3,
						assertion.WindowEndS,
					}
					var samples []string
					for _, target := range targetTimes {
						point := analysis.Points[0]
						for _, candidate := range analysis.Points[1:] {
							if math.Abs(candidate.TimeS-target) >= math.Abs(point.TimeS-target) {
								continue
							}
							point = candidate
						}
						value, valueOK := testAnalysisNodeReal(point, assertion.Node)
						reference := 0.0
						referenceOK := true
						if assertion.ReferenceNode != "" {
							reference, referenceOK = testAnalysisNodeReal(point, assertion.ReferenceNode)
						}
						if valueOK && referenceOK {
							samples = append(samples, fmt.Sprintf("%.9gs=%.9g", point.TimeS, value-reference))
						}
					}
					return fmt.Sprintf(
						"response waveform node=%s reference=%s window=%.9g..%.9g fundamental=%.9g samples=[%s]",
						assertion.Node,
						assertion.ReferenceNode,
						assertion.WindowStartS,
						assertion.WindowEndS,
						analysis.FundamentalFrequencyHz,
						strings.Join(samples, ","),
					)
				}
			}
		}
	}
	return "response-time evidence unavailable"
}

func testAnalysisNodeReal(point simmodel.AnalysisPoint, node string) (float64, bool) {
	for _, result := range point.Nodes {
		if result.Node == node {
			return result.Real, true
		}
	}
	return 0, false
}

func sameClosedLoopState(left, right closedloopsynthesis.CandidateState) bool {
	if left.Fingerprint != right.Fingerprint || len(left.Variables) != len(right.Variables) {
		return false
	}
	for index := range left.Variables {
		if left.Variables[index].ID != right.Variables[index].ID || left.Variables[index].Value != right.Variables[index].Value {
			return false
		}
	}
	return true
}

func operatingPointSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		for _, analysis := range evidence.Reports[link.Plan].Analyses {
			if len(analysis.Points) == 0 {
				continue
			}
			point := analysis.Points[len(analysis.Points)-1]
			nodes := make([]string, 0, len(point.Nodes))
			for _, node := range point.Nodes {
				nodes = append(nodes, fmt.Sprintf("%s=%.9g", node.Node, node.Real))
			}
			devices := make([]string, 0, len(point.Devices))
			for _, device := range point.Devices {
				devices = append(devices, fmt.Sprintf("%s:I=%.9g,V=%.9g", device.Component, device.CurrentMagnitudeA, device.VoltageV))
			}
			slices.Sort(nodes)
			slices.Sort(devices)
			return "operating point nodes=[" + strings.Join(nodes, ",") + "] devices=[" + strings.Join(devices, ",") + "]"
		}
	}
	return "operating point evidence unavailable"
}

func transimpedanceSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		indices := append([]int(nil), link.Assertions...)
		if len(indices) == 0 {
			indices = []int{link.Assertion}
		}
		var samples []string
		for _, assertionIndex := range indices {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			reported := math.NaN()
			if assertionIndex < len(evidence.Reports[link.Plan].Assertions) {
				reported = evidence.Reports[link.Plan].Assertions[assertionIndex].Actual
			}
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.ID != assertion.AnalysisID {
					continue
				}
				minimum, maximum := math.Inf(1), math.Inf(-1)
				minimumAt := 0.0
				for _, point := range analysis.Points {
					voltage, current := math.NaN(), math.NaN()
					for _, node := range point.Nodes {
						if node.Node == assertion.Node {
							voltage = node.Real
						}
					}
					for _, device := range point.Devices {
						if device.Component == assertion.Component {
							current = device.CurrentMagnitudeA
						}
					}
					if current > 1e-15 {
						ratio := voltage / current
						if ratio < minimum {
							minimum, minimumAt = ratio, point.SweepValue
						}
						maximum = math.Max(maximum, ratio)
					}
				}
				samples = append(samples, fmt.Sprintf("%s:reported=%.9g,Zmin=%.9g@%.9g,Zmax=%.9g", analysis.ID, reported, minimum, minimumAt, maximum))
			}
		}
		slices.Sort(samples)
		return "transimpedance samples=[" + strings.Join(samples, "; ") + "]"
	}
	return "transimpedance evidence unavailable"
}

func thresholdSweepSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string, actual float64) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		indices := append([]int(nil), link.Assertions...)
		if len(indices) == 0 {
			indices = []int{link.Assertion}
		}
		for _, assertionIndex := range indices {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.ID != assertion.AnalysisID {
					continue
				}
				bestDistance, best := math.Inf(1), simmodel.AnalysisPoint{}
				maximumSweep := simmodel.AnalysisPoint{SweepValue: math.Inf(-1)}
				for _, point := range analysis.Points {
					if distance := math.Abs(point.SweepValue - actual); distance < bestDistance {
						bestDistance, best = distance, point
					}
					if point.Sweep == "forward" && point.SweepValue > maximumSweep.SweepValue {
						maximumSweep = point
					}
				}
				nodes := make([]string, 0, len(best.Nodes))
				for _, node := range best.Nodes {
					nodes = append(nodes, fmt.Sprintf("%s=%.9g", node.Node, node.Real))
				}
				slices.Sort(nodes)
				maximumNodes := make([]string, 0, len(maximumSweep.Nodes))
				for _, node := range maximumSweep.Nodes {
					maximumNodes = append(maximumNodes, fmt.Sprintf("%s=%.9g", node.Node, node.Real))
				}
				slices.Sort(maximumNodes)
				source := "unknown"
				for _, plannedAnalysis := range plans[link.Plan].Analyses {
					if plannedAnalysis.ID == assertion.AnalysisID && plannedAnalysis.DCSweep != nil {
						source = plannedAnalysis.DCSweep.Component
						break
					}
				}
				return fmt.Sprintf("threshold sweep source=%s sample=%.12g nodes=[%s] max_sample=%.12g max_nodes=[%s] neighborhood=[%s]", source, best.SweepValue, strings.Join(nodes, ", "), maximumSweep.SweepValue, strings.Join(maximumNodes, ", "), thresholdSourceNeighborhood(plans[link.Plan], source))
			}
		}
	}
	return "threshold sweep evidence unavailable"
}

func thresholdSourceNeighborhood(plan simmodel.Plan, source string) string {
	nets := map[string]bool{}
	for _, device := range plan.Devices {
		if device.Component != source {
			continue
		}
		for _, terminal := range device.Terminals {
			nets[terminal.Net] = true
		}
	}
	var devices []string
	for _, device := range plan.Devices {
		var terminals []string
		for _, terminal := range device.Terminals {
			if nets[terminal.Net] {
				terminals = append(terminals, terminal.Terminal+"="+terminal.Net)
			}
		}
		if len(terminals) == 0 {
			continue
		}
		slices.Sort(terminals)
		devices = append(devices, fmt.Sprintf("%s:%s{%s}", device.Component, device.PrimitiveModel, strings.Join(terminals, ",")))
	}
	slices.Sort(devices)
	return strings.Join(devices, "; ")
}

func dominantNoiseSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		indices := append([]int(nil), link.Assertions...)
		if len(indices) == 0 {
			indices = []int{link.Assertion}
		}
		for _, assertionIndex := range indices {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.ID != assertion.AnalysisID {
					continue
				}
				maximum, source, frequency := 0.0, "", 0.0
				for _, point := range analysis.Points {
					for _, node := range point.Nodes {
						if node.Node == assertion.Node && node.DominantNoiseDensityVSqrtHz > maximum {
							maximum, source, frequency = node.DominantNoiseDensityVSqrtHz, node.DominantNoiseSource, point.FrequencyHz
						}
					}
				}
				return fmt.Sprintf("noise target=%s dominant=%s density=%.12g V/sqrt(Hz) at %.12g Hz", assertion.Node, source, maximum, frequency)
			}
		}
	}
	return "noise contribution evidence unavailable"
}

func runOpenSetWorkflow(t *testing.T, request designworkflow.Request, index libraryresolver.LibraryIndex, cli string, output string) designworkflow.WorkflowResult {
	t.Helper()
	if os.Getenv("KICADAI_PLACEMENT_DIAGNOSTICS") != "" && request.ExplicitCircuit != nil {
		diagnostic := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{LibraryIndex: &index})
		order := make([]string, 0, len(diagnostic.Request.Components))
		for _, component := range diagnostic.Request.Components {
			order = append(order, component.Ref)
		}
		t.Logf("placement diagnostics: order=%v rules=%#v placements=%#v scoring=%#v", order, diagnostic.Request.ProximityRules, diagnostic.Result.Placements, diagnostic.Result.CandidateScoring)
	}
	indexBefore, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("marshal library index before workflow: %v", err)
	}
	opts := designworkflow.CreateOptions{
		OutputDir: output, Overwrite: true, LibraryIndex: &index,
		Writer: writercorrectness.Options{LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true},
	}
	if cli != "" {
		opts.Validation = designworkflow.ValidationOptions{StrictUnrouted: true, RequireDRC: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "validation")}
		opts.KiCadChecks = designworkflow.KiCadCheckOptions{KiCadCLI: cli, RequireERC: true, RequireDRC: true, EnforceRequirements: true, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "checks")}
		opts.Writer = writercorrectness.Options{RequireKiCadRoundTrip: true, StrictDiffs: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "roundtrip"), LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true}
	}
	result := designworkflow.Create(context.Background(), request, opts)
	indexAfter, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("marshal library index after workflow: %v", err)
	}
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Fatal("design workflow mutated its immutable library index input")
	}
	if os.Getenv("KICADAI_ROUTE_DIAGNOSTICS") != "" {
		t.Logf("routing diagnostics: %#v", openSetWorkflowStage(result, designworkflow.StageRouting))
	}
	for _, stageName := range []designworkflow.StageName{designworkflow.StageSchematic, designworkflow.StageSchematicElectrical, designworkflow.StagePlacement, designworkflow.StageRouting, designworkflow.StageProjectWrite, designworkflow.StageWriterCorrect, designworkflow.StageValidation, designworkflow.StageSimulation} {
		stage := openSetWorkflowStage(result, stageName)
		if stage == nil || stage.Status == designworkflow.StageStatusBlocked || stage.Status == designworkflow.StageStatusSkipped {
			t.Fatalf("%s stage = %#v; workflow issues = %#v", stageName, stage, designworkflow.WorkflowIssues(result))
		}
		if cli != "" && stage.Status != designworkflow.StageStatusOK {
			t.Fatalf("%s stage = %#v, want clean KiCad-backed status; workflow issues = %#v", stageName, stage, designworkflow.WorkflowIssues(result))
		}
	}
	if cli != "" {
		stage := openSetWorkflowStage(result, designworkflow.StageKiCadChecks)
		if stage == nil || stage.Status != designworkflow.StageStatusOK {
			t.Fatalf("KiCad stage = %#v; workflow issues = %#v", stage, designworkflow.WorkflowIssues(result))
		}
	}
	return result
}

func openSetWorkflowStage(result designworkflow.WorkflowResult, name designworkflow.StageName) *designworkflow.StageResult {
	for index := range result.Stages {
		if result.Stages[index].Name == name {
			return &result.Stages[index]
		}
	}
	return nil
}

func openSetSyntheticLibraryIndex(resolved circuitgraph.ResolvedDocument) libraryresolver.LibraryIndex {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{}, Footprints: map[string]libraryresolver.FootprintRecord{}}
	type pinKey struct {
		symbol string
		unit   int
		pin    string
	}
	seenPins := map[pinKey]bool{}
	seenPads := map[string]map[string]bool{}
	for _, component := range resolved.Components {
		for pinIndex, function := range component.Functions {
			key := pinKey{symbol: function.SymbolID, unit: function.Unit, pin: function.SymbolPin}
			if !seenPins[key] {
				record := index.Symbols[function.SymbolID]
				record.LibraryID = function.SymbolID
				record.Pins = append(record.Pins, libraryresolver.SymbolPin{Number: function.SymbolPin, Name: function.Function, Unit: function.Unit, Position: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(float64(pinIndex) * 2.54)}, Orientation: "0"})
				index.Symbols[function.SymbolID] = record
				seenPins[key] = true
			}
			if seenPads[component.FootprintID] == nil {
				seenPads[component.FootprintID] = map[string]bool{}
			}
			if !seenPads[component.FootprintID][function.Pad] {
				record := index.Footprints[component.FootprintID]
				record.FootprintID = component.FootprintID
				record.Pads = append(record.Pads, libraryresolver.FootprintPad{Name: function.Pad})
				index.Footprints[component.FootprintID] = record
				seenPads[component.FootprintID][function.Pad] = true
			}
		}
	}
	return index
}
