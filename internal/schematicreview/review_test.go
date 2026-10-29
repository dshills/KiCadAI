package schematicreview

import (
	"bytes"
	"context"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/schematicassistant"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchematicReviewReadOnlyPortableAndEscaped(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	session, err := schematicassistant.Example("divider")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = schematicassistant.Generate(context.Background(), session, source, schematicassistant.Options{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(source, "divider.kicad_sch")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "review")
	report, err := Write(context.Background(), source, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.SourceUnchanged || len(report.Sheets) != 1 || report.NativeERC != "not_checked" || len(report.Sheets[0].Parts) != 6 {
		t.Fatalf("report %+v", report)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source modified")
	}
	if err = Verify(out); err != nil {
		t.Fatal(err)
	}
	if err = schematicassistant.Verify(source); err != nil {
		t.Fatalf("source bundle changed during review: %v", err)
	}
	if _, err = Write(context.Background(), source, filepath.Join(source, "new", "review"), Options{}); err == nil {
		t.Fatal("allowed output in source")
	}
	alias := filepath.Join(root, "alias")
	if err = os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	if _, err = Write(context.Background(), source, filepath.Join(alias, "new", "report"), Options{}); err == nil {
		t.Fatal("symlink output bypassed read-only boundary")
	}
	if _, err = Write(context.Background(), source, out, Options{}); err == nil {
		t.Fatal("overwrote report")
	}
	report.RootSheet = `<script>alert(1)</script>`
	html, err := render(report)
	if err != nil || strings.Contains(string(html), report.RootSheet) {
		t.Fatal("unescaped HTML")
	}
	if err = os.WriteFile(filepath.Join(out, "source", "divider.kicad_sch"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = Verify(out); err == nil {
		t.Fatal("tamper accepted")
	}
}
func TestSchematicReviewHierarchyAndUnsafeSources(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("root.kicad_sch", `(kicad_sch (version 20260306) (generator "test") (sheet (at 1 1) (size 10 10) (uuid "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa") (property "Sheetname" "child" (at 1 1)) (property "Sheetfile" "child.kicad_sch" (at 1 2))))`)
	write("child.kicad_sch", `(kicad_sch (version 20260306) (generator "test"))`)
	report, files, err := Analyze(context.Background(), filepath.Join(source, "root.kicad_sch"), Options{})
	if err != nil || len(report.Sheets) != 2 || len(files) != 2 {
		t.Fatalf("hierarchy %v %+v", err, report)
	}
	write("root.kicad_sch", strings.ReplaceAll(string(files["source/root.kicad_sch"]), "child.kicad_sch", "../outside.kicad_sch"))
	if _, _, err = Analyze(context.Background(), filepath.Join(source, "root.kicad_sch"), Options{}); err == nil {
		t.Fatal("parent traversal accepted")
	}
	write("huge.kicad_sch", strings.Repeat(" ", evidencebundle.MaxFile+1))
	if _, _, err = Analyze(context.Background(), filepath.Join(source, "huge.kicad_sch"), Options{}); err == nil {
		t.Fatal("oversized source accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = Analyze(ctx, source, Options{}); err == nil {
		t.Fatal("cancel ignored")
	}
}
func TestSchematicReviewNativeOptIn(t *testing.T) {
	if os.Getenv("KICADAI_RUN_KICAD_CLI") != "1" {
		t.Skip("native KiCad opt-in")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	session, _ := schematicassistant.Example("divider")
	if _, err := schematicassistant.Generate(context.Background(), session, source, schematicassistant.Options{}); err != nil {
		t.Fatal(err)
	}
	report, err := Write(context.Background(), source, filepath.Join(root, "review"), Options{RequireERC: true})
	if err != nil || report.NativeERC != "checked" || !report.SourceUnchanged {
		t.Fatalf("native %v %+v", err, report)
	}
}

func TestSchematicReviewPurposeUsesExactDesignator(t *testing.T) {
	for _, ref := range []string{"CN1", "CR1", "RV1", "RT1", "RN1", "123", "日本語1"} {
		if !strings.HasPrefix(purpose(ref), "Purpose not inferred") {
			t.Fatalf("misleading purpose for %s: %s", ref, purpose(ref))
		}
	}
	if !strings.HasPrefix(purpose("R1"), "Resistor;") || !strings.HasPrefix(purpose("C1"), "Capacitor;") || !strings.HasPrefix(purpose("TP1"), "External connection") {
		t.Fatal("standard designators not described")
	}
}
