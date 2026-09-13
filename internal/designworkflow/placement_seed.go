package designworkflow

import (
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
)

const ExplicitPlacementSeedAnnotationIndependentV1 = "annotation-independent-v1"

// ExplicitPlacementSeedSpec decouples the opt-in annotation policy from the
// legacy generation hash used by PCB placement. It does not replace provenance.
type ExplicitPlacementSeedSpec struct {
	Policy string `json:"policy"`
	Hash   string `json:"hash"`
}

func validateExplicitPlacementSeed(circuit ExplicitCircuitSpec) []reports.Issue {
	seed := circuit.PlacementSeed
	if seed == nil {
		return nil
	}
	if seed.Policy != ExplicitPlacementSeedAnnotationIndependentV1 || !validSHA256(seed.Hash) || circuit.Schematic.Layout.NativeProfile != schematiclayout.NativeAnnotationV2 {
		return []reports.Issue{issue("explicit_circuit.placement_seed", "placement seed requires annotation-independent-v1, a lowercase SHA-256 digest and the annotation-v2 schematic profile")}
	}
	return nil
}

func explicitPlacementSeed(circuit ExplicitCircuitSpec) string {
	if circuit.PlacementSeed != nil {
		return circuit.PlacementSeed.Hash
	}
	if circuit.GenerationHash != "" {
		return circuit.GenerationHash
	}
	return circuit.ResolutionHash
}
