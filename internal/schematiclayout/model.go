package schematiclayout

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
)

type Profile string

const (
	ProfileOff      Profile = "off"
	ProfileBasic    Profile = "basic"
	ProfileStandard Profile = "standard"
	ProfileStrict   Profile = "strict"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Sheet struct {
	Name       string
	Width      kicadfiles.IU
	Height     kicadfiles.IU
	Margin     kicadfiles.IU
	TitleBlock Rect
}

type Request struct {
	Sheet      Sheet
	Components []Component
	Nets       []Net
	Groups     []Group
	Rules      Rules
}

type Rules struct {
	Profile              Profile
	Grid                 kicadfiles.IU
	MinorGrid            kicadfiles.IU
	MinComponentSpacing  kicadfiles.IU
	MinTextSpacing       kicadfiles.IU
	MinStageSpacing      kicadfiles.IU
	MinGroupGutter       kicadfiles.IU
	LongWireThreshold    kicadfiles.IU
	MaxDiagnostics       int
	LabelFallbackEnabled bool
	// LabelFallbackConfigured distinguishes an explicit false from the zero
	// value, which inherits the profile default during normalization.
	LabelFallbackConfigured bool
}

type Component struct {
	Ref       string
	Value     string
	LibraryID string
	Role      string
	GroupID   string
	Stage     Stage
	Lane      Lane
	// FlowRank is an optional left-to-right graph rank. RankFixed distinguishes
	// an explicit rank of zero from an inferred rank.
	FlowRank  int
	RankFixed bool
	Near      []string
	Position  kicadfiles.Point
	Fixed     bool
	Rotation  kicadfiles.Angle
	Mirror    Mirror
	Body      Rect
	// BodyKnown distinguishes an intentional pin-only symbol from a missing
	// body geometry value that should use the role-based fallback.
	BodyKnown       bool
	GeometrySource  GeometrySource
	ReferenceText   TextBox
	ValueText       TextBox
	Pins            []Pin
	OriginalOrdinal int
}

// GeometrySource records the evidence used to reserve schematic space for a
// symbol. It lets diagnostics distinguish KiCad-backed geometry from a
// conservative fallback without changing the placed symbol's coordinates.
type GeometrySource string

const (
	GeometrySourceUnknown             GeometrySource = "unknown"
	GeometrySourceExplicitBody        GeometrySource = "explicit_body"
	GeometrySourceEmbeddedTemplate    GeometrySource = "embedded_template"
	GeometrySourceResolverGraphics    GeometrySource = "resolver_graphics"
	GeometrySourceResolverPinEnvelope GeometrySource = "resolver_pin_envelope"
	GeometrySourceExplicitPinEnvelope GeometrySource = "explicit_pin_envelope"
	GeometrySourceConservative        GeometrySource = "conservative"
)

type Mirror string

const (
	MirrorNone Mirror = ""
	// MirrorX follows KiCad's mirror-x transform: reflect across the X axis.
	MirrorX Mirror = "x"
	// MirrorY follows KiCad's mirror-y transform: reflect across the Y axis.
	MirrorY Mirror = "y"
)

type Pin struct {
	Number string
	Role   string
	At     kicadfiles.Point
}

type Net struct {
	Name            string
	Role            string
	Endpoints       []Endpoint
	PreferredLabels bool
	PreferDirect    bool
	OriginalOrdinal int
}

type Endpoint struct {
	Ref string
	Pin string
}

type Group struct {
	ID              string
	Role            string
	AnchorRef       string
	Stage           Stage
	Inferred        bool
	OriginalOrdinal int
}

type Stage int

const (
	StageUnknown Stage = iota
	StageBoundaryInput
	StageConditioning
	StageProcessing
	StageDriverOutput
	StageBoundaryOutput
)

type Lane int

const (
	LaneUnknown Lane = iota
	LanePositiveRail
	LaneSignal
	LaneReference
	LaneGround
	LaneNegativeRail
)

type Result struct {
	Sheet       Sheet
	Partition   *PartitionResult
	Components  []PlacedComponent
	Connections []RoutedConnection
	Wires       []WireSegment
	Labels      []Label
	Junctions   []Junction
	Diagnostics []Diagnostic
	Report      Report
}

type PlacedComponent struct {
	Component
	PlacedAt kicadfiles.Point
}

type WireSegment struct {
	NetName string
	From    kicadfiles.Point
	To      kicadfiles.Point
}

type RoutedConnection struct {
	NetName     string
	From        Endpoint
	To          Endpoint
	Points      []kicadfiles.Point
	UseLabels   bool
	FromLabelAt *kicadfiles.Point
	ToLabelAt   *kicadfiles.Point
}

type Label struct {
	NetName  string
	Text     string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
}

type Junction struct {
	Position kicadfiles.Point
}

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Ref      string   `json:"ref,omitempty"`
	NetName  string   `json:"net_name,omitempty"`
	Message  string   `json:"message"`
	Repair   string   `json:"repair,omitempty"`
}

