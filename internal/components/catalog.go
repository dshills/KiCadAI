package components

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	assets "kicadai"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
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
	CodeInvalidLifecycle        reports.Code = "COMPONENT_INVALID_LIFECYCLE"
	CodeInvalidMetadata         reports.Code = "COMPONENT_INVALID_METADATA"
	CodeInvalidComponentID      reports.Code = "COMPONENT_INVALID_ID"
	CodeInvalidComponentFamily  reports.Code = "COMPONENT_INVALID_FAMILY"
	CodeInvalidComponentPackage reports.Code = "COMPONENT_INVALID_PACKAGE"
	CodeInvalidSymbolUnit       reports.Code = "COMPONENT_INVALID_SYMBOL_UNIT"
)

var symbolUnitIDPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,62}$`)

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
		return loadEmbeddedCatalog(ctx)
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
	return loadCatalogFiles(ctx, dir, files, readCatalogFile)
}

func loadEmbeddedCatalog(ctx context.Context) (*Catalog, error) {
	entries, err := fs.ReadDir(assets.DefaultComponentCatalog, DefaultCatalogDir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}
		files = append(files, path.Join(DefaultCatalogDir, entry.Name()))
	}
	sort.Strings(files)
	return loadCatalogFiles(ctx, "embedded:"+DefaultCatalogDir, files, func(file string) (catalogFile, []reports.Issue) {
		body, err := fs.ReadFile(assets.DefaultComponentCatalog, file)
		if err != nil {
			return catalogFile{}, []reports.Issue{NewIssue(CodeCatalogReadFailed, reports.SeverityBlocked, file, err.Error())}
		}
		return parseCatalogFile(file, body)
	})
}

func loadCatalogFiles(ctx context.Context, source string, files []string, readFile func(string) (catalogFile, []reports.Issue)) (*Catalog, error) {
	now := time.Now().UTC()
	catalog := &Catalog{
		Version:     CatalogVersion,
		GeneratedAt: &now,
		Records:     []ComponentRecord{},
		Families:    []FamilyDefinition{},
		Diagnostics: []reports.Issue{},
	}
	if len(files) == 0 {
		catalog.Diagnostics = append(catalog.Diagnostics, NewIssue(CodeCatalogEmpty, reports.SeverityWarning, source, "component catalog directory contains no JSON files"))
		return catalog, nil
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		partial, issues := readFile(file)
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
	equivalenceGroups := map[string][]equivalenceMember{}
	for i := range catalog.Records {
		record := &catalog.Records[i]
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
		issues = append(issues, validateLifecycle(path+".lifecycle", record.Lifecycle)...)
		if len(record.Symbols) == 0 {
			issues = append(issues, NewIssue(CodeMissingSymbolBinding, reports.SeverityBlocked, path+".symbols", "component record has no symbol bindings"))
		}
		if len(record.Packages) == 0 {
			issues = append(issues, NewIssue(CodeMissingPackageVariant, reports.SeverityBlocked, path+".packages", "component record has no package variants"))
		}
		issues = append(issues, validateSymbols(path, record.Family, record.Symbols)...)
		issues = append(issues, validatePackages(path, record.Packages)...)
		issues = append(issues, validateConstraints(path+".values", valueConstraintsAsGeneric(record.Values))...)
		issues = append(issues, validateConstraints(path+".ratings", ratingConstraintsAsGeneric(record.Ratings))...)
		issues = append(issues, validateConstraints(path+".tolerances", toleranceConstraintsAsGeneric(record.Tolerances))...)
		issues = append(issues, validateTemperatureRange(path+".temperature", record.Temperature)...)
		issues = append(issues, validateCompanions(path+".companions", record, record.Companions)...)
		issues = append(issues, validateDeratingRules(path+".derating_rules", record.DeratingRules)...)
		issues = append(issues, validateRegulatorEvidence(path+".regulator_evidence", record.Regulator)...)
		issues = append(issues, validateCapacitorEvidence(path+".capacitor_evidence", record.Generic, record.Capacitor)...)
		issues = append(issues, validateOpAmpEvidence(path+".opamp_evidence", record.OpAmp)...)
		issues = append(issues, validateSensorEvidence(path+".sensor_evidence", record)...)
		issues = append(issues, validateAmplifierOutputEvidence(path+".amplifier_output_evidence", record)...)
		issues = append(issues, validatePowerSemiconductorEvidence(path+".power_semiconductor_evidence", record)...)
		for _, diagnostic := range simmodel.ValidateCatalogEvidence(record.Family, record.SimulationModels) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+diagnostic.Path, diagnostic.Message))
		}
		issues = append(issues, validatePlacementHints(path+".placement_hints", record.PlacementHints)...)
		issues = append(issues, validateRoutingHints(path+".routing_hints", record.RoutingHints)...)
		issues = append(issues, validateSchematicProperties(path+".properties", record.Properties)...)
		issues = append(issues, validateEquivalenceMetadata(path+".equivalence", record.Equivalence)...)
		if record.Equivalence != nil && strings.TrimSpace(record.Equivalence.Group) != "" {
			group := normalizeMetadata(record.Equivalence.Group)
			equivalenceGroups[group] = append(equivalenceGroups[group], equivalenceMember{
				path:      path,
				recordID:  record.ID,
				role:      record.Equivalence.Role,
				signature: equivalenceSignatureForRecord(*record),
			})
		}
	}
	issues = append(issues, validateEquivalenceGroups(equivalenceGroups)...)
	sortIssues(issues)
	return reports.ResultWithIssues("component validate", map[string]any{
		"family_count": len(catalog.Families),
		"record_count": len(catalog.Records),
	}, issues, nil)
}

type equivalenceMember struct {
	path      string
	recordID  string
	role      EquivalenceRole
	signature string
}

func readCatalogFile(path string) (catalogFile, []reports.Issue) {
	body, err := os.ReadFile(path)
	if err != nil {
		return catalogFile{}, []reports.Issue{NewIssue(CodeCatalogReadFailed, reports.SeverityBlocked, path, err.Error())}
	}
	return parseCatalogFile(path, body)
}

func parseCatalogFile(path string, body []byte) (catalogFile, []reports.Issue) {
	var file catalogFile
	if err := json.Unmarshal(body, &file); err != nil {
		return catalogFile{}, []reports.Issue{NewIssue(CodeCatalogParseFailed, reports.SeverityBlocked, path, err.Error())}
	}
	return file, nil
}

func validateSymbols(path, family string, symbols []SymbolBinding) []reports.Issue {
	var issues []reports.Issue
	namedCount := 0
	unitIDs := map[string]int{}
	unitNumbers := map[int]int{}
	symbolID := ""
	powerUnits := 0
	for i, symbol := range symbols {
		symbolPath := fmt.Sprintf("%s.symbols[%d]", path, i)
		if strings.TrimSpace(symbol.SymbolID) == "" {
			issues = append(issues, NewIssue(CodeMissingSymbolBinding, reports.SeverityBlocked, symbolPath+".symbol_id", "symbol binding requires symbol_id"))
		}
		if issue, ok := ValidateConfidenceIssue(symbolPath+".verification.confidence", symbol.Verification.Confidence); ok {
			issues = append(issues, issue)
		}
		if symbol.Unit < 0 {
			issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".unit", "symbol unit must not be negative"))
		}
		unitID := strings.ToUpper(strings.TrimSpace(symbol.UnitID))
		if unitID != "" {
			namedCount++
			if symbol.UnitID != unitID || !symbolUnitIDPattern.MatchString(unitID) {
				issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".unit_id", "named symbol unit id must be a canonical safe identifier"))
			}
			if first, exists := unitIDs[unitID]; exists {
				issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".unit_id", fmt.Sprintf("unit id duplicates symbols[%d]", first)))
			}
			unitIDs[unitID] = i
			if symbol.Unit <= 0 {
				issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".unit", "named symbol unit requires a positive KiCad unit number"))
			} else if first, exists := unitNumbers[symbol.Unit]; exists {
				issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".unit", fmt.Sprintf("KiCad unit number duplicates symbols[%d]", first)))
			} else {
				unitNumbers[symbol.Unit] = i
			}
			switch symbol.UnitType {
			case SymbolUnitFunctional:
			case SymbolUnitPower:
				powerUnits++
				if !symbol.RequiredUnit {
					issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".required_unit", "power symbol unit must be required"))
				}
			default:
				issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".unit_type", "named symbol unit requires functional or power type"))
			}
			if symbolID == "" {
				symbolID = symbol.SymbolID
			} else if symbol.SymbolID != symbolID {
				issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".symbol_id", "named units must share one KiCad symbol id"))
			}
		} else if symbol.UnitType != "" || symbol.RequiredUnit {
			issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, symbolPath+".unit_id", "unit type and required flag require a named unit"))
		}
		for j, pin := range symbol.FunctionPins {
			pinPath := fmt.Sprintf("%s.function_pins[%d]", symbolPath, j)
			if strings.TrimSpace(pin.Function) == "" || strings.TrimSpace(pin.SymbolPin) == "" {
				issues = append(issues, NewIssue(CodeInvalidFunctionPin, reports.SeverityBlocked, pinPath, "function pin requires function and symbol_pin"))
			}
		}
	}
	if namedCount != 0 && namedCount != len(symbols) {
		issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, path+".symbols", "catalog record must not mix named and anonymous symbol units"))
	}
	if namedCount > 1 && family == "opamp" && powerUnits != 1 {
		issues = append(issues, NewIssue(CodeInvalidSymbolUnit, reports.SeverityBlocked, path+".symbols", "named multi-unit op-amp requires exactly one power unit"))
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
		issues = append(issues, validateLifecycle(packagePath+".lifecycle", pkg.Lifecycle)...)
		if pkg.HeightMM < 0 {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, packagePath+".height_mm", "package height must not be negative"))
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

type evidenceTextField struct {
	pathSuffix string
	value      func(*AmplifierOutputEvidence) string
	label      string
}

var amplifierOutputRequiredFields = []evidenceTextField{
	{pathSuffix: "device_class", value: func(e *AmplifierOutputEvidence) string { return e.DeviceClass }, label: "amplifier output device class"},
	{pathSuffix: "polarity", value: func(e *AmplifierOutputEvidence) string { return e.Polarity }, label: "amplifier output polarity"},
	{pathSuffix: "package", value: func(e *AmplifierOutputEvidence) string { return e.Package }, label: "amplifier output package"},
	{pathSuffix: "symbol_id", value: func(e *AmplifierOutputEvidence) string { return e.SymbolID }, label: "amplifier output symbol ID"},
	{pathSuffix: "footprint_id", value: func(e *AmplifierOutputEvidence) string { return e.FootprintID }, label: "amplifier output footprint ID"},
	{pathSuffix: "pinmap_evidence", value: func(e *AmplifierOutputEvidence) string { return e.PinmapEvidence }, label: "amplifier output pinmap evidence"},
	{pathSuffix: "control_terminal", value: func(e *AmplifierOutputEvidence) string { return e.ControlTerminal }, label: "amplifier output control terminal"},
	{pathSuffix: "upper_or_lower_terminal", value: func(e *AmplifierOutputEvidence) string { return e.UpperOrLowerTerminal }, label: "amplifier output upper/lower terminal"},
	{pathSuffix: "output_terminal", value: func(e *AmplifierOutputEvidence) string { return e.OutputTerminal }, label: "amplifier output output terminal"},
}

var amplifierOutputStatusFields = []evidenceTextField{
	{pathSuffix: "voltage_rating_status", value: func(e *AmplifierOutputEvidence) string { return e.VoltageRatingStatus }, label: "voltage rating"},
	{pathSuffix: "current_rating_status", value: func(e *AmplifierOutputEvidence) string { return e.CurrentRatingStatus }, label: "current rating"},
	{pathSuffix: "power_dissipation_status", value: func(e *AmplifierOutputEvidence) string { return e.PowerDissipationStatus }, label: "power dissipation"},
	{pathSuffix: "thermal_review", value: func(e *AmplifierOutputEvidence) string { return e.ThermalReview }, label: "thermal review"},
	{pathSuffix: "safe_operating_area_status", value: func(e *AmplifierOutputEvidence) string { return e.SafeOperatingAreaStatus }, label: "safe operating area"},
}

type opAmpEvidenceTextField struct {
	pathSuffix string
	value      func(*OpAmpEvidence) string
	label      string
}

var opAmpStatusFields = []opAmpEvidenceTextField{
	{pathSuffix: "output_drive_status", value: func(e *OpAmpEvidence) string { return e.OutputDriveStatus }, label: "op-amp output-drive"},
	{pathSuffix: "load_compatibility_status", value: func(e *OpAmpEvidence) string { return e.LoadCompatibilityStatus }, label: "op-amp load compatibility"},
	{pathSuffix: "gain_bandwidth_status", value: func(e *OpAmpEvidence) string { return e.GainBandwidthStatus }, label: "op-amp gain-bandwidth"},
	{pathSuffix: "stability_status", value: func(e *OpAmpEvidence) string { return e.StabilityStatus }, label: "op-amp stability"},
	{pathSuffix: "input_common_mode_status", value: func(e *OpAmpEvidence) string { return e.InputCommonModeStatus }, label: "op-amp input common-mode"},
	{pathSuffix: "output_swing_status", value: func(e *OpAmpEvidence) string { return e.OutputSwingStatus }, label: "op-amp output-swing"},
	{pathSuffix: "noise_status", value: func(e *OpAmpEvidence) string { return e.NoiseStatus }, label: "op-amp noise"},
	{pathSuffix: "distortion_status", value: func(e *OpAmpEvidence) string { return e.DistortionStatus }, label: "op-amp distortion"},
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

func toleranceConstraintsAsGeneric(tolerances []ToleranceConstraint) []genericConstraint {
	out := make([]genericConstraint, len(tolerances))
	for i, tolerance := range tolerances {
		out[i] = genericConstraint{kind: tolerance.Kind, typ: tolerance.Typ, max: tolerance.Max, unit: tolerance.Unit}
	}
	return out
}

func validateTemperatureRange(path string, temperature *TemperatureRange) []reports.Issue {
	if temperature == nil {
		return nil
	}
	var issues []reports.Issue
	if strings.TrimSpace(temperature.Unit) == "" {
		issues = append(issues, NewIssue(CodeInvalidConstraint, reports.SeverityBlocked, path+".unit", "constraint unit is required"))
	}
	for _, value := range []struct {
		name string
		text string
	}{{"min", temperature.Min}, {"max", temperature.Max}} {
		if strings.TrimSpace(value.text) == "" {
			continue
		}
		compact := strings.TrimSpace(value.text + temperature.Unit)
		spaced := strings.TrimSpace(value.text + " " + temperature.Unit)
		if _, ok := parseLeadingEngineeringNumber(compact); !ok {
			_, ok = parseLeadingEngineeringNumber(spaced)
			if !ok {
				issues = append(issues, NewIssue(CodeInvalidConstraint, reports.SeverityBlocked, path+"."+value.name, "constraint value cannot be parsed: "+value.text+" "+temperature.Unit))
			}
		}
	}
	return issues
}

func validateLifecycle(path string, lifecycle string) []reports.Issue {
	trimmed := strings.TrimSpace(lifecycle)
	if trimmed == "" {
		return nil
	}
	if lifecycle != trimmed {
		return []reports.Issue{NewIssue(CodeInvalidLifecycle, reports.SeverityBlocked, path, "lifecycle value must not have leading or trailing whitespace: "+lifecycle)}
	}
	switch lifecycle {
	case "active", "preferred", "mature", "nrnd", "obsolete", "unknown":
		return nil
	default:
		return []reports.Issue{NewIssue(CodeInvalidLifecycle, reports.SeverityBlocked, path, "invalid lifecycle value: "+lifecycle)}
	}
}

func validateCompanions(path string, record *ComponentRecord, companions []CompanionRequirement) []reports.Issue {
	var issues []reports.Issue
	type companionKey struct {
		ID   string
		Role string
	}
	seen := map[companionKey]int{}
	for i, companion := range companions {
		companionPath := fmt.Sprintf("%s[%d]", path, i)
		valid := true
		if issue, ok := validateTrimmedMetadata(companionPath+".id", companion.ID, "companion requirement id"); ok {
			issues = append(issues, issue)
			valid = false
		} else if companion.ID == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, companionPath+".id", "companion requirement id is required"))
			valid = false
		}
		if issue, ok := validateTrimmedMetadata(companionPath+".role", companion.Role, "companion requirement role"); ok {
			issues = append(issues, issue)
			valid = false
		} else if companion.Role == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, companionPath+".role", "companion requirement role is required"))
			valid = false
		}
		if !valid {
			continue
		}
		key := companionKey{ID: companion.ID, Role: companion.Role}
		if first, ok := seen[key]; ok {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, companionPath, fmt.Sprintf("duplicate companion requirement duplicates %s[%d]", path, first)))
		} else {
			seen[key] = i
		}
		recipeIDs := map[string]int{}
		for recipeIndex, recipe := range companion.Recipes {
			recipePath := fmt.Sprintf("%s.recipes[%d]", companionPath, recipeIndex)
			if strings.TrimSpace(recipe.ID) == "" || strings.TrimSpace(recipe.Family) == "" || strings.TrimSpace(string(recipe.Role)) == "" {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath, "companion recipe id, family, and component role are required"))
			}
			if previous, duplicate := recipeIDs[recipe.ID]; duplicate {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".id", fmt.Sprintf("duplicate companion recipe duplicates %s.recipes[%d]", companionPath, previous)))
			}
			recipeIDs[recipe.ID] = recipeIndex
			if recipe.MinimumConfidence != "" && !ValidConfidence(recipe.MinimumConfidence) {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".minimum_confidence", "invalid companion recipe confidence"))
			}
			if recipe.MinVoltageV < 0 {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".min_voltage_v", "companion recipe minimum voltage cannot be negative"))
			}
			if recipe.Value != "" && recipe.ValueFormula != nil {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".value_formula", "companion recipe value and value formula are mutually exclusive"))
			}
			if formula := recipe.ValueFormula; formula != nil {
				if formula.Kind != "divider_upper_from_output_v1" {
					issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".value_formula.kind", "unsupported companion value formula kind"))
				}
				if strings.TrimSpace(formula.Parameter) == "" || formula.ReferenceVoltageV <= 0 || formula.LowerResistanceOhm <= 0 {
					issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".value_formula", "divider formula requires a parameter and positive reference voltage and lower resistance"))
				}
				if formula.PreferredSeries != "E96" {
					issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".value_formula.preferred_series", "divider formula requires the deterministic E96 preferred series"))
				}
			}
			if len(recipe.Connections) < 2 {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, recipePath+".connections", "companion recipe requires at least two semantic connections"))
			}
			connectedFunctions := map[string]bool{}
			for connectionIndex, connection := range recipe.Connections {
				connectionPath := fmt.Sprintf("%s.connections[%d]", recipePath, connectionIndex)
				if strings.TrimSpace(connection.Function) == "" || strings.TrimSpace(connection.ParentFunction) == "" {
					issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, connectionPath, "companion and parent semantic functions are required"))
				}
				if connectedFunctions[connection.Function] {
					issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, connectionPath+".function", "companion semantic function is connected more than once"))
				}
				connectedFunctions[connection.Function] = true
			}
		}
		for tieIndex, tie := range companion.Ties {
			tiePath := fmt.Sprintf("%s.ties[%d]", companionPath, tieIndex)
			if strings.TrimSpace(tie.Function) == "" || (tie.Level != "high" && tie.Level != "low") {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, tiePath, "companion tie requires a semantic function and high or low level"))
			}
			if tie.ParentFunction != "" && !recordHasFunction(*record, tie.ParentFunction) {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, tiePath+".parent_function", "companion tie parent function is not present in the symbol bindings"))
			}
		}
		for noConnectIndex, function := range companion.NoConnects {
			if strings.TrimSpace(function) == "" {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, fmt.Sprintf("%s.no_connects[%d]", companionPath, noConnectIndex), "companion no-connect function is required"))
			}
		}
	}
	return issues
}

func validateDeratingRules(path string, rules []DeratingRule) []reports.Issue {
	var issues []reports.Issue
	for i, rule := range rules {
		rulePath := fmt.Sprintf("%s[%d]", path, i)
		if issue, ok := validateTrimmedMetadata(rulePath+".kind", rule.Kind, "derating rule kind"); ok {
			issues = append(issues, issue)
		} else if rule.Kind == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, rulePath+".kind", "derating rule kind is required"))
		}
		if strings.TrimSpace(rule.Expression) == "" && strings.TrimSpace(rule.Description) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, rulePath, "derating rule requires expression or description"))
		}
	}
	return issues
}

func validateRegulatorEvidence(path string, evidence *RegulatorEvidence) []reports.Issue {
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	issues = append(issues, validateReviewStatus(path+".thermal_review", evidence.ThermalReview, "thermal review")...)
	if evidence.OutputCapacitor != nil {
		issues = append(issues, validateRegulatorCapacitorStability(path+".output_capacitor", *evidence.OutputCapacitor)...)
	}
	return issues
}

func validateRegulatorCapacitorStability(path string, stability RegulatorCapacitorStability) []reports.Issue {
	var issues []reports.Issue
	switch stability.Kind {
	case "ceramic_stable", "esr_window_required", "datasheet_specific", "unknown":
	case "":
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".kind", "regulator capacitor stability kind is required"))
	default:
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".kind", "invalid regulator capacitor stability kind: "+stability.Kind))
	}
	if strings.TrimSpace(stability.ProofStatus) != "" {
		switch stability.ProofStatus {
		case "proven", "review_required", "blocked", "unknown":
		default:
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".proof_status", "invalid regulator capacitor stability proof status: "+stability.ProofStatus))
		}
	}
	if stability.Kind != "" && strings.TrimSpace(stability.MinCapacitance) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".min_capacitance", "regulator capacitor stability requires minimum capacitance"))
	}
	issues = append(issues, validateEvidenceEngineeringValue(path+".min_capacitance", stability.MinCapacitance, path+".capacitance_unit", stability.CapacitanceUnit, "minimum capacitance")...)
	issues = append(issues, validateEvidenceEngineeringValue(path+".max_capacitance", stability.MaxCapacitance, path+".capacitance_unit", stability.CapacitanceUnit, "maximum capacitance")...)
	issues = append(issues, validateEvidenceEngineeringValue(path+".esr_min", stability.ESRMin, path+".esr_unit", stability.ESRUnit, "minimum ESR")...)
	issues = append(issues, validateEvidenceEngineeringValue(path+".esr_max", stability.ESRMax, path+".esr_unit", stability.ESRUnit, "maximum ESR")...)
	if strings.TrimSpace(stability.MinCapacitance) != "" && strings.TrimSpace(stability.MaxCapacitance) != "" {
		min, minOK := parseEvidenceEngineeringValue(stability.MinCapacitance, stability.CapacitanceUnit)
		max, maxOK := parseEvidenceEngineeringValue(stability.MaxCapacitance, stability.CapacitanceUnit)
		if minOK && maxOK && min > max {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".min_capacitance", "minimum capacitance must not exceed maximum capacitance"))
		}
	}
	if strings.TrimSpace(stability.ESRMin) != "" && strings.TrimSpace(stability.ESRMax) != "" {
		min, minOK := parseEvidenceEngineeringValue(stability.ESRMin, stability.ESRUnit)
		max, maxOK := parseEvidenceEngineeringValue(stability.ESRMax, stability.ESRUnit)
		if minOK && maxOK && min > max {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".esr_min", "minimum ESR must not exceed maximum ESR"))
		}
	}
	for i, dielectric := range stability.AcceptedDielectrics {
		if issue, ok := validateTrimmedMetadata(fmt.Sprintf("%s.accepted_dielectrics[%d]", path, i), dielectric, "accepted dielectric"); ok {
			issues = append(issues, issue)
		} else if dielectric == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, fmt.Sprintf("%s.accepted_dielectrics[%d]", path, i), "accepted dielectric is required"))
		}
	}
	return issues
}

func validateCapacitorEvidence(path string, generic bool, evidence *CapacitorEvidence) []reports.Issue {
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	if issue, ok := validateTrimmedMetadata(path+".dielectric", evidence.Dielectric, "capacitor dielectric"); ok {
		issues = append(issues, issue)
	}
	issues = append(issues, validateEvidenceEngineeringValue(path+".nominal_capacitance", evidence.NominalCapacitance, path+".capacitance_unit", evidence.CapacitanceUnit, "nominal capacitance")...)
	issues = append(issues, validateEvidenceEngineeringValue(path+".voltage_rating", evidence.VoltageRating, path+".voltage_unit", evidence.VoltageUnit, "voltage rating")...)
	for _, status := range []struct {
		path  string
		value string
		label string
	}{
		{path: path + ".dc_bias_review", value: evidence.DCBiasReview, label: "DC-bias review"},
		{path: path + ".effective_capacitance_review", value: evidence.EffectiveCapacitanceReview, label: "effective-capacitance review"},
		{path: path + ".esr_review", value: evidence.ESRReview, label: "ESR review"},
	} {
		issues = append(issues, validateReviewStatus(status.path, status.value, status.label)...)
	}
	if evidence.Polarity != "" {
		switch evidence.Polarity {
		case "polarized", "nonpolar", "bipolar":
		default:
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".polarity", "invalid capacitor polarity: "+evidence.Polarity))
		}
	}
	if evidence.CapacitanceTolerancePct != nil && (*evidence.CapacitanceTolerancePct <= 0 || *evidence.CapacitanceTolerancePct > 100) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".capacitance_tolerance_percent", "capacitance tolerance must be greater than zero and at most 100 percent"))
	}
	issues = append(issues, validateEvidenceMeasurement(path+".esr", evidence.ESR, evidence.FabricationProof)...)
	issues = append(issues, validateEvidenceMeasurement(path+".ripple_current", evidence.RippleCurrent, evidence.FabricationProof)...)
	for suffix, number := range map[string]*float64{
		"endurance_hours":         evidence.EnduranceHours,
		"endurance_temperature_c": evidence.EnduranceTemperatureC,
	} {
		if number != nil && *number <= 0 {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "capacitor endurance evidence must be positive"))
		}
	}
	if generic && evidence.FabricationProof {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".fabrication_proof", "generic capacitor records cannot carry fabrication proof"))
	}
	if evidence.FabricationProof {
		requiredText := map[string]string{
			"technology": evidence.Technology,
			"polarity":   evidence.Polarity,
		}
		for suffix, value := range requiredText {
			if strings.TrimSpace(value) == "" {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-proof capacitor evidence requires "+strings.ReplaceAll(suffix, "_", " ")))
			}
		}
		for _, required := range []struct {
			suffix  string
			missing bool
		}{
			{suffix: "capacitance_tolerance_percent", missing: evidence.CapacitanceTolerancePct == nil},
			{suffix: "esr", missing: evidence.ESR == nil},
			{suffix: "ripple_current", missing: evidence.RippleCurrent == nil},
			{suffix: "endurance_hours", missing: evidence.EnduranceHours == nil},
			{suffix: "endurance_temperature_c", missing: evidence.EnduranceTemperatureC == nil},
		} {
			if required.missing {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+required.suffix, "fabrication-proof capacitor evidence requires "+strings.ReplaceAll(required.suffix, "_", " ")))
			}
		}
	}
	return issues
}

func validateOpAmpEvidence(path string, evidence *OpAmpEvidence) []reports.Issue {
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	switch evidence.SupplyMode {
	case "single_supply", "dual_supply", "rail_to_rail_single_supply":
	case "":
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".supply_mode", "op-amp supply mode is required"))
	default:
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".supply_mode", "invalid op-amp supply mode: "+evidence.SupplyMode))
	}
	for i, role := range evidence.IntendedRoles {
		rolePath := fmt.Sprintf("%s.intended_roles[%d]", path, i)
		if issue, ok := validateTrimmedMetadata(rolePath, role, "op-amp intended role"); ok {
			issues = append(issues, issue)
			continue
		}
		switch role {
		case "input_buffer", "gain_stage", "voltage_follower", "comparator", "headphone_driver", "small_signal_driver":
		default:
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, rolePath, "invalid op-amp intended role: "+role))
		}
	}
	if len(evidence.IntendedRoles) == 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".intended_roles", "op-amp evidence requires at least one intended role"))
	}
	for _, status := range opAmpStatusFields {
		fieldPath := path + "." + status.pathSuffix
		value := status.value(evidence)
		if strings.TrimSpace(value) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, fieldPath, status.label+" status is required"))
			continue
		}
		issues = append(issues, validateReviewStatus(fieldPath, value, status.label)...)
	}
	require := evidence.FabricationProof
	if evidence.FabricationProof && evidence.FabricationCandidateBlocks {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".fabrication_proof", "op-amp fabrication proof cannot coexist with a fabrication-candidate blocker"))
	}
	issues = append(issues, validateEvidenceRange(path+".supply_voltage", evidence.SupplyVoltage, require)...)
	issues = append(issues, validateRailHeadroom(path+".input_common_mode", evidence.InputCommonMode, require)...)
	issues = append(issues, validateRailHeadroom(path+".output_swing", evidence.OutputSwing, require)...)
	for suffix, measurement := range map[string]*EvidenceMeasurement{
		"output_current":        evidence.OutputCurrent,
		"gain_bandwidth":        evidence.GainBandwidth,
		"slew_rate":             evidence.SlewRate,
		"voltage_noise_density": evidence.VoltageNoiseDensity,
	} {
		issues = append(issues, validateEvidenceMeasurement(path+"."+suffix, measurement, require)...)
		if require && measurement == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented op-amp evidence requires this measurement"))
		}
	}
	for suffix, number := range map[string]*float64{
		"max_junction_temperature_c":  evidence.MaxJunctionTemperatureC,
		"junction_to_ambient_c_per_w": evidence.JunctionToAmbientCPerW,
	} {
		if number != nil && *number <= 0 {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "op-amp thermal evidence must be positive"))
		}
		if require && number == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+"."+suffix, "fabrication-oriented op-amp evidence requires this thermal limit"))
		}
	}
	if require {
		if evidence.SupplyVoltage == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".supply_voltage", "fabrication-oriented op-amp evidence requires a supply range"))
		}
		if evidence.InputCommonMode == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".input_common_mode", "fabrication-oriented op-amp evidence requires common-mode limits"))
		}
		if evidence.OutputSwing == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".output_swing", "fabrication-oriented op-amp evidence requires output-swing limits"))
		}
	}
	return issues
}

func validateSensorEvidence(path string, record *ComponentRecord) []reports.Issue {
	evidence := record.Sensor
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	if record.Family != "sensor" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "sensor evidence is only valid for sensor-family records"))
	}
	interfaces := make(map[string]bool, len(evidence.Interfaces))
	for i, raw := range evidence.Interfaces {
		interfacePath := fmt.Sprintf("%s.interfaces[%d]", path, i)
		value := strings.ToLower(strings.TrimSpace(raw))
		switch value {
		case "i2c", "spi":
		default:
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, interfacePath, "unsupported sensor interface: "+raw))
			continue
		}
		if interfaces[value] {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, interfacePath, "duplicate sensor interface: "+value))
		}
		interfaces[value] = true
	}
	if len(interfaces) == 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".interfaces", "sensor evidence requires at least one interface"))
	}
	if interfaces["i2c"] && len(evidence.I2CAddresses) == 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".i2c_addresses", "I2C sensor evidence requires at least one address"))
	}
	seenAddresses := make(map[string]bool, len(evidence.I2CAddresses))
	defaultCount := 0
	for i, option := range evidence.I2CAddresses {
		optionPath := fmt.Sprintf("%s.i2c_addresses[%d]", path, i)
		address := strings.ToLower(strings.TrimSpace(option.Address))
		if !validSensorI2CAddress(address) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, optionPath+".address", "sensor I2C address must be an unreserved 7-bit hexadecimal address"))
		} else if seenAddresses[address] {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, optionPath+".address", "duplicate sensor I2C address: "+address))
		}
		seenAddresses[address] = true
		if option.Default {
			defaultCount++
		}
		if len(evidence.I2CAddresses) > 1 || option.SelectFunction != "" || option.Level != "" {
			issues = append(issues, validateSensorFunctionLevel(record, optionPath, option.SelectFunction, option.Level, option.ParentFunction)...)
		}
	}
	if len(evidence.I2CAddresses) > 0 && defaultCount != 1 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".i2c_addresses", "sensor I2C evidence requires exactly one default address"))
	}
	for i, connection := range evidence.I2CModeConnections {
		connectionPath := fmt.Sprintf("%s.i2c_mode_connections[%d]", path, i)
		issues = append(issues, validateSensorFunctionLevel(record, connectionPath, connection.Function, connection.Level, connection.ParentFunction)...)
	}
	if evidence.OptionalInterruptFunction != "" && !recordHasFunction(*record, evidence.OptionalInterruptFunction) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".optional_interrupt_function", "sensor interrupt function is not present in the symbol bindings"))
	}
	for i, policy := range evidence.UnusedPinPolicies {
		policyPath := fmt.Sprintf("%s.unused_pin_policies[%d]", path, i)
		if !recordHasFunction(*record, policy.Function) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, policyPath+".function", "sensor unused-pin function is not present in the symbol bindings"))
		}
		switch policy.Policy {
		case "no_connect", "high", "low":
		default:
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, policyPath+".policy", "invalid sensor unused-pin policy: "+policy.Policy))
		}
	}
	return issues
}

func validateSensorFunctionLevel(record *ComponentRecord, path string, function string, level string, parentFunction string) []reports.Issue {
	var issues []reports.Issue
	if !recordHasFunction(*record, function) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".function", "sensor function is not present in the symbol bindings"))
	}
	switch level {
	case "high", "low":
	default:
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".level", "sensor pin level must be high or low"))
	}
	if parentFunction != "" && !recordHasFunction(*record, parentFunction) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".parent_function", "sensor parent function is not present in the symbol bindings"))
	}
	return issues
}

func validSensorI2CAddress(value string) bool {
	if len(value) != 4 || !strings.HasPrefix(value, "0x") {
		return false
	}
	address, err := strconv.ParseUint(value[2:], 16, 8)
	return err == nil && address >= 0x08 && address <= 0x77
}

func validateAmplifierOutputEvidence(path string, record *ComponentRecord) []reports.Issue {
	evidence := record.AmplifierOutput
	if evidence == nil {
		return nil
	}
	var issues []reports.Issue
	for _, required := range amplifierOutputRequiredFields {
		fieldPath := path + "." + required.pathSuffix
		value := required.value(evidence)
		if issue, ok := validateTrimmedMetadata(fieldPath, value, required.label); ok {
			issues = append(issues, issue)
		} else if value == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, fieldPath, required.label+" is required"))
		}
	}
	switch evidence.DeviceClass {
	case "bjt", "mosfet":
	default:
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".device_class", "invalid amplifier output device class: "+evidence.DeviceClass))
	}
	switch evidence.Polarity {
	case "npn", "pnp", "n_channel", "p_channel":
	default:
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".polarity", "invalid amplifier output polarity: "+evidence.Polarity))
	}
	if evidence.SymbolID != "" && !recordHasSymbol(record, evidence.SymbolID) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".symbol_id", "amplifier output symbol_id must match a component symbol binding"))
	}
	if evidence.FootprintID != "" && !recordHasFootprint(record, evidence.FootprintID) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".footprint_id", "amplifier output footprint_id must match a component package variant"))
	}
	if issue, ok := validateTrimmedMetadata(path+".complementary_group", evidence.ComplementaryGroup, "amplifier output complementary group"); ok {
		issues = append(issues, issue)
	}
	if evidence.PinmapEvidence != "" && !strings.HasPrefix(strings.ToLower(evidence.PinmapEvidence), "blocked:") && !recordHasPinmapEvidence(record, evidence.SymbolID, evidence.FootprintID, evidence.PinmapEvidence) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".pinmap_evidence", "amplifier output pinmap_evidence must match the referenced symbol or package verification sources"))
	}
	for i, role := range evidence.IntendedRoles {
		rolePath := fmt.Sprintf("%s.intended_roles[%d]", path, i)
		if issue, ok := validateTrimmedMetadata(rolePath, role, "amplifier output intended role"); ok {
			issues = append(issues, issue)
			continue
		}
		switch role {
		case "headphone_output", "power_output", "class_a_output", "bias", "small_signal_driver", "blocked_power_output":
		case "":
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, rolePath, "amplifier output intended role is required"))
		default:
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, rolePath, "invalid amplifier output intended role: "+role))
		}
	}
	if len(evidence.IntendedRoles) == 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".intended_roles", "amplifier output evidence requires at least one intended role"))
	}
	for _, status := range amplifierOutputStatusFields {
		fieldPath := path + "." + status.pathSuffix
		value := status.value(evidence)
		if strings.TrimSpace(value) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, fieldPath, "amplifier output "+status.label+" status is required"))
			continue
		}
		issues = append(issues, validateReviewStatus(fieldPath, value, "amplifier output "+status.label)...)
	}
	return issues
}

func recordHasSymbol(record *ComponentRecord, symbolID string) bool {
	for _, symbol := range record.Symbols {
		if symbol.SymbolID == symbolID {
			return true
		}
	}
	return false
}

func recordHasFootprint(record *ComponentRecord, footprintID string) bool {
	for _, pkg := range record.Packages {
		if pkg.FootprintID == footprintID {
			return true
		}
	}
	return false
}

func recordHasPinmapEvidence(record *ComponentRecord, symbolID string, footprintID string, source string) bool {
	symbolHasSource := false
	for _, symbol := range record.Symbols {
		if symbol.SymbolID == symbolID && verificationHasSource(symbol.Verification, source) {
			symbolHasSource = true
			break
		}
	}
	packageHasSource := false
	for _, pkg := range record.Packages {
		if pkg.FootprintID == footprintID && verificationHasSource(pkg.Verification, source) {
			packageHasSource = true
			break
		}
	}
	return symbolHasSource && packageHasSource
}

func verificationHasSource(verification VerificationRecord, source string) bool {
	for _, candidate := range verification.Sources {
		if strings.EqualFold(candidate, source) {
			return true
		}
	}
	return false
}

func validateEvidenceEngineeringValue(path string, value string, unitPath string, unit string, label string) []reports.Issue {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if strings.TrimSpace(unit) == "" {
		return []reports.Issue{NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, unitPath, label+" unit is required")}
	}
	if _, ok := parseEvidenceEngineeringValue(value, unit); !ok {
		return []reports.Issue{NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, label+" cannot be parsed: "+value+" "+unit)}
	}
	return nil
}

func validateReviewStatus(path string, value string, label string) []reports.Issue {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	switch value {
	case "proven", "review_required", "blocked", "unknown", "not_applicable":
		return nil
	default:
		return []reports.Issue{NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "invalid "+label+" status: "+value)}
	}
}

func parseEvidenceEngineeringValue(value string, unit string) (float64, bool) {
	compact := strings.TrimSpace(value + unit)
	if number, ok := parseLeadingEngineeringNumber(compact); ok {
		return number, true
	}
	spaced := strings.TrimSpace(value + " " + unit)
	return parseLeadingEngineeringNumber(spaced)
}

func validatePlacementHints(path string, hints []PlacementHint) []reports.Issue {
	var issues []reports.Issue
	for i, hint := range hints {
		hintPath := fmt.Sprintf("%s[%d]", path, i)
		if issue, ok := validateTrimmedMetadata(hintPath+".kind", hint.Kind, "placement hint kind"); ok {
			issues = append(issues, issue)
		} else if hint.Kind == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, hintPath+".kind", "placement hint kind is required"))
		}
		if strings.TrimSpace(hint.Value) != "" && strings.TrimSpace(hint.Unit) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, hintPath+".unit", "placement hint with value requires unit"))
		}
	}
	return issues
}

func validateRoutingHints(path string, hints []RoutingHint) []reports.Issue {
	var issues []reports.Issue
	for i, hint := range hints {
		hintPath := fmt.Sprintf("%s[%d]", path, i)
		if issue, ok := validateTrimmedMetadata(hintPath+".kind", hint.Kind, "routing hint kind"); ok {
			issues = append(issues, issue)
		} else if hint.Kind == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, hintPath+".kind", "routing hint kind is required"))
		}
		if strings.TrimSpace(hint.Value) != "" && strings.TrimSpace(hint.Unit) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, hintPath+".unit", "routing hint with value requires unit"))
		}
	}
	return issues
}

func validateSchematicProperties(path string, properties []SchematicProperty) []reports.Issue {
	var issues []reports.Issue
	seen := map[string]int{}
	for i, property := range properties {
		propertyPath := fmt.Sprintf("%s[%d]", path, i)
		if issue, ok := validateTrimmedMetadata(propertyPath+".name", property.Name, "schematic property name"); ok {
			issues = append(issues, issue)
			continue
		}
		name := property.Name
		if name == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, propertyPath+".name", "schematic property name is required"))
			continue
		}
		if first, ok := seen[name]; ok {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, propertyPath+".name", fmt.Sprintf("duplicate schematic property duplicates %s[%d]", path, first)))
		} else {
			seen[name] = i
		}
	}
	return issues
}

func validateEquivalenceMetadata(path string, equivalence *EquivalenceMetadata) []reports.Issue {
	if equivalence == nil {
		return nil
	}
	var issues []reports.Issue
	if issue, ok := validateTrimmedMetadata(path+".group", equivalence.Group, "equivalence group"); ok {
		issues = append(issues, issue)
	}
	if issue, ok := validateTrimmedMetadata(path+".role", string(equivalence.Role), "equivalence role"); ok {
		issues = append(issues, issue)
	}
	if strings.TrimSpace(equivalence.Group) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".group", "equivalence group is required when equivalence metadata is present"))
	}
	if !isValidEquivalenceRole(equivalence.Role) {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".role", "equivalence role must be preferred, alternate, or fallback"))
	}
	for i, note := range equivalence.Notes {
		notePath := fmt.Sprintf("%s.notes[%d]", path, i)
		if issue, ok := validateTrimmedMetadata(notePath, note, "equivalence note"); ok {
			issues = append(issues, issue)
		}
	}
	return issues
}

func validateEquivalenceGroups(groups map[string][]equivalenceMember) []reports.Issue {
	var issues []reports.Issue
	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)
	for _, group := range groupNames {
		members := groups[group]
		sort.Slice(members, func(i, j int) bool {
			if members[i].path != members[j].path {
				return members[i].path < members[j].path
			}
			return members[i].recordID < members[j].recordID
		})
		preferredPath := ""
		preferredSignature := ""
		for _, member := range members {
			if member.role == EquivalencePreferred {
				if preferredPath != "" {
					issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, member.path+".equivalence.role", "equivalence group "+group+" has multiple preferred records"))
					continue
				}
				preferredPath = member.path
				preferredSignature = member.signature
			}
		}
		if preferredPath == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, members[0].path+".equivalence.role", "equivalence group "+group+" requires one preferred record"))
			preferredSignature = members[0].signature
		}
		for _, member := range members {
			if member.path == preferredPath {
				continue
			}
			if member.signature != preferredSignature {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, member.path+".equivalence.group", "equivalence group "+group+" contains incompatible family, package, or value metadata"))
			}
		}
	}
	return issues
}

func isValidEquivalenceRole(role EquivalenceRole) bool {
	switch role {
	case EquivalencePreferred, EquivalenceAlternate, EquivalenceFallback:
		return true
	default:
		return false
	}
}

func equivalenceSignatureForRecord(record ComponentRecord) string {
	type equivalencePadSignature struct {
		Function string `json:"function"`
		Pad      string `json:"pad"`
		Polarity string `json:"polarity,omitempty"`
		Aliases  string `json:"aliases,omitempty"`
	}
	type equivalencePackageSignature struct {
		ID          string                    `json:"id"`
		PackageType string                    `json:"package_type"`
		FootprintID string                    `json:"footprint_id"`
		PadMap      []equivalencePadSignature `json:"pad_map"`
	}
	var packages []equivalencePackageSignature
	for _, pkg := range record.Packages {
		padMap := make([]equivalencePadSignature, 0, len(pkg.PadFunctions))
		for _, pad := range pkg.PadFunctions {
			aliases := append([]string(nil), pad.Aliases...)
			for i := range aliases {
				aliases[i] = normalizeMetadata(aliases[i])
			}
			sort.Strings(aliases)
			padMap = append(padMap, equivalencePadSignature{
				Function: normalizeMetadata(pad.Function),
				Pad:      normalizeMetadata(pad.Pad),
				Polarity: normalizeMetadata(pad.Polarity),
				Aliases:  strings.Join(aliases, "\x00"),
			})
		}
		sort.Slice(padMap, func(i, j int) bool {
			if padMap[i].Function != padMap[j].Function {
				return padMap[i].Function < padMap[j].Function
			}
			if padMap[i].Pad != padMap[j].Pad {
				return padMap[i].Pad < padMap[j].Pad
			}
			if padMap[i].Polarity != padMap[j].Polarity {
				return padMap[i].Polarity < padMap[j].Polarity
			}
			return padMap[i].Aliases < padMap[j].Aliases
		})
		packages = append(packages, equivalencePackageSignature{
			ID:          normalizeMetadata(pkg.ID),
			PackageType: normalizeMetadata(pkg.PackageType),
			FootprintID: normalizeMetadata(pkg.FootprintID),
			PadMap:      padMap,
		})
	}
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].ID != packages[j].ID {
			return packages[i].ID < packages[j].ID
		}
		if packages[i].PackageType != packages[j].PackageType {
			return packages[i].PackageType < packages[j].PackageType
		}
		return packages[i].FootprintID < packages[j].FootprintID
	})
	type equivalenceValueSignature struct {
		Kind string `json:"kind"`
		Typ  string `json:"typ"`
		Min  string `json:"min"`
		Max  string `json:"max"`
		Unit string `json:"unit"`
	}
	var values []equivalenceValueSignature
	for _, value := range record.Values {
		values = append(values, equivalenceValueSignature{
			Kind: normalizeMetadata(value.Kind),
			Typ:  normalizeMetadata(value.Typ),
			Min:  normalizeMetadata(value.Min),
			Max:  normalizeMetadata(value.Max),
			Unit: normalizeMetadata(value.Unit),
		})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].Typ != values[j].Typ {
			return values[i].Typ < values[j].Typ
		}
		if values[i].Min != values[j].Min {
			return values[i].Min < values[j].Min
		}
		if values[i].Max != values[j].Max {
			return values[i].Max < values[j].Max
		}
		return values[i].Unit < values[j].Unit
	})
	type equivalenceRatingSignature struct {
		Kind string `json:"kind"`
		Typ  string `json:"typ"`
		Min  string `json:"min"`
		Max  string `json:"max"`
		Unit string `json:"unit"`
	}
	var ratings []equivalenceRatingSignature
	for _, rating := range record.Ratings {
		ratings = append(ratings, equivalenceRatingSignature{
			Kind: normalizeMetadata(rating.Kind),
			Typ:  normalizeMetadata(rating.Typ),
			Min:  normalizeMetadata(rating.Min),
			Max:  normalizeMetadata(rating.Max),
			Unit: normalizeMetadata(rating.Unit),
		})
	}
	sort.Slice(ratings, func(i, j int) bool {
		if ratings[i].Kind != ratings[j].Kind {
			return ratings[i].Kind < ratings[j].Kind
		}
		if ratings[i].Typ != ratings[j].Typ {
			return ratings[i].Typ < ratings[j].Typ
		}
		if ratings[i].Min != ratings[j].Min {
			return ratings[i].Min < ratings[j].Min
		}
		if ratings[i].Max != ratings[j].Max {
			return ratings[i].Max < ratings[j].Max
		}
		return ratings[i].Unit < ratings[j].Unit
	})
	signature := struct {
		Family   string                        `json:"family"`
		Packages []equivalencePackageSignature `json:"packages"`
		Values   []equivalenceValueSignature   `json:"values"`
		Ratings  []equivalenceRatingSignature  `json:"ratings"`
	}{
		Family:   normalizeMetadata(record.Family),
		Packages: packages,
		Values:   values,
		Ratings:  ratings,
	}
	body, err := json.Marshal(signature)
	if err != nil {
		return ""
	}
	return string(body)
}

func normalizeMetadata(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validateTrimmedMetadata(path string, value string, label string) (reports.Issue, bool) {
	if value != strings.TrimSpace(value) {
		return NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, label+" must not have leading or trailing whitespace"), true
	}
	return reports.Issue{}, false
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
