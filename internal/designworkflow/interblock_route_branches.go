package designworkflow

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

var routeBranchOctagonUnitVectors = [...]routing.Point{
	{XMM: 1, YMM: 0},
	{XMM: math.Sqrt2 / 2, YMM: math.Sqrt2 / 2},
	{XMM: 0, YMM: 1},
	{XMM: -math.Sqrt2 / 2, YMM: math.Sqrt2 / 2},
	{XMM: -1, YMM: 0},
	{XMM: -math.Sqrt2 / 2, YMM: -math.Sqrt2 / 2},
	{XMM: 0, YMM: -1},
	{XMM: math.Sqrt2 / 2, YMM: -math.Sqrt2 / 2},
}

const routeBranchExistingCopperCapacityPerBranch = 8

type InterBlockBranchRoutingResult struct {
	NetName        string                            `json:"net_name"`
	Branches       []InterBlockBranchRoutingEvidence `json:"branches,omitempty"`
	Operations     []transactions.Operation          `json:"operations,omitempty"`
	ExistingCopper []routing.ExistingCopper          `json:"existing_copper,omitempty"`
	Issues         []reports.Issue                   `json:"issues,omitempty"`
}

type InterBlockBranchRoutingEvidence struct {
	BranchIndex     int            `json:"branch_index"`
	StartEndpointID string         `json:"start_endpoint_id"`
	EndEndpointID   string         `json:"end_endpoint_id"`
	Status          routing.Status `json:"status"`
	OperationCount  int            `json:"operation_count"`
	IssueCount      int            `json:"issue_count"`
}

func RouteInterBlockTreeBranches(ctx context.Context, base routing.Request, group InterBlockRouteGroup, tree InterBlockRouteTree) InterBlockBranchRoutingResult {
	result := InterBlockBranchRoutingResult{NetName: tree.NetName}
	result.Branches = make([]InterBlockBranchRoutingEvidence, 0, len(tree.Branches))
	endpoints := interBlockRouteGroupEndpointsByID(group)
	defaultLayer := routeBranchDefaultLayer(base.Board)
	rules := routeBranchEffectiveRules(base.Rules)
	currentExisting := make([]routing.ExistingCopper, 0, len(base.Existing)+len(tree.Branches)*routeBranchExistingCopperCapacityPerBranch)
	currentExisting = append(currentExisting, base.Existing...)
	orderedBranches := routeTreeBranchesForRouting(tree.Branches)
	for branchPosition, branch := range orderedBranches {
		evidence := InterBlockBranchRoutingEvidence{
			BranchIndex:     branch.Index,
			StartEndpointID: branch.StartEndpointID,
			EndEndpointID:   branch.EndEndpointID,
		}
		if err := ctx.Err(); err != nil {
			result.Issues = append(result.Issues, routeBranchCancellationIssue(tree.NetName, branch.Index, err))
			evidence.Status = routing.StatusBlocked
			evidence.IssueCount = 1
			result.Branches = append(result.Branches, evidence)
			for _, remaining := range orderedBranches[branchPosition+1:] {
				result.Issues = append(result.Issues, routeBranchCancellationIssue(tree.NetName, remaining.Index, err))
				result.Branches = append(result.Branches, InterBlockBranchRoutingEvidence{
					BranchIndex:     remaining.Index,
					StartEndpointID: remaining.StartEndpointID,
					EndEndpointID:   remaining.EndEndpointID,
					Status:          routing.StatusBlocked,
					IssueCount:      1,
				})
			}
			return result
		}
		start, startOK := endpoints[branch.StartEndpointID]
		end, endOK := endpoints[branch.EndEndpointID]
		if !startOK || !endOK {
			issue := reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityBlocked,
				Path:       fmt.Sprintf("design.inter_block_route_groups[%q].branches[%d]", tree.NetName, branch.Index),
				Message:    "route-tree branch endpoint is missing from route group",
				Nets:       []string{tree.NetName},
				Suggestion: "rebuild route group endpoints before branch routing",
			}
			result.Issues = append(result.Issues, issue)
			evidence.Status = routing.StatusBlocked
			evidence.IssueCount = 1
			result.Branches = append(result.Branches, evidence)
			continue
		}
		branchNet := routing.Net{
			Name: tree.NetName,
			Endpoints: []routing.Endpoint{
				{Ref: start.Ref, Pin: start.Pin},
				{Ref: end.Ref, Pin: end.Pin},
			},
		}
		branchRequest := routing.Request{
			Board:      base.Board,
			Components: base.Components,
			Obstacles:  base.Obstacles,
			Existing:   currentExisting,
			Nets:       []routing.Net{branchNet},
			Rules:      base.Rules,
			Strategy:   base.Strategy,
			Seed:       base.Seed,
		}
		branchResult := routing.RouteRequestContext(ctx, branchRequest)
		branchOperations := transactionRouteOperations(branchResult.Operations)
		branchExisting := existingCopperFromRoutedBranches(branchResult.Routes, defaultLayer, rules)
		branchIssues := annotateInterBlockBranchRoutingIssues(branchResult.Issues, tree.NetName, branch)
		result.Operations = append(result.Operations, branchOperations...)
		result.ExistingCopper = append(result.ExistingCopper, branchExisting...)
		result.Issues = append(result.Issues, branchIssues...)
		currentExisting = append(currentExisting, branchExisting...)
		evidence.Status = branchResult.Status
		evidence.OperationCount = len(branchOperations)
		evidence.IssueCount = len(branchIssues)
		result.Branches = append(result.Branches, evidence)
	}
	return result
}

