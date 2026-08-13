package corpuspublication

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenHeldOutCommitmentsV8ReturnsMetadataOnly(t *testing.T) {
	request, _ := publicationFixtureV8(t)
	result, err := PublishV8(request)
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFileV8))
	if err != nil {
		t.Fatal(err)
	}
	commitments, err := OpenHeldOutCommitmentsV8(key, result.Manifest, ciphertext)
	if err != nil || len(commitments) != 18 {
		t.Fatalf("open V8 commitment metadata: %d, %v", len(commitments), err)
	}
	for _, entry := range commitments {
		if entry.Role != "held_out" || !entry.Sealed || entry.RequirementSHA256 == "" ||
			entry.NeutralSemanticSHA256 == "" || entry.NormalizedSemanticSHA256 == "" {
			t.Fatalf("invalid held-out commitment metadata: %+v", entry)
		}
	}
}
