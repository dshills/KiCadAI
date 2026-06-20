package routing

import (
	"sort"

	"kicadai/internal/reports"
)

func BuildQualityReport(request Request, result Result) QualityReport {
	netByName := map[string]Net{}
	for _, net := range request.Nets {
		netByName[net.Name] = net
	}
	routeByNet := map[string]Route{}
	for _, route := range result.Routes {
		routeByNet[route.Net] = route
	}
	netNames := make([]string, 0, len(netByName))
	for name := range netByName {
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)
	report := QualityReport{Status: result.Status}
	for _, netName := range netNames {
		net := netByName[netName]
		route, ok := routeByNet[netName]
		if !ok {
			report.NetReports = append(report.NetReports, NetQualityReport{
				NetName:         netName,
				Role:            net.Role,
				Class:           net.Class,
				EndpointCount:   len(net.Endpoints),
				Status:          RouteStatusSkipped,
				FailureCategory: "skipped",
				SuggestedRepair: "include the net in route planning",
			})
			continue
		}
		netReport := NetQualityReport{
			NetName:         netName,
			Role:            net.Role,
			Class:           net.Class,
			EndpointCount:   len(net.Endpoints),
			Status:          route.Status,
			SegmentCount:    len(route.Segments),
			ViaCount:        len(route.Vias),
			LengthMM:        routeLength(route),
			Layers:          routeLayers(route),
			SearchNodes:     route.SearchNodes,
			SearchLimitHit:  route.SearchLimitHit,
			FailureCategory: failureCategory(route.Issues),
			SuggestedRepair: suggestedRepair(route.Issues),
		}
		if route.Status == RouteStatusRouted {
			netReport.RoutedEndpoints = len(net.Endpoints)
		} else if netReport.SegmentCount > 0 || netReport.ViaCount > 0 {
			netReport.RoutedEndpoints = min(len(net.Endpoints), netReport.SegmentCount+1)
		}
		report.NetReports = append(report.NetReports, netReport)
	}
	report.Score = buildQualityScore(result, report.NetReports)
	return report
}

func buildQualityScore(result Result, nets []NetQualityReport) QualityScore {
	total := len(nets)
	routed := 0
	failed := 0
	searchHits := 0
	lengthScore := 1.0
	for _, net := range nets {
		if net.Status == RouteStatusRouted {
			routed++
		}
		if net.Status == RouteStatusFailed {
			failed++
		}
		if net.SearchLimitHit {
			searchHits++
		}
	}
	completion := ratioScore(routed, total)
	connectivity := ratioScore(total-failed, total)
	searchPressure := 1.0
	if searchHits > 0 && total > 0 {
		searchPressure = ratioScore(total-searchHits, total)
	}
	status := statusScore(result.Status)
	overall := roundScore((completion + connectivity + lengthScore + searchPressure + status) / 5)
	return QualityScore{
		Overall: overall,
		Dimensions: []QualityScoreDimension{
			{Name: "completion", Score: completion, Message: "share of planned nets routed"},
			{Name: "connectivity", Score: connectivity, Message: "share of planned nets without route failure"},
			{Name: "length", Score: lengthScore, Message: "length policy evidence is informational in this phase"},
			{Name: "search_pressure", Score: searchPressure, Message: "penalizes routes that hit search limits"},
			{Name: "result_status", Score: status, Message: string(result.Status)},
		},
	}
}

func ratioScore(numerator int, denominator int) float64 {
	if denominator <= 0 {
		return 1
	}
	return roundScore(float64(numerator) / float64(denominator))
}

func statusScore(status Status) float64 {
	switch status {
	case StatusRouted:
		return 1
	case StatusPartial:
		return 0.5
	default:
		return 0
	}
}

func routeLength(route Route) float64 {
	total := 0.0
	for _, segment := range route.Segments {
		total += pointDistance(segment.Start, segment.End)
	}
	return roundMM(total)
}

func routeLayers(route Route) []string {
	seen := map[string]struct{}{}
	for _, segment := range route.Segments {
		if segment.Layer != "" {
			seen[segment.Layer] = struct{}{}
		}
	}
	for _, via := range route.Vias {
		for _, layer := range via.Layers {
			if layer != "" {
				seen[layer] = struct{}{}
			}
		}
	}
	layers := make([]string, 0, len(seen))
	for layer := range seen {
		layers = append(layers, layer)
	}
	sort.Strings(layers)
	return layers
}

func failureCategory(issues []reports.Issue) string {
	if len(issues) == 0 {
		return ""
	}
	for _, issue := range issues {
		if issue.Code != "" {
			return string(issue.Code)
		}
	}
	return "route_failed"
}

func suggestedRepair(issues []reports.Issue) string {
	for _, issue := range issues {
		if issue.Suggestion != "" {
			return issue.Suggestion
		}
	}
	if len(issues) != 0 {
		return "inspect route diagnostics and adjust placement or routing rules"
	}
	return ""
}

func roundScore(value float64) float64 {
	return float64(int(value*1000+0.5)) / 1000
}
