package architecturesearch

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	CodeClockFrequencyUnsupported   reports.Code = "CLOCK_FREQUENCY_UNSUPPORTED"
	CodeClockAccuracyUnsupported    reports.Code = "CLOCK_ACCURACY_UNSUPPORTED"
	CodeClockDutyUnsupported        reports.Code = "CLOCK_DUTY_CYCLE_UNSUPPORTED"
	CodeClockStartupUnsupported     reports.Code = "CLOCK_STARTUP_UNSUPPORTED"
	CodeClockJitterUnsupported      reports.Code = "CLOCK_JITTER_UNSUPPORTED"
	CodeClockSupplyUnsupported      reports.Code = "CLOCK_SUPPLY_UNSUPPORTED"
	CodeClockTemperatureUnsupported reports.Code = "CLOCK_TEMPERATURE_UNSUPPORTED"
	CodeClockLoadingUnsupported     reports.Code = "CLOCK_LOADING_UNSUPPORTED"
	CodeClockFanoutUnsupported      reports.Code = "CLOCK_FANOUT_UNSUPPORTED"
	CodeClockEdgeUnsupported        reports.Code = "CLOCK_EDGE_UNSUPPORTED"
	clockBypassCapacitanceF                      = 100e-9
	clockBypassAllowableDroopV                   = 0.1
)

type clockGenerationError struct {
	code    reports.Code
	message string
}

func (err *clockGenerationError) Error() string { return err.message }

func (err *clockGenerationError) ArchitectureRejectionCode() reports.Code { return err.code }

func clockUnsupported(code reports.Code, message string) error {
	return &clockGenerationError{code: code, message: message}
}

type clockGenerationRequirements struct {
	frequencyHz            float64
	frequencyTolerancePct  float64
	dutyCyclePct           float64
	dutyTolerancePct       float64
	dutyConstrained        bool
	maximumStartupS        float64
	startupConstrained     bool
	minimumFanout          float64
	maximumRMSJitterS      float64
	minimumOutputHighV     float64
	maximumRiseTimeS       float64
	maximumFallTimeS       float64
	supplyMinimumV         float64
	supplyMaximumV         float64
	maximumLoadF           float64
	requiredOutputCurrentA float64
	maximumSupplyCurrentA  float64
	temperature            *components.TemperatureRequirement
	temperatureSpanFrom25C float64
}

type clockArchitectureChoice struct {
	source             catalogPart
	modelID            string
	architectureClass  string
	frequencyHz        float64
	accuracyPercent    float64
	dutyMinimumPercent float64
	dutyMaximumPercent float64
	startupS           float64
	jitterS            float64
	timingResistance   float64
	timingResistor     catalogPart
	timingTolerancePct float64
	timingTempcoPPM    float64
}

