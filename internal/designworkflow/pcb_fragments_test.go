package designworkflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestRealizePCBFragmentsCreatesLEDFragment(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	plan := PlanBlocks(ctx, registry, request)
	result := RealizePCBFragments(ctx, registry, plan)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("fragment issues = %#v", result.Stage.Issues)
	}
	if len(result.Fragments) != 1 || len(result.Fragments[0].Realization.Components) != 2 || len(result.Fragments[0].Realization.LocalRoutes) != 1 {
		t.Fatalf("fragments = %#v", result.Fragments)
	}
	if result.Fragments[0].OriginXMM != defaultFragmentMarginMM {
		t.Fatalf("origin = %#v", result.Fragments[0])
	}
}

func TestFragmentColumnCountUsesAvailableWideBoardFlow(t *testing.T) {
	request := Request{Board: BoardSpec{WidthMM: 100}, Blocks: []BlockInstanceSpec{{}, {}, {}, {}}}
	if columns := fragmentColumnCount(request); columns != 4 {
		t.Fatalf("columns = %d, want four left-to-right fragments on a 100mm board", columns)
	}

	narrow := Request{Board: BoardSpec{WidthMM: 40}, Blocks: []BlockInstanceSpec{{}, {}, {}, {}}}
	if columns := fragmentColumnCount(narrow); columns != 1 {
		t.Fatalf("columns = %d, want one column when the board cannot fit another fragment", columns)
	}
}

func TestRealizePCBFragmentsAppliesConnectionAliasesToLocalRoutes(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "i2c_sensor_breakout_candidate",
		Board:   BoardSpec{WidthMM: 55, HeightMM: 35, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "sensor", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48", "include_pullups": true}},
			{ID: "io", BlockID: "connector_breakout", Params: map[string]any{"pin_count": 4, "pin_names": []string{"VCC", "GND", "SDA", "SCL"}}},
		},
		Connections: []ConnectionSpec{
			{From: "sensor.VCC", To: "io.VCC", NetAlias: "VCC"},
			{From: "sensor.GND", To: "io.GND", NetAlias: "GND"},
			{From: "sensor.SDA", To: "io.SDA", NetAlias: "SDA"},
			{From: "sensor.SCL", To: "io.SCL", NetAlias: "SCL"},
		},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	result := RealizePCBFragments(context.Background(), registry, plan)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("fragment issues = %#v", result.Stage.Issues)
	}
	if len(result.Fragments) == 0 {
		t.Fatalf("fragments = %#v", result.Fragments)
	}
	nets := map[string]bool{}
	for _, route := range result.Fragments[0].Realization.LocalRoutes {
		nets[route.NetName] = true
	}
	for _, want := range []string{"VCC", "GND", "SDA", "SCL"} {
		if !nets[want] {
			t.Fatalf("local route nets = %#v, missing alias %s", nets, want)
		}
	}
	for _, stale := range []string{"sensor_vcc", "sensor_gnd", "sensor_sda", "sensor_scl"} {
		if nets[stale] {
			t.Fatalf("local route nets = %#v, still include stale instance net %s", nets, stale)
		}
	}
	operationNets := map[string]bool{}
	for _, operation := range result.Fragments[0].Realization.Operations {
		if operation.Op == transactions.OpRoute {
			operationNets[operation.Net] = true
			var payload transactions.RouteOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				t.Fatalf("route operation raw = %s: %v", string(operation.Raw), err)
			}
			operationNets[payload.NetName] = true
		}
	}
	for _, want := range []string{"VCC", "GND", "SDA", "SCL"} {
		if !operationNets[want] {
			t.Fatalf("route operation nets = %#v, missing alias %s", operationNets, want)
		}
	}
	for _, stale := range []string{"sensor_vcc", "sensor_gnd", "sensor_sda", "sensor_scl"} {
		if operationNets[stale] {
			t.Fatalf("route operation nets = %#v, still include stale instance net %s", operationNets, stale)
		}
	}
}

