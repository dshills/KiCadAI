package opentopologysynthesis

import (
	"bytes"
	"context"
	"fmt"
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
	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
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
	hierarchy := assertHumanQualityPhysicalIntent(t, contract, *first.Physical, &index)

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
	assertHumanQualityPhysicalPromotion(t, contract, first.Physical.DesignRequest, hierarchy, &index, promotion)
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
) *transactions.SchematicHierarchy {
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
		assertHumanQualityInterSheetInterfaces(t, hierarchy, request.ExplicitCircuit.Schematic.Circuit.Nets)
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
	assignedRegions := map[string]bool{}
	for _, component := range request.ExplicitCircuit.Components {
		region := strings.TrimSpace(component.Placement.Region)
		if region == "" || !regionIDs[region] {
			t.Fatalf("component %s has no valid functional placement region: %#v", component.Reference, component.Placement)
		}
		assignedRegions[region] = true
	}
	if len(assignedRegions) < contract.MinimumFunctionalPlacementRegions {
		t.Fatalf("used functional placement regions=%d, want at least %d: %#v", len(assignedRegions), contract.MinimumFunctionalPlacementRegions, assignedRegions)
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
	return hierarchy
}

func assertHumanQualityInterSheetInterfaces(t *testing.T, hierarchy *transactions.SchematicHierarchy, nets []schematicir.Net) {
	t.Helper()
	if len(hierarchy.CrossSheetNets) == 0 {
		t.Fatal("hierarchical schematic has no explicit cross-sheet nets")
	}
	refSheet := map[string]string{}
	for _, sheet := range hierarchy.Sheets {
		if sheet.ID == "" || sheet.Name == "" || sheet.Filename == "" || len(sheet.References) == 0 {
			t.Fatalf("incomplete hierarchy sheet: %#v", sheet)
		}
		if strings.HasPrefix(strings.TrimSpace(sheet.Name), "Sheet ") {
			t.Fatalf("hierarchy sheet %s has non-functional coordinate name %q", sheet.ID, sheet.Name)
		}
		for _, ref := range sheet.References {
			if previous := refSheet[ref]; previous != "" && previous != sheet.ID {
				t.Fatalf("reference %s is owned by both %s and %s", ref, previous, sheet.ID)
			}
			refSheet[ref] = sheet.ID
		}
	}
	netRoles := make(map[string]schematicir.NetRole, len(nets))
	for _, net := range nets {
		netRoles[net.Name] = net.Role
	}
	global, hierarchical := 0, 0
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
		wantGlobal := humanQualityGlobalNetRole(netRoles[net.Name])
		if net.GlobalScope != wantGlobal {
			t.Fatalf("cross-sheet interface %s global_scope=%t, want %t for role %s", net.Name, net.GlobalScope, wantGlobal, netRoles[net.Name])
		}
		if net.GlobalScope {
			global++
		} else {
			hierarchical++
		}
	}
	if global == 0 || hierarchical == 0 {
		t.Fatalf("hierarchy scope coverage global=%d hierarchical=%d, want both supply/reference and explicit signal interfaces", global, hierarchical)
	}
}

