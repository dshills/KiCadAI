package designworkflow

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type PhysicalEndpointKind string

const (
	PhysicalEndpointFootprintPad            PhysicalEndpointKind = "footprint_pad"
	PhysicalEndpointBoardEdgePoint          PhysicalEndpointKind = "board_edge_point"
	PhysicalEndpointImportedMechanicalPoint PhysicalEndpointKind = "imported_mechanical_point"
)

type PhysicalEndpointConfidence string

const (
	PhysicalEndpointConfidenceHigh   PhysicalEndpointConfidence = "high"
	PhysicalEndpointConfidenceMedium PhysicalEndpointConfidence = "medium"
	PhysicalEndpointConfidenceLow    PhysicalEndpointConfidence = "low"
)

type AnchorBindingStatus string

const (
	AnchorBindingStatusBound       AnchorBindingStatus = "bound"
	AnchorBindingStatusUnbound     AnchorBindingStatus = "unbound"
	AnchorBindingStatusAmbiguous   AnchorBindingStatus = "ambiguous"
	AnchorBindingStatusInvalid     AnchorBindingStatus = "invalid"
	AnchorBindingStatusUnsupported AnchorBindingStatus = "unsupported"
)

type AnchorRouteStatus string

const (
	AnchorRouteStatusRouted      AnchorRouteStatus = "routed"
	AnchorRouteStatusRequested   AnchorRouteStatus = "route_requested"
	AnchorRouteStatusNotRoutable AnchorRouteStatus = "not_routable"
	AnchorRouteStatusSkipped     AnchorRouteStatus = "skipped"
)

type AnchorBindingPolicy string

const (
	AnchorBindingPolicyRequired    AnchorBindingPolicy = "required"
	AnchorBindingPolicyOptional    AnchorBindingPolicy = "optional"
	AnchorBindingPolicyAdvisory    AnchorBindingPolicy = "advisory"
	AnchorBindingPolicyUnsupported AnchorBindingPolicy = "unsupported"
)

type AnchorBindingIssueCategory string

const (
	AnchorBindingIssueMissingAnchor            AnchorBindingIssueCategory = "missing_anchor"
	AnchorBindingIssueMissingEndpoint          AnchorBindingIssueCategory = "missing_endpoint"
	AnchorBindingIssueAmbiguousEndpoint        AnchorBindingIssueCategory = "ambiguous_endpoint"
	AnchorBindingIssueMissingEndpointPoint     AnchorBindingIssueCategory = "missing_endpoint_point"
	AnchorBindingIssueNetMismatch              AnchorBindingIssueCategory = "net_mismatch"
	AnchorBindingIssueRoleMismatch             AnchorBindingIssueCategory = "role_mismatch"
	AnchorBindingIssueRouteMissing             AnchorBindingIssueCategory = "route_missing"
	AnchorBindingIssueUnsupportedEndpointKind  AnchorBindingIssueCategory = "unsupported_endpoint_kind"
	AnchorBindingIssueRequiresLayerTransition  AnchorBindingIssueCategory = "requires_layer_transition"
	AnchorBindingIssueWidthPadIncompatibility  AnchorBindingIssueCategory = "width_pad_incompatibility"
	AnchorBindingIssueEquivalentEndpointChosen AnchorBindingIssueCategory = "equivalent_endpoint_chosen"
	AnchorBindingIssueSoftNetAlias             AnchorBindingIssueCategory = "soft_net_alias"
)

type PhysicalEndpoint struct {
	ID         string                     `json:"id"`
	Kind       PhysicalEndpointKind       `json:"kind"`
	Ref        string                     `json:"ref,omitempty"`
	Pad        string                     `json:"pad,omitempty"`
	NetName    string                     `json:"net_name,omitempty"`
	Layers     []string                   `json:"layers,omitempty"`
	Roles      []string                   `json:"roles,omitempty"`
	Point      *transactions.Point        `json:"point,omitempty"`
	Source     string                     `json:"source,omitempty"`
	Confidence PhysicalEndpointConfidence `json:"confidence,omitempty"`
	Issues     []AnchorBindingIssue       `json:"issues,omitempty"`
}

