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

const evaluatorVersion = 21

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
	flags := flag.NewFlagSet("kicadai-discovery-baseline-v21", flag.ContinueOnError)
	var diagnostics bytes.Buffer
	flags.SetOutput(&diagnostics)
	var opts options
	flags.StringVar(&opts.repositoryRoot, "repository-root", ".", "clean committed repository root")
	flags.StringVar(&opts.corpusRoot, "corpus-root", "", "authenticated immutable V10 corpus root")
	flags.StringVar(&opts.workingRoot, "working-root", "", "fresh V21 root for per-case replay and promotion evidence")
	flags.StringVar(&opts.reportPath, "report", "", "fresh V21 generation-zero report path")
	flags.StringVar(&opts.evaluatorManifest, "evaluator-manifest", "", "frozen V21 evaluator checksum manifest")
	flags.StringVar(&opts.toolchainLock, "toolchain-lock", "", "locked KiCad promotion toolchain document")
	flags.DurationVar(&opts.timeout, "timeout", 6*time.Hour, "whole-cohort execution timeout")
	flags.BoolVar(&opts.keepArtifacts, "keep-artifacts", true, "retain installed-KiCad promotion evidence")
	flags.BoolVar(&opts.resume, "resume", false, "resume authenticated V21 completed-case checkpoints")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse V21 discovery evaluator arguments: %s", strings.TrimSpace(diagnostics.String()))
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("parse V21 discovery evaluator arguments: unexpected positional arguments")
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
		return fmt.Errorf("hash V21 evaluator manifest: %w", err)
	}
	legacyCatalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		return fmt.Errorf("load immutable legacy component catalog: %w", err)
	}
	legacyModels, legacyModelDiagnostics := modelprovenance.LoadDefault()
	if len(legacyModelDiagnostics) != 0 {
		return fmt.Errorf("load immutable legacy model provenance: %s", modelDiagnosticMessages(legacyModelDiagnostics))
	}
	legacyCatalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: legacyCatalog}).CatalogHash()
	legacyInventory, legacyInventoryIssues := opentopologysynthesis.BuildPrimitiveInventory(legacyCatalog, legacyCatalogHash, legacyModels)
	if reports.HasBlockingIssue(legacyInventoryIssues) {
		return fmt.Errorf("build immutable legacy primitive inventory: blocking inventory evidence")
	}
	legacySimulation := opentopologysynthesis.SimulationEnvironment{
		Catalog: legacyCatalog, CatalogHash: legacyCatalogHash, ModelRegistry: legacyModels,
	}

	v18Catalog, err := components.LoadCatalogV18(ctx)
	if err != nil {
		return fmt.Errorf("load exact V18 component catalog: %w", err)
	}
	v18Models, v18ModelDiagnostics := modelprovenance.LoadV18()
	if len(v18ModelDiagnostics) != 0 {
		return fmt.Errorf("load exact V18 model provenance: %s", modelDiagnosticMessages(v18ModelDiagnostics))
	}
	v18CatalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: v18Catalog}).CatalogHash()
	v18Inventory, v18InventoryIssues := opentopologysynthesis.BuildPrimitiveInventory(v18Catalog, v18CatalogHash, v18Models)
	if reports.HasBlockingIssue(v18InventoryIssues) {
		return fmt.Errorf("build exact V18 primitive inventory: blocking inventory evidence")
	}
	v18Simulation := opentopologysynthesis.SimulationEnvironment{
		Catalog: v18Catalog, CatalogHash: v18CatalogHash, ModelRegistry: v18Models,
	}

	// V18, V20, and V21 authenticate the same immutable V18 catalog, registry,
	// and inventory. Their executor arguments remain separate version boundaries,
	// but sharing this read-only load avoids repeated I/O and duplicate indexes.
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
	report, err := capabilityexecutorv10.NewV21WithLegacy(v18Inventory, v18Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation).RunV21(ctx, capabilityexecutorv10.Request{
		CorpusManifestSHA256: corpus.ManifestSHA256, OutputRoot: opts.workingRoot,
		Resume: opts.resume, Cases: corpus.Cases,
		Environment: capabilityexecutorv10.Environment{
			Inventory: v18Inventory, Simulation: v18Simulation,
			Policy: opentopologysynthesis.DefaultPolicy(), LibraryIndex: &index, KiCadCLI: toolEvidence.KiCadCLI,
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
	if err := writeAtomicNoReplace(opts.reportPath, data, true); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(summary{
		Schema: "kicadai.closed-loop-open-set-discovery-baseline-run.v21", Version: evaluatorVersion,
		ReportPath: opts.reportPath, ReportHash: report.Hash, CaseCount: report.CaseCount,
		OutcomeCounts: report.OutcomeCounts,
	})
}

func modelDiagnosticMessages(diagnostics []modelprovenance.Diagnostic) string {
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	return strings.Join(messages, "; ")
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
		&opts.evaluatorManifest: filepath.Join(root, "specs", "generic-causal-topology-repair", "V21_EVALUATOR.sha256"),
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
		return fmt.Errorf("report path already exists: %s", opts.reportPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func ensureCleanRepository(ctx context.Context, root string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("V21 evaluation requires git to verify its clean committed source tree: %w", err)
	}
	topLevel, err := exec.CommandContext(ctx, gitPath, "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("resolve V21 repository root with git: %w", err)
	}
	rootInfo, rootErr := os.Stat(root)
	topInfo, topErr := os.Stat(strings.TrimSpace(string(topLevel)))
	if rootErr != nil || topErr != nil || !rootInfo.IsDir() || !topInfo.IsDir() || !os.SameFile(rootInfo, topInfo) {
		return fmt.Errorf("V21 evaluation root is not the Git top-level directory")
	}
	status, err := exec.CommandContext(ctx, gitPath, "-C", root, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return fmt.Errorf("inspect repository state: %w", err)
	}
	if len(status) != 0 {
		return fmt.Errorf("V21 evaluation requires a clean committed tree")
	}
	return nil
}

func writeAtomicNoReplace(path string, data []byte, trailingNewline bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Create the fully written, synced source in the destination directory.
	// The final hard link is therefore same-filesystem and provides atomic
	// no-replace publication without exposing a partially written report.
	temporary, err := os.CreateTemp(filepath.Dir(path), ".v21-baseline-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary V21 discovery report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return err
	}
	if _, err := io.Copy(temporary, bytes.NewReader(data)); err != nil {
		return err
	}
	if trailingNewline {
		if _, err := io.WriteString(temporary, "\n"); err != nil {
			return err
		}
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return fmt.Errorf("atomically publish V21 discovery report without replacement: %w", err)
	}
	return nil
}
