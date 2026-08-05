package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

type physicalSupportGroup struct {
	Controller    string
	Components    []string
	MaxDistanceMM map[string]float64
}

type physicalSupportPart struct {
	ID          string
	Kind        string
	Usage       string
	Package     string
	ValueSI     float64
	MinVoltageV float64
	NearMM      float64
}

// completePhysicalModelSupport realizes the external support assumed by a
// trusted functional model. It is keyed by the model contract and normalized
// pin functions, never by a catalog identity or requirement fixture. The
// averaged synchronous-buck model currently owns the only such contract.
func completePhysicalModelSupport(
	ctx context.Context,
	document *circuitgraph.Document,
	bindings *[]PhysicalSemanticBinding,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	catalog *components.Catalog,
	instancePrimitives map[string]PrimitiveCandidate,
) ([]physicalSupportGroup, []reports.Issue) {
	groups := []physicalSupportGroup{}
	issues := []reports.Issue{}
	for _, instance := range graph.Instances {
		primitive, found := instancePrimitives[instance.ID]
		if !found || !primitiveHasModel(primitive, simmodel.PrimitiveSynchronousBuckRegulatorV1) {
			continue
		}
		group, supportIssues := completeSynchronousBuckPhysicalSupport(
			ctx, document, bindings, graph, instance, primitive, inventory, catalog,
		)
		issues = append(issues, supportIssues...)
		if len(group.Components) != 0 {
			groups = append(groups, group)
		}
	}
	slices.SortFunc(groups, func(left, right physicalSupportGroup) int {
		return cmp.Compare(left.Controller, right.Controller)
	})
	return groups, reports.SortedIssues(issues)
}

func primitiveHasModel(primitive PrimitiveCandidate, modelID string) bool {
	return slices.ContainsFunc(primitive.Models, func(model PrimitiveModelContract) bool {
		return model.ModelID == modelID
	})
}

