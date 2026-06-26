package components

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSelectGenericResistorByPackageAndValue(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "resistor",
			Package:   "0805",
			ValueKind: "resistance",
			Value:     "10k",
		},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select resistor failed: %+v", result.Issues)
	}
	if selection.Component.ID != "resistor.generic.0805" {
		t.Fatalf("selected %s", selection.Component.ID)
	}
}

func TestSelectRejectsCapacitorBelowVoltageRating(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:  "capacitor",
			Package: "0805",
		},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "voltage",
			Value: "50",
			Unit:  "V",
		}},
	})
	if result.OK {
		t.Fatal("expected voltage rating rejection")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectGeneric0603CapacitorByPackage(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "capacitor",
			Package:   "0603",
			ValueKind: "capacitance",
			Value:     "100n",
		},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select capacitor failed: %+v", result.Issues)
	}
	if selection.Component.ID != "capacitor.ceramic.0603" {
		t.Fatalf("selected %s", selection.Component.ID)
	}
}

func TestSelectRegulatorCompanionCapacitorWithRatings(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "capacitor",
			Package:   "0805",
			ValueKind: "capacitance",
			Value:     "10u",
		},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "voltage",
			Value: "5",
			Unit:  "V",
		}},
	})
	if !result.OK {
		t.Fatalf("select regulator capacitor failed: %+v", result.Issues)
	}
	if selection.Component.ID != "capacitor.ceramic.0805" || selection.Candidate.Confidence != ConfidenceRuleInferred {
		t.Fatalf("unexpected capacitor selection: ID=%q candidate=%+v", selection.Component.ID, selection.Candidate)
	}
}

func TestFindConnectorByPinCountAndPackage(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	candidates, result := Find(context.Background(), catalog, Query{
		Family:    "connector",
		Package:   "1x04",
		ValueKind: "pin_count",
		Value:     "4",
	})
	if !result.OK {
		t.Fatalf("find connector failed: %+v", result.Issues)
	}
	if len(candidates) != 1 || candidates[0].ComponentID != "connector.pinheader.1x04.2_54mm" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestFindConnectorByThreePinCount(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	candidates, result := Find(context.Background(), catalog, Query{
		Family:    "connector",
		Package:   "1x03",
		ValueKind: "pin_count",
		Value:     "3",
	})
	if !result.OK {
		t.Fatalf("find connector failed: %+v", result.Issues)
	}
	if len(candidates) != 1 || candidates[0].ComponentID != "connector.pinheader.1x03.2_54mm" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestSelectVerifiedSignalDiodeForConnectivity(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "diode", Package: "sod_123"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select diode failed: %+v", result.Issues)
	}
	if selection.Candidate.Confidence != ConfidenceVerified {
		t.Fatalf("expected verified diode, got %+v", selection.Candidate)
	}
	wantPads := map[string]string{"CATHODE": "1", "ANODE": "2"}
	for _, padFunction := range selection.Variant.PadFunctions {
		if want, ok := wantPads[padFunction.Function]; ok && padFunction.Pad != want {
			t.Fatalf("diode %s mapped to pad %s, want %s", padFunction.Function, padFunction.Pad, want)
		}
	}
	for function := range wantPads {
		found := false
		for _, padFunction := range selection.Variant.PadFunctions {
			if padFunction.Function == function {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("diode missing %s pad function: %+v", function, selection.Variant.PadFunctions)
		}
	}
}

func TestSelectVerifiedRegulatorForPowerRequest(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "regulator",
			Package:   "sot223",
			ValueKind: "output_voltage",
			Value:     "3.3",
		},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "5", Unit: "V"},
			{Kind: "output_current", Value: "500", Unit: "mA"},
		},
	})
	if !result.OK {
		t.Fatalf("select regulator failed: %+v", result.Issues)
	}
	if selection.Component.ID != "regulator.linear.ams1117_3v3.sot223" || selection.Candidate.Confidence != ConfidenceVerified {
		t.Fatalf("unexpected regulator selection: %+v", selection.Candidate)
	}
	if len(selection.Component.Companions) < 2 {
		t.Fatalf("expected regulator companion requirements: %+v", selection.Component.Companions)
	}
}

