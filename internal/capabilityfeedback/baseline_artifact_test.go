package capabilityfeedback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityexpansion"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	ots "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
)

const (
	closedLoopBaselineSchema     = "kicadai.closed-loop-open-set-baseline.v1"
	closedLoopSelectionSchema    = "kicadai.closed-loop-open-set-selection.v1"
	closedLoopBaselineVersion    = 1
	closedLoopCorpusFreezeCommit = "8ce268fb5acb2cb64d3fd26888f2abec38f150e9"
	closedLoopBaselineRoot       = "testdata/closed_loop_open_set_baseline"
)

type closedLoopBaselineReport struct {
	Schema              string                            `json:"schema"`
	Version             int                               `json:"version"`
	CorpusManifestHash  string                            `json:"corpus_manifest_hash"`
	FreezeCommit        string                            `json:"freeze_commit"`
	EvaluatorPolicy     string                            `json:"evaluator_policy"`
	ImpactRegistryHash  string                            `json:"impact_registry_hash"`
	SynthesisPolicyHash string                            `json:"synthesis_policy_hash"`
	Environment         closedLoopEnvironment             `json:"environment"`
	OutcomeCounts       []closedLoopOutcomeCount          `json:"outcome_counts"`
	Discovery           AggregateReport                   `json:"discovery"`
	HeldOut             AggregateReport                   `json:"held_out"`
	ExpansionPlan       capabilityexpansion.ExpansionPlan `json:"expansion_plan"`
	Hash                string                            `json:"hash"`
}

type closedLoopOutcomeCount struct {
	Role        CorpusRole                  `json:"role"`
	Domain      capabilityevaluation.Domain `json:"domain,omitempty"`
	Pass        int                         `json:"pass"`
	Unsupported int                         `json:"unsupported"`
	Unsafe      int                         `json:"unsafe"`
	Exhausted   int                         `json:"exhausted"`
}

type closedLoopSelection struct {
	Schema              string  `json:"schema"`
	Version             int     `json:"version"`
	CorpusManifestHash  string  `json:"corpus_manifest_hash"`
	FreezeCommit        string  `json:"freeze_commit"`
	EvaluatorPolicy     string  `json:"evaluator_policy"`
	ImpactRegistryHash  string  `json:"impact_registry_hash"`
	SynthesisPolicyHash string  `json:"synthesis_policy_hash"`
	BaselineReportHash  string  `json:"baseline_report_hash"`
	Cluster             Cluster `json:"cluster"`
	ExpansionPlanHash   string  `json:"expansion_plan_hash"`
	Hash                string  `json:"hash"`
}

func TestClosedLoopBaselineArtifactsAreFrozen(t *testing.T) {
	manifest := loadClosedLoopManifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, "manifest.json"))
	cases := loadClosedLoopBaselineCases(t, manifest)
	discoveryCases, heldOutCases := splitClosedLoopCases(cases)
	discovery, err := Evaluate(RoleDiscovery, discoveryCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	heldOut, err := Evaluate(RoleHeldOut, heldOutCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}

	specRoot := closedLoopSpecDirectory(t)
	reportBytes := mustCorpusRead(t, filepath.Join(specRoot, "BASELINE_REPORT.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "BASELINE_REPORT.sha256"), "BASELINE_REPORT.json", reportBytes)
	var report closedLoopBaselineReport
	decodeCorpusStrict(t, reportBytes, &report)
	expected := buildClosedLoopBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, heldOut, plan)
	if !bytes.Equal(reportBytes, corpusJSON(t, expected)) {
		t.Fatal("baseline report does not reproduce from frozen case evidence")
	}

	selectionBytes := mustCorpusRead(t, filepath.Join(specRoot, "SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "SELECTION.sha256"), "SELECTION.json", selectionBytes)
	var selection closedLoopSelection
	decodeCorpusStrict(t, selectionBytes, &selection)
	wantSelection := buildClosedLoopSelection(t, expected)
	if !bytes.Equal(selectionBytes, corpusJSON(t, wantSelection)) {
		t.Fatal("rank-one selection does not reproduce from the frozen discovery report")
	}
	if len(heldOut.Clusters) != 0 {
		t.Fatal("held-out cases leaked rankable cluster membership")
	}
}

func TestUpdateClosedLoopBaseline(t *testing.T) {
	if os.Getenv("UPDATE_CLOSED_LOOP_BASELINE") != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_BASELINE=1 to run and record the untouched local baseline")
	}
	manifest := loadClosedLoopManifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, "manifest.json"))
	inventory, environment := closedLoopSynthesisEnvironment(t)

	// Discovery is completed, ranked, and sealed before any held-out synthesis
	// executes. Do not merge these loops or expose held-out gaps to selection.
	discoveryCases := runClosedLoopBaselineRole(t, manifest, RoleDiscovery, inventory, environment)
	discovery, err := Evaluate(RoleDiscovery, discoveryCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Clusters) == 0 || discovery.Clusters[0].Rank != 1 {
		t.Fatal("discovery baseline did not produce an actionable rank-one cluster")
	}

	heldOutCases := runClosedLoopBaselineRole(t, manifest, RoleHeldOut, inventory, environment)
	heldOut, err := Evaluate(RoleHeldOut, heldOutCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	report := buildClosedLoopBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, heldOut, plan)
	writeClosedLoopBaselineArtifacts(t, append(discoveryCases, heldOutCases...), report, buildClosedLoopSelection(t, report))
}

