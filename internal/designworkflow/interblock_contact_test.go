package designworkflow

import (
	"encoding/json"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
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

func TestValidateInterBlockRouteEndpointContactsProvesDirectHit(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header"},
			{Ref: "D1", Pin: "1", InstanceID: "status"},
		},
	}}
	operations := []transactions.Operation{mustContactRouteOperation(t, "SIG", "F.Cu",
		transactions.Point{XMM: 5, YMM: 10},
		transactions.Point{XMM: 15, YMM: 10},
	)}

	evidence := ValidateInterBlockRouteEndpointContacts(candidates, operations, &placed)
	if len(evidence.Issues) != 0 {
		t.Fatalf("issues = %#v", evidence.Issues)
	}
	summary := SummarizeInterBlockContacts(evidence)
	if summary.ContactsRequired != 2 || summary.ContactsProven != 2 || summary.ContactsFailed != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestValidateInterBlockRouteEndpointContactsProvesSegmentInteriorHit(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header"},
			{Ref: "D1", Pin: "1", InstanceID: "status"},
		},
	}}
	operations := []transactions.Operation{mustContactRouteOperation(t, "SIG", "F.Cu",
		transactions.Point{XMM: 0, YMM: 10},
		transactions.Point{XMM: 15, YMM: 10},
	)}

	evidence := ValidateInterBlockRouteEndpointContacts(candidates, operations, &placed)
	if len(evidence.Issues) != 0 {
		t.Fatalf("issues = %#v", evidence.Issues)
	}
	if len(evidence.Proofs) != 2 {
		t.Fatalf("proofs = %#v, want two endpoint proofs", evidence.Proofs)
	}
	var segmentProofs int
	for _, proof := range evidence.Proofs {
		if proof.Status != InterBlockContactProven {
			t.Fatalf("proof = %#v, want proven", proof)
		}
		if proof.EndpointSide == "segment" {
			segmentProofs++
		}
	}
	if segmentProofs != 1 {
		t.Fatalf("proofs = %#v, want one segment-interior contact proof", evidence.Proofs)
	}
}

func TestValidateInterBlockRouteEndpointContactsReportsMiss(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName:   "SIG",
		Status:    InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{{Ref: "J1", Pin: "1", InstanceID: "header"}},
	}}
	operations := []transactions.Operation{mustContactRouteOperation(t, "SIG", "F.Cu",
		transactions.Point{XMM: 6, YMM: 10},
		transactions.Point{XMM: 8, YMM: 10},
	)}

	evidence := ValidateInterBlockRouteEndpointContacts(candidates, operations, &placed)
	if len(evidence.Proofs) != 1 || evidence.Proofs[0].Status != InterBlockContactMiss {
		t.Fatalf("proofs = %#v, want miss", evidence.Proofs)
	}
	assertContactIssueCode(t, evidence.Issues, reports.CodeRouteContactMiss)
}

func TestValidateInterBlockRouteEndpointContactsClassifiesGraphSplit(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header"},
			{Ref: "D1", Pin: "1", InstanceID: "status"},
		},
	}}
	operations := []transactions.Operation{mustContactRouteOperation(t, "SIG", "F.Cu",
		transactions.Point{XMM: 5, YMM: 10},
		transactions.Point{XMM: 8, YMM: 10},
	)}

	evidence := ValidateInterBlockRouteEndpointContacts(candidates, operations, &placed)
	if len(evidence.Proofs) != 2 {
		t.Fatalf("proofs = %#v, want two endpoint proofs", evidence.Proofs)
	}
	var splitProofs int
	for _, proof := range evidence.Proofs {
		if proof.Status == InterBlockContactGraphSplit {
			splitProofs++
		}
	}
	if splitProofs != 1 {
		t.Fatalf("proofs = %#v, want one graph-split proof", evidence.Proofs)
	}
	assertContactIssueCode(t, evidence.Issues, reports.CodeRouteGraphIncomplete)
}

