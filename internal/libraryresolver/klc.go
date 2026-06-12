package libraryresolver

import (
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
)

type KLCReport struct {
	Kind   string          `json:"kind"`
	ID     string          `json:"id"`
	Issues []reports.Issue `json:"issues,omitempty"`
}

const (
	klcSymbolDescription = "KLC S4.1"
	klcSymbolKeywords    = "KLC S4.2"
	klcSymbolPins        = "KLC S4.3"
	klcFootprintMetadata = "KLC F5.1"
	klcFootprintTags     = "KLC F5.2"
	klcFootprintCourt    = "KLC F5.3"
	klcFootprintFab      = "KLC F5.4"
	klcFootprintSilk     = "KLC F5.5"
	klcFootprintPads     = "KLC F6.1"
	klcFootprintAttrs    = "KLC F7.1"
)

func ValidateSymbolKLC(index LibraryIndex, symbolID string) KLCReport {
	report := KLCReport{Kind: "symbol", ID: symbolID, Issues: []reports.Issue{}}
	symbol, ok := ResolveSymbol(index, symbolID)
	if !ok {
		report.Issues = append(report.Issues, missingResolverRecordIssue("library.symbol", symbolID))
		return report
	}
	if strings.TrimSpace(symbol.Description) == "" {
		report.Issues = append(report.Issues, klcWarning("library.symbol."+symbolID, "symbol is missing description metadata", klcSymbolDescription))
	}
	if len(symbol.Keywords) == 0 {
		report.Issues = append(report.Issues, klcWarning("library.symbol."+symbolID, "symbol is missing keyword metadata", klcSymbolKeywords))
	}
	if len(symbol.Pins) == 0 {
		report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.symbol." + symbolID, Message: "symbol has no pins"})
	}
	pinsByNumber := make(map[string][]SymbolPin, len(symbol.Pins))
	for _, pin := range symbol.Pins {
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.symbol." + symbolID, Message: "symbol contains a pin without a number"})
			continue
		}
		pinsByNumber[number] = append(pinsByNumber[number], pin)
	}
	pinNumbers := make([]string, 0, len(pinsByNumber))
	for number := range pinsByNumber {
		pinNumbers = append(pinNumbers, number)
	}
	sort.Strings(pinNumbers)
	for _, number := range pinNumbers {
		pins := pinsByNumber[number]
		if len(pins) < 2 {
			continue
		}
		report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.symbol." + symbolID, Message: "symbol contains duplicate pin number " + number, Suggestion: "review " + klcSymbolPins})
	}
	return report
}

func ValidateFootprintKLC(index LibraryIndex, footprintID string) KLCReport {
	report := KLCReport{Kind: "footprint", ID: footprintID, Issues: []reports.Issue{}}
	footprint, ok := ResolveFootprint(index, footprintID)
	if !ok {
		report.Issues = append(report.Issues, missingResolverRecordIssue("library.footprint", footprintID))
		return report
	}
	if strings.TrimSpace(footprint.Description) == "" {
		report.Issues = append(report.Issues, klcWarning("library.footprint."+footprintID, "footprint is missing description metadata", klcFootprintMetadata))
	}
	if len(footprint.Tags) == 0 {
		report.Issues = append(report.Issues, klcWarning("library.footprint."+footprintID, "footprint is missing tags", klcFootprintTags))
	}
	if len(footprint.Pads) == 0 {
		report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.footprint." + footprintID, Message: "footprint has no pads"})
	}
	for _, pad := range footprint.Pads {
		name := strings.TrimSpace(pad.Name)
		padType := strings.ToLower(strings.TrimSpace(pad.Type))
		padHasCopper := hasCopperLayer(pad.Layers)
		if isNonElectricalPad(padType) && padHasCopper {
			report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.footprint." + footprintID, Message: "non-electrical pad must not include a copper layer", Suggestion: "review " + klcFootprintPads})
		}
		if name == "" && requiresCopperLayer(padType) {
			report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.footprint." + footprintID, Message: "footprint contains an electrical pad without a name", Suggestion: "review " + klcFootprintPads})
		}
		if requiresCopperLayer(padType) && !padHasCopper {
			message := "electrical pad does not include a copper layer"
			if name != "" {
				message = "pad " + name + " does not include a copper layer"
			}
			report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.footprint." + footprintID, Message: message, Suggestion: "review " + klcFootprintPads})
		}
	}
	if !footprint.GraphicsSummary.HasCourtyard {
		report.Issues = append(report.Issues, klcWarning("library.footprint."+footprintID, "footprint has no courtyard graphics", klcFootprintCourt))
	}
	if !footprint.GraphicsSummary.HasFabOutline {
		report.Issues = append(report.Issues, klcWarning("library.footprint."+footprintID, "footprint has no fabrication outline", klcFootprintFab))
	}
	if !footprint.GraphicsSummary.HasSilk {
		report.Issues = append(report.Issues, klcWarning("library.footprint."+footprintID, "footprint has no silkscreen graphics", klcFootprintSilk))
	}
	if len(footprint.Attributes) == 0 {
		report.Issues = append(report.Issues, klcWarning("library.footprint."+footprintID, "footprint has no attributes", klcFootprintAttrs))
	}
	return report
}

func klcWarning(path string, message string, reference string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: path, Message: message, Suggestion: "review " + reference}
}

func hasCopperLayer(layers []kicadfiles.BoardLayer) bool {
	for _, layer := range layers {
		name := string(layer)
		if layer == kicadfiles.LayerAllCu || strings.HasSuffix(name, ".Cu") {
			return true
		}
	}
	return false
}

func requiresCopperLayer(padType string) bool {
	switch padType {
	case "smd", "thru_hole", "connect":
		return true
	default:
		return false
	}
}

func isNonElectricalPad(padType string) bool {
	switch padType {
	case "np_thru_hole", "npth":
		return true
	case "mechanical":
		return true
	default:
		return false
	}
}
