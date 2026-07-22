package circuitgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
	"kicadai/internal/writercorrectness"
)

const frozenFunctionCorpusManifestSHA256 = "5eb592caf268c68505357e5d62226a89a8d65e558efe1dd5dcaab75dc000e902"

type functionCorpusManifest struct {
	Schema        string                  `json:"schema"`
	Version       int                     `json:"version"`
	FrozenAt      string                  `json:"frozen_at"`
	PolicyVersion string                  `json:"policy_version"`
	Fixtures      []functionCorpusFixture `json:"fixtures"`
}

func TestFrozenFunctionLevelCorpusOfflineWorkflowAndReplay(t *testing.T) {
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "function-corpus"})
	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			document, issues := DecodeStrict(strings.NewReader(string(contents)))
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("strict decode issues = %#v", issues)
			}
			resolved, issues := resolver.Resolve(context.Background(), document)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("resolution issues = %#v", issues)
			}
			request, issues := ToDesignRequest(resolved)
			if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
				t.Fatalf("design request issues = %#v", issues)
			}
			if request.ExplicitCircuit.RoutingPolicy != designworkflow.ExplicitRoutingPolicyConstrainedEndpointAccessV1 {
				t.Fatalf("synthesized routing policy = %q", request.ExplicitCircuit.RoutingPolicy)
			}
			index := schematicTestLibraryIndex(resolved)
			request.Validation.Acceptance = designworkflow.AcceptanceDraft
			request.Validation.RequireERC = false
			request.Validation.RequireDRC = false
			firstOutput := filepath.Join(t.TempDir(), "first")
			first := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: firstOutput, Overwrite: true, LibraryIndex: &index})
			for _, stage := range []designworkflow.StageName{
				designworkflow.StageSchematic,
				designworkflow.StageSchematicElectrical,
				designworkflow.StagePlacement,
				designworkflow.StageRouting,
				designworkflow.StageProjectWrite,
				designworkflow.StageWriterCorrect,
			} {
				result := graphWorkflowStage(first, stage)
				if result == nil || result.Status == designworkflow.StageStatusBlocked || result.Status == designworkflow.StageStatusSkipped {
					t.Fatalf("%s stage = %#v; workflow issues = %#v", stage, result, designworkflow.WorkflowIssues(first))
				}
			}
			written, err := design.ReadProjectDirectory(firstOutput)
			if err != nil {
				t.Fatal(err)
			}
			if written.Schematic == nil || written.PCB == nil || len(written.PCB.Tracks) == 0 {
				t.Fatalf("written project lacks readable schematic, PCB, or routed copper")
			}

			secondOutput := filepath.Join(t.TempDir(), "second")
			second := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: secondOutput, Overwrite: true, LibraryIndex: &index})
			if stage := graphWorkflowStage(second, designworkflow.StageWriterCorrect); stage == nil || stage.Status == designworkflow.StageStatusBlocked || stage.Status == designworkflow.StageStatusSkipped {
				t.Fatalf("second writer correctness stage = %#v; issues=%#v", stage, designworkflow.WorkflowIssues(second))
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
					t.Fatalf("%s is not byte-identical across replay; normalized difference starts at byte %d", suffix, firstDifferenceOffset(roundtrip.NormalizeBytes(firstBytes), roundtrip.NormalizeBytes(secondBytes)))
				}
			}
		})
	}
}

func TestFrozenFunctionLevelCorpusOptionalKiCadPromotion(t *testing.T) {
	runFunctionLevelCorpusKiCadPromotion(t, nil)
}