func TestValidateInterBlockRouteEndpointContactsReportsLayerMismatch(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName:   "SIG",
		Status:    InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{{Ref: "J1", Pin: "1", InstanceID: "header"}},
	}}
	operations := []transactions.Operation{mustContactRouteOperation(t, "SIG", "B.Cu",
		transactions.Point{XMM: 5, YMM: 10},
		transactions.Point{XMM: 8, YMM: 10},
	)}

	evidence := ValidateInterBlockRouteEndpointContacts(candidates, operations, &placed)
	if len(evidence.Proofs) != 1 || evidence.Proofs[0].Status != InterBlockContactLayerMismatch {
		t.Fatalf("proofs = %#v, want layer mismatch", evidence.Proofs)
	}
	assertContactIssueCode(t, evidence.Issues, reports.CodeRouteContactLayerMismatch)
}

func TestValidateInterBlockRouteEndpointContactsProvesViaConnectedOperations(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header"},
			{Ref: "D1", Pin: "1", InstanceID: "status"},
		},
	}}
	operations := []transactions.Operation{
		mustContactRouteOperationWithVias(t, "SIG", "B.Cu",
			[]transactions.Point{{XMM: 5, YMM: 10}, {XMM: 10, YMM: 10}},
			[]transactions.RouteViaSpec{{At: transactions.Point{XMM: 5, YMM: 10}, Layers: []string{"F.Cu", "B.Cu"}}},
		),
		mustContactRouteOperationWithVias(t, "SIG", "F.Cu",
			[]transactions.Point{{XMM: 10, YMM: 10}, {XMM: 15, YMM: 10}},
			[]transactions.RouteViaSpec{{At: transactions.Point{XMM: 10, YMM: 10}, Layers: []string{"F.Cu", "B.Cu"}}},
		),
	}

	evidence := ValidateInterBlockRouteEndpointContacts(candidates, operations, &placed)
	if len(evidence.Issues) != 0 {
		t.Fatalf("issues = %#v", evidence.Issues)
	}
	summary := SummarizeInterBlockContacts(evidence)
	if summary.ContactsRequired != 2 || summary.ContactsProven != 2 || summary.ContactsFailed != 0 {
		t.Fatalf("summary = %#v, want via-connected route contacts proven", summary)
	}
}

func TestValidateInterBlockRouteEndpointContactsReportsMissingRouteOperation(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName:   "SIG",
		Status:    InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{{Ref: "J1", Pin: "1", InstanceID: "header"}},
	}}

	evidence := ValidateInterBlockRouteEndpointContacts(candidates, nil, &placed)
	if len(evidence.Proofs) != 1 || evidence.Proofs[0].Status != InterBlockContactMissingTarget {
		t.Fatalf("proofs = %#v, want missing target", evidence.Proofs)
	}
	assertContactIssueCode(t, evidence.Issues, reports.CodeRouteContactMissingTarget)
}

