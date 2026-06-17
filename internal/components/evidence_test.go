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

func TestValidateCatalogEvidenceElectricalMismatch(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Symbols[0].FunctionPins[0].Electrical = "input"
	index := resolverFixtureForResistor()
	index.Symbols["Device:R"] = libraryresolver.SymbolRecord{
		LibraryID: "Device:R",
		Pins: []libraryresolver.SymbolPin{
			{Number: "1", ElectricalType: "passive"},
			{Number: "2", ElectricalType: "passive"},
		},
	}
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if result.OK {
		t.Fatal("expected electrical mismatch to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentElectricalMismatch)
}

func TestValidateCatalogEvidenceMultiUnitRequiresUnitPolicy(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Symbols[0].SymbolID = "Amplifier:Dual"
	index := resolverFixtureForResistor()
	index.Symbols = map[string]libraryresolver.SymbolRecord{
		"Amplifier:Dual": {
			LibraryID: "Amplifier:Dual",
			Pins: []libraryresolver.SymbolPin{
				{Number: "1", Unit: 1},
				{Number: "2", Unit: 2},
			},
		},
	}
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if result.OK {
		t.Fatal("expected missing unit policy to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentUnitPolicyMissing)
}

func TestValidateCatalogEvidenceExplicitUnitMatchesPinUnit(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Symbols[0].SymbolID = "Amplifier:Dual"
	catalog.Records[0].Symbols[0].Unit = 2
	catalog.Records[0].Symbols[0].FunctionPins = []FunctionPin{{Function: "OUT", SymbolPin: "2", Required: true}}
	index := resolverFixtureForResistor()
	index.Symbols = map[string]libraryresolver.SymbolRecord{
		"Amplifier:Dual": {
			LibraryID: "Amplifier:Dual",
			Pins: []libraryresolver.SymbolPin{
				{Number: "1", Unit: 1},
				{Number: "2", Unit: 2},
				{Number: "8", Unit: 0},
			},
		},
	}
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if !result.OK {
		t.Fatalf("expected explicit unit evidence to pass: %#v", result.Issues)
	}
}

func TestValidateCatalogEvidenceExplicitUnitRejectsWrongPinUnit(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Symbols[0].SymbolID = "Amplifier:Dual"
	catalog.Records[0].Symbols[0].Unit = 2
	catalog.Records[0].Symbols[0].FunctionPins = []FunctionPin{{Function: "IN", SymbolPin: "1", Required: true}}
	index := resolverFixtureForResistor()
	index.Symbols = map[string]libraryresolver.SymbolRecord{
		"Amplifier:Dual": {
			LibraryID: "Amplifier:Dual",
			Pins: []libraryresolver.SymbolPin{
				{Number: "1", Unit: 1},
				{Number: "2", Unit: 2},
			},
		},
	}
	result := ValidateCatalogEvidence(&catalog, EvidenceOptions{LibraryIndex: &index})
	if result.OK {
		t.Fatal("expected wrong unit pin to fail")
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
