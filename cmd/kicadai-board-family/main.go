package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	return runWithPipeline(defaultCommandPipeline())
}

// The default keeps the v2 selector. An explicit experimental protocol requires
// a separate evidence journal and budget policy. Injection lets offline tests
// exercise both through this exact command flow without a network socket.
// The record preserves any richer
// candidate evidence without losing the common, validated selection fields.
type commandPipeline struct {
	interpret        func(context.Context, string, string, boardfamily.LedgerPolicy) (boardfamily.Selection, any, error)
	interpretIndexed func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretOwned   func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	generate         func(boardfamily.Config, string) (boardfamily.Electrical, error)
	validate         func(context.Context, string, string) (boardfamily.Validation, error)
}

func defaultCommandPipeline() commandPipeline {
	return commandPipeline{
		interpret: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretWithPolicy(ctx, prompt, ledger, policy)
			return s, s, err
		},
		interpretIndexed: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretReferencedWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretOwned: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretOwnedWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		generate: boardfamily.Generate,
		validate: boardfamily.Validate,
	}
}

func runWithPipeline(pipeline commandPipeline) error {
	if pipeline.interpret == nil || pipeline.generate == nil || pipeline.validate == nil {
		return errors.New("incomplete board-family command pipeline")
	}
	config := flag.String("config", "", "explicit board-family configuration JSON")
	prompt := flag.String("prompt", "", "ordinary-language board request (one approved OpenAI request)")
	promptFile := flag.String("prompt-file", "", "UTF-8 file containing an ordinary-language request")
	ledger := flag.String("ledger", "", "persistent ledger required with a prompt; legacy limits unless --live-budget supplies a separate approved goal")
	budgetFile := flag.String("live-budget", "", "JSON goal/request/microdollar policy for a separately approved new goal; not spending authorization")
	protocol := flag.String("intent-protocol", "typed-v2", "typed-v2 (default), indexed-v3 or owned-v4 (experimental; requires separate approval and evidence journal)")
	journal := flag.String("evidence-journal", "", "new private evidence directory outside output; required for experimental indexed-v3 or owned-v4 prompts")
	inspectJournal := flag.String("inspect-indexed-journal", "", "verify an existing indexed journal by local byte replay; no key or API request")
	inspectOwnedJournal := flag.String("inspect-owned-journal", "", "verify an existing owned-v4 journal by local byte replay; no key or API request")
	exportContract := flag.String("export-live-contract", "", "write the exact non-secret capability context/schema for inspection; no API call")
	listFamilies := flag.Bool("list-families", false, "print supported families, profiles and fixed conditions; no API call")
	out := flag.String("output", "", "new output directory (required)")
	cli := flag.String("kicad-cli", "kicad-cli", "KiCad 10.0.3 executable")
	flag.Parse()
	if *inspectJournal != "" || *inspectOwnedJournal != "" {
		if (*inspectJournal != "" && *inspectOwnedJournal != "") || *protocol != "typed-v2" || *journal != "" || *config != "" || *prompt != "" || *promptFile != "" || *ledger != "" || *budgetFile != "" || *exportContract != "" || *listFamilies || *out != "" || flag.NArg() != 0 {
			return errors.New("journal inspection cannot be combined with other modes")
		}
		inspect, path := boardfamily.InspectReferencedJournal, *inspectJournal
		if *inspectOwnedJournal != "" {
			inspect, path = boardfamily.InspectOwnedJournal, *inspectOwnedJournal
		}
		audit, err := inspect(path)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(audit)
	}
	if *protocol != "typed-v2" && *protocol != "indexed-v3" && *protocol != "owned-v4" {
		return errors.New("unknown --intent-protocol; expected typed-v2, indexed-v3 or owned-v4")
	}
	experimental := *protocol == "indexed-v3" || *protocol == "owned-v4"
	if *journal != "" && (!experimental || *listFamilies || *exportContract != "" || *config != "") {
		return errors.New("--evidence-journal is only valid for experimental prompt generation")
	}
	if *listFamilies {
		if *config != "" || *prompt != "" || *promptFile != "" || *out != "" || *ledger != "" || *budgetFile != "" || *exportContract != "" || *protocol != "typed-v2" || flag.NArg() != 0 {
			return fmt.Errorf("--list-families cannot be combined with generation or export flags")
		}
		return json.NewEncoder(os.Stdout).Encode(boardfamily.Catalog())
	}
	if *exportContract != "" {
		if *config != "" || (*protocol != "owned-v4" && (*prompt != "" || *promptFile != "")) || *out != "" || *budgetFile != "" || flag.NArg() != 0 {
			return fmt.Errorf("--export-live-contract cannot be combined with generation")
		}
		if *protocol == "owned-v4" {
			if (*prompt == "") == (*promptFile == "") || *ledger != "" {
				return errors.New("owned-v4 contract export requires exactly one --prompt or --prompt-file and no ledger")
			}
			if *promptFile != "" {
				b, err := readPrompt(*promptFile)
				if err != nil {
					return err
				}
				*prompt = string(b)
			}
			contract, err := boardfamily.OwnedEvidenceContract(*prompt)
			if err != nil {
				return err
			}
			return save(*exportContract, contract)
		}
		if *protocol == "indexed-v3" {
			return save(*exportContract, map[string]any{"admission_version": boardfamily.ReferenceIntentVersion, "destination": "https://api.openai.com/v1/responses", "model": boardfamily.SelectionModel,
				"capability_context": boardfamily.ReferencedIntentLanguageContext(), "schema_name": boardfamily.ReferenceIntentSchemaName, "schema": boardfamily.ReferencedIntentSchema(), "max_output_tokens": 1600,
				"experimental": true, "request_revision": boardfamily.ReferenceIntentRequestRevision,
				"schema_scope":  "maximum-inventory blueprint; each request tightens clause/quantity ID and array bounds, omits the numeric branch when no quantities exist, and offers sensor facts only for literal sensor names already permitted by the decoder's identity rule",
				"other_payload": "Original request, application-numbered source clauses and literal quantity table; attempt=1, no diagnostics, and the existing provider's generic JSON-only system instructions. No gold answers, source files, native geometry, keys or environment variables are model input. The existing key is sent only in the HTTPS Authorization header. Separate immutable request/response/selection evidence is required. This is an experimental successor, not the frozen typed-v2 contract."})
		}
		return save(*exportContract, map[string]any{"admission_version": boardfamily.IntentAdmissionVersion, "destination": "https://api.openai.com/v1/responses", "model": boardfamily.SelectionModel, "capability_context": boardfamily.IntentLanguageContext(), "schema_name": boardfamily.IntentSchemaName, "schema": boardfamily.IntentSchema(), "max_output_tokens": 1600, "other_payload": "A JSON object containing the original request and application-numbered source clauses, attempt=1, no diagnostics, and the existing KiCadAI provider's generic JSON-only system instructions. No source files, native geometry, keys or environment variables are included as model input. The existing key is sent only in the HTTPS Authorization header. This typed-requirement payload is a successor contract, not the frozen final-01 payload."})
	}
	modes := 0
	for _, s := range []string{*config, *prompt, *promptFile} {
		if s != "" {
			modes++
		}
	}
	if flag.NArg() != 0 || modes != 1 || *out == "" {
		return fmt.Errorf("usage: kicadai-board-family (--config FILE | --prompt TEXT | --prompt-file FILE) --output NEW_DIRECTORY [--ledger FILE --live-budget FILE] [--kicad-cli PATH]")
	}
	if *budgetFile != "" && (*config != "" || *ledger == "") {
		return errors.New("--live-budget requires a prompt and a separate --ledger")
	}
	if experimental {
		if *config != "" || *ledger == "" || *budgetFile == "" || *journal == "" || (*protocol == "indexed-v3" && pipeline.interpretIndexed == nil) || (*protocol == "owned-v4" && pipeline.interpretOwned == nil) {
			return fmt.Errorf("%s requires a prompt, a separate --live-budget, --ledger and --evidence-journal", *protocol)
		}
		if err := separateEvidenceOutput(*journal, *out); err != nil {
			return err
		}
	}
	if _, e := os.Stat(*out); !os.IsNotExist(e) {
		return fmt.Errorf("output must not exist before work starts")
	}
	start := time.Now()
	var c boardfamily.Config
	var selection *boardfamily.Selection
	var selectionRecord any
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
		var policy boardfamily.LedgerPolicy
		if *budgetFile != "" {
			f, err := os.Open(*budgetFile)
			if err != nil {
				return err
			}
			policy, err = boardfamily.DecodeLedgerPolicy(f)
			if err = errors.Join(err, f.Close()); err != nil {
				return err
			}
		}
		if *promptFile != "" {
			b, e := readPrompt(*promptFile)
			if e != nil {
				return e
			}
			*prompt = string(b)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		var s boardfamily.Selection
		var record any
		var e error
		if *protocol == "owned-v4" {
			s, record, e = pipeline.interpretOwned(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "indexed-v3" {
			s, record, e = pipeline.interpretIndexed(ctx, *prompt, *ledger, policy, *journal)
		} else {
			s, record, e = pipeline.interpret(ctx, *prompt, *ledger, policy)
		}
		cancel()
		if ce := clearProviderCredentials(); ce != nil {
			return ce
		}
		selection = &s
		selectionRecord = record
		if selectionRecord == nil {
			selectionRecord = s
		}
		if e == nil && s.Decision.Disposition == "supported" && s.Decision.Configuration == nil {
			e = errors.New("supported selection has no validated configuration")
		}
		if e != nil || s.Decision.Disposition != "supported" {
			if me := os.MkdirAll(filepath.Dir(*out), 0755); me != nil {
				return me
			}
			if me := os.Mkdir(*out, 0755); me != nil {
				return me
			}
			if me := save(filepath.Join(*out, "selection.json"), selectionRecord); me != nil {
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
	_, e := pipeline.generate(c, *out)
	generation := time.Since(generationStart).Seconds()
	if e != nil {
		return e
	}
	if selection != nil {
		if e = save(filepath.Join(*out, "selection.json"), selectionRecord); e != nil {
			return e
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	v, e := pipeline.validate(ctx, *out, *cli)
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

// Resolve existing ancestors without creating output paths. Keeping both trees
// disjoint prevents generation cleanup or journal creation from consuming output.
func separateEvidenceOutput(journal, output string) error {
	resolve := func(p string) (string, error) {
		p, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		var suffix []string
		for {
			if _, err := os.Lstat(p); err == nil {
				root, err := filepath.EvalSymlinks(p)
				if err != nil {
					return "", err
				}
				for i := len(suffix) - 1; i >= 0; i-- {
					root = filepath.Join(root, suffix[i])
				}
				return root, nil
			} else if !os.IsNotExist(err) {
				return "", err
			}
			suffix = append(suffix, filepath.Base(p))
			p = filepath.Dir(p)
		}
	}
	j, err := resolve(journal)
	if err != nil {
		return err
	}
	o, err := resolve(output)
	if err != nil {
		return err
	}
	for _, pair := range [][2]string{{j, o}, {o, j}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil {
			return err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return errors.New("evidence journal and output directories must be disjoint")
		}
	}
	return nil
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
