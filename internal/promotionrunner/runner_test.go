package promotionrunner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/creationevidence"
	"kicadai/internal/designworkflow"
	"kicadai/internal/promotiontoolchain"
)

func TestLoadMatrixRejectsUnknownLaneAndUnsafeFixture(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "request.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := testMatrixJSON("intent", "request.json")
	path := filepath.Join(root, "matrix.json")
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := LoadMatrix(path, root)
	if err != nil {
		t.Fatal(err)
	}
	if document.SHA256 == "" || LaneRegistrySHA256() == "" {
		t.Fatal("matrix or lane registry identity is empty")
	}
	if err := os.WriteFile(path, []byte(testMatrixJSON("invented", "request.json")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMatrix(path, root); err == nil || !strings.Contains(err.Error(), "unsupported promotion lane") {
		t.Fatalf("expected lane error, got %v", err)
	}
	if err := os.WriteFile(path, []byte(testMatrixJSON("intent", "../request.json")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMatrix(path, root); err == nil || !strings.Contains(err.Error(), "unsafe fixture") {
		t.Fatalf("expected path error, got %v", err)
	}
	unsafeID := strings.Replace(testMatrixJSON("intent", "request.json"), `"id":"case"`, `"id":"../../escape"`, 1)
	if err := os.WriteFile(path, []byte(unsafeID), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMatrix(path, root); err == nil || !strings.Contains(err.Error(), "invalid or duplicate id") {
		t.Fatalf("expected id error, got %v", err)
	}
}

func TestRunExecutesEveryScenarioTwiceAndRequiresPromotionGates(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("fake promotion CLI requires a POSIX shell")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "request.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(root, "matrix.json")
	if err := os.WriteFile(matrixPath, []byte(testMatrixJSON("intent", "request.json")), 0o600); err != nil {
		t.Fatal(err)
	}
	matrix, err := LoadMatrix(matrixPath, root)
	if err != nil {
		t.Fatal(err)
	}
	promotion := validPromotionDocument(t)
	script := fakePromotionCLI(t, promotion)
	output := filepath.Join(root, "output")
	results, err := Run(context.Background(), matrix, promotiontoolchain.Evidence{
		KiCadVersion: "10.0.3", KiCadCLI: "/locked/kicad-cli",
		SymbolsRoot: "/locked/symbols", FootprintsRoot: "/locked/footprints",
		SymbolTable: "/locked/template/sym-lib-table", FootprintTable: "/locked/template/fp-lib-table",
	}, Options{RepositoryRoot: root, KiCadAI: script, OutputRoot: output})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Run != 1 || results[1].Run != 2 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Project == results[1].Project {
		t.Fatal("runs were not isolated")
	}
	if _, err := Run(context.Background(), matrix, promotiontoolchain.Evidence{}, Options{
		RepositoryRoot: root, KiCadAI: script, OutputRoot: output,
	}); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected reused-output failure, got %v", err)
	}
}

func TestRunReturnsCompletedPairWhenComparisonFails(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("fake promotion CLI requires a POSIX shell")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "request.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(root, "matrix.json")
	if err := os.WriteFile(matrixPath, []byte(testMatrixJSONWithIDs("intent", "request.json", "case", "second")), 0o600); err != nil {
		t.Fatal(err)
	}
	matrix, err := LoadMatrix(matrixPath, root)
	if err != nil {
		t.Fatal(err)
	}
	script := fakeNondeterministicPromotionCLI(t, validPromotionDocument(t))
	output := filepath.Join(root, "output")
	results, err := Run(context.Background(), matrix, promotiontoolchain.Evidence{
		KiCadVersion: "10.0.3", KiCadCLI: "/locked/kicad-cli",
		SymbolsRoot: "/locked/symbols", FootprintsRoot: "/locked/footprints",
		SymbolTable: "/locked/template/sym-lib-table", FootprintTable: "/locked/template/fp-lib-table",
	}, Options{RepositoryRoot: root, KiCadAI: script, OutputRoot: output})
	if err == nil || !strings.Contains(err.Error(), "case:") || !strings.Contains(err.Error(), "second:") {
		t.Fatalf("expected comparison failure, got %v", err)
	}
	if len(results) != 4 || results[1].Comparison == nil || results[1].Comparison.Status != "failed" ||
		results[3].Comparison == nil || results[3].Comparison.Status != "failed" {
		t.Fatalf("missing completed failing pairs: %#v", results)
	}
	if _, statErr := os.Stat(filepath.Join(output, "scenarios", "case", "comparison.json")); statErr != nil {
		t.Fatalf("comparison artifact: %v", statErr)
	}
}

func validPromotionDocument(t *testing.T) []byte {
	t.Helper()
	required := []designworkflow.PromotionReadiness{designworkflow.PromotionReadinessPass}
	report := designworkflow.PromotionReport{
		ID: "test", DeclaredReadiness: designworkflow.PromotionReadinessPass,
		AchievedReadiness: designworkflow.PromotionReadinessPass,
		Status:            designworkflow.PromotionStatusPass, MatchesExpectation: true,
		Gates: []designworkflow.PromotionGate{
			{ID: "connectivity", Status: designworkflow.PromotionGateStatusPass, RequiredFor: required},
			{ID: "kicad_checks", Status: designworkflow.PromotionGateStatusPass, RequiredFor: required},
			{ID: "route_completion", Status: designworkflow.PromotionGateStatusPass, RequiredFor: required},
			{ID: "writer_correctness", Status: designworkflow.PromotionGateStatusPass, RequiredFor: required},
		},
		KiCadVersion: "10.0.3",
	}
	encoded, err := json.Marshal(creationevidence.DesignPromotionDocument{
		SchemaVersion:   creationevidence.DesignPromotionSchema,
		Applicability:   creationevidence.Applicability{Status: "applicable"},
		PromotionReport: report,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func fakePromotionCLI(t *testing.T, promotion []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicadai")
	body := "#!/bin/sh\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output\" ]; then shift; output=\"$1\"; fi\n" +
		"  shift\n" +
		"done\n" +
		"/bin/mkdir -p \"$output/.kicadai\"\n"
	for _, name := range []string{"design-request.json", "transaction.json", "workflow-result.json", "validation-summary.json", "manifest.json"} {
		body += "printf '{}' > \"$output/.kicadai/" + name + "\"\n"
	}
	body += "printf '%s' '" + string(promotion) + "' > \"$output/.kicadai/design-promotion.json\"\n"
	body += "printf '{\"ok\":true}'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeNondeterministicPromotionCLI(t *testing.T, promotion []byte) string {
	t.Helper()
	path := fakePromotionCLI(t, promotion)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := "printf '{\"ok\":true}'\n"
	replacement := "case \"$output\" in\n" +
		"  *run-1*) printf '{\"value\":1}' > \"$output/semantic.json\" ;;\n" +
		"  *run-2*) printf '{\"value\":2}' > \"$output/semantic.json\" ;;\n" +
		"esac\n" + marker
	body := strings.Replace(string(raw), marker, replacement, 1)
	if body == string(raw) {
		t.Fatal("fake CLI marker not found")
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func testMatrixJSON(lane, fixture string) string {
	return testMatrixJSONWithIDs(lane, fixture, "case")
}

func testMatrixJSONWithIDs(lane, fixture string, ids ...string) string {
	scenarios := make([]string, 0, len(ids))
	for _, id := range ids {
		scenarios = append(scenarios, `{
		  "id":"`+id+`","review_equivalent":"test","lane":"`+lane+`","fixture":"`+fixture+`",
		  "board":{"mode":"declared","width_mm":10,"height_mm":10,"layers":2},"expected_status":"pass",
		  "required_artifacts":["design-request.json","transaction.json","workflow-result.json","validation-summary.json","design-promotion.json","manifest.json"],
		  "internal_gates":["routing","connectivity","route_completion","writer_correctness","round_trip","deterministic_repeat"],
		  "optional_kicad_gates":["erc","strict_drc","writer_correctness","round_trip"]
		}`)
	}
	return `{"schema_version":"kicadai.external-review-matrix.v1","scenarios":[` +
		strings.Join(scenarios, ",") + `],"negative_cases":[{"id":"negative"}]}`
}
