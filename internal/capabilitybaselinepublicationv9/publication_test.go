package capabilitybaselinepublicationv9

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev9"
	"kicadai/internal/capabilityroundsv9"
)

func TestPublishVerifyIsAtomicCanonicalAndPublicOnly(t *testing.T) {
	repository := t.TempDir()
	destination := filepath.Join(repository, "v9-baseline")
	report := testReport(t)
	request := Request{RepositoryRoot: repository, DestinationRoot: destination, Binding: testBinding(report), Report: report}
	result, err := Publish(request)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(destination)
	if err != nil {
		t.Fatal(err)
	}
	if result.ManifestSHA256 != verified.ManifestSHA256 || result.Manifest.Hash != verified.Manifest.Hash || verified.Report.Hash != report.Hash ||
		verified.Manifest.CaseCount != ExpectedCases || len(verified.Manifest.Cases) != ExpectedCases {
		t.Fatalf("publication did not reproduce: result=%+v verified=%+v", result, verified)
	}
	for index := 0; index < reflect.TypeOf(Request{}).NumField(); index++ {
		name := strings.ToLower(reflect.TypeOf(Request{}).Field(index).Name)
		if strings.Contains(name, "key") || strings.Contains(name, "heldout") || strings.Contains(name, "held_out") {
			t.Fatalf("public baseline request exposes forbidden field %q", name)
		}
	}
	if _, err := Publish(request); err == nil {
		t.Fatal("publication replaced an existing destination")
	}
}

func TestPublishRejectsBindingDriftAndDestinationEscape(t *testing.T) {
	repository := t.TempDir()
	report := testReport(t)
	binding := testBinding(report)
	binding.EnvironmentSHA256 = strings.Repeat("f", 64)
	destination := filepath.Join(repository, "invalid")
	if _, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: destination, Binding: binding, Report: report}); err == nil {
		t.Fatal("environment binding drift was accepted")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("invalid publication created a destination: %v", err)
	}
	outside := filepath.Join(filepath.Dir(repository), "outside-v9-baseline")
	if _, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: outside, Binding: testBinding(report), Report: report}); err == nil {
		t.Fatal("destination outside repository was accepted")
	}
}

func TestVerifyRejectsTamperAndUnexpectedFiles(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string) error
	}{
		{name: "case tamper", mutate: func(root string) error {
			return os.WriteFile(filepath.Join(root, CaseDirectory, "v9_case_001.json"), []byte("{}\n"), 0o644)
		}},
		{name: "extra file", mutate: func(root string) error {
			return os.WriteFile(filepath.Join(root, "unexpected"), []byte("x"), 0o644)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			destination := filepath.Join(repository, "baseline")
			report := testReport(t)
			if _, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: destination, Binding: testBinding(report), Report: report}); err != nil {
				t.Fatal(err)
			}
			if err := test.mutate(destination); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(destination); err == nil {
				t.Fatal("tampered publication verified")
			}
		})
	}
}

func TestCanonicalJSONRejectsMapBackedArtifacts(t *testing.T) {
	if _, err := canonicalJSON(struct {
		Values map[string]string `json:"values"`
	}{Values: map[string]string{"a": "b"}}); err == nil {
		t.Fatal("map-backed artifact was accepted as canonical")
	}
}

func TestReadRegularFileRejectsNonCanonicalInternalPaths(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"../escape", `discovery\case.json`, "discovery/../report.json", "/absolute"} {
		if _, err := readRegularFile(root, relative); err == nil {
			t.Fatalf("accepted noncanonical path %q", relative)
		}
	}
}

func testReport(t *testing.T) capabilitybaselinev9.Report {
	t.Helper()
	domains := []string{"analog_signal_path", "power_energy_conversion", "digital_control", "mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity"}
	roles := []string{"source_bias", "amplification_conditioning", "conversion_regulation", "sensing_measurement", "interface_control", "protection_supervision"}
	stages := []string{"topology", "component", "model", "simulation", "physical_design", "verification"}
	records := make([]capabilitybaselinev9.CaseEvidence, 0, ExpectedCases)
	for index := 1; index <= ExpectedCases; index++ {
		id := fmt.Sprintf("v9_case_%03d", index)
		stage := stages[(index-1)%len(stages)]
		current := capabilityroundsv9.Case{ID: id, Role: "discovery", ReportingDomain: domains[(index-1)%len(domains)], CircuitRole: roles[(index-1)%len(roles)],
			SafetyImpact: "review_required", Outcome: "unsupported", Frontier: []capabilityroundsv9.Gap{{ObligationAnchor: testDigest("anchor-" + id),
				Path: []capabilityroundsv9.Leaf{{Stage: stage, Category: stage, Scope: "scope_" + id, Capability: "capability_" + id, Code: "CODE_" + id, RequiredEvidence: []string{"evidence"}}}, Diagnostics: []string{"diagnostic"}}}}
		replay := testDigest("replay-" + id)
		records = append(records, capabilitybaselinev9.CaseEvidence{Schema: capabilitybaselinev9.CaseEvidenceSchema, Version: capabilitybaselinev9.Version,
			Case: current, RequirementSHA256: testDigest("requirement-" + id), EnvironmentSHA256: testDigest("environment"),
			EvaluatorManifestSHA256: testDigest("evaluator"), ReplaySHA256: []string{replay, replay},
			Gates: capabilitybaselinev9.GateEvidence{DeterministicReplay: true, FailClosed: true}})
	}
	report, err := capabilitybaselinev9.Build(testDigest("corpus"), records)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func testBinding(report capabilitybaselinev9.Report) Binding {
	commit := strings.Repeat("a", 40)
	digest := strings.Repeat("b", 64)
	return Binding{StartingCommit: commit, ContractFreezeCommit: commit, CorpusFreezeCommit: commit, EvaluatorFreezeCommit: commit, PublisherParentCommit: commit,
		CorpusManifestSHA256: report.CorpusManifestSHA256, ContractManifestSHA256: digest, AuthorPacketManifestSHA256: digest,
		ValidatorManifestSHA256: digest, CorpusPublisherManifestSHA256: digest, BaselineEvidenceManifestSHA256: digest,
		BaselinePublisherSHA256: digest, ValidationReportSHA256: digest, HistoricalCommitmentsSHA256: digest,
		DiscoveryObligationsSHA256: digest, EnvironmentSHA256: report.EnvironmentSHA256, EvaluatorManifestSHA256: report.EvaluatorManifestSHA256}
}

func testDigest(value string) string { return hashBytes([]byte(value)) }
