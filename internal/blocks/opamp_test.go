package blocks

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestOpAmpGainStageInstantiatesNonInvertingGain(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"gain": 2.0,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 4 || !strings.HasPrefix(got[0], "U") || !strings.HasPrefix(got[1], "R") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) != 5 || got[2] != "amp_feedback" {
		t.Fatalf("nets = %#v", got)
	}
	if !feedbackRatioClose(2.0, 10000, 10000) {
		t.Fatal("feedback ratio helper failed")
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestOpAmpGainStageRejectsInvalidGainAndTopology(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "bad",
		Params: map[string]any{
			"gain":     1.0,
			"topology": "inverting",
		},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestOpAmpGainStageACCouplingAddsBiasNetwork(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"input_coupling": "ac",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Nets; len(got) != 6 || got[5] != "amp_bias" {
		t.Fatalf("nets = %#v", got)
	}
	definition, ok := registry.GetBlock("opamp_gain_stage")
	if !ok {
		t.Fatal("missing opamp gain stage definition")
	}
	topology := projectBlockTopology(t, definition, "amp", output.Instance.Params, output.Operations)
	biasNet := InstanceNetName("amp", "bias")
	topology.requirePinNet(t, "opamp", lmv321Pins.INP, biasNet)
	topology.requirePinNet(t, "gain_to_ground", "2", biasNet)
	emittedGain, outputDCFraction := opAmpACGainAndDCFraction(t, topology)
	if math.Abs(emittedGain-2) > 1e-12 {
		t.Fatalf("AC small-signal gain = %g, want 2", emittedGain)
	}
	if math.Abs(outputDCFraction-0.5) > 1e-12 {
		t.Fatalf("AC-coupled DC output fraction = %g, want midpoint 0.5", outputDCFraction)
	}
}

func TestOpAmpGainStageACCouplingPreservesGainAndMidpointAcrossSupportedValues(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("opamp_gain_stage")
	if !ok {
		t.Fatal("missing opamp gain stage definition")
	}
	for _, gain := range []float64{2, 3, 10} {
		t.Run(fmt.Sprintf("gain_%g", gain), func(t *testing.T) {
			output, issues := registry.Instantiate(context.Background(), BlockRequest{
				BlockID:    "opamp_gain_stage",
				InstanceID: "amp",
				Params: map[string]any{
					"gain":           gain,
					"input_coupling": "ac",
				},
			})
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("issues = %#v", issues)
			}
			topology := projectBlockTopology(t, definition, "amp", output.Instance.Params, output.Operations)
			emittedGain, outputDCFraction := opAmpACGainAndDCFraction(t, topology)
			if relativeError := math.Abs(emittedGain-gain) / gain; relativeError > 0.02 {
				t.Fatalf("AC small-signal gain = %g, want %g within 2%%", emittedGain, gain)
			}
			if math.Abs(outputDCFraction-0.5) > 1e-12 {
				t.Fatalf("AC-coupled DC output fraction = %g, want midpoint 0.5", outputDCFraction)
			}
		})
	}
}

func opAmpACGainAndDCFraction(t *testing.T, topology projectedBlockTopology) (float64, float64) {
	t.Helper()
	biasTop, topOK := parseUnit(topology.symbolsByRole["bias_top"].Value, "Ω", resistanceMultipliers())
	biasBottom, bottomOK := parseUnit(topology.symbolsByRole["bias_bottom"].Value, "Ω", resistanceMultipliers())
	rg, rgOK := parseUnit(topology.symbolsByRole["gain_to_ground"].Value, "Ω", resistanceMultipliers())
	rf, rfOK := parseUnit(topology.symbolsByRole["feedback"].Value, "Ω", resistanceMultipliers())
	if !topOK || !bottomOK || !rgOK || !rfOK || biasTop <= 0 || biasBottom <= 0 || rg <= 0 || rf <= 0 {
		t.Fatalf("invalid emitted AC-coupled resistor values: top=%g bottom=%g rg=%g rf=%g", biasTop, biasBottom, rg, rf)
	}
	midpointFraction := biasBottom / (biasTop + biasBottom)
	if math.Abs(midpointFraction-0.5) > 1e-12 {
		t.Fatalf("bias divider fraction = %g, want 0.5", midpointFraction)
	}
	vPlus := midpointFraction
	feedbackReference := midpointFraction
	outputDCFraction := feedbackReference + (vPlus-feedbackReference)*(1+rf/rg)
	return 1 + rf/rg, outputDCFraction
}

func TestOpAmpGainStageACCouplingRealizesPCBInputRoles(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("opamp_gain_stage")
	if !ok {
		t.Fatal("missing opamp gain stage definition")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"input_coupling": "ac",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 20, OriginYMM: 10})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realization issues = %#v", realized.Issues)
	}
	for _, role := range []string{"input_coupling", "bias_top", "bias_bottom"} {
		if realized.RoleRefs[role] == "" {
			t.Fatalf("role refs = %#v, missing %s", realized.RoleRefs, role)
		}
	}
	if !realizedRouteExists(realized, "gain_bias") {
		t.Fatalf("routes = %#v, missing AC gain_bias reference", realized.LocalRoutes)
	}
	if realizedRouteExists(realized, "gain_ground") {
		t.Fatalf("routes = %#v, AC coupling must not ground the gain reference", realized.LocalRoutes)
	}
	if !slices.Contains(realized.Validation.RequiredRoutes, "gain_bias") || slices.Contains(realized.Validation.RequiredRoutes, "gain_ground") {
		t.Fatalf("required routes = %#v, want only active AC gain reference", realized.Validation.RequiredRoutes)
	}
}

