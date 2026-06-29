package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func TestAdaptSchematicPreservesSymbolsAndWires(t *testing.T) {
	file := &schematic.SchematicFile{
		Paper: kicadfiles.Paper{Name: "A4"},
		Symbols: []schematic.SchematicSymbol{{
			Reference:  "R1",
			Value:      "10k",
			LibraryID:  "Device:R",
			Position:   kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(30)},
			Pins:       []schematic.SymbolPin{{Number: "1"}},
			PinAnchors: []kicadfiles.Point{{X: kicadfiles.MM(17.46), Y: kicadfiles.MM(30)}},
		}},
		Wires: []schematic.Wire{{Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		}}},
	}
	request, result := AdaptSchematic(file)
	if len(request.Components) != 1 || len(result.Components) != 1 {
		t.Fatalf("components request=%#v result=%#v", request.Components, result.Components)
	}
	if result.Components[0].Ref != "R1" || result.Components[0].PlacedAt != file.Symbols[0].Position {
		t.Fatalf("component not preserved: %#v", result.Components[0])
	}
	if len(result.Wires) != 1 || result.Wires[0].From != file.Wires[0].Points[0] || result.Wires[0].To != file.Wires[0].Points[1] {
		t.Fatalf("wires not preserved: %#v", result.Wires)
	}
	if len(result.Components[0].Pins) != 1 || result.Components[0].Pins[0].At.X == 0 {
		t.Fatalf("pin anchors not made relative: %#v", result.Components[0].Pins)
	}
}

func TestAdaptSchematicHandlesMissingPinAnchors(t *testing.T) {
	file := &schematic.SchematicFile{Symbols: []schematic.SchematicSymbol{{Reference: "U1", Value: "MCU", LibraryID: "MCU:Generic", Position: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}, Pins: []schematic.SymbolPin{{Number: "1"}}}}}
	_, result := AdaptSchematic(file)
	if len(result.Components) != 1 || len(result.Components[0].Pins) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Components[0].Pins[0].At != (kicadfiles.Point{}) {
		t.Fatalf("missing anchor should map to zero relative point: %#v", result.Components[0].Pins[0])
	}
}

func TestAdaptSchematicPreservesLabelsAndJunctions(t *testing.T) {
	file := &schematic.SchematicFile{
		Labels:    []schematic.Label{{Text: "SIG", Position: kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(30)}}},
		Junctions: []schematic.Junction{{Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(30)}}},
	}
	_, result := AdaptSchematic(file)
	if len(result.Labels) != 1 || result.Labels[0].Text != "SIG" {
		t.Fatalf("labels = %#v", result.Labels)
	}
	if len(result.Junctions) != 1 {
		t.Fatalf("junctions = %#v", result.Junctions)
	}
}
