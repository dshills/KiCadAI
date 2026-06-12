package libraryresolver

import (
	"path"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

func ValidateAssignment(index LibraryIndex, symbolID string, footprintID string) CompatibilityResult {
	symbol, ok := ResolveSymbol(index, symbolID)
	if !ok {
		return CompatibilityResult{SymbolID: symbolID, FootprintID: footprintID, Status: CompatibilityUnknown, Issues: []reports.Issue{missingResolverRecordIssue("library.symbol", symbolID)}}
	}
	footprint, ok := ResolveFootprint(index, footprintID)
	if !ok {
		return CompatibilityResult{SymbolID: symbolID, FootprintID: footprintID, Status: CompatibilityUnknown, Issues: []reports.Issue{missingResolverRecordIssue("library.footprint", footprintID)}}
	}
	return compareSymbolFootprint(symbol, footprint)
}

func missingResolverRecordIssue(path string, id string) reports.Issue {
	return reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: path, Message: "library record not found: " + id}
}

func CompatibleFootprints(index LibraryIndex, symbolID string, opts MatchOptions) []CompatibilityResult {
	symbol, ok := ResolveSymbol(index, symbolID)
	if !ok {
		return nil
	}
	pins := electricalSymbolPins(symbol)
	symbolPackageSearch := packageSearchText(symbol.Name, symbol.Description, symbol.Keywords)
	results := make([]CompatibilityResult, 0, len(index.Footprints))
	for _, footprint := range index.Footprints {
		result := compareSymbolFootprintWithPins(symbol, pins, symbolPackageSearch, footprint)
		if result.Status != CompatibilityIncompatible && result.Status != CompatibilityUnknown {
			results = append(results, result)
		}
	}
	slices.SortFunc(results, func(a, b CompatibilityResult) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(a.FootprintID, b.FootprintID)
	})
	if opts.Limit > 0 && opts.Limit < len(results) {
		return results[:opts.Limit]
	}
	return results
}

func compareSymbolFootprint(symbol SymbolRecord, footprint FootprintRecord) CompatibilityResult {
	return compareSymbolFootprintWithPins(symbol, electricalSymbolPins(symbol), packageSearchText(symbol.Name, symbol.Description, symbol.Keywords), footprint)
}

func compareSymbolFootprintWithPins(symbol SymbolRecord, electricalPins []SymbolPin, symbolPackageSearch string, footprint FootprintRecord) CompatibilityResult {
	result := CompatibilityResult{SymbolID: symbol.LibraryID, FootprintID: footprint.FootprintID, Status: CompatibilityNeedsVerification}
	mappedPins := uniquePinmapPins(electricalPins)
	if len(footprint.Pads) == 0 && len(mappedPins) > 0 {
		result.Status = CompatibilityIncompatible
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.compatibility", Message: "footprint has no pads for electrical symbol pins"})
		return result
	}
	if uniquePadDesignatorCount(footprint.Pads) < len(mappedPins) {
		result.Status = CompatibilityIncompatible
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.compatibility", Message: "footprint has fewer pads than symbol electrical pins"})
		return result
	}
	missingPins := missingPadNames(mappedPins, footprint.Pads)
	if len(missingPins) > 0 {
		result.Status = CompatibilityIncompatible
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.compatibility", Message: "footprint is missing pads for symbol pins: " + strings.Join(missingPins, ", ")})
		return result
	}
	result.Score = 0.35
	result.Evidence = append(result.Evidence, CompatibilityEvidence{Kind: "pin_coverage", Message: "footprint contains pads for all symbol electrical pins", Score: 0.35})
	if footprintFilterMatches(symbol, footprint) {
		result.Score += 0.35
		result.Evidence = append(result.Evidence, CompatibilityEvidence{Kind: "footprint_filter", Message: "symbol footprint filter matches footprint", Score: 0.35})
	}
	if packageFamilyMatches(symbolPackageSearch, footprint) {
		result.Score += 0.2
		result.Evidence = append(result.Evidence, CompatibilityEvidence{Kind: "package_family", Message: "symbol and footprint names share a package family hint", Score: 0.2})
	}
	if trustedPassiveMatch(mappedPins, footprint.Pads) && result.Score >= 0.7 {
		result.Status = CompatibilityCompatible
		return result
	}
	if result.Score >= 0.55 {
		result.Status = CompatibilityCandidate
	} else {
		result.Status = CompatibilityNeedsVerification
	}
	result.Issues = append(result.Issues, reports.Issue{Code: reports.CodePinmapUnverified, Severity: reports.SeverityWarning, Path: "library.compatibility", Message: "symbol-footprint assignment needs pinmap verification"})
	return result
}

