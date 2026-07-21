package behavioralintent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
)

var (
	ErrSourceUnavailable       = errors.New("behavioral intent source is unavailable")
	ErrCapabilitiesUnavailable = errors.New("behavioral generation capabilities are unavailable")
)

type ProviderContext struct {
	TargetContract   string            `json:"target_contract"`
	Source           Source            `json:"source"`
	CapabilitySHA256 string            `json:"capability_sha256"`
	Capabilities     json.RawMessage   `json:"capabilities"`
	Policy           []string          `json:"policy"`
	FollowUp         *ProviderFollowUp `json:"follow_up,omitempty"`
}

type ProviderFollowUp struct {
	PriorProposal    Proposal `json:"prior_proposal"`
	PriorCompilation Result   `json:"prior_compilation"`
	Input            FollowUp `json:"input"`
}

type InstalledCapabilities struct {
	Schema              string          `json:"schema"`
	Version             int             `json:"version"`
	Architecture        json.RawMessage `json:"architecture"`
	CatalogSHA256       string          `json:"catalog_sha256"`
	ModelRegistrySHA256 string          `json:"model_registry_sha256"`
	TrustedAnalyses     []string        `json:"trusted_analyses"`
}

// BuildInstalledCapabilities binds the semantic architecture vocabulary to
// the exact catalog and reviewed model registry available to this process.
func BuildInstalledCapabilities(architecture json.RawMessage, catalogSHA256, modelRegistrySHA256 string, trustedAnalyses []string) (json.RawMessage, error) {
	architecture = bytes.TrimSpace(architecture)
	if len(architecture) == 0 || !json.Valid(architecture) || !sha256Pattern.MatchString(strings.TrimSpace(catalogSHA256)) || !sha256Pattern.MatchString(strings.TrimSpace(modelRegistrySHA256)) {
		return nil, ErrCapabilitiesUnavailable
	}
	var semantic architecturesearch.SemanticCapabilityDocument
	decoder := json.NewDecoder(bytes.NewReader(architecture))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&semantic); err != nil || architecturesearch.ValidateSemanticCapabilities(semantic) != nil {
		return nil, ErrCapabilitiesUnavailable
	}
	trustedAnalyses = trimmedSortedUnique(trustedAnalyses)
	if len(trustedAnalyses) == 0 {
		return nil, ErrCapabilitiesUnavailable
	}
	trusted := map[string]bool{}
	for _, analysis := range trustedAnalyses {
		if !validSemanticID(analysis) {
			return nil, ErrCapabilitiesUnavailable
		}
		trusted[analysis] = true
	}
	for _, metric := range semantic.BehavioralMetrics {
		if !trusted[metric.Analysis] {
			return nil, ErrCapabilitiesUnavailable
		}
	}
	snapshot := InstalledCapabilities{
		Schema: "kicadai.installed-behavioral-capabilities.v1", Version: 1,
		Architecture: append(json.RawMessage(nil), architecture...), CatalogSHA256: strings.TrimSpace(catalogSHA256),
		ModelRegistrySHA256: strings.TrimSpace(modelRegistrySHA256), TrustedAnalyses: slices.Clone(trustedAnalyses),
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// BuildProviderContext combines compiler-owned source statements with a
// caller-supplied snapshot of the installed semantic capabilities. Missing or
// malformed capability evidence fails closed before model execution.
func BuildProviderContext(prompt string, capabilities json.RawMessage) (string, error) {
	source := PrepareSource(prompt)
	if len(source.Statements) == 0 {
		return "", ErrSourceUnavailable
	}
	capabilities = bytes.TrimSpace(capabilities)
	if len(capabilities) == 0 || bytes.Equal(capabilities, []byte("null")) || !json.Valid(capabilities) {
		return "", ErrCapabilitiesUnavailable
	}
	if _, err := ValidateInstalledCapabilities(capabilities); err != nil {
		return "", err
	}
	hash := sha256.Sum256(capabilities)
	context := ProviderContext{
		TargetContract:   "kicadai.open-set-requirement.v3",
		Source:           source,
		CapabilitySHA256: hex.EncodeToString(hash[:]),
		Capabilities:     append(json.RawMessage(nil), capabilities...),
		Policy: []string{
			"compile behavior, interfaces, operating conditions, tolerances, safety limits, and board constraints only",
			"do not name topology, components, parts, nets, coordinates, layers, routes, solver controls, or fixture identities",
			"account for every source statement exactly once and do not invent unrequested objectives, operating cases, or guarantees",
			"emit the minimum blocking clarification when a required choice is ambiguous",
			"emit a capability gap instead of a requirement when trusted generation or verification capability is unavailable",
		},
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ValidateInstalledCapabilities strict-decodes and verifies the complete
// immutable capability snapshot accepted at the provider trust boundary.
func ValidateInstalledCapabilities(capabilities json.RawMessage) (InstalledCapabilities, error) {
	capabilities = bytes.TrimSpace(capabilities)
	if !json.Valid(capabilities) {
		return InstalledCapabilities{}, ErrCapabilitiesUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(capabilities))
	decoder.DisallowUnknownFields()
	var snapshot InstalledCapabilities
	if err := decoder.Decode(&snapshot); err != nil {
		return InstalledCapabilities{}, ErrCapabilitiesUnavailable
	}
	if snapshot.Schema != "kicadai.installed-behavioral-capabilities.v1" || snapshot.Version != 1 || !sha256Pattern.MatchString(snapshot.CatalogSHA256) || !sha256Pattern.MatchString(snapshot.ModelRegistrySHA256) || len(snapshot.TrustedAnalyses) == 0 || !slices.IsSorted(snapshot.TrustedAnalyses) {
		return InstalledCapabilities{}, ErrCapabilitiesUnavailable
	}
	for index, analysis := range snapshot.TrustedAnalyses {
		if !validSemanticID(analysis) || (index > 0 && snapshot.TrustedAnalyses[index-1] == analysis) {
			return InstalledCapabilities{}, ErrCapabilitiesUnavailable
		}
	}
	var semantic architecturesearch.SemanticCapabilityDocument
	architectureDecoder := json.NewDecoder(bytes.NewReader(snapshot.Architecture))
	architectureDecoder.DisallowUnknownFields()
	if err := architectureDecoder.Decode(&semantic); err != nil || architecturesearch.ValidateSemanticCapabilities(semantic) != nil {
		return InstalledCapabilities{}, ErrCapabilitiesUnavailable
	}
	trusted := map[string]bool{}
	for _, analysis := range snapshot.TrustedAnalyses {
		trusted[analysis] = true
	}
	for _, metric := range semantic.BehavioralMetrics {
		if !trusted[metric.Analysis] {
			return InstalledCapabilities{}, ErrCapabilitiesUnavailable
		}
	}
	return snapshot, nil
}
