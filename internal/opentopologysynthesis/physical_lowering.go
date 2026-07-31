package opentopologysynthesis

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/reports"
)

type physicalConnectorSelection struct {
	CatalogID      string `json:"catalog_id"`
	VariantID      string `json:"variant_id"`
	SignalFunction string `json:"signal_function"`
	ReturnFunction string `json:"return_function"`
	EvidenceSHA    string `json:"evidence_sha256"`
}

func LowerPassingCandidate(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	evaluation SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
) PhysicalLoweringResult {
	result := PhysicalLoweringResult{
		Schema:         PhysicalLoweringSchema,
		Version:        PhysicalLoweringVersion,
		PolicyVersion:  PolicyVersion,
		InventoryHash:  inventory.Hash,
		EvaluationHash: evaluation.Hash,
		Status:         PhysicalLoweringInvalid,
		Bindings:       []PhysicalSemanticBinding{},
		Issues:         []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash physical-lowering requirement: "+err.Error(), "")}
		return finalizePhysicalLowering(result)
	}
	result.RequirementHash = requirementHash
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize physical-lowering graph: "+err.Error(), "")}
		return finalizePhysicalLowering(result)
	}
	result.GraphHash, err = GraphHash(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash physical-lowering graph: "+err.Error(), "")}
		return finalizePhysicalLowering(result)
	}
	if evaluation.Status != SimulationEvaluationPassed ||
		evaluation.RequirementHash != requirementHash ||
		evaluation.InventoryHash != inventory.Hash ||
		evaluation.GraphHash != result.GraphHash ||
		evaluation.Hash == "" {
		result.Status = PhysicalLoweringUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePhysicalPromotionFailed, "evaluation", "physical lowering requires trusted passing evidence for the exact graph", "evaluate the selected graph before lowering")}
		return finalizePhysicalLowering(result)
	}
	if issues := validateSimulationEnvironment(inventory, environment); len(issues) != 0 {
		result.Status = PhysicalLoweringUnsupported
		result.Issues = issues
		return finalizePhysicalLowering(result)
	}
	connector, connectorNeeded := selectPhysicalConnector(environment.Catalog)
	if connectorNeeded == false && physicalConnectorCount(graph) != 0 {
		result.Status = PhysicalLoweringUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePrimitiveUnavailable, "physical.connector", "no reviewed two-contact connector is available for external interfaces", "onboard a reviewed connector symbol, footprint, and function-to-pad map")}
		return finalizePhysicalLowering(result)
	}
	document, bindings, lowerIssues := lowerCandidateDocument(requirement, graph, inventory, connector)
	result.Document = document
	result.Bindings = bindings
	if len(lowerIssues) != 0 {
		result.Issues = lowerIssues
		return finalizePhysicalLowering(result)
	}
	document = circuitgraph.Normalize(document)
	result.Document = document
	if issues := circuitgraph.Validate(document); len(issues) != 0 {
		result.Issues = issues
		return finalizePhysicalLowering(result)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{
		Catalog: environment.Catalog, CatalogID: "open-topology",
		CatalogHash: environment.CatalogHash,
	})
	resolved, resolveIssues := resolver.Resolve(ctx, document)
	result.Resolved = resolved
	if len(resolveIssues) != 0 {
		result.Issues = resolveIssues
		return finalizePhysicalLowering(result)
	}
	request, requestIssues := circuitgraph.ToDesignRequest(resolved)
	result.DesignRequest = request
	if len(requestIssues) != 0 {
		result.Issues = requestIssues
		return finalizePhysicalLowering(result)
	}
	result.Status = PhysicalLoweringReady
	return finalizePhysicalLowering(result)
}

