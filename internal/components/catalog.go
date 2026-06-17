package components

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kicadai/internal/reports"
)

const DefaultCatalogDir = "data/components"

const (
	CodeCatalogEmpty            reports.Code = "COMPONENT_CATALOG_EMPTY"
	CodeCatalogReadFailed       reports.Code = "COMPONENT_CATALOG_READ_FAILED"
	CodeCatalogParseFailed      reports.Code = "COMPONENT_CATALOG_PARSE_FAILED"
	CodeDuplicateComponentID    reports.Code = "COMPONENT_DUPLICATE_ID"
	CodeUnknownFamily           reports.Code = "COMPONENT_UNKNOWN_FAMILY"
	CodeMissingSymbolBinding    reports.Code = "COMPONENT_MISSING_SYMBOL"
	CodeMissingPackageVariant   reports.Code = "COMPONENT_MISSING_PACKAGE"
	CodeMissingFootprint        reports.Code = "COMPONENT_MISSING_FOOTPRINT"
	CodeInvalidFunctionPin      reports.Code = "COMPONENT_INVALID_FUNCTION_PIN"
	CodeInvalidPadFunction      reports.Code = "COMPONENT_INVALID_PAD_FUNCTION"
	CodeInvalidConstraint       reports.Code = "COMPONENT_INVALID_CONSTRAINT"
	CodeInvalidComponentID      reports.Code = "COMPONENT_INVALID_ID"
	CodeInvalidComponentFamily  reports.Code = "COMPONENT_INVALID_FAMILY"
	CodeInvalidComponentPackage reports.Code = "COMPONENT_INVALID_PACKAGE"
)

type LoadOptions struct {
	CatalogDir string `json:"catalog_dir,omitempty"`
}

type catalogFile struct {
	Version  string             `json:"version,omitempty"`
	Families []FamilyDefinition `json:"families,omitempty"`
	Records  []ComponentRecord  `json:"records,omitempty"`
}

func LoadCatalog(ctx context.Context, opts LoadOptions) (*Catalog, error) {
	dir := strings.TrimSpace(opts.CatalogDir)
	if dir == "" {
		dir = DefaultCatalogDir
	}
	cleanDir, err := cleanCatalogDir(dir)
	if err != nil {
		return nil, err
	}
	dir = cleanDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	now := time.Now().UTC()
	catalog := &Catalog{
		Version:     CatalogVersion,
		GeneratedAt: &now,
		Records:     []ComponentRecord{},
		Families:    []FamilyDefinition{},
		Diagnostics: []reports.Issue{},
	}
	if len(files) == 0 {
		catalog.Diagnostics = append(catalog.Diagnostics, NewIssue(CodeCatalogEmpty, reports.SeverityWarning, dir, "component catalog directory contains no JSON files"))
		return catalog, nil
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		partial, issues := readCatalogFile(file)
		catalog.Diagnostics = append(catalog.Diagnostics, issues...)
		if partial.Version != "" && catalog.Version == CatalogVersion {
			catalog.Version = partial.Version
		} else if partial.Version != "" && partial.Version != catalog.Version {
			catalog.Diagnostics = append(catalog.Diagnostics, NewIssue(CodeCatalogParseFailed, reports.SeverityWarning, file, "component catalog version differs from earlier files: "+partial.Version))
		}
		catalog.Families = append(catalog.Families, partial.Families...)
		catalog.Records = append(catalog.Records, partial.Records...)
	}
	SortCatalog(catalog)
	catalog.Diagnostics = append(catalog.Diagnostics, ValidateCatalog(catalog).Issues...)
	sortIssues(catalog.Diagnostics)
	return catalog, nil
}

