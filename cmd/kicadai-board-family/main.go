package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"kicadai/internal/boardfamily"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	config := flag.String("config", "", "explicit board-family configuration JSON")
	prompt := flag.String("prompt", "", "ordinary-language board request (one approved OpenAI request)")
	promptFile := flag.String("prompt-file", "", "UTF-8 file containing an ordinary-language request")
	ledger := flag.String("ledger", "", "persistent ledger for the approved goal's 41-request / $10 limit; required with a prompt")
	exportContract := flag.String("export-live-contract", "", "write the exact non-secret capability context/schema for inspection; no API call")
	out := flag.String("output", "", "new output directory (required)")
	cli := flag.String("kicad-cli", "kicad-cli", "KiCad 10.0.3 executable")
	flag.Parse()
	if *exportContract != "" {
		if *config != "" || *prompt != "" || *promptFile != "" || *out != "" || flag.NArg() != 0 {
			return fmt.Errorf("--export-live-contract cannot be combined with generation")
		}
		return save(*exportContract, map[string]any{"destination": "https://api.openai.com/v1/responses", "model": boardfamily.SelectionModel, "capability_context": boardfamily.LanguageContext, "schema": boardfamily.SelectionSchema(), "max_output_tokens": 1600, "other_payload": "The original request text, attempt=1, no diagnostics, and the existing KiCadAI provider's generic JSON-only system instructions. No source files, native geometry, keys or environment variables are included as model input. The existing key is sent only in the HTTPS Authorization header."})
	}
	modes := 0
	for _, s := range []string{*config, *prompt, *promptFile} {
		if s != "" {
			modes++
		}
	}
	if flag.NArg() != 0 || modes != 1 || *out == "" {
		return fmt.Errorf("usage: kicadai-board-family (--config FILE | --prompt TEXT | --prompt-file FILE) --output NEW_DIRECTORY [--ledger FILE] [--kicad-cli PATH]")
	}
	if _, e := os.Stat(*out); !os.IsNotExist(e) {
		return fmt.Errorf("output must not exist before work starts")
	}
	start := time.Now()
	var c boardfamily.Config
	var selection *boardfamily.Selection
	if *config != "" {
		f, e := os.Open(*config)
		if e != nil {
			return e
		}
		var err error
		c, err = boardfamily.Decode(f)
		ce := f.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
	} else {
		if *ledger == "" {
			return fmt.Errorf("--ledger is required for a live request")
		}
		if *promptFile != "" {
			b, e := readPrompt(*promptFile)
			if e != nil {
				return e
			}
			*prompt = string(b)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		s, e := boardfamily.Interpret(ctx, *prompt, *ledger)
		cancel()
		if ce := clearProviderCredentials(); ce != nil {
			return ce
		}
		selection = &s
		if e != nil || s.Decision.Disposition != "supported" {
			if me := os.MkdirAll(filepath.Dir(*out), 0755); me != nil {
				return me
			}
			if me := os.Mkdir(*out, 0755); me != nil {
				return me
			}
			if me := save(filepath.Join(*out, "selection.json"), s); me != nil {
				return me
			}
			if e != nil {
				return errors.Join(e, json.NewEncoder(os.Stdout).Encode(map[string]any{"passed": false, "disposition": "failed", "output": *out, "ledger_index": s.LedgerIndex}))
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"passed": false, "disposition": s.Decision.Disposition, "message": s.Decision.Message, "output": *out})
		}
		c = *s.Decision.Configuration
	}
	// Native tools and round-trip helpers must not inherit provider secrets.
	// This is a single-request CLI process; no concurrent provider work exists.
	if e := clearProviderCredentials(); e != nil {
		return e
	}
	generationStart := time.Now()
	_, e := boardfamily.Generate(c, *out)
	generation := time.Since(generationStart).Seconds()
	if e != nil {
		return e
	}
	if selection != nil {
		if e = save(filepath.Join(*out, "selection.json"), selection); e != nil {
			return e
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	v, e := boardfamily.Validate(ctx, *out, *cli)
	summary := struct {
		Passed            bool    `json:"passed"`
		Output            string  `json:"output"`
		GenerationSeconds float64 `json:"generation_seconds"`
		ValidationSeconds float64 `json:"validation_seconds"`
		TotalSeconds      float64 `json:"total_seconds"`
	}{e == nil, *out, generation, v.Seconds, time.Since(start).Seconds()}
	if je := json.NewEncoder(os.Stdout).Encode(summary); je != nil {
		return je
	}
	return e
}

func readPrompt(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(f, 2001))
	if err = errors.Join(err, f.Close()); err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b) > 2000 {
		return nil, fmt.Errorf("prompt must contain 1–2000 bytes")
	}
	return b, nil
}

func clearProviderCredentials() error {
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		if err := os.Unsetenv(key); err != nil {
			return err
		}
	}
	return nil
}

func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