func completeSynchronousBuckPhysicalSupport(
	ctx context.Context,
	document *circuitgraph.Document,
	bindings *[]PhysicalSemanticBinding,
	graph CandidateGraph,
	instance GraphInstance,
	primitive PrimitiveCandidate,
	inventory PrimitiveInventory,
	catalog *components.Catalog,
) (physicalSupportGroup, []reports.Issue) {
	group := physicalSupportGroup{
		Controller: instance.ID, Components: []string{}, MaxDistanceMM: map[string]float64{},
	}
	issues := []reports.Issue{}
	if catalog == nil {
		return group, []reports.Issue{physicalLoweringError(
			"physical.components."+instance.ID,
			"trusted synchronous-buck support completion requires the bound component catalog",
		)}
	}
	record, found := components.LookupRecord(catalog, primitive.CatalogID)
	if !found {
		return group, []reports.Issue{physicalLoweringError(
			"physical.components."+instance.ID,
			"trusted synchronous-buck support completion cannot find the selected controller record",
		)}
	}
	functions := physicalRecordFunctions(record, primitive.VariantID)
	inputNode, inputOK := graphInstanceTerminalNode(instance, "PVIN")
	switchNode, switchOK := graphInstanceTerminalNode(instance, "SW")
	groundNode, groundOK := graphInstanceTerminalNode(instance, "PGND")
	outputNode, outputOK := synchronousBuckPhysicalOutputNode(graph, switchNode)
	if !inputOK || !switchOK || !groundOK || !outputOK {
		return group, []reports.Issue{physicalLoweringError(
			"physical.components."+instance.ID,
			"trusted synchronous-buck support completion requires resolved input, switch, output, and ground nodes",
		)}
	}
	graphNets := map[string]string{
		"input":  physicalGraphNetName(graph, inputNode),
		"switch": physicalGraphNetName(graph, switchNode),
		"ground": physicalGraphNetName(graph, groundNode),
		"output": physicalGraphNetName(graph, outputNode),
	}
	for role, net := range graphNets {
		if net == "" {
			issues = append(issues, physicalLoweringError(
				"physical.components."+instance.ID+"."+role,
				"trusted synchronous-buck support node has no lowered physical net",
			))
		}
	}
	if len(issues) != 0 {
		return group, issues
	}

	handled := map[string]bool{}
	for _, terminal := range primitive.Terminals {
		handled[strings.ToUpper(strings.TrimSpace(terminal.Function))] = true
	}
	connectControllerFunction := func(function, net string) {
		key := strings.ToUpper(function)
		binding, exists := functions[key]
		if !exists {
			return
		}
		appendPhysicalNetEndpoint(document, net, circuitgraph.Endpoint{
			Component: instance.ID, Unit: binding.unit,
			SelectorKind: circuitgraph.SelectorFunction, Selector: binding.function,
		})
		appendPhysicalRequiredFunction(document, instance.ID, binding.function)
		handled[key] = true
	}
	for key, binding := range functions {
		switch {
		case strings.HasPrefix(key, "PVIN_AUX"):
			connectControllerFunction(binding.function, graphNets["input"])
		case strings.HasPrefix(key, "SW_AUX"):
			connectControllerFunction(binding.function, graphNets["switch"])
		case strings.HasPrefix(key, "PGND_AUX"), strings.HasPrefix(key, "NC_GND_"):
			connectControllerFunction(binding.function, graphNets["ground"])
		}
	}
	connectControllerFunction("BIAS", graphNets["output"])

	modelSupport := "model_support:" + simmodel.PrimitiveSynchronousBuckRegulatorV1
	supportCount := 0
	for _, companion := range record.Companions {
		if !companion.Required || !slices.Contains(companion.AppliesTo, modelSupport) {
			continue
		}
		supportCount++
		path := "physical.components." + instance.ID + ".model_support." + companion.ID
		for _, tie := range companion.Ties {
			binding, exists := functions[strings.ToUpper(strings.TrimSpace(tie.Function))]
			if !exists {
				issues = append(issues, physicalLoweringError(path+".ties."+tie.Function, "model-support tie references an unavailable controller function"))
				continue
			}
			net := ""
			if tie.ParentFunction != "" {
				net = physicalControllerFunctionNet(*document, instance.ID, tie.ParentFunction)
			} else if strings.EqualFold(tie.Level, "low") {
				net = graphNets["ground"]
			}
			if net == "" {
				issues = append(issues, physicalLoweringError(path+".ties."+tie.Function, "model-support tie cannot resolve its reviewed physical net"))
				continue
			}
			connectControllerFunction(binding.function, net)
		}
		for _, function := range companion.NoConnects {
			binding, exists := functions[strings.ToUpper(strings.TrimSpace(function))]
			if !exists {
				issues = append(issues, physicalLoweringError(path+".no_connects."+function, "model-support no-connect policy references an unavailable controller function"))
				continue
			}
			if physicalControllerFunctionNet(*document, instance.ID, binding.function) != "" {
				issues = append(issues, physicalLoweringError(path+".no_connects."+function, "model-support no-connect policy conflicts with an existing physical connection"))
				continue
			}
			appendPhysicalNoConnect(document, circuitgraph.Endpoint{
				Component: instance.ID, Unit: binding.unit,
				SelectorKind: circuitgraph.SelectorFunction, Selector: binding.function,
			})
			appendPhysicalRequiredFunction(document, instance.ID, binding.function)
			handled[strings.ToUpper(binding.function)] = true
		}
		for _, recipe := range companion.Recipes {
			valueSI, valueOK := components.ParseEngineeringValue(recipe.Value)
			nearMM, nearOK := physicalSupportNearMM(record, companion.ID)
			if !valueOK || valueSI <= 0 || !nearOK {
				issues = append(issues, physicalLoweringError(path+".recipes."+recipe.ID, "model-support recipe requires a positive fixed value and reviewed near-placement bound"))
				continue
			}
			part := physicalSupportPart{
				ID:   instance.ID + "_" + canonicalIdentifier(companion.ID+"_"+recipe.ID),
				Kind: recipe.Family, Usage: companion.Role, Package: recipe.Package,
				ValueSI: valueSI, MinVoltageV: recipe.MinVoltageV, NearMM: nearMM,
			}
			selected, selectedOK := selectPhysicalSupportPrimitive(
				ctx, catalog, part.Kind, part.Package, part.ValueSI, part.MinVoltageV,
			)
			if !selectedOK {
				issues = append(issues, physicalLoweringError(
					path+".recipes."+recipe.ID,
					fmt.Sprintf("model-support recipe requires a reviewed %.12g SI %s in package %s", part.ValueSI, part.Kind, part.Package),
				))
				continue
			}
			component := physicalSupportComponent(part, selected)
			document.Components = append(document.Components, component)
			connectedSupportFunctions := map[string]bool{}
			for _, connection := range recipe.Connections {
				controllerBinding, exists := functions[strings.ToUpper(strings.TrimSpace(connection.ParentFunction))]
				supportEndpoint, supportOK := selected.endpointForFunction(part.ID, connection.Function)
				if !exists || !supportOK {
					issues = append(issues, physicalLoweringError(path+".recipes."+recipe.ID+"."+connection.Function, "model-support connection cannot resolve its controller or support function"))
					continue
				}
				net := physicalControllerFunctionNet(*document, instance.ID, controllerBinding.function)
				if net == "" {
					net = "SUPPORT_" + strings.ToUpper(canonicalIdentifier(instance.ID+"_"+controllerBinding.function))
					document.Nets = append(document.Nets, circuitgraph.Net{
						Name: net, Role: physicalSupportNetRole(controllerBinding.function), Required: graphBool(true),
						Endpoints: []circuitgraph.Endpoint{{
							Component: instance.ID, Unit: controllerBinding.unit,
							SelectorKind: circuitgraph.SelectorFunction, Selector: controllerBinding.function,
						}},
					})
				}
				appendPhysicalNetEndpoint(document, net, supportEndpoint)
				appendPhysicalRequiredFunction(document, instance.ID, controllerBinding.function)
				handled[strings.ToUpper(controllerBinding.function)] = true
				connectedSupportFunctions[strings.ToUpper(connection.Function)] = true
			}
			allSelectedTerminalsConnected := len(connectedSupportFunctions) == len(selected.terminals)
			for _, terminal := range selected.terminals {
				if !connectedSupportFunctions[strings.ToUpper(strings.TrimSpace(terminal.function))] {
					allSelectedTerminalsConnected = false
				}
			}
			if !allSelectedTerminalsConnected {
				issues = append(issues, physicalLoweringError(path+".recipes."+recipe.ID, "model-support recipe does not connect every selected support terminal exactly once"))
			}
			group.Components = append(group.Components, part.ID)
			group.MaxDistanceMM[part.ID] = part.NearMM
			*bindings = append(*bindings, PhysicalSemanticBinding{
				Kind: "model_support", SemanticID: instance.ID + "." + companion.ID + "." + recipe.ID,
				Component: part.ID, CatalogID: selected.record.ID, VariantID: selected.variant.ID,
				EvidenceSHA: hashJSON(struct {
					Model     string
					Companion components.CompanionRequirement
					Part      physicalSupportPart
					Key       string
				}{simmodel.PrimitiveSynchronousBuckRegulatorV1, companion, part, selected.key()}),
			})
		}
	}
	if supportCount == 0 {
		issues = append(issues, physicalLoweringError(
			"physical.components."+instance.ID+".model_support",
			"trusted synchronous-buck model requires reviewed catalog model-support companion metadata",
		))
	}

	for key, binding := range functions {
		if handled[key] || !binding.required {
			continue
		}
		issues = append(issues, physicalLoweringError(
			"physical.components."+instance.ID+".functions."+strings.ToLower(binding.function),
			"trusted synchronous-buck physical function has no model-backed connection or support disposition",
		))
	}
	slices.Sort(group.Components)
	return group, reports.SortedIssues(issues)
}

