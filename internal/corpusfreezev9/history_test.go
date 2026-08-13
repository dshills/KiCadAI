package corpusfreezev9

import (
	"path/filepath"
	"testing"
)

func TestV9HistoryWrapperRejectsPredecessorOnlyHistory(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHistoricalBoundary(history); err == nil {
		t.Fatal("predecessor-only history was accepted as V9")
	}
}
