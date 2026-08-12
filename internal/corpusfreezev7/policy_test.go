package corpusfreezev7

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev6"
)

func TestPolicyChangesOnlyV7IdentityAndCommitments(t *testing.T) {
	v6 := corpusfreezev6.Policy()
	v7 := Policy()
	if v7.AssignmentSchema != "kicadai.closed-loop-open-set-author-assignment.v7" ||
		v7.AuthorshipSchema != "kicadai.closed-loop-open-set-authorship.v7" || v7.Version != 7 ||
		v7.PacketSetSHA256 != PacketSetSHA256 || v7.HistoricalCommitmentsSHA256 != HistoricalCommitmentsSHA256 ||
		!reflect.DeepEqual(v7.ProhibitedIdentityPrefixes, []string{"v7_case_", "v7_source_"}) {
		t.Fatal("V7 policy identity or commitment boundary is invalid")
	}
	v6.AssignmentSchema, v7.AssignmentSchema = "", ""
	v6.AuthorshipSchema, v7.AuthorshipSchema = "", ""
	v6.Version, v7.Version = 0, 0
	v6.PacketSetSHA256, v7.PacketSetSHA256 = "", ""
	v6.HistoricalCommitmentsSHA256, v7.HistoricalCommitmentsSHA256 = "", ""
	v6.ProhibitedIdentityPrefixes, v7.ProhibitedIdentityPrefixes = nil, nil
	if !reflect.DeepEqual(v6, v7) {
		t.Fatal("V7 policy relaxed a frozen V6 corpus rule")
	}
}

func TestPolicyReturnsIndependentIdentityPrefixes(t *testing.T) {
	first := Policy()
	first.ProhibitedIdentityPrefixes[0] = "mutated"
	if Policy().ProhibitedIdentityPrefixes[0] != "v7_case_" {
		t.Fatal("V7 policy returned aliased identity prefixes")
	}
}

func TestFrozenV7PacketLoadsThroughProductionBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate V7 packet fixture")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "specs", "closed-loop-open-set-capability-expansion", "v7-authoring-packet")
	packet, err := corpusfreeze.LoadPacket(root, Policy())
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Assignments) != 3 || packet.Binding.PacketSetSHA256 != PacketSetSHA256 ||
		packet.Binding.ContractBindingSHA256 != "a05533b2f7cd05bf45d5b0e0209d7dfde63d98f3071d4d9ee9ccadf58b66f77b" {
		t.Fatalf("V7 production packet binding = %#v", packet.Binding)
	}
	wantAuthorPackets := map[string]string{
		"author_1": "212f00f7efee92dec3d449950e43b64c26a62e9d3dce1d2cba941c431a8a7961",
		"author_2": "57ec64f2292d6675c11f20674c120e6403ae4dc70430c60a8e9bb3b4a016dca2",
		"author_3": "569df64dad4f4348a5bcfda36d3177330bdaa36e7e853deb0ebaffb12c050168",
	}
	if !reflect.DeepEqual(packet.Binding.AuthorPacketSHA256, wantAuthorPackets) {
		t.Fatalf("V7 author packet hashes = %#v", packet.Binding.AuthorPacketSHA256)
	}
}
