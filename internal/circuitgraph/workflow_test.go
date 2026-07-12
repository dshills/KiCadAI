package circuitgraph

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/design"
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
	index := schematicTestLibraryIndex(resolved)
	output := filepath.Join(t.TempDir(), request.Name)
	result := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{
		OutputDir: output, Overwrite: true, SkipRouting: true, LibraryIndex: &index,
	})
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

func graphWorkflowStage(result designworkflow.WorkflowResult, name designworkflow.StageName) *designworkflow.StageResult {
	for index := range result.Stages {
		if result.Stages[index].Name == name {
			return &result.Stages[index]
		}
	}
	return nil
}
