package architecturesearch

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestCatalogProviderExpandsPrecisionRectificationFromBehavioralEnvelope(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	expansions, err := provider.Expand(context.Background(), precisionRectifierProviderRequest(.05))
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() count=%d, err=%v", len(expansions), err)
	}
	for _, expansion := range expansions {
		realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		for _, want := range []string{
			"regulator.ti.lm7705mme.vssop8",
			"opamp.ti.opa197id.soic8",
			"diode.onsemi.1n4148w.sod_123",
			"resistor.vishay.tnpw0805.680k.1p0",
		} {
			if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
				return instance.CatalogID == want
			}) {
				t.Fatalf("realization lacks %s: %#v", want, realization.Instances)
			}
		}
		if len(realization.Instances) > 18 {
			t.Fatalf("component count = %d, want <= 18", len(realization.Instances))
		}
		dampingIndex := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == "steering_damping"
		})
		if dampingIndex < 0 {
			t.Fatalf("realization lacks steering-loop damping: %#v", realization.Instances)
		}
		wantDamping := "resistor.yageo.rc0805fr_0747rl.0805"
		if expansion.ID == "single_supply_precision_full_wave_rectifier_alt" {
			wantDamping = "resistor.vishay.crcw080547r0fkea.0805"
		}
		if realization.Instances[dampingIndex].CatalogID != wantDamping {
			t.Fatalf("%s damping = %s, want %s: %#v", expansion.ID, realization.Instances[dampingIndex].CatalogID, wantDamping, realization.Instances)
		}
		for _, wantNet := range []string{
			"rectifier_input",
			"rectifier_sum_node",
			"rectifier_output",
			"rectifier_steering_node",
			"rectifier_steering_output",
			"rectifier_negative_bias",
			"charge_pump_reserve",
		} {
			if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
				return connection.ID == wantNet
			}) {
				t.Fatalf("realization lacks semantic net %s: %#v", wantNet, realization.Connections)
			}
		}
		calculation := slices.IndexFunc(expansion.Calculations, func(calculation CalculationEvidence) bool {
			return calculation.ID == "precision_rectifier_worst_case"
		})
		if calculation < 0 || !expansion.Calculations[calculation].Pass || expansion.Calculations[calculation].Hash == "" {
			t.Fatalf("precision transfer calculation is absent or unproven: %#v", expansion.Calculations)
		}
		inputPort := slices.IndexFunc(expansion.OfferedPorts, func(port RoleContract) bool { return port.Role == "input" })
		if inputPort < 0 || expansion.OfferedPorts[inputPort].Contract.InputImpedanceMinOhm == nil ||
			*expansion.OfferedPorts[inputPort].Contract.InputImpedanceMinOhm < 100_000 {
			t.Fatalf("offered input impedance is unproven: %#v", expansion.OfferedPorts)
		}
	}
}

func TestCatalogProviderPrecisionRectifierIsCatalogOrderDeterministic(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	forward, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := NewCatalogProvider(reversedArchitectureCatalog(catalog))
	if err != nil {
		t.Fatal(err)
	}
	first, firstErr := forward.Expand(context.Background(), precisionRectifierProviderRequest(.05))
	second, secondErr := reversed.Expand(context.Background(), precisionRectifierProviderRequest(.05))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("forward err=%v reversed err=%v", firstErr, secondErr)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("catalog reorder changed precision-rectifier expansion")
	}
}

func TestPrecisionRectifierSearchPrefersCatalogPreferredDampingPart(t *testing.T) {
	file, err := os.Open("testdata/held_out_capability_expansion_corpus/analog_precision_rectifier.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	requirement, issues := DecodeStrict(file)
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	catalog := loadArchitectureCatalog(t)
	registry, registryIssues := NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	result := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "test"})
	if result.Status != SearchSelected || result.Selected == nil {
		t.Fatalf("search = %#v", result)
	}
	for _, selection := range result.Selected.Selections {
		if selection.Capability != "precision_rectification" {
			continue
		}
		damping := slices.IndexFunc(selection.Components, func(component SelectedComponent) bool {
			return component.InstanceID == "steering_damping"
		})
		if damping < 0 || selection.Components[damping].CatalogID != "resistor.yageo.rc0805fr_0747rl.0805" {
			t.Fatalf("selected damping = %#v", selection.Components)
		}
		return
	}
	t.Fatal("selected candidate lacks precision-rectification selection")
}

func TestCatalogProviderPrecisionRectifierFailsClosedOutsideProvenEnvelope(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	if expansions, expandErr := provider.Expand(context.Background(), precisionRectifierProviderRequest(.01)); expandErr == nil || len(expansions) != 0 {
		t.Fatalf("Expand() = %#v, %v; want transfer-error rejection", expansions, expandErr)
	}
	request := precisionRectifierProviderRequest(.05)
	request.Ports[0].Contract.InputImpedanceMinOhm = float64Pointer(400_000)
	if expansions, expandErr := provider.Expand(context.Background(), request); expandErr == nil || len(expansions) != 0 {
		t.Fatalf("Expand() = %#v, %v; want input-impedance rejection", expansions, expandErr)
	}
}

func precisionRectifierProviderRequest(transferError float64) ProviderRequest {
	input := providerRole("input", "analog_voltage", "sink", -1, 1)
	input.Contract.InputImpedanceMinOhm = float64Pointer(100_000)
	input.Contract.FrequencyMaxHz = float64Pointer(2_000)
	output := providerRole("output", "analog_voltage", "source", 0, 1.1)
	output.Contract.FrequencyMaxHz = float64Pointer(2_000)
	return ProviderRequest{
		Capability: "precision_rectification",
		Ports: []RoleContract{
			input,
			output,
			providerRole("power", "power", "sink", 4.75, 5.25),
			providerRole("reference", "reference", "bidirectional", 0, 0),
		},
		Constraints: []Constraint{
			constraintNumber("input_peak", "maximum", 1, "V", 0),
			constraintNumber("transfer_error", "maximum", transferError, "V", 0),
			constraintNumber("minimum_input_impedance", "minimum", 100_000, "Ohm", 0),
		},
	}
}
