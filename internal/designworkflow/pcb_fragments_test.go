package designworkflow

import (
	"context"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

func TestRealizePCBFragmentsCreatesLEDFragment(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	result := RealizePCBFragments(context.Background(), registry, plan)
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
