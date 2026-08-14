package opentopologysynthesis

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
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

func TestPhysicalPromotionScopesInstalledLibraryDiagnosticsToSelectedDesign(t *testing.T) {
	selectedSymbolPath := "/symbols/Device.kicad_sym"
	index := libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {
				LibraryID: "Device:R", Path: selectedSymbolPath,
			},
			"Converter_DCDC:Broken": {
				LibraryID: "Converter_DCDC:Broken",
				Path:      "/symbols/Converter_DCDC.kicad_sym",
			},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0603_1608Metric": {
				FootprintID: "Resistor_SMD:R_0603_1608Metric",
				Path:        "/footprints/Resistor_SMD.pretty/R_0603_1608Metric.kicad_mod",
			},
		},
		Diagnostics: []reports.Issue{{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
			Path: "library.symbol.Converter_DCDC:Broken", Message: "unrelated symbol defect",
		}},
	}
	run := physicalPromotionLibraryClosureTestRun()
	scoped, issues := physicalPromotionLibraryIndex(run, index)
	if len(issues) != 0 || len(scoped.Diagnostics) != 0 {
		t.Fatalf("unrelated diagnostics blocked selected design: %#v", issues)
	}

	index.Diagnostics = append(index.Diagnostics, reports.Issue{
		Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning,
		Path: selectedSymbolPath, Message: "selected symbol defect",
	})
	scoped, issues = physicalPromotionLibraryIndex(run, index)
	if len(issues) != 1 || !issues[0].Blocking() || len(scoped.Diagnostics) != 1 {
		t.Fatalf("selected diagnostics did not fail closed: %#v", issues)
	}

	catalog := &components.Catalog{}
	run.Report = Report{
		Status: StatusPassed, Selected: &SelectedResult{}, RequirementHash: "requirement",
		PrimitiveInventoryHash: "inventory", CatalogHash: "catalog",
	}
	run.SelectedGraph = &CandidateGraph{}
	run.Hash = "synthesis"
	promotion := PromoteSynthesisRun(
		context.Background(),
		run,
		SimulationEnvironment{Catalog: catalog, CatalogHash: "catalog"},
		PhysicalPromotionOptions{
			OutputRoot: t.TempDir(), KiCadCLI: "unused", LibraryIndex: &index,
		},
	)
	if promotion.Status != PhysicalPromotionFailed || len(promotion.Issues) != 1 ||
		!promotion.Issues[0].Blocking() || len(promotion.Runs) != 0 {
		t.Fatalf("selected library defect promotion = %#v", promotion)
	}
}

func physicalPromotionLibraryClosureTestRun() SynthesisRun {
	return SynthesisRun{Physical: &PhysicalLoweringResult{
		Status: PhysicalLoweringReady, Hash: "physical",
		Resolved: circuitgraph.ResolvedDocument{Components: []circuitgraph.ResolvedComponent{{
			ComponentID: "resistor", VariantID: "0603", SymbolID: "Device:R",
			FootprintID: "Resistor_SMD:R_0603_1608Metric",
			Units:       []circuitgraph.ResolvedUnit{{Unit: 1, SymbolID: "Device:R"}},
			Functions: []circuitgraph.ResolvedFunction{
				{SymbolID: "Device:R", Unit: 1, SymbolPin: "1", Pad: "1"},
				{SymbolID: "Device:R", Unit: 1, SymbolPin: "2", Pad: "2"},
			},
		}}},
	}}
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

func TestProtectedCurrentOutputCorpusOptionalKiCadPromotion(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
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
	root := protectedCurrentOutputCorpusRoot()
	var manifest protectedCurrentOutputCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(root, "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := protectedCurrentOutputSynthesisPolicy()
	executed := 0
	for _, entry := range manifest.Cases {
		if target := os.Getenv(protectedCurrentOutputCaseEnv); target != "" && target != entry.ID {
			continue
		}
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			executed++
			requirement := testProtectedCurrentOutputRequirement(t, root, entry)
			run := Synthesize(ctx, requirement, inventory, environment, policy)
			assertProtectedCurrentOutputPass(t, run)
			outputRoot := t.TempDir()
			if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
				outputRoot = filepath.Join(retained, entry.ID)
			}
			promotion := PromoteSynthesisRun(
				ctx,
				run,
				environment,
				PhysicalPromotionOptions{
					OutputRoot: outputRoot, KiCadCLI: kicadCLI, LibraryIndex: &index,
					Timeout: 2 * time.Minute, KeepArtifacts: true,
				},
			)
			if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical ||
				promotion.ProjectHash == "" || len(promotion.Runs) != 2 {
				t.Fatalf(
					"protected current-output physical promotion = status=%s replay=%t runs=%d issues=%#v",
					promotion.Status, promotion.ReplayIdentical, len(promotion.Runs), promotion.Issues,
				)
			}
			t.Logf(
				"synthesis_hash=%s topology_hash=%s physical_hash=%s project_hash=%s",
				run.Hash, run.Report.Selected.TopologyHash, run.Physical.Hash, promotion.ProjectHash,
			)
		})
	}
	if executed == 0 {
		t.Fatal("protected current-output KiCad promotion filter selected no frozen case")
	}
}

