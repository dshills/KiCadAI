// Package capabilityselectionv10 converts an authenticated V10 public
// baseline and mechanically evidenced generic effect plans into the frozen,
// deterministic rank-one capability selection.
package capabilityselectionv10

import "kicadai/internal/capabilityroundsv10"

const (
	PlanSetSchema = "kicadai.closed-loop-open-set-effect-plan-set.v10"
	RankingSchema = "kicadai.closed-loop-open-set-ranking.v10"
	Version       = 10

	// EffectExposureEngineManifestSHA256 binds the already-frozen V10
	// selection implementation adopted from V9.
	EffectExposureEngineManifestSHA256 = "e8f2e796efe5a10a6c2d88039685112b9e4efa2a43dd607bf7b3174b8dce6c2e"
)

type FileEvidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type StaticEvidence struct {
	ProductionFiles                  []FileEvidence `json:"production_files"`
	VerificationFiles                []FileEvidence `json:"verification_files"`
	ReverseCallGraph                 []string       `json:"reverse_call_graph"`
	RegistryReferences               []string       `json:"registry_references"`
	ConfigurationLoaderReferences    []string       `json:"configuration_loader_references"`
	CatalogModelReferences           []string       `json:"catalog_model_references"`
	DataReferences                   []string       `json:"data_references"`
	FocusedNonCorpusRuntimeConsumers []string       `json:"focused_non_corpus_runtime_consumers"`
}

type Plan struct {
	DirectAtomKeys         []string                     `json:"direct_atom_keys"`
	DirectMemberKeys       []string                     `json:"direct_member_keys"`
	ClosureAtoms           []capabilityroundsv10.Atom   `json:"closure_atoms"`
	ClosureMembers         []capabilityroundsv10.Member `json:"closure_members"`
	PlannedMemberKeys      []string                     `json:"planned_member_keys"`
	RequiredEvidence       []string                     `json:"required_evidence"`
	Executable             bool                         `json:"executable"`
	MechanicallyProven     bool                         `json:"mechanically_proven"`
	UnboundedDynamicLookup bool                         `json:"unbounded_dynamic_lookup"`
	UnmappedConsumers      []string                     `json:"unmapped_consumers"`
	StaticEvidence         StaticEvidence               `json:"static_evidence"`
}

type PlanSet struct {
	Schema                     string `json:"schema"`
	Version                    int    `json:"version"`
	Generation                 int    `json:"generation"`
	BaselineManifestSHA256     string `json:"baseline_manifest_sha256"`
	BaselineReportSHA256       string `json:"baseline_report_sha256"`
	EffectExposureEngineSHA256 string `json:"effect_exposure_engine_sha256"`
	Plans                      []Plan `json:"plans"`
}

type Ranking struct {
	Schema                     string                         `json:"schema"`
	Version                    int                            `json:"version"`
	Generation                 int                            `json:"generation"`
	BaselineManifestSHA256     string                         `json:"baseline_manifest_sha256"`
	BaselineReportSHA256       string                         `json:"baseline_report_sha256"`
	PlanSetSHA256              string                         `json:"plan_set_sha256"`
	EffectExposureEngineSHA256 string                         `json:"effect_exposure_engine_sha256"`
	State                      capabilityroundsv10.RoundState `json:"state"`
	Selection                  capabilityroundsv10.Selection  `json:"selection"`
	Hash                       string                         `json:"hash"`
}
