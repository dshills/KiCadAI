package circuitgraph

import (
	"kicadai/internal/designworkflow"
	"kicadai/internal/schematiclayout"
)

// annotationIndependentPlacementSeed reuses the existing deterministic hash
// algorithm with exactly one drawing-only tag omitted. This preserves the
// topology-v1 placement seed for the same circuit, without hard-coded seeds or
// erasing annotation provenance from GenerationHash / ResolutionHash. Other
// physical, electrical and layout inputs remain hashed. This is deliberately
// not a claim that all schematic metadata is physically irrelevant.
func annotationIndependentPlacementSeed(resolved ResolvedDocument) *designworkflow.ExplicitPlacementSeedSpec {
	if resolved.Source.Schematic.Rules.NativeProfile != schematiclayout.NativeAnnotationV2 {
		return nil
	}
	copy := resolved
	copy.Source.Schematic.Rules.NativeProfile = ""
	return &designworkflow.ExplicitPlacementSeedSpec{Policy: designworkflow.ExplicitPlacementSeedAnnotationIndependentV1, Hash: generationHash(copy)}
}
