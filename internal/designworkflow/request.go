package designworkflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
)

const RequestVersion = "0.1.0"
const maxRequestBytes = 1 << 20
const anchorBindingGeometryEpsilonMM = 0.001

type AcceptanceLevel string

const (
	AcceptanceDraft                AcceptanceLevel = "draft"
	AcceptanceStructural           AcceptanceLevel = "structural"
	AcceptanceConnectivity         AcceptanceLevel = "connectivity"
	AcceptanceERCDRC               AcceptanceLevel = "erc-drc"
	AcceptanceFabricationCandidate AcceptanceLevel = "fabrication-candidate"
)

type Request struct {
	Version             string                 `json:"version"`
	Name                string                 `json:"name"`
	Intent              Intent                 `json:"intent,omitempty"`
	SchematicLayout     *schematicir.Layout    `json:"schematic_layout,omitempty"`
	AutoSchematicLayout bool                   `json:"auto_schematic_layout,omitempty"`
	Board               BoardSpec              `json:"board"`
	Libraries           LibrarySpec            `json:"libraries,omitempty"`
	Components          ComponentPolicySpec    `json:"component_policy,omitempty"`
	Blocks              []BlockInstanceSpec    `json:"blocks"`
	ExplicitCircuit     *ExplicitCircuitSpec   `json:"explicit_circuit,omitempty"`
	Connections         []ConnectionSpec       `json:"connections,omitempty"`
	ExternalEndpoints   []ExternalEndpointSpec `json:"external_endpoints,omitempty"`
	Constraints         ConstraintSpec         `json:"constraints,omitempty"`
	Validation          ValidationSpec         `json:"validation,omitempty"`
	RoutingRetry        RoutingRetryPolicySpec `json:"routing_retry,omitempty"`
}

type ExplicitCircuitSpec struct {
	ResolutionHash string                  `json:"resolution_hash"`
	CatalogID      string                  `json:"catalog_id"`
	CatalogHash    string                  `json:"catalog_hash"`
	Schematic      schematicir.Document    `json:"schematic"`
	Components     []ExplicitComponentSpec `json:"components"`
	Nets           []ExplicitNetSpec       `json:"nets"`
}

type ExplicitComponentSpec struct {
	ID          string            `json:"id"`
	Reference   string            `json:"reference"`
	Role        string            `json:"role,omitempty"`
	Value       string            `json:"value,omitempty"`
	FootprintID string            `json:"footprint_id"`
	Pads        []ExplicitPadSpec `json:"pads"`
}

type ExplicitPadSpec struct {
	Name      string `json:"name"`
	SymbolPin string `json:"symbol_pin"`
	Net       string `json:"net,omitempty"`
}

type ExplicitNetSpec struct {
	Name      string                `json:"name"`
	Endpoints []ExplicitNetEndpoint `json:"endpoints"`
}

