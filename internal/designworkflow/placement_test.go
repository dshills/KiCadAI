package designworkflow

import (
	"context"
	"math"
	"slices"
	"strings"
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
	if result.Request.Components[0].Mobility.Class != placement.MobilityFixed {
		t.Fatalf("expected fixed mobility when retry disabled: %#v", result.Request.Components[0].Mobility)
	}
	if len(result.Request.Groups) == 0 {
		t.Fatalf("expected block-derived placement groups")
	}
	if len(result.Request.ProximityRules) == 0 {
		t.Fatalf("expected block-derived proximity rules")
	}
}

func TestPlacementKeepoutsPreserveAppliedRoleAsExemptRef(t *testing.T) {
	fragment := BlockFragment{
		InstanceID: "usb_power",
		Realization: blocks.BlockPCBRealizationResult{
			RoleRefs: map[string]string{"usb_c_receptacle": "J1", "alternate_connector_role": "j1"},
		},
		Keepouts: []blocks.PCBKeepout{{
			ID:        "usb_c_edge_keepout",
			Layer:     "F.Cu",
			Bounds:    blocks.RelativeBounds{MinXMM: -5, MinYMM: -8, MaxXMM: 3, MaxYMM: 8},
			AppliesTo: []string{"usb_c_receptacle", "alternate_connector_role"},
		}},
	}

	keepouts := placementKeepoutsFromFragment(fragment)
	if len(keepouts) != 1 {
		t.Fatalf("keepouts = %#v", keepouts)
	}
	if got := keepouts[0].ExemptRefs; len(got) != 1 || got[0] != "J1" {
		t.Fatalf("exempt refs = %#v, want J1", got)
	}
}

func TestPlacementKeepoutFollowsAppliedRoleTranslatedGroup(t *testing.T) {
	fragment := BlockFragment{
		InstanceID:      "controller",
		Realization:     blocks.BlockPCBRealizationResult{RoleRefs: map[string]string{"module": "U1"}},
		PlacementGroups: []blocks.PCBPlacementGroup{{ID: "module_system", ComponentRoles: []string{"module"}, AnchorRole: "module", TranslateAsUnit: true}},
		Keepouts:        []blocks.PCBKeepout{{ID: "antenna", PlacementGroupID: "module_system", AppliesTo: []string{"module"}}},
	}

	keepouts := placementKeepoutsFromFragment(fragment)
	if len(keepouts) != 1 || keepouts[0].GroupID != "controller.module_system" {
		t.Fatalf("keepouts = %#v", keepouts)
	}
}

func TestPlacementGroupBoundsAreRelativeToAuthoredAnchor(t *testing.T) {
	fragment := BlockFragment{
		OriginXMM: 3,
		OriginYMM: 4,
		Realization: blocks.BlockPCBRealizationResult{
			RoleRefs: map[string]string{"module": "U1"},
			Components: []blocks.RealizedPCBComponent{{
				Ref: "U1", Placement: blocks.RelativePlacement{XMM: 20, YMM: 15},
			}},
		},
		PlacementGroups: []blocks.PCBPlacementGroup{{
			ID: "module_system", ComponentRoles: []string{"module"}, AnchorRole: "module",
			Bounds: &blocks.RelativeBounds{MinXMM: -5, MinYMM: -2, MaxXMM: 3, MaxYMM: 4},
		}},
	}

	groups := placementGroupsFromFragment(fragment)
	if len(groups) != 1 || groups[0].Bounds == nil {
		t.Fatalf("groups = %#v", groups)
	}
	want := placement.Rect{Min: placement.Point{XMM: 15, YMM: 13}, Max: placement.Point{XMM: 23, YMM: 19}}
	if *groups[0].Bounds != want {
		t.Fatalf("bounds = %#v, want %#v", *groups[0].Bounds, want)
	}
}

func TestPlacementKeepoutsPreserveRoutingPolicy(t *testing.T) {
	blocksRoute := false
	fragment := BlockFragment{
		InstanceID: "usb_power",
		Keepouts: []blocks.PCBKeepout{{
			ID:          "usb_c_edge_keepout",
			Layer:       "F.Cu",
			Bounds:      blocks.RelativeBounds{MinXMM: -5, MinYMM: -8, MaxXMM: 3, MaxYMM: 8},
			BlocksRoute: &blocksRoute,
		}},
	}

	keepouts := placementKeepoutsFromFragment(fragment)
	if len(keepouts) != 1 {
		t.Fatalf("keepouts = %#v", keepouts)
	}
	if keepouts[0].BlocksRoute == nil || *keepouts[0].BlocksRoute {
		t.Fatalf("blocks route = %#v, want false", keepouts[0].BlocksRoute)
	}
	if keepouts[0].BlocksRoute == fragment.Keepouts[0].BlocksRoute {
		t.Fatal("routing policy pointer should be cloned")
	}
}