func lowerCandidateDocument(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	connector physicalConnectorSelection,
) (circuitgraph.Document, []PhysicalSemanticBinding, []reports.Issue) {
	document := circuitgraph.Document{
		Schema:  circuitgraph.SchemaID,
		Version: circuitgraph.Version,
		Project: circuitgraph.Project{
			Name: requirement.Project.Name, Title: requirement.Project.Title,
			Description: requirement.Project.Description,
			Acceptance:  physicalAcceptance(requirement.Acceptance),
			Board:       physicalBoard(requirement, graph, inventory),
		},
		Components: []circuitgraph.Component{},
		Nets:       []circuitgraph.Net{},
		NoConnects: []circuitgraph.Endpoint{},
		PowerFlags: []circuitgraph.PowerFlag{},
		Buses:      []circuitgraph.Bus{},
		Policy: circuitgraph.Policy{
			AllowReferenceAssignment: graphBool(true), AllowValueNormalization: graphBool(true),
			AllowLayoutInference: graphBool(true), AllowSpacingAdjustment: graphBool(true),
			AllowLabelInsertion: graphBool(true), AllowPlacementAdjustment: graphBool(true),
			AllowRouteRetry: graphBool(true),
		},
	}
	bindings := []PhysicalSemanticBinding{}
	issues := []reports.Issue{}
	instancePrimitives := map[string]PrimitiveCandidate{}
	for _, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			issues = append(issues, graphIssue(CodePrimitiveUnavailable, "graph.instances."+instance.ID, "physical primitive is absent from the immutable inventory", "rebuild the candidate from the bound inventory"))
			continue
		}
		instancePrimitives[instance.ID] = primitive
		component := circuitgraph.Component{
			ID: instance.ID, Role: physicalComponentRole(instance.Kind),
			Usage:       "open_topology_" + instance.Kind,
			ComponentID: primitive.CatalogID, VariantID: primitive.VariantID,
			Population: circuitgraph.PopulationPopulate,
		}
		if primitive.UnitID != "" {
			component.Units = []circuitgraph.ComponentUnit{{ID: primitive.UnitID, Role: instance.Kind}}
		}
		if instance.ValueSI != nil && primitive.ValueDomain != nil {
			component.Value = physicalEngineeringValue(*instance.ValueSI, primitive.ValueDomain.Unit)
		}
		for _, terminal := range primitive.Terminals {
			component.RequiredFunctions = append(component.RequiredFunctions, terminal.Function)
		}
		slices.Sort(component.RequiredFunctions)
		document.Components = append(document.Components, component)
		bindings = append(bindings, PhysicalSemanticBinding{
			Kind: "primitive", SemanticID: instance.ID, Component: instance.ID,
			CatalogID: primitive.CatalogID, VariantID: primitive.VariantID,
			EvidenceSHA: hashJSON(struct {
				Key      string
				Evidence string
				Models   []PrimitiveModelContract
			}{primitive.Key, primitive.Evidence, primitive.Models}),
		})
		connected := map[string]bool{}
		for _, connection := range instance.Terminals {
			connected[connection.Terminal] = true
		}
		for _, terminal := range primitive.Terminals {
			if connected[terminal.Terminal] {
				continue
			}
			document.NoConnects = append(document.NoConnects, circuitgraph.Endpoint{
				Component: instance.ID, Unit: terminal.UnitID,
				SelectorKind: circuitgraph.SelectorFunction, Selector: terminal.Function,
			})
		}
	}
	referenceNode := physicalReferenceNode(graph)
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role == "reference" {
			continue
		}
		componentID := physicalInterfaceComponentID(node)
		document.Components = append(document.Components, circuitgraph.Component{
			ID: componentID, Role: physicalInterfaceRole(node),
			Usage: "external_" + node.Role, ComponentID: connector.CatalogID,
			VariantID: connector.VariantID, Population: circuitgraph.PopulationPopulate,
			RequiredFunctions: []string{connector.ReturnFunction, connector.SignalFunction},
		})
		bindings = append(bindings, PhysicalSemanticBinding{
			Kind: "external_interface", SemanticID: node.SemanticID, GraphNode: node.ID,
			Component: componentID, Function: connector.SignalFunction,
			CatalogID: connector.CatalogID, VariantID: connector.VariantID,
			EvidenceSHA: connector.EvidenceSHA,
		})
	}
	topologyNetRoles := physicalTopologyNetRoles(graph)
	for _, node := range graph.Nodes {
		netRole := physicalNetRole(node)
		if inferred := topologyNetRoles[node.ID]; inferred != "" && netRole == circuitgraph.NetRoleSignal {
			netRole = inferred
		}
		net := circuitgraph.Net{
			Name: physicalNetName(node), Role: netRole,
			Required: graphBool(true), VoltageDomain: node.Domain,
			CurrentMA: physicalNodeCurrentMA(requirement, node),
			Endpoints: []circuitgraph.Endpoint{},
		}
		for _, instance := range graph.Instances {
			primitive, found := instancePrimitives[instance.ID]
			if !found {
				continue
			}
			for _, connection := range instance.Terminals {
				if connection.Node != node.ID {
					continue
				}
				terminal, terminalFound := primitiveTerminalByName(primitive, connection.Terminal)
				if !terminalFound {
					continue
				}
				net.Endpoints = append(net.Endpoints, circuitgraph.Endpoint{
					Component: instance.ID, Unit: terminal.UnitID,
					SelectorKind: circuitgraph.SelectorFunction, Selector: terminal.Function,
				})
			}
		}
		if node.Scope == "external" && node.Role != "reference" {
			net.Endpoints = append(net.Endpoints, circuitgraph.Endpoint{
				Component:    physicalInterfaceComponentID(node),
				SelectorKind: circuitgraph.SelectorFunction, Selector: connector.SignalFunction,
			})
		}
		if node.ID == referenceNode {
			for _, external := range graph.Nodes {
				if external.Scope == "external" && external.Role != "reference" {
					net.Endpoints = append(net.Endpoints, circuitgraph.Endpoint{
						Component:    physicalInterfaceComponentID(external),
						SelectorKind: circuitgraph.SelectorFunction, Selector: connector.ReturnFunction,
					})
				}
			}
		}
		document.Nets = append(document.Nets, net)
		if node.Scope == "external" && (node.Role == "reference" || node.Role == "supply") {
			document.PowerFlags = append(document.PowerFlags, circuitgraph.PowerFlag{Net: net.Name})
		}
		bindings = append(bindings, PhysicalSemanticBinding{
			Kind: "net", SemanticID: node.SemanticID, GraphNode: node.ID,
			EvidenceSHA: hashJSON(struct {
				Node GraphNode
				Net  string
			}{node, net.Name}),
		})
	}
	document.Schematic = physicalSchematicIntent(graph)
	document.PCB = physicalPCBIntent(document.Project.Board, document.Components)
	return document, bindings, reports.SortedIssues(issues)
}

