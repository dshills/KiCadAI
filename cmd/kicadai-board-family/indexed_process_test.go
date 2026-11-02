package main

import (
	"bytes"
	"context"
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

func TestExportIndexedContractOffline(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	path := filepath.Join(t.TempDir(), "contract.json")
	if err := runForTest(t, "--intent-protocol", "indexed-v3", "--export-live-contract", path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	var c map[string]any
	if err != nil || json.Unmarshal(b, &c) != nil || c["admission_version"] != boardfamily.ReferenceIntentVersion || c["schema_name"] != boardfamily.ReferenceIntentSchemaName || c["capability_context"] != boardfamily.ReferencedIntentLanguageContext() || c["experimental"] != true {
		t.Fatal("wrong experimental contract", err)
	}
	if c["request_revision"] != boardfamily.ReferenceIntentRequestRevision {
		t.Fatal("exported request revision differs from the model-facing contract")
	}
	want, _ := json.Marshal(boardfamily.ReferencedIntentSchema())
	got, _ := json.Marshal(c["schema"])
	if !bytes.Equal(want, got) || c["schema_scope"] == "" {
		t.Fatal("schema blueprint or request-specific bounds lost")
	}
}

func TestIndexedFlagsRequireExplicitSeparateEvidence(t *testing.T) {
	for _, mode := range []string{"unknown", "no-journal", "no-budget", "no-ledger", "config", "typed-journal", "list", "export-journal", "overlap"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			out, ledger, budget, journal := filepath.Join(dir, "out"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "budget.json"), filepath.Join(dir, "evidence")
			if err := os.WriteFile(budget, []byte(`{"goal":"offline-flags","max_requests":1,"max_micro_usd":50000}`), 0600); err != nil {
				t.Fatal(err)
			}
			protocol := "indexed-v3"
			switch mode {
			case "unknown":
				protocol = "unknown"
			case "no-journal":
				journal = ""
			case "no-budget":
				budget = ""
			case "no-ledger":
				ledger = ""
			case "typed-journal":
				protocol = "typed-v2"
			case "overlap":
				journal = filepath.Join(out, "evidence")
			}
			args := []string{"--intent-protocol", protocol, "--output", out, "--ledger", ledger, "--live-budget", budget, "--evidence-journal", journal, "--prompt", "Please use BMP280."}
			switch mode {
			case "config":
				args = append(args, "--config", "never-open.json")
			case "list":
				args = append(args, "--list-families")
			case "export-journal":
				args = append(args, "--export-live-contract", filepath.Join(dir, "contract.json"))
			}
			pipeline, counts := indexedCommandPipeline(t, []byte(`{}`), "", "")
			stdout, err := runIndexedCommand(t, pipeline, args...)
			if err == nil || counts.requests != 0 || counts.generate != 0 || counts.validate != 0 || len(stdout) != 0 {
				t.Fatal("invalid experimental flags performed work", err, *counts)
			}
			if _, err := os.Lstat(out); !os.IsNotExist(err) {
				t.Fatal("invalid flags wrote output")
			}
		})
	}
}

func TestSeparateEvidenceOutput(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, journal, output string
		valid                 bool
	}{
		{"siblings", "evidence", "output", true}, {"similar-prefix", "out-evidence", "out", true},
		{"same", "out", "out", false}, {"journal-inside", "out/evidence", "out", false},
		{"output-inside", "evidence", "evidence/out", false}, {"nested-nonexistent", "new/a/journal", "new/b/output", true},
		{"alias-same", "alias/out", "out", false}, {"alias-ancestor", "alias/out/evidence", "out", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := separateEvidenceOutput(filepath.Join(dir, tc.journal), filepath.Join(dir, tc.output)); (err == nil) != tc.valid {
				t.Fatal("wrong path separation", err)
			}
		})
	}
}

