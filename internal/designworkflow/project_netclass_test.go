package designworkflow

import (
	"math"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestProjectNetClassClearanceFitsIntrinsicFinePitchPadGap(t *testing.T) {
	placed := PlacementStageResult{Request: placement.Request{Components: []placement.Component{{
		Ref: "U1",
		Pads: []placement.PadSummary{
			{Name: "1", Net: "A", XMM: -1.4, YMM: -0.25, WidthMM: 1.25, HeightMM: 0.35, Layers: []string{"F.Cu"}},
			{Name: "2", Net: "B", XMM: -1.4, YMM: 0.25, WidthMM: 1.25, HeightMM: 0.35, Layers: []string{"F.Cu"}},
		},
	}}}}
	routed := RoutingStageResult{Request: routing.Request{Rules: routing.Rules{ClearanceMM: 0.2}}}

	if got := projectNetClassClearanceMM(&routed, &placed); math.Abs(got-0.15) > 1e-9 {
		t.Fatalf("project clearance = %.9f, want intrinsic 0.15 mm", got)
	}
}

func TestFitRoutingClearanceUsesIntrinsicGapForImplicitDefaults(t *testing.T) {
	components := []placement.Component{{Pads: []placement.PadSummary{
		{Name: "1", Net: "A", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 0.35, Layers: []string{"F.Cu"}},
		{Name: "2", Net: "B", XMM: 0, YMM: 0.5, WidthMM: 1, HeightMM: 0.35, Layers: []string{"F.Cu"}},
	}}}
	request := routing.Request{Rules: routing.Rules{
		ClearanceMM: 0.2, ViaClearanceMM: 0.2,
		NetClasses:   map[string]routing.NetClass{"signal": {ClearanceMM: 0.2, ViaClearanceMM: 0.2}},
		NetOverrides: map[string]routing.NetRule{"A": {ClearanceMM: 0.2, ViaClearanceMM: 0.2}},
	}}

	if issues := fitRoutingClearanceToIntrinsicPads(&request, components, false); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if request.Rules.ClearanceMM != 0.15 || request.Rules.ViaClearanceMM != 0.15 || request.Rules.NetClasses["signal"].ClearanceMM != 0.15 || request.Rules.NetOverrides["A"].ClearanceMM != 0.15 {
		t.Fatalf("fitted rules = %#v", request.Rules)
	}
}

func TestFitRoutingClearanceFailsClosedForExplicitRequirement(t *testing.T) {
	components := []placement.Component{{Pads: []placement.PadSummary{
		{Name: "1", Net: "A", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 0.35},
		{Name: "2", Net: "B", XMM: 0, YMM: 0.5, WidthMM: 1, HeightMM: 0.35},
	}}}
	request := routing.Request{Rules: routing.Rules{ClearanceMM: 0.2}}

	issues := fitRoutingClearanceToIntrinsicPads(&request, components, true)
	if !reports.HasBlockingIssue(issues) || request.Rules.ClearanceMM != 0.2 {
		t.Fatalf("issues = %#v rules = %#v", issues, request.Rules)
	}
}

func TestProjectNetClassClearanceDoesNotReduceForSameNetPads(t *testing.T) {
	placed := PlacementStageResult{Request: placement.Request{Components: []placement.Component{{
		Ref: "U1",
		Pads: []placement.PadSummary{
			{Name: "1", Net: "GND", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1},
			{Name: "2", Net: "GND", XMM: 1.05, YMM: 0, WidthMM: 1, HeightMM: 1},
		},
	}}}}
	routed := RoutingStageResult{Request: routing.Request{Rules: routing.Rules{ClearanceMM: 0.2}}}

	if got := projectNetClassClearanceMM(&routed, &placed); got != 0.2 {
		t.Fatalf("project clearance = %.9f, want routed 0.2 mm", got)
	}
}

func TestProjectMinimumThroughHoleDiameterUsesVerifiedIntrinsicDrill(t *testing.T) {
	placed := PlacementStageResult{Request: placement.Request{Components: []placement.Component{{Pads: []placement.PadSummary{
		{Name: "1", DrillMM: 0.8},
		{Name: "2", DrillMM: 0.2},
	}}}}}
	if got := projectMinimumThroughHoleDiameterMM(&placed); math.Abs(got-0.2) > 1e-9 {
		t.Fatalf("minimum through-hole diameter = %v, want intrinsic 0.2 mm", got)
	}
	placed.Request.Components[0].Pads[1].DrillMM = 0.3
	if got := projectMinimumThroughHoleDiameterMM(&placed); got != 0 {
		t.Fatalf("minimum through-hole diameter = %v, want omitted default", got)
	}
}
