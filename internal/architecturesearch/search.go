package architecturesearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const searchResultSchema = "kicadai.architecture-search-result.v1"

type searchObligation struct {
	Path        string
	Capability  string
	Ports       []RoleContract
	Constraints []Constraint
}

type searchState struct {
	Depth       int
	Obligations []searchObligation
	Selections  []FragmentSelection
}

type searchAccumulator struct {
	policy      SearchPolicy
	requirement Requirement
	registry    *Registry
	result      SearchResult
	rejections  []ExpansionRejection
	complete    []CandidateResult
	visited     map[string]bool
	budgetHit   bool
}

func Search(ctx context.Context, requirement Requirement, registry *Registry, options SearchOptions) SearchResult {
	normalized := Normalize(requirement)
	policy, policyIssues := effectiveSearchPolicy(options.Policy)
	result := SearchResult{
		Schema: searchResultSchema, PolicyVersion: PolicyVersion, Status: SearchFailed,
		RegistryHash: registry.Hash(), CatalogHash: strings.TrimSpace(options.CatalogHash), FormulaLibraryHash: FormulaLibraryHash(), Policy: policy,
	}
	validationIssues := Validate(normalized)
	result.Issues = append(result.Issues, policyIssues...)
	result.Issues = append(result.Issues, validationIssues...)
	if registry == nil {
		result.Issues = append(result.Issues, architectureIssue(CodeProviderInvalid, "providers", "architecture search requires a provider registry"))
	}
	if len(result.Issues) != 0 {
		sortIssues(result.Issues)
		return result
	}
	requirementHash, err := CanonicalHash(normalized)
	if err != nil {
		result.Issues = []reports.Issue{architectureIssue(CodeSchemaInvalid, "document", "hash architecture requirement: "+err.Error())}
		return result
	}
	result.RequirementHash = requirementHash
	obligations, obligationIssues := initialSearchObligations(normalized, EvidenceRuleInferred)
	if len(obligationIssues) != 0 {
		result.Issues = obligationIssues
		sortIssues(result.Issues)
		return result
	}
	accumulator := searchAccumulator{
		policy: policy, requirement: normalized, registry: registry, result: result,
		visited: map[string]bool{},
	}
	initial := normalizeSearchState(searchState{Obligations: obligations})
	initialKey := searchStateKey(initial)
	accumulator.visited[initialKey] = true
	frontier := []searchState{initial}
	accumulator.result.Consumption.MaximumFrontier = 1

	for len(frontier) != 0 {
		if err := ctx.Err(); err != nil {
			accumulator.result.Status = SearchFailed
			accumulator.result.Issues = []reports.Issue{architectureIssue(CodeSearchCanceled, "search", "architecture search canceled: "+err.Error())}
			return accumulator.finish()
		}
		slices.SortStableFunc(frontier, func(left, right searchState) int {
			return strings.Compare(searchStateKey(left), searchStateKey(right))
		})
		state := frontier[0]
		frontier = frontier[1:]
		if len(state.Obligations) == 0 {
			candidate, err := candidateFromState(state)
			if err != nil {
				accumulator.reject(CodeProviderExpansionInvalid, "candidate", "", "", err.Error())
				continue
			}
			accumulator.complete = append(accumulator.complete, candidate)
			continue
		}
		if accumulator.result.Consumption.ExpandedStates >= policy.MaxExpandedStates {
			accumulator.budgetHit = true
			break
		}
		if state.Depth >= policy.MaxDepth {
			accumulator.budgetHit = true
			accumulator.reject(CodeSearchBudgetExhausted, state.Obligations[0].Path, "", "", "candidate reached maximum search depth")
			continue
		}
		accumulator.result.Consumption.ExpandedStates++
		if state.Depth > accumulator.result.Consumption.MaximumDepthReached {
			accumulator.result.Consumption.MaximumDepthReached = state.Depth
		}
		obligation := state.Obligations[0]
		remaining := append([]searchObligation(nil), state.Obligations[1:]...)
		providers := registry.providersFor(obligation.Capability)
		if len(providers) == 0 {
			accumulator.reject(CodeCapabilityUnsupported, obligation.Path, "", "", "no registered provider supplies capability "+obligation.Capability)
			continue
		}
		generatedForObligation := 0
		for _, provider := range providers {
			request := providerRequestFor(obligation, normalized.Requirements.Constraints)
			expansions, err := provider.provider.Expand(ctx, request)
			if err != nil {
				accumulator.reject(CodeProviderExpansionInvalid, obligation.Path, provider.descriptor.ID, "", "provider expansion failed: "+err.Error())
				continue
			}
			for index := range expansions {
				expansions[index] = normalizeProviderExpansion(expansions[index])
			}
			slices.SortStableFunc(expansions, func(left, right ProviderExpansion) int { return strings.Compare(left.ID, right.ID) })
			if len(expansions) > policy.MaxProviderExpansions {
				accumulator.reject(CodeProviderExpansionLimit, obligation.Path, provider.descriptor.ID, "", fmt.Sprintf("provider returned %d expansions; maximum is %d", len(expansions), policy.MaxProviderExpansions))
				accumulator.budgetHit = true
				expansions = expansions[:policy.MaxProviderExpansions]
			}
			seenExpansionIDs := map[string]bool{}
			for _, expansion := range expansions {
				if seenExpansionIDs[expansion.ID] {
					accumulator.reject(CodeProviderExpansionInvalid, obligation.Path, provider.descriptor.ID, expansion.ID, "provider expansion id is duplicated")
					continue
				}
				seenExpansionIDs[expansion.ID] = true
				selection, children, rejections := validateProviderExpansion(obligation, provider.descriptor, expansion, policy)
				if len(rejections) != 0 {
					for _, rejection := range rejections {
						accumulator.reject(rejection.Code, rejection.Path, provider.descriptor.ID, expansion.ID, rejection.Message)
					}
					continue
				}
				componentCount := selectedComponentCount(state.Selections) + len(selection.Components)
				componentLimit := minInt(policy.MaxComponents, normalized.Requirements.Constraints.MaxComponents)
				if componentCount > componentLimit {
					accumulator.reject(CodeLimitExceeded, obligation.Path, provider.descriptor.ID, expansion.ID, fmt.Sprintf("candidate component count %d exceeds limit %d", componentCount, componentLimit))
					continue
				}
				if len(remaining)+len(children) > policy.MaxUnresolvedObligations {
					accumulator.reject(CodeLimitExceeded, obligation.Path, provider.descriptor.ID, expansion.ID, "candidate exceeds unresolved-obligation limit")
					continue
				}
				candidateSelections := append(append([]FragmentSelection(nil), state.Selections...), selection)
				if area := selectedAreaMM2(candidateSelections); area > normalized.Requirements.Constraints.MaxWidthMM*normalized.Requirements.Constraints.MaxHeightMM {
					accumulator.reject(CodeLimitExceeded, obligation.Path, provider.descriptor.ID, expansion.ID, "candidate area estimate exceeds board-area limit")
					continue
				}
				candidateObligations := append(append([]searchObligation(nil), remaining...), namespaceChildren(obligation, provider.descriptor, expansion, children)...)
				next := normalizeSearchState(searchState{Depth: state.Depth + 1, Obligations: candidateObligations, Selections: candidateSelections})
				key := searchStateKey(next)
				if accumulator.visited[key] {
					continue
				}
				accumulator.visited[key] = true
				frontier = append(frontier, next)
				generatedForObligation++
				accumulator.result.Consumption.GeneratedStates++
			}
		}
		if generatedForObligation == 0 {
			// Detailed provider rejections already explain why the branch ended.
		}
		if len(frontier) > accumulator.result.Consumption.MaximumFrontier {
			accumulator.result.Consumption.MaximumFrontier = len(frontier)
		}
	}
	return accumulator.finish()
}

