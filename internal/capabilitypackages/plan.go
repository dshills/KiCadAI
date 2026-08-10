package capabilitypackages

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/capabilityexpansion"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/capabilitygate"
)

func BuildGenericPlan(candidate Candidate) (GenericPlan, error) {
	if candidate.Rank != 1 || candidate.Key != Tuple(string(candidate.Scope), candidate.Capability) || len(candidate.Members) == 0 {
		return GenericPlan{}, fmt.Errorf("generic package plan requires canonical rank one")
	}
	evidence := make([]capabilitygate.Evidence, 0, len(candidate.Members))
	gaps := make([]capabilitygate.Gap, 0, len(candidate.Members))
	requirements := []capabilitygate.Requirement{{
		Kind: scopeRequirementKind(candidate.Scope), ID: candidate.Capability,
		Description: "frozen rank-one reusable capability package", EvidenceIDs: []string{"closed_loop_package"},
	}}
	for _, domain := range candidate.Domains {
		requirements = append(requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementDomain, ID: domain,
			Description: "electrical domain affected by the frozen capability package", EvidenceIDs: []string{"closed_loop_package"},
		})
	}
	evidence = append(evidence, capabilitygate.Evidence{
		ID: "closed_loop_package", Kind: "closed_loop_open_set_baseline", Status: capabilitygate.EvidenceMissing,
		Stage: candidate.Members[0].Stage, Description: "content-addressed discovery package requires generic expansion evidence",
	})
	for _, member := range candidate.Members {
		if member.Key != Tuple(member.Stage, string(member.Scope), member.Capability, member.Code) || member.Scope != candidate.Scope || member.Capability != candidate.Capability {
			return GenericPlan{}, fmt.Errorf("rank-one package contains an invalid member")
		}
		gaps = append(gaps, capabilitygate.Gap{
			Code: member.Code, Kind: scopeRequirementKind(member.Scope), ID: member.Capability,
			Stage: member.Stage, Reason: "exact member of the frozen rank-one capability package",
			Action: strings.Join(candidate.RequiredEvidence, "; "),
		})
	}
	assessment, err := capabilitygate.Assess(capabilitygate.Input{
		Stage: "closed_loop_open_set_v5_baseline", Requirements: requirements, Evidence: evidence, Gaps: gaps,
		Risks: []capabilitygate.Risk{{
			Code: "CLOSED_LOOP_PACKAGE_QUARANTINE", Stage: candidate.Members[0].Stage,
			Summary:    "selected generic package remains experimental until exact reviewed promotion",
			Mitigation: "use source-backed capability expansion and all frozen promotion gates",
		}},
	})
	if err != nil {
		return GenericPlan{}, fmt.Errorf("build rank-one package assessment: %w", err)
	}
	expansion, err := capabilityexpansion.Plan(assessment)
	if err != nil {
		return GenericPlan{}, err
	}
	bindings := make([]MemberBinding, 0, len(candidate.Members))
	for _, member := range candidate.Members {
		needID := ""
		for _, need := range expansion.Needs {
			if need.CapabilityID == member.Capability && slices.Contains(need.GapCodes, member.Code) {
				needID = need.ID
				break
			}
		}
		if needID == "" {
			return GenericPlan{}, fmt.Errorf("expansion plan does not cover member %q", member.Key)
		}
		bindings = append(bindings, MemberBinding{MemberKey: member.Key, NeedID: needID})
	}
	plan := GenericPlan{Schema: PlanSchema, Version: PlanVersion, PackageKey: candidate.Key, Members: slices.Clone(candidate.Members), Bindings: bindings, ExpansionPlan: expansion}
	hash, err := hashPlan(plan)
	if err != nil {
		return GenericPlan{}, err
	}
	plan.Hash = hash
	return plan, ValidatePlan(plan)
}

func ValidatePlan(plan GenericPlan) error {
	if plan.Schema != PlanSchema || plan.Version != PlanVersion || plan.PackageKey == "" || len(plan.Members) == 0 || len(plan.Bindings) != len(plan.Members) {
		return fmt.Errorf("generic package plan header is invalid")
	}
	if err := capabilityexpansion.ValidatePlan(plan.ExpansionPlan); err != nil {
		return err
	}
	memberKeys := make([]string, len(plan.Members))
	bindingKeys := make([]string, len(plan.Bindings))
	members := make(map[string]Member, len(plan.Members))
	needs := make(map[string]capabilityexpansion.ExpansionNeed, len(plan.ExpansionPlan.Needs))
	for _, need := range plan.ExpansionPlan.Needs {
		needs[need.ID] = need
	}
	for index, member := range plan.Members {
		if member.Key != Tuple(member.Stage, string(member.Scope), member.Capability, member.Code) {
			return fmt.Errorf("generic package plan member is invalid")
		}
		if _, duplicate := members[member.Key]; duplicate {
			return fmt.Errorf("generic package plan contains duplicate members")
		}
		members[member.Key] = member
		memberKeys[index] = member.Key
	}
	seenBindings := map[string]bool{}
	for index, binding := range plan.Bindings {
		if binding.MemberKey == "" || binding.NeedID == "" {
			return fmt.Errorf("generic package plan binding is invalid")
		}
		if seenBindings[binding.MemberKey] {
			return fmt.Errorf("generic package plan contains duplicate bindings")
		}
		seenBindings[binding.MemberKey] = true
		member, memberFound := members[binding.MemberKey]
		need, needFound := needs[binding.NeedID]
		if !memberFound || !needFound || need.CapabilityID != member.Capability || !slices.Contains(need.GapCodes, member.Code) {
			return fmt.Errorf("generic package plan binding does not cover its member")
		}
		bindingKeys[index] = binding.MemberKey
	}
	slices.Sort(memberKeys)
	slices.Sort(bindingKeys)
	if !slices.Equal(memberKeys, bindingKeys) {
		return fmt.Errorf("generic package plan bindings do not exactly cover members")
	}
	expected, err := hashPlan(plan)
	if err != nil || plan.Hash != expected {
		return fmt.Errorf("generic package plan hash mismatch")
	}
	return nil
}

func scopeRequirementKind(scope capabilityfeedback.GapScope) capabilitygate.RequirementKind {
	switch scope {
	case capabilityfeedback.ScopeTopology:
		return capabilitygate.RequirementArchitecture
	case capabilityfeedback.ScopeComponent:
		return capabilitygate.RequirementComponent
	case capabilityfeedback.ScopeModel:
		return capabilitygate.RequirementModel
	case capabilityfeedback.ScopePhysical, capabilityfeedback.ScopeRouting:
		return capabilitygate.RequirementPhysical
	default:
		return capabilitygate.RequirementVerification
	}
}
