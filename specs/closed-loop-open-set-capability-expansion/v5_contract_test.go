package closedloopopensetcontract

import (
	"bufio"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	v5StartingCommit                = "d8e98b4dee3212823525c5955e8e025bd0039d03"
	v5ImpactRegistryFileHash        = "c0229f216b3024627992327ddaa90f44df7f3f1f97412d05b22161284d15afa0"
	v5SynthesisPolicyFileHash       = "7e415c9a6b6d30142840c8bd56e598db70b1a2103bc663ccd73df762871cbb66"
	v5GapPolicyFileHash             = "ba73b2db190f48c70b31bc77b7689240df122f73b41e8b63624e540635139aa8"
	v5RetirementAuditHash           = "ff3ba5f1d1895b7c733aee02f99b41f21af70316b01414f5e673919eba85e1e8"
	v5MaximumSafetyWeight     int64 = 1<<63 - 1
)

type v5SelectionPolicy struct {
	Schema                      string                 `json:"schema"`
	Version                     int                    `json:"version"`
	PackageIdentityFields       []string               `json:"package_identity_fields"`
	MemberIdentityFields        []string               `json:"member_identity_fields"`
	IdentityEncoding            string                 `json:"identity_encoding"`
	AffectedCaseAggregation     string                 `json:"affected_case_aggregation"`
	RequiredEvidenceAggregation string                 `json:"required_evidence_aggregation"`
	Eligibility                 v5SelectionEligibility `json:"eligibility"`
	RankOnePlanAdmission        v5PlanAdmission        `json:"rank_one_plan_admission"`
	Ranking                     []string               `json:"ranking"`
	TieBehavior                 string                 `json:"tie_behavior"`
	IneligibleBehavior          string                 `json:"ineligible_behavior"`
	NoEligiblePackageBehavior   string                 `json:"no_eligible_package_behavior"`
	UnexecutableRankOneBehavior string                 `json:"unexecutable_rank_one_behavior"`
	HeldOutInfluence            string                 `json:"held_out_influence"`
	SelectedMemberGapRemoval    string                 `json:"selected_member_gap_removal"`
	HeldOutSuccessCausality     string                 `json:"held_out_success_causality"`
}

type v5SelectionEligibility struct {
	MinimumAffectedDiscoveryCases int `json:"minimum_affected_discovery_cases"`
	MinimumReportingDomains       int `json:"minimum_reporting_domains"`
}

type v5PlanAdmission struct {
	RequireCompleteGenericPlan bool `json:"require_complete_generic_plan"`
	RequireAllMembersPlanned   bool `json:"require_all_members_planned"`
}

type v5GapObservation struct {
	Role             string
	CaseID           string
	ReportingDomain  string
	SafetyWeight     int64
	Stage            string
	Scope            string
	Capability       string
	Code             string
	RequiredEvidence []string
}

type v5PackageCandidate struct {
	Key              string
	Scope            string
	Capability       string
	Cases            []string
	Domains          []string
	SafetyWeight     int64
	Members          []string
	RequiredEvidence []string
}

type v5GenericPlanEvidence struct {
	Executable bool
	Members    []string
}

type v5PackageAccumulator struct {
	Scope        string
	Capability   string
	Cases        map[string]v5CaseContribution
	Domains      map[string]bool
	Members      map[string]bool
	Requirements map[string]bool
}

type v5CaseContribution struct {
	Domain       string
	SafetyWeight int64
}

type v5MemberIdentity struct {
	Stage      string
	Scope      string
	Capability string
	Code       string
}

type v5GapIdentity struct {
	v5MemberIdentity
	RequiredEvidence []string
}

type v5CaseGaps struct {
	ID      string
	Outcome string
	Gaps    []v5GapIdentity
}

