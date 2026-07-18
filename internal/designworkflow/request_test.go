package designworkflow

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestValidateRequestAcceptsExplicitComposition(t *testing.T) {
	request := validRequest()
	if issues := ValidateRequest(request); len(issues) != 0 {
		t.Fatalf("ValidateRequest issues = %#v", issues)
	}
	composition, issues := ToCompositionRequest(request)
	if len(issues) != 0 {
		t.Fatalf("ToCompositionRequest issues = %#v", issues)
	}
	if composition.ProjectName != "sensor_breakout" || len(composition.Instances) != 2 || len(composition.Connections) != 1 {
		t.Fatalf("composition = %#v", composition)
	}
	if composition.Connections[0].From.InstanceID != "sensor" || composition.Connections[0].To.Port != "SDA" {
		t.Fatalf("connection = %#v", composition.Connections[0])
	}
}

func TestValidateRequestRejectsDuplicateBlocks(t *testing.T) {
	request := validRequest()
	request.Blocks[1].ID = "sensor"
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "blocks[1].id")
}

func TestValidateRequestDoesNotMutateInput(t *testing.T) {
	request := validRequest()
	request.Name = " Sensor Demo "
	request.Blocks[0].ID = " sensor "
	request.Connections[0].From = " sensor.SDA "
	_ = ValidateRequest(request)
	if request.Name != " Sensor Demo " || request.Blocks[0].ID != " sensor " || request.Connections[0].From != " sensor.SDA " {
		t.Fatalf("ValidateRequest mutated input: %#v", request)
	}
}

func TestNormalizeRequestTrimsFabricationMetadata(t *testing.T) {
	request := validRequest()
	request.Fabrication = FabricationMetadataSpec{
		BoardFinish:      " ENIG ",
		FabricationNotes: " Lead-free assembly. ",
	}
	normalized := NormalizeRequest(request)
	if normalized.Fabrication.BoardFinish != "ENIG" || normalized.Fabrication.FabricationNotes != "Lead-free assembly." {
		t.Fatalf("fabrication metadata = %#v", normalized.Fabrication)
	}
}

func TestNormalizeRequestBoundsFabricationMetadataAtUTF8Boundary(t *testing.T) {
	request := validRequest()
	request.Fabrication.BoardFinish = strings.Repeat("é", 100)
	request.Fabrication.FabricationNotes = strings.Repeat("é", 3000)
	normalized := NormalizeRequest(request)
	if len(normalized.Fabrication.BoardFinish) > 128 || len(normalized.Fabrication.FabricationNotes) > 4096 || !utf8.ValidString(normalized.Fabrication.BoardFinish) || !utf8.ValidString(normalized.Fabrication.FabricationNotes) {
		t.Fatalf("normalized fabrication metadata has unsafe byte bounds: finish=%d notes=%d", len(normalized.Fabrication.BoardFinish), len(normalized.Fabrication.FabricationNotes))
	}
}

func TestValidateRequestRejectsOversizedFabricationMetadata(t *testing.T) {
	request := validRequest()
	request.Fabrication.BoardFinish = strings.Repeat("x", 129)
	request.Fabrication.FabricationNotes = strings.Repeat("x", 4097)
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "fabrication.board_finish")
	assertIssuePath(t, issues, "fabrication.fabrication_notes")
}

func TestValidateRequestRejectsUnknownAcceptance(t *testing.T) {
	request := validRequest()
	request.Validation.Acceptance = "magic"
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "validation.acceptance")
}

func TestDecodeRequestStrictNormalizesWorkflowComponentAcceptance(t *testing.T) {
	request, issues := DecodeRequestStrict(strings.NewReader(`{
	  "version": "0.1.0",
	  "name": "fabrication candidate",
	  "board": {"width_mm": 10, "height_mm": 10},
	  "component_policy": {
	    "acceptance": "fabrication-candidate",
	    "overrides": {
	      "U1": {"acceptance": "erc-drc"}
	    }
	  },
	  "blocks": [{"id": "led", "block_id": "led_indicator"}]
	}`))
	if len(issues) != 0 {
		t.Fatalf("DecodeRequestStrict issues = %#v", issues)
	}
	normalized := NormalizeRequest(request)
	if normalized.Components.Acceptance != components.AcceptanceFabricationCandidate {
		t.Fatalf("component acceptance = %q", normalized.Components.Acceptance)
	}
	if got := normalized.Components.Overrides["U1"].Acceptance; got != components.AcceptanceERCDRC {
		t.Fatalf("override acceptance = %q", got)
	}
}

