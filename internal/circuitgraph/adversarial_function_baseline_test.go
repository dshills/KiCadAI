package circuitgraph

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
	"kicadai/internal/writercorrectness"
)

const (
	adversarialBaselineSchema          = "kicadai.adversarial-function-corpus-baseline.v1"
	adversarialBaselineEvaluator       = "adversarial-promotion-v1"
	adversarialBaselineEvaluatedCommit = "93063343e99d2ec1d7390396ddbd7b94e5983a77"
	adversarialBaselineReportSHA256    = "33285964855f9b2dc41b8d5f44f851cd36877b48cb5b595da78546dfdde46f3e"
	adversarialPromotionModeEnv        = "KICADAI_ADVERSARIAL_PROMOTION"
	adversarialPromotionCasesEnv       = "KICADAI_ADVERSARIAL_CASES"
	adversarialPromotionArtifactDirEnv = "KICADAI_ADVERSARIAL_ARTIFACT_DIR"
)

type adversarialCapabilityIssue struct {
	Code       reports.Code `json:"code"`
	Stage      string       `json:"stage"`
	Path       string       `json:"path,omitempty"`
	Message    string       `json:"message"`
	Suggestion string       `json:"suggestion,omitempty"`
}

type adversarialCapabilityCircuit struct {
	ID             string                      `json:"id"`
	Status         string                      `json:"status"`
	Category       string                      `json:"category,omitempty"`
	RootKey        string                      `json:"root_key,omitempty"`
	RootIssue      *adversarialCapabilityIssue `json:"root_issue,omitempty"`
	Simulation     string                      `json:"simulation"`
	ComponentCount int                         `json:"component_count,omitempty"`
	BoardMM        string                      `json:"board_mm,omitempty"`
	FailureSummary map[string]any              `json:"failure_summary,omitempty"`
	Hashes         map[string]string           `json:"hashes"`
}

type adversarialCapabilityAggregate struct {
	Circuits   int            `json:"circuits"`
	Passed     int            `json:"passed"`
	Blocked    int            `json:"blocked"`
	ByCategory map[string]int `json:"by_category"`
	ByCode     map[string]int `json:"by_code"`
	ByRootKey  map[string]int `json:"by_root_key"`
}

type adversarialCapabilityReport struct {
	Schema               string                         `json:"schema"`
	GeneratedAt          string                         `json:"generated_at"`
	CorpusManifestSHA256 string                         `json:"corpus_manifest_sha256"`
	BaseCommit           string                         `json:"base_commit"`
	EvaluatedCommit      string                         `json:"evaluated_commit"`
	Evaluator            string                         `json:"evaluator"`
	GateProfile          map[string]string              `json:"gate_profile"`
	Circuits             []adversarialCapabilityCircuit `json:"circuits"`
	Aggregate            adversarialCapabilityAggregate `json:"aggregate"`
}

