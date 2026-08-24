package canonicaljsonstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestSHA256MatchesJSONMarshal(t *testing.T) {
	value := struct {
		Name   string         `json:"name"`
		Values map[string]any `json:"values"`
	}{Name: "evidence", Values: map[string]any{"z": nil, "a": []any{1.25, "x"}}}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := sha256.Sum256(encoded)
	want := hex.EncodeToString(wantBytes[:])
	got, err := SHA256(value)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SHA256() = %s, want %s", got, want)
	}
}
