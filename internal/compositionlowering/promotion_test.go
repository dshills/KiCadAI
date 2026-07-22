package compositionlowering

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
	"kicadai/internal/writercorrectness"
)

func TestFrozenOpenSetCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotion(t, "open_set_composition_corpus", 5, "KICADAI_OPEN_SET_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestNeutralMCUSynthesisCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "mcu_synthesis_corpus"), 3, "KICADAI_MCU_SYNTHESIS_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestNeutralMCUSynthesisCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed MCU synthesis corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "mcu_synthesis_corpus"), 3, "KICADAI_MCU_SYNTHESIS_ARTIFACT_DIR", cli, index)
}

func TestPowerInterfaceSynthesisCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "power_interface_synthesis_corpus"), 4, "KICADAI_POWER_INTERFACE_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestPowerInterfaceSynthesisCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed power/interface synthesis corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "power_interface_synthesis_corpus"), 4, "KICADAI_POWER_INTERFACE_ARTIFACT_DIR", cli, index)
}

func TestFrozenOpenSetCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed open-set corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotion(t, "open_set_composition_corpus", 5, "KICADAI_OPEN_SET_ARTIFACT_DIR", cli, index)
}

func TestFrozenAdversarialMultiFunctionCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotion(t, "adversarial_multi_function_composition_corpus", 10, "KICADAI_ADVERSARIAL_MULTI_FUNCTION_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestFrozenAdversarialMultiFunctionCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed adversarial multi-function corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotion(t, "adversarial_multi_function_composition_corpus", 10, "KICADAI_ADVERSARIAL_MULTI_FUNCTION_ARTIFACT_DIR", cli, index)
}

func TestFrozenSimulationGroundedCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "simulation_grounded_closed_loop_corpus"), 10, "KICADAI_SIMULATION_GROUNDED_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestFrozenSimulationGroundedCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed simulation-grounded corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	runFrozenPromotionAt(t, filepath.Join("..", "architecturesearch", "testdata", "simulation_grounded_closed_loop_corpus"), 10, "KICADAI_SIMULATION_GROUNDED_ARTIFACT_DIR", cli, index)
}

func TestFrozenBehavioralIntentHeldOutReadyCorpusPassesOfflineWorkflow(t *testing.T) {
	requireLongPromotionTest(t)
	corpusRoot, count := behavioralIntentHeldOutReadyCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_BEHAVIORAL_INTENT_ARTIFACT_DIR", "", libraryresolver.LibraryIndex{})
}

func TestFrozenBehavioralIntentHeldOutReadyCorpusOptionalKiCadPromotion(t *testing.T) {
	requireLongPromotionTest(t)
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_KICAD_CLI to run the KiCad-backed behavioral-intent corpus")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" {
		t.Skipf("installed KiCad libraries are required: %#v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library index is empty: %#v", loadIssues)
	}
	corpusRoot, count := behavioralIntentHeldOutReadyCorpus(t)
	runFrozenPromotionAt(t, corpusRoot, count, "KICADAI_BEHAVIORAL_INTENT_ARTIFACT_DIR", cli, index)
}

