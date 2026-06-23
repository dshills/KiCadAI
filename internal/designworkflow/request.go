package designworkflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const RequestVersion = "0.1.0"
const maxRequestBytes = 1 << 20

type AcceptanceLevel string

const (
	AcceptanceDraft                AcceptanceLevel = "draft"
	AcceptanceStructural           AcceptanceLevel = "structural"
	AcceptanceConnectivity         AcceptanceLevel = "connectivity"
	AcceptanceERCDRC               AcceptanceLevel = "erc-drc"
	AcceptanceFabricationCandidate AcceptanceLevel = "fabrication-candidate"
)

type Request struct {
	Version      string                 `json:"version"`
	Name         string                 `json:"name"`
	Intent       Intent                 `json:"intent,omitempty"`
	Board        BoardSpec              `json:"board"`
	Libraries    LibrarySpec            `json:"libraries,omitempty"`
	Components   ComponentPolicySpec    `json:"component_policy,omitempty"`
	Blocks       []BlockInstanceSpec    `json:"blocks"`
	Connections  []ConnectionSpec       `json:"connections,omitempty"`
	Constraints  ConstraintSpec         `json:"constraints,omitempty"`
	Validation   ValidationSpec         `json:"validation,omitempty"`
	RoutingRetry RoutingRetryPolicySpec `json:"routing_retry,omitempty"`
}

type Intent struct {
	Summary  string `json:"summary,omitempty"`
	Category string `json:"category,omitempty"`
}

type BoardSpec struct {
	WidthMM         float64 `json:"width_mm"`
	HeightMM        float64 `json:"height_mm"`
	Layers          int     `json:"layers,omitempty"`
	EdgeClearanceMM float64 `json:"edge_clearance_mm,omitempty"`
}

type LibrarySpec struct {
	RequireResolver bool     `json:"require_resolver,omitempty"`
	SymbolRoots     []string `json:"symbol_roots,omitempty"`
	FootprintRoots  []string `json:"footprint_roots,omitempty"`
}

type ComponentPolicySpec struct {
	CatalogDir         string                           `json:"catalog_dir,omitempty"`
	MinimumConfidence  components.ConfidenceLevel       `json:"minimum_confidence,omitempty"`
	Acceptance         components.AcceptanceLevel       `json:"acceptance,omitempty"`
	Overrides          map[string]ComponentOverrideSpec `json:"overrides,omitempty"`
	PackagePreferences map[string]string                `json:"package_preferences,omitempty"`
}

type ComponentOverrideSpec struct {
	ComponentID       string                      `json:"component_id,omitempty"`
	VariantID         string                      `json:"variant_id,omitempty"`
	Package           string                      `json:"package,omitempty"`
	MinimumConfidence components.ConfidenceLevel  `json:"minimum_confidence,omitempty"`
	Acceptance        components.AcceptanceLevel  `json:"acceptance,omitempty"`
	AllowAlternatives bool                        `json:"allow_alternatives,omitempty"`
	RequiredRatings   []components.RequiredRating `json:"required_ratings,omitempty"`
}

type BlockInstanceSpec struct {
	ID      string         `json:"id"`
	BlockID string         `json:"block_id"`
	Params  map[string]any `json:"params,omitempty"`
}

type ConnectionSpec struct {
	From     string `json:"from"`
	To       string `json:"to"`
	NetAlias string `json:"net_alias,omitempty"`
}

type ConstraintSpec struct {
	RouteWidthMM   float64 `json:"route_width_mm,omitempty"`
	ClearanceMM    float64 `json:"clearance_mm,omitempty"`
	PreferTopLayer bool    `json:"prefer_top_layer,omitempty"`
	AllowBackLayer bool    `json:"allow_back_layer,omitempty"`
}

type ValidationSpec struct {
	Acceptance      AcceptanceLevel `json:"acceptance,omitempty"`
	RequireERC      bool            `json:"require_erc,omitempty"`
	RequireDRC      bool            `json:"require_drc,omitempty"`
	StrictUnrouted  bool            `json:"strict_unrouted,omitempty"`
	StrictZones     bool            `json:"strict_zones,omitempty"`
	SkipRouting     bool            `json:"skip_routing,omitempty"`
	SkipKiCadChecks bool            `json:"skip_kicad_checks,omitempty"`
}

