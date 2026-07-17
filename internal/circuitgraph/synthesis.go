package circuitgraph

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	SynthesisReportSchema              = "kicadai.function-synthesis-report.v1"
	synthesisEstimatedComponentPitchMM = 5.0
)

type SynthesisReport struct {
	Schema             string                        `json:"schema"`
	Status             string                        `json:"status"`
	PolicyVersion      string                        `json:"policy_version"`
	InputHash          string                        `json:"input_hash"`
	LoweredGraphHash   string                        `json:"lowered_graph_hash,omitempty"`
	Selections         []SynthesisSelection          `json:"selections"`
	InterfaceBindings  []SynthesisInterfaceBinding   `json:"interface_bindings"`
	DerivedConstraints []SynthesisConstraintEvidence `json:"derived_constraints"`
	UnusedPinDecisions []SynthesisUnusedPinDecision  `json:"unused_pin_decisions"`
	Simulation         SynthesisSimulationEvidence   `json:"simulation"`
	Issues             []reports.Issue               `json:"issues"`
}

type SynthesisSimulationEvidence struct {
	Status  string `json:"status"`
	ModelID string `json:"model_id,omitempty"`
	Reason  string `json:"reason"`
}

type SynthesisUnusedPinDecision struct {
	Component string `json:"component"`
	Unit      string `json:"unit,omitempty"`
	Function  string `json:"function"`
	Policy    string `json:"policy"`
}

type SynthesisSelection struct {
	IntentID    string `json:"intent_id"`
	Kind        string `json:"kind"`
	ParentID    string `json:"parent_id,omitempty"`
	ComponentID string `json:"component_id"`
	VariantID   string `json:"variant_id"`
	Reason      string `json:"reason"`
}

type SynthesisInterfaceBinding struct {
	Interface string `json:"interface"`
	Signal    string `json:"signal"`
	Component string `json:"component"`
	Function  string `json:"function"`
}

type SynthesisConstraintEvidence struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Value   string `json:"value"`
	Source  string `json:"source"`
}

