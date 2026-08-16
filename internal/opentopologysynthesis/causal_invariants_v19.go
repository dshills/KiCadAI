package opentopologysynthesis

import (
	"cmp"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

// CausalFeedbackPathV19 is evidence that a future V19 repair operation
// intentionally introduced one passive feedback path. The invariant layer
// consumes this evidence but does not create or mutate graph operations.
type CausalFeedbackPathV19 struct {
	FromInstance string `json:"from_instance"`
	FromTerminal string `json:"from_terminal"`
	ToInstance   string `json:"to_instance"`
	ToTerminal   string `json:"to_terminal"`
	ObligationID string `json:"obligation_id"`
}

// CausalInvariantContextV19 carries operation evidence into the V19-only
// invariant boundary. An empty context rejects every directed causal cycle.
type CausalInvariantContextV19 struct {
	FeedbackPaths []CausalFeedbackPathV19 `json:"feedback_paths,omitempty"`
}

// ValidateCausalGraphV19 extends, but never changes, the historical complete
// graph validator. It is deliberately not called by any V18 entry point.
func ValidateCausalGraphV19(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	limits GraphLimits,
	context CausalInvariantContextV19,
) []reports.Issue {
	requirement = Normalize(requirement)
	if issues := Validate(requirement); len(issues) != 0 {
		return reports.SortedIssues(issues)
	}
	if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) != 0 {
		return reports.SortedIssues(issues)
	}
	normalized := causalOrderedGraphV19(graph)
	validator := newCausalInvariantValidatorV19(requirement, normalized, inventory, context)
	validator.validateRequirementBindings()
	validator.validatePrimitiveRegistry()
	if len(validator.issues) != 0 {
		return reports.SortedIssues(validator.issues)
	}
	validator.validateTerminalDomainsAndRatings()
	validator.validateActiveOutputContention()
	validator.validateReferenceClosure()
	validator.validateCausalCycles()
	return reports.SortedIssues(validator.issues)
}

func causalOrderedGraphV19(graph CandidateGraph) CandidateGraph {
	result := CloneGraph(graph)
	slices.SortFunc(result.Nodes, compareGraphNodes)
	for index := range result.Instances {
		slices.SortFunc(result.Instances[index].Terminals, compareTerminalConnections)
	}
	slices.SortFunc(result.Instances, func(left, right GraphInstance) int {
		return cmp.Compare(left.ID, right.ID)
	})
	return result
}

type causalInvariantValidatorV19 struct {
	requirement Requirement
	graph       CandidateGraph
	inventory   PrimitiveInventory
	context     CausalInvariantContextV19
	domains     map[string]Domain
	ports       map[string]Port
	nodes       map[string]GraphNode
	instances   map[string]GraphInstance
	primitives  map[string]PrimitiveCandidate
	terminals   map[string]map[string]PrimitiveTerminal
	passive     map[string][]string
	issues      []reports.Issue
}

func newCausalInvariantValidatorV19(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	context CausalInvariantContextV19,
) *causalInvariantValidatorV19 {
	validator := &causalInvariantValidatorV19{
		requirement: requirement,
		graph:       graph,
		inventory:   inventory,
		context:     context,
		domains:     map[string]Domain{},
		ports:       map[string]Port{},
		nodes:       map[string]GraphNode{},
		instances:   map[string]GraphInstance{},
		primitives:  map[string]PrimitiveCandidate{},
		terminals:   map[string]map[string]PrimitiveTerminal{},
		passive:     map[string][]string{},
	}
	for _, domain := range requirement.Requirements.Domains {
		validator.domains[domain.ID] = domain
	}
	for _, port := range requirement.Requirements.Ports {
		validator.ports[port.ID] = port
	}
	for _, node := range graph.Nodes {
		validator.nodes[node.ID] = node
	}
	for _, instance := range graph.Instances {
		validator.instances[instance.ID] = instance
	}
	for _, primitive := range inventory.Primitives {
		if _, exists := validator.primitives[primitive.Key]; !exists {
			validator.primitives[primitive.Key] = primitive
			validator.terminals[primitive.Key] = causalTerminalContractsV19(primitive)
		}
	}
	validator.buildPassiveAdjacency()
	return validator
}

func (validator *causalInvariantValidatorV19) buildPassiveAdjacency() {
	for _, instance := range validator.graph.Instances {
		if !causalPurePassiveV19(validator.terminals[instance.PrimitiveKey], instance) {
			continue
		}
		instanceVertex := "instance:" + instance.ID
		for _, connection := range instance.Terminals {
			nodeVertex := "node:" + connection.Node
			validator.passive[nodeVertex] = append(validator.passive[nodeVertex], instanceVertex)
			validator.passive[instanceVertex] = append(validator.passive[instanceVertex], nodeVertex)
		}
	}
	for vertex := range validator.passive {
		slices.Sort(validator.passive[vertex])
		validator.passive[vertex] = slices.Compact(validator.passive[vertex])
	}
}