type AnchorBinding struct {
	ID                    string               `json:"id"`
	BlockInstanceID       string               `json:"block_instance_id,omitempty"`
	AnchorID              string               `json:"anchor_id,omitempty"`
	AnchorPort            string               `json:"anchor_port,omitempty"`
	AnchorNetName         string               `json:"anchor_net_name,omitempty"`
	AnchorPoint           *transactions.Point  `json:"anchor_point,omitempty"`
	AnchorLayers          []string             `json:"anchor_layers,omitempty"`
	EndpointID            string               `json:"endpoint_id,omitempty"`
	EndpointKind          PhysicalEndpointKind `json:"endpoint_kind,omitempty"`
	EndpointRef           string               `json:"endpoint_ref,omitempty"`
	EndpointPad           string               `json:"endpoint_pad,omitempty"`
	EndpointNetName       string               `json:"endpoint_net_name,omitempty"`
	EndpointLayers        []string             `json:"endpoint_layers,omitempty"`
	EndpointPoint         *transactions.Point  `json:"endpoint_point,omitempty"`
	Status                AnchorBindingStatus  `json:"status"`
	Required              bool                 `json:"required"`
	Policy                AnchorBindingPolicy  `json:"policy,omitempty"`
	RouteStatus           AnchorRouteStatus    `json:"route_status,omitempty"`
	DistanceMM            float64              `json:"distance_mm,omitempty"`
	IssueIDs              []string             `json:"issue_ids,omitempty"`
	EquivalentEndpointIDs []string             `json:"equivalent_endpoint_ids,omitempty"`
}

type AnchorBindingIssue struct {
	ID              string                     `json:"id"`
	Severity        reports.Severity           `json:"severity"`
	Category        AnchorBindingIssueCategory `json:"category"`
	BlockInstanceID string                     `json:"block_instance_id,omitempty"`
	AnchorID        string                     `json:"anchor_id,omitempty"`
	EndpointID      string                     `json:"endpoint_id,omitempty"`
	Message         string                     `json:"message"`
	RepairHint      string                     `json:"repair_hint,omitempty"`
}

type AnchorBindingSummary struct {
	Total          int                  `json:"total"`
	Bound          int                  `json:"bound"`
	Unbound        int                  `json:"unbound,omitempty"`
	Ambiguous      int                  `json:"ambiguous,omitempty"`
	Invalid        int                  `json:"invalid,omitempty"`
	Unsupported    int                  `json:"unsupported,omitempty"`
	Required       int                  `json:"required,omitempty"`
	Routed         int                  `json:"routed,omitempty"`
	RouteRequested int                  `json:"route_requested,omitempty"`
	NotRoutable    int                  `json:"not_routable,omitempty"`
	SkippedRoutes  int                  `json:"skipped_routes,omitempty"`
	IssueCount     int                  `json:"issue_count,omitempty"`
	BlockingIssues int                  `json:"blocking_issues,omitempty"`
	ErrorIssues    int                  `json:"error_issues,omitempty"`
	WarningIssues  int                  `json:"warning_issues,omitempty"`
	InfoIssues     int                  `json:"info_issues,omitempty"`
	Bindings       []AnchorBinding      `json:"bindings,omitempty"`
	Issues         []AnchorBindingIssue `json:"issues,omitempty"`
}

func SummarizeAnchorBindings(bindings []AnchorBinding, issues []AnchorBindingIssue) AnchorBindingSummary {
	summary := AnchorBindingSummary{
		Total: len(bindings),
	}
	for _, binding := range bindings {
		switch binding.Status {
		case AnchorBindingStatusBound:
			summary.Bound++
		case AnchorBindingStatusAmbiguous:
			summary.Ambiguous++
		case AnchorBindingStatusInvalid:
			summary.Invalid++
		case AnchorBindingStatusUnsupported:
			summary.Unsupported++
		default:
			summary.Unbound++
		}
		if AnchorBindingRequired(binding.Required, binding.Policy) {
			summary.Required++
		}
		switch binding.RouteStatus {
		case AnchorRouteStatusRouted:
			summary.Routed++
		case AnchorRouteStatusRequested:
			summary.RouteRequested++
		case AnchorRouteStatusNotRoutable:
			summary.NotRoutable++
		case AnchorRouteStatusSkipped:
			summary.SkippedRoutes++
		}
	}
	for _, issue := range issues {
		summary.IssueCount++
		switch issue.Severity {
		case reports.SeverityWarning:
			summary.WarningIssues++
		case reports.SeverityError:
			summary.ErrorIssues++
		case reports.SeverityBlocked:
			summary.BlockingIssues++
		case reports.SeverityInfo:
			summary.InfoIssues++
		default:
			summary.WarningIssues++
		}
	}
	summary.Bindings = cloneAnchorBindings(bindings)
	summary.Issues = cloneAnchorBindingIssues(issues)
	return summary
}

func NewAnchorBindingIssue(category AnchorBindingIssueCategory, severity reports.Severity, blockInstanceID string, anchorID string, endpointID string, message string, repairHint string) AnchorBindingIssue {
	category = AnchorBindingIssueCategory(strings.TrimSpace(string(category)))
	id := anchorBindingIssueID(category, blockInstanceID, anchorID, endpointID)
	return AnchorBindingIssue{
		ID:              id,
		Severity:        severity,
		Category:        category,
		BlockInstanceID: strings.TrimSpace(blockInstanceID),
		AnchorID:        strings.TrimSpace(anchorID),
		EndpointID:      strings.TrimSpace(endpointID),
		Message:         strings.TrimSpace(message),
		RepairHint:      strings.TrimSpace(repairHint),
	}
}

