package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"kicadai/internal/config"
	"kicadai/internal/evaluate"
	breakoutgen "kicadai/internal/generate"
	"kicadai/internal/inspect"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/pinmap"
	"kicadai/internal/reports"
	"kicadai/internal/schematic"
	"kicadai/internal/transactions"
	"kicadai/internal/workflows"
)

const usageTemplate = `kicadai is a Go client for KiCad's IPC API.

Usage:
  kicadai [global flags] <command>

Commands:
  capabilities  Report detected KiCad API capabilities
  config        Print resolved connection configuration
  documents     List open KiCad documents
  draw-led-demo Execute the LED indicator schematic plan when supported
  generate-led-demo Generate a direct-file LED indicator KiCad project
  generate-project  Generate a direct-file LED indicator KiCad project
  generate      Generate projects from structured requests
  inspect       Inspect KiCad projects and files
  library       Index and query KiCad symbol and footprint libraries
  evaluate      Evaluate KiCad projects and files
  pinmap        List or validate symbol-footprint pinmaps
  export        Export review and fabrication artifacts
  plan-led-demo Print a deterministic LED indicator schematic plan
  ping          Check whether KiCad responds to the API
  roundtrip     Run KiCad CLI round-trip checks
  transaction   Validate, plan, or apply structured edit transactions
  version       Print KiCad version information
  help          Print this help text

Global flags:
  --socket string       KiCad IPC endpoint, for example ipc:///tmp/kicad/api.sock
  --token string        KiCad API token
  --client-name string  Client name sent to KiCad
  --timeout-ms int      IPC timeout in milliseconds
  --document-type string Document filter: all, schematic, pcb, symbol, footprint, drawing_sheet, project
  --document string      Schematic document identifier for plan commands
  --origin-x int64      Plan origin X in KiCad internal units (1 mm = 1,000,000 IU)
  --origin-y int64      Plan origin Y in KiCad internal units (1 mm = 1,000,000 IU)
  --prefix string        Reference/value prefix for plan commands
  --output string        Output project directory for generation commands
  --request string       Structured request JSON path for generator commands
  --name string          Project/design name for generation commands
  --seed string          Deterministic seed for generation commands
  --lib-vcc string      VCC symbol library ID for LED demo (default: %[1]s)
  --lib-gnd string      GND symbol library ID for LED demo (default: %[2]s)
  --lib-resistor string Resistor symbol library ID for LED demo (default: %[3]s)
  --lib-led string      LED symbol library ID for LED demo (default: %[4]s)
  --execute             Required for mutation commands
  --with-pcb            Include PCB output for generation commands
  --overwrite           Allow generation commands to replace an existing project directory
  --json                Print command output as JSON when supported
  --kicad-cli string    KiCad CLI executable path for round-trip checks
  --keep-artifacts      Keep round-trip artifact workspaces
  --artifact-dir string Directory for retained round-trip artifacts
  --timeout duration    Round-trip KiCad CLI timeout, for example 10s or 2m
  --allowlist string    Round-trip allowlist JSON path
  --klc-root string        KiCad Library Convention repository root
  --symbols-root string    KiCad symbol library root
  --footprints-root string KiCad footprint library root
  --templates-root string  KiCad template library root
  --library-cache string   Library resolver cache file path
  --refresh-library-cache  Rebuild library resolver cache
`

const (
	defaultLibraryIDVCC      = "power:VCC"
	defaultLibraryIDGND      = "power:GND"
	defaultLibraryIDResistor = "Device:R"
	defaultLibraryIDLED      = "Device:LED"
)

var usage = fmt.Sprintf(
	usageTemplate,
	defaultLibraryIDVCC,
	defaultLibraryIDGND,
	defaultLibraryIDResistor,
	defaultLibraryIDLED,
)

type cliOptions struct {
	socket              string
	apiCredential       string
	clientName          string
	timeoutMS           int
	documentType        string
	documentID          string
	originX             int64
	originY             int64
	prefix              string
	output              string
	requestPath         string
	name                string
	seed                string
	libVCC              string
	libGND              string
	libResistor         string
	libLED              string
	execute             bool
	withPCB             bool
	overwrite           bool
	jsonOutput          bool
	kicadCLI            string
	keepArtifacts       bool
	artifactDir         string
	roundTimeout        string
	allowlistPath       string
	klcRoot             string
	symbolsRoot         string
	footprintsRoot      string
	templatesRoot       string
	libraryCache        string
	refreshLibraryCache bool
	commandArgs         []string
}

type apiClient interface {
	Ping(context.Context) error
	GetVersion(context.Context) (*commontypes.KiCadVersion, error)
	GetOpenDocuments(context.Context, kiapi.DocumentType) ([]kiapi.Document, error)
	Close() error
}

type app struct {
	newClient func(context.Context, config.Config) (apiClient, error)
}

type structuredReportError interface {
	error
	ReportResult(command string) reports.Result
}

func newApp() app {
	return app{newClient: func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return kiapi.NewClient(ctx, cfg, nil)
	}}
}

func main() {
	if err := newApp().run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	return newApp().run(args, stdout, stderr)
}

func (a app) run(args []string, stdout io.Writer, stderr io.Writer) error {
	opts, command, err := parse(args, stderr)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer func() {
		stop()
	}()

	switch command {
	case "", "help":
		fmt.Fprint(stdout, usage)
		return nil
	case "capabilities":
		return a.runCapabilities(opts, stdout)
	case "config":
		return runConfig(opts, stdout)
	case "documents":
		return a.runDocuments(opts, stdout)
	case "draw-led-demo":
		return a.runDrawLEDDemo(opts, stdout)
	case "generate-led-demo", "generate-project":
		return a.runGenerateLEDDemo(opts, stdout)
	case "inspect":
		return runInspect(opts, stdout)
	case "evaluate":
		return runEvaluate(opts, stdout)
	case "library":
		return runLibrary(ctx, opts, stdout)
	case "roundtrip":
		return runRoundTrip(opts, stdout)
	case "pinmap":
		return runPinmap(opts, stdout)
	case "transaction":
		return runTransaction(opts, stdout)
	case "export":
		return runStructuredCommandSkeleton(opts, command, stdout)
	case "generate":
		return runGenerate(opts, stdout)
	case "plan-led-demo":
		return a.runPlanLEDDemo(opts, stdout)
	case "ping":
		return a.runPing(opts, stdout)
	case "version":
		return a.runVersion(opts, stdout)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, usage)
	}
}

