package architecturesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCatalogProviderSynthesizesCalculatedMCUPowerIntegrity(t *testing.T) {
	tests := []struct {
		name        string
		constraints []Constraint
		wantMCU     string
		wantLocal   int
		wantBulk    int
	}{
		{
			name: "wireless_single_domain",
			constraints: []Constraint{
				constraintStringArray("required_capabilities", "all_of", []string{"wifi", "bluetooth"}),
			},
			wantMCU: "mcu.espressif.esp32_wroom_32e", wantLocal: 1, wantBulk: 1,
		},
		{
			name: "swd_single_domain",
			constraints: []Constraint{
				constraintString("programming_kind", "equal", "swd"),
			},
			wantMCU: "mcu.st.stm32g031k8t6.lqfp32", wantLocal: 1, wantBulk: 1,
		},
		{
			name: "isp_multi_domain",
			constraints: []Constraint{
				constraintString("programming_kind", "equal", "spi_isp"),
			},
			wantMCU: "mcu.microchip.atmega328p_a.tqfp32", wantLocal: 2, wantBulk: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
			if err != nil {
				t.Fatal(err)
			}
			request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
			request.Constraints = append(request.Constraints, test.constraints...)
			expansions, err := provider.Expand(context.Background(), request)
			if err != nil || len(expansions) < 1 {
				t.Fatalf("power-integrity expansion = %#v, %v", expansions, err)
			}
			if len(expansions[0].Components) == 0 || expansions[0].Components[0].CatalogID != test.wantMCU {
				t.Fatalf("selected MCU = %#v, want %s", expansions[0].Components, test.wantMCU)
			}
			realization, err := DecodeFragmentRealization(expansions[0].Payload)
			if err != nil {
				t.Fatal(err)
			}
			local, bulk := 0, 0
			for _, instance := range realization.Instances {
				switch instance.Usage {
				case "decoupling_capacitor":
					if strings.HasPrefix(instance.ID, "mcu_power_local_") {
						local++
						assertCalculatedMCUPowerInstance(t, instance)
					} else if strings.HasPrefix(instance.ID, "mcu_power_bulk_") {
						bulk++
						assertCalculatedMCUPowerInstance(t, instance)
					}
				}
			}
			if local != test.wantLocal || bulk != test.wantBulk {
				t.Fatalf("calculated local/bulk instances = %d/%d, want %d/%d: %#v", local, bulk, test.wantLocal, test.wantBulk, realization.Instances)
			}
			calculations := 0
			for _, calculation := range expansions[0].Calculations {
				if strings.HasPrefix(calculation.ID, "mcu_power_") {
					calculations++
					if !calculation.Pass || calculation.Hash == "" || len(calculation.Bounds) < 7 {
						t.Fatalf("incomplete power-integrity calculation: %#v", calculation)
					}
				}
			}
			if calculations != local+bulk {
				t.Fatalf("power-integrity calculations = %d, want %d", calculations, local+bulk)
			}
		})
	}
}

func TestCatalogProviderPowerIntegrityFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CatalogProvider)
		add    []Constraint
		code   string
	}{
		{
			name: "missing_transient_evidence",
			mutate: func(provider *CatalogProvider) {
				for index := range provider.catalog.Records {
					record := &provider.catalog.Records[index]
					if record.ID == "mcu.st.stm32g031k8t6.lqfp32" {
						record.MCU.PowerIntegrity[0].TransientCurrentStep = nil
					}
				}
			},
			code: string(CodeMCUPowerIntegrityEvidence),
		},
		{
			name: "missing_capacitor_esr",
			mutate: func(provider *CatalogProvider) {
				records := provider.familyRecords["capacitor"]
				for index := range records {
					if records[index].Capacitor != nil {
						records[index].Capacitor.ESR = nil
					}
				}
				provider.familyRecords["capacitor"] = records
			},
			code: string(CodeMCUDecouplingUnavailable),
		},
		{
			name: "missing_capacitor_voltage_derating",
			mutate: func(provider *CatalogProvider) {
				records := provider.familyRecords["capacitor"]
				for index := range records {
					if records[index].Capacitor != nil {
						records[index].Capacitor.VoltageDeratingReview = ""
						records[index].Capacitor.MaximumVoltageUseRatio = nil
					}
				}
				provider.familyRecords["capacitor"] = records
			},
			code: string(CodeMCUDecouplingUnavailable),
		},
		{
			name: "missing_supply_domain_functions",
			mutate: func(provider *CatalogProvider) {
				for index := range provider.catalog.Records {
					record := &provider.catalog.Records[index]
					if record.ID == "mcu.st.stm32g031k8t6.lqfp32" {
						record.MCU.SupplyDomains[0].PowerFunctions = nil
					}
				}
			},
			code: string(CodeMCUPowerIntegrityDomain),
		},
		{
			name: "brownout_budget_exceeded",
			add: []Constraint{
				constraintNumber("mcu_brownout_voltage", "minimum", 3.3, "V", 0),
			},
			code: string(CodeMCUPowerIntegrityBudget),
		},
		{
			name: "temperature_unavailable",
			add: []Constraint{
				constraintNumber("ambient_temperature_maximum", "maximum", 125, "degC", 0),
			},
			code: string(CodeMCUDecouplingUnavailable),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
			if err != nil {
				t.Fatal(err)
			}
			if test.mutate != nil {
				test.mutate(provider)
			}
			request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
			request.Constraints = append(request.Constraints, constraintString("programming_kind", "equal", "swd"))
			request.Constraints = append(request.Constraints, test.add...)
			_, err = provider.Expand(context.Background(), request)
			var assignmentErr *mcuAssignmentError
			if !errors.As(err, &assignmentErr) || string(assignmentErr.Code) != test.code {
				t.Fatalf("power-integrity error = %v, want %s", err, test.code)
			}
		})
	}
}