func (accumulator *searchAccumulator) finish() SearchResult {
	accumulator.result.Consumption.CompleteCandidates = len(accumulator.complete)
	accumulator.result.Consumption.RejectedExpansions = len(accumulator.rejections)
	accumulator.result.Rejections = summarizeRejections(accumulator.rejections, accumulator.policy.MaxRejectionSamples)
	if accumulator.result.Status == SearchFailed && len(accumulator.result.Issues) != 0 {
		sortIssues(accumulator.result.Issues)
		return accumulator.result
	}
	if accumulator.budgetHit {
		accumulator.result.Status = SearchExhausted
		accumulator.result.Issues = []reports.Issue{architectureIssue(CodeSearchBudgetExhausted, "search", "search budget exhausted before the candidate frontier was proven complete")}
		return accumulator.result
	}
	if len(accumulator.complete) == 0 {
		accumulator.result.Status = SearchUnsupported
		code := CodeSearchNoCandidate
		message := "no complete architecture candidate satisfies the requirement"
		if rejectionSummaryContains(accumulator.result.Rejections, CodeCapabilityUnsupported) {
			code = CodeCapabilityUnsupported
			message = "one or more required capabilities have no registered provider"
		}
		accumulator.result.Issues = []reports.Issue{architectureIssue(code, "search", message)}
		return accumulator.result
	}
	slices.SortStableFunc(accumulator.complete, compareCandidateResults)
	if len(accumulator.complete) > 1 {
		for _, candidate := range accumulator.complete[1:] {
			if !scoresEquivalentBeforeFingerprint(accumulator.complete[0].Score, candidate.Score) {
				break
			}
			if candidatesRequireChoice(accumulator.complete[0], candidate) {
				accumulator.result.Status = SearchAmbiguous
				accumulator.result.Issues = []reports.Issue{architectureIssue(CodeSearchAmbiguous, "search", "top-ranked architectures require an unresolved user-visible choice")}
				return accumulator.result
			}
		}
	}
	accumulator.result.Status = SearchSelected
	selected := accumulator.complete[0]
	accumulator.result.Selected = &selected
	limit := minInt(len(accumulator.complete), accumulator.policy.MaxCompleteCandidates)
	if limit > 1 {
		accumulator.result.Alternatives = append([]CandidateResult(nil), accumulator.complete[1:limit]...)
	}
	rationale := buildSelectionRationale(selected, accumulator.result.Alternatives)
	accumulator.result.Rationale = &rationale
	return accumulator.result
}

