package circuitgraph

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

const (
	adversarialFinalReportSchema  = "kicadai.adversarial-function-corpus-capability.v1"
	adversarialFinalEvaluator     = "adversarial-promotion-v2"
	adversarialFinalGeneratedAt   = "2026-07-17T00:00:00Z"
	adversarialFinalReportEnv     = "KICADAI_WRITE_ADVERSARIAL_FINAL_REPORT"
	adversarialEvaluatedCommitEnv = "KICADAI_ADVERSARIAL_EVALUATED_COMMIT"
)

type adversarialFinalAggregate struct {
	Circuits        int            `json:"circuits"`
	Passed          int            `json:"passed"`
	Blocked         int            `json:"blocked"`
	PassRatePercent float64        `json:"pass_rate_percent"`
	ByCategory      map[string]int `json:"by_category"`
	ByCode          map[string]int `json:"by_code"`
	ByRootKey       map[string]int `json:"by_root_key"`
}

type adversarialFinalDelta struct {
	Passed     int            `json:"passed"`
	Blocked    int            `json:"blocked"`
	ByCategory map[string]int `json:"by_category"`
	ByCode     map[string]int `json:"by_code"`
	ByRootKey  map[string]int `json:"by_root_key"`
}

type adversarialFinalReport struct {
	Schema               string                         `json:"schema"`
	GeneratedAt          string                         `json:"generated_at"`
	CorpusManifestSHA256 string                         `json:"corpus_manifest_sha256"`
	FrozenBaselineSHA256 string                         `json:"frozen_baseline_sha256"`
	ClosureHistorySHA256 string                         `json:"closure_history_sha256"`
	BaseCommit           string                         `json:"base_commit"`
	EvaluatedCommit      string                         `json:"evaluated_commit"`
	Evaluator            string                         `json:"evaluator"`
	GateProfile          map[string]string              `json:"gate_profile"`
	Delta                adversarialFinalDelta          `json:"delta_from_frozen_baseline"`
	Circuits             []adversarialCapabilityCircuit `json:"circuits"`
	Aggregate            adversarialFinalAggregate      `json:"aggregate"`
}