func electricalSymbolPins(symbol SymbolRecord) []SymbolPin {
	var pins []SymbolPin
	for _, pin := range symbol.Pins {
		if pin.Electrical == "no_connect" {
			continue
		}
		pins = append(pins, pin)
	}
	return pins
}

func missingPadNames(pins []SymbolPin, pads []FootprintPad) []string {
	lookup := newPadLookup(pins, pads)
	var missing []string
	for _, pin := range pins {
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			continue
		}
		if !lookup.has(nameKey(number)) {
			missing = append(missing, number)
		}
	}
	return missing
}

type padLookup struct {
	pads []FootprintPad
	set  map[string]struct{}
}

func newPadLookup(pins []SymbolPin, pads []FootprintPad) padLookup {
	if len(pins)*len(pads) < 128 {
		return padLookup{pads: pads}
	}
	set := make(map[string]struct{}, len(pads))
	for _, pad := range pads {
		name := nameKey(pad.Name)
		if name != "" {
			set[name] = struct{}{}
		}
	}
	return padLookup{set: set}
}

func (lookup padLookup) has(name string) bool {
	if lookup.set != nil {
		_, ok := lookup.set[name]
		return ok
	}
	return hasPadName(lookup.pads, name)
}

func hasPadName(pads []FootprintPad, name string) bool {
	for _, pad := range pads {
		if nameKey(pad.Name) == name {
			return true
		}
	}
	return false
}

func nameKey(name string) string {
	return strings.TrimSpace(name)
}

func footprintFilterMatches(symbol SymbolRecord, footprint FootprintRecord) bool {
	candidates := []string{footprint.Name, footprint.FootprintID}
	for _, filter := range symbol.FootprintFilter {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		for _, candidate := range candidates {
			ok, err := path.Match(filter, candidate)
			if err != nil {
				continue
			}
			if ok {
				return true
			}
		}
	}
	return false
}

func packageSearchText(name string, description string, keywords []string) string {
	return strings.ToLower(name + " " + description + " " + strings.Join(keywords, " "))
}

func packageFamilyMatches(symbolPackageSearch string, footprint FootprintRecord) bool {
	for _, token := range []string{"resistor", "capacitor", "led", "diode", "transistor", "connector", "soic", "sot", "to-92"} {
		if !strings.Contains(symbolPackageSearch, token) {
			continue
		}
		if footprintContainsToken(footprint, token) {
			return true
		}
	}
	return false
}

func footprintContainsToken(footprint FootprintRecord, token string) bool {
	if strings.Contains(strings.ToLower(footprint.FootprintID), token) || strings.Contains(strings.ToLower(footprint.Description), token) {
		return true
	}
	for _, tag := range footprint.Tags {
		if strings.Contains(strings.ToLower(tag), token) {
			return true
		}
	}
	return false
}

func trustedPassiveMatch(pins []SymbolPin, pads []FootprintPad) bool {
	if len(pins) != 2 || len(pads) != 2 {
		return false
	}
	for _, pin := range pins {
		if pin.Electrical != "passive" {
			return false
		}
		if pin.Number != "1" && pin.Number != "2" {
			return false
		}
	}
	return hasPadName(pads, "1") && hasPadName(pads, "2")
}
