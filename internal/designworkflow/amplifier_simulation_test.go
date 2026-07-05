package designworkflow

import (
	"context"
	"testing"

	"kicadai/internal/blocks"
)

func TestClassABOutputPairPlanningSummarizesSimulationNotRun(t *testing.T) {
	request := readClassABHeadphoneFixture(t)
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary, ok := plan.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if !ok {
		t.Fatalf("amplifier output summary = %#v", plan.Stage.Summary["amplifier_output_stage"])
	}
	if summary.BlockID != "class_ab_output_pair" || summary.SimulationStatus != "not_run" {
		t.Fatalf("amplifier summary = %#v, want class_ab_output_pair simulation_status=not_run", summary)
	}
}
