package compositionlowering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/designworkflow"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	ArchitectureVariableFunctionParameter = "function_parameter"
	ArchitectureVariableComponentValue    = "component_value"
	ArchitectureVariableModelParameter    = "component_model_parameter"
	ArchitectureVariableCatalogVariant    = "catalog_variant_index"
	behavioralDistortionFrequencyHz       = 1000.0
	behavioralDistortionCycles            = 4
	behavioralDistortionSamplesPerCycle   = 32
	behavioralTransientSamplesPerCycle    = 32
	behavioralRatedPowerVoltageGuard      = 1.02
	behavioralStartupSteps                = 256
)

// ArchitectureVariantChoice is a resolver-owned catalog identity selected by
// a bounded numeric catalog-variant repair variable. Providers cannot add
// identities to this reviewed choice set.
type ArchitectureVariantChoice struct {
	Index       float64 `json:"index"`
	ComponentID string  `json:"component_id"`
	VariantID   string  `json:"variant_id,omitempty"`
}

// ArchitectureVariableBinding authorizes one candidate variable to modify a
// typed public circuit-graph field. CandidateFingerprint prevents a variable
// name shared by materially different architectures from crossing scopes.
type ArchitectureVariableBinding struct {
	CandidateFingerprint string                      `json:"candidate_fingerprint"`
	VariableID           string                      `json:"variable_id"`
	Kind                 string                      `json:"kind"`
	Function             string                      `json:"function,omitempty"`
	Component            string                      `json:"component,omitempty"`
	Parameter            string                      `json:"parameter,omitempty"`
	Unit                 string                      `json:"unit,omitempty"`
	VariantChoices       []ArchitectureVariantChoice `json:"variant_choices,omitempty"`
}

// ArchitectureSimulationPlanResolver connects retained architecture-search
// candidates to the production lowering, synthesis, catalog-resolution, and
// graph-simulation path. Every ResolveSimulationPlans call starts from the
// immutable search candidate and rebuilds all downstream evidence.
type ArchitectureSimulationPlanResolver struct {
	Requirement        architecturesearch.Requirement
	Search             architecturesearch.SearchResult
	GraphResolver      *circuitgraph.Resolver
	RequiredAnalyses   []string
	BaseIntents        map[string]simmodel.Intent
	ProvenanceRegistry modelprovenance.Registry
	VariableBindings   []ArchitectureVariableBinding
}

type ClosedLoopCandidateVariables struct {
	Fingerprint string                         `json:"fingerprint"`
	Variables   []closedloopsynthesis.Variable `json:"variables,omitempty"`
}

// ClosedLoopPromotion is the single trusted handoff from behavioral candidate
// selection to the physical design workflow. Request is lowered from the exact
// selected and repaired state recorded by Report.
type ClosedLoopPromotion struct {
	Report   closedloopsynthesis.Report    `json:"report"`
	Resolved circuitgraph.ResolvedDocument `json:"resolved"`
	Request  designworkflow.Request        `json:"request"`
}

// SynthesizeClosedLoop evaluates every retained architecture, selects a
// passing candidate under the deterministic policy, re-resolves that exact
// final state, and binds the resulting circuit hash and report into the normal
// physical workflow request. A caller must not promote a returned request when
// issues block.
func SynthesizeClosedLoop(
	ctx context.Context,
	requirement architecturesearch.Requirement,
	search architecturesearch.SearchResult,
	resolver ArchitectureSimulationPlanResolver,
	modelRegistryHash string,
	variableSets []ClosedLoopCandidateVariables,
	policy closedloopsynthesis.Policy,
) (ClosedLoopPromotion, []reports.Issue) {
	if len(variableSets) == 0 {
		derivedSets, derivedBindings, derivationDiagnostics := providerRepairVariables(search)
		if len(derivationDiagnostics) != 0 {
			return ClosedLoopPromotion{}, closedLoopReportIssues("closed_loop.repair_variables", derivationDiagnostics)
		}
		variableSets = derivedSets
		resolver.VariableBindings = append(resolver.VariableBindings, derivedBindings...)
	}
	input, diagnostics := BuildClosedLoopInput(requirement, search, modelRegistryHash, variableSets)
	if len(diagnostics) != 0 {
		return ClosedLoopPromotion{}, closedLoopReportIssues("closed_loop.input", diagnostics)
	}
	resolver.Requirement = requirement
	resolver.Search = search
	evaluator := closedloopsynthesis.SimModelEvaluator{
		Resolver:           closedloopsynthesis.PlannedSimulationResolver{Base: resolver},
		ProvenanceRegistry: resolver.ProvenanceRegistry,
	}
	report := closedloopsynthesis.Run(ctx, input, evaluator, policy)
	if report.Status != "pass" || report.Selected == nil {
		issues := closedLoopReportIssues("closed_loop", report.Diagnostics)
		if len(issues) == 0 {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "closed_loop.status", Message: "behavioral synthesis did not select a passing candidate"})
		}
		return ClosedLoopPromotion{Report: report}, issues
	}
	resolvedCandidate, err := resolver.resolveArchitectureCandidate(ctx, report.Selected.State)
	if err != nil {
		return ClosedLoopPromotion{Report: report}, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "closed_loop.selected", Message: err.Error()}}
	}
	request, requestIssues := circuitgraph.ToDesignRequest(resolvedCandidate.Resolved)
	if reports.HasBlockingIssue(requestIssues) || request.ExplicitCircuit == nil {
		return ClosedLoopPromotion{Report: report, Resolved: resolvedCandidate.Resolved, Request: request}, requestIssues
	}
	report.SelectedCircuitHash = request.ExplicitCircuit.ResolutionHash
	request.ExplicitCircuit.ClosedLoop = &report
	if validationDiagnostics := closedloopsynthesis.ValidatePromotionReport(report, request.ExplicitCircuit.CatalogHash); len(validationDiagnostics) != 0 {
		return ClosedLoopPromotion{Report: report, Resolved: resolvedCandidate.Resolved, Request: request}, closedLoopReportIssues("closed_loop.promotion", validationDiagnostics)
	}
	return ClosedLoopPromotion{Report: report, Resolved: resolvedCandidate.Resolved, Request: request}, requestIssues
}

func providerRepairVariables(search architecturesearch.SearchResult) ([]ClosedLoopCandidateVariables, []ArchitectureVariableBinding, []closedloopsynthesis.Diagnostic) {
	retained := []architecturesearch.CandidateResult{}
	if search.Selected != nil {
		retained = append(retained, *search.Selected)
	}
	retained = append(retained, search.Alternatives...)
	slices.SortStableFunc(retained, func(left, right architecturesearch.CandidateResult) int {
		return strings.Compare(left.Fingerprint, right.Fingerprint)
	})
	var sets []ClosedLoopCandidateVariables
	var bindings []ArchitectureVariableBinding
	var diagnostics []closedloopsynthesis.Diagnostic
	for candidateIndex, candidate := range retained {
		var variables []closedloopsynthesis.Variable
		seen := map[string]bool{}
		for selectionIndex, selection := range candidate.Selections {
			realization, err := architecturesearch.DecodeFragmentRealization(selection.Payload)
			if err != nil {
				diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: fmt.Sprintf("candidates[%d].selections[%d]", candidateIndex, selectionIndex), Message: err.Error()})
				continue
			}
			prefix := safeID(selection.ObligationPath)
			for repairIndex, repair := range realization.RepairVariables {
				variableID := safeID(prefix + "__" + repair.ID)
				if seen[variableID] {
					diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: fmt.Sprintf("candidates[%d].selections[%d].repair_variables[%d]", candidateIndex, selectionIndex, repairIndex), Message: "provider repair variable is duplicated after candidate namespacing"})
					continue
				}
				seen[variableID] = true
				effects := make([]closedloopsynthesis.RepairEffect, 0, len(repair.Effects))
				for _, effect := range repair.Effects {
					effects = append(effects, closedloopsynthesis.RepairEffect{Analysis: effect.Analysis, Metric: effect.Metric, Direction: effect.Direction})
				}
				variables = append(variables, closedloopsynthesis.Variable{ID: variableID, Kind: repair.Kind, Value: repair.Value, AllowedValues: append([]float64(nil), repair.AllowedValues...), Effects: effects})
				bindings = append(bindings, ArchitectureVariableBinding{
					CandidateFingerprint: candidate.Fingerprint, VariableID: variableID, Kind: ArchitectureVariableComponentValue,
					Component: safeID(prefix + "__" + repair.Instance), Unit: repair.Unit,
				})
			}
		}
		if len(variables) != 0 {
			slices.SortStableFunc(variables, func(left, right closedloopsynthesis.Variable) int { return strings.Compare(left.ID, right.ID) })
			sets = append(sets, ClosedLoopCandidateVariables{Fingerprint: candidate.Fingerprint, Variables: variables})
		}
	}
	slices.SortStableFunc(bindings, func(left, right ArchitectureVariableBinding) int {
		if order := strings.Compare(left.CandidateFingerprint, right.CandidateFingerprint); order != 0 {
			return order
		}
		return strings.Compare(left.VariableID, right.VariableID)
	})
	slices.SortStableFunc(diagnostics, compareClosedLoopDiagnostic)
	if len(diagnostics) != 0 {
		return nil, nil, diagnostics
	}
	return sets, bindings, nil
}

func compareClosedLoopDiagnostic(left, right closedloopsynthesis.Diagnostic) int {
	if order := strings.Compare(left.Path, right.Path); order != 0 {
		return order
	}
	return strings.Compare(left.Message, right.Message)
}

func closedLoopReportIssues(prefix string, diagnostics []closedloopsynthesis.Diagnostic) []reports.Issue {
	issues := make([]reports.Issue, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		path := prefix
		if diagnostic.Path != "" {
			path += "." + diagnostic.Path
		}
		issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
	}
	return issues
}

type resolvedArchitectureCandidate struct {
	Lowered         Result
	Resolved        circuitgraph.ResolvedDocument
	SynthesisReport circuitgraph.SynthesisReport
}

