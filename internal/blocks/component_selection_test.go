package blocks

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"kicadai/internal/components"
)

func TestSelectDefinitionComponentsForLED(t *testing.T) {
	catalog := loadBlockTestCatalog(t)
	definition := ledIndicatorDefinition()
	report := SelectDefinitionComponents(context.Background(), definition, catalog, components.AcceptanceConnectivity)
	if len(report.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", report.Issues)
	}
	if len(report.Selections) != 2 {
		t.Fatalf("expected two selections, got %+v", report.Selections)
	}
	roles := map[string]bool{}
	for _, selection := range report.Selections {
		roles[selection.Role] = true
	}
	if !roles["resistor"] || !roles["led"] {
		t.Fatalf("missing roles: %+v", roles)
	}
}

func TestSelectDefinitionComponentsBlocksPlaceholderAtConnectivity(t *testing.T) {
	catalog := loadBlockTestCatalog(t)
	definition := opampGainStageDefinition()
	definition.Components[0].ComponentQuery = &components.Query{Family: "opamp"}
	definition.Components[0].Acceptance = components.AcceptanceConnectivity
	report := SelectDefinitionComponents(context.Background(), definition, catalog, components.AcceptanceConnectivity)
	if len(report.Issues) == 0 {
		t.Fatal("expected placeholder opamp issue")
	}
}

func loadBlockTestCatalog(t *testing.T) *components.Catalog {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	catalogDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "data", "components")
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{CatalogDir: catalogDir})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}
