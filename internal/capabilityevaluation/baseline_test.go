package capabilityevaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestFrozenOpenWorldBaselineReportsReproduce(t *testing.T) {
	specRoot := filepath.Join("..", "..", "specs", "open-world-capability-evaluation")
	corpusRoot := filepath.Join("testdata", "open_world_corpus")
	registry, err := LoadImpactRegistry(filepath.Join(specRoot, "CAPABILITY_IMPACT_REGISTRY.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		role         CorpusRole
		corpus       string
		evidence     string
		report       string
		reportSHA256 string
	}{
		{
			CorpusDiscovery, "discovery.json", "BASELINE_DISCOVERY_EVIDENCE.json",
			"BASELINE_DISCOVERY_REPORT.json", "31db38e5ebb93bd52938289374965eacde194bfd961ce0c25d875628dce8f99e",
		},
		{
			CorpusHeldOut, "held_out.json", "BASELINE_HELD_OUT_EVIDENCE.json",
			"BASELINE_HELD_OUT_REPORT.json", "228263d5f8646278fdf7708c347f225e895ebdc4319130fe8948dd85cb71b433",
		},
	}
	for _, test := range tests {
		t.Run(string(test.role), func(t *testing.T) {
			corpus, err := LoadCorpus(filepath.Join(corpusRoot, test.corpus))
			if err != nil {
				t.Fatal(err)
			}
			evidence, err := LoadEvidenceSet(filepath.Join(specRoot, test.evidence))
			if err != nil {
				t.Fatal(err)
			}
			report, err := EvaluateEvidenceSet(corpus, evidence, registry, DefaultRankingPolicy())
			if err != nil {
				t.Fatal(err)
			}
			generated, err := report.MarshalJSONStable()
			if err != nil {
				t.Fatal(err)
			}
			generated = append(generated, '\n')
			frozen, err := os.ReadFile(filepath.Join(specRoot, test.report))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(generated, frozen) {
				t.Fatalf("%s does not reproduce from frozen inputs", test.report)
			}
			sum := sha256.Sum256(frozen)
			if got := hex.EncodeToString(sum[:]); got != test.reportSHA256 {
				t.Fatalf("%s sha256 = %s, want %s", test.report, got, test.reportSHA256)
			}
		})
	}
}
