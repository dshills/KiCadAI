package opentopologysynthesis

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	multiStageOODBaselineReportHash                      = "eb87f34f3f9d9b4e46e76a7ef74211919289dae82146e3a8072249a517f8e113"
	multiStageOODBaselineManifestHash                    = "0d800442b4006af19b4b375ea268d1d0156386846a9ef1af0dfd209d5d24a9be"
	multiStageOODBaselineEnabledCurrentRequirementSHA256 = "b90fd639f3ec80bf858360c4e9d1802fc82d613411943b8f04ce874664e09834"
)

type multiStageOODBaselineReport struct {
	Schema             string                         `json:"schema"`
	Version            int                            `json:"version"`
	BaseCommit         string                         `json:"base_commit"`
	ExecutionCommit    string                         `json:"execution_commit"`
	MeasuredAt         string                         `json:"measured_at"`
	ManifestSHA256     string                         `json:"manifest_sha256"`
	CommandContract    string                         `json:"command_contract"`
	Toolchain          multiStageOODBaselineToolchain `json:"toolchain"`
	InvocationsPerCase int                            `json:"invocations_per_case"`
	Summary            multiStageOODBaselineSummary   `json:"summary"`
	GapClusters        []multiStageOODGapCluster      `json:"gap_clusters"`
	Cases              []multiStageOODBaselineCase    `json:"cases"`
}

type multiStageOODBaselineToolchain struct {
	KiCadAIVersion  string `json:"kicadai_version"`
	KiCadCLIVersion string `json:"kicad_cli_version"`
	GoVersion       string `json:"go_version"`
	Platform        string `json:"platform"`
}

type multiStageOODBaselineSummary struct {
	Cases               int `json:"cases"`
	DesignCases         int `json:"design_cases"`
	AdversarialCases    int `json:"adversarial_cases"`
	ExpectedOutcomesMet int `json:"expected_outcomes_met"`
	ReplayIdentical     int `json:"replay_identical"`
	ProjectFilesEmitted int `json:"project_files_emitted"`
	NoCompleteGraph     int `json:"no_complete_graph"`
	SearchExhausted     int `json:"search_exhausted"`
	ValueExhausted      int `json:"value_exhausted"`
}

type multiStageOODGapCluster struct {
	ID       string   `json:"id"`
	Priority int      `json:"priority"`
	Cases    []string `json:"cases"`
	Evidence string   `json:"evidence"`
}

type multiStageOODBaselineCase struct {
	ID                      string      `json:"id"`
	Kind                    string      `json:"kind"`
	RequirementSHA256       string      `json:"requirement_sha256"`
	RequirementHash         string      `json:"requirement_hash"`
	ExpectedOutcome         string      `json:"expected_outcome"`
	Status                  Status      `json:"status"`
	StopReason              StopReason  `json:"stop_reason"`
	FirstCode               string      `json:"first_code"`
	TerminalStage           string      `json:"terminal_stage"`
	Consumption             Consumption `json:"consumption"`
	EvidenceHash            string      `json:"evidence_hash"`
	StdoutSHA256            string      `json:"stdout_sha256"`
	ExitStatus              int         `json:"exit_status"`
	ArtifactsEmitted        int         `json:"artifacts_emitted"`
	ProjectFilesEmitted     int         `json:"project_files_emitted"`
	ReplayIdentical         bool        `json:"replay_identical"`
	BaselineMatchesExpected bool        `json:"baseline_matches_expected"`
}