// This helper calls the real main entrypoint in a separate OS process. The only
// replaced dependency is HTTP transport; no fixture switch exists in the CLI.
func TestIndexedCommandProcessHelper(t *testing.T) {
	mode := os.Getenv("KICADAI_INDEXED_PROCESS_MODE")
	separator := -1
	for i, a := range os.Args {
		if a == "--" {
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
	args := append([]string{os.Args[0]}, os.Args[separator+1:]...)
	// The offline collector uses this test-only entrypoint prefix, never a
	// fixture flag in the shipped command. Case directory IDs select fixtures.
	if mode == "" {
		for i, arg := range args {
			if arg == "--inspect-indexed-journal" || arg == "--export-live-contract" {
				mode = "audit"
			}
			if arg == "--prompt-file" && i+1 < len(args) {
				mode = filepath.Base(filepath.Dir(args[i+1]))
			}
		}
		if mode == "" {
			t.Fatal("missing offline collector fixture identity")
		}
	}
	flag.CommandLine = flag.NewFlagSet("offline-indexed-process", flag.ExitOnError)
	os.Args = args
	if err := os.Setenv("OPENAI_API_KEY", "offline-process-placeholder"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	http.DefaultTransport = indexedCommandTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls != 1 || r.URL.String() != "https://api.openai.com/v1/responses" || r.Method != "POST" {
			t.Fatal("wrong physical request count or endpoint")
		}
		body, err := io.ReadAll(r.Body)
		if err = errors.Join(err, r.Body.Close()); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(boardfamily.ReferenceIntentSchemaName)) || bytes.Contains(body, []byte("offline-process-placeholder")) {
			t.Fatal("wrong protocol or leaked key")
		}
		if mode == "crash-after-request" {
			os.Exit(74)
		}
		if mode == "transport-error" {
			return nil, errors.New("offline transport failure")
		}
		if mode == "generation-conflict" {
			for i, a := range os.Args {
				if a == "--output" {
					if err := os.Mkdir(os.Args[i+1], 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(os.Args[i+1], "user-owned.txt"), []byte("do not replace"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		sensor := "BMP280"
		if mode == "native-SHT31" || mode == "unsupported" {
			sensor = "SHT31"
		}
		facts := []boardfamily.ReferencedFact{indexedCommandFact("sensor", sensor, "required", 0), indexedCommandFact("profile", "standard", "required", 0)}
		if mode == "clarify" {
			facts = nil
		}
		if mode == "unsupported" {
			facts = []boardfamily.ReferencedFact{indexedCommandFact("sensor", "SHT31", "required", 0), indexedCommandFact("feature", "heater_operation", "required", 1)}
		}
		raw := indexedCommandRaw(t, facts...)
		if strings.HasPrefix(mode, "useful-") || strings.HasPrefix(mode, "choice-") || strings.HasPrefix(mode, "refuse-") {
			var prompt []byte
			for i, arg := range os.Args {
				if arg == "--prompt-file" && i+1 < len(os.Args) {
					prompt, err = os.ReadFile(os.Args[i+1])
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			raw = indexedCorpusFixture(t, mode, string(prompt))
			if os.Getenv("KICADAI_INDEXED_CORPUS_FAILURE") == "invalid-extraction" {
				raw = []byte(`{"version":"invalid","facts":[]}`)
			}
		}
		if mode == "invalid-extraction" {
			raw = []byte(`{"version":"invalid","facts":[]}`)
		}
		if mode == "malformed-output" {
			raw = []byte(`{"broken":`)
		}
		content := []any{map[string]any{"type": "output_text", "text": string(raw)}}
		if mode == "refusal" {
			content = []any{map[string]any{"type": "refusal", "refusal": "Offline recorded refusal."}}
		}
		response := map[string]any{"id": "offline-process-" + mode, "model": boardfamily.SelectionModel, "status": "completed", "error": nil,
			"output": []any{map[string]any{"type": "message", "status": "completed", "content": content}}, "usage": map[string]any{"input_tokens": 100, "output_tokens": 200, "total_tokens": 300}}
		if mode == "missing-model" {
			delete(response, "model")
		}
		wire, err := json.Marshal(map[string]any{"type": "response.completed", "response": response})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: response.completed\ndata: " + string(wire) + "\n\n"))}, nil
	})
	main()     // Failure really exits 1; success returns to the helper.
	os.Exit(0) // Keep go test's PASS text out of command stdout.
}

func TestIndexedCommandProcessOutcomes(t *testing.T) {
	for _, mode := range []string{"clarify", "unsupported", "invalid-extraction", "malformed-output", "refusal", "transport-error", "missing-model", "crash-after-request", "generation-conflict", "validation-failure"} {
		t.Run(mode, func(t *testing.T) { runIndexedProcessCase(t, mode, "") })
	}
}

func TestIndexedCommandNativeProcesses(t *testing.T) {
	cli := os.Getenv("KICADAI_OFFLINE_NATIVE_CLI")
	if cli == "" {
		t.Skip("set KICADAI_OFFLINE_NATIVE_CLI for real native subprocess checks")
	}
	for _, mode := range []string{"native-BMP280", "native-SHT31"} {
		t.Run(mode, func(t *testing.T) { runIndexedProcessCase(t, mode, cli) })
	}
}

func runIndexedProcessCase(t *testing.T, mode, cli string) {
	t.Helper()
	dir := t.TempDir()
	budget, ledger, out, journal, promptFile := filepath.Join(dir, "budget.json"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "out"), filepath.Join(dir, "evidence"), filepath.Join(dir, "prompt.txt")
	prompt := "Please use BMP280 with standard profile."
	if mode == "native-SHT31" {
		prompt = "Please use SHT31 with standard profile."
	}
	if mode == "unsupported" {
		prompt = "Please use SHT31. I require heater operation."
	}
	if mode == "clarify" {
		prompt = "I have not chosen a sensor."
	}
	for file, data := range map[string]string{budget: `{"goal":"offline-process","max_requests":1,"max_micro_usd":50000}`, promptFile: prompt} {
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if cli == "" {
		cli = filepath.Join(dir, "nonexistent-kicad-cli")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIndexedCommandProcessHelper$", "--", "--intent-protocol", "indexed-v3", "--evidence-journal", journal, "--prompt-file", promptFile, "--output", out, "--ledger", ledger, "--live-budget", budget, "--kicad-cli", cli)
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if !memberCommand(key, "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "KICADAI_LIVE_PROVIDER_TESTS", "KICADAI_INDEXED_PROCESS_MODE") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "KICADAI_INDEXED_PROCESS_MODE="+mode)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	started := time.Now()
	err := cmd.Run()
	elapsed := time.Since(started)
	wantCode := 1
	if mode == "clarify" || mode == "unsupported" || strings.HasPrefix(mode, "native-") {
		wantCode = 0
	}
	if mode == "crash-after-request" {
		wantCode = 74
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() || cmd.ProcessState.ExitCode() != wantCode || ctx.Err() != nil || (err == nil) != (wantCode == 0) {
		t.Fatalf("wrong terminal process result: %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("offline-process-placeholder")) || bytes.Contains(stderr.Bytes(), []byte("offline-process-placeholder")) {
		t.Fatal("secret in process logs")
	}
	b, err := os.ReadFile(ledger)
	var l boardfamily.Ledger
	if err != nil || json.Unmarshal(b, &l) != nil || len(l.Entries) != 1 || l.Entries[0].ReserveMicroUSD != 50000 {
		t.Fatal("missing single request reservation", err)
	}
	if mode == "crash-after-request" {
		if l.Entries[0].Status != "reserved_unknown_outcome" {
			t.Fatal("crashed child invented settlement")
		}
		if _, err := os.Stat(filepath.Join(journal, "request", "receipt.json")); err != nil {
			t.Fatal("crash lost request")
		}
		if _, err := os.Lstat(filepath.Join(journal, "selection")); !os.IsNotExist(err) {
			t.Fatal("crash fabricated completed selection")
		}
		return
	}
	b, err = os.ReadFile(filepath.Join(journal, "selection", "selection.json"))
	var selection boardfamily.ReferencedSelection
	if err != nil || json.Unmarshal(b, &selection) != nil || selection.OriginalRequest != prompt || selection.ProviderEvidence == nil || !selection.ProviderEvidence.TransportStarted || selection.ProviderEvidence.PersistenceError {
		t.Fatal("child evidence missing or disputed", err)
	}
	wantOutcome := "decision"
	switch mode {
	case "invalid-extraction", "malformed-output":
		wantOutcome = "invalid_extraction"
	case "refusal":
		wantOutcome = "provider_refusal"
	case "transport-error":
		wantOutcome = "provider_failed_or_unknown"
	case "missing-model":
		wantOutcome = "invalid_response_evidence"
	}
	if selection.Outcome != wantOutcome {
		t.Fatal("exit conflated with extraction outcome", selection.Outcome)
	}
	if mode == "generation-conflict" {
		b, err := os.ReadFile(filepath.Join(out, "user-owned.txt"))
		if err != nil || string(b) != "do not replace" || selection.Decision.Configuration == nil {
			t.Fatal("generation conflict damaged evidence or user output")
		}
		return
	}
	var result struct {
		Passed       bool
		Disposition  string
		TotalSeconds float64 `json:"total_seconds"`
	}
	if json.Unmarshal(stdout.Bytes(), &result) != nil || result.Passed != strings.HasPrefix(mode, "native-") {
		t.Fatal("wrong command result", stdout.String())
	}
	if strings.HasPrefix(mode, "native-") {
		b, err := os.ReadFile(filepath.Join(out, "validation.json"))
		var v boardfamily.Validation
		if err != nil || json.Unmarshal(b, &v) != nil || !v.Passed || len(v.Checks) != 14 {
			t.Fatal("native subprocess bundle unqualified", err)
		}
		for _, check := range v.Checks {
			if !check.Passed {
				t.Fatal("native subprocess check failed", check.Name)
			}
		}
		for _, name := range []string{"bom.csv", "preview/board.svg", "preview/pcb.svg", "manufacturing/manifest.json"} {
			if info, err := os.Stat(filepath.Join(out, name)); err != nil || info.Size() == 0 {
				t.Fatal("native subprocess bundle incomplete", name, err)
			}
		}
		if result.TotalSeconds <= 0 || result.TotalSeconds > elapsed.Seconds() {
			t.Fatal("reported timing excludes/overstates process duration")
		}
	}
	t.Logf("%s: terminal exit=%d outcome=%s wall=%.3fs; journal retained", mode, wantCode, selection.Outcome, elapsed.Seconds())
}
