package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"kicadai/internal/blocks"
	"kicadai/internal/config"
	"kicadai/internal/evaluate"
	breakoutgen "kicadai/internal/generate"
	"kicadai/internal/inspect"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/checks"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/pinmap"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
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
  block         List, inspect, and validate built-in circuit blocks
  check         Run KiCad CLI ERC/DRC checks
  inspect       Inspect KiCad projects and files
  library       Index and query KiCad symbol and footprint libraries
  evaluate      Evaluate KiCad projects and files
  pinmap        List or validate symbol-footprint pinmaps
  place         Run PCB placement planning
  route         Run PCB routing
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
  --lib-vcc string      VCC symbol library ID for LED demo (default: {{defaultLibraryIDVCC}})
  --lib-gnd string      GND symbol library ID for LED demo (default: {{defaultLibraryIDGND}})
  --lib-resistor string Resistor symbol library ID for LED demo (default: {{defaultLibraryIDResistor}})
  --lib-led string      LED symbol library ID for LED demo (default: {{defaultLibraryIDLED}})
  --execute             Required for mutation commands
  --with-pcb            Include PCB output for generation commands
  --overwrite           Allow generation commands to replace an existing project directory
  --json                Print command output as JSON when supported
  --kicad-cli string    KiCad CLI executable path for KiCad-backed checks
  --keep-artifacts      Keep KiCad-backed artifact workspaces
  --artifact-dir string Directory for retained KiCad-backed artifacts
  --timeout duration    KiCad CLI timeout, for example 10s or 2m
  --allowlist string    Round-trip or ERC/DRC allowlist JSON path
  --klc-root string        KiCad Library Convention repository root
  --symbols-root string    KiCad symbol library root
  --footprints-root string KiCad footprint library root
  --templates-root string  KiCad template library root
  --library-cache string   Library resolver cache file path
  --refresh-library-cache  Rebuild library resolver cache
  --mode string                      Routing mode: single_layer, two_layer, validate_only
  --grid float                       Routing grid in millimeters
  --trace-width float                Routing trace width in millimeters
  --clearance float                  Routing clearance in millimeters
  --allow-partial                    Allow partial routing results
  --placement-board-width float      Placement feedback board width in millimeters (default {{defaultPlacementBoardWidthMM}})
  --placement-board-height float     Placement feedback board height in millimeters (default {{defaultPlacementBoardHeightMM}})
  --placement-board-margin float     Placement feedback board margin in millimeters (default {{defaultPlacementBoardMarginMM}})
  --placement-estimated-width float  Placement feedback estimated component width in millimeters (default {{defaultPlacementEstimatedWidthMM}})
  --placement-estimated-height float Placement feedback estimated component height in millimeters (default {{defaultPlacementEstimatedHeightMM}})
  --skip-placement-feedback          Skip placement feedback in block project generation output
  --pretty                           Pretty-print JSON output
