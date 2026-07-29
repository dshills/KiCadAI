package atomicfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLockRejectsConcurrentHolderAndOwnerRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apply.lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(path); err == nil {
		t.Fatal("second holder acquired a live lock")
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("lock was not reusable after owner release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestLockReleaseDoesNotRemoveReplacementOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apply.lock")
	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// Reuse the token deliberately: inode ownership must still prevent an old
	// handle from removing a path that was replaced behind it.
	replacement := lockRecord{Token: lock.token, PID: os.Getpid(), ProcessStart: processStartIdentity(os.Getpid())}
	raw, err := json.Marshal(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("replacement owner was removed: %v", err)
	}
	if string(after) != string(append(raw, '\n')) {
		t.Fatalf("replacement lock changed: %q", after)
	}
}

func TestLockTakesOverStaleTokenCheckedOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apply.lock")
	raw, err := json.Marshal(lockRecord{Token: "stale-token", PID: 999999})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("stale lock was not taken over: %v", err)
	}
	if lock.token == "stale-token" {
		t.Fatal("takeover reused stale ownership token")
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}
