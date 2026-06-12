package libraryresolver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestDiscoverTemplatesIndexesFixtureDirectory(t *testing.T) {
	root := templateTestRoot(t)
	records, issues := DiscoverTemplates(context.Background(), LibraryRoots{TemplatesRoot: root})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	record := records[0]
	if record.Name != "Demo" || len(record.ProjectFiles) != 1 || len(record.SchematicFiles) != 1 || len(record.BoardFiles) != 1 || len(record.LibraryTables) != 2 || len(record.MetadataFiles) != 1 {
		t.Fatalf("record = %#v", record)
	}
	resolved, ok := ResolveTemplate(records, "Demo")
	if !ok || resolved.Name != "Demo" {
		t.Fatalf("resolved = %#v/%v", resolved, ok)
	}
}

func TestDiscoverTemplatesMissingRootWarning(t *testing.T) {
	records, issues := DiscoverTemplates(context.Background(), LibraryRoots{})
	if len(records) != 0 {
		t.Fatalf("records = %#v", records)
	}
	if len(issues) != 1 || issues[0].Path != "roots.templates_root" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDiscoverTemplatesSortsDeterministically(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "Projects", "Zed", "Zed.kicad_pro"), `{}`)
	mustWrite(t, filepath.Join(root, "Projects", "Alpha", "Alpha.kicad_pro"), `{}`)
	records, issues := DiscoverTemplates(context.Background(), LibraryRoots{TemplatesRoot: root})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(records) != 2 || records[0].Name != "Alpha" || records[1].Name != "Zed" {
		t.Fatalf("records = %#v", records)
	}
}

func TestDiscoverTemplatesDoesNotIncludeNestedProjectFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "Projects", "Outer", "Outer.kicad_pro"), `{}`)
	mustWrite(t, filepath.Join(root, "Projects", "Outer", "Nested", "Nested.kicad_pro"), `{}`)
	mustWrite(t, filepath.Join(root, "Projects", "Outer", "Nested", "Nested.kicad_sch"), `(kicad_sch)`)
	records, issues := DiscoverTemplates(context.Background(), LibraryRoots{TemplatesRoot: root})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	outer, ok := ResolveTemplate(records, "Outer")
	if !ok {
		t.Fatalf("records = %#v", records)
	}
	if len(outer.OtherFiles) != 0 || len(outer.SchematicFiles) != 0 {
		t.Fatalf("outer included nested files: %#v", outer)
	}
	nested, issues, ok := DiscoverTemplate(context.Background(), LibraryRoots{TemplatesRoot: root}, "Nested")
	if !ok || len(issues) != 0 || nested.Name != "Nested" {
		t.Fatalf("nested = %#v issues=%#v ok=%v", nested, issues, ok)
	}
}

func TestDiscoverTemplateRejectsAmbiguousName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "VendorA", "Demo", "Demo.kicad_pro"), `{}`)
	mustWrite(t, filepath.Join(root, "VendorB", "Demo", "Demo.kicad_pro"), `{}`)
	_, issues, ok := DiscoverTemplate(context.Background(), LibraryRoots{TemplatesRoot: root}, "Demo")
	if ok || !hasTemplateIssue(issues, "multiple templates match name Demo") {
		t.Fatalf("expected ambiguous template diagnostic: issues=%#v ok=%v", issues, ok)
	}
}

func templateTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "Projects", "Demo")
	mustWrite(t, filepath.Join(project, "Demo.kicad_pro"), `{}`)
	mustWrite(t, filepath.Join(project, "Demo.kicad_sch"), `(kicad_sch)`)
	mustWrite(t, filepath.Join(project, "Demo.kicad_pcb"), `(kicad_pcb)`)
	mustWrite(t, filepath.Join(project, "fp-lib-table"), ``)
	mustWrite(t, filepath.Join(project, "sym-lib-table"), ``)
	mustWrite(t, filepath.Join(project, "meta", "info.html"), `<p>demo</p>`)
	return root
}

func hasTemplateIssue(issues []reports.Issue, contains string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, contains) {
			return true
		}
	}
	return false
}
