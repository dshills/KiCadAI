package designworkflow

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

const AutonomousCorrectionSchemaV1 = "kicadai.autonomous-correction.v1"

type AutonomousCorrectionCategory string

const (
	CorrectionComponentOverlap                AutonomousCorrectionCategory = "component_overlap"
	CorrectionInaccessiblePad                 AutonomousCorrectionCategory = "inaccessible_pad"
	CorrectionBlockedEscapeDirection          AutonomousCorrectionCategory = "blocked_escape_direction"
	CorrectionRouteTreeBranchOrder            AutonomousCorrectionCategory = "route_tree_branch_order"
	CorrectionMissingLayerTransition          AutonomousCorrectionCategory = "missing_layer_transition"
	CorrectionSameNetBranchMerge              AutonomousCorrectionCategory = "same_net_branch_merge"
	CorrectionRequiredNetDisconnectedEndpoint AutonomousCorrectionCategory = "required_net_disconnected_endpoint"
	CorrectionRoutingRegionExhaustion         AutonomousCorrectionCategory = "routing_region_exhaustion"
	CorrectionUnsupportedGeometry             AutonomousCorrectionCategory = "unsupported_geometry"
)

type AutonomousCorrectionDiagnostic struct {
	Category          AutonomousCorrectionCategory `json:"category"`
	Source            string                       `json:"source"`
	SourceCategory    routing.RepairCategory       `json:"source_category,omitempty"`
	SourceAction      routing.RepairAction         `json:"source_action,omitempty"`
	IssueCode         reports.Code                 `json:"issue_code"`
	Severity          reports.Severity             `json:"severity"`
	Path              string                       `json:"path,omitempty"`
	Refs              []string                     `json:"refs,omitempty"`
	Nets              []string                     `json:"nets,omitempty"`
	Evidence          []string                     `json:"evidence,omitempty"`
	AutomaticAction   bool                         `json:"automatic_action"`
	UnsupportedReason string                       `json:"unsupported_reason,omitempty"`
}

// BuildAutonomousCorrectionDiagnostics converts subsystem issues into the
// stable correction taxonomy. Messages and suggestions are intentionally not
// copied because provider or external-tool text is not correction evidence.
func BuildAutonomousCorrectionDiagnostics(placementIssues, routingIssues []reports.Issue) []AutonomousCorrectionDiagnostic {
	diagnostics := make([]AutonomousCorrectionDiagnostic, 0, len(placementIssues)+len(routingIssues))
	for _, issue := range placementIssues {
		diagnostics = append(diagnostics, autonomousCorrectionDiagnostic("placement", issue))
	}
	for _, issue := range routingIssues {
		diagnostics = append(diagnostics, autonomousCorrectionDiagnostic("routing", issue))
	}
	diagnostics = dedupeAutonomousCorrectionDiagnostics(diagnostics)
	slices.SortFunc(diagnostics, compareAutonomousCorrectionDiagnostic)
	return diagnostics
}

func autonomousCorrectionDiagnostic(source string, issue reports.Issue) AutonomousCorrectionDiagnostic {
	sourceDiagnostic := routing.DiagnosticForIssue(issue)
	category := autonomousCorrectionCategory(issue, sourceDiagnostic)
	diagnostic := AutonomousCorrectionDiagnostic{
		Category:       category,
		Source:         source,
		SourceCategory: sourceDiagnostic.Category,
		SourceAction:   sourceDiagnostic.Action,
		IssueCode:      issue.Code,
		Severity:       issue.Severity,
		Path:           normalizeAutonomousCorrectionPath(issue.Path),
		Refs:           correctionSortedStrings(issue.Refs),
		Nets:           correctionSortedStrings(issue.Nets),
		Evidence:       autonomousCorrectionEvidence(issue, sourceDiagnostic),
	}
	diagnostic.AutomaticAction, diagnostic.UnsupportedReason = autonomousCorrectionSupport(diagnostic)
	return diagnostic
}