const (
	DiagnosticWireCrossing      = "wire_crossing"
	DiagnosticWireSymbolOverlap = "wire_symbol_overlap"
	DiagnosticWirePinOverlap    = "wire_pin_overlap"
	DiagnosticTextWireOverlap   = "text_wire_overlap"
)

type Report struct {
	Profile                  Profile                `json:"profile"`
	Passed                   bool                   `json:"passed"`
	SelectedPaper            string                 `json:"selected_paper,omitempty"`
	PageEscalationCount      int                    `json:"page_escalation_count,omitempty"`
	PartitionCount           int                    `json:"partition_count,omitempty"`
	CrossSheetNetCount       int                    `json:"cross_sheet_net_count,omitempty"`
	ComponentCount           int                    `json:"component_count"`
	GroupCount               int                    `json:"group_count"`
	RoutedNetCount           int                    `json:"routed_net_count"`
	LabelFallbackCount       int                    `json:"label_fallback_count"`
	GeometrySourceCounts     map[GeometrySource]int `json:"geometry_source_counts,omitempty"`
	OverlapCounts            map[string]int         `json:"overlap_counts,omitempty"`
	DiagonalWireCount        int                    `json:"diagonal_wire_count"`
	StageOrderViolationCount int                    `json:"stage_order_violation_count"`
	PowerPlacementViolations int                    `json:"power_placement_violation_count"`
	IslandCount              int                    `json:"island_count"`
	RankCount                int                    `json:"rank_count"`
	OccupiedBounds           Rect                   `json:"occupied_bounds"`
	CenterOffset             kicadfiles.Point       `json:"center_offset"`
	DiagnosticCount          int                    `json:"diagnostic_count"`
	ErrorCount               int                    `json:"error_count"`
	WarningCount             int                    `json:"warning_count"`
}

type TextBox struct {
	Text string
	At   kicadfiles.Point
	Box  Rect
}

type Rect struct {
	MinX kicadfiles.IU
	MinY kicadfiles.IU
	MaxX kicadfiles.IU
	MaxY kicadfiles.IU
}

// PartitionResult describes the deterministic hierarchy fallback for a
// drawing that cannot fit on one standard sheet. It is layout evidence until
// the schematic writer emits the corresponding KiCad child sheets.
type PartitionResult struct {
	Sheets         []PartitionSheet `json:"sheets"`
	CrossSheetNets []CrossSheetNet  `json:"cross_sheet_nets,omitempty"`
	Complete       bool             `json:"complete"`
}

type PartitionSheet struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Components []string `json:"components"`
	Nets       []string `json:"nets,omitempty"`
	Bounds     Rect     `json:"bounds"`
}

type CrossSheetNet struct {
	Name   string   `json:"name"`
	Sheets []string `json:"sheets"`
}

func ParseProfile(value string) (Profile, error) {
	switch profile := Profile(strings.ToLower(strings.TrimSpace(value))); profile {
	case "", ProfileStandard:
		return ProfileStandard, nil
	case ProfileOff, ProfileBasic, ProfileStrict:
		return profile, nil
	default:
		return "", fmt.Errorf("unknown schematic layout profile %q", value)
	}
}

func DefaultRules(profile Profile) Rules {
	if profile == "" {
		profile = ProfileStandard
	}
	return Rules{
		Profile:                 profile,
		Grid:                    kicadfiles.MM(2.54),
		MinorGrid:               kicadfiles.MM(1.27),
		MinComponentSpacing:     kicadfiles.MM(10.16),
		MinTextSpacing:          kicadfiles.MM(2.54),
		MinStageSpacing:         kicadfiles.MM(25.4),
		MinGroupGutter:          kicadfiles.MM(12.7),
		LongWireThreshold:       kicadfiles.MM(80),
		MaxDiagnostics:          100,
		LabelFallbackEnabled:    profile != ProfileOff,
		LabelFallbackConfigured: true,
	}
}

