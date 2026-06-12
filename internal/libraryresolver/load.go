package libraryresolver

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kicadai/internal/reports"
)

func Load(ctx context.Context, roots LibraryRoots, opts LoadOptions) (LibraryIndex, []reports.Issue) {
	if issue, ok := contextIssue(ctx); ok {
		return LibraryIndex{Roots: roots}, []reports.Issue{issue}
	}
	var issues []reports.Issue
	cachePath := strings.TrimSpace(opts.CachePath)
	cachePathIssues := ValidateCachePath(cachePath)
	issues = append(issues, cachePathIssues...)
	if cachePath != "" {
		cachePath = filepath.Clean(cachePath)
	}
	inventory := DiscoverContext(ctx, roots)
	issues = append(issues, inventory.Diagnostics...)
	if issue, ok := contextIssue(ctx); ok {
		issues = append(issues, issue)
		return LibraryIndex{Roots: roots, Inventory: inventory, Diagnostics: issues}, issues
	}
	cacheEnabled := cachePath != "" && len(cachePathIssues) == 0
	var cacheFiles []libraryCacheFileMeta
	if cacheEnabled {
		var metadataIssues []reports.Issue
		cacheFiles, metadataIssues = cacheMetadata(inventory)
		if len(metadataIssues) != 0 {
			issues = append(issues, metadataIssues...)
			cacheEnabled = false
		}
	}
	if cacheEnabled && !opts.Refresh {
		if cached, cacheIssues, ok := loadCache(cachePath, roots, inventory, cacheFiles); ok {
			cached.Diagnostics = mergeIssues(issues, cached.Diagnostics)
			return cached, cached.Diagnostics
		} else {
			issues = append(issues, cacheIssues...)
		}
	}
	var symbols map[string]SymbolRecord
	var symbolIssues []reports.Issue
	var footprints map[string]FootprintRecord
	var footprintIssues []reports.Issue
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		symbols, symbolIssues = IndexSymbolsContext(ctx, inventory)
	}()
	go func() {
		defer waitGroup.Done()
		footprints, footprintIssues = IndexFootprintsContext(ctx, inventory)
	}()
	waitGroup.Wait()
	issues = append(issues, symbolIssues...)
	issues = append(issues, footprintIssues...)
	if ctx != nil && ctx.Err() != nil {
		return LibraryIndex{Roots: roots, Inventory: inventory, Symbols: symbols, Footprints: footprints, Diagnostics: issues}, issues
	}
	index := LibraryIndex{
		GeneratedAt: time.Now().UTC(),
		Roots:       roots,
		Inventory:   inventory,
		Symbols:     symbols,
		Footprints:  footprints,
		Diagnostics: issues,
	}
	if cacheEnabled {
		cacheIssues := writeCache(cachePath, index, cacheFiles)
		if len(cacheIssues) != 0 {
			issues = append(issues, cacheIssues...)
			index.Diagnostics = issues
		}
	}
	return index, issues
}

func Summary(index LibraryIndex) LoadSummary {
	return LoadSummary{
		SymbolFileCount:    len(index.Inventory.SymbolFiles),
		FootprintFileCount: len(index.Inventory.FootprintFiles),
		SymbolCount:        len(index.Symbols),
		FootprintCount:     len(index.Footprints),
		DiagnosticCount:    len(index.Diagnostics),
	}
}

func FindSymbols(index LibraryIndex, query Query) []SymbolRecord {
	text := strings.ToLower(strings.TrimSpace(query.Text))
	records := make([]SymbolRecord, 0, resultCapacity(query.Limit, len(index.Symbols)))
	for _, record := range index.Symbols {
		if symbolMatches(record, text) {
			records = appendLimitedSymbol(records, record, query.Limit)
		}
	}
	slices.SortFunc(records, func(a, b SymbolRecord) int {
		return cmp.Compare(a.LibraryID, b.LibraryID)
	})
	return limitSymbols(records, query.Limit)
}

func FindFootprints(index LibraryIndex, query Query) []FootprintRecord {
	text := strings.ToLower(strings.TrimSpace(query.Text))
	records := make([]FootprintRecord, 0, resultCapacity(query.Limit, len(index.Footprints)))
	for _, record := range index.Footprints {
		if footprintMatches(record, text) {
			records = appendLimitedFootprint(records, record, query.Limit)
		}
	}
	slices.SortFunc(records, func(a, b FootprintRecord) int {
		return cmp.Compare(a.FootprintID, b.FootprintID)
	})
	return limitFootprints(records, query.Limit)
}

