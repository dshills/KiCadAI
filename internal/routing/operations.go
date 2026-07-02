package routing

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

const routeOperationName = "route"

func OperationsFromResult(result Result) []Operation {
	operations, _ := OperationsFromResultWithIssues(result)
	return operations
}

func OperationsFromResultWithIssues(result Result) ([]Operation, []reports.Issue) {
	operations := make([]Operation, 0, len(result.Routes))
	issues := make([]reports.Issue, 0)
	seenIssues := map[issueKey]struct{}{}
	for _, route := range result.Routes {
		issues = appendUniqueIssues(issues, route.Issues, seenIssues)
		if !routeEligibleForOperations(route) {
			continue
		}
		for _, group := range segmentOperationGroups(route.Segments) {
			payload := RouteOperation{
				Op:      routeOperationName,
				NetName: group.Net,
				Layer:   group.Layer,
				WidthMM: group.WidthMM,
				Points:  group.Points,
			}
			if operation, ok := routeOperation(payload); ok {
				operations = append(operations, operation)
			} else {
				issue := operationIssue(route.Net, "route segment operation contains invalid numeric values")
				issues = appendUniqueIssues(issues, []reports.Issue{issue}, seenIssues)
			}
		}
		if len(route.Vias) != 0 {
			payload := RouteOperation{
				Op:      routeOperationName,
				NetName: route.Net,
				Vias:    make([]RouteViaOperation, 0, len(route.Vias)),
			}
			for _, via := range route.Vias {
				payload.Vias = append(payload.Vias, RouteViaOperation{
					At:         OperationPoint{XMM: via.At.XMM, YMM: via.At.YMM},
					DiameterMM: via.DiameterMM,
					DrillMM:    via.DrillMM,
					Layers:     append([]string(nil), via.Layers...),
				})
			}
			if operation, ok := routeOperation(payload); ok {
				operations = append(operations, operation)
			} else {
				issue := operationIssue(route.Net, "route via operation contains invalid numeric values")
				issues = appendUniqueIssues(issues, []reports.Issue{issue}, seenIssues)
			}
		}
	}
	return operations, issues
}

type issueKey struct {
	Code        reports.Code
	Severity    reports.Severity
	Path        string
	Message     string
	UUIDs       string
	Refs        string
	Nets        string
	Suggestion  string
	OperationID string
}

func appendUniqueIssues(out []reports.Issue, issues []reports.Issue, seen map[issueKey]struct{}) []reports.Issue {
	for _, issue := range issues {
		key := newIssueKey(issue)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, issue)
	}
	return out
}

func newIssueKey(issue reports.Issue) issueKey {
	return issueKey{
		Code:        issue.Code,
		Severity:    issue.Severity,
		Path:        issue.Path,
		Message:     issue.Message,
		UUIDs:       issueKeySlice(issue.UUIDs),
		Refs:        issueKeySlice(issue.Refs),
		Nets:        issueKeySlice(issue.Nets),
		Suggestion:  issue.Suggestion,
		OperationID: issue.OperationID,
	}
}

func issueKeySlice(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return issueKeyPart(values[0])
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	var builder strings.Builder
	for _, value := range out {
		builder.WriteString(issueKeyPart(value))
	}
	return builder.String()
}

func issueKeyPart(value string) string {
	return strconv.Itoa(len(value)) + ":" + value + ";"
}

func routeEligibleForOperations(route Route) bool {
	if reports.HasBlockingIssue(route.Issues) {
		return false
	}
	return route.Status == RouteStatusRouted
}

type segmentOperationGroup struct {
	Net     string
	Layer   string
	WidthMM float64
	Points  []OperationPoint
}

func segmentOperationGroups(segments []Segment) []segmentOperationGroup {
	groups := []segmentOperationGroup{}
	for _, segment := range segments {
		pointStart := OperationPoint{XMM: segment.Start.XMM, YMM: segment.Start.YMM}
		pointEnd := OperationPoint{XMM: segment.End.XMM, YMM: segment.End.YMM}
		if len(groups) == 0 || !segmentContinuesGroup(groups[len(groups)-1], segment) {
			groups = append(groups, segmentOperationGroup{
				Net:     segment.Net,
				Layer:   segment.Layer,
				WidthMM: segment.WidthMM,
				Points:  []OperationPoint{pointStart, pointEnd},
			})
			continue
		}
		groups[len(groups)-1].Points = append(groups[len(groups)-1].Points, pointEnd)
	}
	return groups
}

func segmentContinuesGroup(group segmentOperationGroup, segment Segment) bool {
	if group.Net != segment.Net || group.Layer != segment.Layer || group.WidthMM != segment.WidthMM || len(group.Points) == 0 {
		return false
	}
	last := group.Points[len(group.Points)-1]
	return roundMM(last.XMM) == roundMM(segment.Start.XMM) && roundMM(last.YMM) == roundMM(segment.Start.YMM)
}

func routeOperation(payload RouteOperation) (Operation, bool) {
	if !routeOperationFinite(payload) {
		return Operation{}, false
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Operation{}, false
	}
	return Operation{Op: routeOperationName, Raw: raw}, true
}

func routeOperationFinite(payload RouteOperation) bool {
	for _, point := range payload.Points {
		if !finite(point.XMM) || !finite(point.YMM) {
			return false
		}
	}
	for _, via := range payload.Vias {
		if !finite(via.At.XMM) || !finite(via.At.YMM) || !finite(via.DiameterMM) || !finite(via.DrillMM) {
			return false
		}
	}
	return finite(payload.WidthMM)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func operationIssue(netName string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: message, Nets: []string{netName}}
}
