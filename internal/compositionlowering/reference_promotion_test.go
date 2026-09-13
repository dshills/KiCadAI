package compositionlowering

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

func referencePromotionRequirement(t *testing.T, controller bool) architecturesearch.Requirement {
	r := endpointIntegrationRequirement(t, controller)
	for i := range r.Requirements.Domains {
		if r.Requirements.Domains[i].Kind == "supply" {
			r.Requirements.Domains[i].ReferenceDomain = "ground"
		}
	}
	if controller {
		maxLoad := 0.1
		for i := range r.Requirements.OperatingCases {
			for j := range r.Requirements.OperatingCases[i].Conditions {
				condition := &r.Requirements.OperatingCases[i].Conditions[j]
				if condition.Axis == "load_current" {
					condition.Max = &maxLoad
				}
			}
		}
	}
	return architecturesearch.Normalize(r)
}

// Opt-in, real-library native validation. It retains negative stage receipts;
// it does not repair generated artifacts or substitute synthetic libraries.
func TestExplicitReferenceNativePromotion(t *testing.T) {
	root, cli := os.Getenv("KICADAI_REFERENCE_PROMOTION_ARTIFACTS"), os.Getenv("KICADAI_KICAD_CLI")
	if root == "" || cli == "" {
		t.Skip("requires explicit artifact directory and native KiCad CLI")
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal("artifact directory must be new:", err)
	}
	write := func(path string, value any) {
		t.Helper()
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roots, issues := libraryresolver.ResolveRoots()
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	index, issues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	write(filepath.Join(root, "library_inventory_issues.json"), issues)
	// As in the existing native promotion runner, inventory diagnostics for
	// unrelated library entries are retained, while selected entries undergo
	// the complete workflow's library, writer and native validation gates.
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatal("native libraries unavailable; see inventory receipt")
	}
	write(filepath.Join(root, "library_index.json"), index)
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, issues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	provenance, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	modelHash, err := modelprovenance.Hash(provenance)
	if err != nil {
		t.Fatal(err)
	}
	for _, controller := range []bool{false, true} {
		name := "standalone_regulator"
		if controller {
			name = "controller_adc_100ma"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			dir := filepath.Join(root, name)
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			r := referencePromotionRequirement(t, controller)
			write(filepath.Join(dir, "requirement.json"), r)
			if issues := architecturesearch.Validate(r); reports.HasBlockingIssue(issues) {
				t.Fatal(issues)
			}
			search := architecturesearch.Search(ctx, r, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
			write(filepath.Join(dir, "search.json"), search)
			if search.Status != architecturesearch.SearchSelected {
				t.Fatal(search.Issues)
			}
			promotion, issues := SynthesizeClosedLoop(ctx, r, search, ArchitectureSimulationPlanResolver{GraphResolver: resolver, ProvenanceRegistry: provenance}, modelHash, nil, closedloopsynthesis.DefaultPolicy())
			write(filepath.Join(dir, "promotion.json"), promotion)
			write(filepath.Join(dir, "promotion_issues.json"), issues)
			if reports.HasBlockingIssue(issues) || promotion.Report.Status != "pass" {
				t.Fatalf("closed-loop promotion failed: %v", issues)
			}
			write(filepath.Join(dir, "workflow_request.json"), promotion.Request)
			for _, run := range []string{"first", "second"} {
				output := filepath.Join(dir, run)
				opts := designworkflow.CreateOptions{OutputDir: output, Overwrite: false, LibraryIndex: &index,
					Validation:  designworkflow.ValidationOptions{StrictUnrouted: true, RequireDRC: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "validation")},
					KiCadChecks: designworkflow.KiCadCheckOptions{KiCadCLI: cli, RequireERC: true, RequireDRC: true, EnforceRequirements: true, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "checks")},
					Writer:      writercorrectness.Options{RequireKiCadRoundTrip: true, StrictDiffs: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "roundtrip"), LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true},
				}
				result := designworkflow.Create(ctx, promotion.Request, opts)
				write(filepath.Join(dir, run+"_workflow.json"), result)
				failed := false
				for _, name := range []designworkflow.StageName{designworkflow.StageSchematic, designworkflow.StageSchematicElectrical, designworkflow.StagePlacement, designworkflow.StageRouting, designworkflow.StageProjectWrite, designworkflow.StageWriterCorrect, designworkflow.StageValidation, designworkflow.StageSimulation, designworkflow.StageKiCadChecks} {
					stage := openSetWorkflowStage(result, name)
					if stage == nil || stage.Status != designworkflow.StageStatusOK {
						t.Errorf("%s native stage did not pass; see retained receipt", name)
						failed = true
					}
				}
				if failed {
					return
				}
			}
			// File-level replay authentication is performed by the phase verifier
			// over both retained trees, not inferred from successful stage statuses.
		})
	}
}
