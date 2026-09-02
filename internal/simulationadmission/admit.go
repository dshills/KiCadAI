package simulationadmission

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

type resolvedRecord struct {
	sourceID     string
	sourceKind   SourceKind
	sourceSHA256 string
	record       modelprovenance.Record
	hash         string
}

// PreparedEnvironment is an immutable, concurrency-safe admission index. Its
// fields are intentionally private so only validated normalization can create
// one; callers may reuse a value across candidate, corner, and repair checks.
type PreparedEnvironment struct {
	ready       bool
	hash        string
	enabled     []string
	records     map[string]resolvedRecord
	diagnostics []Diagnostic
}

func Admit(request Request, environment Environment) Decision {
	return AdmitPrepared(request, PrepareEnvironment(environment))
}

// PrepareEnvironment performs model-source validation, conflict detection,
// and record hashing once before the simulation hot path.
func PrepareEnvironment(environment Environment) PreparedEnvironment {
	environment = NormalizeEnvironment(environment)
	digest, hashErr := environmentHash(environment)
	prepared := PreparedEnvironment{
		ready: true, hash: digest, enabled: append([]string(nil), environment.EnabledSolvers...),
		records: map[string]resolvedRecord{}, diagnostics: []Diagnostic{},
	}
	if hashErr != nil {
		prepared.hash = hashFailure("environment", hashErr)
		prepared.diagnostics = append(prepared.diagnostics, Diagnostic{Code: CodeInvalidModelParameters, Path: "environment", Message: "model environment contains a value that cannot be represented canonically"})
	}
	prepared.diagnostics = append(prepared.diagnostics, validateEnvironment(environment)...)
	if len(prepared.diagnostics) == 0 {
		prepared.records, prepared.diagnostics = resolveRecords(environment.Sources)
	}
	slices.SortStableFunc(prepared.diagnostics, compareDiagnostics)
	return prepared
}