// BuildClosedLoopInput carries every retained materially distinct search
// candidate into behavioral evaluation. It preserves the architecture score,
// rejects stale requirement/search pairings, and scopes trusted repair
// variables by candidate fingerprint.
func BuildClosedLoopInput(requirement architecturesearch.Requirement, search architecturesearch.SearchResult, modelRegistryHash string, variableSets []ClosedLoopCandidateVariables) (closedloopsynthesis.Input, []closedloopsynthesis.Diagnostic) {
	input := closedloopsynthesis.Input{
		Requirement: requirement, CatalogHash: search.CatalogHash, FormulaLibraryHash: search.FormulaLibraryHash, ModelRegistryHash: modelRegistryHash,
	}
	var diagnostics []closedloopsynthesis.Diagnostic
	if requirement.Schema != architecturesearch.SchemaIDV3 || requirement.Version != architecturesearch.VersionV3 {
		diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: "requirement", Message: "behavioral closed-loop integration requires a v3 requirement"})
	}
	requirementHash, err := architecturesearch.CanonicalHash(architecturesearch.Normalize(requirement))
	if err != nil || requirementHash != search.RequirementHash {
		diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: "search.requirement_hash", Message: "architecture search is stale or belongs to a different requirement"})
	}
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: "search", Message: "closed-loop integration requires retained complete architecture candidates"})
	}
	variablesByFingerprint := map[string][]closedloopsynthesis.Variable{}
	for index, set := range variableSets {
		if set.Fingerprint == "" || variablesByFingerprint[set.Fingerprint] != nil {
			diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: fmt.Sprintf("variable_sets[%d]", index), Message: "candidate variable set is duplicate or lacks a fingerprint"})
			continue
		}
		variablesByFingerprint[set.Fingerprint] = set.Variables
	}
	retained := []architecturesearch.CandidateResult{}
	if search.Selected != nil {
		retained = append(retained, *search.Selected)
	}
	retained = append(retained, search.Alternatives...)
	slices.SortStableFunc(retained, func(left, right architecturesearch.CandidateResult) int {
		return strings.Compare(left.Fingerprint, right.Fingerprint)
	})
	seen := map[string]bool{}
	for index, candidate := range retained {
		if !validClosedLoopHash(candidate.Fingerprint) || seen[candidate.Fingerprint] {
			diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: fmt.Sprintf("search.candidates[%d].fingerprint", index), Message: "retained candidate fingerprint is invalid or duplicated"})
			continue
		}
		seen[candidate.Fingerprint] = true
		input.Candidates = append(input.Candidates, closedloopsynthesis.Candidate{Fingerprint: candidate.Fingerprint, Score: candidate.Score, Variables: variablesByFingerprint[candidate.Fingerprint]})
		delete(variablesByFingerprint, candidate.Fingerprint)
	}
	if len(variablesByFingerprint) != 0 {
		diagnostics = append(diagnostics, closedloopsynthesis.Diagnostic{Path: "variable_sets", Message: "candidate variable set references an architecture that was not retained"})
	}
	slices.SortStableFunc(diagnostics, func(left, right closedloopsynthesis.Diagnostic) int {
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		return strings.Compare(left.Message, right.Message)
	})
	if len(diagnostics) != 0 {
		return closedloopsynthesis.Input{}, diagnostics
	}
	return input, nil
}

func (resolver ArchitectureSimulationPlanResolver) ResolveSimulationPlans(ctx context.Context, state closedloopsynthesis.CandidateState) (map[string]simmodel.Plan, error) {
	resolvedCandidate, err := resolver.resolveArchitectureCandidate(ctx, state)
	if err != nil {
		return nil, err
	}
	lowered := resolvedCandidate.Lowered
	resolved := resolvedCandidate.Resolved
	synthesisReport := resolvedCandidate.SynthesisReport
	primaryInputNode, _ := behavioralPrimaryInputNode(resolver.Requirement, lowered.Evidence.SemanticBindings)
	transientStimulus, hasTransientStimulus := behavioralTransientStimulusForRequirement(resolver.Requirement, lowered.Evidence.SemanticBindings)
	harness, err := operatingHarnessDevices(resolver.Requirement, lowered.Evidence.SemanticBindings, resolved.Simulation, "")
	if err != nil {
		return nil, fmt.Errorf("resolve operating harness: %w", err)
	}
	dcHarness, err := operatingHarnessDevices(resolver.Requirement, lowered.Evidence.SemanticBindings, resolved.Simulation, simmodel.AnalysisDCOperatingPoint)
	if err != nil {
		return nil, fmt.Errorf("resolve DC operating harness: %w", err)
	}
	startupHarness, err := operatingHarnessDevices(resolver.Requirement, lowered.Evidence.SemanticBindings, resolved.Simulation, simmodel.AnalysisStartup)
	if err != nil {
		return nil, fmt.Errorf("resolve startup operating harness: %w", err)
	}
	var inputStimulusHarness *operatingHarnessDevice
	if hasTransientStimulus && (resolved.Simulation == nil || !planHasVoltageSourceAtNode(*resolved.Simulation, transientStimulus.Node)) {
		stimulusHarness, harnessErr := transientStimulusHarnessDevice(resolver.Requirement, lowered.Evidence.SemanticBindings, transientStimulus)
		if harnessErr != nil {
			return nil, fmt.Errorf("resolve transient stimulus harness: %w", harnessErr)
		}
		inputStimulusHarness = &stimulusHarness
	}
	resolveWorkflow := func(kind string, intent simmodel.Intent) (simmodel.Plan, []reports.Issue) {
		selectedHarness := operatingHarnessForAnalysis(kind, harness, dcHarness, startupHarness)
		workflowHarness := append([]operatingHarnessDevice(nil), selectedHarness...)
		if analysisUsesBehavioralInputStimulus(kind) && inputStimulusHarness != nil {
			workflowHarness = append(workflowHarness, *inputStimulusHarness)
			slices.SortStableFunc(workflowHarness, func(left, right operatingHarnessDevice) int {
				return strings.Compare(left.Device.InstanceID, right.Device.InstanceID)
			})
		}
		addOperatingHarnessExcitations(&intent, workflowHarness)
		devices := make([]circuitgraph.SimulationHarnessDevice, 0, len(workflowHarness))
		for _, entry := range workflowHarness {
			devices = append(devices, entry.Device)
		}
		return resolver.GraphResolver.ResolveSimulationPlanWithHarness(intent, resolved, devices)
	}
	inputStimulusComponent := ""
	if inputStimulusHarness != nil {
		inputStimulusComponent = inputStimulusHarness.Device.InstanceID
	}
	if resolved.Simulation == nil && len(resolver.BaseIntents) == 0 {
		reason := "no synthesis simulation evidence"
		if synthesisReport.Simulation.Reason != "" {
			reason = synthesisReport.Simulation.Reason
		} else if resolved.Synthesis != nil && resolved.Synthesis.Simulation.Reason != "" {
			reason = resolved.Synthesis.Simulation.Reason
		}
		return nil, fmt.Errorf("resolved architecture has no complete trusted graph simulation plan: %s", reason)
	}
	kinds := append([]string(nil), resolver.RequiredAnalyses...)
	if len(kinds) == 0 {
		kinds = requiredBehavioralAnalyses(resolver.Requirement)
	}
	slices.Sort(kinds)
	result := make(map[string]simmodel.Plan, len(kinds))
	centeredBiasCache := make(map[string]simmodel.CenteredBiasSelection)
	recordPlan := func(kind string, plan simmodel.Plan) error {
		plan = planScopedToAnalysis(plan, kind)
		if kind == simmodel.AnalysisACSweep && len(plan.Analyses) == 1 {
			plan.Analyses[0] = behavioralBoundedACSweep(resolver.Requirement, modelBoundedACSweep(plan, plan.Analyses[0]))
		}
		centered, centerErr := centerBehavioralInputBiasCached(plan, kind, primaryInputNode, centeredBiasCache)
		if centerErr != nil {
			return centerErr
		}
		normalMute, _, hasMuteControl := closedloopsynthesis.ResolveMuteExcitationStates(resolver.Requirement, lowered.Evidence.SemanticBindings, centered)
		defaulted, defaultErr := configureBehavioralMuteState(centered, kind, normalMute, hasMuteControl)
		if defaultErr != nil {
			return defaultErr
		}
		stimulated, stimulusErr := configureBehavioralTransientStimulus(defaulted, kind, transientStimulus, hasTransientStimulus)
		if stimulusErr != nil {
			return stimulusErr
		}
		result[kind] = stimulated
		return nil
	}
	previous := ""
	for _, kind := range kinds {
		if kind == previous {
			return nil, fmt.Errorf("required analysis kinds must be unique")
		}
		previous = kind
		if intent, exists := resolver.BaseIntents[kind]; exists {
			plan, issues := resolveWorkflow(kind, intent)
			if reports.HasBlockingIssue(issues) {
				return nil, fmt.Errorf("resolve %s simulation workflow: %s", kind, joinReportIssues(issues))
			}
			if !simmodel.SupportsAnalysis(plan.ModelID, kind) {
				return nil, fmt.Errorf("trusted base intent model %s does not execute required analysis %s", plan.ModelID, kind)
			}
			if err := recordPlan(kind, plan); err != nil {
				return nil, fmt.Errorf("center %s simulation workflow: %w", kind, err)
			}
			continue
		}
		if resolved.Simulation == nil || !simmodel.SupportsAnalysis(resolved.Simulation.ModelID, kind) {
			if resolved.Simulation == nil {
				return nil, fmt.Errorf("resolved architecture has no trusted base intent for required analysis %s", kind)
			}
			intent, deriveErr := derivedGraphWorkflowIntent(resolver.Requirement, *resolved.Simulation, kind, primaryInputNode, inputStimulusComponent)
			if deriveErr != nil {
				return nil, fmt.Errorf("derive %s simulation workflow: %w", kind, deriveErr)
			}
			plan, issues := resolveWorkflow(kind, intent)
			if reports.HasBlockingIssue(issues) {
				return nil, fmt.Errorf("resolve derived %s simulation workflow: %s", kind, joinReportIssues(issues))
			}
			if !simmodel.SupportsAnalysis(plan.ModelID, kind) {
				return nil, fmt.Errorf("derived model %s does not execute required analysis %s", plan.ModelID, kind)
			}
			if err := recordPlan(kind, plan); err != nil {
				return nil, fmt.Errorf("center %s simulation workflow: %w", kind, err)
			}
			continue
		}
		if len(harness) != 0 {
			intent, deriveErr := derivedGraphWorkflowIntent(resolver.Requirement, *resolved.Simulation, kind, primaryInputNode, inputStimulusComponent)
			if deriveErr != nil {
				return nil, fmt.Errorf("derive harnessed %s simulation workflow: %w", kind, deriveErr)
			}
			plan, issues := resolveWorkflow(kind, intent)
			if reports.HasBlockingIssue(issues) {
				return nil, fmt.Errorf("resolve harnessed %s simulation workflow: %s", kind, joinReportIssues(issues))
			}
			if err := recordPlan(kind, plan); err != nil {
				return nil, fmt.Errorf("center %s simulation workflow: %w", kind, err)
			}
			continue
		}
		if err := recordPlan(kind, *resolved.Simulation); err != nil {
			return nil, fmt.Errorf("center %s simulation workflow: %w", kind, err)
		}
	}
	return result, nil
}