func TestSelectVerifiedRegulatorRequiresCompanions(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "regulator", Package: "sot223", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:        AcceptanceConnectivity,
		RequireConcrete:   true,
		RequireCompanions: true,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "5", Unit: "V"},
			{Kind: "output_current", Value: "250", Unit: "mA"},
		},
	})
	if !result.OK {
		t.Fatalf("select regulator with companions failed: %+v", result.Issues)
	}
	if selection.Component.ID != "regulator.linear.ams1117_3v3.sot223" {
		t.Fatalf("unexpected regulator selection: got ID %q, want %q", selection.Component.ID, "regulator.linear.ams1117_3v3.sot223")
	}
	for _, role := range []string{"input_capacitor", "output_capacitor"} {
		if !componentHasRequiredCompanionRole(selection.Component, role) {
			t.Fatalf("selected regulator missing companion role %s: %+v", role, selection.Component.Companions)
		}
	}
}

func TestSelectRejectsRegulatorOverCurrent(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "regulator", Package: "sot223"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "output_current",
			Value: "2",
			Unit:  "A",
		}},
	})
	if result.OK {
		t.Fatal("expected regulator over-current request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectRejectsPlaceholderForConnectivity(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", "unsafe_placeholder")})
	if err != nil {
		t.Fatalf("load unsafe placeholder fixture: %v", err)
	}
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "opamp"},
		Acceptance: AcceptanceConnectivity,
	})
	if result.OK {
		t.Fatal("expected placeholder opamp to be rejected")
	}
	assertIssueCode(t, result.Issues, CodeComponentUnsafe)
}

func TestSelectAllowsPlaceholderForDraft(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", "unsafe_placeholder")})
	if err != nil {
		t.Fatalf("load unsafe placeholder fixture: %v", err)
	}
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "opamp"},
		Acceptance: AcceptanceDraft,
	})
	if !result.OK {
		t.Fatalf("draft placeholder select failed: %+v", result.Issues)
	}
	if selection.Candidate.Confidence != ConfidencePlaceholder {
		t.Fatalf("expected placeholder confidence, got %s", selection.Candidate.Confidence)
	}
	if len(selection.Warnings) == 0 {
		t.Fatal("expected placeholder warning")
	}
}

func TestSelectVerifiedOpAmpBySupplyRange(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "supply_voltage",
			Value: "3.3",
			Unit:  "V",
		}},
	})
	if !result.OK {
		t.Fatalf("select opamp failed: %+v", result.Issues)
	}
	if selection.Component.ID != "opamp.ti.lmv321.sot23_5" {
		t.Fatalf("unexpected opamp selection: %+v", selection.Candidate)
	}
}

func TestSelectRejectsOpAmpOutsideSupplyRange(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "supply_voltage",
			Value: "6",
			Unit:  "V",
		}},
	})
	if result.OK {
		t.Fatal("expected opamp over-voltage request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectRejectsOpAmpBelowMinimumSupplyRange(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "supply_voltage",
			Value: "1.8",
			Unit:  "V",
		}},
	})
	if result.OK {
		t.Fatal("expected opamp under-voltage request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectWithRejectedPlaceholderAlternativeStillSucceeds(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("expected verified opamp to win despite rejected placeholder: %+v", result.Issues)
	}
	if selection.Component.ID != "opamp.ti.lmv321.sot23_5" {
		t.Fatalf("unexpected selection: %+v", selection.Candidate)
	}
	if len(selection.Rejected) == 0 {
		t.Fatal("expected rejected placeholder diagnostics")
	}
}

func TestSelectRequiresFunction(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "usb_c", Package: "6p"},
		Acceptance:        AcceptanceConnectivity,
		RequiredFunctions: []string{"CC1", "CC2"},
	})
	if !result.OK {
		t.Fatalf("expected usb-c function selection to pass: %+v", result.Issues)
	}
	if selection.Component.ID != "usb_c.gct.usb4125_power_only_6p" {
		t.Fatalf("unexpected selection: %+v", selection.Candidate)
	}

	_, result = Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "usb_c", Package: "6p"},
		Acceptance:        AcceptanceConnectivity,
		RequiredFunctions: []string{"D_PLUS"},
	})
	if result.OK {
		t.Fatal("expected missing USB data function to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentFunctionMissing)
}

func TestSelectRequiresConcreteAndCompanions(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:           Query{Family: "resistor", Package: "0805"},
		Acceptance:      AcceptanceConnectivity,
		RequireConcrete: true,
	})
	if result.OK {
		t.Fatal("expected generic resistor to fail concrete requirement")
	}
	assertIssueCode(t, result.Issues, CodeComponentConcreteRequired)

	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance:        AcceptanceConnectivity,
		RequireConcrete:   true,
		RequireCompanions: true,
	})
	if !result.OK {
		t.Fatalf("expected concrete opamp with companions to pass: %+v", result.Issues)
	}
	if selection.Component.ID != "opamp.ti.lmv321.sot23_5" {
		t.Fatalf("unexpected selection: %+v", selection.Candidate)
	}
}

