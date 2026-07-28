// Package capabilityexpansion converts fail-closed capability gaps into
// source-backed, quarantined promotion packages.
package capabilityexpansion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/capabilitygate"
)

const (
	PlanSchema              = "kicadai.capability-expansion-plan.v1"
	CandidateRegistrySchema = "kicadai.capability-candidate-registry.v1"
	PromotionBundleSchema   = "kicadai.capability-promotion-bundle.v1"
	SupportedRegistrySchema = "kicadai.supported-capability-registry.v1"
	ApprovalSchema          = "kicadai.capability-promotion-approval.v1"
	PolicyVersion           = "evidence-driven-capability-expansion-v1"
	MaxExpansionNeeds       = 256
	MaxCandidateSources     = 512
	MaxCandidateSourceBytes = 64 << 20
)

type NeedKind string

const (
	NeedArchitecture NeedKind = "architecture"
	NeedComponent    NeedKind = "component"
	NeedModel        NeedKind = "model"
	NeedPhysicalRule NeedKind = "physical_rule"
	NeedRouting      NeedKind = "routing"
	NeedVerification NeedKind = "verification"
)

type SourceKind string

const (
	SourceDatasheet            SourceKind = "datasheet"
	SourceEngineeringReference SourceKind = "engineering_reference"
	SourceLibraryBinding       SourceKind = "library_binding"
	SourceModel                SourceKind = "model"
	SourceStandard             SourceKind = "standard"
	SourceVerification         SourceKind = "verification"
)

type PackageStatus string

const (
	StatusExperimental PackageStatus = "experimental"
	StatusReviewReady  PackageStatus = "review_ready"
	StatusSupported    PackageStatus = "supported"
)

type ExpansionPlan struct {
	Schema               string                        `json:"schema"`
	PolicyVersion        string                        `json:"policy_version"`
	SourceAssessmentHash string                        `json:"source_assessment_hash"`
	SourceClassification capabilitygate.Classification `json:"source_classification"`
	Domains              []string                      `json:"domains"`
	Needs                []ExpansionNeed               `json:"needs"`
	Risks                []string                      `json:"risks,omitempty"`
	Hash                 string                        `json:"hash,omitempty"`
}

type ExpansionNeed struct {
	ID                     string                         `json:"id"`
	CapabilityID           string                         `json:"capability_id"`
	Kind                   NeedKind                       `json:"kind"`
	RequirementKind        capabilitygate.RequirementKind `json:"requirement_kind"`
	Stage                  string                         `json:"stage,omitempty"`
	GapCodes               []string                       `json:"gap_codes"`
	RequiredSourceKinds    []SourceKind                   `json:"required_source_kinds"`
	RequiredArtifact       string                         `json:"required_artifact"`
	RequiredPromotionGates []string                       `json:"required_promotion_gates"`
	Action                 string                         `json:"action"`
}

type SourceInput struct {
	ID             string     `json:"id"`
	Kind           SourceKind `json:"kind"`
	Publisher      string     `json:"publisher"`
	Locator        string     `json:"locator"`
	License        string     `json:"license,omitempty"`
	Claims         []string   `json:"claims"`
	ExpectedSHA256 string     `json:"expected_sha256"`
	Content        []byte     `json:"-"`
}

type SourceRecord struct {
	ID        string     `json:"id"`
	Kind      SourceKind `json:"kind"`
	Publisher string     `json:"publisher"`
	Locator   string     `json:"locator"`
	License   string     `json:"license,omitempty"`
	Claims    []string   `json:"claims"`
	SHA256    string     `json:"sha256"`
	Bytes     int        `json:"bytes"`
}

type DeclarativeProviderRecord struct {
	ID          string                               `json:"id"`
	Revision    string                               `json:"revision"`
	Capability  string                               `json:"capability"`
	NeedID      string                               `json:"need_id"`
	EvidenceIDs []string                             `json:"evidence_ids"`
	Expansion   architecturesearch.ProviderExpansion `json:"expansion"`
}

