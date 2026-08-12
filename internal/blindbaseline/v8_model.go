package blindbaseline

import "io"

const (
	ManifestSchemaV8  = "kicadai.closed-loop-open-set-held-out-baseline-seal.v8"
	ManifestVersionV8 = 8
	AlgorithmV8       = "AES-256-GCM/v8-record-set/random-unique-nonce-per-record/length-delimited-aad"
	CipherFileV8      = "held_out_baseline.records.sealed"
	nonceBytesV8      = 12
	expectedCasesV8   = 18
)

// V8Binding commits every public input that may affect the generation-zero
// held-out baseline. It intentionally contains no held-out case, outcome,
// frontier, path, diagnostic, or timing data.
type V8Binding struct {
	StartingCommit                    string `json:"starting_commit"`
	ContractFreezeCommit              string `json:"contract_freeze_commit"`
	CorpusFreezeCommit                string `json:"corpus_freeze_commit"`
	EvaluatorFreezeCommit             string `json:"evaluator_freeze_commit"`
	SelectionFreezeCommit             string `json:"selection_freeze_commit"`
	PublisherParentCommit             string `json:"publisher_parent_commit"`
	CorpusManifestSHA256              string `json:"corpus_manifest_sha256"`
	ContractManifestSHA256            string `json:"contract_manifest_sha256"`
	ValidatorManifestSHA256           string `json:"validator_manifest_sha256"`
	CorpusPublisherManifestSHA256     string `json:"corpus_publisher_manifest_sha256"`
	BaselinePublisherManifestSHA256   string `json:"baseline_publisher_manifest_sha256"`
	ValidationReportSHA256            string `json:"validation_report_sha256"`
	PacketSetSHA256                   string `json:"packet_set_sha256"`
	ContractBindingSHA256             string `json:"contract_binding_sha256"`
	HistoricalCommitmentsSHA256       string `json:"historical_commitments_sha256"`
	SourceCiphertextSHA256            string `json:"source_ciphertext_sha256"`
	DiscoveryObligationsSHA256        string `json:"discovery_obligations_sha256"`
	HeldOutObligationCommitmentSHA256 string `json:"held_out_obligation_commitment_sha256"`
	DiscoveryBaselineSHA256           string `json:"discovery_baseline_sha256"`
	FrontierSHA256                    string `json:"frontier_sha256"`
	RankingSHA256                     string `json:"ranking_sha256"`
	SelectionSHA256                   string `json:"selection_sha256"`
	GenericPlanSHA256                 string `json:"generic_plan_sha256"`
	EvaluatorManifestSHA256           string `json:"evaluator_manifest_sha256"`
	EvaluatorPolicy                   string `json:"evaluator_policy"`
	GapRegistrySHA256                 string `json:"gap_registry_sha256"`
	ImpactRegistrySHA256              string `json:"impact_registry_sha256"`
	SynthesisPolicySHA256             string `json:"synthesis_policy_sha256"`
	GapPolicySHA256                   string `json:"gap_policy_sha256"`
	SelectionPolicySHA256             string `json:"selection_policy_sha256"`
	InventorySHA256                   string `json:"inventory_sha256"`
	CatalogSHA256                     string `json:"catalog_sha256"`
	ModelRegistrySHA256               string `json:"model_registry_sha256"`
	EnvironmentPolicySHA256           string `json:"environment_policy_sha256"`
	ResourceCeilingsSHA256            string `json:"resource_ceilings_sha256"`
	SeedSetSHA256                     string `json:"seed_set_sha256"`
	PromotionPlatform                 string `json:"promotion_platform"`
	KiCadVersion                      string `json:"kicad_version"`
	PromotionToolchainSHA256          string `json:"promotion_toolchain_sha256"`
	PromotionToolchainLockSHA256      string `json:"promotion_toolchain_lock_sha256"`
	KiCadCLISHA256                    string `json:"kicad_cli_sha256"`
	SymbolTableSHA256                 string `json:"symbol_table_sha256"`
	FootprintTableSHA256              string `json:"footprint_table_sha256"`
	SymbolsSHA256                     string `json:"symbols_sha256"`
	FootprintsSHA256                  string `json:"footprints_sha256"`
}

