package simmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

type valueRule struct {
	Name        string  `json:"name"`
	Positive    bool    `json:"positive,omitempty"`
	Nonnegative bool    `json:"nonnegative,omitempty"`
	Minimum     float64 `json:"minimum,omitempty"`
	Maximum     float64 `json:"maximum,omitempty"`
}

type roleDefinition struct {
	Role              string      `json:"role"`
	Family            string      `json:"family"`
	RequiresValueSI   bool        `json:"requires_value_si,omitempty"`
	CatalogParameters []valueRule `json:"catalog_parameters,omitempty"`
}

type definition struct {
	ID          string           `json:"id"`
	Roles       []roleDefinition `json:"roles"`
	Inputs      []valueRule      `json:"inputs"`
	Metrics     []string         `json:"metrics"`
	Description string           `json:"description"`
	GraphMNA    bool             `json:"graph_mna,omitempty"`
	NonlinearDC bool             `json:"nonlinear_dc,omitempty"`
	Transient   bool             `json:"transient,omitempty"`
}

var registry = []definition{
	{
		ID: ModelLinearRegulatorIdealV1,
		Roles: []roleDefinition{{Role: "regulator", Family: "regulator", CatalogParameters: []valueRule{
			{Name: "output_voltage_v", Positive: true, Maximum: 100},
			{Name: "min_headroom_v", Nonnegative: true, Maximum: 20},
			{Name: "max_load_current_ma", Positive: true, Maximum: 100000},
		}}},
		Inputs:  []valueRule{{Name: "input_voltage_v", Positive: true, Maximum: 1000}, {Name: "load_current_ma", Nonnegative: true, Maximum: 100000}},
		Metrics: []string{"output_voltage_v"}, Description: "Catalog-parameterized ideal fixed linear regulator operating-point model.",
	},
	{
		ID:      ModelResistorDividerDCV1,
		Roles:   []roleDefinition{{Role: "upper_resistor", Family: "resistor", RequiresValueSI: true}, {Role: "lower_resistor", Family: "resistor", RequiresValueSI: true}},
		Inputs:  []valueRule{{Name: "input_voltage_v", Nonnegative: true, Maximum: 1000}},
		Metrics: []string{"output_voltage_v", "source_current_ma"}, Description: "Ideal unloaded two-resistor divider DC model.",
	},
	{
		ID:      ModelRCLowpassACV1,
		Roles:   []roleDefinition{{Role: "resistor", Family: "resistor", RequiresValueSI: true}, {Role: "capacitor", Family: "capacitor", RequiresValueSI: true}},
		Inputs:  []valueRule{{Name: "frequency_hz", Positive: true, Maximum: 1e12}},
		Metrics: []string{"cutoff_frequency_hz", "gain_ratio"}, Description: "Ideal first-order unloaded RC low-pass AC magnitude model.",
	},
	{
		ID: ModelLinearCircuitMNAV1, GraphMNA: true,
		Description: "Graph-derived deterministic modified nodal analysis using trusted catalog primitive models.",
	},
	{
		ID: ModelNonlinearCircuitDCV1, GraphMNA: true, NonlinearDC: true,
		Description: "Graph-derived bounded nonlinear DC operating-point analysis using trusted catalog primitive models.",
	},
	{
		ID: ModelTransientCircuitV1, GraphMNA: true, NonlinearDC: true, Transient: true,
		Description: "Graph-derived fixed-step transient analysis using trusted catalog primitive models.",
	},
}

