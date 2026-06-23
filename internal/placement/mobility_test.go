package placement

import "testing"

func TestNormalizeRequestDefaultsMobilityConservatively(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Fixed = false
	req.Components[0].Mobility = MobilityPolicy{}

	got := NormalizeRequest(req)
	policy := got.Components[0].Mobility
	if policy.Class != MobilityUnowned {
		t.Fatalf("mobility class = %q, want %q", policy.Class, MobilityUnowned)
	}
	if policy.RouteHandling != RouteHandlingUnsupported {
		t.Fatalf("route handling = %q, want %q", policy.RouteHandling, RouteHandlingUnsupported)
	}
	if policy.Reason == "" {
		t.Fatalf("reason is empty: %#v", policy)
	}
}

func TestNormalizeRequestKeepsExplicitFixedMobility(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Components[0].Mobility = MobilityPolicy{Class: MobilitySoftPreferred, RouteHandling: RouteHandlingInvalidateRebuild}

	got := NormalizeRequest(req)
	policy := got.Components[0].Mobility
	if policy.Class != MobilityFixed {
		t.Fatalf("mobility class = %q, want %q", policy.Class, MobilityFixed)
	}
	if policy.RouteHandling != RouteHandlingPreserveFixed {
		t.Fatalf("route handling = %q, want %q", policy.RouteHandling, RouteHandlingPreserveFixed)
	}
}

func TestNormalizeRequestPreservesGeneratedMobilityPolicy(t *testing.T) {
	req := minimalRequest()
	req.Components[0].GroupID = " led "
	req.Components[0].Mobility = MobilityPolicy{
		Class:       MobilityGroupTransform,
		OwnerScope:  " block/status ",
		Transforms:  []string{"translate", "translate"},
		Constraints: []string{" edge ", "edge"},
	}

	got := NormalizeRequest(req)
	policy := got.Components[0].Mobility
	if policy.Class != MobilityGroupTransform {
		t.Fatalf("mobility class = %q, want %q", policy.Class, MobilityGroupTransform)
	}
	if policy.GroupID != "led" {
		t.Fatalf("group id = %q, want led", policy.GroupID)
	}
	if policy.OwnerScope != "block/status" {
		t.Fatalf("owner scope = %q, want block/status", policy.OwnerScope)
	}
	if policy.RouteHandling != RouteHandlingTransformWithGroup {
		t.Fatalf("route handling = %q, want %q", policy.RouteHandling, RouteHandlingTransformWithGroup)
	}
	if len(policy.Transforms) != 1 || policy.Transforms[0] != "translate" {
		t.Fatalf("transforms = %#v, want unique translate", policy.Transforms)
	}
	if len(policy.Constraints) != 1 || policy.Constraints[0] != "edge" {
		t.Fatalf("constraints = %#v, want unique edge", policy.Constraints)
	}
}

func TestMobilitySummaryForComponentsCountsClasses(t *testing.T) {
	components := []Component{
		{Ref: "R2", Mobility: MobilityPolicy{Class: MobilityLocalRebuild}},
		{Ref: "J1", Fixed: true},
		{Ref: "R1", Mobility: MobilityPolicy{Class: MobilityGroupTransform}},
		{Ref: "U1"},
	}

	got := MobilitySummaryForComponents(components)
	if got.Total != 4 || got.EligibleCount != 2 || got.FixedCount != 1 || got.UnownedCount != 1 {
		t.Fatalf("summary counts = %#v", got)
	}
	if got.ByClass[string(MobilityGroupTransform)] != 1 || got.ByClass[string(MobilityLocalRebuild)] != 1 {
		t.Fatalf("class counts = %#v", got.ByClass)
	}
	if got.TransformableRouteCnt != 1 || got.RebuildableRouteCnt != 1 || got.UnsupportedRouteCnt != 1 {
		t.Fatalf("route counts = %#v", got)
	}
	if got.PreservedRouteCnt != 1 {
		t.Fatalf("preserved route count = %d, want 1", got.PreservedRouteCnt)
	}
}

func TestPlacementResultEchoesMobility(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Mobility = MobilityPolicy{Class: MobilitySoftPreferred, OwnerScope: "generated"}
	req = NormalizeRequest(req)

	placed, ok := NewPlacementResult(req.Components[0], Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}, req.Rules)
	if !ok {
		t.Fatal("NewPlacementResult returned false")
	}
	if placed.Mobility.Class != MobilitySoftPreferred || placed.Mobility.OwnerScope != "generated" {
		t.Fatalf("placement mobility = %#v", placed.Mobility)
	}
}

func TestMobilitySummaryForResultsCountsEchoedPolicy(t *testing.T) {
	placements := []PlacementResult{
		{Ref: "R1", Mobility: MobilityPolicy{Class: MobilityGroupTransform, RouteHandling: RouteHandlingTransformWithGroup}},
		{Ref: "J1", Mobility: MobilityPolicy{Class: MobilityFixed, RouteHandling: RouteHandlingPreserveFixed}},
	}

	got := MobilitySummaryForResults(placements)
	if got.Total != 2 || got.EligibleCount != 1 || got.GroupTransformCount != 1 || got.FixedCount != 1 {
		t.Fatalf("summary = %#v", got)
	}
	if got.TransformableRouteCnt != 1 || got.PreservedRouteCnt != 1 {
		t.Fatalf("route handling summary = %#v", got)
	}
}
