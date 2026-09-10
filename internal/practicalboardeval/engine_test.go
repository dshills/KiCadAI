package practicalboardeval

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

func TestLibraryIdentityIgnoresOnlyCollectionTime(t *testing.T) {
	first := libraryresolver.LibraryIndex{GeneratedAt: time.Unix(1, 0)}
	second := first
	second.GeneratedAt = time.Unix(2, 0)
	a, err := LibraryIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LibraryIdentity(second)
	if err != nil || a != b {
		t.Fatalf("collection clock changed identity: %v", err)
	}
	second.Roots.SymbolsRoot = "different-library"
	b, err = LibraryIdentity(second)
	if err != nil || a == b {
		t.Fatalf("library change escaped identity: %v", err)
	}
}

// This pre-freeze plumbing control is NOT a member of the new corpus and is
// never counted as a new board. It reads an existing regression requirement
// without modifying any historical input or publication artifact.
func TestEvaluatorOptionalNativePlumbingControl(t *testing.T) {
	if os.Getenv("KICADAI_PRACTICAL_EVAL_SELFTEST") != "1" {
		t.Skip("explicit native evaluator plumbing control only")
	}
	cli := os.Getenv("KICADAI_KICAD_CLI")
	if cli == "" {
		t.Fatal("KiCad CLI required")
	}
	data, err := os.ReadFile(filepath.Join("..", "architecturesearch", "testdata", "power_interface_synthesis_corpus", "regulated_mcu_sensor_subsystem.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	root := t.TempDir()
	if configured := os.Getenv("KICADAI_PRACTICAL_EVAL_SELFTEST_OUTPUT"); configured != "" {
		if err := os.Mkdir(configured, 0o700); err != nil {
			t.Fatal(err)
		}
		root = configured
	}
	var identity string
	for _, name := range []string{"first", "second"} {
		input, issues := architecturesearch.DecodeStrict(bytes.NewReader(data))
		if reports.HasBlockingIssue(issues) {
			t.Fatal(issues)
		}
		e, err := loadEngine(ctx)
		if err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(root, name)
		if err := os.Mkdir(output, 0o700); err != nil {
			t.Fatal(err)
		}
		got, failed, err := e.downstream(ctx, input, output, cli)
		if err != nil || failed != "" {
			t.Fatalf("plumbing control gate=%s err=%v artifacts=%s", failed, err, root)
		}
		if identity != "" && got != identity {
			t.Fatalf("normalization mismatch: %s != %s; artifacts=%s", got, identity, root)
		}
		identity = got
	}
}