func ValidateCatalog(catalog *Catalog) reports.Result {
	if catalog == nil {
		return reports.ErrorResult("component validate", NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "catalog", "component catalog is nil"))
	}
	var issues []reports.Issue
	families := map[string]struct{}{}
	for i, family := range catalog.Families {
		path := fmt.Sprintf("families[%d]", i)
		if strings.TrimSpace(family.ID) == "" {
			issues = append(issues, NewIssue(CodeInvalidComponentFamily, reports.SeverityBlocked, path+".id", "component family id is required"))
			continue
		}
		if _, ok := families[family.ID]; ok {
			issues = append(issues, NewIssue(CodeInvalidComponentFamily, reports.SeverityBlocked, path+".id", "duplicate component family id: "+family.ID))
			continue
		}
		families[family.ID] = struct{}{}
	}
	seen := map[string]int{}
	for i, record := range catalog.Records {
		path := fmt.Sprintf("records[%d]", i)
		if strings.TrimSpace(record.ID) == "" {
			issues = append(issues, NewIssue(CodeInvalidComponentID, reports.SeverityBlocked, path+".id", "component id is required"))
		} else if first, ok := seen[record.ID]; ok {
			issues = append(issues, NewIssue(CodeDuplicateComponentID, reports.SeverityBlocked, path+".id", fmt.Sprintf("component id %q duplicates records[%d]", record.ID, first)))
		} else {
			seen[record.ID] = i
		}
		if strings.TrimSpace(record.Family) == "" {
			issues = append(issues, NewIssue(CodeInvalidComponentFamily, reports.SeverityBlocked, path+".family", "component family is required"))
		} else if _, ok := families[record.Family]; !ok {
			issues = append(issues, NewIssue(CodeUnknownFamily, reports.SeverityBlocked, path+".family", "component references unknown family: "+record.Family))
		}
		if issue, ok := ValidateConfidenceIssue(path+".verification.confidence", record.Verification.Confidence); ok {
			issues = append(issues, issue)
		}
		if len(record.Symbols) == 0 {
			issues = append(issues, NewIssue(CodeMissingSymbolBinding, reports.SeverityBlocked, path+".symbols", "component record has no symbol bindings"))
		}
		if len(record.Packages) == 0 {
			issues = append(issues, NewIssue(CodeMissingPackageVariant, reports.SeverityBlocked, path+".packages", "component record has no package variants"))
		}
		issues = append(issues, validateSymbols(path, record.Symbols)...)
		issues = append(issues, validatePackages(path, record.Packages)...)
		issues = append(issues, validateConstraints(path+".values", valueConstraintsAsGeneric(record.Values))...)
		issues = append(issues, validateConstraints(path+".ratings", ratingConstraintsAsGeneric(record.Ratings))...)
	}
	sortIssues(issues)
	return reports.ResultWithIssues("component validate", map[string]any{
		"family_count": len(catalog.Families),
		"record_count": len(catalog.Records),
	}, issues, nil)
}

func readCatalogFile(path string) (catalogFile, []reports.Issue) {
	body, err := os.ReadFile(path)
	if err != nil {
		return catalogFile{}, []reports.Issue{NewIssue(CodeCatalogReadFailed, reports.SeverityBlocked, path, err.Error())}
	}
	var file catalogFile
	if err := json.Unmarshal(body, &file); err != nil {
		return catalogFile{}, []reports.Issue{NewIssue(CodeCatalogParseFailed, reports.SeverityBlocked, path, err.Error())}
	}
	return file, nil
}

func validateSymbols(path string, symbols []SymbolBinding) []reports.Issue {
	var issues []reports.Issue
	for i, symbol := range symbols {
		symbolPath := fmt.Sprintf("%s.symbols[%d]", path, i)
		if strings.TrimSpace(symbol.SymbolID) == "" {
			issues = append(issues, NewIssue(CodeMissingSymbolBinding, reports.SeverityBlocked, symbolPath+".symbol_id", "symbol binding requires symbol_id"))
		}
		if issue, ok := ValidateConfidenceIssue(symbolPath+".verification.confidence", symbol.Verification.Confidence); ok {
			issues = append(issues, issue)
		}
		for j, pin := range symbol.FunctionPins {
			pinPath := fmt.Sprintf("%s.function_pins[%d]", symbolPath, j)
			if strings.TrimSpace(pin.Function) == "" || strings.TrimSpace(pin.SymbolPin) == "" {
				issues = append(issues, NewIssue(CodeInvalidFunctionPin, reports.SeverityBlocked, pinPath, "function pin requires function and symbol_pin"))
			}
		}
	}
	return issues
}

