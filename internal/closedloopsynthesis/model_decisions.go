package closedloopsynthesis

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

type planModelDependency struct {
	Component string
	CatalogID string
	Family    string
	Claim     simmodel.CatalogEvidence
}

// ResolvePlanModelDecisions derives model trust from the resolved simulation
// plan and the reviewed local registry. Callers cannot substitute provider or
// resolver-authored provenance for this decision.
func ResolvePlanModelDecisions(plan simmodel.Plan, registry modelprovenance.Registry) ([]ModelDecision, []Diagnostic) {
	var diagnostics []Diagnostic
	for _, diagnostic := range modelprovenance.Validate(modelprovenance.Normalize(registry)) {
		diagnostics = append(diagnostics, Diagnostic{Path: "model_registry." + diagnostic.Path, Message: diagnostic.Message})
	}
	required := requiredPlanAnalyses(plan)
	if len(required) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "plan.analyses", Message: "resolved simulation plan has no executable analysis class"})
	}
	for _, analysis := range required {
		if !simmodel.SupportsAnalysis(plan.ModelID, analysis) {
			diagnostics = append(diagnostics, Diagnostic{Path: "plan.analyses", Message: fmt.Sprintf("trusted model %s has no executable %s analysis", plan.ModelID, analysis), Suggestion: "select a registered workflow model with an implemented numerical evaluator"})
		}
	}

	dependencies := planModelDependencies(plan)
	if len(dependencies) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "plan.devices", Message: "resolved simulation plan has no catalog model dependencies"})
	}
	decisions := make([]ModelDecision, 0, len(dependencies))
	for _, dependency := range dependencies {
		decision := ModelDecision{Component: dependency.Component, Family: dependency.Family, Claim: dependency.Claim}
		dependencyAnalyses := simmodel.CatalogAnalysisDependencies(dependency.Claim.ModelID, required)
		record, exists := modelprovenance.Lookup(registry, dependency.CatalogID, dependency.Claim.ModelID)
		if !exists {
			decision.Status = "rejected"
			decision.Reason = "reviewed catalog/model provenance is missing"
			diagnostics = append(diagnostics, Diagnostic{Path: "model_decisions." + dependency.Component, Message: fmt.Sprintf("no reviewed provenance for catalog %s model %s", dependency.CatalogID, dependency.Claim.ModelID)})
			decisions = append(decisions, decision)
			continue
		}
		if record.Family != dependency.Family {
			decision.Status = "rejected"
			decision.Reason = "reviewed provenance family does not match the resolved component"
			diagnostics = append(diagnostics, Diagnostic{Path: "model_decisions." + dependency.Component, Message: fmt.Sprintf("provenance family %s does not match resolved family %s", record.Family, dependency.Family)})
			decisions = append(decisions, decision)
			continue
		}
		claimDiagnostics := simmodel.ValidateCatalogEvidence(dependency.Family, []simmodel.CatalogEvidence{dependency.Claim})
		provenanceDiagnostics := simmodel.ValidateRequiredModelProvenance(&record.Provenance, dependencyAnalyses)
		if len(claimDiagnostics) != 0 || len(provenanceDiagnostics) != 0 {
			decision.Status = "rejected"
			decision.Reason = "catalog model claim or reviewed applicability is invalid"
			for _, diagnostic := range append(claimDiagnostics, provenanceDiagnostics...) {
				diagnostics = append(diagnostics, Diagnostic{Path: "model_decisions." + dependency.Component + "." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
			}
			decisions = append(decisions, decision)
			continue
		}
		provenance := record.Provenance
		decision.Provenance = &provenance
		decision.Status = "used"
		decision.Reason = "resolved catalog model has reviewed provenance for every consumed analysis behavior"
		decision.RequiredAnalyses = append([]string(nil), dependencyAnalyses...)
		decisions = append(decisions, decision)
	}
	slices.SortStableFunc(decisions, compareModelDecisions)
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return decisions, diagnostics
}

func requiredPlanAnalyses(plan simmodel.Plan) []string {
	set := map[string]bool{}
	for _, analysis := range plan.Analyses {
		kind := strings.TrimSpace(analysis.Kind)
		if kind != "" {
			set[kind] = true
		}
	}
	if len(set) == 0 {
		for _, kind := range simmodel.SupportedAnalysisKinds(plan.ModelID) {
			set[kind] = true
		}
	}
	result := make([]string, 0, len(set))
	for kind := range set {
		result = append(result, kind)
	}
	slices.Sort(result)
	return result
}

func planModelDependencies(plan simmodel.Plan) []planModelDependency {
	dependencies := make([]planModelDependency, 0, len(plan.Devices)+len(plan.Bindings))
	if len(plan.Devices) != 0 {
		for _, device := range plan.Devices {
			dependencies = append(dependencies, planModelDependency{
				Component: device.Component,
				CatalogID: device.CatalogID,
				Family:    device.Family,
				Claim:     simmodel.CatalogEvidence{ModelID: device.PrimitiveModel, Parameters: append([]simmodel.NamedValue(nil), device.ModelParameters...)},
			})
		}
	} else {
		for _, binding := range plan.Bindings {
			dependencies = append(dependencies, planModelDependency{
				Component: binding.Component,
				CatalogID: binding.CatalogID,
				Family:    binding.Family,
				Claim:     simmodel.CatalogEvidence{ModelID: plan.ModelID, Parameters: append([]simmodel.NamedValue(nil), binding.ModelParameters...)},
			})
		}
	}
	slices.SortStableFunc(dependencies, func(left, right planModelDependency) int {
		if order := strings.Compare(left.Component, right.Component); order != 0 {
			return order
		}
		return strings.Compare(left.Claim.ModelID, right.Claim.ModelID)
	})
	return slices.CompactFunc(dependencies, func(left, right planModelDependency) bool {
		return left.Component == right.Component && left.Claim.ModelID == right.Claim.ModelID
	})
}
