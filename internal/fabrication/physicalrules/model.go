package physicalrules

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

const ReportSchema = "kicadai.fabrication.physical_rules.v1"

type Status string

const (
	StatusPass    Status = "pass"
	StatusWarning Status = "warning"
	StatusBlocked Status = "blocked"
	StatusSkipped Status = "skipped"
)

type Category string

const (
	CategoryStackup          Category = "stackup"
	CategoryNetClass         Category = "net_class"
	CategorySolderMask       Category = "solder_mask"
	CategorySolderPaste      Category = "solder_paste"
	CategoryEdgeCuts         Category = "edge_cuts"
	CategoryCourtyard        Category = "courtyard"
	CategoryAnnularRing      Category = "annular_ring"
	CategoryCopperSliver     Category = "copper_sliver"
	CategoryEdgePlating      Category = "edge_plating"
	CategoryImpedance        Category = "impedance"
	CategoryDifferentialPair Category = "differential_pair"
	CategoryFabMetadata      Category = "fabrication_metadata"
	CategorySilkscreen       Category = "silkscreen"
	CategoryMountingHole     Category = "mounting_hole"
)

type Source string

const (
	SourceWriter          Source = "writer"
	SourceParser          Source = "parser"
	SourceProfile         Source = "profile"
	SourceBoardValidation Source = "board_validation"
	SourceKiCadDRC        Source = "kicad_drc"
	SourceHeuristic       Source = "heuristic"
)

const (
	statusRankSkipped = iota
	statusRankPass
	statusRankWarning
	statusRankBlocked
)

const (
	CheckStackupCopperLayers        = "physical.stackup.copper_layers"
	CheckStackupThickness           = "physical.stackup.thickness"
	CheckStackupSolderMask          = "physical.stackup.solder_mask"
	CheckNetClassDefault            = "physical.net_class.default"
	CheckNetClassEffectiveRules     = "physical.net_class.effective_rules"
	CheckNetClassRoutedWidth        = "physical.net_class.routed_width"
	CheckNetClassViaDimensions      = "physical.net_class.via_dimensions"
	CheckNetClassAssignmentCoverage = "physical.net_class.assignment_coverage"
	CheckSolderMaskPadLayers        = "physical.solder_mask.pad_layers"
	CheckSolderMaskArtifacts        = "physical.solder_mask.artifacts"
	CheckSolderPastePadLayers       = "physical.solder_paste.pad_layers"
	CheckSolderPasteArtifacts       = "physical.solder_paste.artifacts"
	CheckEdgeCutsOutline            = "physical.edge_cuts.outline"
	CheckEdgeCutsContainment        = "physical.edge_cuts.containment"
	CheckCourtyardPresence          = "physical.courtyard.presence"
	CheckCourtyardOverlap           = "physical.courtyard.overlap"
	CheckSilkscreenPadClearance     = "physical.silkscreen.pad_clearance"
	CheckSilkscreenBoardClearance   = "physical.silkscreen.board_clearance"
	CheckSilkscreenReference        = "physical.silkscreen.reference"
	CheckMountingHolePresence       = "physical.mounting_hole.presence"
	CheckMountingHoleGeometry       = "physical.mounting_hole.geometry"
	CheckMountingHoleEdgeClearance  = "physical.mounting_hole.edge_clearance"
	CheckAnnularRingPlatedPad       = "physical.annular_ring.plated_pad"
	CheckAnnularRingVia             = "physical.annular_ring.via"
	CheckAnnularRingProfile         = "physical.annular_ring.profile_threshold"
	CheckCopperSliverTrackWidth     = "physical.copper_sliver.track_width"
	CheckCopperSliverZoneMinWidth   = "physical.copper_sliver.zone_min_width"
	CheckCopperSliverUnsupported    = "physical.copper_sliver.unsupported_geometry"
	CheckSolderMaskWebWidth         = "physical.solder_mask.web_width"
	CheckSolderMaskPadExpansion     = "physical.solder_mask.pad_expansion"
	CheckSolderMaskUnsupported      = "physical.solder_mask.unsupported_geometry"
	CheckEdgePlatingCastellation    = "physical.edge_plating.castellation_detected"
	CheckEdgePlatingProfile         = "physical.edge_plating.profile_support"
	CheckEdgePlatingContact         = "physical.edge_plating.edge_contact"
	CheckImpedanceStackupEvidence   = "physical.impedance.stackup_evidence"
	CheckImpedanceWidthGapEvidence  = "physical.impedance.width_gap_evidence"
	CheckDiffPairFabrication        = "physical.differential_pair.fabrication_evidence"
	CheckFabMetadataBoardFinish     = "physical.fabrication_metadata.board_finish"
	CheckFabMetadataPanelization    = "physical.fabrication_metadata.panelization"
	CheckFabMetadataNotes           = "physical.fabrication_metadata.fabrication_notes"
)

