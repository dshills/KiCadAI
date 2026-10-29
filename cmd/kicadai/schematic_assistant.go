package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"kicadai/internal/atomicdir"
	"kicadai/internal/atomicfile"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/reports"
	"kicadai/internal/schematicassistant"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func runSchematicAssistant(ctx context.Context, opts cliOptions, input io.Reader, stdout, prompts io.Writer) error {
	if opts.overwrite || opts.withPCB || opts.requireDRC {
		return errors.New("schematic assistant never overwrites projects or generates/checks a PCB")
	}
	if opts.aiProvider != "" || opts.aiPrompt != "" || opts.aiPromptFile != "" {
		return errors.New("schematic assistant is offline; use intent compile for provider-assisted drafting")
	}
	if len(opts.commandArgs) == 0 {
		return fmt.Errorf("schematic requires list, new, resume, generate, example <divider|rc-filter>, or verify")
	}
	command := opts.commandArgs[0]
	if command == "list" && len(opts.commandArgs) == 1 {
		return writeJSON(stdout, map[string]any{"families": schematicassistant.Families(), "input_v": []float64{schematicassistant.InputMin, schematicassistant.InputMax}, "resistance_ohms": []float64{schematicassistant.ResistanceMin, schematicassistant.ResistanceMax}, "power_budget_w": schematicassistant.PowerBudget, "api_required": false, "hardware_validated": false})
	}
	if command == "verify" && len(opts.commandArgs) == 1 {
		if strings.TrimSpace(opts.target) == "" {
			return errors.New("schematic verify requires --target")
		}
		if err := schematicassistant.Verify(opts.target); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{"integrity_verified": true, "independent_attestation": false})
	}
	if strings.TrimSpace(opts.output) == "" {
		return errors.New("schematic workflow requires --output (a new bundle directory, or the existing session directory for resume)")
	}
	switch command {
	case "example":
		if len(opts.commandArgs) != 2 {
			return errors.New("example requires divider or rc-filter")
		}
		session, err := schematicassistant.Example(opts.commandArgs[1])
		if err != nil {
			return err
		}
		report, err := schematicassistant.Generate(ctx, session, opts.output, schematicassistant.Options{KiCadCLI: opts.kicadCLI, RequireERC: opts.requireERC})
		if err != nil {
			return err
		}
		return writeJSON(stdout, report)
	case "generate":
		if len(opts.commandArgs) != 1 || opts.requestPath == "" {
			return errors.New("generate requires --request confirmed-session.json")
		}
		session, err := loadAssistantSession(opts.requestPath)
		if err != nil {
			return err
		}
		report, err := schematicassistant.Generate(ctx, session, opts.output, schematicassistant.Options{KiCadCLI: opts.kicadCLI, RequireERC: opts.requireERC})
		if err != nil {
			return err
		}
		return writeJSON(stdout, report)
	case "new", "resume":
		if len(opts.commandArgs) != 1 {
			return errors.New("new/resume accepts no positional arguments")
		}
		var session schematicassistant.Session
		if command == "new" {
			// The workspace contains only the resumable draft until explicit confirmation.
			parent := filepath.Dir(opts.output)
			if err := os.MkdirAll(parent, 0755); err != nil {
				return err
			}
			if err := os.Mkdir(opts.output, 0755); err != nil {
				return err
			}
			session = schematicassistant.Session{Schema: schematicassistant.Schema}
		} else {
			if info, err := os.Lstat(opts.output); err != nil || !info.IsDir() {
				return errors.New("resume requires an existing session directory")
			}
		}
		// Cooperating resumptions cannot replace one another's answers.
		lock, err := atomicfile.AcquireLock(filepath.Join(opts.output, ".assistant-session.lock"))
		if err != nil {
			return err
		}
		defer func() { _ = lock.Release() }()
		if command == "resume" {
			session, err = loadAssistantSession(filepath.Join(opts.output, "session.json"))
			if err != nil {
				return err
			}
		}
		save := func(value schematicassistant.Session) error {
			raw, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				return err
			}
			return atomicfile.Write(filepath.Join(opts.output, "session.json"), append(raw, '\n'), 0600)
		}
		session, err = schematicassistant.Wizard(ctx, input, prompts, session, save)
		if err != nil {
			return fmt.Errorf("%w; resume with --output %q schematic resume", err, opts.output)
		}
		projectOutput := filepath.Join(opts.output, "project")
		if _, statErr := os.Lstat(projectOutput); statErr == nil {
			projectOutput = filepath.Join(opts.output, "project-"+session.ConfirmationSHA256)
		}
		report, err := schematicassistant.Generate(ctx, session, projectOutput, schematicassistant.Options{KiCadCLI: opts.kicadCLI, RequireERC: opts.requireERC})
		if err != nil {
			return err
		}
		return writeJSON(stdout, report)
	default:
		return fmt.Errorf("unsupported schematic command %q", command)
	}
}
func loadAssistantSession(path string) (schematicassistant.Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return schematicassistant.Session{}, err
	}
	defer func() { _ = file.Close() }()
	return schematicassistant.Decode(file)
}