type RoutingRetryPolicySpec struct {
	Enabled                 bool                         `json:"enabled,omitempty"`
	MaxAttempts             int                          `json:"max_attempts,omitempty"`
	MinRoutingScoreDelta    float64                      `json:"min_routing_score_delta,omitempty"`
	AllowedHintCategories   []PlacementRetryHintCategory `json:"allowed_hint_categories,omitempty"`
	DRCPolicy               RetryDRCPolicy               `json:"drc_policy,omitempty"`
	PreserveFixed           bool                         `json:"preserve_fixed,omitempty"`
	StopOnNewBlockers       bool                         `json:"stop_on_new_blockers,omitempty"`
	StopOnRepeatedSignature bool                         `json:"stop_on_repeated_signature,omitempty"`
	StopOnNonImprovement    bool                         `json:"stop_on_non_improvement,omitempty"`
}

var projectNamePattern = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func DecodeRequestStrict(reader io.Reader) (Request, []reports.Issue) {
	var buffer bytes.Buffer
	limited := io.LimitReader(reader, maxRequestBytes+1)
	if _, err := io.Copy(&buffer, limited); err != nil {
		return Request{}, []reports.Issue{issue("request", "read request: "+err.Error())}
	}
	if buffer.Len() > maxRequestBytes {
		return Request{}, []reports.Issue{issue("request", "request exceeds maximum size")}
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Request{}, []reports.Issue{issue("request", "decode request: "+err.Error())}
	}
	return request, ValidateRequest(request)
}

func NormalizeRequest(request Request) Request {
	request.Name = NormalizeProjectName(request.Name)
	if request.Board.Layers == 0 {
		request.Board.Layers = 2
	}
	if request.Validation.Acceptance == "" {
		request.Validation.Acceptance = AcceptanceStructural
	}
	request.RoutingRetry = normalizeRoutingRetryPolicy(request.RoutingRetry)
	request.Components = normalizeComponentPolicy(request.Components)
	request.Blocks = append([]BlockInstanceSpec(nil), request.Blocks...)
	for i := range request.Blocks {
		request.Blocks[i].ID = strings.TrimSpace(request.Blocks[i].ID)
		request.Blocks[i].BlockID = strings.TrimSpace(request.Blocks[i].BlockID)
		request.Blocks[i].Params = cloneParams(request.Blocks[i].Params)
	}
	request.Connections = append([]ConnectionSpec(nil), request.Connections...)
	for i := range request.Connections {
		request.Connections[i].From = strings.TrimSpace(request.Connections[i].From)
		request.Connections[i].To = strings.TrimSpace(request.Connections[i].To)
		request.Connections[i].NetAlias = strings.TrimSpace(request.Connections[i].NetAlias)
	}
	return request
}

func NormalizeProjectName(name string) string {
	name = strings.TrimSpace(name)
	name = projectNamePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_-")
	if name == "" {
		return "kicadai_design"
	}
	return name
}

func normalizeRoutingRetryPolicy(policy RoutingRetryPolicySpec) RoutingRetryPolicySpec {
	if policy.MaxAttempts == 0 {
		policy.MaxAttempts = 1
	}
	if policy.AllowedHintCategories == nil {
		policy.AllowedHintCategories = []PlacementRetryHintCategory{
			PlacementRetryReduceDistance,
			PlacementRetryIncreaseSpacing,
			PlacementRetryImproveFanout,
			PlacementRetryMoveFromEdge,
		}
	} else {
		policy.AllowedHintCategories = append([]PlacementRetryHintCategory(nil), policy.AllowedHintCategories...)
	}
	if !policy.Enabled && policy.MaxAttempts > 0 {
		policy.MaxAttempts = max(1, policy.MaxAttempts)
	}
	if policy.Enabled && policy.MaxAttempts == 1 {
		policy.MaxAttempts = 2
	}
	return policy
}

