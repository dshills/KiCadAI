package verification

import (
	"context"
	"testing"
)

func TestSelectKiCadCorpusManifestsDisabledReturnsAll(t *testing.T) {
	manifests := []Manifest{validManifest(), corpusManifest("connector_breakout_4pin", KiCadCorpusTierSmoke)}
	selected, summary := SelectKiCadCorpusManifests(manifests, KiCadCorpusOptions{})
	if len(selected) != 2 {
		t.Fatalf("selected = %d, want 2", len(selected))
	}
	if summary.Enabled || summary.TotalCount != 2 || summary.SelectedCount != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestSelectKiCadCorpusManifestsFiltersIncludedCases(t *testing.T) {
	manifests := []Manifest{validManifest(), corpusManifest("connector_breakout_4pin", KiCadCorpusTierSmoke)}
	selected, summary := SelectKiCadCorpusManifests(manifests, KiCadCorpusOptions{Enabled: true})
	if len(selected) != 1 || selected[0].ID != "connector_breakout_4pin" {
		t.Fatalf("selected = %#v", selected)
	}
	if summary.SelectedCount != 1 || summary.TotalCount != 2 || summary.CountsByStatus[KiCadCorpusResultSkip] != 1 || summary.CountsByStatus[KiCadCorpusResultNotInCorpus] != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(summary.CaseIDs) != 1 || summary.CaseIDs[0] != "connector_breakout_4pin" {
		t.Fatalf("case IDs = %#v", summary.CaseIDs)
	}
}

func TestSelectKiCadCorpusManifestsFiltersTiers(t *testing.T) {
	manifests := []Manifest{
		corpusManifest("led_indicator_default", KiCadCorpusTierSmoke),
		corpusManifest("voltage_regulator_3v3", KiCadCorpusTierBlock),
	}
	selected, summary := SelectKiCadCorpusManifests(manifests, KiCadCorpusOptions{Enabled: true, Tiers: []KiCadCorpusTier{KiCadCorpusTierBlock}})
	if len(selected) != 1 || selected[0].ID != "voltage_regulator_3v3" {
		t.Fatalf("selected = %#v", selected)
	}
	if summary.SelectedCount != 1 || summary.CountsByTier[KiCadCorpusTierBlock] != 1 || summary.CountsByStatus[KiCadCorpusResultNotInCorpus] != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestBuildKiCadCorpusSummaryCountsResults(t *testing.T) {
	results := []RunResult{{
		CaseID:  "blocked_case",
		BlockID: "led_indicator",
		KiCadCorpus: &KiCadCorpusCaseResult{
			CaseID:  "blocked_case",
			BlockID: "led_indicator",
			Tier:    KiCadCorpusTierSmoke,
			Status:  KiCadCorpusResultBlocked,
		},
	}, {
		CaseID:  "passing_case",
		BlockID: "connector_breakout",
		KiCadCorpus: &KiCadCorpusCaseResult{
			CaseID:  "passing_case",
			BlockID: "connector_breakout",
			Tier:    KiCadCorpusTierBlock,
			Status:  KiCadCorpusResultPass,
		},
	}}
	summary := BuildKiCadCorpusSummary(results)
	if !summary.Enabled || summary.SelectedCount != 2 || summary.CountsByStatus[KiCadCorpusResultPass] != 1 || summary.CountsByStatus[KiCadCorpusResultBlocked] != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.CountsByTier[KiCadCorpusTierSmoke] != 1 || summary.CountsByTier[KiCadCorpusTierBlock] != 1 {
		t.Fatalf("tier counts = %#v", summary.CountsByTier)
	}
	if len(summary.CaseIDs) != 2 || summary.CaseIDs[0] != "blocked_case" || summary.CaseIDs[1] != "passing_case" {
		t.Fatalf("case IDs = %#v", summary.CaseIDs)
	}
}

func TestRunCaseUpdatesKiCadCorpusStatus(t *testing.T) {
	manifest := corpusManifest("led_indicator_default", KiCadCorpusTierSmoke)
	manifest.Expected.EvidenceLevel = EvidenceSchematicVerified
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.KiCadCorpus.RequiresDRC = false
	result := RunCase(context.Background(), manifest, RunOptions{})
	if result.Status != StatusPass {
		t.Fatalf("status = %s issues=%#v", result.Status, result.Issues)
	}
	if result.KiCadCorpus == nil || result.KiCadCorpus.Status != KiCadCorpusResultPass {
		t.Fatalf("corpus = %#v", result.KiCadCorpus)
	}
}

func corpusManifest(id string, tier KiCadCorpusTier) Manifest {
	manifest := validManifest()
	manifest.ID = id
	manifest.Expected.KiCadCorpus = ExpectedKiCadCorpus{
		Include:        true,
		Tier:           tier,
		Readiness:      KiCadCorpusReadinessCandidate,
		ExpectedStatus: KiCadCorpusStatusPass,
		RequiresDRC:    true,
	}
	return manifest
}