func initialSearchObligations(requirement Requirement, minimumEvidence EvidenceConfidence) ([]searchObligation, []reports.Issue) {
	var obligations []searchObligation
	var issues []reports.Issue
	for _, participant := range requirement.Requirements.Participants {
		ports := make([]RoleContract, 0, len(participant.RequiredPorts)+2)
		for _, port := range participant.RequiredPorts {
			binding := Binding{Role: port.ID, Participant: participant.ID, ParticipantPort: port.ID}
			contract, contractIssues := ContractFromBinding(requirement, binding, minimumEvidence)
			issues = append(issues, contractIssues...)
			ports = append(ports, RoleContract{Role: port.ID, Anchor: participantAnchor(participant.ID, port.ID), Contract: contract})
		}
		domain := requirementDomain(requirement, participant.Domain)
		powerContract := PortContract{
			ID: participant.ID + "_power", Kind: "power", Direction: "sink", Domain: participant.Domain,
			Voltage: domainVoltageRange(domain), MaximumCurrentDemandA: cloneFloat64(domain.MaxCurrentA),
			Evidence:        ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:participant-domain-contract"}},
			MinimumEvidence: minimumEvidence,
		}
		ports = append(ports, RoleContract{Role: "power", Anchor: domainAnchor(participant.Domain), Contract: NormalizePortContract(powerContract)})
		referenceDomain := firstReferenceDomain(requirement)
		referenceContract := PortContract{
			ID: participant.ID + "_reference", Kind: "reference", Direction: "bidirectional", Domain: referenceDomain.ID,
			Voltage:         domainVoltageRange(referenceDomain),
			Evidence:        ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:participant-domain-contract"}},
			MinimumEvidence: minimumEvidence,
		}
		ports = append(ports, RoleContract{Role: "reference", Anchor: domainAnchor(referenceDomain.ID), Contract: NormalizePortContract(referenceContract)})
		obligations = append(obligations, searchObligation{Path: "participant:" + participant.ID, Capability: participant.Capability, Ports: ports, Constraints: cloneConstraints(participant.Constraints)})
	}
	for _, objective := range requirement.Requirements.Objectives {
		ports := make([]RoleContract, 0, len(objective.Bindings))
		for _, binding := range objective.Bindings {
			contract, contractIssues := ContractFromBinding(requirement, binding, minimumEvidence)
			issues = append(issues, contractIssues...)
			anchor := externalAnchor(binding.Port)
			if binding.Participant != "" {
				anchor = participantAnchor(binding.Participant, binding.ParticipantPort)
			}
			ports = append(ports, RoleContract{Role: binding.Role, Anchor: anchor, Contract: contract})
		}
		obligations = append(obligations, searchObligation{Path: "objective:" + objective.ID, Capability: objective.Capability, Ports: ports, Constraints: cloneConstraints(objective.Constraints)})
	}
	slices.SortStableFunc(obligations, compareSearchObligations)
	return obligations, issues
}

func providerRequestFor(obligation searchObligation, limits BoardLimits) ProviderRequest {
	ports := make([]RoleContract, len(obligation.Ports))
	for index, role := range obligation.Ports {
		role.Anchor = ""
		role.Contract.ID = ""
		ports[index] = role
	}
	return ProviderRequest{Capability: obligation.Capability, Ports: ports, Constraints: cloneConstraints(obligation.Constraints), BoardLimits: limits}
}

