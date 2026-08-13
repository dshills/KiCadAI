package corpusfreezev10

import (
	"path/filepath"
	"testing"
)

func TestV10HistoryWrapperRejectsPredecessorOnlyHistory(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHistoricalBoundary(history); err == nil {
		t.Fatal("predecessor-only history was accepted as V10")
	}
}

func TestV10HistoryWrapperAcceptsDigestOnlyV1ThroughV8History(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V9_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHistoricalBoundary(history); err != nil {
		t.Fatal(err)
	}
}
