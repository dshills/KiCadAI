package opentopologysynthesis

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
)

const multiStageOODCaseEnv = "KICADAI_MULTI_STAGE_OOD_CASE"

func multiStageOODPromotionPolicy() Policy {
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

func TestMultiStageOODAmbientControlOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "ambient_tracking_airflow_control")
}

func TestMultiStageOODUndervoltageDisconnectOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "undervoltage_load_permission")
}

func TestMultiStageOODWindowedHeatingOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "windowed_heating_power_control")
}

func TestMultiStageOODEnabledCurrentRegulationOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "enabled_current_regulation")
}

func TestMultiStageOODBipolarMagnitudeOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "bipolar_magnitude_fault_indicator")
}

func TestMultiStageOODBoundedAudioPowerOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "bounded_audio_power_transfer")
}

func TestMultiStageOODIlluminationPowerOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODInfeasibleRequirement(t, "illumination_proportional_power_control")
}

func TestMultiStageOODInductiveLoadCurrentOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "inductive_load_current_control")
}

func TestMultiStageOODLowVoltageSoftStartOptionalKiCadPromotion(t *testing.T) {
	testMultiStageOODOptionalKiCadPromotion(t, "low_voltage_power_with_soft_start")
}

func TestMultiStageOODAdversarialFailClosedPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set " + openTopologyKiCadPromotionEnv + "=1 to run multi-stage adversarial promotion")
	}
	var manifest multiStageOODCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "manifest.json")), &manifest)
	target := strings.TrimSpace(os.Getenv(multiStageOODCaseEnv))
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	ran := 0
	for _, entry := range manifest.AdversarialCases {
		if target != "" && target != entry.ID {
			continue
		}
		ran++
		t.Run(entry.ID, func(t *testing.T) {
			requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
				t, filepath.Join(multiStageOODCorpusRoot(), entry.RequirementFile),
			)))
			if len(issues) != 0 {
				t.Fatalf("requirement decode issues: %#v", issues)
			}
			first := Synthesize(context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy())
			second := Synthesize(context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy())
			if first.Report.Status == StatusPassed || first.Report.Selected != nil ||
				first.SelectedGraph != nil || first.Physical != nil || len(first.Report.Diagnostics) == 0 {
				t.Fatalf(
					"adversarial design did not fail closed: status=%s stop=%s selected=%t physical=%t diagnostics=%#v",
					first.Report.Status, first.Report.StopReason, first.Report.Selected != nil,
					first.Physical != nil, first.Report.Diagnostics,
				)
			}
			if first.Hash == "" || first.Hash != second.Hash || !reflect.DeepEqual(first, second) {
				t.Fatalf("adversarial replay differs: first=%s second=%s", first.Hash, second.Hash)
			}
			safetyEvidence, capabilityEvidence := multiStageOODAdversarialEvidence(first)
			switch entry.ExpectedFailureKind {
			case "unsafe_thermal_soa":
				if !safetyEvidence {
					t.Fatalf("unsafe adversarial result lacks thermal/SOA evidence: status=%s stop=%s", first.Report.Status, first.Report.StopReason)
				}
			case "unsupported_dynamic_envelope", "unsupported_high_energy_domain":
				if !capabilityEvidence {
					t.Fatalf("unsupported adversarial result lacks capability-gap evidence: status=%s stop=%s", first.Report.Status, first.Report.StopReason)
				}
			default:
				t.Fatalf("unknown adversarial failure kind %q", entry.ExpectedFailureKind)
			}
			assertSynthesisConsumptionMatchesEvidence(t, first)
			t.Logf(
				"multi-stage adversarial %s stable fail-closed hash=%s status=%s stop=%s diagnostic=%s",
				entry.ID, first.Hash, first.Report.Status, first.Report.StopReason,
				first.Report.Diagnostics[0].Code,
			)
		})
	}
	if ran == 0 {
		t.Skip("multi-stage promotion filter excludes adversarial cases")
	}
}

