package simmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"
)

const (
	maxMNAAnalyses        = 8
	maxMNASweepPoints     = 64
	maxMNAExcitations     = 16
	maxMNASourceMagnitude = 1e6
	maxTransientSteps     = 2048
	maxTransientWork      = (maxTransientSteps + 6) * nonlinearMaxIterations
	minTransientTimeStepS = 1e-9
	maxTransientDurationS = 10
)

type primitiveDefinition struct {
	ID                string      `json:"id"`
	Family            string      `json:"family"`
	Terminals         []string    `json:"terminals"`
	RequiresValueSI   bool        `json:"requires_value_si,omitempty"`
	CatalogParameters []valueRule `json:"catalog_parameters,omitempty"`
	Source            bool        `json:"source,omitempty"`
	OpAmp             bool        `json:"op_amp,omitempty"`
	Nonlinear         bool        `json:"nonlinear,omitempty"`
	Transient         bool        `json:"transient,omitempty"`
}

var primitiveRegistry = []primitiveDefinition{
	{ID: PrimitiveResistorV1, Family: "resistor", Terminals: []string{"A", "B"}, RequiresValueSI: true},
	{ID: PrimitiveCapacitorV1, Family: "capacitor", Terminals: []string{"A", "B"}, RequiresValueSI: true},
	{ID: PrimitiveCapacitorTransientV1, Family: "capacitor", Terminals: []string{"A", "B"}, RequiresValueSI: true, Transient: true,
		CatalogParameters: []valueRule{{Name: "max_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6}}},
	{ID: PrimitiveVoltageSourceV1, Family: "voltage_source", Terminals: []string{"POSITIVE", "NEGATIVE"}, Source: true},
	{ID: PrimitiveConnectorVoltageSourceV1, Family: "connector", Terminals: []string{"PIN_1", "PIN_2"}, Source: true},
	{ID: PrimitiveCurrentSourceV1, Family: "current_source", Terminals: []string{"POSITIVE", "NEGATIVE"}, Source: true},
	{
		ID: PrimitiveOpAmpV1, Family: "opamp", Terminals: []string{"IN_PLUS", "IN_MINUS", "OUT", "V_PLUS", "V_MINUS"}, OpAmp: true,
		CatalogParameters: []valueRule{
			{Name: "dc_open_loop_gain", Positive: true, Maximum: 1e9},
			{Name: "gain_bandwidth_hz", Positive: true, Maximum: 1e12},
			{Name: "supply_min_v", Positive: true, Maximum: 1000},
			{Name: "supply_max_v", Positive: true, Maximum: 1000},
			{Name: "output_low_margin_v", Nonnegative: true, Maximum: 100},
			{Name: "output_high_margin_v", Nonnegative: true, Maximum: 100},
		},
	},
	{
		ID: PrimitiveDiodeShockleyV1, Family: "diode", Terminals: []string{"ANODE", "CATHODE"}, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "saturation_current_a", Positive: true, Minimum: 1e-30, Maximum: 1e-3},
			{Name: "emission_coefficient", Positive: true, Minimum: .5, Maximum: 10},
			{Name: "junction_temperature_k", Positive: true, Minimum: 200, Maximum: 1000},
			{Name: "max_forward_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_reverse_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
		},
	},
	// NPN and PNP are distinct primitive equations under the catalog's
	// shared bjt family; polarity is selected by the trusted primitive ID.
	{ID: PrimitiveBJTNPNV1, Family: "bjt", Terminals: []string{"BASE", "COLLECTOR", "EMITTER"}, Nonlinear: true, CatalogParameters: bjtParameterRules()},
	{ID: PrimitiveBJTPNPV1, Family: "bjt", Terminals: []string{"BASE", "COLLECTOR", "EMITTER"}, Nonlinear: true, CatalogParameters: bjtParameterRules()},
}

func bjtParameterRules() []valueRule {
	return []valueRule{
		{Name: "saturation_current_a", Positive: true, Minimum: 1e-30, Maximum: 1e-3},
		{Name: "forward_beta", Positive: true, Minimum: 1, Maximum: 1e6},
		{Name: "reverse_beta", Positive: true, Minimum: .01, Maximum: 1e6},
		{Name: "emission_coefficient", Positive: true, Minimum: .5, Maximum: 10},
		{Name: "junction_temperature_k", Positive: true, Minimum: 200, Maximum: 1000},
		{Name: "max_collector_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
		{Name: "max_collector_emitter_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
	}
}

type NodeEvidence struct {
	Name          string
	Role          string
	VoltageDomain string
}

func primitiveByID(id string) (primitiveDefinition, bool) {
	for _, primitive := range primitiveRegistry {
		if primitive.ID == id {
			return primitive, true
		}
	}
	return primitiveDefinition{}, false
}

func primitiveFamilyCompatible(primitiveFamily, componentFamily string) bool {
	return primitiveFamily == componentFamily || (primitiveFamily == "diode" && componentFamily == "led")
}

func componentHasSourceClaim(component ComponentEvidence) bool {
	for _, claim := range component.ModelClaims {
		primitive, exists := primitiveByID(strings.TrimSpace(claim.ModelID))
		if exists && primitive.Source && primitiveFamilyCompatible(primitive.Family, component.Family) {
			return true
		}
	}
	return false
}

type primitiveClaim struct {
	primitive primitiveDefinition
	claim     CatalogEvidence
}

func compatiblePrimitiveClaims(component ComponentEvidence, model definition) []primitiveClaim {
	var matches []primitiveClaim
	for _, claim := range component.ModelClaims {
		primitive, exists := primitiveByID(strings.TrimSpace(claim.ModelID))
		if !exists || !primitiveFamilyCompatible(primitive.Family, component.Family) || (primitive.Nonlinear && !model.NonlinearDC) || (primitive.Transient && !model.Transient) || (model.Transient && primitive.Family == "capacitor" && !primitive.Transient) {
			continue
		}
		matches = append(matches, primitiveClaim{primitive: primitive, claim: claim})
	}
	return matches
}

// ApplicableGraphModel returns a graph workflow only when every connected
// non-boundary component has exactly one compatible trusted primitive. This
// keeps synthesis fail-closed: an incomplete catalog model never produces a
// partial or optimistic simulation.
func ApplicableGraphModel(components []ComponentEvidence) (string, bool, string) {
	return applicableGraphModel(components, "")
}

// ApplicableGraphModelForAnalysis selects a registered graph workflow for an
// explicitly requested trusted analysis kind. It does not accept provider
// model IDs and still requires every connected component to have exactly one
// compatible reviewed primitive.
func ApplicableGraphModelForAnalysis(components []ComponentEvidence, analysisKind string) (string, bool, string) {
	if analysisKind != AnalysisTransient {
		return "", false, "unsupported_graph_analysis"
	}
	return applicableGraphModel(components, ModelTransientCircuitV1)
}

func applicableGraphModel(components []ComponentEvidence, requestedModelID string) (string, bool, string) {
	hasSource := false
	hasDevice := false
	hasNonlinear := false
	for _, component := range components {
		if len(component.Connections) == 0 || (mnaBoundaryFamily(component.Family) && !componentHasSourceClaim(component)) {
			continue
		}
		for _, claim := range component.ModelClaims {
			primitive, exists := primitiveByID(strings.TrimSpace(claim.ModelID))
			if !exists || !primitiveFamilyCompatible(primitive.Family, component.Family) {
				continue
			}
			hasSource = hasSource || primitive.Source
			hasDevice = hasDevice || !primitive.Source
			hasNonlinear = hasNonlinear || primitive.Nonlinear
		}
	}
	if !hasSource || !hasDevice {
		return "", false, "missing_trusted_source_or_device"
	}
	modelID := requestedModelID
	if modelID == "" {
		modelID = ModelLinearCircuitMNAV1
		if hasNonlinear {
			modelID = ModelNonlinearCircuitDCV1
		}
	}
	model, exists := definitionByID(modelID)
	if !exists {
		return "", false, "registered_graph_model_missing"
	}
	for _, component := range components {
		if len(component.Connections) == 0 || (mnaBoundaryFamily(component.Family) && !componentHasSourceClaim(component)) {
			continue
		}
		// compatiblePrimitiveClaims admits trusted primitive IDs only; legacy
		// component/workflow claims never participate in this uniqueness rule.
		if len(compatiblePrimitiveClaims(component, model)) != 1 {
			return "", false, "component_" + component.InstanceID + "_has_no_unique_compatible_primitive"
		}
	}
	return modelID, true, "complete_registered_graph_model"
}

func validateMNAIntent(intent Intent, components map[string]string) []Diagnostic {
	var diagnostics []Diagnostic
	model, _ := definitionByID(strings.TrimSpace(intent.ModelID))
	if len(intent.Bindings) != 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "bindings", Message: "graph MNA derives devices from resolved connectivity and does not accept topology bindings"})
	}
	if len(intent.Inputs) != 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "inputs", Message: "graph MNA accepts operating conditions only inside trusted analyses"})
	}
	if len(intent.Analyses) == 0 || len(intent.Analyses) > maxMNAAnalyses {
		diagnostics = append(diagnostics, Diagnostic{Path: "analyses", Message: fmt.Sprintf("graph MNA requires 1..%d bounded analyses", maxMNAAnalyses)})
	}
	if model.Transient && len(intent.Analyses) != 1 {
		diagnostics = append(diagnostics, Diagnostic{Path: "analyses", Message: "transient v1 requires exactly one bounded analysis grid"})
	}
	analysisKinds := make(map[string]string, len(intent.Analyses))
	for index, analysis := range intent.Analyses {
		path := fmt.Sprintf("analyses[%d]", index)
		id := strings.TrimSpace(analysis.ID)
		if !validAnalysisID(id) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".id", Message: "analysis id must contain only lowercase letters, digits, and underscores and start with a letter"})
		} else if _, duplicate := analysisKinds[id]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".id", Message: "analysis id is duplicated"})
		}
		analysisKinds[id] = analysis.Kind
		switch analysis.Kind {
		case AnalysisDCOperatingPoint:
			if model.Transient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "transient circuit workflow supports transient analysis only"})
			}
			if analysis.StartFrequencyHz != 0 || analysis.StopFrequencyHz != 0 || analysis.Points != 0 || analysis.DurationS != 0 || analysis.TimeStepS != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "DC operating-point analysis cannot contain AC sweep fields"})
			}
		case AnalysisACSweep:
			if model.NonlinearDC {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "nonlinear circuit analysis supports DC operating points only", Suggestion: "use dc_operating_point or select the linear MNA workflow for AC analysis"})
			}
			if !finite(analysis.StartFrequencyHz) || !finite(analysis.StopFrequencyHz) || analysis.StartFrequencyHz <= 0 || analysis.StopFrequencyHz < analysis.StartFrequencyHz || analysis.StopFrequencyHz > 1e12 || analysis.Points < 2 || analysis.Points > maxMNASweepPoints {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("AC sweep requires finite 0 < start <= stop <= 1e12 Hz and 2..%d points", maxMNASweepPoints)})
			}
			if analysis.DurationS != 0 || analysis.TimeStepS != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "AC sweep cannot contain transient grid fields"})
			}
		case AnalysisTransient:
			if !model.Transient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "transient analysis requires transient_circuit_v1"})
			}
			if analysis.StartFrequencyHz != 0 || analysis.StopFrequencyHz != 0 || analysis.Points != 0 || !validTransientGrid(analysis.DurationS, analysis.TimeStepS) || transientWork(analysis) > maxTransientWork {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("transient analysis requires finite %.0e <= time_step_s, duration_s <= %d, an exact integer grid, and at most %d steps", minTransientTimeStepS, maxTransientDurationS, maxTransientSteps)})
			}
		default:
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "analysis kind is not supported by graph MNA", Suggestion: "use dc_operating_point, ac_sweep, or transient in its dedicated workflow"})
		}
		if len(analysis.Excitations) == 0 || len(analysis.Excitations) > maxMNAExcitations {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".excitations", Message: fmt.Sprintf("analysis requires 1..%d catalog source excitations", maxMNAExcitations)})
		}
		seenSources := map[string]struct{}{}
		for sourceIndex, excitation := range analysis.Excitations {
			sourcePath := fmt.Sprintf("%s.excitations[%d]", path, sourceIndex)
			component := strings.TrimSpace(excitation.Component)
			_, exists := components[component]
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath + ".component", Message: "source component is not declared in the circuit graph"})
			}
			if _, duplicate := seenSources[component]; duplicate {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath + ".component", Message: "source component is duplicated within the analysis"})
			}
			seenSources[component] = struct{}{}
			if !boundedMagnitude(excitation.DCValue) || !boundedMagnitude(excitation.ACMagnitude) || excitation.ACMagnitude < 0 || !finite(excitation.ACPhaseDeg) || excitation.ACPhaseDeg < -360 || excitation.ACPhaseDeg > 360 || !boundedMagnitude(excitation.PulseInitialValue) || !boundedMagnitude(excitation.PulseValue) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "source conditions must be finite and bounded; AC magnitude must be nonnegative and phase within -360..360 degrees"})
			}
			if analysis.Kind == AnalysisDCOperatingPoint && (excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "DC operating-point excitation cannot contain AC magnitude or phase"})
			}
			if analysis.Kind != AnalysisTransient && hasPulse(excitation) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "pulse conditions are accepted only by transient analysis"})
			}
			if analysis.Kind == AnalysisTransient {
				if excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0 {
					diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "transient excitation cannot contain AC magnitude or phase"})
				}
				if excitation.PulsePeriodS != 0 {
					// PulseInitialValue and PulseValue are absolute source levels,
					// not offsets from DCValue. Requiring zero DCValue keeps one
					// canonical representation and prevents an ambiguous double bias.
					if excitation.DCValue != 0 || !finite(excitation.PulseDelayS) || !finite(excitation.PulseWidthS) || !finite(excitation.PulsePeriodS) || excitation.PulseDelayS < 0 || excitation.PulseWidthS <= 0 || excitation.PulsePeriodS <= excitation.PulseWidthS || excitation.PulseDelayS+excitation.PulseWidthS > analysis.DurationS || !onTransientGrid(excitation.PulseDelayS, analysis.TimeStepS) || !onTransientGrid(excitation.PulseWidthS, analysis.TimeStepS) || !onTransientGrid(excitation.PulsePeriodS, analysis.TimeStepS) {
						diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "transient pulse uses absolute initial/pulsed levels and requires zero dc_value, 0 <= delay, 0 < width < period, a falling edge within duration, and all times exactly on the observation grid"})
					}
				} else if excitation.PulseDelayS != 0 || excitation.PulseWidthS != 0 || excitation.PulseInitialValue != 0 || excitation.PulseValue != 0 {
					diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "transient pulse fields require a positive pulse_period_s"})
				}
			}
		}
	}
	if len(intent.Assertions) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "assertions", Message: "graph MNA requires at least one structured node assertion"})
	}
	seenAssertions := map[string]struct{}{}
	for index, assertion := range intent.Assertions {
		path := fmt.Sprintf("assertions[%d]", index)
		if assertion.Metric != "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".metric", Message: "graph MNA assertions are structured by analysis, node, quantity, and optional frequency"})
		}
		kind, exists := analysisKinds[assertion.AnalysisID]
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".analysis_id", Message: "assertion references an unknown analysis"})
		}
		if strings.TrimSpace(assertion.Node) == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".node", Message: "assertion node is required"})
		}
		switch assertion.Quantity {
		case QuantityVoltageV:
			if kind == AnalysisACSweep {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "AC assertions must use magnitude, phase, or dBV"})
			}
		case QuantityVoltageMagnitudeV, QuantityVoltagePhaseDeg, QuantityVoltageDBV:
			if kind == AnalysisDCOperatingPoint {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "DC assertions must use voltage_v"})
			}
		case QuantityRiseTimeS, QuantityFallTimeS:
			if kind != AnalysisTransient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "edge-time assertions require transient analysis"})
			}
		default:
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "assertion quantity is not supported"})
		}
		if kind == AnalysisACSweep && (!finite(assertion.FrequencyHz) || assertion.FrequencyHz <= 0) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "AC assertion requires a finite positive sweep frequency"})
		}
		if kind == AnalysisDCOperatingPoint && assertion.FrequencyHz != 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "DC assertion cannot specify a frequency"})
		}
		if kind == AnalysisTransient {
			if assertion.FrequencyHz != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "transient assertion cannot specify a frequency"})
			}
			analysis, _ := analysisByID(intent.Analyses, assertion.AnalysisID)
			if assertion.Quantity == QuantityVoltageV && (!finite(assertion.TimeS) || assertion.TimeS < 0 || assertion.TimeS > analysis.DurationS || !onTransientGrid(assertion.TimeS, analysis.TimeStepS)) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".time_s", Message: "transient voltage assertion time must be an exact observation point"})
			}
			if assertion.Quantity != QuantityVoltageV && assertion.TimeS != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".time_s", Message: "edge-time assertion derives its interval and cannot specify time_s"})
			}
		} else if assertion.TimeS != 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".time_s", Message: "non-transient assertion cannot specify time_s"})
		}
		if !finite(assertion.Min) || !finite(assertion.Max) || assertion.Min > assertion.Max {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "assertion bounds must be finite and minimum must not exceed maximum"})
		}
		key := assertionKey(assertion)
		if _, duplicate := seenAssertions[key]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "structured assertion is duplicated"})
		}
		seenAssertions[key] = struct{}{}
	}
	return diagnostics
}

