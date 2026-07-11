package schematicir

import (
	"math"
	"testing"
)

func TestNormalizeLayoutIntentAddsGroupsAndPlacements(t *testing.T) {
	doc := validLEDDocument()

	normalized := NormalizeLayoutIntent(doc)

	groupsByID := map[string]Group{}
	for _, group := range normalized.Layout.Groups {
		groupsByID[group.ID] = group
	}
	if got := groupsByID["inputs"].Members; len(got) != 1 || got[0] != "vin" {
		t.Fatalf("input group members = %+v", got)
	}
	if got := groupsByID["signal"].Members; len(got) != 2 || got[0] != "r_limit" || got[1] != "led" {
		t.Fatalf("signal group members = %+v", got)
	}
	if len(normalized.Layout.Placements) != len(doc.Circuit.Components) {
		t.Fatalf("expected one placement per component, got %d", len(normalized.Layout.Placements))
	}
	placements := placementsByTarget(normalized.Layout.Placements)
	if placements["r_limit"].Group != "signal" || placements["r_limit"].Orientation != OrientationRotated {
		t.Fatalf("unexpected resistor placement: %+v", placements["r_limit"])
	}
	if placements["vin"].Group != "inputs" || placements["vin"].Orientation != OrientationNormal {
		t.Fatalf("unexpected input placement: %+v", placements["vin"])
	}
}

func TestNormalizeLayoutIntentPreservesExplicitGroupsAndPlacements(t *testing.T) {
	doc := validLEDDocument()
	doc.Layout.Groups = []Group{
		{ID: "front_panel", Label: "Front Panel", Role: GroupRoleInputStage, Members: []string{"vin"}, Rank: 7, Side: SideLeft},
	}
	doc.Layout.Placements = []Placement{
		{Target: "vin", Group: "front_panel", Orientation: OrientationRotated},
	}

	normalized := NormalizeLayoutIntent(doc)

	if normalized.Layout.Groups[0].ID != "front_panel" || normalized.Layout.Groups[0].Rank != 7 {
		t.Fatalf("explicit group was not preserved: %+v", normalized.Layout.Groups[0])
	}
	placements := placementsByTarget(normalized.Layout.Placements)
	if placements["vin"].Group != "front_panel" || placements["vin"].Orientation != OrientationRotated {
		t.Fatalf("explicit placement was not preserved: %+v", placements["vin"])
	}
	if placements["led"].Group == "" {
		t.Fatalf("missing generated placement for LED: %+v", placements["led"])
	}
}

func TestNormalizeLayoutIntentDoesNotDuplicateExistingGroupMembers(t *testing.T) {
	doc := validLEDDocument()
	doc.Layout.Groups = []Group{
		{ID: "signal", Role: GroupRoleProcessingStage, Members: []string{"r_limit"}, Rank: defaultLayoutFallbackRank},
	}

	normalized := NormalizeLayoutIntent(doc)

	var signal Group
	for _, group := range normalized.Layout.Groups {
		if group.ID == "signal" {
			signal = group
		}
	}
	seen := map[string]int{}
	for _, member := range signal.Members {
		seen[member]++
	}
	if seen["r_limit"] != 1 {
		t.Fatalf("existing group member duplicated: %+v", signal.Members)
	}
	if seen["led"] != 1 {
		t.Fatalf("missing generated signal member: %+v", signal.Members)
	}
}

func TestNormalizeLayoutIntentSnapsBusGeometryToKiCadGrid(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Buses = []Bus{{
		ID:      "data_bus",
		Name:    "DATA[0..1]",
		Members: []BusMember{{Net: "VIN", Label: "DATA0"}},
	}}
	doc.Layout.Buses = []BusLayout{{
		Bus:    "data_bus",
		Points: []LayoutPoint{{XMM: 75, YMM: 80}, {XMM: 150, YMM: 80}},
		Entries: []BusEntryLayout{{
			Member:   "VIN",
			Endpoint: "vin.1",
			At:       LayoutPoint{XMM: 82.5, YMM: 80},
			Size:     LayoutPoint{XMM: 1, YMM: -1},
		}},
	}}

	normalized := NormalizeLayoutIntent(doc)
	bus := normalized.Layout.Buses[0]
	if !closeLayoutPoint(bus.Points[0], LayoutPoint{XMM: 76.2, YMM: 78.74}) || !closeLayoutPoint(bus.Points[1], LayoutPoint{XMM: 149.86, YMM: 78.74}) {
		t.Fatalf("snapped bus points = %#v", bus.Points)
	}
	entry := bus.Entries[0]
	if !closeLayoutPoint(entry.At, LayoutPoint{XMM: 81.28, YMM: 78.74}) || !closeLayoutPoint(entry.Size, LayoutPoint{XMM: 2.54, YMM: -2.54}) {
		t.Fatalf("snapped bus entry = %#v", entry)
	}
	if doc.Layout.Buses[0].Points[0] != (LayoutPoint{XMM: 75, YMM: 80}) {
		t.Fatalf("NormalizeLayoutIntent mutated the source document: %#v", doc.Layout.Buses[0].Points)
	}
}

func TestPlacementRelationCycleIsDeterministic(t *testing.T) {
	placements := []Placement{
		{Target: "b", Above: []string{"a"}},
		{Target: "a", Above: []string{"b"}},
	}
	cycle := PlacementRelationCycle(placements, "above")
	if got := FormatPlacementRelationCycle(cycle); got != "a -> b -> a" {
		t.Fatalf("cycle = %q", got)
	}
	if cycle := PlacementRelationCycle(placements, "near"); cycle != nil {
		t.Fatalf("unsupported relation cycle = %#v", cycle)
	}
}

func closeLayoutPoint(left, right LayoutPoint) bool {
	const epsilon = 1e-9
	return math.Abs(left.XMM-right.XMM) <= epsilon && math.Abs(left.YMM-right.YMM) <= epsilon
}

func placementsByTarget(placements []Placement) map[string]Placement {
	out := map[string]Placement{}
	for _, placement := range placements {
		out[placement.Target] = placement
	}
	return out
}
