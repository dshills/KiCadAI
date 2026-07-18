package designworkflow

import (
	"math"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/transactions"
)

func TestPlacedPadEndpointResolverResolvesZeroRotationPad(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "F.Cu"}, placement.PadSummary{Name: "2", Net: "SIG", XMM: 1.5, YMM: -0.5})
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))

	endpoint, ok := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "2"})
	if !ok {
		t.Fatalf("endpoint not resolved; issues=%#v", resolver.Issues())
	}
	assertEndpointPoint(t, endpoint, 11.5, 19.5)
	if endpoint.NetCode != 1 || endpoint.NetName != "SIG" || endpoint.Layer != "F.Cu" {
		t.Fatalf("endpoint = %#v", endpoint)
	}
}

func TestPlacedPadEndpointResolverRotatesPadOffsets(t *testing.T) {
	tests := []struct {
		name     string
		rotation float64
		wantX    float64
		wantY    float64
	}{
		{name: "ninety", rotation: 90, wantX: 10, wantY: 18},
		{name: "one eighty", rotation: 180, wantX: 8, wantY: 20},
		{name: "arbitrary", rotation: 45, wantX: 10 + math.Sqrt2, wantY: 20 - math.Sqrt2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, RotationDeg: tt.rotation, Layer: "F.Cu"}, placement.PadSummary{Name: "1", Net: "SIG", XMM: 2, YMM: 0})
			resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))
			endpoint, ok := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "1"})
			if !ok {
				t.Fatalf("endpoint not resolved; issues=%#v", resolver.Issues())
			}
			assertEndpointPoint(t, endpoint, tt.wantX, tt.wantY)
		})
	}
}

func TestPlacedPadEndpointResolverMirrorsBackLayerPadOffset(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "B.Cu"}, placement.PadSummary{Name: "1", Net: "SIG", XMM: 2, YMM: 0})
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))

	endpoint, ok := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "1"})
	if !ok {
		t.Fatalf("endpoint not resolved; issues=%#v", resolver.Issues())
	}
	assertEndpointPoint(t, endpoint, 8, 20)
	if endpoint.Layer != "B.Cu" {
		t.Fatalf("endpoint layer = %q, want B.Cu", endpoint.Layer)
	}
}

func TestPlacedPadEndpointResolverReportsMissingPadGeometry(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "F.Cu"})
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))

	if _, ok := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "1"}); ok {
		t.Fatalf("resolved endpoint without pad geometry")
	}
	if len(resolver.Issues()) == 0 {
		t.Fatalf("missing resolver issue")
	}
}

func TestPlacedPadEndpointResolverReportsPadNetMissingFromTable(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "F.Cu"}, placement.PadSummary{Name: "1", Net: "SIG"})
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("GND"))

	endpoint, ok := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "1"})
	if !ok {
		t.Fatalf("missing-net endpoint location should still resolve")
	}
	if endpoint.NetCodeResolved || endpoint.NetCode != 0 {
		t.Fatalf("endpoint = %#v, want unresolved net code", endpoint)
	}
	if len(resolver.Issues()) == 0 {
		t.Fatalf("missing net-table diagnostic")
	}
}

func TestPlacedPadEndpointResolverEndpointsAreStable(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "F.Cu"},
		placement.PadSummary{Name: "2", Net: "SIG"},
		placement.PadSummary{Name: "1", Net: "SIG"},
	)
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))

	endpoints := resolver.Endpoints()
	if len(endpoints) != 2 || endpoints[0].Pad != "1" || endpoints[1].Pad != "2" {
		t.Fatalf("endpoints = %#v, want sorted by pad", endpoints)
	}
}

func TestPlacedPadEndpointResolverQueriesSpatialBoundsDeterministically(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "F.Cu"},
		placement.PadSummary{Name: "2", Net: "SIG", XMM: 8, WidthMM: 2, HeightMM: 2},
		placement.PadSummary{Name: "1", Net: "SIG", XMM: 1, WidthMM: 1, HeightMM: 1},
	)
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))

	near := resolver.EndpointsWithinBounds(9, 19, 12, 21)
	if len(near) != 1 || near[0].Pad != "1" {
		t.Fatalf("near endpoints = %#v, want only sorted pad 1", near)
	}
	if resolver.MaximumPadRadiusMM() < math.Sqrt2 {
		t.Fatalf("maximum pad radius = %g, want conservative half diagonal", resolver.MaximumPadRadiusMM())
	}
	overlap := resolver.EndpointsWithinBounds(17, 19, 17.5, 21)
	if len(overlap) != 1 || overlap[0].Pad != "2" {
		t.Fatalf("overlapping endpoints = %#v, want pad 2 whose extent enters bounds", overlap)
	}
}

func TestPlacedPadEndpointResolverResolvesSameNetDuplicatePhysicalPads(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "F.Cu"},
		placement.PadSummary{Name: "2", Net: "SIG", XMM: 0, YMM: 1},
		placement.PadSummary{Name: "2", Net: "SIG", XMM: 0, YMM: -1},
	)
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))

	if len(resolver.Issues()) != 0 {
		t.Fatalf("same-net pad alias should not create resolver issues: %#v", resolver.Issues())
	}
	first, firstOK := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "2"})
	second, secondOK := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "2#2"})
	if !firstOK || !secondOK {
		t.Fatalf("same-net physical pads should resolve: first=%t second=%t", firstOK, secondOK)
	}
	if first.Point == second.Point {
		t.Fatalf("duplicate physical pads resolved to the same point: %#v", first.Point)
	}
	if first.Pad != "2" || second.Pad != "2" {
		t.Fatalf("resolved physical names = %q, %q; want 2", first.Pad, second.Pad)
	}
}

func TestPlacedPadEndpointResolverDoesNotResolveUnknownPad(t *testing.T) {
	placed := endpointResolverPlacement(placement.Placement{XMM: 10, YMM: 20, Layer: "F.Cu"}, placement.PadSummary{Name: "1", Net: "SIG"})
	resolver := NewPlacedPadEndpointResolver(&placed, generatedNetTableFromNames("SIG"))

	if _, ok := resolver.Resolve(transactions.Endpoint{Ref: "R1", Pin: "2"}); ok {
		t.Fatalf("resolved unknown pad")
	}
}

func endpointResolverPlacement(position placement.Placement, pads ...placement.PadSummary) PlacementStageResult {
	return PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{{
				Ref:         "R1",
				FootprintID: "Test:R_0603",
				Role:        "resistor",
				Pads:        pads,
			}},
		},
		Result: placement.Result{
			Status: placement.StatusPlaced,
			Placements: []placement.PlacementResult{{
				Ref: "R1", FootprintID: "Test:R_0603", Position: position,
			}},
		},
		Stage: NewStageResult(StagePlacement, nil),
	}
}

func assertEndpointPoint(t *testing.T, endpoint PlacedPadEndpoint, wantX float64, wantY float64) {
	t.Helper()
	if math.Abs(endpoint.Point.XMM-wantX) > 1e-9 || math.Abs(endpoint.Point.YMM-wantY) > 1e-9 {
		t.Fatalf("endpoint point = (%g,%g), want (%g,%g)", endpoint.Point.XMM, endpoint.Point.YMM, wantX, wantY)
	}
}
