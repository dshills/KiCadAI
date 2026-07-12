package aiprovider

import (
	"context"
	"os"
	"path/filepath"
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

func designBlockByID(request designworkflow.Request, id string) designworkflow.BlockInstanceSpec {
	for _, block := range request.Blocks {
		if block.ID == id {
			return block
		}
	}
	return designworkflow.BlockInstanceSpec{}
}