func TestFragmentNetAliasesIncludesDestinationEndpoints(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "inbound_aliases",
		Connections: []ConnectionSpec{
			{From: "sensor.VCC", To: "io.VCC", NetAlias: "VCC"},
			{From: "sensor.GND", To: "io.GND", NetAlias: "GND"},
		},
	}

	aliases, issues := fragmentNetAliases("io", request)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	for _, want := range []string{"io_VCC", "io_GND"} {
		if aliases[want] == "" {
			t.Fatalf("aliases = %#v, missing destination alias for %s", aliases, want)
		}
	}
	if aliases["io_VCC"] != "VCC" {
		t.Fatalf("aliases[io_VCC] = %q, want VCC", aliases["io_VCC"])
	}
	if aliases["io_GND"] != "GND" {
		t.Fatalf("aliases[io_GND] = %q, want GND", aliases["io_GND"])
	}
}

func TestFragmentNetAliasesIncludesNormalizedInstanceKeys(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "mixed_case_aliases",
		Connections: []ConnectionSpec{
			{From: "Sensor.VCC", To: "IO.VCC", NetAlias: "VCC"},
		},
	}

	aliases, issues := fragmentNetAliases("sensor", request)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if aliases["sensor_vcc"] != "VCC" {
		t.Fatalf("aliases[sensor_vcc] = %q, want VCC", aliases["sensor_vcc"])
	}
	if aliases["Sensor_vcc"] != "VCC" {
		t.Fatalf("aliases[Sensor_vcc] = %q, want VCC", aliases["Sensor_vcc"])
	}
}

func TestFragmentNetAliasesReportsConflictingAliases(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "conflicting_aliases",
		Connections: []ConnectionSpec{
			{From: "sensor.VCC", To: "io.VCC", NetAlias: "VCC"},
			{From: "sensor.VCC", To: "debug.VCC", NetAlias: "ALT_VCC"},
		},
	}

	aliases, issues := fragmentNetAliases("sensor", request)
	if aliases["sensor_vcc"] != "VCC" {
		t.Fatalf("aliases[sensor_vcc] = %q, want first alias VCC", aliases["sensor_vcc"])
	}
	if len(issues) == 0 {
		t.Fatalf("expected alias conflict issue")
	}
	foundLowerRoleConflict := false
	for _, issue := range issues {
		if issue.Path == "net_aliases.sensor_vcc" {
			foundLowerRoleConflict = true
		}
	}
	if !foundLowerRoleConflict {
		t.Fatalf("issues = %#v, missing sensor_vcc conflict", issues)
	}
}

func TestFragmentNetAliasesReportsInvalidEndpoints(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "invalid_endpoint",
		Connections: []ConnectionSpec{
			{From: "sensor.VCC", To: "missing_separator", NetAlias: "VCC"},
		},
	}

	_, issues := fragmentNetAliases("sensor", request)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one invalid endpoint issue", issues)
	}
	if issues[0].Path != "connections[0].to" {
		t.Fatalf("issue path = %q, want connections[0].to", issues[0].Path)
	}
}

func TestFragmentCountsIncludesTimingResults(t *testing.T) {
	componentCount, routeCount, timingCount := fragmentCounts([]BlockFragment{{
		Realization: blocks.BlockPCBRealizationResult{
			Components:  []blocks.RealizedPCBComponent{{Ref: "Y1"}},
			LocalRoutes: []blocks.RealizedPCBLocalRoute{{ID: "xtal"}},
			Timing:      []blocks.TimingFixtureEvidence{{ID: "clock"}},
		},
	}})
	if componentCount != 1 || routeCount != 1 || timingCount != 1 {
		t.Fatalf("counts = %d %d %d", componentCount, routeCount, timingCount)
	}
}

func TestRealizePCBFragmentsIncludesCannedOscillatorTiming(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "clock_board",
		Board:   BoardSpec{WidthMM: 50, HeightMM: 30, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "clk1", BlockID: "canned_oscillator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	result := RealizePCBFragments(context.Background(), registry, plan)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("fragment issues = %#v", result.Stage.Issues)
	}
	if len(result.Fragments) != 1 || len(result.Fragments[0].Realization.Timing) != 1 {
		t.Fatalf("fragments = %#v", result.Fragments)
	}
	if result.Stage.Summary["timing_results"] != 1 {
		t.Fatalf("summary = %#v", result.Stage.Summary)
	}
	rules := proximityRulesFromFragment(result.Fragments[0])
	foundClockRule := false
	for _, rule := range rules {
		if rule.Role == placement.IntentClock {
			foundClockRule = true
			break
		}
	}
	if !foundClockRule {
		t.Fatalf("proximity rules = %#v, want clock-intent oscillator rule", rules)
	}
}