func behavioralIntentHeldOutReadyCorpus(t *testing.T) (string, int) {
	t.Helper()
	type manifestCase struct {
		Outcome         string `json:"outcome"`
		RequirementFile string `json:"requirement_file"`
	}
	var manifest struct {
		Cases []manifestCase `json:"cases"`
	}
	manifestPath := filepath.Join("..", "behavioralintent", "testdata", "held_out_corpus", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	unique := map[string]bool{}
	for _, fixture := range manifest.Cases {
		if fixture.Outcome == "ready" {
			unique[fixture.RequirementFile] = true
		}
	}
	paths := make([]string, 0, len(unique))
	for path := range unique {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	root := t.TempDir()
	for _, path := range paths {
		if strings.HasPrefix(path, "testdata/") {
			path = filepath.Join("..", "behavioralintent", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read held-out requirement %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.Base(path)), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, len(paths)
}

func requireLongPromotionTest(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping frozen end-to-end promotion corpus in short mode")
	}
}

func runFrozenPromotion(t *testing.T, corpusDir string, expectedCount int, artifactEnv string, cli string, installedIndex libraryresolver.LibraryIndex) {
	t.Helper()
	runFrozenPromotionAt(t, filepath.Join("..", "circuitgraph", "testdata", corpusDir), expectedCount, artifactEnv, cli, installedIndex)
}

func runFrozenPromotionAt(t *testing.T, corpusRoot string, expectedCount int, artifactEnv string, cli string, installedIndex libraryresolver.LibraryIndex) {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance diagnostics = %#v", provenanceDiagnostics)
	}
	modelRegistryHash, err := modelprovenance.Hash(provenance)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(corpusRoot, "*.json"))
	paths = slices.DeleteFunc(paths, func(path string) bool { return filepath.Base(path) == "manifest.json" })
	if err != nil || len(paths) != expectedCount {
		t.Fatalf("corpus paths = %#v, %v", paths, err)
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			if cli == "" {
				t.Parallel()
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
			if search.Status != architecturesearch.SearchSelected {
				t.Fatalf("search status = %s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			var request designworkflow.Request
			var resolved circuitgraph.ResolvedDocument
			if requirement.Version == architecturesearch.VersionV3 {
				promotion, promotionIssues := SynthesizeClosedLoop(context.Background(), requirement, search, ArchitectureSimulationPlanResolver{
					GraphResolver: resolver, ProvenanceRegistry: provenance,
				}, modelRegistryHash, nil, closedloopsynthesis.DefaultPolicy())
				if reports.HasBlockingIssue(promotionIssues) || promotion.Report.Status != "pass" {
					t.Fatalf("closed-loop promotion issues=%#v\n%s", promotionIssues, closedLoopFailureSummary(promotion.Report))
				}
				request = promotion.Request
				resolved = promotion.Resolved
				if request.ExplicitCircuit == nil || request.ExplicitCircuit.ClosedLoop == nil || request.ExplicitCircuit.ClosedLoop.SelectedCircuitHash != request.ExplicitCircuit.ResolutionHash {
					t.Fatalf("closed-loop request is not bound to selected resolved circuit: %#v", request.ExplicitCircuit)
				}
			} else {
				lowered, lowerIssues := Lower(requirement, search)
				if len(lowerIssues) != 0 {
					t.Fatalf("lower issues = %#v", lowerIssues)
				}
				var resolveIssues []reports.Issue
				resolved, resolveIssues = resolver.Resolve(context.Background(), lowered.Document)
				if reports.HasBlockingIssue(resolveIssues) {
					t.Fatalf("resolve issues = %#v", resolveIssues)
				}
				var requestIssues []reports.Issue
				request, requestIssues = circuitgraph.ToDesignRequest(resolved)
				if reports.HasBlockingIssue(requestIssues) {
					t.Fatalf("design request issues = %#v", requestIssues)
				}
			}

			index := installedIndex
			if cli == "" {
				index = openSetSyntheticLibraryIndex(resolved)
				request.Validation.RequireERC = false
				request.Validation.RequireDRC = false
			}
			artifactRoot := t.TempDir()
			if configured := os.Getenv(artifactEnv); configured != "" {
				artifactRoot = filepath.Join(configured, filepath.Base(path))
				if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			firstDir := filepath.Join(artifactRoot, "first")
			secondDir := filepath.Join(artifactRoot, "second")
			runOpenSetWorkflow(t, request, index, cli, firstDir)
			runOpenSetWorkflow(t, request, index, cli, secondDir)
			for _, suffix := range []string{".kicad_sch", ".kicad_pcb"} {
				firstBytes, err := os.ReadFile(filepath.Join(firstDir, request.Name+suffix))
				if err != nil {
					t.Fatal(err)
				}
				secondBytes, err := os.ReadFile(filepath.Join(secondDir, request.Name+suffix))
				if err != nil {
					t.Fatal(err)
				}
				if roundtrip.NormalizeBytes(firstBytes) != roundtrip.NormalizeBytes(secondBytes) {
					t.Fatalf("normalized %s differs across deterministic replay", suffix)
				}
			}
			project, err := design.ReadProjectDirectory(firstDir)
			if err != nil || project.Schematic == nil || project.PCB == nil || len(project.PCB.Tracks) == 0 {
				t.Fatalf("written project is incomplete: project=%#v err=%v", project, err)
			}
		})
	}
}

func closedLoopFailureSummary(report closedloopsynthesis.Report) string {
	var lines []string
	for _, candidate := range report.Candidates {
		if len(candidate.Attempts) == 0 {
			lines = append(lines, candidate.Fingerprint+" no attempts")
			continue
		}
		attempt := candidate.Attempts[0]
		for _, candidateAttempt := range candidate.Attempts {
			if sameClosedLoopState(candidateAttempt.State, candidate.FinalState) {
				attempt = candidateAttempt
			}
		}
		for _, repair := range candidate.Repairs {
			lines = append(lines, fmt.Sprintf("%s repair %d %s %.12g->%.12g for %s/%s", candidate.Fingerprint, repair.Number, repair.Variable, repair.From, repair.To, repair.Analysis, repair.Metric))
		}
		for _, candidateAttempt := range candidate.Attempts {
			if candidateAttempt.Status == "pass" {
				continue
			}
			values := make([]string, 0, len(candidateAttempt.State.Variables))
			for _, variable := range candidateAttempt.State.Variables {
				values = append(values, fmt.Sprintf("%s=%.6g", variable.ID, variable.Value))
			}
			failures := make([]string, 0)
			for _, assertion := range candidateAttempt.Assertions {
				if !assertion.Pass {
					failures = append(failures, fmt.Sprintf("%s=%.6g", assertion.Metric, assertion.Actual))
				}
			}
			lines = append(lines, fmt.Sprintf("%s attempt %d [%s] failed [%s]", candidate.Fingerprint, candidateAttempt.Number, strings.Join(values, ","), strings.Join(failures, ",")))
		}
		for _, variable := range attempt.State.Variables {
			lines = append(lines, fmt.Sprintf("%s final variable %s=%.12g", candidate.Fingerprint, variable.ID, variable.Value))
		}
		for _, decision := range attempt.ModelDecisions {
			lines = append(lines, fmt.Sprintf("%s model %s/%s %s parameters=%v", candidate.Fingerprint, decision.Component, decision.Family, decision.Claim.ModelID, decision.Claim.Parameters))
		}
		for _, diagnostic := range attempt.Diagnostics {
			lines = append(lines, fmt.Sprintf("%s %s: %s", candidate.Fingerprint, diagnostic.Path, diagnostic.Message))
		}
		for _, assertion := range attempt.Assertions {
			if !assertion.Pass {
				lines = append(lines, fmt.Sprintf("%s %s/%s %s actual=%.12g margin=%.12g", candidate.Fingerprint, assertion.RequirementID, assertion.OperatingCase, assertion.Metric, assertion.Actual, assertion.Margin))
				if assertion.Metric == "integrated_output_noise" && attempt.Simulation != nil {
					lines = append(lines, dominantNoiseSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if assertion.Metric == "bandwidth" && attempt.Simulation != nil {
					lines = append(lines, acSweepRangeSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if assertion.Metric == "transimpedance" && attempt.Simulation != nil {
					lines = append(lines, transimpedanceSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if assertion.Metric == "quiescent_current" && attempt.Simulation != nil {
					lines = append(lines, operatingPointSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase))
				}
				if (assertion.Metric == "threshold_voltage" || assertion.Metric == "threshold_current") && attempt.Simulation != nil {
					lines = append(lines, thresholdSweepSummary(*attempt.Simulation, assertion.RequirementID, assertion.OperatingCase, assertion.Actual))
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func acSweepRangeSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		assertionIndexes := append([]int(nil), link.Assertions...)
		if len(assertionIndexes) == 0 {
			assertionIndexes = append(assertionIndexes, link.Assertion)
		}
		for _, assertionIndex := range assertionIndexes {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.Kind != simmodel.AnalysisACSweep || len(analysis.Points) == 0 {
					continue
				}
				gains := make([]float64, 0, len(analysis.Points))
				for _, point := range analysis.Points {
					output, outputOK := testAnalysisNodeMagnitude(point, assertion.Node)
					reference, referenceOK := testAnalysisNodeMagnitude(point, assertion.ReferenceNode)
					if !outputOK || !referenceOK || reference <= 0 {
						return "AC sweep nodes are absent from the recorded evidence"
					}
					gains = append(gains, output/reference)
				}
				threshold := gains[0] / math.Sqrt2
				crossing := len(gains) - 1
				for index := 1; index < len(gains); index++ {
					if gains[index-1] >= threshold && gains[index] <= threshold {
						crossing = index
						break
					}
				}
				start, stop := max(0, crossing-2), min(len(gains), crossing+2)
				points := make([]string, 0, stop-start)
				for index := start; index < stop; index++ {
					points = append(points, fmt.Sprintf("%.6gHz=%.6g", analysis.Points[index].FrequencyHz, gains[index]))
				}
				return fmt.Sprintf("AC sweep range %.12g..%.12g Hz (%d points), passband %.6g threshold %.6g crossing [%s]", analysis.Points[0].FrequencyHz, analysis.Points[len(analysis.Points)-1].FrequencyHz, len(analysis.Points), gains[0], threshold, strings.Join(points, ", "))
			}
		}
	}
	return "AC sweep evidence unavailable"
}

func testAnalysisNodeMagnitude(point simmodel.AnalysisPoint, node string) (float64, bool) {
	for _, result := range point.Nodes {
		if result.Node == node {
			return result.Magnitude, true
		}
	}
	return 0, false
}

func sameClosedLoopState(left, right closedloopsynthesis.CandidateState) bool {
	if left.Fingerprint != right.Fingerprint || len(left.Variables) != len(right.Variables) {
		return false
	}
	for index := range left.Variables {
		if left.Variables[index].ID != right.Variables[index].ID || left.Variables[index].Value != right.Variables[index].Value {
			return false
		}
	}
	return true
}

func operatingPointSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		for _, analysis := range evidence.Reports[link.Plan].Analyses {
			if len(analysis.Points) == 0 {
				continue
			}
			point := analysis.Points[len(analysis.Points)-1]
			nodes := make([]string, 0, len(point.Nodes))
			for _, node := range point.Nodes {
				nodes = append(nodes, fmt.Sprintf("%s=%.9g", node.Node, node.Real))
			}
			devices := make([]string, 0, len(point.Devices))
			for _, device := range point.Devices {
				devices = append(devices, fmt.Sprintf("%s:I=%.9g,V=%.9g", device.Component, device.CurrentMagnitudeA, device.VoltageV))
			}
			slices.Sort(nodes)
			slices.Sort(devices)
			return "operating point nodes=[" + strings.Join(nodes, ",") + "] devices=[" + strings.Join(devices, ",") + "]"
		}
	}
	return "operating point evidence unavailable"
}

func transimpedanceSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		indices := append([]int(nil), link.Assertions...)
		if len(indices) == 0 {
			indices = []int{link.Assertion}
		}
		var samples []string
		for _, assertionIndex := range indices {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			reported := math.NaN()
			if assertionIndex < len(evidence.Reports[link.Plan].Assertions) {
				reported = evidence.Reports[link.Plan].Assertions[assertionIndex].Actual
			}
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.ID != assertion.AnalysisID {
					continue
				}
				minimum, maximum := math.Inf(1), math.Inf(-1)
				minimumAt := 0.0
				for _, point := range analysis.Points {
					voltage, current := math.NaN(), math.NaN()
					for _, node := range point.Nodes {
						if node.Node == assertion.Node {
							voltage = node.Real
						}
					}
					for _, device := range point.Devices {
						if device.Component == assertion.Component {
							current = device.CurrentMagnitudeA
						}
					}
					if current > 1e-15 {
						ratio := voltage / current
						if ratio < minimum {
							minimum, minimumAt = ratio, point.SweepValue
						}
						maximum = math.Max(maximum, ratio)
					}
				}
				samples = append(samples, fmt.Sprintf("%s:reported=%.9g,Zmin=%.9g@%.9g,Zmax=%.9g", analysis.ID, reported, minimum, minimumAt, maximum))
			}
		}
		slices.Sort(samples)
		return "transimpedance samples=[" + strings.Join(samples, "; ") + "]"
	}
	return "transimpedance evidence unavailable"
}

func thresholdSweepSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string, actual float64) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		indices := append([]int(nil), link.Assertions...)
		if len(indices) == 0 {
			indices = []int{link.Assertion}
		}
		for _, assertionIndex := range indices {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.ID != assertion.AnalysisID {
					continue
				}
				bestDistance, best := math.Inf(1), simmodel.AnalysisPoint{}
				maximumSweep := simmodel.AnalysisPoint{SweepValue: math.Inf(-1)}
				for _, point := range analysis.Points {
					if distance := math.Abs(point.SweepValue - actual); distance < bestDistance {
						bestDistance, best = distance, point
					}
					if point.Sweep == "forward" && point.SweepValue > maximumSweep.SweepValue {
						maximumSweep = point
					}
				}
				nodes := make([]string, 0, len(best.Nodes))
				for _, node := range best.Nodes {
					nodes = append(nodes, fmt.Sprintf("%s=%.9g", node.Node, node.Real))
				}
				slices.Sort(nodes)
				maximumNodes := make([]string, 0, len(maximumSweep.Nodes))
				for _, node := range maximumSweep.Nodes {
					maximumNodes = append(maximumNodes, fmt.Sprintf("%s=%.9g", node.Node, node.Real))
				}
				slices.Sort(maximumNodes)
				source := "unknown"
				for _, plannedAnalysis := range plans[link.Plan].Analyses {
					if plannedAnalysis.ID == assertion.AnalysisID && plannedAnalysis.DCSweep != nil {
						source = plannedAnalysis.DCSweep.Component
						break
					}
				}
				return fmt.Sprintf("threshold sweep source=%s sample=%.12g nodes=[%s] max_sample=%.12g max_nodes=[%s] neighborhood=[%s]", source, best.SweepValue, strings.Join(nodes, ", "), maximumSweep.SweepValue, strings.Join(maximumNodes, ", "), thresholdSourceNeighborhood(plans[link.Plan], source))
			}
		}
	}
	return "threshold sweep evidence unavailable"
}

func thresholdSourceNeighborhood(plan simmodel.Plan, source string) string {
	nets := map[string]bool{}
	for _, device := range plan.Devices {
		if device.Component != source {
			continue
		}
		for _, terminal := range device.Terminals {
			nets[terminal.Net] = true
		}
	}
	var devices []string
	for _, device := range plan.Devices {
		var terminals []string
		for _, terminal := range device.Terminals {
			if nets[terminal.Net] {
				terminals = append(terminals, terminal.Terminal+"="+terminal.Net)
			}
		}
		if len(terminals) == 0 {
			continue
		}
		slices.Sort(terminals)
		devices = append(devices, fmt.Sprintf("%s:%s{%s}", device.Component, device.PrimitiveModel, strings.Join(terminals, ",")))
	}
	slices.Sort(devices)
	return strings.Join(devices, "; ")
}

func dominantNoiseSummary(evidence closedloopsynthesis.SimulationEvidence, requirementID, operatingCase string) string {
	plans := evidence.Resolution.Plans
	if len(plans) == 0 && evidence.Resolution.Plan.ModelID != "" {
		plans = []simmodel.Plan{evidence.Resolution.Plan}
	}
	for _, link := range evidence.Resolution.Measurements {
		if link.RequirementID != requirementID || link.OperatingCase != operatingCase || link.Plan < 0 || link.Plan >= len(plans) || link.Plan >= len(evidence.Reports) {
			continue
		}
		indices := append([]int(nil), link.Assertions...)
		if len(indices) == 0 {
			indices = []int{link.Assertion}
		}
		for _, assertionIndex := range indices {
			if assertionIndex < 0 || assertionIndex >= len(plans[link.Plan].Assertions) {
				continue
			}
			assertion := plans[link.Plan].Assertions[assertionIndex]
			for _, analysis := range evidence.Reports[link.Plan].Analyses {
				if analysis.ID != assertion.AnalysisID {
					continue
				}
				maximum, source, frequency := 0.0, "", 0.0
				for _, point := range analysis.Points {
					for _, node := range point.Nodes {
						if node.Node == assertion.Node && node.DominantNoiseDensityVSqrtHz > maximum {
							maximum, source, frequency = node.DominantNoiseDensityVSqrtHz, node.DominantNoiseSource, point.FrequencyHz
						}
					}
				}
				return fmt.Sprintf("noise target=%s dominant=%s density=%.12g V/sqrt(Hz) at %.12g Hz", assertion.Node, source, maximum, frequency)
			}
		}
	}
	return "noise contribution evidence unavailable"
}

func runOpenSetWorkflow(t *testing.T, request designworkflow.Request, index libraryresolver.LibraryIndex, cli string, output string) designworkflow.WorkflowResult {
	t.Helper()
	opts := designworkflow.CreateOptions{
		OutputDir: output, Overwrite: true, LibraryIndex: &index,
		Writer: writercorrectness.Options{LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true},
	}
	if cli != "" {
		opts.Validation = designworkflow.ValidationOptions{StrictUnrouted: true, RequireDRC: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "validation")}
		opts.KiCadChecks = designworkflow.KiCadCheckOptions{KiCadCLI: cli, RequireERC: true, RequireDRC: true, EnforceRequirements: true, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "checks")}
		opts.Writer = writercorrectness.Options{RequireKiCadRoundTrip: true, StrictDiffs: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "roundtrip"), LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true}
	}
	result := designworkflow.Create(context.Background(), request, opts)
	if os.Getenv("KICADAI_ROUTE_DIAGNOSTICS") != "" {
		t.Logf("routing diagnostics: %#v", openSetWorkflowStage(result, designworkflow.StageRouting))
	}
	for _, stageName := range []designworkflow.StageName{designworkflow.StageSchematic, designworkflow.StageSchematicElectrical, designworkflow.StagePlacement, designworkflow.StageRouting, designworkflow.StageProjectWrite, designworkflow.StageWriterCorrect, designworkflow.StageValidation} {
		stage := openSetWorkflowStage(result, stageName)
		if stage == nil || stage.Status == designworkflow.StageStatusBlocked || stage.Status == designworkflow.StageStatusSkipped {
			t.Fatalf("%s stage = %#v; workflow issues = %#v", stageName, stage, designworkflow.WorkflowIssues(result))
		}
		if cli != "" && stage.Status != designworkflow.StageStatusOK {
			t.Fatalf("%s stage = %#v, want clean KiCad-backed status; workflow issues = %#v", stageName, stage, designworkflow.WorkflowIssues(result))
		}
	}
	if cli != "" {
		stage := openSetWorkflowStage(result, designworkflow.StageKiCadChecks)
		if stage == nil || stage.Status != designworkflow.StageStatusOK {
			t.Fatalf("KiCad stage = %#v; workflow issues = %#v", stage, designworkflow.WorkflowIssues(result))
		}
	}
	return result
}

