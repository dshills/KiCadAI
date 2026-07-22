package architecturesearch

import (
	"context"
	"errors"
	"math"
	"testing"

	"kicadai/internal/components"
)

func TestRegulatorDropoutSelectionUsesTypedEvidence(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	part, err := provider.selectRegulatorWithDropout(context.Background(), "adjustable", []components.RequiredRating{
		{Kind: "input_voltage", Value: "5", Unit: "V"}, {Kind: "output_current", Value: ".1", Unit: "A"},
	}, .35)
	if err != nil {
		t.Fatal(err)
	}
	dropout, ok := recordRegulatorDropoutV(part.record)
	if !ok || dropout > .35 {
		t.Fatalf("selected dropout = %.12g ok=%v part=%s", dropout, ok, part.record.ID)
	}

	_, err = provider.selectRegulatorWithDropout(context.Background(), "adjustable", nil, .01)
	var typed *powerSynthesisError
	if !errors.As(err, &typed) || typed.code != CodePowerDropoutMarginUnavailable {
		t.Fatalf("dropout rejection = %#v", err)
	}
}

func TestRegulatorDropoutCalculationFailsClosed(t *testing.T) {
	record := components.ComponentRecord{Regulator: &components.RegulatorEvidence{DropoutVoltage: &components.EvidenceMeasurement{Value: 300, Unit: "mV"}}}
	calculation, err := regulatorDropoutCalculation(3.7, 3.3, record)
	if err != nil || !calculation.Pass || len(ValidateCalculation(calculation)) != 0 {
		t.Fatalf("dropout calculation = %#v err=%v", calculation, err)
	}
	_, err = regulatorDropoutCalculation(3.5, 3.3, record)
	var typed *powerSynthesisError
	if !errors.As(err, &typed) || typed.code != CodePowerDropoutMarginUnavailable {
		t.Fatalf("insufficient dropout error = %#v", err)
	}
}

func TestTypedQuiescentAndEfficiencyEvidenceDrivePowerDemand(t *testing.T) {
	minimumEfficiency := .8
	record := components.ComponentRecord{Regulator: &components.RegulatorEvidence{
		QuiescentCurrent: &components.EvidenceMeasurement{Value: 100, Unit: "mA"},
		Efficiency:       &components.EvidenceRange{Minimum: &minimumEfficiency, Unit: "ratio"},
	}}
	current, ok := recordSupplyCurrentA(record)
	if !ok || current != .1 {
		t.Fatalf("typed quiescent current = %.12g ok=%v", current, ok)
	}
	inputMinimum, outputMaximum, outputCurrent := 10.0, 5.0, 1.0
	ports := []RoleContract{
		{Role: "input", Contract: PortContract{Kind: "power", Direction: "sink", Voltage: NumericRange{Minimum: &inputMinimum}}},
		{Role: "output", Contract: PortContract{Kind: "power", Direction: "source", Voltage: NumericRange{Maximum: &outputMaximum}, RequiredCurrentCapacityA: &outputCurrent}},
	}
	demand, proven := catalogConvertedPowerDemandA(ports, []catalogPart{{record: record}}, []string{"input"})
	if !proven || math.Abs(demand["input"]-.625) > 1e-12 {
		t.Fatalf("converted demand = %#v proven=%v", demand, proven)
	}
}

func TestConvertedPowerDemandFailsClosedWithoutEfficiencyEvidence(t *testing.T) {
	inputMinimum, outputMaximum, outputCurrent := 10.0, 5.0, 1.0
	ports := []RoleContract{
		{Role: "input", Contract: PortContract{Kind: "power", Direction: "sink", Voltage: NumericRange{Minimum: &inputMinimum}}},
		{Role: "output", Contract: PortContract{Kind: "power", Direction: "source", Voltage: NumericRange{Maximum: &outputMaximum}, RequiredCurrentCapacityA: &outputCurrent}},
	}
	demand, proven := catalogConvertedPowerDemandA(ports, []catalogPart{{record: components.ComponentRecord{Family: "regulator"}}}, []string{"input"})
	if proven || len(demand) != 0 {
		t.Fatalf("converted demand = %#v proven=%v, want unproven without efficiency", demand, proven)
	}
}

