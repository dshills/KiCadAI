package components

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kicadai/internal/reports"
)

const CatalogVersion = "0.1.0"
const maxEngineeringValueLength = 128

type ConfidenceLevel string

const (
	ConfidenceVerified       ConfidenceLevel = "verified"
	ConfidenceLibraryDerived ConfidenceLevel = "library_derived"
	ConfidenceRuleInferred   ConfidenceLevel = "rule_inferred"
	ConfidencePlaceholder    ConfidenceLevel = "placeholder"
	ConfidenceBlocked        ConfidenceLevel = "blocked"
)

type AcceptanceLevel string

const (
	AcceptanceDraft                AcceptanceLevel = "draft"
	AcceptanceStructural           AcceptanceLevel = "structural"
	AcceptanceConnectivity         AcceptanceLevel = "connectivity"
	AcceptanceERCDRC               AcceptanceLevel = "erc_drc"
	AcceptanceFabricationCandidate AcceptanceLevel = "fabrication_candidate"
)

type EquivalenceRole string

const (
	EquivalencePreferred EquivalenceRole = "preferred"
	EquivalenceAlternate EquivalenceRole = "alternate"
	EquivalenceFallback  EquivalenceRole = "fallback"
)

const (
	CodeInvalidConfidence reports.Code = "COMPONENT_INVALID_CONFIDENCE"
	CodeInvalidAcceptance reports.Code = "COMPONENT_INVALID_ACCEPTANCE"
)

type Catalog struct {
	Version              string                                     `json:"version"`
	GeneratedAt          *time.Time                                 `json:"generated_at,omitempty"`
	Records              []ComponentRecord                          `json:"records"`
	Families             []FamilyDefinition                         `json:"families"`
	Diagnostics          []reports.Issue                            `json:"diagnostics,omitempty"`
	mu                   sync.RWMutex                               `json:"-"`
	recordIndex          map[string]int                             `json:"-"`
	variantIndex         map[string]CatalogVariantIndex             `json:"-"`
	amplifierOutputIndex map[string][]amplifierOutputIndexCandidate `json:"-"`
}

type CatalogVariantIndex struct {
	Record  int
	Variant int
}

type amplifierOutputIndexCandidate struct {
	Record    int
	Variant   int
	Candidate Candidate
}

type FamilyDefinition struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type ComponentRecord struct {
	ID              string                   `json:"id"`
	Family          string                   `json:"family"`
	Name            string                   `json:"name"`
	Description     string                   `json:"description,omitempty"`
	Generic         bool                     `json:"generic"`
	Manufacturer    string                   `json:"manufacturer,omitempty"`
	MPN             string                   `json:"mpn,omitempty"`
	Lifecycle       string                   `json:"lifecycle,omitempty"`
	Equivalence     *EquivalenceMetadata     `json:"equivalence,omitempty"`
	Tags            []string                 `json:"tags,omitempty"`
	Values          []ValueConstraint        `json:"values,omitempty"`
	Ratings         []RatingConstraint       `json:"ratings,omitempty"`
	Tolerances      []ToleranceConstraint    `json:"tolerances,omitempty"`
	Temperature     *TemperatureRange        `json:"temperature,omitempty"`
	ElectricalRoles []ElectricalRole         `json:"electrical_roles,omitempty"`
	Symbols         []SymbolBinding          `json:"symbols,omitempty"`
	Packages        []PackageVariant         `json:"packages,omitempty"`
	Companions      []CompanionRequirement   `json:"companions,omitempty"`
	DeratingRules   []DeratingRule           `json:"derating_rules,omitempty"`
	Regulator       *RegulatorEvidence       `json:"regulator_evidence,omitempty"`
	Capacitor       *CapacitorEvidence       `json:"capacitor_evidence,omitempty"`
	OpAmp           *OpAmpEvidence           `json:"opamp_evidence,omitempty"`
	Sensor          *SensorEvidence          `json:"sensor_evidence,omitempty"`
	AmplifierOutput *AmplifierOutputEvidence `json:"amplifier_output_evidence,omitempty"`
	PlacementHints  []PlacementHint          `json:"placement_hints,omitempty"`
	RoutingHints    []RoutingHint            `json:"routing_hints,omitempty"`
	Properties      []SchematicProperty      `json:"properties,omitempty"`
	SelectionRules  []SelectionRule          `json:"selection_rules,omitempty"`
	Verification    VerificationRecord       `json:"verification"`
	SearchText      string                   `json:"-"`
}