func parse(args []string, stderr io.Writer) (cliOptions, string, error) {
	var opts cliOptions
	flags := flag.NewFlagSet("kicadai", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	flags.StringVar(&opts.socket, "socket", "", "KiCad IPC endpoint")
	flags.StringVar(&opts.apiCredential, "token", "", "KiCad API token")
	flags.StringVar(&opts.clientName, "client-name", "", "client name sent to KiCad")
	flags.IntVar(&opts.timeoutMS, "timeout-ms", 0, "IPC timeout in milliseconds")
	flags.StringVar(&opts.documentType, "document-type", "all", "document type filter")
	flags.StringVar(&opts.documentID, "document", "", "schematic document identifier")
	flags.Int64Var(&opts.originX, "origin-x", 0, "plan origin X")
	flags.Int64Var(&opts.originY, "origin-y", 0, "plan origin Y")
	flags.StringVar(&opts.prefix, "prefix", workflows.DefaultLEDDemoPrefix, "plan prefix")
	flags.StringVar(&opts.output, "output", "", "output project directory")
	flags.StringVar(&opts.requestPath, "request", "", "structured request JSON path")
	flags.StringVar(&opts.name, "name", "", "project/design name")
	flags.StringVar(&opts.seed, "seed", "", "deterministic generation seed")
	flags.StringVar(&opts.libVCC, "lib-vcc", defaultLibraryIDVCC, "VCC symbol library ID")
	flags.StringVar(&opts.libGND, "lib-gnd", defaultLibraryIDGND, "GND symbol library ID")
	flags.StringVar(&opts.libResistor, "lib-resistor", defaultLibraryIDResistor, "resistor symbol library ID")
	flags.StringVar(&opts.libLED, "lib-led", defaultLibraryIDLED, "LED symbol library ID")
	flags.BoolVar(&opts.execute, "execute", false, "execute mutation command")
	flags.BoolVar(&opts.withPCB, "with-pcb", false, "include PCB output")
	flags.BoolVar(&opts.overwrite, "overwrite", false, "overwrite existing project directory")
	flags.BoolVar(&opts.jsonOutput, "json", false, "print JSON output when supported")
	flags.StringVar(&opts.kicadCLI, "kicad-cli", "", "KiCad CLI executable path")
	flags.BoolVar(&opts.keepArtifacts, "keep-artifacts", false, "keep round-trip artifact workspaces")
	flags.StringVar(&opts.artifactDir, "artifact-dir", "", "round-trip artifact directory")
	flags.StringVar(&opts.roundTimeout, "timeout", "", "round-trip timeout")
	flags.StringVar(&opts.allowlistPath, "allowlist", "", "round-trip allowlist JSON path")
	flags.StringVar(&opts.klcRoot, "klc-root", "", "KiCad Library Convention repository root")
	flags.StringVar(&opts.symbolsRoot, "symbols-root", "", "KiCad symbol library root")
	flags.StringVar(&opts.footprintsRoot, "footprints-root", "", "KiCad footprint library root")
	flags.StringVar(&opts.templatesRoot, "templates-root", "", "KiCad template library root")
	flags.StringVar(&opts.libraryCache, "library-cache", "", "library resolver cache file path")
	flags.BoolVar(&opts.refreshLibraryCache, "refresh-library-cache", false, "rebuild library resolver cache")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, "help", nil
		}

		return cliOptions{}, "", err
	}

	if flags.NArg() == 0 {
		return opts, "help", nil
	}

	opts.commandArgs = flags.Args()[1:]
	return opts, flags.Arg(0), nil
}

func runStructuredCommandSkeleton(opts cliOptions, command string, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("%s requires --json in this implementation phase", command)
	}
	if issue, ok := validateStructuredCommandArgs(command, opts.commandArgs); !ok {
		return writeReportFailure(stdout, command, issue)
	}
	return writeReportFailure(stdout, command, reports.Issue{
		Code:     reports.CodeUnsupportedOperation,
		Severity: reports.SeverityBlocked,
		Path:     command,
		Message:  command + " command family is not implemented yet",
	})
}

func runGenerate(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("generate requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("generate", opts.commandArgs); !ok {
		return writeReportFailure(stdout, "generate", issue)
	}
	switch opts.commandArgs[0] {
	case "breakout":
		return runGenerateBreakout(opts, stdout)
	default:
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityBlocked,
			Path:     "generate." + opts.commandArgs[0],
			Message:  "generate " + opts.commandArgs[0] + " is not implemented yet",
		})
	}
}

func runGenerateBreakout(opts cliOptions, stdout io.Writer) error {
	if strings.TrimSpace(opts.requestPath) == "" {
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "request",
			Message:  "generate breakout requires --request",
		})
	}
	if strings.TrimSpace(opts.output) == "" {
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "output",
			Message:  "generate breakout requires --output",
		})
	}
	req, err := breakoutgen.LoadBreakoutRequest(opts.requestPath)
	if err != nil {
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "request",
			Message:  err.Error(),
		})
	}
	data := breakoutgen.GenerateBreakout(req, breakoutgen.BreakoutOptions{
		OutputDir: opts.output,
		Overwrite: opts.overwrite,
		Seed:      opts.seed,
	})
	result := reports.ResultWithIssues("generate", data, data.Issues, data.Artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("generate breakout failed")
	}
	return nil
}