// Synthesize lowers function-level intent to the existing explicit graph. The
// result remains fail-closed: a partially lowered graph is returned for
// diagnostics, but callers must not resolve or write it when issues block.
func (resolver *Resolver) Synthesize(ctx context.Context, document Document) (Document, SynthesisReport, []reports.Issue) {
	normalized := Normalize(document)
	report := SynthesisReport{
		Schema: SynthesisReportSchema, Status: "blocked", PolicyVersion: SynthesisPolicyVersion,
		InputHash: hashGraphValue(normalized), Selections: []SynthesisSelection{},
		InterfaceBindings: []SynthesisInterfaceBinding{}, DerivedConstraints: []SynthesisConstraintEvidence{}, UnusedPinDecisions: []SynthesisUnusedPinDecision{},
		Simulation: SynthesisSimulationEvidence{Status: "not_applicable", Reason: "no_complete_registered_graph_model"}, Issues: []reports.Issue{},
	}
	if normalized.Synthesis == nil {
		issue := synthesisIssue(CodeSynthesisIntentInvalid, "synthesis", "function-level synthesis intent is required", "provide the strict function-level intent form")
		report.Issues = []reports.Issue{issue}
		return Document{}, report, report.Issues
	}
	if issues := Validate(normalized); reports.HasBlockingIssue(issues) {
		report.Issues = issues
		return Document{}, report, issues
	}
	if resolver == nil || resolver.options.Catalog == nil {
		issue := synthesisIssue(CodeSynthesisComponentUnresolved, "catalog", "component catalog is required for function synthesis", "load one immutable verified component catalog")
		report.Issues = []reports.Issue{issue}
		return Document{}, report, report.Issues
	}

	intent := *normalized.Synthesis
	lowered := Document{
		Schema: SchemaID, Version: Version,
		Project:    Project{Name: normalized.Project.Name, Title: normalized.Project.Title, Description: normalized.Project.Description, Acceptance: normalized.Project.Acceptance},
		Components: []Component{}, Nets: []Net{}, NoConnects: []Endpoint{}, PowerFlags: []PowerFlag{}, Buses: []Bus{},
		Policy: normalized.Policy, Extensions: cloneRawMessages(normalized.Extensions),
	}
	selectedByIntent := map[string]ResolvedComponent{}
	var issues []reports.Issue
	for index, requirement := range intent.Functions {
		query := cloneComponentQuery(requirement.Query)
		instance := Component{
			ID: requirement.ID, Role: requirement.Role, ComponentID: requirement.ComponentID,
			Query: query, Value: requirement.Value, Parameters: append([]Parameter(nil), requirement.Parameters...),
			RequiredRatings:   append([]RequiredRating(nil), requirement.RequiredRatings...),
			RequiredFunctions: append([]string(nil), requirement.RequiredFunctions...),
			Population:        PopulationPopulate, Extensions: cloneRawMessages(requirement.Extensions),
		}
		applyRegulatorParameterConstraints(&instance)
		selected, selectionIssues := resolveComponent(ctx, instance, resolver.options, resolver.recordsByID, normalized.Project.Acceptance, index)
		if reports.HasBlockingIssue(selectionIssues) {
			issues = append(issues, synthesisSelectionIssues(requirement.ID, selectionIssues)...)
			continue
		}
		instance.ComponentID = selected.ComponentID
		instance.VariantID = selected.VariantID
		instance.Units = append([]ComponentUnit(nil), selected.Instance.Units...)
		instance.Query = nil
		lowered.Components = append(lowered.Components, instance)
		selected.Instance = instance
		selectedByIntent[requirement.ID] = selected
		report.Selections = append(report.Selections, SynthesisSelection{
			IntentID: requirement.ID, Kind: "primary", ComponentID: selected.ComponentID, VariantID: selected.VariantID,
			Reason: "selected by the existing catalog acceptance, function, rating, value, package, and confidence rules",
		})
	}

	interfaceComponents := map[string]string{}
	interfaceFunctions := map[string]map[string]string{}
	for _, requirement := range intent.Interfaces {
		instance, selected, bindings, selectionIssues := resolver.synthesizeInterface(ctx, requirement, normalized.Project.Acceptance, len(lowered.Components))
		if reports.HasBlockingIssue(selectionIssues) {
			issues = append(issues, selectionIssues...)
			continue
		}
		lowered.Components = append(lowered.Components, instance)
		selectedByIntent[instance.ID] = selected
		interfaceComponents[requirement.ID] = instance.ID
		interfaceFunctions[requirement.ID] = map[string]string{}
		for _, binding := range bindings {
			interfaceFunctions[requirement.ID][binding.Signal] = binding.Function
			report.InterfaceBindings = append(report.InterfaceBindings, binding)
		}
		report.Selections = append(report.Selections, SynthesisSelection{
			IntentID: requirement.ID, Kind: "interface", ComponentID: selected.ComponentID, VariantID: selected.VariantID,
			Reason: "smallest verified connector satisfying the normalized interface signal count",
		})
	}

	connected := map[string]bool{}
	for _, connection := range intent.Connections {
		net := Net{
			Name: connection.Name, Role: connection.Role, Required: synthesisBool(true),
			VoltageDomain: connection.VoltageDomain, CurrentMA: connection.CurrentMA,
			NetClass: synthesisNetClass(connection.Role), WidthMM: synthesisNetWidth(connection), ClearanceMM: 0.2,
			Endpoints: []Endpoint{},
		}
		for _, endpoint := range connection.Endpoints {
			if endpoint.Function != "" {
				selection, exists := selectedByIntent[endpoint.Function]
				if !exists {
					continue
				}
				binding, ok := uniqueResolvedFunction(selection.Functions, endpoint.Port)
				if !ok {
					issues = append(issues, synthesisIssue(CodeSynthesisConnectionUnresolved, "synthesis.connections."+connection.Name+"."+endpoint.Function+"."+endpoint.Port, "connection references an unavailable semantic function or alias", "correct the function port or catalog alias evidence"))
					continue
				}
				net.Endpoints = append(net.Endpoints, Endpoint{Component: endpoint.Function, Unit: binding.UnitID, SelectorKind: SelectorFunction, Selector: binding.Function})
				connected[endpoint.Function+"\x00"+normalizedFunctionKey(binding.Function)] = true
				continue
			}
			componentID := interfaceComponents[endpoint.Interface]
			function := interfaceFunctions[endpoint.Interface][endpoint.Signal]
			if componentID == "" || function == "" {
				continue
			}
			net.Endpoints = append(net.Endpoints, Endpoint{Component: componentID, SelectorKind: SelectorFunction, Selector: function})
			connected[componentID+"\x00"+normalizedFunctionKey(function)] = true
		}
		if len(net.Endpoints) < 2 {
			issues = append(issues, synthesisIssue(CodeSynthesisConnectionUnresolved, "synthesis.connections."+connection.Name, "connection has fewer than two resolved physical endpoints", "correct the component or interface capability that failed to resolve"))
		}
		lowered.Nets = append(lowered.Nets, net)
	}

	domains := make(map[string]PowerDomainIntent, len(intent.PowerDomains))
	functionRoles := make(map[string]ComponentRole, len(intent.Functions))
	for _, function := range intent.Functions {
		functionRoles[function.ID] = function.Role
	}
	for _, domain := range intent.PowerDomains {
		domains[domain.Name] = domain
		if domain.Source != PowerDomainExternal {
			continue
		}
		connectionFound := false
		physicalSourceFound := false
		for _, connection := range intent.Connections {
			if connection.VoltageDomain != domain.Name || !validPowerFlagNetRole(connection.Role) {
				continue
			}
			connectionFound = true
			for _, endpoint := range connection.Endpoints {
				if endpoint.Interface != "" {
					physicalSourceFound = true
				} else if role := functionRoles[endpoint.Function]; role == RoleConnector || role == RoleInputConnector {
					physicalSourceFound = true
				}
			}
			if physicalSourceFound {
				lowered.PowerFlags = append(lowered.PowerFlags, PowerFlag{Net: connection.Name})
			}
			break
		}
		if !connectionFound || !physicalSourceFound {
			issues = append(issues, synthesisIssue(CodeSynthesisPowerDomainInvalid, "synthesis.power_domains."+domain.Name, "external power domain has no connected external interface signal", "connect the domain to one named external interface signal"))
		}
	}

	issues = append(issues, applySensorFunctionPolicies(&lowered, intent, selectedByIntent, connected)...)
	issues = append(issues, resolver.expandCompanionRecipes(ctx, &lowered, intent, selectedByIntent, connected, &report)...)
	issues = append(issues, applyCatalogNoConnectPolicies(&lowered, selectedByIntent, connected)...)
	if !reports.HasBlockingIssue(issues) {
		lowered.Simulation, report.Simulation = deriveSynthesisSimulation(lowered, intent, selectedByIntent)
	}

	selectedIDs := make([]string, 0, len(selectedByIntent))
	for id := range selectedByIntent {
		selectedIDs = append(selectedIDs, id)
	}
	slices.Sort(selectedIDs)
	for _, selectionID := range selectedIDs {
		selection := selectedByIntent[selectionID]
		for _, function := range selection.Functions {
			key := selection.Instance.ID + "\x00" + normalizedFunctionKey(function.Function)
			if connected[key] {
				continue
			}
			if function.Required || slices.ContainsFunc(selection.Instance.RequiredFunctions, func(required string) bool {
				canonical, ok := canonicalFunction(selection.Functions, required)
				return ok && normalizedFunctionKey(canonical) == normalizedFunctionKey(function.Function)
			}) {
				issues = append(issues, synthesisIssue(CodeSynthesisUnusedPinPolicyMissing, "synthesis.functions."+selection.Instance.ID+"."+function.Function, "required semantic function is not connected and has no generated support or unused-pin policy", "add reviewed catalog support or unused-pin policy evidence"))
				continue
			}
			lowered.NoConnects = append(lowered.NoConnects, Endpoint{Component: selection.Instance.ID, Unit: function.UnitID, SelectorKind: SelectorFunction, Selector: function.Function})
		}
	}

	layoutIssues := deriveFunctionLayout(&lowered, intent, report.Selections, resolver.recordsByID)
	issues = append(issues, layoutIssues...)
	if !reports.HasBlockingIssue(issues) {
		if validationIssues := Validate(Normalize(lowered)); reports.HasBlockingIssue(validationIssues) {
			for _, issue := range validationIssues {
				issues = append(issues, synthesisIssue(CodeSynthesisIntentInvalid, "synthesis.lowered_graph."+issue.Path, issue.Message, "correct the deterministic synthesis policy or reviewed catalog recipe"))
			}
		}
	}
	lowered = Normalize(lowered)
	for _, endpoint := range lowered.NoConnects {
		report.UnusedPinDecisions = append(report.UnusedPinDecisions, SynthesisUnusedPinDecision{
			Component: endpoint.Component,
			Unit:      endpoint.Unit,
			Function:  endpoint.Selector,
			Policy:    "no_connect",
		})
	}
	report.DerivedConstraints = append(report.DerivedConstraints,
		SynthesisConstraintEvidence{Kind: "board_width", Subject: lowered.Project.Name, Value: strconv.FormatFloat(lowered.Project.Board.WidthMM, 'f', -1, 64) + " mm", Source: SynthesisPolicyVersion},
		SynthesisConstraintEvidence{Kind: "board_height", Subject: lowered.Project.Name, Value: strconv.FormatFloat(lowered.Project.Board.HeightMM, 'f', -1, 64) + " mm", Source: SynthesisPolicyVersion},
		SynthesisConstraintEvidence{Kind: "board_layers", Subject: lowered.Project.Name, Value: strconv.Itoa(lowered.Project.Board.Layers), Source: SynthesisPolicyVersion},
	)
	slices.SortStableFunc(report.Selections, func(left, right SynthesisSelection) int {
		if left.Kind != right.Kind {
			return strings.Compare(left.Kind, right.Kind)
		}
		return strings.Compare(left.IntentID, right.IntentID)
	})
	slices.SortStableFunc(report.InterfaceBindings, func(left, right SynthesisInterfaceBinding) int {
		if left.Interface != right.Interface {
			return strings.Compare(left.Interface, right.Interface)
		}
		return strings.Compare(left.Signal, right.Signal)
	})
	slices.SortStableFunc(report.UnusedPinDecisions, func(left, right SynthesisUnusedPinDecision) int {
		if left.Component != right.Component {
			return strings.Compare(left.Component, right.Component)
		}
		if left.Unit != right.Unit {
			return strings.Compare(left.Unit, right.Unit)
		}
		return strings.Compare(left.Function, right.Function)
	})
	report.Issues = finalizeGraphIssues(issues)
	if !reports.HasBlockingIssue(report.Issues) {
		report.Status = "ready"
		report.LoweredGraphHash = hashGraphValue(lowered)
	}
	return lowered, report, report.Issues
}

