package boardfamily

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evaluationPolicy() LedgerPolicy { return LedgerPolicy{"board-family-v2-final-01", 14, 1_000_000} }
func ledgerBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func readTestLedger(t *testing.T, path string) Ledger {
	t.Helper()
	var l Ledger
	if err := json.Unmarshal(ledgerBytes(t, path), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestDecodeLedgerPolicyStrict(t *testing.T) {
	good := `{"goal":"board-family-v2-final-01","max_requests":14,"max_micro_usd":1000000}`
	p, err := DecodeLedgerPolicy(strings.NewReader(good))
	if err != nil || p != evaluationPolicy() {
		t.Fatalf("policy: %+v %v", p, err)
	}
	for _, bad := range []string{
		`{}`, `null`, `[]`, good + ` {}`, good + ` garbage`, strings.Repeat(" ", 4097),
		strings.Replace(good, `"max_requests":14`, `"max_requests":0`, 1),
		strings.Replace(good, `"max_requests":14`, `"max_requests":42`, 1),
		strings.Replace(good, `"max_requests":14`, `"max_requests":14.5`, 1),
		strings.Replace(good, `"max_requests":14`, `"max_requests":null`, 1),
		strings.Replace(good, `"max_requests":14`, `"max_requests":14,"max_requests":41`, 1),
		strings.Replace(good, `"max_micro_usd":1000000`, `"max_micro_usd":49999`, 1),
		strings.Replace(good, `"max_micro_usd":1000000`, `"max_micro_usd":10000001`, 1),
		strings.Replace(good, `"max_micro_usd":1000000`, `"max_micro_usd":9223372036854775808`, 1),
		strings.Replace(good, `"goal":"board-family-v2-final-01"`, `"goal":""`, 1),
		strings.Replace(good, `board-family-v2-final-01`, ledgerGoal, 1),
		strings.Replace(good, `board-family-v2-final-01`, `../escape`, 1),
		strings.Replace(good, `board-family-v2-final-01`, strings.Repeat("x", 101), 1),
		strings.Replace(good, `"max_micro_usd":1000000`, `"cost":1000000`, 1),
		strings.TrimSuffix(good, "}"),
	} {
		if _, err := DecodeLedgerPolicy(strings.NewReader(bad)); err == nil {
			t.Fatalf("accepted invalid policy: %.150s", bad)
		}
	}
}

func TestV2TransportReservationsEnforceCountAndNoRefund(t *testing.T) {
	p := evaluationPolicy()
	path := filepath.Join(t.TempDir(), "v2.json")
	calls := 0
	for i := 1; i <= 15; i++ {
		tr := &reservedTransport{Path: path, Policy: p, Base: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			l := readTestLedger(t, path)
			if l.Version != 2 || l.Goal != p.Goal || l.MaxRequests != 14 || l.MaxMicroUSD != 1_000_000 || len(l.Entries) != calls+1 {
				t.Fatal("dispatch preceded durable goal-bound reservation", l)
			}
			calls++
			return nil, errors.New("offline simulated transport failure")
		})}
		r, _ := http.NewRequest("POST", "https://api.openai.com/v1/responses", strings.NewReader("{}"))
		_, err := tr.RoundTrip(r)
		if err == nil {
			t.Fatal("expected synthetic failure or exhausted budget")
		}
		if i <= 14 {
			if tr.Index != i || calls != i {
				t.Fatal("missing failed request reservation")
			}
			if err := finishReservationWithPolicy(path, p, i, "failed_or_unknown", "", 0, 0); err != nil {
				t.Fatal(err)
			}
			before := ledgerBytes(t, path)
			if _, err := tr.RoundTrip(r); err == nil {
				t.Fatal("retry admitted")
			}
			if !bytes.Equal(before, ledgerBytes(t, path)) || calls != i {
				t.Fatal("retry changed accounting or reached transport")
			}
		} else if tr.Index != 0 || calls != 14 {
			t.Fatal("fifteenth dispatch admitted")
		}
	}
	if len(readTestLedger(t, path).Entries) != 14 {
		t.Fatal("failed calls were refunded")
	}
}

