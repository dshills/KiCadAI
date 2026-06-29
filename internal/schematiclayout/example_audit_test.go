package schematiclayout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditExampleSchematicsFindsScopedExamples(t *testing.T) {
	audits, err := AuditExampleSchematics(repoRoot(t))
	if err != nil {
		t.Fatalf("AuditExampleSchematics returned error: %v", err)
	}
	if len(audits) != len(ScopedExampleDirectories) {
		t.Fatalf("audit count = %d, want %d", len(audits), len(ScopedExampleDirectories))
	}
	for index, directory := range ScopedExampleDirectories {
		if audits[index].Directory != directory {
			t.Fatalf("audit[%d].Directory = %s, want %s", index, audits[index].Directory, directory)
		}
		if audits[index].SchematicPath == "" {
			t.Fatalf("%s has no schematic path", directory)
		}
	}
}

func TestFormatExampleAuditMarkdownIsDeterministic(t *testing.T) {
	audits := []ExampleAudit{
		{Directory: "b", SymbolCount: 2},
		{Directory: "a", SymbolCount: 1},
	}
	got := FormatExampleAuditMarkdown(audits)
	first := strings.Index(got, "`a`")
	second := strings.Index(got, "`b`")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("audit markdown not sorted:\n%s", got)
	}
}

func TestWriteExampleAuditMarkdownWhenRequested(t *testing.T) {
	if os.Getenv("KICADAI_WRITE_SCHEMATIC_EXAMPLE_AUDIT") != "1" {
		t.Skip("set KICADAI_WRITE_SCHEMATIC_EXAMPLE_AUDIT=1 to refresh audit markdown")
	}
	root := repoRoot(t)
	audits, err := AuditExampleSchematics(root)
	if err != nil {
		t.Fatalf("AuditExampleSchematics returned error: %v", err)
	}
	path := filepath.Join(root, "specs", "schematic-example-readability", "AUDIT.md")
	if err := os.WriteFile(path, []byte(FormatExampleAuditMarkdown(audits)), 0o644); err != nil {
		t.Fatalf("write audit markdown: %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatalf("repo root not found from %s", root)
		}
		root = next
	}
}