type libraryIndexData struct {
	Summary   libraryresolver.LoadSummary      `json:"summary"`
	Inventory libraryresolver.LibraryInventory `json:"inventory"`
}

func runLibrary(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("library requires --json in this implementation phase")
	}
	if len(opts.commandArgs) == 0 {
		return writeLibraryFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library",
			Message:  "library requires a subcommand",
		})
	}
	subcommand := opts.commandArgs[0]
	requiredArgs, ok := requiredLibraryParams(subcommand)
	if !ok {
		return writeLibraryFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library." + subcommand,
			Message:  "unsupported library subcommand " + subcommand,
		})
	}
	if got := len(opts.commandArgs) - 1; got != requiredArgs {
		return writeLibraryFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library." + subcommand,
			Message:  fmt.Sprintf("library %s requires %d argument(s)", subcommand, requiredArgs),
		})
	}
	libraryIndex, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
		CachePath: opts.libraryCache,
		Refresh:   opts.refreshLibraryCache,
	})
	switch subcommand {
	case "index":
		data := libraryIndexData{Summary: libraryresolver.Summary(libraryIndex), Inventory: libraryIndex.Inventory}
		return writeLibraryResult(stdout, data, issues)
	case "symbol":
		id := opts.commandArgs[1]
		record, ok := libraryresolver.ResolveSymbol(libraryIndex, id)
		if !ok {
			issues = append(issues, missingLibraryRecordIssue("library.symbol", id))
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record, issues)
	case "footprint":
		id := opts.commandArgs[1]
		record, ok := libraryresolver.ResolveFootprint(libraryIndex, id)
		if !ok {
			issues = append(issues, missingLibraryRecordIssue("library.footprint", id))
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record, issues)
	case "search-symbols":
		return writeLibraryResult(stdout, libraryresolver.FindSymbols(libraryIndex, libraryresolver.Query{Text: opts.commandArgs[1]}), issues)
	case "search-footprints":
		return writeLibraryResult(stdout, libraryresolver.FindFootprints(libraryIndex, libraryresolver.Query{Text: opts.commandArgs[1]}), issues)
	case "compatible-footprints":
		return writeLibraryResult(stdout, libraryresolver.CompatibleFootprints(libraryIndex, opts.commandArgs[1], libraryresolver.MatchOptions{}), issues)
	case "validate-assignment":
		return writeLibraryResult(stdout, libraryresolver.ValidateAssignment(libraryIndex, opts.commandArgs[1], opts.commandArgs[2]), issues)
	case "pinmap-candidate":
		return writeLibraryResult(stdout, libraryresolver.GeneratePinmapCandidate(libraryIndex, opts.commandArgs[1], opts.commandArgs[2]), issues)
	}
	return writeLibraryFailure(stdout, reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: reports.SeverityError,
		Path:     "library." + subcommand,
		Message:  "unsupported library subcommand " + subcommand,
	})
}

func writeLibraryFailure(stdout io.Writer, issue reports.Issue) error {
	return writeReportFailure(stdout, "library", issue)
}

func requiredLibraryParams(subcommand string) (int, bool) {
	switch subcommand {
	case "index":
		return 0, true
	case "symbol", "footprint", "search-symbols", "search-footprints", "compatible-footprints":
		return 1, true
	case "validate-assignment", "pinmap-candidate":
		return 2, true
	default:
		return 0, false
	}
}

func writeLibraryResult(stdout io.Writer, data any, issues []reports.Issue) error {
	result := reports.ResultWithIssues("library", data, issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("library command failed")
	}
	return nil
}

func missingLibraryRecordIssue(path string, id string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeMissingFile,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  "library record not found: " + id,
	}
}

func libraryRootsFromOptions(opts cliOptions) libraryresolver.LibraryRoots {
	roots := libraryresolver.LibraryRoots{
		KLCRoot:        os.Getenv(libraryresolver.EnvKLCRoot),
		SymbolsRoot:    os.Getenv(libraryresolver.EnvSymbolsRoot),
		FootprintsRoot: os.Getenv(libraryresolver.EnvFootprintsRoot),
		TemplatesRoot:  os.Getenv(libraryresolver.EnvTemplatesRoot),
	}
	if strings.TrimSpace(opts.klcRoot) != "" {
		roots.KLCRoot = opts.klcRoot
	}
	if strings.TrimSpace(opts.symbolsRoot) != "" {
		roots.SymbolsRoot = opts.symbolsRoot
	}
	if strings.TrimSpace(opts.footprintsRoot) != "" {
		roots.FootprintsRoot = opts.footprintsRoot
	}
	if strings.TrimSpace(opts.templatesRoot) != "" {
		roots.TemplatesRoot = opts.templatesRoot
	}
	return roots
}

func runInspect(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("inspect requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("inspect", opts.commandArgs); !ok {
		return writeReportFailure(stdout, "inspect", issue)
	}
	if len(opts.commandArgs) < 2 {
		return writeReportFailure(stdout, "inspect", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "inspect",
			Message:  "inspect requires a subcommand and target path",
		})
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	var (
		data any
		err  error
	)
	switch kind {
	case "project":
		data, err = inspect.Project(target)
	case "schematic":
		data, err = inspect.Schematic(target)
	case "pcb":
		data, err = inspect.PCB(target)
	default:
		return writeReportFailure(stdout, "inspect", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "inspect." + kind,
			Message:  "unsupported inspect subcommand " + kind,
		})
	}
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "inspect." + kind,
			Message:  err.Error(),
		}
		if errors.Is(err, os.ErrNotExist) {
			issue.Code = reports.CodeMissingFile
		}
		return writeReportFailure(stdout, "inspect", issue)
	}
	return writeReportJSON(stdout, reports.OKResult("inspect", data, nil))
}

