package architecturesearch

import "slices"

// ProviderVocabularyV3 is compiler vocabulary, not evidence that a particular
// installed registry can synthesize a design. Installed capability hashes and
// objective kinds must still come from a validated registry snapshot.
type ProviderVocabularyV3 struct {
	PortKinds           []string
	Directions          []string
	CanonicalUnits      []string
	ConstraintRelations []string
	ProtocolModes       []string
	BehavioralMetrics   []BehavioralMetricCapability
	OperatingAxes       []OperatingAxisCapability
}

// V3ProviderVocabulary returns fresh copies of the same tables used by Validate.
func V3ProviderVocabulary() ProviderVocabularyV3 {
	return ProviderVocabularyV3{
		PortKinds: slices.Clone(registeredPortKinds), Directions: slices.Clone(registeredDirections),
		CanonicalUnits: slices.Clone(registeredCanonicalUnits), ProtocolModes: slices.Clone(registeredProtocolModes),
		ConstraintRelations: slices.Clone(registeredConstraintRelations),
		BehavioralMetrics:   slices.Clone(registeredBehavioralMetrics), OperatingAxes: slices.Clone(registeredOperatingAxes),
	}
}