type Policy string

const (
	PolicyIgnore Policy = "ignore"
	PolicyWarn   Policy = "warn"
	PolicyBlock  Policy = "block"
	PolicyAllow  Policy = "allow"
)

type BoardRef struct {
	Path       string `json:"path,omitempty"`
	LayerCount int    `json:"layer_count,omitempty"`
}

type Summary struct {
	PassCount    int                 `json:"pass_count"`
	WarningCount int                 `json:"warning_count"`
	BlockedCount int                 `json:"blocked_count"`
	SkippedCount int                 `json:"skipped_count"`
	Categories   map[Category]Status `json:"categories,omitempty"`
}

type Measurement struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}

type Evidence struct {
	Kind string `json:"kind,omitempty"`
	Path string `json:"path,omitempty"`
	Note string `json:"note,omitempty"`
}

type Check struct {
	ID           string           `json:"id"`
	Category     Category         `json:"category"`
	Status       Status           `json:"status"`
	Severity     reports.Severity `json:"severity,omitempty"`
	Message      string           `json:"message"`
	Suggestion   string           `json:"suggestion,omitempty"`
	References   []string         `json:"references,omitempty"`
	Nets         []string         `json:"nets,omitempty"`
	Layers       []string         `json:"layers,omitempty"`
	Objects      []string         `json:"objects,omitempty"`
	Measurements []Measurement    `json:"measurements,omitempty"`
	Source       Source           `json:"source,omitempty"`
	Evidence     []Evidence       `json:"evidence,omitempty"`
	IssueCode    reports.Code     `json:"issue_code,omitempty"`
	IssuePath    string           `json:"issue_path,omitempty"`
	Issue        *reports.Issue   `json:"-"`
}

type Report struct {
	Schema   string          `json:"schema"`
	Status   Status          `json:"status"`
	Profile  string          `json:"profile,omitempty"`
	Board    BoardRef        `json:"board,omitempty"`
	Summary  Summary         `json:"summary"`
	Checks   []Check         `json:"checks"`
	Issues   []reports.Issue `json:"issues,omitempty"`
	Evidence []Evidence      `json:"evidence,omitempty"`
}

type Options struct {
	ProfileID                 string  `json:"profile_id,omitempty"`
	Strict                    bool    `json:"strict,omitempty"`
	RequireCourtyard          bool    `json:"require_courtyard,omitempty"`
	RequireMountingHoles      bool    `json:"require_mounting_holes,omitempty"`
	MinCopperEdgeMM           float64 `json:"min_copper_edge_mm,omitempty"`
	MinHoleEdgeMM             float64 `json:"min_hole_edge_mm,omitempty"`
	MinCourtyardSpacingMM     float64 `json:"min_courtyard_spacing_mm,omitempty"`
	MinSilkPadClearanceMM     float64 `json:"min_silk_pad_clearance_mm,omitempty"`
	MinSilkEdgeClearanceMM    float64 `json:"min_silk_edge_clearance_mm,omitempty"`
	MinPlatedPadAnnularRingMM float64 `json:"min_plated_pad_annular_ring_mm,omitempty"`
	MinViaRingMM              float64 `json:"min_via_annular_ring_mm,omitempty"`
	MinCopperFeatureMM        float64 `json:"min_copper_feature_mm,omitempty"`
	MinSolderMaskWebMM        float64 `json:"min_solder_mask_web_mm,omitempty"`
	EdgePlatingPolicy         Policy  `json:"edge_plating_policy,omitempty"`
	RequireBoardFinish        bool    `json:"require_board_finish,omitempty"`
	RequireFabricationNotes   bool    `json:"require_fabrication_notes,omitempty"`
	ImpedancePolicy           Policy  `json:"controlled_impedance_policy,omitempty"`
	PanelizationPolicy        Policy  `json:"panelization_policy,omitempty"`
}