func TestPlaceFragmentsHydratesGeneratedMobilityWhenRetryEnabled(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		RoutingRetry: RoutingRetryPolicySpec{
			Enabled: true,
		},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	if len(result.Request.Components) != 2 {
		t.Fatalf("components = %#v", result.Request.Components)
	}
	for _, component := range result.Request.Components {
		if component.Fixed {
			t.Fatalf("retry-enabled generated component unexpectedly fixed: %#v", component)
		}
		if component.Mobility.Class != placement.MobilityGroupTransform {
			t.Fatalf("%s mobility = %#v, want group transform", component.Ref, component.Mobility)
		}
		if component.Mobility.OwnerScope != "block:led_indicator/status" {
			t.Fatalf("%s owner scope = %q", component.Ref, component.Mobility.OwnerScope)
		}
		if component.Mobility.RouteHandling != placement.RouteHandlingTransformWithGroup {
			t.Fatalf("%s route handling = %q", component.Ref, component.Mobility.RouteHandling)
		}
	}
	summary, ok := result.Stage.Summary["mobility"].(placement.MobilitySummary)
	if !ok {
		t.Fatalf("mobility summary = %#v", result.Stage.Summary["mobility"])
	}
	if summary.GroupTransformCount != 2 || summary.EligibleCount != 2 {
		t.Fatalf("mobility summary = %#v", summary)
	}
}

func TestGeneratedPlacementMobilityAllowsEdgeConstrainedGroupTransform(t *testing.T) {
	request := Request{RoutingRetry: RoutingRetryPolicySpec{Enabled: true}}
	component := blocks.RealizedPCBComponent{Ref: "U1"}

	policy, fixed := generatedPlacementMobility(request, BlockFragment{BlockID: "radio", InstanceID: "controller"}, component, "controller.module", placement.EdgeAny, true)

	if fixed {
		t.Fatal("edge-constrained translated group member unexpectedly fixed")
	}
	if policy.Class != placement.MobilityGroupTransform || policy.RouteHandling != placement.RouteHandlingTransformWithGroup {
		t.Fatalf("mobility = %#v, want group transform with group route handling", policy)
	}
}

func TestPreserveAuthoredTranslatedGroupSpreadAccountsForAsymmetricOrigins(t *testing.T) {
	request := placement.Request{
		Components: []placement.Component{
			{Ref: "J1", Position: &placement.Placement{XMM: 0, YMM: 0}, Bounds: placement.Bounds{WidthMM: 10, HeightMM: 8, AnchorOffset: placement.Point{XMM: 2, YMM: 4}, Source: placement.BoundsExplicit}},
			{Ref: "C1", Position: &placement.Placement{XMM: 18, YMM: -1}, Bounds: placement.Bounds{WidthMM: 2, HeightMM: 1, AnchorOffset: placement.Point{XMM: 1, YMM: 0.5}, Source: placement.BoundsExplicit}},
		},
		Groups: []placement.Group{{ID: "entry", Components: []string{"J1", "C1"}, TranslateAsUnit: true, MaxSpreadMM: 10}},
	}

	result := preserveAuthoredTranslatedGroupSpread(request)
	want := math.Hypot(15, 1)
	if math.Abs(result.Groups[0].MaxSpreadMM-want) > 1e-9 {
		t.Fatalf("max spread = %v, want %v", result.Groups[0].MaxSpreadMM, want)
	}
}