func routeTreeBranchesForRouting(branches []InterBlockRouteTreeBranch) []InterBlockRouteTreeBranch {
	if len(branches) == 0 {
		return nil
	}
	ordered := slices.Clone(branches)
	slices.SortFunc(ordered, compareRouteTreeBranchForRouting)
	return ordered
}

func compareRouteTreeBranchForRouting(left, right InterBlockRouteTreeBranch) int {
	leftDistance := routeTreeBranchDistanceRank(left.PlannedDistanceMM)
	rightDistance := routeTreeBranchDistanceRank(right.PlannedDistanceMM)
	if compare := cmp.Compare(leftDistance, rightDistance); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(left.StartEndpointID, right.StartEndpointID); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(left.EndEndpointID, right.EndEndpointID); compare != 0 {
		return compare
	}
	return cmp.Compare(left.Index, right.Index)
}

func routeTreeBranchDistanceRank(distance float64) int64 {
	if distance < 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		return math.MaxInt64
	}
	return int64(math.Round(distance * 1000))
}

func annotateInterBlockBranchRoutingIssues(issues []reports.Issue, netName string, branch InterBlockRouteTreeBranch) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]reports.Issue, len(issues))
	for index, issue := range issues {
		out[index] = issue
		basePath := fmt.Sprintf("design.inter_block_route_groups[%q].branches[%d]", netName, branch.Index)
		trimmedPath := strings.TrimPrefix(issue.Path, ".")
		if trimmedPath != "" {
			basePath += "." + trimmedPath
		}
		out[index].Path = basePath
		out[index].Suggestion = branchRepairSuggestion(issue.Suggestion)
	}
	return out
}

func branchRepairSuggestion(suggestion string) string {
	suggestion = strings.TrimSpace(suggestion)
	if suggestion == "" {
		return "adjust placement spacing or route-tree layer access for this branch"
	}
	return suggestion + "; route-tree branch may need more spacing, fanout, or layer access"
}

func routeBranchCancellationIssue(netName string, branchIndex int, err error) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityBlocked,
		Path:       fmt.Sprintf("design.inter_block_route_groups[%q].branches[%d]", netName, branchIndex),
		Message:    err.Error(),
		Nets:       []string{netName},
		Suggestion: "retry branch routing with an active context",
	}
}

func interBlockRouteGroupEndpointsByID(group InterBlockRouteGroup) map[string]InterBlockRouteGroupEndpoint {
	endpoints := make(map[string]InterBlockRouteGroupEndpoint, len(group.RequiredEndpoints)+len(group.OptionalEndpoints))
	for _, endpoint := range group.RequiredEndpoints {
		endpoints[endpoint.ID] = endpoint
	}
	for _, endpoint := range group.OptionalEndpoints {
		if _, exists := endpoints[endpoint.ID]; !exists {
			endpoints[endpoint.ID] = endpoint
		}
	}
	return endpoints
}

func existingCopperFromRoutedBranches(routes []routing.Route, defaultLayer string, rules routing.Rules) []routing.ExistingCopper {
	capacity := 0
	for _, route := range routes {
		if route.Status != routing.RouteStatusRouted {
			continue
		}
		capacity += len(route.Segments)
		for _, via := range route.Vias {
			capacity += len(via.Layers)
		}
	}
	existing := make([]routing.ExistingCopper, 0, capacity)
	for _, route := range routes {
		if route.Status != routing.RouteStatusRouted {
			continue
		}
		for _, segment := range route.Segments {
			existing = append(existing, routing.ExistingCopper{
				Kind:     routing.CopperSegment,
				Net:      segment.Net,
				Layer:    routeBranchCanonicalLayer(segment.Layer, defaultLayer),
				Geometry: routeBranchSegmentShape(segment, rules),
			})
		}
		for _, via := range route.Vias {
			shape := routeBranchViaShape(via, rules)
			for _, layer := range via.Layers {
				existing = append(existing, routing.ExistingCopper{
					Kind:     routing.CopperVia,
					Net:      via.Net,
					Layer:    routeBranchCanonicalLayer(layer, defaultLayer),
					Geometry: shape,
				})
			}
		}
	}
	return existing
}

