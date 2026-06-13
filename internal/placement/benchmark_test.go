package placement

import (
	"fmt"
	"testing"
)

func BenchmarkPlaceModerateBoard(b *testing.B) {
	request := Request{
		Board: BoardPlacementArea{WidthMM: 100, HeightMM: 60, MarginMM: 1},
		Rules: Rules{GridMM: 1, ComponentSpacingMM: 0.5, BoardEdgeClearanceMM: 1, MaxCandidatesPerPart: 500},
	}
	for i := 0; i < 40; i++ {
		ref := fmt.Sprintf("R%d", i+1)
		request.Components = append(request.Components, Component{
			Ref:         ref,
			FootprintID: "Resistor_SMD:R_0805_2012Metric",
			Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
			Pads:        []PadSummary{{Name: "1"}, {Name: "2"}},
		})
		if i > 0 {
			request.Nets = append(request.Nets, Net{
				Name:      fmt.Sprintf("N%d", i),
				Endpoints: []Endpoint{{Ref: fmt.Sprintf("R%d", i), Pin: "2"}, {Ref: ref, Pin: "1"}},
			})
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := Place(request)
		if result.Status != StatusPlaced {
			b.Fatalf("status = %s; issues=%#v", result.Status, result.Issues)
		}
	}
}
