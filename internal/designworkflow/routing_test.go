package designworkflow

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestRoutePlacementUsesGeneratedPadSummariesForLocalRoutes(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	placed := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})

	result := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{})
	if countTransactionOps(result.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want local route operation", result.Operations)
	}
	if result.Stage.Status != StageStatusOK {
		t.Fatalf("stage = %#v, want clean local route connectivity", result.Stage)
	}
	localRoutes, ok := result.Stage.Summary["local_route_mobility"].(LocalRouteMobilitySummary)
	if !ok || localRoutes.Total == 0 || localRoutes.Preserved == 0 {
		t.Fatalf("local route mobility summary = %#v", result.Stage.Summary["local_route_mobility"])
	}
	assertNoIssueCode(t, result.Stage.Issues, reports.CodeDisconnectedPad)
}

func TestRoutePlacementAuditShowsNamedLocalRouteCanStillMissPhysicalPads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(ctx, registry, request)
	if !stageUsableForRoutingTest(plan.Stage) {
		t.Fatalf("planning failed: %#v", plan.Stage.Issues)
	}
	fragments := RealizePCBFragments(ctx, registry, plan)
	if !stageUsableForRoutingTest(fragments.Stage) {
		t.Fatalf("PCB realization failed: %#v", fragments.Stage.Issues)
	}
	placed := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if !stageUsableForRoutingTest(placed.Stage) {
		t.Fatalf("placement failed: %#v", placed.Stage.Issues)
	}
	routed := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	netAssignment := SummarizeGeneratedNetAssignment(&placed, &routed)

	if netAssignment.AssignedCopperObjects == 0 {
		t.Fatalf("net assignment = %#v, want assigned local-route copper", netAssignment)
	}
	if routed.Stage.Status != StageStatusOK {
		t.Fatalf("routing stage = %#v, want physical route endpoint proof", routed.Stage)
	}
	assertNoIssueCode(t, routed.Stage.Issues, reports.CodeDisconnectedPad)
	if countTransactionOps(routed.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want named local route operation", routed.Operations)
	}
}

func TestLocalRouteOperationsBindToPlacedPadEndpoints(t *testing.T) {
	extraRoute := mustGeneratedNetAssignmentRouteOperation(t, "EXTRA")
	fragments := PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "status",
		BlockID:    "led_indicator",
		Realization: blocks.BlockPCBRealizationResult{
			LocalRoutes: []blocks.RealizedPCBLocalRoute{{
				ID:      "series",
				NetName: "SIG",
				From:    transactions.Endpoint{Ref: "R1", Pin: "2"},
				To:      transactions.Endpoint{Ref: "D1", Pin: "1"},
				Layer:   "F.Cu",
				WidthMM: 0.25,
			}},
			Operations: []transactions.Operation{extraRoute},
		},
	}}}
	placed := PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{
				{Ref: "R1", FootprintID: "Test:R", Pads: []placement.PadSummary{{Name: "2", Net: "SIG", XMM: 1, YMM: 0}}},
				{Ref: "D1", FootprintID: "Test:D", Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: -1, YMM: 0}}},
			},
			Nets: []placement.Net{{Name: "SIG", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}}},
		},
		Result: placement.Result{Status: placement.StatusPlaced, Placements: []placement.PlacementResult{
			{Ref: "R1", FootprintID: "Test:R", Position: placement.Placement{XMM: 10, YMM: 5, Layer: "F.Cu"}},
			{Ref: "D1", FootprintID: "Test:D", Position: placement.Placement{XMM: 20, YMM: 5, Layer: "F.Cu"}},
		}},
		Stage: NewStageResult(StagePlacement, nil),
	}

	operations, issues, summary := localRouteOperations(fragments, &placed)
	if len(issues) != 0 {
		t.Fatalf("local route binding issues = %#v", issues)
	}
	if summary.RoutesAttempted != 1 || summary.RoutesBound != 1 || summary.EndpointsResolved != 2 || summary.EndpointContactsProven != 2 || summary.EmittedTrackSegments != 1 {
		t.Fatalf("route connectivity summary = %#v", summary)
	}
	if len(operations) != 2 {
		t.Fatalf("operations = %#v, want preserved extra route and one bound route", operations)
	}
	var route transactions.RouteOperation
	if err := json.Unmarshal(operations[1].Raw, &route); err != nil {
		t.Fatal(err)
	}
	if len(route.Points) != 2 ||
		route.Points[0].XMM != 11 || route.Points[0].YMM != 5 ||
		route.Points[1].XMM != 19 || route.Points[1].YMM != 5 {
		t.Fatalf("route points = %#v, want physical pad centers", route.Points)
	}
}

