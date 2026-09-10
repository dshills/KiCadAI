package opentopologysynthesis

import (
	"cmp"
	"fmt"
	"slices"
)

// FeedbackBindingV22 identifies an actual signal-return connection in one exact
// graph. It is not stability evidence and does not bypass numerical admission.
type FeedbackBindingV22 struct {
	GraphHash    string `json:"graph_sha256"`
	FromInstance string `json:"from_instance"`
	FromTerminal string `json:"from_terminal"`
	ToInstance   string `json:"to_instance"`
	ToTerminal   string `json:"to_terminal"`
	Observation  string `json:"observation"`
}

// feedbackCycleGraphV22 contracts passive connectivity exactly as the frozen
// causal validator does. Removing explicitly bound control edges must break all
// active cycles; no active device, supply, or arbitrary edge is silently removed.
func feedbackCycleGraphV22(v *causalInvariantValidatorV19, removed map[string]bool) (*causalUnionV19, map[string][]string) {
	union := newCausalUnionV19(v.graph.Nodes)
	passive := map[string]bool{}
	for _, instance := range v.graph.Instances {
		if !causalPurePassiveV19(v.terminals[instance.PrimitiveKey], instance) {
			continue
		}
		passive[instance.ID] = true
		for i := 1; i < len(instance.Terminals); i++ {
			union.join(instance.Terminals[0].Node, instance.Terminals[i].Node)
		}
	}
	adjacency := map[string][]string{}
	for _, instance := range v.graph.Instances {
		if passive[instance.ID] {
			continue
		}
		inputs, outputs := []string{}, []string{}
		for _, connection := range instance.Terminals {
			role := causalTerminalRoleV19(v.terminals[instance.PrimitiveKey][connection.Terminal])
			if role == "input" && !removed[instance.ID+"\x00"+connection.Terminal] {
				inputs = append(inputs, union.find(connection.Node))
			}
			if role == "output" || role == "open_collector" || role == "power_output" {
				outputs = append(outputs, union.find(connection.Node))
			}
		}
		for _, input := range inputs {
			for _, output := range outputs {
				adjacency[input] = append(adjacency[input], output)
				if _, found := adjacency[output]; !found {
					adjacency[output] = []string{}
				}
			}
		}
	}
	for node := range adjacency {
		slices.Sort(adjacency[node])
		adjacency[node] = slices.Compact(adjacency[node])
	}
	return union, adjacency
}

func validateFeedbackBindingsV22(v *causalInvariantValidatorV19, bindings []FeedbackBindingV22) {
	graphHash, err := GraphHash(v.graph)
	if err != nil {
		v.add("feedback.graph", "feedback graph hash is invalid", "supply a canonical graph")
		return
	}
	union, adjacency := feedbackCycleGraphV22(v, nil)
	cycles := causalStrongCyclesV19(adjacency)
	removed := map[string]bool{}
	_, directed := causalGraphReachabilityV19(v.graph, v.inventory)
	for _, binding := range bindings {
		key := binding.ToInstance + "\x00" + binding.ToTerminal
		from, fromFound := v.instances[binding.FromInstance]
		to, toFound := v.instances[binding.ToInstance]
		if binding.GraphHash != graphHash || removed[key] || !fromFound || !toFound {
			v.add("feedback.binding", "feedback binding is stale, duplicate, or names an absent device", "bind each feedback control exactly once to the current graph")
			continue
		}
		fc, ft, fok := v.instanceTerminal(from, binding.FromTerminal)
		tc, tt, tok := v.instanceTerminal(to, binding.ToTerminal)
		role := causalTerminalRoleV19(ft)
		if !fok || !tok || (role != "output" && role != "open_collector" && role != "power_output") || causalTerminalRoleV19(tt) != "input" {
			v.add("feedback.terminals", "feedback must connect a catalog output to a catalog control input", "retain exact terminal-role evidence")
			continue
		}
		fn, tn := v.nodes[fc.Node], v.nodes[tc.Node]
		if fn.Role == "supply" || fn.Role == "reference" || tn.Role == "supply" || tn.Role == "reference" || (fn.Domain != "" && tn.Domain != "" && fn.Domain != tn.Domain) || (fc.Node != tc.Node && !v.passivePath(fc.Node, tc.Node, true)) {
			v.add("feedback.path", "feedback lacks a compatible direct or passive signal-return path", "do not use supply/reference paths as feedback evidence")
			continue
		}
		inCycle := false
		for _, cycle := range cycles {
			inCycle = inCycle || (cycle[union.find(fc.Node)] && cycle[union.find(tc.Node)])
		}
		observation := ""
		for _, assertion := range v.requirement.Requirements.BehavioralRequirements {
			if assertion.Observation.Kind == "port" && assertion.Observation.ID == binding.Observation {
				observation, _ = ExternalNodeForObservation(v.graph, v.requirement, assertion.Observation)
			}
		}
		if !inCycle || observation == "" || !graphPathExists(directed, fc.Node, observation) {
			v.add("feedback.observation", "feedback is not bound to an actual cycle in the required observation cone", "supply an exact observed-cycle binding")
			continue
		}
		removed[key] = true
	}
	_, opened := feedbackCycleGraphV22(v, removed)
	if len(causalStrongCyclesV19(opened)) != 0 {
		v.add("feedback.unexplained_cycle", "active causal cycle lacks complete role-bound feedback evidence", "identify every feedback control and require numerical verification")
	}
}