func TestAdversarialFunctionCorpusBaselineReportIsFrozen(t *testing.T) {
	path := adversarialCapabilityReportPath(t, "BASELINE_REPORT.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Hex(contents); got != adversarialBaselineReportSHA256 {
		t.Fatalf("baseline report changed after freeze: got %s want %s", got, adversarialBaselineReportSHA256)
	}
	checksum, err := os.ReadFile(adversarialCapabilityReportPath(t, "BASELINE_REPORT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(checksum)) != adversarialBaselineReportSHA256+"  BASELINE_REPORT.json" {
		t.Fatalf("baseline checksum sidecar does not match the frozen report")
	}
	var report adversarialCapabilityReport
	if err := json.Unmarshal(contents, &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != adversarialBaselineSchema || report.CorpusManifestSHA256 != frozenAdversarialCorpusManifestSHA256 || report.BaseCommit != frozenAdversarialCorpusBaseCommit || report.EvaluatedCommit != adversarialBaselineEvaluatedCommit || report.Evaluator != adversarialBaselineEvaluator {
		t.Fatalf("invalid baseline report header: %#v", report)
	}
	if report.Aggregate.Circuits != 18 || report.Aggregate.Passed+report.Aggregate.Blocked != 18 || len(report.Circuits) != 18 {
		t.Fatalf("invalid baseline aggregate: %#v", report.Aggregate)
	}
	categoryTotal := 0
	for _, count := range report.Aggregate.ByCategory {
		categoryTotal += count
	}
	codeTotal := 0
	for _, count := range report.Aggregate.ByCode {
		codeTotal += count
	}
	rootKeyTotal := 0
	for _, count := range report.Aggregate.ByRootKey {
		rootKeyTotal += count
	}
	if categoryTotal != report.Aggregate.Blocked || codeTotal != report.Aggregate.Blocked || rootKeyTotal != report.Aggregate.Blocked {
		t.Fatalf("baseline failure taxonomy is incomplete: categories=%d codes=%d root_keys=%d blocked=%d", categoryTotal, codeTotal, rootKeyTotal, report.Aggregate.Blocked)
	}
	for index, circuit := range report.Circuits {
		if index > 0 && report.Circuits[index-1].ID >= circuit.ID {
			t.Fatalf("baseline circuits are not strictly sorted at %q", circuit.ID)
		}
		if circuit.Status == "pass" {
			if circuit.Category != "" || circuit.RootIssue != nil || circuit.Hashes["generated_files"] == "" {
				t.Fatalf("baseline pass %s lacks complete evidence: %#v", circuit.ID, circuit)
			}
			continue
		}
		if circuit.Status != "blocked" || circuit.Category == "" || circuit.RootKey == "" || circuit.RootIssue == nil || circuit.RootIssue.Code == "" || circuit.RootIssue.Stage == "" || circuit.RootIssue.Message == "" {
			t.Fatalf("baseline blocker %s is unclassified: %#v", circuit.ID, circuit)
		}
	}
}

func TestAdversarialFunctionCorpusOptionalKiCadPromotion(t *testing.T) {
	mode := strings.TrimSpace(os.Getenv(adversarialPromotionModeEnv))
	if mode == "" {
		t.Skipf("set %s=probe or %s=require-pass to run the adversarial KiCad promotion", adversarialPromotionModeEnv, adversarialPromotionModeEnv)
	}
	if mode != "probe" && mode != "require-pass" {
		t.Fatalf("%s must be probe or require-pass, got %q", adversarialPromotionModeEnv, mode)
	}
	report := evaluateAdversarialCorpus(t, true, adversarialFixtureFilter(t))
	for _, circuit := range report.Circuits {
		if circuit.Status == "pass" {
			t.Logf("%s: pass", circuit.ID)
			continue
		}
		t.Logf("%s: blocked root=%s stage=%s path=%s components=%d board_mm=%s message=%s", circuit.ID, circuit.RootKey, circuit.RootIssue.Stage, circuit.RootIssue.Path, circuit.ComponentCount, circuit.BoardMM, circuit.RootIssue.Message)
		if circuit.FailureSummary != nil {
			t.Logf("%s: failure_summary=%#v", circuit.ID, circuit.FailureSummary)
		}
	}
	if mode == "require-pass" && report.Aggregate.Blocked != 0 {
		t.Fatalf("adversarial promotion blocked: %#v", report.Aggregate)
	}
}

func evaluateAdversarialCorpus(t *testing.T, requireKiCad bool, only map[string]bool) adversarialCapabilityReport {
	t.Helper()
	root := adversarialFunctionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest adversarialCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	footprintsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvFootprintsRoot))
	if requireKiCad && (cliPath == "" || symbolsRoot == "" || footprintsRoot == "") {
		t.Fatalf("set %s, %s, and %s to regenerate the adversarial baseline", checks.EnvKiCadCLI, libraryresolver.EnvSymbolsRoot, libraryresolver.EnvFootprintsRoot)
	}
	var index libraryresolver.LibraryIndex
	if requireKiCad {
		index, _ = libraryresolver.Load(ctx, libraryresolver.LibraryRoots{SymbolsRoot: symbolsRoot, FootprintsRoot: footprintsRoot}, libraryresolver.LoadOptions{})
		if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
			t.Fatalf("library resolution produced no usable symbols or footprints: %#v", libraryresolver.Summary(index))
		}
	}

	report := adversarialCapabilityReport{
		Schema: adversarialBaselineSchema, GeneratedAt: manifest.FrozenAt,
		CorpusManifestSHA256: frozenAdversarialCorpusManifestSHA256,
		BaseCommit:           frozenAdversarialCorpusBaseCommit, EvaluatedCommit: adversarialBaselineEvaluatedCommit,
		Evaluator: adversarialBaselineEvaluator,
		GateProfile: map[string]string{
			"byte_identical_replay": "required", "connectivity_and_route_completion": "required",
			"deterministic_lowering": "required", "kicad_erc": "required", "strict_kicad_drc": "required",
			"simulation_where_applicable": "required", "verified_resolution": "required",
			"writer_correctness": "required", "zero_round_trip_diffs": "required",
		},
		Aggregate: adversarialCapabilityAggregate{
			ByCategory: map[string]int{}, ByCode: map[string]int{}, ByRootKey: map[string]int{},
		},
	}
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "adversarial-function-corpus"})
	for _, fixture := range manifest.Fixtures {
		if only != nil && !only[fixture.ID] {
			continue
		}
		report.Aggregate.Circuits++
		circuit := evaluateAdversarialCircuit(t, ctx, root, fixture, resolver, index, cliPath, requireKiCad)
		report.Circuits = append(report.Circuits, circuit)
		if circuit.Status == "pass" {
			report.Aggregate.Passed++
		} else {
			report.Aggregate.Blocked++
			report.Aggregate.ByCategory[circuit.Category]++
			report.Aggregate.ByCode[string(circuit.RootIssue.Code)]++
			report.Aggregate.ByRootKey[circuit.RootKey]++
		}
	}
	slices.SortFunc(report.Circuits, func(left, right adversarialCapabilityCircuit) int {
		return strings.Compare(left.ID, right.ID)
	})
	return report
}

