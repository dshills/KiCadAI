package circuitgraph

import (
	"math"
	"slices"
	"strconv"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

const (
	synthesisAnalogBiasRailFraction = 0.1
	synthesisACStartFrequencyHz     = 100.0
	synthesisACStopFrequencyHz      = 10000.0
	synthesisACSweepPoints          = 21
	synthesisMaxIndependentACInputs = 7
)

type synthesisSourceCondition struct {
	component      string
	node           string
	dcValue        float64
	sourcePolarity float64
	acInput        bool
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
		evidence = append(evidence, simmodel.ComponentEvidence{
			InstanceID: selection.Instance.ID, CatalogID: selection.ComponentID, Family: selection.Record.Family,
			ValueSI: value, HasValueSI: hasValue, ModelClaims: selection.Record.SimulationModels,
			Connections: connections[selection.Instance.ID],
		})
	}
	// Applicability is model completeness, not numerical optimism. Resolution
	// validates the full terminal topology, and evaluation's pivoted MNA solver
	// returns a singular/floating-node diagnostic instead of a partial result.
	modelID, applicable, applicabilityReason := simmodel.ApplicableGraphModel(evidence)
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
		case InterfaceDigitalIn:
			if operatingRail == 0 {
				return nil, SynthesisSimulationEvidence{Status: "not_applicable", Reason: "no_bounded_interface_operating_condition"}
			}
			condition.dcValue = operatingRail
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