// runIntentClarify collects exact compiler-owned questions without calling a provider.
// A recorded provider may explicitly resume compilation; live use stays in intent compile.
func runIntentClarify(ctx context.Context, opts cliOptions, input io.Reader, stdout, prompts io.Writer) error {
	if len(opts.commandArgs) != 1 || opts.target == "" || opts.output == "" {
		return errors.New("intent clarify requires --target prior-compilation-directory and --output new-answer-workspace")
	}
	if opts.overwrite {
		return errors.New("clarification preserves prior evidence; choose a new output workspace")
	}
	if opts.aiProvider != "" && opts.aiProvider != "recorded" {
		return errors.New("intent clarify makes no live API calls; use intent compile explicitly after collecting answers")
	}
	source, err := readClarificationArtifact(filepath.Join(opts.target, ".kicadai", "behavioral-source.txt"))
	if err != nil {
		return err
	}
	proposalRaw, err := readClarificationArtifact(filepath.Join(opts.target, ".kicadai", "behavioral-proposal.json"))
	if err != nil {
		return err
	}
	proposal, issues := behavioralintent.DecodeProposalStrict(strings.NewReader(string(proposalRaw)))
	if reports.HasBlockingIssue(issues) {
		return errors.New(issues[0].Message)
	}
	var prior behavioralintent.Result
	if err = decodeBehavioralArtifact(filepath.Join(opts.target, ".kicadai", "behavioral-compilation.json"), &prior); err != nil {
		return err
	}
	if len(prior.Clarifications) == 0 || len(prior.Clarifications) > 32 {
		return errors.New("prior compilation must have 1–32 clarification questions")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), behavioralintent.MaxAnswerBytes+1)
	var answers []behavioralintent.ClarificationAnswer
	for _, question := range prior.Clarifications {
		if err = ctx.Err(); err != nil {
			return err
		}
		clean := func(text string) string {
			return strings.Map(func(r rune) rune {
				if unicode.IsControl(r) && r != '\n' && r != '\t' {
					return -1
				}
				return r
			}, text)
		}
		if _, err = fmt.Fprintf(prompts, "%s\nWhy: %s\nChoices: %s\n", clean(question.Question), clean(question.WhyNeeded), clean(strings.Join(question.Choices, ", "))); err != nil {
			return err
		}
		text, readErr := schematicassistant.ReadLine(ctx, scanner)
		if readErr != nil {
			return fmt.Errorf("clarification interrupted; prior evidence remains unchanged: %w", readErr)
		}
		answer := strings.TrimSpace(text)
		if answer == "" || answer == "cancel" {
			return errors.New("clarification cancelled; no answer workspace written")
		}
		answers = append(answers, behavioralintent.ClarificationAnswer{ClarificationID: question.ID, UncertaintyIDs: question.UncertaintyIDs, Answer: answer})
	}
	followUp, err := behavioralintent.BindFollowUp(proposal, prior, answers)
	if err != nil {
		return err
	}
	followUp, issues = behavioralintent.ValidateFollowUp(string(source), proposal, prior, followUp, prior.CapabilitySHA256)
	if reports.HasBlockingIssue(issues) {
		return errors.New(issues[0].Message)
	}
	// Use a fresh workspace. The original proposal/compilation always remain available.
	if err = os.MkdirAll(filepath.Dir(opts.output), 0755); err != nil {
		return err
	}
	err = atomicdir.Publish(opts.output, func(workspace string) error {
		artifactDir := filepath.Join(workspace, ".kicadai")
		if createErr := os.Mkdir(artifactDir, 0700); createErr != nil {
			return createErr
		}
		values := map[string]any{"behavioral-compilation.json": prior}
		for name, raw := range map[string][]byte{"behavioral-source.txt": source, "behavioral-proposal.json": proposalRaw} {
			if writeErr := atomicfile.Write(filepath.Join(artifactDir, name), raw, 0600); writeErr != nil {
				return writeErr
			}
		}
		for name, value := range values {
			raw, encodeErr := json.MarshalIndent(value, "", "  ")
			if encodeErr != nil {
				return encodeErr
			}
			if writeErr := atomicfile.Write(filepath.Join(artifactDir, name), raw, 0600); writeErr != nil {
				return writeErr
			}
		}
		raw, encodeErr := json.MarshalIndent(followUp, "", "  ")
		if encodeErr != nil {
			return encodeErr
		}
		if writeErr := atomicfile.Write(filepath.Join(workspace, "answers.json"), raw, 0600); writeErr != nil {
			return writeErr
		}
		return ctx.Err()
	})
	if err != nil {
		return err
	}
	artifactDir := filepath.Join(opts.output, ".kicadai")
	answersPath := filepath.Join(opts.output, "answers.json")
	if opts.aiProviderRecord != "" {
		resumed := opts
		resumed.intentText = string(source)
		resumed.intentFile = ""
		resumed.aiProvider = "recorded"
		resumed.behavioralFollowUp = answersPath
		resumed.overwrite = true
		resumed.jsonOutput = true
		resumed.maxAIAttempts = 1
		return runBehavioralIntentCompile(ctx, resumed, stdout)
	}
	return writeJSON(stdout, map[string]any{"status": "answers_bound", "answers": answersPath, "prior_preserved": opts.target, "resume": fmt.Sprintf("kicadai --file %q --output %q --follow-up %q --overwrite --provider <explicit-provider> intent compile", filepath.Join(artifactDir, "behavioral-source.txt"), opts.output, answersPath), "api_requests": 0})
}
func readClarificationArtifact(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, behavioralintent.MaxProposalBytes+1))
	if len(raw) > behavioralintent.MaxProposalBytes {
		return nil, errors.New("clarification artifact too large")
	}
	return raw, err
}
