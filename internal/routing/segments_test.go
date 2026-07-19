package routing

import "testing"

func TestBuildSegmentsFromPathAppliesBoundedEndpointNeckdown(t *testing.T) {
	path := GridPath{Net: "VCC", Layer: "F.Cu", Points: []Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}}

	segments, metrics := BuildSegmentsFromPathWithNeckdown(path, 0.8, 0.2, 3)

	if metrics.TotalLengthMM != 10 || len(segments) != 3 {
		t.Fatalf("segments = %#v metrics = %#v", segments, metrics)
	}
	if segments[0].WidthMM != 0.2 || segments[1].WidthMM != 0.8 || segments[2].WidthMM != 0.2 {
		t.Fatalf("neckdown widths = %#v", segments)
	}
	if segments[0].End.XMM != 3 || segments[1].End.XMM != 7 {
		t.Fatalf("neckdown boundaries = %#v", segments)
	}
}

func TestBuildSegmentsFromPathMergesStraightPath(t *testing.T) {
	path := GridPath{
		Net:   "SIG",
		Layer: "F.CU",
		Points: []Point{
			{XMM: 1, YMM: 2},
			{XMM: 2, YMM: 2},
			{XMM: 3, YMM: 2},
		},
		SearchNodes: 12,
	}

	segments, metrics := BuildSegmentsFromPath(path, 0.2)
	if len(segments) != 1 {
		t.Fatalf("segments = %#v, want one merged segment", segments)
	}
	if segments[0].Start != (Point{XMM: 1, YMM: 2}) || segments[0].End != (Point{XMM: 3, YMM: 2}) {
		t.Fatalf("segment = %#v", segments[0])
	}
	if metrics.SegmentCount != 1 || metrics.TotalLengthMM != 2 || metrics.SearchNodes != 12 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestBuildSegmentsFromPathPreservesBends(t *testing.T) {
	path := GridPath{
		Net:   "SIG",
		Layer: "F.CU",
		Points: []Point{
			{XMM: 1, YMM: 1},
			{XMM: 3, YMM: 1},
			{XMM: 3, YMM: 4},
		},
	}

	segments, metrics := BuildSegmentsFromPath(path, 0.25)
	if len(segments) != 2 {
		t.Fatalf("segments = %#v, want two bend segments", segments)
	}
	if segments[0].End != segments[1].Start {
		t.Fatalf("bend not preserved: %#v", segments)
	}
	if metrics.TotalLengthMM != 5 {
		t.Fatalf("length = %f, want 5", metrics.TotalLengthMM)
	}
}

func TestBuildSegmentsFromPathRemovesZeroLengthSegments(t *testing.T) {
	path := GridPath{
		Net:   "SIG",
		Layer: "F.CU",
		Points: []Point{
			{XMM: 1, YMM: 1},
			{XMM: 1, YMM: 1},
			{XMM: 2, YMM: 1},
			{XMM: 2, YMM: 1},
		},
	}

	segments, metrics := BuildSegmentsFromPath(path, 0.25)
	if len(segments) != 1 {
		t.Fatalf("segments = %#v, want one non-zero segment", segments)
	}
	if metrics.TotalLengthMM != 1 {
		t.Fatalf("length = %f, want 1", metrics.TotalLengthMM)
	}
}

func TestBuildSegmentsFromPathPreservesDoublingBackSegments(t *testing.T) {
	path := GridPath{
		Net:   "SIG",
		Layer: "F.CU",
		Points: []Point{
			{XMM: 1, YMM: 1},
			{XMM: 2, YMM: 1},
			{XMM: 1, YMM: 1},
		},
	}

	segments, metrics := BuildSegmentsFromPath(path, 0.25)
	if len(segments) != 2 {
		t.Fatalf("segments = %#v, want two non-zero doubling-back segments", segments)
	}
	if metrics.SegmentCount != 2 || metrics.TotalLengthMM != 2 {
		t.Fatalf("metrics = %#v, want two millimeters of route", metrics)
	}
}

func TestBuildSegmentsFromPathRoundsOutput(t *testing.T) {
	path := GridPath{
		Net:   "SIG",
		Layer: "F.CU",
		Points: []Point{
			{XMM: 1.0000004, YMM: 1.0000004},
			{XMM: 2.0000004, YMM: 1.0000004},
		},
	}

	segments, _ := BuildSegmentsFromPath(path, 0.1234567)
	if len(segments) != 1 {
		t.Fatalf("segments = %#v, want one segment", segments)
	}
	if segments[0].Start != (Point{XMM: 1, YMM: 1}) || segments[0].WidthMM != 0.123457 {
		t.Fatalf("rounded segment = %#v", segments[0])
	}
}

func TestBuildSegmentsFromPathPlacesTerminalTransitionConnectorOnPadLayer(t *testing.T) {
	path := GridPath{
		Net:         "SIG",
		Layer:       "B.Cu",
		LayerNames:  map[int]string{0: "B.Cu", 1: "F.Cu"},
		Coordinates: []GridCoord{{X: 10, Y: 20, Layer: 0}, {X: 10, Y: 20, Layer: 1}},
		Points:      []Point{{XMM: 2.5, YMM: 5}, {XMM: 2.63, YMM: 5.17}},
	}
	segments, _ := BuildSegmentsFromPath(path, 0.25)
	if len(segments) != 1 || normalizeLayer(segments[0].Layer) != "F.CU" || segments[0].Start != path.Points[0] || segments[0].End != path.Points[1] {
		t.Fatalf("segments = %#v, want terminal connector on destination pad layer", segments)
	}
}

func TestBuildSegmentsFromPathPreventsZeroWidthAfterRounding(t *testing.T) {
	path := GridPath{
		Net:   "SIG",
		Layer: "F.CU",
		Points: []Point{
			{XMM: 1, YMM: 1},
			{XMM: 2, YMM: 1},
		},
	}

	segments, _ := BuildSegmentsFromPath(path, 0.0000001)
	if len(segments) != 1 {
		t.Fatalf("segments = %#v, want one segment", segments)
	}
	if segments[0].WidthMM <= 0 {
		t.Fatalf("width = %f, want positive fallback", segments[0].WidthMM)
	}
}
