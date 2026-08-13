package capabilityroundsv9

import (
	"slices"
	"testing"
)

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

func TestCaseHashCommitsDiagnosticsAndIsFrontierOrderIndependent(t *testing.T) {
	first := testCase("case_001", "analog_signal_path", "source_bias", "non_safety", testLeaf("topology", "a", "a", "A"))
	first.Frontier = append(first.Frontier, Gap{ObligationAnchor: identityDigest("anchor", "second"),
		Path: []Leaf{testLeaf("component", "b", "b", "B")}, Diagnostics: []string{"diagnostic"}})
	hash, err := CaseHash(first)
	if err != nil {
		t.Fatal(err)
	}
	reordered := cloneCases([]Case{first})[0]
	slices.Reverse(reordered.Frontier)
	reorderedHash, err := CaseHash(reordered)
	if err != nil || reorderedHash != hash {
		t.Fatalf("frontier order changed canonical case hash: %s %s %v", hash, reorderedHash, err)
	}
	diagnosticDrift := cloneCases([]Case{first})[0]
	diagnosticDrift.Frontier[0].Diagnostics = []string{"different"}
	driftHash, err := CaseHash(diagnosticDrift)
	if err != nil || driftHash == hash {
		t.Fatalf("diagnostic drift was not committed: %s %s %v", hash, driftHash, err)
	}
}