func TestLocalRouteConnectivitySummaryJSONStable(t *testing.T) {
	summary := LocalRouteConnectivitySummary{
		RoutesAttempted:        1,
		RoutesBound:            1,
		EndpointsResolved:      2,
		EndpointsUnresolved:    0,
		EndpointContactsProven: 2,
		EndpointNetMismatches:  0,
		EmittedTrackSegments:   1,
		IssueCount:             0,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"routes_attempted":1,"routes_bound":1,"endpoints_resolved":2,"endpoints_unresolved":0,"endpoint_contacts_proven":2,"endpoint_net_mismatches":0,"emitted_track_segments":1,"issue_count":0}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func TestRoutePlacementAddsAnchorBindingRoutes(t *testing.T) {
	request := Request{Version: RequestVersion, Name: "anchor", Board: BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1}}
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 7, YMM: 10, Layer: "F.Cu"},
	})
	placed := PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 30, HeightMM: 20},
			Rules: placement.DefaultRules(),
			Components: []placement.Component{{
				Ref:         "J1",
				Role:        "connector",
				FootprintID: "Test:Pad",
				Pads:        []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
			}},
		},
		Result: placement.Result{
			Status: placement.StatusPlaced,
			Placements: []placement.PlacementResult{{
				Ref: "J1", FootprintID: "Test:Pad", Position: placement.Placement{XMM: 5, YMM: 10, Layer: "F.Cu"},
			}},
			Metrics: placement.Metrics{PlacedCount: 1},
		},
		Stage: NewStageResult(StagePlacement, nil),
	}

	result := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{Mode: routing.ModeSingleLayer, TraceWidthMM: 0.3})

	value, ok := result.Stage.Summary["anchor_bindings"]
	if !ok {
		t.Fatalf("anchor binding summary missing: %#v", result.Stage.Summary)
	}
	summary, ok := value.(AnchorBindingSummary)
	if !ok || summary.Bound != 1 || summary.Routed != 1 {
		t.Fatalf("anchor binding summary = %#v", value)
	}
	var found bool
	for _, operation := range result.Operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		var payload transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("route unmarshal = %v", err)
		}
		if payload.NetName == "SIG" && len(payload.Points) == 2 && payload.Points[0].XMM == 5 && payload.Points[1].XMM == 7 {
			found = true
		}
	}
	if !found {
		t.Fatalf("operations = %#v, want anchor binding route", result.Operations)
	}
}

func TestRoutePlacementRoutesSimpleSignalWithPads(t *testing.T) {
	placed := simplePlacedPads()
	request := Request{Version: RequestVersion, Name: "simple", Board: BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1}}

	result := RoutePlacement(context.Background(), request, PCBFragmentResult{}, placed, RoutingOptions{Mode: routing.ModeSingleLayer})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("routing issues = %#v", result.Stage.Issues)
	}
	if result.Result.Status != routing.StatusRouted || len(result.Result.Operations) == 0 {
		t.Fatalf("routing result = %#v", result.Result)
	}
	if countTransactionOps(result.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want transaction route operation", result.Operations)
	}
	if result.Stage.Summary["quality_score"] == nil ||
		result.Stage.Summary["route_reports"] == nil ||
		result.Stage.Summary["repair_diagnostics"] == nil {
		t.Fatalf("routing summary missing quality evidence: %#v", result.Stage.Summary)
	}
}

