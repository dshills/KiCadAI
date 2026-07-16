package circuitgraph

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/design"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

func TestResolvedGraphWritesExplicitSchematicAndPCB(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), loadGraphExample(t, "rc_filter.json"))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	request.Validation.Acceptance = designworkflow.AcceptanceDraft
	request.Validation.RequireERC = false
	request.Validation.RequireDRC = false
	for index := range request.ExplicitCircuit.Nets {
		if request.ExplicitCircuit.Nets[index].Name == "FILTER_IN" {
			request.ExplicitCircuit.Nets[index].Required = true
			request.ExplicitCircuit.Nets[index].WidthMM = 0.55
			request.ExplicitCircuit.Nets[index].ClearanceMM = 0.25
		}
	}
	index := schematicTestLibraryIndex(resolved)
	output := filepath.Join(t.TempDir(), request.Name)
	result := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{
		OutputDir: output, Overwrite: true, LibraryIndex: &index,
	})
	routing := graphWorkflowStage(result, designworkflow.StageRouting)
	if routing == nil || routing.Status == designworkflow.StageStatusBlocked || routing.Status == designworkflow.StageStatusSkipped {
		t.Fatalf("routing stage = %#v; workflow issues = %#v", routing, designworkflow.WorkflowIssues(result))
	}
	write := graphWorkflowStage(result, designworkflow.StageProjectWrite)
	if write == nil || write.Status == designworkflow.StageStatusBlocked {
		t.Fatalf("project write stage = %#v; workflow issues = %#v", write, designworkflow.WorkflowIssues(result))
	}
	writer := graphWorkflowStage(result, designworkflow.StageWriterCorrect)
	if writer == nil || writer.Status == designworkflow.StageStatusBlocked {
		t.Fatalf("writer correctness stage = %#v", writer)
	}
	written, err := design.ReadProjectDirectory(output)
	if err != nil {
		t.Fatal(err)
	}
	if written.Schematic == nil || len(written.Schematic.Symbols) != len(resolved.Components) {
		t.Fatalf("schematic symbols = %#v", written.Schematic)
	}
	if written.PCB == nil || len(written.PCB.Footprints) != len(resolved.Components) {
		t.Fatalf("PCB footprints = %#v", written.PCB)
	}
	if len(written.PCB.Tracks) == 0 {
		t.Fatalf("PCB has no routed copper: %#v", written.PCB)
	}
	foundWidth := false
	for _, track := range written.PCB.Tracks {
		if track.NetName == "FILTER_IN" && track.Width == 550000 {
			foundWidth = true
		}
	}
	if !foundWidth {
		t.Fatalf("FILTER_IN did not preserve 0.55 mm width: %#v", written.PCB.Tracks)
	}
	wantNets := map[string]map[string]string{
		"J1": {"1": "FILTER_IN", "2": "GND"},
		"R1": {"1": "FILTER_IN", "2": "FILTER_OUT"},
		"C1": {"1": "FILTER_OUT", "2": "GND"},
		"J2": {"1": "FILTER_OUT", "2": "GND"},
	}
	for _, footprint := range written.PCB.Footprints {
		for _, pad := range footprint.Pads {
			if expected := wantNets[footprint.Reference][pad.Name]; expected != "" && pad.NetName != expected {
				t.Fatalf("%s pad %s net = %q, want %q", footprint.Reference, pad.Name, pad.NetName, expected)
			}
		}
	}
}

func TestResolvedGraphPlacementIsDeterministicAcrossProjectNames(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), loadGraphExample(t, "rc_filter.json"))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	index := schematicTestLibraryIndex(resolved)
	first := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{LibraryIndex: &index})
	second := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{LibraryIndex: &index})
	renamed := request
	renamed.Name = "renamed_project"
	third := designworkflow.PlaceExplicitCircuit(context.Background(), renamed, designworkflow.PlacementOptions{LibraryIndex: &index})
	if first.Stage.Status == designworkflow.StageStatusBlocked || !reflect.DeepEqual(first.Result.Placements, second.Result.Placements) || !reflect.DeepEqual(first.Result.Placements, third.Result.Placements) {
		t.Fatalf("placements are not deterministic: first=%#v second=%#v renamed=%#v", first.Result.Placements, second.Result.Placements, third.Result.Placements)
	}
}

func TestResolvedGraphWritesRequestedHierarchy(t *testing.T) {
	graph := loadGraphExample(t, "rc_filter.json")
	graph.Schematic.Hierarchy = HierarchyPolicy{Mode: "auto", MaxComponentsPerSheet: 2}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	index := schematicTestLibraryIndex(resolved)
	output := filepath.Join(t.TempDir(), request.Name)
	result := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
	write := graphWorkflowStage(result, designworkflow.StageProjectWrite)
	if write == nil || write.Status == designworkflow.StageStatusBlocked {
		t.Fatalf("project write stage = %#v; workflow issues = %#v", write, designworkflow.WorkflowIssues(result))
	}
	written, err := design.ReadProjectDirectory(output)
	if err != nil {
		t.Fatal(err)
	}
	if written.Schematic == nil || len(written.Schematic.Sheets) < 2 || len(written.SheetFiles) < 2 {
		t.Fatalf("requested hierarchy was not written: root=%#v child_files=%d", written.Schematic, len(written.SheetFiles))
	}
}

func TestGenericUSBCBMP280RoutesWithInstalledLibraries(t *testing.T) {
	symbolsRoot := os.Getenv(libraryresolver.EnvSymbolsRoot)
	footprintsRoot := os.Getenv(libraryresolver.EnvFootprintsRoot)
	if symbolsRoot == "" || footprintsRoot == "" {
		t.Skip("installed KiCad libraries are required")
	}
	index, loadIssues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{SymbolsRoot: symbolsRoot, FootprintsRoot: footprintsRoot}, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty; issues = %#v", loadIssues)
	}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), loadGraphExample(t, "usb_c_bmp280_breakout.json"))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	placed := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{
		LibraryIndex: &index,
		Rules:        placement.Rules{BoardEdgeClearanceMM: 2},
	})
	if placed.Stage.Status == designworkflow.StageStatusBlocked {
		t.Fatalf("placement issues = %#v", placed.Stage.Issues)
	}
	routed := designworkflow.RouteExplicitCircuit(context.Background(), request, placed, designworkflow.RoutingOptions{})
	if routed.Stage.Status == designworkflow.StageStatusBlocked {
		t.Fatalf("routing issues = %#v routes = %#v", routed.Stage.Issues, routed.Result.Routes)
	}
}

func graphWorkflowStage(result designworkflow.WorkflowResult, name designworkflow.StageName) *designworkflow.StageResult {
	for index := range result.Stages {
		if result.Stages[index].Name == name {
			return &result.Stages[index]
		}
	}
	return nil
}
