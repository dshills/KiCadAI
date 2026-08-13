package corpuspublication

import (
	"io"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev9"
)

const (
	ManifestSchemaV9             = "kicadai.closed-loop-open-set-corpus.v9"
	ManifestVersionV9            = 9
	HeldOutSchemaV9              = "kicadai.closed-loop-open-set-held-out-records.v9"
	HeldOutVersionV9             = 9
	HeldOutCipherFileV9          = "held_out_requirements.records.sealed"
	DiscoveryObligationsFileV9   = "discovery_obligations.json"
	HeldOutCommitmentFileV9      = "held_out_obligation_commitment.json"
	ValidationFileV9             = "validation_report.json"
	ManifestFileV9               = "manifest.json"
	AuditFileV9                  = "CORPUS_AUDIT.md"
	AuthorshipAttestationsFileV9 = "authorship_attestations.json"
	ChecksumFileV9               = "CHECKSUMS.sha256"
	SealAlgorithmV9              = "AES-256-GCM/random-unique-nonce-per-record/length-delimited-aad"
	expectedAuthorsV9            = 6
	expectedCasesV9              = 48
	expectedDiscoveryV9          = 24
	expectedHeldOutV9            = 24
)

type EntryV9 struct {
	ID                       string `json:"id"`
	AuthorSlot               string `json:"author_slot"`
	Role                     string `json:"role"`
	Domain                   string `json:"domain"`
	CircuitRole              string `json:"circuit_role"`
	SafetyImpact             string `json:"safety_impact"`
	PrimaryClass             string `json:"primary_class"`
	RequiredPrimaryAnalysis  string `json:"required_primary_analysis"`
	OutputMultiplicity       string `json:"output_multiplicity"`
	RequireOffNominal        bool   `json:"require_off_nominal"`
	SourceID                 string `json:"source_id"`
	StablePath               string `json:"stable_path"`
	RequirementSHA256        string `json:"requirement_sha256"`
	NeutralSemanticSHA256    string `json:"neutral_semantic_sha256"`
	NormalizedSemanticSHA256 string `json:"normalized_semantic_sha256"`
	Sealed                   bool   `json:"sealed"`
}

type HeldOutSealV9 struct {
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

type ManifestV9 struct {
	Schema                       string                    `json:"schema"`
	Version                      int                       `json:"version"`
	Commits                      Commits                   `json:"commits"`
	ContractManifestSHA256       string                    `json:"contract_manifest_sha256"`
	ValidatorManifestSHA256      string                    `json:"validator_manifest_sha256"`
	PublisherManifestSHA256      string                    `json:"publisher_manifest_sha256"`
	ValidationReportSHA256       string                    `json:"validation_report_sha256"`
	PolicySHA256                 string                    `json:"policy_sha256"`
	PacketSetSHA256              string                    `json:"packet_set_sha256"`
	ContractBindingSHA256        string                    `json:"contract_binding_sha256"`
	HistoricalCommitmentsSHA256  string                    `json:"historical_commitments_sha256"`
	AuthorPacketSHA256           map[string]string         `json:"author_packet_sha256"`
	AssignmentSHA256             map[string]string         `json:"assignment_sha256"`
	AuthorshipSHA256             map[string]string         `json:"authorship_sha256"`
	AuthorshipAttestationsSHA256 string                    `json:"authorship_attestations_sha256"`
	Counts                       map[string]map[string]int `json:"counts"`
	DiscoveryCaseCount           int                       `json:"discovery_case_count"`
	HeldOutCaseCount             int                       `json:"held_out_case_count"`
	HeldOutSource                HeldOutSealV9             `json:"held_out_source"`
	Entries                      []EntryV9                 `json:"entries"`
}

type AuthorshipAttestationsV9 struct {
	Schema  string               `json:"schema"`
	Version int                  `json:"version"`
	Records []PublicAuthorshipV9 `json:"records"`
}

// PublicAuthorshipV9 preserves the outcome-neutral provenance and isolation
// attestations needed to audit publication without exposing author context,
// quarantine paths, requirement paths, or per-requirement source hashes.
type PublicAuthorshipV9 struct {
	AuthorSlot              string                                `json:"author_slot"`
	AuthorshipSHA256        string                                `json:"authorship_sha256"`
	PerAuthorPacketManifest string                                `json:"per_author_packet_manifest"`
	PerAuthorPacketSHA256   string                                `json:"per_author_packet_sha256"`
	ContractBindingSHA256   string                                `json:"contract_binding_sha256"`
	AssignmentSHA256        string                                `json:"assignment_sha256"`
	AuthoringStartedUTC     string                                `json:"authoring_started_utc"`
	AuthoringEndedUTC       string                                `json:"authoring_ended_utc"`
	UncertaintyCount        int                                   `json:"uncertainty_count"`
	Attestations            corpusfreezev9.AuthorshipAttestations `json:"attestations"`
}

type RequestV9 struct {
	RepositoryRoot         string
	DestinationRoot        string
	KeyPath                string
	ContractManifestSHA256 string
	ValidatorManifest      []byte
	PublisherManifest      []byte
	Commits                Commits
	Report                 corpusfreezev9.Report
	Bundles                map[string]corpusfreeze.Bundle
	Random                 io.Reader
}

type ResultV9 struct {
	Manifest             ManifestV9
	ManifestSHA256       string
	DiscoveryCases       int
	HeldOutCases         int
	DiscoveryObligations int
	HeldOutObligations   int
}

type PublicValidationReportV9 struct {
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
	DiscoveryEntries            []corpusfreezev9.EntryEvidence `json:"discovery_entries"`
	HeldOutEntryCount           int                            `json:"held_out_entry_count"`
	HeldOutEntryAggregateSHA256 string                         `json:"held_out_entry_aggregate_sha256"`
}

type heldOutCaseV9 struct {
	Entry  EntryV9
	Source []byte
}

type ObligationV9 struct {
	Anchor          string `json:"anchor"`
	Role            string `json:"role"`
	CaseID          string `json:"case_id"`
	OperatingCaseID string `json:"operating_case_id"`
	AssertionID     string `json:"assertion_id"`
	ObservationKind string `json:"observation_kind"`
	ObservationID   string `json:"observation_id"`
	OutputID        string `json:"output_id"`
}

type DiscoveryObligationsV9 struct {
	Schema               string         `json:"schema"`
	Version              int            `json:"version"`
	CorpusManifestSHA256 string         `json:"corpus_manifest_sha256"`
	Obligations          []ObligationV9 `json:"obligations"`
}

type HeldOutObligationCommitmentV9 struct {
	Schema               string `json:"schema"`
	Version              int    `json:"version"`
	CorpusManifestSHA256 string `json:"corpus_manifest_sha256"`
	ObligationCount      int    `json:"obligation_count"`
	AggregateSHA256      string `json:"aggregate_sha256"`
}
