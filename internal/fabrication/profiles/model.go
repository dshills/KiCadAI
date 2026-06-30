package profiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"kicadai/internal/reports"
)

const SchemaV1 = "kicadai.fabrication.profile.v1"

var asciiTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

var warningOnlyFieldPaths = map[string]struct{}{
	"stackup":                                        {},
	"stackup.min_layers":                             {},
	"stackup.max_layers":                             {},
	"stackup.allowed_layer_counts":                   {},
	"stackup.min_board_thickness_mm":                 {},
	"stackup.max_board_thickness_mm":                 {},
	"stackup.default_board_thickness_mm":             {},
	"stackup.requires_two_outer_copper_layers":       {},
	"stackup.requires_internal_planes_for_impedance": {},
	"copper":                                          {},
	"copper.min_trace_width_mm":                       {},
	"copper.min_spacing_mm":                           {},
	"copper.min_zone_min_thickness_mm":                {},
	"copper.min_copper_to_edge_mm":                    {},
	"copper.min_copper_sliver_mm":                     {},
	"copper.allow_neckdown":                           {},
	"copper.min_neckdown_width_mm":                    {},
	"copper.max_neckdown_length_mm":                   {},
	"drill":                                           {},
	"drill.min_drill_mm":                              {},
	"drill.min_via_drill_mm":                          {},
	"drill.min_finished_hole_mm":                      {},
	"drill.min_plated_slot_width_mm":                  {},
	"drill.min_pad_annular_ring_mm":                   {},
	"drill.min_via_annular_ring_mm":                   {},
	"drill.min_hole_to_edge_mm":                       {},
	"drill.allow_microvias":                           {},
	"drill.allow_blind_buried_vias":                   {},
	"solder_mask":                                     {},
	"solder_mask.min_solder_mask_web_mm":              {},
	"solder_mask.default_mask_expansion_mm":           {},
	"solder_mask.min_mask_clearance_mm":               {},
	"solder_mask.paste_required_for_smd":              {},
	"solder_mask.allow_missing_paste_on_smd":          {},
	"silkscreen":                                      {},
	"silkscreen.min_silkscreen_line_width_mm":         {},
	"silkscreen.min_silkscreen_text_height_mm":        {},
	"silkscreen.min_silkscreen_to_mask_mm":            {},
	"assembly":                                        {},
	"assembly.require_courtyards":                     {},
	"assembly.require_reference_designators":          {},
	"assembly.require_bom_identity":                   {},
	"assembly.require_cpl_for_assembled_parts":        {},
	"edge_plating":                                    {},
	"edge_plating.allow_castellations":                {},
	"edge_plating.allow_edge_plating":                 {},
	"edge_plating.min_castellation_drill_mm":          {},
	"edge_plating.min_castellation_pitch_mm":          {},
	"edge_plating.requires_edge_plating_notes":        {},
	"edge_plating.requires_manual_review":             {},
	"impedance":                                       {},
	"impedance.allow_impedance_claims_without_solver": {},
	"impedance.require_stackup_for_impedance":         {},
	"impedance.require_diff_pair_width_gap_evidence":  {},
	"impedance.require_diff_pair_skew_evidence":       {},
	"metadata":                           {},
	"metadata.require_board_finish":      {},
	"metadata.allowed_board_finishes":    {},
	"metadata.require_panelization":      {},
	"metadata.require_fabrication_notes": {},
	"metadata.require_assembly_notes":    {},
}

type SourceKind string

const (
	SourceBuiltin SourceKind = "builtin"
	SourceLocal   SourceKind = "local"
)

type Source struct {
	Kind        SourceKind `json:"kind,omitempty"`
	Path        string     `json:"path,omitempty"`
	URL         string     `json:"url,omitempty"`
	RetrievedAt string     `json:"retrieved_at,omitempty"`
}

