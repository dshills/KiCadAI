package components

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

var plannedVerifiedExpansionTargetMatrix = []struct {
	Family           string
	Current          bool
	BaselinePackages []string
	BaselineValues   []string
	PlannedPackages  []string
	PlannedValues    []string
	PlannedUses      []string
}{
	{
		Family:           "resistor",
		Current:          true,
		BaselinePackages: []string{"0805"},
		BaselineValues:   []string{"10k"},
		PlannedPackages:  []string{"0603", "0805"},
		PlannedValues:    []string{"0R", "47R", "100R", "220R", "330R", "470R", "1k", "2.2k", "4.7k", "10k", "22k", "47k", "100k"},
		PlannedUses:      []string{"bias", "pullup", "current_limit", "feedback"},
	},
	{
		Family:           "capacitor",
		Current:          true,
		BaselinePackages: []string{"0805"},
		BaselineValues:   []string{"100n"},
		PlannedPackages:  []string{"0603", "0805", "radial"},
		PlannedValues:    []string{"10p", "18p", "22p", "100p", "1n", "10n", "100n", "1u", "4.7u", "10u", "47u", "100u", "220u"},
		PlannedUses:      []string{"decoupling", "filter", "bulk"},
	},
	{Family: "connector", Current: true, BaselinePackages: []string{"pin_header_1x04"}, BaselineValues: []string{"4"}, PlannedPackages: []string{"1x02", "1x03", "1x04", "1x05", "1x06"}, PlannedUses: []string{"power", "signal", "programming", "audio_io"}},
	{Family: "diode", Current: true, BaselinePackages: []string{"sod_123", "sma"}, PlannedPackages: []string{"sod_123", "sod_323", "sma"}, PlannedUses: []string{"signal", "schottky", "reverse_polarity"}},
	{Family: "led", Current: true, BaselinePackages: []string{"0805"}, BaselineValues: []string{"green"}, PlannedPackages: []string{"0603", "0805"}, PlannedValues: []string{"green", "red", "blue", "amber"}, PlannedUses: []string{"indicator"}},
	{Family: "protection", Current: true, BaselinePackages: []string{"sod_323"}, PlannedPackages: []string{"sod_323", "sot23", "dfn"}, PlannedUses: []string{"esd", "tvs", "power_entry"}},
	{Family: "opamp", Current: true, BaselinePackages: []string{"sot23_5"}, PlannedPackages: []string{"sot23_5", "soic8"}, PlannedUses: []string{"audio_buffer", "gain_stage"}},
	{Family: "crystal", Current: true, BaselinePackages: []string{"5032_2pin"}, BaselineValues: []string{"16"}, PlannedPackages: []string{"5032_2pin"}, PlannedValues: []string{"16"}, PlannedUses: []string{"clock_source"}},
	{Family: "bjt", Current: true, BaselinePackages: []string{"sot23"}, PlannedPackages: []string{"sot23", "to92", "to220"}, PlannedUses: []string{"class_ab_driver", "small_signal_switch", "power_output_blocked"}},
	{Family: "usb_c", Current: true, BaselinePackages: []string{"gct_usb4125_6p"}, PlannedPackages: []string{"power_only"}, PlannedUses: []string{"power_entry"}},
	{Family: "sensor", Current: true, BaselinePackages: []string{"lga8"}, PlannedPackages: []string{"lga8", "dfn8_ep"}, PlannedValues: []string{"BME280", "BMP280", "SHT31-DIS"}, PlannedUses: []string{"environmental_i2c", "pressure", "humidity", "temperature"}},
}

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
	report, result := ComponentCoverage(catalog, CoverageOptions{RequiredFamilies: currentVerifiedExpansionFamilies()})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("coverage should not block: %+v", result.Issues)
	}
	for _, family := range currentVerifiedExpansionFamilies() {
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

func TestDefaultRoadmapRequiredFamiliesIncludeCurrentCoverageFamilies(t *testing.T) {
	required := defaultRoadmapRequiredFamilies()
	for _, family := range currentVerifiedExpansionFamilies() {
		if !slices.Contains(required, family) {
			t.Errorf("default required families missing %s: %#v", family, required)
		}
	}
}

func TestCheckedInCatalogMeetsExpandedCoverageThresholds(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	report, result := ComponentCoverage(catalog, CoverageOptions{RequiredFamilies: currentVerifiedExpansionFamilies()})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("coverage should not block: %+v", result.Issues)
	}
	if report.AlternativeCoverage.ConcreteRecords < 27 {
		t.Fatalf("concrete records = %d, want at least 27", report.AlternativeCoverage.ConcreteRecords)
	}
	if report.AlternativeCoverage.EquivalenceGroups < 19 {
		t.Fatalf("equivalence groups = %d, want at least 19", report.AlternativeCoverage.EquivalenceGroups)
	}
	thresholds := map[string]struct {
		concrete int
		generic  int
		blocked  int
	}{
		"bjt":        {concrete: 2, generic: 1, blocked: 1},
		"capacitor":  {concrete: 3, generic: 3},
		"connector":  {concrete: 5, generic: 5},
		"diode":      {concrete: 2, generic: 3},
		"led":        {concrete: 3, generic: 2},
		"protection": {concrete: 1, generic: 1},
		"resistor":   {concrete: 4, generic: 2},
		"sensor":     {concrete: 3, generic: 1},
	}
	for family, threshold := range thresholds {
		coverage, ok := familyCoverageByID(report, family)
		if !ok {
			t.Fatalf("missing coverage for %s", family)
		}
		if coverage.ConcreteRecords < threshold.concrete {
			t.Errorf("%s concrete records = %d, want at least %d", family, coverage.ConcreteRecords, threshold.concrete)
		}
		if coverage.GenericFallbackRecords < threshold.generic {
			t.Errorf("%s generic records = %d, want at least %d", family, coverage.GenericFallbackRecords, threshold.generic)
		}
		if coverage.BlockedRecords < threshold.blocked {
			t.Errorf("%s blocked records = %d, want at least %d", family, coverage.BlockedRecords, threshold.blocked)
		}
	}
}

