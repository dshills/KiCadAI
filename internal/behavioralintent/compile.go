package behavioralintent

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
)

const (
	CodeProposalInvalid          reports.Code = "BEHAVIORAL_INTENT_PROPOSAL_INVALID"
	CodeProposalLimit            reports.Code = "BEHAVIORAL_INTENT_PROPOSAL_LIMIT"
	CodeSourceCoverageInvalid    reports.Code = "BEHAVIORAL_INTENT_SOURCE_COVERAGE_INVALID"
	CodeUncertaintyInvalid       reports.Code = "BEHAVIORAL_INTENT_UNCERTAINTY_INVALID"
	CodeClarificationInvalid     reports.Code = "BEHAVIORAL_INTENT_CLARIFICATION_INVALID"
	CodeCapabilityGapInvalid     reports.Code = "BEHAVIORAL_INTENT_CAPABILITY_GAP_INVALID"
	CodeCapabilityContextInvalid reports.Code = "BEHAVIORAL_INTENT_CAPABILITY_CONTEXT_INVALID"
	CodeRequirementInvalid       reports.Code = "BEHAVIORAL_INTENT_REQUIREMENT_INVALID"
	CodeFollowUpInvalid          reports.Code = "BEHAVIORAL_INTENT_FOLLOW_UP_INVALID"
)

var semanticIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func Compile(prompt string, proposal Proposal, capabilitySHA256 string) Result {
	source := PrepareSource(prompt)
	result := Result{
		Schema:           ResultSchema,
		Version:          ResultVersion,
		Status:           StatusInvalid,
		Source:           source,
		CapabilitySHA256: strings.TrimSpace(capabilitySHA256),
		Coverage:         normalizeCoverage(proposal.Coverage),
		Uncertainties:    normalizeUncertainties(proposal.Uncertainties),
		Clarifications:   normalizeClarifications(proposal.Clarifications),
		CapabilityGaps:   normalizeCapabilityGaps(proposal.CapabilityGaps),
	}
	if strings.TrimSpace(prompt) == "" {
		result.Issues = append(result.Issues, compilerIssue(CodeProposalInvalid, "source", "circuit-design request is required"))
	}
	if !sha256Pattern.MatchString(result.CapabilitySHA256) {
		result.Issues = append(result.Issues, compilerIssue(CodeCapabilityContextInvalid, "capability_sha256", "validated installed capability snapshot hash is required"))
	}
	if proposal.Version != ProposalVersion {
		result.Issues = append(result.Issues, compilerIssue(CodeProposalInvalid, "proposal.version", fmt.Sprintf("behavioral intent proposal version must be %d", ProposalVersion)))
	}
	requirementIDs := map[string]bool{}
	materialRequirementIDs := map[string]bool{}
	if proposal.Requirement != nil {
		normalized := architecturesearch.Normalize(*proposal.Requirement)
		applyMandatoryAcceptance(&normalized.Acceptance)
		for _, issue := range architecturesearch.Validate(normalized) {
			issue.Path = prefixedPath("proposal.requirement", issue.Path)
			result.Issues = append(result.Issues, issue)
		}
		result.Requirement = &normalized
		requirementIDs, materialRequirementIDs = collectRequirementIDs(normalized)
	}
	result.Issues = append(result.Issues, validateBlockerNamespaces(result.Uncertainties, result.Clarifications, result.CapabilityGaps)...)
	result.Issues = append(result.Issues, validateClarifications(result.Clarifications, result.Uncertainties)...)
	result.Issues = append(result.Issues, validateCapabilityGaps(result.CapabilityGaps)...)
	result.Issues = append(result.Issues, validateUncertainties(result.Uncertainties, result.Clarifications, result.CapabilityGaps)...)
	result.Issues = append(result.Issues, validateCoverage(source, result.Coverage, requirementIDs, materialRequirementIDs, result.Uncertainties, result.Clarifications, result.CapabilityGaps)...)
	if reports.HasBlockingIssue(result.Issues) {
		result.Requirement = nil
		return result
	}
	switch {
	case len(result.CapabilityGaps) > 0:
		result.Status = StatusUnsupported
		result.Requirement = nil
	case len(result.Clarifications) > 0:
		result.Status = StatusNeedsClarification
		result.Requirement = nil
	case proposal.Requirement == nil:
		result.Issues = append(result.Issues, compilerIssue(CodeRequirementInvalid, "proposal.requirement", "ready behavioral intent requires a strict v3 requirement"))
		result.Requirement = nil
	default:
		result.Status = StatusReady
	}
	return result
}

