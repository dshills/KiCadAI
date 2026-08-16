package opentopologysynthesis

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/simmodel"
)

func TestV19InsertRoleCompleteStageUsesReviewedInventoryAndValidates(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	inventory := causalV19Inventory(t)
	batch, err := InsertRoleCompleteStagesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, RoleCompleteStageRequestV19{
		ObligationID: "transfer_bound", UpstreamNode: "port_signal_in", ObservationID: "signal_out",
	}, causalV19OperationBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) == 0 || batch.Consumption.TopologyRepairs == 0 || batch.Consumption.GeneratedGraphs == 0 || batch.Exhausted {
		t.Fatalf("unexpected batch: %#v", batch)
	}
	keys := map[string]bool{}
	for _, primitive := range inventory.Primitives {
		keys[primitive.Key] = true
	}
	for _, proposal := range batch.Proposals {
		if proposal.LogicalChanges != 1 || len(proposal.Operations) != 1 || proposal.Operations[0].Kind != CausalOperationInsertRoleCompleteStageV19 {
			t.Fatalf("unexpected proposal operation: %#v", proposal)
		}
		operation := proposal.Operations[0]
		if !keys[operation.PrimitiveKey] || operation.ObligationID != "transfer_bound" || operation.BeforeHash == "" || operation.AfterHash == "" || operation.CanonicalCost <= 0 || proposal.CanonicalKey == "" {
			t.Fatalf("incomplete operation evidence: %#v", operation)
		}
		primitive := causalV19Primitive(&inventory, operation.PrimitiveKey)
		if len(operation.Connections) != len(primitive.Terminals) {
			t.Fatalf("connection map is not role-complete: got %d want %d", len(operation.Connections), len(primitive.Terminals))
		}
		if got := ValidateCausalGraphV19(requirement, proposal.Graph, inventory, GraphLimits{}, proposal.Context); len(got) != 0 {
			t.Fatalf("retained proposal bypassed invariants: %#v", got)
		}
	}
}

func TestV19InsertRoleCompleteStageSupportsRegistryPassiveTransfer(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph, _ := InitialGraph(requirement)
	inventory := causalV19Inventory(t)
	resistor := causalV19Primitive(&inventory, "resistor")
	graph = AddPrimitive(graph, resistor, resistor.ValueDomain.Nominal, []TerminalConnection{{Terminal: "A", Node: "port_signal_in"}, {Terminal: "B", Node: "port_ground"}})
	graph = AddPrimitive(graph, resistor, resistor.ValueDomain.Nominal, []TerminalConnection{{Terminal: "A", Node: "port_vcc"}, {Terminal: "B", Node: "port_ground"}})
	batch, err := InsertRoleCompleteStagesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, RoleCompleteStageRequestV19{
		ObligationID: "transfer_bound", UpstreamNode: "port_signal_in", ObservationID: "signal_out",
	}, causalV19OperationBudget())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, proposal := range batch.Proposals {
		if proposal.Operations[0].PrimitiveKey == "resistor" {
			found = true
			if len(proposal.Operations[0].Connections) != 2 || proposal.Operations[0].InstanceID == "" {
				t.Fatalf("passive stage evidence incomplete: %#v", proposal.Operations[0])
			}
			if got := ValidateCausalGraphV19(requirement, proposal.Graph, inventory, GraphLimits{}, proposal.Context); len(got) != 0 {
				t.Fatalf("passive stage failed invariants: %#v", got)
			}
			break
		}
	}
	if !found {
		upstream, _ := graphNodeByID(graph, "port_signal_in")
		observation, _ := graphNodeByID(graph, "port_signal_out")
		maps := causalStageConnectionMapsV19(graph, resistor, upstream, observation)
		direct := AddPrimitive(graph, resistor, resistor.ValueDomain.Nominal, maps[0])
		t.Fatalf("no registry passive stage candidate: maps=%#v issues=%#v consumption=%#v", maps, ValidateCausalGraphV19(requirement, direct, inventory, GraphLimits{}, CausalInvariantContextV19{}), batch.Consumption)
	}
}

func TestV19StageConnectionMappingAllowsCompatiblePowerPinsToShareRail(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph, _ := InitialGraph(requirement)
	inventory := causalV19Inventory(t)
	primitive := causalV19Primitive(&inventory, "buffer")
	terminals := []PrimitiveTerminal{}
	for _, terminal := range primitive.Terminals {
		if terminal.Terminal != "VCC" {
			terminals = append(terminals, terminal)
		}
	}
	for _, id := range []string{"VCC1", "VCC2"} {
		terminals = append(terminals, PrimitiveTerminal{Terminal: id, SymbolID: primitive.SymbolIDs[0], SymbolPin: id, Pad: id, Electrical: "power_in", Required: true})
	}
	primitive.Terminals = terminals
	upstream, _ := graphNodeByID(graph, "port_signal_in")
	observation, _ := graphNodeByID(graph, "port_signal_out")
	for _, candidate := range causalStageConnectionMapsV19(graph, primitive, upstream, observation) {
		connections := map[string]string{}
		for _, connection := range candidate {
			connections[connection.Terminal] = connection.Node
		}
		if connections["VCC1"] == "port_vcc" && connections["VCC2"] == "port_vcc" {
			return
		}
	}
	t.Fatal("no connection map lets compatible power pins share the declared rail")
}

