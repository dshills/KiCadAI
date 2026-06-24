package designworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestSummarizeAnchorBindingsCountsStatusesAndIssues(t *testing.T) {
	bindings := []AnchorBinding{
		{ID: "b1", Status: AnchorBindingStatusBound, Policy: AnchorBindingPolicyRequired, RouteStatus: AnchorRouteStatusRouted},
		{ID: "b2", Status: AnchorBindingStatusAmbiguous, Required: true, RouteStatus: AnchorRouteStatusNotRoutable},
		{ID: "b3", Status: AnchorBindingStatusUnsupported, Policy: AnchorBindingPolicyUnsupported, RouteStatus: AnchorRouteStatusSkipped},
	}
	issues := []AnchorBindingIssue{
		NewAnchorBindingIssue(AnchorBindingIssueAmbiguousEndpoint, reports.SeverityError, "esd1", "signal_entry", "", "ambiguous endpoint", "select a connector pad"),
		NewAnchorBindingIssue(AnchorBindingIssueEquivalentEndpointChosen, reports.SeverityInfo, "esd1", "gnd_entry", "J1:1", "selected equivalent endpoint", ""),
		NewAnchorBindingIssue(AnchorBindingIssueRouteMissing, reports.SeverityWarning, "rp1", "vin_entry", "", "route missing", ""),
	}

	summary := SummarizeAnchorBindings(bindings, issues)

	if summary.Total != 3 || summary.Bound != 1 || summary.Ambiguous != 1 || summary.Unsupported != 1 || summary.Required != 2 {
		t.Fatalf("status summary = %#v", summary)
	}
	if summary.Routed != 1 || summary.NotRoutable != 1 || summary.SkippedRoutes != 1 {
		t.Fatalf("route summary = %#v", summary)
	}
	if summary.IssueCount != 3 || summary.ErrorIssues != 1 || summary.WarningIssues != 1 || summary.InfoIssues != 1 {
		t.Fatalf("issue summary = %#v", summary)
	}
}

func TestAnchorBindingJSONShape(t *testing.T) {
	point := transactions.Point{XMM: 10, YMM: 12}
	summary := SummarizeAnchorBindings([]AnchorBinding{{
		ID:              "esd1.signal_entry",
		BlockInstanceID: "esd1",
		AnchorID:        "signal_entry",
		AnchorPort:      "SIG",
		AnchorNetName:   "USB_D+",
		AnchorPoint:     &point,
		AnchorLayers:    []string{"F.Cu"},
		EndpointID:      "footprint_pad:J1:A6:9f3a12c0",
		EndpointKind:    PhysicalEndpointFootprintPad,
		EndpointRef:     "J1",
		EndpointPad:     "A6",
		EndpointNetName: "USB_D+",
		EndpointLayers:  []string{"F.Cu"},
		EndpointPoint:   &point,
		Status:          AnchorBindingStatusBound,
		Required:        true,
		Policy:          AnchorBindingPolicyRequired,
		RouteStatus:     AnchorRouteStatusRouted,
		DistanceMM:      2.5,
	}}, nil)

	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	text := string(payload)
	for _, want := range []string{
		`"endpoint_id":"footprint_pad:J1:A6:9f3a12c0"`,
		`"endpoint_layers":["F.Cu"]`,
		`"route_status":"routed"`,
		`"required":true`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("JSON %s missing %s", text, want)
		}
	}
}

func TestAnchorBindingIssuesToReports(t *testing.T) {
	issues := []AnchorBindingIssue{
		NewAnchorBindingIssue(AnchorBindingIssueMissingEndpoint, reports.SeverityError, "esd1", "signal_entry", "", "missing endpoint", "add connector"),
	}

	reportIssues := AnchorBindingIssuesToReports("design.anchor_bindings", issues)

	if len(reportIssues) != 1 {
		t.Fatalf("report issues = %#v", reportIssues)
	}
	issue := reportIssues[0]
	if issue.Path != "design.anchor_bindings.signal_entry.missing_endpoint" || issue.Suggestion != "add connector" || issue.Code != reports.CodeValidationFailed {
		t.Fatalf("report issue = %#v", issue)
	}
	if len(issue.Refs) != 1 || issue.Refs[0] != "esd1" {
		t.Fatalf("refs = %#v", issue.Refs)
	}
}
