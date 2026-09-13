package boardfamily

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUsageAnomalyPersistsHalt(t *testing.T) {
	for _, tokens := range []int{200_000, -1, 2_000_000} {
		path := filepath.Join(t.TempDir(), "ledger.json")
		n, err := reserve(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := finishReservation(path, n, "completed", "anomaly", tokens, 1); err == nil {
			t.Fatal("usage anomaly accepted")
		}
		if _, err := reserve(path); err == nil {
			t.Fatal("new request allowed after usage anomaly")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var l Ledger
		if err := json.Unmarshal(b, &l); err != nil || l.HaltReason == "" || len(l.Entries) != 1 || l.Entries[0].InputTokens != tokens {
			t.Fatalf("lost anomaly evidence: %s (%v)", b, err)
		}
	}
}

func TestLedgerBoundsAndUnknownRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	for i := 1; i <= MaxLiveRequests; i++ {
		n, e := reserve(path)
		if e != nil || n != i {
			t.Fatalf("reserve %d: %d %v", i, n, e)
		}
	}
	if _, e := reserve(path); e == nil {
		t.Fatal("overran count")
	}
	if e := finishReservation(path, 1, "completed", "response-test", 1000, 500); e != nil {
		t.Fatal(e)
	}
	if _, e := reserve(path); e == nil {
		t.Fatal("completion incorrectly replenished request budget")
	}
	if e := finishReservation(path, 1, "completed", "repeat", 1, 1); e == nil {
		t.Fatal("duplicate settlement")
	}
}
func TestLedgerCorruptionFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if e := os.WriteFile(path, []byte("bad json"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := reserve(path); e == nil {
		t.Fatal("reset corrupt ledger")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "bad json" {
		t.Fatal("modified corrupt evidence")
	}
}
func TestLedgerLockFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if e := os.Mkdir(path+".lock", 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := reserve(path); e == nil {
		t.Fatal("ignored active lock")
	}
}