type EquivalenceMetadata struct {
	Group string          `json:"group,omitempty"`
	Role  EquivalenceRole `json:"role,omitempty"`
	Notes []string        `json:"notes,omitempty"`
}

type PackageVariant struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	FootprintID  string               `json:"footprint_id"`
	PackageType  string               `json:"package_type,omitempty"`
	MPN          string               `json:"mpn,omitempty"`
	Lifecycle    string               `json:"lifecycle,omitempty"`
	PinMapID     string               `json:"pinmap_id,omitempty"`
	PadFunctions []PadFunction        `json:"pad_functions,omitempty"`
	DimensionsMM *Bounds              `json:"dimensions_mm,omitempty"`
	HeightMM     float64              `json:"height_mm,omitempty"`
	Constraints  []PhysicalConstraint `json:"constraints,omitempty"`
	Verification VerificationRecord   `json:"verification"`
	SearchText   string               `json:"-"`
}

type SymbolBinding struct {
	SymbolID     string             `json:"symbol_id"`
	Unit         int                `json:"unit,omitempty"`
	FunctionPins []FunctionPin      `json:"function_pins,omitempty"`
	PinMapID     string             `json:"pinmap_id,omitempty"`
	Verification VerificationRecord `json:"verification"`
}

type FunctionPin struct {
	Function   string   `json:"function"`
	SymbolPin  string   `json:"symbol_pin"`
	Electrical string   `json:"electrical,omitempty"`
	Polarity   string   `json:"polarity,omitempty"`
	Required   bool     `json:"required"`
	Aliases    []string `json:"aliases,omitempty"`
}

type PadFunction struct {
	Function string   `json:"function"`
	Pad      string   `json:"pad"`
	Polarity string   `json:"polarity,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

type ValueConstraint struct {
	Kind string `json:"kind"`
	Min  string `json:"min,omitempty"`
	Typ  string `json:"typ,omitempty"`
	Max  string `json:"max,omitempty"`
	Unit string `json:"unit,omitempty"`
}

type RatingConstraint struct {
	Kind string `json:"kind"`
	Min  string `json:"min,omitempty"`
	Typ  string `json:"typ,omitempty"`
	Max  string `json:"max,omitempty"`
	Unit string `json:"unit,omitempty"`
}

type ToleranceConstraint struct {
	Kind string `json:"kind"`
	Typ  string `json:"typ,omitempty"`
	Max  string `json:"max,omitempty"`
	Unit string `json:"unit,omitempty"`
}

type TemperatureRange struct {
	Min  string `json:"min,omitempty"`
	Max  string `json:"max,omitempty"`
	Unit string `json:"unit,omitempty"`
}

type ElectricalRole struct {
	Role        string `json:"role"`
	Description string `json:"description,omitempty"`
}

type SelectionRule struct {
	Kind        string   `json:"kind"`
	Expression  string   `json:"expression,omitempty"`
	Description string   `json:"description,omitempty"`
	AppliesTo   []string `json:"applies_to,omitempty"`
}

type PhysicalConstraint struct {
	Kind        string `json:"kind"`
	Value       string `json:"value,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
}

type CompanionRequirement struct {
	ID          string   `json:"id"`
	Family      string   `json:"family,omitempty"`
	Role        string   `json:"role"`
	Required    bool     `json:"required"`
	AppliesTo   []string `json:"applies_to,omitempty"`
	Description string   `json:"description,omitempty"`
}

type DeratingRule struct {
	Kind        string `json:"kind"`
	Expression  string `json:"expression,omitempty"`
	Description string `json:"description,omitempty"`
}

type RegulatorEvidence struct {
	OutputCapacitor *RegulatorCapacitorStability `json:"output_capacitor,omitempty"`
	ThermalReview   string                       `json:"thermal_review,omitempty"`
	Notes           []string                     `json:"notes,omitempty"`
}

