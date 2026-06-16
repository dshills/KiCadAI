package designworkflow

import (
	"context"
	"strconv"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

type BlockPlanResult struct {
	Request     Request                   `json:"request"`
	Composition blocks.CompositionRequest `json:"composition"`
	Output      blocks.CompositionOutput  `json:"output"`
	Stage       StageResult               `json:"stage"`
}

func PlanBlocks(ctx context.Context, registry blocks.Registry, request Request) BlockPlanResult {
	normalized := NormalizeRequest(request)
	var issues []reports.Issue
	issues = append(issues, ValidateRequest(normalized)...)
	if registry == nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "registry",
			Message:  "block registry is required",
		})
	}
	composition, compositionIssues := ToCompositionRequest(normalized)
	issues = append(issues, compositionIssues...)
	if !reports.HasBlockingIssue(issues) && registry != nil {
		issues = append(issues, validateBlocksAgainstRegistry(registry, normalized)...)
	}
	if reports.HasBlockingIssue(issues) {
		return BlockPlanResult{
			Request:     normalized,
			Composition: composition,
			Stage:       NewStageResult(StageBlockPlanning, issues),
		}
	}
	output := blocks.ComposeBlocks(ctx, registry, composition)
	issues = append(issues, output.Issues...)
	stage := NewStageResult(StageBlockPlanning, issues)
	stage.Summary = map[string]any{
		"block_count":      len(normalized.Blocks),
		"connection_count": len(normalized.Connections),
		"operation_count":  len(output.Operations),
	}
	return BlockPlanResult{
		Request:     normalized,
		Composition: composition,
		Output:      output,
		Stage:       stage,
	}
}

func validateBlocksAgainstRegistry(registry blocks.Registry, request Request) []reports.Issue {
	var issues []reports.Issue
	for index, instance := range request.Blocks {
		definition, ok := registry.GetBlock(instance.BlockID)
		if !ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityError,
				Path:     "blocks[" + strconv.Itoa(index) + "].block_id",
				Message:  "block not found: " + instance.BlockID,
			})
			continue
		}
		blockIssues := registry.ValidateRequest(blocks.BlockRequest{
			BlockID:    definition.ID,
			InstanceID: instance.ID,
			Params:     instance.Params,
		})
		for _, issue := range blockIssues {
			if issue.Path != "" {
				issue.Path = "blocks[" + strconv.Itoa(index) + "]." + issue.Path
			}
			issues = append(issues, issue)
		}
	}
	return issues
}