// deriveFeedbackBindingsV22 is deterministic structural discovery. It generates
// no numerical claim. The certificate constructor below must bind a fully
// admitted evaluation before this evidence can justify selecting a circuit.
func deriveFeedbackBindingsV22(requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory) []FeedbackBindingV22 {
	v := newCausalInvariantValidatorV19(Normalize(requirement), graph, inventory, CausalInvariantContextV19{})
	union, adjacency := feedbackCycleGraphV22(v, nil)
	cycles := causalStrongCyclesV19(adjacency)
	_, directed := causalGraphReachabilityV19(graph, inventory)
	graphHash, err := GraphHash(graph)
	if err != nil {
		return nil
	}
	result := []FeedbackBindingV22{}
	for _, to := range graph.Instances {
		for _, tc := range to.Terminals {
			if causalTerminalRoleV19(v.terminals[to.PrimitiveKey][tc.Terminal]) != "input" {
				continue
			}
			options := []FeedbackBindingV22{}
			for _, from := range graph.Instances {
				for _, fc := range from.Terminals {
					role := causalTerminalRoleV19(v.terminals[from.PrimitiveKey][fc.Terminal])
					if role != "output" && role != "open_collector" && role != "power_output" {
						continue
					}
					fn, tn := v.nodes[fc.Node], v.nodes[tc.Node]
					if fn.Role == "reference" || fn.Role == "supply" || tn.Role == "reference" || tn.Role == "supply" || (fn.Domain != "" && tn.Domain != "" && fn.Domain != tn.Domain) {
						continue
					}
					inCycle := false
					for _, cycle := range cycles {
						inCycle = inCycle || (cycle[union.find(fc.Node)] && cycle[union.find(tc.Node)])
					}
					if !inCycle || (fc.Node != tc.Node && !v.passivePath(fc.Node, tc.Node, true)) {
						continue
					}
					for _, assertion := range v.requirement.Requirements.BehavioralRequirements {
						observation, found := ExternalNodeForObservation(graph, requirement, assertion.Observation)
						if assertion.Observation.Kind == "port" && found && graphPathExists(directed, fc.Node, observation) {
							options = append(options, FeedbackBindingV22{GraphHash: graphHash, FromInstance: from.ID, FromTerminal: fc.Terminal, ToInstance: to.ID, ToTerminal: tc.Terminal, Observation: assertion.Observation.ID})
						}
					}
				}
			}
			slices.SortFunc(options, compareFeedbackBindingsV22)
			if len(options) != 0 {
				result = append(result, options[0])
			}
		}
	}
	slices.SortFunc(result, compareFeedbackBindingsV22)
	return result
}

func compareFeedbackBindingsV22(a, b FeedbackBindingV22) int {
	return cmp.Or(cmp.Compare(a.ToInstance, b.ToInstance), cmp.Compare(a.ToTerminal, b.ToTerminal), cmp.Compare(a.FromInstance, b.FromInstance), cmp.Compare(a.FromTerminal, b.FromTerminal), cmp.Compare(a.Observation, b.Observation))
}

// checkControlChangeV22 authenticates the entire edit, not merely a claimed
// after-graph digest. No extra topology, component, power, or value edits pass.
func checkControlChangeV22(before, after CandidateGraph, inventory PrimitiveInventory, change GraphChange) error {
	if change.Kind != "redirect_terminal" || change.FromValue != nil || change.ToValue != nil || change.FromNode == change.ToNode {
		return fmt.Errorf("invalid control-rebinding operation")
	}
	index := graphInstanceIndex(before, change.Primitive)
	if index < 0 {
		return fmt.Errorf("control device is absent")
	}
	primitive, found := primitiveByKey(inventory, before.Instances[index].PrimitiveKey)
	if !found {
		return fmt.Errorf("control primitive is absent")
	}
	terminal, found := primitiveTerminalByName(primitive, change.Terminal)
	if !found || causalTerminalRoleV19(terminal) != "input" {
		return fmt.Errorf("operation does not rebind a catalog input")
	}
	from, fromFound := graphNodeByID(before, change.FromNode)
	to, toFound := graphNodeByID(before, change.ToNode)
	if !fromFound || !toFound || !causalTerminalNodeCompatibleV19(terminal, to) || (from.Domain != "" && to.Domain != "" && from.Domain != to.Domain) {
		return fmt.Errorf("incompatible control-rebinding domain")
	}
	expected := CloneGraph(before)
	found = false
	for i, connection := range expected.Instances[index].Terminals {
		if connection.Terminal == change.Terminal && connection.Node == change.FromNode {
			expected.Instances[index].Terminals[i].Node = change.ToNode
			found = true
		}
	}
	if !found {
		return fmt.Errorf("claimed control binding differs from source")
	}
	want, err := GraphHash(expected)
	got, gotErr := GraphHash(after)
	if err != nil || gotErr != nil || want != got {
		return fmt.Errorf("control-rebinding graph contains unrecorded changes")
	}
	return nil
}