func validateProviderExpansion(obligation searchObligation, descriptor ProviderDescriptor, expansion ProviderExpansion, policy SearchPolicy) (FragmentSelection, []ChildObligation, []ExpansionRejection) {
	var rejections []ExpansionRejection
	if !validSemanticID(expansion.ID) {
		rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: obligation.Path, Message: "expansion id must be a normalized semantic identifier"})
	}
	if !validEvidenceConfidence(expansion.Evidence.Confidence) || confidenceRank(expansion.Evidence.Confidence) < confidenceRank(EvidenceRuleInferred) {
		rejections = append(rejections, ExpansionRejection{Code: CodeEvidenceInsufficient, Path: obligation.Path + ".evidence", Message: "expansion evidence is below rule_inferred"})
	}
	if expansion.Metrics.UnprovenNonSafety < 0 || !validOptionalNonnegative(expansion.Metrics.QuiescentPowerW) || !validOptionalNonnegative(expansion.Metrics.AreaMM2) || !validOptionalFinite(expansion.Metrics.WorstMargin) {
		rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: obligation.Path + ".metrics", Message: "expansion metrics are non-finite or invalid"})
	}
	if expansion.Metrics.WorstMargin != nil && *expansion.Metrics.WorstMargin < 0 {
		rejections = append(rejections, ExpansionRejection{Code: CodeConstraintInvalid, Path: obligation.Path + ".metrics.worst_margin", Message: "expansion has a failed hard constraint margin"})
	}
	if len(expansion.OfferedPorts) != len(obligation.Ports) {
		rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: obligation.Path + ".ports", Message: "expansion must offer exactly one port for every obligation role"})
	}
	requiredByRole := map[string]RoleContract{}
	for _, port := range obligation.Ports {
		requiredByRole[port.Role] = port
	}
	selectionPorts := make([]RoleContract, 0, len(expansion.OfferedPorts))
	seenRoles := map[string]bool{}
	for _, offered := range expansion.OfferedPorts {
		if !validSemanticID(offered.Role) || seenRoles[offered.Role] {
			rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: obligation.Path + ".ports", Message: "expansion port role is invalid or duplicated"})
			continue
		}
		seenRoles[offered.Role] = true
		required, exists := requiredByRole[offered.Role]
		if !exists {
			rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: obligation.Path + ".ports." + offered.Role, Message: "expansion offers an undeclared role"})
			continue
		}
		report := SatisfiesPortRequirement(required.Contract, offered.Contract)
		for _, check := range report.Checks {
			if check.Status == ContractCheckReject {
				rejections = append(rejections, ExpansionRejection{Code: check.Code, Path: obligation.Path + ".ports." + offered.Role + "." + check.Path, Message: check.Message})
			}
		}
		offered.Anchor = required.Anchor
		offered.Contract.ID = required.Contract.ID
		selectionPorts = append(selectionPorts, offered)
	}
	for role := range requiredByRole {
		if !seenRoles[role] {
			rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: obligation.Path + ".ports." + role, Message: "expansion omits required role"})
		}
	}
	seenComponents := map[string]bool{}
	for index, component := range expansion.Components {
		path := fmt.Sprintf("%s.components[%d]", obligation.Path, index)
		if !validSemanticID(component.InstanceID) || component.CatalogID == "" || seenComponents[component.InstanceID] {
			rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: path, Message: "selected component requires a unique local instance id and catalog id"})
		}
		seenComponents[component.InstanceID] = true
		if !validEvidenceConfidence(component.Evidence) || confidenceRank(component.Evidence) < confidenceRank(EvidenceRuleInferred) {
			rejections = append(rejections, ExpansionRejection{Code: CodeEvidenceInsufficient, Path: path + ".evidence", Message: "selected component evidence is below rule_inferred"})
		}
	}
	for index, calculation := range expansion.Calculations {
		for _, issue := range ValidateCalculation(calculation) {
			rejections = append(rejections, ExpansionRejection{Code: issue.Code, Path: fmt.Sprintf("%s.calculations[%d].%s", obligation.Path, index, issue.Path), Message: issue.Message})
		}
		if !calculation.Pass {
			rejections = append(rejections, ExpansionRejection{Code: CodeToleranceFailed, Path: fmt.Sprintf("%s.calculations[%d]", obligation.Path, index), Message: "provider calculation fails a required tolerance or rating bound"})
		}
	}
	if len(expansion.Children) > policy.MaxUnresolvedObligations {
		rejections = append(rejections, ExpansionRejection{Code: CodeLimitExceeded, Path: obligation.Path + ".children", Message: "expansion exceeds child-obligation limit"})
	}
	seenChildren := map[string]bool{}
	for index, child := range expansion.Children {
		path := fmt.Sprintf("%s.children[%d]", obligation.Path, index)
		if !validSemanticID(child.ID) || !validSemanticID(child.Capability) || seenChildren[child.ID] {
			rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: path, Message: "child obligation requires a unique id and valid capability"})
		}
		seenChildren[child.ID] = true
		seenRoles := map[string]bool{}
		for _, port := range child.Ports {
			if !validSemanticID(port.Role) || seenRoles[port.Role] {
				rejections = append(rejections, ExpansionRejection{Code: CodeProviderExpansionInvalid, Path: path + ".ports", Message: "child port role is invalid or duplicated"})
			}
			seenRoles[port.Role] = true
			for _, check := range contractValidityChecks("contract", port.Contract) {
				if check.Status == ContractCheckReject {
					rejections = append(rejections, ExpansionRejection{Code: check.Code, Path: path + ".ports." + port.Role + "." + check.Path, Message: check.Message})
				}
			}
		}
		constraintValidator := requirementValidator{}
		constraintValidator.constraints("constraints", child.Constraints)
		for _, issue := range constraintValidator.issues {
			rejections = append(rejections, ExpansionRejection{Code: issue.Code, Path: path + "." + issue.Path, Message: issue.Message})
		}
	}
	slices.SortStableFunc(rejections, compareExpansionRejections)
	selection := FragmentSelection{
		ObligationPath: obligation.Path, Capability: obligation.Capability,
		ProviderID: descriptor.ID, ProviderRevision: descriptor.Revision, ExpansionID: expansion.ID,
		Ports: selectionPorts, Components: expansion.Components, Calculations: expansion.Calculations, Metrics: expansion.Metrics, Evidence: expansion.Evidence,
		DecisionClass: expansion.DecisionClass, RequiresUserChoice: expansion.RequiresUserChoice, Payload: expansion.Payload,
	}
	return normalizeFragmentSelection(selection), expansion.Children, rejections
}

