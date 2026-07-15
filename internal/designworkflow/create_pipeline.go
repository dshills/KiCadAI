package designworkflow

// createPipelineStages is the ordered stage contract for block-planned project
// creation. Optional repair and fabrication stages are added by Create only
// after their prerequisite evidence exists.
var createPipelineStages = []StageName{
	StageBlockPlanning,
	StageComponentSelection,
	StageSchematic,
	StageSchematicElectrical,
	StagePCBRealization,
	StagePlacement,
	StageRouting,
	StageProjectWrite,
	StageWriterCorrect,
	StageValidation,
	StageKiCadChecks,
}

func skippedCreateStages(failed StageName, reason string) []StageResult {
	for index, stage := range createPipelineStages {
		if stage == failed {
			return skippedWorkflowStages(reason, createPipelineStages[index+1:]...)
		}
	}
	return nil
}

func blockedCreateResult(request Request, opts CreateOptions, stages []StageResult, failed StageName, reason string) WorkflowResult {
	stages = append(stages, skippedCreateStages(failed, reason)...)
	return BuildWorkflowResult(ProjectSummary{Name: request.Name, OutputDir: opts.OutputDir}, request.Validation.Acceptance, stages)
}