func openSetWorkflowStage(result designworkflow.WorkflowResult, name designworkflow.StageName) *designworkflow.StageResult {
	for index := range result.Stages {
		if result.Stages[index].Name == name {
			return &result.Stages[index]
		}
	}
	return nil
}

func openSetSyntheticLibraryIndex(resolved circuitgraph.ResolvedDocument) libraryresolver.LibraryIndex {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{}, Footprints: map[string]libraryresolver.FootprintRecord{}}
	type pinKey struct {
		symbol string
		unit   int
		pin    string
	}
	seenPins := map[pinKey]bool{}
	seenPads := map[string]map[string]bool{}
	for _, component := range resolved.Components {
		for pinIndex, function := range component.Functions {
			key := pinKey{symbol: function.SymbolID, unit: function.Unit, pin: function.SymbolPin}
			if !seenPins[key] {
				record := index.Symbols[function.SymbolID]
				record.LibraryID = function.SymbolID
				record.Pins = append(record.Pins, libraryresolver.SymbolPin{Number: function.SymbolPin, Name: function.Function, Unit: function.Unit, Position: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(float64(pinIndex) * 2.54)}, Orientation: "0"})
				index.Symbols[function.SymbolID] = record
				seenPins[key] = true
			}
			if seenPads[component.FootprintID] == nil {
				seenPads[component.FootprintID] = map[string]bool{}
			}
			if !seenPads[component.FootprintID][function.Pad] {
				record := index.Footprints[component.FootprintID]
				record.FootprintID = component.FootprintID
				record.Pads = append(record.Pads, libraryresolver.FootprintPad{Name: function.Pad})
				index.Footprints[component.FootprintID] = record
				seenPads[component.FootprintID][function.Pad] = true
			}
		}
	}
	return index
}
