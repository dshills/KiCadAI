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
	StartingCommit         string `json:"starting_commit"`
	ContractFreezeCommit   string `json:"contract_freeze_commit"`
	CorpusFreezeCommit     string `json:"corpus_freeze_commit"`
	SelectionFreezeCommit  string `json:"selection_freeze_commit"`
	PublisherParentCommit  string `json:"publisher_parent_commit"`
	CorpusManifestSHA256   string `json:"corpus_manifest_sha256"`
	SourceCiphertextSHA256 string `json:"source_ciphertext_sha256"`
	SelectionSHA256        string `json:"selection_sha256"`
	EvaluatorPolicy        string `json:"evaluator_policy"`
	ImpactRegistrySHA256   string `json:"impact_registry_sha256"`
	SynthesisPolicySHA256  string `json:"synthesis_policy_sha256"`
	GapPolicySHA256        string `json:"gap_policy_sha256"`
	SelectionPolicySHA256  string `json:"selection_policy_sha256"`
	InventorySHA256        string `json:"inventory_sha256"`
	CatalogSHA256          string `json:"catalog_sha256"`
	ModelRegistrySHA256    string `json:"model_registry_sha256"`
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
