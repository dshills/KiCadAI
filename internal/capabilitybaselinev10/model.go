// Package capabilitybaselinev10 validates the frozen V10 generation-zero
// discovery evidence envelope without running synthesis or opening held-out
// material.
package capabilitybaselinev10

import "kicadai/internal/capabilityroundsv10"

const (
	CaseEvidenceSchema = "kicadai.closed-loop-open-set-case-evidence.v10"
	ReportSchema       = "kicadai.closed-loop-open-set-discovery-baseline.v10"
	Version            = 10
)

type GateEvidence struct {
	PrimitiveOnly       bool `json:"primitive_only"`
	TopologySearch      bool `json:"topology_search"`
	Simulation          bool `json:"simulation"`
	AllCorners          bool `json:"all_corners"`
	ModelProvenance     bool `json:"model_provenance"`
	ClosedLoopEvidence  bool `json:"closed_loop_evidence"`
	CompleteRouting     bool `json:"complete_routing"`
	Connectivity        bool `json:"connectivity"`
	WriterCorrectness   bool `json:"writer_correctness"`
	RoundTripZeroDiff   bool `json:"round_trip_zero_diff"`
	ERC                 bool `json:"erc"`
	StrictDRC           bool `json:"strict_drc"`
	DeterministicReplay bool `json:"deterministic_replay"`
	FailClosed          bool `json:"fail_closed"`
}

type PromotionEvidence struct {
	CleanRootSHA256 string `json:"clean_root_sha256"`
	RunSHA256       string `json:"run_sha256"`
	ProjectSHA256   string `json:"project_sha256"`
	InstalledKiCad  bool   `json:"installed_kicad"`
	ReplayIdentical bool   `json:"replay_identical"`
}

type CaseEvidence struct {
	Schema                  string                   `json:"schema"`
	Version                 int                      `json:"version"`
	Case                    capabilityroundsv10.Case `json:"case"`
	RequirementSHA256       string                   `json:"requirement_sha256"`
	EnvironmentSHA256       string                   `json:"environment_sha256"`
	EvaluatorManifestSHA256 string                   `json:"evaluator_manifest_sha256"`
	ReplaySHA256            []string                 `json:"replay_sha256"`
	ReplayRootSHA256        []string                 `json:"replay_root_sha256"`
	Gates                   GateEvidence             `json:"gates"`
	Promotions              []PromotionEvidence      `json:"promotions"`
	Hash                    string                   `json:"hash"`
}

type OutcomeCount struct {
	Outcome string `json:"outcome"`
	Count   int    `json:"count"`
}

type Report struct {
	Schema                  string         `json:"schema"`
	Version                 int            `json:"version"`
	CorpusManifestSHA256    string         `json:"corpus_manifest_sha256"`
	EnvironmentSHA256       string         `json:"environment_sha256"`
	EvaluatorManifestSHA256 string         `json:"evaluator_manifest_sha256"`
	CaseCount               int            `json:"case_count"`
	OutcomeCounts           []OutcomeCount `json:"outcome_counts"`
	Cases                   []CaseEvidence `json:"cases"`
	Hash                    string         `json:"hash"`
}