func (resolver ArchitectureSimulationPlanResolver) resolveArchitectureCandidate(ctx context.Context, state closedloopsynthesis.CandidateState) (resolvedArchitectureCandidate, error) {
	if resolver.GraphResolver == nil {
		return resolvedArchitectureCandidate{}, fmt.Errorf("circuit-graph resolver is required")
	}
	candidate, ok := retainedArchitectureCandidate(resolver.Search, state.Fingerprint)
	if !ok {
		return resolvedArchitectureCandidate{}, fmt.Errorf("candidate fingerprint is absent from the retained architecture search")
	}
	search := resolver.Search
	search.Status = architecturesearch.SearchSelected
	search.Selected = &candidate
	search.Alternatives = nil
	search.Rationale = nil
	lowered, loweringIssues := Lower(resolver.Requirement, search)
	if reports.HasBlockingIssue(loweringIssues) {
		return resolvedArchitectureCandidate{}, fmt.Errorf("lower architecture candidate: %s", joinReportIssues(loweringIssues))
	}
	bindings, err := architectureBindingsForState(state, resolver.VariableBindings)
	if err != nil {
		return resolvedArchitectureCandidate{}, err
	}
	document := lowered.Document
	if err := applyArchitectureFunctionVariables(&document, state, bindings); err != nil {
		return resolvedArchitectureCandidate{}, err
	}
	synthesized, synthesisReport, synthesisIssues := resolver.GraphResolver.Synthesize(ctx, document)
	if reports.HasBlockingIssue(synthesisIssues) {
		return resolvedArchitectureCandidate{}, fmt.Errorf("synthesize repaired architecture candidate: %s", joinReportIssues(synthesisIssues))
	}
	if err := applyArchitectureComponentVariables(&synthesized, state, bindings); err != nil {
		return resolvedArchitectureCandidate{}, err
	}
	resolved, resolutionIssues := resolver.GraphResolver.Resolve(ctx, synthesized)
	if reports.HasBlockingIssue(resolutionIssues) {
		return resolvedArchitectureCandidate{}, fmt.Errorf("resolve repaired architecture candidate: %s", joinReportIssues(resolutionIssues))
	}
	return resolvedArchitectureCandidate{Lowered: lowered, Resolved: resolved, SynthesisReport: synthesisReport}, nil
}

func centerBehavioralInputBias(plan simmodel.Plan, kind, primaryInputNode string) (simmodel.Plan, error) {
	return centerBehavioralInputBiasCached(plan, kind, primaryInputNode, nil)
}

func centerBehavioralInputBiasCached(plan simmodel.Plan, kind, primaryInputNode string, cache map[string]simmodel.CenteredBiasSelection) (simmodel.Plan, error) {
	if primaryInputNode == "" {
		return plan, nil
	}
	switch kind {
	case simmodel.AnalysisDCOperatingPoint, simmodel.AnalysisACSweep, simmodel.AnalysisNoise, simmodel.AnalysisStability, simmodel.AnalysisTransient, simmodel.AnalysisDistortion, simmodel.AnalysisThermal:
	default:
		return plan, nil
	}
	for _, analysis := range plan.Analyses {
		if analysis.DCSweep != nil {
			return plan, nil
		}
	}
	hasOpAmp := false
	for _, device := range plan.Devices {
		if device.PrimitiveModel == simmodel.PrimitiveOpAmpV1 {
			hasOpAmp = true
			break
		}
	}
	if !hasOpAmp && kind != simmodel.AnalysisThermal {
		return plan, nil
	}
	source, ok := uniquePlanSourceAtNode(plan, primaryInputNode)
	if !ok {
		return plan, nil
	}
	cacheKey, cacheable := behavioralCenteredBiasCacheKey(plan, source, kind == simmodel.AnalysisThermal)
	selection, cached := cache[cacheKey]
	var diagnostics []simmodel.Diagnostic
	if !cacheable || !cached {
		selection, diagnostics = simmodel.SelectCenteredSourceBias(plan, source, primaryInputNode)
		if kind == simmodel.AnalysisThermal {
			selection, diagnostics = simmodel.SelectThermallyFeasibleSourceBias(plan, source, primaryInputNode)
		}
		if len(diagnostics) == 0 && cacheable && cache != nil {
			cache[cacheKey] = selection
		}
	}
	if len(diagnostics) != 0 {
		return simmodel.Plan{}, fmt.Errorf("%s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	clone := simmodel.ClonePlan(plan)
	initialV, finalV := selection.ValueV*.95, selection.ValueV*1.05
	for analysisIndex := range clone.Analyses {
		for excitationIndex := range clone.Analyses[analysisIndex].Excitations {
			excitation := &clone.Analyses[analysisIndex].Excitations[excitationIndex]
			if excitation.Component != source {
				continue
			}
			if kind == simmodel.AnalysisTransient {
				excitation.DCValue = 0
				excitation.PulseInitialValue = initialV
				excitation.PulseValue = finalV
				excitation.PulseDelayS = clone.Analyses[analysisIndex].TimeStepS
				// Keep exactly one commanded edge inside the observation window.
				// The deterministic return edge occurs one timestep after the
				// final sample and cannot contaminate settling/swing evidence.
				excitation.PulseWidthS = clone.Analyses[analysisIndex].DurationS
				excitation.PulsePeriodS = clone.Analyses[analysisIndex].DurationS * 2
				continue
			}
			excitation.DCValue = selection.ValueV
		}
	}
	return clone, nil
}

func behavioralCenteredBiasCacheKey(plan simmodel.Plan, source string, thermal bool) (string, bool) {
	if len(plan.Analyses) != 1 {
		return "", false
	}
	clone := simmodel.ClonePlan(plan)
	clone.Assertions = nil
	analysis := clone.Analyses[0]
	analysis.ID = ""
	analysis.Kind = simmodel.AnalysisDCOperatingPoint
	analysis.StartFrequencyHz = 0
	analysis.StopFrequencyHz = 0
	analysis.Points = 0
	analysis.DurationS = 0
	analysis.TimeStepS = 0
	analysis.DCSweep = nil
	for index := range analysis.Excitations {
		excitation := &analysis.Excitations[index]
		if excitation.Component == source {
			excitation.DCValue = 0
		}
		excitation.ACMagnitude = 0
		excitation.ACPhaseDeg = 0
		excitation.PulseInitialValue = 0
		excitation.PulseValue = 0
		excitation.PulseDelayS = 0
		excitation.PulseWidthS = 0
		excitation.PulsePeriodS = 0
		excitation.SineAmplitude = 0
		excitation.SineFrequencyHz = 0
		excitation.SinePhaseDeg = 0
	}
	clone.Analyses = []simmodel.Analysis{analysis}
	encoded, err := json.Marshal(clone)
	if err != nil {
		return "", false
	}
	domain := "electrical\x00"
	if thermal {
		domain = "thermal\x00"
	}
	digest := sha256.Sum256(append([]byte(domain), encoded...))
	return hex.EncodeToString(digest[:]), true
}

type behavioralTransientStimulus struct {
	Node                string
	InitialV            float64
	FinalV              float64
	SemanticID          string
	Periodic            bool
	PeriodicFrequencyHz float64
}

func configureBehavioralMuteState(plan simmodel.Plan, kind string, control closedloopsynthesis.SimulationExcitationOverride, available bool) (simmodel.Plan, error) {
	if !available || kind == simmodel.AnalysisStartup {
		return plan, nil
	}
	for analysisIndex := range plan.Analyses {
		found := false
		for excitationIndex := range plan.Analyses[analysisIndex].Excitations {
			if plan.Analyses[analysisIndex].Excitations[excitationIndex].Component == control.Component {
				plan.Analyses[analysisIndex].Excitations[excitationIndex].DCValue = control.DCValue
				found = true
			}
		}
		if !found {
			return simmodel.Plan{}, fmt.Errorf("mute-state source %q is absent from analysis %q", control.Component, plan.Analyses[analysisIndex].ID)
		}
	}
	return plan, nil
}

func behavioralTransientStimulusForRequirement(requirement architecturesearch.Requirement, bindings []closedloopsynthesis.SemanticBinding) (behavioralTransientStimulus, bool) {
	requiresStimulus := false
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if analysisUsesBehavioralInputStimulus(behavior.Analysis) {
			requiresStimulus = true
			break
		}
	}
	if !requiresStimulus {
		return behavioralTransientStimulus{}, false
	}
	port, node, ok := behavioralPrimaryInputPort(requirement, bindings)
	if !ok {
		return behavioralTransientStimulus{}, false
	}
	initialV, finalV := 0.0, 0.0
	thresholdBounded := false
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Metric != "threshold_voltage" || behavior.Min == nil || behavior.Max == nil || !finiteArchitectureValue(*behavior.Min) || !finiteArchitectureValue(*behavior.Max) || *behavior.Min > *behavior.Max {
			continue
		}
		scale := math.Max(math.Abs(*behavior.Min), math.Abs(*behavior.Max))
		margin := math.Max(*behavior.Max-*behavior.Min, math.Max(scale*.1, 1e-3))
		initialV = *behavior.Min - margin
		finalV = *behavior.Max + margin
		if initialV < 0 && *behavior.Min >= 0 {
			initialV = 0
		}
		thresholdBounded = true
		break
	}
	if !thresholdBounded {
		domains := make(map[string]architecturesearch.Domain, len(requirement.Requirements.Domains))
		for _, domain := range requirement.Requirements.Domains {
			domains[domain.ID] = domain
		}
		finalV = domains[port.Domain].NominalVoltageV
		if port.Electrical != nil && port.Electrical.NominalVoltageV != nil {
			finalV = *port.Electrical.NominalVoltageV
		}
		if finalV == 0 {
			finalV, _ = behavioralOutputSpanInputAmplitude(requirement)
		}
		periodic := false
		for _, behavior := range requirement.Requirements.BehavioralRequirements {
			if (behavior.Metric == "output_swing" || behavior.Metric == "output_power") && behavior.Analysis == simmodel.AnalysisTransient {
				initialV = -finalV
				periodic = true
				break
			}
		}
		if periodic {
			for _, behavior := range requirement.Requirements.BehavioralRequirements {
				if behavior.Analysis == simmodel.AnalysisTransient && behavior.Metric != "output_swing" && behavior.Metric != "output_power" && behavior.Metric != "muted_output_voltage" {
					periodic = false
					break
				}
			}
		}
		if periodic {
			return behavioralTransientStimulus{Node: node, InitialV: initialV, FinalV: finalV, SemanticID: port.ID, Periodic: true, PeriodicFrequencyHz: behavioralDistortionFrequency(requirement)}, true
		}
	}
	if !finiteArchitectureValue(initialV) || !finiteArchitectureValue(finalV) || initialV == finalV {
		return behavioralTransientStimulus{}, false
	}
	return behavioralTransientStimulus{Node: node, InitialV: initialV, FinalV: finalV, SemanticID: port.ID}, true
}