func TestV2DollarCapIndependentOfCount(t *testing.T) {
	p := evaluationPolicy()
	p.MaxMicroUSD = 100_000
	path := filepath.Join(t.TempDir(), "budget.json")
	for i := 1; i <= 2; i++ {
		n, e := reserveWithPolicy(path, p)
		if e != nil || n != i {
			t.Fatal(n, e)
		}
		if e := finishReservationWithPolicy(path, p, n, "completed", "local-test", 1, 1); e != nil {
			t.Fatal(e)
		}
	}
	before := ledgerBytes(t, path)
	if _, err := reserveWithPolicy(path, p); err == nil {
		t.Fatal("cost cap ignored below request cap")
	}
	if !bytes.Equal(before, ledgerBytes(t, path)) {
		t.Fatal("exhausted budget changed history")
	}
}

func TestPoliciesCannotReuseOrResizeLedger(t *testing.T) {
	p := evaluationPolicy()
	path := filepath.Join(t.TempDir(), "v2.json")
	if _, err := reserveWithPolicy(path, p); err != nil {
		t.Fatal(err)
	}
	before := ledgerBytes(t, path)
	for _, other := range []LedgerPolicy{{}, legacyLedgerPolicy(), {"another-goal", 14, 1_000_000}, {p.Goal, 15, 1_000_000}, {p.Goal, 13, 1_000_000}, {p.Goal, 14, 2_000_000}, {p.Goal, 14, 500_000}} {
		if _, err := reserveWithPolicy(path, other); err == nil {
			t.Fatal("changed goal/budget admitted", other)
		}
		if err := finishReservationWithPolicy(path, other, 1, "completed", "wrong", 1, 1); err == nil {
			t.Fatal("changed goal settled existing reservation")
		}
		if !bytes.Equal(before, ledgerBytes(t, path)) {
			t.Fatal("mismatched policy modified ledger")
		}
	}
	legacy := filepath.Join(t.TempDir(), "v1.json")
	if _, err := reserve(legacy); err != nil {
		t.Fatal(err)
	}
	old := ledgerBytes(t, legacy)
	if _, err := reserveWithPolicy(legacy, p); err == nil {
		t.Fatal("v2 reused v1 ledger")
	}
	if !bytes.Equal(old, ledgerBytes(t, legacy)) {
		t.Fatal("v1 history changed")
	}
	if l := readTestLedger(t, legacy); l.Version != 1 || l.MaxRequests != 0 || l.MaxMicroUSD != 0 {
		t.Fatal("legacy format was migrated")
	}
}

func TestV2UsageAnomalyLocksAndMissingMetadataFailClosed(t *testing.T) {
	p := evaluationPolicy()
	path := filepath.Join(t.TempDir(), "v2.json")
	n, err := reserveWithPolicy(path, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = finishReservationWithPolicy(path, p, n, "completed", "usage-anomaly", 200_000, 1); err == nil {
		t.Fatal("usage beyond reservation accepted")
	}
	if _, err = reserveWithPolicy(path, p); err == nil {
		t.Fatal("halt was ignored")
	}
	if readTestLedger(t, path).HaltReason == "" {
		t.Fatal("anomaly halt not durable")
	}
	for _, text := range []string{`{}`, `{"version":2,"goal":"board-family-v2-final-01","entries":[]}`, `{"version":2,"goal":"board-family-v2-final-01","max_requests":14,"max_micro_usd":1000000}`, `{"version":2,"goal":"board-family-v2-final-01","max_requests":14,"max_micro_usd":1000000,"entries":null}`} {
		f := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(f, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := reserveWithPolicy(f, p); err == nil {
			t.Fatal("incomplete metadata reset history")
		}
		if string(ledgerBytes(t, f)) != text {
			t.Fatal("bad ledger mutated")
		}
	}
	locked := filepath.Join(t.TempDir(), "locked.json")
	if err := os.Mkdir(locked+".lock", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := reserveWithPolicy(locked, p); err == nil {
		t.Fatal("lock ignored")
	}
	symlink := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	before := ledgerBytes(t, path)
	if _, err := reserveWithPolicy(symlink, p); err == nil {
		t.Fatal("symlink accepted")
	}
	if !bytes.Equal(before, ledgerBytes(t, path)) {
		t.Fatal("symlink target changed")
	}
}

func TestInvalidPolicyCannotCreateLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "ledger.json")
	if _, err := reserveWithPolicy(path, LedgerPolicy{Goal: "invalid", MaxRequests: 0, MaxMicroUSD: 1}); err == nil {
		t.Fatal("invalid policy admitted")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("invalid policy created directories")
	}
}
