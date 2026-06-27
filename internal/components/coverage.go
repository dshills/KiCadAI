package components

import (
	"sort"
	"strings"

	"kicadai/internal/reports"
)

const (
	CodeComponentCoverageMissing     reports.Code = "COMPONENT_COVERAGE_MISSING"
	CodeComponentCoveragePlaceholder reports.Code = "COMPONENT_COVERAGE_PLACEHOLDER_ONLY"
)

type CoverageOptions struct {
	RequiredFamilies []string          `json:"required_families,omitempty"`
	Sources          *SourceCollection `json:"-"`
}

type CoverageReport struct {
	RecordCount         int                 `json:"record_count"`
	FamilyCount         int                 `json:"family_count"`
	ConfidenceCounts    map[string]int      `json:"confidence_counts"`
	AlternativeCoverage AlternativeCoverage `json:"alternative_coverage"`
	SourceCoverage      *SourceCoverage     `json:"source_coverage,omitempty"`
	Families            []FamilyCoverage    `json:"families"`
	Issues              []reports.Issue     `json:"issues,omitempty"`
}

type SourceCoverage struct {
	ConcreteRecords               int `json:"concrete_records"`
	LifecycleEvidenceRecords      int `json:"lifecycle_evidence_records"`
	AvailabilityEvidenceRecords   int `json:"availability_evidence_records"`
	UnknownAvailabilityRecords    int `json:"unknown_availability_records"`
	NotCheckedAvailabilityRecords int `json:"not_checked_availability_records"`
}

type AlternativeCoverage struct {
	ConcreteRecords              int      `json:"concrete_records"`
	GenericFallbackRecords       int      `json:"generic_fallback_records"`
	EquivalenceGroups            int      `json:"equivalence_groups"`
	GroupsMissingPreferred       []string `json:"groups_missing_preferred,omitempty"`
	GroupsWithDuplicatePreferred []string `json:"groups_with_duplicate_preferred,omitempty"`
	ConcreteRecordsMissingMPN    []string `json:"concrete_records_missing_mpn,omitempty"`
}

type FamilyCoverage struct {
	Family                  string   `json:"family"`
	Defined                 bool     `json:"defined"`
	RecordCount             int      `json:"record_count"`
	ConcreteRecords         int      `json:"concrete_records"`
	GenericFallbackRecords  int      `json:"generic_fallback_records"`
	EquivalenceGroups       int      `json:"equivalence_groups"`
	VerifiedRecords         int      `json:"verified_records"`
	LibraryDerivedRecords   int      `json:"library_derived_records"`
	RuleInferredRecords     int      `json:"rule_inferred_records"`
	PlaceholderRecords      int      `json:"placeholder_records"`
	BlockedRecords          int      `json:"blocked_records"`
	MissingResolverEvidence []string `json:"missing_resolver_evidence,omitempty"`
	MissingPinmapEvidence   []string `json:"missing_pinmap_evidence,omitempty"`
	RecordIDs               []string `json:"record_ids,omitempty"`
}

