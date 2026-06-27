package components

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

var verifiedExpansionTargetFamilies = []string{"connector", "crystal", "diode", "protection", "usb_c"}

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

func TestComponentCoverageReportsAlternativeMetrics(t *testing.T) {
	catalog := validCatalog()
	concrete := catalog.Records[0]
	concrete.ID = "resistor.concrete.0805"
	concrete.Generic = false
	concrete.Manufacturer = "Example"
	concrete.MPN = "EX-10K"
	concrete.Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
	missingMPN := concrete
	missingMPN.ID = "resistor.concrete.no_mpn"
	missingMPN.MPN = ""
	missingMPN.Equivalence = &EquivalenceMetadata{Group: "resistor.no_mpn.0805", Role: EquivalenceAlternate}
	duplicatePreferred := concrete
	duplicatePreferred.ID = "resistor.concrete.duplicate_preferred"
	duplicatePreferred.Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
	catalog.Records = append(catalog.Records, concrete, missingMPN, duplicatePreferred)

	report, result := ComponentCoverage(&catalog, CoverageOptions{RequiredFamilies: []string{"resistor"}})
	if !result.OK {
		t.Fatalf("coverage failed: %+v", result.Issues)
	}
	if report.AlternativeCoverage.ConcreteRecords != 3 || report.AlternativeCoverage.GenericFallbackRecords != 1 {
		t.Fatalf("alternative counts = %+v", report.AlternativeCoverage)
	}
	if report.AlternativeCoverage.EquivalenceGroups != 2 {
		t.Fatalf("equivalence groups = %+v", report.AlternativeCoverage)
	}
	if !slices.Contains(report.AlternativeCoverage.GroupsMissingPreferred, "resistor:resistor.no_mpn.0805") {
		t.Fatalf("missing preferred groups = %+v", report.AlternativeCoverage.GroupsMissingPreferred)
	}
	if !slices.Contains(report.AlternativeCoverage.GroupsWithDuplicatePreferred, "resistor:resistor.10k.0805") {
		t.Fatalf("duplicate preferred groups = %+v", report.AlternativeCoverage.GroupsWithDuplicatePreferred)
	}
	if !slices.Contains(report.AlternativeCoverage.ConcreteRecordsMissingMPN, "resistor.concrete.no_mpn") {
		t.Fatalf("missing MPN records = %+v", report.AlternativeCoverage.ConcreteRecordsMissingMPN)
	}
	family, ok := familyCoverageByID(report, "resistor")
	if !ok {
		t.Fatal("missing resistor coverage")
	}
	if family.ConcreteRecords != 3 || family.GenericFallbackRecords != 1 || family.EquivalenceGroups != 2 {
		t.Fatalf("family alternative counts = %+v", family)
	}
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

func TestCheckedInCatalogIncludesVerifiedExpansionFamilies(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	report, result := ComponentCoverage(catalog, CoverageOptions{RequiredFamilies: verifiedExpansionTargetFamilies})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("coverage should not block: %+v", result.Issues)
	}
	for _, family := range verifiedExpansionTargetFamilies {
		coverage, ok := familyCoverageByID(report, family)
		if !ok {
			t.Errorf("missing family coverage for %s; found families: %#v", family, familyIDs(report))
			continue
		}
		if coverage.RecordCount == 0 {
			t.Errorf("family %s has no records", family)
		}
	}
}

func TestDefaultRoadmapRequiredFamiliesIncludeCoverageExpansionTargets(t *testing.T) {
	required := defaultRoadmapRequiredFamilies()
	for _, family := range verifiedExpansionTargetFamilies {
		if !slices.Contains(required, family) {
			t.Errorf("default required families missing %s: %#v", family, required)
		}
	}
}

func familyCoverageByID(report CoverageReport, family string) (FamilyCoverage, bool) {
	for _, coverage := range report.Families {
		if coverage.Family == family {
			return coverage, true
		}
	}
	return FamilyCoverage{}, false
}

func familyIDs(report CoverageReport) []string {
	ids := make([]string, 0, len(report.Families))
	for _, coverage := range report.Families {
		ids = append(ids, coverage.Family)
	}
	return ids
}
