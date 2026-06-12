package libraryresolver

import (
	"cmp"
	"io/fs"
	slashpath "path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"kicadai/internal/reports"
)

func Discover(roots LibraryRoots) LibraryInventory {
	var inventory LibraryInventory
	inventory.Diagnostics = append(inventory.Diagnostics, ValidateRoots(roots)...)
	var symbolFiles []LibraryFile
	var symbolIssues []reports.Issue
	var footprintFiles []LibraryFile
	var footprintIssues []reports.Issue
	var waitGroup sync.WaitGroup
	if strings.TrimSpace(roots.SymbolsRoot) != "" {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			symbolFiles, symbolIssues = discoverSymbols(roots.SymbolsRoot)
		}()
	}
	if strings.TrimSpace(roots.FootprintsRoot) != "" {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			footprintFiles, footprintIssues = discoverFootprints(roots.FootprintsRoot)
		}()
	}
	waitGroup.Wait()
	inventory.SymbolFiles = append(inventory.SymbolFiles, symbolFiles...)
	inventory.FootprintFiles = append(inventory.FootprintFiles, footprintFiles...)
	inventory.Diagnostics = append(inventory.Diagnostics, symbolIssues...)
	inventory.Diagnostics = append(inventory.Diagnostics, footprintIssues...)
	sortLibraryFiles(inventory.SymbolFiles)
	sortLibraryFiles(inventory.FootprintFiles)
	inventory.SymbolLibraryCount = countLibraryNicknames(inventory.SymbolFiles)
	inventory.FootprintLibraryCount = countLibraryNicknames(inventory.FootprintFiles)
	inventory.Diagnostics = append(inventory.Diagnostics, duplicateNicknameIssues("symbol", inventory.SymbolFiles)...)
	inventory.Diagnostics = append(inventory.Diagnostics, duplicateNicknameIssues("footprint", inventory.FootprintFiles)...)
	inventory.Diagnostics = append(inventory.Diagnostics, duplicatePathIssues(inventory.SymbolFiles, inventory.FootprintFiles)...)
	return inventory
}

func discoverSymbols(root string) ([]LibraryFile, []reports.Issue) {
	return discoverFiles(root, LibraryFileSymbol, ".kicad_sym", symbolLibraryNickname)
}

func discoverFootprints(root string) ([]LibraryFile, []reports.Issue) {
	return discoverFiles(root, LibraryFileFootprint, ".kicad_mod", footprintLibraryNickname)
}

func discoverFiles(root string, kind LibraryFileKind, ext string, nicknameFn func(string, string, string) string) ([]LibraryFile, []reports.Issue) {
	var files []LibraryFile
	var issues []reports.Issue
	rootBase := cleanBase(root)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			issues = append(issues, discoveryIssue(path, err))
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			return nil
		}
		nickname := nicknameFn(root, rootBase, path)
		if nickname == "" {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: filepath.ToSlash(path), Message: "could not derive " + string(kind) + " library nickname"})
			return nil
		}
		name := trimSuffixFold(entry.Name(), ext)
		files = append(files, LibraryFile{Kind: kind, Path: filepath.ToSlash(path), LibraryNickname: nickname, Name: name, IDPrefix: nickname + ":"})
		return nil
	})
	if err != nil {
		issues = append(issues, discoveryIssue(root, err))
	}
	return files, issues
}

func symbolLibraryNickname(root string, rootBase string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if strings.HasSuffix(strings.ToLower(parts[i]), ".kicad_symdir") {
			return trimSuffixFold(parts[i], ".kicad_symdir")
		}
	}
	if strings.HasSuffix(strings.ToLower(rootBase), ".kicad_symdir") {
		return trimSuffixFold(rootBase, ".kicad_symdir")
	}
	if len(parts) > 0 && strings.HasSuffix(strings.ToLower(parts[len(parts)-1]), ".kicad_sym") {
		return trimSuffixFold(parts[len(parts)-1], ".kicad_sym")
	}
	return ""
}

func footprintLibraryNickname(root string, rootBase string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if strings.HasSuffix(strings.ToLower(parts[i]), ".pretty") {
			return trimSuffixFold(parts[i], ".pretty")
		}
	}
	if strings.HasSuffix(strings.ToLower(rootBase), ".pretty") {
		return trimSuffixFold(rootBase, ".pretty")
	}
	return ""
}

func sortLibraryFiles(files []LibraryFile) {
	slices.SortFunc(files, func(a, b LibraryFile) int {
		if v := cmp.Compare(a.Kind, b.Kind); v != 0 {
			return v
		}
		if v := cmp.Compare(a.LibraryNickname, b.LibraryNickname); v != 0 {
			return v
		}
		if v := cmp.Compare(a.Name, b.Name); v != 0 {
			return v
		}
		return cmp.Compare(a.Path, b.Path)
	})
}

func countLibraryNicknames(files []LibraryFile) int {
	seen := map[string]struct{}{}
	for _, file := range files {
		seen[file.LibraryNickname] = struct{}{}
	}
	return len(seen)
}

func duplicateNicknameIssues(kind string, files []LibraryFile) []reports.Issue {
	type nicknameContainer struct {
		nickname  string
		container string
	}
	seen := map[nicknameContainer]struct{}{}
	containerCounts := map[string]int{}
	for _, file := range files {
		key := nicknameContainer{nickname: file.LibraryNickname, container: libraryContainer(file)}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		containerCounts[file.LibraryNickname]++
	}
	var nicknames []string
	for nickname, count := range containerCounts {
		if count > 1 {
			nicknames = append(nicknames, nickname)
		}
	}
	slices.Sort(nicknames)
	var issues []reports.Issue
	for _, nickname := range nicknames {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     "library." + kind + "." + nickname,
			Message:  "multiple " + kind + " library files use nickname " + nickname,
		})
	}
	return issues
}

func libraryContainer(file LibraryFile) string {
	if file.Kind == LibraryFileSymbol && strings.EqualFold(slashpath.Ext(file.Path), ".kicad_sym") {
		dir := slashpath.Dir(file.Path)
		if strings.HasSuffix(strings.ToLower(slashpath.Base(dir)), ".kicad_symdir") {
			return dir
		}
		return file.Path
	}
	return slashpath.Dir(file.Path)
}

func duplicatePathIssues(groups ...[]LibraryFile) []reports.Issue {
	seen := map[string]LibraryFile{}
	var issues []reports.Issue
	for _, files := range groups {
		for _, file := range files {
			key := strings.ToLower(slashpath.Clean(file.Path))
			if existing, ok := seen[key]; ok {
				issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: file.Path, Message: "duplicate library source path also discovered as " + string(existing.Kind)})
				continue
			}
			seen[key] = file
		}
	}
	return issues
}

func discoveryIssue(path string, err error) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: filepath.ToSlash(path), Message: err.Error()}
}

func trimSuffixFold(value string, suffix string) string {
	if len(value) < len(suffix) || !strings.EqualFold(value[len(value)-len(suffix):], suffix) {
		return value
	}
	return value[:len(value)-len(suffix)]
}

func cleanBase(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		return filepath.Base(absolute)
	}
	return filepath.Base(path)
}
