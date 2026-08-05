package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const protectedCurrentOutputPromotionEnv = "KICADAI_PROTECTED_CURRENT_OUTPUT_PROMOTION"

func TestProtectedCurrentOutputCorpusPromotion(t *testing.T) {
	if os.Getenv(protectedCurrentOutputPromotionEnv) != "1" {
		t.Skip("set " + protectedCurrentOutputPromotionEnv + "=1 to run protected current-output promotion")
	}
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
			first := Synthesize(context.Background(), requirement, inventory, environment, policy)
			assertProtectedCurrentOutputPass(t, first)
			second := Synthesize(context.Background(), requirement, inventory, environment, policy)
			assertProtectedCurrentOutputPass(t, second)
			firstJSON, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			secondJSON, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("protected current-output synthesis replay is not byte-identical")
			}
			assertSynthesisConsumptionMatchesEvidence(t, first)
		})
	}
	if executed == 0 {
		t.Fatal("protected current-output promotion filter selected no frozen case")
	}
}

func testProtectedCurrentOutputRequirement(
	t *testing.T,
	root string,
	entry protectedCurrentOutputCorpusCase,
) Requirement {
	t.Helper()
	path := filepath.Clean(filepath.Join(root, entry.RequirementFile))
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, path)))
	if len(issues) != 0 {
		t.Fatalf("%s requirement issues: %#v", entry.ID, issues)
	}
	return requirement
}

func assertProtectedCurrentOutputPass(t *testing.T, run SynthesisRun) {
	t.Helper()
	if run.Report.Status != StatusPassed || run.Report.StopReason != StopPassed ||
		run.Report.Selected == nil || run.SelectedGraph == nil || run.Physical == nil ||
		run.Physical.Status != PhysicalLoweringReady || len(run.Report.Diagnostics) != 0 {
		t.Fatalf(
			"protected current-output synthesis = status=%s stop=%s selected=%t physical=%#v diagnostics=%#v consumption=%#v failures=%s",
			run.Report.Status, run.Report.StopReason, run.Report.Selected != nil,
			run.Physical, run.Report.Diagnostics, run.Report.Consumption,
			protectedCurrentOutputFailureSummary(run),
		)
	}
}

func protectedCurrentOutputFailureSummary(run SynthesisRun) string {
	counts := map[string]int{}
	for _, candidate := range run.Candidates {
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				key := strings.Join([]string{
					diagnosis.Code,
					diagnosis.RequirementID,
					diagnosis.OperatingCase,
					diagnosis.Metric,
				}, "/")
				counts[key]++
			}
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ",")
}
