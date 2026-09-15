package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/boardfamily"
)

// Offline counterpart of the indexed orchestration seam. It substitutes only
// an in-memory provider and, unless explicitly requested, a validation stub.
// The actual generator runs; no stub result is called native qualification.
func ownedCommandPipeline(t *testing.T, raw []byte, mode, nativeCLI string) (commandPipeline, *indexedCommandCounts) {
	return evidenceCommandPipeline(t, raw, mode, nativeCLI, "owned-v4")
}

func connectionCommandPipeline(t *testing.T, raw []byte, mode, nativeCLI string) (commandPipeline, *indexedCommandCounts) {
	return evidenceCommandPipeline(t, raw, mode, nativeCLI, "connection-v5")
}

func directCommandPipeline(t *testing.T, raw []byte, mode, nativeCLI string) (commandPipeline, *indexedCommandCounts) {
	return evidenceCommandPipeline(t, raw, mode, nativeCLI, "direct-v6")
}

func evidenceCommandPipeline(t *testing.T, raw []byte, mode, nativeCLI, protocol string) (commandPipeline, *indexedCommandCounts) {
	t.Helper()
	if protocol != "owned-v4" && protocol != "connection-v5" && protocol != "direct-v6" {
		t.Fatal("unknown offline evidence protocol")
	}
	counts := &indexedCommandCounts{}
	pipeline := defaultCommandPipeline()
	var journalRoot string
	interpret := func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
		transport := indexedCommandTransport(func(r *http.Request) (*http.Response, error) {
			counts.requests++
			if counts.requests != 1 || r.Method != "POST" || r.URL.String() != "https://api.openai.com/v1/responses" {
				t.Fatal("unexpected provider endpoint or retry")
			}
			body, err := io.ReadAll(r.Body)
			if err = errors.Join(err, r.Body.Close()); err != nil {
				t.Fatal(err)
			}
			schemaName := boardfamily.OwnedEvidenceSchemaName
			if protocol == "connection-v5" {
				schemaName = boardfamily.ConnectionEvidenceSchemaName
			}
			if protocol == "direct-v6" {
				schemaName = boardfamily.DirectEvidenceSchemaName
			}
			if !bytes.Contains(body, []byte(schemaName)) || bytes.Contains(body, []byte("offline-command-placeholder")) {
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
			if mode == "server-error" {
				wire = "event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"server_error\",\"message\":\"Synthetic provider failure\"}}\n\n"
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil
		})
		journalRoot = journal
		selectWithJournal := boardfamily.InterpretOwnedWithJournal
		if protocol == "connection-v5" {
			selectWithJournal = boardfamily.InterpretConnectionWithJournal
		}
		if protocol == "direct-v6" {
			selectWithJournal = boardfamily.InterpretDirectWithJournal
		}
		s, err := selectWithJournal(ctx, prompt, ledger, policy, transport, journalRoot)
		return s.Selection, s, err
	}
	if protocol == "direct-v6" {
		pipeline.interpretDirect = interpret
	} else if protocol == "connection-v5" {
		pipeline.interpretConnection = interpret
	} else {
		pipeline.interpretOwned = interpret
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

func TestOwnedCandidateCommandNative(t *testing.T) {
	cli := os.Getenv("KICADAI_OFFLINE_NATIVE_CLI")
	if testing.Short() || cli == "" {
		t.Skip("requires explicit offline KiCad CLI; not a live model test")
	}
	for _, sensor := range []string{"BMP280", "SHT31"} {
		t.Run(sensor, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			prompt := "Please use " + sensor + " with standard profile."
			raw := []byte(`{"version":"4-owned-evidence-experimental","facts":[{"kind":"sensor","value":"` + sensor + `","state":"required","evidence":["c0"]},{"kind":"profile","value":"standard","state":"required","evidence":["c0"]}]}`)
			dir := t.TempDir()
			output, ledger, budget, journal := filepath.Join(dir, "output"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "budget.json"), filepath.Join(dir, "journal")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-owned-native", MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			p, counts := ownedCommandPipeline(t, raw, "", cli)
			stdout, err := runIndexedCommand(t, p, "--intent-protocol", "owned-v4", "--prompt", prompt, "--output", output, "--ledger", ledger, "--live-budget", budget, "--evidence-journal", journal, "--kicad-cli", cli)
			if err != nil || counts.requests != 1 || counts.generate != 1 || counts.validate != 1 {
				t.Fatalf("offline native command failed: %v counts=%+v stdout=%s", err, counts, stdout)
			}
			read := func(path string) boardfamily.Validation {
				t.Helper()
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var v boardfamily.Validation
				if err := json.Unmarshal(b, &v); err != nil {
					t.Fatal(err)
				}
				return v
			}
			v := read(filepath.Join(output, "validation.json"))
			reviewed := read(filepath.Join("..", "..", "examples", "board-family-v2", strings.ToLower(sensor)+"-standard", "validation.json"))
			if !v.Passed || v.KiCadVersion != "10.0.3" || len(v.Checks) != 14 || !reflect.DeepEqual(v.NativeSHA256, reviewed.NativeSHA256) {
				t.Fatalf("complete native checks or reviewed native bytes differ: %+v", v)
			}
			for i, check := range v.Checks {
				if !check.Passed || check.Error != "" || check.Name != reviewed.Checks[i].Name {
					t.Fatal("required validation gate missing or failed", check)
				}
			}
			if _, err := boardfamily.InspectOwnedJournal(journal); err != nil {
				t.Fatal("native command's journal does not authenticate", err)
			}
			t.Logf("synthetic selector -> real generator -> KiCad %s: all %d validation/export/immutability gates passed, reviewed native hashes unchanged; model accuracy untested", v.KiCadVersion, len(v.Checks))
		})
	}
}
