package architecturesearch

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestStandaloneClockGenerationSelectsDistinctCatalogBackedArchitectures(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	registry, issues := NewCatalogRegistry(catalog)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	wantArchitectures := map[string]string{
		"precision_logic_clock": "buffered_fixed_packaged_oscillator",
		"relaxed_logic_clock":   "buffered_resistor_programmed_relaxation_oscillator",
	}
	for id, wantArchitecture := range wantArchitectures {
		t.Run(id, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join("testdata", "standalone_clock_generation_corpus", id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
			if len(decodeIssues) != 0 {
				t.Fatal(decodeIssues)
			}
			result := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
			if result.Status != SearchSelected || result.Selected == nil || len(result.Selected.Selections) != 1 {
				t.Fatalf("search status = %s, issues = %#v, rejections = %#v", result.Status, result.Issues, result.Rejections)
			}
			selection := result.Selected.Selections[0]
			if selection.ExpansionID != wantArchitecture {
				t.Fatalf("selected architecture = %s, want %s", selection.ExpansionID, wantArchitecture)
			}
			if len(result.Alternatives) == 0 {
				t.Fatal("clock search did not retain a materially distinct complete alternative")
			}
			if len(selection.Calculations) != 1 || selection.Calculations[0].CornerEvaluations != 8 || len(selection.Calculations[0].Corners) != 8 {
				t.Fatalf("clock corner proof = %#v, want eight supply/temperature/tolerance corners", selection.Calculations)
			}
			requiredBounds := map[string]bool{
				"frequency_accuracy": false, "rms_jitter": false, "output_current": false,
				"capacitive_load": false, "fanout": false, "supply_current": false,
				"local_bypass_capacitance": false, "output_high_voltage": false,
				"rise_time": false, "fall_time": false, "maximum_frequency": false,
				"duty_cycle_minimum": false, "duty_cycle_maximum": false, "startup_time": false,
			}
			for _, bound := range selection.Calculations[0].Bounds {
				if _, expected := requiredBounds[bound.Name]; expected {
					requiredBounds[bound.Name] = true
				}
			}
			for name, found := range requiredBounds {
				if !found {
					t.Fatalf("clock calculation lacks %s bound: %#v", name, selection.Calculations[0].Bounds)
				}
			}
			realization, err := DecodeFragmentRealization(selection.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if len(realization.Instances) < 4 || len(realization.Connections) < 3 {
				t.Fatalf("incomplete clock realization = %#v", realization)
			}
			instances := make(map[string]RealizationInstance, len(realization.Instances))
			for _, instance := range realization.Instances {
				instances[instance.ID] = instance
				if instance.Near != "" {
					if _, ok := instances[instance.Near]; !ok {
						// Realizations are canonically sorted, so the target may
						// appear later; validate all references below.
						continue
					}
				}
			}
			for _, instance := range realization.Instances {
				if instance.Near != "" {
					if _, ok := instances[instance.Near]; !ok || instance.MaxDistanceMM <= 0 {
						t.Fatalf("invalid clock proximity contract: %#v", instance)
					}
				}
			}
			for _, id := range []string{"clock_buffer", "clock_buffer_bypass", "clock_source_bypass"} {
				if instance := instances[id]; instance.Near == "" || instance.MaxDistanceMM <= 0 {
					t.Fatalf("clock instance %s lacks a bounded proximity contract: %#v", id, instance)
				}
			}
			if id == "relaxed_logic_clock" {
				foundTimingResistor := false
				selectedTimingResistance := 0.0
				timingCatalogID := ""
				for _, instance := range realization.Instances {
					if instance.Usage == "timing_resistor" {
						foundTimingResistor = true
						timingCatalogID = instance.CatalogID
						if instance.Near != "clock_source" || instance.MaxDistanceMM > 2 {
							t.Fatalf("timing resistor lacks source-proximity contract: %#v", instance)
						}
						var ok bool
						selectedTimingResistance, ok = components.ParseEngineeringValue(instance.Value)
						if !ok {
							t.Fatalf("timing resistor has an invalid selected value: %#v", instance)
						}
					}
				}
				if !foundTimingResistor {
					t.Fatalf("relaxed architecture lacks a calculated timing resistor: %#v", realization.Instances)
				}
				reportedTolerance, toleranceReported := namedQuantityValue(selection.Calculations[0].Inputs, "timing_resistance_tolerance")
				reportedTempco, tempcoReported := namedQuantityValue(selection.Calculations[0].Inputs, "timing_resistance_tempco")
				var timingRecord components.ComponentRecord
				for _, record := range catalog.Records {
					if record.ID == timingCatalogID {
						timingRecord = record
					}
				}
				catalogTolerance, catalogToleranceOK := catalogToleranceMaximum(timingRecord, "resistance", "%")
				catalogTempco, catalogTempcoOK := recordValueMaximum(timingRecord, "temperature_coefficient", "ppm/C")
				if !toleranceReported || !tempcoReported || !catalogToleranceOK || !catalogTempcoOK ||
					reportedTolerance != catalogTolerance || reportedTempco != catalogTempco {
					t.Fatalf("timing-resistor error evidence tolerance=%g tempco=%g does not match catalog record %#v",
						reportedTolerance, reportedTempco, timingRecord)
				}
				calculatedTimingResistance := 0.0
				for _, output := range selection.Calculations[0].NominalOutputs {
					if output.Name == "timing_resistance" {
						calculatedTimingResistance = output.Value
					}
				}
				if calculatedTimingResistance <= 0 || selectedTimingResistance != calculatedTimingResistance {
					t.Fatalf("selected timing resistance %g does not equal the electrically calculated value %g", selectedTimingResistance, calculatedTimingResistance)
				}
				for _, corner := range selection.Calculations[0].Corners {
					foundTimingCorner := false
					for _, output := range corner.Outputs {
						foundTimingCorner = foundTimingCorner || output.Name == "timing_resistance"
					}
					if !foundTimingCorner {
						t.Fatalf("relaxation corner %s lacks timing-resistance tolerance evidence: %#v", corner.ID, corner.Outputs)
					}
				}
			}
		})
	}
}

func TestClockCascadedAlternativeAccountsForBothBuffers(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("testdata", "standalone_clock_generation_corpus", "relaxed_logic_clock.json"))
	if err != nil {
		t.Fatal(err)
	}
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
	if len(decodeIssues) != 0 {
		t.Fatal(decodeIssues)
	}
	obligations, obligationIssues := initialSearchObligations(requirement, EvidenceRuleInferred)
	if len(obligationIssues) != 0 {
		t.Fatal(obligationIssues)
	}
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	var clockObligation searchObligation
	for _, obligation := range obligations {
		if obligation.Capability == "clock_generation" {
			clockObligation = obligation
		}
	}
	expansions, err := provider.Expand(context.Background(), providerRequestFor(clockObligation, requirement.Requirements.Constraints))
	if err != nil {
		t.Fatal(err)
	}
	baseSupplyCurrent := 0.0
	cascadedSupplyCurrent := 0.0
	cascadedBufferStages := 0.0
	for _, expansion := range expansions {
		if len(expansion.Calculations) != 1 {
			t.Fatalf("clock expansion %s calculations = %#v", expansion.ID, expansion.Calculations)
		}
		supplyCurrent, supplyOK := calculationOutput(expansion.Calculations[0], "supply_current")
		if !supplyOK {
			t.Fatalf("clock expansion %s lacks supply-current evidence", expansion.ID)
		}
		if strings.HasSuffix(expansion.ID, "_cascaded_endpoint") {
			cascadedSupplyCurrent = supplyCurrent
			cascadedBufferStages, _ = namedQuantityValue(expansion.Calculations[0].Inputs, "endpoint_buffer_stages")
		} else {
			baseSupplyCurrent = supplyCurrent
		}
	}
	if cascadedBufferStages != 2 || cascadedSupplyCurrent <= baseSupplyCurrent {
		t.Fatalf("cascaded clock evidence stages=%g supply=%g, want two stages and more than base %g",
			cascadedBufferStages, cascadedSupplyCurrent, baseSupplyCurrent)
	}
}

