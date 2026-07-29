package designworkflow

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestAutonomousCorrectionDiagnosticsCorrelateAllAffectedNetOperations(t *testing.T) {
	operations := []transactions.Operation{
		correctionRouteOperation(t, "ALPHA", 7, 1, 1, 8, 1),
		correctionRouteOperation(t, "BETA", 8, 4, 0, 4, 5),
		correctionRouteOperation(t, "DECOY", 9, 1, 8, 8, 8),
		correctionRouteOperation(t, "ALPHA", 10, 8, 1, 9, 2),
	}
	issue := reports.Issue{
		Code:        reports.CodeValidationFailed,
		Severity:    reports.SeverityBlocked,
		Path:        "routing.clearance[0]",
		Message:     "track clearance conflict",
		Nets:        []string{"BETA", "ALPHA"},
		OperationID: "route:7",
	}
	routed := RoutingStageResult{Operations: operations, Stage: StageResult{Issues: []reports.Issue{issue}}}

	first := BuildAutonomousCorrectionDiagnosticsForRouting(nil, routed)
	second := BuildAutonomousCorrectionDiagnosticsForRouting(nil, routed)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("operation correlation is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first) != 1 {
		t.Fatalf("diagnostics = %#v", first)
	}
	got := first[0]
	if got.Category != CorrectionForeignNetCrossing {
		t.Fatalf("category = %q, want %q", got.Category, CorrectionForeignNetCrossing)
	}
	if !reflect.DeepEqual(got.OperationIndexes, []int{0, 1, 3}) {
		t.Fatalf("operation indexes = %#v, want affected ALPHA/BETA operations only", got.OperationIndexes)
	}
	if len(got.OperationIDs) != 3 || got.OperationScope != correctionOperationScopeIdentity+"+"+correctionOperationScopeNet {
		t.Fatalf("operation scope = %#v", got)
	}
}

func TestAutonomousCorrectionDiagnosticsCorrelateExactPathWithoutNet(t *testing.T) {
	operations := []transactions.Operation{
		correctionRouteOperation(t, "ALPHA", 0, 1, 1, 8, 1),
		correctionRouteOperation(t, "BETA", 1, 4, 0, 4, 5),
	}
	issue := reports.Issue{
		Code:     reports.CodeRouteContactLayerMismatch,
		Severity: reports.SeverityBlocked,
		Path:     "operations[1].vias[0]",
		Message:  "routing layer is not available",
	}
	routed := RoutingStageResult{Operations: operations, Stage: StageResult{Issues: []reports.Issue{issue}}}
	diagnostics := BuildAutonomousCorrectionDiagnosticsForRouting(nil, routed)
	if len(diagnostics) != 1 || !reflect.DeepEqual(diagnostics[0].OperationIndexes, []int{1}) || diagnostics[0].OperationScope != correctionOperationScopePath+"+"+correctionOperationScopeNet || !reflect.DeepEqual(diagnostics[0].Nets, []string{"BETA"}) {
		t.Fatalf("path correlation = %#v", diagnostics)
	}
}

func TestAutonomousCorrectionDiagnosticsFailClosedOnIncompleteOrAmbiguousScope(t *testing.T) {
	operations := []transactions.Operation{
		correctionRouteOperation(t, "ALPHA", 0, 1, 1, 8, 1),
		correctionRouteOperation(t, "BETA", 0, 4, 0, 4, 5),
	}
	tests := []struct {
		name  string
		issue reports.Issue
		want  string
	}{
		{
			name:  "missing named net",
			issue: reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: "track clearance conflict", Nets: []string{"ALPHA", "MISSING"}},
			want:  correctionOperationScopeMissing,
		},
		{
			name:  "ambiguous legacy identity",
			issue: reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: "no legal route", Nets: []string{"ALPHA"}, OperationID: "route:0"},
			want:  correctionOperationScopeAmbiguous,
		},
		{
			name:  "identity net conflict",
			issue: reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: "no legal route", Nets: []string{"BETA"}, OperationID: autonomousCorrectionRouteOperationTraces(operations)[0].ID},
			want:  correctionOperationScopeAmbiguous,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			routed := RoutingStageResult{Operations: operations, Stage: StageResult{Issues: []reports.Issue{test.issue}}}
			diagnostics := BuildAutonomousCorrectionDiagnosticsForRouting(nil, routed)
			if len(diagnostics) != 1 || diagnostics[0].OperationScope != test.want || len(diagnostics[0].OperationIndexes) != 0 || len(diagnostics[0].OperationIDs) != 0 {
				t.Fatalf("diagnostic = %#v, want fail-closed scope %q", diagnostics, test.want)
			}
		})
	}
}

