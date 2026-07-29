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
	"class_ab_headphone_protected":   "b69fbb69dab12affbe184c4d1a92484725b3543c12410262aed44f116a362004",
	"class_ab_speaker_10w_protected": "0338c2cf16392948def41d948e73bec817797f6579ef3b468754d954296db44a",
	"usb_c_led_indicator_protected":  "5e9a90251632bd37a4781e7ddbd87f14fa9929f40400b79091ac22c80db72842",
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
