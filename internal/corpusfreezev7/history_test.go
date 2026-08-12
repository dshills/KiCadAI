package corpusfreezev7

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestV7HistoricalCommitmentsBindV1ThroughV6WithoutOpeningRetiredSource(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate V7 historical commitment fixture")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "specs", "closed-loop-open-set-capability-expansion", "V7_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(path)
	if err != nil {
		t.Fatal(err)
	}
	if history.Base.SourceSHA256 != HistoricalCommitmentsSHA256 {
		t.Fatalf("V7 historical commitments SHA-256 = %s", history.Base.SourceSHA256)
	}
	if len(history.Base.RawSHA256) != 168 || len(history.Base.NeutralSemanticSHA256) != 96 || len(history.NormalizedSemanticSHA256) != 72 {
		t.Fatalf("V7 historical commitment counts = raw:%d neutral:%d normalized:%d",
			len(history.Base.RawSHA256), len(history.Base.NeutralSemanticSHA256), len(history.NormalizedSemanticSHA256))
	}
	for name, commitments := range map[string]map[string]string{
		"raw": history.Base.RawSHA256, "neutral": history.Base.NeutralSemanticSHA256, "normalized": history.NormalizedSemanticSHA256,
	} {
		v6Count := 0
		for _, id := range commitments {
			if strings.HasPrefix(id, "v6:v6_source_") {
				v6Count++
			}
		}
		if v6Count != 36 {
			t.Fatalf("V7 historical %s V6 commitments = %d, want 36", name, v6Count)
		}
	}
}
