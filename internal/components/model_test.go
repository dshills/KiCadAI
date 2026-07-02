package components

import (
	"encoding/json"
	"math"
	"testing"

	"kicadai/internal/reports"
)

func TestModelJSONShape(t *testing.T) {
	catalog := Catalog{
		Version: CatalogVersion,
		Families: []FamilyDefinition{{
			ID:   "resistor",
			Name: "Resistor",
		}},
		Records: []ComponentRecord{{
			ID:      "resistor.generic.0805",
			Family:  "resistor",
			Name:    "Generic 0805 resistor",
			Generic: true,
			Symbols: []SymbolBinding{{
				SymbolID: "Device:R",
				FunctionPins: []FunctionPin{
					{Function: "A", SymbolPin: "1", Required: true},
					{Function: "B", SymbolPin: "2", Required: true},
				},
				Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
			}},
			Packages: []PackageVariant{{
				ID:          "0805",
				Name:        "0805",
				FootprintID: "Resistor_SMD:R_0805_2012Metric",
				PadFunctions: []PadFunction{
					{Function: "A", Pad: "1"},
					{Function: "B", Pad: "2"},
				},
				Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
			}},
			Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
		}},
	}

	body, err := json.Marshal(&catalog)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal catalog: %v", err)
	}
	for _, key := range []string{"version", "families", "records"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("expected JSON key %q in %s", key, string(body))
		}
	}
}

func TestConfidenceValidation(t *testing.T) {
	valid := []ConfidenceLevel{
		ConfidenceVerified,
		ConfidenceLibraryDerived,
		ConfidenceRuleInferred,
		ConfidencePlaceholder,
		ConfidenceBlocked,
	}
	for _, level := range valid {
		if !ValidConfidence(level) {
			t.Fatalf("expected %s to be valid", level)
		}
	}
	if ValidConfidence("maybe") {
		t.Fatal("unexpected valid confidence")
	}
	issue, ok := ValidateConfidenceIssue("component.test", "maybe")
	if !ok {
		t.Fatal("expected confidence issue")
	}
	if issue.Code != CodeInvalidConfidence || issue.Severity != reports.SeverityBlocked {
		t.Fatalf("unexpected issue: %+v", issue)
	}
}

func TestAcceptanceOrderingAndGating(t *testing.T) {
	if CompareAcceptance(AcceptanceDraft, AcceptanceConnectivity) >= 0 {
		t.Fatal("draft should rank below connectivity")
	}
	if CompareAcceptance(AcceptanceFabricationCandidate, AcceptanceERCDRC) <= 0 {
		t.Fatal("fabrication candidate should rank above erc_drc")
	}
	if !AcceptanceAllows(AcceptanceDraft, ConfidencePlaceholder) {
		t.Fatal("draft should allow placeholder")
	}
	if AcceptanceAllows(AcceptanceConnectivity, ConfidencePlaceholder) {
		t.Fatal("connectivity should reject placeholder")
	}
	if AcceptanceAllows(AcceptanceConnectivity, ConfidenceRuleInferred) {
		t.Fatal("generic connectivity should not allow all rule-inferred cases")
	}
	if !AcceptanceAllowsPassiveRuleInferred(AcceptanceConnectivity, ConfidenceRuleInferred) {
		t.Fatal("connectivity should allow explicitly passive rule-inferred cases")
	}
	if AcceptanceAllows(AcceptanceFabricationCandidate, ConfidenceRuleInferred) {
		t.Fatal("fabrication candidate should require verified data")
	}
}

func TestSortCatalogStable(t *testing.T) {
	catalog := Catalog{
		Diagnostics: []reports.Issue{
			{Code: "B", Path: "b", Message: "b"},
			{Code: "A", Path: "a", Message: "a"},
		},
		Families: []FamilyDefinition{
			{ID: "z"},
			{ID: "a"},
		},
		Records: []ComponentRecord{
			{
				ID:   "z",
				Tags: []string{"b", "a"},
				Ratings: []RatingConstraint{
					{Kind: "voltage", Max: "10", Unit: "V"},
					{Kind: "voltage", Max: "2", Unit: "V"},
					{Kind: "voltage", Max: "100", Unit: "mV"},
				},
				Packages: []PackageVariant{
					{ID: "sot23"},
					{ID: "0805"},
				},
				Symbols: []SymbolBinding{
					{SymbolID: "Device:R", Unit: 2},
					{SymbolID: "Device:R", Unit: 1},
				},
			},
			{ID: "a"},
		},
	}

	SortCatalog(&catalog)

	if catalog.Families[0].ID != "a" || catalog.Records[0].ID != "a" {
		t.Fatalf("catalog not sorted: %+v", &catalog)
	}
	record := catalog.Records[1]
	if record.Tags[0] != "a" || record.Packages[0].ID != "0805" || record.Symbols[0].Unit != 1 {
		t.Fatalf("record internals not sorted: %+v", record)
	}
	if record.Ratings[0].Max != "100" || record.Ratings[0].Unit != "mV" {
		t.Fatalf("ratings not sorted by normalized engineering value: %+v", record.Ratings)
	}
	if catalog.Diagnostics[0].Path != "a" {
		t.Fatalf("issues not sorted: %+v", catalog.Diagnostics)
	}
}

func TestParseLeadingEngineeringNumber(t *testing.T) {
	tests := []struct {
		value string
		want  float64
	}{
		{value: "10 k", want: 10000},
		{value: "2u", want: 0.000002},
		{value: "2µ", want: 0.000002},
		{value: "2μ", want: 0.000002},
		{value: "10 units", want: 10},
		{value: "10 max", want: 10},
		{value: "4k7", want: 4700},
		{value: "2R2", want: 2.2},
		{value: "1M5", want: 1500000},
		{value: "3f", want: 0.000000000000003},
		{value: "2T", want: 2000000000000},
	}
	for _, tt := range tests {
		got, ok := parseLeadingEngineeringNumber(tt.value)
		if !ok {
			t.Fatalf("expected %q to parse", tt.value)
		}
		if math.Abs(got-tt.want) > math.Abs(tt.want)*1e-12 {
			t.Fatalf("parseLeadingEngineeringNumber(%q) = %g, want %g", tt.value, got, tt.want)
		}
	}
}
