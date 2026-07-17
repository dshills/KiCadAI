package circuitgraph

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

const (
	synthesisAnalogBiasRailFraction = 0.1
	synthesisACStartFrequencyHz     = 100.0
	synthesisACStopFrequencyHz      = 10000.0
	synthesisACSweepPoints          = 21
	synthesisMaxIndependentACInputs = 7
	celsiusToKelvin                 = 273.15
)

type synthesisSourceCondition struct {
	component      string
	node           string
	dcValue        float64
	sourcePolarity float64
	acInput        bool
	pulseInput     bool
}

type synthesisTransientCondition struct {
	initialValueV float64
	pulseValueV   float64
	delayS        float64
	widthS        float64
	periodS       float64
	durationS     float64
	timeStepS     float64
}

func deriveSynthesisSimulation(document Document, intent FunctionIntent, selected map[string]ResolvedComponent) (*SimulationIntent, SynthesisSimulationEvidence) {
	connections := make(map[string][]simmodel.ConnectionEvidence, len(selected))
	for _, net := range document.Nets {
		for _, endpoint := range net.Endpoints {
			connections[endpoint.Component] = append(connections[endpoint.Component], simmodel.ConnectionEvidence{
				Function: endpoint.Selector, UnitID: endpoint.Unit, Net: net.Name,
			})
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	evidence := make([]simmodel.ComponentEvidence, 0, len(ids))
	for _, id := range ids {
		selection := selected[id]
		value, hasValue := components.ParseEngineeringValue(selection.Instance.Value)
		evidence = append(evidence, componentSimulationEvidence(
			selection.Instance.ID, selection.ComponentID, selection.Record.Family, value, hasValue,
			configuredSimulationModels(selection), connections[selection.Instance.ID], selection.Units, selection.Record,
		)...)
	}
	// Applicability is model completeness, not numerical optimism. Resolution
	// validates the full terminal topology, and evaluation's pivoted MNA solver
	// returns a singular/floating-node diagnostic instead of a partial result.
	transient, transientRequested, transientReason := synthesisTransientOperatingCase(intent.Functions)
	if transientReason != "" {
		return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: transientReason}
	}
	var modelID string
	var applicable bool
	var applicabilityReason string
	if transientRequested {
		modelID, applicable, applicabilityReason = simmodel.ApplicableGraphModelForAnalysis(evidence, simmodel.AnalysisTransient)
	} else {
		modelID, applicable, applicabilityReason = simmodel.ApplicableGraphModel(evidence)
	}
	if !applicable {
		return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: applicabilityReason}
	}

	domains := make(map[string]PowerDomainIntent, len(intent.PowerDomains))
	for _, domain := range intent.PowerDomains {
		domains[domain.Name] = domain
	}
	operatingRail := synthesisOperatingRail(intent.PowerDomains)
	var sources []synthesisSourceCondition
	for _, requirement := range intent.Interfaces {
		if requirement.Role != InterfacePowerInput && requirement.Role != InterfaceAnalogInput && requirement.Role != InterfaceDigitalIn {
			continue
		}
		componentID := stableGeneratedID("iface", requirement.ID)
		selection, exists := selected[componentID]
		if !exists || !synthesisRecordHasModel(selection.Record.SimulationModels, simmodel.PrimitiveConnectorVoltageSourceV1) || len(requirement.Signals) != 2 {
			continue
		}
		positiveSignal, sourcePolarity, sourceCompatible := synthesisSourceInterfaceSignal(requirement)
		if !sourceCompatible {
			continue
		}
		node, voltageDomain := synthesisInterfaceSignalNet(intent.Connections, requirement.ID, positiveSignal)
		if node == "" {
			return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "no_bounded_interface_operating_condition"}
		}
		condition := synthesisSourceCondition{component: componentID, node: node, sourcePolarity: sourcePolarity}
		switch requirement.Role {
		case InterfacePowerInput:
			domain, ok := domains[voltageDomain]
			if !ok || domain.VoltageV == 0 {
				return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "no_bounded_interface_operating_condition"}
			}
			condition.dcValue = domain.VoltageV
		case InterfaceAnalogInput:
			if operatingRail == 0 {
				return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "no_bounded_interface_operating_condition"}
			}
			// A deterministic low common-mode bias leaves headroom for catalog-
			// resolved non-inverting gain without requiring topology-specific
			// equations or provider-controlled operating points.
			condition.dcValue = operatingRail * synthesisAnalogBiasRailFraction
			condition.acInput = true
			condition.pulseInput = true
		case InterfaceDigitalIn:
			if operatingRail == 0 {
				return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "no_bounded_interface_operating_condition"}
			}
			condition.dcValue = operatingRail
			condition.pulseInput = true
		default:
			continue
		}
		sources = append(sources, condition)
	}
	if len(sources) == 0 {
		return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "no_bounded_interface_operating_condition"}
	}
	slices.SortStableFunc(sources, func(left, right synthesisSourceCondition) int {
		if left.component < right.component {
			return -1
		}
		if left.component > right.component {
			return 1
		}
		return 0
	})
	if transientRequested {
		return deriveSynthesisTransient(modelID, sources, transient)
	}

	dc := simmodel.Analysis{ID: "dc_operating_point", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{}}
	assertions := make([]simmodel.Assertion, 0, len(sources)*2)
	hasACInput := false
	for _, source := range sources {
		dc.Excitations = append(dc.Excitations, simmodel.SourceExcitation{Component: source.component, DCValue: source.dcValue * source.sourcePolarity})
		tolerance := math.Max(1e-3, math.Abs(source.dcValue)*0.01)
		assertions = append(assertions, simmodel.Assertion{
			AnalysisID: dc.ID, Node: source.node, Quantity: simmodel.QuantityVoltageV,
			Min: source.dcValue - tolerance, Max: source.dcValue + tolerance,
		})
		hasACInput = hasACInput || source.acInput
	}
	analyses := []simmodel.Analysis{dc}
	// The registered nonlinear workflow is deliberately DC-only and rejects AC
	// intent. Small-signal AC remains inapplicable until the trusted registry has
	// an explicit operating-point linearization model and bounded implementation.
	if modelID == simmodel.ModelLinearCircuitMNAV1 && hasACInput {
		var acInputs []synthesisSourceCondition
		for _, source := range sources {
			if source.acInput {
				acInputs = append(acInputs, source)
			}
		}
		if len(acInputs) > synthesisMaxIndependentACInputs {
			return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "too_many_independent_ac_inputs"}
		}
		for inputIndex, target := range acInputs {
			ac := simmodel.Analysis{
				ID: "ac_sweep_" + strconv.Itoa(inputIndex+1), Kind: simmodel.AnalysisACSweep,
				StartFrequencyHz: synthesisACStartFrequencyHz, StopFrequencyHz: synthesisACStopFrequencyHz,
				Points: synthesisACSweepPoints, Excitations: []simmodel.SourceExcitation{},
			}
			for _, source := range sources {
				magnitude := 0.0
				phase := 0.0
				if source.component == target.component {
					magnitude = 1
					if source.sourcePolarity < 0 {
						phase = 180
					}
				}
				ac.Excitations = append(ac.Excitations, simmodel.SourceExcitation{
					Component: source.component, DCValue: source.dcValue * source.sourcePolarity,
					ACMagnitude: magnitude, ACPhaseDeg: phase,
				})
			}
			assertions = append(assertions, simmodel.Assertion{
				AnalysisID: ac.ID, Node: target.node, Quantity: simmodel.QuantityVoltageMagnitudeV,
				FrequencyHz: ac.StartFrequencyHz, Min: .99, Max: 1.01,
			})
			analyses = append(analyses, ac)
		}
	}
	return &SimulationIntent{ModelID: modelID, Analyses: analyses, Assertions: assertions}, SynthesisSimulationEvidence{
		Status: "derived", ModelID: modelID, Reason: "complete_registered_graph_model_and_bounded_interface_conditions",
	}
}

