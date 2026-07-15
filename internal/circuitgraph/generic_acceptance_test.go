package circuitgraph

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/reports"
)

// TestGenericCompositionAcceptanceCorpus exercises catalog-resolved graph
// compositions without using a project name or fixture path as a planner input.
// The corpus is deliberately small and offline; KiCad CLI promotion remains an
// environment-gated layer above these deterministic prerequisites.
func TestGenericCompositionAcceptanceCorpus(t *testing.T) {
	for _, name := range []string{"rc_filter.json", "transistor_switch.json"} {
		t.Run(name, func(t *testing.T) {
			document := loadGraphExample(t, name)
			resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "generic-acceptance"})
			resolved, issues := resolver.Resolve(context.Background(), document)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("catalog resolution issues = %#v", issues)
			}
			request, issues := ToDesignRequest(resolved)
			if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
				t.Fatalf("design request issues = %#v request=%#v", issues, request)
			}
			index := schematicTestLibraryIndex(resolved)
			firstPlacement := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{LibraryIndex: &index})
			secondPlacement := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{LibraryIndex: &index})
			if firstPlacement.Stage.Status == designworkflow.StageStatusBlocked || !reflect.DeepEqual(firstPlacement.Result.Placements, secondPlacement.Result.Placements) {
				t.Fatalf("placement is not deterministic: first=%#v second=%#v", firstPlacement, secondPlacement)
			}

			request.Validation.Acceptance = designworkflow.AcceptanceDraft
			request.Validation.RequireERC = false
			request.Validation.RequireDRC = false
			output := filepath.Join(t.TempDir(), "project")
			workflow := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
			for _, stage := range []designworkflow.StageName{
				designworkflow.StageSchematic,
				designworkflow.StageSchematicElectrical,
				designworkflow.StagePlacement,
				designworkflow.StageRouting,
				designworkflow.StageProjectWrite,
				designworkflow.StageWriterCorrect,
			} {
				result := graphWorkflowStage(workflow, stage)
				if result == nil || result.Status == designworkflow.StageStatusBlocked || result.Status == designworkflow.StageStatusSkipped {
					t.Fatalf("%s stage = %#v; workflow issues = %#v", stage, result, designworkflow.WorkflowIssues(workflow))
				}
			}
			written, err := design.ReadProjectDirectory(output)
			if err != nil {
				t.Fatal(err)
			}
			if written.Schematic == nil || written.PCB == nil || len(written.PCB.Tracks) == 0 {
				t.Fatalf("written project lacks readable schematic, PCB, or routed copper: %#v", written)
			}
			secondOutput := filepath.Join(t.TempDir(), "project")
			secondWorkflow := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: secondOutput, Overwrite: true, LibraryIndex: &index})
			if stage := graphWorkflowStage(secondWorkflow, designworkflow.StageWriterCorrect); stage == nil || stage.Status == designworkflow.StageStatusBlocked || stage.Status == designworkflow.StageStatusSkipped {
				t.Fatalf("second writer correctness stage = %#v", stage)
			}
			for _, suffix := range []string{".kicad_sch", ".kicad_pcb"} {
				first, err := os.ReadFile(filepath.Join(output, request.Name+suffix))
				if err != nil {
					t.Fatal(err)
				}
				second, err := os.ReadFile(filepath.Join(secondOutput, request.Name+suffix))
				if err != nil {
					t.Fatal(err)
				}
				firstNormalized := roundtrip.NormalizeBytes(first)
				secondNormalized := roundtrip.NormalizeBytes(second)
				if firstNormalized != secondNormalized {
					t.Fatalf("normalized %s differs across repeated generic writes at byte %d", suffix, firstDifferenceOffset(firstNormalized, secondNormalized))
				}
			}
		})
	}
}

func firstDifferenceOffset(first, second string) int {
	limit := len(first)
	if len(second) < limit {
		limit = len(second)
	}
	for index := 0; index < limit; index++ {
		if first[index] != second[index] {
			return index
		}
	}
	return limit
}

func TestGenericCompositionAcceptanceCorpusFailsBeforeProjectWrite(t *testing.T) {
	document := loadGraphExample(t, "unsupported_unknown_component.json")
	output := filepath.Join(t.TempDir(), "must-not-exist")
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "generic-acceptance"}).Resolve(context.Background(), document)
	if !reports.HasBlockingIssue(issues) || resolved.ResolutionHash != "" {
		t.Fatalf("unsupported graph must fail closed: resolved=%#v issues=%#v", resolved, issues)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unsupported graph wrote project output: stat err=%v", err)
	}
}
