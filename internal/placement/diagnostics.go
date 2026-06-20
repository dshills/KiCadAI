package placement

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/reports"
)

type PlacementDiagnosticCategory string

const (
	PlacementDiagnosticLibraryGeometry  PlacementDiagnosticCategory = "library_geometry"
	PlacementDiagnosticMissingPlacement PlacementDiagnosticCategory = "missing_placement"
	PlacementDiagnosticConstraint       PlacementDiagnosticCategory = "constraint"
	PlacementDiagnosticGrouping         PlacementDiagnosticCategory = "grouping"
	PlacementDiagnosticNetProximity     PlacementDiagnosticCategory = "net_proximity"
	PlacementDiagnosticFanout           PlacementDiagnosticCategory = "fanout"
	PlacementDiagnosticRoutingReadiness PlacementDiagnosticCategory = "routing_readiness"
	PlacementDiagnosticValidation       PlacementDiagnosticCategory = "validation"
)

type PlacementDiagnosticAction string

const (
	PlacementActionAssignCourtyardFootprint PlacementDiagnosticAction = "assign_courtyard_footprint"
	PlacementActionPlaceMissingComponents   PlacementDiagnosticAction = "place_missing_components"
	PlacementActionAdjustConstraints        PlacementDiagnosticAction = "adjust_constraints"
	PlacementActionMoveGroupTogether        PlacementDiagnosticAction = "move_group_together"
	PlacementActionReviewNetProximity       PlacementDiagnosticAction = "review_net_proximity"
	PlacementActionImproveFanout            PlacementDiagnosticAction = "improve_fanout"
	PlacementActionMoveOutOfKeepout         PlacementDiagnosticAction = "move_out_of_keepout"
	PlacementActionMoveToRegion             PlacementDiagnosticAction = "move_to_region"
	PlacementActionImproveRoutingReadiness  PlacementDiagnosticAction = "improve_routing_readiness"
	PlacementActionProceedToRouting         PlacementDiagnosticAction = "proceed_to_routing"
	PlacementActionInspectValidationIssue   PlacementDiagnosticAction = "inspect_validation_issue"
)

type PlacementDiagnostic struct {
	Category   PlacementDiagnosticCategory `json:"category"`
	Action     PlacementDiagnosticAction   `json:"action"`
	Severity   reports.Severity            `json:"severity"`
	Message    string                      `json:"message"`
	Suggestion string                      `json:"suggestion,omitempty"`
	Path       string                      `json:"path,omitempty"`
	Refs       []string                    `json:"refs,omitempty"`
	Nets       []string                    `json:"nets,omitempty"`
}

func DiagnosticsForQuality(request Request, result Result, quality QualityReport) []PlacementDiagnostic {
	request = NormalizeRequest(request)
	placementsByRef := placementResultsByRef(successfulPlacementResults(result.Placements))
	diagnostics := make([]PlacementDiagnostic, 0, len(result.Issues)+len(quality.GroupReports)+10)
	diagnostics = append(diagnostics, diagnosticsForIssues(result.Issues)...)
	if len(quality.EstimatedBoundsRefs) > 0 {
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticLibraryGeometry,
			Action:     PlacementActionAssignCourtyardFootprint,
			Severity:   reports.SeverityWarning,
			Message:    "Placement used estimated or pad-derived bounds for " + strings.Join(quality.EstimatedBoundsRefs, ", ") + ".",
			Suggestion: "Index KiCad footprint libraries and prefer footprints with valid courtyard graphics before final routing.",
			Path:       "quality.estimated_bounds_refs",
			Refs:       append([]string(nil), quality.EstimatedBoundsRefs...),
		})
	}
	if len(quality.UnplacedRefs) > 0 {
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticMissingPlacement,
			Action:     PlacementActionPlaceMissingComponents,
			Severity:   reports.SeverityBlocked,
			Message:    "Placement left components unplaced: " + strings.Join(quality.UnplacedRefs, ", ") + ".",
			Suggestion: "Increase board area, relax keepouts or spacing, or provide fixed positions for the missing components.",
			Path:       "quality.unplaced_refs",
			Refs:       append([]string(nil), quality.UnplacedRefs...),
		})
	}
	if quality.EdgeConstraintCount > quality.EdgeConstraintSatisfied {
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticConstraint,
			Action:     PlacementActionAdjustConstraints,
			Severity:   reports.SeverityWarning,
			Message:    fmt.Sprintf("Only %d of %d edge placement constraints were satisfied.", quality.EdgeConstraintSatisfied, quality.EdgeConstraintCount),
			Suggestion: "Review connector and edge-placement hints, enlarge the board, or allow a denser candidate search.",
			Path:       "quality.edge_constraint_satisfied",
		})
	}
	if quality.SideConstraintCount > quality.SideConstraintSatisfied {
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticConstraint,
			Action:     PlacementActionAdjustConstraints,
			Severity:   reports.SeverityWarning,
			Message:    fmt.Sprintf("Only %d of %d side placement constraints were satisfied.", quality.SideConstraintSatisfied, quality.SideConstraintCount),
			Suggestion: "Enable the requested board side or update component side constraints.",
			Path:       "quality.side_constraint_satisfied",
		})
	}
	for _, group := range quality.GroupReports {
		if group.MaxSpreadMM <= 0 || group.SpreadSatisfied {
			continue
		}
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticGrouping,
			Action:     PlacementActionMoveGroupTogether,
			Severity:   reports.SeverityWarning,
			Message:    fmt.Sprintf("Group %s spread %.2fmm exceeds %.2fmm.", group.ID, group.ActualSpreadMM, group.MaxSpreadMM),
			Suggestion: "Move grouped components closer, add a group anchor, or increase the requested max spread.",
			Path:       "quality.group_reports." + group.ID,
		})
	}
	diagnostics = append(diagnostics, keepoutReportDiagnostics(quality.KeepoutReports)...)
	diagnostics = append(diagnostics, proximityReportDiagnostics(quality.ProximityReports)...)
	diagnostics = append(diagnostics, regionReportDiagnostics(quality.RegionReports)...)
	diagnostics = append(diagnostics, fanoutReportDiagnostics(quality.FanoutReports)...)
	diagnostics = append(diagnostics, netReportDiagnostics(quality.NetReports)...)
	diagnostics = append(diagnostics, netProximityDiagnostics(request, placementsByRef)...)
	if quality.Ready && len(diagnostics) == 0 {
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticRoutingReadiness,
			Action:     PlacementActionProceedToRouting,
			Severity:   reports.SeverityInfo,
			Message:    "Placement is ready for routing.",
			Suggestion: "Proceed to route generation and KiCad DRC validation.",
			Path:       "quality.ready",
		})
	}
	return diagnostics
}