func TestRoutePlacementPromotedInterBlockConnectorLEDNetReportsDisconnectedCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request := Request{
		Version: RequestVersion,
		Name:    "connector_led",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_count": 2, "pin_names": []string{"SIG", "GND"}}},
			{ID: "status", BlockID: "led_indicator"},
		},
		Connections: []ConnectionSpec{{From: "header.SIG", To: "status.IN", NetAlias: "LED_EN"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(ctx, registry, request)
	if !stageUsableForRoutingTest(plan.Stage) {
		t.Fatalf("planning failed: %#v", plan.Stage.Issues)
	}
	fragments := RealizePCBFragments(ctx, registry, plan)
	if !stageUsableForRoutingTest(fragments.Stage) {
		t.Fatalf("PCB realization failed: %#v", fragments.Stage.Issues)
	}
	placed := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if !stageUsableForRoutingTest(placed.Stage) {
		t.Fatalf("placement failed: %#v", placed.Stage.Issues)
	}
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	if _, ok := interBlockCandidateByNetForRoutingTest(candidates, "LED_EN"); !ok {
		t.Fatalf("candidates = %#v, want LED_EN", candidates)
	}

	result := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	if result.Stage.Status != StageStatusOK {
		t.Fatalf("routing status = %s, want proven route-completion evidence; issues=%#v", result.Stage.Status, result.Stage.Issues)
	}
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, result.Stage)
	if !stringSliceContains(routeTrees.ManagedNets, "LED_EN") {
		t.Fatalf("route-tree managed nets = %#v, want LED_EN", routeTrees.ManagedNets)
	}
	if routingRequestHasNet(result.Request, "LED_EN") {
		t.Fatalf("routing request nets = %#v, did not want route-tree managed LED_EN in fallback request", result.Request.Nets)
	}
	routes := requireRouteOperationsForNet(t, result.Operations, "LED_EN")
	for _, route := range routes {
		if len(route.Points) < 2 {
			t.Fatalf("LED_EN route has %d points, want at least 2", len(route.Points))
		}
		for pointIndex := 0; pointIndex < len(route.Points)-1; pointIndex++ {
			current := route.Points[pointIndex]
			next := route.Points[pointIndex+1]
			if pointsNearlyEqual(current, next) {
				t.Fatalf("LED_EN route has degenerate segment at point %d: (%.9f, %.9f) -> (%.9f, %.9f)", pointIndex, current.XMM, current.YMM, next.XMM, next.YMM)
			}
		}
	}
	assertNoIssueCode(t, result.Stage.Issues, reports.CodeDisconnectedPad)
	interBlock := requireInterBlockRouteSummary(t, result.Stage)
	if interBlock.Candidates == 0 || interBlock.RoutesAttempted == 0 {
		t.Fatalf("inter-block summary = %#v, want attempted candidate", interBlock)
	}
	if interBlock.RoutesCompleted == 0 || interBlock.PartialNets != 0 || interBlock.EmittedSegments == 0 || interBlock.IssueCount != 0 {
		t.Fatalf("inter-block summary = %#v, want completed routed evidence for LED_EN", interBlock)
	}
	contacts := requireInterBlockContactSummary(t, result.Stage)
	if contacts.ContactsRequired == 0 || contacts.ContactsProven == 0 || contacts.ContactMisses != 0 {
		t.Fatalf("inter-block contact summary = %#v, want snapped contact evidence", contacts)
	}
}

