package pcb

import (
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestValidateGeneratedConnectivityAcceptsCorrectnessFixture(t *testing.T) {
	board, err := CorrectnessFixturePCB(CorrectnessFixtureInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "pcb-object-correctness",
	})
	if err != nil {
		t.Fatalf("CorrectnessFixturePCB returned error: %v", err)
	}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityRejectsDisconnectedPad(t *testing.T) {
	board, err := CorrectnessFixturePCB(CorrectnessFixtureInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "pcb-object-correctness",
	})
	if err != nil {
		t.Fatalf("CorrectnessFixturePCB returned error: %v", err)
	}
	board.TrackArcs = nil

	err = ValidateGeneratedConnectivity(board)
	if err == nil {
		t.Fatal("expected disconnected pad error")
	}
	if !strings.Contains(err.Error(), "footprints") || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedConnectivityRejectsDanglingRouteEndpoint(t *testing.T) {
	board, err := CorrectnessFixturePCB(CorrectnessFixtureInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "pcb-object-correctness",
	})
	if err != nil {
		t.Fatalf("CorrectnessFixturePCB returned error: %v", err)
	}
	board.Tracks[0].Start = point(12, 22.73)

	err = ValidateGeneratedConnectivity(board)
	if err == nil {
		t.Fatal("expected dangling endpoint error")
	}
	if !strings.Contains(err.Error(), "tracks[0].start") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsRotatedPadAnchor(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Position = point(10, 10)
	footprint.Rotation = 90
	footprint.Pads[0].UUID = kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	footprint.Pads = append(footprint.Pads, Pad{
		UUID:     kicadfiles.UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
		Name:     "2",
		NetCode:  1,
		Shape:    "rect",
		Position: point(2, 0),
		Size:     point(1, 1),
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
	})
	board.Footprints = []Footprint{footprint}
	board.Tracks = []Track{{
		UUID:    kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
		Start:   point(10, 10),
		End:     point(10, 12),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityRejectsRectangularPadCornerFalsePositive(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{
		connectivityFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1", point(10, 10), Pad{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 4),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
	}
	board.Tracks = []Track{{
		UUID:    kicadfiles.UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
		Start:   point(10.8, 11.8),
		End:     point(14, 10),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}
	board.Vias = []Via{{
		UUID:     kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
		Position: point(14, 10),
		Size:     kicadfiles.MM(0.8),
		Drill:    kicadfiles.MM(0.4),
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		NetCode:  1,
	}}

	err := ValidateGeneratedConnectivity(board)
	if err == nil {
		t.Fatal("expected rectangular pad corner to remain disconnected")
	}
	if !strings.Contains(err.Error(), "tracks[0].start") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsTrackTJunction(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{
		connectivityFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1", point(10, 10), Pad{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "J2", point(20, 10), Pad{
			UUID:    kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", "J3", point(15, 12), Pad{
			UUID:    kicadfiles.UUID("ffffffff-ffff-4fff-8fff-ffffffffffff"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
	}
	board.Tracks = []Track{
		{
			UUID:    kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
			Start:   point(10, 10),
			End:     point(20, 10),
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 1,
		},
		{
			UUID:    kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
			Start:   point(15, 12),
			End:     point(15, 10),
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 1,
		},
	}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsOverlappingPads(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{
		connectivityFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1", point(10, 10), Pad{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(2, 2),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "J2", point(11.9, 10), Pad{
			UUID:    kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(2, 2),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
	}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsViaPadEdgeOverlap(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{
		connectivityFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1", point(10, 10), Pad{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(2, 2),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "J2", point(14, 10), Pad{
			UUID:    kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
	}
	board.Vias = []Via{{
		UUID:     kicadfiles.UUID("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"),
		Position: point(11.4, 11.4),
		Size:     kicadfiles.MM(1.2),
		Drill:    kicadfiles.MM(0.4),
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		NetCode:  1,
	}}
	board.Tracks = []Track{{
		UUID:    kicadfiles.UUID("ffffffff-ffff-4fff-8fff-ffffffffffff"),
		Start:   point(11.4, 11.4),
		End:     point(14, 10),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsTrackWidthViaOverlap(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{
		connectivityFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1", point(10, 10), Pad{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "J2", point(20, 10), Pad{
			UUID:    kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
	}
	board.Vias = []Via{{
		UUID:     kicadfiles.UUID("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"),
		Position: point(15, 10),
		Size:     kicadfiles.MM(0.8),
		Drill:    kicadfiles.MM(0.4),
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		NetCode:  1,
	}}
	board.Tracks = []Track{
		{
			UUID:    kicadfiles.UUID("ffffffff-ffff-4fff-8fff-ffffffffffff"),
			Start:   point(10, 10),
			End:     point(14.55, 10),
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 1,
		},
		{
			UUID:    kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
			Start:   point(15, 10),
			End:     point(20, 10),
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 1,
		},
	}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsTrackArcTJunction(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{
		connectivityFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1", point(10, 10), Pad{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "J2", point(20, 10), Pad{
			UUID:    kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", "J3", point(15, 16), Pad{
			UUID:    kicadfiles.UUID("ffffffff-ffff-4fff-8fff-ffffffffffff"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
	}
	board.TrackArcs = []TrackArc{{
		UUID:    kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Start:   point(10, 10),
		Mid:     point(15, 15),
		End:     point(20, 10),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}
	board.Tracks = []Track{{
		UUID:    kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Start:   point(15, 16),
		End:     point(15, 15),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsPadOnTrackArcBody(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{
		connectivityFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1", point(10, 10), Pad{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "J2", point(20, 10), Pad{
			UUID:    kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
		connectivityFootprint("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", "J3", point(15, 15), Pad{
			UUID:    kicadfiles.UUID("ffffffff-ffff-4fff-8fff-ffffffffffff"),
			Name:    "1",
			NetCode: 1,
			Shape:   "rect",
			Size:    point(1, 1),
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		}),
	}
	board.TrackArcs = []TrackArc{{
		UUID:    kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Start:   point(10, 10),
		Mid:     point(15, 15),
		End:     point(20, 10),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}

	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func connectivityFootprint(uuid, reference string, position kicadfiles.Point, pad Pad) Footprint {
	textSeed := strings.TrimPrefix(reference, "J")
	if textSeed == "" {
		textSeed = "0"
	}
	return Footprint{
		UUID:      kicadfiles.UUID(uuid),
		Path:      "root.connectivity." + strings.ToLower(reference),
		LibraryID: "Test:Pad",
		Reference: reference,
		Value:     "pad",
		Position:  position,
		Layer:     kicadfiles.LayerFCu,
		Texts: []FootprintText{
			{UUID: kicadfiles.UUID("10000000-0000-4000-8000-00000000000" + textSeed), Kind: "reference", Text: reference, Layer: kicadfiles.LayerFSilkS},
			{UUID: kicadfiles.UUID("20000000-0000-4000-8000-00000000000" + textSeed), Kind: "value", Text: "pad", Layer: kicadfiles.LayerFSilkS},
		},
		Pads: []Pad{pad},
	}
}
