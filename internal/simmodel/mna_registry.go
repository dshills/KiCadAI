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
)

type primitiveDefinition struct {
	ID                string      `json:"id"`
	Family            string      `json:"family"`
	Terminals         []string    `json:"terminals"`
	RequiresValueSI   bool        `json:"requires_value_si,omitempty"`
	CatalogParameters []valueRule `json:"catalog_parameters,omitempty"`
	Source            bool        `json:"source,omitempty"`
	OpAmp             bool        `json:"op_amp,omitempty"`
}

var primitiveRegistry = []primitiveDefinition{
	{ID: PrimitiveResistorV1, Family: "resistor", Terminals: []string{"A", "B"}, RequiresValueSI: true},
	{ID: PrimitiveCapacitorV1, Family: "capacitor", Terminals: []string{"A", "B"}, RequiresValueSI: true},
	{ID: PrimitiveVoltageSourceV1, Family: "voltage_source", Terminals: []string{"POSITIVE", "NEGATIVE"}, Source: true},
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

func primitiveForFamily(family string) (primitiveDefinition, bool) {
	for _, primitive := range primitiveRegistry {
		if primitive.Family == family {
			return primitive, true
		}
	}
	return primitiveDefinition{}, false
}

func validateMNAIntent(intent Intent, components map[string]string) []Diagnostic {
	var diagnostics []Diagnostic
	if len(intent.Bindings) != 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "bindings", Message: "graph MNA derives devices from resolved connectivity and does not accept topology bindings"})
	}
	if len(intent.Inputs) != 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "inputs", Message: "graph MNA accepts operating conditions only inside trusted analyses"})
	}
	if len(intent.Analyses) == 0 || len(intent.Analyses) > maxMNAAnalyses {
		diagnostics = append(diagnostics, Diagnostic{Path: "analyses", Message: fmt.Sprintf("graph MNA requires 1..%d bounded analyses", maxMNAAnalyses)})
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
			if analysis.StartFrequencyHz != 0 || analysis.StopFrequencyHz != 0 || analysis.Points != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "DC operating-point analysis cannot contain AC sweep fields"})
			}
		case AnalysisACSweep:
			if !finite(analysis.StartFrequencyHz) || !finite(analysis.StopFrequencyHz) || analysis.StartFrequencyHz <= 0 || analysis.StopFrequencyHz < analysis.StartFrequencyHz || analysis.StopFrequencyHz > 1e12 || analysis.Points < 2 || analysis.Points > maxMNASweepPoints {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("AC sweep requires finite 0 < start <= stop <= 1e12 Hz and 2..%d points", maxMNASweepPoints)})
			}
		default:
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "analysis kind is not supported by graph MNA", Suggestion: "use dc_operating_point or ac_sweep"})
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
			if !boundedMagnitude(excitation.DCValue) || !boundedMagnitude(excitation.ACMagnitude) || excitation.ACMagnitude < 0 || !finite(excitation.ACPhaseDeg) || excitation.ACPhaseDeg < -360 || excitation.ACPhaseDeg > 360 {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "source conditions must be finite and bounded; AC magnitude must be nonnegative and phase within -360..360 degrees"})
			}
			if analysis.Kind == AnalysisDCOperatingPoint && (excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "DC operating-point excitation cannot contain AC magnitude or phase"})
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
		default:
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "assertion quantity is not supported"})
		}
		if kind == AnalysisACSweep && (!finite(assertion.FrequencyHz) || assertion.FrequencyHz <= 0) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "AC assertion requires a finite positive sweep frequency"})
		}
		if kind == AnalysisDCOperatingPoint && assertion.FrequencyHz != 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "DC assertion cannot specify a frequency"})
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
	for _, component := range sortedComponents {
		primitive, supported := primitiveForFamily(component.Family)
		_, sourceReferenced := referencedSources[component.InstanceID]
		if !supported {
			if sourceReferenced {
				diagnostics = append(diagnostics, Diagnostic{Path: "analyses.excitations." + component.InstanceID, Message: "source component family has no trusted MNA primitive"})
			} else if len(component.Connections) != 0 && !mnaBoundaryFamily(component.Family) {
				diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID, Message: fmt.Sprintf("connected component family %s has no trusted linear MNA primitive", component.Family), Suggestion: "remove the unsupported/nonlinear device or add reviewed catalog primitive evidence"})
			}
			continue
		}
		if sourceReferenced && !primitive.Source {
			diagnostics = append(diagnostics, Diagnostic{Path: "analyses.excitations." + component.InstanceID, Message: fmt.Sprintf("catalog component family %s is not a trusted independent source", component.Family)})
			continue
		}
		if primitive.Source && !sourceReferenced {
			continue
		}
		claim, claimed := claimByID(component.ModelClaims, primitive.ID)
		if !claimed {
			diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID, Message: fmt.Sprintf("catalog component %s does not declare trusted primitive %s", component.CatalogID, primitive.ID), Suggestion: "select a catalog component with verified MNA primitive evidence"})
			continue
		}
		if parameterDiagnostics := validatePrimitiveParameters("topology.devices."+component.InstanceID+".model_parameters", primitive, claim.Parameters); len(parameterDiagnostics) != 0 {
			diagnostics = append(diagnostics, parameterDiagnostics...)
			continue
		}
		device := ResolvedDevice{Component: component.InstanceID, CatalogID: component.CatalogID, Family: component.Family, PrimitiveModel: primitive.ID, ModelParameters: normalizeNamedValues(claim.Parameters)}
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
		}
	}
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	plan := Plan{
		RegistryVersion: RegistryVersion, RegistryHash: RegistryHash(), CatalogID: catalogID, CatalogHash: catalogHash,
		ModelID: ModelLinearCircuitMNAV1, GroundNode: ground, Nodes: nodeNames, Devices: devices,
		Analyses: canonicalAnalyses(intent.Analyses), Assertions: append([]Assertion(nil), intent.Assertions...),
	}
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
	for index, device := range plan.Devices {
		path := fmt.Sprintf("devices[%d]", index)
		if index > 0 && plan.Devices[index-1].Component >= device.Component {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "resolved devices must be unique and canonically ordered"})
		}
		primitive, exists := primitiveByID(device.PrimitiveModel)
		if !exists || device.Component == "" || device.CatalogID == "" || primitive.Family != device.Family {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "resolved device is missing compatible primitive/catalog evidence"})
			continue
		}
		deviceFamilies[device.Component] = device.Family
		devicePrimitives[device.Component] = primitive.ID
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
	intent := Intent{ModelID: ModelLinearCircuitMNAV1, Analyses: cloneAnalyses(plan.Analyses), Assertions: append([]Assertion(nil), plan.Assertions...)}
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
	return fmt.Sprintf("mna\x00%s\x00%s\x00%s\x00%024.12e", assertion.AnalysisID, assertion.Node, assertion.Quantity, assertion.FrequencyHz)
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

func frequencyInSweep(analysis Analysis, frequency float64) bool {
	for _, candidate := range sweepFrequencies(analysis) {
		if math.Abs(candidate-frequency) <= math.Max(1, math.Abs(candidate))*1e-12 {
			return true
		}
	}
	return false
}
