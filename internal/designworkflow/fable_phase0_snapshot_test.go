package designworkflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/transactions"
)

var fablePhase0DesignTransactionDigests = map[string]string{
	"class_ab_headphone_protected":   "c6601ddc366202f9b5ada37eefa61ba81bcd5ec032de073ca7179960d7d71148",
	"class_ab_speaker_10w_protected": "1f6c6fb6b24bc344d26850ba640414bf06cb3b3669b79c9be942dc9694b38bdf",
	"usb_c_led_indicator_protected":  "8bace70676c0f1f6665bce8c55c19f66911f2e1e6ec8c31dff2099788604573b",
	"usb_c_i2c_sensor_3v3_protected": "a697e4afb24974a7c5d142c6d18a579f1beda3c917d8b708d60a5b1c0797e0f0",
}

func assertFablePhase0DesignTransactionSnapshot(t *testing.T, fixtureID string, outputDir string) {
	t.Helper()
	expected, tracked := fablePhase0DesignTransactionDigests[fixtureID]
	if !tracked {
		return
	}
	data, err := os.ReadFile(filepath.Join(outputDir, ".kicadai", "transaction.json"))
	if err != nil {
		t.Fatalf("%s read Phase 0 transaction snapshot: %v", fixtureID, err)
	}
	var provenance struct {
		Transaction transactions.Transaction `json:"transaction"`
	}
	if err := json.Unmarshal(data, &provenance); err != nil {
		t.Fatalf("%s decode Phase 0 transaction snapshot: %v", fixtureID, err)
	}
	// The destination directory is environmental. It is the only excluded
	// field; every semantic operation field remains part of the digest.
	for index, operation := range provenance.Transaction.Operations {
		if operation.Op != transactions.OpWriteProject {
			continue
		}
		var write transactions.WriteProjectOperation
		if err := json.Unmarshal(operation.Raw, &write); err != nil {
			t.Fatalf("%s decode write_project: %v", fixtureID, err)
		}
		write.OutputDir = "<OUTPUT>"
		raw, err := json.Marshal(write)
		if err != nil {
			t.Fatalf("%s normalize write_project: %v", fixtureID, err)
		}
		provenance.Transaction.Operations[index] = transactions.NewOperation(transactions.OpWriteProject, raw)
	}
	normalized, err := json.Marshal(provenance.Transaction)
	if err != nil {
		t.Fatalf("%s encode normalized transaction: %v", fixtureID, err)
	}
	sum := sha256.Sum256(normalized)
	got := hex.EncodeToString(sum[:])
	if got != expected {
		t.Fatalf("%s normalized transaction digest = %s, want %s", fixtureID, got, expected)
	}
}
