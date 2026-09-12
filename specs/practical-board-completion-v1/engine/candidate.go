//go:build completion_candidate

package main

import (
	"kicadai/internal/circuitgraph"
	"kicadai/internal/compositionlowering"
	"kicadai/internal/schematiclayout"
)

const engineVariant = "candidate"

func configureGraph(opts *circuitgraph.ResolveOptions) {
	opts.SchematicLayoutProfile = circuitgraph.SchematicLayoutTopologyV2
}
func configurePromotion(opts *compositionlowering.ArchitectureSimulationPlanResolver) {
	opts.FunctionalLayoutProfile = schematiclayout.FunctionalOwnershipV6
}
