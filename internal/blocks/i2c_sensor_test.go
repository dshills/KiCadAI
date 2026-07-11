package blocks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestI2CSensorInstantiatesDefaultOperations(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "temp",
		Params: map[string]any{
			"i2c_address": "0x48",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 4 || !strings.HasPrefix(got[0], "U") || !strings.HasPrefix(got[1], "C") {
		t.Fatalf("refs = %#v", got)
	}
	if len(output.Instance.Ports) != 4 {
		t.Fatalf("ports = %#v", output.Instance.Ports)
	}
	if len(output.Operations) != 23 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	if !hasNoConnect(output.Operations, output.Instance.Refs[0], genericI2CSensorPins.INT) {
		t.Fatalf("operations missing no-connect for disabled INT pin: %#v", output.Operations)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestI2CSensorUsesCalibratedPhysicalAnchors(t *testing.T) {
	pins := i2cSensorPins(genericI2CSensorPins)
	want := []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: 3.81},
		{Number: "2", XMM: -2.54, YMM: -3.81},
		{Number: "3", XMM: -2.54, YMM: 1.27},
		{Number: "4", XMM: -2.54, YMM: -1.27},
		{Number: "5", XMM: 2.54, YMM: 0},
	}
	if len(pins) != len(want) {
		t.Fatalf("sensor pin count = %d, want %d", len(pins), len(want))
	}
	for index := range want {
		if pins[index] != want[index] {
			t.Fatalf("sensor pin %d = %#v, want %#v", index+1, pins[index], want[index])
		}
	}
}

func TestI2CSensorDecouplingUsesKiCadCapacitorPinAnchors(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "temp",
		Params: map[string]any{
			"i2c_address": "0x48",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	capRef := output.Instance.Refs[1]
	for _, operation := range output.Operations {
		if operation.Op != transactions.OpAddSymbol || operation.Ref != capRef {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("unmarshal add-symbol operation: %v", err)
		}
		pins := payload.Pins
		if len(pins) != 2 || pins[0].Number != "1" || pins[0].XMM != 0 || pins[0].YMM != 3.81 || pins[1].Number != "2" || pins[1].XMM != 0 || pins[1].YMM != -3.81 {
			t.Fatalf("decoupling capacitor pins = %#v, want KiCad Device:C anchors", pins)
		}
		return
	}
	t.Fatalf("missing decoupling capacitor add-symbol operation for %s: %#v", capRef, output.Operations)
}

func TestI2CSensorWithoutPullupsStillExportsBus(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "imu",
		Params: map[string]any{
			"i2c_address":     "0x68",
			"include_pullups": false,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Nets; len(got) != 4 || got[2] != "imu_sda" || got[3] != "imu_scl" {
		t.Fatalf("nets = %#v", got)
	}
}

func TestI2CSensorSkipsDisabledPassiveValidation(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "bare",
		Params: map[string]any{
			"i2c_address":          "0x49",
			"include_pullups":      false,
			"pullup_footprint":     "",
			"include_decoupling":   false,
			"decoupling_footprint": "",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestI2CSensorUsesCustomPassiveFootprints(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "light",
		Params: map[string]any{
			"i2c_address":          "0x23",
			"pullup_footprint":     "Resistor_SMD:R_0603_1608Metric",
			"decoupling_value":     "1uF",
			"decoupling_footprint": "Capacitor_SMD:C_0603_1608Metric",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	data, err := json.Marshal(output.Operations)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"Resistor_SMD:R_0603_1608Metric", "Capacitor_SMD:C_0603_1608Metric", `"value":"1uF"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("operations missing %q: %s", want, text)
		}
	}
}

func TestI2CSensorRejectsInvalidAddress(t *testing.T) {
	registry := NewBuiltinRegistry()
	for _, address := range []string{"0x07", "0x78", "0x88"} {
		_, issues := registry.Instantiate(context.Background(), BlockRequest{
			BlockID:    "i2c_sensor",
			InstanceID: "bad",
			Params: map[string]any{
				"i2c_address": address,
			},
		})
		if len(issues) != 1 || issues[0].Path != "params.i2c_address" {
			t.Fatalf("address %s issues = %#v", address, issues)
		}
	}
}

func TestI2CSensorOptionalInterruptExportsPort(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "accel",
		Params: map[string]any{
			"i2c_address":       "0x1d",
			"include_interrupt": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Ports) != 5 || output.Instance.Ports[4].Name != "INT" {
		t.Fatalf("ports = %#v", output.Instance.Ports)
	}
}

func TestI2CSensorConcreteProfilesEmitVerifiedTopology(t *testing.T) {
	tests := []struct {
		name          string
		componentID   string
		address       string
		symbolID      string
		footprintID   string
		vccPins       []string
		gndPins       []string
		sdaPin        string
		sclPin        string
		noConnectPins []string
	}{
		{name: "bme280", componentID: "sensor.bosch.bme280.lga8", address: "0x76", symbolID: "Sensor:BME280", footprintID: "Package_LGA:Bosch_LGA-8_2.5x2.5mm_P0.65mm_ClockwisePinNumbering", vccPins: []string{"8", "6", "2"}, gndPins: []string{"1", "7", "5"}, sdaPin: "3", sclPin: "4"},
		{name: "bmp280", componentID: "sensor.bosch.bmp280.lga8", address: "0x77", symbolID: "Sensor_Pressure:BMP280", footprintID: "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering", vccPins: []string{"8", "6", "2", "5"}, gndPins: []string{"1", "7"}, sdaPin: "3", sclPin: "4"},
		{name: "sht31", componentID: "sensor.sensirion.sht31_dis.dfn8", address: "0x44", symbolID: "Sensor_Humidity:SHT31-DIS", footprintID: "Sensor_Humidity:Sensirion_DFN-8-1EP_2.5x2.5mm_P0.5mm_EP1.1x1.7mm", vccPins: []string{"5"}, gndPins: []string{"8", "9", "2", "7"}, sdaPin: "1", sclPin: "4", noConnectPins: []string{"3", "6"}},
	}
	registry := NewBuiltinRegistry()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, issues := registry.Instantiate(context.Background(), BlockRequest{
				BlockID:    "i2c_sensor",
				InstanceID: tt.name,
				Params: map[string]any{
					"sensor_component_id": tt.componentID,
					"i2c_address":         tt.address,
				},
			})
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("issues = %#v", issues)
			}
			ref := output.Instance.Refs[0]
			symbol := addSymbolOperations(t, output.Operations)[ref]
			if symbol.LibraryID != tt.symbolID || !strings.HasSuffix(symbol.Value, " "+tt.address) {
				t.Fatalf("symbol = %#v", symbol)
			}
			if got := stringParam(output.Instance.Params, "sensor_footprint"); got != tt.footprintID {
				t.Fatalf("footprint param = %q, want %q", got, tt.footprintID)
			}
			vccNet := InstanceNetName(tt.name, "vcc")
			gndNet := InstanceNetName(tt.name, "gnd")
			for _, pin := range tt.vccPins {
				if !hasPinOnNet(output.Operations, ref, pin, vccNet) {
					t.Errorf("pin %s missing from %s", pin, vccNet)
				}
			}
			for _, pin := range tt.gndPins {
				if !hasPinOnNet(output.Operations, ref, pin, gndNet) {
					t.Errorf("pin %s missing from %s", pin, gndNet)
				}
			}
			if !hasPinOnNet(output.Operations, ref, tt.sdaPin, InstanceNetName(tt.name, "sda")) || !hasPinOnNet(output.Operations, ref, tt.sclPin, InstanceNetName(tt.name, "scl")) {
				t.Errorf("missing I2C bus pin connectivity")
			}
			for _, pin := range tt.noConnectPins {
				if !hasNoConnect(output.Operations, ref, pin) {
					t.Errorf("pin %s missing no-connect", pin)
				}
			}
			if validation := transactions.Validate(transactions.Transaction{Operations: output.Operations}); len(validation.Issues) != 0 {
				t.Fatalf("transaction validation issues = %#v", validation.Issues)
			}
		})
	}
}

func TestI2CSensorSHT31InterruptReplacesAlertNoConnect(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "humidity",
		Params: map[string]any{
			"sensor_component_id": "sensor.sensirion.sht31_dis.dfn8",
			"i2c_address":         "0x45",
			"include_interrupt":   true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	ref := output.Instance.Refs[0]
	if hasNoConnect(output.Operations, ref, "3") || !hasPinOnNet(output.Operations, ref, "3", InstanceNetName("humidity", "int")) {
		t.Fatalf("ALERT pin handling is not connected-only: %#v", output.Operations)
	}
}

func TestI2CSensorConcreteProfilesFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		path   string
	}{
		{name: "unknown component", params: map[string]any{"sensor_component_id": "sensor.unknown", "i2c_address": "0x44"}, path: "params.sensor_component_id"},
		{name: "unsupported address", params: map[string]any{"sensor_component_id": "sensor.bosch.bmp280.lga8", "i2c_address": "0x44"}, path: "params.i2c_address"},
		{name: "mismatched symbol", params: map[string]any{"sensor_component_id": "sensor.bosch.bmp280.lga8", "sensor_symbol": "Sensor:BME280", "i2c_address": "0x76"}, path: "params.sensor_symbol"},
		{name: "mismatched footprint", params: map[string]any{"sensor_component_id": "sensor.bosch.bmp280.lga8", "sensor_footprint": "Package_LGA:Bosch_LGA-8_2.5x2.5mm_P0.65mm_ClockwisePinNumbering", "i2c_address": "0x76"}, path: "params.sensor_footprint"},
		{name: "unsupported interrupt", params: map[string]any{"sensor_component_id": "sensor.bosch.bmp280.lga8", "i2c_address": "0x76", "include_interrupt": true}, path: "params.include_interrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{BlockID: "i2c_sensor", InstanceID: "bad", Params: tt.params})
			if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, tt.path) {
				t.Fatalf("issues = %#v, want blocker at %s", issues, tt.path)
			}
		})
	}
}

func TestI2CSensorDuplicateAddressOnSharedBusBlocks(t *testing.T) {
	output := ComposeBlocks(context.Background(), NewBuiltinRegistry(), CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "a", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48"}},
			{ID: "b", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48"}},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a", Port: "SDA"}, To: PortRef{InstanceID: "b", Port: "SDA"}},
			{From: PortRef{InstanceID: "a", Port: "SCL"}, To: PortRef{InstanceID: "b", Port: "SCL"}},
		},
	})
	if !reports.HasBlockingIssue(output.Issues) {
		t.Fatalf("expected address collision, issues = %#v", output.Issues)
	}
}

func TestI2CSensorProjectTransactionApplies(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "i2c_sensor",
		InstanceID: "temp",
		Params: map[string]any{
			"i2c_address": "0x48",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("temp_sensor", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "temp_sensor")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues = %#v", result.Issues)
	}
	for _, name := range []string{"temp_sensor.kicad_pro", "temp_sensor.kicad_sch", "temp_sensor.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}
