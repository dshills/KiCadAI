package designworkflow

import (
	"context"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

func TestPlanBlocksComposesExplicitRequest(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"SIG", "GND"}}},
			{ID: "status", BlockID: "led_indicator"},
		},
		Connections: []ConnectionSpec{{From: "header.SIG", To: "status.IN", NetAlias: "STATUS"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("PlanBlocks issues = %#v", result.Stage.Issues)
	}
	if result.Stage.Status != StageStatusOK || len(result.Output.Operations) == 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.Composition.ProjectName != "status_board" {
		t.Fatalf("project name = %q", result.Composition.ProjectName)
	}
	evidence, ok := result.Stage.Summary["block_evidence"].([]BlockEvidenceSummary)
	if !ok || len(evidence) != 2 {
		t.Fatalf("block evidence = %#v", result.Stage.Summary["block_evidence"])
	}
	if evidence[1].BlockID != "led_indicator" || evidence[1].EvidenceLevel == "" {
		t.Fatalf("LED evidence = %#v", evidence[1])
	}
	if evidence[1].Readiness != blocks.BlockReadinessPartial {
		t.Fatalf("LED readiness = %q", evidence[1].Readiness)
	}
	if !slices.Contains(evidence[1].ValidationRules, "led.series_route.required") {
		t.Fatalf("LED validation rules = %#v", evidence[1].ValidationRules)
	}
	if !slices.Contains(evidence[1].RequiredRoutes, "series") {
		t.Fatalf("LED required routes = %#v", evidence[1].RequiredRoutes)
	}
}

func TestPlanBlocksComposesExpandedVerifiedCoverageBlocks(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "protected_mcu_support",
		Board:   BoardSpec{WidthMM: 60, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "clk", BlockID: "crystal_oscillator"},
			{ID: "prog", BlockID: "reset_programming_header"},
			{ID: "esd", BlockID: "esd_protection"},
			{ID: "polarity", BlockID: "reverse_polarity_protection"},
		},
	}
	result := PlanBlocks(context.Background(), newExpandedCoverageTestRegistry(), request)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("PlanBlocks issues = %#v", result.Stage.Issues)
	}
	if result.Stage.Status != StageStatusWarning || len(result.Output.Instances) != len(request.Blocks) {
		t.Fatalf("result = %#v", result)
	}
	evidence, ok := result.Stage.Summary["block_evidence"].([]BlockEvidenceSummary)
	if !ok || len(evidence) != len(request.Blocks) {
		t.Fatalf("block evidence = %#v", result.Stage.Summary["block_evidence"])
	}
	requiredRoutesByBlock := map[string][]string{}
	for _, summary := range evidence {
		if summary.Readiness != blocks.BlockReadinessPartial {
			t.Fatalf("%s readiness = %q", summary.BlockID, summary.Readiness)
		}
		if summary.VerificationLevel != string(blocks.VerificationStructural) {
			t.Fatalf("%s verification = %#v", summary.BlockID, summary)
		}
		requiredRoutesByBlock[summary.BlockID] = summary.RequiredRoutes
	}
	assertRequiredRouteSet(t, requiredRoutesByBlock, "esd_protection", []string{"esd_signal_entry_to_tvs", "esd_tvs_to_ground"})
	assertRequiredRouteSet(t, requiredRoutesByBlock, "reverse_polarity_protection", []string{"diode_to_protected_output", "raw_input_to_diode"})
}

func TestPlanBlocksSummarizesClassABOutputStageRationale(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_summary",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "coupling", BlockID: "dc_blocking_capacitor"},
		},
		Connections: []ConnectionSpec{{From: "output.AMP_OUT", To: "coupling.IN", NetAlias: "AMP_OUT_DC_BIASED"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("PlanBlocks issues = %#v", result.Stage.Issues)
	}
	summary, ok := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if !ok {
		t.Fatalf("amplifier summary = %#v", result.Stage.Summary["amplifier_output_stage"])
	}
	if summary.Readiness != "headphone_connectivity" || !summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
	if !slices.Contains(summary.OutputDevices, "bjt.onsemi.mmbt3904.sot23") {
		t.Fatalf("output devices = %#v", summary.OutputDevices)
	}
	summaries, ok := result.Stage.Summary["amplifier_output_stages"].([]AmplifierOutputStageSummary)
	if !ok || len(summaries) != 1 {
		t.Fatalf("amplifier summaries = %#v", result.Stage.Summary["amplifier_output_stages"])
	}
}