func TestVersionFiveContractIsFrozen(t *testing.T) {
	directory := v5ContractDirectory(t)
	manifest, err := os.Open(filepath.Join(directory, "V5_CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || seen[fields[1]] || filepath.Base(fields[1]) != fields[1] {
			t.Fatalf("invalid V5 contract entry %q", scanner.Text())
		}
		if got := v5FileSHA256(t, filepath.Join(directory, fields[1])); got != fields[0] {
			t.Fatalf("%s hash = %s, want frozen %s", fields[1], got, fields[0])
		}
		seen[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	requiredFiles := []string{
		"V5_SPEC_ADDENDUM.md", "V5_PLAN.md", "V5_CORPUS_RULES.md", "V5_BASELINE_PROTOCOL.md",
		"V5_SELECTION_POLICY.json", "V5_IMPLEMENTATION.sha256", "v5_contract_test.go",
		"V4_IMPACT_REGISTRY.json", "V4_SYNTHESIS_POLICY.json", "V4_GAP_TRANSITION_POLICY.json",
		"V4_VALIDATION_AUDIT.md",
	}
	for _, required := range requiredFiles {
		if !seen[required] {
			t.Fatalf("V5 frozen contract omits %s", required)
		}
	}
	if len(seen) != len(requiredFiles) {
		t.Fatalf("V5 frozen contract contains %d entries, want exactly %d", len(seen), len(requiredFiles))
	}
}

func TestVersionFiveImplementationHashesAreFrozen(t *testing.T) {
	directory := v5ContractDirectory(t)
	repository := filepath.Clean(filepath.Join(directory, "..", ".."))
	manifest, err := os.Open(filepath.Join(directory, "V5_IMPLEMENTATION.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	want := map[string]bool{
		"internal/opentopologysynthesis/realizability.go":                     false,
		"internal/capabilityfeedback/observe.go":                              false,
		"internal/capabilityfeedback/evaluate.go":                             false,
		"specs/behavioral-contract-feasibility-realizability/CONTRACT.sha256": false,
	}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || filepath.IsAbs(fields[1]) || strings.HasPrefix(filepath.Clean(fields[1]), "..") {
			t.Fatalf("invalid V5 implementation entry %q", scanner.Text())
		}
		seen, exists := want[fields[1]]
		if !exists || seen {
			t.Fatalf("unexpected or duplicate V5 implementation entry %q", fields[1])
		}
		if got := v5FileSHA256(t, filepath.Join(repository, filepath.Clean(fields[1]))); got != fields[0] {
			t.Fatalf("%s hash = %s, want frozen %s", fields[1], got, fields[0])
		}
		want[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for path, seen := range want {
		if !seen {
			t.Fatalf("V5 implementation manifest omits %s", path)
		}
	}
}

func TestVersionFiveStartingStateAndRetirementAreFrozen(t *testing.T) {
	directory := v5ContractDirectory(t)
	for _, name := range []string{"V5_SPEC_ADDENDUM.md", "V5_BASELINE_PROTOCOL.md"} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), v5StartingCommit) {
			t.Fatalf("%s does not bind V5 starting commit", name)
		}
	}
	for name, want := range map[string]string{
		"V4_IMPACT_REGISTRY.json":       v5ImpactRegistryFileHash,
		"V4_SYNTHESIS_POLICY.json":      v5SynthesisPolicyFileHash,
		"V4_GAP_TRANSITION_POLICY.json": v5GapPolicyFileHash,
		"V4_VALIDATION_AUDIT.md":        v5RetirementAuditHash,
	} {
		if got := v5FileSHA256(t, filepath.Join(directory, name)); got != want {
			t.Fatalf("%s hash = %s, want %s", name, got, want)
		}
	}
	audit, err := os.ReadFile(filepath.Join(directory, "V4_VALIDATION_AUDIT.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"- Status: failed", "- Held-out corpus consumed: yes", "- Final updater: retired"} {
		if !strings.Contains(string(audit), required) {
			t.Fatalf("V4 retirement audit omits %q", required)
		}
	}
	for _, forbidden := range []string{"V4_FINAL_REPORT.json", "V4_FINAL_COMPARISON.json", "V4_PROMOTION_MATRIX.json", "V4_HELD_OUT_FINAL_SEAL.json"} {
		if _, err := os.Stat(filepath.Join(directory, forbidden)); !os.IsNotExist(err) {
			t.Fatalf("retired V4 unexpectedly contains %s", forbidden)
		}
	}
}

func TestVersionFiveSelectionPolicyIsExact(t *testing.T) {
	directory := v5ContractDirectory(t)
	var policy v5SelectionPolicy
	v5DecodeStrictFile(t, filepath.Join(directory, "V5_SELECTION_POLICY.json"), &policy)
	want := v5SelectionPolicy{
		Schema: "kicadai.closed-loop-open-set-selection-policy.v5", Version: 5,
		PackageIdentityFields:   []string{"scope", "capability"},
		MemberIdentityFields:    []string{"stage", "scope", "capability", "code"},
		IdentityEncoding:        "utf8-decimal-byte-length-colon",
		AffectedCaseAggregation: "sorted-unique-union", RequiredEvidenceAggregation: "sorted-unique-union",
		Eligibility:          v5SelectionEligibility{MinimumAffectedDiscoveryCases: 2, MinimumReportingDomains: 2},
		RankOnePlanAdmission: v5PlanAdmission{RequireCompleteGenericPlan: true, RequireAllMembersPlanned: true},
		Ranking:              []string{"affected_discovery_cases_desc", "reporting_domains_desc", "safety_impact_weight_desc", "member_cluster_count_asc", "required_evidence_count_asc", "canonical_package_key_asc"},
		TieBehavior:          "ranking_tuple_then_canonical_package_key", IneligibleBehavior: "exclude_before_ranking",
		NoEligiblePackageBehavior: "fail_closed", UnexecutableRankOneBehavior: "fail_closed_no_fallback",
		HeldOutInfluence: "prohibited", SelectedMemberGapRemoval: "exact_member_identity_only",
		HeldOutSuccessCausality: "at_least_one_new_pass_with_matching_baseline_package",
	}
	if !reflect.DeepEqual(policy, want) {
		t.Fatalf("V5 selection policy drifted: %#v", policy)
	}
}

func TestVersionFivePackageRankingIsDeterministicAndDiscoveryOnly(t *testing.T) {
	var policy v5SelectionPolicy
	v5DecodeStrictFile(t, filepath.Join(v5ContractDirectory(t), "V5_SELECTION_POLICY.json"), &policy)
	observations := []v5GapObservation{
		{Role: "discovery", CaseID: "a1", ReportingDomain: "analog", SafetyWeight: 1, Stage: "s1", Scope: "topology", Capability: "cap_a", Code: "x", RequiredEvidence: []string{"erc"}},
		{Role: "discovery", CaseID: "a2", ReportingDomain: "power", SafetyWeight: 1, Stage: "s2", Scope: "topology", Capability: "cap_a", Code: "y", RequiredEvidence: []string{"drc"}},
		{Role: "discovery", CaseID: "a3", ReportingDomain: "analog", SafetyWeight: 1, Stage: "s1", Scope: "topology", Capability: "cap_a", Code: "x", RequiredEvidence: []string{"erc"}},
		{Role: "discovery", CaseID: "b1", ReportingDomain: "sensor", SafetyWeight: 9, Stage: "s", Scope: "model", Capability: "cap_b", Code: "x"},
		{Role: "discovery", CaseID: "b2", ReportingDomain: "mixed-signal", SafetyWeight: 9, Stage: "s", Scope: "model", Capability: "cap_b", Code: "x"},
		{Role: "discovery", CaseID: "c1", ReportingDomain: "power", SafetyWeight: 20, Stage: "s", Scope: "simulation", Capability: "cap_c", Code: "x"},
		{Role: "discovery", CaseID: "c2", ReportingDomain: "power", SafetyWeight: 20, Stage: "s", Scope: "simulation", Capability: "cap_c", Code: "y"},
		{Role: "held_out", CaseID: "hidden", ReportingDomain: "power", SafetyWeight: 1000, Stage: "s", Scope: "topology", Capability: "cap_z", Code: "x"},
	}
	plans := map[string]v5GenericPlanEvidence{
		v5Tuple("topology", "cap_a"):   {Executable: true, Members: []string{v5Tuple("s1", "topology", "cap_a", "x"), v5Tuple("s2", "topology", "cap_a", "y")}},
		v5Tuple("model", "cap_b"):      {Executable: true, Members: []string{v5Tuple("s", "model", "cap_b", "x")}},
		v5Tuple("simulation", "cap_c"): {Executable: true, Members: []string{v5Tuple("s", "simulation", "cap_c", "x"), v5Tuple("s", "simulation", "cap_c", "y")}},
	}
	first, err := v5BuildPackages(observations, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].Capability != "cap_a" || first[1].Capability != "cap_b" {
		t.Fatalf("V5 package ranking = %#v", first)
	}
	selected, err := v5SelectRankOne(first, policy, plans)
	if err != nil || selected.Capability != "cap_a" {
		t.Fatalf("V5 rank-one plan admission = %#v, %v", selected, err)
	}
	mutated := append(slices.Clone(observations), v5GapObservation{Role: "held_out", CaseID: "hidden2", ReportingDomain: "analog", SafetyWeight: 9999, Stage: "s", Scope: "topology", Capability: "cap_z", Code: "y"})
	second, err := v5BuildPackages(mutated, policy)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("held-out mutation changed V5 discovery ranking: %v %#v", err, second)
	}
	reversed := slices.Clone(observations)
	slices.Reverse(reversed)
	third, err := v5BuildPackages(reversed, policy)
	if err != nil || !reflect.DeepEqual(first, third) {
		t.Fatalf("input order changed V5 package ranking: %v %#v", err, third)
	}
	onlySingleDomainObservations := []v5GapObservation{
		{Role: "discovery", CaseID: "one", ReportingDomain: "power", Stage: "s", Scope: "topology", Capability: "cap", Code: "x"},
		{Role: "discovery", CaseID: "two", ReportingDomain: "power", Stage: "s", Scope: "topology", Capability: "cap", Code: "y"},
	}
	onlySingleDomain, err := v5BuildPackages(onlySingleDomainObservations, policy)
	if err != nil || len(onlySingleDomain) != 0 {
		t.Fatalf("single-domain V5 package crossed eligibility floor: %v %#v", err, onlySingleDomain)
	}
	missingPlan := mapsWithoutV5Plan(plans, v5Tuple("topology", "cap_a"))
	if selected, err := v5SelectRankOne(first, policy, missingPlan); err == nil {
		t.Fatalf("V5 fell back from unplanned rank one to %#v", selected)
	}
	partialPlans := cloneV5Plans(plans)
	partialPlans[v5Tuple("topology", "cap_a")] = v5GenericPlanEvidence{Executable: true, Members: []string{v5Tuple("s1", "topology", "cap_a", "x")}}
	if selected, err := v5SelectRankOne(first, policy, partialPlans); err == nil {
		t.Fatalf("V5 admitted partially planned rank one %#v", selected)
	}
	if selected, err := v5SelectRankOne(nil, policy, plans); err == nil {
		t.Fatalf("V5 selected rank one from an empty eligible set: %#v", selected)
	}
	negative := slices.Clone(observations)
	negative[0].SafetyWeight = -1
	if _, err := v5BuildPackages(negative, policy); err == nil {
		t.Fatal("V5 package ranking accepted a negative safety impact weight")
	}
	incomplete := slices.Clone(observations)
	incomplete[0].Stage = ""
	if _, err := v5BuildPackages(incomplete, policy); err == nil {
		t.Fatal("V5 package ranking accepted an incomplete member identity")
	}
	overflowObservations := []v5GapObservation{
		{Role: "discovery", CaseID: "max", ReportingDomain: "analog", SafetyWeight: v5MaximumSafetyWeight, Stage: "s", Scope: "topology", Capability: "overflow", Code: "x"},
		{Role: "discovery", CaseID: "one", ReportingDomain: "power", SafetyWeight: 1, Stage: "s", Scope: "topology", Capability: "overflow", Code: "x"},
	}
	if _, err := v5BuildPackages(overflowObservations, policy); err == nil {
		t.Fatal("V5 package ranking accepted safety impact overflow")
	}
	mutatedPolicy := policy
	mutatedPolicy.Ranking = []string{"safety_impact_weight_desc", "affected_discovery_cases_desc", "reporting_domains_desc", "member_cluster_count_asc", "required_evidence_count_asc", "canonical_package_key_asc"}
	mutatedRanking, err := v5BuildPackages(observations, mutatedPolicy)
	if err != nil || len(mutatedRanking) != 2 || mutatedRanking[0].Capability != "cap_b" {
		t.Fatalf("V5 package ranking did not follow configured clause order: %v %#v", err, mutatedRanking)
	}
	mutatedPolicy = policy
	mutatedPolicy.Ranking = append(slices.Clone(policy.Ranking), "unknown_clause")
	if _, err := v5BuildPackages(observations, mutatedPolicy); err == nil {
		t.Fatal("V5 package ranking accepted an unknown ranking clause")
	}
	mutatedPolicy = policy
	mutatedPolicy.IdentityEncoding = "delimiter-only"
	if _, err := v5BuildPackages(observations, mutatedPolicy); err == nil {
		t.Fatal("V5 package ranking accepted identity-encoding drift")
	}
}

func TestVersionFiveIdentityEncodingIsInjectiveAndByteLengthBased(t *testing.T) {
	if v5Tuple("a", "bc") == v5Tuple("ab", "c") {
		t.Fatal("V5 length-delimited identities are ambiguous")
	}
	if got := v5Tuple("é"); got != "2:é" {
		t.Fatalf("V5 identity uses non-byte length: %q", got)
	}
	if got := v5Tuple("", "", ""); got != "0:0:0:" || got == v5Tuple("", "") {
		t.Fatalf("V5 empty-component identity is ambiguous: %q", got)
	}
}

func TestVersionFiveSelectedMembersBoundGapRemoval(t *testing.T) {
	selected := []v5MemberIdentity{{Stage: "topology_search", Scope: "topology", Capability: "complete_topology", Code: "SELECTED"}}
	selectedGap := v5GapIdentity{v5MemberIdentity: selected[0]}
	remaining := v5GapIdentity{v5MemberIdentity: v5MemberIdentity{Stage: "simulation", Scope: "simulation", Capability: "transient", Code: "REMAINS"}, RequiredEvidence: []string{"erc", "drc"}}
	caseWith := func(gaps ...v5GapIdentity) []v5CaseGaps {
		return []v5CaseGaps{{ID: "case", Outcome: "unsupported", Gaps: gaps}}
	}
	if !v5GapsPreserved(caseWith(selectedGap, remaining), caseWith(remaining), selected) {
		t.Fatal("exact selected V5 member removal was rejected")
	}
	if !v5GapsPreserved(caseWith(remaining), caseWith(remaining, v5GapIdentity{v5MemberIdentity: v5MemberIdentity{Stage: "later", Scope: "verification", Capability: "proof", Code: "NEW"}}), selected) {
		t.Fatal("monotonic V5 final gap superset was rejected")
	}
	sameCapabilityDifferentCode := v5GapIdentity{v5MemberIdentity: v5MemberIdentity{Stage: selected[0].Stage, Scope: selected[0].Scope, Capability: selected[0].Capability, Code: "OTHER"}}
	if v5GapsPreserved(caseWith(sameCapabilityDifferentCode), caseWith(), selected) {
		t.Fatal("V5 removed a nonselected member sharing only capability")
	}
	mutatedEvidence := remaining
	mutatedEvidence.RequiredEvidence = []string{"erc", "connectivity"}
	if v5GapsPreserved(caseWith(remaining), caseWith(mutatedEvidence), selected) {
		t.Fatal("V5 accepted required-evidence mutation as preservation")
	}
	duplicate := append(caseWith(remaining), caseWith(remaining)...)
	if v5GapsPreserved(duplicate, caseWith(remaining), selected) || v5GapsPreserved(caseWith(remaining), nil, selected) {
		t.Fatal("V5 accepted duplicate or mismatched case sets")
	}
	if v5GapsPreserved([]v5CaseGaps{{ID: "case", Outcome: "pass"}}, caseWith(remaining), selected) {
		t.Fatal("V5 accepted baseline pass regression")
	}
	if v5GapsPreserved([]v5CaseGaps{{ID: "case", Outcome: "unsafe"}}, []v5CaseGaps{{ID: "case", Outcome: "pass"}}, selected) {
		t.Fatal("V5 accepted unsafe-to-pass transition")
	}
}

func v5BuildPackages(observations []v5GapObservation, policy v5SelectionPolicy) ([]v5PackageCandidate, error) {
	if err := v5ValidateFixedPolicy(policy); err != nil {
		return nil, err
	}
	if err := v5ValidateIdentityFields(policy.PackageIdentityFields, []string{"scope", "capability"}); err != nil {
		return nil, err
	}
	if err := v5ValidateIdentityFields(policy.MemberIdentityFields, []string{"stage", "scope", "capability", "code"}); err != nil {
		return nil, err
	}
	if err := v5ValidateRanking(policy.Ranking); err != nil {
		return nil, err
	}
	groups := map[string]*v5PackageAccumulator{}
	for _, observation := range observations {
		if observation.Role != "discovery" {
			continue
		}
		if observation.CaseID == "" || observation.ReportingDomain == "" || observation.Stage == "" || observation.Scope == "" || observation.Capability == "" || observation.Code == "" || observation.SafetyWeight < 0 {
			return nil, fmt.Errorf("incomplete discovery observation")
		}
		key, err := v5ObservationIdentity(observation, policy.PackageIdentityFields)
		if err != nil {
			return nil, err
		}
		group := groups[key]
		if group == nil {
			group = &v5PackageAccumulator{Scope: observation.Scope, Capability: observation.Capability, Cases: map[string]v5CaseContribution{}, Domains: map[string]bool{}, Members: map[string]bool{}, Requirements: map[string]bool{}}
			groups[key] = group
		}
		contribution := v5CaseContribution{Domain: observation.ReportingDomain, SafetyWeight: observation.SafetyWeight}
		if existing, found := group.Cases[observation.CaseID]; found && existing != contribution {
			return nil, fmt.Errorf("inconsistent case contribution")
		}
		group.Cases[observation.CaseID] = contribution
		group.Domains[observation.ReportingDomain] = true
		member, err := v5ObservationIdentity(observation, policy.MemberIdentityFields)
		if err != nil {
			return nil, err
		}
		group.Members[member] = true
		for _, evidence := range observation.RequiredEvidence {
			group.Requirements[evidence] = true
		}
	}

	candidates := make([]v5PackageCandidate, 0, len(groups))
	for key, group := range groups {
		if len(group.Cases) < policy.Eligibility.MinimumAffectedDiscoveryCases || len(group.Domains) < policy.Eligibility.MinimumReportingDomains {
			continue
		}
		candidate := v5PackageCandidate{Key: key, Scope: group.Scope, Capability: group.Capability}
		for id, contribution := range group.Cases {
			candidate.Cases = append(candidate.Cases, id)
			if candidate.SafetyWeight > v5MaximumSafetyWeight-contribution.SafetyWeight {
				return nil, fmt.Errorf("safety impact weight overflow")
			}
			candidate.SafetyWeight += contribution.SafetyWeight
		}
		for domain := range group.Domains {
			candidate.Domains = append(candidate.Domains, domain)
		}
		for member := range group.Members {
			candidate.Members = append(candidate.Members, member)
		}
		for evidence := range group.Requirements {
			candidate.RequiredEvidence = append(candidate.RequiredEvidence, evidence)
		}
		sort.Strings(candidate.Cases)
		sort.Strings(candidate.Domains)
		sort.Strings(candidate.Members)
		sort.Strings(candidate.RequiredEvidence)
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, func(left, right v5PackageCandidate) int {
		return v5ComparePackages(left, right, policy.Ranking)
	})
	return candidates, nil
}

func v5SelectRankOne(candidates []v5PackageCandidate, policy v5SelectionPolicy, plans map[string]v5GenericPlanEvidence) (v5PackageCandidate, error) {
	if err := v5ValidateFixedPolicy(policy); err != nil {
		return v5PackageCandidate{}, err
	}
	if len(candidates) == 0 {
		return v5PackageCandidate{}, fmt.Errorf("no eligible package: %s", policy.NoEligiblePackageBehavior)
	}
	rankOne := candidates[0]
	planEvidence, planned := plans[rankOne.Key]
	if policy.RankOnePlanAdmission.RequireCompleteGenericPlan && (!planned || !planEvidence.Executable) {
		return v5PackageCandidate{}, fmt.Errorf("rank one is not executable: %s", policy.UnexecutableRankOneBehavior)
	}
	planMembers := slices.Clone(planEvidence.Members)
	sort.Strings(planMembers)
	planMembers = slices.Compact(planMembers)
	if policy.RankOnePlanAdmission.RequireAllMembersPlanned && !slices.Equal(planMembers, rankOne.Members) {
		return v5PackageCandidate{}, fmt.Errorf("rank one plan does not cover every member: %s", policy.UnexecutableRankOneBehavior)
	}
	return rankOne, nil
}

func v5ValidateFixedPolicy(policy v5SelectionPolicy) error {
	checks := []struct {
		name  string
		valid bool
	}{
		{name: "identity encoding", valid: policy.IdentityEncoding == "utf8-decimal-byte-length-colon"},
		{name: "affected-case aggregation", valid: policy.AffectedCaseAggregation == "sorted-unique-union"},
		{name: "required-evidence aggregation", valid: policy.RequiredEvidenceAggregation == "sorted-unique-union"},
		{name: "tie behavior", valid: policy.TieBehavior == "ranking_tuple_then_canonical_package_key"},
		{name: "ineligible behavior", valid: policy.IneligibleBehavior == "exclude_before_ranking"},
		{name: "no-eligible behavior", valid: policy.NoEligiblePackageBehavior == "fail_closed"},
		{name: "unexecutable rank-one behavior", valid: policy.UnexecutableRankOneBehavior == "fail_closed_no_fallback"},
		{name: "held-out influence", valid: policy.HeldOutInfluence == "prohibited"},
	}
	for _, check := range checks {
		if !check.valid {
			return fmt.Errorf("%s policy is unsupported", check.name)
		}
	}
	return nil
}

func v5ValidateIdentityFields(actual, required []string) error {
	if len(actual) != len(required) {
		return fmt.Errorf("identity field arity is invalid")
	}
	seen := map[string]bool{}
	for _, field := range actual {
		if seen[field] || !slices.Contains(required, field) {
			return fmt.Errorf("identity field %q is invalid", field)
		}
		seen[field] = true
	}
	return nil
}

func v5ObservationIdentity(observation v5GapObservation, fields []string) (string, error) {
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "stage":
			values = append(values, observation.Stage)
		case "scope":
			values = append(values, observation.Scope)
		case "capability":
			values = append(values, observation.Capability)
		case "code":
			values = append(values, observation.Code)
		default:
			return "", fmt.Errorf("identity field %q is unsupported", field)
		}
	}
	return v5Tuple(values...), nil
}