type CapabilityArtifact struct {
	ID           string          `json:"id"`
	NeedID       string          `json:"need_id"`
	Kind         NeedKind        `json:"kind"`
	ArtifactType string          `json:"artifact_type"`
	SHA256       string          `json:"sha256"`
	EvidenceIDs  []string        `json:"evidence_ids"`
	Payload      json.RawMessage `json:"payload"`
}

type GeneratedCaseKind string

const (
	CaseRepresentative      GeneratedCaseKind = "representative"
	CaseMissingEvidence     GeneratedCaseKind = "adversarial_missing_evidence"
	CaseConflictingEvidence GeneratedCaseKind = "adversarial_conflicting_evidence"
	CaseIrrelevantEvidence  GeneratedCaseKind = "adversarial_irrelevant_evidence"
	CaseFabricatedEvidence  GeneratedCaseKind = "adversarial_fabricated_evidence"
)

type GeneratedCase struct {
	ID            string            `json:"id"`
	NeedID        string            `json:"need_id"`
	Kind          GeneratedCaseKind `json:"kind"`
	ExpectedPass  bool              `json:"expected_pass"`
	RequiredGates []string          `json:"required_gates"`
}

type CandidateRegistry struct {
	Schema        string                      `json:"schema"`
	PolicyVersion string                      `json:"policy_version"`
	Status        PackageStatus               `json:"status"`
	Plan          ExpansionPlan               `json:"plan"`
	Sources       []SourceRecord              `json:"sources"`
	Artifacts     []CapabilityArtifact        `json:"artifacts"`
	Providers     []DeclarativeProviderRecord `json:"providers"`
	Assumptions   []string                    `json:"assumptions,omitempty"`
	Risks         []string                    `json:"risks,omitempty"`
	Cases         []GeneratedCase             `json:"cases"`
	Hash          string                      `json:"hash,omitempty"`
}

