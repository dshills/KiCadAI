package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const protectedVoltageOutputPromotionEnv = "KICADAI_PROTECTED_VOLTAGE_OUTPUT_PROMOTION"

func TestProtectedVoltageOutputCorpusPromotion(t *testing.T) {
	if os.Getenv(protectedVoltageOutputPromotionEnv) != "1" {
		t.Skip("set " + protectedVoltageOutputPromotionEnv + "=1 to run protected voltage-output promotion")
	}
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
			first := Synthesize(context.Background(), requirement, inventory, environment, policy)
			assertProtectedVoltageOutputPass(t, first)
			second := Synthesize(context.Background(), requirement, inventory, environment, policy)
			assertProtectedVoltageOutputPass(t, second)
			firstJSON, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			secondJSON, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("protected voltage-output synthesis replay is not byte-identical")
			}
			assertSynthesisConsumptionMatchesEvidence(t, first)
		})
	}
	if executed == 0 {
		t.Fatal("protected voltage-output promotion filter selected no frozen case")
	}
}

func assertProtectedVoltageOutputPass(t *testing.T, run SynthesisRun) {
	t.Helper()
	if run.Report.Status != StatusPassed || run.Report.StopReason != StopPassed ||
		run.Report.Selected == nil || run.SelectedGraph == nil || run.Physical == nil ||
		run.Physical.Status != PhysicalLoweringReady || len(run.Report.Diagnostics) != 0 {
		t.Fatalf(
			"protected voltage-output synthesis = status=%s stop=%s selected=%t physical=%#v diagnostics=%#v consumption=%#v",
			run.Report.Status, run.Report.StopReason, run.Report.Selected != nil,
			run.Physical, run.Report.Diagnostics, run.Report.Consumption,
		)
	}
}
