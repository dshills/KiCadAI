package designworkflow

import (
	"encoding/json"
	"testing"

	"kicadai/internal/placement"
)

func TestBuildInterBlockContactTargetsResolvesPlacedPad(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header", BlockID: "connector_breakout"},
			{Ref: "D1", Pin: "1", InstanceID: "status", BlockID: "led_indicator"},
		},
	}}

	evidence := BuildInterBlockContactTargets(candidates, &placed)
	if len(evidence.Issues) != 0 {
		t.Fatalf("issues = %#v", evidence.Issues)
	}
	if len(evidence.Targets) != 2 {
		t.Fatalf("targets = %#v, want 2", evidence.Targets)
	}
	target := evidence.Targets[0]
	if target.Kind != InterBlockContactTargetPad || target.Confidence != InterBlockContactConfidenceHigh || target.NetCode == 0 {
		t.Fatalf("target = %#v", target)
	}
	if target.Ref != "J1" || target.Pad != "1" || target.InstanceID != "header" || target.BlockID != "connector_breakout" {
		t.Fatalf("target identity = %#v", target)
	}
}

func TestBuildInterBlockContactTargetsReportsMissingPad(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName:   "SIG",
		Status:    InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{{Ref: "J1", Pin: "9", InstanceID: "header"}},
	}}

	evidence := BuildInterBlockContactTargets(candidates, &placed)
	if len(evidence.Targets) != 0 || len(evidence.Issues) == 0 {
		t.Fatalf("evidence = %#v, want missing-pad issue", evidence)
	}
}

func TestBuildInterBlockContactTargetsReportsNetMismatch(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "OTHER")
	candidates := []InterBlockRouteCandidate{{
		NetName:   "SIG",
		Status:    InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{{Ref: "D1", Pin: "1", InstanceID: "status"}},
	}}

	evidence := BuildInterBlockContactTargets(candidates, &placed)
	if len(evidence.Targets) != 0 || len(evidence.Issues) == 0 {
		t.Fatalf("evidence = %#v, want net-mismatch issue", evidence)
	}
}

func TestInterBlockContactProofJSONStable(t *testing.T) {
	proof := contactProofForTarget(InterBlockContactTarget{
		NetName:     "SIG",
		NetCode:     1,
		Kind:        InterBlockContactTargetPad,
		Ref:         "J1",
		Pad:         "1",
		Layer:       "F.Cu",
		ToleranceMM: interBlockContactToleranceMM,
		Confidence:  InterBlockContactConfidenceHigh,
	}, InterBlockContactMiss, "snap endpoint to pad")

	data, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"route_class":"inter_block","net_name":"SIG","net_code":1,"target":{"net_name":"SIG","net_code":1,"kind":"pad","ref":"J1","pad":"1","point":{"x_mm":0,"y_mm":0},"layer":"F.Cu","tolerance_mm":0.0001,"confidence":"high"},"tolerance_mm":0.0001,"status":"miss","blocking":true,"suggestion":"snap endpoint to pad"}`
	if string(data) != want {
		t.Fatalf("proof JSON = %s, want %s", data, want)
	}
}

func interBlockContactPlaced(firstNet string, secondNet string) PlacementStageResult {
	return PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{
				{
					Ref:         "J1",
					FootprintID: "Connector:Test",
					Pads:        []placement.PadSummary{{Name: "1", Net: firstNet, XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
				},
				{
					Ref:         "D1",
					FootprintID: "LED:Test",
					Pads:        []placement.PadSummary{{Name: "1", Net: secondNet, XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
				},
			},
			Nets: []placement.Net{{
				Name:      "SIG",
				Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "D1", Pin: "1"}},
			}},
		},
		Result: placement.Result{
			Status: placement.StatusPlaced,
			Placements: []placement.PlacementResult{
				{Ref: "J1", FootprintID: "Connector:Test", Position: placement.Placement{XMM: 5, YMM: 10, Layer: "F.Cu"}},
				{Ref: "D1", FootprintID: "LED:Test", Position: placement.Placement{XMM: 15, YMM: 10, Layer: "F.Cu"}},
			},
		},
		Stage: NewStageResult(StagePlacement, nil),
	}
}