func AnchorBindingIssuesToReports(pathPrefix string, issues []AnchorBindingIssue) []reports.Issue {
	out := make([]reports.Issue, 0, len(issues))
	pathPrefix = strings.TrimSpace(pathPrefix)
	if pathPrefix == "" {
		pathPrefix = "anchor_bindings"
	}
	for _, issue := range issues {
		path := pathPrefix
		if issue.AnchorID != "" {
			path += "." + issue.AnchorID
		}
		if issue.Category != "" {
			path += "." + string(issue.Category)
		}
		out = append(out, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   issue.Severity,
			Path:       path,
			Message:    issue.Message,
			Refs:       nonEmptyStrings(issue.BlockInstanceID, issue.EndpointID),
			Suggestion: issue.RepairHint,
		})
	}
	return out
}

func AnchorBindingRequired(required bool, policy AnchorBindingPolicy) bool {
	return required || policy == AnchorBindingPolicyRequired
}

func RequiredAnchorBindingIssueSeverity(required bool, policy AnchorBindingPolicy, status AnchorBindingStatus, routeStatus AnchorRouteStatus) reports.Severity {
	if !AnchorBindingRequired(required, policy) {
		return reports.SeverityInfo
	}
	switch status {
	case AnchorBindingStatusBound:
		if routeStatus == AnchorRouteStatusNotRoutable {
			return reports.SeverityError
		}
		return reports.SeverityInfo
	default:
		return reports.SeverityError
	}
}

func cloneAnchorBindings(bindings []AnchorBinding) []AnchorBinding {
	if len(bindings) == 0 {
		return nil
	}
	out := append([]AnchorBinding(nil), bindings...)
	for i := range out {
		if out[i].AnchorPoint != nil {
			point := *out[i].AnchorPoint
			out[i].AnchorPoint = &point
		}
		if out[i].EndpointPoint != nil {
			point := *out[i].EndpointPoint
			out[i].EndpointPoint = &point
		}
		out[i].AnchorLayers = append([]string(nil), bindings[i].AnchorLayers...)
		out[i].EndpointLayers = append([]string(nil), bindings[i].EndpointLayers...)
		out[i].IssueIDs = append([]string(nil), bindings[i].IssueIDs...)
		out[i].EquivalentEndpointIDs = append([]string(nil), bindings[i].EquivalentEndpointIDs...)
	}
	return out
}

func cloneAnchorBindingIssues(issues []AnchorBindingIssue) []AnchorBindingIssue {
	if len(issues) == 0 {
		return nil
	}
	return append([]AnchorBindingIssue(nil), issues...)
}

func anchorBindingIssueID(category AnchorBindingIssueCategory, blockInstanceID string, anchorID string, endpointID string) string {
	parts := nonEmptyStrings("anchor_binding", string(category), blockInstanceID, anchorID, endpointID)
	return strings.Join(parts, ".")
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func sortAnchorBindings(bindings []AnchorBinding) {
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].BlockInstanceID != bindings[j].BlockInstanceID {
			return bindings[i].BlockInstanceID < bindings[j].BlockInstanceID
		}
		if bindings[i].AnchorID != bindings[j].AnchorID {
			return bindings[i].AnchorID < bindings[j].AnchorID
		}
		if bindings[i].EndpointRef != bindings[j].EndpointRef {
			return bindings[i].EndpointRef < bindings[j].EndpointRef
		}
		if bindings[i].EndpointPad != bindings[j].EndpointPad {
			return bindings[i].EndpointPad < bindings[j].EndpointPad
		}
		return bindings[i].ID < bindings[j].ID
	})
}

func sortAnchorBindingIssues(issues []AnchorBindingIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].ID < issues[j].ID
	})
}

func physicalEndpointID(kind PhysicalEndpointKind, ref string, pad string) string {
	return physicalEndpointOccurrenceID(kind, ref, pad, 1)
}

func physicalEndpointOccurrenceID(kind PhysicalEndpointKind, ref string, pad string, occurrence int) string {
	kind = PhysicalEndpointKind(strings.TrimSpace(string(kind)))
	ref = strings.TrimSpace(ref)
	pad = strings.TrimSpace(pad)
	input := "kind=" + string(kind) + "\nref=" + ref + "\npad=" + pad + "\n"
	if occurrence > 1 {
		input += "occurrence=" + strconv.Itoa(occurrence) + "\n"
	}
	sum := sha256.Sum256([]byte(input))
	hash := hex.EncodeToString(sum[:])[:8]
	return string(kind) + ":" + ref + ":" + pad + ":" + hash
}