func multiStageOODAdversarialEvidence(run SynthesisRun) (bool, bool) {
	safety, capability := false, false
	for _, diagnostic := range run.Report.Diagnostics {
		switch diagnostic.Code {
		case CodeModelUnavailable, CodePrimitiveUnavailable, CodeValueExhausted:
			capability = true
		case CodeRequirementInfeasible:
			safety = true
		}
	}
	for _, candidate := range run.Candidates {
		for _, rejection := range candidate.ValuePlan.Rejections {
			if rejection.Code == "rating_envelope" {
				safety = true
			}
		}
		for _, issue := range candidate.ValuePlan.Issues {
			if issue.Code == CodeModelUnavailable || issue.Code == CodePrimitiveUnavailable || issue.Code == CodeValueExhausted {
				capability = true
			}
		}
		for _, evaluation := range candidate.Evaluations {
			for _, issue := range evaluation.Issues {
				if issue.Code == CodeModelUnavailable || issue.Code == CodePrimitiveUnavailable || issue.Code == CodeValueExhausted {
					capability = true
				}
			}
			for _, diagnosis := range evaluation.Diagnoses {
				switch diagnosis.Code {
				case diagnosisMetricUnsupported, diagnosisModelUnavailable, diagnosisThermalUnavailable:
					capability = true
				case diagnosisAssertionBelowMinimum, diagnosisAssertionAboveMaximum, diagnosisSimulationInvalid:
					message := strings.ToLower(diagnosis.RequirementID + " " + diagnosis.Metric + " " + diagnosis.Message)
					if strings.Contains(message, "thermal") || strings.Contains(message, "temperature") ||
						strings.Contains(message, "soa") || strings.Contains(message, "safe operating") ||
						strings.Contains(message, "catalog-backed limit") {
						safety = true
					}
				}
			}
		}
	}
	return safety, capability
}

func testMultiStageOODOptionalKiCadPromotion(t *testing.T, caseName string) {
	t.Helper()
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set " + openTopologyKiCadPromotionEnv + "=1 to run installed-KiCad multi-stage promotion")
	}
	if target := strings.TrimSpace(os.Getenv(multiStageOODCaseEnv)); target != "" && target != caseName {
		t.Skip("multi-stage promotion filter excludes " + caseName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), caseName+".json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := multiStageOODPromotionPolicy()
	first := Synthesize(ctx, requirement, inventory, environment, policy)
	assertNonlinearSwitchingDesignPass(t, requirement, first)
	second := Synthesize(ctx, requirement, inventory, environment, policy)
	assertNonlinearSwitchingDesignPass(t, requirement, second)
	assertNonlinearSwitchingReplay(t, first, second)
	assertSynthesisConsumptionMatchesEvidence(t, first)

	index, _ := libraryresolver.Load(
		ctx,
		libraryresolver.LibraryRoots{
			SymbolsRoot:    openTopologyLibraryRoot(t, libraryresolver.EnvSymbolsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols"),
			FootprintsRoot: openTopologyLibraryRoot(t, libraryresolver.EnvFootprintsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints"),
			TemplatesRoot:  strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
		},
		libraryresolver.LoadOptions{},
	)
	outputRoot := t.TempDir()
	if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
		outputRoot = filepath.Join(retained, caseName)
	}
	promotion := PromoteSynthesisRun(ctx, first, environment, PhysicalPromotionOptions{
		OutputRoot:    outputRoot,
		KiCadCLI:      openTopologyKiCadCLI(t),
		LibraryIndex:  &index,
		Timeout:       3 * time.Minute,
		KeepArtifacts: true,
	})
	if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical ||
		promotion.ProjectHash == "" || len(promotion.Runs) != 2 || len(promotion.Issues) != 0 {
		for _, stage := range promotionRunStages(promotion.Runs) {
			if stage.Name == designworkflow.StageKiCadChecks || stage.Name == designworkflow.StageWriterCorrect {
				t.Logf("multi-stage %s status=%s summary=%#v issues=%#v", stage.Name, stage.Status, stage.Summary, stage.Issues)
			}
		}
		placed := designworkflow.PlaceExplicitCircuit(
			ctx,
			first.Physical.DesignRequest,
			designworkflow.PlacementOptions{LibraryIndex: &index},
		)
		t.Logf(
			"multi-stage placement diagnostics status=%s stage=%#v metrics=%#v scoring=%#v",
			placed.Result.Status, placed.Stage.Summary, placed.Result.Metrics, placed.Result.CandidateScoring,
		)
		t.Logf(
			"multi-stage placement results=%#v regions=%#v",
			placed.Result.Placements, placed.Request.RegionRules,
		)
		t.Fatalf(
			"multi-stage KiCad promotion status=%s replay=%t project=%s runs=%d issues=%#v stages=%#v board=%#v components=%v routing=%#v",
			promotion.Status,
			promotion.ReplayIdentical,
			promotion.ProjectHash,
			len(promotion.Runs),
			promotion.Issues,
			promotionRunStages(promotion.Runs),
			first.Physical.Document.Project.Board,
			nonlinearSwitchingPhysicalComponents(first.Physical.Bindings),
			nonlinearSwitchingRoutingSummary(promotion.Runs),
		)
	}
	assertMultiStageReturnPathEvidence(t, first.Physical.DesignRequest, promotion.Runs)
	t.Logf(
		"multi-stage %s promotion synthesis=%s physical=%s project=%s evidence=%s",
		caseName,
		first.Hash,
		first.Physical.Hash,
		promotion.ProjectHash,
		promotion.Hash,
	)
}

// testMultiStageOODInfeasibleRequirement retains the optional promotion entry
// point for a frozen positive whose immutable behavior envelope was later
// proven physically contradictory. A fail-closed result before topology search
// is the only safe promotion outcome, so KiCad must never receive a project.
func testMultiStageOODInfeasibleRequirement(t *testing.T, caseName string) {
	t.Helper()
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set " + openTopologyKiCadPromotionEnv + "=1 to run installed-KiCad multi-stage promotion")
	}
	if target := strings.TrimSpace(os.Getenv(multiStageOODCaseEnv)); target != "" && target != caseName {
		t.Skip("multi-stage promotion filter excludes " + caseName)
	}
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), caseName+".json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := multiStageOODPromotionPolicy()
	first := Synthesize(context.Background(), requirement, inventory, environment, policy)
	second := Synthesize(context.Background(), requirement, inventory, environment, policy)
	if first.Report.Status != StatusInfeasible || first.Report.StopReason != StopRequirementInfeasible ||
		first.Report.Selected != nil || first.SelectedGraph != nil || first.Physical != nil ||
		first.Search.Schema != "" || len(first.Report.Diagnostics) != 1 ||
		first.Report.Diagnostics[0].Code != CodeRequirementInfeasible {
		t.Fatalf("multi-stage infeasible requirement did not fail closed before search: %#v", first)
	}
	if first.Hash == "" || first.Hash != second.Hash || !reflect.DeepEqual(first, second) {
		t.Fatalf("multi-stage infeasible replay differs: first=%s second=%s", first.Hash, second.Hash)
	}
	assertSynthesisConsumptionMatchesEvidence(t, first)
	t.Logf(
		"multi-stage %s stable fail-closed hash=%s diagnostic=%s",
		caseName, first.Hash, first.Report.Diagnostics[0].Message,
	)
}