func v5ValidateRanking(ranking []string) error {
	known := []string{"affected_discovery_cases_desc", "reporting_domains_desc", "safety_impact_weight_desc", "member_cluster_count_asc", "required_evidence_count_asc", "canonical_package_key_asc"}
	if len(ranking) != len(known) {
		return fmt.Errorf("ranking clause count is invalid")
	}
	seen := map[string]bool{}
	for _, clause := range ranking {
		if seen[clause] || !slices.Contains(known, clause) {
			return fmt.Errorf("ranking clause %q is invalid", clause)
		}
		seen[clause] = true
	}
	return nil
}

func v5ComparePackages(left, right v5PackageCandidate, ranking []string) int {
	for _, clause := range ranking {
		var order int
		switch clause {
		case "affected_discovery_cases_desc":
			order = cmp.Compare(len(right.Cases), len(left.Cases))
		case "reporting_domains_desc":
			order = cmp.Compare(len(right.Domains), len(left.Domains))
		case "safety_impact_weight_desc":
			order = cmp.Compare(right.SafetyWeight, left.SafetyWeight)
		case "member_cluster_count_asc":
			order = cmp.Compare(len(left.Members), len(right.Members))
		case "required_evidence_count_asc":
			order = cmp.Compare(len(left.RequiredEvidence), len(right.RequiredEvidence))
		case "canonical_package_key_asc":
			order = cmp.Compare(left.Key, right.Key)
		}
		if order != 0 {
			return order
		}
	}
	return 0
}

