package fabrication

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/checks"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

func TestExportPreviewDryRunWritesNoFiles(t *testing.T) {
	root := testFabricationProject(t)
	result := ExportPreview(context.Background(), root, Options{})
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if _, err := os.Stat(filepath.Join(root, "fabrication", "readiness.json")); !os.IsNotExist(err) {
		t.Fatalf("readiness file exists after dry-run or stat failed: %v", err)
	}
}

func TestExportPackageExecuteWritesArtifacts(t *testing.T) {
	root := testFabricationProject(t)
	result := ExportPackage(context.Background(), root, Options{Execute: true})
	if result.DryRun {
		t.Fatalf("DryRun = true, want false")
	}
	for _, rel := range []string{"readiness.json", "package-manifest.json", "physical-rules.json", "bom.csv", "cpl.csv"} {
		if _, err := os.Stat(filepath.Join(root, "fabrication", rel)); err != nil {
			t.Fatalf("%s not written: %v", rel, err)
		}
	}
	if result.ManifestPath == "" {
		t.Fatalf("ManifestPath is empty")
	}
	if result.Summary.BOM != EvidencePass || result.Summary.CPL != EvidencePass {
		t.Fatalf("BOM/CPL summary = %s/%s, want pass/pass", result.Summary.BOM, result.Summary.CPL)
	}
	if hasIssuePath(result.Issues, "bom") || hasIssuePath(result.Issues, "cpl") {
		t.Fatalf("stale BOM/CPL issues = %#v", result.Issues)
	}
}

