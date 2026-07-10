package transactions

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
)

func TestResolveSymbolPinsPrefersEmbeddedTemplateGeometry(t *testing.T) {
	pins, err := resolveSymbolPins([]PinSpec{
		{Number: "1", XMM: -5.08, YMM: 0},
		{Number: "2", XMM: 5.08, YMM: 0},
	}, nil, "Device:R")
	if err != nil {
		t.Fatalf("resolveSymbolPins returned error: %v", err)
	}
	if len(pins) != 2 {
		t.Fatalf("pins = %#v, want two embedded template pins", pins)
	}
	if pins[0].Number != "1" || pins[0].XMM != 0 || pins[0].YMM != 3.81 {
		t.Fatalf("pin 1 = %#v, want Device:R embedded vertical pin", pins[0])
	}
	if pins[1].Number != "2" || pins[1].XMM != 0 || pins[1].YMM != -3.81 {
		t.Fatalf("pin 2 = %#v, want Device:R embedded vertical pin", pins[1])
	}
}

func TestResolveSymbolPinsPreservesCallerOnlyPins(t *testing.T) {
	pins, err := resolveSymbolPins([]PinSpec{
		{Number: "1", XMM: -5.08, YMM: 0},
		{Number: "99", XMM: 12.7, YMM: 1.27},
	}, nil, "Device:R")
	if err != nil {
		t.Fatalf("resolveSymbolPins returned error: %v", err)
	}
	if len(pins) != 2 {
		t.Fatalf("pins = %#v, want caller pin count preserved", pins)
	}
	if pins[0].Number != "1" || pins[0].XMM != 0 || pins[0].YMM != 3.81 {
		t.Fatalf("pin 1 = %#v, want embedded offset applied", pins[0])
	}
	if pins[1].Number != "99" || pins[1].XMM != 12.7 || pins[1].YMM != 1.27 {
		t.Fatalf("pin 99 = %#v, want caller-only pin preserved", pins[1])
	}
}

func TestResolveSymbolPinsPreservesExplicitOriginOffset(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Custom:Origin": {
			LibraryID: "Custom:Origin",
			Pins:      []libraryresolver.SymbolPin{{Number: "1", Position: kicadfiles.Point{X: kicadfiles.MM(2.54)}}},
		},
	}}
	pins, err := resolveSymbolPins([]PinSpec{{Number: "1", ExplicitOffset: true}}, &index, "Custom:Origin")
	if err != nil {
		t.Fatalf("resolveSymbolPins returned error: %v", err)
	}
	if len(pins) != 1 || pins[0].XMM != 0 || pins[0].YMM != 0 || !pins[0].ExplicitOffset {
		t.Fatalf("pin = %#v, want explicit origin offset preserved", pins)
	}
}

func TestResolveSymbolPinsExpandsDistinctMultiUnitPins(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"MultiUnit:Distinct": {
			LibraryID: "MultiUnit:Distinct",
			Pins: []libraryresolver.SymbolPin{
				{Number: "1", Unit: 1},
				{Number: "2", Unit: 1},
				{Number: "3", Unit: 2},
				{Number: "4", Unit: 2},
			},
		},
	}}
	pins, err := resolveSymbolPinsForUnit([]PinSpec{{Number: "1"}}, &index, "MultiUnit:Distinct", 1)
	if err != nil {
		t.Fatalf("resolveSymbolPinsForUnit returned error: %v", err)
	}
	if len(pins) != 4 {
		t.Fatalf("pins = %#v, want all distinct physical pins", pins)
	}
	for index, want := range []string{"1", "2", "3", "4"} {
		if pins[index].Number != want {
			t.Fatalf("pin[%d] = %#v, want number %s", index, pins[index], want)
		}
	}
}
