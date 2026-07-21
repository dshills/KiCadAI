package behavioralintent

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
)

// ApplySearchEvidence is the final compiler gate before a behavioral contract
// may be executed. It converts authoritative search ambiguity or missing
// capability into a non-executable result instead of exposing a guessed design.
func ApplySearchEvidence(compilation Result, search architecturesearch.SearchResult) Result {
	result := compilation
	result.Architecture = &ArchitectureEvidence{
		Status: search.Status, RequirementHash: search.RequirementHash,
		RegistryHash: search.RegistryHash, CatalogHash: search.CatalogHash,
	}
	if result.Status != StatusReady {
		return result
	}
	if search.Status == architecturesearch.SearchSelected && search.Selected != nil {
		return result
	}
	result.Requirement = nil
	switch search.Status {
	case architecturesearch.SearchAmbiguous:
		result.Status = StatusNeedsClarification
		result.Uncertainties = append(result.Uncertainties, Uncertainty{
			ID: "architecture_tradeoff", Path: "requirements.objectives", Kind: "selection_priority",
			Description: "materially distinct candidates require a user-owned behavioral tradeoff", Resolution: ResolutionClarification, ResolvedBy: "architecture_priority",
		})
		result.Clarifications = append(result.Clarifications, Clarification{
			ID: "architecture_priority", Path: "requirements.objectives",
			Question:       "Which behavioral tradeoff should decide between the valid candidates?",
			WhyNeeded:      "the installed capability registry found materially distinct candidates that cannot be selected safely from the stated requirements",
			UncertaintyIDs: []string{"architecture_tradeoff"},
		})
		result.Coverage = rewriteCompiledCoverage(result.Coverage, DispositionClarification, []Reference{{Kind: "clarification", ID: "architecture_priority"}, {Kind: "uncertainty", ID: "architecture_tradeoff"}}, "architecture search requires a user-owned behavioral priority")
	case architecturesearch.SearchUnsupported, architecturesearch.SearchExhausted:
		result.Status = StatusUnsupported
		result.CapabilityGaps = searchCapabilityGaps(search)
		references := make([]Reference, 0, len(result.CapabilityGaps))
		for _, gap := range result.CapabilityGaps {
			references = append(references, Reference{Kind: "capability_gap", ID: gap.ID})
		}
		result.Coverage = rewriteCompiledCoverage(result.Coverage, DispositionCapabilityGap, references, "installed architecture search could not satisfy the compiled behavior")
	default:
		result.Status = StatusInvalid
		result.Issues = append(result.Issues, compilerIssue(CodeRequirementInvalid, "architecture.status", "architecture search did not produce a safe terminal result"))
	}
	result.Uncertainties = normalizeUncertainties(result.Uncertainties)
	result.Clarifications = normalizeClarifications(result.Clarifications)
	result.CapabilityGaps = normalizeCapabilityGaps(result.CapabilityGaps)
	result.Coverage = normalizeCoverage(result.Coverage)
	return result
}

func searchCapabilityGaps(search architecturesearch.SearchResult) []CapabilityGap {
	var gaps []CapabilityGap
	seen := map[string]bool{}
	if search.Coverage != nil {
		for _, record := range search.Coverage.Records {
			if record.Status == architecturesearch.CoverageSelected {
				continue
			}
			capability := searchRejectionCapability(search, record.Path)
			if capability == "" {
				capability = strings.TrimSpace(record.Capability)
			}
			if !validSemanticID(capability) {
				capability = "architecture_search"
			}
			id := capability
			if !strings.HasPrefix(capability, "mcu_") {
				id = stableSearchGapID(capability, record.Path)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			gaps = append(gaps, CapabilityGap{
				ID: id, Capability: capability, Path: strings.TrimSpace(record.Path),
				Reason:           "the installed architecture registry did not produce a verified candidate for this behavioral obligation",
				RequiredEvidence: []string{"registered architecture provider coverage", "trusted model and analysis coverage for every required operating case"},
			})
		}
	}
	if len(gaps) == 0 {
		gaps = append(gaps, CapabilityGap{
			ID: stableSearchGapID("architecture_search", "requirements.objectives"), Capability: "architecture_search", Path: "requirements.objectives",
			Reason:           "the installed architecture registry did not produce a verified candidate for the compiled behavioral contract",
			RequiredEvidence: []string{"registered architecture provider coverage", "trusted model and analysis coverage for every required operating case"},
		})
	}
	slices.SortFunc(gaps, func(left, right CapabilityGap) int { return strings.Compare(left.ID, right.ID) })
	return gaps
}

func searchRejectionCapability(search architecturesearch.SearchResult, path string) string {
	for _, summary := range search.Rejections {
		code := strings.ToLower(string(summary.Code))
		if !strings.HasPrefix(code, "mcu_") {
			continue
		}
		for _, sample := range summary.Samples {
			if sample.Path == path {
				return code
			}
		}
	}
	return ""
}

func stableSearchGapID(capability, path string) string {
	hash := sha256.Sum256([]byte(capability + "\x00" + strings.TrimSpace(path)))
	return "gap_" + hex.EncodeToString(hash[:6])
}

func rewriteCompiledCoverage(values []CoverageRecord, disposition Disposition, references []Reference, rationale string) []CoverageRecord {
	result := slices.Clone(values)
	for index := range result {
		if result[index].Disposition != DispositionCompiled {
			continue
		}
		result[index].Disposition = disposition
		result[index].References = slices.Clone(references)
		result[index].Rationale = rationale
	}
	return result
}