// physicalTopologyNetRoles recognizes control-loop return paths from graph
// connectivity rather than component or fixture names. A control input is a
// feedback node when a passive-only path connects it back to an output of the
// same active device. Those edges must not participate in forward-flow rank
// assignment, otherwise a normal control loop collapses into one column.
func physicalTopologyNetRoles(graph CandidateGraph) map[string]circuitgraph.NetRole {
	passiveNeighbors := map[string]map[string]struct{}{}
	for _, instance := range graph.Instances {
		if !physicalPassiveKind(instance.Kind) {
			continue
		}
		nodes := []string{}
		for _, terminal := range instance.Terminals {
			if terminal.Node != "" {
				nodes = append(nodes, terminal.Node)
			}
		}
		slices.Sort(nodes)
		nodes = slices.Compact(nodes)
		for left := 0; left < len(nodes); left++ {
			for right := left + 1; right < len(nodes); right++ {
				if passiveNeighbors[nodes[left]] == nil {
					passiveNeighbors[nodes[left]] = map[string]struct{}{}
				}
				if passiveNeighbors[nodes[right]] == nil {
					passiveNeighbors[nodes[right]] = map[string]struct{}{}
				}
				passiveNeighbors[nodes[left]][nodes[right]] = struct{}{}
				passiveNeighbors[nodes[right]][nodes[left]] = struct{}{}
			}
		}
	}
	roles := map[string]circuitgraph.NetRole{}
	for _, instance := range graph.Instances {
		var controlInputs, outputs []string
		for _, terminal := range instance.Terminals {
			switch physicalTerminalFlow(terminal.Terminal) {
			case "control_input":
				controlInputs = append(controlInputs, terminal.Node)
			case "output":
				outputs = append(outputs, terminal.Node)
			}
		}
		slices.Sort(controlInputs)
		controlInputs = slices.Compact(controlInputs)
		slices.Sort(outputs)
		outputs = slices.Compact(outputs)
		for _, input := range controlInputs {
			if input == "" {
				continue
			}
			for _, output := range outputs {
				if output != "" && input != output && physicalPassiveReachable(input, output, passiveNeighbors) {
					roles[input] = circuitgraph.NetRoleFeedback
					break
				}
			}
		}
	}
	return roles
}

func physicalPassiveKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "resistor", "capacitor", "inductor", "diode", "zener", "tvs":
		return true
	default:
		return false
	}
}

// physicalPassiveOrientations follows the conventional schematic distinction
// between rail branches and forward/feedback paths. Two-terminal passives tied
// to a supply or reference rail are vertical; passives carrying signal, bias,
// or feedback flow are horizontal. The decision depends only on graph
// topology and node roles, so new circuit families receive the same treatment
// without named templates or placement coordinates.
func physicalPassiveOrientations(graph CandidateGraph) map[string]string {
	nodesByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodesByID[node.ID] = node
	}
	inferredRoles := physicalTopologyNetRoles(graph)
	orientations := map[string]string{}
	for _, instance := range graph.Instances {
		if !physicalPassiveKind(instance.Kind) {
			continue
		}
		nodes := map[string]struct{}{}
		railBranch := false
		for _, terminal := range instance.Terminals {
			if terminal.Node == "" {
				continue
			}
			nodes[terminal.Node] = struct{}{}
			role := inferredRoles[terminal.Node]
			if role == "" {
				if node, ok := nodesByID[terminal.Node]; ok {
					role = physicalNetRole(node)
				}
			}
			switch role {
			case circuitgraph.NetRolePower, circuitgraph.NetRolePowerPos, circuitgraph.NetRolePowerNeg,
				circuitgraph.NetRoleGround, circuitgraph.NetRoleReturn:
				railBranch = true
			}
		}
		if len(nodes) < 2 {
			continue
		}
		if railBranch {
			orientations[instance.ID] = "normal"
		} else {
			orientations[instance.ID] = "rotated_90"
		}
	}
	return orientations
}

func physicalTerminalFlow(terminal string) string {
	switch strings.ToUpper(strings.TrimSpace(terminal)) {
	case "IN_PLUS", "IN_MINUS", "INPUT", "BASE", "GATE", "FB", "FEEDBACK", "SENSE":
		return "control_input"
	case "OUT", "OUTPUT", "COLLECTOR", "EMITTER", "DRAIN", "SOURCE":
		return "output"
	default:
		return ""
	}
}

func physicalPassiveReachable(start, target string, neighbors map[string]map[string]struct{}) bool {
	queue := []string{start}
	seen := map[string]struct{}{start: {}}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		next := make([]string, 0, len(neighbors[current]))
		for candidate := range neighbors[current] {
			next = append(next, candidate)
		}
		slices.Sort(next)
		for _, candidate := range next {
			if candidate == target {
				return true
			}
			if _, exists := seen[candidate]; exists {
				continue
			}
			seen[candidate] = struct{}{}
			queue = append(queue, candidate)
		}
	}
	return false
}

