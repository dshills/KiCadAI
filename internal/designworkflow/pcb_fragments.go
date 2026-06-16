package designworkflow

import (
	"context"
	"fmt"
	"math"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

const (
	defaultFragmentSpacingXMM = 25.0
	defaultFragmentSpacingYMM = 18.0
	defaultFragmentMarginMM   = 5.0
)

type PCBFragmentResult struct {
	Fragments []BlockFragment `json:"fragments,omitempty"`
	Stage     StageResult     `json:"stage"`
}

type BlockFragment struct {
	InstanceID  string                           `json:"instance_id"`
	BlockID     string                           `json:"block_id"`
	OriginXMM   float64                          `json:"origin_x_mm"`
	OriginYMM   float64                          `json:"origin_y_mm"`
	Realization blocks.BlockPCBRealizationResult `json:"realization"`
}

func RealizePCBFragments(ctx context.Context, registry blocks.Registry, plan BlockPlanResult) PCBFragmentResult {
	var issues []reports.Issue
	if ctx == nil {
		issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "context", Message: "context is required"})
	} else if err := ctx.Err(); err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "context", Message: err.Error()})
	}
	if registry == nil {
		issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "registry", Message: "block registry is required"})
	}
	if reports.HasBlockingIssue(issues) {
		return PCBFragmentResult{Stage: NewStageResult(StagePCBRealization, issues)}
	}
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		return PCBFragmentResult{Stage: StageResult{Name: StagePCBRealization, Status: StageStatusSkipped, Summary: map[string]any{"reason": "block planning did not complete"}}}
	}
	request := NormalizeRequest(plan.Request)
	columns := fragmentColumnCount(request)
	fragments := make([]BlockFragment, 0, len(request.Blocks))
	for index, instance := range request.Blocks {
		if err := ctx.Err(); err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "context", Message: err.Error()})
			break
		}
		definition, ok := registry.GetBlock(instance.BlockID)
		if !ok {
			issues = append(issues, reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: fmt.Sprintf("blocks[%d].block_id", index), Message: "block not found: " + instance.BlockID})
			continue
		}
		output, instantiateIssues := registry.Instantiate(ctx, blocks.BlockRequest{BlockID: instance.BlockID, InstanceID: instance.ID, Params: instance.Params})
		issues = append(issues, prefixIssues(fmt.Sprintf("blocks[%d]", index), instantiateIssues)...)
		if reports.HasBlockingIssue(instantiateIssues) {
			continue
		}
		originX, originY := fragmentOrigin(index, columns)
		realization := blocks.RealizeBlockPCB(definition, output, blocks.PCBRealizationOptions{OriginXMM: originX, OriginYMM: originY})
		issues = append(issues, prefixIssues(fmt.Sprintf("blocks[%d].pcb_realization", index), realization.Issues)...)
		fragments = append(fragments, BlockFragment{
			InstanceID:  instance.ID,
			BlockID:     instance.BlockID,
			OriginXMM:   originX,
			OriginYMM:   originY,
			Realization: realization,
		})
	}
	issues = append(issues, validateFragmentBounds(request, fragments)...)
	componentCount, routeCount := fragmentCounts(fragments)
	stage := NewStageResult(StagePCBRealization, issues)
	stage.Summary = map[string]any{
		"block_count":     len(request.Blocks),
		"fragment_count":  len(fragments),
		"component_count": componentCount,
		"local_routes":    routeCount,
	}
	return PCBFragmentResult{Fragments: fragments, Stage: stage}
}

func fragmentColumnCount(request Request) int {
	count := len(request.Blocks)
	if count == 0 {
		return 1
	}
	columns := int(math.Ceil(math.Sqrt(float64(count))))
	if request.Board.WidthMM > 0 {
		maxColumns := int(math.Max(1, math.Floor((request.Board.WidthMM-defaultFragmentMarginMM*2)/defaultFragmentSpacingXMM)+1))
		if maxColumns < columns {
			columns = maxColumns
		}
	}
	if columns < 1 {
		return 1
	}
	return columns
}

func fragmentOrigin(index int, columns int) (float64, float64) {
	if columns < 1 {
		columns = 1
	}
	column := index % columns
	row := index / columns
	return defaultFragmentMarginMM + float64(column)*defaultFragmentSpacingXMM, defaultFragmentMarginMM + float64(row)*defaultFragmentSpacingYMM
}

func validateFragmentBounds(request Request, fragments []BlockFragment) []reports.Issue {
	var issues []reports.Issue
	if request.Board.WidthMM <= 0 || request.Board.HeightMM <= 0 {
		return nil
	}
	for _, fragment := range fragments {
		for _, component := range fragment.Realization.Components {
			if component.Placement.XMM < 0 || component.Placement.YMM < 0 || component.Placement.XMM > request.Board.WidthMM || component.Placement.YMM > request.Board.HeightMM {
				issues = append(issues, reports.Issue{
					Code:     reports.CodePlacementOutsideBoard,
					Severity: reports.SeverityWarning,
					Path:     "pcb_realization." + fragment.InstanceID,
					Message:  "fragment component placement exceeds board outline",
					Refs:     []string{component.Ref},
				})
			}
		}
	}
	return issues
}

func fragmentCounts(fragments []BlockFragment) (int, int) {
	componentCount := 0
	routeCount := 0
	for _, fragment := range fragments {
		componentCount += len(fragment.Realization.Components)
		routeCount += len(fragment.Realization.LocalRoutes)
	}
	return componentCount, routeCount
}

func prefixIssues(prefix string, issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	prefixed := cloneIssues(issues)
	for i := range prefixed {
		if prefixed[i].Path == "" {
			prefixed[i].Path = prefix
		} else {
			prefixed[i].Path = prefix + "." + prefixed[i].Path
		}
	}
	return prefixed
}
