package designworkflow

import (
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/transactions"
)

func TestClassifyLocalRouteMobilityPreservesFixedRoutes(t *testing.T) {
	fragments := localRouteMobilityFixture()
	request := placement.Request{Components: []placement.Component{
		{Ref: "R1", Mobility: placement.MobilityPolicy{Class: placement.MobilityFixed, RouteHandling: placement.RouteHandlingPreserveFixed}},
		{Ref: "D1", Mobility: placement.MobilityPolicy{Class: placement.MobilityFixed, RouteHandling: placement.RouteHandlingPreserveFixed}},
	}}

	got := classifyLocalRouteMobility(fragments, request)
	if got.Total != 1 || got.Preserved != 1 || got.Transformable != 0 || got.Rebuildable != 0 || got.Blocked != 0 {
		t.Fatalf("summary = %#v, want preserved local route", got)
	}
}

func TestClassifyLocalRouteMobilityTransformsSameGroupRoutes(t *testing.T) {
	fragments := localRouteMobilityFixture()
	request := placement.Request{Components: []placement.Component{
		{Ref: "R1", Mobility: placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "status", RouteHandling: placement.RouteHandlingTransformWithGroup}},
		{Ref: "D1", Mobility: placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "status", RouteHandling: placement.RouteHandlingTransformWithGroup}},
	}}

	got := classifyLocalRouteMobility(fragments, request)
	if got.Total != 1 || got.Transformable != 1 || got.Rebuildable != 0 || got.Preserved != 0 || got.Blocked != 0 {
		t.Fatalf("summary = %#v, want transformable local route", got)
	}
}

func TestClassifyLocalRouteMobilityRebuildsIndependentRoutes(t *testing.T) {
	fragments := localRouteMobilityFixture()
	request := placement.Request{Components: []placement.Component{
		{Ref: "R1", Mobility: placement.MobilityPolicy{Class: placement.MobilityLocalRebuild, RouteHandling: placement.RouteHandlingInvalidateRebuild}},
		{Ref: "D1", Mobility: placement.MobilityPolicy{Class: placement.MobilitySoftPreferred, RouteHandling: placement.RouteHandlingInvalidateRebuild}},
	}}

	got := classifyLocalRouteMobility(fragments, request)
	if got.Total != 1 || got.Rebuildable != 1 || got.Transformable != 0 || got.Preserved != 0 || got.Blocked != 0 {
		t.Fatalf("summary = %#v, want rebuildable local route", got)
	}
}

func TestClassifyLocalRouteMobilityRebuildsMixedFixedMovableRoutes(t *testing.T) {
	fragments := localRouteMobilityFixture()
	request := placement.Request{Components: []placement.Component{
		{Ref: "R1", Mobility: placement.MobilityPolicy{Class: placement.MobilityFixed, RouteHandling: placement.RouteHandlingPreserveFixed}},
		{Ref: "D1", Mobility: placement.MobilityPolicy{Class: placement.MobilityLocalRebuild, RouteHandling: placement.RouteHandlingInvalidateRebuild}},
	}}

	got := classifyLocalRouteMobility(fragments, request)
	if got.Total != 1 || got.Rebuildable != 1 || got.Preserved != 0 {
		t.Fatalf("summary = %#v, want rebuildable mixed fixed/movable route", got)
	}
}

func TestClassifyLocalRouteMobilityRebuildsCrossGroupRoutes(t *testing.T) {
	fragments := localRouteMobilityFixture()
	request := placement.Request{Components: []placement.Component{
		{Ref: "R1", Mobility: placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "left", RouteHandling: placement.RouteHandlingTransformWithGroup}},
		{Ref: "D1", Mobility: placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "right", RouteHandling: placement.RouteHandlingTransformWithGroup}},
	}}

	got := classifyLocalRouteMobility(fragments, request)
	if got.Total != 1 || got.Rebuildable != 1 || got.Transformable != 0 {
		t.Fatalf("summary = %#v, want rebuildable cross-group route", got)
	}
}

func TestClassifyLocalRouteMobilityBlocksUnownedRoutes(t *testing.T) {
	fragments := localRouteMobilityFixture()
	request := placement.Request{Components: []placement.Component{
		{Ref: "R1", Mobility: placement.MobilityPolicy{Class: placement.MobilityUnowned, RouteHandling: placement.RouteHandlingUnsupported}},
		{Ref: "D1", Mobility: placement.MobilityPolicy{Class: placement.MobilitySoftPreferred, RouteHandling: placement.RouteHandlingInvalidateRebuild}},
	}}

	got := classifyLocalRouteMobility(fragments, request)
	if got.Total != 1 || got.Blocked != 1 {
		t.Fatalf("summary = %#v, want blocked local route", got)
	}
}

func TestClassifyLocalRoutePolicyMatrix(t *testing.T) {
	tests := []struct {
		name string
		from placement.MobilityPolicy
		to   placement.MobilityPolicy
		want placement.RouteHandlingPolicy
	}{
		{
			name: "both fixed preserve",
			from: placement.MobilityPolicy{Class: placement.MobilityFixed, RouteHandling: placement.RouteHandlingPreserveFixed},
			to:   placement.MobilityPolicy{Class: placement.MobilityFixed, RouteHandling: placement.RouteHandlingPreserveFixed},
			want: placement.RouteHandlingPreserveFixed,
		},
		{
			name: "same group transforms",
			from: placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "g", RouteHandling: placement.RouteHandlingTransformWithGroup},
			to:   placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "g", RouteHandling: placement.RouteHandlingTransformWithGroup},
			want: placement.RouteHandlingTransformWithGroup,
		},
		{
			name: "cross group rebuilds",
			from: placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "a", RouteHandling: placement.RouteHandlingTransformWithGroup},
			to:   placement.MobilityPolicy{Class: placement.MobilityGroupTransform, GroupID: "b", RouteHandling: placement.RouteHandlingTransformWithGroup},
			want: placement.RouteHandlingInvalidateRebuild,
		},
		{
			name: "mixed fixed movable rebuilds",
			from: placement.MobilityPolicy{Class: placement.MobilityFixed, RouteHandling: placement.RouteHandlingPreserveFixed},
			to:   placement.MobilityPolicy{Class: placement.MobilitySoftPreferred, RouteHandling: placement.RouteHandlingInvalidateRebuild},
			want: placement.RouteHandlingInvalidateRebuild,
		},
		{
			name: "unowned blocks",
			from: placement.MobilityPolicy{Class: placement.MobilityUnowned, RouteHandling: placement.RouteHandlingUnsupported},
			to:   placement.MobilityPolicy{Class: placement.MobilitySoftPreferred, RouteHandling: placement.RouteHandlingInvalidateRebuild},
			want: placement.RouteHandlingUnsupported,
		},
	}
	route := localRouteMobilityFixture().Fragments[0].Realization.LocalRoutes[0]
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyLocalRoute(route, map[string]placement.MobilityPolicy{
				"R1": tc.from,
				"D1": tc.to,
			})
			if got != tc.want {
				t.Fatalf("classifyLocalRoute = %q, want %q", got, tc.want)
			}
		})
	}
}

func localRouteMobilityFixture() PCBFragmentResult {
	return PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "status",
		BlockID:    "led_indicator",
		Realization: blocks.BlockPCBRealizationResult{LocalRoutes: []blocks.RealizedPCBLocalRoute{{
			ID:      "series",
			NetName: "LED",
			From:    transactions.Endpoint{Ref: "R1", Pin: "2"},
			To:      transactions.Endpoint{Ref: "D1", Pin: "1"},
		}}},
	}}}
}
