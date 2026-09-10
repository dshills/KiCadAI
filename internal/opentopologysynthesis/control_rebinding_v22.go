package opentopologysynthesis

import (
	"context"
	"fmt"
	"slices"
)

// controlRebindingV22 is a proposal, never electrical pass evidence. The
// version-isolated continuation must admit and simulate its exact graph.
type controlRebindingV22 struct {
	graph    CandidateGraph
	change   GraphChange
	hash     string
	bindings []FeedbackBindingV22
}

type controlRebindingBatchV22 struct {
	candidates []controlRebindingV22
	work       int
	exhausted  bool
}

// controlRebindingsV22 enumerates single input-terminal edits using catalog
// roles, not component kinds or terminal names. Connecting an input to an
// existing output of the same device is permitted structurally: the numerical
// model, not a blanket same-device-node prohibition, decides its behavior.
// Every considered nonidentity binding consumes work, including rejected ones.
func controlRebindingsV22(ctx context.Context, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, maximumWork int) (controlRebindingBatchV22, error) {
	return enumerateControlRebindingsV22(ctx, requirement, graph, inventory, maximumWork, false)
}

func enumerateControlRebindingsV22(ctx context.Context, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, maximumWork int, continuation bool) (result controlRebindingBatchV22, err error) {
	result.candidates = []controlRebindingV22{}
	// Budget exits use the same canonical order as complete enumeration.
	defer func() {
		slices.SortFunc(result.candidates, func(a, b controlRebindingV22) int {
			if a.hash < b.hash {
				return -1
			}
			if a.hash > b.hash {
				return 1
			}
			return 0
		})
	}()
	if maximumWork <= 0 {
		return result, fmt.Errorf("control rebinding requires a positive work bound")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	canonical, err := NormalizeGraph(graph)
	if err != nil {
		return result, err
	}
	graph = canonical
	initial := AnalyzeTopologyV21(requirement, graph, inventory)
	if continuation {
		initial = analyzeElectricalTopologyV22(requirement, graph, inventory, deriveFeedbackBindingsV22(requirement, graph, inventory))
	}
	if !initial.Complete || initial.Contradictory {
		return result, fmt.Errorf("control rebinding requires a complete structural graph")
	}
	seen := map[string]bool{initial.GraphHash: true}
	for index, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			return result, fmt.Errorf("control rebinding primitive is absent from inventory")
		}
		contracts := causalTerminalContractsV19(primitive)
		for terminalIndex, connection := range instance.Terminals {
			contract := contracts[connection.Terminal]
			if causalTerminalRoleV19(contract) != "input" {
				continue
			}
			oldNode, found := graphNodeByID(graph, connection.Node)
			if !found {
				return result, fmt.Errorf("control rebinding source node is absent")
			}
			for _, node := range graph.Nodes {
				if err := ctx.Err(); err != nil {
					return result, err
				}
				if node.ID == connection.Node {
					continue
				}
				if result.work == maximumWork {
					result.exhausted = true
					return result, nil
				}
				result.work++
				if !causalTerminalNodeCompatibleV19(contract, node) ||
					(oldNode.Domain != "" && node.Domain != "" && oldNode.Domain != node.Domain) {
					continue
				}
				candidate := CloneGraph(graph)
				candidate.Instances[index].Terminals[terminalIndex].Node = node.ID
				candidate, err = NormalizeGraph(candidate)
				if err != nil {
					continue
				}
				change := GraphChange{Kind: "redirect_terminal", Primitive: instance.ID, Terminal: connection.Terminal, FromNode: connection.Node, ToNode: node.ID}
				if checkControlChangeV22(graph, candidate, inventory, change) != nil {
					continue
				}
				bindings := deriveFeedbackBindingsV22(requirement, candidate, inventory)
				certificate := analyzeElectricalTopologyV22(requirement, candidate, inventory, bindings)
				if !certificate.Complete || certificate.Contradictory || seen[certificate.GraphHash] {
					continue
				}
				seen[certificate.GraphHash] = true
				result.candidates = append(result.candidates, controlRebindingV22{
					graph: candidate, hash: certificate.GraphHash,
					change: change, bindings: bindings,
				})
			}
		}
	}
	return result, nil
}