func TestV19AllocateIndependentObservationConesSharesOnlyInputResources(t *testing.T) {
	requirement := causalV19TwoObservationRequirement()
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	inventory := causalV19Inventory(t)
	batch, err := AllocateIndependentObservationConesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, []RoleCompleteStageRequestV19{
		{ObligationID: "transfer_a", UpstreamNode: "port_signal_in", ObservationID: "signal_out"},
		{ObligationID: "transfer_b", UpstreamNode: "port_signal_in", ObservationID: "signal_out_b"},
	}, causalV19OperationBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) == 0 || batch.Consumption.TopologyRepairs < 2 {
		t.Fatalf("no independent-cone proposal: %#v", batch)
	}
	for _, proposal := range batch.Proposals {
		if proposal.PlannerKind != CausalOperationAllocateObservationConeV19 || proposal.LogicalChanges != 2 || len(proposal.Operations) != 2 {
			t.Fatalf("wrong allocation shape: %#v", proposal)
		}
		drivers := map[string]string{}
		for _, operation := range proposal.Operations {
			if operation.UpstreamNode != "port_signal_in" {
				t.Fatalf("allocation did not share the diagnosed input: %#v", operation)
			}
			for _, connection := range operation.Connections {
				if connection.Node == "port_signal_out" || connection.Node == "port_signal_out_b" {
					if prior := drivers[connection.Node]; prior != "" && prior != operation.InstanceID {
						t.Fatalf("observation output is actively shared by %q and %q", prior, operation.InstanceID)
					}
					drivers[connection.Node] = operation.InstanceID
				}
			}
		}
		if len(drivers) != 2 || drivers["port_signal_out"] == drivers["port_signal_out_b"] {
			t.Fatalf("cones are not independent: %#v", drivers)
		}
		if got := ValidateCausalGraphV19(requirement, proposal.Graph, inventory, GraphLimits{}, proposal.Context); len(got) != 0 {
			t.Fatalf("allocation failed V19 gate: %#v", got)
		}
	}
}

func TestV19AllocateHeterogeneousSupplyOutputConesFromTerminalRoles(t *testing.T) {
	requirement := causalV19HeterogeneousSupplyRequirement()
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	inventory := causalV19SupplyStageInventory(t)
	batch, err := AllocateIndependentObservationConesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, []RoleCompleteStageRequestV19{
		{ObligationID: "voltage_source", UpstreamNode: "port_vcc", ObservationID: "voltage_out"},
		{ObligationID: "current_source", UpstreamNode: "port_vcc", ObservationID: "current_out"},
	}, causalV19OperationBudget())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, proposal := range batch.Proposals {
		keys := map[string]bool{}
		for _, operation := range proposal.Operations {
			keys[operation.PrimitiveKey] = true
		}
		if keys["voltage_stage"] && keys["current_stage"] {
			found = true
			if got := ValidateCausalGraphV19(requirement, proposal.Graph, inventory, GraphLimits{}, proposal.Context); len(got) != 0 {
				t.Fatalf("heterogeneous source proposal failed invariants: %#v", got)
			}
			break
		}
	}
	if !found {
		t.Fatalf("no inventory-derived voltage/current source pair: %#v", batch)
	}
}

func TestV19IndependentDecisionConesCanUseOppositeInputPolarity(t *testing.T) {
	requirement := causalV19TwoObservationRequirement()
	graph, _ := InitialGraph(requirement)
	inventory := causalV19Inventory(t)
	batch, err := AllocateIndependentObservationConesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, []RoleCompleteStageRequestV19{
		{ObligationID: "transfer_a", UpstreamNode: "port_signal_in", ObservationID: "signal_out"},
		{ObligationID: "transfer_b", UpstreamNode: "port_signal_in", ObservationID: "signal_out_b"},
	}, causalV19OperationBudget())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, proposal := range batch.Proposals {
		if len(proposal.Operations) != 2 || proposal.Operations[0].PrimitiveKind != "opamp" || proposal.Operations[1].PrimitiveKind != "opamp" {
			continue
		}
		left := causalV19TerminalAtNode(proposal.Operations[0].Connections, "port_signal_in")
		right := causalV19TerminalAtNode(proposal.Operations[1].Connections, "port_signal_in")
		if (left == "IN_PLUS" && right == "IN_MINUS") || (left == "IN_MINUS" && right == "IN_PLUS") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no opposite-polarity independent decision cones: %#v", batch)
	}
}