type RegulatorCapacitorStability struct {
	Kind                       string   `json:"kind"`
	MinCapacitance             string   `json:"min_capacitance,omitempty"`
	MaxCapacitance             string   `json:"max_capacitance,omitempty"`
	CapacitanceUnit            string   `json:"capacitance_unit,omitempty"`
	AcceptedDielectrics        []string `json:"accepted_dielectrics,omitempty"`
	ESRMin                     string   `json:"esr_min,omitempty"`
	ESRMax                     string   `json:"esr_max,omitempty"`
	ESRUnit                    string   `json:"esr_unit,omitempty"`
	ProofStatus                string   `json:"proof_status,omitempty"`
	FabricationCandidateBlocks bool     `json:"fabrication_candidate_blocks,omitempty"`
	ReviewNote                 string   `json:"review_note,omitempty"`
}

type CapacitorEvidence struct {
	Dielectric                 string `json:"dielectric,omitempty"`
	NominalCapacitance         string `json:"nominal_capacitance,omitempty"`
	CapacitanceUnit            string `json:"capacitance_unit,omitempty"`
	VoltageRating              string `json:"voltage_rating,omitempty"`
	VoltageUnit                string `json:"voltage_unit,omitempty"`
	DCBiasReview               string `json:"dc_bias_review,omitempty"`
	EffectiveCapacitanceReview string `json:"effective_capacitance_review,omitempty"`
	ESRReview                  string `json:"esr_review,omitempty"`
	FabricationProof           bool   `json:"fabrication_proof,omitempty"`
	FabricationCandidateBlocks bool   `json:"fabrication_candidate_blocks,omitempty"`
	ReviewNote                 string `json:"review_note,omitempty"`
}

type OpAmpEvidence struct {
	IntendedRoles              []string `json:"intended_roles,omitempty"`
	SupplyMode                 string   `json:"supply_mode,omitempty"`
	OutputDriveStatus          string   `json:"output_drive_status,omitempty"`
	LoadCompatibilityStatus    string   `json:"load_compatibility_status,omitempty"`
	GainBandwidthStatus        string   `json:"gain_bandwidth_status,omitempty"`
	StabilityStatus            string   `json:"stability_status,omitempty"`
	InputCommonModeStatus      string   `json:"input_common_mode_status,omitempty"`
	FabricationCandidateBlocks bool     `json:"fabrication_candidate_blocks,omitempty"`
	ReviewNote                 string   `json:"review_note,omitempty"`
}

type SensorEvidence struct {
	Interfaces                []string                `json:"interfaces"`
	I2CAddresses              []SensorI2CAddress      `json:"i2c_addresses,omitempty"`
	I2CModeConnections        []SensorPinConnection   `json:"i2c_mode_connections,omitempty"`
	OptionalInterruptFunction string                  `json:"optional_interrupt_function,omitempty"`
	UnusedPinPolicies         []SensorUnusedPinPolicy `json:"unused_pin_policies,omitempty"`
	ReviewNote                string                  `json:"review_note,omitempty"`
}

type SensorI2CAddress struct {
	Address        string `json:"address"`
	SelectFunction string `json:"select_function,omitempty"`
	Level          string `json:"level,omitempty"`
	Default        bool   `json:"default,omitempty"`
}

type SensorPinConnection struct {
	Function string `json:"function"`
	Level    string `json:"level"`
}

type SensorUnusedPinPolicy struct {
	Function string `json:"function"`
	Policy   string `json:"policy"`
}

type AmplifierOutputEvidence struct {
	DeviceClass                string   `json:"device_class,omitempty"`
	Polarity                   string   `json:"polarity,omitempty"`
	IntendedRoles              []string `json:"intended_roles,omitempty"`
	Package                    string   `json:"package,omitempty"`
	SymbolID                   string   `json:"symbol_id,omitempty"`
	FootprintID                string   `json:"footprint_id,omitempty"`
	PinmapEvidence             string   `json:"pinmap_evidence,omitempty"`
	ComplementaryGroup         string   `json:"complementary_group,omitempty"`
	ControlTerminal            string   `json:"control_terminal,omitempty"`
	UpperOrLowerTerminal       string   `json:"upper_or_lower_terminal,omitempty"`
	OutputTerminal             string   `json:"output_terminal,omitempty"`
	VoltageRatingStatus        string   `json:"voltage_rating_status,omitempty"`
	CurrentRatingStatus        string   `json:"current_rating_status,omitempty"`
	PowerDissipationStatus     string   `json:"power_dissipation_status,omitempty"`
	ThermalReview              string   `json:"thermal_review,omitempty"`
	SafeOperatingAreaStatus    string   `json:"safe_operating_area_status,omitempty"`
	FabricationCandidateBlocks bool     `json:"fabrication_candidate_blocks,omitempty"`
	ReviewNote                 string   `json:"review_note,omitempty"`
}

