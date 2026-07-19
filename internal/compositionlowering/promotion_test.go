package compositionlowering

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

func TestFrozenOpenSetCorpusPassesOfflineWorkflow(t *testing.T) {
	runFrozenOpenSetPromotion(t, "", libraryresolver.LibraryIndex{})
}

func TestFrozenOpenSetCorpusOptionalKiCadPromotion(t *testing.T) {
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
	runFrozenOpenSetPromotion(t, cli, index)
}

func runFrozenOpenSetPromotion(t *testing.T, cli string, installedIndex libraryresolver.LibraryIndex) {
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
	paths, err := filepath.Glob(filepath.Join("..", "circuitgraph", "testdata", "open_set_composition_corpus", "*.json"))
	paths = slices.DeleteFunc(paths, func(path string) bool { return filepath.Base(path) == "manifest.json" })
	if err != nil || len(paths) != 5 {
		t.Fatalf("corpus paths = %#v, %v", paths, err)
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: "checked-in"})
			if search.Status != architecturesearch.SearchSelected {
				t.Fatalf("search status = %s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			lowered, lowerIssues := Lower(requirement, search)
			if len(lowerIssues) != 0 {
				t.Fatalf("lower issues = %#v", lowerIssues)
			}
			resolved, resolveIssues := resolver.Resolve(context.Background(), lowered.Document)
			if reports.HasBlockingIssue(resolveIssues) {
				t.Fatalf("resolve issues = %#v", resolveIssues)
			}
			request, requestIssues := circuitgraph.ToDesignRequest(resolved)
			if reports.HasBlockingIssue(requestIssues) {
				t.Fatalf("design request issues = %#v", requestIssues)
			}

			index := installedIndex
			if cli == "" {
				index = openSetSyntheticLibraryIndex(resolved)
				request.Validation.RequireERC = false
				request.Validation.RequireDRC = false
			}
			artifactRoot := t.TempDir()
			if configured := os.Getenv("KICADAI_OPEN_SET_ARTIFACT_DIR"); configured != "" {
				artifactRoot = filepath.Join(configured, filepath.Base(path))
				if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
					t.Fatal(err)
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

func runOpenSetWorkflow(t *testing.T, request designworkflow.Request, index libraryresolver.LibraryIndex, cli string, output string) designworkflow.WorkflowResult {
	t.Helper()
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
	for _, stageName := range []designworkflow.StageName{designworkflow.StageSchematic, designworkflow.StageSchematicElectrical, designworkflow.StagePlacement, designworkflow.StageRouting, designworkflow.StageProjectWrite, designworkflow.StageWriterCorrect, designworkflow.StageValidation} {
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
