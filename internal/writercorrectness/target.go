package writercorrectness

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/reports"
)

const (
	kicadProjectExt       = ".kicad_pro"
	kicadSchematicExt     = ".kicad_sch"
	kicadPCBExt           = ".kicad_pcb"
	maxSheetDiscoverDepth = 32
)

func ValidateProjectStructure(input string, opts Options) Result {
	result := NewResult(input)
	target, check := ResolveTarget(input, opts)
	result.Target = target
	result.AddCheck(check)
	result.Finish()
	return result
}

func ResolveTarget(input string, opts Options) (Target, CheckResult) {
	target := Target{Input: slashPath(strings.TrimSpace(input))}
	if target.Input == "" {
		return target, failedCheck(CheckProjectStructure, "target", "writer correctness target is required")
	}
	absInput, err := filepath.Abs(target.Input)
	if err != nil {
		return target, failedCheck(CheckProjectStructure, target.Input, err.Error())
	}
	info, err := os.Stat(absInput)
	if err != nil {
		return target, failedCheck(CheckProjectStructure, absInput, err.Error())
	}

	var issues []reports.Issue
	if info.IsDir() {
		target = resolveDirectoryTarget(absInput, &issues)
	} else {
		target = resolveFileTarget(absInput, &issues)
	}
	if target.ProjectDir == "" {
		target.ProjectDir = slashPath(filepath.Dir(absInput))
	}

	issues = append(issues, validateBasenames(target)...)
	if target.SchematicPath != "" {
		target.SchematicFiles = discoverSchematicFiles(target.SchematicPath, &issues)
	}
	issues = append(issues, validateLocalLibraryTables(target, opts)...)

	return target, CheckResult{
		Name:     CheckProjectStructure,
		Required: true,
		Issues:   issues,
		Summary:  projectStructureSummary(target),
	}
}

func resolveDirectoryTarget(dir string, issues *[]reports.Issue) Target {
	target := Target{Input: slashPath(dir), ProjectDir: slashPath(dir)}
	projects := filesWithExt(dir, kicadProjectExt, issues)
	schematics := filesWithExt(dir, kicadSchematicExt, issues)
	pcbs := filesWithExt(dir, kicadPCBExt, issues)
	switch len(projects) {
	case 0:
		*issues = append(*issues, BlockingIssue(reports.CodeMissingFile, dir, "no .kicad_pro file found"))
	case 1:
		target.ProjectPath = slashPath(projects[0])
		base := baseNoExt(projects[0])
		target.SchematicPath = slashPath(selectSameBase(base, schematics))
		target.PCBPath = slashPath(selectSameBase(base, pcbs))
		if target.SchematicPath == "" && len(schematics) == 1 {
			target.SchematicPath = slashPath(schematics[0])
		}
		if target.SchematicPath == "" {
			*issues = append(*issues, BlockingIssue(reports.CodeMissingFile, filepath.Join(dir, base+kicadSchematicExt), "matching root schematic file not found"))
		}
	default:
		*issues = append(*issues, BlockingIssue(reports.CodeInvalidArgument, dir, "multiple .kicad_pro files found"))
	}
	return target
}

func resolveFileTarget(path string, issues *[]reports.Issue) Target {
	target := Target{Input: slashPath(path), ProjectDir: slashPath(filepath.Dir(path))}
	switch strings.ToLower(filepath.Ext(path)) {
	case kicadProjectExt:
		target.ProjectPath = slashPath(path)
		dir := filepath.Dir(path)
		base := baseNoExt(path)
		schematicPath := filepath.Join(dir, base+kicadSchematicExt)
		if fileExists(schematicPath) {
			target.SchematicPath = slashPath(schematicPath)
		} else {
			*issues = append(*issues, BlockingIssue(reports.CodeMissingFile, schematicPath, "matching root schematic file not found"))
		}
		pcbPath := filepath.Join(dir, base+kicadPCBExt)
		if fileExists(pcbPath) {
			target.PCBPath = slashPath(pcbPath)
		}
	case kicadSchematicExt:
		target.SchematicPath = slashPath(path)
		projectPath := replaceExt(path, kicadProjectExt)
		if fileExists(projectPath) {
			target.ProjectPath = slashPath(projectPath)
		}
		pcbPath := replaceExt(path, kicadPCBExt)
		if fileExists(pcbPath) {
			target.PCBPath = slashPath(pcbPath)
		}
	case kicadPCBExt:
		target.PCBPath = slashPath(path)
		projectPath := replaceExt(path, kicadProjectExt)
		if fileExists(projectPath) {
			target.ProjectPath = slashPath(projectPath)
		}
		schematicPath := replaceExt(path, kicadSchematicExt)
		if fileExists(schematicPath) {
			target.SchematicPath = slashPath(schematicPath)
		}
	default:
		*issues = append(*issues, BlockingIssue(reports.CodeInvalidArgument, path, "target must be a KiCad project directory, .kicad_pro, .kicad_sch, or .kicad_pcb file"))
	}
	return target
}