type ExplicitNetEndpoint struct {
	Component string `json:"component"`
	Pad       string `json:"pad"`
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
	SourceDir          string                           `json:"source_dir,omitempty"`
	MinimumConfidence  components.ConfidenceLevel       `json:"minimum_confidence,omitempty"`
	Acceptance         components.AcceptanceLevel       `json:"acceptance,omitempty"`
	Procurement        components.ProcurementPolicy     `json:"procurement_policy,omitempty"`
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

type ExternalEndpointSpec struct {
	ID          string                     `json:"id"`
	Kind        PhysicalEndpointKind       `json:"kind"`
	NetName     string                     `json:"net_name,omitempty"`
	Roles       []string                   `json:"roles,omitempty"`
	Layers      []string                   `json:"layers,omitempty"`
	Point       *transactions.Point        `json:"point,omitempty"`
	Edge        string                     `json:"edge,omitempty"`
	Source      string                     `json:"source,omitempty"`
	Confidence  PhysicalEndpointConfidence `json:"confidence,omitempty"`
	Required    bool                       `json:"required,omitempty"`
	Description string                     `json:"description,omitempty"`
}

type ConstraintSpec struct {
	RouteWidthMM                     float64  `json:"route_width_mm,omitempty"`
	ClearanceMM                      float64  `json:"clearance_mm,omitempty"`
	PreferTopLayer                   bool     `json:"prefer_top_layer,omitempty"`
	AllowBackLayer                   bool     `json:"allow_back_layer,omitempty"`
	TreatLocalPowerRoutesAsObstacles bool     `json:"treat_local_power_routes_as_obstacles,omitempty"`
	LocalRouteObstacleNets           []string `json:"local_route_obstacle_nets,omitempty"`
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
var externalEndpointIDPattern = regexp.MustCompile(`[^a-z0-9_]+`)
var innerCopperLayerPattern = regexp.MustCompile(`(?i)^in([0-9]+)\.cu$`)

var reservedExternalEndpointIDPrefixes = []string{
	string(PhysicalEndpointBoardEdgePoint) + "_",
	string(PhysicalEndpointFootprintPad) + "_",
	string(PhysicalEndpointImportedMechanicalPoint) + "_",
}

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
	if request.SchematicLayout != nil {
		layout := schematicir.CloneLayout(*request.SchematicLayout)
		request.SchematicLayout = &layout
	}
	if request.Board.Layers == 0 {
		request.Board.Layers = 2
	}
	if request.Validation.Acceptance == "" {
		request.Validation.Acceptance = AcceptanceStructural
	}
	request.RoutingRetry = normalizeRoutingRetryPolicy(request.RoutingRetry)
	request.Constraints.LocalRouteObstacleNets = normalizeStringList(request.Constraints.LocalRouteObstacleNets, strings.TrimSpace)
	request.Components = normalizeComponentPolicy(request.Components)
	request.Blocks = append([]BlockInstanceSpec(nil), request.Blocks...)
	for i := range request.Blocks {
		request.Blocks[i].ID = strings.TrimSpace(request.Blocks[i].ID)
		request.Blocks[i].BlockID = strings.TrimSpace(request.Blocks[i].BlockID)
		request.Blocks[i].Params = cloneParams(request.Blocks[i].Params)
	}
	request.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	request.Connections = append([]ConnectionSpec(nil), request.Connections...)
	for i := range request.Connections {
		request.Connections[i].From = strings.TrimSpace(request.Connections[i].From)
		request.Connections[i].To = strings.TrimSpace(request.Connections[i].To)
		request.Connections[i].NetAlias = strings.TrimSpace(request.Connections[i].NetAlias)
	}
	request.ExternalEndpoints = normalizeExternalEndpoints(request.ExternalEndpoints)
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

func normalizeExternalEndpoints(endpoints []ExternalEndpointSpec) []ExternalEndpointSpec {
	if len(endpoints) == 0 {
		return nil
	}
	normalized := make([]ExternalEndpointSpec, len(endpoints))
	for i, endpoint := range endpoints {
		endpoint.ID = normalizeExternalEndpointID(endpoint.ID)
		endpoint.Kind = PhysicalEndpointKind(strings.ToLower(strings.TrimSpace(string(endpoint.Kind))))
		endpoint.NetName = strings.TrimSpace(endpoint.NetName)
		endpoint.Edge = strings.ToLower(strings.TrimSpace(endpoint.Edge))
		endpoint.Source = strings.TrimSpace(endpoint.Source)
		if endpoint.Source == "" {
			endpoint.Source = "request.external_endpoints"
		}
		endpoint.Confidence = PhysicalEndpointConfidence(strings.ToLower(strings.TrimSpace(string(endpoint.Confidence))))
		if endpoint.Confidence == "" {
			if endpoint.Kind == PhysicalEndpointImportedMechanicalPoint {
				endpoint.Confidence = PhysicalEndpointConfidenceMedium
			} else {
				endpoint.Confidence = PhysicalEndpointConfidenceHigh
			}
		}
		endpoint.Description = strings.TrimSpace(endpoint.Description)
		endpoint.Roles = normalizeStringList(endpoint.Roles, strings.ToLower)
		endpoint.Layers = normalizeEndpointLayers(endpoint.Layers)
		if endpoint.Point != nil {
			point := *endpoint.Point
			endpoint.Point = &point
		}
		normalized[i] = endpoint
	}
	return normalized
}

func normalizeExternalEndpointID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = externalEndpointIDPattern.ReplaceAllString(id, "_")
	id = strings.Trim(id, "_")
	return id
}

func normalizeStringList(values []string, normalize func(string) string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if normalize != nil {
			value = normalize(value)
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeEndpointLayers(layers []string) []string {
	if len(layers) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(layers))
	for _, layer := range layers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		if canonical, ok := canonicalEndpointLayer(layer); ok {
			normalized = append(normalized, canonical)
			continue
		}
		normalized = append(normalized, layer)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func canonicalEndpointLayer(layer string) (string, bool) {
	trimmed := strings.TrimSpace(layer)
	switch strings.ToLower(trimmed) {
	case "f.cu":
		return "F.Cu", true
	case "b.cu":
		return "B.Cu", true
	case "edge.cuts":
		return "Edge.Cuts", true
	}
	matches := innerCopperLayerPattern.FindStringSubmatch(trimmed)
	if len(matches) == 2 {
		index, err := strconv.Atoi(matches[1])
		if err == nil {
			return fmt.Sprintf("In%d.Cu", index), true
		}
	}
	return "", false
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

func EnableGeneratedRoutingRetry(request *Request, minAttempts int) {
	if request == nil {
		return
	}
	request.RoutingRetry.Enabled = true
	if request.RoutingRetry.MaxAttempts < minAttempts {
		request.RoutingRetry.MaxAttempts = minAttempts
	}
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
	if invalidComponentPolicySourceDir(request.Components.SourceDir) {
		issues = append(issues, issue("component_policy.source_dir", "component source directory must be a project-relative path without parent traversal"))
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
	blockMode := len(request.Blocks) != 0
	explicitMode := request.ExplicitCircuit != nil
	if blockMode == explicitMode {
		issues = append(issues, issue("design_mode", "exactly one of blocks or explicit_circuit is required"))
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
		if (request.SchematicLayout != nil || request.AutoSchematicLayout) && strings.Contains(block.ID, schematicLayoutTargetDelimiter) {
			issues = append(issues, issue(path+".id", "block instance ID cannot contain reserved schematic layout delimiter __"))
		}
	}
	if request.AutoSchematicLayout && request.SchematicLayout != nil {
		issues = append(issues, issue("auto_schematic_layout", "automatic schematic layout cannot be combined with explicit schematic_layout intent"))
	}
	issues = append(issues, validateSchematicLayoutRequest(request.SchematicLayout, seenBlocks)...)
	if explicitMode {
		if len(request.Connections) != 0 || len(request.ExternalEndpoints) != 0 || request.SchematicLayout != nil || request.AutoSchematicLayout {
			issues = append(issues, issue("explicit_circuit", "explicit circuit mode cannot include block connections, external endpoints, or top-level schematic layout"))
		}
		issues = append(issues, validateExplicitCircuit(*request.ExplicitCircuit)...)
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
	issues = append(issues, validateExternalEndpoints(request)...)
	if request.Constraints.RouteWidthMM < 0 {
		issues = append(issues, issue("constraints.route_width_mm", "route width must be non-negative"))
	}
	if request.Constraints.ClearanceMM < 0 {
		issues = append(issues, issue("constraints.clearance_mm", "clearance must be non-negative"))
	}
	return issues
}

func cloneExplicitCircuit(source *ExplicitCircuitSpec) *ExplicitCircuitSpec {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Schematic = schematicir.Normalize(source.Schematic)
	clone.Components = append([]ExplicitComponentSpec(nil), source.Components...)
	for index := range clone.Components {
		clone.Components[index].Pads = append([]ExplicitPadSpec(nil), source.Components[index].Pads...)
	}
	clone.Nets = append([]ExplicitNetSpec(nil), source.Nets...)
	for index := range clone.Nets {
		clone.Nets[index].Endpoints = append([]ExplicitNetEndpoint(nil), source.Nets[index].Endpoints...)
	}
	return &clone
}

func validateExplicitCircuit(circuit ExplicitCircuitSpec) []reports.Issue {
	var issues []reports.Issue
	if !validSHA256(circuit.ResolutionHash) {
		issues = append(issues, issue("explicit_circuit.resolution_hash", "resolution hash must be a lowercase SHA-256 digest"))
	}
	if strings.TrimSpace(circuit.CatalogID) == "" || !validSHA256(circuit.CatalogHash) {
		issues = append(issues, issue("explicit_circuit.catalog", "catalog id and lowercase SHA-256 hash are required"))
	}
	issues = append(issues, schematicir.Validate(circuit.Schematic)...)
	if len(circuit.Components) == 0 {
		issues = append(issues, issue("explicit_circuit.components", "at least one explicit component is required"))
	}
	componentsByID := map[string]ExplicitComponentSpec{}
	references := map[string]string{}
	padsByComponent := map[string]map[string]ExplicitPadSpec{}
	for index, component := range circuit.Components {
		path := fmt.Sprintf("explicit_circuit.components[%d]", index)
		if component.ID == "" || component.Reference == "" || component.FootprintID == "" {
			issues = append(issues, issue(path, "component id, reference, and footprint_id are required"))
		}
		if _, exists := componentsByID[component.ID]; exists {
			issues = append(issues, issue(path+".id", "duplicate explicit component id"))
		}
		componentsByID[component.ID] = component
		refKey := strings.ToUpper(component.Reference)
		if owner, exists := references[refKey]; exists && owner != component.ID {
			issues = append(issues, issue(path+".reference", "reference is already owned by "+owner))
		}
		references[refKey] = component.ID
		pads := map[string]ExplicitPadSpec{}
		for padIndex, pad := range component.Pads {
			padPath := fmt.Sprintf("%s.pads[%d]", path, padIndex)
			if pad.Name == "" || pad.SymbolPin == "" {
				issues = append(issues, issue(padPath, "pad name and verified symbol_pin are required"))
			}
			if _, exists := pads[pad.Name]; exists {
				issues = append(issues, issue(padPath+".name", "duplicate explicit pad name"))
			}
			pads[pad.Name] = pad
		}
		padsByComponent[component.ID] = pads
	}
	schematicComponents := make(map[string]schematicir.Component, len(circuit.Schematic.Circuit.Components))
	for _, component := range circuit.Schematic.Circuit.Components {
		schematicComponents[component.ID] = component
	}
	for id, component := range componentsByID {
		schematicComponent, exists := schematicComponents[id]
		if !exists {
			issues = append(issues, issue("explicit_circuit.components", "component "+id+" is missing from schematic IR"))
			continue
		}
		if schematicComponent.Ref != component.Reference || schematicComponent.Footprint != component.FootprintID {
			issues = append(issues, issue("explicit_circuit.components", "component "+id+" disagrees with schematic reference or footprint"))
		}
	}
	for id := range schematicComponents {
		if _, exists := componentsByID[id]; !exists {
			issues = append(issues, issue("explicit_circuit.schematic", "schematic component "+id+" has no resolved explicit component"))
		}
	}
	if len(circuit.Nets) == 0 {
		issues = append(issues, issue("explicit_circuit.nets", "at least one explicit net is required"))
	}
	ownedPads := map[string]string{}
	seenNets := map[string]struct{}{}
	schematicNets := make(map[string]map[string]struct{}, len(circuit.Schematic.Circuit.Nets))
	for _, net := range circuit.Schematic.Circuit.Nets {
		endpoints := make(map[string]struct{}, len(net.Connect))
		for _, endpoint := range net.Connect {
			component, pin, ok := endpoint.Split()
			if ok {
				endpoints[component+"\x00"+pin] = struct{}{}
			}
		}
		schematicNets[net.Name] = endpoints
	}
	for netIndex, net := range circuit.Nets {
		path := fmt.Sprintf("explicit_circuit.nets[%d]", netIndex)
		if net.Name == "" || len(net.Endpoints) < 2 {
			issues = append(issues, issue(path, "net name and at least two endpoints are required"))
		}
		if _, exists := seenNets[net.Name]; exists {
			issues = append(issues, issue(path+".name", "duplicate explicit net name"))
		}
		seenNets[net.Name] = struct{}{}
		schematicEndpoints, schematicNetExists := schematicNets[net.Name]
		if !schematicNetExists {
			issues = append(issues, issue(path, "net is missing from schematic IR"))
		}
		explicitEndpoints := map[string]struct{}{}
		for endpointIndex, endpoint := range net.Endpoints {
			endpointPath := fmt.Sprintf("%s.endpoints[%d]", path, endpointIndex)
			pad, exists := padsByComponent[endpoint.Component][endpoint.Pad]
			if !exists {
				issues = append(issues, issue(endpointPath, "endpoint does not resolve to an explicit component pad"))
				continue
			}
			key := endpoint.Component + "\x00" + endpoint.Pad
			if owner, exists := ownedPads[key]; exists && owner != net.Name {
				issues = append(issues, issue(endpointPath, "pad is already assigned to net "+owner))
			}
			ownedPads[key] = net.Name
			if pad.Net != net.Name {
				issues = append(issues, issue(endpointPath, "endpoint net disagrees with component pad net"))
			}
			explicitEndpoints[endpoint.Component+"\x00"+pad.SymbolPin] = struct{}{}
		}
		if schematicNetExists && !sameExplicitEndpointSet(explicitEndpoints, schematicEndpoints) {
			issues = append(issues, issue(path, "resolved pad endpoints disagree with schematic symbol-pin endpoints"))
		}
	}
	for name := range schematicNets {
		if _, exists := seenNets[name]; !exists {
			issues = append(issues, issue("explicit_circuit.schematic", "schematic net "+name+" has no resolved explicit net"))
		}
	}
	for componentID, pads := range padsByComponent {
		for padName, pad := range pads {
			if pad.Net == "" {
				continue
			}
			key := componentID + "\x00" + padName
			if ownedPads[key] != pad.Net {
				issues = append(issues, issue("explicit_circuit.components", "pad "+componentID+"."+padName+" names net "+pad.Net+" without a matching resolved endpoint"))
			}
		}
	}
	return issues
}

func sameExplicitEndpointSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for endpoint := range left {
		if _, exists := right[endpoint]; !exists {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
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

func validateExternalEndpoints(request Request) []reports.Issue {
	var issues []reports.Issue
	seen := map[string]int{}
	for index, endpoint := range request.ExternalEndpoints {
		path := fmt.Sprintf("external_endpoints[%d]", index)
		if endpoint.ID == "" {
			issues = append(issues, issue(path+".id", "external endpoint ID is required and must contain at least one alphanumeric or underscore character"))
		} else {
			if firstIndex, exists := seen[endpoint.ID]; exists {
				issues = append(issues, issue(path+".id", fmt.Sprintf("duplicate external endpoint ID %q after normalization; first seen at external_endpoints[%d]", endpoint.ID, firstIndex)))
			} else {
				seen[endpoint.ID] = index
			}
			endpointID := strings.ToLower(endpoint.ID)
			for _, prefix := range reservedExternalEndpointIDPrefixes {
				if strings.HasPrefix(endpointID, strings.ToLower(prefix)) {
					issues = append(issues, issue(path+".id", "external endpoint ID uses reserved system prefix "+prefix))
					break
				}
			}
		}
		if !validExternalEndpointKind(endpoint.Kind) {
			issues = append(issues, issue(path+".kind", "unsupported external endpoint kind "+string(endpoint.Kind)))
		}
		if !validExternalEndpointConfidence(endpoint.Confidence) {
			issues = append(issues, issue(path+".confidence", "unsupported external endpoint confidence "+string(endpoint.Confidence)))
		}
		if endpoint.Edge != "" && !validExternalEndpointEdge(endpoint.Edge) {
			issues = append(issues, issue(path+".edge", "external endpoint edge must be left, right, top, or bottom"))
		}
		if endpoint.Required && endpoint.Point == nil {
			issues = append(issues, issue(path+".point", "required external endpoint must include a point"))
		}
		if endpoint.Required && endpoint.NetName == "" {
			issues = append(issues, issue(path+".net_name", "required external endpoint must include net_name"))
		}
		if endpoint.Point != nil {
			issues = append(issues, validateExternalEndpointPoint(path+".point", *endpoint.Point, request.Board)...)
		}
		issues = append(issues, validateExternalEndpointLayers(path, endpoint, request.Board)...)
	}
	return issues
}

func validExternalEndpointKind(kind PhysicalEndpointKind) bool {
	switch kind {
	case PhysicalEndpointBoardEdgePoint, PhysicalEndpointImportedMechanicalPoint:
		return true
	default:
		return false
	}
}

func validExternalEndpointConfidence(confidence PhysicalEndpointConfidence) bool {
	switch confidence {
	case PhysicalEndpointConfidenceHigh, PhysicalEndpointConfidenceMedium, PhysicalEndpointConfidenceLow:
		return true
	default:
		return false
	}
}

func validExternalEndpointEdge(edge string) bool {
	switch edge {
	case "left", "right", "top", "bottom":
		return true
	default:
		return false
	}
}

func validateExternalEndpointPoint(path string, point transactions.Point, board BoardSpec) []reports.Issue {
	var issues []reports.Issue
	if math.IsNaN(point.XMM) || math.IsInf(point.XMM, 0) {
		issues = append(issues, issue(path+".x_mm", "external endpoint x coordinate must be finite"))
	} else if point.XMM < -anchorBindingGeometryEpsilonMM {
		issues = append(issues, issue(path+".x_mm", "external endpoint x coordinate is outside the board frame; check imported coordinate origin or Y-up conversion"))
	} else if board.WidthMM > 0 && point.XMM > board.WidthMM+anchorBindingGeometryEpsilonMM {
		issues = append(issues, issue(path+".x_mm", "external endpoint x coordinate is outside board width; check imported coordinate origin or Y-up conversion"))
	}
	if math.IsNaN(point.YMM) || math.IsInf(point.YMM, 0) {
		issues = append(issues, issue(path+".y_mm", "external endpoint y coordinate must be finite"))
	} else if point.YMM < -anchorBindingGeometryEpsilonMM {
		issues = append(issues, issue(path+".y_mm", "external endpoint y coordinate is outside the board frame; check imported coordinate origin or Y-up conversion"))
	} else if board.HeightMM > 0 && point.YMM > board.HeightMM+anchorBindingGeometryEpsilonMM {
		issues = append(issues, issue(path+".y_mm", "external endpoint y coordinate is outside board height; check imported coordinate origin or Y-up conversion"))
	}
	return issues
}

func validateExternalEndpointLayers(path string, endpoint ExternalEndpointSpec, board BoardSpec) []reports.Issue {
	var issues []reports.Issue
	if len(endpoint.Layers) == 0 {
		return nil
	}
	hasCopper := false
	hasTechnicalOnly := false
	for index, layer := range endpoint.Layers {
		layerPath := fmt.Sprintf("%s.layers[%d]", path, index)
		if layer == "" {
			continue
		}
		if isDiagnosticTechnicalLayer(layer) {
			hasTechnicalOnly = true
			continue
		}
		if !isCopperLayer(layer) {
			issues = append(issues, issue(layerPath, "external endpoint layer must be a supported KiCad copper layer or diagnostic technical layer"))
			continue
		}
		hasCopper = true
		inner, isInner := innerLayerNumber(layer)
		if isInner && board.Layers > 0 && (inner < 1 || inner > board.Layers-2) {
			issues = append(issues, issue(layerPath, "internal copper layer does not exist in declared board stackup"))
		}
	}
	if !hasCopper && hasTechnicalOnly && (endpoint.Required || endpoint.NetName != "") {
		issues = append(issues, issue(path+".layers", "electrical external endpoint must include at least one copper layer or omit layers for any copper"))
	}
	return issues
}

func isCopperLayer(layer string) bool {
	if strings.EqualFold(layer, "F.Cu") || strings.EqualFold(layer, "B.Cu") {
		return true
	}
	_, ok := innerLayerNumber(layer)
	return ok
}

func innerLayerNumber(layer string) (int, bool) {
	matches := innerCopperLayerPattern.FindStringSubmatch(layer)
	if len(matches) != 2 {
		return 0, false
	}
	index, err := strconv.Atoi(matches[1])
	return index, err == nil
}

func isDiagnosticTechnicalLayer(layer string) bool {
	return strings.EqualFold(layer, "Edge.Cuts")
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
	policy.SourceDir = strings.TrimSpace(policy.SourceDir)
	policy.Overrides = cloneComponentOverrides(policy.Overrides)
	policy.PackagePreferences = cloneStringMap(policy.PackagePreferences)
	return policy
}

func invalidComponentPolicySourceDir(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if filepath.IsAbs(value) || strings.HasPrefix(normalized, "/") || windowsAbsolutePath(normalized) {
		return true
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	clean := filepath.Clean(value)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func ComponentSourceDirAllowed(value string) bool {
	return !invalidComponentPolicySourceDir(value)
}

func windowsAbsolutePath(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	drive := value[0]
	return (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')
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
