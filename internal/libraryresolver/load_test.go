package libraryresolver

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kicadai/internal/reports"
)

func TestLoadMixedFixtureTree(t *testing.T) {
	roots := mixedLibraryFixture(t)
	index, issues := Load(context.Background(), roots, LoadOptions{})
	if len(issues) != 2 {
		t.Fatalf("expected missing KLC/templates warnings only, got %#v", issues)
	}
	summary := Summary(index)
	if summary.SymbolFileCount != 1 || summary.FootprintFileCount != 1 || summary.SymbolCount != 1 || summary.FootprintCount != 1 || summary.DiagnosticCount != 2 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestLoadResolveExactRecords(t *testing.T) {
	index, _ := Load(context.Background(), mixedLibraryFixture(t), LoadOptions{})
	symbol, ok := ResolveSymbol(index, "Device:R")
	if !ok || symbol.Name != "R" {
		t.Fatalf("symbol = %#v/%v", symbol, ok)
	}
	footprint, ok := ResolveFootprint(index, "Resistor_SMD:R_0805_2012Metric")
	if !ok || footprint.Name != "R_0805_2012Metric" {
		t.Fatalf("footprint = %#v/%v", footprint, ok)
	}
}

func TestFindSymbolsByKeyword(t *testing.T) {
	index, _ := Load(context.Background(), mixedLibraryFixture(t), LoadOptions{})
	records := FindSymbols(index, Query{Text: "resistor"})
	if len(records) != 1 || records[0].LibraryID != "Device:R" {
		t.Fatalf("records = %#v", records)
	}
}

func TestFindFootprintsByTag(t *testing.T) {
	index, _ := Load(context.Background(), mixedLibraryFixture(t), LoadOptions{})
	records := FindFootprints(index, Query{Text: "0805"})
	if len(records) != 1 || records[0].FootprintID != "Resistor_SMD:R_0805_2012Metric" {
		t.Fatalf("records = %#v", records)
	}
}

func TestFindLimitsAndSortsDeterministically(t *testing.T) {
	index := LibraryIndex{
		Symbols: map[string]SymbolRecord{
			"Device:Z": {LibraryID: "Device:Z", Name: "Z"},
			"Device:A": {LibraryID: "Device:A", Name: "A"},
		},
		Footprints: map[string]FootprintRecord{
			"Lib:Z": {FootprintID: "Lib:Z", Name: "Z"},
			"Lib:A": {FootprintID: "Lib:A", Name: "A"},
		},
	}
	symbols := FindSymbols(index, Query{Limit: 1})
	if len(symbols) != 1 || symbols[0].LibraryID != "Device:A" {
		t.Fatalf("symbols = %#v", symbols)
	}
	footprints := FindFootprints(index, Query{Limit: 1})
	if len(footprints) != 1 || footprints[0].FootprintID != "Lib:A" {
		t.Fatalf("footprints = %#v", footprints)
	}
}

func TestFindSymbolsUnicodeCaseInsensitiveSubstring(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{
		"Device:Cafe": {LibraryID: "Device:Cafe", Name: "CaféSensor", Description: "Temperature", SearchText: " cafésensor temperature"},
	}}
	records := FindSymbols(index, Query{Text: "CAFÉ"})
	if len(records) != 1 || records[0].LibraryID != "Device:Cafe" {
		t.Fatalf("records = %#v", records)
	}
}

func TestLoadContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	index, issues := Load(ctx, mixedLibraryFixture(t), LoadOptions{})
	if len(index.Symbols) != 0 {
		t.Fatalf("index = %#v", index)
	}
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Message != context.Canceled.Error() {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestLoadDeterministicRepeatedOutput(t *testing.T) {
	roots := mixedLibraryFixture(t)
	first, _ := Load(context.Background(), roots, LoadOptions{})
	second, _ := Load(context.Background(), roots, LoadOptions{})
	first.GeneratedAt = second.GeneratedAt
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("loads differ\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestLoadCacheWriteReadRoundTrip(t *testing.T) {
	roots := mixedLibraryFixture(t)
	cachePath := filepath.Join(t.TempDir(), "library-index.json")
	first, firstIssues := Load(context.Background(), roots, LoadOptions{CachePath: cachePath})
	if len(firstIssues) != 2 {
		t.Fatalf("first issues = %#v", firstIssues)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	second, secondIssues := Load(context.Background(), roots, LoadOptions{CachePath: cachePath})
	if len(secondIssues) != len(firstIssues) {
		t.Fatalf("second issues = %#v", secondIssues)
	}
	first.GeneratedAt = second.GeneratedAt
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("cached load differs\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestLoadCacheInvalidatedByChangedFileModtime(t *testing.T) {
	roots := mixedLibraryFixture(t)
	cachePath := filepath.Join(t.TempDir(), "library-index.json")
	Load(context.Background(), roots, LoadOptions{CachePath: cachePath})
	symbolPath := filepath.Join(roots.SymbolsRoot, "Device.kicad_sym")
	newTime := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(symbolPath, newTime, newTime); err != nil {
		t.Fatalf("touch symbol file: %v", err)
	}
	_, issues := Load(context.Background(), roots, LoadOptions{CachePath: cachePath})
	if !hasLoadIssue(issues, "cache file metadata changed") {
		t.Fatalf("expected cache invalidation issue: %#v", issues)
	}
}

func TestLoadCacheInvalidatedBySchemaVersion(t *testing.T) {
	roots := mixedLibraryFixture(t)
	cachePath := filepath.Join(t.TempDir(), "library-index.json")
	Load(context.Background(), roots, LoadOptions{CachePath: cachePath})
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	data = []byte(strings.Replace(string(data), `"schema_version":1`, `"schema_version":999`, 1))
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	_, issues := Load(context.Background(), roots, LoadOptions{CachePath: cachePath})
	if !hasLoadIssue(issues, "cache schema version changed") {
		t.Fatalf("expected schema invalidation issue: %#v", issues)
	}
}

func TestLoadCacheRefreshSkipsCorruptCache(t *testing.T) {
	roots := mixedLibraryFixture(t)
	cachePath := filepath.Join(t.TempDir(), "library-index.json")
	if err := os.WriteFile(cachePath, []byte(`not-json`), 0o644); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}
	_, issues := Load(context.Background(), roots, LoadOptions{CachePath: cachePath, Refresh: true})
	if hasLoadIssue(issues, "read cache") {
		t.Fatalf("refresh should not read corrupt cache: %#v", issues)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read refreshed cache: %v", err)
	}
	if !strings.Contains(string(data), `"schema_version":1`) {
		t.Fatalf("cache was not refreshed:\n%s", data)
	}
}

func TestLoadCorruptCacheFallsBackToRebuild(t *testing.T) {
	roots := mixedLibraryFixture(t)
	cachePath := filepath.Join(t.TempDir(), "library-index.json")
	if err := os.WriteFile(cachePath, []byte(`not-json`), 0o644); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}
	index, issues := Load(context.Background(), roots, LoadOptions{CachePath: cachePath})
	if Summary(index).SymbolCount != 1 || Summary(index).FootprintCount != 1 {
		t.Fatalf("index did not rebuild: %#v", Summary(index))
	}
	if !hasLoadIssue(issues, "read cache") {
		t.Fatalf("expected corrupt cache diagnostic: %#v", issues)
	}
}

func mixedLibraryFixture(t *testing.T) LibraryRoots {
	t.Helper()
	root := t.TempDir()
	symbols := root + "/symbols"
	footprints := root + "/footprints"
	mustWrite(t, symbols+"/Device.kicad_sym", resistorSymbolLibrary())
	mustWrite(t, footprints+"/Resistor_SMD.pretty/R_0805_2012Metric.kicad_mod", resistor0805Footprint())
	return LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints}
}

func hasLoadIssue(issues []reports.Issue, contains string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, contains) {
			return true
		}
	}
	return false
}
