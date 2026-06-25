package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestRunCaseLEDIndicatorPassesSemantics(t *testing.T) {
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "led_indicator_default", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusPass {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Stages) != 3 || result.Stages[2].Name != "semantic_assertions" {
		t.Fatalf("stages = %#v", result.Stages)
	}
}

func TestRunCaseOmitsERCDRCStageWhenNotRequested(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusPass || hasStage(result.Stages, "erc_drc") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseWriterRequiredReportsKnownPadNetGap(t *testing.T) {
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "connector_breakout_4pin", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	manifest.Expected.Writer = ExpectedWriter{Required: true, OK: true, AllowUnrouted: true}
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:  blocks.NewBuiltinRegistry(),
		OutputDir: t.TempDir(),
		Overwrite: true,
	})
	if result.Status != StatusBlocked || !hasStage(result.Stages, "writer_correctness") || len(result.Artifacts) == 0 || !hasIssue(result.Issues, "PCB pad references missing net code") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseWriterSkipsWhenOutputDirMissingAndNotRequired(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.Writer = ExpectedWriter{OK: true}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	stage, ok := findStage(result.Stages, "writer_correctness")
	if !ok || stage.Status != StatusSkipped || result.Status != StatusPass {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseWriterBlocksWhenOutputDirMissingAndRequired(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.Writer = ExpectedWriter{Required: true}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "writer verification requires an output directory") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseWriterIssuesIncludeCaseContext(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.Writer = ExpectedWriter{Required: true}
	outputDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(outputDir, []byte("file blocks project directory creation"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry(), OutputDir: outputDir})
	if result.Status != StatusBlocked {
		t.Fatalf("result = %#v", result)
	}
	for _, issue := range result.Issues {
		if issue.Path != "" && strings.HasPrefix(issue.Path, "verification.led_indicator_default.") {
			return
		}
	}
	t.Fatalf("missing contextualized issue path: %#v", result.Issues)
}

func TestRunCaseERCDRCSkipsWhenKiCadMissingAndOptional(t *testing.T) {
	t.Setenv(checks.EnvKiCadCLI, filepath.Join(t.TempDir(), "missing-kicad-cli"))
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.ERCDRC.AllowedCodes = []string{"OPTIONAL_EXPECTATION"}
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:  blocks.NewBuiltinRegistry(),
		OutputDir: filepath.Join(t.TempDir(), "out"),
		Overwrite: true,
	})
	stage, ok := findStage(result.Stages, "erc_drc")
	if !ok || stage.Status != StatusSkipped || result.Status != StatusPass {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseERCDRCSkipsWhenOutputDirMissingAndOptional(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.ERCDRC.AllowedCodes = []string{"OPTIONAL_EXPECTATION"}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	stage, ok := findStage(result.Stages, "erc_drc")
	if !ok || stage.Status != StatusSkipped || result.Status != StatusPass || !strings.Contains(stage.Summary, "no output directory") {
		t.Fatalf("status=%s stage=%#v issues=%#v", result.Status, stage, result.Issues)
	}
}

func TestRunCaseERCDRCBlocksWhenRequiredAndKiCadMissing(t *testing.T) {
	t.Setenv(checks.EnvKiCadCLI, filepath.Join(t.TempDir(), "missing-kicad-cli"))
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.ERCDRC.Required = true
	manifest.Expected.ERCDRC.RequireDRC = true
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:  blocks.NewBuiltinRegistry(),
		OutputDir: filepath.Join(t.TempDir(), "out"),
		Overwrite: true,
	})
	stage, ok := findStage(result.Stages, "erc_drc")
	if result.Status != StatusBlocked || !ok || stage.Status != StatusBlocked || !hasIssue(result.Issues, "kicad-cli") || !hasIssuePath(result.Issues, ".erc_drc.kicad_cli") || !strings.Contains(stage.Summary, "KiCad CLI is required") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseERCDRCBlocksWhenOutputDirMissingAndRequired(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.ERCDRC.Required = true
	manifest.Expected.ERCDRC.RequireDRC = true
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	stage, ok := findStage(result.Stages, "erc_drc")
	if result.Status != StatusBlocked || !ok || stage.Status != StatusBlocked || !hasIssue(result.Issues, "requires an output directory") || !strings.Contains(stage.Summary, "missing output directory") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseERCDRCMockedViolationBlocks(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.ERCDRC.Required = true
	manifest.Expected.ERCDRC.RequireDRC = true
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:  blocks.NewBuiltinRegistry(),
		OutputDir: filepath.Join(t.TempDir(), "out"),
		Overwrite: true,
		KiCadCLI:  fakeExecutable(t, "kicad-cli"),
		CheckRunner: func(_ context.Context, kind checks.CheckKind, _ checks.KiCadCLI, _ string, _ checks.Options) (checks.CheckResult, error) {
			return checks.CheckResult{
				Kind: kind,
				Findings: []checks.CheckFinding{{
					Kind:     kind,
					Severity: "error",
					Code:     "KICADAI_TEST_FINDING",
					Message:  "mocked unallowlisted violation",
				}},
			}, nil
		},
	})
	stage, ok := findStage(result.Stages, "erc_drc")
	if result.Status != StatusBlocked || !ok || stage.Status != StatusBlocked || !hasIssue(result.Issues, "mocked unallowlisted violation") || !hasIssuePath(result.Issues, ".erc_drc.drc") || !strings.Contains(stage.Summary, "ran 1 KiCad ERC/DRC check(s)") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseERCDRCMockedPass(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.EvidenceLevel = EvidenceERCDRCVerified
	reportDir := t.TempDir()
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:  blocks.NewBuiltinRegistry(),
		OutputDir: filepath.Join(t.TempDir(), "out"),
		Overwrite: true,
		KiCadCLI:  fakeExecutable(t, "kicad-cli"),
		CheckRunner: func(_ context.Context, kind checks.CheckKind, _ checks.KiCadCLI, _ string, _ checks.Options) (checks.CheckResult, error) {
			reportPath := filepath.Join(reportDir, string(kind)+".json")
			if err := os.WriteFile(reportPath, []byte(`{"findings":[]}`), 0o644); err != nil {
				return checks.CheckResult{}, fmt.Errorf("write mocked report: %w", err)
			}
			return checks.CheckResult{Kind: kind, ReportPath: reportPath}, nil
		},
	})
	stage, ok := findStage(result.Stages, "erc_drc")
	if result.Status != StatusPass || !ok || stage.Status != StatusPass || result.EvidenceLevel != EvidenceERCDRCVerified || !hasArtifactKind(result.Artifacts, reports.ArtifactERCReport) || !hasArtifactKind(result.Artifacts, reports.ArtifactDRCReport) || !strings.Contains(stage.Summary, "ran 2 KiCad ERC/DRC check(s)") || !strings.Contains(stage.Summary, "produced") {
		t.Fatalf("status=%s stage=%#v artifacts=%#v issues=%#v", result.Status, stage, result.Artifacts, result.Issues)
	}
}

