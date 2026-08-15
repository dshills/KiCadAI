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

func TestVerifyRepositorySourceManifestAllowsContainedParentTraversal(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "internal", "publisher source.go")
	manifest := filepath.Join(root, "specs", "freeze", "publisher.sha256")
	writeTestFile(t, source, []byte("package internal\n"))
	digest := hashBytes([]byte("package internal\n"))
	writeTestFile(t, manifest, []byte(digest+"  ../../internal/publisher source.go\n"))
	if err := verifyRepositorySourceManifest(root, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRepositorySourceManifestRejectsEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	outside := filepath.Join(parent, "outside.go")
	manifest := filepath.Join(root, "specs", "publisher.sha256")
	writeTestFile(t, outside, []byte("outside\n"))
	digest := hashBytes([]byte("outside\n"))
	writeTestFile(t, manifest, []byte(digest+"  ../../outside.go\n"))
	if err := verifyRepositorySourceManifest(root, manifest); err == nil || !strings.Contains(err.Error(), "escapes repository root") {
		t.Fatalf("error=%v", err)
	}
}

func TestVerifyRepositorySourceManifestRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	link := filepath.Join(root, "source-link.go")
	manifest := filepath.Join(root, "publisher.sha256")
	writeTestFile(t, source, []byte("source\n"))
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	digest := hashBytes([]byte("source\n"))
	writeTestFile(t, manifest, []byte(digest+"  source-link.go\n"))
	if err := verifyRepositorySourceManifest(root, manifest); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("error=%v", err)
	}
}

func TestVerifyRepositorySourceManifestRejectsMismatch(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	manifest := filepath.Join(root, "publisher.sha256")
	writeTestFile(t, source, []byte("source\n"))
	writeTestFile(t, manifest, []byte(strings.Repeat("0", 64)+"  source.go\n"))
	if err := verifyRepositorySourceManifest(root, manifest); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error=%v", err)
	}
}

func writeTestFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
