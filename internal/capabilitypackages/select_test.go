package capabilitypackages

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestBuildUsesValidatedDiscoveryEvidence(t *testing.T) {
	observations := []Observation{
		{Role: "discovery", CaseID: "a1", ReportingDomain: "analog", SafetyWeight: 1, Stage: "s1", Scope: "topology", Capability: "cap_a", Code: "x", RequiredEvidence: []string{"erc"}},
		{Role: "discovery", CaseID: "a2", ReportingDomain: "power", SafetyWeight: 3, Stage: "s2", Scope: "topology", Capability: "cap_a", Code: "y", RequiredEvidence: []string{"drc"}},
		{Role: "discovery", CaseID: "a3", ReportingDomain: "analog", SafetyWeight: 5, Stage: "s1", Scope: "topology", Capability: "cap_a", Code: "x", RequiredEvidence: []string{"erc"}},
		{Role: "discovery", CaseID: "b1", ReportingDomain: "sensor", SafetyWeight: 5, Stage: "s", Scope: "model", Capability: "cap_b", Code: "x"},
		{Role: "discovery", CaseID: "b2", ReportingDomain: "mixed_signal", SafetyWeight: 5, Stage: "s", Scope: "model", Capability: "cap_b", Code: "x"},
		{Role: "held_out", CaseID: "hidden", ReportingDomain: "power", SafetyWeight: 1000, Stage: "s", Scope: "topology", Capability: "cap_z", Code: "x"},
	}
	candidates, err := Build(observations, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].Capability != "cap_a" || candidates[1].Capability != "cap_b" {
		t.Fatalf("package ranking = %#v", candidates)
	}
	for index, candidate := range candidates {
		if candidate.Rank != index+1 || candidate.Key != Tuple(candidate.Scope, candidate.Capability) {
			t.Fatalf("candidate %d is not canonically ranked: %#v", index, candidate)
		}
	}
	mutated := append(slices.Clone(observations), Observation{Role: "held_out", CaseID: "hidden2", ReportingDomain: "analog", SafetyWeight: 9999, Stage: "s", Scope: "topology", Capability: "cap_z", Code: "y"})
	again, err := Build(mutated, testPolicy())
	if err != nil || !reflect.DeepEqual(candidates, again) {
		t.Fatalf("held-out evidence changed discovery ranking: %v %#v", err, again)
	}
}

func TestFrozenPolicyAndRanking(t *testing.T) {
	policy := testPolicy()
	if err := ValidatePolicy(policy); err != nil {
		t.Fatal(err)
	}
	candidates := []Candidate{
		{Key: Tuple("model", "cap_b"), Scope: "model", Capability: "cap_b", Cases: []string{"b1", "b2"}, Domains: []string{"mixed_signal", "sensor"}, SafetyWeight: 18, Members: []Member{{Key: Tuple("s", "model", "cap_b", "x")}}, Rank: 2},
		{Key: Tuple("topology", "cap_a"), Scope: "topology", Capability: "cap_a", Cases: []string{"a1", "a2", "a3"}, Domains: []string{"analog", "power"}, SafetyWeight: 3, Members: []Member{{Key: Tuple("s1", "topology", "cap_a", "x")}, {Key: Tuple("s2", "topology", "cap_a", "y")}}, Rank: 1},
	}
	slices.SortFunc(candidates, func(left, right Candidate) int { return compare(left, right, policy.Ranking) })
	if candidates[0].Capability != "cap_a" {
		t.Fatalf("rank one = %s, want cap_a", candidates[0].Capability)
	}
	reversed := slices.Clone(policy.Ranking)
	reversed[0], reversed[2] = reversed[2], reversed[0]
	slices.SortFunc(candidates, func(left, right Candidate) int { return compare(left, right, reversed) })
	if candidates[0].Capability != "cap_b" {
		t.Fatalf("configured safety-first rank one = %s, want cap_b", candidates[0].Capability)
	}
}