type GateResult struct {
	CaseID         string `json:"case_id"`
	Gate           string `json:"gate"`
	Passed         bool   `json:"passed"`
	EvidencePath   string `json:"evidence_path"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	Summary        string `json:"summary,omitempty"`
}

type PromotionBundle struct {
	Schema         string            `json:"schema"`
	PolicyVersion  string            `json:"policy_version"`
	Status         PackageStatus     `json:"status"`
	Candidate      CandidateRegistry `json:"candidate"`
	Results        []GateResult      `json:"results"`
	RemainingRisks []string          `json:"remaining_risks,omitempty"`
	Hash           string            `json:"hash,omitempty"`
}

type PromotionApproval struct {
	Schema       string `json:"schema"`
	BundleHash   string `json:"bundle_hash"`
	Decision     string `json:"decision"`
	Reviewer     string `json:"reviewer"`
	ReviewRef    string `json:"review_ref"`
	ReviewSHA256 string `json:"review_sha256"`
}

type SupportedCapability struct {
	Capability   string                     `json:"capability"`
	Kind         NeedKind                   `json:"kind"`
	Artifact     CapabilityArtifact         `json:"artifact"`
	Provider     *DeclarativeProviderRecord `json:"provider,omitempty"`
	SourceHashes []string                   `json:"source_hashes"`
	BundleHash   string                     `json:"bundle_hash"`
	ReviewRef    string                     `json:"review_ref"`
	ReviewSHA256 string                     `json:"review_sha256"`
}

type SupportedRegistry struct {
	Schema        string                `json:"schema"`
	PolicyVersion string                `json:"policy_version"`
	Status        PackageStatus         `json:"status"`
	Capabilities  []SupportedCapability `json:"capabilities"`
	Hash          string                `json:"hash,omitempty"`
}

func digest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func normalizedSourceKinds(values []SourceKind) []SourceKind {
	result := append([]SourceKind(nil), values...)
	slices.Sort(result)
	return slices.Compact(result)
}

func validNeedKind(kind NeedKind) bool {
	switch kind {
	case NeedArchitecture, NeedComponent, NeedModel, NeedPhysicalRule, NeedRouting, NeedVerification:
		return true
	default:
		return false
	}
}

func validSourceKind(kind SourceKind) bool {
	switch kind {
	case SourceDatasheet, SourceEngineeringReference, SourceLibraryBinding, SourceModel, SourceStandard, SourceVerification:
		return true
	default:
		return false
	}
}

type declarativeProvider struct {
	record  DeclarativeProviderRecord
	sources []string
}

func (provider declarativeProvider) Descriptor() architecturesearch.ProviderDescriptor {
	return architecturesearch.ProviderDescriptor{
		ID: provider.record.ID, Revision: provider.record.Revision,
		Capabilities: []string{provider.record.Capability},
		Evidence: architecturesearch.ContractEvidence{
			Confidence: architecturesearch.EvidenceVerified,
			Sources:    append([]string(nil), provider.sources...),
		},
	}
}

func (provider declarativeProvider) Expand(ctx context.Context, request architecturesearch.ProviderRequest) ([]architecturesearch.ProviderExpansion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Capability) != provider.record.Capability {
		return nil, nil
	}
	expansion := provider.record.Expansion
	expansion.OfferedPorts = make([]architecturesearch.RoleContract, 0, len(request.Ports))
	for _, required := range request.Ports {
		index := slices.IndexFunc(provider.record.Expansion.OfferedPorts, func(offered architecturesearch.RoleContract) bool {
			return offered.Role == required.Role
		})
		if index < 0 {
			return nil, fmt.Errorf("promoted provider %q lacks reviewed role %q", provider.record.ID, required.Role)
		}
		offered := provider.record.Expansion.OfferedPorts[index]
		offered.Anchor = required.Anchor
		expansion.OfferedPorts = append(expansion.OfferedPorts, offered)
	}
	return []architecturesearch.ProviderExpansion{expansion}, nil
}

// Providers instantiates promoted declarative providers for a fresh
// architecture search.
func Providers(registry SupportedRegistry) ([]architecturesearch.FragmentProvider, error) {
	if err := ValidateSupportedRegistry(registry); err != nil {
		return nil, err
	}
	providers := make([]architecturesearch.FragmentProvider, 0, len(registry.Capabilities))
	for _, capability := range registry.Capabilities {
		if capability.Provider == nil {
			continue
		}
		sources := make([]string, 0, len(capability.SourceHashes))
		for _, hash := range capability.SourceHashes {
			sources = append(sources, "capability-expansion:sha256:"+hash)
		}
		providers = append(providers, declarativeProvider{record: *capability.Provider, sources: sources})
	}
	return providers, nil
}

func MarshalJSONStable(value any) ([]byte, error) {
	switch typed := value.(type) {
	case ExpansionPlan:
		if err := ValidatePlan(typed); err != nil {
			return nil, err
		}
	case *ExpansionPlan:
		if typed == nil {
			return nil, fmt.Errorf("capability expansion plan is nil")
		}
		if err := ValidatePlan(*typed); err != nil {
			return nil, err
		}
	case CandidateRegistry:
		if err := ValidateCandidate(typed); err != nil {
			return nil, err
		}
	case *CandidateRegistry:
		if typed == nil {
			return nil, fmt.Errorf("capability candidate registry is nil")
		}
		if err := ValidateCandidate(*typed); err != nil {
			return nil, err
		}
	case PromotionBundle:
		if err := ValidateBundle(typed); err != nil {
			return nil, err
		}
	case *PromotionBundle:
		if typed == nil {
			return nil, fmt.Errorf("capability promotion bundle is nil")
		}
		if err := ValidateBundle(*typed); err != nil {
			return nil, err
		}
	case SupportedRegistry:
		if err := ValidateSupportedRegistry(typed); err != nil {
			return nil, err
		}
	case *SupportedRegistry:
		if typed == nil {
			return nil, fmt.Errorf("supported capability registry is nil")
		}
		if err := ValidateSupportedRegistry(*typed); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported capability expansion artifact %T", value)
	}
	return json.Marshal(value)
}