func TestSelectVerifiedMCUAndRequiredFunctions(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "microchip", Family: "mcu", Package: "tqfp32"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select mcu failed: %+v", result.Issues)
	}
	for _, fn := range []string{"VCC", "GND", "RESET"} {
		if !componentHasFunction(selection.Component, fn) {
			t.Fatalf("selected MCU missing function %s", fn)
		}
	}
}

func TestSelectVerifiedI2CSensor(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "sensor", Package: "lga8"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select sensor failed: %+v", result.Issues)
	}
	for _, fn := range []string{"VDD", "GND", "SDA", "SCL"} {
		if !componentHasFunction(selection.Component, fn) {
			t.Fatalf("selected sensor missing function %s", fn)
		}
	}
}

func TestSelectVerifiedCrystal(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Text:      "generic",
			Family:    "crystal",
			Package:   "5032",
			ValueKind: "frequency",
			Value:     "16",
		},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select crystal failed: %+v", result.Issues)
	}
	if selection.Component.ID != "crystal.generic.5032_2pin" {
		t.Fatalf("unexpected crystal selection: %+v", selection.Candidate)
	}
}

func TestSelectConcreteCrystalForFabricationCandidate(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "crystal",
			Package:   "5032",
			ValueKind: "frequency",
			Value:     "16",
		},
		Acceptance:        AcceptanceFabricationCandidate,
		RequireConcrete:   true,
		RequireCompanions: true,
	})
	if !result.OK {
		t.Fatalf("select concrete crystal failed: %+v", result.Issues)
	}
	if selection.Component.ID != "crystal.abracon.abm3_16mhz.5032_2pin" {
		t.Fatalf("selected component ID = %s", selection.Component.ID)
	}
	if selection.Component.Manufacturer != "Abracon" || selection.Component.MPN != "ABM3-16.000MHZ-D2Y-T" {
		t.Fatalf("selected crystal missing manufacturer evidence: %+v", selection.Component)
	}
	for _, fn := range []string{"XTAL_1", "XTAL_2"} {
		if !componentHasFunction(selection.Component, fn) {
			t.Fatalf("selected crystal missing function %s", fn)
		}
	}
}

func TestSelectVerifiedUSBCPowerOnlyConnector(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "usb_c", Package: "6p"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "current",
			Value: "3",
			Unit:  "A",
		}},
	})
	if !result.OK {
		t.Fatalf("select usb-c failed: %+v", result.Issues)
	}
	if !componentHasFunction(selection.Component, "CC1") || !componentHasFunction(selection.Component, "CC2") {
		t.Fatalf("selected USB-C record missing CC pins")
	}
}

func TestSelectReportsAmbiguousEqualCandidates(t *testing.T) {
	catalog := &Catalog{
		Families: []FamilyDefinition{{ID: "resistor", Name: "Resistor"}},
		Records: []ComponentRecord{
			testSelectableResistor("resistor.a"),
			testSelectableResistor("resistor.b"),
		},
	}
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "resistor", Package: "0805"},
		Acceptance: AcceptanceConnectivity,
	})
	if result.OK {
		t.Fatal("expected ambiguous selection")
	}
	assertIssueCode(t, result.Issues, CodeComponentAmbiguous)
}

func TestResolveBinding(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	resolved, result := ResolveBinding(context.Background(), catalog, "resistor.generic.0805", "0805")
	if !result.OK {
		t.Fatalf("resolve binding failed: %+v", result.Issues)
	}
	if resolved.Symbol.SymbolID != "Device:R" || resolved.Variant.FootprintID == "" {
		t.Fatalf("unexpected resolved binding: %+v", resolved)
	}
}

func loadCheckedInCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	return catalog
}

func componentHasFunction(record ComponentRecord, function string) bool {
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if pin.Function == function {
				return true
			}
		}
	}
	return false
}

func componentHasRequiredCompanionRole(record ComponentRecord, role string) bool {
	for _, companion := range record.Companions {
		if companion.Role == role && companion.Required {
			return true
		}
	}
	return false
}

func testSelectableResistor(id string) ComponentRecord {
	return ComponentRecord{
		ID:      id,
		Family:  "resistor",
		Name:    id,
		Generic: true,
		Symbols: []SymbolBinding{{
			SymbolID:     "Device:R",
			Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
		}},
		Packages: []PackageVariant{{
			ID:           "0805",
			FootprintID:  "Resistor_SMD:R_0805_2012Metric",
			Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
		}},
		Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
	}
}