// AdmitPrepared deterministically checks one request against a previously
// validated immutable model and solver index.
func AdmitPrepared(request Request, prepared PreparedEnvironment) Decision {
	request = NormalizeRequest(request)
	requestDigest, requestHashErr := requestHash(request)
	decision := Decision{
		RequestSHA256: requestDigest, EnvironmentSHA256: prepared.hash,
		Analyses: []AnalysisDecision{}, Models: []ModelDecision{}, RejectedModels: []RejectedModelClaim{}, Diagnostics: []Diagnostic{},
	}
	if requestHashErr != nil {
		decision.RequestSHA256 = hashFailure("request", requestHashErr)
		decision.Diagnostics = append(decision.Diagnostics, Diagnostic{Code: CodeInvalidModelParameters, Path: "request", Message: "request contains a value that cannot be represented canonically"})
	}
	if !prepared.ready {
		decision.EnvironmentSHA256 = hashFailure("environment", fmt.Errorf("unprepared environment"))
		decision.Diagnostics = append(decision.Diagnostics, Diagnostic{Code: CodeSolverUnavailable, Path: "environment", Message: "admission requires an environment prepared by the trusted resolver"})
	}
	decision.Diagnostics = append(decision.Diagnostics, validateRequest(request)...)
	decision.Diagnostics = append(decision.Diagnostics, prepared.diagnostics...)
	if len(decision.Diagnostics) != 0 {
		return finalize(decision)
	}
	enabled := map[string]bool{}
	for _, solverID := range preparedEnabledSolvers(prepared) {
		enabled[solverID] = true
	}

	for _, required := range request.Analyses {
		canonical, known := canonicalAnalysis(required.AuthoredKind)
		if required.CanonicalKind != "" && required.CanonicalKind != canonical {
			decision.Diagnostics = append(decision.Diagnostics, Diagnostic{
				Code: CodeMissingAnalysisDefinition, Path: "analyses." + required.ID + ".canonical_kind", Analysis: required.AuthoredKind,
				Message:    "canonical analysis does not match the registered authored analysis",
				Suggestion: "derive canonical analyses from the immutable analysis registry",
			})
			continue
		}
		if !known {
			decision.Diagnostics = append(decision.Diagnostics, Diagnostic{
				Code: CodeUnsupportedAnalysis, Path: "analyses." + required.ID + ".authored_kind", Analysis: required.AuthoredKind,
				Message:    "analysis is not registered by the trusted analysis planner",
				Suggestion: "use a supported behavioral analysis or add a reviewed deterministic analysis implementation",
			})
			continue
		}
		solver, found := solverForAnalysis(canonical)
		if !found {
			decision.Diagnostics = append(decision.Diagnostics, Diagnostic{
				Code: CodeMissingAnalysisDefinition, Path: "analyses." + required.ID, Analysis: canonical,
				Message:    "registered analysis has no immutable solver definition",
				Suggestion: "add and review a deterministic solver definition before admitting the analysis",
			})
			continue
		}
		analysisDecision := AnalysisDecision{
			ID: required.ID, AuthoredKind: required.AuthoredKind, CanonicalKind: canonical,
			Assertions: required.Assertions, OperatingCases: required.OperatingCases,
			DCSweep: required.DCSweep, PeriodicExcitation: required.PeriodicExcitation,
			ThermalBoundary: required.ThermalBoundary, SmallSignalOperatingPoint: required.SmallSignalOperatingPoint,
			SolverID: solver.ID, SolverSHA256: solver.SHA256, Status: StatusAdmitted, Reason: "registered deterministic solver is enabled",
		}
		if !enabled[solver.ID] {
			analysisDecision.Status = StatusRefused
			analysisDecision.Reason = "registered deterministic solver is disabled in the executing environment"
			decision.Diagnostics = append(decision.Diagnostics, Diagnostic{
				Code: CodeSolverUnavailable, Path: "analyses." + required.ID + ".solver", Analysis: canonical,
				Message: analysisDecision.Reason, Suggestion: "enable the immutable solver or refuse the analysis",
			})
			decision.Analyses = append(decision.Analyses, analysisDecision)
			continue
		}
		if len(request.Components) == 0 {
			if !inventoryHasCompatibleModel(request.InventoryModels, prepared.records, canonical) {
				analysisDecision.Status = StatusRefused
				analysisDecision.Reason = "inventory contains no reviewed model applicable to the analysis"
				decision.Diagnostics = append(decision.Diagnostics, Diagnostic{
					Code: CodeMissingModel, Path: "analyses." + required.ID + ".models", Analysis: canonical,
					Message:    analysisDecision.Reason,
					Suggestion: "onboard a reviewed catalog model with compatible analysis applicability",
				})
			}
			decision.Analyses = append(decision.Analyses, analysisDecision)
			continue
		}

		selectedComponents, models, rejected, diagnostics := selectComponentModels(required, canonical, request.Components, prepared.records)
		decision.Models = append(decision.Models, models...)
		decision.RejectedModels = append(decision.RejectedModels, rejected...)
		decision.Diagnostics = append(decision.Diagnostics, diagnostics...)
		if len(diagnostics) == 0 {
			workflowModel, compatible, reason := simmodel.ApplicableGraphModelForAnalysis(selectedComponents, canonical)
			if !compatible || !slices.Contains(solver.WorkflowModels, workflowModel) || !simmodel.SupportsAnalysis(workflowModel, canonical) {
				analysisDecision.Status = StatusRefused
				analysisDecision.Reason = "selected primitive models are incompatible with the registered solver workflow: " + reason
				decision.Diagnostics = append(decision.Diagnostics, Diagnostic{
					Code: CodeSolverModelIncompatible, Path: "analyses." + required.ID + ".workflow_model", Analysis: canonical,
					Message:    analysisDecision.Reason,
					Suggestion: "select a complete unambiguous reviewed primitive set compatible with the solver workflow",
				})
			} else {
				analysisDecision.WorkflowModelID = workflowModel
			}
		} else {
			analysisDecision.Status = StatusRefused
			analysisDecision.Reason = "one or more connected components lack an admissible model"
		}
		decision.Analyses = append(decision.Analyses, analysisDecision)
	}
	return finalize(decision)
}

func preparedEnabledSolvers(prepared PreparedEnvironment) []string {
	if !prepared.ready {
		return nil
	}
	return prepared.enabled
}

