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
	interpret             func(context.Context, string, string, boardfamily.LedgerPolicy) (boardfamily.Selection, any, error)
	interpretIndexed      func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretOwned        func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretConnection   func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretDirect       func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretGrounded     func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretGroundedFull func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretEligible     func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretAddressed    func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretBoundary     func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretFidelity     func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	interpretCoverage     func(context.Context, string, string, boardfamily.LedgerPolicy, string) (boardfamily.Selection, any, error)
	generate              func(boardfamily.Config, string) (boardfamily.Electrical, error)
	validate              func(context.Context, string, string) (boardfamily.Validation, error)
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
		interpretConnection: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretConnectionWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretDirect: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretDirectWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretGrounded: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretGroundedWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretGroundedFull: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretGroundedFullWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretEligible: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretSourceEligibleWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretAddressed: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretSourceAddressedWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretBoundary: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretSemanticBoundaryWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretCoverage: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretCoverageWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
			return s.Selection, s, err
		},
		interpretFidelity: func(ctx context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy, journal string) (boardfamily.Selection, any, error) {
			s, err := boardfamily.InterpretFidelityWithJournal(ctx, prompt, ledger, policy, http.DefaultTransport, journal)
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
	protocol := flag.String("intent-protocol", "typed-v2", "typed-v2 (default), indexed-v3, owned-v4, connection-v5, direct-v6, partitioned-v7, partitioned-full-v7, source-eligible-v8, source-addressed-v9, semantic-boundaries-v10, requirement-coverage-v11 or requirement-fidelity-v12 (experimental; requires separate approval and evidence journal)")
	journal := flag.String("evidence-journal", "", "new private evidence directory outside output; required for experimental prompts")
	inspectJournal := flag.String("inspect-indexed-journal", "", "verify an existing indexed journal by local byte replay; no key or API request")
	inspectOwnedJournal := flag.String("inspect-owned-journal", "", "verify an existing owned-v4 journal by local byte replay; no key or API request")
	inspectConnectionJournal := flag.String("inspect-connection-journal", "", "verify an existing connection-v5 journal by local byte replay; no key or API request")
	inspectDirectJournal := flag.String("inspect-direct-journal", "", "verify an existing direct-v6 journal by local byte replay; no key or API request")
	inspectGroundedJournal := flag.String("inspect-partitioned-journal", "", "verify an existing partitioned-v7 journal by local byte replay; no key or API request")
	inspectGroundedFullJournal := flag.String("inspect-partitioned-full-journal", "", "verify an existing full-model partitioned-v7 journal by local byte replay; no key or API request")
	inspectEligibleJournal := flag.String("inspect-source-eligible-journal", "", "verify an existing source-eligible-v8 journal by local byte replay; no key or API request")
	inspectAddressedJournal := flag.String("inspect-source-addressed-journal", "", "verify an existing source-addressed-v9 journal by local byte replay; no key or API request")
	inspectBoundaryJournal := flag.String("inspect-semantic-boundary-journal", "", "verify an existing semantic-boundaries-v10 journal by local byte replay; no key or API request")
	inspectCoverageJournal := flag.String("inspect-requirement-coverage-journal", "", "verify an existing requirement-coverage-v11 journal by local byte replay; no key or API request")
	inspectFidelityJournal := flag.String("inspect-requirement-fidelity-journal", "", "verify an existing requirement-fidelity-v12 journal by local byte replay; no key or API request")
	exportContract := flag.String("export-live-contract", "", "write the exact non-secret capability context/schema for inspection; no API call")
	listFamilies := flag.Bool("list-families", false, "print supported families, profiles and fixed conditions; no API call")
	out := flag.String("output", "", "new output directory (required)")
	cli := flag.String("kicad-cli", "kicad-cli", "KiCad 10.0.3 executable")
	flag.Parse()
	inspectModes := 0
	for _, value := range []string{*inspectJournal, *inspectOwnedJournal, *inspectConnectionJournal, *inspectDirectJournal, *inspectGroundedJournal, *inspectGroundedFullJournal, *inspectEligibleJournal, *inspectAddressedJournal, *inspectBoundaryJournal, *inspectCoverageJournal, *inspectFidelityJournal} {
		if value != "" {
			inspectModes++
		}
	}
	if inspectModes != 0 {
		if inspectModes != 1 || *protocol != "typed-v2" || *journal != "" || *config != "" || *prompt != "" || *promptFile != "" || *ledger != "" || *budgetFile != "" || *exportContract != "" || *listFamilies || *out != "" || flag.NArg() != 0 {
			return errors.New("journal inspection cannot be combined with other modes")
		}
		inspect, path := boardfamily.InspectReferencedJournal, *inspectJournal
		if *inspectOwnedJournal != "" {
			inspect, path = boardfamily.InspectOwnedJournal, *inspectOwnedJournal
		}
		if *inspectConnectionJournal != "" {
			inspect, path = boardfamily.InspectConnectionJournal, *inspectConnectionJournal
		}
		if *inspectDirectJournal != "" {
			inspect, path = boardfamily.InspectDirectJournal, *inspectDirectJournal
		}
		if *inspectGroundedJournal != "" {
			inspect, path = boardfamily.InspectGroundedJournal, *inspectGroundedJournal
		}
		if *inspectGroundedFullJournal != "" {
			inspect, path = boardfamily.InspectGroundedFullJournal, *inspectGroundedFullJournal
		}
		if *inspectEligibleJournal != "" {
			inspect, path = boardfamily.InspectSourceEligibleJournal, *inspectEligibleJournal
		}
		if *inspectAddressedJournal != "" {
			inspect, path = boardfamily.InspectSourceAddressedJournal, *inspectAddressedJournal
		}
		if *inspectBoundaryJournal != "" {
			inspect, path = boardfamily.InspectSemanticBoundaryJournal, *inspectBoundaryJournal
		}
		if *inspectCoverageJournal != "" {
			inspect, path = boardfamily.InspectCoverageJournal, *inspectCoverageJournal
		}
		if *inspectFidelityJournal != "" {
			inspect, path = boardfamily.InspectFidelityJournal, *inspectFidelityJournal
		}
		audit, err := inspect(path)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(audit)
	}
	if *protocol != "typed-v2" && *protocol != "indexed-v3" && *protocol != "owned-v4" && *protocol != "connection-v5" && *protocol != "direct-v6" && *protocol != "partitioned-v7" && *protocol != "partitioned-full-v7" && *protocol != "source-eligible-v8" && *protocol != "source-addressed-v9" && *protocol != "semantic-boundaries-v10" && *protocol != "requirement-coverage-v11" && *protocol != "requirement-fidelity-v12" {
		return errors.New("unknown --intent-protocol; expected typed-v2, indexed-v3, owned-v4, connection-v5, direct-v6, partitioned-v7, partitioned-full-v7, source-eligible-v8, source-addressed-v9, semantic-boundaries-v10, requirement-coverage-v11 or requirement-fidelity-v12")
	}
	requestSpecific := *protocol == "owned-v4" || *protocol == "connection-v5" || *protocol == "direct-v6" || *protocol == "partitioned-v7" || *protocol == "partitioned-full-v7" || *protocol == "source-eligible-v8" || *protocol == "source-addressed-v9" || *protocol == "semantic-boundaries-v10" || *protocol == "requirement-coverage-v11" || *protocol == "requirement-fidelity-v12"
	experimental := *protocol == "indexed-v3" || requestSpecific
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
		if *config != "" || (!requestSpecific && (*prompt != "" || *promptFile != "")) || *out != "" || *budgetFile != "" || flag.NArg() != 0 {
			return fmt.Errorf("--export-live-contract cannot be combined with generation")
		}
		if requestSpecific {
			if (*prompt == "") == (*promptFile == "") || *ledger != "" {
				return fmt.Errorf("%s contract export requires exactly one --prompt or --prompt-file and no ledger", *protocol)
			}
			if *promptFile != "" {
				b, err := readPrompt(*promptFile)
				if err != nil {
					return err
				}
				*prompt = string(b)
			}
			export := boardfamily.OwnedEvidenceContract
			if *protocol == "connection-v5" {
				export = boardfamily.ConnectionEvidenceContract
			}
			if *protocol == "direct-v6" {
				export = boardfamily.DirectEvidenceContract
			}
			if *protocol == "partitioned-v7" {
				export = boardfamily.GroundedEvidenceContract
			}
			if *protocol == "partitioned-full-v7" {
				export = boardfamily.GroundedFullEvidenceContract
			}
			if *protocol == "source-eligible-v8" {
				export = boardfamily.SourceEligibleEvidenceContract
			}
			if *protocol == "source-addressed-v9" {
				export = boardfamily.SourceAddressedEvidenceContract
			}
			if *protocol == "semantic-boundaries-v10" {
				export = boardfamily.SemanticBoundaryEvidenceContract
			}
			if *protocol == "requirement-coverage-v11" {
				export = boardfamily.CoverageEvidenceContract
			}
			if *protocol == "requirement-fidelity-v12" {
				export = boardfamily.FidelityEvidenceContract
			}
			contract, err := export(*prompt)
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
		if *protocol == "requirement-fidelity-v12" && pipeline.interpretFidelity == nil {
			return errors.New("requirement-fidelity-v12 requires a configured selector")
		}
		if *protocol == "requirement-coverage-v11" && pipeline.interpretCoverage == nil {
			return errors.New("requirement-coverage-v11 requires a configured selector")
		}
		if *config != "" || *ledger == "" || *budgetFile == "" || *journal == "" || (*protocol == "indexed-v3" && pipeline.interpretIndexed == nil) || (*protocol == "owned-v4" && pipeline.interpretOwned == nil) || (*protocol == "connection-v5" && pipeline.interpretConnection == nil) || (*protocol == "direct-v6" && pipeline.interpretDirect == nil) || (*protocol == "partitioned-v7" && pipeline.interpretGrounded == nil) || (*protocol == "partitioned-full-v7" && pipeline.interpretGroundedFull == nil) || (*protocol == "source-eligible-v8" && pipeline.interpretEligible == nil) || (*protocol == "source-addressed-v9" && pipeline.interpretAddressed == nil) || (*protocol == "semantic-boundaries-v10" && pipeline.interpretBoundary == nil) {
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
		if *protocol == "requirement-fidelity-v12" {
			s, record, e = pipeline.interpretFidelity(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "requirement-coverage-v11" {
			s, record, e = pipeline.interpretCoverage(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "semantic-boundaries-v10" {
			s, record, e = pipeline.interpretBoundary(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "source-addressed-v9" {
			s, record, e = pipeline.interpretAddressed(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "source-eligible-v8" {
			s, record, e = pipeline.interpretEligible(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "partitioned-full-v7" {
			s, record, e = pipeline.interpretGroundedFull(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "partitioned-v7" {
			s, record, e = pipeline.interpretGrounded(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "direct-v6" {
			s, record, e = pipeline.interpretDirect(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "connection-v5" {
			s, record, e = pipeline.interpretConnection(ctx, *prompt, *ledger, policy, *journal)
		} else if *protocol == "owned-v4" {
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