func (provider *CatalogProvider) expandClockGeneration(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	requirements, err := clockRequirements(request)
	if err != nil {
		return nil, err
	}
	choice, err := provider.selectClockArchitecture(ctx, requirements)
	if err != nil {
		return nil, err
	}

	buffer, err := provider.selectComponentWithTemperature(ctx, "logic_buffer", "", []components.RequiredRating{
		{Kind: "supply_voltage", Value: numericString(requirements.supplyMinimumV), Unit: "V"},
		{Kind: "supply_voltage", Value: numericString(requirements.supplyMaximumV), Unit: "V"},
		{Kind: "output_current", Value: numericString(requirements.requiredOutputCurrentA), Unit: "A"},
		{Kind: "capacitive_load", Value: numericString(requirements.maximumLoadF), Unit: "F"},
		{Kind: "fanout", Value: numericString(requirements.minimumFanout), Unit: "count"},
	}, true, requirements.temperature)
	if err != nil {
		withoutFanout, fanoutErr := provider.selectComponentWithTemperature(ctx, "logic_buffer", "", []components.RequiredRating{
			{Kind: "supply_voltage", Value: numericString(requirements.supplyMinimumV), Unit: "V"},
			{Kind: "supply_voltage", Value: numericString(requirements.supplyMaximumV), Unit: "V"},
			{Kind: "output_current", Value: numericString(requirements.requiredOutputCurrentA), Unit: "A"},
			{Kind: "capacitive_load", Value: numericString(requirements.maximumLoadF), Unit: "F"},
		}, true, requirements.temperature)
		if fanoutErr == nil && withoutFanout.record.ID != "" {
			return nil, clockUnsupported(CodeClockFanoutUnsupported, "no qualified clock endpoint buffer satisfies the requested fanout")
		}
		withoutLoad, loadErr := provider.selectComponentWithTemperature(ctx, "logic_buffer", "", []components.RequiredRating{
			{Kind: "supply_voltage", Value: numericString(requirements.supplyMinimumV), Unit: "V"},
			{Kind: "supply_voltage", Value: numericString(requirements.supplyMaximumV), Unit: "V"},
		}, true, requirements.temperature)
		if loadErr == nil && withoutLoad.record.ID != "" {
			return nil, clockUnsupported(CodeClockLoadingUnsupported, "no qualified clock endpoint buffer satisfies the requested current and capacitive loading")
		}
		return nil, fmt.Errorf("clock endpoint-buffer selection failed: %w", err)
	}
	if buffer.record.Interface == nil || !buffer.record.Interface.FabricationProof {
		return nil, fmt.Errorf("clock endpoint buffer lacks fabrication-grade interface evidence")
	}
	interfaceSupplyMinimum, interfaceSupplyMaximum, interfaceSupplyOK := clockEvidenceRange(buffer.record.Interface.Voltage, "V")
	if !interfaceSupplyOK ||
		requirements.supplyMinimumV < interfaceSupplyMinimum ||
		requirements.supplyMaximumV > interfaceSupplyMaximum {
		return nil, clockUnsupported(
			CodeClockSupplyUnsupported,
			"no qualified clock endpoint buffer has condition-specific evidence for the requested supply range",
		)
	}
	_, endpointEdgeMaximum, endpointEdgeOK := clockEvidenceRange(buffer.record.Interface.EdgeTime, "s")
	if !endpointEdgeOK || endpointEdgeMaximum > requirements.maximumRiseTimeS || endpointEdgeMaximum > requirements.maximumFallTimeS {
		return nil, clockUnsupported(CodeClockEdgeUnsupported, "no qualified clock endpoint buffer satisfies the requested rise and fall times")
	}
	buffer.selected.InstanceID, buffer.usage = "clock_buffer", "clock_buffer"

	bypass, err := provider.selectPassiveComponentWithRatings(ctx, "capacitor", "capacitance", "100n", []components.RequiredRating{
		{Kind: "voltage", Value: numericString(requirements.supplyMaximumV), Unit: "V"},
	})
	if err != nil {
		return nil, fmt.Errorf("clock bypass selection failed: %w", err)
	}
	bypass.value = engineeringValue(clockBypassCapacitanceF, "F")
	sourceBypass := bypass
	sourceBypass.selected.InstanceID, sourceBypass.usage = "clock_source_bypass", "decoupling_capacitor"
	sourceBypass.near, sourceBypass.maxDistanceMM = "clock_source", 2
	bufferBypass := bypass
	bufferBypass.selected.InstanceID, bufferBypass.usage = "clock_buffer_bypass", "decoupling_capacitor"
	bufferBypass.near, bufferBypass.maxDistanceMM = "clock_buffer", 2

	choice.source.selected.InstanceID, choice.source.usage = "clock_source", "standalone_clock_source"
	buffer.near, buffer.maxDistanceMM = "clock_source", 5
	parts := []catalogPart{choice.source, buffer, sourceBypass, bufferBypass}
	connections := []RealizationConnection{
		semanticNet("clock_source_output", "clock", endpoint(choice.source, "OUT"), endpoint(buffer, "IN")),
		semanticNet("clock_power", "power",
			endpoint(choice.source, "VDD"), endpoint(buffer, "VCC"),
			endpoint(sourceBypass, "A"), endpoint(bufferBypass, "A")),
		semanticNet("clock_reference", "reference",
			endpoint(choice.source, "GND"), endpoint(buffer, "GND"),
			endpoint(sourceBypass, "B"), endpoint(bufferBypass, "B")),
	}

	if choice.architectureClass == "fixed_packaged_oscillator" {
		connections[1].Endpoints = append(connections[1].Endpoints, endpoint(choice.source, "ENABLE"))
	} else {
		timing := choice.timingResistor
		if timing.record.ID == "" {
			return nil, fmt.Errorf("clock timing-resistor selection lacks catalog evidence")
		}
		timing.selected.InstanceID, timing.usage = "clock_timing_resistor", "timing_resistor"
		timing.value = engineeringValue(choice.timingResistance, "Ohm")
		timing.near, timing.maxDistanceMM = "clock_source", 2
		parts = append(parts, timing)
		connections = append(connections,
			semanticNet("clock_timing", "timing", endpoint(choice.source, "SET"), endpoint(timing, "A")),
		)
		connections[2].Endpoints = append(connections[2].Endpoints, endpoint(choice.source, "DIV"), endpoint(timing, "B"))
	}

	bindings := bindRoles(request.Ports, choice.source.selected.InstanceID, map[string]string{
		"output": "OUT", "power": "VDD", "reference": "GND",
	})
	for index := range bindings {
		switch bindings[index].Role {
		case "output":
			bindings[index].Instance, bindings[index].Function = buffer.selected.InstanceID, "OUT"
			bindings[index].NetRole = "clock"
		case "power":
			bindings[index].Instance, bindings[index].Function = choice.source.selected.InstanceID, "VDD"
		case "reference":
			bindings[index].Instance, bindings[index].Function = choice.source.selected.InstanceID, "GND"
		}
	}

	calculation, err := clockGenerationCalculation(requirements, choice, buffer.record, 1)
	if err != nil {
		return nil, err
	}
	id := "buffered_" + choice.architectureClass
	expansions, err := provider.expansion(request, id, parts, bindings, connections, []CalculationEvidence{calculation}, 0)
	if err != nil {
		return nil, err
	}

	// A cascaded endpoint stage is a materially distinct, still-generic
	// architecture alternative. It preserves the source selection and clock
	// evidence while adding isolation between the oscillator and public load.
	isolationBuffer := buffer
	isolationBuffer.selected.InstanceID, isolationBuffer.usage = "clock_isolation_buffer", "clock_buffer"
	isolationBuffer.near = "clock_source"
	isolationBypass := bufferBypass
	isolationBypass.selected.InstanceID = "clock_isolation_buffer_bypass"
	isolationBypass.near = "clock_isolation_buffer"
	alternativeParts := append(append([]catalogPart(nil), parts...), isolationBuffer, isolationBypass)
	for index := range alternativeParts {
		if alternativeParts[index].selected.InstanceID == buffer.selected.InstanceID {
			alternativeParts[index].near = isolationBuffer.selected.InstanceID
		}
	}
	alternativeConnections := append([]RealizationConnection(nil), connections...)
	for index := range alternativeConnections {
		alternativeConnections[index].Endpoints = append([]RealizationEndpoint(nil), alternativeConnections[index].Endpoints...)
		switch alternativeConnections[index].ID {
		case "clock_source_output":
			alternativeConnections[index].Endpoints = []RealizationEndpoint{endpoint(choice.source, "OUT"), endpoint(isolationBuffer, "IN")}
		case "clock_power":
			alternativeConnections[index].Endpoints = append(alternativeConnections[index].Endpoints, endpoint(isolationBuffer, "VCC"), endpoint(isolationBypass, "A"))
		case "clock_reference":
			alternativeConnections[index].Endpoints = append(alternativeConnections[index].Endpoints, endpoint(isolationBuffer, "GND"), endpoint(isolationBypass, "B"))
		}
	}
	alternativeConnections = append(alternativeConnections,
		semanticNet("clock_isolated_output", "clock", endpoint(isolationBuffer, "OUT"), endpoint(buffer, "IN")),
	)
	alternativeCalculation, err := clockGenerationCalculation(requirements, choice, buffer.record, 2)
	if err != nil {
		// The base architecture remains valid when the extra endpoint stage
		// alone exceeds a whole-circuit budget.
		return expansions, nil
	}
	alternative, err := provider.buildCatalogExpansion(
		request, id+"_cascaded_endpoint", alternativeParts, bindings, nil,
		alternativeConnections, []CalculationEvidence{alternativeCalculation}, nil, 0,
	)
	if err != nil {
		return nil, err
	}
	return append(expansions, alternative), nil
}

