package libraryresolver

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/reports"
)

var kicadVariablePattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)
var shellVariableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func discoverProjectSymbolTable(ctx context.Context, roots LibraryRoots, envVariables map[string]string) ([]LibraryFile, []reports.Issue) {
	if issue, ok := contextIssue(ctx); ok {
		return nil, []reports.Issue{issue}
	}
	projectDir := strings.TrimSpace(roots.ProjectDir)
	if projectDir == "" {
		return nil, nil
	}
	tablePath := filepath.Join(projectDir, "sym-lib-table")
	if _, err := os.Stat(tablePath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []reports.Issue{parseIssue(filepath.ToSlash(tablePath), err.Error())}
	}
	variables := symbolTableVariables(roots, projectDir, envVariables)
	return discoverSymbolTableLibraries(ctx, tablePath, projectDir, variables, LibrarySourceProjectTable)
}

func discoverGlobalSymbolTable(ctx context.Context, roots LibraryRoots, envVariables map[string]string) ([]LibraryFile, []reports.Issue) {
	if issue, ok := contextIssue(ctx); ok {
		return nil, []reports.Issue{issue}
	}
	tablePath := strings.TrimSpace(roots.GlobalSymbolTable)
	if tablePath == "" {
		return nil, nil
	}
	variables := symbolTableVariables(roots, strings.TrimSpace(roots.ProjectDir), envVariables)
	return discoverSymbolTableLibraries(ctx, tablePath, filepath.Dir(tablePath), variables, LibrarySourceGlobalTable)
}

func discoverSymbolTableLibraries(ctx context.Context, tablePath string, projectDir string, variables map[string]string, source string) ([]LibraryFile, []reports.Issue) {
	if issue, ok := contextIssue(ctx); ok {
		return nil, []reports.Issue{issue}
	}
	entries, issues := ParseSymbolLibraryTable(tablePath)
	var files []LibraryFile
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if issue, ok := contextIssue(ctx); ok {
			issues = append(issues, issue)
			return files, issues
		}
		if _, exists := seen[entry.Name]; exists {
			issues = append(issues, symbolTableEntryIssue(entry.Name, tablePath, "duplicate symbol library nickname "+entry.Name+" in "+source))
			continue
		}
		seen[entry.Name] = struct{}{}
		resolved, ok := expandSymbolTableURI(entry.URI, variables)
		if !ok {
			issues = append(issues, symbolTableEntryIssue(entry.Name, tablePath, "unresolved symbol library URI variable in "+entry.Name+": "+entry.URI))
			continue
		}
		if !filepath.IsAbs(resolved) && projectDir != "" {
			resolved = filepath.Join(projectDir, resolved)
		}
		if !filepath.IsAbs(resolved) {
			issues = append(issues, symbolTableEntryIssue(entry.Name, tablePath, "relative symbol library URI requires a base directory for "+entry.Name+": "+entry.URI))
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil {
			code := reports.CodeValidationFailed
			message := "symbol library path error for " + entry.Name + ": " + filepath.ToSlash(resolved) + ": " + err.Error()
			if os.IsNotExist(err) {
				code = reports.CodeMissingFile
				message = "symbol library path not found for " + entry.Name + ": " + filepath.ToSlash(resolved)
			}
			issue := symbolTableEntryIssue(entry.Name, tablePath, message)
			issue.Code = code
			issues = append(issues, issue)
			continue
		}
		if info.IsDir() {
			issue := symbolTableEntryIssue(entry.Name, tablePath, "symbol library path is a directory for "+entry.Name+": "+filepath.ToSlash(resolved))
			issue.Code = reports.CodeInvalidArgument
			issues = append(issues, issue)
			continue
		}
		// Name is the container stem for monolithic symbol files; individual
		// symbol names are discovered by parseSymbolFile. Footprint LibraryFile
		// values instead represent one object per .kicad_mod file.
		files = append(files, LibraryFile{Kind: LibraryFileSymbol, Path: filepath.ToSlash(resolved), LibraryNickname: entry.Name, Name: trimSuffixFold(filepath.Base(resolved), ".kicad_sym"), IDPrefix: entry.Name + ":", Source: source})
	}
	return files, issues
}