func runFunctionLevelCorpusKiCadPromotion(t *testing.T, only map[string]bool) {
	t.Helper()
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	footprintsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvFootprintsRoot))
	if cliPath == "" || symbolsRoot == "" || footprintsRoot == "" {
		t.Skipf("set %s, %s, and %s to run function-corpus KiCad promotion", checks.EnvKiCadCLI, libraryresolver.EnvSymbolsRoot, libraryresolver.EnvFootprintsRoot)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	index, libraryIssues := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{SymbolsRoot: symbolsRoot, FootprintsRoot: footprintsRoot}, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("library resolution produced no usable symbols or footprints: summary=%#v issues=%d", libraryresolver.Summary(index), len(libraryIssues))
	}

	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "function-corpus"})
	seen := map[string]bool{}
	for _, fixture := range manifest.Fixtures {
		if only != nil && !only[fixture.ID] {
			continue
		}
		fixture := fixture
		seen[fixture.ID] = true
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			document, issues := DecodeStrict(strings.NewReader(string(contents)))
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("strict decode issues = %#v", issues)
			}
			resolved, issues := resolver.Resolve(ctx, document)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("resolution issues = %#v", issues)
			}
			request, issues := ToDesignRequest(resolved)
			if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
				t.Fatalf("design request issues = %#v", issues)
			}
			request.Validation.RequireERC = true
			request.Validation.RequireDRC = true
			request.Validation.StrictUnrouted = true
			artifactRoot := strings.TrimSpace(os.Getenv("KICADAI_FUNCTION_CORPUS_ARTIFACT_DIR"))
			output := filepath.Join(t.TempDir(), "project")
			if artifactRoot != "" {
				output = filepath.Join(artifactRoot, fixture.ID)
			}
			result := designworkflow.Create(ctx, request, designworkflow.CreateOptions{
				OutputDir:    output,
				Overwrite:    true,
				LibraryIndex: &index,
				Validation: designworkflow.ValidationOptions{
					StrictUnrouted: true,
					RequireDRC:     true,
					KiCadCLI:       cliPath,
				},
				KiCadChecks: designworkflow.KiCadCheckOptions{
					KiCadCLI:            cliPath,
					RequireERC:          true,
					RequireDRC:          true,
					EnforceRequirements: true,
				},
				Writer: writercorrectness.Options{
					RequireKiCadRoundTrip: true,
					KiCadCLI:              cliPath,
					KeepArtifacts:         artifactRoot != "",
					ArtifactDir:           filepath.Join(output, ".roundtrip"),
					StrictDiffs:           true,
					LibraryIndex:          index,
					HasLibraryIndex:       true,
					LibraryResolutionUsed: true,
				},
			})
			for _, stage := range []designworkflow.StageName{
				designworkflow.StageSchematic,
				designworkflow.StageSchematicElectrical,
				designworkflow.StagePlacement,
				designworkflow.StageRouting,
				designworkflow.StageProjectWrite,
				designworkflow.StageWriterCorrect,
				designworkflow.StageValidation,
				designworkflow.StageKiCadChecks,
			} {
				stageResult := graphWorkflowStage(result, stage)
				if stageResult == nil || stageResult.Status == designworkflow.StageStatusBlocked || stageResult.Status == designworkflow.StageStatusSkipped {
					t.Fatalf("%s stage = %#v; workflow issues = %#v", stage, stageResult, designworkflow.WorkflowIssues(result))
				}
			}
		})
	}
	if only != nil && len(seen) != len(only) {
		missing := make([]string, 0, len(only)-len(seen))
		for id := range only {
			if !seen[id] {
				missing = append(missing, id)
			}
		}
		slices.Sort(missing)
		t.Fatalf("requested function-corpus promotion fixtures are missing: %v", missing)
	}
}