func physicalControllerFunctionNet(document circuitgraph.Document, component, function string) string {
	matched := ""
	for _, net := range document.Nets {
		for _, endpoint := range net.Endpoints {
			if endpoint.Component != component || endpoint.SelectorKind != circuitgraph.SelectorFunction ||
				!strings.EqualFold(strings.TrimSpace(endpoint.Selector), strings.TrimSpace(function)) {
				continue
			}
			if matched != "" && matched != net.Name {
				return ""
			}
			matched = net.Name
		}
	}
	return matched
}

func physicalSupportNearMM(record components.ComponentRecord, target string) (float64, bool) {
	for _, hint := range record.PlacementHints {
		if hint.Kind != "near" || !strings.EqualFold(strings.TrimSpace(hint.Target), strings.TrimSpace(target)) ||
			!strings.EqualFold(strings.TrimSpace(hint.Unit), "mm") {
			continue
		}
		value, ok := components.ParseEngineeringValue(hint.Value)
		return value, ok && value > 0
	}
	return 0, false
}

func physicalSupportNetRole(function string) circuitgraph.NetRole {
	switch strings.ToUpper(strings.TrimSpace(function)) {
	case "VCC", "BIAS":
		return circuitgraph.NetRolePower
	case "RT":
		return circuitgraph.NetRoleTiming
	default:
		return circuitgraph.NetRoleBias
	}
}

