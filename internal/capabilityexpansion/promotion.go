package capabilityexpansion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
)

func BuildBundle(candidate CandidateRegistry, results []GateResult, remainingRisks []string) (PromotionBundle, error) {
	if err := ValidateCandidate(candidate); err != nil {
		return PromotionBundle{}, err
	}
	normalizedResults := append([]GateResult(nil), results...)
	for index := range normalizedResults {
		normalizedResults[index].CaseID = strings.TrimSpace(normalizedResults[index].CaseID)
		normalizedResults[index].Gate = canonicalID(normalizedResults[index].Gate)
		normalizedResults[index].EvidencePath = strings.TrimSpace(normalizedResults[index].EvidencePath)
		normalizedResults[index].EvidenceSHA256 = strings.ToLower(strings.TrimSpace(normalizedResults[index].EvidenceSHA256))
		normalizedResults[index].Summary = strings.TrimSpace(normalizedResults[index].Summary)
	}
	slices.SortStableFunc(normalizedResults, func(left, right GateResult) int {
		if order := strings.Compare(left.CaseID, right.CaseID); order != 0 {
			return order
		}
		return strings.Compare(left.Gate, right.Gate)
	})
	bundle := PromotionBundle{
		Schema: PromotionBundleSchema, PolicyVersion: PolicyVersion,
		Status: StatusExperimental, Candidate: candidate,
		Results: normalizedResults, RemainingRisks: normalizedStrings(remainingRisks),
	}
	if resultsCompleteAndPassing(candidate.Cases, normalizedResults) {
		bundle.Status = StatusReviewReady
	}
	hash, err := bundleHash(bundle)
	if err != nil {
		return PromotionBundle{}, err
	}
	bundle.Hash = hash
	if err := ValidateBundle(bundle); err != nil {
		return PromotionBundle{}, err
	}
	return bundle, nil
}

func ValidateBundle(bundle PromotionBundle) error {
	if bundle.Schema != PromotionBundleSchema || bundle.PolicyVersion != PolicyVersion ||
		(bundle.Status != StatusExperimental && bundle.Status != StatusReviewReady) {
		return fmt.Errorf("unsupported promotion bundle schema, policy, or status")
	}
	if err := ValidateCandidate(bundle.Candidate); err != nil {
		return err
	}
	caseByID := map[string]GeneratedCase{}
	for _, testCase := range bundle.Candidate.Cases {
		caseByID[testCase.ID] = testCase
	}
	seen := map[string]bool{}
	for _, result := range bundle.Results {
		testCase, ok := caseByID[result.CaseID]
		key := result.CaseID + "\x00" + result.Gate
		if !ok || seen[key] || result.Gate == "" || !slices.Contains(testCase.RequiredGates, result.Gate) ||
			result.EvidencePath == "" || !validSHA256(result.EvidenceSHA256) {
			return fmt.Errorf("invalid or duplicate promotion result %q/%q", result.CaseID, result.Gate)
		}
		seen[key] = true
	}
	complete := resultsCompleteAndPassing(bundle.Candidate.Cases, bundle.Results)
	if bundle.Status == StatusReviewReady && !complete {
		return fmt.Errorf("review-ready bundle lacks complete passing results")
	}
	if bundle.Status == StatusExperimental && complete {
		return fmt.Errorf("complete passing bundle must be review-ready")
	}
	if !validSHA256(bundle.Hash) {
		return fmt.Errorf("promotion bundle hash is invalid")
	}
	expected, err := bundleHash(bundle)
	if err != nil {
		return err
	}
	if bundle.Hash != expected {
		return fmt.Errorf("promotion bundle hash mismatch")
	}
	return nil
}

func resultsCompleteAndPassing(cases []GeneratedCase, results []GateResult) bool {
	byKey := map[string]GateResult{}
	for _, result := range results {
		byKey[result.CaseID+"\x00"+result.Gate] = result
	}
	for _, testCase := range cases {
		for _, gate := range testCase.RequiredGates {
			result, ok := byKey[testCase.ID+"\x00"+gate]
			if !ok || !result.Passed {
				return false
			}
		}
	}
	return true
}