func cloneComponentQuery(query *ComponentQuery) *ComponentQuery {
	if query == nil {
		return nil
	}
	return &ComponentQuery{
		Text: query.Text, Family: query.Family, Package: query.Package,
		ValueKind: query.ValueKind, Value: query.Value, MinVoltageV: query.MinVoltageV,
		MinimumConfidence: query.MinimumConfidence,
	}
}

func applyRegulatorParameterConstraints(instance *Component) {
	if instance == nil || instance.Role != RoleRegulator || instance.Query == nil {
		return
	}
	if outputVoltage := synthesisParameterString(instance.Parameters, "output_voltage_v"); outputVoltage != "" && instance.Query.ValueKind == "" {
		instance.Query.ValueKind = "output_voltage"
		instance.Query.Value = outputVoltage
	}
	if outputCurrent := synthesisParameterString(instance.Parameters, "maximum_output_current_ma"); outputCurrent != "" {
		upsertRequiredRating(instance, RequiredRating{Kind: "output_current", Value: outputCurrent, Unit: "mA"})
	}
}

func upsertRequiredRating(instance *Component, required RequiredRating) {
	requiredValue, requiredOK := parseRatingValue(required)
	for index, existing := range instance.RequiredRatings {
		if !strings.EqualFold(existing.Kind, required.Kind) {
			continue
		}
		existingValue, existingOK := parseRatingValue(existing)
		if requiredOK && existingOK && requiredValue > existingValue {
			instance.RequiredRatings[index] = required
		}
		return
	}
	instance.RequiredRatings = append(instance.RequiredRatings, required)
}

func parseRatingValue(rating RequiredRating) (float64, bool) {
	value := strings.TrimSpace(rating.Value)
	if strings.IndexFunc(value, func(character rune) bool {
		return unicode.IsLetter(character) || character == '%' || character == 'Ω'
	}) >= 0 {
		return components.ParseEngineeringValue(value)
	}
	return components.ParseEngineeringValue(value + strings.TrimSpace(rating.Unit))
}

func (resolver *Resolver) synthesizeInterface(ctx context.Context, requirement InterfaceRequirement, acceptance AcceptanceLevel, index int) (Component, ResolvedComponent, []SynthesisInterfaceBinding, []reports.Issue) {
	type candidate struct {
		record components.ComponentRecord
		pins   int
	}
	var candidates []candidate
	for _, record := range resolver.options.Catalog.Records {
		if record.Family != "connector" || len(record.Packages) != 1 {
			continue
		}
		functions := map[string]bool{}
		for _, symbol := range record.Symbols {
			for _, function := range symbol.FunctionPins {
				functions[normalizedFunctionKey(function.Function)] = true
			}
		}
		count := 0
		for count < len(requirement.Signals) && functions["PIN_"+strconv.Itoa(count+1)] {
			count++
		}
		if count != len(requirement.Signals) {
			continue
		}
		if !components.AcceptanceAllows(componentAcceptance(acceptance), record.Verification.Confidence) ||
			!components.AcceptanceAllows(componentAcceptance(acceptance), record.Packages[0].Verification.Confidence) {
			continue
		}
		candidates = append(candidates, candidate{record: record, pins: len(functions)})
	}
	if len(candidates) == 0 {
		issue := synthesisIssue(CodeSynthesisInterfaceUnsupported, "synthesis.interfaces."+requirement.ID, "no accepted connector satisfies the interface signal count", "add verified connector catalog evidence for the required signal count")
		return Component{}, ResolvedComponent{}, nil, []reports.Issue{issue}
	}
	slices.SortStableFunc(candidates, func(left, right candidate) int {
		leftExtra := left.pins - len(requirement.Signals)
		rightExtra := right.pins - len(requirement.Signals)
		if leftExtra != rightExtra {
			return leftExtra - rightExtra
		}
		if left.record.Generic != right.record.Generic {
			if left.record.Generic {
				return 1
			}
			return -1
		}
		return strings.Compare(left.record.ID, right.record.ID)
	})
	chosen := candidates[0].record
	componentID := stableGeneratedID("iface", requirement.ID)
	instance := Component{
		ID: componentID, Role: synthesisInterfaceComponentRole(requirement.Role), ComponentID: chosen.ID,
		VariantID: chosen.Packages[0].ID, Population: PopulationPopulate,
	}
	for signalIndex := range requirement.Signals {
		instance.RequiredFunctions = append(instance.RequiredFunctions, "PIN_"+strconv.Itoa(signalIndex+1))
	}
	selected, selectionIssues := resolveComponent(ctx, instance, resolver.options, resolver.recordsByID, acceptance, index)
	if reports.HasBlockingIssue(selectionIssues) {
		return Component{}, ResolvedComponent{}, nil, synthesisSelectionIssues(requirement.ID, selectionIssues)
	}
	selected.Instance = instance
	bindings := make([]SynthesisInterfaceBinding, 0, len(requirement.Signals))
	for signalIndex, signal := range requirement.Signals {
		bindings = append(bindings, SynthesisInterfaceBinding{
			Interface: requirement.ID, Signal: signal.Name, Component: componentID,
			Function: "PIN_" + strconv.Itoa(signalIndex+1),
		})
	}
	return instance, selected, bindings, nil
}

