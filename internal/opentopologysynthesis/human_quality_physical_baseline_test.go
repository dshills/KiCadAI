package opentopologysynthesis

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const humanQualityPhysicalBaselineHash = "1e3b0d6cba92c665507d92cbccf5cfdfa792bedf1df945b5bf28ec0911d6d65d"

type humanQualityPhysicalBaseline struct {
	Schema             string                                `json:"schema"`
	Version            int                                   `json:"version"`
	BaseCommit         string                                `json:"base_commit"`
	MeasuredAt         string                                `json:"measured_at"`
	ManifestSHA256     string                                `json:"manifest_sha256"`
	CommandContract    string                                `json:"command_contract"`
	InvocationsPerCase int                                   `json:"invocations_per_case"`
	Toolchain          humanQualityPhysicalBaselineToolchain `json:"toolchain"`
	Summary            humanQualityPhysicalBaselineSummary   `json:"summary"`
	GapClusters        []humanQualityPhysicalGapCluster      `json:"gap_clusters"`
	Cases              []humanQualityPhysicalBaselineCase    `json:"cases"`
}

type humanQualityPhysicalBaselineToolchain struct {
	KiCadAIVersion  string `json:"kicadai_version"`
	KiCadCLIVersion string `json:"kicad_cli_version"`
	GoVersion       string `json:"go_version"`
	Platform        string `json:"platform"`
}

type humanQualityPhysicalBaselineSummary struct {
	Cases                          int `json:"cases"`
	ElectricalSynthesisPasses      int `json:"electrical_synthesis_passes"`
	PhysicalPromotionsReached      int `json:"physical_promotions_reached"`
	LegacyPhysicalPromotionsPassed int `json:"legacy_physical_promotions_passed"`
	NewPhysicalContractPasses      int `json:"new_physical_contract_passes"`
	ReplayIdentical                int `json:"replay_identical"`
	PublicPolicyExhausted          int `json:"public_policy_exhausted"`
}

type humanQualityPhysicalGapCluster struct {
	ID       string   `json:"id"`
	Priority int      `json:"priority"`
	Cases    []string `json:"cases"`
	Evidence string   `json:"evidence"`
}

type humanQualityPhysicalBaselineCase struct {
	ID                           string      `json:"id"`
	Category                     string      `json:"category"`
	RequirementHash              string      `json:"requirement_hash"`
	Status                       Status      `json:"status"`
	StopReason                   StopReason  `json:"stop_reason"`
	FirstCode                    string      `json:"first_code"`
	PolicyHash                   string      `json:"policy_hash"`
	Consumption                  Consumption `json:"consumption"`
	EvidenceHash                 string      `json:"evidence_hash"`
	PhysicalHash                 string      `json:"physical_hash,omitempty"`
	PromotionStatus              string      `json:"promotion_status,omitempty"`
	PromotionEvidenceHash        string      `json:"promotion_evidence_hash,omitempty"`
	ProjectHash                  string      `json:"project_hash,omitempty"`
	ReplayIdentical              bool        `json:"replay_identical"`
	ProjectEmitted               bool        `json:"project_emitted"`
	PhysicalContractStatus       string      `json:"physical_contract_status"`
	HierarchyMode                string      `json:"hierarchy_mode,omitempty"`
	SchematicSheetCount          int         `json:"schematic_sheet_count,omitempty"`
	ExplicitInterSheetInterfaces int         `json:"explicit_inter_sheet_interfaces,omitempty"`
	BoardLayers                  int         `json:"board_layers,omitempty"`
	CopperLayers                 []string    `json:"copper_layers,omitempty"`
	FunctionalPlacementRegions   int         `json:"functional_placement_regions,omitempty"`
	Zones                        int         `json:"zones,omitempty"`
	NetsWithControlledReturnPath int         `json:"nets_with_controlled_return_path,omitempty"`
	LayerTransitionEvidence      bool        `json:"layer_transition_evidence,omitempty"`
	ThermalPlacementEvidence     bool        `json:"thermal_placement_evidence,omitempty"`
}