func TestV19RedirectRoleTerminalPreservesCompleteContract(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	inventory := causalV19Inventory(t)
	graph := causalV19RedirectableGraph(t, requirement)
	ampID := causalV19InstanceByKind(graph, "opamp")
	targetNode := causalV19FirstInternalNode(graph)
	batch, err := RedirectRoleTerminalV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, RoleTerminalRedirectRequestV19{
		ObligationID: "transfer_bound", InstanceID: ampID, Terminal: "IN_PLUS", Node: targetNode,
	}, causalV19OperationBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 || batch.Consumption.TopologyRepairs != 1 || batch.Consumption.GeneratedGraphs != 1 {
		t.Fatalf("unexpected redirect batch: %#v", batch)
	}
	proposal := batch.Proposals[0]
	if proposal.Operations[0].Kind != CausalOperationRedirectRoleTerminalV19 || len(proposal.Operations[0].Connections) != 1 {
		t.Fatalf("unexpected redirect evidence: %#v", proposal.Operations[0])
	}
	for _, instance := range proposal.Graph.Instances {
		primitive := causalV19Primitive(&inventory, instance.PrimitiveKey)
		if len(instance.Terminals) != len(primitive.Terminals) {
			t.Fatalf("redirect damaged terminal contract: %#v", instance)
		}
	}
	if got := ValidateCausalGraphV19(requirement, proposal.Graph, inventory, GraphLimits{}, proposal.Context); len(got) != 0 {
		t.Fatalf("redirect failed V19 gate: %#v", got)
	}
}

func TestV19InsertTypedFeedbackPathIsPassiveBoundAndReplayable(t *testing.T) {
	requirement := causalV19Requirement("hysteresis", "transient")
	inventory := causalV19Inventory(t)
	graph := causalV19FeedbackReadyGraph(t, requirement)
	ampID := causalV19InstanceByKind(graph, "opamp")
	request := TypedFeedbackPathRequestV19{ObligationID: "transfer_bound", FromInstance: ampID, FromTerminal: "OUT", ToInstance: ampID, ToTerminal: "IN_MINUS"}
	batch, err := InsertTypedFeedbackPathsV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, request, causalV19OperationBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 {
		t.Fatalf("expected one registry passive feedback candidate: %#v", batch)
	}
	proposal := batch.Proposals[0]
	operation := proposal.Operations[0]
	if operation.Kind != CausalOperationInsertTypedFeedbackPathV19 || operation.FeedbackPath == nil || operation.PrimitiveKey != "resistor" || len(proposal.Context.FeedbackPaths) != 1 {
		t.Fatalf("incomplete typed-feedback evidence: %#v", proposal)
	}
	if got := ValidateCausalGraphV19(requirement, proposal.Graph, inventory, GraphLimits{}, proposal.Context); len(got) != 0 {
		t.Fatalf("typed feedback failed V19 gate: %#v", got)
	}
	want, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	for replay := 0; replay < 32; replay++ {
		gotBatch, replayErr := InsertTypedFeedbackPathsV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, request, causalV19OperationBudget())
		if replayErr != nil {
			t.Fatal(replayErr)
		}
		got, _ := json.Marshal(gotBatch)
		if string(got) != string(want) {
			t.Fatalf("replay %d changed output", replay)
		}
	}
}

func TestV19OperationsFailClosedOnContractAnalysisAndComposition(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph, _ := InitialGraph(requirement)
	inventory := causalV19Inventory(t)

	badInventory := causalV19CloneInventory(inventory)
	for primitiveIndex := range badInventory.Primitives {
		for modelIndex := range badInventory.Primitives[primitiveIndex].Models {
			badInventory.Primitives[primitiveIndex].Models[modelIndex].AllowedAnalyses = []string{"transient"}
		}
	}
	causalV19SealInventory(t, &badInventory)
	batch, err := InsertRoleCompleteStagesV19(requirement, graph, badInventory, GraphLimits{}, CausalInvariantContextV19{}, RoleCompleteStageRequestV19{ObligationID: "transfer_bound", UpstreamNode: "port_signal_in", ObservationID: "signal_out"}, causalV19OperationBudget())
	if err != nil || len(batch.Proposals) != 0 {
		t.Fatalf("analysis-incomplete primitive was retained: batch=%#v err=%v", batch, err)
	}

	_, err = AllocateIndependentObservationConesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, []RoleCompleteStageRequestV19{{ObligationID: "transfer_bound"}, {ObligationID: "transfer_bound"}, {ObligationID: "transfer_bound"}}, causalV19OperationBudget())
	if !errors.Is(err, ErrCausalOperationRequestV19) {
		t.Fatalf("three-change allocation did not fail closed: %v", err)
	}

	validGraph := causalV19FeedForwardGraph(t, requirement)
	_, err = RedirectRoleTerminalV19(requirement, validGraph, inventory, GraphLimits{}, CausalInvariantContextV19{}, RoleTerminalRedirectRequestV19{ObligationID: "transfer_bound", InstanceID: "amp", Terminal: "OUT", Node: "port_signal_in"}, causalV19OperationBudget())
	if !errors.Is(err, ErrCausalOperationRequestV19) {
		t.Fatalf("output-to-input redirect did not fail closed: %v", err)
	}

	_, err = InsertTypedFeedbackPathsV19(requirement, validGraph, inventory, GraphLimits{}, CausalInvariantContextV19{}, TypedFeedbackPathRequestV19{ObligationID: "transfer_bound", FromInstance: "amp", FromTerminal: "OUT", ToInstance: "amp", ToTerminal: "IN_MINUS"}, causalV19OperationBudget())
	if !errors.Is(err, ErrCausalOperationRequestV19) {
		t.Fatalf("non-feedback obligation admitted a back-edge: %v", err)
	}

	stageBatch, err := InsertRoleCompleteStagesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, RoleCompleteStageRequestV19{ObligationID: "transfer_bound", UpstreamNode: "port_signal_in", ObservationID: "signal_out"}, causalV19OperationBudget())
	if err != nil || len(stageBatch.Proposals) == 0 {
		t.Fatalf("stage setup: %#v %v", stageBatch, err)
	}
	stage := causalV19ProposalWithTerminal(t, stageBatch.Proposals, "IN_MINUS")
	ampID := stage.Operations[0].InstanceID
	redirectBatch, err := RedirectRoleTerminalV19(requirement, stage.Graph, inventory, GraphLimits{}, stage.Context, RoleTerminalRedirectRequestV19{ObligationID: "transfer_bound", InstanceID: ampID, Terminal: "IN_MINUS", Node: "port_signal_in"}, causalV19OperationBudget())
	if err != nil || len(redirectBatch.Proposals) == 0 {
		t.Fatalf("second-new setup: %#v %v", redirectBatch, err)
	}
	_, err = ComposeCausalProposalsV19(requirement, inventory, GraphLimits{}, stage, redirectBatch.Proposals[0], causalV19OperationBudget())
	if !errors.Is(err, ErrCausalOperationCompositionV19) {
		t.Fatalf("two new operations were composed: %v", err)
	}
}