func (validator *causalInvariantValidatorV19) add(path, message, suggestion string) {
	validator.issues = append(validator.issues, graphIssue(CodeNoCompleteGraph, path, message, suggestion))
}

func (validator *causalInvariantValidatorV19) validateRequirementBindings() {
	referenceNodes := map[string]int{}
	for index, node := range validator.graph.Nodes {
		if node.Scope != "external" {
			continue
		}
		path := fmt.Sprintf("nodes[%d]", index)
		switch node.SemanticKind {
		case "port":
			port, exists := validator.ports[node.SemanticID]
			if !exists {
				validator.add(path+".semantic_id", "external graph node does not map to a declared port", "rebuild the graph from the normalized requirement")
				continue
			}
			if node.Domain != port.Domain || node.Role != graphRoleForPort(port) {
				validator.add(path, "external graph node changes its declared port domain or role", "preserve the requirement port identity exactly")
			}
		case "domain":
			domain, exists := validator.domains[node.SemanticID]
			if !exists || node.Domain != domain.ID || node.Role != domain.Kind || domain.Source != "external" {
				validator.add(path, "external source-domain node does not match a declared external domain", "rebuild the graph from the normalized requirement")
			}
		}
		if node.Role == "reference" {
			referenceNodes[node.Domain]++
		}
	}
	for _, port := range validator.requirement.Requirements.Ports {
		if port.Kind == "reference" {
			referenceNodes[port.Domain] += 0
		}
	}
	for _, domain := range validator.requirement.Requirements.Domains {
		if domain.Kind == "reference" && referenceNodes[domain.ID] != 1 {
			validator.add("domains."+domain.ID, "declared reference domain must map to exactly one external graph reference", "preserve one explicit reference node per declared reference domain")
		}
	}
}