func configuredSimulationModels(selection ResolvedComponent) []simmodel.CatalogEvidence {
	models := make([]simmodel.CatalogEvidence, len(selection.Record.SimulationModels))
	for index, model := range selection.Record.SimulationModels {
		models[index] = simmodel.CatalogEvidence{ModelID: model.ModelID, Parameters: append([]simmodel.NamedValue(nil), model.Parameters...), Uncertainties: append([]simmodel.Uncertainty(nil), model.Uncertainties...)}
		for parameterIndex := range models[index].Parameters {
			name := models[index].Parameters[parameterIndex].Name
			configured := synthesisParameterString(selection.Instance.Parameters, name)
			value, parsed := components.ParseEngineeringValue(configured)
			if configured != "" && parsed && !math.IsNaN(value) && !math.IsInf(value, 0) {
				models[index].Parameters[parameterIndex].Value = value
			}
		}
	}
	return models
}

func componentSimulationEvidence(instanceID, catalogID, family string, value float64, hasValue bool, models []simmodel.CatalogEvidence, connections []simmodel.ConnectionEvidence, units []ResolvedUnit, record components.ComponentRecord) []simmodel.ComponentEvidence {
	uncertainties := append(catalogValueUncertainties(value, hasValue, record), catalogModelUncertainties(models, record)...)
	functionalUnits := []ResolvedUnit{}
	sharedUnits := map[string]bool{}
	for _, unit := range units {
		if unit.Type == components.SymbolUnitFunctional {
			functionalUnits = append(functionalUnits, unit)
		} else if unit.Required || unit.Type == components.SymbolUnitPower {
			sharedUnits[unit.ID] = true
		}
	}
	if len(functionalUnits) <= 1 || !synthesisRecordHasModel(models, simmodel.PrimitiveOpAmpV1) {
		return []simmodel.ComponentEvidence{{
			InstanceID: instanceID, CatalogID: catalogID, Family: family, ValueSI: value, HasValueSI: hasValue,
			ModelClaims: models, Connections: connections, Uncertainties: uncertainties,
		}}
	}
	evidence := make([]simmodel.ComponentEvidence, 0, len(functionalUnits))
	for _, unit := range functionalUnits {
		unitConnections := []simmodel.ConnectionEvidence{}
		for _, connection := range connections {
			if connection.UnitID == unit.ID || sharedUnits[connection.UnitID] {
				unitConnections = append(unitConnections, connection)
			}
		}
		evidence = append(evidence, simmodel.ComponentEvidence{
			InstanceID: instanceID + "." + unit.ID, PhysicalComponent: instanceID,
			CatalogID: catalogID, Family: family, ValueSI: value, HasValueSI: hasValue,
			ModelClaims: models, Connections: unitConnections, Uncertainties: uncertainties,
		})
	}
	return evidence
}

