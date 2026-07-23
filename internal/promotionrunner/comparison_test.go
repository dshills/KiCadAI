package promotionrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/promotiontoolchain"
)

func TestCompareProjectsNormalizesRunLocalEvidenceAndManifestHashes(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "run-1", "project")
	second := filepath.Join(root, "run-2", "project")
	writeComparisonProject(t, first, "run-1", "stable")
	writeComparisonProject(t, second, "run-2", "stable")
	comparison, err := CompareProjects("case", first, second, root, comparisonToolchain())
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Status != "pass" || comparison.Run1SHA256 != comparison.Run2SHA256 || len(comparison.Files) == 0 {
		t.Fatalf("comparison = %#v", comparison)
	}
	foundManifest := false
	for _, file := range comparison.Files {
		if file.Path == ".kicadai/manifest.json" {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Fatal("normalized manifest was not inventoried")
	}
}

func TestCompareProjectsDetectsSemanticMutation(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "run-1", "project")
	second := filepath.Join(root, "run-2", "project")
	writeComparisonProject(t, first, "run-1", "stable")
	writeComparisonProject(t, second, "run-2", "changed")
	comparison, err := CompareProjects("case", first, second, root, comparisonToolchain())
	if err == nil || comparison.Status != "failed" || len(comparison.Differences) == 0 || len(comparison.Files) == 0 {
		t.Fatalf("comparison = %#v, err = %v", comparison, err)
	}
	foundWorkflow := false
	for _, difference := range comparison.Differences {
		foundWorkflow = foundWorkflow || difference.Path == ".kicadai/workflow-result.json"
	}
	if !foundWorkflow {
		t.Fatalf("unexpected differences: %#v", comparison.Differences)
	}
}

func TestCompareProjectsRejectsSymbolicLinks(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("symbolic-link test requires POSIX semantics")
	}
	root := t.TempDir()
	first := filepath.Join(root, "run-1", "project")
	second := filepath.Join(root, "run-2", "project")
	writeComparisonProject(t, first, "run-1", "stable")
	writeComparisonProject(t, second, "run-2", "stable")
	if err := os.Symlink(filepath.Join(first, "board.kicad_pcb"), filepath.Join(first, "linked.kicad_pcb")); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareProjects("case", first, second, root, comparisonToolchain()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected symbolic-link rejection, got %v", err)
	}
}

func TestCompareProjectsReturnsContextOnInventoryFailure(t *testing.T) {
	root := t.TempDir()
	comparison, err := CompareProjects("context-case", filepath.Join(root, "missing"), root, root, comparisonToolchain())
	if err == nil {
		t.Fatal("expected inventory failure")
	}
	if comparison.Schema != ComparisonSchema || comparison.Scenario != "context-case" || comparison.Status != "failed" {
		t.Fatalf("comparison = %#v", comparison)
	}
}

func TestInventoryFileRejectsOversizedParseRequiredFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxJSONBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := inventoryFile(path, "oversized.json", newNormalizationContext(t.TempDir(), t.TempDir(), comparisonToolchain())); err == nil ||
		!strings.Contains(err.Error(), "normalization limit") {
		t.Fatalf("expected normalization-limit failure, got %v", err)
	}
}

func TestNormalizeJSONRejectsExcessiveNesting(t *testing.T) {
	raw := strings.Repeat("[", maxJSONDepth+2) + "0" + strings.Repeat("]", maxJSONDepth+2)
	if _, err := normalizeJSON([]byte(raw), newNormalizationContext(t.TempDir(), t.TempDir(), comparisonToolchain())); err == nil ||
		!strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("expected nesting-limit failure, got %v", err)
	}
}

func TestNormalizeJSONPreservesLargeIntegerPrecision(t *testing.T) {
	normalized, err := normalizeJSON([]byte(`{"large":9007199254740993}`), newNormalizationContext(t.TempDir(), t.TempDir(), comparisonToolchain()))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(normalized), `{"large":9007199254740993}`; got != want {
		t.Fatalf("normalizeJSON() = %s, want %s", got, want)
	}
}

func TestNormalizeJSONCanonicalizesObjectKeyOrder(t *testing.T) {
	context := newNormalizationContext(t.TempDir(), t.TempDir(), comparisonToolchain())
	first, err := normalizeJSON([]byte(`{"z":1,"nested":{"b":2,"a":1}}`), context)
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeJSON([]byte(`{"nested":{"a":1,"b":2},"z":1}`), context)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || string(first) != `{"nested":{"a":1,"b":2},"z":1}` {
		t.Fatalf("non-canonical JSON: %s != %s", first, second)
	}
}

