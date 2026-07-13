package designworkflow

import (
	"reflect"
	"testing"

	"kicadai/internal/routing"
)

func TestExpandExplicitPhysicalPadEndpointsIncludesDuplicateSameNetPads(t *testing.T) {
	request := routing.Request{
		Components: []routing.Component{{
			Ref: "J1",
			Pads: []routing.Pad{
				{Name: "1", Net: "VBUS"},
				{Name: "SH", Net: "GND"},
				{Name: "SH", Net: "GND"},
				{Name: "SH", Net: "GND"},
				{Name: "SH", Net: "GND"},
			},
		}},
		Nets: []routing.Net{
			{Name: "VBUS", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}}},
			{Name: "GND", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "SH"}}},
		},
	}

	got := expandExplicitPhysicalPadEndpoints(request)
	wantPads := []string{"1", "SH", "SH#2", "SH#3", "SH#4"}
	padNames := make([]string, len(got.Components[0].Pads))
	for index, pad := range got.Components[0].Pads {
		padNames[index] = pad.Name
	}
	if !reflect.DeepEqual(padNames, wantPads) {
		t.Fatalf("routing pad names = %#v, want %#v", padNames, wantPads)
	}
	wantEndpoints := []routing.Endpoint{{Ref: "J1", Pin: "SH"}, {Ref: "J1", Pin: "SH#2"}, {Ref: "J1", Pin: "SH#3"}, {Ref: "J1", Pin: "SH#4"}}
	if !reflect.DeepEqual(got.Nets[1].Endpoints, wantEndpoints) {
		t.Fatalf("GND endpoints = %#v, want %#v", got.Nets[1].Endpoints, wantEndpoints)
	}
	if request.Components[0].Pads[2].Name != "SH" {
		t.Fatalf("input request mutated: %#v", request.Components[0].Pads)
	}
}

func TestExpandExplicitPhysicalPadEndpointsDoesNotAddUnrelatedComponent(t *testing.T) {
	request := routing.Request{
		Components: []routing.Component{
			{Ref: "J1", Pads: []routing.Pad{{Name: "1", Net: "GND"}}},
			{Ref: "MH1", Pads: []routing.Pad{{Name: "1", Net: "GND"}}},
		},
		Nets: []routing.Net{{Name: "GND", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}}}},
	}

	got := expandExplicitPhysicalPadEndpoints(request)
	if !reflect.DeepEqual(got.Nets[0].Endpoints, request.Nets[0].Endpoints) {
		t.Fatalf("endpoints = %#v, want unrelated component excluded", got.Nets[0].Endpoints)
	}
}