func appendPhysicalNoConnect(document *circuitgraph.Document, endpoint circuitgraph.Endpoint) {
	if slices.ContainsFunc(document.NoConnects, func(existing circuitgraph.Endpoint) bool {
		return existing.Component == endpoint.Component && existing.Unit == endpoint.Unit &&
			existing.SelectorKind == endpoint.SelectorKind && strings.EqualFold(existing.Selector, endpoint.Selector)
	}) {
		return
	}
	document.NoConnects = append(document.NoConnects, endpoint)
}

type physicalFunctionBinding struct {
	function string
	unit     string
	required bool
}

func physicalRecordFunctions(record components.ComponentRecord, variantID string) map[string]physicalFunctionBinding {
	availablePads := map[string]bool{}
	for _, variant := range record.Packages {
		if variant.ID != variantID {
			continue
		}
		for _, pad := range variant.PadFunctions {
			availablePads[strings.ToUpper(strings.TrimSpace(pad.Function))] = true
		}
	}
	result := map[string]physicalFunctionBinding{}
	for _, symbol := range record.Symbols {
		unit := strings.TrimSpace(symbol.UnitID)
		if unit == "" && symbol.Unit != 0 {
			unit = fmt.Sprintf("%d", symbol.Unit)
		}
		for _, pin := range symbol.FunctionPins {
			key := strings.ToUpper(strings.TrimSpace(pin.Function))
			if key == "" || !availablePads[key] {
				continue
			}
			result[key] = physicalFunctionBinding{function: pin.Function, unit: unit, required: pin.Required}
		}
	}
	return result
}

func graphInstanceTerminalNode(instance GraphInstance, terminal string) (string, bool) {
	for _, connection := range instance.Terminals {
		if strings.EqualFold(connection.Terminal, terminal) && connection.Node != "" {
			return connection.Node, true
		}
	}
	return "", false
}

func synchronousBuckPhysicalOutputNode(graph CandidateGraph, switchNode string) (string, bool) {
	output := ""
	for _, instance := range graph.Instances {
		if instance.Kind != "inductor" {
			continue
		}
		nodes := []string{}
		for _, terminal := range instance.Terminals {
			if terminal.Node != "" {
				nodes = append(nodes, terminal.Node)
			}
		}
		if len(nodes) != 2 {
			continue
		}
		other := ""
		if nodes[0] == switchNode {
			other = nodes[1]
		} else if nodes[1] == switchNode {
			other = nodes[0]
		}
		if other == "" {
			continue
		}
		if output != "" && output != other {
			return "", false
		}
		output = other
	}
	return output, output != ""
}

func physicalGraphNetName(graph CandidateGraph, nodeID string) string {
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			return physicalNetName(node)
		}
	}
	return ""
}

type physicalSupportTerminal struct {
	function string
	unit     string
}

