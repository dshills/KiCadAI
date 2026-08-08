package opentopologysynthesis

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
)

const humanQualityPhysicalCaseEnv = "KICADAI_HUMAN_QUALITY_CASE"

// TestHumanQualityPhysicalCorpusOptionalKiCadPromotion is the milestone's
// public-policy qualification lane. The frozen requirements are deliberately
// loaded through their manifest references, synthesized with DefaultPolicy,
// and promoted through the same fail-closed behavior-to-KiCad path used by the
// exported API. No corpus-specific search policy participates in this proof.
func TestHumanQualityPhysicalCorpusOptionalKiCadPromotion(t *testing.T) {
	if os.Getenv(openTopologyKiCadPromotionEnv) != "1" {
		t.Skip("set " + openTopologyKiCadPromotionEnv + "=1 to run installed-KiCad human-quality promotion")
	}

	root := humanQualityPhysicalCorpusRoot()
	var manifest humanQualityPhysicalCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(root, "manifest.json")), &manifest)
	target := strings.TrimSpace(os.Getenv(humanQualityPhysicalCaseEnv))
	matched := false
	for _, entry := range manifest.DesignCases {
		if target != "" && target != entry.ID {
			continue
		}
		matched = true
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			testHumanQualityPhysicalOptionalKiCadPromotion(t, root, manifest.PhysicalContract, entry)
		})
	}
	if target != "" && !matched {
		t.Fatalf("%s=%q does not name a frozen human-quality case", humanQualityPhysicalCaseEnv, target)
	}
}