func (resolver *Resolver) expandCompanionRecipes(ctx context.Context, document *Document, intent FunctionIntent, selected map[string]ResolvedComponent, connected map[string]bool, report *SynthesisReport) []reports.Issue {
	var issues []reports.Issue
	for _, requirement := range intent.Functions {
		parent, exists := selected[requirement.ID]
		if !exists {
			continue
		}
		for _, companion := range parent.Record.Companions {
			if !synthesisCompanionApplies(companion, requirement.Usage) {
				continue
			}
			enabled := companion.Required || (intent.Constraints.Protection == "required" && strings.Contains(strings.ToLower(companion.Role), "protection"))
			if !enabled {
				continue
			}
			path := "synthesis.functions." + requirement.ID + ".companions." + companion.ID
			if len(companion.Recipes) == 0 && len(companion.Ties) == 0 && len(companion.NoConnects) == 0 {
				issues = append(issues, synthesisIssue(CodeSynthesisSupportRecipeMissing, path, "required companion has no synthesizable component/network recipe", "add reviewed generic catalog companion recipe evidence"))
				continue
			}
			for _, tie := range companion.Ties {
				if issue := tieParentFunctionToLevel(document, parent, tie.Function, tie.Level, tie.ParentFunction, connected); issue != nil {
					issue.Path = path + ".ties." + tie.Function
					issues = append(issues, *issue)
				}
			}
			for _, function := range companion.NoConnects {
				if !resolvedComponentHasFunction(parent, function) {
					issues = append(issues, synthesisIssue(CodeSynthesisConnectionUnresolved, path+".no_connects."+function, "catalog no-connect policy references an unavailable parent function", "correct the reviewed catalog companion recipe"))
					continue
				}
				appendSynthesisNoConnect(document, parent.Instance.ID, function)
				connected[parent.Instance.ID+"\x00"+normalizedFunctionKey(function)] = true
			}
			for _, recipe := range companion.Recipes {
				supportID := stableGeneratedID("support", requirement.ID+"_"+companion.ID+"_"+recipe.ID)
				value, valueIssue := deriveCompanionRecipeValue(recipe, requirement.Parameters)
				if valueIssue != nil {
					valueIssue.Path = path + ".recipes." + recipe.ID + ".value_formula"
					issues = append(issues, *valueIssue)
					continue
				}
				query := &ComponentQuery{
					Family: recipe.Family, Package: recipe.Package, ValueKind: recipe.ValueKind,
					Value: value, MinVoltageV: recipe.MinVoltageV, MinimumConfidence: recipe.MinimumConfidence,
				}
				instance := Component{
					ID: supportID, Role: recipe.Role, Query: query, Value: value,
					RequiredFunctions: append([]string(nil), recipe.RequiredFunctions...), Population: PopulationPopulate,
				}
				resolved, selectionIssues := resolveComponent(ctx, instance, resolver.options, resolver.recordsByID, document.Project.Acceptance, len(document.Components))
				if reports.HasBlockingIssue(selectionIssues) {
					issues = append(issues, synthesisSelectionIssues(supportID, selectionIssues)...)
					continue
				}
				instance.ComponentID = resolved.ComponentID
				instance.VariantID = resolved.VariantID
				instance.Query = nil
				resolved.Instance = instance
				document.Components = append(document.Components, instance)
				selected[supportID] = resolved
				report.Selections = append(report.Selections, SynthesisSelection{
					IntentID: supportID, ParentID: requirement.ID, Kind: "support", ComponentID: resolved.ComponentID, VariantID: resolved.VariantID,
					Reason: "selected from a reviewed catalog companion component/network recipe",
				})
				if recipe.ValueFormula != nil {
					report.DerivedConstraints = append(report.DerivedConstraints, SynthesisConstraintEvidence{
						Kind: "calculated_support_value", Subject: supportID, Value: value,
						Source: "reviewed catalog " + recipe.ValueFormula.Kind + " using function parameter " + recipe.ValueFormula.Parameter,
					})
				}
				for _, connection := range recipe.Connections {
					if !resolvedComponentHasFunction(parent, connection.ParentFunction) {
						issues = append(issues, synthesisIssue(CodeSynthesisConnectionUnresolved, path+".recipes."+recipe.ID+"."+connection.ParentFunction, "companion recipe references an unavailable parent function", "correct the reviewed catalog companion recipe"))
						continue
					}
					if !resolvedComponentHasFunction(resolved, connection.Function) {
						issues = append(issues, synthesisIssue(CodeSynthesisConnectionUnresolved, path+".recipes."+recipe.ID+"."+connection.Function, "selected support component lacks the recipe function", "tighten the companion query or required-functions evidence"))
						continue
					}
					netIndex := ensureParentFunctionNet(document, parent.Instance.ID, connection.ParentFunction, connected)
					appendEndpointToNet(&document.Nets[netIndex], Endpoint{Component: supportID, SelectorKind: SelectorFunction, Selector: connection.Function})
					connected[supportID+"\x00"+normalizedFunctionKey(connection.Function)] = true
				}
			}
		}
	}
	return issues
}

