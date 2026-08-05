package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/simmodel"
)

const (
	nonlinearSwitchingPromotionEnv = "KICADAI_NONLINEAR_SWITCHING_PROMOTION"
	nonlinearSwitchingCaseEnv      = "KICADAI_NONLINEAR_SWITCHING_CASE"
)

func nonlinearSwitchingPromotionPolicy() Policy {
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384
	return policy
}

func TestNonlinearSwitchingCorpusPromotion(t *testing.T) {
	if os.Getenv(nonlinearSwitchingPromotionEnv) != "1" {
		t.Skip("set " + nonlinearSwitchingPromotionEnv + "=1 to run nonlinear/switching corpus promotion")
	}
	var manifest nonlinearSwitchingCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(nonlinearSwitchingCorpusRoot(), "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := nonlinearSwitchingPromotionPolicy()
	executed := 0
	for _, entry := range manifest.DesignCases {
		entry := entry
		if target := strings.TrimSpace(os.Getenv(nonlinearSwitchingCaseEnv)); target != "" && target != entry.ID {
			continue
		}
		t.Run(entry.ID, func(t *testing.T) {
			executed++
			requirement := testNonlinearSwitchingRequirement(t, entry.RequirementFile)
			first := Synthesize(context.Background(), requirement, inventory, environment, policy)
			assertNonlinearSwitchingDesignPass(t, requirement, first)
			second := Synthesize(context.Background(), requirement, inventory, environment, policy)
			assertNonlinearSwitchingDesignPass(t, requirement, second)
			assertNonlinearSwitchingReplay(t, first, second)
			assertSynthesisConsumptionMatchesEvidence(t, first)
		})
	}
	for _, entry := range manifest.AdversarialCases {
		entry := entry
		if target := strings.TrimSpace(os.Getenv(nonlinearSwitchingCaseEnv)); target != "" && target != entry.ID {
			continue
		}
		t.Run(entry.ID, func(t *testing.T) {
			executed++
			requirement := testNonlinearSwitchingRequirement(t, entry.RequirementFile)
			first := Synthesize(context.Background(), requirement, inventory, environment, policy)
			second := Synthesize(context.Background(), requirement, inventory, environment, policy)
			assertNonlinearSwitchingReplay(t, first, second)
			assertNonlinearSwitchingAdversarialOutcome(t, entry, first)
			assertSynthesisConsumptionMatchesEvidence(t, first)
		})
	}
	if executed == 0 {
		t.Fatal("nonlinear/switching promotion filter selected no frozen case")
	}
}

func TestNonlinearSwitchingCorpusOptionalKiCadPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set " + openTopologyKiCadPromotionEnv + "=1 to run installed-KiCad nonlinear/switching promotion")
	}
	kicadCLI := openTopologyKiCadCLI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	index, _ := libraryresolver.Load(
		ctx,
		libraryresolver.LibraryRoots{
			SymbolsRoot:    openTopologyLibraryRoot(t, libraryresolver.EnvSymbolsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols"),
			FootprintsRoot: openTopologyLibraryRoot(t, libraryresolver.EnvFootprintsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints"),
			TemplatesRoot:  strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
		},
		libraryresolver.LoadOptions{},
	)
	var manifest nonlinearSwitchingCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(nonlinearSwitchingCorpusRoot(), "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := nonlinearSwitchingPromotionPolicy()
	executed := 0
	for _, entry := range manifest.DesignCases {
		entry := entry
		if target := strings.TrimSpace(os.Getenv(nonlinearSwitchingCaseEnv)); target != "" && target != entry.ID {
			continue
		}
		t.Run(entry.ID, func(t *testing.T) {
			executed++
			requirement := testNonlinearSwitchingRequirement(t, entry.RequirementFile)
			run := Synthesize(ctx, requirement, inventory, environment, policy)
			assertNonlinearSwitchingDesignPass(t, requirement, run)
			outputRoot := t.TempDir()
			if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
				outputRoot = filepath.Join(retained, entry.ID)
			}
			promotion := PromoteSynthesisRun(ctx, run, environment, PhysicalPromotionOptions{
				OutputRoot: outputRoot, KiCadCLI: kicadCLI, LibraryIndex: &index,
				Timeout: 3 * time.Minute, KeepArtifacts: true,
			})
			if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical ||
				promotion.ProjectHash == "" || len(promotion.Runs) != 2 || len(promotion.Issues) != 0 {
				t.Fatalf("nonlinear/switching KiCad promotion status=%s replay=%t project=%s runs=%d issues=%#v board=%#v components=%v routing=%#v", promotion.Status, promotion.ReplayIdentical, promotion.ProjectHash, len(promotion.Runs), promotion.Issues, run.Physical.Document.Project.Board, nonlinearSwitchingPhysicalComponents(run.Physical.Bindings), nonlinearSwitchingRoutingSummary(promotion.Runs))
			}
			t.Logf("nonlinear/switching promotion synthesis=%s physical=%s project=%s evidence=%s", run.Hash, run.Physical.Hash, promotion.ProjectHash, promotion.Hash)
		})
	}
	if executed == 0 {
		t.Fatal("nonlinear/switching KiCad promotion filter selected no frozen design case")
	}
}

func nonlinearSwitchingPhysicalComponents(bindings []PhysicalSemanticBinding) []string {
	components := []string{}
	for _, binding := range bindings {
		if binding.Component == "" || binding.CatalogID == "" {
			continue
		}
		components = append(components, binding.Component+"="+binding.CatalogID+"/"+binding.VariantID)
	}
	slices.Sort(components)
	return components
}

func nonlinearSwitchingRoutingSummary(runs []PhysicalPromotionRun) map[string]any {
	if len(runs) == 0 {
		return nil
	}
	for _, stage := range runs[0].Workflow.Stages {
		if string(stage.Name) == "routing" {
			return stage.Summary
		}
	}
	return nil
}

func testNonlinearSwitchingRequirement(t *testing.T, file string) Requirement {
	t.Helper()
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(nonlinearSwitchingCorpusRoot(), file))))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	return requirement
}

