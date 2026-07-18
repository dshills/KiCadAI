package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/transactions"
)

func TestClassABSpeakerPowerStageDefinitionContract(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock(classABSpeakerPowerStageID)
	if !ok {
		t.Fatalf("missing block %s", classABSpeakerPowerStageID)
	}
	for _, port := range []string{"DRIVER_OUT", "VCC", "VEE", "POWER_STAR", "RAW_OUT"} {
		if !classABHasPort(definition, port) {
			t.Fatalf("missing port %s", port)
		}
	}
	roles := blockComponentByRole(definition.Components)
	for _, role := range []string{"upper_driver_base_stopper", "lower_driver_base_stopper", "upper_driver", "lower_driver", "upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor", "upper_current_limit", "lower_current_limit", "zobel_resistor", "zobel_capacitor"} {
		if _, ok := roles[role]; !ok {
			t.Fatalf("missing component role %s", role)
		}
	}
	if roles["upper_driver"].SymbolID != "Transistor_BJT:Q_NPN_ECB" || roles["upper_output"].SymbolID != "Transistor_BJT:Q_NPN_BCE" {
		t.Fatalf("driver/output pin contracts = %#v %#v", roles["upper_driver"], roles["upper_output"])
	}
	if definition.PCBRealization == nil || definition.PCBRealization.FabricationReadiness {
		t.Fatalf("new block must carry PCB metadata without claiming unproven fabrication readiness: %#v", definition.PCBRealization)
	}
	for _, category := range []PCBConstraintCategory{PCBConstraintReturnTopology, PCBConstraintFeedbackSense, PCBConstraintKelvinSense, PCBConstraintCurrentPath, PCBConstraintThermalCoupling, PCBConstraintDeviceSymmetry, PCBConstraintThermalKeepout, PCBConstraintAnalogInputSeparation} {
		if !slices.ContainsFunc(definition.PCBRealization.Constraints, func(constraint PCBConstraint) bool { return constraint.Category == category }) {
			t.Fatalf("missing PCB constraint category %s: %#v", category, definition.PCBRealization.Constraints)
		}
	}
}

func TestClassABSpeakerPowerStageInstantiatesTenWattEnvelope(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: classABSpeakerPowerStageID, InstanceID: "speaker_power"})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if got := output.Instance.Params["available_output_power_w"].(float64); got < 10 {
		t.Fatalf("available output power = %g", got)
	}
	if got := output.Instance.Params["sense_threshold_v"].(float64); got < 0.659 || got > 0.661 {
		t.Fatalf("sense threshold = %g, want 0.66 V", got)
	}
	if len(output.Instance.Refs) != 18 || classABOperationCount(output.Operations, transactions.OpAddSymbol) != 18 {
		t.Fatalf("refs/operations = %d/%d, want eighteen components", len(output.Instance.Refs), classABOperationCount(output.Operations, transactions.OpAddSymbol))
	}
	if validation := transactions.Validate(transactions.Transaction{Operations: output.Operations}); len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestClassABSpeakerPowerStageBlocksUnderpoweredEnvelope(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID: classABSpeakerPowerStageID, InstanceID: "underpowered",
		Params: map[string]any{"supply_voltage": "24V", "target_power": 10.0, "target_load": "8Ω"},
	})
	if len(issues) == 0 || len(output.Operations) != 0 {
		t.Fatalf("underpowered result issues=%#v operations=%d", issues, len(output.Operations))
	}
}

func TestClassABSpeakerPowerStageBlocksInconsistentBJTCurrentLimit(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID: classABSpeakerPowerStageID, InstanceID: "bad_limit",
		Params: map[string]any{"current_limit": "1.7A", "current_limit_vbe": "0.66V", "emitter_resistor_value": "0.22Ω"},
	})
	if len(issues) == 0 || len(output.Operations) != 0 {
		t.Fatalf("inconsistent limiter result issues=%#v operations=%d", issues, len(output.Operations))
	}
}

func TestClassABSpeakerPowerStageUsesDeclaredCurrentLimitVBETolerance(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID: classABSpeakerPowerStageID, InstanceID: "bounded_limit",
		Params: map[string]any{"current_limit": "2.8A", "current_limit_vbe": "0.66V", "current_limit_vbe_tolerance": "0.05V", "emitter_resistor_value": "0.22Ω"},
	})
	if len(issues) != 0 || len(output.Operations) == 0 {
		t.Fatalf("declared limiter tolerance result issues=%#v operations=%d", issues, len(output.Operations))
	}
}

func TestClassABSpeakerPowerStageAcceptsFeasibleLowerPowerEnvelope(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID: classABSpeakerPowerStageID, InstanceID: "five_watt",
		Params: map[string]any{"target_power": 5.0, "target_load": "8Ω"},
	})
	if len(issues) != 0 || len(output.Operations) == 0 {
		t.Fatalf("feasible lower-power result issues=%#v operations=%d", issues, len(output.Operations))
	}
}

func TestClassABSpeakerPowerStageAcceptsSixteenOhmEnvelope(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID: classABSpeakerPowerStageID, InstanceID: "sixteen_ohm",
		Params: map[string]any{"target_power": 5.0, "target_load": "16Ω", "minimum_load": "8Ω"},
	})
	if len(issues) != 0 || len(output.Operations) == 0 {
		t.Fatalf("sixteen-ohm result issues=%#v operations=%d", issues, len(output.Operations))
	}
}

func TestClassABSpeakerPowerStagePCBRealizationIsComplete(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, _ := registry.GetBlock(classABSpeakerPowerStageID)
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: classABSpeakerPowerStageID, InstanceID: "speaker_power"})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 10, OriginYMM: 20})
	if len(realized.Issues) != 0 {
		t.Fatalf("PCB realization issues = %#v", realized.Issues)
	}
	if len(realized.Components) != 18 || len(realized.LocalRoutes) != len(definition.PCBRealization.LocalRoutes) {
		t.Fatalf("realized components/routes = %d/%d want 18/%d", len(realized.Components), len(realized.LocalRoutes), len(definition.PCBRealization.LocalRoutes))
	}
	for _, routeID := range definition.PCBRealization.Validation.RequiredRoutes {
		if !slices.ContainsFunc(realized.LocalRoutes, func(route RealizedPCBLocalRoute) bool { return route.ID == routeID }) {
			t.Fatalf("missing required realized route %s", routeID)
		}
	}
}
