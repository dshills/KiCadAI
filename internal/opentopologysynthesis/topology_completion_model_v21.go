package opentopologysynthesis

import "kicadai/internal/reports"

const (
	TopologyCompletionSchemaV21  = "kicadai.topology-completion-plan.v21"
	TopologyCompletionVersionV21 = 21

	TopologyObligationMissingBindingV21       = "missing_port_binding"
	TopologyObligationUnreachablePortV21      = "unreachable_external_port"
	TopologyObligationObservationConeV21      = "missing_observation_cone"
	TopologyObligationCausalPathV21           = "incomplete_causal_path"
	TopologyObligationReferenceV21            = "missing_domain_reference"
	TopologyObligationDirectionV21            = "invalid_terminal_direction"
	TopologyObligationDisconnectedSubgraphV21 = "disconnected_required_subgraph"
	TopologyObligationBranchV21               = "missing_required_branch"
	TopologyObligationIrrelevantFragmentV21   = "causally_irrelevant_fragment"
	TopologyObligationInvalidEvidenceV21      = "invalid_canonical_evidence"

	TopologyOperationConnectPortV21      = "connect_unreachable_port"
	TopologyOperationCompletePathV21     = "complete_causal_path"
	TopologyOperationExtendConeV21       = "extend_observation_cone"
	TopologyOperationJoinPathsV21        = "join_compatible_partial_paths"
	TopologyOperationIntroduceBranchV21  = "introduce_branch_or_convergence"
	TopologyOperationRedirectTerminalV21 = "redirect_invalid_terminal"
	TopologyOperationAttachReferenceV21  = "attach_domain_reference"
	TopologyOperationRemoveIrrelevantV21 = "remove_irrelevant_fragment"

	TopologyRejectionNoneV21          = ""
	TopologyRejectionNoOperationV21   = "no_applicable_generic_operation"
	TopologyRejectionDuplicateV21     = "duplicate_candidate"
	TopologyRejectionCycleV21         = "ancestor_cycle"
	TopologyRejectionDominatedV21     = "dominated_candidate"
	TopologyRejectionInvalidV21       = "invalid_terminal_domain_or_observation"
	TopologyRejectionContradictoryV21 = "electrically_contradictory_path"
	TopologyRejectionDepthV21         = "depth_limit"
	TopologyRejectionWidthV21         = "width_limit"
	TopologyRejectionWorkV21          = "work_limit"
	TopologyRejectionMemoryV21        = "memory_limit"
)

const (
	CodeTopologyNoApplicableV21  reports.Code = "OPEN_TOPOLOGY_V21_NO_APPLICABLE_OPERATION"
	CodeTopologyContradictoryV21 reports.Code = "OPEN_TOPOLOGY_V21_CONTRADICTORY_PATH"
	CodeTopologyCycleV21         reports.Code = "OPEN_TOPOLOGY_V21_CYCLE_OR_DUPLICATE"
	CodeTopologyBoundV21         reports.Code = "OPEN_TOPOLOGY_V21_BOUND_EXHAUSTED"
	CodeTopologyInvalidRepairV21 reports.Code = "OPEN_TOPOLOGY_V21_INVALID_REPAIR"
	CodeTopologyCertifiedV21     reports.Code = "OPEN_TOPOLOGY_V21_STRUCTURALLY_COMPLETE"
)

// TopologyCompletionLimitsV21 bounds every dimension that can grow during
// structural planning. Workers changes execution only, never result ordering.
type TopologyCompletionLimitsV21 struct {
	MaximumDepth      int `json:"maximum_depth"`
	MaximumWidth      int `json:"maximum_width"`
	MaximumWork       int `json:"maximum_work"`
	MaximumRetained   int `json:"maximum_retained"`
	MaximumGraphBytes int `json:"maximum_graph_bytes"`
	Workers           int `json:"workers"`
}

func DefaultTopologyCompletionLimitsV21() TopologyCompletionLimitsV21 {
	return TopologyCompletionLimitsV21{
		MaximumDepth: 3, MaximumWidth: 8, MaximumWork: 48,
		MaximumRetained: 64, MaximumGraphBytes: 1 << 20, Workers: 1,
	}
}

