package externalkey

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateReadAndNoOverwrite(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	path := filepath.Join(external, "held-out.key")
	key, err := Create(root, path, bytes.NewReader(bytes.Repeat([]byte{0x5a}, Size)))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != Size {
		t.Fatalf("key size = %d", len(key))
	}
	loaded, err := Read(root, path)
	if err != nil || !bytes.Equal(loaded, key) {
		t.Fatalf("read key = %x, %v", loaded, err)
	}
	if _, err := Create(root, path, bytes.NewReader(bytes.Repeat([]byte{0x6b}, Size))); err == nil {
		t.Fatal("external key creation overwrote an existing key")
	}
}

func TestRejectsRepositoryPathSymlinkAndWeakMode(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "key")
	if _, err := Create(root, inside, bytes.NewReader(bytes.Repeat([]byte{1}, Size))); err == nil {
		t.Fatal("external key creation accepted a repository path")
	}
	external := t.TempDir()
	real := filepath.Join(external, "real")
	if err := os.WriteFile(real, bytes.Repeat([]byte{2}, Size), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(external, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, link); err == nil {
		t.Fatal("external key reader accepted a symlink")
	}
	if err := os.Chmod(real, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, real); err == nil {
		t.Fatal("external key reader accepted weak permissions")
	}
}

func TestDistinctUsesCanonicalPaths(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	one := filepath.Join(external, "one")
	if err := Distinct(root, one, filepath.Join(external, ".", "one")); err == nil {
		t.Fatal("distinct key validation accepted one canonical path twice")
	}
}

func TestCreateFailsClosedOnShortRandomnessAndRemoveRejectsRepository(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	path := filepath.Join(external, "short.key")
	if _, err := Create(root, path, bytes.NewReader([]byte{1})); err == nil {
		t.Fatal("external key creation accepted short randomness")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("short randomness left an external key")
	}
	inside := filepath.Join(root, "key")
	if err := os.WriteFile(inside, bytes.Repeat([]byte{1}, Size), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, inside); err == nil {
		t.Fatal("external key removal accepted a repository path")
	}
	if _, err := os.Stat(inside); err != nil {
		t.Fatal("rejected repository-key removal changed the file")
	}
}