func namespaceChildren(parent searchObligation, descriptor ProviderDescriptor, expansion ProviderExpansion, children []ChildObligation) []searchObligation {
	result := make([]searchObligation, 0, len(children))
	for _, child := range children {
		prefix := parent.Path + "/" + descriptor.ID + "/" + expansion.ID + "/" + child.ID
		ports := make([]RoleContract, len(child.Ports))
		for index, port := range child.Ports {
			if port.Anchor == "" {
				port.Anchor = prefix + ":" + port.Role
			} else {
				port.Anchor = prefix + ":" + canonicalIdentifier(port.Anchor)
			}
			ports[index] = port
		}
		result = append(result, searchObligation{Path: prefix, Capability: child.Capability, Ports: ports, Constraints: cloneConstraints(child.Constraints)})
	}
	slices.SortStableFunc(result, compareSearchObligations)
	return result
}

func normalizeProviderExpansion(expansion ProviderExpansion) ProviderExpansion {
	expansion.ID = canonicalIdentifier(expansion.ID)
	expansion.DecisionClass = canonicalIdentifier(expansion.DecisionClass)
	expansion.Evidence = normalizeContractEvidence(expansion.Evidence)
	for index := range expansion.OfferedPorts {
		expansion.OfferedPorts[index].Role = canonicalIdentifier(expansion.OfferedPorts[index].Role)
		expansion.OfferedPorts[index].Anchor = ""
		expansion.OfferedPorts[index].Contract = NormalizePortContract(expansion.OfferedPorts[index].Contract)
		expansion.OfferedPorts[index].Contract.ID = ""
	}
	slices.SortStableFunc(expansion.OfferedPorts, compareRoleContracts)
	for index := range expansion.Components {
		expansion.Components[index].InstanceID = canonicalIdentifier(expansion.Components[index].InstanceID)
		expansion.Components[index].CatalogID = strings.TrimSpace(expansion.Components[index].CatalogID)
		expansion.Components[index].VariantID = strings.TrimSpace(expansion.Components[index].VariantID)
	}
	slices.SortStableFunc(expansion.Components, func(left, right SelectedComponent) int {
		return strings.Compare(left.InstanceID, right.InstanceID)
	})
	sortCalculationEvidence(expansion.Calculations)
	for _, calculation := range expansion.Calculations {
		if expansion.Metrics.WorstMargin == nil || calculation.WorstMargin < *expansion.Metrics.WorstMargin {
			expansion.Metrics.WorstMargin = float64Pointer(calculation.WorstMargin)
		}
	}
	for index := range expansion.Children {
		child := &expansion.Children[index]
		child.ID = canonicalIdentifier(child.ID)
		child.Capability = canonicalIdentifier(child.Capability)
		for portIndex := range child.Ports {
			child.Ports[portIndex].Role = canonicalIdentifier(child.Ports[portIndex].Role)
			child.Ports[portIndex].Anchor = canonicalIdentifier(child.Ports[portIndex].Anchor)
			child.Ports[portIndex].Contract = NormalizePortContract(child.Ports[portIndex].Contract)
		}
		slices.SortStableFunc(child.Ports, compareRoleContracts)
		normalizeConstraints(child.Constraints)
	}
	slices.SortStableFunc(expansion.Children, func(left, right ChildObligation) int { return strings.Compare(left.ID, right.ID) })
	expansion.Payload = canonicalRawJSON(expansion.Payload)
	return expansion
}

func normalizeFragmentSelection(selection FragmentSelection) FragmentSelection {
	selection.Ports = append([]RoleContract(nil), selection.Ports...)
	slices.SortStableFunc(selection.Ports, compareRoleContracts)
	selection.Components = append([]SelectedComponent(nil), selection.Components...)
	slices.SortStableFunc(selection.Components, func(left, right SelectedComponent) int { return strings.Compare(left.InstanceID, right.InstanceID) })
	selection.Calculations = append([]CalculationEvidence(nil), selection.Calculations...)
	sortCalculationEvidence(selection.Calculations)
	selection.Evidence = normalizeContractEvidence(selection.Evidence)
	selection.Payload = canonicalRawJSON(selection.Payload)
	return selection
}

