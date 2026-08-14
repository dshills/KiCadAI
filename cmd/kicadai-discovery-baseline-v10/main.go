package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityexecutorv10"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/opentopologysynthesis"
	"kicadai/internal/promotiontoolchain"
	"kicadai/internal/reports"
)

type options struct {
	repositoryRoot    string
	corpusRoot        string
	workingRoot       string
	reportPath        string
	evaluatorManifest string
	toolchainLock     string
	timeout           time.Duration
	keepArtifacts     bool
	resume            bool
}

type summary struct {
	Schema        string                               `json:"schema"`
	Version       int                                  `json:"version"`
	ReportPath    string                               `json:"report_path"`
	ReportHash    string                               `json:"report_hash"`
	CaseCount     int                                  `json:"case_count"`
	OutcomeCounts []capabilitybaselinev10.OutcomeCount `json:"outcome_counts"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-discovery-baseline-v10", flag.ContinueOnError)
	var flagDiagnostics bytes.Buffer
	flags.SetOutput(&flagDiagnostics)
	var opts options
	flags.StringVar(&opts.repositoryRoot, "repository-root", ".", "clean committed repository root")
	flags.StringVar(&opts.corpusRoot, "corpus-root", "", "authenticated published V10 corpus root")
	flags.StringVar(&opts.workingRoot, "working-root", "", "fresh root for per-case replay and promotion evidence")
	flags.StringVar(&opts.reportPath, "report", "", "fresh generation-zero report path")
	flags.StringVar(&opts.evaluatorManifest, "evaluator-manifest", "", "frozen V10 evaluator checksum manifest")
	flags.StringVar(&opts.toolchainLock, "toolchain-lock", "", "locked KiCad promotion toolchain document")
	flags.DurationVar(&opts.timeout, "timeout", 4*time.Hour, "whole-cohort execution timeout")
	flags.BoolVar(&opts.keepArtifacts, "keep-artifacts", true, "retain installed-KiCad promotion evidence")
	flags.BoolVar(&opts.resume, "resume", false, "resume authenticated completed-case checkpoints from the working root")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse V10 discovery evaluator arguments: %s", strings.TrimSpace(flagDiagnostics.String()))
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("parse V10 discovery evaluator arguments: unexpected positional arguments")
	}
	if err := normalizeOptions(&opts); err != nil {
		return err
	}
	if err := ensureCleanRepository(parent, opts.repositoryRoot); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, opts.timeout)
	defer cancel()

	corpus, err := capabilityexecutorv10.LoadPublicDiscovery(opts.corpusRoot)
	if err != nil {
		return err
	}
	evaluatorManifestSHA256, err := capabilityexecutorv10.RegularFileSHA256(opts.evaluatorManifest)
	if err != nil {
		return fmt.Errorf("hash evaluator manifest: %w", err)
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		return fmt.Errorf("load embedded component catalog: %w", err)
	}
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		return fmt.Errorf("load model provenance: %s", diagnostics[0].Message)
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	inventory, inventoryIssues := opentopologysynthesis.BuildPrimitiveInventory(catalog, catalogHash, models)
	if reports.HasBlockingIssue(inventoryIssues) {
		return fmt.Errorf("build primitive inventory: blocking inventory evidence")
	}
	toolchain, err := promotiontoolchain.Load(opts.toolchainLock)
	if err != nil {
		return err
	}
	toolEvidence, err := promotiontoolchain.Resolve(ctx, toolchain, promotiontoolchain.ResolveOptions{})
	if err != nil {
		return err
	}
	promotionEnvironmentSHA256, err := capabilityexecutorv10.PromotionEnvironmentHash(toolEvidence)
	if err != nil {
		return err
	}
	cliSHA256, err := capabilityexecutorv10.RegularFileSHA256(toolEvidence.KiCadCLI)
	if err != nil {
		return err
	}
	index, libraryIssues := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{
		SymbolsRoot: toolEvidence.SymbolsRoot, FootprintsRoot: toolEvidence.FootprintsRoot,
	}, libraryresolver.LoadOptions{})
	index.Diagnostics = libraryIssues
	report, err := capabilityexecutorv10.New().Run(ctx, capabilityexecutorv10.Request{
		CorpusManifestSHA256: corpus.ManifestSHA256, OutputRoot: opts.workingRoot,
		Resume: opts.resume, Cases: corpus.Cases,
		Environment: capabilityexecutorv10.Environment{
			Inventory:  inventory,
			Simulation: opentopologysynthesis.SimulationEnvironment{Catalog: catalog, CatalogHash: catalogHash, ModelRegistry: models},
			Policy:     opentopologysynthesis.DefaultPolicy(), LibraryIndex: &index, KiCadCLI: toolEvidence.KiCadCLI,
			KiCadCLISHA256: cliSHA256, PromotionEnvironmentSHA256: promotionEnvironmentSHA256,
			EvaluatorManifestSHA256: evaluatorManifestSHA256, PromotionTimeout: 3 * time.Minute,
			KeepPhysicalPromotionArtifacts: opts.keepArtifacts,
		},
	})
	if err != nil {
		return err
	}
	data, err := capabilitybaselinev10.MarshalJSONStable(report)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeAtomicNoReplace(opts.reportPath, data); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(summary{
		Schema: "kicadai.closed-loop-open-set-discovery-baseline-run.v10", Version: 10,
		ReportPath: opts.reportPath, ReportHash: report.Hash, CaseCount: report.CaseCount, OutcomeCounts: report.OutcomeCounts,
	})
}

func normalizeOptions(opts *options) error {
	if opts.timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	root, err := filepath.Abs(opts.repositoryRoot)
	if err != nil {
		return err
	}
	opts.repositoryRoot = root
	defaults := map[*string]string{
		&opts.corpusRoot:        filepath.Join(root, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"),
		&opts.evaluatorManifest: filepath.Join(root, "specs", "closed-loop-open-set-capability-expansion", "V10_EVALUATOR.sha256"),
		&opts.toolchainLock:     filepath.Join(root, "toolchain", "kicad-promotion.lock.json"),
	}
	for target, fallback := range defaults {
		if strings.TrimSpace(*target) == "" {
			*target = fallback
		}
	}
	if strings.TrimSpace(opts.workingRoot) == "" || strings.TrimSpace(opts.reportPath) == "" {
		return fmt.Errorf("--working-root and --report are required")
	}
	for _, target := range []*string{&opts.corpusRoot, &opts.workingRoot, &opts.reportPath, &opts.evaluatorManifest, &opts.toolchainLock} {
		absolute, err := filepath.Abs(*target)
		if err != nil {
			return err
		}
		*target = absolute
	}
	if _, err := os.Lstat(opts.reportPath); err == nil {
		return fmt.Errorf("report path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func ensureCleanRepository(ctx context.Context, root string) error {
	topLevel, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("V10 evaluation requires the repository root")
	}
	rootInfo, rootErr := os.Stat(root)
	topInfo, topErr := os.Stat(strings.TrimSpace(string(topLevel)))
	if rootErr != nil || topErr != nil || !rootInfo.IsDir() || !topInfo.IsDir() || !os.SameFile(rootInfo, topInfo) {
		return fmt.Errorf("V10 evaluation requires the repository root")
	}
	status, err := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return fmt.Errorf("inspect repository state: %w", err)
	}
	if len(status) != 0 {
		return fmt.Errorf("V10 evaluation requires a clean committed tree")
	}
	return nil
}

func writeAtomicNoReplace(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".v10-baseline-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary V10 discovery report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	_, writeErr := temporary.Write(data)
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return fmt.Errorf("atomically publish V10 discovery report without replacement: %w", err)
	}
	return nil
}
