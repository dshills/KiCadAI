package circuitgraph

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"kicadai/internal/components"
)

func TestEvidenceHashesAreDeterministicAcrossProcesses(t *testing.T) {
	if os.Getenv("KICADAI_PHASE6_HASH_HELPER") == "1" {
		catalog := &components.Catalog{
			Version:  "test",
			Families: []components.FamilyDefinition{{ID: "z"}, {ID: "a"}},
			ThermalPaths: []components.ThermalPathRecord{
				{ID: "sink-z", NaturalSinkToAmbientCPerW: 12},
				{ID: "sink-a", NaturalSinkToAmbientCPerW: 8},
			},
		}
		fmt.Printf("phase6-hashes:%s:%s\n", catalogHash(catalog), hashGraphValue(map[string]string{"z": "last", "a": "first"}))
		return
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var baseline string
	for run := 0; run < 8; run++ {
		command := exec.Command(executable, "-test.run=^TestEvidenceHashesAreDeterministicAcrossProcesses$", "-test.v")
		command.Env = append(os.Environ(), "KICADAI_PHASE6_HASH_HELPER=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("process %d failed: %v\n%s", run, err, output)
		}
		line := phase6HashLine(string(output))
		if line == "" {
			t.Fatalf("process %d emitted no hash evidence:\n%s", run, output)
		}
		if run == 0 {
			baseline = line
		} else if line != baseline {
			t.Fatalf("process %d changed hash evidence:\nfirst=%s\ncurrent=%s", run, baseline, line)
		}
	}
}

func phase6HashLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "phase6-hashes:") {
			return line
		}
	}
	return ""
}