func validatePackages(path string, packages []PackageVariant) []reports.Issue {
	var issues []reports.Issue
	for i, pkg := range packages {
		packagePath := fmt.Sprintf("%s.packages[%d]", path, i)
		if strings.TrimSpace(pkg.ID) == "" {
			issues = append(issues, NewIssue(CodeInvalidComponentPackage, reports.SeverityBlocked, packagePath+".id", "package variant id is required"))
		}
		if strings.TrimSpace(pkg.FootprintID) == "" {
			issues = append(issues, NewIssue(CodeMissingFootprint, reports.SeverityBlocked, packagePath+".footprint_id", "package variant requires footprint_id"))
		}
		if issue, ok := ValidateConfidenceIssue(packagePath+".verification.confidence", pkg.Verification.Confidence); ok {
			issues = append(issues, issue)
		}
		for j, pad := range pkg.PadFunctions {
			padPath := fmt.Sprintf("%s.pad_functions[%d]", packagePath, j)
			if strings.TrimSpace(pad.Function) == "" || strings.TrimSpace(pad.Pad) == "" {
				issues = append(issues, NewIssue(CodeInvalidPadFunction, reports.SeverityBlocked, padPath, "pad function requires function and pad"))
			}
		}
	}
	return issues
}

type genericConstraint struct {
	kind string
	min  string
	typ  string
	max  string
	unit string
}

func valueConstraintsAsGeneric(values []ValueConstraint) []genericConstraint {
	out := make([]genericConstraint, len(values))
	for i, value := range values {
		out[i] = genericConstraint{kind: value.Kind, min: value.Min, typ: value.Typ, max: value.Max, unit: value.Unit}
	}
	return out
}

func ratingConstraintsAsGeneric(ratings []RatingConstraint) []genericConstraint {
	out := make([]genericConstraint, len(ratings))
	for i, rating := range ratings {
		out[i] = genericConstraint{kind: rating.Kind, min: rating.Min, typ: rating.Typ, max: rating.Max, unit: rating.Unit}
	}
	return out
}

func validateConstraints(path string, constraints []genericConstraint) []reports.Issue {
	var issues []reports.Issue
	for i, constraint := range constraints {
		constraintPath := fmt.Sprintf("%s[%d]", path, i)
		if strings.TrimSpace(constraint.kind) == "" {
			issues = append(issues, NewIssue(CodeInvalidConstraint, reports.SeverityBlocked, constraintPath+".kind", "constraint kind is required"))
		}
		if strings.TrimSpace(constraint.unit) == "" {
			issues = append(issues, NewIssue(CodeInvalidConstraint, reports.SeverityBlocked, constraintPath+".unit", "constraint unit is required"))
		}
		for _, value := range []struct {
			name string
			text string
		}{{"min", constraint.min}, {"typ", constraint.typ}, {"max", constraint.max}} {
			if strings.TrimSpace(value.text) == "" {
				continue
			}
			compact := strings.TrimSpace(value.text + constraint.unit)
			spaced := strings.TrimSpace(value.text + " " + constraint.unit)
			if _, ok := parseLeadingEngineeringNumber(compact); !ok {
				_, ok = parseLeadingEngineeringNumber(spaced)
				if !ok {
					issues = append(issues, NewIssue(CodeInvalidConstraint, reports.SeverityBlocked, constraintPath+"."+value.name, "constraint value cannot be parsed: "+value.text+" "+constraint.unit))
				}
			}
		}
	}
	return issues
}

func cleanCatalogDir(dir string) (string, error) {
	clean := filepath.Clean(dir)
	if filepath.IsAbs(clean) {
		return clean, nil
	}
	for _, part := range strings.Split(clean, string(os.PathSeparator)) {
		if part == ".." {
			return "", fmt.Errorf("component catalog directory must not contain parent traversal: %s", dir)
		}
	}
	return clean, nil
}