func TestPlaceFragmentsPreservesRegulatorCoreRelativePlacement(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "regulated_board",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 75, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "regulator",
			BlockID: "voltage_regulator",
			Params: map[string]any{
				"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
				"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
				"input_voltage":       "5V",
				"output_voltage":      "3.3V",
				"output_current":      "0.1A",
				"enable_mode":         "tied_input",
			},
		}},
		RoutingRetry: RoutingRetryPolicySpec{Enabled: true},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	if len(result.Request.Groups) != 1 || !result.Request.Groups[0].TranslateAsUnit {
		t.Fatalf("regulator placement groups = %#v", result.Request.Groups)
	}

	refs := fragments.Fragments[0].Realization.RoleRefs
	placements := map[string]placement.Placement{}
	for _, placed := range result.Result.Placements {
		placements[placed.Ref] = placed.Position
	}
	regulator := placements[refs["regulator"]]
	input := placements[refs["input_capacitor"]]
	output := placements[refs["output_capacitor"]]
	if input.XMM-regulator.XMM != -6 || input.YMM-regulator.YMM != -4 {
		t.Fatalf("input capacitor offset = (%v,%v), want (-6,-4)", input.XMM-regulator.XMM, input.YMM-regulator.YMM)
	}
	if output.XMM-regulator.XMM != 6 || output.YMM-regulator.YMM != -4 {
		t.Fatalf("output capacitor offset = (%v,%v), want (6,-4)", output.XMM-regulator.XMM, output.YMM-regulator.YMM)
	}
}

func TestPlaceFragmentsMovesRouteFreeConnectorToRequestedEdge(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "bottom_connector",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "io",
			BlockID: "connector_breakout",
			Params: map[string]any{
				"pin_names":   []any{"VCC", "GND", "SDA", "SCL"},
				"edge_facing": true,
				"edge_side":   "bottom",
			},
		}},
		RoutingRetry: RoutingRetryPolicySpec{Enabled: true},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	component := result.Request.Components[0]
	if component.Fixed || component.Edge != placement.EdgeBottom {
		t.Fatalf("connector placement intent = %#v", component)
	}
	placed := result.Result.Placements[0]
	if placed.Position.YMM < request.Board.HeightMM/2 {
		t.Fatalf("connector position = %#v, want bottom half of board", placed.Position)
	}
}

func TestPlaceFragmentsPreservesLegacyRightEdgeConnectorPosition(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "right_connector",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "io",
			BlockID: "connector_breakout",
			Params: map[string]any{
				"pin_names":   []any{"VCC", "GND"},
				"edge_facing": true,
			},
		}},
		RoutingRetry: RoutingRetryPolicySpec{Enabled: true},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	component := result.Request.Components[0]
	if !component.Fixed || component.Edge != placement.EdgeRight {
		t.Fatalf("legacy connector placement intent = %#v", component)
	}
	placed := result.Result.Placements[0]
	if component.Position == nil || placed.Position.XMM != component.Position.XMM || placed.Position.YMM != component.Position.YMM {
		t.Fatalf("legacy connector position = %#v, want authored %#v", placed.Position, component.Position)
	}
}

func TestPlaceFragmentsPromotesRequestConnectionsToPlacementNets(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "connector_led",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_count": 2, "pin_names": []any{"SIG", "GND"}}},
			{ID: "status", BlockID: "led_indicator"},
		},
		Connections: []ConnectionSpec{
			{From: "header.SIG", To: "status.IN", NetAlias: "LED_EN"},
			{From: "header.GND", To: "status.GND", NetAlias: "GND"},
		},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	ledNet, ok := placementNetByName(result.Request.Nets, "LED_EN")
	if !ok {
		t.Fatalf("placement nets = %#v, want LED_EN", result.Request.Nets)
	}
	if !placementNetHasEndpointPrefix(ledNet, "J", "1") || !placementNetHasEndpointPrefix(ledNet, "R", "1") {
		t.Fatalf("LED_EN endpoints = %#v, want connector pad and LED input resistor pad", ledNet.Endpoints)
	}
	gndNet, ok := placementNetByName(result.Request.Nets, "GND")
	if !ok {
		t.Fatalf("placement nets = %#v, want GND", result.Request.Nets)
	}
	if !placementNetHasEndpointPrefix(gndNet, "J", "2") || !placementNetHasEndpointPrefix(gndNet, "D", "1") {
		t.Fatalf("GND endpoints = %#v, want connector ground and LED ground pad", gndNet.Endpoints)
	}
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, result)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	ledCandidate, ok := interBlockCandidateByNet(candidates, "LED_EN")
	if !ok {
		t.Fatalf("candidates = %#v, want LED_EN inter-block candidate", candidates)
	}
	if ledCandidate.Status != InterBlockRouteCandidateRoutable {
		t.Fatalf("LED_EN candidate = %#v, want routable", ledCandidate)
	}
	if !slices.Contains(ledCandidate.InstanceIDs, "header") || !slices.Contains(ledCandidate.InstanceIDs, "status") {
		t.Fatalf("LED_EN candidate instances = %#v, want header and status", ledCandidate.InstanceIDs)
	}
}