func normalizeSearchState(state searchState) searchState {
	for index := range state.Obligations {
		obligation := &state.Obligations[index]
		obligation.Capability = canonicalIdentifier(obligation.Capability)
		for portIndex := range obligation.Ports {
			obligation.Ports[portIndex].Role = canonicalIdentifier(obligation.Ports[portIndex].Role)
			obligation.Ports[portIndex].Contract = NormalizePortContract(obligation.Ports[portIndex].Contract)
		}
		slices.SortStableFunc(obligation.Ports, compareRoleContracts)
		normalizeConstraints(obligation.Constraints)
	}
	slices.SortStableFunc(state.Obligations, compareSearchObligations)
	for index := range state.Selections {
		state.Selections[index] = normalizeFragmentSelection(state.Selections[index])
	}
	slices.SortStableFunc(state.Selections, func(left, right FragmentSelection) int {
		return strings.Compare(left.ObligationPath, right.ObligationPath)
	})
	return state
}

func candidateFromState(state searchState) (CandidateResult, error) {
	selections := append([]FragmentSelection(nil), state.Selections...)
	slices.SortStableFunc(selections, func(left, right FragmentSelection) int {
		return strings.Compare(left.ObligationPath, right.ObligationPath)
	})
	if err := validateCandidateAnchors(selections); err != nil {
		return CandidateResult{}, err
	}
	encoded, err := json.Marshal(selections)
	if err != nil {
		return CandidateResult{}, err
	}
	sum := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(sum[:])
	score := CandidateScore{EvidenceRank: confidenceRank(EvidenceVerified), Fingerprint: fingerprint}
	var marginKnown, powerKnown, areaKnown bool
	for _, selection := range selections {
		score.UnprovenNonSafety += selection.Metrics.UnprovenNonSafety
		score.ComponentCount += len(selection.Components)
		score.FragmentCount++
		if rank := confidenceRank(selection.Evidence.Confidence); rank < score.EvidenceRank {
			score.EvidenceRank = rank
		}
		if selection.Metrics.WorstMargin != nil {
			if !marginKnown || *selection.Metrics.WorstMargin < *score.WorstMargin {
				score.WorstMargin = float64Pointer(*selection.Metrics.WorstMargin)
				marginKnown = true
			}
		}
		if selection.Metrics.QuiescentPowerW != nil {
			if !powerKnown {
				score.QuiescentPowerW = float64Pointer(0)
				powerKnown = true
			}
			*score.QuiescentPowerW += *selection.Metrics.QuiescentPowerW
		}
		if selection.Metrics.AreaMM2 != nil {
			if !areaKnown {
				score.AreaMM2 = float64Pointer(0)
				areaKnown = true
			}
			*score.AreaMM2 += *selection.Metrics.AreaMM2
		}
	}
	return CandidateResult{Fingerprint: fingerprint, Score: score, Selections: selections}, nil
}

func validateCandidateAnchors(selections []FragmentSelection) error {
	byAnchor := map[string][]PortContract{}
	for _, selection := range selections {
		for _, port := range selection.Ports {
			byAnchor[port.Anchor] = append(byAnchor[port.Anchor], port.Contract)
		}
	}
	anchors := make([]string, 0, len(byAnchor))
	for anchor := range byAnchor {
		anchors = append(anchors, anchor)
	}
	slices.Sort(anchors)
	for _, anchor := range anchors {
		ports := byAnchor[anchor]
		sourceCount := 0
		for _, port := range ports {
			if port.Direction == "source" {
				sourceCount++
			}
		}
		if sourceCount > 1 {
			return fmt.Errorf("anchor %s has %d direct sources", anchor, sourceCount)
		}
		for leftIndex := range ports {
			for rightIndex := leftIndex + 1; rightIndex < len(ports); rightIndex++ {
				left, right := ports[leftIndex], ports[rightIndex]
				if left.Direction == "sink" && right.Direction == "sink" {
					continue
				}
				if report := ConnectPorts(left, right); !report.Compatible {
					return fmt.Errorf("anchor %s has incompatible selected ports", anchor)
				}
			}
		}
	}
	return nil
}

func compareCandidateResults(left, right CandidateResult) int {
	return compareCandidateScores(left.Score, right.Score, true)
}

func buildSelectionRationale(selected CandidateResult, alternatives []CandidateResult) SelectionRationale {
	rationale := SelectionRationale{SelectedFingerprint: selected.Fingerprint}
	if len(alternatives) == 0 {
		rationale.Summary = "selected the only retained complete architecture candidate"
		return rationale
	}
	rationale.Summary = fmt.Sprintf("selected the highest-ranked candidate from %d retained complete architectures", len(alternatives)+1)
	for _, alternative := range alternatives {
		field, reason := firstScoreDifference(selected.Score, alternative.Score)
		rationale.Comparisons = append(rationale.Comparisons, AlternativeComparison{Fingerprint: alternative.Fingerprint, FirstScoreField: field, Reason: reason})
	}
	return rationale
}