func cloneV5Plans(source map[string]v5GenericPlanEvidence) map[string]v5GenericPlanEvidence {
	result := make(map[string]v5GenericPlanEvidence, len(source))
	for key, value := range source {
		value.Members = slices.Clone(value.Members)
		result[key] = value
	}
	return result
}

func mapsWithoutV5Plan(source map[string]v5GenericPlanEvidence, omitted string) map[string]v5GenericPlanEvidence {
	result := cloneV5Plans(source)
	delete(result, omitted)
	return result
}

func v5GapsPreserved(before, after []v5CaseGaps, selected []v5MemberIdentity) bool {
	beforeByID, beforeOK := v5UniqueCases(before)
	afterByID, afterOK := v5UniqueCases(after)
	if !beforeOK || !afterOK || len(beforeByID) != len(afterByID) {
		return false
	}
	selectedSet := map[string]bool{}
	for _, member := range selected {
		selectedSet[v5Tuple(member.Stage, member.Scope, member.Capability, member.Code)] = true
	}
	for id, current := range beforeByID {
		next, found := afterByID[id]
		if !found {
			return false
		}
		if current.Outcome == "pass" && next.Outcome != "pass" {
			return false
		}
		if current.Outcome == "unsafe" && next.Outcome == "pass" {
			return false
		}
		if next.Outcome == "pass" {
			continue
		}
		final := map[string]bool{}
		for _, gap := range next.Gaps {
			final[v5FullGapIdentity(gap)] = true
		}
		for _, gap := range current.Gaps {
			member := v5Tuple(gap.Stage, gap.Scope, gap.Capability, gap.Code)
			if selectedSet[member] {
				continue
			}
			if !final[v5FullGapIdentity(gap)] {
				return false
			}
		}
	}
	return true
}