func symbolMatches(record SymbolRecord, text string) bool {
	if text == "" {
		return true
	}
	return strings.EqualFold(record.LibraryID, text) ||
		strings.EqualFold(record.LibraryNickname, text) ||
		strings.Contains(record.SearchText, text)
}

func footprintMatches(record FootprintRecord, text string) bool {
	if text == "" {
		return true
	}
	return strings.EqualFold(record.FootprintID, text) ||
		strings.EqualFold(record.LibraryNickname, text) ||
		strings.Contains(record.SearchText, text)
}

func limitSymbols(records []SymbolRecord, limit int) []SymbolRecord {
	if limit <= 0 || limit >= len(records) {
		return records
	}
	return records[:limit]
}

func appendLimitedSymbol(records []SymbolRecord, record SymbolRecord, limit int) []SymbolRecord {
	if limit <= 0 || len(records) < limit {
		return append(records, record)
	}
	worst := 0
	for index := 1; index < len(records); index++ {
		if records[index].LibraryID > records[worst].LibraryID {
			worst = index
		}
	}
	if record.LibraryID < records[worst].LibraryID {
		records[worst] = record
	}
	return records
}

func limitFootprints(records []FootprintRecord, limit int) []FootprintRecord {
	if limit <= 0 || limit >= len(records) {
		return records
	}
	return records[:limit]
}

func appendLimitedFootprint(records []FootprintRecord, record FootprintRecord, limit int) []FootprintRecord {
	if limit <= 0 || len(records) < limit {
		return append(records, record)
	}
	worst := 0
	for index := 1; index < len(records); index++ {
		if records[index].FootprintID > records[worst].FootprintID {
			worst = index
		}
	}
	if record.FootprintID < records[worst].FootprintID {
		records[worst] = record
	}
	return records
}

func resultCapacity(limit int, total int) int {
	if limit > 0 {
		if limit < total {
			return limit
		}
		return total
	}
	if total <= 64 {
		return total
	}
	capacity := total / 10
	if capacity < 64 {
		return 64
	}
	return capacity
}

func contextIssue(ctx context.Context) (reports.Issue, bool) {
	if ctx == nil || ctx.Err() == nil {
		return reports.Issue{}, false
	}
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "library.load",
		Message:  ctx.Err().Error(),
	}, true
}

func mergeIssues(first []reports.Issue, second []reports.Issue) []reports.Issue {
	merged := make([]reports.Issue, 0, len(first)+len(second))
	seen := map[string]struct{}{}
	for _, issue := range first {
		key := issueKey(issue)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, issue)
	}
	for _, issue := range second {
		key := issueKey(issue)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, issue)
	}
	return merged
}

func issueKey(issue reports.Issue) string {
	return string(issue.Code) + "\x00" + string(issue.Severity) + "\x00" + issue.Path + "\x00" + issue.Message + "\x00" + issue.Suggestion
}

func buildSymbolSearchText(record SymbolRecord) string {
	var builder strings.Builder
	writeSearchPart(&builder, record.LibraryID)
	writeSearchPart(&builder, record.LibraryNickname)
	writeSearchPart(&builder, record.Name)
	writeSearchPart(&builder, record.Description)
	writeSearchParts(&builder, record.Keywords)
	writeSearchParts(&builder, record.FootprintFilter)
	writeSearchMap(&builder, record.Properties)
	return strings.ToLower(builder.String())
}

func buildFootprintSearchText(record FootprintRecord) string {
	var builder strings.Builder
	writeSearchPart(&builder, record.FootprintID)
	writeSearchPart(&builder, record.LibraryNickname)
	writeSearchPart(&builder, record.Name)
	writeSearchPart(&builder, record.Description)
	writeSearchParts(&builder, record.Tags)
	writeSearchParts(&builder, record.Attributes)
	writeSearchMap(&builder, record.Properties)
	return strings.ToLower(builder.String())
}

func writeSearchParts(builder *strings.Builder, values []string) {
	for _, value := range values {
		writeSearchPart(builder, value)
	}
}

func writeSearchMap(builder *strings.Builder, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		writeSearchPart(builder, key)
		writeSearchPart(builder, values[key])
	}
}

func writeSearchPart(builder *strings.Builder, value string) {
	if value == "" {
		return
	}
	builder.WriteByte(' ')
	builder.WriteString(value)
}
