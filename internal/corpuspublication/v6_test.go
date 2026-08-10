package corpuspublication

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishV6UsesDistinctAuthenticatedFormat(t *testing.T) {
	request, wantHeldOut := publicationFixture(t)
	result, err := PublishV6(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Schema != ManifestSchemaV6 || result.Manifest.Version != ManifestVersionV6 {
		t.Fatalf("V6 manifest identity = %q/%d", result.Manifest.Schema, result.Manifest.Version)
	}
	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFile))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenHeldOutV6(key, result.Manifest, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened) != expectedHeldOut {
		t.Fatalf("opened V6 cases = %d, want %d", len(opened), expectedHeldOut)
	}
	for _, item := range opened {
		if string(item.Source) != string(wantHeldOut[item.Entry.ID]) {
			t.Fatalf("V6 held-out source %s changed", item.Entry.ID)
		}
	}
	if _, err := OpenHeldOut(key, result.Manifest, ciphertext); err == nil {
		t.Fatal("V5 opener accepted V6 ciphertext")
	}
	audit, err := os.ReadFile(filepath.Join(request.DestinationRoot, AuditFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(audit), "# Closed-Loop Open-Set V6 Corpus Freeze Audit\n") || strings.Contains(string(audit), "V5 Corpus") {
		t.Fatal("V6 publisher emitted the wrong audit identity")
	}
}

func TestPublishRemainsV5AfterV6Extension(t *testing.T) {
	request, _ := publicationFixture(t)
	result, err := Publish(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Schema != ManifestSchema || result.Manifest.Version != ManifestVersion {
		t.Fatal("V5 publisher identity changed")
	}
}