func TestNormalizeJSONOnlyZeroesNumericDurationTelemetry(t *testing.T) {
	context := newNormalizationContext(t.TempDir(), t.TempDir(), comparisonToolchain())
	normalized, err := normalizeJSON([]byte(`{"duration_ms":42,"structured":{"duration_ms":{"unit":"ms","value":42}}}`), context)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"duration_ms":0,"structured":{"duration_ms":{"unit":"ms","value":42}}}`
	if string(normalized) != want {
		t.Fatalf("normalizeJSON() = %s, want %s", normalized, want)
	}
}

func TestNormalizeStringCanonicalizesWindowsTemporaryCheckPath(t *testing.T) {
	context := newNormalizationContext(`C:\repo\run-1\project`, `C:\repo`, promotiontoolchain.Evidence{})
	got := normalizeString(`Saved DRC Report to C:\Users\Test User\AppData\Local\Temp\kicadai-check-drc-123\drc.json`, context)
	if want := `Saved DRC Report to ${KICAD_CHECK}/drc.json`; got != want {
		t.Fatalf("normalizeString() = %q, want %q", got, want)
	}
	got = normalizeString(`C:\repo\run-1\project\board.kicad_pcb`, context)
	if want := `${PROJECT}/board.kicad_pcb`; got != want {
		t.Fatalf("normalizeString() = %q, want %q", got, want)
	}
	if got, want := normalizeString(`^\d+\w+$`, context), `^\d+\w+$`; got != want {
		t.Fatalf("normalizeString() corrupted non-path literal: %q, want %q", got, want)
	}
}

func TestNormalizeKiCadFilePreservesSemanticAbsolutePaths(t *testing.T) {
	normalized, err := normalizeKiCadFile(
		"board.kicad_pcb", []byte(`(kicad_pcb (property "source" "C:\repo\project\board.kicad_pcb"))`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(normalized), `C:\repo\project\board.kicad_pcb`) {
		t.Fatalf("KiCad semantic path was masked: %s", normalized)
	}
}

func writeComparisonProject(t *testing.T, project, run, semantic string) {
	t.Helper()
	evidence := filepath.Join(project, ".kicadai")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "board.kicad_pcb"), []byte("(kicad_pcb (version 20240108))\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "opaque.bin"), []byte("stable opaque artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkRoot := "/tmp/kicadai-check-drc-123"
	if run == "run-2" {
		checkRoot = "/tmp/kicadai-check-drc-987"
	}
	workflow := map[string]any{
		"schema_version": "kicadai.workflow-result.v1",
		"semantic":       semantic,
		"target_path":    project,
		"duration_ms":    map[string]any{"ignored": false},
		"check": map[string]any{
			"duration_ms": 123, "working_dir": checkRoot + "/project",
			"stdout": "Saved DRC Report to " + checkRoot + "/drc.json",
		},
	}
	if run == "run-2" {
		workflow["check"].(map[string]any)["duration_ms"] = 999
	}
	workflowBytes, err := json.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidence, "workflow-result.json"), workflowBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	promotion := `{"schema_version":"kicadai.design-promotion.v1","generated_at":"2026-01-01T00:00:00Z","path":"` +
		project + `/board.kicad_pcb"}`
	if run == "run-2" {
		promotion = strings.Replace(promotion, "2026-01-01T00:00:00Z", "2026-01-01T00:00:01Z", 1)
	}
	if err := os.WriteFile(filepath.Join(evidence, "design-promotion.json"), []byte(promotion), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"schema_version": "kicadai.manifest.v1",
		"file_hashes": map[string]any{
			".kicadai/workflow-result.json":  "raw-" + run,
			".kicadai/design-promotion.json": "promotion-" + run,
			"board.kicad_pcb":                "board-" + run,
		},
		"artifacts": []any{
			map[string]any{"path": ".kicadai/workflow-result.json", "sha256": "raw-" + run},
		},
		"evidence": []any{
			map[string]any{"path": ".kicadai/design-promotion.json", "sha256": "promotion-" + run},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidence, "manifest.json"), manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

func comparisonToolchain() promotiontoolchain.Evidence {
	return promotiontoolchain.Evidence{
		KiCadCLI: "/locked/kicad-cli", SymbolsRoot: "/locked/symbols",
		FootprintsRoot: "/locked/footprints", SymbolTable: "/locked/sym-lib-table",
		FootprintTable: "/locked/fp-lib-table",
	}
}
