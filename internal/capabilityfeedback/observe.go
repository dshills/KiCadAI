package capabilityfeedback

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
)

var caseIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[-_][a-z0-9]+)*$`)

func Observe(
	meta CaseMeta,
	requirement opentopologysynthesis.Requirement,
	run opentopologysynthesis.SynthesisRun,
	promotion *opentopologysynthesis.PhysicalPromotionResult,
) (CaseEvidence, error) {
	if err := validateCaseMeta(meta); err != nil {
		return CaseEvidence{}, err
	}
	if issues := opentopologysynthesis.Validate(requirement); len(issues) != 0 {
		return CaseEvidence{}, fmt.Errorf("case %q requirement is invalid: %#v", meta.ID, issues)
	}
	requirementHash, err := opentopologysynthesis.CanonicalHash(requirement)
	if err != nil {
		return CaseEvidence{}, fmt.Errorf("case %q requirement hash: %w", meta.ID, err)
	}
	if run.Schema != opentopologysynthesis.SynthesisRunSchema ||
		run.Version != opentopologysynthesis.SynthesisRunVersion ||
		run.Report.Schema != opentopologysynthesis.ReportSchema ||
		run.Report.Version != opentopologysynthesis.ReportVersion ||
		run.Report.RequirementHash != requirementHash || !validSHA256(run.Hash) {
		return CaseEvidence{}, fmt.Errorf("case %q synthesis evidence does not match the normalized requirement", meta.ID)
	}
	if err := validateSynthesisEnvelope(run); err != nil {
		return CaseEvidence{}, fmt.Errorf("case %q synthesis evidence: %w", meta.ID, err)
	}

	evidence := CaseEvidence{
		Schema:              CaseEvidenceSchema,
		PolicyVersion:       PolicyVersion,
		Case:                meta,
		StopReason:          string(run.Report.StopReason),
		RequirementHash:     requirementHash,
		InventoryHash:       run.Report.PrimitiveInventoryHash,
		CatalogHash:         run.Report.CatalogHash,
		ModelRegistryHash:   run.Report.ModelRegistryHash,
		SynthesisPolicyHash: run.Report.PolicyHash,
		SynthesisHash:       run.Hash,
		AnalysisKinds:       requirementAnalysisKinds(requirement),
		Consumption:         run.Report.Consumption,
	}

	switch run.Report.Status {
	case opentopologysynthesis.StatusPassed:
		if promotion == nil {
			return CaseEvidence{}, fmt.Errorf("case %q passing synthesis requires physical-promotion evidence", meta.ID)
		}
		switch promotion.Status {
		case opentopologysynthesis.PhysicalPromotionPassed:
			if err := validatePassingPromotion(requirementHash, run.Hash, *promotion); err != nil {
				return CaseEvidence{}, fmt.Errorf("case %q invalid passing physical-promotion evidence: %w", meta.ID, err)
			}
			evidence.Outcome = OutcomePass
			evidence.PromotionHash = promotion.Hash
			evidence.ProjectHash = promotion.ProjectHash
		case opentopologysynthesis.PhysicalPromotionFailed:
			if err := validatePromotionEnvelope(requirementHash, run.Hash, *promotion); err != nil {
				return CaseEvidence{}, fmt.Errorf("case %q invalid failed physical-promotion evidence: %w", meta.ID, err)
			}
			evidence.Outcome = OutcomeUnsupported
			evidence.StopReason = string(opentopologysynthesis.StopPhysicalPromotionFailed)
			evidence.PromotionHash = promotion.Hash
			evidence.Gaps = promotionGaps(*promotion, run.Hash)
		case opentopologysynthesis.PhysicalPromotionInvalid:
			return CaseEvidence{}, fmt.Errorf("case %q invalid physical-promotion evidence is not a capability outcome", meta.ID)
		default:
			return CaseEvidence{}, fmt.Errorf("case %q has unknown physical-promotion status %q", meta.ID, promotion.Status)
		}
	case opentopologysynthesis.StatusUnsupported:
		evidence.Outcome = OutcomeUnsupported
		evidence.Gaps = synthesisGaps(requirement, run)
	case opentopologysynthesis.StatusInfeasible:
		evidence.Outcome = OutcomeUnsafe
		evidence.Gaps = synthesisGaps(requirement, run)
	case opentopologysynthesis.StatusExhausted:
		evidence.Outcome = OutcomeExhausted
		evidence.Gaps = synthesisGaps(requirement, run)
	case opentopologysynthesis.StatusFailed:
		if run.Report.Consumption.BudgetExhausted {
			evidence.Outcome = OutcomeExhausted
		} else if allCandidateFailuresAreCriticalAssertions(requirement, run) {
			evidence.Outcome = OutcomeUnsafe
		} else {
			evidence.Outcome = OutcomeUnsupported
		}
		evidence.Gaps = synthesisGaps(requirement, run)
	case opentopologysynthesis.StatusInvalid, opentopologysynthesis.StatusCanceled:
		return CaseEvidence{}, fmt.Errorf("case %q synthesis status %q is an evaluation failure, not a capability outcome", meta.ID, run.Report.Status)
	default:
		return CaseEvidence{}, fmt.Errorf("case %q has unknown synthesis status %q", meta.ID, run.Report.Status)
	}
	if evidence.Outcome != OutcomePass && len(evidence.Gaps) == 0 {
		return CaseEvidence{}, fmt.Errorf("case %q non-passing outcome has no causal capability gap", meta.ID)
	}
	evidence.Gaps = normalizeGaps(evidence.Gaps)
	hash, err := caseEvidenceHash(evidence)
	if err != nil {
		return CaseEvidence{}, err
	}
	evidence.Hash = hash
	if err := ValidateCaseEvidence(evidence); err != nil {
		return CaseEvidence{}, fmt.Errorf("case %q produced invalid capability evidence: %w", meta.ID, err)
	}
	return evidence, nil
}

// ObserveRealizabilityAware preserves the legacy observation contract while
// refining only otherwise-generic terminal topology failures with deterministic
// requirement realizability evidence. V1/V2 frozen artifacts continue to use
// Observe and PolicyVersion unchanged.
func ObserveRealizabilityAware(
	meta CaseMeta,
	requirement opentopologysynthesis.Requirement,
	run opentopologysynthesis.SynthesisRun,
	promotion *opentopologysynthesis.PhysicalPromotionResult,
) (CaseEvidence, error) {
	evidence, err := Observe(meta, requirement, run, promotion)
	if err != nil {
		return CaseEvidence{}, err
	}
	if topologyFailureCanBeRefined(evidence, run) {
		assessment := opentopologysynthesis.AssessRequirementRealizability(requirement)
		if len(assessment.Issues) != 0 {
			return CaseEvidence{}, fmt.Errorf("case %q realizability assessment rejected validated requirement", meta.ID)
		}
		if gaps := realizabilityFindingGaps(assessment.Findings, evidence.Gaps, run.Hash); len(gaps) != 0 {
			evidence.Gaps = normalizeGaps(gaps)
		}
	}
	evidence.PolicyVersion = RealizabilityPolicyVersion
	evidence.Hash = ""
	evidence.Hash, err = caseEvidenceHash(evidence)
	if err != nil {
		return CaseEvidence{}, err
	}
	if err := ValidateCaseEvidence(evidence); err != nil {
		return CaseEvidence{}, fmt.Errorf("case %q produced invalid realizability-aware evidence: %w", meta.ID, err)
	}
	return evidence, nil
}

func topologyFailureCanBeRefined(evidence CaseEvidence, run opentopologysynthesis.SynthesisRun) bool {
	if run.Report.StopReason != opentopologysynthesis.StopSearchExhausted &&
		run.Report.StopReason != opentopologysynthesis.StopNoCompleteGraph {
		return false
	}
	if len(evidence.Gaps) == 0 {
		return false
	}
	for _, gap := range evidence.Gaps {
		if gap.Stage != "topology_search" || gap.Scope != ScopeTopology || gap.Capability != "complete_topology" {
			return false
		}
	}
	return true
}

func realizabilityFindingGaps(
	findings []opentopologysynthesis.RequirementRealizabilityFinding,
	terminal []Gap,
	evidenceHash string,
) []Gap {
	symptoms := make([]string, 0, len(terminal))
	for _, gap := range terminal {
		symptoms = append(symptoms, gap.Code)
	}
	gaps := make([]Gap, 0, len(findings))
	for _, finding := range findings {
		capability := ""
		switch finding.Code {
		case opentopologysynthesis.CodeEnergyDomainCreationRequired:
			capability = "energy_domain_creation"
		case opentopologysynthesis.CodeMultiOutputCompositionRequired,
			opentopologysynthesis.CodeMultiControlCompositionRequired:
			capability = "multi_obligation_composition"
		default:
			continue
		}
		gaps = append(gaps, Gap{
			Stage: "requirement_realizability", Scope: ScopeTopology,
			Capability: capability, Code: string(finding.Code),
			RequirementIDs: finding.RequirementIDs, OperatingCases: finding.OperatingCases,
			AnalysisKinds:    finding.AnalysisKinds,
			RequiredEvidence: requiredEvidence(ScopeTopology, capability),
			EvidenceHashes:   []string{evidenceHash}, DownstreamSymptoms: symptoms,
		})
	}
	return gaps
}

func validateSynthesisEnvelope(run opentopologysynthesis.SynthesisRun) error {
	if !validSHA256(run.Report.PolicyHash) || !validSHA256(run.Report.RequirementHash) ||
		!validSHA256(run.Report.PrimitiveInventoryHash) || !validSHA256(run.Report.CatalogHash) ||
		!validSHA256(run.Report.ModelRegistryHash) {
		return fmt.Errorf("required content hashes are missing")
	}
	if run.Report.Status == opentopologysynthesis.StatusPassed {
		if run.SelectedGraph == nil {
			return fmt.Errorf("passing synthesis lacks a selected graph")
		}
		if run.Report.StopReason != opentopologysynthesis.StopPassed || run.Report.Selected == nil ||
			run.SelectedTrial == nil || run.Physical == nil ||
			run.Physical.Status != opentopologysynthesis.PhysicalLoweringReady ||
			!validSHA256(run.Physical.Hash) || len(run.Report.Diagnostics) != 0 {
			return fmt.Errorf("passing synthesis lacks a complete selected and physically ready result")
		}
		selected := run.Report.Selected
		selectedGraphHash, graphHashErr := opentopologysynthesis.GraphHash(*run.SelectedGraph)
		selectedTopologyHash, topologyHashErr := opentopologysynthesis.TopologyHash(*run.SelectedGraph)
		if !validSHA256(selected.TopologyHash) || !validSHA256(selected.ActiveStructureHash) ||
			!validSHA256(selected.EvaluationHash) || selected.PhysicalHash != run.Physical.Hash ||
			graphHashErr != nil || topologyHashErr != nil ||
			run.Physical.RequirementHash != run.Report.RequirementHash ||
			run.Physical.InventoryHash != run.Report.PrimitiveInventoryHash ||
			selected.TopologyHash != selectedTopologyHash ||
			run.Physical.GraphHash != selectedGraphHash ||
			run.Physical.EvaluationHash != selected.EvaluationHash {
			return fmt.Errorf("passing synthesis selection and physical evidence are not hash-linked")
		}
		return nil
	}
	if run.Report.StopReason == opentopologysynthesis.StopPassed || run.Report.Selected != nil ||
		run.SelectedGraph != nil || run.SelectedTrial != nil || run.Physical != nil {
		return fmt.Errorf("non-passing synthesis retains passing selection state")
	}
	return nil
}

func validateCaseMeta(meta CaseMeta) error {
	if !caseIDPattern.MatchString(meta.ID) || (meta.Role != RoleDiscovery && meta.Role != RoleHeldOut) {
		return fmt.Errorf("invalid closed-loop corpus case identity or role")
	}
	switch meta.Domain {
	case capabilityevaluation.DomainAnalog, capabilityevaluation.DomainPower,
		capabilityevaluation.DomainMCU, capabilityevaluation.DomainSensor,
		capabilityevaluation.DomainDigital, capabilityevaluation.DomainMixedSignal:
	default:
		return fmt.Errorf("case %q has invalid reporting domain %q", meta.ID, meta.Domain)
	}
	switch meta.SafetyImpact {
	case capabilityevaluation.SafetyNonSafety, capabilityevaluation.SafetyReviewRequired,
		capabilityevaluation.SafetyRelevant, capabilityevaluation.SafetyCritical:
	default:
		return fmt.Errorf("case %q has invalid safety impact %q", meta.ID, meta.SafetyImpact)
	}
	return nil
}

func validatePassingPromotion(requirementHash, synthesisHash string, promotion opentopologysynthesis.PhysicalPromotionResult) error {
	if err := validatePromotionEnvelope(requirementHash, synthesisHash, promotion); err != nil {
		return err
	}
	if promotion.Status != opentopologysynthesis.PhysicalPromotionPassed ||
		!promotion.ReplayIdentical || len(promotion.Runs) != 2 || len(promotion.Issues) != 0 ||
		!validSHA256(promotion.ProjectHash) {
		return fmt.Errorf("physical promotion is incomplete or inconsistent")
	}
	for index, run := range promotion.Runs {
		if run.Number != index+1 || !validSHA256(run.ProjectHash) || run.ProjectHash != promotion.ProjectHash {
			return fmt.Errorf("physical promotion run %d is incomplete or divergent", index+1)
		}
	}
	return nil
}

func validatePromotionEnvelope(requirementHash, synthesisHash string, promotion opentopologysynthesis.PhysicalPromotionResult) error {
	if promotion.Schema != opentopologysynthesis.PhysicalPromotionSchema ||
		promotion.Version != opentopologysynthesis.PhysicalPromotionVersion ||
		promotion.RequirementHash != requirementHash || promotion.SynthesisHash != synthesisHash ||
		!validSHA256(promotion.Hash) {
		return fmt.Errorf("physical promotion envelope is incomplete or inconsistent")
	}
	return nil
}

func synthesisGaps(requirement opentopologysynthesis.Requirement, run opentopologysynthesis.SynthesisRun) []Gap {
	if universal := universalDiagnosisGaps(requirement, run); len(universal) != 0 {
		symptoms := []string{}
		for _, diagnostic := range run.Report.Diagnostics {
			symptoms = append(symptoms, string(diagnostic.Code))
		}
		for index := range universal {
			universal[index].DownstreamSymptoms = normalizedStrings(symptoms)
			universal[index].EvidenceHashes = normalizedStrings(append(universal[index].EvidenceHashes, run.Hash))
		}
		return universal
	}
	gaps := make([]Gap, 0, len(run.Report.Diagnostics))
	for _, diagnostic := range run.Report.Diagnostics {
		gaps = append(gaps, synthesisDiagnosticGap(diagnostic, run.Hash))
	}
	if len(gaps) == 0 {
		code := codeForStopReason(run.Report.StopReason)
		gaps = append(gaps, gapForCode(code, "", run.Hash))
	}
	return gaps
}

type diagnosisAggregate struct {
	gap        Gap
	candidates map[string]bool
}

func universalDiagnosisGaps(requirement opentopologysynthesis.Requirement, run opentopologysynthesis.SynthesisRun) []Gap {
	if len(run.Candidates) == 0 {
		return nil
	}
	byKey := map[string]*diagnosisAggregate{}
	for _, candidate := range run.Candidates {
		seen := map[string]bool{}
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				key := strings.Join([]string{diagnosis.Code, diagnosis.Analysis, diagnosis.Metric}, "\x00")
				if seen[key] {
					continue
				}
				seen[key] = true
				current := byKey[key]
				if current == nil {
					gap := diagnosisGap(diagnosis)
					current = &diagnosisAggregate{gap: gap, candidates: map[string]bool{}}
					byKey[key] = current
				}
				current.candidates[candidate.Fingerprint] = true
				current.gap.RequirementIDs = append(current.gap.RequirementIDs, diagnosis.RequirementID)
				current.gap.OperatingCases = append(current.gap.OperatingCases, diagnosis.OperatingCase)
				current.gap.AnalysisKinds = append(current.gap.AnalysisKinds, diagnosis.Analysis)
				current.gap.EvidenceHashes = append(current.gap.EvidenceHashes, diagnosis.EvidenceHash)
			}
		}
	}
	result := []Gap{}
	for _, current := range byKey {
		if len(current.candidates) != len(run.Candidates) {
			continue
		}
		result = append(result, current.gap)
	}
	return normalizeGaps(result)
}

func diagnosisGap(diagnosis opentopologysynthesis.Diagnosis) Gap {
	code := normalizeCode(diagnosis.Code)
	analysis, metric := canonicalID(diagnosis.Analysis), canonicalID(diagnosis.Metric)
	gap := Gap{Stage: "simulation", Scope: ScopeSimulation, Code: code}
	switch diagnosis.Code {
	case "model_unavailable", "thermal_model_unavailable", "metric_unsupported":
		gap.Scope = ScopeModel
		gap.Capability = firstNonEmpty(analysis+"_model", "trusted_simulation_model")
	case "simulation_nonconvergent", "simulation_invalid", "unstable":
		gap.Capability = firstNonEmpty(analysis+"_solver", "trusted_simulation_solver")
	default:
		gap.Capability = firstNonEmpty(metric+"_evidence", analysis+"_evidence", "behavioral_simulation_evidence")
	}
	gap.RequiredEvidence = requiredEvidence(gap.Scope, gap.Capability)
	return gap
}

func synthesisDiagnosticGap(diagnostic opentopologysynthesis.Diagnostic, evidenceHash string) Gap {
	return gapForCode(string(diagnostic.Code), diagnostic.Path, evidenceHash)
}

func gapForCode(code, stageHint, evidenceHash string) Gap {
	normalizedCode := normalizeCode(code)
	gap := Gap{Code: normalizedCode, EvidenceHashes: normalizedStrings([]string{evidenceHash})}
	switch reports.Code(code) {
	case opentopologysynthesis.CodeRequirementInfeasible:
		gap.Stage, gap.Scope, gap.Capability = "requirement", ScopeVerification, "behavioral_feasibility"
	case opentopologysynthesis.CodePrimitiveUnavailable:
		gap.Stage, gap.Scope, gap.Capability = "component_selection", ScopeComponent, "primitive_inventory"
	case opentopologysynthesis.CodeModelUnavailable:
		gap.Stage, gap.Scope, gap.Capability = "simulation", ScopeModel, "trusted_simulation_model"
	case opentopologysynthesis.CodeSearchExhausted, opentopologysynthesis.CodeNoCompleteGraph:
		gap.Stage, gap.Scope, gap.Capability = "topology_search", ScopeTopology, "complete_topology"
	case opentopologysynthesis.CodeNoPassingGraph:
		gap.Stage, gap.Scope, gap.Capability = "simulation", ScopeSimulation, "passing_behavioral_evidence"
	case opentopologysynthesis.CodeValueExhausted:
		gap.Stage, gap.Scope, gap.Capability = "value_search", ScopeComponent, "catalog_value_domain"
	case opentopologysynthesis.CodeRepairUnsupported, opentopologysynthesis.CodeRepairExhausted:
		gap.Stage, gap.Scope, gap.Capability = "topology_repair", ScopeTopology, "causal_topology_repair"
	case opentopologysynthesis.CodePhysicalPromotionFailed:
		gap.Stage, gap.Scope, gap.Capability = "physical_promotion", ScopePhysical, "physical_promotion"
	default:
		gap.Stage, gap.Scope, gap.Capability = stageScopeCapability(stageHint, reports.Code(code))
	}
	gap.RequiredEvidence = requiredEvidence(gap.Scope, gap.Capability)
	return gap
}

func promotionGaps(promotion opentopologysynthesis.PhysicalPromotionResult, synthesisHash string) []Gap {
	if len(promotion.Issues) == 0 {
		return []Gap{gapForCode(string(opentopologysynthesis.CodePhysicalPromotionFailed), "physical_promotion", synthesisHash)}
	}
	issuesByID := map[string]reports.Issue{}
	for _, issue := range promotion.Issues {
		if issue.IssueID != "" {
			issuesByID[issue.IssueID] = issue
		}
	}
	roots := []reports.Issue{}
	downstream := map[string][]string{}
	for _, issue := range promotion.Issues {
		if issue.RootCauseID != "" {
			if _, found := issuesByID[issue.RootCauseID]; found {
				downstream[issue.RootCauseID] = append(downstream[issue.RootCauseID], string(issue.Code))
				continue
			}
		}
		roots = append(roots, issue)
	}
	gaps := make([]Gap, 0, len(roots))
	for _, issue := range roots {
		gap := gapForCode(string(issue.Code), firstNonEmpty(issue.Stage, issue.Path), promotion.Hash)
		gap.DownstreamSymptoms = normalizedStrings(downstream[issue.IssueID])
		gaps = append(gaps, gap)
	}
	return gaps
}

func stageScopeCapability(stageHint string, code reports.Code) (string, GapScope, string) {
	stage := canonicalID(stageHint)
	switch code {
	case reports.CodeMissingFootprint, reports.CodeUnknownSymbolLibrary,
		reports.CodeUnknownFootprintLibrary, reports.CodePinmapUnverified:
		return "component_selection", ScopeComponent, "catalog_physical_binding"
	case reports.CodePlacementCollision, reports.CodePlacementOutsideBoard:
		return "placement", ScopePhysical, "placement_feasibility"
	case reports.CodeDisconnectedPad, reports.CodeRouteContactMissingTarget,
		reports.CodeRouteContactNetMismatch, reports.CodeRouteContactLayerMismatch,
		reports.CodeRouteContactMiss, reports.CodeRouteContactAmbiguous,
		reports.CodeRouteContactUnsupported, reports.CodeRouteGraphIncomplete,
		reports.CodeRouteCompletionPartial, reports.CodeRouteCopperConflict,
		reports.CodeFixedNetSkipped:
		return "routing", ScopeRouting, "route_completion"
	case reports.CodeRoundTripDiff:
		return "round_trip", ScopeVerification, "round_trip_fidelity"
	case reports.CodeKiCadCLIFailed, reports.CodeSkippedExternalTool:
		return "kicad_verification", ScopeVerification, "installed_kicad_verification"
	}
	switch {
	case strings.Contains(stage, "rout"):
		return "routing", ScopeRouting, "route_completion"
	case strings.Contains(stage, "place") || strings.Contains(stage, "pcb"):
		return "physical", ScopePhysical, "physical_realization"
	case strings.Contains(stage, "model"):
		return "simulation", ScopeModel, "trusted_simulation_model"
	case strings.Contains(stage, "sim"):
		return "simulation", ScopeSimulation, "trusted_simulation_evidence"
	default:
		return firstNonEmpty(stage, "verification"), ScopeVerification, "workflow_verification"
	}
}

func codeForStopReason(reason opentopologysynthesis.StopReason) string {
	switch reason {
	case opentopologysynthesis.StopRequirementInfeasible:
		return string(opentopologysynthesis.CodeRequirementInfeasible)
	case opentopologysynthesis.StopPrimitiveUnavailable:
		return string(opentopologysynthesis.CodePrimitiveUnavailable)
	case opentopologysynthesis.StopModelUnavailable:
		return string(opentopologysynthesis.CodeModelUnavailable)
	case opentopologysynthesis.StopSearchExhausted:
		return string(opentopologysynthesis.CodeSearchExhausted)
	case opentopologysynthesis.StopNoCompleteGraph:
		return string(opentopologysynthesis.CodeNoCompleteGraph)
	case opentopologysynthesis.StopNoPassingGraph:
		return string(opentopologysynthesis.CodeNoPassingGraph)
	case opentopologysynthesis.StopValueExhausted:
		return string(opentopologysynthesis.CodeValueExhausted)
	case opentopologysynthesis.StopRepairUnsupported:
		return string(opentopologysynthesis.CodeRepairUnsupported)
	case opentopologysynthesis.StopRepairExhausted:
		return string(opentopologysynthesis.CodeRepairExhausted)
	case opentopologysynthesis.StopPhysicalPromotionFailed:
		return string(opentopologysynthesis.CodePhysicalPromotionFailed)
	default:
		return normalizeCode(string(reason))
	}
}

func allCandidateFailuresAreCriticalAssertions(requirement opentopologysynthesis.Requirement, run opentopologysynthesis.SynthesisRun) bool {
	critical := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		critical[assertion.ID] = assertion.Critical
	}
	found := false
	for _, candidate := range run.Candidates {
		candidateFound := false
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				if diagnosis.Code != "assertion_below_minimum" && diagnosis.Code != "assertion_above_maximum" {
					return false
				}
				if !critical[diagnosis.RequirementID] {
					return false
				}
				found = true
				candidateFound = true
			}
		}
		if !candidateFound {
			return false
		}
	}
	return found
}

func requirementAnalysisKinds(requirement opentopologysynthesis.Requirement) []string {
	values := []string{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		values = append(values, canonicalID(assertion.Analysis))
	}
	return normalizedStrings(values)
}

func requiredEvidence(scope GapScope, capability string) []string {
	var evidence string
	switch scope {
	case ScopeTopology:
		evidence = "reviewed reusable topology construction and complete-graph evidence"
	case ScopeComponent:
		evidence = "catalog-backed component, rating, value, symbol, pin, footprint, and pad evidence"
	case ScopeModel:
		evidence = "reviewed model provenance and applicability bounds"
	case ScopeSimulation:
		evidence = "trusted deterministic analysis, convergence, corner, and assertion evidence"
	case ScopePhysical:
		evidence = "generic deterministic physical-rule and placement evidence"
	case ScopeRouting:
		evidence = "generic complete routing, endpoint-access, layer-transition, and connectivity evidence"
	case ScopeVerification:
		evidence = "passing writer, installed-KiCad, round-trip, and replay evidence"
	}
	return []string{capability + ": " + evidence}
}

func normalizeGaps(values []Gap) []Gap {
	byKey := map[string]Gap{}
	for _, gap := range values {
		gap.Stage = canonicalID(gap.Stage)
		gap.Capability = canonicalID(gap.Capability)
		gap.Code = normalizeCode(gap.Code)
		gap.RequirementIDs = normalizedStrings(gap.RequirementIDs)
		gap.OperatingCases = normalizedStrings(gap.OperatingCases)
		gap.AnalysisKinds = normalizedStrings(gap.AnalysisKinds)
		gap.RequiredEvidence = normalizedStrings(gap.RequiredEvidence)
		gap.EvidenceHashes = normalizedHashes(gap.EvidenceHashes)
		gap.DownstreamSymptoms = normalizedStrings(gap.DownstreamSymptoms)
		key := clusterKey(gap, OutcomeUnsupported)
		if prior, found := byKey[key]; found {
			prior.RequirementIDs = normalizedStrings(append(prior.RequirementIDs, gap.RequirementIDs...))
			prior.OperatingCases = normalizedStrings(append(prior.OperatingCases, gap.OperatingCases...))
			prior.AnalysisKinds = normalizedStrings(append(prior.AnalysisKinds, gap.AnalysisKinds...))
			prior.RequiredEvidence = normalizedStrings(append(prior.RequiredEvidence, gap.RequiredEvidence...))
			prior.EvidenceHashes = normalizedHashes(append(prior.EvidenceHashes, gap.EvidenceHashes...))
			prior.DownstreamSymptoms = normalizedStrings(append(prior.DownstreamSymptoms, gap.DownstreamSymptoms...))
			byKey[key] = prior
		} else {
			byKey[key] = gap
		}
	}
	result := make([]Gap, 0, len(byKey))
	for _, gap := range byKey {
		result = append(result, gap)
	}
	slices.SortFunc(result, func(left, right Gap) int {
		return cmp.Or(
			cmp.Compare(left.Stage, right.Stage), cmp.Compare(left.Scope, right.Scope),
			cmp.Compare(left.Capability, right.Capability), cmp.Compare(left.Code, right.Code),
		)
	})
	return result
}

func canonicalID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	separator := false
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			if separator && builder.Len() != 0 {
				builder.WriteByte('_')
			}
			builder.WriteRune(current)
			separator = false
		} else {
			separator = true
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result != "" && result[0] >= '0' && result[0] <= '9' {
		result = "capability_" + result
	}
	return result
}

func normalizeCode(value string) string {
	value = strings.TrimSpace(strings.ToUpper(value))
	var builder strings.Builder
	separator := false
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			if separator && builder.Len() != 0 {
				builder.WriteByte('_')
			}
			builder.WriteRune(current)
			separator = false
		} else {
			separator = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func normalizedStrings(values []string) []string {
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func normalizedHashes(values []string) []string {
	return normalizedStrings(values)
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func caseEvidenceHash(evidence CaseEvidence) (string, error) {
	hashless := evidence
	hashless.Hash = ""
	return digest(hashless)
}

func digest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.Trim(value, "_"); value != "" {
			return value
		}
	}
	return ""
}