func TestTimingEvidenceIssuesReportsWarningsAndRelativePaths(t *testing.T) {
	issues := timingEvidenceIssues(blocks.BlockPCBRealizationResult{
		Timing: []blocks.TimingFixtureEvidence{{
			Findings: []blocks.TimingFixtureFinding{{
				Severity: reports.SeverityWarning,
				Message:  "near limit",
			}},
		}, {
			ID: "clock",
			Findings: []blocks.TimingFixtureFinding{{
				ID:       blocks.TimingFindingGroundReturnPresent,
				Severity: reports.SeverityError,
				Message:  "missing ground",
				Refs:     []string{"C1"},
				Nets:     []string{"GND"},
			}},
		}},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	if issues[0].Path != "timing.result.0.finding.0" || issues[1].Path != "timing.clock.timing.ground_return.present" {
		t.Fatalf("issue paths = %#v", issues)
	}
	if issues[0].Severity != reports.SeverityWarning || issues[1].Suggestion == "" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestTimingFindingSuggestionsIncludeOscillatorEvidence(t *testing.T) {
	issues := timingEvidenceIssues(blocks.BlockPCBRealizationResult{
		Timing: []blocks.TimingFixtureEvidence{{
			ID: "osc",
			Findings: []blocks.TimingFixtureFinding{{
				ID:       blocks.TimingFindingDecouplingProximity,
				Severity: reports.SeverityError,
				Message:  "decoupling far away",
			}, {
				ID:       blocks.TimingFindingEnableControlPresent,
				Severity: reports.SeverityError,
				Message:  "enable missing",
			}},
		}},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	if issues[0].Suggestion != "move timing decoupling closer to the clock source" {
		t.Fatalf("decoupling suggestion = %q", issues[0].Suggestion)
	}
	if issues[1].Suggestion != "place the required timing enable/control component in the PCB realization" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestTimingFindingSuggestionsIncludeResetProgrammingEvidence(t *testing.T) {
	issues := timingEvidenceIssues(blocks.BlockPCBRealizationResult{
		Timing: []blocks.TimingFixtureEvidence{{
			ID: "reset",
			Findings: []blocks.TimingFixtureFinding{{
				ID:       blocks.TimingFindingResetProgrammingRouteLength,
				Severity: reports.SeverityError,
				Message:  "reset route too long",
			}, {
				ID:       blocks.TimingFindingProgrammingGroundReference,
				Severity: reports.SeverityError,
				Message:  "ground missing",
			}},
		}},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	if issues[0].Suggestion != "shorten reset/programming routes or relax the reset timing threshold" {
		t.Fatalf("reset route suggestion = %q", issues[0].Suggestion)
	}
	if issues[1].Suggestion != "add local programming-header ground reference evidence" {
		t.Fatalf("ground reference suggestion = %q", issues[1].Suggestion)
	}
}

func TestRealizePCBFragmentsWarnsWhenBoardTooSmall(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "tiny",
		Board:   BoardSpec{WidthMM: 4, HeightMM: 4, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	result := RealizePCBFragments(context.Background(), registry, plan)
	if result.Stage.Status != StageStatusWarning {
		t.Fatalf("stage = %#v", result.Stage)
	}
	assertIssueCode(t, result.Stage.Issues, reports.CodePlacementOutsideBoard)
}

func TestRealizePCBFragmentsSkipsAfterPlanFailure(t *testing.T) {
	result := RealizePCBFragments(context.Background(), blocks.NewBuiltinRegistry(), BlockPlanResult{
		Stage: NewStageResult(StageBlockPlanning, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "bad"}}),
	})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestRealizePCBFragmentsRequiresContext(t *testing.T) {
	result := RealizePCBFragments(nil, blocks.NewBuiltinRegistry(), BlockPlanResult{})
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want context issue", result.Stage.Issues)
	}
}