func catalogModelUncertainties(models []simmodel.CatalogEvidence, record components.ComponentRecord) []simmodel.Uncertainty {
	var result []simmodel.Uncertainty
	for _, model := range models {
		hasTemperatureEvidence := false
		for _, uncertainty := range model.Uncertainties {
			if strings.HasPrefix(uncertainty.Target, "model_parameters.") || uncertainty.Target == "excitation_dc_value" {
				result = append(result, uncertainty)
			}
			hasTemperatureEvidence = hasTemperatureEvidence || uncertainty.Target == "model_parameters.junction_temperature_k"
		}
		if temperatureUncertainty, ok := catalogTemperatureUncertainty(model, record); ok && !hasTemperatureEvidence {
			result = append(result, temperatureUncertainty)
		}
	}
	slices.SortStableFunc(result, func(a, b simmodel.Uncertainty) int { return strings.Compare(a.Target, b.Target) })
	return result
}

func catalogTemperatureUncertainty(model simmodel.CatalogEvidence, record components.ComponentRecord) (simmodel.Uncertainty, bool) {
	if record.Temperature == nil || (record.Temperature.Unit != "C" && record.Temperature.Unit != "K") || len(record.Verification.Sources) == 0 {
		return simmodel.Uncertainty{}, false
	}
	nominal, found := 0.0, false
	for _, parameter := range model.Parameters {
		if parameter.Name == "junction_temperature_k" {
			nominal, found = parameter.Value, true
			break
		}
	}
	if !found {
		return simmodel.Uncertainty{}, false
	}
	minimum, minimumOK := components.ParseEngineeringValue(record.Temperature.Min)
	maximum, maximumOK := components.ParseEngineeringValue(record.Temperature.Max)
	if !minimumOK || !maximumOK || minimum > maximum {
		return simmodel.Uncertainty{}, false
	}
	if record.Temperature.Unit == "C" {
		minimum += celsiusToKelvin
		maximum += celsiusToKelvin
	}
	if nominal < minimum || nominal > maximum {
		return simmodel.Uncertainty{}, false
	}
	return simmodel.Uncertainty{Target: "model_parameters.junction_temperature_k", Source: "catalog:" + record.ID + ":temperature", Nominal: nominal, Minimum: minimum, Maximum: maximum}, true
}

