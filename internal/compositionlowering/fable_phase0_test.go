package compositionlowering

import "testing"

func TestFableH16ReproductionMetadataMergeIsEncounterOrderDependent(t *testing.T) {
	// Exercise two explicit encounter orders without relying on map iteration.
	analog := nodeMetadata{domain: "analog"}
	digital := nodeMetadata{domain: "digital"}
	leftFirst := combineMetadata(combineMetadata(nodeMetadata{}, analog), digital)
	rightFirst := combineMetadata(combineMetadata(nodeMetadata{}, digital), analog)
	if leftFirst.domain == rightFirst.domain {
		t.Fatalf("metadata merge is now order-independent: %#v %#v", leftFirst, rightFirst)
	}
	if leftFirst.domain != "analog" || rightFirst.domain != "digital" {
		t.Fatalf("unexpected order-dependent domains: %#v %#v", leftFirst, rightFirst)
	}
}