func TestLinearPowerDemandUsesTransferredLoadCurrentWithoutEfficiencyEvidence(t *testing.T) {
	inputMinimum, outputMaximum, outputCurrent := 10.0, 5.0, 1.0
	ports := []RoleContract{
		{Role: "input", Contract: PortContract{Kind: "power", Direction: "sink", Voltage: NumericRange{Minimum: &inputMinimum}}},
		{Role: "output", Contract: PortContract{Kind: "power", Direction: "source", Voltage: NumericRange{Maximum: &outputMaximum}, RequiredCurrentCapacityA: &outputCurrent}},
	}
	demand, proven := catalogConvertedPowerDemandA(ports, []catalogPart{{record: components.ComponentRecord{
		Family: "regulator",
		Tags:   []string{"linear_regulator"},
	}}}, []string{"input"})
	if !proven || len(demand) != 0 {
		t.Fatalf("linear converted demand = %#v proven=%v, want direct current transfer", demand, proven)
	}
}

func TestConvertedPowerDemandRejectsNonFiniteCurrent(t *testing.T) {
	minimumEfficiency := math.SmallestNonzeroFloat64
	record := components.ComponentRecord{Regulator: &components.RegulatorEvidence{
		Efficiency: &components.EvidenceRange{Minimum: &minimumEfficiency, Unit: "ratio"},
	}}
	inputMinimum, outputMaximum, outputCurrent := math.SmallestNonzeroFloat64, 1.0, 1.0
	ports := []RoleContract{
		{Role: "input", Contract: PortContract{Kind: "power", Direction: "sink", Voltage: NumericRange{Minimum: &inputMinimum}}},
		{Role: "output", Contract: PortContract{Kind: "power", Direction: "source", Voltage: NumericRange{Maximum: &outputMaximum}, RequiredCurrentCapacityA: &outputCurrent}},
	}
	demand, proven := catalogConvertedPowerDemandA(ports, []catalogPart{{record: record}}, []string{"input"})
	if proven || len(demand) != 0 {
		t.Fatalf("converted demand = %#v proven=%v, want non-finite result rejected", demand, proven)
	}
}

func TestRegulatorThermalCalculationUsesTypedEvidence(t *testing.T) {
	maximumTemperature, thermalResistance := 125.0, 50.0
	part := catalogPart{
		selected: SelectedComponent{InstanceID: "regulator"},
		evidence: ContractEvidence{Confidence: EvidenceVerified},
		record: components.ComponentRecord{
			Regulator: &components.RegulatorEvidence{QuiescentCurrent: &components.EvidenceMeasurement{Value: 1, Unit: "mA"}},
			Thermal:   &components.ThermalEvidence{MaxJunctionTemperatureC: &maximumTemperature, JunctionToAmbientCPerW: &thermalResistance},
		},
	}
	request := regulatorProviderRequest(5, 3.3, .1)
	request.Constraints = append(request.Constraints, constraintNumber("ambient_temperature", "maximum", 25, "degC", 0))
	calculation, ok := catalogRegulatorThermalCalculation(request, part)
	if !ok || !calculation.Pass || calculation.WorstMargin <= 0 {
		t.Fatalf("typed thermal calculation = %#v ok=%v", calculation, ok)
	}
}

func TestRegulatorThermalCalculationFailsClosedWithoutQuiescentCurrent(t *testing.T) {
	maximumTemperature, thermalResistance := 125.0, 50.0
	part := catalogPart{
		selected: SelectedComponent{InstanceID: "regulator"},
		evidence: ContractEvidence{Confidence: EvidenceVerified},
		record: components.ComponentRecord{Thermal: &components.ThermalEvidence{
			MaxJunctionTemperatureC: &maximumTemperature,
			JunctionToAmbientCPerW:  &thermalResistance,
		}},
	}
	request := regulatorProviderRequest(5, 3.3, .1)
	request.Constraints = append(request.Constraints, constraintNumber("ambient_temperature", "maximum", 25, "degC", 0))
	if calculation, ok := catalogRegulatorThermalCalculation(request, part); ok || calculation.Hash != "" {
		t.Fatalf("thermal calculation = %#v ok=%v, want missing quiescent-current evidence rejected", calculation, ok)
	}
}