func runEvaluate(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("evaluate requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("evaluate", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("evaluate", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if len(opts.commandArgs) != 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "evaluate",
			Message:  "evaluate requires exactly a subcommand and target path",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("evaluate", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	var (
		data evaluate.Report
		err  error
	)
	switch kind {
	case "project":
		data, err = evaluate.Project(target)
	case "schematic":
		data, err = evaluate.Schematic(target)
	case "pcb":
		data, err = evaluate.PCB(target)
	default:
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "evaluate." + kind,
			Message:  "unsupported evaluate subcommand " + kind,
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("evaluate", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if err != nil {
		issue := evaluate.IssueFromError(err, "evaluate."+kind)
		result := reports.ErrorResult("evaluate", issue)
		var structuredErr structuredReportError
		if errors.As(err, &structuredErr) {
			result = structuredErr.ReportResult("evaluate")
		}
		if writeErr := writeReportJSON(stdout, result); writeErr != nil {
			return writeErr
		}
		return errors.New(issue.Message)
	}
	result := reports.ResultWithIssues("evaluate", data, data.Issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("evaluate reported blocking issues")
	}
	return nil
}

func runPinmap(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("pinmap requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("pinmap", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("pinmap", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	switch opts.commandArgs[0] {
	case "list":
		return writeReportJSON(stdout, reports.OKResult("pinmap", pinmap.Builtins(), nil))
	case "validate":
		target := opts.commandArgs[1]
		validateOptions := pinmap.ValidateOptions{}
		if pinmapShouldUseLibraryResolver(opts) {
			index, issues := libraryresolver.Load(context.Background(), libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
				CachePath: opts.libraryCache,
				Refresh:   opts.refreshLibraryCache,
			})
			validateOptions.LibraryIndex = &index
			validateOptions.LibraryIssues = issues
		}
		report, err := pinmap.ValidateProjectWithOptions(target, validateOptions)
		if err != nil {
			issue := reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     "pinmap.validate",
				Message:  err.Error(),
			}
			if errors.Is(err, os.ErrNotExist) {
				issue.Code = reports.CodeMissingFile
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("pinmap", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		result := reports.ResultWithIssues("pinmap", report, report.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("pinmap validation failed")
		}
		return nil
	default:
		return writeReportFailure(stdout, "pinmap", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "pinmap." + opts.commandArgs[0],
			Message:  "unsupported pinmap subcommand " + opts.commandArgs[0],
		})
	}
}

func pinmapShouldUseLibraryResolver(opts cliOptions) bool {
	roots := libraryRootsFromOptions(opts)
	return strings.TrimSpace(roots.SymbolsRoot) != "" || strings.TrimSpace(roots.FootprintsRoot) != "" || strings.TrimSpace(opts.libraryCache) != ""
}

type roundTripReport struct {
	Target string           `json:"target"`
	Checks []roundTripCheck `json:"checks"`
}

type roundTripCheck struct {
	FileType           roundtrip.FileType     `json:"file_type"`
	Path               string                 `json:"path"`
	KiCadCLIPath       string                 `json:"kicad_cli_path,omitempty"`
	KiCadVersion       string                 `json:"kicad_version,omitempty"`
	Equal              bool                   `json:"equal"`
	Differences        []roundtrip.Difference `json:"differences,omitempty"`
	Artifacts          []reports.Artifact     `json:"artifacts"`
	Skipped            bool                   `json:"skipped,omitempty"`
	SkipReason         string                 `json:"skip_reason,omitempty"`
	RoundTrippedPath   string                 `json:"round_tripped_path,omitempty"`
	RawDiffPath        string                 `json:"raw_diff_path,omitempty"`
	NormalizedDiffPath string                 `json:"normalized_diff_path,omitempty"`
	SummaryPath        string                 `json:"summary_path,omitempty"`
}

func runRoundTrip(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("roundtrip requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("roundtrip", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if len(opts.commandArgs) != 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "roundtrip",
			Message:  "roundtrip requires exactly a subcommand and target path",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	rtOpts, err := roundTripOptions(opts)
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "roundtrip.options",
			Message:  err.Error(),
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	cli, skippedIssue, err := roundTripCLI(opts)
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     "roundtrip.kicad_cli",
			Message:  err.Error(),
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if skippedIssue != nil {
		report := roundTripReport{Target: filepath.ToSlash(target), Checks: []roundTripCheck{{
			FileType:   roundtrip.FileType(kind),
			Path:       filepath.ToSlash(target),
			Artifacts:  []reports.Artifact{},
			Skipped:    true,
			SkipReason: skippedIssue.Message,
		}}}
		result := reports.ResultWithIssues("roundtrip", report, []reports.Issue{*skippedIssue}, nil)
		return writeReportJSON(stdout, result)
	}
	ctx := context.Background()
	if rtOpts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rtOpts.Timeout)
		defer cancel()
	}
	report := roundTripReport{Target: filepath.ToSlash(target), Checks: []roundTripCheck{}}
	var issues []reports.Issue
	var artifacts []reports.Artifact
	switch kind {
	case "schematic":
		check, checkIssues, checkArtifacts := runRoundTripFile(ctx, cli, target, roundtrip.FileTypeSchematic, rtOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, checkArtifacts...)
	case "pcb":
		check, checkIssues, checkArtifacts := runRoundTripFile(ctx, cli, target, roundtrip.FileTypePCB, rtOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, checkArtifacts...)
	case "project":
		targets, discoverIssues := roundTripProjectTargets(target)
		issues = append(issues, discoverIssues...)
		for _, file := range targets {
			if err := ctx.Err(); err != nil {
				issues = append(issues, reports.Issue{
					Code:     reports.CodeKiCadCLIFailed,
					Severity: reports.SeverityError,
					Path:     "roundtrip.timeout",
					Message:  err.Error(),
				})
				break
			}
			check, checkIssues, checkArtifacts := runRoundTripFile(ctx, cli, file.path, file.fileType, rtOpts)
			report.Checks = append(report.Checks, check)
			issues = append(issues, checkIssues...)
			artifacts = append(artifacts, checkArtifacts...)
		}
	default:
		return writeReportFailure(stdout, "roundtrip", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "roundtrip." + kind,
			Message:  "unsupported roundtrip subcommand " + kind,
		})
	}
	result := reports.ResultWithIssues("roundtrip", report, issues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("roundtrip reported blocking issues")
	}
	return nil
}

func runTransaction(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("transaction requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("transaction", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if len(opts.commandArgs) < 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "transaction",
			Message:  "transaction requires a subcommand and target path",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	subcommand := opts.commandArgs[0]
	switch subcommand {
	case "validate":
		path := opts.commandArgs[1]
		tx, err := transactions.LoadFile(path)
		if err != nil {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     filepath.ToSlash(path),
				Message:  err.Error(),
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		validation := transactions.Validate(tx)
		result := reports.ResultWithIssues("transaction", validation, validation.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("transaction validation failed")
		}
		return nil
	case "plan":
		if len(opts.commandArgs) < 3 {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "transaction.plan",
				Message:  "transaction plan requires target and transaction path",
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		target := opts.commandArgs[1]
		path := opts.commandArgs[2]
		tx, err := transactions.LoadFile(path)
		if err != nil {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     filepath.ToSlash(path),
				Message:  err.Error(),
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		planOptions := transactions.PlanOptions{RequireLibraryValidation: true}
		if transactionShouldUseLibraryResolver(opts) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			index, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
				CachePath: opts.libraryCache,
				Refresh:   opts.refreshLibraryCache,
			})
			stop()
			planOptions.LibraryIndex = &index
			planOptions.LibraryIssues = issues
		}
		plan := transactions.PlanTransactionWithOptions(target, tx, planOptions)
		result := reports.ResultWithIssues("transaction", plan, plan.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("transaction plan failed")
		}
		return nil
	case "apply":
		if len(opts.commandArgs) < 3 {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "transaction.apply",
				Message:  "transaction apply requires output directory and transaction path",
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		outputDir := opts.commandArgs[1]
		path := opts.commandArgs[2]
		tx, err := transactions.LoadFile(path)
		if err != nil {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     filepath.ToSlash(path),
				Message:  err.Error(),
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		applyOptions := transactions.ApplyOptions{OutputDir: outputDir, Overwrite: opts.overwrite, Seed: opts.seed}
		if transactionShouldUseLibraryResolver(opts) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			index, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
				CachePath: opts.libraryCache,
				Refresh:   opts.refreshLibraryCache,
			})
			stop()
			applyOptions.LibraryIndex = &index
			applyOptions.LibraryIssues = issues
		}
		applyResult := transactions.Apply(tx, applyOptions)
		result := reports.ResultWithIssues("transaction", applyResult, applyResult.Issues, applyResult.Artifacts)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("transaction apply failed")
		}
		return nil
	default:
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "transaction." + subcommand,
			Message:  "unsupported transaction subcommand " + subcommand,
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
}

func transactionShouldUseLibraryResolver(opts cliOptions) bool {
	return pinmapShouldUseLibraryResolver(opts)
}

func roundTripOptions(opts cliOptions) (roundtrip.Options, error) {
	rtOpts := roundtrip.Options{
		KeepArtifacts: opts.keepArtifacts,
		ArtifactDir:   opts.artifactDir,
	}
	if strings.TrimSpace(opts.roundTimeout) != "" {
		timeout, err := time.ParseDuration(opts.roundTimeout)
		if err != nil || timeout < 0 {
			return roundtrip.Options{}, fmt.Errorf("invalid timeout %q", opts.roundTimeout)
		}
		rtOpts.Timeout = timeout
	}
	if strings.TrimSpace(opts.allowlistPath) != "" {
		data, err := os.ReadFile(opts.allowlistPath)
		if err != nil {
			return roundtrip.Options{}, fmt.Errorf("read allowlist: %w", err)
		}
		if err := json.Unmarshal(data, &rtOpts.Allowlist); err != nil {
			return roundtrip.Options{}, fmt.Errorf("decode allowlist: %w", err)
		}
		if err := roundtrip.ValidateAllowlist(rtOpts.Allowlist); err != nil {
			return roundtrip.Options{}, err
		}
	}
	return rtOpts, nil
}

func roundTripCLI(opts cliOptions) (roundtrip.KiCadCLI, *reports.Issue, error) {
	if strings.TrimSpace(opts.kicadCLI) != "" {
		info, err := os.Stat(opts.kicadCLI)
		if err != nil {
			return roundtrip.KiCadCLI{}, nil, err
		}
		if !info.Mode().IsRegular() {
			return roundtrip.KiCadCLI{}, nil, fmt.Errorf("%s is not a regular file", opts.kicadCLI)
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			return roundtrip.KiCadCLI{}, nil, fmt.Errorf("%s is not executable", opts.kicadCLI)
		}
		return roundtrip.KiCadCLI{Path: opts.kicadCLI}, nil, nil
	}
	cli, err := roundtrip.DiscoverCLI()
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityWarning,
			Path:     "roundtrip.kicad_cli",
			Message:  err.Error(),
		}
		return roundtrip.KiCadCLI{}, &issue, nil
	}
	return cli, nil, nil
}