func TestRunCaseERCDRCExpectedAllowedIssuePasses(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.EvidenceLevel = EvidenceERCDRCVerified
	manifest.Expected.ERCDRC.AllowedCodes = []string{"KICADAI_ALLOWED"}
	manifest.Expected.ERCDRC.ExpectedIssues = []string{"KICADAI_ALLOWED"}
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:  blocks.NewBuiltinRegistry(),
		OutputDir: filepath.Join(t.TempDir(), "out"),
		Overwrite: true,
		KiCadCLI:  fakeExecutable(t, "kicad-cli"),
		CheckRunner: func(_ context.Context, kind checks.CheckKind, _ checks.KiCadCLI, _ string, opts checks.Options) (checks.CheckResult, error) {
			if len(opts.Allowlist) == 0 || opts.Allowlist[0].Code != "KICADAI_ALLOWED" {
				t.Fatalf("allowlist = %#v", opts.Allowlist)
			}
			return checks.CheckResult{
				Kind: kind,
				Allowed: []checks.CheckFinding{{
					Kind:     kind,
					Severity: "warning",
					Code:     "KICADAI_ALLOWED",
					Message:  "mocked allowed violation",
				}},
			}, nil
		},
	})
	stage, ok := findStage(result.Stages, "erc_drc")
	if result.Status != StatusPass || !ok || stage.Status != StatusPass || len(result.Issues) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseERCDRCGlobalOptionsStrengthenManifest(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:   true,
		RequireERC: true,
	}
	var gotKindsMu sync.Mutex
	var gotKinds []checks.CheckKind
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:   blocks.NewBuiltinRegistry(),
		OutputDir:  filepath.Join(t.TempDir(), "out"),
		Overwrite:  true,
		KiCadCLI:   fakeExecutable(t, "kicad-cli"),
		RequireDRC: true,
		CheckRunner: func(_ context.Context, kind checks.CheckKind, _ checks.KiCadCLI, _ string, _ checks.Options) (checks.CheckResult, error) {
			gotKindsMu.Lock()
			gotKinds = append(gotKinds, kind)
			gotKindsMu.Unlock()
			return checks.CheckResult{Kind: kind}, nil
		},
	})
	stage, ok := findStage(result.Stages, "erc_drc")
	gotKindsMu.Lock()
	gotERC := hasCheckKind(gotKinds, checks.CheckKindERC)
	gotDRC := hasCheckKind(gotKinds, checks.CheckKindDRC)
	gotKindCount := len(gotKinds)
	gotKindsSnapshot := append([]checks.CheckKind(nil), gotKinds...)
	gotKindsMu.Unlock()
	if result.Status != StatusPass || !ok || stage.Status != StatusPass || gotKindCount != 2 || !gotERC || !gotDRC {
		t.Fatalf("kinds=%#v result=%#v", gotKindsSnapshot, result)
	}
}