func TestAutonomousCorrectionRouteOperationIdentityCanonicalizesJSON(t *testing.T) {
	left := transactions.NewOperationWithMetadata(transactions.OpRoute, json.RawMessage(`{"op":"route","net_name":"SIG","layer":"F.Cu","width_mm":0.25,"points":[{"x_mm":1,"y_mm":2},{"x_mm":3,"y_mm":4}]}`), "", "SIG")
	right := transactions.NewOperationWithMetadata(transactions.OpRoute, json.RawMessage(`{ "points" : [ { "y_mm":2, "x_mm":1 }, { "y_mm":4, "x_mm":3 } ], "width_mm":0.25, "layer":"F.Cu", "net_name":"SIG", "op":"route" }`), "", "SIG")
	if leftID, rightID := autonomousCorrectionRouteOperationID(left), autonomousCorrectionRouteOperationID(right); leftID != rightID {
		t.Fatalf("canonical identities differ: %q != %q", leftID, rightID)
	}
}

func TestAutonomousCorrectionRetryKeyIncludesOperationScope(t *testing.T) {
	base := AutonomousCorrectionDiagnostic{
		Category: CorrectionForeignNetCrossing, Source: "routing",
		IssueCode: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
		Nets: []string{"ALPHA", "BETA"}, OperationIDs: []string{"route-a"}, OperationIndexes: []int{0},
		OperationScope: correctionOperationScopeNet,
	}
	changed := base
	changed.OperationIDs = []string{"route-b"}
	first := AutonomousCorrectionRetryKey([]AutonomousCorrectionDiagnostic{base}, []string{"rebuild_route_tree"}, "invariant", "state")
	second := AutonomousCorrectionRetryKey([]AutonomousCorrectionDiagnostic{changed}, []string{"rebuild_route_tree"}, "invariant", "state")
	if first == second {
		t.Fatal("operation scope did not change retry key")
	}
}