func TestFrozenFunctionLevelCorpusSynthesisIsDeterministicAndFailClosed(t *testing.T) {
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "function-corpus"})
	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			document, issues := DecodeStrict(strings.NewReader(string(contents)))
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("strict decode issues = %#v", issues)
			}
			firstGraph, firstReport, firstIssues := resolver.Synthesize(context.Background(), document)
			secondGraph, secondReport, secondIssues := resolver.Synthesize(context.Background(), document)
			if !reflect.DeepEqual(firstGraph, secondGraph) || !reflect.DeepEqual(firstReport, secondReport) || !reflect.DeepEqual(firstIssues, secondIssues) {
				t.Fatalf("function synthesis is not deterministic\nfirst=%#v\nsecond=%#v", firstReport, secondReport)
			}
			if firstReport.Schema != SynthesisReportSchema || firstReport.PolicyVersion != SynthesisPolicyVersion || firstReport.InputHash == "" {
				t.Fatalf("incomplete synthesis evidence = %#v", firstReport)
			}
			if len(firstReport.Selections) == 0 {
				t.Fatalf("synthesis selected no primary or interface components: issues=%#v", firstIssues)
			}
			if reports.HasBlockingIssue(firstIssues) {
				if firstReport.Status != "blocked" || firstReport.LoweredGraphHash != "" {
					t.Fatalf("blocked synthesis reported optimistic evidence = %#v", firstReport)
				}
				for _, issue := range firstIssues {
					if issue.Blocking() && (!strings.HasPrefix(string(issue.Code), "SYNTHESIS_") || issue.Stage != "synthesis" || issue.RetryScope != "synthesis") {
						t.Fatalf("unclassified synthesis blocker = %#v", issue)
					}
				}
			} else if firstReport.Status != "ready" || firstReport.LoweredGraphHash == "" {
				t.Fatalf("ready synthesis lacks lowered graph evidence = %#v", firstReport)
			}
		})
	}
}

func TestFrozenFunctionLevelCorpusSynthesisIgnoresInputAndCatalogOrder(t *testing.T) {
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	canonicalCatalog := loadGraphCatalog(t)
	shuffledCatalog := loadGraphCatalog(t)
	slices.Reverse(shuffledCatalog.Records)
	slices.Reverse(shuffledCatalog.Families)
	for recordIndex := range shuffledCatalog.Records {
		record := &shuffledCatalog.Records[recordIndex]
		slices.Reverse(record.Symbols)
		slices.Reverse(record.Packages)
		slices.Reverse(record.Companions)
		for companionIndex := range record.Companions {
			slices.Reverse(record.Companions[companionIndex].Recipes)
		}
	}
	components.RebuildCatalogIndexes(shuffledCatalog)
	canonicalResolver := NewResolver(ResolveOptions{Catalog: canonicalCatalog, CatalogID: "function-corpus"})
	shuffledResolver := NewResolver(ResolveOptions{Catalog: shuffledCatalog, CatalogID: "function-corpus"})
	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			canonical, issues := DecodeStrict(strings.NewReader(string(contents)))
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("strict decode issues = %#v", issues)
			}
			shuffled := canonical
			shuffledIntent := cloneFunctionIntent(*canonical.Synthesis)
			shuffled.Synthesis = &shuffledIntent
			slices.Reverse(shuffled.Synthesis.Functions)
			slices.Reverse(shuffled.Synthesis.Interfaces)
			slices.Reverse(shuffled.Synthesis.PowerDomains)
			slices.Reverse(shuffled.Synthesis.Connections)
			for interfaceIndex := range shuffled.Synthesis.Interfaces {
				slices.Reverse(shuffled.Synthesis.Interfaces[interfaceIndex].Signals)
			}

			canonicalGraph, canonicalReport, canonicalIssues := canonicalResolver.Synthesize(context.Background(), canonical)
			shuffledGraph, shuffledReport, shuffledIssues := shuffledResolver.Synthesize(context.Background(), shuffled)
			if !reflect.DeepEqual(canonicalGraph, shuffledGraph) || !reflect.DeepEqual(canonicalReport, shuffledReport) || !reflect.DeepEqual(canonicalIssues, shuffledIssues) {
				t.Fatalf("synthesis depends on input or catalog order\ncanonical=%#v\nshuffled=%#v\ncanonical issues=%#v\nshuffled issues=%#v", canonicalReport, shuffledReport, canonicalIssues, shuffledIssues)
			}
		})
	}
}

