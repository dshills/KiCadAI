package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"kicadai/internal/airequirementeval"
	"kicadai/internal/practicalboardeval"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, string(practicalboardeval.Redact([]byte(err.Error()), os.Getenv("OPENAI_API_KEY"))))
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ai-requirement-eval prepare --repo PATH --output PATH | verify --repo PATH | run --repo PATH --live | replay --evidence PATH --output PATH")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	repo := flags.String("repo", ".", "repository root")
	output := flags.String("output", "", "new offline snapshot directory or external replay report")
	evidence := flags.String("evidence", "", "terminal evidence root for offline replay")
	live := flags.Bool("live", false, "execute the one user-approved frozen campaign")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch args[0] {
	case "prepare":
		if *live || *output == "" || *evidence != "" {
			return fmt.Errorf("prepare requires a new output path and forbids --live")
		}
		return airequirementeval.Prepare(ctx, root, *output)
	case "run":
		if !*live || *output != "" || *evidence != "" {
			return fmt.Errorf("run requires explicit --live and the fixed frozen output root")
		}
		return airequirementeval.Run(ctx, root)
	case "verify":
		if *live || *output != "" || *evidence != "" {
			return fmt.Errorf("verify is a read-only sealed preflight")
		}
		return airequirementeval.VerifyReady(ctx, root)
	case "replay":
		if *live || *output == "" || *evidence == "" {
			return fmt.Errorf("replay requires evidence and a new external output, without --live")
		}
		return airequirementeval.Replay(*evidence, *output)
	default:
		return fmt.Errorf("unknown command")
	}
}