var e96PreferredValues = []float64{
	100, 102, 105, 107, 110, 113, 115, 118, 121, 124, 127, 130, 133, 137, 140, 143,
	147, 150, 154, 158, 162, 165, 169, 174, 178, 182, 187, 191, 196, 200, 205, 210,
	215, 221, 226, 232, 237, 243, 249, 255, 261, 267, 274, 280, 287, 294, 301, 309,
	316, 324, 332, 340, 348, 357, 365, 374, 383, 392, 402, 412, 422, 432, 442, 453,
	464, 475, 487, 499, 511, 523, 536, 549, 562, 576, 590, 604, 619, 634, 649, 665,
	681, 698, 715, 732, 750, 768, 787, 806, 825, 845, 866, 887, 909, 931, 953, 976,
}

// Keep catalog-derived feedback parts inside the checked-in generic resistor
// envelope; higher-impedance networks require reviewed leakage/noise evidence.
const maxDerivedResistanceOhm = 10_000_000

func deriveCompanionRecipeValue(recipe components.CompanionPartRecipe, parameters []Parameter) (string, *reports.Issue) {
	// The caller has already selected one catalog recipe; this function derives
	// that recipe's value and performs no candidate or order-based tie-breaking.
	if recipe.ValueFormula == nil {
		return recipe.Value, nil
	}
	formula := recipe.ValueFormula
	if formula.Kind != "divider_upper_from_output_v1" || formula.PreferredSeries != "E96" {
		issue := synthesisIssue(CodeSynthesisSupportRecipeMissing, "", "companion value formula is unsupported", "select a catalog component with a supported deterministic value formula")
		return "", &issue
	}
	parameter := synthesisParameterString(parameters, formula.Parameter)
	outputVoltage, parsed := components.ParseEngineeringValue(parameter)
	referenceTolerance := 1e-12 * math.Max(1, math.Abs(formula.ReferenceVoltageV))
	if !parsed || math.IsNaN(outputVoltage) || math.IsInf(outputVoltage, 0) || outputVoltage < formula.ReferenceVoltageV-referenceTolerance {
		issue := synthesisIssue(CodeSynthesisSupportRecipeMissing, "", "divider output parameter must be a finite voltage at or above the catalog reference voltage", "provide a supported output voltage or select a compatible fixed regulator")
		return "", &issue
	}
	if math.Abs(outputVoltage-formula.ReferenceVoltageV) <= referenceTolerance {
		// Preserve the reviewed two-part feedback topology with a catalog-backed
		// 0-ohm link. Rewriting it as a direct net would change the BOM and graph
		// shape only at this boundary and make replay topology parameter-dependent.
		return "0", nil
	}
	target := formula.LowerResistanceOhm * (outputVoltage/formula.ReferenceVoltageV - 1)
	value := nearestE96Value(target)
	if value <= 0 || value > maxDerivedResistanceOhm {
		issue := synthesisIssue(CodeSynthesisSupportRecipeMissing, "", "derived divider resistance is outside the supported 0..10 Mohm passive range", "select a compatible reference voltage or divider base resistance")
		return "", &issue
	}
	return formatResistanceValue(value), nil
}

func nearestE96Value(target float64) float64 {
	best := 0.0
	bestDelta := math.Inf(1)
	bestOrdinal := -1
	for exponent := -4; exponent <= 7; exponent++ {
		scale := math.Pow10(exponent)
		for index, preferred := range e96PreferredValues {
			candidate := preferred * scale
			delta := math.Abs(candidate - target)
			ordinal := (exponent+4)*len(e96PreferredValues) + index
			tieTolerance := 1e-12 * math.Max(1, math.Max(delta, bestDelta))
			if bestOrdinal < 0 || delta < bestDelta-tieTolerance || (math.Abs(delta-bestDelta) <= tieTolerance && e96TieCandidatePreferred(candidate, ordinal, best, bestOrdinal)) {
				best, bestDelta, bestOrdinal = candidate, delta, ordinal
			}
		}
	}
	return best
}

func e96TieCandidatePreferred(candidate float64, ordinal int, current float64, currentOrdinal int) bool {
	if currentOrdinal < 0 {
		return true
	}
	candidateEven := ordinal%2 == 0
	currentEven := currentOrdinal%2 == 0
	if candidateEven != currentEven {
		return candidateEven
	}
	return candidate < current
}