func TestPlaceFragmentsPromotesI2CSensorBreakoutConnections(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "i2c_sensor_breakout",
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
		RoutingRetry: RoutingRetryPolicySpec{Enabled: true},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, result)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	for _, netName := range []string{"VCC", "GND", "SDA", "SCL"} {
		candidate, ok := interBlockCandidateByNet(candidates, netName)
		if !ok {
			t.Fatalf("candidates = %#v, want %s", candidates, netName)
		}
		if candidate.Status != InterBlockRouteCandidateRoutable || len(candidate.Endpoints) < 2 {
			t.Fatalf("%s candidate = %#v, want routable endpoint evidence", netName, candidate)
		}
	}
}

func TestPlaceFragmentsReportsUnresolvedRequestConnectionEndpoint(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "connector_led",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_count": 1, "pin_names": []any{"SIG"}}},
			{ID: "status", BlockID: "led_indicator"},
		},
		Connections: []ConnectionSpec{{From: "header.NOPE", To: "status.IN", NetAlias: "LED_EN"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v, want unresolved endpoint warning without blocking placement", result.Stage.Issues)
	}
	assertIssueCode(t, result.Stage.Issues, reports.CodeValidationFailed)
}

func TestPlaceFragmentsSummarizesCandidateScoring(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	rules := placement.DefaultRules()
	rules.CandidateScoring.Enabled = true

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{Rules: rules})
	summary, ok := result.Stage.Summary["candidate_scoring"].(*PlacementCandidateScoringSummary)
	if !ok {
		t.Fatalf("candidate scoring summary = %#v", result.Stage.Summary["candidate_scoring"])
	}
	if !summary.Enabled || summary.WinningCount == 0 || summary.ScoreVersion == "" {
		t.Fatalf("candidate scoring summary incomplete: %#v", summary)
	}
}

func placementNetByName(nets []placement.Net, name string) (placement.Net, bool) {
	for _, net := range nets {
		if strings.EqualFold(net.Name, name) {
			return net, true
		}
	}
	return placement.Net{}, false
}

func placementNetHasEndpointPrefix(net placement.Net, refPrefix string, pin string) bool {
	for _, endpoint := range net.Endpoints {
		if strings.HasPrefix(strings.ToUpper(endpoint.Ref), strings.ToUpper(refPrefix)) && strings.EqualFold(endpoint.Pin, pin) {
			return true
		}
	}
	return false
}

func interBlockCandidateByNet(candidates []InterBlockRouteCandidate, name string) (InterBlockRouteCandidate, bool) {
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.NetName, name) {
			return candidate, true
		}
	}
	return InterBlockRouteCandidate{}, false
}

func TestPlacementCandidateScoringSummaryIncludesAdvancedRules(t *testing.T) {
	report := &placement.CandidateScoringReport{
		RejectedByReason: map[string]int{string(placement.CandidateRejectAdvancedRule): 2},
		WinningCandidates: []placement.CandidateScore{{
			Ref: "U1",
			Dimensions: []placement.CandidateScoreDimension{{
				Name:     placement.CandidateScoreControlledImpedance,
				Score:    0.4,
				Evidence: []string{"rule=usb reference_plane_missing"},
			}},
		}},
	}

	summary := placementCandidateScoringSummary(report)
	if summary == nil || summary.AdvancedRules == nil {
		t.Fatalf("advanced summary missing: %#v", summary)
	}
	if summary.AdvancedRules.DimensionCounts[string(placement.CandidateScoreControlledImpedance)] != 1 {
		t.Fatalf("dimension counts = %#v", summary.AdvancedRules.DimensionCounts)
	}
	if summary.AdvancedRules.HardViolations != 2 || summary.AdvancedRules.Warnings != 1 || summary.AdvancedRules.Unsupported != 1 {
		t.Fatalf("advanced summary counts = %#v", summary.AdvancedRules)
	}
}

