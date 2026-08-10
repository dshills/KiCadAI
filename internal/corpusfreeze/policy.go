package corpusfreeze

func V5Policy() Policy {
	return Policy{
		AssignmentSchema:               "kicadai.closed-loop-open-set-author-assignment.v5",
		AuthorshipSchema:               "kicadai.closed-loop-open-set-authorship.v5",
		Version:                        5,
		AuthorSlots:                    []string{"author_1", "author_2", "author_3"},
		Roles:                          []string{RoleDiscovery, RoleHeldOut},
		Domains:                        []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"},
		SafetyImpacts:                  []string{"non_safety", "review_required", "safety_relevant", "safety_critical"},
		CasesPerAuthorRoleDomain:       1,
		MinimumOperatingCases:          2,
		MinimumAssertions:              4,
		MinimumAnalysesPerRequirement:  2,
		MinimumAnalysisKindsPerAuthor:  4,
		MinimumEventKindsPerAuthor:     3,
		RequiredSupplyConfigurations:   []string{"single_positive", "bipolar", "multiple"},
		RequiredObservationKinds:       []string{"voltage", "current", "power"},
		RequiredAnalysisCategories:     []string{"dc", "ac_noise_stability", "transient_startup", "thermal"},
		RequiredVariationCategories:    []string{"load", "tolerance_model", "temperature", "supply"},
		RequiredEventKinds:             []string{"input_step", "load_step", "power_step", "startup", "rail_loss", "short_circuit"},
		MinimumMultiOutputPerRole:      5,
		MinimumConvergingInputsPerRole: 5,
		MinimumCriticalDomainsPerRole:  4,
		PacketSetSHA256:                "004dc3ab1325e34d12190cf0358adb607597f2e7bc8fff44eb412309b63c42b9",
		HistoricalCommitmentsSHA256:    "0de93fd451ab322d6b0dbaaf1a74cc088e208dce28ea22e6f4513435bc95e700",
		ProhibitedIdentityPrefixes:     []string{"v5_case_", "v5_source_"},
		ProhibitedTerms: []string{
			"allowlist", "block family", "candidate", "component", "coordinate", "fixture",
			"footprint", "manufacturer", "model id", "net", "package", "pad", "part number",
			"pin", "placement", "provider", "reference designator", "repair", "route", "solver",
			"symbol", "template", "topology", "track", "via",
		},
	}
}