func formatResistanceValue(value float64) string {
	switch {
	case value >= 1_000_000:
		return strconv.FormatFloat(value/1_000_000, 'g', -1, 64) + "M"
	case value >= 1_000:
		return strconv.FormatFloat(value/1_000, 'g', -1, 64) + "k"
	case value >= .001 && value < 1:
		return strconv.FormatFloat(value*1_000, 'g', -1, 64) + "m"
	default:
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
}

func applySensorFunctionPolicies(document *Document, intent FunctionIntent, selected map[string]ResolvedComponent, connected map[string]bool) []reports.Issue {
	var issues []reports.Issue
	requirements := make(map[string]FunctionRequirement, len(intent.Functions))
	for _, requirement := range intent.Functions {
		requirements[requirement.ID] = requirement
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		component := selected[id]
		if component.Record.Sensor == nil || !strings.EqualFold(requirements[id].Usage, "i2c_peripheral") {
			continue
		}
		evidence := component.Record.Sensor
		for _, mode := range evidence.I2CModeConnections {
			if issue := tieParentFunctionToLevel(document, component, mode.Function, mode.Level, mode.ParentFunction, connected); issue != nil {
				issue.Path = "synthesis.functions." + id + ".sensor_evidence.i2c_mode_connections." + mode.Function
				issues = append(issues, *issue)
			}
		}
		address := synthesisParameterString(requirements[id].Parameters, "i2c_address")
		var addressEvidence *components.SensorI2CAddress
		for index := range evidence.I2CAddresses {
			candidate := &evidence.I2CAddresses[index]
			if (address != "" && strings.EqualFold(candidate.Address, address)) || (address == "" && candidate.Default) {
				addressEvidence = candidate
				break
			}
		}
		if address != "" && addressEvidence == nil {
			issues = append(issues, synthesisIssue(CodeSynthesisConnectionUnresolved, "synthesis.functions."+id+".parameters.i2c_address", "requested I2C address is absent from reviewed sensor evidence", "select one reviewed catalog address"))
		} else if addressEvidence != nil {
			if issue := tieParentFunctionToLevel(document, component, addressEvidence.SelectFunction, addressEvidence.Level, addressEvidence.ParentFunction, connected); issue != nil {
				issue.Path = "synthesis.functions." + id + ".sensor_evidence.i2c_addresses." + addressEvidence.Address
				issues = append(issues, *issue)
			}
		}
		for _, policy := range evidence.UnusedPinPolicies {
			if policy.Policy != "no_connect" {
				issues = append(issues, synthesisIssue(CodeSynthesisUnusedPinPolicyMissing, "synthesis.functions."+id+".sensor_evidence.unused_pin_policies."+policy.Function, "unsupported reviewed unused-pin policy "+policy.Policy, "add a supported no_connect or connection policy"))
				continue
			}
			appendSynthesisNoConnect(document, id, policy.Function)
			connected[id+"\x00"+normalizedFunctionKey(policy.Function)] = true
		}
	}
	return issues
}

func applyCatalogNoConnectPolicies(document *Document, selected map[string]ResolvedComponent, connected map[string]bool) []reports.Issue {
	var issues []reports.Issue
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		component := selected[id]
		for _, hint := range component.Record.RoutingHints {
			if !strings.EqualFold(strings.TrimSpace(hint.Kind), "no_connect") {
				continue
			}
			function, ok := canonicalFunction(component.Functions, strings.TrimSpace(hint.NetRole))
			if !ok {
				issues = append(issues, synthesisIssue(CodeSynthesisUnusedPinPolicyMissing, "synthesis.functions."+id+".routing_hints.no_connect", "catalog no-connect hint does not identify an available semantic function", "set net_role to the reviewed semantic function name"))
				continue
			}
			appendSynthesisNoConnect(document, component.Instance.ID, function)
			connected[component.Instance.ID+"\x00"+normalizedFunctionKey(function)] = true
		}
	}
	return issues
}

func tieParentFunctionToLevel(document *Document, component ResolvedComponent, function, level, parentFunction string, connected map[string]bool) *reports.Issue {
	if !resolvedComponentHasFunction(component, function) {
		issue := synthesisIssue(CodeSynthesisConnectionUnresolved, "synthesis.functions."+component.Instance.ID+"."+function, "reviewed policy references an unavailable semantic function", "correct the catalog evidence")
		return &issue
	}
	high := strings.EqualFold(level, "high")
	if !high && !strings.EqualFold(level, "low") {
		issue := synthesisIssue(CodeSynthesisConnectionUnresolved, "synthesis.functions."+component.Instance.ID+"."+function, "reviewed tie level must be high or low", "correct the catalog evidence")
		return &issue
	}
	matchingNets := map[int][]string{}
	if parentFunction != "" {
		if !resolvedComponentHasFunction(component, parentFunction) {
			issue := synthesisIssue(CodeSynthesisPowerDomainInvalid, "synthesis.functions."+component.Instance.ID+"."+function, "reviewed tie references an unavailable parent supply function", "correct the catalog evidence")
			return &issue
		}
		if netIndex := findParentFunctionNet(document, component.Instance.ID, parentFunction); netIndex >= 0 && synthesisSupplyRoleMatchesLevel(document.Nets[netIndex].Role, high) {
			matchingNets[netIndex] = append(matchingNets[netIndex], parentFunction)
		}
	} else {
		for _, candidate := range component.Functions {
			if !synthesisPowerElectrical(candidate.Electrical) {
				continue
			}
			if netIndex := findParentFunctionNet(document, component.Instance.ID, candidate.Function); netIndex >= 0 && synthesisSupplyRoleMatchesLevel(document.Nets[netIndex].Role, high) {
				matchingNets[netIndex] = append(matchingNets[netIndex], candidate.Function)
			}
		}
	}
	if len(matchingNets) > 1 {
		issue := synthesisIssue(CodeSynthesisPowerDomainInvalid, "synthesis.functions."+component.Instance.ID+"."+function, "reviewed "+level+" tie is ambiguous across multiple connected parent supplies", "name the required rail explicitly in reviewed catalog evidence")
		return &issue
	}
	for netIndex := range matchingNets {
		appendEndpointToNet(&document.Nets[netIndex], Endpoint{Component: component.Instance.ID, SelectorKind: SelectorFunction, Selector: function})
		connected[component.Instance.ID+"\x00"+normalizedFunctionKey(function)] = true
		return nil
	}
	issue := synthesisIssue(CodeSynthesisPowerDomainInvalid, "synthesis.functions."+component.Instance.ID+"."+function, "cannot find a connected parent supply for reviewed "+level+" tie", "connect the parent power and return functions to named power domains")
	return &issue
}

func synthesisPowerElectrical(electrical string) bool {
	switch strings.ToLower(strings.TrimSpace(electrical)) {
	case "power_in", "power_out", "power_input", "power_output":
		return true
	default:
		return false
	}
}

func synthesisSupplyRoleMatchesLevel(role NetRole, high bool) bool {
	if high {
		return role == NetRolePower || role == NetRolePowerPos
	}
	return role == NetRoleGround || role == NetRoleReturn || role == NetRolePowerNeg
}

func ensureParentFunctionNet(document *Document, component, function string, connected map[string]bool) int {
	if index := findParentFunctionNet(document, component, function); index >= 0 {
		return index
	}
	name := stableGeneratedNetName(component, function, document.Nets)
	document.Nets = append(document.Nets, Net{
		Name: name, Role: NetRoleBias, Required: synthesisBool(true), NetClass: "signal", WidthMM: 0.2, ClearanceMM: 0.2,
		Endpoints: []Endpoint{{Component: component, SelectorKind: SelectorFunction, Selector: function}},
	})
	connected[component+"\x00"+normalizedFunctionKey(function)] = true
	return len(document.Nets) - 1
}

func findParentFunctionNet(document *Document, component, function string) int {
	want := normalizedFunctionKey(function)
	for netIndex, net := range document.Nets {
		for _, endpoint := range net.Endpoints {
			if endpoint.Component == component && endpoint.SelectorKind == SelectorFunction && normalizedFunctionKey(endpoint.Selector) == want {
				return netIndex
			}
		}
	}
	return -1
}