func (validator *causalInvariantValidatorV19) validatePrimitiveRegistry() {
	if validator.inventory.Schema != PrimitiveInventorySchema || validator.inventory.Version != PrimitiveInventoryVersion {
		validator.add("inventory.schema", "V19 requires the trusted primitive-inventory schema and version", "rebuild the inventory from the reviewed catalog and model registry")
	}
	for path, value := range map[string]string{
		"inventory.catalog_hash":            validator.inventory.CatalogHash,
		"inventory.model_registry_hash":     validator.inventory.ModelRegistryHash,
		"inventory.primitive_registry_hash": validator.inventory.PrimitiveRegistry,
		"inventory.hash":                    validator.inventory.Hash,
	} {
		if !causalSHA256V19(value) {
			validator.add(path, "V19 requires a canonical SHA-256 inventory binding", "rebuild the inventory from authenticated inputs")
		}
	}
	if validator.inventory.PrimitiveRegistry != primitiveRegistryHash() {
		validator.add("inventory.primitive_registry_hash", "inventory is not bound to the running trusted primitive registry", "use the registry bound by the committed evaluator")
	}
	if computed, err := primitiveInventoryHash(validator.inventory); err != nil || computed != validator.inventory.Hash {
		validator.add("inventory.hash", "primitive inventory content does not match its authenticated hash", "rebuild the inventory rather than editing it")
	}

	seen := map[string]bool{}
	for index, primitive := range validator.inventory.Primitives {
		path := fmt.Sprintf("inventory.primitives[%d]", index)
		if seen[primitive.Key] {
			validator.add(path+".key", "primitive inventory key must be unique", "remove the ambiguous registry entry")
		}
		seen[primitive.Key] = true
	}

	requiredAnalyses := causalRequiredAnalysesV19(validator.requirement)
	descriptors := map[string]simmodel.PrimitiveDescriptor{}
	for _, descriptor := range simmodel.PrimitiveDescriptors() {
		descriptors[descriptor.ID] = descriptor
	}
	used := make([]GraphInstance, len(validator.graph.Instances))
	copy(used, validator.graph.Instances)
	slices.SortFunc(used, func(left, right GraphInstance) int { return cmp.Compare(left.ID, right.ID) })
	for _, instance := range used {
		primitive, exists := validator.primitives[instance.PrimitiveKey]
		if !exists {
			continue
		}
		path := "instances." + instance.ID
		if strings.TrimSpace(primitive.Key) == "" || strings.TrimSpace(primitive.CatalogID) == "" ||
			strings.TrimSpace(primitive.VariantID) == "" || strings.TrimSpace(primitive.Kind) == "" ||
			strings.TrimSpace(primitive.Evidence) == "" || strings.TrimSpace(primitive.FootprintID) == "" ||
			strings.TrimSpace(primitive.PackageType) == "" || len(primitive.SymbolIDs) == 0 {
			validator.add(path+".primitive_key", "primitive lacks complete catalog-backed physical identity", "use a reviewed inventory primitive with symbol, footprint, package, and evidence")
		}
		seenTerminals := map[string]bool{}
		inputs := 0
		outputs := 0
		for _, terminal := range primitive.Terminals {
			terminalPath := path + ".terminals." + terminal.Terminal
			if terminal.Terminal == "" || seenTerminals[terminal.Terminal] {
				validator.add(terminalPath, "primitive terminal contract is empty or ambiguous", "use a unique reviewed terminal contract")
			}
			seenTerminals[terminal.Terminal] = true
			if terminal.DefaultNet != "" {
				continue
			}
			if terminal.Function == "" || terminal.SymbolID == "" || terminal.SymbolPin == "" || terminal.Pad == "" {
				validator.add(terminalPath, "primitive terminal lacks physical function/pin/pad evidence", "complete the reviewed terminal contract")
			}
			role := causalTerminalRoleV19(terminal)
			if role == "unknown" {
				validator.add(terminalPath+".electrical", "primitive terminal lacks a supported causal electrical role", "add reviewed input, output, open-collector, power, passive, bidirectional, or no-connect metadata")
			}
			if role == "input" {
				inputs++
			}
			if role == "output" || role == "open_collector" || role == "power_output" {
				outputs++
			}
		}
		if inputs > 0 && outputs == 0 {
			validator.add(path+".primitive_key", "active primitive contract has causal inputs but no classified output", "complete the reviewed output-terminal electrical metadata")
		}
		if len(primitive.Models) == 0 {
			validator.add(path+".primitive_key", "primitive has no reviewed simulation model", "use a primitive with bound model provenance")
		}
		for modelIndex, model := range primitive.Models {
			modelPath := fmt.Sprintf("%s.models[%d]", path, modelIndex)
			if model.ModelID == "" || model.Family == "" || model.ProvenanceSource == "" || model.ProvenanceRevision == "" ||
				!causalSHA256V19(model.ProvenanceSHA256) || len(model.AllowedAnalyses) == 0 {
				validator.add(modelPath, "primitive model lacks complete reviewed provenance", "bind a reviewed model source, revision, digest, and analysis set")
			}
			descriptor, exists := descriptors[model.ModelID]
			if !exists {
				validator.add(modelPath+".model_id", "primitive model is absent from the trusted solver registry", "use only a registered reviewed primitive model")
				continue
			}
			for _, terminalID := range descriptor.Terminals {
				if !seenTerminals[terminalID] && descriptor.TerminalDefaults[terminalID] == "" {
					validator.add(modelPath+".model_id", "physical terminal contract does not cover trusted model terminal "+terminalID, "complete the catalog-to-model terminal binding")
				}
			}
		}
		for _, analysis := range requiredAnalyses {
			supported := false
			for _, model := range primitive.Models {
				if reviewedPrimitiveModelSupportsCircuitAnalysis(model, analysis) {
					supported = true
					break
				}
			}
			if !supported {
				validator.add(path+".models", "primitive has no reviewed model for required analysis "+analysis, "select only registry primitives covering every authored analysis")
			}
		}
	}
}

func (validator *causalInvariantValidatorV19) validateTerminalDomainsAndRatings() {
	for _, instance := range validator.graph.Instances {
		primitive := validator.primitives[instance.PrimitiveKey]
		terminalContracts := validator.terminals[instance.PrimitiveKey]
		minimum := math.Inf(1)
		maximum := math.Inf(-1)
		knownVoltages := 0
		for _, connection := range instance.Terminals {
			terminal := terminalContracts[connection.Terminal]
			node := validator.nodes[connection.Node]
			role := causalTerminalRoleV19(terminal)
			if role == "power_input" && node.Role != "supply" && node.Role != "reference" {
				validator.add("instances."+instance.ID+".terminals."+connection.Terminal, "power-input terminal is not bound to a declared supply or reference", "bind power terminals only to compatible external power resources")
			}
			if (role == "output" || role == "open_collector" || role == "power_output") && node.Role == "reference" {
				validator.add("instances."+instance.ID+".terminals."+connection.Terminal, "driven terminal may not drive an external reference", "use the declared reference only as a return")
			}
			low, high, found := validator.nodeVoltageEnvelope(node)
			if found {
				minimum = math.Min(minimum, low)
				maximum = math.Max(maximum, high)
				knownVoltages++
			}
			if role == "output" || role == "open_collector" || role == "power_output" {
				requiredCurrent := validator.nodeOutputCurrent(node)
				if requiredCurrent > 0 {
					available, found := causalRatingMaximumV19(primitive.Ratings, "current")
					if !found || available+1e-15 < requiredCurrent {
						validator.add("instances."+instance.ID+".ratings", fmt.Sprintf("driver current rating %.12g A does not cover declared %.12g A output demand", available, requiredCurrent), "select a registry primitive with sufficient current rating")
					}
				}
			}
		}
		if knownVoltages >= 2 && maximum > minimum {
			span := maximum - minimum
			available, found := causalRatingMaximumV19(primitive.Ratings, "voltage")
			if !found || available+1e-12 < span {
				validator.add("instances."+instance.ID+".ratings", fmt.Sprintf("voltage rating %.12g V does not cover declared %.12g V terminal span", available, span), "select a registry primitive rated for the complete declared domain envelope")
			}
		}
	}
}