func TestAutonomousCorrectionPlacementAffectedNetsExpandsFromMovedPads(t *testing.T) {
	plan := AutonomousCorrectionPlan{Actions: []AutonomousCorrectionAction{{
		Kind: CorrectionActionImproveEndpointFanout, Nets: []string{"SIGNAL"},
	}}}
	request := routing.Request{Nets: []routing.Net{
		{Name: "SIGNAL", Endpoints: []routing.Endpoint{{Ref: "U1", Pin: "1"}, {Ref: "R1", Pin: "1"}}},
		{Name: "SUPPLY", Endpoints: []routing.Endpoint{{Ref: "U1", Pin: "2"}, {Ref: "J1", Pin: "1"}}},
		{Name: "DECOY", Endpoints: []routing.Endpoint{{Ref: "D1", Pin: "1"}, {Ref: "D2", Pin: "1"}}},
	}}
	before := []placement.PlacementResult{
		{Ref: "U1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "R1", Position: placement.Placement{XMM: 8, YMM: 5, Layer: "F.Cu"}},
	}
	after := append([]placement.PlacementResult(nil), before...)
	after[0].Position.XMM = 6
	got := autonomousCorrectionPlacementAffectedNets(plan, request, before, after)
	if want := []string{"SIGNAL", "SUPPLY"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("placement-affected nets = %#v, want %#v", got, want)
	}
}

func TestApplyAutonomousRoutingCorrectionPlanReroutesOnlyAffectedNets(t *testing.T) {
	operations := []transactions.Operation{
		correctionRouteOperation(t, "ALPHA", 0, 2, 10, 18, 10),
		correctionRouteOperation(t, "BETA", 1, 10, 4, 10, 18),
		correctionRouteOperation(t, "DECOY", 2, 2, 2, 18, 2),
	}
	routingRequest := correctionSelectiveRoutingRequest()
	crossingIssues := routing.ValidatePhysicalClearance(routingRequest, routingRoutesFromOperations(operations))
	if len(crossingIssues) == 0 || crossingIssues[0].Code != reports.CodeRouteCopperConflict || !reflect.DeepEqual(crossingIssues[0].Nets, []string{"ALPHA", "BETA"}) {
		t.Fatalf("initial crossing was not normalized: %#v", crossingIssues)
	}
	crossing := crossingIssues[0]
	crossing.Path = "routing.clearance[0]"
	current := RoutingStageResult{
		Request: routingRequest,
		Result: routing.Result{
			Status:  routing.StatusBlocked,
			Metrics: routing.Metrics{NetCount: 3, RoutedNetCount: 1, FailedNetCount: 2},
			Issues:  []reports.Issue{crossing},
		},
		Operations: operations,
		Stage:      NewStageResult(StageRouting, []reports.Issue{crossing}),
	}
	diagnostics := BuildAutonomousCorrectionDiagnosticsForRouting(nil, current)
	if len(diagnostics) != 1 || !diagnostics[0].AutomaticAction {
		t.Fatalf("crossing diagnostic = %#v", diagnostics)
	}
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, diagnostics, AutonomousCorrectionPlanOptions{
		Attempt: 2, MaxAttempts: 3, RouteOperations: current.Operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Actions) != 1 || plan.Actions[0].Kind != CorrectionActionRerouteAffectedNets || plan.RouteStateHash == "" {
		t.Fatalf("selective plan = %#v", plan)
	}

	first, firstApplication, err := ApplyAutonomousRoutingCorrectionPlan(context.Background(), request, current, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, secondApplication, err := ApplyAutonomousRoutingCorrectionPlan(context.Background(), request, current, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !firstApplication.Applied || !firstApplication.ProtectedInvariantsPreserved || first.Result.Status != routing.StatusRouted {
		t.Fatalf("selective application = %#v result=%#v", firstApplication, first.Result)
	}
	if !reflect.DeepEqual(first.Operations, second.Operations) || !reflect.DeepEqual(firstApplication, secondApplication) {
		t.Fatal("selective routing replay is not byte-identical")
	}
	if firstApplication.ReplacedOperationCount != 2 || firstApplication.PreservedOperationCount != 1 {
		t.Fatalf("replacement counts = %#v", firstApplication)
	}
	preserved := autonomousCorrectionPreservedOperations(first.Operations, map[string]struct{}{"ALPHA": {}, "BETA": {}})
	if len(preserved) != 1 || !reflect.DeepEqual(preserved[0], operations[2]) {
		t.Fatalf("decoy operation changed:\nwant=%#v\ngot=%#v", operations[2], preserved)
	}
	if issues := routing.ValidatePhysicalClearance(first.Request, routingRoutesFromOperations(first.Operations)); reports.HasBlockingIssue(issues) {
		t.Fatalf("selective replacement is not clearance clean: %#v", issues)
	}
}

func TestPlanAutonomousRoutingCorrectionRejectsStaleOperationScope(t *testing.T) {
	operations := []transactions.Operation{
		correctionRouteOperation(t, "ALPHA", 0, 2, 10, 18, 10),
		correctionRouteOperation(t, "BETA", 1, 10, 4, 10, 18),
	}
	current := RoutingStageResult{
		Operations: operations,
		Stage: StageResult{Issues: []reports.Issue{{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
			Message: "track clearance conflict", Nets: []string{"ALPHA", "BETA"},
		}}},
	}
	diagnostics := BuildAutonomousCorrectionDiagnosticsForRouting(nil, current)
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	stale := append([]transactions.Operation(nil), operations...)
	stale[0] = correctionRouteOperation(t, "ALPHA", 0, 2, 11, 18, 11)
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, diagnostics, AutonomousCorrectionPlanOptions{
		Attempt: 2, MaxAttempts: 3, RouteOperations: stale,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || plan.StopReason != CorrectionStopRouteOperationScopeMismatch {
		t.Fatalf("stale operation scope plan = %#v", plan)
	}
}

func TestApplyAutonomousRoutingCorrectionPlanInsertsProvenLayerTransition(t *testing.T) {
	operations := []transactions.Operation{
		correctionLayerRouteOperation(t, "SIGNAL", 0, "F.Cu", 2, 10, 10, 10),
		correctionLayerRouteOperation(t, "SIGNAL", 1, "B.Cu", 10, 10, 18, 10),
		correctionLayerRouteOperation(t, "DECOY", 2, "F.Cu", 2, 3, 18, 3),
	}
	layerIssue := reports.Issue{
		Code: reports.CodeRouteContactLayerMismatch, Severity: reports.SeverityBlocked,
		Path: "operations[0]", Message: "routing layer is not available", Nets: []string{"SIGNAL"},
	}
	routingRequest := correctionLayerTransitionRequest()
	current := RoutingStageResult{
		Request:    routingRequest,
		Result:     routing.Result{Status: routing.StatusBlocked, Metrics: routing.Metrics{NetCount: 2, RoutedNetCount: 1, FailedNetCount: 1}},
		Operations: operations,
		Stage:      NewStageResult(StageRouting, []reports.Issue{layerIssue}),
	}
	diagnostics := BuildAutonomousCorrectionDiagnosticsForRouting(nil, current)
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, diagnostics, AutonomousCorrectionPlanOptions{
		Attempt: 2, MaxAttempts: 3, RouteOperations: current.Operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Actions) != 1 || plan.Actions[0].Kind != CorrectionActionInsertLayerTransition {
		t.Fatalf("transition plan = %#v", plan)
	}
	corrected, application, err := ApplyAutonomousRoutingCorrectionPlan(context.Background(), request, current, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !application.Applied || corrected.Result.Status != routing.StatusRouted {
		t.Fatalf("transition application = %#v result=%#v", application, corrected.Result)
	}
	summary, ok := corrected.Stage.Summary["selective_route_correction"].(map[string]any)
	if !ok || summary["direct_layer_transition"] != true {
		t.Fatalf("direct transition evidence = %#v", corrected.Stage.Summary)
	}
	decoded := decodeRouteOperations(corrected.Operations)
	viaCount := 0
	for _, operation := range decoded {
		if operation.payload.NetName == "SIGNAL" {
			viaCount += len(operation.payload.Vias)
		}
	}
	if viaCount != 1 {
		t.Fatalf("inserted via count = %d, operations=%#v", viaCount, decoded)
	}
	preserved := autonomousCorrectionPreservedOperations(corrected.Operations, map[string]struct{}{"SIGNAL": {}})
	if len(preserved) != 1 || !reflect.DeepEqual(preserved[0], operations[2]) {
		t.Fatalf("transition changed decoy route: %#v", preserved)
	}
}

func TestApplyAutonomousRoutingCorrectionPlanRejectsIllegalLayerTransitionWithoutMutation(t *testing.T) {
	operations := []transactions.Operation{
		correctionLayerRouteOperation(t, "SIGNAL", 0, "F.Cu", 2, 10, 10, 10),
		correctionLayerRouteOperation(t, "SIGNAL", 1, "B.Cu", 10, 10, 18, 10),
	}
	layerIssue := reports.Issue{
		Code: reports.CodeRouteContactLayerMismatch, Severity: reports.SeverityBlocked,
		Path: "operations[0]", Message: "routing layer is not available", Nets: []string{"SIGNAL"},
	}
	routingRequest := correctionLayerTransitionRequest()
	deny := false
	routingRequest.Rules.AllowVias = &deny
	routingRequest.Rules.MaxViasPerNet = 0
	current := RoutingStageResult{
		Request:    routingRequest,
		Result:     routing.Result{Status: routing.StatusBlocked, Metrics: routing.Metrics{NetCount: 2, FailedNetCount: 1}},
		Operations: operations,
		Stage:      NewStageResult(StageRouting, []reports.Issue{layerIssue}),
	}
	diagnostics := BuildAutonomousCorrectionDiagnosticsForRouting(nil, current)
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, diagnostics, AutonomousCorrectionPlanOptions{
		Attempt: 2, MaxAttempts: 3, RouteOperations: current.Operations,
	})
	if err != nil || !plan.Authorized {
		t.Fatalf("illegal transition should reach guarded application: plan=%#v err=%v", plan, err)
	}
	corrected, application, err := ApplyAutonomousRoutingCorrectionPlan(context.Background(), request, current, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if application.Applied || application.StopReason != CorrectionStopRouteReplacementInvalid {
		t.Fatalf("illegal transition application = %#v", application)
	}
	if !reflect.DeepEqual(corrected.Operations, current.Operations) {
		t.Fatal("illegal transition mutated route operations")
	}
}

func correctionSelectiveRoutingRequest() routing.Request {
	allow := true
	return routing.Request{
		Board: routing.Board{
			WidthMM: 20, HeightMM: 20, MarginMM: 1,
			Layers: []routing.Layer{
				{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
				{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
			},
		},
		Components: []routing.Component{
			correctionRoutingEndpointComponent("A1", "ALPHA", 2, 10),
			correctionRoutingEndpointComponent("A2", "ALPHA", 18, 10),
			correctionRoutingEndpointComponent("B1", "BETA", 10, 4),
			correctionRoutingEndpointComponent("B2", "BETA", 10, 18),
			correctionRoutingEndpointComponent("D1", "DECOY", 2, 2),
			correctionRoutingEndpointComponent("D2", "DECOY", 18, 2),
		},
		Nets: []routing.Net{
			{Name: "ALPHA", Endpoints: []routing.Endpoint{{Ref: "A1", Pin: "1"}, {Ref: "A2", Pin: "1"}}},
			{Name: "BETA", Endpoints: []routing.Endpoint{{Ref: "B1", Pin: "1"}, {Ref: "B2", Pin: "1"}}},
			{Name: "DECOY", Endpoints: []routing.Endpoint{{Ref: "D1", Pin: "1"}, {Ref: "D2", Pin: "1"}}},
		},
		Rules: routing.Rules{
			GridMM: 0.5, TraceWidthMM: 0.25, ClearanceMM: 0.25,
			ViaDiameterMM: 0.7, ViaDrillMM: 0.3, ViaClearanceMM: 0.25,
			MaxSearchNodes: 200000, MaxViasPerNet: 2, AllowVias: &allow, AllowBackLayer: &allow,
		},
		Strategy: routing.Strategy{Mode: routing.ModeTwoLayer, NetOrder: routing.NetOrderConstrainedEndpointAccessV1},
	}
}

func correctionLayerTransitionRequest() routing.Request {
	allow := true
	return routing.Request{
		Board: routing.Board{
			WidthMM: 20, HeightMM: 20, MarginMM: 1,
			Layers: []routing.Layer{
				{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
				{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
			},
		},
		Components: []routing.Component{
			correctionRoutingEndpointComponentOnLayer("S1", "SIGNAL", 2, 10, "F.Cu"),
			correctionRoutingEndpointComponentOnLayer("S2", "SIGNAL", 18, 10, "B.Cu"),
			correctionRoutingEndpointComponent("D1", "DECOY", 2, 3),
			correctionRoutingEndpointComponent("D2", "DECOY", 18, 3),
		},
		Nets: []routing.Net{
			{Name: "SIGNAL", Endpoints: []routing.Endpoint{{Ref: "S1", Pin: "1"}, {Ref: "S2", Pin: "1"}}},
			{Name: "DECOY", Endpoints: []routing.Endpoint{{Ref: "D1", Pin: "1"}, {Ref: "D2", Pin: "1"}}},
		},
		Rules: routing.Rules{
			GridMM: 0.5, TraceWidthMM: 0.25, ClearanceMM: 0.25,
			ViaDiameterMM: 0.7, ViaDrillMM: 0.3, ViaClearanceMM: 0.25,
			MaxSearchNodes: 200000, MaxViasPerNet: 2, AllowVias: &allow, AllowBackLayer: &allow,
		},
		Strategy: routing.Strategy{Mode: routing.ModeTwoLayer, NetOrder: routing.NetOrderConstrainedEndpointAccessV1},
	}
}

func correctionRoutingEndpointComponent(ref, net string, x, y float64) routing.Component {
	return correctionRoutingEndpointComponentOnLayer(ref, net, x, y, "F.Cu")
}

func correctionRoutingEndpointComponentOnLayer(ref, net string, x, y float64, layer string) routing.Component {
	return routing.Component{
		Ref: ref, Position: routing.Placement{XMM: x, YMM: y, Layer: layer},
		Pads: []routing.Pad{{
			Name: "1", Net: net, Position: routing.Point{}, Shape: routing.PadCircle, Type: routing.PadSMD,
			Size: routing.Size{WidthMM: 1, HeightMM: 1}, Layers: []string{layer},
		}},
	}
}

func correctionRouteOperation(t *testing.T, net string, operationIndex int, x1, y1, x2, y2 float64) transactions.Operation {
	return correctionLayerRouteOperation(t, net, operationIndex, "F.Cu", x1, y1, x2, y2)
}

func correctionLayerRouteOperation(t *testing.T, net string, operationIndex int, layer string, x1, y1, x2, y2 float64) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"op": "route", "net_name": net, "layer": layer, "width_mm": 0.25,
		"points": []map[string]float64{{"x_mm": x1, "y_mm": y1}, {"x_mm": x2, "y_mm": y2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	operation := transactions.NewOperationWithMetadata(transactions.OpRoute, raw, "", net)
	operation.Index = operationIndex
	return operation
}