type physicalSupportSelection struct {
	record    components.ComponentRecord
	variant   components.PackageVariant
	terminals [2]physicalSupportTerminal
	valueSI   float64
}

func (selection physicalSupportSelection) key() string {
	return selection.record.ID + "|" + selection.variant.ID + "|" +
		selection.terminals[0].function + "|" + selection.terminals[1].function
}

func (selection physicalSupportSelection) endpointForFunction(component, function string) (circuitgraph.Endpoint, bool) {
	for _, terminal := range selection.terminals {
		if !strings.EqualFold(strings.TrimSpace(terminal.function), strings.TrimSpace(function)) {
			continue
		}
		return circuitgraph.Endpoint{
			Component: component, Unit: terminal.unit,
			SelectorKind: circuitgraph.SelectorFunction, Selector: terminal.function,
		}, true
	}
	return circuitgraph.Endpoint{}, false
}

func selectPhysicalSupportPrimitive(
	ctx context.Context,
	catalog *components.Catalog,
	kind string,
	packageType string,
	valueSI float64,
	minVoltageV float64,
) (physicalSupportSelection, bool) {
	valueKind, unit := "", ""
	switch kind {
	case "resistor":
		valueKind, unit = "resistance", "Ohm"
	case "capacitor":
		valueKind, unit = "capacitance", "F"
	default:
		return physicalSupportSelection{}, false
	}
	selection, result := components.Select(ctx, catalog, components.SelectionRequest{
		Query: components.Query{
			Family: kind, Package: packageType, ValueKind: valueKind,
			Value:             physicalEngineeringValue(valueSI, unit),
			MinVoltageV:       minVoltageV,
			MinimumConfidence: components.ConfidenceRuleInferred, Limit: 64,
		},
		Acceptance: components.AcceptanceFabricationCandidate, AllowAlternatives: true,
		RequireConcrete: true,
		RequiredRatings: []components.RequiredRating{{Kind: "voltage", Value: physicalEngineeringValue(minVoltageV, "V"), Unit: "V"}},
	})
	if result.OK {
		terminals, found := physicalTwoTerminalFunctions(selection.Component, selection.Variant)
		if found {
			return physicalSupportSelection{
				record: selection.Component, variant: selection.Variant,
				terminals: terminals, valueSI: valueSI,
			}, true
		}
	}
	return physicalSupportSelection{}, false
}

func physicalTwoTerminalFunctions(
	record components.ComponentRecord,
	variant components.PackageVariant,
) ([2]physicalSupportTerminal, bool) {
	padFunctions := map[string]bool{}
	for _, pad := range variant.PadFunctions {
		padFunctions[strings.ToUpper(strings.TrimSpace(pad.Function))] = true
	}
	type candidate struct {
		symbolID string
		pins     []components.FunctionPin
		unit     string
	}
	candidates := []candidate{}
	for _, symbol := range record.Symbols {
		pins := []components.FunctionPin{}
		for _, pin := range symbol.FunctionPins {
			if padFunctions[strings.ToUpper(strings.TrimSpace(pin.Function))] {
				pins = append(pins, pin)
			}
		}
		if len(pins) != 2 {
			continue
		}
		unit := strings.TrimSpace(symbol.UnitID)
		if unit == "" && symbol.Unit != 0 {
			unit = fmt.Sprintf("%d", symbol.Unit)
		}
		candidates = append(candidates, candidate{symbolID: symbol.SymbolID, pins: pins, unit: unit})
	}
	slices.SortFunc(candidates, func(left, right candidate) int {
		return cmp.Compare(left.symbolID, right.symbolID)
	})
	if len(candidates) == 0 {
		return [2]physicalSupportTerminal{}, false
	}
	selected := candidates[0]
	slices.SortFunc(selected.pins, func(left, right components.FunctionPin) int {
		return cmp.Or(
			cmp.Compare(physicalSupportPinRank(left), physicalSupportPinRank(right)),
			cmp.Compare(left.Function, right.Function),
		)
	})
	return [2]physicalSupportTerminal{
		{function: selected.pins[0].Function, unit: selected.unit},
		{function: selected.pins[1].Function, unit: selected.unit},
	}, true
}