func adversarialFixtureFilter(t *testing.T) map[string]bool {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(adversarialPromotionCasesEnv))
	if raw == "" {
		return nil
	}
	wanted := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			t.Fatalf("%s contains an empty circuit ID", adversarialPromotionCasesEnv)
		}
		wanted[id] = true
	}
	manifestBytes, err := os.ReadFile(filepath.Join(adversarialFunctionCorpusRoot(t), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest adversarialCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range manifest.Fixtures {
		delete(wanted, fixture.ID)
	}
	if len(wanted) != 0 {
		t.Fatalf("%s contains unknown circuit IDs: %#v", adversarialPromotionCasesEnv, wanted)
	}
	selected := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		selected[strings.TrimSpace(id)] = true
	}
	return selected
}

func evaluateAdversarialCircuit(t *testing.T, ctx context.Context, root string, fixture adversarialCorpusFixture, resolver *Resolver, index libraryresolver.LibraryIndex, cliPath string, requireKiCad bool) adversarialCapabilityCircuit {
	t.Helper()
	circuit := adversarialCapabilityCircuit{ID: fixture.ID, Simulation: "not_evaluated", Hashes: map[string]string{"input": fixture.SHA256}}
	contents, err := os.ReadFile(filepath.Join(root, fixture.File))
	if err != nil {
		t.Fatal(err)
	}
	document, issues := DecodeStrict(strings.NewReader(string(contents)))
	if reports.HasBlockingIssue(issues) {
		return adversarialBlockedCircuit(circuit, "schema", adversarialRootIssue(issues))
	}
	lowered, synthesis, issues := resolver.Synthesize(ctx, document)
	circuit.Hashes["lowered_graph"] = hashGraphValue(lowered)
	if reports.HasBlockingIssue(issues) {
		rootIssue := adversarialRootIssue(issues)
		blocked := adversarialBlockedCircuit(circuit, adversarialSynthesisCategory(rootIssue.Code), rootIssue)
		blocked.RootKey = adversarialSynthesisRootKey(document, rootIssue, blocked.RootKey)
		return blocked
	}
	resolved, issues := resolver.Resolve(ctx, document)
	if reports.HasBlockingIssue(issues) {
		rootIssue := adversarialRootIssue(issues)
		category := "electrical_validation"
		if rootIssue.Code == CodeComponentUnresolved {
			category = "catalog"
		}
		return adversarialBlockedCircuit(circuit, category, rootIssue)
	}
	circuit.Hashes["resolution"] = resolved.ResolutionHash
	requiresTransient := slices.Contains(fixture.Pressures, "bjt_transient") || slices.Contains(fixture.Pressures, "diode_transient")
	if resolved.Simulation == nil {
		if requiresTransient {
			return adversarialBlockedCircuit(circuit, "simulation", reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Stage: "simulation", Path: "simulation.model_id",
				Message:    "transient operating case has no complete trusted transient model",
				Suggestion: "derive a bounded transient analysis from reviewed catalog primitives and operating-case parameters",
			})
		}
		circuit.Simulation = "not_applicable:" + synthesis.Simulation.Reason
	} else {
		if requiresTransient && resolved.Simulation.ModelID != simmodel.ModelTransientCircuitV1 {
			return adversarialBlockedCircuit(circuit, "simulation", reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Stage: "simulation", Path: "simulation.model_id",
				Message:    "transient operating case resolved only " + resolved.Simulation.ModelID + " instead of a trusted transient analysis",
				Suggestion: "derive the bounded transient grid and pulse excitation from function intent without provider-controlled solver policy",
			})
		}
		simulationReport, diagnostics := simmodel.Evaluate(*resolved.Simulation)
		if len(diagnostics) != 0 || simulationReport.Status != "pass" {
			message := "trusted simulation did not pass"
			path := "simulation"
			suggestion := "correct graph connectivity or add compatible reviewed catalog model evidence"
			if len(diagnostics) != 0 {
				message = diagnostics[0].Message
				path = diagnostics[0].Path
				suggestion = diagnostics[0].Suggestion
			}
			return adversarialBlockedCircuit(circuit, "simulation", reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Stage: "simulation", Path: path, Message: message, Suggestion: suggestion})
		}
		circuit.Simulation = "pass:" + resolved.Simulation.ModelID
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
		return adversarialBlockedCircuit(circuit, "schematic", adversarialRootIssue(issues))
	}
	request.Validation.RequireERC = true
	request.Validation.RequireDRC = true
	request.Validation.StrictUnrouted = true
	circuit.ComponentCount = len(request.ExplicitCircuit.Components)
	circuit.BoardMM = strconv.FormatFloat(request.Board.WidthMM, 'f', -1, 64) + "x" + strconv.FormatFloat(request.Board.HeightMM, 'f', -1, 64)
	circuit.Hashes["request"] = hashGraphValue(request)
	if !requireKiCad {
		return adversarialBlockedCircuit(circuit, "writer", reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityBlocked, Stage: "kicad_checks", Path: "kicad_cli", Message: "complete baseline requires KiCad-backed promotion"})
	}

	artifactRoot := strings.TrimSpace(os.Getenv(adversarialPromotionArtifactDirEnv))
	if artifactRoot != "" {
		if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
			t.Fatalf("create adversarial artifact root: %v", err)
		}
	}
	firstOutput := filepath.Join(t.TempDir(), "first")
	if artifactRoot != "" {
		firstOutput = filepath.Join(artifactRoot, fixture.ID, "first")
	}
	first := designworkflow.Create(ctx, request, adversarialCreateOptions(firstOutput, index, cliPath))
	if category, issue, failed := adversarialWorkflowFailure(first); failed {
		blocked := adversarialBlockedCircuit(circuit, category, issue)
		stageName := designworkflow.StageName(issue.Stage)
		if category == "placement" {
			stageName = designworkflow.StagePlacement
		}
		if stage := graphWorkflowStage(first, stageName); stage != nil {
			blocked.FailureSummary = stage.Summary
		}
		return blocked
	}
	circuit.Hashes["generated_files"] = hashFunctionGeneratedFiles(t, firstOutput)

	secondOutput := filepath.Join(t.TempDir(), "second")
	if artifactRoot != "" {
		secondOutput = filepath.Join(artifactRoot, fixture.ID, "second")
	}
	second := designworkflow.Create(ctx, request, adversarialCreateOptions(secondOutput, index, cliPath))
	if category, issue, failed := adversarialWorkflowFailure(second); failed {
		return adversarialBlockedCircuit(circuit, category, issue)
	}
	for _, suffix := range []string{".kicad_pro", ".kicad_sch", ".kicad_pcb"} {
		firstBytes, err := os.ReadFile(filepath.Join(firstOutput, request.Name+suffix))
		if err != nil {
			t.Fatal(err)
		}
		secondBytes, err := os.ReadFile(filepath.Join(secondOutput, request.Name+suffix))
		if err != nil {
			t.Fatal(err)
		}
		if string(firstBytes) != string(secondBytes) {
			return adversarialBlockedCircuit(circuit, "round_trip", reports.Issue{
				Code: reports.CodeRoundTripDiff, Severity: reports.SeverityError, Stage: "replay", Path: suffix,
				Message: "generated file is not byte-identical across replay; normalized difference starts at byte " + strconv.Itoa(firstDifferenceOffset(roundtrip.NormalizeBytes(firstBytes), roundtrip.NormalizeBytes(secondBytes))),
			})
		}
	}
	circuit.Status = "pass"
	return circuit
}

