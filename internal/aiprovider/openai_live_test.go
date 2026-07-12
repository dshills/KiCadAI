package aiprovider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"

	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
)

func TestOpenAILiveBMP280Intent(t *testing.T) {
	if os.Getenv("KICADAI_OPENAI_LIVE_TEST") != "1" {
		t.Skip("set KICADAI_OPENAI_LIVE_TEST=1 to run the live provider test")
	}
	prompt, err := os.ReadFile(referenceFixturePath(t, "prompt.txt"))
	if err != nil {
		t.Fatalf("read reference prompt: %v", err)
	}
	provider, err := NewOpenAIProvider(OpenAIOptionsFromEnvironment())
	if err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := provider.GenerateIntent(ctx, GenerateRequest{
		Prompt:            string(prompt),
		CapabilityContext: BMP280ReferenceCapabilityContext,
		OutputSchemaName:  "kicadai_bmp280_intent_v1",
		OutputSchema:      BMP280ReferenceIntentEnvelopeSchema(),
		SchemaVersion:     EnvelopeSchemaV1,
		Attempt:           1,
	})
	if err != nil {
		t.Fatalf("generate reference intent: %v (cause %T: %v)", err, errors.Unwrap(err), errors.Unwrap(err))
	}
	request, issues := DecodeIntent(result.IntentJSON)
	if len(issues) != 0 {
		t.Fatalf("provider intent issues = %#v", issues)
	}
	plan := intentplanner.Plan(request)
	if plan.Status != intentplanner.PlanStatusReady || plan.GeneratedRequest == nil {
		t.Fatalf("provider plan status=%s issues=%#v gaps=%#v", plan.Status, plan.Issues, plan.KnownGaps)
	}
	recordedData, err := os.ReadFile(referenceFixturePath(t, "recorded-response.json"))
	if err != nil {
		t.Fatalf("read recorded reference: %v", err)
	}
	recordedIntentJSON, err := DecodeEnvelope(recordedData)
	if err != nil {
		t.Fatalf("decode recorded reference: %v", err)
	}
	recordedRequest, recordedIssues := DecodeIntent(recordedIntentJSON)
	if len(recordedIssues) != 0 {
		t.Fatalf("recorded intent issues = %#v", recordedIssues)
	}
	recordedPlan := intentplanner.Plan(recordedRequest)
	if recordedPlan.GeneratedRequest == nil {
		t.Fatalf("recorded plan status=%s gaps=%#v", recordedPlan.Status, recordedPlan.KnownGaps)
	}
	live := referenceGeneratedRequestProjectionFor(t, plan.GeneratedRequest)
	recorded := referenceGeneratedRequestProjectionFor(t, recordedPlan.GeneratedRequest)
	assertReferenceProjectionComplete(t, live)
	if !reflect.DeepEqual(live, recorded) {
		t.Fatalf("live generated request differs from recorded reference\nlive=%#v\nrecorded=%#v", live, recorded)
	}
}

type referenceGeneratedRequestProjection struct {
	Board             designworkflow.BoardSpec
	USBProtection     referenceUSBProtectionProjection
	Regulator         referenceRegulatorProjection
	Sensor            referenceSensorProjection
	ConnectorPins     any
	ConnectionAliases []string
}

type referenceUSBProtectionProjection struct {
	Fuse bool
	TVS  bool
	Bulk bool
}

type referenceRegulatorProjection struct {
	Symbol        string
	Footprint     string
	InputMinimum  string
	InputMaximum  string
	OutputVoltage string
	OutputCurrent string
}

type referenceSensorProjection struct {
	ComponentID string
	Pullups     bool
	Decoupling  bool
	FixedLayout bool
}