func diagnosticsForIssues(issues []reports.Issue) []PlacementDiagnostic {
	diagnostics := make([]PlacementDiagnostic, 0, len(issues))
	for _, issue := range issues {
		category := PlacementDiagnosticValidation
		action := PlacementActionInspectValidationIssue
		switch issue.Code {
		case reports.CodePlacementCollision, reports.CodePlacementOutsideBoard:
			category = PlacementDiagnosticConstraint
			action = PlacementActionAdjustConstraints
		}
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   category,
			Action:     action,
			Severity:   issue.Severity,
			Message:    issue.Message,
			Suggestion: placementDiagnosticSuggestion(issue.Suggestion, category),
			Path:       issue.Path,
			Refs:       append([]string(nil), issue.Refs...),
			Nets:       append([]string(nil), issue.Nets...),
		})
	}
	return diagnostics
}

func placementDiagnosticSuggestion(explicit string, category PlacementDiagnosticCategory) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	switch category {
	case PlacementDiagnosticConstraint:
		return "Move components, relax placement constraints, or enlarge the usable board area."
	default:
		return "Inspect the placement issue and update the placement request before rerunning."
	}
}

func keepoutReportDiagnostics(keepoutReports []KeepoutReport) []PlacementDiagnostic {
	diagnostics := []PlacementDiagnostic{}
	for _, report := range keepoutReports {
		severity := reports.SeverityError
		kind := "required"
		if report.Optional {
			severity = reports.SeverityWarning
			kind = "optional"
		}
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticConstraint,
			Action:     PlacementActionMoveOutOfKeepout,
			Severity:   severity,
			Message:    keepoutViolationMessage(kind, report),
			Suggestion: "Move listed refs out of the keepout, relax optional keepouts, or enlarge the board.",
			Path:       "quality.keepout_reports." + report.ID,
			Refs:       append([]string(nil), report.Refs...),
		})
	}
	return diagnostics
}

func keepoutViolationMessage(kind string, report KeepoutReport) string {
	total := len(report.Refs)
	label := "violation"
	if total != 1 {
		label = "violations"
	}
	name := report.ID
	if strings.TrimSpace(name) == "" {
		name = "unnamed"
	}
	return fmt.Sprintf("%d %s keepout %s detected for %s.", total, kind, label, name)
}

func proximityReportDiagnostics(proximityReports []ProximityReport) []PlacementDiagnostic {
	diagnostics := []PlacementDiagnostic{}
	for _, report := range proximityReports {
		if report.Satisfied {
			continue
		}
		severity := reports.SeverityWarning
		if report.Required {
			severity = reports.SeverityError
		}
		refs := append([]string{report.AnchorRef}, report.TargetRefs...)
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticNetProximity,
			Action:     PlacementActionReviewNetProximity,
			Severity:   severity,
			Message:    "Proximity rule " + report.ID + " was not satisfied.",
			Suggestion: "Move target components closer to the anchor or relax the proximity rule.",
			Path:       "quality.proximity_reports." + report.ID,
			Refs:       uniqueSortedStrings(refs),
		})
	}
	return diagnostics
}

