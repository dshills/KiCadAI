package libraryresolver

import (
	"embed"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"kicadai/internal/reports"
)

// bundledSymbolLibraries contains reviewed symbols for qualified parts that
// are not yet present in the supported KiCad installation. They are indexed
// exactly like external libraries and embedded into generated schematics, so
// clean-checkout generation does not depend on mutating the KiCad install.
//
//go:embed bundled/*.kicad_sym
var bundledSymbolLibraries embed.FS

var (
	bundledSymbolOnce    sync.Once
	bundledSymbolRecords map[string]SymbolRecord
	bundledSymbolIssues  []reports.Issue
)

func resolveBundledSymbol(libraryID string) (SymbolRecord, bool) {
	bundledSymbolOnce.Do(loadBundledSymbols)
	record, ok := bundledSymbolRecords[libraryID]
	return record, ok
}

func loadBundledSymbols() {
	bundledSymbolRecords = map[string]SymbolRecord{}
	names, err := bundledSymbolLibraries.ReadDir("bundled")
	if err != nil {
		bundledSymbolIssues = append(bundledSymbolIssues, parseIssue("bundled", err.Error()))
		return
	}
	slices.SortFunc(names, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	for _, entry := range names {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".kicad_sym") {
			continue
		}
		filePath := path.Join("bundled", entry.Name())
		data, readErr := bundledSymbolLibraries.ReadFile(filePath)
		if readErr != nil {
			bundledSymbolIssues = append(bundledSymbolIssues, parseIssue(filePath, readErr.Error()))
			continue
		}
		nickname := strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		file := LibraryFile{
			Kind:            LibraryFileSymbol,
			Path:            "embedded://" + filePath,
			LibraryNickname: nickname,
			Name:            nickname,
			IDPrefix:        nickname + ":",
			Source:          LibrarySourceBundled,
		}
		records, parseIssues := parseSymbolData(file, data)
		bundledSymbolIssues = append(bundledSymbolIssues, parseIssues...)
		for _, record := range records {
			record.SearchText = buildSymbolSearchText(record)
			bundledSymbolRecords[record.LibraryID] = record
		}
	}
}