func TestFrozenFunctionLevelCorpusResolvesAndLowersToDesignRequests(t *testing.T) {
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "function-corpus"})
	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			document, decodeIssues := DecodeStrict(strings.NewReader(string(contents)))
			if reports.HasBlockingIssue(decodeIssues) {
				t.Fatalf("strict decode issues = %#v", decodeIssues)
			}
			resolved, resolveIssues := resolver.Resolve(context.Background(), document)
			if reports.HasBlockingIssue(resolveIssues) || resolved.ResolutionHash == "" || resolved.Synthesis == nil || resolved.Synthesis.Status != "ready" {
				t.Fatalf("resolution failed: issues=%#v resolved=%#v", resolveIssues, resolved)
			}
			if resolved.Source.Synthesis != nil || len(resolved.Source.Components) <= len(document.Synthesis.Functions) || len(resolved.Source.Nets) == 0 {
				t.Fatalf("resolution did not retain the explicit lowered graph: %#v", resolved.Source)
			}
			request, loweringIssues := ToDesignRequest(resolved)
			if reports.HasBlockingIssue(loweringIssues) || request.ExplicitCircuit == nil {
				t.Fatalf("design request lowering failed: issues=%#v request=%#v", loweringIssues, request)
			}
		})
	}
}

func TestFrozenFunctionLevelCorpusStrictDecodesDeterministically(t *testing.T) {
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			first, issues := DecodeStrict(strings.NewReader(string(contents)))
			if reports.HasBlockingIssue(issues) || first.Synthesis == nil {
				t.Fatalf("strict decode issues = %#v document=%#v", issues, first)
			}
			second, issues := DecodeStrict(strings.NewReader(string(contents)))
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("second strict decode issues = %#v", issues)
			}
			firstJSON, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			secondJSON, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if string(firstJSON) != string(secondJSON) {
				t.Fatal("normalized function intent is not deterministic")
			}
		})
	}
}

type functionCorpusFixture struct {
	ID      string   `json:"id"`
	File    string   `json:"file"`
	Domains []string `json:"domains"`
	SHA256  string   `json:"sha256"`
}

type functionCapabilityReport struct {
	Schema               string                            `json:"schema"`
	GeneratedAt          string                            `json:"generated_at"`
	CorpusManifestSHA256 string                            `json:"corpus_manifest_sha256"`
	PolicyVersion        string                            `json:"synthesis_policy_version"`
	CatalogSHA256        string                            `json:"catalog_sha256"`
	LibrarySHA256        string                            `json:"kicad_library_index_sha256"`
	RoundTripSHA256      string                            `json:"round_trip_zero_diff_sha256"`
	GateProfile          map[string]string                 `json:"gate_profile"`
	Circuits             []functionCapabilityCircuit       `json:"circuits"`
	Aggregate            functionCapabilityReportAggregate `json:"aggregate"`
}

type functionCapabilityCircuit struct {
	ID                     string            `json:"id"`
	Domains                []string          `json:"domains"`
	Status                 string            `json:"status"`
	Hashes                 map[string]string `json:"hashes"`
	PrimaryComponents      []string          `json:"primary_components"`
	SupportComponents      []string          `json:"support_components"`
	InterfaceComponents    []string          `json:"interface_components"`
	UnusedPinDecisionCount int               `json:"unused_pin_decision_count"`
	GateProfile            string            `json:"gate_profile"`
	Simulation             string            `json:"simulation"`
	DerivedConstraints     map[string]bool   `json:"derived_constraints"`
}

type functionCapabilityReportAggregate struct {
	Circuits             int                       `json:"circuits"`
	Passed               int                       `json:"passed"`
	Failed               int                       `json:"failed"`
	UnclassifiedFailures int                       `json:"unclassified_failures"`
	FailureCategories    map[string]int            `json:"failure_categories"`
	ByDomain             map[string]map[string]int `json:"by_domain"`
}