func TestAmplifierOutputStageSummaryKeepsWarningsOutOfBlockers(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_warning_summary",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{
				"topology":                  "diode_string",
				"supply_voltage":            "9V",
				"load_impedance":            "32Ω",
				"upper_output_component_id": "bjt.example.power_npn.to220",
				"lower_output_component_id": "bjt.example.power_pnp.to220",
			}},
			{ID: "coupling", BlockID: "dc_blocking_capacitor"},
		},
		Connections: []ConnectionSpec{{From: "output.AMP_OUT", To: "coupling.IN", NetAlias: "AMP_OUT_DC_BIASED"}},
	}
	summary := amplifierOutputStageSummary(request, []reports.Issue{{
		Severity: reports.SeverityWarning,
		Path:     "blocks[0].params",
		Message:  "style warning",
	}}, request.Blocks[0], 0, blocksByInstanceID(request))
	if summary.Readiness != "headphone_connectivity" || slices.Contains(summary.Blockers, "style warning") {
		t.Fatalf("summary = %#v", summary)
	}
	if !slices.Contains(summary.Notes, "style warning") {
		t.Fatalf("notes = %#v", summary.Notes)
	}
	if !strings.Contains(strings.Join(summary.Notes, "\n"), "bjt.example.power_npn.to220, bjt.example.power_pnp.to220") {
		t.Fatalf("notes do not mention selected output devices: %#v", summary.Notes)
	}
}

func TestPlanBlocksSummarizesMissingClassABDCBlocking(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_missing_dc_block",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary, ok := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if !ok {
		t.Fatalf("amplifier summary = %#v", result.Stage.Summary["amplifier_output_stage"])
	}
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
	if !slices.Contains(summary.Blockers, "single-supply headphone outputs require a DC blocking capacitor before the load") {
		t.Fatalf("blockers = %#v", summary.Blockers)
	}
}