func autonomousCorrectionCategory(issue reports.Issue, diagnostic routing.RepairDiagnostic) AutonomousCorrectionCategory {
	switch issue.Code {
	case reports.CodePlacementCollision:
		return CorrectionComponentOverlap
	case reports.CodePlacementOutsideBoard:
		return CorrectionBlockedEscapeDirection
	case reports.CodeRouteContactLayerMismatch:
		return CorrectionMissingLayerTransition
	case reports.CodeRouteContactAmbiguous, reports.CodeRouteContactUnsupported:
		return CorrectionUnsupportedGeometry
	case reports.CodeRouteGraphIncomplete:
		return CorrectionSameNetBranchMerge
	case reports.CodeDisconnectedPad, reports.CodeRouteContactMissingTarget, reports.CodeRouteContactMiss, reports.CodeRouteCompletionPartial:
		return CorrectionRequiredNetDisconnectedEndpoint
	}
	if strings.Contains(normalizeAutonomousCorrectionPath(issue.Path), "branches[") && diagnostic.Category == routing.RepairRouteSearch {
		return CorrectionRouteTreeBranchOrder
	}
	switch diagnostic.Category {
	case routing.RepairPadAccess:
		return CorrectionInaccessiblePad
	case routing.RepairBoardBoundary:
		return CorrectionBlockedEscapeDirection
	case routing.RepairLayerAccess, routing.RepairViaPolicy:
		return CorrectionMissingLayerTransition
	case routing.RepairRouteSearch, routing.RepairClearance, routing.RepairLengthPolicy:
		return CorrectionRoutingRegionExhaustion
	case routing.RepairConnectivity:
		return CorrectionRequiredNetDisconnectedEndpoint
	default:
		return CorrectionUnsupportedGeometry
	}
}

func autonomousCorrectionSupport(diagnostic AutonomousCorrectionDiagnostic) (bool, string) {
	switch diagnostic.Category {
	case CorrectionComponentOverlap, CorrectionInaccessiblePad, CorrectionBlockedEscapeDirection, CorrectionRoutingRegionExhaustion:
		return true, ""
	case CorrectionSameNetBranchMerge:
		if len(diagnostic.Refs) >= 1 && len(diagnostic.Nets) >= 1 {
			return true, ""
		}
		return false, "same-net merge correction requires resolved refs and nets"
	case CorrectionRequiredNetDisconnectedEndpoint:
		if len(diagnostic.Refs) >= 2 && len(diagnostic.Nets) >= 1 {
			switch diagnostic.SourceCategory {
			case routing.RepairRouteSearch, routing.RepairLengthPolicy, routing.RepairPadAccess, routing.RepairLayerAccess, routing.RepairClearance, routing.RepairConnectivity:
				return true, ""
			}
		}
		return false, "disconnected endpoint correction requires an unambiguous source category, net, and endpoint refs"
	case CorrectionRouteTreeBranchOrder:
		return false, "route-tree branch reordering is reserved for a future correction contract"
	case CorrectionMissingLayerTransition:
		return false, "layer-transition insertion and relocation are not authorized in v1"
	default:
		return false, "no deterministic correction is authorized for this geometry"
	}
}

func autonomousCorrectionEvidence(issue reports.Issue, diagnostic routing.RepairDiagnostic) []string {
	evidence := []string{
		"issue_code:" + string(issue.Code),
		"source_category:" + string(diagnostic.Category),
		"source_action:" + string(diagnostic.Action),
	}
	return correctionSortedStrings(evidence)
}

func normalizeAutonomousCorrectionPath(value string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	if normalized == "" {
		return ""
	}
	if autonomousCorrectionPathIsAbsolute(normalized) {
		normalized = path.Base(strings.TrimRight(normalized, "/"))
	}
	normalized = path.Clean(normalized)
	if normalized == "." {
		return ""
	}
	return normalized
}

func autonomousCorrectionPathIsAbsolute(value string) bool {
	if strings.HasPrefix(value, "/") {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && value[2] == '/'
}

func correctionSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	slices.Sort(result)
	if len(result) == 0 {
		return nil
	}
	return result
}

func dedupeAutonomousCorrectionDiagnostics(diagnostics []AutonomousCorrectionDiagnostic) []AutonomousCorrectionDiagnostic {
	seen := map[string]struct{}{}
	result := make([]AutonomousCorrectionDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		key := autonomousCorrectionDiagnosticKey(diagnostic)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, diagnostic)
	}
	return result
}