type PlacementHint struct {
	Kind        string `json:"kind"`
	Target      string `json:"target,omitempty"`
	Value       string `json:"value,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
}

type RoutingHint struct {
	Kind        string `json:"kind"`
	NetRole     string `json:"net_role,omitempty"`
	Value       string `json:"value,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
}

type SchematicProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Bounds struct {
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

type VerificationRecord struct {
	Confidence      ConfidenceLevel `json:"confidence"`
	Sources         []string        `json:"sources,omitempty"`
	ResolverChecked bool            `json:"resolver_checked"`
	PinMapChecked   bool            `json:"pinmap_checked"`
	Tests           []string        `json:"tests,omitempty"`
	Notes           []string        `json:"notes,omitempty"`
}

func ValidConfidence(level ConfidenceLevel) bool {
	switch level {
	case ConfidenceVerified, ConfidenceLibraryDerived, ConfidenceRuleInferred, ConfidencePlaceholder, ConfidenceBlocked:
		return true
	default:
		return false
	}
}

func ValidAcceptance(level AcceptanceLevel) bool {
	if level == "" {
		return true
	}
	_, ok := acceptanceRank(level)
	return ok
}

func AcceptanceAllows(requested AcceptanceLevel, available ConfidenceLevel) bool {
	if requested == "" {
		requested = AcceptanceDraft
	}
	if available == ConfidenceBlocked || !ValidConfidence(available) {
		return false
	}
	switch requested {
	case AcceptanceDraft:
		return available == ConfidenceVerified || available == ConfidenceLibraryDerived || available == ConfidenceRuleInferred || available == ConfidencePlaceholder
	case AcceptanceStructural:
		return available == ConfidenceVerified || available == ConfidenceLibraryDerived || available == ConfidenceRuleInferred
	case AcceptanceConnectivity, AcceptanceERCDRC:
		return available == ConfidenceVerified
	case AcceptanceFabricationCandidate:
		return available == ConfidenceVerified
	default:
		return false
	}
}

func AcceptanceAllowsPassiveRuleInferred(requested AcceptanceLevel, available ConfidenceLevel) bool {
	// Use this only after the caller has proved the component is a symmetric
	// passive case, such as a two-terminal resistor or nonpolar capacitor.
	// Generic rule-inferred active parts must continue through AcceptanceAllows.
	if AcceptanceAllows(requested, available) {
		return true
	}
	return available == ConfidenceRuleInferred && (requested == AcceptanceConnectivity || requested == AcceptanceERCDRC)
}

func CompareAcceptance(a, b AcceptanceLevel) int {
	if a == "" {
		a = AcceptanceDraft
	}
	if b == "" {
		b = AcceptanceDraft
	}
	ar, aok := acceptanceRank(a)
	br, bok := acceptanceRank(b)
	if !aok && !bok {
		return strings.Compare(string(a), string(b))
	}
	if !aok {
		return -1
	}
	if !bok {
		return 1
	}
	if ar < br {
		return -1
	}
	if ar > br {
		return 1
	}
	return 0
}

func SortCatalog(catalog *Catalog) {
	if catalog == nil {
		return
	}
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	sort.SliceStable(catalog.Families, func(i, j int) bool {
		return catalog.Families[i].ID < catalog.Families[j].ID
	})
	sort.SliceStable(catalog.Records, func(i, j int) bool {
		return catalog.Records[i].ID < catalog.Records[j].ID
	})
	for i := range catalog.Records {
		sortRecord(&catalog.Records[i])
	}
	sortIssues(catalog.Diagnostics)
	rebuildCatalogIndexesLocked(catalog)
}

func RebuildCatalogIndexes(catalog *Catalog) {
	if catalog == nil {
		return
	}
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	rebuildCatalogIndexesLocked(catalog)
}

func rebuildCatalogIndexesLocked(catalog *Catalog) {
	catalog.recordIndex = map[string]int{}
	catalog.variantIndex = map[string]CatalogVariantIndex{}
	catalog.amplifierOutputIndex = map[string][]amplifierOutputIndexCandidate{}
	for i, record := range catalog.Records {
		catalog.Records[i].SearchText = strings.ToLower(record.ID + " " + record.Name + " " + record.Description + " " + strings.Join(record.Tags, " "))
		catalog.recordIndex[record.ID] = i
		for j, variant := range record.Packages {
			catalog.Records[i].Packages[j].SearchText = strings.ToLower(variant.ID + " " + variant.Name + " " + variant.PackageType + " " + variant.FootprintID)
			catalog.variantIndex[record.ID+"\x00"+variant.ID] = CatalogVariantIndex{Record: i, Variant: j}
			if amplifierOutputVariantIndexed(&catalog.Records[i], &catalog.Records[i].Packages[j]) {
				polarity := strings.ToLower(strings.TrimSpace(record.AmplifierOutput.Polarity))
				score := 100
				if record.Generic {
					score -= 25
				}
				catalog.amplifierOutputIndex[polarity] = append(catalog.amplifierOutputIndex[polarity], amplifierOutputIndexCandidate{
					Record:  i,
					Variant: j,
					Candidate: Candidate{
						ComponentID: record.ID,
						VariantID:   variant.ID,
						Family:      record.Family,
						Name:        record.Name,
						FootprintID: variant.FootprintID,
						Confidence:  record.Verification.Confidence,
						Score:       score,
						Generic:     record.Generic,
						Reasons:     []string{"amplifier_output_evidence", "polarity:" + polarity, "role:headphone_output"},
					},
				})
			}
		}
	}
	for polarity := range catalog.amplifierOutputIndex {
		sort.SliceStable(catalog.amplifierOutputIndex[polarity], func(i, j int) bool {
			left := catalog.amplifierOutputIndex[polarity][i].Candidate
			right := catalog.amplifierOutputIndex[polarity][j].Candidate
			if left.Score == right.Score {
				return left.ComponentID < right.ComponentID
			}
			return left.Score > right.Score
		})
	}
}

func amplifierOutputVariantIndexed(record *ComponentRecord, variant *PackageVariant) bool {
	if record.AmplifierOutput == nil {
		return false
	}
	if !containsString(record.AmplifierOutput.IntendedRoles, "headphone_output") {
		return false
	}
	if strings.TrimSpace(record.AmplifierOutput.Polarity) == "" {
		return false
	}
	if !amplifierOutputSupportsComplementaryBJT(record.AmplifierOutput) {
		return false
	}
	if record.AmplifierOutput.FootprintID != "" && variant.FootprintID != record.AmplifierOutput.FootprintID {
		return false
	}
	return true
}

func amplifierOutputSupportsComplementaryBJT(evidence *AmplifierOutputEvidence) bool {
	if evidence == nil {
		return false
	}
	return strings.EqualFold(evidence.DeviceClass, "bjt") &&
		strings.EqualFold(evidence.ControlTerminal, "BASE") &&
		strings.EqualFold(evidence.UpperOrLowerTerminal, "COLLECTOR") &&
		strings.EqualFold(evidence.OutputTerminal, "EMITTER")
}

func NewIssue(code reports.Code, severity reports.Severity, path string, message string) reports.Issue {
	return reports.Issue{
		Code:     code,
		Severity: severity,
		Path:     path,
		Message:  message,
	}
}

func ValidateConfidenceIssue(path string, level ConfidenceLevel) (reports.Issue, bool) {
	if ValidConfidence(level) {
		return reports.Issue{}, false
	}
	return NewIssue(CodeInvalidConfidence, reports.SeverityBlocked, path, "invalid component confidence level: "+string(level)), true
}

func ValidateAcceptanceIssue(path string, level AcceptanceLevel) (reports.Issue, bool) {
	if ValidAcceptance(level) {
		return reports.Issue{}, false
	}
	return NewIssue(CodeInvalidAcceptance, reports.SeverityBlocked, path, "invalid component acceptance level: "+string(level)), true
}

func sortRecord(record *ComponentRecord) {
	sort.Strings(record.Tags)
	sortVerification(&record.Verification)
	sortValueConstraints(record.Values)
	sortRatingConstraints(record.Ratings)
	sortToleranceConstraints(record.Tolerances)
	sort.SliceStable(record.ElectricalRoles, func(i, j int) bool {
		if record.ElectricalRoles[i].Role == record.ElectricalRoles[j].Role {
			return record.ElectricalRoles[i].Description < record.ElectricalRoles[j].Description
		}
		return record.ElectricalRoles[i].Role < record.ElectricalRoles[j].Role
	})
	sort.SliceStable(record.Symbols, func(i, j int) bool {
		if record.Symbols[i].SymbolID == record.Symbols[j].SymbolID {
			return record.Symbols[i].Unit < record.Symbols[j].Unit
		}
		return record.Symbols[i].SymbolID < record.Symbols[j].SymbolID
	})
	for i := range record.Symbols {
		sortVerification(&record.Symbols[i].Verification)
		sort.SliceStable(record.Symbols[i].FunctionPins, func(a, b int) bool {
			if record.Symbols[i].FunctionPins[a].Function == record.Symbols[i].FunctionPins[b].Function {
				return record.Symbols[i].FunctionPins[a].SymbolPin < record.Symbols[i].FunctionPins[b].SymbolPin
			}
			return record.Symbols[i].FunctionPins[a].Function < record.Symbols[i].FunctionPins[b].Function
		})
		for j := range record.Symbols[i].FunctionPins {
			sort.Strings(record.Symbols[i].FunctionPins[j].Aliases)
		}
	}
	sort.SliceStable(record.Packages, func(i, j int) bool {
		return record.Packages[i].ID < record.Packages[j].ID
	})
	for i := range record.Packages {
		sortVerification(&record.Packages[i].Verification)
		sort.SliceStable(record.Packages[i].PadFunctions, func(a, b int) bool {
			if record.Packages[i].PadFunctions[a].Function == record.Packages[i].PadFunctions[b].Function {
				return record.Packages[i].PadFunctions[a].Pad < record.Packages[i].PadFunctions[b].Pad
			}
			return record.Packages[i].PadFunctions[a].Function < record.Packages[i].PadFunctions[b].Function
		})
		for j := range record.Packages[i].PadFunctions {
			sort.Strings(record.Packages[i].PadFunctions[j].Aliases)
		}
		sort.SliceStable(record.Packages[i].Constraints, func(a, b int) bool {
			if record.Packages[i].Constraints[a].Kind == record.Packages[i].Constraints[b].Kind {
				return record.Packages[i].Constraints[a].Value < record.Packages[i].Constraints[b].Value
			}
			return record.Packages[i].Constraints[a].Kind < record.Packages[i].Constraints[b].Kind
		})
	}
	sort.SliceStable(record.Companions, func(i, j int) bool {
		if record.Companions[i].ID == record.Companions[j].ID {
			return record.Companions[i].Role < record.Companions[j].Role
		}
		return record.Companions[i].ID < record.Companions[j].ID
	})
	for i := range record.Companions {
		sort.Strings(record.Companions[i].AppliesTo)
	}
	sort.SliceStable(record.DeratingRules, func(i, j int) bool {
		if record.DeratingRules[i].Kind == record.DeratingRules[j].Kind {
			return record.DeratingRules[i].Expression < record.DeratingRules[j].Expression
		}
		return record.DeratingRules[i].Kind < record.DeratingRules[j].Kind
	})
	sort.SliceStable(record.PlacementHints, func(i, j int) bool {
		if record.PlacementHints[i].Kind == record.PlacementHints[j].Kind {
			return record.PlacementHints[i].Target < record.PlacementHints[j].Target
		}
		return record.PlacementHints[i].Kind < record.PlacementHints[j].Kind
	})
	sort.SliceStable(record.RoutingHints, func(i, j int) bool {
		if record.RoutingHints[i].Kind == record.RoutingHints[j].Kind {
			return record.RoutingHints[i].NetRole < record.RoutingHints[j].NetRole
		}
		return record.RoutingHints[i].Kind < record.RoutingHints[j].Kind
	})
	sort.SliceStable(record.Properties, func(i, j int) bool {
		return record.Properties[i].Name < record.Properties[j].Name
	})
	sort.SliceStable(record.SelectionRules, func(i, j int) bool {
		if record.SelectionRules[i].Kind == record.SelectionRules[j].Kind {
			return record.SelectionRules[i].Expression < record.SelectionRules[j].Expression
		}
		return record.SelectionRules[i].Kind < record.SelectionRules[j].Kind
	})
}

func sortVerification(record *VerificationRecord) {
	sort.Strings(record.Sources)
	sort.Strings(record.Tests)
	sort.Strings(record.Notes)
}

func sortIssues(issues []reports.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			if issues[i].Code == issues[j].Code {
				return issues[i].Message < issues[j].Message
			}
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Path < issues[j].Path
	})
}

