package blocks

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestClassAVoltageStageCalculatesAndInstantiatesBJT(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    classAVoltageStageID,
		InstanceID: "preamp",
		Params: map[string]any{
			"device_technology":              "bjt",
			"device_component_id":            "bjt.onsemi.mmbt3904.sot23",
			"device_symbol":                  "Device:Q_NPN_BEC",
			"device_footprint":               "Package_TO_SOT_SMD:SOT-23",
			"supply_voltage":                 "12V",
			"target_quiescent_current":       "1mA",
			"target_output_bias":             "6V",
			"target_gain":                    10.0,
			"control_drop_voltage":           "0.65V",
			"bias_divider_current":           "100uA",
			"minimum_current_gain":           100.0,
			"input_impedance":                "47kΩ",
			"load_impedance":                 "10kΩ",
			"low_frequency_cutoff":           "20Hz",
			"coupling_policy":                "ac_both",
			"resistor_footprint":             "Resistor_SMD:R_0805_2012Metric",
			"coupling_capacitor_footprint":   "Capacitor_SMD:C_0805_2012Metric",
			"decoupling_capacitor_footprint": "Capacitor_SMD:C_0805_2012Metric",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 8 {
		t.Fatalf("refs = %#v, want 8 Class A components", output.Instance.Refs)
	}
	for _, net := range []string{"preamp_in", "preamp_control", "preamp_source_or_emitter", "preamp_output_dc", "preamp_out", "preamp_vcc", "preamp_agnd"} {
		if !slices.Contains(output.Instance.Nets, net) {
			t.Fatalf("nets = %#v, want %s", output.Instance.Nets, net)
		}
	}
	if got := output.Instance.Params["calculated_stage_load_ohms"].(float64); math.Abs(got-6000) > 1e-9 {
		t.Fatalf("calculated load = %g, want 6000", got)
	}
	baseCurrent := output.Instance.Params["calculated_conservative_base_current_a"].(float64)
	if math.Abs(baseCurrent-10e-6) > 1e-15 {
		t.Fatalf("calculated base current = %g, want 10uA", baseCurrent)
	}
	controlBias := output.Instance.Params["calculated_control_bias_v"].(float64)
	biasTop := output.Instance.Params["calculated_bias_top_ohms"].(float64)
	wantBiasTop := (12 - controlBias) / (100e-6 + baseCurrent)
	if math.Abs(biasTop-wantBiasTop) > wantBiasTop*1e-12 {
		t.Fatalf("calculated upper bias resistor = %g, want %g with base-current loading", biasTop, wantBiasTop)
	}
	if got := output.Instance.Params["calculated_device_dissipation_w"].(float64); got <= 0 || got >= 0.01 {
		t.Fatalf("calculated device dissipation = %g, want bounded low-power result", got)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
	if labels := classABLabels(t, output.Operations); len(labels) != 0 {
		t.Fatalf("Class A stage should rely on materialized port labels: %#v", labels)
	}
}

func TestClassAVoltageStageLowersMOSFETTerminalContract(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    classAVoltageStageID,
		InstanceID: "mos",
		Params: map[string]any{
			"device_technology":    "mosfet",
			"device_component_id":  "mosfet.vishay.irfp240.to247",
			"device_symbol":        "Device:Q_NMOS_GDS",
			"device_footprint":     "Package_TO_SOT_THT:TO-247-3_Vertical",
			"control_drop_voltage": "3V",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	deviceRef := output.Instance.Refs[3]
	foundDrain := false
	foundSource := false
	for _, operation := range output.Operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.From.Ref == deviceRef || payload.To.Ref == deviceRef {
			pin := payload.From.Pin
			if payload.To.Ref == deviceRef {
				pin = payload.To.Pin
			}
			foundDrain = foundDrain || (pin == "2" && payload.NetName == "mos_output_dc")
			foundSource = foundSource || (pin == "3" && payload.NetName == "mos_source_or_emitter")
		}
	}
	if !foundDrain || !foundSource {
		t.Fatalf("MOSFET terminal contract drain=%t source=%t", foundDrain, foundSource)
	}
	if got := output.Instance.Params["calculated_intrinsic_device_resistance_ohms"].(float64); math.Abs(got-20) > 1e-9 {
		t.Fatalf("MOSFET intrinsic source resistance = %g, want 20 ohms from minimum gm", got)
	}
}

func TestClassAVoltageStageBlocksMOSFETGainBeyondReviewedTransconductance(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    classAVoltageStageID,
		InstanceID: "low_gm",
		Params: map[string]any{
			"device_technology":          "mosfet",
			"device_component_id":        "mosfet.vishay.irfp240.to247",
			"device_symbol":              "Device:Q_NMOS_GDS",
			"device_footprint":           "Package_TO_SOT_THT:TO-247-3_Vertical",
			"control_drop_voltage":       "3V",
			"minimum_transconductance_s": 0.0001,
		},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want low-gm gain request blocked", issues)
	}
	found := false
	for _, issue := range issues {
		found = found || issue.Path == "params.target_gain"
	}
	if !found {
		t.Fatalf("issues = %#v, want target_gain diagnostic", issues)
	}
}

func TestClassAVoltageStageBlocksUnsafeOperatingPoints(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    classAVoltageStageID,
		InstanceID: "unsafe",
		Params: map[string]any{
			"target_output_bias":       "12V",
			"target_gain":              1000.0,
			"bias_divider_current":     "1uA",
			"minimum_current_gain":     100.0,
			"target_quiescent_current": "10mA",
		},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want unsafe operating point blocked", issues)
	}
	for _, path := range []string{"params.target_output_bias", "params.bias_divider_current"} {
		found := false
		for _, issue := range issues {
			found = found || issue.Path == path
		}
		if !found {
			t.Fatalf("issues = %#v, want path %s", issues, path)
		}
	}
}

func TestPreferredCapacitanceAtLeast(t *testing.T) {
	for _, test := range []struct{ in, want float64 }{{0.9e-6, 1e-6}, {1.01e-6, 1.2e-6}, {8.3e-6, 10e-6}} {
		if got := preferredCapacitanceAtLeast(test.in); math.Abs(got-test.want) > test.want*1e-12 {
			t.Fatalf("preferredCapacitanceAtLeast(%g) = %g, want %g", test.in, got, test.want)
		}
	}
}