func normalizeCoverage(values []CoverageRecord) []CoverageRecord {
	result := slices.Clone(values)
	for index := range result {
		result[index].StatementID = strings.TrimSpace(result[index].StatementID)
		result[index].Rationale = strings.TrimSpace(result[index].Rationale)
		result[index].References = slices.Clone(result[index].References)
		for refIndex := range result[index].References {
			result[index].References[refIndex].Kind = strings.TrimSpace(result[index].References[refIndex].Kind)
			result[index].References[refIndex].ID = strings.TrimSpace(result[index].References[refIndex].ID)
		}
		slices.SortFunc(result[index].References, compareReference)
	}
	slices.SortFunc(result, func(left, right CoverageRecord) int { return strings.Compare(left.StatementID, right.StatementID) })
	return result
}

func normalizeUncertainties(values []Uncertainty) []Uncertainty {
	result := slices.Clone(values)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Path = strings.TrimSpace(result[index].Path)
		result[index].Kind = strings.TrimSpace(result[index].Kind)
		result[index].Description = strings.TrimSpace(result[index].Description)
		result[index].ResolvedBy = strings.TrimSpace(result[index].ResolvedBy)
	}
	slices.SortFunc(result, func(left, right Uncertainty) int { return strings.Compare(left.ID, right.ID) })
	return result
}

func normalizeClarifications(values []Clarification) []Clarification {
	result := slices.Clone(values)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Path = strings.TrimSpace(result[index].Path)
		result[index].Question = strings.TrimSpace(result[index].Question)
		result[index].WhyNeeded = strings.TrimSpace(result[index].WhyNeeded)
		result[index].Choices = trimmedSortedUnique(result[index].Choices)
		result[index].UncertaintyIDs = trimmedSortedUnique(result[index].UncertaintyIDs)
	}
	slices.SortFunc(result, func(left, right Clarification) int { return strings.Compare(left.ID, right.ID) })
	return result
}

func normalizeCapabilityGaps(values []CapabilityGap) []CapabilityGap {
	result := slices.Clone(values)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Capability = strings.TrimSpace(result[index].Capability)
		result[index].Path = strings.TrimSpace(result[index].Path)
		result[index].Reason = strings.TrimSpace(result[index].Reason)
		result[index].RequiredEvidence = trimmedSortedUnique(result[index].RequiredEvidence)
	}
	slices.SortFunc(result, func(left, right CapabilityGap) int { return strings.Compare(left.ID, right.ID) })
	return result
}

func validateBlockerNamespaces(uncertainties []Uncertainty, clarifications []Clarification, gaps []CapabilityGap) []reports.Issue {
	var issues []reports.Issue
	owners := map[string]string{}
	for _, values := range []struct {
		kind string
		ids  []string
	}{
		{kind: "uncertainty", ids: uncertaintyIDs(uncertainties)},
		{kind: "clarification", ids: clarificationIDs(clarifications)},
		{kind: "capability gap", ids: gapIDs(gaps)},
	} {
		for _, id := range values.ids {
			if owner, exists := owners[id]; exists && id != "" {
				issues = append(issues, compilerIssue(CodeProposalInvalid, "proposal", values.kind+" id "+id+" conflicts with "+owner+" id"))
			}
			owners[id] = values.kind
		}
	}
	if len(clarifications) != 0 && len(gaps) != 0 {
		issues = append(issues, compilerIssue(CodeProposalInvalid, "proposal", "a proposal cannot mix clarification and capability-gap terminal outcomes"))
	}
	return issues
}