func assertMultiStageReturnPathEvidence(
	t *testing.T,
	request designworkflow.Request,
	runs []PhysicalPromotionRun,
) {
	t.Helper()
	if request.Board.Layers != 4 || request.ExplicitCircuit == nil {
		return
	}
	expected := 0
	for _, net := range request.ExplicitCircuit.Nets {
		if net.ReturnNet != "" {
			expected++
		}
	}
	if expected == 0 {
		t.Fatal("four-layer promotion has no generated return-path obligations")
	}
	var baseline []designworkflow.ExplicitReturnPathEvidence
	for runIndex, run := range runs {
		var evidence []designworkflow.ExplicitReturnPathEvidence
		for _, stage := range run.Workflow.Stages {
			if stage.Name != designworkflow.StageRouting {
				continue
			}
			evidence, _ = stage.Summary["return_path_evidence"].([]designworkflow.ExplicitReturnPathEvidence)
			break
		}
		if len(evidence) != expected {
			t.Fatalf("run %d return-path evidence count = %d, want %d", runIndex+1, len(evidence), expected)
		}
		transitionCount := 0
		for _, item := range evidence {
			if !item.Pass || !item.SamplingComplete || item.ReturnNet == "" || item.SampleCount == 0 || len(item.UsedLayers) == 0 || len(item.ReturnPlaneLayers) == 0 {
				t.Fatalf("run %d incomplete return-path evidence: %#v", runIndex+1, item)
			}
			for _, transition := range item.LayerTransitions {
				transitionCount++
				if !transition.Pass || len(transition.SignalLayers) != 2 ||
					(transition.ReturnViaRequired && !transition.ReturnViaFound) ||
					(!transition.ReturnViaRequired && len(transition.ReferenceLayers) != 1) {
					t.Fatalf("run %d incomplete return-transition evidence: %#v", runIndex+1, transition)
				}
			}
		}
		if transitionCount == 0 {
			t.Fatalf("run %d has no multilayer return-transition evidence", runIndex+1)
		}
		if runIndex == 0 {
			baseline = append([]designworkflow.ExplicitReturnPathEvidence(nil), evidence...)
		} else if !reflect.DeepEqual(baseline, evidence) {
			t.Fatalf("return-path evidence differs across deterministic runs:\nfirst=%#v\nrun%d=%#v", baseline, runIndex+1, evidence)
		}
	}
}

func promotionRunStages(runs []PhysicalPromotionRun) []designworkflow.StageResult {
	if len(runs) == 0 {
		return nil
	}
	return runs[len(runs)-1].Workflow.Stages
}
