package designworkflow

import (
	"sort"
	"strings"

	"kicadai/internal/reports"
)

type PadHydrationSource string

const (
	PadHydrationSourceResolver         PadHydrationSource = "resolver"
	PadHydrationSourceVerifiedTemplate PadHydrationSource = "verified_template"
	PadHydrationSourceMissing          PadHydrationSource = "missing"
)

type PadHydrationEntry struct {
	Ref           string             `json:"ref"`
	FootprintID   string             `json:"footprint_id,omitempty"`
	Source        PadHydrationSource `json:"source"`
	PadCount      int                `json:"pad_count,omitempty"`
	MissingReason string             `json:"missing_reason,omitempty"`
}

type PadHydrationSummary struct {
	ComponentCount     int                        `json:"component_count"`
	HydratedComponents int                        `json:"hydrated_components"`
	MissingComponents  int                        `json:"missing_components"`
	PadCount           int                        `json:"pad_count"`
	SourceCounts       map[PadHydrationSource]int `json:"source_counts,omitempty"`
	MissingRefs        []string                   `json:"missing_refs,omitempty"`
	BlockingIssues     int                        `json:"blocking_issues,omitempty"`
}

func summarizePadHydration(entries []PadHydrationEntry, issues []reports.Issue) PadHydrationSummary {
	summary := PadHydrationSummary{
		ComponentCount: len(entries),
		SourceCounts:   map[PadHydrationSource]int{},
	}
	missing := map[string]struct{}{}
	for _, entry := range entries {
		source := normalizePadHydrationSource(entry.Source)
		summary.SourceCounts[source]++
		if entry.PadCount > 0 && source != PadHydrationSourceMissing {
			summary.HydratedComponents++
			summary.PadCount += entry.PadCount
			continue
		}
		summary.MissingComponents++
		ref := strings.TrimSpace(entry.Ref)
		if ref != "" {
			missing[ref] = struct{}{}
		}
	}
	for _, issue := range issues {
		if issue.Blocking() {
			summary.BlockingIssues++
		}
	}
	summary.MissingRefs = sortedKeys(missing)
	if len(summary.SourceCounts) == 0 {
		summary.SourceCounts = nil
	}
	return summary
}

func normalizePadHydrationSource(source PadHydrationSource) PadHydrationSource {
	if source == "" {
		return PadHydrationSourceMissing
	}
	return source
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
