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

func TestPhysicalPromotionHashIsIndependentOfCleanRoot(t *testing.T) {
	resultAt := func(root string) PhysicalPromotionResult {
		return PhysicalPromotionResult{
			Schema:          PhysicalPromotionSchema,
			Version:         PhysicalPromotionVersion,
			PolicyVersion:   PolicyVersion,
			RequirementHash: "requirement",
			InventoryHash:   "inventory",
			SynthesisHash:   "synthesis",
			PhysicalHash:    "physical",
			Status:          PhysicalPromotionPassed,
			ReplayIdentical: true,
			ProjectHash:     "project",
			Runs: []PhysicalPromotionRun{
				{
					Number:      1,
					ProjectRoot: filepath.Join(root, "run-1"),
					ProjectHash: "project",
					Workflow: designworkflow.WorkflowResult{
						Acceptance: designworkflow.AcceptanceResult{FabricationReady: true},
						Stages: []designworkflow.StageResult{{
							Name:   designworkflow.StageWriterCorrect,
							Status: designworkflow.StageStatusOK,
							Artifacts: []reports.Artifact{{
								Kind: reports.ArtifactRoundTripReport,
								Path: filepath.Join(root, "run-1", ".evidence", "writer.json"),
							}},
						}},
					},
					Artifacts: []reports.Artifact{{
						Kind: reports.ArtifactKiCadProject,
						Path: filepath.Join(root, "run-1", "design.kicad_pro"),
					}},
				},
				{Number: 2, ProjectRoot: filepath.Join(root, "run-2"), ProjectHash: "project"},
			},
			Issues: []reports.Issue{},
		}
	}
	first := finalizePhysicalPromotion(resultAt(filepath.Join(t.TempDir(), "first")))
	second := finalizePhysicalPromotion(resultAt(filepath.Join(t.TempDir(), "second")))
	if first.Hash != second.Hash {
		t.Fatalf("physical promotion hash depends on clean root: %s != %s", first.Hash, second.Hash)
	}
	second.ProjectHash = "different-project"
	second = finalizePhysicalPromotion(second)
	if first.Hash == second.Hash {
		t.Fatal("physical promotion hash ignored project evidence")
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

func TestArchitectureCorpusOptionalKiCadPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 to run installed-KiCad architecture promotion")
	}
	kicadCLI := openTopologyKiCadCLI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	index, _ := libraryresolver.Load(
		ctx,
		libraryresolver.LibraryRoots{
			SymbolsRoot: openTopologyLibraryRoot(
				t, libraryresolver.EnvSymbolsRoot,
				"/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols",
			),
			FootprintsRoot: openTopologyLibraryRoot(
				t, libraryresolver.EnvFootprintsRoot,
				"/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints",
			),
			TemplatesRoot: strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
		},
		libraryresolver.LoadOptions{},
	)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	for _, name := range []string{
		"continuous_conduction_audio_stage.json",
		"efficient_audio_power_stage.json",
		"mains_notch_filter.json",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			policy := DefaultPolicy()
			requirement := testArchitectureRequirement(t, name)
			search := SearchPrimitiveTopologies(ctx, requirement, inventory, policy)
			if len(search.Candidates) < 2 {
				t.Fatalf("architecture search retained %d candidates, want at least 2: status=%s rejections=%#v", len(search.Candidates), search.Status, search.Rejections)
			}
			policy.MaxValueTrials = max(1, len(search.Candidates))
			run := Synthesize(
				ctx, requirement, inventory, environment, policy,
			)
			if run.Report.Status != StatusPassed || run.Physical == nil {
				t.Fatalf("architecture synthesis status=%s stop=%s diagnostics=%#v", run.Report.Status, run.Report.StopReason, run.Report.Diagnostics)
			}
			evaluatedTopologies := map[string]bool{}
			for _, candidate := range run.Candidates {
				if len(candidate.Evaluations) != 0 {
					evaluatedTopologies[candidate.TopologyHash] = true
				}
			}
			if len(evaluatedTopologies) < 2 {
				t.Fatalf("trusted simulation evaluated %d distinct topologies, want at least 2", len(evaluatedTopologies))
			}
			if run.Report.Selected == nil || run.SelectedTrial == nil ||
				strings.TrimSpace(run.Report.Selected.SelectionSummary) == "" ||
				strings.TrimSpace(run.Report.Selected.Ranking.Policy) == "" ||
				len(run.Report.Selected.Ranking.Alternatives) == 0 {
				t.Fatalf("ranked selection evidence is incomplete: %#v", run.Report.Selected)
			}
			selectedPlanFound := false
			for _, candidate := range run.Candidates {
				if candidate.Fingerprint != run.Report.Selected.Fingerprint {
					continue
				}
				selectedPlanFound = true
				assertValueTrialHasExplainableComponents(t, candidate.ValuePlan, *run.SelectedTrial)
			}
			if !selectedPlanFound {
				t.Fatal("selected architecture is not bound to retained component/value evidence")
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
				!promotion.ReplayIdentical || promotion.ProjectHash == "" || len(promotion.Runs) != 2 {
				t.Fatalf("architecture promotion status=%s replay=%t runs=%d issues=%#v", promotion.Status, promotion.ReplayIdentical, len(promotion.Runs), promotion.Issues)
			}
			t.Logf(
				"architecture promotion synthesis=%s topology=%s alternatives=%d physical=%s project=%s evidence=%s",
				run.Hash,
				run.Report.Selected.TopologyHash,
				len(run.Report.Selected.Ranking.Alternatives),
				run.Physical.Hash,
				promotion.ProjectHash,
				promotion.Hash,
			)
		})
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
