package designworkflow

import (
	"fmt"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

// InterBlockContactTargetKind identifies the physical object a generated
// inter-block route is allowed to touch.
type InterBlockContactTargetKind string

const (
	InterBlockContactTargetPad           InterBlockContactTargetKind = "pad"
	InterBlockContactTargetAccessPoint   InterBlockContactTargetKind = "access_point"
	InterBlockContactTargetVia           InterBlockContactTargetKind = "via"
	InterBlockContactTargetTrackEndpoint InterBlockContactTargetKind = "track_endpoint"
	InterBlockContactTargetSameNetCopper InterBlockContactTargetKind = "same_net_copper"
)

// InterBlockContactConfidence reports how directly a contact target was
// derived from physical placement and library evidence.
type InterBlockContactConfidence string

const (
	InterBlockContactConfidenceHigh    InterBlockContactConfidence = "high"
	InterBlockContactConfidenceMedium  InterBlockContactConfidence = "medium"
	InterBlockContactConfidenceBlocked InterBlockContactConfidence = "blocked"
)

// InterBlockContactProofStatus is the normalized result of comparing route
// copper against a contact target.
type InterBlockContactProofStatus string

const (
	InterBlockContactProven               InterBlockContactProofStatus = "proven"
	InterBlockContactMiss                 InterBlockContactProofStatus = "miss"
	InterBlockContactNetMismatch          InterBlockContactProofStatus = "net_mismatch"
	InterBlockContactLayerMismatch        InterBlockContactProofStatus = "layer_mismatch"
	InterBlockContactMissingTarget        InterBlockContactProofStatus = "missing_target"
	InterBlockContactUnsupportedGeometry  InterBlockContactProofStatus = "unsupported_geometry"
	InterBlockContactAmbiguous            InterBlockContactProofStatus = "ambiguous"
)

// InterBlockContactTarget is a same-net physical endpoint or access point that
// an inter-block route may use for electrical contact.
type InterBlockContactTarget struct {
	NetName       string                         `json:"net_name"`
	NetCode       int                            `json:"net_code"`
	Kind          InterBlockContactTargetKind    `json:"kind"`
	Ref           string                         `json:"ref,omitempty"`
	Pad           string                         `json:"pad,omitempty"`
	InstanceID    string                         `json:"instance_id,omitempty"`
	BlockID       string                         `json:"block_id,omitempty"`
	Point         transactions.Point             `json:"point"`
	Layer         string                         `json:"layer,omitempty"`
	ToleranceMM   float64                        `json:"tolerance_mm,omitempty"`
	GeometrySource string                        `json:"geometry_source,omitempty"`
	Confidence    InterBlockContactConfidence   `json:"confidence"`
	Path          string                         `json:"path,omitempty"`
}

// InterBlockContactProof records whether one emitted route endpoint contacts a
// required inter-block target.
type InterBlockContactProof struct {
	OperationID   string                       `json:"operation_id,omitempty"`
	RouteClass    string                       `json:"route_class"`
	NetName       string                       `json:"net_name"`
	NetCode       int                          `json:"net_code"`
	EndpointSide  string                       `json:"endpoint_side,omitempty"`
	EmittedPoint  *transactions.Point          `json:"emitted_point,omitempty"`
	Layer         string                       `json:"layer,omitempty"`
	Target        InterBlockContactTarget      `json:"target"`
	DistanceMM    float64                      `json:"distance_mm,omitempty"`
	ToleranceMM   float64                      `json:"tolerance_mm,omitempty"`
	Status        InterBlockContactProofStatus `json:"status"`
	Blocking      bool                         `json:"blocking,omitempty"`
	Suggestion    string                       `json:"suggestion,omitempty"`
}

// InterBlockContactEvidence bundles resolved targets, proof records, and
// blocking diagnostics for inter-block route completion. Target resolution
// populates Targets and Issues; route-emission validation appends Proofs after
// there is emitted copper to compare with those targets.
type InterBlockContactEvidence struct {
	Targets []InterBlockContactTarget `json:"targets,omitempty"`
	Proofs  []InterBlockContactProof  `json:"proofs,omitempty"`
	Issues  []reports.Issue           `json:"issues,omitempty"`
}

// interBlockContactToleranceMM is a geometry-proof tolerance for generated
// endpoint contact, not a manufacturing clearance. It allows writer/reader
// coordinate rounding while still requiring the route to terminate at the
// intended pad/access target.
const interBlockContactToleranceMM = 1e-4

// BuildInterBlockContactTargets resolves route-candidate endpoints into
// physical contact targets using placed, hydrated pad evidence. It does not
// populate Proofs because contact proof requires emitted route geometry.
func BuildInterBlockContactTargets(candidates []InterBlockRouteCandidate, placed *PlacementStageResult) InterBlockContactEvidence {
	if placed == nil {
		return InterBlockContactEvidence{Issues: []reports.Issue{interBlockContactIssue(
			"design.inter_block_contact.placement",
			"placement result is required for inter-block contact target resolution",
			nil,
			nil,
			"run placement before proving inter-block route contacts",
		)}}
	}
	if placed.Result.Status != placement.StatusPlaced {
		return InterBlockContactEvidence{Issues: []reports.Issue{interBlockContactIssue(
			"design.inter_block_contact.placement.status",
			"placement result must be placed before inter-block contact target resolution",
			nil,
			nil,
			"complete placement before proving inter-block route contacts",
		)}}
	}
	table, tableIssues := BuildGeneratedNetTable(placed, nil)
	resolver := NewPlacedPadEndpointResolver(placed, table)
	evidence := InterBlockContactEvidence{}
	evidence.Issues = append(evidence.Issues, tableIssues...)
	evidence.Issues = append(evidence.Issues, resolver.Issues()...)
	for candidateIndex, candidate := range candidates {
		routeNet := strings.TrimSpace(candidate.NetName)
		for endpointIndex, endpoint := range candidate.Endpoints {
			path := fmt.Sprintf("design.inter_block_contact.nets[%d].endpoints[%d]", candidateIndex, endpointIndex)
			target, ok, issue := interBlockContactTarget(path, routeNet, endpoint, &resolver)
			if issue != nil {
				evidence.Issues = append(evidence.Issues, *issue)
			}
			if ok {
				evidence.Targets = append(evidence.Targets, target)
			}
		}
	}
	return evidence
}

func interBlockContactTarget(path string, netName string, endpoint InterBlockRouteEndpoint, resolver *PlacedPadEndpointResolver) (InterBlockContactTarget, bool, *reports.Issue) {
	ref := strings.TrimSpace(endpoint.Ref)
	pin := strings.TrimSpace(endpoint.Pin)
	if ref == "" || pin == "" {
		issue := interBlockContactIssue(path, "inter-block contact endpoint requires ref and pin", nil, []string{netName}, "provide generated endpoint ref and pin evidence before routing")
		return InterBlockContactTarget{}, false, &issue
	}
	resolved, ok := resolver.Resolve(transactions.Endpoint{Ref: ref, Pin: pin})
	if !ok {
		issue := interBlockContactIssue(path, "inter-block contact target does not resolve to a placed pad", []string{ref}, []string{netName}, "verify footprint pad geometry and placement for "+ref+"."+pin)
		return InterBlockContactTarget{}, false, &issue
	}
	if !resolved.NetCodeResolved {
		issue := interBlockContactIssue(path+".net_code", "inter-block contact target net code is unresolved", []string{ref}, []string{netName}, "assign the pad net before proving route contact")
		return InterBlockContactTarget{}, false, &issue
	}
	// KiCad preserves net-name case in files, so contact proof must not merge
	// names that differ only by case.
	if strings.TrimSpace(resolved.NetName) != netName {
		issue := interBlockContactIssue(path+".net_name", fmt.Sprintf("inter-block contact target pad net %q does not match route net %q", resolved.NetName, netName), []string{ref}, []string{netName, resolved.NetName}, "repair net assignment before routing between these endpoints")
		return InterBlockContactTarget{}, false, &issue
	}
	// Future phases may derive access-point, via, track-endpoint, and
	// same-net-copper targets from validated route geometry. Phase 2 only
	// resolves placed physical pads.
	return InterBlockContactTarget{
		NetName:        resolved.NetName,
		NetCode:        resolved.NetCode,
		Kind:           InterBlockContactTargetPad,
		Ref:            resolved.Ref,
		Pad:            resolved.Pad,
		InstanceID:     endpoint.InstanceID,
		BlockID:        endpoint.BlockID,
		Point:          resolved.Point,
		Layer:          resolved.Layer,
		ToleranceMM:    interBlockContactToleranceMM,
		GeometrySource: resolved.Source,
		Confidence:     InterBlockContactConfidenceHigh,
		Path:           path,
	}, true, nil
}

func interBlockContactIssue(path string, message string, refs []string, nets []string, suggestion string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityBlocked,
		Path:       path,
		Message:    message,
		Refs:       append([]string(nil), refs...),
		Nets:       compactContactStrings(nets),
		Suggestion: suggestion,
	}
}

func contactProofForTarget(target InterBlockContactTarget, status InterBlockContactProofStatus, suggestion string) InterBlockContactProof {
	return InterBlockContactProof{
		RouteClass:  "inter_block",
		NetName:     target.NetName,
		NetCode:     target.NetCode,
		Target:      target,
		ToleranceMM: target.ToleranceMM,
		Status:      status,
		Blocking:    status != InterBlockContactProven,
		Suggestion:  suggestion,
	}
}

func compactContactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