func autonomousCorrectionDiagnosticKey(diagnostic AutonomousCorrectionDiagnostic) string {
	hash := sha256.New()
	writeHashBytes(hash, []byte(diagnostic.Category))
	writeHashBytes(hash, []byte(diagnostic.Source))
	writeHashBytes(hash, []byte(diagnostic.SourceCategory))
	writeHashBytes(hash, []byte(diagnostic.SourceAction))
	writeHashBytes(hash, []byte(diagnostic.IssueCode))
	writeHashBytes(hash, []byte(diagnostic.Path))
	writeHashBytes(hash, []byte("refs:"+strconv.Itoa(len(diagnostic.Refs))))
	for _, ref := range diagnostic.Refs {
		writeHashBytes(hash, []byte(ref))
	}
	writeHashBytes(hash, []byte("nets:"+strconv.Itoa(len(diagnostic.Nets))))
	for _, net := range diagnostic.Nets {
		writeHashBytes(hash, []byte(net))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func compareAutonomousCorrectionDiagnostic(left, right AutonomousCorrectionDiagnostic) int {
	if value := cmp.Compare(left.Category, right.Category); value != 0 {
		return value
	}
	if value := cmp.Compare(left.Source, right.Source); value != 0 {
		return value
	}
	if value := cmp.Compare(left.SourceCategory, right.SourceCategory); value != 0 {
		return value
	}
	if value := cmp.Compare(left.SourceAction, right.SourceAction); value != 0 {
		return value
	}
	if value := cmp.Compare(left.IssueCode, right.IssueCode); value != 0 {
		return value
	}
	if value := cmp.Compare(left.Path, right.Path); value != 0 {
		return value
	}
	if value := slices.Compare(left.Refs, right.Refs); value != 0 {
		return value
	}
	return slices.Compare(left.Nets, right.Nets)
}

// AutonomousCorrectionInvariantFingerprint hashes the design fields that an
// autonomous placement/routing correction is never allowed to change.
func AutonomousCorrectionInvariantFingerprint(request Request) (string, error) {
	normalized := NormalizeRequest(request)
	projection := struct {
		Version         string               `json:"version"`
		Intent          Intent               `json:"intent"`
		Board           BoardSpec            `json:"board"`
		Constraints     ConstraintSpec       `json:"constraints"`
		Validation      ValidationSpec       `json:"validation"`
		ExplicitCircuit *ExplicitCircuitSpec `json:"explicit_circuit,omitempty"`
	}{
		Version:         normalized.Version,
		Intent:          normalized.Intent,
		Board:           normalized.Board,
		Constraints:     normalized.Constraints,
		Validation:      normalized.Validation,
		ExplicitCircuit: cloneExplicitCircuit(normalized.ExplicitCircuit),
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func IsGenericAutonomousCorrectionRequest(request Request) bool {
	return request.ExplicitCircuit != nil && strings.TrimSpace(request.Intent.Category) == "explicit_circuit_graph"
}

func AutonomousCorrectionRetryKey(diagnostics []AutonomousCorrectionDiagnostic, actionKinds []string, invariantFingerprint, placementState string) string {
	diagnostics = slices.Clone(diagnostics)
	slices.SortFunc(diagnostics, compareAutonomousCorrectionDiagnostic)
	actionKinds = correctionSortedStrings(actionKinds)
	hash := sha256.New()
	// writeHashBytes length-prefixes every field, so adjacent variable-length
	// values cannot create an ambiguous hash input.
	writeHashBytes(hash, []byte(AutonomousCorrectionSchemaV1))
	writeHashBytes(hash, []byte(invariantFingerprint))
	writeHashBytes(hash, []byte(placementState))
	writeHashBytes(hash, []byte("diagnostics:"+strconv.Itoa(len(diagnostics))))
	for _, diagnostic := range diagnostics {
		writeHashBytes(hash, []byte("diagnostic"))
		writeHashBytes(hash, []byte(diagnostic.Category))
		writeHashBytes(hash, []byte(diagnostic.SourceCategory))
		writeHashBytes(hash, []byte(diagnostic.SourceAction))
		writeHashBytes(hash, []byte(diagnostic.IssueCode))
		writeHashBytes(hash, []byte(diagnostic.Path))
		writeHashBytes(hash, []byte("refs:"+strconv.Itoa(len(diagnostic.Refs))))
		for _, ref := range diagnostic.Refs {
			writeHashBytes(hash, []byte("ref:"+ref))
		}
		writeHashBytes(hash, []byte("nets:"+strconv.Itoa(len(diagnostic.Nets))))
		for _, net := range diagnostic.Nets {
			writeHashBytes(hash, []byte("net:"+net))
		}
	}
	writeHashBytes(hash, []byte("actions:"+strconv.Itoa(len(actionKinds))))
	for _, action := range actionKinds {
		writeHashBytes(hash, []byte("action:"+action))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