func TestVerifiedExpansionTargetMatrixCoversPlannedFamilies(t *testing.T) {
	seen := map[string]bool{}
	expected := map[string]bool{
		"bjt":        true,
		"capacitor":  true,
		"connector":  true,
		"crystal":    true,
		"diode":      true,
		"led":        true,
		"opamp":      true,
		"protection": true,
		"resistor":   true,
		"sensor":     true,
		"usb_c":      true,
	}
	for _, target := range plannedVerifiedExpansionTargetMatrix {
		if target.Family == "" {
			t.Error("target family is required")
		}
		if len(target.PlannedPackages) == 0 {
			t.Errorf("target family %s has no planned packages", target.Family)
		}
		if len(target.PlannedUses) == 0 {
			t.Errorf("target family %s has no planned uses", target.Family)
		}
		if seen[target.Family] {
			t.Errorf("duplicate family in expansion target matrix: %s", target.Family)
		}
		if !expected[target.Family] {
			t.Errorf("expansion target matrix has unplanned family %s", target.Family)
		}
		seen[target.Family] = true
	}
	for family := range expected {
		if !seen[family] {
			t.Errorf("expansion target matrix missing expected family %s", family)
		}
	}
}

func TestVerifiedExpansionTargetMatrixBaselineCoverage(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	recordsByFamily := catalogRecordsByFamily(catalog)
	valuesByFamily := catalogValuesByFamily(recordsByFamily)
	for _, target := range plannedVerifiedExpansionTargetMatrix {
		if target.Current && len(target.BaselinePackages) == 0 {
			t.Errorf("current family %s must declare at least one baseline package", target.Family)
		}
		for _, pkg := range target.BaselinePackages {
			if !catalogFamilyHasPackage(recordsByFamily[target.Family], pkg) {
				t.Errorf("family %s missing baseline package %s", target.Family, pkg)
			}
		}
		for _, value := range target.BaselineValues {
			if !valuesByFamily[target.Family][normalizeMetadata(value)] {
				t.Errorf("family %s missing baseline value %s", target.Family, value)
			}
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

func currentVerifiedExpansionFamilies() []string {
	var families []string
	for _, target := range plannedVerifiedExpansionTargetMatrix {
		if target.Current {
			families = append(families, target.Family)
		}
	}
	return families
}

func familyIDs(report CoverageReport) []string {
	ids := make([]string, 0, len(report.Families))
	for _, coverage := range report.Families {
		ids = append(ids, coverage.Family)
	}
	return ids
}

func catalogRecordsByFamily(catalog *Catalog) map[string][]ComponentRecord {
	recordsByFamily := map[string][]ComponentRecord{}
	for _, record := range catalog.Records {
		recordsByFamily[record.Family] = append(recordsByFamily[record.Family], record)
	}
	return recordsByFamily
}

func catalogFamilyHasPackage(records []ComponentRecord, packageID string) bool {
	for _, record := range records {
		for _, pkg := range record.Packages {
			if pkg.ID == packageID || pkg.PackageType == packageID {
				return true
			}
		}
	}
	return false
}

func catalogValuesByFamily(recordsByFamily map[string][]ComponentRecord) map[string]map[string]bool {
	valuesByFamily := map[string]map[string]bool{}
	for family, records := range recordsByFamily {
		values := map[string]bool{}
		for _, record := range records {
			for _, candidate := range record.Values {
				values[normalizeMetadata(candidate.Min)] = true
				values[normalizeMetadata(candidate.Typ)] = true
				values[normalizeMetadata(candidate.Max)] = true
			}
			for _, tag := range record.Tags {
				values[normalizeMetadata(tag)] = true
			}
		}
		valuesByFamily[family] = values
	}
	return valuesByFamily
}