func TestEnsureProjectForExternalChecksReturnsArtifactOnCacheHit(t *testing.T) {
	manifest := validManifest()
	output := &blocks.BlockOutput{}
	outputDir := t.TempDir()
	projectDir := filepath.Join(outputDir, manifest.ID)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := writeProjectSentinel(projectDir, manifest, output, RunOptions{}); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	gotProjectDir, artifacts, issues := ensureProjectForExternalChecks(manifest, output, RunOptions{OutputDir: outputDir})
	if gotProjectDir != projectDir || len(issues) != 0 {
		t.Fatalf("projectDir=%q artifacts=%#v issues=%#v", gotProjectDir, artifacts, issues)
	}
	if len(artifacts) != 1 || string(artifacts[0].Kind) != "kicad_project" || artifacts[0].Path != filepath.ToSlash(projectDir) {
		t.Fatalf("artifacts = %#v", artifacts)
	}
}

func TestEnsureProjectForExternalChecksBlocksStaleCacheHit(t *testing.T) {
	manifest := validManifest()
	outputDir := t.TempDir()
	projectDir := filepath.Join(outputDir, manifest.ID)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	_, _, issues := ensureProjectForExternalChecks(manifest, &blocks.BlockOutput{}, RunOptions{OutputDir: outputDir})
	if !hasIssue(issues, "stale or incomplete") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDedupeArtifactsByKindAndPath(t *testing.T) {
	artifacts := dedupeArtifacts([]reports.Artifact{
		{Kind: reports.ArtifactKiCadProject, Path: "out/project"},
		{Kind: reports.ArtifactKiCadProject, Path: "out/project"},
		{Kind: reports.ArtifactERCReport, Path: "out/project"},
	})
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %#v", artifacts)
	}
}

