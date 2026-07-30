package main

import (
	"context"
	"strings"

	"kicadai/internal/components"
)

// loadComponentCatalog uses the embedded catalog whenever the caller retains
// the default CLI value. An explicit non-default directory remains an
// intentional override.
func loadComponentCatalog(ctx context.Context, catalogDir string) (*components.Catalog, error) {
	if strings.TrimSpace(catalogDir) == components.DefaultCatalogDir {
		catalogDir = ""
	}
	return components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: catalogDir})
}
