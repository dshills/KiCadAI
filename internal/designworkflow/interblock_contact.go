package designworkflow

import (
	"encoding/json"
	"fmt"
	"math"
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

type InterBlockContactSummary struct {
	ContactsRequired int `json:"contacts_required"`
	ContactsProven   int `json:"contacts_proven"`
	ContactsFailed   int `json:"contacts_failed"`
	ContactMisses    int `json:"contact_misses"`
	NetMismatches    int `json:"net_mismatches"`
	LayerMismatches  int `json:"layer_mismatches"`
	MissingTargets   int `json:"missing_targets"`
	IssueCount       int `json:"issue_count"`
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

func ValidateInterBlockRouteEndpointContacts(candidates []InterBlockRouteCandidate, operations []transactions.Operation, placed *PlacementStageResult) InterBlockContactEvidence {
	evidence := BuildInterBlockContactTargets(candidates, placed)
	targetsByNet := interBlockContactTargetsByNet(evidence.Targets)
	operationsByNet, operationIssues := decodeInterBlockRouteOperations(operations)
	evidence.Issues = append(evidence.Issues, operationIssues...)
	for netName, targets := range targetsByNet {
		routeOperations := operationsByNet[netName]
		if len(routeOperations) == 0 {
			for _, target := range targets {
				proof := contactProofForTarget(target, InterBlockContactMissingTarget, "emit an inter-block route operation for this target net")
				proof.EndpointSide = "target"
				evidence.Proofs = append(evidence.Proofs, proof)
				evidence.Issues = append(evidence.Issues, interBlockContactProofIssue(proof, reports.CodeRouteContactMissingTarget, "inter-block contact target has no emitted route operation"))
			}
			continue
		}
		for _, target := range targets {
			proof := proveContactTarget(target, routeOperations)
			evidence.Proofs = append(evidence.Proofs, proof)
			if proof.Status != InterBlockContactProven {
				evidence.Issues = append(evidence.Issues, interBlockContactProofIssue(proof, contactIssueCode(proof.Status), contactIssueMessage(proof.Status)))
			}
		}
	}
	return evidence
}

func SummarizeInterBlockContacts(evidence InterBlockContactEvidence) InterBlockContactSummary {
	summary := InterBlockContactSummary{ContactsRequired: len(evidence.Targets), IssueCount: len(evidence.Issues)}
	for _, proof := range evidence.Proofs {
		switch proof.Status {
		case InterBlockContactProven:
			summary.ContactsProven++
		case InterBlockContactMiss:
			summary.ContactsFailed++
			summary.ContactMisses++
		case InterBlockContactNetMismatch:
			summary.ContactsFailed++
			summary.NetMismatches++
		case InterBlockContactLayerMismatch:
			summary.ContactsFailed++
			summary.LayerMismatches++
		case InterBlockContactMissingTarget:
			summary.ContactsFailed++
			summary.MissingTargets++
		default:
			summary.ContactsFailed++
		}
	}
	return summary
}

func interBlockConnectedNets(evidence InterBlockContactEvidence, operations []transactions.Operation) map[string]bool {
	connected := map[string]bool{}
	targetsByNet := interBlockContactTargetsByNet(evidence.Targets)
	if len(targetsByNet) == 0 {
		return connected
	}
	operationsByNet, operationIssues := decodeInterBlockRouteOperations(operations)
	if len(operationIssues) != 0 {
		return connected
	}
	for netName, targets := range targetsByNet {
		if len(targets) < 2 {
			continue
		}
		graph := newInterBlockContactGraph(operationsByNet[netName])
		if graph.connectedTargets(targets) {
			connected[netName] = true
		}
	}
	return connected
}

type interBlockContactGraph struct {
	parent []int
	rank   []int
	nodes  []interBlockContactGraphNode
	byKey  map[interBlockContactGraphKey][]int
}

type interBlockContactGraphNode struct {
	Point transactions.Point
	Layer string
}

type interBlockContactGraphKey struct {
	layer string
	x     int64
	y     int64
}

func newInterBlockContactGraph(operations []decodedContactRouteOperation) interBlockContactGraph {
	graph := interBlockContactGraph{byKey: map[interBlockContactGraphKey][]int{}}
	for _, operation := range operations {
		previous := -1
		layer := normalizeContactLayer(operation.Layer)
		for _, point := range operation.Points {
			node := graph.add(point, layer)
			if previous != -1 {
				graph.union(previous, node)
			}
			previous = node
		}
	}
	return graph
}

func (graph *interBlockContactGraph) add(point transactions.Point, layer string) int {
	if existing, ok := graph.nearbyNode(point, layer); ok {
		return existing
	}
	index := len(graph.nodes)
	graph.nodes = append(graph.nodes, interBlockContactGraphNode{Point: point, Layer: layer})
	graph.parent = append(graph.parent, index)
	graph.rank = append(graph.rank, 0)
	key := contactGraphKey(point, layer)
	graph.byKey[key] = append(graph.byKey[key], index)
	return index
}

func (graph *interBlockContactGraph) connectedTargets(targets []InterBlockContactTarget) bool {
	root := -1
	for _, target := range targets {
		node, ok := graph.findTargetNode(target)
		if !ok {
			return false
		}
		nodeRoot := graph.find(node)
		if root == -1 {
			root = nodeRoot
			continue
		}
		if root != nodeRoot {
			return false
		}
	}
	return root != -1
}

func (graph *interBlockContactGraph) findTargetNode(target InterBlockContactTarget) (int, bool) {
	return graph.nearbyNode(target.Point, normalizeContactLayer(target.Layer))
}

func (graph *interBlockContactGraph) nearbyNode(point transactions.Point, layer string) (int, bool) {
	key := contactGraphKey(point, layer)
	best := -1
	bestDistance := math.Inf(1)
	// Bucket size equals the contact tolerance, so any point within tolerance
	// can only live in the same bucket or one of the eight neighboring buckets.
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			candidateKey := interBlockContactGraphKey{layer: key.layer, x: key.x + dx, y: key.y + dy}
			for _, index := range graph.byKey[candidateKey] {
				node := graph.nodes[index]
				distance := pointDistanceMM(node.Point, point)
				if distance < bestDistance {
					bestDistance = distance
					best = index
				}
			}
		}
	}
	return best, best != -1 && bestDistance <= interBlockContactToleranceMM
}