const (
	defaultMinPlatedPadRingMM = 0.15
	defaultMinViaRingMM       = 0.10
	defaultMinCopperFeatureMM = 0.127
	defaultMinSolderMaskWebMM = 0.10
)

func NormalizeOptions(opts Options) Options {
	// Thresholds use zero as "unset"; use the related Policy fields to disable
	// optional checks instead of expressing a zero physical minimum.
	if opts.MinPlatedPadAnnularRingMM <= 0 {
		opts.MinPlatedPadAnnularRingMM = defaultMinPlatedPadRingMM
	}
	if opts.MinViaRingMM <= 0 {
		opts.MinViaRingMM = defaultMinViaRingMM
	}
	if opts.MinCopperFeatureMM <= 0 {
		opts.MinCopperFeatureMM = defaultMinCopperFeatureMM
	}
	if opts.MinSolderMaskWebMM <= 0 {
		opts.MinSolderMaskWebMM = defaultMinSolderMaskWebMM
	}
	opts.EdgePlatingPolicy = normalizePolicy(opts.EdgePlatingPolicy, PolicyWarn)
	opts.ImpedancePolicy = normalizePolicy(opts.ImpedancePolicy, PolicyWarn)
	opts.PanelizationPolicy = normalizePolicy(opts.PanelizationPolicy, PolicyIgnore)
	return opts
}

func normalizePolicy(policy Policy, fallback Policy) Policy {
	switch policy {
	case PolicyIgnore, PolicyWarn, PolicyBlock, PolicyAllow:
		return policy
	default:
		return fallback
	}
}

func NewReport(profile string, board BoardRef, checks []Check) Report {
	report := Report{
		Schema:  ReportSchema,
		Profile: strings.TrimSpace(profile),
		Board:   board,
		Checks:  checks,
	}
	return Normalize(report)
}

func Normalize(report Report) Report {
	if strings.TrimSpace(report.Schema) == "" {
		report.Schema = ReportSchema
	}
	report.Profile = strings.TrimSpace(report.Profile)
	report.Checks = slices.Clone(report.Checks)
	report.Evidence = slices.Clone(report.Evidence)
	issues := slices.Clone(report.Issues)
	summary := Summary{Categories: map[Category]Status{}}
	status := StatusSkipped
	for index := range report.Checks {
		check := normalizeCheck(report.Checks[index])
		report.Checks[index] = check
		status = worstStatus(status, check.Status)
		summary.Categories[check.Category] = worstStatus(summary.Categories[check.Category], check.Status)
		switch check.Status {
		case StatusPass:
			summary.PassCount++
		case StatusWarning:
			summary.WarningCount++
		case StatusBlocked:
			summary.BlockedCount++
		case StatusSkipped:
			summary.SkippedCount++
		}
		if check.Issue != nil {
			issues = append(issues, *check.Issue)
		} else if issue, ok := IssueForCheck(check); ok {
			issues = append(issues, issue)
		}
	}
	slices.SortFunc(report.Checks, compareChecks)
	issues = dedupeIssues(issues)
	slices.SortFunc(issues, compareIssues)
	report.Status = status
	report.Summary = summary
	report.Issues = issues
	return report
}

type reportAlias Report