func TestRunCaseBlocksWrongExpectedSymbol(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.Components[0].SymbolID = "Device:C"
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "expected symbol Device:C") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseBlocksMissingNetPinMembership(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.Nets[0].Pins[0].Pin = "1"
	manifest.Expected.Nets[0].Pins[1].Pin = "2"
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "expected role") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCasePCBPlacementAssertionPasses(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{
		{Role: "resistor", XMM: floatRef(0), YMM: floatRef(0), ToleranceMM: floatRef(0.001)},
		{Role: "led", XMM: floatRef(12.7), YMM: floatRef(0), ToleranceMM: floatRef(0.001)},
	}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusPass || !hasStage(result.Stages, "pcb_assertions") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCasePCBPlacementAssertionBlocksWrongLocation(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{
		{Role: "resistor", XMM: floatRef(99), YMM: floatRef(0), ToleranceMM: floatRef(0.001)},
	}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "expected placement role resistor") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCasePCBRequiredRouteBlocksWhenMissing(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.RequiredRoutes = []string{"status_led_series"}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "missing required route status_led_series") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCasePCBRealizationAssertionsPass(t *testing.T) {
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "canned_oscillator_default", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	satisfied := true
	manifest.Expected.PCB.RequireRealization = true
	manifest.Expected.PCB.RequiredLocalRoutes = []string{"osc_vcc_decoupling", "osc_gnd_decoupling"}
	manifest.Expected.PCB.TimingFixtures = []ExpectedTimingFixture{{ID: "canned_oscillator_core", Satisfied: &satisfied}}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusPass || !hasStage(result.Stages, "pcb_realization") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCasePCBRealizationBlocksMissingLocalRoute(t *testing.T) {
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "canned_oscillator_default", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	manifest.Expected.PCB.RequiredLocalRoutes = []string{"missing_route"}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "missing required local route missing_route") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCasePCBRealizationBlocksMissingTimingFixture(t *testing.T) {
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "canned_oscillator_default", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	manifest.Expected.PCB.TimingFixtures = []ExpectedTimingFixture{{ID: "missing_timing"}}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "missing expected timing fixture missing_timing") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCasePCBRealizationBlocksTimingExpectationMismatch(t *testing.T) {
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "canned_oscillator_default", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	satisfied := false
	manifest.Expected.PCB.TimingFixtures = []ExpectedTimingFixture{{
		ID:               "canned_oscillator_core",
		Satisfied:        &satisfied,
		RequiredFindings: []string{blocks.TimingFindingDecouplingPresent},
	}}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "expected timing fixture satisfied=false, got true") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseBoardValidationRequiresOutputDir(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.PCB.RequireBoardValidation = true
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasStage(result.Stages, "board_validation") || !hasIssue(result.Issues, "board validation requires an output directory") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseBoardValidationCatchesGeneratedProjectNetAssignments(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.PCB.RequireBoardValidation = true
	manifest.Expected.PCB.AllowUnrouted = true
	result := RunCase(context.Background(), manifest, RunOptions{
		Registry:  blocks.NewBuiltinRegistry(),
		OutputDir: filepath.Join(t.TempDir(), "out"),
		Overwrite: true,
	})
	stage, ok := findStage(result.Stages, "board_validation")
	if result.Status != StatusBlocked || !ok || stage.Status != StatusBlocked || len(result.Artifacts) == 0 || !hasIssue(result.Issues, "uses unknown net code") {
		t.Fatalf("result = %#v", result)
	}
}

func TestContextualizeBoardValidationIssuesAddsCaseContext(t *testing.T) {
	manifest := validManifest()
	issues := []reports.Issue{{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "footprints.0.pads.0.net_code",
		Message:  "bad net",
	}}
	got := contextualizeBoardValidationIssues(manifest, issues)
	if len(got) != 1 {
		t.Fatalf("issues = %#v", got)
	}
	if got[0].Path != "verification.led_indicator_default.board_validation.footprints_0_pads_0_net_code" {
		t.Fatalf("path = %q", got[0].Path)
	}
	if got[0].Suggestion == "" || issues[0].Suggestion != "" {
		t.Fatalf("got=%#v original=%#v", got, issues)
	}
}

func TestAssertPCBRequiredRouteTrimsExpectedName(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.RequiredRoutes = []string{" GND "}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{},
		PadNets:    map[padKey]string{},
		Routes:     map[string]struct{}{"GND": {}},
		ZoneNames:  map[string]struct{}{},
		ZoneNets:   map[string]struct{}{},
	}}
	if issues := assertPCB(manifest, summary); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBBlocksWrongPadNet(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.PadNets = []ExpectedPadNet{{Ref: "R1", Pad: "1", Net: "LED_A"}}
	summary := semanticSummary{
		PCB: actualPCB{
			Placements: map[string]actualPlacement{},
			PadNets:    map[padKey]string{{Ref: "R1", Pad: "1"}: "GND"},
			Routes:     map[string]struct{}{},
			ZoneNames:  map[string]struct{}{},
			ZoneNets:   map[string]struct{}{},
		},
	}
	issues := assertPCB(manifest, summary)
	if !hasIssue(issues, "expected pad net LED_A, got GND") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsMatchesRepeatedComponentRoles(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R", SymbolID: "Device:R"},
		{Role: "resistor", RefPrefix: "R", SymbolID: "Device:R"},
		{Role: "led", RefPrefix: "D", SymbolID: "Device:LED"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsDoesNotDoubleMatchExplicitRef(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
		{Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "missing expected component resistor") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsMatchesExplicitRefsBeforeRoles(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R", SymbolID: "Device:R"},
		{Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsAcceptsAnyMatchingRolePin(t *testing.T) {
	manifest := validManifest()
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsReportsMissingRolePin(t *testing.T) {
	manifest := validManifest()
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "C1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "expected role resistor pin 2") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestRunCaseStrictReportsUnexpectedNetWarning(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry(), Strict: true})
	if result.Status != StatusWarning || !hasIssue(result.Issues, "unexpected generated net") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAssertSemanticsChecksRoleForExplicitRef(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components[0].Ref = "R1"
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "capacitor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "expected role resistor, got capacitor") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertStrictSemanticsReportsUnexpectedComponentAndPort(t *testing.T) {
	manifest := validManifest()
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
			"C1": {Role: "capacitor", Ref: "C1", SymbolID: "Device:C"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}, "OUT": {Name: "OUT", Direction: blocks.PortOutput}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	for _, want := range []string{"unexpected generated component C1", "unexpected generated port OUT"} {
		if !hasIssue(issues, want) {
			t.Fatalf("issues missing %q: %#v", want, issues)
		}
	}
}

func TestAssertStrictSemanticsReportsExtraSharedRoleComponent(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if !hasIssue(issues, "unexpected generated component R2") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsChecksExplicitRefPrefix(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", Ref: "R1", RefPrefix: "C", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "expected ref prefix C, got R1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsUsesRefPrefixForRoleMatching(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R2", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if len(issues) != 1 || !hasIssue(issues, "unexpected generated component R1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsDoesNotSkipRoleCandidatesAfterPrefixMiss(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R2", SymbolID: "Device:R"},
		{Role: "resistor", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestComparePinNamesSortsNumericPinsNaturally(t *testing.T) {
	pins := []actualPin{{Ref: "U1", Pin: "A10"}, {Ref: "U1", Pin: "10"}, {Ref: "U1", Pin: "2"}, {Ref: "U1", Pin: "A2"}, {Ref: "U1", Pin: "01"}, {Ref: "U1", Pin: "1"}}
	got := uniquePins(pins)
	want := []string{"01", "1", "2", "10", "A2", "A10"}
	for index, pin := range got {
		if pin.Pin != want[index] {
			t.Fatalf("pins = %#v", got)
		}
	}
}

func TestAssertPCBPlacementRotationWrapAround(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Role: "led", XMM: floatRef(1), YMM: floatRef(2), RotationDeg: floatRef(0.1), ToleranceDeg: floatRef(0.3)}}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{"D1": {Ref: "D1", Role: "led", XMM: 1, YMM: 2, RotationDeg: 359.9}},
		PadNets:    map[padKey]string{},
		Routes:     map[string]struct{}{},
		ZoneNames:  map[string]struct{}{},
		ZoneNets:   map[string]struct{}{},
	}}
	if issues := assertPCB(manifest, summary); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBPlacementRoleMatchUsesCoordinates(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Role: "decoupling_capacitor", XMM: floatRef(20), YMM: floatRef(0), ToleranceMM: floatRef(0.01)}}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{
			"C1": {Ref: "C1", Role: "decoupling_capacitor", XMM: 10, YMM: 0},
			"C2": {Ref: "C2", Role: "decoupling_capacitor", XMM: 20, YMM: 0},
		},
		PadNets:   map[padKey]string{},
		Routes:    map[string]struct{}{},
		ZoneNames: map[string]struct{}{},
		ZoneNets:  map[string]struct{}{},
	}}
	if issues := assertPCB(manifest, summary); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBPlacementAllowsExplicitZeroTolerance(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Role: "led", XMM: floatRef(1), YMM: floatRef(2), ToleranceMM: floatRef(0)}}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{"D1": {Ref: "D1", Role: "led", XMM: 1.0005, YMM: 2}},
		PadNets:    map[padKey]string{},
		Routes:     map[string]struct{}{},
		ZoneNames:  map[string]struct{}{},
		ZoneNets:   map[string]struct{}{},
	}}
	issues := assertPCB(manifest, summary)
	if !hasIssue(issues, "expected placement role led") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBPlacementDoesNotReuseSameComponent(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{
		{Role: "led", XMM: floatRef(1), YMM: floatRef(2), ToleranceMM: floatRef(0.01)},
		{Role: "led", XMM: floatRef(1), YMM: floatRef(2), ToleranceMM: floatRef(0.01)},
	}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{"D1": {Ref: "D1", Role: "led", XMM: 1, YMM: 2}},
		PadNets:    map[padKey]string{},
		Routes:     map[string]struct{}{},
		ZoneNames:  map[string]struct{}{},
		ZoneNets:   map[string]struct{}{},
	}}
	issues := assertPCB(manifest, summary)
	if !hasIssue(issues, "missing expected PCB placement") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBPlacementChecksRoleForExplicitRef(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Ref: "R1", Role: "led"}}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{"R1": {Ref: "R1", Role: "resistor"}},
		PadNets:    map[padKey]string{},
		Routes:     map[string]struct{}{},
		ZoneNames:  map[string]struct{}{},
		ZoneNets:   map[string]struct{}{},
	}}
	issues := assertPCB(manifest, summary)
	if !hasIssue(issues, "expected placement role led") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBPlacementRoleMatchUsesRotation(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Role: "led", XMM: floatRef(1), YMM: floatRef(2), RotationDeg: floatRef(90), ToleranceMM: floatRef(0.01), ToleranceDeg: floatRef(0.1)}}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{
			"D1": {Ref: "D1", Role: "led", XMM: 1, YMM: 2, RotationDeg: 0},
			"D2": {Ref: "D2", Role: "led", XMM: 1, YMM: 2, RotationDeg: 90},
		},
		PadNets:   map[padKey]string{},
		Routes:    map[string]struct{}{},
		ZoneNames: map[string]struct{}{},
		ZoneNets:  map[string]struct{}{},
	}}
	if issues := assertPCB(manifest, summary); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBPlacementChecksFootprintID(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Ref: "D1", Role: "led", FootprintID: "LED_SMD:LED_0805_2012Metric"}}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{"D1": {Ref: "D1", Role: "led", FootprintID: "LED_THT:LED_D5.0mm"}},
		PadNets:    map[padKey]string{},
		Routes:     map[string]struct{}{},
		ZoneNames:  map[string]struct{}{},
		ZoneNets:   map[string]struct{}{},
	}}
	issues := assertPCB(manifest, summary)
	if !hasIssue(issues, "footprint LED_SMD:LED_0805_2012Metric") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBPlacementMatchesSpecificExpectationsBeforeGeneral(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.Placements = []ExpectedPlacement{
		{Role: "decoupling_capacitor"},
		{Role: "decoupling_capacitor", XMM: floatRef(10), YMM: floatRef(0), ToleranceMM: floatRef(0.01)},
	}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{
			"C1": {Ref: "C1", Role: "decoupling_capacitor", XMM: 10, YMM: 0},
			"C2": {Ref: "C2", Role: "decoupling_capacitor", XMM: 20, YMM: 0},
		},
		PadNets:   map[padKey]string{},
		Routes:    map[string]struct{}{},
		ZoneNames: map[string]struct{}{},
		ZoneNets:  map[string]struct{}{},
	}}
	if issues := assertPCB(manifest, summary); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertPCBRequiredZoneMatchesZoneNet(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.PCB.RequiredZones = []string{"GND"}
	summary := semanticSummary{PCB: actualPCB{
		Placements: map[string]actualPlacement{},
		PadNets:    map[padKey]string{},
		Routes:     map[string]struct{}{},
		ZoneNames:  map[string]struct{}{},
		ZoneNets:   map[string]struct{}{"GND": {}},
	}}
	if issues := assertPCB(manifest, summary); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestSummarizeOutputKeepsAnonymousNetsSeparate(t *testing.T) {
	output := blocks.BlockOutput{
		Operations: []transactions.Operation{
			rawConnect(t, "R1", "1", "R2", "1", ""),
			rawConnect(t, "R3", "1", "R4", "1", ""),
		},
	}
	summary, issues := summarizeOutput(output)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if got := len(summary.Nets); got != 2 {
		t.Fatalf("net count = %d, nets = %#v", got, summary.Nets)
	}
	for _, netName := range []string{"__anonymous_net_0", "__anonymous_net_1"} {
		if _, ok := summary.Nets[netName]; !ok {
			t.Fatalf("missing %s in %#v", netName, summary.Nets)
		}
	}
}

func TestSummarizeOutputMergesAnonymousConnectionChains(t *testing.T) {
	output := blocks.BlockOutput{
		Operations: []transactions.Operation{
			rawConnect(t, "R1", "1", "R2", "1", ""),
			rawConnect(t, "R2", "1", "R3", "1", ""),
		},
	}
	summary, issues := summarizeOutput(output)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	pins := summary.Nets["__anonymous_net_0"]
	if len(summary.Nets) != 1 || len(pins) != 3 {
		t.Fatalf("nets = %#v", summary.Nets)
	}
	for _, pin := range []actualPin{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}, {Ref: "R3", Pin: "1"}} {
		if _, ok := pinSet(pins)[pin]; !ok {
			t.Fatalf("missing pin %#v in %#v", pin, pins)
		}
	}
}

