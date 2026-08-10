package corpuspublication

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyChecksumManifestFailsClosed(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(root, "artifact.txt")
	data := []byte("frozen bytes\n")
	if err := os.WriteFile(artifact, data, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "MANIFEST.sha256")
	manifest := []byte(fmt.Sprintf("%s  artifact.txt\n", hashBytes(data)))
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyChecksumManifest(root, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(verified) != string(manifest) {
		t.Fatal("verified manifest bytes changed")
	}

	if err := os.WriteFile(artifact, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyChecksumManifest(root, manifestPath); err == nil || !strings.Contains(err.Error(), "commitment") {
		t.Fatalf("changed artifact error = %v", err)
	}
	if err := os.WriteFile(artifact, data, 0o644); err != nil {
		t.Fatal(err)
	}
	duplicate := append(append([]byte(nil), manifest...), manifest...)
	if err := os.WriteFile(manifestPath, duplicate, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyChecksumManifest(root, manifestPath); err == nil || !strings.Contains(err.Error(), "invalid entry") {
		t.Fatalf("duplicate manifest error = %v", err)
	}
}

func TestVerifyChecksumManifestRejectsSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	data := []byte("target\n")
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	manifestPath := filepath.Join(root, "MANIFEST.sha256")
	if err := os.WriteFile(manifestPath, []byte(fmt.Sprintf("%s  link.txt\n", hashBytes(data))), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyChecksumManifest(root, manifestPath); err == nil || !strings.Contains(err.Error(), "symbolic") {
		t.Fatalf("symlink entry error = %v", err)
	}
}
