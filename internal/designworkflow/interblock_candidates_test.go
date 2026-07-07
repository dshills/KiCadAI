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
