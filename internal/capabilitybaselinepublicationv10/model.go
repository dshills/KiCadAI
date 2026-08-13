// Package capabilitybaselinepublicationv10 atomically publishes and verifies
// the public V10 generation-zero discovery baseline. It has no held-out or key
// access surface.
package capabilitybaselinepublicationv10

import "kicadai/internal/capabilitybaselinev10"

const (
	ManifestSchema = "kicadai.closed-loop-open-set-baseline-publication.v10"
	Version        = 10
	ManifestFile   = "manifest.json"
	ReportFile     = "report.json"
	AuditFile      = "BASELINE_AUDIT.md"
	ChecksumFile   = "CHECKSUMS.sha256"
	CaseDirectory  = "discovery"
	ExpectedCases  = 24
)

var frozenOutcomeOrder = []string{"pass", "unsupported", "unsafe", "exhausted"}

type Binding struct {
	StartingCommit                 string `json:"starting_commit" binding:"commit"`
	ContractFreezeCommit           string `json:"contract_freeze_commit" binding:"commit"`
	CorpusFreezeCommit             string `json:"corpus_freeze_commit" binding:"commit"`
	EvaluatorFreezeCommit          string `json:"evaluator_freeze_commit" binding:"commit"`
	PublisherParentCommit          string `json:"publisher_parent_commit" binding:"commit"`
	CorpusManifestSHA256           string `json:"corpus_manifest_sha256" binding:"digest"`
	ContractManifestSHA256         string `json:"contract_manifest_sha256" binding:"digest"`
	AuthorPacketManifestSHA256     string `json:"author_packet_manifest_sha256" binding:"digest"`
	ValidatorManifestSHA256        string `json:"validator_manifest_sha256" binding:"digest"`
	CorpusPublisherManifestSHA256  string `json:"corpus_publisher_manifest_sha256" binding:"digest"`
	BaselineEvidenceManifestSHA256 string `json:"baseline_evidence_manifest_sha256" binding:"digest"`
	BaselinePublisherSHA256        string `json:"baseline_publisher_sha256" binding:"digest"`
	ValidationReportSHA256         string `json:"validation_report_sha256" binding:"digest"`
	HistoricalCommitmentsSHA256    string `json:"historical_commitments_sha256" binding:"digest"`
	DiscoveryObligationsSHA256     string `json:"discovery_obligations_sha256" binding:"digest"`
	EnvironmentSHA256              string `json:"environment_sha256" binding:"digest"`
	EvaluatorManifestSHA256        string `json:"evaluator_manifest_sha256" binding:"digest"`
}

type CaseReference struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Schema        string                               `json:"schema"`
	Version       int                                  `json:"version"`
	Binding       Binding                              `json:"binding"`
	ReportSHA256  string                               `json:"report_sha256"`
	CaseCount     int                                  `json:"case_count"`
	OutcomeCounts []capabilitybaselinev10.OutcomeCount `json:"outcome_counts"`
	Cases         []CaseReference                      `json:"cases"`
	Hash          string                               `json:"hash"`
}

type Request struct {
	RepositoryRoot  string
	DestinationRoot string
	Binding         Binding
	Report          capabilitybaselinev10.Report
}

type Result struct {
	Manifest       Manifest
	ManifestSHA256 string
}

type Verification struct {
	Manifest       Manifest
	ManifestSHA256 string
	Report         capabilitybaselinev10.Report
}
