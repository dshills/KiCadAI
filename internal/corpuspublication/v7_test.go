package corpuspublication

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishV7UsesDistinctAuthenticatedFormat(t *testing.T) {
	request, wantHeldOut := publicationFixture(t)
	result, err := PublishV7(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Schema != ManifestSchemaV7 || result.Manifest.Version != ManifestVersionV7 {
		t.Fatalf("V7 manifest identity = %q/%d", result.Manifest.Schema, result.Manifest.Version)
	}
	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("V7 key mode = %v, want 0600", info.Mode().Perm())
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFile))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenHeldOutV7(key, result.Manifest, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened) != expectedHeldOut {
		t.Fatalf("opened V7 cases = %d, want %d", len(opened), expectedHeldOut)
	}
	for _, item := range opened {
		if string(item.Source) != string(wantHeldOut[item.Entry.ID]) {
			t.Fatalf("V7 held-out source %s changed", item.Entry.ID)
		}
	}
	if _, err := OpenHeldOutV6(key, result.Manifest, ciphertext); err == nil {
		t.Fatal("V6 opener accepted V7 ciphertext")
	}
	if _, err := OpenHeldOut(key, result.Manifest, ciphertext); err == nil {
		t.Fatal("V5 opener accepted V7 ciphertext")
	}
	audit, err := os.ReadFile(filepath.Join(request.DestinationRoot, AuditFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(audit), "# Closed-Loop Open-Set V7 Corpus Freeze Audit\n") ||
		strings.Contains(string(audit), "V6 Corpus") || strings.Contains(string(audit), "V5 Corpus") {
		t.Fatal("V7 publisher emitted the wrong audit identity")
	}
}

func TestOpenHeldOutV7RejectsTamperingAndCrossVersionMetadata(t *testing.T) {
	request, _ := publicationFixture(t)
	result, err := PublishV7(request)
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFile))
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 1
	if _, err := OpenHeldOutV7(key, result.Manifest, tampered); err == nil {
		t.Fatal("V7 opener accepted tampered ciphertext")
	}
	wrongVersion := result.Manifest
	wrongVersion.Version = ManifestVersionV6
	if _, err := OpenHeldOutV7(key, wrongVersion, ciphertext); err == nil {
		t.Fatal("V7 opener accepted cross-version manifest metadata")
	}
}

func TestPublishV7OccupiedDestinationFailsClosed(t *testing.T) {
	request, _ := publicationFixture(t)
	if err := os.MkdirAll(request.DestinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(request.DestinationRoot, "marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishV7(request); err == nil {
		t.Fatal("V7 publisher replaced an occupied destination")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" {
		t.Fatalf("occupied destination marker = %q, error %v", data, err)
	}
	if _, err := os.Lstat(request.KeyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed V7 publication retained a new key: %v", err)
	}
}
