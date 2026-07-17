package libraryresolver

import (
	"reflect"
	"testing"
)

func TestGroupedPinMembers(t *testing.T) {
	tests := []struct {
		name string
		pin  string
		want []string
	}{
		{name: "plain", pin: "15", want: []string{"15"}},
		{name: "grouped", pin: "[1,15, 38,39]", want: []string{"1", "15", "38", "39"}},
		{name: "deduplicated", pin: "[1,1,2]", want: []string{"1", "2"}},
		{name: "empty", pin: "", want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := GroupedPinMembers(test.pin); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("GroupedPinMembers(%q) = %#v, want %#v", test.pin, got, test.want)
			}
		})
	}
}

func TestCanonicalSymbolPinNumberPrefersUnitAndPreservesGroup(t *testing.T) {
	record := SymbolRecord{Pins: []SymbolPin{
		{Number: "[1,15,38,39]", Unit: 0},
		{Number: "1", Unit: 2},
	}}
	if got, ok := CanonicalSymbolPinNumber(record, 1, "15"); !ok || got != "[1,15,38,39]" {
		t.Fatalf("canonical common grouped pin = %q/%v", got, ok)
	}
	if got, ok := CanonicalSymbolPinNumber(record, 2, "1"); !ok || got != "1" {
		t.Fatalf("canonical unit-specific pin = %q/%v", got, ok)
	}
}
