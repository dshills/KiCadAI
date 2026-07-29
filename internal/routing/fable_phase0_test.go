package routing

import "testing"

func TestFableH13BoardEdgeIncludesTraceRadius(t *testing.T) {
	rules := Rules{EdgeClearanceMM: 0.25, TraceWidthMM: 0.25}
	got := UsableBoardRect(Board{WidthMM: 10, HeightMM: 10}, rules)
	wantCopperSafe := rules.EdgeClearanceMM + rules.TraceWidthMM/2
	if got.Min.XMM != wantCopperSafe || got.Min.YMM != wantCopperSafe {
		t.Fatalf("usable minimum = %#v, want copper-safe margin %v", got.Min, wantCopperSafe)
	}
}

func TestFableH13BoardEdgeQuantizesInward(t *testing.T) {
	request := minimalRequest()
	request.Rules.GridMM = 0.25
	request.Rules.EdgeClearanceMM = 0.2
	request.Rules.TraceWidthMM = 0.2
	occupancy, err := BuildOccupancy(request, "SIG")
	if err != nil {
		t.Fatal(err)
	}
	if !occupancy.BlockedCell(occupancy.Grid.ToGrid(Point{XMM: 0.25, YMM: 5}, 0)) {
		t.Fatal("grid point outside the copper-safe boundary is routable")
	}
	if occupancy.BlockedCell(occupancy.Grid.ToGrid(Point{XMM: 0.5, YMM: 5}, 0)) {
		t.Fatal("first grid point inside the copper-safe boundary is blocked")
	}
}

func TestFableH14ViaOccupancyUsesViaClearance(t *testing.T) {
	request := minimalRequest()
	request.Rules.GridMM = 0.1
	request.Rules.ClearanceMM = 0.1
	request.Rules.ViaClearanceMM = 0.6
	request.Rules.ViaDiameterMM = 0.6
	request.Components = []Component{{
		Ref:      "U1",
		Position: Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{{
			Ref: "U1", Name: "1", Net: "OTHER", Shape: PadRect, Type: PadSMD,
			Size: Size{WidthMM: 1, HeightMM: 1}, Layers: []string{"F.Cu"},
		}},
	}}
	occupancy, err := BuildViaOccupancy(request, "SIG")
	if err != nil {
		t.Fatal(err)
	}
	probe := occupancy.Grid.ToGrid(Point{XMM: 11.2, YMM: 10}, 0)
	if !occupancy.BlockedCell(probe) {
		t.Fatal("probe inside declared via clearance is routable")
	}
	exact := occupancy.Grid.ToGrid(Point{XMM: 11.4, YMM: 10}, 0)
	if occupancy.BlockedCell(exact) {
		t.Fatal("probe at exact declared via clearance is blocked")
	}
	outside := occupancy.Grid.ToGrid(Point{XMM: 11.5, YMM: 10}, 0)
	if occupancy.BlockedCell(outside) {
		t.Fatal("probe beyond declared via clearance is blocked")
	}
}
