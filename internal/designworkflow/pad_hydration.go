package designworkflow

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
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

type padHydrationResult struct {
	Bounds placement.Bounds
	Pads   []placement.PadSummary
	Entry  PadHydrationEntry
	Issues []reports.Issue
}

type padHydrationResolver struct {
	index libraryresolver.LibraryIndex
	cache map[string]padHydrationResult
	mu    sync.Mutex
}

func (resolver *padHydrationResolver) Hydrate(ref string, footprintID string) padHydrationResult {
	ref = strings.TrimSpace(ref)
	footprintID = strings.TrimSpace(footprintID)
	resolver.mu.Lock()
	resolver.ensureCache()
	base, ok := resolver.cache[footprintID]
	resolver.mu.Unlock()
	if ok {
		return padHydrationResultForRef(base, ref)
	}

	base = hydratePadsFromResolverRecord(resolver.index, "", footprintID)
	resolver.mu.Lock()
	resolver.ensureCache()
	if cached, ok := resolver.cache[footprintID]; ok {
		base = cached
	} else {
		resolver.cache[footprintID] = base
	}
	resolver.mu.Unlock()
	return padHydrationResultForRef(base, ref)
}

func (resolver *padHydrationResolver) ensureCache() {
	if resolver.cache == nil {
		resolver.cache = map[string]padHydrationResult{}
	}
}

func hydratePadsFromResolverRecord(index libraryresolver.LibraryIndex, ref string, footprintID string) padHydrationResult {
	result := padHydrationResult{Entry: PadHydrationEntry{Ref: ref, FootprintID: footprintID, Source: PadHydrationSourceMissing}}
	if footprintID == "" {
		result.Entry.MissingReason = "missing footprint id"
		result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, "footprint_id", "component has no footprint id for pad hydration"))
		return result
	}
	record, ok := libraryresolver.ResolveFootprint(index, footprintID)
	if !ok {
		result.Entry.MissingReason = "footprint not resolved"
		result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, "footprint_id", "footprint library record not found: "+footprintID))
		return result
	}
	bounds, pads, issues := placement.BoundsFromFootprint(record)
	result.Bounds = bounds
	result.Issues = append(result.Issues, contextualizePadHydrationIssues(ref, issues)...)
	for padIndex, pad := range pads {
		pad.Name = strings.TrimSpace(pad.Name)
		if pad.Name == "" {
			result.Issues = append(result.Issues, padHydrationWarning(ref, footprintID, fmt.Sprintf("pads[%d].name", padIndex), "unnamed footprint pad skipped during routing summary hydration"))
			continue
		}
		if pad.WidthMM <= 0 || pad.HeightMM <= 0 {
			result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, fmt.Sprintf("pads[%d].size", padIndex), "footprint pad size must be positive"))
			continue
		}
		result.Pads = append(result.Pads, pad)
	}
	if len(result.Pads) == 0 {
		result.Entry.MissingReason = "no routable footprint pads"
		result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, "pads", "footprint has no routable pads"))
		return result
	}
	result.Entry.Source = PadHydrationSourceResolver
	result.Entry.PadCount = len(result.Pads)
	return result
}

type padNetAssignment struct {
	NetName string
	Pin     string
}

type padNetAssignmentIndex map[string][]padNetAssignment

func buildPadNetAssignmentIndex(nets []placement.Net) padNetAssignmentIndex {
	index := padNetAssignmentIndex{}
	for _, net := range nets {
		netName := strings.TrimSpace(net.Name)
		if netName == "" {
			continue
		}
		for _, endpoint := range net.Endpoints {
			ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
			if ref == "" {
				continue
			}
			index[ref] = append(index[ref], padNetAssignment{NetName: netName, Pin: strings.TrimSpace(endpoint.Pin)})
		}
	}
	return index
}

func assignPadNetsFromIndex(ref string, pads []placement.PadSummary, assignments padNetAssignmentIndex) ([]placement.PadSummary, []reports.Issue) {
	ref = strings.TrimSpace(ref)
	out := append([]placement.PadSummary(nil), pads...)
	padByName := map[string][]int{}
	for index, pad := range out {
		name := strings.TrimSpace(pad.Name)
		out[index].Name = name
		if name != "" {
			padByName[name] = append(padByName[name], index)
		}
	}
	var issues []reports.Issue
	for _, assignment := range assignments[strings.ToUpper(ref)] {
		pin := assignment.Pin
		if pin == "" {
			issues = append(issues, padHydrationIssue(ref, "", "nets."+assignment.NetName, "net endpoint pin is required for pad assignment"))
			continue
		}
		padIndexes := padByName[pin]
		if len(padIndexes) == 0 {
			issues = append(issues, padHydrationIssue(ref, "", "nets."+assignment.NetName+"."+pin, "net endpoint pin has no matching footprint pad"))
			continue
		}
		for _, padIndex := range padIndexes {
			if out[padIndex].Net != "" && out[padIndex].Net != assignment.NetName {
				issues = append(issues, padHydrationIssue(ref, "", "pads."+pin+".net", fmt.Sprintf("footprint pad maps to multiple generated nets: %s and %s", out[padIndex].Net, assignment.NetName)))
				continue
			}
			out[padIndex].Net = assignment.NetName
		}
	}
	return out, issues
}

func padHydrationResultForRef(base padHydrationResult, ref string) padHydrationResult {
	result := padHydrationResult{
		Bounds: base.Bounds,
		Pads:   append([]placement.PadSummary(nil), base.Pads...),
		Entry:  base.Entry,
		Issues: append([]reports.Issue(nil), base.Issues...),
	}
	result.Entry.Ref = ref
	for index := range result.Issues {
		result.Issues[index] = contextualizePadHydrationIssue(ref, result.Issues[index])
	}
	return result
}

func padHydrationIssue(ref string, footprintID string, path string, message string) reports.Issue {
	return newPadHydrationIssue(ref, footprintID, path, message, reports.SeverityBlocked)
}

func padHydrationWarning(ref string, footprintID string, path string, message string) reports.Issue {
	return newPadHydrationIssue(ref, footprintID, path, message, reports.SeverityWarning)
}

func newPadHydrationIssue(ref string, footprintID string, path string, message string, severity reports.Severity) reports.Issue {
	issuePath := "pad_hydration"
	if ref != "" {
		issuePath += "." + ref
	}
	if path != "" {
		issuePath += "." + path
	}
	issue := reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: severity,
		Path:     issuePath,
		Message:  message,
	}
	if ref != "" {
		issue.Refs = []string{ref}
	}
	if footprintID != "" {
		issue.Suggestion = "resolve footprint pad metadata for " + footprintID
	}
	return issue
}

func contextualizePadHydrationIssues(ref string, issues []reports.Issue) []reports.Issue {
	if ref == "" || len(issues) == 0 {
		return issues
	}
	out := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, contextualizePadHydrationIssue(ref, issue))
	}
	return out
}

func contextualizePadHydrationIssue(ref string, issue reports.Issue) reports.Issue {
	if ref == "" {
		return issue
	}
	if issue.Path != "" {
		if !strings.HasPrefix(issue.Path, "pad_hydration.") {
			issue.Path = "pad_hydration." + ref + "." + issue.Path
		} else if !strings.HasPrefix(issue.Path, "pad_hydration."+ref+".") {
			issue.Path = "pad_hydration." + ref + strings.TrimPrefix(issue.Path, "pad_hydration")
		}
	} else {
		issue.Path = "pad_hydration." + ref
	}
	if len(issue.Refs) == 0 {
		issue.Refs = []string{ref}
	}
	return issue
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
