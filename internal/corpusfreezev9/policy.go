// Package corpusfreezev9 implements the V9 independent-corpus validator core.
// The final freeze pins the complete V1-V8 history digest; this package never
// invokes synthesis or outcome logic.
package corpusfreezev9

const (
	PacketSetSHA256 = "276c9741b299a75e9838bd2aab6e48683634fad7d8db77446eb5676e4a6af6a1"
	AuthorCount     = 6
	CasesPerAuthor  = 8
	CorpusCaseCount = AuthorCount * CasesPerAuthor
)

type Policy struct {
	AssignmentSchema                string
	AuthorshipSchema                string
	Version                         int
	AuthorSlots                     []string
	Roles                           []string
	Domains                         []string
	CircuitRoles                    []string
	SafetyImpacts                   []string
	CasesPerAuthor                  int
	MinimumOperatingCases           int
	MinimumAssertions               int
	MinimumAnalysisKindsPerAuthor   int
	MinimumStaticCasesPerAuthor     int
	MinimumDynamicCasesPerAuthor    int
	MinimumOffNominalPerAuthor      int
	MinimumMultiOutputPerRoleAuthor int
	ProhibitedIdentityPrefixes      []string
	ProhibitedTerms                 []string
	PacketSetSHA256                 string
	HistoricalCommitmentsSHA256     string
}

var analyses = []string{
	"ac_sweep", "dc_operating_point", "dc_sweep", "distortion", "electrothermal",
	"noise", "stability", "startup", "thermal", "transient",
}

// PolicyForHistory constructs the otherwise-frozen V9 policy around an exact
// sanitized V1-V8 commitment artifact. The digest remains an explicit input
// until the isolated history custodian has extended V8 without exposing its
// encrypted held-out records; the final validator freeze pins that digest.
func PolicyForHistory(historicalCommitmentsSHA256 string) Policy {
	return Policy{
		AssignmentSchema: "kicadai.closed-loop-open-set-author-assignment.v9",
		AuthorshipSchema: "kicadai.closed-loop-open-set-authorship.v9",
		Version:          9,
		AuthorSlots:      []string{"author_1", "author_2", "author_3", "author_4", "author_5", "author_6"},
		Roles:            []string{"discovery", "held_out"},
		Domains: []string{
			"analog_signal_path", "power_energy_conversion", "digital_control",
			"mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity",
		},
		CircuitRoles: []string{
			"source_bias", "amplification_conditioning", "conversion_regulation",
			"sensing_measurement", "interface_control", "protection_supervision",
		},
		SafetyImpacts:  []string{"non_safety", "review_required", "safety_relevant", "safety_critical"},
		CasesPerAuthor: CasesPerAuthor, MinimumOperatingCases: 2, MinimumAssertions: 4,
		MinimumAnalysisKindsPerAuthor: 8, MinimumStaticCasesPerAuthor: 4, MinimumDynamicCasesPerAuthor: 4,
		MinimumOffNominalPerAuthor: 2, MinimumMultiOutputPerRoleAuthor: 1,
		PacketSetSHA256: PacketSetSHA256, HistoricalCommitmentsSHA256: historicalCommitmentsSHA256,
		ProhibitedIdentityPrefixes: []string{"v9_case_", "v9_source_"},
		ProhibitedTerms: []string{
			"allowlist", "block family", "candidate", "component", "coordinate", "fixture",
			"footprint", "manufacturer", "model id", "net", "package", "pad", "part number",
			"pin", "placement", "provider", "reference designator", "repair", "route", "solver",
			"symbol", "template", "topology", "track", "via",
		},
	}
}

func Analyses() []string { return append([]string(nil), analyses...) }
