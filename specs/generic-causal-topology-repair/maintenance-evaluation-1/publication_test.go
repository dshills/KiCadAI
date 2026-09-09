package maintenanceevaluation

import (
	"os"
	"path/filepath"
	"testing"
)

// These bindings seal the published outputs, not a required passing outcome.
// The pre-run evaluation_test.go independently authenticates and assesses them.
func TestMaintenancePublishedArtifacts(t *testing.T) {
	const reportSHA256 = "949963f6f38574626d125fe8d4e2f34966437b9a3c21dfe6518ceff37985efb1"
	const assessmentSHA256 = "26456f6f1e68d076b5b564c3f0a04d017dd8ee12d15a4806f6ff117a9fe34574"
	reportPath := filepath.Join(root(), filepath.FromSlash(publication))
	if fileHash(t, reportPath) != reportSHA256 {
		t.Fatal("published maintenance report bytes changed")
	}
	if fileHash(t, "ASSESSMENT.json") != assessmentSHA256 {
		t.Fatal("published maintenance assessment bytes changed")
	}
	checksum, err := os.ReadFile(filepath.Join(filepath.Dir(reportPath), "report.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if string(checksum) != reportSHA256+"  report.json\n" {
		t.Fatal("published report checksum differs")
	}
}
