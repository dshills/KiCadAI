package capabilitybundles

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodePolicyIsStrictAndExact(t *testing.T) {
	policy := testPolicy()
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePolicy(bytes.NewReader(data))
	if err != nil || decoded.Schema != policy.Schema {
		t.Fatalf("DecodePolicy() = %#v, %v", decoded, err)
	}

	unknown := strings.Replace(string(data), `"version":1`, `"version":1,"unknown":true`, 1)
	if _, err := DecodePolicy(strings.NewReader(unknown)); err == nil {
		t.Fatal("DecodePolicy accepted an unknown field")
	}
	trailing := append(data, []byte(` {}`)...)
	if _, err := DecodePolicy(bytes.NewReader(trailing)); err == nil {
		t.Fatal("DecodePolicy accepted multiple JSON values")
	}
	policy.UnlockRule = "partial_overlap"
	invalid, _ := json.Marshal(policy)
	if _, err := DecodePolicy(bytes.NewReader(invalid)); err == nil {
		t.Fatal("DecodePolicy accepted partial blocker coverage")
	}
}

func TestValidatePolicyRejectsGateRelaxation(t *testing.T) {
	base := testPolicy()
	mutations := []func(*Policy){
		func(value *Policy) { value.BundleGeneration.DiscoveryOnly = false },
		func(value *Policy) { value.BundleGeneration.MaximumCapabilityAtoms = 0 },
		func(value *Policy) { value.BundleGeneration.MaximumCandidateBundles = 0 },
		func(value *Policy) { value.BundleGeneration.MinimumAtomCaseSupport = 0 },
		func(value *Policy) { value.TieBehavior = "choose_first" },
		func(value *Policy) { value.RankOnePlanAdmission.RequireAllMembersPlanned = false },
		func(value *Policy) { value.HeldOutInfluence = "allowed" },
		func(value *Policy) { value.UnsafeCaseUnlockCredit = "allowed" },
		func(value *Policy) { value.PublicAdmission.RequireTotalPassUplift = false },
		func(value *Policy) { value.PublicAdmission.MinimumRealizedClaimedUnlocks = 0 },
	}
	for index, mutate := range mutations {
		candidate := base
		candidate.AtomIdentityFields = append([]string(nil), base.AtomIdentityFields...)
		candidate.MemberIdentityFields = append([]string(nil), base.MemberIdentityFields...)
		candidate.EligibleBaselineOutcomes = append([]string(nil), base.EligibleBaselineOutcomes...)
		candidate.Ranking = append([]string(nil), base.Ranking...)
		mutate(&candidate)
		if err := ValidatePolicy(candidate); err == nil {
			t.Fatalf("ValidatePolicy accepted relaxation %d", index)
		}
	}
}