func runClosedLoopBaselineRole(
	t *testing.T,
	manifest closedLoopManifest,
	role CorpusRole,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
) []CaseEvidence {
	t.Helper()
	results := []CaseEvidence{}
	for _, entry := range manifest.Entries {
		if entry.Role != role {
			continue
		}
		t.Logf("baseline %s %s starting", role, entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s requirement issues: %#v", entry.ID, issues)
		}
		first := runClosedLoopSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		t.Logf("baseline %s %s synthesis-1 status=%s stop=%s", role, entry.ID, first.Report.Status, first.Report.StopReason)
		second := runClosedLoopSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		t.Logf("baseline %s %s synthesis-2 status=%s stop=%s", role, entry.ID, second.Report.Status, second.Report.StopReason)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil {
			t.Fatalf("%s encode synthesis replay: first=%v second=%v", entry.ID, firstErr, secondErr)
		}
		// The baseline contract requires exact normalized bytes, not merely
		// semantic JSON equality. A float, ordering, or timestamp variation is
		// replay nondeterminism and must fail closed.
		if !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf(
				"%s synthesis replay differs at byte %d: first_len=%d second_len=%d first_sha256=%s second_sha256=%s",
				entry.ID, firstDifferentByte(firstBytes, secondBytes), len(firstBytes), len(secondBytes), corpusHash(firstBytes), corpusHash(secondBytes),
			)
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopRun(t, entry.ID, first, environment)
			promotion = &current
		}
		evidence, err := Observe(CaseMeta{ID: entry.ID, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact}, requirement, first, promotion)
		if err != nil {
			t.Fatalf("%s observe: %v", entry.ID, err)
		}
		writeClosedLoopCaseEvidence(t, evidence)
		t.Logf("baseline %s %s outcome=%s stop=%s gaps=%d synthesis=%s", role, entry.ID, evidence.Outcome, evidence.StopReason, len(evidence.Gaps), evidence.SynthesisHash)
		results = append(results, evidence)
	}
	return results
}

func runClosedLoopSynthesis(
	t *testing.T,
	requirement ots.Requirement,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
	policy ots.Policy,
) ots.SynthesisRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	run := ots.Synthesize(ctx, requirement, inventory, environment, policy)
	if run.Report.Status == ots.StatusInvalid || run.Report.Status == ots.StatusCanceled {
		t.Fatalf("synthesis aborted with status=%s stop=%s diagnostics=%#v", run.Report.Status, run.Report.StopReason, run.Report.Diagnostics)
	}
	return run
}

