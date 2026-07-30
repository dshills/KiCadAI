package capabilitygate

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
)

// AssessArchitecture derives a capability decision from normalized behavioral
// requirements and persisted architecture-search evidence. It never inspects
// project names or corpus membership.
func AssessArchitecture(requirement architecturesearch.Requirement, search architecturesearch.SearchResult, experimentalOptIn bool) (Assessment, error) {
	requirement = architecturesearch.Normalize(requirement)
	requirementDigest, err := Digest(requirement)
	if err != nil {
		return Assessment{}, fmt.Errorf("digest normalized requirement: %w", err)
	}
	searchDigest, err := Digest(search)
	if err != nil {
		return Assessment{}, fmt.Errorf("digest architecture search: %w", err)
	}
	input := Input{Stage: "architecture_selection", ExperimentalOptIn: experimentalOptIn}
	requirementEvidenceID := "requirement:" + requirementDigest[:16]
	input.Evidence = append(input.Evidence, Evidence{
		ID: requirementEvidenceID, Kind: "normalized_requirement", Status: EvidenceVerified,
		Source: "requirement://normalized", Digest: requirementDigest, Stage: "architecture_selection",
		Description: "normalized electrical requirement and acceptance policy",
	})
	for _, domain := range requirement.Requirements.Domains {
		input.Requirements = append(input.Requirements, Requirement{
			Kind: RequirementDomain, ID: domain.Kind + ":" + domain.ID,
			Description: "declared electrical domain", EvidenceIDs: []string{requirementEvidenceID},
		})
	}
	if len(input.Requirements) == 0 {
		input.Requirements = append(input.Requirements, Requirement{
			Kind: RequirementDomain, ID: "electrical_domains",
			Description: "at least one normalized electrical domain", EvidenceIDs: []string{requirementEvidenceID},
		})
	}
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		statusEvidenceID := "architecture-search:" + searchDigest[:16]
		input.Evidence = append(input.Evidence, Evidence{
			ID: statusEvidenceID, Kind: "architecture_search", Status: EvidenceFailed,
			Source: "architecture-search://result", Digest: searchDigest, Stage: "architecture_selection",
			Description: "architecture search did not select a complete candidate",
		})
		input.Requirements = append(input.Requirements, Requirement{
			Kind: RequirementArchitecture, ID: "complete_candidate", EvidenceIDs: []string{statusEvidenceID},
		})
		input.Gaps = append(input.Gaps, Gap{
			Code: "CAPABILITY_ARCHITECTURE_" + strings.ToUpper(string(search.Status)),
			Kind: RequirementArchitecture, ID: "complete_candidate", Stage: "architecture_selection",
			Reason: architectureSearchFailureReason(search.Status),
			Action: "add a provider with the missing typed capability evidence or refine the requirement constraints",
		})
		return Assess(input)
	}

	selected := *search.Selected
	selections := append([]architecturesearch.FragmentSelection(nil), selected.Selections...)
	slices.SortStableFunc(selections, func(left, right architecturesearch.FragmentSelection) int {
		if order := strings.Compare(left.ObligationPath, right.ObligationPath); order != 0 {
			return order
		}
		if order := strings.Compare(left.Capability, right.Capability); order != 0 {
			return order
		}
		return strings.Compare(left.ProviderID, right.ProviderID)
	})
	for _, selection := range selections {
		digest, digestErr := Digest(struct {
			SearchDigest string
			Selection    architecturesearch.FragmentSelection
		}{SearchDigest: searchDigest, Selection: selection})
		if digestErr != nil {
			return Assessment{}, fmt.Errorf("digest selected architecture %q: %w", selection.Capability, digestErr)
		}
		evidenceID := "architecture:" + stableCapabilityID(selection.ObligationPath, selection.Capability, selection.ProviderID)
		status := EvidenceVerified
		if selection.RequiresUserChoice {
			status = EvidenceInferred
			input.Risks = append(input.Risks, Risk{
				Code: "CAPABILITY_ARCHITECTURE_PROVISIONAL", Stage: "architecture_selection",
				Summary:    "selected architecture " + selection.Capability + " requires an unresolved user choice",
				Mitigation: "provide the missing constraint before fabrication use",
			})
		}
		input.Evidence = append(input.Evidence, Evidence{
			ID: evidenceID, Kind: "architecture_selection", Status: status,
			Source: "architecture-search://" + selection.ProviderID + "/" + selection.Capability,
			Digest: digest, Stage: "architecture_selection",
			Description: "deterministically selected typed provider expansion",
		})
		input.Requirements = append(input.Requirements, Requirement{
			Kind: RequirementArchitecture, ID: selection.Capability,
			Description: "selected functional architecture capability", EvidenceIDs: []string{evidenceID},
		})
		if selection.Metrics.UnprovenNonSafety > 0 {
			assumptionEvidenceID := evidenceID + ":non_safety_assumptions"
			input.Evidence = append(input.Evidence, Evidence{
				ID: assumptionEvidenceID, Kind: "architecture_confidence", Status: EvidenceInferred,
				Advisory: true,
				Source:   "architecture-search://" + selection.ProviderID + "/" + selection.Capability,
				Digest:   digest, Stage: "architecture_selection",
				Description: "non-safety architecture assumptions remain visible for downstream verification",
			})
			input.Risks = append(input.Risks, Risk{
				Code: "CAPABILITY_ARCHITECTURE_ASSUMPTION", Stage: "architecture_selection",
				Summary:    "selected architecture " + selection.Capability + " retains a non-safety assumption",
				Mitigation: "verify the assumption in downstream simulation and KiCad checks",
			})
		}
		for _, component := range selection.Components {
			componentDigest, componentErr := Digest(struct {
				CatalogHash string
				Component   architecturesearch.SelectedComponent
			}{CatalogHash: search.CatalogHash, Component: component})
			if componentErr != nil {
				return Assessment{}, fmt.Errorf("digest selected component %q: %w", component.CatalogID, componentErr)
			}
			componentEvidenceID := "component:" + stableCapabilityID(component.CatalogID, component.VariantID, component.InstanceID)
			componentStatus := architectureEvidenceStatus(component.Evidence)
			input.Evidence = append(input.Evidence, Evidence{
				ID: componentEvidenceID, Kind: "catalog_component", Status: componentStatus,
				Source: "catalog://" + component.CatalogID + optionalPathSegment(component.VariantID),
				Digest: componentDigest, Stage: "component_selection",
				Description: "catalog-backed component and package selection",
			})
			input.Requirements = append(input.Requirements, Requirement{
				Kind: RequirementComponent, ID: component.CatalogID + optionalIdentitySuffix(component.VariantID),
				Description: "selected catalog component", EvidenceIDs: []string{componentEvidenceID},
			})
			if componentStatus == EvidenceInferred {
				confidenceEvidenceID := componentEvidenceID + ":confidence"
				input.Evidence = append(input.Evidence, Evidence{
					ID: confidenceEvidenceID, Kind: "catalog_component_confidence", Status: EvidenceInferred,
					Advisory: true,
					Source:   "catalog://" + component.CatalogID + optionalPathSegment(component.VariantID),
					Digest:   componentDigest, Stage: "component_selection",
					Description: "component value or suitability is rule-inferred and requires downstream verification",
				})
				input.Risks = append(input.Risks, Risk{
					Code: "CAPABILITY_COMPONENT_INFERRED", Stage: "component_selection",
					Summary:    "selected component " + component.CatalogID + " includes rule-inferred confidence",
					Mitigation: "retain verified downstream electrical and physical checks before fabrication use",
				})
			}
			if componentStatus == EvidenceMissing || componentStatus == EvidenceFailed {
				input.Gaps = append(input.Gaps, Gap{
					Code: "CAPABILITY_COMPONENT_EVIDENCE_MISSING", Kind: RequirementComponent,
					ID: component.CatalogID, Stage: "component_selection",
					Reason: "selected component lacks sufficient catalog verification evidence",
					Action: "add verified component, package, pinmap, and rating evidence",
				})
			}
		}
	}
	if len(selections) == 0 {
		input.Gaps = append(input.Gaps, Gap{
			Code: "CAPABILITY_ARCHITECTURE_EMPTY", Kind: RequirementArchitecture,
			ID: "complete_candidate", Stage: "architecture_selection",
			Reason: "selected candidate contains no architecture fragments",
		})
		input.Requirements = append(input.Requirements, Requirement{
			Kind: RequirementArchitecture, ID: "complete_candidate", EvidenceIDs: []string{requirementEvidenceID},
		})
	}

	physicalEvidenceID := "physical-search:" + searchDigest[:16]
	input.Evidence = append(input.Evidence, Evidence{
		ID: physicalEvidenceID, Kind: "physical_bounds", Status: EvidenceVerified,
		Source: "architecture-search://physical-bounds", Digest: searchDigest, Stage: "architecture_selection",
		Description: "selected candidate satisfies normalized component-count and board bounds",
	})
	input.Requirements = append(input.Requirements, Requirement{
		Kind: RequirementPhysical, ID: "bounded_pcb_realization",
		Description: "bounded physical realization is required", EvidenceIDs: []string{physicalEvidenceID},
	})
	for _, verification := range requiredArchitectureVerification(requirement.Acceptance) {
		input.Requirements = append(input.Requirements, Requirement{
			Kind: RequirementVerification, ID: verification,
			Description: "requested verification capability", EvidenceIDs: []string{physicalEvidenceID},
		})
	}
	return Assess(input)
}