func ResolveWithTopology(intent Intent, catalogID, catalogHash string, components []ComponentEvidence, nodes []NodeEvidence) (Plan, []Diagnostic) {
	intent.ModelID = strings.TrimSpace(intent.ModelID)
	model, ok := definitionByID(intent.ModelID)
	if ok && model.GraphMNA {
		return resolveMNA(intent, catalogID, catalogHash, components, nodes)
	}
	return Resolve(intent, catalogID, catalogHash, components)
}

func resolveMNA(intent Intent, catalogID, catalogHash string, components []ComponentEvidence, nodes ...[]NodeEvidence) (Plan, []Diagnostic) {
	families := make(map[string]string, len(components))
	for _, component := range components {
		families[component.InstanceID] = component.Family
	}
	if diagnostics := validateMNAIntent(intent, families); len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	var nodeEvidence []NodeEvidence
	if len(nodes) != 0 {
		nodeEvidence = nodes[0]
	}
	if len(nodeEvidence) == 0 {
		return Plan{}, []Diagnostic{{Path: "topology.nodes", Message: "graph MNA resolution requires resolved circuit net evidence"}}
	}
	ground, nodeNames, diagnostics := canonicalNodes(nodeEvidence)
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	referencedSources := map[string]struct{}{}
	for _, analysis := range intent.Analyses {
		for _, excitation := range analysis.Excitations {
			referencedSources[excitation.Component] = struct{}{}
		}
	}
	sortedComponents := append([]ComponentEvidence(nil), components...)
	slices.SortStableFunc(sortedComponents, func(a, b ComponentEvidence) int { return strings.Compare(a.InstanceID, b.InstanceID) })
	devices := make([]ResolvedDevice, 0, len(sortedComponents))
	uncertainties := []Uncertainty{}
	model, _ := definitionByID(intent.ModelID)
	for _, component := range sortedComponents {
		matches := compatiblePrimitiveClaims(component, model)
		_, sourceReferenced := referencedSources[component.InstanceID]
		if len(matches) == 0 {
			if sourceReferenced {
				diagnostics = append(diagnostics, Diagnostic{Path: "analyses.excitations." + component.InstanceID, Message: "source component family has no trusted MNA primitive"})
			} else if len(component.Connections) != 0 && !mnaBoundaryFamily(component.Family) {
				kind := "linear MNA"
				if model.NonlinearDC {
					kind = "nonlinear DC"
				}
				if model.Transient {
					kind = "transient"
				}
				diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID, Message: fmt.Sprintf("connected component family %s has no trusted %s primitive claim", component.Family, kind), Suggestion: "select a component with a unique reviewed catalog primitive claim"})
			}
			continue
		}
		if len(matches) != 1 {
			diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID, Message: "catalog component declares ambiguous trusted primitive claims", Suggestion: "retain exactly one reviewed primitive claim compatible with this workflow"})
			continue
		}
		primitive, claim := matches[0].primitive, matches[0].claim
		if sourceReferenced && !primitive.Source {
			diagnostics = append(diagnostics, Diagnostic{Path: "analyses.excitations." + component.InstanceID, Message: fmt.Sprintf("catalog component family %s is not a trusted independent source", component.Family)})
			continue
		}
		if primitive.Source && !sourceReferenced {
			continue
		}
		if parameterDiagnostics := validatePrimitiveParameters("topology.devices."+component.InstanceID+".model_parameters", primitive, claim.Parameters); len(parameterDiagnostics) != 0 {
			diagnostics = append(diagnostics, parameterDiagnostics...)
			continue
		}
		device := ResolvedDevice{
			Component: component.InstanceID, PhysicalComponent: component.PhysicalComponent,
			CatalogID: component.CatalogID, Family: component.Family, PrimitiveModel: primitive.ID,
			ModelParameters: normalizeNamedValues(claim.Parameters),
		}
		if primitive.RequiresValueSI {
			if !component.HasValueSI || !finite(component.ValueSI) || component.ValueSI <= 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID + ".value_si", Message: "trusted primitive requires a finite positive catalog-validated component value"})
				continue
			}
			value := component.ValueSI
			device.ValueSI = &value
		}
		for _, terminal := range primitive.Terminals {
			net, terminalDiagnostics := connectedNet(component, terminal)
			if len(terminalDiagnostics) != 0 {
				diagnostics = append(diagnostics, terminalDiagnostics...)
				continue
			}
			device.Terminals = append(device.Terminals, TerminalBinding{Terminal: terminal, Net: net})
		}
		if len(device.Terminals) == len(primitive.Terminals) {
			devices = append(devices, device)
			for _, uncertainty := range component.Uncertainties {
				if !intent.WorstCase {
					break
				}
				if uncertainty.Target == "excitation_dc_value" {
					matched := false
					for _, analysis := range intent.Analyses {
						for _, excitation := range analysis.Excitations {
							if excitation.Component != component.InstanceID {
								continue
							}
							if !sameUncertaintyValue(excitation.DCValue, uncertainty.Nominal) {
								diagnostics = append(diagnostics, Diagnostic{Path: "analyses." + analysis.ID + ".excitations." + component.InstanceID, Message: "reviewed source uncertainty nominal does not match the bounded operating condition"})
								continue
							}
							bound := uncertainty
							bound.Target = "analyses." + analysis.ID + ".excitations." + component.InstanceID + ".dc_value"
							uncertainties = append(uncertainties, bound)
							matched = true
						}
					}
					if !matched {
						diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID + ".uncertainties", Message: "reviewed source uncertainty has no matching bounded DC excitation"})
					}
					continue
				}
				if uncertainty.Target != "value_si" && !strings.HasPrefix(uncertainty.Target, "model_parameters.") {
					diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID + ".uncertainties", Message: "catalog uncertainty target is incompatible with the trusted MNA primitive"})
					continue
				}
				uncertainty.Target = "devices." + component.InstanceID + "." + uncertainty.Target
				uncertainties = append(uncertainties, uncertainty)
			}
		}
	}
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	plan := Plan{
		RegistryVersion: RegistryVersion, RegistryHash: RegistryHash(), CatalogID: catalogID, CatalogHash: catalogHash,
		ModelID: intent.ModelID, GroundNode: ground, Nodes: nodeNames, Devices: devices,
		Analyses: canonicalAnalyses(intent.Analyses), Assertions: append([]Assertion(nil), intent.Assertions...), WorstCase: intent.WorstCase,
		Uncertainties: append([]Uncertainty(nil), uncertainties...),
	}
	slices.SortStableFunc(plan.Uncertainties, func(a, b Uncertainty) int { return strings.Compare(a.Target, b.Target) })
	slices.SortStableFunc(plan.Assertions, func(a, b Assertion) int { return strings.Compare(assertionKey(a), assertionKey(b)) })
	plan.TopologyHash = topologyHash(plan.GroundNode, plan.Nodes, plan.Devices)
	if diagnostics := validateMNAPlan(plan); len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	return plan, nil
}

