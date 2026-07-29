package compositionlowering

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
)

func TestLowerConnectionsIsDeterministicAcrossProcesses(t *testing.T) {
	if os.Getenv("KICADAI_PHASE6_PROCESS_HELPER") == "1" {
		union := newDisjointSet()
		union.join("function:a:A", "function:b:B")
		connections, _, issues := lowerConnections(
			union,
			map[string]circuitgraph.FunctionalEndpoint{
				"function:a:A": {Function: "a:A"},
				"function:b:B": {Function: "b:B"},
			},
			map[string]nodeMetadata{
				"function:a:A": {domain: "digital", current: 2},
				"function:b:B": {domain: "analog", current: 1},
			},
		)
		body, _ := json.Marshal(struct {
			Connections any `json:"connections"`
			Issues      any `json:"issues"`
		}{Connections: connections, Issues: issues})
		t.Log(string(body))
		return
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var baseline string
	for run := 0; run < 8; run++ {
		command := exec.Command(executable, "-test.run=^TestLowerConnectionsIsDeterministicAcrossProcesses$", "-test.v")
		command.Env = append(os.Environ(), "KICADAI_PHASE6_PROCESS_HELPER=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("process %d failed: %v\n%s", run, err, output)
		}
		line := phase6LoggedJSON(string(output))
		if line == "" {
			t.Fatalf("process %d emitted no JSON:\n%s", run, output)
		}
		if run == 0 {
			baseline = line
		} else if line != baseline {
			t.Fatalf("process %d changed lowering output:\nfirst=%s\ncurrent=%s", run, baseline, line)
		}
	}
}

func phase6LoggedJSON(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if marker := strings.Index(line, `{"connections"`); marker >= 0 {
			return line[marker:]
		}
	}
	return ""
}
