package architecturesearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

type registeredProvider struct {
	descriptor ProviderDescriptor
	provider   FragmentProvider
}

type Registry struct {
	providers []registeredProvider
	hash      string
}

func NewRegistry(providers ...FragmentProvider) (*Registry, []reports.Issue) {
	registry := &Registry{}
	seen := map[string]bool{}
	var issues []reports.Issue
	for index, provider := range providers {
		path := fmt.Sprintf("providers[%d]", index)
		if provider == nil {
			issues = append(issues, architectureIssue(CodeProviderInvalid, path, "provider is nil"))
			continue
		}
		descriptor := normalizeProviderDescriptor(provider.Descriptor())
		if !validSemanticID(descriptor.ID) || !validRevision(descriptor.Revision) || len(descriptor.Capabilities) == 0 {
			issues = append(issues, architectureIssue(CodeProviderInvalid, path, "provider requires a normalized id, stable revision, and at least one capability"))
			continue
		}
		if seen[descriptor.ID] {
			issues = append(issues, architectureIssue(CodeProviderDuplicate, path+".id", "provider id is duplicated"))
			continue
		}
		seen[descriptor.ID] = true
		validCapabilities := true
		for _, capability := range descriptor.Capabilities {
			if !validSemanticID(capability) {
				validCapabilities = false
				break
			}
		}
		if !validCapabilities || !validEvidenceConfidence(descriptor.Evidence.Confidence) || confidenceRank(descriptor.Evidence.Confidence) < confidenceRank(EvidenceRuleInferred) {
			issues = append(issues, architectureIssue(CodeProviderInvalid, path, "provider capabilities or evidence are invalid or insufficient"))
			continue
		}
		registry.providers = append(registry.providers, registeredProvider{descriptor: descriptor, provider: provider})
	}
	slices.SortStableFunc(registry.providers, func(left, right registeredProvider) int {
		if order := strings.Compare(left.descriptor.ID, right.descriptor.ID); order != 0 {
			return order
		}
		return strings.Compare(left.descriptor.Revision, right.descriptor.Revision)
	})
	slices.SortStableFunc(issues, func(left, right reports.Issue) int {
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		return strings.Compare(string(left.Code), string(right.Code))
	})
	if len(issues) != 0 {
		return registry, issues
	}
	descriptors := make([]ProviderDescriptor, len(registry.providers))
	for index, provider := range registry.providers {
		descriptors[index] = provider.descriptor
	}
	encoded, err := json.Marshal(descriptors)
	if err != nil {
		return registry, []reports.Issue{architectureIssue(CodeProviderInvalid, "providers", "hash provider registry: "+err.Error())}
	}
	sum := sha256.Sum256(encoded)
	registry.hash = hex.EncodeToString(sum[:])
	return registry, nil
}

func (registry *Registry) Hash() string {
	if registry == nil {
		return ""
	}
	return registry.hash
}

func (registry *Registry) providersFor(capability string) []registeredProvider {
	if registry == nil {
		return nil
	}
	capability = canonicalIdentifier(capability)
	var matches []registeredProvider
	for _, provider := range registry.providers {
		if slices.Contains(provider.descriptor.Capabilities, capability) {
			matches = append(matches, provider)
		}
	}
	return matches
}

func normalizeProviderDescriptor(descriptor ProviderDescriptor) ProviderDescriptor {
	descriptor.ID = canonicalIdentifier(descriptor.ID)
	descriptor.Revision = strings.TrimSpace(descriptor.Revision)
	descriptor.Capabilities = normalizeStringSet(descriptor.Capabilities)
	descriptor.Evidence = normalizeContractEvidence(descriptor.Evidence)
	return descriptor
}

func validRevision(revision string) bool {
	if revision == "" || len(revision) > 64 {
		return false
	}
	for _, character := range revision {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '.' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