func discoverSchematicFiles(root string, issues *[]reports.Issue) []string {
	visited := map[string]struct{}{}
	var files []string
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	if canonicalRoot, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = canonicalRoot
	}
	projectRoot := filepath.Dir(rootAbs)
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		if depth > maxSheetDiscoverDepth {
			*issues = append(*issues, BlockingIssue(reports.CodeValidationFailed, path, "schematic hierarchy exceeds maximum depth"))
			return
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			canonical = path
		}
		abs, err := filepath.Abs(canonical)
		if err != nil {
			*issues = append(*issues, BlockingIssue(reports.CodeValidationFailed, path, err.Error()))
			return
		}
		if _, ok := visited[abs]; ok {
			return
		}
		visited[abs] = struct{}{}
		files = append(files, slashPath(abs))
		sch, err := schematic.ReadFile(abs)
		if err != nil {
			*issues = append(*issues, BlockingIssue(reports.CodeValidationFailed, abs, err.Error()))
			return
		}
		for _, sheet := range sch.Sheets {
			if strings.TrimSpace(sheet.Filename) == "" {
				continue
			}
			child := filepath.Clean(filepath.Join(filepath.Dir(abs), sheet.Filename))
			if !pathWithin(child, projectRoot) {
				*issues = append(*issues, BlockingIssue(reports.CodeValidationFailed, child, "hierarchical child sheet path resolves outside project directory"))
				continue
			}
			if !fileExists(child) {
				*issues = append(*issues, BlockingIssue(reports.CodeMissingFile, child, "hierarchical child sheet file not found"))
				continue
			}
			walk(child, depth+1)
		}
	}
	walk(root, 0)
	slices.Sort(files)
	return files
}

func validateBasenames(target Target) []reports.Issue {
	paths := []string{target.ProjectPath, target.SchematicPath, target.PCBPath}
	base := ""
	var issues []reports.Issue
	for _, path := range paths {
		if path == "" {
			continue
		}
		current := baseNoExt(path)
		if base == "" {
			base = current
			continue
		}
		if !strings.EqualFold(current, base) {
			issues = append(issues, BlockingIssue(reports.CodeValidationFailed, path, "project, schematic, and PCB basenames must match"))
		}
	}
	return issues
}

func validateLocalLibraryTables(target Target, opts Options) []reports.Issue {
	if target.ProjectDir == "" {
		return nil
	}
	dir := filepath.FromSlash(target.ProjectDir)
	env := environmentMap()
	replacements := variableReplacements(dir, env)
	allowedRoots := allowedLibraryRoots(dir, opts, env)
	var issues []reports.Issue
	for _, table := range []string{"fp-lib-table", "sym-lib-table"} {
		path := filepath.Join(dir, table)
		if !fileExists(path) {
			continue
		}
		entries, err := readLibraryTableURIs(path)
		if err != nil {
			issues = append(issues, BlockingIssue(reports.CodeValidationFailed, path, err.Error()))
			continue
		}
		exists := fileExists
		if table == "fp-lib-table" {
			exists = pathExists
		}
		for _, uri := range entries {
			resolved, ok, untrusted := resolveLibraryURI(uri, dir, allowedRoots, replacements)
			switch {
			case untrusted:
				issues = append(issues, WarningIssue(reports.CodeValidationFailed, path, fmt.Sprintf("library URI %q resolves outside allowed roots", uri)))
			case !ok:
				issues = append(issues, WarningIssue(reports.CodeValidationFailed, path, fmt.Sprintf("library URI %q contains unresolved variables", uri)))
			case !exists(resolved):
				issues = append(issues, BlockingIssue(reports.CodeMissingFile, resolved, "library table URI does not resolve to an existing path"))
			}
		}
	}
	return issues
}