func validateRequest(request Request) []Diagnostic {
	diagnostics := []Diagnostic{}
	if len(request.Analyses) == 0 || len(request.Analyses) > MaxAnalyses {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: "analyses", Message: fmt.Sprintf("admission requires 1..%d analyses", MaxAnalyses)})
	}
	if len(request.Components) > MaxComponents {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: "components", Message: fmt.Sprintf("admission supports at most %d connected components", MaxComponents)})
	}
	seen := map[string]bool{}
	assertionCount := 0
	for index, analysis := range request.Analyses {
		path := fmt.Sprintf("analyses[%d]", index)
		if analysis.ID == "" || analysis.AuthoredKind == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path, Message: "analysis requires stable identity and authored kind"})
		}
		if len(analysis.Assertions) == 0 || len(analysis.OperatingCases) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path, Message: "analysis requires at least one assertion and operating case"})
		}
		if seen[analysis.ID] {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path + ".id", Message: "analysis identity is duplicated"})
		}
		seen[analysis.ID] = true
		assertionCount += len(analysis.Assertions)
		canonical, known := canonicalAnalysis(analysis.AuthoredKind)
		if known {
			if analysis.DCSweep != (analysis.AuthoredKind == "dc_sweep") {
				diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path + ".dc_sweep", Analysis: analysis.AuthoredKind, Message: "DC sweep shape must be derived exactly from the authored dc_sweep analysis"})
			}
			smallSignal := canonical == simmodel.AnalysisACSweep || canonical == simmodel.AnalysisNoise || canonical == simmodel.AnalysisStability
			if analysis.SmallSignalOperatingPoint != smallSignal {
				diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path + ".small_signal_operating_point", Analysis: analysis.AuthoredKind, Message: "small-signal operating-point dependency does not match the registered analysis shape"})
			}
			thermal := canonical == simmodel.AnalysisThermal || canonical == simmodel.AnalysisElectrothermal
			if analysis.ThermalBoundary != thermal {
				diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path + ".thermal_boundary", Analysis: analysis.AuthoredKind, Message: "thermal-boundary dependency does not match the registered analysis shape"})
			}
			dynamic := canonical == simmodel.AnalysisTransient || canonical == simmodel.AnalysisStartup || canonical == simmodel.AnalysisDistortion || canonical == simmodel.AnalysisElectrothermal
			if analysis.PeriodicExcitation && !dynamic {
				diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path + ".periodic_excitation", Analysis: analysis.AuthoredKind, Message: "periodic excitation is incompatible with the registered analysis shape"})
			}
		}
	}
	for index, component := range request.Components {
		path := fmt.Sprintf("components[%d]", index)
		if component.HasValueSI && !finite(component.ValueSI) {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeInvalidModelParameters, Path: path + ".value_si", Component: component.InstanceID, Message: "component value must be finite"})
		}
		for claimIndex, claim := range component.ModelClaims {
			for parameterIndex, parameter := range claim.Parameters {
				if !finite(parameter.Value) {
					diagnostics = append(diagnostics, Diagnostic{
						Code:      CodeInvalidModelParameters,
						Path:      fmt.Sprintf("%s.model_claims[%d].parameters[%d].value", path, claimIndex, parameterIndex),
						Component: component.InstanceID, Message: "model parameter must be finite",
					})
				}
			}
		}
	}
	if assertionCount > MaxAssertions {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: "analyses.assertions", Message: fmt.Sprintf("admission supports at most %d assertion references", MaxAssertions)})
	}
	return diagnostics
}

func validateEnvironment(environment Environment) []Diagnostic {
	diagnostics := []Diagnostic{}
	diagnostics = append(diagnostics, validateSolverRegistry()...)
	if len(environment.Sources) == 0 || len(environment.Sources) > MaxSources {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingModel, Path: "environment.sources", Message: fmt.Sprintf("admission requires 1..%d trusted model sources", MaxSources)})
	}
	if len(environment.EnabledSolvers) > MaxEnabledSolvers {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeSolverUnavailable, Path: "environment.enabled_solvers", Message: fmt.Sprintf("admission supports at most %d enabled solvers", MaxEnabledSolvers)})
	}
	seenSources := map[string]bool{}
	for _, source := range environment.Sources {
		for _, diagnostic := range validateSource(source) {
			diagnostics = append(diagnostics, diagnostic)
		}
		key := string(source.Kind) + "\x00" + source.ID
		if seenSources[key] {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: "environment.sources." + source.ID, Message: "model source identity is duplicated"})
		}
		seenSources[key] = true
	}
	knownSolvers := map[string]bool{}
	for _, solver := range BuiltinSolvers() {
		knownSolvers[solver.ID] = true
	}
	for _, solverID := range environment.EnabledSolvers {
		if !knownSolvers[solverID] {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeSolverUnavailable, Path: "environment.enabled_solvers." + solverID, Message: "enabled solver is not present in the immutable registry"})
		}
	}
	return diagnostics
}