func TestRoutePlacementI2CSensorBreakoutReportsInterBlockContactEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	candidateNets := interBlockCandidateNetSetForRoutingTest(candidates)
	for _, connection := range request.Connections {
		netName := connection.NetAlias
		candidate, ok := interBlockCandidateByNetForRoutingTest(candidates, netName)
		if !ok || candidate.Status != InterBlockRouteCandidateRoutable || len(candidate.Endpoints) < 2 {
			t.Fatalf("candidate %s = %#v, ok=%v", netName, candidate, ok)
		}
	}
	for net := range connectionAliasSet(request.Connections) {
		if !candidateNets[net] {
			t.Fatalf("candidate nets = %#v, want request alias %s", candidateNets, net)
		}
	}
	expectedEndpoints := interBlockCandidateEndpointCount(candidates)

	result := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, result.Stage)
	for _, connection := range request.Connections {
		if !stringSliceContains(routeTrees.ManagedNets, connection.NetAlias) {
			t.Fatalf("route-tree managed nets = %#v, want %s", routeTrees.ManagedNets, connection.NetAlias)
		}
	}
	interBlock := requireInterBlockRouteSummary(t, result.Stage)
	if interBlock.Candidates != len(candidates) || interBlock.EndpointsResolved != expectedEndpoints {
		t.Fatalf("inter-block summary counts = candidates %d endpoints %d, want candidate builder counts %d and %d", interBlock.Candidates, interBlock.EndpointsResolved, len(candidates), expectedEndpoints)
	}
	if interBlock.MultiEndpointNets != len(request.Connections) || interBlock.RequiredEndpoints != expectedEndpoints {
		t.Fatalf("inter-block group summary = %#v, want multi-endpoint net and required endpoint counts", interBlock)
	}
	if interBlock.BranchesPlanned == 0 || interBlock.GraphComponentCount == 0 {
		t.Fatalf("inter-block route-tree summary = %#v, want planned branches and graph component evidence", interBlock)
	}
	contacts := requireInterBlockContactSummary(t, result.Stage)
	if contacts.ContactsRequired != expectedEndpoints || contacts.ContactsProven+contacts.ContactsFailed != expectedEndpoints {
		t.Fatalf("inter-block contact counts = required %d resolved %d, want %d", contacts.ContactsRequired, contacts.ContactsProven+contacts.ContactsFailed, expectedEndpoints)
	}
}

func TestRoutePlacementI2CSensorBreakoutAuditsMultiEndpointBlocker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)

	result := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	local := requireStageSummary[LocalRouteConnectivitySummary](t, result.Stage, "route_connectivity")
	if local.RoutesAttempted == 0 || local.RoutesBound != local.RoutesAttempted {
		t.Fatalf("local route connectivity = %#v, want every local route bound", local)
	}
	if local.EndpointsUnresolved != 0 || local.EndpointNetMismatches != 0 || local.IssueCount != 0 {
		t.Fatalf("local route connectivity = %#v, want no local endpoint blockers", local)
	}
	if local.EndpointContactsProven < local.RoutesAttempted*2 {
		t.Fatalf("local route connectivity = %#v, want at least two proven endpoint contacts per local route", local)
	}

	interBlock := requireInterBlockRouteSummary(t, result.Stage)
	expectedNets := len(connectionAliasSet(request.Connections))
	if interBlock.Candidates != expectedNets || interBlock.EndpointsResolved <= expectedNets*2 {
		t.Fatalf("inter-block summary = %#v, want four multi-endpoint I2C candidates", interBlock)
	}
	if interBlock.MultiEndpointNets != expectedNets || interBlock.RequiredEndpoints != interBlock.EndpointsResolved {
		t.Fatalf("inter-block group summary = %#v, want all I2C nets represented as multi-endpoint groups", interBlock)
	}
	if interBlock.BranchesPlanned < expectedNets || interBlock.BranchesAttempted == 0 || interBlock.BranchesAttempted > interBlock.BranchesPlanned {
		t.Fatalf("inter-block branch summary = %#v, want attempted branches bounded by planned branches", interBlock)
	}
	if interBlock.MissingRequired != 0 {
		t.Fatalf("inter-block route-tree missing endpoints = %#v, want target resolution complete", interBlock)
	}
	// Phase 1 intentionally documents the pre-implementation failure boundary.
	// Later multi-endpoint routing phases should invert this assertion when the
	// I2C route groups become graph-complete.
	if interBlock.RoutesCompleted >= interBlock.Candidates || interBlock.PartialNets+interBlock.UnroutedNets == 0 {
		t.Fatalf("inter-block summary = %#v, want current multi-endpoint route completion blocker", interBlock)
	}
	if interBlock.IssueCount == 0 {
		t.Fatalf("inter-block summary = %#v, want actionable route/contact issues", interBlock)
	}

	contacts := requireInterBlockContactSummary(t, result.Stage)
	if contacts.ContactsRequired != interBlock.EndpointsResolved {
		t.Fatalf("contact summary = %#v, inter-block summary = %#v, want contact targets for every resolved endpoint", contacts, interBlock)
	}
	if contacts.ContactsFailed == 0 || contacts.MissingTargets+contacts.ContactMisses == 0 {
		t.Fatalf("contact summary = %#v, want current missing-target or contact-miss blocker", contacts)
	}
	if contacts.NetMismatches != 0 {
		t.Fatalf("contact summary = %#v, want no net-alias mismatch after I2C alias hydration", contacts)
	}

	blockedNets := issueNetSet(result.Stage.Issues)
	i2cNets := connectionAliasSet(request.Connections)
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, result.Stage)
	for net := range i2cNets {
		if !stringSliceContains(routeTrees.ManagedNets, net) {
			t.Fatalf("route-tree managed nets = %#v, want %s", routeTrees.ManagedNets, net)
		}
	}
	if len(blockedNets) == 0 {
		t.Fatalf("issues = %#v, want named I2C net blockers", result.Stage.Issues)
	}
	for net := range blockedNets {
		if !i2cNets[net] {
			t.Fatalf("blocked nets = %#v, want blockers tied to I2C nets", blockedNets)
		}
	}
}