func (report Report) MarshalJSON() ([]byte, error) {
	normalized := Normalize(report)
	data, err := json.Marshal(reportAlias(normalized))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func MarshalReportJSON(report Report) ([]byte, error) {
	normalized := Normalize(report)
	data, err := json.MarshalIndent(reportAlias(normalized), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func IssueForCheck(check Check) (reports.Issue, bool) {
	if check.Status != StatusWarning && check.Status != StatusBlocked {
		return reports.Issue{}, false
	}
	severity := check.Severity
	if severity == "" {
		if check.Status == StatusBlocked {
			severity = reports.SeverityError
		} else {
			severity = reports.SeverityWarning
		}
	}
	code := check.IssueCode
	if code == "" {
		code = reports.CodeValidationFailed
	}
	path := strings.TrimSpace(check.IssuePath)
	if path == "" {
		path = check.ID
	}
	if path == "" {
		path = "physical.unknown"
	}
	return reports.Issue{
		Code:       code,
		Severity:   severity,
		Path:       path,
		Message:    check.Message,
		Refs:       slices.Clone(check.References),
		Nets:       slices.Clone(check.Nets),
		UUIDs:      slices.Clone(check.Objects),
		Suggestion: check.Suggestion,
	}, true
}

func normalizeCheck(check Check) Check {
	check.ID = strings.TrimSpace(check.ID)
	check.Message = strings.TrimSpace(check.Message)
	if check.Message == "" {
		check.Message = "physical fabrication rule check did not provide a message"
	}
	check.Suggestion = strings.TrimSpace(check.Suggestion)
	check.IssuePath = strings.TrimSpace(check.IssuePath)
	if check.Status == "" {
		check.Status = StatusPass
	}
	check.References = cleanStrings(check.References)
	check.Nets = cleanStrings(check.Nets)
	check.Layers = cleanStrings(check.Layers)
	check.Objects = cleanStrings(check.Objects)
	check.Measurements = slices.Clone(check.Measurements)
	check.Evidence = slices.Clone(check.Evidence)
	return check
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func worstStatus(a, b Status) Status {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if statusRank(b) > statusRank(a) {
		return b
	}
	return a
}

func statusRank(status Status) int {
	switch status {
	case StatusBlocked:
		return statusRankBlocked
	case StatusWarning:
		return statusRankWarning
	case StatusPass:
		return statusRankPass
	case StatusSkipped, "":
		return statusRankSkipped
	default:
		// Unknown statuses are treated as blocking so new or malformed states
		// cannot accidentally pass fabrication readiness.
		return statusRankBlocked
	}
}

func compareChecks(a, b Check) int {
	if a.Category != b.Category {
		if a.Category < b.Category {
			return -1
		}
		return 1
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return strings.Compare(a.Message, b.Message)
}

func compareIssues(a, b reports.Issue) int {
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	if a.Message != b.Message {
		return strings.Compare(a.Message, b.Message)
	}
	return strings.Compare(string(a.Severity), string(b.Severity))
}

func dedupeIssues(issues []reports.Issue) []reports.Issue {
	if len(issues) < 2 {
		return issues
	}
	out := make([]reports.Issue, 0, len(issues))
	seen := map[string]struct{}{}
	for _, issue := range issues {
		issue.Refs = cleanStrings(issue.Refs)
		issue.Nets = cleanStrings(issue.Nets)
		issue.UUIDs = cleanStrings(issue.UUIDs)
		key := issueKey(issue)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, issue)
	}
	return out
}

func issueKey(issue reports.Issue) string {
	var builder strings.Builder
	appendKeyPart(&builder, string(issue.Code))
	appendKeyPart(&builder, string(issue.Severity))
	appendKeyPart(&builder, issue.Path)
	appendKeyPart(&builder, issue.Message)
	appendKeyList(&builder, issue.Refs)
	appendKeyList(&builder, issue.Nets)
	appendKeyList(&builder, issue.UUIDs)
	return builder.String()
}

func appendKeyList(builder *strings.Builder, values []string) {
	appendKeyPart(builder, "")
	for _, value := range values {
		appendKeyPart(builder, value)
	}
}

func appendKeyPart(builder *strings.Builder, value string) {
	builder.WriteString(strconv.Itoa(len(value)))
	builder.WriteByte(':')
	builder.WriteString(value)
}