func mnaBoundaryFamily(family string) bool {
	switch family {
	case "connector", "testpoint":
		return true
	default:
		return false
	}
}

func validatePrimitiveParameters(path string, primitive primitiveDefinition, parameters []NamedValue) []Diagnostic {
	diagnostics := validateNamedValues(path, parameters, primitive.CatalogParameters)
	if len(diagnostics) != 0 || !primitive.OpAmp {
		return diagnostics
	}
	values := namedValueMap(parameters)
	minimum := values["supply_min_v"]
	maximum := values["supply_max_v"]
	lowMargin := values["output_low_margin_v"]
	highMargin := values["output_high_margin_v"]
	if maximum <= minimum {
		diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "op-amp supply_max_v must exceed supply_min_v"})
	}
	if lowMargin+highMargin >= minimum {
		diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "op-amp output margins leave no valid output range at supply_min_v"})
	}
	return diagnostics
}

func validateMNAPlan(plan Plan) []Diagnostic {
	if strings.TrimSpace(plan.CatalogID) == "" || strings.TrimSpace(plan.CatalogHash) == "" {
		return []Diagnostic{{Path: "catalog", Message: "resolved MNA plan is missing immutable catalog identity evidence"}}
	}
	if len(plan.Bindings) != 0 || len(plan.Inputs) != 0 {
		return []Diagnostic{{Path: "topology", Message: "resolved MNA plan contains legacy topology bindings or inputs"}}
	}
	var diagnostics []Diagnostic
	model, modelExists := definitionByID(plan.ModelID)
	if !modelExists || !model.GraphMNA {
		diagnostics = append(diagnostics, Diagnostic{Path: "model_id", Message: "resolved MNA plan references an unsupported workflow model"})
	}
	if plan.GroundNode == "" || !slices.Contains(plan.Nodes, plan.GroundNode) {
		diagnostics = append(diagnostics, Diagnostic{Path: "ground_node", Message: "resolved MNA plan is missing its reference node"})
	}
	for index, node := range plan.Nodes {
		if strings.TrimSpace(node) == "" || (index > 0 && plan.Nodes[index-1] >= node) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("nodes[%d]", index), Message: "resolved nodes must be nonempty, unique, and canonically ordered"})
		}
	}
	deviceFamilies := make(map[string]string, len(plan.Devices))
	devicePrimitives := make(map[string]string, len(plan.Devices))
	nonlinearDevices := 0
	for index, device := range plan.Devices {
		path := fmt.Sprintf("devices[%d]", index)
		if index > 0 && plan.Devices[index-1].Component >= device.Component {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "resolved devices must be unique and canonically ordered"})
		}
		primitive, exists := primitiveByID(device.PrimitiveModel)
		if !exists || device.Component == "" || device.CatalogID == "" || !primitiveFamilyCompatible(primitive.Family, device.Family) {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "resolved device is missing compatible primitive/catalog evidence"})
			continue
		}
		deviceFamilies[device.Component] = device.Family
		devicePrimitives[device.Component] = primitive.ID
		if primitive.Nonlinear {
			nonlinearDevices++
		}
		if primitive.Nonlinear && !model.NonlinearDC {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".primitive_model", Message: "linear MNA plan contains a nonlinear primitive"})
		}
		if primitive.Transient && !model.Transient {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".primitive_model", Message: "non-transient plan contains a transient-only primitive"})
		}
		if model.Transient && primitive.Family == "capacitor" && !primitive.Transient {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".primitive_model", Message: "transient plan capacitor requires a reviewed transient capacitor primitive"})
		}
		if primitive.RequiresValueSI && (device.ValueSI == nil || !finite(*device.ValueSI) || *device.ValueSI <= 0) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".value_si", Message: "resolved primitive requires a finite positive value"})
		}
		diagnostics = append(diagnostics, validatePrimitiveParameters(path+".model_parameters", primitive, device.ModelParameters)...)
		if len(device.Terminals) != len(primitive.Terminals) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".terminals", Message: "resolved primitive has an incomplete terminal set"})
			continue
		}
		for terminalIndex, terminal := range device.Terminals {
			if terminal.Terminal != primitive.Terminals[terminalIndex] || !slices.Contains(plan.Nodes, terminal.Net) {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.terminals[%d]", path, terminalIndex), Message: "resolved terminal is not canonical or references an unknown node"})
			}
		}
	}
	if model.NonlinearDC && nonlinearDevices == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "devices", Message: "nonlinear DC workflow requires at least one reviewed nonlinear device"})
	}
	intent := Intent{ModelID: plan.ModelID, Analyses: cloneAnalyses(plan.Analyses), Assertions: append([]Assertion(nil), plan.Assertions...)}
	diagnostics = append(diagnostics, validateMNAIntent(intent, deviceFamilies)...)
	for analysisIndex, analysis := range plan.Analyses {
		if analysisIndex > 0 && plan.Analyses[analysisIndex-1].ID >= analysis.ID {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("analyses[%d].id", analysisIndex), Message: "resolved analyses must be unique and canonically ordered"})
		}
		for sourceIndex, excitation := range analysis.Excitations {
			if sourceIndex > 0 && analysis.Excitations[sourceIndex-1].Component >= excitation.Component {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("analyses[%d].excitations[%d]", analysisIndex, sourceIndex), Message: "resolved excitations must be unique and canonically ordered"})
			}
			primitive, exists := primitiveByID(devicePrimitives[excitation.Component])
			if !exists || !primitive.Source {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("analyses[%d].excitations[%d].component", analysisIndex, sourceIndex), Message: "resolved excitation does not reference a trusted source primitive"})
			}
		}
	}
	for index, assertion := range plan.Assertions {
		if !slices.Contains(plan.Nodes, assertion.Node) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].node", index), Message: "assertion references a node absent from resolved topology"})
		}
		analysis, exists := analysisByID(plan.Analyses, assertion.AnalysisID)
		if exists && analysis.Kind == AnalysisACSweep && !frequencyInSweep(analysis, assertion.FrequencyHz) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].frequency_hz", index), Message: "assertion frequency is not an exact point in the deterministic AC sweep", Suggestion: "choose one of the frequencies generated by start, stop, and point count"})
		}
		if exists && analysis.Kind == AnalysisTransient && assertion.Quantity == QuantityVoltageV && !onTransientGrid(assertion.TimeS, analysis.TimeStepS) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].time_s", index), Message: "assertion time is not an exact point in the deterministic transient grid"})
		}
		if index > 0 && assertionKey(plan.Assertions[index-1]) >= assertionKey(assertion) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d]", index), Message: "resolved assertions must be unique and canonically ordered"})
		}
	}
	if expected := topologyHash(plan.GroundNode, plan.Nodes, plan.Devices); plan.TopologyHash == "" || plan.TopologyHash != expected {
		diagnostics = append(diagnostics, Diagnostic{Path: "topology_hash", Message: "resolved topology hash does not match canonical nodes and devices", Suggestion: "resolve the circuit again"})
	}
	return diagnostics
}

