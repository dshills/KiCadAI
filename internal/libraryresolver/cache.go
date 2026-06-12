package libraryresolver

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"kicadai/internal/reports"
)

const libraryCacheSchemaVersion = 1

type libraryCacheFile struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Roots         LibraryRoots           `json:"roots"`
	Files         []libraryCacheFileMeta `json:"files"`
	Index         LibraryIndex           `json:"index"`
}

type libraryCacheFileMeta struct {
	Kind       LibraryFileKind `json:"kind"`
	Path       string          `json:"path"`
	Size       int64           `json:"size"`
	ModTimeUTC time.Time       `json:"mod_time_utc"`
}

func loadCache(path string, roots LibraryRoots, inventory LibraryInventory, currentFiles []libraryCacheFileMeta) (LibraryIndex, []reports.Issue, bool) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LibraryIndex{}, nil, false
		}
		return LibraryIndex{}, []reports.Issue{cacheIssue("read cache: " + err.Error())}, false
	}
	defer file.Close()
	var cache libraryCacheFile
	if err := json.NewDecoder(file).Decode(&cache); err != nil {
		return LibraryIndex{}, []reports.Issue{cacheIssue("read cache: " + err.Error())}, false
	}
	if cache.SchemaVersion != libraryCacheSchemaVersion {
		return LibraryIndex{}, []reports.Issue{cacheIssue("cache schema version changed")}, false
	}
	if cache.Roots != roots {
		return LibraryIndex{}, []reports.Issue{cacheIssue("cache roots changed")}, false
	}
	if !cacheMetadataEqual(cache.Files, currentFiles) {
		return LibraryIndex{}, []reports.Issue{cacheIssue("cache file metadata changed")}, false
	}
	index := cache.Index
	index.GeneratedAt = cache.GeneratedAt
	index.Roots = roots
	index.Inventory = inventory
	rehydrateSearchText(index.Symbols, index.Footprints)
	return index, nil, true
}

func writeCache(path string, index LibraryIndex, files []libraryCacheFileMeta) []reports.Issue {
	cache := libraryCacheFile{
		SchemaVersion: libraryCacheSchemaVersion,
		GeneratedAt:   index.GeneratedAt,
		Roots:         index.Roots,
		Files:         files,
		Index:         index,
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return []reports.Issue{cacheIssue("write cache: " + err.Error())}
	}
	temp, err := os.CreateTemp(dir, ".library-index-*.tmp")
	if err != nil {
		return []reports.Issue{cacheIssue("write cache: " + err.Error())}
	}
	tempName := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()
	if err := json.NewEncoder(temp).Encode(cache); err != nil {
		_ = temp.Close()
		return []reports.Issue{cacheIssue("write cache: " + err.Error())}
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return []reports.Issue{cacheIssue("write cache: " + err.Error())}
	}
	if err := temp.Close(); err != nil {
		return []reports.Issue{cacheIssue("write cache: " + err.Error())}
	}
	if err := os.Rename(tempName, path); err != nil {
		return []reports.Issue{cacheIssue("write cache: " + err.Error())}
	}
	removeTemp = false
	return nil
}

func cacheMetadata(inventory LibraryInventory) ([]libraryCacheFileMeta, []reports.Issue) {
	files := append([]LibraryFile{}, inventory.SymbolFiles...)
	files = append(files, inventory.FootprintFiles...)
	metadata := make([]libraryCacheFileMeta, 0, len(files))
	var issues []reports.Issue
	for _, file := range files {
		info, err := os.Stat(file.Path)
		if err != nil {
			issues = append(issues, cacheIssue(fmt.Sprintf("stat cache source %s: %v", file.Path, err)))
			continue
		}
		metadata = append(metadata, libraryCacheFileMeta{
			Kind:       file.Kind,
			Path:       file.Path,
			Size:       info.Size(),
			ModTimeUTC: info.ModTime().UTC(),
		})
	}
	slices.SortFunc(metadata, func(a, b libraryCacheFileMeta) int {
		if a.Kind != b.Kind {
			return cmp.Compare(a.Kind, b.Kind)
		}
		return cmp.Compare(a.Path, b.Path)
	})
	return metadata, issues
}

func cacheMetadataEqual(a []libraryCacheFileMeta, b []libraryCacheFileMeta) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index].Kind != b[index].Kind || a[index].Path != b[index].Path || a[index].Size != b[index].Size || !a[index].ModTimeUTC.Equal(b[index].ModTimeUTC) {
			return false
		}
	}
	return true
}

func rehydrateSearchText(symbols map[string]SymbolRecord, footprints map[string]FootprintRecord) {
	for id, record := range symbols {
		record.SearchText = buildSymbolSearchText(record)
		symbols[id] = record
	}
	for id, record := range footprints {
		record.SearchText = buildFootprintSearchText(record)
		footprints[id] = record
	}
}

func cacheIssue(message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityWarning,
		Path:     "library_cache",
		Message:  message,
	}
}
