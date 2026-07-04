package designworkflow

import (
	"cmp"
	"context"
	"fmt"
	// geometry helpers for route access point sampling and deduplication
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
const routeTreeSyntheticAccessPadSizeMM = 0.4
const routeTreeObstacleAuditCellMM = 5.0
const routeTreeImmediateObstaclePressureMM = 0.75
const routeTreeNearObstaclePressureMM = 1.5
const routeTreeMaxPolygonCopperAccessPoints = 16
const routeTreeSameNetExistingCopperSource = "same_net_existing_copper"
const routeTreeGeneratedSameNetCopperNonPreferredRankPenalty = 3
const routeTreeAccessDedupeUnitsPerMM = 1e6

type InterBlockBranchRoutingResult struct {
	NetName        string                            `json:"net_name"`
	Branches       []InterBlockBranchRoutingEvidence `json:"branches,omitempty"`
	Operations     []transactions.Operation          `json:"operations,omitempty"`
	ExistingCopper []routing.ExistingCopper          `json:"existing_copper,omitempty"`
	Issues         []reports.Issue                   `json:"issues,omitempty"`
}

type InterBlockBranchRoutingEvidence struct {
	BranchIndex              int                                    `json:"branch_index"`
	StartEndpointID          string                                 `json:"start_endpoint_id"`
	EndEndpointID            string                                 `json:"end_endpoint_id"`
	Status                   routing.Status                         `json:"status"`
	OperationCount           int                                    `json:"operation_count"`
	IssueCount               int                                    `json:"issue_count"`
	BlockingIssueCount       int                                    `json:"blocking_issue_count,omitempty"`
	WarningIssueCount        int                                    `json:"warning_issue_count,omitempty"`
	InfoIssueCount           int                                    `json:"info_issue_count,omitempty"`
	FixedNetSkipNotices      int                                    `json:"fixed_net_skip_notices,omitempty"`
	AccessPairsTried         int                                    `json:"access_pairs_tried,omitempty"`
	AccessSourceCount        int                                    `json:"access_source_count,omitempty"`
	AccessTargetCount        int                                    `json:"access_target_count,omitempty"`
	AccessPairCount          int                                    `json:"access_pair_count,omitempty"`
	AccessPairLimit          int                                    `json:"access_pair_limit,omitempty"`
	AccessPairsTruncated     bool                                   `json:"access_pairs_truncated,omitempty"`
	SelectedSourceRole       RouteTreeEndpointAccessRole            `json:"selected_source_role,omitempty"`
	SelectedTargetRole       RouteTreeEndpointAccessRole            `json:"selected_target_role,omitempty"`
	SelectedSourceEndpointID string                                 `json:"selected_source_endpoint_id,omitempty"`
	SelectedTargetEndpointID string                                 `json:"selected_target_endpoint_id,omitempty"`
	SelectedSourceRef        string                                 `json:"selected_source_ref,omitempty"`
	SelectedSourcePad        string                                 `json:"selected_source_pad,omitempty"`
	SelectedSourceLayer      string                                 `json:"selected_source_layer,omitempty"`
	SelectedSourceXMM        float64                                `json:"selected_source_x_mm"`
	SelectedSourceYMM        float64                                `json:"selected_source_y_mm"`
	SelectedTargetRef        string                                 `json:"selected_target_ref,omitempty"`
	SelectedTargetPad        string                                 `json:"selected_target_pad,omitempty"`
	SelectedTargetLayer      string                                 `json:"selected_target_layer,omitempty"`
	SelectedTargetXMM        float64                                `json:"selected_target_x_mm"`
	SelectedTargetYMM        float64                                `json:"selected_target_y_mm"`
	SelectedSourceReason     string                                 `json:"selected_source_reason,omitempty"`
	SelectedTargetReason     string                                 `json:"selected_target_reason,omitempty"`
	SnapExemptRoute          bool                                   `json:"snap_exempt_route,omitempty"`
	AccessAttempts           []RouteTreeBranchAccessAttemptEvidence `json:"access_attempts,omitempty"`
}

type RouteTreeBranchAccessAttemptEvidence struct {
	PairRank         int                         `json:"pair_rank"`
	SourceRole       RouteTreeEndpointAccessRole `json:"source_role,omitempty"`
	TargetRole       RouteTreeEndpointAccessRole `json:"target_role,omitempty"`
	SourceEndpointID string                      `json:"source_endpoint_id,omitempty"`
	TargetEndpointID string                      `json:"target_endpoint_id,omitempty"`
	SourceLayer      string                      `json:"source_layer,omitempty"`
	TargetLayer      string                      `json:"target_layer,omitempty"`
	SourceXMM        float64                     `json:"source_x_mm"`
	SourceYMM        float64                     `json:"source_y_mm"`
	TargetXMM        float64                     `json:"target_x_mm"`
	TargetYMM        float64                     `json:"target_y_mm"`
	SourceReason     string                      `json:"source_reason,omitempty"`
	TargetReason     string                      `json:"target_reason,omitempty"`
	Status           routing.Status              `json:"status"`
	IssueCount       int                         `json:"issue_count,omitempty"`
	PrimaryCode      reports.Code                `json:"primary_code,omitempty"`
	PrimaryMessage   string                      `json:"primary_message,omitempty"`
	PrimaryRef       string                      `json:"primary_ref,omitempty"`
	PrimaryNet       string                      `json:"primary_net,omitempty"`
	SameNetPads      int                         `json:"same_net_pads,omitempty"`
	SameNetAnchors   int                         `json:"same_net_local_route_anchors,omitempty"`
	SameNetCopper    int                         `json:"same_net_existing_copper,omitempty"`
	ObstacleKind     string                      `json:"nearest_obstacle_kind,omitempty"`
	ObstacleRef      string                      `json:"nearest_obstacle_ref,omitempty"`
	ObstacleNet      string                      `json:"nearest_obstacle_net,omitempty"`
	ObstacleDistMM   float64                     `json:"nearest_obstacle_distance_mm,omitempty"`
}

type routeTreeAccessCandidateCache map[routeTreeAccessCandidateCacheKey][]routeTreeBranchAccessCandidate

type routeTreeAccessCandidateCacheKey struct {
	endpointID       string
	netName          string
	oppositeRole     RouteTreeEndpointAccessRole
	oppositeEndpoint string
	oppositeRef      string
	oppositePad      string
	oppositeLayer    string
	oppositeXMicron  int64
	oppositeYMicron  int64
}

func RouteInterBlockTreeBranches(ctx context.Context, base routing.Request, group InterBlockRouteGroup, tree InterBlockRouteTree) InterBlockBranchRoutingResult {
	return RouteInterBlockTreeBranchesWithAccess(ctx, base, group, tree, nil)
}

func RouteInterBlockTreeBranchesWithAccess(ctx context.Context, base routing.Request, group InterBlockRouteGroup, tree InterBlockRouteTree, access []RouteTreeEndpointAccess) InterBlockBranchRoutingResult {
	result := InterBlockBranchRoutingResult{NetName: tree.NetName}
	result.Branches = make([]InterBlockBranchRoutingEvidence, 0, len(tree.Branches))
	endpoints := interBlockRouteGroupEndpointsByID(group)
	defaultLayer := routeBranchDefaultLayer(base.Board)
	rules := routeBranchEffectiveRules(base.Rules)
	currentExisting := make([]routing.ExistingCopper, 0, len(base.Existing)+len(tree.Branches)*routeBranchExistingCopperCapacityPerBranch)
	currentExisting = append(currentExisting, base.Existing...)
	branchAccess := access
	preferSameNetCopperMerge := routeTreePrefersSameNetCopperAccess(base.Nets, tree.NetName)
	branchAccess = routeTreeEndpointAccessWithSameNetCopper(branchAccess, base.Existing, tree.NetName)
	accessCandidateCache := routeTreeAccessCandidateCache{}
	orderedBranches := routeTreeBranchesForRoutingWithAccess(tree.Branches, branchAccess, tree.NetName, accessCandidateCache)
	mergeAuditBase := routeTreeMergeAuditBaseForRequest(base, tree.NetName, preferSameNetCopperMerge)
	for branchPosition, branch := range orderedBranches {
		evidence := InterBlockBranchRoutingEvidence{
			BranchIndex:     branch.Index,
			StartEndpointID: branch.StartEndpointID,
			EndEndpointID:   branch.EndEndpointID,
		}
		if ctx != nil && ctx.Err() != nil {
			err := ctx.Err()
			result.Issues = append(result.Issues, routeBranchCancellationIssue(tree.NetName, branch.Index, err))
			evidence.Status = routing.StatusBlocked
			evidence.IssueCount = 1
			result.Branches = append(result.Branches, evidence)
			for _, remaining := range orderedBranches[branchPosition+1:] {
				result.Branches = append(result.Branches, InterBlockBranchRoutingEvidence{
					BranchIndex:     remaining.Index,
					StartEndpointID: remaining.StartEndpointID,
					EndEndpointID:   remaining.EndEndpointID,
					Status:          routing.StatusBlocked,
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
		branchBase := base
		branchBase.Existing = currentExisting
		accessAudit := routeTreeBranchAccessAuditForBranchWithMergeAudit(branchAccess, tree.NetName, branch, accessCandidateCache, mergeAuditBase)
		mergeAudit := routeTreeMergeAuditForBranch(mergeAuditBase, branchBase.Existing, tree.NetName)
		branchResult, selectedPair, selectedPairOK, accessPairsTried, accessAttempts := routeTreeRouteBranch(ctx, branchBase, tree.NetName, branch, start, end, accessAudit, mergeAudit)
		accessSelected := selectedPairOK
		evidence.AccessPairsTried = accessPairsTried
		populateRouteTreeAccessAuditEvidence(&evidence, accessAudit)
		evidence.AccessAttempts = accessAttempts
		if accessSelected {
			populateSelectedRouteTreeAccessEvidence(&evidence, selectedPair)
		}
		branchOperations := transactionRouteOperations(branchResult.Operations)
		evidence.OperationCount = len(branchOperations)
		branchExisting := existingCopperFromRoutedBranches(branchResult.Routes, defaultLayer, rules)
		branchIssues := annotateInterBlockBranchRoutingIssues(branchResult.Issues, tree.NetName, branch)
		if accessSelected {
			for index := range branchOperations {
				if branchOperations[index].Op == transactions.OpRoute {
					branchOperations[index].SnapExempt = true
					evidence.SnapExemptRoute = true
				}
			}
		}
		result.Operations = append(result.Operations, branchOperations...)
		result.ExistingCopper = append(result.ExistingCopper, branchExisting...)
		result.Issues = append(result.Issues, branchIssues...)
		currentExisting = append(currentExisting, branchExisting...)
		if len(branchExisting) != 0 {
			branchAccess = routeTreeEndpointAccessWithSameNetCopper(branchAccess, branchExisting, tree.NetName)
			accessCandidateCache = routeTreeAccessCandidateCache{}
		}
		evidence.Status = branchResult.Status
		evidence.IssueCount = len(branchIssues)
		evidence.BlockingIssueCount, evidence.WarningIssueCount, evidence.InfoIssueCount, evidence.FixedNetSkipNotices = routeTreeIssueCounters(branchIssues)
		result.Branches = append(result.Branches, evidence)
	}
	return result
}

func routeTreeIssueCounters(issues []reports.Issue) (blocking int, warnings int, info int, fixedNetSkips int) {
	for _, issue := range issues {
		switch issue.Severity {
		case reports.SeverityBlocked, reports.SeverityError:
			blocking++
		case reports.SeverityWarning:
			warnings++
		case reports.SeverityInfo:
			info++
		default:
			blocking++
		}
		if routeTreeIssueIsFixedNetSkip(issue) {
			fixedNetSkips++
		}
	}
	return blocking, warnings, info, fixedNetSkips
}

func routeTreeIssueIsFixedNetSkip(issue reports.Issue) bool {
	return issue.Code == reports.CodeFixedNetSkipped
}

func routeTreeRouteBranch(ctx context.Context, base routing.Request, netName string, branch InterBlockRouteTreeBranch, start InterBlockRouteGroupEndpoint, end InterBlockRouteGroupEndpoint, accessAudit routeTreeBranchAccessAudit, mergeAudit routeTreeMergeAudit) (routing.Result, routeTreeBranchAccessPair, bool, int, []RouteTreeBranchAccessAttemptEvidence) {
	pairs := accessAudit.Pairs
	if len(pairs) == 0 {
		return routing.RouteRequestContext(ctx, routeTreeEndpointBranchRequest(base, netName, start, end)), routeTreeBranchAccessPair{}, false, 0, nil
	}
	var lastResult routing.Result
	attempts := make([]RouteTreeBranchAccessAttemptEvidence, 0, len(pairs))
	for index := range pairs {
		if ctx != nil && ctx.Err() != nil {
			return routing.Result{
				Status: routing.StatusBlocked,
				Issues: []reports.Issue{
					routeBranchCancellationIssue(netName, branch.Index, ctx.Err()),
				},
			}, routeTreeBranchAccessPair{}, false, index, attempts
		}
		pair := pairs[index]
		result := routing.RouteRequestContext(ctx, routeTreeAccessBranchRequest(base, netName, pair))
		lastResult = result
		attempts = append(attempts, routeTreeBranchAccessAttemptEvidenceFor(mergeAudit, netName, pair, result))
		if result.Status == routing.StatusRouted {
			return result, pair, true, index + 1, attempts
		}
	}
	return lastResult, routeTreeBranchAccessPair{}, false, len(pairs), attempts
}

func populateSelectedRouteTreeAccessEvidence(evidence *InterBlockBranchRoutingEvidence, pair routeTreeBranchAccessPair) {
	if evidence == nil {
		return
	}
	source := pair.Source.Access
	target := pair.Target.Access
	evidence.SelectedSourceRole = source.Role
	evidence.SelectedSourceEndpointID = source.EndpointID
	evidence.SelectedSourceRef = source.Ref
	evidence.SelectedSourcePad = source.Pad
	evidence.SelectedSourceLayer = source.Layer
	evidence.SelectedSourceXMM = source.XMM
	evidence.SelectedSourceYMM = source.YMM
	evidence.SelectedSourceReason = pair.Source.RankReason
	evidence.SelectedTargetRole = target.Role
	evidence.SelectedTargetEndpointID = target.EndpointID
	evidence.SelectedTargetRef = target.Ref
	evidence.SelectedTargetPad = target.Pad
	evidence.SelectedTargetLayer = target.Layer
	evidence.SelectedTargetXMM = target.XMM
	evidence.SelectedTargetYMM = target.YMM
	evidence.SelectedTargetReason = pair.Target.RankReason
}

func populateRouteTreeAccessAuditEvidence(evidence *InterBlockBranchRoutingEvidence, audit routeTreeBranchAccessAudit) {
	if evidence == nil {
		return
	}
	evidence.AccessSourceCount = len(audit.SourceCandidates)
	evidence.AccessTargetCount = len(audit.TargetCandidates)
	evidence.AccessPairCount = audit.TotalPairCount
	if audit.TotalPairCount > 0 {
		evidence.AccessPairLimit = audit.Limit
	}
	evidence.AccessPairsTruncated = audit.Truncated
}

func routeTreeBranchAccessAttemptEvidenceFor(mergeAudit routeTreeMergeAudit, netName string, pair routeTreeBranchAccessPair, result routing.Result) RouteTreeBranchAccessAttemptEvidence {
	attempt := RouteTreeBranchAccessAttemptEvidence{
		PairRank:         pair.Rank,
		SourceRole:       pair.Source.Access.Role,
		TargetRole:       pair.Target.Access.Role,
		SourceEndpointID: pair.Source.Access.EndpointID,
		TargetEndpointID: pair.Target.Access.EndpointID,
		SourceLayer:      pair.Source.Access.Layer,
		TargetLayer:      pair.Target.Access.Layer,
		SourceXMM:        pair.Source.Access.XMM,
		SourceYMM:        pair.Source.Access.YMM,
		TargetXMM:        pair.Target.Access.XMM,
		TargetYMM:        pair.Target.Access.YMM,
		SourceReason:     pair.Source.RankReason,
		TargetReason:     pair.Target.RankReason,
		Status:           result.Status,
		IssueCount:       len(result.Issues),
	}
	populateRouteTreeSameNetMergeAudit(&attempt, mergeAudit, netName, pair)
	if len(result.Issues) != 0 {
		issue := result.Issues[0]
		attempt.PrimaryCode = issue.Code
		attempt.PrimaryMessage = issue.Message
		if len(issue.Refs) != 0 {
			attempt.PrimaryRef = issue.Refs[0]
		}
		if len(issue.Nets) != 0 {
			attempt.PrimaryNet = issue.Nets[0]
		}
	}
	return attempt
}

type routeTreeMergeAudit struct {
	SameNetPads     int
	SameNetCopper   int
	OtherNetPadGrid routeTreeOtherNetPadGrid
}

type routeTreeOtherNetPad struct {
	Ref   string
	Net   string
	Point routing.Point
}

type routeTreeMergeAuditBase struct {
	SameNetPads              int
	PreferSameNetCopperMerge bool
	OtherNetPadGrid          routeTreeOtherNetPadGrid
}

type routeTreeOtherNetPadGrid struct {
	Cells map[routeTreeOtherNetPadCell][]routeTreeOtherNetPad
	Count int
	MinX  int
	MaxX  int
	MinY  int
	MaxY  int
}

type routeTreeOtherNetPadCell struct {
	X int
	Y int
}

func routeTreeMergeAuditBaseForRequest(base routing.Request, netName string, preferSameNetCopperMerge bool) routeTreeMergeAuditBase {
	audit := routeTreeMergeAuditBase{
		PreferSameNetCopperMerge: preferSameNetCopperMerge,
		OtherNetPadGrid:          routeTreeOtherNetPadGrid{Cells: map[routeTreeOtherNetPadCell][]routeTreeOtherNetPad{}},
	}
	for _, component := range base.Components {
		for _, pad := range component.Pads {
			padNet := pad.Net
			if strings.EqualFold(padNet, netName) {
				audit.SameNetPads++
				continue
			}
			if padNet == "" {
				continue
			}
			ref := pad.Ref
			if ref == "" {
				ref = component.Ref
			}
			if pad.Name != "" {
				ref += "." + pad.Name
			}
			audit.OtherNetPadGrid.add(routeTreeOtherNetPad{
				Ref:   ref,
				Net:   padNet,
				Point: pad.Position,
			})
		}
	}
	return audit
}

func routeTreeMergeAuditForBranch(base routeTreeMergeAuditBase, existing []routing.ExistingCopper, netName string) routeTreeMergeAudit {
	audit := routeTreeMergeAudit{
		SameNetPads:     base.SameNetPads,
		OtherNetPadGrid: base.OtherNetPadGrid,
	}
	for _, copper := range existing {
		if strings.EqualFold(copper.Net, netName) {
			audit.SameNetCopper++
		}
	}
	return audit
}

func (grid *routeTreeOtherNetPadGrid) add(pad routeTreeOtherNetPad) {
	if grid.Cells == nil {
		grid.Cells = map[routeTreeOtherNetPadCell][]routeTreeOtherNetPad{}
	}
	cell := routeTreeOtherNetPadCellFor(pad.Point)
	grid.Cells[cell] = append(grid.Cells[cell], pad)
	grid.Count++
	if grid.Count == 1 {
		grid.MinX = cell.X
		grid.MaxX = cell.X
		grid.MinY = cell.Y
		grid.MaxY = cell.Y
		return
	}
	if cell.X < grid.MinX {
		grid.MinX = cell.X
	}
	if cell.X > grid.MaxX {
		grid.MaxX = cell.X
	}
	if cell.Y < grid.MinY {
		grid.MinY = cell.Y
	}
	if cell.Y > grid.MaxY {
		grid.MaxY = cell.Y
	}
}

func populateRouteTreeSameNetMergeAudit(attempt *RouteTreeBranchAccessAttemptEvidence, mergeAudit routeTreeMergeAudit, netName string, pair routeTreeBranchAccessPair) {
	if attempt == nil {
		return
	}
	attempt.SameNetPads = mergeAudit.SameNetPads
	attempt.SameNetAnchors = routeTreeSameNetAnchorCount(pair, netName)
	attempt.SameNetCopper = mergeAudit.SameNetCopper
	kind, ref, net, distance := nearestRouteTreeOtherNetPad(mergeAudit.OtherNetPadGrid, pair)
	attempt.ObstacleKind = kind
	attempt.ObstacleRef = ref
	attempt.ObstacleNet = net
	attempt.ObstacleDistMM = distance
}

func routeTreeSameNetAnchorCount(pair routeTreeBranchAccessPair, netName string) int {
	count := 0
	for _, access := range []RouteTreeEndpointAccess{pair.Source.Access, pair.Target.Access} {
		if access.Role == RouteTreeAccessLocalRouteAnchor && strings.EqualFold(access.Net, netName) {
			count++
		}
	}
	return count
}

func nearestRouteTreeOtherNetPad(grid routeTreeOtherNetPadGrid, pair routeTreeBranchAccessPair) (string, string, string, float64) {
	if grid.Count == 0 {
		return "", "", "", 0
	}
	points := []routing.Point{
		{XMM: pair.Source.Access.XMM, YMM: pair.Source.Access.YMM},
		{XMM: pair.Target.Access.XMM, YMM: pair.Target.Access.YMM},
	}
	bestDistance := math.Inf(1)
	bestRef := ""
	bestNet := ""
	for _, point := range points {
		center := routeTreeOtherNetPadCellFor(point)
		for radius := 0; radius <= routeTreeOtherNetPadSearchRadius(grid, center); radius++ {
			for x := center.X - radius; x <= center.X+radius; x++ {
				for y := center.Y - radius; y <= center.Y+radius; y++ {
					if radius > 0 && x > center.X-radius && x < center.X+radius && y > center.Y-radius && y < center.Y+radius {
						continue
					}
					cell := routeTreeOtherNetPadCell{X: x, Y: y}
					for _, pad := range grid.Cells[cell] {
						distance := math.Hypot(point.XMM-pad.Point.XMM, point.YMM-pad.Point.YMM)
						if distance < bestDistance {
							bestDistance = distance
							bestRef = pad.Ref
							bestNet = pad.Net
						}
					}
				}
			}
			if radius > 0 && bestRef != "" && bestDistance <= float64(radius-1)*routeTreeObstacleAuditCellMM {
				break
			}
		}
	}
	if bestRef == "" {
		return "", "", "", 0
	}
	return "other_net_pad", bestRef, bestNet, bestDistance
}

func nearestRouteTreeOtherNetPadDistance(grid routeTreeOtherNetPadGrid, point routing.Point) (float64, bool) {
	if grid.Count == 0 {
		return 0, false
	}
	center := routeTreeOtherNetPadCellFor(point)
	bestDistance := math.Inf(1)
	for radius := 0; radius <= routeTreeOtherNetPadSearchRadius(grid, center); radius++ {
		for x := center.X - radius; x <= center.X+radius; x++ {
			for y := center.Y - radius; y <= center.Y+radius; y++ {
				if radius > 0 && x > center.X-radius && x < center.X+radius && y > center.Y-radius && y < center.Y+radius {
					continue
				}
				cell := routeTreeOtherNetPadCell{X: x, Y: y}
				for _, pad := range grid.Cells[cell] {
					distance := math.Hypot(point.XMM-pad.Point.XMM, point.YMM-pad.Point.YMM)
					if distance < bestDistance {
						bestDistance = distance
					}
				}
			}
		}
		if radius > 0 && bestDistance <= float64(radius-1)*routeTreeObstacleAuditCellMM {
			break
		}
	}
	if math.IsInf(bestDistance, 1) {
		return 0, false
	}
	return bestDistance, true
}

func routeTreeOtherNetPadCellFor(point routing.Point) routeTreeOtherNetPadCell {
	return routeTreeOtherNetPadCell{
		X: int(math.Floor(point.XMM / routeTreeObstacleAuditCellMM)),
		Y: int(math.Floor(point.YMM / routeTreeObstacleAuditCellMM)),
	}
}

func routeTreeOtherNetPadSearchRadius(grid routeTreeOtherNetPadGrid, center routeTreeOtherNetPadCell) int {
	if grid.Count <= 0 {
		return 0
	}
	radius := max(
		absInt(center.X-grid.MinX),
		absInt(center.X-grid.MaxX),
		absInt(center.Y-grid.MinY),
		absInt(center.Y-grid.MaxY),
	)
	if radius < 1 {
		return 1
	}
	return radius
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func routeTreeAccessBranchRequest(base routing.Request, netName string, pair routeTreeBranchAccessPair) routing.Request {
	defaultLayer := routeBranchDefaultLayer(base.Board)
	sourceRef := routeTreeSyntheticAccessRef("SRC", pair.Rank)
	targetRef := routeTreeSyntheticAccessRef("DST", pair.Rank)
	endpoints := []routing.Endpoint{
		{Ref: sourceRef, Pin: "1"},
		{Ref: targetRef, Pin: "1"},
	}
	request := base
	// This helper only appends synthetic components and replaces Nets. The
	// router clones and normalizes the full request before pathfinding.
	request.Components = make([]routing.Component, 0, len(base.Components)+2)
	request.Components = append(request.Components, base.Components...)
	branchNet, branchNets := routeTreeAccessBranchNetSet(base.Nets, netName, endpoints)
	request.Nets = branchNets
	request.Components = append(request.Components,
		routeTreeSyntheticAccessComponent(sourceRef, branchNet.Name, pair.Source.Access, defaultLayer),
		routeTreeSyntheticAccessComponent(targetRef, branchNet.Name, pair.Target.Access, defaultLayer),
	)
	return request
}

func routeTreeEndpointAccessWithSameNetCopper(access []RouteTreeEndpointAccess, existing []routing.ExistingCopper, netName string) []RouteTreeEndpointAccess {
	if len(existing) == 0 {
		return access
	}
	trimmedNetName := strings.TrimSpace(netName)
	out := make([]RouteTreeEndpointAccess, 0, len(access)+len(existing))
	out = append(out, access...)
	for _, copper := range existing {
		copperNet := strings.TrimSpace(copper.Net)
		if !strings.EqualFold(copperNet, trimmedNetName) {
			continue
		}
		layer := normalizeContactLayer(copper.Layer)
		for _, point := range routeTreeExistingCopperAccessPoints(copper) {
			out = append(out, RouteTreeEndpointAccess{
				Role:   RouteTreeAccessSameNetCopper,
				Net:    trimmedNetName,
				Layer:  layer,
				XMM:    point.XMM,
				YMM:    point.YMM,
				Source: routeTreeSameNetExistingCopperSource,
			})
		}
	}
	out = uniqueRouteTreeEndpointAccess(out)
	slices.SortFunc(out, compareRouteTreeEndpointAccess)
	return out
}

func routeTreePrefersSameNetCopperAccess(nets []routing.Net, netName string) bool {
	trimmedNetName := strings.TrimSpace(netName)
	for _, net := range nets {
		if !strings.EqualFold(strings.TrimSpace(net.Name), trimmedNetName) {
			continue
		}
		return net.Role == routing.NetPower || net.Role == routing.NetGround || net.Role == routing.NetHighCurrent
	}
	return false
}

func routeTreeExistingCopperAccessPoints(copper routing.ExistingCopper) []routing.Point {
	if len(copper.Centerline) >= 2 {
		return uniqueRouteTreeCopperAccessPoints(copper.Centerline)
	}
	shape := copper.Geometry
	if shape.Rect != nil {
		rect := normalizeRoutingRect(*shape.Rect)
		center := routing.Point{
			XMM: (rect.Min.XMM + rect.Max.XMM) / 2,
			YMM: (rect.Min.YMM + rect.Max.YMM) / 2,
		}
		if rect.WidthMM() >= rect.HeightMM() {
			inset := rect.HeightMM() / 2
			minX := min(rect.Min.XMM+inset, center.XMM)
			maxX := max(rect.Max.XMM-inset, center.XMM)
			return uniqueRouteTreeCopperAccessPoints([]routing.Point{
				{XMM: minX, YMM: center.YMM},
				center,
				{XMM: maxX, YMM: center.YMM},
			})
		}
		inset := rect.WidthMM() / 2
		minY := min(rect.Min.YMM+inset, center.YMM)
		maxY := max(rect.Max.YMM-inset, center.YMM)
		return uniqueRouteTreeCopperAccessPoints([]routing.Point{
			{XMM: center.XMM, YMM: minY},
			center,
			{XMM: center.XMM, YMM: maxY},
		})
	}
	if len(shape.Polygon) == 0 {
		return nil
	}
	// routing.Shape currently supports only rectangles and polygons. For
	// large polygons, sample deterministic boundary vertices to keep access
	// pair generation bounded.
	if len(shape.Polygon) <= routeTreeMaxPolygonCopperAccessPoints {
		return uniqueRouteTreeCopperAccessPoints(shape.Polygon)
	}
	points := make([]routing.Point, 0, routeTreeMaxPolygonCopperAccessPoints)
	denominator := max(1, routeTreeMaxPolygonCopperAccessPoints-1)
	for index := 0; index < routeTreeMaxPolygonCopperAccessPoints; index++ {
		sourceIndex := int(math.Round(float64(index) * float64(len(shape.Polygon)-1) / float64(denominator)))
		points = append(points, shape.Polygon[sourceIndex])
	}
	return uniqueRouteTreeCopperAccessPoints(points)
}

// uniqueRouteTreeCopperAccessPoints keeps centerline/shape access candidates deterministic.
func uniqueRouteTreeCopperAccessPoints(points []routing.Point) []routing.Point {
	seen := map[[2]int64]struct{}{}
	out := make([]routing.Point, 0, len(points))
	for _, point := range points {
		if math.IsNaN(point.XMM) || math.IsNaN(point.YMM) || math.IsInf(point.XMM, 0) || math.IsInf(point.YMM, 0) {
			continue
		}
		key := [2]int64{
			int64(math.Round(point.XMM * routeTreeAccessDedupeUnitsPerMM)),
			int64(math.Round(point.YMM * routeTreeAccessDedupeUnitsPerMM)),
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, point)
	}
	// Return the filtered slice, not the caller-owned input.
	return out
}

func normalizeRoutingRect(rect routing.Rect) routing.Rect {
	return routing.Rect{
		Min: routing.Point{
			XMM: min(rect.Min.XMM, rect.Max.XMM),
			YMM: min(rect.Min.YMM, rect.Max.YMM),
		},
		Max: routing.Point{
			XMM: max(rect.Min.XMM, rect.Max.XMM),
			YMM: max(rect.Min.YMM, rect.Max.YMM),
		},
	}
}

func routeTreeBranchAccessPairsForBranch(access []RouteTreeEndpointAccess, netName string, branch InterBlockRouteTreeBranch, cache routeTreeAccessCandidateCache) []routeTreeBranchAccessPair {
	audit := routeTreeBranchAccessAuditForBranch(access, netName, branch, cache)
	return audit.Pairs
}

type routeTreeBranchAccessAudit struct {
	SourceCandidates []routeTreeBranchAccessCandidate
	TargetCandidates []routeTreeBranchAccessCandidate
	Pairs            []routeTreeBranchAccessPair
	TotalPairCount   int
	Limit            int
	Truncated        bool
}

func routeTreeBranchAccessAuditForBranch(access []RouteTreeEndpointAccess, netName string, branch InterBlockRouteTreeBranch, cache routeTreeAccessCandidateCache) routeTreeBranchAccessAudit {
	return routeTreeBranchAccessAuditForBranchWithMergeAudit(access, netName, branch, cache, routeTreeMergeAuditBase{})
}

func routeTreeBranchAccessAuditForBranchWithMergeAudit(access []RouteTreeEndpointAccess, netName string, branch InterBlockRouteTreeBranch, cache routeTreeAccessCandidateCache, mergeAuditBase routeTreeMergeAuditBase) routeTreeBranchAccessAudit {
	sourceOpposite := routeTreeFirstAccessForEndpoint(access, branch.EndEndpointID, netName, cache)
	targetOpposite := routeTreeFirstAccessForEndpoint(access, branch.StartEndpointID, netName, cache)
	sourceCandidates := routeTreeCachedAccessCandidates(cache, access, branch.StartEndpointID, netName, sourceOpposite)
	targetCandidates := routeTreeCachedAccessCandidates(cache, access, branch.EndEndpointID, netName, targetOpposite)
	sourceCandidates = routeTreeAccessCandidatesWithMergePriority(sourceCandidates, mergeAuditBase)
	targetCandidates = routeTreeAccessCandidatesWithMergePriority(targetCandidates, mergeAuditBase)
	sourceCandidates = routeTreeAccessCandidatesWithObstacleRanks(sourceCandidates, mergeAuditBase.OtherNetPadGrid)
	targetCandidates = routeTreeAccessCandidatesWithObstacleRanks(targetCandidates, mergeAuditBase.OtherNetPadGrid)
	totalPairCount := len(sourceCandidates) * len(targetCandidates)
	limit := routeTreeBranchAccessPairLimit
	return routeTreeBranchAccessAudit{
		SourceCandidates: sourceCandidates,
		TargetCandidates: targetCandidates,
		Pairs:            routeTreeBranchAccessPairs(sourceCandidates, targetCandidates, limit),
		TotalPairCount:   totalPairCount,
		Limit:            limit,
		Truncated:        totalPairCount > limit,
	}
}

func routeTreeAccessCandidatesWithMergePriority(candidates []routeTreeBranchAccessCandidate, mergeAuditBase routeTreeMergeAuditBase) []routeTreeBranchAccessCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	hasGeneratedCopper := false
	for _, candidate := range candidates {
		if routeTreeAccessIsGeneratedSameNetCopper(candidate.Access) {
			hasGeneratedCopper = true
			break
		}
	}
	if !hasGeneratedCopper {
		return candidates
	}
	ranked := slices.Clone(candidates)
	for index := range ranked {
		if routeTreeAccessIsGeneratedSameNetCopper(ranked[index].Access) {
			if mergeAuditBase.PreferSameNetCopperMerge {
				ranked[index].RoleRank = routeTreeAccessPreferredRoleRank
				ranked[index].RankReason = joinRouteTreeAccessRankReasons([]string{ranked[index].RankReason, "preferred_same_net_copper_merge"})
			} else {
				// Same-net generated copper remains merge evidence, but it must not
				// outrank an exact target pad when the branch still has a specific
				// endpoint to prove.
				ranked[index].RoleRank += routeTreeGeneratedSameNetCopperNonPreferredRankPenalty
			}
		}
	}
	slices.SortStableFunc(ranked, compareRouteTreeAccessCandidate)
	return ranked
}

func routeTreeAccessCandidatesWithObstacleRanks(candidates []routeTreeBranchAccessCandidate, grid routeTreeOtherNetPadGrid) []routeTreeBranchAccessCandidate {
	if grid.Count == 0 || len(candidates) == 0 {
		return candidates
	}
	ranked := append([]routeTreeBranchAccessCandidate(nil), candidates...)
	for index := range ranked {
		distance, ok := nearestRouteTreeOtherNetPadDistance(grid, routing.Point{
			XMM: ranked[index].Access.XMM,
			YMM: ranked[index].Access.YMM,
		})
		if !ok {
			continue
		}
		switch {
		case distance <= routeTreeImmediateObstaclePressureMM:
			ranked[index].RoleRank += 4
			ranked[index].ObstacleRank = 2
		case distance <= routeTreeNearObstaclePressureMM:
			ranked[index].ObstacleRank = 1
		}
	}
	slices.SortFunc(ranked, compareRouteTreeAccessCandidate)
	return ranked
}

func routeTreeCachedAccessCandidates(cache routeTreeAccessCandidateCache, access []RouteTreeEndpointAccess, endpointID string, netName string, opposite RouteTreeEndpointAccess) []routeTreeBranchAccessCandidate {
	if cache == nil {
		return routeTreeAccessCandidatesForEndpoint(access, endpointID, netName, opposite)
	}
	key := routeTreeAccessCandidateCacheKeyFor(endpointID, netName, opposite)
	if candidates, ok := cache[key]; ok {
		return append([]routeTreeBranchAccessCandidate(nil), candidates...)
	}
	candidates := routeTreeAccessCandidatesForEndpoint(access, endpointID, netName, opposite)
	cache[key] = append([]routeTreeBranchAccessCandidate(nil), candidates...)
	return candidates
}

func routeTreeAccessCandidateCacheKeyFor(endpointID string, netName string, opposite RouteTreeEndpointAccess) routeTreeAccessCandidateCacheKey {
	return routeTreeAccessCandidateCacheKey{
		endpointID:       strings.TrimSpace(endpointID),
		netName:          strings.TrimSpace(netName),
		oppositeRole:     opposite.Role,
		oppositeEndpoint: opposite.EndpointID,
		oppositeRef:      opposite.Ref,
		oppositePad:      opposite.Pad,
		oppositeLayer:    opposite.Layer,
		oppositeXMicron:  routeTreeMicronKey(opposite.XMM),
		oppositeYMicron:  routeTreeMicronKey(opposite.YMM),
	}
}

func routeTreeEndpointBranchRequest(base routing.Request, netName string, start InterBlockRouteGroupEndpoint, end InterBlockRouteGroupEndpoint) routing.Request {
	branchNet := routeTreeEndpointBranchNet(base.Nets, netName, []routing.Endpoint{
		{Ref: start.Ref, Pin: start.Pin},
		{Ref: end.Ref, Pin: end.Pin},
	})
	return routing.Request{
		Board:      base.Board,
		Components: base.Components,
		Obstacles:  base.Obstacles,
		Existing:   base.Existing,
		Nets:       []routing.Net{branchNet},
		Rules:      base.Rules,
		Strategy:   base.Strategy,
		Seed:       base.Seed,
	}
}

func routeTreeEndpointBranchNet(nets []routing.Net, netName string, endpoints []routing.Endpoint) routing.Net {
	branchNet, _ := routeTreeAccessBranchNetSet(nets, netName, endpoints)
	return branchNet
}

func routeTreeFirstAccessForEndpoint(access []RouteTreeEndpointAccess, endpointID string, netName string, cache routeTreeAccessCandidateCache) RouteTreeEndpointAccess {
	candidates := routeTreeCachedAccessCandidates(cache, access, endpointID, netName, RouteTreeEndpointAccess{})
	if len(candidates) == 0 {
		return RouteTreeEndpointAccess{}
	}
	return candidates[0].Access
}

func routeTreeAccessBranchNetSet(nets []routing.Net, netName string, endpoints []routing.Endpoint) (routing.Net, []routing.Net) {
	trimmedNetName := strings.TrimSpace(netName)
	branchNet := routing.Net{Name: trimmedNetName}
	out := make([]routing.Net, 0, len(nets)+1)
	matched := false
	for _, net := range nets {
		netNameMatches := strings.EqualFold(strings.TrimSpace(net.Name), trimmedNetName)
		if netNameMatches && matched {
			continue
		}
		if netNameMatches {
			branchNet = net
			branchNet.Fixed = false
			branchNet.Endpoints = append([]routing.Endpoint(nil), endpoints...)
			out = append(out, branchNet)
			matched = true
			continue
		}
		net.Endpoints = append([]routing.Endpoint(nil), net.Endpoints...)
		net.Fixed = true
		out = append(out, net)
	}
	if strings.TrimSpace(branchNet.Name) == "" {
		branchNet.Name = trimmedNetName
	}
	if !matched {
		branchNet.Fixed = false
		branchNet.Endpoints = append([]routing.Endpoint(nil), endpoints...)
		out = append(out, branchNet)
	}
	return branchNet, out
}

func routeTreeSyntheticAccessComponent(ref string, netName string, access RouteTreeEndpointAccess, defaultLayer string) routing.Component {
	layer := routeBranchCanonicalLayer(access.Layer, defaultLayer)
	return routing.Component{
		Ref:   ref,
		Fixed: true,
		Position: routing.Placement{
			Layer: layer,
		},
		Pads: []routing.Pad{{
			Ref:      ref,
			Name:     "1",
			Net:      netName,
			Position: routing.Point{XMM: access.XMM, YMM: access.YMM},
			Shape:    routing.PadCircle,
			Type:     routing.PadSMD,
			Size:     routing.Size{WidthMM: routeTreeSyntheticAccessPadSizeMM, HeightMM: routeTreeSyntheticAccessPadSizeMM},
			Layers:   []string{layer},
		}},
	}
}

func routeTreeSyntheticAccessRef(kind string, rank int) string {
	return fmt.Sprintf("__KICADAI_RT_%s_%d", kind, rank)
}

func routeTreeBranchesForRouting(branches []InterBlockRouteTreeBranch) []InterBlockRouteTreeBranch {
	if len(branches) == 0 {
		return nil
	}
	ordered := slices.Clone(branches)
	slices.SortFunc(ordered, compareRouteTreeBranchForRouting)
	return ordered
}

func routeTreeBranchesForRoutingWithAccess(branches []InterBlockRouteTreeBranch, access []RouteTreeEndpointAccess, netName string, cache routeTreeAccessCandidateCache) []InterBlockRouteTreeBranch {
	if len(branches) == 0 {
		return nil
	}
	ranked := make([]routeTreeRankedBranchForRouting, 0, len(branches))
	for _, branch := range branches {
		ranked = append(ranked, routeTreeRankedBranchForRouting{
			Branch: branch,
			Rank:   routeTreeBranchAccessConstraintRank(branch, access, netName, cache),
		})
	}
	slices.SortFunc(ranked, func(left, right routeTreeRankedBranchForRouting) int {
		if compare := cmp.Compare(left.Rank, right.Rank); compare != 0 {
			return compare
		}
		return compareRouteTreeBranchForRouting(left.Branch, right.Branch)
	})
	ordered := make([]InterBlockRouteTreeBranch, 0, len(ranked))
	for _, item := range ranked {
		ordered = append(ordered, item.Branch)
	}
	return ordered
}

type routeTreeRankedBranchForRouting struct {
	Branch InterBlockRouteTreeBranch
	Rank   int
}

func routeTreeBranchAccessConstraintRank(branch InterBlockRouteTreeBranch, access []RouteTreeEndpointAccess, netName string, cache routeTreeAccessCandidateCache) int {
	sourceCandidates := routeTreeCachedAccessCandidates(cache, access, branch.StartEndpointID, netName, RouteTreeEndpointAccess{})
	targetCandidates := routeTreeCachedAccessCandidates(cache, access, branch.EndEndpointID, netName, RouteTreeEndpointAccess{})
	if len(sourceCandidates) == 0 || len(targetCandidates) == 0 {
		return int(^uint(0) >> 1)
	}
	return len(sourceCandidates) + len(targetCandidates)
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
				Kind:       routing.CopperSegment,
				Net:        segment.Net,
				Layer:      routeBranchCanonicalLayer(segment.Layer, defaultLayer),
				Geometry:   routeBranchSegmentShape(segment, rules),
				Centerline: []routing.Point{segment.Start, segment.End},
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
