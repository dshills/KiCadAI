package designworkflow

import (
	"context"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestFailedNetNegotiationRevisitsFailureAfterPriorityStateChanges(t *testing.T) {
	request := routing.Request{Nets: []routing.Net{{Name: "A"}, {Name: "B"}}}
	calls := 0
	route := func(_ context.Context, candidate routing.Request) routing.Result {
		calls++
		priorities := map[string]int{}
		for _, net := range candidate.Nets {
			priorities[net.Name] = net.Priority
		}
		failed := "A"
		switch {
		case priorities["A"] >= 3:
			return routing.Result{Status: routing.StatusRouted, Metrics: routing.Metrics{RoutedNetCount: 2}}
		case priorities["B"] > priorities["A"]:
			failed = "A"
		case priorities["A"] > priorities["B"]:
			failed = "B"
		}
		return routing.Result{Status: routing.StatusPartial, Metrics: routing.Metrics{RoutedNetCount: 1, FailedNetCount: 1}, Issues: []reports.Issue{{Severity: reports.SeverityBlocked, Nets: []string{failed}}}}
	}
	result, summary := routeWithFailedNetFirstNegotiationUsing(context.Background(), request, route)
	if result.Status != routing.StatusRouted || result.Metrics.FailedNetCount != 0 || calls != 4 || summary.Attempts != 4 {
		t.Fatalf("result=%#v calls=%d summary=%#v", result, calls, summary)
	}
}

func TestFailedNetNegotiationExpandsExhaustedSearchBudgetOnce(t *testing.T) {
	request := routing.Request{
		Rules: routing.Rules{MaxSearchNodes: 100},
		Nets:  []routing.Net{{Name: "A"}},
	}
	calls := 0
	route := func(_ context.Context, candidate routing.Request) routing.Result {
		calls++
		if candidate.Rules.MaxSearchNodes >= 400 {
			return routing.Result{Status: routing.StatusRouted, Metrics: routing.Metrics{RoutedNetCount: 1}}
		}
		return routing.Result{
			Status:  routing.StatusPartial,
			Routes:  []routing.Route{{Net: "A", Status: routing.RouteStatusFailed, SearchLimitHit: true}},
			Metrics: routing.Metrics{FailedNetCount: 1, MaxSearchNodesHit: true},
			Issues:  []reports.Issue{{Severity: reports.SeverityBlocked, Nets: []string{"A"}}},
		}
	}

	result, summary := routeWithFailedNetFirstNegotiationUsing(context.Background(), request, route)
	if result.Status != routing.StatusRouted || calls != 3 || summary.Attempts != 3 {
		t.Fatalf("result=%#v calls=%d summary=%#v", result, calls, summary)
	}
}

func TestFailedNetNegotiationCanConvergeBeyondLegacyTwelveAttemptCeiling(t *testing.T) {
	request := routing.Request{Nets: make([]routing.Net, 13)}
	for index := range request.Nets {
		request.Nets[index].Name = string(rune('A' + index))
	}
	calls := 0
	route := func(_ context.Context, candidate routing.Request) routing.Result {
		calls++
		priority := 0
		for _, net := range candidate.Nets {
			if net.Name == "A" {
				priority = net.Priority
				break
			}
		}
		if priority >= 13 {
			return routing.Result{Status: routing.StatusRouted, Metrics: routing.Metrics{RoutedNetCount: len(candidate.Nets)}}
		}
		return routing.Result{
			Status:  routing.StatusPartial,
			Metrics: routing.Metrics{RoutedNetCount: len(candidate.Nets) - 1, FailedNetCount: 1},
			Issues:  []reports.Issue{{Severity: reports.SeverityBlocked, Nets: []string{"A"}}},
		}
	}

	result, summary := routeWithFailedNetFirstNegotiationUsing(context.Background(), request, route)
	if result.Status != routing.StatusRouted || calls != 14 || summary.Attempts != 14 {
		t.Fatalf("result=%#v calls=%d summary=%#v", result, calls, summary)
	}
}