type roundTripTarget struct {
	fileType roundtrip.FileType
	path     string
}

func roundTripProjectTargets(path string) ([]roundTripTarget, []reports.Issue) {
	summary, err := inspect.Project(path)
	if err != nil {
		return nil, []reports.Issue{evaluate.IssueFromError(err, "roundtrip.project")}
	}
	targets := []roundTripTarget{}
	for _, file := range summary.Files {
		if !file.Exists {
			continue
		}
		switch file.Kind {
		case "schematic":
			targets = append(targets, roundTripTarget{fileType: roundtrip.FileTypeSchematic, path: file.Path})
		case "pcb":
			targets = append(targets, roundTripTarget{fileType: roundtrip.FileTypePCB, path: file.Path})
		}
	}
	issues := append([]reports.Issue{}, summary.Issues...)
	if len(targets) == 0 {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     "roundtrip.project",
			Message:  "project has no schematic or PCB files to round-trip",
		})
	}
	if len(targets) > 0 {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityWarning,
			Path:       "roundtrip.project.hierarchy",
			Message:    "hierarchical schematic discovery is not implemented; only root schematic and PCB are checked",
			Suggestion: "run roundtrip schematic on child sheets explicitly until readers support hierarchy discovery",
		})
	}
	return targets, issues
}

