package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresInputs(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{"--report", "report.json"}, &out)
	if err == nil || !strings.Contains(err.Error(), "--report, --binding, and --destination are required") {
		t.Fatalf("error=%v", err)
	}
}
func TestRunRejectsPositionalArguments(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{"unexpected"}, &out)
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("error=%v", err)
	}
}

func TestCleanRepositoryAllowanceHandlesUnusualDestinationPaths(t *testing.T) {
	root := t.TempDir()
	git := func(arguments ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
	}
	git("init", "-q")
	git("config", "user.email", "fixture@example.invalid")
	git("config", "user.name", "Fixture")
	if err := os.WriteFile(filepath.Join(root, "tracked"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "tracked")
	git("commit", "-q", "-m", "base")
	destination := filepath.Join(root, "baseline ü space")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "report ü.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireCleanRepositoryAllowing(context.Background(), root, destination); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireCleanRepositoryAllowing(context.Background(), root, destination); err == nil {
		t.Fatal("unrelated untracked file accepted")
	}
}
