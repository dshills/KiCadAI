//go:build !completion_candidate

package main

import (
	"kicadai/internal/circuitgraph"
	"kicadai/internal/compositionlowering"
)

const engineVariant = "baseline"

// Preserve baseline policy exactly. No candidate behavior is injected here.
func configureGraph(_ *circuitgraph.ResolveOptions)                                {}
func configurePromotion(_ *compositionlowering.ArchitectureSimulationPlanResolver) {}
