package aiprovider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
)

func TestRecordedBMP280ReferenceProducesReadyProtectedPlan(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "ai", "usb_c_bmp280_breakout", "recorded-response.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read reference response: %v", err)
	}
	provider, err := NewRecordedProvider("usb_c_bmp280_breakout", data)
	if err != nil {
		t.Fatalf("new recorded provider: %v", err)
	}
	result, err := provider.GenerateIntent(context.Background(), GenerateRequest{
		Prompt:        "Create a protected USB-C powered BMP280 I2C breakout with a 3.3 V regulator, pull-ups, decoupling, and an external I2C connector.",
		SchemaVersion: EnvelopeSchemaV1,
		Attempt:       1,
	})
	if err != nil {
		t.Fatalf("generate intent: %v", err)
	}
	request, issues := DecodeIntent(result.IntentJSON)
	if len(issues) != 0 {
		t.Fatalf("decode intent issues = %#v", issues)
	}
	plan := intentplanner.Plan(request)
	if plan.Status != intentplanner.PlanStatusReady || plan.GeneratedRequest == nil {
		t.Fatalf("plan status=%s issues=%#v gaps=%#v", plan.Status, plan.Issues, plan.KnownGaps)
	}
	usb := designBlockByID(*plan.GeneratedRequest, "usb_power")
	for _, key := range []string{"include_fuse", "include_tvs", "include_bulk_capacitor"} {
		if usb.Params[key] != true {
			t.Fatalf("usb_power %s=%#v, want true; params=%#v", key, usb.Params[key], usb.Params)
		}
	}
	sensor := designBlockByID(*plan.GeneratedRequest, "sensor")
	if sensor.Params["sensor_component_id"] != "sensor.bosch.bmp280.lga8" || sensor.Params["include_pullups"] != true || sensor.Params["include_decoupling"] != true {
		t.Fatalf("sensor params = %#v", sensor.Params)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request issues = %#v", issues)
	}
}

func TestRecordedProtectedLEDReferenceMatchesDeterministicFixture(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "examples", "ai", "usb_c_led_indicator_protected")
	data, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewRecordedProvider("usb_c_led_indicator_protected", data)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateIntent(context.Background(), GenerateRequest{
		Prompt:        "Create a protected USB-C powered active-high LED indicator with a fuse, TVS, and bulk capacitance.",
		SchemaVersion: EnvelopeSchemaV1,
		Attempt:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, issues := DecodeIntent(result.IntentJSON)
	if len(issues) != 0 {
		t.Fatalf("decode intent issues = %#v", issues)
	}
	plan := intentplanner.Plan(request)
	if plan.Status != intentplanner.PlanStatusReady || plan.GeneratedRequest == nil {
		t.Fatalf("plan status=%s issues=%#v gaps=%#v", plan.Status, plan.Issues, plan.KnownGaps)
	}
	deterministicData, err := os.ReadFile(filepath.Join("..", "..", "examples", "design", "kicad-backed", "usb_c_led_indicator_protected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var deterministic designworkflow.Request
	if err := json.Unmarshal(deterministicData, &deterministic); err != nil {
		t.Fatal(err)
	}
	got := protectedLEDProjectionFor(t, *plan.GeneratedRequest)
	want := protectedLEDProjectionFor(t, deterministic)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded projection differs from deterministic fixture\ngot=%#v\nwant=%#v", got, want)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request issues = %#v", issues)
	}
}

type protectedLEDProjection struct {
	Board         designworkflow.BoardSpec
	AutoLayout    bool
	USBProtection [3]bool
	LEDParams     [4]string
	Connections   []string
	Acceptance    designworkflow.AcceptanceLevel
	RequireERC    bool
	RequireDRC    bool
}

func protectedLEDProjectionFor(t *testing.T, request designworkflow.Request) protectedLEDProjection {
	t.Helper()
	usb := designBlockByID(request, "usb_power")
	indicator := designBlockByID(request, "indicator")
	if usb.BlockID != "usb_c_power" || indicator.BlockID != "led_indicator" {
		t.Fatalf("unexpected blocks: %#v", request.Blocks)
	}
	connections := make([]string, 0, len(request.Connections))
	for _, connection := range request.Connections {
		connections = append(connections, connection.NetAlias+":"+connection.From+"->"+connection.To)
	}
	slices.Sort(connections)
	return protectedLEDProjection{
		Board:      request.Board,
		AutoLayout: request.AutoSchematicLayout,
		USBProtection: [3]bool{
			usb.Params["include_fuse"] == true,
			usb.Params["include_tvs"] == true,
			usb.Params["include_bulk_capacitor"] == true,
		},
		LEDParams: [4]string{
			stringParam(t, indicator.Params, "supply_voltage"),
			stringParam(t, indicator.Params, "led_forward_voltage"),
			stringParam(t, indicator.Params, "led_current"),
			stringParam(t, indicator.Params, "resistor_value"),
		},
		Connections: connections,
		Acceptance:  request.Validation.Acceptance,
		RequireERC:  request.Validation.RequireERC,
		RequireDRC:  request.Validation.RequireDRC,
	}
}

func stringParam(t *testing.T, params map[string]any, name string) string {
	t.Helper()
	value, ok := params[name].(string)
	if !ok {
		t.Fatalf("parameter %q = %#v, want string", name, params[name])
	}
	return value
}

func designBlockByID(request designworkflow.Request, id string) designworkflow.BlockInstanceSpec {
	for _, block := range request.Blocks {
		if block.ID == id {
			return block
		}
	}
	return designworkflow.BlockInstanceSpec{}
}
