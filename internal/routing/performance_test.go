package routing

import "testing"

func TestRouteRequestRepeatedDeterministic(t *testing.T) {
	request := crossingNetsRequest()
	request.Seed = "stress-deterministic-routing"
	iterations := 25
	if testing.Short() {
		iterations = 5
	}

	var wantSignature string
	var wantRoutedNets int
	var wantSegments int
	var wantVias int
	var wantSearchNodes int
	for i := 0; i < iterations; i++ {
		result := RouteRequest(cloneRequest(request))
		if result.Status != StatusRouted {
			t.Fatalf("run %d status = %s issues = %#v", i, result.Status, result.Issues)
		}
		signature := routeSignature(result.Routes)
		if i == 0 {
			wantSignature = signature
			wantRoutedNets = result.Metrics.RoutedNetCount
			wantSegments = result.Metrics.SegmentCount
			wantVias = result.Metrics.ViaCount
			wantSearchNodes = result.Metrics.SearchNodes
			continue
		}
		if signature != wantSignature {
			t.Fatalf("run %d signature = %s, want %s", i, signature, wantSignature)
		}
		if result.Metrics.RoutedNetCount != wantRoutedNets {
			t.Fatalf("run %d routed nets = %d, want %d", i, result.Metrics.RoutedNetCount, wantRoutedNets)
		}
		if result.Metrics.SegmentCount != wantSegments {
			t.Fatalf("run %d segments = %d, want %d", i, result.Metrics.SegmentCount, wantSegments)
		}
		if result.Metrics.ViaCount != wantVias {
			t.Fatalf("run %d vias = %d, want %d", i, result.Metrics.ViaCount, wantVias)
		}
		if result.Metrics.SearchNodes != wantSearchNodes {
			t.Fatalf("run %d search nodes = %d, want %d", i, result.Metrics.SearchNodes, wantSearchNodes)
		}
	}
}

func TestRouteRequestSearchLimitFailsCleanly(t *testing.T) {
	request := goldenSingleLayerDetourRequest()
	request.Rules.MaxSearchNodes = 1
	request.Strategy.AllowPartial = false

	result := RouteRequest(cloneRequest(request))
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked; result = %#v", result.Status, result)
	}
	if result.Metrics.RoutedNetCount != 0 || result.Metrics.FailedNetCount != len(request.Nets) {
		t.Fatalf("metrics = %#v, want all %d nets to fail cleanly", result.Metrics, len(request.Nets))
	}
	if len(result.Routes) != len(request.Nets) {
		t.Fatalf("routes = %#v, want failed route for every input net", result.Routes)
	}
	for _, route := range result.Routes {
		if route.Status != RouteStatusFailed {
			t.Fatalf("route = %#v, want failed status", route)
		}
		if len(route.Segments) != 0 || len(route.Vias) != 0 {
			t.Fatalf("failed route produced geometry: %#v", route)
		}
	}
	if len(DiagnosticsForResult(result)) == 0 {
		t.Fatalf("diagnostics missing for blocked search result: %#v", result)
	}
}

func BenchmarkRouteRequestGoldenDetour(b *testing.B) {
	request := goldenSingleLayerDetourRequest()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		next := cloneRequest(request)
		b.StartTimer()
		result := RouteRequest(next)
		if result.Status != StatusRouted {
			b.Fatalf("status = %s issues = %#v", result.Status, result.Issues)
		}
	}
}
