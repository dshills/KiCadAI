package components

import (
	"context"
	"io/fs"
	"path"
	"sort"

	assets "kicadai"
	"kicadai/internal/reports"
)

// V18CatalogExtensionDir is the separately versioned component evidence used
// only by the V18 capability path. LoadCatalog intentionally does not read it.
const V18CatalogExtensionDir = "data/components-v18"

// LoadCatalogV18 loads the immutable legacy catalog plus the reviewed V18
// extension as one validated snapshot. Keeping this explicit prevents a new
// component from changing historical catalog and primitive-inventory hashes.
func LoadCatalogV18(ctx context.Context) (*Catalog, error) {
	files := make([]string, 0)
	for _, dir := range []string{DefaultCatalogDir, V18CatalogExtensionDir} {
		entries, err := fs.ReadDir(assets.DefaultComponentCatalog, dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
				continue
			}
			files = append(files, path.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return loadCatalogFiles(ctx, "embedded:v18", files, func(file string) (catalogFile, []reports.Issue) {
		body, err := fs.ReadFile(assets.DefaultComponentCatalog, file)
		if err != nil {
			return catalogFile{}, []reports.Issue{NewIssue(CodeCatalogReadFailed, reports.SeverityBlocked, file, err.Error())}
		}
		return parseCatalogFile(file, body)
	})
}
