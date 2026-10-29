package evidencebundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEvidencePublishIntegrityAndSafety(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bundle")
	files := map[string][]byte{"input.json": []byte("{}"), "nested/result.txt": []byte("evidence")}
	if err := Publish(root, files); err != nil {
		t.Fatal(err)
	}
	if err := Verify(root); err != nil {
		t.Fatal(err)
	}
	if err := Publish(root, files); err == nil {
		t.Fatal("overwrote")
	}
	for _, name := range []string{"../escape", "/absolute", "manifest.json", "nested/../escape", `..\escape`, `C:\absolute`} {
		if err := Publish(filepath.Join(t.TempDir(), "bad"), map[string][]byte{name: []byte("x")}); err == nil {
			t.Fatal("accepted path", name)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "extra"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(root); err == nil {
		t.Fatal("extra file accepted")
	}
	if err := os.Remove(filepath.Join(root, "extra")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "input.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nested/result.txt"), filepath.Join(root, "input.json")); err != nil {
		t.Fatal(err)
	}
	if err := Verify(root); err == nil {
		t.Fatal("symlink accepted")
	}
	var value map[string]string
	if err := Decode([]byte("{} {}"), &value); err == nil {
		t.Fatal("trailing data accepted")
	}
	if err := Publish(filepath.Join(t.TempDir(), "large"), map[string][]byte{"large": make([]byte, MaxFile+1)}); err == nil {
		t.Fatal("oversize artifact accepted")
	}
}

func TestEvidenceFileLimitIncludesManifest(t *testing.T) {
	files := map[string][]byte{}
	for i := 0; i < MaxFiles-1; i++ {
		files[fmt.Sprintf("file-%03d", i)] = []byte("bounded")
	}
	root := filepath.Join(t.TempDir(), "maximum")
	if err := Publish(root, files); err != nil {
		t.Fatal(err)
	}
	if err := Verify(root); err != nil {
		t.Fatal(err)
	}
	files["one-too-many"] = []byte("x")
	if err := Publish(filepath.Join(t.TempDir(), "overflow"), files); err == nil {
		t.Fatal("accepted 257 files including manifest")
	}
}