func (validator *causalInvariantValidatorV19) validateActiveOutputContention() {
	type driver struct {
		instance string
		terminal string
		role     string
	}
	drivers := map[string][]driver{}
	for _, instance := range validator.graph.Instances {
		contracts := validator.terminals[instance.PrimitiveKey]
		for _, connection := range instance.Terminals {
			role := causalTerminalRoleV19(contracts[connection.Terminal])
			if role == "output" || role == "open_collector" || role == "power_output" {
				drivers[connection.Node] = append(drivers[connection.Node], driver{instance: instance.ID, terminal: connection.Terminal, role: role})
			}
		}
	}
	nodes := make([]string, 0, len(drivers))
	for node := range drivers {
		nodes = append(nodes, node)
	}
	slices.Sort(nodes)
	for _, node := range nodes {
		nodeDrivers := drivers[node]
		slices.SortFunc(nodeDrivers, func(left, right driver) int {
			return cmp.Or(cmp.Compare(left.instance, right.instance), cmp.Compare(left.terminal, right.terminal))
		})
		allOpenCollector := true
		for _, item := range nodeDrivers {
			allOpenCollector = allOpenCollector && item.role == "open_collector"
		}
		if len(nodeDrivers) > 1 && !allOpenCollector {
			validator.add("nodes."+node, "node has multiple active push-pull or mixed-mode output drivers", "allocate independent observation cones instead of shorting active outputs")
			continue
		}
		if allOpenCollector && !validator.passivePathToRole(node, "supply") {
			validator.add("nodes."+node, "open-collector output has no registry-backed passive pull resource", "connect a rated passive pull to a compatible declared supply")
		}
	}
}