func TestPlanBlocksSummarizesMultipleClassABOutputStages(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "stereo_class_ab_summary",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 60, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "left_output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "left_coupling", BlockID: "dc_blocking_capacitor"},
			{ID: "right_output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
		},
		Connections: []ConnectionSpec{{From: "left_output.AMP_OUT", To: "left_coupling.IN", NetAlias: "LEFT_DC_BIASED"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if _, ok := result.Stage.Summary["amplifier_output_stage"]; ok {
		t.Fatalf("single-stage compatibility summary should be omitted for multiple stages: %#v", result.Stage.Summary)
	}
	summaries, ok := result.Stage.Summary["amplifier_output_stages"].([]AmplifierOutputStageSummary)
	if !ok || len(summaries) != 2 {
		t.Fatalf("amplifier summaries = %#v", result.Stage.Summary["amplifier_output_stages"])
	}
	byInstance := map[string]AmplifierOutputStageSummary{}
	for _, summary := range summaries {
		byInstance[summary.InstanceID] = summary
	}
	if byInstance["left_output"].Readiness != "headphone_connectivity" || !byInstance["left_output"].DCBlockingPresent {
		t.Fatalf("left summary = %#v", byInstance["left_output"])
	}
	if byInstance["right_output"].Readiness != "blocked" || byInstance["right_output"].DCBlockingPresent {
		t.Fatalf("right summary = %#v", byInstance["right_output"])
	}
}

func TestPlanBlocksRequiresClassABDCBlockingOnOutputConnection(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_unrelated_dc_block",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "unused_coupling", BlockID: "dc_blocking_capacitor"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksDetectsClassABDCBlockingOnSameOutputNetAlias(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_dc_block_same_alias",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "series_placeholder", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"A", "B"}}},
			{ID: "coupling", BlockID: "dc_blocking_capacitor"},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "series_placeholder.A", NetAlias: "AMP_OUT_DC_BIASED"},
			{From: "series_placeholder.B", To: "coupling.IN", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "headphone_connectivity" || !summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksDetectsClassABHeadphoneProtectionAsOutputCoupling(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_headphone_protected",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "protection", BlockID: "headphone_output_protection"},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "protection.AMP_OUT", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "headphone_connectivity" || !summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsHeadphoneProtectionWithoutDCBlockingAsOutputCoupling(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_headphone_unblocked",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "protection", BlockID: "headphone_output_protection", Params: map[string]any{"dc_blocking_capacitance": "0F"}},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "protection.AMP_OUT", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsHeadphoneProtectionOutputPortAsDCCouplingInput(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_headphone_wrong_protection_port",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "protection", BlockID: "headphone_output_protection"},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "protection.HP_OUT", NetAlias: "HP_OUT"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksAcceptsFractionalHeadphoneProtectionCapacitance(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_headphone_fractional_cap",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "protection", BlockID: "headphone_output_protection", Params: map[string]any{"dc_blocking_capacitance": "0.47uF"}},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "protection.AMP_OUT", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "headphone_connectivity" || !summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsScientificZeroProtectionCapacitance(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_headphone_zero_scientific_cap",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "protection", BlockID: "headphone_output_protection", Params: map[string]any{"dc_blocking_capacitance": "0e-6F"}},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "protection.AMP_OUT", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsInvalidSignZeroProtectionCapacitance(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_headphone_zero_invalid_sign_cap",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "protection", BlockID: "headphone_output_protection", Params: map[string]any{"dc_blocking_capacitance": "0-0.1F"}},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "protection.AMP_OUT", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsMalformedProtectionCapacitance(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_headphone_malformed_cap",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "protection", BlockID: "headphone_output_protection", Params: map[string]any{"dc_blocking_capacitance": "not-a-capacitor"}},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "protection.AMP_OUT", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksDetectsClassABDCBlockingOnNetOnlyOutputAlias(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_dc_block_net_alias",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "coupling", BlockID: "dc_blocking_capacitor"},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", NetAlias: "AMP_OUT_DC_BIASED"},
			{From: "coupling.IN", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	if !classABHasOutputCoupling(request, request.Blocks[0], blocksByInstanceID(request)) {
		t.Fatal("expected net-alias-only coupling to be detected by helper")
	}
}

func TestPlanBlocksDetectsClassABDCBlockingAtCapacitorOutputPort(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_dc_block_output_port",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "coupling", BlockID: "dc_blocking_capacitor"},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "coupling.OUT", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	if !classABHasOutputCoupling(request, request.Blocks[0], blocksByInstanceID(request)) {
		t.Fatal("expected coupling connected through capacitor OUT to be detected")
	}
}