func connectionAliasSet(connections []ConnectionSpec) map[string]bool {
	nets := map[string]bool{}
	for _, connection := range connections {
		if connection.NetAlias != "" {
			nets[connection.NetAlias] = true
		}
	}
	return nets
}

func i2cSensorBreakoutRoutingFixture(t *testing.T, ctx context.Context) (Request, PCBFragmentResult, PlacementStageResult) {
	t.Helper()
	request := Request{
		Version: RequestVersion,
		Name:    "i2c_sensor_breakout_candidate",
		Board:   BoardSpec{WidthMM: 90, HeightMM: 60, Layers: 2},
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
	plan := PlanBlocks(ctx, registry, request)
	if !stageUsableForRoutingTest(plan.Stage) {
		t.Fatalf("planning failed: %#v", plan.Stage.Issues)
	}
	fragments := RealizePCBFragments(ctx, registry, plan)
	if !stageUsableForRoutingTest(fragments.Stage) {
		t.Fatalf("PCB realization failed: %#v", fragments.Stage.Issues)
	}
	placed := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if !stageUsableForRoutingTest(placed.Stage) {
		t.Fatalf("placement failed: %#v", placed.Stage.Issues)
	}
	return request, fragments, placed
}

func issueNetSet(issues []reports.Issue) map[string]bool {
	nets := map[string]bool{}
	for _, issue := range issues {
		for _, net := range issue.Nets {
			if net != "" {
				nets[net] = true
			}
		}
	}
	return nets
}

// requireRouteOperationsForNet decodes every transaction route operation for a
// net so tests can inspect multi-segment routing evidence without relying on
// the first matching operation.
func requireRouteOperationsForNet(t *testing.T, operations []transactions.Operation, name string) []transactions.RouteOperation {
	t.Helper()
	var routes []transactions.RouteOperation
	for _, operation := range operations {
		if operation.Op != transactions.OpRoute || operation.Net != name {
			continue
		}
		var route transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &route); err != nil {
			t.Fatalf("route operation raw = %s: %v", operation.Raw, err)
		}
		routes = append(routes, route)
	}
	if len(routes) == 0 {
		t.Fatalf("route operation nets = %#v, want route operation for net %s", routeOperationNets(operations), name)
	}
	return routes
}

// routeOperationNets returns the net names carried by route transactions for
// compact failure output.
func routeOperationNets(operations []transactions.Operation) []string {
	var nets []string
	for _, operation := range operations {
		if operation.Op == transactions.OpRoute {
			nets = append(nets, operation.Net)
		}
	}
	return nets
}

func pointsNearlyEqual(left transactions.Point, right transactions.Point) bool {
	const tolerance = 1e-6
	return math.Abs(left.XMM-right.XMM) <= tolerance && math.Abs(left.YMM-right.YMM) <= tolerance
}