func namedQuantityValue(values []NamedQuantity, name string) (float64, bool) {
	for _, value := range values {
		if value.Name == name {
			return value.Value, true
		}
	}
	return 0, false
}

func TestClockOutputCurrentRequirementAggregatesAllBoundOutputs(t *testing.T) {
	first, second, unrelated := 0.003, 0.005, 0.1
	ports := []RoleContract{
		{Role: "output", Contract: PortContract{RequiredCurrentCapacityA: &first}},
		{Role: "reference", Contract: PortContract{RequiredCurrentCapacityA: &unrelated}},
		{Role: "output", Contract: PortContract{RequiredCurrentCapacityA: &second}},
	}
	if got, want := totalRequiredRoleCurrentA(ports, "output"), first+second; got != want {
		t.Fatalf("aggregated output current = %g, want %g", got, want)
	}
}

func TestStandaloneClockGenerationRejectsInvalidProgrammedDivider(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	found := false
	for recordIndex := range catalog.Records {
		record := &catalog.Records[recordIndex]
		if record.ID != "clock_source.analog_devices.ltc6906is6" {
			continue
		}
		for modelIndex := range record.SimulationModels {
			for parameterIndex := range record.SimulationModels[modelIndex].Parameters {
				parameter := &record.SimulationModels[modelIndex].Parameters[parameterIndex]
				if parameter.Name == "divider_ratio" {
					parameter.Value = 0
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("test catalog lacks the programmed-clock divider")
	}
	components.RebuildCatalogIndexes(catalog)
	registry, registryIssues := NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatal(registryIssues)
	}
	contents, err := os.ReadFile(filepath.Join("testdata", "standalone_clock_generation_corpus", "relaxed_logic_clock.json"))
	if err != nil {
		t.Fatal(err)
	}
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
	if len(decodeIssues) != 0 {
		t.Fatal(decodeIssues)
	}
	result := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
	if result.Status == SearchSelected {
		t.Fatalf("invalid programmed-clock divider selected an architecture: %#v", result.Selected)
	}
}

func TestStandaloneClockGenerationFailsClosedByRequirementClass(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("testdata", "standalone_clock_generation_corpus", "precision_logic_clock.json"))
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(registryIssues) != 0 {
		t.Fatal(registryIssues)
	}

	tests := []struct {
		name   string
		code   string
		mutate func(*Requirement)
	}{
		{name: "accuracy", code: string(CodeClockAccuracyUnsupported), mutate: func(requirement *Requirement) {
			replaceClockConstraint(requirement, targetConstraint("output_frequency", 8_000_000, "Hz", 0.000001))
		}},
		{name: "startup", code: string(CodeClockStartupUnsupported), mutate: func(requirement *Requirement) {
			replaceClockConstraint(requirement, constraintNumber("maximum_startup_time", "maximum", 1e-6, "s", 0))
		}},
		{name: "loading", code: string(CodeClockLoadingUnsupported), mutate: func(requirement *Requirement) {
			setClockOperatingBounds(requirement, "load_capacitance", 5e-12, 100e-12)
		}},
		{name: "fanout", code: string(CodeClockFanoutUnsupported), mutate: func(requirement *Requirement) {
			replaceClockConstraint(requirement, constraintNumber("clock_fanout", "minimum", 5, "", 0))
		}},
		{name: "jitter", code: string(CodeClockJitterUnsupported), mutate: func(requirement *Requirement) {
			replaceClockConstraint(requirement, constraintNumber("maximum_rms_jitter", "maximum", 1e-13, "s", 0))
		}},
		{name: "edge", code: string(CodeClockEdgeUnsupported), mutate: func(requirement *Requirement) {
			for index := range requirement.Requirements.BehavioralRequirements {
				behavior := &requirement.Requirements.BehavioralRequirements[index]
				if behavior.Metric == "rise_time" || behavior.Metric == "fall_time" {
					value := 1e-10
					behavior.Max = &value
				}
			}
		}},
		{name: "duty", code: string(CodeClockDutyUnsupported), mutate: func(requirement *Requirement) {
			replaceClockConstraint(requirement, targetConstraint("duty_cycle", 50, "%", 1))
		}},
		{name: "supply", code: string(CodeClockSupplyUnsupported), mutate: func(requirement *Requirement) {
			setClockOperatingBounds(requirement, "supply_voltage", 6, 7)
			for index := range requirement.Requirements.Domains {
				if requirement.Requirements.Domains[index].Kind == "supply" {
					minimum, maximum := 6.0, 7.0
					requirement.Requirements.Domains[index].MinVoltageV = &minimum
					requirement.Requirements.Domains[index].NominalVoltageV = 6.5
					requirement.Requirements.Domains[index].MaxVoltageV = &maximum
				}
			}
		}},
		{name: "interface_supply_evidence", code: string(CodeClockSupplyUnsupported), mutate: func(requirement *Requirement) {
			setClockOperatingBounds(requirement, "supply_voltage", 1.65, 1.95)
			for index := range requirement.Requirements.Domains {
				if requirement.Requirements.Domains[index].Kind == "supply" {
					minimum, maximum := 1.65, 1.95
					requirement.Requirements.Domains[index].MinVoltageV = &minimum
					requirement.Requirements.Domains[index].NominalVoltageV = 1.8
					requirement.Requirements.Domains[index].MaxVoltageV = &maximum
				}
			}
		}},
		{name: "temperature", code: string(CodeClockTemperatureUnsupported), mutate: func(requirement *Requirement) {
			setClockOperatingBounds(requirement, "ambient_temperature", -60, 150)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
			if len(decodeIssues) != 0 {
				t.Fatal(decodeIssues)
			}
			test.mutate(&requirement)
			first := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
			second := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
			if first.Status == SearchSelected || !rejectionSummaryContains(first.Rejections, reports.Code(test.code)) {
				t.Fatalf("unsupported %s accepted or misclassified: status=%s rejections=%#v", test.name, first.Status, first.Rejections)
			}
			if !reflect.DeepEqual(first.Rejections, second.Rejections) {
				t.Fatalf("unsupported %s diagnostics are not deterministic:\nfirst=%#v\nsecond=%#v", test.name, first.Rejections, second.Rejections)
			}
		})
	}
}

func replaceClockConstraint(requirement *Requirement, replacement Constraint) {
	for index := range requirement.Requirements.Objectives[0].Constraints {
		if requirement.Requirements.Objectives[0].Constraints[index].Name == replacement.Name {
			requirement.Requirements.Objectives[0].Constraints[index] = replacement
			return
		}
	}
}

func setClockOperatingBounds(requirement *Requirement, axis string, minimum, maximum float64) {
	for caseIndex := range requirement.Requirements.OperatingCases {
		for conditionIndex := range requirement.Requirements.OperatingCases[caseIndex].Conditions {
			condition := &requirement.Requirements.OperatingCases[caseIndex].Conditions[conditionIndex]
			if condition.Axis == axis {
				condition.Min = &minimum
				condition.Max = &maximum
			}
		}
	}
}
