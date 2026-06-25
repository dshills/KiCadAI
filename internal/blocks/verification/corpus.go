package verification

import (
	"slices"
	"strings"
)

type KiCadCorpusMode string

const (
	KiCadCorpusModeDisabled KiCadCorpusMode = ""
	KiCadCorpusModeEnabled  KiCadCorpusMode = "enabled"
)

type KiCadCorpusOptions struct {
	Enabled bool
	Tiers   []KiCadCorpusTier
}

type KiCadCorpusResultStatus string

const (
	KiCadCorpusResultPass         KiCadCorpusResultStatus = "pass"
	KiCadCorpusResultSkip         KiCadCorpusResultStatus = "skip"
	KiCadCorpusResultExpectedFail KiCadCorpusResultStatus = "expected_fail"
	KiCadCorpusResultBlocked      KiCadCorpusResultStatus = "blocked"
	KiCadCorpusResultNotInCorpus  KiCadCorpusResultStatus = "not_in_corpus"
)

type KiCadCorpusCaseResult struct {
	CaseID         string                    `json:"case_id"`
	BlockID        string                    `json:"block_id"`
	Tier           KiCadCorpusTier           `json:"tier,omitempty"`
	Readiness      KiCadCorpusReadiness      `json:"readiness,omitempty"`
	Status         KiCadCorpusResultStatus   `json:"status"`
	ExpectedStatus KiCadCorpusExpectedStatus `json:"expected_status,omitempty"`
	ExpectedIssues []string                  `json:"expected_issues,omitempty"`
	Notes          string                    `json:"notes,omitempty"`
}

type KiCadCorpusSummary struct {
	Enabled        bool                            `json:"enabled"`
	SelectedCount  int                             `json:"selected_count"`
	TotalCount     int                             `json:"total_count"`
	CaseIDs        []string                        `json:"case_ids,omitempty"`
	CountsByStatus map[KiCadCorpusResultStatus]int `json:"counts_by_status,omitempty"`
	CountsByTier   map[KiCadCorpusTier]int         `json:"counts_by_tier,omitempty"`
	CountsByBlock  map[string]int                  `json:"counts_by_block,omitempty"`
	Results        []KiCadCorpusCaseResult         `json:"results,omitempty"`
}

func SelectKiCadCorpusManifests(manifests []Manifest, opts KiCadCorpusOptions) ([]Manifest, KiCadCorpusSummary) {
	if !opts.Enabled {
		return manifests, KiCadCorpusSummary{Enabled: false, TotalCount: len(manifests)}
	}
	tierSet := corpusTierSet(opts.Tiers)
	selected := make([]Manifest, 0, len(manifests))
	results := make([]KiCadCorpusCaseResult, 0, len(manifests))
	for _, manifest := range manifests {
		corpus := manifest.Expected.KiCadCorpus
		if !corpus.Include {
			results = append(results, corpusCaseResult(manifest, KiCadCorpusResultNotInCorpus))
			continue
		}
		if len(tierSet) > 0 {
			if _, ok := tierSet[normalizeKiCadCorpusTier(corpus.Tier)]; !ok {
				results = append(results, corpusCaseResult(manifest, KiCadCorpusResultNotInCorpus))
				continue
			}
		}
		selected = append(selected, manifest)
		results = append(results, corpusCaseResult(manifest, KiCadCorpusResultSkip))
	}
	return selected, buildKiCadCorpusSummary(true, len(manifests), results)
}

func BuildKiCadCorpusSummary(results []RunResult) KiCadCorpusSummary {
	cases := make([]KiCadCorpusCaseResult, 0, len(results))
	for _, result := range results {
		if result.KiCadCorpus == nil {
			cases = append(cases, KiCadCorpusCaseResult{
				CaseID:  result.CaseID,
				BlockID: result.BlockID,
				Status:  KiCadCorpusResultNotInCorpus,
			})
			continue
		}
		cases = append(cases, *result.KiCadCorpus)
	}
	return buildKiCadCorpusSummary(len(cases) > 0, len(cases), cases)
}

func corpusCaseResult(manifest Manifest, status KiCadCorpusResultStatus) KiCadCorpusCaseResult {
	corpus := manifest.Expected.KiCadCorpus
	return KiCadCorpusCaseResult{
		CaseID:         manifest.ID,
		BlockID:        manifest.BlockID,
		Tier:           corpus.Tier,
		Readiness:      corpus.Readiness,
		Status:         status,
		ExpectedStatus: corpus.ExpectedStatus,
		ExpectedIssues: slices.Clone(corpus.ExpectedIssues),
		Notes:          strings.TrimSpace(corpus.Notes),
	}
}

func buildKiCadCorpusSummary(enabled bool, total int, results []KiCadCorpusCaseResult) KiCadCorpusSummary {
	slices.SortFunc(results, func(a, b KiCadCorpusCaseResult) int {
		return strings.Compare(a.CaseID, b.CaseID)
	})
	summary := KiCadCorpusSummary{
		Enabled:        enabled,
		TotalCount:     total,
		CountsByStatus: map[KiCadCorpusResultStatus]int{},
		CountsByTier:   map[KiCadCorpusTier]int{},
		CountsByBlock:  map[string]int{},
		Results:        results,
	}
	for _, result := range results {
		summary.CountsByStatus[result.Status]++
		if result.Status == KiCadCorpusResultNotInCorpus {
			continue
		}
		summary.SelectedCount++
		summary.CaseIDs = append(summary.CaseIDs, result.CaseID)
		if result.Tier != "" {
			summary.CountsByTier[result.Tier]++
		}
		if result.BlockID != "" {
			summary.CountsByBlock[result.BlockID]++
		}
	}
	if len(summary.CountsByStatus) == 0 {
		summary.CountsByStatus = nil
	}
	if len(summary.CountsByTier) == 0 {
		summary.CountsByTier = nil
	}
	if len(summary.CountsByBlock) == 0 {
		summary.CountsByBlock = nil
	}
	if len(summary.Results) == 0 {
		summary.Results = nil
	}
	return summary
}

func corpusTierSet(tiers []KiCadCorpusTier) map[KiCadCorpusTier]struct{} {
	if len(tiers) == 0 {
		return nil
	}
	set := map[KiCadCorpusTier]struct{}{}
	for _, tier := range tiers {
		tier = KiCadCorpusTier(strings.TrimSpace(string(tier)))
		if tier != "" {
			set[tier] = struct{}{}
		}
	}
	return set
}

func normalizeKiCadCorpusTier(tier KiCadCorpusTier) KiCadCorpusTier {
	return KiCadCorpusTier(strings.TrimSpace(string(tier)))
}