func catalogValueUncertainties(value float64, hasValue bool, record components.ComponentRecord) []simmodel.Uncertainty {
	if !hasValue || value <= 0 || len(record.Verification.Sources) == 0 {
		return nil
	}
	kind := ""
	switch record.Family {
	case "resistor":
		kind = "resistance"
	case "capacitor":
		kind = "capacitance"
	default:
		return nil
	}
	for _, tolerance := range record.Tolerances {
		if tolerance.Kind != kind || tolerance.Unit != "%" {
			continue
		}
		amount, ok := components.ParseEngineeringValue(tolerance.Max)
		if !ok || amount <= 0 || amount >= 100 {
			continue
		}
		fraction := amount / 100
		return []simmodel.Uncertainty{{Target: "value_si", Source: "catalog:" + record.ID + ":" + kind + "_tolerance", Nominal: value, Minimum: value * (1 - fraction), Maximum: value * (1 + fraction)}}
	}
	return nil
}

func deriveSynthesisTransient(modelID string, sources []synthesisSourceCondition, condition synthesisTransientCondition) (*SimulationIntent, SynthesisSimulationEvidence) {
	pulseInputs := 0
	for _, source := range sources {
		if source.pulseInput {
			pulseInputs++
		}
	}
	if pulseInputs != 1 {
		// A single operating-case parameter set cannot identify which of
		// multiple independent inputs should be pulsed. Fail closed instead of
		// silently choosing one or exciting unrelated inputs together.
		return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "transient_requires_exactly_one_input_source"}
	}
	analysis := simmodel.Analysis{
		ID: "transient_operating_case", Kind: simmodel.AnalysisTransient,
		DurationS: condition.durationS, TimeStepS: condition.timeStepS,
		Excitations: []simmodel.SourceExcitation{},
	}
	assertions := make([]simmodel.Assertion, 0, len(sources))
	for _, source := range sources {
		excitation := simmodel.SourceExcitation{Component: source.component}
		expected := source.dcValue
		timeS := 0.0
		if source.pulseInput {
			// Polarity corrects the trusted connector primitive's pin equation;
			// expected remains the physical voltage at source.node. For example,
			// a signal on pin 2 requires a negative source value to produce a
			// positive node voltage relative to grounded pin 1.
			excitation.PulseInitialValue = condition.initialValueV * source.sourcePolarity
			excitation.PulseValue = condition.pulseValueV * source.sourcePolarity
			excitation.PulseDelayS = condition.delayS
			excitation.PulseWidthS = condition.widthS
			excitation.PulsePeriodS = condition.periodS
			expected = condition.pulseValueV
			// The trusted pulse contract changes an ideal source at delayS:
			// transientSourceValue uses the initial value only for time < delay.
			// Equality is therefore the first exact-grid pulsed sample. A
			// midpoint could fall between grid points when width spans odd steps.
			timeS = condition.delayS
		} else {
			excitation.DCValue = source.dcValue * source.sourcePolarity
		}
		analysis.Excitations = append(analysis.Excitations, excitation)
		tolerance := math.Max(1e-3, math.Abs(expected)*0.01)
		assertions = append(assertions, simmodel.Assertion{
			AnalysisID: analysis.ID, Node: source.node, Quantity: simmodel.QuantityVoltageV,
			TimeS: timeS, Min: expected - tolerance, Max: expected + tolerance,
		})
	}
	return &SimulationIntent{ModelID: modelID, Analyses: []simmodel.Analysis{analysis}, Assertions: assertions}, SynthesisSimulationEvidence{
		Status: "derived", ModelID: modelID, Reason: "complete_registered_graph_model_and_bounded_transient_operating_case",
	}
}

