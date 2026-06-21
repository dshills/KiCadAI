package designworkflow

import (
	"context"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

const testTimeout = 10 * time.Second

func TestPlaceFragmentsPlacesRealizedLED(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	if result.Result.Metrics.PlacedCount != 2 || len(result.Result.Operations) != 2 {
		t.Fatalf("placement result = %#v", result.Result)
	}
	if !result.Request.Components[0].Fixed {
		t.Fatalf("expected fixed realized placement: %#v", result.Request.Components[0])
	}
	if len(result.Request.Groups) == 0 {
		t.Fatalf("expected block-derived placement groups")
	}
	if len(result.Request.ProximityRules) == 0 {
		t.Fatalf("expected block-derived proximity rules")
	}
}

func TestPlaceFragmentsCurrentlyLacksGeneratedPadSummaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(ctx, registry, request)
	fragments := RealizePCBFragments(ctx, registry, plan)

	result := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if len(result.Request.Components) == 0 {
		t.Fatalf("expected generated components")
	}
	for _, component := range result.Request.Components {
		if len(component.Pads) != 0 {
			t.Fatalf("unexpected generated pad summaries before hydration: %#v", component)
		}
	}
}

func TestPlaceFragmentsHydratesGeneratedPadsFromResolver(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(ctx, registry, request)
	fragments := RealizePCBFragments(ctx, registry, plan)
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		"Resistor_SMD:R_0805_2012Metric": placementTestFootprint("Resistor_SMD:R_0805_2012Metric"),
		"LED_SMD:LED_0805_2012Metric":    placementTestFootprint("LED_SMD:LED_0805_2012Metric"),
	}}

	result := PlaceFragments(ctx, request, fragments, PlacementOptions{LibraryIndex: &index})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	var sawNet bool
	for _, component := range result.Request.Components {
		if len(component.Pads) != 2 {
			t.Fatalf("%s pads = %#v, want two hydrated pads", component.Ref, component.Pads)
		}
		for _, pad := range component.Pads {
			if pad.Net != "" {
				sawNet = true
			}
		}
	}
	if !sawNet {
		t.Fatalf("expected at least one generated net assignment: %#v", result.Request.Components)
	}
	summary, ok := result.Stage.Summary["pad_hydration"].(PadHydrationSummary)
	if !ok || summary.HydratedComponents != 2 || summary.PadCount != 4 {
		t.Fatalf("pad hydration summary = %#v", result.Stage.Summary["pad_hydration"])
	}
}

func TestPlaceFragmentsDerivesBlockPlacementIntent(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "analog_board",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 60, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "amp", BlockID: "opamp_gain_stage"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if len(result.Request.Groups) == 0 {
		t.Fatalf("expected placement groups in request")
	}
	group := result.Request.Groups[0]
	if group.ID == "" || len(group.Components) == 0 || !group.KeepTogether || group.MaxSpreadMM <= 0 {
		t.Fatalf("unexpected group: %#v", group)
	}
	if len(result.Request.ProximityRules) == 0 {
		t.Fatalf("expected proximity rules in request")
	}
	if result.Request.ProximityRules[0].Source == "" {
		t.Fatalf("expected proximity source metadata: %#v", result.Request.ProximityRules[0])
	}
}

func placementTestFootprint(id string) libraryresolver.FootprintRecord {
	return libraryresolver.FootprintRecord{
		FootprintID: id,
		BoundingBox: libraryresolver.BoundingBox{
			Min: kicadfiles.Point{X: -1_000_000, Y: -500_000},
			Max: kicadfiles.Point{X: 1_000_000, Y: 500_000},
		},
		Pads: []libraryresolver.FootprintPad{
			{Name: "1", Position: kicadfiles.Point{X: -600_000}, Size: kicadfiles.Point{X: 500_000, Y: 600_000}},
			{Name: "2", Position: kicadfiles.Point{X: 600_000}, Size: kicadfiles.Point{X: 500_000, Y: 600_000}},
		},
	}
}

func TestPlaceFragmentsDerivesConnectorEdgeIntent(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "usb_power",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 60, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "usb", BlockID: "usb_c_power"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	for _, component := range result.Request.Components {
		if component.Role == "usb_c_receptacle" {
			if component.Edge != placement.EdgeAny {
				t.Fatalf("usb connector edge = %q, want any edge", component.Edge)
			}
			return
		}
	}
	t.Fatalf("missing usb_c_receptacle in placement request: %#v", result.Request.Components)
}

func TestPlaceFragmentsSkipsAfterRealizationFailure(t *testing.T) {
	result := PlaceFragments(context.Background(), validRequest(), PCBFragmentResult{
		Stage: NewStageResult(StagePCBRealization, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "bad"}}),
	}, PlacementOptions{})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestPlaceFragmentsReportsTinyBoardCollision(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "tiny",
		Board:   BoardSpec{WidthMM: 4, HeightMM: 4, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{DefaultBounds: placement.Bounds{WidthMM: 2, HeightMM: 1, Source: placement.BoundsEstimated}})
	if result.Stage.Status == StageStatusOK {
		t.Fatalf("expected placement warning/block for tiny board: %#v", result.Stage)
	}
}

func TestNetRoleFromNameUsesTokens(t *testing.T) {
	if got := netRoleFromName("saving_mode"); got != placement.NetSignal {
		t.Fatalf("saving_mode role = %q", got)
	}
	if got := netRoleFromName("main_vbus"); got != placement.NetPower {
		t.Fatalf("main_vbus role = %q", got)
	}
}

func TestPlaceFragmentsMergesRulesWithoutDroppingCustomValues(t *testing.T) {
	request := Request{
		Version:     RequestVersion,
		Name:        "status_board",
		Board:       BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:      []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Constraints: ConstraintSpec{AllowBackLayer: false, PreferTopLayer: false},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{Rules: placement.Rules{ComponentSpacingMM: 3, AllowBackLayer: true, PreferTopLayer: true}})
	if result.Request.Rules.ComponentSpacingMM != 3 {
		t.Fatalf("component spacing = %v", result.Request.Rules.ComponentSpacingMM)
	}
	if result.Request.Rules.AllowBackLayer || result.Request.Rules.PreferTopLayer {
		t.Fatalf("request constraints did not override layer preferences: %#v", result.Request.Rules)
	}
}
