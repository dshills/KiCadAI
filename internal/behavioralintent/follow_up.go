package behavioralintent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

// BindFollowUp binds user answers to one exact clarification result. Callers
// still pass the returned value through ValidateFollowUp before provider use.
func BindFollowUp(priorProposal Proposal, prior Result, answers []ClarificationAnswer) (FollowUp, error) {
	proposalHash, err := valueSHA256(priorProposal)
	if err != nil {
		return FollowUp{}, fmt.Errorf("hash prior behavioral proposal: %w", err)
	}
	compilationHash, err := valueSHA256(prior)
	if err != nil {
		return FollowUp{}, fmt.Errorf("hash prior behavioral compilation: %w", err)
	}
	return FollowUp{
		Schema: FollowUpSchema, Version: FollowUpVersion, SourceSHA256: prior.Source.SHA256,
		CapabilitySHA256: prior.CapabilitySHA256, PriorProposalSHA256: proposalHash,
		PriorCompilationSHA256: compilationHash, Answers: normalizeAnswers(answers),
	}, nil
}

// ValidateFollowUp proves that answers name only uncertainties owned by the
// exact prior clarification result and that all prior evidence is reproducible.
func ValidateFollowUp(prompt string, priorProposal Proposal, prior Result, followUp FollowUp, capabilitySHA256 string) (FollowUp, []reports.Issue) {
	normalized := followUp
	normalized.Schema = strings.TrimSpace(normalized.Schema)
	normalized.SourceSHA256 = strings.TrimSpace(normalized.SourceSHA256)
	normalized.CapabilitySHA256 = strings.TrimSpace(normalized.CapabilitySHA256)
	normalized.PriorProposalSHA256 = strings.TrimSpace(normalized.PriorProposalSHA256)
	normalized.PriorCompilationSHA256 = strings.TrimSpace(normalized.PriorCompilationSHA256)
	normalized.Answers = normalizeAnswers(normalized.Answers)

	var issues []reports.Issue
	if normalized.Schema != FollowUpSchema || normalized.Version != FollowUpVersion {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "follow_up", fmt.Sprintf("follow-up schema/version must be %s version %d", FollowUpSchema, FollowUpVersion)))
	}
	prepared := PrepareSource(prompt)
	if prior.Status != StatusNeedsClarification || prior.Requirement != nil || reports.HasBlockingIssue(prior.Issues) {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "prior_compilation.status", "follow-up requires a valid non-executable needs_clarification result"))
	}
	if prepared.SHA256 != prior.Source.SHA256 || normalized.SourceSHA256 != prior.Source.SHA256 {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "follow_up.source_sha256", "follow-up source does not match the complete original source"))
	}
	capabilitySHA256 = strings.TrimSpace(capabilitySHA256)
	if !sha256Pattern.MatchString(capabilitySHA256) || prior.CapabilitySHA256 != capabilitySHA256 || normalized.CapabilitySHA256 != capabilitySHA256 {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "follow_up.capability_sha256", "follow-up capability snapshot does not match the prior compilation"))
	}
	recompiled := Compile(prompt, priorProposal, capabilitySHA256)
	if hash, err := valueSHA256(recompiled); err != nil || !sameHash(hash, normalized.PriorCompilationSHA256) {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "follow_up.prior_compilation_sha256", "prior compilation is not reproducible from the original source and proposal"))
	}
	if hash, err := valueSHA256(prior); err != nil || !sameHash(hash, normalized.PriorCompilationSHA256) {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "follow_up.prior_compilation_sha256", "follow-up is not bound to the supplied prior compilation"))
	}
	if hash, err := valueSHA256(priorProposal); err != nil || !sameHash(hash, normalized.PriorProposalSHA256) {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "follow_up.prior_proposal_sha256", "follow-up is not bound to the supplied prior proposal"))
	}
	if len(normalized.Answers) == 0 {
		issues = append(issues, compilerIssue(CodeFollowUpInvalid, "follow_up.answers", "at least one clarification answer is required"))
	}
	clarifications := map[string]Clarification{}
	for _, clarification := range prior.Clarifications {
		clarifications[clarification.ID] = clarification
	}
	seen := map[string]bool{}
	for index, answer := range normalized.Answers {
		path := fmt.Sprintf("follow_up.answers[%d]", index)
		if !validSemanticID(answer.ClarificationID) || seen[answer.ClarificationID] {
			issues = append(issues, compilerIssue(CodeFollowUpInvalid, path+".clarification_id", "answer must name one unique prior clarification"))
		}
		seen[answer.ClarificationID] = true
		clarification, exists := clarifications[answer.ClarificationID]
		if !exists {
			issues = append(issues, compilerIssue(CodeFollowUpInvalid, path+".clarification_id", "answer references an unknown or unrelated clarification"))
		}
		if answer.Answer == "" || len(answer.Answer) > MaxAnswerBytes {
			issues = append(issues, compilerIssue(CodeFollowUpInvalid, path+".answer", fmt.Sprintf("answer must contain 1 to %d bytes", MaxAnswerBytes)))
		}
		if !slices.Equal(answer.UncertaintyIDs, clarification.UncertaintyIDs) {
			issues = append(issues, compilerIssue(CodeFollowUpInvalid, path+".uncertainty_ids", "answer must name exactly the uncertainties owned by its clarification"))
		}
	}
	return normalized, issues
}

