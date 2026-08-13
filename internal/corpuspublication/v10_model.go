package corpuspublication

import (
	"io"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev10"
)

const (
	ManifestSchemaV10             = "kicadai.closed-loop-open-set-corpus.v10"
	ManifestVersionV10            = 10
	HeldOutSchemaV10              = "kicadai.closed-loop-open-set-held-out-records.v10"
	HeldOutVersionV10             = 10
	HeldOutCipherFileV10          = "held_out_requirements.records.sealed"
	DiscoveryObligationsFileV10   = "discovery_obligations.json"
	HeldOutCommitmentFileV10      = "held_out_obligation_commitment.json"
	ValidationFileV10             = "validation_report.json"
	ManifestFileV10               = "manifest.json"
	AuditFileV10                  = "CORPUS_AUDIT.md"
	AuthorshipAttestationsFileV10 = "authorship_attestations.json"
	ChecksumFileV10               = "CHECKSUMS.sha256"
	SealAlgorithmV10              = "AES-256-GCM/random-unique-nonce-per-record/length-delimited-aad"
	expectedAuthorsV10            = 6
	expectedCasesV10              = 48
	expectedDiscoveryV10          = 24
	expectedHeldOutV10            = 24
)

type EntryV10 struct {
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

type HeldOutSealV10 struct {
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

type ManifestV10 struct {
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
	HeldOutSource                HeldOutSealV10            `json:"held_out_source"`
	Entries                      []EntryV10                `json:"entries"`
}

type AuthorshipAttestationsV10 struct {
	Schema  string                `json:"schema"`
	Version int                   `json:"version"`
	Records []PublicAuthorshipV10 `json:"records"`
}

// PublicAuthorshipV10 preserves the outcome-neutral provenance and isolation
// attestations needed to audit publication without exposing author context,
// quarantine paths, requirement paths, or per-requirement source hashes.
type PublicAuthorshipV10 struct {
	AuthorSlot              string                                 `json:"author_slot"`
	AuthorshipSHA256        string                                 `json:"authorship_sha256"`
	PerAuthorPacketManifest string                                 `json:"per_author_packet_manifest"`
	PerAuthorPacketSHA256   string                                 `json:"per_author_packet_sha256"`
	ContractBindingSHA256   string                                 `json:"contract_binding_sha256"`
	AssignmentSHA256        string                                 `json:"assignment_sha256"`
	AuthoringStartedUTC     string                                 `json:"authoring_started_utc"`
	AuthoringEndedUTC       string                                 `json:"authoring_ended_utc"`
	UncertaintyCount        int                                    `json:"uncertainty_count"`
	Attestations            corpusfreezev10.AuthorshipAttestations `json:"attestations"`
}

type RequestV10 struct {
	RepositoryRoot         string
	DestinationRoot        string
	KeyPath                string
	ContractManifestSHA256 string
	ValidatorManifest      []byte
	PublisherManifest      []byte
	Commits                Commits
	Report                 corpusfreezev10.Report
	Bundles                map[string]corpusfreeze.Bundle
	Random                 io.Reader
}

type ResultV10 struct {
	Manifest             ManifestV10
	ManifestSHA256       string
	DiscoveryCases       int
	HeldOutCases         int
	DiscoveryObligations int
	HeldOutObligations   int
}

type PublicValidationReportV10 struct {
	Schema                      string                          `json:"schema"`
	Version                     int                             `json:"version"`
	PolicySHA256                string                          `json:"policy_sha256"`
	PacketSetSHA256             string                          `json:"packet_set_sha256"`
	ContractBindingSHA256       string                          `json:"contract_binding_sha256"`
	HistoricalCommitmentsSHA256 string                          `json:"historical_commitments_sha256"`
	AuthorPacketSHA256          map[string]string               `json:"author_packet_sha256"`
	AssignmentSHA256            map[string]string               `json:"assignment_sha256"`
	AuthorshipSHA256            map[string]string               `json:"authorship_sha256"`
	Counts                      map[string]map[string]int       `json:"counts"`
	DiscoveryEntries            []corpusfreezev10.EntryEvidence `json:"discovery_entries"`
	HeldOutEntryCount           int                             `json:"held_out_entry_count"`
	HeldOutEntryAggregateSHA256 string                          `json:"held_out_entry_aggregate_sha256"`
}

type heldOutCaseV10 struct {
	Entry  EntryV10
	Source []byte
}

type ObligationV10 struct {
	Anchor          string `json:"anchor"`
	Role            string `json:"role"`
	CaseID          string `json:"case_id"`
	OperatingCaseID string `json:"operating_case_id"`
	AssertionID     string `json:"assertion_id"`
	ObservationKind string `json:"observation_kind"`
	ObservationID   string `json:"observation_id"`
	OutputID        string `json:"output_id"`
}

type DiscoveryObligationsV10 struct {
	Schema               string          `json:"schema"`
	Version              int             `json:"version"`
	CorpusManifestSHA256 string          `json:"corpus_manifest_sha256"`
	Obligations          []ObligationV10 `json:"obligations"`
}

type HeldOutObligationCommitmentV10 struct {
	Schema               string `json:"schema"`
	Version              int    `json:"version"`
	CorpusManifestSHA256 string `json:"corpus_manifest_sha256"`
	ObligationCount      int    `json:"obligation_count"`
	AggregateSHA256      string `json:"aggregate_sha256"`
}