func TestValidateRequestRejectsUnknownComponentAcceptance(t *testing.T) {
	request := validRequest()
	request.Components.Acceptance = "magic"
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "component_policy.acceptance")
}

func TestValidateRequestRejectsUnsafeComponentSourceDir(t *testing.T) {
	request := validRequest()
	request.Components.SourceDir = "../component-sources"
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "component_policy.source_dir")
}

func TestValidateRequestRejectsInvalidEndpoint(t *testing.T) {
	request := validRequest()
	request.Connections[0].From = "sensor"
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "connections[0].from")
}

func TestValidateRequestRejectsUnknownConnectionInstance(t *testing.T) {
	request := validRequest()
	request.Connections[0].To = "missing.SDA"
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "connections[0].to")
}

func TestDecodeRequestStrictRejectsUnknownFields(t *testing.T) {
	_, issues := DecodeRequestStrict(strings.NewReader(`{
	  "version": "0.1.0",
	  "name": "demo",
	  "board": {"width_mm": 10, "height_mm": 10, "surprise": true},
	  "blocks": [{"id": "led", "block_id": "led_indicator"}]
	}`))
	if len(issues) == 0 {
		t.Fatal("expected unknown field issue")
	}
}

func TestDecodeRequestStrictRejectsOversizedPayload(t *testing.T) {
	_, issues := DecodeRequestStrict(strings.NewReader(strings.Repeat(" ", maxRequestBytes+1)))
	assertIssuePath(t, issues, "request")
}

func TestToCompositionRequestDeepClonesParams(t *testing.T) {
	request := validRequest()
	request.Blocks[1].Params = map[string]any{"pin_names": []any{"SDA", "SCL"}}
	composition, issues := ToCompositionRequest(request)
	if len(issues) != 0 {
		t.Fatalf("ToCompositionRequest issues = %#v", issues)
	}
	composition.Instances[1].Params["pin_names"].([]any)[0] = "MUTATED"
	if request.Blocks[1].Params["pin_names"].([]any)[0] != "SDA" {
		t.Fatalf("request params were mutated: %#v", request.Blocks[1].Params)
	}
}

func TestNormalizeProjectName(t *testing.T) {
	if got := NormalizeProjectName(" Sensor Breakout! "); got != "Sensor_Breakout" {
		t.Fatalf("NormalizeProjectName = %q", got)
	}
	if got := NormalizeProjectName(" !!! "); got != "kicadai_design" {
		t.Fatalf("empty NormalizeProjectName = %q", got)
	}
}

func TestNormalizeRequestDefaultsRoutingRetryDisabled(t *testing.T) {
	request := NormalizeRequest(validRequest())
	if request.RoutingRetry.Enabled {
		t.Fatalf("retry enabled by default: %#v", request.RoutingRetry)
	}
	if request.RoutingRetry.MaxAttempts != 1 || len(request.RoutingRetry.AllowedHintCategories) == 0 {
		t.Fatalf("retry defaults = %#v", request.RoutingRetry)
	}
}

func TestNormalizeRequestEnabledRoutingRetryIsBounded(t *testing.T) {
	request := validRequest()
	request.RoutingRetry = RoutingRetryPolicySpec{Enabled: true, MaxAttempts: 1}

	normalized := NormalizeRequest(request)
	if normalized.RoutingRetry.MaxAttempts != 2 {
		t.Fatalf("max attempts = %d, want total attempts bumped to 2", normalized.RoutingRetry.MaxAttempts)
	}
}

func TestNormalizeRequestLocalRouteObstacleNets(t *testing.T) {
	request := validRequest()
	request.Constraints.LocalRouteObstacleNets = []string{" GND ", "", "Sda"}

	normalized := NormalizeRequest(request)
	if !slices.Equal(normalized.Constraints.LocalRouteObstacleNets, []string{"GND", "Sda"}) {
		t.Fatalf("local-route obstacle nets = %#v", normalized.Constraints.LocalRouteObstacleNets)
	}
	normalized.Constraints.LocalRouteObstacleNets[0] = "changed"
	if request.Constraints.LocalRouteObstacleNets[0] != " GND " {
		t.Fatalf("NormalizeRequest did not clone local-route obstacle nets")
	}
}