func TestRoutePlacementSingleLayerUsesPlacedLayer(t *testing.T) {
	placed := simplePlacedPads()
	placed.Result.Placements[0].Position.Layer = "B.Cu"
	placed.Result.Placements[1].Position.Layer = "B.Cu"
	request := Request{Version: RequestVersion, Name: "bottom", Board: BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1}}

	result := RoutePlacement(context.Background(), request, PCBFragmentResult{}, placed, RoutingOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("routing issues = %#v", result.Stage.Issues)
	}
	if len(result.Request.Board.Layers) != 1 || result.Request.Board.Layers[0].Name != "B.Cu" {
		t.Fatalf("routing board layers = %#v", result.Request.Board.Layers)
	}
	if result.Request.Rules.PreferLayer != "B.Cu" {
		t.Fatalf("prefer layer = %q", result.Request.Rules.PreferLayer)
	}
}

func TestInterBlockRouteCompletionSummaryJSONStable(t *testing.T) {
	summary := InterBlockRouteCompletionSummary{
		NetsConsidered:      1,
		Candidates:          1,
		RoutesAttempted:     1,
		RoutesCompleted:     0,
		EndpointsResolved:   2,
		EndpointsUnresolved: 0,
		PartialNets:         1,
		UnroutedNets:        0,
		EmittedSegments:     1,
		IssueCount:          1,
		MultiEndpointNets:   1,
		RequiredEndpoints:   3,
		ProvenEndpoints:     2,
		BranchesPlanned:     2,
		BranchesAttempted:   2,
		BranchesCompleted:   1,
		GraphComponentCount: 2,
		MissingRequired:     1,
		CompleteGroups:      0,
		PartialGroups:       1,
		BlockedGroups:       0,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"nets_considered":1,"candidates":1,"routes_attempted":1,"routes_completed":0,"endpoints_resolved":2,"endpoints_unresolved":0,"partial_nets":1,"unrouted_nets":0,"emitted_segments":1,"issue_count":1,"multi_endpoint_nets":1,"required_endpoints":3,"proven_endpoints":2,"branches_planned":2,"branches_attempted":2,"branches_completed":1,"graph_component_count":2,"missing_required_endpoints":1,"complete_groups":0,"partial_groups":1,"blocked_groups":0}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func TestInterBlockRouteTreeExecutionSummaryJSONStable(t *testing.T) {
	summary := InterBlockRouteTreeExecutionSummary{
		GroupsPlanned:     1,
		GroupsAttempted:   1,
		GroupsComplete:    0,
		GroupsPartial:     1,
		GroupsBlocked:     0,
		BranchesPlanned:   2,
		BranchesAttempted: 2,
		BranchesRouted:    1,
		BranchesBlocked:   1,
		ContactMisses:     1,
		GraphSplits:       1,
		IssueCount:        1,
		ManagedNets:       []string{"SDA"},
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"groups_planned":1,"groups_attempted":1,"groups_complete":0,"groups_partial":1,"groups_blocked":0,"branches_planned":2,"branches_attempted":2,"branches_routed":1,"branches_blocked":1,"contact_misses":1,"graph_splits":1,"issue_count":1,"managed_nets":["SDA"]}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func routingRequestHasNet(request routing.Request, name string) bool {
	for _, net := range request.Nets {
		if net.Name == name {
			return true
		}
	}
	return false
}

func routeOperationsContainNet(operations []transactions.Operation, name string) bool {
	for _, operation := range operations {
		if operation.Op == transactions.OpRoute && operation.Net == name {
			return true
		}
	}
	return false
}

func interBlockCandidateByNetForRoutingTest(candidates []InterBlockRouteCandidate, name string) (InterBlockRouteCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.NetName == name {
			return candidate, true
		}
	}
	return InterBlockRouteCandidate{}, false
}

func interBlockCandidateNetSetForRoutingTest(candidates []InterBlockRouteCandidate) map[string]bool {
	nets := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.NetName != "" {
			nets[candidate.NetName] = true
		}
	}
	return nets
}

func interBlockCandidateEndpointCount(candidates []InterBlockRouteCandidate) int {
	count := 0
	for _, candidate := range candidates {
		count += len(candidate.Endpoints)
	}
	return count
}

func assertNetHasIssueCode(t *testing.T, issues []reports.Issue, net string, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code != code {
			continue
		}
		for _, issueNet := range issue.Nets {
			if issueNet == net {
				return
			}
		}
	}
	t.Fatalf("issues = %#v, want issue code %s for net %s", issues, code, net)
}

func assertNoIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			t.Fatalf("issues = %#v, did not want issue code %s", issues, code)
		}
	}
}

func stageUsableForRoutingTest(stage StageResult) bool {
	return stage.Status == StageStatusOK || stage.Status == StageStatusWarning
}

func requireInterBlockRouteSummary(t *testing.T, stage StageResult) InterBlockRouteCompletionSummary {
	t.Helper()
	return requireStageSummary[InterBlockRouteCompletionSummary](t, stage, "inter_block_routing")
}

func requireInterBlockContactSummary(t *testing.T, stage StageResult) InterBlockContactSummary {
	t.Helper()
	return requireStageSummary[InterBlockContactSummary](t, stage, "inter_block_contacts")
}

func requireInterBlockRouteTreeExecutionSummary(t *testing.T, stage StageResult) InterBlockRouteTreeExecutionSummary {
	t.Helper()
	return requireStageSummary[InterBlockRouteTreeExecutionSummary](t, stage, "inter_block_route_trees")
}

func requireStageSummary[T any](t *testing.T, stage StageResult, key string) T {
	t.Helper()
	var zero T
	value, exists := stage.Summary[key]
	if !exists {
		t.Fatalf("missing %s summary: %#v", key, stage.Summary)
	}
	summary, ok := value.(T)
	if !ok {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s summary has type %T and could not be marshaled: %v", key, value, err)
		}
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatalf("%s summary has type %T and could not be decoded into %T: %v", key, value, zero, err)
		}
	}
	return summary
}

func TestRoutePlacementReportsUnroutableSignal(t *testing.T) {
	placed := simplePlacedPads()
	placed.Request.Keepouts = []placement.Keepout{{
		ID: "wall",
		Bounds: placement.Rect{
			Min: placement.Point{XMM: 0, YMM: 0},
			Max: placement.Point{XMM: 30, YMM: 20},
		},
		Layers: []string{"F.Cu"},
	}}
	request := Request{
		Version:    RequestVersion,
		Name:       "blocked",
		Board:      BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1},
		Validation: ValidationSpec{StrictUnrouted: true},
	}

	result := RoutePlacement(context.Background(), request, PCBFragmentResult{}, placed, RoutingOptions{Mode: routing.ModeSingleLayer})
	if result.Stage.Status != StageStatusBlocked {
		t.Fatalf("stage = %#v, want blocked route", result.Stage)
	}
	if len(result.Stage.Issues) == 0 {
		t.Fatalf("expected routing issue")
	}
}

func TestRoutePlacementSkipsWhenRequested(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "status_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{SkipRouting: true},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := RoutePlacement(context.Background(), request, fragments, PlacementStageResult{}, RoutingOptions{})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
	if countTransactionOps(result.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want local route operation", result.Operations)
	}
}

func simplePlacedPads() PlacementStageResult {
	return PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 30, HeightMM: 20},
			Rules: placement.DefaultRules(),
			Components: []placement.Component{
				{
					Ref:         "U1",
					FootprintID: "Test:Pad",
					Bounds:      placement.Bounds{WidthMM: 2, HeightMM: 2, Source: placement.BoundsExplicit},
					Pads:        []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
				},
				{
					Ref:         "U2",
					FootprintID: "Test:Pad",
					Bounds:      placement.Bounds{WidthMM: 2, HeightMM: 2, Source: placement.BoundsExplicit},
					Pads:        []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
				},
			},
			Nets: []placement.Net{{
				Name:      "SIG",
				Role:      placement.NetSignal,
				Endpoints: []placement.Endpoint{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}},
			}},
		},
		Result: placement.Result{
			Status: placement.StatusPlaced,
			Placements: []placement.PlacementResult{
				{Ref: "U1", FootprintID: "Test:Pad", Position: placement.Placement{XMM: 5, YMM: 10, Layer: "F.Cu"}},
				{Ref: "U2", FootprintID: "Test:Pad", Position: placement.Placement{XMM: 20, YMM: 10, Layer: "F.Cu"}},
			},
			Metrics: placement.Metrics{PlacedCount: 2},
		},
		Stage: NewStageResult(StagePlacement, nil),
	}
}

func countTransactionOps(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