func synthesisTransientOperatingCase(functions []FunctionRequirement) (synthesisTransientCondition, bool, string) {
	names := []string{
		"pulse_initial_value_v", "pulse_value_v", "pulse_delay_s", "pulse_width_s", "pulse_period_s",
		"analysis_duration_s", "analysis_time_step_s",
	}
	var result synthesisTransientCondition
	found := false
	for _, function := range functions {
		values := make(map[string]float64, len(names))
		present := 0
		for _, name := range names {
			value, exists, valid := synthesisParameterFloat(function.Parameters, name)
			if exists {
				present++
				if !valid {
					return synthesisTransientCondition{}, true, "invalid_bounded_transient_operating_parameter"
				}
				values[name] = value
			}
		}
		if present == 0 {
			continue
		}
		if present != len(names) {
			return synthesisTransientCondition{}, true, "incomplete_bounded_transient_operating_case"
		}
		if found {
			return synthesisTransientCondition{}, true, "ambiguous_bounded_transient_operating_case"
		}
		found = true
		result = synthesisTransientCondition{
			initialValueV: values["pulse_initial_value_v"], pulseValueV: values["pulse_value_v"],
			delayS: values["pulse_delay_s"], widthS: values["pulse_width_s"], periodS: values["pulse_period_s"],
			durationS: values["analysis_duration_s"], timeStepS: values["analysis_time_step_s"],
		}
	}
	return result, found, ""
}

func synthesisParameterFloat(parameters []Parameter, name string) (float64, bool, bool) {
	for _, parameter := range parameters {
		if !strings.EqualFold(parameter.Name, name) {
			continue
		}
		var value float64
		if parameter.Value.Number != nil {
			value = *parameter.Value.Number
		} else if parameter.Value.String != nil {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(*parameter.Value.String), 64)
			if err != nil {
				return 0, true, false
			}
			value = parsed
		} else {
			return 0, true, false
		}
		return value, true, !math.IsNaN(value) && !math.IsInf(value, 0)
	}
	return 0, false, false
}

func synthesisOperatingRail(domains []PowerDomainIntent) float64 {
	// Choose the reviewed supply with the greatest excursion from the zero-volt
	// reference. Equal-magnitude bipolar rails prefer the positive rail, while a
	// negative-only system retains its sign for input bias and asserted levels.
	rail := 0.0
	for _, domain := range domains {
		if domain.Role != NetRolePower && domain.Role != NetRolePowerPos && domain.Role != NetRolePowerNeg {
			continue
		}
		magnitude := math.Abs(domain.VoltageV)
		if magnitude > math.Abs(rail) || magnitude == math.Abs(rail) && domain.VoltageV > rail {
			rail = domain.VoltageV
		}
	}
	return rail
}

func synthesisSourceInterfaceSignal(requirement InterfaceRequirement) (string, float64, bool) {
	// The attached trusted primitive is explicitly a 1x02 model. Wider
	// connectors remain boundaries unless their catalog record declares its own
	// reviewed source-terminal primitive.
	if len(requirement.Signals) != 2 {
		return "", 0, false
	}
	groundCount := 0
	positiveIndex := -1
	for index, signal := range requirement.Signals {
		if signal.Role == NetRoleGround || signal.Role == NetRoleReturn {
			groundCount++
			continue
		}
		switch requirement.Role {
		case InterfacePowerInput:
			if signal.Role == NetRolePower || signal.Role == NetRolePowerPos || signal.Role == NetRolePowerNeg {
				positiveIndex = index
			}
		case InterfaceAnalogInput, InterfaceDigitalIn:
			if signal.Role == NetRoleSignal {
				positiveIndex = index
			}
		}
	}
	if groundCount != 1 || positiveIndex < 0 {
		return "", 0, false
	}
	polarity := 1.0
	if positiveIndex == 1 {
		polarity = -1
	}
	return requirement.Signals[positiveIndex].Name, polarity, true
}

func synthesisRecordHasModel(claims []simmodel.CatalogEvidence, modelID string) bool {
	for _, claim := range claims {
		if claim.ModelID == modelID {
			return true
		}
	}
	return false
}

func synthesisInterfaceSignalNet(connections []FunctionConnection, interfaceID, signal string) (string, string) {
	for _, connection := range connections {
		for _, endpoint := range connection.Endpoints {
			if endpoint.Interface == interfaceID && endpoint.Signal == signal {
				return connection.Name, connection.VoltageDomain
			}
		}
	}
	return "", ""
}
