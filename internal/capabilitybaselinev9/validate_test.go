package capabilitybaselinev9

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/capabilityroundsv9"
)

func TestBuildValidatesAllFourOutcomesReplayGatesAndPromotions(t *testing.T) {
	report, err := Build(strings.Repeat("a", 64), baselineRecords())
	if err != nil {
		t.Fatal(err)
	}
	if report.CaseCount != 24 || report.EnvironmentSHA256 != strings.Repeat("b", 64) || report.EvaluatorManifestSHA256 != strings.Repeat("c", 64) ||
		len(report.Cases) != 24 || len(report.OutcomeCounts) != 4 || !digestPattern.MatchString(report.Hash) {
		t.Fatalf("unexpected V9 baseline report: %+v", report)
	}
	for index, count := range report.OutcomeCounts {
		if count.Outcome != outcomeOrder[index] || count.Count != 6 {
			t.Fatalf("outcome count %d = %+v", index, count)
		}
	}
	if err := Validate(report); err != nil {
		t.Fatal(err)
	}
	first, err := MarshalJSONStable(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalJSONStable(report)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("stable report marshal differs: %v", err)
	}
}

func TestBuildFailsClosedOnIncompleteOrConflictingEvidence(t *testing.T) {
	tests := map[string]func([]CaseEvidence){
		"case order":             func(records []CaseEvidence) { records[0].Case.ID = "v9_case_024" },
		"environment drift":      func(records []CaseEvidence) { records[1].EnvironmentSHA256 = strings.Repeat("f", 64) },
		"replay drift":           func(records []CaseEvidence) { records[0].ReplaySHA256[1] = strings.Repeat("f", 64) },
		"nonpass replay gate":    func(records []CaseEvidence) { records[1].Gates.DeterministicReplay = false },
		"nonpass fail closed":    func(records []CaseEvidence) { records[1].Gates.FailClosed = false },
		"pass gate failure":      func(records []CaseEvidence) { records[0].Gates.StrictDRC = false },
		"pass promotion missing": func(records []CaseEvidence) { records[0].Promotions = records[0].Promotions[:1] },
		"promotion roots reused": func(records []CaseEvidence) {
			records[0].Promotions[1].CleanRootSHA256 = records[0].Promotions[0].CleanRootSHA256
		},
		"nonpass claims all gates": func(records []CaseEvidence) { records[1].Gates = allPassedGates() },
		"nonpass promotion":        func(records []CaseEvidence) { records[1].Promotions = slices.Clone(records[0].Promotions) },
		"unknown outcome":          func(records []CaseEvidence) { records[1].Case.Outcome = "unknown" },
		"nonpass frontier missing": func(records []CaseEvidence) { records[1].Case.Frontier = nil },
		"nonroot path": func(records []CaseEvidence) {
			records[1].Case.Frontier[0].Path = append(records[1].Case.Frontier[0].Path, records[1].Case.Frontier[0].Path[0])
		},
		"unsorted evidence": func(records []CaseEvidence) {
			records[1].Case.Frontier[0].Path[0].RequiredEvidence = []string{"z", "a"}
		},
		"satisfied gap retained": func(records []CaseEvidence) {
			records[1].Case.SatisfiedObligations = []string{records[1].Case.Frontier[0].ObligationAnchor}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			records := baselineRecords()
			mutate(records)
			if _, err := Build(strings.Repeat("a", 64), records); !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateRejectsTamperedAggregateAndCaseHashes(t *testing.T) {
	report, err := Build(strings.Repeat("a", 64), baselineRecords())
	if err != nil {
		t.Fatal(err)
	}
	tampered := cloneReport(report)
	tampered.OutcomeCounts[0].Count++
	if err := Validate(tampered); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("aggregate tamper error = %v", err)
	}
	tampered = cloneReport(report)
	tampered.Cases[0].Hash = strings.Repeat("f", 64)
	tampered.Hash, _ = reportHash(tampered)
	if err := Validate(tampered); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("case hash tamper error = %v", err)
	}
}

func baselineRecords() []CaseEvidence {
	domains := []string{"analog_signal_path", "power_energy_conversion", "digital_control", "mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity"}
	roles := []string{"source_bias", "amplification_conditioning", "conversion_regulation", "sensing_measurement", "interface_control", "protection_supervision"}
	result := make([]CaseEvidence, 0, 24)
	for index := 1; index <= 24; index++ {
		id := "v9_case_" + pad3(index)
		outcome := outcomeOrder[(index-1)%len(outcomeOrder)]
		current := capabilityroundsv9.Case{ID: id, Role: "discovery", ReportingDomain: domains[(index-1)%len(domains)],
			CircuitRole: roles[(index-1)%len(roles)], SafetyImpact: "review_required", Outcome: outcome}
		gates := allPassedGates()
		promotions := []PromotionEvidence{}
		if outcome == "pass" {
			promotions = []PromotionEvidence{
				{CleanRootSHA256: digest("root-a-" + id), RunSHA256: digest("promotion-" + id), ProjectSHA256: digest("project-" + id), InstalledKiCad: true, ReplayIdentical: true},
				{CleanRootSHA256: digest("root-b-" + id), RunSHA256: digest("promotion-" + id), ProjectSHA256: digest("project-" + id), InstalledKiCad: true, ReplayIdentical: true},
			}
		} else {
			gates.CompleteRouting = false
			stage := []string{"topology", "component", "model", "simulation", "physical_design", "verification"}[(index-1)%6]
			leaf := capabilityroundsv9.Leaf{Stage: stage, Category: stage, Scope: "scope_" + pad3(index), Capability: "capability_" + pad3(index),
				Code: "CODE_" + pad3(index), RequiredEvidence: []string{"diagnostic_evidence"}}
			current.Frontier = []capabilityroundsv9.Gap{{ObligationAnchor: digest("anchor-" + id), Path: []capabilityroundsv9.Leaf{leaf}, Diagnostics: []string{"diagnostic"}}}
		}
		replay := digest("replay-" + id)
		result = append(result, CaseEvidence{Schema: CaseEvidenceSchema, Version: Version, Case: current,
			RequirementSHA256: digest("requirement-" + id), EnvironmentSHA256: strings.Repeat("b", 64), EvaluatorManifestSHA256: strings.Repeat("c", 64),
			ReplaySHA256: []string{replay, replay}, Gates: gates, Promotions: promotions})
	}
	return result
}

func allPassedGates() GateEvidence {
	return GateEvidence{PrimitiveOnly: true, TopologySearch: true, Simulation: true, AllCorners: true, ModelProvenance: true,
		ClosedLoopEvidence: true, CompleteRouting: true, Connectivity: true, WriterCorrectness: true, RoundTripZeroDiff: true,
		ERC: true, StrictDRC: true, DeterministicReplay: true, FailClosed: true}
}

func digest(value string) string { return hashBytes([]byte(value)) }

func pad3(value int) string {
	if value < 10 {
		return "00" + string(rune('0'+value))
	}
	return "0" + string(rune('0'+value/10)) + string(rune('0'+value%10))
}

func cloneReport(value Report) Report {
	data, _ := json.Marshal(value)
	var result Report
	_ = json.Unmarshal(data, &result)
	return result
}
