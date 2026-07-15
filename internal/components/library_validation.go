package components

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

const (
	CodeLibrarySymbolMissing       reports.Code = "COMPONENT_LIBRARY_SYMBOL_MISSING"
	CodeLibrarySymbolUnitMissing   reports.Code = "COMPONENT_LIBRARY_SYMBOL_UNIT_MISSING"
	CodeLibrarySymbolPinMissing    reports.Code = "COMPONENT_LIBRARY_SYMBOL_PIN_MISSING"
	CodeLibraryFootprintMissing    reports.Code = "COMPONENT_LIBRARY_FOOTPRINT_MISSING"
	CodeLibraryFootprintPadMissing reports.Code = "COMPONENT_LIBRARY_FOOTPRINT_PAD_MISSING"
)

// LibraryValidationSummary describes the resolver evidence checked for a
// catalog. Structural catalog validation remains available without configured
// library roots; this pass is authoritative whenever roots are configured.
type LibraryValidationSummary struct {
	Configured                 bool `json:"configured"`
	SymbolValidationEnabled    bool `json:"symbol_validation_enabled"`
	FootprintValidationEnabled bool `json:"footprint_validation_enabled"`
	SelectableRecords          int  `json:"selectable_records"`
	SymbolBindingsChecked      int  `json:"symbol_bindings_checked"`
	PackageVariantsChecked     int  `json:"package_variants_checked"`
}

// ValidateCatalogLibraries resolves selectable catalog bindings against a
// loaded KiCad library index. Explicitly blocked records and bindings are not
// selectable and are therefore excluded.
func ValidateCatalogLibraries(catalog *Catalog, index libraryresolver.LibraryIndex) (LibraryValidationSummary, []reports.Issue) {
	summary := LibraryValidationSummary{
		SymbolValidationEnabled:    strings.TrimSpace(index.Roots.SymbolsRoot) != "",
		FootprintValidationEnabled: strings.TrimSpace(index.Roots.FootprintsRoot) != "",
	}
	summary.Configured = summary.SymbolValidationEnabled || summary.FootprintValidationEnabled
	if catalog == nil || !summary.Configured {
		return summary, nil
	}

	records := append([]ComponentRecord(nil), catalog.Records...)
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	var issues []reports.Issue
	for _, record := range records {
		if record.Verification.Confidence == ConfidenceBlocked {
			continue
		}
		summary.SelectableRecords++
		if summary.SymbolValidationEnabled {
			for symbolIndex, binding := range record.Symbols {
				if binding.Verification.Confidence == ConfidenceBlocked {
					continue
				}
				summary.SymbolBindingsChecked++
				path := fmt.Sprintf("catalog.records.%s.symbols.%d", record.ID, symbolIndex)
				issues = append(issues, validateCatalogSymbolBinding(index, binding, path)...)
			}
		}
		if summary.FootprintValidationEnabled {
			for packageIndex, variant := range record.Packages {
				if variant.Verification.Confidence == ConfidenceBlocked {
					continue
				}
				summary.PackageVariantsChecked++
				path := fmt.Sprintf("catalog.records.%s.packages.%d", record.ID, packageIndex)
				issues = append(issues, validateCatalogPackageVariant(index, variant, path)...)
			}
		}
	}
	sortIssues(issues)
	return summary, issues
}

func validateCatalogSymbolBinding(index libraryresolver.LibraryIndex, binding SymbolBinding, path string) []reports.Issue {
	symbol, ok := libraryresolver.ResolveSymbol(index, binding.SymbolID)
	if !ok {
		return []reports.Issue{libraryEvidenceIssue(CodeLibrarySymbolMissing, path+".symbol_id", "symbol is absent from configured KiCad library evidence: "+binding.SymbolID)}
	}
	var issues []reports.Issue
	if binding.Unit > 0 && !librarySymbolHasUnit(symbol, binding.Unit) {
		return []reports.Issue{libraryEvidenceIssue(CodeLibrarySymbolUnitMissing, path+".unit", fmt.Sprintf("symbol %s has no unit %d", binding.SymbolID, binding.Unit))}
	}
	for functionIndex, functionPin := range binding.FunctionPins {
		if !librarySymbolHasPin(symbol, binding.Unit, functionPin.SymbolPin) {
			issues = append(issues, libraryEvidenceIssue(
				CodeLibrarySymbolPinMissing,
				fmt.Sprintf("%s.function_pins.%d.symbol_pin", path, functionIndex),
				fmt.Sprintf("symbol %s unit %d has no declared pin %s", binding.SymbolID, binding.Unit, functionPin.SymbolPin),
			))
		}
	}
	return issues
}

func validateCatalogPackageVariant(index libraryresolver.LibraryIndex, variant PackageVariant, path string) []reports.Issue {
	footprint, ok := libraryresolver.ResolveFootprint(index, variant.FootprintID)
	if !ok {
		return []reports.Issue{libraryEvidenceIssue(CodeLibraryFootprintMissing, path+".footprint_id", "footprint is absent from configured KiCad library evidence: "+variant.FootprintID)}
	}
	pads := make(map[string]struct{}, len(footprint.Pads))
	for _, pad := range footprint.Pads {
		pads[strings.TrimSpace(pad.Name)] = struct{}{}
	}
	var issues []reports.Issue
	for padIndex, padFunction := range variant.PadFunctions {
		if _, exists := pads[strings.TrimSpace(padFunction.Pad)]; !exists {
			issues = append(issues, libraryEvidenceIssue(
				CodeLibraryFootprintPadMissing,
				fmt.Sprintf("%s.pad_functions.%d.pad", path, padIndex),
				fmt.Sprintf("footprint %s has no declared pad %s", variant.FootprintID, padFunction.Pad),
			))
		}
	}
	return issues
}

func librarySymbolHasUnit(symbol libraryresolver.SymbolRecord, unit int) bool {
	for _, candidate := range symbol.Units {
		if candidate.Unit == unit {
			return true
		}
	}
	for _, pin := range symbol.Pins {
		if pin.Unit == unit || pin.Common {
			return true
		}
	}
	return false
}

func librarySymbolHasPin(symbol libraryresolver.SymbolRecord, unit int, number string) bool {
	number = strings.TrimSpace(number)
	for _, pin := range symbol.Pins {
		if strings.TrimSpace(pin.Number) != number {
			continue
		}
		if unit <= 0 || pin.Unit == unit || pin.Common {
			return true
		}
	}
	return false
}

func libraryEvidenceIssue(code reports.Code, path, message string) reports.Issue {
	return reports.Issue{
		Code:       code,
		Severity:   reports.SeverityError,
		Path:       path,
		Message:    message,
		Suggestion: "correct the catalog identity or configure matching KiCad library roots",
	}
}