func TestSummarizeOutputRejectsEmptyPlacementRef(t *testing.T) {
	output := blocks.BlockOutput{
		Operations: []transactions.Operation{
			rawPlaceFootprint(t, "", "led", "LED_SMD:LED_0805_2012Metric"),
		},
	}
	summary, issues := summarizeOutput(output)
	if !hasIssue(issues, "requires ref") {
		t.Fatalf("issues = %#v", issues)
	}
	if _, ok := summary.PCB.Placements[""]; ok {
		t.Fatalf("empty placement ref was recorded: %#v", summary.PCB.Placements)
	}
}

func TestSummarizeOutputTrimsPlacementRef(t *testing.T) {
	output := blocks.BlockOutput{
		Operations: []transactions.Operation{
			rawPlaceFootprint(t, " D1 ", "led", "LED_SMD:LED_0805_2012Metric"),
		},
	}
	summary, issues := summarizeOutput(output)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if _, ok := summary.PCB.Placements["D1"]; !ok {
		t.Fatalf("trimmed placement ref missing: %#v", summary.PCB.Placements)
	}
	if _, ok := summary.PCB.Placements[" D1 "]; ok {
		t.Fatalf("untrimmed placement ref was recorded: %#v", summary.PCB.Placements)
	}
}

func rawConnect(t *testing.T, fromRef string, fromPin string, toRef string, toPin string, netName string) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(transactions.ConnectOperation{
		Op:      transactions.OpConnect,
		From:    transactions.Endpoint{Ref: fromRef, Pin: fromPin},
		To:      transactions.Endpoint{Ref: toRef, Pin: toPin},
		NetName: netName,
	})
	if err != nil {
		t.Fatalf("marshal connect: %v", err)
	}
	return transactions.NewOperation(transactions.OpConnect, raw)
}