func TestV19OperationAccountingUsesInheritedLimitsExactly(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph, _ := InitialGraph(requirement)
	inventory := causalV19Inventory(t)
	request := RoleCompleteStageRequestV19{ObligationID: "transfer_bound", UpstreamNode: "port_signal_in", ObservationID: "signal_out"}

	zero, err := InsertRoleCompleteStagesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, request, CausalOperationBudgetV19{})
	if err != nil || !zero.Exhausted || zero.Consumption != (CausalOperationConsumptionV19{}) || len(zero.Proposals) != 0 {
		t.Fatalf("zero budget was not exact: %#v %v", zero, err)
	}
	one, err := InsertRoleCompleteStagesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, request, CausalOperationBudgetV19{TopologyRepairs: 1, GeneratedGraphs: 1})
	if err != nil || one.Consumption.TopologyRepairs != 1 || one.Consumption.GeneratedGraphs != 1 || !one.Exhausted {
		t.Fatalf("one-attempt accounting mismatch: %#v %v", one, err)
	}
	policy := DefaultPolicy()
	_, err = InsertRoleCompleteStagesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, request, CausalOperationBudgetV19{TopologyRepairs: policy.MaxTopologyRepairs + 1, GeneratedGraphs: 1})
	if !errors.Is(err, ErrCausalOperationBudgetV19) {
		t.Fatalf("raised inherited repair limit was accepted: %v", err)
	}

	twoRequirement := causalV19TwoObservationRequirement()
	twoGraph, _ := InitialGraph(twoRequirement)
	tooSmall, err := AllocateIndependentObservationConesV19(twoRequirement, twoGraph, inventory, GraphLimits{}, CausalInvariantContextV19{}, []RoleCompleteStageRequestV19{
		{ObligationID: "transfer_a", UpstreamNode: "port_signal_in", ObservationID: "signal_out"},
		{ObligationID: "transfer_b", UpstreamNode: "port_signal_in", ObservationID: "signal_out_b"},
	}, CausalOperationBudgetV19{TopologyRepairs: 1, GeneratedGraphs: 1})
	if err != nil || !tooSmall.Exhausted || tooSmall.Consumption != (CausalOperationConsumptionV19{}) {
		t.Fatalf("two-change proposal partially consumed budget: %#v %v", tooSmall, err)
	}
}