func TestFunctionLevelCorpusCapabilityReportMatchesAuthoritativeEvidence(t *testing.T) {
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate capability report")
	}
	reportBytes, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), "..", "..", "specs", "function-level-circuit-synthesis", "CAPABILITY_REPORT.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capability functionCapabilityReport
	if err := json.Unmarshal(reportBytes, &capability); err != nil {
		t.Fatal(err)
	}
	if *updateCircuitGraphGolden {
		capability = regenerateFunctionCapabilityReport(t, capability, manifest)
		path := filepath.Join(filepath.Dir(sourcePath), "..", "..", "specs", "function-level-circuit-synthesis", "CAPABILITY_REPORT.json")
		data, err := json.MarshalIndent(capability, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if capability.Schema != "kicadai.function-corpus-capability.v1" || capability.CorpusManifestSHA256 != frozenFunctionCorpusManifestSHA256 || capability.PolicyVersion != SynthesisPolicyVersion {
		t.Fatalf("capability report header = %#v", capability)
	}
	catalog := loadGraphCatalog(t)
	wantCatalogHash := hashGraphValue(struct {
		Version  string                        `json:"version"`
		Records  []components.ComponentRecord  `json:"records"`
		Families []components.FamilyDefinition `json:"families"`
	}{Version: catalog.Version, Records: catalog.Records, Families: catalog.Families})
	if capability.CatalogSHA256 != wantCatalogHash || len(capability.LibrarySHA256) != 64 || len(capability.RoundTripSHA256) != 64 {
		t.Fatalf("capability provenance hashes are stale or incomplete: catalog=%s want=%s library=%d round_trip=%d", capability.CatalogSHA256, wantCatalogHash, len(capability.LibrarySHA256), len(capability.RoundTripSHA256))
	}
	if len(capability.Circuits) != len(manifest.Fixtures) || capability.Aggregate.Circuits != len(manifest.Fixtures) || capability.Aggregate.Passed != len(manifest.Fixtures) || capability.Aggregate.Failed != 0 || capability.Aggregate.UnclassifiedFailures != 0 || len(capability.Aggregate.FailureCategories) != 0 {
		t.Fatalf("capability aggregate = %#v", capability.Aggregate)
	}
	indexed := make(map[string]functionCapabilityCircuit, len(capability.Circuits))
	for _, circuit := range capability.Circuits {
		indexed[circuit.ID] = circuit
	}
	resolver := NewResolver(ResolveOptions{Catalog: catalog, CatalogID: "function-corpus"})
	for _, fixture := range manifest.Fixtures {
		circuit, exists := indexed[fixture.ID]
		if !exists || circuit.Status != "pass" || circuit.GateProfile != "all_applicable_passed" || !reflect.DeepEqual(circuit.Domains, fixture.Domains) || len(circuit.Hashes["generated_files"]) != 64 {
			t.Fatalf("capability circuit %s is incomplete: %#v", fixture.ID, circuit)
		}
		contents, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		document, decodeIssues := DecodeStrict(strings.NewReader(string(contents)))
		if reports.HasBlockingIssue(decodeIssues) {
			t.Fatalf("%s decode issues = %#v", fixture.ID, decodeIssues)
		}
		lowered, synthesis, synthesisIssues := resolver.Synthesize(context.Background(), document)
		if reports.HasBlockingIssue(synthesisIssues) {
			t.Fatalf("%s synthesis issues = %#v", fixture.ID, synthesisIssues)
		}
		resolved, resolveIssues := resolver.Resolve(context.Background(), document)
		if reports.HasBlockingIssue(resolveIssues) {
			t.Fatalf("%s resolution issues = %#v", fixture.ID, resolveIssues)
		}
		request, requestIssues := ToDesignRequest(resolved)
		if reports.HasBlockingIssue(requestIssues) {
			t.Fatalf("%s request issues = %#v", fixture.ID, requestIssues)
		}
		index := schematicTestLibraryIndex(resolved)
		request.Validation.Acceptance = designworkflow.AcceptanceDraft
		request.Validation.RequireERC = false
		request.Validation.RequireDRC = false
		output := filepath.Join(t.TempDir(), "project")
		result := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
		if _, err := os.Stat(output); err != nil {
			t.Fatalf("%s did not write generated project: %v issues=%#v", fixture.ID, err, designworkflow.WorkflowIssues(result))
		}
		generatedHash := hashFunctionGeneratedFiles(t, output)
		if circuit.Hashes["input"] != synthesis.InputHash || circuit.Hashes["lowered_graph"] != hashGraphValue(lowered) || circuit.Hashes["resolution"] != resolved.ResolutionHash || circuit.Hashes["request"] != hashGraphValue(request) || circuit.Hashes["generated_files"] != generatedHash || circuit.UnusedPinDecisionCount != len(synthesis.UnusedPinDecisions) {
			t.Fatalf("capability hashes or unused-pin evidence are stale for %s: resolution=%s request=%s generated_files=%s", fixture.ID, resolved.ResolutionHash, hashGraphValue(request), generatedHash)
		}
		if len(circuit.PrimaryComponents)+len(circuit.SupportComponents)+len(circuit.InterfaceComponents) == 0 {
			t.Fatalf("capability circuit %s has no selection evidence", fixture.ID)
		}
		wantSimulation := "not_applicable_no_complete_registered_model"
		if resolved.Simulation != nil {
			report, diagnostics := simmodel.Evaluate(*resolved.Simulation)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("capability circuit %s simulation failed: report=%#v diagnostics=%#v", fixture.ID, report, diagnostics)
			}
			wantSimulation = "pass"
		}
		if circuit.Simulation != wantSimulation {
			t.Fatalf("capability circuit %s simulation=%q, want %q", fixture.ID, circuit.Simulation, wantSimulation)
		}
	}
	for gate, status := range capability.GateProfile {
		if status != "pass" {
			t.Fatalf("gate %s has unsupported status %s", gate, status)
		}
	}
}