func RegistryHash() string {
	data, err := json.Marshal(struct {
		Models     []definition          `json:"models"`
		Primitives []primitiveDefinition `json:"primitives"`
	}{Models: registry, Primitives: primitiveRegistry})
	if err != nil {
		panic("trusted simulation registry is not serializable: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ModelIDs() []string {
	ids := make([]string, 0, len(registry))
	for _, model := range registry {
		ids = append(ids, model.ID)
	}
	return ids
}

func ValidateCatalogEvidence(family string, evidence []CatalogEvidence) []Diagnostic {
	var diagnostics []Diagnostic
	seen := map[string]struct{}{}
	for index, claim := range evidence {
		path := fmt.Sprintf("simulation_models[%d]", index)
		modelID := strings.TrimSpace(claim.ModelID)
		model, workflowModel := definitionByID(modelID)
		primitive, primitiveModel := primitiveByID(modelID)
		if !workflowModel && !primitiveModel {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".model_id", Message: "simulation model is not present in the trusted registry", Suggestion: "use one of: " + strings.Join(ModelIDs(), ", ")})
			continue
		}
		if claim.ModelID != modelID {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".model_id", Message: "simulation model id must use canonical trimmed spelling " + modelID})
			continue
		}
		if _, duplicate := seen[modelID]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".model_id", Message: "duplicate simulation model evidence"})
			continue
		}
		seen[modelID] = struct{}{}
		if primitiveModel {
			if !primitiveFamilyCompatible(primitive.Family, family) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".model_id", Message: fmt.Sprintf("trusted primitive %s requires component family %s, got %s", primitive.ID, primitive.Family, family)})
				continue
			}
			diagnostics = append(diagnostics, validatePrimitiveParameters(path+".parameters", primitive, claim.Parameters)...)
			continue
		}
		if model.GraphMNA {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".model_id", Message: "graph analysis models cannot be attached to component catalog records", Suggestion: "attach a trusted primitive model instead"})
			continue
		}
		roles := rolesForFamily(model, family)
		if len(roles) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".model_id", Message: fmt.Sprintf("trusted model %s has no role compatible with component family %s", model.ID, family)})
			continue
		}
		var shortest []Diagnostic
		for _, role := range roles {
			candidate := validateNamedValues(path+".parameters", claim.Parameters, role.CatalogParameters)
			if len(candidate) == 0 {
				shortest = nil
				break
			}
			if shortest == nil || len(candidate) < len(shortest) {
				shortest = candidate
			}
		}
		diagnostics = append(diagnostics, shortest...)
	}
	return diagnostics
}

func ValidateIntent(intent Intent, components map[string]string) []Diagnostic {
	model, ok := definitionByID(strings.TrimSpace(intent.ModelID))
	if !ok {
		return []Diagnostic{{Path: "model_id", Message: "simulation model is not present in the trusted registry", Suggestion: "use one of: " + strings.Join(ModelIDs(), ", ")}}
	}
	if model.GraphMNA {
		return validateMNAIntent(intent, components)
	}
	var diagnostics []Diagnostic
	roles := map[string]roleDefinition{}
	for _, role := range model.Roles {
		roles[role.Role] = role
	}
	seenRoles := map[string]struct{}{}
	seenComponents := map[string]struct{}{}
	for index, binding := range intent.Bindings {
		path := fmt.Sprintf("bindings[%d]", index)
		role, known := roles[strings.TrimSpace(binding.Role)]
		if !known {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".role", Message: "binding role is not defined by trusted model " + model.ID})
			continue
		}
		if _, duplicate := seenRoles[role.Role]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".role", Message: "binding role is duplicated"})
		}
		seenRoles[role.Role] = struct{}{}
		component := strings.TrimSpace(binding.Component)
		family, exists := components[component]
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "binding component is not declared in the circuit graph"})
		} else if family != role.Family {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: fmt.Sprintf("binding role %s requires family %s, got %s", role.Role, role.Family, family)})
		}
		if _, duplicate := seenComponents[component]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "one component cannot satisfy multiple model roles"})
		}
		seenComponents[component] = struct{}{}
	}
	for _, role := range model.Roles {
		if _, exists := seenRoles[role.Role]; !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: "bindings", Message: "missing required binding role " + role.Role})
		}
	}
	diagnostics = append(diagnostics, validateNamedValues("inputs", intent.Inputs, model.Inputs)...)
	if len(intent.Assertions) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "assertions", Message: "at least one trusted metric assertion is required"})
	}
	metrics := map[string]struct{}{}
	for _, metric := range model.Metrics {
		metrics[metric] = struct{}{}
	}
	seenMetrics := map[string]struct{}{}
	for index, assertion := range intent.Assertions {
		path := fmt.Sprintf("assertions[%d]", index)
		if _, exists := metrics[assertion.Metric]; !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".metric", Message: "assertion metric is not emitted by trusted model " + model.ID})
		}
		if _, duplicate := seenMetrics[assertion.Metric]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".metric", Message: "assertion metric is duplicated"})
		}
		seenMetrics[assertion.Metric] = struct{}{}
		if !finite(assertion.Min) || !finite(assertion.Max) || assertion.Min > assertion.Max {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "assertion bounds must be finite and minimum must not exceed maximum"})
		}
	}
	return diagnostics
}

