package fablecodereviewremediation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type findingLedger struct {
	Findings []struct {
		ID              string  `json:"id"`
		Disposition     string  `json:"disposition"`
		ClosingCommit   *string `json:"closing_commit"`
		CurrentEvidence string  `json:"current_evidence"`
		Reproduction    struct {
			Test    string `json:"test"`
			Path    string `json:"path"`
			Command string `json:"command"`
		} `json:"reproduction"`
	} `json:"findings"`
}

type transactionSnapshotLedger struct {
	Snapshots []struct {
		ID           string `json:"id"`
		Source       string `json:"source"`
		SHA256       string `json:"sha256"`
		Verification string `json:"verification"`
	} `json:"snapshots"`
}

func TestFindingLedgerIsCompleteAndAuditable(t *testing.T) {
	root := filepath.Join("..", "..")
	var ledger findingLedger
	readJSON(t, "FINDINGS.json", &ledger)
	wantIDs := []string{"C1", "H1", "H2", "H3", "H4", "H5", "H6", "H7", "H8", "H9", "H10", "H11", "H12", "H13", "H14", "H15", "H16", "H17"}
	gotIDs := make([]string, 0, len(ledger.Findings))
	seen := map[string]bool{}
	for _, finding := range ledger.Findings {
		if seen[finding.ID] {
			t.Fatalf("duplicate finding %s", finding.ID)
		}
		seen[finding.ID] = true
		gotIDs = append(gotIDs, finding.ID)
		if strings.TrimSpace(finding.Reproduction.Test) == "" || strings.TrimSpace(finding.Reproduction.Command) == "" {
			t.Fatalf("finding %s lacks a reproduction test or command", finding.ID)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(finding.Reproduction.Path))); err != nil {
			t.Fatalf("finding %s reproduction path: %v", finding.ID, err)
		}
		if strings.TrimSpace(finding.CurrentEvidence) == "" {
			t.Fatalf("finding %s lacks current evidence disposition", finding.ID)
		}
		switch finding.Disposition {
		case "closed":
			if finding.ClosingCommit == nil || strings.TrimSpace(*finding.ClosingCommit) == "" {
				t.Fatalf("closed finding %s lacks closing commit", finding.ID)
			}
		case "reproduced_pending":
			if finding.ClosingCommit != nil {
				t.Fatalf("pending finding %s has closing commit %q", finding.ID, *finding.ClosingCommit)
			}
		default:
			t.Fatalf("finding %s has unsupported disposition %q", finding.ID, finding.Disposition)
		}
	}
	sort.Strings(gotIDs)
	sort.Strings(wantIDs)
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("finding IDs = %v, want %v", gotIDs, wantIDs)
	}
}

func TestTransactionSnapshotLedgerIsComplete(t *testing.T) {
	root := filepath.Join("..", "..")
	var ledger transactionSnapshotLedger
	readJSON(t, "TRANSACTION_SNAPSHOTS.json", &ledger)
	want := map[string]bool{
		"class_ab_output_stage":          true,
		"opamp_gain_stage_ac_coupled":    true,
		"amplifier_bias_network":         true,
		"class_ab_headphone_protected":   true,
		"class_ab_speaker_10w_protected": true,
		"usb_c_led_indicator_protected":  true,
		"usb_c_i2c_sensor_3v3_protected": true,
	}
	for _, snapshot := range ledger.Snapshots {
		if !want[snapshot.ID] {
			t.Fatalf("unexpected or duplicate snapshot %q", snapshot.ID)
		}
		delete(want, snapshot.ID)
		if len(snapshot.SHA256) != 64 || strings.Trim(snapshot.SHA256, "0123456789abcdef") != "" {
			t.Fatalf("snapshot %s has invalid SHA-256 %q", snapshot.ID, snapshot.SHA256)
		}
		if strings.TrimSpace(snapshot.Verification) == "" {
			t.Fatalf("snapshot %s lacks verification command", snapshot.ID)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(snapshot.Source))); err != nil {
			t.Fatalf("snapshot %s source: %v", snapshot.ID, err)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing snapshots: %v", want)
	}
}

func readJSON(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}