func acceptanceRank(level AcceptanceLevel) (int, bool) {
	switch level {
	case AcceptanceDraft:
		return 0, true
	case AcceptanceStructural:
		return 1, true
	case AcceptanceConnectivity:
		return 2, true
	case AcceptanceERCDRC:
		return 3, true
	case AcceptanceFabricationCandidate:
		return 4, true
	default:
		return 0, false
	}
}

type constraintSortKey struct {
	value    string
	unit     string
	number   float64
	hasValue bool
}

func sortValueConstraints(values []ValueConstraint) {
	keyed := make([]struct {
		value ValueConstraint
		key   constraintSortKey
	}, len(values))
	for i, value := range values {
		keyed[i].value = value
		keyed[i].key = makeConstraintSortKey(value.Typ, value.Unit)
	}
	sort.SliceStable(keyed, func(i, j int) bool {
		if keyed[i].value.Kind == keyed[j].value.Kind {
			return compareConstraintSortKeys(keyed[i].key, keyed[j].key) < 0
		}
		return keyed[i].value.Kind < keyed[j].value.Kind
	})
	for i := range keyed {
		values[i] = keyed[i].value
	}
}

func sortRatingConstraints(ratings []RatingConstraint) {
	keyed := make([]struct {
		rating RatingConstraint
		key    constraintSortKey
	}, len(ratings))
	for i, rating := range ratings {
		keyed[i].rating = rating
		keyed[i].key = makeConstraintSortKey(rating.Max, rating.Unit)
	}
	sort.SliceStable(keyed, func(i, j int) bool {
		if keyed[i].rating.Kind == keyed[j].rating.Kind {
			return compareConstraintSortKeys(keyed[i].key, keyed[j].key) < 0
		}
		return keyed[i].rating.Kind < keyed[j].rating.Kind
	})
	for i := range keyed {
		ratings[i] = keyed[i].rating
	}
}