func validateClarifications(values []Clarification, uncertainties []Uncertainty) []reports.Issue {
	var issues []reports.Issue
	ids := map[string]bool{}
	paths := map[string]bool{}
	uncertaintyByID := map[string]Uncertainty{}
	clarificationUncertaintiesByPath := map[string][]string{}
	for _, uncertainty := range uncertainties {
		uncertaintyByID[uncertainty.ID] = uncertainty
		if uncertainty.Resolution == ResolutionClarification {
			clarificationUncertaintiesByPath[uncertainty.Path] = append(clarificationUncertaintiesByPath[uncertainty.Path], uncertainty.ID)
		}
	}
	for index, value := range values {
		path := fmt.Sprintf("proposal.clarifications[%d]", index)
		if !validSemanticID(value.ID) || ids[value.ID] {
			issues = append(issues, compilerIssue(CodeClarificationInvalid, path+".id", "clarification id must be unique normalized semantic identity"))
		}
		ids[value.ID] = true
		if value.Path == "" || paths[value.Path] {
			issues = append(issues, compilerIssue(CodeClarificationInvalid, path+".path", "clarifications must use one unique requirement path so questions remain minimal"))
		}
		paths[value.Path] = true
		if value.Question == "" || value.WhyNeeded == "" || len(value.UncertaintyIDs) == 0 {
			issues = append(issues, compilerIssue(CodeClarificationInvalid, path, "clarification requires a targeted question, rationale, and at least one uncertainty"))
		}
		for _, uncertaintyID := range value.UncertaintyIDs {
			uncertainty, exists := uncertaintyByID[uncertaintyID]
			if !exists || uncertainty.Path != value.Path || uncertainty.Resolution != ResolutionClarification || uncertainty.ResolvedBy != value.ID {
				issues = append(issues, compilerIssue(CodeClarificationInvalid, path+".uncertainty_ids", "clarification may own only clarification-resolved uncertainties at its exact requirement path"))
			}
		}
		expected := trimmedSortedUnique(clarificationUncertaintiesByPath[value.Path])
		if !slices.Equal(value.UncertaintyIDs, expected) {
			issues = append(issues, compilerIssue(CodeClarificationInvalid, path+".uncertainty_ids", "one clarification must coalesce every unresolved uncertainty at its requirement path"))
		}
	}
	return issues
}

func validateCapabilityGaps(values []CapabilityGap) []reports.Issue {
	var issues []reports.Issue
	ids := map[string]bool{}
	for index, value := range values {
		path := fmt.Sprintf("proposal.capability_gaps[%d]", index)
		if !validSemanticID(value.ID) || ids[value.ID] {
			issues = append(issues, compilerIssue(CodeCapabilityGapInvalid, path+".id", "capability gap id must be unique normalized semantic identity"))
		}
		ids[value.ID] = true
		if !validSemanticID(value.Capability) || value.Path == "" || value.Reason == "" || len(value.RequiredEvidence) == 0 {
			issues = append(issues, compilerIssue(CodeCapabilityGapInvalid, path, "capability gap requires semantic capability, path, reason, and evidence needed to close the gap"))
		}
	}
	return issues
}