func adversarialCreateOptions(output string, index libraryresolver.LibraryIndex, cliPath string) designworkflow.CreateOptions {
	return designworkflow.CreateOptions{
		OutputDir: output, Overwrite: true, LibraryIndex: &index,
		Validation:  designworkflow.ValidationOptions{StrictUnrouted: true, RequireDRC: true, KiCadCLI: cliPath},
		KiCadChecks: designworkflow.KiCadCheckOptions{KiCadCLI: cliPath, RequireERC: true, RequireDRC: true, EnforceRequirements: true},
		Writer: writercorrectness.Options{
			RequireKiCadRoundTrip: true, KiCadCLI: cliPath, ArtifactDir: filepath.Join(output, ".roundtrip"), StrictDiffs: true,
			LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true,
		},
	}
}

func adversarialWorkflowFailure(result designworkflow.WorkflowResult) (string, reports.Issue, bool) {
	required := []designworkflow.StageName{
		designworkflow.StageSchematic, designworkflow.StageSchematicElectrical, designworkflow.StagePlacement,
		designworkflow.StageRouting, designworkflow.StageProjectWrite, designworkflow.StageWriterCorrect,
		designworkflow.StageValidation, designworkflow.StageKiCadChecks,
	}
	for _, name := range required {
		stage := graphWorkflowStage(result, name)
		if stage == nil {
			return adversarialStageCategory(name, reports.Issue{}), reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Stage: string(name), Path: string(name), Message: "required workflow stage is missing"}, true
		}
		if stage.Status == designworkflow.StageStatusBlocked || reports.HasBlockingIssue(stage.Issues) {
			issue := adversarialRootIssue(stage.Issues)
			return adversarialStageCategory(name, issue), issue, true
		}
		if stage.Status == designworkflow.StageStatusSkipped {
			return adversarialStageCategory(name, reports.Issue{}), reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Stage: string(name), Path: string(name), Message: "required workflow stage was skipped"}, true
		}
	}
	return "", reports.Issue{}, false
}

