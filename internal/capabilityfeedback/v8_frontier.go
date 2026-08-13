package capabilityfeedback

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityroundsv8"
	"kicadai/internal/corpuspublication"
	"kicadai/internal/obligationanchor"
)

const discoveryObligationsSchemaV8 = "kicadai.closed-loop-open-set-discovery-obligations.v8"

// BuildV8DiscoveryRoundCases binds an authenticated public V8 corpus to a
// validated discovery report and expands its normalized causal gaps into
// immutable, obligation-anchored generation-zero root frontiers.
func BuildV8DiscoveryRoundCases(
	manifestSource []byte,
	obligationSource []byte,
	report AggregateReport,
	registry capabilityevaluation.ImpactRegistry,
) ([]capabilityroundsv8.Case, error) {
	var manifest corpuspublication.ManifestV8
	if err := decodeV8FrontierJSON(manifestSource, &manifest); err != nil {
		return nil, fmt.Errorf("decode V8 corpus manifest: %w", err)
	}
	var obligations corpuspublication.DiscoveryObligationsV8
	if err := decodeV8FrontierJSON(obligationSource, &obligations); err != nil {
		return nil, fmt.Errorf("decode V8 discovery obligations: %w", err)
	}
	if err := ValidateAggregateReport(report, registry); err != nil {
		return nil, fmt.Errorf("validate V8 discovery report: %w", err)
	}
	manifestDigest := sha256.Sum256(manifestSource)
	return buildV8DiscoveryRoundCases(
		manifest,
		hex.EncodeToString(manifestDigest[:]),
		obligations,
		report,
	)
}

