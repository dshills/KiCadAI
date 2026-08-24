package closedloopsynthesis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestStreamingHashMatchesBufferedJSON(t *testing.T) {
	value := CandidateState{Fingerprint: "candidate", Variables: []Variable{{ID: "gain", Kind: "ratio", Value: 2}}}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := sha256.Sum256(encoded)
	want := hex.EncodeToString(wantBytes[:])
	if got := hashJSON(value); got != want {
		t.Fatalf("streamed hash=%s, buffered hash=%s", got, want)
	}
}