func selectPhysicalConnector(catalog *components.Catalog) (physicalConnectorSelection, bool) {
	type candidate struct {
		record    components.ComponentRecord
		variant   components.PackageVariant
		functions [2]string
	}
	candidates := []candidate{}
	if catalog == nil {
		return physicalConnectorSelection{}, false
	}
	for _, record := range catalog.Records {
		if record.Family != "connector" || !acceptedConfidence(record.Verification.Confidence) {
			continue
		}
		for _, variant := range record.Packages {
			if !acceptedConfidence(variant.Verification.Confidence) {
				continue
			}
			for _, symbol := range record.Symbols {
				if !acceptedConfidence(symbol.Verification.Confidence) {
					continue
				}
				functions := []string{}
				for _, pin := range symbol.FunctionPins {
					for _, pad := range variant.PadFunctions {
						if equalFold(pin.Function, pad.Function) {
							functions = append(functions, pin.Function)
							break
						}
					}
				}
				slices.Sort(functions)
				functions = slices.Compact(functions)
				if len(functions) == 2 {
					candidates = append(candidates, candidate{record: record, variant: variant, functions: [2]string{functions[0], functions[1]}})
				}
			}
		}
	}
	slices.SortFunc(candidates, func(left, right candidate) int {
		return cmp.Or(
			cmp.Compare(left.record.ID, right.record.ID),
			cmp.Compare(left.variant.ID, right.variant.ID),
			cmp.Compare(left.functions[0], right.functions[0]),
		)
	})
	if len(candidates) == 0 {
		return physicalConnectorSelection{}, false
	}
	selected := candidates[0]
	evidence := struct {
		CatalogID  string
		VariantID  string
		Footprint  string
		Functions  [2]string
		Confidence components.ConfidenceLevel
	}{
		selected.record.ID, selected.variant.ID, selected.variant.FootprintID,
		selected.functions, selected.variant.Verification.Confidence,
	}
	return physicalConnectorSelection{
		CatalogID: selected.record.ID, VariantID: selected.variant.ID,
		SignalFunction: selected.functions[0], ReturnFunction: selected.functions[1],
		EvidenceSHA: hashJSON(evidence),
	}, true
}

func physicalConnectorCount(graph CandidateGraph) int {
	count := 0
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role != "reference" {
			count++
		}
	}
	return count
}

func physicalReferenceNode(graph CandidateGraph) string {
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "reference" {
			return node.ID
		}
	}
	return ""
}

func physicalInterfaceComponentID(node GraphNode) string {
	return canonicalIdentifier("interface_" + node.SemanticID)
}

func physicalInterfaceRole(node GraphNode) circuitgraph.ComponentRole {
	if node.Role == "output" {
		return circuitgraph.RoleOutputConnector
	}
	return circuitgraph.RoleInputConnector
}

func physicalComponentRole(kind string) circuitgraph.ComponentRole {
	switch kind {
	case "resistor":
		return circuitgraph.RoleResistor
	case "capacitor":
		return circuitgraph.RoleCapacitor
	case "inductor":
		return circuitgraph.RoleInductor
	case "reference_diode", "clamp_diode", "signal_diode":
		return circuitgraph.RoleDiode
	case "npn_bjt", "pnp_bjt":
		return circuitgraph.RoleBJT
	case "n_channel_mosfet", "p_channel_mosfet":
		return circuitgraph.RoleMOSFET
	case "opamp", "comparator", "fixed_voltage_regulator", "adjustable_voltage_regulator":
		return circuitgraph.RoleIC
	default:
		return circuitgraph.RoleGeneric
	}
}

func physicalNetRole(node GraphNode) circuitgraph.NetRole {
	switch node.Role {
	case "reference":
		return circuitgraph.NetRoleGround
	case "supply":
		return circuitgraph.NetRolePowerPos
	case "feedback":
		return circuitgraph.NetRoleFeedback
	case "bias":
		return circuitgraph.NetRoleBias
	default:
		return circuitgraph.NetRoleSignal
	}
}

func physicalNetName(node GraphNode) string {
	name := node.ID
	if node.Scope == "external" && node.SemanticID != "" {
		name = node.SemanticID
	}
	return strings.ToUpper(canonicalIdentifier(name))
}

func physicalNodeCurrentMA(requirement Requirement, node GraphNode) float64 {
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == node.Domain && domain.MaxCurrentA != nil {
			return *domain.MaxCurrentA * 1000
		}
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID == node.SemanticID && port.Electrical.MaxCurrentA != nil {
			return *port.Electrical.MaxCurrentA * 1000
		}
	}
	return 0
}

func physicalEngineeringValue(value float64, unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "ohm" {
		unit = "Ohm"
	}
	return strconv.FormatFloat(value, 'g', 12, 64) + unit
}