func firstScoreDifference(selected, alternative CandidateScore) (string, string) {
	if selected.UnprovenNonSafety != alternative.UnprovenNonSafety {
		return "unproven_non_safety", fmt.Sprintf("selected has %d unproven non-safety obligations versus %d", selected.UnprovenNonSafety, alternative.UnprovenNonSafety)
	}
	if compareOptionalDescending(selected.WorstMargin, alternative.WorstMargin) != 0 {
		return "worst_margin", fmt.Sprintf("selected worst normalized margin %s ranks ahead of %s", optionalFloatText(selected.WorstMargin), optionalFloatText(alternative.WorstMargin))
	}
	if selected.EvidenceRank != alternative.EvidenceRank {
		return "evidence_rank", fmt.Sprintf("selected evidence rank %d exceeds %d", selected.EvidenceRank, alternative.EvidenceRank)
	}
	if selected.ComponentCount != alternative.ComponentCount {
		return "component_count", fmt.Sprintf("selected uses %d components versus %d", selected.ComponentCount, alternative.ComponentCount)
	}
	if selected.FragmentCount != alternative.FragmentCount {
		return "fragment_count", fmt.Sprintf("selected uses %d fragments versus %d", selected.FragmentCount, alternative.FragmentCount)
	}
	if compareOptionalAscending(selected.QuiescentPowerW, alternative.QuiescentPowerW) != 0 {
		return "quiescent_power_w", fmt.Sprintf("selected quiescent power %s ranks ahead of %s", optionalFloatText(selected.QuiescentPowerW), optionalFloatText(alternative.QuiescentPowerW))
	}
	if compareOptionalAscending(selected.AreaMM2, alternative.AreaMM2) != 0 {
		return "area_mm2", fmt.Sprintf("selected area %s ranks ahead of %s", optionalFloatText(selected.AreaMM2), optionalFloatText(alternative.AreaMM2))
	}
	return "fingerprint", "all substantive score fields tie; canonical architecture fingerprint is the deterministic tie-breaker"
}

func optionalFloatText(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%.12g", *value)
}

func compareCandidateScores(left, right CandidateScore, includeFingerprint bool) int {
	if order := left.UnprovenNonSafety - right.UnprovenNonSafety; order != 0 {
		return order
	}
	if order := compareOptionalDescending(left.WorstMargin, right.WorstMargin); order != 0 {
		return order
	}
	if order := right.EvidenceRank - left.EvidenceRank; order != 0 {
		return order
	}
	if order := left.ComponentCount - right.ComponentCount; order != 0 {
		return order
	}
	if order := left.FragmentCount - right.FragmentCount; order != 0 {
		return order
	}
	if order := compareOptionalAscending(left.QuiescentPowerW, right.QuiescentPowerW); order != 0 {
		return order
	}
	if order := compareOptionalAscending(left.AreaMM2, right.AreaMM2); order != 0 {
		return order
	}
	if includeFingerprint {
		return strings.Compare(left.Fingerprint, right.Fingerprint)
	}
	return 0
}

func scoresEquivalentBeforeFingerprint(left, right CandidateScore) bool {
	return compareCandidateScores(left, right, false) == 0
}

func candidatesRequireChoice(left, right CandidateResult) bool {
	leftClasses := choiceClasses(left)
	rightClasses := choiceClasses(right)
	if len(leftClasses) == 0 && len(rightClasses) == 0 {
		return false
	}
	return !slices.Equal(leftClasses, rightClasses)
}

func choiceClasses(candidate CandidateResult) []string {
	var classes []string
	for _, selection := range candidate.Selections {
		if selection.RequiresUserChoice {
			classes = append(classes, selection.DecisionClass)
		}
	}
	slices.Sort(classes)
	return slices.Compact(classes)
}

func effectiveSearchPolicy(input SearchPolicy) (SearchPolicy, []reports.Issue) {
	defaults := DefaultSearchPolicy()
	policy := input
	if policy.MaxExpandedStates == 0 {
		policy.MaxExpandedStates = defaults.MaxExpandedStates
	}
	if policy.MaxDepth == 0 {
		policy.MaxDepth = defaults.MaxDepth
	}
	if policy.MaxComponents == 0 {
		policy.MaxComponents = defaults.MaxComponents
	}
	if policy.MaxUnresolvedObligations == 0 {
		policy.MaxUnresolvedObligations = defaults.MaxUnresolvedObligations
	}
	if policy.MaxProviderExpansions == 0 {
		policy.MaxProviderExpansions = defaults.MaxProviderExpansions
	}
	if policy.MaxCompleteCandidates == 0 {
		policy.MaxCompleteCandidates = defaults.MaxCompleteCandidates
	}
	if policy.MaxRejectionSamples == 0 {
		policy.MaxRejectionSamples = defaults.MaxRejectionSamples
	}
	var issues []reports.Issue
	checks := []struct {
		path           string
		value, maximum int
	}{
		{"max_expanded_states", policy.MaxExpandedStates, defaults.MaxExpandedStates},
		{"max_depth", policy.MaxDepth, defaults.MaxDepth},
		{"max_components", policy.MaxComponents, defaults.MaxComponents},
		{"max_unresolved_obligations", policy.MaxUnresolvedObligations, defaults.MaxUnresolvedObligations},
		{"max_provider_expansions", policy.MaxProviderExpansions, defaults.MaxProviderExpansions},
		{"max_complete_candidates", policy.MaxCompleteCandidates, defaults.MaxCompleteCandidates},
		{"max_rejection_samples", policy.MaxRejectionSamples, defaults.MaxRejectionSamples},
	}
	for _, check := range checks {
		if check.value <= 0 || check.value > check.maximum {
			issues = append(issues, architectureIssue(CodeLimitExceeded, "policy."+check.path, fmt.Sprintf("value must be between 1 and %d", check.maximum)))
		}
	}
	return policy, issues
}

