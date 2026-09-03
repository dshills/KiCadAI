package simulationadmission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

// DecodeDecisionStrict decodes one bounded admission artifact and rejects
// unknown fields, trailing values, non-canonical ordering, and hash tampering.
func DecodeDecisionStrict(reader io.Reader) (Decision, error) {
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, io.LimitReader(reader, MaxDecisionBytes+1)); err != nil {
		return Decision{}, err
	}
	if buffer.Len() > MaxDecisionBytes {
		return Decision{}, fmt.Errorf("simulation admission artifact exceeds maximum encoded size")
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var decision Decision
	if err := decoder.Decode(&decision); err != nil {
		return Decision{}, fmt.Errorf("decode simulation admission artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Decision{}, fmt.Errorf("simulation admission artifact contains trailing data")
	}
	if diagnostics := ValidateDecision(decision); len(diagnostics) != 0 {
		return Decision{}, fmt.Errorf("invalid simulation admission artifact: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	return CloneDecision(decision), nil
}

// ValidateDecision verifies the canonical artifact identity and all embedded
// digests without consulting mutable external state.
func ValidateDecision(decision Decision) []Diagnostic {
	diagnostics := []Diagnostic{}
	if decision.Schema != Schema || decision.Version != Version {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: "schema", Message: fmt.Sprintf("admission identity must be %s/%d", Schema, Version)})
	}
	if decision.Status != StatusAdmitted && decision.Status != StatusRefused {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: "status", Message: "admission status must be admitted or refused"})
	}
	for name, value := range map[string]string{
		"request_sha256": decision.RequestSHA256, "environment_sha256": decision.EnvironmentSHA256, "hash": decision.Hash,
	} {
		if !validSHA256(value) {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: name, Message: "admission digest must be canonical SHA-256"})
		}
	}
	if !slices.IsSortedFunc(decision.Analyses, compareAnalysisDecisions) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: "analyses", Message: "analysis decisions are not canonically ordered"})
	}
	if !slices.IsSortedFunc(decision.Models, compareModelDecisions) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: "models", Message: "model decisions are not canonically ordered"})
	}
	if !slices.IsSortedFunc(decision.RejectedModels, compareRejectedModels) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: "rejected_models", Message: "rejected model claims are not canonically ordered"})
	}
	if !slices.IsSortedFunc(decision.Diagnostics, compareDiagnostics) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: "diagnostics", Message: "admission diagnostics are not canonically ordered"})
	}
	if (decision.Status == StatusAdmitted && len(decision.Diagnostics) != 0) ||
		(decision.Status == StatusRefused && len(decision.Diagnostics) == 0) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: "status", Message: "admission status is inconsistent with its typed diagnostics"})
	}
	solvers := map[string]SolverDefinition{}
	for _, solver := range BuiltinSolvers() {
		solvers[solver.ID] = solver
	}
	for index, analysis := range decision.Analyses {
		solver, found := solvers[analysis.SolverID]
		if !found || analysis.SolverSHA256 != solver.SHA256 ||
			analysis.CanonicalKind != solver.Analysis ||
			(analysis.WorkflowModelID != "" && !slices.Contains(solver.WorkflowModels, analysis.WorkflowModelID)) {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeSolverUnavailable, Path: fmt.Sprintf("analyses[%d].solver_sha256", index), Message: "solver identity, digest, analysis, or workflow compatibility does not match the immutable registry"})
		}
		if analysis.Status != StatusAdmitted && analysis.Status != StatusRefused {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: fmt.Sprintf("analyses[%d].status", index), Message: "analysis admission status is invalid"})
		}
		dynamic := analysis.CanonicalKind == simmodel.AnalysisTransient || analysis.CanonicalKind == simmodel.AnalysisStartup ||
			analysis.CanonicalKind == simmodel.AnalysisDistortion || analysis.CanonicalKind == simmodel.AnalysisElectrothermal
		if len(analysis.Assertions) == 0 || len(analysis.OperatingCases) == 0 ||
			!slices.IsSorted(analysis.Assertions) || !slices.IsSorted(analysis.OperatingCases) ||
			analysis.DCSweep != (analysis.AuthoredKind == "dc_sweep") ||
			analysis.SmallSignalOperatingPoint != (analysis.CanonicalKind == simmodel.AnalysisACSweep || analysis.CanonicalKind == simmodel.AnalysisNoise || analysis.CanonicalKind == simmodel.AnalysisStability) ||
			analysis.ThermalBoundary != (analysis.CanonicalKind == simmodel.AnalysisThermal || analysis.CanonicalKind == simmodel.AnalysisElectrothermal) ||
			(analysis.PeriodicExcitation && !dynamic) {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: fmt.Sprintf("analyses[%d].shape", index), Message: "recorded analysis dependencies are noncanonical or inconsistent"})
		}
	}
	for index, model := range decision.Models {
		for name, value := range map[string]string{
			"parameters_sha256": model.ParametersSHA256, "model_claim_sha256": model.ModelClaimSHA256,
			"registry_source_sha256": model.RegistrySourceSHA256, "registry_record_sha256": model.RegistryRecordSHA256,
		} {
			if !validSHA256(value) {
				diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: fmt.Sprintf("models[%d].%s", index, name), Message: "model evidence digest must be canonical SHA-256"})
			}
		}
		parameterPayload := struct {
			Parameters []simmodel.NamedValue `json:"parameters,omitempty"`
			ValueSI    *float64              `json:"value_si,omitempty"`
		}{Parameters: normalizeNamedValues(model.Parameters), ValueSI: model.ValueSI}
		parameterHash, parameterHashErr := hashJSON(parameterPayload)
		if parameterHashErr != nil || model.ParametersSHA256 != parameterHash {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeInvalidModelParameters, Path: fmt.Sprintf("models[%d].parameters_sha256", index), Message: "parameter digest does not authenticate the selected scalar model parameters"})
		}
		claimHash, claimHashErr := hashJSON(model.ModelClaim)
		claimParametersHash, claimParametersHashErr := hashJSON(normalizeNamedValues(model.ModelClaim.Parameters))
		parametersHash, parametersHashErr := hashJSON(normalizeNamedValues(model.Parameters))
		if claimHashErr != nil || claimParametersHashErr != nil || parametersHashErr != nil ||
			model.ModelClaimSHA256 != claimHash ||
			model.ModelClaim.ModelID != model.ModelID ||
			claimParametersHash != parametersHash {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: fmt.Sprintf("models[%d].model_claim_sha256", index), Message: "model claim digest or projected identity does not authenticate the selected model claim"})
		}
		if model.CompatibilityStatus != StatusAdmitted ||
			(model.RegistrySourceKind != SourceBundled && model.RegistrySourceKind != SourceProject && model.RegistrySourceKind != SourceConfigured) {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: fmt.Sprintf("models[%d].compatibility_status", index), Message: "selected model compatibility or source trust classification is invalid"})
		}
		if evidenceDiagnostics := simmodel.ValidateCatalogEvidence(model.Family, []simmodel.CatalogEvidence{model.ModelClaim}); len(evidenceDiagnostics) != 0 {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeInvalidModelParameters, Path: fmt.Sprintf("models[%d].model_claim", index), Message: evidenceDiagnostics[0].Message})
		}
		if provenanceDiagnostics := simmodel.ValidateRequiredModelProvenance(&model.Provenance, model.RequiredAnalyses); len(provenanceDiagnostics) != 0 {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: fmt.Sprintf("models[%d].provenance", index), Message: provenanceDiagnostics[0].Message})
		}
		record := modelprovenance.Record{
			CatalogID: model.CatalogID, Family: model.Family, ModelID: model.ModelID,
			Provenance: model.Provenance,
		}
		recordData, recordMarshalErr := json.Marshal(record)
		recordHash, recordHashErr := hashJSON(json.RawMessage(recordData))
		if recordMarshalErr != nil || recordHashErr != nil || model.RegistryRecordSHA256 != recordHash {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: fmt.Sprintf("models[%d].registry_record_sha256", index), Message: "registry record digest does not authenticate the selected provenance record"})
		}
	}
	payload := decision
	payload.Hash = ""
	payloadHash, payloadHashErr := hashJSON(payload)
	if validSHA256(decision.Hash) && (payloadHashErr != nil || decision.Hash != payloadHash) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIncompatibleModel, Path: "hash", Message: "admission hash does not authenticate the complete artifact"})
	}
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return diagnostics
}