func TestEnableGeneratedRoutingRetrySetsMinimumAttempts(t *testing.T) {
	request := validRequest()
	request.RoutingRetry.MaxAttempts = 1

	EnableGeneratedRoutingRetry(&request, 2)

	if !request.RoutingRetry.Enabled || request.RoutingRetry.MaxAttempts != 2 {
		t.Fatalf("routing retry = %#v", request.RoutingRetry)
	}
}

func TestValidateRequestRejectsInvalidRoutingRetryPolicy(t *testing.T) {
	request := validRequest()
	request.RoutingRetry = RoutingRetryPolicySpec{
		MaxAttempts:          -1,
		MinRoutingScoreDelta: -0.1,
		DRCPolicy:            RetryDRCPolicy("always"),
		AllowedHintCategories: []PlacementRetryHintCategory{
			PlacementRetryHintCategory("teleport"),
		},
	}

	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "routing_retry.max_attempts")
	assertIssuePath(t, issues, "routing_retry.min_routing_score_delta")
	assertIssuePath(t, issues, "routing_retry.drc_policy")
	assertIssuePath(t, issues, "routing_retry.allowed_hint_categories[0]")
}

func TestNormalizeRequestExternalEndpoints(t *testing.T) {
	request := validRequest()
	request.ExternalEndpoints = []ExternalEndpointSpec{{
		ID:         " Edge VIN ",
		Kind:       PhysicalEndpointBoardEdgePoint,
		NetName:    " VIN_RAW ",
		Roles:      []string{" Power ", "", "EDGE"},
		Layers:     []string{" f.cu ", "", "edge.cuts"},
		Edge:       " LEFT ",
		Confidence: "",
		Point:      &transactions.Point{XMM: 0, YMM: 5},
	}}

	normalized := NormalizeRequest(request)
	endpoint := normalized.ExternalEndpoints[0]
	if endpoint.ID != "edge_vin" || endpoint.NetName != "VIN_RAW" || endpoint.Edge != "left" {
		t.Fatalf("normalized endpoint identity = %#v", endpoint)
	}
	if endpoint.Source != "request.external_endpoints" || endpoint.Confidence != PhysicalEndpointConfidenceHigh {
		t.Fatalf("normalized endpoint metadata = %#v", endpoint)
	}
	if len(endpoint.Roles) != 2 || endpoint.Roles[0] != "power" || endpoint.Roles[1] != "edge" {
		t.Fatalf("normalized roles = %#v", endpoint.Roles)
	}
	if len(endpoint.Layers) != 2 || endpoint.Layers[0] != "F.Cu" || endpoint.Layers[1] != "Edge.Cuts" {
		t.Fatalf("normalized layers = %#v", endpoint.Layers)
	}
	endpoint.Point.XMM = 99
	if request.ExternalEndpoints[0].Point.XMM != 0 {
		t.Fatalf("NormalizeRequest did not clone endpoint point")
	}
}

func TestValidateRequestAcceptsExternalEndpoints(t *testing.T) {
	request := validRequest()
	request.ExternalEndpoints = []ExternalEndpointSpec{
		{
			ID:       "edge_sig",
			Kind:     PhysicalEndpointBoardEdgePoint,
			NetName:  "SIG",
			Roles:    []string{"signal", "edge"},
			Point:    &transactions.Point{XMM: 0, YMM: 10},
			Edge:     "left",
			Required: true,
		},
		{
			ID:      "mechanical_vin",
			Kind:    PhysicalEndpointImportedMechanicalPoint,
			NetName: "VIN",
			Roles:   []string{"power_entry"},
			Layers:  []string{},
			Point:   &transactions.Point{XMM: 5, YMM: 10},
		},
		{
			ID:     "advisory_no_point",
			Kind:   PhysicalEndpointImportedMechanicalPoint,
			Roles:  []string{"mechanical_interface"},
			Source: "import.fixture",
		},
	}

	if issues := ValidateRequest(request); len(issues) != 0 {
		t.Fatalf("ValidateRequest issues = %#v", issues)
	}
}