func TestProtectedVoltageOutputCorpusOptionalKiCadPromotion(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	index, _ := libraryresolver.Load(
		ctx,
		libraryresolver.LibraryRoots{
			SymbolsRoot: symbolsRoot, FootprintsRoot: footprintsRoot,
			TemplatesRoot: strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
		},
		libraryresolver.LoadOptions{},
	)
	root := protectedVoltageOutputCorpusRoot()
	var manifest protectedVoltageOutputCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(root, "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := protectedVoltageOutputSynthesisPolicy()
	executed := 0
	for _, entry := range manifest.Cases {
		if target := os.Getenv(protectedVoltageOutputCaseEnv); target != "" && target != entry.ID {
			continue
		}
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			executed++
			requirement := testProtectedVoltageOutputRequirement(t, entry.RequirementFile)
			run := Synthesize(ctx, requirement, inventory, environment, policy)
			assertProtectedVoltageOutputPass(t, run)
			outputRoot := t.TempDir()
			if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
				outputRoot = filepath.Join(retained, entry.ID)
			}
			promotion := PromoteSynthesisRun(
				ctx,
				run,
				environment,
				PhysicalPromotionOptions{
					OutputRoot: outputRoot, KiCadCLI: kicadCLI, LibraryIndex: &index,
					Timeout: 2 * time.Minute, KeepArtifacts: true,
				},
			)
			if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical ||
				promotion.ProjectHash == "" || len(promotion.Runs) != 2 {
				t.Fatalf(
					"protected voltage-output physical promotion = status=%s replay=%t runs=%d issues=%#v",
					promotion.Status, promotion.ReplayIdentical, len(promotion.Runs), promotion.Issues,
				)
			}
			t.Logf(
				"synthesis_hash=%s topology_hash=%s physical_hash=%s project_hash=%s",
				run.Hash, run.Report.Selected.TopologyHash, run.Physical.Hash, promotion.ProjectHash,
			)
		})
	}
	if executed == 0 {
		t.Fatal("protected voltage-output KiCad promotion filter selected no frozen case")
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
	passed, executed := 0, 0
	for _, name := range testHeldOutRequirementNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			executed++
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
	if executed == 0 {
		t.Skip("no held-out promotion subtests matched the test filter")
	}
	want := min(6, executed)
	if passed < want {
		t.Fatalf(
			"installed-KiCad held-out promotions = %d, want at least %d of %d executed cases",
			passed,
			want,
			executed,
		)
	}
}

func TestMultiBranchAnalogNeutralCorpusOptionalKiCadPromotion(t *testing.T) {
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
			TemplatesRoot:  strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
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

	for _, name := range []string{
		"outside_window_supply_guard.json",
		"precision_low_voltage_rail.json",
	} {
		t.Run(name, func(t *testing.T) {
			requirement := testMultiBranchAnalogRequirement(t, name)
			run := Synthesize(ctx, requirement, inventory, environment, policy)
			if run.Report.Status != StatusPassed {
				t.Fatalf(
					"neutral synthesis = status=%s stop=%s consumption=%#v diagnostics=%#v",
					run.Report.Status,
					run.Report.StopReason,
					run.Report.Consumption,
					run.Report.Diagnostics,
				)
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
		})
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

func TestArchitectureGeneralizationCorpusOptionalKiCadPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 to run installed-KiCad architecture-generalization promotion")
	}
	kicadCLI := openTopologyKiCadCLI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
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
	var manifest generalizationCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(architectureGeneralizationCorpusRoot(), "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384
	passed, executed := 0, 0
	for _, entry := range manifest.DesignCases {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			executed++
			requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
				t,
				filepath.Join(architectureGeneralizationCorpusRoot(), entry.RequirementFile),
			)))
			if len(issues) != 0 {
				t.Fatalf("requirement decode issues: %#v", issues)
			}
			first := Synthesize(ctx, requirement, inventory, environment, policy)
			second := Synthesize(ctx, requirement, inventory, environment, policy)
			if first.Hash == "" || first.Hash != second.Hash {
				t.Fatalf("synthesis replay differs first=%s second=%s", first.Hash, second.Hash)
			}
			if first.Report.Status != StatusPassed {
				if first.Physical != nil || len(first.Report.Diagnostics) == 0 {
					t.Fatalf("non-pass design did not fail closed: status=%s physical=%t diagnostics=%#v", first.Report.Status, first.Physical != nil, first.Report.Diagnostics)
				}
				t.Logf("stable unsupported design hash=%s status=%s", first.Hash, first.Report.Status)
				return
			}
			outputRoot := t.TempDir()
			if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
				outputRoot = filepath.Join(retained, entry.ID)
			}
			promotion := PromoteSynthesisRun(
				ctx,
				first,
				environment,
				PhysicalPromotionOptions{
					OutputRoot:    outputRoot,
					KiCadCLI:      kicadCLI,
					LibraryIndex:  &index,
					Timeout:       3 * time.Minute,
					KeepArtifacts: true,
				},
			)
			if promotion.Status != PhysicalPromotionPassed ||
				!promotion.ReplayIdentical || promotion.ProjectHash == "" || len(promotion.Runs) != 2 {
				t.Fatalf(
					"architecture-generalization promotion status=%s replay=%t runs=%d issues=%#v",
					promotion.Status,
					promotion.ReplayIdentical,
					len(promotion.Runs),
					promotion.Issues,
				)
			}
			t.Logf(
				"architecture-generalization promotion synthesis=%s topology=%s physical=%s project=%s evidence=%s",
				first.Hash,
				first.Report.Selected.TopologyHash,
				first.Physical.Hash,
				promotion.ProjectHash,
				promotion.Hash,
			)
			passed++
		})
	}
	if executed == 0 {
		t.Skip("no architecture-generalization promotion subtests matched the test filter")
	}
	want := min(5, executed)
	if passed < want {
		t.Fatalf("installed-KiCad architecture-generalization promotions=%d, want at least %d of %d executed cases", passed, want, executed)
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
