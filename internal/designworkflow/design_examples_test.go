package designworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"kicadai/internal/blocks"
	"kicadai/internal/componentprops"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

const designExamplePlanningTimeout = 15 * time.Second

type designExampleMetadata struct {
	ID                string          `json:"id"`
	Request           string          `json:"request"`
	Tier              string          `json:"tier"`
	Readiness         string          `json:"readiness"`
	Acceptance        AcceptanceLevel `json:"acceptance"`
	RequireERC        *bool           `json:"require_erc"`
	RequireDRC        *bool           `json:"require_drc"`
	Allowlists        []string        `json:"allowlists,omitempty"`
	ExpectedArtifacts []string        `json:"expected_artifacts,omitempty"`
	ExpectedStages    []StageName     `json:"expected_stages"`
	KnownGaps         []string        `json:"known_gaps"`
	Notes             string          `json:"notes,omitempty"`
}

var designExampleMetadataTiers = map[string]struct{}{
	"smoke":             {},
	"block-composition": {},
	"routing":           {},
	"fabrication":       {},
}

var designExampleMetadataReadiness = map[string]struct{}{
	"candidate":     {},
	"pass":          {},
	"expected_fail": {},
	"blocked":       {},
}

var designExampleMetadataAcceptance = map[AcceptanceLevel]struct{}{
	AcceptanceDraft:                {},
	AcceptanceStructural:           {},
	AcceptanceConnectivity:         {},
	AcceptanceERCDRC:               {},
	AcceptanceFabricationCandidate: {},
}

func TestDesignExamplesPassBlockPlanning(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	for _, name := range designExampleRequestFiles(t, repoRoot) {
		t.Run(name, func(t *testing.T) {
			request, issues := loadDesignExampleRequest(t, repoRoot, name)
			if len(issues) != 0 {
				t.Fatalf("decode %s issues: %#v", name, issues)
			}
			ctx, cancel := context.WithTimeout(context.Background(), designExamplePlanningTimeout)
			defer cancel()
			stage := designExamplePlanStage(ctx, request)
			if len(stage.Issues) != 0 {
				t.Fatalf("%s block planning issues:\n%s", name, formatDesignExampleIssues(stage.Issues))
			}
		})
	}
}

func TestDesignExampleValidationReportsContractDrift(t *testing.T) {
	tests := []struct {
		name        string
		request     Request
		wantPath    string
		wantMessage string
		wantIssues  int
	}{
		{
			name: "unknown LED parameter",
			request: Request{
				Version: RequestVersion,
				Name:    "invalid_led",
				Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
				Blocks: []BlockInstanceSpec{{
					ID:      "status",
					BlockID: "led_indicator",
					Params:  map[string]any{"resistor_ohms": 1000},
				}},
			},
			wantPath:    "blocks[0].params.resistor_ohms",
			wantMessage: "unknown parameter resistor_ohms",
			wantIssues:  1,
		},
		{
			name: "unknown connection endpoint",
			request: Request{
				Version: RequestVersion,
				Name:    "invalid_sensor",
				Board:   BoardSpec{WidthMM: 120, HeightMM: 90, Layers: 2},
				Blocks: []BlockInstanceSpec{
					{ID: "sensor", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48", "include_pullups": true}},
					{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"VCC", "GND", "SDA", "SCL", "INT"}}},
				},
				Connections: []ConnectionSpec{
					{From: "sensor.INT", To: "header.INT", NetAlias: "INT"},
				},
			},
			wantPath:    "connections[0].from",
			wantMessage: "unknown port sensor.INT",
			wantIssues:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), designExamplePlanningTimeout)
			defer cancel()
			stage := designExamplePlanStage(ctx, test.request)
			if len(stage.Issues) != test.wantIssues {
				t.Fatalf("issue count = %d, want %d:\n%s", len(stage.Issues), test.wantIssues, formatDesignExampleIssues(stage.Issues))
			}
			if !hasDesignExampleIssue(stage.Issues, test.wantPath, test.wantMessage) {
				t.Fatalf("missing expected issue %s containing %q in:\n%s", test.wantPath, test.wantMessage, formatDesignExampleIssues(stage.Issues))
			}
		})
	}
}

func TestDesignExamplesGenerateReadableProjectArtifacts(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	createTimeout := designExampleCreateTimeout(t)
	for _, name := range designExampleRequestFiles(t, repoRoot) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request, issues := loadDesignExampleRequest(t, repoRoot, name)
			if len(issues) != 0 {
				t.Fatalf("decode %s issues:\n%s", name, formatDesignExampleIssues(issues))
			}
			projectName := NormalizeProjectName(request.Name)
			outputDir := filepath.Join(t.TempDir(), projectName)
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), createTimeout)
			defer cancel()
			result := Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})
			if result.Acceptance.Achieved == "" {
				t.Fatalf("%s did not achieve an acceptance level:\n%s", name, formatDesignExampleStages(result.Stages))
			}
			projectWrite, ok := designExampleStageByName(result, StageProjectWrite)
			if !ok {
				t.Fatalf("%s missing project_write stage:\n%s", name, formatDesignExampleStages(result.Stages))
			}
			if projectWrite.Status != StageStatusOK {
				t.Fatalf("%s project_write status = %q, want %q:\n%s", name, projectWrite.Status, StageStatusOK, formatDesignExampleIssues(projectWrite.Issues))
			}
			projectPath := filepath.Join(outputDir, projectName+".kicad_pro")
			schematicPath := filepath.Join(outputDir, projectName+".kicad_sch")
			pcbPath := filepath.Join(outputDir, projectName+".kicad_pcb")
			transactionPath := filepath.Join(outputDir, ".kicadai", "transaction.json")
			manifestPath := filepath.Join(outputDir, ".kicadai", "manifest.json")
			requiredArtifacts := []string{projectPath, schematicPath, pcbPath, transactionPath, manifestPath}
			for _, path := range requiredArtifacts {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("%s missing generated artifact %s: %v", name, path, err)
				}
			}
			schematicFile, err := schematic.ReadFile(schematicPath)
			if err != nil {
				t.Fatalf("%s generated schematic is not readable: %v", name, err)
			}
			if got := countDesignExampleSymbolsWithProperty(schematicFile.Symbols, componentprops.PropertyComponentID); got == 0 {
				t.Fatalf("%s generated schematic has no %q properties", name, componentprops.PropertyComponentID)
			}
			if _, err := pcb.ReadFile(pcbPath); err != nil {
				t.Fatalf("%s generated PCB is not readable: %v", name, err)
			}
		})
	}
}

func TestDesignExamplesOptionalKiCadBackedTier(t *testing.T) {
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	if cliPath == "" {
		t.Skipf("set %s to run optional KiCad-backed design examples", checks.EnvKiCadCLI)
	}
	repoRoot := designExampleRepoRoot(t)
	metadataPaths := optionalDesignExampleMetadataFiles(t, repoRoot)
	if len(metadataPaths) == 0 {
		t.Skip("no optional KiCad-backed design examples found under examples/design/kicad-backed")
	}
	createTimeout := designExampleCreateTimeout(t)
	for _, metadataPath := range metadataPaths {
		metadataPath := metadataPath
		t.Run(strings.TrimSuffix(filepath.Base(metadataPath), ".metadata.json"), func(t *testing.T) {
			metadata, err := loadDesignExampleMetadataPath(metadataPath)
			if err != nil {
				t.Fatalf("load %s: %v", metadataPath, err)
			}
			if metadata.Readiness == "blocked" {
				t.Skipf("%s blocked: %s", metadata.ID, strings.Join(metadata.KnownGaps, "; "))
			}
			requestPath, err := designExampleRequestPathForMetadata(metadataPath, metadata)
			if err != nil {
				t.Fatalf("%s request path: %v", metadata.ID, err)
			}
			request, issues := loadDesignExampleRequestPath(t, requestPath)
			if len(issues) != 0 {
				t.Fatalf("decode %s issues:\n%s", requestPath, formatDesignExampleIssues(issues))
			}
			projectName := NormalizeProjectName(request.Name)
			outputDir := designExamplePersistentOutputDir(t, projectName)
			ctx, cancel := context.WithTimeout(context.Background(), createTimeout*2)
			defer cancel()
			result := Create(ctx, request, CreateOptions{
				OutputDir: outputDir,
				Overwrite: true,
				KiCadChecks: KiCadCheckOptions{
					KiCadCLI:      cliPath,
					Timeout:       createTimeout,
					RequireERC:    requiredDesignExampleBool(t, metadata.ID, "require_erc", metadata.RequireERC),
					RequireDRC:    requiredDesignExampleBool(t, metadata.ID, "require_drc", metadata.RequireDRC),
					KeepArtifacts: true,
					ArtifactDir:   filepath.Join(outputDir, ".kicadai", "checks"),
				},
			})
			promotion := BuildInternalPromotionReport(promotionFixtureFromDesignExampleMetadata(metadata), result)
			promotionArtifact, promotionIssue := WritePromotionReportArtifact(outputDir, promotion, true)
			if promotionIssue != nil {
				t.Fatalf("%s write promotion report: %s", metadata.ID, promotionIssue.Message)
			}
			if promotionArtifact.Kind != reports.ArtifactPromotionReport {
				t.Fatalf("%s promotion artifact = %#v", metadata.ID, promotionArtifact)
			}
			if _, err := os.Stat(designExampleArtifactPath(outputDir, PromotionReportArtifactPath)); err != nil {
				t.Fatalf("%s missing promotion report artifact: %v", metadata.ID, err)
			}
			assertDesignExampleExpectedStages(t, metadata, result, outputDir)
			assertDesignExampleExpectedArtifacts(t, metadata, result, outputDir)
			assertDesignExamplePromotionMatchesMetadata(t, metadata, promotion, outputDir, result)
			kicadChecks, ok := designExampleStageByName(result, StageKiCadChecks)
			if !ok {
				if metadata.Readiness == "expected_fail" && designExampleHasBlockedStage(result) {
					if !designExampleHasBlockedIssue(result) {
						t.Fatalf("%s expected at least one blocked issue:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
					}
					return
				}
				t.Fatalf("%s missing kicad_checks stage:\n%s", metadata.ID, formatDesignExampleStages(result.Stages))
			}
			switch metadata.Readiness {
			case "pass", "candidate":
				if kicadChecks.Status != StageStatusOK {
					t.Fatalf("%s kicad_checks status = %q, want %q:\n%s", metadata.ID, kicadChecks.Status, StageStatusOK, formatDesignExampleRun(metadata, outputDir, result))
				}
				assertDesignExampleKiCadArtifacts(t, metadata, outputDir, kicadChecks)
			case "expected_fail":
				if kicadChecks.Status == StageStatusOK || !designExampleHasBlockedStage(result) {
					t.Fatalf("%s expected blocked evidence, got kicad_checks=%q:\n%s", metadata.ID, kicadChecks.Status, formatDesignExampleRun(metadata, outputDir, result))
				}
				if !designExampleHasBlockedIssue(result) {
					t.Fatalf("%s expected at least one blocked issue:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
				}
			default:
				t.Fatalf("%s unsupported readiness %q", metadata.ID, metadata.Readiness)
			}
		})
	}
}

func TestDesignExamplePromotionReportArtifactWrite(t *testing.T) {
	metadata := designExampleMetadata{
		ID:                "promotion_fixture",
		Request:           "promotion_fixture.json",
		Tier:              "smoke",
		Readiness:         "candidate",
		Acceptance:        AcceptanceStructural,
		RequireERC:        designExampleBool(false),
		RequireDRC:        designExampleBool(false),
		ExpectedArtifacts: []string{PromotionReportArtifactPath},
		ExpectedStages:    []StageName{StageBlockPlanning},
		KnownGaps:         []string{},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "promotion_fixture"}, AcceptanceStructural, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK, Artifacts: []reports.Artifact{{
			Kind: reports.ArtifactPromotionReport,
			Path: PromotionReportArtifactPath,
		}}},
	})
	report := BuildInternalPromotionReport(promotionFixtureFromDesignExampleMetadata(metadata), result)
	outputDir := t.TempDir()
	artifact, issue := WritePromotionReportArtifact(outputDir, report, true)
	if issue != nil {
		t.Fatalf("write promotion report: %#v", issue)
	}
	if artifact.Path != PromotionReportArtifactPath || artifact.Kind != reports.ArtifactPromotionReport {
		t.Fatalf("artifact = %#v", artifact)
	}
	var decoded PromotionReport
	data, err := os.ReadFile(designExampleArtifactPath(outputDir, PromotionReportArtifactPath))
	if err != nil {
		t.Fatalf("read promotion report: %v", err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode promotion report: %v", err)
	}
	if decoded.ID != metadata.ID || decoded.Status == "" {
		t.Fatalf("decoded promotion report = %#v", decoded)
	}
}

func TestDesignExamplePromotionClassificationMatchesMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping design workflow classification integration test in short mode")
	}
	if strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI)) != "" {
		t.Skipf("set %s: promotion classification is covered by TestDesignExamplesOptionalKiCadBackedTier", checks.EnvKiCadCLI)
	}
	repoRoot := designExampleRepoRoot(t)
	createTimeout := designExampleCreateTimeout(t)
	for _, metadataPath := range optionalDesignExampleMetadataFiles(t, repoRoot) {
		metadataPath := metadataPath
		t.Run(strings.TrimSuffix(filepath.Base(metadataPath), ".metadata.json"), func(t *testing.T) {
			t.Parallel()
			metadata, err := loadDesignExampleMetadataPath(metadataPath)
			if err != nil {
				t.Fatalf("load %s: %v", metadataPath, err)
			}
			if metadata.Readiness == "blocked" {
				t.Skipf("%s blocked: %s", metadata.ID, strings.Join(metadata.KnownGaps, "; "))
			}
			requestPath, err := designExampleRequestPathForMetadata(metadataPath, metadata)
			if err != nil {
				t.Fatalf("%s request path: %v", metadata.ID, err)
			}
			request, issues := loadDesignExampleRequestPath(t, requestPath)
			if len(issues) != 0 {
				if metadata.Readiness != "expected_fail" {
					t.Fatalf("decode %s issues:\n%s", requestPath, formatDesignExampleIssues(issues))
				}
			}
			projectName := metadata.ID
			outputDir := filepath.Join(t.TempDir(), NormalizeProjectName(projectName))
			ctx, cancel := context.WithTimeout(context.Background(), createTimeout)
			defer cancel()
			var result WorkflowResult
			if len(issues) != 0 {
				result = BuildWorkflowResult(ProjectSummary{Name: projectName, OutputDir: outputDir}, metadata.Acceptance, []StageResult{{
					Name:   StageParseRequest,
					Status: StageStatusBlocked,
					Issues: issues,
				}})
			} else {
				result = Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})
			}
			if metadata.ID == "class_ab_headphone_protected" {
				assertDesignExampleProtectedAmplifierEvidence(t, metadata, outputDir, result)
			}
			if metadata.ID == "class_ab_headphone_driver" {
				assertDesignExampleClassABDriverEvidence(t, metadata, outputDir, result)
			}
			report := BuildInternalPromotionReport(promotionFixtureFromDesignExampleMetadata(metadata), result)
			reportJSON, err := MarshalPromotionReportJSON(report)
			if err != nil {
				t.Fatalf("%s invalid promotion report: %v", metadata.ID, err)
			}
			if len(reportJSON) == 0 {
				t.Fatalf("%s promotion report JSON is empty", metadata.ID)
			}
			assertDesignExamplePromotionMatchesMetadata(t, metadata, report, outputDir, result)
		})
	}
}