func TestV19OperationPermutationAndReplayAreByteStable(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph, _ := InitialGraph(requirement)
	inventory := causalV19Inventory(t)
	request := RoleCompleteStageRequestV19{ObligationID: "transfer_bound", UpstreamNode: "port_signal_in", ObservationID: "signal_out"}
	wantBatch, err := InsertRoleCompleteStagesV19(requirement, graph, inventory, GraphLimits{}, CausalInvariantContextV19{}, request, causalV19OperationBudget())
	if err != nil || len(wantBatch.Proposals) == 0 {
		t.Fatalf("baseline: %#v %v", wantBatch, err)
	}
	want, _ := json.Marshal(wantBatch)
	random := rand.New(rand.NewSource(19020))
	for iteration := 0; iteration < 256; iteration++ {
		shuffledRequirement := requirement
		shuffledRequirement.Requirements.Domains = slices.Clone(requirement.Requirements.Domains)
		shuffledRequirement.Requirements.Ports = slices.Clone(requirement.Requirements.Ports)
		shuffledRequirement.Requirements.BehavioralRequirements = slices.Clone(requirement.Requirements.BehavioralRequirements)
		random.Shuffle(len(shuffledRequirement.Requirements.Domains), func(i, j int) {
			shuffledRequirement.Requirements.Domains[i], shuffledRequirement.Requirements.Domains[j] = shuffledRequirement.Requirements.Domains[j], shuffledRequirement.Requirements.Domains[i]
		})
		random.Shuffle(len(shuffledRequirement.Requirements.Ports), func(i, j int) {
			shuffledRequirement.Requirements.Ports[i], shuffledRequirement.Requirements.Ports[j] = shuffledRequirement.Requirements.Ports[j], shuffledRequirement.Requirements.Ports[i]
		})
		shuffledGraph := CloneGraph(graph)
		random.Shuffle(len(shuffledGraph.Nodes), func(i, j int) {
			shuffledGraph.Nodes[i], shuffledGraph.Nodes[j] = shuffledGraph.Nodes[j], shuffledGraph.Nodes[i]
		})
		shuffledInventory := causalV19CloneInventory(inventory)
		random.Shuffle(len(shuffledInventory.Primitives), func(i, j int) {
			shuffledInventory.Primitives[i], shuffledInventory.Primitives[j] = shuffledInventory.Primitives[j], shuffledInventory.Primitives[i]
		})
		for index := range shuffledInventory.Primitives {
			random.Shuffle(len(shuffledInventory.Primitives[index].Terminals), func(i, j int) {
				shuffledInventory.Primitives[index].Terminals[i], shuffledInventory.Primitives[index].Terminals[j] = shuffledInventory.Primitives[index].Terminals[j], shuffledInventory.Primitives[index].Terminals[i]
			})
		}
		causalV19SealInventory(t, &shuffledInventory)
		gotBatch, gotErr := InsertRoleCompleteStagesV19(shuffledRequirement, shuffledGraph, shuffledInventory, GraphLimits{}, CausalInvariantContextV19{}, request, causalV19OperationBudget())
		if gotErr != nil {
			t.Fatal(gotErr)
		}
		got, _ := json.Marshal(gotBatch)
		if string(got) != string(want) {
			t.Fatalf("permutation %d changed operation bytes", iteration)
		}
	}
}

func TestV19AddedInstanceEvidenceRejectsRenumberedExistingInstances(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	inventory := causalV19Inventory(t)
	before := causalV19FeedForwardGraph(t, requirement)
	resistor := causalV19Primitive(&inventory, "resistor")
	before = AddPrimitive(before, resistor, resistor.ValueDomain.Nominal, []TerminalConnection{{Terminal: "A", Node: "port_signal_in"}, {Terminal: "B", Node: "port_ground"}})
	var err error
	before, err = NormalizeGraph(before)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Instances) < 2 {
		t.Fatal("test graph needs two preserved instances")
	}
	after := AddPrimitive(before, resistor, resistor.ValueDomain.Nominal, []TerminalConnection{{Terminal: "A", Node: "port_signal_out"}, {Terminal: "B", Node: "port_ground"}})
	if got := causalAddedInstanceIDV19(before, after); got == "" {
		t.Fatal("canonical append was not identified")
	}

	renumbered := CloneGraph(after)
	renumbered.Instances[0].ID, renumbered.Instances[1].ID = renumbered.Instances[1].ID, renumbered.Instances[0].ID
	if !graphUsesCanonicalIDs(renumbered) {
		t.Fatal("adversarial graph must retain a superficially canonical ID set")
	}
	if got := causalAddedInstanceIDV19(before, renumbered); got != "" {
		t.Fatalf("renumbered existing instances produced false addition evidence %q", got)
	}
}

func TestV19OperationPhasePreservesExactV18Delegation(t *testing.T) {
	t.Run("noneligible exact delegation", TestSynthesizeV18DelegatesNoneligibleRequirementByteForByte)
	t.Run("legacy extension isolation", TestSynthesizeV18WithLegacyIsolatesExtensionForNoneligibleRequirement)
}

