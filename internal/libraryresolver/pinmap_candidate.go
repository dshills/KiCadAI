package libraryresolver

import (
	"fmt"
	"strings"

	"kicadai/internal/reports"
)

const (
	pinmapConfidenceNumberMatch          = 0.92
	pinmapConfidenceDuplicateNumberMatch = 0.68
	pinmapConfidenceFunctionMatch        = 0.72
	pinmapConfidenceAmbiguousFunction    = 0.52
)

func GeneratePinmapCandidate(index LibraryIndex, symbolID string, footprintID string) CompatibilityResult {
	symbol, ok := ResolveSymbol(index, symbolID)
	if !ok {
		return CompatibilityResult{SymbolID: symbolID, FootprintID: footprintID, Status: CompatibilityUnknown, Issues: []reports.Issue{missingResolverRecordIssue("library.symbol", symbolID)}}
	}
	footprint, ok := ResolveFootprint(index, footprintID)
	if !ok {
		return CompatibilityResult{SymbolID: symbolID, FootprintID: footprintID, Status: CompatibilityUnknown, Issues: []reports.Issue{missingResolverRecordIssue("library.footprint", footprintID)}}
	}

	pins := uniquePinmapPins(electricalSymbolPins(symbol))
	padCount := uniquePadDesignatorCount(footprint.Pads)
	if padCount < len(pins) {
		return pinmapMismatch(symbol.LibraryID, footprint.FootprintID, fmt.Sprintf("footprint has fewer unique pad designators than symbol electrical pins: %d pads for %d pins", padCount, len(pins)))
	}

	usedPads := map[int]struct{}{}
	padIndex := indexFootprintPads(footprint.Pads)
	candidates := make([]PinmapCandidate, 0, len(pins))
	for _, pin := range pins {
		row, matchedPad, ok := candidateForPin(pin, footprint.Pads, padIndex, usedPads)
		if !ok {
			return pinmapMismatch(symbol.LibraryID, footprint.FootprintID, "no candidate footprint pad for symbol pin "+pinLabel(pin))
		}
		usedPads[matchedPad] = struct{}{}
		candidates = append(candidates, row)
	}

	result := compareSymbolFootprint(symbol, footprint)
	if result.Status == CompatibilityUnknown {
		return result
	}
	if result.Status == CompatibilityIncompatible {
		result = CompatibilityResult{SymbolID: symbol.LibraryID, FootprintID: footprint.FootprintID}
	}
	result.Status = CompatibilityCandidate
	result.PinmapCandidate = candidates
	result.Issues = append(result.Issues, reports.Issue{Code: reports.CodePinmapUnverified, Severity: reports.SeverityWarning, Path: "library.pinmap", Message: "pinmap candidate is inferred and requires human verification"})
	return result
}

func uniquePinmapPins(pins []SymbolPin) []SymbolPin {
	seen := map[string]struct{}{}
	unique := make([]SymbolPin, 0, len(pins))
	for _, pin := range pins {
		key := nameKey(pin.Number)
		if key == "" {
			key = normalizePinToken(pin.Name)
		}
		if key == "" {
			unique = append(unique, pin)
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, pin)
	}
	return unique
}

func uniquePadDesignatorCount(pads []FootprintPad) int {
	seen := map[string]struct{}{}
	for _, pad := range pads {
		name := nameKey(pad.Name)
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	return len(seen)
}

func pinLabel(pin SymbolPin) string {
	number := strings.TrimSpace(pin.Number)
	name := strings.TrimSpace(pin.Name)
	switch {
	case number != "" && name != "":
		return number + " (" + name + ")"
	case number != "":
		return number
	case name != "":
		return name
	default:
		return "<unnamed>"
	}
}

func pinmapMismatch(symbolID string, footprintID string, message string) CompatibilityResult {
	return CompatibilityResult{
		SymbolID:    symbolID,
		FootprintID: footprintID,
		Status:      CompatibilityIncompatible,
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "library.pinmap",
			Message:  message,
		}},
	}
}

func candidateForPin(pin SymbolPin, pads []FootprintPad, padIndex padCandidateIndex, usedPads map[int]struct{}) (PinmapCandidate, int, bool) {
	number := nameKey(pin.Number)
	if number != "" {
		matches := padIndex.byName[number]
		if len(matches) > 0 {
			padIndex := firstUnusedPad(matches, usedPads)
			if padIndex < 0 {
				return PinmapCandidate{}, -1, false
			}
			confidence := pinmapConfidenceNumberMatch
			reason := "symbol pin number matches footprint pad name"
			if len(matches) > 1 {
				confidence = pinmapConfidenceDuplicateNumberMatch
				reason = "symbol pin number matches a duplicate footprint pad group"
			}
			return candidateRow(pin, pads[padIndex], confidence, reason), padIndex, true
		}
	}

	matches := padIndexesByFunction(padIndex, pinFunctionTokens(pin), usedPads)
	if len(matches) == 0 {
		return PinmapCandidate{}, -1, false
	}
	confidence := pinmapConfidenceFunctionMatch
	reason := "symbol pin name or function matches footprint pinfunction hint"
	if len(matches) > 1 {
		confidence = pinmapConfidenceAmbiguousFunction
		reason = "symbol pin name or function matches multiple footprint pinfunction hints"
	}
	return candidateRow(pin, pads[matches[0]], confidence, reason), matches[0], true
}

func candidateRow(pin SymbolPin, pad FootprintPad, confidence float64, reason string) PinmapCandidate {
	function := strings.TrimSpace(pin.FunctionHint)
	if function == "" {
		function = strings.TrimSpace(pin.Name)
	}
	return PinmapCandidate{
		SymbolPin:    strings.TrimSpace(pin.Number),
		SymbolName:   strings.TrimSpace(pin.Name),
		Function:     function,
		FootprintPad: strings.TrimSpace(pad.Name),
		Confidence:   confidence,
		Reason:       reason,
	}
}

func firstUnusedPad(matches []int, usedPads map[int]struct{}) int {
	for _, match := range matches {
		if _, used := usedPads[match]; !used {
			return match
		}
	}
	return -1
}

type padCandidateIndex struct {
	byName     map[string][]int
	byFunction map[string][]int
}

func indexFootprintPads(pads []FootprintPad) padCandidateIndex {
	index := padCandidateIndex{
		byName:     map[string][]int{},
		byFunction: map[string][]int{},
	}
	for i, pad := range pads {
		if name := nameKey(pad.Name); name != "" {
			index.byName[name] = append(index.byName[name], i)
		}
		for _, token := range []string{normalizePinToken(pad.PinFunction), normalizePinToken(pad.PinType)} {
			if token != "" {
				index.byFunction[token] = append(index.byFunction[token], i)
			}
		}
	}
	return index
}

func padIndexesByFunction(index padCandidateIndex, tokens []string, usedPads map[int]struct{}) []int {
	var matches []int
	seen := map[int]struct{}{}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		for _, match := range index.byFunction[token] {
			if _, used := usedPads[match]; used {
				continue
			}
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			matches = append(matches, match)
		}
	}
	return matches
}

func pinFunctionTokens(pin SymbolPin) []string {
	return []string{
		normalizePinToken(pin.Name),
		normalizePinToken(pin.FunctionHint),
	}
}

var pinTokenReplacer = strings.NewReplacer("_", "", "-", "", " ", "")

func normalizePinToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return pinTokenReplacer.Replace(value)
}
