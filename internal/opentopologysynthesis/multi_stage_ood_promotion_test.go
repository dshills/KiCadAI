package opentopologysynthesis

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set " + openTopologyKiCadPromotionEnv + "=1 to run installed-KiCad multi-stage promotion")
	}
	if target := strings.TrimSpace(os.Getenv(multiStageOODCaseEnv)); target != "" && target != "ambient_tracking_airflow_control" {
		t.Skip("multi-stage promotion filter excludes ambient_tracking_airflow_control")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "ambient_tracking_airflow_control.json"),
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
		outputRoot = filepath.Join(retained, "ambient_tracking_airflow_control")
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
		t.Fatalf(
			"multi-stage KiCad promotion status=%s replay=%t project=%s runs=%d issues=%#v board=%#v components=%v routing=%#v",
			promotion.Status,
			promotion.ReplayIdentical,
			promotion.ProjectHash,
			len(promotion.Runs),
			promotion.Issues,
			first.Physical.Document.Project.Board,
			nonlinearSwitchingPhysicalComponents(first.Physical.Bindings),
			nonlinearSwitchingRoutingSummary(promotion.Runs),
		)
	}
	t.Logf(
		"multi-stage ambient promotion synthesis=%s physical=%s project=%s evidence=%s",
		first.Hash,
		first.Physical.Hash,
		promotion.ProjectHash,
		promotion.Hash,
	)
}