func TestV19ExistingOperationsRemainAuthenticAndComposeWithOneNewChange(t *testing.T) {
	requirement := causalV19Requirement("hysteresis", "transient")
	inventory := causalV19Inventory(t)
	base := causalV19FeedbackReadyGraph(t, requirement)
	biasID := causalV19InstanceByKind(base, "resistor")
	valueGraph := CloneGraph(base)
	for index := range valueGraph.Instances {
		if valueGraph.Instances[index].ID == biasID {
			value := 20_000.0
			valueGraph.Instances[index].ValueSI = &value
		}
	}
	beforeHash, _ := GraphHash(base)
	afterHash, _ := GraphHash(valueGraph)
	existing, err := RecordExistingCausalOperationV19(requirement, base, valueGraph, inventory, GraphLimits{}, CausalInvariantContextV19{}, GraphOperation{
		Kind: "set_value", Node: biasID, ValueSI: causalV19Float(20_000), BeforeHash: beforeHash, AfterHash: afterHash,
	}, "transfer_bound")
	if err != nil {
		t.Fatal(err)
	}
	ampID := causalV19InstanceByKind(existing.Graph, "opamp")
	feedbackBatch, err := InsertTypedFeedbackPathsV19(requirement, existing.Graph, inventory, GraphLimits{}, existing.Context, TypedFeedbackPathRequestV19{
		ObligationID: "transfer_bound", FromInstance: ampID, FromTerminal: "OUT", ToInstance: ampID, ToTerminal: "IN_MINUS",
	}, causalV19OperationBudget())
	if err != nil || len(feedbackBatch.Proposals) == 0 {
		t.Fatalf("feedback after existing value: %#v %v", feedbackBatch, err)
	}
	composed, err := ComposeCausalProposalsV19(requirement, inventory, GraphLimits{}, existing, feedbackBatch.Proposals[0], CausalOperationBudgetV19{TopologyRepairs: 2, GeneratedGraphs: 1})
	if err != nil || len(composed.Proposals) != 1 || composed.Consumption.TopologyRepairs != 2 || composed.Proposals[0].LogicalChanges != 2 {
		t.Fatalf("existing+new composition failed: %#v %v", composed, err)
	}
	if composed.Proposals[0].Operations[0].Kind != "set_value" || composed.Proposals[0].Operations[1].Kind != CausalOperationInsertTypedFeedbackPathV19 {
		t.Fatalf("operation order changed: %#v", composed.Proposals[0].Operations)
	}

	for _, test := range causalV19ExistingDeltaCases(t, inventory, base) {
		kind, _, _, _, ok := causalExistingDeltaV19(test.before, test.after, test.inventory)
		if !ok || kind != test.kind {
			t.Errorf("%s delta classified as %q ok=%t", test.kind, kind, ok)
		}
	}

	forged := existing
	forged.Operations[0].Kind = CausalOperationInsertRoleCompleteStageV19
	_, err = ComposeCausalProposalsV19(requirement, inventory, GraphLimits{}, forged, feedbackBatch.Proposals[0], causalV19OperationBudget())
	if !errors.Is(err, ErrCausalOperationCompositionV19) {
		t.Fatalf("forged historical operation was accepted: %v", err)
	}
}

type causalExistingDeltaCaseV19 struct {
	kind      string
	before    CandidateGraph
	after     CandidateGraph
	inventory PrimitiveInventory
}

func causalV19ExistingDeltaCases(t *testing.T, inventory PrimitiveInventory, base CandidateGraph) []causalExistingDeltaCaseV19 {
	t.Helper()
	result := []causalExistingDeltaCaseV19{}
	biasID := causalV19InstanceByKind(base, "resistor")
	ampID := causalV19InstanceByKind(base, "opamp")

	valueAfter := CloneGraph(base)
	for index := range valueAfter.Instances {
		if valueAfter.Instances[index].ID == biasID {
			valueAfter.Instances[index].ValueSI = causalV19Float(30_000)
		}
	}
	result = append(result, causalExistingDeltaCaseV19{kind: "set_value", before: base, after: valueAfter, inventory: inventory})

	polarityAfter := CloneGraph(base)
	ampIndex := graphInstanceIndex(polarityAfter, ampID)
	left, right := -1, -1
	for index, terminal := range polarityAfter.Instances[ampIndex].Terminals {
		if terminal.Terminal == "IN_PLUS" {
			left = index
		}
		if terminal.Terminal == "IN_MINUS" {
			right = index
		}
	}
	polarityAfter.Instances[ampIndex].Terminals[left].Node, polarityAfter.Instances[ampIndex].Terminals[right].Node = polarityAfter.Instances[ampIndex].Terminals[right].Node, polarityAfter.Instances[ampIndex].Terminals[left].Node
	result = append(result, causalExistingDeltaCaseV19{kind: "correct_polarity", before: base, after: polarityAfter, inventory: inventory})

	resistor := causalV19Primitive(&inventory, "resistor")
	passiveAfter, err := BridgeNodesWithPrimitive(base, resistor, resistor.ValueDomain.Nominal, "port_signal_in", "port_ground")
	if err != nil {
		t.Fatal(err)
	}
	result = append(result, causalExistingDeltaCaseV19{kind: "add_primitive", before: base, after: passiveAfter, inventory: inventory})

	splitAfter, _, err := splitPrimitiveInSeries(base, inventory, biasID, resistor, resistor.ValueDomain.Nominal)
	if err != nil {
		t.Fatal(err)
	}
	result = append(result, causalExistingDeltaCaseV19{kind: "split_primitive", before: base, after: splitAfter, inventory: inventory})

	substitutionInventory := causalV19CloneInventory(inventory)
	replacement := causalV19Primitive(&substitutionInventory, "resistor")
	replacement.Key = "resistor_alt"
	replacement.CatalogID = "catalog.resistor_alt"
	substitutionInventory.Primitives = append(substitutionInventory.Primitives, replacement)
	causalV19SealInventory(t, &substitutionInventory)
	substituteAfter, err := SubstitutePrimitive(base, substitutionInventory, biasID, replacement.Key)
	if err != nil {
		t.Fatal(err)
	}
	result = append(result, causalExistingDeltaCaseV19{kind: "substitute_primitive", before: base, after: substituteAfter, inventory: substitutionInventory})

	redirectBase, passiveTarget := addInternalNode(base, "internal")
	redirectBase = AddPrimitive(redirectBase, resistor, causalV19Float(10_000), []TerminalConnection{{Terminal: "A", Node: passiveTarget}, {Terminal: "B", Node: "port_ground"}})
	redirectAfter := CloneGraph(redirectBase)
	biasIndex := graphInstanceIndex(redirectAfter, biasID)
	for index := range redirectAfter.Instances[biasIndex].Terminals {
		if redirectAfter.Instances[biasIndex].Terminals[index].Terminal == "B" {
			redirectAfter.Instances[biasIndex].Terminals[index].Node = passiveTarget
		}
	}
	result = append(result, causalExistingDeltaCaseV19{kind: "redirect_terminal", before: redirectBase, after: redirectAfter, inventory: inventory})
	return result
}