func configureBehavioralTransientStimulus(plan simmodel.Plan, kind string, stimulus behavioralTransientStimulus, available bool) (simmodel.Plan, error) {
	if kind != simmodel.AnalysisTransient || !available {
		return plan, nil
	}
	source, ok := uniquePlanSourceAtNode(plan, stimulus.Node)
	if !ok {
		return simmodel.Plan{}, fmt.Errorf("transient stimulus %q requires exactly one catalog-backed source at its resolved semantic input", stimulus.SemanticID)
	}
	polarity, ok := voltageSourcePolarityAtNode(plan, source, stimulus.Node)
	if !ok {
		return simmodel.Plan{}, fmt.Errorf("transient stimulus %q requires a catalog-backed voltage source at its resolved semantic input", stimulus.SemanticID)
	}
	clone := simmodel.ClonePlan(plan)
	initialV, finalV, diagnostics := simmodel.SelectFeasibleTransientSourceEdge(clone, source, stimulus.InitialV*polarity, stimulus.FinalV*polarity)
	if len(diagnostics) != 0 {
		return simmodel.Plan{}, fmt.Errorf("%s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	for analysisIndex := range clone.Analyses {
		analysis := &clone.Analyses[analysisIndex]
		for excitationIndex := range analysis.Excitations {
			excitation := &analysis.Excitations[excitationIndex]
			if excitation.Component != source {
				continue
			}
			if stimulus.Periodic {
				if !finiteArchitectureValue(stimulus.PeriodicFrequencyHz) || stimulus.PeriodicFrequencyHz <= 0 {
					return simmodel.Plan{}, fmt.Errorf("transient stimulus %q requires a bounded periodic frequency", stimulus.SemanticID)
				}
				component := excitation.Component
				*excitation = simmodel.SourceExcitation{
					Component: component, DCValue: (initialV + finalV) / 2,
					SineAmplitude: math.Abs(finalV-initialV) / 2, SineFrequencyHz: stimulus.PeriodicFrequencyHz,
				}
				if polarity < 0 {
					excitation.SinePhaseDeg = 180
				}
				return clone, nil
			}
			excitation.DCValue = 0
			excitation.PulseInitialValue = initialV
			excitation.PulseValue = finalV
			excitation.PulseDelayS = analysis.TimeStepS
			excitation.PulseWidthS = analysis.DurationS
			excitation.PulsePeriodS = analysis.DurationS * 2
			return clone, nil
		}
	}
	return simmodel.Plan{}, fmt.Errorf("transient stimulus %q has no resolved source excitation", stimulus.SemanticID)
}

func voltageSourcePolarityAtNode(plan simmodel.Plan, component, node string) (float64, bool) {
	for _, device := range plan.Devices {
		if device.Component != component {
			continue
		}
		terminals := make(map[string]string, len(device.Terminals))
		for _, terminal := range device.Terminals {
			terminals[terminal.Terminal] = terminal.Net
		}
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1:
			if terminals["POSITIVE"] == node {
				return 1, true
			}
			if terminals["NEGATIVE"] == node {
				return -1, true
			}
		case simmodel.PrimitiveConnectorVoltageSourceV1:
			if terminals["PIN_1"] == node {
				return 1, true
			}
			if terminals["PIN_2"] == node {
				return -1, true
			}
		}
		return 0, false
	}
	return 0, false
}

func planHasVoltageSourceAtNode(plan simmodel.Plan, node string) bool {
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1, simmodel.PrimitiveConnectorVoltageSourceV1:
		default:
			continue
		}
		for _, terminal := range device.Terminals {
			if terminal.Net == node {
				return true
			}
		}
	}
	return false
}

func transientStimulusHarnessDevice(requirement architecturesearch.Requirement, bindings []closedloopsynthesis.SemanticBinding, stimulus behavioralTransientStimulus) (operatingHarnessDevice, error) {
	ground := ""
	referenceID := firstReferenceDomain(requirement)
	for _, binding := range bindings {
		if binding.Kind == "domain" && binding.ID == referenceID {
			ground = binding.Target
			break
		}
	}
	if ground == "" || ground == stimulus.Node {
		return operatingHarnessDevice{}, fmt.Errorf("semantic transient stimulus requires one distinct resolved reference domain")
	}
	return operatingHarnessDevice{
		Source: true,
		Device: circuitgraph.SimulationHarnessDevice{
			InstanceID: "analysis_transient_stimulus_" + stimulus.SemanticID,
			CatalogID:  "source.voltage.connector.1x02",
			Connections: []simmodel.ConnectionEvidence{
				{Function: "POSITIVE", Net: stimulus.Node},
				{Function: "NEGATIVE", Net: ground},
			},
		},
	}, nil
}

func operatingHarnessForAnalysis(kind string, ordinary, dc, startup []operatingHarnessDevice) []operatingHarnessDevice {
	switch kind {
	case simmodel.AnalysisDCOperatingPoint:
		return dc
	case simmodel.AnalysisStartup:
		return startup
	default:
		return ordinary
	}
}

func analysisUsesBehavioralInputStimulus(kind string) bool {
	switch kind {
	case simmodel.AnalysisACSweep, simmodel.AnalysisTransient, simmodel.AnalysisDistortion:
		return true
	default:
		return false
	}
}

type operatingHarnessDevice struct {
	Device          circuitgraph.SimulationHarnessDevice
	Source          bool
	DefaultValue    float64
	HasDefaultValue bool
}

func operatingHarnessDevices(requirement architecturesearch.Requirement, bindings []closedloopsynthesis.SemanticBinding, resolvedPlan *simmodel.Plan, analysisKind string) ([]operatingHarnessDevice, error) {
	loadConditions := make([]architecturesearch.OperatingCondition, 0)
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			switch condition.Axis {
			case "load_current", "load_resistance", "load_capacitance":
				loadConditions = append(loadConditions, condition)
			}
		}
	}
	targets := map[string]string{}
	for _, binding := range bindings {
		targets[binding.Kind+"\x00"+binding.ID] = binding.Target
	}
	ground := targets["domain\x00"+firstReferenceDomain(requirement)]
	controlHarness, err := participantControlOutputHarnessDevices(requirement, targets, ground, resolvedPlan, analysisKind)
	if err != nil {
		return nil, err
	}
	if len(loadConditions) == 0 {
		return controlHarness, nil
	}
	if ground == "" {
		return nil, fmt.Errorf("catalog-backed operating loads require one resolved reference domain")
	}
	seen := map[string]bool{}
	result := append([]operatingHarnessDevice(nil), controlHarness...)
	for _, entry := range result {
		seen[entry.Device.InstanceID] = true
	}
	for _, condition := range loadConditions {
		var catalogID string
		var terminals [2]string
		source := false
		startupLoadResistance := 0.0
		switch condition.Axis {
		case "load_current":
			catalogID, terminals, source = "source.current.connector.1x02", [2]string{"POSITIVE", "NEGATIVE"}, true
			if analysisKind == simmodel.AnalysisStartup {
				var resistanceErr error
				startupLoadResistance, resistanceErr = startupLoadResistanceOhm(requirement, condition)
				if resistanceErr != nil {
					return nil, resistanceErr
				}
				catalogID, terminals, source = "resistor.generic.0603", [2]string{"A", "B"}, false
			}
		case "load_resistance":
			catalogID, terminals = "resistor.generic.0603", [2]string{"A", "B"}
		case "load_capacitance":
			catalogID, terminals = "capacitor.ceramic.0603", [2]string{"A", "B"}
		default:
			continue
		}
		target, ok := operatingConditionSemanticTarget(condition, targets)
		if !ok || target == ground {
			return nil, fmt.Errorf("%s target %q does not resolve to a non-reference semantic net", condition.Axis, condition.Target)
		}
		instanceID := closedloopsynthesis.OperatingHarnessComponentID(condition.Axis, target)
		if seen[instanceID] {
			continue
		}
		seen[instanceID] = true
		positive, negative := target, ground
		if condition.Axis == "load_current" {
			var endpointErr error
			positive, negative, endpointErr = loadCurrentHarnessEndpoints(requirement, condition, targets, target, ground, resolvedPlan, analysisKind)
			if endpointErr != nil {
				return nil, endpointErr
			}
		}
		device := circuitgraph.SimulationHarnessDevice{
			InstanceID: instanceID, CatalogID: catalogID,
			Connections: []simmodel.ConnectionEvidence{{Function: terminals[0], Net: positive}, {Function: terminals[1], Net: negative}},
		}
		if !source {
			value, valueOK := startupLoadResistance, startupLoadResistance > 0
			if !valueOK {
				value, valueOK = positiveOperatingValue(condition)
			}
			if !valueOK {
				return nil, fmt.Errorf("%s target %q requires at least one positive bounded load value", condition.Axis, condition.Target)
			}
			device.ValueSI, device.HasValueSI = value, true
		}
		entry := operatingHarnessDevice{Device: device, Source: source}
		if condition.Axis == "load_current" {
			entry.DefaultValue, entry.HasDefaultValue = maximumPositiveOperatingValue(condition)
		}
		result = append(result, entry)
	}
	slices.SortStableFunc(result, func(left, right operatingHarnessDevice) int {
		return strings.Compare(left.Device.InstanceID, right.Device.InstanceID)
	})
	return result, nil
}

func participantControlOutputHarnessDevices(requirement architecturesearch.Requirement, targets map[string]string, ground string, resolvedPlan *simmodel.Plan, analysisKind string) ([]operatingHarnessDevice, error) {
	participants := map[string]architecturesearch.Participant{}
	for _, participant := range requirement.Requirements.Participants {
		participants[participant.ID] = participant
	}
	domains := map[string]architecturesearch.Domain{}
	for _, domain := range requirement.Requirements.Domains {
		domains[domain.ID] = domain
	}
	seen := map[string]bool{}
	var result []operatingHarnessDevice
	for _, objective := range requirement.Requirements.Objectives {
		for _, binding := range objective.Bindings {
			if binding.Role != "control" || binding.Participant == "" || binding.ParticipantPort == "" {
				continue
			}
			participant, ok := participants[binding.Participant]
			if !ok {
				return nil, fmt.Errorf("control binding references unknown participant %q", binding.Participant)
			}
			var port architecturesearch.ParticipantPort
			found := false
			for _, candidate := range participant.RequiredPorts {
				if candidate.ID == binding.ParticipantPort {
					port, found = candidate, true
					break
				}
			}
			if !found || port.Direction != "source" || (port.Kind != "digital_logic" && port.Kind != "analog_control") {
				continue
			}
			if port.Protocol != nil && strings.TrimSpace(port.Protocol.Mode) != "" && !strings.EqualFold(strings.TrimSpace(port.Protocol.Mode), "push_pull") {
				continue
			}
			if ground == "" {
				return nil, fmt.Errorf("participant control-output harness requires one resolved reference domain")
			}
			semanticID := participant.ID + "." + port.ID
			target := targets["participant_port\x00"+semanticID]
			if target == "" || target == ground {
				return nil, fmt.Errorf("participant control output %q does not resolve to a non-reference semantic net", semanticID)
			}
			if resolvedPlan != nil && planHasVoltageSourceAtNode(*resolvedPlan, target) {
				continue
			}
			instanceID := closedloopsynthesis.OperatingHarnessComponentID("participant_output", semanticID)
			if seen[instanceID] {
				continue
			}
			high := domains[participant.Domain].NominalVoltageV
			if !finiteArchitectureValue(high) || high <= 0 {
				return nil, fmt.Errorf("participant control output %q requires a positive nominal domain voltage", semanticID)
			}
			if analysisKind == simmodel.AnalysisStartup {
				high = 0
			}
			seen[instanceID] = true
			result = append(result, operatingHarnessDevice{
				Source: true, DefaultValue: high, HasDefaultValue: true,
				Device: circuitgraph.SimulationHarnessDevice{
					InstanceID: instanceID,
					CatalogID:  "source.voltage.connector.1x02",
					Connections: []simmodel.ConnectionEvidence{
						{Function: "POSITIVE", Net: target},
						{Function: "NEGATIVE", Net: ground},
					},
				},
			})
		}
	}
	slices.SortStableFunc(result, func(left, right operatingHarnessDevice) int {
		return strings.Compare(left.Device.InstanceID, right.Device.InstanceID)
	})
	return result, nil
}

