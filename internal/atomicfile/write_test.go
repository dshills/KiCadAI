package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesExistingAndAppliesMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o640); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%v, want 0640", info.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".atomic-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestWriteCleansTemporaryFileWhenReplacementFails(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "destination")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Write(destination, []byte("replacement"), 0o640); err == nil {
		t.Fatal("Write succeeded while replacing a directory")
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("failed replacement changed destination: mode=%v", info.Mode())
	}
	matches, err := filepath.Glob(filepath.Join(root, ".atomic-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain after failed replacement: %v", matches)
	}
}
