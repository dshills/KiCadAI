package atomicdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishCommitsCompleteTreeAndRefusesOverwrite(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "published")
	if err := Publish(destination, func(root string) error {
		if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, "nested", "evidence.json"), []byte("evidence"), 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "nested", "evidence.json"))
	if err != nil || string(data) != "evidence" {
		t.Fatalf("published data = %q, %v", data, err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("published root mode = %v; want 0755", info.Mode().Perm())
	}
	if err := Publish(destination, func(string) error { return nil }); err == nil {
		t.Fatal("atomic directory publication overwrote an existing destination")
	}
}

func TestPublishCleansFailedStagingTree(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "published")
	wantErr := os.ErrInvalid
	if err := Publish(destination, func(root string) error {
		if err := os.WriteFile(filepath.Join(root, "partial"), []byte("partial"), 0o644); err != nil {
			return err
		}
		return wantErr
	}); err != wantErr {
		t.Fatalf("publish error = %v, want %v", err, wantErr)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed publication left entries %#v: %v", entries, err)
	}
}

func TestPublishRejectsSymlinkedContent(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "published")
	if err := Publish(destination, func(root string) error {
		return os.Symlink(filepath.Join(parent, "outside"), filepath.Join(root, "link"))
	}); err == nil {
		t.Fatal("atomic directory publication accepted symbolic content")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("failed symbolic publication created a destination")
	}
}