func Resolve(intent Intent, catalogID, catalogHash string, components []ComponentEvidence) (Plan, []Diagnostic) {
	intent.ModelID = strings.TrimSpace(intent.ModelID)
	families := make(map[string]string, len(components))
	byID := make(map[string]ComponentEvidence, len(components))
	for _, component := range components {
		families[component.InstanceID] = component.Family
		byID[component.InstanceID] = component
	}
	if diagnostics := ValidateIntent(intent, families); len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	model, _ := definitionByID(intent.ModelID)
	if model.GraphMNA {
		return resolveMNA(intent, catalogID, catalogHash, components)
	}
	plan := Plan{RegistryVersion: RegistryVersion, RegistryHash: RegistryHash(), CatalogID: catalogID, CatalogHash: catalogHash, ModelID: model.ID}
	var diagnostics []Diagnostic
	for _, binding := range intent.Bindings {
		component := byID[binding.Component]
		role, _ := roleByName(model, binding.Role)
		claim, supported := claimByID(component.ModelClaims, model.ID)
		if !supported {
			diagnostics = append(diagnostics, Diagnostic{Path: "bindings." + binding.Role, Message: fmt.Sprintf("catalog component %s does not declare trusted model %s compatibility", component.CatalogID, model.ID), Suggestion: "select a catalog component with verified simulation model evidence"})
			continue
		}
		if claimDiagnostics := validateNamedValues("bindings."+binding.Role+".model_parameters", claim.Parameters, role.CatalogParameters); len(claimDiagnostics) != 0 {
			diagnostics = append(diagnostics, claimDiagnostics...)
			continue
		}
		resolved := ResolvedBinding{Role: role.Role, Component: component.InstanceID, CatalogID: component.CatalogID, Family: component.Family, ModelParameters: normalizeNamedValues(claim.Parameters)}
		if role.RequiresValueSI {
			if !component.HasValueSI || !finite(component.ValueSI) || component.ValueSI <= 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: "bindings." + binding.Role, Message: "model role requires a finite positive catalog-validated component value", Suggestion: "provide a component value compatible with the selected catalog record"})
				continue
			}
			value := component.ValueSI
			resolved.ValueSI = &value
		}
		plan.Bindings = append(plan.Bindings, resolved)
	}
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	plan.Inputs = normalizeNamedValues(intent.Inputs)
	plan.Assertions = append([]Assertion(nil), intent.Assertions...)
	slices.SortStableFunc(plan.Bindings, func(a, b ResolvedBinding) int { return strings.Compare(a.Role, b.Role) })
	slices.SortStableFunc(plan.Assertions, func(a, b Assertion) int { return strings.Compare(assertionKey(a), assertionKey(b)) })
	return plan, nil
}

func Evaluate(plan Plan) (Report, []Diagnostic) {
	report := Report{Schema: ReportSchema, RegistryVersion: plan.RegistryVersion, RegistryHash: plan.RegistryHash, CatalogID: plan.CatalogID, CatalogHash: plan.CatalogHash, ModelID: plan.ModelID, Bindings: append([]ResolvedBinding(nil), plan.Bindings...), Inputs: append([]NamedValue(nil), plan.Inputs...), GroundNode: plan.GroundNode, Nodes: append([]string(nil), plan.Nodes...), Devices: cloneDevices(plan.Devices), TopologyHash: plan.TopologyHash, Status: "blocked"}
	if diagnostics := ValidatePlan(plan); len(diagnostics) != 0 {
		return report, diagnostics
	}
	model, _ := definitionByID(plan.ModelID)
	if model.GraphMNA {
		return evaluateMNA(plan, report)
	}
	measurements, diagnostics := evaluateModel(model, plan)
	if len(diagnostics) != 0 {
		return report, diagnostics
	}
	report.Measurements = measurements
	values := map[string]float64{}
	for _, measurement := range measurements {
		values[measurement.Metric] = measurement.Value
	}
	for _, assertion := range plan.Assertions {
		actual, exists := values[assertion.Metric]
		if !exists {
			return report, []Diagnostic{{Path: "assertions." + assertion.Metric, Message: "trusted model did not emit an asserted metric"}}
		}
		pass := actual >= assertion.Min && actual <= assertion.Max
		report.Assertions = append(report.Assertions, AssertionResult{Metric: assertion.Metric, Min: assertion.Min, Max: assertion.Max, Actual: actual, Pass: pass})
		if !pass {
			diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + assertion.Metric, Message: fmt.Sprintf("measured %.12g is outside trusted bounds %.12g..%.12g", actual, assertion.Min, assertion.Max), Suggestion: "adjust catalog-backed component values or operating conditions"})
		}
	}
	if len(diagnostics) == 0 {
		report.Status = "pass"
	}
	return report, diagnostics
}