func closedLoopSynthesisEnvironment(t *testing.T) (ots.PrimitiveInventory, ots.SimulationEnvironment) {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model provenance diagnostics: %#v", diagnostics)
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	inventory, issues := ots.BuildPrimitiveInventory(catalog, catalogHash, registry)
	if len(issues) != 0 {
		t.Fatalf("primitive inventory issues: %#v", issues)
	}
	return inventory, ots.SimulationEnvironment{Catalog: catalog, CatalogHash: catalogHash, ModelRegistry: registry}
}

func promoteClosedLoopRun(t *testing.T, id string, run ots.SynthesisRun, environment ots.SimulationEnvironment) ots.PhysicalPromotionResult {
	t.Helper()
	cli := closedLoopKiCadCLI(t)
	symbols := closedLoopLibraryRoot(t, libraryresolver.EnvSymbolsRoot)
	footprints := closedLoopLibraryRoot(t, libraryresolver.EnvFootprintsRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	index, issues := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints}, libraryresolver.LoadOptions{})
	closure, closureIssues := resolveClosedLoopLibraryClosure(index, run)
	closureIssues = append(closureIssues, libraryresolver.DesignClosureIssuesFrom(issues, closure)...)
	for _, issue := range closureIssues {
		if issue.Severity == reports.SeverityError || issue.Severity == reports.SeverityBlocked {
			t.Fatalf("%s library index issue: %#v", id, issue)
		}
	}
	return ots.PromoteSynthesisRun(ctx, run, environment, ots.PhysicalPromotionOptions{
		OutputRoot: filepath.Join(t.TempDir(), id), KiCadCLI: cli, LibraryIndex: &index, Timeout: 3 * time.Minute,
	})
}

func resolveClosedLoopLibraryClosure(index libraryresolver.LibraryIndex, run ots.SynthesisRun) (libraryresolver.DesignClosure, []reports.Issue) {
	request := libraryresolver.ClosureRequest{}
	if run.Physical == nil {
		return libraryresolver.ResolveDesignClosure(index, request)
	}
	for _, component := range run.Physical.Resolved.Components {
		symbol := libraryresolver.SymbolReference{LibraryID: component.SymbolID}
		for _, unit := range component.Units {
			symbol.Units = append(symbol.Units, unit.Unit)
		}
		footprint := libraryresolver.FootprintReference{LibraryID: component.FootprintID}
		for _, function := range component.Functions {
			symbol.Pins = append(symbol.Pins, function.SymbolPin)
			footprint.Pads = append(footprint.Pads, function.Pad)
		}
		request.Symbols = append(request.Symbols, symbol)
		request.Footprints = append(request.Footprints, footprint)
		request.Variants = append(request.Variants, libraryresolver.VariantReference{
			ComponentID: component.ComponentID,
			VariantID:   component.VariantID,
			FootprintID: component.FootprintID,
		})
	}
	return libraryresolver.ResolveDesignClosure(index, request)
}

func closedLoopKiCadCLI(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("KICADAI_KICAD_CLI"))
	if path == "" {
		var err error
		path, err = exec.LookPath("kicad-cli")
		if err != nil {
			t.Fatal("installed kicad-cli is unavailable; set KICADAI_KICAD_CLI or add it to PATH")
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		t.Fatalf("installed kicad-cli is unavailable at %s", path)
	}
	return path
}

func closedLoopLibraryRoot(t *testing.T, envName string) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(envName))
	if path == "" {
		t.Fatalf("installed-KiCad promotion requires %s", envName)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("installed KiCad library root %s is unavailable at %s", envName, path)
	}
	return path
}

func loadClosedLoopManifest(t *testing.T) closedLoopManifest {
	t.Helper()
	var manifest closedLoopManifest
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, "manifest.json")), &manifest)
	return manifest
}

func loadClosedLoopBaselineCases(t *testing.T, manifest closedLoopManifest) []CaseEvidence {
	t.Helper()
	result := make([]CaseEvidence, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		path := filepath.Join(closedLoopBaselineRoot, entry.ID+".json")
		current, err := DecodeCaseEvidence(bytes.NewReader(mustCorpusRead(t, path)))
		if err != nil {
			t.Fatalf("%s: %v", entry.ID, err)
		}
		if current.Case.ID != entry.ID || current.Case.Role != entry.Role || current.Case.Domain != entry.Domain || current.Case.SafetyImpact != entry.SafetyImpact {
			t.Fatalf("%s baseline metadata does not match manifest", entry.ID)
		}
		result = append(result, current)
	}
	return result
}