func CloneDecision(source Decision) Decision {
	clone := source
	clone.Analyses = make([]AnalysisDecision, len(source.Analyses))
	for index, analysis := range source.Analyses {
		clone.Analyses[index] = analysis
		clone.Analyses[index].Assertions = append([]string(nil), analysis.Assertions...)
		clone.Analyses[index].OperatingCases = append([]string(nil), analysis.OperatingCases...)
	}
	clone.Models = make([]ModelDecision, len(source.Models))
	for index, model := range source.Models {
		clone.Models[index] = model
		clone.Models[index].Parameters = append([]simmodel.NamedValue(nil), model.Parameters...)
		clone.Models[index].ModelClaim = simmodel.CloneCatalogEvidence(model.ModelClaim)
		clone.Models[index].RequiredAnalyses = append([]string(nil), model.RequiredAnalyses...)
		clone.Models[index].Provenance = cloneModelProvenance(model.Provenance)
		if model.ValueSI != nil {
			value := *model.ValueSI
			clone.Models[index].ValueSI = &value
		}
	}
	clone.RejectedModels = append([]RejectedModelClaim(nil), source.RejectedModels...)
	clone.Diagnostics = append([]Diagnostic(nil), source.Diagnostics...)
	return clone
}

func cloneModelProvenance(source simmodel.ModelProvenance) simmodel.ModelProvenance {
	clone := source
	clone.AllowedAnalyses = append([]string(nil), source.AllowedAnalyses...)
	if source.MinTemperatureC != nil {
		value := *source.MinTemperatureC
		clone.MinTemperatureC = &value
	}
	if source.MaxTemperatureC != nil {
		value := *source.MaxTemperatureC
		clone.MaxTemperatureC = &value
	}
	return clone
}
