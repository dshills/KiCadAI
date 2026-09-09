package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/simulationadmission"
)

// Build a real V21 certificate and numerical failure around an independent
// hand-authored graph; this does not rerun or inspect any evaluation corpus.
func testCompletedV21RunV22(t *testing.T, r Requirement, graph CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment) SynthesisRun {
	t.Helper()
	initial := EvaluateCandidateV20(context.Background(), r, graph, nil, inventory, environment, admission, DefaultPolicy())
	repair := RepairCandidateV21(context.Background(), r, graph, initial, inventory, environment, admission, DefaultPolicy())
	if repair.TopologyCompletionV21 == nil || repair.TopologyCompletionV21.Selected == nil || !repair.TopologyCompletionV21.Selected.Invariant.Complete {
		t.Fatalf("independent V21 certificate missing: %+v", repair.Issues)
	}
	rh, _ := CanonicalHash(r)
	gh, _ := GraphHash(graph)
	run := SynthesisRun{Schema: SynthesisRunSchema, Version: SynthesisRunVersion, Report: Report{Schema: ReportSchema, Version: ReportVersion, PolicyVersion: PolicyVersion, RequirementHash: rh, PrimitiveInventoryHash: inventory.Hash, CatalogHash: environment.CatalogHash, ModelRegistryHash: inventory.ModelRegistryHash, Policy: DefaultPolicy(), Status: StatusFailed, StopReason: StopNoPassingGraph, Candidates: []CandidateReport{{Fingerprint: gh, Status: StatusFailed}}}, Candidates: []SynthesisCandidateEvidence{{Fingerprint: gh, Evaluations: []SimulationEvaluation{initial}, Repair: &repair}}}
	return finalizeSynthesisRunV17(run)
}

func TestElectricalSynthesisV22ContinuesCompleteCertificateAndLowers(t *testing.T) {
	r, graph, inventory, environment, admission := testMultiControlFixtureV22(t)
	v21 := testCompletedV21RunV22(t, r, graph, inventory, environment, admission)
	before, _ := json.Marshal(v21)
	result := ContinueElectricalV22(context.Background(), r, v21, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if result.ElectricalRepair == nil || result.ElectricalRepair.Status != RepairSearchPassed {
		_, _, _, found := electricalSourceV22(r, v21, inventory)
		t.Logf("source=%t status=%s stop=%s critical=%t universal=%t plan=%s diagnoses=%+v", found, v21.Report.Status, v21.Report.StopReason, allCandidateFailuresCriticalV19(r, v21), universalDiagnosisExistsV19(v21), v21.Candidates[0].Repair.TopologyCompletionV21.Status, v21.Candidates[0].Evaluations[0].Diagnoses)
		t.Fatalf("electrical continuation absent: %+v", result.ElectricalRepair)
	}
	if result.Synthesis.Report.Status != StatusPassed || result.Synthesis.Physical == nil || result.Synthesis.Physical.Status != PhysicalLoweringReady {
		t.Fatalf("physical lowering failed: %+v", result.Synthesis.Report.Diagnostics)
	}
	if result.Synthesis.Report.Selected.EvaluationHash != result.ElectricalRepair.Selected.Evaluation.Hash {
		t.Fatal("physical selection lost electrical binding")
	}
	after, _ := json.Marshal(v21)
	if string(before) != string(after) {
		t.Fatal("predecessor was mutated")
	}
	replay := ContinueElectricalV22(context.Background(), r, v21, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
	if !reflect.DeepEqual(result, replay) {
		t.Fatal("synthesis continuation is not deterministic")
	}
}

func TestElectricalSynthesisV22PreservesIneligibleAndTamperedSources(t *testing.T) {
	r, graph, inventory, environment, admission := testControlBindingFixtureV22(t)
	base := testCompletedV21RunV22(t, r, graph, inventory, environment, admission)
	for _, category := range []string{"passed", "incomplete", "canceled", "invalid", "certificate", "evaluation", "requirement", "source_hash", "source_metadata"} {
		t.Run(category, func(t *testing.T) {
			data, _ := json.Marshal(base)
			var original SynthesisRun
			if err := json.Unmarshal(data, &original); err != nil {
				t.Fatal(err)
			}
			req := cloneRequirement(r)
			switch category {
			case "passed":
				original.Report.Status = StatusPassed
			case "incomplete":
				original.Report.StopReason = StopNoCompleteGraph
			case "canceled":
				original.Report.Status = StatusCanceled
			case "invalid":
				original.Report.Status = StatusInvalid
			case "certificate":
				original.Candidates[0].Repair.TopologyCompletionV21.Selected.Invariant.Hash = "tampered"
			case "evaluation":
				original.Candidates[0].Evaluations[0].GraphHash = "tampered"
			case "requirement":
				req.Project.Name = "different_requirement"
			case "source_hash":
				original.Hash = strings.Repeat("1", 64)
			case "source_metadata":
				original.Report.Consumption.GeneratedGraphs++
			}
			result := ContinueElectricalV22(context.Background(), req, original, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
			if !reflect.DeepEqual(original, result.Synthesis) || (result.ElectricalRepair != nil && result.ElectricalRepair.BindingWork != 0) {
				t.Fatal("ineligible predecessor changed or consumed search work")
			}
		})
	}
}

func TestElectricalSynthesisV22IndependentOptionalKiCadPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 for installed-KiCad validation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	index, _ := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{
		SymbolsRoot:    openTopologyLibraryRoot(t, libraryresolver.EnvSymbolsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols"),
		FootprintsRoot: openTopologyLibraryRoot(t, libraryresolver.EnvFootprintsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints"),
		TemplatesRoot:  strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
	}, libraryresolver.LoadOptions{})
	for _, compound := range []bool{false, true} {
		name := "single_monitor"
		if compound {
			name = "dual_monitor"
		}
		t.Run(name, func(t *testing.T) {
			r, graph, inventory, environment, admission := testControlBindingFixtureV22(t)
			if compound {
				r, graph, inventory, environment, admission = testMultiControlFixtureV22(t)
			}
			v21 := testCompletedV21RunV22(t, r, graph, inventory, environment, admission)
			result := ContinueElectricalV22(ctx, r, v21, inventory, environment, admission, DefaultPolicy(), DefaultElectricalRepairLimitsV22())
			if result.Synthesis.Report.Status != StatusPassed {
				t.Fatalf("electrical/lowering status=%s", result.Synthesis.Report.Status)
			}
			root := t.TempDir()
			if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
				root = filepath.Join(retained, "v22_"+name)
			}
			promotion := PromoteSynthesisRun(ctx, result.Synthesis, environment, PhysicalPromotionOptions{OutputRoot: root, KiCadCLI: openTopologyKiCadCLI(t), LibraryIndex: &index, Timeout: 3 * time.Minute, KeepArtifacts: true})
			if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical || len(promotion.Runs) != 2 {
				t.Fatalf("installed-KiCad promotion status=%s replay=%t issues=%+v", promotion.Status, promotion.ReplayIdentical, promotion.Issues)
			}
			t.Logf("electrical=%s synthesis=%s physical=%s project=%s", result.ElectricalRepair.Hash, result.Synthesis.Hash, promotion.Hash, promotion.ProjectHash)
		})
	}
}