func (validator *causalInvariantValidatorV19) validateReferenceClosure() {
	adjacency, instanceNodes := validator.signalAdjacency()
	instancesByNode := map[string][]string{}
	for instanceID, nodes := range instanceNodes {
		for _, node := range nodes {
			instancesByNode[node] = append(instancesByNode[node], instanceID)
		}
	}
	for node := range instancesByNode {
		slices.Sort(instancesByNode[node])
		instancesByNode[node] = slices.Compact(instancesByNode[node])
	}
	visited := map[string]bool{}
	nodeIDs := make([]string, 0, len(validator.graph.Nodes))
	for _, node := range validator.graph.Nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	slices.Sort(nodeIDs)
	for _, start := range nodeIDs {
		if visited[start] {
			continue
		}
		component := []string{}
		queue := []string{start}
		visited[start] = true
		for len(queue) != 0 {
			current := queue[0]
			queue = queue[1:]
			if _, exists := validator.nodes[current]; exists {
				component = append(component, current)
			}
			for _, next := range adjacency[current] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		slices.Sort(component)
		declared := map[string]bool{}
		physical := map[string]bool{}
		hasSignalPort := false
		participatingInstances := map[string]bool{}
		for _, nodeID := range component {
			node := validator.nodes[nodeID]
			for _, instanceID := range instancesByNode[nodeID] {
				participatingInstances[instanceID] = true
			}
			if node.Scope == "external" && node.SemanticKind == "port" {
				port := validator.ports[node.SemanticID]
				if port.Kind != "power" && port.Kind != "reference" {
					hasSignalPort = true
					if domain, exists := validator.domains[port.Domain]; exists && domain.Kind == "reference" {
						declared[port.Domain] = true
					} else {
						validator.add("nodes."+node.ID, "signal port is not referenced to a declared reference domain", "bind analog/control ports to an explicit compatible reference")
					}
				}
			}
			if node.Role == "reference" {
				physical[node.Domain] = true
			}
		}
		if !hasSignalPort {
			continue
		}
		instanceIDs := make([]string, 0, len(participatingInstances))
		for instanceID := range participatingInstances {
			instanceIDs = append(instanceIDs, instanceID)
		}
		slices.Sort(instanceIDs)
		for _, instanceID := range instanceIDs {
			instance := validator.instances[instanceID]
			contracts := validator.terminals[instance.PrimitiveKey]
			for _, connection := range instance.Terminals {
				if causalTerminalRoleV19(contracts[connection.Terminal]) == "power_input" {
					node := validator.nodes[connection.Node]
					if node.Role == "reference" {
						physical[node.Domain] = true
					}
				}
			}
		}
		if len(declared) != 1 {
			validator.add("nodes."+start, "causal signal cone crosses or lacks a unique declared reference domain", "split incompatible domains through an explicitly modeled isolation boundary")
			continue
		}
		if len(physical) != 1 {
			validator.add("nodes."+start, "causal signal cone has a floating or multiply merged physical reference", "bind the cone to exactly one compatible external reference")
			continue
		}
		declaredID := causalOnlyKeyV19(declared)
		physicalID := causalOnlyKeyV19(physical)
		if declaredID != physicalID {
			validator.add("nodes."+start, "causal signal cone physical reference does not match its declared domain", "bind the cone to its declared external reference")
		}
	}
}

func (validator *causalInvariantValidatorV19) validateCausalCycles() {
	union := newCausalUnionV19(validator.graph.Nodes)
	passiveInstances := map[string]bool{}
	for _, instance := range validator.graph.Instances {
		if causalPurePassiveV19(validator.terminals[instance.PrimitiveKey], instance) {
			passiveInstances[instance.ID] = true
			if len(instance.Terminals) > 1 {
				first := instance.Terminals[0].Node
				for _, connection := range instance.Terminals[1:] {
					union.join(first, connection.Node)
				}
			}
		}
	}
	adjacency := map[string][]string{}
	for _, instance := range validator.graph.Instances {
		if passiveInstances[instance.ID] {
			continue
		}
		contracts := validator.terminals[instance.PrimitiveKey]
		inputs := []string{}
		outputs := []string{}
		for _, connection := range instance.Terminals {
			switch causalTerminalRoleV19(contracts[connection.Terminal]) {
			case "input":
				inputs = append(inputs, union.find(connection.Node))
			case "output", "open_collector", "power_output":
				outputs = append(outputs, union.find(connection.Node))
			}
		}
		slices.Sort(inputs)
		inputs = slices.Compact(inputs)
		slices.Sort(outputs)
		outputs = slices.Compact(outputs)
		for _, input := range inputs {
			for _, output := range outputs {
				adjacency[input] = append(adjacency[input], output)
				if _, exists := adjacency[output]; !exists {
					adjacency[output] = []string{}
				}
			}
		}
	}
	for node := range adjacency {
		slices.Sort(adjacency[node])
		adjacency[node] = slices.Compact(adjacency[node])
	}
	cycles := causalStrongCyclesV19(adjacency)
	feedback := validator.validatedFeedbackPaths(union, adjacency)
	usedFeedback := map[int]bool{}
	usedObservations := map[string]bool{}
	for _, cycle := range cycles {
		matches := []int{}
		for index, item := range feedback {
			if cycle[item.fromRoot] && cycle[item.toRoot] {
				matches = append(matches, index)
			}
		}
		if len(matches) != 1 {
			validator.add("graph.causal_cycle", "directed causal cycle lacks exactly one valid typed passive feedback path", "remove the cycle or provide one obligation-bound feedback operation")
			continue
		}
		index := matches[0]
		usedFeedback[index] = true
		observation := feedback[index].observation
		if usedObservations[observation] {
			validator.add("context.feedback_paths", "one observation cone contains more than one typed feedback path", "retain at most one non-nested feedback path per observation cone")
		}
		usedObservations[observation] = true
	}
	for index := range feedback {
		if !usedFeedback[index] {
			validator.add("context.feedback_paths", "typed feedback evidence does not correspond to a directed causal cycle", "remove stale operation evidence")
		}
	}
}

type causalValidatedFeedbackV19 struct {
	fromRoot    string
	toRoot      string
	observation string
}

func (validator *causalInvariantValidatorV19) validatedFeedbackPaths(
	union *causalUnionV19,
	causalAdjacency map[string][]string,
) []causalValidatedFeedbackV19 {
	paths := slices.Clone(validator.context.FeedbackPaths)
	slices.SortFunc(paths, func(left, right CausalFeedbackPathV19) int {
		return cmp.Or(
			cmp.Compare(left.FromInstance, right.FromInstance),
			cmp.Compare(left.FromTerminal, right.FromTerminal),
			cmp.Compare(left.ToInstance, right.ToInstance),
			cmp.Compare(left.ToTerminal, right.ToTerminal),
			cmp.Compare(left.ObligationID, right.ObligationID),
		)
	})
	result := []causalValidatedFeedbackV19{}
	seen := map[string]bool{}
	type endpointPair struct {
		from string
		to   string
	}
	passiveReachability := map[endpointPair]bool{}
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range validator.requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	for index, path := range paths {
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", path.FromInstance, path.FromTerminal, path.ToInstance, path.ToTerminal, path.ObligationID)
		if seen[key] {
			validator.add(fmt.Sprintf("context.feedback_paths[%d]", index), "typed feedback evidence must be unique", "remove duplicate operation evidence")
			continue
		}
		seen[key] = true
		fromInstance, fromOK := validator.instances[path.FromInstance]
		toInstance, toOK := validator.instances[path.ToInstance]
		if !fromOK || !toOK {
			validator.add(fmt.Sprintf("context.feedback_paths[%d]", index), "typed feedback evidence names an unknown instance", "bind evidence to the canonical graph")
			continue
		}
		fromConnection, fromTerminal, fromOK := validator.instanceTerminal(fromInstance, path.FromTerminal)
		toConnection, toTerminal, toOK := validator.instanceTerminal(toInstance, path.ToTerminal)
		if !fromOK || !toOK || (causalTerminalRoleV19(fromTerminal) != "output" && causalTerminalRoleV19(fromTerminal) != "open_collector" && causalTerminalRoleV19(fromTerminal) != "power_output") || causalTerminalRoleV19(toTerminal) != "input" {
			validator.add(fmt.Sprintf("context.feedback_paths[%d]", index), "typed feedback endpoints must run from a classified output to a classified input", "record the exact loop-break terminal pair")
			continue
		}
		fromNode := validator.nodes[fromConnection.Node]
		toNode := validator.nodes[toConnection.Node]
		if fromNode.Role == "supply" || fromNode.Role == "reference" || toNode.Role == "supply" || toNode.Role == "reference" {
			validator.add(fmt.Sprintf("context.feedback_paths[%d]", index), "typed feedback endpoints may not be supply/reference nodes", "bind feedback only within one causal observation cone")
			continue
		}
		assertion, exists := assertions[path.ObligationID]
		if !exists || !causalFeedbackSensitiveMetricV19(assertion.Metric) || assertion.Observation.Kind != "port" {
			validator.add(fmt.Sprintf("context.feedback_paths[%d].obligation_id", index), "typed feedback is not bound to a feedback-sensitive external obligation", "use a hysteresis, stability, closed-loop, or directional-threshold obligation")
			continue
		}
		endpoints := endpointPair{from: fromConnection.Node, to: toConnection.Node}
		reachable, checked := passiveReachability[endpoints]
		if !checked {
			reachable = validator.passivePath(endpoints.from, endpoints.to, true)
			passiveReachability[endpoints] = reachable
		}
		if !reachable {
			validator.add(fmt.Sprintf("context.feedback_paths[%d]", index), "typed feedback endpoints lack a passive path that excludes supply/reference nodes", "insert at least one registry-backed passive feedback element")
			continue
		}
		fromRoot := union.find(fromConnection.Node)
		toRoot := union.find(toConnection.Node)
		observationNode := ""
		for _, node := range validator.graph.Nodes {
			if node.Scope == "external" && node.SemanticKind == "port" && node.SemanticID == assertion.Observation.ID {
				observationNode = union.find(node.ID)
				break
			}
		}
		if observationNode == "" || !graphPathExists(causalAdjacency, fromRoot, observationNode) {
			validator.add(fmt.Sprintf("context.feedback_paths[%d].obligation_id", index), "typed feedback output does not causally reach its bound observation", "bind feedback evidence to the affected observation cone")
			continue
		}
		result = append(result, causalValidatedFeedbackV19{fromRoot: fromRoot, toRoot: toRoot, observation: assertion.Observation.ID})
	}
	return result
}

func (validator *causalInvariantValidatorV19) instanceTerminal(instance GraphInstance, terminalID string) (TerminalConnection, PrimitiveTerminal, bool) {
	contracts := validator.terminals[instance.PrimitiveKey]
	for _, connection := range instance.Terminals {
		if connection.Terminal == terminalID {
			terminal, exists := contracts[terminalID]
			return connection, terminal, exists
		}
	}
	return TerminalConnection{}, PrimitiveTerminal{}, false
}

func (validator *causalInvariantValidatorV19) signalAdjacency() (map[string][]string, map[string][]string) {
	adjacency := map[string][]string{}
	instanceNodes := map[string][]string{}
	for _, node := range validator.graph.Nodes {
		adjacency[node.ID] = []string{}
	}
	for _, instance := range validator.graph.Instances {
		contracts := validator.terminals[instance.PrimitiveKey]
		nodes := []string{}
		for _, connection := range instance.Terminals {
			role := causalTerminalRoleV19(contracts[connection.Terminal])
			if role != "power_input" && role != "ignore" && role != "unknown" {
				nodes = append(nodes, connection.Node)
			}
		}
		slices.Sort(nodes)
		nodes = slices.Compact(nodes)
		instanceNodes[instance.ID] = nodes
		instanceVertex := "\x00instance:" + instance.ID
		adjacency[instanceVertex] = []string{}
		for _, node := range nodes {
			adjacency[node] = append(adjacency[node], instanceVertex)
			adjacency[instanceVertex] = append(adjacency[instanceVertex], node)
		}
	}
	for node := range adjacency {
		slices.Sort(adjacency[node])
		adjacency[node] = slices.Compact(adjacency[node])
	}
	return adjacency, instanceNodes
}

func (validator *causalInvariantValidatorV19) passivePathToRole(start, role string) bool {
	return validator.passivePathTo(
		start,
		func(node GraphNode) bool { return node.Role == role },
		func(node GraphNode) bool { return node.ID != start && node.Role == "reference" },
	)
}

func (validator *causalInvariantValidatorV19) passivePath(start, target string, excludePower bool) bool {
	return validator.passivePathTo(start, func(node GraphNode) bool { return node.ID == target }, func(node GraphNode) bool {
		return excludePower && node.ID != start && node.ID != target && (node.Role == "supply" || node.Role == "reference")
	})
}

func (validator *causalInvariantValidatorV19) passivePathTo(
	start string,
	target func(GraphNode) bool,
	excluded ...func(GraphNode) bool,
) bool {
	type state struct {
		vertex      string
		usedPassive bool
	}
	queue := []state{{vertex: "node:" + start}}
	visited := map[state]bool{queue[0]: true}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if strings.HasPrefix(current.vertex, "node:") {
			node := validator.nodes[strings.TrimPrefix(current.vertex, "node:")]
			blocked := false
			for _, reject := range excluded {
				blocked = blocked || reject(node)
			}
			if blocked {
				continue
			}
			if current.usedPassive && target(node) {
				return true
			}
		}
		for _, next := range validator.passive[current.vertex] {
			nextState := state{vertex: next, usedPassive: current.usedPassive || strings.HasPrefix(next, "instance:")}
			if !visited[nextState] {
				visited[nextState] = true
				queue = append(queue, nextState)
			}
		}
	}
	return false
}