func humanQualityGlobalNetRole(role schematicir.NetRole) bool {
	switch role {
	case schematicir.NetRolePower, schematicir.NetRolePowerPos, schematicir.NetRolePowerNeg,
		schematicir.NetRoleGround, schematicir.NetRoleReturn, schematicir.NetRoleShield:
		return true
	default:
		return false
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
	hierarchy *transactions.SchematicHierarchy,
	libraryIndex *libraryresolver.LibraryIndex,
	promotion PhysicalPromotionResult,
) {
	t.Helper()
	if promotion.Status != PhysicalPromotionPassed || !promotion.ReplayIdentical ||
		promotion.ProjectHash == "" || len(promotion.Runs) != 2 || len(promotion.Issues) != 0 {
		if os.Getenv("KICADAI_HUMAN_QUALITY_VERBOSE_DIAGNOSTICS") == "1" && libraryIndex != nil {
			placed := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{LibraryIndex: libraryIndex})
			refs, nets := map[string]bool{}, map[string]bool{}
			for _, issue := range promotion.Issues {
				for _, ref := range issue.Refs {
					refs[ref] = true
				}
				for _, net := range issue.Nets {
					nets[net] = true
				}
			}
			var components []placement.Component
			for _, component := range placed.Request.Components {
				if refs[component.Ref] {
					components = append(components, component)
				}
			}
			var placements []placement.PlacementResult
			for _, candidate := range placed.Result.Placements {
				if refs[candidate.Ref] {
					placements = append(placements, candidate)
				}
			}
			var relatedNets []placement.Net
			for _, net := range placed.Request.Nets {
				if nets[net.Name] {
					relatedNets = append(relatedNets, net)
				}
			}
			t.Logf("human-quality failed-promotion placement components=%#v placements=%#v nets=%#v", components, placements, relatedNets)
		}
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
		assertHumanQualityWrittenSheets(t, index+1, contract.MinimumSchematicSheets+1, run.ProjectRoot, hierarchy, request.ExplicitCircuit.Schematic)
		assertHumanQualityWrittenBoard(t, index+1, contract, request, run.ProjectRoot)
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

func assertHumanQualityWrittenBoard(
	t *testing.T,
	run int,
	contract humanQualityPhysicalContract,
	request designworkflow.Request,
	projectRoot string,
) {
	t.Helper()
	project, err := kicaddesign.ReadProjectDirectory(projectRoot)
	if err != nil || project.PCB == nil {
		t.Fatalf("run %d read written PCB: pcb=%t err=%v", run, project.PCB != nil, err)
	}
	board := project.PCB
	copper := make([]string, 0, len(contract.CopperLayers))
	for _, layer := range board.Layers {
		if strings.HasSuffix(string(layer.Name), ".Cu") {
			copper = append(copper, string(layer.Name))
		}
	}
	wantCopper := append([]string(nil), contract.CopperLayers...)
	slices.Sort(copper)
	slices.Sort(wantCopper)
	if !slices.Equal(copper, wantCopper) {
		t.Fatalf("run %d written copper layers=%#v, want %#v", run, copper, wantCopper)
	}
	if !board.Setup.HasStackup {
		t.Fatalf("run %d written PCB has no fabrication stackup", run)
	}
	stackupCopper := map[string]bool{}
	dielectrics := 0
	for _, layer := range board.Setup.Stackup.Layers {
		if strings.HasSuffix(layer.Name, ".Cu") && layer.Thickness > 0 {
			stackupCopper[layer.Name] = true
		}
		if strings.HasPrefix(strings.ToLower(layer.Name), "dielectric ") && layer.Thickness > 0 && strings.TrimSpace(layer.Material) != "" && layer.EpsilonR > 0 {
			dielectrics++
		}
	}
	for _, layer := range contract.CopperLayers {
		if !stackupCopper[layer] {
			t.Fatalf("run %d stackup has no physical copper row for %s", run, layer)
		}
	}
	if dielectrics < len(contract.CopperLayers)-1 {
		t.Fatalf("run %d stackup dielectric rows=%d, want at least %d", run, dielectrics, len(contract.CopperLayers)-1)
	}

	for _, required := range request.ExplicitCircuit.Zones {
		for _, layer := range required.Layers {
			found, filled := false, false
			for _, zone := range board.Zones {
				if zone.NetName != required.Net || !slices.Contains(zone.Layers, kicadfiles.BoardLayer(layer)) {
					continue
				}
				found = true
				for _, polygon := range zone.FilledPolygons {
					if polygon.Layer == kicadfiles.BoardLayer(layer) && len(polygon.Points) >= 3 {
						filled = true
						break
					}
				}
			}
			if !found || !filled {
				t.Fatalf("run %d zone %s on %s written=%t filled=%t", run, required.Net, layer, found, filled)
			}
		}
	}

	regions := make(map[string]designworkflow.ExplicitRegionSpec, len(request.ExplicitCircuit.Regions))
	for _, region := range request.ExplicitCircuit.Regions {
		regions[region.ID] = region
	}
	footprints := make(map[string]kicadfiles.Point, len(board.Footprints))
	for _, footprint := range board.Footprints {
		footprints[strings.ToUpper(footprint.Reference)] = footprint.Position
	}
	usedRegions := map[string]bool{}
	for _, component := range request.ExplicitCircuit.Components {
		position, ok := footprints[strings.ToUpper(component.Reference)]
		region, regionOK := regions[component.Placement.Region]
		if !ok || !regionOK {
			t.Fatalf("run %d component %s footprint=%t region=%t", run, component.Reference, ok, regionOK)
		}
		if component.Placement.ThermalEdgeRequired {
			if strings.TrimSpace(component.Placement.Edge) == "" {
				t.Fatalf("run %d thermal-edge component %s has no derived board edge", run, component.Reference)
			}
			// The hard catalog-backed thermal rule intentionally supersedes region
			// containment so a power device can reach its required board edge.
			continue
		}
		x := float64(position.X) / float64(kicadfiles.MM(1))
		y := float64(position.Y) / float64(kicadfiles.MM(1))
		if x < region.XMM || x > region.XMM+region.WidthMM || y < region.YMM || y > region.YMM+region.HeightMM {
			t.Fatalf("run %d component %s at %.3f,%.3f mm is outside region %s %#v", run, component.Reference, x, y, region.ID, region)
		}
		usedRegions[region.ID] = true
	}
	if len(usedRegions) < contract.MinimumFunctionalPlacementRegions {
		t.Fatalf("run %d written PCB uses %d functional regions, want at least %d", run, len(usedRegions), contract.MinimumFunctionalPlacementRegions)
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

func assertHumanQualityWrittenSheets(
	t *testing.T,
	run, minimum int,
	projectRoot string,
	expected *transactions.SchematicHierarchy,
	document schematicir.Document,
) {
	t.Helper()
	project, err := kicaddesign.ReadProjectDirectory(projectRoot)
	if err != nil {
		t.Fatalf("run %d read written project: %v", run, err)
	}
	if project.Schematic == nil {
		t.Fatalf("run %d written project has no root schematic", run)
	}
	count := 1 + len(project.SheetFiles)
	if count < minimum {
		t.Fatalf("run %d wrote %d schematic sheets, want at least %d under %s", run, count, minimum, projectRoot)
	}
	if expected == nil {
		t.Fatalf("run %d has no expected hierarchy", run)
	}

	rootSheetByFilename := make(map[string]schematic.Sheet, len(project.Schematic.Sheets))
	for _, sheet := range project.Schematic.Sheets {
		filename := filepath.ToSlash(filepath.Clean(sheet.Filename))
		rootSheetByFilename[filename] = sheet
		if strings.HasPrefix(strings.TrimSpace(sheet.Name), "Sheet ") {
			t.Fatalf("run %d root sheet %s retained a coordinate-style name %q", run, filename, sheet.Name)
		}
	}
	childByFilename := make(map[string]*schematic.SchematicFile, len(project.SheetFiles))
	for _, child := range project.SheetFiles {
		if child != nil {
			childByFilename[filepath.ToSlash(filepath.Clean(child.Filename))] = child
		}
	}
	expectedSheetByID := make(map[string]transactions.SchematicHierarchySheet, len(expected.Sheets))
	refUnitSheet := map[string]string{}
	refSheet := map[string]string{}
	for _, sheet := range expected.Sheets {
		expectedSheetByID[sheet.ID] = sheet
		filename := filepath.ToSlash(filepath.Clean(sheet.Filename))
		written, ok := rootSheetByFilename[filename]
		if !ok || written.Name != sheet.Name {
			t.Fatalf("run %d functional sheet %s = %#v, want name %q", run, filename, written, sheet.Name)
		}
		if childByFilename[filename] == nil {
			t.Fatalf("run %d functional sheet %s has no written child", run, filename)
		}
		for _, symbol := range sheet.Symbols {
			unit := symbol.Unit
			if unit <= 0 {
				unit = 1
			}
			refUnitSheet[strings.ToUpper(symbol.Ref)+"#"+strconv.Itoa(unit)] = sheet.ID
			if previous, exists := refSheet[strings.ToUpper(symbol.Ref)]; !exists || previous == sheet.ID {
				refSheet[strings.ToUpper(symbol.Ref)] = sheet.ID
			} else {
				refSheet[strings.ToUpper(symbol.Ref)] = ""
			}
		}
		if len(sheet.Symbols) == 0 {
			for _, ref := range sheet.References {
				refUnitSheet[strings.ToUpper(ref)+"#1"] = sheet.ID
				refSheet[strings.ToUpper(ref)] = sheet.ID
			}
		}
	}
	assertHumanQualityWrittenSignalFlow(t, run, document, expected, childByFilename)

	for _, net := range expected.CrossSheetNets {
		participating := map[string]bool{}
		for _, endpoint := range net.Endpoints {
			unit := endpoint.Unit
			if unit <= 0 {
				unit = 1
			}
			sheetID := refUnitSheet[strings.ToUpper(endpoint.Ref)+"#"+strconv.Itoa(unit)]
			if sheetID == "" {
				sheetID = refSheet[strings.ToUpper(endpoint.Ref)]
			}
			if sheetID != "" {
				participating[sheetID] = true
			}
		}
		if len(participating) < 2 {
			t.Fatalf("run %d written interface %s has expected owners %#v", run, net.Name, participating)
		}
		for sheetID := range participating {
			spec := expectedSheetByID[sheetID]
			filename := filepath.ToSlash(filepath.Clean(spec.Filename))
			rootSheet := rootSheetByFilename[filename]
			child := childByFilename[filename]
			if net.GlobalScope {
				if hierarchySheetHasPin(rootSheet, net.Name) {
					t.Fatalf("run %d global net %s was redundantly exposed as a pin on %s", run, net.Name, filename)
				}
				if !hierarchyChildHasLabel(child, net.Name, schematic.LabelGlobal) || hierarchyChildHasLabel(child, net.Name, schematic.LabelHierarchical) {
					t.Fatalf("run %d global net %s scope is wrong in %s", run, net.Name, filename)
				}
				continue
			}
			if !hierarchySheetHasPin(rootSheet, net.Name) {
				t.Fatalf("run %d signal net %s has no parent pin on %s", run, net.Name, filename)
			}
			if !hierarchyRootPinConnected(project.Schematic, rootSheet, net.Name) {
				t.Fatalf("run %d signal net %s parent pin is not connected on %s", run, net.Name, filename)
			}
			if !hierarchyChildHasLabel(child, net.Name, schematic.LabelHierarchical) || hierarchyChildHasLabel(child, net.Name, schematic.LabelGlobal) {
				t.Fatalf("run %d signal net %s scope is wrong in %s", run, net.Name, filename)
			}
		}
	}
	assertHumanQualityNormalizedWriterDiffs(t, run, projectRoot)
}

func assertHumanQualityWrittenSignalFlow(
	t *testing.T,
	run int,
	document schematicir.Document,
	expected *transactions.SchematicHierarchy,
	children map[string]*schematic.SchematicFile,
) {
	t.Helper()
	components := make(map[string]schematicir.Component, len(document.Circuit.Components))
	for _, component := range document.Circuit.Components {
		components[component.ID] = component
	}
	for _, sheet := range expected.Sheets {
		filename := filepath.ToSlash(filepath.Clean(sheet.Filename))
		child := children[filename]
		if child == nil {
			continue
		}
		positionByRef := map[string]kicadfiles.IU{}
		positionCountByRef := map[string]int{}
		for _, symbol := range child.Symbols {
			key := strings.ToUpper(symbol.Reference)
			positionByRef[key] += symbol.Position.X
			positionCountByRef[key]++
		}
		type stagePosition struct {
			rank int
			name string
			x    kicadfiles.IU
		}
		stages := make([]stagePosition, 0)
		for _, group := range document.Layout.Groups {
			if group.Side == schematicir.SideTop || group.Side == schematicir.SideBottom || group.Role == schematicir.GroupRoleDecouplingStage {
				continue
			}
			total, count := kicadfiles.IU(0), 0
			for _, componentID := range group.Members {
				component, ok := components[componentID]
				if !ok {
					continue
				}
				key := strings.ToUpper(component.Ref)
				if positionCountByRef[key] == 0 {
					continue
				}
				total += positionByRef[key] / kicadfiles.IU(positionCountByRef[key])
				count++
			}
			if count == 0 {
				continue
			}
			name := strings.TrimSpace(group.Label)
			if name == "" {
				name = group.ID
			}
			stages = append(stages, stagePosition{rank: group.Rank, name: name, x: total / kicadfiles.IU(count)})
		}
		slices.SortFunc(stages, func(left, right stagePosition) int {
			if left.rank != right.rank {
				return left.rank - right.rank
			}
			return strings.Compare(left.name, right.name)
		})
		for left := 0; left < len(stages); left++ {
			for right := left + 1; right < len(stages); right++ {
				if stages[left].rank == stages[right].rank {
					continue
				}
				if stages[left].x >= stages[right].x {
					t.Fatalf(
						"run %d child %s reverses functional flow: %s rank %d x=%.2f mm, %s rank %d x=%.2f mm",
						run, filename,
						stages[left].name, stages[left].rank, float64(stages[left].x)/float64(kicadfiles.MM(1)),
						stages[right].name, stages[right].rank, float64(stages[right].x)/float64(kicadfiles.MM(1)),
					)
				}
			}
		}
	}
}

func hierarchySheetHasPin(sheet schematic.Sheet, name string) bool {
	for _, pin := range sheet.Pins {
		if pin.Text == name {
			return true
		}
	}
	return false
}

func hierarchyChildHasLabel(child *schematic.SchematicFile, name string, kind schematic.LabelKind) bool {
	if child == nil {
		return false
	}
	for _, label := range child.Labels {
		if label.Text == name && label.Kind == kind {
			return true
		}
	}
	return false
}

func hierarchyRootPinConnected(root *schematic.SchematicFile, sheet schematic.Sheet, name string) bool {
	if root == nil {
		return false
	}
	for _, pin := range sheet.Pins {
		if pin.Text != name {
			continue
		}
		for _, wire := range root.Wires {
			if len(wire.Points) < 2 {
				continue
			}
			var other kicadfiles.Point
			switch {
			case wire.Points[0] == pin.Position:
				other = wire.Points[len(wire.Points)-1]
			case wire.Points[len(wire.Points)-1] == pin.Position:
				other = wire.Points[0]
			default:
				continue
			}
			for _, label := range root.Labels {
				if label.Kind == schematic.LabelLocal && label.Text == name && label.Position == other {
					return true
				}
			}
			for _, candidate := range root.Sheets {
				for _, candidatePin := range candidate.Pins {
					if candidatePin.Text == name && candidatePin.Position == other {
						return true
					}
				}
			}
		}
	}
	return false
}

func assertHumanQualityNormalizedWriterDiffs(t *testing.T, run int, projectRoot string) {
	t.Helper()
	writerRoot := filepath.Join(projectRoot, ".evidence", "writer")
	normalizedDiffs := 0
	err := filepath.Walk(writerRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() != "normalized.diff" {
			return nil
		}
		normalizedDiffs++
		if info.Size() != 0 {
			return fmt.Errorf("non-zero normalized writer diff %s (%d bytes)", path, info.Size())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run %d writer round-trip evidence: %v", run, err)
	}
	if normalizedDiffs == 0 {
		t.Fatalf("run %d has no normalized writer round-trip diffs under %s", run, writerRoot)
	}
}