func causalV19OperationBudget() CausalOperationBudgetV19 {
	return CausalOperationBudgetV19{TopologyRepairs: DefaultPolicy().MaxTopologyRepairs, GeneratedGraphs: DefaultPolicy().MaxGeneratedGraphs}
}

func causalV19TwoObservationRequirement() Requirement {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	requirement.Requirements.Ports = append(requirement.Requirements.Ports, Port{ID: "signal_out_b", Kind: "digital", Direction: "source", Domain: "ground", Electrical: Electrical{MinVoltageV: causalV19Float(0), NominalVoltageV: causalV19Float(.5), MaxVoltageV: causalV19Float(1), MaxCurrentA: causalV19Float(.01)}})
	second := requirement.Requirements.BehavioralRequirements[0]
	second.ID = "transfer_b"
	second.Metric = "output_high_voltage"
	second.Observation = Observation{Kind: "port", ID: "signal_out_b"}
	requirement.Requirements.BehavioralRequirements[0].ID = "transfer_a"
	requirement.Requirements.BehavioralRequirements = append(requirement.Requirements.BehavioralRequirements, second)
	return Normalize(requirement)
}

func causalV19HeterogeneousSupplyRequirement() Requirement {
	requirement := causalV19Requirement("dc_voltage", "dc_operating_point")
	ports := []Port{}
	for _, port := range requirement.Requirements.Ports {
		if port.ID == "signal_in" || port.ID == "signal_out" {
			continue
		}
		ports = append(ports, port)
	}
	ports = append(ports,
		Port{ID: "voltage_out", Kind: "analog_voltage", Direction: "source", Domain: "ground", Electrical: Electrical{MinVoltageV: causalV19Float(.5), NominalVoltageV: causalV19Float(1), MaxVoltageV: causalV19Float(2), MaxCurrentA: causalV19Float(.02)}},
		Port{ID: "current_out", Kind: "controlled_current", Direction: "source", Domain: "ground", Electrical: Electrical{MinVoltageV: causalV19Float(0), NominalVoltageV: causalV19Float(1), MaxVoltageV: causalV19Float(3), MaxCurrentA: causalV19Float(.02)}},
	)
	requirement.Requirements.Ports = ports
	requirement.Requirements.OperatingCases[0].Conditions = []OperatingCondition{{Axis: "supply_voltage", Target: "supply", Min: 3, Max: 5, Unit: "V"}}
	minimumVoltage, maximumVoltage := .5, 2.0
	minimumCurrent, maximumCurrent := .001, .02
	requirement.Requirements.BehavioralRequirements = []BehavioralAssertion{
		{ID: "voltage_source", Metric: "dc_voltage", Analysis: "dc_operating_point", Excitation: &Observation{Kind: "port", ID: "vcc"}, Observation: Observation{Kind: "port", ID: "voltage_out"}, Min: &minimumVoltage, Max: &maximumVoltage, Unit: "V", OperatingCases: []string{"bounded"}},
		{ID: "current_source", Metric: "output_current", Analysis: "dc_operating_point", Excitation: &Observation{Kind: "port", ID: "vcc"}, Observation: Observation{Kind: "port", ID: "current_out"}, Min: &minimumCurrent, Max: &maximumCurrent, Unit: "A", OperatingCases: []string{"bounded"}},
	}
	return Normalize(requirement)
}