func physicalAcceptance(acceptance Acceptance) circuitgraph.AcceptanceLevel {
	if acceptance.RequireCompleteRouting && acceptance.RequireConnectivity &&
		acceptance.RequireWriterCorrectness && acceptance.RequireRoundTripZeroDiff &&
		acceptance.RequireERC && acceptance.RequireStrictDRC {
		return circuitgraph.AcceptanceFabricationCandidate
	}
	if acceptance.RequireERC || acceptance.RequireStrictDRC {
		return circuitgraph.AcceptanceERCDRC
	}
	if acceptance.RequireConnectivity {
		return circuitgraph.AcceptanceConnectivity
	}
	return circuitgraph.AcceptanceStructural
}

func physicalBoard(requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory) circuitgraph.Board {
	area := 0.0
	for _, instance := range graph.Instances {
		if primitive, found := primitiveByKey(inventory, instance.PrimitiveKey); found {
			area += math.Max(primitive.AreaMM2, 1)
		}
	}
	side := math.Max(20, math.Sqrt(math.Max(area, 1))*6)
	width := math.Min(requirement.Requirements.Constraints.MaxWidthMM, side*1.25)
	height := math.Min(requirement.Requirements.Constraints.MaxHeightMM, side)
	return circuitgraph.Board{WidthMM: width, HeightMM: height, Layers: 2, EdgeClearanceMM: .25}
}

func physicalSchematicIntent(graph CandidateGraph) circuitgraph.SchematicIntent {
	inputs, core, outputs := []string{}, []string{}, []string{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role == "reference" {
			continue
		}
		component := physicalInterfaceComponentID(node)
		if node.Role == "output" {
			outputs = append(outputs, component)
		} else {
			inputs = append(inputs, component)
		}
	}
	for _, instance := range graph.Instances {
		core = append(core, instance.ID)
	}
	groups := []circuitgraph.SchematicGroup{}
	if len(inputs) != 0 {
		groups = append(groups, circuitgraph.SchematicGroup{ID: "external_inputs", Label: "External Inputs", Role: "input_stage", Members: inputs, Rank: 0, Side: circuitgraph.SideLeft})
	}
	if len(outputs) != 0 {
		groups = append(groups, circuitgraph.SchematicGroup{ID: "external_outputs", Label: "External Outputs", Role: "output_stage", Members: outputs, Rank: 4, Side: circuitgraph.SideRight})
	}
	coreRanks := physicalTopologyRanks(graph)
	coreByRank := map[int][]string{}
	for _, component := range core {
		rank := coreRanks[component]
		if rank < 1 || rank > 3 {
			rank = 2
		}
		coreByRank[rank] = append(coreByRank[rank], component)
	}
	for rank := 1; rank <= 3; rank++ {
		members := coreByRank[rank]
		if len(members) == 0 {
			continue
		}
		slices.Sort(members)
		groups = append(groups, circuitgraph.SchematicGroup{
			ID: "synthesized_stage_" + strconv.Itoa(rank), Label: "Synthesized Stage " + strconv.Itoa(rank),
			Role: "processing_stage", Members: members, Rank: rank,
		})
	}
	placements := []circuitgraph.SchematicPlacement{}
	passiveOrientations := physicalPassiveOrientations(graph)
	for _, group := range groups {
		for _, component := range group.Members {
			placements = append(placements, circuitgraph.SchematicPlacement{
				Component:   component,
				Group:       group.ID,
				Orientation: passiveOrientations[component],
			})
		}
	}
	return circuitgraph.SchematicIntent{
		Flow: circuitgraph.FlowLeftToRight, Origin: circuitgraph.OriginCentered,
		Groups: groups,
		Lanes: circuitgraph.SchematicLanes{
			Power: circuitgraph.LaneTop, Signals: circuitgraph.LaneMiddle, Ground: circuitgraph.LaneBottom,
		},
		Placements: placements,
		Rules: circuitgraph.SchematicRules{
			PositivePowerTop: graphBool(true), GroundBottom: graphBool(true), CenterOnPage: graphBool(true),
			PreferLabelsForLongNets: graphBool(true), AvoidWireCrossings: graphBool(true),
			MinGroupSpacingMM: 30.48, MinComponentSpacingMM: 12.7,
		},
		Hierarchy: circuitgraph.HierarchyPolicy{Mode: "flat"},
	}
}

