package components

import (
	"fmt"
	"sync"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/pinmap"
	"kicadai/internal/reports"
)

var builtinPinmapIndex struct {
	once    sync.Once
	entries map[pinmapKey]pinmap.Entry
}

type pinmapKey struct {
	Symbol    string
	Footprint string
}

const (
	CodeComponentSymbolUnresolved    reports.Code = "COMPONENT_SYMBOL_UNRESOLVED"
	CodeComponentFootprintUnresolved reports.Code = "COMPONENT_FOOTPRINT_UNRESOLVED"
	CodeComponentPinmapMissing       reports.Code = "COMPONENT_PINMAP_MISSING"
	CodeComponentFunctionPinUnmapped reports.Code = "COMPONENT_FUNCTION_PIN_UNMAPPED"
	CodeComponentPadFunctionUnmapped reports.Code = "COMPONENT_PAD_FUNCTION_UNMAPPED"
)

type EvidenceOptions struct {
	LibraryIndex      *libraryresolver.LibraryIndex `json:"-"`
	RequirePinmaps    bool                          `json:"require_pinmaps,omitempty"`
	AllowPassiveRules bool                          `json:"allow_passive_rules,omitempty"`
}

func ValidateCatalogEvidence(catalog *Catalog, opts EvidenceOptions) reports.Result {
	if catalog == nil {
		return ValidateCatalog(catalog)
	}
	base := ValidateCatalog(catalog)
	issues := append([]reports.Issue{}, base.Issues...)
	if opts.LibraryIndex == nil {
		if opts.RequirePinmaps {
			issues = append(issues, NewIssue(CodeComponentPinmapMissing, reports.SeverityBlocked, "component.evidence.library_index", "pinmap evidence requires a library index"))
		}
		return reports.ResultWithIssues("component validate evidence", base.Data, issues, nil)
	}
	for i, record := range catalog.Records {
		recordPath := fmt.Sprintf("records[%d]", i)
		for j, symbol := range record.Symbols {
			symbolPath := fmt.Sprintf("%s.symbols[%d]", recordPath, j)
			resolvedSymbol, ok := opts.LibraryIndex.Symbols[symbol.SymbolID]
			if !ok {
				issues = append(issues, NewIssue(CodeComponentSymbolUnresolved, reports.SeverityBlocked, symbolPath+".symbol_id", "component symbol does not resolve: "+symbol.SymbolID))
				continue
			}
			issues = append(issues, functionPinIssues(symbolPath, symbol, resolvedSymbol)...)
		}
		for j, variant := range record.Packages {
			variantPath := fmt.Sprintf("%s.packages[%d]", recordPath, j)
			resolvedFootprint, ok := opts.LibraryIndex.Footprints[variant.FootprintID]
			if !ok {
				issues = append(issues, NewIssue(CodeComponentFootprintUnresolved, reports.SeverityBlocked, variantPath+".footprint_id", "component footprint does not resolve: "+variant.FootprintID))
				continue
			}
			issues = append(issues, padFunctionIssues(variantPath, variant, resolvedFootprint)...)
		}
		if opts.RequirePinmaps && record.Verification.Confidence == ConfidenceVerified && !recordHasAllPinmaps(record) {
			if !(opts.AllowPassiveRules && passiveRuleInferred(record)) {
				issues = append(issues, NewIssue(CodeComponentPinmapMissing, reports.SeverityBlocked, recordPath+".verification", "verified component record requires a verified symbol-footprint pinmap"))
			}
		}
	}
	sortIssues(issues)
	return reports.ResultWithIssues("component validate evidence", base.Data, issues, nil)
}

func functionPinIssues(path string, binding SymbolBinding, symbol libraryresolver.SymbolRecord) []reports.Issue {
	pins := map[string]struct{}{}
	for _, pin := range symbol.Pins {
		pins[pin.Number] = struct{}{}
	}
	var issues []reports.Issue
	for i, functionPin := range binding.FunctionPins {
		if _, ok := pins[functionPin.SymbolPin]; !ok {
			issues = append(issues, NewIssue(CodeComponentFunctionPinUnmapped, reports.SeverityBlocked, fmt.Sprintf("%s.function_pins[%d]", path, i), "function pin does not exist on resolved symbol: "+functionPin.SymbolPin))
		}
	}
	return issues
}

func padFunctionIssues(path string, variant PackageVariant, footprint libraryresolver.FootprintRecord) []reports.Issue {
	pads := map[string]struct{}{}
	for _, pad := range footprint.Pads {
		pads[pad.Name] = struct{}{}
	}
	var issues []reports.Issue
	for i, padFunction := range variant.PadFunctions {
		if _, ok := pads[padFunction.Pad]; !ok {
			issues = append(issues, NewIssue(CodeComponentPadFunctionUnmapped, reports.SeverityBlocked, fmt.Sprintf("%s.pad_functions[%d]", path, i), "pad function does not exist on resolved footprint: "+padFunction.Pad))
		}
	}
	return issues
}

func recordHasAllPinmaps(record ComponentRecord) bool {
	for _, symbol := range record.Symbols {
		for _, variant := range record.Packages {
			if !hasBuiltinPinmap(symbol.SymbolID, variant.FootprintID) {
				return false
			}
		}
	}
	return len(record.Symbols) > 0 && len(record.Packages) > 0
}

func hasBuiltinPinmap(symbol string, footprint string) bool {
	builtinPinmapIndex.once.Do(func() {
		out := map[pinmapKey]pinmap.Entry{}
		for _, entry := range pinmap.Builtins() {
			out[pinmapKey{Symbol: entry.Symbol, Footprint: entry.Footprint}] = entry
		}
		builtinPinmapIndex.entries = out
	})
	_, ok := builtinPinmapIndex.entries[pinmapKey{Symbol: symbol, Footprint: footprint}]
	return ok
}
