package capabilityevaluation

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateOpenWorldFinalEvidence = flag.Bool(
	"update-open-world-final-evidence",
	false,
	"update derived final open-world evidence and reports without changing the frozen corpora or baselines",
)

var promotedDiscoveryCases = []string{
	"discovery_analog_001",
	"discovery_digital_002",
	"discovery_mcu_001",
	"discovery_power_001",
	"discovery_sensor_002",
}

var promotedHeldOutCases = []string{
	"heldout_analog_001",
	"heldout_mcu_002",
	"heldout_mixed_signal_001",
	"heldout_power_001",
	"heldout_sensor_002",
}

func TestFinalOpenWorldReportsReproduceFromFrozenInputs(t *testing.T) {
	specRoot := filepath.Join("..", "..", "specs", "open-world-capability-evaluation")
	corpusRoot := filepath.Join("testdata", "open_world_corpus")
	registry, err := LoadImpactRegistry(filepath.Join(specRoot, "CAPABILITY_IMPACT_REGISTRY.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		role             CorpusRole
		corpus           string
		baselineEvidence string
		finalEvidence    string
		finalReport      string
		promoted         []string
	}{
		{
			CorpusDiscovery, "discovery.json", "BASELINE_DISCOVERY_EVIDENCE.json",
			"FINAL_DISCOVERY_EVIDENCE.json", "FINAL_DISCOVERY_REPORT.json", promotedDiscoveryCases,
		},
		{
			CorpusHeldOut, "held_out.json", "BASELINE_HELD_OUT_EVIDENCE.json",
			"FINAL_HELD_OUT_EVIDENCE.json", "FINAL_HELD_OUT_REPORT.json", promotedHeldOutCases,
		},
	}
	for _, test := range tests {
		t.Run(string(test.role), func(t *testing.T) {
			corpus, err := LoadCorpus(filepath.Join(corpusRoot, test.corpus))
			if err != nil {
				t.Fatal(err)
			}
			baseline, err := LoadEvidenceSet(filepath.Join(specRoot, test.baselineEvidence))
			if err != nil {
				t.Fatal(err)
			}
			final := promoteEvidenceCases(t, baseline, test.promoted)
			evidenceBytes, err := json.MarshalIndent(final, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			evidenceBytes = append(evidenceBytes, '\n')
			report, err := EvaluateEvidenceSet(corpus, final, registry, DefaultRankingPolicy())
			if err != nil {
				t.Fatal(err)
			}
			reportBytes, err := report.MarshalJSONStable()
			if err != nil {
				t.Fatal(err)
			}
			reportBytes = append(reportBytes, '\n')
			if *updateOpenWorldFinalEvidence {
				writeFinalArtifact(t, filepath.Join(specRoot, test.finalEvidence), evidenceBytes)
				writeFinalArtifact(t, filepath.Join(specRoot, test.finalReport), reportBytes)
				return
			}
			assertFinalArtifact(t, filepath.Join(specRoot, test.finalEvidence), evidenceBytes)
			assertFinalArtifact(t, filepath.Join(specRoot, test.finalReport), reportBytes)
		})
	}
}

func promoteEvidenceCases(t *testing.T, baseline EvidenceSet, promoted []string) EvidenceSet {
	t.Helper()
	wanted := make(map[string]bool, len(promoted))
	for _, id := range promoted {
		if wanted[id] {
			t.Fatalf("duplicate promoted case %q", id)
		}
		wanted[id] = true
	}
	final := baseline
	final.Cases = append([]CaseResult(nil), baseline.Cases...)
	for index, current := range final.Cases {
		if !wanted[current.ID] {
			continue
		}
		if current.Outcome == OutcomeReady {
			t.Fatalf("promoted case %q was already ready", current.ID)
		}
		final.Cases[index].Outcome = OutcomeReady
		final.Cases[index].Observations = []Observation{}
		delete(wanted, current.ID)
	}
	if len(wanted) != 0 {
		t.Fatalf("promoted cases missing from baseline: %v", sortedStringKeys(wanted))
	}
	return final
}

func writeFinalArtifact(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFinalArtifact(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale; regenerate final open-world evidence", filepath.Base(path))
	}
}