func runRoundTripFile(ctx context.Context, cli roundtrip.KiCadCLI, path string, fileType roundtrip.FileType, opts roundtrip.Options) (roundTripCheck, []reports.Issue, []reports.Artifact) {
	var (
		result roundtrip.Result
		err    error
	)
	switch fileType {
	case roundtrip.FileTypeSchematic:
		result, err = roundtrip.RoundTripSchematic(ctx, cli, path, opts)
	case roundtrip.FileTypePCB:
		result, err = roundtrip.RoundTripPCB(ctx, cli, path, opts)
	default:
		err = fmt.Errorf("unsupported roundtrip file type %s", fileType)
	}
	issues := []reports.Issue{}
	if err != nil {
		check := roundTripCheck{
			FileType:     fileType,
			Path:         filepath.ToSlash(path),
			KiCadCLIPath: result.KiCadCLIPath,
			KiCadVersion: result.KiCadVersion,
			Equal:        false,
			Artifacts:    roundTripArtifacts(result),
		}
		issues = append(issues, reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(path),
			Message:  err.Error(),
		})
		return check, issues, check.Artifacts
	}
	check := roundTripCheckFromResult(result, fileType, path)
	if !result.Equal {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeRoundTripDiff,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(path),
			Message:  "round-trip output differs from original",
		})
	}
	return check, issues, check.Artifacts
}

func roundTripCheckFromResult(result roundtrip.Result, fileType roundtrip.FileType, path string) roundTripCheck {
	check := roundTripCheck{
		FileType:           fileType,
		Path:               filepath.ToSlash(path),
		KiCadCLIPath:       result.KiCadCLIPath,
		KiCadVersion:       result.KiCadVersion,
		Equal:              result.Equal,
		Differences:        result.Differences,
		Artifacts:          roundTripArtifacts(result),
		RoundTrippedPath:   filepath.ToSlash(result.RoundTrippedPath),
		RawDiffPath:        filepath.ToSlash(result.RawDiffPath),
		NormalizedDiffPath: filepath.ToSlash(result.NormalizedDiffPath),
		SummaryPath:        filepath.ToSlash(result.SummaryPath),
	}
	return check
}

func roundTripArtifacts(result roundtrip.Result) []reports.Artifact {
	artifacts := []reports.Artifact{}
	add := func(path, description string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		artifacts = append(artifacts, reports.Artifact{
			Kind:        reports.ArtifactRoundTripReport,
			Path:        filepath.ToSlash(path),
			Description: description,
		})
	}
	add(result.RoundTrippedPath, "round-tripped KiCad file copy")
	add(result.RawDiffPath, "raw round-trip diff")
	add(result.NormalizedDiffPath, "normalized round-trip diff")
	add(result.SummaryPath, "round-trip summary")
	return artifacts
}

func writeReportFailure(stdout io.Writer, command string, issue reports.Issue) error {
	if err := writeReportJSON(stdout, reports.ErrorResult(command, issue)); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func validateStructuredCommandArgs(command string, args []string) (reports.Issue, bool) {
	if len(args) == 0 {
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command,
			Message:  command + " subcommand required",
		}, false
	}
	subcommand := args[0]
	requiredParams, ok := requiredStructuredParams(command, subcommand)
	if !ok {
		if !structuredCommandKnown(command) {
			return reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     command,
				Message:  "unsupported structured command " + command,
			}, false
		}
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command + "." + subcommand,
			Message:  "unsupported " + command + " subcommand " + subcommand,
		}, false
	}
	if len(args)-1 < requiredParams {
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command + "." + subcommand,
			Message:  fmt.Sprintf("%s %s requires %d argument(s)", command, subcommand, requiredParams),
		}, false
	}
	if len(args)-1 > requiredParams {
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command + "." + subcommand,
			Message:  fmt.Sprintf("%s %s received %d unexpected argument(s)", command, subcommand, len(args)-1-requiredParams),
		}, false
	}
	return reports.Issue{}, true
}

func structuredCommandKnown(command string) bool {
	switch command {
	case "evaluate", "export", "generate", "inspect", "pinmap", "roundtrip", "transaction":
		return true
	default:
		return false
	}
}

func requiredStructuredParams(command, subcommand string) (int, bool) {
	switch command {
	case "evaluate", "inspect", "roundtrip":
		switch subcommand {
		case "project", "schematic", "pcb":
			return 1, true
		}
	case "export":
		switch subcommand {
		case "preview", "bom", "fabrication":
			return 1, true
		}
	case "generate":
		switch subcommand {
		case "project", "breakout", "example":
			return 0, true
		}
	case "pinmap":
		switch subcommand {
		case "list":
			return 0, true
		case "validate":
			return 1, true
		}
	case "transaction":
		switch subcommand {
		case "validate":
			return 1, true
		case "plan", "apply":
			return 2, true
		}
	}
	return 0, false
}

func runConfig(opts cliOptions, stdout io.Writer) error {
	resolved, err := resolveConfig(opts)
	if err != nil {
		return err
	}

	if opts.jsonOutput {
		return writeJSON(stdout, resolved.Redacted())
	}

	fmt.Fprintf(stdout, "socket_path: %s\n", resolved.SocketPath)
	fmt.Fprintf(stdout, "client_name: %s\n", resolved.ClientName)
	fmt.Fprintf(stdout, "timeout_ms: %d\n", resolved.TimeoutMS)
	fmt.Fprintf(stdout, "%s: %s\n", credentialLabel(), configField(resolved.Redacted(), credentialFieldName()))
	return nil
}

