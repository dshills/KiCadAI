// Package corpusfreezev10 implements the V10 independent-corpus validator core.
// The final freeze pins the digest-only V1-V8 history artifact retained after
// V9 retired before corpus publication; this package never
// invokes synthesis or outcome logic.
package corpusfreezev10

const (
	PacketSetSHA256 = "77804df628be0979727ea2821c272f1eb2b39483db45ee6cd91284c54086d423"
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

// PolicyForHistory constructs the otherwise-frozen V10 policy around an exact
// sanitized V1-V8 commitment artifact published during V9. V9 added no corpus
// history because it retired before publication, so the exact artifact remains
// the V10 predecessor boundary.
func PolicyForHistory(historicalCommitmentsSHA256 string) Policy {
	return Policy{
		AssignmentSchema: "kicadai.closed-loop-open-set-author-assignment.v10",
		AuthorshipSchema: "kicadai.closed-loop-open-set-authorship.v10",
		Version:          10,
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
		ProhibitedIdentityPrefixes: []string{"v10_case_", "v10_source_"},
		ProhibitedTerms: []string{
			"allowlist", "block family", "candidate", "component", "coordinate", "fixture",
			"footprint", "manufacturer", "model id", "net", "package", "pad", "part number",
			"pin", "placement", "provider", "reference designator", "repair", "route", "solver",
			"symbol", "template", "topology", "track", "via",
		},
	}
}

func Analyses() []string { return append([]string(nil), analyses...) }