// physicalTopologyRanks projects the bipartite instance/net graph onto the
// three conventional core columns between boundary input rank 0 and boundary
// output rank 4. The projection uses only graph distance, so it applies to new
// circuit families without named templates or instance-order assumptions.
func physicalTopologyRanks(graph CandidateGraph) map[string]int {
	neighbors := map[string]map[string]struct{}{}
	nodeKey := func(id string) string { return "node:" + id }
	instanceKey := func(id string) string { return "instance:" + id }
	connect := func(left, right string) {
		if neighbors[left] == nil {
			neighbors[left] = map[string]struct{}{}
		}
		if neighbors[right] == nil {
			neighbors[right] = map[string]struct{}{}
		}
		neighbors[left][right] = struct{}{}
		neighbors[right][left] = struct{}{}
	}
	for _, instance := range graph.Instances {
		for _, terminal := range instance.Terminals {
			connect(instanceKey(instance.ID), nodeKey(terminal.Node))
		}
	}
	var inputRoots, outputRoots []string
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role == "reference" {
			continue
		}
		if node.Role == "output" {
			outputRoots = append(outputRoots, nodeKey(node.ID))
		} else {
			inputRoots = append(inputRoots, nodeKey(node.ID))
		}
	}
	inputDistance := physicalGraphDistances(inputRoots, neighbors)
	outputDistance := physicalGraphDistances(outputRoots, neighbors)
	ranks := map[string]int{}
	for _, instance := range graph.Instances {
		key := instanceKey(instance.ID)
		fromInput, inputOK := inputDistance[key]
		toOutput, outputOK := outputDistance[key]
		switch {
		case inputOK && outputOK && fromInput+toOutput > 0:
			position := 2 * float64(fromInput) / float64(fromInput+toOutput)
			ranks[instance.ID] = 1 + int(math.Round(position))
		case inputOK:
			ranks[instance.ID] = 1
		case outputOK:
			ranks[instance.ID] = 3
		default:
			ranks[instance.ID] = 2
		}
	}
	return ranks
}

func physicalGraphDistances(roots []string, neighbors map[string]map[string]struct{}) map[string]int {
	slices.Sort(roots)
	roots = slices.Compact(roots)
	orderedNeighbors := make(map[string][]string, len(neighbors))
	for node, adjacent := range neighbors {
		ordered := make([]string, 0, len(adjacent))
		for candidate := range adjacent {
			ordered = append(ordered, candidate)
		}
		slices.Sort(ordered)
		orderedNeighbors[node] = ordered
	}
	distance := map[string]int{}
	queue := []string{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		distance[root] = 0
		queue = append(queue, root)
	}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, candidate := range orderedNeighbors[current] {
			if _, exists := distance[candidate]; exists {
				continue
			}
			distance[candidate] = distance[current] + 1
			queue = append(queue, candidate)
		}
	}
	return distance
}

func physicalPCBIntent(board circuitgraph.Board, components []circuitgraph.Component) circuitgraph.PCBIntent {
	region := circuitgraph.PCBRegion{
		ID: "synthesized_circuit", Role: "signal",
		Bounds: circuitgraph.Bounds{XMM: 0, YMM: 0, WidthMM: board.WidthMM, HeightMM: board.HeightMM},
	}
	placements := make([]circuitgraph.PCBPlacement, 0, len(components))
	for _, component := range components {
		priority := 80
		if component.Role == circuitgraph.RoleInputConnector || component.Role == circuitgraph.RoleOutputConnector {
			priority = 100
		}
		placements = append(placements, circuitgraph.PCBPlacement{Component: component.ID, Region: region.ID, Priority: priority})
	}
	return circuitgraph.PCBIntent{
		Regions: []circuitgraph.PCBRegion{region}, Placements: placements,
		Keepouts: []circuitgraph.PCBKeepout{}, Zones: []circuitgraph.PCBZone{},
	}
}

func graphBool(value bool) *bool {
	return &value
}

func finalizePhysicalLowering(result PhysicalLoweringResult) PhysicalLoweringResult {
	copy := result
	copy.Hash = ""
	result.Hash = hashJSON(copy)
	return result
}

func physicalLoweringError(path, message string) reports.Issue {
	return graphIssue(CodePhysicalPromotionFailed, path, message, "")
}
