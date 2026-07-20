package compositionlowering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	ArchitectureVariableFunctionParameter = "function_parameter"
	ArchitectureVariableComponentValue    = "component_value"
	ArchitectureVariableModelParameter    = "component_model_parameter"
	ArchitectureVariableCatalogVariant    = "catalog_variant_index"
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
	if resolver.GraphResolver == nil {
		return nil, fmt.Errorf("circuit-graph resolver is required")
	}
	candidate, ok := retainedArchitectureCandidate(resolver.Search, state.Fingerprint)
	if !ok {
		return nil, fmt.Errorf("candidate fingerprint is absent from the retained architecture search")
	}
	search := resolver.Search
	search.Status = architecturesearch.SearchSelected
	search.Selected = &candidate
	search.Alternatives = nil
	search.Rationale = nil
	lowered, loweringIssues := Lower(resolver.Requirement, search)
	if reports.HasBlockingIssue(loweringIssues) {
		return nil, fmt.Errorf("lower architecture candidate: %s", joinReportIssues(loweringIssues))
	}

	bindings, err := architectureBindingsForState(state, resolver.VariableBindings)
	if err != nil {
		return nil, err
	}
	document := lowered.Document
	if err := applyArchitectureFunctionVariables(&document, state, bindings); err != nil {
		return nil, err
	}
	synthesized, synthesisReport, synthesisIssues := resolver.GraphResolver.Synthesize(ctx, document)
	if reports.HasBlockingIssue(synthesisIssues) {
		return nil, fmt.Errorf("synthesize repaired architecture candidate: %s", joinReportIssues(synthesisIssues))
	}
	if err := applyArchitectureComponentVariables(&synthesized, state, bindings); err != nil {
		return nil, err
	}
	resolved, resolutionIssues := resolver.GraphResolver.Resolve(ctx, synthesized)
	if reports.HasBlockingIssue(resolutionIssues) {
		return nil, fmt.Errorf("resolve repaired architecture candidate: %s", joinReportIssues(resolutionIssues))
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
	previous := ""
	for _, kind := range kinds {
		if kind == previous {
			return nil, fmt.Errorf("required analysis kinds must be unique")
		}
		previous = kind
		if intent, exists := resolver.BaseIntents[kind]; exists {
			plan, issues := circuitgraph.ResolveSimulationPlan(intent, resolved)
			if reports.HasBlockingIssue(issues) {
				return nil, fmt.Errorf("resolve %s simulation workflow: %s", kind, joinReportIssues(issues))
			}
			if !simmodel.SupportsAnalysis(plan.ModelID, kind) {
				return nil, fmt.Errorf("trusted base intent model %s does not execute required analysis %s", plan.ModelID, kind)
			}
			result[kind] = planScopedToAnalysis(plan, kind)
			continue
		}
		if resolved.Simulation == nil || !simmodel.SupportsAnalysis(resolved.Simulation.ModelID, kind) {
			return nil, fmt.Errorf("resolved architecture has no trusted base intent for required analysis %s", kind)
		}
		result[kind] = planScopedToAnalysis(*resolved.Simulation, kind)
	}
	return result, nil
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
	case simmodel.AnalysisNoise, simmodel.AnalysisStability:
		if analysis.StartFrequencyHz <= 0 || analysis.StopFrequencyHz < analysis.StartFrequencyHz || analysis.Points < 2 {
			analysis.StartFrequencyHz, analysis.StopFrequencyHz, analysis.Points = 10, 100_000, 64
		}
		for index := range analysis.Excitations {
			analysis.Excitations[index] = simmodel.SourceExcitation{Component: analysis.Excitations[index].Component}
		}
	case simmodel.AnalysisThermal:
		analysis.StartFrequencyHz, analysis.StopFrequencyHz, analysis.Points = 0, 0, 0
		analysis.Conditions = []simmodel.NamedValue{{Name: "ambient_temperature_c", Value: 25}}
	}
	return analysis
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
		if issue.Severity == reports.SeverityBlocked {
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