func (graph *interBlockContactGraph) find(index int) int {
	for graph.parent[index] != index {
		graph.parent[index] = graph.parent[graph.parent[index]]
		index = graph.parent[index]
	}
	return index
}

func (graph *interBlockContactGraph) union(left int, right int) {
	leftRoot := graph.find(left)
	rightRoot := graph.find(right)
	if leftRoot == rightRoot {
		return
	}
	if graph.rank[leftRoot] < graph.rank[rightRoot] {
		graph.parent[leftRoot] = rightRoot
		return
	}
	if graph.rank[leftRoot] > graph.rank[rightRoot] {
		graph.parent[rightRoot] = leftRoot
		return
	}
	graph.parent[rightRoot] = leftRoot
	graph.rank[leftRoot]++
}

func contactGraphKey(point transactions.Point, layer string) interBlockContactGraphKey {
	return interBlockContactGraphKey{
		layer: layer,
		x:     int64(math.Round(point.XMM / interBlockContactToleranceMM)),
		y:     int64(math.Round(point.YMM / interBlockContactToleranceMM)),
	}
}

func normalizeContactLayer(layer string) string {
	return strings.ToUpper(strings.TrimSpace(layer))
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

type decodedContactRouteOperation struct {
	OperationID string
	NetName     string
	Layer       string
	Points      []transactions.Point
}

func decodeInterBlockRouteOperations(operations []transactions.Operation) (map[string][]decodedContactRouteOperation, []reports.Issue) {
	byNet := map[string][]decodedContactRouteOperation{}
	var issues []reports.Issue
	for index, operation := range operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		var payload transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			issues = append(issues, reports.Issue{
				Code:        reports.CodeRouteContactUnsupported,
				Severity:    reports.SeverityBlocked,
				Path:        fmt.Sprintf("design.inter_block_contact.operations[%d]", index),
				Message:     "route operation could not be decoded for contact validation: " + err.Error(),
				OperationID: contactOperationID(operation),
			})
			continue
		}
		netName := strings.TrimSpace(operation.Net)
		if netName == "" {
			netName = strings.TrimSpace(payload.NetName)
		}
		if netName == "" {
			issues = append(issues, reports.Issue{
				Code:        reports.CodeRouteContactNetMismatch,
				Severity:    reports.SeverityBlocked,
				Path:        fmt.Sprintf("design.inter_block_contact.operations[%d].net_name", index),
				Message:     "route operation has no net name for contact validation",
				OperationID: contactOperationID(operation),
			})
			continue
		}
		payloadNet := strings.TrimSpace(payload.NetName)
		if payloadNet != "" && payloadNet != netName {
			issues = append(issues, reports.Issue{
				Code:        reports.CodeRouteContactNetMismatch,
				Severity:    reports.SeverityBlocked,
				Path:        fmt.Sprintf("design.inter_block_contact.operations[%d].net_name", index),
				Message:     fmt.Sprintf("route operation cached net %q does not match payload net %q", netName, payloadNet),
				Nets:        []string{netName, payloadNet},
				OperationID: contactOperationID(operation),
			})
			continue
		}
		if len(payload.Points) == 0 {
			issues = append(issues, reports.Issue{
				Code:        reports.CodeRouteContactMissingTarget,
				Severity:    reports.SeverityBlocked,
				Path:        fmt.Sprintf("design.inter_block_contact.operations[%d].points", index),
				Message:     "route operation has no points for contact validation",
				Nets:        []string{netName},
				OperationID: contactOperationID(operation),
			})
			continue
		}
		byNet[netName] = append(byNet[netName], decodedContactRouteOperation{
			OperationID: contactOperationID(operation),
			NetName:     netName,
			Layer:       strings.TrimSpace(payload.Layer),
			Points:      append([]transactions.Point(nil), payload.Points...),
		})
	}
	return byNet, issues
}