func ValidateRequest(request Request) []reports.Issue {
	request = NormalizeRequest(request)
	var issues []reports.Issue
	if request.Version == "" {
		issues = append(issues, issue("version", "request version is required"))
	} else if request.Version != RequestVersion {
		issues = append(issues, issue("version", "unsupported request version "+request.Version))
	}
	if request.Board.WidthMM <= 0 {
		issues = append(issues, issue("board.width_mm", "board width must be positive"))
	}
	if request.Board.HeightMM <= 0 {
		issues = append(issues, issue("board.height_mm", "board height must be positive"))
	}
	if request.Board.Layers != 1 && request.Board.Layers != 2 {
		issues = append(issues, issue("board.layers", "board layers must be 1 or 2"))
	}
	if request.Board.EdgeClearanceMM < 0 {
		issues = append(issues, issue("board.edge_clearance_mm", "board edge clearance must be non-negative"))
	}
	if !validAcceptanceLevel(request.Validation.Acceptance) {
		issues = append(issues, issue("validation.acceptance", "unsupported acceptance level "+string(request.Validation.Acceptance)))
	}
	issues = append(issues, validateRoutingRetryPolicy(request.RoutingRetry)...)
	if request.Components.MinimumConfidence != "" {
		componentIssue, ok := components.ValidateConfidenceIssue("component_policy.minimum_confidence", request.Components.MinimumConfidence)
		if ok {
			issues = append(issues, componentIssue)
		}
	}
	if componentIssue, ok := components.ValidateAcceptanceIssue("component_policy.acceptance", request.Components.Acceptance); ok {
		issues = append(issues, componentIssue)
	}
	for key, override := range request.Components.Overrides {
		path := "component_policy.overrides." + key
		if strings.TrimSpace(key) == "" {
			issues = append(issues, issue(path, "component override key is required"))
		}
		if override.MinimumConfidence != "" {
			componentIssue, ok := components.ValidateConfidenceIssue(path+".minimum_confidence", override.MinimumConfidence)
			if ok {
				issues = append(issues, componentIssue)
			}
		}
		if componentIssue, ok := components.ValidateAcceptanceIssue(path+".acceptance", override.Acceptance); ok {
			issues = append(issues, componentIssue)
		}
	}
	if len(request.Blocks) == 0 {
		issues = append(issues, issue("blocks", "at least one block is required"))
	}
	seenBlocks := map[string]struct{}{}
	for index, block := range request.Blocks {
		path := fmt.Sprintf("blocks[%d]", index)
		if strings.TrimSpace(block.ID) == "" {
			issues = append(issues, issue(path+".id", "block instance ID is required"))
		} else if _, exists := seenBlocks[block.ID]; exists {
			issues = append(issues, issue(path+".id", "duplicate block instance ID "+block.ID))
		} else {
			seenBlocks[block.ID] = struct{}{}
		}
		if strings.TrimSpace(block.BlockID) == "" {
			issues = append(issues, issue(path+".block_id", "block ID is required"))
		}
	}
	for index, connection := range request.Connections {
		path := fmt.Sprintf("connections[%d]", index)
		from, ok := ParseEndpoint(connection.From)
		if !ok {
			issues = append(issues, issue(path+".from", "connection endpoint must use instance.port syntax"))
		} else if _, exists := seenBlocks[from.InstanceID]; !exists {
			issues = append(issues, issue(path+".from", "connection references unknown block instance "+from.InstanceID))
		}
		to, ok := ParseEndpoint(connection.To)
		if !ok {
			issues = append(issues, issue(path+".to", "connection endpoint must use instance.port syntax"))
		} else if _, exists := seenBlocks[to.InstanceID]; !exists {
			issues = append(issues, issue(path+".to", "connection references unknown block instance "+to.InstanceID))
		}
	}
	if request.Constraints.RouteWidthMM < 0 {
		issues = append(issues, issue("constraints.route_width_mm", "route width must be non-negative"))
	}
	if request.Constraints.ClearanceMM < 0 {
		issues = append(issues, issue("constraints.clearance_mm", "clearance must be non-negative"))
	}
	return issues
}

