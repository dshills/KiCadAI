package routing

import "testing"

func TestFableH13ReproductionUsableBoardRectIgnoresTraceRadius(t *testing.T) {
	rules := Rules{EdgeClearanceMM: 0.25, TraceWidthMM: 0.25}
	got := UsableBoardRect(Board{WidthMM: 10, HeightMM: 10}, rules)
	if got.Min.XMM != 0.25 || got.Min.YMM != 0.25 {
		t.Fatalf("usable minimum = %#v, want current centerline-only margin", got.Min)
	}
	wantCopperSafe := rules.EdgeClearanceMM + rules.TraceWidthMM/2
	if got.Min.XMM >= wantCopperSafe {
		t.Fatalf("H13 reproduction no longer observes missing trace radius: got %v, copper-safe %v", got.Min.XMM, wantCopperSafe)
	}
}

func TestFableH14ReproductionViaOccupancyIgnoresViaClearance(t *testing.T) {
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
	if occupancy.BlockedCell(probe) {
		t.Fatal("probe is blocked; H14 reproduction no longer demonstrates ignored via clearance")
	}
}
