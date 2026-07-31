package schematiclayout

import "testing"

func TestNormalizeRequestMergesSameNameNetFragments(t *testing.T) {
	request := NormalizeRequest(Request{Nets: []Net{
		{Name: "SENSE", Role: "signal", Endpoints: []Endpoint{{Ref: "Q1", Pin: "3"}, {Ref: "R1", Pin: "1"}}},
		{Name: "SENSE", Role: "feedback", PreferredLabels: true, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "U1", Pin: "2"}}},
	}})
	if len(request.Nets) != 1 {
		t.Fatalf("nets = %#v, want one hyperedge", request.Nets)
	}
	net := request.Nets[0]
	if net.Role != "feedback" || !net.PreferredLabels || len(net.Endpoints) != 3 {
		t.Fatalf("merged net = %#v", net)
	}
}
