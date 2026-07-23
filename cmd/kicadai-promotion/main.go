package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"kicadai/internal/promotionrunner"
	"kicadai/internal/promotiontoolchain"
)

const defaultLockPath = "toolchain/kicad-promotion.lock.json"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kicadai-promotion:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("usage: kicadai-promotion <resolve|bootstrap|promote> [options]")
	}
	switch arguments[0] {
	case "resolve":
		return runResolve(arguments[1:])
	case "bootstrap":
		return runBootstrap(arguments[1:])
	case "promote":
		return runPromote(arguments[1:])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func runPromote(arguments []string) error {
	flags := flag.NewFlagSet("promote", flag.ContinueOnError)
	lockPath := flags.String("lock", defaultLockPath, "toolchain lock path")
	matrixPath := flags.String("matrix", "testdata/external-review-mitigation/matrix.json", "promotion matrix path")
	repositoryRoot := flags.String("repository", ".", "repository root")
	kicadaiPath := flags.String("kicadai", "bin/kicadai", "kicadai executable path")
	outputRoot := flags.String("output", "", "empty promotion output root")
	timeout := flags.Duration("scenario-timeout", 20*time.Minute, "maximum duration for each scenario run")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *outputRoot == "" || *timeout <= 0 {
		return errors.New("positive --scenario-timeout and --output are required")
	}
	repository, err := filepath.Abs(*repositoryRoot)
	if err != nil {
		return err
	}
	document, err := promotiontoolchain.Load(*lockPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	toolchain, err := promotiontoolchain.Resolve(ctx, document, promotiontoolchain.ResolveOptions{})
	if err != nil {
		return err
	}
	matrix, err := promotionrunner.LoadMatrix(*matrixPath, repository)
	if err != nil {
		return err
	}
	results, err := promotionrunner.Run(ctx, matrix, toolchain, promotionrunner.Options{
		RepositoryRoot: repository, KiCadAI: *kicadaiPath, OutputRoot: *outputRoot, ScenarioTimeout: *timeout,
	})
	if err != nil {
		return err
	}
	return writeJSON(struct {
		Schema             string                      `json:"schema"`
		MatrixSHA256       string                      `json:"matrix_sha256"`
		LaneRegistrySHA256 string                      `json:"lane_registry_sha256"`
		Toolchain          promotiontoolchain.Evidence `json:"toolchain"`
		Results            []promotionrunner.RunResult `json:"results"`
	}{
		Schema: "kicadai.promotion-run.v1", MatrixSHA256: matrix.SHA256,
		LaneRegistrySHA256: promotionrunner.LaneRegistrySHA256(), Toolchain: toolchain, Results: results,
	})
}

func runResolve(arguments []string) error {
	flags := flag.NewFlagSet("resolve", flag.ContinueOnError)
	lockPath := flags.String("lock", defaultLockPath, "toolchain lock path")
	cli := flags.String("kicad-cli", "", "explicit kicad-cli path")
	symbols := flags.String("symbols-root", "", "explicit stock symbol root")
	footprints := flags.String("footprints-root", "", "explicit stock footprint root")
	timeout := flags.Duration("timeout", 30*time.Minute, "maximum toolchain resolution duration")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	document, err := promotiontoolchain.Load(*lockPath)
	if err != nil {
		return err
	}
	interruptContext, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(interruptContext, *timeout)
	defer cancel()
	evidence, err := promotiontoolchain.Resolve(ctx, document, promotiontoolchain.ResolveOptions{
		KiCadCLI: *cli, SymbolsRoot: *symbols, FootprintsRoot: *footprints,
	})
	if err != nil {
		return err
	}
	return writeJSON(evidence)
}

func runBootstrap(arguments []string) error {
	flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	lockPath := flags.String("lock", defaultLockPath, "toolchain lock path")
	cacheDir := flags.String("cache-dir", "", "caller-owned toolchain cache")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	document, err := promotiontoolchain.Load(*lockPath)
	if err != nil {
		return err
	}
	cache := *cacheDir
	if cache != "" {
		absolute, absErr := filepath.Abs(cache)
		if absErr != nil {
			return absErr
		}
		cache = absolute
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	evidence, err := promotiontoolchain.BootstrapToolchain(ctx, document, promotiontoolchain.BootstrapOptions{CacheDir: cache})
	if err != nil {
		return err
	}
	return writeJSON(evidence)
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