func startupLoadResistanceOhm(requirement architecturesearch.Requirement, condition architecturesearch.OperatingCondition) (float64, error) {
	current, ok := maximumPositiveOperatingValue(condition)
	if !ok {
		return 0, fmt.Errorf("startup load-current target %q requires a positive maximum current", condition.Target)
	}
	powerPort := ""
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "load_switch" {
			continue
		}
		matchesOutput := false
		candidatePower := ""
		for _, binding := range objective.Bindings {
			matchesOutput = matchesOutput || ((binding.Role == "output" || binding.Role == "load") && binding.Port == condition.Target)
			if (binding.Role == "power" || binding.Role == "load_power") && binding.Port != "" {
				candidatePower = binding.Port
			}
		}
		if matchesOutput && candidatePower != "" {
			powerPort = candidatePower
			break
		}
	}
	domainID := ""
	if powerPort == "" {
		// A startup current attached directly to a powered port is an ordinary
		// voltage-dependent load. The public target may name its domain, a port,
		// or a signal. Load-switch objectives may instead identify a distinct
		// upstream power port.
		domainID = condition.Target
		if port, found := requirementPortByID(requirement, condition.Target); found {
			domainID = port.Domain
		} else {
			for _, signal := range requirement.Requirements.Signals {
				if signal.ID == condition.Target {
					domainID = signal.Domain
					break
				}
			}
		}
	} else {
		port, found := requirementPortByID(requirement, powerPort)
		if !found {
			return 0, fmt.Errorf("startup load-current target %q requires a semantic load-switch power port with a bounded nominal-voltage domain", condition.Target)
		}
		domainID = port.Domain
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == domainID && finiteArchitectureValue(domain.NominalVoltageV) && math.Abs(domain.NominalVoltageV) > 0 {
			return math.Abs(domain.NominalVoltageV) / current, nil
		}
	}
	return 0, fmt.Errorf("startup load-current target %q requires a positive nominal load-supply voltage", condition.Target)
}

func requirementPortByID(requirement architecturesearch.Requirement, id string) (architecturesearch.Port, bool) {
	for _, port := range requirement.Requirements.Ports {
		if port.ID == id {
			return port, true
		}
	}
	return architecturesearch.Port{}, false
}

// loadCurrentHarnessEndpoints models the external load implied by a semantic
// load-switch objective. A low-side switch's power and output roles bound the
// two load terminals; other current loads retain the ordinary output-to-
// reference testbench connection. This is derived from public capability roles
// and never from selected topology, part identity, or fixture names.
func loadCurrentHarnessEndpoints(requirement architecturesearch.Requirement, condition architecturesearch.OperatingCondition, targets map[string]string, target, ground string, plan *simmodel.Plan, analysisKind string) (string, string, error) {
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "load_switch" {
			continue
		}
		matchesOutput := false
		powerPort := ""
		for _, binding := range objective.Bindings {
			if (binding.Role == "output" || binding.Role == "load") && binding.Port == condition.Target {
				matchesOutput = true
			}
			if (binding.Role == "power" || binding.Role == "load_power") && binding.Port != "" {
				powerPort = binding.Port
			}
		}
		if matchesOutput && powerPort != "" {
			if power := targets["port\x00"+powerPort]; power != "" && power != target {
				sensedPower, err := loadCurrentSenseDownstreamNode(requirement, powerPort, power, plan)
				if err != nil {
					return "", "", err
				}
				if analysisKind == simmodel.AnalysisDCOperatingPoint && sensedPower != power {
					// A trip-threshold sweep imposes current through the sensing
					// element independently of the controlled actuator. Otherwise
					// opening the switch removes the sweep stimulus and no stable DC
					// threshold can exist. Dynamic analyses retain the physical load
					// between supply and switched output.
					return sensedPower, ground, nil
				}
				if highSideSwitchSpansNodes(plan, sensedPower, target) {
					return target, ground, nil
				}
				return sensedPower, target, nil
			}
		}
	}
	return target, ground, nil
}

func highSideSwitchSpansNodes(plan *simmodel.Plan, power, output string) bool {
	if plan == nil {
		return false
	}
	for _, device := range plan.Devices {
		if device.PrimitiveModel != simmodel.PrimitivePMOSSwitchV1 {
			continue
		}
		terminals := map[string]string{}
		for _, terminal := range device.Terminals {
			terminals[terminal.Terminal] = terminal.Net
		}
		if terminals["SOURCE"] == power && terminals["DRAIN"] == output {
			return true
		}
	}
	return false
}

func loadCurrentSenseDownstreamNode(requirement architecturesearch.Requirement, powerPort, externalPower string, plan *simmodel.Plan) (string, error) {
	requiresSensedPath := false
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "current_sensing" {
			continue
		}
		for _, binding := range objective.Bindings {
			if binding.Port == powerPort && (binding.Role == "power" || binding.Role == "input") {
				requiresSensedPath = true
				break
			}
		}
	}
	if !requiresSensedPath {
		return externalPower, nil
	}
	if plan == nil {
		return "", fmt.Errorf("load-current harness requires the resolved current-sense series path")
	}
	var candidates []string
	for _, device := range plan.Devices {
		if device.PrimitiveModel != simmodel.PrimitiveCurrentSenseAmplifierV1 {
			continue
		}
		terminals := map[string]string{}
		for _, terminal := range device.Terminals {
			terminals[terminal.Terminal] = terminal.Net
		}
		downstream := ""
		switch {
		case terminals["IN_PLUS"] == externalPower:
			downstream = terminals["IN_MINUS"]
		case terminals["IN_MINUS"] == externalPower:
			downstream = terminals["IN_PLUS"]
		}
		if downstream != "" && resistorSpansNodes(*plan, externalPower, downstream) {
			candidates = append(candidates, downstream)
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	if len(candidates) != 1 {
		return "", fmt.Errorf("load-current harness requires exactly one catalog-backed current-sense series path from semantic power %q", powerPort)
	}
	return candidates[0], nil
}

func resistorSpansNodes(plan simmodel.Plan, first, second string) bool {
	for _, device := range plan.Devices {
		if device.PrimitiveModel != simmodel.PrimitiveResistorV1 {
			continue
		}
		touchesFirst, touchesSecond := false, false
		for _, terminal := range device.Terminals {
			touchesFirst = touchesFirst || terminal.Net == first
			touchesSecond = touchesSecond || terminal.Net == second
		}
		if touchesFirst && touchesSecond {
			return true
		}
	}
	return false
}

func operatingConditionSemanticTarget(condition architecturesearch.OperatingCondition, targets map[string]string) (string, bool) {
	if condition.Axis == "supply_voltage" {
		target, ok := targets["domain\x00"+condition.Target]
		return target, ok
	}
	for _, kind := range []string{"port", "signal", "domain"} {
		if target := targets[kind+"\x00"+condition.Target]; target != "" {
			return target, true
		}
	}
	return "", false
}

func positiveOperatingValue(condition architecturesearch.OperatingCondition) (float64, bool) {
	for _, candidate := range []*float64{condition.Min, condition.Max} {
		if candidate != nil && finiteArchitectureValue(*candidate) && *candidate > 0 {
			return *candidate, true
		}
	}
	return 0, false
}

func maximumPositiveOperatingValue(condition architecturesearch.OperatingCondition) (float64, bool) {
	maximum := 0.0
	for _, candidate := range []*float64{condition.Min, condition.Max} {
		if candidate != nil && finiteArchitectureValue(*candidate) && *candidate > maximum {
			maximum = *candidate
		}
	}
	return maximum, maximum > 0
}

func addOperatingHarnessExcitations(intent *simmodel.Intent, harness []operatingHarnessDevice) {
	for analysisIndex := range intent.Analyses {
		analysis := &intent.Analyses[analysisIndex]
		for _, entry := range harness {
			if !entry.Source {
				continue
			}
			if slices.ContainsFunc(analysis.Excitations, func(excitation simmodel.SourceExcitation) bool {
				return excitation.Component == entry.Device.InstanceID
			}) {
				continue
			}
			excitation := simmodel.SourceExcitation{Component: entry.Device.InstanceID}
			if entry.HasDefaultValue {
				excitation.DCValue = entry.DefaultValue
				if analysis.Kind == simmodel.AnalysisTransient && analysis.TimeStepS > 0 && analysis.DurationS >= 4*analysis.TimeStepS {
					excitation.DCValue = 0
					excitation.PulseInitialValue = 0
					excitation.PulseValue = entry.DefaultValue
					excitation.PulseDelayS = 2 * analysis.TimeStepS
					excitation.PulseWidthS = analysis.DurationS
					excitation.PulsePeriodS = analysis.DurationS + analysis.TimeStepS
				}
			}
			analysis.Excitations = append(analysis.Excitations, excitation)
		}
		slices.SortStableFunc(analysis.Excitations, func(left, right simmodel.SourceExcitation) int {
			return strings.Compare(left.Component, right.Component)
		})
	}
}

func planScopedToAnalysis(plan simmodel.Plan, kind string) simmodel.Plan {
	clone := simmodel.ClonePlan(plan)
	template, found := simulationAnalysisOfKind(plan.Analyses, kind)
	if !found {
		template = derivedAnalysisTemplate(plan, kind)
	}
	template.ID = "base_" + kind
	clone.Analyses = []simmodel.Analysis{template}
	clone.Assertions = nil
	return clone
}

func simulationAnalysisOfKind(analyses []simmodel.Analysis, kind string) (simmodel.Analysis, bool) {
	for _, analysis := range analyses {
		if analysis.Kind == kind {
			return analysis, true
		}
	}
	return simmodel.Analysis{}, false
}

func derivedAnalysisTemplate(plan simmodel.Plan, kind string) simmodel.Analysis {
	analysis := simmodel.Analysis{Kind: kind}
	if ac, ok := simulationAnalysisOfKind(plan.Analyses, simmodel.AnalysisACSweep); ok {
		analysis.Excitations = append([]simmodel.SourceExcitation(nil), ac.Excitations...)
		analysis.StartFrequencyHz, analysis.StopFrequencyHz, analysis.Points = ac.StartFrequencyHz, ac.StopFrequencyHz, ac.Points
	}
	if dc, ok := simulationAnalysisOfKind(plan.Analyses, simmodel.AnalysisDCOperatingPoint); ok && len(analysis.Excitations) == 0 {
		analysis.Excitations = append([]simmodel.SourceExcitation(nil), dc.Excitations...)
	}
	switch kind {
	case simmodel.AnalysisACSweep, simmodel.AnalysisNoise, simmodel.AnalysisStability:
		if analysis.StartFrequencyHz <= 0 || analysis.StopFrequencyHz < analysis.StartFrequencyHz || analysis.Points < 2 {
			analysis.StartFrequencyHz, analysis.StopFrequencyHz, analysis.Points = 10, 100_000, 64
		}
		if kind == simmodel.AnalysisACSweep || kind == simmodel.AnalysisStability {
			analysis = modelBoundedACSweep(plan, analysis)
		}
		for index := range analysis.Excitations {
			analysis.Excitations[index] = simmodel.SourceExcitation{Component: analysis.Excitations[index].Component, DCValue: analysis.Excitations[index].DCValue}
		}
	case simmodel.AnalysisThermal:
		analysis.StartFrequencyHz, analysis.StopFrequencyHz, analysis.Points = 0, 0, 0
		analysis.Conditions = []simmodel.NamedValue{{Name: "ambient_temperature_c", Value: 25}}
	}
	return analysis
}

func modelBoundedACSweep(plan simmodel.Plan, analysis simmodel.Analysis) simmodel.Analysis {
	minimumPole, maximumBandwidth := math.Inf(1), 0.0
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case simmodel.PrimitiveOpAmpV1:
			gain, gainOK := simulationModelParameter(device.ModelParameters, "dc_open_loop_gain")
			bandwidth, bandwidthOK := simulationModelParameter(device.ModelParameters, "gain_bandwidth_hz")
			if !gainOK || !bandwidthOK || gain <= 0 || bandwidth <= 0 {
				continue
			}
			minimumPole = math.Min(minimumPole, bandwidth/gain)
			maximumBandwidth = math.Max(maximumBandwidth, bandwidth)
		case simmodel.PrimitiveBJTNPNV1, simmodel.PrimitiveBJTPNPV1:
			transition, transitionOK := simulationModelParameter(device.ModelParameters, "transition_frequency_hz")
			beta, betaOK := simulationModelParameter(device.ModelParameters, "forward_beta")
			if !transitionOK || !betaOK || transition <= 0 || beta <= 0 {
				continue
			}
			minimumPole = math.Min(minimumPole, transition/beta)
			maximumBandwidth = math.Max(maximumBandwidth, transition)
		}
	}
	if finiteArchitectureValue(minimumPole) && minimumPole > 0 {
		analysis.StartFrequencyHz = math.Min(analysis.StartFrequencyHz, minimumPole/10)
	}
	if maximumBandwidth > 0 {
		analysis.StopFrequencyHz = math.Max(analysis.StopFrequencyHz, maximumBandwidth*10)
		analysis.Points = max(analysis.Points, 64)
	}
	return analysis
}

