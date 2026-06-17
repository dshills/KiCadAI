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