func architectureEvidenceStatus(confidence architecturesearch.EvidenceConfidence) EvidenceStatus {
	switch confidence {
	case architecturesearch.EvidenceVerified, architecturesearch.EvidenceLibraryDerived:
		return EvidenceVerified
	case architecturesearch.EvidenceRuleInferred:
		return EvidenceInferred
	case architecturesearch.EvidenceBlocked:
		return EvidenceFailed
	default:
		return EvidenceMissing
	}
}

func architectureSearchFailureReason(status architecturesearch.SearchStatus) string {
	switch status {
	case architecturesearch.SearchUnsupported:
		return "no registered provider covers every required architecture capability"
	case architecturesearch.SearchExhausted:
		return "bounded architecture search exhausted its deterministic budget"
	case architecturesearch.SearchAmbiguous:
		return "architecture alternatives remain electrically indistinguishable without a user constraint"
	case architecturesearch.SearchFailed:
		return "architecture search failed before producing a complete candidate"
	default:
		return "architecture search did not produce a complete candidate"
	}
}

func stableCapabilityID(parts ...string) string {
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			encoded = append(encoded, fmt.Sprintf("%d:%s", len(part), part))
		}
	}
	joined := strings.Join(encoded, "|")
	if joined == "" {
		return "unnamed"
	}
	return joined
}