func canonicalNodes(nodes []NodeEvidence) (string, []string, []Diagnostic) {
	var diagnostics []Diagnostic
	names := make([]string, 0, len(nodes))
	groundCandidates := map[string]struct{}{}
	seen := map[string]struct{}{}
	for index, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("topology.nodes[%d]", index), Message: "resolved node name is empty"})
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("topology.nodes[%d]", index), Message: "resolved node name is duplicated"})
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
		if strings.EqualFold(strings.TrimSpace(node.Role), "ground") || strings.EqualFold(strings.TrimSpace(node.VoltageDomain), "0V") {
			groundCandidates[name] = struct{}{}
		}
	}
	slices.Sort(names)
	grounds := make([]string, 0, len(groundCandidates))
	for name := range groundCandidates {
		grounds = append(grounds, name)
	}
	slices.Sort(grounds)
	if len(grounds) != 1 {
		diagnostics = append(diagnostics, Diagnostic{Path: "topology.ground", Message: fmt.Sprintf("graph MNA requires exactly one resolved ground/0V node, got %d", len(grounds)), Suggestion: "mark one circuit net as ground with a 0V domain"})
		return "", names, diagnostics
	}
	return grounds[0], names, diagnostics
}

func connectedNet(component ComponentEvidence, terminal string) (string, []Diagnostic) {
	nets := map[string]struct{}{}
	for _, connection := range component.Connections {
		if strings.EqualFold(strings.TrimSpace(connection.Function), terminal) {
			nets[strings.TrimSpace(connection.Net)] = struct{}{}
		}
	}
	if len(nets) != 1 {
		return "", []Diagnostic{{Path: "topology.devices." + component.InstanceID + ".terminals." + terminal, Message: fmt.Sprintf("trusted primitive terminal must resolve to exactly one net, got %d", len(nets)), Suggestion: "connect every simulated terminal once through the resolved circuit graph"}}
	}
	for net := range nets {
		return net, nil
	}
	panic("unreachable")
}