type TopologyObligationV21 struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	AssertionID   string `json:"assertion_id,omitempty"`
	ObservationID string `json:"observation_id,omitempty"`
	FromNode      string `json:"from_node,omitempty"`
	ToNode        string `json:"to_node,omitempty"`
	Domain        string `json:"domain,omitempty"`
	InstanceID    string `json:"instance_id,omitempty"`
	Terminal      string `json:"terminal,omitempty"`
	Critical      bool   `json:"critical"`
	EvidenceHash  string `json:"evidence_sha256"`
}

type TopologyInvariantReportV21 struct {
	Schema          string                  `json:"schema"`
	Version         int                     `json:"version"`
	RequirementHash string                  `json:"requirement_sha256"`
	InventoryHash   string                  `json:"inventory_sha256"`
	GraphHash       string                  `json:"graph_sha256"`
	Obligations     []TopologyObligationV21 `json:"obligations"`
	Complete        bool                    `json:"complete"`
	Contradictory   bool                    `json:"contradictory"`
	Hash            string                  `json:"hash"`
}

type TopologyOperationEvidenceV21 struct {
	Number          int                   `json:"number"`
	Kind            string                `json:"kind"`
	Obligation      TopologyObligationV21 `json:"obligation"`
	AffectedScope   []string              `json:"affected_scope"`
	PrimitiveKey    string                `json:"primitive_key,omitempty"`
	InstanceID      string                `json:"instance_id,omitempty"`
	Terminal        string                `json:"terminal,omitempty"`
	BeforeGraphHash string                `json:"before_graph_sha256"`
	AfterGraphHash  string                `json:"after_graph_sha256"`
	ParentStateHash string                `json:"parent_state_sha256"`
	WorkConsumed    int                   `json:"work_consumed"`
	Accepted        bool                  `json:"accepted"`
	Reason          string                `json:"reason"`
	Hash            string                `json:"hash"`
}

type TopologyCandidateEvidenceV21 struct {
	Graph       CandidateGraph                 `json:"graph"`
	GraphHash   string                         `json:"graph_sha256"`
	StateHash   string                         `json:"state_sha256"`
	ParentHash  string                         `json:"parent_state_sha256,omitempty"`
	Depth       int                            `json:"depth"`
	Operations  []TopologyOperationEvidenceV21 `json:"operations"`
	Invariant   TopologyInvariantReportV21     `json:"invariant"`
	Disposition string                         `json:"disposition"`
	Reason      string                         `json:"reason"`
	Hash        string                         `json:"hash"`
}

type TopologyCompletionConsumptionV21 struct {
	ExpandedStates      int  `json:"expanded_states"`
	GeneratedCandidates int  `json:"generated_candidates"`
	DuplicateCandidates int  `json:"duplicate_candidates"`
	CycleCandidates     int  `json:"cycle_candidates"`
	DominatedCandidates int  `json:"dominated_candidates"`
	InvalidCandidates   int  `json:"invalid_candidates"`
	MaximumFrontier     int  `json:"maximum_frontier"`
	MaximumRetained     int  `json:"maximum_retained"`
	WorkConsumed        int  `json:"work_consumed"`
	BudgetExhausted     bool `json:"budget_exhausted"`
}

type TopologyCompletionPlanV21 struct {
	Schema           string                           `json:"schema"`
	Version          int                              `json:"version"`
	RequirementHash  string                           `json:"requirement_sha256"`
	InventoryHash    string                           `json:"inventory_sha256"`
	InitialGraphHash string                           `json:"initial_graph_sha256"`
	Limits           TopologyCompletionLimitsV21      `json:"limits"`
	Initial          TopologyInvariantReportV21       `json:"initial"`
	Consumption      TopologyCompletionConsumptionV21 `json:"consumption"`
	Candidates       []TopologyCandidateEvidenceV21   `json:"candidates"`
	Selected         *TopologyCandidateEvidenceV21    `json:"selected,omitempty"`
	Status           string                           `json:"status"`
	Issues           []reports.Issue                  `json:"issues"`
	Hash             string                           `json:"hash"`
}
