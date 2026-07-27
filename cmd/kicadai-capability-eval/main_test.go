package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/capabilityevaluation"
)

func TestRunRequiresAllInputs(t *testing.T) {
	if err := run(nil, testWriter{t}); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestWriteAtomicReplacesReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.json")
	if err := writeAtomic(path, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("second\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second\n" {
		t.Fatalf("report bytes = %q", data)
	}
}

func TestRunCompareExplainAndAffectedModes(t *testing.T) {
	root := t.TempDir()
	observation := capabilityevaluation.Observation{
		Capability: "clock_fanout_loading", Outcome: capabilityevaluation.OutcomeUnsupported,
		Stage: "architecture_search", Code: capabilityevaluation.CodeCapabilityUnsupported,
		Path: "requirements.clock", Reason: "fanout unavailable",
		RequiredEvidence: []string{"reviewed fanout evidence"},
	}
	baseline := capabilityevaluation.Report{
		Schema: capabilityevaluation.ReportSchema, PolicyVersion: capabilityevaluation.DefaultPolicyVersion,
		CorpusRole: capabilityevaluation.CorpusHeldOut, CorpusSHA256: "corpus_hash",
		RegistryVersion: "registry_v1", RegistrySHA256: "registry_hash", CaseCount: 1,
		Cases: []capabilityevaluation.CaseResult{{
			ID: "case_001", Domain: capabilityevaluation.DomainDigital,
			SafetyImpact: capabilityevaluation.SafetyRelevant,
			Outcome:      capabilityevaluation.OutcomeUnsupported,
			Observations: []capabilityevaluation.Observation{observation},
		}},
		RankedClusters: []capabilityevaluation.Cluster{{
			Rank: 1, Key: "unsupported:architecture_search:clock_fanout_loading:CAPABILITY_UNSUPPORTED",
			Outcome: capabilityevaluation.OutcomeUnsupported, Stage: "architecture_search",
			Capability: "clock_fanout_loading", Code: capabilityevaluation.CodeCapabilityUnsupported,
			Cases: []string{"case_001"},
		}},
	}
	final := baseline
	final.Cases = []capabilityevaluation.CaseResult{{
		ID: "case_001", Domain: capabilityevaluation.DomainDigital,
		SafetyImpact: capabilityevaluation.SafetyRelevant,
		Outcome:      capabilityevaluation.OutcomeReady,
		Observations: []capabilityevaluation.Observation{},
	}}
	final.RankedClusters = nil
	writeReport := func(name string, report capabilityevaluation.Report) string {
		t.Helper()
		data, err := report.MarshalJSONStable()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	baselinePath := writeReport("baseline.json", baseline)
	finalPath := writeReport("final.json", final)
	promotionsPath := filepath.Join(root, "promotions.json")
	promotions := capabilityevaluation.PromotionEvidenceSet{
		Schema: capabilityevaluation.PromotionEvidenceSchema, Version: 1,
		CorpusRole: capabilityevaluation.CorpusHeldOut, CorpusSHA256: "corpus_hash",
		Cases: []capabilityevaluation.PromotionEvidence{{
			CaseID: "case_001",
			Gates:  []string{"connectivity", "erc", "routing", "simulation", "strict_drc", "writer_correctness", "zero_diff_replay"},
		}},
	}
	data, err := json.Marshal(promotions)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promotionsPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	comparePath := filepath.Join(root, "comparison.json")
	if err := run([]string{
		"-mode", "compare", "-baseline", baselinePath, "-final", finalPath,
		"-promotions", promotionsPath, "-required-capabilities", "clock_fanout_loading",
		"-output", comparePath,
	}, testWriter{t}); err != nil {
		t.Fatal(err)
	}
	explainPath := filepath.Join(root, "explain.json")
	if err := run([]string{
		"-mode", "explain", "-report", baselinePath,
		"-capability", "clock_fanout_loading", "-output", explainPath,
	}, testWriter{t}); err != nil {
		t.Fatal(err)
	}
	affectedPath := filepath.Join(root, "affected.json")
	if err := run([]string{
		"-mode", "affected", "-report", baselinePath,
		"-capability", "clock_fanout_loading", "-output", affectedPath,
	}, testWriter{t}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{comparePath, explainPath, affectedPath} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("output %s missing: %v", path, err)
		}
	}
}

type testWriter struct {
	t *testing.T
}

func (writer testWriter) Write(data []byte) (int, error) {
	writer.t.Helper()
	return len(data), nil
}