func routeBranchSegmentShape(segment routing.Segment, rules routing.Rules) routing.Shape {
	halfWidth := segment.WidthMM / 2
	if halfWidth <= 0 {
		halfWidth = rules.TraceWidthMM / 2
	}
	dx := segment.End.XMM - segment.Start.XMM
	dy := segment.End.YMM - segment.Start.YMM
	const epsilon = 1e-9
	if math.Abs(dx) < epsilon && math.Abs(dy) < epsilon {
		return routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: min(segment.Start.XMM, segment.End.XMM) - halfWidth, YMM: min(segment.Start.YMM, segment.End.YMM) - halfWidth},
			Max: routing.Point{XMM: max(segment.Start.XMM, segment.End.XMM) + halfWidth, YMM: max(segment.Start.YMM, segment.End.YMM) + halfWidth},
		}}
	}
	if math.Abs(dy) < epsilon {
		return routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: min(segment.Start.XMM, segment.End.XMM) - halfWidth, YMM: segment.Start.YMM - halfWidth},
			Max: routing.Point{XMM: max(segment.Start.XMM, segment.End.XMM) + halfWidth, YMM: segment.Start.YMM + halfWidth},
		}}
	}
	if math.Abs(dx) < epsilon {
		return routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: segment.Start.XMM - halfWidth, YMM: min(segment.Start.YMM, segment.End.YMM) - halfWidth},
			Max: routing.Point{XMM: segment.Start.XMM + halfWidth, YMM: max(segment.Start.YMM, segment.End.YMM) + halfWidth},
		}}
	}
	length := math.Hypot(dx, dy)
	extendX := dx / length * halfWidth
	extendY := dy / length * halfWidth
	offsetX := -dy / length * halfWidth
	offsetY := dx / length * halfWidth
	start := routing.Point{XMM: segment.Start.XMM - extendX, YMM: segment.Start.YMM - extendY}
	end := routing.Point{XMM: segment.End.XMM + extendX, YMM: segment.End.YMM + extendY}
	return routing.Shape{Polygon: []routing.Point{
		{XMM: start.XMM + offsetX, YMM: start.YMM + offsetY},
		{XMM: end.XMM + offsetX, YMM: end.YMM + offsetY},
		{XMM: end.XMM - offsetX, YMM: end.YMM - offsetY},
		{XMM: start.XMM - offsetX, YMM: start.YMM - offsetY},
	}}
}

func routeBranchViaShape(via routing.Via, rules routing.Rules) routing.Shape {
	radius := via.DiameterMM / 2
	if radius <= 0 {
		radius = rules.ViaDiameterMM / 2
	}
	vertexRadius := radius / math.Cos(math.Pi/float64(len(routeBranchOctagonUnitVectors)))
	points := make([]routing.Point, 0, len(routeBranchOctagonUnitVectors))
	for _, unit := range routeBranchOctagonUnitVectors {
		points = append(points, routing.Point{
			XMM: via.At.XMM + unit.XMM*vertexRadius,
			YMM: via.At.YMM + unit.YMM*vertexRadius,
		})
	}
	return routing.Shape{Polygon: points}
}

func routeBranchEffectiveRules(rules routing.Rules) routing.Rules {
	request := routing.Request{Rules: rules}
	routing.NormalizeRequest(&request)
	return request.Rules
}

func routeBranchDefaultLayer(board routing.Board) string {
	firstCopper := ""
	for _, layer := range board.Layers {
		if layer.Kind == routing.LayerCopper {
			if name := routeBranchLayerName(layer); name != "" {
				if layer.Routable {
					return name
				}
				if firstCopper == "" {
					firstCopper = name
				}
			}
		}
	}
	if firstCopper != "" {
		return firstCopper
	}
	return routing.DefaultRules().PreferLayer
}

func routeBranchLayerName(layer routing.Layer) string {
	if canonical := canonicalCopperLayer(layer.Name); canonical != "" {
		return canonical
	}
	return layer.Name
}

func routeBranchCanonicalLayer(layer string, defaultLayer string) string {
	canonical := canonicalCopperLayer(layer)
	if canonical == "" {
		if layer == "" {
			return defaultLayer
		}
		return layer
	}
	return canonical
}
