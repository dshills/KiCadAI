package capabilityrounds

import "errors"

const (
	IdentityEncodingU32BigEndian = "ordered_utf8_fields_each_prefixed_by_u32_big_endian_byte_length"
	TieBehaviorCanonicalFallback = "publish_complete_semantic_co_rank_one_set_then_select_canonical_bundle_key_asc"
	UnknownStageFailClosed       = "fail_closed"
)

var (
	ErrInvalidInput      = errors.New("invalid adaptive capability-round input")
	ErrInvalidPolicy     = errors.New("invalid adaptive capability-round policy")
	ErrCandidateOverflow = errors.New("adaptive capability-round candidate ceiling exceeded")
	ErrNoEligibleBundle  = errors.New("no eligible adaptive capability-round bundle")
	ErrRoundGate         = errors.New("adaptive capability-round gate failed")
)

type Policy struct {
	EligibleOutcomes             []string
	ExpectedDiscoveryCaseCount   int
	MaximumRounds                int
	MaximumTotalCapabilityAtoms  int
	MaximumTotalExactMembers     int
	MaximumRoundCapabilityAtoms  int
	MaximumRoundExactMembers     int
	MaximumCandidateBundles      int
	MinimumAtomActiveCaseSupport int
	MinimumAdvancedActiveCases   int
	MinimumReportingDomains      int
	IdentityEncoding             string
	Ranking                      []string
	TieBehavior                  string
	UnknownGapStage              string
	StageOrdinal                 map[string]int
	StageAliases                 map[string]string
	SafetyWeights                map[string]int64
}

type RoundState struct {
	Generation          int
	UsedCapabilityAtoms int
	UsedExactMembers    int
	PriorAtomKeys       []string
}

type Gap struct {
	Stage            string
	Scope            string
	Capability       string
	Code             string
	CausalToken      string
	RequiredEvidence []string
}

type Case struct {
	ID              string
	Role            string
	ReportingDomain string
	SafetyImpact    string
	Outcome         string
	Frontier        []Gap
}

type Atom struct {
	Key        string
	Scope      string
	Capability string
	CaseCount  int
}

type Member struct {
	Key        string
	Stage      string
	Scope      string
	Capability string
	Code       string
}

type Candidate struct {
	Key              string
	Atoms            []Atom
	Members          []Member
	CoveredCaseIDs   []string
	ReportingDomains []string
	SafetyWeight     int64
}

type Selection struct {
	Generation         int
	CandidateCount     int
	EligibleCandidates []Candidate
	CoRankOne          []Candidate
	Selected           Candidate
}

type RoundEvidence struct {
	DeterministicReplayComplete bool
	PhysicalPromotionComplete   bool
	SealEnvironmentValid        bool
}

type LineageEdge struct {
	CaseID string
	From   Gap
	To     Gap
}

type EvaluationStatus string

const (
	EvaluationContinue       EvaluationStatus = "continue"
	EvaluationPublicAdmitted EvaluationStatus = "public_admitted"
)

type Evaluation struct {
	Status                   EvaluationStatus
	DiscoveryPassBefore      int
	DiscoveryPassAfter       int
	NewActiveCohortPasses    int
	AdvancedCaseIDs          []string
	AdvancedReportingDomains []string
	NextState                RoundState
}