func splitClosedLoopCases(cases []CaseEvidence) ([]CaseEvidence, []CaseEvidence) {
	discovery, heldOut := []CaseEvidence{}, []CaseEvidence{}
	for _, current := range cases {
		if current.Case.Role == RoleDiscovery {
			discovery = append(discovery, current)
		} else {
			heldOut = append(heldOut, current)
		}
	}
	return discovery, heldOut
}

func buildClosedLoopBaselineReport(
	t *testing.T,
	manifestHash string,
	manifest closedLoopManifest,
	discovery AggregateReport,
	heldOut AggregateReport,
	plan capabilityexpansion.ExpansionPlan,
) closedLoopBaselineReport {
	t.Helper()
	report := closedLoopBaselineReport{
		Schema: closedLoopBaselineSchema, Version: closedLoopBaselineVersion,
		CorpusManifestHash: manifestHash, FreezeCommit: closedLoopCorpusFreezeCommit,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, Environment: manifest.Environment,
		OutcomeCounts: closedLoopOutcomeCounts(append(append([]CaseEvidence{}, discovery.Cases...), heldOut.Cases...)),
		Discovery:     discovery, HeldOut: heldOut, ExpansionPlan: plan,
	}
	hash, err := hashClosedLoopBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.Hash = hash
	return report
}

func buildClosedLoopSelection(t *testing.T, report closedLoopBaselineReport) closedLoopSelection {
	t.Helper()
	if len(report.Discovery.Clusters) == 0 || report.Discovery.Clusters[0].Rank != 1 {
		t.Fatal("baseline report lacks rank one")
	}
	selection := closedLoopSelection{
		Schema: closedLoopSelectionSchema, Version: closedLoopBaselineVersion,
		CorpusManifestHash: report.CorpusManifestHash, FreezeCommit: report.FreezeCommit,
		EvaluatorPolicy: report.EvaluatorPolicy, ImpactRegistryHash: report.ImpactRegistryHash,
		SynthesisPolicyHash: report.SynthesisPolicyHash, BaselineReportHash: report.Hash,
		Cluster: report.Discovery.Clusters[0], ExpansionPlanHash: report.ExpansionPlan.Hash,
	}
	hash, err := hashClosedLoopSelection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.Hash = hash
	return selection
}

func closedLoopOutcomeCounts(cases []CaseEvidence) []closedLoopOutcomeCount {
	result := []closedLoopOutcomeCount{}
	for _, role := range []CorpusRole{RoleDiscovery, RoleHeldOut} {
		result = append(result, countClosedLoopOutcomes(cases, role, ""))
		for _, domain := range []capabilityevaluation.Domain{capabilityevaluation.DomainAnalog, capabilityevaluation.DomainPower, capabilityevaluation.DomainDigital, capabilityevaluation.DomainMCU, capabilityevaluation.DomainSensor, capabilityevaluation.DomainMixedSignal} {
			result = append(result, countClosedLoopOutcomes(cases, role, domain))
		}
	}
	return result
}

func countClosedLoopOutcomes(cases []CaseEvidence, role CorpusRole, domain capabilityevaluation.Domain) closedLoopOutcomeCount {
	result := closedLoopOutcomeCount{Role: role, Domain: domain}
	for _, current := range cases {
		if current.Case.Role != role || domain != "" && current.Case.Domain != domain {
			continue
		}
		switch current.Outcome {
		case OutcomePass:
			result.Pass++
		case OutcomeUnsupported:
			result.Unsupported++
		case OutcomeUnsafe:
			result.Unsafe++
		case OutcomeExhausted:
			result.Exhausted++
		}
	}
	return result
}

func hashClosedLoopBaseline(report closedLoopBaselineReport) (string, error) {
	report.Hash = ""
	return digest(report)
}