type Profile struct {
	// Keep cloneProfile in builtin.go updated when adding reference-typed fields.
	Schema      string     `json:"schema"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Source      Source     `json:"source,omitempty"`
	Units       string     `json:"units"`
	Stackup     Stackup    `json:"stackup,omitempty"`
	Copper      Copper     `json:"copper,omitempty"`
	Drill       Drill      `json:"drill,omitempty"`
	SolderMask  SolderMask `json:"solder_mask,omitempty"`
	Silkscreen  Silkscreen `json:"silkscreen,omitempty"`
	Assembly    Assembly   `json:"assembly,omitempty"`
	EdgePlating EdgePolicy `json:"edge_plating,omitempty"`
	Impedance   Impedance  `json:"impedance,omitempty"`
	Metadata    Metadata   `json:"metadata,omitempty"`
}

type Stackup struct {
	MinLayers                          int     `json:"min_layers,omitempty"`
	MaxLayers                          int     `json:"max_layers,omitempty"`
	AllowedLayerCounts                 []int   `json:"allowed_layer_counts,omitempty"`
	MinBoardThicknessMM                float64 `json:"min_board_thickness_mm,omitempty"`
	MaxBoardThicknessMM                float64 `json:"max_board_thickness_mm,omitempty"`
	DefaultBoardThicknessMM            float64 `json:"default_board_thickness_mm,omitempty"`
	RequiresTwoOuterCopperLayers       bool    `json:"requires_two_outer_copper_layers,omitempty"`
	RequiresInternalPlanesForImpedance bool    `json:"requires_internal_planes_for_impedance,omitempty"`
}

type Copper struct {
	MinTraceWidthMM       float64 `json:"min_trace_width_mm,omitempty"`
	MinSpacingMM          float64 `json:"min_spacing_mm,omitempty"`
	MinZoneMinThicknessMM float64 `json:"min_zone_min_thickness_mm,omitempty"`
	MinCopperToEdgeMM     float64 `json:"min_copper_to_edge_mm,omitempty"`
	MinCopperSliverMM     float64 `json:"min_copper_sliver_mm,omitempty"`
	AllowNeckdown         bool    `json:"allow_neckdown,omitempty"`
	MinNeckdownWidthMM    float64 `json:"min_neckdown_width_mm,omitempty"`
	MaxNeckdownLengthMM   float64 `json:"max_neckdown_length_mm,omitempty"`
}

type Drill struct {
	MinDrillMM           float64 `json:"min_drill_mm,omitempty"`
	MinViaDrillMM        float64 `json:"min_via_drill_mm,omitempty"`
	MinFinishedHoleMM    float64 `json:"min_finished_hole_mm,omitempty"`
	MinPlatedSlotWidthMM float64 `json:"min_plated_slot_width_mm,omitempty"`
	MinPadAnnularRingMM  float64 `json:"min_pad_annular_ring_mm,omitempty"`
	MinViaAnnularRingMM  float64 `json:"min_via_annular_ring_mm,omitempty"`
	MinHoleToEdgeMM      float64 `json:"min_hole_to_edge_mm,omitempty"`
	AllowMicrovias       bool    `json:"allow_microvias,omitempty"`
	AllowBlindBuriedVias bool    `json:"allow_blind_buried_vias,omitempty"`
}

type SolderMask struct {
	MinSolderMaskWebMM     float64 `json:"min_solder_mask_web_mm,omitempty"`
	DefaultMaskExpansionMM float64 `json:"default_mask_expansion_mm,omitempty"`
	MinMaskClearanceMM     float64 `json:"min_mask_clearance_mm,omitempty"`
	PasteRequiredForSMD    bool    `json:"paste_required_for_smd,omitempty"`
	AllowMissingPasteOnSMD bool    `json:"allow_missing_paste_on_smd,omitempty"`
}

type Silkscreen struct {
	MinLineWidthMM        float64 `json:"min_silkscreen_line_width_mm,omitempty"`
	MinTextHeightMM       float64 `json:"min_silkscreen_text_height_mm,omitempty"`
	MinSilkscreenToMaskMM float64 `json:"min_silkscreen_to_mask_mm,omitempty"`
}

type Assembly struct {
	RequireCourtyards           bool `json:"require_courtyards,omitempty"`
	RequireReferenceDesignators bool `json:"require_reference_designators,omitempty"`
	RequireBOMIdentity          bool `json:"require_bom_identity,omitempty"`
	RequireCPLForAssembledParts bool `json:"require_cpl_for_assembled_parts,omitempty"`
}

type EdgePolicy struct {
	AllowCastellations       bool    `json:"allow_castellations,omitempty"`
	AllowEdgePlating         bool    `json:"allow_edge_plating,omitempty"`
	MinCastellationDrillMM   float64 `json:"min_castellation_drill_mm,omitempty"`
	MinCastellationPitchMM   float64 `json:"min_castellation_pitch_mm,omitempty"`
	RequiresEdgePlatingNotes bool    `json:"requires_edge_plating_notes,omitempty"`
	RequiresManualReview     bool    `json:"requires_manual_review,omitempty"`
}

type Impedance struct {
	AllowClaimsWithoutSolver        bool `json:"allow_impedance_claims_without_solver,omitempty"`
	RequireStackupForImpedance      bool `json:"require_stackup_for_impedance,omitempty"`
	RequireDiffPairWidthGapEvidence bool `json:"require_diff_pair_width_gap_evidence,omitempty"`
	RequireDiffPairSkewEvidence     bool `json:"require_diff_pair_skew_evidence,omitempty"`
}

type Metadata struct {
	RequireBoardFinish      bool     `json:"require_board_finish,omitempty"`
	AllowedBoardFinishes    []string `json:"allowed_board_finishes,omitempty"`
	RequirePanelization     bool     `json:"require_panelization,omitempty"`
	RequireFabricationNotes bool     `json:"require_fabrication_notes,omitempty"`
	RequireAssemblyNotes    bool     `json:"require_assembly_notes,omitempty"`
	WarningOnlyFields       []string `json:"warning_only_fields,omitempty"`
}

type Summary struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Source  Source          `json:"source,omitempty"`
	Hash    string          `json:"hash"`
	Issues  []reports.Issue `json:"issues,omitempty"`
}

func Validate(profile Profile) []reports.Issue {
	var issues []reports.Issue
	if strings.TrimSpace(profile.Schema) != SchemaV1 {
		issues = append(issues, issue("schema", "unsupported fabrication profile schema"))
	}
	id := strings.TrimSpace(profile.ID)
	if id == "" {
		issues = append(issues, issue("id", "fabrication profile id is required"))
	} else if !asciiTokenPattern.MatchString(id) {
		issues = append(issues, issue("id", "fabrication profile id must be an ASCII identifier"))
	}
	if strings.TrimSpace(profile.Name) == "" {
		issues = append(issues, issue("name", "fabrication profile name is required"))
	}
	if strings.TrimSpace(profile.Version) == "" {
		issues = append(issues, issue("version", "fabrication profile version is required"))
	} else if !asciiTokenPattern.MatchString(strings.TrimSpace(profile.Version)) {
		issues = append(issues, issue("version", "fabrication profile version must be an ASCII version token"))
	}
	issues = append(issues, validateSource(profile.Source)...)
	if units := strings.TrimSpace(profile.Units); units != "mm" {
		issues = append(issues, issue("units", "fabrication profile units must be mm"))
	}
	issues = append(issues, validateStackup(profile.Stackup)...)
	issues = append(issues, validateMetadata(profile.Metadata)...)
	positiveChecks := []struct {
		path  string
		value float64
	}{
		{"copper.min_trace_width_mm", profile.Copper.MinTraceWidthMM},
		{"copper.min_spacing_mm", profile.Copper.MinSpacingMM},
		{"copper.min_zone_min_thickness_mm", profile.Copper.MinZoneMinThicknessMM},
		{"copper.min_copper_to_edge_mm", profile.Copper.MinCopperToEdgeMM},
		{"copper.min_copper_sliver_mm", profile.Copper.MinCopperSliverMM},
		{"copper.min_neckdown_width_mm", profile.Copper.MinNeckdownWidthMM},
		{"copper.max_neckdown_length_mm", profile.Copper.MaxNeckdownLengthMM},
		{"drill.min_drill_mm", profile.Drill.MinDrillMM},
		{"drill.min_via_drill_mm", profile.Drill.MinViaDrillMM},
		{"drill.min_finished_hole_mm", profile.Drill.MinFinishedHoleMM},
		{"drill.min_plated_slot_width_mm", profile.Drill.MinPlatedSlotWidthMM},
		{"drill.min_pad_annular_ring_mm", profile.Drill.MinPadAnnularRingMM},
		{"drill.min_via_annular_ring_mm", profile.Drill.MinViaAnnularRingMM},
		{"drill.min_hole_to_edge_mm", profile.Drill.MinHoleToEdgeMM},
		{"solder_mask.min_solder_mask_web_mm", profile.SolderMask.MinSolderMaskWebMM},
		{"solder_mask.default_mask_expansion_mm", profile.SolderMask.DefaultMaskExpansionMM},
		{"solder_mask.min_mask_clearance_mm", profile.SolderMask.MinMaskClearanceMM},
		{"silkscreen.min_silkscreen_line_width_mm", profile.Silkscreen.MinLineWidthMM},
		{"silkscreen.min_silkscreen_text_height_mm", profile.Silkscreen.MinTextHeightMM},
		{"silkscreen.min_silkscreen_to_mask_mm", profile.Silkscreen.MinSilkscreenToMaskMM},
		{"edge_plating.min_castellation_drill_mm", profile.EdgePlating.MinCastellationDrillMM},
		{"edge_plating.min_castellation_pitch_mm", profile.EdgePlating.MinCastellationPitchMM},
	}
	for _, check := range positiveChecks {
		if check.value < 0 {
			issues = append(issues, issue(check.path, "fabrication profile threshold cannot be negative"))
		}
	}
	if profile.EdgePlating.MinCastellationPitchMM > 0 &&
		profile.EdgePlating.MinCastellationDrillMM > 0 &&
		profile.EdgePlating.MinCastellationPitchMM <= profile.EdgePlating.MinCastellationDrillMM {
		issues = append(issues, issue("edge_plating.min_castellation_pitch_mm", "minimum castellation pitch must exceed minimum castellation drill"))
	}
	slices.SortFunc(issues, compareIssues)
	return issues
}

func validateSource(source Source) []reports.Issue {
	var issues []reports.Issue
	switch source.Kind {
	case "", SourceBuiltin, SourceLocal:
	default:
		issues = append(issues, issue("source.kind", "fabrication profile source kind must be builtin or local"))
	}
	if retrievedAt := strings.TrimSpace(source.RetrievedAt); retrievedAt != "" {
		if _, err := time.Parse(time.RFC3339, retrievedAt); err != nil {
			issues = append(issues, issue("source.retrieved_at", "fabrication profile retrieved_at must be RFC3339"))
		}
	}
	return issues
}

func validateMetadata(metadata Metadata) []reports.Issue {
	var issues []reports.Issue
	issues = append(issues, validateStringSet("metadata.allowed_board_finishes", metadata.AllowedBoardFinishes)...)
	seenWarningOnlyFields := map[string]struct{}{}
	for index, value := range metadata.WarningOnlyFields {
		normalized := strings.TrimSpace(value)
		itemPath := fmt.Sprintf("metadata.warning_only_fields[%d]", index)
		if normalized == "" {
			issues = append(issues, issue(itemPath, "fabrication profile list value cannot be empty"))
			continue
		}
		if _, exists := seenWarningOnlyFields[normalized]; exists {
			issues = append(issues, issue(itemPath, "duplicate fabrication profile list value"))
		}
		seenWarningOnlyFields[normalized] = struct{}{}
		if _, ok := warningOnlyFieldPaths[normalized]; !ok {
			issues = append(issues, issue(itemPath, "warning-only field must match a known fabrication profile path"))
		}
	}
	return issues
}

func validateStringSet(path string, values []string) []reports.Issue {
	var issues []reports.Issue
	seen := map[string]struct{}{}
	for index, value := range values {
		normalized := strings.TrimSpace(value)
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		if normalized == "" {
			issues = append(issues, issue(itemPath, "fabrication profile list value cannot be empty"))
			continue
		}
		if _, exists := seen[normalized]; exists {
			issues = append(issues, issue(itemPath, "duplicate fabrication profile list value"))
		}
		seen[normalized] = struct{}{}
	}
	return issues
}

func validateStackup(stackup Stackup) []reports.Issue {
	var issues []reports.Issue
	if stackup.MinLayers < 0 {
		issues = append(issues, issue("stackup.min_layers", "minimum layer count cannot be negative"))
	}
	if stackup.MaxLayers < 0 {
		issues = append(issues, issue("stackup.max_layers", "maximum layer count cannot be negative"))
	}
	if stackup.MinLayers > 0 && stackup.MaxLayers == 0 {
		issues = append(issues, issue("stackup.max_layers", "maximum layer count is required when minimum layer count is set"))
	}
	if stackup.MinLayers > 0 && stackup.MaxLayers > 0 && stackup.MinLayers > stackup.MaxLayers {
		issues = append(issues, issue("stackup.min_layers", "minimum layer count cannot exceed maximum layer count"))
	}
	seen := map[int]struct{}{}
	for index, count := range stackup.AllowedLayerCounts {
		if count <= 0 {
			issues = append(issues, issue(fmt.Sprintf("stackup.allowed_layer_counts[%d]", index), "allowed layer counts must be positive"))
			continue
		}
		if _, exists := seen[count]; exists {
			issues = append(issues, issue(fmt.Sprintf("stackup.allowed_layer_counts[%d]", index), "duplicate allowed layer count"))
		}
		if stackup.MinLayers > 0 && count < stackup.MinLayers {
			issues = append(issues, issue(fmt.Sprintf("stackup.allowed_layer_counts[%d]", index), "allowed layer count cannot be below minimum layer count"))
		}
		if stackup.MaxLayers > 0 && count > stackup.MaxLayers {
			issues = append(issues, issue(fmt.Sprintf("stackup.allowed_layer_counts[%d]", index), "allowed layer count cannot exceed maximum layer count"))
		}
		seen[count] = struct{}{}
	}
	for _, check := range []struct {
		path  string
		value float64
	}{
		{"stackup.min_board_thickness_mm", stackup.MinBoardThicknessMM},
		{"stackup.max_board_thickness_mm", stackup.MaxBoardThicknessMM},
		{"stackup.default_board_thickness_mm", stackup.DefaultBoardThicknessMM},
	} {
		if check.value < 0 {
			issues = append(issues, issue(check.path, "board thickness cannot be negative"))
		}
	}
	if stackup.MinBoardThicknessMM > 0 && stackup.MaxBoardThicknessMM > 0 && stackup.MinBoardThicknessMM > stackup.MaxBoardThicknessMM {
		issues = append(issues, issue("stackup.min_board_thickness_mm", "minimum board thickness cannot exceed maximum board thickness"))
	}
	if stackup.MinBoardThicknessMM > 0 && stackup.MaxBoardThicknessMM == 0 {
		issues = append(issues, issue("stackup.max_board_thickness_mm", "maximum board thickness is required when minimum board thickness is set"))
	}
	if stackup.DefaultBoardThicknessMM > 0 && stackup.MinBoardThicknessMM > 0 && stackup.DefaultBoardThicknessMM < stackup.MinBoardThicknessMM {
		issues = append(issues, issue("stackup.default_board_thickness_mm", "default board thickness cannot be below minimum board thickness"))
	}
	if stackup.DefaultBoardThicknessMM > 0 && stackup.MaxBoardThicknessMM > 0 && stackup.DefaultBoardThicknessMM > stackup.MaxBoardThicknessMM {
		issues = append(issues, issue("stackup.default_board_thickness_mm", "default board thickness cannot exceed maximum board thickness"))
	}
	return issues
}

func Hash(profile Profile) (string, error) {
	normalized := canonicalProfile(profile)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func Summarize(profile Profile) Summary {
	hash, err := Hash(profile)
	issues := Validate(profile)
	if err != nil {
		issues = append(issues, issue("hash", "fabrication profile hash could not be computed"))
		slices.SortFunc(issues, compareIssues)
	}
	return Summary{
		ID:      strings.TrimSpace(profile.ID),
		Name:    strings.TrimSpace(profile.Name),
		Version: strings.TrimSpace(profile.Version),
		Source:  profile.Source,
		Hash:    hash,
		Issues:  issues,
	}
}

func canonicalProfile(profile Profile) Profile {
	normalized := profile
	normalized.Schema = strings.TrimSpace(profile.Schema)
	normalized.ID = strings.TrimSpace(profile.ID)
	normalized.Name = strings.TrimSpace(profile.Name)
	normalized.Version = strings.TrimSpace(profile.Version)
	normalized.Source = Source{}
	normalized.Units = strings.TrimSpace(profile.Units)
	normalized.Stackup.AllowedLayerCounts = slices.Clone(profile.Stackup.AllowedLayerCounts)
	normalized.Metadata.AllowedBoardFinishes = canonicalStringSet(profile.Metadata.AllowedBoardFinishes)
	normalized.Metadata.WarningOnlyFields = canonicalStringSet(profile.Metadata.WarningOnlyFields)
	slices.Sort(normalized.Stackup.AllowedLayerCounts)
	normalized.Stackup.AllowedLayerCounts = slices.Compact(normalized.Stackup.AllowedLayerCounts)
	return normalized
}

func canonicalStringSet(values []string) []string {
	if values == nil {
		return nil
	}
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		trimmed = append(trimmed, normalized)
	}
	slices.Sort(trimmed)
	trimmed = slices.Compact(trimmed)
	return trimmed
}

func issue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "fabrication_profile." + path, Message: message}
}

func compareIssues(a, b reports.Issue) int {
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	if a.Code != b.Code {
		return strings.Compare(string(a.Code), string(b.Code))
	}
	return strings.Compare(a.Message, b.Message)
}