func (validator *causalInvariantValidatorV19) nodeVoltageEnvelope(node GraphNode) (float64, float64, bool) {
	domain, domainFound := validator.domains[node.Domain]
	minimum, maximum := (*float64)(nil), (*float64)(nil)
	if node.Scope == "external" && node.SemanticKind == "port" {
		port, exists := validator.ports[node.SemanticID]
		if exists {
			minimum = port.Electrical.MinVoltageV
			maximum = port.Electrical.MaxVoltageV
			if minimum == nil {
				minimum = port.Electrical.NominalVoltageV
			}
			if maximum == nil {
				maximum = port.Electrical.NominalVoltageV
			}
		}
	}
	if domainFound {
		if minimum == nil {
			minimum = domain.MinVoltageV
			if minimum == nil {
				minimum = domain.NominalVoltageV
			}
		}
		if maximum == nil {
			maximum = domain.MaxVoltageV
			if maximum == nil {
				maximum = domain.NominalVoltageV
			}
		}
	}
	if minimum == nil || maximum == nil || !finite(*minimum) || !finite(*maximum) {
		return 0, 0, false
	}
	return math.Min(*minimum, *maximum), math.Max(*minimum, *maximum), true
}

func (validator *causalInvariantValidatorV19) nodeOutputCurrent(node GraphNode) float64 {
	if node.Scope != "external" || node.SemanticKind != "port" {
		return 0
	}
	port, exists := validator.ports[node.SemanticID]
	if !exists || port.Direction != "source" {
		return 0
	}
	if port.Electrical.MaxCurrentA != nil {
		return math.Abs(*port.Electrical.MaxCurrentA)
	}
	if domain, found := validator.domains[port.Domain]; found && domain.MaxCurrentA != nil {
		return math.Abs(*domain.MaxCurrentA)
	}
	return 0
}

