// Package capabilityadvancementv10 validates and publishes the V10 public
// before/after capability round without any held-out access.
package capabilityadvancementv10

import (
	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityroundsv10"
)

const (
	ImplementationSealSchema = "kicadai.closed-loop-open-set-implementation-seal.v10"
	RoundSchema              = "kicadai.closed-loop-open-set-public-round.v10"
	Version                  = 10
)

type FileTransition struct {
	Path         string `json:"path"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
	Kind         string `json:"kind"`
}

type ImplementationSeal struct {
	Schema                   string           `json:"schema"`
	Version                  int              `json:"version"`
	SelectionSHA256          string           `json:"selection_sha256"`
	PlanSetSHA256            string           `json:"plan_set_sha256"`
	SelectedEffectPlanSHA256 string           `json:"selected_effect_plan_sha256"`
	BaseCommit               string           `json:"base_commit"`
	ImplementationCommit     string           `json:"implementation_commit"`
	Transitions              []FileTransition `json:"transitions"`
	FocusedTests             []string         `json:"focused_tests"`
	FullLocalRegression      bool             `json:"full_local_regression"`
	InstalledKiCadChecks     bool             `json:"installed_kicad_checks"`
	PrismReviewComplete      bool             `json:"prism_review_complete"`
	FixtureSpecificContent   bool             `json:"fixture_specific_content"`
	Hash                     string           `json:"hash"`
}

type Round struct {
	Schema                   string                               `json:"schema"`
	Version                  int                                  `json:"version"`
	Generation               int                                  `json:"generation"`
	BaselineManifestSHA256   string                               `json:"baseline_manifest_sha256"`
	BaselineReportSHA256     string                               `json:"baseline_report_sha256"`
	NextReportSHA256         string                               `json:"next_report_sha256"`
	SelectionSHA256          string                               `json:"selection_sha256"`
	ImplementationSealSHA256 string                               `json:"implementation_seal_sha256"`
	NextOutcomeCounts        []capabilitybaselinev10.OutcomeCount `json:"next_outcome_counts"`
	Evaluation               capabilityroundsv10.Evaluation       `json:"evaluation"`
	Hash                     string                               `json:"hash"`
}