func validateUncertainties(values []Uncertainty, clarifications []Clarification, gaps []CapabilityGap) []reports.Issue {
	var issues []reports.Issue
	ids := map[string]bool{}
	clarificationIDs := idSetClarifications(clarifications)
	gapIDs := idSetGaps(gaps)
	covered := map[string]int{}
	for _, clarification := range clarifications {
		for _, id := range clarification.UncertaintyIDs {
			covered[id]++
		}
	}
	for index, value := range values {
		path := fmt.Sprintf("proposal.uncertainties[%d]", index)
		if !validSemanticID(value.ID) || ids[value.ID] {
			issues = append(issues, compilerIssue(CodeUncertaintyInvalid, path+".id", "uncertainty id must be unique normalized semantic identity"))
		}
		ids[value.ID] = true
		if value.Path == "" || !validSemanticID(value.Kind) || value.Description == "" {
			issues = append(issues, compilerIssue(CodeUncertaintyInvalid, path, "uncertainty requires path, semantic kind, and description"))
		}
		switch value.Resolution {
		case ResolutionExplicit, ResolutionBounded:
			if value.ResolvedBy != "" {
				issues = append(issues, compilerIssue(CodeUncertaintyInvalid, path+".resolved_by", "explicit or bounded uncertainty cannot reference a blocker"))
			}
		case ResolutionClarification:
			if !clarificationIDs[value.ResolvedBy] || covered[value.ID] != 1 {
				issues = append(issues, compilerIssue(CodeUncertaintyInvalid, path+".resolved_by", "clarification uncertainty must be owned by exactly one clarification"))
			}
		case ResolutionCapabilityGap:
			if !gapIDs[value.ResolvedBy] {
				issues = append(issues, compilerIssue(CodeUncertaintyInvalid, path+".resolved_by", "capability uncertainty must reference an existing capability gap"))
			}
		default:
			issues = append(issues, compilerIssue(CodeUncertaintyInvalid, path+".resolution", "unsupported uncertainty resolution"))
		}
	}
	for _, clarification := range clarifications {
		for _, id := range clarification.UncertaintyIDs {
			if !ids[id] {
				issues = append(issues, compilerIssue(CodeClarificationInvalid, "proposal.clarifications."+clarification.ID, "clarification references unknown uncertainty "+id))
			}
		}
	}
	return issues
}

func validateCoverage(source Source, coverage []CoverageRecord, requirementIDs, materialRequirementIDs map[string]bool, uncertainties []Uncertainty, clarifications []Clarification, gaps []CapabilityGap) []reports.Issue {
	var issues []reports.Issue
	statementIDs := map[string]bool{}
	for _, statement := range source.Statements {
		statementIDs[statement.ID] = true
	}
	seen := map[string]bool{}
	uncertaintyIDs := idSetUncertainties(uncertainties)
	clarificationIDs := idSetClarifications(clarifications)
	gapIDs := idSetGaps(gaps)
	referencedRequirements := map[string]bool{}
	referencedUncertainties := map[string]bool{}
	referencedClarifications := map[string]bool{}
	referencedGaps := map[string]bool{}
	for index, record := range coverage {
		path := fmt.Sprintf("proposal.coverage[%d]", index)
		if !statementIDs[record.StatementID] || seen[record.StatementID] {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, path+".statement_id", "coverage must reference each compiler-owned source statement exactly once"))
		}
		seen[record.StatementID] = true
		if record.Rationale == "" {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, path+".rationale", "source coverage requires an explicit accounting rationale"))
		}
		if record.Disposition == DispositionContext {
			if len(record.References) != 0 {
				issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, path+".references", "context-only source coverage cannot claim requirement or blocker references"))
			}
			continue
		}
		if len(record.References) == 0 {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, path+".references", "material source coverage requires at least one normalized reference"))
		}
		for _, reference := range record.References {
			valid := false
			switch reference.Kind {
			case "requirement":
				valid = requirementIDs[reference.ID]
				if valid {
					referencedRequirements[reference.ID] = true
				}
			case "uncertainty":
				valid = uncertaintyIDs[reference.ID]
				if valid {
					referencedUncertainties[reference.ID] = true
				}
			case "clarification":
				valid = clarificationIDs[reference.ID]
				if valid {
					referencedClarifications[reference.ID] = true
				}
			case "capability_gap":
				valid = gapIDs[reference.ID]
				if valid {
					referencedGaps[reference.ID] = true
				}
			}
			if !valid || !referenceMatchesDisposition(record.Disposition, reference.Kind) {
				issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, path+".references", "coverage reference is unknown or inconsistent with its disposition"))
			}
		}
	}
	for id := range statementIDs {
		if !seen[id] {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, "proposal.coverage", "source statement "+id+" is not accounted for"))
		}
	}
	for id := range materialRequirementIDs {
		if !referencedRequirements[id] {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, "proposal.coverage", "material requirement "+id+" has no source statement evidence"))
		}
	}
	for id := range uncertaintyIDs {
		if !referencedUncertainties[id] {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, "proposal.coverage", "uncertainty "+id+" has no source statement evidence"))
		}
	}
	for id := range clarificationIDs {
		if !referencedClarifications[id] {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, "proposal.coverage", "clarification "+id+" has no source statement evidence"))
		}
	}
	for id := range gapIDs {
		if !referencedGaps[id] {
			issues = append(issues, compilerIssue(CodeSourceCoverageInvalid, "proposal.coverage", "capability gap "+id+" has no source statement evidence"))
		}
	}
	return issues
}

