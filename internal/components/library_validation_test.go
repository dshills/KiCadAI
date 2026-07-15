package components

import (
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

func TestValidateCatalogLibrariesAcceptsResolvedBindings(t *testing.T) {
	catalog := validCatalog()
	index := resolvedResistorLibraryIndex()
	summary, issues := ValidateCatalogLibraries(&catalog, index)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if !summary.Configured || summary.SelectableRecords != 1 || summary.SymbolBindingsChecked != 1 || summary.PackageVariantsChecked != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestValidateCatalogLibrariesAggregatesDeterministically(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Symbols[0].Unit = 2
	catalog.Records[0].Symbols[0].FunctionPins = append(catalog.Records[0].Symbols[0].FunctionPins, FunctionPin{Function: "MISSING", SymbolPin: "9"})
	catalog.Records[0].Packages[0].PadFunctions = append(catalog.Records[0].Packages[0].PadFunctions, PadFunction{Function: "MISSING", Pad: "9"})
	index := resolvedResistorLibraryIndex()
	delete(index.Footprints, "Resistor_SMD:R_0805_2012Metric")

	_, first := ValidateCatalogLibraries(&catalog, index)
	_, second := ValidateCatalogLibraries(&catalog, index)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("issues are not deterministic:\n%#v\n%#v", first, second)
	}
	wantCodes := []reports.Code{CodeLibraryFootprintMissing, CodeLibrarySymbolUnitMissing}
	gotCodes := make([]reports.Code, 0, len(first))
	for _, issue := range first {
		gotCodes = append(gotCodes, issue.Code)
	}
	if !slices.Equal(gotCodes, wantCodes) {
		t.Fatalf("codes = %v, want %v; issues=%#v", gotCodes, wantCodes, first)
	}
}

func TestValidateCatalogLibrariesReportsMissingSymbolAndPin(t *testing.T) {
	catalog := validCatalog()
	index := resolvedResistorLibraryIndex()
	delete(index.Symbols, "Device:R")
	_, issues := ValidateCatalogLibraries(&catalog, index)
	assertIssueCode(t, issues, CodeLibrarySymbolMissing)

	index = resolvedResistorLibraryIndex()
	catalog.Records[0].Symbols[0].FunctionPins[1].SymbolPin = "9"
	_, issues = ValidateCatalogLibraries(&catalog, index)
	assertIssueCode(t, issues, CodeLibrarySymbolPinMissing)
}

func TestValidateCatalogLibrariesReportsMissingPad(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Packages[0].PadFunctions[1].Pad = "9"
	_, issues := ValidateCatalogLibraries(&catalog, resolvedResistorLibraryIndex())
	assertIssueCode(t, issues, CodeLibraryFootprintPadMissing)
}

func TestValidateCatalogLibrariesSkipsBlockedRecords(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Verification.Confidence = ConfidenceBlocked
	index := libraryresolver.LibraryIndex{Roots: libraryresolver.LibraryRoots{SymbolsRoot: "/symbols", FootprintsRoot: "/footprints"}}
	summary, issues := ValidateCatalogLibraries(&catalog, index)
	if len(issues) != 0 || summary.SelectableRecords != 0 {
		t.Fatalf("summary=%#v issues=%#v", summary, issues)
	}
}

func resolvedResistorLibraryIndex() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Roots: libraryresolver.LibraryRoots{SymbolsRoot: "/symbols", FootprintsRoot: "/footprints"},
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {
				LibraryID: "Device:R",
				Units:     []libraryresolver.SymbolUnit{{Unit: 1}},
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Unit: 1, Position: kicadfiles.Point{}},
					{Number: "2", Unit: 1, Position: kicadfiles.Point{}},
				},
			},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0805_2012Metric": {
				FootprintID: "Resistor_SMD:R_0805_2012Metric",
				Pads:        []libraryresolver.FootprintPad{{Name: "1"}, {Name: "2"}},
			},
		},
	}
}
