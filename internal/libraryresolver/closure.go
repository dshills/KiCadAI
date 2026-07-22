package libraryresolver

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

// ClosureRequest describes the concrete library objects selected for one
// design. Units, pins, pads, and variants are retained in the closure identity
// so diagnostics and future derived caches cannot be reused across materially
// different selections.
type ClosureRequest struct {
	Symbols    []SymbolReference    `json:"symbols,omitempty"`
	Footprints []FootprintReference `json:"footprints,omitempty"`
	Variants   []VariantReference   `json:"variants,omitempty"`
}

type SymbolReference struct {
	LibraryID string   `json:"library_id"`
	Units     []int    `json:"units,omitempty"`
	Pins      []string `json:"pins,omitempty"`
}

type FootprintReference struct {
	LibraryID string   `json:"library_id"`
	Pads      []string `json:"pads,omitempty"`
}

type VariantReference struct {
	ComponentID string `json:"component_id"`
	VariantID   string `json:"variant_id"`
	FootprintID string `json:"footprint_id"`
}

type DesignClosure struct {
	Identity   string             `json:"identity"`
	Symbols    []ClosureSymbol    `json:"symbols,omitempty"`
	Footprints []ClosureFootprint `json:"footprints,omitempty"`
	Variants   []VariantReference `json:"variants,omitempty"`
}

type ClosureSymbol struct {
	LibraryID string   `json:"library_id"`
	Sources   []string `json:"sources,omitempty"`
	Extends   string   `json:"extends,omitempty"`
	Units     []int    `json:"units,omitempty"`
	Pins      []string `json:"pins,omitempty"`
}

type ClosureFootprint struct {
	LibraryID string   `json:"library_id"`
	Sources   []string `json:"sources,omitempty"`
	Pads      []string `json:"pads,omitempty"`
}

// ResolveDesignClosure binds a concrete selection to indexed objects and all
// inherited symbol bases. Missing objects make the closure unknowable and are
// therefore blocking.
func ResolveDesignClosure(index LibraryIndex, request ClosureRequest) (DesignClosure, []reports.Issue) {
	request = normalizeClosureRequest(request)
	closure := DesignClosure{Variants: append([]VariantReference(nil), request.Variants...)}
	var issues []reports.Issue
	candidateSources := buildClosureCandidateSourceIndex(index.Inventory)
	seenSymbols := map[string]struct{}{}
	requestedSymbols := make(map[string]SymbolReference, len(request.Symbols))
	for _, reference := range request.Symbols {
		requestedSymbols[reference.LibraryID] = reference
	}
	var addSymbol func(string)
	addSymbol = func(id string) {
		if _, exists := seenSymbols[id]; exists {
			return
		}
		seenSymbols[id] = struct{}{}
		record, exists := index.Symbols[id]
		if !exists {
			closure.Symbols = append(closure.Symbols, ClosureSymbol{LibraryID: id, Sources: candidateSources.symbols[idNickname(id)]})
			issues = append(issues, closureMissingIssue("symbol", id))
			return
		}
		reference := requestedSymbols[id]
		closure.Symbols = append(closure.Symbols, ClosureSymbol{
			LibraryID: id, Sources: nonemptySortedStrings(filepath.ToSlash(record.Path)), Extends: strings.TrimSpace(record.Extends),
			Units: append([]int(nil), reference.Units...), Pins: append([]string(nil), reference.Pins...),
		})
		if base := strings.TrimSpace(record.Extends); base != "" {
			addSymbol(record.LibraryNickname + ":" + base)
		}
	}
	for _, reference := range request.Symbols {
		addSymbol(reference.LibraryID)
	}
	for _, reference := range request.Footprints {
		record, exists := index.Footprints[reference.LibraryID]
		if !exists {
			closure.Footprints = append(closure.Footprints, ClosureFootprint{LibraryID: reference.LibraryID, Sources: candidateSources.footprints[reference.LibraryID]})
			issues = append(issues, closureMissingIssue("footprint", reference.LibraryID))
			continue
		}
		closure.Footprints = append(closure.Footprints, ClosureFootprint{
			LibraryID: reference.LibraryID, Sources: nonemptySortedStrings(filepath.ToSlash(record.Path)), Pads: append([]string(nil), reference.Pads...),
		})
	}
	slices.SortFunc(closure.Symbols, func(a, b ClosureSymbol) int { return cmp.Compare(a.LibraryID, b.LibraryID) })
	slices.SortFunc(closure.Footprints, func(a, b ClosureFootprint) int { return cmp.Compare(a.LibraryID, b.LibraryID) })
	var err error
	closure.Identity, err = closureIdentity(index.Roots, closure)
	if err != nil {
		issues = append(issues, reports.Issue{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityError,
			Path: "library.closure", Message: err.Error(),
		})
	}
	return closure, mergeIssues(nil, issues)
}

