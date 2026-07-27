package capabilityevaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	frozenDiscoveryCorpusSHA256 = "af5df6f4811c5555fddd9f79a371af5de81acca1469b4146b9d04d7097ffec4b"
	frozenHeldOutCorpusSHA256   = "d4704e12ca77a6a8588bf7d3e975c8cc17e6db17097e80d0f90db91d15f9d6eb"
)

func TestFrozenOpenWorldCorpora(t *testing.T) {
	root := filepath.Join("testdata", "open_world_corpus")
	discovery, err := LoadCorpus(filepath.Join(root, "discovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	heldOut, err := LoadCorpus(filepath.Join(root, "held_out.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCorpusPair(discovery, heldOut); err != nil {
		t.Fatal(err)
	}
	assertCorpusFileSHA256(t, filepath.Join(root, "discovery.json"), frozenDiscoveryCorpusSHA256)
	assertCorpusFileSHA256(t, filepath.Join(root, "held_out.json"), frozenHeldOutCorpusSHA256)
	if len(discovery.Cases) != 12 || len(heldOut.Cases) != 12 {
		t.Fatalf("corpus sizes = %d/%d", len(discovery.Cases), len(heldOut.Cases))
	}
	for _, corpus := range []Corpus{discovery, heldOut} {
		for _, current := range corpus.Cases {
			lower := strings.ToLower(current.Prompt)
			for _, forbidden := range []string{
				"kicad", "fixture", "expected outcome", "issue code", "gap code",
				"coordinate", "footprint", "part number", "model file",
			} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("%s source contains forbidden implementation text %q", current.ID, forbidden)
				}
			}
		}
	}
}

func TestCorpusValidationRejectsLeakageAndMutation(t *testing.T) {
	discovery := testCorpus(CorpusDiscovery, "discovery")
	heldOut := testCorpus(CorpusHeldOut, "heldout")
	if err := ValidateCorpusPair(discovery, heldOut); err != nil {
		t.Fatal(err)
	}
	heldOut.Cases[0].SourceSHA256 = discovery.Cases[0].SourceSHA256
	if err := ValidateCorpusPair(discovery, heldOut); err == nil {
		t.Fatal("expected cross-corpus source leakage error")
	}
	discovery = testCorpus(CorpusDiscovery, "discovery")
	discovery.Cases[0].Prompt += " changed"
	if err := ValidateCorpus(discovery); err == nil {
		t.Fatal("expected source hash mutation error")
	}
}

func testCorpus(role CorpusRole, prefix string) Corpus {
	corpus := Corpus{Schema: CorpusSchema, Version: CorpusVersion, Role: role}
	for index, domain := range allDomains() {
		prompt := prefix + " behavior request " + string(domain)
		corpus.Cases = append(corpus.Cases, CorpusCase{
			ID: fmt.Sprintf("%s_case_%03d", prefix, index+1), Domain: domain,
			SafetyImpact: SafetyReviewRequired, Prompt: prompt, SourceSHA256: sourceHash(prompt),
		})
	}
	return corpus
}

func sourceHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func assertCorpusFileSHA256(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("%s sha256 = %s, want %s", path, got, want)
	}
}
