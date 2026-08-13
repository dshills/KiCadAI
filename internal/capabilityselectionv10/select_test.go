package capabilityselectionv10

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinepublicationv10"
	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityroundsv10"
	"kicadai/internal/capabilityroundsv9"
)

func TestBuildRanksGenericCapabilityByUnlockAndDiversity(t *testing.T) {
	repositoryRoot, plan := selectionFixturePlan(t)
	baseline := selectionFixtureBaseline(t)
	set := PlanSet{
		Schema: PlanSetSchema, Version: Version, Generation: 0,
		BaselineManifestSHA256:     baseline.ManifestSHA256,
		BaselineReportSHA256:       baseline.Report.Hash,
		EffectExposureEngineSHA256: EffectExposureEngineManifestSHA256,
		Plans:                      []Plan{plan},
	}
	data, err := MarshalPlanSet(set)
	if err != nil {
		t.Fatal(err)
	}
	decoded, planSetHash, err := DecodePlanSet(data)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(baseline, decoded, planSetHash, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(baseline, decoded, planSetHash, repositoryRoot)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("selection did not reproduce: %v", err)
	}
	selected := first.Selection.Selected
	if len(selected.FullyCoveredCaseIDs) != 2 || len(selected.ReportingDomains) != 2 || len(selected.CircuitRoles) != 2 ||
		len(selected.Atoms) != 1 || selected.Atoms[0].Capability != "generic_capability" {
		t.Fatalf("selected = %#v", selected)
	}
	if first.Selection.CandidateCount != 1 || len(first.Selection.EligibleCandidates) != 1 || len(first.Selection.CoRankOne) != 1 {
		t.Fatalf("ranking = %#v", first.Selection)
	}
	encoded, err := MarshalRanking(first)
	if err != nil || encoded[len(encoded)-1] != '\n' {
		t.Fatalf("marshal ranking: %v", err)
	}
	if err := Validate(first, baseline, decoded, planSetHash, repositoryRoot); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFailsClosedOnDriftAndMalformedPlanSets(t *testing.T) {
	repositoryRoot, plan := selectionFixturePlan(t)
	baseline := selectionFixtureBaseline(t)
	set := PlanSet{Schema: PlanSetSchema, Version: Version, Generation: 0,
		BaselineManifestSHA256: baseline.ManifestSHA256, BaselineReportSHA256: baseline.Report.Hash,
		EffectExposureEngineSHA256: EffectExposureEngineManifestSHA256, Plans: []Plan{plan}}
	data, err := MarshalPlanSet(set)
	if err != nil {
		t.Fatal(err)
	}
	decoded, planSetHash, err := DecodePlanSet(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := DecodePlanSet(append([]byte(" \n"), data...)); err == nil {
		t.Fatal("noncanonical plan set accepted")
	}
	if _, _, err := DecodePlanSet([]byte(`{"schema":"unknown","extra":true}` + "\n")); err == nil {
		t.Fatal("unknown plan-set field accepted")
	}
	if err := os.WriteFile(filepath.Join(repositoryRoot, "internal", "production.go"), []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(baseline, decoded, planSetHash, repositoryRoot); err == nil || !strings.Contains(err.Error(), "drifted") {
		t.Fatalf("source drift error = %v", err)
	}
}

func TestWriteAtomicNoReplace(t *testing.T) {
	output := filepath.Join(t.TempDir(), "ranking.json")
	if err := WriteAtomicNoReplace(output, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomicNoReplace(output, []byte("second\n")); err == nil {
		t.Fatal("ranking output was replaced")
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "first\n" {
		t.Fatalf("output = %q, %v", data, err)
	}
}

func selectionFixturePlan(t *testing.T) (string, Plan) {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string][]byte{
		"internal/production.go":      []byte("package fixture\n"),
		"internal/production_test.go": []byte("package fixture\n"),
	} {
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	leaf := capabilityroundsv10.Leaf{Stage: "topology", Category: "topology", Scope: "generic_scope", Capability: "generic_capability", Code: "GENERIC_MISSING"}
	atomKey, err := capabilityroundsv9.AtomKey(leaf.Category, leaf.Scope, leaf.Capability)
	if err != nil {
		t.Fatal(err)
	}
	memberKey, err := capabilityroundsv9.MemberKey(leaf)
	if err != nil {
		t.Fatal(err)
	}
	return root, Plan{
		DirectAtomKeys: []string{atomKey}, DirectMemberKeys: []string{memberKey},
		ClosureAtoms: []capabilityroundsv10.Atom{}, ClosureMembers: []capabilityroundsv10.Member{},
		PlannedMemberKeys: []string{memberKey}, RequiredEvidence: append([]string(nil), capabilityroundsv10.FrozenPolicy().MechanicalEvidence...),
		Executable: true, MechanicallyProven: true, UnboundedDynamicLookup: false, UnmappedConsumers: []string{},
		StaticEvidence: StaticEvidence{
			ProductionFiles:   []FileEvidence{{Path: "internal/production.go", SHA256: hashBytes([]byte("package fixture\n"))}},
			VerificationFiles: []FileEvidence{{Path: "internal/production_test.go", SHA256: hashBytes([]byte("package fixture\n"))}},
			ReverseCallGraph:  []string{"caller -> generic capability"}, RegistryReferences: []string{},
			ConfigurationLoaderReferences: []string{}, CatalogModelReferences: []string{}, DataReferences: []string{},
			FocusedNonCorpusRuntimeConsumers: []string{"production synthesis"},
		},
	}
}

func selectionFixtureBaseline(t *testing.T) capabilitybaselinepublicationv10.Verification {
	t.Helper()
	policy := capabilityroundsv10.FrozenPolicy()
	domains := []string{"analog_signal_path", "power_energy_conversion", "digital_control", "mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity"}
	roles := []string{"source_bias", "amplification_conditioning", "conversion_regulation", "sensing_measurement", "interface_control", "protection_supervision"}
	records := make([]capabilitybaselinev10.CaseEvidence, policy.ExpectedDiscoveryCases)
	for index := range records {
		id := "v10_case_" + leftPad(index+1)
		current := capabilityroundsv10.Case{ID: id, Role: "discovery", ReportingDomain: domains[index%len(domains)], CircuitRole: roles[index%len(roles)], SafetyImpact: "review_required", Outcome: "pass", Frontier: []capabilityroundsv10.Gap{}, SatisfiedObligations: []string{}}
		gates := capabilitybaselinev10.GateEvidence{PrimitiveOnly: true, TopologySearch: true, Simulation: true, AllCorners: true, ModelProvenance: true, ClosedLoopEvidence: true, CompleteRouting: true, Connectivity: true, WriterCorrectness: true, RoundTripZeroDiff: true, ERC: true, StrictDRC: true, DeterministicReplay: true, FailClosed: true}
		promotions := []capabilitybaselinev10.PromotionEvidence{
			{CleanRootSHA256: hashBytes([]byte("root-a-" + id)), RunSHA256: hashBytes([]byte("run-" + id)), ProjectSHA256: hashBytes([]byte("project-" + id)), InstalledKiCad: true, ReplayIdentical: true},
			{CleanRootSHA256: hashBytes([]byte("root-b-" + id)), RunSHA256: hashBytes([]byte("run-" + id)), ProjectSHA256: hashBytes([]byte("project-" + id)), InstalledKiCad: true, ReplayIdentical: true},
		}
		if index < 2 {
			leaf := capabilityroundsv10.Leaf{Stage: "topology", Category: "topology", Scope: "generic_scope", Capability: "generic_capability", Code: "GENERIC_MISSING", RequiredEvidence: []string{"generic implementation evidence"}}
			current.Outcome = "unsupported"
			current.Frontier = []capabilityroundsv10.Gap{{ObligationAnchor: hashBytes([]byte("anchor-" + id)), Path: []capabilityroundsv10.Leaf{leaf}, Diagnostics: []string{"generic capability unavailable"}}}
			gates = capabilitybaselinev10.GateEvidence{DeterministicReplay: true, FailClosed: true}
			promotions = []capabilitybaselinev10.PromotionEvidence{}
		}
		replay := hashBytes([]byte("replay-" + id))
		records[index] = capabilitybaselinev10.CaseEvidence{Schema: capabilitybaselinev10.CaseEvidenceSchema, Version: 10, Case: current,
			RequirementSHA256: hashBytes([]byte("requirement-" + id)), EnvironmentSHA256: hashBytes([]byte("environment")),
			EvaluatorManifestSHA256: hashBytes([]byte("evaluator")), ReplaySHA256: []string{replay, replay},
			ReplayRootSHA256: []string{hashBytes([]byte("replay-root-a-" + id)), hashBytes([]byte("replay-root-b-" + id))}, Gates: gates, Promotions: promotions}
	}
	report, err := capabilitybaselinev10.Build(hashBytes([]byte("corpus")), records)
	if err != nil {
		t.Fatal(err)
	}
	return capabilitybaselinepublicationv10.Verification{ManifestSHA256: hashBytes([]byte("baseline-manifest")), Report: report}
}

func leftPad(value int) string {
	return fmt.Sprintf("%03d", value)
}