func TestPlaceFragmentsHydratesGeneratedPadsFromVerifiedTemplates(t *testing.T) {
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
		if len(component.Pads) != 2 {
			t.Fatalf("%s pads = %#v, want verified template pads", component.Ref, component.Pads)
		}
	}
	summary, ok := result.Stage.Summary["pad_hydration"].(PadHydrationSummary)
	if !ok || summary.SourceCounts[PadHydrationSourceVerifiedTemplate] != 2 {
		t.Fatalf("pad hydration summary = %#v", result.Stage.Summary["pad_hydration"])
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

func TestHydratePlacementRequestPadsExpandsLogicalPadsFromResolverFootprint(t *testing.T) {
	const footprintID = "Connector_Test:DuplicateShieldPads"
	request := placement.Request{
		Components: []placement.Component{{
			Ref:         "J1",
			FootprintID: footprintID,
			Pads: []placement.PadSummary{
				{Name: "1", Net: "VBUS"},
				{Name: "SH", Net: "GND"},
			},
		}},
		Nets: []placement.Net{
			{Name: "VBUS", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "1"}}},
			{Name: "GND", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "SH"}}},
		},
	}
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		footprintID: {
			FootprintID: footprintID,
			BoundingBox: libraryresolver.BoundingBox{
				Min: kicadfiles.Point{X: -5_000_000, Y: -3_000_000},
				Max: kicadfiles.Point{X: 5_000_000, Y: 3_000_000},
			},
			Pads: []libraryresolver.FootprintPad{
				{Name: "1", Position: kicadfiles.Point{X: 0, Y: -2_000_000}, Size: kicadfiles.Point{X: 800_000, Y: 1_200_000}},
				{Name: "SH", Position: kicadfiles.Point{X: -4_000_000, Y: -2_000_000}, Size: kicadfiles.Point{X: 1_100_000, Y: 1_700_000}},
				{Name: "SH", Position: kicadfiles.Point{X: -4_000_000, Y: 1_000_000}, Size: kicadfiles.Point{X: 1_100_000, Y: 1_700_000}},
				{Name: "SH", Position: kicadfiles.Point{X: 4_000_000, Y: -2_000_000}, Size: kicadfiles.Point{X: 1_100_000, Y: 1_700_000}},
				{Name: "SH", Position: kicadfiles.Point{X: 4_000_000, Y: 1_000_000}, Size: kicadfiles.Point{X: 1_100_000, Y: 1_700_000}},
			},
		},
	}}

	got, entries, issues := hydratePlacementRequestPads(request, &index)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("hydration issues = %#v", issues)
	}
	if len(entries) != 1 || entries[0].Source != PadHydrationSourceResolver || entries[0].PadCount != 5 {
		t.Fatalf("hydration entries = %#v, want five resolver pads", entries)
	}
	if len(got.Components[0].Pads) != 5 {
		t.Fatalf("physical pads = %#v, want complete resolver footprint", got.Components[0].Pads)
	}
	shieldCount := 0
	for _, pad := range got.Components[0].Pads {
		if pad.Name == "SH" {
			shieldCount++
			if pad.Net != "GND" {
				t.Fatalf("shield pad net = %q, want GND", pad.Net)
			}
		}
	}
	if shieldCount != 4 {
		t.Fatalf("shield pad count = %d, want 4", shieldCount)
	}
}

func TestHydratePlacementRequestPadsBlocksUnknownFootprint(t *testing.T) {
	request := placement.Request{
		Components: []placement.Component{{Ref: "X1", FootprintID: "Unknown:Missing", Bounds: defaultWorkflowBounds}},
	}
	_, entries, issues := hydratePlacementRequestPads(request, nil)
	if len(entries) != 1 || entries[0].Source != PadHydrationSourceMissing {
		t.Fatalf("entries = %#v", entries)
	}
	if len(issues) != 2 || !issues[0].Blocking() || !issues[1].Blocking() {
		t.Fatalf("issues = %#v", issues)
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
			if !component.Fixed || component.Mobility.Class != placement.MobilityFixed {
				t.Fatalf("usb connector should remain fixed: fixed=%v mobility=%#v", component.Fixed, component.Mobility)
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

func TestPlaceFragmentsHonorsExplicitBoardEdgeClearance(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, EdgeClearanceMM: 0.25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if result.Request.Board.MarginMM != 0.25 || result.Request.Rules.BoardEdgeClearanceMM != 0.25 {
		t.Fatalf("edge clearance not honored: board=%#v rules=%#v", result.Request.Board, result.Request.Rules)
	}
}
