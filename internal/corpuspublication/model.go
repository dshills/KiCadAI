// Package corpuspublication atomically publishes a validated V5 behavior
// corpus without exposing held-out requirement sources.
package corpuspublication

import (
	"io"

	"kicadai/internal/corpusfreeze"
)

const (
	ManifestSchema    = "kicadai.closed-loop-open-set-corpus.v5"
	ManifestVersion   = 5
	HeldOutSchema     = "kicadai.closed-loop-open-set-held-out-source.v5"
	HeldOutVersion    = 5
	SealAlgorithm     = "AES-256-GCM/random-nonce-prefixed/length-delimited-aad"
	HeldOutCipherFile = "held_out_requirements.sealed"
	ValidationFile    = "validation_report.json"
	ManifestFile      = "manifest.json"
	AuditFile         = "CORPUS_AUDIT.md"
	ChecksumFile      = "CHECKSUMS.sha256"

	expectedAuthors   = 3
	expectedCases     = 36
	expectedDiscovery = 18
	expectedHeldOut   = 18
)

type Commits struct {
	StartingCommit        string `json:"starting_commit"`
	ContractFreezeCommit  string `json:"contract_freeze_commit"`
	AuthoringPacketCommit string `json:"authoring_packet_commit"`
	ValidatorCommit       string `json:"validator_commit"`
	FreezeParentCommit    string `json:"freeze_parent_commit"`
}

type Entry struct {
	ID                       string `json:"id"`
	AuthorSlot               string `json:"author_slot"`
	Role                     string `json:"role"`
	Domain                   string `json:"domain"`
	SafetyImpact             string `json:"safety_impact"`
	SourceID                 string `json:"source_id"`
	StablePath               string `json:"stable_path"`
	RequirementSHA256        string `json:"requirement_sha256"`
	NeutralSemanticSHA256    string `json:"neutral_semantic_sha256"`
	NormalizedSemanticSHA256 string `json:"normalized_semantic_sha256"`
	Sealed                   bool   `json:"sealed"`
}

type HeldOutSeal struct {
	Algorithm        string `json:"algorithm"`
	File             string `json:"file"`
	PayloadSHA256    string `json:"payload_sha256"`
	CiphertextSHA256 string `json:"ciphertext_sha256"`
	AADSHA256        string `json:"aad_sha256"`
	NonceBytes       int    `json:"nonce_bytes"`
	CaseCount        int    `json:"case_count"`
}

type Manifest struct {
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
	HeldOutSource               HeldOutSeal               `json:"held_out_source"`
	Entries                     []Entry                   `json:"entries"`
}

type Request struct {
	RepositoryRoot         string
	DestinationRoot        string
	KeyPath                string
	ContractManifestSHA256 string
	ValidatorManifest      []byte
	PublisherManifest      []byte
	Commits                Commits
	Report                 corpusfreeze.Report
	Bundles                map[string]corpusfreeze.Bundle
	Random                 io.Reader
}

type Result struct {
	Manifest       Manifest
	ManifestSHA256 string
	DiscoveryCases int
	HeldOutCases   int
}

type HeldOutCase struct {
	Entry  Entry
	Source []byte
}
