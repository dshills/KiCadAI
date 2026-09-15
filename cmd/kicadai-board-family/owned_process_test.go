package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/boardfamily"
)

// Test-only subprocess seam: the real main and default pipeline run with an
// in-memory HTTP transport. No fixture flag or alternate endpoint is shipped.
func TestOwnedProcessHelper(t *testing.T) {
	mode := os.Getenv("KICADAI_OWNED_PROCESS_MODE")
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		if mode == "" {
			t.Skip("subprocess helper only")
		}
		t.Fatal("missing command arguments")
	}
	os.Args = append([]string{os.Args[0]}, os.Args[separator+1:]...)
	argValue := func(name string) string {
		for i, arg := range os.Args {
			if arg == name && i+1 < len(os.Args) {
				return os.Args[i+1]
			}
		}
		return ""
	}
	if mode == "" {
		mode = filepath.Base(filepath.Dir(argValue("--prompt-file")))
		if argValue("--inspect-owned-journal") != "" || argValue("--export-live-contract") != "" {
			mode = "audit"
		}
		if mode == "." {
			t.Fatal("missing offline collector fixture identity")
		}
	}
	flag.CommandLine = flag.NewFlagSet("offline-owned-process", flag.ExitOnError)
	calls := 0
	http.DefaultTransport = indexedCommandTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if mode == "audit" || calls != 1 || r.URL.String() != "https://api.openai.com/v1/responses" || r.Method != "POST" {
			t.Fatal("audit used provider transport, or extraction retried")
		}
		body, err := io.ReadAll(r.Body)
		if err = errors.Join(err, r.Body.Close()); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(boardfamily.OwnedEvidenceSchemaName)) || bytes.Contains(body, []byte("offline-owned-process-placeholder")) {
			t.Fatal("wrong protocol or credential in input")
		}
		switch mode {
		case "crash-after-request":
			os.Exit(74)
		case "transport-error":
			return nil, errors.New("offline owned transport failure")
		case "timeout":
			time.Sleep(30 * time.Second)
		case "log-overflow":
			_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), 2*1024*1024))
		case "generation-conflict":
			output := argValue("--output")
			if err := os.Mkdir(output, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(output, "user-owned.txt"), []byte("do not replace"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		raw := `{"version":"4-owned-evidence-experimental","facts":[{"kind":"sensor","value":"SHT31","state":"required","evidence":["c0"]},{"kind":"feature","value":"heater_operation","state":"required","evidence":["c1"]}]}`
		if strings.HasPrefix(mode, "useful-") || strings.HasPrefix(mode, "choice-") || strings.HasPrefix(mode, "refuse-") {
			prompt, err := os.ReadFile(argValue("--prompt-file"))
			if err != nil {
				t.Fatal(err)
			}
			_, fixture := ownedCorpusFixture(t, mode, string(prompt))
			raw = string(fixture)
		}
		if mode == "clarify" {
			raw = `{"version":"4-owned-evidence-experimental","facts":[]}`
		}
		if mode == "generation-conflict" || mode == "validation-failure" {
			raw = `{"version":"4-owned-evidence-experimental","facts":[{"kind":"sensor","value":"BMP280","state":"required","evidence":["c0"]}]}`
		}
		if mode == "invalid" || mode == "invalid-extraction" {
			raw = `{"version":"wrong","facts":[]}`
		}
		if mode == "malformed-output" {
			raw = `{"broken":`
		}
		content := []any{map[string]any{"type": "output_text", "text": raw}}
		if mode == "refusal" {
			content = []any{map[string]any{"type": "refusal", "refusal": "Synthetic refusal to retain."}}
		}
		response := map[string]any{"id": "offline-owned-process-" + mode, "model": boardfamily.SelectionModel, "status": "completed", "error": nil,
			"output": []any{map[string]any{"type": "message", "status": "completed", "content": content}},
			"usage":  map[string]any{"input_tokens": 100, "output_tokens": 200, "total_tokens": 300}}
		if mode == "missing-model" {
			delete(response, "model")
		}
		b, err := json.Marshal(map[string]any{"type": "response.completed", "response": response})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: response.completed\ndata: " + string(b) + "\n\n"))}, nil
	})
	if mode != "audit" {
		t.Setenv("OPENAI_API_KEY", "offline-owned-process-placeholder")
	}
	main()
	os.Exit(0) // No go test PASS trailer in the real command's JSON stdout.
}

func TestOwnedRealEntrypointAndCredentialFreeAudit(t *testing.T) {
	run := func(mode string, args ...string) ([]byte, error) {
		t.Helper()
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestOwnedProcessHelper$", "--"}, args...)...)
		for _, e := range os.Environ() {
			key, _, _ := strings.Cut(e, "=")
			if memberCommand(key, "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "KICADAI_LIVE_PROVIDER_TESTS", "KICADAI_OWNED_PROCESS_MODE") {
				continue
			}
			cmd.Env = append(cmd.Env, e)
		}
		cmd.Env = append(cmd.Env, "KICADAI_OWNED_PROCESS_MODE="+mode)
		return cmd.CombinedOutput()
	}
	for _, mode := range []string{"unsupported", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			output, ledger, budget, journal := filepath.Join(dir, "out"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "budget.json"), filepath.Join(dir, "journal")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-owned-process", MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			stdout, err := run(mode, "--intent-protocol", "owned-v4", "--prompt", "Please use SHT31. Require its heater.", "--output", output, "--ledger", ledger, "--live-budget", budget, "--evidence-journal", journal)
			if (err == nil) != (mode == "unsupported") {
				t.Fatalf("wrong observed child exit: %v %s", err, stdout)
			}
			entries, err := os.ReadDir(output)
			if err != nil || len(entries) != 1 || entries[0].Name() != "selection.json" {
				t.Fatal("refusal/failure generated native files", err)
			}
			stdout, err = run("audit", "--inspect-owned-journal", journal)
			if err != nil || !bytes.Contains(stdout, []byte(`"version":"owned-journal-audit-1"`)) {
				t.Fatalf("key-free process audit failed: %v %s", err, stdout)
			}
			if _, err := run("audit", "--inspect-indexed-journal", journal); err == nil {
				t.Fatal("real legacy inspector accepted new evidence")
			}
		})
	}
}