func TestWriteAdversarialFunctionCorpusFinalReport(t *testing.T) {
	if os.Getenv(adversarialFinalReportEnv) != "1" {
		t.Skipf("set %s=1 to regenerate the final KiCad-backed report", adversarialFinalReportEnv)
	}
	commit := strings.TrimSpace(os.Getenv(adversarialEvaluatedCommitEnv))
	if len(commit) != 40 {
		t.Fatalf("set %s to the 40-character implementation commit", adversarialEvaluatedCommitEnv)
	}
	capability := evaluateAdversarialCorpus(t, true, nil)
	final := buildAdversarialFinalReport(t, capability, commit)
	contents, err := json.MarshalIndent(final, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	contents = append(contents, '\n')
	if err := os.WriteFile(adversarialCapabilityReportPath(t, "FINAL_REPORT.json"), contents, 0o644); err != nil {
		t.Fatal(err)
	}
	checksum := sha256Hex(contents) + "  FINAL_REPORT.json\n"
	if err := os.WriteFile(adversarialCapabilityReportPath(t, "FINAL_REPORT.sha256"), []byte(checksum), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAdversarialFunctionCorpusFinalReportIsComplete(t *testing.T) {
	contents, err := os.ReadFile(adversarialCapabilityReportPath(t, "FINAL_REPORT.json"))
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := os.ReadFile(adversarialCapabilityReportPath(t, "FINAL_REPORT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(checksum)) != sha256Hex(contents)+"  FINAL_REPORT.json" {
		t.Fatal("final checksum sidecar does not match the capability report")
	}
	var final adversarialFinalReport
	if err := json.Unmarshal(contents, &final); err != nil {
		t.Fatal(err)
	}
	if final.Schema != adversarialFinalReportSchema || final.Evaluator != adversarialFinalEvaluator || final.GeneratedAt != adversarialFinalGeneratedAt || len(final.EvaluatedCommit) != 40 {
		t.Fatalf("invalid final report header: %#v", final)
	}
	if final.CorpusManifestSHA256 != frozenAdversarialCorpusManifestSHA256 || final.FrozenBaselineSHA256 != adversarialBaselineReportSHA256 {
		t.Fatalf("final frozen evidence hashes are stale: %#v", final)
	}
	closure, err := os.ReadFile(adversarialCapabilityReportPath(t, "CLOSURE_HISTORY.md"))
	if err != nil || final.ClosureHistorySHA256 != sha256Hex(closure) {
		t.Fatalf("closure history hash is stale: err=%v", err)
	}
	if final.Aggregate.Circuits != 18 || final.Aggregate.Passed != 17 || final.Aggregate.Blocked != 1 || final.Aggregate.PassRatePercent != 94.44 || len(final.Circuits) != 18 {
		t.Fatalf("final aggregate = %#v", final.Aggregate)
	}
	if final.Delta.Passed != 14 || final.Delta.Blocked != -14 {
		t.Fatalf("final baseline delta = %#v", final.Delta)
	}
	blocked := 0
	for _, circuit := range final.Circuits {
		if circuit.Status == "pass" {
			if len(circuit.Hashes["generated_files"]) != 64 || circuit.Category != "" || circuit.RootIssue != nil {
				t.Fatalf("final pass %s lacks complete evidence: %#v", circuit.ID, circuit)
			}
			continue
		}
		blocked++
		if circuit.ID != "bipolar_lmv321_inverting_amplifier" || circuit.Category != "simulation" || circuit.RootIssue == nil || circuit.RootIssue.Code != reports.CodeValidationFailed || circuit.RootIssue.Path != "devices.amplifier.supply" {
			t.Fatalf("unexpected final blocker: %#v", circuit)
		}
	}
	if blocked != 1 || countAdversarialTaxonomy(final.Aggregate.ByCategory) != 1 || countAdversarialTaxonomy(final.Aggregate.ByCode) != 1 || countAdversarialTaxonomy(final.Aggregate.ByRootKey) != 1 {
		t.Fatalf("final failure taxonomy is incomplete: aggregate=%#v blocked=%d", final.Aggregate, blocked)
	}
}

func TestAdversarialPassRateHandlesEmptyCorpus(t *testing.T) {
	if got := adversarialPassRate(0, 0); got != 0 {
		t.Fatalf("empty-corpus pass rate = %g, want 0", got)
	}
}

func buildAdversarialFinalReport(t *testing.T, capability adversarialCapabilityReport, commit string) adversarialFinalReport {
	t.Helper()
	baselineBytes, err := os.ReadFile(adversarialCapabilityReportPath(t, "BASELINE_REPORT.json"))
	if err != nil {
		t.Fatal(err)
	}
	var baseline adversarialCapabilityReport
	if err := json.Unmarshal(baselineBytes, &baseline); err != nil {
		t.Fatal(err)
	}
	closure, err := os.ReadFile(adversarialCapabilityReportPath(t, "CLOSURE_HISTORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	return adversarialFinalReport{
		Schema: adversarialFinalReportSchema, GeneratedAt: adversarialFinalGeneratedAt,
		CorpusManifestSHA256: capability.CorpusManifestSHA256, FrozenBaselineSHA256: sha256Hex(baselineBytes), ClosureHistorySHA256: sha256Hex(closure),
		BaseCommit: capability.BaseCommit, EvaluatedCommit: commit, Evaluator: adversarialFinalEvaluator, GateProfile: capability.GateProfile,
		Delta: adversarialFinalDelta{
			Passed: capability.Aggregate.Passed - baseline.Aggregate.Passed, Blocked: capability.Aggregate.Blocked - baseline.Aggregate.Blocked,
			ByCategory: adversarialTaxonomyDelta(capability.Aggregate.ByCategory, baseline.Aggregate.ByCategory),
			ByCode:     adversarialTaxonomyDelta(capability.Aggregate.ByCode, baseline.Aggregate.ByCode),
			ByRootKey:  adversarialTaxonomyDelta(capability.Aggregate.ByRootKey, baseline.Aggregate.ByRootKey),
		},
		Circuits: capability.Circuits,
		Aggregate: adversarialFinalAggregate{
			Circuits: capability.Aggregate.Circuits, Passed: capability.Aggregate.Passed, Blocked: capability.Aggregate.Blocked,
			PassRatePercent: adversarialPassRate(capability.Aggregate.Passed, capability.Aggregate.Circuits),
			ByCategory:      capability.Aggregate.ByCategory, ByCode: capability.Aggregate.ByCode, ByRootKey: capability.Aggregate.ByRootKey,
		},
	}
}

func adversarialPassRate(passed, circuits int) float64 {
	if circuits <= 0 {
		return 0
	}
	return math.Round(10_000*float64(passed)/float64(circuits)) / 100
}

func adversarialTaxonomyDelta(current, baseline map[string]int) map[string]int {
	delta := map[string]int{}
	for key, count := range baseline {
		delta[key] = -count
	}
	for key, count := range current {
		delta[key] += count
	}
	for key, count := range delta {
		if count == 0 {
			delete(delta, key)
		}
	}
	return delta
}

func countAdversarialTaxonomy(taxonomy map[string]int) int {
	total := 0
	for _, count := range taxonomy {
		total += count
	}
	return total
}
