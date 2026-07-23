package promotiontoolchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsUnknownFieldsAndUnsafePaths(t *testing.T) {
	valid := testLockJSON(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "lock.json")
	if err := os.WriteFile(path, []byte(strings.Replace(valid, `"version":1`, `"version":1,"invented":true`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(valid, `"bin/kicad-cli"`, `"../kicad-cli"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "clean relative path") {
		t.Fatalf("expected unsafe-path error, got %v", err)
	}
}

func TestResolveUsesExplicitPathsAndHashesLibraries(t *testing.T) {
	root := t.TempDir()
	cli, symbols, footprints := fakeToolchain(t, root, "10.0.3")
	lockPath := filepath.Join(t.TempDir(), "lock.json")
	if err := os.WriteFile(lockPath, []byte(testLockJSON(t, root)), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := Load(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Resolve(context.Background(), document, ResolveOptions{
		OS: "testos", Arch: "testarch", KiCadCLI: cli, SymbolsRoot: symbols, FootprintsRoot: footprints,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Resolution != "explicit" || evidence.KiCadVersion != "10.0.3" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
	if evidence.SymbolsIdentity.FileCount != 1 || evidence.FootprintsIdentity.FileCount != 1 {
		t.Fatalf("unexpected library identities: %+v %+v", evidence.SymbolsIdentity, evidence.FootprintsIdentity)
	}
	first := evidence.SymbolsIdentity.SHA256
	if err := os.WriteFile(filepath.Join(symbols, "Device.kicad_sym"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := HashLibrary(symbols)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SHA256 == first {
		t.Fatal("library content mutation did not change identity")
	}
}

func TestResolveRejectsPartialEnvironmentAndVersionMismatch(t *testing.T) {
	root := t.TempDir()
	cli, symbols, footprints := fakeToolchain(t, root, "10.0.2")
	lockPath := filepath.Join(t.TempDir(), "lock.json")
	if err := os.WriteFile(lockPath, []byte(testLockJSON(t, root)), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := Load(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resolve(context.Background(), document, ResolveOptions{
		OS: "testos", Arch: "testarch", Getenv: func(name string) string {
			if name == "TEST_KICAD_CLI" {
				return cli
			}
			return ""
		},
	})
	if err == nil || !strings.Contains(err.Error(), "provided together") {
		t.Fatalf("expected partial-environment error, got %v", err)
	}
	_, err = Resolve(context.Background(), document, ResolveOptions{
		OS: "testos", Arch: "testarch", KiCadCLI: cli, SymbolsRoot: symbols, FootprintsRoot: footprints,
	})
	if err == nil || !strings.Contains(err.Error(), `does not match lock "10.0.3"`) {
		t.Fatalf("expected version mismatch, got %v", err)
	}
}

func TestDownloadVerifiedRejectsSizeAndChecksumMismatch(t *testing.T) {
	payload := []byte("locked distribution")
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK",
			Body: io.NopCloser(strings.NewReader(string(payload))), Header: make(http.Header),
		}, nil
	})}
	sum := sha256.Sum256(payload)
	distribution := Bootstrap{URL: "https://example.invalid/kicad.dmg", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])}
	if err := DownloadVerified(context.Background(), client, distribution, filepath.Join(t.TempDir(), "ok.dmg")); err != nil {
		t.Fatal(err)
	}
	distribution.SizeBytes++
	if err := DownloadVerified(context.Background(), client, distribution, filepath.Join(t.TempDir(), "size.dmg")); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("expected size mismatch, got %v", err)
	}
	distribution.SizeBytes--
	distribution.SHA256 = strings.Repeat("0", 64)
	if err := DownloadVerified(context.Background(), client, distribution, filepath.Join(t.TempDir(), "hash.dmg")); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func fakeToolchain(t *testing.T, root, version string) (string, string, string) {
	t.Helper()
	cli := filepath.Join(root, "bin", "kicad-cli")
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	for _, directory := range []string{filepath.Dir(cli), symbols, footprints} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(cli, []byte("#!/bin/sh\nprintf '"+version+"\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(symbols, "Device.kicad_sym"), []byte("symbols"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(footprints, "Device.kicad_mod"), []byte("footprints"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cli, symbols, footprints
}

func testLockJSON(t *testing.T, root string) string {
	t.Helper()
	return `{
  "schema":"kicadai.kicad-promotion-toolchain.v1",
  "version":1,
  "kicad_version":"10.0.3",
  "environment":{"kicad_cli":"TEST_KICAD_CLI","symbols_root":"TEST_SYMBOLS_ROOT","footprints_root":"TEST_FOOTPRINTS_ROOT"},
  "platforms":[{
    "os":"testos","arch":"testarch","trusted_roots":[` + quoteJSON(t, root) + `],
    "kicad_cli":"bin/kicad-cli","symbols_root":"symbols","footprints_root":"footprints",
    "bootstrap":{"kind":"dmg","url":"https://example.invalid/kicad.dmg","sha256":"` + strings.Repeat("a", 64) + `","size_bytes":1}
  }]
}`
}

func quoteJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
