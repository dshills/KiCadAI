package libraryresolver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/reports"
)

func TestResolveRootsUsesEnvironment(t *testing.T) {
	klc := t.TempDir()
	symbols := t.TempDir()
	footprints := t.TempDir()
	templates := t.TempDir()
	t.Setenv(EnvKLCRoot, klc)
	t.Setenv(EnvSymbolsRoot, symbols)
	t.Setenv(EnvFootprintsRoot, footprints)
	t.Setenv(EnvTemplatesRoot, templates)

	roots, issues := ResolveRoots()
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %#v", issues)
	}
	if roots.KLCRoot != klc || roots.SymbolsRoot != symbols || roots.FootprintsRoot != footprints || roots.TemplatesRoot != templates {
		t.Fatalf("roots = %#v", roots)
	}
}

func TestValidateRootsReportsMissingAsWarnings(t *testing.T) {
	issues := ValidateRoots(LibraryRoots{})
	if len(issues) != 4 {
		t.Fatalf("issues = %d, want 4: %#v", len(issues), issues)
	}
	for _, issue := range issues {
		if issue.Code != reports.CodeMissingFile || issue.Severity != reports.SeverityWarning {
			t.Fatalf("unexpected issue: %#v", issue)
		}
	}
}

func TestValidateRootsRejectsFilesAndAllowsDirectorySymlinks(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	issues := ValidateRoots(LibraryRoots{SymbolsRoot: file})
	if !hasIssue(issues, "roots.symbols_root", reports.CodeInvalidArgument) {
		t.Fatalf("expected file issue: %#v", issues)
	}
	target := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unsupported: %v", err)
	}
	issues = ValidateRoots(LibraryRoots{FootprintsRoot: link})
	if hasIssue(issues, "roots.footprints_root", reports.CodeInvalidArgument) {
		t.Fatalf("expected directory symlink to be accepted: %#v", issues)
	}
}

func TestValidateCachePathRejectsTraversal(t *testing.T) {
	issues := ValidateCachePath(filepath.Join("..", "cache.json"))
	if len(issues) != 1 || issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("expected traversal issue: %#v", issues)
	}
	issues = ValidateCachePath(`C:..\cache.json`)
	if len(issues) != 1 || issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("expected volume-relative traversal issue: %#v", issues)
	}
	issues = ValidateCachePath(t.TempDir())
	if len(issues) != 1 || issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("expected directory issue: %#v", issues)
	}
	if issues := ValidateCachePath(filepath.Join(t.TempDir(), "cache.json")); len(issues) != 0 {
		t.Fatalf("unexpected valid cache path issue: %#v", issues)
	}
}

func TestLibraryIndexJSONShape(t *testing.T) {
	index := LibraryIndex{
		GeneratedAt: time.Unix(1, 0).UTC(),
		Roots:       LibraryRoots{SymbolsRoot: "/symbols"},
		Symbols: map[string]SymbolRecord{
			"Device:R": {LibraryID: "Device:R", LibraryNickname: "Device", Name: "R"},
		},
		Footprints: map[string]FootprintRecord{
			"Resistor_SMD:R_0805_2012Metric": {FootprintID: "Resistor_SMD:R_0805_2012Metric", LibraryNickname: "Resistor_SMD", Name: "R_0805_2012Metric"},
		},
		Diagnostics: []reports.Issue{},
	}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"generated_at"`, `"symbols_root"`, `"library_id"`, `"footprint_id"`, `"diagnostics"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("JSON missing %q: %s", want, text)
		}
	}
}

func hasIssue(issues []reports.Issue, path string, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Path == path && issue.Code == code {
			return true
		}
	}
	return false
}
