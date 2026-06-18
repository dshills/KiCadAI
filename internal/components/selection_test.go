package components

import (
	"context"
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
	catalog := loadCheckedInCatalog(t)
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
	catalog := loadCheckedInCatalog(t)
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
