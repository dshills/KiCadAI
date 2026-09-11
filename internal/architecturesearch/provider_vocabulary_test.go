package architecturesearch

import (
	"reflect"
	"testing"
)

func TestV3ProviderVocabularyMatchesValidatorAndIsFresh(t *testing.T) {
	want := ProviderVocabularyV3{PortKinds: registeredPortKinds, Directions: registeredDirections, CanonicalUnits: registeredCanonicalUnits, ConstraintRelations: registeredConstraintRelations, ProtocolModes: registeredProtocolModes, BehavioralMetrics: registeredBehavioralMetrics, OperatingAxes: registeredOperatingAxes}
	first := V3ProviderVocabulary()
	if !reflect.DeepEqual(first, want) {
		t.Fatal("provider vocabulary differs from validator")
	}
	first.PortKinds[0] = "changed"
	first.Directions[0] = "changed"
	first.CanonicalUnits[0] = "changed"
	first.ConstraintRelations[0] = "changed"
	first.ProtocolModes[0] = "changed"
	first.BehavioralMetrics[0].Metric = "changed"
	first.OperatingAxes[0].Axis = "changed"
	if !reflect.DeepEqual(V3ProviderVocabulary(), want) {
		t.Fatal("caller mutated validator vocabulary")
	}
}