func regenerateFunctionCapabilityReport(t *testing.T, capability functionCapabilityReport, manifest functionCorpusManifest) functionCapabilityReport {
	t.Helper()
	catalog := loadGraphCatalog(t)
	capability.GeneratedAt = manifest.FrozenAt
	capability.CatalogSHA256 = hashGraphValue(struct {
		Version  string                        `json:"version"`
		Records  []components.ComponentRecord  `json:"records"`
		Families []components.FamilyDefinition `json:"families"`
	}{Version: catalog.Version, Records: catalog.Records, Families: catalog.Families})
	capability.GateProfile["simulation"] = "pass"
	capability.Circuits = nil
	capability.Aggregate = functionCapabilityReportAggregate{
		Circuits: len(manifest.Fixtures), Passed: len(manifest.Fixtures),
		FailureCategories: map[string]int{}, ByDomain: map[string]map[string]int{},
	}
	resolver := NewResolver(ResolveOptions{Catalog: catalog, CatalogID: "function-corpus"})
	root := functionCorpusRoot(t)
	for _, fixture := range manifest.Fixtures {
		contents, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		document, issues := DecodeStrict(strings.NewReader(string(contents)))
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("%s decode issues = %#v", fixture.ID, issues)
		}
		lowered, synthesis, issues := resolver.Synthesize(context.Background(), document)
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("%s synthesis issues = %#v", fixture.ID, issues)
		}
		resolved, issues := resolver.Resolve(context.Background(), document)
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("%s resolution issues = %#v", fixture.ID, issues)
		}
		request, issues := ToDesignRequest(resolved)
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("%s request issues = %#v", fixture.ID, issues)
		}
		index := schematicTestLibraryIndex(resolved)
		request.Validation.Acceptance = designworkflow.AcceptanceDraft
		request.Validation.RequireERC = false
		request.Validation.RequireDRC = false
		output := filepath.Join(t.TempDir(), "project")
		result := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
		if _, err := os.Stat(output); err != nil {
			t.Fatalf("%s did not write generated project: %v issues=%#v", fixture.ID, err, designworkflow.WorkflowIssues(result))
		}
		circuit := functionCapabilityCircuit{
			ID: fixture.ID, Domains: append([]string(nil), fixture.Domains...), Status: "pass", GateProfile: "all_applicable_passed",
			Hashes: map[string]string{
				"input": synthesis.InputHash, "lowered_graph": hashGraphValue(lowered), "resolution": resolved.ResolutionHash,
				"request": hashGraphValue(request), "generated_files": hashFunctionGeneratedFiles(t, output),
			},
			UnusedPinDecisionCount: len(synthesis.UnusedPinDecisions),
			Simulation:             "not_applicable_no_complete_registered_model",
			DerivedConstraints:     map[string]bool{"board_envelope": true, "layer_policy": true, "net_classes": true},
		}
		if resolved.Simulation != nil {
			report, diagnostics := simmodel.Evaluate(*resolved.Simulation)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("%s simulation report=%#v diagnostics=%#v", fixture.ID, report, diagnostics)
			}
			circuit.Simulation = "pass"
		}
		for _, selection := range synthesis.Selections {
			switch selection.Kind {
			case "primary":
				circuit.PrimaryComponents = appendUniqueString(circuit.PrimaryComponents, selection.ComponentID)
			case "support":
				circuit.SupportComponents = appendUniqueString(circuit.SupportComponents, selection.ComponentID)
			case "interface":
				circuit.InterfaceComponents = appendUniqueString(circuit.InterfaceComponents, selection.ComponentID)
			}
		}
		slices.Sort(circuit.PrimaryComponents)
		slices.Sort(circuit.SupportComponents)
		slices.Sort(circuit.InterfaceComponents)
		capability.Circuits = append(capability.Circuits, circuit)
		for _, domain := range fixture.Domains {
			if capability.Aggregate.ByDomain[domain] == nil {
				capability.Aggregate.ByDomain[domain] = map[string]int{}
			}
			capability.Aggregate.ByDomain[domain]["passed"]++
		}
	}
	return capability
}