func buildV8DiscoveryRoundCases(
	manifest corpuspublication.ManifestV8,
	manifestSHA256 string,
	obligations corpuspublication.DiscoveryObligationsV8,
	report AggregateReport,
) ([]capabilityroundsv8.Case, error) {
	policy := capabilityroundsv8.FrozenPolicy()
	if manifest.Schema != corpuspublication.ManifestSchemaV8 || manifest.Version != corpuspublication.ManifestVersionV8 ||
		manifest.DiscoveryCaseCount != policy.ExpectedDiscoveryCases || manifest.HeldOutCaseCount != policy.ExpectedDiscoveryCases ||
		len(manifest.Entries) != 2*policy.ExpectedDiscoveryCases {
		return nil, fmt.Errorf("invalid V8 corpus manifest shape")
	}
	if obligations.Schema != discoveryObligationsSchemaV8 || obligations.Version != corpuspublication.ManifestVersionV8 ||
		obligations.CorpusManifestSHA256 != manifestSHA256 {
		return nil, fmt.Errorf("V8 discovery obligations are not bound to the corpus manifest")
	}
	if report.CorpusRole != RoleDiscovery || report.CaseCount != policy.ExpectedDiscoveryCases ||
		len(report.Cases) != policy.ExpectedDiscoveryCases {
		return nil, fmt.Errorf("invalid V8 discovery report cohort")
	}

	discoveryEntries := make([]corpuspublication.EntryV8, 0, policy.ExpectedDiscoveryCases)
	entryByID := make(map[string]corpuspublication.EntryV8, policy.ExpectedDiscoveryCases)
	seenManifestIDs := make(map[string]bool, len(manifest.Entries))
	heldOutCount := 0
	for _, entry := range manifest.Entries {
		if entry.ID == "" || seenManifestIDs[entry.ID] {
			return nil, fmt.Errorf("duplicate or empty V8 manifest case identity")
		}
		seenManifestIDs[entry.ID] = true
		switch entry.Role {
		case string(RoleDiscovery):
			if entry.Sealed {
				return nil, fmt.Errorf("discovery case %q is unexpectedly sealed", entry.ID)
			}
			discoveryEntries = append(discoveryEntries, entry)
			entryByID[entry.ID] = entry
		case string(RoleHeldOut):
			if !entry.Sealed {
				return nil, fmt.Errorf("held-out case %q is unexpectedly public", entry.ID)
			}
			heldOutCount++
		default:
			return nil, fmt.Errorf("case %q has invalid corpus role %q", entry.ID, entry.Role)
		}
	}
	if len(discoveryEntries) != policy.ExpectedDiscoveryCases || heldOutCount != policy.ExpectedDiscoveryCases {
		return nil, fmt.Errorf("invalid V8 corpus role allocation")
	}

	obligationsByCase := make(map[string][]corpuspublication.ObligationV8, policy.ExpectedDiscoveryCases)
	seenAnchors := make(map[string]bool, len(obligations.Obligations))
	previousAnchor := ""
	for _, obligation := range obligations.Obligations {
		if obligation.Role != string(RoleDiscovery) || entryByID[obligation.CaseID].ID == "" {
			return nil, fmt.Errorf("obligation references a non-discovery V8 case")
		}
		derived, err := obligationanchor.Derive(obligationanchor.Input{
			CorpusManifestSHA256: manifestSHA256,
			Role:                 obligation.Role,
			CaseID:               obligation.CaseID,
			OperatingCaseID:      obligation.OperatingCaseID,
			AssertionID:          obligation.AssertionID,
			ObservationKind:      obligation.ObservationKind,
			ObservationID:        obligation.ObservationID,
			OutputID:             obligation.OutputID,
		})
		if err != nil || derived != obligation.Anchor || seenAnchors[obligation.Anchor] ||
			(previousAnchor != "" && obligation.Anchor <= previousAnchor) {
			return nil, fmt.Errorf("invalid, duplicate, or noncanonical V8 discovery obligation")
		}
		seenAnchors[obligation.Anchor] = true
		previousAnchor = obligation.Anchor
		obligationsByCase[obligation.CaseID] = append(obligationsByCase[obligation.CaseID], obligation)
	}
	for _, entry := range discoveryEntries {
		if len(obligationsByCase[entry.ID]) == 0 {
			return nil, fmt.Errorf("discovery case %q has no committed obligations", entry.ID)
		}
	}

	evidenceByID := make(map[string]CaseEvidence, policy.ExpectedDiscoveryCases)
	for _, evidence := range report.Cases {
		if _, exists := evidenceByID[evidence.Case.ID]; exists {
			return nil, fmt.Errorf("duplicate V8 discovery evidence for %q", evidence.Case.ID)
		}
		evidenceByID[evidence.Case.ID] = evidence
	}

	result := make([]capabilityroundsv8.Case, 0, policy.ExpectedDiscoveryCases)
	for _, entry := range discoveryEntries {
		evidence, exists := evidenceByID[entry.ID]
		feedbackDomain, domainErr := v8FeedbackDomain(entry.Domain)
		if domainErr != nil {
			return nil, fmt.Errorf("case %q: %w", entry.ID, domainErr)
		}
		if !exists || evidence.Case.Role != RoleDiscovery || evidence.Case.Domain != feedbackDomain ||
			string(evidence.Case.SafetyImpact) != entry.SafetyImpact || evidence.RequirementHash != entry.RequirementSHA256 {
			return nil, fmt.Errorf("discovery evidence does not match manifest case %q", entry.ID)
		}
		if !policy.ReportingDomains[entry.Domain] || !policy.CircuitRoles[entry.CircuitRole] {
			return nil, fmt.Errorf("case %q has metadata outside the frozen V8 policy", entry.ID)
		}
		frontier, satisfied, err := v8RootFrontier(obligationsByCase[entry.ID], evidence.Gaps)
		if err != nil {
			return nil, fmt.Errorf("case %q: %w", entry.ID, err)
		}
		if evidence.Outcome == OutcomePass && len(frontier) != 0 {
			return nil, fmt.Errorf("passing case %q has a root frontier", entry.ID)
		}
		if evidence.Outcome != OutcomePass && len(frontier) == 0 {
			return nil, fmt.Errorf("nonpassing case %q has no root frontier", entry.ID)
		}
		result = append(result, capabilityroundsv8.Case{
			ID: entry.ID, Role: entry.Role, ReportingDomain: entry.Domain,
			CircuitRole: entry.CircuitRole, SafetyImpact: entry.SafetyImpact,
			Outcome: string(evidence.Outcome), Frontier: frontier,
			SatisfiedObligations: satisfied,
		})
	}
	if len(evidenceByID) != len(result) {
		return nil, fmt.Errorf("V8 discovery report contains an unknown case")
	}
	return result, nil
}