func appendEndpointToNet(net *Net, endpoint Endpoint) {
	for _, existing := range net.Endpoints {
		if existing.Component == endpoint.Component && existing.Unit == endpoint.Unit && existing.SelectorKind == endpoint.SelectorKind && normalizedFunctionKey(existing.Selector) == normalizedFunctionKey(endpoint.Selector) {
			return
		}
	}
	net.Endpoints = append(net.Endpoints, endpoint)
}

func appendSynthesisNoConnect(document *Document, component, function string) {
	for _, existing := range document.NoConnects {
		if existing.Component == component && existing.SelectorKind == SelectorFunction && normalizedFunctionKey(existing.Selector) == normalizedFunctionKey(function) {
			return
		}
	}
	document.NoConnects = append(document.NoConnects, Endpoint{Component: component, SelectorKind: SelectorFunction, Selector: function})
}

func resolvedComponentHasFunction(component ResolvedComponent, function string) bool {
	want := normalizedFunctionKey(function)
	return slices.ContainsFunc(component.Functions, func(candidate ResolvedFunction) bool {
		if normalizedFunctionKey(candidate.Function) == want {
			return true
		}
		return slices.ContainsFunc(candidate.Aliases, func(alias string) bool { return normalizedFunctionKey(alias) == want })
	})
}

func synthesisParameterString(parameters []Parameter, name string) string {
	for _, parameter := range parameters {
		if strings.EqualFold(parameter.Name, name) && parameter.Value.String != nil {
			return strings.TrimSpace(*parameter.Value.String)
		}
	}
	return ""
}

func stableGeneratedNetName(component, function string, nets []Net) string {
	base := strings.ToUpper(stableGeneratedID("support", component+"_"+normalizedFunctionKey(function)))
	used := make(map[string]bool, len(nets))
	for _, net := range nets {
		used[net.Name] = true
	}
	if !used[base] {
		return base
	}
	for ordinal := 2; ; ordinal++ {
		candidate := base + "_" + strconv.Itoa(ordinal)
		if !used[candidate] {
			return candidate
		}
	}
}

func deriveFunctionLayout(document *Document, intent FunctionIntent, selections []SynthesisSelection, recordsByID map[string]components.ComponentRecord) []reports.Issue {
	count := len(document.Components)
	columns := int(math.Ceil(math.Sqrt(float64(max(count, 1)))))
	rows := int(math.Ceil(float64(max(count, 1)) / float64(columns)))
	spacing := intent.Constraints.PreferredComponentSpacingMM
	if spacing == 0 {
		spacing = 1.5
	}
	maxEnvelopeWidth, maxEnvelopeHeight := synthesisPhysicalEnvelope(document.Components, recordsByID)
	// Leave one board-edge clearance and two placement-spacing bands on each
	// side. The second spacing band gives an asymmetric library courtyard a
	// non-zero deterministic candidate interval after the synthesized region is
	// inset from the board outline.
	envelopeMargin := 2 * (1 + 2*spacing)
	// A single large physical envelope (for example a module keepout) must
	// not consume the packing bands implied by the remaining grid columns and
	// rows. Add those deterministic bands around the largest envelope instead
	// of treating the envelope and component-count estimates as alternatives.
	minimumWidth := math.Max(20, maxEnvelopeWidth+envelopeMargin)
	minimumHeight := math.Max(15, maxEnvelopeHeight+envelopeMargin)
	if minimumWidth > intent.Constraints.MaxWidthMM || minimumHeight > intent.Constraints.MaxHeightMM {
		issue := synthesisIssue(CodeSynthesisLayoutConstraintUnsupported, "synthesis.constraints", fmt.Sprintf("minimum %.1fx%.1f mm physical envelope exceeds %.1fx%.1f mm function-intent bounds", minimumWidth, minimumHeight, intent.Constraints.MaxWidthMM, intent.Constraints.MaxHeightMM), "increase bounded board dimensions or reduce physical-envelope constraints")
		return []reports.Issue{issue}
	}
	packingWidth := math.Max(math.Max(20, 15+synthesisEstimatedComponentPitchMM*float64(columns)), maxEnvelopeWidth+envelopeMargin+synthesisEstimatedComponentPitchMM*math.Max(float64(columns-1), 0))
	packingHeight := math.Max(math.Max(15, 12+synthesisEstimatedComponentPitchMM*float64(rows)), maxEnvelopeHeight+envelopeMargin+synthesisEstimatedComponentPitchMM*math.Max(float64(rows-1), 0))
	// The packing estimate is conservative, not a physical minimum. Respect a
	// tighter provider bound when the largest reviewed envelope still fits and
	// let the deterministic placer prove or reject the denser arrangement.
	width := math.Min(packingWidth, intent.Constraints.MaxWidthMM)
	height := math.Min(packingHeight, intent.Constraints.MaxHeightMM)
	layers := synthesisCopperLayerCount(count, document.Nets)
	document.Project.Board = Board{WidthMM: width, HeightMM: height, Layers: layers, EdgeClearanceMM: 1}
	members := make([]string, 0, len(document.Components))
	for _, component := range document.Components {
		members = append(members, component.ID)
		document.Schematic.Placements = append(document.Schematic.Placements, SchematicPlacement{Component: component.ID, Group: "synthesized", Orientation: "normal", Mirror: "none"})
		document.PCB.Placements = append(document.PCB.Placements, PCBPlacement{Component: component.ID, Region: "main"})
	}
	document.Schematic.Flow = FlowLeftToRight
	document.Schematic.Origin = OriginCentered
	document.Schematic.Groups = []SchematicGroup{{ID: "synthesized", Label: "Synthesized circuit", Role: "functional", Members: members, Rank: 0}}
	document.Schematic.Lanes = SchematicLanes{Power: LaneTop, Signals: LaneMiddle, Ground: LaneBottom}
	if slices.ContainsFunc(document.Nets, func(net Net) bool { return net.Role == NetRolePowerNeg }) {
		lower := LaneLower
		document.Schematic.Lanes.PowerNegative = &lower
	}
	document.Schematic.Rules = SchematicRules{
		PositivePowerTop: synthesisBool(true), GroundBottom: synthesisBool(true), CenterOnPage: synthesisBool(true),
		PreferLabelsForLongNets: synthesisBool(true), AvoidWireCrossings: synthesisBool(true),
		MinGroupSpacingMM: math.Max(2, spacing*2), MinComponentSpacingMM: spacing,
	}
	hierarchyMode := "flat"
	if count > 24 {
		hierarchyMode = "auto"
	}
	document.Schematic.Hierarchy = HierarchyPolicy{Mode: hierarchyMode, MaxComponentsPerSheet: 24}
	// The placement engine applies the board edge clearance independently. Keep
	// the synthesized main region coincident with the board so region membership
	// does not apply that clearance a second time.
	document.PCB.Regions = []PCBRegion{{ID: "main", Role: "synthesized", Bounds: Bounds{WidthMM: width, HeightMM: height}}}
	document.PCB.Keepouts = []PCBKeepout{}
	document.PCB.Zones = []PCBZone{}
	return nil
}