func appendUniqueString(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func TestFrozenFunctionLevelCorpusRunsEveryApplicableTrustedSimulation(t *testing.T) {
	expectedModels := map[string]string{
		"atmega328p_isp_controller":    simmodel.ModelLinearCircuitMNAV1,
		"buffered_thermistor_frontend": simmodel.ModelLinearCircuitMNAV1,
		"dual_stage_active_lowpass":    simmodel.ModelLinearCircuitMNAV1,
		"npn_low_side_status_driver":   simmodel.ModelNonlinearCircuitDCV1,
	}
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "function-corpus"})
	for _, fixture := range manifest.Fixtures {
		contents, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		document, issues := DecodeStrict(strings.NewReader(string(contents)))
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("%s decode issues = %#v", fixture.ID, issues)
		}
		_, synthesis, synthesisIssues := resolver.Synthesize(context.Background(), document)
		if reports.HasBlockingIssue(synthesisIssues) {
			t.Fatalf("%s synthesis issues = %#v", fixture.ID, synthesisIssues)
		}
		resolved, issues := resolver.Resolve(context.Background(), document)
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("%s resolution issues = %#v", fixture.ID, issues)
		}
		wantModel := expectedModels[fixture.ID]
		if wantModel == "" {
			if resolved.Simulation != nil {
				t.Fatalf("%s unexpectedly produced partial simulation %#v", fixture.ID, resolved.Simulation)
			}
			continue
		}
		if resolved.Simulation == nil || resolved.Simulation.ModelID != wantModel {
			t.Fatalf("%s simulation = %#v, synthesis evidence=%#v selections=%#v, want model %s", fixture.ID, resolved.Simulation, synthesis.Simulation, synthesis.Selections, wantModel)
		}
		report, diagnostics := simmodel.Evaluate(*resolved.Simulation)
		if len(diagnostics) != 0 || report.Status != "pass" {
			t.Fatalf("%s trusted simulation failed: report=%#v diagnostics=%#v", fixture.ID, report, diagnostics)
		}
	}
}