// DesignClosureIssues returns only diagnostics associated with the resolved
// closure. Every associated diagnostic is promoted to a blocker; unrelated
// inventory diagnostics remain available on LibraryIndex for explicit audits.
func DesignClosureIssues(index LibraryIndex, closure DesignClosure) []reports.Issue {
	return DesignClosureIssuesFrom(index.Diagnostics, closure)
}

// DesignClosureIssuesFrom classifies one load's diagnostics against a design
// closure. Callers that retain Load's returned issue slice can pass it directly
// so no load-time diagnostic is lost through an intermediate representation.
func DesignClosureIssuesFrom(diagnostics []reports.Issue, closure DesignClosure) []reports.Issue {
	symbolIDs := make(map[string]struct{}, len(closure.Symbols))
	symbolNicknames := map[string]struct{}{}
	footprintIDs := make(map[string]struct{}, len(closure.Footprints))
	footprintNicknames := map[string]struct{}{}
	sources := map[string]struct{}{}
	for _, symbol := range closure.Symbols {
		symbolIDs[symbol.LibraryID] = struct{}{}
		if nickname, _, ok := strings.Cut(symbol.LibraryID, ":"); ok {
			symbolNicknames[nickname] = struct{}{}
		}
		for _, source := range symbol.Sources {
			if source = closureSourceKey(source); source != "" {
				sources[source] = struct{}{}
			}
		}
	}
	for _, footprint := range closure.Footprints {
		footprintIDs[footprint.LibraryID] = struct{}{}
		if nickname, _, ok := strings.Cut(footprint.LibraryID, ":"); ok {
			footprintNicknames[nickname] = struct{}{}
		}
		for _, source := range footprint.Sources {
			if source = closureSourceKey(source); source != "" {
				sources[source] = struct{}{}
			}
		}
	}
	var issues []reports.Issue
	for _, issue := range diagnostics {
		path := filepath.ToSlash(issue.Path)
		if !diagnosticBelongsToClosure(path, issue.Blocking(), symbolIDs, symbolNicknames, footprintIDs, footprintNicknames, sources) {
			continue
		}
		issue.Severity = reports.SeverityBlocked
		issues = append(issues, issue)
	}
	return mergeIssues(nil, issues)
}

func normalizeClosureRequest(request ClosureRequest) ClosureRequest {
	request = cloneClosureRequest(request)
	for index := range request.Symbols {
		request.Symbols[index].LibraryID = strings.TrimSpace(request.Symbols[index].LibraryID)
		slices.Sort(request.Symbols[index].Units)
		request.Symbols[index].Units = slices.Compact(request.Symbols[index].Units)
		slices.Sort(request.Symbols[index].Pins)
		request.Symbols[index].Pins = slices.Compact(request.Symbols[index].Pins)
	}
	for index := range request.Footprints {
		request.Footprints[index].LibraryID = strings.TrimSpace(request.Footprints[index].LibraryID)
		slices.Sort(request.Footprints[index].Pads)
		request.Footprints[index].Pads = slices.Compact(request.Footprints[index].Pads)
	}
	slices.SortFunc(request.Symbols, func(a, b SymbolReference) int { return cmp.Compare(a.LibraryID, b.LibraryID) })
	request.Symbols = compactSymbolReferences(request.Symbols)
	slices.SortFunc(request.Footprints, func(a, b FootprintReference) int { return cmp.Compare(a.LibraryID, b.LibraryID) })
	request.Footprints = compactFootprintReferences(request.Footprints)
	for index := range request.Variants {
		request.Variants[index].ComponentID = strings.TrimSpace(request.Variants[index].ComponentID)
		request.Variants[index].VariantID = strings.TrimSpace(request.Variants[index].VariantID)
		request.Variants[index].FootprintID = strings.TrimSpace(request.Variants[index].FootprintID)
	}
	slices.SortFunc(request.Variants, func(a, b VariantReference) int {
		if value := cmp.Compare(a.ComponentID, b.ComponentID); value != 0 {
			return value
		}
		if value := cmp.Compare(a.VariantID, b.VariantID); value != 0 {
			return value
		}
		return cmp.Compare(a.FootprintID, b.FootprintID)
	})
	request.Variants = slices.Compact(request.Variants)
	return request
}

func cloneClosureRequest(request ClosureRequest) ClosureRequest {
	cloned := ClosureRequest{
		Symbols:    append([]SymbolReference(nil), request.Symbols...),
		Footprints: append([]FootprintReference(nil), request.Footprints...),
		Variants:   append([]VariantReference(nil), request.Variants...),
	}
	for index := range cloned.Symbols {
		cloned.Symbols[index].Units = append([]int(nil), cloned.Symbols[index].Units...)
		cloned.Symbols[index].Pins = append([]string(nil), cloned.Symbols[index].Pins...)
	}
	for index := range cloned.Footprints {
		cloned.Footprints[index].Pads = append([]string(nil), cloned.Footprints[index].Pads...)
	}
	return cloned
}

