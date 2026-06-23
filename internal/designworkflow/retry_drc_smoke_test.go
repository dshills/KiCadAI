package designworkflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetryDRCEvidenceRealKiCadSmoke(t *testing.T) {
	cli := os.Getenv("KICADAI_REAL_KICAD_CLI")
	if cli == "" {
		t.Skip("set KICADAI_REAL_KICAD_CLI to run real KiCad retry DRC smoke coverage")
	}
	if strings.ContainsRune(cli, filepath.Separator) {
		if info, err := os.Stat(cli); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			t.Fatalf("KICADAI_REAL_KICAD_CLI is not an executable file: %q", cli)
		}
	} else if _, err := exec.LookPath(cli); err != nil {
		t.Fatalf("KICADAI_REAL_KICAD_CLI is not on PATH: %q", cli)
	}
	request := loadFullBoardRetryRequestForTest(t, "generated_led_connectivity")
	output := filepath.Join(t.TempDir(), "retry_drc_smoke")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result := Create(ctx, request, CreateOptions{
		OutputDir: output,
		Overwrite: true,
		KiCadChecks: KiCadCheckOptions{
			KiCadCLI: cli,
			Timeout:  30 * time.Second,
		},
	})
	writeStage, ok := stageByName(result, StageProjectWrite)
	if !ok || writeStage.Status != StageStatusOK {
		t.Fatalf("project write stage = %#v ok=%v", writeStage, ok)
	}
	pcbPath := filepath.Join(output, request.Name+".kicad_pcb")
	adapter := kicadRetryDRCEvidenceAdapter{}
	evidence := retryDRCEvidenceForAttempt(ctx, RetryDRCPolicyOptional, adapter, retryDRCEvidenceRequest{
		Attempt: 1,
		PCBPath: pcbPath,
		Options: KiCadCheckOptions{
			KiCadCLI: cli,
			Timeout:  30 * time.Second,
		},
	})
	if evidence.Status == retryEvidenceMissing {
		t.Fatalf("real KiCad DRC evidence missing: %#v", evidence)
	}
	if evidence.Source != "kicad-cli" {
		t.Fatalf("DRC evidence source = %q", evidence.Source)
	}
}