func regionReportDiagnostics(regionReports []RegionReport) []PlacementDiagnostic {
	diagnostics := []PlacementDiagnostic{}
	for _, report := range regionReports {
		if report.Satisfied {
			continue
		}
		severity := reports.SeverityWarning
		if report.Required {
			severity = reports.SeverityError
		}
		refs := append([]string(nil), report.OutsideRefs...)
		refs = append(refs, report.MissingRefs...)
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticConstraint,
			Action:     PlacementActionMoveToRegion,
			Severity:   severity,
			Message:    "Region rule " + report.ID + " for " + report.Region + " was not satisfied.",
			Suggestion: "Move listed refs into the preferred region, provide placements for missing refs, or relax the region rule.",
			Path:       "quality.region_reports." + report.ID,
			Refs:       uniqueSortedStrings(refs),
		})
	}
	return diagnostics
}

func fanoutReportDiagnostics(fanoutReports []FanoutReport) []PlacementDiagnostic {
	var diagnostics []PlacementDiagnostic
	for _, report := range fanoutReports {
		if report.Status == scoreStatusPass {
			continue
		}
		severity := reports.SeverityWarning
		if report.Status == scoreStatusFail {
			severity = reports.SeverityError
		}
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticFanout,
			Action:     PlacementActionImproveFanout,
			Severity:   severity,
			Message:    fmt.Sprintf("Component %s fanout readiness status: %s.", report.Ref, report.Status),
			Suggestion: fanoutDiagnosticSuggestion(report),
			Path:       "quality.fanout_reports." + report.Ref,
			Refs:       []string{report.Ref},
		})
	}
	return diagnostics
}

func fanoutDiagnosticSuggestion(report FanoutReport) string {
	if strings.TrimSpace(report.SuggestedAction) != "" {
		return report.SuggestedAction
	}
	return "Increase spacing around the component, move it away from keepouts or edges, or reduce local net density before routing."
}

func netReportDiagnostics(netReports []NetQualityReport) []PlacementDiagnostic {
	diagnostics := []PlacementDiagnostic{}
	for _, report := range netReports {
		if report.Status == netStatusPass {
			continue
		}
		severity := reports.SeverityWarning
		if report.Status == netStatusFail {
			severity = reports.SeverityError
		}
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticRoutingReadiness,
			Action:     PlacementActionImproveRoutingReadiness,
			Severity:   severity,
			Message:    fmt.Sprintf("Net %s routing readiness status: %s.", report.Name, report.Status),
			Suggestion: "Place connected components closer, complete missing placements, or adjust net priorities before routing.",
			Path:       "quality.net_reports." + report.Name,
			Nets:       []string{report.Name},
		})
	}
	return diagnostics
}

func netProximityDiagnostics(request Request, placementsByRef map[string]PlacementResult) []PlacementDiagnostic {
	boardDiagonal := math.Hypot(request.Board.WidthMM, request.Board.HeightMM)
	threshold := math.Max(10, boardDiagonal*0.35)
	var diagnostics []PlacementDiagnostic
	for _, net := range request.Nets {
		if len(net.Endpoints) < 2 || net.Weight <= 1 {
			continue
		}
		refs, spread, ok := netPlacementSpread(net, placementsByRef)
		if !ok || spread <= threshold {
			continue
		}
		diagnostics = append(diagnostics, PlacementDiagnostic{
			Category:   PlacementDiagnosticNetProximity,
			Action:     PlacementActionReviewNetProximity,
			Severity:   reports.SeverityInfo,
			Message:    fmt.Sprintf("Weighted net %s spans %.2fmm after placement.", net.Name, spread),
			Suggestion: "Review component placement for this important net before routing.",
			Path:       "nets." + net.Name,
			Refs:       refs,
			Nets:       []string{net.Name},
		})
	}
	return diagnostics
}

func netPlacementSpread(net Net, placementsByRef map[string]PlacementResult) ([]string, float64, bool) {
	refs := make([]string, 0, len(net.Endpoints))
	points := make([]Point, 0, len(net.Endpoints))
	seen := map[string]struct{}{}
	for _, endpoint := range net.Endpoints {
		ref := normalizeRef(endpoint.Ref)
		if ref == "" {
			continue
		}
		placement, ok := placementsByRef[ref]
		if !ok {
			continue
		}
		if _, exists := seen[ref]; !exists {
			seen[ref] = struct{}{}
			refs = append(refs, strings.TrimSpace(endpoint.Ref))
			points = append(points, placement.Bounds.Center())
		}
	}
	if len(points) < 2 {
		return nil, 0, false
	}
	sort.Strings(refs)
	return refs, maxPointSpread(points), true
}
