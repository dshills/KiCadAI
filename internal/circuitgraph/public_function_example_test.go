package circuitgraph

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/design"
	"kicadai/internal/reports"
)

func TestPublicFunctionLevelExampleCreatesOffline(t *testing.T) {
	document := loadPublicFunctionExample(t)
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "public-function-example"})
	resolved, issues := resolver.Resolve(context.Background(), document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolution issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
		t.Fatalf("design request issues = %#v", issues)
	}
	index := schematicTestLibraryIndex(resolved)
	request.Validation.Acceptance = designworkflow.AcceptanceDraft
	request.Validation.RequireERC = false
	request.Validation.RequireDRC = false
	output := filepath.Join(t.TempDir(), "project")
	result := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
	for _, stage := range []designworkflow.StageName{
		designworkflow.StageSchematic,
		designworkflow.StageSchematicElectrical,
		designworkflow.StagePlacement,
		designworkflow.StageRouting,
		designworkflow.StageProjectWrite,
		designworkflow.StageWriterCorrect,
	} {
		stageResult := graphWorkflowStage(result, stage)
		if stageResult == nil || stageResult.Status == designworkflow.StageStatusBlocked || stageResult.Status == designworkflow.StageStatusSkipped {
			t.Fatalf("%s stage = %#v; workflow issues = %#v", stage, stageResult, designworkflow.WorkflowIssues(result))
		}
	}
	written, err := design.ReadProjectDirectory(output)
	if err != nil {
		t.Fatal(err)
	}
	if written.Schematic == nil || written.PCB == nil || len(written.PCB.Tracks) == 0 {
		t.Fatal("public example lacks readable schematic, PCB, or routed copper")
	}
}

func TestPublicFunctionLevelExampleMatchesPromotedFixture(t *testing.T) {
	public := Normalize(loadPublicFunctionExample(t))
	contents, err := os.ReadFile(filepath.Join(functionCorpusRoot(t), "npn_low_side_status_driver.json"))
	if err != nil {
		t.Fatal(err)
	}
	promoted, issues := DecodeStrict(strings.NewReader(string(contents)))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("promoted fixture decode issues = %#v", issues)
	}
	promoted = Normalize(promoted)
	public.Project = promoted.Project
	if !reflect.DeepEqual(public, promoted) {
		t.Fatal("public example topology drifted from its promoted function-level fixture")
	}
}

func TestPublicFunctionLevelExampleOptionalKiCadPromotion(t *testing.T) {
	TestPublicFunctionLevelExampleMatchesPromotedFixture(t)
	runFunctionLevelCorpusKiCadPromotion(t, map[string]bool{"npn_low_side_status_driver": true})
}

func loadPublicFunctionExample(t *testing.T) Document {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve public example path")
	}
	path := filepath.Join(filepath.Dir(sourcePath), "..", "..", "examples", "circuit-graph", "function_low_side_status_driver.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document, issues := DecodeStrict(strings.NewReader(string(contents)))
	if reports.HasBlockingIssue(issues) || document.Synthesis == nil {
		t.Fatalf("public example decode issues = %#v", issues)
	}
	return document
}
