package components

import (
	"fmt"
	"strings"
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
	CodeComponentElectricalMismatch  reports.Code = "COMPONENT_ELECTRICAL_MISMATCH"
	CodeComponentUnitPolicyMissing   reports.Code = "COMPONENT_UNIT_POLICY_MISSING"
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
	pins := map[string][]libraryresolver.SymbolPin{}
	unitSet := map[int]struct{}{}
	for _, pin := range symbol.Pins {
		if pin.Number != "" {
			pins[pin.Number] = append(pins[pin.Number], pin)
		}
		if pin.Unit > 0 {
			unitSet[pin.Unit] = struct{}{}
		}
	}
	var issues []reports.Issue
	if binding.Unit == 0 && len(unitSet) > 1 {
		issues = append(issues, NewIssue(CodeComponentUnitPolicyMissing, reports.SeverityBlocked, path+".unit", "multi-unit component symbol requires an explicit unit policy: "+symbol.LibraryID))
	}
	for i, functionPin := range binding.FunctionPins {
		candidates, ok := pins[functionPin.SymbolPin]
		if !ok {
			issues = append(issues, NewIssue(CodeComponentFunctionPinUnmapped, reports.SeverityBlocked, fmt.Sprintf("%s.function_pins[%d]", path, i), "function pin does not exist on resolved symbol: "+functionPin.SymbolPin))
			continue
		}
		candidates = matchingUnitPins(candidates, binding.Unit)
		if len(candidates) == 0 {
			issues = append(issues, NewIssue(CodeComponentFunctionPinUnmapped, reports.SeverityBlocked, fmt.Sprintf("%s.function_pins[%d]", path, i), "function pin does not exist on requested symbol unit: "+functionPin.SymbolPin))
			continue
		}
		expectedElectrical := strings.ToLower(strings.TrimSpace(functionPin.Electrical))
		if expectedElectrical != "" {
			if pin, actualElectrical, ok := firstElectricalMismatch(candidates, expectedElectrical); ok {
				issues = append(issues, NewIssue(CodeComponentElectricalMismatch, reports.SeverityBlocked, fmt.Sprintf("%s.function_pins[%d].electrical", path, i), "function pin electrical type "+expectedElectrical+" does not match resolved symbol pin "+symbolPinLabel(pin)+": "+actualElectrical))
			}
		}
	}
	return issues
}

func matchingUnitPins(pins []libraryresolver.SymbolPin, unit int) []libraryresolver.SymbolPin {
	if unit == 0 {
		return pins
	}
	matches := make([]libraryresolver.SymbolPin, 0, len(pins))
	for _, pin := range pins {
		if pin.Unit == 0 || pin.Unit == unit {
			matches = append(matches, pin)
		}
	}
	return matches
}

func firstElectricalMismatch(pins []libraryresolver.SymbolPin, expected string) (libraryresolver.SymbolPin, string, bool) {
	if len(pins) == 0 {
		return libraryresolver.SymbolPin{}, "", false
	}
	firstActual := ""
	for _, pin := range pins {
		actual := strings.ToLower(strings.TrimSpace(pin.ElectricalType))
		if firstActual == "" {
			firstActual = actual
		}
		if actual == "" || actual == expected {
			return libraryresolver.SymbolPin{}, "", false
		}
	}
	return pins[0], firstActual, true
}

func symbolPinLabel(pin libraryresolver.SymbolPin) string {
	name := strings.TrimSpace(pin.Name)
	if name == "" {
		return pin.Number
	}
	return pin.Number + " (" + name + ")"
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
