package capabilityroundsv8

import "testing"

func TestPathIdentityCommitsObligationMembersAndEvidenceUnambiguously(t *testing.T) {
	leaf := testLeaf("model", "scope", "capability", "CODE")
	gap := Gap{ObligationAnchor: identityDigest("anchor"), Path: []Leaf{leaf}, Diagnostics: []string{"diagnostic"}}
	first, err := PathHash(gap)
	if err != nil {
		t.Fatal(err)
	}
	changedEvidence := gap
	changedEvidence.Path = []Leaf{leaf}
	changedEvidence.Path[0].RequiredEvidence = []string{"evidence", "simulation"}
	second, err := PathHash(changedEvidence)
	if err != nil || first == second {
		t.Fatalf("evidence did not affect path identity: %s %s %v", first, second, err)
	}
	changedAnchor := gap
	changedAnchor.ObligationAnchor = identityDigest("other-anchor")
	third, err := PathHash(changedAnchor)
	if err != nil || first == third {
		t.Fatalf("anchor did not affect path identity: %s %s %v", first, third, err)
	}
	ambiguousA := identityDigest("ab", "c")
	ambiguousB := identityDigest("a", "bc")
	if ambiguousA == ambiguousB {
		t.Fatal("length-prefixed identity encoding is ambiguous")
	}
	separateEvidence := gap
	separateEvidence.Path = []Leaf{leaf}
	separateEvidence.Path[0].RequiredEvidence = []string{"a", "b"}
	joinedEvidence := gap
	joinedEvidence.Path = []Leaf{leaf}
	joinedEvidence.Path[0].RequiredEvidence = []string{"a\x00b"}
	separateHash, separateErr := PathHash(separateEvidence)
	joinedHash, joinedErr := PathHash(joinedEvidence)
	if separateErr != nil || joinedErr != nil || separateHash == joinedHash {
		t.Fatalf("evidence element boundaries are ambiguous: %s %s %v %v", separateHash, joinedHash, separateErr, joinedErr)
	}
}
