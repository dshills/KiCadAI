package fabrication

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

func TestEvaluateMissingTargetBlocks(t *testing.T) {
	result := Evaluate(context.Background(), filepath.Join(t.TempDir(), "missing"), EvaluateOptions{})
	if result.Status != StatusBlocked || result.Summary.Project != EvidenceFail {
		t.Fatalf("result = %#v, want blocked missing target", result)
	}
	if len(result.Issues) == 0 || result.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("issues = %#v, want invalid target issue", result.Issues)
	}
}

func TestEvaluateGeneratedProjectReportsProvenanceAndMissingCoreFiles(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "demo.kicad_pro")
	if err := os.WriteFile(projectPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Write(root, manifest.Manifest{
		ProjectName: "demo",
		Artifacts:   []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: projectPath}},
	}); err != nil {
		t.Fatal(err)
	}

	result := Evaluate(context.Background(), root, EvaluateOptions{})
	if !result.Summary.Generated || result.Summary.Manifest != EvidencePass {
		t.Fatalf("summary = %#v, want generated manifest evidence", result.Summary)
	}
	if result.Summary.Project != EvidencePass || result.Summary.Schematic != EvidenceMissing || result.Summary.PCB != EvidenceMissing {
		t.Fatalf("summary = %#v, want project present with missing schematic and PCB", result.Summary)
	}
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked because core design files are missing", result.Status)
	}
	if !hasIssueCode(result.Issues, reports.CodeMissingFile) {
		t.Fatalf("issues = %#v, want missing core file issue", result.Issues)
	}
}

func TestEvaluateProjectFileTargetAndPreviewOnlyProvenance(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "imported.kicad_pro")
	if err := os.WriteFile(projectPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := Evaluate(context.Background(), projectPath, EvaluateOptions{})
	if result.Summary.Generated || result.Summary.Manifest != EvidenceMissing {
		t.Fatalf("summary = %#v, want preview-only imported project", result.Summary)
	}
	if result.Summary.Project != EvidencePass {
		t.Fatalf("summary = %#v, want project evidence from .kicad_pro target", result.Summary)
	}
	if !hasIssuePath(result.Issues, "manifest") {
		t.Fatalf("issues = %#v, want missing provenance issue", result.Issues)
	}
}

func TestEvaluateUppercaseProjectExtension(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "Mixed.KICAD_PRO")
	if err := os.WriteFile(projectPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := Evaluate(context.Background(), projectPath, EvaluateOptions{})
	if result.Summary.Project != EvidencePass {
		t.Fatalf("summary = %#v, want project evidence from uppercase project extension", result.Summary)
	}
	target, err := resolveEvaluationTarget(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "Mixed" {
		t.Fatalf("target name = %s, want extension-stripped project name", target.Name)
	}
}

func TestEvaluateDirectoryWithoutProjectBlocks(t *testing.T) {
	result := Evaluate(context.Background(), t.TempDir(), EvaluateOptions{})
	if result.Status != StatusBlocked || result.Summary.Project != EvidenceFail {
		t.Fatalf("result = %#v, want blocked missing project", result)
	}
}

func TestEvaluateDirectoryWithAmbiguousProjectsBlocks(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha.kicad_pro", "beta.kicad_pro"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := Evaluate(context.Background(), root, EvaluateOptions{})
	if result.Status != StatusBlocked || result.Summary.Project != EvidenceFail {
		t.Fatalf("result = %#v, want blocked ambiguous project", result)
	}
}

func TestDryRunSuppressesExternalKiCadCLI(t *testing.T) {
	if got := validationKiCadCLI(EvaluateOptions{KiCadCLI: "/usr/bin/kicad-cli", DryRun: true}); got != "" {
		t.Fatalf("validationKiCadCLI dry-run = %q, want empty", got)
	}
	if got := validationKiCadCLI(EvaluateOptions{KiCadCLI: "/usr/bin/kicad-cli"}); got == "" {
		t.Fatalf("validationKiCadCLI execute = empty, want configured CLI")
	}
}

func TestRunValidationEvidenceHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := runValidationEvidence(ctx, t.TempDir(), EvaluateOptions{})
	if result.Writer != EvidenceFail || result.Board != EvidenceFail || !hasIssueCode(result.Issues, reports.CodeOperationCanceled) {
		t.Fatalf("result = %#v, want canceled validation evidence", result)
	}
}

func TestEvaluateMissingFabricationEvidencePreventsReady(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "demo.kicad_pro")
	if err := os.WriteFile(projectPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Write(root, manifest.Manifest{
		ProjectName: "demo",
		Artifacts:   []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: projectPath}},
	}); err != nil {
		t.Fatal(err)
	}

	result := Evaluate(context.Background(), root, EvaluateOptions{})
	for _, key := range []string{"erc", "bom", "cpl", "gerber", "drill"} {
		if resultEvidence(result, key) != EvidenceMissing {
			t.Fatalf("evidence %s = %s, want missing; result=%#v", key, resultEvidence(result, key), result)
		}
	}
	if result.Status == StatusReady {
		t.Fatalf("status = ready, want missing fabrication evidence to prevent ready")
	}
}

func resultEvidence(result Result, key string) EvidenceStatus {
	switch key {
	case "erc":
		return result.Summary.ERC
	case "bom":
		return result.Summary.BOM
	case "cpl":
		return result.Summary.CPL
	case "gerber":
		return result.Summary.Gerber
	case "drill":
		return result.Summary.Drill
	default:
		return ""
	}
}

func hasIssueCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasIssuePath(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}
