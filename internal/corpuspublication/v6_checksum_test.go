package corpuspublication

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyV6ContractManifestAllowsConfinedParentPaths(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "specs", "contract")
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	local := []byte("local\n")
	internal := []byte("internal\n")
	if err := os.WriteFile(filepath.Join(directory, "local.txt"), local, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "code.go"), internal, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(directory, "V6_CONTRACT.sha256")
	data := []byte(fmt.Sprintf("%s  local.txt\n%s  ../../internal/code.go\n", hashBytes(local), hashBytes(internal)))
	if err := os.WriteFile(manifest, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := VerifyV6ContractManifest(root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatal("V6 contract manifest bytes changed")
	}
}

func TestVerifyV6ContractManifestRejectsEscapesAliasesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "specs", "contract")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceBytes := []byte("source\n")
	source := filepath.Join(directory, "source.txt")
	if err := os.WriteFile(source, sourceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	outsideBytes := []byte("outside\n")
	outside := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-outside.txt")
	if err := os.WriteFile(outside, outsideBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	symlink := filepath.Join(directory, "link.txt")
	if err := os.Symlink(source, symlink); err != nil {
		t.Fatal(err)
	}

	for name, test := range map[string]struct {
		entry  string
		digest string
	}{
		"escape":       {"../../../" + filepath.Base(outside), hashBytes(outsideBytes)},
		"absolute":     {source, hashBytes(sourceBytes)},
		"noncanonical": {"sub/../source.txt", hashBytes(sourceBytes)},
		"symlink":      {"link.txt", hashBytes(sourceBytes)},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := filepath.Join(directory, name+".sha256")
			data := []byte(fmt.Sprintf("%s  %s\n", test.digest, filepath.ToSlash(test.entry)))
			if err := os.WriteFile(manifest, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyV6ContractManifest(root, manifest); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	manifest := filepath.Join(directory, "alias.sha256")
	line := fmt.Sprintf("%s  source.txt\n", hashBytes(sourceBytes))
	if err := os.WriteFile(manifest, []byte(line+line), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyV6ContractManifest(root, manifest); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestHashV6ContractFileRejectsIdentityChangeBeforeRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := hashV6ContractFile(path, expected, v6ContractMaximumFileSize); err == nil {
		t.Fatal("expected identity-change error")
	}
}
