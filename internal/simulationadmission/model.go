// Package simulationadmission proves that a requested electrical analysis has
// an exact trusted model set and an enabled deterministic solver before
// numerical simulation begins.
package simulationadmission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

const (
	Schema  = "kicadai.simulation-admission.v1"
	Version = 1

	MaxSources           = 16
	MaxAnalyses          = 32
	MaxAssertions        = 256
	MaxComponents        = 128
	MaxEnabledSolvers    = 32
	MaxDiagnostics       = 256
	MaxSourceIdentityLen = 256
	MaxDecisionBytes     = 8 * 1024 * 1024
)

type Status string

const (
	StatusAdmitted Status = "admitted"
	StatusRefused  Status = "refused"
)

type DiagnosticCode string

const (
	CodeMissingModel              DiagnosticCode = "MISSING_MODEL"
	CodeIncompatibleModel         DiagnosticCode = "INCOMPATIBLE_MODEL"
	CodeMissingAnalysisDefinition DiagnosticCode = "MISSING_ANALYSIS_DEFINITION"
	CodeUnsupportedAnalysis       DiagnosticCode = "UNSUPPORTED_ANALYSIS"
	CodeSolverUnavailable         DiagnosticCode = "SOLVER_UNAVAILABLE"
	CodeSolverModelIncompatible   DiagnosticCode = "SOLVER_MODEL_INCOMPATIBLE"
	CodeInvalidModelParameters    DiagnosticCode = "INVALID_MODEL_PARAMETERS"
)

type SourceKind string

const (
	SourceBundled    SourceKind = "bundled"
	SourceProject    SourceKind = "project"
	SourceConfigured SourceKind = "configured"
)

type Source struct {
	ID       string                   `json:"id"`
	Kind     SourceKind               `json:"kind"`
	SHA256   string                   `json:"sha256"`
	Registry modelprovenance.Registry `json:"registry"`
}

type Environment struct {
	Sources        []Source `json:"sources"`
	EnabledSolvers []string `json:"enabled_solvers,omitempty"`
}

type AnalysisRequirement struct {
	ID                        string   `json:"id"`
	AuthoredKind              string   `json:"authored_kind"`
	CanonicalKind             string   `json:"canonical_kind,omitempty"`
	Assertions                []string `json:"assertions"`
	OperatingCases            []string `json:"operating_cases"`
	DCSweep                   bool     `json:"dc_sweep,omitempty"`
	PeriodicExcitation        bool     `json:"periodic_excitation,omitempty"`
	ThermalBoundary           bool     `json:"thermal_boundary,omitempty"`
	SmallSignalOperatingPoint bool     `json:"small_signal_operating_point,omitempty"`
}

type CatalogModel struct {
	CatalogID string `json:"catalog_id"`
	Family    string `json:"family"`
	ModelID   string `json:"model_id"`
}

type Request struct {
	Analyses        []AnalysisRequirement        `json:"analyses"`
	InventoryModels []CatalogModel               `json:"inventory_models,omitempty"`
	Components      []simmodel.ComponentEvidence `json:"-"`
}

type Diagnostic struct {
	Code       DiagnosticCode `json:"code"`
	Path       string         `json:"path"`
	Analysis   string         `json:"analysis,omitempty"`
	Component  string         `json:"component,omitempty"`
	Message    string         `json:"message"`
	Suggestion string         `json:"suggestion,omitempty"`
}

type AnalysisDecision struct {
	ID                        string   `json:"id"`
	AuthoredKind              string   `json:"authored_kind"`
	CanonicalKind             string   `json:"canonical_kind"`
	Assertions                []string `json:"assertions"`
	OperatingCases            []string `json:"operating_cases"`
	DCSweep                   bool     `json:"dc_sweep,omitempty"`
	PeriodicExcitation        bool     `json:"periodic_excitation,omitempty"`
	ThermalBoundary           bool     `json:"thermal_boundary,omitempty"`
	SmallSignalOperatingPoint bool     `json:"small_signal_operating_point,omitempty"`
	SolverID                  string   `json:"solver_id"`
	SolverSHA256              string   `json:"solver_sha256"`
	WorkflowModelID           string   `json:"workflow_model_id,omitempty"`
	Status                    Status   `json:"status"`
	Reason                    string   `json:"reason"`
}