func validateRoutingRetryPolicy(policy RoutingRetryPolicySpec) []reports.Issue {
	var issues []reports.Issue
	if policy.MaxAttempts < 1 {
		issues = append(issues, issue("routing_retry.max_attempts", "routing retry max attempts must be at least 1"))
	}
	if policy.MinRoutingScoreDelta < 0 {
		issues = append(issues, issue("routing_retry.min_routing_score_delta", "routing retry minimum score delta must be non-negative"))
	}
	if policy.DRCPolicy != "" && normalizeRetryDRCPolicy(policy.DRCPolicy) != policy.DRCPolicy {
		issues = append(issues, issue("routing_retry.drc_policy", "unsupported routing retry DRC policy "+string(policy.DRCPolicy)))
	}
	for index, category := range policy.AllowedHintCategories {
		if !validPlacementRetryHintCategory(category) {
			issues = append(issues, issue(fmt.Sprintf("routing_retry.allowed_hint_categories[%d]", index), "unsupported placement retry hint category "+string(category)))
		}
	}
	return issues
}

func validPlacementRetryHintCategory(category PlacementRetryHintCategory) bool {
	switch category {
	case PlacementRetryReduceDistance, PlacementRetryIncreaseSpacing, PlacementRetryImproveFanout, PlacementRetryMoveFromEdge, PlacementRetryRelaxRules, PlacementRetryUnsupported:
		return true
	default:
		return false
	}
}

func ToCompositionRequest(request Request) (blocks.CompositionRequest, []reports.Issue) {
	request = NormalizeRequest(request)
	if issues := ValidateRequest(request); len(issues) != 0 {
		return blocks.CompositionRequest{}, issues
	}
	composition := blocks.CompositionRequest{
		ProjectName: request.Name,
		Instances:   make([]blocks.CompositionInstance, 0, len(request.Blocks)),
		Connections: make([]blocks.CompositionConnection, 0, len(request.Connections)),
	}
	for _, block := range request.Blocks {
		composition.Instances = append(composition.Instances, blocks.CompositionInstance{
			ID:      block.ID,
			BlockID: block.BlockID,
			Params:  cloneParams(block.Params),
		})
	}
	for _, connection := range request.Connections {
		from, _ := ParseEndpoint(connection.From)
		to, _ := ParseEndpoint(connection.To)
		composition.Connections = append(composition.Connections, blocks.CompositionConnection{
			From:     from,
			To:       to,
			NetAlias: connection.NetAlias,
		})
	}
	return composition, nil
}

func ParseEndpoint(value string) (blocks.PortRef, bool) {
	value = strings.TrimSpace(value)
	left, right, ok := strings.Cut(value, ".")
	if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return blocks.PortRef{}, false
	}
	if strings.Contains(right, ".") {
		return blocks.PortRef{}, false
	}
	return blocks.PortRef{InstanceID: strings.TrimSpace(left), Port: strings.TrimSpace(right)}, true
}

func validAcceptanceLevel(level AcceptanceLevel) bool {
	switch level {
	case AcceptanceDraft, AcceptanceStructural, AcceptanceConnectivity, AcceptanceERCDRC, AcceptanceFabricationCandidate:
		return true
	default:
		return false
	}
}

func cloneParams(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	clone := make(map[string]any, len(params))
	for key, value := range params {
		clone[key] = cloneJSONValue(value)
	}
	return clone
}

func normalizeComponentPolicy(policy ComponentPolicySpec) ComponentPolicySpec {
	policy.CatalogDir = strings.TrimSpace(policy.CatalogDir)
	policy.Overrides = cloneComponentOverrides(policy.Overrides)
	policy.PackagePreferences = cloneStringMap(policy.PackagePreferences)
	return policy
}

func cloneComponentOverrides(overrides map[string]ComponentOverrideSpec) map[string]ComponentOverrideSpec {
	if overrides == nil {
		return nil
	}
	clone := make(map[string]ComponentOverrideSpec, len(overrides))
	for key, override := range overrides {
		override.ComponentID = strings.TrimSpace(override.ComponentID)
		override.VariantID = strings.TrimSpace(override.VariantID)
		override.Package = strings.TrimSpace(override.Package)
		override.RequiredRatings = append([]components.RequiredRating(nil), override.RequiredRatings...)
		clone[strings.TrimSpace(key)] = override
	}
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return clone
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, nested := range typed {
			clone[key] = cloneJSONValue(nested)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for i, nested := range typed {
			clone[i] = cloneJSONValue(nested)
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func issue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}
