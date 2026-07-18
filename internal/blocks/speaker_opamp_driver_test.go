package blocks

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestSpeakerOpAmpDriverUsesLoadSideFeedback(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock(speakerOpAmpDriverID)
	if !ok {
		t.Fatal("missing speaker op-amp driver")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: speakerOpAmpDriverID, InstanceID: "speaker_gain"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if actual := output.Instance.Params["actual_gain"].(float64); math.Abs(actual-11) > 1e-9 {
		t.Fatalf("actual gain = %g", actual)
	}
	if len(output.Instance.Refs) != 6 || len(output.Instance.Nets) != 8 {
		t.Fatalf("refs/nets = %d/%d", len(output.Instance.Refs), len(output.Instance.Nets))
	}
	if validation := transactions.Validate(transactions.Transaction{Operations: output.Operations}); len(validation.Issues) != 0 {
		t.Fatalf("transaction issues = %#v", validation.Issues)
	}
	noConnectPins := []string{}
	for _, operation := range output.Operations {
		if operation.Op != transactions.OpAddNoConnect {
			continue
		}
		var payload transactions.AddNoConnectOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatal(err)
		}
		noConnectPins = append(noConnectPins, payload.Endpoint.Pin)
	}
	slices.Sort(noConnectPins)
	if !slices.Equal(noConnectPins, []string{"1", "8"}) {
		t.Fatalf("op-amp no-connect pins = %#v, want visible VOS pins 1 and 8; hidden library NC pin 5 needs no marker", noConnectPins)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 5, OriginYMM: 20})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("PCB realization issues = %#v", realized.Issues)
	}
	for _, route := range []string{"feedback_sense", "feedback_to_input", "gain_return", "driver_output"} {
		if !realizedRouteExists(realized, route) {
			t.Fatalf("missing route %s", route)
		}
	}
	if !slices.ContainsFunc(definition.PCBRealization.LocalRoutes, func(route PCBLocalRoute) bool {
		return route.ID == "feedback_sense" && len(route.Waypoints) == 2 && route.Waypoints[0].YMM == route.Waypoints[1].YMM
	}) {
		t.Fatal("load-side feedback route must retain its quiet-current dogleg")
	}
	if !slices.ContainsFunc(definition.PCBRealization.EntryAnchors, func(anchor PCBEntryAnchor) bool {
		return anchor.ID == "feedback_sense" && anchor.Placement.XMM == 17.0875 && anchor.Placement.YMM == -7
	}) {
		t.Fatal("load-side feedback handoff must originate at the fixed R_0805 pin-1 Kelvin point")
	}
	if !slices.ContainsFunc(definition.PCBRealization.EntryAnchors, func(anchor PCBEntryAnchor) bool {
		return anchor.ID == "signal_star" && anchor.Placement.XMM == 10.9125 && anchor.Placement.YMM == 8
	}) {
		t.Fatal("quiet-return handoff must originate at the fixed R_0805 pin-2 star point")
	}
}

func TestSpeakerOpAmpDriverFailsClosedOnInvalidGainOrIdentity(t *testing.T) {
	registry := NewBuiltinRegistry()
	for _, params := range []map[string]any{{"gain": 1.0}, {"opamp_component_id": ""}} {
		output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: speakerOpAmpDriverID, InstanceID: "bad", Params: params})
		if !reports.HasBlockingIssue(issues) || len(output.Operations) != 0 {
			t.Fatalf("params=%#v issues=%#v operations=%d", params, issues, len(output.Operations))
		}
	}
}

func TestSpeakerOpAmpDriverFailsClosedWhenRequiredRoleIsMissing(t *testing.T) {
	definition := newSpeakerOpAmpDriverDefinition()
	definition.Components = slices.DeleteFunc(definition.Components, func(component BlockComponent) bool {
		return component.Role == "opamp"
	})
	output := instantiateSpeakerOpAmpDriver(definition, BlockRequest{InstanceID: "missing_role"}, nil, nil)
	if !reports.HasBlockingIssue(output.Issues) {
		t.Fatalf("issues = %#v, want missing-role blocker", output.Issues)
	}
}
