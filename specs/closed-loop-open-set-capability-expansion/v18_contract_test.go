package closedloopopensetcontract

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestVersionEighteenEvaluatorIsFrozenPublicOnlyAndVersionIsolated(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, directory, "V18_CONTRACT.sha256")
	v8VerifyManifest(t, directory, "V18_EVALUATOR.sha256")
	data := v7ReadFile(t, filepath.Join(directory, "V18_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                               string `json:"schema"`
		Version                              int    `json:"version"`
		FreezeParentCommit                   string `json:"freeze_parent_commit"`
		EvaluatorManifest                    string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256              string `json:"evaluator_manifest_sha256"`
		InheritedV17ManifestSHA256           string `json:"inherited_v17_evaluator_manifest_sha256"`
		CorpusManifestSHA256                 string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256                string `json:"corpus_checksums_sha256"`
		DiscoveryCaseCount                   int    `json:"discovery_case_count"`
		ReplaysPerCase                       int    `json:"replays_per_case"`
		MaximumParallelCases                 int    `json:"maximum_parallel_cases"`
		TransportSchemaVersion               int    `json:"transport_schema_version"`
		CatalogExtensionSeparate             bool   `json:"v18_catalog_extension_separate"`
		LegacyCatalogHashUnchanged           bool   `json:"legacy_catalog_hash_unchanged"`
		LegacyModelRegistryHashUnchanged     bool   `json:"legacy_model_registry_hash_unchanged"`
		NoneligibleRequirementsDelegateV17   bool   `json:"noneligible_requirements_delegate_v17"`
		GenericThresholdCompositionOnly      bool   `json:"generic_threshold_composition_only"`
		FixtureSpecificIdentifiersForbidden bool   `json:"fixture_specific_identifiers_forbidden"`
		InstalledKiCadPromotionRequired      bool   `json:"installed_kicad_promotion_required_for_pass"`
		ImmutableV10CorpusReused             bool   `json:"immutable_v10_corpus_reused"`
		HistoricalSourcesUnchanged           bool   `json:"historical_v6_v17_sources_unchanged"`
		ProductionPath                       bool   `json:"production_path"`
		HeldOutAccessSurface                 bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated                  bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened                    bool   `json:"external_key_opened"`
	}
	decodeV11Strict(t, data, &freeze)
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v18" || freeze.Version != 18 ||
		freeze.FreezeParentCommit == "" || freeze.EvaluatorManifest != "V18_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.InheritedV17ManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V17_EVALUATOR.sha256")) ||
		freeze.CorpusManifestSHA256 != "0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94" ||
		freeze.CorpusChecksumsSHA256 != "24541ffe0f4ee372e0f1db5508eb22b102343f4eca2f376cacbca51cf275dcdf" {
		t.Fatalf("invalid V18 freeze identity: %+v", freeze)
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 ||
		freeze.TransportSchemaVersion != 17 || !freeze.CatalogExtensionSeparate ||
		!freeze.LegacyCatalogHashUnchanged || !freeze.LegacyModelRegistryHashUnchanged ||
		!freeze.NoneligibleRequirementsDelegateV17 || !freeze.GenericThresholdCompositionOnly ||
		!freeze.FixtureSpecificIdentifiersForbidden || !freeze.InstalledKiCadPromotionRequired ||
		!freeze.ImmutableV10CorpusReused || !freeze.HistoricalSourcesUnchanged || !freeze.ProductionPath ||
		freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatalf("V18 freeze crossed its public-only versioned boundary: %+v", freeze)
	}
	var exact map[string]any
	if err := json.Unmarshal(data, &exact); err != nil || len(exact) != 25 {
		t.Fatalf("V18 freeze shape = keys=%d error=%v", len(exact), err)
	}
}
