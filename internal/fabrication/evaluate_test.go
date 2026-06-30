package fabrication

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fabricationprofiles "kicadai/internal/fabrication/profiles"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

func TestEvaluateMissingTargetBlocks(t *testing.T) {
	result := Evaluate(context.Background(), filepath.Join(t.TempDir(), "missing"), EvaluateOptions{DryRun: true})
	if result.Status != StatusBlocked || result.Summary.Project != EvidenceFail {
		t.Fatalf("result = %#v, want blocked missing target", result)
	}
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if len(result.Issues) == 0 || result.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("issues = %#v, want invalid target issue", result.Issues)
	}
}

func TestPhysicalRuleOptionsUseDefaultFabricationProfile(t *testing.T) {
	opts, issues := physicalRuleOptions(EvaluateOptions{})
	if len(issues) != 0 {
		t.Fatalf("physicalRuleOptions issues = %#v", issues)
	}
	if opts.ProfileID != fabricationprofiles.DefaultProfileID {
		t.Fatalf("ProfileID = %q", opts.ProfileID)
	}
	if opts.ProfileDetails == nil || opts.ProfileDetails.Hash == "" || opts.ProfileDetails.SourceKind != string(fabricationprofiles.SourceBuiltin) {
		t.Fatalf("ProfileDetails = %#v", opts.ProfileDetails)
	}
	if opts.MinPlatedPadAnnularRingMM != 0.15 || opts.MinViaRingMM != 0.10 || opts.MinCopperFeatureMM != 0.127 || opts.MinSolderMaskWebMM != 0.10 {
		t.Fatalf("thresholds = %#v", opts)
	}
}

func TestPhysicalRuleOptionsUseLocalFabricationProfile(t *testing.T) {
	dir := t.TempDir()
	writeEvaluateProfileFixture(t, filepath.Join(dir, "local.json"), fabricationprofiles.Profile{
		Schema:  fabricationprofiles.SchemaV1,
		ID:      "local_rules",
		Name:    "Local Rules",
		Version: "2026-06",
		Units:   "mm",
		Stackup: fabricationprofiles.Stackup{
			MinLayers:               2,
			MaxLayers:               2,
			AllowedLayerCounts:      []int{2},
			MinBoardThicknessMM:     1.0,
			MaxBoardThicknessMM:     1.6,
			DefaultBoardThicknessMM: 1.6,
		},
		Copper: fabricationprofiles.Copper{
			MinCopperToEdgeMM: 0.42,
			MinCopperSliverMM: 0.21,
		},
		Drill: fabricationprofiles.Drill{
			MinHoleToEdgeMM:     0.77,
			MinPadAnnularRingMM: 0.22,
			MinViaAnnularRingMM: 0.11,
		},
		SolderMask: fabricationprofiles.SolderMask{MinSolderMaskWebMM: 0.16},
		Assembly:   fabricationprofiles.Assembly{RequireCourtyards: true},
		Metadata:   fabricationprofiles.Metadata{RequireBoardFinish: true, RequireFabricationNotes: true, RequirePanelization: true},
	})
	opts, issues := physicalRuleOptions(EvaluateOptions{ManufacturerProfile: "local_rules", ManufacturerProfileDir: dir})
	if len(issues) != 0 {
		t.Fatalf("physicalRuleOptions issues = %#v", issues)
	}
	if opts.ProfileID != "local_rules" || opts.ProfileDetails == nil || opts.ProfileDetails.SourceKind != string(fabricationprofiles.SourceLocal) {
		t.Fatalf("profile = %#v details=%#v", opts.ProfileID, opts.ProfileDetails)
	}
	if opts.MinCopperEdgeMM != 0.42 || opts.MinHoleEdgeMM != 0.77 || opts.MinPlatedPadAnnularRingMM != 0.22 || opts.MinViaRingMM != 0.11 || opts.MinCopperFeatureMM != 0.21 || opts.MinSolderMaskWebMM != 0.16 {
		t.Fatalf("thresholds = %#v", opts)
	}
	if !opts.RequireCourtyard || !opts.RequireBoardFinish || !opts.RequireFabricationNotes {
		t.Fatalf("requirements = %#v", opts)
	}
}

func TestPhysicalRuleOptionsReportUnknownProfile(t *testing.T) {
	opts, issues := physicalRuleOptions(EvaluateOptions{ManufacturerProfile: "missing"})
	if len(issues) == 0 || !hasIssuePath(issues, "fabrication_profile.id") {
		t.Fatalf("issues = %#v", issues)
	}
	if opts.ProfileID != "missing" {
		t.Fatalf("ProfileID = %q", opts.ProfileID)
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

func writeEvaluateProfileFixture(t *testing.T, path string, profile fabricationprofiles.Profile) {
	t.Helper()
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
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
	if got := validationKiCadCLI(EvaluateOptions{KiCadCLI: "/usr/bin/kicad-cli", CLIPolicy: CLIPolicyDisabled}); got != "" {
		t.Fatalf("validationKiCadCLI disabled = %q, want empty", got)
	}
	if got := validationKiCadCLI(EvaluateOptions{KiCadCLI: "/usr/bin/kicad-cli"}); got == "" {
		t.Fatalf("validationKiCadCLI default with CLI = empty, want configured CLI")
	}
	if got := validationKiCadCLI(EvaluateOptions{KiCadCLI: "/usr/bin/kicad-cli", CLIPolicy: CLIPolicyOptional}); got == "" {
		t.Fatalf("validationKiCadCLI optional = empty, want configured CLI")
	}
}

func TestCLIPolicyMissingEvidenceSeverity(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "demo.kicad_pro")
	if err := os.WriteFile(projectPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	optional := Evaluate(context.Background(), root, EvaluateOptions{CLIPolicy: CLIPolicyOptional})
	if severityForIssuePath(optional.Issues, "gerber") != reports.SeverityWarning {
		t.Fatalf("optional issues = %#v, want warning gerber issue", optional.Issues)
	}
	required := Evaluate(context.Background(), root, EvaluateOptions{CLIPolicy: CLIPolicyRequired})
	if severityForIssuePath(required.Issues, "gerber") != reports.SeverityError {
		t.Fatalf("required issues = %#v, want error gerber issue", required.Issues)
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

func severityForIssuePath(issues []reports.Issue, path string) reports.Severity {
	for _, issue := range issues {
		if issue.Path == path {
			return issue.Severity
		}
	}
	return ""
}