func (binding V8Binding) fields() []bindingField {
	return []bindingField{
		{"starting_commit", binding.StartingCommit, bindingCommit},
		{"contract_freeze_commit", binding.ContractFreezeCommit, bindingCommit},
		{"corpus_freeze_commit", binding.CorpusFreezeCommit, bindingCommit},
		{"evaluator_freeze_commit", binding.EvaluatorFreezeCommit, bindingCommit},
		{"selection_freeze_commit", binding.SelectionFreezeCommit, bindingCommit},
		{"publisher_parent_commit", binding.PublisherParentCommit, bindingCommit},
		{"corpus_manifest_sha256", binding.CorpusManifestSHA256, bindingHash},
		{"contract_manifest_sha256", binding.ContractManifestSHA256, bindingHash},
		{"validator_manifest_sha256", binding.ValidatorManifestSHA256, bindingHash},
		{"corpus_publisher_manifest_sha256", binding.CorpusPublisherManifestSHA256, bindingHash},
		{"baseline_publisher_manifest_sha256", binding.BaselinePublisherManifestSHA256, bindingHash},
		{"validation_report_sha256", binding.ValidationReportSHA256, bindingHash},
		{"packet_set_sha256", binding.PacketSetSHA256, bindingHash},
		{"contract_binding_sha256", binding.ContractBindingSHA256, bindingHash},
		{"historical_commitments_sha256", binding.HistoricalCommitmentsSHA256, bindingHash},
		{"source_ciphertext_sha256", binding.SourceCiphertextSHA256, bindingHash},
		{"discovery_obligations_sha256", binding.DiscoveryObligationsSHA256, bindingHash},
		{"held_out_obligation_commitment_sha256", binding.HeldOutObligationCommitmentSHA256, bindingHash},
		{"discovery_baseline_sha256", binding.DiscoveryBaselineSHA256, bindingHash},
		{"frontier_sha256", binding.FrontierSHA256, bindingHash},
		{"ranking_sha256", binding.RankingSHA256, bindingHash},
		{"selection_sha256", binding.SelectionSHA256, bindingHash},
		{"generic_plan_sha256", binding.GenericPlanSHA256, bindingHash},
		{"evaluator_manifest_sha256", binding.EvaluatorManifestSHA256, bindingHash},
		{"evaluator_policy", binding.EvaluatorPolicy, bindingIdentifier},
		{"gap_registry_sha256", binding.GapRegistrySHA256, bindingHash},
		{"impact_registry_sha256", binding.ImpactRegistrySHA256, bindingHash},
		{"synthesis_policy_sha256", binding.SynthesisPolicySHA256, bindingHash},
		{"gap_policy_sha256", binding.GapPolicySHA256, bindingHash},
		{"selection_policy_sha256", binding.SelectionPolicySHA256, bindingHash},
		{"inventory_sha256", binding.InventorySHA256, bindingHash},
		{"catalog_sha256", binding.CatalogSHA256, bindingHash},
		{"model_registry_sha256", binding.ModelRegistrySHA256, bindingHash},
		{"environment_policy_sha256", binding.EnvironmentPolicySHA256, bindingHash},
		{"resource_ceilings_sha256", binding.ResourceCeilingsSHA256, bindingHash},
		{"seed_set_sha256", binding.SeedSetSHA256, bindingHash},
		{"promotion_platform", binding.PromotionPlatform, bindingPlatform},
		{"kicad_version", binding.KiCadVersion, bindingVersion},
		{"promotion_toolchain_sha256", binding.PromotionToolchainSHA256, bindingHash},
		{"promotion_toolchain_lock_sha256", binding.PromotionToolchainLockSHA256, bindingHash},
		{"kicad_cli_sha256", binding.KiCadCLISHA256, bindingHash},
		{"symbol_table_sha256", binding.SymbolTableSHA256, bindingHash},
		{"footprint_table_sha256", binding.FootprintTableSHA256, bindingHash},
		{"symbols_sha256", binding.SymbolsSHA256, bindingHash},
		{"footprints_sha256", binding.FootprintsSHA256, bindingHash},
	}
}

// V8Manifest is the complete non-revealing public commitment to one frozen
// 18-record blind baseline publication.
type V8Manifest struct {
	Schema                   string    `json:"schema"`
	Version                  int       `json:"version"`
	Algorithm                string    `json:"algorithm"`
	Binding                  V8Binding `json:"binding"`
	CiphertextSHA256         string    `json:"ciphertext_sha256"`
	PlaintextAggregateSHA256 string    `json:"plaintext_aggregate_sha256"`
	AADAggregateSHA256       string    `json:"aad_aggregate_sha256"`
	RecordCiphertextSHA256   []string  `json:"record_ciphertext_sha256"`
	NonceBytes               int       `json:"nonce_bytes"`
	CaseCount                int       `json:"case_count"`
	Hash                     string    `json:"hash"`
}

// V8Request supplies opaque per-case evidence and the complete public binding
// to the isolated publisher. Records must already be in frozen corpus order.
type V8Request struct {
	RepositoryRoot   string
	DestinationRoot  string
	KeyPath          string
	ReservedKeyPaths []string
	Binding          V8Binding
	Records          [][]byte
	Random           io.Reader
}

// V8Result reports the exact manifest published by PublishV8.
type V8Result struct {
	Manifest V8Manifest
}