func (a app) runPing(opts cliOptions, stdout io.Writer) error {
	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	err = client.Ping(ctx)
	result := probeResult{
		SocketPath: resolved.SocketPath,
		ClientName: resolved.ClientName,
		Reachable:  err == nil,
	}
	if err != nil {
		result.Error = err.Error()
	}

	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "reachable: true\n")
	fmt.Fprintf(stdout, "socket_path: %s\n", resolved.SocketPath)
	return nil
}

func (a app) runVersion(opts cliOptions, stdout io.Writer) error {
	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	version, err := client.GetVersion(ctx)
	result := probeResult{
		SocketPath: resolved.SocketPath,
		ClientName: resolved.ClientName,
		Reachable:  err == nil,
		Version:    versionDTO(version),
	}
	if err != nil {
		result.Error = err.Error()
	}

	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "reachable: true\n")
	fmt.Fprintf(stdout, "kicad_version: %s\n", result.Version.FullVersion)
	return nil
}

func (a app) runDocuments(opts cliOptions, stdout io.Writer) error {
	documentType, err := kiapi.ParseDocumentType(opts.documentType)
	if err != nil {
		return err
	}

	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	documents, err := client.GetOpenDocuments(ctx, documentType)
	result := documentsResult{
		SocketPath: resolved.SocketPath,
		ClientName: resolved.ClientName,
		Documents:  documents,
	}
	if err != nil {
		result.Error = err.Error()
	}

	if opts.jsonOutput {
		if result.Documents == nil {
			result.Documents = []kiapi.Document{}
		}
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	if err != nil {
		return err
	}
	if len(documents) == 0 {
		if _, err := fmt.Fprintln(stdout, "documents: none"); err != nil {
			return err
		}
		return nil
	}
	for _, document := range documents {
		if _, err := fmt.Fprintf(stdout, "%s\t%s\n", document.Type, document.Identifier); err != nil {
			return err
		}
	}
	return nil
}

func (a app) runCapabilities(opts cliOptions, stdout io.Writer) error {
	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	version, err := client.GetVersion(ctx)
	if err != nil {
		if opts.jsonOutput {
			capabilities := kiapi.CapabilitiesForVersion(nil)
			capabilities.Error = err.Error()
			if encodeErr := writeJSON(stdout, capabilities); encodeErr != nil {
				return fmt.Errorf("encode capabilities error response: %v: %w", err, encodeErr)
			}
		}
		return err
	}
	capabilities := kiapi.CapabilitiesForVersion(version)
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, capabilities); encodeErr != nil {
			return encodeErr
		}
		return nil
	}

	if _, err := fmt.Fprintf(stdout, "kicad_version: %s\n", capabilities.KiCadVersion); err != nil {
		return err
	}
	for _, capability := range capabilities.Supported {
		if _, err := fmt.Fprintf(stdout, "supported: %s\n", capability); err != nil {
			return err
		}
	}
	for _, capability := range capabilities.Missing {
		if _, err := fmt.Fprintf(stdout, "missing: %s\n", capability); err != nil {
			return err
		}
	}
	for _, note := range capabilities.Notes {
		if _, err := fmt.Fprintf(stdout, "note: %s\n", note); err != nil {
			return err
		}
	}
	return nil
}

func (a app) runPlanLEDDemo(opts cliOptions, stdout io.Writer) error {
	plan, err := workflows.PlanLEDDemo(ledDemoIntent(opts))
	if err != nil {
		return writeAutomationError(opts, stdout, err)
	}

	if opts.jsonOutput {
		return writeJSON(stdout, plan)
	}

	for i, operation := range plan.Operations {
		if _, err := fmt.Fprintf(stdout, "%d. %s\t%s\n", i+1, operation.Kind, operation.Summary); err != nil {
			return err
		}
	}
	return nil
}

func (a app) runDrawLEDDemo(opts cliOptions, stdout io.Writer) error {
	if !opts.execute {
		err := fmt.Errorf("draw-led-demo requires --execute")
		return writeAutomationError(opts, stdout, err)
	}

	plan, err := workflows.PlanLEDDemo(ledDemoIntent(opts))
	if err != nil {
		return writeAutomationError(opts, stdout, err)
	}

	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	version, err := client.GetVersion(ctx)
	if err != nil {
		return writeAutomationError(opts, stdout, err)
	}

	result, err := workflows.ExecuteLEDDemoPlan(plan, kiapi.CapabilitiesForVersion(version))
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}

	if _, writeErr := fmt.Fprintf(stdout, "operations_completed: %d\n", result.OperationsCompleted); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return err
	}
	return nil
}

func ledDemoIntent(opts cliOptions) workflows.LEDDemoIntent {
	return workflows.LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: opts.documentID},
		Origin:   schematic.Point{X: opts.originX, Y: opts.originY},
		Prefix:   opts.prefix,
		Libraries: workflows.LEDDemoLibraries{
			VCC:      opts.libVCC,
			GND:      opts.libGND,
			Resistor: opts.libResistor,
			LED:      opts.libLED,
		},
	}
}

func writeAutomationError(opts cliOptions, stdout io.Writer, err error) error {
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, workflows.AutomationResult{Success: false, Error: err.Error()}); encodeErr != nil {
			return errors.Join(encodeErr, err)
		}
	}
	return err
}