func TestCatalogProviderPowerIntegrityIsStableUnderCatalogAndConstraintReordering(t *testing.T) {
	originalProvider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	reorderedProvider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	reorderedCapacitors := reorderedProvider.familyRecords["capacitor"]
	for left, right := 0, len(reorderedCapacitors)-1; left < right; left, right = left+1, right-1 {
		reorderedCapacitors[left], reorderedCapacitors[right] = reorderedCapacitors[right], reorderedCapacitors[left]
	}
	reorderedProvider.familyRecords["capacitor"] = reorderedCapacitors
	originalRequest := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	originalRequest.Constraints = append(originalRequest.Constraints,
		constraintString("programming_kind", "equal", "swd"),
		constraintNumber("mcu_transient_current_step", "maximum", 0.015, "A", 0),
		constraintNumber("maximum_supply_noise", "maximum", 0.04, "V", 0),
		constraintNumber("ambient_temperature_maximum", "maximum", 85, "degC", 0),
	)
	reorderedRequest := originalRequest
	reorderedRequest.Constraints = append([]Constraint(nil), originalRequest.Constraints...)
	for left, right := 0, len(reorderedRequest.Constraints)-1; left < right; left, right = left+1, right-1 {
		reorderedRequest.Constraints[left], reorderedRequest.Constraints[right] = reorderedRequest.Constraints[right], reorderedRequest.Constraints[left]
	}
	first, err := originalProvider.Expand(context.Background(), originalRequest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reorderedProvider.Expand(context.Background(), reorderedRequest)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, firstErr := json.Marshal(first)
	secondJSON, secondErr := json.Marshal(second)
	if firstErr != nil || secondErr != nil || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("reordered expansion differs: first=%v second=%v equal=%t", firstErr, secondErr, bytes.Equal(firstJSON, secondJSON))
	}
}

func TestMCUPowerIntegrityIdentifiersAreUniquePerParentInstance(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		domainID string
	}{
		{name: "local", kind: "local", domainID: "digital"},
		{name: "bulk", kind: "bulk"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := mcuPowerIntegrityIdentifier(test.kind, "controller_alpha", "main_rail", test.domainID)
			second := mcuPowerIntegrityIdentifier(test.kind, "controller_beta", "main_rail", test.domainID)
			replay := mcuPowerIntegrityIdentifier(test.kind, "controller_alpha", "main_rail", test.domainID)
			if first == second {
				t.Fatalf("different parent instances produced duplicate identifier %q", first)
			}
			if first != replay {
				t.Fatalf("identifier replay = %q, want %q", replay, first)
			}
			if !strings.HasPrefix(first, "mcu_power_"+test.kind+"_") {
				t.Fatalf("identifier = %q, want stable role prefix", first)
			}
			if len(first) > 36 || len(second) > 36 {
				t.Fatalf("identifiers exceed writer-safe bound: %q %q", first, second)
			}
		})
	}
}

func TestMCUPowerAmbientRangeAcceptsSingleSidedColdAndHotBounds(t *testing.T) {
	tests := []struct {
		name       string
		constraint Constraint
		wantMin    float64
		wantMax    float64
	}{
		{
			name:       "cold maximum",
			constraint: constraintNumber("ambient_temperature_maximum", "maximum", 10, "degC", 0),
			wantMin:    10, wantMax: 10,
		},
		{
			name:       "hot minimum",
			constraint: constraintNumber("ambient_temperature_minimum", "minimum", 40, "degC", 0),
			wantMin:    40, wantMax: 40,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minimum, maximum, err := mcuPowerAmbientRange([]Constraint{test.constraint})
			if err != nil || minimum != test.wantMin || maximum != test.wantMax {
				t.Fatalf("ambient range = %.9g..%.9g, %v; want %.9g..%.9g", minimum, maximum, err, test.wantMin, test.wantMax)
			}
		})
	}
}

func TestMCUPowerConstraintAllowsZeroOnlyForNonNegativeQuantity(t *testing.T) {
	constraints := []Constraint{constraintNumber("power_source_impedance", "maximum", 0, "Ohm", 0)}
	value, present, err := mcuPowerConstraint(constraints, "power_source_impedance", "maximum", "Ohm", true)
	if err != nil || !present || value != 0 {
		t.Fatalf("ideal source impedance = %.9g present=%t err=%v, want accepted zero", value, present, err)
	}
	if _, _, err := mcuPowerConstraint(constraints, "power_source_impedance", "maximum", "Ohm", false); err == nil {
		t.Fatal("strictly positive quantity accepted zero")
	}
}

func assertCalculatedMCUPowerInstance(t *testing.T, instance RealizationInstance) {
	t.Helper()
	if instance.CatalogID == "" || instance.Value == "" || instance.Near == "" || instance.MaxDistanceMM <= 0 {
		t.Fatalf("power-integrity instance lacks identity/value/placement evidence: %#v", instance)
	}
	required := map[string]bool{
		"nominal_capacitance":           false,
		"effective_capacitance_minimum": false,
		"maximum_esr":                   false,
		"ripple_current_rating":         false,
		"voltage_derating_factor":       false,
	}
	for _, parameter := range instance.Parameters {
		if _, exists := required[parameter.Name]; exists && parameter.Value > 0 {
			required[parameter.Name] = true
		}
	}
	for name, present := range required {
		if !present {
			t.Fatalf("power-integrity instance lacks %s parameter: %#v", name, instance.Parameters)
		}
	}
}