func behavioralBoundedACSweep(requirement architecturesearch.Requirement, analysis simmodel.Analysis) simmodel.Analysis {
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Analysis != simmodel.AnalysisACSweep || behavior.Metric != "bandwidth" && behavior.Metric != "cutoff_frequency" {
			continue
		}
		if behavior.Min != nil && *behavior.Min > 0 && finiteArchitectureValue(*behavior.Min) {
			if behavior.Metric == "cutoff_frequency" {
				analysis.StartFrequencyHz = math.Min(analysis.StartFrequencyHz, *behavior.Min/10)
			}
			analysis.StopFrequencyHz = math.Max(analysis.StopFrequencyHz, *behavior.Min*10)
		}
		if behavior.Max != nil && *behavior.Max > 0 && finiteArchitectureValue(*behavior.Max) {
			analysis.StopFrequencyHz = math.Max(analysis.StopFrequencyHz, *behavior.Max*10)
		}
		analysis.Points = max(analysis.Points, 64)
	}
	return analysis
}

func simulationModelParameter(parameters []simmodel.NamedValue, name string) (float64, bool) {
	for _, parameter := range parameters {
		if parameter.Name == name && finiteArchitectureValue(parameter.Value) {
			return parameter.Value, true
		}
	}
	return 0, false
}

func derivedGraphWorkflowIntent(requirement architecturesearch.Requirement, base simmodel.Plan, kind, primaryInputNode string, inputSourceFallback ...string) (simmodel.Intent, error) {
	modelID := base.ModelID
	switch kind {
	case simmodel.AnalysisTransient, simmodel.AnalysisStartup, simmodel.AnalysisDistortion:
		modelID = simmodel.ModelTransientCircuitV1
	case simmodel.AnalysisNoise, simmodel.AnalysisStability, simmodel.AnalysisACSweep:
		modelID = simmodel.ModelLinearCircuitMNAV1
	case simmodel.AnalysisThermal:
		if _, driven := behavioralInputAmplitude(requirement); driven {
			modelID = simmodel.ModelTransientCircuitV1
		}
	case simmodel.AnalysisDCOperatingPoint:
		// Preserve the resolved linear/nonlinear operating-point workflow.
	default:
		return simmodel.Intent{}, fmt.Errorf("analysis kind is not registered for graph workflow derivation")
	}
	analysis := derivedAnalysisTemplate(base, kind)
	analysis.ID = "derived_" + kind
	if kind == simmodel.AnalysisDistortion {
		var distortionErr error
		analysis, distortionErr = derivedBehavioralDistortionAnalysis(requirement, base, analysis, primaryInputNode, 1, inputSourceFallback...)
		if distortionErr != nil {
			return simmodel.Intent{}, distortionErr
		}
	}
	if kind == simmodel.AnalysisThermal {
		if _, driven := behavioralInputAmplitude(requirement); driven {
			var thermalErr error
			analysis, thermalErr = derivedBehavioralDistortionAnalysis(requirement, base, analysis, primaryInputNode, behavioralRatedPowerVoltageGuard, inputSourceFallback...)
			if thermalErr != nil {
				return simmodel.Intent{}, thermalErr
			}
			analysis.Kind = simmodel.AnalysisThermal
			analysis.Conditions = []simmodel.NamedValue{{Name: "ambient_temperature_c", Value: 25}}
		}
	}
	if kind == simmodel.AnalysisACSweep {
		analysis = behavioralBoundedACSweep(requirement, analysis)
		component, ok := uniquePlanSourceAtNode(base, primaryInputNode)
		if !ok && len(inputSourceFallback) != 0 && inputSourceFallback[0] != "" {
			component, ok = inputSourceFallback[0], true
		}
		if !ok {
			return simmodel.Intent{}, fmt.Errorf("AC analysis requires exactly one catalog-backed source at the resolved primary input")
		}
		found := false
		for index := range analysis.Excitations {
			analysis.Excitations[index].ACMagnitude = 0
			analysis.Excitations[index].ACPhaseDeg = 0
			if analysis.Excitations[index].Component == component {
				analysis.Excitations[index].ACMagnitude = 1
				found = true
			}
		}
		if !found {
			analysis.Excitations = append(analysis.Excitations, simmodel.SourceExcitation{Component: component, ACMagnitude: 1})
		}
	}
	if kind == simmodel.AnalysisTransient || kind == simmodel.AnalysisStartup {
		analysis.DurationS, analysis.TimeStepS = behavioralDynamicGrid(requirement, kind)
		if kind == simmodel.AnalysisTransient {
			for _, behavior := range requirement.Requirements.BehavioralRequirements {
				if behavior.Analysis != simmodel.AnalysisTransient || (behavior.Metric != "output_swing" && behavior.Metric != "output_power") {
					continue
				}
				frequency := behavioralDistortionFrequency(requirement)
				periodicDuration := float64(behavioralDistortionCycles) / frequency
				periodicStep := 1 / (frequency * behavioralTransientSamplesPerCycle)
				if behavioralDynamicTimeRequirement(requirement, kind) {
					analysis.DurationS = math.Max(analysis.DurationS, periodicDuration)
					analysis.TimeStepS = math.Min(analysis.TimeStepS, periodicStep)
				} else {
					// The fallback dynamic grid is deliberately conservative for edge
					// response, but it is not itself a requested time-resolution
					// constraint. A steady periodic power/swing measurement needs an
					// exact multi-cycle grid, so do not stretch that fallback to the
					// maximum step count when no timing assertion depends on it.
					analysis.DurationS = periodicDuration
					analysis.TimeStepS = periodicStep
				}
				break
			}
		}
		analysis.DurationS, analysis.TimeStepS = boundedBehavioralDynamicGrid(analysis.DurationS, analysis.TimeStepS)
		if kind == simmodel.AnalysisTransient {
			if err := configureAutonomousTransientSupplyStep(requirement, base, &analysis); err != nil {
				return simmodel.Intent{}, err
			}
		}
		if len(analysis.Excitations) == 0 {
			return simmodel.Intent{}, fmt.Errorf("dynamic analysis has no catalog-backed source excitation")
		}
	}
	node := ""
	for _, candidate := range base.Nodes {
		if candidate != base.GroundNode {
			node = candidate
			break
		}
	}
	if node == "" {
		return simmodel.Intent{}, fmt.Errorf("resolved graph has no observable non-reference node")
	}
	assertion := simmodel.Assertion{AnalysisID: analysis.ID, Node: node, Min: -1e6, Max: 1e6}
	switch kind {
	case simmodel.AnalysisDCOperatingPoint:
		assertion.Quantity = simmodel.QuantityVoltageV
	case simmodel.AnalysisACSweep:
		assertion.Quantity = simmodel.QuantityVoltageMagnitudeV
		assertion.FrequencyHz = analysis.StartFrequencyHz
	case simmodel.AnalysisNoise:
		assertion.Quantity = simmodel.QuantityIntegratedNoiseVRMS
		assertion.Min = 0
	case simmodel.AnalysisStability:
		assertion.Quantity = simmodel.QuantityPhaseMarginDeg
	case simmodel.AnalysisStartup:
		assertion.Quantity = simmodel.QuantityPeakAbsVoltageV
	case simmodel.AnalysisTransient:
		assertion.Quantity, assertion.TimeS = simmodel.QuantityVoltageV, analysis.DurationS
	case simmodel.AnalysisDistortion:
		assertion.Quantity = simmodel.QuantityTHDPercent
		assertion.Min = 0
	case simmodel.AnalysisThermal:
		assertion.Node = ""
		assertion.Quantity = simmodel.QuantityJunctionTemperatureC
		for _, device := range base.Devices {
			if deviceHasThermalPath(device) {
				assertion.Component = device.Component
				break
			}
		}
		if assertion.Component == "" {
			return simmodel.Intent{}, fmt.Errorf("thermal workflow has no catalog-backed device thermal path")
		}
	}
	return simmodel.Intent{ModelID: modelID, Analyses: []simmodel.Analysis{analysis}, Assertions: []simmodel.Assertion{assertion}}, nil
}

