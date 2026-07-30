package main

import (
	"context"
	"testing"

	"kicadai/internal/components"
)

func TestLoadComponentCatalogUsesEmbeddedDefault(t *testing.T) {
	catalog, err := loadComponentCatalog(context.Background(), components.DefaultCatalogDir)
	if err != nil {
		t.Fatalf("load embedded default catalog: %v", err)
	}
	if catalog == nil || len(catalog.Records) == 0 {
		t.Fatalf("embedded default catalog is empty: %#v", catalog)
	}
}