func compactSymbolReferences(references []SymbolReference) []SymbolReference {
	result := make([]SymbolReference, 0, len(references))
	for _, reference := range references {
		if reference.LibraryID == "" {
			continue
		}
		if len(result) != 0 && result[len(result)-1].LibraryID == reference.LibraryID {
			last := &result[len(result)-1]
			last.Units = append(last.Units, reference.Units...)
			slices.Sort(last.Units)
			last.Units = slices.Compact(last.Units)
			last.Pins = append(last.Pins, reference.Pins...)
			slices.Sort(last.Pins)
			last.Pins = slices.Compact(last.Pins)
			continue
		}
		result = append(result, reference)
	}
	return result
}

func compactFootprintReferences(references []FootprintReference) []FootprintReference {
	result := make([]FootprintReference, 0, len(references))
	for _, reference := range references {
		if reference.LibraryID == "" {
			continue
		}
		if len(result) != 0 && result[len(result)-1].LibraryID == reference.LibraryID {
			last := &result[len(result)-1]
			last.Pads = append(last.Pads, reference.Pads...)
			slices.Sort(last.Pads)
			last.Pads = slices.Compact(last.Pads)
			continue
		}
		result = append(result, reference)
	}
	return result
}

func closureIdentity(roots LibraryRoots, closure DesignClosure) (string, error) {
	encoded, err := json.Marshal(struct {
		Schema int          `json:"schema"`
		Roots  LibraryRoots `json:"roots"`
		// Identity is deliberately absent: it is the output of this hash.
		Symbols    []ClosureSymbol    `json:"symbols,omitempty"`
		Footprints []ClosureFootprint `json:"footprints,omitempty"`
		Variants   []VariantReference `json:"variants,omitempty"`
	}{
		Schema: libraryCacheSchemaVersion, Roots: roots,
		Symbols: closure.Symbols, Footprints: closure.Footprints, Variants: closure.Variants,
	})
	if err != nil {
		return "", fmt.Errorf("encode closure identity: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func closureMissingIssue(kind string, id string) reports.Issue {
	return reports.Issue{
		Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
		Path:    "library." + kind + "." + id,
		Message: "referenced " + kind + " is absent from the library index",
	}
}

type closureCandidateSourceIndex struct {
	symbols    map[string][]string
	footprints map[string][]string
}

func buildClosureCandidateSourceIndex(inventory LibraryInventory) closureCandidateSourceIndex {
	index := closureCandidateSourceIndex{symbols: map[string][]string{}, footprints: map[string][]string{}}
	for _, file := range inventory.SymbolFiles {
		// One .kicad_sym file is a container for every symbol under its
		// nickname, so a missing object may have been invalidated by any
		// file-level diagnostic on that container.
		index.symbols[file.LibraryNickname] = append(index.symbols[file.LibraryNickname], filepath.ToSlash(file.Path))
	}
	for _, file := range inventory.FootprintFiles {
		// One .kicad_mod file contains exactly one footprint object.
		id := file.IDPrefix + file.Name
		index.footprints[id] = append(index.footprints[id], filepath.ToSlash(file.Path))
	}
	for nickname, sources := range index.symbols {
		index.symbols[nickname] = nonemptySortedStrings(sources...)
	}
	for id, sources := range index.footprints {
		index.footprints[id] = nonemptySortedStrings(sources...)
	}
	return index
}

func idNickname(id string) string {
	nickname, _, _ := strings.Cut(id, ":")
	return nickname
}

func nonemptySortedStrings(values ...string) []string {
	result := values[:0]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func diagnosticBelongsToClosure(path string, blocking bool, symbolIDs map[string]struct{}, symbolNicknames map[string]struct{}, footprintIDs map[string]struct{}, footprintNicknames map[string]struct{}, sources map[string]struct{}) bool {
	if _, exists := sources[closureSourceKey(path)]; exists {
		return true
	}
	if id, ok := strings.CutPrefix(path, "library.symbol."); ok {
		if _, exists := symbolIDs[id]; exists {
			return true
		}
		_, exists := symbolNicknames[id]
		return exists
	}
	if id, ok := strings.CutPrefix(path, "library.footprint."); ok {
		if _, exists := footprintIDs[id]; exists {
			return true
		}
		_, exists := footprintNicknames[id]
		return exists
	}
	return blocking && (path == "library.load" || path == "library_cache")
}

func closureSourceKey(source string) string {
	source = filepath.ToSlash(filepath.Clean(strings.TrimSpace(source)))
	if source == "." {
		return ""
	}
	// KiCad library IDs and nicknames remain exact, case-sensitive object
	// identities. Only filesystem source paths follow host case semantics.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		source = strings.ToLower(source)
	}
	return source
}