func assertNonlinearSwitchingReplay(t *testing.T, first, second SynthesisRun) {
	t.Helper()
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash == "" || first.Hash != second.Hash || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("nonlinear/switching replay differs first=%s second=%s", first.Hash, second.Hash)
	}
}

func assertNonlinearSwitchingDesignPass(t *testing.T, requirement Requirement, run SynthesisRun) {
	t.Helper()
	if run.Report.Status != StatusPassed || run.Report.StopReason != StopPassed ||
		run.Report.Selected == nil || run.SelectedGraph == nil || run.SelectedTrial == nil ||
		run.Physical == nil || run.Physical.Status != PhysicalLoweringReady ||
		len(run.Report.Diagnostics) != 0 {
		t.Fatalf("nonlinear/switching design status=%s stop=%s selected=%t physical=%#v diagnostics=%#v", run.Report.Status, run.Report.StopReason, run.Report.Selected != nil, run.Physical, run.Report.Diagnostics)
	}
	evaluation, found := nonlinearSwitchingSelectedEvaluation(run)
	if !found || evaluation.Status != SimulationEvaluationPassed {
		t.Fatalf("selected simulation evaluation is absent or not passing: %#v", evaluation)
	}
	wantAttempts := map[string]bool{}
	operatingCases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		operatingCases[operatingCase.ID] = operatingCase
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		for _, caseID := range assertion.OperatingCases {
			operatingCase := operatingCases[caseID]
			operatingCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
			for _, corner := range operatingCaseCorners(operatingCase) {
				wantAttempts[assertion.ID+"\x00"+caseID+"\x00"+corner.ID] = true
			}
		}
	}
	for _, attempt := range evaluation.Attempts {
		key := attempt.RequirementID + "\x00" + attempt.OperatingCase + "\x00" + attempt.CornerID
		if !wantAttempts[key] || attempt.Status != SimulationEvaluationPassed || !attempt.AssertionPass ||
			attempt.PlanHash == "" || attempt.ReportHash == "" || len(attempt.ModelEvidenceSHA256s) == 0 || attempt.Report == nil {
			t.Fatalf("invalid or unexpected selected simulation attempt %q: %#v", key, attempt)
		}
		delete(wantAttempts, key)
		if trustedModelAnalysisKind(attempt.Analysis) == simmodel.AnalysisTransient ||
			trustedModelAnalysisKind(attempt.Analysis) == simmodel.AnalysisDistortion ||
			trustedModelAnalysisKind(attempt.Analysis) == simmodel.AnalysisElectrothermal {
			assertNonlinearSwitchingConvergenceEvidence(t, attempt)
		}
	}
	if len(wantAttempts) != 0 {
		t.Fatalf("selected simulation omitted exhaustive requirement corners: %v", wantAttempts)
	}
}