func TestExportPackageManifestsPhysicalRulesReport(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	result := ExportPackage(context.Background(), root, Options{Execute: true})
	if result.PhysicalRules == nil {
		t.Fatalf("PhysicalRules = nil, want report")
	}
	if status := artifactStatus(result.Artifacts, ArtifactPhysicalRules); status != ArtifactGenerated {
		t.Fatalf("physical rules artifact status = %s, want generated", status)
	}
	data, err := os.ReadFile(filepath.Join(root, "fabrication", "package-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Evidence["physical_rules"] == "" {
		t.Fatalf("manifest evidence missing physical_rules: %#v", manifest.Evidence)
	}
	if result.ManufacturerProfile == nil || result.ManufacturerProfile.ID == "" {
		t.Fatalf("result manufacturer profile = %#v", result.ManufacturerProfile)
	}
	if manifest.ManufacturerProfile == nil || manifest.ManufacturerProfile.ID != result.ManufacturerProfile.ID || manifest.ManufacturerProfile.Hash == "" {
		t.Fatalf("manifest manufacturer profile = %#v result=%#v", manifest.ManufacturerProfile, result.ManufacturerProfile)
	}
	if artifact := manifestArtifact(manifest.Artifacts, ArtifactPhysicalRules); artifact.Path != "fabrication/physical-rules.json" {
		t.Fatalf("physical rules manifest artifact = %#v, want physical-rules.json", artifact)
	}
}

func TestExportPackageCopiesPassingBlockReadinessEvidence(t *testing.T) {
	root := testFabricationProject(t)
	promotionDir := filepath.Join(root, ".kicadai")
	if err := os.MkdirAll(promotionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	promotion := []byte(`{"status":"pass","achieved_readiness":"pass","matches_expectation":true,"gates":[{"status":"pass"}]}`)
	if err := os.WriteFile(filepath.Join(promotionDir, "design-promotion.json"), promotion, 0o644); err != nil {
		t.Fatal(err)
	}
	result := ExportPackage(context.Background(), root, Options{Execute: true, Overwrite: true})
	if result.Summary.BlockReadiness != EvidencePass {
		t.Fatalf("block readiness = %s, issues = %#v", result.Summary.BlockReadiness, result.Issues)
	}
	artifact := manifestArtifact(result.Artifacts, ArtifactBlockReadiness)
	if artifact.Status != ArtifactGenerated || artifact.Generator != GeneratorKiCadAI {
		t.Fatalf("block readiness artifact = %#v", artifact)
	}
	if _, err := os.Stat(filepath.Join(root, "fabrication", "block-readiness.json")); err != nil {
		t.Fatal(err)
	}
}

func TestExportPackageDryRunDoesNotCreatePlotDirectories(t *testing.T) {
	root := testFabricationProject(t)
	result := ExportPackage(context.Background(), root, Options{KiCadCLI: "kicad-cli"})
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	for _, rel := range []string{"gerbers", "drill"} {
		if _, err := os.Stat(filepath.Join(root, "fabrication", rel)); !os.IsNotExist(err) {
			t.Fatalf("%s exists after dry-run or stat failed: %v", rel, err)
		}
	}
	if status := artifactStatus(result.Artifacts, ArtifactGerber); status != ArtifactExpected {
		t.Fatalf("gerber artifact status = %s, want expected", status)
	}
}

func TestExportPackageExecuteWithFakeRunnerWritesPlotDirectories(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	writeTestSchematic(t, root)
	result := ExportPackage(context.Background(), root, Options{
		Execute:     true,
		Overwrite:   true,
		KiCadCLI:    "kicad-cli",
		PlotRunner:  writingPlotRunner{},
		CheckRunner: passingCheckRunner{},
	})
	for _, rel := range []string{"gerbers/demo-F_Cu.gbr", "drill/demo.drl", "erc.json", "drc.json"} {
		if _, err := os.Stat(filepath.Join(root, "fabrication", rel)); err != nil {
			t.Fatalf("%s not written: %v", rel, err)
		}
	}
	if status := artifactStatus(result.Artifacts, ArtifactGerber); status != ArtifactGenerated {
		t.Fatalf("gerber artifact status = %s, want generated", status)
	}
	if status := artifactStatus(result.Artifacts, ArtifactDrill); status != ArtifactGenerated {
		t.Fatalf("drill artifact status = %s, want generated", status)
	}
	if result.Summary.Gerber != EvidencePass || result.Summary.Drill != EvidencePass {
		t.Fatalf("summary gerber/drill = %s/%s, want pass/pass", result.Summary.Gerber, result.Summary.Drill)
	}
	if result.Summary.ERC != EvidencePass || result.Summary.DRC != EvidencePass {
		t.Fatalf("summary ERC/DRC = %s/%s, want pass/pass", result.Summary.ERC, result.Summary.DRC)
	}
	for _, kind := range []ArtifactKind{ArtifactERC, ArtifactDRC} {
		artifact := manifestArtifact(result.Artifacts, kind)
		if artifact.Status != ArtifactGenerated || artifact.Generator != GeneratorKiCad || len(artifact.Files) != 1 {
			t.Fatalf("%s artifact = %#v", kind, artifact)
		}
	}
	if hasIssuePath(result.Issues, "gerber") || hasIssuePath(result.Issues, "drill") {
		t.Fatalf("stale Gerber/drill issues = %#v", result.Issues)
	}
	data, err := os.ReadFile(filepath.Join(root, "fabrication", "package-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	gerber := manifestArtifact(manifest.Artifacts, ArtifactGerber)
	if gerber.Generator != GeneratorKiCad || len(gerber.Files) == 0 {
		t.Fatalf("gerber manifest artifact = %#v, want kicad-cli files", gerber)
	}
	drill := manifestArtifact(manifest.Artifacts, ArtifactDrill)
	if drill.Generator != GeneratorKiCad || len(drill.Files) == 0 {
		t.Fatalf("drill manifest artifact = %#v, want kicad-cli files", drill)
	}
}

func TestExportPackageUsesCustomOutputPathForPlotArtifacts(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	writeTestSchematic(t, root)
	result := ExportPackage(context.Background(), root, Options{
		Output:      "release",
		Execute:     true,
		Overwrite:   true,
		KiCadCLI:    "kicad-cli",
		PlotRunner:  writingPlotRunner{},
		CheckRunner: passingCheckRunner{},
	})
	if path := artifactPath(result.Artifacts, ArtifactGerber); path != "release/gerbers" {
		t.Fatalf("gerber artifact path = %q, want release/gerbers", path)
	}
	if path := artifactPath(result.Artifacts, ArtifactDrill); path != "release/drill" {
		t.Fatalf("drill artifact path = %q, want release/drill", path)
	}
}

func writeTestSchematic(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte("(kicad_sch)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverPlotPCBPathUsesExistingBoardWhenNameDiffers(t *testing.T) {
	root := testFabricationProject(t)
	boardPath := filepath.Join(root, "board.kicad_pcb")
	writeTestPCB(t, root, "board.kicad_pcb")
	got, issue := discoverPlotPCBPath(root, "demo")
	if issue != nil {
		t.Fatalf("discoverPlotPCBPath issue: %#v", issue)
	}
	if got != boardPath {
		t.Fatalf("PCB path = %q, want %q", got, boardPath)
	}
}

func TestDiscoverPlotPCBPathBlocksAmbiguousBoards(t *testing.T) {
	root := testFabricationProject(t)
	for _, name := range []string{"a.kicad_pcb", "b.kicad_pcb"} {
		writeTestPCB(t, root, name)
	}
	_, issue := discoverPlotPCBPath(root, "demo")
	if issue == nil {
		t.Fatalf("discoverPlotPCBPath accepted ambiguous board files")
	}
}

func TestExportPackageExecuteMissingKiCadCLIBlocksPlotArtifacts(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	result := ExportPackage(context.Background(), root, Options{Execute: true})
	if !hasIssuePath(result.Issues, "fabrication.kicad_cli") {
		t.Fatalf("issues = %#v, want missing KiCad CLI issue", result.Issues)
	}
	if status := artifactStatus(result.Artifacts, ArtifactGerber); status != ArtifactMissing {
		t.Fatalf("gerber artifact status = %s, want missing", status)
	}
}

func TestExportPackageBlocksExistingPlotDirWithoutOverwrite(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	gerberDir := filepath.Join(root, "fabrication", "gerbers")
	if err := os.MkdirAll(gerberDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gerberDir, "existing.gbr"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ExportPackage(context.Background(), root, Options{
		Execute:  true,
		KiCadCLI: "kicad-cli",
	})
	if !hasIssuePath(result.Issues, "fabrication/gerbers") {
		t.Fatalf("issues = %#v, want overwrite issue", result.Issues)
	}
}

func TestExportBlocksExistingFileWithoutOverwrite(t *testing.T) {
	root := testFabricationProject(t)
	fabricationDir := filepath.Join(root, "fabrication")
	if err := os.MkdirAll(fabricationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fabricationDir, "readiness.json"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ExportPreview(context.Background(), root, Options{Execute: true})
	if !hasIssuePath(result.Issues, "fabrication/readiness.json") {
		t.Fatalf("issues = %#v, want existing file issue", result.Issues)
	}
	if status := artifactStatus(result.Artifacts, ArtifactReadinessReport); status != ArtifactBlocked {
		t.Fatalf("readiness artifact status = %s, want blocked", status)
	}
}

func TestExportRejectsOutputOutsideProject(t *testing.T) {
	root := testFabricationProject(t)
	result := ExportPreview(context.Background(), root, Options{Output: filepath.Join(filepath.Dir(root), "outside")})
	if !hasIssueCode(result.Issues, reports.CodeInvalidArgument) {
		t.Fatalf("issues = %#v, want invalid output issue", result.Issues)
	}
}

func TestDiscoverPlotPCBPathBlocksMissingBoard(t *testing.T) {
	root := testFabricationProject(t)
	_, issue := discoverPlotPCBPath(root, "demo")
	if issue == nil || issue.Code != reports.CodeMissingFile {
		t.Fatalf("issue = %#v, want missing PCB issue", issue)
	}
}

func testFabricationProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeTestPCB(t *testing.T, root string, name string) {
	t.Helper()
	file, err := os.Create(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	board := pcbfiles.PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "kicadai-test",
		GeneratorVersion: "phase3",
		General:          pcbfiles.DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           pcbfiles.DefaultTwoLayerStack(),
		Setup:            pcbfiles.DefaultSetup(),
		Drawings: []pcbfiles.Drawing{{
			UUID:  kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
			Layer: kicadfiles.LayerEdge,
			Kind:  "line",
			Line: &pcbfiles.LineDrawing{
				Start: kicadfiles.Point{X: kicadfiles.MM(0), Y: kicadfiles.MM(0)},
				End:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(0)},
				Width: kicadfiles.MM(0.1),
			},
		}},
		Vias: []pcbfiles.Via{{
			UUID:     kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
			Position: kicadfiles.Point{X: kicadfiles.MM(5), Y: kicadfiles.MM(5)},
			Size:     kicadfiles.MM(0.8),
			Drill:    kicadfiles.MM(0.4),
			Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		}},
	}
	if err := pcbfiles.Write(file, board); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func artifactStatus(artifacts []Artifact, kind ArtifactKind) ArtifactStatus {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact.Status
		}
	}
	return ""
}

func artifactPath(artifacts []Artifact, kind ArtifactKind) string {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact.Path
		}
	}
	return ""
}

func manifestArtifact(artifacts []Artifact, kind ArtifactKind) Artifact {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact
		}
	}
	return Artifact{}
}

type writingPlotRunner struct{}

type passingCheckRunner struct{}

func (passingCheckRunner) Version(context.Context, string) (string, error) {
	return "10.0.3", nil
}

func (passingCheckRunner) Run(_ context.Context, _ string, _ string, args ...string) checks.CommandResult {
	reportPath := ""
	kind := "drc"
	for index, arg := range args {
		if arg == "erc" {
			kind = "erc"
		}
		if arg == "--output" && index+1 < len(args) {
			reportPath = args[index+1]
		}
	}
	data := []byte("{\"$schema\":\"https://schemas.kicad.org/drc.v1.json\",\"coordinate_units\":\"mm\",\"kicad_version\":\"10.0.3\",\"violations\":[],\"schematic_parity\":[]}\n")
	if kind == "erc" {
		data = []byte("{\"$schema\":\"https://schemas.kicad.org/erc.v1.json\",\"coordinate_units\":\"mm\",\"kicad_version\":\"10.0.3\",\"sheets\":[{\"path\":\"/\",\"uuid_path\":\"/11111111-1111-4111-8111-111111111111\",\"violations\":[]}]}\n")
	}
	if reportPath == "" {
		return checks.CommandResult{Args: args, ExitCode: 2, Err: os.ErrInvalid}
	}
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		return checks.CommandResult{Args: args, ExitCode: 2, Err: err}
	}
	return checks.CommandResult{Args: args, ExitCode: 0}
}