func adversarialRootIssue(issues []reports.Issue) reports.Issue {
	candidates := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Blocking() {
			candidates = append(candidates, issue)
		}
	}
	if len(candidates) == 0 {
		return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Stage: "unknown", Message: "blocking stage has no structured root issue"}
	}
	slices.SortStableFunc(candidates, func(left, right reports.Issue) int {
		leftPriority := adversarialIssuePriority(left)
		rightPriority := adversarialIssuePriority(right)
		if leftPriority != rightPriority {
			return leftPriority - rightPriority
		}
		if left.Path != right.Path {
			return strings.Compare(left.Path, right.Path)
		}
		return strings.Compare(string(left.Code), string(right.Code))
	})
	return candidates[0]
}

func adversarialIssuePriority(issue reports.Issue) int {
	if strings.HasPrefix(issue.Path, "explicit_circuit.nets.") {
		// Route-completion enforcement is a downstream wrapper. Prefer the
		// router's endpoint/search diagnostic when both are present.
		return 5
	}
	switch issue.Code {
	case CodeSynthesisComponentUnresolved, CodeComponentUnresolved:
		return 0
	case CodeSynthesisInterfaceUnsupported, CodeSynthesisSupportRecipeMissing, CodeSynthesisSupportRecipeAmbiguous:
		return 1
	case CodeSynthesisUnusedPinPolicyMissing:
		return 2
	case CodeSynthesisPowerDomainInvalid, CodeSynthesisConnectionUnresolved:
		return 3
	default:
		return 4
	}
}