func proveContactTarget(target InterBlockContactTarget, operations []decodedContactRouteOperation) InterBlockContactProof {
	best := InterBlockContactProof{
		RouteClass:  "inter_block",
		NetName:     target.NetName,
		NetCode:     target.NetCode,
		EndpointSide: "target",
		Target:      target,
		ToleranceMM: target.ToleranceMM,
		Status:      InterBlockContactMiss,
		Blocking:    true,
		Suggestion:  "snap the route endpoint to the resolved contact target",
	}
	bestDistance := math.Inf(1)
	layerCoordinateMatch := false
	for _, operation := range operations {
		if operation.NetName != target.NetName {
			continue
		}
		if len(operation.Points) == 0 {
			continue
		}
		endpoints := []struct {
			side  string
			point transactions.Point
		}{
			{side: "start", point: operation.Points[0]},
			{side: "end", point: operation.Points[len(operation.Points)-1]},
		}
		for _, endpoint := range endpoints {
			distance := pointDistanceMM(endpoint.point, target.Point)
			if distance <= target.ToleranceMM && !sameLayer(operation.Layer, target.Layer) {
				layerCoordinateMatch = true
			}
			if distance < bestDistance {
				bestDistance = distance
				point := endpoint.point
				best.OperationID = operation.OperationID
				best.EndpointSide = endpoint.side
				best.EmittedPoint = &point
				best.Layer = operation.Layer
				best.DistanceMM = distance
			}
			if distance <= target.ToleranceMM && sameLayer(operation.Layer, target.Layer) {
				point := endpoint.point
				return InterBlockContactProof{
					OperationID:  operation.OperationID,
					RouteClass:   "inter_block",
					NetName:      target.NetName,
					NetCode:      target.NetCode,
					EndpointSide: endpoint.side,
					EmittedPoint: &point,
					Layer:        operation.Layer,
					Target:       target,
					DistanceMM:   distance,
					ToleranceMM:  target.ToleranceMM,
					Status:       InterBlockContactProven,
				}
			}
		}
	}
	if layerCoordinateMatch {
		best.Status = InterBlockContactLayerMismatch
		best.Suggestion = "route to the contact target on the target copper layer or insert a validated via"
	}
	return best
}

func interBlockContactTargetsByNet(targets []InterBlockContactTarget) map[string][]InterBlockContactTarget {
	byNet := map[string][]InterBlockContactTarget{}
	for _, target := range targets {
		byNet[target.NetName] = append(byNet[target.NetName], target)
	}
	return byNet
}

func interBlockContactProofIssue(proof InterBlockContactProof, code reports.Code, message string) reports.Issue {
	path := proof.Target.Path
	if proof.EndpointSide != "" {
		path += "." + proof.EndpointSide
	}
	return reports.Issue{
		Code:        code,
		Severity:    reports.SeverityBlocked,
		Path:        path,
		Message:     message,
		Refs:        compactContactStrings([]string{proof.Target.Ref}),
		Nets:        compactContactStrings([]string{proof.NetName}),
		Suggestion:  proof.Suggestion,
		OperationID: proof.OperationID,
	}
}

func contactIssueCode(status InterBlockContactProofStatus) reports.Code {
	switch status {
	case InterBlockContactNetMismatch:
		return reports.CodeRouteContactNetMismatch
	case InterBlockContactLayerMismatch:
		return reports.CodeRouteContactLayerMismatch
	case InterBlockContactMissingTarget:
		return reports.CodeRouteContactMissingTarget
	case InterBlockContactUnsupportedGeometry:
		return reports.CodeRouteContactUnsupported
	case InterBlockContactAmbiguous:
		return reports.CodeRouteContactAmbiguous
	default:
		return reports.CodeRouteContactMiss
	}
}

func contactIssueMessage(status InterBlockContactProofStatus) string {
	switch status {
	case InterBlockContactLayerMismatch:
		return "route endpoint reaches the contact coordinate on the wrong layer"
	case InterBlockContactMissingTarget:
		return "route endpoint contact target is missing"
	case InterBlockContactNetMismatch:
		return "route endpoint net does not match contact target net"
	default:
		return "route endpoint does not contact the required same-net target"
	}
}

func sameLayer(left string, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func contactOperationID(operation transactions.Operation) string {
	return fmt.Sprintf("route:%d", operation.Index)
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
