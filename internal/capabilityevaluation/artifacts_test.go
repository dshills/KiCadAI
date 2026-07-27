package capabilityevaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStrictArtifactLoadersAndCorpusBinding(t *testing.T) {
	corpus := testCorpus(CorpusDiscovery, "discovery")
	corpusHash, err := CorpusSHA256(corpus)
	if err != nil {
		t.Fatal(err)
	}
	evidence := EvidenceSet{
		Schema: EvidenceSchema, Version: 1, CorpusRole: corpus.Role, CorpusSHA256: corpusHash,
	}
	for _, current := range corpus.Cases {
		evidence.Cases = append(evidence.Cases, CaseResult{
			ID: current.ID, Domain: current.Domain, SafetyImpact: current.SafetyImpact,
			Outcome: OutcomeReady, Observations: []Observation{},
		})
	}
	path := filepath.Join(t.TempDir(), "evidence.json")
	writeTestJSON(t, path, evidence)
	loaded, err := LoadEvidenceSet(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateEvidenceSet(corpus, loaded, ImpactRegistry{Version: "registry_v1"}, DefaultRankingPolicy()); err != nil {
		t.Fatal(err)
	}
	loaded.CorpusSHA256 = sourceHash("different corpus")
	if _, err := EvaluateEvidenceSet(corpus, loaded, ImpactRegistry{Version: "registry_v1"}, DefaultRankingPolicy()); err == nil {
		t.Fatal("expected corpus binding error")
	}

	if err := os.WriteFile(path, append(mustTestJSON(t, evidence), []byte(` {}`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEvidenceSet(path); err == nil {
		t.Fatal("expected trailing JSON error")
	}
}

func TestLoadImpactRegistryRejectsCycles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	writeTestJSON(t, path, ImpactRegistry{
		Version: "registry_v1",
		Records: []ImpactRecord{
			{Capability: "capability_a", Consumers: []string{"capability_b"}},
			{Capability: "capability_b", Consumers: []string{"capability_a"}},
		},
	})
	if _, err := LoadImpactRegistry(path); err == nil {
		t.Fatal("expected cyclic registry error")
	}
}

func TestStrictReportAndPromotionEvidenceLoaders(t *testing.T) {
	root := t.TempDir()
	baseline, _ := improvementReports(t)
	reportBytes, err := baseline.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, "report.json")
	if err := os.WriteFile(reportPath, reportBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReport(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CorpusSHA256 != baseline.CorpusSHA256 || loaded.CaseCount != len(baseline.Cases) {
		t.Fatalf("loaded report identity = %#v", loaded)
	}

	promotionPath := filepath.Join(root, "promotions.json")
	promotion := `{
	  "schema": "kicadai.open-world-promotion-evidence.v1",
	  "version": 1,
	  "corpus_role": "held_out",
	  "corpus_sha256": "` + baseline.CorpusSHA256 + `",
	  "cases": [{"case_id":"case_002","gates":["connectivity","erc","routing","simulation","strict_drc","writer_correctness","zero_diff_replay"]}]
	}`
	if err := os.WriteFile(promotionPath, []byte(promotion), 0o600); err != nil {
		t.Fatal(err)
	}
	promotions, err := LoadPromotionEvidenceSet(promotionPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(promotions.Cases) != 1 || promotions.Cases[0].CaseID != "case_002" {
		t.Fatalf("promotions = %#v", promotions)
	}

	if err := os.WriteFile(promotionPath, []byte(promotion+`{"extra":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPromotionEvidenceSet(promotionPath); err == nil {
		t.Fatal("expected trailing promotion evidence to fail")
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.WriteFile(path, mustTestJSON(t, value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