func adversarialBlockedCircuit(circuit adversarialCapabilityCircuit, category string, issue reports.Issue) adversarialCapabilityCircuit {
	if issue.Code == "" {
		issue.Code = reports.CodeValidationFailed
	}
	if issue.Stage == "" {
		issue.Stage = category
	}
	if issue.Message == "" {
		issue.Message = "blocking stage has no structured root issue"
	}
	circuit.Status = "blocked"
	circuit.Category = category
	circuit.RootKey = adversarialIssueRootKey(category, issue)
	circuit.RootIssue = &adversarialCapabilityIssue{
		Code: issue.Code, Stage: issue.Stage, Path: issue.Path, Message: issue.Message, Suggestion: issue.Suggestion,
	}
	return circuit
}

func adversarialSynthesisRootKey(document Document, issue reports.Issue, fallback string) string {
	const prefix = "synthesis.functions."
	if document.Synthesis == nil || !strings.HasPrefix(issue.Path, prefix) {
		return fallback
	}
	functionID := strings.TrimPrefix(issue.Path, prefix)
	if separator := strings.IndexByte(functionID, '.'); separator >= 0 {
		functionID = functionID[:separator]
	}
	for _, function := range document.Synthesis.Functions {
		if function.ID == functionID && function.Query != nil && strings.TrimSpace(function.Query.Family) != "" {
			return "catalog." + strings.TrimSpace(function.Query.Family) + ".component_unresolved"
		}
	}
	return fallback
}

func adversarialIssueRootKey(category string, issue reports.Issue) string {
	path := issue.Path
	for _, collection := range []string{"components", "nets", "functions"} {
		prefix := collection + "."
		if strings.HasPrefix(path, prefix) {
			path = collection
			break
		}
		marker := "." + collection + "."
		if index := strings.Index(path, marker); index >= 0 {
			path = path[:index+len(marker)-1]
			break
		}
	}
	parts := []string{category, string(issue.Code)}
	if path != "" {
		parts = append(parts, path)
	}
	return strings.Join(parts, ".")
}

func adversarialSynthesisCategory(code reports.Code) string {
	switch code {
	case CodeSynthesisComponentUnresolved, CodeSynthesisInterfaceUnsupported, CodeSynthesisSupportRecipeMissing, CodeSynthesisSupportRecipeAmbiguous:
		return "catalog"
	case CodeSynthesisUnusedPinPolicyMissing:
		return "electrical_validation"
	default:
		return "synthesis"
	}
}

func adversarialStageCategory(stage designworkflow.StageName, issue reports.Issue) string {
	switch stage {
	case designworkflow.StageSchematic:
		return "schematic"
	case designworkflow.StageSchematicElectrical:
		return "electrical_validation"
	case designworkflow.StagePlacement:
		return "placement"
	case designworkflow.StageRouting:
		return "routing"
	case designworkflow.StageProjectWrite:
		return "writer"
	case designworkflow.StageWriterCorrect:
		if issue.Code == reports.CodeRoundTripDiff || strings.Contains(strings.ToLower(issue.Path), "round") {
			return "round_trip"
		}
		return "writer"
	case designworkflow.StageValidation:
		if issue.Code == reports.CodeDisconnectedPad || issue.Code == reports.CodeRouteGraphIncomplete || issue.Code == reports.CodeRouteCompletionPartial {
			return "routing"
		}
		return "electrical_validation"
	case designworkflow.StageKiCadChecks:
		if strings.Contains(strings.ToLower(issue.Path), "drc") || strings.Contains(strings.ToLower(issue.Message), "clearance") || strings.Contains(strings.ToLower(issue.Message), "unrouted") {
			return "routing"
		}
		return "electrical_validation"
	default:
		return "writer"
	}
}

func adversarialCapabilityReportPath(t *testing.T, name string) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate adversarial capability report")
	}
	return filepath.Join(filepath.Dir(sourcePath), "..", "..", "specs", "adversarial-function-corpus", name)
}
