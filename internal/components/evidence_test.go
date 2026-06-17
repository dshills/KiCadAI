package components

import (
	"testing"

	"kicadai/internal/libraryresolver"
)

func TestValidateCatalogEvidencePassesWithResolverFixture(t *testing.T) {
	catalog := validCatalog()
	index := resolverFixtureForResistor()
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index, RequirePinmaps: true, AllowPassiveRules: true})
	if !result.OK {
		t.Fatalf("expected evidence validation to pass: %+v", result.Issues)
	}
}

func TestValidateCatalogEvidenceMissingSymbol(t *testing.T) {
	catalog := validCatalog()
	index := resolverFixtureForResistor()
	index.Symbols = map[string]libraryresolver.SymbolRecord{}
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if result.OK {
		t.Fatal("expected missing symbol to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentSymbolUnresolved)
}

func TestValidateCatalogEvidenceMissingFootprint(t *testing.T) {
	catalog := validCatalog()
	index := resolverFixtureForResistor()
	index.Footprints = map[string]libraryresolver.FootprintRecord{}
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if result.OK {
		t.Fatal("expected missing footprint to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentFootprintUnresolved)
}

func TestValidateCatalogEvidenceUnmappedPin(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Symbols[0].FunctionPins[0].SymbolPin = "99"
	index := resolverFixtureForResistor()
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if result.OK {
		t.Fatal("expected unmapped symbol pin to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentFunctionPinUnmapped)
}

func TestValidateCatalogEvidenceUnmappedPad(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Packages[0].PadFunctions[0].Pad = "99"
	index := resolverFixtureForResistor()
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if result.OK {
		t.Fatal("expected unmapped footprint pad to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentPadFunctionUnmapped)
}

func TestValidateCatalogEvidenceMissingPinmapForVerifiedActive(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Family = "opamp"
	catalog.Records[0].Verification.Confidence = ConfidenceVerified
	catalog.Records[0].Symbols[0].SymbolID = "Amplifier_Operational:LMV321"
	catalog.Records[0].Packages[0].FootprintID = "Package_TO_SOT_SMD:SOT-23-5"
	index := libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Amplifier_Operational:LMV321": {LibraryID: "Amplifier_Operational:LMV321", Pins: []libraryresolver.SymbolPin{{Number: "1"}, {Number: "2"}}},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Package_TO_SOT_SMD:SOT-23-5": {FootprintID: "Package_TO_SOT_SMD:SOT-23-5", Pads: []libraryresolver.FootprintPad{{Name: "1"}, {Name: "2"}}},
		},
	}
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index, RequirePinmaps: true})
	if result.OK {
		t.Fatal("expected missing pinmap to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentPinmapMissing)
}

func resolverFixtureForResistor() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {
				LibraryID: "Device:R",
				Pins: []libraryresolver.SymbolPin{
					{Number: "1"},
					{Number: "2"},
				},
			},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0805_2012Metric": {
				FootprintID: "Resistor_SMD:R_0805_2012Metric",
				Pads: []libraryresolver.FootprintPad{
					{Name: "1"},
					{Name: "2"},
				},
			},
		},
	}
}