func TestMultiStageOODPublicCLIBaselineIsFrozen(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "multi-stage-out-of-distribution-synthesis", "BASELINE_REPORT.json")
	data := mustRead(t, path)
	if got := frozenHash(data); got != multiStageOODBaselineReportHash {
		t.Fatalf("baseline report sha256 = %s, want %s", got, multiStageOODBaselineReportHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, strings.TrimSuffix(path, ".json")+".sha256"))); sidecar != multiStageOODBaselineReportHash+"  BASELINE_REPORT.json" {
		t.Fatalf("baseline checksum sidecar = %q", sidecar)
	}

	var report multiStageOODBaselineReport
	decodeFrozenStrict(t, data, &report)
	if report.Schema != "kicadai.multi-stage-ood-baseline.v1" || report.Version != 1 {
		t.Fatalf("baseline schema/version = %q/%d", report.Schema, report.Version)
	}
	if report.BaseCommit != multiStageOODCorpusBaseCommit {
		t.Fatalf("baseline base commit = %q, want %q", report.BaseCommit, multiStageOODCorpusBaseCommit)
	}
	if report.ExecutionCommit != "a5effe06154b5c08b76de03e82be97a1b2eed8a2" {
		t.Fatalf("baseline execution commit = %q", report.ExecutionCommit)
	}
	if report.ManifestSHA256 != multiStageOODBaselineManifestHash {
		t.Fatalf("baseline manifest hash = %q, want historical %q", report.ManifestSHA256, multiStageOODBaselineManifestHash)
	}
	if report.MeasuredAt != "2026-08-06" {
		t.Fatalf("baseline measurement date = %q", report.MeasuredAt)
	}
	wantToolchain := multiStageOODBaselineToolchain{"0.1.0", "10.0.3", "go1.26.5", "darwin/arm64"}
	if report.Toolchain != wantToolchain {
		t.Fatalf("baseline toolchain = %#v, want %#v", report.Toolchain, wantToolchain)
	}
	if report.InvocationsPerCase != 2 || strings.TrimSpace(report.CommandContract) == "" {
		t.Fatalf("baseline invocation contract count=%d command=%q", report.InvocationsPerCase, report.CommandContract)
	}
	wantSummary := multiStageOODBaselineSummary{
		Cases: 13, DesignCases: 9, AdversarialCases: 4, ExpectedOutcomesMet: 0,
		ReplayIdentical: 13, ProjectFilesEmitted: 0, NoCompleteGraph: 3,
		SearchExhausted: 9, ValueExhausted: 1,
	}
	if report.Summary != wantSummary || len(report.Cases) != 13 || len(report.GapClusters) != 3 {
		t.Fatalf("baseline coverage summary=%#v cases=%d clusters=%d", report.Summary, len(report.Cases), len(report.GapClusters))
	}

	var manifest multiStageOODCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "manifest.json")), &manifest)
	type expectedCase struct{ kind, requirementSHA, outcome string }
	wantCases := make(map[string]expectedCase, len(report.Cases))
	for _, entry := range manifest.DesignCases {
		wantCases[entry.ID] = expectedCase{"design", entry.RequirementSHA256, "passed"}
	}
	// The historical CLI evidence predates correction of an infeasible 18 V
	// load demand on a 15 V maximum rail. Preserve that evidence rather than
	// relabeling it as a run of the amended, physically valid requirement.
	wantCases["enabled_current_regulation"] = expectedCase{
		"design", multiStageOODBaselineEnabledCurrentRequirementSHA256, "passed",
	}
	for _, entry := range manifest.AdversarialCases {
		wantCases[entry.ID] = expectedCase{"adversarial", entry.RequirementSHA256, entry.ExpectedFailureKind}
	}

	stopCounts := map[StopReason]int{}
	statusCounts := map[Status]int{}
	previousID := ""
	for _, entry := range report.Cases {
		want, ok := wantCases[entry.ID]
		if !ok || entry.ID <= previousID || entry.Kind != want.kind ||
			entry.RequirementSHA256 != want.requirementSHA || entry.ExpectedOutcome != want.outcome {
			t.Fatalf("baseline case identity/order %q after %q = %#v", entry.ID, previousID, entry)
		}
		previousID = entry.ID
		if !entry.ReplayIdentical || entry.BaselineMatchesExpected || entry.ExitStatus != 1 ||
			entry.ArtifactsEmitted != 0 || entry.ProjectFilesEmitted != 0 ||
			entry.RequirementHash == "" || entry.EvidenceHash == "" || entry.StdoutSHA256 == "" ||
			entry.TerminalStage != "open_topology_synthesis" || entry.Consumption.TopologyRepairs != 0 {
			t.Fatalf("baseline case evidence %s = %#v", entry.ID, entry)
		}
		stopCounts[entry.StopReason]++
		statusCounts[entry.Status]++
		delete(wantCases, entry.ID)
	}
	if len(wantCases) != 0 || stopCounts[StopNoCompleteGraph] != 3 || stopCounts[StopSearchExhausted] != 9 ||
		stopCounts[StopValueExhausted] != 1 || statusCounts[StatusExhausted] != 12 || statusCounts[StatusUnsupported] != 1 {
		t.Fatalf("baseline omissions/statuses remaining=%v stops=%v statuses=%v", wantCases, stopCounts, statusCounts)
	}

	clustered := map[string]bool{}
	for index, cluster := range report.GapClusters {
		if cluster.Priority != index+1 || strings.TrimSpace(cluster.ID) == "" || strings.TrimSpace(cluster.Evidence) == "" ||
			len(cluster.Cases) == 0 || !slices.IsSorted(cluster.Cases) {
			t.Fatalf("gap cluster %d = %#v", index, cluster)
		}
		for _, id := range cluster.Cases {
			if clustered[id] {
				t.Fatalf("case %s appears in more than one primary gap cluster", id)
			}
			clustered[id] = true
		}
	}
	if len(clustered) != 13 {
		t.Fatalf("primary gap clusters cover %d cases, want 13", len(clustered))
	}
}