func ValidatePlan(plan Plan) []Diagnostic {
	if plan.RegistryVersion != RegistryVersion || plan.RegistryHash != RegistryHash() {
		return []Diagnostic{{Path: "registry_hash", Message: "resolved simulation plan does not match the active trusted registry", Suggestion: "resolve the circuit again against the active registry"}}
	}
	model, ok := definitionByID(plan.ModelID)
	if !ok {
		return []Diagnostic{{Path: "model_id", Message: "resolved simulation plan references an unknown model"}}
	}
	if model.GraphMNA {
		return validateMNAPlan(plan)
	}
	if strings.TrimSpace(plan.CatalogID) == "" || strings.TrimSpace(plan.CatalogHash) == "" {
		return []Diagnostic{{Path: "catalog", Message: "resolved simulation plan is missing catalog identity evidence", Suggestion: "resolve the circuit against an immutable catalog snapshot"}}
	}
	var diagnostics []Diagnostic
	bindings := map[string]ResolvedBinding{}
	boundComponents := map[string]struct{}{}
	for index, binding := range plan.Bindings {
		path := fmt.Sprintf("bindings[%d]", index)
		role, exists := roleByName(model, binding.Role)
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".role", Message: "resolved binding role is not defined by the trusted model"})
			continue
		}
		if _, duplicate := bindings[role.Role]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".role", Message: "resolved binding role is duplicated"})
		}
		bindings[role.Role] = binding
		if _, duplicate := boundComponents[binding.Component]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "one resolved component cannot satisfy multiple model roles"})
		}
		boundComponents[binding.Component] = struct{}{}
		if binding.Component == "" || binding.CatalogID == "" || binding.Family != role.Family {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "resolved binding is missing compatible component/catalog evidence"})
		}
		if role.RequiresValueSI && (binding.ValueSI == nil || !finite(*binding.ValueSI) || *binding.ValueSI <= 0) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".value_si", Message: "resolved binding requires a finite positive component value"})
		}
		diagnostics = append(diagnostics, validateNamedValues(path+".model_parameters", binding.ModelParameters, role.CatalogParameters)...)
	}
	for _, role := range model.Roles {
		if _, exists := bindings[role.Role]; !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: "bindings", Message: "resolved plan is missing role " + role.Role})
		}
	}
	diagnostics = append(diagnostics, validateNamedValues("inputs", plan.Inputs, model.Inputs)...)
	metrics := map[string]struct{}{}
	for _, metric := range model.Metrics {
		metrics[metric] = struct{}{}
	}
	if len(plan.Assertions) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "assertions", Message: "resolved plan requires at least one assertion"})
	}
	assertedMetrics := map[string]struct{}{}
	for index, assertion := range plan.Assertions {
		if _, duplicate := assertedMetrics[assertion.Metric]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].metric", index), Message: "resolved assertion metric is duplicated"})
		}
		assertedMetrics[assertion.Metric] = struct{}{}
		if _, exists := metrics[assertion.Metric]; !exists || !finite(assertion.Min) || !finite(assertion.Max) || assertion.Min > assertion.Max {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d]", index), Message: "resolved assertion is not valid for the trusted model"})
		}
	}
	return diagnostics
}

