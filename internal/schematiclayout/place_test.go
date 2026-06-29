package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestPlaceSignalFlowLeftToRight(t *testing.T) {
	result := Place(Request{
		Sheet: testSheet(),
		Components: []Component{
			{Ref: "J1", Role: "input_connector"},
			{Ref: "U1", Role: "opamp"},
			{Ref: "R1", Role: "output_resistor"},
			{Ref: "J2", Role: "output_connector"},
		},
	})
	positions := placedPositions(result.Components)
	if !(positions["J1"].X < positions["U1"].X && positions["U1"].X < positions["R1"].X && positions["R1"].X < positions["J2"].X) {
		t.Fatalf("positions = %#v, want left-to-right signal flow", positions)
	}
}

func TestPlacePowerAndGroundVerticalLanes(t *testing.T) {
	result := Place(Request{
		Sheet: testSheet(),
		Components: []Component{
			{Ref: "#PWR01", Role: "positive_rail"},
			{Ref: "U1", Role: "opamp"},
			{Ref: "#PWR02", Role: "ground"},
			{Ref: "#PWR03", Role: "negative_rail"},
		},
	})
	positions := placedPositions(result.Components)
	if !(positions["#PWR01"].Y < positions["U1"].Y && positions["U1"].Y < positions["#PWR02"].Y && positions["#PWR02"].Y < positions["#PWR03"].Y) {
		t.Fatalf("positions = %#v, want power, signal, ground, negative ordering", positions)
	}
}

func TestPlacePreservesFixedCoordinates(t *testing.T) {
	fixed := kicadfiles.Point{X: kicadfiles.MM(77), Y: kicadfiles.MM(88)}
	result := Place(Request{Sheet: testSheet(), Components: []Component{{Ref: "R1", Role: "resistor", Fixed: true, Position: fixed}}})
	if got := result.Components[0].PlacedAt; got != SnapPoint(fixed, DefaultRules(ProfileStandard).Grid) {
		t.Fatalf("fixed position = %#v", got)
	}
	if !hasDiagnostic(result.Diagnostics, "fixed_component", SeverityInfo) {
		t.Fatalf("diagnostics = %#v, want fixed component info", result.Diagnostics)
	}
}

func placedPositions(components []PlacedComponent) map[string]kicadfiles.Point {
	positions := map[string]kicadfiles.Point{}
	for _, component := range components {
		positions[component.Ref] = component.PlacedAt
	}
	return positions
}