func TestOpAmpGainStageDCCouplingRealizesPCBInputRoute(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("opamp_gain_stage")
	if !ok {
		t.Fatal("missing opamp gain stage definition")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 20, OriginYMM: 10})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realization issues = %#v", realized.Issues)
	}
	if !realizedRouteExists(realized, "dc_input") {
		t.Fatalf("routes = %#v, missing dc_input", realized.LocalRoutes)
	}
	if !realizedRouteExists(realized, "gain_ground") {
		t.Fatalf("routes = %#v, missing DC gain_ground reference", realized.LocalRoutes)
	}
	if realizedRouteExists(realized, "gain_bias") {
		t.Fatalf("routes = %#v, DC coupling must not emit gain_bias", realized.LocalRoutes)
	}
	if !slices.Contains(realized.Validation.RequiredRoutes, "gain_ground") || slices.Contains(realized.Validation.RequiredRoutes, "gain_bias") {
		t.Fatalf("required routes = %#v, want only active DC gain reference", realized.Validation.RequiredRoutes)
	}
}

func TestOpAmpGainStageSchematicLayoutIsReadable(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"input_coupling":          "ac",
			"include_output_resistor": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	positions := addSymbolPositionsByRole(t, output.Operations)
	wantRoles := []string{"input_coupling", "bias_top", "bias_bottom", "feedback", "opamp", "gain_to_ground", "decoupling_capacitor", "output_resistor"}
	for _, role := range wantRoles {
		if _, ok := positions[role]; !ok {
			t.Fatalf("missing role %s in positions %#v", role, positions)
		}
	}
	if !(positions["input_coupling"].XMM < positions["opamp"].XMM && positions["opamp"].XMM < positions["output_resistor"].XMM) {
		t.Fatalf("expected left-to-right signal flow, positions=%#v", positions)
	}
	if !(positions["feedback"].YMM < positions["opamp"].YMM && positions["decoupling_capacitor"].YMM < positions["opamp"].YMM) {
		t.Fatalf("expected feedback and decoupling above op-amp, positions=%#v", positions)
	}
	if !(positions["gain_to_ground"].YMM > positions["opamp"].YMM && positions["bias_bottom"].YMM > positions["bias_top"].YMM) {
		t.Fatalf("expected ground/reference elements lower on page, positions=%#v", positions)
	}
	if spread := positions["output_resistor"].XMM - positions["input_coupling"].XMM; spread < 60 {
		t.Fatalf("schematic spread = %g mm, want at least 60 mm", spread)
	}
}

func TestOpAmpGainStageDualSupplyWarnsAndBlocksACBias(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"single_supply": false,
		},
	})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("issues = %#v", issues)
	}
	_, issues = registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"single_supply":  false,
			"input_coupling": "ac",
		},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("expected blocking issue, got %#v", issues)
	}
}

func TestOpAmpGainStageOptionalOutputResistor(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"include_output_resistor": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Nets; len(got) != 6 || got[5] != "amp_out_drive" {
		t.Fatalf("nets = %#v", got)
	}
	outputResistorRef := output.Instance.Refs[len(output.Instance.Refs)-1]
	pinNets := map[string]string{}
	for _, operation := range output.Operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var connect transactions.ConnectOperation
		if err := decodeBlockOperation(operation, &connect); err != nil {
			t.Fatalf("decode connect: %v", err)
		}
		for _, endpoint := range []transactions.Endpoint{connect.From, connect.To} {
			if endpoint.Ref == outputResistorRef {
				pinNets[endpoint.Pin] = connect.NetName
			}
		}
	}
	if pinNets["1"] == "" || pinNets["2"] == "" || pinNets["1"] == pinNets["2"] {
		t.Fatalf("output resistor pin nets = %#v, want distinct drive and output nets", pinNets)
	}
	definition, ok := registry.GetBlock("opamp_gain_stage")
	if !ok {
		t.Fatal("missing opamp gain stage definition")
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 20, OriginYMM: 10})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realization issues = %#v", realized.Issues)
	}
	for _, routeID := range []string{"output_resistor_input", "output_resistor_output"} {
		if !realizedRouteExists(realized, routeID) {
			t.Fatalf("routes = %#v, missing %s", realized.LocalRoutes, routeID)
		}
	}
}

func feedbackRatioClose(gain float64, rfOhms float64, rgOhms float64) bool {
	return math.Abs(gain-(1+rfOhms/rgOhms)) < 1e-9
}

func addSymbolPositionsByRole(t *testing.T, operations []transactions.Operation) map[string]transactions.Point {
	t.Helper()
	positions := map[string]transactions.Point{}
	for index, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			t.Fatalf("decode add_symbol operation %d: %v", index, err)
		}
		positions[payload.Role] = payload.At
	}
	return positions
}

func TestOpAmpGainStageProjectTransactionApplies(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("amp", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "amp")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues = %#v", result.Issues)
	}
	for _, name := range []string{"amp.kicad_pro", "amp.kicad_sch", "amp.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}