func testHumanQualityPhysicalOptionalKiCadPromotion(
	t *testing.T,
	corpusRoot string,
	contract humanQualityPhysicalContract,
	entry humanQualityPhysicalCase,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	requirementPath := filepath.Clean(filepath.Join(corpusRoot, entry.RequirementFile))
	requirementBytes := mustRead(t, requirementPath)
	if got := frozenHash(requirementBytes); got != entry.RequirementSHA256 {
		t.Fatalf("requirement sha256 = %s, want %s", got, entry.RequirementSHA256)
	}
	requirement, issues := DecodeStrict(bytes.NewReader(requirementBytes))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	first := Synthesize(ctx, requirement, inventory, environment, policy)
	t.Logf(
		"human-quality %s synthesis-1 status=%s stop=%s consumption=%+v candidates=%d",
		entry.ID, first.Report.Status, first.Report.StopReason, first.Report.Consumption, len(first.Candidates),
	)
	if first.Report.Status != StatusPassed {
		t.Logf("human-quality %s candidate metrics=%#v", entry.ID, humanQualityCandidateMetricSummaries(first))
		if os.Getenv("KICADAI_HUMAN_QUALITY_VERBOSE_DIAGNOSTICS") == "1" {
			t.Logf("human-quality %s candidate failures=%#v", entry.ID, humanQualityCandidateFailureSummaries(first))
		}
	}
	assertNonlinearSwitchingDesignPass(t, requirement, first)
	second := Synthesize(ctx, requirement, inventory, environment, policy)
	t.Logf(
		"human-quality %s synthesis-2 status=%s stop=%s consumption=%+v candidates=%d",
		entry.ID, second.Report.Status, second.Report.StopReason, second.Report.Consumption, len(second.Candidates),
	)
	assertNonlinearSwitchingDesignPass(t, requirement, second)
	assertNonlinearSwitchingReplay(t, first, second)
	assertSynthesisConsumptionMatchesEvidence(t, first)

	index, _ := libraryresolver.Load(
		ctx,
		libraryresolver.LibraryRoots{
			SymbolsRoot:    openTopologyLibraryRoot(t, libraryresolver.EnvSymbolsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols"),
			FootprintsRoot: openTopologyLibraryRoot(t, libraryresolver.EnvFootprintsRoot, "/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints"),
			TemplatesRoot:  strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
		},
		libraryresolver.LoadOptions{},
	)
	assertHumanQualityPhysicalIntent(t, contract, *first.Physical, &index)

	outputRoot := t.TempDir()
	if retained := strings.TrimSpace(os.Getenv("KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT")); retained != "" {
		outputRoot = filepath.Join(retained, entry.ID)
	}
	promotion := PromoteSynthesisRun(ctx, first, environment, PhysicalPromotionOptions{
		OutputRoot:    outputRoot,
		KiCadCLI:      openTopologyKiCadCLI(t),
		LibraryIndex:  &index,
		Timeout:       3 * time.Minute,
		KeepArtifacts: true,
	})
	assertHumanQualityPhysicalPromotion(t, contract, first.Physical.DesignRequest, promotion)
	t.Logf(
		"human-quality %s promotion policy=%s synthesis=%s physical=%s project=%s evidence=%s",
		entry.ID,
		first.Report.PolicyHash,
		first.Hash,
		first.Physical.Hash,
		promotion.ProjectHash,
		promotion.Hash,
	)
}

type humanQualityCandidateMetricSummary struct {
	Fingerprint string
	Instances   int
	Topology    []string
	FirstTrial  []string
	Actuals     []string
}

func humanQualityCandidateMetricSummaries(run SynthesisRun) []humanQualityCandidateMetricSummary {
	type metricRange struct {
		minimum float64
		maximum float64
		found   bool
	}
	result := make([]humanQualityCandidateMetricSummary, 0, len(run.Candidates))
	for _, candidate := range run.Candidates {
		ranges := map[string]metricRange{}
		addEvaluation := func(evaluation SimulationEvaluation) {
			for _, attempt := range evaluation.Attempts {
				if attempt.Actual == nil {
					continue
				}
				key := attempt.RequirementID + "/" + attempt.Metric
				current := ranges[key]
				if !current.found || *attempt.Actual < current.minimum {
					current.minimum = *attempt.Actual
				}
				if !current.found || *attempt.Actual > current.maximum {
					current.maximum = *attempt.Actual
				}
				current.found = true
				ranges[key] = current
			}
		}
		for _, evaluation := range candidate.Evaluations {
			addEvaluation(evaluation)
		}
		if candidate.Repair != nil {
			for _, attempt := range candidate.Repair.Attempts {
				addEvaluation(attempt.Evaluation)
			}
		}
		summary := humanQualityCandidateMetricSummary{Fingerprint: candidate.Fingerprint}
		for _, topology := range run.Search.Candidates {
			if topology.Fingerprint == candidate.Fingerprint {
				summary.Instances = len(topology.Graph.Instances)
				for _, instance := range topology.Graph.Instances {
					summary.Topology = append(summary.Topology, instance.ID+"="+instance.Kind)
				}
				break
			}
		}
		if enumeration := EnumerateValueTrials(candidate.ValuePlan, 1); len(enumeration.Trials) == 1 {
			for _, selection := range enumeration.Trials[0].Selections {
				value := "fixed"
				if selection.ValueSI != nil {
					value = strconv.FormatFloat(*selection.ValueSI, 'g', -1, 64)
				}
				summary.FirstTrial = append(summary.FirstTrial, selection.InstanceID+"="+selection.PrimitiveKey+"@"+value)
			}
		}
		for key, value := range ranges {
			summary.Actuals = append(summary.Actuals, key+"="+
				strconv.FormatFloat(value.minimum, 'g', 6, 64)+".."+
				strconv.FormatFloat(value.maximum, 'g', 6, 64))
		}
		slices.Sort(summary.Actuals)
		slices.Sort(summary.FirstTrial)
		slices.Sort(summary.Topology)
		result = append(result, summary)
	}
	return result
}

type humanQualityCandidateFailureSummary struct {
	Fingerprint      string
	Topology         []string
	FirstTrial       []string
	AttemptSamples   []string
	ValueStatus      ValuePlanStatus
	DomainCandidates []string
	DomainHeads      []string
	EvaluationCounts []string
	DiagnosisCounts  []string
	RepairStatus     RepairSearchStatus
	RepairAttempts   int
	RepairImproved   int
}

func humanQualityCandidateFailureSummaries(run SynthesisRun) []humanQualityCandidateFailureSummary {
	summaries := make([]humanQualityCandidateFailureSummary, 0, len(run.Candidates))
	fingerprintFilter := strings.TrimSpace(os.Getenv("KICADAI_HUMAN_QUALITY_DIAGNOSTIC_FINGERPRINT"))
	for _, candidate := range run.Candidates {
		if fingerprintFilter != "" && candidate.Fingerprint != fingerprintFilter {
			continue
		}
		evaluationCounts := map[string]int{}
		diagnosisCounts := map[string]int{}
		attemptSamples := []string{}
		sampledRequirements := map[string]bool{}
		addEvaluation := func(origin string, evaluation SimulationEvaluation) {
			evaluationCounts[string(evaluation.Status)]++
			for _, diagnosis := range evaluation.Diagnoses {
				diagnosisCounts[diagnosis.Code+"|"+diagnosis.RequirementID+"|"+diagnosis.Analysis+"|"+diagnosis.Metric+"|"+diagnosis.Message]++
			}
			for _, attempt := range evaluation.Attempts {
				if attempt.AssertionPass || attempt.Report == nil || sampledRequirements[attempt.RequirementID] || len(attemptSamples) >= 4 {
					continue
				}
				sampledRequirements[attempt.RequirementID] = true
				sample := origin + ":" + attempt.RequirementID + "=" + string(attempt.Status)
				if attempt.Actual != nil {
					sample += "@" + strconv.FormatFloat(*attempt.Actual, 'g', -1, 64)
				}
				if len(attempt.Report.Analyses) > 0 && len(attempt.Report.Analyses[0].Points) > 0 {
					point := attempt.Report.Analyses[0].Points[len(attempt.Report.Analyses[0].Points)-1]
					if point.Solver != nil {
						sample += "|solver:" + point.Solver.Method
					}
					for _, node := range point.Nodes {
						sample += "|n:" + node.Node + "=" + strconv.FormatFloat(node.Real, 'g', -1, 64)
					}
					for _, device := range point.Devices {
						sample += "|d:" + device.Component + "=" + strconv.FormatFloat(device.VoltageV, 'g', -1, 64) + "/" + strconv.FormatFloat(device.CurrentA, 'g', -1, 64)
					}
				}
				if spectrum := humanQualityDistortionSpectrum(attempt); spectrum != "" {
					sample += "|" + spectrum
				}
				attemptSamples = append(attemptSamples, sample)
			}
		}
		for index, evaluation := range candidate.Evaluations {
			addEvaluation("base#"+strconv.Itoa(index+1), evaluation)
		}
		summary := humanQualityCandidateFailureSummary{
			Fingerprint: candidate.Fingerprint, ValueStatus: candidate.ValuePlan.Status,
		}
		for _, topology := range run.Search.Candidates {
			if topology.Fingerprint != candidate.Fingerprint {
				continue
			}
			for _, instance := range topology.Graph.Instances {
				terminals := make([]string, 0, len(instance.Terminals))
				for _, terminal := range instance.Terminals {
					terminals = append(terminals, terminal.Terminal+"="+terminal.Node)
				}
				slices.Sort(terminals)
				summary.Topology = append(summary.Topology, instance.ID+":"+instance.Kind+":"+strings.Join(terminals, ","))
			}
			break
		}
		for _, domain := range candidate.ValuePlan.Domains {
			summary.DomainCandidates = append(summary.DomainCandidates, domain.InstanceID+"="+strconv.Itoa(len(domain.Candidates)))
			if len(domain.AnalyticScales) > 0 {
				for index, valueCandidate := range domain.Candidates {
					if index == 3 {
						break
					}
					value := "fixed"
					if valueCandidate.ValueSI != nil {
						value = strconv.FormatFloat(*valueCandidate.ValueSI, 'g', -1, 64)
					}
					summary.DomainHeads = append(summary.DomainHeads, domain.InstanceID+"#"+strconv.Itoa(index+1)+"="+valueCandidate.PrimitiveKey+"@"+value+"/"+strconv.FormatFloat(valueCandidate.RelativeError, 'g', -1, 64))
				}
			}
		}
		if enumeration := EnumerateValueTrials(candidate.ValuePlan, 1); len(enumeration.Trials) == 1 {
			for _, selection := range enumeration.Trials[0].Selections {
				value := "fixed"
				if selection.ValueSI != nil {
					value = strconv.FormatFloat(*selection.ValueSI, 'g', -1, 64)
				}
				summary.FirstTrial = append(summary.FirstTrial, selection.InstanceID+"="+selection.PrimitiveKey+"@"+value)
			}
		}
		if candidate.Repair != nil {
			summary.RepairStatus = candidate.Repair.Status
			summary.RepairAttempts = len(candidate.Repair.Attempts)
			for index, attempt := range candidate.Repair.Attempts {
				addEvaluation("repair#"+strconv.Itoa(index+1), attempt.Evaluation)
				if attempt.Improved {
					summary.RepairImproved++
				}
			}
		}
		summary.AttemptSamples = attemptSamples
		for status, count := range evaluationCounts {
			summary.EvaluationCounts = append(summary.EvaluationCounts, status+"="+strconv.Itoa(count))
		}
		for code, count := range diagnosisCounts {
			summary.DiagnosisCounts = append(summary.DiagnosisCounts, code+"="+strconv.Itoa(count))
		}
		slices.Sort(summary.DomainCandidates)
		slices.Sort(summary.DomainHeads)
		slices.Sort(summary.FirstTrial)
		slices.Sort(summary.Topology)
		slices.Sort(summary.EvaluationCounts)
		slices.Sort(summary.DiagnosisCounts)
		summaries = append(summaries, summary)
	}
	return summaries
}

func humanQualityDistortionSpectrum(attempt SimulationAttempt) string {
	if attempt.Report == nil || attempt.Analysis != "distortion" || len(attempt.Report.Analyses) == 0 {
		return ""
	}
	analysis := attempt.Report.Analyses[0]
	if analysis.FundamentalFrequencyHz <= 0 || len(analysis.Points) < 2 || analysis.Points[1].TimeS <= 0 {
		return ""
	}
	node := ""
	for _, assertion := range attempt.Report.Assertions {
		if assertion.AnalysisID == analysis.ID && assertion.Node != "" {
			node = assertion.Node
			break
		}
	}
	if node == "" {
		return ""
	}
	samplesPerCycle := int(math.Round(1 / (analysis.FundamentalFrequencyHz * analysis.Points[1].TimeS)))
	window := 2 * samplesPerCycle
	if samplesPerCycle <= 0 || len(analysis.Points)-1 < window {
		return ""
	}
	values := make([]float64, 0, window)
	for _, point := range analysis.Points[len(analysis.Points)-1-window : len(analysis.Points)-1] {
		found := false
		for _, value := range point.Nodes {
			if value.Node == node {
				values = append(values, value.Real)
				found = true
				break
			}
		}
		if !found {
			return ""
		}
	}
	dft := func(bin int) float64 {
		realPart, imaginary := 0.0, 0.0
		for index, value := range values {
			angle := 2 * math.Pi * float64(bin*index) / float64(len(values))
			realPart += value * math.Cos(angle)
			imaginary -= value * math.Sin(angle)
		}
		return 2 * math.Hypot(realPart, imaginary) / float64(len(values))
	}
	minimum, maximum, mean := values[0], values[0], 0.0
	for _, value := range values {
		minimum = math.Min(minimum, value)
		maximum = math.Max(maximum, value)
		mean += value
	}
	mean /= float64(len(values))
	parts := []string{
		"wave:min=" + strconv.FormatFloat(minimum, 'g', 6, 64),
		"max=" + strconv.FormatFloat(maximum, 'g', 6, 64),
		"mean=" + strconv.FormatFloat(mean, 'g', 6, 64),
	}
	for harmonic := 1; harmonic <= 5; harmonic++ {
		parts = append(parts, "h"+strconv.Itoa(harmonic)+"="+strconv.FormatFloat(dft(2*harmonic), 'g', 6, 64))
	}
	return strings.Join(parts, ",")
}

func assertHumanQualityPhysicalIntent(
	t *testing.T,
	contract humanQualityPhysicalContract,
	physical PhysicalLoweringResult,
	index *libraryresolver.LibraryIndex,
) {
	t.Helper()
	if physical.Status != PhysicalLoweringReady || physical.DesignRequest.ExplicitCircuit == nil {
		t.Fatalf("physical lowering status=%s explicit=%t issues=%#v", physical.Status, physical.DesignRequest.ExplicitCircuit != nil, physical.Issues)
	}
	document := physical.Document
	request := physical.DesignRequest
	if document.Schematic.Hierarchy.Mode != "auto" || !request.ExplicitCircuit.AutoHierarchy {
		t.Fatalf("hierarchy intent=%#v auto_hierarchy=%t", document.Schematic.Hierarchy, request.ExplicitCircuit.AutoHierarchy)
	}
	hierarchy, issues := schematicir.HierarchyForProject(request.ExplicitCircuit.Schematic, index)
	if len(issues) != 0 || hierarchy == nil {
		t.Fatalf("derived hierarchy=%#v issues=%#v", hierarchy, issues)
	}
	if got := len(hierarchy.Sheets); got < contract.MinimumSchematicSheets {
		t.Fatalf("functional child-sheet count=%d, want at least %d", got, contract.MinimumSchematicSheets)
	}
	if contract.RequireExplicitInterSheetInterfaces {
		assertHumanQualityInterSheetInterfaces(t, hierarchy)
	}

	if document.Project.Board.Layers != len(contract.CopperLayers) || request.Board.Layers != len(contract.CopperLayers) {
		t.Fatalf("board layers document/request=%d/%d, want %d", document.Project.Board.Layers, request.Board.Layers, len(contract.CopperLayers))
	}
	assertHumanQualityPlaneZones(t, contract, document.Nets, request.ExplicitCircuit.Zones)

	regionIDs := map[string]bool{}
	for _, region := range request.ExplicitCircuit.Regions {
		if region.ID != "" && region.Role != "" && region.WidthMM > 0 && region.HeightMM > 0 {
			regionIDs[region.ID] = true
		}
	}
	if len(regionIDs) < contract.MinimumFunctionalPlacementRegions {
		t.Fatalf("functional placement regions=%d, want at least %d: %#v", len(regionIDs), contract.MinimumFunctionalPlacementRegions, request.ExplicitCircuit.Regions)
	}
	if contract.RequireThermalPlacement {
		found := false
		for _, evidence := range physical.Placement {
			if evidence.Kind == "thermal_placement" && evidence.Component != "" && evidence.Region != "" &&
				evidence.ThermalPathID != "" && evidence.ThermalPathCPerW > 0 && evidence.EvidenceSHA != "" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("no complete thermal placement evidence: %#v", physical.Placement)
		}
	}
	if contract.RequireControlledReturnPaths {
		controlled := 0
		for _, net := range request.ExplicitCircuit.Nets {
			if net.ReturnNet != "" && net.ReturnPathMaxDistanceMM > 0 {
				controlled++
			}
		}
		if controlled == 0 {
			t.Fatal("four-layer design has no controlled return-path obligations")
		}
	}
}

func assertHumanQualityInterSheetInterfaces(t *testing.T, hierarchy *transactions.SchematicHierarchy) {
	t.Helper()
	if len(hierarchy.CrossSheetNets) == 0 {
		t.Fatal("hierarchical schematic has no explicit cross-sheet nets")
	}
	refSheet := map[string]string{}
	for _, sheet := range hierarchy.Sheets {
		if sheet.ID == "" || sheet.Name == "" || sheet.Filename == "" || len(sheet.References) == 0 {
			t.Fatalf("incomplete hierarchy sheet: %#v", sheet)
		}
		for _, ref := range sheet.References {
			if previous := refSheet[ref]; previous != "" && previous != sheet.ID {
				t.Fatalf("reference %s is owned by both %s and %s", ref, previous, sheet.ID)
			}
			refSheet[ref] = sheet.ID
		}
	}
	for _, net := range hierarchy.CrossSheetNets {
		sheets := map[string]bool{}
		for _, endpoint := range net.Endpoints {
			if sheet := refSheet[endpoint.Ref]; sheet != "" {
				sheets[sheet] = true
			}
		}
		if net.Name == "" || len(net.Endpoints) < 2 || len(sheets) < 2 {
			t.Fatalf("cross-sheet interface does not cross two child sheets: %#v owners=%#v", net, sheets)
		}
	}
}

func assertHumanQualityPlaneZones(
	t *testing.T,
	contract humanQualityPhysicalContract,
	nets []circuitgraph.Net,
	zones []designworkflow.ExplicitZoneSpec,
) {
	t.Helper()
	roles := map[string]circuitgraph.NetRole{}
	for _, net := range nets {
		roles[net.Name] = net.Role
	}
	foundGround := false
	foundPower := false
	for _, zone := range zones {
		if len(zone.Layers) != 1 {
			continue
		}
		switch {
		case zone.Layers[0] == contract.GroundReferenceLayer &&
			(roles[zone.Net] == circuitgraph.NetRoleGround || roles[zone.Net] == circuitgraph.NetRoleReturn):
			foundGround = true
		case zone.Layers[0] == contract.PowerDistributionLayer &&
			(roles[zone.Net] == circuitgraph.NetRolePower || roles[zone.Net] == circuitgraph.NetRolePowerPos):
			foundPower = true
		}
	}
	if !foundGround || !foundPower {
		t.Fatalf("plane zones ground=%t power=%t zones=%#v roles=%#v", foundGround, foundPower, zones, roles)
	}
}

func assertHumanQualityPhysicalPromotion(
	t *testing.T,
	contract humanQualityPhysicalContract,
	request designworkflow.Request,
	promotion PhysicalPromotionResult,
) {
	t.Helper()
	if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical ||
		promotion.ProjectHash == "" || len(promotion.Runs) != 2 || len(promotion.Issues) != 0 {
		t.Fatalf(
			"human-quality promotion status=%s replay=%t project=%s runs=%d issues=%#v stages=%#v routing=%#v",
			promotion.Status,
			promotion.ReplayIdentical,
			promotion.ProjectHash,
			len(promotion.Runs),
			promotion.Issues,
			promotionRunStages(promotion.Runs),
			nonlinearSwitchingRoutingSummary(promotion.Runs),
		)
	}
	var baselineStages []string
	for index, run := range promotion.Runs {
		if run.ProjectHash != promotion.ProjectHash ||
			(run.Workflow.Acceptance.Achieved != designworkflow.AcceptanceERCDRC &&
				run.Workflow.Acceptance.Achieved != designworkflow.AcceptanceFabricationCandidate) {
			t.Fatalf("run %d project=%s acceptance=%#v", index+1, run.ProjectHash, run.Workflow.Acceptance)
		}
		assertHumanQualityRequiredStages(t, index+1, run.Workflow.Stages)
		assertHumanQualityWrittenSheets(t, index+1, contract.MinimumSchematicSheets+1, run.ProjectRoot)
		stageEvidence := humanQualityDeterministicStageEvidence(run.Workflow.Stages)
		if index == 0 {
			baselineStages = stageEvidence
		} else if !reflect.DeepEqual(baselineStages, stageEvidence) {
			t.Fatalf("normalized workflow stage evidence differs across deterministic runs:\nfirst=%#v\nsecond=%#v", baselineStages, stageEvidence)
		}
	}
	if contract.RequireLayerTransitionEvidence || contract.RequireControlledReturnPaths {
		assertMultiStageReturnPathEvidence(t, request, promotion.Runs)
	}
}

// humanQualityDeterministicStageEvidence excludes run-root paths, KiCad
// temporary paths, command transcripts, and elapsed timings while preserving
// the semantic gate result: stage order/status plus issue and artifact kinds.
// Raw KiCad project identity is enforced by promotion.ReplayIdentical, and
// routing's numeric return-path evidence is compared independently below.
func humanQualityDeterministicStageEvidence(stages []designworkflow.StageResult) []string {
	evidence := make([]string, 0, len(stages)*2)
	for _, stage := range stages {
		evidence = append(evidence, "stage:"+string(stage.Name)+":"+string(stage.Status))
		stageItems := make([]string, 0, len(stage.Issues)+len(stage.Artifacts))
		for _, issue := range stage.Issues {
			stageItems = append(stageItems, "issue:"+string(issue.Code)+":"+string(issue.Severity)+":"+issue.Stage)
		}
		for _, artifact := range stage.Artifacts {
			stageItems = append(stageItems, "artifact:"+string(artifact.Kind))
		}
		slices.Sort(stageItems)
		evidence = append(evidence, stageItems...)
	}
	return evidence
}

func assertHumanQualityRequiredStages(t *testing.T, run int, stages []designworkflow.StageResult) {
	t.Helper()
	required := map[designworkflow.StageName]bool{
		designworkflow.StageRouting:       false,
		designworkflow.StageProjectWrite:  false,
		designworkflow.StageWriterCorrect: false,
		designworkflow.StageValidation:    false,
		designworkflow.StageKiCadChecks:   false,
	}
	for _, stage := range stages {
		if _, ok := required[stage.Name]; !ok {
			continue
		}
		// Stage status is the workflow's authoritative severity reduction. An OK
		// stage may retain informational provenance (for example, that structural
		// evaluation deferred ERC/DRC to the later installed-KiCad stage). Keep
		// those notices without weakening warning, error, or blocked failures.
		if stage.Status != designworkflow.StageStatusOK {
			t.Fatalf("run %d required stage %s status=%s issues=%#v", run, stage.Name, stage.Status, stage.Issues)
		}
		required[stage.Name] = true
	}
	for stage, found := range required {
		if !found {
			t.Fatalf("run %d did not reach required stage %s", run, stage)
		}
	}
}

func assertHumanQualityWrittenSheets(t *testing.T, run, minimum int, projectRoot string) {
	t.Helper()
	count := 0
	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".kicad_sch" {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run %d schematic artifact scan: %v", run, err)
	}
	if count < minimum {
		t.Fatalf("run %d wrote %d schematic sheets, want at least %d under %s", run, count, minimum, projectRoot)
	}
}