func canonicalAnalyses(source []Analysis) []Analysis {
	analyses := cloneAnalyses(source)
	for index := range analyses {
		analyses[index].ID = strings.TrimSpace(analyses[index].ID)
		slices.SortStableFunc(analyses[index].Excitations, func(a, b SourceExcitation) int { return strings.Compare(a.Component, b.Component) })
	}
	slices.SortStableFunc(analyses, func(a, b Analysis) int { return strings.Compare(a.ID, b.ID) })
	return analyses
}

func topologyHash(ground string, nodes []string, devices []ResolvedDevice) string {
	payload := struct {
		Ground  string           `json:"ground"`
		Nodes   []string         `json:"nodes"`
		Devices []ResolvedDevice `json:"devices"`
	}{Ground: ground, Nodes: nodes, Devices: devices}
	data, err := json.Marshal(payload)
	if err != nil {
		panic("MNA topology is not serializable: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cloneDevices(source []ResolvedDevice) []ResolvedDevice {
	clone := append([]ResolvedDevice(nil), source...)
	for index := range clone {
		clone[index].ModelParameters = append([]NamedValue(nil), source[index].ModelParameters...)
		clone[index].Terminals = append([]TerminalBinding(nil), source[index].Terminals...)
		if source[index].ValueSI != nil {
			value := *source[index].ValueSI
			clone[index].ValueSI = &value
		}
	}
	return clone
}

func assertionKey(assertion Assertion) string {
	if assertion.Metric != "" {
		return "legacy\x00" + assertion.Metric
	}
	return fmt.Sprintf("mna\x00%s\x00%s\x00%s\x00%024.12e\x00%024.12e", assertion.AnalysisID, assertion.Node, assertion.Quantity, assertion.FrequencyHz, assertion.TimeS)
}

func analysisByID(analyses []Analysis, id string) (Analysis, bool) {
	for _, analysis := range analyses {
		if analysis.ID == id {
			return analysis, true
		}
	}
	return Analysis{}, false
}

func validAnalysisID(value string) bool {
	for index, r := range value {
		if index == 0 && !unicode.IsLower(r) {
			return false
		}
		if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return value != "" && len(value) <= 64
}

func boundedMagnitude(value float64) bool {
	return finite(value) && math.Abs(value) <= maxMNASourceMagnitude
}

func hasPulse(excitation SourceExcitation) bool {
	return excitation.PulseInitialValue != 0 || excitation.PulseValue != 0 || excitation.PulseDelayS != 0 || excitation.PulseWidthS != 0 || excitation.PulsePeriodS != 0
}

func validTransientGrid(duration, step float64) bool {
	if !finite(duration) || !finite(step) || step < minTransientTimeStepS || duration < step || duration > maxTransientDurationS {
		return false
	}
	steps := math.Round(duration / step)
	return steps >= 1 && steps <= maxTransientSteps && math.Abs(duration-steps*step) <= math.Max(duration, step)*1e-12
}

func transientWork(analysis Analysis) int {
	if !finite(analysis.DurationS) || !finite(analysis.TimeStepS) || analysis.TimeStepS <= 0 {
		return maxTransientWork + 1
	}
	return (int(math.Round(analysis.DurationS/analysis.TimeStepS)) + len(nonlinearContinuation)) * nonlinearMaxIterations
}

func onTransientGrid(value, step float64) bool {
	if !finite(value) || !finite(step) || step <= 0 || value < 0 {
		return false
	}
	index := math.Round(value / step)
	return math.Abs(value-index*step) <= math.Max(step, math.Abs(value))*1e-12
}

func frequencyInSweep(analysis Analysis, frequency float64) bool {
	for _, candidate := range sweepFrequencies(analysis) {
		if math.Abs(candidate-frequency) <= math.Max(1, math.Abs(candidate))*1e-12 {
			return true
		}
	}
	return false
}