func physicalSupportPinRank(pin components.FunctionPin) int {
	polarity := strings.ToLower(strings.TrimSpace(pin.Polarity))
	function := strings.ToUpper(strings.TrimSpace(pin.Function))
	if polarity == "positive" || function == "POSITIVE" || function == "A" || function == "1" {
		return 0
	}
	if polarity == "negative" || function == "NEGATIVE" || function == "B" || function == "2" {
		return 1
	}
	return 2
}

func physicalSupportComponent(part physicalSupportPart, selection physicalSupportSelection) circuitgraph.Component {
	component := circuitgraph.Component{
		ID: part.ID, Role: physicalComponentRole(part.Kind), Usage: "open_topology_" + part.Usage,
		ComponentID: selection.record.ID, VariantID: selection.variant.ID,
		Population: circuitgraph.PopulationPopulate,
	}
	if part.Kind == "resistor" {
		component.Value = physicalEngineeringValue(selection.valueSI, "Ohm")
	} else {
		component.Value = physicalEngineeringValue(selection.valueSI, "F")
	}
	for _, terminal := range selection.terminals {
		component.RequiredFunctions = append(component.RequiredFunctions, terminal.function)
		if terminal.unit != "" && !slices.ContainsFunc(component.Units, func(unit circuitgraph.ComponentUnit) bool {
			return unit.ID == terminal.unit
		}) {
			component.Units = append(component.Units, circuitgraph.ComponentUnit{ID: terminal.unit, Role: part.Kind})
		}
	}
	slices.Sort(component.RequiredFunctions)
	return component
}

func appendPhysicalNetEndpoint(document *circuitgraph.Document, netName string, endpoint circuitgraph.Endpoint) {
	for index := range document.Nets {
		if document.Nets[index].Name == netName {
			if slices.ContainsFunc(document.Nets[index].Endpoints, func(existing circuitgraph.Endpoint) bool {
				return existing.Component == endpoint.Component && existing.Unit == endpoint.Unit &&
					existing.SelectorKind == endpoint.SelectorKind && strings.EqualFold(existing.Selector, endpoint.Selector)
			}) {
				return
			}
			document.Nets[index].Endpoints = append(document.Nets[index].Endpoints, endpoint)
			return
		}
	}
}

func appendPhysicalRequiredFunction(document *circuitgraph.Document, componentID, function string) {
	for index := range document.Components {
		if document.Components[index].ID != componentID {
			continue
		}
		document.Components[index].RequiredFunctions = append(document.Components[index].RequiredFunctions, function)
		slices.Sort(document.Components[index].RequiredFunctions)
		document.Components[index].RequiredFunctions = slices.Compact(document.Components[index].RequiredFunctions)
		return
	}
}

func appendPhysicalSupportSchematicIntent(intent *circuitgraph.SchematicIntent, groups []physicalSupportGroup) {
	for index, group := range groups {
		if len(group.Components) == 0 {
			continue
		}
		groupID := canonicalIdentifier("support_" + group.Controller)
		intent.Groups = append(intent.Groups, circuitgraph.SchematicGroup{
			ID: groupID, Label: "Controller Support", Role: "support_stage",
			Members: append([]string(nil), group.Components...), Rank: 2 + index,
		})
		for _, component := range group.Components {
			intent.Placements = append(intent.Placements, circuitgraph.SchematicPlacement{
				Component: component, Group: groupID, Near: group.Controller,
			})
		}
	}
}

func applyPhysicalSupportPCBIntent(intent *circuitgraph.PCBIntent, groups []physicalSupportGroup) {
	for _, group := range groups {
		for index := range intent.Placements {
			placement := &intent.Placements[index]
			if !slices.Contains(group.Components, placement.Component) {
				continue
			}
			placement.Near = group.Controller
			placement.MaxDistanceMM = group.MaxDistanceMM[placement.Component]
			placement.Priority = 95
		}
	}
}
