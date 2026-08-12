// Package corpusfreezev8 implements the frozen V8 independent-corpus boundary.
// It validates behavior-only inputs and never invokes synthesis or outcome logic.
package corpusfreezev8

const (
	PacketSetSHA256             = "5a243103b6dee088470a521617a88f33685cf2bfb170c68cffa0e1f93bfacc76"
	HistoricalCommitmentsSHA256 = "f56d30c27b30e90f4c8568e06870718bac7e9db7d29ed24dac6c768ad163cebf"
	AuthorCount                 = 6
	CasesPerAuthor              = 6
	CorpusCaseCount             = AuthorCount * CasesPerAuthor
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

func FrozenPolicy() Policy {
	return Policy{
		AssignmentSchema: "kicadai.closed-loop-open-set-author-assignment.v8",
		AuthorshipSchema: "kicadai.closed-loop-open-set-authorship.v8",
		Version:          8,
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
		MinimumAnalysisKindsPerAuthor: 4, MinimumStaticCasesPerAuthor: 2, MinimumDynamicCasesPerAuthor: 2,
		MinimumOffNominalPerAuthor: 2, MinimumMultiOutputPerRoleAuthor: 1,
		PacketSetSHA256: PacketSetSHA256, HistoricalCommitmentsSHA256: HistoricalCommitmentsSHA256,
		ProhibitedIdentityPrefixes: []string{"v8_case_", "v8_source_"},
		ProhibitedTerms: []string{
			"allowlist", "block family", "candidate", "component", "coordinate", "fixture",
			"footprint", "manufacturer", "model id", "net", "package", "pad", "part number",
			"pin", "placement", "provider", "reference designator", "repair", "route", "solver",
			"symbol", "template", "topology", "track", "via",
		},
	}
}

func Analyses() []string { return append([]string(nil), analyses...) }