func TestHumanQualityPhysicalPublicCLIBaselineIsFrozen(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "human-quality-hierarchical-multilayer", "BASELINE_REPORT.json")
	data := mustRead(t, path)
	if got := frozenHash(data); got != humanQualityPhysicalBaselineHash {
		t.Fatalf("baseline sha256 = %s, want %s", got, humanQualityPhysicalBaselineHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, strings.TrimSuffix(path, ".json")+".sha256"))); sidecar != humanQualityPhysicalBaselineHash+"  BASELINE_REPORT.json" {
		t.Fatalf("baseline sidecar = %q", sidecar)
	}
	var report humanQualityPhysicalBaseline
	decodeFrozenStrict(t, data, &report)
	if report.Schema != "kicadai.human-quality-physical-baseline.v1" || report.Version != 1 ||
		report.BaseCommit != humanQualityPhysicalCorpusBaseCommit || report.MeasuredAt != "2026-08-07" ||
		report.ManifestSHA256 != humanQualityPhysicalCorpusManifestHash || report.InvocationsPerCase != 2 ||
		strings.TrimSpace(report.CommandContract) == "" {
		t.Fatalf("baseline identity = %#v", report)
	}
	wantToolchain := humanQualityPhysicalBaselineToolchain{
		KiCadAIVersion:  "0.1.0",
		KiCadCLIVersion: "10.0.3",
		GoVersion:       "go1.26.5",
		Platform:        "darwin/arm64",
	}
	if report.Toolchain != wantToolchain {
		t.Fatalf("toolchain = %#v, want %#v", report.Toolchain, wantToolchain)
	}
	wantSummary := humanQualityPhysicalBaselineSummary{
		Cases:                          4,
		ElectricalSynthesisPasses:      1,
		PhysicalPromotionsReached:      1,
		LegacyPhysicalPromotionsPassed: 1,
		NewPhysicalContractPasses:      0,
		ReplayIdentical:                4,
		PublicPolicyExhausted:          3,
	}
	if report.Summary != wantSummary || len(report.Cases) != 4 || len(report.GapClusters) != 4 {
		t.Fatalf("baseline coverage summary=%#v cases=%d clusters=%d", report.Summary, len(report.Cases), len(report.GapClusters))
	}
	previousID := ""
	for _, entry := range report.Cases {
		if entry.ID <= previousID || !entry.ReplayIdentical || entry.EvidenceHash == "" || entry.PolicyHash == "" {
			t.Fatalf("invalid baseline case after %q: %#v", previousID, entry)
		}
		previousID = entry.ID
		if entry.ID == "regulated_enabled_power_stage" {
			if entry.Status != StatusPassed || !entry.ProjectEmitted || entry.PromotionStatus != "passed" ||
				entry.PhysicalContractStatus != "failed" || entry.HierarchyMode != "flat" ||
				entry.SchematicSheetCount != 1 || entry.BoardLayers != 2 ||
				!slices.Equal(entry.CopperLayers, []string{"F.Cu", "B.Cu"}) ||
				entry.FunctionalPlacementRegions != 1 || entry.Zones != 0 ||
				entry.NetsWithControlledReturnPath != 0 || entry.LayerTransitionEvidence ||
				entry.ThermalPlacementEvidence || entry.ProjectHash == "" || entry.PromotionEvidenceHash == "" {
				t.Fatalf("power physical baseline = %#v", entry)
			}
		} else if entry.Status != StatusExhausted || entry.StopReason != StopSearchExhausted ||
			entry.ProjectEmitted || entry.PhysicalContractStatus != "not_reached" || !entry.Consumption.BudgetExhausted {
			t.Fatalf("exhausted physical baseline = %#v", entry)
		}
	}
	for index, cluster := range report.GapClusters {
		if cluster.Priority != index+1 || cluster.ID == "" || cluster.Evidence == "" ||
			len(cluster.Cases) == 0 || !slices.IsSorted(cluster.Cases) {
			t.Fatalf("gap cluster %d = %#v", index, cluster)
		}
	}
}
