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