func referenceGeneratedRequestProjectionFor(t *testing.T, request *designworkflow.Request) referenceGeneratedRequestProjection {
	t.Helper()
	blocksByID := make(map[string]designworkflow.BlockInstanceSpec, len(request.Blocks))
	for _, block := range request.Blocks {
		blocksByID[block.ID] = block
	}
	usb := requireReferenceBlock(t, blocksByID, "usb_power", "usb_c_power")
	regulator := requireReferenceBlock(t, blocksByID, "regulator", "voltage_regulator")
	sensor := requireReferenceBlock(t, blocksByID, "sensor", "i2c_sensor")
	connector := requireReferenceBlock(t, blocksByID, "i2c_connector", "connector_breakout")
	aliases := make([]string, 0, len(request.Connections))
	for _, connection := range request.Connections {
		aliases = append(aliases, connection.From+"->"+connection.To+"="+connection.NetAlias)
	}
	slices.Sort(aliases)
	return referenceGeneratedRequestProjection{
		Board: request.Board,
		USBProtection: referenceUSBProtectionProjection{
			Fuse: parameterBool(t, usb.Params, "include_fuse"),
			TVS:  parameterBool(t, usb.Params, "include_tvs"),
			Bulk: parameterBool(t, usb.Params, "include_bulk_capacitor"),
		},
		Regulator: referenceRegulatorProjection{
			Symbol:        parameterString(t, regulator.Params, "regulator_symbol"),
			Footprint:     parameterString(t, regulator.Params, "regulator_footprint"),
			InputMinimum:  parameterString(t, regulator.Params, "input_voltage_min"),
			InputMaximum:  parameterString(t, regulator.Params, "input_voltage_max"),
			OutputVoltage: parameterString(t, regulator.Params, "output_voltage"),
			OutputCurrent: parameterString(t, regulator.Params, "output_current"),
		},
		Sensor: referenceSensorProjection{
			ComponentID: parameterString(t, sensor.Params, "sensor_component_id"),
			Pullups:     parameterBool(t, sensor.Params, "include_pullups"),
			Decoupling:  parameterBool(t, sensor.Params, "include_decoupling"),
			FixedLayout: parameterBool(t, sensor.Params, "fixed_pcb_layout"),
		},
		ConnectorPins:     connector.Params["pin_names"],
		ConnectionAliases: aliases,
	}
}

func parameterString(t *testing.T, params map[string]any, name string) string {
	t.Helper()
	value, ok := params[name].(string)
	if !ok {
		t.Fatalf("parameter %q = %#v, want string", name, params[name])
	}
	return value
}

func parameterBool(t *testing.T, params map[string]any, name string) bool {
	t.Helper()
	value, ok := params[name].(bool)
	if !ok {
		t.Fatalf("parameter %q = %#v, want bool", name, params[name])
	}
	return value
}

func assertReferenceProjectionComplete(t *testing.T, projection referenceGeneratedRequestProjection) {
	t.Helper()
	if projection.Board.WidthMM != 100 || projection.Board.HeightMM != 75 || projection.Board.Layers != 2 {
		t.Fatalf("reference board = %#v", projection.Board)
	}
	if !projection.USBProtection.Fuse || !projection.USBProtection.TVS || !projection.USBProtection.Bulk {
		t.Fatalf("reference USB protection = %#v", projection.USBProtection)
	}
	if projection.Regulator.Symbol != "Regulator_Linear:AP2112K-3.3" || projection.Regulator.Footprint != "Package_TO_SOT_SMD:SOT-23-5" || projection.Regulator.OutputCurrent != "0.1A" {
		t.Fatalf("reference regulator = %#v", projection.Regulator)
	}
	if projection.Sensor.ComponentID != "sensor.bosch.bmp280.lga8" || !projection.Sensor.Pullups || !projection.Sensor.Decoupling {
		t.Fatalf("reference sensor = %#v", projection.Sensor)
	}
	if len(projection.ConnectionAliases) == 0 || projection.ConnectorPins == nil {
		t.Fatalf("reference interconnect = pins %#v aliases %#v", projection.ConnectorPins, projection.ConnectionAliases)
	}
}

func requireReferenceBlock(t *testing.T, blocksByID map[string]designworkflow.BlockInstanceSpec, id, blockID string) designworkflow.BlockInstanceSpec {
	t.Helper()
	block, ok := blocksByID[id]
	if !ok || block.BlockID != blockID {
		t.Fatalf("reference block %q = %#v, want block_id %q", id, block, blockID)
	}
	return block
}

func referenceFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate live provider test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", "..", "examples", "ai", "usb_c_bmp280_breakout", name))
}