func configureAutonomousTransientSupplyStep(requirement architecturesearch.Requirement, base simmodel.Plan, analysis *simmodel.Analysis) error {
	hasAutonomousCurrentRegulator := slices.ContainsFunc(base.Devices, func(device simmodel.ResolvedDevice) bool {
		return device.PrimitiveModel == simmodel.PrimitiveProgrammableCurrentSourceV1
	})
	if !hasAutonomousCurrentRegulator {
		return nil
	}
	needsEdge := false
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Analysis != simmodel.AnalysisTransient {
			continue
		}
		switch behavior.Metric {
		case "rise_time", "fall_time", "settling_time", "response_time":
			needsEdge = true
		}
	}
	if !needsEdge || simulationAnalysisHasDynamicExcitation(*analysis) {
		return nil
	}
	voltageSources := map[string]string{}
	reachableSupplyNets := autonomousCurrentRegulatorSupplyNets(base)
	for _, device := range base.Devices {
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1, simmodel.PrimitiveConnectorVoltageSourceV1:
			positive := resolvedDeviceTerminalNet(device, "POSITIVE")
			if positive == "" {
				positive = resolvedDeviceTerminalNet(device, "PIN_1")
			}
			voltageSources[device.Component] = positive
		}
	}
	var candidates []int
	for index, excitation := range analysis.Excitations {
		if reachableSupplyNets[voltageSources[excitation.Component]] && excitation.DCValue != 0 {
			candidates = append(candidates, index)
		}
	}
	if len(candidates) != 1 {
		return fmt.Errorf("autonomous edge-response analysis requires exactly one bounded DC source electrically upstream of the programmable current source")
	}
	index := candidates[0]
	value := analysis.Excitations[index].DCValue
	delay := 2 * analysis.TimeStepS
	if delay >= analysis.DurationS {
		return fmt.Errorf("autonomous edge-response analysis has no bounded pulse window")
	}
	analysis.Excitations[index].DCValue = 0
	analysis.Excitations[index].PulseInitialValue = 0
	analysis.Excitations[index].PulseValue = value
	analysis.Excitations[index].PulseDelayS = delay
	analysis.Excitations[index].PulseWidthS = analysis.DurationS
	analysis.Excitations[index].PulsePeriodS = analysis.DurationS + analysis.TimeStepS
	return nil
}

func autonomousCurrentRegulatorSupplyNets(base simmodel.Plan) map[string]bool {
	reachable := map[string]bool{}
	for _, device := range base.Devices {
		if device.PrimitiveModel == simmodel.PrimitiveProgrammableCurrentSourceV1 {
			if input := resolvedDeviceTerminalNet(device, "IN"); input != "" {
				reachable[input] = true
			}
		}
	}
	changed := true
	for changed {
		changed = false
		for _, device := range base.Devices {
			if device.PrimitiveModel != simmodel.PrimitivePMOSSwitchV1 {
				continue
			}
			drain := resolvedDeviceTerminalNet(device, "DRAIN")
			source := resolvedDeviceTerminalNet(device, "SOURCE")
			if reachable[drain] && source != "" && !reachable[source] {
				reachable[source] = true
				changed = true
			}
		}
	}
	return reachable
}

func resolvedDeviceTerminalNet(device simmodel.ResolvedDevice, terminal string) string {
	for _, binding := range device.Terminals {
		if strings.EqualFold(binding.Terminal, terminal) {
			return binding.Net
		}
	}
	return ""
}

func simulationAnalysisHasDynamicExcitation(analysis simmodel.Analysis) bool {
	for _, excitation := range analysis.Excitations {
		if excitation.PulsePeriodS > 0 || excitation.SineFrequencyHz > 0 {
			return true
		}
	}
	return false
}

func derivedBehavioralDistortionAnalysis(requirement architecturesearch.Requirement, base simmodel.Plan, analysis simmodel.Analysis, primaryInputNode string, voltageGuard float64, inputSourceFallback ...string) (simmodel.Analysis, error) {
	component, ok := uniquePlanSourceAtNode(base, primaryInputNode)
	fallback := false
	if !ok && len(inputSourceFallback) != 0 && inputSourceFallback[0] != "" {
		component, ok, fallback = inputSourceFallback[0], true, true
	}
	if !ok {
		return simmodel.Analysis{}, fmt.Errorf("distortion analysis requires exactly one catalog-backed source at the resolved primary input")
	}
	polarity := 1.0
	if !fallback {
		polarity, ok = voltageSourcePolarityAtNode(base, component, primaryInputNode)
	}
	if !ok {
		return simmodel.Analysis{}, fmt.Errorf("distortion analysis requires a catalog-backed voltage source at the resolved primary input")
	}
	amplitude, ok := behavioralInputAmplitudeForGain(requirement, positiveBehavioralNominal, voltageGuard)
	if !ok {
		return simmodel.Analysis{}, fmt.Errorf("distortion analysis requires a bounded output-swing or output-power requirement and voltage-gain requirement")
	}
	frequency := behavioralDistortionFrequency(requirement)
	analysis.StartFrequencyHz, analysis.StopFrequencyHz, analysis.Points = 0, 0, 0
	analysis.DurationS = float64(behavioralDistortionCycles) / frequency
	analysis.TimeStepS = 1 / (frequency * behavioralDistortionSamplesPerCycle)
	analysis.DCSweep = nil
	found := false
	for index := range analysis.Excitations {
		excitation := &analysis.Excitations[index]
		dcValue := excitation.DCValue
		*excitation = simmodel.SourceExcitation{Component: excitation.Component, DCValue: dcValue}
		if excitation.Component == component {
			found = true
			excitation.SineAmplitude = amplitude
			excitation.SineFrequencyHz = frequency
			if polarity < 0 {
				excitation.SinePhaseDeg = 180
			}
		}
	}
	if !found {
		analysis.Excitations = append(analysis.Excitations, simmodel.SourceExcitation{Component: component, SineAmplitude: amplitude, SineFrequencyHz: frequency})
	}
	return analysis, nil
}

func behavioralInputAmplitude(requirement architecturesearch.Requirement) (float64, bool) {
	return behavioralInputAmplitudeForGain(requirement, positiveBehavioralNominal, behavioralRatedPowerVoltageGuard)
}

func behavioralOutputSpanInputAmplitude(requirement architecturesearch.Requirement) (float64, bool) {
	return behavioralInputAmplitudeForGain(requirement, positiveBehavioralLowerBound, behavioralRatedPowerVoltageGuard)
}

func behavioralInputAmplitudeForGain(requirement architecturesearch.Requirement, selectGain func(architecturesearch.BehavioralRequirement) float64, voltageGuard float64) (float64, bool) {
	if !finiteArchitectureValue(voltageGuard) || voltageGuard <= 0 {
		return 0, false
	}
	gain := 0.0
	outputPeak := 0.0
	outputPower := 0.0
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		switch behavior.Metric {
		case "voltage_gain":
			gain = selectGain(behavior)
		case "output_swing":
			if behavior.Unit == "V_pp" && behavior.Min != nil && *behavior.Min > 0 {
				outputPeak = math.Max(outputPeak, *behavior.Min/2)
			}
		case "output_power":
			if behavior.Unit == "W" && behavior.Min != nil && *behavior.Min > 0 {
				outputPower = math.Max(outputPower, *behavior.Min)
			}
		}
	}
	if gain <= 0 || !finiteArchitectureValue(gain) {
		return 0, false
	}
	if outputPeak <= 0 && outputPower > 0 {
		loadResistance := math.Inf(1)
		for _, operatingCase := range requirement.Requirements.OperatingCases {
			for _, condition := range operatingCase.Conditions {
				if condition.Axis != "load_resistance" {
					continue
				}
				for _, candidate := range []*float64{condition.Min, condition.Max} {
					if candidate != nil && *candidate > 0 {
						loadResistance = math.Min(loadResistance, *candidate)
					}
				}
			}
		}
		if finiteArchitectureValue(loadResistance) && loadResistance > 0 {
			// Transient power and thermal workflows use a small deterministic
			// voltage guard so preferred-value and integration rounding cannot
			// turn an exactly rated ideal sine into an artificial power miss.
			// Distortion is measured at the requested rated power itself: adding
			// this guard there would measure a different operating point.
			outputPeak = math.Sqrt(2*outputPower*loadResistance) * voltageGuard
		}
	}
	amplitude := outputPeak / gain
	return amplitude, finiteArchitectureValue(amplitude) && amplitude > 0
}

func positiveBehavioralNominal(behavior architecturesearch.BehavioralRequirement) float64 {
	if behavior.Min != nil && behavior.Max != nil && *behavior.Min > 0 && *behavior.Max > 0 {
		return (*behavior.Min + *behavior.Max) / 2
	}
	if behavior.Min != nil && *behavior.Min > 0 {
		return *behavior.Min
	}
	if behavior.Max != nil && *behavior.Max > 0 {
		return *behavior.Max
	}
	return 0
}

func positiveBehavioralLowerBound(behavior architecturesearch.BehavioralRequirement) float64 {
	if behavior.Min != nil && *behavior.Min > 0 {
		return *behavior.Min
	}
	return positiveBehavioralNominal(behavior)
}

func behavioralDistortionFrequency(requirement architecturesearch.Requirement) float64 {
	frequency := behavioralDistortionFrequencyHz
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Metric != "bandwidth" || behavior.Unit != "Hz" || behavior.Min == nil || *behavior.Min <= 0 {
			continue
		}
		frequency = math.Min(frequency, *behavior.Min/10)
	}
	return math.Max(1, frequency)
}

func behavioralPrimaryInputNode(requirement architecturesearch.Requirement, bindings []closedloopsynthesis.SemanticBinding) (string, bool) {
	_, node, ok := behavioralPrimaryInputPort(requirement, bindings)
	return node, ok
}

func behavioralPrimaryInputPort(requirement architecturesearch.Requirement, bindings []closedloopsynthesis.SemanticBinding) (architecturesearch.Port, string, bool) {
	targets := map[string]string{}
	for _, binding := range bindings {
		targets[binding.Kind+"\x00"+binding.ID] = binding.Target
	}
	signalKinds := make(map[string]string, len(requirement.Requirements.Signals))
	for _, signal := range requirement.Requirements.Signals {
		signalKinds[signal.ID] = signal.Kind
	}
	type candidate struct {
		port  architecturesearch.Port
		node  string
		score int
	}
	var candidates []candidate
	for _, port := range requirement.Requirements.Ports {
		if port.Kind == "power" || port.Kind == "reference" || port.Direction == "source" {
			continue
		}
		node := targets["port\x00"+port.ID]
		if node == "" {
			continue
		}
		ingress, roleScore := port.Direction == "sink", 0
		for _, objective := range requirement.Requirements.Objectives {
			boundToPort, producesSignal, consumesSignal := false, false, false
			for _, binding := range objective.Bindings {
				if binding.Port == port.ID {
					boundToPort = true
					switch binding.Role {
					case "input", "signal", "sense":
						roleScore = max(roleScore, 5)
					case "control", "enable", "mute", "bias":
						roleScore = min(roleScore, -5)
					}
				}
				if binding.Signal == "" || signalKinds[binding.Signal] == "power" || signalKinds[binding.Signal] == "reference" {
					continue
				}
				switch binding.Direction {
				case "source":
					producesSignal = true
				case "sink":
					consumesSignal = true
				}
			}
			if boundToPort && producesSignal && !consumesSignal {
				ingress = true
			}
		}
		if !ingress {
			continue
		}
		kindScore := 10
		switch port.Kind {
		case "analog_voltage", "analog_current":
			kindScore = 30
		case "digital_logic", "digital_bus", "open_drain_bus":
			kindScore = 20
		}
		candidates = append(candidates, candidate{port: port, node: node, score: kindScore + roleScore})
	}
	slices.SortStableFunc(candidates, func(left, right candidate) int {
		if left.score != right.score {
			return right.score - left.score
		}
		if order := strings.Compare(left.node, right.node); order != 0 {
			return order
		}
		return strings.Compare(left.port.ID, right.port.ID)
	})
	if len(candidates) == 0 || (len(candidates) > 1 && candidates[0].score == candidates[1].score && candidates[0].node != candidates[1].node) {
		return architecturesearch.Port{}, "", false
	}
	return candidates[0].port, candidates[0].node, true
}