func causalV19SupplyStageInventory(t *testing.T) PrimitiveInventory {
	t.Helper()
	inventory := causalV19Inventory(t)
	modelFrom := func(source PrimitiveModelContract, modelID, family string) PrimitiveModelContract {
		source.ModelID = modelID
		source.Family = family
		return source
	}
	terminal := func(id, electrical string) PrimitiveTerminal {
		return PrimitiveTerminal{Terminal: id, Function: id, SymbolID: "symbol", SymbolPin: id, Pad: id, Electrical: electrical, Required: true}
	}
	voltageRating, currentRating := 10.0, .1
	baseModel := inventory.Primitives[0].Models[0]
	inventory.Primitives = append(inventory.Primitives,
		PrimitiveCandidate{
			Key: "voltage_stage", CatalogID: "catalog.voltage_stage", VariantID: "variant", Kind: "regulated_source", Family: "power_transfer", Evidence: "verified", SymbolIDs: []string{"symbol"}, FootprintID: "footprint", PackageType: "test",
			Terminals: []PrimitiveTerminal{terminal("VIN", "power_in"), terminal("VOUT", "power_out"), terminal("GND", "power_in")},
			Models:    []PrimitiveModelContract{modelFrom(baseModel, simmodel.PrimitiveFixedLinearRegulatorV1, "regulator")},
			Ratings:   []PrimitiveBound{{Kind: "working_voltage", Unit: "V", Maximum: &voltageRating}, {Kind: "output_current", Unit: "A", Maximum: &currentRating}},
		},
		PrimitiveCandidate{
			Key: "current_stage", CatalogID: "catalog.current_stage", VariantID: "variant", Kind: "controlled_source", Family: "power_transfer", Evidence: "verified", SymbolIDs: []string{"symbol"}, FootprintID: "footprint", PackageType: "test",
			Terminals: []PrimitiveTerminal{terminal("IN", "power_in"), terminal("SET", "input"), terminal("OUT", "power_out")},
			Models:    []PrimitiveModelContract{modelFrom(baseModel, simmodel.PrimitiveProgrammableCurrentSourceV1, "current_regulator")},
			Ratings:   []PrimitiveBound{{Kind: "working_voltage", Unit: "V", Maximum: &voltageRating}, {Kind: "output_current", Unit: "A", Maximum: &currentRating}},
		},
	)
	causalV19SealInventory(t, &inventory)
	return inventory
}

func causalV19TerminalAtNode(connections []TerminalConnection, node string) string {
	for _, connection := range connections {
		if connection.Node == node {
			return connection.Terminal
		}
	}
	return ""
}

func causalV19RedirectableGraph(t *testing.T, requirement Requirement) CandidateGraph {
	t.Helper()
	graph := causalV19FeedForwardGraph(t, requirement)
	graph.Nodes = append(graph.Nodes, GraphNode{ID: "redirect_target", Scope: "internal", Role: "internal"})
	graph.Instances = append(graph.Instances,
		GraphInstance{ID: "target_input", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: causalV19Float(10_000), Terminals: []TerminalConnection{{Terminal: "A", Node: "redirect_target"}, {Terminal: "B", Node: "port_signal_in"}}},
		GraphInstance{ID: "target_reference", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: causalV19Float(100_000), Terminals: []TerminalConnection{{Terminal: "A", Node: "redirect_target"}, {Terminal: "B", Node: "port_ground"}}},
	)
	normalized, err := NormalizeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	return normalized
}

func causalV19FeedbackReadyGraph(t *testing.T, requirement Requirement) CandidateGraph {
	t.Helper()
	graph := causalV19FeedForwardGraph(t, requirement)
	graph.Nodes = append(graph.Nodes, GraphNode{ID: "feedback_node", Scope: "internal", Role: "internal"})
	for index := range graph.Instances[0].Terminals {
		if graph.Instances[0].Terminals[index].Terminal == "IN_MINUS" {
			graph.Instances[0].Terminals[index].Node = "feedback_node"
		}
	}
	graph.Instances = append(graph.Instances, GraphInstance{ID: "bias", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: causalV19Float(10_000), Terminals: []TerminalConnection{{Terminal: "A", Node: "feedback_node"}, {Terminal: "B", Node: "port_ground"}}})
	normalized, err := NormalizeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	return normalized
}

func causalV19InstanceByKind(graph CandidateGraph, kind string) string {
	result := []string{}
	for _, instance := range graph.Instances {
		if instance.Kind == kind {
			result = append(result, instance.ID)
		}
	}
	slices.Sort(result)
	if len(result) == 0 {
		return ""
	}
	return result[0]
}

func causalV19FirstInternalNode(graph CandidateGraph) string {
	result := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "internal" {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	if len(result) == 0 {
		return ""
	}
	return result[0]
}

func causalV19ProposalWithTerminal(t *testing.T, proposals []CausalOperationProposalV19, terminal string) CausalOperationProposalV19 {
	t.Helper()
	for _, proposal := range proposals {
		for _, connection := range proposal.Operations[0].Connections {
			if connection.Terminal == terminal {
				return proposal
			}
		}
	}
	t.Fatalf("no proposal has terminal %q", terminal)
	return CausalOperationProposalV19{}
}

func causalV19RequireByteEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func causalV19HasOperationKind(proposal CausalOperationProposalV19, kind string) bool {
	for _, operation := range proposal.Operations {
		if strings.EqualFold(operation.Kind, kind) {
			return true
		}
	}
	return false
}
