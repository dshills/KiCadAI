package architecturesearch

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/components"
)

func TestCatalogProviderExpandsGenericConstantCurrentEnvelopes(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name             string
		request          ProviderRequest
		wantReference    bool
		wantControlInput bool
		wantSwitch       string
		wantRegulator    string
	}{
		{name: "sensor_precision", request: constantCurrentProviderRequest(8.1, 9.9, .001, 1, 5, .001, false), wantReference: true, wantSwitch: "mosfet.aos.aod21357.to252", wantRegulator: "current_regulator.analog_devices.lt3092mpst.sot223"},
		{name: "power_output", request: constantCurrentProviderRequest(10.8, 13.2, .02, 5, 8, .001, false), wantSwitch: "mosfet.aos.aod21357.to252", wantRegulator: "current_regulator.analog_devices.lt3080ist.sot223"},
		{name: "mcu_controlled", request: constantCurrentProviderRequest(4.75, 5.25, .1, 5, 3, .002, true), wantControlInput: true, wantSwitch: "mosfet.aos.aod21357.to252", wantRegulator: "current_regulator.analog_devices.lt3080ist.sot223"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expansions, expandErr := provider.Expand(context.Background(), test.request)
			if expandErr != nil || len(expansions) == 0 {
				t.Fatalf("Expand() count=%d, err=%v", len(expansions), expandErr)
			}
			for _, expansion := range expansions {
				realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				hasReference := slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
					return strings.HasPrefix(instance.CatalogID, "voltage_reference.")
				})
				if hasReference != test.wantReference {
					t.Fatalf("precision reference present=%t, want %t; instances=%#v", hasReference, test.wantReference, realization.Instances)
				}
				if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
					return instance.CatalogID == test.wantRegulator
				}) {
					t.Fatalf("realization lacks reviewed current regulator: %#v", realization.Instances)
				}
				if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
					return instance.CatalogID == test.wantSwitch
				}) {
					t.Fatalf("realization lacks reviewed default-off input switch: %#v", realization.Instances)
				}
				for _, instance := range realization.Instances {
					if instance.Usage != "series_current_shunt" && instance.Usage != "bias_current" && instance.Usage != "threshold_divider" {
						continue
					}
					if !catalogRecordHasPrecisionResistorEvidence(catalog, instance.CatalogID) {
						t.Fatalf("programming resistor %s (%s) lacks <=%g%% tolerance and <=%g ppm/C tempco evidence", instance.ID, instance.CatalogID, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC)
					}
				}
				controlBound := slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
					return binding.Role == "control" && binding.Instance == "enable_base" && binding.Function == "A"
				})
				if controlBound != test.wantControlInput {
					t.Fatalf("control binding present=%t, want %t; bindings=%#v", controlBound, test.wantControlInput, realization.PortBindings)
				}
				for _, calculation := range expansion.Calculations {
					if !calculation.Pass || calculation.Hash == "" {
						t.Fatalf("unproven calculation: %#v", calculation)
					}
				}
				currentCalculation := slices.IndexFunc(expansion.Calculations, func(calculation CalculationEvidence) bool {
					return calculation.ID == "constant_current_worst_case"
				})
				if currentCalculation < 0 {
					t.Fatal("expansion lacks constant-current calculation")
				}
				for _, selected := range expansion.Calculations[currentCalculation].SelectedValues {
					instanceID := strings.TrimSuffix(selected.Name, "_resistance")
					instanceIndex := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
						return instance.ID == instanceID
					})
					if instanceIndex < 0 {
						t.Fatalf("selected value %q lacks realization instance %q", selected.Name, instanceID)
					}
					emitted, emittedOK := components.ParseEngineeringValue(realization.Instances[instanceIndex].Value)
					if !emittedOK || math.Abs(emitted-selected.Selected) > math.Max(1e-12, selected.Selected*1e-12) {
						t.Fatalf("%s calculation selected %.12g Ohm but realization emitted %q", instanceID, selected.Selected, realization.Instances[instanceIndex].Value)
					}
				}
			}
		})
	}
}

func catalogRecordHasPrecisionResistorEvidence(catalog *components.Catalog, componentID string) bool {
	for _, record := range catalog.Records {
		if record.ID != componentID {
			continue
		}
		for _, tolerance := range record.Tolerances {
			if tolerance.Kind != "resistance" || tolerance.Unit != "%" {
				continue
			}
			raw := tolerance.Max
			if raw == "" {
				raw = tolerance.Typ
			}
			value, err := strconv.ParseFloat(raw, 64)
			tempco, tempcoOK := recordValueMaximum(record, "temperature_coefficient", "ppm/C")
			return err == nil && value <= currentRegulatorPassiveTolerance && tempcoOK && tempco <= currentRegulatorPassiveTempcoPPMPerC
		}
	}
	return false
}

func TestCatalogProviderConstantCurrentExpansionIsCatalogOrderDeterministic(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	forward, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := NewCatalogProvider(reversedArchitectureCatalog(catalog))
	if err != nil {
		t.Fatal(err)
	}
	request := constantCurrentProviderRequest(4.75, 5.25, .1, 5, 3, .002, true)
	first, firstErr := forward.Expand(context.Background(), request)
	second, secondErr := reversed.Expand(context.Background(), request)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("forward err=%v reversed err=%v", firstErr, secondErr)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		difference := 0
		for difference < len(firstJSON) && difference < len(secondJSON) && firstJSON[difference] == secondJSON[difference] {
			difference++
		}
		firstEnd := min(difference+160, len(firstJSON))
		secondEnd := min(difference+160, len(secondJSON))
		t.Fatalf("catalog reorder changed expansion at byte %d\nforward=%s\nreversed=%s", difference, firstJSON[difference:firstEnd], secondJSON[difference:secondEnd])
	}
}

