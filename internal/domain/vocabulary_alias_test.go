package domain_test

import (
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/domain"
	"kicadai/internal/placement"
	"kicadai/internal/routing"
	"kicadai/internal/schematicir"
)

func TestPackageVocabularyTypesShareIdentity(t *testing.T) {
	var _ domain.AcceptanceLevel = components.AcceptanceERCDRC
	var _ domain.AcceptanceLevel = designworkflow.AcceptanceERCDRC
	var _ domain.AcceptanceLevel = circuitgraph.AcceptanceERCDRC
	var _ domain.ComponentRole = circuitgraph.RoleIC
	var _ domain.ComponentRole = schematicir.ComponentRoleIC
	var _ domain.NetRole = circuitgraph.NetRolePowerPos
	var _ domain.NetRole = schematicir.NetRoleNoConnect
	var _ domain.NetRole = placement.NetDifferential
	var _ domain.NetRole = routing.NetHighCurrent
}
