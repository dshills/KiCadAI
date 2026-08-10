package corpusfreezev6

import (
	"reflect"
	"testing"

	"kicadai/internal/corpusfreeze"
)

func TestPolicyChangesOnlyV6IdentityAndCommitments(t *testing.T) {
	v5 := corpusfreeze.V5Policy()
	v6 := Policy()
	if v6.AssignmentSchema != "kicadai.closed-loop-open-set-author-assignment.v6" ||
		v6.AuthorshipSchema != "kicadai.closed-loop-open-set-authorship.v6" || v6.Version != 6 ||
		v6.PacketSetSHA256 != "664b6d20ceb1433509e20016e0fbe3ddf98f6c8c1da01f5aeca7f50f2db6f31a" ||
		v6.HistoricalCommitmentsSHA256 != "eb329517366df07d5364bdc43424a8caf2f86d8bd806086b0af8ea68f02f5807" ||
		!reflect.DeepEqual(v6.ProhibitedIdentityPrefixes, []string{"v6_case_", "v6_source_"}) {
		t.Fatal("V6 policy identity or commitment boundary is invalid")
	}
	v5.AssignmentSchema, v6.AssignmentSchema = "", ""
	v5.AuthorshipSchema, v6.AuthorshipSchema = "", ""
	v5.Version, v6.Version = 0, 0
	v5.PacketSetSHA256, v6.PacketSetSHA256 = "", ""
	v5.HistoricalCommitmentsSHA256, v6.HistoricalCommitmentsSHA256 = "", ""
	v5.ProhibitedIdentityPrefixes, v6.ProhibitedIdentityPrefixes = nil, nil
	if !reflect.DeepEqual(v5, v6) {
		t.Fatal("V6 policy relaxed a frozen V5 corpus rule")
	}
}

func TestPolicyReturnsIndependentIdentityPrefixes(t *testing.T) {
	first := Policy()
	first.ProhibitedIdentityPrefixes[0] = "mutated"
	if Policy().ProhibitedIdentityPrefixes[0] != "v6_case_" {
		t.Fatal("V6 policy returned aliased identity prefixes")
	}
}