func readLibraryTableURIs(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root, err := sexpr.Parse(data)
	if err != nil {
		return nil, err
	}
	var uris []string
	for _, lib := range root.ChildrenByHead("lib") {
		if uri, ok := lib.Child("uri"); ok {
			value := strings.TrimSpace(uri.ListValue(1))
			if value != "" {
				uris = append(uris, value)
			}
		}
	}
	return uris, nil
}

func resolveLibraryURI(uri string, projectDir string, allowedRoots []string, replacements map[string]string) (string, bool, bool) {
	expanded, ok := expandKiCadVariables(uri, projectDir, replacements)
	if !ok {
		return "", false, false
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(projectDir, expanded)
	}
	cleaned, err := filepath.Abs(filepath.Clean(expanded))
	if err != nil {
		return "", false, true
	}
	for _, root := range allowedRoots {
		if pathWithin(cleaned, root) {
			return cleaned, true, false
		}
	}
	return cleaned, true, true
}

func variableReplacements(projectDir string, env map[string]string) map[string]string {
	replacements := map[string]string{}
	for key, value := range env {
		if strings.TrimSpace(value) != "" {
			replacements[key] = value
		}
	}
	replacements["KIPRJMOD"] = projectDir
	replacements["PRJMOD"] = projectDir
	replacements["KICAD_PROJECT_DIR"] = projectDir
	return replacements
}

func expandKiCadVariables(uri string, _ string, replacements map[string]string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(uri); {
		if i+2 > len(uri) || uri[i:i+2] != "${" {
			b.WriteByte(uri[i])
			i++
			continue
		}
		end := strings.Index(uri[i+2:], "}")
		if end < 0 {
			return "", false
		}
		end += i + 2
		name := uri[i+2 : end]
		value, ok := replacements[name]
		if !ok {
			return "", false
		}
		b.WriteString(value)
		i = end + 1
	}
	return b.String(), true
}

func allowedLibraryRoots(projectDir string, opts Options, env map[string]string) []string {
	roots := []string{projectDir}
	if opts.ArtifactDir != "" {
		roots = append(roots, opts.ArtifactDir)
	}
	for key, value := range env {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.HasPrefix(key, "KICAD") || strings.HasPrefix(key, "KIPRJ") || strings.HasPrefix(key, "PRJ") {
			roots = append(roots, value)
		}
	}
	normalized := make([]string, 0, len(roots))
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err == nil {
			normalized = append(normalized, filepath.Clean(abs))
		}
	}
	return normalized
}

func environmentMap() map[string]string {
	values := map[string]string{}
	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func pathWithin(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func filesWithExt(dir string, ext string, issues *[]reports.Issue) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		*issues = append(*issues, BlockingIssue(reports.CodeValidationFailed, dir, err.Error()))
		return nil
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			matches = append(matches, filepath.Join(dir, entry.Name()))
		}
	}
	slices.Sort(matches)
	return matches
}

func selectSameBase(base string, paths []string) string {
	for _, path := range paths {
		if strings.EqualFold(baseNoExt(path), base) {
			return path
		}
	}
	return ""
}

func failedCheck(name string, path string, message string) CheckResult {
	return CheckResult{
		Name:     name,
		Required: true,
		Issues:   []reports.Issue{BlockingIssue(reports.CodeInvalidArgument, path, message)},
	}
}

func projectStructureSummary(target Target) string {
	parts := []string{}
	if target.ProjectPath != "" {
		parts = append(parts, "project")
	}
	if target.SchematicPath != "" {
		parts = append(parts, fmt.Sprintf("%d schematic file(s)", len(target.SchematicFiles)))
	}
	if target.PCBPath != "" {
		parts = append(parts, "pcb")
	}
	if len(parts) == 0 {
		return "no KiCad files resolved"
	}
	return "resolved " + strings.Join(parts, ", ")
}

func baseNoExt(path string) string {
	base := filepath.Base(filepath.FromSlash(path))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func replaceExt(path string, ext string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ext
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