func validateSource(source Source) []Diagnostic {
	diagnostics := []Diagnostic{}
	path := "environment.sources." + source.ID
	if source.ID == "" || len(source.ID) > MaxSourceIdentityLen || source.ID != strings.TrimSpace(source.ID) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: path + ".id", Message: "model source requires a bounded canonical identity"})
	}
	if source.Kind != SourceBundled && source.Kind != SourceProject && source.Kind != SourceConfigured {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: path + ".kind", Message: "model source kind is not trusted"})
	}
	digest, err := modelprovenance.Hash(source.Registry)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeInvalidModelParameters, Path: path + ".registry", Message: err.Error()})
	} else if !validSHA256(source.SHA256) || source.SHA256 != digest {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: path + ".sha256", Message: "model source digest does not authenticate its normalized registry"})
	}
	return diagnostics
}

func resolveRecords(sources []Source) (map[string]resolvedRecord, []Diagnostic) {
	result := map[string]resolvedRecord{}
	diagnostics := []Diagnostic{}
	for _, source := range sources {
		for _, record := range source.Registry.Records {
			key := record.CatalogID + "\x00" + record.ModelID
			recordHash, err := hashJSON(record)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{Code: CodeInvalidModelParameters, Path: "environment.sources." + source.ID + ".records." + record.CatalogID + "." + record.ModelID, Message: "model provenance record cannot be authenticated"})
				continue
			}
			if existing, found := result[key]; found {
				if existing.hash != recordHash {
					diagnostics = append(diagnostics, Diagnostic{
						Code: CodeIncompatibleModel, Path: "environment.sources." + source.ID + ".records." + record.CatalogID + "." + record.ModelID,
						Message:    "trusted sources contain conflicting records for the same catalog/model identity",
						Suggestion: "retain one reviewed byte-equivalent normalized record",
					})
				}
				continue
			}
			result[key] = resolvedRecord{
				sourceID: source.ID, sourceKind: source.Kind, sourceSHA256: source.SHA256,
				record: record, hash: recordHash,
			}
		}
	}
	return result, diagnostics
}

func inventoryHasCompatibleModel(models []CatalogModel, records map[string]resolvedRecord, analysis string) bool {
	for _, model := range models {
		if _, found := records[model.CatalogID+"\x00"+model.ModelID]; found && simmodel.SupportsCatalogAnalysis(model.ModelID, analysis) {
			return true
		}
	}
	return false
}

