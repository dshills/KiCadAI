package designworkflow

import (
	"sort"
	"strings"
)

// PromotionFixtureSummary is a deterministic readiness inventory for promotion fixtures.
type PromotionFixtureSummary struct {
	Total       int                         `json:"total"`
	ByReadiness []PromotionReadinessSummary `json:"by_readiness"`
}

// PromotionReadinessSummary groups promotion fixtures at one readiness level.
type PromotionReadinessSummary struct {
	Readiness PromotionReadiness             `json:"readiness"`
	Count     int                            `json:"count"`
	Fixtures  []PromotionFixtureSummaryEntry `json:"fixtures"`
}

// PromotionFixtureSummaryEntry records the metadata needed to pick the next promotion fixture.
type PromotionFixtureSummaryEntry struct {
	ID            string          `json:"id"`
	Tier          string          `json:"tier,omitempty"`
	Acceptance    AcceptanceLevel `json:"acceptance,omitempty"`
	RequireERC    bool            `json:"require_erc"`
	RequireDRC    bool            `json:"require_drc"`
	KnownGapCount int             `json:"known_gap_count"`
}

// SummarizePromotionFixtures returns a stable readiness grouping for promotion fixtures.
func SummarizePromotionFixtures(fixtures []PromotionFixture) PromotionFixtureSummary {
	groups := map[PromotionReadiness][]PromotionFixtureSummaryEntry{}
	for _, fixture := range fixtures {
		entry := PromotionFixtureSummaryEntry{
			ID:            fixture.ID,
			Tier:          fixture.Tier,
			Acceptance:    fixture.Acceptance,
			RequireERC:    fixture.RequireERC,
			RequireDRC:    fixture.RequireDRC,
			KnownGapCount: countNonEmptyPromotionGaps(fixture.KnownGaps),
		}
		groups[fixture.DeclaredReadiness] = append(groups[fixture.DeclaredReadiness], entry)
	}
	readinessValues := make([]PromotionReadiness, 0, len(groups))
	for readiness := range groups {
		readinessValues = append(readinessValues, readiness)
	}
	sort.Slice(readinessValues, func(i, j int) bool {
		return promotionReadinessSummaryRank(readinessValues[i]) < promotionReadinessSummaryRank(readinessValues[j])
	})
	summary := PromotionFixtureSummary{Total: len(fixtures), ByReadiness: make([]PromotionReadinessSummary, 0, len(readinessValues))}
	for _, readiness := range readinessValues {
		entries := groups[readiness]
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Tier != entries[j].Tier {
				return entries[i].Tier < entries[j].Tier
			}
			return entries[i].ID < entries[j].ID
		})
		summary.ByReadiness = append(summary.ByReadiness, PromotionReadinessSummary{
			Readiness: readiness,
			Count:     len(entries),
			Fixtures:  entries,
		})
	}
	return summary
}

func countNonEmptyPromotionGaps(gaps []string) int {
	count := 0
	for _, gap := range gaps {
		if hasText(gap) {
			count++
		}
	}
	return count
}

func hasText(value string) bool {
	return strings.TrimSpace(value) != ""
}

func promotionReadinessSummaryRank(readiness PromotionReadiness) int {
	switch readiness {
	case PromotionReadinessPass:
		return 0
	case PromotionReadinessCandidate:
		return 1
	case PromotionReadinessExpectedFail:
		return 2
	case PromotionReadinessBlocked:
		return 3
	default:
		return 4
	}
}
