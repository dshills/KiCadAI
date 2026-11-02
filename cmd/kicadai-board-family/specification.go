package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"kicadai/internal/boardfamily"
)

func runSpecification(args []string, p commandPipeline, w io.Writer) (resultErr error) {
	defer func() {
		if errors.Is(resultErr, flag.ErrHelp) {
			resultErr = nil
		}
	}()
	if len(args) == 0 {
		return errors.New("usage: kicadai-board-family spec new|draft|review|confirm|build (use --help after the action)")
	}
	f := flag.NewFlagSet("spec "+args[0], flag.ContinueOnError)
	f.SetOutput(w)
	switch args[0] {
	case "help", "--help", "-h":
		_, err := fmt.Fprintln(w, "Confirmed specification workflow: spec new|draft|review|confirm|build. Use ACTION --help for flags. Drafting never generates a board; build is offline and requires a confirmed specification.")
		return err
	case "new":
		config := f.String("config", "", "existing explicit configuration JSON")
		request := f.String("request-file", "", "optional original request for human reconciliation")
		out := f.String("output", "", "new editable specification JSON file")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 || *config == "" || *out == "" {
			return errors.New("new requires --config and --output")
		}
		file, err := os.Open(*config)
		if err != nil {
			return err
		}
		c, err := boardfamily.Decode(file)
		if err = errors.Join(err, file.Close()); err != nil {
			return err
		}
		if _, err = boardfamily.Check(c); err != nil {
			return err
		}
		s := boardfamily.Specification{Version: boardfamily.SpecificationVersion, Origin: "manual", Configuration: &c, Unresolved: []string{}, ReviewNotes: []string{}}
		if *request != "" {
			b, err := readPrompt(*request)
			if err != nil {
				return err
			}
			s.OriginalRequest = string(b)
		}
		return writeNewSpecification(*out, s)
	case "draft":
		prompt := f.String("prompt", "", "original request; one paid request, no retries")
		promptFile := f.String("prompt-file", "", "original request file")
		ledger := f.String("ledger", "", "persistent spending ledger")
		budget := f.String("live-budget", "", "approved request/spending policy JSON (not itself authorization)")
		out := f.String("output", "", "new editable draft JSON; never generates a board")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 || (*prompt == "") == (*promptFile == "") || *ledger == "" || *budget == "" || *out == "" || p.interpret == nil {
			return errors.New("draft requires exactly one prompt, --ledger, --live-budget and --output")
		}
		if *promptFile != "" {
			b, err := readPrompt(*promptFile)
			if err != nil {
				return err
			}
			*prompt = string(b)
		}
		for _, input := range []string{*ledger, *budget, *promptFile} {
			if input != "" {
				a, err := filepath.Abs(input)
				if err != nil {
					return err
				}
				b, err := filepath.Abs(*out)
				if err != nil {
					return err
				}
				if a == b {
					return errors.New("draft output must be separate from inputs and ledger")
				}
			}
		}
		file, err := os.Open(*budget)
		if err != nil {
			return err
		}
		policy, err := boardfamily.DecodeLedgerPolicy(file)
		if err = errors.Join(err, file.Close()); err != nil {
			return err
		}
		// Reserve a new destination before spending. Existing files and symlinks fail closed.
		destination, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		selection, _, callErr := p.interpret(ctx, *prompt, *ledger, policy)
		cancel()
		clearErr := clearProviderCredentials()
		s := boardfamily.Specification{Version: boardfamily.SpecificationVersion, Origin: "ai", OriginalRequest: *prompt, Unresolved: []string{}, ReviewNotes: []string{
			"Untrusted AI proposal: compare every original requirement with the configuration and catalog. The model may omit restrictions. Confirmation is a human decision, not automatic semantic validation.",
			fmt.Sprintf("Draft source: model=%s, response=%s, ledger entry=%d. No board was generated.", selection.Model, selection.ResponseID, selection.LedgerIndex),
		}}
		if callErr == nil && selection.Decision.Disposition == "supported" && selection.Decision.Configuration != nil {
			s.Configuration = selection.Decision.Configuration
		} else {
			s.Unresolved = append(s.Unresolved, "No supported configuration proposed. Choose or edit a configuration and resolve the original request before confirmation.")
		}
		if selection.Decision.Message != "" {
			s.ReviewNotes = append(s.ReviewNotes, selection.Decision.Message)
		}
		writeErr := json.NewEncoder(destination).Encode(s)
		return errors.Join(callErr, clearErr, writeErr, destination.Close())
	case "review", "confirm":
		input := f.String("input", "", "editable specification JSON")
		var out, digest *string
		if args[0] == "confirm" {
			out = f.String("output", "", "new confirmed specification file")
			digest = f.String("accept-sha256", "", "fingerprint from review; explicitly accepts the displayed acknowledgement")
		}
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 || *input == "" {
			return errors.New("review/confirm requires --input")
		}
		s, err := readSpecification(*input)
		if err != nil {
			return err
		}
		if args[0] == "review" {
			r, err := boardfamily.ReviewSpecification(s)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		}
		if *out == "" {
			return errors.New("confirm requires --output and --accept-sha256")
		}
		c, err := boardfamily.ConfirmSpecification(s, *digest)
		if err != nil {
			return err
		}
		return writeNewSpecification(*out, c)
	case "build":
		input := f.String("input", "", "confirmed specification JSON")
		out := f.String("output", "", "new output bundle directory")
		cli := f.String("kicad-cli", "kicad-cli", "reviewed KiCad executable")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 || *input == "" || *out == "" || p.generate == nil || p.validate == nil {
			return errors.New("build requires --input, --output and a generation/validation pipeline")
		}
		file, err := os.Open(*input)
		if err != nil {
			return err
		}
		confirmed, err := boardfamily.DecodeConfirmedSpecification(file)
		if err = errors.Join(err, file.Close()); err != nil {
			return err
		}
		c, err := boardfamily.VerifyConfirmedSpecification(confirmed)
		if err != nil {
			return err
		}
		if err = clearProviderCredentials(); err != nil {
			return err
		}
		if _, err = p.generate(c, *out); err != nil {
			return err
		}
		if err = writeNewSpecification(filepath.Join(*out, "confirmed-specification.json"), confirmed); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
		defer cancel()
		v, validationErr := p.validate(ctx, *out, *cli)
		if validationErr == nil && (!v.Passed || len(v.Checks) != 14) {
			validationErr = errors.New("complete 14-gate validation did not pass")
		}
		for _, check := range v.Checks {
			if !check.Passed {
				validationErr = errors.Join(validationErr, fmt.Errorf("validation gate failed: %s", check.Name))
			}
		}
		if validationErr != nil {
			return errors.Join(validationErr, json.NewEncoder(w).Encode(map[string]any{"passed": false, "output": *out, "manual_repair": false}))
		}
		files, err := specificationBundleFiles(*out)
		if err != nil {
			return err
		}
		manifest := map[string]any{"version": "confirmed-specification-bundle-1", "passed": true, "review_sha256": confirmed.ReviewSHA256, "manual_repair": false, "validation_gates": len(v.Checks), "files_sha256": files}
		if err = writeNewSpecification(filepath.Join(*out, "bundle-manifest.json"), manifest); err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(map[string]any{"passed": true, "output": *out, "review_sha256": confirmed.ReviewSHA256, "manifest": "bundle-manifest.json", "files": len(files)})
	default:
		return fmt.Errorf("unknown specification action %q", args[0])
	}
}

func readSpecification(name string) (boardfamily.Specification, error) {
	f, err := os.Open(name)
	if err != nil {
		return boardfamily.Specification{}, err
	}
	s, err := boardfamily.DecodeSpecification(f)
	return s, errors.Join(err, f.Close())
}

func writeNewSpecification(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return errors.Join(err, f.Close())
}

func specificationBundleFiles(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("bundle contains a non-regular file")
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		files[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	return files, err
}
