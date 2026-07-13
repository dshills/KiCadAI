package designworkflow

import (
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/transactions"
)

func TestBuildInterBlockRouteCandidatesPrunesPerLocalRouteIsland(t *testing.T) {
	fragments := PCBFragmentResult{Fragments: []BlockFragment{
		{
			InstanceID: "multi",
			BlockID:    "two_islands",
			Realization: blocks.BlockPCBRealizationResult{
				Components: []blocks.RealizedPCBComponent{
					{Ref: "U1"},
					{Ref: "R1"},
					{Ref: "U2"},
					{Ref: "R2"},
				},
				LocalRoutes: []blocks.RealizedPCBLocalRoute{
					{
						NetName: "SIG",
						From:    transactions.Endpoint{Ref: "U1", Pin: "1"},
						To:      transactions.Endpoint{Ref: "R1", Pin: "1"},
					},
					{
						NetName: "SIG",
						From:    transactions.Endpoint{Ref: "U2", Pin: "1"},
						To:      transactions.Endpoint{Ref: "R2", Pin: "1"},
					},
				},
			},
		},
		{
			InstanceID: "io",
			BlockID:    "connector",
			Realization: blocks.BlockPCBRealizationResult{
				Components: []blocks.RealizedPCBComponent{{Ref: "J1"}},
			},
		},
	}}
	placed := PlacementStageResult{Request: placement.Request{Nets: []placement.Net{{
		Name: "SIG",
		Endpoints: []placement.Endpoint{
			{Ref: "U1", Pin: "1"},
			{Ref: "R1", Pin: "1"},
			{Ref: "U2", Pin: "1"},
			{Ref: "R2", Pin: "1"},
			{Ref: "J1", Pin: "1"},
		},
	}}}}

	candidates, issues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one SIG candidate", candidates)
	}
	got := map[string]bool{}
	for _, endpoint := range candidates[0].Endpoints {
		got[normalizedRouteGroupEndpointKey(endpoint.Ref, endpoint.Pin)] = true
	}
	if len(got) != 3 || !got["U1.1"] || !got["U2.1"] || !got["J1.1"] {
		t.Fatalf("endpoints = %#v, want one endpoint per local island plus connector", candidates[0].Endpoints)
	}
}

func TestBuildInterBlockRouteCandidatesIncludesParticipatingPhysicalPads(t *testing.T) {
	fragments := PCBFragmentResult{Fragments: []BlockFragment{
		{
			InstanceID: "usb",
			BlockID:    "usb_input",
			Realization: blocks.BlockPCBRealizationResult{
				Components: []blocks.RealizedPCBComponent{{Ref: "J1"}},
			},
		},
		{
			InstanceID: "load",
			BlockID:    "load",
			Realization: blocks.BlockPCBRealizationResult{
				Components: []blocks.RealizedPCBComponent{{Ref: "C1"}},
			},
		},
		{
			InstanceID: "unrelated",
			BlockID:    "unrelated",
			Realization: blocks.BlockPCBRealizationResult{
				Components: []blocks.RealizedPCBComponent{{Ref: "J9"}},
			},
		},
	}}
	placed := PlacementStageResult{Request: placement.Request{
		Components: []placement.Component{
			{
				Ref: "J1",
				Pads: []placement.PadSummary{
					{Name: "SH", Net: "GND"},
					{Name: "SH", Net: "GND"},
					{Name: "SH", Net: "GND"},
					{Name: "SH", Net: "GND"},
					{Name: "A5", Net: "USB_CC1"},
					{Name: "NC"},
				},
			},
			{Ref: "C1", Pads: []placement.PadSummary{{Name: "1", Net: "3V3"}, {Name: "2", Net: "GND"}}},
			{Ref: "J9", Pads: []placement.PadSummary{{Name: "1", Net: "GND"}}},
		},
		Nets: []placement.Net{{
			Name: "GND",
			Endpoints: []placement.Endpoint{
				{Ref: "J1", Pin: "SH"},
				{Ref: "C1", Pin: "2"},
			},
		}},
	}}

	candidates, issues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one GND candidate", candidates)
	}
	got := map[string]bool{}
	for _, endpoint := range candidates[0].Endpoints {
		got[normalizedRouteGroupEndpointKey(endpoint.Ref, endpoint.Pin)] = true
	}
	want := []string{"J1.SH", "J1.SH#2", "J1.SH#3", "J1.SH#4", "C1.2"}
	if len(got) != len(want) {
		t.Fatalf("endpoints = %#v, want %v", candidates[0].Endpoints, want)
	}
	for _, endpoint := range want {
		if !got[endpoint] {
			t.Fatalf("endpoints = %#v, missing %s", candidates[0].Endpoints, endpoint)
		}
	}
	if got["J1.A5"] || got["J1.NC"] || got["J9.1"] {
		t.Fatalf("endpoints include unrelated or mismatched pads: %#v", candidates[0].Endpoints)
	}
}