func v5UniqueCases(cases []v5CaseGaps) (map[string]v5CaseGaps, bool) {
	result := make(map[string]v5CaseGaps, len(cases))
	for _, current := range cases {
		if current.ID == "" {
			return nil, false
		}
		if _, found := result[current.ID]; found {
			return nil, false
		}
		result[current.ID] = current
	}
	return result, true
}

func v5FullGapIdentity(gap v5GapIdentity) string {
	required := slices.Clone(gap.RequiredEvidence)
	slices.Sort(required)
	required = slices.Compact(required)
	values := append([]string{gap.Stage, gap.Scope, gap.Capability, gap.Code}, required...)
	return v5Tuple(values...)
}

func v5Tuple(values ...string) string {
	var encoded strings.Builder
	for _, value := range values {
		encoded.WriteString(strconv.Itoa(len(value)))
		encoded.WriteByte(':')
		encoded.WriteString(value)
	}
	return encoded.String()
}

func TestVersionFiveSelectionPolicyRejectsUnknownFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(v5ContractDirectory(t), "V5_SELECTION_POLICY.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unexpected"] = true
	mutated, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(mutated)))
	decoder.DisallowUnknownFields()
	var policy v5SelectionPolicy
	if err := decoder.Decode(&policy); err == nil {
		t.Fatal("V5 selection policy accepted an unknown field")
	}
}

func v5ContractDirectory(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate V5 contract test source")
	}
	return filepath.Dir(source)
}

func v5FileSHA256(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func v5DecodeStrictFile(t *testing.T, path string, target any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("%s contains trailing JSON data", path)
	}
}