func uniquePlanSourceAtNode(plan simmodel.Plan, node string) (string, bool) {
	if node == "" {
		return "", false
	}
	var candidates []string
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1, simmodel.PrimitiveConnectorVoltageSourceV1, simmodel.PrimitiveCurrentSourceV1:
		default:
			continue
		}
		for _, terminal := range device.Terminals {
			if terminal.Net == node {
				candidates = append(candidates, device.Component)
				break
			}
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0], true
}

func deviceHasThermalPath(device simmodel.ResolvedDevice) bool {
	for _, parameter := range device.ModelParameters {
		switch parameter.Name {
		case "thermal_resistance_c_per_w", "junction_to_ambient_c_per_w", "junction_to_case_c_per_w":
			return true
		}
	}
	return false
}

func behavioralDynamicGrid(requirement architecturesearch.Requirement, kind string) (float64, float64) {
	referenceTime := math.Inf(1)
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Analysis != kind || behavior.Unit != "s" || behavior.Max == nil || *behavior.Max <= 0 {
			continue
		}
		referenceTime = math.Min(referenceTime, *behavior.Max)
	}
	if !finiteArchitectureValue(referenceTime) {
		referenceTime = 1e-6
	}
	timeStep := math.Max(1e-9, referenceTime/20)
	duration := math.Max(referenceTime*20, timeStep*100)
	if kind == simmodel.AnalysisStartup {
		duration = math.Max(duration, 100e-6)
		timeStep = math.Max(timeStep, duration/behavioralStartupSteps)
	}
	return boundedBehavioralDynamicGrid(duration, timeStep)
}

func behavioralDynamicTimeRequirement(requirement architecturesearch.Requirement, kind string) bool {
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Analysis == kind && behavior.Unit == "s" && behavior.Max != nil && *behavior.Max > 0 && finiteArchitectureValue(*behavior.Max) {
			return true
		}
	}
	return false
}

func boundedBehavioralDynamicGrid(duration, requestedTimeStep float64) (float64, float64) {
	steps := math.Ceil(duration / requestedTimeStep)
	steps = math.Max(1, math.Min(2048, steps))
	return duration, duration / steps
}

func (resolver ArchitectureSimulationPlanResolver) ResolveSimulationPlanSet(ctx context.Context, state closedloopsynthesis.CandidateState) (closedloopsynthesis.FreshSimulationPlanSet, error) {
	plans, err := resolver.ResolveSimulationPlans(ctx, state)
	if err != nil {
		return closedloopsynthesis.FreshSimulationPlanSet{}, err
	}
	candidate, ok := retainedArchitectureCandidate(resolver.Search, state.Fingerprint)
	if !ok {
		return closedloopsynthesis.FreshSimulationPlanSet{}, fmt.Errorf("candidate fingerprint is absent from the retained architecture search")
	}
	search := resolver.Search
	search.Status, search.Selected, search.Alternatives, search.Rationale = architecturesearch.SearchSelected, &candidate, nil, nil
	lowered, issues := Lower(resolver.Requirement, search)
	if reports.HasBlockingIssue(issues) {
		return closedloopsynthesis.FreshSimulationPlanSet{}, fmt.Errorf("lower architecture semantic bindings: %s", joinReportIssues(issues))
	}
	decisions, decisionDiagnostics := closedloopsynthesis.ResolvePlanSetModelDecisions(plans, resolver.ProvenanceRegistry)
	if len(decisionDiagnostics) != 0 {
		return closedloopsynthesis.FreshSimulationPlanSet{}, fmt.Errorf("resolve simulation model provenance: %s", joinClosedLoopDiagnostics(decisionDiagnostics))
	}
	analysisPlan, planDiagnostics := closedloopsynthesis.BuildAnalysisPlan(resolver.Requirement, lowered.Evidence.SemanticBindings, decisions)
	if len(planDiagnostics) != 0 {
		return closedloopsynthesis.FreshSimulationPlanSet{}, fmt.Errorf("bind candidate behavioral analysis plan: %s", joinClosedLoopDiagnostics(planDiagnostics))
	}
	templates, assertionBindings, operatingBindings, contractDiagnostics := closedloopsynthesis.BuildResolvedSimulationContracts(resolver.Requirement, analysisPlan, plans)
	if len(contractDiagnostics) != 0 {
		return closedloopsynthesis.FreshSimulationPlanSet{}, fmt.Errorf("bind resolved simulation contracts: %s", joinClosedLoopDiagnostics(contractDiagnostics))
	}
	return closedloopsynthesis.FreshSimulationPlanSet{
		Plans: plans, AnalysisPlan: analysisPlan, Templates: templates, Assertions: assertionBindings, OperatingBindings: operatingBindings,
	}, nil
}

func requiredBehavioralAnalyses(requirement architecturesearch.Requirement) []string {
	seen := map[string]bool{}
	var result []string
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if !seen[behavior.Analysis] {
			seen[behavior.Analysis] = true
			result = append(result, behavior.Analysis)
		}
	}
	slices.Sort(result)
	return result
}

func retainedArchitectureCandidate(search architecturesearch.SearchResult, fingerprint string) (architecturesearch.CandidateResult, bool) {
	if search.Selected != nil && search.Selected.Fingerprint == fingerprint {
		return *search.Selected, true
	}
	for _, candidate := range search.Alternatives {
		if candidate.Fingerprint == fingerprint {
			return candidate, true
		}
	}
	return architecturesearch.CandidateResult{}, false
}

func architectureBindingsForState(state closedloopsynthesis.CandidateState, bindings []ArchitectureVariableBinding) (map[string]ArchitectureVariableBinding, error) {
	result := map[string]ArchitectureVariableBinding{}
	for _, binding := range bindings {
		if binding.CandidateFingerprint != state.Fingerprint {
			continue
		}
		if binding.VariableID == "" || result[binding.VariableID].VariableID != "" || !validArchitectureVariableBinding(binding) {
			return nil, fmt.Errorf("candidate architecture variable binding is duplicate or structurally invalid")
		}
		result[binding.VariableID] = binding
	}
	for _, variable := range state.Variables {
		if _, ok := result[variable.ID]; !ok {
			return nil, fmt.Errorf("repair variable %s has no candidate-scoped architecture binding", variable.ID)
		}
	}
	if len(result) != len(state.Variables) {
		return nil, fmt.Errorf("candidate-scoped architecture binding does not identify a declared repair variable")
	}
	return result, nil
}

func validArchitectureVariableBinding(binding ArchitectureVariableBinding) bool {
	if !validClosedLoopHash(binding.CandidateFingerprint) || binding.VariableID == "" {
		return false
	}
	switch binding.Kind {
	case ArchitectureVariableFunctionParameter:
		return binding.Function != "" && binding.Parameter != "" && binding.Component == "" && len(binding.VariantChoices) == 0
	case ArchitectureVariableComponentValue:
		return binding.Component != "" && binding.Function == "" && binding.Parameter == "" && len(binding.VariantChoices) == 0
	case ArchitectureVariableModelParameter:
		return binding.Component != "" && binding.Function == "" && binding.Parameter != "" && len(binding.VariantChoices) == 0
	case ArchitectureVariableCatalogVariant:
		if binding.Component == "" || binding.Function != "" || binding.Parameter != "" || binding.Unit != "" || len(binding.VariantChoices) < 2 {
			return false
		}
		previous := math.Inf(-1)
		for _, choice := range binding.VariantChoices {
			if !finiteArchitectureValue(choice.Index) || choice.Index <= previous || choice.ComponentID == "" {
				return false
			}
			previous = choice.Index
		}
		return true
	default:
		return false
	}
}

func applyArchitectureFunctionVariables(document *circuitgraph.Document, state closedloopsynthesis.CandidateState, bindings map[string]ArchitectureVariableBinding) error {
	for _, variable := range state.Variables {
		binding := bindings[variable.ID]
		if binding.Kind != ArchitectureVariableFunctionParameter {
			continue
		}
		if document.Synthesis == nil {
			return fmt.Errorf("function-parameter repair requires unresolved synthesis intent")
		}
		found := false
		for functionIndex := range document.Synthesis.Functions {
			function := &document.Synthesis.Functions[functionIndex]
			if function.ID != binding.Function {
				continue
			}
			setCircuitGraphParameter(&function.Parameters, binding.Parameter, variable.Value)
			found = true
			break
		}
		if !found {
			return fmt.Errorf("function-parameter repair target %s is absent", binding.Function)
		}
	}
	return nil
}

func applyArchitectureComponentVariables(document *circuitgraph.Document, state closedloopsynthesis.CandidateState, bindings map[string]ArchitectureVariableBinding) error {
	for _, variable := range state.Variables {
		binding := bindings[variable.ID]
		if binding.Kind == ArchitectureVariableFunctionParameter {
			continue
		}
		found := false
		for componentIndex := range document.Components {
			component := &document.Components[componentIndex]
			if component.ID != binding.Component {
				continue
			}
			switch binding.Kind {
			case ArchitectureVariableComponentValue:
				component.Value = strconv.FormatFloat(variable.Value, 'g', -1, 64) + binding.Unit
			case ArchitectureVariableModelParameter:
				setCircuitGraphParameter(&component.Parameters, binding.Parameter, variable.Value)
			case ArchitectureVariableCatalogVariant:
				choice, ok := architectureVariantChoice(binding.VariantChoices, variable.Value)
				if !ok {
					return fmt.Errorf("catalog-variant repair value has no reviewed choice")
				}
				component.ComponentID, component.VariantID = choice.ComponentID, choice.VariantID
				component.Query = nil
			default:
				return fmt.Errorf("unsupported architecture variable binding kind")
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("component repair target %s is absent", binding.Component)
		}
	}
	return nil
}

func setCircuitGraphParameter(parameters *[]circuitgraph.Parameter, name string, value float64) {
	for index := range *parameters {
		if (*parameters)[index].Name == name {
			parameterValue := value
			(*parameters)[index].Value = circuitgraph.ParameterValue{Number: &parameterValue}
			return
		}
	}
	parameterValue := value
	*parameters = append(*parameters, circuitgraph.Parameter{Name: name, Value: circuitgraph.ParameterValue{Number: &parameterValue}})
	slices.SortStableFunc(*parameters, func(left, right circuitgraph.Parameter) int { return strings.Compare(left.Name, right.Name) })
}

func architectureVariantChoice(choices []ArchitectureVariantChoice, value float64) (ArchitectureVariantChoice, bool) {
	for _, choice := range choices {
		if choice.Index == value {
			return choice, true
		}
	}
	return ArchitectureVariantChoice{}, false
}

func joinReportIssues(issues []reports.Issue) string {
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.Blocking() {
			messages = append(messages, issue.Path+": "+issue.Message)
		}
	}
	slices.Sort(messages)
	return strings.Join(messages, "; ")
}

func joinClosedLoopDiagnostics(diagnostics []closedloopsynthesis.Diagnostic) string {
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Path+": "+diagnostic.Message)
	}
	slices.Sort(messages)
	return strings.Join(messages, "; ")
}

func validClosedLoopHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func finiteArchitectureValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
