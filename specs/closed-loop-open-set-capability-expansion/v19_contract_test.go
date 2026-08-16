package closedloopopensetcontract

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestVersionNineteenEvaluatorIsFrozenBeforePublicEvaluation(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, directory, "V19_CONTRACT.sha256")
	v8VerifyManifest(t, directory, "V19_EVALUATOR.sha256")
	data := v7ReadFile(t, filepath.Join(directory, "V19_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                              string `json:"schema"`
		Version                             int    `json:"version"`
		FreezeParentCommit                  string `json:"freeze_parent_commit"`
		EvaluatorManifest                   string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256             string `json:"evaluator_manifest_sha256"`
		InheritedV18ManifestSHA256          string `json:"inherited_v18_evaluator_manifest_sha256"`
		CorpusManifestSHA256                string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256               string `json:"corpus_checksums_sha256"`
		DiscoveryCaseCount                  int    `json:"discovery_case_count"`
		ReplaysPerCase                      int    `json:"replays_per_case"`
		MaximumParallelCases                int    `json:"maximum_parallel_cases"`
		TransportSchemaVersion              int    `json:"transport_schema_version"`
		MaximumBeamDepth                    int    `json:"maximum_beam_depth"`
		MaximumBeamWidth                    int    `json:"maximum_beam_width"`
		BaseEvaluationsPerDepth             int    `json:"base_evaluations_per_depth"`
		MaximumEvaluatedCausalCandidates    int    `json:"maximum_evaluated_causal_candidates"`
		MaximumLogicalChangesPerProposal    int    `json:"maximum_logical_changes_per_proposal"`
		MaximumLogicalChangesPerPath        int    `json:"maximum_logical_changes_per_path"`
		MaximumPlateauChildrenPerParent     int    `json:"maximum_plateau_children_per_parent"`
		V19EnvironmentSeparate              bool   `json:"v19_environment_separate"`
		ExactV18EnvironmentSeparate         bool   `json:"exact_v18_environment_separate"`
		ExactV17EnvironmentSeparate         bool   `json:"exact_v17_environment_separate"`
		V18FirstDelegationRequired          bool   `json:"v18_first_delegation_required"`
		V18IneligibleResultsByteIdentical   bool   `json:"v18_ineligible_results_byte_identical"`
		UnsafeV18ResultsTerminal            bool   `json:"unsafe_v18_results_terminal"`
		GlobalGraphHashDeduplication        bool   `json:"global_graph_hash_deduplication"`
		DuplicateSimulationForbidden        bool   `json:"duplicate_simulation_forbidden"`
		InheritedPolicyLimitsUnchanged      bool   `json:"inherited_policy_limits_unchanged"`
		FixtureSpecificIdentifiersForbidden bool   `json:"fixture_specific_identifiers_forbidden"`
		InstalledKiCadPromotionRequired     bool   `json:"installed_kicad_promotion_required_for_pass"`
		ImmutableV10CorpusReused            bool   `json:"immutable_v10_corpus_reused"`
		HistoricalSourcesUnchanged          bool   `json:"historical_v6_v18_sources_unchanged"`
		ProductionPath                      bool   `json:"production_path"`
		HeldOutAccessSurface                bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated                 bool   `json:"real_corpus_evaluated"`
		PublicOutcomesObserved              bool   `json:"public_outcomes_observed"`
		ExternalKeyOpened                   bool   `json:"external_key_opened"`
	}
	decodeV11Strict(t, data, &freeze)
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v19" || freeze.Version != 19 ||
		freeze.FreezeParentCommit != "32f35623511bbb9a6030d26271dd971649006995" ||
		freeze.EvaluatorManifest != "V19_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.InheritedV18ManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V18_EVALUATOR.sha256")) ||
		freeze.CorpusManifestSHA256 != "0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94" ||
		freeze.CorpusChecksumsSHA256 != "24541ffe0f4ee372e0f1db5508eb22b102343f4eca2f376cacbca51cf275dcdf" {
		t.Fatalf("invalid V19 freeze identity: %+v", freeze)
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 ||
		freeze.TransportSchemaVersion != 17 || freeze.MaximumBeamDepth != 4 || freeze.MaximumBeamWidth != 8 ||
		freeze.BaseEvaluationsPerDepth != 12 || freeze.MaximumEvaluatedCausalCandidates != 48 ||
		freeze.MaximumLogicalChangesPerProposal != 2 || freeze.MaximumLogicalChangesPerPath != 8 ||
		freeze.MaximumPlateauChildrenPerParent != 2 || !freeze.V19EnvironmentSeparate ||
		!freeze.ExactV18EnvironmentSeparate || !freeze.ExactV17EnvironmentSeparate ||
		!freeze.V18FirstDelegationRequired || !freeze.V18IneligibleResultsByteIdentical ||
		!freeze.UnsafeV18ResultsTerminal || !freeze.GlobalGraphHashDeduplication ||
		!freeze.DuplicateSimulationForbidden || !freeze.InheritedPolicyLimitsUnchanged ||
		!freeze.FixtureSpecificIdentifiersForbidden || !freeze.InstalledKiCadPromotionRequired ||
		!freeze.ImmutableV10CorpusReused || !freeze.HistoricalSourcesUnchanged || !freeze.ProductionPath ||
		freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.PublicOutcomesObserved || freeze.ExternalKeyOpened {
		t.Fatalf("V19 freeze crossed its pre-evaluation boundary: %+v", freeze)
	}
	var exact map[string]any
	if err := json.Unmarshal(data, &exact); err != nil || len(exact) != 37 {
		t.Fatalf("V19 freeze shape = keys=%d error=%v", len(exact), err)
	}
}