func symbolTableEntryIssue(nickname string, tablePath string, message string) reports.Issue {
	return reports.Issue{
		Code: reports.CodeValidationFailed, Severity: reports.SeverityError,
		Path: "library.symbol." + strings.TrimSpace(nickname), Message: message,
		Refs: []string{filepath.ToSlash(tablePath)},
	}
}

func ParseSymbolLibraryTable(path string) ([]SymbolLibraryTableEntry, []reports.Issue) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []reports.Issue{parseIssue(filepath.ToSlash(path), err.Error())}
	}
	root, err := sexpr.Parse(data)
	if err != nil {
		return nil, []reports.Issue{parseIssue(filepath.ToSlash(path), err.Error())}
	}
	if root.Head() != "sym_lib_table" {
		return nil, []reports.Issue{parseIssue(filepath.ToSlash(path), "expected sym_lib_table root, got "+root.Head())}
	}
	libs := root.ChildrenByHead("lib")
	entries := make([]SymbolLibraryTableEntry, 0, len(libs))
	var issues []reports.Issue
	for _, lib := range libs {
		entry := SymbolLibraryTableEntry{Path: filepath.ToSlash(path)}
		for _, child := range lib.Children {
			if len(child.Children) < 2 {
				continue
			}
			switch child.Head() {
			case "name":
				entry.Name = strings.TrimSpace(child.ListValue(1))
			case "type":
				entry.Type = strings.TrimSpace(child.ListValue(1))
			case "uri":
				entry.URI = strings.TrimSpace(child.ListValue(1))
			case "options":
				entry.Options = child.ListValue(1)
			case "descr":
				entry.Description = child.ListValue(1)
			}
		}
		if entry.Name == "" || entry.URI == "" {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: filepath.ToSlash(path), Message: "skipping malformed symbol library table entry with missing name or uri"})
			continue
		}
		entries = append(entries, entry)
	}
	return entries, issues
}

func environmentVariables() map[string]string {
	variables := map[string]string{}
	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if ok && allowedKiCadVariableName(key) {
			variables[key] = value
		}
	}
	return variables
}

func allowedKiCadVariableName(name string) bool {
	name = strings.TrimSpace(name)
	return shellVariableNamePattern.MatchString(name)
}

func symbolTableVariables(roots LibraryRoots, projectDir string, base map[string]string) map[string]string {
	variables := make(map[string]string, len(base)+20)
	for key, value := range base {
		variables[key] = value
	}
	if strings.TrimSpace(projectDir) != "" {
		variables["KIPRJMOD"] = projectDir
	}
	if strings.TrimSpace(roots.SymbolsRoot) != "" {
		variables["KICAD_SYMBOL_DIR"] = roots.SymbolsRoot
		variables["KICAD_SYMBOLS_DIR"] = roots.SymbolsRoot
		for version := 6; version <= 20; version++ {
			variables["KICAD"+strconv.Itoa(version)+"_SYMBOL_DIR"] = roots.SymbolsRoot
		}
	}
	addCaseInsensitiveVariableAliases(variables)
	return variables
}

func addCaseInsensitiveVariableAliases(variables map[string]string) {
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		lower := strings.ToLower(key)
		if _, exists := variables[lower]; !exists {
			variables[lower] = variables[key]
		}
	}
}

func expandSymbolTableURI(uri string, variables map[string]string) (string, bool) {
	ok := true
	expanded := kicadVariablePattern.ReplaceAllStringFunc(uri, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		value, exists := lookupSymbolTableVariable(variables, name)
		if !exists {
			ok = false
			return match
		}
		return value
	})
	return expanded, ok
}

func lookupSymbolTableVariable(variables map[string]string, name string) (string, bool) {
	value, ok := variables[name]
	if ok {
		return value, true
	}
	value, ok = variables[strings.ToLower(name)]
	if ok {
		return value, true
	}
	return "", false
}