func NormalizeRequest(request Request) Request {
	request.Rules = normalizeRules(request.Rules)
	request.Components = append([]Component(nil), request.Components...)
	request.Nets = append([]Net(nil), request.Nets...)
	request.Groups = append([]Group(nil), request.Groups...)
	for index := range request.Components {
		request.Components[index].Pins = append([]Pin(nil), request.Components[index].Pins...)
		request.Components[index].Near = append([]string(nil), request.Components[index].Near...)
		sort.Strings(request.Components[index].Near)
		sort.SliceStable(request.Components[index].Pins, func(i, j int) bool {
			return comparePins(request.Components[index].Pins[i], request.Components[index].Pins[j]) < 0
		})
	}
	for index := range request.Nets {
		request.Nets[index].Endpoints = append([]Endpoint(nil), request.Nets[index].Endpoints...)
		sort.SliceStable(request.Nets[index].Endpoints, func(i, j int) bool {
			return compareEndpoints(request.Nets[index].Endpoints[i], request.Nets[index].Endpoints[j]) < 0
		})
	}
	sort.SliceStable(request.Components, func(i, j int) bool {
		return compareComponents(request.Components[i], request.Components[j]) < 0
	})
	sort.SliceStable(request.Nets, func(i, j int) bool {
		return compareNets(request.Nets[i], request.Nets[j]) < 0
	})
	sort.SliceStable(request.Groups, func(i, j int) bool {
		return compareGroups(request.Groups[i], request.Groups[j]) < 0
	})
	return request
}

func NormalizeResult(result Result, rules Rules) Result {
	rules = normalizeRules(rules)
	result.Components = append([]PlacedComponent(nil), result.Components...)
	result.Connections = append([]RoutedConnection(nil), result.Connections...)
	result.Wires = append([]WireSegment(nil), result.Wires...)
	result.Labels = append([]Label(nil), result.Labels...)
	result.Junctions = append([]Junction(nil), result.Junctions...)
	result.Diagnostics = append([]Diagnostic(nil), result.Diagnostics...)
	for index := range result.Components {
		result.Components[index].Pins = append([]Pin(nil), result.Components[index].Pins...)
		sort.SliceStable(result.Components[index].Pins, func(i, j int) bool {
			return comparePins(result.Components[index].Pins[i], result.Components[index].Pins[j]) < 0
		})
	}
	for index := range result.Connections {
		result.Connections[index].Points = append([]kicadfiles.Point(nil), result.Connections[index].Points...)
		if result.Connections[index].FromLabelAt != nil {
			point := *result.Connections[index].FromLabelAt
			result.Connections[index].FromLabelAt = &point
		}
		if result.Connections[index].ToLabelAt != nil {
			point := *result.Connections[index].ToLabelAt
			result.Connections[index].ToLabelAt = &point
		}
	}
	for index := range result.Wires {
		if comparePoints(result.Wires[index].From, result.Wires[index].To) > 0 {
			result.Wires[index].From, result.Wires[index].To = result.Wires[index].To, result.Wires[index].From
		}
	}
	sort.SliceStable(result.Components, func(i, j int) bool {
		return compareComponents(result.Components[i].Component, result.Components[j].Component) < 0
	})
	sort.SliceStable(result.Wires, func(i, j int) bool {
		return compareWires(result.Wires[i], result.Wires[j]) < 0
	})
	sort.SliceStable(result.Connections, func(i, j int) bool {
		return compareRoutedConnections(result.Connections[i], result.Connections[j]) < 0
	})
	sort.SliceStable(result.Labels, func(i, j int) bool {
		return compareLabels(result.Labels[i], result.Labels[j]) < 0
	})
	sort.SliceStable(result.Junctions, func(i, j int) bool {
		return comparePoints(result.Junctions[i].Position, result.Junctions[j].Position) < 0
	})
	result.Diagnostics = NormalizeDiagnostics(result.Diagnostics, rules.MaxDiagnostics)
	result.Report = BuildReport(result, rules.Profile)
	return result
}

func compareRoutedConnections(first, second RoutedConnection) int {
	if value := strings.Compare(first.NetName, second.NetName); value != 0 {
		return value
	}
	if value := compareEndpoints(first.From, second.From); value != 0 {
		return value
	}
	return compareEndpoints(first.To, second.To)
}

