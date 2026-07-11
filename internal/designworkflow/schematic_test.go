package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
)

func TestApplySchematicWritesProject(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues = %#v", plan.Stage.Issues)
	}
	outputDir := filepath.Join(t.TempDir(), "status_board")
	result := ApplySchematic(context.Background(), plan, SchematicApplyOptions{OutputDir: outputDir, Overwrite: true, Seed: "test"})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("apply issues = %#v", result.Stage.Issues)
	}
	for _, path := range []string{
		filepath.Join(outputDir, "status_board.kicad_pro"),
		filepath.Join(outputDir, "status_board.kicad_sch"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing generated file %s: %v", path, err)
		}
	}
	if result.Stage.Summary["operation_count"].(int) == 0 || len(result.ApplyResult.Artifacts) == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestApplySchematicRequiresOutput(t *testing.T) {
	plan := BlockPlanResult{Stage: NewStageResult(StageBlockPlanning, nil)}
	result := ApplySchematic(context.Background(), plan, SchematicApplyOptions{})
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want blocking output issue", result.Stage.Issues)
	}
}

func TestApplySchematicPropagatesPlanFailure(t *testing.T) {
	plan := BlockPlanResult{Stage: NewStageResult(StageBlockPlanning, []reports.Issue{{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "blocks[0]",
		Message:  "bad block",
	}})}
	result := ApplySchematic(context.Background(), plan, SchematicApplyOptions{OutputDir: t.TempDir()})
	if result.Stage.Status != StageStatusSkipped || len(result.Transaction.Operations) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestApplySchematicChecksContext(t *testing.T) {
	result := ApplySchematic(nil, BlockPlanResult{Stage: NewStageResult(StageBlockPlanning, nil)}, SchematicApplyOptions{OutputDir: t.TempDir()})
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want context issue", result.Stage.Issues)
	}
}

func TestSchematicStageIncludesReadabilitySummary(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "readable_status",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	stage := schematicStageFromPlan(plan)
	readability, ok := stage.Summary["readability"].(map[string]any)
	if !ok {
		t.Fatalf("readability summary missing: %#v", stage.Summary)
	}
	if readability["profile"] == "" || readability["diagonal_wire_count"] == nil {
		t.Fatalf("readability summary = %#v", readability)
	}
	if readability["rule_profile"] != schematiclayout.RuleProfileStandard {
		t.Fatalf("rule_profile = %#v, want standard; summary=%#v", readability["rule_profile"], readability)
	}
	if got := summaryInt(t, readability, "rule_count"); got == 0 {
		t.Fatalf("rule_count = %d, want nonzero; summary=%#v", got, readability)
	}
	if readability["repair_guidance_available"] != true {
		t.Fatalf("repair_guidance_available = %#v, want true; summary=%#v", readability["repair_guidance_available"], readability)
	}
	if got := summaryInt(t, readability, "repair_guidance_count"); got == 0 {
		t.Fatalf("repair_guidance_count = %d, want nonzero repairable diagnostics; summary=%#v", got, readability)
	}
	if got := summaryInt(t, readability, "diagonal_wire_count"); got != 0 {
		t.Fatalf("diagonal_wire_count = %d, want 0; summary=%#v", got, readability)
	}
	if got := summaryInt(t, readability, "decode_error_count"); got != 0 {
		t.Fatalf("decode_error_count = %d, want 0; summary=%#v", got, readability)
	}
}

func TestSchematicStageOpAmpReadabilitySummaryIsClean(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "readable_opamp",
		Board:   BoardSpec{WidthMM: 60, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "amp",
			BlockID: "opamp_gain_stage",
			Params: map[string]any{
				"input_coupling":          "ac",
				"include_output_resistor": true,
			},
		}},
	}
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues = %#v", plan.Stage.Issues)
	}
	stage := schematicStageFromPlan(plan)
	readability, ok := stage.Summary["readability"].(map[string]any)
	if !ok {
		t.Fatalf("readability summary missing: %#v", stage.Summary)
	}
	if got := summaryInt(t, readability, "diagonal_wire_count"); got != 0 {
		t.Fatalf("diagonal_wire_count = %d, want 0; summary=%#v", got, readability)
	}
	if got := summaryInt(t, readability, "stage_order_violation_count"); got != 0 {
		t.Fatalf("stage_order_violation_count = %d, want 0; summary=%#v", got, readability)
	}
	if got := summaryInt(t, readability, "power_placement_violation_count"); got != 0 {
		t.Fatalf("power_placement_violation_count = %d, want 0; summary=%#v", got, readability)
	}
	roles, ok := readability["roles"].(map[string]string)
	if !ok || len(roles) == 0 {
		t.Fatalf("roles missing from readability summary: %#v", readability)
	}
}

func TestConcreteI2CSensorReadabilitySummariesAreClean(t *testing.T) {
	tests := []struct {
		name        string
		componentID string
		address     string
	}{
		{name: "bme280", componentID: "sensor.bosch.bme280.lga8", address: "0x76"},
		{name: "bmp280", componentID: "sensor.bosch.bmp280.lga8", address: "0x76"},
		{name: "sht31", componentID: "sensor.sensirion.sht31_dis.dfn8", address: "0x44"},
	}
	registry := blocks.NewBuiltinRegistry()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			plan := PlanBlocks(ctx, registry, Request{
				Version: RequestVersion,
				Name:    tt.name,
				Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
				Blocks: []BlockInstanceSpec{{
					ID:      "sensor",
					BlockID: "i2c_sensor",
					Params: map[string]any{
						"sensor_component_id": tt.componentID,
						"i2c_address":         tt.address,
						"supply_voltage":      "3.3V",
					},
				}},
			})
			if err := ctx.Err(); err != nil {
				t.Fatalf("planning context: %v", err)
			}
			if reports.HasBlockingIssue(plan.Stage.Issues) {
				t.Fatalf("plan issues = %#v", plan.Stage.Issues)
			}
			stage := schematicStageFromPlan(plan)
			readability, ok := stage.Summary["readability"].(map[string]any)
			if !ok {
				t.Fatalf("readability summary missing: %#v", stage.Summary)
			}
			for _, key := range []string{"diagonal_wire_count", "decode_error_count", "stage_order_violation_count", "power_placement_violation_count"} {
				if got := summaryInt(t, readability, key); got != 0 {
					t.Errorf("%s = %d, want 0; summary=%#v", key, got, readability)
				}
			}
		})
	}
}

func summaryInt(t *testing.T, summary map[string]any, key string) int {
	t.Helper()
	value, ok := summary[key]
	if !ok {
		t.Fatalf("summary[%s] missing from %#v", key, summary)
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		t.Fatalf("summary[%s] = %#v, want numeric", key, value)
	}
	return 0
}