func TestPlanBlocksRejectsZeroLegacyDCCapacitorAsOutputCoupling(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_zero_legacy_dc_cap",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "coupling", BlockID: "dc_blocking_capacitor", Params: map[string]any{"capacitance": "0F"}},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "coupling.IN", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsMalformedLegacyDCCapacitorAsOutputCoupling(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_malformed_legacy_dc_cap",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "coupling", BlockID: "dc_blocking_capacitor", Params: map[string]any{"capacitance": "invalid"}},
		},
		Connections: []ConnectionSpec{
			{From: "output.AMP_OUT", To: "coupling.IN", NetAlias: "AMP_OUT_DC_BIASED"},
		},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || summary.DCBlockingPresent {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksDoesNotRequireClassABDCBlockingForDualRail(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_dual_rail",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "power", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"VCC", "VEE"}}},
		},
		Connections: []ConnectionSpec{{From: "power.VEE", To: "output.VEE", NetAlias: "VEE"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary, ok := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if !ok {
		t.Fatalf("amplifier summary = %#v", result.Stage.Summary["amplifier_output_stage"])
	}
	if summary.Readiness != "headphone_connectivity" || slices.Contains(summary.Blockers, "single-supply headphone outputs require a DC blocking capacitor before the load") {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksDetectsDualRailClassABWithoutNetAlias(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_dual_rail_unaliased",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "power", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"VCC", "VEE"}}},
		},
		Connections: []ConnectionSpec{{From: "power.VEE", To: "output.VEE"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "headphone_connectivity" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksTreatsVSSAsClassABNegativeRail(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_vss_rail",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "power", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"VDD", "VSS"}}},
		},
		Connections: []ConnectionSpec{{From: "power.VSS", To: "output.VEE", NetAlias: "VSS"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "headphone_connectivity" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksTreatsUnaliasedVSSPortAsClassABNegativeRail(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_unaliased_vss_rail",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "power", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"VDD", "VSS"}}},
		},
		Connections: []ConnectionSpec{{From: "power.VSS", To: "output.VEE"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "headphone_connectivity" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksDetectsDualRailClassABWithLowercaseVEEEndpoint(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_lowercase_vee_endpoint",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
			{ID: "power", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"VCC", "VEE"}}},
		},
		Connections: []ConnectionSpec{{From: "power.VEE", To: "output.vee", NetAlias: "VEE"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "headphone_connectivity" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsArbitraryClassABVEENetAliasAsNegativeRail(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_unknown_vee_alias",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
		},
		Connections: []ConnectionSpec{{From: "output.VEE", To: "output.LOAD_REF", NetAlias: "BIAS_REFERENCE"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || !slices.Contains(summary.Blockers, "single-supply headphone outputs require a DC blocking capacitor before the load") {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksRejectsNegativeSignalAliasAsClassABNegativeRail(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_negative_signal_alias",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
		},
		Connections: []ConnectionSpec{{From: "output.VEE", To: "output.LOAD_REF", NetAlias: "-IN"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || !slices.Contains(summary.Blockers, "single-supply headphone outputs require a DC blocking capacitor before the load") {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanBlocksTreatsAGNDAsClassABLoadReference(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "class_ab_agnd_reference",
		Board:   BoardSpec{WidthMM: 70, HeightMM: 40, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "output", BlockID: "class_ab_output_stage", Params: map[string]any{"topology": "diode_string", "supply_voltage": "9V", "load_impedance": "32Ω"}},
		},
		Connections: []ConnectionSpec{{From: "output.VEE", To: "output.LOAD_REF", NetAlias: "AGND"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	summary := result.Stage.Summary["amplifier_output_stage"].(AmplifierOutputStageSummary)
	if summary.Readiness != "blocked" || !slices.Contains(summary.Blockers, "single-supply headphone outputs require a DC blocking capacitor before the load") {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestIssueBelongsToBlockRejectsNeighborIndex(t *testing.T) {
	if !issueBelongsToBlock(reports.Issue{Path: "blocks[1].params.load_impedance"}, 1) {
		t.Fatal("expected exact indexed block path to match")
	}
	for _, path := range []string{"blocks[10].params.load_impedance", "blocks[1]0.params.load_impedance", "blocks[1]_suffix.params.load_impedance"} {
		if issueBelongsToBlock(reports.Issue{Path: path}, 1) {
			t.Fatalf("path %q should not match block index 1", path)
		}
	}
}

func assertRequiredRouteSet(t *testing.T, routesByBlock map[string][]string, blockID string, want []string) {
	t.Helper()
	got, ok := routesByBlock[blockID]
	if !ok {
		t.Fatalf("block %s missing from evidence summary", blockID)
	}
	got = slices.Clone(got)
	want = slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s required routes = %#v, want exactly %#v", blockID, got, want)
	}
}

func TestPlanBlocksWarnsForMissingBlockEvidence(t *testing.T) {
	registry := testDesignRegistry{definition: testBlockDefinition("custom_block", blocks.VerificationStructural)}
	request := Request{
		Version: RequestVersion,
		Name:    "custom",
		Board:   BoardSpec{WidthMM: 10, HeightMM: 10, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "custom", BlockID: "custom_block"}},
	}
	result := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(result.Stage.Issues) || !containsIssueMessage(result.Stage.Issues, "no built-in verification evidence") {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestPlanBlocksBlocksFabricationClaimWithoutStrongEvidence(t *testing.T) {
	registry := testDesignRegistry{definition: testBlockDefinition("custom_fab", blocks.VerificationERCDRCVerified)}
	request := Request{
		Version:    RequestVersion,
		Name:       "custom",
		Board:      BoardSpec{WidthMM: 10, HeightMM: 10, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "custom", BlockID: "custom_fab"}},
		Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate},
	}
	result := PlanBlocks(context.Background(), registry, request)
	if !reports.HasBlockingIssue(result.Stage.Issues) || !containsIssueMessage(result.Stage.Issues, "fabrication readiness claim lacks") {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestPlanBlocksReportsUnknownBlock(t *testing.T) {
	request := validRequest()
	request.Blocks[0].BlockID = "missing"
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want blocking", result.Stage.Issues)
	}
	assertIssueCode(t, result.Stage.Issues, reports.CodeMissingFile)
}

func TestPlanBlocksReportsUnknownPortFromComposition(t *testing.T) {
	request := validRequest()
	request.Connections[0].From = "sensor.NOPE"
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want blocking", result.Stage.Issues)
	}
	if !containsIssueMessage(result.Stage.Issues, "unknown port sensor.NOPE") {
		t.Fatalf("issues = %#v", result.Stage.Issues)
	}
}

func TestPlanBlocksReportsConflictingVoltageDomains(t *testing.T) {
	request := validRequest()
	request.Blocks = []BlockInstanceSpec{
		{ID: "reg_a", BlockID: "voltage_regulator", Params: map[string]any{"output_voltage": "3.3V"}},
		{ID: "reg_b", BlockID: "voltage_regulator", Params: map[string]any{"output_voltage": "1.8V"}},
	}
	request.Connections = []ConnectionSpec{{From: "reg_a.VOUT", To: "reg_b.VOUT"}}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want blocking", result.Stage.Issues)
	}
	if !containsIssueMessage(result.Stage.Issues, "conflicting voltage domains") {
		t.Fatalf("issues = %#v", result.Stage.Issues)
	}
}

func assertIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("missing issue code %q in %#v", code, issues)
}

func containsIssueMessage(issues []reports.Issue, text string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, text) {
			return true
		}
	}
	return false
}

type testDesignRegistry struct {
	definition blocks.BlockDefinition
}

func (registry testDesignRegistry) ListBlocks() []blocks.BlockSummary {
	return []blocks.BlockSummary{testBlockSummary(registry.definition)}
}

func (registry testDesignRegistry) GetBlock(id string) (blocks.BlockDefinition, bool) {
	return registry.definition, id == registry.definition.ID
}

func (registry testDesignRegistry) ValidateDefinition(definition blocks.BlockDefinition) []reports.Issue {
	return nil
}

func (registry testDesignRegistry) ValidateRequest(request blocks.BlockRequest) []reports.Issue {
	if request.BlockID != registry.definition.ID {
		return []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Message: "missing"}}
	}
	return nil
}

func (registry testDesignRegistry) Instantiate(ctx context.Context, request blocks.BlockRequest) (blocks.BlockOutput, []reports.Issue) {
	return blocks.BlockOutput{
		Definition: testBlockSummary(registry.definition),
		Instance: blocks.BlockInstance{
			BlockID:    request.BlockID,
			InstanceID: request.InstanceID,
			Params:     request.Params,
		},
	}, nil
}

func testBlockSummary(definition blocks.BlockDefinition) blocks.BlockSummary {
	return blocks.BlockSummary{
		ID:                definition.ID,
		Name:              definition.Name,
		Description:       definition.Description,
		Version:           definition.Version,
		Category:          definition.Category,
		VerificationLevel: definition.Verification.Level,
	}
}

func testBlockDefinition(id string, level blocks.VerificationLevel) blocks.BlockDefinition {
	return blocks.BlockDefinition{
		ID:          id,
		Name:        id,
		Version:     "0.1.0",
		Category:    "test",
		Description: "test block",
		Verification: blocks.VerificationRecord{
			Level: level,
		},
	}
}

type expandedCoverageTestRegistry struct {
	definitions map[string]blocks.BlockDefinition
}

func newExpandedCoverageTestRegistry() expandedCoverageTestRegistry {
	ids := []string{
		"crystal_oscillator",
		"reset_programming_header",
		"esd_protection",
		"reverse_polarity_protection",
	}
	definitions := map[string]blocks.BlockDefinition{}
	for _, id := range ids {
		definitions[id] = testBlockDefinition(id, blocks.VerificationStructural)
	}
	definitions["esd_protection"] = withRequiredRoutes(definitions["esd_protection"], "esd_signal_entry_to_tvs", "esd_tvs_to_ground")
	definitions["reverse_polarity_protection"] = withRequiredRoutes(definitions["reverse_polarity_protection"], "diode_to_protected_output", "raw_input_to_diode")
	return expandedCoverageTestRegistry{definitions: definitions}
}

func withRequiredRoutes(definition blocks.BlockDefinition, routes ...string) blocks.BlockDefinition {
	if definition.PCBRealization == nil {
		definition.PCBRealization = &blocks.PCBRealization{}
	} else {
		realization := *definition.PCBRealization
		definition.PCBRealization = &realization
	}
	validation := definition.PCBRealization.Validation
	validation.RequiredRoutes = slices.Clone(routes)
	definition.PCBRealization.Validation = validation
	return definition
}

func (registry expandedCoverageTestRegistry) ListBlocks() []blocks.BlockSummary {
	summaries := make([]blocks.BlockSummary, 0, len(registry.definitions))
	for _, id := range registry.definitionIDs() {
		definition := registry.definitions[id]
		summaries = append(summaries, testBlockSummary(definition))
	}
	return summaries
}

func (registry expandedCoverageTestRegistry) GetBlock(id string) (blocks.BlockDefinition, bool) {
	definition, ok := registry.definitions[id]
	return definition, ok
}

func (registry expandedCoverageTestRegistry) ValidateDefinition(definition blocks.BlockDefinition) []reports.Issue {
	return nil
}

func (registry expandedCoverageTestRegistry) ValidateRequest(request blocks.BlockRequest) []reports.Issue {
	if _, ok := registry.definitions[request.BlockID]; !ok {
		return []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Message: "missing"}}
	}
	return nil
}

func (registry expandedCoverageTestRegistry) Instantiate(ctx context.Context, request blocks.BlockRequest) (blocks.BlockOutput, []reports.Issue) {
	definition, ok := registry.definitions[request.BlockID]
	if !ok {
		issues := []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Message: "missing"}}
		return blocks.BlockOutput{Issues: issues}, issues
	}
	output := blocks.BlockOutput{
		Definition: testBlockSummary(definition),
		Instance: blocks.BlockInstance{
			BlockID:    request.BlockID,
			InstanceID: request.InstanceID,
			Params:     request.Params,
		},
	}
	return output, nil
}

func (registry expandedCoverageTestRegistry) Inventory() blocks.BlockLibraryInventory {
	families := make([]blocks.BlockFamilyInventory, 0, len(registry.definitions))
	for _, id := range registry.definitionIDs() {
		definition := registry.definitions[id]
		families = append(families, blocks.BlockFamilyInventory{
			ID:                definition.ID,
			Name:              definition.Name,
			Category:          definition.Category,
			Implemented:       true,
			Readiness:         blocks.BlockReadinessPartial,
			VerificationLevel: definition.Verification.Level,
		})
	}
	return blocks.BlockLibraryInventory{Families: families}
}

func (registry expandedCoverageTestRegistry) definitionIDs() []string {
	ids := make([]string, 0, len(registry.definitions))
	for id := range registry.definitions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
