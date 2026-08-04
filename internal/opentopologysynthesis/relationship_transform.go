package opentopologysynthesis

// relationshipActiveStage describes an active primitive inserted between two
// behavior-derived nodes. It is deliberately independent of circuit families:
// terminal names and rail attachments come from the selected primitive's
// contract, while feedback is optional for follower and controller stages.
type relationshipActiveStage struct {
	Primitive PrimitiveCandidate
	Input     TerminalConnection
	Output    TerminalConnection
	Feedback  *TerminalConnection
	Bias      []TerminalConnection
}

func addRelationshipActiveStage(
	state topologySearchState,
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	stage relationshipActiveStage,
	consumption *Consumption,
) (topologySearchState, bool) {
	if stage.Primitive.Key == "" || stage.Input.Terminal == "" || stage.Input.Node == "" ||
		stage.Output.Terminal == "" || stage.Output.Node == "" {
		return state, false
	}
	connections := []TerminalConnection{stage.Input, stage.Output}
	if stage.Feedback != nil {
		connections = append(connections, *stage.Feedback)
	}
	connections = append(connections, stage.Bias...)
	terminals := make(map[string]bool, len(connections))
	for _, connection := range connections {
		if connection.Terminal == "" || connection.Node == "" || terminals[connection.Terminal] {
			return state, false
		}
		terminals[connection.Terminal] = true
	}
	next := addRelationshipPrimitive(
		state, requirement, inventory, stage.Primitive, connections, consumption,
	)
	added := len(next.graph.Instances) == len(state.graph.Instances)+1 && next.hash != state.hash
	return next, added
}
