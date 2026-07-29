package transactions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/atomicfile"
)

func TestFableH6ImportedCommitNeverRemovesLiveFileBeforeReplacement(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "demo.kicad_pcb")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := atomicfile.BeginGroup([]atomicfile.Mutation{
		{Path: target, Data: []byte("replacement"), Mode: 0o644},
	}, atomicfile.GroupOptions{
		Root:            root,
		PreserveOnError: true,
		Fault: func(transition atomicfile.Transition) error {
			if transition == atomicfile.TransitionReplacement {
				return errors.New("simulated process interruption")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected injected interruption")
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Fatalf("live target was missing at an observable commit boundary: %v", statErr)
	}
	release, err := AcquireProjectApplyLock(root)
	if err != nil {
		t.Fatal(err)
	}
	release()
	restored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(restored) != "original" {
		t.Fatalf("restored target = %q, want original", restored)
	}
}

func TestFableH7ReleaseCannotRemoveReplacementOwnerLock(t *testing.T) {
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
	replacement, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("release removed a replacement owner's lock: %v", err)
	}
	if string(replacement) != "pid=999999\nowner=replacement\n" {
		t.Fatalf("replacement lock changed: %q", replacement)
	}
}