func TestDecodePolicyStrict(t *testing.T) {
	data, err := json.Marshal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePolicy(bytes.NewReader(data))
	if err != nil || !reflect.DeepEqual(decoded, testPolicy()) {
		t.Fatalf("strict policy decode = %#v, %v", decoded, err)
	}
	mutated := append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
	if _, err := DecodePolicy(bytes.NewReader(mutated)); err == nil {
		t.Fatal("strict policy decode accepted an unknown field")
	}
}

func TestGenericPlanCoversEveryExactMember(t *testing.T) {
	members := []Member{
		{Key: Tuple("topology_search", "topology", "complete_topology", "FIRST"), Stage: "topology_search", Scope: "topology", Capability: "complete_topology", Code: "FIRST"},
		{Key: Tuple("topology_search", "topology", "complete_topology", "SECOND"), Stage: "topology_search", Scope: "topology", Capability: "complete_topology", Code: "SECOND"},
	}
	candidate := Candidate{Rank: 1, Key: Tuple("topology", "complete_topology"), Scope: "topology", Capability: "complete_topology", Cases: []string{"a", "b"}, Domains: []string{"analog", "power"}, Members: members, RequiredEvidence: []string{"simulation", "writer_correctness"}}
	plan, err := BuildGenericPlan(candidate)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectRankOne([]Candidate{candidate}, plan, testPolicy())
	if err != nil || selected.Key != candidate.Key {
		t.Fatalf("select rank one = %#v, %v", selected, err)
	}
	partial := plan
	partial.Members = partial.Members[:1]
	partial.Bindings = partial.Bindings[:1]
	partial.Hash, err = hashPlan(partial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SelectRankOne([]Candidate{candidate}, partial, testPolicy()); err == nil {
		t.Fatal("rank-one admission accepted a partial generic plan")
	}
	mutated := plan
	mutated.Bindings = slices.Clone(plan.Bindings)
	mutated.Bindings[0].NeedID = "missing:need"
	mutated.Hash, err = hashPlan(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePlan(mutated); err == nil {
		t.Fatal("generic plan accepted a binding to a missing need")
	}
}

func TestTupleIsInjectiveAndUsesUTF8ByteLength(t *testing.T) {
	if Tuple("a", "bc") == Tuple("ab", "c") || Tuple("é") != "2:é" || Tuple("", "", "") != "0:0:0:" {
		t.Fatal("length-delimited identity encoding is ambiguous")
	}
}

func testPolicy() SelectionPolicy {
	return SelectionPolicy{
		Schema: PolicySchema, Version: PolicyVersion,
		PackageIdentityFields: []string{"scope", "capability"}, MemberIdentityFields: []string{"stage", "scope", "capability", "code"},
		IdentityEncoding: "utf8-decimal-byte-length-colon", AffectedCaseAggregation: "sorted-unique-union", RequiredEvidenceAggregation: "sorted-unique-union",
		Eligibility:          SelectionEligibility{MinimumAffectedDiscoveryCases: 2, MinimumReportingDomains: 2},
		RankOnePlanAdmission: PlanAdmission{RequireCompleteGenericPlan: true, RequireAllMembersPlanned: true},
		Ranking:              []string{"affected_discovery_cases_desc", "reporting_domains_desc", "safety_impact_weight_desc", "member_cluster_count_asc", "required_evidence_count_asc", "canonical_package_key_asc"},
		TieBehavior:          "ranking_tuple_then_canonical_package_key", IneligibleBehavior: "exclude_before_ranking", NoEligiblePackageBehavior: "fail_closed", UnexecutableRankOneBehavior: "fail_closed_no_fallback", HeldOutInfluence: "prohibited", SelectedMemberGapRemoval: "exact_member_identity_only", HeldOutSuccessCausality: "at_least_one_new_pass_with_matching_baseline_package",
	}
}
