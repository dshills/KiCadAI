package routing

import (
	"encoding/json"
	"math"

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
	for _, route := range result.Routes {
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
				issues = append(issues, operationIssue(route.Net, "route segment operation contains invalid numeric values"))
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
				issues = append(issues, operationIssue(route.Net, "route via operation contains invalid numeric values"))
			}
		}
	}
	return operations, issues
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