func causalRequiredAnalysesV19(requirement Requirement) []string {
	analyses := []string{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		analyses = append(analyses, assertion.Analysis)
	}
	slices.Sort(analyses)
	return slices.Compact(analyses)
}

func causalTerminalContractsV19(primitive PrimitiveCandidate) map[string]PrimitiveTerminal {
	result := make(map[string]PrimitiveTerminal, len(primitive.Terminals))
	for _, terminal := range primitive.Terminals {
		result[terminal.Terminal] = terminal
	}
	return result
}

func causalTerminalRoleV19(terminal PrimitiveTerminal) string {
	switch strings.ToLower(strings.TrimSpace(terminal.Electrical)) {
	case "input":
		return "input"
	case "output":
		return "output"
	case "open_collector", "open_drain":
		return "open_collector"
	case "power_in":
		return "power_input"
	case "power_out":
		return "power_output"
	case "passive", "bidirectional":
		return "passive"
	case "no_connect":
		return "ignore"
	default:
		return "unknown"
	}
}

func causalPurePassiveV19(contracts map[string]PrimitiveTerminal, instance GraphInstance) bool {
	if len(instance.Terminals) < 2 {
		return false
	}
	for _, connection := range instance.Terminals {
		if causalTerminalRoleV19(contracts[connection.Terminal]) != "passive" {
			return false
		}
	}
	return true
}