`

const (
	defaultLibraryIDVCC      = "power:VCC"
	defaultLibraryIDGND      = "power:GND"
	defaultLibraryIDResistor = "Device:R"
	defaultLibraryIDLED      = "Device:LED"

	defaultPlacementBoardWidthMM      = 100.0
	defaultPlacementBoardHeightMM     = 60.0
	defaultPlacementBoardMarginMM     = 2.0
	defaultPlacementEstimatedWidthMM  = 2.0
	defaultPlacementEstimatedHeightMM = 1.25
)

var usage = strings.NewReplacer(
	"{{defaultLibraryIDVCC}}", defaultLibraryIDVCC,
	"{{defaultLibraryIDGND}}", defaultLibraryIDGND,
	"{{defaultLibraryIDResistor}}", defaultLibraryIDResistor,
	"{{defaultLibraryIDLED}}", defaultLibraryIDLED,
	"{{defaultPlacementBoardWidthMM}}", fmt.Sprintf("%g", defaultPlacementBoardWidthMM),
	"{{defaultPlacementBoardHeightMM}}", fmt.Sprintf("%g", defaultPlacementBoardHeightMM),
	"{{defaultPlacementBoardMarginMM}}", fmt.Sprintf("%g", defaultPlacementBoardMarginMM),
	"{{defaultPlacementEstimatedWidthMM}}", fmt.Sprintf("%g", defaultPlacementEstimatedWidthMM),
	"{{defaultPlacementEstimatedHeightMM}}", fmt.Sprintf("%g", defaultPlacementEstimatedHeightMM),
).Replace(usageTemplate)

type cliOptions struct {
	socket                string
	apiCredential         string
	clientName            string
	timeoutMS             int
	documentType          string
	documentID            string
	originX               int64
	originY               int64
	prefix                string
	output                string
	requestPath           string
	name                  string
	seed                  string
	libVCC                string
	libGND                string
	libResistor           string
	libLED                string
	execute               bool
	withPCB               bool
	overwrite             bool
	jsonOutput            bool
	kicadCLI              string
	keepArtifacts         bool
	artifactDir           string
	roundTimeout          string
	allowlistPath         string
	klcRoot               string
	symbolsRoot           string
	footprintsRoot        string
	templatesRoot         string
	libraryCache          string
	refreshLibraryCache   bool
	routeMode             string
	routeGridMM           float64
	routeTraceWidthMM     float64
	routeClearanceMM      float64
	routeAllowPartial     bool
	routeAllowPartialSet  bool
	placementBoardWidth   float64
	placementBoardHeight  float64
	placementBoardMargin  float64
	placementEstWidth     float64
	placementEstHeight    float64
	skipPlacementFeedback bool
	prettyOutput          bool
	commandArgs           []string
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
	case "check":
		return runCheckCommand(ctx, opts, stdout)
	case "roundtrip":
		return runRoundTrip(opts, stdout)
	case "pinmap":
		return runPinmap(opts, stdout)
	case "place":
		return runPlace(opts, stdout)
	case "route":
		return runRoute(opts, stdout)
	case "transaction":
		return runTransaction(opts, stdout)
	case "export":
		return runStructuredCommandSkeleton(opts, command, stdout)
	case "generate":
		return runGenerate(opts, stdout)
	case "block":
		return runBlock(ctx, opts, stdout)
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
	flags.StringVar(&opts.libraryCache, "library-cache", os.Getenv(libraryresolver.EnvLibraryCache), "library resolver cache file path")
	flags.BoolVar(&opts.refreshLibraryCache, "refresh-library-cache", false, "rebuild library resolver cache")
	flags.StringVar(&opts.routeMode, "mode", "", "routing mode")
	flags.Float64Var(&opts.routeGridMM, "grid", 0, "routing grid in millimeters")
	flags.Float64Var(&opts.routeTraceWidthMM, "trace-width", 0, "routing trace width in millimeters")
	flags.Float64Var(&opts.routeClearanceMM, "clearance", 0, "routing clearance in millimeters")
	flags.BoolVar(&opts.routeAllowPartial, "allow-partial", false, "allow partial routing results")
	flags.Float64Var(&opts.placementBoardWidth, "placement-board-width", defaultPlacementBoardWidthMM, "placement feedback board width in millimeters")
	flags.Float64Var(&opts.placementBoardHeight, "placement-board-height", defaultPlacementBoardHeightMM, "placement feedback board height in millimeters")
	flags.Float64Var(&opts.placementBoardMargin, "placement-board-margin", defaultPlacementBoardMarginMM, "placement feedback board margin in millimeters")
	flags.Float64Var(&opts.placementEstWidth, "placement-estimated-width", defaultPlacementEstimatedWidthMM, "placement feedback estimated component width in millimeters")
	flags.Float64Var(&opts.placementEstHeight, "placement-estimated-height", defaultPlacementEstimatedHeightMM, "placement feedback estimated component height in millimeters")
	flags.BoolVar(&opts.skipPlacementFeedback, "skip-placement-feedback", false, "skip placement feedback in block project generation output")
	flags.BoolVar(&opts.prettyOutput, "pretty", false, "pretty-print JSON output")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, "help", nil
		}

		return cliOptions{}, "", err
	}

	if err := validatePlacementFeedbackOptions(opts); err != nil {
		return opts, "", err
	}
	if flags.NArg() == 0 {
		return opts, "help", nil
	}

	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == "allow-partial" {
			opts.routeAllowPartialSet = true
		}
	})
	opts.commandArgs = flags.Args()[1:]
	return opts, flags.Arg(0), nil
}

func validatePlacementFeedbackOptions(opts cliOptions) error {
	if !positiveFiniteFloat(opts.placementBoardWidth) {
		return fmt.Errorf("--placement-board-width must be positive")
	}
	if !positiveFiniteFloat(opts.placementBoardHeight) {
		return fmt.Errorf("--placement-board-height must be positive")
	}
	if !nonNegativeFiniteFloat(opts.placementBoardMargin) {
		return fmt.Errorf("--placement-board-margin must be zero or positive")
	}
	if opts.placementBoardMargin*2 >= opts.placementBoardWidth || opts.placementBoardMargin*2 >= opts.placementBoardHeight {
		return fmt.Errorf("--placement-board-margin must leave positive usable board area")
	}
	if opts.placementBoardWidth-2*opts.placementBoardMargin < opts.placementEstWidth ||
		opts.placementBoardHeight-2*opts.placementBoardMargin < opts.placementEstHeight {
		return fmt.Errorf("placement board dimensions and margin must leave enough usable area for estimated component bounds")
	}
	if !positiveFiniteFloat(opts.placementEstWidth) {
		return fmt.Errorf("--placement-estimated-width must be positive")
	}
	if !positiveFiniteFloat(opts.placementEstHeight) {
		return fmt.Errorf("--placement-estimated-height must be positive")
	}
	return nil
}

func positiveFiniteFloat(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nonNegativeFiniteFloat(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
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

func runBlock(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("block requires --json in this implementation phase")
	}
	if len(opts.commandArgs) == 0 {
		return writeBlockFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "block",
			Message:  "block requires a subcommand",
		})
	}
	registry := blocks.NewBuiltinRegistry()
	subcommand := opts.commandArgs[0]
	var feedbackOptions blockPlacementFeedbackOptions
	feedbackOptionsReady := false
	blockFeedbackOptions := func() blockPlacementFeedbackOptions {
		if !feedbackOptionsReady {
			feedbackOptions = blockPlacementFeedbackOptionsFromCLI(ctx, opts)
			feedbackOptionsReady = true
		}
		return feedbackOptions
	}
	switch subcommand {
	case "list":
		if len(opts.commandArgs) != 1 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("list", 0))
		}
		return writeBlockResult(stdout, registry.ListBlocks(), nil)
	case "show":
		if len(opts.commandArgs) != 2 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("show", 1))
		}
		id := opts.commandArgs[1]
		definition, ok := registry.GetBlock(id)
		if !ok {
			return writeBlockResult(stdout, nil, []reports.Issue{missingBlockIssue(id)})
		}
		return writeBlockResult(stdout, definition, nil)
	case "validate":
		if len(opts.commandArgs) != 2 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("validate", 1))
		}
		id := opts.commandArgs[1]
		request, err := blockRequestFromOptions(opts, id)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "request",
				Message:  err.Error(),
			})
		}
		issues := registry.ValidateRequest(request)
		return writeBlockResult(stdout, request, issues)
	case "instantiate":
		if len(opts.commandArgs) != 2 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("instantiate", 1))
		}
		id := opts.commandArgs[1]
		request, err := blockRequestFromOptions(opts, id)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "request",
				Message:  err.Error(),
			})
		}
		output, issues := registry.Instantiate(ctx, request)
		if opts.output == "" || reports.HasBlockingIssue(issues) {
			return writeBlockResult(stdout, output, issues)
		}
		projectName := blockProjectName(opts, request.InstanceID)
		tx, err := blocks.ProjectTransactionForBlockOutput(projectName, output, opts.overwrite)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "block.instantiate",
				Message:  err.Error(),
			})
		}
		feedbackOptions := blockFeedbackOptions()
		placementApplication := blockPlacementApplicationForBlockOutput(ctx, tx, output, feedbackOptions)
		tx = placementApplication.Transaction
		applyResult := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: opts.output, Overwrite: opts.overwrite, Seed: opts.seed})
		resultIssues := combinedBlockIssues(issues, placementApplication.Issues)
		return writeBlockApplyResult(stdout, blockProjectGenerationResult{
			Output:      output,
			Transaction: tx,
			ApplyResult: applyResult,
			Feedback:    placementApplication.Feedback,
		}, combinedBlockIssues(resultIssues, applyResult.Issues), applyResult.Artifacts)
	case "compose":
		if len(opts.commandArgs) != 1 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("compose", 0))
		}
		request, err := compositionRequestFromOptions(opts)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "request",
				Message:  err.Error(),
			})
		}
		output := blocks.ComposeBlocks(ctx, registry, request)
		issues := output.Issues
		if opts.output == "" || reports.HasBlockingIssue(issues) {
			return writeBlockResult(stdout, output, issues)
		}
		projectName := blockProjectName(opts, output.ProjectName)
		tx, err := blocks.ProjectTransactionForCompositionOutput(projectName, output, opts.overwrite)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "block.compose",
				Message:  err.Error(),
			})
		}
		feedbackOptions := blockFeedbackOptions()
		placementApplication := compositionPlacementApplication(ctx, tx, output, feedbackOptions)
		tx = placementApplication.Transaction
		applyResult := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: opts.output, Overwrite: opts.overwrite, Seed: opts.seed})
		resultIssues := combinedBlockIssues(issues, placementApplication.Issues)
		return writeBlockApplyResult(stdout, compositionProjectGenerationResult{
			Output:      output,
			Transaction: tx,
			ApplyResult: applyResult,
			Feedback:    placementApplication.Feedback,
		}, combinedBlockIssues(resultIssues, applyResult.Issues), applyResult.Artifacts)
	default:
		return writeBlockFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "block." + subcommand,
			Message:  "unsupported block subcommand " + subcommand,
		})
	}
}

func blockRequestFromOptions(opts cliOptions, id string) (blocks.BlockRequest, error) {
	if strings.TrimSpace(opts.requestPath) == "" {
		request := blocks.BlockRequest{BlockID: id}
		if strings.TrimSpace(opts.output) != "" {
			request.InstanceID = blockProjectName(opts, id)
		}
		return request, nil
	}
	data, err := os.ReadFile(opts.requestPath)
	if err != nil {
		return blocks.BlockRequest{}, err
	}
	var request blocks.BlockRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return blocks.BlockRequest{}, err
	}
	if request.BlockID == "" {
		request.BlockID = id
	}
	if request.BlockID != id {
		return blocks.BlockRequest{}, fmt.Errorf("request block_id %q does not match command block ID %q", request.BlockID, id)
	}
	if request.InstanceID == "" && strings.TrimSpace(opts.output) != "" {
		request.InstanceID = blockProjectName(opts, id)
	}
	return request, nil
}

func compositionRequestFromOptions(opts cliOptions) (blocks.CompositionRequest, error) {
	if strings.TrimSpace(opts.requestPath) == "" {
		return blocks.CompositionRequest{}, fmt.Errorf("block compose requires --request")
	}
	data, err := os.ReadFile(opts.requestPath)
	if err != nil {
		return blocks.CompositionRequest{}, err
	}
	var request blocks.CompositionRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return blocks.CompositionRequest{}, err
	}
	if len(request.Instances) == 0 {
		return blocks.CompositionRequest{}, fmt.Errorf("composition request must contain at least one instance")
	}
	return request, nil
}

func blockProjectName(opts cliOptions, fallback string) string {
	if strings.TrimSpace(opts.name) != "" {
		return opts.name
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	if strings.TrimSpace(opts.output) != "" {
		base := filepath.Base(filepath.Clean(opts.output))
		if base != "." && base != ".." && strings.TrimSpace(base) != "" {
			return base
		}
	}
	return blocks.DefaultGeneratedProjectName
}

type blockProjectGenerationResult struct {
	Output      blocks.BlockOutput        `json:"output"`
	Transaction transactions.Transaction  `json:"transaction"`
	ApplyResult transactions.ApplyResult  `json:"apply_result"`
	Feedback    *workflows.DesignFeedback `json:"feedback,omitempty"`
}

type compositionProjectGenerationResult struct {
	Output      blocks.CompositionOutput  `json:"output"`
	Transaction transactions.Transaction  `json:"transaction"`
	ApplyResult transactions.ApplyResult  `json:"apply_result"`
	Feedback    *workflows.DesignFeedback `json:"feedback,omitempty"`
}

type blockPlacementFeedbackOptions struct {
	Adapter placement.AdapterOptions
	Issues  []reports.Issue
	Enabled bool
}

type blockPlacementApplication struct {
	Transaction transactions.Transaction
	Feedback    *workflows.DesignFeedback
	Issues      []reports.Issue
}

func blockPlacementApplicationForBlockOutput(ctx context.Context, tx transactions.Transaction, output blocks.BlockOutput, feedbackOptions blockPlacementFeedbackOptions) blockPlacementApplication {
	if !feedbackOptions.Enabled {
		return blockPlacementApplication{Transaction: tx}
	}
	request, issues := placement.RequestFromBlockOutput(output, feedbackOptions.Adapter)
	issues = combinedPlacementFeedbackIssues(feedbackOptions.Issues, issues)
	return blockPlacementApplicationForRequest(ctx, tx, request, issues)
}

func compositionPlacementApplication(ctx context.Context, tx transactions.Transaction, output blocks.CompositionOutput, feedbackOptions blockPlacementFeedbackOptions) blockPlacementApplication {
	if !feedbackOptions.Enabled {
		return blockPlacementApplication{Transaction: tx}
	}
	request, issues := placement.RequestFromCompositionOutput(output, feedbackOptions.Adapter)
	issues = combinedPlacementFeedbackIssues(feedbackOptions.Issues, issues)
	return blockPlacementApplicationForRequest(ctx, tx, request, issues)
}

func blockPlacementApplicationForRequest(ctx context.Context, tx transactions.Transaction, request placement.Request, adapterIssues []reports.Issue) blockPlacementApplication {
	if reports.HasBlockingIssue(adapterIssues) {
		feedback := placementFeedbackForBlockedRequest(request, adapterIssues)
		return blockPlacementApplication{Transaction: tx, Feedback: feedback, Issues: cloneIssues(adapterIssues)}
	}
	result := placement.PlaceContext(ctx, request)
	result.Issues = combinedPlacementFeedbackIssues(adapterIssues, result.Issues)
	if result.Status != placement.StatusPlaced || reports.HasBlockingIssue(result.Issues) {
		feedback := workflows.EvaluatePlacement(request, result)
		return blockPlacementApplication{Transaction: tx, Feedback: &feedback, Issues: result.Issues}
	}
	operations, operationIssues := placement.PlacementOperations(request, result.Placements)
	result.Issues = combinedPlacementFeedbackIssues(result.Issues, operationIssues)
	if len(operations) != 0 && !reports.HasBlockingIssue(operationIssues) {
		tx = replaceSupersededPlacementOperationsBeforeWriteProject(tx, operations)
	}
	feedback := workflows.EvaluatePlacement(request, result)
	return blockPlacementApplication{Transaction: tx, Feedback: &feedback, Issues: result.Issues}
}

func replaceSupersededPlacementOperationsBeforeWriteProject(tx transactions.Transaction, operations []transactions.Operation) transactions.Transaction {
	if len(operations) == 0 {
		return tx
	}
	replacementRefs := placementOperationRefs(operations)
	if len(replacementRefs) == 0 {
		return tx
	}
	filtered := make([]transactions.Operation, 0, len(tx.Operations))
	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpPlaceFootprint {
			ref, err := placementOperationRef(operation)
			if err == nil {
				if _, replace := replacementRefs[normalizedPlacementRef(ref)]; replace {
					continue
				}
			}
		}
		filtered = append(filtered, operation)
	}
	insertAt := len(filtered)
	for index := len(filtered) - 1; index >= 0; index-- {
		if filtered[index].Op == transactions.OpWriteProject {
			insertAt = index
			break
		}
	}
	next := make([]transactions.Operation, 0, len(filtered)+len(operations))
	next = append(next, filtered[:insertAt]...)
	next = append(next, operations...)
	next = append(next, filtered[insertAt:]...)
	tx.Operations = next
	return tx
}

func placementOperationRefs(operations []transactions.Operation) map[string]struct{} {
	refs := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		ref, err := placementOperationRef(operation)
		if err != nil {
			continue
		}
		key := normalizedPlacementRef(ref)
		if key != "" {
			refs[key] = struct{}{}
		}
	}
	return refs
}

func placementOperationRef(operation transactions.Operation) (string, error) {
	if operation.Op != transactions.OpPlaceFootprint {
		return "", fmt.Errorf("operation is not place_footprint")
	}
	var payload struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		return "", err
	}
	ref := strings.TrimSpace(payload.Ref)
	if ref == "" {
		return "", fmt.Errorf("placement operation missing ref")
	}
	return ref, nil
}

func normalizedPlacementRef(ref string) string {
	return strings.ToUpper(strings.TrimSpace(ref))
}

func combinedPlacementFeedbackIssues(first []reports.Issue, second []reports.Issue) []reports.Issue {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	if len(first) == 0 {
		return cloneIssues(second)
	}
	if len(second) == 0 {
		return cloneIssues(first)
	}
	combined := make([]reports.Issue, 0, len(first)+len(second))
	combined = append(combined, first...)
	combined = append(combined, second...)
	return combined
}

func cloneIssues(issues []reports.Issue) []reports.Issue {
	return slices.Clone(issues)
}

func placementFeedbackForBlockedRequest(request placement.Request, issues []reports.Issue) *workflows.DesignFeedback {
	result := placement.Result{
		Status: placement.StatusBlocked,
		Issues: issues,
		Metrics: placement.Metrics{
			ComponentCount: len(request.Components),
			UnplacedCount:  len(request.Components),
		},
	}
	feedback := workflows.EvaluatePlacement(request, result)
	return &feedback
}

func blockPlacementFeedbackOptionsFromCLI(ctx context.Context, opts cliOptions) blockPlacementFeedbackOptions {
	adapterOptions := blockPlacementAdapterOptions(opts)
	if opts.skipPlacementFeedback {
		return blockPlacementFeedbackOptions{Adapter: adapterOptions}
	}
	roots := libraryRootsFromOptions(opts)
	index, ok, issues := blockPlacementLibraryIndex(ctx, roots, opts)
	if ok {
		adapterOptions.LibraryIndex = &index
	}
	return blockPlacementFeedbackOptions{Adapter: adapterOptions, Issues: issues, Enabled: true}
}

func blockPlacementAdapterOptions(opts cliOptions) placement.AdapterOptions {
	adapterOptions := placement.AdapterOptions{
		Board: placement.BoardPlacementArea{
			WidthMM:  opts.placementBoardWidth,
			HeightMM: opts.placementBoardHeight,
			MarginMM: opts.placementBoardMargin,
		},
		Rules: placement.DefaultRules(),
		DefaultBounds: placement.Bounds{
			WidthMM:  opts.placementEstWidth,
			HeightMM: opts.placementEstHeight,
			Source:   placement.BoundsEstimated,
		},
	}
	return adapterOptions
}

func blockPlacementLibraryIndex(ctx context.Context, roots libraryresolver.LibraryRoots, opts cliOptions) (libraryresolver.LibraryIndex, bool, []reports.Issue) {
	if !blockPlacementShouldUseLibraryResolver(roots) {
		return libraryresolver.LibraryIndex{}, false, nil
	}
	loadedIndex, issues := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{
		CachePath: opts.libraryCache,
		Refresh:   opts.refreshLibraryCache,
	})
	if reports.HasBlockingIssue(issues) {
		return libraryresolver.LibraryIndex{}, false, issues
	}
	if len(loadedIndex.Footprints) == 0 {
		return libraryresolver.LibraryIndex{}, false, issues
	}
	return loadedIndex, true, issues
}

func blockPlacementShouldUseLibraryResolver(roots libraryresolver.LibraryRoots) bool {
	return strings.TrimSpace(roots.FootprintsRoot) != ""
}

func writeBlockResult(stdout io.Writer, data any, issues []reports.Issue) error {
	result := reports.ResultWithIssues("block", data, issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("block command failed")
	}
	return nil
}

func writeBlockApplyResult(stdout io.Writer, data any, issues []reports.Issue, artifacts []reports.Artifact) error {
	result := reports.ResultWithIssues("block", data, issues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("block command failed")
	}
	return nil
}

func combinedBlockIssues(groups ...[]reports.Issue) []reports.Issue {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	combined := make([]reports.Issue, 0, total)
	for _, group := range groups {
		combined = append(combined, group...)
	}
	return combined
}

func writeBlockFailure(stdout io.Writer, issue reports.Issue) error {
	if err := writeReportJSON(stdout, reports.ErrorResult("block", issue)); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func missingBlockIssue(id string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeMissingFile,
		Severity: reports.SeverityError,
		Path:     "block_id",
		Message:  "block not found: " + id,
	}
}

func invalidBlockArgCountIssue(subcommand string, required int) reports.Issue {
	message := fmt.Sprintf("block %s requires %d argument(s)", subcommand, required)
	if required == 0 {
		message = "block " + subcommand + " does not accept arguments"
	} else if required == 1 {
		message = "block " + subcommand + " requires 1 argument"
	}
	return reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: reports.SeverityError,
		Path:     "block." + subcommand,
		Message:  message,
	}
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
	if subcommand == "templates" || subcommand == "template" {
		if subcommand == "templates" {
			records, issues := libraryresolver.DiscoverTemplates(ctx, libraryRootsFromOptions(opts))
			return writeLibraryResult(stdout, records, issues)
		}
		name := opts.commandArgs[1]
		record, issues, ok := libraryresolver.DiscoverTemplate(ctx, libraryRootsFromOptions(opts), name)
		if !ok {
			if !hasBlockingIssue(issues) {
				issues = append(issues, missingLibraryRecordIssue("library.template", name))
			}
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record, issues)
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
	case "klc-symbol":
		return writeLibraryKLCResult(stdout, libraryresolver.ValidateSymbolKLC(libraryIndex, opts.commandArgs[1]), issues)
	case "klc-footprint":
		return writeLibraryKLCResult(stdout, libraryresolver.ValidateFootprintKLC(libraryIndex, opts.commandArgs[1]), issues)
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

func writeLibraryKLCResult(stdout io.Writer, report libraryresolver.KLCReport, issues []reports.Issue) error {
	reportIssues := report.Issues
	report.Issues = nil
	return writeLibraryResult(stdout, report, append(issues, reportIssues...))
}

func requiredLibraryParams(subcommand string) (int, bool) {
	switch subcommand {
	case "index", "templates":
		return 0, true
	case "symbol", "footprint", "search-symbols", "search-footprints", "compatible-footprints", "klc-symbol", "klc-footprint", "template":
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

func hasBlockingIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
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

func runPlace(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("place requires --json in this implementation phase")
	}
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "place", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "place",
			Message:  "place requires a subcommand",
		})
	}
	switch opts.commandArgs[0] {
	case "request":
		if opts.requestPath == "" {
			return writeReportFailure(stdout, "place", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "place.request",
				Message:  "place request requires --request",
			})
		}
		file, err := os.Open(opts.requestPath)
		if err != nil {
			return writeReportFailure(stdout, "place", reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		defer file.Close()
		var request placement.Request
		if err := json.NewDecoder(file).Decode(&request); err != nil {
			return writeReportFailure(stdout, "place", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		result := placement.Place(request)
		quality := placement.BuildQualityReport(request, result)
		result.Quality = &quality
		return writeReportJSON(stdout, reports.ResultWithIssues("place", result, result.Issues, nil))
	default:
		return writeReportFailure(stdout, "place", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "place." + opts.commandArgs[0],
			Message:  "unsupported place subcommand " + opts.commandArgs[0],
		})
	}
}

func runRoute(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("route requires --json in this implementation phase")
	}
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "route", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "route",
			Message:  "route requires a subcommand",
		})
	}
	switch opts.commandArgs[0] {
	case "request":
		if opts.requestPath == "" {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "route.request",
				Message:  "route request requires --request",
			})
		}
		file, err := os.Open(opts.requestPath)
		if err != nil {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		defer file.Close()
		var request routing.Request
		if err := json.NewDecoder(file).Decode(&request); err != nil {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		if err := applyRouteOverrides(&request, opts); err != nil {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "route",
				Message:  err.Error(),
			})
		}
		result := routing.RouteRequest(request)
		report := reports.ResultWithIssues("route", result, result.Issues, nil)
		if opts.prettyOutput {
			return writePrettyReportJSON(stdout, report)
		}
		return writeReportJSON(stdout, report)
	default:
		return writeReportFailure(stdout, "route", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "route." + opts.commandArgs[0],
			Message:  "unsupported route subcommand " + opts.commandArgs[0],
		})
	}
}

func applyRouteOverrides(request *routing.Request, opts cliOptions) error {
	if opts.routeMode != "" {
		switch routing.RouteMode(opts.routeMode) {
		case routing.ModeSingleLayer, routing.ModeTwoLayer, routing.ModeValidateOnly:
		default:
			return fmt.Errorf("unsupported route mode %q", opts.routeMode)
		}
		request.Strategy.Mode = routing.RouteMode(opts.routeMode)
	}
	if opts.routeGridMM > 0 {
		request.Rules.GridMM = opts.routeGridMM
	}
	if opts.routeTraceWidthMM > 0 {
		request.Rules.TraceWidthMM = opts.routeTraceWidthMM
	}
	if opts.routeClearanceMM > 0 {
		request.Rules.ClearanceMM = opts.routeClearanceMM
	}
	if opts.routeAllowPartialSet {
		request.Strategy.AllowPartial = opts.routeAllowPartial
	}
	return nil
}

type checkCommandReport struct {
	Target string               `json:"target"`
	Checks []checks.CheckResult `json:"checks"`
}

func runCheckCommand(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("check requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("check", opts.commandArgs); !ok {
		return writeReportFailure(stdout, "check", issue)
	}
	if len(opts.commandArgs) < 2 {
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "check",
			Message:  "check requires a subcommand and target path",
		})
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	checkOpts, err := checkOptions(opts)
	if err != nil {
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "check.options",
			Message:  err.Error(),
		})
	}
	cli, skippedIssue, err := checkCLI(opts)
	if err != nil {
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     "check.kicad_cli",
			Message:  err.Error(),
		})
	}
	if skippedIssue != nil {
		report := checkCommandReport{Target: filepath.ToSlash(target), Checks: []checks.CheckResult{{
			TargetPath: filepath.ToSlash(target),
			Status:     checks.CheckStatusSkipped,
		}}}
		result := reports.ResultWithIssues("check", report, []reports.Issue{*skippedIssue}, nil)
		return writeReportJSON(stdout, result)
	}

	report := checkCommandReport{Target: filepath.ToSlash(target), Checks: []checks.CheckResult{}}
	var issues []reports.Issue
	var artifacts []reports.Artifact
	switch kind {
	case "erc":
		check, checkIssues, resultArtifacts := runCheckERC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, resultArtifacts...)
	case "drc":
		check, checkIssues, resultArtifacts := runCheckDRC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, resultArtifacts...)
	case "project":
		erc, ercIssues, ercArtifacts := runCheckERC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, erc)
		issues = append(issues, ercIssues...)
		artifacts = append(artifacts, ercArtifacts...)
		if err := ctx.Err(); err != nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeKiCadCLIFailed,
				Severity: reports.SeverityError,
				Path:     "check.project",
				Message:  err.Error(),
			})
			break
		}
		drc, drcIssues, drcArtifacts := runCheckDRC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, drc)
		issues = append(issues, drcIssues...)
		artifacts = append(artifacts, drcArtifacts...)
	default:
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "check." + kind,
			Message:  "unsupported check subcommand " + kind,
		})
	}
	result := reports.ResultWithIssues("check", report, issues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("check reported blocking issues")
	}
	return nil
}

func checkOptions(opts cliOptions) (checks.Options, error) {
	checkOpts := checks.DefaultOptions()
	checkOpts.KiCadCLI = opts.kicadCLI
	checkOpts.KeepArtifacts = opts.keepArtifacts
	checkOpts.ArtifactDir = opts.artifactDir
	if strings.TrimSpace(opts.roundTimeout) != "" {
		timeout, err := time.ParseDuration(opts.roundTimeout)
		if err != nil || timeout < 0 {
			return checks.Options{}, fmt.Errorf("invalid timeout %q", opts.roundTimeout)
		}
		checkOpts.Timeout = timeout
	}
	if strings.TrimSpace(opts.allowlistPath) != "" {
		data, err := os.ReadFile(opts.allowlistPath)
		if err != nil {
			return checks.Options{}, fmt.Errorf("read allowlist: %w", err)
		}
		if err := json.Unmarshal(data, &checkOpts.Allowlist); err != nil {
			return checks.Options{}, fmt.Errorf("decode allowlist: %w", err)
		}
	}
	return checkOpts, nil
}

func checkCLI(opts cliOptions) (checks.KiCadCLI, *reports.Issue, error) {
	cli, err := checks.DiscoverCLI(opts.kicadCLI)
	if err != nil {
		if strings.TrimSpace(opts.kicadCLI) != "" {
			return checks.KiCadCLI{}, nil, err
		}
		issue := reports.Issue{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityWarning,
			Path:     "check.kicad_cli",
			Message:  err.Error(),
		}
		return checks.KiCadCLI{}, &issue, nil
	}
	return cli, nil, nil
}

func runCheckERC(ctx context.Context, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	result, err := checks.RunERC(ctx, cli, target, opts)
	return checkResultWithIssues(result, err)
}

func runCheckDRC(ctx context.Context, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	result, err := checks.RunDRC(ctx, cli, target, opts)
	return checkResultWithIssues(result, err)
}

func checkResultWithIssues(result checks.CheckResult, err error) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	issues := []reports.Issue{}
	for _, finding := range result.Findings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   checkSeverity(finding.Severity),
			Path:       filepath.ToSlash(finding.File),
			Message:    finding.Message,
			Refs:       finding.References,
			Nets:       checkFindingNets(finding),
			Suggestion: "repair category: " + string(finding.RepairCategory),
		})
	}
	for _, parserIssue := range result.ParserIssues {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     result.ReportPath,
			Message:  parserIssue.Message,
		})
	}
	if err != nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     result.TargetPath,
			Message:  err.Error(),
		})
	}
	return result, issues, checkArtifacts(result)
}

func checkFindingNets(finding checks.CheckFinding) []string {
	seen := map[string]struct{}{}
	nets := make([]string, 0, len(finding.Nets)+1)
	add := func(net string) {
		net = strings.TrimSpace(net)
		if net == "" {
			return
		}
		key := net
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		nets = append(nets, net)
	}
	for _, net := range finding.Nets {
		add(net)
	}
	if strings.TrimSpace(finding.Net) != "" {
		add(finding.Net)
	}
	return nets
}

func checkSeverity(severity string) reports.Severity {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "warning", "warn", "exclusion", "excluded":
		return reports.SeverityWarning
	case "info", "notice":
		return reports.SeverityInfo
	default:
		return reports.SeverityError
	}
}

func checkArtifacts(result checks.CheckResult) []reports.Artifact {
	if strings.TrimSpace(result.ReportPath) == "" {
		return nil
	}
	kind := reports.ArtifactERCReport
	if result.Kind == checks.CheckKindDRC {
		kind = reports.ArtifactDRCReport
	}
	return []reports.Artifact{{
		Kind:        kind,
		Path:        filepath.ToSlash(result.ReportPath),
		Description: string(result.Kind) + " JSON report",
	}}
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
	case "check", "evaluate", "export", "generate", "inspect", "pinmap", "roundtrip", "transaction":
		return true
	default:
		return false
	}
}

func requiredStructuredParams(command, subcommand string) (int, bool) {
	switch command {
	case "check":
		switch subcommand {
		case "erc", "drc", "project":
			return 1, true
		}
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
	fmt.Fprintf(stdout, "token: %s\n", resolved.Redacted().Token)
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
		Token:       opts.apiCredential,
		Environment: os.Environ(),
	}

	return config.Resolve(explicit)
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

func writePrettyReportJSON(stdout io.Writer, result reports.Result) error {
	return writeJSON(stdout, result)
}
