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
	CodeInvalidLifecycle        reports.Code = "COMPONENT_INVALID_LIFECYCLE"
	CodeInvalidMetadata         reports.Code = "COMPONENT_INVALID_METADATA"
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
		issues = append(issues, validateSymbols(path, record.Symbols)...)
		issues = append(issues, validatePackages(path, record.Packages)...)
		issues = append(issues, validateConstraints(path+".values", valueConstraintsAsGeneric(record.Values))...)
		issues = append(issues, validateConstraints(path+".ratings", ratingConstraintsAsGeneric(record.Ratings))...)
		issues = append(issues, validateConstraints(path+".tolerances", toleranceConstraintsAsGeneric(record.Tolerances))...)
		issues = append(issues, validateTemperatureRange(path+".temperature", record.Temperature)...)
		issues = append(issues, validateCompanions(path+".companions", record.Companions)...)
		issues = append(issues, validateDeratingRules(path+".derating_rules", record.DeratingRules)...)
		issues = append(issues, validateRegulatorEvidence(path+".regulator_evidence", record.Regulator)...)
		issues = append(issues, validateCapacitorEvidence(path+".capacitor_evidence", record.Generic, record.Capacitor)...)
		issues = append(issues, validateAmplifierOutputEvidence(path+".amplifier_output_evidence", record)...)
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

func validateCompanions(path string, companions []CompanionRequirement) []reports.Issue {
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
	if generic && evidence.FabricationProof {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".fabrication_proof", "generic capacitor records cannot carry fabrication proof"))
	}
	return issues
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
		case "headphone_output", "bias", "small_signal_driver", "blocked_power_output":
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