func bundleHash(bundle PromotionBundle) (string, error) {
	hashless := bundle
	hashless.Hash = ""
	return digest(hashless)
}

func EmptySupportedRegistry() SupportedRegistry {
	registry := SupportedRegistry{
		Schema: SupportedRegistrySchema, PolicyVersion: PolicyVersion,
		Status: StatusSupported, Capabilities: []SupportedCapability{},
	}
	registry.Hash, _ = supportedRegistryHash(registry)
	return registry
}

func Promote(existing SupportedRegistry, bundle PromotionBundle, approval PromotionApproval, execute bool) (SupportedRegistry, error) {
	if !execute {
		return SupportedRegistry{}, fmt.Errorf("promotion requires explicit mutation authorization")
	}
	if err := ValidateSupportedRegistry(existing); err != nil {
		return SupportedRegistry{}, err
	}
	if err := ValidateBundle(bundle); err != nil {
		return SupportedRegistry{}, err
	}
	if bundle.Status != StatusReviewReady {
		return SupportedRegistry{}, fmt.Errorf("only review-ready bundles can be promoted")
	}
	if err := validateApproval(approval, bundle.Hash); err != nil {
		return SupportedRegistry{}, err
	}
	next := existing
	next.Capabilities = append([]SupportedCapability(nil), existing.Capabilities...)
	next.Hash = ""
	byCapability := map[string]bool{}
	byProvider := map[string]bool{}
	for _, entry := range next.Capabilities {
		byCapability[entry.Capability] = true
		if entry.Provider != nil {
			byProvider[entry.Provider.ID] = true
		}
	}
	sourceHashesByNeed := map[string][]string{}
	for _, source := range bundle.Candidate.Sources {
		for _, needID := range source.Claims {
			sourceHashesByNeed[needID] = append(sourceHashesByNeed[needID], source.SHA256)
		}
	}
	needByID := map[string]ExpansionNeed{}
	for _, need := range bundle.Candidate.Plan.Needs {
		needByID[need.ID] = need
	}
	providerByNeed := map[string]DeclarativeProviderRecord{}
	for _, provider := range bundle.Candidate.Providers {
		providerByNeed[provider.NeedID] = provider
	}
	for _, artifact := range bundle.Candidate.Artifacts {
		need := needByID[artifact.NeedID]
		if byCapability[need.CapabilityID] {
			return SupportedRegistry{}, fmt.Errorf("promotion conflicts with existing capability %q", need.CapabilityID)
		}
		byCapability[need.CapabilityID] = true
		var promotedProvider *DeclarativeProviderRecord
		if provider, ok := providerByNeed[artifact.NeedID]; ok {
			if byProvider[provider.ID] {
				return SupportedRegistry{}, fmt.Errorf("promotion conflicts with existing provider %q", provider.ID)
			}
			providerCopy := provider
			promotedProvider = &providerCopy
			byProvider[provider.ID] = true
		}
		if need.Kind == NeedArchitecture && promotedProvider == nil {
			return SupportedRegistry{}, fmt.Errorf("architecture capability %q has no provider", need.CapabilityID)
		}
		next.Capabilities = append(next.Capabilities, SupportedCapability{
			Capability: need.CapabilityID, Kind: need.Kind, Artifact: artifact, Provider: promotedProvider,
			SourceHashes: normalizedStrings(sourceHashesByNeed[artifact.NeedID]),
			BundleHash:   bundle.Hash, ReviewRef: approval.ReviewRef,
			ReviewSHA256: approval.ReviewSHA256,
		})
	}
	slices.SortStableFunc(next.Capabilities, func(left, right SupportedCapability) int {
		return strings.Compare(left.Capability, right.Capability)
	})
	hash, err := supportedRegistryHash(next)
	if err != nil {
		return SupportedRegistry{}, err
	}
	next.Hash = hash
	if err := ValidateSupportedRegistry(next); err != nil {
		return SupportedRegistry{}, err
	}
	return next, nil
}