func selectComponentModels(
	required AnalysisRequirement,
	analysis string,
	components []simmodel.ComponentEvidence,
	records map[string]resolvedRecord,
) ([]simmodel.ComponentEvidence, []ModelDecision, []RejectedModelClaim, []Diagnostic) {
	selectedComponents := cloneComponents(components)
	models := []ModelDecision{}
	rejected := []RejectedModelClaim{}
	diagnostics := []Diagnostic{}
	for componentIndex := range selectedComponents {
		component := &selectedComponents[componentIndex]
		if len(component.Connections) == 0 {
			continue
		}
		rejectedStart := len(rejected)
		compatible := []struct {
			claim  simmodel.CatalogEvidence
			record resolvedRecord
			deps   []string
		}{}
		for _, claim := range component.ModelClaims {
			record, found := records[component.CatalogID+"\x00"+strings.TrimSpace(claim.ModelID)]
			if !found {
				rejected = append(rejected, RejectedModelClaim{AnalysisID: required.ID, Component: component.InstanceID, CatalogID: component.CatalogID, ModelID: claim.ModelID, Code: CodeMissingModel, Reason: "model claim has no authenticated provenance record"})
				continue
			}
			if !simmodel.IsPrimitiveModel(claim.ModelID) {
				rejected = append(rejected, RejectedModelClaim{AnalysisID: required.ID, Component: component.InstanceID, CatalogID: component.CatalogID, ModelID: claim.ModelID, Code: CodeIncompatibleModel, Reason: "circuit-level workflow models cannot substitute for component primitive models"})
				continue
			}
			dependencies := simmodel.CatalogAnalysisDependencies(claim.ModelID, []string{analysis})
			if !simmodel.SupportsCatalogAnalysis(claim.ModelID, analysis) || !containsAll(record.record.Provenance.AllowedAnalyses, dependencies) {
				rejected = append(rejected, RejectedModelClaim{AnalysisID: required.ID, Component: component.InstanceID, CatalogID: component.CatalogID, ModelID: claim.ModelID, Code: CodeIncompatibleModel, Reason: "reviewed model applicability does not cover the required analysis dependencies"})
				continue
			}
			if evidenceDiagnostics := simmodel.ValidateCatalogEvidence(component.Family, []simmodel.CatalogEvidence{claim}); len(evidenceDiagnostics) != 0 {
				rejected = append(rejected, RejectedModelClaim{AnalysisID: required.ID, Component: component.InstanceID, CatalogID: component.CatalogID, ModelID: claim.ModelID, Code: CodeInvalidModelParameters, Reason: evidenceDiagnostics[0].Message})
				continue
			}
			if provenanceDiagnostics := simmodel.ValidateRequiredModelProvenance(&record.record.Provenance, dependencies); len(provenanceDiagnostics) != 0 {
				rejected = append(rejected, RejectedModelClaim{AnalysisID: required.ID, Component: component.InstanceID, CatalogID: component.CatalogID, ModelID: claim.ModelID, Code: CodeIncompatibleModel, Reason: provenanceDiagnostics[0].Message})
				continue
			}
			compatible = append(compatible, struct {
				claim  simmodel.CatalogEvidence
				record resolvedRecord
				deps   []string
			}{claim: claim, record: record, deps: dependencies})
		}
		if len(compatible) != 1 {
			code := CodeMissingModel
			message := "connected component has no authenticated compatible model"
			if len(compatible) > 1 {
				code = CodeIncompatibleModel
				message = "connected component has multiple compatible models; silent substitution is forbidden"
			} else {
				for _, claim := range rejected[rejectedStart:] {
					if claim.Code == CodeInvalidModelParameters {
						code = CodeInvalidModelParameters
						message = "connected component model parameters are invalid"
						break
					}
					if claim.Code == CodeIncompatibleModel {
						code = CodeIncompatibleModel
						message = "connected component models are incompatible with the required analysis"
					}
				}
			}
			diagnostics = append(diagnostics, Diagnostic{
				Code: code, Path: "components." + component.InstanceID + ".models", Analysis: analysis, Component: component.InstanceID,
				Message: message, Suggestion: "retain exactly one reviewed model applicable to this analysis",
			})
			component.ModelClaims = nil
			continue
		}
		selection := compatible[0]
		selection.claim.Parameters = normalizeNamedValues(selection.claim.Parameters)
		component.ModelClaims = []simmodel.CatalogEvidence{selection.claim}
		parameterPayload := struct {
			Parameters []simmodel.NamedValue `json:"parameters,omitempty"`
			ValueSI    *float64              `json:"value_si,omitempty"`
		}{Parameters: selection.claim.Parameters}
		if component.HasValueSI {
			value := component.ValueSI
			parameterPayload.ValueSI = &value
		}
		parametersHash, parametersHashErr := hashJSON(parameterPayload)
		claimHash, claimHashErr := hashJSON(selection.claim)
		if parametersHashErr != nil || claimHashErr != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code: CodeInvalidModelParameters, Path: "components." + component.InstanceID + ".models", Analysis: analysis, Component: component.InstanceID,
				Message: "selected model parameters cannot be represented canonically",
			})
			component.ModelClaims = nil
			continue
		}
		models = append(models, ModelDecision{
			AnalysisID: required.ID, Component: component.InstanceID, CatalogID: component.CatalogID, Family: component.Family,
			ModelID: selection.claim.ModelID, Parameters: selection.claim.Parameters, ValueSI: parameterPayload.ValueSI,
			ParametersSHA256: parametersHash, ModelClaim: simmodel.CloneCatalogEvidence(selection.claim),
			ModelClaimSHA256: claimHash,
			Provenance:       selection.record.record.Provenance,
			RegistrySourceID: selection.record.sourceID, RegistrySourceKind: selection.record.sourceKind,
			RegistrySourceSHA256: selection.record.sourceSHA256, RegistryRecordSHA256: selection.record.hash,
			RequiredAnalyses: selection.deps, CompatibilityStatus: StatusAdmitted,
			CompatibilityReason: "exactly one authenticated reviewed primitive model satisfies the analysis dependencies",
		})
	}
	return selectedComponents, models, rejected, diagnostics
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func containsAll(values, required []string) bool {
	for _, target := range required {
		if !slices.Contains(values, target) {
			return false
		}
	}
	return true
}
