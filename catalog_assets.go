package kicadai

import "embed"

// DefaultComponentCatalog contains the catalog shipped with the KiCadAI binary.
// Callers that need local or private records can still select a filesystem
// catalog explicitly through components.LoadOptions.CatalogDir.
//
//go:embed data/components/*.json
var DefaultComponentCatalog embed.FS