func sortToleranceConstraints(tolerances []ToleranceConstraint) {
	keyed := make([]struct {
		tolerance ToleranceConstraint
		key       constraintSortKey
	}, len(tolerances))
	for i, tolerance := range tolerances {
		keyed[i].tolerance = tolerance
		keyed[i].key = makeConstraintSortKey(tolerance.Max, tolerance.Unit)
	}
	sort.SliceStable(keyed, func(i, j int) bool {
		if keyed[i].tolerance.Kind == keyed[j].tolerance.Kind {
			return compareConstraintSortKeys(keyed[i].key, keyed[j].key) < 0
		}
		return keyed[i].tolerance.Kind < keyed[j].tolerance.Kind
	})
	for i := range keyed {
		tolerances[i] = keyed[i].tolerance
	}
}

func makeConstraintSortKey(value string, unit string) constraintSortKey {
	combined := strings.TrimSpace(value) + strings.TrimSpace(unit)
	number, ok := parseLeadingEngineeringNumber(combined)
	return constraintSortKey{
		value:    value,
		unit:     unit,
		number:   number,
		hasValue: ok,
	}
}

func compareConstraintSortKeys(a constraintSortKey, b constraintSortKey) int {
	if a.hasValue && b.hasValue && a.number != b.number {
		if a.number < b.number {
			return -1
		}
		return 1
	}
	if !strings.EqualFold(a.unit, b.unit) {
		return strings.Compare(a.unit, b.unit)
	}
	return strings.Compare(a.value, b.value)
}

