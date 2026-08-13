package capabilityadvancementv10

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinepublicationv10"
	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityroundsv10"
	"kicadai/internal/capabilityroundsv9"
	"kicadai/internal/capabilityselectionv10"
)

func TestBuildRoundAdmitsMeasuredDiverseUplift(t *testing.T) {
	baseline := advancementBaseline(t)
	repositoryRoot := t.TempDir()
	productionData, verificationData := []byte("package fixture\n"), []byte("package fixture_test\n")
	writeFixtureFile(t, repositoryRoot, "internal/production.go", productionData)
	writeFixtureFile(t, repositoryRoot, "internal/production_test.go", verificationData)
	leaf := baseline.Report.Cases[0].Case.Frontier[0].Path[0]
	atomKey, err := capabilityroundsv9.AtomKey(leaf.Category, leaf.Scope, leaf.Capability)
	if err != nil {
		t.Fatal(err)
	}
	memberKey, err := capabilityroundsv9.MemberKey(leaf)
	if err != nil {
		t.Fatal(err)
	}
	plan := capabilityselectionv10.Plan{
		DirectAtomKeys: []string{atomKey}, DirectMemberKeys: []string{memberKey},
		ClosureAtoms: []capabilityroundsv10.Atom{}, ClosureMembers: []capabilityroundsv10.Member{}, PlannedMemberKeys: []string{memberKey},
		RequiredEvidence: slices.Clone(capabilityroundsv10.FrozenPolicy().MechanicalEvidence), Executable: true, MechanicallyProven: true,
		UnmappedConsumers: []string{},
		StaticEvidence: capabilityselectionv10.StaticEvidence{
			ProductionFiles:   []capabilityselectionv10.FileEvidence{{Path: "internal/production.go", SHA256: hashBytes(productionData)}},
			VerificationFiles: []capabilityselectionv10.FileEvidence{{Path: "internal/production_test.go", SHA256: hashBytes(verificationData)}},
			ReverseCallGraph:  []string{"caller -> capability"}, RegistryReferences: []string{}, ConfigurationLoaderReferences: []string{},
			CatalogModelReferences: []string{}, DataReferences: []string{}, FocusedNonCorpusRuntimeConsumers: []string{"production synthesis"},
		},
	}
	planSet := capabilityselectionv10.PlanSet{Schema: capabilityselectionv10.PlanSetSchema, Version: 10, Generation: 0,
		BaselineManifestSHA256: baseline.ManifestSHA256, BaselineReportSHA256: baseline.Report.Hash,
		EffectExposureEngineSHA256: capabilityselectionv10.EffectExposureEngineManifestSHA256, Plans: []capabilityselectionv10.Plan{plan}}
	planBytes, err := capabilityselectionv10.MarshalPlanSet(planSet)
	if err != nil {
		t.Fatal(err)
	}
	_, planSetHash, err := capabilityselectionv10.DecodePlanSet(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := capabilityselectionv10.Build(baseline, planSet, planSetHash, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	seal := ImplementationSeal{Schema: ImplementationSealSchema, Version: 10, SelectionSHA256: selection.Hash,
		PlanSetSHA256: planSetHash, SelectedEffectPlanSHA256: selection.Selection.Selected.EffectPlanSHA256,
		BaseCommit: strings.Repeat("a", 40), ImplementationCommit: strings.Repeat("b", 40),
		Transitions:  []FileTransition{{Path: "internal/production.go", BeforeSHA256: hashBytes(productionData), AfterSHA256: hashBytes([]byte("changed")), Kind: "production"}},
		FocusedTests: []string{"TestGenericCapability"}, FullLocalRegression: true, InstalledKiCadChecks: true, PrismReviewComplete: true}
	seal.Hash, err = sealHash(seal)
	if err != nil {
		t.Fatal(err)
	}

	nextRecords := slices.Clone(baseline.Report.Cases)
	for index := 0; index < 2; index++ {
		prior := nextRecords[index].Case
		prior.Outcome = "pass"
		prior.SatisfiedObligations = []string{prior.Frontier[0].ObligationAnchor}
		prior.Frontier = []capabilityroundsv10.Gap{}
		nextRecords[index].Case = prior
		nextRecords[index].Gates = allGates()
		nextRecords[index].Promotions = promotions(prior.ID)
		nextRecords[index].Hash = ""
	}
	next, err := capabilitybaselinev10.Build(baseline.Report.CorpusManifestSHA256, nextRecords)
	if err != nil {
		t.Fatal(err)
	}
	round, err := BuildRound(baseline, next, selection, seal)
	if err != nil {
		t.Fatal(err)
	}
	if round.Evaluation.Status != capabilityroundsv10.EvaluationPublicAdmitted || round.Evaluation.DiscoveryPassAfter-round.Evaluation.DiscoveryPassBefore != 2 ||
		len(round.Evaluation.AdvancedReportingDomains) != 2 || len(round.Evaluation.AdvancedCircuitRoles) != 2 {
		t.Fatalf("round = %#v", round)
	}
	next.EnvironmentSHA256 = hashBytes([]byte("drift"))
	if _, err := BuildRound(baseline, next, selection, seal); err == nil {
		t.Fatal("environment drift accepted")
	}
}

func TestBuildSealConfinesChangesToMechanicallyMappedFiles(t *testing.T) {
	baseline := advancementBaseline(t)
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "fixture@example.invalid")
	git(t, root, "config", "user.name", "Fixture")
	beforeProduction, verification := []byte("package fixture\n"), []byte("package fixture_test\n")
	writeFixtureFile(t, root, "internal/production.go", beforeProduction)
	writeFixtureFile(t, root, "internal/production_test.go", verification)
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "base")
	base := gitOutput(t, root, "rev-parse", "HEAD")

	leaf := baseline.Report.Cases[0].Case.Frontier[0].Path[0]
	atomKey, _ := capabilityroundsv9.AtomKey(leaf.Category, leaf.Scope, leaf.Capability)
	memberKey, _ := capabilityroundsv9.MemberKey(leaf)
	plan := capabilityselectionv10.Plan{DirectAtomKeys: []string{atomKey}, DirectMemberKeys: []string{memberKey}, ClosureAtoms: []capabilityroundsv10.Atom{}, ClosureMembers: []capabilityroundsv10.Member{}, PlannedMemberKeys: []string{memberKey}, RequiredEvidence: slices.Clone(capabilityroundsv10.FrozenPolicy().MechanicalEvidence), Executable: true, MechanicallyProven: true, UnmappedConsumers: []string{}, StaticEvidence: capabilityselectionv10.StaticEvidence{ProductionFiles: []capabilityselectionv10.FileEvidence{{Path: "internal/production.go", SHA256: hashBytes(beforeProduction)}}, VerificationFiles: []capabilityselectionv10.FileEvidence{{Path: "internal/production_test.go", SHA256: hashBytes(verification)}}, ReverseCallGraph: []string{"caller -> capability"}, RegistryReferences: []string{}, ConfigurationLoaderReferences: []string{}, CatalogModelReferences: []string{}, DataReferences: []string{}, FocusedNonCorpusRuntimeConsumers: []string{"production synthesis"}}}
	planSet := capabilityselectionv10.PlanSet{Schema: capabilityselectionv10.PlanSetSchema, Version: 10, Generation: 0, BaselineManifestSHA256: baseline.ManifestSHA256, BaselineReportSHA256: baseline.Report.Hash, EffectExposureEngineSHA256: capabilityselectionv10.EffectExposureEngineManifestSHA256, Plans: []capabilityselectionv10.Plan{plan}}
	planBytes, _ := capabilityselectionv10.MarshalPlanSet(planSet)
	_, planSetHash, err := capabilityselectionv10.DecodePlanSet(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := capabilityselectionv10.Build(baseline, planSet, planSetHash, root)
	if err != nil {
		t.Fatal(err)
	}

	afterProduction := []byte("package fixture\n\nfunc GenericCapability() {}\n")
	writeFixtureFile(t, root, "internal/production.go", afterProduction)
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "implement")
	implementation := gitOutput(t, root, "rev-parse", "HEAD")
	request := ImplementationSeal{Schema: ImplementationSealSchema, Version: 10, SelectionSHA256: selection.Hash, PlanSetSHA256: planSetHash, SelectedEffectPlanSHA256: selection.Selection.Selected.EffectPlanSHA256, BaseCommit: base, ImplementationCommit: implementation, Transitions: []FileTransition{{Path: "internal/production.go", BeforeSHA256: hashBytes(beforeProduction), AfterSHA256: hashBytes(afterProduction), Kind: "production"}}, FocusedTests: []string{"TestGenericCapability"}, FullLocalRegression: true, InstalledKiCadChecks: true, PrismReviewComplete: true}
	seal, err := BuildSeal(root, selection, planSet, planSetHash, request)
	if err != nil || !digestPattern.MatchString(seal.Hash) {
		t.Fatalf("seal = %#v, %v", seal, err)
	}
	request.Transitions[0].Path = "internal/unmapped.go"
	if _, err := BuildSeal(root, selection, planSet, planSetHash, request); err == nil {
		t.Fatal("unmapped implementation path accepted")
	}
}

