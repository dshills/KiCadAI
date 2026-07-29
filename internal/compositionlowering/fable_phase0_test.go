package compositionlowering

import "testing"

func TestFableH16MetadataMergeIsOrderIndependent(t *testing.T) {
	// Exercise two explicit encounter orders without relying on map iteration.
	analog := nodeMetadata{domain: "analog"}
	digital := nodeMetadata{domain: "digital"}
	leftFirst := combineMetadata(combineMetadata(nodeMetadata{}, analog), digital)
	rightFirst := combineMetadata(combineMetadata(nodeMetadata{}, digital), analog)
	if leftFirst != rightFirst {
		t.Fatalf("metadata merge depends on encounter order: %#v %#v", leftFirst, rightFirst)
	}
	if leftFirst.domain != "analog" {
		t.Fatalf("conflicting domains did not use the canonical lexical choice: %#v", leftFirst)
	}
}
