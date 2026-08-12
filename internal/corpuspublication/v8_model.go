package corpuspublication

import (
	"io"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev8"
)

const (
	ManifestSchemaV8           = "kicadai.closed-loop-open-set-corpus.v8"
	ManifestVersionV8          = 8
	HeldOutSchemaV8            = "kicadai.closed-loop-open-set-held-out-records.v8"
	HeldOutVersionV8           = 8
	HeldOutCipherFileV8        = "held_out_requirements.records.sealed"
	DiscoveryObligationsFileV8 = "discovery_obligations.json"
	HeldOutCommitmentFileV8    = "held_out_obligation_commitment.json"
	ValidationFileV8           = "validation_report.json"
	ManifestFileV8             = "manifest.json"
	AuditFileV8                = "CORPUS_AUDIT.md"
	ChecksumFileV8             = "CHECKSUMS.sha256"
	SealAlgorithmV8            = "AES-256-GCM/random-unique-nonce-per-record/length-delimited-aad"
	expectedAuthorsV8          = 6
	expectedCasesV8            = 36
	expectedDiscoveryV8        = 18
	expectedHeldOutV8          = 18
)

type EntryV8 struct {
	ID                       string `json:"id"`
	AuthorSlot               string `json:"author_slot"`
	Role                     string `json:"role"`
	Domain                   string `json:"domain"`
	CircuitRole              string `json:"circuit_role"`
	SafetyImpact             string `json:"safety_impact"`
	SourceID                 string `json:"source_id"`
	StablePath               string `json:"stable_path"`
	RequirementSHA256        string `json:"requirement_sha256"`
	NeutralSemanticSHA256    string `json:"neutral_semantic_sha256"`
	NormalizedSemanticSHA256 string `json:"normalized_semantic_sha256"`
	Sealed                   bool   `json:"sealed"`
}

type HeldOutSealV8 struct {
	Algorithm                string   `json:"algorithm"`
	File                     string   `json:"file"`
	CiphertextSHA256         string   `json:"ciphertext_sha256"`
	PlaintextAggregateSHA256 string   `json:"plaintext_aggregate_sha256"`
	AADAggregateSHA256       string   `json:"aad_aggregate_sha256"`
	MetadataAggregateSHA256  string   `json:"metadata_aggregate_sha256"`
	RecordCiphertextSHA256   []string `json:"record_ciphertext_sha256"`
	NonceBytes               int      `json:"nonce_bytes"`
	RecordCount              int      `json:"record_count"`
}

type ManifestV8 struct {
	Schema                      string                    `json:"schema"`
	Version                     int                       `json:"version"`
	Commits                     Commits                   `json:"commits"`
	ContractManifestSHA256      string                    `json:"contract_manifest_sha256"`
	ValidatorManifestSHA256     string                    `json:"validator_manifest_sha256"`
	PublisherManifestSHA256     string                    `json:"publisher_manifest_sha256"`
	ValidationReportSHA256      string                    `json:"validation_report_sha256"`
	PolicySHA256                string                    `json:"policy_sha256"`
	PacketSetSHA256             string                    `json:"packet_set_sha256"`
	ContractBindingSHA256       string                    `json:"contract_binding_sha256"`
	HistoricalCommitmentsSHA256 string                    `json:"historical_commitments_sha256"`
	AuthorPacketSHA256          map[string]string         `json:"author_packet_sha256"`
	AssignmentSHA256            map[string]string         `json:"assignment_sha256"`
	AuthorshipSHA256            map[string]string         `json:"authorship_sha256"`
	Counts                      map[string]map[string]int `json:"counts"`
	DiscoveryCaseCount          int                       `json:"discovery_case_count"`
	HeldOutCaseCount            int                       `json:"held_out_case_count"`
	HeldOutSource               HeldOutSealV8             `json:"held_out_source"`
	Entries                     []EntryV8                 `json:"entries"`
}

type RequestV8 struct {
	RepositoryRoot         string
	DestinationRoot        string
	KeyPath                string
	ContractManifestSHA256 string
	ValidatorManifest      []byte
	PublisherManifest      []byte
	Commits                Commits
	Report                 corpusfreezev8.Report
	Bundles                map[string]corpusfreeze.Bundle
	Random                 io.Reader
}

type ResultV8 struct {
	Manifest             ManifestV8
	ManifestSHA256       string
	DiscoveryCases       int
	HeldOutCases         int
	DiscoveryObligations int
	HeldOutObligations   int
}

type PublicValidationReportV8 struct {
	Schema                      string                         `json:"schema"`
	Version                     int                            `json:"version"`
	PolicySHA256                string                         `json:"policy_sha256"`
	PacketSetSHA256             string                         `json:"packet_set_sha256"`
	ContractBindingSHA256       string                         `json:"contract_binding_sha256"`
	HistoricalCommitmentsSHA256 string                         `json:"historical_commitments_sha256"`
	AuthorPacketSHA256          map[string]string              `json:"author_packet_sha256"`
	AssignmentSHA256            map[string]string              `json:"assignment_sha256"`
	AuthorshipSHA256            map[string]string              `json:"authorship_sha256"`
	Counts                      map[string]map[string]int      `json:"counts"`
	DiscoveryEntries            []corpusfreezev8.EntryEvidence `json:"discovery_entries"`
	HeldOutEntryCount           int                            `json:"held_out_entry_count"`
	HeldOutEntryAggregateSHA256 string                         `json:"held_out_entry_aggregate_sha256"`
}

type heldOutCaseV8 struct {
	Entry  EntryV8
	Source []byte
}

type ObligationV8 struct {
	Anchor          string `json:"anchor"`
	Role            string `json:"role"`
	CaseID          string `json:"case_id"`
	OperatingCaseID string `json:"operating_case_id"`
	AssertionID     string `json:"assertion_id"`
	ObservationKind string `json:"observation_kind"`
	ObservationID   string `json:"observation_id"`
	OutputID        string `json:"output_id"`
}

type DiscoveryObligationsV8 struct {
	Schema               string         `json:"schema"`
	Version              int            `json:"version"`
	CorpusManifestSHA256 string         `json:"corpus_manifest_sha256"`
	Obligations          []ObligationV8 `json:"obligations"`
}

type HeldOutObligationCommitmentV8 struct {
	Schema               string `json:"schema"`
	Version              int    `json:"version"`
	CorpusManifestSHA256 string `json:"corpus_manifest_sha256"`
	ObligationCount      int    `json:"obligation_count"`
	AggregateSHA256      string `json:"aggregate_sha256"`
}