func hashClosedLoopSelection(selection closedLoopSelection) (string, error) {
	selection.Hash = ""
	return digest(selection)
}

func writeClosedLoopBaselineArtifacts(t *testing.T, cases []CaseEvidence, report closedLoopBaselineReport, selection closedLoopSelection) {
	t.Helper()
	if err := os.MkdirAll(closedLoopBaselineRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, current := range cases {
		writeClosedLoopCaseEvidence(t, current)
	}
	specRoot := closedLoopSpecDirectory(t)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "BASELINE_REPORT.json"), report)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "SELECTION.json"), selection)
}

func writeClosedLoopCaseEvidence(t *testing.T, current CaseEvidence) {
	t.Helper()
	if err := os.MkdirAll(closedLoopBaselineRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(closedLoopBaselineRoot, current.Case.ID+".json"), corpusJSON(t, current), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeClosedLoopArtifact(t *testing.T, path string, value any) {
	t.Helper()
	data := corpusJSON(t, value)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	checksumPath := strings.TrimSuffix(path, ".json") + ".sha256"
	checksum := []byte(corpusHash(data) + "  " + filepath.Base(path) + "\n")
	if err := os.WriteFile(checksumPath, checksum, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertArtifactChecksum(t *testing.T, checksumPath, name string, data []byte) {
	t.Helper()
	want := corpusHash(data) + "  " + name
	if got := strings.TrimSpace(string(mustCorpusRead(t, checksumPath))); got != want {
		t.Fatalf("%s = %q, want %q", checksumPath, got, want)
	}
}

func firstDifferentByte(left, right []byte) int {
	limit := min(len(left), len(right))
	for index := range limit {
		if left[index] != right[index] {
			return index
		}
	}
	return limit
}

func closedLoopSpecDirectory(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate baseline test source")
	}
	for directory := filepath.Dir(source); ; directory = filepath.Dir(directory) {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && !info.IsDir() {
			return filepath.Join(directory, "specs", "closed-loop-open-set-capability-expansion")
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("locate module root for baseline artifacts")
		}
	}
}

func TestClosedLoopBaselineHashRejectsMutation(t *testing.T) {
	report := closedLoopBaselineReport{Schema: closedLoopBaselineSchema, Version: closedLoopBaselineVersion, FreezeCommit: closedLoopCorpusFreezeCommit}
	hash, err := hashClosedLoopBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.Hash = hash
	report.FreezeCommit = fmt.Sprintf("%040d", 0)
	mutated, err := hashClosedLoopBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	if mutated == hash {
		t.Fatal("baseline mutation did not change its content hash")
	}
}

func TestClosedLoopPromotionLibraryClosureIgnoresUnselectedDiagnostics(t *testing.T) {
	index := libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {LibraryID: "Device:R", LibraryNickname: "Device", Path: "/symbols/Device.kicad_sym"},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0603": {FootprintID: "Resistor_SMD:R_0603", LibraryNickname: "Resistor_SMD", Path: "/footprints/Resistor_SMD.pretty/R_0603.kicad_mod"},
		},
	}
	run := ots.SynthesisRun{Physical: &ots.PhysicalLoweringResult{Resolved: circuitgraph.ResolvedDocument{Components: []circuitgraph.ResolvedComponent{{
		ComponentID: "resistor.generic", VariantID: "0603", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0603",
		Units: []circuitgraph.ResolvedUnit{{Unit: 1}}, Functions: []circuitgraph.ResolvedFunction{{SymbolPin: "1", Pad: "1"}},
	}}}}}
	closure, issues := resolveClosedLoopLibraryClosure(index, run)
	if len(issues) != 0 || len(closure.Symbols) != 1 || len(closure.Footprints) != 1 || len(closure.Variants) != 1 {
		t.Fatalf("closure=%#v issues=%#v", closure, issues)
	}
	loadIssues := []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library.symbol.Unrelated:Bad", Message: "unselected defect"}}
	if filtered := libraryresolver.DesignClosureIssuesFrom(loadIssues, closure); len(filtered) != 0 {
		t.Fatalf("unselected library diagnostic leaked into design closure: %#v", filtered)
	}
}
