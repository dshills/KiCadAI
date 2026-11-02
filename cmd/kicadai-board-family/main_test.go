package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/boardfamily"
)

func TestExportTypedContractOffline(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	path := filepath.Join(t.TempDir(), "contract.json")
	if err := runForTest(t, "--export-live-contract", path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c["admission_version"] != boardfamily.IntentAdmissionVersion || c["model"] != boardfamily.SelectionModel || c["schema_name"] != boardfamily.IntentSchemaName || c["max_output_tokens"] != float64(1600) || c["capability_context"] != boardfamily.IntentLanguageContext() {
		t.Fatal("exported contract differs from production typed intent")
	}
	if !strings.Contains(c["other_payload"].(string), "successor contract") {
		t.Fatal("new contract not distinguished from frozen history")
	}
}

func runForTest(t *testing.T, args ...string) error {
	t.Helper()
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	flag.CommandLine = flag.NewFlagSet("board-family-test", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"board-family-test"}, args...)
	return run()
}

func TestBudgetFlagRejectsIncompatibleModesWithoutOutputs(t *testing.T) {
	for _, mode := range [][]string{
		{"--list-families"},
		{"--export-live-contract", "unused.json"},
		{"--config", "unused.json"},
		{"--prompt", "sensor"}, // Missing required separate ledger.
	} {
		t.Run(strings.Join(mode, " "), func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "")
			out := filepath.Join(t.TempDir(), "output")
			args := append(append([]string{}, mode...), "--live-budget", "must-not-be-opened.json")
			if mode[0] != "--list-families" && mode[0] != "--export-live-contract" {
				args = append(args, "--output", out)
			}
			if err := runForTest(t, args...); err == nil || !strings.Contains(err.Error(), "--") {
				t.Fatalf("incompatible mode was not rejected: %v", err)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("incompatible flags produced outputs")
			}
		})
	}
}

func TestBudgetFileValidatedBeforeProviderOrOutput(t *testing.T) {
	for _, policy := range []string{"{}", `{"goal":"board-family-v2-final-01","max_requests":14,"max_micro_usd":1000000}`} {
		t.Run(policy, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "")
			dir := t.TempDir()
			budget, ledger, out := filepath.Join(dir, "budget.json"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "out")
			if err := os.WriteFile(budget, []byte(policy), 0600); err != nil {
				t.Fatal(err)
			}
			// A whitespace prompt fails before provider construction; a valid
			// policy must reach that guard, while an incomplete one must not.
			err := runForTest(t, "--prompt", " ", "--live-budget", budget, "--ledger", ledger, "--output", out)
			if err == nil {
				t.Fatal("invalid invocation succeeded")
			}
			if gotPromptGuard := strings.Contains(err.Error(), "prompt must contain"); gotPromptGuard != (policy != "{}") {
				t.Fatalf("wrong guard reached: %v", err)
			}
			if _, err := os.Stat(ledger); !os.IsNotExist(err) {
				t.Fatal("pre-provider failure reserved allowance")
			}
			if policy == "{}" {
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("invalid policy wrote output")
				}
			}
		})
	}
}

func TestPromptFileBound(t *testing.T) {
	for _, size := range []int{0, 1, 2000, 2001, 10000} {
		path := filepath.Join(t.TempDir(), "prompt.txt")
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0600); err != nil {
			t.Fatal(err)
		}
		b, err := readPrompt(path)
		valid := size > 0 && size <= 2000
		if (err == nil) != valid || (valid && len(b) != size) {
			t.Fatalf("size %d: %d bytes, %v", size, len(b), err)
		}
	}
}

func TestNativeChildrenHaveNoProviderCredentials(t *testing.T) {
	keys := []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"}
	for _, key := range keys {
		t.Setenv(key, "test-only-dummy")
	}
	if err := clearProviderCredentials(); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if _, exists := os.LookupEnv(key); exists {
			t.Fatalf("%s would be inherited", key)
		}
	}
}
