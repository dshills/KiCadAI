package designworkflow

import (
	"slices"
	"testing"
)

func TestSkippedCreateStagesFollowPipelineContract(t *testing.T) {
	stages := skippedCreateStages(StageSchematicElectrical, "schematic electrical rules did not pass")
	got := make([]StageName, 0, len(stages))
	for _, stage := range stages {
		if stage.Status != StageStatusSkipped {
			t.Fatalf("stage %s status = %s, want skipped", stage.Name, stage.Status)
		}
		got = append(got, stage.Name)
	}
	want := []StageName{StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks}
	if !slices.Equal(got, want) {
		t.Fatalf("skipped stages = %v, want %v", got, want)
	}
}

func TestSkippedCreateStagesRejectsUnknownStage(t *testing.T) {
	if stages := skippedCreateStages(StageSimulation, "not part of create pipeline"); stages != nil {
		t.Fatalf("unknown stage skipped stages = %#v, want nil", stages)
	}
}
