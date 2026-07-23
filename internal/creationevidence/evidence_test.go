package creationevidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/manifest"
	"kicadai/internal/provenance"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestWriteProducesTypedDeterministicCoreEvidence(t *testing.T) {
	root := t.TempDir()
	writeTestProvenance(t, root)
	bundle := testBundle()
	artifacts, issues := Write(root, bundle)
	if len(issues) != 0 || len(artifacts) != 6 {
		t.Fatalf("write artifacts=%#v issues=%#v", artifacts, issues)
	}

	var request DesignRequestDocument
	readJSON(t, filepath.Join(root, DesignRequestPath), &request)
	if request.SchemaVersion != DesignRequestSchema || request.Name != "demo" {
		t.Fatalf("request document = %#v", request)
	}
	var workflow WorkflowResultDocument
	readJSON(t, filepath.Join(root, WorkflowResultPath), &workflow)
	if workflow.SchemaVersion != WorkflowResultSchema || workflow.Project.Name != "demo" {
		t.Fatalf("workflow document = %#v", workflow)
	}
	var validation ValidationSummary
	readJSON(t, filepath.Join(root, ValidationSummaryPath), &validation)
	if validation.SchemaVersion != ValidationSummarySchema || validation.Status != "ready" || validation.Gates == nil {
		t.Fatalf("validation document = %#v", validation)
	}
	var promotion DesignPromotionDocument
	readJSON(t, filepath.Join(root, DesignPromotionPath), &promotion)
	if promotion.SchemaVersion != DesignPromotionSchema || promotion.Applicability.Status != "inapplicable" || promotion.Applicability.Rationale == "" {
		t.Fatalf("promotion document = %#v", promotion)
	}

	writtenManifest, status, err := manifest.Read(root)
	if err != nil || !status.Present || status.Stale {
		t.Fatalf("manifest status=%#v err=%v", status, err)
	}
	if writtenManifest.SchemaVersion != manifest.SchemaVersion || writtenManifest.CreationLane != "circuit" || len(writtenManifest.Evidence) != 5 {
		t.Fatalf("manifest = %#v", writtenManifest)
	}
	for _, evidence := range writtenManifest.Evidence {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(evidence.Path)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if evidence.SHA256 != hex.EncodeToString(sum[:]) || evidence.SchemaVersion == "" || evidence.GenerationStage == "" || evidence.Kind == "" {
			t.Fatalf("evidence entry = %#v", evidence)
		}
	}

	paths := []string{DesignRequestPath, WorkflowResultPath, ValidationSummaryPath, DesignPromotionPath, manifest.RelativePath}
	before := readFiles(t, root, paths)
	if _, issues := Write(root, bundle); len(issues) != 0 {
		t.Fatalf("repeat write issues = %#v", issues)
	}
	after := readFiles(t, root, paths)
	for index := range before {
		if !bytes.Equal(before[index], after[index]) {
			t.Fatalf("%s changed across repeated generation", paths[index])
		}
	}
}

func TestWriteFailurePreservesPreviousCoreEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, DesignRequestPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := []byte("previous-complete-evidence\n")
	if err := os.WriteFile(path, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, issues := Write(root, testBundle()); len(issues) == 0 {
		t.Fatal("write without transaction provenance unexpectedly succeeded")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, previous) {
		t.Fatalf("failed evidence write replaced previous bytes: %q", current)
	}
}

func TestExternalPathNormalizationPreservesIdentity(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	firstPath := filepath.Join(firstDir, "report.json")
	secondPath := filepath.Join(secondDir, "report.json")
	if err := os.WriteFile(firstPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := normalizeOptionalPath("/project", firstPath, "first")
	second := normalizeOptionalPath("/project", secondPath, "second")
	if first == second || !strings.HasPrefix(first, "external-evidence://") || !strings.HasSuffix(first, "/report.json") || !strings.HasSuffix(second, "/report.json") {
		t.Fatalf("external path normalization first=%q second=%q", first, second)
	}
	thirdDir := t.TempDir()
	thirdPath := filepath.Join(thirdDir, "report.json")
	if err := os.WriteFile(thirdPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if third := normalizeOptionalPath("/another-machine/project", thirdPath, "third"); third != first {
		t.Fatalf("same external evidence content is machine-dependent: first=%q third=%q", first, third)
	}
	missingFirst := normalizeOptionalPath("/project", "/missing/one/report.json", "promotion.issues.0")
	missingSecond := normalizeOptionalPath("/project", "/missing/two/report.json", "promotion.issues.1")
	if missingFirst == missingSecond {
		t.Fatalf("missing external evidence identities collide: %q", missingFirst)
	}
}

func TestWriteIndexesExternalArtifactsWithoutBrokenPaths(t *testing.T) {
	root := t.TempDir()
	writeTestProvenance(t, root)
	externalPath := filepath.Join(t.TempDir(), "erc.json")
	if err := os.WriteFile(externalPath, []byte("external report"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := testBundle()
	bundle.Artifacts = []reports.Artifact{{Kind: reports.ArtifactERCReport, Path: externalPath}}
	if _, issues := Write(root, bundle); len(issues) != 0 {
		t.Fatalf("write issues = %#v", issues)
	}
	writtenManifest, _, err := manifest.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(writtenManifest.ExternalEvidence) != 1 || !strings.HasPrefix(writtenManifest.ExternalEvidence[0].URI, "external-evidence://") || writtenManifest.ExternalEvidence[0].SHA256 == "" {
		t.Fatalf("external evidence = %#v", writtenManifest.ExternalEvidence)
	}
}

func TestWriteRecordsPromotionEvaluationError(t *testing.T) {
	root := t.TempDir()
	writeTestProvenance(t, root)
	bundle := testBundle()
	bundle.PromotionApplicability = &Applicability{Status: "error", Rationale: "promotion metadata invalid"}
	if _, issues := Write(root, bundle); len(issues) != 0 {
		t.Fatalf("write issues = %#v", issues)
	}
	var promotion DesignPromotionDocument
	readJSON(t, filepath.Join(root, DesignPromotionPath), &promotion)
	if promotion.Applicability.Status != "error" || promotion.Applicability.Rationale == "" {
		t.Fatalf("promotion applicability = %#v", promotion.Applicability)
	}
}

func testBundle() Bundle {
	workflow := designworkflow.WorkflowResult{Project: designworkflow.ProjectSummary{Name: "demo", OutputDir: "project"}, Stages: []designworkflow.StageResult{}}
	return Bundle{
		Lane:       "circuit",
		Request:    designworkflow.Request{Version: designworkflow.RequestVersion, Name: "demo"},
		Workflow:   workflow,
		Validation: ValidationSummary{Status: "ready", Stage: string(designworkflow.StageValidation), Message: "complete", Gates: []Gate{}},
	}
}

func writeTestProvenance(t *testing.T, root string) {
	t.Helper()
	tx, err := transactions.Parse([]byte(`{"name":"demo","project":"demo","operations":[{"op":"create_project","name":"demo"},{"op":"write_project"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provenance.Write(root, provenance.New("demo", tx, "test")); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func readFiles(t *testing.T, root string, paths []string) [][]byte {
	t.Helper()
	result := make([][]byte, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, data)
	}
	return result
}
