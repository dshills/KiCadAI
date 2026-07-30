package capabilityexpansion

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
)

const (
	maxSourceBytes          = 8 << 20
	maxArtifactPayloadBytes = 2 << 20
)

// BuildCandidate ingests source bytes into a quarantined experimental
// registry. It never changes a supported registry.
func BuildCandidate(
	plan ExpansionPlan,
	sources []SourceInput,
	artifacts []CapabilityArtifact,
	providers []DeclarativeProviderRecord,
	assumptions []string,
	risks []string,
) (CandidateRegistry, error) {
	if err := ValidatePlan(plan); err != nil {
		return CandidateRegistry{}, err
	}
	sourceRecords, err := ingestSources(plan, sources)
	if err != nil {
		return CandidateRegistry{}, err
	}
	candidate := CandidateRegistry{
		Schema: CandidateRegistrySchema, PolicyVersion: PolicyVersion,
		Status: StatusExperimental, Plan: plan, Sources: sourceRecords,
		Artifacts:   append([]CapabilityArtifact(nil), artifacts...),
		Providers:   append([]DeclarativeProviderRecord(nil), providers...),
		Assumptions: normalizedStrings(assumptions),
		Risks: normalizedStrings(append([]string{
			"candidate registry is quarantined and cannot authorize fabrication-ready generation",
		}, risks...)),
	}
	if err := normalizeCandidate(&candidate); err != nil {
		return CandidateRegistry{}, err
	}
	candidate.Cases = GenerateCases(plan)
	hash, err := candidateHash(candidate)
	if err != nil {
		return CandidateRegistry{}, err
	}
	candidate.Hash = hash
	if err := ValidateCandidate(candidate); err != nil {
		return CandidateRegistry{}, err
	}
	return candidate, nil
}

func ingestSources(plan ExpansionPlan, inputs []SourceInput) ([]SourceRecord, error) {
	if len(inputs) > MaxCandidateSources {
		return nil, fmt.Errorf("candidate exceeds %d-source limit", MaxCandidateSources)
	}
	needByID := map[string]ExpansionNeed{}
	for _, need := range plan.Needs {
		needByID[need.ID] = need
	}
	records := make([]SourceRecord, 0, len(inputs))
	seen := map[string]SourceRecord{}
	totalBytes := 0
	for _, input := range inputs {
		input.ID = canonicalID(input.ID)
		input.Publisher = strings.TrimSpace(input.Publisher)
		input.Locator = strings.TrimSpace(input.Locator)
		input.License = strings.TrimSpace(input.License)
		input.Claims = normalizedStrings(input.Claims)
		input.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(input.ExpectedSHA256))
		if input.ID == "" || !validSourceKind(input.Kind) || input.Publisher == "" || input.Locator == "" ||
			len(input.Claims) == 0 || len(input.Content) == 0 || len(input.Content) > maxSourceBytes {
			return nil, fmt.Errorf("source %q is incomplete or outside bounded ingestion limits", input.ID)
		}
		if len(input.Content) > MaxCandidateSourceBytes-totalBytes {
			return nil, fmt.Errorf("candidate sources exceed %d-byte aggregate limit", MaxCandidateSourceBytes)
		}
		totalBytes += len(input.Content)
		if err := validateSourceLocator(input.Locator); err != nil {
			return nil, fmt.Errorf("source %q: %w", input.ID, err)
		}
		if input.Kind == SourceModel && input.License == "" {
			return nil, fmt.Errorf("model source %q requires license provenance", input.ID)
		}
		sum := sha256.Sum256(input.Content)
		actual := hex.EncodeToString(sum[:])
		if !validSHA256(input.ExpectedSHA256) || actual != input.ExpectedSHA256 {
			return nil, fmt.Errorf("source %q content digest mismatch", input.ID)
		}
		for _, claim := range input.Claims {
			if _, ok := needByID[claim]; !ok {
				return nil, fmt.Errorf("source %q claims unrelated need %q", input.ID, claim)
			}
		}
		record := SourceRecord{
			ID: input.ID, Kind: input.Kind, Publisher: input.Publisher,
			Locator: input.Locator, License: input.License,
			Claims: input.Claims, SHA256: actual, Bytes: len(input.Content),
		}
		if previous, ok := seen[record.ID]; ok {
			if previous.ID != record.ID || previous.Kind != record.Kind ||
				previous.Publisher != record.Publisher || previous.Locator != record.Locator ||
				previous.License != record.License || previous.SHA256 != record.SHA256 ||
				previous.Bytes != record.Bytes || !slices.Equal(previous.Claims, record.Claims) {
				return nil, fmt.Errorf("source %q has conflicting records", record.ID)
			}
			continue
		}
		seen[record.ID] = record
		records = append(records, record)
	}
	slices.SortStableFunc(records, func(left, right SourceRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	return records, nil
}

func validateSourceLocator(locator string) error {
	if strings.HasPrefix(locator, "doi:") || strings.HasPrefix(locator, "urn:") || strings.HasPrefix(locator, "kicadai:") {
		return nil
	}
	parsed, err := url.Parse(locator)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "file") || parsed.Path == "" {
		return fmt.Errorf("source locator must use https, file, doi, urn, or kicadai")
	}
	return nil
}