func NormalizeDiagnostics(diagnostics []Diagnostic, limit int) []Diagnostic {
	diagnostics = append([]Diagnostic(nil), diagnostics...)
	diagnostics = enrichDiagnosticRepairs(diagnostics)
	sort.SliceStable(diagnostics, func(i, j int) bool {
		return compareDiagnostics(diagnostics[i], diagnostics[j]) < 0
	})
	if limit > 0 && len(diagnostics) > limit {
		return append([]Diagnostic(nil), diagnostics[:limit]...)
	}
	return diagnostics
}

func enrichDiagnosticRepairs(diagnostics []Diagnostic) []Diagnostic {
	repairs := map[string]string{}
	for index := range diagnostics {
		if diagnostics[index].Repair != "" {
			continue
		}
		if repair, ok := repairs[diagnostics[index].Code]; ok {
			diagnostics[index].Repair = repair
			continue
		}
		repair := ""
		if rule, ok := RuleForDiagnostic(diagnostics[index].Code); ok {
			repair = rule.Repair
		}
		repairs[diagnostics[index].Code] = repair
		diagnostics[index].Repair = repair
	}
	return diagnostics
}

func BuildReport(result Result, profile Profile) Report {
	report := Report{
		Profile:              profile,
		Passed:               true,
		SelectedPaper:        result.Report.SelectedPaper,
		PageEscalationCount:  result.Report.PageEscalationCount,
		PartitionCount:       result.Report.PartitionCount,
		CrossSheetNetCount:   result.Report.CrossSheetNetCount,
		ComponentCount:       len(result.Components),
		GroupCount:           countGroups(result.Components),
		RoutedNetCount:       countRoutedNets(result.Wires),
		LabelFallbackCount:   len(result.Labels),
		GeometrySourceCounts: map[GeometrySource]int{},
		OverlapCounts:        map[string]int{},
		DiagnosticCount:      len(result.Diagnostics),
		IslandCount:          result.Report.IslandCount,
		RankCount:            result.Report.RankCount,
		OccupiedBounds:       result.Report.OccupiedBounds,
		CenterOffset:         result.Report.CenterOffset,
	}
	for _, wire := range result.Wires {
		if wire.From.X != wire.To.X && wire.From.Y != wire.To.Y {
			report.DiagonalWireCount++
		}
	}
	for _, component := range result.Components {
		source := component.GeometrySource
		if source == "" {
			source = GeometrySourceUnknown
		}
		report.GeometrySourceCounts[source]++
	}
	if len(report.GeometrySourceCounts) == 0 {
		report.GeometrySourceCounts = nil
	}
	for _, diagnostic := range result.Diagnostics {
		switch diagnostic.Severity {
		case SeverityError:
			report.ErrorCount++
		case SeverityWarning:
			report.WarningCount++
		}
		switch diagnostic.Code {
		case "stage_order":
			report.StageOrderViolationCount++
		case "power_placement":
			report.PowerPlacementViolations++
		case "symbol_overlap", "text_symbol_overlap", "text_wire_overlap", "label_overlap", "wire_symbol_overlap", "wire_crossing", DiagnosticWirePinOverlap:
			report.OverlapCounts[diagnostic.Code]++
		}
	}
	if len(report.OverlapCounts) == 0 {
		report.OverlapCounts = nil
	}
	report.Passed = report.ErrorCount == 0 && (profile != ProfileStrict || report.WarningCount == 0)
	return report
}

func SnapPoint(point kicadfiles.Point, grid kicadfiles.IU) kicadfiles.Point {
	return kicadfiles.Point{X: SnapIU(point.X, grid), Y: SnapIU(point.Y, grid)}
}

func SnapIU(value kicadfiles.IU, grid kicadfiles.IU) kicadfiles.IU {
	if grid <= 0 {
		return value
	}
	remainder := value % grid
	if remainder == 0 {
		return value
	}
	down := value - remainder
	up := down + grid
	if value < 0 {
		up = down - grid
	}
	if closerTo(value, down, up) < 0 {
		return down
	}
	return up
}

func normalizeRules(rules Rules) Rules {
	defaults := DefaultRules(rules.Profile)
	rules.Profile = defaults.Profile
	if rules.Grid <= 0 {
		rules.Grid = defaults.Grid
	}
	if rules.MinorGrid <= 0 {
		rules.MinorGrid = defaults.MinorGrid
	}
	if rules.MinComponentSpacing <= 0 {
		rules.MinComponentSpacing = defaults.MinComponentSpacing
	}
	if rules.MinTextSpacing <= 0 {
		rules.MinTextSpacing = defaults.MinTextSpacing
	}
	if rules.MinStageSpacing <= 0 {
		rules.MinStageSpacing = defaults.MinStageSpacing
	}
	if rules.MinGroupGutter <= 0 {
		rules.MinGroupGutter = defaults.MinGroupGutter
	}
	if rules.LongWireThreshold <= 0 {
		rules.LongWireThreshold = defaults.LongWireThreshold
	}
	if rules.MaxDiagnostics < 0 {
		rules.MaxDiagnostics = defaults.MaxDiagnostics
	}
	if !rules.LabelFallbackConfigured {
		rules.LabelFallbackEnabled = defaults.LabelFallbackEnabled
	}
	return rules
}

