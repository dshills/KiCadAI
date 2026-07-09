package schematiclayout

import (
	"reflect"
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

func TestPlaceUsesGraphTopologyForArbitraryPassiveChain(t *testing.T) {
	request := Request{
		Sheet: testSheet(),
		Components: []Component{
			{Ref: "J1", Role: "input_connector", Pins: []Pin{{Number: "1", Role: "output"}}},
			{Ref: "R1", Role: "resistor", Pins: []Pin{{Number: "1", Role: "input"}, {Number: "2", Role: "output"}}},
			{Ref: "C1", Role: "capacitor", Pins: []Pin{{Number: "1", Role: "input"}, {Number: "2", Role: "output"}}},
			{Ref: "J2", Role: "output_connector", Pins: []Pin{{Number: "1", Role: "input"}}},
		},
		Nets: []Net{
			{Name: "N1", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "R1", Pin: "1"}}},
			{Name: "N2", Endpoints: []Endpoint{{Ref: "R1", Pin: "2"}, {Ref: "C1", Pin: "1"}}},
			{Name: "N3", Endpoints: []Endpoint{{Ref: "C1", Pin: "2"}, {Ref: "J2", Pin: "1"}}},
		},
	}
	result := Place(request)
	positions := placedPositions(result.Components)
	if !(positions["J1"].X < positions["R1"].X && positions["R1"].X < positions["C1"].X && positions["C1"].X < positions["J2"].X) {
		t.Fatalf("positions = %#v, want graph-derived chain order", positions)
	}
	if result.Report.IslandCount != 1 || result.Report.RankCount != 4 {
		t.Fatalf("report = %#v, want one four-rank island", result.Report)
	}
}

func TestPlaceCondensesFeedbackCycle(t *testing.T) {
	request := Request{
		Sheet: testSheet(),
		Components: []Component{
			{Ref: "J1", Role: "input_connector", Pins: []Pin{{Number: "1", Role: "output"}}},
			{Ref: "U1", Role: "ic", Pins: []Pin{{Number: "1", Role: "input"}, {Number: "2", Role: "output"}}},
			{Ref: "U2", Role: "ic", Pins: []Pin{{Number: "1", Role: "input"}, {Number: "2", Role: "output"}}},
		},
		Nets: []Net{
			{Name: "IN", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "1"}}},
			{Name: "FORWARD", Endpoints: []Endpoint{{Ref: "U1", Pin: "2"}, {Ref: "U2", Pin: "1"}}},
			{Name: "FEEDBACK", Role: "feedback", Endpoints: []Endpoint{{Ref: "U2", Pin: "2"}, {Ref: "U1", Pin: "1"}}},
		},
	}
	result := Place(request)
	positions := placedPositions(result.Components)
	if positions["J1"].X >= positions["U1"].X || positions["U1"].X >= positions["U2"].X {
		t.Fatalf("positions = %#v, feedback should not reverse forward flow", positions)
	}
}

func TestPlacePacksDisconnectedIslandsAndCentersDrawing(t *testing.T) {
	request := Request{
		Sheet: testSheet(),
		Components: []Component{
			{Ref: "J1", Role: "input_connector"},
			{Ref: "R1", Role: "resistor"},
			{Ref: "J2", Role: "input_connector"},
			{Ref: "R2", Role: "resistor"},
		},
		Nets: []Net{
			{Name: "A", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "R1", Pin: "1"}}},
			{Name: "B", Endpoints: []Endpoint{{Ref: "J2", Pin: "1"}, {Ref: "R2", Pin: "1"}}},
		},
	}
	result := Place(request)
	if result.Report.IslandCount != 2 {
		t.Fatalf("island count = %d, want 2", result.Report.IslandCount)
	}
	usable := UsableSheet(request.Sheet)
	bounds := result.Report.OccupiedBounds
	if delta := absIU((usable.MinX+usable.MaxX)/2 - (bounds.MinX+bounds.MaxX)/2); delta > DefaultRules(ProfileStandard).Grid {
		t.Fatalf("horizontal center delta = %v, bounds=%#v usable=%#v", delta, bounds, usable)
	}
	if delta := absIU((usable.MinY+usable.MaxY)/2 - (bounds.MinY+bounds.MaxY)/2); delta > DefaultRules(ProfileStandard).Grid {
		t.Fatalf("vertical center delta = %v, bounds=%#v usable=%#v", delta, bounds, usable)
	}
}

func TestPlaceIsStableUnderInputPermutation(t *testing.T) {
	request := Request{
		Sheet: testSheet(),
		Components: []Component{
			{Ref: "J1", Role: "input_connector"},
			{Ref: "R1", Role: "resistor"},
			{Ref: "C1", Role: "capacitor"},
		},
		Nets: []Net{
			{Name: "A", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "R1", Pin: "1"}}},
			{Name: "B", Endpoints: []Endpoint{{Ref: "R1", Pin: "2"}, {Ref: "C1", Pin: "1"}}},
		},
	}
	reversed := request
	reversed.Components = []Component{request.Components[2], request.Components[1], request.Components[0]}
	reversed.Nets = []Net{request.Nets[1], request.Nets[0]}
	first := placedPositions(Place(request).Components)
	second := placedPositions(Place(reversed).Components)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("placement changed under permutation: first=%#v second=%#v", first, second)
	}
}

func TestPlaceKeepsComponentBodiesSeparated(t *testing.T) {
	var components []Component
	for index := 0; index < 12; index++ {
		components = append(components, Component{Ref: string(rune('A' + index)), Role: "ic"})
	}
	result := Place(Request{Sheet: testSheet(), Components: components})
	for index, component := range result.Components {
		first := componentBody(component)
		for other := index + 1; other < len(result.Components); other++ {
			second := componentBody(result.Components[other])
			if first.Intersects(second) {
				t.Fatalf("component bodies overlap: %s %#v and %s %#v", component.Ref, first, result.Components[other].Ref, second)
			}
		}
	}
}

func TestPlaceKeepsGeneratedFieldsClear(t *testing.T) {
	result := Place(Request{
		Sheet: testSheet(),
		Components: []Component{
			{Ref: "U1", Value: "controller", Role: "mcu", FlowRank: 1, RankFixed: true},
			{Ref: "R1", Value: "10k", Role: "resistor", FlowRank: 1, RankFixed: true},
			{Ref: "C1", Value: "100n", Role: "decoupling", FlowRank: 1, RankFixed: true},
		},
	})
	if result.Report.OverlapCounts != nil || result.Report.WarningCount != 0 || result.Report.ErrorCount != 0 {
		t.Fatalf("field placement report = %#v diagnostics=%#v", result.Report, result.Diagnostics)
	}
	for _, component := range result.Components {
		if component.ReferenceText.At == (kicadfiles.Point{}) || component.ValueText.At == (kicadfiles.Point{}) {
			t.Fatalf("component %s missing explicit field anchors: %#v %#v", component.Ref, component.ReferenceText, component.ValueText)
		}
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
