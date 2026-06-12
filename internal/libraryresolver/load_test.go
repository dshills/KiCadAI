package libraryresolver

import (
	"context"
	"reflect"
	"testing"
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

func mixedLibraryFixture(t *testing.T) LibraryRoots {
	t.Helper()
	root := t.TempDir()
	symbols := root + "/symbols"
	footprints := root + "/footprints"
	mustWrite(t, symbols+"/Device.kicad_sym", resistorSymbolLibrary())
	mustWrite(t, footprints+"/Resistor_SMD.pretty/R_0805_2012Metric.kicad_mod", resistor0805Footprint())
	return LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints}
}
