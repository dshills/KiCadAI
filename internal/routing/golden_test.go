package routing

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

func TestGoldenRoutedExamples(t *testing.T) {
	cases := []struct {
		name          string
		request       Request
		wantStatus    Status
		wantRoutes    int
		wantSegments  int
		wantVias      int
		wantSignature string
	}{
		{
			name:          "straight_single_layer_signal",
			request:       goldenStraightSingleLayerRequest(),
			wantStatus:    StatusRouted,
			wantRoutes:    1,
			wantSegments:  1,
			wantVias:      0,
			wantSignature: "S:SIG:F.CU:5.000,10.000>20.000,10.000",
		},
		{
			name:          "single_layer_keepout_detour",
			request:       goldenSingleLayerDetourRequest(),
			wantStatus:    StatusRouted,
			wantRoutes:    1,
			wantSegments:  3,
			wantVias:      0,
			wantSignature: "S:SIG:F.CU:20.000,7.000>20.000,10.000|S:SIG:F.CU:5.000,7.000>20.000,7.000|S:SIG:F.CU:5.000,7.000>5.000,10.000",
		},
		{
			name:       "two_layer_smd_via",
			request:    goldenTwoLayerViaRequest(),
			wantStatus: StatusRouted,
			wantRoutes: 1,
			// The destination pad is only on B.Cu, so the golden path places a
			// via directly at that endpoint after a single F.Cu segment.
			wantSegments:  1,
			wantVias:      1,
			wantSignature: "S:SIG:F.CU:5.000,10.000>20.000,10.000|V:SIG:20.000,10.000:B.CU,F.CU",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := RouteRequest(tc.request)
			if result.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s; issues = %#v", result.Status, tc.wantStatus, result.Issues)
			}
			if len(result.Routes) != tc.wantRoutes {
				t.Fatalf("routes = %d, want %d: %#v", len(result.Routes), tc.wantRoutes, result.Routes)
			}
			segments := 0
			vias := 0
			for _, route := range result.Routes {
				segments += len(route.Segments)
				vias += len(route.Vias)
			}
			if signature := routeSignature(result.Routes); signature != tc.wantSignature {
				t.Fatalf("signature mismatch\n got: %s\nwant: %s", signature, tc.wantSignature)
			}
			if segments != tc.wantSegments || vias != tc.wantVias {
				t.Fatalf("segments/vias = %d/%d, want %d/%d; routes = %#v", segments, vias, tc.wantSegments, tc.wantVias, result.Routes)
			}
			report := ValidateResult(tc.request, result)
			if len(report.Issues) != 0 {
				t.Fatalf("validation issues = %#v", report.Issues)
			}
			if len(result.Operations) == 0 {
				t.Fatalf("operations missing for routed result: %#v", result)
			}
		})
	}
}

func goldenStraightSingleLayerRequest() Request {
	request := singleLayerSearchRequest()
	request.Seed = "golden-straight-single-layer"
	return request
}

func goldenSingleLayerDetourRequest() Request {
	request := singleLayerSearchRequest()
	request.Seed = "golden-single-layer-detour"
	request.Obstacles = cloneGoldenObstacles(request.Obstacles)
	request.Obstacles = append(request.Obstacles, Obstacle{
		Kind:  ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: Shape{Rect: &Rect{
			Min: Point{XMM: 9, YMM: 8},
			Max: Point{XMM: 16, YMM: 12},
		}},
		Source: "golden.keepout",
	})
	return request
}

func goldenTwoLayerViaRequest() Request {
	request := twoLayerViaRequest()
	request.Seed = "golden-two-layer-via"
	return request
}

func cloneGoldenObstacles(obstacles []Obstacle) []Obstacle {
	out := slices.Clone(obstacles)
	for i := range out {
		if out[i].Geometry.Rect != nil {
			rect := *out[i].Geometry.Rect
			out[i].Geometry.Rect = &rect
		}
		if len(out[i].Geometry.Polygon) != 0 {
			out[i].Geometry.Polygon = slices.Clone(out[i].Geometry.Polygon)
		}
	}
	return out
}

func routeSignature(routes []Route) string {
	parts := []string{}
	for _, route := range routes {
		for _, segment := range route.Segments {
			start := segment.Start
			end := segment.End
			if pointLess(end, start) {
				start, end = end, start
			}
			parts = append(parts, fmt.Sprintf("S:%s:%s:%s,%s>%s,%s", route.Net, segment.Layer, formatGoldenCoord(start.XMM), formatGoldenCoord(start.YMM), formatGoldenCoord(end.XMM), formatGoldenCoord(end.YMM)))
		}
		for _, via := range route.Vias {
			layers := slices.Clone(via.Layers)
			slices.Sort(layers)
			parts = append(parts, fmt.Sprintf("V:%s:%s,%s:%s", route.Net, formatGoldenCoord(via.At.XMM), formatGoldenCoord(via.At.YMM), strings.Join(layers, ",")))
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, "|")
}

func pointLess(left Point, right Point) bool {
	leftX := goldenCoord(left.XMM)
	rightX := goldenCoord(right.XMM)
	if leftX != rightX {
		return leftX < rightX
	}
	return goldenCoord(left.YMM) < goldenCoord(right.YMM)
}

func formatGoldenCoord(value float64) string {
	return fmt.Sprintf("%.3f", float64(goldenCoord(value))/1000)
}

func goldenCoord(value float64) int64 {
	return int64(math.Round(value * 1000))
}
