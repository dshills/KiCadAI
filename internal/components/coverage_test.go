package components

import (
	"context"
	"testing"

	"kicadai/internal/reports"
)

func TestComponentCoverageCountsCatalog(t *testing.T) {
	catalog := validCatalog()
	report, result := ComponentCoverage(&catalog, CoverageOptions{RequiredFamilies: []string{"resistor"}})
	if !result.OK {
		t.Fatalf("coverage failed: %+v", result.Issues)
	}
	if report.RecordCount != 1 {
		t.Fatalf("expected one record, got %d", report.RecordCount)
	}
	if report.ConfidenceCounts[string(ConfidenceRuleInferred)] != 1 {
		t.Fatalf("expected one rule-inferred record, got %+v", report.ConfidenceCounts)
	}
	if len(report.Families) != 1 || report.Families[0].Family != "resistor" {
		t.Fatalf("unexpected family coverage: %+v", report.Families)
	}
}

func TestComponentCoverageReportsMissingRequiredFamily(t *testing.T) {
	catalog := validCatalog()
	_, result := ComponentCoverage(&catalog, CoverageOptions{RequiredFamilies: []string{"regulator"}})
	assertIssueCode(t, result.Issues, CodeComponentCoverageMissing)
}

func TestComponentCoverageReportsPlaceholderOnlyFamily(t *testing.T) {
	catalog := validCatalog()
	catalog.Families = append(catalog.Families, FamilyDefinition{ID: "opamp", Name: "Op-Amp"})
	catalog.Records = append(catalog.Records, ComponentRecord{
		ID:      "opamp.placeholder",
		Family:  "opamp",
		Name:    "Placeholder op-amp",
		Generic: true,
		Symbols: []SymbolBinding{{
			SymbolID:     "Amplifier_Operational:LMV321",
			FunctionPins: []FunctionPin{{Function: "OUT", SymbolPin: "4", Required: true}},
			Verification: VerificationRecord{Confidence: ConfidencePlaceholder},
		}},
		Packages: []PackageVariant{{
			ID:           "sot23_5",
			Name:         "SOT-23-5",
			FootprintID:  "Package_TO_SOT_SMD:SOT-23-5",
			PadFunctions: []PadFunction{{Function: "OUT", Pad: "4"}},
			Verification: VerificationRecord{Confidence: ConfidencePlaceholder},
		}},
		Verification: VerificationRecord{Confidence: ConfidencePlaceholder},
	})
	_, result := ComponentCoverage(&catalog, CoverageOptions{RequiredFamilies: []string{"opamp"}})
	assertIssueCode(t, result.Issues, CodeComponentCoveragePlaceholder)
}

func TestCheckedInCatalogCoverageIsDeterministic(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	report, result := ComponentCoverage(catalog, CoverageOptions{})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("coverage should not block: %+v", result.Issues)
	}
	if report.RecordCount != len(catalog.Records) {
		t.Fatalf("coverage record count mismatch: %d != %d", report.RecordCount, len(catalog.Records))
	}
	for i := 1; i < len(report.Families); i++ {
		if report.Families[i-1].Family > report.Families[i].Family {
			t.Fatalf("families not sorted: %+v", report.Families)
		}
	}
}