func synthesisCopperLayerCount(componentCount int, nets []Net) int {
	routingBranches := 0
	for _, net := range nets {
		if len(net.Endpoints) > 1 {
			routingBranches += len(net.Endpoints) - 1
		}
	}
	// Two copper layers are sufficient while the connection forest remains
	// sparse. Once there are at least two independent tree branches per placed
	// component, reserve two internal routing planes deterministically.
	if componentCount > 0 && routingBranches >= 2*componentCount {
		return 4
	}
	return 2
}

func uniqueResolvedFunction(functions []ResolvedFunction, requested string) (ResolvedFunction, bool) {
	matches := []ResolvedFunction{}
	for _, function := range functions {
		matched := strings.EqualFold(function.Function, requested) || slices.ContainsFunc(function.Aliases, func(alias string) bool {
			return strings.EqualFold(alias, requested)
		})
		if matched {
			matches = append(matches, function)
		}
	}
	if len(matches) == 0 {
		return ResolvedFunction{}, false
	}
	canonical := normalizedFunctionKey(matches[0].Function)
	for _, match := range matches[1:] {
		if normalizedFunctionKey(match.Function) != canonical || match.Unit != matches[0].Unit || match.UnitID != matches[0].UnitID {
			return ResolvedFunction{}, false
		}
	}
	return matches[0], true
}

func synthesisPhysicalEnvelope(instances []Component, recordsByID map[string]components.ComponentRecord) (float64, float64) {
	var maxWidth, maxHeight float64
	for _, instance := range instances {
		record, ok := recordsByID[strings.TrimSpace(instance.ComponentID)]
		if !ok {
			continue
		}
		for _, variant := range record.Packages {
			if variant.ID != strings.TrimSpace(instance.VariantID) {
				continue
			}
			if variant.DimensionsMM != nil {
				maxWidth = math.Max(maxWidth, variant.DimensionsMM.Width)
				maxHeight = math.Max(maxHeight, variant.DimensionsMM.Height)
			}
			for _, constraint := range variant.Constraints {
				width, height, ok := physicalConstraintDimensionsMM(constraint)
				if ok {
					maxWidth = math.Max(maxWidth, width)
					maxHeight = math.Max(maxHeight, height)
				}
			}
			break
		}
	}
	return maxWidth, maxHeight
}

func physicalConstraintDimensionsMM(constraint components.PhysicalConstraint) (float64, float64, bool) {
	if !strings.EqualFold(strings.TrimSpace(constraint.Unit), "mm") {
		return 0, 0, false
	}
	parts := strings.FieldsFunc(strings.TrimSpace(constraint.Value), func(r rune) bool {
		return r == 'x' || r == 'X' || r == '\u00d7'
	})
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, widthErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	height, heightErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 || math.IsNaN(width) || math.IsNaN(height) || math.IsInf(width, 0) || math.IsInf(height, 0) {
		return 0, 0, false
	}
	return width, height, true
}

func synthesisSelectionIssues(intentID string, source []reports.Issue) []reports.Issue {
	result := make([]reports.Issue, 0, len(source))
	for _, issue := range source {
		result = append(result, synthesisIssue(CodeSynthesisComponentUnresolved, "synthesis.functions."+intentID, issue.Message, "correct the catalog query, rating, function, package, or confidence requirement"))
	}
	return result
}

func synthesisIssue(code reports.Code, path, message, suggestion string) reports.Issue {
	return annotateGraphIssue(reports.Issue{
		Code: code, Severity: reports.SeverityError, Stage: "synthesis", RetryScope: "synthesis",
		Path: path, Message: message, Suggestion: suggestion,
	})
}

func synthesisCompanionApplies(companion components.CompanionRequirement, usage string) bool {
	if len(companion.AppliesTo) == 0 {
		return true
	}
	return slices.ContainsFunc(companion.AppliesTo, func(candidate string) bool {
		return strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(usage))
	})
}

func synthesisInterfaceComponentRole(role InterfaceRole) ComponentRole {
	switch role {
	case InterfacePowerInput, InterfaceAnalogInput, InterfaceDigitalIn:
		return RoleInputConnector
	case InterfacePowerOutput, InterfaceAnalogOut, InterfaceDigitalOut:
		return RoleOutputConnector
	default:
		return RoleConnector
	}
}

func synthesisNetClass(role NetRole) string {
	switch role {
	case NetRolePower, NetRolePowerPos, NetRolePowerNeg:
		return "power"
	case NetRoleGround, NetRoleReturn:
		return "ground"
	default:
		return "signal"
	}
}

func synthesisNetWidth(connection FunctionConnection) float64 {
	switch connection.Role {
	case NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround, NetRoleReturn:
		if connection.CurrentMA >= 300 {
			return 0.5
		}
		return 0.3
	default:
		return 0.2
	}
}

func stableGeneratedID(prefix, source string) string {
	raw := prefix + "_" + source
	var sanitized strings.Builder
	sanitized.Grow(len(raw))
	for _, character := range raw {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' {
			sanitized.WriteRune(character)
		} else {
			sanitized.WriteByte('_')
		}
	}
	candidate := sanitized.String()
	if len(candidate) <= 63 {
		return candidate
	}
	hash := hashGraphValue(raw)
	return candidate[:54] + "_" + hash[:8]
}

func synthesisBool(value bool) *bool {
	return &value
}
