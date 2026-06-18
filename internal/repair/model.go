package repair

import (
	"strings"

	"kicadai/internal/reports"
)

type Status string

const (
	StatusNotNeeded Status = "not_needed"
	StatusRepaired  Status = "repaired"
	StatusPartial   Status = "partial"
	StatusBlocked   Status = "blocked"
	StatusSkipped   Status = "skipped"
)

type Category string

const (
	CategoryMissingFootprint     Category = "missing_footprint"
	CategoryUnknownSymbol        Category = "unknown_symbol"
	CategoryInvalidNetAssignment Category = "invalid_net_assignment"
	CategoryDisconnectedPad      Category = "disconnected_pad"
	CategoryUnroutedNet          Category = "unrouted_net"
	CategoryRouteClearance       Category = "route_clearance"
	CategoryMissingBoardOutline  Category = "missing_board_outline"
	CategoryZoneUnfilled         Category = "zone_unfilled"
	CategoryZoneWrongNet         Category = "zone_wrong_net"
	CategoryPlacementCollision   Category = "placement_collision"
	CategoryPlacementOutside     Category = "placement_outside_board"
	CategoryRoundTripDiff        Category = "roundtrip_diff"
	CategoryKiCadCLIUnavailable  Category = "kicad_cli_unavailable"
	CategoryUnsupportedObject    Category = "unsupported_object"
	CategoryUnsafeUserContent    Category = "unsafe_user_content"
	CategoryUnknown              Category = "unknown"
)

type Action string

const (
	ActionAssignFootprint    Action = "assign_footprint"
	ActionRegeneratePadNets  Action = "regenerate_pad_net_hints"
	ActionRerouteNet         Action = "reroute_net"
	ActionRetryPlacement     Action = "retry_placement"
	ActionGenerateOutline    Action = "generate_outline"
	ActionRequireKiCadRefill Action = "require_kicad_refill"
	ActionRepairZoneNet      Action = "repair_zone_net"
	ActionUnsupported        Action = "unsupported"
	ActionNoop               Action = "noop"
)

type Options struct {
	Enabled                  bool   `json:"enabled,omitempty"`
	Apply                    bool   `json:"apply,omitempty"`
	MaxAttempts              int    `json:"max_attempts,omitempty"`
	MaxAttemptsPerIssue      int    `json:"max_attempts_per_issue,omitempty"`
	AllowPlacementRetry      bool   `json:"allow_placement_retry,omitempty"`
	AllowRoutingRetry        bool   `json:"allow_routing_retry,omitempty"`
	AllowFootprintAssignment bool   `json:"allow_footprint_assignment,omitempty"`
	AllowOutlineGeneration   bool   `json:"allow_outline_generation,omitempty"`
	AllowKiCadCLI            bool   `json:"allow_kicad_cli,omitempty"`
	Acceptance               string `json:"acceptance,omitempty"`
}

type Result struct {
	Status      Status             `json:"status"`
	Attempts    []Attempt          `json:"attempts,omitempty"`
	FinalIssues []reports.Issue    `json:"final_issues,omitempty"`
	Artifacts   []reports.Artifact `json:"artifacts,omitempty"`
	Summary     Summary            `json:"summary"`
}

type Summary struct {
	AttemptCount  int `json:"attempt_count"`
	AppliedCount  int `json:"applied_count"`
	SkippedCount  int `json:"skipped_count"`
	BlockedCount  int `json:"blocked_count"`
	RepairedCount int `json:"repaired_count"`
}

type Attempt struct {
	Number       int             `json:"number"`
	Stage        string          `json:"stage,omitempty"`
	Issue        reports.Issue   `json:"issue"`
	Category     Category        `json:"category"`
	Action       Action          `json:"action"`
	Status       Status          `json:"status"`
	DryRun       bool            `json:"dry_run"`
	Message      string          `json:"message,omitempty"`
	Operations   []string        `json:"operations,omitempty"`
	BeforeIssues int             `json:"before_issues,omitempty"`
	AfterIssues  int             `json:"after_issues,omitempty"`
	Issues       []reports.Issue `json:"issues,omitempty"`
}

type Plan struct {
	Status   Status    `json:"status"`
	Options  Options   `json:"options"`
	Attempts []Attempt `json:"attempts,omitempty"`
	Summary  Summary   `json:"summary"`
}

type StageIssues struct {
	Stage  string          `json:"stage"`
	Issues []reports.Issue `json:"issues"`
}

type Classification struct {
	Category   Category `json:"category"`
	Repairable bool     `json:"repairable"`
	Reason     string   `json:"reason,omitempty"`
}

func DefaultOptions() Options {
	return Options{
		MaxAttempts:         3,
		MaxAttemptsPerIssue: 1,
	}
}

func Classify(issue reports.Issue) Classification {
	text := strings.ToLower(strings.TrimSpace(issue.Path + " " + issue.Message + " " + issue.Suggestion))
	switch issue.Code {
	case reports.CodeMissingFootprint, reports.CodeUnknownFootprintLibrary:
		return repairable(CategoryMissingFootprint)
	case reports.CodeUnknownSymbolLibrary:
		return repairable(CategoryUnknownSymbol)
	case reports.CodeInvalidNetAssignment:
		if containsAny(text, "zone") {
			return repairable(CategoryZoneWrongNet)
		}
		return repairable(CategoryInvalidNetAssignment)
	case reports.CodeDisconnectedPad:
		if containsAny(text, "unrouted", "not routed", "route does not connect") {
			return repairable(CategoryUnroutedNet)
		}
		return repairable(CategoryDisconnectedPad)
	case reports.CodeMissingBoardOutline:
		return repairable(CategoryMissingBoardOutline)
	case reports.CodePlacementCollision:
		return repairable(CategoryPlacementCollision)
	case reports.CodePlacementOutsideBoard:
		return repairable(CategoryPlacementOutside)
	case reports.CodeKiCadCLIFailed, reports.CodeSkippedExternalTool:
		return blocked(CategoryKiCadCLIUnavailable, "requires KiCad CLI availability")
	case reports.CodeRoundTripDiff:
		return blocked(CategoryRoundTripDiff, "round-trip diffs require preservation-aware handling")
	case reports.CodeUnsupportedImportedObject, reports.CodeUnsupportedOperation:
		return blocked(CategoryUnsupportedObject, "unsupported object cannot be repaired safely")
	case reports.CodeUnsafeRemove, reports.CodePreservationConflict:
		return blocked(CategoryUnsafeUserContent, "unsafe user-authored content cannot be repaired automatically")
	}
	switch {
	case containsAny(text, "clearance"):
		return repairable(CategoryRouteClearance)
	case containsAny(text, "unrouted", "not routed"):
		return repairable(CategoryUnroutedNet)
	case containsAny(text, "zone") && containsAny(text, "unfilled", "fill"):
		return repairable(CategoryZoneUnfilled)
	case containsAny(text, "zone") && containsAny(text, "wrong net", "net name does not match"):
		return repairable(CategoryZoneWrongNet)
	case containsAny(text, "unknown symbol", "missing symbol"):
		return repairable(CategoryUnknownSymbol)
	default:
		return blocked(CategoryUnknown, "no deterministic repair is registered")
	}
}

func repairable(category Category) Classification {
	return Classification{Category: category, Repairable: true}
}

func blocked(category Category, reason string) Classification {
	return Classification{Category: category, Repairable: false, Reason: reason}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
