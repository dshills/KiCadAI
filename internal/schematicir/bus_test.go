package schematicir

import (
	"reflect"
	"testing"
)

func TestValidateVectorBusLayout(t *testing.T) {
	document := validLEDDocument()
	document.Circuit.Buses = []Bus{{
		ID:   "data_bus",
		Name: "DATA[0..1]",
		Members: []BusMember{
			{Net: "LED_A", Label: "DATA1"},
			{Net: "VIN", Label: "DATA0"},
		},
	}}
	document.Layout.Buses = []BusLayout{{
		Bus:    "data_bus",
		Points: []LayoutPoint{{XMM: 50, YMM: 80}, {XMM: 100, YMM: 80}},
		Entries: []BusEntryLayout{
			{Member: "LED_A", At: LayoutPoint{XMM: 75, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
			{Member: "VIN", At: LayoutPoint{XMM: 60, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
		},
	}}

	if issues := Validate(document); len(issues) != 0 {
		t.Fatalf("valid vector bus produced issues: %#v", issues)
	}
	normalized := Normalize(document)
	if got := normalized.Circuit.Buses[0].Members[0].Net; got != "LED_A" {
		t.Fatalf("normalized bus members start with %q, want LED_A", got)
	}
	if got := normalized.Layout.Buses[0].Entries[0].Member; got != "LED_A" {
		t.Fatalf("normalized bus entries start with %q, want LED_A", got)
	}
}

func TestValidateVectorBusRejectsAmbiguousGeometry(t *testing.T) {
	document := validLEDDocument()
	document.Circuit.Buses = []Bus{{
		ID:      "data_bus",
		Name:    "DATA[0..1]",
		Members: []BusMember{{Net: "MISSING", Label: "DATA0"}},
	}}
	document.Layout.Buses = []BusLayout{{
		Bus:    "data_bus",
		Points: []LayoutPoint{{XMM: 50, YMM: 80}, {XMM: 60, YMM: 90}},
		Entries: []BusEntryLayout{{
			Member: "MISSING",
			At:     LayoutPoint{XMM: 55, YMM: 85},
			Size:   LayoutPoint{XMM: 0, YMM: 2.54},
		}},
	}}

	issues := Validate(document)
	for _, want := range []string{
		"circuit.buses[0].members[0].net",
		"layout.buses[0].points[1]",
		"layout.buses[0].entries[0].size",
		"layout.buses[0].entries[0].at",
	} {
		if !schematicIRIssueContains(issues, want) {
			t.Fatalf("missing issue at %s: %#v", want, issues)
		}
	}
}

func TestNormalizeVectorBusIsDeterministic(t *testing.T) {
	document := validLEDDocument()
	document.Circuit.Buses = []Bus{
		{ID: "z_bus", Name: "Z", Members: []BusMember{{Net: "VIN", Label: "Z"}}},
		{ID: "a_bus", Name: "A", Members: []BusMember{{Net: "LED_OUT", Label: "A"}}},
	}
	document.Layout.Buses = []BusLayout{
		{Bus: "z_bus", Points: []LayoutPoint{{XMM: 1, YMM: 2}, {XMM: 1, YMM: 4}}, Entries: []BusEntryLayout{{Member: "VIN", At: LayoutPoint{XMM: 1, YMM: 2}, Size: LayoutPoint{XMM: 1, YMM: 1}}}},
		{Bus: "a_bus", Points: []LayoutPoint{{XMM: 2, YMM: 2}, {XMM: 2, YMM: 4}}, Entries: []BusEntryLayout{{Member: "LED_OUT", At: LayoutPoint{XMM: 2, YMM: 2}, Size: LayoutPoint{XMM: 1, YMM: 1}}}},
	}
	left := Normalize(document)
	right := Normalize(document)
	if !reflect.DeepEqual(left.Circuit.Buses, right.Circuit.Buses) || !reflect.DeepEqual(left.Layout.Buses, right.Layout.Buses) {
		t.Fatalf("normalization is not deterministic: left=%#v right=%#v", left, right)
	}
}
