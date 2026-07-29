package transactions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFableH6ReproductionBackupStepRemovesLiveImportedFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "demo.kicad_pcb")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged := stagedImportedProjectWrite{target: target}
	if err := backupImportedProjectTarget(&staged); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("live target remains present after backup step: %v", err)
	}
	if staged.backup == "" {
		t.Fatal("backup path was not recorded")
	}
	restoreImportedProjectBackups([]stagedImportedProjectWrite{staged})
	restored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(restored) != "original" {
		t.Fatalf("restored target = %q, want original", restored)
	}
}

func TestFableH7ReproductionReleaseRemovesReplacementLock(t *testing.T) {
	root := t.TempDir()
	release, err := AcquireProjectApplyLock(root)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, ApplyLockFileName)
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("pid=999999\nowner=replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("release retained a replacement owner's lock: %v", err)
	}
}
