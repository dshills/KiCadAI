package opentopologysynthesis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

const openTopologyKiCadPromotionEnv = "KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION"

func TestPhysicalPromotionRejectsUnprovenSynthesis(t *testing.T) {
	first := PromoteSynthesisRun(
		context.Background(),
		SynthesisRun{},
		SimulationEnvironment{},
		PhysicalPromotionOptions{},
	)
	second := PromoteSynthesisRun(
		context.Background(),
		SynthesisRun{},
		SimulationEnvironment{},
		PhysicalPromotionOptions{},
	)
	if first.Status != PhysicalPromotionInvalid ||
		len(first.Issues) != 1 ||
		first.Issues[0].Code != CodePhysicalPromotionFailed ||
		first.Hash == "" ||
		first.Hash != second.Hash {
		t.Fatalf("unproven physical promotion = %#v", first)
	}
}

func TestPhysicalPromotionWorkflowIssuesIgnoreInformationalChecks(t *testing.T) {
	required := []designworkflow.StageName{
		designworkflow.StageSchematic,
		designworkflow.StageSchematicElectrical,
		designworkflow.StagePlacement,
		designworkflow.StageRouting,
		designworkflow.StageProjectWrite,
		designworkflow.StageWriterCorrect,
		designworkflow.StageValidation,
		designworkflow.StageKiCadChecks,
	}
	workflow := designworkflow.WorkflowResult{}
	for _, name := range required {
		workflow.Stages = append(workflow.Stages, designworkflow.StageResult{
			Name:   name,
			Status: designworkflow.StageStatusOK,
		})
	}
	workflow.Stages[0].Issues = []reports.Issue{{
		Code:     reports.CodeSkippedExternalTool,
		Severity: reports.SeverityInfo,
		Message:  "the dedicated installed-KiCad stage owns this check",
	}}
	if issues := physicalPromotionWorkflowIssues(workflow); len(issues) != 0 {
		t.Fatalf("informational workflow issue blocked promotion: %#v", issues)
	}

	workflow.Stages[0].Issues[0].Severity = reports.SeverityError
	if issues := physicalPromotionWorkflowIssues(workflow); len(issues) != 1 {
		t.Fatalf("blocking workflow issue count = %d, want 1: %#v", len(issues), issues)
	}
}

func TestPreparePhysicalPromotionRootCreatesMissingParents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "promotion")
	if err := preparePhysicalPromotionRoot(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		t.Fatalf("physical promotion root = %#v, %v", info, err)
	}
}

func TestFrozenHeldOutCorpusOptionalKiCadPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 to run installed-KiCad promotion")
	}
	kicadCLI := openTopologyKiCadCLI(t)
	symbolsRoot := openTopologyLibraryRoot(
		t,
		libraryresolver.EnvSymbolsRoot,
		"/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols",
	)
	footprintsRoot := openTopologyLibraryRoot(
		t,
		libraryresolver.EnvFootprintsRoot,
		"/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	index, _ := libraryresolver.Load(
		ctx,
		libraryresolver.LibraryRoots{
			SymbolsRoot:    symbolsRoot,
			FootprintsRoot: footprintsRoot,
			TemplatesRoot: strings.TrimSpace(
				os.Getenv(libraryresolver.EnvTemplatesRoot),
			),
		},
		libraryresolver.LoadOptions{},
	)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 256
	policy.MaxTopologyRepairs = 32
	policy.MaxCandidateSimulations = 50_000
	policy.MaxCornerEvaluations = 200_000
	passed := 0
	for _, name := range testHeldOutRequirementNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			requirement := testOpenTopologyRequirement(t, name)
			run := Synthesize(
				ctx,
				requirement,
				inventory,
				environment,
				policy,
			)
			if run.Report.Status != StatusPassed {
				t.Logf(
					"stable non-pass synthesis_hash=%s status=%s stop=%s consumption=%#v diagnostics=%#v",
					run.Hash,
					run.Report.Status,
					run.Report.StopReason,
					run.Report.Consumption,
					run.Report.Diagnostics,
				)
				return
			}
			outputRoot := t.TempDir()
			if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
				outputRoot = filepath.Join(retained, strings.TrimSuffix(name, filepath.Ext(name)))
			}
			promotion := PromoteSynthesisRun(
				ctx,
				run,
				environment,
				PhysicalPromotionOptions{
					OutputRoot:    outputRoot,
					KiCadCLI:      kicadCLI,
					LibraryIndex:  &index,
					Timeout:       2 * time.Minute,
					KeepArtifacts: true,
				},
			)
			if promotion.Status != PhysicalPromotionPassed ||
				!promotion.ReplayIdentical ||
				promotion.ProjectHash == "" ||
				len(promotion.Runs) != 2 {
				t.Fatalf(
					"physical promotion = status=%s replay=%t runs=%d issues=%#v",
					promotion.Status,
					promotion.ReplayIdentical,
					len(promotion.Runs),
					promotion.Issues,
				)
			}
			t.Logf(
				"synthesis_hash=%s topology_hash=%s physical_hash=%s project_hash=%s",
				run.Hash,
				run.Report.Selected.TopologyHash,
				run.Physical.Hash,
				promotion.ProjectHash,
			)
			passed++
		})
	}
	if passed < 6 {
		t.Fatalf(
			"installed-KiCad held-out promotions = %d, want at least 6 of 8",
			passed,
		)
	}
}

func openTopologyKiCadCLI(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("KICADAI_KICAD_CLI"))
	if path == "" {
		path = "/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli"
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		t.Fatalf("installed KiCad CLI is unavailable at %s", path)
	}
	return path
}

func openTopologyLibraryRoot(
	t *testing.T,
	name string,
	fallback string,
) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(name))
	if path == "" {
		path = fallback
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf(
			"installed KiCad library root %s is unavailable at %s",
			name,
			path,
		)
	}
	return path
}