func parseLeadingEngineeringNumber(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if len(value) > maxEngineeringValueLength {
		return 0, false
	}
	if number, ok := parseEmbeddedEngineeringNumber(value); ok {
		return number, true
	}
	end := scanLeadingFloat(value)
	if end <= 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(value[:end], 64)
	if err != nil {
		return 0, false
	}
	multiplier := 1.0
	suffix := strings.TrimSpace(value[end:])
	if suffix != "" {
		r, _ := utf8.DecodeRuneInString(suffix)
		if validEngineeringSuffix(suffix) {
			switch r {
			case 'f':
				multiplier = 1e-15
			case 'p':
				multiplier = 1e-12
			case 'n':
				multiplier = 1e-9
			case 'u', 'µ', 'μ':
				multiplier = 1e-6
			case 'm':
				multiplier = 1e-3
			case 'k', 'K':
				multiplier = 1e3
			case 'M':
				multiplier = 1e6
			case 'G':
				multiplier = 1e9
			case 'T':
				multiplier = 1e12
			case 'P':
				multiplier = 1e15
			}
		}
	}
	number *= multiplier
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func parseEmbeddedEngineeringNumber(value string) (float64, bool) {
	runes := []rune(value)
	for i := 1; i < len(runes)-1; i++ {
		if !isEmbeddedEngineeringMarker(runes[i]) {
			continue
		}
		if !asciiDigitRune(runes[i-1]) || !asciiDigitRune(runes[i+1]) {
			continue
		}
		left := string(runes[:i])
		rightEnd := i + 1
		for rightEnd < len(runes) && asciiDigitRune(runes[rightEnd]) {
			rightEnd++
		}
		right := string(runes[i+1 : rightEnd])
		number, err := strconv.ParseFloat(left+"."+right, 64)
		if err != nil {
			return 0, false
		}
		number *= engineeringMultiplier(runes[i])
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return 0, false
		}
		return number, true
	}
	return 0, false
}