func evaluateModel(model definition, plan Plan) ([]Measurement, []Diagnostic) {
	inputs := namedValueMap(plan.Inputs)
	bindings := map[string]ResolvedBinding{}
	for _, binding := range plan.Bindings {
		bindings[binding.Role] = binding
	}
	switch model.ID {
	case ModelLinearRegulatorIdealV1:
		parameters := namedValueMap(bindings["regulator"].ModelParameters)
		output := parameters["output_voltage_v"]
		if inputs["input_voltage_v"] < output+parameters["min_headroom_v"] {
			return nil, []Diagnostic{{Path: "inputs.input_voltage_v", Message: "input voltage does not satisfy catalog minimum headroom", Suggestion: "increase input voltage or select a compatible regulator"}}
		}
		if inputs["load_current_ma"] > parameters["max_load_current_ma"] {
			return nil, []Diagnostic{{Path: "inputs.load_current_ma", Message: "load current exceeds catalog-backed model limit", Suggestion: "reduce load or select a compatible regulator"}}
		}
		return []Measurement{{Metric: "output_voltage_v", Value: output}}, nil
	case ModelResistorDividerDCV1:
		upper := *bindings["upper_resistor"].ValueSI
		lower := *bindings["lower_resistor"].ValueSI
		input := inputs["input_voltage_v"]
		return []Measurement{{Metric: "output_voltage_v", Value: input * lower / (upper + lower)}, {Metric: "source_current_ma", Value: input / (upper + lower) * 1000}}, nil
	case ModelRCLowpassACV1:
		resistance := *bindings["resistor"].ValueSI
		capacitance := *bindings["capacitor"].ValueSI
		frequency := inputs["frequency_hz"]
		cutoff := 1 / (2 * math.Pi * resistance * capacitance)
		gain := 1 / math.Sqrt(1+math.Pow(frequency/cutoff, 2))
		return []Measurement{{Metric: "cutoff_frequency_hz", Value: cutoff}, {Metric: "gain_ratio", Value: gain}}, nil
	default:
		return nil, []Diagnostic{{Path: "model_id", Message: "trusted model has no evaluator"}}
	}
}

func validateNamedValues(path string, values []NamedValue, rules []valueRule) []Diagnostic {
	rulesByName := map[string]valueRule{}
	for _, rule := range rules {
		rulesByName[rule.Name] = rule
	}
	seen := map[string]struct{}{}
	var diagnostics []Diagnostic
	for index, value := range values {
		valuePath := fmt.Sprintf("%s[%d]", path, index)
		rule, exists := rulesByName[strings.TrimSpace(value.Name)]
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: valuePath + ".name", Message: "parameter is not defined by the trusted model"})
			continue
		}
		if _, duplicate := seen[rule.Name]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: valuePath + ".name", Message: "parameter is duplicated"})
		}
		seen[rule.Name] = struct{}{}
		if !finite(value.Value) || (rule.Positive && value.Value <= 0) || (rule.Nonnegative && value.Value < 0) || (rule.Minimum > 0 && value.Value < rule.Minimum) || (rule.Maximum > 0 && value.Value > rule.Maximum) {
			diagnostics = append(diagnostics, Diagnostic{Path: valuePath + ".value", Message: "parameter is outside the trusted finite range"})
		}
	}
	for _, rule := range rules {
		if _, exists := seen[rule.Name]; !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "missing required parameter " + rule.Name})
		}
	}
	return diagnostics
}

func normalizeNamedValues(values []NamedValue) []NamedValue {
	normalized := append([]NamedValue(nil), values...)
	for index := range normalized {
		normalized[index].Name = strings.TrimSpace(normalized[index].Name)
	}
	slices.SortStableFunc(normalized, func(a, b NamedValue) int { return strings.Compare(a.Name, b.Name) })
	return normalized
}

func definitionByID(id string) (definition, bool) {
	for _, model := range registry {
		if model.ID == id {
			return model, true
		}
	}
	return definition{}, false
}

func roleByName(model definition, name string) (roleDefinition, bool) {
	for _, role := range model.Roles {
		if role.Role == name {
			return role, true
		}
	}
	return roleDefinition{}, false
}

func rolesForFamily(model definition, family string) []roleDefinition {
	var roles []roleDefinition
	for _, role := range model.Roles {
		if role.Family == family {
			roles = append(roles, role)
		}
	}
	return roles
}

func claimByID(claims []CatalogEvidence, id string) (CatalogEvidence, bool) {
	for _, claim := range claims {
		if strings.TrimSpace(claim.ModelID) == id {
			return claim, true
		}
	}
	return CatalogEvidence{}, false
}

func namedValueMap(values []NamedValue) map[string]float64 {
	result := make(map[string]float64, len(values))
	for _, value := range values {
		result[value.Name] = value.Value
	}
	return result
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