func ComponentCoverage(catalog *Catalog, opts CoverageOptions) (CoverageReport, reports.Result) {
	if catalog == nil {
		issue := NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "catalog", "component catalog is nil")
		report := CoverageReport{ConfidenceCounts: emptyConfidenceCounts(), Issues: []reports.Issue{issue}}
		return report, reports.ErrorResult("component coverage", issue)
	}
	required := opts.RequiredFamilies
	if len(required) == 0 {
		required = defaultRoadmapRequiredFamilies()
	}
	report := CoverageReport{
		RecordCount:      len(catalog.Records),
		FamilyCount:      len(catalog.Families),
		ConfidenceCounts: emptyConfidenceCounts(),
	}
	if opts.Sources != nil {
		report.SourceCoverage = &SourceCoverage{}
	}
	defined := map[string]bool{}
	byFamily := map[string]*FamilyCoverage{}
	equivalenceGroups := map[equivalenceCoverageKey]int{}
	familyEquivalenceGroups := map[string]map[string]struct{}{}
	for _, family := range catalog.Families {
		id := normalizeCoverageFamily(family.ID)
		if id == "" {
			continue
		}
		defined[id] = true
		coverage := ensureFamilyCoverage(byFamily, id)
		coverage.Defined = true
	}
	for i := range catalog.Records {
		record := &catalog.Records[i]
		family := normalizeCoverageFamily(record.Family)
		if family == "" {
			family = "unknown"
		}
		coverage := ensureFamilyCoverage(byFamily, family)
		if !coverage.Defined {
			coverage.Defined = defined[family]
		}
		coverage.RecordCount++
		coverage.RecordIDs = append(coverage.RecordIDs, record.ID)
		if record.Generic {
			report.AlternativeCoverage.GenericFallbackRecords++
			coverage.GenericFallbackRecords++
		} else {
			report.AlternativeCoverage.ConcreteRecords++
			coverage.ConcreteRecords++
			if strings.TrimSpace(record.MPN) == "" {
				report.AlternativeCoverage.ConcreteRecordsMissingMPN = append(report.AlternativeCoverage.ConcreteRecordsMissingMPN, record.ID)
			}
		}
		if record.Equivalence != nil && strings.TrimSpace(record.Equivalence.Group) != "" {
			group := normalizeMetadata(record.Equivalence.Group)
			if _, ok := familyEquivalenceGroups[family]; !ok {
				familyEquivalenceGroups[family] = map[string]struct{}{}
			}
			familyEquivalenceGroups[family][group] = struct{}{}
			groupKey := equivalenceCoverageKey{family: family, group: group}
			if record.Equivalence.Role == EquivalencePreferred {
				equivalenceGroups[groupKey]++
			} else if _, ok := equivalenceGroups[groupKey]; !ok {
				equivalenceGroups[groupKey] = 0
			}
		}
		confidence := record.Verification.Confidence
		report.ConfidenceCounts[string(confidence)]++
		switch confidence {
		case ConfidenceVerified:
			coverage.VerifiedRecords++
		case ConfidenceLibraryDerived:
			coverage.LibraryDerivedRecords++
		case ConfidenceRuleInferred:
			coverage.RuleInferredRecords++
		case ConfidencePlaceholder:
			coverage.PlaceholderRecords++
		case ConfidenceBlocked:
			coverage.BlockedRecords++
		}
		if !record.Verification.ResolverChecked {
			coverage.MissingResolverEvidence = append(coverage.MissingResolverEvidence, record.ID)
		}
		if !record.Verification.PinMapChecked && record.Verification.Confidence == ConfidenceVerified && !passiveRuleInferred(*record) {
			coverage.MissingPinmapEvidence = append(coverage.MissingPinmapEvidence, record.ID)
		}
		updateSourceCoverage(report.SourceCoverage, record, opts.Sources)
	}
	report.AlternativeCoverage.EquivalenceGroups = len(equivalenceGroups)
	for groupKey, preferredCount := range equivalenceGroups {
		formattedGroup := groupKey.String()
		switch {
		case preferredCount == 0:
			report.AlternativeCoverage.GroupsMissingPreferred = append(report.AlternativeCoverage.GroupsMissingPreferred, formattedGroup)
		case preferredCount > 1:
			report.AlternativeCoverage.GroupsWithDuplicatePreferred = append(report.AlternativeCoverage.GroupsWithDuplicatePreferred, formattedGroup)
		}
	}
	for family, groups := range familyEquivalenceGroups {
		coverage := ensureFamilyCoverage(byFamily, family)
		coverage.EquivalenceGroups = len(groups)
	}
	sort.Strings(report.AlternativeCoverage.GroupsMissingPreferred)
	sort.Strings(report.AlternativeCoverage.GroupsWithDuplicatePreferred)
	sort.Strings(report.AlternativeCoverage.ConcreteRecordsMissingMPN)
	for _, requiredFamily := range required {
		family := normalizeCoverageFamily(requiredFamily)
		if family == "" {
			continue
		}
		coverage := ensureFamilyCoverage(byFamily, family)
		if coverage.RecordCount == 0 {
			report.Issues = append(report.Issues, NewIssue(CodeComponentCoverageMissing, reports.SeverityWarning, "coverage."+family, "required component family has no records: "+family))
		} else if coverage.VerifiedRecords == 0 && coverage.RuleInferredRecords == 0 && coverage.LibraryDerivedRecords == 0 {
			report.Issues = append(report.Issues, NewIssue(CodeComponentCoveragePlaceholder, reports.SeverityWarning, "coverage."+family, "required component family has only placeholder or blocked records: "+family))
		}
	}
	families := make([]FamilyCoverage, 0, len(byFamily))
	for _, coverage := range byFamily {
		sort.Strings(coverage.RecordIDs)
		sort.Strings(coverage.MissingResolverEvidence)
		sort.Strings(coverage.MissingPinmapEvidence)
		families = append(families, *coverage)
	}
	sort.SliceStable(families, func(i, j int) bool {
		return families[i].Family < families[j].Family
	})
	sortIssues(report.Issues)
	report.FamilyCount = len(families)
	report.Families = families
	return report, reports.ResultWithIssues("component coverage", report, report.Issues, nil)
}

type equivalenceCoverageKey struct {
	family string
	group  string
}

func (key equivalenceCoverageKey) String() string {
	if key.family == "" {
		return key.group
	}
	return key.family + ":" + key.group
}

func ensureFamilyCoverage(byFamily map[string]*FamilyCoverage, family string) *FamilyCoverage {
	if coverage, ok := byFamily[family]; ok {
		return coverage
	}
	coverage := &FamilyCoverage{Family: family}
	byFamily[family] = coverage
	return coverage
}

func updateSourceCoverage(coverage *SourceCoverage, record *ComponentRecord, sources *SourceCollection) {
	if coverage == nil {
		return
	}
	if record == nil {
		return
	}
	manufacturer := strings.TrimSpace(record.Manufacturer)
	mpn := strings.TrimSpace(record.MPN)
	if record.Generic || manufacturer == "" || mpn == "" {
		return
	}
	coverage.ConcreteRecords++
	if sources == nil {
		return
	}
	source, ok := sources.Find(manufacturer, mpn)
	if !ok {
		return
	}
	if source.Lifecycle != nil {
		coverage.LifecycleEvidenceRecords++
	}
	if source.Availability != nil {
		coverage.AvailabilityEvidenceRecords++
		switch source.Availability.Status {
		case AvailabilityUnknown:
			coverage.UnknownAvailabilityRecords++
		case AvailabilityNotChecked:
			coverage.NotCheckedAvailabilityRecords++
		}
	}
}

func normalizeCoverageFamily(family string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(family)), "-", "_")
}

func emptyConfidenceCounts() map[string]int {
	return map[string]int{
		string(ConfidenceVerified):       0,
		string(ConfidenceLibraryDerived): 0,
		string(ConfidenceRuleInferred):   0,
		string(ConfidencePlaceholder):    0,
		string(ConfidenceBlocked):        0,
	}
}

func defaultRoadmapRequiredFamilies() []string {
	return []string{
		"capacitor",
		"connector",
		"crystal",
		"diode",
		"led",
		"mcu",
		"opamp",
		"protection",
		"regulator",
		"resistor",
		"sensor",
		"usb_c",
	}
}
