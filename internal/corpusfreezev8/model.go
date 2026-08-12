package corpusfreezev8

import "time"

type Assignment struct {
	Schema     string            `json:"schema"`
	Version    int               `json:"version"`
	AuthorSlot string            `json:"author_slot"`
	Entries    []AssignmentEntry `json:"entries"`
}

type AssignmentEntry struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Domain          string `json:"domain"`
	CircuitRole     string `json:"circuit_role"`
	SafetyImpact    string `json:"safety_impact"`
	SourceID        string `json:"source_id"`
	RequirementFile string `json:"requirement_file"`
}

type Authorship struct {
	Schema                    string                 `json:"schema"`
	Version                   int                    `json:"version"`
	AuthorContextIdentity     string                 `json:"author_context_identity"`
	AuthorSlot                string                 `json:"author_slot"`
	AuthoringToolModelVersion string                 `json:"authoring_tool_model_version"`
	AuthoringStartedUTC       string                 `json:"authoring_started_utc"`
	AuthoringEndedUTC         string                 `json:"authoring_ended_utc"`
	PerAuthorPacketManifest   string                 `json:"per_author_packet_manifest"`
	PerAuthorPacketSHA256     string                 `json:"per_author_packet_sha256"`
	ContractBindingSHA256     string                 `json:"contract_binding_sha256"`
	AssignmentSHA256          string                 `json:"assignment_sha256"`
	ReturnedBundleRoot        string                 `json:"returned_bundle_root"`
	RequirementSourceSHA256   []SourceHash           `json:"requirement_source_sha256"`
	Uncertainties             []string               `json:"uncertainties"`
	Attestations              AuthorshipAttestations `json:"attestations"`
}

type SourceHash struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type AuthorshipAttestations struct {
	PacketOnlyInput                                         bool `json:"packet_only_input"`
	ContractBoundBeforeAuthoring                            bool `json:"contract_bound_before_authoring"`
	NoRepositoryOrPriorCorpusAccess                         bool `json:"no_repository_or_prior_corpus_access"`
	NoCrossAuthorAssignmentOrContentAccess                  bool `json:"no_cross_author_assignment_or_content_access"`
	IndependentlyConceivedBehaviorOnlyRequirements          bool `json:"independently_conceived_behavior_only_requirements"`
	NoSynthesisSimulationClassificationRankingOrFeasibility bool `json:"no_synthesis_simulation_classification_ranking_or_feasibility"`
	FixedDiscoveryHeldOutMembership                         bool `json:"fixed_discovery_held_out_membership"`
	NoImplementationOrExpectedOutcomePrescription           bool `json:"no_implementation_or_expected_outcome_prescription"`
	NoObligationAnchorOrCausalPathAuthorship                bool `json:"no_obligation_anchor_or_causal_path_authorship"`
	NoPostEvaluationInspectionOrModification                bool `json:"no_post_evaluation_inspection_or_modification"`
	AllUncertaintiesDisclosed                               bool `json:"all_uncertainties_disclosed"`
}

func (a AuthorshipAttestations) allTrue() bool {
	return a.PacketOnlyInput && a.ContractBoundBeforeAuthoring && a.NoRepositoryOrPriorCorpusAccess &&
		a.NoCrossAuthorAssignmentOrContentAccess && a.IndependentlyConceivedBehaviorOnlyRequirements &&
		a.NoSynthesisSimulationClassificationRankingOrFeasibility && a.FixedDiscoveryHeldOutMembership &&
		a.NoImplementationOrExpectedOutcomePrescription && a.NoObligationAnchorOrCausalPathAuthorship &&
		a.NoPostEvaluationInspectionOrModification && a.AllUncertaintiesDisclosed
}

type Binding struct {
	PacketSetSHA256       string
	ContractBindingSHA256 string
	AuthorPacketSHA256    map[string]string
	AssignmentSHA256      map[string]string
}

type Packet struct {
	Assignments map[string][]byte
	Binding     Binding
}

type EntryEvidence struct {
	ID                       string `json:"id"`
	AuthorSlot               string `json:"author_slot"`
	Role                     string `json:"role"`
	Domain                   string `json:"domain"`
	CircuitRole              string `json:"circuit_role"`
	SafetyImpact             string `json:"safety_impact"`
	SourceID                 string `json:"source_id"`
	RequirementFile          string `json:"requirement_file"`
	RequirementSHA256        string `json:"requirement_sha256"`
	NeutralSemanticSHA256    string `json:"neutral_semantic_sha256"`
	NormalizedSemanticSHA256 string `json:"normalized_semantic_sha256"`
}

type Report struct {
	Schema                      string                    `json:"schema"`
	Version                     int                       `json:"version"`
	PolicySHA256                string                    `json:"policy_sha256"`
	PacketSetSHA256             string                    `json:"packet_set_sha256"`
	ContractBindingSHA256       string                    `json:"contract_binding_sha256"`
	HistoricalCommitmentsSHA256 string                    `json:"historical_commitments_sha256"`
	AuthorPacketSHA256          map[string]string         `json:"author_packet_sha256"`
	AssignmentSHA256            map[string]string         `json:"assignment_sha256"`
	AuthorshipSHA256            map[string]string         `json:"authorship_sha256"`
	Entries                     []EntryEvidence           `json:"entries"`
	Counts                      map[string]map[string]int `json:"counts"`
	AuthorStartedAt             map[string]time.Time      `json:"author_started_at"`
	AuthorEndedAt               map[string]time.Time      `json:"author_ended_at"`
}