func TestCatalogProviderModelParameterSelectionAcceptsFiniteZero(t *testing.T) {
	const (
		componentID = "current_regulator.analog_devices.lt3080ist.sot223"
		modelID     = "mna_programmable_current_source_v1"
		parameter   = "min_headroom_v"
	)
	catalog := loadArchitectureCatalog(t)
	updated := false
	for recordIndex := range catalog.Records {
		record := &catalog.Records[recordIndex]
		if record.ID != componentID {
			continue
		}
		for modelIndex := range record.SimulationModels {
			model := &record.SimulationModels[modelIndex]
			if model.ModelID != modelID {
				continue
			}
			for parameterIndex := range model.Parameters {
				if model.Parameters[parameterIndex].Name == parameter {
					model.Parameters[parameterIndex].Value = 0
					updated = true
				}
			}
		}
	}
	if !updated {
		t.Fatalf("catalog fixture lacks %s.%s", modelID, parameter)
	}
	components.RebuildCatalogIndexes(catalog)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := provider.selectComponentMinimizingModelParameterWithTemperature(
		context.Background(), "current_regulator", "", nil, true, nil, nil,
		modelID, parameter, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected.record.ID != componentID {
		t.Fatalf("zero-valued model parameter selected %s, want %s", selected.record.ID, componentID)
	}
}

func TestCatalogProviderHighSideSwitchChoosesLeastSufficientGateRatedDevice(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	ratings := []components.RequiredRating{
		{Kind: "drain_source_voltage", Value: "17.5", Unit: "V"},
		{Kind: "drain_current", Value: "3.75", Unit: "A"},
		{Kind: "gate_source_voltage", Value: "17.5", Unit: "V"},
	}
	selected, err := provider.selectComponentMinimizingRatingsWithTemperature(context.Background(), "mosfet", "p_channel", ratings, true, nil, []string{"drain_source_voltage", "drain_current"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.record.ID != "mosfet.aos.aoss21311c.sot23" {
		t.Fatalf("least sufficient gate-rated switch = %s", selected.record.ID)
	}
	reversed, err := NewCatalogProvider(reversedArchitectureCatalog(catalog))
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := reversed.selectComponentMinimizingRatingsWithTemperature(context.Background(), "mosfet", "p_channel", ratings, true, nil, []string{"drain_source_voltage", "drain_current"})
	if err != nil {
		t.Fatal(err)
	}
	if reordered.record.ID != selected.record.ID {
		t.Fatalf("catalog reorder selected %s, want %s", reordered.record.ID, selected.record.ID)
	}
}

func TestCatalogProviderConstantCurrentRejectsInsufficientComplianceHeadroom(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := constantCurrentProviderRequest(4.75, 5.25, .1, 5, 3.5, .002, true)
	if expansions, expandErr := provider.Expand(context.Background(), request); expandErr == nil || len(expansions) != 0 {
		t.Fatalf("Expand() = %#v, %v; want headroom rejection", expansions, expandErr)
	}
}

func TestCatalogProviderConstantCurrentRejectsBelowRegulatorMinimumCurrent(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := constantCurrentProviderRequest(8.1, 9.9, .0001, 1, 5, .001, false)
	if expansions, expandErr := provider.Expand(context.Background(), request); expandErr == nil || len(expansions) != 0 {
		t.Fatalf("Expand() = %#v, %v; want minimum-output-current rejection", expansions, expandErr)
	}
}

func TestCurrentRegulatorPassiveToleranceIncludesTemperatureCoefficient(t *testing.T) {
	minimum, maximum := -20.0, 70.0
	got := currentRegulatorEffectivePassiveTolerance(&components.TemperatureRequirement{MinimumC: &minimum, MaximumC: &maximum})
	if math.Abs(got-.2125) > 1e-12 {
		t.Fatalf("effective passive tolerance = %g%%, want 0.2125%%", got)
	}
	if conservative := currentRegulatorEffectivePassiveTolerance(nil); math.Abs(conservative-.475) > 1e-12 {
		t.Fatalf("unbounded-temperature passive tolerance = %g%%, want 0.475%%", conservative)
	}
}

func constantCurrentProviderRequest(inputMinimum, inputMaximum, current, tolerance, compliance, response float64, controlled bool) ProviderRequest {
	ports := []RoleContract{
		providerRole("input", "power", "sink", inputMinimum, inputMaximum),
		providerRole("output", "protected_output", "source", 0, compliance),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}
	if controlled {
		ports = append(ports, providerRole("control", "digital_logic", "source", 0, inputMaximum))
	}
	return ProviderRequest{
		Capability: "constant_current_regulation",
		Ports:      ports,
		Constraints: []Constraint{
			constraintNumber("output_current", "target", current, "A", tolerance),
			constraintNumber("minimum_compliance_voltage", "minimum", compliance, "V", 0),
			constraintBool("startup_isolation", "required", true),
			constraintNumber("startup_output_voltage", "maximum", .5, "V", 0),
			constraintNumber("response_time", "maximum", response, "s", 0),
			constraintNumber("junction_temperature", "maximum", 125, "degC", 0),
			constraintNumber("ambient_temperature", "minimum", -20, "degC", 0),
			constraintNumber("ambient_temperature", "maximum", 70, "degC", 0),
		},
	}
}
