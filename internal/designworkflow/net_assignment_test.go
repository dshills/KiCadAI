package designworkflow

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestBuildGeneratedNetTableAssignsDeterministicCodes(t *testing.T) {
	placed := PlacementStageResult{Request: placement.Request{
		Nets: []placement.Net{
			{Name: "LED_A", Endpoints: []placement.Endpoint{{Ref: "D1", Pin: "2"}}},
			{Name: "GND", Endpoints: []placement.Endpoint{{Ref: "D1", Pin: "1"}}},
			{Name: "VCC", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "1"}}},
		},
		Components: []placement.Component{
			{Ref: "D1", Pads: []placement.PadSummary{{Name: "1", Net: "GND"}, {Name: "2", Net: "LED_A"}}},
		},
	}}
	routed := RoutingStageResult{Request: routing.Request{
		Nets: []routing.Net{{Name: "VCC"}, {Name: "GND"}},
	}}

	table, issues := BuildGeneratedNetTable(&placed, &routed)
	if len(issues) != 0 {
		t.Fatalf("BuildGeneratedNetTable issues = %#v", issues)
	}
	got := generatedNetTablePairs(table)
	want := []string{"GND=1", "LED_A=2", "VCC=3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generated net table = %#v, want %#v", got, want)
	}
	if code, ok := generatedNetCode(table, "LED_A"); !ok || code != 2 {
		t.Fatalf("generatedNetCode(LED_A) = %d, %v; want 2, true", code, ok)
	}
	if _, ok := generatedNetCode(table, ""); ok {
		t.Fatalf("empty no-net unexpectedly resolved")
	}
}

func TestBuildGeneratedNetTableCollectsRouteOperations(t *testing.T) {
	operation := mustGeneratedNetAssignmentRouteOperation(t, "SDA")
	routed := RoutingStageResult{Operations: []transactions.Operation{operation}}
	table, issues := BuildGeneratedNetTable(nil, &routed)
	if len(issues) != 0 {
		t.Fatalf("BuildGeneratedNetTable issues = %#v", issues)
	}
	if got := generatedNetTablePairs(table); !reflect.DeepEqual(got, []string{"SDA=1"}) {
		t.Fatalf("generated net table = %#v", got)
	}
	if len(table.Nets[0].Sources) != 1 || table.Nets[0].Sources[0] != GeneratedNetSourceRouteOp {
		t.Fatalf("sources = %#v, want route operation", table.Nets[0].Sources)
	}
}

func TestBuildGeneratedNetTableRejectsReservedZeroName(t *testing.T) {
	placed := PlacementStageResult{Request: placement.Request{Nets: []placement.Net{{Name: "0"}}}}
	table, issues := BuildGeneratedNetTable(&placed, nil)
	if len(table.Nets) != 0 {
		t.Fatalf("reserved no-net name should not be assigned: %#v", table.Nets)
	}
	if len(issues) != 1 || issues[0].Path != "generated_net_assignment.placement.nets[0]" {
		t.Fatalf("issues = %#v, want reserved no-net issue", issues)
	}
}

func TestGeneratedNetNameHelpersSortAndTrim(t *testing.T) {
	placed := placementNetNames([]placement.Net{{Name: " VCC "}, {Name: ""}, {Name: "GND"}, {Name: "VCC"}})
	if !reflect.DeepEqual(placed, []string{"GND", "VCC"}) {
		t.Fatalf("placement names = %#v", placed)
	}
	routed := routingNetNames([]routing.Net{{Name: " SDA "}, {Name: ""}, {Name: "SCL"}})
	if !reflect.DeepEqual(routed, []string{"SCL", "SDA"}) {
		t.Fatalf("routing names = %#v", routed)
	}
}

func generatedNetTablePairs(table GeneratedNetTable) []string {
	pairs := make([]string, 0, len(table.Nets))
	for _, net := range table.Nets {
		pairs = append(pairs, net.Name+"="+strconv.Itoa(net.Code))
	}
	return pairs
}

func mustGeneratedNetAssignmentRouteOperation(t *testing.T, netName string) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: netName,
		Points:  []transactions.Point{{XMM: 1, YMM: 2}, {XMM: 3, YMM: 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return transactions.NewOperation(transactions.OpRoute, raw)
}