func TestI2CDesignExampleExpectedFailIsKiCadERCConnectivity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping I2C KiCad ERC classification integration test in short mode")
	}
	if strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI)) != "" {
		t.Skipf("set %s: local KiCad evidence may differ from default fake/internal ERC classification", checks.EnvKiCadCLI)
	}
	repoRoot := designExampleRepoRoot(t)
	metadataPath := filepath.Join(repoRoot, "examples", "design", "kicad-backed", "i2c_sensor_breakout_candidate.metadata.json")
	metadata, err := loadDesignExampleMetadataPath(metadataPath)
	if err != nil {
		t.Fatalf("load %s: %v", metadataPath, err)
	}
	if metadata.Readiness != "expected_fail" {
		t.Fatalf("%s readiness = %q, want expected_fail until KiCad ERC connectivity is clean", metadata.ID, metadata.Readiness)
	}
	requestPath, err := designExampleRequestPathForMetadata(metadataPath, metadata)
	if err != nil {
		t.Fatalf("%s request path: %v", metadata.ID, err)
	}
	request, issues := loadDesignExampleRequestPath(t, requestPath)
	if len(issues) != 0 {
		t.Fatalf("decode %s issues:\n%s", requestPath, formatDesignExampleIssues(issues))
	}
	outputDir := filepath.Join(t.TempDir(), NormalizeProjectName(metadata.ID))
	ctx, cancel := context.WithTimeout(context.Background(), designExampleCreateTimeout(t))
	defer cancel()
	result := Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})
	report := BuildInternalPromotionReport(promotionFixtureFromDesignExampleMetadata(metadata), result)
	if report.Status != PromotionStatusExpectedFail || report.AchievedReadiness != PromotionReadinessExpectedFail {
		t.Fatalf("%s promotion status=%q achieved=%q, want expected_fail\n%s", metadata.ID, report.Status, report.AchievedReadiness, formatDesignExampleRun(metadata, outputDir, result))
	}
	ercDependencyStages := []StageName{StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation}
	for _, stageName := range ercDependencyStages {
		stage, ok := designExampleStageByName(result, stageName)
		if !ok {
			t.Fatalf("%s missing downstream stage %q:\n%s", metadata.ID, stageName, formatDesignExampleRun(metadata, outputDir, result))
		}
		if stage.Status == StageStatusBlocked || stage.Status == StageStatusSkipped {
			t.Fatalf("%s stage %q status = %q, want progressed evidence before ERC blocker:\n%s", metadata.ID, stageName, stage.Status, formatDesignExampleRun(metadata, outputDir, result))
		}
	}
	kicadChecks, ok := designExampleStageByName(result, StageKiCadChecks)
	if !ok {
		t.Fatalf("%s missing kicad_checks stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	schematicPath := filepath.Join(outputDir, NormalizeProjectName(request.Name)+".kicad_sch")
	schematicFile, err := schematic.ReadFile(schematicPath)
	if err != nil {
		t.Fatalf("%s generated schematic is not readable for connectivity diagnostics: %v", metadata.ID, err)
	}
	connectivityReport := schematic.InspectGeneratedConnectivity(schematicFile)
	if connectivityReport.ConnectedComponentCount == 0 {
		t.Fatalf("%s generated schematic produced no connectivity components:\n%s", metadata.ID, formatGeneratedConnectivityDiagnostics(connectivityReport))
	}
	if len(connectivityReport.OffGridObjects) != 0 {
		t.Fatalf("%s generated schematic still has off-grid connectivity diagnostics:\n%s", metadata.ID, formatGeneratedConnectivityDiagnostics(connectivityReport))
	}
	if kicadChecks.Status != StageStatusBlocked {
		t.Fatalf("%s kicad_checks status = %q, want blocked by KiCad ERC connectivity:\n%s\nschematic connectivity diagnostics:\n%s", metadata.ID, kicadChecks.Status, formatDesignExampleRun(metadata, outputDir, result), formatGeneratedConnectivityDiagnostics(connectivityReport))
	}
	// KiCad versions report the same ERC connectivity blocker with different wording.
	acceptedERCConnectivityMessages := []string{"Pin not connected", "Unconnected wire endpoint"}
	foundERCConnectivityBlocker := false
	for _, want := range acceptedERCConnectivityMessages {
		if designExampleStageHasIssueMessage(kicadChecks, want) || designExamplePromotionHasIssueMessage(report, StageKiCadChecks, want) {
			foundERCConnectivityBlocker = true
			break
		}
	}
	if !foundERCConnectivityBlocker {
		t.Errorf("%s missing ERC connectivity blocker matching one of %v\nstage issues:\n%s\npromotion issues:\n%s\nschematic connectivity diagnostics:\n%s", metadata.ID, acceptedERCConnectivityMessages, formatDesignExampleIssues(kicadChecks.Issues), formatDesignExamplePromotionIssues(report.Issues), formatGeneratedConnectivityDiagnostics(connectivityReport))
	}
	for _, stageName := range ercDependencyStages {
		if designExamplePromotionHasBlockingStage(report, stageName) {
			t.Fatalf("%s promotion issues include stale downstream blocking issue at %s:\n%s", metadata.ID, stageName, formatDesignExamplePromotionIssues(report.Issues))
		}
	}
}

func TestI2CDesignExampleFakeCleanKiCadEvidencePassesCandidateGate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping I2C fake KiCad promotion integration test in short mode")
	}
	repoRoot := designExampleRepoRoot(t)
	metadataPath := filepath.Join(repoRoot, "examples", "design", "kicad-backed", "i2c_sensor_breakout_candidate.metadata.json")
	metadata, err := loadDesignExampleMetadataPath(metadataPath)
	if err != nil {
		t.Fatalf("load %s: %v", metadataPath, err)
	}
	requestPath, err := designExampleRequestPathForMetadata(metadataPath, metadata)
	if err != nil {
		t.Fatalf("%s request path: %v", metadata.ID, err)
	}
	request, issues := loadDesignExampleRequestPath(t, requestPath)
	if len(issues) != 0 {
		t.Fatalf("decode %s issues:\n%s", requestPath, formatDesignExampleIssues(issues))
	}
	outputDir := filepath.Join(t.TempDir(), NormalizeProjectName(metadata.ID))
	ctx, cancel := context.WithTimeout(context.Background(), designExampleCreateTimeout(t))
	defer cancel()
	result := Create(ctx, request, CreateOptions{
		OutputDir: outputDir,
		Overwrite: true,
		KiCadChecks: KiCadCheckOptions{
			KiCadCLI:      fakeKiCadCheckCLI(t),
			RequireERC:    true,
			RequireDRC:    true,
			KeepArtifacts: true,
			ArtifactDir:   filepath.Join(outputDir, ".kicadai", "checks"),
		},
	})
	kicadChecks, ok := designExampleStageByName(result, StageKiCadChecks)
	if !ok {
		t.Fatalf("%s missing kicad_checks stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if kicadChecks.Status != StageStatusOK {
		t.Fatalf("%s fake clean kicad_checks status = %q, want ok:\n%s", metadata.ID, kicadChecks.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	fixture := promotionFixtureFromDesignExampleMetadata(metadata)
	fixture.DeclaredReadiness = PromotionReadinessCandidate
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, string(StageKiCadChecks))
	if gate.Status != PromotionGateStatusPass {
		t.Fatalf("%s fake clean KiCad gate = %q, want pass:\n%s", metadata.ID, gate.Status, formatDesignExamplePromotionIssues(report.Issues))
	}
	if !designExamplePromotionGateHasArtifactPath(gate, "erc") {
		t.Errorf("%s fake clean KiCad artifacts = %#v, missing ERC report", metadata.ID, gate.Artifacts)
	}
	if !designExamplePromotionGateHasArtifactPath(gate, "drc") {
		t.Errorf("%s fake clean KiCad artifacts = %#v, missing DRC report", metadata.ID, gate.Artifacts)
	}
}

func TestProtectedAmplifierValidationRoutingBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping protected amplifier validation/routing integration baseline in short mode")
	}
	if strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI)) != "" {
		t.Skipf("set %s: local KiCad evidence may move the first blocker", checks.EnvKiCadCLI)
	}
	repoRoot := designExampleRepoRoot(t)
	metadataPath := filepath.Join(repoRoot, "examples", "design", "kicad-backed", "class_ab_headphone_protected.metadata.json")
	metadata, err := loadDesignExampleMetadataPath(metadataPath)
	if err != nil {
		t.Fatalf("load %s: %v", metadataPath, err)
	}
	requestPath, err := designExampleRequestPathForMetadata(metadataPath, metadata)
	if err != nil {
		t.Fatalf("%s request path: %v", metadata.ID, err)
	}
	request, issues := loadDesignExampleRequestPath(t, requestPath)
	if len(issues) != 0 {
		t.Fatalf("decode %s issues:\n%s", requestPath, formatDesignExampleIssues(issues))
	}
	if request.Validation.SkipRouting {
		t.Fatalf("%s route policy still has fixture-level skip_routing after routing closeout", metadata.ID)
	}
	outputDir := filepath.Join(t.TempDir(), NormalizeProjectName(metadata.ID))
	ctx, cancel := context.WithTimeout(context.Background(), designExampleCreateTimeout(t))
	defer cancel()
	result := Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})

	for _, stageName := range []StageName{StagePCBRealization, StagePlacement} {
		stage, ok := designExampleStageByName(result, stageName)
		if !ok {
			t.Fatalf("%s missing stage %q:\n%s", metadata.ID, stageName, formatDesignExampleRun(metadata, outputDir, result))
		}
		if stage.Status == StageStatusSkipped {
			t.Fatalf("%s stage %q unexpectedly skipped:\n%s", metadata.ID, stageName, formatDesignExampleRun(metadata, outputDir, result))
		}
	}
	routing, ok := designExampleStageByName(result, StageRouting)
	if !ok {
		t.Fatalf("%s missing routing stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if routing.Status != StageStatusBlocked {
		t.Fatalf("%s routing status = %q, want blocked with explicit route evidence:\n%s", metadata.ID, routing.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	if reason, ok := routing.Summary["reason"].(string); ok && reason == "routing skipped" {
		t.Fatalf("%s routing still reports stale skip reason:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	routeConnectivity := requireRouteConnectivitySummary(t, routing)
	if routeConnectivity.EndpointsUnresolved != 0 || routeConnectivity.EndpointNetMismatches != 0 {
		t.Fatalf("%s routing endpoint baseline regressed: %#v", metadata.ID, routeConnectivity)
	}
	if routeConnectivity.RoutesAttempted == 0 || routeConnectivity.RoutesBound == 0 || routeConnectivity.EndpointContactsProven == 0 {
		t.Fatalf("%s routing did not attempt bound local routes: %#v", metadata.ID, routeConnectivity)
	}
	interBlock := requireInterBlockRouteSummary(t, routing)
	if interBlock.EndpointsUnresolved != 0 || interBlock.MissingRequired != 0 || interBlock.RequiredEndpoints == 0 || interBlock.ProvenEndpoints == 0 {
		t.Fatalf("%s inter-block routing handoff is not endpoint-complete enough to route: %#v", metadata.ID, interBlock)
	}
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, routing)
	if routeTrees.GroupsAttempted == 0 || routeTrees.FixedNetSkipNotices == 0 || routeTrees.BlockingIssueCount == 0 {
		t.Fatalf("%s route-tree policy evidence missing fixed-net/blocking details: %#v", metadata.ID, routeTrees)
	}
	contactGraph := requireStageSummary[RouteTreeContactGraphSummary](t, routing, "route_tree_contact_graph")
	if contactGraph.RequiredEndpoints == 0 || contactGraph.ProvenEndpoints == 0 || contactGraph.PartialGroups == 0 {
		t.Fatalf("%s route-tree contact graph missing explicit partial-route evidence: %#v", metadata.ID, contactGraph)
	}
	requiredNets := requireStageSummary[RequiredNetClassificationSummary](t, routing, "required_net_classification")
	if requiredNets.RequiredInterBlock != 7 || requiredNets.Complete != 6 || requiredNets.Partial != 1 || requiredNets.MissingEndpoints != 1 {
		t.Fatalf("%s required-net classification = %#v, want six complete nets and one partial blocker", metadata.ID, requiredNets)
	}
	vccClassified := false
	for _, item := range requiredNets.Nets {
		if item.NetName == "VCC" && item.Kind == RequiredNetKindInterBlock && item.Status == RouteTreeContactGraphGroupPartial && item.Blocking && slices.Equal(item.MissingEndpointIDs, []string{"output.3"}) {
			vccClassified = true
		}
	}
	if !vccClassified {
		t.Fatalf("%s required-net classification missing partial VCC blocker: %#v", metadata.ID, requiredNets.Nets)
	}
	if !designExampleIssuesContainNet(routing.Issues, "VCC") {
		t.Fatalf("%s routing blocker does not identify VCC:\n%s", metadata.ID, formatDesignExampleIssues(routing.Issues))
	}
	projectWrite, ok := designExampleStageByName(result, StageProjectWrite)
	if !ok {
		t.Fatalf("%s missing project_write stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if projectWrite.Status != StageStatusSkipped {
		t.Fatalf("%s project_write status = %q, want skipped after explicit routing blocker:\n%s", metadata.ID, projectWrite.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	kicadChecks, ok := designExampleStageByName(result, StageKiCadChecks)
	if !ok {
		t.Fatalf("%s missing kicad_checks stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if kicadChecks.Status != StageStatusSkipped {
		t.Fatalf("%s kicad_checks status = %q, want skipped until validation/routing closeout:\n%s", metadata.ID, kicadChecks.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	report := BuildInternalPromotionReport(promotionFixtureFromDesignExampleMetadata(metadata), result)
	if report.Status != PromotionStatusExpectedFail {
		t.Fatalf("%s promotion report status = %q, want %q", metadata.ID, report.Status, PromotionStatusExpectedFail)
	}
	if report.AchievedReadiness != PromotionReadinessExpectedFail {
		t.Fatalf("%s promotion report achieved readiness = %q, want %q", metadata.ID, report.AchievedReadiness, PromotionReadinessExpectedFail)
	}
	for _, expectation := range []struct {
		id             string
		status         PromotionGateStatus
		wantIssueCodes bool
	}{
		{id: "route_completion", status: PromotionGateStatusFailed, wantIssueCodes: true},
		{id: "kicad_checks", status: PromotionGateStatusSkipped},
		{id: "writer_correctness", status: PromotionGateStatusSkipped},
		{id: "connectivity", status: PromotionGateStatusSkipped},
	} {
		t.Run("promotion_gate_"+expectation.id, func(t *testing.T) {
			gate := promotionGateByID(t, report, expectation.id)
			if gate.Status != expectation.status {
				t.Errorf("%s gate status = %q, want %q: %#v", expectation.id, gate.Status, expectation.status, gate)
			}
			if expectation.wantIssueCodes && len(gate.IssueCodes) == 0 {
				t.Errorf("%s gate issue code count = 0, want issue evidence: %#v", expectation.id, gate)
			}
			if !expectation.wantIssueCodes && len(gate.IssueCodes) > 0 {
				t.Errorf("%s gate issue code count = %d, want 0 with status %q: %#v", expectation.id, len(gate.IssueCodes), gate.Status, gate)
			}
		})
	}
}

func assertDesignExampleProtectedAmplifierEvidence(t *testing.T, metadata designExampleMetadata, outputDir string, result WorkflowResult) {
	t.Helper()
	stage, ok := designExampleStageByName(result, StageBlockPlanning)
	if !ok {
		t.Fatalf("%s missing block planning stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	rawSummary, ok := stage.Summary["headphone_output_protection"]
	if !ok {
		t.Fatalf("%s missing headphone_output_protection summary:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	summary, ok := rawSummary.(HeadphoneOutputProtectionSummary)
	if !ok {
		t.Fatalf("%s headphone_output_protection summary type = %T", metadata.ID, rawSummary)
	}
	if summary.InstanceID != "output_protection" || summary.BlockID != "headphone_output_protection" {
		t.Fatalf("%s protected output identity = %#v", metadata.ID, summary)
	}
	if summary.LoadKind != "headphone" || summary.NominalLoadOhms != "32Ω" {
		t.Fatalf("%s protected output load = %#v", metadata.ID, summary)
	}
	if !summary.ACOutputCouplingPresent || summary.DCBlockingCapacitance != "220uF" {
		t.Fatalf("%s protected output coupling = %#v", metadata.ID, summary)
	}
	if summary.BleedPolicyStatus != "present" || summary.SeriesResistorStatus != "omitted" {
		t.Fatalf("%s protected output resistor policy = %#v", metadata.ID, summary)
	}
	if summary.ConnectorReturnStatus != "load_return_and_reference_connected" {
		t.Fatalf("%s protected output return status = %#v", metadata.ID, summary)
	}
	if summary.FaultProtectionStatus != "placeholder_blocked" || summary.Readiness != "connectivity" {
		t.Fatalf("%s protected output readiness = %#v", metadata.ID, summary)
	}
	schematicElectrical, ok := designExampleStageByName(result, StageSchematicElectrical)
	if !ok {
		t.Fatalf("%s missing schematic electrical stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if schematicElectrical.Status != StageStatusOK {
		t.Fatalf("%s schematic electrical status = %q, want ok after alias cleanup:\n%s", metadata.ID, schematicElectrical.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	schematicPath := filepath.Join(outputDir, NormalizeProjectName(metadata.ID)+".kicad_sch")
	if _, err := os.Stat(schematicPath); err == nil {
		schematicFile, err := schematic.ReadFile(schematicPath)
		if err != nil {
			t.Fatalf("%s generated schematic is not readable for label/connectivity diagnostics: %v", metadata.ID, err)
		}
		connectivityReport := schematic.InspectGeneratedConnectivity(schematicFile)
		if len(connectivityReport.FloatingLabels) != 0 || len(connectivityReport.OffGridObjects) != 0 || len(connectivityReport.DanglingWireEndpoints) != 0 {
			t.Fatalf("%s generated schematic label/connectivity diagnostics are not clean:\n%s", metadata.ID, formatGeneratedConnectivityDiagnostics(connectivityReport))
		}
	} else if errors.Is(err, os.ErrNotExist) {
		projectWrite, ok := designExampleStageByName(result, StageProjectWrite)
		if !ok || projectWrite.Status != StageStatusSkipped {
			t.Fatalf("%s generated schematic is missing without a skipped project write stage: %v", metadata.ID, err)
		}
	} else {
		t.Fatalf("%s generated schematic stat failed: %v", metadata.ID, err)
	}
	for _, labels := range []string{
		"headphones_SIG,output_amp_out",
		"output_lower_emitter,output_upper_emitter",
		"AMP_OUT_DC_BIASED,HP_OUT",
	} {
		if designExampleIssuesContainNet(schematicElectrical.Issues, labels) {
			t.Fatalf("%s still has schematic label conflict %q in:\n%s", metadata.ID, labels, formatDesignExampleIssues(schematicElectrical.Issues))
		}
	}
	pcbRealization, ok := designExampleStageByName(result, StagePCBRealization)
	if !ok {
		t.Fatalf("%s missing PCB realization stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if pcbRealization.Status != StageStatusOK {
		t.Fatalf("%s PCB realization status = %q, want ok after board envelope fit:\n%s", metadata.ID, pcbRealization.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	if designExampleIssuesContainPath(pcbRealization.Issues, "pcb_realization.output") {
		t.Fatalf("%s PCB realization still has output placement warning:\n%s", metadata.ID, formatDesignExampleIssues(pcbRealization.Issues))
	}
	placement, ok := designExampleStageByName(result, StagePlacement)
	if !ok {
		t.Fatalf("%s missing placement stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if placement.Status != StageStatusOK {
		t.Fatalf("%s placement status = %q, want ok after endpoint binding:\n%s", metadata.ID, placement.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	for _, path := range []string{
		"components.Qccde149e001.position",
		"components.Qccde149e002.position",
		"design.inter_block_routing.connections[0].to",
		"design.inter_block_routing.connections[5].to",
		"design.inter_block_routing.connections[6].from",
		"design.inter_block_routing.connections[7].to",
		"design.inter_block_routing.connections[8].from",
	} {
		if designExampleIssuesContainPath(placement.Issues, path) {
			t.Fatalf("%s placement still has resolved blocker %s:\n%s", metadata.ID, path, formatDesignExampleIssues(placement.Issues))
		}
	}
	for _, expectation := range []struct {
		stage  StageName
		status StageStatus
	}{
		{StageRouting, StageStatusBlocked},
		{StageProjectWrite, StageStatusSkipped},
		{StageWriterCorrect, StageStatusSkipped},
		{StageValidation, StageStatusSkipped},
		{StageKiCadChecks, StageStatusSkipped},
	} {
		stage, ok := designExampleStageByName(result, expectation.stage)
		if !ok {
			t.Fatalf("%s missing downstream stage %q:\n%s", metadata.ID, expectation.stage, formatDesignExampleRun(metadata, outputDir, result))
		}
		if stage.Status != expectation.status {
			t.Fatalf("%s downstream stage %q status = %q, want %q:\n%s", metadata.ID, expectation.stage, stage.Status, expectation.status, formatDesignExampleRun(metadata, outputDir, result))
		}
	}
	routing, ok := designExampleStageByName(result, StageRouting)
	if !ok {
		t.Fatalf("%s missing routing stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	routeConnectivity := requireRouteConnectivitySummary(t, routing)
	if routeConnectivity.EndpointsUnresolved != 0 || routeConnectivity.EndpointNetMismatches != 0 {
		t.Fatalf("%s routing endpoint handoff regressed: %#v\n%s", metadata.ID, routeConnectivity, formatDesignExampleIssues(routing.Issues))
	}
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, routing)
	if routeTrees.GroupsAttempted == 0 || routeTrees.BlockingIssueCount == 0 {
		t.Fatalf("%s routing lacks explicit route-tree blocker evidence: %#v", metadata.ID, routeTrees)
	}
	requiredNets := requireStageSummary[RequiredNetClassificationSummary](t, routing, "required_net_classification")
	if requiredNets.RequiredInterBlock == 0 || requiredNets.Partial == 0 || requiredNets.MissingEndpoints == 0 {
		t.Fatalf("%s routing lacks required-net classification evidence: %#v", metadata.ID, requiredNets)
	}
}

func assertDesignExampleClassABDriverEvidence(t *testing.T, metadata designExampleMetadata, outputDir string, result WorkflowResult) {
	t.Helper()
	componentSelection, ok := designExampleStageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("%s missing component selection stage:\n%s", metadata.ID, formatDesignExampleRun(metadata, outputDir, result))
	}
	if componentSelection.Status != StageStatusBlocked {
		t.Fatalf("%s component selection status = %q, want blocked:\n%s", metadata.ID, componentSelection.Status, formatDesignExampleRun(metadata, outputDir, result))
	}
	for _, path := range []string{
		"component_selection.supply_decoupling.vcc_bulk",
		"component_selection.output.upper_emitter_resistor",
		"component_selection.output.lower_emitter_resistor",
	} {
		if !designExampleIssuesContainPath(componentSelection.Issues, path) {
			t.Fatalf("%s component selection issues missing %s:\n%s", metadata.ID, path, formatDesignExampleIssues(componentSelection.Issues))
		}
	}
	for _, stageName := range []StageName{StageSchematic, StageSchematicElectrical, StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks} {
		stage, ok := designExampleStageByName(result, stageName)
		if !ok {
			t.Fatalf("%s missing downstream stage %q:\n%s", metadata.ID, stageName, formatDesignExampleRun(metadata, outputDir, result))
		}
		if stage.Status != StageStatusSkipped {
			t.Fatalf("%s downstream stage %q status = %q, want skipped:\n%s", metadata.ID, stageName, stage.Status, formatDesignExampleRun(metadata, outputDir, result))
		}
	}
	report := BuildInternalPromotionReport(promotionFixtureFromDesignExampleMetadata(metadata), result)
	if report.Status != PromotionStatusExpectedFail || !report.MatchesExpectation {
		t.Fatalf("%s promotion = %#v, want matched expected_fail", metadata.ID, report)
	}
	if !promotionReportHasStageIssue(report, StageComponentSelection, "COMPONENT_NOT_FOUND") {
		t.Fatalf("%s promotion issues missing component selection blocker:\n%s", metadata.ID, formatDesignExamplePromotionIssues(report.Issues))
	}
}

func designExampleIssuesContainNet(issues []reports.Issue, labels string) bool {
	var targets []string
	for _, target := range strings.Split(labels, ",") {
		if target = strings.TrimSpace(target); target != "" {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return false
	}
	for _, issue := range issues {
		matched := true
		for _, target := range targets {
			if !designExampleIssueContainsNet(issue, target) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func designExampleIssuesContainPath(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}

func designExampleIssueContainsNet(issue reports.Issue, target string) bool {
	for _, net := range issue.Nets {
		for _, token := range strings.Split(net, ",") {
			if strings.TrimSpace(token) == target {
				return true
			}
		}
	}
	return containsDelimitedToken(issue.Message, target)
}

func containsDelimitedToken(text string, target string) bool {
	if target == "" {
		return false
	}
	for start := 0; ; {
		index := strings.Index(text[start:], target)
		if index < 0 {
			return false
		}
		index += start
		end := index + len(target)
		if isTokenPrefixBoundary(text[:index]) && isTokenSuffixBoundary(text[end:]) {
			return true
		}
		start = end
	}
}

func isTokenPrefixBoundary(prefix string) bool {
	if prefix == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(prefix)
	return !isWordRune(r)
}

func isTokenSuffixBoundary(suffix string) bool {
	if suffix == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(suffix)
	return !isWordRune(r)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.[]/-+", r)
}

func TestDesignExampleMetadataValidation(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "led_indicator_kicad_smoke.json")
	if err := os.WriteFile(requestPath, []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:                "led_indicator_kicad_smoke",
		Request:           "led_indicator_kicad_smoke.json",
		Tier:              "smoke",
		Readiness:         "candidate",
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        designExampleBool(true),
		RequireDRC:        designExampleBool(true),
		ExpectedArtifacts: []string{PromotionReportArtifactPath},
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
		KnownGaps:         []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "led_indicator_kicad_smoke.metadata.json", metadata)
	loaded, err := loadDesignExampleMetadataPath(path)
	if err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}
	if !reflect.DeepEqual(loaded, metadata) {
		t.Fatalf("loaded metadata = %#v, want %#v", loaded, metadata)
	}
}

func TestDesignExampleOptionalMetadataFilesAreValid(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	metadataPaths := optionalDesignExampleMetadataFiles(t, repoRoot)
	if len(metadataPaths) == 0 {
		t.Fatal("no optional KiCad-backed metadata files found")
	}
	for _, metadataPath := range metadataPaths {
		t.Run(strings.TrimSuffix(filepath.Base(metadataPath), ".metadata.json"), func(t *testing.T) {
			metadata, err := loadDesignExampleMetadataPath(metadataPath)
			if err != nil {
				t.Fatalf("load %s: %v", metadataPath, err)
			}
			if _, err := designExampleRequestPathForMetadata(metadataPath, metadata); err != nil {
				t.Fatalf("%s request path: %v", metadata.ID, err)
			}
		})
	}
}

func TestDesignExampleMetadataRejectsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixture.metadata.json")
	if err := os.WriteFile(path, []byte(`{"id":"fixture","request":"fixture.json","tier":"smoke","readiness":"candidate"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "acceptance is required") {
		t.Fatalf("missing acceptance error = %v, want acceptance is required", err)
	}

	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "fixture.json",
		Tier:           "smoke",
		Readiness:      "candidate",
		Acceptance:     AcceptanceStructural,
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{},
	}
	path = writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err = loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "require_erc is required") {
		t.Fatalf("missing require_erc error = %v, want require_erc is required", err)
	}
	metadata.RequireERC = designExampleBool(false)
	metadata.RequireDRC = nil
	path = writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err = loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "require_drc is required") {
		t.Fatalf("missing require_drc error = %v, want require_drc is required", err)
	}
}

func TestDesignExampleMetadataRejectsInvalidEnums(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "fixture.json",
		Tier:           "slow",
		Readiness:      "candidate",
		Acceptance:     AcceptanceStructural,
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported tier") {
		t.Fatalf("invalid enum error = %v, want unsupported tier", err)
	}
}

func TestDesignExampleMetadataRejectsExpectedFailWithoutKnownGaps(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "fixture.json",
		Tier:           "smoke",
		Readiness:      "expected_fail",
		Acceptance:     AcceptanceERCDRC,
		RequireERC:     designExampleBool(true),
		RequireDRC:     designExampleBool(true),
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{"   "},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "known_gaps must describe") {
		t.Fatalf("expected_fail known_gaps error = %v, want known_gaps must describe", err)
	}
}

func TestDesignExampleMetadataRejectsBlockedWithoutNotes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "fixture.json",
		Tier:           "smoke",
		Readiness:      "blocked",
		Acceptance:     AcceptanceStructural,
		RequireERC:     designExampleBool(false),
		RequireDRC:     designExampleBool(false),
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{"unsupported topology"},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "notes must describe blocked") {
		t.Fatalf("blocked notes error = %v, want notes must describe blocked", err)
	}
}

func TestDesignExampleMetadataRejectsCandidateWithoutRequiredEvidencePolicy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:                "fixture",
		Request:           "fixture.json",
		Tier:              "smoke",
		Readiness:         "candidate",
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        designExampleBool(false),
		RequireDRC:        designExampleBool(true),
		ExpectedArtifacts: []string{PromotionReportArtifactPath},
		ExpectedStages:    []StageName{StageBlockPlanning, StagePCBRealization},
		KnownGaps:         []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), `fixture readiness "candidate" must require ERC`) {
		t.Fatalf("candidate ERC error = %v, want require ERC", err)
	}
	metadata.RequireERC = designExampleBool(true)
	metadata.RequireDRC = designExampleBool(false)
	path = writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err = loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), `PCB fixture readiness "candidate" must require DRC`) {
		t.Fatalf("candidate DRC error = %v, want require DRC", err)
	}
	metadata.RequireDRC = designExampleBool(true)
	metadata.ExpectedArtifacts = nil
	path = writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err = loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), `fixture readiness "candidate" must expect promotion report artifact`) {
		t.Fatalf("candidate promotion artifact error = %v, want promotion artifact", err)
	}
}

func TestDesignExampleMetadataRejectsPassWithGapsOrAllowlists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:                "fixture",
		Request:           "fixture.json",
		Tier:              "smoke",
		Readiness:         "pass",
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        designExampleBool(true),
		RequireDRC:        designExampleBool(true),
		Allowlists:        []string{"known_false_positive"},
		ExpectedArtifacts: []string{PromotionReportArtifactPath},
		ExpectedStages:    []StageName{StageBlockPlanning, StagePCBRealization},
		KnownGaps:         []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "pass fixtures must not use allowlists") {
		t.Fatalf("pass allowlist error = %v, want no allowlists", err)
	}
	metadata.Allowlists = nil
	metadata.KnownGaps = []string{"still missing evidence"}
	path = writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err = loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "pass fixtures must not have known_gaps") {
		t.Fatalf("pass gaps error = %v, want no known_gaps", err)
	}
}

func TestDesignExampleMetadataRejectsUnsafeArtifactPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		artifact string
		want     string
	}{
		{name: "absolute", artifact: "/tmp/artifact.json", want: "must be relative"},
		{name: "parent", artifact: "../artifact.json", want: "must stay in output directory"},
		{name: "empty", artifact: " ", want: "must not contain empty paths"},
		{name: "windows absolute", artifact: `C:\tmp\artifact.json`, want: "slash-separated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := designExampleMetadata{
				ID:                "fixture",
				Request:           "fixture.json",
				Tier:              "smoke",
				Readiness:         "candidate",
				Acceptance:        AcceptanceStructural,
				RequireERC:        designExampleBool(false),
				RequireDRC:        designExampleBool(false),
				ExpectedStages:    []StageName{StageBlockPlanning},
				KnownGaps:         []string{},
				ExpectedArtifacts: []string{test.artifact},
			}
			path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
			_, err := loadDesignExampleMetadataPath(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("artifact error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDesignExampleMetadataRejectsDotID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:             ".",
		Request:        ".json",
		Tier:           "smoke",
		Readiness:      "candidate",
		Acceptance:     AcceptanceStructural,
		RequireERC:     designExampleBool(false),
		RequireDRC:     designExampleBool(false),
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "id must be a local fixture identifier") {
		t.Fatalf("dot id error = %v, want local fixture identifier", err)
	}
}

func TestDesignExampleMetadataRejectsUnsafeIDCharacters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"bad:name", "CON", "lpt1", "CLOCK$", "fixture."} {
		t.Run(id, func(t *testing.T) {
			metadata := designExampleMetadata{
				ID:             id,
				Request:        "fixture.json",
				Tier:           "smoke",
				Readiness:      "candidate",
				Acceptance:     AcceptanceStructural,
				RequireERC:     designExampleBool(false),
				RequireDRC:     designExampleBool(false),
				ExpectedStages: []StageName{StageBlockPlanning},
				KnownGaps:      []string{},
			}
			path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
			_, err := loadDesignExampleMetadataPath(path)
			if err == nil || !strings.Contains(err.Error(), "id must be a local fixture identifier") {
				t.Fatalf("unsafe id error = %v, want local fixture identifier", err)
			}
		})
	}
}

func TestDesignExampleMetadataRejectsRequestPathTraversal(t *testing.T) {
	dir := t.TempDir()
	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "../fixture.json",
		Tier:           "smoke",
		Readiness:      "candidate",
		Acceptance:     AcceptanceStructural,
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "request must be a local JSON filename") {
		t.Fatalf("path traversal error = %v, want local filename rejection", err)
	}
}

func TestFormatDesignExampleStagesGroupsIssuesUnderStage(t *testing.T) {
	got := formatDesignExampleStages([]StageResult{{
		Name:   StageProjectWrite,
		Status: StageStatusBlocked,
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "operations[0]",
			Message:  "example failure",
		}, {
			Code:     reports.CodeMissingFootprint,
			Severity: reports.SeverityWarning,
			Path:     "components.R1",
			Message:  "missing footprint evidence",
		}},
	}})
	want := "- project_write: blocked\n  - error VALIDATION_FAILED at operations[0]: example failure\n  - warning MISSING_FOOTPRINT at components.R1: missing footprint evidence"
	if got != want {
		t.Fatalf("formatted stages:\n%q\nwant:\n%q", got, want)
	}
}

func TestKiCadBackedLEDExampleClearsWriterNetAssignmentAndBindsLocalRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping design workflow integration test in short mode")
	}
	repoRoot := designExampleRepoRoot(t)
	requestPath := filepath.Join(repoRoot, "examples", "design", "kicad-backed", "led_indicator_kicad_smoke.json")
	request, issues := loadDesignExampleRequestPath(t, requestPath)
	if len(issues) != 0 {
		t.Fatalf("decode %s issues:\n%s", requestPath, formatDesignExampleIssues(issues))
	}
	outputDir := filepath.Join(t.TempDir(), NormalizeProjectName(request.Name))
	ctx, cancel := context.WithTimeout(context.Background(), designExampleCreateTimeout(t))
	defer cancel()
	result := Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})
	writer, ok := designExampleStageByName(result, StageWriterCorrect)
	if !ok {
		t.Fatalf("missing writer correctness stage:\n%s", formatDesignExampleStages(result.Stages))
	}
	if writer.Status != StageStatusOK && writer.Status != StageStatusWarning {
		t.Fatalf("writer correctness status = %q, want ok or warning:\n%s", writer.Status, formatDesignExampleStages(result.Stages))
	}
	for _, issue := range writer.Issues {
		if issue.Code == reports.CodeInvalidNetAssignment {
			t.Fatalf("unexpected writer net-assignment issue after generated net assignment:\n%s", formatDesignExampleIssues(writer.Issues))
		}
	}
	routingStage, ok := designExampleStageByName(result, StageRouting)
	if !ok {
		t.Fatalf("missing routing stage:\nissues:\n%s\nstages:\n%s", formatDesignExampleIssues(resultIssues(result)), formatDesignExampleStages(result.Stages))
	}
	connectivity := requireRouteConnectivitySummary(t, routingStage)
	const minLEDLocalRouteEndpoints = 2
	if connectivity.RoutesAttempted == 0 {
		t.Fatalf("route connectivity summary = %#v, want at least one attempted LED local route", connectivity)
	}
	if connectivity.RoutesBound == 0 {
		t.Fatalf("route connectivity summary = %#v, want at least one bound LED local route", connectivity)
	}
	if connectivity.EndpointsResolved < minLEDLocalRouteEndpoints {
		t.Fatalf("route connectivity summary = %#v, want at least %d resolved LED local-route endpoints", connectivity, minLEDLocalRouteEndpoints)
	}
	if connectivity.EndpointContactsProven < minLEDLocalRouteEndpoints {
		t.Fatalf("route connectivity summary = %#v, want at least %d proven LED local-route endpoint contacts", connectivity, minLEDLocalRouteEndpoints)
	}
}

func TestKiCadBackedConnectorLEDExampleBindsLocalRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping design workflow integration test in short mode")
	}
	repoRoot := designExampleRepoRoot(t)
	requestName := "kicad-backed/connector_led_kicad_smoke.json"
	request, issues := loadDesignExampleRequest(t, repoRoot, requestName)
	if len(issues) != 0 {
		t.Fatalf("decode %s issues:\n%s", requestName, formatDesignExampleIssues(issues))
	}
	outputDir := filepath.Join(t.TempDir(), NormalizeProjectName(request.Name))
	ctx, cancel := context.WithTimeout(context.Background(), designExampleCreateTimeout(t))
	defer cancel()
	result := Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})
	routingStage, ok := designExampleStageByName(result, StageRouting)
	if !ok {
		t.Fatalf("missing routing stage:\nissues:\n%s\nstages:\n%s", formatDesignExampleIssues(resultIssues(result)), formatDesignExampleStages(result.Stages))
	}
	connectivity := requireRouteConnectivitySummary(t, routingStage)
	if connectivity.RoutesAttempted == 0 {
		t.Fatalf("route connectivity summary = %#v, want attempted connector/LED local route", connectivity)
	}
	if connectivity.RoutesBound == 0 {
		t.Fatalf("route connectivity summary = %#v, want bound connector/LED local route", connectivity)
	}
	if connectivity.EndpointsUnresolved != 0 || connectivity.EndpointNetMismatches != 0 {
		t.Fatalf("route connectivity summary = %#v, want no local route endpoint blockers", connectivity)
	}
}

func designExamplePlanStage(ctx context.Context, request Request) StageResult {
	planResult := PlanBlocks(ctx, blocks.NewBuiltinRegistry(), request)
	return planResult.Stage
}

func requireRouteConnectivitySummary(t *testing.T, stage StageResult) LocalRouteConnectivitySummary {
	t.Helper()
	connectivityValue, exists := stage.Summary["route_connectivity"]
	if !exists {
		t.Fatalf("missing route_connectivity summary key; summary keys=%v", sortedSummaryKeys(stage.Summary))
	}
	connectivity, ok := connectivityValue.(LocalRouteConnectivitySummary)
	if !ok {
		t.Fatalf("route_connectivity summary has type %T, want %T", connectivityValue, LocalRouteConnectivitySummary{})
	}
	return connectivity
}

func sortedSummaryKeys(summary map[string]any) []string {
	keys := make([]string, 0, len(summary))
	for key := range summary {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func resultIssues(result WorkflowResult) []reports.Issue {
	var issues []reports.Issue
	for _, stage := range result.Stages {
		issues = append(issues, stage.Issues...)
	}
	return issues
}

func loadDesignExampleRequest(t *testing.T, repoRoot, name string) (Request, []reports.Issue) {
	t.Helper()
	return loadDesignExampleRequestPath(t, filepath.Join(repoRoot, "examples", "design", name))
}

func loadDesignExampleRequestPath(t *testing.T, path string) (Request, []reports.Issue) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	return request, issues
}

func loadDesignExampleMetadataPath(path string) (designExampleMetadata, error) {
	var metadata designExampleMetadata
	file, err := os.Open(path)
	if err != nil {
		return metadata, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return designExampleMetadata{}, err
	}
	if err := validateDesignExampleMetadata(filepath.Dir(path), metadata); err != nil {
		return designExampleMetadata{}, err
	}
	return metadata, nil
}

func validateDesignExampleMetadata(dir string, metadata designExampleMetadata) error {
	if strings.TrimSpace(metadata.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if metadata.ID != strings.TrimSpace(metadata.ID) || metadata.ID != strings.TrimRight(metadata.ID, ". ") || metadata.ID == "." || strings.ContainsAny(metadata.ID, `/\:*?"<>|`) || strings.Contains(metadata.ID, "..") || isWindowsReservedFilename(metadata.ID) {
		return fmt.Errorf("id must be a local fixture identifier")
	}
	if strings.TrimSpace(metadata.Request) == "" {
		return fmt.Errorf("request is required")
	}
	if filepath.Base(metadata.Request) != metadata.Request || filepath.Ext(metadata.Request) != ".json" || strings.HasSuffix(metadata.Request, ".metadata.json") {
		return fmt.Errorf("request must be a local JSON filename")
	}
	if strings.TrimSuffix(filepath.Base(metadata.Request), ".json") != metadata.ID {
		return fmt.Errorf("id must match request basename")
	}
	requestPath := filepath.Join(dir, metadata.Request)
	info, err := os.Stat(requestPath)
	if err != nil {
		return fmt.Errorf("request %s: %w", metadata.Request, err)
	}
	if info.IsDir() {
		return fmt.Errorf("request %s must be a file", metadata.Request)
	}
	if _, ok := designExampleMetadataTiers[metadata.Tier]; !ok {
		return fmt.Errorf("unsupported tier %q", metadata.Tier)
	}
	if _, ok := designExampleMetadataReadiness[metadata.Readiness]; !ok {
		return fmt.Errorf("unsupported readiness %q", metadata.Readiness)
	}
	if _, ok := designExampleMetadataAcceptance[metadata.Acceptance]; !ok {
		if metadata.Acceptance == "" {
			return fmt.Errorf("acceptance is required")
		}
		return fmt.Errorf("unsupported acceptance %q", metadata.Acceptance)
	}
	if metadata.RequireERC == nil {
		return fmt.Errorf("require_erc is required")
	}
	if metadata.RequireDRC == nil {
		return fmt.Errorf("require_drc is required")
	}
	if len(metadata.ExpectedStages) == 0 {
		return fmt.Errorf("expected_stages must not be empty")
	}
	if metadata.KnownGaps == nil {
		return fmt.Errorf("known_gaps is required")
	}
	if (metadata.Readiness == "expected_fail" || metadata.Readiness == "blocked") && !hasNonEmptyString(metadata.KnownGaps) {
		return fmt.Errorf("known_gaps must describe expected_fail or blocked fixtures")
	}
	if metadata.Readiness == "blocked" && strings.TrimSpace(metadata.Notes) == "" {
		return fmt.Errorf("notes must describe blocked fixtures")
	}
	if err := validateDesignExampleArtifactPaths(metadata.ExpectedArtifacts); err != nil {
		return err
	}
	if err := validateDesignExampleProgressionPolicy(metadata); err != nil {
		return err
	}
	return nil
}

func validateDesignExampleProgressionPolicy(metadata designExampleMetadata) error {
	switch metadata.Readiness {
	case "candidate", "pass":
		if metadata.RequireERC == nil || !*metadata.RequireERC {
			return fmt.Errorf("fixture readiness %q must require ERC", metadata.Readiness)
		}
		if designExampleMetadataExpectsPCB(metadata) && (metadata.RequireDRC == nil || !*metadata.RequireDRC) {
			return fmt.Errorf("PCB fixture readiness %q must require DRC", metadata.Readiness)
		}
		if !containsDesignExampleString(metadata.ExpectedArtifacts, PromotionReportArtifactPath) {
			return fmt.Errorf("fixture readiness %q must expect promotion report artifact %q", metadata.Readiness, PromotionReportArtifactPath)
		}
	}
	if metadata.Readiness == "pass" {
		if len(metadata.KnownGaps) != 0 {
			return fmt.Errorf("pass fixtures must not have known_gaps")
		}
		if len(metadata.Allowlists) != 0 {
			return fmt.Errorf("pass fixtures must not use allowlists")
		}
	}
	return nil
}

func designExampleMetadataExpectsPCB(metadata designExampleMetadata) bool {
	for _, stage := range metadata.ExpectedStages {
		switch stage {
		case StagePCBRealization, StageSchematicToPCB, StagePlacement, StageRouting, StageFabricationReady:
			return true
		}
	}
	for _, artifact := range metadata.ExpectedArtifacts {
		if strings.HasSuffix(strings.ToLower(artifact), ".kicad_pcb") {
			return true
		}
	}
	return false
}

func validateDesignExampleArtifactPaths(paths []string) error {
	for _, artifactPath := range paths {
		if strings.TrimSpace(artifactPath) == "" {
			return fmt.Errorf("expected_artifacts must not contain empty paths")
		}
		if strings.ContainsAny(artifactPath, `\:*?"<>|`) {
			return fmt.Errorf("expected artifact %q must use relative slash-separated paths", artifactPath)
		}
		if pathpkg.IsAbs(artifactPath) {
			return fmt.Errorf("expected artifact %q must be relative", artifactPath)
		}
		cleaned := pathpkg.Clean(artifactPath)
		if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
			return fmt.Errorf("expected artifact %q must stay in output directory", artifactPath)
		}
	}
	return nil
}

func hasNonEmptyString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func containsDesignExampleString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func isWindowsReservedFilename(name string) bool {
	base := strings.ToUpper(name)
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	base = strings.TrimRight(base, ". ")
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return true
	}
	if len(base) == 4 {
		prefix := base[:3]
		suffix := base[3]
		return (prefix == "COM" || prefix == "LPT") && suffix >= '1' && suffix <= '9'
	}
	return false
}

func designExampleBool(value bool) *bool {
	return &value
}

func requiredDesignExampleBool(t *testing.T, id, field string, value *bool) bool {
	t.Helper()
	if value == nil {
		t.Fatalf("%s metadata missing %s", id, field)
	}
	return *value
}

func promotionFixtureFromDesignExampleMetadata(metadata designExampleMetadata) PromotionFixture {
	return PromotionFixture{
		ID:                metadata.ID,
		Request:           metadata.Request,
		Tier:              metadata.Tier,
		DeclaredReadiness: PromotionReadiness(metadata.Readiness),
		Acceptance:        metadata.Acceptance,
		RequireERC:        metadata.RequireERC != nil && *metadata.RequireERC,
		RequireDRC:        metadata.RequireDRC != nil && *metadata.RequireDRC,
		ExpectedArtifacts: append([]string(nil), metadata.ExpectedArtifacts...),
		ExpectedStages:    append([]StageName(nil), metadata.ExpectedStages...),
		KnownGaps:         append([]string(nil), metadata.KnownGaps...),
	}
}

func assertDesignExamplePromotionMatchesMetadata(t *testing.T, metadata designExampleMetadata, report PromotionReport, outputDir string, result WorkflowResult) {
	t.Helper()
	switch metadata.Readiness {
	case "expected_fail":
		if report.Status != PromotionStatusExpectedFail || !report.MatchesExpectation {
			t.Fatalf("%s promotion status=%q achieved=%q matches=%v, want expected_fail match\n%s", metadata.ID, report.Status, report.AchievedReadiness, report.MatchesExpectation, formatDesignExampleRun(metadata, outputDir, result))
		}
	case "candidate":
		if report.AchievedReadiness != PromotionReadinessCandidate && report.AchievedReadiness != PromotionReadinessPass {
			t.Fatalf("%s promotion achieved=%q, want candidate or pass\n%s", metadata.ID, report.AchievedReadiness, formatDesignExampleRun(metadata, outputDir, result))
		}
	case "pass":
		if report.AchievedReadiness != PromotionReadinessPass {
			t.Fatalf("%s promotion achieved=%q, want pass\n%s", metadata.ID, report.AchievedReadiness, formatDesignExampleRun(metadata, outputDir, result))
		}
	default:
		t.Fatalf("%s unsupported readiness %q", metadata.ID, metadata.Readiness)
	}
}

func designExamplePersistentOutputDir(t *testing.T, projectName string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kicadai-design-example-*")
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(dir, projectName)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("preserved design example output at %s", outputDir)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("remove design example temp dir %s: %v", dir, err)
		}
	})
	return outputDir
}

func writeDesignExampleMetadataFixture(t *testing.T, dir, name string, metadata designExampleMetadata) string {
	t.Helper()
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func designExampleRequestFiles(t *testing.T, repoRoot string) []string {
	t.Helper()
	pattern := filepath.Join(repoRoot, "examples", "design", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("no design examples matched %s", pattern)
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}
	sort.Strings(names)
	return names
}

func optionalDesignExampleMetadataFiles(t *testing.T, repoRoot string) []string {
	t.Helper()
	pattern := filepath.Join(repoRoot, "examples", "design", "kicad-backed", "*.metadata.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(matches)
	return matches
}

func designExampleRequestPathForMetadata(metadataPath string, metadata designExampleMetadata) (string, error) {
	baseDir := filepath.Clean(filepath.Dir(metadataPath))
	requestPath := filepath.Clean(filepath.Join(baseDir, metadata.Request))
	expectedPath := filepath.Clean(filepath.Join(baseDir, filepath.Base(requestPath)))
	if requestPath != expectedPath {
		return "", fmt.Errorf("request %q must stay in metadata directory", metadata.Request)
	}
	return requestPath, nil
}

func designExampleRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root")
		}
		dir = parent
	}
}

func hasDesignExampleIssue(issues []reports.Issue, path, message string) bool {
	for _, issue := range issues {
		if issue.Path == path && strings.Contains(issue.Message, message) {
			return true
		}
	}
	return false
}

func assertDesignExampleExpectedStages(t *testing.T, metadata designExampleMetadata, result WorkflowResult, outputDir string) {
	t.Helper()
	for _, name := range metadata.ExpectedStages {
		stage, ok := designExampleStageByName(result, name)
		if !ok {
			t.Fatalf("%s missing expected stage %q:\n%s", metadata.ID, name, formatDesignExampleRun(metadata, outputDir, result))
		}
		if metadata.Readiness != "expected_fail" && stage.Status == StageStatusBlocked {
			t.Fatalf("%s stage %q blocked unexpectedly:\n%s", metadata.ID, name, formatDesignExampleRun(metadata, outputDir, result))
		}
	}
}

func assertDesignExampleExpectedArtifacts(t *testing.T, metadata designExampleMetadata, result WorkflowResult, outputDir string) {
	t.Helper()
	if metadata.Readiness == "expected_fail" {
		projectWrite, ok := designExampleStageByName(result, StageProjectWrite)
		if !ok || projectWrite.Status != StageStatusOK {
			return
		}
	}
	for _, relative := range metadata.ExpectedArtifacts {
		path := designExampleArtifactPath(outputDir, relative)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s missing expected artifact %s:\n%s", metadata.ID, path, err)
		}
	}
}

func assertDesignExampleKiCadArtifacts(t *testing.T, metadata designExampleMetadata, outputDir string, stage StageResult) {
	t.Helper()
	if requiredDesignExampleBool(t, metadata.ID, "require_erc", metadata.RequireERC) && !designExampleStageHasArtifact(t, outputDir, stage, reports.ArtifactERCReport) {
		t.Fatalf("%s missing ERC report artifact:\n%s", metadata.ID, formatDesignExampleStageArtifacts(stage, outputDir))
	}
	if requiredDesignExampleBool(t, metadata.ID, "require_drc", metadata.RequireDRC) && !designExampleStageHasArtifact(t, outputDir, stage, reports.ArtifactDRCReport) {
		t.Fatalf("%s missing DRC report artifact:\n%s", metadata.ID, formatDesignExampleStageArtifacts(stage, outputDir))
	}
}

func designExampleStageHasArtifact(t *testing.T, outputDir string, stage StageResult, kind reports.ArtifactKind) bool {
	t.Helper()
	for _, artifact := range stage.Artifacts {
		if artifact.Kind == kind && strings.TrimSpace(artifact.Path) != "" {
			if _, err := os.Stat(designExampleArtifactPath(outputDir, artifact.Path)); err == nil {
				return true
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stat %s artifact %s: %v", kind, artifact.Path, err)
			}
		}
	}
	return false
}

func designExampleArtifactPath(outputDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(outputDir, filepath.FromSlash(path))
}

func designExampleHasBlockedStage(result WorkflowResult) bool {
	for _, stage := range result.Stages {
		if stage.Status == StageStatusBlocked {
			return true
		}
	}
	return false
}

func designExampleHasBlockedIssue(result WorkflowResult) bool {
	for _, stage := range result.Stages {
		if reports.HasBlockingIssue(stage.Issues) {
			return true
		}
	}
	return false
}

func countDesignExampleSymbolsWithProperty(symbols []schematic.SchematicSymbol, name string) int {
	count := 0
	for _, symbol := range symbols {
		for _, property := range symbol.Properties {
			if property.Name == name {
				count++
				break
			}
		}
	}
	return count
}

func formatDesignExampleIssues(issues []reports.Issue) string {
	if len(issues) == 0 {
		return "(none)"
	}
	var builder strings.Builder
	for _, issue := range issues {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(string(issue.Severity))
		builder.WriteByte(' ')
		builder.WriteString(string(issue.Code))
		if issue.Path != "" {
			builder.WriteString(" at ")
			builder.WriteString(issue.Path)
		}
		builder.WriteString(": ")
		builder.WriteString(issue.Message)
	}
	return builder.String()
}

func designExampleStageHasIssueMessage(stage StageResult, message string) bool {
	for _, issue := range stage.Issues {
		if strings.Contains(issue.Message, message) {
			return true
		}
	}
	return false
}

func designExamplePromotionHasIssueMessage(report PromotionReport, stage StageName, message string) bool {
	for _, issue := range report.Issues {
		if issue.Stage == stage && strings.Contains(issue.Message, message) {
			return true
		}
	}
	return false
}

func designExamplePromotionHasBlockingStage(report PromotionReport, stage StageName) bool {
	for _, issue := range report.Issues {
		if issue.Stage == stage && (issue.Severity == reports.SeverityError || issue.Severity == reports.SeverityBlocked) {
			return true
		}
	}
	return false
}

func designExamplePromotionGateHasArtifactPath(gate PromotionGate, fragment string) bool {
	fragment = strings.ToLower(fragment)
	for _, artifact := range gate.Artifacts {
		if strings.Contains(strings.ToLower(artifact), fragment) {
			return true
		}
	}
	return false
}

func formatGeneratedConnectivityDiagnostics(report schematic.GeneratedConnectivityReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "status=%s symbols=%d wires=%d labels=%d components=%d diagnostics=%d issues=%d", report.Status, report.SymbolCount, report.WireCount, report.LabelCount, report.ConnectedComponentCount, report.DiagnosticCount, report.IssueCount)
	writeDiagnostics := func(title string, diagnostics []schematic.GeneratedConnectivityDiagnostic) {
		if len(diagnostics) == 0 {
			return
		}
		builder.WriteByte('\n')
		builder.WriteString(title)
		builder.WriteByte(':')
		for _, diagnostic := range diagnostics {
			builder.WriteString("\n- ")
			builder.WriteString(diagnostic.Kind)
			if diagnostic.Reference != "" {
				builder.WriteString(" ")
				builder.WriteString(diagnostic.Reference)
			}
			if diagnostic.Pin != "" {
				builder.WriteString(" pin ")
				builder.WriteString(diagnostic.Pin)
			}
			if diagnostic.Net != "" {
				builder.WriteString(" net ")
				builder.WriteString(diagnostic.Net)
			}
			if diagnostic.Path != "" {
				builder.WriteString(" at ")
				builder.WriteString(diagnostic.Path)
			}
			if diagnostic.Point != "" {
				builder.WriteString(" ")
				builder.WriteString(diagnostic.Point)
			}
			if diagnostic.Message != "" {
				builder.WriteString(": ")
				builder.WriteString(diagnostic.Message)
			}
		}
	}
	writeDiagnostics("off-grid objects", report.OffGridObjects)
	writeDiagnostics("floating labels", report.FloatingLabels)
	writeDiagnostics("dangling wire endpoints", report.DanglingWireEndpoints)
	writeDiagnostics("disconnected symbol pins", report.DisconnectedSymbolPinAnchors)
	if len(report.Issues) > 0 {
		builder.WriteString("\ngenerated connectivity issues:")
		for _, issue := range report.Issues {
			builder.WriteString("\n- ")
			if issue.Path != "" {
				builder.WriteString(issue.Path)
				builder.WriteString(": ")
			}
			builder.WriteString(issue.Message)
		}
	}
	if len(report.ConnectedComponents) > 0 {
		builder.WriteString("\nconnected components:")
		for _, component := range report.ConnectedComponents {
			fmt.Fprintf(&builder, "\n- #%d points=%d", component.Index, component.PointCount)
			if len(component.Labels) > 0 {
				builder.WriteString(" labels=")
				builder.WriteString(strings.Join(component.Labels, ","))
			}
			if len(component.References) > 0 {
				builder.WriteString(" refs=")
				builder.WriteString(strings.Join(component.References, ","))
			}
		}
	}
	return builder.String()
}

func formatDesignExamplePromotionIssues(issues []PromotionIssue) string {
	if len(issues) == 0 {
		return "(none)"
	}
	var builder strings.Builder
	for _, issue := range issues {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(string(issue.Severity))
		builder.WriteByte(' ')
		builder.WriteString(issue.Code)
		if issue.Stage != "" {
			builder.WriteString(" at ")
			builder.WriteString(string(issue.Stage))
		}
		if issue.Path != "" {
			builder.WriteString(" ")
			builder.WriteString(issue.Path)
		}
		builder.WriteString(": ")
		builder.WriteString(issue.Message)
	}
	return builder.String()
}

func promotionReportHasStageIssue(report PromotionReport, stage StageName, code string) bool {
	want := strings.ToLower(strings.TrimSpace(code))
	for _, issue := range report.Issues {
		got := strings.ToLower(strings.TrimSpace(issue.Code))
		if issue.Stage == stage && (got == want || strings.Contains(got, want)) {
			return true
		}
	}
	return false
}

func designExampleStageByName(result WorkflowResult, name StageName) (StageResult, bool) {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return StageResult{}, false
}

func formatDesignExampleStages(stages []StageResult) string {
	if len(stages) == 0 {
		return "(none)"
	}
	var builder strings.Builder
	for _, stage := range stages {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(string(stage.Name))
		builder.WriteString(": ")
		builder.WriteString(string(stage.Status))
		if len(stage.Issues) != 0 {
			builder.WriteString("\n")
			builder.WriteString(indentDesignExampleText(formatDesignExampleIssues(stage.Issues), "  "))
		}
	}
	return builder.String()
}

func formatDesignExampleRun(metadata designExampleMetadata, outputDir string, result WorkflowResult) string {
	var builder strings.Builder
	builder.WriteString("fixture: ")
	builder.WriteString(metadata.ID)
	builder.WriteString("\nreadiness: ")
	builder.WriteString(metadata.Readiness)
	builder.WriteString("\noutput: ")
	builder.WriteString(outputDir)
	builder.WriteString("\nstages:\n")
	builder.WriteString(indentDesignExampleText(formatDesignExampleStages(result.Stages), "  "))
	return builder.String()
}

func formatDesignExampleStageArtifacts(stage StageResult, outputDir string) string {
	if len(stage.Artifacts) == 0 {
		return "output: " + outputDir + "\nartifacts: (none)"
	}
	var builder strings.Builder
	builder.WriteString("output: ")
	builder.WriteString(outputDir)
	builder.WriteString("\nartifacts:")
	for _, artifact := range stage.Artifacts {
		builder.WriteString("\n  - ")
		builder.WriteString(string(artifact.Kind))
		builder.WriteString(": ")
		builder.WriteString(artifact.Path)
	}
	return builder.String()
}

func designExampleCreateTimeout(t *testing.T) time.Duration {
	t.Helper()
	if value := strings.TrimSpace(os.Getenv("KICADAI_EXAMPLE_TEST_TIMEOUT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			t.Fatalf("KICADAI_EXAMPLE_TEST_TIMEOUT=%q is not a valid duration: %v", value, err)
		}
		return duration
	}
	return time.Minute
}

func indentDesignExampleText(text, prefix string) string {
	if text == "" {
		return text
	}
	trimmed := strings.TrimSuffix(text, "\n")
	return prefix + strings.ReplaceAll(trimmed, "\n", "\n"+prefix)
}