func validateApproval(approval PromotionApproval, bundleHash string) error {
	if approval.Schema != ApprovalSchema || approval.Decision != "approve" ||
		approval.BundleHash != bundleHash || strings.TrimSpace(approval.Reviewer) == "" ||
		strings.TrimSpace(approval.ReviewRef) == "" || !validSHA256(approval.ReviewSHA256) {
		return fmt.Errorf("promotion approval is invalid or not bound to the bundle")
	}
	return nil
}

func ValidateSupportedRegistry(registry SupportedRegistry) error {
	if registry.Schema != SupportedRegistrySchema || registry.PolicyVersion != PolicyVersion ||
		registry.Status != StatusSupported {
		return fmt.Errorf("unsupported supported-registry schema, policy, or status")
	}
	seenCapabilities := map[string]bool{}
	seenProviders := map[string]bool{}
	for _, entry := range registry.Capabilities {
		canonicalPayload, payloadErr := canonicalArtifactPayload(entry.Artifact.Payload)
		if entry.Capability == "" || !validNeedKind(entry.Kind) ||
			seenCapabilities[entry.Capability] || entry.Artifact.Kind != entry.Kind ||
			entry.Artifact.ID == "" || entry.Artifact.NeedID == "" ||
			entry.Artifact.ArtifactType == "" || len(entry.Artifact.EvidenceIDs) == 0 ||
			payloadErr != nil || !bytes.Equal(canonicalPayload, entry.Artifact.Payload) ||
			entry.Artifact.SHA256 != sourceContentSHA256(entry.Artifact.Payload) ||
			!validSHA256(entry.BundleHash) ||
			!validSHA256(entry.ReviewSHA256) || entry.ReviewRef == "" ||
			len(entry.SourceHashes) == 0 {
			return fmt.Errorf("invalid or conflicting supported capability %q", entry.Capability)
		}
		if entry.Kind == NeedArchitecture {
			if entry.Provider == nil || entry.Provider.Capability != entry.Capability ||
				seenProviders[entry.Provider.ID] {
				return fmt.Errorf("invalid or conflicting supported architecture capability %q", entry.Capability)
			}
			seenProviders[entry.Provider.ID] = true
			realization, err := architectureRealization(*entry.Provider)
			if err != nil || realization.Capability != entry.Capability {
				return fmt.Errorf("capability %q has invalid provider", entry.Capability)
			}
			providerPayload, err := json.Marshal(entry.Provider)
			if err == nil {
				providerPayload, err = canonicalArtifactPayload(providerPayload)
			}
			if err != nil || !bytes.Equal(providerPayload, entry.Artifact.Payload) {
				return fmt.Errorf("capability %q provider does not match its artifact", entry.Capability)
			}
		} else if entry.Provider != nil {
			return fmt.Errorf("non-architecture capability %q cannot install a provider", entry.Capability)
		}
		for _, hash := range entry.SourceHashes {
			if !validSHA256(hash) {
				return fmt.Errorf("capability %q has invalid source hash", entry.Capability)
			}
		}
		seenCapabilities[entry.Capability] = true
	}
	if !validSHA256(registry.Hash) {
		return fmt.Errorf("supported registry hash is invalid")
	}
	expected, err := supportedRegistryHash(registry)
	if err != nil {
		return err
	}
	if registry.Hash != expected {
		return fmt.Errorf("supported registry hash mismatch")
	}
	return nil
}

func architectureRealization(provider DeclarativeProviderRecord) (realization architecturesearch.FragmentRealization, err error) {
	return architecturesearch.DecodeFragmentRealization(provider.Expansion.Payload)
}

func supportedRegistryHash(registry SupportedRegistry) (string, error) {
	hashless := registry
	hashless.Hash = ""
	return digest(hashless)
}