func normalizeCandidate(candidate *CandidateRegistry) error {
	for index := range candidate.Artifacts {
		candidate.Artifacts[index].ID = canonicalID(candidate.Artifacts[index].ID)
		candidate.Artifacts[index].NeedID = strings.TrimSpace(candidate.Artifacts[index].NeedID)
		candidate.Artifacts[index].ArtifactType = canonicalID(candidate.Artifacts[index].ArtifactType)
		candidate.Artifacts[index].SHA256 = strings.ToLower(strings.TrimSpace(candidate.Artifacts[index].SHA256))
		candidate.Artifacts[index].EvidenceIDs = normalizedStrings(candidate.Artifacts[index].EvidenceIDs)
		payload, err := canonicalArtifactPayload(candidate.Artifacts[index].Payload)
		if err != nil {
			return fmt.Errorf("artifact %q payload: %w", candidate.Artifacts[index].ID, err)
		}
		candidate.Artifacts[index].Payload = payload
	}
	slices.SortStableFunc(candidate.Artifacts, func(left, right CapabilityArtifact) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range candidate.Providers {
		provider := &candidate.Providers[index]
		provider.ID = canonicalID(provider.ID)
		provider.Revision = strings.TrimSpace(provider.Revision)
		provider.Capability = canonicalID(provider.Capability)
		provider.NeedID = strings.TrimSpace(provider.NeedID)
		provider.EvidenceIDs = normalizedStrings(provider.EvidenceIDs)
		realization, err := architecturesearch.DecodeFragmentRealization(provider.Expansion.Payload)
		if err != nil {
			return fmt.Errorf("provider %q realization: %w", provider.ID, err)
		}
		payload, err := architecturesearch.MarshalFragmentRealization(realization)
		if err != nil {
			return fmt.Errorf("provider %q realization: %w", provider.ID, err)
		}
		provider.Expansion.Payload = payload
	}
	slices.SortStableFunc(candidate.Providers, func(left, right DeclarativeProviderRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	return nil
}

func ValidateCandidate(candidate CandidateRegistry) error {
	if candidate.Schema != CandidateRegistrySchema || candidate.PolicyVersion != PolicyVersion ||
		candidate.Status != StatusExperimental {
		return fmt.Errorf("candidate registry must use the experimental schema and policy")
	}
	if err := ValidatePlan(candidate.Plan); err != nil {
		return err
	}
	needByID := map[string]ExpansionNeed{}
	for _, need := range candidate.Plan.Needs {
		needByID[need.ID] = need
	}
	sourceByID := map[string]SourceRecord{}
	kindsByNeed := map[string]map[SourceKind]bool{}
	if len(candidate.Sources) > MaxCandidateSources {
		return fmt.Errorf("candidate exceeds %d-source limit", MaxCandidateSources)
	}
	totalSourceBytes := 0
	for _, source := range candidate.Sources {
		if sourceByID[source.ID].ID != "" || source.ID == "" || !validSourceKind(source.Kind) ||
			source.Publisher == "" || source.Locator == "" || !validSHA256(source.SHA256) ||
			source.Bytes <= 0 || source.Bytes > maxSourceBytes || len(source.Claims) == 0 {
			return fmt.Errorf("invalid candidate source %q", source.ID)
		}
		if source.Bytes > MaxCandidateSourceBytes-totalSourceBytes {
			return fmt.Errorf("candidate sources exceed %d-byte aggregate limit", MaxCandidateSourceBytes)
		}
		totalSourceBytes += source.Bytes
		if source.Kind == SourceModel && source.License == "" {
			return fmt.Errorf("model source %q lacks license provenance", source.ID)
		}
		sourceByID[source.ID] = source
		for _, claim := range source.Claims {
			if _, ok := needByID[claim]; !ok {
				return fmt.Errorf("source %q claims unrelated need %q", source.ID, claim)
			}
			if kindsByNeed[claim] == nil {
				kindsByNeed[claim] = map[SourceKind]bool{}
			}
			kindsByNeed[claim][source.Kind] = true
		}
	}
	artifactByNeed := map[string]CapabilityArtifact{}
	for _, artifact := range candidate.Artifacts {
		need, ok := needByID[artifact.NeedID]
		actualArtifactHash := sourceContentSHA256(artifact.Payload)
		if !ok || artifact.ID == "" || artifact.Kind != need.Kind ||
			artifact.ArtifactType != need.RequiredArtifact || !validSHA256(artifact.SHA256) ||
			artifact.SHA256 != actualArtifactHash || len(artifact.EvidenceIDs) == 0 ||
			artifactByNeed[artifact.NeedID].ID != "" {
			return fmt.Errorf("invalid or duplicate candidate artifact %q", artifact.ID)
		}
		for _, evidenceID := range artifact.EvidenceIDs {
			source, ok := sourceByID[evidenceID]
			if !ok || !slices.Contains(source.Claims, artifact.NeedID) {
				return fmt.Errorf("artifact %q references unrelated evidence %q", artifact.ID, evidenceID)
			}
		}
		artifactByNeed[artifact.NeedID] = artifact
	}
	providerByNeed := map[string]DeclarativeProviderRecord{}
	for _, provider := range candidate.Providers {
		need, ok := needByID[provider.NeedID]
		if !ok || need.Kind != NeedArchitecture || providerByNeed[provider.NeedID].ID != "" ||
			provider.ID == "" || provider.Revision == "" || provider.Capability != need.CapabilityID ||
			len(provider.EvidenceIDs) == 0 || provider.Expansion.ID == "" {
			return fmt.Errorf("invalid or duplicate declarative provider %q", provider.ID)
		}
		realization, err := architecturesearch.DecodeFragmentRealization(provider.Expansion.Payload)
		if err != nil || realization.Capability != provider.Capability {
			return fmt.Errorf("provider %q has invalid capability realization", provider.ID)
		}
		if provider.Expansion.Evidence.Confidence != architecturesearch.EvidenceVerified ||
			len(provider.Expansion.Evidence.Sources) == 0 || len(provider.Expansion.OfferedPorts) == 0 {
			return fmt.Errorf("provider %q lacks verified expansion evidence", provider.ID)
		}
		for _, evidenceID := range provider.Expansion.Evidence.Sources {
			if !slices.Contains(provider.EvidenceIDs, evidenceID) {
				return fmt.Errorf("provider %q expansion references unclaimed evidence %q", provider.ID, evidenceID)
			}
		}
		seenRoles := map[string]bool{}
		for _, port := range provider.Expansion.OfferedPorts {
			if port.Role == "" || seenRoles[port.Role] ||
				port.Contract.Evidence.Confidence != architecturesearch.EvidenceVerified ||
				len(port.Contract.Evidence.Sources) == 0 {
				return fmt.Errorf("provider %q has an invalid reviewed port contract", provider.ID)
			}
			for _, evidenceID := range port.Contract.Evidence.Sources {
				if !slices.Contains(provider.EvidenceIDs, evidenceID) {
					return fmt.Errorf("provider %q port %q references unclaimed evidence %q", provider.ID, port.Role, evidenceID)
				}
			}
			seenRoles[port.Role] = true
		}
		for _, binding := range realization.PortBindings {
			if !seenRoles[binding.Role] {
				return fmt.Errorf("provider %q lacks a reviewed contract for role %q", provider.ID, binding.Role)
			}
		}
		for _, evidenceID := range provider.EvidenceIDs {
			source, ok := sourceByID[evidenceID]
			if !ok || !slices.Contains(source.Claims, provider.NeedID) {
				return fmt.Errorf("provider %q references unrelated evidence %q", provider.ID, evidenceID)
			}
		}
		providerPayload, err := json.Marshal(provider)
		if err == nil {
			providerPayload, err = canonicalArtifactPayload(providerPayload)
		}
		if err != nil || !bytes.Equal(artifactByNeed[provider.NeedID].Payload, providerPayload) {
			return fmt.Errorf("provider %q does not match its candidate artifact payload", provider.ID)
		}
		providerByNeed[provider.NeedID] = provider
	}
	for _, need := range candidate.Plan.Needs {
		for _, sourceKind := range need.RequiredSourceKinds {
			if !kindsByNeed[need.ID][sourceKind] {
				return fmt.Errorf("need %q lacks required %s source", need.ID, sourceKind)
			}
		}
		if artifactByNeed[need.ID].ID == "" {
			return fmt.Errorf("need %q lacks required artifact", need.ID)
		}
		if need.Kind == NeedArchitecture && providerByNeed[need.ID].ID == "" {
			return fmt.Errorf("architecture need %q lacks declarative provider", need.ID)
		}
	}
	if len(candidate.Cases) != len(candidate.Plan.Needs)*5 {
		return fmt.Errorf("candidate case inventory is incomplete")
	}
	if !validSHA256(candidate.Hash) {
		return fmt.Errorf("candidate registry hash is invalid")
	}
	expected, err := candidateHash(candidate)
	if err != nil {
		return err
	}
	if candidate.Hash != expected {
		return fmt.Errorf("candidate registry hash mismatch")
	}
	return nil
}

func candidateHash(candidate CandidateRegistry) (string, error) {
	hashless := candidate
	hashless.Hash = ""
	return digest(hashless)
}

func GenerateCases(plan ExpansionPlan) []GeneratedCase {
	var cases []GeneratedCase
	for _, need := range plan.Needs {
		for _, kind := range []GeneratedCaseKind{
			CaseRepresentative,
			CaseMissingEvidence,
			CaseConflictingEvidence,
			CaseIrrelevantEvidence,
			CaseFabricatedEvidence,
		} {
			gates := []string{"source_integrity", "candidate_registry", "deterministic_replay"}
			if kind == CaseRepresentative {
				gates = append(gates, need.RequiredPromotionGates...)
			} else {
				gates = append(gates, "fail_closed_rejection")
			}
			cases = append(cases, GeneratedCase{
				ID: need.ID + ":" + string(kind), NeedID: need.ID,
				Kind:          kind,
				RequiredGates: normalizedStrings(gates),
			})
		}
	}
	slices.SortStableFunc(cases, func(left, right GeneratedCase) int {
		return strings.Compare(left.ID, right.ID)
	})
	return cases
}

func sourceContentSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func canonicalArtifactPayload(payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 || len(payload) > maxArtifactPayloadBytes {
		return nil, fmt.Errorf("payload is empty or exceeds %d bytes", maxArtifactPayloadBytes)
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("payload contains trailing JSON")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}
