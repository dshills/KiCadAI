package corpusfreezev9

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/corpusfreezev6"
)

func TestExtendHistoricalCommitmentsIsCanonicalAndComplete(t *testing.T) {
	previousPath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json")
	previous, err := os.ReadFile(previousPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]CommitmentEntry, 36)
	for index := range entries {
		entries[index] = CommitmentEntry{
			SourceID:                 fmt.Sprintf("v8_source_%03d", index+1),
			RequirementSHA256:        testDigest("raw", index),
			NeutralSemanticSHA256:    testDigest("neutral", index),
			NormalizedSemanticSHA256: testDigest("normalized", index),
		}
	}
	data, err := ExtendHistoricalCommitments(previous, entries)
	if err != nil {
		t.Fatal(err)
	}
	dataAgain, err := ExtendHistoricalCommitments(previous, entries)
	if err != nil || string(dataAgain) != string(data) {
		t.Fatal("history extension is not deterministic")
	}
	var got corpusfreezev6.HistoricalCommitmentFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Raw) != 240 || len(got.NeutralSemantic) != 168 || len(got.NormalizedSemantic) != 144 || got.RetiredSourceOpened {
		t.Fatalf("extended counts = %d/%d/%d opened=%v", len(got.Raw), len(got.NeutralSemantic), len(got.NormalizedSemantic), got.RetiredSourceOpened)
	}
}

func TestHistoricalBoundaryRejectsPredecessorOnlyHistory(t *testing.T) {
	previousPath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json")
	previous, err := LoadHistoricalCommitments(previousPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHistoricalBoundary(previous); err == nil {
		t.Fatal("predecessor-only history was accepted as V9")
	}
}

func TestExtendHistoricalCommitmentsFailsClosed(t *testing.T) {
	if _, err := ExtendHistoricalCommitments([]byte(`{}`), make([]CommitmentEntry, 36)); err == nil {
		t.Fatal("invalid predecessor was accepted")
	}
	previousPath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json")
	previous, err := os.ReadFile(previousPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExtendHistoricalCommitments(previous, nil); err == nil {
		t.Fatal("incomplete V8 commitment set was accepted")
	}
}

func testDigest(kind string, index int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("v9-history-test:%s:%d", kind, index)))
	return hex.EncodeToString(digest[:])
}
