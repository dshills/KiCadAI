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
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/boardfamily"
)

type indexedCommandTransport func(*http.Request) (*http.Response, error)

func (f indexedCommandTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func indexedCommandFact(kind, value, state string, sources ...int) boardfamily.ReferencedFact {
	return boardfamily.ReferencedFact{Kind: kind, Value: value, State: state, Sources: sources, Quantities: []int{}}
}

func indexedCommandRaw(t testing.TB, facts ...boardfamily.ReferencedFact) []byte {
	t.Helper()
	if facts == nil {
		facts = []boardfamily.ReferencedFact{}
	}
	b, err := json.Marshal(boardfamily.ReferencedIntent{Version: boardfamily.ReferenceIntentVersion, Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type indexedCommandCounts struct{ requests, generate, validate int }

func indexedCommandPipeline(t *testing.T, raw []byte, mode, nativeCLI string) (commandPipeline, *indexedCommandCounts) {
	t.Helper()
	counts := &indexedCommandCounts{}
	pipeline := defaultCommandPipeline()
	var journalRoot string
	pipeline.interpretIndexed = func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
		transport := indexedCommandTransport(func(r *http.Request) (*http.Response, error) {
			counts.requests++
			if counts.requests != 1 || r.Method != "POST" || r.URL.String() != "https://api.openai.com/v1/responses" {
				t.Fatal("unexpected provider endpoint or retry")
			}
			body, err := io.ReadAll(r.Body)
			if err = errors.Join(err, r.Body.Close()); err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(body, []byte(boardfamily.ReferenceIntentSchemaName)) || bytes.Contains(body, []byte("offline-command-placeholder")) {
				t.Fatal("wrong contract or credential in request body")
			}
			content := []any{map[string]any{"type": "output_text", "text": string(raw)}}
			if mode == "refusal" {
				content = []any{map[string]any{"type": "refusal", "refusal": "Synthetic refusal to retain."}}
			}
			response := map[string]any{"id": "offline-command-response", "status": "completed", "model": boardfamily.SelectionModel, "error": nil,
				"output": []any{map[string]any{"type": "message", "status": "completed", "content": content}},
				"usage":  map[string]any{"input_tokens": 100, "output_tokens": 200, "total_tokens": 300}}
			if mode == "missing-model" {
				delete(response, "model")
			}
			b, err := json.Marshal(map[string]any{"type": "response.completed", "response": response})
			if err != nil {
				t.Fatal(err)
			}
			wire := "event: response.completed\ndata: " + string(b) + "\n\n"
			if mode == "broken-stream" {
				wire = "data: {\"type\":\"response.created\"}\n\n"
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil
		})
		journalRoot = journal
		s, err := boardfamily.InterpretReferencedWithJournal(ctx, prompt, ledger, policy, transport, journalRoot)
		return s.Selection, s, err
	}
	checkCleared := func() {
		t.Helper()
		for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"} {
			if _, exists := os.LookupEnv(key); exists {
				t.Fatalf("%s remains available to native work", key)
			}
		}
	}
	pipeline.generate = func(c boardfamily.Config, output string) (boardfamily.Electrical, error) {
		counts.generate++
		checkCleared()
		for _, name := range []string{"request/receipt.json", "response.bin", "response/receipt.json", "selection/selection.json", "selection/ledger.json", "selection/receipt.json"} {
			if _, err := os.Stat(filepath.Join(journalRoot, name)); err != nil {
				t.Fatal("native generation started before durable evidence", name, err)
			}
		}
		if mode == "generation-failure" {
			return boardfamily.Electrical{}, errors.New("synthetic native generation failure")
		}
		return boardfamily.Generate(c, output)
	}
	pipeline.validate = func(ctx context.Context, output, cli string) (boardfamily.Validation, error) {
		counts.validate++
		checkCleared()
		if _, err := os.Stat(filepath.Join(output, "selection.json")); err != nil {
			t.Fatal("selection evidence was not saved before validation")
		}
		if nativeCLI != "" {
			if cli != nativeCLI {
				t.Fatal("native executable flag was lost")
			}
			return boardfamily.Validate(ctx, output, cli)
		}
		if mode == "validation-failure" {
			return boardfamily.Validation{}, errors.New("synthetic native validation failure")
		}
		return boardfamily.Validation{Passed: true}, nil // Orchestration only, not native qualification.
	}
	return pipeline, counts
}

func runIndexedCommand(t *testing.T, pipeline commandPipeline, args ...string) ([]byte, error) {
	t.Helper()
	previousFlags, previousArgs, previousStdout := flag.CommandLine, os.Args, os.Stdout
	defer func() { flag.CommandLine, os.Args, os.Stdout = previousFlags, previousArgs, previousStdout }()
	flag.CommandLine = flag.NewFlagSet("offline-indexed-command", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"offline-indexed-command"}, args...)
	f, err := os.CreateTemp(t.TempDir(), "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = f
	runErr := runWithPipeline(pipeline)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return output, runErr
}

func TestIndexedCandidateCommandOffline(t *testing.T) {
	cases := []struct{ sensor, profile string }{{"BMP280", "standard"}, {"BMP280", "fast"}, {"BMP280", "low_current"}, {"SHT31", "standard"}, {"SHT31", "fast"}}
	for _, tc := range cases {
		t.Run(tc.sensor+"-"+tc.profile, func(t *testing.T) {
			runIndexedCommandCase(t, tc.sensor, tc.profile, "", "")
		})
	}
	for _, mode := range []string{"clarify", "unsupported", "malformed-output", "invalid-extraction", "refusal", "missing-model", "broken-stream", "generation-failure", "validation-failure"} {
		t.Run(mode, func(t *testing.T) { runIndexedCommandCase(t, "SHT31", "standard", mode, "") })
	}
}

// Deliberate opt-in to existing local KiCad, never a provider. Both families
// exercise the real command validation/export handoff; pure tests use a stub.
func TestIndexedCandidateCommandNative(t *testing.T) {
	cli := os.Getenv("KICADAI_OFFLINE_NATIVE_CLI")
	if cli == "" {
		t.Skip("set KICADAI_OFFLINE_NATIVE_CLI for two-family native command verification")
	}
	for _, sensor := range []string{"BMP280", "SHT31"} {
		t.Run(sensor, func(t *testing.T) { runIndexedCommandCase(t, sensor, "standard", "", cli) })
	}
}

func runIndexedCommandCase(t *testing.T, sensor, profile, mode, nativeCLI string) {
	t.Helper()
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		t.Setenv(key, "offline-command-placeholder")
	}
	prompt := "Please use " + sensor + " with " + profile + " profile."
	raw := indexedCommandRaw(t, indexedCommandFact("sensor", sensor, "required", 0), indexedCommandFact("profile", profile, "required", 0))
	wantDisposition, wantOutcome := "supported", "decision"
	switch mode {
	case "clarify":
		prompt, raw = "I have not chosen a sensor.", indexedCommandRaw(t)
		wantDisposition = "clarify"
	case "unsupported":
		prompt = "Please use SHT31. I require heater operation."
		raw = indexedCommandRaw(t, indexedCommandFact("sensor", "SHT31", "required", 0), indexedCommandFact("feature", "heater_operation", "required", 1))
		wantDisposition = "unsupported"
	case "malformed-output":
		raw, wantOutcome = []byte(`{"broken":`), "invalid_extraction"
	case "invalid-extraction":
		raw, wantOutcome = []byte(`{"version":"bad","facts":[]}`), "invalid_extraction"
	case "refusal":
		wantOutcome = "provider_refusal"
	case "missing-model", "broken-stream":
		wantOutcome = "invalid_response_evidence"
	}
	dir := t.TempDir()
	budget, ledger, out, promptFile := filepath.Join(dir, "budget.json"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "out"), filepath.Join(dir, "request.txt")
	if err := os.WriteFile(budget, []byte(`{"goal":"offline-command","max_requests":1,"max_micro_usd":50000}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptFile, []byte(prompt), 0600); err != nil {
		t.Fatal(err)
	}
	pipeline, counts := indexedCommandPipeline(t, raw, mode, nativeCLI)
	args := []string{"--intent-protocol", "indexed-v3", "--evidence-journal", ledger + ".evidence", "--prompt-file", promptFile, "--live-budget", budget, "--ledger", ledger, "--output", out}
	if nativeCLI != "" {
		args = append(args, "--kicad-cli", nativeCLI)
	}
	stdout, err := runIndexedCommand(t, pipeline, args...)
	wantError := !memberCommand(mode, "", "clarify", "unsupported")
	if (err != nil) != wantError || counts.requests != 1 {
		t.Fatalf("unexpected command result: %v, requests %d, stdout %s", err, counts.requests, stdout)
	}
	wantNative := wantOutcome == "decision" && wantDisposition == "supported"
	if (counts.generate == 1) != wantNative || (counts.validate == 1) != (wantNative && mode != "generation-failure") {
		t.Fatal("wrong native-generation handoff", *counts)
	}
	selectionPath := filepath.Join(out, "selection.json")
	if mode == "generation-failure" {
		selectionPath = filepath.Join(ledger+".evidence", "selection", "selection.json")
		if _, err := os.Lstat(out); !os.IsNotExist(err) {
			t.Fatal("failed generation unexpectedly published an output")
		}
	}
	b, readErr := os.ReadFile(selectionPath)
	if readErr != nil {
		t.Fatal("command lost selection evidence:", readErr)
	}
	var saved boardfamily.ReferencedSelection
	if json.Unmarshal(b, &saved) != nil || saved.OriginalRequest != prompt || saved.AdmissionVersion != boardfamily.ReferenceIntentVersion || saved.Outcome != wantOutcome || saved.ProviderEvidence == nil || len(saved.ProviderEvidence.Body) == 0 {
		t.Fatal("candidate identity, outcome or wire evidence was lost by command")
	}
	if wantOutcome == "decision" && saved.Decision.Disposition != wantDisposition {
		t.Fatalf("wrong disposition: %s want %s", saved.Decision.Disposition, wantDisposition)
	}
	if mode == "generation-failure" {
		if len(stdout) != 0 || saved.Decision.Configuration == nil {
			t.Fatal("generation failure lost selection or emitted a success result")
		}
		return // No native output; the independent journal remains complete.
	}
	var summary map[string]any
	if json.Unmarshal(stdout, &summary) != nil || summary["passed"] != (wantNative && !wantError) {
		t.Fatalf("incorrect command stdout: %s", stdout)
	}
	if !wantNative {
		entries, err := os.ReadDir(out)
		if err != nil || len(entries) != 1 || entries[0].Name() != "selection.json" || saved.Decision.Configuration != nil {
			t.Fatal("failed/non-design selection emitted a board or lost evidence")
		}
	} else {
		family := boardfamily.Family
		if sensor == "SHT31" {
			family = boardfamily.FamilySHT31
		}
		if saved.Decision.Configuration == nil || saved.Decision.Configuration.Family != family || saved.Decision.Configuration.Profile != profile {
			t.Fatal("command changed family/profile")
		}
	}
	if nativeCLI != "" {
		b, err := os.ReadFile(filepath.Join(out, "validation.json"))
		var v boardfamily.Validation
		if err != nil || json.Unmarshal(b, &v) != nil || !v.Passed || len(v.Checks) != 14 {
			t.Fatal("complete native/export qualification failed", err)
		}
		for _, check := range v.Checks {
			if !check.Passed {
				t.Fatal("native check failed", check.Name)
			}
		}
		for _, name := range []string{"bom.csv", "preview/board.svg", "preview/pcb.svg", "manufacturing/manifest.json"} {
			if info, err := os.Stat(filepath.Join(out, name)); err != nil || info.Size() == 0 {
				t.Fatal("incomplete output bundle", name, err)
			}
		}
		t.Logf("%s/%s command: 14/14 real native/export gates; evidence saved; no output repair", sensor, profile)
	}
}

func memberCommand(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
