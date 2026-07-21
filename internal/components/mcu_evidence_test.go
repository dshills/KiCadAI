package components

import (
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestValidateMCUEvidenceAcceptsMappedResources(t *testing.T) {
	catalog := validMCUCatalog()
	result := ValidateCatalog(&catalog)
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("valid MCU evidence rejected: %+v", result.Issues)
	}
}

func TestValidateMCUEvidenceRequiredForVerifiedMCU(t *testing.T) {
	catalog := validMCUCatalog()
	catalog.Records[0].MCU = nil
	result := ValidateCatalog(&catalog)
	assertIssueCode(t, result.Issues, CodeInvalidMetadata)
}

func TestValidateMCUEvidenceRejectsUnmappedPhysicalFunction(t *testing.T) {
	catalog := validMCUCatalog()
	catalog.Records[0].MCU.Pins[0].Function = "PB99"
	result := ValidateCatalog(&catalog)
	assertIssueCode(t, result.Issues, CodeInvalidMetadata)
}

func TestValidateMCUEvidenceRequiresPinDomainsForIndependentRails(t *testing.T) {
	catalog := validMCUCatalog()
	catalog.Records[0].MCU.SupplyDomains = append(catalog.Records[0].MCU.SupplyDomains, MCUSupplyDomain{
		ID: "io", PowerFunctions: []string{"VDD"}, GroundFunctions: []string{"VSS"}, MinimumV: 3.0, MaximumV: 3.6,
	})
	result := ValidateCatalog(&catalog)
	for _, issue := range result.Issues {
		if issue.Path == "records[0].mcu_evidence.pins[0].supply_domain" {
			return
		}
	}
	t.Fatalf("independent supply rails did not require per-pin domains: %+v", result.Issues)
}

func TestSortMCUEvidenceIgnoresInputOrdering(t *testing.T) {
	left := validMCUCatalog()
	right := validMCUCatalog()
	right.Records[0].MCU.Pins[0].AlternateFunctions[0], right.Records[0].MCU.Pins[0].AlternateFunctions[1] = right.Records[0].MCU.Pins[0].AlternateFunctions[1], right.Records[0].MCU.Pins[0].AlternateFunctions[0]
	right.Records[0].MCU.ClockOptions[0], right.Records[0].MCU.ClockOptions[1] = right.Records[0].MCU.ClockOptions[1], right.Records[0].MCU.ClockOptions[0]
	SortCatalog(&left)
	SortCatalog(&right)
	if !reflect.DeepEqual(left.Records[0].MCU, right.Records[0].MCU) {
		t.Fatalf("normalized MCU evidence differs:\nleft=%#v\nright=%#v", left.Records[0].MCU, right.Records[0].MCU)
	}
}

func validMCUCatalog() Catalog {
	return Catalog{
		Version:  CatalogVersion,
		Families: []FamilyDefinition{{ID: "mcu", Name: "MCU"}},
		Records: []ComponentRecord{{
			ID:           "mcu.test.part",
			Family:       "mcu",
			Name:         "Test MCU",
			Manufacturer: "Test",
			MPN:          "MCU1",
			Symbols: []SymbolBinding{{
				SymbolID: "MCU_Test:MCU1",
				FunctionPins: []FunctionPin{
					{Function: "VDD", SymbolPin: "1", Required: true},
					{Function: "VSS", SymbolPin: "2", Required: true},
					{Function: "PA0", SymbolPin: "3"},
					{Function: "PA1", SymbolPin: "4"},
				},
				Verification: VerificationRecord{Confidence: ConfidenceVerified},
			}},
			Packages: []PackageVariant{{
				ID: "package", Name: "Package", FootprintID: "Package:MCU1",
				PadFunctions: []PadFunction{{Function: "VDD", Pad: "1"}, {Function: "VSS", Pad: "2"}, {Function: "PA0", Pad: "3"}, {Function: "PA1", Pad: "4"}},
				Verification: VerificationRecord{Confidence: ConfidenceVerified},
			}},
			MCU: &MCUEvidence{
				Architecture:  "test8",
				Family:        "test",
				SupplyDomains: []MCUSupplyDomain{{ID: "main", PowerFunctions: []string{"VDD"}, GroundFunctions: []string{"VSS"}, MinimumV: 1.8, MaximumV: 3.6}},
				Pins: []MCUPinEvidence{
					{Function: "PA0", GPIO: "PA0", ElectricalModes: []string{"open_drain", "push_pull"}, AlternateFunctions: []MCUAlternateFunction{{Kind: "uart", Instance: "uart1", Signal: "tx"}, {Kind: "i2c", Instance: "i2c1", Signal: "sda", Mode: "open_drain"}}},
					{Function: "PA1", GPIO: "PA1", ElectricalModes: []string{"open_drain", "push_pull"}, AlternateFunctions: []MCUAlternateFunction{{Kind: "uart", Instance: "uart1", Signal: "rx"}, {Kind: "i2c", Instance: "i2c1", Signal: "scl", Mode: "open_drain"}}},
				},
				ProgrammingInterfaces: []MCUProgrammingInterface{{ID: "serial", Kind: "serial", Signals: []MCUInterfaceSignal{{Signal: "tx", PinFunction: "PA0"}, {Signal: "rx", PinFunction: "PA1"}}}},
				ClockOptions:          []MCUClockOption{{ID: "external", Kind: "external", MaximumHz: 16_000_000}, {ID: "internal", Kind: "internal", MaximumHz: 8_000_000, Default: true}},
				CurrentBudget: &MCUCurrentBudget{
					MaximumSourcePerPinMA: float64MCUPointer(20), MaximumSinkPerPinMA: float64MCUPointer(20), MaximumAggregateMA: float64MCUPointer(80),
				},
				ReviewNote: "Test-only evidence.",
			},
			Verification: VerificationRecord{Confidence: ConfidenceVerified},
		}},
	}
}

func float64MCUPointer(value float64) *float64 {
	return &value
}