type ModelDecision struct {
	AnalysisID           string                   `json:"analysis_id"`
	Component            string                   `json:"component"`
	CatalogID            string                   `json:"catalog_id"`
	Family               string                   `json:"family"`
	ModelID              string                   `json:"model_id"`
	Parameters           []simmodel.NamedValue    `json:"parameters,omitempty"`
	ValueSI              *float64                 `json:"value_si,omitempty"`
	ParametersSHA256     string                   `json:"parameters_sha256"`
	ModelClaim           simmodel.CatalogEvidence `json:"model_claim"`
	ModelClaimSHA256     string                   `json:"model_claim_sha256"`
	Provenance           simmodel.ModelProvenance `json:"provenance"`
	RegistrySourceID     string                   `json:"registry_source_id"`
	RegistrySourceKind   SourceKind               `json:"registry_source_kind"`
	RegistrySourceSHA256 string                   `json:"registry_source_sha256"`
	RegistryRecordSHA256 string                   `json:"registry_record_sha256"`
	RequiredAnalyses     []string                 `json:"required_analyses"`
	CompatibilityStatus  Status                   `json:"compatibility_status"`
	CompatibilityReason  string                   `json:"compatibility_reason"`
}

type RejectedModelClaim struct {
	AnalysisID string         `json:"analysis_id"`
	Component  string         `json:"component"`
	CatalogID  string         `json:"catalog_id"`
	ModelID    string         `json:"model_id"`
	Code       DiagnosticCode `json:"code"`
	Reason     string         `json:"reason"`
}

type Decision struct {
	Schema            string               `json:"schema"`
	Version           int                  `json:"version"`
	Status            Status               `json:"status"`
	RequestSHA256     string               `json:"request_sha256"`
	EnvironmentSHA256 string               `json:"environment_sha256"`
	Analyses          []AnalysisDecision   `json:"analyses"`
	Models            []ModelDecision      `json:"models"`
	RejectedModels    []RejectedModelClaim `json:"rejected_models"`
	Diagnostics       []Diagnostic         `json:"diagnostics"`
	Hash              string               `json:"hash"`
}