func nonlinearSwitchingSelectedEvaluation(run SynthesisRun) (SimulationEvaluation, bool) {
	if run.Report.Selected == nil {
		return SimulationEvaluation{}, false
	}
	for _, candidate := range run.Candidates {
		for _, evaluation := range candidate.Evaluations {
			if evaluation.Hash == run.Report.Selected.EvaluationHash {
				return evaluation, true
			}
		}
	}
	return SimulationEvaluation{}, false
}

func assertNonlinearSwitchingConvergenceEvidence(t *testing.T, attempt SimulationAttempt) {
	t.Helper()
	if attempt.Report == nil || len(attempt.Report.Analyses) == 0 {
		t.Fatalf("dynamic attempt lacks analysis evidence: %#v", attempt)
	}
	for _, analysis := range attempt.Report.Analyses {
		if len(analysis.Points) < 2 {
			t.Fatalf("dynamic analysis %s has %d points", analysis.ID, len(analysis.Points))
		}
		for _, point := range analysis.Points {
			evidence := point.Solver
			zeroEnergyInitial := evidence != nil && evidence.Method == "zero_energy_transient_v1" && evidence.InitialCondition != ""
			if evidence == nil || evidence.Method == "" || (!zeroEnergyInitial && evidence.Iterations <= 0) ||
				evidence.MaxIterationsPerStep <= 0 || evidence.MaxTotalIterations <= 0 ||
				evidence.TotalIterations > evidence.MaxTotalIterations ||
				math.IsNaN(evidence.FinalMaxUpdateV) || math.IsNaN(evidence.FinalMaxCurrentUpdateA) || math.IsNaN(evidence.FinalMaxResidual) {
				t.Fatalf("dynamic analysis %s has incomplete bounded convergence evidence at %.12g s: %#v", analysis.ID, point.TimeS, evidence)
			}
		}
	}
}

func assertNonlinearSwitchingAdversarialOutcome(t *testing.T, entry nonlinearSwitchingAdversarialCase, run SynthesisRun) {
	t.Helper()
	if run.Report.Status == StatusPassed || run.Report.Selected != nil || run.SelectedGraph != nil || run.Physical != nil {
		t.Fatalf("adversarial case %s emitted a selected physical design: status=%s selected=%t physical=%t", entry.ID, run.Report.Status, run.Report.Selected != nil, run.Physical != nil)
	}
	safetyRejected := false
	capabilityGap := run.Report.Status == StatusUnsupported || run.Report.StopReason == StopModelUnavailable || run.Report.StopReason == StopPrimitiveUnavailable
	for _, candidate := range run.Candidates {
		for _, rejection := range candidate.ValuePlan.Rejections {
			if rejection.Code == "rating_envelope" {
				safetyRejected = true
			}
		}
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				switch diagnosis.Code {
				case diagnosisAssertionBelowMinimum, diagnosisAssertionAboveMaximum, diagnosisThermalUnavailable:
					safetyRejected = true
				case diagnosisMetricUnsupported, diagnosisModelUnavailable:
					capabilityGap = true
				}
			}
		}
	}
	for _, diagnostic := range run.Report.Diagnostics {
		if diagnostic.Code == CodeModelUnavailable || diagnostic.Code == CodePrimitiveUnavailable ||
			diagnostic.Code == CodeValueExhausted {
			capabilityGap = true
		}
	}
	switch entry.ExpectedFailureKind {
	case "unsafe_thermal_soa":
		if !safetyRejected {
			t.Fatalf("unsafe case %s lacks thermal/SOA or rated-envelope rejection evidence: status=%s diagnostics=%#v", entry.ID, run.Report.Status, run.Report.Diagnostics)
		}
	case "unsupported_dynamic_envelope":
		if !capabilityGap {
			plans := make([]ValueSearchPlan, 0, len(run.Candidates))
			for _, candidate := range run.Candidates {
				plans = append(plans, candidate.ValuePlan)
			}
			t.Fatalf("unsupported case %s lacks a stable capability-gap diagnostic: status=%s stop=%s diagnostics=%#v value_plans=%#v", entry.ID, run.Report.Status, run.Report.StopReason, run.Report.Diagnostics, plans)
		}
	default:
		t.Fatalf("unknown adversarial failure kind %q", entry.ExpectedFailureKind)
	}
}
