package compositionlowering

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
)

func TestHeldOutCapabilityAdjacentFamilyControlsOptionalKiCad(t *testing.T) {
	if os.Getenv("KICADAI_HELD_OUT_ORDER_CHECK") != "1" {
		t.Skip("set KICADAI_HELD_OUT_ORDER_CHECK=1 to run the ordered KiCad control regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cli := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	footprintsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvFootprintsRoot))
	if cli == "" || symbolsRoot == "" || footprintsRoot == "" {
		t.Fatal("KiCad CLI, symbol root, and footprint root are required")
	}
	index, _ := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{
		SymbolsRoot: symbolsRoot, FootprintsRoot: footprintsRoot,
	}, libraryresolver.LoadOptions{})
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(registryIssues) {
		t.Fatalf("catalog registry issues: %#v", registryIssues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance diagnostics: %#v", provenanceDiagnostics)
	}
	modelRegistryHash, err := modelprovenance.Hash(provenance)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "architecturesearch", "testdata", "held_out_capability_expansion_corpus")
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest heldOutCapabilityEvaluationManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	byID := map[string]heldOutCapabilityEvaluationCase{}
	for _, entry := range manifest.Cases {
		byID[entry.ID] = entry
	}
	for _, id := range []string{
		"mcu_constant_current_peripheral",
		"mcu_regulated_sensor_controller",
		"sensor_constant_current_excitation",
		"sensor_low_noise_decision_chain",
	} {
		entry, ok := byID[id]
		if !ok {
			t.Fatalf("missing held-out case %s", id)
		}
		result := evaluateHeldOutCapabilityCase(
			t, ctx, root, entry, manifest.Stages, registry, resolver,
			provenance, modelRegistryHash, index, cli,
		)
		if result.Status != "pass" {
			t.Fatalf("%s ordered result = %#v", id, result)
		}
	}
}