func clockRequirements(request ProviderRequest) (clockGenerationRequirements, error) {
	frequency, frequencyTolerance, err := requiredNumber(request.Constraints, "output_frequency", "target", "Hz")
	if err != nil || frequency <= 0 || frequencyTolerance <= 0 {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a positive output-frequency target and tolerance")
	}
	duty, dutyTolerance, dutyConstrained, err := optionalRequiredNumber(request.Constraints, "duty_cycle", "target", "%")
	if err != nil {
		return clockGenerationRequirements{}, err
	}
	if dutyConstrained && (duty <= 0 || duty >= 100 || dutyTolerance <= 0) {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation duty-cycle target and tolerance must be positive and bounded")
	}
	startup, _, startupConstrained, err := optionalRequiredNumber(request.Constraints, "maximum_startup_time", "maximum", "s")
	if err != nil {
		return clockGenerationRequirements{}, err
	}
	if startupConstrained && startup <= 0 {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation startup-time maximum must be positive")
	}
	fanout, _, err := requiredNumber(request.Constraints, "clock_fanout", "minimum", "")
	if err != nil || fanout < 1 || fanout != math.Trunc(fanout) {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires an integral positive fanout")
	}
	jitter, _, err := requiredNumber(request.Constraints, "maximum_rms_jitter", "maximum", "s")
	if err != nil || jitter <= 0 {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a positive RMS-jitter maximum")
	}
	supplyMinimum, supplyMaximum, supplyOK := roleVoltageRange(request.Ports, "power")
	if !supplyOK || supplyMinimum <= 0 || supplyMaximum < supplyMinimum {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a bounded positive supply")
	}
	loadMinimum, loadMaximum, loadOK := numericConstraintBounds(request.Constraints, "load_capacitance")
	if !loadOK || loadMinimum < 0 || loadMaximum <= 0 || loadMaximum < loadMinimum {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a bounded capacitive load")
	}
	requiredCurrent := totalRequiredRoleCurrentA(request.Ports, "output")
	if requiredCurrent <= 0 {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a positive output-current capacity")
	}
	maximumSupplyCurrent := maximumRoleCurrentDemandA(request.Ports, "power")
	if maximumSupplyCurrent <= 0 {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a positive supply-current budget")
	}
	temperature := temperatureRequirementFromConstraints(request.Constraints)
	if temperature == nil || temperature.MinimumC == nil || temperature.MaximumC == nil {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a bounded ambient-temperature range")
	}
	temperatureSpan := math.Max(math.Abs(*temperature.MinimumC-25), math.Abs(*temperature.MaximumC-25))
	outputHigh, outputHighOK := numericConstraintLowerBound(request.Constraints, "output_high_voltage")
	if !outputHighOK {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires a minimum output-high voltage")
	}
	riseTime, _, riseOK := firstNumericConstraint(request.Constraints, "rise_time")
	fallTime, _, fallOK := firstNumericConstraint(request.Constraints, "fall_time")
	if !riseOK || !fallOK || riseTime <= 0 || fallTime <= 0 {
		return clockGenerationRequirements{}, fmt.Errorf("clock generation requires positive rise- and fall-time maxima")
	}
	return clockGenerationRequirements{
		frequencyHz: frequency, frequencyTolerancePct: frequencyTolerance,
		dutyCyclePct: duty, dutyTolerancePct: dutyTolerance, dutyConstrained: dutyConstrained,
		maximumStartupS: startup, startupConstrained: startupConstrained,
		minimumFanout: fanout, maximumRMSJitterS: jitter,
		minimumOutputHighV: outputHigh, maximumRiseTimeS: riseTime, maximumFallTimeS: fallTime,
		supplyMinimumV: supplyMinimum, supplyMaximumV: supplyMaximum,
		maximumLoadF: loadMaximum, requiredOutputCurrentA: requiredCurrent, maximumSupplyCurrentA: maximumSupplyCurrent,
		temperature: temperature, temperatureSpanFrom25C: temperatureSpan,
	}, nil
}