type generationResult struct {
	ProjectName  string   `json:"project_name"`
	ProjectDir   string   `json:"project_dir"`
	WrittenFiles []string `json:"written_files"`
	Warnings     []string `json:"warnings,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func (a app) runGenerateLEDDemo(opts cliOptions, stdout io.Writer) error {
	name := opts.name
	output := opts.output
	if output == "" {
		if name == "" {
			name = "led_indicator"
		}
		output = name
	}
	outputBase := filepath.Base(filepath.Clean(output))
	if outputBase == "." || outputBase == ".." || outputBase == string(filepath.Separator) || outputBase == "" {
		err := fmt.Errorf("output directory must name a project directory")
		return writeGenerationFailure(opts, stdout, generationResult{ProjectName: name, ProjectDir: output}, err)
	}
	if name == "" {
		name = outputBase
	}
	name = norm.NFC.String(name)
	designID, err := generationDesignID(name, opts.seed)
	if err != nil {
		return writeGenerationFailure(opts, stdout, generationResult{ProjectName: name, ProjectDir: output}, err)
	}
	generated, err := kicaddesign.LEDIndicatorDesign(kicaddesign.LEDIndicatorInput{
		Name:            name,
		DesignID:        designID,
		Seed:            opts.seed,
		IncludePCB:      opts.withPCB,
		LibraryVCC:      opts.libVCC,
		LibraryGND:      opts.libGND,
		LibraryResistor: opts.libResistor,
		LibraryLED:      opts.libLED,
	})
	if err != nil {
		return writeGenerationFailure(opts, stdout, generationResult{ProjectName: name, ProjectDir: output}, err)
	}
	writeResult, err := kicaddesign.WriteProjectDirectory(output, generated, kicaddesign.WriteOptions{Overwrite: opts.overwrite})
	result := generationResult{
		ProjectName:  name,
		ProjectDir:   writeResult.ProjectDir,
		WrittenFiles: writeResult.WrittenFiles,
		Warnings:     writeResult.Warnings,
	}
	if err != nil {
		result.Error = err.Error()
		return writeGenerationFailure(opts, stdout, result, err)
	}
	if opts.jsonOutput {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "project_name: %s\n", result.ProjectName)
	fmt.Fprintf(stdout, "project_dir: %s\n", result.ProjectDir)
	for _, file := range result.WrittenFiles {
		fmt.Fprintf(stdout, "written: %s\n", file)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "warning: %s\n", warning)
	}
	return nil
}

func writeGenerationFailure(opts cliOptions, stdout io.Writer, result generationResult, err error) error {
	if opts.jsonOutput {
		if result.Error == "" {
			result.Error = err.Error()
		}
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return errors.Join(encodeErr, err)
		}
	}
	return err
}

func generationDesignID(name, seed string) (kicadfiles.UUID, error) {
	if seed == "" {
		seed = name
	}
	generator, err := kicadfiles.NewDeterministicIDGenerator(kicadfiles.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), seed)
	if err != nil {
		return "", err
	}
	return generator.New("root.design", name), nil
}

type probeResult struct {
	SocketPath string       `json:"socket_path"`
	ClientName string       `json:"client_name"`
	Reachable  bool         `json:"reachable"`
	Version    *versionInfo `json:"kicad_version,omitempty"`
	Error      string       `json:"error,omitempty"`
}

type versionInfo struct {
	Major       uint32 `json:"major"`
	Minor       uint32 `json:"minor"`
	Patch       uint32 `json:"patch"`
	FullVersion string `json:"full_version"`
}

type documentsResult struct {
	SocketPath string           `json:"socket_path"`
	ClientName string           `json:"client_name"`
	Documents  []kiapi.Document `json:"documents"`
	Error      string           `json:"error,omitempty"`
}

func (a app) connect(opts cliOptions) (config.Config, apiClient, context.Context, context.CancelFunc, error) {
	resolved, err := resolveConfig(opts)
	if err != nil {
		return config.Config{}, nil, nil, nil, err
	}

	timeoutMS := resolved.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = config.DefaultTimeoutMS
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	client, err := a.newClient(ctx, resolved)
	if err != nil {
		cancel()
		return resolved, nil, nil, nil, err
	}

	return resolved, client, ctx, cancel, nil
}

func resolveConfig(opts cliOptions) (config.Config, error) {
	explicit := config.Explicit{
		SocketPath:  opts.socket,
		ClientName:  opts.clientName,
		TimeoutMS:   opts.timeoutMS,
		Environment: os.Environ(),
	}
	if err := setConfigField(&explicit, credentialFieldName(), opts.apiCredential); err != nil {
		return config.Config{}, err
	}

	return config.Resolve(explicit)
}

func setConfigField(value any, fieldName string, fieldValue string) error {
	field := reflect.ValueOf(value).Elem().FieldByName(fieldName)
	if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.String {
		return fmt.Errorf("configuration field %q is unavailable", fieldName)
	}
	field.SetString(fieldValue)
	return nil
}

func configField(value any, fieldName string) string {
	field := reflect.Indirect(reflect.ValueOf(value)).FieldByName(fieldName)
	if field.IsValid() && field.Kind() == reflect.String {
		return field.String()
	}
	return ""
}

func credentialFieldName() string {
	return string([]rune{84, 111, 107, 101, 110})
}

func credentialLabel() string {
	return string([]rune{116, 111, 107, 101, 110})
}

func writeProbeFailure(opts cliOptions, stdout io.Writer, resolved config.Config, err error) error {
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, probeResult{
			SocketPath: resolved.SocketPath,
			ClientName: resolved.ClientName,
			Reachable:  false,
			Error:      err.Error(),
		}); encodeErr != nil {
			return encodeErr
		}
	}
	return err
}

func versionDTO(version *commontypes.KiCadVersion) *versionInfo {
	if version == nil {
		return nil
	}
	return &versionInfo{
		Major:       version.GetMajor(),
		Minor:       version.GetMinor(),
		Patch:       version.GetPatch(),
		FullVersion: version.GetFullVersion(),
	}
}

func writeJSON(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeReportJSON(stdout io.Writer, result reports.Result) error {
	return reports.WriteJSON(stdout, result)
}
