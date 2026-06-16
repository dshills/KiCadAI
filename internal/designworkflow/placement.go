package designworkflow

import (
	"context"
	"strings"
	"unicode"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

var defaultWorkflowBounds = placement.Bounds{WidthMM: 2.0, HeightMM: 1.25, Source: placement.BoundsEstimated}

type PlacementOptions struct {
	DefaultBounds placement.Bounds
	Rules         placement.Rules
}

type PlacementStageResult struct {
	Request placement.Request `json:"request"`
	Result  placement.Result  `json:"result"`
	Stage   StageResult       `json:"stage"`
}

func PlaceFragments(ctx context.Context, request Request, fragments PCBFragmentResult, opts PlacementOptions) PlacementStageResult {
	var issues []reports.Issue
	if ctx == nil {
		issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "context", Message: "context is required"})
	} else if err := ctx.Err(); err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "context", Message: err.Error()})
	}
	if reports.HasBlockingIssue(fragments.Stage.Issues) {
		return PlacementStageResult{Stage: StageResult{Name: StagePlacement, Status: StageStatusSkipped, Summary: map[string]any{"reason": "PCB realization did not complete"}}}
	}
	if reports.HasBlockingIssue(issues) {
		return PlacementStageResult{Stage: NewStageResult(StagePlacement, issues)}
	}
	normalized := NormalizeRequest(request)
	placementRequest := placement.Request{
		Board: placement.BoardPlacementArea{
			WidthMM:  normalized.Board.WidthMM,
			HeightMM: normalized.Board.HeightMM,
			MarginMM: normalized.Board.EdgeClearanceMM,
		},
		Rules: mergePlacementRules(opts.Rules),
		Seed:  normalized.Name,
	}
	if placementRequest.Board.MarginMM == 0 {
		placementRequest.Board.MarginMM = 1
	}
	placementRequest.Rules.AllowBackLayer = normalized.Constraints.AllowBackLayer
	placementRequest.Rules.PreferTopLayer = normalized.Constraints.PreferTopLayer
	defaultBounds := opts.DefaultBounds
	if defaultBounds.WidthMM <= 0 || defaultBounds.HeightMM <= 0 {
		defaultBounds = defaultWorkflowBounds
	}
	for _, fragment := range fragments.Fragments {
		for _, component := range fragment.Realization.Components {
			position := placement.Placement{
				XMM:         component.Placement.XMM,
				YMM:         component.Placement.YMM,
				RotationDeg: component.Placement.RotationDeg,
				Layer:       firstNonEmpty(component.Placement.Layer, "F.Cu"),
			}
			placementRequest.Components = append(placementRequest.Components, placement.Component{
				Ref:         component.Ref,
				Value:       component.Value,
				FootprintID: component.FootprintID,
				Role:        component.ComponentRole,
				Bounds:      defaultBounds,
				Fixed:       true,
				Position:    &position,
				Side:        sideFromLayer(position.Layer),
				Rotation:    fixedRotation(component.Placement.RotationDeg),
			})
		}
		for _, route := range fragment.Realization.LocalRoutes {
			placementRequest.Nets = append(placementRequest.Nets, placement.Net{
				Name: route.NetName,
				Endpoints: []placement.Endpoint{
					{Ref: route.From.Ref, Pin: route.From.Pin},
					{Ref: route.To.Ref, Pin: route.To.Pin},
				},
				Role:   netRoleFromName(route.NetName),
				Weight: 10,
			})
		}
	}
	placementRequest = placement.NormalizeRequest(placementRequest)
	result := placement.PlaceContext(ctx, placementRequest)
	issues = append(issues, result.Issues...)
	stage := NewStageResult(StagePlacement, issues)
	stage.Summary = map[string]any{
		"component_count": result.Metrics.ComponentCount,
		"placed_count":    result.Metrics.PlacedCount,
		"unplaced_count":  result.Metrics.UnplacedCount,
		"fixed_count":     result.Metrics.FixedCount,
	}
	if result.Status != placement.StatusPlaced && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return PlacementStageResult{Request: placementRequest, Result: result, Stage: stage}
}

func mergePlacementRules(rules placement.Rules) placement.Rules {
	defaults := placement.DefaultRules()
	if rules.GridMM <= 0 {
		rules.GridMM = defaults.GridMM
	}
	if rules.ComponentSpacingMM <= 0 {
		rules.ComponentSpacingMM = defaults.ComponentSpacingMM
	}
	if rules.BoardEdgeClearanceMM <= 0 {
		rules.BoardEdgeClearanceMM = defaults.BoardEdgeClearanceMM
	}
	if rules.GroupSpacingMM <= 0 {
		rules.GroupSpacingMM = defaults.GroupSpacingMM
	}
	if rules.ConnectorEdgeClearanceMM <= 0 {
		rules.ConnectorEdgeClearanceMM = defaults.ConnectorEdgeClearanceMM
	}
	if rules.MaxCandidatesPerPart <= 0 {
		rules.MaxCandidatesPerPart = defaults.MaxCandidatesPerPart
	}
	return rules
}

func fixedRotation(rotation float64) placement.RotationConstraint {
	value := rotation
	return placement.RotationConstraint{FixedDeg: &value}
}

func sideFromLayer(layer string) placement.SideConstraint {
	if layer == "B.Cu" {
		return placement.SideBottom
	}
	return placement.SideTop
}

func netRoleFromName(name string) placement.NetRole {
	switch {
	case containsToken(name, "gnd"), containsToken(name, "ground"):
		return placement.NetGround
	case containsToken(name, "vcc"), containsToken(name, "vdd"), containsToken(name, "vbus"), containsToken(name, "vin"), containsToken(name, "vout"):
		return placement.NetPower
	case containsToken(name, "scl"), containsToken(name, "clk"), containsToken(name, "clock"):
		return placement.NetClock
	default:
		return placement.NetSignal
	}
}

func containsToken(name string, token string) bool {
	name = "_" + normalizeRoleName(name) + "_"
	return strings.Contains(name, "_"+token+"_")
}

func normalizeRoleName(name string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}
