package simmodel

import (
	"slices"
	"testing"
)

func TestPrimitiveDescriptorsAreCompleteAndDefensive(t *testing.T) {
	descriptors := PrimitiveDescriptors()
	ids := PrimitiveModelIDs()
	if len(descriptors) != len(ids) {
		t.Fatalf("descriptor count = %d, want %d", len(descriptors), len(ids))
	}
	for index, descriptor := range descriptors {
		if descriptor.ID != ids[index] || descriptor.Family == "" || len(descriptor.Terminals) < 2 {
			t.Fatalf("descriptor %d = %#v", index, descriptor)
		}
		if len(descriptor.TerminalAliases) != 0 {
			keys := make([]string, 0, len(descriptor.TerminalAliases))
			for key := range descriptor.TerminalAliases {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				if !slices.Contains(descriptor.Terminals, key) {
					t.Fatalf("%s alias terminal %s is undeclared", descriptor.ID, key)
				}
			}
		}
	}
	descriptors[0].Terminals[0] = "MUTATED"
	if PrimitiveDescriptors()[0].Terminals[0] == "MUTATED" {
		t.Fatal("descriptor terminals were not defensively copied")
	}
	for terminal := range descriptors[0].TerminalAliases {
		descriptors[0].TerminalAliases[terminal][0] = "MUTATED"
		if PrimitiveDescriptors()[0].TerminalAliases[terminal][0] == "MUTATED" {
			t.Fatal("descriptor aliases were not defensively copied")
		}
		break
	}
}