func TestInterBlockConnectedNetsRequiresSameRouteGraph(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header"},
			{Ref: "D1", Pin: "1", InstanceID: "status"},
		},
	}}
	connectedOperation := mustContactRouteOperation(t, "SIG", "F.Cu",
		transactions.Point{XMM: 5, YMM: 10},
		transactions.Point{XMM: 15, YMM: 10},
	)
	evidence := ValidateInterBlockRouteEndpointContacts(candidates, []transactions.Operation{connectedOperation}, &placed)
	connected := interBlockConnectedNets(evidence, []transactions.Operation{connectedOperation})
	if !connected["SIG"] {
		t.Fatalf("connected nets = %#v, want SIG connected", connected)
	}

	disconnectedOperations := []transactions.Operation{
		mustContactRouteOperation(t, "SIG", "F.Cu", transactions.Point{XMM: 5, YMM: 10}, transactions.Point{XMM: 7, YMM: 10}),
		mustContactRouteOperation(t, "SIG", "F.Cu", transactions.Point{XMM: 13, YMM: 10}, transactions.Point{XMM: 15, YMM: 10}),
	}
	evidence = ValidateInterBlockRouteEndpointContacts(candidates, disconnectedOperations, &placed)
	connected = interBlockConnectedNets(evidence, disconnectedOperations)
	if connected["SIG"] {
		t.Fatalf("connected nets = %#v, want SIG disconnected", connected)
	}

	touchingOperations := []transactions.Operation{
		mustContactRouteOperation(t, "SIG", "F.Cu", transactions.Point{XMM: 5, YMM: 10}, transactions.Point{XMM: 10, YMM: 10}),
		mustContactRouteOperation(t, "SIG", "F.Cu", transactions.Point{XMM: 10, YMM: 10}, transactions.Point{XMM: 15, YMM: 10}),
	}
	evidence = ValidateInterBlockRouteEndpointContacts(candidates, touchingOperations, &placed)
	connected = interBlockConnectedNets(evidence, touchingOperations)
	if !connected["SIG"] {
		t.Fatalf("connected nets = %#v, want touching operations to connect SIG", connected)
	}
}

func TestInterBlockRouteCompletionUsesContactGraph(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header"},
			{Ref: "D1", Pin: "1", InstanceID: "status"},
		},
	}}
	operation := mustContactRouteOperation(t, "SIG", "F.Cu",
		transactions.Point{XMM: 5, YMM: 10},
		transactions.Point{XMM: 15, YMM: 10},
	)
	evidence := ValidateInterBlockRouteEndpointContacts(candidates, []transactions.Operation{operation}, &placed)

	summary := summarizeInterBlockRouteCompletion(candidates, []transactions.Operation{operation}, nil, evidence)
	if summary.RoutesCompleted != 1 || summary.PartialNets != 0 {
		t.Fatalf("summary = %#v, want connected route completion", summary)
	}
}

func TestInterBlockRouteCompletionIgnoresNonBlockingRouteTreeIssues(t *testing.T) {
	placed := interBlockContactPlaced("SIG", "SIG")
	candidates := []InterBlockRouteCandidate{{
		NetName: "SIG",
		Status:  InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "J1", Pin: "1", InstanceID: "header"},
			{Ref: "D1", Pin: "1", InstanceID: "status"},
		},
	}}
	operation := mustContactRouteOperation(t, "SIG", "F.Cu",
		transactions.Point{XMM: 5, YMM: 10},
		transactions.Point{XMM: 15, YMM: 10},
	)
	evidence := ValidateInterBlockRouteEndpointContacts(candidates, []transactions.Operation{operation}, &placed)
	issues := []reports.Issue{{
		Code:     reports.CodeFixedNetSkipped,
		Severity: reports.SeverityInfo,
		Nets:     []string{"SIG"},
		Message:  "fixed net preserved",
	}}

	summary := summarizeInterBlockRouteCompletion(candidates, []transactions.Operation{operation}, issues, evidence)
	if summary.RoutesCompleted != 1 || summary.PartialNets != 0 || summary.CompleteGroups != 1 {
		t.Fatalf("summary = %#v, want graph-complete route despite non-blocking issue", summary)
	}
	if summary.IssueCount != 1 {
		t.Fatalf("summary = %#v, want total issue diagnostics preserved", summary)
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

func mustContactRouteOperation(t *testing.T, netName string, layer string, points ...transactions.Point) transactions.Operation {
	t.Helper()
	return mustContactRouteOperationWithVias(t, netName, layer, points, nil)
}

func mustContactRouteOperationWithVias(t *testing.T, netName string, layer string, points []transactions.Point, vias []transactions.RouteViaSpec) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: netName,
		Layer:   layer,
		WidthMM: 0.25,
		Points:  points,
		Vias:    vias,
	})
	if err != nil {
		t.Fatal(err)
	}
	return transactions.NewOperation(transactions.OpRoute, raw)
}

func assertContactIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("issues = %#v, want code %s", issues, code)
}