func (accumulator *searchAccumulator) reject(code reports.Code, path, providerID, expansionID, message string) {
	accumulator.rejections = append(accumulator.rejections, ExpansionRejection{Code: code, Path: path, ProviderID: providerID, ExpansionID: expansionID, Message: message})
}

func summarizeRejections(rejections []ExpansionRejection, sampleLimit int) []RejectionSummary {
	rejections = append([]ExpansionRejection(nil), rejections...)
	slices.SortStableFunc(rejections, compareExpansionRejections)
	byCode := map[reports.Code][]ExpansionRejection{}
	var codes []reports.Code
	for _, rejection := range rejections {
		if _, exists := byCode[rejection.Code]; !exists {
			codes = append(codes, rejection.Code)
		}
		byCode[rejection.Code] = append(byCode[rejection.Code], rejection)
	}
	slices.SortFunc(codes, func(left, right reports.Code) int { return strings.Compare(string(left), string(right)) })
	result := make([]RejectionSummary, 0, len(codes))
	for _, code := range codes {
		items := byCode[code]
		limit := minInt(len(items), sampleLimit)
		result = append(result, RejectionSummary{Code: code, Count: len(items), Samples: append([]ExpansionRejection(nil), items[:limit]...)})
	}
	return result
}

func compareExpansionRejections(left, right ExpansionRejection) int {
	if order := strings.Compare(string(left.Code), string(right.Code)); order != 0 {
		return order
	}
	if order := strings.Compare(left.Path, right.Path); order != 0 {
		return order
	}
	if order := strings.Compare(left.ProviderID, right.ProviderID); order != 0 {
		return order
	}
	if order := strings.Compare(left.ExpansionID, right.ExpansionID); order != 0 {
		return order
	}
	return strings.Compare(left.Message, right.Message)
}

func rejectionSummaryContains(summaries []RejectionSummary, code reports.Code) bool {
	for _, summary := range summaries {
		if summary.Code == code {
			return true
		}
	}
	return false
}

func compareSearchObligations(left, right searchObligation) int {
	if order := strings.Compare(left.Path, right.Path); order != 0 {
		return order
	}
	return strings.Compare(left.Capability, right.Capability)
}

func compareRoleContracts(left, right RoleContract) int {
	if order := strings.Compare(left.Role, right.Role); order != 0 {
		return order
	}
	return strings.Compare(left.Anchor, right.Anchor)
}

func searchStateKey(state searchState) string {
	encoded, _ := json.Marshal(state)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func cloneConstraints(constraints []Constraint) []Constraint {
	result := make([]Constraint, len(constraints))
	for index, constraint := range constraints {
		result[index] = constraint
		result[index].Value = append(json.RawMessage(nil), constraint.Value...)
		result[index].TolerancePercent = cloneFloat64(constraint.TolerancePercent)
	}
	normalizeConstraints(result)
	return result
}

func canonicalRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	return encoded
}

func externalAnchor(port string) string { return "external:" + canonicalIdentifier(port) }
func domainAnchor(domain string) string { return "domain:" + canonicalIdentifier(domain) }
func participantAnchor(participant, port string) string {
	return "participant:" + canonicalIdentifier(participant) + ":" + canonicalIdentifier(port)
}

func firstReferenceDomain(requirement Requirement) Domain {
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind == "reference" {
			return domain
		}
	}
	return Domain{}
}

func selectedComponentCount(selections []FragmentSelection) int {
	count := 0
	for _, selection := range selections {
		count += len(selection.Components)
	}
	return count
}

func selectedAreaMM2(selections []FragmentSelection) float64 {
	area := 0.0
	for _, selection := range selections {
		if selection.Metrics.AreaMM2 != nil {
			area += *selection.Metrics.AreaMM2
		}
	}
	return area
}

func validOptionalNonnegative(value *float64) bool {
	return value == nil || finiteInRange(*value, 0, 1e15)
}
func validOptionalFinite(value *float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0))
}

func compareOptionalDescending(left, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	if *left > *right {
		return -1
	}
	if *left < *right {
		return 1
	}
	return 0
}

func compareOptionalAscending(left, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

func sortIssues(issues []reports.Issue) {
	slices.SortStableFunc(issues, func(left, right reports.Issue) int {
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		if order := strings.Compare(string(left.Code), string(right.Code)); order != 0 {
			return order
		}
		return strings.Compare(left.Message, right.Message)
	})
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