func totalRequiredRoleCurrentA(ports []RoleContract, role string) float64 {
	total := 0.0
	for _, port := range ports {
		if port.Role == role && port.Contract.RequiredCurrentCapacityA != nil {
			total += *port.Contract.RequiredCurrentCapacityA
		}
	}
	return total
}

func optionalRequiredNumber(constraints []Constraint, name, relation, unit string) (float64, float64, bool, error) {
	if _, present := namedConstraint(constraints, name); !present {
		return 0, 0, false, nil
	}
	value, tolerance, err := requiredNumber(constraints, name, relation, unit)
	return value, tolerance, true, err
}

func (provider *CatalogProvider) selectClockArchitecture(ctx context.Context, requirements clockGenerationRequirements) (clockArchitectureChoice, error) {
	var records []components.ComponentRecord
	var timingResistors []components.ComponentRecord
	for _, record := range provider.catalog.Records {
		if record.Family == "resistor" {
			timingResistors = append(timingResistors, record)
		}
		evidence := record.Clock
		if record.Family == "clock_source" && evidence != nil && evidence.FabricationProof &&
			evidence.Frequency != nil && evidence.FrequencyAccuracy != nil && evidence.DutyCycle != nil &&
			evidence.StartupTime != nil && evidence.RMSJitter != nil {
			records = append(records, record)
		}
	}
	slices.SortFunc(records, func(left, right components.ComponentRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortFunc(timingResistors, func(left, right components.ComponentRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	var choices []clockArchitectureChoice
	matchedFrequency := false
	matchedAccuracy := false
	matchedDuty := false
	matchedStartup := false
	matchedJitter := false
	matchedSupply := false
	matchedTemperature := false
	for _, record := range records {
		evidence := record.Clock
		frequencyMinimum, frequencyMaximum, ok := clockEvidenceRange(evidence.Frequency, "Hz")
		if !ok || requirements.frequencyHz < frequencyMinimum || requirements.frequencyHz > frequencyMaximum {
			continue
		}
		if modelID := clockModelID(record); modelID == simmodel.PrimitiveFixedClockSourceV1 &&
			(frequencyMinimum != frequencyMaximum || math.Abs(requirements.frequencyHz-frequencyMinimum) > 1e-9*requirements.frequencyHz) {
			continue
		}
		matchedFrequency = true
		accuracyPercent, ok := clockAccuracyPercent(evidence.FrequencyAccuracy)
		if !ok {
			continue
		}
		modelID := clockModelID(record)
		if modelID == "" {
			continue
		}
		timingResistance := 0.0
		var choiceTiming catalogPart
		choiceTimingTolerance := 0.0
		choiceTimingTempco := 0.0
		if modelID != simmodel.PrimitiveFixedClockSourceV1 {
			scale, scaleOK := catalogSimulationParameterForModel(record, modelID, "frequency_scale_hz_ohm")
			divider, dividerOK := catalogSimulationParameterForModel(record, modelID, "divider_ratio")
			minimumR, minimumOK := catalogSimulationParameterForModel(record, modelID, "timing_resistance_min_ohm")
			maximumR, maximumOK := catalogSimulationParameterForModel(record, modelID, "timing_resistance_max_ohm")
			if !scaleOK || !dividerOK || !minimumOK || !maximumOK ||
				!finitePositive(scale) || !finitePositive(divider) ||
				!finitePositive(minimumR) || !finitePositive(maximumR) || minimumR > maximumR {
				continue
			}
			denominator := divider * requirements.frequencyHz
			if !finitePositive(denominator) {
				continue
			}
			timingResistance = scale / denominator
			if !finitePositive(timingResistance) || timingResistance < minimumR || timingResistance > maximumR {
				continue
			}
			timing, tolerancePercent, tempcoPPMPerC, timingErr := selectClockTimingResistor(
				ctx, timingResistors, timingResistance, requirements.temperatureSpanFrom25C,
				requirements.frequencyTolerancePct-accuracyPercent,
			)
			if timingErr != nil {
				continue
			}
			accuracyPercent += tolerancePercent + requirements.temperatureSpanFrom25C*tempcoPPMPerC/10_000
			choiceTiming = timing
			choiceTimingTolerance = tolerancePercent
			choiceTimingTempco = tempcoPPMPerC
		}
		if accuracyPercent > requirements.frequencyTolerancePct {
			continue
		}
		matchedAccuracy = true
		dutyMinimum, dutyMaximum, dutyOK := clockEvidenceRange(evidence.DutyCycle, "%")
		startup, startupOK := clockMeasurement(evidence.StartupTime, "s")
		jitter, jitterOK := clockMeasurement(evidence.RMSJitter, "s")
		if !dutyOK || !startupOK || !jitterOK {
			continue
		}
		if requirements.dutyConstrained &&
			(dutyMinimum < requirements.dutyCyclePct*(1-requirements.dutyTolerancePct/100) ||
				dutyMaximum > requirements.dutyCyclePct*(1+requirements.dutyTolerancePct/100)) {
			continue
		}
		matchedDuty = true
		if requirements.startupConstrained && startup > requirements.maximumStartupS {
			continue
		}
		matchedStartup = true
		if jitter > requirements.maximumRMSJitterS {
			continue
		}
		matchedJitter = true
		ratings := []components.RequiredRating{
			{Kind: "supply_voltage", Value: numericString(requirements.supplyMinimumV), Unit: "V"},
			{Kind: "supply_voltage", Value: numericString(requirements.supplyMaximumV), Unit: "V"},
		}
		supplyPart, supplyErr := provider.selectComponent(ctx, "clock_source", record.ID, ratings, true)
		if supplyErr != nil || supplyPart.record.ID != record.ID {
			continue
		}
		matchedSupply = true
		part, selectErr := provider.selectComponentWithTemperature(ctx, "clock_source", record.ID, ratings, true, requirements.temperature)
		if selectErr != nil || part.record.ID != record.ID {
			continue
		}
		matchedTemperature = true
		choices = append(choices, clockArchitectureChoice{
			source: part, modelID: modelID, architectureClass: evidence.ArchitectureClass,
			frequencyHz: requirements.frequencyHz, accuracyPercent: accuracyPercent,
			dutyMinimumPercent: dutyMinimum, dutyMaximumPercent: dutyMaximum,
			startupS: startup, jitterS: jitter, timingResistance: timingResistance,
			timingResistor: choiceTiming, timingTolerancePct: choiceTimingTolerance, timingTempcoPPM: choiceTimingTempco,
		})
	}
	if len(choices) == 0 {
		switch {
		case !matchedFrequency:
			return clockArchitectureChoice{}, clockUnsupported(CodeClockFrequencyUnsupported, "no fabrication-proven clock architecture supports the requested frequency")
		case !matchedAccuracy:
			return clockArchitectureChoice{}, clockUnsupported(CodeClockAccuracyUnsupported, "no fabrication-proven clock architecture supports the requested frequency accuracy")
		case !matchedDuty:
			return clockArchitectureChoice{}, clockUnsupported(CodeClockDutyUnsupported, "no fabrication-proven clock architecture supports the requested duty-cycle envelope")
		case !matchedStartup:
			return clockArchitectureChoice{}, clockUnsupported(CodeClockStartupUnsupported, "no fabrication-proven clock architecture supports the requested startup time")
		case !matchedJitter:
			return clockArchitectureChoice{}, clockUnsupported(CodeClockJitterUnsupported, "no fabrication-proven clock architecture supports the requested RMS jitter")
		case !matchedSupply:
			return clockArchitectureChoice{}, clockUnsupported(CodeClockSupplyUnsupported, "no fabrication-proven clock architecture supports the requested supply range")
		case !matchedTemperature:
			return clockArchitectureChoice{}, clockUnsupported(CodeClockTemperatureUnsupported, "no fabrication-proven clock architecture supports the requested temperature range")
		default:
			return clockArchitectureChoice{}, fmt.Errorf("no fabrication-proven clock architecture satisfies the requested envelope")
		}
	}
	slices.SortFunc(choices, func(left, right clockArchitectureChoice) int {
		if order := strings.Compare(left.architectureClass, right.architectureClass); order != 0 {
			return order
		}
		return strings.Compare(left.source.record.ID, right.source.record.ID)
	})
	return choices[0], nil
}

func selectClockTimingResistor(
	ctx context.Context,
	records []components.ComponentRecord,
	resistance float64,
	temperatureSpanC float64,
	maximumAddedErrorPercent float64,
) (catalogPart, float64, float64, error) {
	type qualified struct {
		part             catalogPart
		tolerancePercent float64
		tempcoPPMPerC    float64
		addedError       float64
	}
	var choices []qualified
	for _, record := range records {
		tolerancePercent, toleranceOK := catalogToleranceMaximum(record, "resistance", "%")
		if !toleranceOK || !finitePositive(tolerancePercent) {
			continue
		}
		tempcoPPMPerC := 0.0
		if temperatureSpanC > 0 {
			var tempcoOK bool
			tempcoPPMPerC, tempcoOK = recordValueMaximum(record, "temperature_coefficient", "ppm/C")
			if !tempcoOK || !finitePositive(tempcoPPMPerC) {
				continue
			}
		}
		if tolerancePercent+temperatureSpanC*tempcoPPMPerC/10_000 > maximumAddedErrorPercent {
			continue
		}
		actualValue, actualValueOK := recordValue(record, "resistance", "Ohm")
		if !actualValueOK {
			actualValue, actualValueOK = recordPreferredSeriesValue(record, "resistance", "Ohm", resistance)
		}
		if !actualValueOK || !finitePositive(actualValue) ||
			math.Abs(actualValue-resistance) > resistance*1e-12 {
			continue
		}
		variantID := ""
		for _, variant := range record.Packages {
			if variant.ID != "" && (variantID == "" || variant.ID < variantID) {
				variantID = variant.ID
			}
		}
		if variantID == "" {
			continue
		}
		addedError := tolerancePercent + temperatureSpanC*tempcoPPMPerC/10_000
		if !finitePositive(addedError) || addedError > maximumAddedErrorPercent {
			continue
		}
		evidence := componentEvidence(record, record.Verification.Confidence)
		part := catalogPart{
			selected: SelectedComponent{
				InstanceID: canonicalIdentifier(record.Family), CatalogID: record.ID,
				VariantID: variantID, Evidence: evidence.Confidence,
			},
			record: record, usage: canonicalIdentifier(record.Family), evidence: evidence,
			value:         engineeringValue(actualValue, "Ohm"),
			toleranceKind: "resistance", maximumTolerance: tolerancePercent, toleranceUnit: "%",
			maximumTempcoPPMPerC: tempcoPPMPerC,
		}
		choices = append(choices, qualified{
			part: part, tolerancePercent: tolerancePercent,
			tempcoPPMPerC: tempcoPPMPerC, addedError: addedError,
		})
	}
	if err := ctx.Err(); err != nil {
		return catalogPart{}, 0, 0, err
	}
	slices.SortStableFunc(choices, func(left, right qualified) int {
		if left.addedError < right.addedError {
			return -1
		}
		if left.addedError > right.addedError {
			return 1
		}
		if left.tolerancePercent < right.tolerancePercent {
			return -1
		}
		if left.tolerancePercent > right.tolerancePercent {
			return 1
		}
		if left.tempcoPPMPerC < right.tempcoPPMPerC {
			return -1
		}
		if left.tempcoPPMPerC > right.tempcoPPMPerC {
			return 1
		}
		if order := strings.Compare(left.part.record.ID, right.part.record.ID); order != 0 {
			return order
		}
		return strings.Compare(left.part.selected.VariantID, right.part.selected.VariantID)
	})
	if len(choices) == 0 {
		return catalogPart{}, 0, 0, fmt.Errorf("no exact-value catalog timing resistor satisfies the remaining frequency-error budget")
	}
	selected := choices[0]
	return selected.part, selected.tolerancePercent, selected.tempcoPPMPerC, nil
}

func clockGenerationCalculation(requirements clockGenerationRequirements, choice clockArchitectureChoice, buffer components.ComponentRecord, bufferStages int) (CalculationEvidence, error) {
	if bufferStages <= 0 {
		return CalculationEvidence{}, fmt.Errorf("clock generation requires at least one endpoint-buffer stage")
	}
	if buffer.Interface == nil || buffer.Interface.OutputCurrent == nil || buffer.Interface.EdgeTime == nil ||
		buffer.Interface.OutputHighMinimumV == nil || buffer.Interface.OutputLowMaximumV == nil || buffer.Interface.MaximumFrequency == nil {
		return CalculationEvidence{}, fmt.Errorf("clock endpoint buffer lacks complete quantitative evidence")
	}
	bufferCurrent, currentOK := clockMeasurement(buffer.Interface.OutputCurrent, "A")
	bufferResistance, resistanceOK := clockMeasurement(buffer.Interface.OutputImpedance, "Ohm")
	edgeMinimum, edgeMaximum, edgeOK := clockEvidenceRange(buffer.Interface.EdgeTime, "s")
	_ = edgeMinimum
	bufferFrequency, frequencyOK := clockMeasurement(buffer.Interface.MaximumFrequency, "Hz")
	bufferLoad := recordRatingOrZero(buffer, "capacitive_load", "F")
	bufferFanout := recordRatingOrZero(buffer, "fanout", "count")
	sourceSupplyCurrent, sourceSupplyOK := clockMeasurement(choice.source.record.Clock.SupplyCurrent, "A")
	bufferSupplyCurrent, bufferSupplyOK := catalogSimulationParameterForModel(buffer, simmodel.PrimitiveCMOSBufferV1, "supply_current_a")
	if !currentOK || !resistanceOK || !edgeOK || !frequencyOK || !sourceSupplyOK || !bufferSupplyOK {
		return CalculationEvidence{}, fmt.Errorf("clock endpoint-buffer evidence uses unsupported units")
	}
	outputHighWorstV := requirements.supplyMinimumV - bufferResistance*requirements.requiredOutputCurrentA
	if outputHighWorstV < 0 {
		outputHighWorstV = 0
	}
	outputLowWorstV := *buffer.Interface.OutputLowMaximumV
	totalSupplyCurrentA := sourceSupplyCurrent + float64(bufferStages)*bufferSupplyCurrent
	requiredBypassF := bufferCurrent * edgeMaximum / clockBypassAllowableDroopV
	bounds := []CalculationBound{
		maximumBound("frequency_accuracy", requirements.frequencyTolerancePct, choice.accuracyPercent, "%"),
		maximumBound("rms_jitter", requirements.maximumRMSJitterS, choice.jitterS, "s"),
		minimumBound("output_current", requirements.requiredOutputCurrentA, bufferCurrent, "A"),
		minimumBound("capacitive_load", requirements.maximumLoadF, bufferLoad, "F"),
		minimumBound("fanout", requirements.minimumFanout, bufferFanout, "count"),
		maximumBound("supply_current", requirements.maximumSupplyCurrentA, totalSupplyCurrentA, "A"),
		minimumBound("local_bypass_capacitance", requiredBypassF, clockBypassCapacitanceF, "F"),
		minimumBound("output_high_voltage", requirements.minimumOutputHighV, outputHighWorstV, "V"),
		maximumBound("rise_time", requirements.maximumRiseTimeS, edgeMaximum, "s"),
		maximumBound("fall_time", requirements.maximumFallTimeS, edgeMaximum, "s"),
		minimumBound("maximum_frequency", requirements.frequencyHz, bufferFrequency, "Hz"),
	}
	if requirements.dutyConstrained {
		dutyMinimumRequired := requirements.dutyCyclePct * (1 - requirements.dutyTolerancePct/100)
		dutyMaximumRequired := requirements.dutyCyclePct * (1 + requirements.dutyTolerancePct/100)
		bounds = append(bounds,
			minimumBound("duty_cycle_minimum", dutyMinimumRequired, choice.dutyMinimumPercent, "%"),
			maximumBound("duty_cycle_maximum", dutyMaximumRequired, choice.dutyMaximumPercent, "%"),
		)
	}
	if requirements.startupConstrained {
		bounds = append(bounds, maximumBound("startup_time", requirements.maximumStartupS, choice.startupS, "s"))
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	evidence := CalculationEvidence{
		ID: "clock_generation_worst_case", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "target_frequency", Value: requirements.frequencyHz, Unit: "Hz"},
			{Name: "supply_minimum", Value: requirements.supplyMinimumV, Unit: "V"},
			{Name: "supply_maximum", Value: requirements.supplyMaximumV, Unit: "V"},
			{Name: "load_capacitance", Value: requirements.maximumLoadF, Unit: "F"},
			{Name: "fanout", Value: requirements.minimumFanout, Unit: "count"},
			{Name: "temperature_span_from_25c", Value: requirements.temperatureSpanFrom25C, Unit: "degC"},
			{Name: "endpoint_buffer_stages", Value: float64(bufferStages), Unit: "count"},
		},
		NominalOutputs: []NamedQuantity{
			{Name: "frequency", Value: choice.frequencyHz, Unit: "Hz"},
			{Name: "timing_resistance", Value: choice.timingResistance, Unit: "Ohm"},
			{Name: "output_high_voltage", Value: outputHighWorstV, Unit: "V"},
			{Name: "output_low_voltage", Value: outputLowWorstV, Unit: "V"},
			{Name: "edge_time", Value: edgeMaximum, Unit: "s"},
			{Name: "supply_current", Value: totalSupplyCurrentA, Unit: "A"},
			{Name: "local_bypass_capacitance", Value: clockBypassCapacitanceF, Unit: "F"},
		},
		Corners: clockGenerationCorners(requirements, choice, bufferResistance, outputLowWorstV, edgeMaximum, totalSupplyCurrentA),
		Bounds:  bounds, CornerEvaluations: 8, WorstMargin: worstMargin, Pass: pass,
	}
	if choice.timingResistance > 0 {
		evidence.Inputs = append(evidence.Inputs,
			NamedQuantity{Name: "timing_resistance_tolerance", Value: choice.timingTolerancePct, Unit: "%"},
			NamedQuantity{Name: "timing_resistance_tempco", Value: choice.timingTempcoPPM, Unit: "ppm/C"},
		)
	}
	finalized, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, fmt.Errorf("clock-generation calculation finalization failed: %w", err)
	}
	if !pass {
		return CalculationEvidence{}, fmt.Errorf("clock-generation worst-case envelope is unproven")
	}
	return finalized, nil
}

func clockGenerationCorners(requirements clockGenerationRequirements, choice clockArchitectureChoice, bufferResistance, outputLowWorstV, edgeMaximum, totalSupplyCurrentA float64) []CornerEvidence {
	type cornerValue struct {
		id    string
		value float64
	}
	supplies := []cornerValue{{id: "minimum_supply", value: requirements.supplyMinimumV}, {id: "maximum_supply", value: requirements.supplyMaximumV}}
	temperatures := []cornerValue{{id: "minimum_temperature", value: *requirements.temperature.MinimumC}, {id: "maximum_temperature", value: *requirements.temperature.MaximumC}}
	tolerances := []cornerValue{{id: "negative_tolerance", value: -choice.accuracyPercent}, {id: "positive_tolerance", value: choice.accuracyPercent}}
	corners := make([]CornerEvidence, 0, len(supplies)*len(temperatures)*len(tolerances))
	for _, supply := range supplies {
		for _, temperature := range temperatures {
			for _, tolerance := range tolerances {
				outputHigh := math.Max(0, supply.value-bufferResistance*requirements.requiredOutputCurrentA)
				dutyCycle := choice.dutyMinimumPercent
				if tolerance.value > 0 {
					dutyCycle = choice.dutyMaximumPercent
				}
				outputs := []NamedQuantity{
					{Name: "frequency", Value: requirements.frequencyHz * (1 + tolerance.value/100), Unit: "Hz"},
					{Name: "frequency_accuracy", Value: math.Abs(tolerance.value), Unit: "%"},
					{Name: "duty_cycle", Value: dutyCycle, Unit: "%"},
					{Name: "output_high_voltage", Value: outputHigh, Unit: "V"},
					{Name: "output_low_voltage", Value: outputLowWorstV, Unit: "V"},
					{Name: "edge_time", Value: edgeMaximum, Unit: "s"},
					{Name: "supply_current", Value: totalSupplyCurrentA, Unit: "A"},
				}
				if choice.timingResistance > 0 {
					outputs = append(outputs, NamedQuantity{
						Name:  "timing_resistance",
						Value: choice.timingResistance * (1 + math.Copysign(choice.timingTolerancePct, tolerance.value)/100),
						Unit:  "Ohm",
					})
				}
				corners = append(corners, CornerEvidence{
					ID: supply.id + "_" + temperature.id + "_" + tolerance.id,
					Inputs: []NamedQuantity{
						{Name: "supply", Value: supply.value, Unit: "V"},
						{Name: "temperature", Value: temperature.value, Unit: "degC"},
						{Name: "component_tolerance", Value: tolerance.value, Unit: "%"},
						{Name: "load_capacitance", Value: requirements.maximumLoadF, Unit: "F"},
						{Name: "fanout", Value: requirements.minimumFanout, Unit: "count"},
					},
					Outputs: outputs,
				})
			}
		}
	}
	return corners
}

func clockModelID(record components.ComponentRecord) string {
	for _, model := range record.SimulationModels {
		if model.ModelID == simmodel.PrimitiveFixedClockSourceV1 || model.ModelID == simmodel.PrimitiveResistorProgrammedClockSourceV1 {
			return model.ModelID
		}
	}
	return ""
}

func clockEvidenceRange(evidence *components.EvidenceRange, unit string) (float64, float64, bool) {
	if evidence == nil {
		return 0, 0, false
	}
	minimum, maximum := evidence.Minimum, evidence.Maximum
	if minimum == nil {
		minimum = evidence.Typical
	}
	if maximum == nil {
		maximum = evidence.Typical
	}
	if minimum == nil || maximum == nil {
		return 0, 0, false
	}
	convertedMinimum, minimumOK := convertCatalogUnit(*minimum, evidence.Unit, unit)
	convertedMaximum, maximumOK := convertCatalogUnit(*maximum, evidence.Unit, unit)
	return convertedMinimum, convertedMaximum, minimumOK && maximumOK && convertedMaximum >= convertedMinimum
}

func clockMeasurement(evidence *components.EvidenceMeasurement, unit string) (float64, bool) {
	if evidence == nil {
		return 0, false
	}
	return convertCatalogUnit(evidence.Value, evidence.Unit, unit)
}

func clockAccuracyPercent(evidence *components.EvidenceMeasurement) (float64, bool) {
	if evidence == nil {
		return 0, false
	}
	switch evidence.Unit {
	case "%":
		return evidence.Value, finiteNumbers(evidence.Value) && evidence.Value >= 0
	case "ppm":
		return evidence.Value / 10_000, finiteNumbers(evidence.Value) && evidence.Value >= 0
	default:
		return 0, false
	}
}
