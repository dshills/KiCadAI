package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var updateLibrarySymbolGoldens = flag.Bool("update-library-symbol-goldens", false, "update library symbol CLI golden files")

var librarySymbolGoldenOffsetPattern = regexp.MustCompile(`at offset [0-9]+:`)

func TestRunLibrarySymbolsGolden(t *testing.T) {
	symbolsRoot := filepath.FromSlash("testdata/library_symbols/symbols")
	blockedSymbolsRoot := filepath.FromSlash("testdata/library_symbols_blocked/symbols")
	goldenDir := filepath.FromSlash("testdata/golden/library_symbols")

	for _, tc := range []struct {
		name      string
		args      []string
		symbols   string
		wantError bool
	}{
		{
			name:    "list",
			args:    []string{"library", "symbols", "list"},
			symbols: symbolsRoot,
		},
		{
			name:    "show_device_r",
			args:    []string{"library", "symbols", "show", "Device:R"},
			symbols: symbolsRoot,
		},
		{
			name:    "pins_dual_opamp",
			args:    []string{"library", "symbols", "pins", "Amplifier:Dual_OpAmp"},
			symbols: symbolsRoot,
		},
		{
			name:      "validation_blocked",
			args:      []string{"library", "symbols", "validate", "Device:R"},
			symbols:   blockedSymbolsRoot,
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--symbols-root", tc.symbols}, tc.args...)
			err := run(args, &stdout, &stderr)
			if tc.wantError && err == nil {
				t.Fatalf("expected error\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
			}
			if !tc.wantError && err != nil {
				t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
			}

			got := normalizeLibrarySymbolGolden(stdout.String())
			path := filepath.Join(goldenDir, tc.name+".json")
			if *updateLibrarySymbolGoldens {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir golden dir: %v", err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			wantBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v", path, err)
			}
			want := strings.ReplaceAll(string(wantBytes), "\r\n", "\n")
			if got != want {
				t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, want)
			}
		})
	}
}

func normalizeLibrarySymbolGolden(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	replacements := []struct {
		path        string
		placeholder string
	}{
		{filepath.Join("testdata", "library_symbols_blocked", "symbols"), "$BLOCKED_SYMBOLS_ROOT"},
		{filepath.Join("testdata", "library_symbols", "symbols"), "$SYMBOLS_ROOT"},
	}
	sort.Slice(replacements, func(i, j int) bool {
		return len(replacements[i].path) > len(replacements[j].path)
	})
	for _, replacement := range replacements {
		for _, variant := range librarySymbolGoldenPathVariants(replacement.path) {
			text = strings.ReplaceAll(text, variant, replacement.placeholder)
		}
		text = strings.ReplaceAll(text, replacement.placeholder+`\\`, replacement.placeholder+"/")
		text = strings.ReplaceAll(text, replacement.placeholder+"\\", replacement.placeholder+"/")
	}
	text = librarySymbolGoldenOffsetPattern.ReplaceAllString(text, "at offset $$OFFSET:")
	return text
}

func librarySymbolGoldenPathVariants(path string) []string {
	slashPath := filepath.ToSlash(path)
	escapedNativePath := strings.ReplaceAll(path, `\`, `\\`)
	escapedSlashPath := strings.ReplaceAll(slashPath, `/`, `\/`)
	return []string{path, slashPath, escapedNativePath, escapedSlashPath}
}

func TestExternalKiCadSymbolsSmoke(t *testing.T) {
	if os.Getenv("KICADAI_RUN_EXTERNAL_SYMBOL_TESTS") != "1" {
		t.Skip("set KICADAI_RUN_EXTERNAL_SYMBOL_TESTS=1 to run external KiCad symbol smoke test")
	}
	symbolsRoot := strings.TrimSpace(os.Getenv("KICAD_SYMBOLS_DIR"))
	if symbolsRoot == "" {
		t.Fatal("KICAD_SYMBOLS_DIR must be set when KICADAI_RUN_EXTERNAL_SYMBOL_TESTS=1")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cachePath := filepath.Join(t.TempDir(), "library-index.json")
	err := run([]string{"--json", "--symbols-root", symbolsRoot, "--library-cache", cachePath, "library", "symbols", "show", "Device:R"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("external symbol smoke failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var response struct {
		Command string `json:"command"`
		Data    struct {
			LibraryID string `json:"library_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode external smoke output: %v\n%s", err, stdout.String())
	}
	if response.Command != "library" || response.Data.LibraryID != "Device:R" {
		t.Fatalf("unexpected external smoke response: %#v\n%s", response, stdout.String())
	}
}