func compareComponents(first, second Component) int {
	if first.Stage != second.Stage {
		return compareInts(int(first.Stage), int(second.Stage))
	}
	if first.Lane != second.Lane {
		return compareInts(int(first.Lane), int(second.Lane))
	}
	if cmp := strings.Compare(first.GroupID, second.GroupID); cmp != 0 {
		return cmp
	}
	if cmp := compareInts(first.OriginalOrdinal, second.OriginalOrdinal); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Ref, second.Ref); cmp != 0 {
		return cmp
	}
	return 0
}

func compareNets(first, second Net) int {
	if cmp := compareInts(first.OriginalOrdinal, second.OriginalOrdinal); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Role, second.Role); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Name, second.Name); cmp != 0 {
		return cmp
	}
	return 0
}

func compareGroups(first, second Group) int {
	if first.Stage != second.Stage {
		return compareInts(int(first.Stage), int(second.Stage))
	}
	if cmp := compareInts(first.OriginalOrdinal, second.OriginalOrdinal); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.ID, second.ID); cmp != 0 {
		return cmp
	}
	return 0
}

func compareWires(first, second WireSegment) int {
	if cmp := strings.Compare(first.NetName, second.NetName); cmp != 0 {
		return cmp
	}
	if cmp := comparePoints(first.From, second.From); cmp != 0 {
		return cmp
	}
	if cmp := comparePoints(first.To, second.To); cmp != 0 {
		return cmp
	}
	return 0
}

func compareLabels(first, second Label) int {
	if cmp := strings.Compare(first.NetName, second.NetName); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Text, second.Text); cmp != 0 {
		return cmp
	}
	if cmp := comparePoints(first.Position, second.Position); cmp != 0 {
		return cmp
	}
	return 0
}

func compareDiagnostics(first, second Diagnostic) int {
	if cmp := compareInts(severityRank(first.Severity), severityRank(second.Severity)); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Code, second.Code); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Ref, second.Ref); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.NetName, second.NetName); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Message, second.Message); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Repair, second.Repair); cmp != 0 {
		return cmp
	}
	return 0
}

func comparePins(first, second Pin) int {
	if cmp := strings.Compare(first.Number, second.Number); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(first.Role, second.Role); cmp != 0 {
		return cmp
	}
	return comparePoints(first.At, second.At)
}

func compareEndpoints(first, second Endpoint) int {
	if cmp := strings.Compare(first.Ref, second.Ref); cmp != 0 {
		return cmp
	}
	return strings.Compare(first.Pin, second.Pin)
}

func comparePoints(first, second kicadfiles.Point) int {
	if first.X != second.X {
		if first.X < second.X {
			return -1
		}
		return 1
	}
	if first.Y != second.Y {
		if first.Y < second.Y {
			return -1
		}
		return 1
	}
	return 0
}

func countGroups(components []PlacedComponent) int {
	seen := map[string]bool{}
	for _, component := range components {
		if component.GroupID != "" {
			seen[component.GroupID] = true
		}
	}
	return len(seen)
}

func countRoutedNets(wires []WireSegment) int {
	seen := map[string]bool{}
	for _, wire := range wires {
		if wire.NetName != "" {
			seen[wire.NetName] = true
		}
	}
	return len(seen)
}

func closerTo(target, first, second kicadfiles.IU) int {
	firstDelta := target - first
	if firstDelta < 0 {
		firstDelta = -firstDelta
	}
	secondDelta := target - second
	if secondDelta < 0 {
		secondDelta = -secondDelta
	}
	if firstDelta < secondDelta {
		return -1
	}
	if firstDelta > secondDelta {
		return 1
	}
	if first < second {
		return -1
	}
	if first > second {
		return 1
	}
	return 0
}

func compareInts(first, second int) int {
	if first < second {
		return -1
	}
	if first > second {
		return 1
	}
	return 0
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}
