package designworkflow

import (
	"strings"
	"testing"

	"kicadai/internal/reports"
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

func TestValidateRequestRejectsUnknownAcceptance(t *testing.T) {
	request := validRequest()
	request.Validation.Acceptance = "magic"
	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "validation.acceptance")
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

func TestValidateRequestRejectsInvalidRoutingRetryPolicy(t *testing.T) {
	request := validRequest()
	request.RoutingRetry = RoutingRetryPolicySpec{
		MaxAttempts:          -1,
		MinRoutingScoreDelta: -0.1,
		AllowedHintCategories: []PlacementRetryHintCategory{
			PlacementRetryHintCategory("teleport"),
		},
	}

	issues := ValidateRequest(request)
	assertIssuePath(t, issues, "routing_retry.max_attempts")
	assertIssuePath(t, issues, "routing_retry.min_routing_score_delta")
	assertIssuePath(t, issues, "routing_retry.allowed_hint_categories[0]")
}

func TestDecodeRequestStrictAcceptsRoutingRetryPolicy(t *testing.T) {
	request, issues := DecodeRequestStrict(strings.NewReader(`{
	  "version": "0.1.0",
	  "name": "demo",
	  "board": {"width_mm": 10, "height_mm": 10},
	  "blocks": [{"id": "led", "block_id": "led_indicator"}],
	  "routing_retry": {"enabled": true, "max_attempts": 3, "allowed_hint_categories": ["increase_spacing"]}
	}`))
	if len(issues) != 0 {
		t.Fatalf("DecodeRequestStrict issues = %#v", issues)
	}
	if !request.RoutingRetry.Enabled || request.RoutingRetry.MaxAttempts != 3 || request.RoutingRetry.AllowedHintCategories[0] != PlacementRetryIncreaseSpacing {
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