func rawPlaceFootprint(t *testing.T, ref string, role string, footprintID string) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(transactions.PlaceFootprintOperation{
		Op:          transactions.OpPlaceFootprint,
		Ref:         ref,
		Role:        role,
		FootprintID: footprintID,
	})
	if err != nil {
		t.Fatalf("marshal place footprint: %v", err)
	}
	return transactions.NewOperation(transactions.OpPlaceFootprint, raw)
}

func fakeExecutable(t *testing.T, name string) string {
	t.Helper()
	contents := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		name += ".bat"
		contents = "@echo off\r\nexit /b 0\r\n"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}

func hasStage(stages []StageResult, name string) bool {
	_, ok := findStage(stages, name)
	return ok
}

func findStage(stages []StageResult, name string) (StageResult, bool) {
	for _, stage := range stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return StageResult{}, false
}

func hasIssuePath(issues []reports.Issue, fragment string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Path, fragment) {
			return true
		}
	}
	return false
}

func hasArtifactKind(artifacts []reports.Artifact, kind reports.ArtifactKind) bool {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return true
		}
	}
	return false
}

func hasCheckKind(kinds []checks.CheckKind, kind checks.CheckKind) bool {
	for _, got := range kinds {
		if got == kind {
			return true
		}
	}
	return false
}
