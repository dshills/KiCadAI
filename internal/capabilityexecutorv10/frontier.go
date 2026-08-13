package capabilityexecutorv10

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"

	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/capabilityroundsv10"
	"kicadai/internal/corpuspublication"
)

func buildRoundCase(input CaseInput, observation capabilityfeedback.CaseEvidence) (capabilityroundsv10.Case, error) {
	if observation.Case.ID != input.Entry.ID || observation.Case.Role != capabilityfeedback.RoleDiscovery ||
		string(observation.Case.SafetyImpact) != input.Entry.SafetyImpact {
		return capabilityroundsv10.Case{}, fmt.Errorf("capability observation differs from public case metadata")
	}
	frontier, satisfied, err := buildRootFrontier(input.Obligations, observation.Gaps)
	if err != nil {
		return capabilityroundsv10.Case{}, err
	}
	if observation.Outcome == capabilityfeedback.OutcomePass && len(frontier) != 0 {
		return capabilityroundsv10.Case{}, fmt.Errorf("passing case retains a root frontier")
	}
	if observation.Outcome != capabilityfeedback.OutcomePass && len(frontier) == 0 {
		return capabilityroundsv10.Case{}, fmt.Errorf("nonpassing case has no root frontier")
	}
	return capabilityroundsv10.Case{
		ID: input.Entry.ID, Role: input.Entry.Role, ReportingDomain: input.Entry.Domain,
		CircuitRole: input.Entry.CircuitRole, SafetyImpact: input.Entry.SafetyImpact,
		Outcome: string(observation.Outcome), Frontier: frontier, SatisfiedObligations: satisfied,
	}, nil
}

func buildRootFrontier(obligations []corpuspublication.ObligationV10, gaps []capabilityfeedback.Gap) ([]capabilityroundsv10.Gap, []string, error) {
	type keyedRoot struct {
		gap      capabilityroundsv10.Gap
		pathHash string
	}
	covered := map[string]bool{}
	seenPaths := map[string]bool{}
	knownRequirements := map[string]bool{}
	knownCases := map[string]bool{}
	for _, obligation := range obligations {
		knownRequirements[obligation.AssertionID] = true
		knownCases[obligation.OperatingCaseID] = true
	}
	roots := []keyedRoot{}
	for _, gap := range gaps {
		category, err := gapCategory(gap.Scope)
		if err != nil {
			return nil, nil, err
		}
		requirementIDs := stringSet(gap.RequirementIDs)
		operatingCases, err := operatingCaseSet(gap.OperatingCases, knownCases)
		if err != nil {
			return nil, nil, err
		}
		if err := validateGapSelectors(knownRequirements, knownCases, requirementIDs, operatingCases); err != nil {
			return nil, nil, err
		}
		diagnostics := gapDiagnostics(gap)
		if len(diagnostics) == 0 {
			return nil, nil, fmt.Errorf("causal gap has no diagnostic commitment")
		}
		matched := 0
		for _, obligation := range obligations {
			if len(requirementIDs) > 0 && !requirementIDs[obligation.AssertionID] {
				continue
			}
			if len(operatingCases) > 0 && !operatingCases[obligation.OperatingCaseID] {
				continue
			}
			root := capabilityroundsv10.Gap{
				ObligationAnchor: obligation.Anchor,
				Path: []capabilityroundsv10.Leaf{{
					Stage: category, Category: category, Scope: string(gap.Scope), Capability: gap.Capability,
					Code: gap.Code, RequiredEvidence: slices.Clone(gap.RequiredEvidence),
				}},
				Diagnostics: slices.Clone(diagnostics),
			}
			pathHash, err := capabilityroundsv10.PathHash(root)
			if err != nil || seenPaths[pathHash] {
				return nil, nil, fmt.Errorf("invalid or duplicate V10 root path")
			}
			seenPaths[pathHash] = true
			covered[obligation.Anchor] = true
			roots = append(roots, keyedRoot{gap: root, pathHash: pathHash})
			matched++
		}
		if matched == 0 {
			return nil, nil, fmt.Errorf("causal gap does not map to a committed obligation")
		}
	}
	slices.SortFunc(roots, func(left, right keyedRoot) int { return cmp.Compare(left.pathHash, right.pathHash) })
	frontier := make([]capabilityroundsv10.Gap, len(roots))
	for index := range roots {
		frontier[index] = roots[index].gap
	}
	satisfied := make([]string, 0, len(obligations)-len(covered))
	for _, obligation := range obligations {
		if !covered[obligation.Anchor] {
			satisfied = append(satisfied, obligation.Anchor)
		}
	}
	sort.Strings(satisfied)
	return frontier, satisfied, nil
}

func operatingCaseSet(values []string, known map[string]bool) (map[string]bool, error) {
	result := map[string]bool{}
	for _, value := range values {
		base := value
		if before, after, found := strings.Cut(value, "/"); found {
			if before == "" || after == "" || strings.Contains(after, "/") {
				return nil, fmt.Errorf("causal gap has malformed expanded operating case %q", value)
			}
			base = before
		}
		if !known[base] {
			return nil, fmt.Errorf("causal gap references unknown operating case %q", value)
		}
		result[base] = true
	}
	return result, nil
}

func gapCategory(scope capabilityfeedback.GapScope) (string, error) {
	switch scope {
	case capabilityfeedback.ScopeTopology:
		return "topology", nil
	case capabilityfeedback.ScopeComponent:
		return "component", nil
	case capabilityfeedback.ScopeModel:
		return "model", nil
	case capabilityfeedback.ScopeSimulation:
		return "simulation", nil
	case capabilityfeedback.ScopePhysical, capabilityfeedback.ScopeRouting:
		return "physical_design", nil
	case capabilityfeedback.ScopeVerification:
		return "verification", nil
	default:
		return "", fmt.Errorf("unknown V10 root-gap scope %q", scope)
	}
}

func validateGapSelectors(knownRequirements, knownCases, requirementIDs, operatingCases map[string]bool) error {
	for value := range requirementIDs {
		if !knownRequirements[value] {
			return fmt.Errorf("causal gap references unknown assertion %q", value)
		}
	}
	for value := range operatingCases {
		if !knownCases[value] {
			return fmt.Errorf("causal gap references unknown operating case %q", value)
		}
	}
	return nil
}

func gapDiagnostics(gap capabilityfeedback.Gap) []string {
	values := map[string]bool{}
	for _, value := range gap.EvidenceHashes {
		values["evidence_sha256:"+value] = true
	}
	for _, value := range gap.DownstreamSymptoms {
		values["downstream:"+value] = true
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