func validEngineeringSuffix(suffix string) bool {
	if suffix == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(suffix)
	if !isEngineeringPrefix(r) {
		return false
	}
	rest := strings.TrimSpace(suffix[size:])
	if rest == "" {
		return true
	}
	return isElectricalUnitSuffix(rest)
}

func isEngineeringPrefix(r rune) bool {
	switch r {
	case 'f', 'p', 'n', 'u', 'µ', 'μ', 'm', 'k', 'K', 'M', 'G', 'T', 'P':
		return true
	default:
		return false
	}
}

func isEmbeddedEngineeringMarker(r rune) bool {
	return isEngineeringPrefix(r) || r == 'R' || r == 'r'
}

func engineeringMultiplier(r rune) float64 {
	switch r {
	case 'f':
		return 1e-15
	case 'p':
		return 1e-12
	case 'n':
		return 1e-9
	case 'u', 'µ', 'μ':
		return 1e-6
	case 'm':
		return 1e-3
	case 'k', 'K':
		return 1e3
	case 'M':
		return 1e6
	case 'G':
		return 1e9
	case 'T':
		return 1e12
	case 'P':
		return 1e15
	default:
		return 1
	}
}

func asciiDigitRune(r rune) bool {
	return r >= '0' && r <= '9'
}

func isElectricalUnitSuffix(suffix string) bool {
	switch strings.ToLower(strings.TrimSpace(suffix)) {
	case "a", "amp", "amps", "ampere", "amperes",
		"f", "farad", "farads",
		"h", "henry", "henries", "hz",
		"v", "volt", "volts",
		"w", "watt", "watts",
		"o", "ohm", "ohms", "r", "s", "siemens", "Ω":
		return true
	default:
		return false
	}
}

func scanLeadingFloat(value string) int {
	i := 0
	if i < len(value) && (value[i] == '+' || value[i] == '-') {
		i++
	}
	digits := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
		digits++
	}
	if i < len(value) && value[i] == '.' {
		i++
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return -1
	}
	if i < len(value) && (value[i] == 'e' || value[i] == 'E') {
		expStart := i
		i++
		if i < len(value) && (value[i] == '+' || value[i] == '-') {
			i++
		}
		expDigits := 0
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
			expDigits++
		}
		if expDigits == 0 {
			return expStart
		}
	}
	return i
}
