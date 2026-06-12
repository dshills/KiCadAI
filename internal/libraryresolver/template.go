package libraryresolver

import (
	"cmp"
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

func DiscoverTemplates(ctx context.Context, roots LibraryRoots) ([]TemplateRecord, []reports.Issue) {
	root := strings.TrimSpace(roots.TemplatesRoot)
	if root == "" {
		return nil, []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityWarning,
			Path:     "roots.templates_root",
			Message:  "templates root is not configured",
		}}
	}
	if issues := validateRoot("roots.templates_root", "templates root", root); len(issues) != 0 {
		return nil, issues
	}
	templateDirs := map[string]struct{}{}
	var issues []reports.Issue
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			issues = append(issues, discoveryIssue(path, err))
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".kicad_pro") {
			templateDirs[filepath.Dir(path)] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		issues = append(issues, discoveryIssue(root, walkErr))
	}
	records := make([]TemplateRecord, 0, len(templateDirs))
	for dir := range templateDirs {
		record, recordIssues := indexTemplateDir(ctx, dir, templateDirs)
		issues = append(issues, recordIssues...)
		if record.Name != "" {
			records = append(records, record)
		}
	}
	slices.SortFunc(records, func(a, b TemplateRecord) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Path, b.Path))
	})
	return records, issues
}

func ResolveTemplate(records []TemplateRecord, name string) (TemplateRecord, bool) {
	name = strings.TrimSpace(name)
	for _, record := range records {
		if record.Name == name {
			return record, true
		}
	}
	return TemplateRecord{}, false
}

func DiscoverTemplate(ctx context.Context, roots LibraryRoots, name string) (TemplateRecord, []reports.Issue, bool) {
	root := strings.TrimSpace(roots.TemplatesRoot)
	if root == "" {
		return TemplateRecord{}, []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityWarning,
			Path:     "roots.templates_root",
			Message:  "templates root is not configured",
		}}, false
	}
	if issues := validateRoot("roots.templates_root", "templates root", root); len(issues) != 0 {
		return TemplateRecord{}, issues, false
	}
	name = strings.TrimSpace(name)
	var issues []reports.Issue
	var templateDir string
	duplicateName := false
	templateDirs := map[string]struct{}{}
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			issues = append(issues, discoveryIssue(path, err))
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".kicad_pro") {
			dir := filepath.Dir(path)
			templateDirs[dir] = struct{}{}
			if filepath.Base(dir) == name {
				if templateDir != "" && templateDir != dir {
					duplicateName = true
				}
				templateDir = dir
			}
		}
		return nil
	})
	if walkErr != nil {
		issues = append(issues, discoveryIssue(root, walkErr))
	}
	if templateDir == "" {
		return TemplateRecord{}, issues, false
	}
	if duplicateName {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "library.template." + name,
			Message:  "multiple templates match name " + name,
		})
		return TemplateRecord{}, issues, false
	}
	record, recordIssues := indexTemplateDir(ctx, templateDir, templateDirs)
	issues = append(issues, recordIssues...)
	return record, issues, true
}

func indexTemplateDir(ctx context.Context, dir string, templateDirs map[string]struct{}) (TemplateRecord, []reports.Issue) {
	record := TemplateRecord{Name: filepath.Base(dir), Path: filepath.ToSlash(dir)}
	var issues []reports.Issue
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			issues = append(issues, discoveryIssue(path, err))
			return nil
		}
		if entry.IsDir() {
			if _, ok := templateDirs[path]; path != dir && ok {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			issues = append(issues, discoveryIssue(path, err))
			return nil
		}
		rel = filepath.ToSlash(rel)
		switch {
		case strings.EqualFold(filepath.Ext(entry.Name()), ".kicad_pro"):
			record.ProjectFiles = append(record.ProjectFiles, rel)
		case strings.EqualFold(filepath.Ext(entry.Name()), ".kicad_sch"):
			record.SchematicFiles = append(record.SchematicFiles, rel)
		case strings.EqualFold(filepath.Ext(entry.Name()), ".kicad_pcb"):
			record.BoardFiles = append(record.BoardFiles, rel)
		case rel == "fp-lib-table" || rel == "sym-lib-table":
			record.LibraryTables = append(record.LibraryTables, rel)
		case strings.HasPrefix(rel, "meta/"):
			record.MetadataFiles = append(record.MetadataFiles, rel)
		default:
			record.OtherFiles = append(record.OtherFiles, rel)
		}
		return nil
	})
	if err != nil {
		issues = append(issues, discoveryIssue(dir, err))
	}
	sortTemplateRecord(&record)
	return record, issues
}

func sortTemplateRecord(record *TemplateRecord) {
	slices.Sort(record.ProjectFiles)
	slices.Sort(record.MetadataFiles)
	slices.Sort(record.LibraryTables)
	slices.Sort(record.SchematicFiles)
	slices.Sort(record.BoardFiles)
	slices.Sort(record.OtherFiles)
}