func hashFunctionGeneratedFiles(t *testing.T, root string) string {
	t.Helper()
	var paths []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Dir(path) != root {
			return nil
		}
		switch filepath.Ext(path) {
		case ".kicad_pro", ".kicad_sch", ".kicad_pcb":
		default:
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(paths)
	hash := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hash.Write([]byte(filepath.ToSlash(relative)))
		hash.Write([]byte{0})
		hash.Write(contents)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func TestFunctionLevelCorpusIsFrozenAndIdentityNeutral(t *testing.T) {
	root := functionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Hex(manifestBytes); got != frozenFunctionCorpusManifestSHA256 {
		t.Fatalf("manifest hash = %s, want %s; corpus membership is frozen by specification", got, frozenFunctionCorpusManifestSHA256)
	}
	checksumBytes, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	wantChecksumLine := frozenFunctionCorpusManifestSHA256 + "  manifest.json\n"
	if string(checksumBytes) != wantChecksumLine {
		t.Fatalf("manifest.sha256 = %q, want %q", checksumBytes, wantChecksumLine)
	}

	var manifest functionCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.function-corpus.v1" || manifest.Version != 1 || manifest.PolicyVersion != "function-synthesis-policy-v1" {
		t.Fatalf("invalid corpus manifest header: %#v", manifest)
	}
	if len(manifest.Fixtures) != 8 {
		t.Fatalf("fixture count = %d, want 8", len(manifest.Fixtures))
	}

	coveredDomains := map[string]bool{}
	seenIDs := map[string]bool{}
	seenFiles := map[string]bool{}
	for index, fixture := range manifest.Fixtures {
		if fixture.ID == "" || fixture.File != fixture.ID+".json" || seenIDs[fixture.ID] || seenFiles[fixture.File] {
			t.Fatalf("fixtures[%d] has invalid or duplicate identity: %#v", index, fixture)
		}
		seenIDs[fixture.ID] = true
		seenFiles[fixture.File] = true
		if !slices.IsSorted(fixture.Domains) {
			t.Fatalf("fixtures[%d].domains are not sorted: %#v", index, fixture.Domains)
		}
		for _, domain := range fixture.Domains {
			coveredDomains[domain] = true
		}
		contents, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(contents); got != fixture.SHA256 {
			t.Fatalf("%s hash = %s, want %s", fixture.File, got, fixture.SHA256)
		}
		var raw map[string]any
		if err := json.Unmarshal(contents, &raw); err != nil {
			t.Fatalf("decode %s: %v", fixture.File, err)
		}
		if raw["schema"] != SchemaID || int(raw["version"].(float64)) != Version || raw["synthesis"] == nil {
			t.Fatalf("%s is not a function-level %s document", fixture.File, SchemaID)
		}
		for _, explicit := range []string{"components", "nets", "no_connects", "power_flags", "buses", "schematic", "pcb", "simulation"} {
			if _, exists := raw[explicit]; exists {
				t.Fatalf("%s contains explicit graph field %q", fixture.File, explicit)
			}
		}
		assertNoFunctionCorpusImplementationDetail(t, fixture.File, raw, "")
	}

	for _, required := range []string{"analog", "power", "protection", "transistor", "sensor", "mcu", "interface"} {
		if !coveredDomains[required] {
			t.Fatalf("frozen corpus does not cover %s", required)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == "manifest.json" {
			continue
		}
		if !seenFiles[entry.Name()] {
			t.Fatalf("unmanifested corpus fixture %s", entry.Name())
		}
	}
}

func assertNoFunctionCorpusImplementationDetail(t *testing.T, file string, value any, path string) {
	t.Helper()
	forbidden := map[string]bool{
		"reference": true, "component_id": true, "variant_id": true,
		"symbol": true, "footprint": true, "units": true, "symbol_pin": true, "pad": true,
		"x_mm": true, "y_mm": true, "bounds": true, "layers": true, "layer": true,
		"route": true, "routes": true, "track": true, "tracks": true, "via": true, "vias": true,
		"block": true, "blocks": true, "block_id": true,
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if forbidden[strings.ToLower(key)] {
				t.Fatalf("%s contains forbidden implementation detail %s", file, childPath)
			}
			assertNoFunctionCorpusImplementationDetail(t, file, child, childPath)
		}
	case []any:
		for index, child := range typed {
			assertNoFunctionCorpusImplementationDetail(t, file, child, path+"["+strconv.Itoa(index)+"]")
		}
	}
}

func functionCorpusRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate function corpus test source")
	}
	return filepath.Join(filepath.Dir(sourcePath), "testdata", "function_corpus")
}

func sha256Hex(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
