// practical-board-eval is an experimental acceptance runner, not an alternate
// production generator. Live cases require a verified, explicitly frozen set.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"kicadai/internal/practicalboardeval"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, string(practicalboardeval.Redact([]byte(err.Error()), os.Getenv("OPENAI_API_KEY"))))
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "snapshot or case")
	root := flag.String("root", ".", "repository root")
	output := flag.String("output", "", "new evidence directory")
	caseID := flag.String("case", "", "frozen corpus case ID")
	campaign := flag.String("campaign", "", "baseline, final or paired")
	journal := flag.String("journal", "", "shared append-only request journal")
	paired := flag.String("paired-input", "", "retained baseline case directory; no API calls")
	freezePath := flag.String("freeze", "specs/practical-sensor-controller-boards/freeze.json", "frozen file inventory")
	cli := flag.String("kicad-cli", "", "installed KiCad CLI")
	flag.Parse()
	if flag.NArg() != 0 || *output == "" {
		return fmt.Errorf("output and an explicit mode are required; positional arguments are not accepted")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	if err := os.Chdir(absRoot); err != nil {
		return err
	}
	absOutput, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if *mode == "snapshot" {
		return practicalboardeval.Snapshot(ctx, absOutput)
	}
	if *mode != "case" {
		return fmt.Errorf("mode must be snapshot or case")
	}
	if os.Getenv("KICADAI_PRACTICAL_EVAL_SUPERVISED") != "1" {
		return fmt.Errorf("use the frozen campaign supervisor for resource and retention limits")
	}
	freezeBytes, err := os.ReadFile(*freezePath)
	if err != nil {
		return err
	}
	if practicalboardeval.FreezeSHA256 == "" || practicalboardeval.SHA(freezeBytes) != practicalboardeval.FreezeSHA256 {
		return fmt.Errorf("binary is not bound to this freeze; rebuild using the recorded freeze SHA-256")
	}
	var freeze practicalboardeval.Freeze
	if err := practicalboardeval.ReadJSON(*freezePath, &freeze); err != nil {
		return err
	}
	if err := practicalboardeval.VerifyFreeze(absRoot, freeze); err != nil {
		return err
	}
	corpusPath := "specs/practical-sensor-controller-boards/corpus.json"
	var corpus practicalboardeval.Corpus
	if err := practicalboardeval.ReadJSON(corpusPath, &corpus); err != nil {
		return err
	}
	if corpus.Status != "frozen" {
		return fmt.Errorf("corpus is not frozen; no evaluation or live calls allowed")
	}
	sealed := false
	for _, file := range freeze.Files {
		if file.Path == corpusPath {
			sealed = true
		}
	}
	if !sealed {
		return fmt.Errorf("corpus is absent from the freeze inventory")
	}
	inputs, err := corpus.Inputs()
	if err != nil {
		return err
	}
	for _, item := range inputs {
		if item.ID != *caseID {
			continue
		}
		return practicalboardeval.RunCase(ctx, item, practicalboardeval.RunOptions{Output: absOutput, Journal: *journal, Campaign: *campaign, KiCadCLI: *cli, PairedInput: *paired})
	}
	return fmt.Errorf("case %q is not in the frozen corpus", *caseID)
}