func (writingPlotRunner) RunPlotCommand(ctx context.Context, command PlotCommand) PlotCommandEvidence {
	if err := ctx.Err(); err != nil {
		return PlotCommandEvidence{Kind: command.Kind, Argv: command.Argv, OutputDir: command.OutputDir, ExitCode: -1, StderrSnippet: err.Error()}
	}
	if command.Kind == PlotKindDrill {
		path := filepath.Join(command.OutputDir, "demo.drl")
		if err := os.WriteFile(path, []byte("plot"), 0o644); err != nil {
			return PlotCommandEvidence{Kind: command.Kind, Argv: command.Argv, OutputDir: command.OutputDir, ExitCode: -1, StderrSnippet: err.Error()}
		}
		return PlotCommandEvidence{
			Kind:           command.Kind,
			Argv:           command.Argv,
			OutputDir:      command.OutputDir,
			GeneratedPaths: []string{path},
		}
	}
	paths := []string{
		filepath.Join(command.OutputDir, "demo-F_Cu.gbr"),
		filepath.Join(command.OutputDir, "demo-B_Cu.gbr"),
		filepath.Join(command.OutputDir, "demo-F_Mask.gbr"),
		filepath.Join(command.OutputDir, "demo-B_Mask.gbr"),
		filepath.Join(command.OutputDir, "demo-F_SilkS.gbr"),
		filepath.Join(command.OutputDir, "demo-B_SilkS.gbr"),
		filepath.Join(command.OutputDir, "demo-Edge_Cuts.gbr"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("plot"), 0o644); err != nil {
			return PlotCommandEvidence{Kind: command.Kind, Argv: command.Argv, OutputDir: command.OutputDir, ExitCode: -1, StderrSnippet: err.Error()}
		}
	}
	return PlotCommandEvidence{
		Kind:           command.Kind,
		Argv:           command.Argv,
		OutputDir:      command.OutputDir,
		GeneratedPaths: paths,
	}
}