func uncertaintyIDs(values []Uncertainty) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}

func clarificationIDs(values []Clarification) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}

func gapIDs(values []CapabilityGap) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}

func collectRequirementIDs(requirement architecturesearch.Requirement) (map[string]bool, map[string]bool) {
	ids := map[string]bool{}
	material := map[string]bool{}
	for _, value := range requirement.Requirements.Domains {
		ids[value.ID] = true
		material[value.ID] = true
	}
	for _, value := range requirement.Requirements.Ports {
		ids[value.ID] = true
		material[value.ID] = true
	}
	for _, value := range requirement.Requirements.Signals {
		ids[value.ID] = true
	}
	for _, value := range requirement.Requirements.Participants {
		ids[value.ID] = true
		material[value.ID] = true
	}
	for _, value := range requirement.Requirements.Objectives {
		ids[value.ID] = true
		material[value.ID] = true
	}
	for _, value := range requirement.Requirements.OperatingCases {
		ids[value.ID] = true
		material[value.ID] = true
	}
	for _, value := range requirement.Requirements.BehavioralRequirements {
		ids[value.ID] = true
		material[value.ID] = true
	}
	return ids, material
}

func applyMandatoryAcceptance(value *architecturesearch.Acceptance) {
	*value = architecturesearch.Acceptance{
		RequireERC: true, RequireStrictDRC: true, RequireCompleteRouting: true,
		RequireConnectivity: true, RequireWriterCorrectness: true, RequireRoundTripZeroDiff: true,
		RequireDeterministicReplay: true, RequireContractComposition: true, RequireGlobalReasoning: true,
		RequireCoverageAccounting: true, RequireAlternatives: true, RequireFailClosed: true,
		RequireSimulation: true, RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true,
	}
}

func referenceMatchesDisposition(disposition Disposition, kind string) bool {
	switch disposition {
	case DispositionCompiled:
		return kind == "requirement" || kind == "uncertainty"
	case DispositionClarification:
		return kind == "clarification" || kind == "uncertainty"
	case DispositionCapabilityGap:
		return kind == "capability_gap" || kind == "uncertainty"
	default:
		return false
	}
}

func validSemanticID(value string) bool { return semanticIDPattern.MatchString(value) }

func compareReference(left, right Reference) int {
	if value := strings.Compare(left.Kind, right.Kind); value != 0 {
		return value
	}
	return strings.Compare(left.ID, right.ID)
}

func trimmedSortedUnique(values []string) []string {
	set := map[string]bool{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func idSetUncertainties(values []Uncertainty) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value.ID] = true
	}
	return result
}

func idSetClarifications(values []Clarification) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value.ID] = true
	}
	return result
}

func idSetGaps(values []CapabilityGap) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value.ID] = true
	}
	return result
}

func compilerIssue(code reports.Code, path, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityError, Path: path, Message: message}
}

func prefixedPath(prefix, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "document" {
		return prefix
	}
	return prefix + "." + path
}
