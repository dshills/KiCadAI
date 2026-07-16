package designworkflow

import (
	"encoding/json"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type AnchorBindingRouteOptions struct {
	WidthMM float64
	Layer   string
}

const (
	defaultAnchorBindingRouteLayer   = "F.Cu"
	defaultAnchorBindingRouteWidthMM = 0.25
)

// AddAnchorBindingRoutes generates route operations from bound physical
// endpoints to their anchor points and returns updated binding evidence.
func AddAnchorBindingRoutes(summary AnchorBindingSummary, opts AnchorBindingRouteOptions) (AnchorBindingSummary, []transactions.Operation) {
	bindings := cloneAnchorBindings(summary.Bindings)
	issues := cloneAnchorBindingIssues(summary.Issues)
	operations := []transactions.Operation{}
	widthMM := opts.WidthMM
	if widthMM <= 0 {
		widthMM = defaultAnchorBindingRouteWidthMM
	}
	for index := range bindings {
		binding := &bindings[index]
		if binding.Status != AnchorBindingStatusBound {
			continue
		}
		// Optional anchors record placement/topology evidence. They are not a
		// copper contract: materializing an unconnected conceptual point creates
		// a dangling track in the written board.
		if !binding.Required {
			binding.RouteStatus = AnchorRouteStatusSkipped
			continue
		}
		if binding.RouteStatus == AnchorRouteStatusRouted {
			continue
		}
		if binding.AnchorPoint == nil || binding.EndpointPoint == nil {
			binding.RouteStatus = AnchorRouteStatusNotRoutable
			issue := NewAnchorBindingIssue(AnchorBindingIssueMissingEndpointPoint, RequiredAnchorBindingIssueSeverity(binding.Required, binding.Policy, binding.Status, binding.RouteStatus), binding.BlockInstanceID, binding.AnchorID, binding.EndpointID, "anchor binding cannot route without endpoint and anchor coordinates", "provide placed endpoint and anchor coordinates before routing")
			binding.IssueIDs = append(binding.IssueIDs, issue.ID)
			issues = append(issues, issue)
			continue
		}
		anchorNet := strings.TrimSpace(binding.AnchorNetName)
		endpointNet := strings.TrimSpace(binding.EndpointNetName)
		netName := anchorNet
		if netName == "" {
			netName = endpointNet
		}
		if anchorNet != "" && endpointNet != "" && !netNamesMatch(anchorNet, endpointNet) {
			binding.Status = AnchorBindingStatusInvalid
			binding.RouteStatus = AnchorRouteStatusNotRoutable
			issue := NewAnchorBindingIssue(AnchorBindingIssueNetMismatch, RequiredAnchorBindingIssueSeverity(binding.Required, binding.Policy, binding.Status, binding.RouteStatus), binding.BlockInstanceID, binding.AnchorID, binding.EndpointID, "anchor and endpoint nets differ; refusing to route binding", "fix the endpoint net assignment before routing the anchor binding")
			binding.IssueIDs = append(binding.IssueIDs, issue.ID)
			issues = append(issues, issue)
			continue
		}
		if netName == "" {
			binding.RouteStatus = AnchorRouteStatusNotRoutable
			issue := NewAnchorBindingIssue(AnchorBindingIssueRouteMissing, RequiredAnchorBindingIssueSeverity(binding.Required, binding.Policy, binding.Status, binding.RouteStatus), binding.BlockInstanceID, binding.AnchorID, binding.EndpointID, "anchor binding cannot route without a net name", "assign a net to the anchor and endpoint before routing")
			binding.IssueIDs = append(binding.IssueIDs, issue.ID)
			issues = append(issues, issue)
			continue
		}
		layer := firstNonEmpty(opts.Layer, firstString(binding.AnchorLayers), firstString(binding.EndpointLayers), defaultAnchorBindingRouteLayer)
		payload := transactions.RouteOperation{
			Op:      transactions.OpRoute,
			NetName: netName,
			Layer:   layer,
			WidthMM: widthMM,
			Points: []transactions.Point{
				{XMM: binding.EndpointPoint.XMM, YMM: binding.EndpointPoint.YMM},
				{XMM: binding.AnchorPoint.XMM, YMM: binding.AnchorPoint.YMM},
			},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			binding.RouteStatus = AnchorRouteStatusNotRoutable
			issue := NewAnchorBindingIssue(AnchorBindingIssueRouteMissing, reports.SeverityError, binding.BlockInstanceID, binding.AnchorID, binding.EndpointID, "failed to build anchor binding route operation: "+err.Error(), "review anchor binding route payload")
			binding.IssueIDs = append(binding.IssueIDs, issue.ID)
			issues = append(issues, issue)
			continue
		}
		operation := transactions.NewOperation(transactions.OpRoute, raw)
		operation.Index = len(operations)
		operations = append(operations, operation)
		binding.RouteStatus = AnchorRouteStatusRouted
	}
	sortAnchorBindings(bindings)
	sortAnchorBindingIssues(issues)
	return SummarizeAnchorBindings(bindings, issues), operations
}

func firstString(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
