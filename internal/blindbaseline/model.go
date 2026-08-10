// Package blindbaseline atomically publishes encrypted held-out baseline
// evidence without exposing case outcomes or diagnostics.
package blindbaseline

import "io"

const (
	ManifestSchema  = "kicadai.closed-loop-open-set-held-out-baseline-seal.v5"
	ManifestVersion = 5
	Algorithm       = "AES-256-GCM/random-nonce-prefixed/length-delimited-aad"
	CipherFile      = "held_out_baseline.sealed"
	ManifestFile    = "manifest.json"
	AuditFile       = "BASELINE_AUDIT.md"
	ChecksumFile    = "CHECKSUMS.sha256"
	maximumCases    = 1000
)

type Binding struct {
	StartingCommit               string `json:"starting_commit"`
	ContractFreezeCommit         string `json:"contract_freeze_commit"`
	CorpusFreezeCommit           string `json:"corpus_freeze_commit"`
	SelectionFreezeCommit        string `json:"selection_freeze_commit"`
	PublisherParentCommit        string `json:"publisher_parent_commit"`
	CorpusManifestSHA256         string `json:"corpus_manifest_sha256"`
	ContractManifestSHA256       string `json:"contract_manifest_sha256"`
	ValidatorManifestSHA256      string `json:"validator_manifest_sha256"`
	PublisherManifestSHA256      string `json:"publisher_manifest_sha256"`
	ValidationReportSHA256       string `json:"validation_report_sha256"`
	PacketSetSHA256              string `json:"packet_set_sha256"`
	ContractBindingSHA256        string `json:"contract_binding_sha256"`
	HistoricalCommitmentsSHA256  string `json:"historical_commitments_sha256"`
	SourceCiphertextSHA256       string `json:"source_ciphertext_sha256"`
	DiscoveryBaselineSHA256      string `json:"discovery_baseline_sha256"`
	RankingSHA256                string `json:"ranking_sha256"`
	SelectionSHA256              string `json:"selection_sha256"`
	GenericPlanSHA256            string `json:"generic_plan_sha256"`
	EvaluatorPolicy              string `json:"evaluator_policy"`
	ImpactRegistrySHA256         string `json:"impact_registry_sha256"`
	SynthesisPolicySHA256        string `json:"synthesis_policy_sha256"`
	GapPolicySHA256              string `json:"gap_policy_sha256"`
	SelectionPolicySHA256        string `json:"selection_policy_sha256"`
	ImplementationManifestSHA256 string `json:"implementation_manifest_sha256"`
	InventorySHA256              string `json:"inventory_sha256"`
	CatalogSHA256                string `json:"catalog_sha256"`
	ModelRegistrySHA256          string `json:"model_registry_sha256"`
	EnvironmentPolicySHA256      string `json:"environment_policy_sha256"`
	PromotionPlatform            string `json:"promotion_platform"`
	KiCadVersion                 string `json:"kicad_version"`
	PromotionToolchainSHA256     string `json:"promotion_toolchain_sha256"`
	PromotionToolchainLockSHA256 string `json:"promotion_toolchain_lock_sha256"`
	KiCadCLISHA256               string `json:"kicad_cli_sha256"`
	SymbolTableSHA256            string `json:"symbol_table_sha256"`
	FootprintTableSHA256         string `json:"footprint_table_sha256"`
	SymbolsSHA256                string `json:"symbols_sha256"`
	FootprintsSHA256             string `json:"footprints_sha256"`
}

type bindingFieldKind uint8

const (
	bindingCommit bindingFieldKind = iota + 1
	bindingHash
	bindingIdentifier
	bindingPlatform
	bindingVersion
)

type bindingField struct {
	name  string
	value string
	kind  bindingFieldKind
}

// fields is the single normative order for validation and authenticated
// commitment encoding. model_test.go proves it matches every Binding JSON
// field exactly once and in declaration order.
func (binding Binding) fields() []bindingField {
	return []bindingField{
		{"starting_commit", binding.StartingCommit, bindingCommit},
		{"contract_freeze_commit", binding.ContractFreezeCommit, bindingCommit},
		{"corpus_freeze_commit", binding.CorpusFreezeCommit, bindingCommit},
		{"selection_freeze_commit", binding.SelectionFreezeCommit, bindingCommit},
		{"publisher_parent_commit", binding.PublisherParentCommit, bindingCommit},
		{"corpus_manifest_sha256", binding.CorpusManifestSHA256, bindingHash},
		{"contract_manifest_sha256", binding.ContractManifestSHA256, bindingHash},
		{"validator_manifest_sha256", binding.ValidatorManifestSHA256, bindingHash},
		{"publisher_manifest_sha256", binding.PublisherManifestSHA256, bindingHash},
		{"validation_report_sha256", binding.ValidationReportSHA256, bindingHash},
		{"packet_set_sha256", binding.PacketSetSHA256, bindingHash},
		{"contract_binding_sha256", binding.ContractBindingSHA256, bindingHash},
		{"historical_commitments_sha256", binding.HistoricalCommitmentsSHA256, bindingHash},
		{"source_ciphertext_sha256", binding.SourceCiphertextSHA256, bindingHash},
		{"discovery_baseline_sha256", binding.DiscoveryBaselineSHA256, bindingHash},
		{"ranking_sha256", binding.RankingSHA256, bindingHash},
		{"selection_sha256", binding.SelectionSHA256, bindingHash},
		{"generic_plan_sha256", binding.GenericPlanSHA256, bindingHash},
		{"evaluator_policy", binding.EvaluatorPolicy, bindingIdentifier},
		{"impact_registry_sha256", binding.ImpactRegistrySHA256, bindingHash},
		{"synthesis_policy_sha256", binding.SynthesisPolicySHA256, bindingHash},
		{"gap_policy_sha256", binding.GapPolicySHA256, bindingHash},
		{"selection_policy_sha256", binding.SelectionPolicySHA256, bindingHash},
		{"implementation_manifest_sha256", binding.ImplementationManifestSHA256, bindingHash},
		{"inventory_sha256", binding.InventorySHA256, bindingHash},
		{"catalog_sha256", binding.CatalogSHA256, bindingHash},
		{"model_registry_sha256", binding.ModelRegistrySHA256, bindingHash},
		{"environment_policy_sha256", binding.EnvironmentPolicySHA256, bindingHash},
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

type Manifest struct {
	Schema           string  `json:"schema"`
	Version          int     `json:"version"`
	Algorithm        string  `json:"algorithm"`
	Binding          Binding `json:"binding"`
	PayloadSHA256    string  `json:"payload_sha256"`
	CiphertextSHA256 string  `json:"ciphertext_sha256"`
	AADSHA256        string  `json:"aad_sha256"`
	NonceBytes       int     `json:"nonce_bytes"`
	CaseCount        int     `json:"case_count"`
	Hash             string  `json:"hash"`
}

type Request struct {
	RepositoryRoot   string
	DestinationRoot  string
	KeyPath          string
	ReservedKeyPaths []string
	Binding          Binding
	Payload          []byte
	CaseCount        int
	Random           io.Reader
}

type Result struct {
	Manifest Manifest
}