func NewSource(id string, kind SourceKind, registry modelprovenance.Registry) (Source, error) {
	source := Source{ID: strings.TrimSpace(id), Kind: kind, Registry: modelprovenance.Normalize(registry)}
	digest, err := modelprovenance.Hash(source.Registry)
	if err != nil {
		return Source{}, err
	}
	source.SHA256 = digest
	if diagnostics := validateSource(source); len(diagnostics) != 0 {
		return Source{}, fmt.Errorf("invalid model source: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	return source, nil
}

// SplitMergedSources preserves the exact origin of a trusted base registry and
// the records added by one reviewed project or configured overlay. A merged
// registry may extend the base but may not silently remove or rewrite a base
// model identity.
func SplitMergedSources(
	baseID string,
	baseKind SourceKind,
	base modelprovenance.Registry,
	overlayID string,
	overlayKind SourceKind,
	merged modelprovenance.Registry,
) ([]Source, error) {
	base = modelprovenance.Normalize(base)
	merged = modelprovenance.Normalize(merged)
	if _, err := modelprovenance.Hash(merged); err != nil {
		return nil, fmt.Errorf("invalid merged model registry: %w", err)
	}
	baseByKey := make(map[string]modelprovenance.Record, len(base.Records))
	for _, record := range base.Records {
		baseByKey[record.CatalogID+"\x00"+record.ModelID] = record
	}
	mergedByKey := make(map[string]modelprovenance.Record, len(merged.Records))
	for _, record := range merged.Records {
		mergedByKey[record.CatalogID+"\x00"+record.ModelID] = record
	}
	for key, record := range baseByKey {
		candidate, found := mergedByKey[key]
		candidateHash, candidateErr := hashJSON(candidate)
		recordHash, recordErr := hashJSON(record)
		if !found || candidateErr != nil || recordErr != nil || candidateHash != recordHash {
			return nil, fmt.Errorf("merged model registry removed or rewrote trusted base identity %q", key)
		}
	}
	baseSource, err := NewSource(baseID, baseKind, base)
	if err != nil {
		return nil, err
	}
	delta := modelprovenance.Registry{Schema: modelprovenance.Schema, Version: modelprovenance.Version}
	for key, record := range mergedByKey {
		if _, inherited := baseByKey[key]; !inherited {
			delta.Records = append(delta.Records, record)
		}
	}
	if len(delta.Records) == 0 {
		return []Source{baseSource}, nil
	}
	overlaySource, err := NewSource(overlayID, overlayKind, delta)
	if err != nil {
		return nil, err
	}
	return NormalizeEnvironment(Environment{Sources: []Source{baseSource, overlaySource}}).Sources, nil
}

// NormalizeRequest returns an ownership-isolated canonical request. The deep
// component clone is deliberate: callers may reuse or mutate synthesis graph
// evidence after admission without changing a prepared decision or its hash.
func NormalizeRequest(request Request) Request {
	result := Request{
		Analyses:        append([]AnalysisRequirement(nil), request.Analyses...),
		InventoryModels: append([]CatalogModel(nil), request.InventoryModels...),
		Components:      cloneComponents(request.Components),
	}
	for index := range result.Analyses {
		analysis := &result.Analyses[index]
		analysis.ID = strings.TrimSpace(analysis.ID)
		analysis.AuthoredKind = strings.TrimSpace(analysis.AuthoredKind)
		analysis.CanonicalKind = strings.TrimSpace(analysis.CanonicalKind)
		analysis.Assertions = normalizeStrings(analysis.Assertions)
		analysis.OperatingCases = normalizeStrings(analysis.OperatingCases)
	}
	for index := range result.InventoryModels {
		model := &result.InventoryModels[index]
		model.CatalogID = strings.TrimSpace(model.CatalogID)
		model.Family = strings.TrimSpace(model.Family)
		model.ModelID = strings.TrimSpace(model.ModelID)
	}
	slices.SortStableFunc(result.Analyses, compareAnalysisRequirements)
	slices.SortStableFunc(result.InventoryModels, compareCatalogModels)
	result.InventoryModels = slices.CompactFunc(result.InventoryModels, func(left, right CatalogModel) bool {
		return compareCatalogModels(left, right) == 0
	})
	slices.SortStableFunc(result.Components, func(left, right simmodel.ComponentEvidence) int {
		return strings.Compare(strings.TrimSpace(left.InstanceID), strings.TrimSpace(right.InstanceID))
	})
	return result
}

func NormalizeEnvironment(environment Environment) Environment {
	result := Environment{
		Sources:        append([]Source(nil), environment.Sources...),
		EnabledSolvers: normalizeStrings(environment.EnabledSolvers),
	}
	for index := range result.Sources {
		result.Sources[index].ID = strings.TrimSpace(result.Sources[index].ID)
		result.Sources[index].SHA256 = strings.TrimSpace(result.Sources[index].SHA256)
		result.Sources[index].Registry = modelprovenance.Normalize(result.Sources[index].Registry)
	}
	slices.SortStableFunc(result.Sources, compareSources)
	return result
}

func requestHash(request Request) (string, error) {
	normalized := NormalizeRequest(request)
	type hashRequest struct {
		Analyses        []AnalysisRequirement `json:"analyses"`
		InventoryModels []CatalogModel        `json:"inventory_models,omitempty"`
		Components      []hashComponent       `json:"components,omitempty"`
	}
	payload := hashRequest{
		Analyses: normalized.Analyses, InventoryModels: normalized.InventoryModels,
		Components: hashComponents(normalized.Components),
	}
	return hashJSON(payload)
}

func environmentHash(environment Environment) (string, error) {
	return hashJSON(NormalizeEnvironment(environment))
}

func finalize(decision Decision) Decision {
	decision.Schema = Schema
	decision.Version = Version
	slices.SortStableFunc(decision.Analyses, compareAnalysisDecisions)
	slices.SortStableFunc(decision.Models, compareModelDecisions)
	slices.SortStableFunc(decision.RejectedModels, compareRejectedModels)
	slices.SortStableFunc(decision.Diagnostics, compareDiagnostics)
	if len(decision.Diagnostics) == 0 {
		decision.Status = StatusAdmitted
	} else {
		decision.Status = StatusRefused
	}
	decision.Hash = ""
	hash, err := hashJSON(decision)
	if err != nil {
		decision.Analyses = nil
		decision.Models = nil
		decision.RejectedModels = nil
		decision.Diagnostics = append(decision.Diagnostics, Diagnostic{
			Code: CodeInvalidModelParameters, Path: "decision", Message: "admission evidence is not canonically serializable",
		})
		slices.SortStableFunc(decision.Diagnostics, compareDiagnostics)
		decision.Status = StatusRefused
		decision.Hash = ""
		if fallbackHash, fallbackErr := hashJSON(decision); fallbackErr == nil {
			hash = fallbackHash
		} else {
			hash = ""
		}
	}
	decision.Hash = hash
	return decision
}

func hashFailure(kind string, err error) string {
	digest := sha256.Sum256([]byte("kicadai.simulation-admission.hash-failure.v1\x00" + kind + "\x00" + err.Error()))
	return hex.EncodeToString(digest[:])
}

func hashJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("canonical JSON: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func normalizeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func cloneComponents(source []simmodel.ComponentEvidence) []simmodel.ComponentEvidence {
	result := make([]simmodel.ComponentEvidence, len(source))
	for index, component := range source {
		result[index] = component
		result[index].ModelClaims = make([]simmodel.CatalogEvidence, len(component.ModelClaims))
		for claimIndex, claim := range component.ModelClaims {
			result[index].ModelClaims[claimIndex] = simmodel.CloneCatalogEvidence(claim)
		}
		result[index].Connections = append([]simmodel.ConnectionEvidence(nil), component.Connections...)
		result[index].Uncertainties = append([]simmodel.Uncertainty(nil), component.Uncertainties...)
	}
	return result
}

type hashComponent struct {
	InstanceID        string                        `json:"instance_id"`
	PhysicalComponent string                        `json:"physical_component,omitempty"`
	CatalogID         string                        `json:"catalog_id"`
	Family            string                        `json:"family"`
	Usage             string                        `json:"usage,omitempty"`
	ValueSI           *float64                      `json:"value_si,omitempty"`
	ModelClaims       []simmodel.CatalogEvidence    `json:"model_claims"`
	Connections       []simmodel.ConnectionEvidence `json:"connections"`
}

func hashComponents(components []simmodel.ComponentEvidence) []hashComponent {
	result := make([]hashComponent, 0, len(components))
	for _, component := range components {
		item := hashComponent{
			InstanceID: strings.TrimSpace(component.InstanceID), PhysicalComponent: strings.TrimSpace(component.PhysicalComponent),
			CatalogID: strings.TrimSpace(component.CatalogID), Family: strings.TrimSpace(component.Family), Usage: strings.TrimSpace(component.Usage),
			ModelClaims: append([]simmodel.CatalogEvidence(nil), component.ModelClaims...),
			Connections: append([]simmodel.ConnectionEvidence(nil), component.Connections...),
		}
		if component.HasValueSI {
			value := component.ValueSI
			item.ValueSI = &value
		}
		for index := range item.ModelClaims {
			item.ModelClaims[index].Parameters = normalizeNamedValues(item.ModelClaims[index].Parameters)
		}
		slices.SortStableFunc(item.ModelClaims, func(left, right simmodel.CatalogEvidence) int { return strings.Compare(left.ModelID, right.ModelID) })
		slices.SortStableFunc(item.Connections, func(left, right simmodel.ConnectionEvidence) int {
			if order := strings.Compare(left.Function, right.Function); order != 0 {
				return order
			}
			if order := strings.Compare(left.UnitID, right.UnitID); order != 0 {
				return order
			}
			return strings.Compare(left.Net, right.Net)
		})
		result = append(result, item)
	}
	return result
}

func normalizeNamedValues(values []simmodel.NamedValue) []simmodel.NamedValue {
	result := append([]simmodel.NamedValue(nil), values...)
	for index := range result {
		result[index].Name = strings.TrimSpace(result[index].Name)
	}
	slices.SortStableFunc(result, func(left, right simmodel.NamedValue) int { return strings.Compare(left.Name, right.Name) })
	return result
}

func compareAnalysisRequirements(left, right AnalysisRequirement) int {
	if order := strings.Compare(left.CanonicalKind, right.CanonicalKind); order != 0 {
		return order
	}
	if order := strings.Compare(left.AuthoredKind, right.AuthoredKind); order != 0 {
		return order
	}
	return strings.Compare(left.ID, right.ID)
}

func compareCatalogModels(left, right CatalogModel) int {
	if order := strings.Compare(left.CatalogID, right.CatalogID); order != 0 {
		return order
	}
	if order := strings.Compare(left.ModelID, right.ModelID); order != 0 {
		return order
	}
	return strings.Compare(left.Family, right.Family)
}

func compareSources(left, right Source) int {
	if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
		return order
	}
	if order := strings.Compare(left.ID, right.ID); order != 0 {
		return order
	}
	return strings.Compare(left.SHA256, right.SHA256)
}

func compareAnalysisDecisions(left, right AnalysisDecision) int {
	if order := strings.Compare(left.CanonicalKind, right.CanonicalKind); order != 0 {
		return order
	}
	return strings.Compare(left.ID, right.ID)
}

func compareModelDecisions(left, right ModelDecision) int {
	if order := strings.Compare(left.AnalysisID, right.AnalysisID); order != 0 {
		return order
	}
	if order := strings.Compare(left.Component, right.Component); order != 0 {
		return order
	}
	return strings.Compare(left.ModelID, right.ModelID)
}

func compareRejectedModels(left, right RejectedModelClaim) int {
	if order := strings.Compare(left.AnalysisID, right.AnalysisID); order != 0 {
		return order
	}
	if order := strings.Compare(left.Component, right.Component); order != 0 {
		return order
	}
	if order := strings.Compare(left.ModelID, right.ModelID); order != 0 {
		return order
	}
	return strings.Compare(string(left.Code), string(right.Code))
}

func compareDiagnostics(left, right Diagnostic) int {
	if order := strings.Compare(string(left.Code), string(right.Code)); order != 0 {
		return order
	}
	if order := strings.Compare(left.Path, right.Path); order != 0 {
		return order
	}
	if order := strings.Compare(left.Analysis, right.Analysis); order != 0 {
		return order
	}
	if order := strings.Compare(left.Component, right.Component); order != 0 {
		return order
	}
	return strings.Compare(left.Message, right.Message)
}
