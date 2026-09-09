package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCaseIDsAreExplicitUniqueAndCanonical(t *testing.T) {
	ids, err := caseIDs("case-b,case-a")
	if err != nil || !reflect.DeepEqual(ids, []string{"case-a", "case-b"}) {
		t.Fatalf("ids %v: %v", ids, err)
	}
	for _, text := range []string{"", "a,a", "a,", "../a", "a/b", "a\\b", "a, b", ".", ".."} {
		if _, err := caseIDs(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}

func TestBaselineAuthenticationRejectsTamper(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v21_maintenance_1", "report.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := decodeBaseline(data)
	if err != nil || baseline.CaseCount != 24 {
		t.Fatalf("baseline %v", err)
	}
	if _, err := decodeBaseline(append(data, []byte("{}")...)); err == nil {
		t.Fatal("trailing data accepted")
	}
	if _, err := decodeBaseline([]byte(`{"schema":"tampered"}`)); err == nil {
		t.Fatal("invalid baseline accepted")
	}
	baseline.Cases[0].ReplaySHA256 = nil
	withoutReplays, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeBaseline(withoutReplays); err == nil {
		t.Fatal("empty replay evidence accepted before indexed access")
	}
}

func TestPublicationIsNoReplace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "trace.json")
	if err := writeNew(path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(path, []byte("replacement")); err == nil {
		t.Fatal("existing output replaced")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatal("existing data changed")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary output leaked: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o444 {
		t.Fatal("output not sealed read-only")
	}
}

func TestInvalidInvocationDoesNotCreateOutput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "output")
	for _, args := range [][]string{
		{"--output-root", root},
		{"--output-root", root, "--cases", "a", "--timeout", "0s"},
		{"--output-root", "relative", "--cases", "a"},
		{"--output-root", root, "--cases", "a", "extra"},
	} {
		if err := run(context.Background(), args, io.Discard); err == nil {
			t.Fatal("invalid invocation accepted")
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("invalid invocation created output")
		}
	}
}