func optionalPathSegment(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return "/" + value
	}
	return ""
}

func optionalIdentitySuffix(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return ":" + value
	}
	return ""
}

func requiredArchitectureVerification(acceptance architecturesearch.Acceptance) []string {
	checks := []struct {
		id       string
		required bool
	}{
		{"erc", acceptance.RequireERC},
		{"strict_drc", acceptance.RequireStrictDRC},
		{"complete_routing", acceptance.RequireCompleteRouting},
		{"connectivity", acceptance.RequireConnectivity},
		{"writer_correctness", acceptance.RequireWriterCorrectness},
		{"round_trip_zero_diff", acceptance.RequireRoundTripZeroDiff},
		{"deterministic_replay", acceptance.RequireDeterministicReplay},
		{"simulation", acceptance.RequireSimulation},
		{"model_provenance", acceptance.RequireModelProvenance},
		{"closed_loop_evidence", acceptance.RequireClosedLoopEvidence},
		{"dynamic_model_provenance", acceptance.RequireDynamicModelProvenance},
		{"return_ratio_evidence", acceptance.RequireReturnRatioEvidence},
		{"dynamic_electrothermal_evidence", acceptance.RequireDynamicElectrothermalEvidence},
		{"event_coverage", acceptance.RequireEventCoverage},
	}
	result := make([]string, 0, len(checks))
	for _, check := range checks {
		if check.required {
			result = append(result, check.id)
		}
	}
	return result
}