func causalFeedbackSensitiveMetricV19(metric string) bool {
	metric = strings.ToLower(strings.TrimSpace(metric))
	return strings.Contains(metric, "hysteresis") || strings.Contains(metric, "stability") ||
		strings.Contains(metric, "phase_margin") || strings.Contains(metric, "loop_gain") ||
		strings.Contains(metric, "closed_loop_gain") || strings.Contains(metric, "threshold")
}

func causalRatingMaximumV19(bounds []PrimitiveBound, quantity string) (float64, bool) {
	maximum := 0.0
	found := false
	for _, bound := range bounds {
		if !causalRatingKindV19(bound.Kind, quantity) {
			continue
		}
		value := math.Abs(boundMaximum(bound))
		if finite(value) && value > 0 {
			maximum = math.Max(maximum, value)
			found = true
		}
	}
	return maximum, found
}

func causalRatingKindV19(kind, quantity string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch quantity {
	case "voltage":
		return slices.Contains([]string{
			"voltage", "working_voltage", "supply_voltage", "input_voltage",
			"reverse_voltage", "drain_source_voltage", "collector_emitter_voltage",
			"contact_voltage_dc", "coil_voltage", "gate_source_voltage",
			"output_voltage", "max_voltage",
		}, kind)
	case "current":
		return slices.Contains([]string{
			"current", "rated_current", "output_current", "output_sink_current",
			"forward_current", "collector_current", "drain_current",
			"contact_current_dc", "pulse_current",
		}, kind)
	default:
		return false
	}
}

func causalSHA256V19(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func causalOnlyKeyV19(values map[string]bool) string {
	for value := range values {
		return value
	}
	return ""
}

type causalUnionV19 struct {
	parent map[string]string
}

func newCausalUnionV19(nodes []GraphNode) *causalUnionV19 {
	result := &causalUnionV19{parent: map[string]string{}}
	for _, node := range nodes {
		result.parent[node.ID] = node.ID
	}
	return result
}

func (union *causalUnionV19) find(value string) string {
	parent, exists := union.parent[value]
	if !exists {
		union.parent[value] = value
		return value
	}
	if parent != value {
		union.parent[value] = union.find(parent)
	}
	return union.parent[value]
}

func (union *causalUnionV19) join(left, right string) {
	left = union.find(left)
	right = union.find(right)
	if left == right {
		return
	}
	if left < right {
		union.parent[right] = left
	} else {
		union.parent[left] = right
	}
}

func causalStrongCyclesV19(adjacency map[string][]string) []map[string]bool {
	index := 0
	indices := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	stack := []string{}
	result := []map[string]bool{}
	nodes := make([]string, 0, len(adjacency))
	for node := range adjacency {
		nodes = append(nodes, node)
	}
	slices.Sort(nodes)
	var visit func(string)
	visit = func(node string) {
		indices[node] = index
		low[node] = index
		index++
		stack = append(stack, node)
		onStack[node] = true
		for _, next := range adjacency[node] {
			if _, seen := indices[next]; !seen {
				visit(next)
				low[node] = min(low[node], low[next])
			} else if onStack[next] {
				low[node] = min(low[node], indices[next])
			}
		}
		if low[node] != indices[node] {
			return
		}
		component := map[string]bool{}
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component[last] = true
			if last == node {
				break
			}
		}
		cyclic := len(component) > 1
		if !cyclic {
			for member := range component {
				cyclic = slices.Contains(adjacency[member], member)
			}
		}
		if cyclic {
			result = append(result, component)
		}
	}
	for _, node := range nodes {
		if _, seen := indices[node]; !seen {
			visit(node)
		}
	}
	slices.SortFunc(result, func(left, right map[string]bool) int {
		return cmp.Compare(causalMapHeadV19(left), causalMapHeadV19(right))
	})
	return result
}

func causalMapHeadV19(values map[string]bool) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}
