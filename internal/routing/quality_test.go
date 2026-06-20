package routing

import "testing"

func TestBuildQualityReportForRoutedNet(t *testing.T) {
	request := singleLayerSearchRequest()
	result := RouteRequest(request)
	if result.Quality == nil {
		t.Fatalf("expected quality report")
	}
	if result.Quality.Status != StatusRouted {
		t.Fatalf("quality status = %s", result.Quality.Status)
	}
	if len(result.Quality.NetReports) != 1 {
		t.Fatalf("net reports = %d, want 1", len(result.Quality.NetReports))
	}
	net := result.Quality.NetReports[0]
	if net.Status != RouteStatusRouted {
		t.Fatalf("net status = %s", net.Status)
	}
	if net.EndpointCount != 2 || net.RoutedEndpoints != 2 {
		t.Fatalf("endpoint counts = %d/%d", net.RoutedEndpoints, net.EndpointCount)
	}
	if net.SegmentCount == 0 || net.LengthMM <= 0 {
		t.Fatalf("expected routed geometry in report: %#v", net)
	}
	if result.Quality.Score.Overall <= 0 {
		t.Fatalf("overall score = %v", result.Quality.Score.Overall)
	}
}

func TestBuildQualityReportForFailedNet(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Strategy.AllowPartial = true
	request.Obstacles = append(request.Obstacles, Obstacle{
		Kind:  ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: Shape{Rect: &Rect{
			Min: Point{XMM: 0, YMM: 0},
			Max: Point{XMM: 30, YMM: 30},
		}},
	})
	result := RouteRequest(request)
	if result.Quality == nil {
		t.Fatalf("expected quality report")
	}
	net := result.Quality.NetReports[0]
	if net.Status != RouteStatusFailed {
		t.Fatalf("net status = %s", net.Status)
	}
	if net.FailureCategory == "" || net.SuggestedRepair == "" {
		t.Fatalf("expected failure evidence: %#v", net)
	}
}