func TestChangedPathsPreservesWhitespace(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "fixture@example.invalid")
	git(t, root, "config", "user.name", "Fixture")
	writeFixtureFile(t, root, "internal/file with space.go", []byte("package fixture\n"))
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "base")
	base := gitOutput(t, root, "rev-parse", "HEAD")
	writeFixtureFile(t, root, "internal/file with space.go", []byte("package fixture\n\nfunc Changed() {}\n"))
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "change")
	after := gitOutput(t, root, "rev-parse", "HEAD")
	paths, err := changedPaths(root, base, after)
	if err != nil || !slices.Equal(paths, []string{"internal/file with space.go"}) {
		t.Fatalf("paths = %q, %v", paths, err)
	}
}

func advancementBaseline(t *testing.T) capabilitybaselinepublicationv10.Verification {
	t.Helper()
	domains := []string{"analog_signal_path", "power_energy_conversion", "digital_control", "mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity"}
	roles := []string{"source_bias", "amplification_conditioning", "conversion_regulation", "sensing_measurement", "interface_control", "protection_supervision"}
	records := make([]capabilitybaselinev10.CaseEvidence, 24)
	for index := range records {
		id := fmt.Sprintf("v10_case_%03d", index+1)
		current := capabilityroundsv10.Case{ID: id, Role: "discovery", ReportingDomain: domains[index%6], CircuitRole: roles[index%6], SafetyImpact: "review_required", Outcome: "pass", Frontier: []capabilityroundsv10.Gap{}, SatisfiedObligations: []string{}}
		gates, casePromotions := allGates(), promotions(id)
		if index < 2 {
			current.Outcome = "unsupported"
			current.Frontier = []capabilityroundsv10.Gap{{ObligationAnchor: hashBytes([]byte("anchor-" + id)), Path: []capabilityroundsv10.Leaf{{Stage: "topology", Category: "topology", Scope: "generic_scope", Capability: "generic_capability", Code: "GENERIC_MISSING", RequiredEvidence: []string{"generic evidence"}}}, Diagnostics: []string{"generic capability unavailable"}}}
			gates = capabilitybaselinev10.GateEvidence{DeterministicReplay: true, FailClosed: true}
			casePromotions = []capabilitybaselinev10.PromotionEvidence{}
		}
		replay := hashBytes([]byte("replay-" + id))
		records[index] = capabilitybaselinev10.CaseEvidence{Schema: capabilitybaselinev10.CaseEvidenceSchema, Version: 10, Case: current,
			RequirementSHA256: hashBytes([]byte("requirement-" + id)), EnvironmentSHA256: hashBytes([]byte("environment")), EvaluatorManifestSHA256: hashBytes([]byte("evaluator")),
			ReplaySHA256: []string{replay, replay}, ReplayRootSHA256: []string{hashBytes([]byte("root-a-" + id)), hashBytes([]byte("root-b-" + id))}, Gates: gates, Promotions: casePromotions}
	}
	report, err := capabilitybaselinev10.Build(hashBytes([]byte("corpus")), records)
	if err != nil {
		t.Fatal(err)
	}
	return capabilitybaselinepublicationv10.Verification{ManifestSHA256: hashBytes([]byte("manifest")), Report: report}
}

func allGates() capabilitybaselinev10.GateEvidence {
	return capabilitybaselinev10.GateEvidence{PrimitiveOnly: true, TopologySearch: true, Simulation: true, AllCorners: true, ModelProvenance: true, ClosedLoopEvidence: true, CompleteRouting: true, Connectivity: true, WriterCorrectness: true, RoundTripZeroDiff: true, ERC: true, StrictDRC: true, DeterministicReplay: true, FailClosed: true}
}

func promotions(id string) []capabilitybaselinev10.PromotionEvidence {
	return []capabilitybaselinev10.PromotionEvidence{
		{CleanRootSHA256: hashBytes([]byte("promotion-a-" + id)), RunSHA256: hashBytes([]byte("run-" + id)), ProjectSHA256: hashBytes([]byte("project-" + id)), InstalledKiCad: true, ReplayIdentical: true},
		{CleanRootSHA256: hashBytes([]byte("promotion-b-" + id)), RunSHA256: hashBytes([]byte("run-" + id)), ProjectSHA256: hashBytes([]byte("project-" + id)), InstalledKiCad: true, ReplayIdentical: true},
	}
}

func writeFixtureFile(t *testing.T, root, relative string, data []byte) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return strings.TrimSpace(string(output))
}