func TestValidateRequestRejectsInvalidExternalEndpointDeclarations(t *testing.T) {
	request := validRequest()
	request.ExternalEndpoints = []ExternalEndpointSpec{
		{ID: "!!!", Kind: PhysicalEndpointKind("pad")},
		{ID: "edge sig", Kind: PhysicalEndpointBoardEdgePoint, Required: true, Edge: "north", Confidence: PhysicalEndpointConfidence("maybe")},
		{ID: "edge-sig", Kind: PhysicalEndpointBoardEdgePoint},
		{ID: "board_edge_point_spoof", Kind: PhysicalEndpointBoardEdgePoint},
	}

	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "external_endpoints[0].id")
	assertIssuePath(t, issues, "external_endpoints[0].kind")
	assertIssuePath(t, issues, "external_endpoints[1].point")
	assertIssuePath(t, issues, "external_endpoints[1].net_name")
	assertIssuePath(t, issues, "external_endpoints[1].edge")
	assertIssuePath(t, issues, "external_endpoints[1].confidence")
	assertIssuePath(t, issues, "external_endpoints[2].id")
	assertIssuePath(t, issues, "external_endpoints[3].id")
}

func TestValidateRequestRejectsExternalEndpointBoundsAndLayers(t *testing.T) {
	request := validRequest()
	request.ExternalEndpoints = []ExternalEndpointSpec{
		{
			ID:      "negative",
			Kind:    PhysicalEndpointBoardEdgePoint,
			Point:   &transactions.Point{XMM: -0.01, YMM: 0},
			NetName: "SIG",
		},
		{
			ID:      "too_wide",
			Kind:    PhysicalEndpointBoardEdgePoint,
			Point:   &transactions.Point{XMM: request.Board.WidthMM + 0.01, YMM: 0},
			NetName: "SIG",
		},
		{
			ID:      "inner_layer",
			Kind:    PhysicalEndpointImportedMechanicalPoint,
			NetName: "SIG",
			Layers:  []string{"In1.Cu"},
		},
		{
			ID:       "technical_only",
			Kind:     PhysicalEndpointImportedMechanicalPoint,
			NetName:  "SIG",
			Layers:   []string{"Edge.Cuts"},
			Required: true,
		},
		{
			ID:     "bad_layer",
			Kind:   PhysicalEndpointImportedMechanicalPoint,
			Layers: []string{"F.SilkS"},
		},
	}

	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "external_endpoints[0].point.x_mm")
	assertIssuePath(t, issues, "external_endpoints[1].point.x_mm")
	assertIssuePath(t, issues, "external_endpoints[2].layers[0]")
	assertIssuePath(t, issues, "external_endpoints[3].layers")
	assertIssuePath(t, issues, "external_endpoints[4].layers[0]")
}

func TestDecodeRequestStrictAcceptsRoutingRetryPolicy(t *testing.T) {
	request, issues := DecodeRequestStrict(strings.NewReader(`{
	  "version": "0.1.0",
	  "name": "demo",
	  "board": {"width_mm": 10, "height_mm": 10},
	  "blocks": [{"id": "led", "block_id": "led_indicator"}],
	  "routing_retry": {"enabled": true, "max_attempts": 3, "allowed_hint_categories": ["increase_spacing"], "drc_policy": "optional"}
	}`))
	if len(issues) != 0 {
		t.Fatalf("DecodeRequestStrict issues = %#v", issues)
	}
	if !request.RoutingRetry.Enabled || request.RoutingRetry.MaxAttempts != 3 || request.RoutingRetry.AllowedHintCategories[0] != PlacementRetryIncreaseSpacing || request.RoutingRetry.DRCPolicy != RetryDRCPolicyOptional {
		t.Fatalf("routing retry = %#v", request.RoutingRetry)
	}
}

func validRequest() Request {
	return Request{
		Version: RequestVersion,
		Name:    "sensor_breakout",
		Board:   BoardSpec{WidthMM: 50, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "sensor", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48"}},
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"SDA", "SCL", "GND"}}},
		},
		Connections: []ConnectionSpec{{From: "sensor.SDA", To: "header.SDA", NetAlias: "SDA"}},
		Validation:  ValidationSpec{Acceptance: AcceptanceConnectivity},
	}
}

func assertIssuePath(t *testing.T, issues []reports.Issue, path string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Path == path {
			return
		}
	}
	t.Fatalf("missing issue path %q in %#v", path, issues)
}