// CompileFollowUp recompiles the complete original source and rejects circular
// output that tries to reuse an answered clarification or uncertainty identity.
func CompileFollowUp(prompt string, priorProposal Proposal, prior Result, followUp FollowUp, proposal Proposal, capabilitySHA256 string) Result {
	normalized, followUpIssues := ValidateFollowUp(prompt, priorProposal, prior, followUp, capabilitySHA256)
	result := Compile(prompt, proposal, capabilitySHA256)
	result.Issues = append(followUpIssues, result.Issues...)
	answeredClarifications := map[string]bool{}
	answeredUncertainties := map[string]bool{}
	for _, answer := range normalized.Answers {
		answeredClarifications[answer.ClarificationID] = true
		for _, id := range answer.UncertaintyIDs {
			answeredUncertainties[id] = true
		}
	}
	for _, clarification := range result.Clarifications {
		if answeredClarifications[clarification.ID] {
			result.Issues = append(result.Issues, compilerIssue(CodeFollowUpInvalid, "proposal.clarifications."+clarification.ID, "answered clarification identity cannot recur in follow-up output"))
		}
	}
	for _, uncertainty := range result.Uncertainties {
		if answeredUncertainties[uncertainty.ID] {
			result.Issues = append(result.Issues, compilerIssue(CodeFollowUpInvalid, "proposal.uncertainties."+uncertainty.ID, "answered uncertainty identity cannot recur in follow-up output"))
		}
	}
	if reports.HasBlockingIssue(result.Issues) {
		result.Status = StatusInvalid
		result.Requirement = nil
	}
	return result
}

// BuildFollowUpProviderContext exposes scoped answers alongside the complete
// original source. It never mutates or concatenates answer text into the source.
func BuildFollowUpProviderContext(prompt string, capabilities json.RawMessage, priorProposal Proposal, prior Result, followUp FollowUp) (string, []reports.Issue) {
	base, err := BuildProviderContext(prompt, capabilities)
	if err != nil {
		return "", []reports.Issue{compilerIssue(CodeFollowUpInvalid, "follow_up", err.Error())}
	}
	var context ProviderContext
	if err := json.Unmarshal([]byte(base), &context); err != nil {
		return "", []reports.Issue{compilerIssue(CodeFollowUpInvalid, "follow_up", "decode provider context: "+err.Error())}
	}
	normalized, issues := ValidateFollowUp(prompt, priorProposal, prior, followUp, context.CapabilitySHA256)
	if reports.HasBlockingIssue(issues) {
		return "", issues
	}
	context.FollowUp = &ProviderFollowUp{PriorProposal: priorProposal, PriorCompilation: prior, Input: normalized}
	context.Policy = append(context.Policy, "apply each follow-up answer only to the named clarification and uncertainty identities; recompile every original source statement")
	encoded, err := json.Marshal(context)
	if err != nil {
		return "", []reports.Issue{compilerIssue(CodeFollowUpInvalid, "follow_up", "encode provider context: "+err.Error())}
	}
	return string(encoded), nil
}

func normalizeAnswers(values []ClarificationAnswer) []ClarificationAnswer {
	result := slices.Clone(values)
	for index := range result {
		result[index].ClarificationID = strings.TrimSpace(result[index].ClarificationID)
		result[index].UncertaintyIDs = trimmedSortedUnique(result[index].UncertaintyIDs)
		result[index].Answer = strings.TrimSpace(result[index].Answer)
	}
	slices.SortFunc(result, func(left, right ClarificationAnswer) int {
		return strings.Compare(left.ClarificationID, right.ClarificationID)
	})
	return result
}

func valueSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func sameHash(actual, expected string) bool {
	return sha256Pattern.MatchString(expected) && actual == expected
}