// v8FeedbackDomain preserves the six-way V8 reporting-domain partition while
// adapting it to the legacy observation vocabulary used by capabilityfeedback.
func v8FeedbackDomain(reportingDomain string) (capabilityevaluation.Domain, error) {
	switch reportingDomain {
	case "analog_signal_path":
		return capabilityevaluation.DomainAnalog, nil
	case "power_energy_conversion":
		return capabilityevaluation.DomainPower, nil
	case "digital_control":
		return capabilityevaluation.DomainDigital, nil
	case "mixed_signal_data_conversion":
		return capabilityevaluation.DomainMixedSignal, nil
	case "sensing_instrumentation":
		return capabilityevaluation.DomainSensor, nil
	case "protection_power_integrity":
		return capabilityevaluation.DomainMCU, nil
	default:
		return "", fmt.Errorf("unknown V8 reporting domain %q", reportingDomain)
	}
}

func v8RootFrontier(obligations []corpuspublication.ObligationV8, gaps []Gap) ([]capabilityroundsv8.Gap, []string, error) {
	type keyedRoot struct {
		gap       capabilityroundsv8.Gap
		pathHash  string
		memberKey string
	}
	covered := make(map[string]bool, len(obligations))
	seenPaths := map[string]bool{}
	roots := []keyedRoot{}
	knownRequirements := map[string]bool{}
	knownCases := map[string]bool{}
	for _, obligation := range obligations {
		knownRequirements[obligation.AssertionID] = true
		knownCases[obligation.OperatingCaseID] = true
	}
	for _, gap := range gaps {
		category, err := v8GapCategory(gap.Scope)
		if err != nil {
			return nil, nil, err
		}
		requirementIDs := stringSetV8(gap.RequirementIDs)
		operatingCases := stringSetV8(gap.OperatingCases)
		if err := v8ValidateGapSelectors(knownRequirements, knownCases, requirementIDs, operatingCases); err != nil {
			return nil, nil, err
		}
		diagnostics := v8GapDiagnostics(gap)
		matched := 0
		for _, obligation := range obligations {
			if len(requirementIDs) > 0 && !requirementIDs[obligation.AssertionID] {
				continue
			}
			if len(operatingCases) > 0 && !operatingCases[obligation.OperatingCaseID] {
				continue
			}
			root := capabilityroundsv8.Gap{
				ObligationAnchor: obligation.Anchor,
				Path: []capabilityroundsv8.Leaf{{
					Stage: category, Category: category, Scope: string(gap.Scope),
					Capability: gap.Capability, Code: gap.Code,
					RequiredEvidence: slices.Clone(gap.RequiredEvidence),
				}},
				Diagnostics: slices.Clone(diagnostics),
			}
			pathHash, err := capabilityroundsv8.PathHash(root)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid V8 root path: %w", err)
			}
			if seenPaths[pathHash] {
				return nil, nil, fmt.Errorf("duplicate V8 root path %q", pathHash)
			}
			memberKey, err := capabilityroundsv8.MemberKey(root.Path[0])
			if err != nil {
				return nil, nil, fmt.Errorf("invalid V8 root member")
			}
			seenPaths[pathHash] = true
			covered[obligation.Anchor] = true
			roots = append(roots, keyedRoot{gap: root, pathHash: pathHash, memberKey: memberKey})
			matched++
		}
		if matched == 0 {
			return nil, nil, fmt.Errorf("causal gap does not map to a committed obligation")
		}
	}

	slices.SortFunc(roots, func(left, right keyedRoot) int {
		if order := cmp.Compare(left.gap.ObligationAnchor, right.gap.ObligationAnchor); order != 0 {
			return order
		}
		if order := cmp.Compare(left.pathHash, right.pathHash); order != 0 {
			return order
		}
		return cmp.Compare(left.memberKey, right.memberKey)
	})
	frontier := make([]capabilityroundsv8.Gap, len(roots))
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

func v8GapCategory(scope GapScope) (string, error) {
	switch scope {
	case ScopeTopology:
		return "topology", nil
	case ScopeComponent:
		return "component", nil
	case ScopeModel:
		return "model", nil
	case ScopeSimulation:
		return "simulation", nil
	case ScopePhysical, ScopeRouting:
		return "physical_design", nil
	case ScopeVerification:
		return "verification", nil
	default:
		return "", fmt.Errorf("unknown V8 root-gap scope %q", scope)
	}
}

func v8ValidateGapSelectors(knownRequirements, knownCases, requirementIDs, operatingCases map[string]bool) error {
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

func v8GapDiagnostics(gap Gap) []string {
	values := make(map[string]bool, len(gap.EvidenceHashes)+len(gap.DownstreamSymptoms))
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

func stringSetV8(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func decodeV8FrontierJSON(source []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
